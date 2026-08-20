package server

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/sirupsen/logrus"
	schedapp "k8s.io/kubernetes/cmd/kube-scheduler/app"

	"github.com/casosorg/casos/util"
)

const (
	// componentBindAddress is the interface the in-process scheduler and
	// controller-manager serve their secure ports on.
	componentBindAddress = "127.0.0.1"

	// schedulerDefaultPort is the secure port kube-scheduler serves on. Like
	// the controller-manager port it is loopback-only and dialled by nobody,
	// so it can move when the number is already taken.
	schedulerDefaultPort = 10259
)

// StartScheduler launches kube-scheduler in-process. Must be called after the
// apiserver is ready (i.e. after the readyCh from Start is closed).
func StartScheduler(ctx context.Context, cfg Config) error {
	certDir := filepath.Join(cfg.DataDir, "tls")
	kubeconfigPath, err := ensureComponentKubeconfig(
		certDir,
		fmt.Sprintf("https://127.0.0.1:%d", cfg.ApiserverPort),
		"scheduler",
	)
	if err != nil {
		return fmt.Errorf("scheduler kubeconfig: %w", err)
	}

	securePort, err := util.FreePortFrom(componentBindAddress, schedulerDefaultPort)
	if err != nil {
		return fmt.Errorf("scheduler port: %w", err)
	}
	if securePort != schedulerDefaultPort {
		logrus.Warnf("port %d is taken by another program, scheduler is using %d instead", schedulerDefaultPort, securePort)
	}

	go func() {
		cmd := schedapp.NewSchedulerCommand(ctx.Done())
		cmd.SetArgs([]string{
			"--kubeconfig=" + kubeconfigPath,
			"--leader-elect=false",
			"--bind-address=" + componentBindAddress,
			fmt.Sprintf("--secure-port=%d", securePort),
		})
		if err := cmd.ExecuteContext(ctx); err != nil && ctx.Err() == nil {
			logrus.Errorf("scheduler exited: %v", err)
		}
	}()

	logrus.Info("scheduler started in-process")
	return nil
}
