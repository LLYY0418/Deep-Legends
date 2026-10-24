package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	netproxy "golang.org/x/net/proxy"
)

// Only the automatic system-proxy route may fall back. Target HTTP errors are
// responses, never proxy failures. CONNECT rejection is reported by Transport.
type proxyConnectionError struct {
	address string
	cause   error
}

func (e *proxyConnectionError) Error() string {
	return "设置的代理 " + e.address + " 无法连接"
}
func (e *proxyConnectionError) Unwrap() error { return e.cause }

type networkTraceKey struct{}
type networkTrace struct{ proxy, stage string }

func withNetworkTrace(ctx context.Context) context.Context {
	return context.WithValue(ctx, networkTraceKey{}, &networkTrace{proxy: "none"})
}
func networkErrorStage(err error) string {
	if err == nil {
		return ""
	}
	var proxy *proxyConnectionError
	if errors.As(err, &proxy) {
		return "proxyconnect"
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return "dns"
	}
	var op *net.OpError
	if errors.As(err, &op) {
		if op.Op == "proxyconnect" {
			return "proxyconnect"
		}
		return "connect"
	}
	var cert *tls.CertificateVerificationError
	if errors.As(err, &cert) || strings.Contains(strings.ToLower(err.Error()), "tls") {
		return "tls"
	}
	return "headers"
}
func proxyConnectFailed(err error, proxy *url.URL) bool {
	if err == nil || proxy == nil {
		return false
	}
	var op *net.OpError
	if errors.As(err, &op) {
		if op.Op == "proxyconnect" {
			return true
		}
		if (proxy.Scheme == "socks5" || proxy.Scheme == "socks5h") && op.Op == "socks connect" {
			return true
		}
		if op.Op == "dial" && op.Addr != nil {
			host, port, _ := net.SplitHostPort(op.Addr.String())
			return port == proxy.Port() && (strings.EqualFold(host, proxy.Hostname()) || net.ParseIP(host) != nil)
		}
	}
	// Go's HTTP CONNECT rejection is an untyped error (e.g. "Bad Gateway").
	return strings.Contains(err.Error(), "proxy CONNECT") || strings.Contains(err.Error(), "TLS handshake timeout")
}

type networkChoice struct {
	direct  bool
	at      time.Time
	probing bool
}
type automaticNetworkTransport struct {
	mu                sync.Mutex
	settings          championNetworkSettings
	proxyURL          string
	proxy, direct     http.RoundTripper
	until             time.Time
	failed, recovered bool
	recovering        bool
	generation        uint64
	recoveryPending   bool
	count             int
	choices           map[string]networkChoice
	now               func() time.Time
	record            func(map[string]any)
	recover           func()
	state             string
	probes            map[string]string
	resolveProxy      func() string
	routeProvider     *championProvider
	hostRoutes        map[string]hostNetworkRoute
	targetHost        string
}

var networkProbeURLs = map[string]string{
	"riot":      "https://kr.api.riotgames.com/lol/status/v4/platform-data",
	"opgg":      "https://lol-api-champion.op.gg/",
	"aramkit":   "https://api.aramkit.com/",
	"community": "https://raw.communitydragon.org/latest/game/",
}

func networkGroup(host string) string {
	switch {
	case strings.Contains(host, "riotgames.com"):
		return "riot"
	case strings.Contains(host, "op.gg"):
		return "opgg"
	case strings.Contains(host, "aramkit"):
		return "aramkit"
	case strings.Contains(host, "communitydragon"):
		return "community"
	default:
		return host
	}
}
func (t *automaticNetworkTransport) emit(state string) {
	t.mu.Lock()
	if state == t.state {
		t.mu.Unlock()
		return
	}
	t.state = state
	u, _ := url.Parse(t.proxyURL)
	loop := u != nil && (net.ParseIP(u.Hostname()).IsLoopback() || u.Hostname() == "localhost")
	address := "remote"
	if loop {
		address = u.Port()
	}
	event := map[string]any{"event": "network_proxy_state", "mode": t.settings.Mode, "system_proxy_present": t.proxyURL != "", "proxy_is_loopback": loop, "proxy_port": address, "state": state, "fallback_count": t.count}
	t.mu.Unlock()
	if t.record != nil {
		t.record(event)
	}
}
func (t *automaticNetworkTransport) active() string {
	t.mu.Lock()
	children := make([]*automaticNetworkTransport, 0, len(t.hostRoutes))
	for _, route := range t.hostRoutes {
		children = append(children, route.transport)
	}
	t.mu.Unlock()
	active := ""
	for _, child := range children {
		state := child.active()
		if state == "代理不可用，已直连" {
			return state
		}
		if state == "直连更快，已直连" || state == "代理" && active == "" {
			active = state
		}
	}
	if active != "" {
		return active
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.settings.Mode == "direct" || t.proxyURL == "" {
		return "直连"
	}
	if t.failed {
		return "代理不可用，已直连"
	}
	for _, c := range t.choices {
		if c.direct {
			return "直连更快，已直连"
		}
	}
	return "代理"
}
func (t *automaticNetworkTransport) refreshProxy(raw string) {
	t.mu.Lock()
	if t.proxyURL == raw {
		t.mu.Unlock()
		return
	}
	t.recoveryPending = t.failed
	t.recovered = false
	t.proxyURL = raw
	t.generation++
	t.recovering = false
	t.until = time.Time{}
	t.failed = false
	t.choices = map[string]networkChoice{}
	t.mu.Unlock()
	if closer, ok := t.proxy.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
	t.emit(map[bool]string{true: "direct", false: "ok"}[raw == ""])
}
func (t *automaticNetworkTransport) CloseIdleConnections() {
	t.mu.Lock()
	children := make([]*automaticNetworkTransport, 0, len(t.hostRoutes))
	for _, route := range t.hostRoutes {
		children = append(children, route.transport)
	}
	t.mu.Unlock()
	for _, child := range children {
		child.CloseIdleConnections()
	}
	for _, rt := range []http.RoundTripper{t.proxy, t.direct} {
		if c, ok := rt.(interface{ CloseIdleConnections() }); ok {
			c.CloseIdleConnections()
		}
	}
}
func (t *automaticNetworkTransport) markSuccess(generation uint64) {
	t.mu.Lock()
	notify := t.generation == generation && (t.failed || t.recoveryPending) && !t.recovered
	if notify {
		t.recovered = true
		t.recoveryPending = false
	}
	t.mu.Unlock()
	if notify && t.recover != nil {
		t.recover()
	}
}
func (t *automaticNetworkTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	route := t.forRequest(r)
	return route.roundTripRoutes(r, route.proxy, route.direct)
}

type mappedNetworkTransport struct {
	owner         *automaticNetworkTransport
	proxy, direct http.RoundTripper
	mapRoute      func(http.RoundTripper) http.RoundTripper
	mu            sync.Mutex
	routes        map[*automaticNetworkTransport][2]http.RoundTripper
}

func (t *mappedNetworkTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	owner := t.owner.forRequest(r)
	if t.mapRoute == nil {
		return owner.roundTripRoutes(r, t.proxy, t.direct)
	}
	t.mu.Lock()
	if t.routes == nil {
		t.routes = map[*automaticNetworkTransport][2]http.RoundTripper{}
	}
	routes, ok := t.routes[owner]
	if !ok {
		for previous, pair := range t.routes {
			if previous != owner && previous.targetHost == owner.targetHost {
				for _, rt := range pair {
					if c, ok := rt.(interface{ CloseIdleConnections() }); ok {
						c.CloseIdleConnections()
					}
				}
				delete(t.routes, previous)
			}
		}
		if len(t.routes) >= 128 {
			for _, pair := range t.routes {
				for _, rt := range pair {
					if c, ok := rt.(interface{ CloseIdleConnections() }); ok {
						c.CloseIdleConnections()
					}
				}
			}
			t.routes = map[*automaticNetworkTransport][2]http.RoundTripper{}
		}
		routes = [2]http.RoundTripper{t.mapRoute(owner.proxy), t.mapRoute(owner.direct)}
		t.routes[owner] = routes
	}
	t.mu.Unlock()
	return owner.roundTripRoutes(r, routes[0], routes[1])
}
func (t *mappedNetworkTransport) CloseIdleConnections() {
	t.mu.Lock()
	for _, pair := range t.routes {
		for _, rt := range pair {
			if c, ok := rt.(interface{ CloseIdleConnections() }); ok {
				c.CloseIdleConnections()
			}
		}
	}
	t.mu.Unlock()
	for _, rt := range []http.RoundTripper{t.proxy, t.direct} {
		if closer, ok := rt.(interface{ CloseIdleConnections() }); ok {
			closer.CloseIdleConnections()
		}
	}
}
func (t *automaticNetworkTransport) roundTripRoutes(r *http.Request, proxyRoute, directRoute http.RoundTripper) (*http.Response, error) {
	if t.settings.Mode == "auto" {
		t.refreshProxy(strings.TrimSpace(t.currentSystemProxy()))
	}
	trace, _ := r.Context().Value(networkTraceKey{}).(*networkTrace)
	t.mu.Lock()
	raw := t.proxyURL
	generation := t.generation
	skip := t.now().Before(t.until) || t.failed && t.recovering
	recovery := t.settings.Mode == "auto" && raw != "" && t.failed && !skip
	if recovery {
		t.recovering = true
	}
	choice := t.choices[networkGroup(r.URL.Hostname())]
	t.mu.Unlock()
	if recovery {
		defer func() {
			t.mu.Lock()
			if t.generation == generation {
				t.recovering = false
			}
			t.mu.Unlock()
		}()
	}
	if t.settings.Mode == "direct" || raw == "" || skip || choice.direct && !recovery {
		if trace != nil && raw != "" {
			trace.proxy = "fallback-direct"
		}
		res, err := directRoute.RoundTrip(r)
		if trace != nil {
			trace.stage = networkErrorStage(err)
		}
		if err == nil {
			t.markSuccess(generation)
		}
		return res, err
	}
	if trace != nil {
		if t.settings.Mode == "manual" {
			trace.proxy = "manual"
		} else {
			trace.proxy = "system"
		}
	}
	res, err := proxyRoute.RoundTrip(r)
	proxy, _ := url.Parse(raw)
	if proxyConnectFailed(err, proxy) {
		if trace != nil {
			trace.stage = "proxyconnect"
		}
		if t.settings.Mode == "manual" {
			return nil, &proxyConnectionError{proxy.Host, err}
		}
		t.mu.Lock()
		current := t.generation == generation
		if current {
			t.until = t.now().Add(time.Minute)
			if !t.failed {
				t.recovered = false
			}
			t.failed = true
			t.count++
		}
		t.mu.Unlock()
		if current {
			t.emit("unreachable")
		}
		if r.Body != nil && r.GetBody == nil {
			return nil, err
		}
		retry := r.Clone(r.Context())
		if r.GetBody != nil {
			retry.Body, err = r.GetBody()
			if err != nil {
				return nil, err
			}
		}
		if res != nil && res.Body != nil {
			res.Body.Close()
		}
		if trace != nil {
			trace.proxy = "fallback-direct"
		}
		res, err = directRoute.RoundTrip(retry)
		if err == nil && current {
			t.markSuccess(generation)
		}
	} else if err == nil {
		t.mu.Lock()
		current := t.generation == generation
		wasFailed := current && t.failed
		if wasFailed {
			t.failed = false
			t.until = time.Time{}
			t.recovering = false
			t.generation++
			t.choices = map[string]networkChoice{}
		}
		t.mu.Unlock()
		if wasFailed && t.recover != nil {
			t.recover()
		}
		if current {
			t.emit("ok")
		}
	}
	if trace != nil {
		trace.stage = networkErrorStage(err)
	}
	if t.settings.Mode == "auto" && err == nil {
		t.probeGroup(networkGroup(r.URL.Hostname()))
	}
	return res, err
}

// Races contain no player identity, cookies or Riot token. Route decisions live
// for thirty minutes; a failed proxy always gets its one-minute recovery probe.
func (t *automaticNetworkTransport) probeGroup(group string) {
	t.mu.Lock()
	c := t.choices[group]
	if t.settings.Mode != "auto" || t.proxyURL == "" || c.probing || !c.at.IsZero() && t.now().Sub(c.at) < 30*time.Minute {
		t.mu.Unlock()
		return
	}
	c.probing = true
	t.choices[group] = c
	raw := t.proxyURL
	generation := t.generation
	endpoint := t.probes[group]
	t.mu.Unlock()
	if endpoint == "" {
		t.mu.Lock()
		delete(t.choices, group)
		t.mu.Unlock()
		return
	}
	go func() {
		defer recoverPanic("automaticNetworkTransport.probeGroup")
		type result struct {
			direct      bool
			duration    time.Duration
			ok          bool
			proxyFailed bool
		}
		ch := make(chan result, 2)
		for _, direct := range []bool{false, true} {
			go func(d bool) {
				var outcome result
				defer func() { ch <- outcome }()
				defer recoverPanic("automaticNetworkTransport.probeRoute")
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				req, reqErr := http.NewRequestWithContext(ctx, http.MethodHead, endpoint, nil)
				if reqErr != nil {
					outcome = result{direct: d}
					return
				}
				start := time.Now()
				rt := t.proxy
				if d {
					rt = t.direct
				}
				res, err := rt.RoundTrip(req)
				ok := err == nil && res != nil && res.StatusCode < 500
				if res != nil {
					res.Body.Close()
				}
				proxy, _ := url.Parse(raw)
				outcome = result{direct: d, duration: time.Since(start), ok: ok, proxyFailed: !d && proxyConnectFailed(err, proxy)}
			}(direct)
		}
		a, b := <-ch, <-ch
		proxyFailed := a.proxyFailed || b.proxyFailed
		winner := a
		if !a.ok || b.ok && b.duration < a.duration {
			winner = b
		}
		t.mu.Lock()
		current := t.proxyURL == raw && t.generation == generation
		if current && winner.ok {
			t.choices[group] = networkChoice{direct: winner.ok && winner.direct, at: t.now()}
		} else if current {
			delete(t.choices, group)
		}
		if current && proxyFailed {
			if !t.failed {
				t.recovered = false
			}
			t.failed = true
			if t.until.IsZero() {
				t.until = t.now().Add(time.Minute)
			}
		}
		t.mu.Unlock()
		if current && proxyFailed {
			t.emit("unreachable")
		}
		if winner.ok && current {
			if !proxyFailed && winner.direct {
				t.emit("direct")
			} else if !proxyFailed {
				t.emit("ok")
			}
			t.markSuccess(generation)
		}
	}()
}
func systemProxyAddress() string { return getenvSystemProxy() }

func newAutomaticNetworkTransport(settings championNetworkSettings, p *championProvider) *automaticNetworkTransport {
	t := newAutomaticNetworkTransportForProxy(settings, p, systemProxyAddress)
	t.routeProvider = p
	return t
}

func (t *automaticNetworkTransport) currentSystemProxy() string {
	if t.resolveProxy != nil {
		return t.resolveProxy()
	}
	return systemProxyAddress()
}

func newAutomaticNetworkTransportForProxy(settings championNetworkSettings, p *championProvider, resolve func() string) *automaticNetworkTransport {
	raw := settings.URL
	if settings.Mode == "auto" {
		raw = resolve()
	}
	direct := newChampionHTTPClient(nil).Transport
	proxyTransport := newChampionHTTPClient(func(r *http.Request) (*url.URL, error) {
		address := settings.URL
		if settings.Mode == "auto" {
			address = resolve()
		}
		if address == "" {
			return nil, nil
		}
		proxy, err := url.Parse(address)
		if err == nil && (proxy.Scheme == "socks5" || proxy.Scheme == "socks5h") {
			return nil, nil // DialContext bounds the complete SOCKS handshake below.
		}
		return proxy, err
	}).Transport.(*http.Transport)
	proxyTransport.TLSHandshakeTimeout = 3 * time.Second
	proxyTransport.ResponseHeaderTimeout = 8 * time.Second
	proxyTransport.OnProxyConnectResponse = func(_ context.Context, _ *url.URL, _ *http.Request, res *http.Response) error {
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return fmt.Errorf("proxy CONNECT rejected: %d", res.StatusCode)
		}
		return nil
	}
	// Dial/TLS errors before CONNECT are attributable to the selected proxy.
	dial := proxyTransport.DialContext
	proxyTransport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		selected := settings.URL
		if settings.Mode == "auto" {
			selected = resolve()
		}
		u, _ := url.Parse(selected)
		if u != nil && (u.Scheme == "socks5" || u.Scheme == "socks5h") {
			proxyAddress := u.Host
			if u.Port() == "" {
				proxyAddress = net.JoinHostPort(u.Hostname(), "1080")
			}
			dialer, err := netproxy.SOCKS5("tcp", proxyAddress, nil, &net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second})
			if err != nil {
				return nil, &net.OpError{Op: "proxyconnect", Net: network, Err: err}
			}
			connectContext, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			return dialer.(netproxy.ContextDialer).DialContext(connectContext, network, address)
		}
		c, err := dial(ctx, network, address)
		if err != nil {
			raw := settings.URL
			if settings.Mode == "auto" {
				raw = resolve()
			}
			u, _ := url.Parse(raw)
			if u != nil && address == u.Host {
				return nil, &net.OpError{Op: "proxyconnect", Net: network, Err: err}
			}
		}
		return c, err
	}
	return &automaticNetworkTransport{settings: settings, proxyURL: raw, resolveProxy: resolve, proxy: proxyTransport, direct: direct, now: time.Now, choices: map[string]networkChoice{}, probes: networkProbeURLs, record: func(e map[string]any) {
		if p.diag != nil {
			p.diag(e)
		}
	}, recover: func() {
		if p.networkRecovered != nil {
			p.networkRecovered()
		}
	}}
}
