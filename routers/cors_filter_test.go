package routers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/beego/beego"
	"github.com/beego/beego/context"
)

func TestRequestOriginIgnoresForwardedHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://casos.example.com/api/get-pods", nil)
	req.Host = "casos.example.com"
	req.Header.Set("X-Forwarded-Host", "attacker.example.com")
	req.Header.Set("X-Forwarded-Proto", "https")

	ctx := context.NewContext()
	ctx.Reset(httptest.NewRecorder(), req)

	if got, want := requestOrigin(ctx), "http://casos.example.com"; got != want {
		t.Fatalf("requestOrigin = %q, want %q", got, want)
	}
}

func TestRequestOriginUsesConfiguredPublicOrigin(t *testing.T) {
	beego.AppConfig.Set("publicOrigin", "https://casos.example.com")
	defer beego.AppConfig.Set("publicOrigin", "")

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:9000/api/get-pods", nil)
	req.Host = "127.0.0.1:9000"

	ctx := context.NewContext()
	ctx.Reset(httptest.NewRecorder(), req)

	if got, want := requestOrigin(ctx), "https://casos.example.com"; got != want {
		t.Fatalf("requestOrigin = %q, want %q", got, want)
	}
}

func TestAllowedCORSOriginRejectsSpoofedForwardedHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://casos.example.com/api/get-pods", nil)
	req.Host = "casos.example.com"
	req.Header.Set("X-Forwarded-Host", "attacker.example.com")

	ctx := context.NewContext()
	ctx.Reset(httptest.NewRecorder(), req)

	if isAllowedCORSOrigin(ctx, "http://attacker.example.com") {
		t.Fatal("expected spoofed forwarded host to be rejected")
	}
	if !isAllowedCORSOrigin(ctx, "http://casos.example.com") {
		t.Fatal("expected request host origin to be allowed")
	}
}
