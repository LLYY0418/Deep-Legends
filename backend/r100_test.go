package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestR100OverviewLongRoundUsesFreshSingleWaitBudget(t *testing.T) {
	f := r98OverviewFixture(t, false)
	p := f.a.riot
	transport := p.champions.client.Transport
	var once sync.Once
	p.champions.client.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		response, err := transport.RoundTrip(r)
		if strings.HasSuffix(r.URL.Path, "/ids") {
			once.Do(func() {
				time.Sleep(5200 * time.Millisecond) // actual round age, not a fake/shifted clock
				p.limitMu.Lock()
				p.shortWindow = make([]time.Time, 15)
				for i := range p.shortWindow {
					p.shortWindow[i] = time.Now().Add(-800 * time.Millisecond)
				}
				p.limitMu.Unlock()
			})
		}
		return response, err
	})
	started := time.Now()
	response, err := f.a.loadRiotOverview(context.Background(), gameplayReference{PlayerRef: "r98-subject", Region: "kr", Privacy: "PRIVATE"}, 0, 20)
	if err != nil || len(response.Matches) != 20 {
		t.Fatalf("successful slow round aborted: matches=%d err=%v", len(response.Matches), err)
	}
	if time.Since(started) <= 5*time.Second {
		t.Fatal("fixture did not exceed old round deadline")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range f.events {
		if e["event"] == "riot_local_rate_limited" {
			t.Fatal("unexpected throttle", e)
		}
	}
	p.cacheMu.Lock()
	defer p.cacheMu.Unlock()
	if len(p.matchCache) != 20 {
		t.Fatal("paid details not retained", len(p.matchCache))
	}
}

func TestR100QueueCallersKeepDistinctSemantics(t *testing.T) {
	ctx := withRiotSingleWaitLimit(context.Background(), 100*time.Millisecond)
	if _, ok := ctx.Value(riotQueueDeadlineKey{}).(time.Time); ok {
		t.Fatal("single-wait stored an absolute deadline")
	}
	p := newRiotProvider(newChampionProvider())
	p.shortWindow = make([]time.Time, 15)
	for i := range p.shortWindow {
		p.shortWindow[i] = time.Now()
	}
	started := time.Now()
	err := p.wait(ctx)
	if !errors.Is(err, errThrottled) || riotErrorStatus(err) != 429 || time.Since(started) > 250*time.Millisecond {
		t.Fatal("seed single wait did not refuse busy quota", err)
	}
	seedProvider := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) {
		t.Error("busy seed should never reach HTTP")
		return nil, errors.New("unexpected")
	})
	seedProvider.shortWindow = make([]time.Time, 15)
	for i := range seedProvider.shortWindow {
		seedProvider.shortWindow[i] = time.Now()
	}
	started = time.Now()
	_, seedErr := seedProvider.resolveProSeed(context.Background(), r102RookieSeed(), 0)
	if !errors.Is(seedErr, errThrottled) || time.Since(started) > 250*time.Millisecond {
		t.Fatalf("actual seed caller lost 100ms bound: %v", seedErr)
	}

	step, cancel := specialistStepContext(context.Background(), time.Second)
	defer cancel()
	deadline, ok := step.Value(riotQueueDeadlineKey{}).(time.Time)
	if !ok || time.Until(deadline) > specialistRiotQueueTimeout {
		t.Fatal("specialist step lost total queue budget")
	}
	if _, ok := step.Value(riotSingleWaitKey{}).(time.Duration); ok {
		t.Fatal("specialist silently changed semantics")
	}
}

func TestR100SuccessfulDetailSurvivesCallerCancellation(t *testing.T) {
	f := r98OverviewFixture(t, false)
	p := f.a.riot
	transport := p.champions.client.Transport
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.champions.client.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		response, err := transport.RoundTrip(r)
		// Materialize successful bytes before cancellation, like an already-paid response.
		if err == nil {
			body, readErr := io.ReadAll(io.LimitReader(response.Body, riotResponseMax))
			_ = response.Body.Close()
			if readErr != nil {
				return nil, readErr
			}
			response.Body = io.NopCloser(strings.NewReader(string(body)))
			cancel()
		}
		return response, err
	})
	match, _, err := p.matchByIDWithCache(ctx, "KR_1")
	if err != nil || match == nil {
		t.Fatal(err)
	}
	p.cacheMu.Lock()
	cached := p.matchCache["KR_1"]
	p.cacheMu.Unlock()
	if cached == nil {
		t.Fatal("successful detail dropped on cancellation")
	}
	_, status, err := p.matchByIDWithCache(context.Background(), "KR_1")
	if err != nil || status != "hit" {
		t.Fatal("paid data fetched again", status, err)
	}
}

func TestR100AssetHasWholeHandlerDeadline(t *testing.T) {
	p := newChampionProvider()
	p.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > 8100*time.Millisecond {
			t.Error("asset missing whole-handler deadline")
		}
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	a := &app{champions: p}
	started := time.Now()
	w := httptest.NewRecorder()
	a.handleChampionAsset(w, httptest.NewRequest("GET", "/api/champion-asset?source=communitydragon&path=/latest/game/assets/ux/cherry/augments/icons/test_large.png", nil))
	if w.Code != 404 || time.Since(started) > 8500*time.Millisecond {
		t.Fatalf("slow asset held socket: status=%d elapsed=%v", w.Code, time.Since(started))
	}
}

func waitR100PerkEnrichment(t *testing.T, a *app, key string) {
	t.Helper()
	a.perkCatalogMu.Lock()
	done := a.perkAugmentJobs[key]
	a.perkCatalogMu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-time.After(4 * time.Second):
			t.Fatal("augment job stuck")
		}
	}
}

func TestR100PerksNeverWaitForOptionalAugmentsAndPersist(t *testing.T) {
	p := newChampionProvider()
	startedAugment := make(chan struct{})
	var once sync.Once
	p.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		once.Do(func() { close(startedAugment) })
		select {
		case <-r.Context().Done():
			return nil, r.Context().Err()
		case <-time.After(12 * time.Second):
			t.Error("optional 12s reached")
			return nil, errors.New("fixture offline")
		}
	})}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "perkstyles"):
			_, _ = w.Write([]byte(`[{"id":8000,"name":"精密","slots":[{"perks":[8005]}]}]`))
		case strings.Contains(r.URL.Path, "perks.json"):
			_, _ = w.Write([]byte(`[{"id":8005,"name":"强攻","iconPath":"/lol-game-data/assets/v1/perks/8005.png"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	client := &LCUClient{baseURL: server.URL, token: "fixture", http: server.Client()}
	a := &app{connected: true, lcu: client, champions: p, storage: store}
	started := time.Now()
	w := httptest.NewRecorder()
	a.handleGameplayPerks(w, httptest.NewRequest("GET", "/api/gameplay/perks", nil))
	if w.Code != 200 || time.Since(started) > 2*time.Second {
		t.Fatal("perks waited for augments", w.Code, time.Since(started))
	}
	<-startedAugment
	if !a.perkCatalogMu.TryLock() {
		t.Fatal("optional I/O holds catalog mutex")
	}
	a.perkCatalogMu.Unlock()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			a.handleGameplayPerks(w, httptest.NewRequest("GET", "/api/gameplay/perks", nil))
			if w.Code != 200 {
				t.Error(w.Code)
			}
		}()
	}
	wg.Wait()
	if time.Since(started) > 2*time.Second {
		t.Error("parallel perks blocked behind optional I/O")
	}
	waitR100PerkEnrichment(t, a, "lcu")
	reboot := &app{storage: store}
	cached, err := reboot.cachedGameplayPerkCatalog(context.Background(), "lcu", func() (gameplayPerkCatalogResponse, error) {
		t.Error("restart fetched normalized catalog again")
		return gameplayPerkCatalogResponse{}, errors.New("network unavailable")
	})
	if err != nil || len(cached.Perks) == 0 {
		t.Fatal("normalized catalog not persisted", err)
	}
}

func TestR100ProSnapshotAndLadderSurviveRestart(t *testing.T) {
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	a := &app{storage: store}
	a.proPlayers.mu.Lock()
	a.restoreProSnapshotLocked()
	account := proFixtureAccount("fixtureaccount", "fixture-puuid", "CHALLENGER", 1, 1200)
	account.LadderRank = 42
	account.LadderRankKnown = true
	a.proPlayers.teams = []opggProTeam{{ID: 632, Name: "Bilibili Gaming", Members: []opggProMember{proFixtureMember(632, "Bin", "Chen Ze-Bin (陈泽彬)", account)}}}
	a.proPlayers.fetchedAt = time.Now().Add(-time.Hour)
	a.persistProSnapshotLocked()
	a.proPlayers.mu.Unlock()
	reboot := &app{storage: store}
	reboot.proPlayers.mu.Lock()
	restored := reboot.restoreProSnapshotLocked()
	rows := reboot.proPlayers.teams
	reboot.proPlayers.mu.Unlock()
	if !restored || len(rows) != 1 || rows[0].Members[0].Summoners[0].LadderRank != 42 {
		t.Fatal("published snapshot/rank lost on restart")
	}
	var calls int
	makeProvider := func() *championProvider {
		p := newChampionProvider()
		p.cache = newChampionDataCache(store)
		if err := os.MkdirAll(p.cache.dir, 0700); err != nil {
			t.Fatal(err)
		}
		p.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			return proHTTPBody([]byte(`<table><tr id="fixtureaccount-KR1"><td>42</td><td>fixtureaccount</td></tr></table>`)), nil
		})}
		return p
	}
	for i := 0; i < 2; i++ {
		rank, err := fetchOPGGLadderRank(context.Background(), makeProvider(), "fixtureaccount", "KR1")
		if err != nil || rank != 42 {
			t.Fatal(rank, err)
		}
	}
	if calls != 1 {
		t.Fatal("ladder refetched after restart", calls)
	}
}

func TestR100QueueLabelsAndArenaRegistry(t *testing.T) {
	for id, size := range map[int64]int{1700: 2, 1701: 1, 1704: 2, 1710: 2, 1720: 2, 1731: 1, 1732: 1, 1740: 2, 1750: 3} {
		if got := queueLabel(id, "CHERRY", nil); !containsChineseQueueLabel(got) {
			t.Fatalf("queue %d raw label %q", id, got)
		}
		if arenaSquadSize(id) != size {
			t.Fatalf("queue %d size %d", id, arenaSquadSize(id))
		}
		if !strings.Contains(strings.Join(matchHistoryFilterFor("arena").Tags, ","), fmt.Sprintf("q_%d", id)) {
			t.Fatal("missing filter", id)
		}
	}
	for _, mode := range []string{"NEW_UNKNOWN_MODE", "CHERRY", "CLASSIC", "ARAM", "KIWI", "TFT", ""} {
		for _, id := range []int64{0, 999999, 4210, 1704} {
			if got := queueLabel(id, mode, map[int64]string{id: "English raw name"}); !containsChineseQueueLabel(got) {
				t.Fatalf("raw label %q", got)
			}
		}
	}
	if got := queueLabel(99999, "NEW_UNKNOWN_MODE", nil); got != "其他模式" {
		t.Fatal(got)
	}
	if got := queueLabel(1720, "CHERRY", map[int64]string{1720: "客户端中文队列"}); got != "客户端中文队列" {
		t.Fatal(got)
	}
}

func TestR100ProgressPublishesReadyMetadataAndQueueDiagnostic(t *testing.T) {
	f := r98OverviewFixture(t, false)
	f.a.storage = trackTestStore(t, &localStore{root: t.TempDir()})
	if err := os.MkdirAll(f.a.storage.root+"/logs", 0700); err != nil {
		t.Fatal(err)
	}
	original := f.a.riot.champions.client.Transport
	f.a.riot.champions.client.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := ""
		if strings.Contains(r.URL.Path, "/league/") {
			body = `[{"queueType":"RANKED_SOLO_5x5","tier":"MASTER","rank":"I","leaguePoints":100}]`
		}
		if strings.Contains(r.URL.Path, "/champion-mastery/") {
			body = `[{"championId":1,"championLevel":10,"championPoints":123456}]`
		}
		if body != "" {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		}
		return original.RoundTrip(r)
	})
	previews := 0
	ctx := context.WithValue(context.Background(), riotOverviewProgressKey{}, func(p gameplayOverview) {
		if !p.ProfilePending && p.Player.SummonerLevel == 760 && len(p.Ranks) > 0 && len(p.Masteries) > 0 {
			previews++
		}
	})
	result, err := f.a.loadRiotOverview(ctx, gameplayReference{PlayerRef: "r98-subject", Region: "kr", Privacy: "PRIVATE"}, 0, 20)
	if err != nil || len(result.Matches) != 20 || previews == 0 {
		t.Fatalf("missing ready metadata preview: %d err=%v", previews, err)
	}
	events := r90Events(t, f.a, "riot_match_mode")
	for _, event := range events {
		if event["queue_id"] == nil || event["game_mode"] == nil || event["queue_label"] == nil {
			t.Fatal(event)
		}
	}
	if len(events) != 20 {
		t.Fatalf("queue diagnostic count=%d", len(events))
	}
}
