package server

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/sirupsen/logrus"
	cmapp "k8s.io/kubernetes/cmd/kube-controller-manager/app"

	"github.com/casosorg/casos/util"
)

// controllerManagerDefaultPort is the secure port kube-controller-manager
// serves on. It is bound to loopback and nothing dials it, so CasOS moves it
// out of the way when another program already holds the number.
const controllerManagerDefaultPort = 20257

// StartControllerManager launches kube-controller-manager in-process. Must be
// called after the apiserver is ready.
func StartControllerManager(ctx context.Context, cfg Config) error {
	certDir := filepath.Join(cfg.DataDir, "tls")
	kubeconfigPath, err := ensureComponentKubeconfig(
		certDir,
		fmt.Sprintf("https://127.0.0.1:%d", cfg.ApiserverPort),
		"controller-manager",
	)
	if err != nil {
		return fmt.Errorf("controller-manager kubeconfig: %w", err)
	}

	securePort, err := util.FreePortFrom(componentBindAddress, controllerManagerDefaultPort)
	if err != nil {
		return fmt.Errorf("controller-manager port: %w", err)
	}
	if securePort != controllerManagerDefaultPort {
		logrus.Warnf("port %d is taken by another program, controller-manager is using %d instead", controllerManagerDefaultPort, securePort)
	}

	caKey := filepath.Join(certDir, "ca.key")
	caCrt := filepath.Join(certDir, "ca.crt")
	saKey := filepath.Join(certDir, "sa.key")

	go func() {
		cmd := cmapp.NewControllerManagerCommand()
		cmd.SetArgs([]string{
			"--kubeconfig=" + kubeconfigPath,
			"--leader-elect=false",
			"--bind-address=" + componentBindAddress,
			fmt.Sprintf("--secure-port=%d", securePort),
			"--cluster-signing-cert-file=" + caCrt,
			"--cluster-signing-key-file=" + caKey,
			"--root-ca-file=" + caCrt,
			"--service-account-private-key-file=" + saKey,
			"--allocate-node-cidrs=true",
			"--cluster-cidr=10.244.0.0/16",
		})
		if err := cmd.ExecuteContext(ctx); err != nil && ctx.Err() == nil {
			logrus.Errorf("controller-manager exited: %v", err)
		}
	}()

	logrus.Info("controller-manager started in-process")
	return nil
}
