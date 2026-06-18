package server

import (
	"fmt"
	"net"
	"strings"

	"github.com/beego/beego"
)

// Config holds control-plane settings populated from app.conf.
type Config struct {
	DataDir                 string
	ApiserverBind           string // actual bind / SAN IP (may be loopback in dev)
	AdvertiseAddress        string // non-loopback IP registered as kubernetes service endpoint
	ApiserverPort           int
	PublicOrigin            string // external CasOS origin behind reverse proxy, optional
	DSN                     string // MySQL DSN forwarded to kine
	SandboxImage            string // containerd sandbox (pause) image, empty = upstream default
	Socks5Proxy             string // outbound socks5 proxy, e.g. 127.0.0.1:10808
	PodUIProxyBind          string // fixed Pod UI proxy listen address
	PodUIProxyPublicBaseURL string // public Pod UI base URL template, must include {id} in host
}

// ConfigFromAppConf reads server config from the beego app.conf.
func ConfigFromAppConf() (Config, error) {
	dataDir := beego.AppConfig.String("dataDir")
	if dataDir == "" {
		dataDir = "/var/lib/casos"
	}
	bind := beego.AppConfig.String("apiserverBind")
	if bind == "" {
		bind = outboundIP()
	}
	port, _ := beego.AppConfig.Int("apiserverPort")
	if port == 0 {
		port = 6443
	}
	publicOrigin := strings.TrimSpace(beego.AppConfig.String("publicOrigin"))
	dsn := beego.AppConfig.String("dataSourceName")
	if dsn == "" {
		return Config{}, fmt.Errorf("dataSourceName not set in app.conf")
	}
	dbName := beego.AppConfig.String("dbName")
	if dbName == "" {
		dbName = "casos"
	}
	dsn = injectDBName(dsn, dbName)

	advertise := outboundIP()
	if advertise == "127.0.0.1" || advertise == "::1" {
		advertise = bind
	}

	socks5Proxy := beego.AppConfig.String("socks5Proxy")
	podUIProxyBind := strings.TrimSpace(beego.AppConfig.String("podUIProxyBind"))
	if podUIProxyBind == "" {
		podUIProxyBind = "127.0.0.1:9001"
	}
	podUIProxyPublicBaseURL := strings.TrimSpace(beego.AppConfig.String("podUIProxyPublicBaseUrl"))

	sandboxImage := beego.AppConfig.String("sandboxImage")
	if sandboxImage == "" {
		if socks5Proxy != "" {
			sandboxImage = "registry.aliyuncs.com/google_containers/pause:3.10.1"
		} else {
			sandboxImage = "registry.k8s.io/pause:3.10.1"
		}
	}

	return Config{
		DataDir:                 dataDir,
		ApiserverBind:           bind,
		AdvertiseAddress:        advertise,
		ApiserverPort:           port,
		PublicOrigin:            publicOrigin,
		DSN:                     dsn,
		SandboxImage:            sandboxImage,
		Socks5Proxy:             socks5Proxy,
		PodUIProxyBind:          podUIProxyBind,
		PodUIProxyPublicBaseURL: podUIProxyPublicBaseURL,
	}, nil
}

// injectDBName inserts dbName into a MySQL DSN of the form
// user:pass@tcp(host:port)/ (trailing slash, no database).
// If a database is already present it is replaced.
func injectDBName(dsn, dbName string) string {
	idx := strings.LastIndex(dsn, "/")
	if idx < 0 {
		return dsn + dbName
	}
	base := dsn[:idx+1]
	rest := dsn[idx+1:]
	if q := strings.Index(rest, "?"); q >= 0 {
		return base + dbName + rest[q:]
	}
	return base + dbName
}

// outboundIP returns the preferred non-loopback outbound IP of this machine.
func outboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}
