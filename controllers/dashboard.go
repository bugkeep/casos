package controllers

import (
	"fmt"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"

	"github.com/casosorg/casos/object"
)

type unhealthyPod struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Phase     string `json:"phase"`
	Reason    string `json:"reason"`
}

var dashboardContainerFailureReasons = map[string]struct{}{
	"CrashLoopBackOff":           {},
	"OOMKilled":                  {},
	"ImagePullBackOff":           {},
	"ErrImagePull":               {},
	"ErrImageNeverPull":          {},
	"InvalidImageName":           {},
	"CreateContainerConfigError": {},
	"CreateContainerError":       {},
	"RunContainerError":          {},
}

const (
	dashboardHealthHealthy   = "healthy"
	dashboardHealthUnhealthy = "unhealthy"
	dashboardHealthUnknown   = "unknown"
)

type dashboardStats struct {
	NodesTotal           int            `json:"nodesTotal"`
	NodesReady           int            `json:"nodesReady"`
	NodesByOS            map[string]int `json:"nodesByOS"`
	NodesByArch          map[string]int `json:"nodesByArch"`
	PodsTotal            int            `json:"podsTotal"`
	PodsRunning          int            `json:"podsRunning"`
	PodsByPhase          map[string]int `json:"podsByPhase"`
	PodsByNamespace      map[string]int `json:"podsByNamespace"`
	NamespacesTotal      int            `json:"namespacesTotal"`
	ServicesTotal        int            `json:"servicesTotal"`
	ServicesByType       map[string]int `json:"servicesByType"`
	ConfigMapsTotal      int            `json:"configMapsTotal"`
	ServiceAccounts      int            `json:"serviceAccounts"`
	DeploymentsTotal     int            `json:"deploymentsTotal"`
	DeploymentsAvailable int            `json:"deploymentsAvailable"`
	UnhealthyPods        []unhealthyPod `json:"unhealthyPods"`
	// Deprecated compatibility field. False means either unhealthy or unknown;
	// consumers should use HealthStatus when they need to distinguish the two.
	Healthy      bool   `json:"healthy"`
	HealthStatus string `json:"healthStatus"`
	// Resource usage (from metrics-server; zero when metrics-server is absent)
	ClusterCPUUsedM   int64 `json:"clusterCPUUsedM"`
	ClusterCPUTotalM  int64 `json:"clusterCPUTotalM"`
	ClusterMemUsedMi  int64 `json:"clusterMemUsedMi"`
	ClusterMemTotalMi int64 `json:"clusterMemTotalMi"`
}

// GetDashboard returns aggregated cluster statistics.
// @router /api/get-dashboard [get]
func (c *ApiController) GetDashboard() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}

	stats := dashboardStats{
		NodesByOS:       map[string]int{},
		NodesByArch:     map[string]int{},
		PodsByPhase:     map[string]int{},
		PodsByNamespace: map[string]int{},
		ServicesByType:  map[string]int{},
		UnhealthyPods:   []unhealthyPod{},
	}

	nodesLoaded := false
	if nodes, err := object.GetNodes(cfg); err == nil {
		nodesLoaded = true
		stats.NodesTotal = len(nodes)
		for _, n := range nodes {
			for _, cond := range n.Status.Conditions {
				if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
					stats.NodesReady++
				}
			}
			os := n.Status.NodeInfo.OperatingSystem
			if os == "" {
				os = "unknown"
			}
			stats.NodesByOS[os]++
			arch := n.Status.NodeInfo.Architecture
			if arch == "" {
				arch = "unknown"
			}
			stats.NodesByArch[arch]++
		}
	}

	podsLoaded := false
	if pods, err := object.GetPods(cfg, ""); err == nil {
		podsLoaded = true
		stats.PodsTotal = len(pods)
		for i := range pods {
			p := &pods[i]
			phase := string(p.Status.Phase)
			if phase == "" {
				phase = "Unknown"
			}
			stats.PodsByPhase[phase]++
			if phase == "Running" {
				stats.PodsRunning++
			}
			ns := p.Namespace
			if ns == "" {
				ns = "default"
			}
			stats.PodsByNamespace[ns]++

			reason := dashboardPodUnhealthyReason(p)
			if reason != "" {
				stats.UnhealthyPods = append(stats.UnhealthyPods, unhealthyPod{
					Namespace: ns,
					Name:      p.Name,
					Phase:     phase,
					Reason:    reason,
				})
			}
		}
	}

	if namespaces, err := object.GetNamespaces(cfg); err == nil {
		stats.NamespacesTotal = len(namespaces)
	}

	if services, err := object.GetServices(cfg, ""); err == nil {
		stats.ServicesTotal = len(services)
		for _, svc := range services {
			t := string(svc.Spec.Type)
			if t == "" {
				t = "ClusterIP"
			}
			stats.ServicesByType[t]++
		}
	}

	if deployments, err := object.GetDeployments(cfg, ""); err == nil {
		stats.DeploymentsTotal = len(deployments)
		for _, d := range deployments {
			desired := int32(1)
			if d.Spec.Replicas != nil {
				desired = *d.Spec.Replicas
			}
			if d.Status.AvailableReplicas >= desired {
				stats.DeploymentsAvailable++
			}
		}
	}

	if configMaps, err := object.GetConfigMaps(cfg, ""); err == nil {
		stats.ConfigMapsTotal = len(configMaps)
	}

	if sas, err := object.GetServiceAccounts(cfg, ""); err == nil {
		stats.ServiceAccounts = len(sas)
	}

	stats.HealthStatus = dashboardHealthStatus(stats, nodesLoaded, podsLoaded)
	stats.Healthy = stats.HealthStatus == dashboardHealthHealthy

	// Cluster resource usage — best-effort, ignored if kubelet is unreachable
	certDir := ""
	if sc := getServerConfig(); sc != nil {
		certDir = filepath.Join(sc.DataDir, "tls")
	}
	if clusterMetrics, err := object.GetClusterMetrics(cfg, certDir); err == nil {
		for _, nm := range clusterMetrics.Nodes {
			stats.ClusterCPUUsedM += nm.CPUUsedM
			stats.ClusterCPUTotalM += nm.CPUTotalM
			stats.ClusterMemUsedMi += nm.MemUsedMi
			stats.ClusterMemTotalMi += nm.MemTotalMi
		}
	}

	c.ResponseOk(stats)
}

func dashboardHealthStatus(stats dashboardStats, nodesLoaded, podsLoaded bool) string {
	if nodesLoaded && stats.NodesTotal > 0 && stats.NodesReady != stats.NodesTotal {
		return dashboardHealthUnhealthy
	}
	if podsLoaded && len(stats.UnhealthyPods) > 0 {
		return dashboardHealthUnhealthy
	}
	if !nodesLoaded || !podsLoaded || stats.NodesTotal == 0 {
		return dashboardHealthUnknown
	}
	return dashboardHealthHealthy
}

func dashboardPodUnhealthyReason(p *corev1.Pod) string {
	if p.Status.Phase == corev1.PodFailed {
		if p.Status.Reason != "" {
			return p.Status.Reason
		}
		return "Failed"
	}
	if p.Status.Phase == "" || p.Status.Phase == corev1.PodUnknown {
		return "Unknown"
	}

	for _, statuses := range [][]corev1.ContainerStatus{
		p.Status.InitContainerStatuses,
		p.Status.ContainerStatuses,
	} {
		for _, cs := range statuses {
			if reason := dashboardContainerUnhealthyReason(cs); reason != "" {
				return reason
			}
		}
	}
	if p.Status.Phase == corev1.PodPending {
		for _, condition := range p.Status.Conditions {
			if condition.Type != corev1.PodScheduled || condition.Status != corev1.ConditionFalse {
				continue
			}
			switch condition.Reason {
			case corev1.PodReasonUnschedulable, corev1.PodReasonSchedulerError:
				return condition.Reason
			}
		}
	}
	return ""
}

func dashboardContainerUnhealthyReason(status corev1.ContainerStatus) string {
	if status.State.Waiting != nil {
		if _, ok := dashboardContainerFailureReasons[status.State.Waiting.Reason]; ok {
			return status.State.Waiting.Reason
		}
	}
	if status.State.Terminated != nil && status.State.Terminated.ExitCode != 0 {
		if status.State.Terminated.Reason != "" {
			return status.State.Terminated.Reason
		}
		return fmt.Sprintf("ExitCode %d", status.State.Terminated.ExitCode)
	}
	return ""
}
