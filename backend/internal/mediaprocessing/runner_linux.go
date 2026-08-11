//go:build linux

package mediaprocessing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// helperArg marks a re-exec'd process that must apply the package rlimits and
// then exec the real tool. OSCommandRunner.Run re-execs its own binary with
// this private argument because Go's syscall.SysProcAttr no longer carries a Setrlimit
// field: the re-exec'd copy is a fresh process, so the limits it applies with
// syscall.Setrlimit bind every child it execs (ffprobe/ffmpeg) from the
// start. The worker's main() MUST call HandleRlimitHelperInvocation before
// any other logic so re-exec'd copies route into the trampoline.
const helperArg = "--geoguessme-media-rlimit-helper"

// Resource ceilings applied to every ffprobe/ffmpeg child on Linux. They
// mirror the worker container's deployment bounds (cpus: 1, mem: 512m,
// pids: 128) as per-process backstops. The primary per-job wall-clock bound
// is the context deadline the caller passes to Validate/Transcode.
const (
	// maxCPUSeconds is the hard RLIMIT_CPU backstop in CPU seconds. It guards
	// against a child that survives or ignores its context deadline (for
	// example a codec blocked in kernel space). 60s matches the documented
	// per-job bound; the container's cpus=1 limit keeps wall time and CPU time
	// of the same order.
	maxCPUSeconds = 60
	// maxAddrSpaceBytes bounds the child's address space to 512 MiB, defeating
	// decompression-bomb and memory-exhaustion inputs. The container's
	// mem=512m limit remains the authoritative RSS bound.
	maxAddrSpaceBytes = 512 << 20
	// maxChildPIDs bounds the number of processes the (non-root) worker UID
	// may run, capping subprocess storms.
	maxChildPIDs = 128
)

// rlimitNPROC is Linux's RLIMIT_NPROC (0x6). The syscall package exposes only
// a subset of RLIMIT_* constants; the process ceiling is defined here.
const rlimitNPROC = 0x6

// applyChildRlimits applies the package resource ceilings to the current
// process (the re-exec'd trampoline, which immediately execs the real tool).
func applyChildRlimits() error {
	limits := []struct {
		resource int
		value    uint64
		name     string
	}{
		{syscall.RLIMIT_CPU, maxCPUSeconds, "cpu"},
		{syscall.RLIMIT_AS, maxAddrSpaceBytes, "address-space"},
		{rlimitNPROC, maxChildPIDs, "process-count"},
	}
	for _, limit := range limits {
		if err := syscall.Setrlimit(limit.resource, &syscall.Rlimit{Cur: limit.value, Max: limit.value}); err != nil {
			return fmt.Errorf("set %s rlimit: %w", limit.name, err)
		}
	}
	return nil
}

// HandleRlimitHelperInvocation routes a re-exec'd process into the rlimit
// trampoline. The worker's main() must call it as its first statement: when
// the sentinel argument is absent it returns false immediately and normal
// startup proceeds; when present it applies the package rlimits and execs the
// real tool, never returning. Unit tests route through TestMain instead.
func HandleRlimitHelperInvocation() bool {
	if len(os.Args) < 2 || os.Args[1] != helperArg {
		return false
	}
	args := os.Args[2:]
	if len(args) == 0 {
		os.Exit(1)
	}
	if err := applyChildRlimits(); err != nil {
		fmt.Fprintf(os.Stderr, "mediaprocessing: %v\n", err)
		os.Exit(126)
	}
	name := args[0]
	if err := syscall.Exec(name, append([]string{name}, args[1:]...), os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "mediaprocessing: exec %s: %v\n", name, err)
		os.Exit(127)
	}
	return true // unreachable: Exec either succeeds or exits.
}

// Run executes name with args under ctx via the rlimit trampoline. The
// re-exec'd copy of this binary applies the package rlimits and execs the
// tool, so the child inherits the CPU-time, address-space, and process
// ceilings from its first instruction.
func (OSCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, int, error) {
	// execve (used by the trampoline) does not search PATH, so resolve the
	// tool to an absolute path up front; a bare name would fail with ENOENT
	// inside syscall.Exec and abort every ffprobe/ffmpeg invocation. A failed
	// lookup leaves the bare name in place so the trampoline still surfaces a
	// 127 exit (TestRlimitHelperMissingTool).
	if !strings.Contains(name, "/") {
		if resolved, err := exec.LookPath(name); err == nil {
			name = resolved
		}
	}
	self, err := os.Executable()
	if err != nil {
		return nil, 0, fmt.Errorf("mediaprocessing: resolve self: %w", err)
	}
	cmdArgs := append([]string{helperArg, name}, args...)
	cmd := exec.CommandContext(ctx, self, cmdArgs...)
	var stdout, stderr limitedCapture
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			err = fmt.Errorf("%w; stderr: %s", err, msg)
		}
	}
	return append([]byte(nil), stdout.Bytes()...), exitCode, err
}
