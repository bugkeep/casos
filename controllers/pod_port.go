package controllers

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/casosorg/casos/object"
	"github.com/casosorg/casos/server"
	corev1 "k8s.io/api/core/v1"
)

const (
	podUIForwardTTL          = time.Hour
	maxPodUIForwards         = 64
	podUIProxyAuthCookieName = "casos_pod_ui_auth"
	podUIProxyIDPlaceholder  = "{id}"
)

type openPodUIRequest struct {
	Namespace     string `json:"namespace"`
	Name          string `json:"name"`
	ContainerPort int32  `json:"containerPort"`
}

type openPodUIResponse struct {
	LocalPort int    `json:"localPort"`
	ProxyPort int    `json:"proxyPort"`
	URL       string `json:"url"`
}

type podUIForward struct {
	key           string
	sessionID     string
	namespace     string
	name          string
	containerPort int32
	localPort     int
	proxyPort     int
	publicURL     string
	publicHost    string
	publicScheme  string
	token         string
	stop          func()
	timer         *time.Timer
}

var (
	podUIForwards        = map[string]*podUIForward{}
	podUIForwardsByHost  = map[string]*podUIForward{}
	podUIForwardsMu      sync.Mutex
	podUIProxyServerOnce sync.Once
	podUIProxyServerErr  error
	podUIProxyServerPort int
)

func podUIForwardKey(namespace, name string, containerPort int32) string {
	return fmt.Sprintf("%s/%s:%d", namespace, name, containerPort)
}

func hasContainerPort(ports []int32, port int32) bool {
	for _, p := range ports {
		if p == port {
			return true
		}
	}
	return false
}

func localPodUIForwardAlive(localPort int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", localPort), 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func randomPodUIToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func randomPodUISessionID() (string, error) {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)), nil
}

func setPodUIProxyCookie(w http.ResponseWriter, entry *podUIForward) {
	http.SetCookie(w, &http.Cookie{
		Name:     podUIProxyAuthCookieName,
		Value:    entry.token,
		Path:     "/",
		MaxAge:   int(podUIForwardTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   strings.EqualFold(entry.publicScheme, "https"),
	})
}

func cleanTokenURL(r *http.Request) string {
	cleanURL := *r.URL
	q := cleanURL.Query()
	q.Del("token")
	cleanURL.RawQuery = q.Encode()
	return cleanURL.String()
}

func canonicalHost(host string) string {
	return strings.ToLower(strings.TrimSpace(host))
}

func loopbackLikeHost(host string) bool {
	host = strings.ToLower(strings.Trim(strings.TrimSpace(host), "[]"))
	if host == "" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return host == "localhost" ||
		strings.HasSuffix(host, ".localhost") ||
		host == "localtest.me" ||
		strings.HasSuffix(host, ".localtest.me") ||
		host == "lvh.me" ||
		strings.HasSuffix(host, ".lvh.me") ||
		host == "127.0.0.1.nip.io" ||
		strings.HasSuffix(host, ".127.0.0.1.nip.io")
}

func portFromURL(u *url.URL) int {
	if p := u.Port(); p != "" {
		port, _ := strconv.Atoi(p)
		return port
	}
	if strings.EqualFold(u.Scheme, "https") {
		return 443
	}
	if strings.EqualFold(u.Scheme, "http") {
		return 80
	}
	return 0
}

func buildPodUIProxyPublicURLFromTemplate(template, sessionID, token string) (*url.URL, error) {
	template = strings.TrimSpace(template)
	if template == "" {
		return nil, fmt.Errorf("podUIProxyPublicBaseUrl is required")
	}
	if !strings.Contains(template, podUIProxyIDPlaceholder) {
		return nil, fmt.Errorf("podUIProxyPublicBaseUrl must include %q in host", podUIProxyIDPlaceholder)
	}

	placeholderURL, err := url.Parse(strings.ReplaceAll(template, podUIProxyIDPlaceholder, "casospoduiid"))
	if err != nil {
		return nil, fmt.Errorf("invalid podUIProxyPublicBaseUrl: %w", err)
	}
	if placeholderURL.Scheme == "" || placeholderURL.Host == "" {
		return nil, fmt.Errorf("podUIProxyPublicBaseUrl must be an absolute URL")
	}
	if !strings.Contains(strings.ToLower(placeholderURL.Hostname()), "casospoduiid") {
		return nil, fmt.Errorf("podUIProxyPublicBaseUrl must include %q in host", podUIProxyIDPlaceholder)
	}
	if placeholderURL.Path != "" && placeholderURL.Path != "/" {
		return nil, fmt.Errorf("podUIProxyPublicBaseUrl must not include a path")
	}
	if placeholderURL.RawQuery != "" || placeholderURL.Fragment != "" {
		return nil, fmt.Errorf("podUIProxyPublicBaseUrl must not include query or fragment")
	}
	if !strings.EqualFold(placeholderURL.Scheme, "https") && !loopbackLikeHost(placeholderURL.Hostname()) {
		return nil, fmt.Errorf("podUIProxyPublicBaseUrl must use https outside local development")
	}

	rawURL := strings.ReplaceAll(template, podUIProxyIDPlaceholder, sessionID)
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid podUIProxyPublicBaseUrl: %w", err)
	}
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()
	return u, nil
}

func podUIProxyURL(entry *podUIForward) string {
	return entry.publicURL
}

func ensurePodUIProxyServer() error {
	cfg := getServerConfig()
	if cfg == nil {
		return fmt.Errorf("server config not ready")
	}

	podUIProxyServerOnce.Do(func() {
		listener, err := net.Listen("tcp", cfg.PodUIProxyBind)
		if err != nil {
			podUIProxyServerErr = err
			return
		}
		podUIProxyServerPort = listener.Addr().(*net.TCPAddr).Port
		server := &http.Server{
			Handler:           http.HandlerFunc(servePodUIProxy),
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       30 * time.Second,
			IdleTimeout:       60 * time.Second,
			MaxHeaderBytes:    1 << 20,
		}
		go func() {
			_ = server.Serve(listener)
		}()
	})

	return podUIProxyServerErr
}

func expirePodUIForward(key string, entry *podUIForward) {
	var stop func()
	podUIForwardsMu.Lock()
	if current, ok := podUIForwards[key]; ok && current == entry {
		stop = current.stop
		delete(podUIForwards, key)
		delete(podUIForwardsByHost, current.publicHost)
	}
	podUIForwardsMu.Unlock()
	if stop != nil {
		stop()
	}
}

func removePodUIForward(key string, entry *podUIForward) {
	var stop func()
	podUIForwardsMu.Lock()
	if current, ok := podUIForwards[key]; ok && current == entry {
		if current.timer != nil {
			current.timer.Stop()
		}
		stop = current.stop
		delete(podUIForwards, key)
		delete(podUIForwardsByHost, current.publicHost)
	}
	podUIForwardsMu.Unlock()
	if stop != nil {
		stop()
	}
}

func cachedPodUIForward(key string) (*podUIForward, bool) {
	podUIForwardsMu.Lock()
	entry, ok := podUIForwards[key]
	podUIForwardsMu.Unlock()
	if !ok {
		return nil, false
	}
	if !localPodUIForwardAlive(entry.localPort) {
		removePodUIForward(key, entry)
		return nil, false
	}

	podUIForwardsMu.Lock()
	if current, ok := podUIForwards[key]; ok && current == entry {
		if current.timer != nil {
			current.timer.Reset(podUIForwardTTL)
		}
		podUIForwardsMu.Unlock()
		return current, true
	}
	podUIForwardsMu.Unlock()
	return nil, false
}

func storePodUIForward(key string, entry *podUIForward) (*podUIForward, bool, error) {
	podUIForwardsMu.Lock()
	if current, ok := podUIForwards[key]; ok {
		if current.timer != nil {
			current.timer.Reset(podUIForwardTTL)
		}
		podUIForwardsMu.Unlock()
		return current, false, nil
	} else if len(podUIForwards) >= maxPodUIForwards {
		podUIForwardsMu.Unlock()
		return nil, false, fmt.Errorf("too many pod UI port-forwards are open")
	} else if _, exists := podUIForwardsByHost[entry.publicHost]; exists {
		podUIForwardsMu.Unlock()
		return nil, false, fmt.Errorf("pod UI session host already exists")
	}

	entry.timer = time.AfterFunc(podUIForwardTTL, func() {
		expirePodUIForward(key, entry)
	})
	podUIForwards[key] = entry
	podUIForwardsByHost[entry.publicHost] = entry
	podUIForwardsMu.Unlock()
	return entry, true, nil
}

func createPodUIForward(cfg *server.Config, key, namespace, name string, containerPort int32) (*podUIForward, error) {
	adminCfg := getAdminRestConfig()
	localPort, stop, err := object.OpenPodUI(adminCfg, namespace, name, containerPort)
	if err != nil {
		return nil, err
	}
	token, err := randomPodUIToken()
	if err != nil {
		stop()
		return nil, err
	}

	var lastErr error
	template := strings.TrimSpace(cfg.PodUIProxyPublicBaseURL)
	if template == "" {
		return nil, fmt.Errorf("podUIProxyPublicBaseUrl is required")
	}

	for attempts := 0; attempts < 5; attempts++ {
		sessionID, err := randomPodUISessionID()
		if err != nil {
			stop()
			return nil, err
		}
		publicURL, err := buildPodUIProxyPublicURLFromTemplate(template, sessionID, token)
		if err != nil {
			stop()
			return nil, err
		}
		entry := &podUIForward{
			key:           key,
			sessionID:     sessionID,
			namespace:     namespace,
			name:          name,
			containerPort: containerPort,
			localPort:     localPort,
			proxyPort:     portFromURL(publicURL),
			publicURL:     publicURL.String(),
			publicHost:    canonicalHost(publicURL.Host),
			publicScheme:  publicURL.Scheme,
			token:         token,
			stop:          stop,
		}
		return entry, nil
	}

	stop()
	if lastErr == nil {
		lastErr = fmt.Errorf("failed to allocate pod UI session host")
	}
	return nil, lastErr
}

func ensurePodUIForward(namespace, name string, containerPort int32) (*podUIForward, error) {
	cfg := getServerConfig()
	if cfg == nil {
		return nil, fmt.Errorf("server config not ready")
	}
	if getAdminRestConfig() == nil {
		return nil, fmt.Errorf("apiserver not ready")
	}
	if err := validatePodUIAccess(namespace, name, containerPort); err != nil {
		return nil, err
	}
	if err := ensurePodUIProxyServer(); err != nil {
		return nil, fmt.Errorf("start pod ui proxy server: %w", err)
	}

	key := podUIForwardKey(namespace, name, containerPort)
	if entry, ok := cachedPodUIForward(key); ok {
		return entry, nil
	}

	entry, err := createPodUIForward(cfg, key, namespace, name, containerPort)
	if err != nil {
		return nil, err
	}
	stored, storedNew, err := storePodUIForward(key, entry)
	if err != nil {
		entry.stop()
		return nil, err
	}
	if !storedNew {
		entry.stop()
	}
	return stored, nil
}

func validatePodUIAccess(namespace, name string, containerPort int32) error {
	if namespace == "" || name == "" || containerPort < 1 || containerPort > 65535 {
		return fmt.Errorf("namespace, name and containerPort between 1 and 65535 are required")
	}
	pod, err := object.GetPod(getAdminRestConfig(), namespace, name)
	if err != nil {
		return err
	}
	if pod.Status.Phase != corev1.PodRunning {
		return fmt.Errorf("pod is not running")
	}
	if !hasContainerPort(object.ContainerPortsFromPod(pod), containerPort) {
		return fmt.Errorf("containerPort is not declared by this pod")
	}
	return nil
}

func shouldRewritePodUILocationHost(host string, entry *podUIForward) bool {
	host = strings.ToLower(strings.Trim(strings.TrimSpace(host), "[]"))
	if host == "" {
		return false
	}
	if loopbackLikeHost(host) {
		return true
	}
	return host == strings.ToLower(entry.name) ||
		host == strings.ToLower(entry.name+"."+entry.namespace) ||
		host == strings.ToLower(entry.name+"."+entry.namespace+".svc")
}

func rewritePodUILocation(location string, entry *podUIForward) string {
	locURL, err := url.Parse(location)
	if err != nil || !locURL.IsAbs() {
		return location
	}
	if !shouldRewritePodUILocationHost(locURL.Hostname(), entry) {
		return location
	}
	locURL.Scheme = entry.publicScheme
	locURL.Host = entry.publicHost
	return locURL.String()
}

func sanitizePodUISetCookies(res *http.Response, entry *podUIForward) {
	cookies := res.Cookies()
	if len(cookies) == 0 {
		return
	}

	res.Header.Del("Set-Cookie")
	for _, cookie := range cookies {
		if cookie == nil || cookie.Name == "" {
			continue
		}
		cookie.Domain = ""
		if cookie.Path == "" {
			cookie.Path = "/"
		}
		if strings.EqualFold(entry.publicScheme, "https") {
			cookie.Secure = true
		}
		res.Header.Add("Set-Cookie", cookie.String())
	}
}

func newPodUIEntryHandler(entry *podUIForward) http.Handler {
	target := &url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", entry.localPort)}
	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host
		req.Header.Del("Authorization")
		req.Header.Del("Proxy-Authorization")
		req.Header.Del("X-CSRF-Token")
		req.Header.Del("X-XSRF-Token")
		req.Header.Del("Forwarded")
		req.Header.Del("X-Forwarded-For")
		req.Header.Del("X-Forwarded-Host")
		req.Header.Del("X-Forwarded-Proto")
		if req.Header.Get("Origin") != "" {
			req.Header.Set("Origin", target.String())
		}
		req.Header.Set("X-Forwarded-Host", entry.publicHost)
		req.Header.Set("X-Forwarded-Proto", entry.publicScheme)
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "pod UI unavailable", http.StatusBadGateway)
	}
	proxy.ModifyResponse = func(res *http.Response) error {
		res.Header.Set("X-Content-Type-Options", "nosniff")
		sanitizePodUISetCookies(res, entry)
		if location := res.Header.Get("Location"); location != "" {
			res.Header.Set("Location", rewritePodUILocation(location, entry))
		}
		return nil
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !localPodUIForwardAlive(entry.localPort) {
			removePodUIForward(entry.key, entry)
			http.Error(w, "pod UI unavailable", http.StatusBadGateway)
			return
		}

		if tokenFromURL := r.URL.Query().Get("token"); tokenFromURL != "" {
			if subtle.ConstantTimeCompare([]byte(tokenFromURL), []byte(entry.token)) != 1 {
				http.Error(w, "pod UI unauthorized", http.StatusForbidden)
				return
			}
			setPodUIProxyCookie(w, entry)
			http.Redirect(w, r, cleanTokenURL(r), http.StatusFound)
			return
		}

		cookie, err := r.Cookie(podUIProxyAuthCookieName)
		if err != nil || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(entry.token)) != 1 {
			http.Error(w, "pod UI unauthorized", http.StatusForbidden)
			return
		}

		setPodUIProxyCookie(w, entry)
		proxy.ServeHTTP(w, r)
	})
}

func servePodUIProxy(w http.ResponseWriter, r *http.Request) {
	host := canonicalHost(r.Host)

	podUIForwardsMu.Lock()
	entry := podUIForwardsByHost[host]
	podUIForwardsMu.Unlock()
	if entry == nil {
		http.NotFound(w, r)
		return
	}

	newPodUIEntryHandler(entry).ServeHTTP(w, r)
}

// OpenPodUI opens a localhost port-forward to a running Pod's own web UI.
// @router /api/open-pod-ui [post]
func (c *ApiController) OpenPodUI() {
	if c.RequireSignedIn() {
		return
	}

	var req openPodUIRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	entry, err := ensurePodUIForward(req.Namespace, req.Name, req.ContainerPort)
	if err != nil {
		c.ResponseError("start port-forward: " + err.Error())
		return
	}

	c.ResponseOk(openPodUIResponse{
		LocalPort: entry.localPort,
		ProxyPort: entry.proxyPort,
		URL:       podUIProxyURL(entry),
	})
}
