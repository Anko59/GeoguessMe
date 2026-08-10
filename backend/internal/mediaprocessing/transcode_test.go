package mediaprocessing

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestTranscodeArgVectorWithAudio(t *testing.T) {
	args := transcodeArgs("/src/video.mp4", "/dst/out.mp4", true)
	want := []string{
		"-nostdin",
		"-y",
		"-i", "/src/video.mp4",
		"-map", "0:v:0",
		"-map", "0:a:0",
		"-c:v", "libx264",
		"-pix_fmt", "yuv420p",
		"-preset", "fast",
		"-movflags", "+fast-start",
		"-metadata", "title=",
		"-map_metadata", "-1",
		"-map_chapters", "-1",
		"-sn",
		"-dn",
		"-fflags", "+bitexact",
		"-flags:v", "+bitexact",
		"-flags:a", "+bitexact",
		"-c:a", "aac", "-b:a", "128k",
		"/dst/out.mp4",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %v\nwant = %v", args, want)
	}
}

func TestTranscodeArgVectorWithoutAudio(t *testing.T) {
	args := transcodeArgs("/src/video.mp4", "/dst/out.mp4", false)
	for _, seq := range [][]string{{"-map", "0:a:0"}, {"-c:a", "aac"}, {"-b:a", "128k"}} {
		if hasSequence(args, seq...) {
			t.Fatalf("audio arg sequence %v present without audio stream: %v", seq, args)
		}
	}
	for _, required := range []string{"-c:v", "libx264", "yuv420p", "+fast-start", "-sn", "-dn", "-map_chapters", "-1", "-map_metadata", "-1", "/dst/out.mp4"} {
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
	if err := Transcode(context.Background(), "/src/in", "/dst/out.mp4", true, runner); err != nil {
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
	err := Transcode(context.Background(), "/src/in", "/dst/out.mp4", false, runner)
	assertCode(t, err, ErrorTranscodeFailed)
}

func TestTranscodeTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	// Let the deadline fire deterministically.
	time.Sleep(time.Millisecond)
	runner := &fakeRunner{exitCode: 0, err: context.DeadlineExceeded}
	err := Transcode(ctx, "/src/in", "/dst/out.mp4", false, runner)
	assertCode(t, err, ErrorTimeout)
}

func TestTranscodeTimeoutViaCtx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := &fakeRunner{exitCode: 0, err: context.Canceled}
	err := Transcode(ctx, "/src/in", "/dst/out.mp4", false, runner)
	// Canceled (not DeadlineExceeded) is treated as an infra failure, not a
	// validation timeout: the worker restarts the job.
	assertCode(t, err, ErrorTranscodeFailed)
}

func TestTranscodeNilRunnerUsesDefault(t *testing.T) {
	err := Transcode(context.Background(), "/src/in", "/dst/out.mp4", false, nil)
	if err == nil {
		t.Fatal("expected an error from the default runner against a missing ffmpeg")
	}
	assertCode(t, err, ErrorTranscodeFailed)
}
