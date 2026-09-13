package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newSummonerIdentityClient(t *testing.T, next Summoner, requests *atomic.Int64) *LCUClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/lol-summoner/v1/current-summoner" {
			http.NotFound(w, r)
			return
		}
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(next); err != nil {
			t.Errorf("encode summoner: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	return &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
}

func identityTestApp(client *LCUClient, current Summoner) *app {
	return &app{
		connected: true, lcu: client, summoner: current,
		refreshRequests:  make(chan struct{}, 1),
		eventSubscribers: make(map[chan string]struct{}),
		gameplayRefs:     make(map[string]string), gameplayRefDetails: make(map[string]gameplayReference),
	}
}

func TestRefreshSummonerIdentityRunsWhileSnapshotIsSyncing(t *testing.T) {
	current := Summoner{SummonerID: 7, PUUID: strings.Repeat("a", 48), GameName: "旧名字", ProfileIconID: 1}
	next := current
	next.ProfileIconID = 2
	var requests atomic.Int64
	client := newSummonerIdentityClient(t, next, &requests)
	a := identityTestApp(client, current)
	a.syncing = true

	changed, err := a.refreshSummonerIdentity(client)
	if err != nil || !changed {
		t.Fatalf("refreshSummonerIdentity changed=%v err=%v", changed, err)
	}
	if a.summoner.ProfileIconID != 2 || !a.syncing || requests.Load() != 1 {
		t.Fatalf("identity=%#v syncing=%v requests=%d", a.summoner, a.syncing, requests.Load())
	}
}

func TestRefreshSummonerIdentityDoesNotBroadcastWhenUnchanged(t *testing.T) {
	current := Summoner{SummonerID: 7, PUUID: strings.Repeat("b", 48), GameName: "未变化", ProfileIconID: 3, SummonerLevel: 40}
	var requests atomic.Int64
	client := newSummonerIdentityClient(t, current, &requests)
	updates := make(chan string, 1)
	a := identityTestApp(client, current)
	a.eventSubscribers[updates] = struct{}{}

	changed, err := a.refreshSummonerIdentity(client)
	if err != nil || changed {
		t.Fatalf("refreshSummonerIdentity changed=%v err=%v", changed, err)
	}
	select {
	case event := <-updates:
		t.Fatalf("unchanged identity broadcast %q", event)
	default:
	}
}

func TestRefreshSummonerIdentityProfileChangesDoNotRequestFullSnapshot(t *testing.T) {
	current := Summoner{SummonerID: 7, PUUID: strings.Repeat("c", 48), GameName: "旧名字", ProfileIconID: 4, SummonerLevel: 41}
	next := current
	next.GameName = "新名字"
	next.ProfileIconID = 5
	next.SummonerLevel = 42
	var requests atomic.Int64
	client := newSummonerIdentityClient(t, next, &requests)
	a := identityTestApp(client, current)

	if changed, err := a.refreshSummonerIdentity(client); err != nil || !changed {
		t.Fatalf("refreshSummonerIdentity changed=%v err=%v", changed, err)
	}
	select {
	case <-a.refreshRequests:
		t.Fatal("profile-only identity change requested a full snapshot")
	default:
	}
}

func TestRefreshSummonerIdentityAccountChangeClearsReferencesAndRequestsSnapshot(t *testing.T) {
	current := Summoner{SummonerID: 7, PUUID: strings.Repeat("d", 48), GameName: "账号一"}
	next := Summoner{SummonerID: 8, PUUID: strings.Repeat("e", 48), GameName: "账号二"}
	var requests atomic.Int64
	client := newSummonerIdentityClient(t, next, &requests)
	a := identityTestApp(client, current)
	a.token = "session-token"
	publicRef := a.registerGameplayReference(current.PUUID)
	if publicRef == "" {
		t.Fatal("failed to seed gameplay references")
	}

	if changed, err := a.refreshSummonerIdentity(client); err != nil || !changed {
		t.Fatalf("refreshSummonerIdentity changed=%v err=%v", changed, err)
	}
	if _, ok := a.resolveGameplayReference(publicRef); ok {
		t.Fatal("account change left gameplay references intact")
	}
	select {
	case <-a.refreshRequests:
	default:
		t.Fatal("account change did not request a full snapshot")
	}
}

func TestSummonerIdentityEventRejectsIncompletePayload(t *testing.T) {
	current := Summoner{SummonerID: 7, PUUID: strings.Repeat("f", 48), GameName: "完整身份", ProfileIconID: 6}
	a := identityTestApp(&LCUClient{}, current)
	if accepted := a.handleSummonerIdentityEvent(a.lcu, json.RawMessage(`{"puuid":"partial","profileIconId":99}`)); accepted {
		t.Fatal("event without summonerId was accepted")
	}
	if a.summoner != current {
		t.Fatalf("incomplete event overwrote identity: %#v", a.summoner)
	}
}

func TestSummonerIdentityEventDebouncesToLatestPayload(t *testing.T) {
	current := Summoner{SummonerID: 7, PUUID: strings.Repeat("j", 48), GameName: "旧名字", ProfileIconID: 6}
	client := &LCUClient{}
	a := identityTestApp(client, current)
	updates := make(chan string, 2)
	a.eventSubscribers[updates] = struct{}{}
	if !a.handleSummonerIdentityEvent(client, json.RawMessage(`{"summonerId":7,"puuid":"`+current.PUUID+`","gameName":"中间值","profileIconId":7}`)) {
		t.Fatal("first valid identity event was rejected")
	}
	if !a.handleSummonerIdentityEvent(client, json.RawMessage(`{"summonerId":7,"puuid":"`+current.PUUID+`","gameName":"最终值","profileIconId":8}`)) {
		t.Fatal("second valid identity event was rejected")
	}
	// State is committed before broadcastEvent runs. Waiting for the state and
	// then using a non-blocking receive races the publisher under -race/load.
	// Wait for the observable event, then check the state it announces.
	select {
	case event := <-updates:
		if event != "summoner-updated" {
			t.Fatalf("event=%q", event)
		}
	case <-time.After(time.Second):
		t.Fatal("debounced identity did not broadcast")
	}
	a.mu.RLock()
	got := a.summoner
	a.mu.RUnlock()
	if got.GameName != "最终值" || got.ProfileIconID != 8 {
		t.Fatalf("debounced identity=%#v", got)
	}
	select {
	case event := <-updates:
		t.Fatalf("debounced identity broadcast more than once: %q", event)
	default:
	}
}

func TestRefreshSummonerIdentitySingleflightSharesOneLCURequest(t *testing.T) {
	current := Summoner{SummonerID: 7, PUUID: strings.Repeat("g", 48), ProfileIconID: 7}
	next := current
	next.ProfileIconID = 8
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		once.Do(func() { close(started) })
		<-release
		_ = json.NewEncoder(w).Encode(next)
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := identityTestApp(client, current)

	results := make(chan error, 6)
	for index := 0; index < cap(results); index++ {
		go func() {
			_, err := a.refreshSummonerIdentity(client)
			results <- err
		}()
	}
	<-started
	time.Sleep(20 * time.Millisecond)
	close(release)
	for index := 0; index < cap(results); index++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if requests.Load() != 2 {
		t.Fatalf("singleflight sent %d LCU requests, want current summoner plus profile", requests.Load())
	}
}

func TestSummonerProfileEventUpdatesOnlyProfile(t *testing.T) {
	client := &LCUClient{}
	loot := []LootItem{{LootID: "keep", Count: 2}}
	a := identityTestApp(client, Summoner{SummonerID: 7})
	a.account = AccountData{Profile: SummonerProfile{BackgroundSkinID: 1}, Loot: loot}
	updates := make(chan string, 1)
	a.eventSubscribers[updates] = struct{}{}

	if !a.handleSummonerProfileEvent(client, json.RawMessage(`{"backgroundSkinId":103000,"backgroundSkinName":"新背景"}`)) {
		t.Fatal("valid summoner profile event was rejected")
	}
	if a.account.Profile.BackgroundSkinID != 103000 || a.account.Profile.BackgroundSkinName != "新背景" {
		t.Fatalf("profile was not updated: %#v", a.account.Profile)
	}
	if len(a.account.Loot) != 1 || a.account.Loot[0].LootID != "keep" {
		t.Fatalf("profile event replaced other account fields: %#v", a.account)
	}
	select {
	case event := <-updates:
		if event != "summoner-updated" {
			t.Fatalf("profile event broadcast %q", event)
		}
	default:
		t.Fatal("profile event did not broadcast summoner-updated")
	}
}

func TestSummonerFallbackIntervalIsSixtySeconds(t *testing.T) {
	if summonerFallbackInterval != 60*time.Second {
		t.Fatalf("summoner fallback interval=%s", summonerFallbackInterval)
	}
}

func TestHandleRefreshUpdatesIdentityBeforeQueueingFullSnapshot(t *testing.T) {
	current := Summoner{SummonerID: 7, PUUID: strings.Repeat("k", 48), ProfileIconID: 9}
	next := current
	next.ProfileIconID = 10
	var requests atomic.Int64
	client := newSummonerIdentityClient(t, next, &requests)
	a := identityTestApp(client, current)
	recorder := httptest.NewRecorder()
	a.handleRefresh(recorder, httptest.NewRequest(http.MethodPost, "/api/refresh", nil))
	if recorder.Code != http.StatusAccepted || a.summoner.ProfileIconID != 10 || requests.Load() != 1 {
		t.Fatalf("status=%d identity=%#v requests=%d", recorder.Code, a.summoner, requests.Load())
	}
	select {
	case <-a.refreshRequests:
	default:
		t.Fatal("manual refresh did not queue the full snapshot")
	}
}

func identityOverviewMatch(playerRef string) lcuGame {
	game := lcuGame{GameID: 9001, GameCreation: time.Now().UnixMilli(), GameDuration: 1800, QueueID: 420, GameMode: "CLASSIC", MapID: 11}
	for index := 0; index < 10; index++ {
		participantID := int64(index + 1)
		ref := fmt.Sprintf("player-%041d", index)
		if index == 0 {
			ref = playerRef
		}
		identity := lcuParticipantIdentity{ParticipantID: participantID}
		identity.Player.PUUID = ref
		identity.Player.GameName = fmt.Sprintf("玩家%d", index+1)
		identity.Player.TagLine = "CN1"
		participant := lcuParticipant{ParticipantID: participantID, TeamID: 100, ChampionID: 103}
		if index >= 5 {
			participant.TeamID = 200
		}
		participant.Stats.Win = index < 5
		game.ParticipantIdentities = append(game.ParticipantIdentities, identity)
		game.Participants = append(game.Participants, participant)
	}
	game.Teams = []lcuTeam{{TeamID: 100, Win: "Win"}, {TeamID: 200, Win: "Fail"}}
	return game
}

func newIdentityOverviewApp(t *testing.T, identityStatus int, next Summoner) (*app, *atomic.Int64) {
	t.Helper()
	current := next
	current.GameName = "旧名字"
	current.ProfileIconID = 11
	current.SummonerLevel = 50
	var identityRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/lol-summoner/v1/current-summoner":
			identityRequests.Add(1)
			if identityStatus != http.StatusOK {
				http.Error(w, "identity unavailable", identityStatus)
				return
			}
			_ = json.NewEncoder(w).Encode(next)
		case strings.HasPrefix(r.URL.Path, "/lol-match-history/v1/products/lol/"):
			history := lcuMatchHistory{}
			history.Games.GameCount = 1
			history.Games.Games = []lcuGame{identityOverviewMatch(current.PUUID)}
			_ = json.NewEncoder(w).Encode(history)
		case strings.HasPrefix(r.URL.Path, "/lol-ranked/"):
			_, _ = w.Write([]byte(`{"queues":[]}`))
		case strings.HasPrefix(r.URL.Path, "/lol-champion-mastery/") || r.URL.Path == "/lol-game-queues/v1/queues":
			_, _ = w.Write([]byte(`[]`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(server.Close)
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client(), region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	a := identityTestApp(client, current)
	a.token = "session-token"
	a.snapshotReady = true
	a.summonerIdentityAt = time.Now().Add(-time.Minute)
	a.lpTracker = newLPTracker(nil)
	return a, &identityRequests
}

func callCurrentOverview(t *testing.T, a *app, force bool) gameplayOverview {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/gameplay/overview?count=20&force=%d", map[bool]int{true: 1}[force]), nil)
	recorder := httptest.NewRecorder()
	a.handleGameplayOverview(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("overview status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response gameplayOverview
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func summonerCapability(response gameplayOverview) EndpointCapability {
	for _, capability := range response.Capabilities {
		if capability.Name == "summoner" {
			return capability
		}
	}
	return EndpointCapability{}
}

func TestCurrentOverviewForceRefreshesIdentityWhileNonForceUsesCachedIdentity(t *testing.T) {
	next := Summoner{SummonerID: 77, PUUID: strings.Repeat("h", 48), GameName: "新名字", TagLine: "CN1", ProfileIconID: 22, SummonerLevel: 51}
	forcedApp, forcedRequests := newIdentityOverviewApp(t, http.StatusOK, next)
	forced := callCurrentOverview(t, forcedApp, true)
	if forced.Player.ProfileIconID != 22 || forced.Player.GameName != "新名字" || forcedRequests.Load() != 1 {
		t.Fatalf("forced identity=%#v requests=%d", forced.Player, forcedRequests.Load())
	}
	forcedJSON, err := json.Marshal(forced)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(forcedJSON), next.PUUID) {
		t.Fatalf("forced overview leaked the stable PUUID: %s", forcedJSON)
	}
	forcedCapability := summonerCapability(forced)
	if forcedCapability.State != capabilityAvailable || forcedCapability.Count != 1 {
		t.Fatalf("forced summoner capability=%#v", forcedCapability)
	}

	cachedApp, cachedRequests := newIdentityOverviewApp(t, http.StatusOK, next)
	cached := callCurrentOverview(t, cachedApp, false)
	if cached.Player.ProfileIconID != 11 || cached.Player.GameName != "旧名字" || cachedRequests.Load() != 0 {
		t.Fatalf("cached identity=%#v requests=%d", cached.Player, cachedRequests.Load())
	}
	cachedCapability := summonerCapability(cached)
	if cachedCapability.Count != 0 || !strings.Contains(cachedCapability.Detail, "使用缓存身份") {
		t.Fatalf("cached summoner capability=%#v", cachedCapability)
	}
}

func TestCurrentOverviewIdentityFailureKeepsCompleteResponseAndDegradesCapability(t *testing.T) {
	next := Summoner{SummonerID: 77, PUUID: strings.Repeat("i", 48), GameName: "新名字", ProfileIconID: 33, SummonerLevel: 52}
	a, identityRequests := newIdentityOverviewApp(t, http.StatusServiceUnavailable, next)
	response := callCurrentOverview(t, a, true)
	if response.Player.ProfileIconID != 11 || response.Player.GameName != "旧名字" || len(response.Matches) != 1 {
		t.Fatalf("degraded overview lost cached identity or matches: player=%#v matches=%d", response.Player, len(response.Matches))
	}
	capability := summonerCapability(response)
	if capability.State != capabilityFailed || !strings.Contains(capability.Detail, "沿用上次读取的身份") || identityRequests.Load() != 1 {
		t.Fatalf("degraded capability=%#v requests=%d", capability, identityRequests.Load())
	}
}
