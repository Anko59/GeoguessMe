//go:build linux

package mediaprocessing

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestMain routes re-exec'd copies of the test binary into the rlimit
// trampoline: when the sentinel env var is present the process applies the
// package rlimits and execs the requested tool instead of running tests. The
// trampoline re-execs os.Args[0] (this test binary in tests, the worker
// binary in production), so without this hook the re-exec'd copy would try to
// run tests again with tool arguments.
func TestMain(m *testing.M) {
	if HandleRlimitHelperInvocation() {
		os.Exit(0) // unreachable in practice
	}
	os.Exit(m.Run())
}

// TestRlimitHelperAppliesChildLimits exercises the full trampoline: the real
// runner re-execs this test binary with the sentinel env var, the copy routes
// through TestMain/HandleRlimitHelperInvocation, applies the rlimits, and
// execs /bin/sh (a small C binary that starts fine under the 512 MiB
// address-space ceiling; a Go binary would not). The shell then prints the
// inherited limits, proving each per-process ceiling bound the child.
func TestRlimitHelperAppliesChildLimits(t *testing.T) {
	runner := OSCommandRunner{}
	stdout, exitCode, err := runner.Run(context.Background(), "/bin/sh", "-c", "ulimit -t; ulimit -v; ulimit -u")
	if err != nil || exitCode != 0 {
		t.Fatalf("trampoline sh failed: exit=%d err=%v stdout=%s", exitCode, err, stdout)
	}
	fields := strings.Fields(string(stdout))
	if len(fields) != 3 {
		t.Fatalf("unexpected ulimit output %q, want three limit values", stdout)
	}
	// ulimit -v prints kilobytes: 512 MiB == 524288 KiB.
	if fields[0] != "60" || fields[1] != "524288" || fields[2] != "128" {
		t.Errorf("child limits = %v, want [60 524288 128] (CPU seconds, KiB, processes)", fields)
	}
}

// TestRlimitHelperMissingTool proves a failed exec inside the trampoline
// surfaces as a 127 exit without hanging the parent.
func TestRlimitHelperMissingTool(t *testing.T) {
	runner := OSCommandRunner{}
	_, exitCode, err := runner.Run(context.Background(), "geoguessme-test-no-such-tool")
	if err == nil {
		t.Fatal("expected an error for a missing tool")
	}
	if exitCode != 127 {
		t.Errorf("exit code = %d, want 127 (trampoline exec failure)", exitCode)
	}
}
