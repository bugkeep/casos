package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type localNodePortSpec struct {
	Namespace   string
	ServiceName string
	ServicePort int32
	NodePort    int32
}

type localNodePortProxyManager struct {
	baseURL   string
	mu        sync.Mutex
	listeners map[int32]net.Listener
}

var defaultLocalNodePortManager = &localNodePortProxyManager{
	listeners: map[int32]net.Listener{},
}

func StartLocalNodePortProxyManager(ctx context.Context, cfg *rest.Config, baseURL string) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("local nodeport proxy client: %w", err)
	}
	defaultLocalNodePortManager.baseURL = strings.TrimRight(baseURL, "/")
	go defaultLocalNodePortManager.run(ctx, client)
	return nil
}

func LocalNodePortAccessURL(nodePort int32) (string, bool) {
	if runtime.GOOS != "windows" || nodePort <= 0 {
		return "", false
	}
	return fmt.Sprintf("http://127.0.0.1:%d/", nodePort), defaultLocalNodePortManager.hasListener(nodePort)
}

func (m *localNodePortProxyManager) hasListener(nodePort int32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.listeners[nodePort]
	return ok
}

func (m *localNodePortProxyManager) run(ctx context.Context, client kubernetes.Interface) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		m.syncOnce(ctx, client)
		select {
		case <-ctx.Done():
			m.closeAll()
			return
		case <-ticker.C:
		}
	}
}

func (m *localNodePortProxyManager) syncOnce(ctx context.Context, client kubernetes.Interface) {
	list, err := client.CoreV1().Services(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}

	want := map[int32]localNodePortSpec{}
	for _, svc := range list.Items {
		if svc.Spec.Type != corev1.ServiceTypeNodePort && svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
			continue
		}
		for _, port := range svc.Spec.Ports {
			if port.NodePort <= 0 || port.Port <= 0 {
				continue
			}
			if _, exists := want[port.NodePort]; exists {
				continue
			}
			want[port.NodePort] = localNodePortSpec{
				Namespace:   svc.Namespace,
				ServiceName: svc.Name,
				ServicePort: port.Port,
				NodePort:    port.NodePort,
			}
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for nodePort, spec := range want {
		if _, ok := m.listeners[nodePort]; ok {
			continue
		}
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", nodePort))
		if err != nil {
			continue
		}
		m.listeners[nodePort] = ln
		handler := newLocalNodePortProxyHandler(m.baseURL, spec)
		go func(listener net.Listener) {
			_ = (&http.Server{Handler: handler}).Serve(listener)
		}(ln)
	}

	for nodePort, ln := range m.listeners {
		if _, ok := want[nodePort]; ok {
			continue
		}
		_ = ln.Close()
		delete(m.listeners, nodePort)
	}
}

func (m *localNodePortProxyManager) closeAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for nodePort, ln := range m.listeners {
		_ = ln.Close()
		delete(m.listeners, nodePort)
	}
}

func newLocalNodePortProxyHandler(baseURL string, spec localNodePortSpec) http.Handler {
	target, _ := url.Parse(strings.TrimRight(baseURL, "/"))
	rp := httputil.NewSingleHostReverseProxy(target)
	originalDirector := rp.Director
	rp.Director = func(r *http.Request) {
		originalDirector(r)
		basePath := fmt.Sprintf("/api/proxy-service/%s/%s/%d", spec.Namespace, spec.ServiceName, spec.ServicePort)
		if r.URL.Path == "" || r.URL.Path == "/" {
			r.URL.Path = basePath + "/"
		} else {
			r.URL.Path = basePath + r.URL.Path
		}
		r.Host = target.Host
	}
	return rp
}
