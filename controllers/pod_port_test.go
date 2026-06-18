package controllers

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestBuildPodUIProxyPublicURLFromTemplate(t *testing.T) {
	u, err := buildPodUIProxyPublicURLFromTemplate("https://{id}.pod-ui.example.com", "abc123", "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := u.String(), "https://abc123.pod-ui.example.com?token=tok"; got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
}

func TestBuildPodUIProxyPublicURLRejectsNonHTTPSNonLoopback(t *testing.T) {
	if _, err := buildPodUIProxyPublicURLFromTemplate("http://{id}.pod-ui.example.com:9001", "abc123", "tok"); err == nil {
		t.Fatal("expected error for non-https non-loopback base url")
	}
}

func TestBuildPodUIProxyPublicURLAllowsLoopbackHTTP(t *testing.T) {
	u, err := buildPodUIProxyPublicURLFromTemplate("http://{id}.127.0.0.1.nip.io:9001", "abc123", "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := u.Host, "abc123.127.0.0.1.nip.io:9001"; got != want {
		t.Fatalf("host = %q, want %q", got, want)
	}
}

func TestRewritePodUILocation(t *testing.T) {
	entry := &podUIForward{
		name:         "casdoor-all-in-one",
		namespace:    "default",
		publicScheme: "https",
		publicHost:   "abc123.pod-ui.example.com",
	}

	got := rewritePodUILocation("http://127.0.0.1:8000/login", entry)
	want := "https://abc123.pod-ui.example.com/login"
	if got != want {
		t.Fatalf("rewritten location = %q, want %q", got, want)
	}

	unchanged := rewritePodUILocation("https://example.org/login", entry)
	if unchanged != "https://example.org/login" {
		t.Fatalf("unexpected rewrite: %q", unchanged)
	}
}

func TestSanitizePodUISetCookies(t *testing.T) {
	res := &http.Response{Header: make(http.Header)}
	res.Header.Add("Set-Cookie", "session=abc; Path=/; Domain=.example.com; HttpOnly")
	res.Header.Add("Set-Cookie", "theme=dark; Path=/")
	entry := &podUIForward{publicScheme: "https"}

	sanitizePodUISetCookies(res, entry)
	got := res.Header.Values("Set-Cookie")
	if len(got) != 2 {
		t.Fatalf("set-cookie count = %d, want 2", len(got))
	}
	for _, cookieLine := range got {
		if strings.Contains(strings.ToLower(cookieLine), "domain=") {
			t.Fatalf("unexpected domain in cookie: %q", cookieLine)
		}
		if !strings.Contains(strings.ToLower(cookieLine), "secure") {
			t.Fatalf("expected secure cookie: %q", cookieLine)
		}
	}
}

func TestLoopbackLikeHost(t *testing.T) {
	cases := map[string]bool{
		"localhost":            true,
		"demo.localhost":       true,
		"127.0.0.1":            true,
		"abc.127.0.0.1.nip.io": true,
		"abc.localtest.me":     true,
		"pod-ui.example.com":   false,
		"192.168.1.10":         false,
	}
	for host, want := range cases {
		if got := loopbackLikeHost(host); got != want {
			t.Fatalf("loopbackLikeHost(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestCanonicalHostPreservesPort(t *testing.T) {
	if got := canonicalHost(" AbC.Example.Com:9001 "); got != "abc.example.com:9001" {
		t.Fatalf("canonicalHost = %q", got)
	}
}

func TestPortFromURL(t *testing.T) {
	u, _ := url.Parse("https://example.com")
	if got := portFromURL(u); got != 443 {
		t.Fatalf("port = %d, want 443", got)
	}
}
