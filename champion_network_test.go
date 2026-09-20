package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type championRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn championRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestHandleChampionAssetCommunityDragonFallsBackFromLargeToSmall(t *testing.T) {
	smallImage := []byte("\x89PNG\r\n\x1a\nsmall-image")
	provider := newChampionProvider()
	var requested []string
	var requestedMu sync.Mutex
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requestedMu.Lock()
		requested = append(requested, request.URL.Path)
		requestedMu.Unlock()
		status, body := http.StatusNotFound, []byte("missing")
		if request.URL.Path == "/latest/game/assets/ux/cherry/augments/icons/drop_bear_small.png" {
			status, body = http.StatusOK, smallImage
		}
		return &http.Response{
			StatusCode: status, Status: http.StatusText(status), Header: make(http.Header),
			Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Request: request,
		}, nil
	})}
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	if err := os.MkdirAll(filepath.Join(store.root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	a := &app{champions: provider, storage: store}
	provider.diag = a.recordDiagnostic
	recorder := httptest.NewRecorder()
	a.handleChampionAsset(recorder, httptest.NewRequest(http.MethodGet, "/api/champion-asset?source=communitydragon&path=%2Flatest%2Fgame%2Fassets%2Fux%2Fcherry%2Faugments%2Ficons%2Fdrop_bear_large.png", nil))
	if recorder.Code != http.StatusOK || !bytes.Equal(recorder.Body.Bytes(), smallImage) {
		t.Fatalf("large-to-small response = status %d body %q", recorder.Code, recorder.Body.Bytes())
	}
	requestedMu.Lock()
	allowed := communityDragonChampionAssetCandidates("/latest/game/assets/ux/cherry/augments/icons/drop_bear_large.png")
	for _, request := range requested {
		found := false
		for _, candidate := range allowed {
			if request == candidate {
				found = true
			}
		}
		if !found {
			t.Errorf("unexpected racing candidate: %s", request)
		}
	}
	if len(requested) > len(allowed) {
		t.Error("candidate retried")
	}
	requestedMu.Unlock()
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "drop_bear") || strings.Contains(string(data), "2031") {
		t.Fatalf("augment icon diagnostic leaked a concrete icon: %s", data)
	}
	var event map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(data), &event); err != nil {
		t.Fatal(err)
	}
	if event["event"] != "augment_icon_fetch" || event["source"] != "communitydragon" || event["path_template"] != "game/ux/cherry/augments/icons/{icon}_large.png" || event["status"] != float64(http.StatusOK) || event["candidate_index"] != float64(4) || event["fell_back"] != true {
		t.Fatalf("persisted augment icon diagnostic = %#v", event)
	}
}

func TestHandleChampionAssetCommunityDragonReportsFinalAugmentFailure(t *testing.T) {
	provider := newChampionProvider()
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNotFound, Status: http.StatusText(http.StatusNotFound), Header: make(http.Header),
			Body: io.NopCloser(strings.NewReader("missing")), ContentLength: 7, Request: request,
		}, nil
	})}
	var events []map[string]any
	provider.diag = func(event map[string]any) { events = append(events, event) }
	recorder := httptest.NewRecorder()
	(&app{champions: provider}).handleChampionAsset(recorder, httptest.NewRequest(http.MethodGet, "/api/champion-asset?source=communitydragon&path=%2Flatest%2Fgame%2Fassets%2Fux%2Fcherry%2Faugments%2Ficons%2Fmissing_large.png", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("failed augment status = %d, want 404", recorder.Code)
	}
	if len(events) != 1 || events[0]["event"] != "augment_icon_fetch" || events[0]["status"] != http.StatusNotFound || events[0]["candidate_index"] != -1 || events[0]["fell_back"] != true {
		t.Fatalf("final augment failure diagnostic = %#v", events)
	}
}

func TestChampionNetworkSettingsValidation(t *testing.T) {
	for _, mode := range []string{"auto", "direct"} {
		settings, err := validateChampionNetworkSettings(championNetworkSettings{Mode: mode, URL: "http://ignored:7890"})
		if err != nil || settings.URL != "" {
			t.Fatalf("%s should be valid and clear URL: %#v %v", mode, settings, err)
		}
	}
	settings, err := validateChampionNetworkSettings(championNetworkSettings{Mode: "manual", URL: "http://127.0.0.1:7890"})
	if err != nil || settings.URL == "" {
		t.Fatalf("manual proxy should be valid: %#v %v", settings, err)
	}
	for _, value := range []string{"", "ftp://proxy.example:21", "http://user:secret@proxy.example:8080", "http://proxy.example:8080/path"} {
		if _, err := validateChampionNetworkSettings(championNetworkSettings{Mode: "manual", URL: value}); err == nil {
			t.Fatalf("manual proxy %q should be rejected", value)
		}
	}
}

func TestChampionAutoProxyUsesDesktopResolution(t *testing.T) {
	previous, existed := os.LookupEnv("DEEP_LEGENDS_SYSTEM_PROXY")
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv("DEEP_LEGENDS_SYSTEM_PROXY", previous)
		} else {
			_ = os.Unsetenv("DEEP_LEGENDS_SYSTEM_PROXY")
		}
	})
	if err := os.Setenv("DEEP_LEGENDS_SYSTEM_PROXY", "http://127.0.0.1:7890"); err != nil {
		t.Fatal(err)
	}
	proxy, active, err := championProxyFor(defaultChampionNetworkSettings())
	if err != nil || proxy == nil || active != "http://127.0.0.1:7890" {
		t.Fatalf("unexpected auto proxy: %q %v", active, err)
	}
	request, _ := http.NewRequest(http.MethodGet, "https://lol-api-champion.op.gg/", nil)
	resolved, err := proxy(request)
	if err != nil || resolved.String() != "http://127.0.0.1:7890" {
		t.Fatalf("unexpected resolved proxy: %v %v", resolved, err)
	}
}

func TestSystemProxyEndpoint(t *testing.T) {
	previous, existed := os.LookupEnv("DEEP_LEGENDS_SYSTEM_PROXY")
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv("DEEP_LEGENDS_SYSTEM_PROXY", previous)
		} else {
			_ = os.Unsetenv("DEEP_LEGENDS_SYSTEM_PROXY")
		}
	})
	a := &app{}
	post := func(body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/system-proxy", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		a.handleSystemProxy(recorder, request)
		return recorder
	}
	if recorder := post(`{"proxy":"http://127.0.0.1:7890"}`); recorder.Code != http.StatusOK {
		t.Fatalf("valid proxy rejected: %d %s", recorder.Code, recorder.Body.String())
	}
	if value := os.Getenv("DEEP_LEGENDS_SYSTEM_PROXY"); value != "http://127.0.0.1:7890" {
		t.Fatalf("proxy env not applied: %q", value)
	}
	for _, body := range []string{`{"proxy":"ftp://proxy.example:21"}`, `{"proxy":"http://user:secret@proxy.example:8080"}`, `{"proxy":"not a url"}`} {
		if recorder := post(body); recorder.Code != http.StatusBadRequest {
			t.Fatalf("invalid proxy %s accepted: %d", body, recorder.Code)
		}
	}
	if value := os.Getenv("DEEP_LEGENDS_SYSTEM_PROXY"); value != "http://127.0.0.1:7890" {
		t.Fatalf("invalid submissions must not change env: %q", value)
	}
	if recorder := post(`{"proxy":""}`); recorder.Code != http.StatusOK {
		t.Fatalf("empty proxy rejected: %d", recorder.Code)
	}
	if _, present := os.LookupEnv("DEEP_LEGENDS_SYSTEM_PROXY"); present {
		t.Fatal("empty proxy should unset env")
	}
}

func TestChampionDataCacheSingleflightAndStaleFallback(t *testing.T) {
	cache := newChampionDataCache(nil)
	var calls atomic.Int32
	loader := func(context.Context) ([]byte, error) {
		calls.Add(1)
		time.Sleep(15 * time.Millisecond)
		return []byte(`{"ok":true}`), nil
	}
	const clients = 20
	var group sync.WaitGroup
	fetchedTimes := make(chan time.Time, clients)
	group.Add(clients)
	for range clients {
		go func() {
			defer group.Done()
			result, err := cache.loadWithStatus(context.Background(), "same", time.Minute, time.Hour, false, loader)
			if err != nil || string(result.data) != `{"ok":true}` {
				t.Errorf("load failed: %s %v", result.data, err)
				return
			}
			fetchedTimes <- result.fetchedAt
		}()
	}
	group.Wait()
	close(fetchedTimes)
	if calls.Load() != 1 {
		t.Fatalf("loader called %d times", calls.Load())
	}
	var sharedFetchedAt time.Time
	for fetchedAt := range fetchedTimes {
		if fetchedAt.IsZero() {
			t.Fatal("singleflight result has zero fetchedAt")
		}
		if sharedFetchedAt.IsZero() {
			sharedFetchedAt = fetchedAt
		} else if !fetchedAt.Equal(sharedFetchedAt) {
			t.Fatalf("singleflight fetchedAt = %v, want %v", fetchedAt, sharedFetchedAt)
		}
	}

	cache.mu.Lock()
	entry := cache.entries["same"]
	entry.ExpiresAt = time.Now().Add(-time.Minute)
	entry.StaleUntil = time.Now().Add(time.Hour)
	cache.entries["same"] = entry
	cache.mu.Unlock()
	data, err := cache.load(context.Background(), "same", time.Minute, time.Hour, false, func(context.Context) ([]byte, error) { return nil, errors.New("upstream down") })
	if err != nil || string(data) != `{"ok":true}` {
		t.Fatalf("stale fallback failed: %s %v", data, err)
	}
}

func TestChampionDataCacheReportsLoadState(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, championDataCacheDirectory), 0o700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	key := championCacheKey(dataDragonHost, "/api/versions.json", "", "application/json")
	first := newChampionDataCache(store)
	result, err := first.loadWithStatus(context.Background(), key, time.Hour, time.Hour, true, func(context.Context) ([]byte, error) {
		return []byte(`{"cached":true}`), nil
	})
	if err != nil || result.state != championCacheStateMiss {
		t.Fatalf("initial cache state = %q, err=%v", result.state, err)
	}
	initialFetchedAt := result.fetchedAt
	if initialFetchedAt.IsZero() {
		t.Fatal("initial cache fetchedAt is zero")
	}
	result, err = first.loadWithStatus(context.Background(), key, time.Hour, time.Hour, true, func(context.Context) ([]byte, error) {
		return nil, errors.New("memory cache missed")
	})
	if err != nil || result.state != championCacheStateMemory {
		t.Fatalf("memory cache state = %q, err=%v", result.state, err)
	}
	if !result.fetchedAt.Equal(initialFetchedAt) {
		t.Fatalf("memory cache fetchedAt = %v, want %v", result.fetchedAt, initialFetchedAt)
	}
	second := newChampionDataCache(store)
	result, err = second.loadWithStatus(context.Background(), key, time.Hour, time.Hour, true, func(context.Context) ([]byte, error) {
		return nil, errors.New("disk cache missed")
	})
	if err != nil || result.state != championCacheStateDisk {
		t.Fatalf("disk cache state = %q, err=%v", result.state, err)
	}
	if !result.fetchedAt.Equal(initialFetchedAt) {
		t.Fatalf("disk cache fetchedAt = %v, want %v", result.fetchedAt, initialFetchedAt)
	}
	second.mu.Lock()
	entry := second.entries[key]
	entry.ExpiresAt = time.Now().Add(-time.Minute)
	entry.StaleUntil = time.Now().Add(time.Hour)
	second.entries[key] = entry
	second.mu.Unlock()
	result, err = second.loadWithStatus(context.Background(), key, time.Hour, time.Hour, false, func(context.Context) ([]byte, error) {
		return nil, errors.New("upstream unavailable")
	})
	if err != nil || result.state != championCacheStateStale || result.upstreamErr == nil {
		t.Fatalf("stale cache state = %q, upstreamErr=%v, err=%v", result.state, result.upstreamErr, err)
	}
	if !result.fetchedAt.Equal(initialFetchedAt) {
		t.Fatalf("stale cache fetchedAt = %v, want %v", result.fetchedAt, initialFetchedAt)
	}
	result, err = second.loadWithStatus(context.Background(), "uncached-error", time.Hour, 0, false, func(context.Context) ([]byte, error) {
		return nil, errors.New("upstream unavailable")
	})
	if err == nil || result.state != championCacheStateError {
		t.Fatalf("error cache state = %q, err=%v", result.state, err)
	}
	if !result.fetchedAt.IsZero() {
		t.Fatalf("error cache fetchedAt = %v, want zero", result.fetchedAt)
	}
}

func TestStructuredDetailPreservesStaleCacheFetchedAt(t *testing.T) {
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(nil)
	provider.static["sentinel"] = championAssetDescription{}
	provider.client = &http.Client{Transport: championRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("upstream unavailable")
	})}

	key := opggDetailCacheKey("ranked", "KR", 67, "MID", "emerald_plus", "fallback-16.16.1")
	fetchedAt := time.Now().Add(-24 * time.Hour).Round(0)
	payload := []byte(`{"data":{"summary":{"id":67,"average_stats":{"play":100,"win":60}},"core_items":[{"ids":[3153],"play":100,"win":60}]},"meta":{"version":"16.16"}}`)
	provider.cache.mu.Lock()
	provider.cache.storeMemoryLocked(key, championCacheEnvelope{
		Key: key, FetchedAt: fetchedAt, ExpiresAt: time.Now().Add(-time.Hour), StaleUntil: time.Now().Add(time.Hour), Data: payload,
	})
	provider.cache.mu.Unlock()

	provider.patch = "16.16.1"
	response, err := provider.loadStructuredDetail(context.Background(), "ranked", "67", "mid", "emerald_plus")
	if err != nil {
		t.Fatal(err)
	}
	if !response.FetchedAt.Equal(fetchedAt) {
		t.Fatalf("structured detail fetchedAt = %v, want stale cache time %v", response.FetchedAt, fetchedAt)
	}
}

func TestOPGGModeSpecURLMatrix(t *testing.T) {
	tests := []struct {
		mode, position, wantPath, wantQuery, wantAPIMode, wantRegion string
	}{
		{"ranked", "middle", "/api/KR/champions/ranked/99/MID", "tier=emerald_plus", "ranked", "KR"},
		{"aram", "mid", "/api/KR/champions/aram/99/none", "", "aram", "KR"},
		{"arena", "mid", "/api/global/champions/arena/99", "", "arena", "global"},
		{"urf", "top", "/api/KR/champions/urf/99/none", "", "urf", "KR"},
		{"nexus-blitz", "jungle", "/api/KR/champions/nexus_blitz/99/none", "", "nexus_blitz", "KR"},
	}
	for _, test := range tests {
		t.Run(test.mode, func(t *testing.T) {
			spec, path, query, err := opggDetailRequest(test.mode, 99, test.position, "emerald_plus")
			if err != nil || path != test.wantPath || query.Encode() != test.wantQuery || spec.APIMode != test.wantAPIMode || spec.Region != test.wantRegion {
				t.Fatalf("request=(%q,%q,%#v,%v), want path=%q query=%q mode=%q region=%q", path, query.Encode(), spec, err, test.wantPath, test.wantQuery, test.wantAPIMode, test.wantRegion)
			}
		})
	}
	if _, _, _, err := opggDetailRequest("hextech-aram", 99, "support", ""); err == nil {
		t.Fatal("hextech ARAM must not use the OP.GG champion JSON endpoint")
	}
}

func TestOPGGPatchFreshnessUsesNumericSegments(t *testing.T) {
	tests := []struct {
		data  string
		stale bool
	}{
		{"16.16", false},
		{"16.14", false},
		{"16.04", true},
		{"16.4", true},
		{"13.23", true},
		{"", true},
	}
	for _, test := range tests {
		if got := opggPatchIsStale("16.16.1", test.data); got != test.stale {
			t.Fatalf("data version %q stale=%v, want %v", test.data, got, test.stale)
		}
	}
}

func TestOPGGDetailCacheIdentityIncludesModeAndRegion(t *testing.T) {
	ranked := opggDetailCacheKey("ranked", "KR", 99, "MID", "emerald_plus", "16.17")
	previousVersion := opggDetailCacheKey("ranked", "KR", 99, "MID", "emerald_plus", "16.16")
	aram := opggDetailCacheKey("aram", "KR", 99, "none", "emerald_plus", "16.17")
	arena := opggDetailCacheKey("arena", "global", 99, "", "", "16.17")
	for _, part := range []string{"ranked", "KR", "99", "MID", "emerald_plus"} {
		if !strings.Contains(ranked, part) {
			t.Fatalf("ranked cache key %q is missing %q", ranked, part)
		}
	}
	if ranked == previousVersion || ranked == aram || ranked == arena || aram == arena {
		t.Fatalf("mode cache identities collided: %q %q %q", ranked, aram, arena)
	}
}

func TestStructuredMetricsRejectsTinyAndRelativeLongTailSamples(t *testing.T) {
	provider := newChampionProvider()
	provider.patch = "16.16.1"
	rows := provider.structuredMetrics([]opggMetric{
		{IDs: []int{1001}, Play: 10000, Win: 5000, PickRate: 0.9},
		{IDs: []int{1002}, Play: 99, Win: 55, PickRate: 0.009},
		{IDs: []int{1003}, Play: 100, Win: 52, PickRate: 0.01},
		{IDs: []int{1004}, Play: 49, Win: 20, PickRate: 0.004},
	}, "item", 10)
	if len(rows) != 2 || rows[0].Assets[0].ID != 1001 || rows[1].Assets[0].ID != 1003 {
		t.Fatalf("sample gate rows = %#v", rows)
	}
}

func TestStructuredMetricsFallbackKeepsHighestSamplesAndReportsGate(t *testing.T) {
	provider := newChampionProvider()
	provider.patch = "16.16.1"
	events := make([]map[string]any, 0, 1)
	provider.diag = func(event map[string]any) { events = append(events, event) }
	rows := provider.structuredMetricsForKind([]opggMetric{
		{IDs: []int{1001}, Play: 49, Win: 26},
		{IDs: []int{1002}, Play: 31, Win: 15},
		{IDs: []int{1003}, Play: 12, Win: 7},
	}, "item", "core", 2)
	if len(rows) != 2 || rows[0].Assets[0].ID != 1001 || rows[1].Assets[0].ID != 1002 {
		t.Fatalf("fallback rows = %#v", rows)
	}
	if len(events) != 1 {
		t.Fatalf("gate events = %#v", events)
	}
	event := events[0]
	if event["event"] != "structured_metrics_gate" || event["kind"] != "core" || event["in"] != 3 || event["out"] != 0 || event["leading"] != 49 || event["dropped_below_floor"] != 3 || event["returned"] != 2 || event["fallback"] != true {
		t.Fatalf("gate diagnostic = %#v", event)
	}
}

func TestStructuredRecommendationMetricsPrioritizeLargeSamplesBeforeWinRate(t *testing.T) {
	provider := newChampionProvider()
	provider.patch = "16.16.1"
	rows := provider.structuredRecommendationMetrics([]opggMetric{
		{IDs: []int{1001}, Play: 2, Win: 2},
		{IDs: []int{1002}, Play: 5000, Win: 2650},
	}, "item", "starter", 2)
	if len(rows) != 2 {
		t.Fatalf("recommendation rows = %d, want 2", len(rows))
	}
	for index, want := range []int{1002, 1001} {
		if rows[index].Assets[0].ID != want {
			t.Fatalf("recommendation %d = %d, want %d", index, rows[index].Assets[0].ID, want)
		}
	}
}

func TestStructuredRecommendationMetricsFollowPickRateAcrossEveryOPGGRecommendationKind(t *testing.T) {
	provider := newChampionProvider()
	fixture := []opggMetric{
		{IDs: []int{1001}, Play: 500, Win: 350, PickRate: 0.10},
		{IDs: []int{1002}, Play: 900, Win: 450, PickRate: 0.30},
		{IDs: []int{1003}, Play: 700, Win: 420, PickRate: 0.20},
	}
	for _, kind := range []string{"spell", "starter", "boots", "core"} {
		t.Run(kind, func(t *testing.T) {
			rows := provider.structuredRecommendationMetrics(fixture, "item", kind, 3)
			if len(rows) != 3 {
				t.Fatalf("%s rows = %d, want 3", kind, len(rows))
			}
			got := []int{rows[0].Assets[0].ID, rows[1].Assets[0].ID, rows[2].Assets[0].ID}
			want := []int{1002, 1003, 1001}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%s recommendation order = %v, want pick-rate order %v", kind, got, want)
			}
		})
	}
}

func TestRankedDepthCacheSharesDetailSnapshotAndUsesOlderFetchedAt(t *testing.T) {
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(nil)
	detailFetchedAt := time.Date(2026, 9, 3, 6, 0, 0, 123, time.UTC)
	depthFetchedAt := detailFetchedAt.Add(-45 * time.Minute)
	cacheKey := strings.Join([]string{
		"v3", "opgg-depth", "jax", "top", "emerald_plus", "16.17",
		detailFetchedAt.Format(time.RFC3339Nano),
	}, "|")
	depthPayload := []byte(`
1:["$","tr",null,{"children":["depth_4_item_0",{"metaType":"item","metaId":6333},{"children":["61.19","%"]},{"children":"572 场"}]}]
2:["$","tr",null,{"children":["depth_5_item_0",{"metaType":"item","metaId":3026},{"children":["57.14","%"]},{"children":"49 场"}]}]`)
	provider.cache.entries[cacheKey] = championCacheEnvelope{
		Schema: championCacheSchema, Key: cacheKey, Data: depthPayload, FetchedAt: depthFetchedAt,
		ExpiresAt: time.Now().Add(time.Hour), StaleUntil: time.Now().Add(2 * time.Hour),
	}
	provider.cache.order = []string{cacheKey}
	provider.cache.bytes = len(depthPayload)
	response := championDetailResponse{FetchedAt: detailFetchedAt}
	provider.resolveRankedItemDepths(context.Background(), "jax", "top", "emerald_plus", "16.17", nil, "", errors.New("unused"), false, false, &response)
	if !response.FetchedAt.Equal(depthFetchedAt) {
		t.Fatalf("combined detail FetchedAt = %s, want older depth snapshot %s", response.FetchedAt, depthFetchedAt)
	}
	if response.Build.ItemChainStatus != "ready" || len(response.Build.FourthItems) != 1 || len(response.Build.FifthItems) != 1 {
		t.Fatalf("cached depth payload was not used: %#v", response.Build)
	}
}

func TestStructuredRecommendationMetricsKeepUnknownSamplesInUpstreamOrder(t *testing.T) {
	candidates := []structuredMetricCandidate{
		{row: championMetricRow{Assets: []championAsset{{ID: 2001}}, WinRate: 48, GamesUnavailable: true}},
		{row: championMetricRow{Assets: []championAsset{{ID: 2002}}, WinRate: 99, GamesUnavailable: true}},
		{row: championMetricRow{Assets: []championAsset{{ID: 2003}}, WinRate: 60, GamesUnavailable: true}},
	}
	orderStructuredRecommendationCandidates(candidates)
	for index, want := range []int{2001, 2002, 2003} {
		if candidates[index].row.Assets[0].ID != want {
			t.Fatalf("unknown-sample row %d = %d, want %d", index, candidates[index].row.Assets[0].ID, want)
		}
	}
}

func TestStructuredSampleAllowedRequiresAbsoluteAndRelativeMinimums(t *testing.T) {
	tests := []struct {
		name          string
		play          int
		leadingSample int
		want          bool
	}{
		{name: "both boundaries", play: 50, leadingSample: 5000, want: true},
		{name: "below absolute minimum", play: 49, leadingSample: 4900, want: false},
		{name: "below head ratio", play: 50, leadingSample: 5001, want: false},
		{name: "larger sample below head ratio", play: 200, leadingSample: 20001, want: false},
		{name: "representative sample", play: 200, leadingSample: 10000, want: true},
		{name: "zero leading sample", play: 50, leadingSample: 0, want: false},
		{name: "negative leading sample", play: 50, leadingSample: -1, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := structuredSampleAllowed(test.play, test.leadingSample); got != test.want {
				t.Fatalf("structuredSampleAllowed(%d, %d) = %v, want %v", test.play, test.leadingSample, got, test.want)
			}
		})
	}
}

func TestStructuredSpellSampleGateIsRelaxedOnlyForSpells(t *testing.T) {
	if !structuredSampleAllowedForKind("spell", 18, 6078) {
		t.Fatal("18-game spell row should pass the spell-only gate")
	}
	if structuredSampleAllowedForKind("spell", 14, 6078) {
		t.Fatal("spell row below the 15-game floor should fail")
	}
	if structuredSampleAllowedForKind("core", 18, 6078) {
		t.Fatal("spell relaxation leaked into core item metrics")
	}
	if !structuredSampleAllowedForKind("core", 100, 10000) {
		t.Fatal("established core item gate changed")
	}
}

func TestChampionUpstreamDiagnosticUsesSafeFields(t *testing.T) {
	const responseBody = `{"summonerId":"stable-private-account-id"}`
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(nil)
	events := make([]map[string]any, 0, 3)
	provider.diag = func(event map[string]any) { events = append(events, event) }
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		status := http.StatusOK
		body := responseBody
		if strings.Contains(request.URL.Path, "forbidden-private-path") {
			status, body = http.StatusForbidden, "private upstream body"
		}
		return &http.Response{
			StatusCode: status, Status: http.StatusText(status), Header: make(http.Header),
			Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: request,
		}, nil
	})}
	for range 2 {
		data, err := provider.fetch(context.Background(), dataDragonHost, "/cdn/private-account-path.json", nil, championJSONMax, "application/json")
		if err != nil || string(data) != responseBody {
			t.Fatalf("diagnostic fetch failed: %s %v", data, err)
		}
	}
	if _, err := provider.fetch(context.Background(), dataDragonHost, "/cdn/forbidden-private-path.json", nil, championJSONMax, "application/json"); err == nil {
		t.Fatal("expected HTTP 403 failure")
	}
	if len(events) != 3 {
		t.Fatalf("diagnostic event count = %d", len(events))
	}
	wantCache := []string{championCacheStateMiss, championCacheStateMemory, championCacheStateError}
	wantStatus := []int{http.StatusOK, http.StatusOK, http.StatusForbidden}
	allowed := map[string]bool{"event": true, "host": true, "status": true, "duration_ms": true, "bytes": true, "cache": true}
	for index, event := range events {
		if len(event) != len(allowed) || event["event"] != "champion_upstream" || event["host"] != dataDragonHost || event["cache"] != wantCache[index] || event["status"] != wantStatus[index] {
			t.Fatalf("unexpected upstream diagnostic %d: %#v", index, event)
		}
		for key := range event {
			if !allowed[key] {
				t.Fatalf("unexpected upstream diagnostic field %q", key)
			}
		}
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"stable-private-account-id", "private-account-path", "forbidden-private-path", "private upstream body", "summonerId"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("upstream diagnostic leaked %q: %s", secret, encoded)
		}
	}
}

func TestChampionDataCachePersistsAcrossInstances(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, championDataCacheDirectory), 0o700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	first := newChampionDataCache(store)
	key := championCacheKey(dataDragonHost, "/api/versions.json", "", "application/json")
	data, err := first.load(context.Background(), key, time.Hour, time.Hour, true, func(context.Context) ([]byte, error) { return []byte(`{"cached":true}`), nil })
	if err != nil || len(data) == 0 {
		t.Fatalf("initial cache write failed: %v", err)
	}
	second := newChampionDataCache(store)
	var calls atomic.Int32
	data, err = second.load(context.Background(), key, time.Hour, time.Hour, true, func(context.Context) ([]byte, error) { calls.Add(1); return nil, errors.New("should not load") })
	if err != nil || string(data) != `{"cached":true}` || calls.Load() != 0 {
		t.Fatalf("disk cache miss: %s calls=%d err=%v", data, calls.Load(), err)
	}
}

func TestChampionCachePersistentPoliciesUseDiskWhitelistedKeys(t *testing.T) {
	cases := []struct {
		name, host, requestPath, accept, key string
	}{
		{"Data Dragon", dataDragonHost, "/api/versions.json", "application/json", championCacheKey(dataDragonHost, "/api/versions.json", "", "application/json")},
		{"Community Dragon", communityDragonHost, "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/cherry-augments.json", "application/json", championCacheKey(communityDragonHost, "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/cherry-augments.json", "", "application/json")},
		{"OP.GG page", opggPageHost, "/lol/modes/aram-mayhem/ahri/build", "text/html,application/xhtml+xml", championCacheKey(opggPageHost, "/lol/modes/aram-mayhem/ahri/build", "", "text/html,application/xhtml+xml")},
		{"OP.GG champion", opggChampionHost, "/api/KR/champions/ranked", "application/json", championCacheKey(opggChampionHost, "/api/KR/champions/ranked", "", "application/json")},
		{"OP.GG detail", opggChampionHost, "/api/KR/champions/ranked/103/MID", "application/json", opggDetailCacheKey("ranked", "KR", 103, "MID", "emerald_plus", "16.17")},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, _, persistDisk := championCachePolicy(test.host, test.requestPath, test.accept)
			if !persistDisk {
				t.Fatalf("cache policy no longer declares %q persistent", test.name)
			}
			if !championCacheDiskAllowed(test.key) {
				t.Fatalf("persistent %s key is not disk-whitelisted: %q", test.name, test.key)
			}
		})
	}
}

func TestChampionDataCacheKeepsYourGGMemoryOnly(t *testing.T) {
	root := t.TempDir()
	cacheDir := filepath.Join(root, championDataCacheDirectory)
	if err := os.Mkdir(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	cache := newChampionDataCache(store)
	key := championCacheKey(yourGGArenaHost, "/kr/api/arena/champions/799/top-builds", "limit=10", "application/json")
	loaderCalls := 0
	loader := func(context.Context) ([]byte, error) {
		loaderCalls++
		return []byte(`{"summonerId":"must-not-reach-disk"}`), nil
	}
	for range 2 {
		data, err := cache.load(context.Background(), key, 20*time.Minute, 0, false, loader)
		if err != nil || len(data) == 0 {
			t.Fatalf("YOUR.GG memory cache failed: %v", err)
		}
	}
	if loaderCalls != 1 {
		t.Fatalf("YOUR.GG memory cache loader calls = %d", loaderCalls)
	}
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("YOUR.GG wrote %d disk cache files", len(entries))
	}

	restarted := newChampionDataCache(store)
	_, err = restarted.load(context.Background(), key, 20*time.Minute, 0, false, func(context.Context) ([]byte, error) {
		return []byte(`{"fresh":true}`), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, err = os.ReadDir(cacheDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("restarted YOUR.GG cache touched disk: entries=%d err=%v", len(entries), err)
	}
}

func TestChampionDataCachePurgesLegacyYourGGDiskEntry(t *testing.T) {
	root := t.TempDir()
	cacheDir := filepath.Join(root, championDataCacheDirectory)
	if err := os.Mkdir(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	key := championCacheKey(yourGGArenaHost, "/kr/api/arena/champions/799/top-builds", "limit=10", "application/json")
	body := []byte(`{"summonerId":"legacy-private-id"}`)
	hash := sha256.Sum256(body)
	entry := championCacheEnvelope{
		Schema: championCacheSchema, Key: key, FetchedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
		StaleUntil: time.Now().Add(time.Hour), Hash: hex.EncodeToString(hash[:]), Data: body,
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	seed := &championDataCache{dir: cacheDir}
	path := seed.pathFor(key)
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	_ = newChampionDataCache(trackTestStore(t, &localStore{root: root}))
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy YOUR.GG cache was not removed: %v", err)
	}
}

func TestStructuredOPGGDetailSchema(t *testing.T) {
	fixture := []byte(`{"data":{"summary":{"id":67,"average_stats":{"play":100,"win":55,"total_place":320,"first_place":20,"pick_rate":0.14,"ban_rate":0.39},"positions":[{"name":"SUPPORT","stats":{"play":70,"win_rate":0.51,"pick_rate":0.07,"ban_rate":0.02,"role_rate":0.70,"tier_data":{"tier":2,"rank":4}}},{"name":"TOP","stats":{"play":30,"win_rate":0.49,"pick_rate":0.03,"ban_rate":0.01,"role_rate":0.30,"tier":3,"rank":9}},{"name":"TOP2","stats":{"play":999,"win_rate":0.99,"role_rate":0.99}}]},"summoner_spells":[{"ids":[4,21],"play":80,"win":44,"pick_rate":0.8}],"skill_masteries":[{"ids":["Q","W","E"],"play":90,"win":50,"pick_rate":0.9,"builds":[{"order":["Q","W","E","Q","Q","R"],"play":70,"win":42,"pick_rate":0.77}]}],"core_items":[{"ids":[3153,3124,3302],"play":60,"win":36,"pick_rate":0.3}],"augment_group":[{"rarity":8,"augments":[{"id":225,"play":50,"win":35,"pick_rate":0.18,"win_rate":0.7}]}],"synergies":[{"champion_id":350,"play":30,"win":24,"total_place":70,"first_place":12,"pick_rate":0.03}]},"meta":{"version":"16.15","cached_at":"2026-08-11 21:53:48"}}`)
	var payload opggStructuredDetail
	if err := json.Unmarshal(fixture, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Summary.ID != 67 || len(payload.Data.SkillMasteries) != 1 || payload.Data.SkillMasteries[0].IDs[0] != "Q" {
		t.Fatalf("skill schema mismatch: %#v", payload.Data.SkillMasteries)
	}
	if payload.Data.AugmentGroup[0].Augments[0].ID != 225 || payload.Data.Synergies[0].ChampionID != 350 {
		t.Fatal("arena schema mismatch")
	}
	if len(payload.Data.Summary.Positions) != 3 || payload.Data.Summary.Positions[0].Stats.RoleRate != 0.70 {
		t.Fatalf("position schema mismatch: %#v", payload.Data.Summary.Positions)
	}
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(nil)
	provider.patch = "16.15.1"
	provider.static["item/3153.png"] = championAssetDescription{Name: "破败王者之刃"}
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := fixture
		switch request.URL.Host {
		case opggChampionHost:
			if request.URL.Path != "/api/KR/champions/ranked/67/SUPPORT" || request.URL.Query().Get("tier") != "emerald_plus" {
				return nil, fmt.Errorf("unexpected structured detail request: %s", request.URL.String())
			}
		case opggPageHost:
			body = []byte(`<html></html>`)
		case dataDragonHost:
			body = []byte(`[]`)
		default:
			return nil, fmt.Errorf("unexpected host: %s", request.URL.Host)
		}
		return &http.Response{StatusCode: http.StatusOK, Status: http.StatusText(http.StatusOK), Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body))), ContentLength: int64(len(body)), Request: request}, nil
	})}
	ranked, err := provider.loadStructuredDetail(context.Background(), "ranked", "67", "support", "emerald_plus")
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(ranked.Stats.WinRate-55) > 1e-9 || math.Abs(ranked.Stats.PickRate-14) > 1e-9 || math.Abs(ranked.Stats.BanRate-39) > 1e-9 {
		t.Fatalf("summary percentage conversion mismatch: %#v", ranked.Stats)
	}
	if len(ranked.Positions) != 2 || ranked.Positions[0].Position != "support" || ranked.Positions[1].Position != "top" {
		t.Fatalf("position whitelist/order mismatch: %#v", ranked.Positions)
	}
	if got := ranked.Positions[0]; math.Abs(got.WinRate-51) > 1e-9 || math.Abs(got.PickRate-7) > 1e-9 || math.Abs(got.BanRate-2) > 1e-9 || math.Abs(got.RoleRate-70) > 1e-9 || got.Tier != 2 || got.Rank != 4 {
		t.Fatalf("support percentage conversion mismatch: %#v", got)
	}
	if got := ranked.Positions[1]; math.Abs(got.WinRate-49) > 1e-9 || math.Abs(got.PickRate-3) > 1e-9 || math.Abs(got.BanRate-1) > 1e-9 || math.Abs(got.RoleRate-30) > 1e-9 || got.Tier != 3 || got.Rank != 9 {
		t.Fatalf("top percentage conversion mismatch: %#v", got)
	}
	if len(ranked.Build.CoreItems) != 1 || len(ranked.Build.CoreItems[0].Assets) != 3 || ranked.Build.CoreItems[0].WinRate != 60 {
		t.Fatalf("metric conversion mismatch: %#v", ranked.Build.CoreItems)
	}
	if len(ranked.Build.Skills) != 1 || len(ranked.Build.Skills[0].SkillOrder) != 6 || ranked.Build.Skills[0].WinRate != 60 {
		t.Fatalf("skill conversion mismatch: %#v", ranked.Build.Skills)
	}
}

func TestStructuredDetailOmitsPositionsOutsideRanked(t *testing.T) {
	tests := []struct {
		mode, wantPath string
	}{
		{"arena", "/api/global/champions/arena/67"},
		{"aram", "/api/KR/champions/aram/67/none"},
		{"urf", "/api/KR/champions/urf/67/none"},
		{"nexus-blitz", "/api/KR/champions/nexus_blitz/67/none"},
	}
	for _, test := range tests {
		t.Run(test.mode, func(t *testing.T) {
			provider := newChampionProvider()
			provider.cache = newChampionDataCache(nil)
			provider.patch = "16.16.1"
			provider.static["item/3153.png"] = championAssetDescription{Name: "破败王者之刃"}
			requestedPath := ""
			provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.URL.Host == opggPageHost {
					t.Fatalf("%s mode unexpectedly requested ranked item depths: %s", test.mode, request.URL.String())
				}
				if request.URL.Host == opggChampionHost {
					requestedPath = request.URL.Path
				}
				body := `{"data":{"summary":{"id":67,"average_stats":{"play":100,"win_rate":0.5},"positions":[{"name":"SUPPORT","stats":{"play":70,"win_rate":0.51,"role_rate":0.7}}]},"core_items":[{"ids":[3153],"play":70,"win":36}]},"meta":{"version":"16.16"}}`
				return &http.Response{StatusCode: http.StatusOK, Status: http.StatusText(http.StatusOK), Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: request}, nil
			})}

			response, err := provider.loadStructuredDetail(context.Background(), test.mode, "67", "support", "")
			if err != nil {
				t.Fatal(err)
			}
			if requestedPath != test.wantPath {
				t.Fatalf("detail path = %q, want %q", requestedPath, test.wantPath)
			}
			if len(response.Positions) != 0 {
				t.Fatalf("non-ranked detail leaked positions: %#v", response.Positions)
			}
			if response.Build.ItemChainStatus != "" || len(response.Build.FourthItems)+len(response.Build.FifthItems)+len(response.Build.SixthItems) != 0 {
				t.Fatalf("non-ranked detail advertised unavailable item depths: %#v", response.Build)
			}
		})
	}
}

func TestStructuredDetailFallbackKeepsFallbackPositions(t *testing.T) {
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(nil)
	provider.patch = "16.16.1"
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == opggChampionHost && request.URL.Query().Get("tier") == "diamond" {
			return &http.Response{StatusCode: http.StatusBadGateway, Status: http.StatusText(http.StatusBadGateway), Header: make(http.Header), Body: io.NopCloser(strings.NewReader("upstream failure")), ContentLength: int64(len("upstream failure")), Request: request}, nil
		}
		body := `{"data":{"summary":{"id":67,"average_stats":{"play":100,"win_rate":0.5},"positions":[{"name":"TOP","stats":{"play":55,"win_rate":0.52,"pick_rate":0.04,"ban_rate":0.02,"role_rate":0.55,"tier_data":{"tier":3,"rank":8}}},{"name":"SUPPORT","stats":{"play":91,"win_rate":0.51,"pick_rate":0.07,"ban_rate":0.02,"role_rate":0.91,"tier_data":{"tier":2,"rank":4}}},{"name":"TOP2","stats":{"play":999,"win_rate":0.99,"role_rate":0.99}}]},"core_items":[{"ids":[3153],"play":91,"win":48}]},"meta":{"version":"16.16"}}`
		return &http.Response{StatusCode: http.StatusOK, Status: http.StatusText(http.StatusOK), Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: request}, nil
	})}

	response, err := provider.loadDetail(context.Background(), "ranked", "67", "support", "diamond")
	if err != nil {
		t.Fatal(err)
	}
	if response.SampleTier != championCounterFallbackTier || len(response.Positions) != 2 || response.Positions[0].Position != "top" || response.Positions[1].Position != "support" || math.Abs(response.Positions[0].RoleRate-55) > 1e-9 || math.Abs(response.Positions[1].RoleRate-91) > 1e-9 {
		t.Fatalf("fallback positions did not follow fallback response: tier=%q positions=%#v", response.SampleTier, response.Positions)
	}
}

func TestArenaAugmentRowsUseCommunityDragonMetadata(t *testing.T) {
	groups := []opggArenaAugmentGroup{{Rarity: 8, Augments: []opggAugmentMetric{{ID: 225, Play: 50, Win: 35, PickRate: 0.18, WinRate: 0.7}}}}
	catalog := []gameplayAugment{{ID: 225, Name: "中文海克斯", Description: "中文说明", IconPath: "/lol-game-data/assets/ASSETS/Maps/Cherry/Augments/Icons/test.png"}}
	rows := arenaAugmentRows(groups, catalog)
	if len(rows) != 1 || len(rows[0].Assets) != 1 {
		t.Fatalf("arena augment rows = %#v", rows)
	}
	asset := rows[0].Assets[0]
	if asset.Name != "中文海克斯" || asset.Source != "communitydragon" || asset.Path != "/latest/game/assets/maps/cherry/augments/icons/test.png" || rows[0].Rarity != "prismatic" {
		t.Fatalf("arena augment metadata = %#v row=%#v", asset, rows[0])
	}
	if rows[0].PickRate != 18 || rows[0].WinRate != 70 || rows[0].Games != 50 {
		t.Fatalf("arena augment metrics = %#v", rows[0])
	}
}

func TestArenaAugmentMetadataSupplementOnlyFillsMissingFields(t *testing.T) {
	base := []gameplayAugment{{ID: 93, Name: "客户端名称", Rarity: "kSilver", IconPath: "/lol-game-data/assets/base.png"}, {ID: 89, Name: "已有", Description: "客户端说明"}}
	supplement := []gameplayAugment{{ID: 93, Name: "远端名称", Description: "远端说明", Rarity: "kGold", IconPath: "/lol-game-data/assets/arena.png"}, {ID: 89, Description: "不应覆盖"}, {ID: 225, Name: "新增", Description: "新增说明"}}
	merged := mergeGameplayAugmentMetadata(base, supplement)
	if len(merged) != 3 {
		t.Fatalf("merged augments = %#v", merged)
	}
	if merged[0].Name != "客户端名称" || merged[0].Description != "远端说明" || merged[0].Rarity != "kSilver" || merged[0].IconPath != "/lol-game-data/assets/base.png" {
		t.Fatalf("base metadata was overwritten: %#v", merged[0])
	}
	if merged[1].Description != "客户端说明" || merged[2].ID != 225 || merged[2].Description != "新增说明" {
		t.Fatalf("supplement merge mismatch: %#v", merged)
	}
}

func TestLoadCommunityDragonAugmentsSupplementsArenaMetadata(t *testing.T) {
	provider := newChampionProvider()
	requested := make([]string, 0, 2)
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requested = append(requested, request.URL.Path)
		var body string
		switch request.URL.Path {
		case "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/cherry-augments.json":
			body = `[{"id":93,"nameTRA":"客户端名称","rarity":"kSilver","augmentSmallIconPath":"/lol-game-data/assets/ASSETS/Maps/Cherry/base.png"}]`
		case "/latest/cdragon/arena/zh_cn.json":
			body = `{"augments":[{"id":93,"name":"远端名称","desc":"<spellName>远端说明</spellName><br>第二行","tooltip":"不应使用","iconLarge":"assets/maps/cherry/augments/icons/93.png","rarity":1},{"id":225,"name":"新增海克斯","desc":"","tooltip":"<b>备用说明</b>","iconLarge":"assets/maps/cherry/augments/icons/225.png","rarity":4},{"id":226,"name":"不安全图标","desc":"说明","iconLarge":"../secret.png","rarity":9}]}`
		default:
			t.Fatalf("unexpected CommunityDragon path: %s", request.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK, Header: make(http.Header),
			Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: request,
		}, nil
	})}

	augments, err := provider.loadCommunityDragonAugments(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wantPaths := []string{
		"/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/cherry-augments.json",
		"/latest/cdragon/arena/zh_cn.json",
	}
	if len(requested) != len(wantPaths) || requested[0] != wantPaths[0] || requested[1] != wantPaths[1] {
		t.Fatalf("CommunityDragon requests = %#v, want %#v", requested, wantPaths)
	}
	if len(augments) != 3 {
		t.Fatalf("augments = %#v", augments)
	}
	if got := augments[0]; got.ID != 93 || got.Name != "客户端名称" || got.Description != "远端说明\n第二行" || got.Rarity != "kSilver" || got.IconPath != "/lol-game-data/assets/ASSETS/Maps/Cherry/base.png" || got.FallbackIconPath != "" {
		t.Fatalf("base augment metadata = %#v", got)
	}
	if got := augments[1]; got.ID != 225 || got.Name != "新增海克斯" || got.Description != "备用说明" || got.Rarity != "event" || got.IconPath != "/lol-game-data/assets/assets/maps/cherry/augments/icons/225.png" {
		t.Fatalf("supplemented augment metadata = %#v", got)
	}
	if got := augments[2]; got.ID != 226 || got.IconPath != "" || got.Rarity != "" {
		t.Fatalf("unsafe supplement metadata was accepted: %#v", got)
	}
}

func TestArenaAugmentDescriptionRendersDataValuesAndRemovesTemplates(t *testing.T) {
	description := renderArenaAugmentDescription(`<spellName>每秒提升%i:Damage%@BonusDamage@伤害，至多 @MaxStacks@%。</spellName>`, map[string]json.RawMessage{
		"BonusDamage": json.RawMessage(`[0,1.5,3]`),
		"MaxStacks":   json.RawMessage(`[0,20,40]`),
	}, nil)
	if description != "每秒提升1.5伤害，至多20%。" {
		t.Fatalf("rendered description = %q", description)
	}
	if arenaIconPlaceholderPattern.MatchString(description) || arenaValuePlaceholderPattern.MatchString(description) {
		t.Fatalf("unrendered template leaked into description: %q", description)
	}
	if got := renderArenaAugmentDescription("<b>@MissingValue@</b>", nil, nil); got != "" {
		t.Fatalf("unknown variable should be removed, got %q", got)
	}
}

// The live client fills @f1@/@f2@ with per-round counters and
// @spell.Augment_X:Field@ points at another augment's spell block that this
// payload does not ship. Both used to survive into the UI as a stranded label
// ("这个回合的伤害提升：" with nothing after the colon) or as raw template text.
func TestArenaAugmentDescriptionDropsRuntimeCounterLines(t *testing.T) {
	description := renderArenaAugmentDescription(
		`造成<magicDamage>@Damage@魔法伤害</magicDamage>。<br><br>这个回合已造成的伤害：@f1@<br>已造成伤害的总和：@f2@`,
		map[string]json.RawMessage{"Damage": json.RawMessage(`[0,120,180]`)}, nil)
	if description != "造成120魔法伤害。" {
		t.Fatalf("runtime counter lines survived: %q", description)
	}
	crossReference := renderArenaAugmentDescription(
		`获得<speed>@MSAmount@移动速度</speed>。离开潜行后获得持续@spell.Augment_ShadowRunner:BuffDuration@秒的加速。`,
		map[string]json.RawMessage{"MSAmount": json.RawMessage(`[0,40,60]`)}, nil)
	if crossReference != "获得40移动速度。" {
		t.Fatalf("cross-augment reference survived: %q", crossReference)
	}
}

// 13 placeholders used by desc/tooltip have no dataValues entry at all; their
// real numbers live in the sibling calculations map. 巨像的勇气 is the shape
// that broke first: level interpolation plus a max-health ratio.
func TestArenaAugmentDescriptionRendersCalculations(t *testing.T) {
	dataValues := map[string]json.RawMessage{"HealthScalar": json.RawMessage(`[0.03,0.03,0.07,0.11,0.11,0.11,0.11]`)}
	calculations := map[string]json.RawMessage{"TotalShield": json.RawMessage(`{
		"__type": "GameCalculation",
		"mFormulaParts": [
			{"__type": "ByCharLevelInterpolationCalculationPart", "mEndValue": 300.0, "mStartValue": 100.0},
			{"__type": "StatByNamedDataValueCalculationPart", "mDataValue": "HealthScalar", "mStat": 12}
		]}`)}
	description := renderArenaAugmentDescription(`定身一个敌方英雄后获得<shield>@TotalShield@护盾值</shield>。`, dataValues, calculations)
	if description != "定身一个敌方英雄后获得100~300(+3%最大生命值)护盾值。" {
		t.Fatalf("calculation rendering = %q", description)
	}
}

// A percent calculation whose scaling coefficient rounds to zero must not print
// "(+0%法术强度)", and a formula that is only champion-stat scaling drops the
// parentheses so the sentence still reads as prose.
func TestArenaAugmentCalculationTrimsZeroScalingAndBareRatios(t *testing.T) {
	dataValues := map[string]json.RawMessage{
		"BaseDamageAmp": json.RawMessage(`[0.2,0.2,0.4,0.6,0.6,0.6,0.6]`),
		"APRatio":       json.RawMessage(`[0.0001,0.0001,0.0001,0.0001,0.0001,0.0001,0.0001]`),
		"HealAmount":    json.RawMessage(`[0.25,0.25,0.3,0.35,0.35,0.35,0.35]`),
	}
	calculations := map[string]json.RawMessage{
		"DamageAmp": json.RawMessage(`{"__type":"GameCalculation","mDisplayAsPercent":true,"mFormulaParts":[
			{"__type":"NamedDataValueCalculationPart","mDataValue":"BaseDamageAmp"},
			{"__type":"StatByNamedDataValueCalculationPart","mDataValue":"APRatio"}]}`),
		"HealCalc": json.RawMessage(`{"__type":"GameCalculation","mFormulaParts":[
			{"__type":"StatByNamedDataValueCalculationPart","mDataValue":"HealAmount","mStat":12}]}`),
	}
	if got := renderArenaAugmentDescription(`伤害提升@DamageAmp@。`, dataValues, calculations); got != "伤害提升20%。" {
		t.Fatalf("zero scaling term survived: %q", got)
	}
	if got := renderArenaAugmentDescription(`持续治疗@HealCalc@。`, dataValues, calculations); got != "持续治疗25%最大生命值。" {
		t.Fatalf("bare ratio rendering = %q", got)
	}
}

// Only the sentence holding the unresolvable value is dropped; the rest of the
// paragraph has to survive. Buff counters are live state and never resolvable.
func TestArenaAugmentDescriptionDropsOnlyTheUnresolvableSentence(t *testing.T) {
	dataValues := map[string]json.RawMessage{"HealthReductionPercent": json.RawMessage(`[0.1,0.1,0.15,0.2,0.2,0.2,0.2]`)}
	calculations := map[string]json.RawMessage{"MaxHealthReduction": json.RawMessage(`{"__type":"GameCalculation","mFormulaParts":[
		{"__type":"SumOfSubPartsCalculationPart","mSubparts":[
			{"__type":"NamedDataValueCalculationPart","mDataValue":"HealthReductionPercent"},
			{"__type":"BuffCounterByCoefficientCalculationPart","mBuffName":"{e0c8aaa6}","mCoefficient":1.0}]}]}`)}
	got := renderArenaAugmentDescription(
		`献祭你的@HealthReductionPercent*100@%最大生命值。总献祭量将提升至@MaxHealthReduction@%最大生命值。`,
		dataValues, calculations)
	if got != "献祭你的10%最大生命值。" {
		t.Fatalf("sentence pruning = %q", got)
	}
}

// Level breakpoint tables are walked to level 18. Pinned against 闪现向前's
// shipped Arena tooltip (35-120 magic damage from 35 + 5 per level).
func TestArenaAugmentCalculationWalksLevelBreakpoints(t *testing.T) {
	calculations := map[string]json.RawMessage{"ExplosiveDamage": json.RawMessage(`{"__type":"GameCalculation","mFormulaParts":[
		{"__type":"ByCharLevelBreakpointsCalculationPart","mInitialBonusPerLevel":5.0,"mLevel1Value":35.0}]}`)}
	if got := renderArenaAugmentDescription(`造成@ExplosiveDamage@伤害。`, nil, calculations); got != "造成35~120伤害。" {
		t.Fatalf("breakpoint walk = %q", got)
	}
}

func TestCommunityDragonMayhemCatalogShapeReportsBelowKiwiMinimum(t *testing.T) {
	events := make([]map[string]any, 0, 1)
	provider := &championProvider{diag: func(event map[string]any) { events = append(events, event) }}
	catalog := make([]gameplayAugment, communityDragonMayhemMinimumAugments-1)
	for index := range catalog {
		catalog[index] = gameplayAugment{ID: int64(1000 + index), IconPath: "/lol-game-data/assets/ASSETS/UX/Kiwi/Augments/test.png"}
	}
	provider.reportMayhemAugmentCatalogShape(catalog)
	if len(events) != 1 || events[0]["event"] != "communitydragon_mayhem_catalog_incomplete" || events[0]["actual"] != communityDragonMayhemMinimumAugments-1 || events[0]["minimum"] != communityDragonMayhemMinimumAugments {
		t.Fatalf("catalog shape diagnostic = %#v", events)
	}

	provider.reportMayhemAugmentCatalogShape(append(catalog, gameplayAugment{ID: 1119, IconPath: "/lol-game-data/assets/ASSETS/UX/Kiwi/Augments/test.png"}))
	if len(events) != 1 {
		t.Fatalf("complete Kiwi catalog emitted a diagnostic: %#v", events)
	}
}

// Icon directories are shared assets, not namespaces: Mayhem augments ship
// /Cherry/ icons and Arena's 393 ships a /Kiwi/ icon. Metadata therefore has to
// be reachable by ID no matter which directory the artwork happens to live in.
func TestCommunityDragonAugmentIndexIgnoresIconDirectory(t *testing.T) {
	catalog := []gameplayAugment{
		{ID: 1022, Name: "灵巧", IconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/Deft_small.png"},
		{ID: 393, Name: "艾卡西亚的陷落", IconPath: "/lol-game-data/assets/ASSETS/UX/Kiwi/Augments/Icons/Quest_VoidImmolation_small.png"},
		{ID: 3, Name: "无图标", IconPath: ""},
	}
	index := gameplayAugmentIndexAll(catalog)
	if len(index) != 3 {
		t.Fatalf("augment index dropped entries: %#v", index)
	}
	if index[1022].Name != "灵巧" {
		t.Fatalf("Mayhem augment with a Cherry icon was dropped: %#v", index[1022])
	}
	if index[393].Name != "艾卡西亚的陷落" {
		t.Fatalf("Arena augment with a Kiwi icon was dropped: %#v", index[393])
	}
}

func TestArenaAugmentRowsSurviveMissingCommunityDragonMetadata(t *testing.T) {
	groups := []opggArenaAugmentGroup{{Rarity: 4, Augments: []opggAugmentMetric{{ID: 412, Play: 80, Win: 44, TotalPlace: 270, FirstPlace: 20, PickRate: 0.12}}}}
	rows := arenaAugmentRows(groups, nil)
	if len(rows) != 1 || len(rows[0].Assets) != 1 {
		t.Fatalf("arena augment fallback rows = %#v", rows)
	}
	asset := rows[0].Assets[0]
	if asset.ID != 412 || asset.Name != "海克斯 412" || asset.Source != "" || asset.Path != "" {
		t.Fatalf("arena augment fallback asset = %#v", asset)
	}
	if rows[0].Rarity != "gold" || rows[0].PickRate != 12 || math.Abs(rows[0].WinRate-55) > 1e-9 || rows[0].AveragePlacement != 3.375 || rows[0].FirstPlaceRate != 25 {
		t.Fatalf("arena augment fallback metrics = %#v", rows[0])
	}
}

func TestArenaAugmentRowsPreserveAllQualityRows(t *testing.T) {
	metrics := make([]opggAugmentMetric, 16)
	for index := range metrics {
		metrics[index] = opggAugmentMetric{ID: index + 1, Play: 100 + index, Win: 50, PickRate: float64(index+1) / 100}
	}
	rows := arenaAugmentRows([]opggArenaAugmentGroup{{Rarity: 8, Augments: metrics}}, nil)
	if len(rows) != len(metrics) {
		t.Fatalf("arena augment rows were truncated: got %d, want %d", len(rows), len(metrics))
	}
}

func TestStructuredArenaDetailPreservesAllAugmentRows(t *testing.T) {
	augmentGroups := make([]map[string]any, 0, 2)
	for groupIndex, rarity := range []int{1, 8} {
		augments := make([]map[string]any, 0, 8)
		for offset := 0; offset < 8; offset++ {
			index := groupIndex*8 + offset
			augments = append(augments, map[string]any{
				"id":        7001 + index,
				"play":      100 + index,
				"win":       50 + index,
				"pick_rate": float64(index+1) / 100,
			})
		}
		augmentGroups = append(augmentGroups, map[string]any{"rarity": rarity, "augments": augments})
	}
	payload := map[string]any{
		"data": map[string]any{
			"summary": map[string]any{
				"id":            67,
				"average_stats": map[string]any{"play": 1000, "win": 550},
			},
			"core_items":    []any{map[string]any{"ids": []int{3153}, "play": 100, "win": 60}},
			"augment_group": augmentGroups,
		},
		"meta": map[string]any{"version": "16.16"},
	}
	fixture, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	aggregate := []byte(`{"success":true,"statusCode":200,"response":{"version":"16.16","coreItems":[{"itemId":3153,"tier":"S","score":1,"winRate":0.6,"averagePlacement":3,"firstPlacementRate":0.2,"pickRate":0.2,"matches":100}],"prismaticItems":[]}}`)
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(nil)
	provider.patch = "16.16.1"
	// A populated static map keeps this regression focused on the structured
	// detail funnel instead of requiring unrelated Data Dragon catalogs.
	provider.static["item/3153.png"] = championAssetDescription{Name: "测试装备"}
	seen := make(map[string]int)
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		key := request.URL.Host + request.URL.Path
		seen[key]++
		var body []byte
		switch {
		case request.URL.Host == opggChampionHost && request.URL.Path == "/api/global/champions/arena/versions":
			body = []byte(`{"data":["16.16"]}`)
		case request.URL.Host == opggChampionHost && request.URL.Path == "/api/global/champions/arena/67":
			if request.URL.Query().Get("version") != "16.16" {
				return nil, fmt.Errorf("structured arena detail version query = %q", request.URL.RawQuery)
			}
			body = fixture
		case request.URL.Host == communityDragonHost && request.URL.Path == "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/cherry-augments.json":
			body = []byte(`[]`)
		case request.URL.Host == communityDragonHost && request.URL.Path == "/latest/cdragon/arena/zh_cn.json":
			body = []byte(`{"augments":[]}`)
		case request.URL.Host == yourGGArenaHost && request.URL.Path == "/kr/api/arena/champions/67":
			body = aggregate
		case request.URL.Host == communityDragonHost && request.URL.Path == "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/items.json":
			body = []byte(`[{"id":3153,"name":"测试装备","iconPath":"/lol-game-data/assets/ASSETS/Items/3153.png"}]`)
		default:
			return nil, fmt.Errorf("unexpected structured arena request: %s", request.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK, Status: http.StatusText(http.StatusOK), Header: make(http.Header),
			Body: io.NopCloser(strings.NewReader(string(body))), ContentLength: int64(len(body)), Request: request,
		}, nil
	})}

	response, err := provider.loadStructuredDetail(context.Background(), "arena", "67", "mid", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(response.ArenaAugmentGroups) != 2 || len(response.ArenaAugmentGroups[0].Rows) != 8 || len(response.ArenaAugmentGroups[1].Rows) != 8 {
		t.Fatalf("structured arena augment groups = %#v", response.ArenaAugmentGroups)
	}
	if len(response.ArenaAugments) != 16 {
		t.Fatalf("structured arena augments were truncated: got %d, want 16", len(response.ArenaAugments))
	}
	for _, path := range []string{
		opggChampionHost + "/api/global/champions/arena/versions",
		opggChampionHost + "/api/global/champions/arena/67",
		communityDragonHost + "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/cherry-augments.json",
		communityDragonHost + "/latest/cdragon/arena/zh_cn.json",
		yourGGArenaHost + "/kr/api/arena/champions/67",
		communityDragonHost + "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/items.json",
	} {
		if seen[path] == 0 {
			t.Fatalf("structured arena request was not exercised: %s (seen=%v)", path, seen)
		}
	}
}

func TestArenaAugmentDiagnosticsUseSafeFields(t *testing.T) {
	provider := newChampionProvider()
	events := make([]map[string]any, 0, 3)
	provider.diag = func(event map[string]any) { events = append(events, event) }
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		const body = `{"private":"upstream body must not be logged"}`
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable, Status: http.StatusText(http.StatusServiceUnavailable), Header: make(http.Header),
			Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: request,
		}, nil
	})}
	groups := []opggArenaAugmentGroup{{Rarity: 4, Augments: []opggAugmentMetric{{ID: 412, Play: 80, Win: 44}}}}
	result := provider.structuredArenaAugmentGroups(context.Background(), groups)
	if len(result) != 1 || len(result[0].Rows) != 1 {
		t.Fatalf("fallback augment groups = %#v", result)
	}
	byName := make(map[string]map[string]any, len(events))
	for _, event := range events {
		name, _ := event["event"].(string)
		byName[name] = event
	}
	failed := byName["arena_augment_catalog_failed"]
	wantFailed := map[string]bool{"event": true, "source": true, "errorKind": true}
	if len(failed) != len(wantFailed) || failed["source"] != "communitydragon" || failed["errorKind"] != "http-503" {
		t.Fatalf("catalog failure diagnostic = %#v", failed)
	}
	for key := range failed {
		if !wantFailed[key] {
			t.Fatalf("unexpected catalog failure field %q", key)
		}
	}
	built := byName["arena_augments_built"]
	wantBuilt := map[string]bool{"event": true, "groupsIn": true, "augmentsIn": true, "groupsOut": true, "rowsOut": true, "missingMeta": true}
	if len(built) != len(wantBuilt) || built["groupsIn"] != 1 || built["augmentsIn"] != 1 || built["groupsOut"] != 1 || built["rowsOut"] != 1 || built["missingMeta"] != 1 {
		t.Fatalf("augment funnel diagnostic = %#v", built)
	}
	for key := range built {
		if !wantBuilt[key] {
			t.Fatalf("unexpected augment funnel field %q", key)
		}
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "upstream body must not be logged") || strings.Contains(string(encoded), "/latest/plugins/") {
		t.Fatalf("augment diagnostics leaked upstream data: %s", encoded)
	}
}
