package mediaprocessing

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// probeStream mirrors ffprobeStream with exported field names for fixtures.
type fixtureStream struct {
	CodecType    string `json:"codec_type"`
	CodecName    string `json:"codec_name"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Duration     string `json:"duration"`
	AvgFrameRate string `json:"avg_frame_rate"`
	RFrameRate   string `json:"r_frame_rate"`
}

type fixtureFormat struct {
	Duration string `json:"duration"`
}

type fixtureProbe struct {
	Streams []fixtureStream `json:"streams"`
	Format  fixtureFormat   `json:"format"`
}

func videoStream(codec string, w, h int, duration, fps string) fixtureStream {
	return fixtureStream{CodecType: "video", CodecName: codec, Width: w, Height: h, Duration: duration, AvgFrameRate: fps, RFrameRate: fps}
}

func audioStream(codec string) fixtureStream {
	return fixtureStream{CodecType: "audio", CodecName: codec}
}

func probeJSON(streams []fixtureStream, formatDuration string) string {
	p := fixtureProbe{Streams: streams, Format: fixtureFormat{Duration: formatDuration}}
	b, err := json.Marshal(p)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func validVideoProbe() string {
	return probeJSON([]fixtureStream{
		videoStream("h264", 1280, 720, "10.000000", "30000/1001"),
		audioStream("aac"),
	}, "10.000000")
}

// tempInput writes a file of the given size and returns its path, so the
// os.Stat size check inside Validate is deterministic without a real video.
func tempInput(t *testing.T, size int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input.bin")
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		t.Fatalf("write temp input: %v", err)
	}
	return path
}

func mustValidate(t *testing.T, probe string, srcPath string, maxBytes int64) (*VideoSpec, error) {
	t.Helper()
	runner := &fakeRunner{stdout: []byte(probe), exitCode: 0}
	return Validate(context.Background(), srcPath, maxBytes, runner)
}

func assertCode(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error code %q, got nil", want)
	}
	if got := ErrorCode(err); got != want {
		t.Fatalf("error code = %q, want %q (err: %v)", got, want, err)
	}
}

func TestValidateAcceptsValidVideo(t *testing.T) {
	srcPath := tempInput(t, 1024)
	spec, err := mustValidate(t, validVideoProbe(), srcPath, 10<<20)
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if spec.Width != 1280 || spec.Height != 720 {
		t.Errorf("dims = %dx%d, want 1280x720", spec.Width, spec.Height)
	}
	if spec.DurationSeconds != 10 {
		t.Errorf("duration = %v, want 10", spec.DurationSeconds)
	}
	if spec.VideoCodec != "h264" || !spec.HasAudio || spec.AudioCodec != "aac" {
		t.Errorf("codecs = video %q audio %q hasAudio %v", spec.VideoCodec, spec.AudioCodec, spec.HasAudio)
	}
	if spec.FrameRate < 29.9 || spec.FrameRate > 30.1 {
		t.Errorf("frame rate = %v, want ~29.97", spec.FrameRate)
	}
}

func TestValidateRejectsMultipleVideoStreams(t *testing.T) {
	probe := probeJSON([]fixtureStream{
		videoStream("h264", 1280, 720, "10", "30"),
		videoStream("h264", 640, 480, "10", "30"),
	}, "10")
	_, err := mustValidate(t, probe, tempInput(t, 16), 0)
	assertCode(t, err, ErrorMultiStream)
}

func TestValidateRejectsMultipleAudioStreams(t *testing.T) {
	probe := probeJSON([]fixtureStream{
		videoStream("h264", 1280, 720, "10", "30"),
		audioStream("aac"),
		audioStream("aac"),
	}, "10")
	_, err := mustValidate(t, probe, tempInput(t, 16), 0)
	assertCode(t, err, ErrorMultiStream)
}

func TestValidateRejectsSubtitleStream(t *testing.T) {
	probe := probeJSON([]fixtureStream{
		videoStream("h264", 1280, 720, "10", "30"),
		{CodecType: "subtitle", CodecName: "mov_text"},
	}, "10")
	_, err := mustValidate(t, probe, tempInput(t, 16), 0)
	assertCode(t, err, ErrorInvalidVideo)
}

func TestValidateRejectsDataStream(t *testing.T) {
	probe := probeJSON([]fixtureStream{
		videoStream("h264", 1280, 720, "10", "30"),
		{CodecType: "data", CodecName: "bin_data"},
	}, "10")
	_, err := mustValidate(t, probe, tempInput(t, 16), 0)
	assertCode(t, err, ErrorInvalidVideo)
}

func TestValidateRejectsAttachmentStream(t *testing.T) {
	probe := probeJSON([]fixtureStream{
		videoStream("h264", 1280, 720, "10", "30"),
		{CodecType: "attachment", CodecName: "mjpeg"},
	}, "10")
	_, err := mustValidate(t, probe, tempInput(t, 16), 0)
	assertCode(t, err, ErrorInvalidVideo)
}

func TestValidateRejectsNoVideoStream(t *testing.T) {
	probe := probeJSON([]fixtureStream{audioStream("aac")}, "10")
	_, err := mustValidate(t, probe, tempInput(t, 16), 0)
	assertCode(t, err, ErrorInvalidVideo)
}

func TestValidateRejectsTooLong(t *testing.T) {
	probe := probeJSON([]fixtureStream{videoStream("h264", 1280, 720, "31", "30")}, "31")
	_, err := mustValidate(t, probe, tempInput(t, 16), 0)
	assertCode(t, err, ErrorTooLong)
}

func TestValidateRejectsExactlyAtLimit(t *testing.T) {
	// Boundary: 30s, 1280x720, 30fps are all accepted.
	probe := probeJSON([]fixtureStream{videoStream("h264", 1280, 720, "30", "30")}, "30")
	spec, err := mustValidate(t, probe, tempInput(t, 16), 0)
	if err != nil {
		t.Fatalf("boundary input rejected: %v", err)
	}
	if spec.DurationSeconds != 30 {
		t.Errorf("duration = %v, want 30", spec.DurationSeconds)
	}
}

func TestValidateRejectsMalformedDuration(t *testing.T) {
	probe := probeJSON([]fixtureStream{videoStream("h264", 1280, 720, "garbage", "30")}, "N/A")
	_, err := mustValidate(t, probe, tempInput(t, 16), 0)
	assertCode(t, err, ErrorInvalidVideo)
}

func TestValidateRejectsMissingDuration(t *testing.T) {
	probe := probeJSON([]fixtureStream{videoStream("h264", 1280, 720, "", "")}, "")
	_, err := mustValidate(t, probe, tempInput(t, 16), 0)
	assertCode(t, err, ErrorInvalidVideo)
}

func TestValidateRejectsExcessiveDimensions(t *testing.T) {
	probe := probeJSON([]fixtureStream{videoStream("h264", 1920, 1080, "10", "30")}, "10")
	_, err := mustValidate(t, probe, tempInput(t, 16), 0)
	assertCode(t, err, ErrorTooLargeDims)
}

func TestValidateRejectsTooHighFPS(t *testing.T) {
	probe := probeJSON([]fixtureStream{videoStream("h264", 1280, 720, "10", "60")}, "10")
	_, err := mustValidate(t, probe, tempInput(t, 16), 0)
	assertCode(t, err, ErrorTooHighFPS)
}

func TestValidateRejectsUnsupportedVideoCodecs(t *testing.T) {
	for _, codec := range []string{"av1", "hevc"} {
		probe := probeJSON([]fixtureStream{videoStream(codec, 1280, 720, "10", "30")}, "10")
		_, err := mustValidate(t, probe, tempInput(t, 16), 0)
		assertCode(t, err, ErrorUnsupportedCodec)
	}
}

// TestValidateAcceptsBrowserWebMVideoCodecs proves the product's own browser
// recordings (WebM/VP8 or VP9, per the frontend MediaRecorder preference) are
// accepted inputs and are transcoded to canonical H.264 downstream.
func TestValidateAcceptsBrowserWebMVideoCodecs(t *testing.T) {
	for _, codec := range []string{"vp8", "vp9"} {
		probe := probeJSON([]fixtureStream{videoStream(codec, 1280, 720, "10", "30")}, "10")
		spec, err := mustValidate(t, probe, tempInput(t, 16), 0)
		if err != nil {
			t.Fatalf("webm video codec %q rejected: %v", codec, err)
		}
		if spec.VideoCodec != codec {
			t.Errorf("video codec = %q, want %q", spec.VideoCodec, codec)
		}
	}
}

func TestValidateAcceptsMp3AndOpusAudio(t *testing.T) {
	for _, codec := range []string{"mp3", "opus"} {
		probe := probeJSON([]fixtureStream{
			videoStream("h264", 1280, 720, "10", "30"),
			audioStream(codec),
		}, "10")
		spec, err := mustValidate(t, probe, tempInput(t, 16), 0)
		if err != nil {
			t.Fatalf("audio codec %q rejected: %v", codec, err)
		}
		if spec.AudioCodec != codec {
			t.Errorf("audio codec = %q, want %q", spec.AudioCodec, codec)
		}
	}
}

func TestValidateRejectsUnsupportedAudioCodec(t *testing.T) {
	probe := probeJSON([]fixtureStream{
		videoStream("h264", 1280, 720, "10", "30"),
		audioStream("ac3"),
	}, "10")
	_, err := mustValidate(t, probe, tempInput(t, 16), 0)
	assertCode(t, err, ErrorUnsupportedCodec)
}

func TestValidateRejectsOversizeFile(t *testing.T) {
	probe := validVideoProbe()
	runner := &fakeRunner{stdout: []byte(probe), exitCode: 0}
	_, err := Validate(context.Background(), tempInput(t, 2048), 1024, runner)
	assertCode(t, err, ErrorTooLarge)
}

func TestValidateSizeBoundaryAccepted(t *testing.T) {
	probe := validVideoProbe()
	runner := &fakeRunner{stdout: []byte(probe), exitCode: 0}
	if _, err := Validate(context.Background(), tempInput(t, 1024), 1024, runner); err != nil {
		t.Fatalf("file exactly at the cap rejected: %v", err)
	}
}

func TestValidateRejectsNonzeroExit(t *testing.T) {
	// Polyglot/malformed input: ffprobe exits non-zero.
	runner := &fakeRunner{stdout: []byte(""), exitCode: 1}
	_, err := Validate(context.Background(), tempInput(t, 16), 0, runner)
	assertCode(t, err, ErrorInvalidVideo)
}

func TestValidateRejectsGarbageJSON(t *testing.T) {
	runner := &fakeRunner{stdout: []byte("this is not json {"), exitCode: 0}
	_, err := Validate(context.Background(), tempInput(t, 16), 0, runner)
	assertCode(t, err, ErrorInvalidVideo)
}

func TestValidateProbesWithExpectedArgs(t *testing.T) {
	srcPath := tempInput(t, 16)
	runner := &fakeRunner{stdout: []byte(validVideoProbe()), exitCode: 0}
	if _, err := Validate(context.Background(), srcPath, 0, runner); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if runner.name != "ffprobe" {
		t.Errorf("runner name = %q, want ffprobe", runner.name)
	}
	want := fmt.Sprintf("-print_format json -show_streams -show_format %s", srcPath)
	if got := fmt.Sprintf("%v", runner.args); !containsAll(runner.args, "-v", "error", "-print_format", "json", "-show_streams", "-show_format", srcPath) {
		t.Errorf("ffprobe args = %s, want to contain %s", got, want)
	}
}

func containsAll(haystack []string, needles ...string) bool {
	set := make(map[string]int, len(haystack))
	for _, h := range haystack {
		set[h]++
	}
	for _, n := range needles {
		if set[n] == 0 {
			return false
		}
	}
	return true
}
