package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestR280AutomaticDeadProxyAndRecovery(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	defer target.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	t.Setenv("DEEP_LEGENDS_SYSTEM_PROXY", "http://"+address)
	p := newChampionProvider()
	defer p.client.CloseIdleConnections()
	rt := p.client.Transport.(*automaticNetworkTransport)
	rt.probes = map[string]string{}
	now := time.Now()
	rt.now = func() time.Time { return now }
	var recovered atomic.Int32
	rt.recover = func() { recovered.Add(1) }
	for i := 0; i < 2; i++ {
		res, err := p.client.Get(target.URL)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
	}
	if rt.count != 1 || recovered.Load() != 1 {
		t.Fatalf("fallback=%d recovery=%d", rt.count, recovered.Load())
	}
	proxy := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("proxy")) }))
	proxy.Listener, err = net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	proxy.Start()
	defer proxy.Close()
	now = now.Add(61 * time.Second)
	res, err := p.client.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(body) != "proxy" || rt.failed {
		t.Fatal("proxy not recovered")
	}
}
func TestR280ConnectRejectionAndTarget502(t *testing.T) {
	for _, connect := range []bool{true, false} {
		t.Run(map[bool]string{true: "CONNECT", false: "target"}[connect], func(t *testing.T) {
			var direct atomic.Int32
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(502) }))
			defer proxy.Close()
			t.Setenv("DEEP_LEGENDS_SYSTEM_PROXY", proxy.URL)
			p := newChampionProvider()
			rt := p.client.Transport.(*automaticNetworkTransport)
			defer rt.CloseIdleConnections()
			rt.probes = map[string]string{}
			rt.direct = r196RoundTrip(func(r *http.Request) (*http.Response, error) {
				direct.Add(1)
				return updateResponse(200, []byte("ok")), nil
			})
			target := "http://target.invalid/"
			if connect {
				target = "https://target.invalid/"
			}
			res, err := p.client.Get(target)
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			want := int32(0)
			if connect {
				want = 1
			}
			if direct.Load() != want {
				t.Fatal("wrong fallback", direct.Load())
			}
		})
	}
}
func TestR280ManualNeverFallsBackOrProbes(t *testing.T) {
	t.Setenv("DEEP_LEGENDS_SYSTEM_PROXY", "")
	p := newChampionProvider()
	if err := p.setNetworkSettings(championNetworkSettings{Mode: "manual", URL: "http://127.0.0.1:1"}); err != nil {
		t.Fatal(err)
	}
	defer p.client.CloseIdleConnections()
	rt := p.client.Transport.(*automaticNetworkTransport)
	rt.direct = r196RoundTrip(func(*http.Request) (*http.Response, error) {
		t.Error("manual fallback")
		return nil, errors.New("forbidden")
	})
	ctx := withNetworkTrace(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://target.invalid/", nil)
	_, err := p.client.Do(req)
	if trace := ctx.Value(networkTraceKey{}).(*networkTrace); trace.proxy != "manual" || trace.stage != "proxyconnect" {
		t.Fatal("missing manual proxy failure stage", trace)
	}
	if err == nil || !strings.Contains(riotHTTPErrorBody(err).Error, "设置的代理 127.0.0.1:1 无法连接") {
		t.Fatal(err)
	}
	if len(rt.choices) != 0 {
		t.Fatal("manual probed")
	}
}
func TestR280ProbeFasterRouteAndExpiry(t *testing.T) {
	for _, fastDirect := range []bool{true, false} {
		t.Run(map[bool]string{true: "direct", false: "proxy"}[fastDirect], func(t *testing.T) {
			t.Setenv("DEEP_LEGENDS_SYSTEM_PROXY", "http://127.0.0.1:7890")
			p := newChampionProvider()
			rt := p.client.Transport.(*automaticNetworkTransport)
			defer rt.CloseIdleConnections()
			var probes atomic.Int32
			now := time.Now()
			rt.now = func() time.Time { return now }
			rt.probes = map[string]string{"riot": "http://fixture.invalid/"}
			route := func(d bool) http.RoundTripper {
				return r196RoundTrip(func(r *http.Request) (*http.Response, error) {
					probes.Add(1)
					if d != fastDirect {
						time.Sleep(20 * time.Millisecond)
					}
					return updateResponse(200, nil), nil
				})
			}
			rt.direct = route(true)
			rt.proxy = route(false)
			check := func() {
				rt.probeGroup("riot")
				r281WaitUntil(t, "probe hung", func() bool {
					rt.mu.Lock()
					c := rt.choices["riot"]
					rt.mu.Unlock()
					if !c.probing && !c.at.IsZero() {
						if c.direct != fastDirect {
							t.Error("wrong winner")
						}
						return true
					}
					return false
				})
			}
			check()
			rt.probeGroup("riot")
			if probes.Load() != 2 {
				t.Fatal("early reprobe")
			}
			now = now.Add(30 * time.Minute)
			check()
			if probes.Load() != 4 {
				t.Fatal("missing remeasurement")
			}
		})
	}
}
func TestR280BuiltinCopyDroppedWithoutMigration(t *testing.T) {
	s := r204KeyFixture(t)
	riotEmbeddedKey = func() string { return "fixture-built-in" }
	if err := s.migrateEmbedded(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.store.root, riotUserKeyFile)); !os.IsNotExist(err) {
		t.Fatal("migration wrote key")
	}
	s.mu.Lock()
	err := s.writeLocked("fixture-built-in")
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	loaded := loadRiotKeyStore(s.store)
	if loaded.key != "" {
		t.Fatal("builtin retained")
	}
	if _, err := os.Stat(filepath.Join(s.store.root, riotUserKeyFile)); !os.IsNotExist(err) {
		t.Fatal("builtin file retained")
	}
	_, err = s.save(context.Background(), r204ValidationClient(0), r204FixtureKey)
	if err != nil || riotKeyState() != "unverified" {
		t.Fatal(err, riotKeyState())
	}
	s.observe(r204FixtureKey, 200)
	if riotKeyState() != "configured" {
		t.Fatal("didn't validate")
	}
}
func TestR280ProxyErrorsAreSanitized(t *testing.T) {
	for _, raw := range []string{"Get https://secret.invalid/path: proxyconnect tcp: dial tcp 127.0.0.1:7890: refused", "dial tcp: failed"} {
		body := riotHTTPErrorBody(errors.New(raw))
		for _, bad := range []string{"https://", "dial tcp", "proxyconnect"} {
			if strings.Contains(body.Error, bad) {
				t.Fatal(body)
			}
		}
	}
}
func TestR280BackgroundYieldsAt75Percent(t *testing.T) {
	p := newRiotProvider(newChampionProvider())
	now := time.Now()
	p.limitNow = func() time.Time { return now }
	scope := riotRequestRateScope(p.clusterHost(), "/lol/match/v5/matches/by-puuid/fixture/ids")
	p.observeRiotRate(scope, http.Header{"X-App-Rate-Limit": {"100:120"}, "X-App-Rate-Limit-Count": {"75:120"}}, 200)
	ctx := context.WithValue(context.Background(), riotRateScopeKey{}, scope)
	if !errors.Is(p.admitRiotBackground(withRiotBackground(ctx)), errThrottled) {
		t.Fatal("background admitted")
	}
	if err := p.wait(ctx); err != nil {
		t.Fatal("foreground blocked", err)
	}
	now = now.Add(121 * time.Second)
	if err := p.admitRiotBackground(withRiotBackground(ctx)); err != nil {
		t.Fatal("background didn't resume", err)
	}
}

func TestR280RecoveryClearsFailuresAndBroadcastsOnce(t *testing.T) {
	s := r204KeyFixture(t)
	_ = s
	oldRelay := riotRelays
	riotRelays = &riotRelayState{nextTry: time.Now().Add(time.Hour), failures: 3, networkFailures: 3}
	defer func() { riotRelays = oldRelay }()
	p := newChampionProvider()
	p.hexdata.state.Circuits = map[string]hexdataCircuitState{"heroes": {Failures: 3, Until: time.Now().Add(time.Hour)}}
	r := newRiotProvider(p)
	r.accountCache = map[string]riotAccountCacheEntry{"failed": {err: errors.New("network")}, "success": {account: riotAccount{PUUID: "fixture"}}}
	c := newAramkitRatingClient(p, nil, nil)
	c.cache["failed"] = aramkitRatingCacheEntry{outcome: aramkitRatingOutcome{response: mayhemRatingResponse{UnavailableReason: "网络暂不可用"}}}
	c.cache["missing"] = aramkitRatingCacheEntry{outcome: aramkitRatingOutcome{response: mayhemRatingResponse{UnavailableReason: "未收录"}}}
	events := make(chan string, 4)
	a := &app{champions: p, riot: r, mayhemRatings: c, eventSubscribers: map[chan string]struct{}{events: {}}}
	rt := p.client.Transport.(*automaticNetworkTransport)
	rt.failed = true
	rt.recover = a.recoverExternalNetwork
	rt.markSuccess(rt.generation)
	rt.markSuccess(rt.generation)
	if len(events) != 1 || !strings.Contains(<-events, "riot-relay-recovered") {
		t.Fatal("recovery event count")
	}
	if riotRelays.failures != 0 || !riotRelays.nextTry.IsZero() || len(p.hexdata.state.Circuits) != 0 {
		t.Fatal("backoff not cleared")
	}
	if _, ok := r.accountCache["failed"]; ok {
		t.Fatal("negative account retained")
	}
	if _, ok := r.accountCache["success"]; !ok {
		t.Fatal("positive account removed")
	}
	if _, ok := c.cache["failed"]; ok {
		t.Fatal("ARAMKit failure retained")
	}
	if _, ok := c.cache["missing"]; !ok {
		t.Fatal("not-found removed")
	}
}

func TestR280FallbackBoundedWhenBothFail(t *testing.T) {
	t.Setenv("DEEP_LEGENDS_SYSTEM_PROXY", "http://127.0.0.1:7890")
	p := newChampionProvider()
	rt := p.client.Transport.(*automaticNetworkTransport)
	var proxyCalls, directCalls atomic.Int32
	rt.proxy = r196RoundTrip(func(*http.Request) (*http.Response, error) {
		proxyCalls.Add(1)
		return nil, &net.OpError{Op: "proxyconnect", Net: "tcp", Err: errors.New("refused")}
	})
	rt.direct = r196RoundTrip(func(*http.Request) (*http.Response, error) {
		directCalls.Add(1)
		return nil, errors.New("fixture direct unavailable")
	})
	req, _ := http.NewRequest(http.MethodGet, "https://fixture.invalid/", nil)
	_, err := rt.RoundTrip(req)
	if err == nil || proxyCalls.Load() != 1 || directCalls.Load() != 1 {
		t.Fatalf("proxy=%d direct=%d err=%v", proxyCalls.Load(), directCalls.Load(), err)
	}
}
func TestR280PinnedRelayPreservesManualProxy(t *testing.T) {
	p := newChampionProvider()
	if err := p.setNetworkSettings(championNetworkSettings{Mode: "manual", URL: "http://127.0.0.1:1"}); err != nil {
		t.Fatal(err)
	}
	m := &riotRelayEntryManager{clients: map[*http.Client]map[string]*http.Client{}}
	client := m.client(p.client, riotRelayEntry{Label: "fixture", Hostname: "relay.fixture", IP: "127.0.0.1"})
	defer client.CloseIdleConnections()
	_, err := client.Get("https://relay.fixture/")
	var proxyErr *proxyConnectionError
	if !errors.As(err, &proxyErr) {
		t.Fatal("manual proxy bypassed for pinned relay", err)
	}
}

func TestR280AnnouncementSummaryRemainsSeparateFromBody(t *testing.T) {
	notices, err := decodeAnnouncements(bundledAnnouncements)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range notices {
		if strings.TrimSpace(n.Title) == "" || strings.TrimSpace(n.Content) == "" {
			t.Fatal("missing notice body")
		}
		// Summary is an independent tooltip field; decoding never prepends it to the Markdown.
		var objects []announcement
		if err := json.Unmarshal(bundledAnnouncements, &objects); err != nil {
			t.Fatal(err)
		}
		for _, raw := range objects {
			if raw.ID == n.ID && (raw.Content != n.Content || raw.Summary != n.Summary) {
				t.Fatal("summary merged into body")
			}
		}
	}
}

func TestR280ConcurrentProxyRecoveryBroadcastsOnce(t *testing.T) {
	t.Setenv("DEEP_LEGENDS_SYSTEM_PROXY", "http://127.0.0.1:7890")
	p := newChampionProvider()
	rt := p.client.Transport.(*automaticNetworkTransport)
	rt.failed = true
	rt.probes = map[string]string{}
	var recovered atomic.Int32
	rt.recover = func() { recovered.Add(1) }
	rt.proxy = r196RoundTrip(func(*http.Request) (*http.Response, error) { return updateResponse(200, nil), nil })
	var workers sync.WaitGroup
	for i := 0; i < 12; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			req, _ := http.NewRequest(http.MethodGet, "https://fixture.invalid/", nil)
			res, err := rt.RoundTrip(req)
			if err == nil {
				res.Body.Close()
			}
		}()
	}
	workers.Wait()
	if recovered.Load() != 1 {
		t.Fatalf("recovery broadcasts=%d", recovered.Load())
	}
}
