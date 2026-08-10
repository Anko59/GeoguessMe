package mediaprocessing

import (
	"context"
	"errors"
	"testing"
)

// fakeRunner is a scriptable CommandRunner for deterministic unit tests. It
// returns the canned stdout/exit code/error and records the invocation.
type fakeRunner struct {
	stdout   []byte
	exitCode int
	err      error
	name     string
	args     []string
	calls    int
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, int, error) {
	f.calls++
	f.name = name
	f.args = append([]string(nil), args...)
	return f.stdout, f.exitCode, f.err
}

func TestErrorCode(t *testing.T) {
	if got := ErrorCode(validationError(ErrorTooLong, "duration 60s")); got != ErrorTooLong {
		t.Fatalf("ErrorCode(validationError) = %q, want %q", got, ErrorTooLong)
	}
	if got := ErrorCode(errors.New("boom")); got != "" {
		t.Fatalf("ErrorCode(generic error) = %q, want empty", got)
	}
	if got := ErrorCode(nil); got != "" {
		t.Fatalf("ErrorCode(nil) = %q, want empty", got)
	}
}

func TestValidationErrorString(t *testing.T) {
	ve := &ValidationError{Code: ErrorInvalidVideo, Reason: "no video stream"}
	if got := ve.Error(); got != "invalid_video: no video stream" {
		t.Fatalf("Error() = %q", got)
	}
	if got := (&ValidationError{Code: ErrorTooLong}).Error(); got != ErrorTooLong {
		t.Fatalf("Error() without reason = %q, want %q", got, ErrorTooLong)
	}
}

// TestOSCommandRunnerMissingBinary proves the real runner surfaces an error
// for an unstartable command without needing any real tool installed.
func TestOSCommandRunnerMissingBinary(t *testing.T) {
	runner := OSCommandRunner{}
	stdout, exitCode, err := runner.Run(context.Background(), "geoguessme-test-nonexistent-tool")
	if err == nil {
		t.Fatalf("expected an error for a missing binary, got stdout=%q exit=%d", stdout, exitCode)
	}
}

func TestValidateNilRunnerUsesDefault(t *testing.T) {
	// A nil runner must not panic; the default OSCommandRunner fails to find
	// ffprobe in a unit-test environment, which maps to invalid_video.
	_, err := Validate(context.Background(), "/nonexistent", 0, nil)
	if err == nil {
		t.Fatal("expected an error from the default runner against a missing ffprobe")
	}
	if got := ErrorCode(err); got != ErrorInvalidVideo {
		t.Fatalf("ErrorCode = %q, want %q", got, ErrorInvalidVideo)
	}
}
