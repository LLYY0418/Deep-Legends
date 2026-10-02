package main

import (
	"context"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestR166AbilityRadarGradeAnchorsAndBounds(t *testing.T) {
	anchors := []struct{ ratio, score float64 }{
		{0, 0}, {0.50, 5}, {0.66, 15}, {0.74, 23}, {0.82, 31},
		{0.90, 39}, {0.97, 47}, {1, 50}, {1.03, 53}, {1.10, 61},
		{1.20, 69}, {1.35, 77}, {2, 95},
	}
	for _, anchor := range anchors {
		if got := abilityRadarScore(anchor.ratio, 1); math.Abs(got-anchor.score) > 1e-9 {
			t.Fatalf("ratio %.2f score %.2f, want %.2f", anchor.ratio, got, anchor.score)
		}
	}
	if gap := abilityRadarScore(1, 1) - abilityRadarScore(0.929, 1); gap < 6 {
		t.Fatalf("DPM 780/840 gap %.1f, want at least six radial points", gap)
	}
	for index := 1; index <= 2000; index++ {
		previous := abilityRadarScore(float64(index-1)/1000, 1)
		current := abilityRadarScore(float64(index)/1000, 1)
		if current < previous {
			t.Fatalf("mapping decreased at ratio %.3f: %.3f -> %.3f", float64(index)/1000, previous, current)
		}
	}
	if abilityRadarScore(0, 1) != 0 || abilityRadarScore(1e100, 1) != 95 {
		t.Fatal("zero and extreme ratios must remain bounded")
	}
}

func TestR166RSCDiagnosticHasSafeFields(t *testing.T) {
	provider := newChampionProvider()
	var events []map[string]any
	provider.diag = func(event map[string]any) { events = append(events, event) }
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("rsc sample")), Request: request}, nil
	})}
	data, err := provider.fetchOPGGRSCDirect(context.Background(), "/lol/modes/aram-mayhem/ahri/build")
	if err != nil || string(data) != "rsc sample" {
		t.Fatalf("RSC read = %q, %v", data, err)
	}
	if len(events) != 1 {
		t.Fatalf("RSC diagnostics = %#v", events)
	}
	event := events[0]
	if event["event"] != "champion_upstream" || event["host"] != "op.gg" || event["kind"] != "rsc" || event["status"] != http.StatusOK || event["bytes"] != len(data) || event["cache"] != championCacheStateMiss || event["duration_ms"] == nil {
		t.Fatalf("RSC diagnostic fields = %#v", event)
	}
	if event["url"] != nil || event["cookie"] != nil {
		t.Fatalf("RSC diagnostic leaked request details: %#v", event)
	}
}

func TestR166MayhemRSCPrefetchMergesWithDetailFlight(t *testing.T) {
	var requests atomic.Int32
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(nil)
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		entered <- struct{}{}
		select {
		case <-release:
		case <-request.Context().Done():
			return nil, request.Context().Err()
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("rsc fixture")), Request: request}, nil
	})}
	a := &app{champions: provider}
	first := httptest.NewRecorder()
	a.handleMayhemRSCPrefetch(first, httptest.NewRequest(http.MethodGet, "/api/champions/mayhem-rsc-prefetch?champion=ahri", nil))
	if first.Code != http.StatusAccepted {
		t.Fatalf("prefetch status %d", first.Code)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("RSC prefetch did not start")
	}
	for index := 0; index < 10; index++ {
		other := httptest.NewRecorder()
		a.handleMayhemRSCPrefetch(other, httptest.NewRequest(http.MethodGet, "/api/champions/mayhem-rsc-prefetch?champion=yasuo", nil))
		if other.Code != http.StatusTooManyRequests {
			t.Fatalf("busy prefetch status %d", other.Code)
		}
	}
	key := "v2|opgg-rsc|aram-mayhem|ahri"
	joined := make(chan error, 1)
	go func() {
		_, err := provider.cache.loadWithStatus(context.Background(), key, 6*time.Hour, 24*time.Hour, true,
			func(ctx context.Context) ([]byte, error) {
				return provider.fetchOPGGRSCDirect(ctx, "/lol/modes/aram-mayhem/ahri/build")
			})
		joined <- err
	}()
	time.Sleep(20 * time.Millisecond)
	if got := requests.Load(); got != 1 {
		close(release)
		t.Fatalf("joining an in-flight prefetch made %d upstream requests", got)
	}
	close(release)
	if err := <-joined; err != nil {
		t.Fatal(err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("prefetch plus detail made %d upstream RSC requests, want one", got)
	}
}

func TestR166ArenaAggregateStartsBeforeOPGGVersionCompletes(t *testing.T) {
	versionEntered := make(chan struct{}, 1)
	aggregateEntered := make(chan struct{}, 1)
	releaseVersion := make(chan struct{})
	releaseAggregate := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(releaseVersion); close(releaseAggregate) }) }
	defer unblock()
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(nil)
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if strings.HasSuffix(request.URL.Path, "/versions") {
			versionEntered <- struct{}{}
			select {
			case <-releaseVersion:
			case <-request.Context().Done():
				return nil, request.Context().Err()
			}
		} else if request.URL.Host == yourGGArenaHost && request.URL.Path == "/kr/api/arena/champions/67" {
			aggregateEntered <- struct{}{}
			select {
			case <-releaseAggregate:
			case <-request.Context().Done():
				return nil, request.Context().Err()
			}
		}
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("unavailable")), Request: request}, nil
	})}
	done := make(chan struct{})
	go func() { _, _ = provider.loadStructuredDetail(context.Background(), "arena", "67", "", ""); close(done) }()
	select {
	case <-versionEntered:
	case <-time.After(time.Second):
		t.Fatal("OP.GG versions request did not start")
	}
	select {
	case <-aggregateEntered:
	case <-time.After(time.Second):
		t.Fatal("YOUR.GG aggregate waited for OP.GG versions")
	}
	unblock()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("detail did not finish after releasing versions")
	}
}

func TestR166ArenaVersionWarmupSkipsDetailLookupUntilExpiry(t *testing.T) {
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(nil)
	var upstream atomic.Int32
	var events atomic.Int32
	provider.diag = func(event map[string]any) {
		if event["event"] == "champion_upstream" && event["host"] == opggChampionHost {
			events.Add(1)
		}
	}
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		upstream.Add(1)
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":["16.19"]}`)), Request: request}, nil
	})}
	spec := opggModeSpecs["arena"]
	for index := 0; index < 2; index++ {
		version, err := provider.loadOPGGLatestVersion(context.Background(), spec)
		if err != nil || version != "16.19" {
			t.Fatalf("version read %d = %q, %v", index, version, err)
		}
	}
	if upstream.Load() != 1 || events.Load() != 1 {
		t.Fatalf("preheated version reread after click: wire=%d logs=%d", upstream.Load(), events.Load())
	}
	provider.mu.Lock()
	provider.arenaVersionAt = time.Now().Add(-21 * time.Minute)
	provider.mu.Unlock()
	if _, err := provider.loadOPGGLatestVersion(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	if events.Load() != 2 {
		t.Fatalf("expired version was not rechecked: logs=%d", events.Load())
	}
}
