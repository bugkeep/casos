package deploy

import (
	"context"
	"fmt"
)

func (d *NodeDeployer) startKubelet(ctx context.Context, runner *NodeDeploySSHRunner) error {
	d.logStep(nodeDeployPhaseStarting, "Starting kubelet")
	if _, err := runner.RunRootContext(ctx, "systemctl daemon-reload && systemctl enable kubelet && systemctl restart kubelet"); err != nil {
		return fmt.Errorf("start kubelet: %w", err)
	}
	return nil
}

func kubeletConfig() string {
	return fmt.Sprintf(`apiVersion: kubelet.config.k8s.io/v1beta1
kind: KubeletConfiguration
cgroupDriver: systemd
failSwapOn: false
containerRuntimeEndpoint: unix:///run/containerd/containerd.sock
resolvConf: %s
clusterDNS:
  - %s
clusterDomain: cluster.local
`, nodeDeployResolverPath, nodeDeployClusterDNS)
}

func kubeletService(nodeName string) string {
	return fmt.Sprintf(`[Unit]
Description=Kubernetes Kubelet
After=containerd.service
Requires=containerd.service

[Service]
ExecStart=/usr/local/bin/kubelet \
  --kubeconfig=/etc/kubernetes/worker.kubeconfig \
  --config=/var/lib/kubelet/config.yaml \
  --client-ca-file=/etc/kubernetes/ca.crt \
  --register-node=true \
  --register-with-taints=%s=true:NoSchedule \
  --hostname-override=%s \
  --v=2
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`, workerBootstrapTaintKey, nodeName)
}
