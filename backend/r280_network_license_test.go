//go:build license

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestR280ProxyBootstrapKeepsLocalAuthorization(t *testing.T) {
	t.Setenv("DEEP_LEGENDS_SYSTEM_PROXY", "")
	p := newChampionProvider()
	defer p.client.CloseIdleConnections()
	a := &app{token: "fixture", champions: p}
	handler := a.withLicenseProtection(a.authorized(a.handleSystemProxyTargets))
	request := httptest.NewRequest("GET", "/api/system-proxy", nil)
	blocked := httptest.NewRecorder()
	handler.ServeHTTP(blocked, request)
	if blocked.Code == 200 {
		t.Fatal("proxy bootstrap lost local token authorization")
	}
	request.Header.Set("X-Local-Token", "fixture")
	allowed := httptest.NewRecorder()
	handler.ServeHTTP(allowed, request)
	if allowed.Code != http.StatusOK {
		t.Fatal("inactive license prevents configuring its network", allowed.Code)
	}
	request = httptest.NewRequest("GET", "/api/gameplay/overview", nil)
	request.Header.Set("X-Local-Token", "fixture")
	business := httptest.NewRecorder()
	handler.ServeHTTP(business, request)
	if business.Code != http.StatusForbidden {
		t.Fatal("proxy bootstrap exposed unrelated business route")
	}
}
