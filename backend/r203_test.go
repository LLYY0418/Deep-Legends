package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR203DirtyEventsRecordDuringRefreshAndSuppressFiveSecondsAfter(t *testing.T) {
	a := r175App(t)
	end := time.Now()
	a.collectionRefreshFinishedAt = end
	if a.markCollectionDirtyAt(end.Add(3*time.Second), "loot") || a.collectionDirty {
		t.Fatal("refresh echo marked dirty")
	}
	if !a.markCollectionDirtyAt(end.Add(6*time.Second), "inventory") || !a.collectionDirty {
		t.Fatal("external change lost")
	}
	a.collectionDirty = false
	a.syncing = true
	if !a.markCollectionDirtyAt(end.Add(time.Minute), "champions") || !a.collectionDirty {
		t.Fatal("genuine in-flight inventory change lost")
	}
	rows := r175Events(t, a, "collection_dirty_marked")
	if len(rows) != 3 {
		t.Fatal(rows)
	}
	for i, want := range []bool{true, false, false} {
		if rows[i]["suppressed"] != want || rows[i]["during_refresh"] != (i == 2) {
			t.Fatal(rows)
		}
	}
	if rows[0]["uri_kind"] != "loot" || rows[1]["uri_kind"] != "inventory" || rows[2]["uri_kind"] != "champions" {
		t.Fatal(rows)
	}
	for uri, want := range map[string]string{"/lol-inventory/v2/inventory/CHAMPION_SKIN": "inventory", "/lol-loot/v1/player-loot-map": "loot", "/lol-champions/v1/inventories/current": "champions", "/private/account/name": "other"} {
		if collectionURIKind(uri) != want {
			t.Fatal(uri)
		}
	}
}
func TestR203RefreshSourcesAreAllowlistedAndPreserved(t *testing.T) {
	a := r175App(t)
	a.refreshRequests = make(chan struct{}, 1)
	for _, source := range []string{"user_refresh", "overlay_retry", "dirty_rescan", "ensure", "event", "private/path"} {
		a.requestCollectionRefresh(source)
	}
	rows := r175Events(t, a, "collection_refresh_request")
	if len(rows) != 5 {
		t.Fatal(rows)
	}
	for i, want := range []string{"user_refresh", "overlay_retry", "dirty_rescan", "ensure", "event"} {
		if rows[i]["source"] != want {
			t.Fatal(rows)
		}
	}
}
func TestR203IdentityRetryNeverRequestsCatalogEvenForInitialIdentity(t *testing.T) {
	var calls atomic.Int64
	client := newSummonerIdentityClient(t, Summoner{SummonerID: 7, PUUID: strings.Repeat("x", 48)}, &calls)
	a := identityTestApp(client, Summoner{})
	a.storage = r175App(t).storage
	w := httptest.NewRecorder()
	a.handleIdentityRefresh(w, httptest.NewRequest("POST", "/api/identity/refresh?source=overlay_retry", nil))
	if w.Code != http.StatusAccepted || len(a.refreshRequests) != 0 || a.collectionRequested || calls.Load() != 1 {
		t.Fatal(w.Code, a.collectionRequested, calls.Load())
	}
}
func TestR203IdentityAndLPWaitersHonorCancellation(t *testing.T) {
	client := &LCUClient{}
	a := &app{summonerIdentityFlight: &summonerIdentityFlight{client: client, done: make(chan struct{})}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.refreshSummonerIdentityContext(ctx, client); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := waitLPStartFlight(ctx, make(chan struct{})); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestR203SkinDetailsPropagatesRequestCancellation(t *testing.T) {
	finished := make(chan struct{}, 2)
	started := make(chan struct{}, 2)
	client := &LCUClient{baseURL: "https://fixture", token: "fixture-token", http: &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		started <- struct{}{}
		<-r.Context().Done()
		finished <- struct{}{}
		return nil, r.Context().Err()
	})}}
	a := &app{lcu: client, connected: true, snapshotReady: true, allSkins: []Skin{{ID: 1, ChampionID: 1}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		a.handleSkinDetails(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/skin-details?id=1", nil).WithContext(ctx))
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("detail request did not start")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handler ignored cancellation")
	}
	for i := 0; i < 2; i++ {
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Fatal("worker ignored cancellation")
		}
	}
}

func TestR203HeapTopOnlyFunctionsAndMegabytesSorted(t *testing.T) {
	pc, _, _, _ := runtime.Caller(0)
	records := []runtime.MemProfileRecord{}
	for i := int64(1); i <= 20; i++ {
		r := runtime.MemProfileRecord{AllocBytes: i * 1024 * 1024, FreeBytes: 0}
		r.Stack0[0] = pc
		records = append(records, r)
	}
	rows := heapProfileTop(records)
	if len(rows) != 15 || rows[0].MB != 20 || rows[14].MB != 6 {
		t.Fatal(rows)
	}
	for _, row := range rows {
		if strings.Contains(row.Function, "/") || !strings.Contains(row.Function, "TestR203HeapTop") {
			t.Fatal(row)
		}
	}
}
func TestR203LobbyHeapProfileOncePerGameEvenWhenSettingsUnavailable(t *testing.T) {
	a := r175App(t)
	client := &LCUClient{baseURL: "https://fixture", token: "fixture-token", http: &http.Client{Transport: gameplayRoundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("unavailable") })}}
	a.gameSettingsWatch.client = client
	a.gameSettingsWatch.prev = map[string]gameSettingsFileSnapshot{}
	for generation := uint64(1); generation <= 5; generation++ {
		a.gameSettingsWatch.generation = generation
		a.gameSettingsWatch.heapProfileCaptured = false
		job := gameSettingsWatchJob{stage: "lobby_after_60s", generation: generation, delayed: true}
		a.recordGameSettingsWatch(context.Background(), client, job)
		a.recordGameSettingsWatch(context.Background(), client, job)
	}
	if rows := r175Events(t, a, "heap_profile_top"); len(rows) != 5 {
		t.Fatal("heap samples", len(rows))
	}
}
func TestR203FiveGamesCacheCountsStabilize(t *testing.T) {
	t.Parallel()
	a := &app{}
	sgp := &sgpProvider{}
	overview := newOverviewQueryCache()
	timeline := newMatchTimelineCache()
	lp := &lpTracker{history: lpHistoryData{Baselines: map[string]map[string]lpSnapshot{}, BaselineInfo: map[string]map[string]lpBaselineInfo{}, GameStarts: map[string]map[string]lpGameStartBaseline{}, Games: map[string]lpGameRecord{}}}
	riot := &riotProvider{}
	accounts := map[string]riotAccountCacheEntry{}
	specialists := map[string]specialistRecentSummaryCacheEntry{}
	now := time.Now()
	var baseline []int
	for game := 0; game < 5; game++ {
		for i := 0; i < 1300; i++ {
			key := fmt.Sprintf("%d:%d", game, i)
			a.storeAssetLocked(key, []byte("fixture"))
			_, _ = a.loadAsset(context.Background(), "bad:"+key, 64, time.Hour, func(context.Context) ([]byte, error) { return nil, errors.New("missing") })
			overview.putLocked(key, overviewQueryCacheEntry{at: now, response: gameplayOverview{}})
			timeline.put(key, matchTimelineResponse{})
			riot.storeMatchLocked(key, &riotMatch{Info: riotMatchInfo{GameID: int64(i + 1), Participants: make([]riotParticipant, 10)}})
			sgp.cacheHistoryPage("HN1", key, 0, 10, nil, sgpHistoryPage{bytes: 16, games: []*riotMatchInfo{{GameID: int64(i + 1)}}})
			a.cachedLivePlayerMatches(context.Background(), key, func(context.Context) livePlayerMatchesResult {
				return livePlayerMatchesResult{State: "ready", Matches: []gameplayMatch{{GameID: int64(i + 1)}}}
			})
			lp.history.Baselines[key] = map[string]lpSnapshot{}
			lp.history.BaselineInfo[key] = map[string]lpBaselineInfo{"ranked": {TakenAt: int64(game*1300 + i + 1)}}
			lp.history.GameStarts[key] = map[string]lpGameStartBaseline{}
			lp.history.Games[key] = lpGameRecord{RecordedAt: int64(game*1300 + i)}
			lp.pruneHistoryLocked()
			accounts[key] = riotAccountCacheEntry{expiresAt: now.Add(time.Hour)}
			pruneTTLCache(accounts, now, 512, func(v riotAccountCacheEntry) time.Time { return v.expiresAt })
			specialists[key] = specialistRecentSummaryCacheEntry{expiresAt: now.Add(time.Hour)}
			pruneTTLCache(specialists, now, 256, func(v specialistRecentSummaryCacheEntry) time.Time { return v.expiresAt })
		}
		counts := []int{len(a.assetCache), len(a.assetFailureUntil), len(overview.entries), len(timeline.entries), len(sgp.historyCache), len(a.livePlayerMatchCache), len(lp.history.Baselines), len(lp.history.BaselineInfo), len(lp.history.GameStarts), len(lp.history.Games), len(accounts), len(specialists), len(riot.matchCache)}
		if game == 0 {
			baseline = counts
		} else if !reflect.DeepEqual(baseline, counts) {
			t.Fatalf("game %d counts grew: %v -> %v", game+1, baseline, counts)
		}
		if a.assetCacheBytes > assetCacheMaxBytes || sgp.historyBytes > sgpCacheMaxBytes || counts[1] > 1200 || counts[6] > 64 || counts[9] > 400 {
			t.Fatal(counts)
		}
	}
	for key, entry := range accounts {
		entry.expiresAt = now.Add(-time.Second)
		accounts[key] = entry
	}
	pruneTTLCache(accounts, now, 512, func(v riotAccountCacheEntry) time.Time { return v.expiresAt })
	if len(accounts) != 0 {
		t.Fatal("unused expired keys retained")
	}
	a.pruneAssetFailuresLocked(now.Add(2 * time.Hour))
	if len(a.assetFailureUntil) != 0 {
		t.Fatal("negative cache retained")
	}
}
func TestR203BlockingDiagnosticsAcceptOnlyFixedSources(t *testing.T) {
	a := r175App(t)
	for _, source := range []string{"startup", "skin", "champselect", "private/name"} {
		w := httptest.NewRecorder()
		a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(fmt.Sprintf(`{"event":"blocking_state_client","reason":"show","source":%q}`, source))))
		if (w.Code == 204) != (source != "private/name") {
			t.Fatal(source, w.Code)
		}
	}
}

func TestR203SmallDiagnosticAndProfileCachesAreBounded(t *testing.T) {
	a := &app{}
	defer a.clearFacadeEventThrottle()
	profiles := &proProfileCache{entries: map[string]opggProAccount{}}
	for i := 0; i < 1500; i++ {
		key := fmt.Sprint(i)
		a.allowPerkDiagnostic(key)
		a.recordUnknownQueue(int64(10000+i), "other", 11)
		a.setQueueFilterCapability("HN1", key, "supported")
		a.facadeEventChanged(LCUEvent{URI: "/lol-regalia/v1/" + key, Data: []byte(`{"selected":1}`)})
		profiles.entries[key] = opggProAccount{CheckedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		pruneProProfilesLocked(profiles, time.Now())
	}
	if len(a.perkDiagnosticCounts) > 256 || len(a.unknownQueueDiagnosticIDs) > 128 || len(a.queueFilterCapabilities) > 256 || len(a.facadeEventFingerprints) > 256 || len(profiles.entries) > 128 {
		t.Fatal("unbounded small cache")
	}
}
func TestR203LPDiagnosticCallbackRunsWithoutTrackerLock(t *testing.T) {
	store := r175App(t).storage
	tkr := newLPTracker(store)
	calls := 0
	tkr.observeEvent = func(map[string]any) {
		if !tkr.mu.TryLock() {
			t.Error("diagnostics under tracker lock")
			return
		}
		tkr.mu.Unlock()
		calls++
	}
	tkr.observe("fixture-player", []gameplayRank{{QueueType: "RANKED_SOLO_5x5", Tier: "GOLD", Division: "IV", Wins: 4, Losses: 3}})
	if calls == 0 {
		t.Fatal("missing baseline diagnostic")
	}
}
func TestR203LiveHistoryPanicReleasesFlightForNextRequest(t *testing.T) {
	a := &app{}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("missing panic")
			}
		}()
		a.cachedLivePlayerMatches(context.Background(), "key", func(context.Context) livePlayerMatchesResult { panic("fixture") })
	}()
	if len(a.livePlayerMatchFlights) != 0 {
		t.Fatal("flight retained")
	}
	result := a.cachedLivePlayerMatches(context.Background(), "key", func(context.Context) livePlayerMatchesResult { return livePlayerMatchesResult{State: "ready"} })
	if result.State != "ready" {
		t.Fatal(result)
	}
}

func TestR203CollectionRenderFieldsReachDiagnostics(t *testing.T) {
	a := r175App(t)
	w := httptest.NewRecorder()
	a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(`{"event":"collection_render_client","reason":"unchanged-suppressed","view":"owned","itemCount":1183,"force":true,"keptVisible":true}`)))
	rows := r175Events(t, a, "collection_render_client")
	if w.Code != 204 || len(rows) != 1 || rows[0]["item_count"] != float64(1183) || rows[0]["kept_visible"] != true || rows[0]["force"] != true || rows[0]["view"] != "owned" {
		t.Fatal(w.Code, rows)
	}
}
func TestR203SkinDetailsPanicDoesNotReturnFalseSuccess(t *testing.T) {
	client := &LCUClient{baseURL: "https://fixture", token: "fixture-token", http: &http.Client{Transport: gameplayRoundTripFunc(func(*http.Request) (*http.Response, error) { panic("fixture") })}}
	a := &app{lcu: client, connected: true, snapshotReady: true, allSkins: []Skin{{ID: 1, ChampionID: 1}}}
	w := httptest.NewRecorder()
	a.handleSkinDetails(w, httptest.NewRequest("GET", "/api/skin-details?id=1", nil))
	if w.Code != http.StatusBadGateway {
		t.Fatal(w.Code)
	}
}

func TestR203FacadePanicReleasesWaitersAndAllowsRetry(t *testing.T) {
	a := identityTestApp(&LCUClient{}, Summoner{SummonerID: 7})
	a.facadeView.load = func(context.Context, string) facadeState { panic("fixture") }
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	value, err := a.cachedFacadeState(ctx, false, "poll")
	if err != nil || !value.SkinsUnavailable || a.facadeView.flight != nil {
		t.Fatal(value, err)
	}
	a.facadeView.load = func(context.Context, string) facadeState { return facadeState{ChallengesReady: true} }
	next, err := a.cachedFacadeState(ctx, true, "manual")
	if err != nil || !next.ChallengesReady {
		t.Fatal(next, err)
	}
}
