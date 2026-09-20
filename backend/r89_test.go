package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

var r89PNG = []byte{137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 13, 73, 72, 68, 82}

func r89ImageApp(t *testing.T, root string, calls *atomic.Int32) *app {
	t.Helper()
	p := newChampionProvider()
	p.cache = newChampionDataCache(nil)
	p.imageCache = newPublicBinaryCache(&localStore{root: root}, "champion-images", 2048, 64<<20)
	p.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != dataDragonHost {
			t.Errorf("unexpected host %s", r.URL.Host)
		}
		calls.Add(1)
		time.Sleep(20 * time.Millisecond)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(r89PNG))}, nil
	})}
	return &app{champions: p}
}

func TestR89ImagePolicyMemoryDiskAndSingleFlight(t *testing.T) {
	var calls atomic.Int32
	root := t.TempDir()
	a := r89ImageApp(t, root, &calls)
	path := "/cdn/16.18.1/img/item/3006.png"
	load := func(a *app) {
		t.Helper()
		w := httptest.NewRecorder()
		a.handleChampionAsset(w, httptest.NewRequest("GET", "/api/champion-asset?source=ddragon&path="+path, nil))
		if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), r89PNG) {
			t.Errorf("asset %d %s", w.Code, w.Body)
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); load(a) }()
	}
	wg.Wait()
	load(a)
	load(a)
	if calls.Load() != 1 {
		t.Fatalf("singleflight/memory upstream=%d", calls.Load())
	}
	// Independently exercise the policy, so the outer asset cache cannot hide a
	// regression that silently disables provider-level image persistence.
	_, err := a.champions.fetch(context.Background(), dataDragonHost, path, nil, championImageMax, championImageAccept)
	if err != nil || calls.Load() != 1 {
		t.Fatalf("image policy bypassed cache: calls=%d err=%v", calls.Load(), err)
	}
	load(r89ImageApp(t, root, &calls))
	if calls.Load() != 1 {
		t.Fatalf("restart missed disk: %d", calls.Load())
	}
}

func TestR89ImageNegativeCache(t *testing.T) {
	var calls atomic.Int32
	a := r89ImageApp(t, t.TempDir(), &calls)
	a.champions.client.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("missing")), Header: http.Header{}}, nil
	})
	for i := 0; i < 3; i++ {
		_, _ = a.loadChampionRemoteAsset(context.Background(), a.champions, "ddragon", dataDragonHost, "/cdn/16.18.1/img/item/1.png")
	}
	if calls.Load() != 1 {
		t.Fatal("negative cache requests", calls.Load())
	}
}

func TestR89DiskBudgetAfter2000Images(t *testing.T) {
	c := newPublicBinaryCache(&localStore{root: t.TempDir()}, "champion-images", 2048, 64<<20)
	data := bytes.Repeat([]byte("x"), 48<<10)
	for i := 0; i < 2000; i++ {
		key := championCacheKey(dataDragonHost, fmt.Sprintf("/cdn/16.18.1/img/item/%d.png", i), "", championImageAccept)
		if err := c.writeDisk(championCacheEnvelope{Key: key, Data: data}); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, e := range entries {
		info, _ := e.Info()
		total += info.Size()
	}
	if total > 64<<20 || len(entries) > 2048 {
		t.Fatalf("unbounded disk: bytes=%d entries=%d", total, len(entries))
	}
}

func TestR89WarmPathsRankedAndFinalPatch(t *testing.T) {
	descriptions := map[string]championAssetDescription{}
	scores := map[int]int{}
	for i := 1; i <= 200; i++ {
		descriptions[fmt.Sprintf("item/%d.png", i)] = championAssetDescription{Path: fmt.Sprintf("/cdn/16.18.1/img/item/%d.png", i)}
		scores[i] = i
	}
	paths := rankedItemWarmPaths(descriptions, scores)
	if len(paths) != 120 || paths[0] != "/cdn/16.18.1/img/item/200.png" || paths[119] != "/cdn/16.18.1/img/item/81.png" {
		t.Fatal(paths)
	}
}

func TestR89ForegroundCancelsWarmup(t *testing.T) {
	var calls atomic.Int32
	a := r89ImageApp(t, t.TempDir(), &calls)
	ctx, cancel := context.WithCancel(context.Background())
	a.champions.imageWarm.started = true
	a.champions.imageWarm.cancel = cancel
	a.scheduleItemIconWarmup(false)
	defer a.champions.imageWarm.timer.Stop()
	if ctx.Err() != context.Canceled {
		t.Fatal("foreground did not preempt prefetch")
	}
}

func TestR89SupplementIdentityEndpointsAndDedup(t *testing.T) {
	var calls atomic.Int32
	a := newSummaryFixtureApp(t, &calls)
	for _, body := range []string{`{"gameName":"Fixture","tagLine":"KR1","region":"kr"}`, `{}`} {
		w := httptest.NewRecorder()
		a.handleOPGGSeasonSummary(w, httptest.NewRequest("POST", "/", strings.NewReader(body)))
		want := 200
		if body == `{}` {
			want = 400
		}
		if w.Code != want {
			t.Fatalf("season %d %s", w.Code, w.Body)
		}
	}
	ref := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: "subject-puuid-0000001", GameName: "Fixture", TagLine: "KR1", Region: "kr"})
	w := httptest.NewRecorder()
	a.handleOPGGSeasonSummary(w, httptest.NewRequest("POST", "/", strings.NewReader(fmt.Sprintf(`{"playerRef":%q,"gameName":"wrong","tagLine":"wrong","region":"cn"}`, ref))))
	if w.Code != 200 || calls.Load() != 2 {
		t.Fatalf("ref priority/cache: %d calls=%d", w.Code, calls.Load())
	}
	// Current-game's page and action are independently validated; no Riot/LCU.
	a.champions.client.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "op.gg" {
			t.Errorf("unexpected host %s", r.URL.Host)
		}
		data := summaryFixtureHTML("subject-puuid-0000001", 33)
		if r.Method == "POST" {
			data = []byte("0:{\"a\":\"$@1\"}\n1:\"$undefined\"")
		}
		return proHTTPBody(data), nil
	})
	for _, body := range []string{`{"gameName":"Fixture","tagLine":"KR1","region":"kr"}`, `{}`} {
		w := httptest.NewRecorder()
		a.handleOverviewCurrentGame(w, httptest.NewRequest("POST", "/", strings.NewReader(body)))
		want := 200
		if body == `{}` {
			want = 400
		}
		if w.Code != want {
			t.Fatalf("current %d %s", w.Code, w.Body)
		}
	}
}

func TestR89RiotMatchDiskAndImmutableData(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "test-fixture")
	store := &localStore{root: t.TempDir()}
	c := newChampionProvider()
	c.cache = newChampionDataCache(store)
	var calls atomic.Int32
	c.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return proHTTPBody([]byte(`{"metadata":{"matchId":"KR_123"},"info":{"gameId":123,"participants":[{"participantId":1,"puuid":"public-match-fixture","item0":3006,"item6":3340}]}}`)), nil
	})}
	p := newRiotProvider(c)
	if _, _, err := p.matchByIDWithCache(context.Background(), "KR_123"); err != nil {
		t.Fatal(err)
	}
	p = newRiotProvider(c)
	match, status, err := p.matchByIDWithCache(context.Background(), "KR_123")
	if err != nil || status != "disk" || calls.Load() != 1 || match.Info.Participants[0].Item0 != 3006 {
		t.Fatalf("restart status=%s calls=%d err=%v", status, calls.Load(), err)
	}
	files, _ := os.ReadDir(p.matchDisk.dir)
	if len(files) != 1 {
		t.Fatal("no disk match")
	}
	data, _ := os.ReadFile(filepath.Join(p.matchDisk.dir, files[0].Name()))
	if bytes.Contains(data, []byte("test-fixture")) {
		t.Fatal("key persisted")
	}
}

func TestR89Limiter200Reservations(t *testing.T) {
	p := newRiotProvider(newChampionProvider())
	now := time.Unix(1000, 0)
	p.limitNow = func() time.Time { return now }
	p.limitSleep = func(_ context.Context, d time.Duration) error { now = now.Add(d); return nil }
	var reservations []time.Time
	for i := 0; i < 200; i++ {
		if err := p.wait(context.Background()); err != nil {
			t.Fatal(err)
		}
		reservations = append(reservations, now)
		short, long := 0, 0
		for _, at := range reservations {
			if !at.Before(now.Add(-time.Second)) {
				short++
			}
			if !at.Before(now.Add(-2 * time.Minute)) {
				long++
			}
		}
		if short > 15 || long > 90 {
			t.Fatalf("request %d short=%d long=%d", i, short, long)
		}
	}
	if now.Sub(time.Unix(1000, 0)) < 4*time.Minute {
		t.Fatal("long window bypassed")
	}
}

func TestR89KeyConcurrencyGuard(t *testing.T) {
	t.Setenv("DEEP_LEGENDS_RIOT_KEY_TIER", "")
	t.Setenv("DEEP_LEGENDS_RIOT_MATCH_CONCURRENCY", "20")
	if configuredRiotMatchConcurrency() != 8 {
		t.Fatal("personal key concurrency")
	}
	t.Setenv("DEEP_LEGENDS_RIOT_KEY_TIER", "production")
	if configuredRiotMatchConcurrency() != 20 {
		t.Fatal("production configuration")
	}
}

func TestR89DiagnosticEventsReachDisk(t *testing.T) {
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	if err := os.MkdirAll(filepath.Join(store.root, "logs"), 0700); err != nil {
		t.Fatal(err)
	}
	a := &app{storage: store}
	a.reportCatalogLoad("items", "ddragon", 868, time.Now().Add(-time.Millisecond), nil)
	a.reportCatalogLoad("items", "ddragon", 0, time.Now(), context.DeadlineExceeded)
	a.recordDiagnostic(riotMatchItemsDiagnostic(gameplayMatch{Participants: []gameplayParticipant{{ItemIDs: []int64{3006, 0, 0, 0, 0, 0, 3340}}, {ItemIDs: []int64{0, 0, 0, 0, 0, 0, 0}}}}))
	for _, body := range []string{`{"event":"catalog_client","reason":"failed","endpoint":"items","httpStatus":504,"errorKind":"timeout"}`, `{"event":"item_id_not_in_catalog","reason":"missing","endpoint":"items","itemId":9999}`} {
		w := httptest.NewRecorder()
		a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/", strings.NewReader(body)))
		if w.Code != 204 {
			t.Fatal(w.Code, w.Body)
		}
	}
	p := newChampionProvider()
	p.diag = a.recordDiagnostic
	p.observeAssetFetch(dataDragonHost, "disk", time.Millisecond, nil)
	p.observeAssetFetch(dataDragonHost, "memory", time.Microsecond, nil)
	p.observeAssetFetch(dataDragonHost, "miss", 2*time.Millisecond, errors.New("failed"))
	p.flushAssetFetch()
	data, err := store.readDiagnosticLogForExport()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"event":"catalog_load"`, `"outcome":"failure"`, `"event":"catalog_client"`, `"event":"item_id_not_in_catalog"`, `"id":9999`, `"items_present_count":1`, `"items_zero_count":1`, `"length":7`, `"nonzero":2`, `"event":"asset_fetch"`, `"failures":1`, `"disk":1`, `"memory":1`} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("missing %s in %s", want, data)
		}
	}
	var event map[string]any
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if json.Unmarshal(line, &event) == nil && event["event"] == "asset_fetch" {
			if event["hit_rate"].(float64) < .66 {
				t.Fatal(event)
			}
		}
	}
}

func TestR89IdentityRestartAndExpiry(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "test-fixture")
	c := newChampionProvider()
	c.cache = newChampionDataCache(&localStore{root: t.TempDir()})
	var calls atomic.Int32
	c.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return proHTTPBody([]byte(`{"puuid":"public-player","gameName":"Fixture","tagLine":"KR1","summonerLevel":99}`)), nil
	})}
	for i := 0; i < 2; i++ {
		p := newRiotProvider(c)
		if _, err := p.accountByRiotID(t.Context(), "Fixture", "KR1"); err != nil {
			t.Fatal(err)
		}
		if _, err := p.summonerByPUUID(t.Context(), "public-player"); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("restart identity requests=%d", calls.Load())
	}
	p := newRiotProvider(c)
	var result map[string]any
	if err := p.cachedPublicIdentity(t.Context(), "expired", -time.Second, &result, func(context.Context) error { result = map[string]any{"old": true}; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := p.cachedPublicIdentity(t.Context(), "expired", -time.Second, &result, func(context.Context) error { return errors.New("offline") }); err == nil {
		t.Fatal("expired public identity served stale")
	}
}

func TestR89ActualConcurrencyPhasesAndQueue(t *testing.T) {
	t.Setenv("DEEP_LEGENDS_RIOT_MATCH_CONCURRENCY", "8")
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	const detailCount = 20
	var inFlight atomic.Int32
	var maxInFlight atomic.Int32
	champions := newChampionProvider()
	champions.mu.Lock()
	champions.championMeta = map[int]championMetadata{1: {NameZH: "黑暗之女"}}
	champions.mu.Unlock()
	champions.clientMu.Lock()
	champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := "[]"
		switch {
		case strings.Contains(request.URL.Path, "/lol/summoner/v4/summoners/by-puuid/"):
			body = `{"puuid":"subject","profileIconId":1,"summonerLevel":100}`
		case strings.HasSuffix(request.URL.Path, "/ids"):
			ids := make([]string, 20)
			for i := range ids {
				ids[i] = fmt.Sprintf("KR_%d", i+1)
			}
			raw, _ := json.Marshal(ids)
			body = string(raw)
		case strings.Contains(request.URL.Path, "/lol/league/v4/entries/"):
			body = "[]"
		case strings.Contains(request.URL.Path, "/lol/champion-mastery/v4/"):
			body = "[]"
		case strings.Contains(request.URL.Path, "/lol/match/v5/matches/"):
			active := inFlight.Add(1)
			for {
				seen := maxInFlight.Load()
				if active <= seen || maxInFlight.CompareAndSwap(seen, active) {
					break
				}
			}
			time.Sleep(80 * time.Millisecond)
			inFlight.Add(-1)
			body = `{"metadata":{"matchId":"fixture"},"info":{"gameId":1,"queueId":420,"gameDuration":1800,"participants":[{"puuid":"subject","participantId":1,"teamId":100,"championId":1,"win":true}]}}`
		default:
			t.Fatalf("unexpected Riot request: %s", request.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	})}
	champions.clientMu.Unlock()

	events := make(chan map[string]any, 64)
	champions.diag = func(event map[string]any) { events <- event }
	a := &app{riot: newRiotProvider(champions)}
	phases := newOverviewPhaseTimings(time.Now())
	ctx := context.WithValue(context.Background(), overviewPhasesContextKey{}, phases)
	overview, err := a.loadRiotOverview(ctx, gameplayReference{PlayerRef: "subject", Region: riotRegionKR}, 0, detailCount)
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Matches) != detailCount {
		t.Fatalf("matches = %d, want %d", len(overview.Matches), detailCount)
	}
	if !overview.SeasonStatsProgress.Unavailable || overview.SeasonStatsProgress.Collecting || len(overview.SeasonChampionStats) != 0 {
		t.Fatal("KR overview must not start an implicit season crawl")
	}
	if got := maxInFlight.Load(); got != 8 {
		t.Fatalf("maximum concurrent Riot match detail requests = %d, want 8", got)
	}

	var cost map[string]any
	for len(events) > 0 {
		event := <-events
		if event["event"] == "riot_overview_cost" {
			cost = event
		}
	}
	if cost["details_inflight_peak"] != 8 || cost["limiter_queue_ms"].(int64) <= 0 {
		t.Fatalf("cost %+v", cost)
	}
	for _, name := range []string{"account", "summoner", "champion_names", "matchIDs", "ranks", "mastery", "details", "opgg-historical"} {
		if len(phases.spans[name]) != 1 {
			t.Fatalf("missing phase %s: %+v", name, phases.spans)
		}
	}
	if phases.values["details"] <= 0 {
		t.Fatal("details latency disappeared")
	}
}

func TestR89CatalogEndpointFailuresReachDisk(t *testing.T) {
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	os.MkdirAll(filepath.Join(store.root, "logs"), 0700)
	p := newChampionProvider()
	p.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != dataDragonHost {
			t.Errorf("catalog reached unexpected host %s", r.URL.Host)
		}
		return nil, context.DeadlineExceeded
	})}
	a := &app{storage: store, champions: p}
	for name, handler := range map[string]func(http.ResponseWriter, *http.Request){"items": a.handleGameplayItems, "perks": a.handleGameplayPerks, "summoner-spells": a.handleGameplaySummonerSpells} {
		w := httptest.NewRecorder()
		handler(w, httptest.NewRequest("GET", "/api/gameplay/"+name, nil))
		if w.Code < 400 {
			t.Fatalf("failed %s accepted", name)
		}
	}
	data, err := store.readDiagnosticLogForExport()
	if err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"items", "perks", "summoner-spells"} {
		found := false
		for _, line := range bytes.Split(data, []byte{'\n'}) {
			var e map[string]any
			if json.Unmarshal(line, &e) == nil && e["event"] == "catalog_load" && e["endpoint"] == endpoint && e["source"] == "ddragon" && e["outcome"] == "failure" {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s failure not persisted: %s", endpoint, data)
		}
	}
	if output := os.Getenv("R89_DIAGNOSTIC_SAMPLE"); output != "" {
		if err := os.WriteFile(output, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
