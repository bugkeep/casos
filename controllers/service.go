package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"

	"github.com/casosorg/casos/object"
	"github.com/casosorg/casos/server"
)

type portSummary struct {
	Name       string `json:"name"`
	Protocol   string `json:"protocol"`
	Port       int32  `json:"port"`
	TargetPort string `json:"targetPort"`
	NodePort   int32  `json:"nodePort,omitempty"`
}

type serviceSummary struct {
	Namespace       string            `json:"namespace"`
	Name            string            `json:"name"`
	Type            string            `json:"type"`
	ClusterIP       string            `json:"clusterIP"`
	ExternalName    string            `json:"externalName"`
	Selector        map[string]string `json:"selector"`
	Ports           []portSummary     `json:"ports"`
	AccessReady     bool              `json:"accessReady"`
	AccessURL       string            `json:"accessUrl"`
	AccessMessage   string            `json:"accessMessage"`
	CreatedAt       string            `json:"createdAt"`
	ResourceVersion string            `json:"resourceVersion"`
}

func toSvcSummary(svc corev1.Service) serviceSummary {
	ports := make([]portSummary, 0, len(svc.Spec.Ports))
	for _, p := range svc.Spec.Ports {
		ports = append(ports, portSummary{
			Name:       p.Name,
			Protocol:   string(p.Protocol),
			Port:       p.Port,
			TargetPort: p.TargetPort.String(),
			NodePort:   p.NodePort,
		})
	}
	return serviceSummary{
		Namespace:       svc.Namespace,
		Name:            svc.Name,
		Type:            string(svc.Spec.Type),
		ClusterIP:       svc.Spec.ClusterIP,
		ExternalName:    svc.Spec.ExternalName,
		Selector:        svc.Spec.Selector,
		Ports:           ports,
		CreatedAt:       svc.CreationTimestamp.UTC().Format("2006-01-02 15:04:05"),
		ResourceVersion: svc.ResourceVersion,
	}
}

func toServiceAccessStatus(summary *serviceSummary, hasEndpoints bool) {
	if summary == nil {
		return
	}
	if summary.Type != string(corev1.ServiceTypeNodePort) && summary.Type != string(corev1.ServiceTypeLoadBalancer) {
		return
	}
	if !hasEndpoints {
		summary.AccessReady = false
		summary.AccessMessage = "No ready endpoints behind this Service"
		return
	}
	for _, p := range summary.Ports {
		if p.NodePort > 0 {
			if accessURL, ok := server.LocalNodePortAccessURL(p.NodePort); ok {
				summary.AccessReady = true
				summary.AccessURL = accessURL
				summary.AccessMessage = ""
				return
			}
		}
		if p.Port > 0 {
			summary.AccessReady = true
			summary.AccessURL = fmt.Sprintf("/api/proxy-service/%s/%s/%d/", summary.Namespace, summary.Name, p.Port)
			summary.AccessMessage = ""
			return
		}
	}
	summary.AccessMessage = "Service has no routable ports"
}

// GetServices
// @router /api/get-services [get]
func (c *ApiController) GetServices() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	svcs, err := object.GetServices(cfg, namespace)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	result := make([]serviceSummary, 0, len(svcs))
	for _, svc := range svcs {
		summary := toSvcSummary(svc)
		hasEndpoints := false
		if ep, err := object.GetEndpoints(cfg, svc.Namespace, svc.Name); err == nil {
			hasEndpoints = endpointsHasAddresses(ep)
		} else if apierrors.IsNotFound(err) {
			summary.AccessReady = false
			summary.AccessMessage = "This Service has no Endpoints object"
		}
		if summary.AccessMessage == "" {
			toServiceAccessStatus(&summary, hasEndpoints)
		}
		result = append(result, summary)
	}
	c.ResponseOk(result)
}

// GetService
// @router /api/get-service [get]
func (c *ApiController) GetService() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	name := c.GetString("name")
	svc, err := object.GetService(cfg, namespace, name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	summary := toSvcSummary(*svc)
	if ep, err := object.GetEndpoints(cfg, svc.Namespace, svc.Name); err == nil {
		toServiceAccessStatus(&summary, endpointsHasAddresses(ep))
	} else if apierrors.IsNotFound(err) {
		summary.AccessReady = false
		summary.AccessMessage = "This Service has no Endpoints object"
	}
	c.ResponseOk(summary)
}

type portRequest struct {
	Name       string `json:"name"`
	Protocol   string `json:"protocol"`
	Port       int32  `json:"port"`
	TargetPort string `json:"targetPort"`
	NodePort   int32  `json:"nodePort,omitempty"`
}

type serviceRequest struct {
	Namespace       string            `json:"namespace"`
	Name            string            `json:"name"`
	Type            string            `json:"type"`
	ExternalName    string            `json:"externalName"`
	Selector        map[string]string `json:"selector"`
	Ports           []portRequest     `json:"ports"`
	ResourceVersion string            `json:"resourceVersion"`
}

func normalizeServiceRequest(req *serviceRequest) error {
	if req.Type == "" {
		req.Type = string(corev1.ServiceTypeClusterIP)
	}
	if req.Type == string(corev1.ServiceTypeExternalName) {
		req.ExternalName = strings.TrimSpace(req.ExternalName)
		if req.ExternalName == "" {
			return fmt.Errorf("ExternalName service requires externalName")
		}
		req.Selector = nil
		req.Ports = nil
	} else {
		req.ExternalName = ""
	}
	if req.Type != string(corev1.ServiceTypeNodePort) && req.Type != string(corev1.ServiceTypeLoadBalancer) {
		for i := range req.Ports {
			req.Ports[i].NodePort = 0
		}
	}
	return nil
}

func buildServiceSpec(req serviceRequest) corev1.ServiceSpec {
	svcType := corev1.ServiceType(req.Type)
	if svcType == "" {
		svcType = corev1.ServiceTypeClusterIP
	}
	ports := make([]corev1.ServicePort, 0, len(req.Ports))
	for _, p := range req.Ports {
		proto := corev1.Protocol(p.Protocol)
		if proto == "" {
			proto = corev1.ProtocolTCP
		}
		sp := corev1.ServicePort{
			Name:     p.Name,
			Protocol: proto,
			Port:     p.Port,
			NodePort: p.NodePort,
		}
		if n, err := strconv.Atoi(p.TargetPort); err == nil {
			sp.TargetPort = intstr.FromInt32(int32(n))
		} else if p.TargetPort != "" {
			sp.TargetPort = intstr.FromString(p.TargetPort)
		} else {
			sp.TargetPort = intstr.FromInt32(p.Port)
		}
		ports = append(ports, sp)
	}
	return corev1.ServiceSpec{
		Type:         svcType,
		Selector:     req.Selector,
		Ports:        ports,
		ExternalName: req.ExternalName,
	}
}

// AddService
// @router /api/add-service [post]
func (c *ApiController) AddService() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req serviceRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if err := normalizeServiceRequest(&req); err != nil {
		c.ResponseError(err.Error())
		return
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: buildServiceSpec(req),
	}
	created, err := object.AddService(cfg, svc)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toSvcSummary(*created))
}

// UpdateService
// @router /api/update-service [post]
func (c *ApiController) UpdateService() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req serviceRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if err := normalizeServiceRequest(&req); err != nil {
		c.ResponseError(err.Error())
		return
	}
	// Fetch current to preserve immutable fields when still applicable.
	existing, err := object.GetService(cfg, req.Namespace, req.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	newSpec := buildServiceSpec(req)
	if req.Type != string(corev1.ServiceTypeExternalName) {
		newSpec.ClusterIP = existing.Spec.ClusterIP
		newSpec.ClusterIPs = existing.Spec.ClusterIPs
	}
	existing.Spec = newSpec
	existing.ResourceVersion = req.ResourceVersion
	updated, err := object.UpdateService(cfg, existing)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toSvcSummary(*updated))
}

// DeleteService
// @router /api/delete-service [post]
func (c *ApiController) DeleteService() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req serviceRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if err := object.DeleteService(cfg, req.Namespace, req.Name); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}

func endpointsHasAddresses(ep *corev1.Endpoints) bool {
	if ep == nil {
		return false
	}
	for _, subset := range ep.Subsets {
		if len(subset.Addresses) > 0 || len(subset.NotReadyAddresses) > 0 {
			return true
		}
	}
	return false
}

// ProxyService
// @router /api/proxy-service/:namespace/:name/:port [get]
func (c *ApiController) ProxyService() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.Ctx.Input.Param(":namespace")
	name := c.Ctx.Input.Param(":name")
	portValue := c.Ctx.Input.Param(":port")
	if namespace == "" || name == "" || portValue == "" {
		c.CustomAbort(400, "missing service proxy parameters")
		return
	}
	port, err := strconv.Atoi(portValue)
	if err != nil || port <= 0 {
		c.CustomAbort(400, "invalid service port")
		return
	}
	service, err := object.GetService(cfg, namespace, name)
	if err != nil {
		c.CustomAbort(404, err.Error())
		return
	}
	endpoints, err := object.GetEndpoints(cfg, namespace, name)
	if err != nil || !endpointsHasAddresses(endpoints) {
		c.CustomAbort(503, "service has no ready endpoints")
		return
	}
	targetPort := int32(port)
	for _, p := range service.Spec.Ports {
		if p.Port == int32(port) {
			if p.TargetPort.IntValue() > 0 {
				targetPort = int32(p.TargetPort.IntValue())
			}
			break
		}
	}
	podName, podPort := firstEndpointTargetPod(endpoints, targetPort)
	if podName == "" || podPort == 0 {
		c.CustomAbort(503, "service endpoints do not expose the requested port")
		return
	}

	localPort, stopChan, err := startPodPortForward(cfg, namespace, podName, podPort)
	if err != nil {
		c.CustomAbort(502, err.Error())
		return
	}
	defer close(stopChan)

	proxyPath := strings.TrimPrefix(c.Ctx.Request.URL.Path, fmt.Sprintf("/api/proxy-service/%s/%s/%d", namespace, name, port))
	if proxyPath == "" {
		proxyPath = "/"
	}
	targetURL := fmt.Sprintf("http://127.0.0.1:%d%s", localPort, proxyPath)
	if rawQuery := c.Ctx.Request.URL.RawQuery; rawQuery != "" {
		targetURL += "?" + rawQuery
	}
	req, err := http.NewRequest(c.Ctx.Request.Method, targetURL, c.Ctx.Request.Body)
	if err != nil {
		c.CustomAbort(502, err.Error())
		return
	}
	copyHeaders(req.Header, c.Ctx.Request.Header)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		c.CustomAbort(502, err.Error())
		return
	}
	defer resp.Body.Close()
	copyHeaders(c.Ctx.ResponseWriter.Header(), resp.Header)
	c.Ctx.ResponseWriter.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(c.Ctx.ResponseWriter, resp.Body)
}

func firstEndpointTargetPod(ep *corev1.Endpoints, targetPort int32) (string, int32) {
	for _, subset := range ep.Subsets {
		portMatched := false
		for _, p := range subset.Ports {
			if p.Port == targetPort {
				portMatched = true
				break
			}
		}
		if !portMatched {
			continue
		}
		for _, addr := range subset.Addresses {
			if addr.TargetRef != nil && addr.TargetRef.Kind == "Pod" && addr.TargetRef.Name != "" {
				return addr.TargetRef.Name, targetPort
			}
		}
		for _, addr := range subset.NotReadyAddresses {
			if addr.TargetRef != nil && addr.TargetRef.Kind == "Pod" && addr.TargetRef.Name != "" {
				return addr.TargetRef.Name, targetPort
			}
		}
	}
	return "", 0
}

func startPodPortForward(cfg *rest.Config, namespace, podName string, remotePort int32) (uint16, chan struct{}, error) {
	transport, upgrader, err := spdy.RoundTripperFor(cfg)
	if err != nil {
		return 0, nil, err
	}
	serverURL := cfg.Host + fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", namespace, podName)
	url, err := url.Parse(serverURL)
	if err != nil {
		return 0, nil, err
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, url)
	stopChan := make(chan struct{}, 1)
	readyChan := make(chan struct{})
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	forwarder, err := portforward.New(dialer, []string{fmt.Sprintf("0:%d", remotePort)}, stopChan, readyChan, out, errOut)
	if err != nil {
		return 0, nil, err
	}
	go func() {
		_ = forwarder.ForwardPorts()
	}()
	select {
	case <-readyChan:
	case <-time.After(10 * time.Second):
		close(stopChan)
		if errOut.Len() > 0 {
			return 0, nil, fmt.Errorf("%s", strings.TrimSpace(errOut.String()))
		}
		return 0, nil, fmt.Errorf("timed out waiting for pod port-forward")
	}
	ports, err := forwarder.GetPorts()
	if err != nil {
		close(stopChan)
		return 0, nil, err
	}
	if len(ports) == 0 {
		close(stopChan)
		return 0, nil, fmt.Errorf("port-forward did not allocate a local port")
	}
	return ports[0].Local, stopChan, nil
}

func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}
