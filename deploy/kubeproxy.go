package deploy

import (
	"context"
	"fmt"
)

func (d *NodeDeployer) startKubeProxy(ctx context.Context, runner *NodeDeploySSHRunner) error {
	d.logStep(nodeDeployPhaseStarting, "Starting kube-proxy")
	if _, err := runner.RunRootContext(ctx, kubeProxyStartCommand()); err != nil {
		return fmt.Errorf("start kube-proxy: %w", err)
	}
	return nil
}

func kubeProxyStartCommand() string {
	return `systemctl daemon-reload && systemctl enable kube-proxy && systemctl restart kube-proxy
for i in $(seq 1 120); do
  iptables-save 2>/dev/null | grep -q '10.43.0.1' && exit 0
  sleep 1
done
echo "kube-proxy did not program the Kubernetes Service IP" >&2
exit 1`
}

func kubeProxyConfig() string {
	return fmt.Sprintf(`apiVersion: kubeproxy.config.k8s.io/v1alpha1
kind: KubeProxyConfiguration
clientConnection:
  kubeconfig: /etc/kubernetes/worker.kubeconfig
mode: iptables
clusterCIDR: %s
`, nodeDeployClusterCIDR)
}

func kubeProxyService() string {
	return `[Unit]
Description=Kubernetes Kube-Proxy
After=network.target

[Service]
ExecStart=/usr/local/bin/kube-proxy \
  --config=/var/lib/kube-proxy/config.yaml \
  --v=2
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`
}
