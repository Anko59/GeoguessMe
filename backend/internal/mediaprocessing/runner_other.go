//go:build !linux

package mediaprocessing

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Run executes name with args under ctx directly. Platforms without the
// Linux rlimit machinery apply no per-process ceilings here; the worker
// container's cpus/mem/pids deployment bounds still apply, and the per-job
// wall-clock bound is the caller's context deadline.
func (OSCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
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
