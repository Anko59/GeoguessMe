package mediaprocessing

import (
	"context"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTranscodeArgVectorWithAudio(t *testing.T) {
	args := transcodeArgs("/src/video.mp4", "/dst/out.mp4", true, 2048)
	want := []string{
		"-nostdin",
		"-y",
		"-i", "/src/video.mp4",
		"-map", "0:v:0",
		"-map", "0:a:0",
		"-c:v", "libx264",
		"-pix_fmt", "yuv420p",
		"-preset", "fast",
		"-movflags", "+faststart",
		"-metadata", "title=",
		"-map_metadata", "-1",
		"-map_chapters", "-1",
		"-sn",
		"-dn",
		"-fflags", "+bitexact",
		"-flags:v", "+bitexact",
		"-flags:a", "+bitexact",
		"-c:a", "aac", "-b:a", "128k",
		"-fs", "2048",
		"/dst/out.mp4",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %v\nwant = %v", args, want)
	}
}

func TestTranscodeArgVectorWithoutAudio(t *testing.T) {
	args := transcodeArgs("/src/video.mp4", "/dst/out.mp4", false, 2048)
	for _, seq := range [][]string{{"-map", "0:a:0"}, {"-c:a", "aac"}, {"-b:a", "128k"}} {
		if hasSequence(args, seq...) {
			t.Fatalf("audio arg sequence %v present without audio stream: %v", seq, args)
		}
	}
	for _, required := range []string{"-c:v", "libx264", "yuv420p", "+faststart", "-sn", "-dn", "-map_chapters", "-1", "-map_metadata", "-1", "/dst/out.mp4"} {
		found := false
		for _, a := range args {
			if a == required {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("required arg %q missing: %v", required, args)
		}
	}
}

// hasSequence reports whether seq appears contiguously in haystack.
func hasSequence(haystack []string, seq ...string) bool {
	for i := 0; i+len(seq) <= len(haystack); i++ {
		match := true
		for j := range seq {
			if haystack[i+j] != seq[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func TestTranscodeSuccess(t *testing.T) {
	runner := &fakeRunner{exitCode: 0}
	if err := Transcode(context.Background(), "/src/in", "/dst/out.mp4", true, 2048, runner); err != nil {
		t.Fatalf("Transcode returned error: %v", err)
	}
	if runner.name != "ffmpeg" {
		t.Errorf("runner name = %q, want ffmpeg", runner.name)
	}
	if runner.args[0] != "-nostdin" {
		t.Errorf("first arg = %q, want -nostdin", runner.args[0])
	}
}

func TestTranscodeNonZeroExit(t *testing.T) {
	runner := &fakeRunner{exitCode: 1}
	err := Transcode(context.Background(), "/src/in", "/dst/out.mp4", false, 2048, runner)
	assertCode(t, err, ErrorTranscodeFailed)
}

func TestTranscodeTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	// Let the deadline fire deterministically.
	time.Sleep(time.Millisecond)
	runner := &fakeRunner{exitCode: 0, err: context.DeadlineExceeded}
	err := Transcode(ctx, "/src/in", "/dst/out.mp4", false, 2048, runner)
	assertCode(t, err, ErrorTimeout)
}

func TestTranscodeTimeoutViaCtx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := &fakeRunner{exitCode: 0, err: context.Canceled}
	err := Transcode(ctx, "/src/in", "/dst/out.mp4", false, 2048, runner)
	// Canceled (not DeadlineExceeded) is treated as an infra failure, not a
	// validation timeout: the worker restarts the job.
	assertCode(t, err, ErrorTranscodeFailed)
}

func TestTranscodeNilRunnerUsesDefault(t *testing.T) {
	err := Transcode(context.Background(), "/src/in", "/dst/out.mp4", false, 2048, nil)
	if err == nil {
		t.Fatal("expected an error from the default runner against a missing ffmpeg")
	}
	assertCode(t, err, ErrorTranscodeFailed)
}

// TestTranscodeWithRealFFmpeg runs the exact canonical transcode command
// against the real ffmpeg when one is installed. It is the regression guard
// for ffmpeg flag-name compatibility: the pinned alpine ffmpeg 8.1.2-r0 build
// rejects "-movflags +fast-start" (and "fast_start") with a movflags parse
// error, which previously made every transcode fail and the video E2E test
// time out. The test skips when ffmpeg/ffprobe are unavailable or cannot
// produce a VP8 fixture, but fails whenever a valid fixture cannot be
// transcoded to the canonical output.
func TestTranscodeWithRealFFmpeg(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src.webm")
	dst := filepath.Join(dir, "out.mp4")

	// Generate a tiny VP8 WebM with audio. Skip if this ffmpeg cannot encode
	// VP8 (environment-specific), but fail the transcode if a valid fixture
	// cannot be converted to the canonical output.
	gen := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=160x90:rate=15",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-t", "1", "-c:v", "libvpx", "-b:v", "50k", "-c:a", "libvorbis", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Skipf("cannot generate VP8 fixture with this ffmpeg: %v: %s", err, out)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := Transcode(ctx, src, dst, true, 2<<20, nil); err != nil {
		t.Fatalf("Transcode with real ffmpeg failed: %v", err)
	}

	// Verify canonical output: H.264 yuv420p video + AAC audio.
	probeV := exec.Command("ffprobe", "-hide_banner", "-loglevel", "error",
		"-select_streams", "v:0", "-show_entries", "stream=codec_name,pix_fmt", "-of", "csv=p=0", dst)
	vout, err := probeV.Output()
	if err != nil {
		t.Fatalf("ffprobe video failed: %v", err)
	}
	if got := strings.TrimSpace(string(vout)); got != "h264,yuv420p" {
		t.Fatalf("video codec/pix_fmt = %q, want h264,yuv420p", got)
	}
	probeA := exec.Command("ffprobe", "-hide_banner", "-loglevel", "error",
		"-select_streams", "a:0", "-show_entries", "stream=codec_name", "-of", "csv=p=0", dst)
	aout, err := probeA.Output()
	if err != nil {
		t.Fatalf("ffprobe audio failed: %v", err)
	}
	if got := strings.TrimSpace(string(aout)); got != "aac" {
		t.Fatalf("audio codec = %q, want aac", got)
	}
}
