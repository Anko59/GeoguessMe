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

// helperEnv marks a re-exec'd process that must apply the package rlimits and
// then exec the real tool. OSCommandRunner.Run re-execs its own binary with
// this marker because Go's syscall.SysProcAttr no longer carries a Setrlimit
// field: the re-exec'd copy is a fresh process, so the limits it applies with
// syscall.Setrlimit bind every child it execs (ffprobe/ffmpeg) from the
// start. The worker's main() MUST call HandleRlimitHelperInvocation before
// any other logic so re-exec'd copies route into the trampoline.
const helperEnv = "GEOSSMEDIAPROCESSING_RLIMIT_HELPER"

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
// Errors are ignored on purpose: the rlimits are a defense-in-depth backstop
// on top of the worker container's cpus/mem/pids deployment bounds, and
// lowering hard limits is permitted for non-root processes.
func applyChildRlimits() {
	_ = syscall.Setrlimit(syscall.RLIMIT_CPU, &syscall.Rlimit{Cur: maxCPUSeconds, Max: maxCPUSeconds})
	_ = syscall.Setrlimit(syscall.RLIMIT_AS, &syscall.Rlimit{Cur: maxAddrSpaceBytes, Max: maxAddrSpaceBytes})
	_ = syscall.Setrlimit(rlimitNPROC, &syscall.Rlimit{Cur: maxChildPIDs, Max: maxChildPIDs})
}

// HandleRlimitHelperInvocation routes a re-exec'd process into the rlimit
// trampoline. The worker's main() must call it as its first statement: when
// the sentinel env var is absent it returns false immediately and normal
// startup proceeds; when present it applies the package rlimits and execs the
// real tool, never returning. Unit tests route through TestMain instead.
func HandleRlimitHelperInvocation() bool {
	if os.Getenv(helperEnv) != "1" {
		return false
	}
	args := os.Args[1:]
	if len(args) == 0 {
		os.Exit(1)
	}
	applyChildRlimits()
	name := args[0]
	env := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, helperEnv+"=") {
			env = append(env, kv)
		}
	}
	if err := syscall.Exec(name, append([]string{name}, args[1:]...), env); err != nil {
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
	self, err := os.Executable()
	if err != nil {
		return nil, 0, fmt.Errorf("mediaprocessing: resolve self: %w", err)
	}
	cmdArgs := append([]string{name}, args...)
	cmd := exec.CommandContext(ctx, self, cmdArgs...)
	cmd.Env = append(os.Environ(), helperEnv+"=1")
	var stdout, stderr strings.Builder
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
	return []byte(stdout.String()), exitCode, err
}
