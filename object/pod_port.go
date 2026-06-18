package object

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

func OpenPodUI(cfg *rest.Config, namespace, podName string, containerPort int32) (int, func(), error) {
	if cfg == nil {
		return 0, nil, errors.New("nil rest config")
	}
	if namespace == "" || podName == "" || containerPort < 1 || containerPort > 65535 {
		return 0, nil, errors.New("namespace, pod name and containerPort between 1 and 65535 are required")
	}

	transport, upgrader, err := spdy.RoundTripperFor(cfg)
	if err != nil {
		return 0, nil, fmt.Errorf("spdy roundtripper: %w", err)
	}

	serverURL, err := url.Parse(cfg.Host)
	if err != nil {
		return 0, nil, fmt.Errorf("parse apiserver host %q: %w", cfg.Host, err)
	}
	serverURL.Path = fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", namespace, podName)

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", serverURL)
	stopChan := make(chan struct{})
	readyChan := make(chan struct{})
	fw, err := portforward.NewOnAddresses(dialer, []string{"127.0.0.1"}, []string{fmt.Sprintf("0:%d", containerPort)}, stopChan, readyChan, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		return 0, nil, fmt.Errorf("new portforward: %w", err)
	}

	runErr := make(chan error, 1)
	go func() {
		runErr <- fw.ForwardPorts()
	}()

	select {
	case <-readyChan:
	case err := <-runErr:
		if err == nil {
			err = errors.New("portforward exited")
		}
		return 0, nil, fmt.Errorf("portforward exited before ready: %w", err)
	}

	ports, err := fw.GetPorts()
	if err != nil || len(ports) == 0 {
		close(stopChan)
		<-runErr
		if err == nil {
			err = errors.New("no forwarded ports")
		}
		return 0, nil, fmt.Errorf("get forwarded port: %w", err)
	}

	stop := sync.OnceFunc(func() {
		close(stopChan)
		<-runErr
	})
	return int(ports[0].Local), stop, nil
}

func ContainerPortsFromPod(pod *corev1.Pod) []int32 {
	if pod == nil {
		return nil
	}
	seen := map[int32]bool{}
	ports := []int32{}
	for _, container := range pod.Spec.Containers {
		for _, port := range container.Ports {
			if port.ContainerPort > 0 && !seen[port.ContainerPort] {
				seen[port.ContainerPort] = true
				ports = append(ports, port.ContainerPort)
			}
		}
	}
	sort.Slice(ports, func(i, j int) bool {
		return ports[i] < ports[j]
	})
	return ports
}
