package deploy

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

func newTestLocalRunner(t *testing.T) *NodeDeployLocalRunner {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the local runner needs a POSIX shell")
	}
	runner, err := NewNodeDeployLocalRunner()
	if err != nil {
		t.Fatalf("new local runner: %v", err)
	}
	return runner
}

func TestNewNodeDeployLocalRunnerRejectsWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only Windows refuses a local runner")
	}
	if _, err := NewNodeDeployLocalRunner(); err == nil {
		t.Error("expected a Windows host to refuse to deploy a node onto itself")
	}
}

func TestLocalRunnerRunContext(t *testing.T) {
	runner := newTestLocalRunner(t)
	defer runner.Close()

	out, err := runner.RunContext(context.Background(), "printf 'hello'")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "hello" {
		t.Errorf("output = %q, want %q", out, "hello")
	}
}

// A failing command must surface what the shell printed, the way the SSH runner
// does, so a deployment failure is diagnosable from the task log alone.
func TestLocalRunnerRunContextReportsOutputOnFailure(t *testing.T) {
	runner := newTestLocalRunner(t)
	defer runner.Close()

	_, err := runner.RunContext(context.Background(), "echo 'boom' >&2; exit 3")
	if err == nil {
		t.Fatal("expected a non-zero exit to be an error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error = %v, want it to carry the command output", err)
	}
}

func TestLocalRunnerRunContextHonoursCanceledContext(t *testing.T) {
	runner := newTestLocalRunner(t)
	defer runner.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runner.RunContext(ctx, "printf 'hello'"); err == nil {
		t.Error("expected a canceled context to stop the command")
	}
}

// The local runner writes as root, so it must refuse anything outside the set
// of files a node deployment owns, exactly as the SSH runner does.
func TestLocalRunnerWriteFileRejectsUnknownTargets(t *testing.T) {
	runner := newTestLocalRunner(t)
	defer runner.Close()

	cases := map[string]struct{ path, mode string }{
		"path outside the deployment": {"/etc/passwd", "0644"},
		"traversal":                   {"/etc/kubernetes/../../etc/passwd", "0644"},
		"mode without a leading zero": {"/etc/kubernetes/ca.crt", "644"},
		"mode that is not octal":      {"/etc/kubernetes/ca.crt", "0abc"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if err := runner.WriteFileContext(context.Background(), tc.path, "x", tc.mode); err == nil {
				t.Errorf("WriteFileContext(%q, %q) accepted a target it must reject", tc.path, tc.mode)
			}
		})
	}
}

func TestSanitizeMachineName(t *testing.T) {
	cases := []struct{ value, fallback, want string }{
		{"MyLaptop", "localhost", "mylaptop"},
		{"web-01.example.com", "localhost", "web-01-example-com"},
		{"  Ubuntu_22.04  ", "default", "ubuntu-22-04"},
		{"___", "localhost", "localhost"},
		{"", "localhost", "localhost"},
	}
	for _, tc := range cases {
		if got := sanitizeMachineName(tc.value, tc.fallback); got != tc.want {
			t.Errorf("sanitizeMachineName(%q) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestLocalHostMachineNameFitsTheColumn(t *testing.T) {
	name := localHostMachineName(strings.Repeat("host-", 40))
	if len(name) > 100 {
		t.Errorf("name is %d characters, want at most 100", len(name))
	}
	if strings.HasSuffix(name, "-") {
		t.Errorf("name = %q, want no trailing dash after truncation", name)
	}
}
