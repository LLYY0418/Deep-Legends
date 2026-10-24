package main

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR280SOCKSHandshakeFallback(t *testing.T) {
	for _, scheme := range []string{"socks5", "socks5h"} {
		for _, mode := range []string{"auto", "manual"} {
			t.Run(scheme+"/"+mode, func(t *testing.T) {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer listener.Close()
				go func() {
					conn, err := listener.Accept()
					if err != nil {
						return
					}
					defer conn.Close()
					conn.SetDeadline(time.Now().Add(time.Second))
					greeting := make([]byte, 3)
					if _, err = io.ReadFull(conn, greeting); err == nil {
						conn.Write([]byte{5, 255})
					}
				}()
				address := scheme + "://" + listener.Addr().String()
				t.Setenv("DEEP_LEGENDS_SYSTEM_PROXY", address)
				p := newChampionProvider()
				if mode == "manual" {
					if err := p.setNetworkSettings(championNetworkSettings{Mode: mode, URL: address}); err != nil {
						t.Fatal(err)
					}
				}
				defer p.client.CloseIdleConnections()
				route := p.client.Transport.(*automaticNetworkTransport)
				route.probes = map[string]string{}
				var direct atomic.Int32
				route.direct = r196RoundTrip(func(*http.Request) (*http.Response, error) {
					direct.Add(1)
					return updateResponse(200, []byte("direct")), nil
				})
				response, err := p.client.Get("http://target.invalid/")
				if response != nil {
					response.Body.Close()
				}
				if mode == "auto" {
					if err != nil || direct.Load() != 1 || route.count != 1 {
						t.Fatalf("fallback missing: %v direct=%d count=%d", err, direct.Load(), route.count)
					}
				} else {
					var proxyErr *proxyConnectionError
					if !errors.As(err, &proxyErr) || direct.Load() != 0 {
						t.Fatal("manual proxy bypassed or raw error exposed", err)
					}
				}
			})
		}
	}
}

func TestR280SOCKSStalledHandshakeFallsBackBeforeRequestTimeout(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		io.Copy(io.Discard, connection)
	}()
	t.Setenv("DEEP_LEGENDS_SYSTEM_PROXY", "socks5://"+listener.Addr().String())
	p := newChampionProvider()
	defer p.client.CloseIdleConnections()
	route := p.client.Transport.(*automaticNetworkTransport)
	route.probes = map[string]string{}
	route.direct = r196RoundTrip(func(*http.Request) (*http.Response, error) { return updateResponse(200, []byte("direct")), nil })
	response, err := p.client.Get("http://target.invalid/")
	if err != nil {
		t.Fatal("stalled handshake exhausted the complete request budget", err)
	}
	response.Body.Close()
	if route.count != 1 {
		t.Fatal("stalled handshake did not fall back once", route.count)
	}
}

func TestR280SOCKSConnectedTargetErrorDoesNotFallBack(t *testing.T) {
	for _, mode := range []string{"auto", "manual"} {
		t.Run(mode, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			go func() {
				connection, err := listener.Accept()
				if err != nil {
					return
				}
				defer connection.Close()
				connection.SetDeadline(time.Now().Add(3 * time.Second))
				greeting := make([]byte, 3)
				if _, err = io.ReadFull(connection, greeting); err != nil {
					return
				}
				connection.Write([]byte{5, 0})
				header := make([]byte, 4)
				if _, err = io.ReadFull(connection, header); err != nil || header[3] != 3 {
					return
				}
				length := make([]byte, 1)
				if _, err = io.ReadFull(connection, length); err != nil {
					return
				}
				if _, err = io.CopyN(io.Discard, connection, int64(length[0])+2); err != nil {
					return
				}
				connection.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
				if _, err = http.ReadRequest(bufio.NewReader(connection)); err != nil {
					return
				}
				io.WriteString(connection, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 10\r\nConnection: close\r\n\r\ntarget-502")
			}()
			address := "socks5://" + listener.Addr().String()
			t.Setenv("DEEP_LEGENDS_SYSTEM_PROXY", address)
			p := newChampionProvider()
			if mode == "manual" {
				if err := p.setNetworkSettings(championNetworkSettings{Mode: mode, URL: address}); err != nil {
					t.Fatal(err)
				}
			}
			defer p.client.CloseIdleConnections()
			route := p.client.Transport.(*automaticNetworkTransport)
			route.probes = map[string]string{}
			var direct atomic.Int32
			route.direct = r196RoundTrip(func(*http.Request) (*http.Response, error) {
				direct.Add(1)
				return updateResponse(200, []byte("wrong-route")), nil
			})
			response, err := p.client.Get("http://target.invalid/")
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode != 502 || string(body) != "target-502" || direct.Load() != 0 || route.count != 0 {
				t.Fatal("target error bypassed SOCKS proxy", response.StatusCode, string(body), direct.Load())
			}
		})
	}
}

func TestR280PACRoutesAreIsolatedAndRefresh(t *testing.T) {
	t.Setenv("DEEP_LEGENDS_SYSTEM_PROXY", "http://127.0.0.1:1")
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("proxy")) }))
	defer proxy.Close()
	p := newChampionProvider()
	defer p.client.CloseIdleConnections()
	p.systemProxyRoutes = map[string]string{opggPageHost: "", "kr.api.riotgames.com": proxy.URL, aramkitDataCN: "http://127.0.0.1:1"}
	parent := p.client.Transport.(*automaticNetworkTransport)
	parent.probes = map[string]string{}
	request := func(host string) *http.Request { r, _ := http.NewRequest("GET", "http://"+host+"/", nil); return r }
	direct := r196RoundTrip(func(*http.Request) (*http.Response, error) { return updateResponse(200, []byte("direct")), nil })
	parent.forRequest(request(opggPageHost)).direct = direct
	parent.forRequest(request(aramkitDataCN)).direct = direct
	for host, want := range map[string]string{opggPageHost: "direct", aramkitDataCN: "direct", "kr.api.riotgames.com": "proxy"} {
		res, err := p.client.Do(request(host))
		if err != nil {
			t.Fatal(host, err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if string(body) != want {
			t.Fatal(host, string(body), want)
		}
	}
	if parent.forRequest(request("kr.api.riotgames.com")).failed {
		t.Fatal("another site's failed proxy poisoned Riot")
	}
	a := &app{champions: p}
	raw := `{"proxy":"","routes":{"op.gg":"` + proxy.URL + `","kr.api.riotgames.com":""}}`
	rec := httptest.NewRecorder()
	a.handleSystemProxy(rec, httptest.NewRequest("POST", "/api/system-proxy", strings.NewReader(raw)))
	if rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	riot := parent.forRequest(request("kr.api.riotgames.com"))
	riot.direct = direct
	for host, want := range map[string]string{opggPageHost: "proxy", "kr.api.riotgames.com": "direct"} {
		res, err := p.client.Do(request(host))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if string(body) != want {
			t.Fatal(host, string(body))
		}
	}
	bad := httptest.NewRecorder()
	a.handleSystemProxy(bad, httptest.NewRequest("POST", "/api/system-proxy", strings.NewReader(`{"routes":{"unrecognized.invalid":""}}`)))
	if bad.Code != 400 {
		t.Fatal("accepted unrecognized PAC target", bad.Code)
	}
	if networkGroup("api.your.gg") == networkGroup(aramkitDataCN) || networkGroup(dataDragonHost) == networkGroup(aramkitDataCN) {
		t.Fatal("independent sources share ARAMKit route choice")
	}
}

func TestR280ExternalClientFollowsLiveSettings(t *testing.T) {
	t.Setenv("DEEP_LEGENDS_SYSTEM_PROXY", "http://127.0.0.1:1")
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("direct")) }))
	defer target.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("proxy")) }))
	defer proxy.Close()
	p := newChampionProvider()
	defer func() { p.client.CloseIdleConnections() }()
	p.client.Transport.(*automaticNetworkTransport).probes = map[string]string{}
	a := &app{champions: p}
	base := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	client := a.externalHTTPClient(base)
	defer client.CloseIdleConnections()
	read := func(want string, wantError bool) {
		t.Helper()
		response, err := client.Get(target.URL)
		if wantError {
			if response != nil {
				response.Body.Close()
			}
			if err == nil {
				t.Fatal("manual failure silently bypassed")
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if string(body) != want {
			t.Fatal(string(body), want)
		}
	}
	read("direct", false)
	if client.Timeout != base.Timeout || client.CheckRedirect == nil {
		t.Fatal("caller policy dropped")
	}
	if err := p.setNetworkSettings(championNetworkSettings{Mode: "manual", URL: proxy.URL}); err != nil {
		t.Fatal(err)
	}
	read("proxy", false)
	if err := p.setNetworkSettings(championNetworkSettings{Mode: "manual", URL: "http://127.0.0.1:1"}); err != nil {
		t.Fatal(err)
	}
	read("", true)
	if err := p.setNetworkSettings(championNetworkSettings{Mode: "direct"}); err != nil {
		t.Fatal(err)
	}
	read("direct", false)
	// Updater-specific transport deadlines survive the route wrapper as well.
	template := http.DefaultTransport.(*http.Transport).Clone()
	template.TLSHandshakeTimeout = updateSourceTimeout
	template.ResponseHeaderTimeout = updateSourceTimeout
	updateClient := a.externalHTTPClient(&http.Client{Transport: template})
	defer updateClient.CloseIdleConnections()
	res, err := updateClient.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	dynamic := updateClient.Transport.(*providerNetworkTransport)
	mapped := dynamic.route.(*mappedNetworkTransport)
	for _, pair := range mapped.routes {
		for _, rt := range pair {
			transport := rt.(*http.Transport)
			if transport.TLSHandshakeTimeout != updateSourceTimeout || transport.ResponseHeaderTimeout != updateSourceTimeout {
				t.Fatal("updater deadlines changed")
			}
		}
	}
	sgp := newSGPProvider()
	defer sgp.http.CloseIdleConnections()
	if sgp.http.Transport.(*http.Transport).Proxy != nil {
		t.Fatal("SGP inherits environment proxy")
	}
}

func TestR280RecoveryPreservesSuccessfulAssets(t *testing.T) {
	a := &app{assetCache: map[string][]byte{"cached": []byte("keep")}, assetFailureUntil: map[string]time.Time{"failed": time.Now().Add(time.Hour)}, assetHostBackoffUntil: map[string]time.Time{"host": time.Now().Add(time.Hour)}, assetHostTimeouts: map[string]int{"host": 3}}
	c := a.announcementState()
	c.source = "remote"
	c.fetchFailed = true
	c.at = time.Now()
	a.recoverExternalNetwork()
	if !bytes.Equal(a.assetCache["cached"], []byte("keep")) || len(a.assetFailureUntil) != 0 || len(a.assetHostBackoffUntil) != 0 || !c.at.IsZero() {
		t.Fatal("recovery discarded successful assets or retained failed state")
	}
	c.fetchFailed = false
	c.at = time.Now()
	a.recoverExternalNetwork()
	if c.at.IsZero() {
		t.Fatal("recovery forced a successful announcement cache to refetch")
	}
}

func TestR280MappedClientFollowsPACChanges(t *testing.T) {
	t.Setenv("DEEP_LEGENDS_SYSTEM_PROXY", "")
	proxy := func(label string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(label)) }))
	}
	first, second := proxy("first"), proxy("second")
	defer first.Close()
	defer second.Close()
	p := newChampionProvider()
	defer p.client.CloseIdleConnections()
	parent := p.client.Transport.(*automaticNetworkTransport)
	parent.probes = map[string]string{}
	p.systemProxyRoutes = map[string]string{opggPageHost: first.URL}
	a := &app{champions: p}
	client := a.externalHTTPClient(&http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()})
	defer client.CloseIdleConnections()
	read := func(want string) {
		t.Helper()
		response, err := client.Get("http://op.gg/")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if string(body) != want {
			t.Fatalf("stale PAC route: got %s want %s", body, want)
		}
	}
	read("first")
	response := httptest.NewRecorder()
	a.handleSystemProxy(response, httptest.NewRequest("POST", "/api/system-proxy", strings.NewReader(`{"routes":{"op.gg":"`+second.URL+`"}}`)))
	if response.Code != 200 {
		t.Fatal(response.Code)
	}
	read("second")
	dynamic := client.Transport.(*providerNetworkTransport)
	mapped := dynamic.route.(*mappedNetworkTransport)
	if len(mapped.routes) != 1 {
		t.Fatal("obsolete mapped PAC transport retained", len(mapped.routes))
	}
}

func TestR280PinnedRelayUsesItsOwnPACRoute(t *testing.T) {
	t.Setenv("DEEP_LEGENDS_SYSTEM_PROXY", "http://127.0.0.1:1")
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("pinned-direct")) }))
	defer target.Close()
	_, port, _ := net.SplitHostPort(target.Listener.Addr().String())
	p := newChampionProvider()
	defer p.client.CloseIdleConnections()
	p.client.Transport.(*automaticNetworkTransport).probes = map[string]string{}
	p.systemProxyRoutes = map[string]string{"relay.fixture": ""}
	m := &riotRelayEntryManager{clients: map[*http.Client]map[string]*http.Client{}}
	client := m.client(p.client, riotRelayEntry{Label: "fixture", Hostname: "relay.fixture:" + port, IP: "127.0.0.1"})
	defer client.CloseIdleConnections()
	response, err := client.Get("http://relay.fixture:" + port + "/")
	if err != nil {
		t.Fatal("pinned relay ignored DIRECT PAC rule", err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if string(body) != "pinned-direct" {
		t.Fatal(string(body))
	}
}
