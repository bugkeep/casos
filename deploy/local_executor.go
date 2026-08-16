package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// NodeDeployLocalRunner runs the deployment commands on the CasOS host itself,
// so the host can become a worker node of its own cluster. It goes straight to
// the shell instead of looping SSH back to localhost, which means the host
// needs neither an sshd nor a credential for CasOS to deploy onto it.
type NodeDeployLocalRunner struct {
	commandTimeout time.Duration
}

// NewNodeDeployLocalRunner returns a runner for the CasOS host. Windows hosts
// have no POSIX shell and cannot run a kubelet, and reach a node through their
// WSL distribution instead.
func NewNodeDeployLocalRunner() (*NodeDeployLocalRunner, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("a Windows host cannot run a worker node itself, its WSL distribution is deployed instead")
	}
	return &NodeDeployLocalRunner{commandTimeout: defaultCommandTimeout}, nil
}

func (r *NodeDeployLocalRunner) RunContext(ctx context.Context, command string) (string, error) {
	return r.exec(ctx, command, false, nil)
}

func (r *NodeDeployLocalRunner) RunRootContext(ctx context.Context, command string) (string, error) {
	return r.exec(ctx, command, true, nil)
}

func (r *NodeDeployLocalRunner) WriteFileContext(ctx context.Context, path, content, mode string) error {
	if err := validateNodeDeployFile(path, mode); err != nil {
		return err
	}
	command := fmt.Sprintf("install -D -m %s /dev/stdin %s", shellSingleQuote(mode), shellSingleQuote(path))
	_, err := r.exec(ctx, command, true, strings.NewReader(content))
	return err
}

// AppendAuthorizedKeyContext has nothing to do on the CasOS host: the
// deployment never logs in, so there is no login to authorize. Callers skip it
// for a local machine rather than relying on it to do nothing.
func (r *NodeDeployLocalRunner) AppendAuthorizedKeyContext(context.Context, string) error {
	return fmt.Errorf("the CasOS host is deployed without SSH, so it has no login to authorize")
}

func (r *NodeDeployLocalRunner) Close() error {
	return nil
}

// exec runs command through the shell, escalating with sudo when CasOS itself
// is not root. Callers must shell-escape user-controlled values, exactly as
// they must for the SSH runner.
func (r *NodeDeployLocalRunner) exec(ctx context.Context, command string, asRoot bool, stdin io.Reader) (string, error) {
	if err := contextErr(ctx); err != nil {
		return "", err
	}
	timeout := r.effectiveCommandTimeout()
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	name, args := "sh", []string{"-c", command}
	if asRoot && os.Geteuid() != 0 {
		// -n so a host without passwordless sudo fails immediately instead of
		// blocking on a password prompt nobody can answer.
		name, args = "sudo", []string{"-n", "sh", "-c", command}
	}
	cmd := exec.CommandContext(runCtx, name, args...)
	cmd.Stdin = stdin
	output, err := cmd.CombinedOutput()

	text := strings.TrimSpace(string(output))
	if err != nil {
		if parentErr := contextErr(ctx); parentErr != nil {
			return text, parentErr
		}
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return text, fmt.Errorf("local command timed out after %s", timeout)
		}
		if text == "" {
			return "", err
		}
		return text, fmt.Errorf("%w: %s", err, text)
	}
	return text, nil
}

func (r *NodeDeployLocalRunner) effectiveCommandTimeout() time.Duration {
	if r.commandTimeout <= 0 {
		return defaultCommandTimeout
	}
	return r.commandTimeout
}
