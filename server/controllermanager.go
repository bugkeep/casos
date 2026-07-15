package server

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/sirupsen/logrus"
	cmapp "k8s.io/kubernetes/cmd/kube-controller-manager/app"
)

type controllerManagerStartGuard struct {
	mu      sync.Mutex
	running bool
}

func (g *controllerManagerStartGuard) claim() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.running {
		return false
	}
	g.running = true
	return true
}

func (g *controllerManagerStartGuard) release() {
	g.mu.Lock()
	g.running = false
	g.mu.Unlock()
}

var controllerManagerGuard controllerManagerStartGuard

// StartControllerManager launches kube-controller-manager in-process. Must be
// called after the apiserver is ready.
func StartControllerManager(ctx context.Context, cfg Config) error {
	if !controllerManagerGuard.claim() {
		logrus.Info("controller-manager is already running in-process")
		return nil
	}

	certDir := filepath.Join(cfg.DataDir, "tls")
	kubeconfigPath, err := ensureComponentKubeconfig(
		certDir,
		fmt.Sprintf("https://127.0.0.1:%d", cfg.ApiserverPort),
		"controller-manager",
	)
	if err != nil {
		controllerManagerGuard.release()
		return fmt.Errorf("controller-manager kubeconfig: %w", err)
	}

	caKey := filepath.Join(certDir, "ca.key")
	caCrt := filepath.Join(certDir, "ca.crt")
	saKey := filepath.Join(certDir, "sa.key")

	go func() {
		defer controllerManagerGuard.release()
		cmd := cmapp.NewControllerManagerCommand()
		cmd.SetArgs(controllerManagerArgs(kubeconfigPath, caCrt, caKey, saKey))
		if err := cmd.ExecuteContext(ctx); err != nil && ctx.Err() == nil {
			logrus.Errorf("controller-manager exited: %v", err)
		}
	}()

	logrus.Info("controller-manager started in-process")
	return nil
}

func controllerManagerArgs(kubeconfigPath, caCrt, caKey, saKey string) []string {
	return []string{
		"--kubeconfig=" + kubeconfigPath,
		"--leader-elect=true",
		"--leader-elect-resource-namespace=kube-system",
		"--leader-elect-resource-name=casos-controller-manager",
		"--bind-address=127.0.0.1",
		"--secure-port=10257",
		"--cluster-signing-cert-file=" + caCrt,
		"--cluster-signing-key-file=" + caKey,
		"--root-ca-file=" + caCrt,
		"--service-account-private-key-file=" + saKey,
		"--allocate-node-cidrs=true",
		"--cluster-cidr=10.244.0.0/16",
	}
}
