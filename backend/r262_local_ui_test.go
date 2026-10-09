package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestR262LocalUIHostGuard(t *testing.T) {
	listener, err := listenLocalUI("127.0.0.1:0", false)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	otherPort := port%65535 + 1
	a := &app{token: "r262-session-secret"}
	webFS, err := fs.Sub(embedded, "web")
	if err != nil {
		t.Fatal(err)
	}
	staticFiles := newStaticAssetHandler(webFS)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", a.handleBootstrap(staticFiles))
	mux.Handle("GET /", staticFiles)
	mux.HandleFunc("GET /api/status", a.authorized(a.handleStatus))
	mux.HandleFunc("GET /api/image", a.authorized(a.handleImage))
	handler := a.localUIHandler(listener, mux)

	paths := []struct {
		path   string
		status int
		cookie bool
	}{
		{"/", http.StatusOK, true},
		{"/?bootstrap=" + a.token, http.StatusSeeOther, true},
		{"/api/status", http.StatusOK, false},
		{"/app.js", http.StatusOK, false},
		{"/api/image", http.StatusBadRequest, false},
	}
	for _, host := range []string{
		"evil.example:47391", "evil.example:" + strconv.Itoa(port),
		"127.0.0.1:" + strconv.Itoa(otherPort), "localhost:" + strconv.Itoa(otherPort),
		"127.0.0.1", "localhost", "[::1]:" + strconv.Itoa(port),
	} {
		for _, route := range paths {
			t.Run("reject/"+host+route.path, func(t *testing.T) {
				request := httptest.NewRequest(http.MethodGet, route.path, nil)
				request.Host = host
				request.Header.Set("Sec-Fetch-Site", "same-origin")
				request.Header.Set("Sec-Fetch-Mode", "navigate")
				request.Header.Set("Sec-Fetch-Dest", "document")
				// Only the API/static requests carry the valid session. Navigation
				// deliberately has no cookie, reproducing the rebinding issuance.
				if !route.cookie {
					request.AddCookie(&http.Cookie{Name: "lol_loot_token", Value: a.token})
				}
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, request)
				if recorder.Code != http.StatusForbidden || recorder.Body.String() != "forbidden\n" {
					t.Fatalf("Host %q: status=%d body-prefix=%q", host, recorder.Code, recorder.Body.String()[:min(recorder.Body.Len(), 120)])
				}
				if cookies := recorder.Header().Values("Set-Cookie"); len(cookies) != 0 {
					t.Fatalf("rejected Host received Set-Cookie: %v", cookies)
				}
				if recorder.Header().Get("Content-Security-Policy") == "" || recorder.Header().Get("X-Content-Type-Options") != "nosniff" || recorder.Header().Get("Cache-Control") != "no-store" || recorder.Header().Get("Referrer-Policy") != "no-referrer" {
					t.Fatalf("rejected Host lost security headers: %v", recorder.Header())
				}
			})
		}
	}
	for _, host := range []string{"127.0.0.1:" + strconv.Itoa(port), "localhost:" + strconv.Itoa(port)} {
		for _, route := range paths {
			t.Run("allow/"+host+route.path, func(t *testing.T) {
				request := httptest.NewRequest(http.MethodGet, route.path, nil)
				request.Host = host
				request.Header.Set("Sec-Fetch-Site", "same-origin")
				request.Header.Set("Sec-Fetch-Mode", "navigate")
				request.Header.Set("Sec-Fetch-Dest", "document")
				if !route.cookie {
					request.AddCookie(&http.Cookie{Name: "lol_loot_token", Value: a.token})
				}
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, request)
				if recorder.Code != route.status {
					t.Fatalf("allowed Host %q: status=%d want=%d body=%q", host, recorder.Code, route.status, recorder.Body.String())
				}
				cookies := recorder.Result().Cookies()
				if route.cookie && (len(cookies) != 1 || cookies[0].Value != a.token || !cookies[0].HttpOnly) {
					t.Fatalf("allowed navigation lost session cookie: %#v", cookies)
				}
				if !route.cookie && len(cookies) != 0 {
					t.Fatalf("non-navigation received a cookie: %#v", cookies)
				}
			})
		}
	}
}

func TestR262ListenLocalUI(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	address := occupied.Addr().String()
	t.Run("busy-without-fallback", func(t *testing.T) {
		listener, err := listenLocalUI(address, false)
		if listener != nil {
			listener.Close()
		}
		if err == nil || listener != nil {
			t.Fatalf("busy port without fallback: listener=%v err=%v", listener, err)
		}
	})
	t.Run("busy-with-fallback", func(t *testing.T) {
		listener, err := listenLocalUI(address, true)
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
		actual := listener.Addr().(*net.TCPAddr)
		if listener.Addr().String() == address || !actual.IP.IsLoopback() || actual.Port == 0 {
			t.Fatalf("fallback did not choose another loopback port: %v", actual)
		}
		var ready bytes.Buffer
		base := "http://" + listener.Addr().String()
		if err := writeDesktopReady(&ready, base, base+"/?bootstrap=secret", "secret"); err != nil {
			t.Fatal(err)
		}
		var payload struct {
			BaseURL      string `json:"baseUrl"`
			BootstrapURL string `json:"bootstrapUrl"`
		}
		if err := json.Unmarshal(bytes.TrimPrefix(ready.Bytes(), []byte("LOOT_READY ")), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.BaseURL != base || !strings.HasPrefix(payload.BootstrapURL, base+"/") {
			t.Fatalf("desktop ready did not use fallback address: %#v", payload)
		}
		handler := localUIHostGuard(listener, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
		for host, want := range map[string]int{listener.Addr().String(): http.StatusNoContent, address: http.StatusForbidden, "localhost:" + strconv.Itoa(actual.Port): http.StatusNoContent} {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Host = host
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)
			if recorder.Code != want {
				t.Fatalf("fallback Host %q: status=%d want=%d", host, recorder.Code, want)
			}
		}
	})
	t.Run("free-fixed-port", func(t *testing.T) {
		free, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		requested := free.Addr().String()
		if err := free.Close(); err != nil {
			t.Fatal(err)
		}
		for _, fallback := range []bool{false, true} {
			listener, err := listenLocalUI(requested, fallback)
			if err != nil {
				t.Fatal(err)
			}
			actual := listener.Addr().String()
			listener.Close()
			if actual != requested {
				t.Fatalf("free port fallback=%v: got=%s want=%s", fallback, actual, requested)
			}
		}
	})
}
