package deploy

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/beego/beego/logs"
	"github.com/casosorg/casos/conf"
	"github.com/casosorg/casos/object"
	"github.com/casosorg/casos/wsl"
)

// Zero configuration worker node.
//
// A cluster with no node cannot run anything, so CasOS never leaves itself
// without one: at startup it turns the machine it is already running on into a
// worker node. On Linux that is the host itself, reached through a local shell
// that needs neither sshd nor a credential. On Windows the host cannot run a
// kubelet, so its WSL distribution stands in for it, and CasOS installs WSL
// when the host has none.
//
// Everything runs in the background and reports through the server log. The
// deployment itself is an ordinary node deployment task, so the Machines page
// shows its progress and its logs.

const (
	localNodeBootstrapAttempts   = 3
	localNodeBootstrapRetryDelay = time.Minute
	localNodeDeployPollPeriod    = 5 * time.Second
)

// One run at a time: two would fight over wsl --shutdown and over the machine
// record they both write.
var localNodeBootstrapMutex sync.Mutex

// StartLocalNodeBootstrap enrolls the CasOS host as a worker node in the
// background. It is a no-op where that cannot work, so main can call it
// unconditionally.
func StartLocalNodeBootstrap() {
	if !conf.GetConfigBoolDefault("autoEnrollLocalNode", true) {
		logs.Info("automatic node setup is disabled by autoEnrollLocalNode")
		return
	}
	if err := localNodePlatformSupported(); err != nil {
		// A permanent property of the host, so there is nothing to retry.
		logs.Warning("automatic node setup: %v", err)
		return
	}
	go func() {
		defer func() {
			if v := recover(); v != nil {
				logs.Error("automatic node setup panic: %v", v)
			}
		}()
		runLocalNodeBootstrap(defaultService.contextSnapshot())
	}()
}

func localNodePlatformSupported() error {
	switch runtime.GOOS {
	case "windows", "linux":
		return nil
	case "darwin":
		return fmt.Errorf("macOS cannot host a Kubernetes worker node, because the kubelet needs a Linux kernel: add a Linux machine on the Machines page, or run CasOS inside a Linux VM")
	default:
		return fmt.Errorf("%s cannot host a Kubernetes worker node: add a Linux machine on the Machines page", runtime.GOOS)
	}
}

// runLocalNodeBootstrap retries, because a cluster left without a node is the
// one outcome worth spending a few more minutes to avoid: the control plane
// may still have been settling, or WSL may still have been booting.
func runLocalNodeBootstrap(ctx context.Context) {
	for attempt := 1; attempt <= localNodeBootstrapAttempts; attempt++ {
		err := bootstrapLocalNode(ctx)
		if err == nil {
			logs.Info("automatic node setup: this machine is a Ready worker node")
			return
		}
		logs.Warning("automatic node setup (attempt %d/%d): %v", attempt, localNodeBootstrapAttempts, err)
		if ctx.Err() != nil || attempt == localNodeBootstrapAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(localNodeBootstrapRetryDelay):
		}
	}
	logs.Error("automatic node setup did not succeed, the cluster has no worker node until one is added on the Machines page")
}

func bootstrapLocalNode(ctx context.Context) error {
	localNodeBootstrapMutex.Lock()
	defer localNodeBootstrapMutex.Unlock()

	machine, err := localNodeMachine(ctx)
	if err != nil {
		return err
	}
	task, err := defaultService.DeployMachineNode(MachineNodeDeployRequest{
		Owner:       machine.Owner,
		MachineName: machine.Name,
		NodeName:    machine.Name,
	})
	if err != nil {
		return fmt.Errorf("deploy node %s: %w", machine.Name, err)
	}
	logs.Info("automatic node setup: deploying node %s as task %d", machine.Name, task.Id)
	return waitForNodeDeployTask(ctx, task.Id)
}

// localNodeMachine registers whatever stands in for this machine and returns
// the machine record to deploy onto.
func localNodeMachine(ctx context.Context) (*object.Machine, error) {
	if runtime.GOOS == "windows" {
		return localWSLNodeMachine(ctx)
	}
	return AddLocalHostMachine(ctx, "admin")
}

func localWSLNodeMachine(ctx context.Context) (*object.Machine, error) {
	distro, err := localWSLNodeDistro(ctx)
	if err != nil {
		return nil, err
	}
	if _, err = wsl.EnsureSystemd(ctx, distro, func(line string) { logs.Info("wsl setup: %s", line) }); err != nil {
		return nil, err
	}
	result, err := AddLocalWSLMachine(ctx, "admin", distro)
	if err != nil {
		return nil, fmt.Errorf("enroll %s: %w", distro, err)
	}
	return result.Machine, nil
}

// localWSLNodeDistro returns the distro to enroll, installing WSL first when
// the host has nothing that can host a node.
func localWSLNodeDistro(ctx context.Context) (string, error) {
	status, err := wsl.Detect(ctx)
	if err != nil {
		return "", err
	}
	if selected := status.NodeDistro(); selected != nil {
		logs.Info("automatic node setup: using WSL distribution %s", selected.Name)
		return selected.Name, nil
	}

	logs.Info("automatic node setup: no usable WSL distribution (%s), installing one", status.Detail)
	installed, err := wsl.Install(ctx, wsl.DefaultInstallDistro, func(line string) { logs.Info("wsl install: %s", line) })
	if err != nil {
		return "", err
	}
	selected := installed.NodeDistro()
	if selected == nil {
		return "", fmt.Errorf("WSL was installed but no usable distribution is registered yet, restart Windows to finish enabling WSL")
	}
	logs.Info("automatic node setup: installed WSL distribution %s", selected.Name)
	return selected.Name, nil
}

func waitForNodeDeployTask(ctx context.Context, taskId int64) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(localNodeDeployPollPeriod):
		}
		task, err := object.GetMachineNodeDeployTask(taskId)
		if err != nil || task == nil {
			continue
		}
		switch task.Status {
		case object.MachineNodeDeployStatusSucceeded:
			return nil
		case object.MachineNodeDeployStatusFailed:
			if task.ErrorMsg != "" {
				return fmt.Errorf("node deployment failed: %s", task.ErrorMsg)
			}
			return fmt.Errorf("node deployment failed")
		}
	}
}
