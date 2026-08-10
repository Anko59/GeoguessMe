// Package mediaprocessing validates quarantined video uploads with ffprobe
// and transcodes them into canonical MP4/H.264 objects with ffmpeg.
//
// The package is deliberately pure over subprocess execution: it operates on
// local file paths and never touches storage or the database. The worker loop
// (a later layer) downloads the quarantine object to a temporary file, calls
// Validate and Transcode, then promotes the canonical object and records the
// result. All ffprobe/ffmpeg invocation goes through the CommandRunner seam so
// unit tests can substitute a fake runner and exercise every validation and
// transcode branch without a real ffmpeg installation.
//
// Every rejection maps to a stable, owner-facing error code (see the Error*
// constants in validate.go). These codes are surfaced verbatim by the media
// processing status endpoint, so they must never change and must never embed
// file content, paths, or command output.
//
// Resource bounds: the caller supplies the per-job context deadline (60
// seconds per the F-10 acceptance criteria). On Linux each child additionally
// runs under hard per-process rlimits — a CPU-time backstop, a 512 MiB
// address space ceiling, and a 128-process ceiling — matching the worker
// container's cpus=1 / mem=512m / pids=128 deployment bounds. Because Go's
// syscall.SysProcAttr no longer carries a Setrlimit field, the runner
// re-execs its own binary as a trampoline that applies the limits before
// exec'ing the tool; the worker's main() must call
// HandleRlimitHelperInvocation as its first statement (see runner_linux.go).
package mediaprocessing

import (
	"context"
)

// CommandRunner executes an external tool and returns its captured stdout and
// exit status. It is the single seam through which ffprobe and ffmpeg are
// invoked, so tests can inject a fake runner.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) (stdout []byte, exitCode int, err error)
}

// OSCommandRunner runs commands on the host using os/exec. The platform
// runner files (runner_linux.go, runner_other.go) implement Run; on Linux
// each child runs under the package rlimits via a self-exec trampoline.
type OSCommandRunner struct{}
