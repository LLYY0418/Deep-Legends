package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSGPValidatedGateway(t *testing.T) {
	for _, value := range []string{"http://hn10-k8s-sgp.lol.qq.com:21019", "https://example.com", "https://hn10-k8s-sgp.lol.qq.com.evil.com", "https://user:pass@hn10-k8s-sgp.lol.qq.com", "https://hn10-k8s-sgp.lol.qq.com/path", "https://hn10-k8s-sgp.lol.qq.com?", "https://hn10-k8s-sgp.lol.qq.com:80", "https://127.0.0.1:21019", "https://hn10-k8s-sgp.lol.qq.com#fragment"} {
		if _, ok := validatedSGPBase(value); ok {
			t.Fatal("unsafe gateway accepted", value)
		}
	}
	for _, value := range []string{"https://hn10-k8s-sgp.lol.qq.com:21019", "https://new99-sgp.lol.qq.com/"} {
		if _, ok := validatedSGPBase(value); !ok {
			t.Fatal("official gateway rejected")
		}
	}
	p := newSGPProvider()
	account := "account-one"
	reads := 0
	p.gatewayAccount = func(*LCUClient) string { return account }
	c := &LCUClient{region: "TENCENT", rsoPlatform: "HN99", baseURL: "https://127.0.0.1:1", token: "secret", http: &http.Client{Transport: sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		reads++
		if r.URL.Path == sgpEndpointConfig {
			return response2351("https://new99-sgp.lol.qq.com:21019"), nil
		}
		if r.URL.Path != sgpPlatformConfig {
			t.Fatal("wrong platform config path")
		}
		return response2351("HN99"), nil
	})}}
	server, base, ok := p.available(c)
	if !ok || server != "HN99" || base != "https://new99-sgp.lol.qq.com:21019" {
		t.Fatal("unknown official shard silently disabled")
	}
	p.available(c)
	if reads != 2 {
		t.Fatal("gateway not cached")
	}
	account = "account-two"
	p.available(c)
	if reads != 4 {
		t.Fatal("gateway survived account switch")
	}
	if other, _ := p.serverBaseOn(context.Background(), c, "HN10"); other == base {
		t.Fatal("cross-server route used local shard")
	}
	account = "account-three"
	c.http.Transport = sgpRoundTripFunc(func(*http.Request) (*http.Response, error) {
		account = "account-four"
		return response2351("https://new99-sgp.lol.qq.com:21019"), nil
	})
	if g := p.discoverGateway(context.Background(), c, "account-three"); g.Source != "connection-changed" {
		t.Fatal("in-flight account change cached")
	}
}

func TestSGPGatewayFallbackAndNoLCURedirect(t *testing.T) {
	p := newSGPProvider()
	calls := 0
	c := &LCUClient{region: "TENCENT", rsoPlatform: "HN10", baseURL: "https://127.0.0.1:1", token: "secret", http: &http.Client{Transport: sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://evil.example"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}}
	g := p.discoverGateway(context.Background(), c, "account")
	if calls != 2 || g.Source != "builtin-fallback" || g.Base != tencentSGPServers["HN10"] {
		t.Fatal("fallback/redirect safety")
	}
}
