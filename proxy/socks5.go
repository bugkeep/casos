package proxy

import (
	"net/http"
	"net/url"

	"github.com/beego/beego/logs"
	"github.com/casosorg/casos/conf"
	"golang.org/x/net/http/httpproxy"
)

var (
	DefaultHttpClient = http.DefaultClient
	ProxyHttpClient   = http.DefaultClient
)

func InitHttpClient() {
	policy, err := NewEgressPolicy(GetSocks5ProxyAddress(), httpproxy.FromEnvironment().NoProxy)
	if err != nil {
		logs.Error("Control-plane egress blocked by invalid proxy configuration: %v", err)
		policy = &EgressPolicy{
			proxyForURL: func(*url.URL) (*url.URL, error) {
				return nil, err
			},
		}
	}
	client := policy.HTTPClient()
	DefaultHttpClient = client
	ProxyHttpClient = client
}

func GetSocks5ProxyAddress() string {
	return conf.GetConfigString("socks5Proxy")
}

func HTTPClient() *http.Client {
	return ProxyHttpClient
}

// Deprecated: use HTTPClient. The URL parameter never affected client selection.
func GetHttpClient(_ string) *http.Client {
	return HTTPClient()
}
