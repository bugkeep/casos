package deploy

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/beego/beego/logs"
	"github.com/casosorg/casos/conf"
	"github.com/casosorg/casos/wsl"
)

// Zero configuration worker node on Windows.
//
// The one machine CasOS can reach without asking the user anything is the WSL
// distro next to it, so on Windows it enrolls that distro as a worker node by
// itself at startup: it installs WSL when the host has none, switches the
// distro to systemd, opens an SSH path into it and then hands over to the
// ordinary worker node deployment. Everything runs in the background and
// reports through the server log; the deployment itself is a normal node
// deployment task, so the Machines page shows its progress and its logs.

var localWSLBootstrapOnce sync.Mutex

// StartLocalWSLBootstrap kicks the setup off in the background. It is a no-op
// on a host it does not apply to, so main can call it unconditionally.
func StartLocalWSLBootstrap() {
	if runtime.GOOS != "windows" {
		return
	}
	if !conf.GetConfigBoolDefault("autoEnrollLocalWsl", true) {
		logs.Info("automatic local WSL node setup is disabled by autoEnrollLocalWsl")
		return
	}
	go func() {
		defer func() {
			if v := recover(); v != nil {
				logs.Error("automatic WSL node setup panic: %v", v)
			}
		}()
		if err := bootstrapLocalWSLNode(defaultService.contextSnapshot()); err != nil {
			logs.Warning("automatic WSL node setup: %v", err)
		}
	}()
}

func bootstrapLocalWSLNode(ctx context.Context) error {
	// One run at a time: a second one would fight the first over wsl --shutdown
	// and over the machine record they both write.
	localWSLBootstrapOnce.Lock()
	defer localWSLBootstrapOnce.Unlock()

	distro, err := localWSLNodeDistro(ctx)
	if err != nil {
		return err
	}
	if _, err = wsl.EnsureSystemd(ctx, distro, func(line string) { logs.Info("wsl setup: %s", line) }); err != nil {
		return err
	}

	result, err := AddLocalWSLMachine(ctx, "admin", distro)
	if err != nil {
		return fmt.Errorf("enroll %s: %w", distro, err)
	}
	machine := result.Machine

	task, err := defaultService.DeployMachineNode(MachineNodeDeployRequest{
		Owner:       machine.Owner,
		MachineName: machine.Name,
		NodeName:    machine.Name,
	})
	if err != nil {
		return fmt.Errorf("deploy node %s: %w", machine.Name, err)
	}
	logs.Info("automatic WSL node setup: deploying node %s from %s as task %d", machine.Name, distro, task.Id)
	return nil
}

// localWSLNodeDistro returns the distro to enroll, installing WSL first when
// the host has nothing that can host a node.
func localWSLNodeDistro(ctx context.Context) (string, error) {
	status, err := wsl.Detect(ctx)
	if err != nil {
		return "", err
	}
	if selected := status.NodeDistro(); selected != nil {
		logs.Info("automatic WSL node setup: using WSL distribution %s", selected.Name)
		return selected.Name, nil
	}

	logs.Info("automatic WSL node setup: no usable WSL distribution (%s), installing one", status.Detail)
	installed, err := wsl.Install(ctx, wsl.DefaultInstallDistro, func(line string) { logs.Info("wsl install: %s", line) })
	if err != nil {
		return "", err
	}
	selected := installed.NodeDistro()
	if selected == nil {
		return "", fmt.Errorf("WSL was installed but no usable distribution is registered yet, restart Windows to finish enabling WSL")
	}
	logs.Info("automatic WSL node setup: installed WSL distribution %s", selected.Name)
	return selected.Name, nil
}
