package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func r157Response(request *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: request}
}

const r157MayhemRSC = `72:["$","tr","starter_items_0",{"metaType":"item","metaId":1038}]
73:["$","tr","boots_0",{"metaType":"item","metaId":3006}]
74:["$","tr","core_items_0",{"metaType":"item","metaId":3032}]
75:["$","tr",null,{"children":[["$","x","spell_0",{"metaId":4,"metaType":"spell"}],["$","x","spell_1",{"metaId":6,"metaType":"spell"}]]}]
76:["$","div",null,{"children":[["$","x","skill_0",{"extraData":"Q"}],["$","x","skill_1",{"extraData":"W"}],["$","x","skill_2",{"extraData":"E"}],["$","span","0",{"children":"Q"}]]}]
90:["$","x",null,{"src":"https://opgg-static.akamaized.net/meta/images/lol/16.16.1/item/3032.png"}]
91:["$","x",null,{"metaId":2095,"metaType":"aram-augment"}]`

func TestR157CommunityDragonAugmentBackoffAndRecovery(t *testing.T) {
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(nil)
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	provider.cache.now = func() time.Time { return now }
	provider.augmentBackoff.now = func() time.Time { return now }
	var calls atomic.Int32
	var recovered atomic.Bool
	var events []map[string]any
	upstreamEvents := 0
	provider.diag = func(event map[string]any) {
		if event["event"] == "community_dragon_augments_backoff" {
			events = append(events, event)
		}
		if event["event"] == "champion_upstream" && event["host"] == communityDragonHost {
			upstreamEvents++
		}
	}
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		if request.URL.Host == communityDragonHost && !recovered.Load() {
			return nil, context.DeadlineExceeded
		}
		switch request.URL.Host {
		case communityDragonHost:
			if strings.HasSuffix(request.URL.Path, "cherry-augments.json") {
				return r157Response(request, http.StatusOK, `[{"id":93,"nameTRA":"目录名称"}]`), nil
			}
			return r157Response(request, http.StatusOK, `{"augments":[]}`), nil
		case dataDragonHost, opggChampionHost, hexdataHost:
			return r157Response(request, http.StatusOK, `{}`), nil
		default:
			t.Fatalf("unexpected host: %s", request.URL.Host)
			return nil, nil
		}
	})}
	if _, err := provider.loadCommunityDragonAugments(context.Background()); err == nil {
		t.Fatal("first no-response request unexpectedly succeeded")
	}
	if calls.Load() != 1 {
		t.Fatalf("first request count = %d", calls.Load())
	}
	started := time.Now()
	if _, err := provider.loadCommunityDragonAugments(context.Background()); err == nil || time.Since(started) > time.Second {
		t.Fatalf("backed-off request was slow or succeeded: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("backoff issued another upstream request: %d", calls.Load())
	}
	if upstreamEvents != 1 {
		t.Fatalf("short-circuited call was misreported as upstream traffic: %d events", upstreamEvents)
	}
	if _, err := provider.fetch(context.Background(), dataDragonHost, "/cdn/16.16.1/data/zh_CN/champion.json", nil, 1<<20, "application/json"); err != nil {
		t.Fatalf("Data Dragon was blocked by CommunityDragon backoff: %v", err)
	}
	if _, err := provider.fetch(context.Background(), opggChampionHost, "/api/global/champions/arena/versions", nil, 1<<20, "application/json"); err != nil {
		t.Fatalf("OP.GG was blocked by CommunityDragon backoff: %v", err)
	}
	if _, err := provider.fetch(context.Background(), hexdataHost, "/api/hexdata/heroes/157", nil, 1<<20, "application/json"); err != nil {
		t.Fatalf("Hexdata was blocked by CommunityDragon backoff: %v", err)
	}
	if calls.Load() != 4 {
		t.Fatalf("other-host calls = %d, want 4", calls.Load())
	}
	now = now.Add(communityDragonAugmentRetryDelay + time.Second)
	recovered.Store(true)
	augments, err := provider.loadCommunityDragonAugments(context.Background())
	if err != nil || len(augments) != 1 || calls.Load() != 6 {
		t.Fatalf("recovery: augments=%#v err=%v calls=%d", augments, err, calls.Load())
	}
	if len(events) != 3 || events[0]["result"] != "started" || events[1]["result"] != "skipped" || events[2]["result"] != "ended" || events[2]["skipped_requests"] != 1 {
		t.Fatalf("backoff lifecycle diagnostics = %#v", events)
	}
	encoded, _ := json.Marshal(events)
	if strings.Contains(string(encoded), "目录名称") {
		t.Fatalf("backoff diagnostics leaked catalog contents: %s", encoded)
	}
}

func TestR157CommunityDragonHTTPResponsesNeverCreateBackoff(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			provider := newChampionProvider()
			provider.cache = newChampionDataCache(nil)
			now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
			provider.augmentBackoff.now = func() time.Time { return now }
			var calls atomic.Int32
			var respond atomic.Bool
			provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				calls.Add(1)
				if !respond.Load() {
					return nil, context.DeadlineExceeded
				}
				return r157Response(request, status, `{"error":"upstream"}`), nil
			})}
			if _, err := provider.loadCommunityDragonAugments(context.Background()); err == nil {
				t.Fatal("transport failure did not establish backoff")
			}
			now = now.Add(communityDragonAugmentRetryDelay + time.Second)
			respond.Store(true)
			for range 2 {
				if _, err := provider.loadCommunityDragonAugments(context.Background()); championUpstreamHTTPStatus(err) != status {
					t.Fatalf("HTTP %d result = %v", status, err)
				}
			}
			if calls.Load() != 3 {
				t.Fatalf("HTTP %d incorrectly backed off: calls=%d", status, calls.Load())
			}
		})
	}
}

func TestR157CommunityDragonNoResponseBacksOff(t *testing.T) {
	provider := newChampionProvider()
	var calls atomic.Int32
	provider.client = &http.Client{Transport: championRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, io.EOF
	})}
	for range 2 {
		if _, err := provider.loadCommunityDragonAugments(context.Background()); err == nil {
			t.Fatal("request without a response unexpectedly succeeded")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("no-response backoff issued %d upstream calls, want 1", calls.Load())
	}
}

func TestR157CallerTimeoutDoesNotBackOffCatalog(t *testing.T) {
	provider := newChampionProvider()
	var calls atomic.Int32
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			<-request.Context().Done()
			return nil, request.Context().Err()
		}
		return r157Response(request, http.StatusServiceUnavailable, `{}`), nil
	})}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := provider.loadCommunityDragonAugments(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("caller timeout = %v", err)
	}
	if _, err := provider.loadCommunityDragonAugments(context.Background()); championUpstreamHTTPStatus(err) != http.StatusServiceUnavailable || calls.Load() != 2 {
		t.Fatalf("caller timeout incorrectly backed off host: err=%v calls=%d", err, calls.Load())
	}
	provider.augmentBackoff.mu.Lock()
	defer provider.augmentBackoff.mu.Unlock()
	if len(provider.augmentBackoff.stages) != 0 {
		t.Fatalf("caller timeout established backoff: %#v", provider.augmentBackoff.stages)
	}
}

func TestR157CommunityDragonArenaSupplementBacksOffWithoutLosingBase(t *testing.T) {
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(nil)
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	provider.cache.now = func() time.Time { return now }
	provider.augmentBackoff.now = func() time.Time { return now }
	var cherryCalls, arenaCalls int
	var recovered bool
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if strings.HasSuffix(request.URL.Path, "cherry-augments.json") {
			cherryCalls++
			return r157Response(request, http.StatusOK, `[{"id":93,"nameTRA":"基础目录"}]`), nil
		}
		arenaCalls++
		if !recovered {
			return nil, context.DeadlineExceeded
		}
		return r157Response(request, http.StatusOK, `{"augments":[{"id":225,"name":"补充目录"}]}`), nil
	})}
	for range 2 {
		catalog, err := provider.loadCommunityDragonAugments(context.Background())
		if err != nil || len(catalog) != 1 || catalog[0].ID != 93 {
			t.Fatalf("base catalog lost during supplement outage: %#v, %v", catalog, err)
		}
	}
	if cherryCalls != 1 || arenaCalls != 1 {
		t.Fatalf("repeated supplement calls: cherry=%d arena=%d", cherryCalls, arenaCalls)
	}
	now = now.Add(communityDragonAugmentRetryDelay + time.Second)
	recovered = true
	catalog, err := provider.loadCommunityDragonAugments(context.Background())
	if err != nil || len(catalog) != 2 || arenaCalls != 2 {
		t.Fatalf("supplement did not recover: %#v, %v, calls=%d", catalog, err, arenaCalls)
	}
}

func TestR159MayhemDetailDoesNotWaitForFailedCatalog(t *testing.T) {
	provider, _ := r116aMayhemProvider(t, &r116aRecorder{}, r116aFixture(t, "hexdata-hero-157.json"))
	provider.static["item/3032.png"] = championAssetDescription{Name: "测试装备"}
	base := provider.client.Transport
	var catalogCalls atomic.Int32
	provider.client.Transport = championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == opggPageHost && strings.HasSuffix(request.URL.Path, "/yasuo/build") {
			return r157Response(request, http.StatusOK, r157MayhemRSC), nil
		}
		if request.URL.Host == communityDragonHost && strings.HasSuffix(request.URL.Path, "cherry-augments.json") {
			catalogCalls.Add(1)
			return r157Response(request, http.StatusServiceUnavailable, `{}`), nil
		}
		return base.RoundTrip(request)
	})
	response, err := provider.loadMayhemDetail(context.Background(), "157")
	if err != nil || len(response.RecommendedAugments) == 0 || len(response.Build.CoreItems) == 0 || catalogCalls.Load() != 0 {
		t.Fatalf("Mayhem fallback: err=%v rows=%d RSC core=%d catalogCalls=%d", err, len(response.RecommendedAugments), len(response.Build.CoreItems), catalogCalls.Load())
	}
}

func TestR157ArenaDetailLoadsFailedCatalogOnce(t *testing.T) {
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(nil)
	provider.patch = "16.19.1"
	provider.static["item/3153.png"] = championAssetDescription{Name: "测试装备"}
	var catalogCalls atomic.Int32
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		key := request.URL.Host + request.URL.Path
		var body string
		status := http.StatusOK
		switch key {
		case opggChampionHost + "/api/global/champions/arena/versions":
			body = `{"data":["16.19"]}`
		case opggChampionHost + "/api/global/champions/arena/67":
			body = `{"data":{"summary":{"id":67,"average_stats":{"play":1000,"win":550}},"core_items":[{"ids":[3153],"play":100,"win":60}],"augment_group":[{"rarity":1,"augments":[{"id":904,"play":100,"win":55}]}]},"meta":{"version":"16.19"}}`
		case yourGGArenaHost + "/kr/api/arena/champions/67":
			body = `{"success":true,"statusCode":200,"response":{"version":"16.19","coreItems":[],"prismaticItems":[],"augments":[{"augmentId":904,"tier":"S","score":90,"matches":100}]}}`
		case communityDragonHost + "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/cherry-augments.json":
			catalogCalls.Add(1)
			status = http.StatusServiceUnavailable
			body = `{}`
		case communityDragonHost + "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/items.json":
			body = `[{"id":3153,"name":"测试装备"}]`
		default:
			t.Fatalf("unexpected Arena request: %s", key)
		}
		return r157Response(request, status, body), nil
	})}
	response, err := provider.loadStructuredDetail(context.Background(), "arena", "67", "", "")
	if err != nil || len(response.ArenaAugmentGroups) == 0 || catalogCalls.Load() != 0 {
		t.Fatalf("Arena fallback: err=%v groups=%d catalogCalls=%d", err, len(response.ArenaAugmentGroups), catalogCalls.Load())
	}
}

func TestR157ArenaDetailReturnsQuicklyAfterCatalogTimeout(t *testing.T) {
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(nil)
	provider.patch = "16.19.1"
	provider.static["item/3153.png"] = championAssetDescription{Name: "测试装备"}
	var catalogCalls atomic.Int32
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		key := request.URL.Host + request.URL.Path
		var body string
		switch key {
		case opggChampionHost + "/api/global/champions/arena/versions":
			body = `{"data":["16.19"]}`
		case opggChampionHost + "/api/global/champions/arena/67":
			body = `{"data":{"summary":{"id":67,"average_stats":{"play":1000,"win":550}},"core_items":[{"ids":[3153],"play":100,"win":60}],"augment_group":[{"rarity":1,"augments":[{"id":904,"play":100,"win":55}]}]},"meta":{"version":"16.19"}}`
		case yourGGArenaHost + "/kr/api/arena/champions/67":
			body = `{"success":true,"statusCode":200,"response":{"version":"16.19","coreItems":[],"prismaticItems":[],"augments":[{"augmentId":904,"tier":"S","score":90,"matches":100}]}}`
		case communityDragonHost + "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/cherry-augments.json":
			catalogCalls.Add(1)
			<-request.Context().Done()
			return nil, request.Context().Err()
		case communityDragonHost + "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/items.json":
			body = `[{"id":3153,"name":"测试装备"}]`
		default:
			t.Fatalf("unexpected Arena request: %s", key)
		}
		return r157Response(request, http.StatusOK, body), nil
	})}
	firstStarted := time.Now()
	first, err := provider.loadStructuredDetail(context.Background(), "arena", "67", "", "")
	if err != nil || len(first.ArenaAugmentGroups) == 0 || time.Since(firstStarted) >= 2*time.Second {
		t.Fatalf("first Arena fallback: err=%v groups=%d elapsed=%s", err, len(first.ArenaAugmentGroups), time.Since(firstStarted))
	}
	secondStarted := time.Now()
	second, err := provider.loadStructuredDetail(context.Background(), "arena", "67", "", "")
	if err != nil || len(second.ArenaAugmentGroups) == 0 || time.Since(secondStarted) >= 2*time.Second || catalogCalls.Load() != 0 {
		t.Fatalf("backed-off Arena detail: err=%v groups=%d elapsed=%s catalogCalls=%d", err, len(second.ArenaAugmentGroups), time.Since(secondStarted), catalogCalls.Load())
	}
}

func TestR157SlowCommunityDragonDoesNotExhaustGameplayOrManualDetail(t *testing.T) {
	provider, _ := r116aMayhemProvider(t, &r116aRecorder{}, r116aFixture(t, "hexdata-hero-157.json"))
	provider.static["item/3032.png"] = championAssetDescription{Name: "测试装备"}
	base := provider.client.Transport
	var catalogCalls atomic.Int32
	provider.client.Transport = championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == opggPageHost && strings.HasSuffix(request.URL.Path, "/yasuo/build") {
			return r157Response(request, http.StatusOK, r157MayhemRSC), nil
		}
		if request.URL.Host == communityDragonHost && strings.HasSuffix(request.URL.Path, "cherry-augments.json") {
			catalogCalls.Add(1)
			<-request.Context().Done()
			return nil, request.Context().Err()
		}
		return base.RoundTrip(request)
	})
	var mu sync.Mutex
	var phases []map[string]any
	priorDiag := provider.diag
	provider.diag = func(event map[string]any) {
		if priorDiag != nil {
			priorDiag(event)
		}
		if event["event"] == "mayhem_detail_phases_ms" {
			mu.Lock()
			phases = append(phases, event)
			mu.Unlock()
		}
	}
	gameplayCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	firstStarted := time.Now()
	if _, err := provider.loadMayhemDetail(gameplayCtx, "157"); err != nil {
		t.Fatalf("gameplay detail after catalog timeout: %v", err)
	}
	if elapsed := time.Since(firstStarted); elapsed >= 10*time.Second {
		t.Fatalf("catalog probe consumed too much of 15s gameplay budget: %s", elapsed)
	}
	manual := httptest.NewRequest(http.MethodGet, "/api/champions/detail?mode=hextech-aram&champion=157", nil)
	w := httptest.NewRecorder()
	manualStarted := time.Now()
	(&app{champions: provider}).handleChampionDetail(w, manual)
	if w.Code != http.StatusOK || time.Since(manualStarted) >= 2*time.Second || catalogCalls.Load() != 0 {
		t.Fatalf("manual detail: status=%d elapsed=%s catalogCalls=%d", w.Code, time.Since(manualStarted), catalogCalls.Load())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(phases) != 2 {
		t.Fatalf("phase diagnostics = %#v", phases)
	}
	for _, phase := range phases {
		if phase["total"].(int64) >= 15000 {
			t.Fatalf("detail still hit the 15s boundary: %#v", phase)
		}
	}
	if phases[1]["decorate"].(int64) >= 1000 {
		t.Fatalf("backed-off decorate remained slow: %#v", phases[1])
	}
}
