package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

type gameplayRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn gameplayRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestGameplayReferenceDoesNotExposeStablePlayerID(t *testing.T) {
	a := &app{token: "session-secret", gameplayRefs: make(map[string]string)}
	raw := strings.Repeat("p", 48)
	public := a.registerGameplayReference(raw)
	if public == "" || public == raw || strings.Contains(public, raw) {
		t.Fatalf("public reference leaked stable id: %q", public)
	}
	if resolved, ok := a.resolveGameplayReference(public); !ok || resolved != raw {
		t.Fatalf("reference did not resolve: %q, %v", resolved, ok)
	}
	a.clearGameplayReferences()
	if _, ok := a.resolveGameplayReference(public); ok {
		t.Fatal("reference survived session reset")
	}
	if !gameplaySummonerChanged(Summoner{}, Summoner{PUUID: raw}) || gameplaySummonerChanged(Summoner{PUUID: raw}, Summoner{PUUID: raw}) {
		t.Fatal("account transition did not invalidate aliases exactly once")
	}
}

func TestGameplayReferenceCacheEvictsBothTablesTogether(t *testing.T) {
	a := &app{token: "session-secret"}
	aliases := make([]string, 0, gameplayReferenceCacheMax+1)
	for index := 0; index <= gameplayReferenceCacheMax; index++ {
		playerRef := fmt.Sprintf("player-reference-%032d", index)
		aliases = append(aliases, a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: playerRef, GameName: fmt.Sprintf("Player %d", index)}))
	}
	a.gameplayRefsMu.Lock()
	refsLen, detailsLen := len(a.gameplayRefs), len(a.gameplayRefDetails)
	orderLen, entriesLen := a.gameplayRefOrder.Len(), len(a.gameplayRefEntries)
	_, oldestRefExists := a.gameplayRefs[aliases[0]]
	_, oldestDetailsExist := a.gameplayRefDetails[aliases[0]]
	a.gameplayRefsMu.Unlock()
	if refsLen != gameplayReferenceCacheMax || detailsLen != gameplayReferenceCacheMax || orderLen != gameplayReferenceCacheMax || entriesLen != gameplayReferenceCacheMax {
		t.Fatalf("reference cache sizes refs=%d details=%d order=%d entries=%d, want %d", refsLen, detailsLen, orderLen, entriesLen, gameplayReferenceCacheMax)
	}
	if oldestRefExists || oldestDetailsExist {
		t.Fatal("reference cache did not evict the oldest alias from both tables")
	}
	if _, ok := a.resolveGameplayReferenceDetails(aliases[len(aliases)-1]); !ok {
		t.Fatal("reference cache evicted the newest alias")
	}
}

func TestGameplayReferenceCacheUpdatesWithoutGrowing(t *testing.T) {
	a := &app{token: "session-secret"}
	playerRef := strings.Repeat("r", 48)
	first := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: playerRef, GameName: "First"})
	second := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: playerRef, GameName: "Updated"})
	if first != second {
		t.Fatalf("stable account produced different aliases: %q != %q", first, second)
	}
	a.gameplayRefsMu.Lock()
	refsLen, detailsLen, orderLen := len(a.gameplayRefs), len(a.gameplayRefDetails), a.gameplayRefOrder.Len()
	a.gameplayRefsMu.Unlock()
	if refsLen != 1 || detailsLen != 1 || orderLen != 1 {
		t.Fatalf("updating one alias grew reference cache: refs=%d details=%d order=%d", refsLen, detailsLen, orderLen)
	}
}

func TestGameplayOverviewRejectsUnregisteredStablePlayerID(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	a := &app{token: "session-secret", gameplayRefs: make(map[string]string), storage: store}
	request := httptest.NewRequest(http.MethodPost, "/api/gameplay/overview", strings.NewReader(`{"playerRef":"`+strings.Repeat("p", 48)+`","count":20}`))
	recorder := httptest.NewRecorder()
	a.handleGameplayOverview(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	line := string(data)
	for _, field := range []string{`"event":"overview_load_cost"`, `"sgp_requests"`, `"sgp_bytes"`, `"sgp_history_calls"`, `"sgp_history_cache_hits"`, `"duration_ms"`} {
		if !strings.Contains(line, field) {
			t.Fatalf("overview cost diagnostic missing %s: %s", field, line)
		}
	}
}

func newGameplayOverviewSGPFixture(t *testing.T, withMatch bool) (*app, string, *atomic.Int64) {
	t.Helper()
	playerRef := strings.Repeat("o", 48)
	currentRef := strings.Repeat("c", 48)
	var summaryRequests atomic.Int64
	sgpHTTP := &http.Client{Transport: sgpRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		status, body := http.StatusOK, ""
		switch {
		case strings.Contains(request.URL.Path, "/match-history-query/") && strings.HasSuffix(request.URL.Path, "/SUMMARY"):
			summaryRequests.Add(1)
			if !withMatch {
				body = `{"games":[]}`
				break
			}
			queueID := int64(420)
			if request.URL.Query().Get("tag") == "q_440" {
				queueID = 440
			}
			game := fmt.Sprintf(`{"gameId":%d,"queueId":%d,"gameCreation":%d,"gameDuration":1800,"participants":[{"puuid":"%s","participantId":1,"teamId":100,"championId":103,"win":true}]}`, queueID, queueID, time.Now().UnixMilli(), playerRef)
			body = `{"games":[{"json":` + game + `}]}`
		case strings.Contains(request.URL.Path, "/summoner-ledge/"):
			body = `[{"puuid":"` + playerRef + `","name":"测试玩家","level":30}]`
		case strings.Contains(request.URL.Path, "/leagues-ledge/"):
			body = `{"queues":[]}`
		default:
			status, body = http.StatusNotFound, "unexpected SGP endpoint"
		}
		return &http.Response{
			StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)),
			ContentLength: int64(len(body)), Request: request,
		}, nil
	})}

	lcuHTTP := &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := `[]`
		if strings.Contains(request.URL.Path, "/lol-ranked/") {
			body = `{"queues":[]}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})}
	client := &LCUClient{baseURL: "https://lcu.invalid", token: "lcu-token", http: lcuHTTP, region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	provider := newSGPProvider()
	provider.http = sgpHTTP
	provider.serverBases["HN1"] = "https://sgp.invalid"
	provider.token, provider.tokenAt, provider.tokenClient = "entitlements", time.Now(), client
	provider.sessionToken, provider.sessionAt, provider.sessionOwner = "league-session", time.Now(), client
	a := &app{
		token: "session-secret", connected: true, lcu: client, sgp: provider,
		summoner:     Summoner{PUUID: currentRef, GameName: "当前玩家"},
		gameplayRefs: make(map[string]string), gameplayRefDetails: make(map[string]gameplayReference),
	}
	publicRef := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: playerRef, ServerID: "HN1", GameName: "测试玩家"})
	return a, publicRef, &summaryRequests
}

func callGameplayOverviewForTest(t *testing.T, a *app, publicRef string) {
	t.Helper()
	body := strings.NewReader(`{"playerRef":"` + publicRef + `","count":20}`)
	recorder := httptest.NewRecorder()
	a.handleGameplayOverview(recorder, httptest.NewRequest(http.MethodPost, "/api/gameplay/overview", body))
	if recorder.Code != http.StatusOK {
		t.Fatalf("overview status = %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestGameplayOverviewTencentLookupHTTPErrorIsExplainedAndRecorded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/lol-summoner/v1/alias/lookup") {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "fixture failure", http.StatusInternalServerError)
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client(), region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	if err := os.MkdirAll(filepath.Join(store.root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	a := &app{connected: true, lcu: client, summoner: Summoner{PUUID: strings.Repeat("c", 48)}, storage: store}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/gameplay/overview", strings.NewReader(`{"gameName":"测试玩家","tagLine":"12345","serverId":"HN1","count":20}`))
	a.handleGameplayOverview(recorder, request)
	if recorder.Code != http.StatusBadGateway || !strings.Contains(recorder.Body.String(), "HTTP 500") {
		t.Fatalf("lookup response = status:%d body:%q", recorder.Code, recorder.Body.String())
	}
	diagnostics, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	line := string(diagnostics)
	for _, field := range []string{`"event":"tencent_riot_id_lookup"`, `"outcome":"http-error"`, `"http_status":500`, `"remote_lookup":false`, `"server_id":"HN1"`} {
		if !strings.Contains(line, field) {
			t.Fatalf("lookup diagnostic missing %s: %s", field, line)
		}
	}
	if strings.Contains(line, "测试玩家") || strings.Contains(line, "12345") {
		t.Fatalf("lookup diagnostic leaked Riot ID: %s", line)
	}
}

func TestTencentLookupRetriesOneTransportFailure(t *testing.T) {
	var calls atomic.Int64
	puuid := strings.Repeat("r", 48)
	client := &LCUClient{
		baseURL: "https://lcu.invalid", token: "test-token", region: "TENCENT", rsoPlatform: "HN1", platformProbe: true,
		http: &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			if calls.Add(1) == 1 {
				return nil, &net.DNSError{Err: "temporary lookup failure", IsTemporary: true}
			}
			body := `[{"puuid":"` + puuid + `"}]`
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
		})},
	}
	resolved, err := (&app{}).resolveTencentRiotID(context.Background(), client, "测试玩家", "12345", "HN1")
	if err != nil || resolved.PlayerRef != puuid || calls.Load() != 2 {
		t.Fatalf("retried lookup = resolved:%#v calls:%d err:%v", resolved, calls.Load(), err)
	}
}

func TestGameplayOverviewRecordsCompletePhaseTiming(t *testing.T) {
	a, publicRef, _ := newGameplayOverviewSGPFixture(t, false)
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	if err := os.MkdirAll(filepath.Join(store.root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	a.storage = store
	a.sgp.observe = a.recordDiagnostic
	callGameplayOverviewForTest(t, a, publicRef)
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	var costDuration float64 = -1
	var phases map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		var event map[string]any
		if json.Unmarshal(line, &event) != nil {
			continue
		}
		switch event["event"] {
		case "overview_load_cost":
			costDuration, _ = event["duration_ms"].(float64)
		case "overview_phases_ms":
			phases, _ = event["load_phases_ms"].(map[string]any)
		}
	}
	if costDuration < 0 || phases == nil {
		t.Fatalf("missing overview timing diagnostics: %s", data)
	}
	phaseSum := 0.0
	for _, name := range overviewPhaseNames {
		value, ok := phases[name].(float64)
		if !ok {
			t.Fatalf("phase %q missing from %#v", name, phases)
		}
		phaseSum += value
	}
	total, ok := phases["total"].(float64)
	if !ok || math.Abs(total-costDuration) >= 50 || phaseSum+50 < total {
		t.Fatalf("phase accounting = sum:%v total:%v cost:%v phases:%#v", phaseSum, total, costDuration, phases)
	}
}

func TestGameplayOverviewOverlapsIndependentUpstreams(t *testing.T) {
	a, publicRef, _ := newGameplayOverviewSGPFixture(t, true)
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	if err := os.MkdirAll(filepath.Join(store.root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	a.storage = store
	a.sgp.observe = a.recordDiagnostic

	originalSGPTransport := a.sgp.http.Transport
	var delayedSGP atomic.Bool
	a.sgp.http.Transport = gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if strings.Contains(request.URL.Path, "/match-history-query/") && delayedSGP.CompareAndSwap(false, true) {
			time.Sleep(150 * time.Millisecond)
		}
		return originalSGPTransport.RoundTrip(request)
	})
	originalLCUTransport := a.lcu.http.Transport
	a.lcu.http.Transport = gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if strings.Contains(request.URL.Path, "/lol-ranked/") || strings.Contains(request.URL.Path, "/lol-champion-mastery/") {
			time.Sleep(150 * time.Millisecond)
		}
		return originalLCUTransport.RoundTrip(request)
	})

	started := time.Now()
	callGameplayOverviewForTest(t, a, publicRef)
	if elapsed := time.Since(started); elapsed >= 400*time.Millisecond {
		t.Fatalf("independent overview upstreams were serialized: elapsed=%s", elapsed)
	}

	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	var phases map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		var event map[string]any
		if json.Unmarshal(line, &event) == nil && event["event"] == "overview_phases_ms" {
			phases, _ = event["load_phases_ms"].(map[string]any)
		}
	}
	if phases == nil {
		t.Fatalf("missing phase diagnostics: %s", data)
	}
	span := func(name string) [2]float64 {
		values, _ := phases["spans"].(map[string]any)
		entries, _ := values[name].([]any)
		if len(entries) == 0 {
			t.Fatalf("phase %q has no measured span: %#v", name, phases["spans"])
		}
		pair, ok := entries[0].([]any)
		if !ok || len(pair) != 2 {
			t.Fatalf("phase %q span has unexpected shape: %#v", name, entries[0])
		}
		return [2]float64{pair[0].(float64), pair[1].(float64)}
	}
	overlaps := func(left, right [2]float64) bool { return left[0] < right[1] && right[0] < left[1] }
	detailed, ranks, mastery := span("detailed_matches"), span("ranks"), span("mastery")
	if !overlaps(detailed, ranks) || !overlaps(detailed, mastery) {
		t.Fatalf("expected detailed matches to overlap ranks and mastery: detailed=%v ranks=%v mastery=%v", detailed, ranks, mastery)
	}
}

func TestGameplayOverviewReturnsPartialBeforeTwentySeconds(t *testing.T) {
	a, publicRef, _ := newGameplayOverviewSGPFixture(t, true)
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	if err := os.MkdirAll(filepath.Join(store.root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	a.storage = store
	a.sgp.observe = a.recordDiagnostic
	var expire context.CancelFunc
	a.overviewTimeout = func(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
		if d != 18*time.Second {
			t.Fatalf("budget=%v", d)
		}
		ctx, cancel := context.WithCancel(parent)
		expire = cancel
		return r86DeadlineContext{ctx}, cancel
	}
	var summaryCalls atomic.Int64
	a.sgp.http = &http.Client{Transport: sgpRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if !strings.HasSuffix(request.URL.Path, "/SUMMARY") {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`[]`)), Request: request}, nil
		}
		if summaryCalls.Add(1) >= 3 {
			expire() // Drive the real cancellation route after two completed pages.
			<-request.Context().Done()
			return nil, request.Context().Err()
		}
		start, _ := strconv.Atoi(request.URL.Query().Get("startIndex"))
		games := make([]map[string]any, 0, 20)
		for index := 0; index < 20; index++ {
			games = append(games, map[string]any{"json": map[string]any{
				"gameId": start + index + 1, "queueId": 420, "gameCreation": time.Now().UnixMilli(), "gameDuration": 1800,
				"participants": []map[string]any{{"puuid": strings.Repeat("o", 48), "participantId": 1, "teamId": 100, "championId": 103, "win": true}},
			}})
		}
		body, _ := json.Marshal(map[string]any{"games": games})
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Request: request}, nil
	})}
	requestContext, cancel := context.WithTimeout(context.Background(), 21*time.Second)
	defer cancel()
	request := httptest.NewRequest(http.MethodPost, "/api/gameplay/overview", strings.NewReader(`{"playerRef":"`+publicRef+`","count":50}`)).WithContext(requestContext)
	recorder := httptest.NewRecorder()
	started := time.Now()
	a.handleGameplayOverview(recorder, request)
	elapsed := time.Since(started)
	var overview gameplayOverview
	if err := json.Unmarshal(recorder.Body.Bytes(), &overview); err != nil {
		t.Fatalf("partial overview response = status:%d body:%q err:%v", recorder.Code, recorder.Body.String(), err)
	}
	if recorder.Code != http.StatusOK || elapsed >= time.Second || len(overview.Matches) == 0 || !overview.Pagination.Partial || !overview.Pagination.BudgetExceeded || !overview.Pagination.HasMore || summaryCalls.Load() < 3 {
		t.Fatalf("budgeted overview = status:%d elapsed:%s matches:%d pagination:%#v calls:%d", recorder.Code, elapsed, len(overview.Matches), overview.Pagination, summaryCalls.Load())
	}
}

func TestGameplayOverviewHistoryWindowUsesCacheAcrossRequests(t *testing.T) {
	a, publicRef, summaryRequests := newGameplayOverviewSGPFixture(t, false)
	callGameplayOverviewForTest(t, a, publicRef)
	firstRequestCount := int(summaryRequests.Load())
	if firstRequestCount != 3 {
		t.Fatalf("first overview summary requests = %d, want one shared physical page and two ranked samples", firstRequestCount)
	}
	callGameplayOverviewForTest(t, a, publicRef)
	if int(summaryRequests.Load()) != firstRequestCount {
		t.Fatalf("cached overview sent another summary request: first=%d second=%d", firstRequestCount, summaryRequests.Load())
	}
	forceRecorder := httptest.NewRecorder()
	forceBody := strings.NewReader(`{"playerRef":"` + publicRef + `","count":20,"force":true}`)
	a.handleGameplayOverview(forceRecorder, httptest.NewRequest(http.MethodPost, "/api/gameplay/overview", forceBody))
	if forceRecorder.Code != http.StatusOK || int(summaryRequests.Load()) <= firstRequestCount {
		t.Fatalf("force refresh did not bypass physical SGP cache: status=%d first=%d afterForce=%d", forceRecorder.Code, firstRequestCount, summaryRequests.Load())
	}
}

func TestGameplayOverviewSkipsHistoryWindowWhenFirstPageHasMatches(t *testing.T) {
	a, publicRef, summaryRequests := newGameplayOverviewSGPFixture(t, true)
	callGameplayOverviewForTest(t, a, publicRef)
	if summaryRequests.Load() != 3 {
		t.Fatalf("overview with first-page matches sent %d summary requests, want page plus two ranked samples", summaryRequests.Load())
	}
}

func TestGameplayOverviewSearchOtherPlayerLoadsBothRankedSamples(t *testing.T) {
	a, publicRef, summaryRequests := newGameplayOverviewSGPFixture(t, true)
	callGameplayOverviewForTest(t, a, publicRef)
	// The fixture's reference is a different PUUID from the logged-in player,
	// so this exercises the search/other-player path rather than the current tab.
	if summaryRequests.Load() != 3 {
		t.Fatalf("other-player overview sent %d summary requests, want detail page plus q420/q440", summaryRequests.Load())
	}
}

func TestOverviewHistoryOnlyLoadsWhenTheFirstScreenHasNoMatches(t *testing.T) {
	reference := gameplayReference{ServerID: "HN1"}
	playerRef := strings.Repeat("p", 48)
	if shouldLoadOverviewHistory(reference, playerRef, []gameplayMatch{{GameID: 1}}) {
		t.Fatal("invalid short-circuit: a populated first screen would trigger the 30-day query")
	}
	if !shouldLoadOverviewHistory(reference, playerRef, nil) {
		t.Fatal("empty first screen should allow the fallback history query")
	}
	if shouldLoadOverviewHistory(gameplayReference{}, playerRef, nil) {
		t.Fatal("history query must require a selected server")
	}
}

func TestGameplayCoreOptionsPreserveTheFullUpstreamLimit(t *testing.T) {
	const want = 15
	rows := make([]championMetricRow, want+1)
	for index := range rows {
		rows[index].Assets = []championAsset{{ID: index + 1}}
	}
	options := recommendationOptions(rows)
	got := capGameplayCoreOptions(options)
	if len(got) != want || got[0].IDs[0] != 1 || got[want-1].IDs[0] != want {
		t.Fatalf("core options = %#v, want the first %d options", got, want)
	}
}

func TestGameplayHistoryUsesRequestedPageWindow(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"games":{"gameCount":87,"games":[]}}`))
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	_, _, total := loadGameplayHistory(client, strings.Repeat("p", 48), false, 30, 50, true)
	if gotQuery != "begIndex=30&endIndex=79" {
		t.Fatalf("query = %q, want paged window", gotQuery)
	}
	if total != 87 {
		t.Fatalf("total = %d, want 87", total)
	}
	if clampMatchCount(50) != 50 || clampMatchCount(51) != 50 || clampMatchStart(-1) != 0 {
		t.Fatal("gameplay page limits changed")
	}
}

func TestGameplayPerkStylesAcceptClientWrapperAndBareArray(t *testing.T) {
	fixtures := map[string]string{
		"wrapped": `{"schemaVersion":2,"styles":[{"id":8000,"name":"精密","iconPath":"/lol-game-data/assets/v1/perk-images/styles/precision/precision.png","slots":[{"perks":[8005,8006]},{"type":"kStatMod","perks":[5005,5008,5007]},{"type":"kStatMod","perks":[5008,5010,5001]},{"type":"kStatMod","perks":[5011,5013,5001]}]}]}`,
		"array":   `[{"id":8100,"name":"主宰","iconPath":"/lol-game-data/assets/v1/perk-images/styles/domination/domination.png","slots":[{"perks":[8112]}]}]`,
		"objects": `[{"id":8200,"name":"巫术","iconPath":"/lol-game-data/assets/v1/perk-images/styles/sorcery/sorcery.png","slots":[{"perks":[{"id":8214,"name":"艾黎","iconPath":"/lol-game-data/assets/v1/perks/8214.png"}]}]}]`,
	}
	for name, fixture := range fixtures {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/lol-game-data/assets/v1/perkstyles.json":
					_, _ = w.Write([]byte(fixture))
				case "/lol-game-data/assets/v1/perks.json":
					_, _ = w.Write([]byte(`[{"id":8005,"name":"强攻","iconPath":"/lol-game-data/assets/v1/perks/8005.png"},{"id":8006,"name":"凯旋","iconPath":"/lol-game-data/assets/v1/perks/8006.png"},{"id":8112,"name":"电刑","iconPath":"/lol-game-data/assets/v1/perks/8112.png"},{"id":8214,"name":"艾黎","iconPath":"/lol-game-data/assets/v1/perks/8214.png"}]`))
				case "/lol-game-data/assets/v1/cherry-augments.json":
					_, _ = w.Write([]byte(`[]`))
				default:
					http.Error(w, "unexpected endpoint", http.StatusNotFound)
				}
			}))
			defer server.Close()
			client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
			a := &app{connected: true, lcu: client}
			recorder := httptest.NewRecorder()
			a.handleGameplayPerks(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/perks", nil))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
			}
			var payload struct {
				Styles       []gameplayPerkStyle `json:"styles"`
				StatModSlots []gameplayPerkSlot  `json:"statModSlots"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if len(payload.Styles) != 1 || len(payload.Styles[0].Slots) != 1 || len(payload.Styles[0].Slots[0].Perks) == 0 {
				t.Fatalf("styles = %#v", payload.Styles)
			}
			if payload.Styles[0].Slots[0].Perks[0].ID <= 0 || payload.Styles[0].Slots[0].Perks[0].Name == "" || payload.Styles[0].Slots[0].Perks[0].IconPath == "" {
				t.Fatalf("slot perks were not enriched: %#v", payload.Styles[0].Slots[0].Perks)
			}
			if len(payload.StatModSlots) != 3 {
				t.Fatalf("stat mod slots = %#v", payload.StatModSlots)
			}
			for _, slot := range payload.Styles[0].Slots {
				if strings.EqualFold(slot.Type, "kStatMod") {
					t.Fatalf("stat mod slot leaked into style: %#v", payload.Styles[0].Slots)
				}
			}
		})
	}
}

func TestGameplayPerkCatalogCachesLCUFiles(t *testing.T) {
	requests := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests[r.URL.Path]++
		switch r.URL.Path {
		case "/lol-game-data/assets/v1/perkstyles.json":
			_, _ = w.Write([]byte(`[{"id":8000,"name":"精密","iconPath":"/lol-game-data/assets/v1/style.png","slots":[{"perks":[8005]}]}]`))
		case "/lol-game-data/assets/v1/perks.json":
			_, _ = w.Write([]byte(`[{"id":8005,"name":"强攻","iconPath":"/lol-game-data/assets/v1/perk.png"}]`))
		case "/lol-game-data/assets/v1/cherry-augments.json":
			_, _ = w.Write([]byte(`[]`))
		default:
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{connected: true, lcu: client, perkCatalog: make(map[string]gameplayPerkCatalogCacheEntry)}
	for index := 0; index < 2; index++ {
		recorder := httptest.NewRecorder()
		a.handleGameplayPerks(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/perks", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("request %d status = %d: %s", index+1, recorder.Code, recorder.Body.String())
		}
	}
	for _, path := range []string{"/lol-game-data/assets/v1/perkstyles.json", "/lol-game-data/assets/v1/perks.json", "/lol-game-data/assets/v1/cherry-augments.json"} {
		if requests[path] != 1 {
			t.Fatalf("%s requests = %d, want 1", path, requests[path])
		}
	}
}

func TestSanitizeAssetPathKeepsDataDragonFallback(t *testing.T) {
	const path = "/cdn/img/perk-images/StatMods/StatModsHealthPlusIcon.png"
	if got := sanitizeAssetPath("ddragon:" + path); got != "ddragon:"+path {
		t.Fatalf("sanitizeAssetPath(ddragon) = %q", got)
	}
}

func TestNormalizeGameplayPerkCatalogReportsEnrichmentAndMissingIcons(t *testing.T) {
	styles := []gameplayPerkStyle{{
		ID: 8000, IconPath: "/lol-game-data/assets/v1/style.png",
		Slots: []gameplayPerkSlot{{Perks: []gameplayPerk{{ID: 8005}}}},
	}}
	perks := []gameplayPerk{{ID: 8005, Name: "强攻", IconPath: "unsafe://perk.png", StyleID: 8000}}
	styles, perks, enriched, missing := normalizeGameplayPerkCatalog(styles, perks)
	if enriched != 1 || missing != 2 {
		t.Fatalf("enriched=%d missing=%d styles=%#v perks=%#v", enriched, missing, styles, perks)
	}
	if styles[0].Slots[0].Perks[0].Name != "强攻" || styles[0].Slots[0].Perks[0].StyleID != 8000 {
		t.Fatalf("ID-only perk was not enriched: %#v", styles[0].Slots[0].Perks[0])
	}
}

func TestLCUFullHistoryPageKeepsPaginationOpen(t *testing.T) {
	const count = 5
	var history lcuMatchHistory
	history.Games.GameCount = count
	for index := 0; index < count; index++ {
		game := lcuGame{GameID: int64(index + 1), QueueID: 420, GameMode: "CLASSIC"}
		game.ParticipantIdentities = []lcuParticipantIdentity{{ParticipantID: 1}}
		game.Participants = []lcuParticipant{{ParticipantID: 1, TeamID: 100}}
		game.Teams = []lcuTeam{{TeamID: 100, Win: "Win"}}
		history.Games.Games = append(history.Games.Games, game)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/lol-match-history/v1/products/lol/current-summoner/matches" {
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(history)
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client(), platformProbe: true, region: "NA"}
	a := &app{sgp: newSGPProvider()}
	matches, capabilities, pagination := a.loadDetailedMatches(context.Background(), client, gameplayReference{}, strings.Repeat("p", 48), true, 0, count, "all", nil, nil)
	if len(matches) != count || !pagination.HasMore || pagination.Total != 0 {
		t.Fatalf("matches=%d pagination=%#v", len(matches), pagination)
	}
	if len(capabilities) == 0 || len(capabilities[0].Attempts) != 2 || capabilities[0].Attempts[0].Outcome != dataSourceDisabled || capabilities[0].Attempts[1].Source != dataSourceLCU || capabilities[0].Attempts[1].Outcome != dataSourceSuccess {
		t.Fatalf("LCU fallback attempts = %#v", capabilities)
	}
}

func TestLCUSingleParticipantSummaryFetchesFullGameDetail(t *testing.T) {
	playerRef := strings.Repeat("p", 48)
	makeGame := func(participants int) lcuGame {
		game := lcuGame{GameID: 91, QueueID: 420, GameMode: "CLASSIC", GameType: "MATCHED_GAME"}
		for index := 0; index < participants; index++ {
			identity := lcuParticipantIdentity{ParticipantID: int64(index + 1)}
			identity.Player.PUUID = strings.Repeat(string(rune('a'+index)), 48)
			game.ParticipantIdentities = append(game.ParticipantIdentities, identity)
			game.Participants = append(game.Participants, lcuParticipant{ParticipantID: int64(index + 1), TeamID: 100 + int64(index/5)*100})
		}
		game.Teams = []lcuTeam{{TeamID: 100, Win: "Win"}, {TeamID: 200, Win: "Fail"}}
		return game
	}
	requests := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests[r.URL.Path]++
		switch r.URL.Path {
		case "/lol-match-history/v1/products/lol/current-summoner/matches":
			var payload lcuMatchHistory
			payload.Games.Games = []lcuGame{makeGame(1)}
			_ = json.NewEncoder(w).Encode(payload)
		case "/lol-match-history/v1/games/91":
			_ = json.NewEncoder(w).Encode(makeGame(10))
		default:
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client(), platformProbe: true, region: "NA"}
	games, capabilities, _ := loadGameplayHistory(client, playerRef, true, 0, 5, true)
	if len(games) != 1 || len(games[0].Participants) != 10 || requests["/lol-match-history/v1/games/91"] != 1 {
		t.Fatalf("games=%#v requests=%#v", games, requests)
	}
	if len(capabilities) < 2 || capabilities[1].State != capabilityAvailable {
		t.Fatalf("capabilities=%#v", capabilities)
	}
}

func TestLCUSingleParticipantDetailRemainsExplicitlyIncomplete(t *testing.T) {
	game := lcuGame{GameID: 92, QueueID: 420, GameMode: "CLASSIC", GameType: "MATCHED_GAME"}
	game.ParticipantIdentities = []lcuParticipantIdentity{{ParticipantID: 1}}
	game.Participants = []lcuParticipant{{ParticipantID: 1, TeamID: 100}}
	game.Teams = []lcuTeam{{TeamID: 100, Win: "Win"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-match-history/v1/products/lol/current-summoner/matches":
			var payload lcuMatchHistory
			payload.Games.Games = []lcuGame{game}
			_ = json.NewEncoder(w).Encode(payload)
		case "/lol-match-history/v1/games/92":
			_ = json.NewEncoder(w).Encode(game)
		default:
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client(), platformProbe: true, region: "NA"}
	_, capabilities, _ := loadGameplayHistory(client, strings.Repeat("p", 48), true, 0, 5, true)
	if len(capabilities) < 2 || capabilities[1].State != capabilityFailed || !strings.Contains(capabilities[1].Detail, "参与者不完整") {
		t.Fatalf("capabilities=%#v", capabilities)
	}
}

func TestCustomGamesAreRecognizedAcrossModes(t *testing.T) {
	for _, match := range []gameplayMatch{
		{QueueID: 0, GameMode: "CLASSIC", GameType: "MATCHED_GAME"},
		{QueueID: 420, GameMode: "CLASSIC", GameType: "CUSTOM_GAME"},
		{QueueID: 450, GameMode: "CUSTOM", GameType: "MATCHED_GAME"},
		{QueueID: 420, GameMode: "CLASSIC", GameType: "MATCHED_GAME", ModeGroup: "custom"},
	} {
		if !isCustomGameplayMatch(match) {
			t.Fatalf("custom game was not recognized: %#v", match)
		}
	}
	if isCustomGameplayMatch(gameplayMatch{QueueID: 420, GameMode: "CLASSIC", GameType: "MATCHED_GAME"}) {
		t.Fatal("matched ranked game was classified as custom")
	}
}

func TestGameplayFilterDiagnosticsSplitEmptyAndCustomMatches(t *testing.T) {
	ranked := &riotMatchInfo{GameID: 1, QueueID: 420, Participants: []riotParticipant{{ParticipantID: 1}}}
	custom := &riotMatchInfo{GameID: 2, QueueID: 0, GameType: "CUSTOM_GAME", Participants: []riotParticipant{{ParticipantID: 1}}}
	empty := &riotMatchInfo{GameID: 3, QueueID: 420}
	rSummary := summarizeRiotGameplayFilters([]*riotMatchInfo{ranked, custom, empty, nil})
	if rSummary.Visible != 1 || rSummary.SkippedEmptyParticipants != 2 || rSummary.FilteredCustom != 1 || rSummary.CustomReasons["queue_id_zero"] != 1 || rSummary.CustomReasons["game_type"] != 1 {
		t.Fatalf("riot filter summary = %#v", rSummary)
	}
	if 4 != rSummary.Visible+rSummary.SkippedEmptyParticipants+rSummary.FilteredCustom {
		t.Fatalf("riot returned invariant failed: %#v", rSummary)
	}

	lSummary := summarizeLCUGameplayFilters([]lcuGame{{GameID: 1, QueueID: 420}, {GameID: 2, QueueID: 0, GameMode: "CUSTOM"}})
	if lSummary.Visible != 1 || lSummary.SkippedEmptyParticipants != 0 || lSummary.EmptyParticipantPayloads != 2 || lSummary.FilteredCustom != 1 {
		t.Fatalf("LCU filter summary = %#v", lSummary)
	}
	if 2 != lSummary.Visible+lSummary.SkippedEmptyParticipants+lSummary.FilteredCustom {
		t.Fatalf("LCU returned invariant failed: %#v", lSummary)
	}
}

func TestSummarizeRiotParticipantsDetectsShortNonArenaRosters(t *testing.T) {
	short := &riotMatchInfo{GameID: 1, QueueID: 420, GameMode: "CLASSIC", Participants: make([]riotParticipant, 9)}
	full := &riotMatchInfo{GameID: 2, QueueID: 420, GameMode: "CLASSIC", Participants: make([]riotParticipant, 10)}
	arena := &riotMatchInfo{GameID: 3, QueueID: 1700, GameMode: "CHERRY", Participants: make([]riotParticipant, 3)}
	summary := summarizeRiotParticipants([]*riotMatchInfo{short, full, arena})
	if summary.Incomplete != 1 || summary.SingleParticipant != 0 || summary.MinParticipants != 3 || summary.MaxParticipants != 10 {
		t.Fatalf("unexpected participant summary: %#v", summary)
	}
	if summary.Counts[9] != 1 || summary.Counts[10] != 1 || summary.Counts[3] != 1 {
		t.Fatalf("participant count histogram = %#v", summary.Counts)
	}
}

func TestSummarizeRiotParticipantsIgnoresCustomRosters(t *testing.T) {
	custom := &riotMatchInfo{GameID: 4, QueueID: 0, GameMode: "CLASSIC", GameType: "CUSTOM_GAME", Participants: make([]riotParticipant, 1)}
	if summary := summarizeRiotParticipants([]*riotMatchInfo{custom}); summary.Incomplete != 0 {
		t.Fatalf("custom roster was marked incomplete: %#v", summary)
	}
}

func TestVerifiedRankWinRateRejectsMissingLosses(t *testing.T) {
	if rate, complete := verifiedRankWinRate(107, 0); rate != -1 || complete {
		t.Fatalf("rate=%d complete=%v, want unavailable", rate, complete)
	}
	if rate, complete := verifiedRankWinRate(12, 8); rate != 60 || !complete {
		t.Fatalf("rate=%d complete=%v, want 60%%", rate, complete)
	}
}

func TestRiotParticipantAcceptsBothSummonerSpellFieldNames(t *testing.T) {
	for name, payload := range map[string]string{
		"match-v5":   `{"summoner1Id":4,"summoner2Id":12}`,
		"sgp-legacy": `{"spell1Id":4,"spell2Id":12}`,
	} {
		t.Run(name, func(t *testing.T) {
			var participant riotParticipant
			if err := json.Unmarshal([]byte(payload), &participant); err != nil {
				t.Fatal(err)
			}
			info := &riotMatchInfo{GameID: 1, QueueID: 420, GameDuration: 900, Participants: []riotParticipant{participant}}
			match := convertRiotMatchInfo(info, "", nil, nil, "", "HN1")
			if len(match.Participants) != 1 || match.Participants[0].Spell1ID != 4 || match.Participants[0].Spell2ID != 12 {
				t.Fatalf("participant loadout = %#v", match.Participants)
			}
		})
	}
}

func TestRemakeFallbackIsConservative(t *testing.T) {
	tests := []struct {
		name                  string
		explicit, surrendered bool
		duration, queueID     int64
		gameMode, gameType    string
		hasWinner, want       bool
	}{
		{name: "riot explicit signal wins", explicit: true, surrendered: true, duration: 180, queueID: 420, gameMode: "CLASSIC", gameType: "MATCHED_GAME", hasWinner: true, want: true},
		{name: "short match without winner", duration: 180, queueID: 420, gameMode: "CLASSIC", gameType: "MATCHED_GAME", want: true},
		{name: "short match with winner", duration: 180, queueID: 420, gameMode: "CLASSIC", gameType: "MATCHED_GAME", hasWinner: true},
		{name: "ordinary surrender", surrendered: true, duration: 180, queueID: 420, gameMode: "CLASSIC", gameType: "MATCHED_GAME"},
		{name: "long match", duration: 301, queueID: 420, gameMode: "CLASSIC", gameType: "MATCHED_GAME"},
		{name: "arena", duration: 180, queueID: 1700, gameMode: "CHERRY", gameType: "MATCHED_GAME"},
		{name: "bot game", duration: 180, queueID: 850, gameMode: "CLASSIC", gameType: "MATCHED_GAME"},
		{name: "custom game", duration: 180, queueID: 420, gameMode: "CLASSIC", gameType: "CUSTOM_GAME"},
		{name: "unknown queue", duration: 180, gameMode: "CLASSIC", gameType: "MATCHED_GAME"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := isRemakeGame(test.explicit, test.surrendered, test.duration, test.queueID, test.gameMode, test.gameType, test.hasWinner)
			if got != test.want {
				t.Fatalf("isRemakeGame() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestGameplayNormalizersDoNotMarkOrdinarySurrenderAsRemake(t *testing.T) {
	playerRef := strings.Repeat("s", 48)
	t.Run("lcu participant field", func(t *testing.T) {
		game := lcuGame{GameID: 3, GameDuration: 185, QueueID: 420, GameMode: "CLASSIC", GameType: "MATCHED_GAME"}
		identity := lcuParticipantIdentity{ParticipantID: 1}
		identity.Player.PUUID = playerRef
		participant := lcuParticipant{ParticipantID: 1, TeamID: 100, ChampionID: 64}
		participant.Stats.GameEndedInSurrender = true
		game.ParticipantIdentities = []lcuParticipantIdentity{identity}
		game.Participants = []lcuParticipant{participant}

		match := normalizeGameplayMatch(game, gameplayReference{PlayerRef: playerRef}, nil, nil)
		if match.Result != "loss" {
			t.Fatalf("LCU surrender was classified as %q", match.Result)
		}
	})
	t.Run("riot participant field", func(t *testing.T) {
		var info riotMatchInfo
		payload := `{"gameId":4,"gameDuration":185,"queueId":420,"gameMode":"CLASSIC","gameType":"MATCHED_GAME","participants":[{"participantId":1,"puuid":"` + playerRef + `","championId":64,"win":false,"gameEndedInSurrender":true}]}`
		if err := json.Unmarshal([]byte(payload), &info); err != nil {
			t.Fatal(err)
		}
		match := convertRiotMatchInfo(&info, playerRef, nil, nil, riotRegionKR, "")
		if !info.Participants[0].GameEndedInSurrender || match.Result != "loss" {
			t.Fatalf("Riot surrender = %#v", match)
		}
	})
}

func TestGameplayNormalizersMarkEarlySurrenderAsRemake(t *testing.T) {
	playerRef := strings.Repeat("r", 48)
	t.Run("lcu participant field", func(t *testing.T) {
		game := lcuGame{GameID: 1, GameDuration: 185, QueueID: 420, GameMode: "CLASSIC", GameType: "MATCHED_GAME"}
		identity := lcuParticipantIdentity{ParticipantID: 1}
		identity.Player.PUUID = playerRef
		participant := lcuParticipant{ParticipantID: 1, TeamID: 100, ChampionID: 64}
		participant.Stats.GameEndedInEarlySurrender = true
		participant.Stats.Win = true
		game.ParticipantIdentities = []lcuParticipantIdentity{identity}
		game.Participants = []lcuParticipant{participant}

		match := normalizeGameplayMatch(game, gameplayReference{PlayerRef: playerRef}, nil, nil)
		if match.Result != "remake" || match.SubjectParticipantID != 1 || len(match.Participants) != 1 || !match.Participants[0].Win {
			t.Fatalf("LCU remake = %#v", match)
		}
	})
	t.Run("riot participant field", func(t *testing.T) {
		var info riotMatchInfo
		payload := `{"gameId":2,"gameDuration":185,"queueId":420,"gameMode":"CLASSIC","gameType":"MATCHED_GAME","participants":[{"participantId":1,"puuid":"` + playerRef + `","championId":64,"win":true,"gameEndedInEarlySurrender":true}]}`
		if err := json.Unmarshal([]byte(payload), &info); err != nil {
			t.Fatal(err)
		}
		match := convertRiotMatchInfo(&info, playerRef, nil, nil, riotRegionKR, "")
		if !info.Participants[0].GameEndedInEarlySurrender || match.Result != "remake" || match.SubjectParticipantID != 1 || len(match.Participants) != 1 || !match.Participants[0].Win {
			t.Fatalf("Riot remake = %#v", match)
		}
	})
}

func TestRemakesAreExcludedFromWinRateAggregates(t *testing.T) {
	playerRef := strings.Repeat("p", 48)
	participant := func(win bool) gameplayParticipant {
		return gameplayParticipant{ParticipantID: 1, PlayerRef: playerRef, ChampionID: 64, Win: win, Kills: 8, Deaths: 2, Assists: 6, CS: 150}
	}
	matches := []gameplayMatch{
		{Result: "win", QueueID: 420, Duration: 1800, SubjectParticipantID: 1, Participants: []gameplayParticipant{participant(true)}},
		{Result: "remake", QueueID: 420, Duration: 185, SubjectParticipantID: 1, Participants: []gameplayParticipant{participant(false)}},
	}
	overall := aggregateMatches(matches, playerRef, nil)
	if overall.Games != 1 || overall.Wins != 1 || overall.Losses != 0 {
		t.Fatalf("aggregate includes remake: %#v", overall)
	}
	champions := championStats(matches, playerRef, map[int64]string{64: "李青"})
	if len(champions) != 1 || champions[0].Games != 1 || champions[0].Wins != 1 {
		t.Fatalf("champion stats include remake: %#v", champions)
	}
}

func TestRecentRankedSummaryUsesLatestTwentyRankedGames(t *testing.T) {
	playerRef := strings.Repeat("q", 48)
	makeMatch := func(index int, queueID int64, result, position string, kills, deaths, assists int) gameplayMatch {
		return gameplayMatch{
			CreatedAt: int64(index), QueueID: queueID, Result: result, SubjectParticipantID: 1,
			Participants: []gameplayParticipant{{
				ParticipantID: 1, PlayerRef: playerRef, TeamID: 100, Position: position,
				Kills: kills, Deaths: deaths, Assists: assists, Win: result == "win",
			}},
			Teams: []gameplayTeam{{TeamID: 100, Kills: 20}},
		}
	}
	matches := make([]gameplayMatch, 0, 26)
	for index := 1; index <= 24; index++ {
		position := "top"
		if index >= 13 {
			position = "middle"
		}
		kills, deaths, assists := 5, 2, 7
		if index <= 4 {
			kills, deaths, assists = 99, 1, 99
		}
		result := "loss"
		if index%2 == 0 {
			result = "win"
		}
		matches = append(matches, makeMatch(index, 420, result, position, kills, deaths, assists))
	}
	matches = append(matches,
		makeMatch(25, 430, "win", "jungle", 99, 1, 99),
		makeMatch(26, 440, "remake", "utility", 99, 1, 99),
	)

	got := recentRankedSummary(matches, playerRef, nil)
	if got.Games != 20 || got.Wins != 10 || got.Losses != 10 || got.WinRate != 50 {
		t.Fatalf("record = %#v", got)
	}
	if got.Kills != 5 || got.Deaths != 2 || got.Assists != 7 || got.KDA != 6 {
		t.Fatalf("averages = %#v", got)
	}
	if got.KillParticipation != 60 || got.KillParticipationGames != 20 {
		t.Fatalf("kill participation = %#v", got)
	}
	if len(got.Positions) != 2 || got.Positions[0].Position != "middle" || got.Positions[0].Games != 12 || got.Positions[1].Position != "top" || got.Positions[1].Games != 8 {
		t.Fatalf("positions = %#v", got.Positions)
	}
}

func TestRecentRankedSummaryDoesNotInventTeamKills(t *testing.T) {
	playerRef := strings.Repeat("i", 48)
	match := gameplayMatch{
		CreatedAt: 1, QueueID: 420, Result: "win", SubjectParticipantID: 1,
		Participants: []gameplayParticipant{{ParticipantID: 1, PlayerRef: playerRef, TeamID: 100, Position: "top", Kills: 5, Assists: 5, Win: true}},
	}
	got := recentRankedSummary([]gameplayMatch{match}, playerRef, nil)
	if got.Games != 1 || got.KillParticipationGames != 0 || got.KillParticipation != 0 {
		t.Fatalf("incomplete team data produced kill participation: %#v", got)
	}
}

func TestRecentRankedSummaryUsesSoloThenFallsBackToFlex(t *testing.T) {
	playerRef := strings.Repeat("q", 48)
	makeRanked := func(id, queueID int64, position string) gameplayMatch {
		return gameplayMatch{
			GameID: id, CreatedAt: id, QueueID: queueID, Result: "win", SubjectParticipantID: 1,
			Participants: []gameplayParticipant{{ParticipantID: 1, PlayerRef: playerRef, TeamID: 100, Position: position, Win: true}},
		}
	}
	mixed := []gameplayMatch{
		makeRanked(1, 420, "top"),
		makeRanked(2, 440, "middle"),
		makeRanked(3, 440, "middle"),
	}
	if got := recentRankedSummary(mixed, playerRef, nil); got.QueueID != 420 || got.QueueLabel != "单双排" || got.Games != 1 {
		t.Fatalf("solo preference = %#v", got)
	}
	flexOnly := recentRankedSummary(mixed[1:], playerRef, nil)
	if flexOnly.QueueID != 440 || flexOnly.QueueLabel != "灵活组排" || flexOnly.Games != 2 {
		t.Fatalf("flex fallback = %#v", flexOnly)
	}
}

func TestPositionStatsExcludeOtherAndNormalizeKnownPositions(t *testing.T) {
	playerRef := strings.Repeat("o", 48)
	matches := []gameplayMatch{
		{SubjectParticipantID: 1, Participants: []gameplayParticipant{{ParticipantID: 1, PlayerRef: playerRef, Position: "top"}}},
		{SubjectParticipantID: 1, Participants: []gameplayParticipant{{ParticipantID: 1, PlayerRef: playerRef, Position: "other"}}},
	}
	got := positionStats(matches, playerRef)
	if len(got) != 5 {
		t.Fatalf("position rows = %#v", got)
	}
	for _, item := range got {
		if item.Position == "other" || item.Label == "其他" {
			t.Fatalf("other position leaked into overview: %#v", got)
		}
	}
	if got[0].Position != "top" || got[0].Share != 100 {
		t.Fatalf("known position shares were not normalized: %#v", got)
	}
	wantPositions := []string{"top", "jungle", "middle", "bottom", "utility"}
	wantLabels := []string{"上单", "打野", "中单", "下路", "辅助"}
	for index := range wantPositions {
		if got[index].Position != wantPositions[index] || got[index].Label != wantLabels[index] {
			t.Fatalf("position order or label = %#v", got)
		}
		if index > 0 && (got[index].Games != 0 || got[index].Share != 0) {
			t.Fatalf("zero-sample position was not preserved: %#v", got)
		}
	}
}

func TestRankedQueueStatsKeepQueuesSeparateWhenSoloHasNoSample(t *testing.T) {
	playerRef := strings.Repeat("f", 48)
	matches := make([]gameplayMatch, 0, 3)
	for index := int64(1); index <= 3; index++ {
		matches = append(matches, gameplayMatch{
			GameID: index, CreatedAt: index, QueueID: 440, Result: "win", SubjectParticipantID: 1,
			Participants: []gameplayParticipant{{ParticipantID: 1, PlayerRef: playerRef, Position: "jungle", Win: true}},
		})
	}
	queues := buildGameplayRankedQueues(matches, matches, playerRef, nil, "")
	solo, ok := queues["420"]
	if !ok || solo.RecentRanked == nil || solo.RecentRanked.QueueID != 420 || solo.RecentRanked.Games != 0 {
		t.Fatalf("solo recent stats unexpectedly used flex data: %#v", solo)
	}
	if solo.PositionQueueID != 420 || solo.PositionQueueLabel != "单双排" || len(solo.Positions) != 5 || positionStatsGames(solo.Positions) != 0 {
		t.Fatalf("solo position stats unexpectedly used flex source: %#v", solo)
	}
	flex := queues["440"]
	if flex.RecentRanked == nil || flex.RecentRanked.QueueID != 440 || flex.PositionQueueID != 440 {
		t.Fatalf("flex stats changed unexpectedly: %#v", flex)
	}
}

func TestRankedQueueAbilityUsesTheSameRecentMatchesAsTheSummary(t *testing.T) {
	matches := []gameplayMatch{abilityTestMatch(1), abilityTestMatch(2)}
	for index := range matches {
		matches[index].QueueID = 440
	}
	queues := buildGameplayRankedQueues(matches, matches, "subject", nil, "")
	flex := queues["440"]
	if flex.RecentRanked == nil || flex.RecentRanked.Games != 2 {
		t.Fatalf("recent summary did not use the shared first-page sample: %#v", flex.RecentRanked)
	}
	if flex.Ability != nil || flex.AbilitySampleGames != 2 {
		t.Fatalf("ability sample count did not use all comparable matches: %#v", flex)
	}
}

func TestRecentRankedSamplesStayIndependentFromRightSideFilter(t *testing.T) {
	playerRef := strings.Repeat("f", 48)
	var mu sync.Mutex
	requests := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tag := r.URL.Query().Get("tag")
		mu.Lock()
		requests[tag]++
		mu.Unlock()
		if r.URL.Query().Get("startIndex") != "0" || r.URL.Query().Get("count") != "20" {
			t.Fatalf("sample query = %q", r.URL.RawQuery)
		}
		queueID := int64(420)
		if tag == "q_440" {
			queueID = 440
		}
		body := fmt.Sprintf(`{"games":[{"json":{"gameId":%d,"queueId":%d,"gameMode":"CLASSIC","mapId":11,"participants":[{"participantId":1,"teamId":100,"puuid":"%s","championId":5,"win":true}]}}]}`, queueID, queueID, playerRef)
		_, _ = io.WriteString(w, body)
	}))
	defer server.Close()
	client := &LCUClient{}
	provider := newSGPProvider()
	provider.http = server.Client()
	provider.serverBases["HN1"] = server.URL
	provider.token, provider.tokenAt, provider.tokenClient = "test-entitlements", time.Now(), client
	a := &app{sgp: provider}
	// 右侧当前筛选为 flex，左侧样本仍必须同时读取 solo 与 flex。
	got := a.loadRecentRankedSamples(
		context.Background(), client, gameplayReference{ServerID: "HN1"}, playerRef,
		"flex", gameplayPagination{ServerFiltered: true},
		[]gameplayMatch{{GameID: 77, QueueID: 440, Result: "win"}}, nil, nil,
	)
	if len(got.ByQueue[420]) != 1 || got.ByQueue[420][0].QueueID != 420 || len(got.ByQueue[440]) != 1 || got.ByQueue[440][0].QueueID != 440 {
		t.Fatalf("independent samples = %#v", got.ByQueue)
	}
	mu.Lock()
	defer mu.Unlock()
	if requests["q_420"] != 1 || requests["q_440"] != 1 || len(requests) != 2 {
		t.Fatalf("requests = %#v", requests)
	}
}

func TestLoadSGPMatchHistoryPageFlexUsesOneQueueTagRequest(t *testing.T) {
	requests := 0
	playerRef := strings.Repeat("q", 48)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		query := r.URL.Query()
		if strings.Join(query["tag"], ",") != "q_440" || query.Get("tagsQueryType") != "" || query.Get("startIndex") != "0" || query.Get("count") != "20" {
			t.Fatalf("flex query = %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"games":[{"json":{"gameId":87,"queueId":440,"gameMode":"CLASSIC","mapId":11,"participants":[]}}]}`))
	}))
	defer server.Close()
	client := &LCUClient{}
	provider := newSGPProvider()
	provider.http = server.Client()
	provider.serverBases["HN1"] = server.URL
	provider.token, provider.tokenAt, provider.tokenClient = "test-entitlements", time.Now(), client
	a := &app{sgp: provider}
	infos, _, _, resolution, err := a.loadSGPMatchHistoryPage(context.Background(), client, "HN1", playerRef, 0, 20, "flex")
	if err != nil || requests != 1 || len(infos) != 1 || !resolution.ServerFiltered || resolution.Fallback {
		t.Fatalf("requests=%d infos=%d resolution=%#v err=%v", requests, len(infos), resolution, err)
	}
}

func TestRecentRankedSamplesCacheEachQueueSeparately(t *testing.T) {
	requests := make(map[string]int)
	var mu sync.Mutex
	playerRef := strings.Repeat("a", 48)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		tag := query.Get("tag")
		if tag != "q_420" && tag != "q_440" || query.Get("tagsQueryType") != "" || query.Get("startIndex") != "0" || query.Get("count") != "20" {
			t.Fatalf("ranked sample query = %q", r.URL.RawQuery)
		}
		queueID := 420
		gameID := 88
		if tag == "q_440" {
			queueID = 440
			gameID = 89
		}
		mu.Lock()
		requests[tag]++
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"games": []map[string]any{{"json": map[string]any{
			"gameId": gameID, "queueId": queueID, "gameMode": "CLASSIC", "mapId": 11,
			"participants": []map[string]any{{"participantId": 1, "teamId": 100, "puuid": playerRef, "championId": 5, "win": true}},
		}}}})
	}))
	defer server.Close()
	client := &LCUClient{}
	provider := newSGPProvider()
	provider.http = server.Client()
	provider.serverBases["HN1"] = server.URL
	provider.token, provider.tokenAt, provider.tokenClient = "test-entitlements", time.Now(), client
	a := &app{sgp: provider}
	reference := gameplayReference{ServerID: "HN1"}
	got := a.loadRecentRankedSamples(context.Background(), client, reference, playerRef, "all", gameplayPagination{}, nil, nil, nil)
	if len(got.ByQueue[420]) != 1 || got.ByQueue[420][0].QueueID != 420 || len(got.ByQueue[440]) != 1 || got.ByQueue[440][0].QueueID != 440 {
		t.Fatalf("samples=%#v", got.ByQueue)
	}
	mu.Lock()
	firstRequests := requests["q_420"] + requests["q_440"]
	mu.Unlock()
	if firstRequests != 2 {
		t.Fatalf("first requests=%d counts=%#v", firstRequests, requests)
	}
	cached := a.loadRecentRankedSamples(context.Background(), client, reference, playerRef, "all", gameplayPagination{}, nil, nil, nil)
	if len(cached.ByQueue[420]) != 1 || cached.ByQueue[420][0].GameID != 88 || len(cached.ByQueue[440]) != 1 || cached.ByQueue[440][0].GameID != 89 {
		t.Fatalf("cached samples=%#v", cached.ByQueue)
	}
	mu.Lock()
	defer mu.Unlock()
	if requests["q_420"] != 1 || requests["q_440"] != 1 {
		t.Fatalf("cache miss requests=%#v", requests)
	}
}

func TestCurrentProfileBackgroundUsesClientProfile(t *testing.T) {
	a := &app{account: AccountData{Profile: SummonerProfile{BackgroundSkinID: 164001, BackgroundSkinName: "钢铁军团 卡蜜尔"}}}
	id, name, source, path := a.currentProfileBackground()
	if id != 164001 || name != "钢铁军团 卡蜜尔" || source != "gtimg" || path != "/images/lol/act/img/skin/big164001.jpg" {
		t.Fatalf("background = %d %q %q %q", id, name, source, path)
	}
}

// 韩服与“查看别人资料”都拿不到客户端个人主页背景，此时必须退回最高熟练度英雄
// 的默认皮肤原画；已经有客户端背景时不能被覆盖。
func TestApplyMasteryBackgroundFallback(t *testing.T) {
	masteries := []gameplayMastery{
		{ChampionID: 202, ChampionName: "烬", ChampionPoints: 120000},
		{ChampionID: 164, ChampionName: "卡蜜尔", ChampionPoints: 490000},
	}
	player := gameplayPlayer{}
	applyMasteryBackgroundFallback(&player, masteries)
	if player.BackgroundSource != "gtimg" || player.BackgroundPath != "/images/lol/act/img/skin/big164000.jpg" {
		t.Fatalf("fallback background = %q %q", player.BackgroundSource, player.BackgroundPath)
	}
	if player.BackgroundSkinID != 164000 || player.BackgroundSkinName != "卡蜜尔" {
		t.Fatalf("fallback skin = %d %q", player.BackgroundSkinID, player.BackgroundSkinName)
	}

	existing := gameplayPlayer{BackgroundSkinID: 164001, BackgroundSkinName: "钢铁军团 卡蜜尔", BackgroundSource: "gtimg", BackgroundPath: "/images/lol/act/img/skin/big164001.jpg"}
	applyMasteryBackgroundFallback(&existing, masteries)
	if existing.BackgroundPath != "/images/lol/act/img/skin/big164001.jpg" {
		t.Fatalf("client background was overwritten: %q", existing.BackgroundPath)
	}

	empty := gameplayPlayer{}
	applyMasteryBackgroundFallback(&empty, nil)
	if empty.BackgroundSource != "" || empty.BackgroundPath != "" {
		t.Fatalf("no mastery should leave background empty: %q %q", empty.BackgroundSource, empty.BackgroundPath)
	}

	// 兜底路径必须能通过 /api/champion-asset 的白名单，否则前端只会拿到 400。
	if _, ok := validateChampionAssetPath(player.BackgroundSource, player.BackgroundPath); !ok {
		t.Fatalf("fallback background rejected by asset whitelist: %q %q", player.BackgroundSource, player.BackgroundPath)
	}
}

func TestDiagnosticRankedQueueSamplesCompareQueuesAndQueueMap(t *testing.T) {
	samples := diagnosticRankedQueueSamples(json.RawMessage(`{
		"queues":[{"queueType":"RANKED_SOLO_5x5","wins":107,"losses":0,"tier":"GOLD","division":"II","puuid":"private"}],
		"queueMap":{"RANKED_SOLO_5x5":{"wins":107,"losses":93,"tier":"GOLD","division":"II","opaque":"private"}}
	}`))
	queues := samples["queues"].(map[string]any)
	queueMap := samples["queueMap"].(map[string]any)
	if queues["losses"] != float64(0) || queueMap["losses"] != float64(93) || len(queues) != 4 || len(queueMap) != 4 {
		t.Fatalf("ranked queue samples = %#v", samples)
	}
	encoded, _ := json.Marshal(samples)
	if strings.Contains(string(encoded), "puuid") || strings.Contains(string(encoded), "opaque") || strings.Contains(string(encoded), "private") {
		t.Fatalf("ranked queue samples leaked unreviewed fields: %s", encoded)
	}
}

func TestGameplayRankMilestonesNormalizeAndSuppressMissingValues(t *testing.T) {
	stats := sgpRankedStats{
		HighestPreviousSeasonEndTier:      "PLATINUM",
		HighestPreviousSeasonEndRank:      "I",
		HighestPreviousSeasonAchievedTier: "DIAMOND",
		HighestPreviousSeasonAchievedRank: "IV",
		Queues: []sgpRankedQueue{
			{QueueType: "RANKED_FLEX_SR", PreviousSeasonEndTier: "GOLD", PreviousSeasonEndRank: "II"},
			{
				QueueType: "RANKED_SOLO_5x5", PreviousSeasonEndTier: "EMERALD", PreviousSeasonEndRank: "I",
				PreviousSeasonHighestTier: "DIAMOND", PreviousSeasonHighestRank: "IV",
			},
		},
	}
	milestones := gameplayRankMilestonesFromSGP(stats)
	if milestones == nil || milestones.PeakTier != "diamond" || milestones.PeakDivision != "IV" {
		t.Fatalf("peak milestone = %#v", milestones)
	}
	if len(milestones.PreviousSeason) != 1 {
		t.Fatalf("previous season milestones = %#v", milestones.PreviousSeason)
	}
	previous := milestones.PreviousSeason[0]
	if previous.QueueType != "RANKED_SOLO_5x5" || previous.Tier != "emerald" || previous.Division != "I" || previous.HighestTier != "diamond" || previous.HighestDiv != "IV" {
		t.Fatalf("previous season milestone = %#v", previous)
	}

	fallback := gameplayRankMilestonesFromSGP(sgpRankedStats{HighestPreviousSeasonEndTier: "MASTER", HighestPreviousSeasonEndRank: "I"})
	if fallback == nil || fallback.PeakTier != "master" || fallback.PeakDivision != "I" {
		t.Fatalf("end-rank peak fallback = %#v", fallback)
	}

	missing := gameplayRankMilestonesFromSGP(sgpRankedStats{Queues: []sgpRankedQueue{{
		QueueType: "RANKED_SOLO_5x5", PreviousSeasonHighestTier: "GOLD", PreviousSeasonHighestRank: "I",
	}}})
	if missing != nil {
		t.Fatalf("missing end rank must not produce milestones: %#v", missing)
	}
}

func TestRankMilestonesOnlySerializeForTencentResponses(t *testing.T) {
	milestones := &gameplayRankMilestones{PeakTier: "diamond", PeakDivision: "IV"}
	if rankMilestonesForRegion("", milestones) != milestones {
		t.Fatal("Tencent response must retain rank milestones")
	}
	if got := rankMilestonesForRegion(riotRegionKR, milestones); got != nil {
		t.Fatalf("KR response exposed Tencent milestones: %#v", got)
	}

	encoded, err := json.Marshal(gameplayOverview{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"rankMilestones"`) {
		t.Fatalf("nil rank milestones must be omitted: %s", encoded)
	}
}

// queues 数组与 queueMap 对象两种形状都要能被探测到，否则国服客户端换一种形状
// 日志里就会一片空白，看起来像「没有历史赛段字段」。
func TestRankedQueueEntriesCoversBothPayloadShapes(t *testing.T) {
	both := rankedQueueEntries(json.RawMessage(`{"queues":[{"a":1}],"queueMap":{"RANKED_SOLO_5x5":{"b":2}}}`))
	if len(both) != 2 {
		t.Fatalf("entries = %#v", both)
	}
	if len(rankedQueueEntries(json.RawMessage(`{"queueMap":{"RANKED_FLEX_SR":{"c":3}}}`))) != 1 {
		t.Fatal("queueMap-only payload must still yield entries")
	}
	if rankedQueueEntries(json.RawMessage(`[]`)) != nil {
		t.Fatal("non-object payload must yield no entries")
	}
}

func TestLCURankedEntryParsesAllHistoricalFields(t *testing.T) {
	var payload lcuRankedStats
	err := json.Unmarshal([]byte(`{"queues":[{
		"queueType":"RANKED_SOLO_5x5",
		"previousSeasonEndTier":"EMERALD","previousSeasonEndDivision":"II",
		"previousSeasonHighestTier":"DIAMOND","previousSeasonHighestDivision":"IV",
		"highestTier":"MASTER","highestDivision":"I"
	}]}`), &payload)
	if err != nil || len(payload.Queues) != 1 {
		t.Fatalf("LCU ranked fixture: payload=%#v err=%v", payload, err)
	}
	entry := payload.Queues[0]
	if entry.PreviousSeasonEndTier != "EMERALD" || entry.PreviousSeasonEndDivision != "II" ||
		entry.PreviousSeasonHighestTier != "DIAMOND" || entry.PreviousSeasonHighestDivision != "IV" ||
		entry.HighestTier != "MASTER" || entry.HighestDivision != "I" {
		t.Fatalf("historical fields were dropped: %#v", entry)
	}
	milestones := gameplayRankMilestonesFromLCU(payload.Queues)
	if milestones == nil || milestones.PeakTier != "master" || milestones.PeakDivision != "I" ||
		len(milestones.PreviousSeason) != 1 || milestones.PreviousSeason[0].Tier != "emerald" ||
		milestones.PreviousSeason[0].HighestTier != "diamond" {
		t.Fatalf("LCU milestones = %#v", milestones)
	}
}

func TestLCURankedShapeDiagnosticIsProcessScoped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"queues":[{"queueType":"RANKED_SOLO_5x5","tier":"GOLD","division":"II","wins":107,"losses":0}],"queueMap":{"RANKED_SOLO_5x5":{"tier":"GOLD","division":"II","wins":107,"losses":93}}}`))
	}))
	defer server.Close()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	a := &app{storage: trackTestStore(t, &localStore{root: root})}
	client := &LCUClient{baseURL: server.URL, token: "test", http: server.Client()}
	for index := 0; index < 2; index++ {
		if _, _, _, err := a.loadGameplayRanksContext(context.Background(), client, strings.Repeat("p", 48), false); err != nil {
			t.Fatal(err)
		}
	}
	data, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), `"event":"lcu_ranked_stats_shape"`) != 1 || !strings.Contains(string(data), `"queueMap":{"division":"II","losses":93,"tier":"GOLD","wins":107}`) {
		t.Fatalf("LCU ranked shape diagnostic = %s", data)
	}
}

func TestRankedStatsPreferCompleteLCUWithoutCallingSGP(t *testing.T) {
	playerRef := strings.Repeat("p", 48)
	lcuCalls, sgpCalls := 0, 0
	lcuServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		lcuCalls++
		_, _ = io.WriteString(w, `{"queues":[{"queueType":"RANKED_SOLO_5x5","tier":"EMERALD","division":"III","leaguePoints":68,"wins":225,"losses":203,"highestTier":"DIAMOND","highestDivision":"IV"}]}`)
	}))
	defer lcuServer.Close()
	sgpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sgpCalls++
		_, _ = io.WriteString(w, `{"queues":[{"queueType":"RANKED_SOLO_5x5","tier":"GOLD","rank":"I","leaguePoints":1,"wins":1,"losses":1}]}`)
	}))
	defer sgpServer.Close()
	client := &LCUClient{baseURL: lcuServer.URL, token: "test", http: lcuServer.Client(), region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	provider := newSGPProvider()
	provider.http = sgpServer.Client()
	provider.serverBases["HN1"] = sgpServer.URL
	provider.sessionToken, provider.sessionAt, provider.sessionOwner = "league-session", time.Now(), client
	a := &app{sgp: provider}
	ranks, milestones, capability := a.loadRanksWithFallback(context.Background(), client, playerRef, false, "HN1", "PUBLIC")
	if lcuCalls != 1 || sgpCalls != 0 || len(ranks) != 1 || ranks[0].Tier != "EMERALD" || ranks[0].Losses != 203 {
		t.Fatalf("LCU-first result: calls=%d/%d ranks=%#v", lcuCalls, sgpCalls, ranks)
	}
	if capabilitySource(capability) != dataSourceLCU || len(capability.Attempts) != 1 || capability.Attempts[0].Source != dataSourceLCU {
		t.Fatalf("LCU-first capability = %#v", capability)
	}
	if milestones == nil || milestones.PeakTier != "diamond" {
		t.Fatalf("LCU milestones = %#v", milestones)
	}
}

func TestRankedStatsFallBackToSGPWhenLCUFails(t *testing.T) {
	playerRef := strings.Repeat("f", 48)
	lcuCalls, sgpCalls := 0, 0
	lcuServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		lcuCalls++
		http.Error(w, "ranked unavailable", http.StatusServiceUnavailable)
	}))
	defer lcuServer.Close()
	sgpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sgpCalls++
		_, _ = io.WriteString(w, `{"queues":[{"queueType":"RANKED_FLEX_SR","tier":"PLATINUM","rank":"II","leaguePoints":14,"wins":12,"losses":8}]}`)
	}))
	defer sgpServer.Close()
	client := &LCUClient{baseURL: lcuServer.URL, token: "test", http: lcuServer.Client(), region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	provider := newSGPProvider()
	provider.http = sgpServer.Client()
	provider.serverBases["HN1"] = sgpServer.URL
	provider.sessionToken, provider.sessionAt, provider.sessionOwner = "league-session", time.Now(), client
	a := &app{sgp: provider}
	ranks, _, capability := a.loadRanksWithFallback(context.Background(), client, playerRef, false, "HN1", "PUBLIC")
	if lcuCalls != 1 || sgpCalls != 1 || len(ranks) != 1 || ranks[0].Tier != "PLATINUM" {
		t.Fatalf("fallback result: calls=%d/%d ranks=%#v", lcuCalls, sgpCalls, ranks)
	}
	if capabilitySource(capability) != dataSourceSGP || capability.FallbackReason != "lcu-failed" || len(capability.Attempts) != 2 {
		t.Fatalf("fallback capability = %#v", capability)
	}
}

func TestRankedStatsCompletionGateStopsOnlyUnproductiveLCUFallbacks(t *testing.T) {
	playerRef := strings.Repeat("g", 48)
	lcuCalls, sgpCalls := 0, 0
	lcuFails := false
	lcuServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		lcuCalls++
		if lcuFails {
			http.Error(w, "ranked unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, `{"queues":[{"queueType":"RANKED_SOLO_5x5","tier":"GOLD","division":"II","wins":107,"losses":0}]}`)
	}))
	defer lcuServer.Close()
	sgpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sgpCalls++
		_, _ = io.WriteString(w, `{"queues":[{"queueType":"RANKED_SOLO_5x5","tier":"GOLD","rank":"II","wins":107,"losses":0}]}`)
	}))
	defer sgpServer.Close()
	client := &LCUClient{baseURL: lcuServer.URL, token: "test", http: lcuServer.Client(), region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	provider := newSGPProvider()
	provider.http = sgpServer.Client()
	provider.serverBases["HN1"] = sgpServer.URL
	provider.sessionToken, provider.sessionAt, provider.sessionOwner = "league-session", time.Now(), client
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	a := &app{sgp: provider, storage: trackTestStore(t, &localStore{root: root})}
	provider.observe = a.recordDiagnostic
	for index := 0; index < sgpCompletionFailureLimit+1; index++ {
		ranks, _, _ := a.loadRanksWithFallback(context.Background(), client, playerRef, false, "HN1", "PUBLIC")
		if len(ranks) != 1 || ranks[0].WinRate >= 0 {
			t.Fatalf("attempt %d ranks = %#v", index+1, ranks)
		}
	}
	if lcuCalls != sgpCompletionFailureLimit+1 || sgpCalls != sgpCompletionFailureLimit {
		t.Fatalf("completion gate calls = lcu:%d sgp:%d", lcuCalls, sgpCalls)
	}
	data, err := a.storage.readDiagnosticLog()
	if err != nil || !strings.Contains(string(data), `"event":"sgp_completion_gate"`) || !strings.Contains(string(data), `"recent_attempts":10`) || !strings.Contains(string(data), `"recent_success":0`) || !strings.Contains(string(data), `"state":"closed"`) {
		t.Fatalf("completion gate diagnostic = %s err=%v", data, err)
	}

	// Closing the completion-only gate must not suppress SGP after a real LCU failure.
	lcuFails = true
	ranks, _, capability := a.loadRanksWithFallback(context.Background(), client, playerRef, false, "HN1", "PUBLIC")
	if sgpCalls != sgpCompletionFailureLimit+1 || len(ranks) != 1 || capabilitySource(capability) != dataSourceSGP || capability.FallbackReason != "lcu-failed" {
		t.Fatalf("normal SGP fallback was gated: calls=%d ranks=%#v capability=%#v", sgpCalls, ranks, capability)
	}
}

func TestPlayerRankScoreCachesSGPFallbackUnderPreferredLCUKey(t *testing.T) {
	playerRef := strings.Repeat("f", 48)
	lcuCalls, sgpCalls := 0, 0
	lcuServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		lcuCalls++
		http.Error(w, "ranked unavailable", http.StatusServiceUnavailable)
	}))
	defer lcuServer.Close()
	sgpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sgpCalls++
		_, _ = io.WriteString(w, `{"queues":[{"queueType":"RANKED_FLEX_SR","tier":"PLATINUM","rank":"II","leaguePoints":14,"wins":12,"losses":8}]}`)
	}))
	defer sgpServer.Close()
	client := &LCUClient{baseURL: lcuServer.URL, token: "test", http: lcuServer.Client(), region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	provider := newSGPProvider()
	provider.http = sgpServer.Client()
	provider.serverBases["HN1"] = sgpServer.URL
	provider.sessionToken, provider.sessionAt, provider.sessionOwner = "league-session", time.Now(), client
	a := &app{sgp: provider, rankScores: newRankScoreCache()}

	first := a.playerRankScore(context.Background(), client, playerRef, false, "HN1", "PUBLIC")
	second := a.playerRankScore(context.Background(), client, playerRef, false, "HN1", "PUBLIC")
	if !first.known || !second.known || first.score != second.score {
		t.Fatalf("cached fallback scores = first:%#v second:%#v", first, second)
	}
	if first.tier != "PLATINUM" || first.division != "II" || !first.winRateKnown || first.winRate != 60 || second.tier != first.tier || second.division != first.division || second.winRate != first.winRate {
		t.Fatalf("cached fallback rank metadata = first:%#v second:%#v", first, second)
	}
	if len(first.ranks) != 1 || len(second.ranks) != 1 || first.capability.State != capabilityAvailable || second.capability.State != capabilityAvailable {
		t.Fatalf("cached fallback lost the full rank payload: first:%#v second:%#v", first, second)
	}
	if lcuCalls != 1 || sgpCalls != 1 {
		t.Fatalf("cached fallback repeated upstream requests: lcu=%d sgp=%d", lcuCalls, sgpCalls)
	}
	preferred, preferredOK := a.rankScores.get(rankScoreCacheKey(dataSourceLCU, "HN1", playerRef))
	actual, actualOK := a.rankScores.get(rankScoreCacheKey(dataSourceSGP, "HN1", playerRef))
	if !preferredOK || !actualOK || preferred.source != dataSourceSGP || actual.source != dataSourceSGP {
		t.Fatalf("fallback cache provenance = preferred:%#v/%v actual:%#v/%v", preferred, preferredOK, actual, actualOK)
	}
}

func TestLoadQueueLabelsCachesPerLCUClientInstance(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/lol-game-queues/v1/queues" {
			http.NotFound(w, r)
			return
		}
		calls.Add(1)
		_, _ = io.WriteString(w, `[{"id":420,"shortName":"单双排"}]`)
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test", http: server.Client()}
	first := loadQueueLabels(client)
	first[420] = "mutated"
	second := loadQueueLabels(client)
	if calls.Load() != 1 || second[420] != "单双排" {
		t.Fatalf("queue label session cache = calls:%d first:%#v second:%#v", calls.Load(), first, second)
	}
	other := &LCUClient{baseURL: server.URL, token: "test", http: server.Client()}
	_ = loadQueueLabels(other)
	if calls.Load() != 2 {
		t.Fatalf("a new LCU client did not get a fresh queue catalog: %d", calls.Load())
	}
}

func TestRecentRankedSampleCacheEvictsOldestLargeEntries(t *testing.T) {
	a := &app{}
	now := time.Now()
	for index := 0; index < recentRankedSampleCacheMax+1; index++ {
		a.cacheRecentRankedSample(fmt.Sprintf("player-%03d", index), recentRankedSampleCacheEntry{
			at: now, matches: []gameplayMatch{{GameID: int64(index + 1)}},
		})
	}
	if len(a.recentRankedSamples) != recentRankedSampleCacheMax {
		t.Fatalf("recent ranked sample cache size = %d, want %d", len(a.recentRankedSamples), recentRankedSampleCacheMax)
	}
	if _, exists := a.recentRankedSamples["player-000"]; exists {
		t.Fatal("oldest recent ranked sample was not evicted")
	}
	if matches, ok := a.recentRankedSample(fmt.Sprintf("player-%03d", recentRankedSampleCacheMax), now); !ok || len(matches) != 1 {
		t.Fatalf("newest recent ranked sample missing: %#v %v", matches, ok)
	}
}

func TestPlayerRankScoreUsesRiotForKROpponentAndCachesMetadata(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	puuid := strings.Repeat("r", 48)
	var requests atomic.Int64
	champions := newChampionProvider()
	champions.clientMu.Lock()
	champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		if request.URL.Host != riotPlatformHost || request.URL.Path != "/lol/league/v4/entries/by-puuid/"+puuid {
			t.Fatalf("riot rank request = %s", request.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`[{"queueType":"RANKED_SOLO_5x5","tier":"DIAMOND","rank":"I","leaguePoints":88,"wins":19,"losses":11}]`,
			)),
			Request: request,
		}, nil
	})}
	champions.clientMu.Unlock()
	a := &app{rankScores: newRankScoreCache()}
	a.riot = newRiotProvider(champions)

	first := a.playerRankScore(context.Background(), nil, puuid, false, "kr", "")
	second := a.playerRankScore(context.Background(), nil, puuid, false, "KR", "")
	if !first.known || first.source != dataSourceRiot || first.tier != "diamond" || first.division != "I" || !first.winRateKnown || first.winRate != 63 {
		t.Fatalf("riot rank metadata = %#v", first)
	}
	if !reflect.DeepEqual(second, first) || requests.Load() != 1 {
		t.Fatalf("riot rank cache = first:%#v second:%#v requests:%d", first, second, requests.Load())
	}
	if cached, ok := a.rankScores.get(rankScoreCacheKey(dataSourceRiot, "KR", puuid)); !ok || !reflect.DeepEqual(cached, first) {
		t.Fatalf("riot rank cache entry = %#v ok=%v", cached, ok)
	}
}

func TestCanceledRankedRequestIsSilent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	a := &app{storage: trackTestStore(t, &localStore{root: root})}
	client := &LCUClient{baseURL: "http://127.0.0.1:1", token: "test", http: http.DefaultClient}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, capability := a.loadRanksWithFallback(ctx, client, strings.Repeat("c", 48), false, "", "")
	if capability.State == capabilityFailed || capability.State != capabilityCanceled {
		t.Fatalf("canceled capability = %#v", capability)
	}
	data, _ := a.storage.readDiagnosticLog()
	if strings.Contains(string(data), "failed") || strings.Contains(string(data), context.Canceled.Error()) {
		t.Fatalf("cancellation polluted diagnostics: %s", data)
	}
	_, historyCapabilities, _ := a.loadDetailedMatches(ctx, client, gameplayReference{}, strings.Repeat("c", 48), false, 0, 20, "all", nil, nil)
	if len(historyCapabilities) == 0 || historyCapabilities[0].State != capabilityCanceled {
		t.Fatalf("canceled history capability = %#v", historyCapabilities)
	}
	data, _ = a.storage.readDiagnosticLog()
	if strings.Contains(string(data), "match_history_failed") || strings.Contains(string(data), context.Canceled.Error()) {
		t.Fatalf("history cancellation polluted diagnostics: %s", data)
	}
	if timeoutCapability := gameplayCapabilityError("ranked-stats", "/ranked", context.DeadlineExceeded); timeoutCapability.State != capabilityFailed {
		t.Fatalf("deadline must remain a real failure: %#v", timeoutCapability)
	}
}

func TestCanceledPartialSGPHistoryIsSilent(t *testing.T) {
	playerRef := strings.Repeat("h", 48)
	secondPageStarted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("startIndex") != "0" {
			close(secondPageStarted)
			<-r.Context().Done()
			return
		}
		games := make([]map[string]any, 0, sgpPageSize)
		for index := 0; index < sgpPageSize; index++ {
			games = append(games, map[string]any{"json": map[string]any{
				"gameId": index + 1, "queueId": 420, "gameMode": "CLASSIC",
				"participants": []map[string]any{{"participantId": 1, "puuid": playerRef}},
			}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"games": games})
	}))
	defer server.Close()

	client := &LCUClient{region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	provider := newSGPProvider()
	provider.http = server.Client()
	provider.serverBases["HN1"] = server.URL
	provider.token, provider.tokenAt, provider.tokenClient = "test-entitlements", time.Now(), client
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	a := &app{sgp: provider, storage: trackTestStore(t, &localStore{root: root})}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-secondPageStarted
		cancel()
	}()
	_, capabilities, _ := a.loadDetailedMatches(ctx, client, gameplayReference{ServerID: "HN1"}, playerRef, false, 0, sgpPageSize+1, "all", nil, nil)
	if len(capabilities) == 0 || capabilities[0].State != capabilityCanceled {
		t.Fatalf("partial cancellation capability = %#v", capabilities)
	}
	data, _ := a.storage.readDiagnosticLog()
	if strings.Contains(string(data), "sgp_match_history_partial") || strings.Contains(string(data), "sgp_match_history_failed") {
		t.Fatalf("partial cancellation polluted diagnostics: %s", data)
	}
}

func TestRankedWinRateDiagnosticsRecordSGPAndLCUSources(t *testing.T) {
	newStore := func(t *testing.T) *localStore {
		t.Helper()
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
			t.Fatal(err)
		}
		return trackTestStore(t, &localStore{root: root})
	}
	rankedEvent := func(t *testing.T, data []byte) map[string]any {
		t.Helper()
		for _, line := range bytes.Split(data, []byte("\n")) {
			var event map[string]any
			if json.Unmarshal(line, &event) == nil && event["event"] == "ranked_winrate_resolved" {
				return event
			}
		}
		t.Fatalf("ranked aggregate missing from diagnostics: %s", data)
		return nil
	}
	t.Run("lcu", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"queues":[{"queueType":"RANKED_SOLO_5x5","wins":107,"losses":0}]}`))
		}))
		defer server.Close()
		store := newStore(t)
		a := &app{storage: store}
		client := &LCUClient{baseURL: server.URL, token: "test", http: server.Client()}
		playerRef := strings.Repeat("l", 48)
		_, _ = a.loadGameplayRanks(client, playerRef, false)
		a.flushRankedWinrateDiagnostics()
		data, err := store.readDiagnosticLog()
		if err != nil {
			t.Fatal(err)
		}
		event := rankedEvent(t, data)
		encoded, _ := json.Marshal(event)
		if event["source"] != "lcu" || event["suppressed_count"] != float64(1) || event["complete_count"] != float64(0) || strings.Contains(string(encoded), `"wins"`) || strings.Contains(string(encoded), `"losses"`) || strings.Contains(string(encoded), "player_ref_hash") || strings.Contains(string(encoded), playerRef) {
			t.Fatalf("LCU diagnostic=%s err=%v", data, err)
		}
	})
	t.Run("sgp", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"queues":[{"queueType":"RANKED_SOLO_5x5","wins":12,"losses":8}]}`))
		}))
		defer server.Close()
		store := newStore(t)
		client := &LCUClient{region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
		provider := newSGPProvider()
		provider.http = server.Client()
		provider.serverBases["HN1"] = server.URL
		provider.sessionToken = "session"
		provider.sessionAt = time.Now()
		provider.sessionOwner = client
		a := &app{storage: store, sgp: provider}
		playerRef := strings.Repeat("s", 48)
		_, _, _ = a.loadRanksWithFallback(context.Background(), client, playerRef, false, "HN1", "PUBLIC")
		a.flushRankedWinrateDiagnostics()
		data, err := store.readDiagnosticLog()
		if err != nil {
			t.Fatal(err)
		}
		event := rankedEvent(t, data)
		encoded, _ := json.Marshal(event)
		if event["source"] != "sgp" || event["suppressed_count"] != float64(0) || event["complete_count"] != float64(1) || strings.Contains(string(encoded), `"wins"`) || strings.Contains(string(encoded), `"losses"`) || strings.Contains(string(encoded), "player_ref_hash") || strings.Contains(string(encoded), playerRef) {
			t.Fatalf("SGP diagnostic=%s err=%v", data, err)
		}
	})
}

func TestRankedWinRateDiagnosticPlayerHashIsStableAndDistinct(t *testing.T) {
	store := trackTestStore(t, &localStore{salt: bytes.Repeat([]byte{0x5a}, 32)})
	a := &app{storage: store}
	firstRef := strings.Repeat("a", 48)
	secondRef := strings.Repeat("b", 48)
	first := a.rankedWinRateDiagnosticPlayerHash(firstRef)
	if first == "" || len(first) != 16 {
		t.Fatalf("first player hash = %q, want 16 hex characters", first)
	}
	if again := a.rankedWinRateDiagnosticPlayerHash(firstRef); again != first {
		t.Fatalf("same player hash was not stable: first=%q again=%q", first, again)
	}
	second := a.rankedWinRateDiagnosticPlayerHash(secondRef)
	if second == "" || second == first {
		t.Fatalf("different player hashes were not distinct: first=%q second=%q", first, second)
	}
	if strings.Contains(first, firstRef) || strings.Contains(second, secondRef) {
		t.Fatalf("player hash leaked its input: first=%q second=%q", first, second)
	}
	otherInstall := (&app{storage: trackTestStore(t, &localStore{salt: bytes.Repeat([]byte{0x6b}, 32)})}).rankedWinRateDiagnosticPlayerHash(firstRef)
	if otherInstall == first {
		t.Fatalf("different install salts produced the same player hash: %q", first)
	}
	if fallback := (&app{}).rankedWinRateDiagnosticPlayerHash(firstRef); fallback == "" || fallback != (&app{}).rankedWinRateDiagnosticPlayerHash(firstRef) {
		t.Fatalf("deterministic no-store fallback hash = %q", fallback)
	}
}

func TestSeasonRankWinRateFallbackFillsOnlyIncompleteSGPRanks(t *testing.T) {
	a := &app{}
	ranks, capability := a.applySeasonRankWinRateFallback(
		[]gameplayRank{
			{QueueType: "RANKED_SOLO_5x5", Wins: 107, Losses: 0, WinRate: -1},
			{QueueType: "RANKED_FLEX_SR", Wins: 12, Losses: 0, WinRate: -1},
		},
		EndpointCapability{Path: "sgp: /leagues-ledge/v2/rankedStats", Detail: "SGP 未返回排位负场，胜率暂不展示"},
		map[int64]gameplayAggregate{
			420: {QueueID: 420, Games: 50, Wins: 30, Losses: 20, WinRate: 60},
			440: {QueueID: 440, Games: 20, Wins: 5, Losses: 15, WinRate: 25},
		},
	)
	if ranks[0].Wins != 30 || ranks[0].Losses != 20 || ranks[0].WinRate != 60 || ranks[1].Wins != 5 || ranks[1].Losses != 15 || ranks[1].WinRate != 25 {
		t.Fatalf("season fallback ranks = %#v", ranks)
	}
	if ranks[0].Wins == 35 || ranks[1].Wins == 35 {
		t.Fatalf("queue ranks were filled from a merged season total: %#v", ranks)
	}
	if capability.Detail != "上游未返回排位负场，已按赛季战绩聚合补全胜率" {
		t.Fatalf("season fallback capability = %#v", capability)
	}

	lcuFilled, lcuCapability := a.applySeasonRankWinRateFallback(
		[]gameplayRank{{QueueType: "RANKED_SOLO_5x5", Wins: 107, Losses: 0, WinRate: -1}},
		EndpointCapability{Path: "/lol-ranked/v1/ranked-stats/{player}"},
		map[int64]gameplayAggregate{420: {Games: 100, Wins: 57, Losses: 43, WinRate: 57}},
	)
	if lcuFilled[0].Wins != 57 || lcuFilled[0].Losses != 43 || lcuFilled[0].WinRate != 57 || lcuCapability.Detail != "上游未返回排位负场，已按赛季战绩聚合补全胜率" {
		t.Fatalf("LCU season fallback = ranks:%#v capability=%#v", lcuFilled, lcuCapability)
	}
}

func TestSeasonRankFallbackAppearsAfterRefreshSnapshotCompletes(t *testing.T) {
	playerRef := strings.Repeat("r", 48)
	player := Summoner{PUUID: playerRef}
	reference := gameplayReference{PlayerRef: playerRef, ServerID: "HN1"}
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	a := &app{storage: store, sgp: newSGPProvider()}
	incomplete := []gameplayRank{{QueueType: "RANKED_SOLO_5x5", Wins: 107, Losses: 0, WinRate: -1}}
	capability := EndpointCapability{Path: "/lol-ranked/v1/ranked-stats/{player}", Detail: "客户端未返回排位负场，胜率暂不展示"}

	_, firstProgress, _, firstQueues := a.loadSeasonChampionStatsSnapshot(reference, player, playerRef)
	firstRanks, _ := a.applySeasonRankWinRateFallback(append([]gameplayRank(nil), incomplete...), capability, firstQueues)
	if firstRanks[0].WinRate >= 0 || !firstProgress.Collecting || firstProgress.Complete {
		t.Fatalf("initial fallback state = ranks:%#v progress:%#v", firstRanks, firstProgress)
	}

	season, _ := currentRankedSeason(time.Now())
	cache := seasonStatsCache{
		SchemaVersion: seasonStatsCacheSchemaVersion, Source: seasonStatsSource, Season: season,
		AccountHash: store.accountHash(player), GameIDs: []int64{1, 2}, Complete: true, UpdatedAt: time.Now(),
		QueueStats: map[int64]gameplayAggregate{420: {QueueID: 420, Games: 2, Wins: 1, Losses: 1, WinRate: 50}},
	}
	if err := store.saveSeasonStats(cache); err != nil {
		t.Fatal(err)
	}
	_, refreshedProgress, _, refreshedQueues := a.loadSeasonChampionStatsSnapshot(reference, player, playerRef)
	refreshedRanks, refreshedCapability := a.applySeasonRankWinRateFallback(append([]gameplayRank(nil), incomplete...), capability, refreshedQueues)
	if refreshedRanks[0].Wins != 1 || refreshedRanks[0].Losses != 1 || refreshedRanks[0].WinRate != 50 || !refreshedProgress.Complete || refreshedCapability.Detail != "上游未返回排位负场，已按赛季战绩聚合补全胜率" {
		t.Fatalf("refreshed fallback state = ranks:%#v progress:%#v capability:%#v", refreshedRanks, refreshedProgress, refreshedCapability)
	}
}

func TestGameplayOverviewAppliesSeasonFallbackToIncompleteSGPRanks(t *testing.T) {
	playerRef := strings.Repeat("s", 48)
	currentRef := strings.Repeat("c", 48)
	match := func(id int, win bool) string {
		return fmt.Sprintf(`{"gameId":%d,"queueId":420,"gameCreation":%d,"gameDuration":1800,"participants":[{"puuid":"%s","participantId":1,"teamId":100,"championId":103,"win":%t}]}`,
			id, time.Now().UnixMilli(), playerRef, win)
	}
	sgpHTTP := &http.Client{Transport: sgpRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		status, body := http.StatusOK, ""
		switch {
		case strings.Contains(request.URL.Path, "/match-history-query/") && strings.HasSuffix(request.URL.Path, "/SUMMARY"):
			body = `{"games":[{"json":` + match(1, true) + `},{"json":` + match(2, false) + `}]}`
		case strings.Contains(request.URL.Path, "/summoner-ledge/"):
			body = `[{"puuid":"` + playerRef + `","name":"测试玩家","level":30}]`
		case strings.Contains(request.URL.Path, "/leagues-ledge/"):
			// The missing losses field must reach the SGP incomplete-rank branch.
			body = `{"queues":[{"queueType":"RANKED_SOLO_5x5","tier":"GOLD","rank":"II","leaguePoints":44,"wins":107}]}`
		default:
			status, body = http.StatusNotFound, "unexpected SGP endpoint"
		}
		return &http.Response{
			StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)),
			ContentLength: int64(len(body)), Request: request,
		}, nil
	})}
	lcuHTTP := &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := "[]"
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})}
	client := &LCUClient{baseURL: "https://lcu.invalid", token: "lcu-token", http: lcuHTTP, region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	provider := newSGPProvider()
	provider.http = sgpHTTP
	provider.serverBases["HN1"] = "https://sgp.invalid"
	provider.token, provider.tokenAt, provider.tokenClient = "entitlements", time.Now(), client
	provider.sessionToken, provider.sessionAt, provider.sessionOwner = "league-session", time.Now(), client
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	a := &app{
		token: "session-secret", connected: true, lcu: client, sgp: provider, storage: store,
		summoner:     Summoner{PUUID: currentRef, GameName: "当前玩家"},
		gameplayRefs: make(map[string]string), gameplayRefDetails: make(map[string]gameplayReference),
	}
	season, _ := currentRankedSeason(time.Now())
	seasonCache := seasonStatsCache{
		SchemaVersion: seasonStatsCacheSchemaVersion, Source: seasonStatsSource, Season: season,
		AccountHash: store.accountHash(Summoner{PUUID: playerRef}), GameIDs: []int64{1, 2}, Complete: true, UpdatedAt: time.Now(),
		Stats:      []gameplaySeasonChampionStat{{ChampionID: 103, Games: 2, Wins: 1, WinRate: 50}},
		QueueStats: map[int64]gameplayAggregate{420: {QueueID: 420, Games: 2, Wins: 1, Losses: 1, WinRate: 50}},
	}
	if err := store.saveSeasonStats(seasonCache); err != nil {
		t.Fatal(err)
	}
	publicRef := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: playerRef, ServerID: "HN1", GameName: "测试玩家"})
	recorder := httptest.NewRecorder()
	a.handleGameplayOverview(recorder, httptest.NewRequest(http.MethodPost, "/api/gameplay/overview", strings.NewReader(`{"playerRef":"`+publicRef+`","count":20}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("overview status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var overview gameplayOverview
	if err := json.Unmarshal(recorder.Body.Bytes(), &overview); err != nil {
		t.Fatal(err)
	}
	if overview.SeasonOverall.Games != 2 || overview.SeasonOverall.Wins != 1 || overview.SeasonOverall.Losses != 1 || overview.SeasonOverall.WinRate != 50 {
		t.Fatalf("season aggregate = %#v", overview.SeasonOverall)
	}
	if len(overview.Ranks) != 1 {
		t.Fatalf("ranks = %#v", overview.Ranks)
	}
	rank := overview.Ranks[0]
	if rank.Wins != 1 || rank.Losses != 1 || rank.WinRate != 50 {
		t.Fatalf("rank was not filled from season aggregate: %#v", rank)
	}
	var capability EndpointCapability
	for _, item := range overview.Capabilities {
		if item.Name == "ranked-stats" {
			capability = item
			break
		}
	}
	if capability.Detail != "上游未返回排位负场，已按赛季战绩聚合补全胜率" {
		t.Fatalf("ranked capability = %#v", capability)
	}
}

func TestGameplayOverviewReturnsCoreBeforeSlowSeasonScan(t *testing.T) {
	playerRef := strings.Repeat("z", 48)
	seasonPageStarted := make(chan struct{}, 1)
	releaseSeasonPage := make(chan struct{})
	var releaseOnce sync.Once
	releaseSeason := func() { releaseOnce.Do(func() { close(releaseSeasonPage) }) }
	games := make([]string, 0, sgpPageSize)
	for index := 0; index < sgpPageSize; index++ {
		games = append(games, fmt.Sprintf(`{"json":{"gameId":%d,"queueId":420,"gameCreation":%d,"gameDuration":1800,"participants":[{"puuid":"%s","participantId":1,"teamId":100,"championId":103,"win":true}]}}`, 1000+index, time.Now().UnixMilli()-int64(index)*60000, playerRef))
	}
	sgpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/match-history-query/") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("startIndex") == "50" {
			select {
			case seasonPageStarted <- struct{}{}:
			default:
			}
			<-releaseSeasonPage
			_, _ = io.WriteString(w, `{"games":[]}`)
			return
		}
		_, _ = io.WriteString(w, `{"games":[`+strings.Join(games, ",")+`]}`)
	}))
	defer sgpServer.Close()
	lcuServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/lol-ranked/v1/current-ranked-stats":
			_, _ = io.WriteString(w, `{"queues":[{"queueType":"RANKED_SOLO_5x5","tier":"EMERALD","division":"II","leaguePoints":40,"wins":30,"losses":20}]}`)
		case strings.Contains(r.URL.Path, "/champion-mastery"):
			_, _ = io.WriteString(w, `{}`)
		default:
			_, _ = io.WriteString(w, `[]`)
		}
	}))
	defer lcuServer.Close()
	defer releaseSeason()
	client := &LCUClient{baseURL: lcuServer.URL, token: "test", http: lcuServer.Client(), region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	provider := newSGPProvider()
	provider.http = sgpServer.Client()
	provider.serverBases["HN1"] = sgpServer.URL
	provider.token, provider.tokenAt, provider.tokenClient = "entitlements", time.Now(), client
	provider.sessionToken, provider.sessionAt, provider.sessionOwner = "league-session", time.Now(), client
	a := &app{
		sgp: provider, lcu: client, connected: true, storage: trackTestStore(t, &localStore{root: t.TempDir()}),
		summoner: Summoner{PUUID: playerRef, GameName: "当前玩家"}, lpTracker: newLPTracker(nil),
		gameplayRefs: make(map[string]string), gameplayRefDetails: make(map[string]gameplayReference),
	}
	result := make(chan gameplayOverview, 1)
	go func() {
		result <- a.loadGameplayOverview(context.Background(), client, a.summoner, gameplayReference{PlayerRef: playerRef, ServerID: "HN1"}, 0, 20, "all", false)
	}()
	var overview gameplayOverview
	select {
	case overview = <-result:
	case <-time.After(750 * time.Millisecond):
		t.Fatal("overview waited for the slow season scan")
	}
	if len(overview.Matches) == 0 || len(overview.Ranks) == 0 {
		t.Fatalf("core overview is incomplete: matches=%d ranks=%#v", len(overview.Matches), overview.Ranks)
	}
	if len(overview.SeasonChampionStats) != 0 || overview.SeasonOverall.Games != 0 || !overview.SeasonStatsProgress.Collecting || overview.SeasonStatsProgress.Complete {
		t.Fatalf("initial season slice = stats=%#v overall=%#v progress=%#v", overview.SeasonChampionStats, overview.SeasonOverall, overview.SeasonStatsProgress)
	}
	select {
	case <-seasonPageStarted:
	case <-time.After(750 * time.Millisecond):
		t.Fatal("season scan was not started incrementally")
	}
	releaseSeason()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		a.seasonBackfillMu.Lock()
		running := len(a.seasonBackfills)
		a.seasonBackfillMu.Unlock()
		if running == 0 {
			backgroundKey := sgpHistoryPageCacheKey("HN1", playerRef, 50, sgpPageSize, nil)
			provider.mu.Lock()
			_, cached := provider.historyCache[backgroundKey]
			provider.mu.Unlock()
			if !cached {
				t.Fatal("season background scan did not retain its physical SGP history page")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("season background task did not finish after release")
}

func TestNormalizeGameplayMatchIncludesStatPerks(t *testing.T) {
	game := lcuGame{GameID: 88, QueueID: 420, GameMode: "CLASSIC"}
	identity := lcuParticipantIdentity{ParticipantID: 1}
	identity.Player.PUUID = strings.Repeat("r", 48)
	participant := lcuParticipant{ParticipantID: 1, TeamID: 100, ChampionID: 64}
	participant.Stats.Perk0 = 8010
	participant.Stats.Perk1 = 9111
	participant.Stats.Perk2 = 9104
	participant.Stats.Perk3 = 8014
	participant.Stats.Perk4 = 8304
	participant.Stats.Perk5 = 8347
	participant.Stats.StatPerk0 = 5005
	participant.Stats.StatPerk1 = 5008
	participant.Stats.StatPerk2 = 5011
	participant.Stats.PerkPrimaryStyle = 8000
	participant.Stats.PerkSubStyle = 8300
	controlWardsBought := 0
	participant.Stats.VisionWardsBoughtInGame = &controlWardsBought
	game.ParticipantIdentities = []lcuParticipantIdentity{identity}
	game.Participants = []lcuParticipant{participant}

	match := normalizeGameplayMatch(game, gameplayReference{PlayerRef: identity.Player.PUUID}, nil, nil)
	if len(match.Participants) != 1 || len(match.Participants[0].PerkIDs) != 9 {
		t.Fatalf("normalized perks = %#v", match.Participants)
	}
	got := match.Participants[0]
	if got.PerkIDs[6] != 5005 || got.PerkIDs[7] != 5008 || got.PerkIDs[8] != 5011 || got.PrimaryStyleID != 8000 || got.SubStyleID != 8300 {
		t.Fatalf("stat perks/styles were lost: %#v", got)
	}
	if got.ControlWardsBought == nil || *got.ControlWardsBought != 0 {
		t.Fatalf("control ward zero was lost: %#v", got.ControlWardsBought)
	}
}

func TestRecentPlayersAggregatesNamedParticipantsWithoutPUUID(t *testing.T) {
	matches := []gameplayMatch{
		{SubjectParticipantID: 1, Participants: []gameplayParticipant{{ParticipantID: 1, PlayerRef: strings.Repeat("s", 48)}, {ParticipantID: 2, GameName: "同行者", TagLine: "HN1", DisplayName: "同行者"}}},
		{SubjectParticipantID: 3, Participants: []gameplayParticipant{{ParticipantID: 3, PlayerRef: strings.Repeat("s", 48)}, {ParticipantID: 4, GameName: "同行者", TagLine: "HN1", DisplayName: "同行者"}}},
	}
	players := recentPlayers(matches, strings.Repeat("s", 48), 0)
	if len(players) != 1 || players[0].Games != 2 || players[0].PlayerRef != "" || players[0].DisplayName != "同行者" {
		t.Fatalf("recent players = %#v", players)
	}
}

func TestHiddenPlayerUsesTheSameRankAndMatchHistoryLoaders(t *testing.T) {
	hiddenPUUID := strings.Repeat("h", 48)
	currentPUUID := strings.Repeat("c", 48)

	var game lcuGame
	game.GameID = 701
	game.GameCreation = 1_786_300_000_000
	game.GameDuration = 1_800
	game.QueueID = 420
	game.GameMode = "CLASSIC"
	identity := lcuParticipantIdentity{ParticipantID: 1}
	identity.Player.PUUID = hiddenPUUID
	participant := lcuParticipant{ParticipantID: 1, TeamID: 100, ChampionID: 64}
	participant.Stats.Kills = 8
	participant.Stats.Deaths = 2
	participant.Stats.Assists = 6
	participant.Stats.TotalMinionsKilled = 180
	participant.Stats.Win = true
	game.ParticipantIdentities = []lcuParticipantIdentity{identity}
	game.Participants = []lcuParticipant{participant}
	game.Teams = []lcuTeam{{TeamID: 100, Win: "Win"}}
	var history lcuMatchHistory
	history.Games.GameCount = 1
	history.Games.Games = []lcuGame{game}

	requested := make(map[string]int)
	var requestedMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedMu.Lock()
		requested[r.URL.Path]++
		requestedMu.Unlock()
		switch {
		case r.URL.Path == "/lol-summoner/v2/summoners/puuid/"+hiddenPUUID:
			http.Error(w, "profile hidden", http.StatusNotFound)
		case r.URL.Path == "/lol-game-queues/v1/queues":
			_, _ = w.Write([]byte(`[{"id":420,"shortName":"单排/双排"}]`))
		case r.URL.Path == "/lol-ranked/v1/ranked-stats/"+hiddenPUUID:
			_, _ = w.Write([]byte(`{"queues":[{"queueType":"RANKED_SOLO_5x5","tier":"GOLD","division":"I","leaguePoints":55,"wins":12,"losses":8}]}`))
		case r.URL.Path == "/lol-match-history/v1/products/lol/"+hiddenPUUID+"/matches":
			_ = json.NewEncoder(w).Encode(history)
		case r.URL.Path == "/lol-champion-mastery/v1/"+hiddenPUUID+"/champion-mastery":
			_, _ = w.Write([]byte(`[]`))
		default:
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{token: "session-secret", connected: true, lcu: client, summoner: Summoner{PUUID: currentPUUID}, gameplayRefs: make(map[string]string), gameplayRefDetails: make(map[string]gameplayReference)}
	publicRef := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: hiddenPUUID, DisplayName: "隐藏玩家"})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/gameplay/overview", strings.NewReader(`{"playerRef":"`+publicRef+`","count":20}`))
	a.handleGameplayOverview(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	responseBody := recorder.Body.String()
	if strings.Contains(responseBody, hiddenPUUID) {
		t.Fatal("renderer response leaked the hidden player's stable PUUID")
	}
	var response gameplayOverview
	if err := json.Unmarshal([]byte(responseBody), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Player.Hidden || response.Player.DisplayName != "隐藏玩家" {
		t.Fatalf("hidden profile = %#v", response.Player)
	}
	if response.Overall.Games != 1 || response.Overall.Wins != 1 || len(response.Matches) != 1 {
		t.Fatalf("hidden player match data was suppressed: overall=%#v matches=%d", response.Overall, len(response.Matches))
	}
	if len(response.Ranks) != 1 || response.Ranks[0].Wins != 12 || response.Ranks[0].Losses != 8 {
		t.Fatalf("hidden player rank data was suppressed: %#v", response.Ranks)
	}
	if len(response.Matches[0].Participants) != 1 || !response.Matches[0].Participants[0].Hidden || response.Matches[0].Participants[0].PlayerRef == "" {
		t.Fatalf("hidden participant was not kept queryable: %#v", response.Matches[0].Participants)
	}
	requestedMu.Lock()
	defer requestedMu.Unlock()
	for _, path := range []string{
		"/lol-ranked/v1/ranked-stats/" + hiddenPUUID,
		"/lol-match-history/v1/products/lol/" + hiddenPUUID + "/matches",
		"/lol-champion-mastery/v1/" + hiddenPUUID + "/champion-mastery",
	} {
		if requested[path] == 0 {
			t.Fatalf("hidden player did not use normal loader %s", path)
		}
	}
}

func TestChampSelectKeepsObfuscatedIdentityQueryable(t *testing.T) {
	obfuscated := "00000000-0000-0000-0000-000000000000"
	deobfuscated := "817076a9-f451-509b-9598-6813ce9117e7"
	merged := mergeChampSelectPlayers(nil, lcuChampSelectSession{TheirTeam: []lcuChampSelectPlayer{{
		ChampionID: 64, ObfuscatedPUUID: obfuscated, ObfuscatedSummonerID: 88,
		NameVisibilityType: "HIDDEN", Spell1ID: 11, Spell2ID: 4,
	}}}, 0)
	if len(merged) != 1 {
		t.Fatalf("merged players = %d", len(merged))
	}
	player := merged[0].player
	if player.PUUID != deobfuscated || player.ObfuscatedPUUID != obfuscated || player.NameVisibilityType != "HIDDEN" {
		t.Fatalf("obfuscated identity was not recovered internally: %#v", player)
	}
	if player.Spell1ID != 11 || player.Spell2ID != 4 {
		t.Fatalf("champ-select summoner spells were not merged: %#v", player)
	}
	reference := normalizeGameplayReference(gameplayReference{PlayerRef: player.PUUID, AlternatePlayerRef: player.ObfuscatedPUUID, AlternateSummonerID: player.ObfuscatedSummonerID})
	if reference.PlayerRef != deobfuscated || reference.AlternatePlayerRef != obfuscated {
		t.Fatalf("obfuscated player cannot receive a session alias: %#v", reference)
	}
	if deobfuscateHiddenPlayerReference("550e8400-e29b-41d4-a716-446655440000") != "d47ef2a9-16ca-114f-328e-2c759bd517e7" {
		t.Fatal("hidden player UUID compatibility vector changed")
	}
	if deobfuscateHiddenPlayerReference("not-a-uuid") != "" {
		t.Fatal("invalid hidden player reference was accepted")
	}
}

func TestChampSelectMergesObfuscatedIdentityWithGameflowPUUID(t *testing.T) {
	obfuscated := "00000000-0000-0000-0000-000000000000"
	deobfuscated := "817076a9-f451-509b-9598-6813ce9117e7"
	existing := []struct {
		player lcuLivePlayer
		team   int64
	}{{player: lcuLivePlayer{PUUID: deobfuscated, ChampionID: 64}, team: 100}}
	merged := mergeChampSelectPlayers(existing, lcuChampSelectSession{MyTeam: []lcuChampSelectPlayer{{
		ObfuscatedPUUID: obfuscated, NameVisibilityType: "HIDDEN", ChampionID: 64, ChampionPickIntent: 64,
	}}}, len(existing))
	if len(merged) != 1 {
		t.Fatalf("merged players = %d, want one gameflow row", len(merged))
	}
	if merged[0].player.PUUID != deobfuscated || merged[0].player.ObfuscatedPUUID != obfuscated || merged[0].player.ChampionPickIntent != 64 {
		t.Fatalf("obfuscated champ-select identity was not merged: %#v", merged[0].player)
	}
}

func TestArenaChampSelectModeAndIdentityFilter(t *testing.T) {
	if !isArenaChampSelectMode("  cherry ") {
		t.Fatal("CHERRY mode was not recognized case-insensitively")
	}
	for _, mode := range []string{"", "CLASSIC", "ARAM", "KIWI"} {
		if isArenaChampSelectMode(mode) {
			t.Fatalf("non-Arena mode %q was treated as CHERRY", mode)
		}
	}

	players := []struct {
		player lcuLivePlayer
		team   int64
	}{
		{player: lcuLivePlayer{PUUID: "arena-player-1"}, team: 100},
		{player: lcuLivePlayer{PUUID: ""}, team: 100},
		{player: lcuLivePlayer{PUUID: emptyLCUPlayerPUUID}, team: 100},
		{player: lcuLivePlayer{PUUID: "  arena-player-2  "}, team: 200},
		{player: lcuLivePlayer{PUUID: "arena-player-3"}, team: 200},
	}
	visible, filtered := filterArenaChampSelectPlayers(players)
	if filtered != 2 || len(visible) != 3 {
		t.Fatalf("Arena champ-select filter = %d visible/%d filtered, want 3/2: %#v", len(visible), filtered, visible)
	}
	if visible[0].player.PUUID != "arena-player-1" || visible[1].player.PUUID != "  arena-player-2  " || visible[2].player.PUUID != "arena-player-3" {
		t.Fatalf("Arena champ-select filter changed order or identities: %#v", visible)
	}
}

func TestChampSelectCellIdentityMarksCurrentPlayer(t *testing.T) {
	localCellID := int64(0)
	otherCellID := int64(1)
	session := lcuChampSelectSession{
		LocalPlayerCellID: &localCellID,
		MyTeam: []lcuChampSelectPlayer{{
			CellID: &localCellID,
		}},
	}
	merged := mergeChampSelectPlayers(nil, session, 0)
	if len(merged) != 1 || merged[0].player.CellID == nil || *merged[0].player.CellID != localCellID {
		t.Fatalf("cell id was not copied into appended player: %#v", merged)
	}
	if !gameplayLivePlayerIsCurrent(gameplayReference{}, "", merged[0].player.CellID, session.LocalPlayerCellID) {
		t.Fatal("local cell was not recognized as the current player")
	}
	if gameplayLivePlayerIsCurrent(gameplayReference{}, "", &otherCellID, session.LocalPlayerCellID) {
		t.Fatal("different cell was recognized as the current player")
	}
	if gameplayLivePlayerIsCurrent(gameplayReference{}, "", nil, nil) {
		t.Fatal("missing cell ids were treated as a match")
	}

	existing := []struct {
		player lcuLivePlayer
		team   int64
	}{{player: lcuLivePlayer{PUUID: "existing-player", SummonerID: 77}, team: 100}}
	matched := mergeChampSelectPlayers(existing, lcuChampSelectSession{MyTeam: []lcuChampSelectPlayer{{
		PUUID: "existing-player", SummonerID: 77, CellID: &localCellID,
	}}}, len(existing))
	if matched[0].player.CellID == nil || *matched[0].player.CellID != localCellID {
		t.Fatalf("cell id was not copied into matched player: %#v", matched[0].player)
	}
}

func TestCurrentChampionFallbackDrivesRecommendationTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/lol-champ-select/v1/current-champion" {
			t.Fatalf("unexpected current champion request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`64`))
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	currentChampionID, err := loadCurrentChampionID(client)
	if err != nil || currentChampionID != 64 {
		t.Fatalf("current champion = %d, %v", currentChampionID, err)
	}

	championID, position := gameplayLiveRecommendationTarget([]gameplayLivePlayer{{
		gameplayPlayer: gameplayPlayer{IsCurrent: false}, ChampionID: 103, Position: "mid",
	}}, currentChampionID)
	if championID != 64 || position != "" {
		t.Fatalf("fallback target = %d/%q, want 64 with unknown position", championID, position)
	}

	championID, position = gameplayLiveRecommendationTarget([]gameplayLivePlayer{{
		gameplayPlayer: gameplayPlayer{IsCurrent: true}, ChampionID: 103, Position: "mid",
	}}, currentChampionID)
	if championID != 103 || position != "mid" {
		t.Fatalf("identified player did not take priority: %d/%q", championID, position)
	}
}

func TestChampSelectActionsAreLastRecommendationChampionFallback(t *testing.T) {
	localCellID := int64(7)
	baseSession := lcuChampSelectSession{
		LocalPlayerCellID: &localCellID,
		MyTeam:            []lcuChampSelectPlayer{{CellID: &localCellID}},
	}
	currentPlayer := gameplayLivePlayer{gameplayPlayer: gameplayPlayer{IsCurrent: true}, Position: "other"}

	fromActions := baseSession
	fromActions.Actions = [][]lcuChampSelectAction{{
		{ActorCellID: 3, ChampionID: 99, Type: "PICK"},
		{ActorCellID: localCellID, ChampionID: 25, Type: "pick"},
	}}
	championID, position := gameplayLiveRecommendationTargetWithChampSelect([]gameplayLivePlayer{currentPlayer}, 0, fromActions)
	if championID != 25 || position != "other" {
		t.Fatalf("action fallback = %d/%q, want 25/other", championID, position)
	}

	sentinel := baseSession
	sentinel.Actions = [][]lcuChampSelectAction{{{ActorCellID: localCellID, ChampionID: -3, Type: "PICK"}}}
	championID, _ = gameplayLiveRecommendationTargetWithChampSelect([]gameplayLivePlayer{currentPlayer}, 0, sentinel)
	if championID != 0 {
		t.Fatalf("sentinel action champion leaked into recommendations: %d", championID)
	}

	lockedPlayer := currentPlayer
	lockedPlayer.ChampionID = 100
	higherPriority := baseSession
	higherPriority.Actions = [][]lcuChampSelectAction{{{ActorCellID: localCellID, ChampionID: 200, Type: "PICK"}}}
	championID, _ = gameplayLiveRecommendationTargetWithChampSelect([]gameplayLivePlayer{lockedPlayer}, 0, higherPriority)
	if championID != 100 {
		t.Fatalf("action fallback overrode locked champion: %d", championID)
	}
}

func TestRecommendationPayloadSanitizesNegativePickIntent(t *testing.T) {
	localCellID := int64(7)
	merged := mergeChampSelectPlayers(nil, lcuChampSelectSession{
		LocalPlayerCellID: &localCellID,
		MyTeam:            []lcuChampSelectPlayer{{CellID: &localCellID, ChampionPickIntent: -3}},
	}, 0)
	if len(merged) != 1 {
		t.Fatalf("merged players = %d, want 1", len(merged))
	}
	if merged[0].player.ChampionPickIntent != 0 {
		t.Fatalf("negative pick intent leaked into payload source: %d", merged[0].player.ChampionPickIntent)
	}
	if !merged[0].player.ChampionPickPending {
		t.Fatal("negative pick intent did not retain pending presentation state")
	}
	if got := championName(nil, -3); got != "随机待定" {
		t.Fatalf("negative champion name = %q, want random-pending copy", got)
	}
	if got := championName(nil, 0); got != "未知英雄" {
		t.Fatalf("zero champion name = %q, want unknown copy", got)
	}
	if got := championName(nil, 25); got != "英雄 25" {
		t.Fatalf("positive champion name = %q, want fallback id copy", got)
	}
}

func TestGameplayLiveResolvesRandomChampSelectActionWithout400(t *testing.T) {
	for _, test := range []struct {
		name        string
		actionID    int64
		wantID      int64
		wantPending bool
	}{
		{name: "resolved pick", actionID: 25, wantID: 25},
		{name: "still pending", actionID: -3, wantPending: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/lol-gameflow/v1/gameflow-phase":
					_, _ = w.Write([]byte(`"ChampSelect"`))
				case "/lol-gameflow/v1/session":
					_, _ = w.Write([]byte(`{"gameData":{"gameId":42,"queue":{"id":1750,"mapId":30,"gameMode":"CHERRY"},"teamOne":[],"teamTwo":[]},"map":{"id":30,"gameMode":"CHERRY"}}`))
				case "/lol-champ-select/v1/session":
					fmt.Fprintf(w, `{"localPlayerCellId":7,"myTeam":[{"cellId":7,"puuid":"arena-player","championId":0,"championPickIntent":-3}],"theirTeam":[],"actions":[[{"actorCellId":7,"championId":%d,"type":"PICK"}]]}`, test.actionID)
				case "/lol-champ-select/v1/current-champion":
					_, _ = w.Write([]byte(`0`))
				default:
					// Recommendation and enrichment calls are optional for this shape.
					http.Error(w, "not available in fixture", http.StatusNotFound)
				}
			}))
			defer server.Close()
			client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
			a := &app{connected: true, lcu: client, summoner: Summoner{SummonerID: 123}}
			recorder := httptest.NewRecorder()
			a.handleGameplayLive(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/live", nil))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
			}
			var response gameplayLiveResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.ResolvedChampionID != test.wantID {
				t.Fatalf("resolved champion = %d, want %d; response=%s", response.ResolvedChampionID, test.wantID, recorder.Body.String())
			}
			if test.wantPending && response.ResolvedChampionID != 0 {
				t.Fatalf("negative action became a recommendation: %#v", response)
			}
		})
	}
}

func TestGameplayLiveSecondRefreshReusesRankedStatsCache(t *testing.T) {
	playerRef := strings.Repeat("r", 48)
	var rankedCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-gameflow/v1/gameflow-phase":
			_, _ = io.WriteString(w, `"ChampSelect"`)
		case "/lol-gameflow/v1/session":
			_, _ = io.WriteString(w, `{"gameData":{"gameId":42,"queue":{"id":420,"mapId":11,"gameMode":"CLASSIC"},"teamOne":[],"teamTwo":[]},"map":{"id":11,"gameMode":"CLASSIC"}}`)
		case "/lol-champ-select/v1/session":
			fmt.Fprintf(w, `{"localPlayerCellId":7,"myTeam":[{"cellId":7,"puuid":%q,"summonerId":123,"summonerName":"Player","championId":22}],"theirTeam":[],"actions":[]}`, playerRef)
		case "/lol-champ-select/v1/current-champion":
			_, _ = io.WriteString(w, `22`)
		case "/lol-ranked/v1/current-ranked-stats":
			rankedCalls.Add(1)
			_, _ = io.WriteString(w, `{"queues":[{"queueType":"RANKED_SOLO_5x5","tier":"GOLD","division":"II","leaguePoints":55,"wins":12,"losses":8}]}`)
		default:
			http.Error(w, "not available in fixture", http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client(), platformProbe: true}
	a := &app{connected: true, lcu: client, summoner: Summoner{SummonerID: 123, PUUID: playerRef}, rankScores: newRankScoreCache()}
	for range 2 {
		recorder := httptest.NewRecorder()
		a.handleGameplayLive(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/live", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
		}
	}
	if rankedCalls.Load() != 1 {
		t.Fatalf("two live refreshes made %d LCU ranked requests, want one cached round", rankedCalls.Load())
	}
}

func r62ArenaFixturePlayers(t *testing.T, count int) ([]map[string]any, []byte) {
	t.Helper()
	gameflowPlayers := make([]map[string]any, count)
	livePlayers := make([]map[string]any, 18)
	parties := []int{1, 1, 2, 2, 3, 4, 4, 5, 5, 5, 5, 5, 5, 6, 7, 8, 9, 10}
	for index := range livePlayers {
		name := fmt.Sprintf("R62Arena%02d", index+1)
		if index < count {
			gameflowPlayers[index] = map[string]any{"puuid": fmt.Sprintf("r90-player-identity-%02d", index+1), "summonerId": index + 1, "profileIconId": 1, "championId": index + 1, "selectedPosition": "Invalid", "selectedRole": "NONE", "teamParticipantId": parties[index], "summonerName": ""}
		}
		livePlayers[index] = map[string]any{"championName": "Hero", "isBot": false, "isDead": false, "items": []any{}, "level": 1, "position": "", "rawChampionName": "Hero", "rawSkinName": "Skin", "respawnTimer": 0, "riotId": name + "#CN1", "riotIdGameName": name, "riotIdTagLine": "CN1", "runes": map[string]any{}, "scores": map[string]any{}, "skinID": 0, "skinName": "Skin", "summonerName": name + "#CN1", "summonerSpells": map[string]any{}, "team": "ORDER"}
	}
	raw, err := json.Marshal(livePlayers)
	if err != nil {
		t.Fatal(err)
	}
	return gameflowPlayers, raw
}

func r62ArenaGameflowServer(t *testing.T, phase *string, gameID *int64, players []map[string]any) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-gameflow/v1/gameflow-phase":
			_ = json.NewEncoder(w).Encode(*phase)
		case "/lol-gameflow/v1/session":
			_ = json.NewEncoder(w).Encode(map[string]any{"gameData": map[string]any{
				"gameId": *gameID, "queue": map[string]any{"id": 1750, "mapId": 30, "gameMode": "CHERRY"},
				"teamOne": players, "teamTwo": []any{},
			}})
		default:
			if strings.HasPrefix(r.URL.Path, "/lol-game-data/assets/v1/champions/") {
				_, _ = io.WriteString(w, `{"spells":[{"name":"Q","description":"fixture"}]}`)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/lol-summoner/v2/summoners/puuid/r90-player-identity-") {
				id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/lol-summoner/v2/summoners/puuid/r90-player-identity-"))
				_ = json.NewEncoder(w).Encode(Summoner{PUUID: fmt.Sprintf("r90-player-identity-%02d", id), SummonerID: int64(id), GameName: fmt.Sprintf("R62Arena%02d", id), TagLine: "CN1"})
				return
			}
			http.Error(w, "not available in R62 fixture", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func r62GameplayLiveResponse(t *testing.T, a *app) gameplayLiveResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	a.handleGameplayLive(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/live", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response gameplayLiveResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func TestR58ChampSelectDropsStaleGameflowRoster(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	oldPlayers := make([]map[string]any, 18)
	for index := range oldPlayers {
		oldPlayers[index] = map[string]any{"summonerName": fmt.Sprintf("旧玩家%02d", index+1), "championId": index + 1}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-gameflow/v1/gameflow-phase":
			_, _ = w.Write([]byte(`"ChampSelect"`))
		case "/lol-gameflow/v1/session":
			_ = json.NewEncoder(w).Encode(map[string]any{"gameData": map[string]any{
				"gameId": int64(8962886592), "queue": map[string]any{"id": 3270, "mapId": 30, "gameMode": "CHERRY"},
				"teamOne": oldPlayers[:9], "teamTwo": oldPlayers[9:],
			}})
		case "/lol-champ-select/v1/session":
			_, _ = w.Write([]byte(`{"gameId":777,"localPlayerCellId":7,"myTeam":[{"cellId":7,"puuid":"new-player-identity-0001","summonerName":"新玩家","championId":64}],"theirTeam":[]}`))
		case "/lol-lobby/v2/lobby":
			_, _ = w.Write([]byte(`{"gameConfig":{"queueId":420,"mapId":11,"gameMode":"CLASSIC"}}`))
		case "/lol-champ-select/v1/current-champion":
			_, _ = w.Write([]byte(`64`))
		default:
			http.Error(w, "not available", http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{connected: true, lcu: client, storage: trackTestStore(t, &localStore{root: root}), summoner: Summoner{PUUID: "new-player-identity-0001"}}
	recorder := httptest.NewRecorder()
	a.handleGameplayLive(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/live", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response gameplayLiveResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Players) != 1 || response.GameID != 777 || response.QueueID != 420 || !response.DroppedStaleGameData {
		t.Fatalf("stale roster survived ChampSelect: %#v", response)
	}
	if response.Players[0].ChampionID != 64 || !response.Players[0].IsCurrent {
		t.Fatalf("ChampSelect player = %#v", response.Players[0])
	}
	diagnostics, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	text := string(diagnostics)
	for _, expected := range []string{`"event":"live_roster_shape"`, `"raw_count":0`, `"merge_appended":1`, `"dropped_stale_gamedata":true`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing %s in diagnostics: %s", expected, text)
		}
	}
	if strings.Contains(recorder.Body.String(), "旧玩家") {
		t.Fatalf("old player leaked into response: %s", recorder.Body.String())
	}
}

func TestR58InProgressKeepsGameflowRoster(t *testing.T) {
	players := make([]map[string]any, 18)
	for index := range players {
		players[index] = map[string]any{"summonerName": fmt.Sprintf("进行中玩家%02d", index+1), "championId": index + 1}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-gameflow/v1/gameflow-phase":
			_, _ = w.Write([]byte(`"InProgress"`))
		case "/lol-gameflow/v1/session":
			_ = json.NewEncoder(w).Encode(map[string]any{"gameData": map[string]any{
				"gameId": int64(8962886592), "queue": map[string]any{"id": 420, "mapId": 11, "gameMode": "CLASSIC"},
				"teamOne": players[:9], "teamTwo": players[9:],
			}})
		default:
			http.Error(w, "not available", http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{connected: true, lcu: client}
	recorder := httptest.NewRecorder()
	a.handleGameplayLive(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/live", nil))
	var response gameplayLiveResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Players) != 18 || response.GameID != 8962886592 || response.DroppedStaleGameData {
		t.Fatalf("InProgress roster was altered: %#v", response)
	}
}

func TestR58LiveClientPlayerListGroupsSixArenaTeamsAndRedactsDiagnostics(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	gameflowPlayers := make([]map[string]any, 18)
	livePlayers := make([]map[string]any, 18)
	for index := range gameflowPlayers {
		name := fmt.Sprintf("ArenaSecret%02d", index+1)
		gameflowPlayers[index] = map[string]any{"summonerName": name, "championId": index + 1}
		livePlayers[index] = map[string]any{"summonerName": name, "championName": fmt.Sprintf("Hero%d", index+1), "playerSubteamId": index/3 + 1, "subteamName": fmt.Sprintf("客户端小队%d", index/3+1), "team": "ORDER"}
	}
	liveRaw, err := json.Marshal(livePlayers)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-gameflow/v1/gameflow-phase":
			_, _ = w.Write([]byte(`"InProgress"`))
		case "/lol-gameflow/v1/session":
			_ = json.NewEncoder(w).Encode(map[string]any{"gameData": map[string]any{
				"gameId": int64(9001), "queue": map[string]any{"id": 1750, "mapId": 30, "gameMode": "CHERRY"},
				"teamOne": gameflowPlayers[:9], "teamTwo": gameflowPlayers[9:],
			}})
		default:
			http.Error(w, "not available", http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	var probes atomic.Int32
	a := &app{
		connected: true, lcu: client, storage: trackTestStore(t, &localStore{root: root}),
		liveClientPlayerList: func(context.Context) ([]byte, int, error) {
			probes.Add(1)
			return liveRaw, http.StatusOK, nil
		},
	}
	for attempt := 0; attempt < 2; attempt++ {
		recorder := httptest.NewRecorder()
		a.handleGameplayLive(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/live", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("attempt %d status = %d: %s", attempt, recorder.Code, recorder.Body.String())
		}
		var response gameplayLiveResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		counts := make(map[string]int)
		for _, player := range response.Players {
			counts[player.ArenaGroup]++
		}
		if !response.ArenaGrouped || response.ArenaGroupSource != "live-client" || response.ArenaGroupNames["1"] != "客户端小队1" || len(response.Players) != 18 || len(counts) != 6 {
			t.Fatalf("Arena grouping = grouped:%v players:%d counts:%#v", response.ArenaGrouped, len(response.Players), counts)
		}
		for group, count := range counts {
			if group == "" || count != 3 {
				t.Fatalf("invalid Arena group %q=%d", group, count)
			}
		}
	}
	if probes.Load() != 1 {
		t.Fatalf("successful playerlist probe count = %d, want cached result after first request", probes.Load())
	}
	diagnostics, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	text := string(diagnostics)
	if strings.Count(text, `"event":"live_client_playerlist_shape"`) != 1 || !strings.Contains(text, `"player_count":18`) || !strings.Contains(text, `"grouped":true`) || !strings.Contains(text, `"attempt":1`) || !strings.Contains(text, `"phase":"InProgress"`) || !strings.Contains(text, `"arena_group_source":"live-client"`) {
		t.Fatalf("playerlist diagnostics = %s", text)
	}
	if strings.Contains(text, "ArenaSecret") || strings.Contains(text, "客户端小队") {
		t.Fatalf("playerlist diagnostics leaked a player or team name: %s", text)
	}
}

func TestR68LiveClientPlayerListPositionsOverrideGameflowForAllTenPlayers(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	positions := []string{"TOP", "JUNGLE", "MIDDLE", "BOTTOM", "UTILITY", "TOP", "JUNGLE", "MIDDLE", "BOTTOM", "UTILITY"}
	gameflowPlayers := make([]map[string]any, len(positions))
	livePlayers := make([]map[string]any, len(positions))
	for index, position := range positions {
		gameName := fmt.Sprintf("PrivateLane%02d", index+1)
		gameflowPlayers[index] = map[string]any{
			"gameName": gameName, "tagLine": "CN1", "summonerName": fmt.Sprintf("Opaque%02d", index+1),
			"championId": index + 1, "selectedPosition": "TOP",
		}
		livePlayers[index] = map[string]any{
			"riotIdGameName": gameName, "riotIdTagLine": "CN1", "position": position, "team": []string{"ORDER", "CHAOS"}[index/5],
		}
	}
	liveRaw, err := json.Marshal(livePlayers)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-gameflow/v1/gameflow-phase":
			_, _ = w.Write([]byte(`"InProgress"`))
		case "/lol-gameflow/v1/session":
			_ = json.NewEncoder(w).Encode(map[string]any{"gameData": map[string]any{
				"gameId": int64(68001), "queue": map[string]any{"id": 420, "mapId": 11, "gameMode": "CLASSIC"},
				"teamOne": gameflowPlayers[:5], "teamTwo": gameflowPlayers[5:],
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	a := &app{
		connected: true, lcu: &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}, storage: trackTestStore(t, &localStore{root: root}),
		liveClientPlayerList: func(context.Context) ([]byte, int, error) { return liveRaw, http.StatusOK, nil },
	}
	recorder := httptest.NewRecorder()
	a.handleGameplayLive(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/live", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response gameplayLiveResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Players) != len(positions) {
		t.Fatalf("players = %d, want %d", len(response.Players), len(positions))
	}
	for index, player := range response.Players {
		want := strings.ToLower(positions[index])
		if player.Position != want {
			t.Fatalf("player %d position = %q, want Live Client %q (gameflow was TOP)", index, player.Position, want)
		}
	}
	diagnostics, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	text := string(diagnostics)
	for _, expected := range []string{`"event":"live_client_playerlist_shape"`, `"position_matched_count":10`, `"BOTTOM":2`, `"JUNGLE":2`, `"MIDDLE":2`, `"TOP":2`, `"UTILITY":2`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("diagnostic missing %s: %s", expected, text)
		}
	}
	for index := range positions {
		for _, secret := range []string{fmt.Sprintf("PrivateLane%02d", index+1), fmt.Sprintf("Opaque%02d", index+1)} {
			if strings.Contains(text, secret) {
				t.Fatalf("playerlist diagnostic leaked %q: %s", secret, text)
			}
		}
	}
}

func TestR68LiveClientPositionMatchedCountIsNotPlayerCount(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`[{"riotId":"Matched#CN1","position":"TOP"},{"riotId":"NotInRoster#CN1","position":"JUNGLE"}]`)
	a := &app{
		storage:              trackTestStore(t, &localStore{root: root}),
		liveClientPlayerList: func(context.Context) ([]byte, int, error) { return raw, http.StatusOK, nil },
	}
	snapshot, _ := a.liveClientSnapshotForGame(context.Background(), 68002, "InProgress", 0)
	position, source := liveClientPositionForIdentities(snapshot, nil, []string{"Matched#CN1"})
	sources := liveClientPositionMatchSourceCounts(nil)
	if position != "" {
		sources[source]++
	}
	a.finalizeLiveClientPlayerListShape(68002, 1, sources)
	diagnostics, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	text := string(diagnostics)
	if !strings.Contains(text, `"player_count":2`) || !strings.Contains(text, `"position_matched_count":1`) {
		t.Fatalf("matched count must describe roster matches, not playerlist size: %s", text)
	}
}

func TestR58LiveClientPlayerListUnavailableFallsBackWithoutError(t *testing.T) {
	players := make([]map[string]any, 18)
	for index := range players {
		players[index] = map[string]any{"summonerName": fmt.Sprintf("Fallback%02d", index+1), "championId": index + 1}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-gameflow/v1/gameflow-phase":
			_, _ = w.Write([]byte(`"InProgress"`))
		case "/lol-gameflow/v1/session":
			_ = json.NewEncoder(w).Encode(map[string]any{"gameData": map[string]any{
				"gameId": int64(9002), "queue": map[string]any{"id": 1750, "mapId": 30, "gameMode": "CHERRY"},
				"teamOne": players[:9], "teamTwo": players[9:],
			}})
		default:
			http.Error(w, "not available", http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{
		connected: true, lcu: client,
		liveClientPlayerList: func(context.Context) ([]byte, int, error) { return nil, 0, errors.New("offline") },
	}
	recorder := httptest.NewRecorder()
	a.handleGameplayLive(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/live", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response gameplayLiveResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.ArenaGrouped || response.ArenaMascotMapping || response.ArenaGroupSource != "session-order" || len(response.Players) != 18 {
		t.Fatal("session order fallback failed")
	}
}

func TestArena1750UsesCorroboratedGameflowRosterOrderWhenPlayerListHasNoSubteams(t *testing.T) {
	players, _ := r62ArenaFixturePlayers(t, 18)
	phase := "InProgress"
	gameID := int64(9003)
	server := r62ArenaGameflowServer(t, &phase, &gameID, players)
	a := &app{
		connected: true,
		lcu:       &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()},
		liveClientPlayerList: func(context.Context) ([]byte, int, error) {
			return []byte(`[{"riotId":"PositionOnly#CN1","position":"TOP","team":"ORDER"}]`), http.StatusOK, nil
		},
	}
	response := r62GameplayLiveResponse(t, a)
	if !response.ArenaGrouped || response.ArenaMascotMapping || response.ArenaGroupSource != "session-order" {
		t.Fatalf("session-order Arena grouping = %#v", response)
	}
	counts := make(map[string]int)
	for _, player := range response.Players {
		counts[player.ArenaGroup]++
	}
	if len(counts) != 6 {
		t.Fatalf("session-order group counts = %#v", counts)
	}
	for group, count := range counts {
		if group == "" || count != 3 {
			t.Fatalf("session-order group %q count = %d", group, count)
		}
	}
}

func TestR62ArenaPlayerListRetriesAndGroupsSeventeenPlayers(t *testing.T) {
	gameflowPlayers, liveRaw := r62ArenaFixturePlayers(t, 17)
	phase := "InProgress"
	gameID := int64(6201)
	server := r62ArenaGameflowServer(t, &phase, &gameID, gameflowPlayers)
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	var probes atomic.Int32
	now := time.Unix(1_800_000_000, 0)
	a := &app{
		connected: true,
		lcu:       client,
		summoner:  Summoner{PUUID: "r90-player-identity-01"},
		liveClientPlayerListNow: func() time.Time {
			current := now
			now = now.Add(liveClientRetryDelay + time.Second)
			return current
		},
		liveClientPlayerList: func(context.Context) ([]byte, int, error) {
			if probes.Add(1) == 1 {
				return nil, 0, errors.New("game process is not listening yet")
			}
			return liveRaw, http.StatusOK, nil
		},
	}
	knownAllies := make([]struct {
		player lcuLivePlayer
		team   int64
	}, 3)
	for index := range knownAllies {
		knownAllies[index] = struct {
			player lcuLivePlayer
			team   int64
		}{player: lcuLivePlayer{PUUID: fmt.Sprintf("r90-player-identity-%02d", index+1)}, team: 100}
	}
	a.rememberArenaAllies(knownAllies)

	first := r62GameplayLiveResponse(t, a)
	if first.ArenaGrouped {
		t.Fatalf("first unavailable probe unexpectedly grouped players: %#v", first)
	}
	a.liveSnapshots.mu.Lock()
	a.liveSnapshots.at = time.Now().Add(-21 * time.Second)
	a.liveSnapshots.mu.Unlock()
	second := r62GameplayLiveResponse(t, a)
	if !second.ArenaGrouped || second.ArenaMascotMapping || len(second.Players) != 17 {
		t.Fatalf("17-player Arena grouping = grouped:%v mascot:%v players:%d", second.ArenaGrouped, second.ArenaMascotMapping, len(second.Players))
	}
	counts := make(map[string]int)
	for index, player := range second.Players {
		counts[player.ArenaGroup]++
		if index < 3 && !player.IsAlly {
			t.Fatalf("remembered ally %d lost highlight: %#v", index, player)
		}
		if index >= 3 && player.IsAlly {
			t.Fatalf("opponent %d was highlighted as ally: %#v", index, player)
		}
	}
	if len(counts) != 6 || counts["6"] != 2 {
		t.Fatalf("17-player Arena group counts = %#v", counts)
	}
	if probes.Load() != 2 {
		t.Fatalf("playerlist probes = %d, want retry after initial failure", probes.Load())
	}
}

func TestR62ArenaPlayerListProbesReconnectAndInvalidatesGameCache(t *testing.T) {
	gameflowPlayers, liveRaw := r62ArenaFixturePlayers(t, 18)
	phase := "Reconnect"
	gameID := int64(6202)
	server := r62ArenaGameflowServer(t, &phase, &gameID, gameflowPlayers)
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	var probes atomic.Int32
	a := &app{
		connected: true,
		lcu:       client,
		liveClientPlayerList: func(context.Context) ([]byte, int, error) {
			probes.Add(1)
			return liveRaw, http.StatusOK, nil
		},
	}
	first := r62GameplayLiveResponse(t, a)
	if !first.ArenaGrouped || probes.Load() != 1 {
		t.Fatalf("Reconnect probe = grouped:%v probes:%d", first.ArenaGrouped, probes.Load())
	}
	second := r62GameplayLiveResponse(t, a)
	if !second.ArenaGrouped || probes.Load() != 1 {
		t.Fatalf("successful probe was not cached: grouped:%v probes:%d", second.ArenaGrouped, probes.Load())
	}
	gameID = 6203
	third := r62GameplayLiveResponse(t, a)
	if !third.ArenaGrouped || probes.Load() != 2 {
		t.Fatalf("new game did not invalidate playerlist cache: grouped:%v probes:%d", third.ArenaGrouped, probes.Load())
	}
}

func TestR62ArenaGroupValidationRejectsOversizedGroups(t *testing.T) {
	players := []gameplayLivePlayer{
		{ArenaGroup: "1"}, {ArenaGroup: "1"}, {ArenaGroup: "1"}, {ArenaGroup: "1"},
		{ArenaGroup: "2"}, {ArenaGroup: "2"},
	}
	if validateArenaLiveGroups(players, 3) {
		t.Fatalf("four-player Arena group passed validation: %#v", players)
	}
	if liveClientArenaDistribution(map[string]int{"1": 9, "2": 9}, 18, 3) {
		t.Fatal("two coarse nine-player teams passed Arena distribution validation")
	}
}

func TestLiveRecommendationRejectionRecordsOnlyInputShape(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	a := &app{storage: trackTestStore(t, &localStore{root: root})}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/gameplay/recommendations?championId=-3", nil)
	a.handleGameplayRecommendations(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	data, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	logText := string(data)
	for _, want := range []string{`"event":"live_recommendations_rejected"`, `"stage":"championId"`, `"raw_length":2`, `"raw_first_character_class":"minus"`} {
		if !strings.Contains(logText, want) {
			t.Fatalf("rejection shape missing %s: %s", want, data)
		}
	}
	if strings.Contains(logText, `"raw":"-3"`) {
		t.Fatalf("rejection diagnostic leaked the raw input: %s", data)
	}
}

func TestGameplayLiveUsesCurrentChampionWhenRosterIsUnavailable(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-gameflow/v1/gameflow-phase":
			_, _ = w.Write([]byte(`"ChampSelect"`))
		case "/lol-gameflow/v1/session":
			http.Error(w, "gameflow roster unavailable", http.StatusNotFound)
		case "/lol-champ-select/v1/session":
			_, _ = w.Write([]byte(`{"myTeam":[],"theirTeam":[]}`))
		case "/lol-champ-select/v1/current-champion":
			_, _ = w.Write([]byte(`64`))
		case "/lol-lobby/v2/lobby":
			_, _ = w.Write([]byte(`{"gameConfig":{"queueId":1750,"mapId":30,"gameMode":"CHERRY"}}`))
		case "/lol-game-data/assets/v1/champions/64.json":
			_, _ = w.Write([]byte(`{"spells":[{"name":"Q"},{"name":"W"},{"name":"E"},{"name":"R"}]}`))
		default:
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{connected: true, lcu: client, storage: trackTestStore(t, &localStore{root: root})}
	recorder := httptest.NewRecorder()
	a.handleGameplayLive(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/live", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response gameplayLiveResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Available || response.CurrentChampionID != 64 || len(response.Players) != 0 {
		t.Fatalf("current champion fallback response = %#v", response)
	}
	if len(response.ChampionAbilities) != 4 {
		t.Fatalf("current champion abilities = %#v", response.ChampionAbilities)
	}
	diagnostics, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(diagnostics), `"event":"lcu_champ_select_session_shape"`) || !strings.Contains(string(diagnostics), `"game_mode":"CHERRY"`) || !strings.Contains(string(diagnostics), `"queue_id":1750`) || !strings.Contains(string(diagnostics), `"my_team_length":0`) {
		t.Fatalf("champ-select handler shape diagnostic = %s", diagnostics)
	}
}

func TestGameplayLiveMarksCurrentPlayerFromChampSelectCellIdentity(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-gameflow/v1/gameflow-phase":
			_, _ = w.Write([]byte(`"ChampSelect"`))
		case "/lol-gameflow/v1/session":
			http.Error(w, "gameflow roster unavailable", http.StatusNotFound)
		case "/lol-champ-select/v1/session":
			_, _ = w.Write([]byte(`{"localPlayerCellId":0,"myTeam":[{"cellId":0,"puuid":"","summonerId":0,"championId":64},{"cellId":1,"puuid":"","summonerId":0,"championId":103}],"theirTeam":[]}`))
		case "/lol-champ-select/v1/current-champion":
			_, _ = w.Write([]byte(`64`))
		case "/lol-lobby/v2/lobby":
			_, _ = w.Write([]byte(`{"gameConfig":{"queueId":420,"mapId":11,"gameMode":"CLASSIC"}}`))
		default:
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{connected: true, lcu: client, storage: trackTestStore(t, &localStore{root: root})}
	recorder := httptest.NewRecorder()
	a.handleGameplayLive(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/live", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response gameplayLiveResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Players) != 2 {
		t.Fatalf("players = %#v", response.Players)
	}
	if !response.Players[0].IsCurrent || response.Players[1].IsCurrent {
		t.Fatalf("cell identity current flags = %#v", response.Players)
	}
}

func TestGameplayLiveArenaChampSelectFiltersAnonymousCells(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-gameflow/v1/gameflow-phase":
			_, _ = w.Write([]byte(`"ChampSelect"`))
		case "/lol-gameflow/v1/session":
			http.Error(w, "gameflow roster unavailable", http.StatusNotFound)
		case "/lol-champ-select/v1/session":
			_, _ = w.Write([]byte(`{"myTeam":[{"puuid":"arena-player-1","championId":64},{"puuid":"arena-player-2","championId":103},{"puuid":"arena-player-3","championId":266},{"puuid":"","championId":22},{"puuid":"00000000-0000-0000-0000-000000000000","championId":51}],"theirTeam":[]}`))
		case "/lol-champ-select/v1/current-champion":
			_, _ = w.Write([]byte(`64`))
		case "/lol-lobby/v2/lobby":
			_, _ = w.Write([]byte(`{"gameConfig":{"queueId":1750,"mapId":30,"gameMode":"CHERRY"}}`))
		default:
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{connected: true, lcu: client, storage: trackTestStore(t, &localStore{root: root})}
	recorder := httptest.NewRecorder()
	a.handleGameplayLive(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/live", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response gameplayLiveResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Players) != 3 {
		t.Fatalf("Arena champ-select players = %d, want only the three identified players: %#v", len(response.Players), response.Players)
	}
	if response.ChampSelectNotice != arenaChampSelectNotice {
		t.Fatalf("Arena champ-select notice = %q", response.ChampSelectNotice)
	}
}

func TestGameplayLiveChampSelectDropsStaleGameflowRoster(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	oldPlayers := make([]map[string]any, 18)
	for index := range oldPlayers {
		oldPlayers[index] = map[string]any{"puuid": fmt.Sprintf("old-player-%d", index), "championId": 11}
	}
	gameflow, err := json.Marshal(map[string]any{"gameData": map[string]any{
		"gameId": int64(999999), "queue": map[string]any{"id": 1750, "mapId": 30, "gameMode": "CHERRY"},
		"teamOne": oldPlayers, "teamTwo": []any{},
	}, "map": map[string]any{"id": 30, "gameMode": "CHERRY"}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-gameflow/v1/gameflow-phase":
			_, _ = w.Write([]byte(`"ChampSelect"`))
		case "/lol-gameflow/v1/session":
			_, _ = w.Write(gameflow)
		case "/lol-champ-select/v1/session":
			_, _ = w.Write([]byte(`{"gameId":777777,"localPlayerCellId":7,"myTeam":[{"cellId":7,"puuid":"fresh-player"}],"theirTeam":[]}`))
		case "/lol-champ-select/v1/current-champion":
			_, _ = w.Write([]byte(`0`))
		case "/lol-lobby/v2/lobby":
			_, _ = w.Write([]byte(`{"gameConfig":{"queueId":420,"mapId":11,"gameMode":"CLASSIC"}}`))
		default:
			http.Error(w, "not available in fixture", http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{connected: true, lcu: client, summoner: Summoner{PUUID: "fresh-player"}, storage: trackTestStore(t, &localStore{root: root})}
	recorder := httptest.NewRecorder()
	a.handleGameplayLive(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/live", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response gameplayLiveResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Players) != 1 || response.Players[0].PlayerRef == "old-player-0" {
		t.Fatalf("stale roster was retained: %#v", response.Players)
	}
	if response.GameID != 777777 || response.GameID == 999999 || response.RawCount != 0 || !response.DroppedStaleGameData {
		t.Fatalf("stale gameData contract = %#v", response)
	}
	diagnostics, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(diagnostics, []byte(`"dropped_stale_gamedata":true`)) {
		t.Fatalf("stale gameData diagnostic missing: %s", diagnostics)
	}
}

func TestGameplayLiveInProgressPreservesGameflowRoster(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-gameflow/v1/gameflow-phase":
			_, _ = w.Write([]byte(`"InProgress"`))
		case "/lol-gameflow/v1/session":
			_, _ = w.Write([]byte(`{"gameData":{"gameId":888888,"queue":{"id":420,"mapId":11,"gameMode":"CLASSIC"},"teamOne":[{"puuid":"kept-player","championId":64}],"teamTwo":[]}}`))
		default:
			http.Error(w, "not available in fixture", http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{connected: true, lcu: client, summoner: Summoner{PUUID: "kept-player"}}
	recorder := httptest.NewRecorder()
	a.handleGameplayLive(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/live", nil))
	var response gameplayLiveResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.GameID != 888888 || len(response.Players) != 1 || response.Players[0].ChampionID != 64 || response.DroppedStaleGameData {
		t.Fatalf("InProgress gameflow roster changed: %#v", response)
	}
}

func TestArenaLiveClientPlayerListGroupingAndFallback(t *testing.T) {
	entries := make([]map[string]any, 0, 18)
	for group := 1; group <= 6; group++ {
		for member := 1; member <= 3; member++ {
			entries = append(entries, map[string]any{
				"riotId":          fmt.Sprintf("Player-%d-%d#CN1", group, member),
				"playerSubteamId": group,
				"subteamName":     fmt.Sprintf("客户端队名 %d", group),
				"team":            "ORDER",
			})
		}
	}
	raw, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, shape, err := parseLiveClientPlayerList(raw)
	if err != nil || !shape.Grouped || shape.GroupField != "playerSubteamId" || shape.PlayerCount != 18 {
		t.Fatalf("playerlist grouping = %#v/%#v, err=%v", snapshot.Grouping, shape, err)
	}
	if snapshot.Grouping.IdentifiedPlayers != 18 || snapshot.Grouping.NameByGroup["1"] != "客户端队名 1" {
		t.Fatalf("playerlist group names = %#v", snapshot.Grouping)
	}
	counts := make(map[string]int)
	for group := 1; group <= 6; group++ {
		for member := 1; member <= 3; member++ {
			player := lcuLivePlayer{GameName: fmt.Sprintf("Player-%d-%d", group, member), TagLine: "CN1"}
			counts[liveClientGroupingForPlayer(snapshot.Grouping, player, gameplayLivePlayer{})]++
		}
	}
	if len(counts) != 6 {
		t.Fatalf("resolved groups = %#v", counts)
	}
	for group, count := range counts {
		if group == "" || count != 3 {
			t.Fatalf("resolved group %q count = %d", group, count)
		}
	}
	_, fallbackShape, err := parseLiveClientPlayerList([]byte(`[{"riotId":"Only#CN1","team":"ORDER"}]`))
	if err != nil || fallbackShape.Grouped {
		t.Fatalf("invalid playerlist should fall back: %#v, err=%v", fallbackShape, err)
	}
	teamOnly := make([]map[string]any, 18)
	for index := range teamOnly {
		teamOnly[index] = map[string]any{"riotId": fmt.Sprintf("TeamOnly-%d#CN1", index), "team": []string{"ORDER", "CHAOS"}[index/9]}
	}
	teamRaw, err := json.Marshal(teamOnly)
	if err != nil {
		t.Fatal(err)
	}
	_, teamShape, err := parseLiveClientPlayerList(teamRaw)
	if err != nil || teamShape.Grouped {
		t.Fatalf("team field must never be accepted as an Arena subteam: %#v, err=%v", teamShape, err)
	}
}

func TestArenaPlayerListDoesNotCachePositionOnlySnapshotAsFinalGrouping(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	entries := make([]map[string]any, 18)
	for index := range entries {
		entries[index] = map[string]any{"riotId": fmt.Sprintf("Retry-%02d#CN1", index+1), "playerSubteamId": index/3 + 1, "team": "ORDER"}
	}
	groupedRaw, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	var probes atomic.Int32
	a := &app{
		liveClientPlayerListNow: func() time.Time { return now },
		liveClientPlayerList: func(context.Context) ([]byte, int, error) {
			if probes.Add(1) == 1 {
				return []byte(`[{"riotId":"Retry-01#CN1","position":"TOP","team":"ORDER"}]`), http.StatusOK, nil
			}
			return groupedRaw, http.StatusOK, nil
		},
	}
	if snapshot, _ := a.liveClientSnapshotForGame(context.Background(), 9901, "InProgress", 18); len(snapshot.Grouping.ByIdentity) != 0 {
		t.Fatalf("position-only Arena snapshot was accepted: %#v", snapshot)
	}
	now = now.Add(liveClientRetryDelay + time.Second)
	snapshot, _ := a.liveClientSnapshotForGame(context.Background(), 9901, "InProgress", 18)
	if probes.Load() != 2 || len(snapshot.Grouping.ByIdentity) == 0 {
		t.Fatalf("Arena playerlist did not retry to a grouped snapshot: probes=%d snapshot=%#v", probes.Load(), snapshot)
	}
}

func TestArenaSessionOrderGroupingIgnoresPremadePartyBoundaries(t *testing.T) {
	players := make([]lcuLivePlayer, 18)
	for index := range players {
		players[index].TeamParticipantID = int64(index + 1)
	}
	if groups, ok := arenaSessionOrderGroups(1750, players); !ok || len(groups) != 18 || groups[0] != "1" || groups[17] != "6" {
		t.Fatalf("valid Arena session order = %#v/%v", groups, ok)
	}
	players[1].TeamParticipantID = players[0].TeamParticipantID
	if groups, ok := arenaSessionOrderGroups(1750, players); !ok || groups[0] != groups[1] {
		t.Fatalf("party contained within an Arena squad was rejected: %#v/%v", groups, ok)
	}
	players[3].TeamParticipantID = players[0].TeamParticipantID
	if groups, ok := arenaSessionOrderGroups(1750, players); !ok || len(groups) != 18 {
		t.Fatalf("premade IDs incorrectly rejected order: %#v", groups)
	}
	if groups, ok := arenaSessionOrderGroups(1750, players[:17]); ok || groups != nil {
		t.Fatalf("partial Arena roster was accepted as ordered: %#v", groups)
	}
}

func TestLiveClientPlayerListDiagnosticIsStructuralAndDeduplicated(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	entries := make([]map[string]any, 0, 18)
	for group := 1; group <= 6; group++ {
		for member := 1; member <= 3; member++ {
			entries = append(entries, map[string]any{
				"summonerName": fmt.Sprintf("PRIVATE_PLAYER_%d_%d", group, member),
				"team":         "ORDER",
				"subteamId":    group,
				"subteamName":  fmt.Sprintf("私密小队名称%d", group),
			})
		}
	}
	raw, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	_, shape, err := parseLiveClientPlayerList(raw)
	if err != nil {
		t.Fatal(err)
	}
	a := &app{storage: trackTestStore(t, &localStore{root: root})}
	a.recordLiveClientPlayerListShape(123456, 0, liveClientPlayerListShape{}, "unavailable", 1, "InProgress")
	a.recordLiveClientPlayerListShape(123456, 0, liveClientPlayerListShape{}, "unavailable", 2, "InProgress")
	a.recordLiveClientPlayerListShape(123456, http.StatusOK, shape, "grouped", 3, "Reconnect")
	a.recordLiveClientPlayerListShape(123456, http.StatusOK, shape, "grouped", 4, "Reconnect")
	diagnostics, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	text := string(diagnostics)
	if strings.Count(text, `"event":"live_client_playerlist_shape"`) != 2 || strings.Count(text, `"result":"unavailable"`) != 1 || strings.Count(text, `"result":"grouped"`) != 1 || !strings.Contains(text, `"attempt":1`) || !strings.Contains(text, `"phase":"InProgress"`) || !strings.Contains(text, `"attempt":3`) || !strings.Contains(text, `"phase":"Reconnect"`) || !strings.Contains(text, `"http_status":200`) || !strings.Contains(text, `"player_count":18`) || !strings.Contains(text, `"element_keys"`) || !strings.Contains(text, `"team_values"`) || !strings.Contains(text, `"subteam_field_values"`) {
		t.Fatalf("playerlist shape diagnostic = %s", diagnostics)
	}
	if strings.Contains(text, "PRIVATE_PLAYER") || strings.Contains(text, "私密小队名称") {
		t.Fatalf("playerlist diagnostic leaked player-visible text: %s", diagnostics)
	}
}

func TestChampSelectWithoutGameflowRosterKeepsEveryCell(t *testing.T) {
	selected := make([]lcuChampSelectPlayer, 6)
	for index := range selected {
		selected[index] = lcuChampSelectPlayer{
			PUUID:      fmt.Sprintf("champ-select-player-%d", index),
			SummonerID: int64(index + 100),
		}
	}
	merged := mergeChampSelectPlayers(nil, lcuChampSelectSession{MyTeam: selected}, 0)
	if len(merged) != len(selected) {
		t.Fatalf("champ-select-only roster was truncated: got %d rows, want %d", len(merged), len(selected))
	}
}

func TestChampSelectWithGameflowRosterDoesNotAppendPastTeamSizeLimit(t *testing.T) {
	existing := make([]struct {
		player lcuLivePlayer
		team   int64
	}, 5)
	for index := range existing {
		existing[index] = struct {
			player lcuLivePlayer
			team   int64
		}{player: lcuLivePlayer{PUUID: fmt.Sprintf("existing-player-%d", index), SummonerID: int64(index + 1)}, team: 100}
	}
	merged := mergeChampSelectPlayers(existing, lcuChampSelectSession{MyTeam: []lcuChampSelectPlayer{
		{PUUID: "existing-player-0", SummonerID: 1, ChampionID: 64},
		{PUUID: "unmatched-champ-select-player", SummonerID: 999, GameName: "不会追加"},
	}}, len(existing))
	if len(merged) != 5 {
		t.Fatalf("champ-select appended beyond team limit: got %d rows, want 5", len(merged))
	}
	if merged[0].player.ChampionID != 64 {
		t.Fatalf("matching champ-select row was not merged: %#v", merged[0].player)
	}
}

func TestGameplayReferencePreservesPrivacy(t *testing.T) {
	playerRef := strings.Repeat("p", 48)
	reference := gameplayReferenceFromSummoner(Summoner{PUUID: playerRef, Privacy: "private"})
	if reference.Privacy != "PRIVATE" {
		t.Fatalf("summoner privacy was not normalized into reference: %#v", reference)
	}
	merged := mergeGameplayReferences(gameplayReference{PlayerRef: playerRef, Privacy: "unknown"}, reference)
	if merged.Privacy != "PRIVATE" {
		t.Fatalf("reference merge lost privacy: %#v", merged)
	}
	if summoner := summonerFromGameplayReference(merged); summoner.Privacy != "PRIVATE" {
		t.Fatalf("reference conversion lost privacy: %#v", summoner)
	}

	a := &app{token: "session-secret", gameplayRefs: make(map[string]string), gameplayRefDetails: make(map[string]gameplayReference)}
	publicRef := a.registerGameplayReferenceDetails(merged)
	resolved, ok := a.resolveGameplayReferenceDetails(publicRef)
	if !ok || resolved.Privacy != "PRIVATE" {
		t.Fatalf("registered reference privacy = %#v, ok=%v", resolved, ok)
	}
}

func TestChampSelectKeepsLockedChampionAndPickIntentSeparate(t *testing.T) {
	staleGameflow := []struct {
		player lcuLivePlayer
		team   int64
	}{{player: lcuLivePlayer{PUUID: "player-stale-gameflow", SummonerID: 123, ChampionID: 11}, team: 100}}
	stalePreselected := mergeChampSelectPlayers(staleGameflow, lcuChampSelectSession{MyTeam: []lcuChampSelectPlayer{{
		PUUID: "player-stale-gameflow", SummonerID: 123, ChampionPickIntent: 64,
	}}}, len(staleGameflow))
	if player := stalePreselected[0].player; player.ChampionID != 0 || player.ChampionPickIntent != 64 || player.ChampionLocked {
		t.Fatalf("stale gameflow champion was not cleared: %#v", player)
	}

	preselected := mergeChampSelectPlayers(nil, lcuChampSelectSession{MyTeam: []lcuChampSelectPlayer{{
		PUUID: "player-preselected", SummonerID: 123, ChampionPickIntent: 64,
	}}}, 0)
	if len(preselected) != 1 {
		t.Fatalf("preselected players = %d", len(preselected))
	}
	if player := preselected[0].player; player.ChampionID != 0 || player.ChampionPickIntent != 64 || player.ChampionLocked {
		t.Fatalf("preselected champion state = %#v", player)
	}

	locked := mergeChampSelectPlayers(nil, lcuChampSelectSession{MyTeam: []lcuChampSelectPlayer{{
		PUUID: "player-locked", SummonerID: 456, ChampionID: 64, ChampionPickIntent: 64,
	}}}, 0)
	if len(locked) != 1 {
		t.Fatalf("locked players = %d", len(locked))
	}
	if player := locked[0].player; player.ChampionID != 64 || player.ChampionPickIntent != 64 || !player.ChampionLocked {
		t.Fatalf("locked champion state = %#v", player)
	}
}

func TestHiddenPlayerFallsBackToSummonerIDBeforeLoadingStats(t *testing.T) {
	hiddenRef := strings.Repeat("x", 48)
	realPUUID := strings.Repeat("r", 48)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-summoner/v2/summoners/puuid/" + hiddenRef:
			http.Error(w, "hidden", http.StatusNotFound)
		case "/lol-summoner/v1/summoners/77":
			_, _ = w.Write([]byte(`{"summonerId":77,"puuid":"` + realPUUID + `","profileIconId":12,"summonerLevel":99}`))
		default:
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	summoner, capability := loadGameplaySummoner(client, gameplayReference{PlayerRef: hiddenRef, SummonerID: 77})
	if capability.State != capabilityAvailable || summoner.PUUID != realPUUID || summoner.SummonerID != 77 {
		t.Fatalf("summoner id fallback failed: summoner=%#v capability=%#v", summoner, capability)
	}
}

func TestChampionAbilitiesUseClientDescriptionsAndSafeAssetPaths(t *testing.T) {
	httpClient := &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/lol-game-data/assets/v1/champions/103.json" {
			t.Fatalf("unexpected endpoint: %s", request.URL.Path)
		}
		body := `{"spells":[{"name":"欺诈宝珠","description":"放出并收回宝珠。","abilityIconPath":"/lol-game-data/assets/ASSETS/Characters/Ahri/HUD/Icons2D/AhriQ.png","costCoefficients":[55,65,75,85,95],"cooldownCoefficients":[7,7,7,7,7],"range":[970,970,970,970,970]},{"name":"妖异狐火","tooltip":"释放三团狐火。","imagePath":"/lol-game-data/assets/ASSETS/Characters/Ahri/HUD/Icons2D/AhriW.png"},{"name":"魅惑妖术","description":"魅惑第一个敌人。","iconPath":"https://example.com/unsafe.png"},{"name":"灵魄突袭","description":"向前突进。","iconPath":"/lol-game-data/assets/ASSETS/Characters/Ahri/HUD/Icons2D/AhriR.png"}]}`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})}
	client := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "test-token", http: httpClient}
	abilities, err := loadChampionAbilities(client, 103)
	if err != nil || len(abilities) != 4 {
		t.Fatalf("abilities = %#v, err=%v", abilities, err)
	}
	if abilities[0].Slot != "Q" || abilities[0].Description != "放出并收回宝珠。" || abilities[0].IconPath == "" || len(abilities[0].Costs) != 5 || len(abilities[0].Cooldowns) != 5 || len(abilities[0].Ranges) != 5 {
		t.Fatalf("Q ability was not normalized: %#v", abilities[0])
	}
	if abilities[1].Slot != "W" || abilities[1].Description != "释放三团狐火。" || abilities[1].IconPath == "" {
		t.Fatalf("W tooltip fallback failed: %#v", abilities[1])
	}
	if abilities[2].IconPath != "" {
		t.Fatalf("unsafe ability asset path was accepted: %#v", abilities[2])
	}
}

func TestChampionAbilitiesFallBackToDataDragonWhenClientFails(t *testing.T) {
	client := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "test-token", http: &http.Client{Transport: gameplayRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("client ability catalog unavailable")
	})}}
	provider := newChampionProvider()
	provider.mu.Lock()
	provider.patch = "16.16.1"
	provider.championKeys["ahri"] = "Ahri"
	provider.championMeta[103] = championMetadata{ID: 103, Key: "Ahri", Slug: "ahri"}
	provider.abilities["16.16.1/ahri"] = map[string]championAssetDescription{
		"Q": {Name: "欺诈宝珠", Description: "放出并收回宝珠。", Path: "/cdn/16.16.1/img/spell/AhriQ.png", Costs: []float64{55, 65}, Cooldowns: []float64{7}, Ranges: []float64{970}},
		"W": {Name: "妖异狐火", Path: "/cdn/16.16.1/img/spell/AhriW.png"},
		"E": {Name: "魅惑妖术", Path: "/cdn/16.16.1/img/spell/AhriE.png"},
		"R": {Name: "灵魄突袭", Path: "/cdn/16.16.1/img/spell/AhriR.png"},
	}
	provider.mu.Unlock()

	a := &app{champions: provider}
	abilities, err := a.loadChampionAbilitiesWithFallback(context.Background(), client, 103)
	if err != nil || len(abilities) != 4 {
		t.Fatalf("fallback abilities = %#v, err=%v", abilities, err)
	}
	if abilities[0].Slot != "Q" || abilities[0].IconPath != "ddragon:/cdn/16.16.1/img/spell/AhriQ.png" || len(abilities[0].Costs) != 2 {
		t.Fatalf("fallback Q was not normalized: %#v", abilities[0])
	}
	if abilities[3].Slot != "R" || abilities[3].IconPath == "" {
		t.Fatalf("fallback ultimate was not returned: %#v", abilities[3])
	}
}

func TestGameplaySummonerSpellsExposeDescriptionsWithoutExternalAssetPaths(t *testing.T) {
	httpClient := &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/lol-game-data/assets/v1/summoner-spells.json" {
			t.Fatalf("unexpected endpoint: %s", request.URL.Path)
		}
		body := `[{"id":4,"name":"闪现","description":"瞬间传送一小段距离。","iconPath":"/lol-game-data/assets/DATA/Spells/Icons2D/Summoner_flash.png"},{"id":14,"name":"引燃","description":"造成持续真实伤害。","imagePath":"https://example.com/unsafe.png"}]`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})}
	client := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "test-token", http: httpClient}
	a := &app{connected: true, lcu: client}
	recorder := httptest.NewRecorder()
	a.handleGameplaySummonerSpells(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/summoner-spells", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Spells []gameplaySummonerSpell `json:"spells"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || len(response.Spells) != 2 {
		t.Fatalf("summoner spells response = %#v, err=%v", response, err)
	}
	if response.Spells[0].Name != "闪现" || response.Spells[0].Description == "" || response.Spells[0].IconPath == "" {
		t.Fatalf("first summoner spell was not normalized: %#v", response.Spells[0])
	}
	if response.Spells[1].IconPath != "" {
		t.Fatalf("unsafe summoner spell asset path was accepted: %#v", response.Spells[1])
	}
}

func TestGameplayItemsExposeDescriptionsWithoutExternalAssetPaths(t *testing.T) {
	httpClient := &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/lol-game-data/assets/v1/items.json" {
			t.Fatalf("unexpected endpoint: %s", request.URL.Path)
		}
		body := `[{"id":3020,"name":"法师之靴","description":"<mainText>提供移动速度与法术穿透。</mainText>","iconPath":"/lol-game-data/assets/ASSETS/Items/Icons2D/3020.png","priceTotal":1100},{"id":3158,"displayName":"明朗之靴","shortDescription":"提供技能急速。","imagePath":"https://example.com/unsafe.png","priceTotal":900}]`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})}
	client := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "test-token", http: httpClient}
	a := &app{connected: true, lcu: client}
	recorder := httptest.NewRecorder()
	a.handleGameplayItems(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/items", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Items []gameplayItem `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || len(response.Items) != 2 {
		t.Fatalf("items response = %#v, err=%v", response, err)
	}
	if response.Items[0].Name != "法师之靴" || response.Items[0].Description == "" || response.Items[0].IconPath == "" || response.Items[0].Price != 1100 {
		t.Fatalf("first item was not normalized: %#v", response.Items[0])
	}
	if response.Items[1].Name != "明朗之靴" || response.Items[1].Description != "提供技能急速。" || response.Items[1].IconPath != "" || response.Items[1].Price != 900 {
		t.Fatalf("fallback or asset validation failed: %#v", response.Items[1])
	}
}

func TestFallbackGameplayItemsExposePricesInStableIDOrder(t *testing.T) {
	provider := newChampionProvider()
	provider.static = map[string]championAssetDescription{
		"item/3153.png": {Name: "破败王者之刃", Path: "/cdn/16.15/img/item/3153.png", Price: 3200},
		"item/2003.png": {Name: "生命药水", Path: "/cdn/16.15/img/item/2003.png", Price: 50},
	}
	a := &app{champions: provider}
	items, err := a.fallbackGameplayItems(context.Background())
	if err != nil || len(items) != 2 || items[0].ID != 2003 || items[0].Price != 50 || items[1].ID != 3153 || items[1].Price != 3200 {
		t.Fatalf("fallback items = %#v, err=%v", items, err)
	}
}

func TestNormalizeGameplayAugmentsUsesChineseNamesAndSafeAssets(t *testing.T) {
	raw := []gameplayAugmentRaw{
		{ID: 2087, Name: "Eureka", NameTRA: " 大法师 ", Description: "<mainText>根据法力值<br>获得法术强度。</mainText>", Rarity: " kPrismatic ", AugmentSmallIconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/Eureka_small.png"},
		{ID: 1205, Name: "ADAPt", SimpleNameTRA: "物理转魔法", Tooltip: "<b>转化额外攻击力</b>", Rarity: "kSilver", IconPath: "https://example.com/unsafe.png"},
		{ID: 2087, NameTRA: "重复项", AugmentSmallIconPath: "/lol-game-data/assets/duplicate.png"},
		{ID: 0, NameTRA: "无效项"},
		{ID: 9999},
	}

	augments := normalizeGameplayAugments(raw)
	if len(augments) != 2 {
		t.Fatalf("augments = %#v", augments)
	}
	if augments[0].ID != 1205 || augments[0].Name != "物理转魔法" || augments[0].Description != "转化额外攻击力" || augments[0].IconPath != "" {
		t.Fatalf("fallback fields or unsafe asset handling changed: %#v", augments[0])
	}
	if augments[1].ID != 2087 || augments[1].Name != "大法师" || augments[1].Description != "根据法力值\n获得法术强度。" || augments[1].Rarity != "kPrismatic" || augments[1].IconPath != "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/Eureka_large.png" || augments[1].FallbackIconPath != "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/Eureka_small.png" {
		t.Fatalf("Chinese augment normalization changed: %#v", augments[1])
	}
}

func TestFallbackGameplayAugmentsRetainsKiwiAndCherryIcons(t *testing.T) {
	provider := newChampionProvider()
	provider.client = &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		var body string
		switch request.URL.Path {
		case "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/cherry-augments.json":
			body = `[{"id":1,"nameTRA":"Kiwi 海克斯","augmentSmallIconPath":"/lol-game-data/assets/ASSETS/UX/Kiwi/Augments/kiwi.png"},{"id":2,"nameTRA":"Cherry 海克斯","augmentSmallIconPath":"/lol-game-data/assets/ASSETS/UX/Cherry/Augments/cherry.png"}]`
		case "/latest/cdragon/arena/zh_cn.json":
			body = `{"augments":[]}`
		default:
			t.Fatalf("unexpected CommunityDragon path: %s", request.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: request}, nil
	})}

	augments, err := (&app{champions: provider}).fallbackGameplayAugments(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	index := gameplayAugmentIndexAll(augments)
	if len(augments) != 2 || index[1].Name == "" || index[2].Name == "" {
		t.Fatalf("fallback catalog must retain both namespaces: %#v", augments)
	}
}

func TestRequestJSONAuthenticatesAndRejectsUnsafePaths(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("riot:test-token"))
		if r.Method != http.MethodPut || r.URL.Path != "/lol-perks/v1/pages/7" || r.Header.Get("Authorization") != wantAuth {
			t.Fatalf("unexpected request: %s %s auth=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["primaryStyleId"] != float64(8000) {
			t.Fatalf("unexpected JSON body: %#v, %v", body, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	var response struct {
		OK bool `json:"ok"`
	}
	if err := client.RequestJSON(context.Background(), http.MethodPut, "/lol-perks/v1/pages/7", map[string]any{"primaryStyleId": 8000}, &response); err != nil || !response.OK {
		t.Fatalf("RequestJSON failed: response=%#v err=%v", response, err)
	}
	for _, path := range []string{"relative", "//remote/path", "/lol-perks/../summoner", "/%2e%2e/private", `/lol-perks\v1\pages`} {
		if err := client.RequestJSON(context.Background(), http.MethodGet, path, nil, nil); err == nil {
			t.Fatalf("unsafe path accepted: %q", path)
		}
	}
	if err := client.RequestJSON(context.Background(), http.MethodConnect, "/lol-perks/v1/pages", nil, nil); err == nil {
		t.Fatal("unsupported method was accepted")
	}
}

func TestApplyRunePageCreatesNewPageAndMakesItCurrent(t *testing.T) {
	requests := make([]string, 0, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		switch r.Method + " " + r.URL.Path {
		case "GET /lol-perks/v1/inventory":
			_, _ = w.Write([]byte(`{"canAddCustomPage":true}`))
		case "POST /lol-perks/v1/pages/":
			var body struct {
				Name           string `json:"name"`
				IsEditable     bool   `json:"isEditable"`
				PrimaryStyleID string `json:"primaryStyleId"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name != "[DL] 李青 · OPGG" || !body.IsEditable || body.PrimaryStyleID != "8000" {
				t.Fatalf("unexpected new rune page body: %#v, %v", body, err)
			}
			_, _ = w.Write([]byte(`{"id":7}`))
		case "PUT /lol-perks/v1/pages/7":
			var body struct {
				ID              int64   `json:"id"`
				Name            string  `json:"name"`
				SelectedPerkIDs []int64 `json:"selectedPerkIds"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID != 7 || body.Name != "[DL] 李青 · OPGG" || len(body.SelectedPerkIDs) != 9 {
				t.Fatalf("unexpected rune page body: %#v, %v", body, err)
			}
			w.WriteHeader(http.StatusNoContent)
		case "PUT /lol-perks/v1/currentpage":
			var pageID int64
			if err := json.NewDecoder(r.Body).Decode(&pageID); err != nil || pageID != 7 {
				t.Fatalf("current page body = %d, %v", pageID, err)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	request := gameplayRuneApplyRequest{ChampionName: "李青", Source: "OPGG", ChampionID: 64, PrimaryStyleID: 8000, SubStyleID: 8100, SelectedPerkIDs: []int64{8001, 8002, 8003, 8004, 8101, 8102, 5001, 5002, 5003}}
	pageID, err := applyRunePage(context.Background(), client, request)
	if err != nil || pageID != 7 {
		t.Fatalf("applyRunePage = %d, %v", pageID, err)
	}
	want := []string{"GET /lol-perks/v1/inventory", "POST /lol-perks/v1/pages/", "PUT /lol-perks/v1/pages/7", "PUT /lol-perks/v1/currentpage"}
	if strings.Join(requests, "|") != strings.Join(want, "|") {
		t.Fatalf("request order = %#v, want %#v", requests, want)
	}
}

func TestApplyRunePageFullWithOnlyUserPagesNeverDeletes(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.Method + " " + r.URL.Path {
		case "GET /lol-perks/v1/inventory":
			_, _ = w.Write([]byte(`{"canAddCustomPage":false}`))
		case "GET /lol-perks/v1/pages":
			_, _ = w.Write([]byte(`[{"id":11,"name":"我的符文页","isDeletable":true,"lastModified":100}]`))
		default:
			t.Fatalf("user rune page was touched: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	request := gameplayRuneApplyRequest{ChampionName: "李青", Source: "OPGG", ChampionID: 64, PrimaryStyleID: 8000, SubStyleID: 8100, SelectedPerkIDs: []int64{8001, 8002, 8003, 8004, 8101, 8102, 5001, 5002, 5003}}
	pageID, err := applyRunePage(context.Background(), client, request)
	if pageID != 0 || !errors.Is(err, errRunePageLimit) || requests != 2 {
		t.Fatalf("applyRunePage = %d, %v with %d requests", pageID, err, requests)
	}
}

func TestApplyRunePageFullDeletesOnlyOldestOwnedPageThenCreates(t *testing.T) {
	requests := make([]string, 0, 7)
	inventoryReads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestKey := r.Method + " " + r.URL.Path
		requests = append(requests, requestKey)
		switch requestKey {
		case "GET /lol-perks/v1/inventory":
			inventoryReads++
			_, _ = fmt.Fprintf(w, `{"canAddCustomPage":%t}`, inventoryReads > 1)
		case "GET /lol-perks/v1/pages":
			_, _ = w.Write([]byte(`[{"id":9,"name":"用户页","isDeletable":true,"lastModified":1},{"id":22,"name":"[DL] 新页","isDeletable":true,"lastModified":200},{"id":33,"name":"[DL] 最旧页","lastModified":100}]`))
		case "DELETE /lol-perks/v1/pages/33":
			w.WriteHeader(http.StatusNoContent)
		case "POST /lol-perks/v1/pages/":
			_, _ = w.Write([]byte(`{"id":7}`))
		case "PUT /lol-perks/v1/pages/7", "PUT /lol-perks/v1/currentpage":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected rune page request: %s", requestKey)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	request := gameplayRuneApplyRequest{ChampionName: "李青", Source: "OPGG", ChampionID: 64, PrimaryStyleID: 8000, SubStyleID: 8100, SelectedPerkIDs: []int64{8001, 8002, 8003, 8004, 8101, 8102, 5001, 5002, 5003}}
	pageID, err := applyRunePage(context.Background(), client, request)
	if err != nil || pageID != 7 {
		t.Fatalf("applyRunePage = %d, %v", pageID, err)
	}
	want := []string{"GET /lol-perks/v1/inventory", "GET /lol-perks/v1/pages", "DELETE /lol-perks/v1/pages/33", "GET /lol-perks/v1/inventory", "POST /lol-perks/v1/pages/", "PUT /lol-perks/v1/pages/7", "PUT /lol-perks/v1/currentpage"}
	if strings.Join(requests, "|") != strings.Join(want, "|") {
		t.Fatalf("request order = %#v, want %#v", requests, want)
	}
}

func TestApplyRunePageSortsMissingLastModifiedByID(t *testing.T) {
	inventoryReads := 0
	deleted := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /lol-perks/v1/inventory":
			inventoryReads++
			_, _ = fmt.Fprintf(w, `{"canAddCustomPage":%t}`, inventoryReads > 1)
		case "GET /lol-perks/v1/pages":
			_, _ = w.Write([]byte(`[{"id":9,"name":"[DL] 九"},{"id":4,"name":"[DL] 四"}]`))
		case "DELETE /lol-perks/v1/pages/4":
			deleted = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		case "POST /lol-perks/v1/pages/":
			_, _ = w.Write([]byte(`{"id":7}`))
		case "PUT /lol-perks/v1/pages/7", "PUT /lol-perks/v1/currentpage":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected rune page request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	request := gameplayRuneApplyRequest{ChampionName: "李青", Source: "OPGG", ChampionID: 64, PrimaryStyleID: 8000, SubStyleID: 8100, SelectedPerkIDs: []int64{1, 2, 3, 4, 5, 6}}
	if pageID, err := applyRunePage(context.Background(), client, request); err != nil || pageID != 7 || deleted != "/lol-perks/v1/pages/4" {
		t.Fatalf("applyRunePage=%d,%v deleted=%q", pageID, err, deleted)
	}
}

func TestApplyRunePagePrefixMatchMustBeExact(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /lol-perks/v1/inventory":
			_, _ = w.Write([]byte(`{"canAddCustomPage":false}`))
		case "GET /lol-perks/v1/pages":
			_, _ = w.Write([]byte(`[{"id":11,"name":"我的[DL]页","isDeletable":true,"lastModified":100}]`))
		default:
			t.Fatalf("contains-only rune page was touched: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	_, err := applyRunePage(context.Background(), client, gameplayRuneApplyRequest{ChampionName: "李青", Source: "OPGG", ChampionID: 64, PrimaryStyleID: 8000, SubStyleID: 8100, SelectedPerkIDs: []int64{1, 2, 3, 4, 5, 6}})
	if !errors.Is(err, errRunePageLimit) {
		t.Fatalf("applyRunePage error = %v, want errRunePageLimit", err)
	}
}

func TestApplyRunePageSkipsExplicitlyNonDeletableOwnedPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /lol-perks/v1/inventory":
			_, _ = w.Write([]byte(`{"canAddCustomPage":false}`))
		case "GET /lol-perks/v1/pages":
			_, _ = w.Write([]byte(`[{"id":11,"name":"[DL] 李青 · 绝活哥","isDeletable":false,"lastModified":100}]`))
		default:
			t.Fatalf("non-deletable rune page was touched: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	_, err := applyRunePage(context.Background(), client, gameplayRuneApplyRequest{ChampionName: "李青", Source: "OPGG", ChampionID: 64, PrimaryStyleID: 8000, SubStyleID: 8100, SelectedPerkIDs: []int64{1, 2, 3, 4, 5, 6}})
	if !errors.Is(err, errRunePageLimit) {
		t.Fatalf("applyRunePage error = %v, want errRunePageLimit", err)
	}
}

func TestApplyRunePageRecycleLoopStopsAfterFiveDeletes(t *testing.T) {
	inventoryReads := 0
	deleted := make([]string, 0, runePageRecycleLimit)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/lol-perks/v1/inventory":
			inventoryReads++
			_, _ = w.Write([]byte(`{"canAddCustomPage":false}`))
		case r.Method == http.MethodGet && r.URL.Path == "/lol-perks/v1/pages":
			_, _ = w.Write([]byte(`[{"id":1,"name":"[DL] 一","lastModified":1},{"id":2,"name":"[DL] 二","lastModified":2},{"id":3,"name":"[DL] 三","lastModified":3},{"id":4,"name":"[DL] 四","lastModified":4},{"id":5,"name":"[DL] 五","lastModified":5},{"id":6,"name":"[DL] 六","lastModified":6}]`))
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/lol-perks/v1/pages/"):
			deleted = append(deleted, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request after recycle limit: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	_, err := applyRunePage(context.Background(), client, gameplayRuneApplyRequest{ChampionName: "李青", Source: "OPGG", ChampionID: 64, PrimaryStyleID: 8000, SubStyleID: 8100, SelectedPerkIDs: []int64{1, 2, 3, 4, 5, 6}})
	if !errors.Is(err, errRunePageLimit) || len(deleted) != runePageRecycleLimit || inventoryReads != runePageRecycleLimit+1 {
		t.Fatalf("applyRunePage error=%v deleted=%#v inventoryReads=%d", err, deleted, inventoryReads)
	}
	if got := strings.Join(deleted, "|"); got != "/lol-perks/v1/pages/1|/lol-perks/v1/pages/2|/lol-perks/v1/pages/3|/lol-perks/v1/pages/4|/lol-perks/v1/pages/5" {
		t.Fatalf("deleted pages = %s", got)
	}
}

func TestRuneApplyValidationAndReplayActionValidation(t *testing.T) {
	valid := gameplayRuneApplyRequest{ChampionName: "李青", Source: "OPGG", PrimaryStyleID: 8000, SubStyleID: 8100, SelectedPerkIDs: []int64{1, 2, 3, 4, 5, 6}}
	if err := validateRuneApplyRequest(valid); err != nil {
		t.Fatalf("valid rune request rejected: %v", err)
	}
	invalid := valid
	invalid.ChampionName = strings.Repeat("长", 49)
	if err := validateRuneApplyRequest(invalid); err == nil {
		t.Fatal("rune page name whose prefix pushes it over 60 characters was accepted")
	}
	invalid = valid
	invalid.SelectedPerkIDs = []int64{1, 2, 3, 4, 5, 5}
	if err := validateRuneApplyRequest(invalid); err == nil {
		t.Fatal("duplicate perk ids were accepted")
	}
	valid.SelectedPerkIDs = []int64{8001, 8002, 8003, 8004, 8101, 8102, 5008, 5008, 5001}
	if err := validateRuneApplyRequest(valid); err != nil {
		t.Fatalf("duplicate stat shards were rejected: %v", err)
	}
	a := &app{}
	recorder := httptest.NewRecorder()
	a.handleGameplayReplayAction(recorder, httptest.NewRequest(http.MethodPost, "/api/gameplay/replay", strings.NewReader(`{"gameId":42,"action":"watch/../../private"}`)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("replay injection status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestRuneApplyHandlerNeverWritesOutsideChampionSelect(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet || r.URL.Path != "/lol-gameflow/v1/gameflow-phase" {
			t.Fatalf("unexpected write outside champion select: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`"Lobby"`))
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{connected: true, lcu: client}
	body := `{"championName":"李青","source":"OPGG","championId":64,"primaryStyleId":8000,"subStyleId":8100,"selectedPerkIds":[1,2,3,4,5,6]}`
	recorder := httptest.NewRecorder()
	a.handleGameplayRuneApply(recorder, httptest.NewRequest(http.MethodPost, "/api/gameplay/runes/apply", strings.NewReader(body)))
	if recorder.Code != http.StatusConflict || requests != 1 {
		t.Fatalf("status/requests = %d/%d, want %d/1", recorder.Code, requests, http.StatusConflict)
	}
}

func TestRuneApplyHandlerExplainsClientPageLimit(t *testing.T) {
	requests := make([]string, 0, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/lol-gameflow/v1/gameflow-phase":
			_, _ = w.Write([]byte(`"ChampSelect"`))
		case "/lol-perks/v1/inventory":
			_, _ = w.Write([]byte(`{"canAddCustomPage":false}`))
		case "/lol-perks/v1/pages":
			_, _ = w.Write([]byte(`[{"id":11,"name":"用户页","isDeletable":true}]`))
		default:
			t.Fatalf("user rune page was modified: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{connected: true, lcu: client}
	body := `{"championName":"李青","source":"OPGG","championId":64,"primaryStyleId":8000,"subStyleId":8100,"selectedPerkIds":[1,2,3,4,5,6]}`
	recorder := httptest.NewRecorder()
	a.handleGameplayRuneApply(recorder, httptest.NewRequest(http.MethodPost, "/api/gameplay/runes/apply", strings.NewReader(body)))
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), runePageLimitMessage) {
		t.Fatalf("status/body = %d/%q", recorder.Code, recorder.Body.String())
	}
	want := []string{"GET /lol-gameflow/v1/gameflow-phase", "GET /lol-perks/v1/inventory", "GET /lol-perks/v1/pages"}
	if strings.Join(requests, "|") != strings.Join(want, "|") {
		t.Fatalf("request order = %#v, want %#v", requests, want)
	}
}

func TestRuneApplyHandlerReturnsAndRecordsAppliedPerks(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /lol-gameflow/v1/gameflow-phase":
			_, _ = w.Write([]byte(`"ChampSelect"`))
		case "GET /lol-perks/v1/inventory":
			_, _ = w.Write([]byte(`{"canAddCustomPage":true}`))
		case "POST /lol-perks/v1/pages/":
			_, _ = w.Write([]byte(`{"id":7}`))
		case "PUT /lol-perks/v1/pages/7":
			_, _ = w.Write([]byte(`{"updated":true}`))
		case "PUT /lol-perks/v1/currentpage":
			_, _ = w.Write([]byte(`{"current":7}`))
		default:
			http.Error(w, "unexpected rune endpoint", http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	store := trackTestStore(t, &localStore{root: root})
	a := &app{connected: true, lcu: client, storage: store}
	perkIDs := []int64{8001, 8002, 8003, 8004, 8101, 8102, 5001, 5008, 5011}
	body := `{"championName":"李青","source":"OPGG","championId":64,"primaryStyleId":8000,"subStyleId":8100,"selectedPerkIds":[8001,8002,8003,8004,8101,8102,5001,5008,5011]}`
	recorder := httptest.NewRecorder()
	a.handleGameplayRuneApply(recorder, httptest.NewRequest(http.MethodPost, "/api/gameplay/runes/apply", strings.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status/body = %d/%s", recorder.Code, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["pageId"] != float64(7) || len(response["selectedPerkIds"].([]any)) != len(perkIDs) {
		t.Fatalf("apply response = %#v", response)
	}
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"event":"perk_apply_attempt"`) || !strings.Contains(string(data), `"outcome":"success"`) || !strings.Contains(string(data), `"phase":"ChampSelect"`) || !strings.Contains(string(data), `"perk_ids":[8001,8002,8003,8004,8101,8102,5001,5008,5011]`) || !strings.Contains(string(data), `"update-page":"{\"updated\":true}"`) {
		t.Fatalf("perk apply diagnostic = %s", data)
	}
}

func TestGameplayItemSetValidation(t *testing.T) {
	position, err := normalizeGameplayItemSetPosition("support")
	if err != nil || position != "utility" {
		t.Fatalf("position = %q, %v", position, err)
	}
	valid := gameplayItemSetApplyRequest{
		ChampionID: 64, MapID: 11, Position: position,
		Blocks: []gameplayItemSetBlockRequest{{Type: "出门装", Items: []gameplayItemSetItemRequest{{ID: 1055, Count: 1}, {ID: 2003, Count: 1}}}},
	}
	if err := validateGameplayItemSetRequest(valid); err != nil {
		t.Fatalf("valid item set rejected: %v", err)
	}
	invalid := valid
	invalid.Blocks = []gameplayItemSetBlockRequest{{Type: "重复装备", Items: []gameplayItemSetItemRequest{{ID: 1055, Count: 1}, {ID: 1055, Count: 1}}}}
	if err := validateGameplayItemSetRequest(invalid); err == nil {
		t.Fatal("duplicate item IDs were accepted")
	}
	invalid = valid
	invalid.Position = "unknown"
	if _, err := normalizeGameplayItemSetPosition(invalid.Position); err == nil {
		t.Fatal("unknown position was accepted")
	}
}

func TestGameplayItemSetContextAcceptsLocalChampSelectCellIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-gameflow/v1/gameflow-phase":
			_, _ = w.Write([]byte(`"ChampSelect"`))
		case "/lol-champ-select/v1/session":
			_, _ = w.Write([]byte(`{"localPlayerCellId":0,"myTeam":[{"cellId":0,"puuid":"","summonerId":0,"championId":64}]}`))
		default:
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	if err := validateGameplayItemSetContext(context.Background(), client, Summoner{}, 64); err != nil {
		t.Fatalf("local champ-select cell identity was rejected: %v", err)
	}
}

func TestGameplayRecommendationsAdaptCompleteOPGGData(t *testing.T) {
	rankedTier := 2
	detail := championDetailResponse{
		Stats: championDetailStats{Tier: &rankedTier, WinRate: 52.3, PickRate: 8.4, BanRate: 4.1},
		ItemRanking: []championMetricRow{{
			Assets: []championAsset{{ID: 126697, Name: "狂妄", Kind: "item"}}, WinRate: 55.4, Games: 321, Score: 88.6,
		}},
		Counters: championCounterSections{
			StrongAgainst: []championCounterRow{{ChampionID: 24, Name: "贾克斯"}},
			WeakAgainst:   []championCounterRow{{ChampionID: 122, Name: "德莱厄斯"}},
		},
		Runes: []championRunePage{{
			PrimaryStyle: championAsset{ID: 8000, Name: "精密"}, SubStyle: championAsset{ID: 8400, Name: "坚决"},
			Selected:   []championAsset{{ID: 8010}, {ID: 9111}, {ID: 9104}, {ID: 8299}, {ID: 8446}, {ID: 8453}, {ID: 5008}, {ID: 5008}, {ID: 5001}},
			ShardSlots: [][]championAsset{{{ID: 5008, Active: true}}, {{ID: 5008, Active: true}}, {{ID: 5001, Active: true}}},
			PickRate:   75.9, WinRate: 51.2, Games: 3521,
		}},
		Build: championBuildSections{
			SummonerSpells: []championMetricRow{
				{Assets: []championAsset{{ID: 4}, {ID: 12}}, PickRate: 78.2, WinRate: 54.1, Games: 29884},
				{Assets: []championAsset{{ID: 4}, {ID: 14}}, PickRate: 18.6, WinRate: 51.2, Games: 7102},
			},
			Skills:       []championMetricRow{{SkillPriority: []string{"Q", "E", "W"}, SkillOrder: []string{"Q", "W", "E"}, PickRate: 61.2, WinRate: 53.6, Games: 21384}},
			StarterItems: []championMetricRow{{Assets: []championAsset{{ID: 1054}, {ID: 2003}}}},
			Boots:        []championMetricRow{{Assets: []championAsset{{ID: 3047}}}},
			CoreItems:    []championMetricRow{{Assets: []championAsset{{ID: 6630}, {ID: 3071}, {ID: 3053}}, PickRate: 22.1, WinRate: 53.4, Games: 500}},
			FourthItems:  []championMetricRow{{Assets: []championAsset{{ID: 3089}}, WinRate: 61.58, GamesUnavailable: true}},
		},
	}
	result := gameplayRecommendationsFromChampionDetail(164, "top", detail)
	if result.Hero.Tier == nil || *result.Hero.Tier != 2 || result.Hero.WinRate != 52.3 || result.Hero.EmptyReason != "" || len(result.Hero.StrongAgainst) != 1 || result.Hero.StrongAgainst[0].ChampionID != 24 {
		t.Fatalf("hero recommendation = %#v", result.Hero)
	}
	if len(result.Runes.OPGG) != 1 || result.Runes.OPGG[0].ChampionID != 164 || len(result.Runes.OPGG[0].SelectedPerkIDs) != 9 || len(result.Runes.OPGG[0].StatModIDs) != 3 || result.Runes.OPGG[0].StatModIDs[0] != 5008 || result.Runes.OPGG[0].StatModIDs[1] != 5008 {
		t.Fatalf("rune recommendation = %#v", result.Runes.OPGG)
	}
	if len(result.Runes.Specialists) != 0 || len(result.Runes.Pros) != 0 || len(result.Build.SpellOptions) != 2 || len(result.Build.CoreOptions) != 1 || len(result.Build.CoreOptions[0].IDs) != 3 || strings.Join(result.Build.SkillPriority, ",") != "Q,E,W" {
		t.Fatalf("build recommendation = %#v", result.Build)
	}
	if got := result.Build.CoreOptions[0].Stats.PickRate; got == nil || *got != 22.1 {
		t.Fatalf("core distribution stats = %#v", result.Build.CoreOptions[0].Stats)
	}
	if !result.HasItemDepths {
		t.Fatal("ranked recommendations lost the item-depth capability")
	}
	if len(result.Build.FourthOptions) != 1 || !result.Build.FourthOptions[0].GamesUnavailable {
		t.Fatalf("unavailable depth sample flag was lost: %#v", result.Build.FourthOptions)
	}
	if len(result.ItemRanking) != 1 || result.ItemRanking[0].Assets[0].ID != 126697 || result.ItemRanking[0].WinRate != 55.4 || result.ItemRanking[0].Games != 321 || result.ItemRanking[0].Score != 88.6 {
		t.Fatalf("item ranking was not passed through: %#v", result.ItemRanking)
	}
	if position, err := normalizeOPGGPosition("utility"); err != nil || position != "support" {
		t.Fatalf("normalized position = %q, %v", position, err)
	}
	if position, err := normalizeOPGGPosition(""); err != nil || position != "" {
		t.Fatalf("empty position should stay unresolved: %q, %v", position, err)
	}
	if tier, err := gameplayRecommendationTier(opggModeSpecs["ranked"], ""); err != nil || tier != championCounterFallbackTier {
		t.Fatalf("empty ranked tier = %q, %v", tier, err)
	}
	if tier, err := gameplayRecommendationTier(opggModeSpecs["ranked"], "diamond"); err != nil || tier != "diamond" {
		t.Fatalf("ranked tier = %q, %v", tier, err)
	}
	if _, err := gameplayRecommendationTier(opggModeSpecs["ranked"], "not-a-tier"); err == nil {
		t.Fatal("invalid ranked tier was accepted")
	}
	if tier, err := gameplayRecommendationTier(opggModeSpecs["arena"], "not-a-tier"); err != nil || tier != "" {
		t.Fatalf("non-tier mode validated a tier: %q, %v", tier, err)
	}
	if position, source, err := resolveGameplayRecommendationPosition("jungle", []championPositionOption{{Position: "jungle", RoleRate: 87}}); err != nil || position != "jungle" || source != "requested" {
		t.Fatalf("requested position resolution = %q/%q, %v", position, source, err)
	}
	if position, source, err := resolveGameplayRecommendationPosition("", []championPositionOption{{Position: "top", RoleRate: 87}, {Position: "mid", RoleRate: 13}}); err != nil || position != "top" || source != "opgg-primary" {
		t.Fatalf("OP.GG primary position resolution = %q/%q, %v", position, source, err)
	}
	if position, source, err := resolveGameplayRecommendationPosition("", []championPositionOption{{Position: "mid", RoleRate: 13}, {Position: "top", RoleRate: 87}}); err != nil || position != "top" || source != "opgg-primary" {
		t.Fatalf("OP.GG primary resolution depended on upstream order = %q/%q, %v", position, source, err)
	}
	if position, source, err := resolveGameplayRecommendationPosition("support", []championPositionOption{{Position: "mid", RoleRate: 87}}); err != nil || position != "mid" || source != "opgg-primary" {
		t.Fatalf("unsupported requested position resolution = %q/%q, %v", position, source, err)
	}
	if position, source, err := resolveGameplayRecommendationPosition("support", nil); err != nil || position != "support" || source != "fallback" {
		t.Fatalf("missing OP.GG position data should use the requested fallback: %q/%q, %v", position, source, err)
	}
	arenaTier := 0
	arenaDetail := championDetailResponse{
		Mode:          "arena",
		ArenaStats:    arenaChampionStats{Tier: &arenaTier, WinRate: 53.4, PickRate: 7.2, BanRate: 2.1},
		ArenaAugments: []championMetricRow{{Assets: []championAsset{{ID: 225, Name: "中文海克斯", Source: "communitydragon", Path: "/latest/game/assets/test.png"}}, PickRate: 18, WinRate: 70}},
		Build: championBuildSections{
			CoreItems:  []championMetricRow{{Assets: []championAsset{{ID: 6630}, {ID: 3071}, {ID: 3053}}, Grade: "A", WinRate: 63.6, AveragePlacement: 2.88, FirstPlaceRate: 25.1, Games: 1055}},
			PrismItems: []championMetricRow{{Assets: []championAsset{{ID: 447101}}, Tier: "B", WinRate: 62.45, AveragePlacement: 2.92, FirstPlaceRate: 24.38, Games: 983}},
		},
	}
	arena := gameplayRecommendationsFromChampionDetail(164, "mid", arenaDetail)
	if arena.Source != "arena" || len(arena.Augments) != 1 || arena.Hero.Tier == nil || *arena.Hero.Tier != 0 || arena.Hero.WinRate != 53.4 || len(arena.Runes.OPGG) != 0 || len(arena.Build.CoreOptions) != 1 || len(arena.Build.PrismOptions) != 1 {
		t.Fatalf("arena recommendation = %#v", arena)
	}
	if arena.HasItemDepths {
		t.Fatal("arena recommendations must not advertise item depths")
	}
	core, prism := arena.Build.CoreOptions[0], arena.Build.PrismOptions[0]
	if core.Grade != "A" || prism.Grade != "B" || core.Stats.AveragePlacement == nil || *core.Stats.AveragePlacement != 2.88 || core.Stats.FirstPlaceRate == nil || *core.Stats.FirstPlaceRate != 25.1 {
		t.Fatalf("arena build metrics were not preserved: core=%#v prism=%#v", core, prism)
	}
	mayhemTier := 1
	mayhem := gameplayRecommendationsFromChampionDetail(164, "", championDetailResponse{
		Mode:  "hextech-aram",
		Stats: championDetailStats{Tier: &mayhemTier, WinRate: 54.7, PickRate: 7.8},
	})
	if mayhem.Source != "hextech-aram" || mayhem.Hero.Tier == nil || *mayhem.Hero.Tier != 1 || mayhem.Hero.WinRate != 54.7 {
		t.Fatalf("mayhem hero recommendation = %#v", mayhem.Hero)
	}
}

func TestGameplayRecommendationsExplainMissingPositionSample(t *testing.T) {
	result := gameplayRecommendationsFromChampionDetail(5, "mid", championDetailResponse{})
	if result.Hero.EmptyReason != "该英雄在这个位置没有统计样本" {
		t.Fatalf("empty reason = %q", result.Hero.EmptyReason)
	}
	encoded, err := json.Marshal(result.Hero)
	if err != nil || !strings.Contains(string(encoded), `"emptyReason":"该英雄在这个位置没有统计样本"`) {
		t.Fatalf("hero payload = %s err=%v", encoded, err)
	}
}

func TestGameplayRecommendationsPreserveAllArenaAugments(t *testing.T) {
	rows := make([]championMetricRow, 16)
	for index := range rows {
		rows[index] = championMetricRow{Assets: []championAsset{{ID: index + 1, Name: fmt.Sprintf("海克斯 %d", index+1)}}}
	}
	result := gameplayRecommendationsFromChampionDetail(64, "mid", championDetailResponse{Mode: "arena", ArenaAugments: rows})
	if len(result.Augments) != len(rows) {
		t.Fatalf("arena recommendation augments were truncated: got %d, want %d", len(result.Augments), len(rows))
	}
}

func TestGameplayRecommendationsHandlerResolvesPositionThroughHTTP(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantPaths  []string
		wantPos    string
		wantSource string
	}{
		{
			name:       "missing live position uses the OP.GG primary lane",
			query:      "championId=13&queueId=420&gameMode=CLASSIC&mapId=11",
			wantPaths:  []string{"/api/KR/champions/ranked/13/MID"},
			wantPos:    "mid",
			wantSource: "opgg-primary",
		},
		{
			name:       "smite lane outside the candidates uses the OP.GG primary lane",
			query:      "championId=13&queueId=420&gameMode=CLASSIC&mapId=11&spell2Id=11",
			wantPaths:  []string{"/api/KR/champions/ranked/13/JUNGLE", "/api/KR/champions/ranked/13/MID"},
			wantPos:    "mid",
			wantSource: "opgg-primary",
		},
		{
			name:       "Ryze top request resolves to the OP.GG primary mid lane",
			query:      "championId=13&queueId=420&gameMode=CLASSIC&mapId=11&position=top",
			wantPaths:  []string{"/api/KR/champions/ranked/13/TOP", "/api/KR/champions/ranked/13/MID"},
			wantPos:    "mid",
			wantSource: "opgg-primary",
		},
		{
			name:       "requested lane in the candidates is retained",
			query:      "championId=13&queueId=420&gameMode=CLASSIC&mapId=11&position=mid",
			wantPaths:  []string{"/api/KR/champions/ranked/13/MID"},
			wantPos:    "mid",
			wantSource: "requested",
		},
		{
			name:       "a secondary candidate remains selectable",
			query:      "championId=13&queueId=420&gameMode=CLASSIC&mapId=11&position=support",
			wantPaths:  []string{"/api/KR/champions/ranked/13/SUPPORT"},
			wantPos:    "support",
			wantSource: "requested",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := newChampionProvider()
			provider.cache = newChampionDataCache(nil)
			provider.patch = "16.16.1"
			provider.championMeta[13] = championMetadata{ID: 13, Key: "Ryze", Slug: "ryze"}
			provider.championIDs["ryze"] = 13
			provider.championKeys["ryze"] = "Ryze"
			provider.static["item/3153.png"] = championAssetDescription{Name: "破败王者之刃"}
			provider.abilities["16.16.1/ryze"] = map[string]championAssetDescription{"Q": {Name: "超负荷"}}

			var detailPaths []string
			provider.client = &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				body := `<html></html>`
				switch request.URL.Host {
				case opggChampionHost:
					if strings.HasSuffix(request.URL.Path, "/versions") {
						body = `{"data":["16.16"]}`
					} else {
						detailPaths = append(detailPaths, request.URL.Path)
						body = `{"data":{"summary":{"id":13,"average_stats":{"play":100,"win_rate":0.52},"positions":[{"name":"MID","stats":{"play":87,"win_rate":0.54,"role_rate":0.87}},{"name":"SUPPORT","stats":{"play":13,"win_rate":0.5,"role_rate":0.13}}]},"core_items":[{"ids":[3153],"play":100,"win":55}]},"meta":{"version":"16.16"}}`
					}
				case dataDragonHost:
					body = `[]`
				}
				return &http.Response{StatusCode: http.StatusOK, Status: http.StatusText(http.StatusOK), Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: request}, nil
			})}

			a := &app{champions: provider}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/gameplay/recommendations?"+test.query, nil)
			a.handleGameplayRecommendations(recorder, request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status/body = %d/%q", recorder.Code, recorder.Body.String())
			}
			var response gameplayRecommendationsResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Position != test.wantPos || response.Recommendations.ResolvedPosition != test.wantPos || response.Recommendations.PositionSource != test.wantSource {
				t.Fatalf("position resolution = response:%q bundle:%q source:%q", response.Position, response.Recommendations.ResolvedPosition, response.Recommendations.PositionSource)
			}
			if strings.Join(detailPaths, "|") != strings.Join(test.wantPaths, "|") {
				t.Fatalf("detail paths = %#v, want %#v", detailPaths, test.wantPaths)
			}
		})
	}
}

func TestGameplayRecommendationsHandlerKeepsGameIDDiagnosticOnlyAndReturnsUsefulData(t *testing.T) {
	if gameID, err := parseOptionalGameID("8956354574"); err != nil || gameID != 8_956_354_574 {
		t.Fatalf("real Riot game ID parsing = %d, %v", gameID, err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(nil)
	provider.patch = "16.16.1"
	provider.championMeta[13] = championMetadata{ID: 13, Key: "Ryze", Slug: "ryze", NameZH: "瑞兹"}
	provider.championMeta[1] = championMetadata{ID: 1, Key: "Annie", Slug: "annie", NameZH: "安妮"}
	provider.championMeta[2] = championMetadata{ID: 2, Key: "Olaf", Slug: "olaf", NameZH: "奥拉夫"}
	provider.championIDs["ryze"] = 13
	provider.championKeys["ryze"] = "Ryze"
	provider.static["item/6657.png"] = championAssetDescription{Name: "永霜"}
	provider.client = &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := `<html></html>`
		if request.URL.Host == opggChampionHost {
			if strings.HasSuffix(request.URL.Path, "/versions") {
				body = `{"data":["16.16"]}`
			} else {
				body = `{"data":{"summary":{"id":13,"average_stats":{"play":1000,"win_rate":0.52,"pick_rate":0.1,"ban_rate":0.03},"positions":[{"name":"MID","stats":{"play":900,"win_rate":0.52,"role_rate":0.9}}]},"core_items":[{"ids":[6657],"play":800,"win":430}],"counters":[{"champion_id":1,"play":100,"win":40},{"champion_id":2,"play":100,"win":60}]},"meta":{"version":"16.16"}}`
			}
		} else if request.URL.Host == dataDragonHost {
			body = `[]`
		}
		return &http.Response{StatusCode: http.StatusOK, Status: http.StatusText(http.StatusOK), Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: request}, nil
	})}
	a := &app{champions: provider, storage: trackTestStore(t, &localStore{root: root})}
	provider.diag = a.recordDiagnostic

	queries := []struct {
		name       string
		gameID     string
		wantReject bool
	}{
		{name: "real ten digit game id", gameID: "8956354574"},
		{name: "malformed diagnostic game id", gameID: "not-a-riot-game", wantReject: true},
	}
	for _, test := range queries {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/gameplay/recommendations?championId=13&queueId=420&gameMode=CLASSIC&mapId=11&position=mid&gameId="+url.QueryEscape(test.gameID), nil)
			a.handleGameplayRecommendations(recorder, request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status/body = %d/%q", recorder.Code, recorder.Body.String())
			}
			var response gameplayRecommendationsResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			bundle := response.Recommendations
			if bundle.Hero.WinRate <= 0 || len(bundle.Build.CoreOptions) == 0 || len(bundle.Hero.StrongAgainst)+len(bundle.Hero.WeakAgainst) == 0 {
				t.Fatalf("recommendation guard failed: hero=%#v build=%#v", bundle.Hero, bundle.Build)
			}
		})
	}
	data, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	logText := string(data)
	if !strings.Contains(logText, `"event":"recommendation_mode_resolved"`) || !strings.Contains(logText, `"event":"live_recommendations_rejected"`) || !strings.Contains(logText, `"stage":"gameId"`) || !strings.Contains(logText, `"status":400`) {
		t.Fatalf("game ID diagnostics = %s", data)
	}
}

func TestGameplayRecommendationModeResolutionMatrix(t *testing.T) {
	tests := []struct {
		name               string
		queueID, mapID     int64
		gameMode, wantMode string
		wantFallback       bool
		wantTopPlayers     bool
		wantItemDepths     bool
	}{
		{"solo queue wins over payload", 420, 30, "CHERRY", "ranked", false, true, true},
		{"flex queue", 440, 11, "CLASSIC", "ranked", false, true, true},
		{"normal queue", 400, 11, "CLASSIC", "ranked", false, true, true},
		{"classic by shape before queue resolves", 0, 11, "CLASSIC", "ranked", false, true, true},
		{"custom classic queue", -1, 11, "CLASSIC", "ranked", false, true, true},
		{"custom queue 3100", 3100, 11, "CLASSIC", "ranked", false, true, true},
		{"aram queue has no top players", 450, 12, "ARAM", "aram", false, false, false},
		{"Tencent ARAM variant", 3220, 12, "ARAM", "aram", false, false, false},
		{"arena queue has no top players", 1700, 30, "CHERRY", "arena", false, false, false},
		{"hextech queue", 2300, 12, "KIWI", "hextech-aram", false, false, false},
		{"queue 2400 is ordinary hextech", 2400, 12, "KIWI", "hextech-aram", false, false, false},
		{"queue 3270 is ordinary hextech", 3270, 12, "KIWI", "hextech-aram", false, false, false},
		{"classic semantics win over queue 2400", 2400, 12, "ARAM_MAYHEM_CLASSIC", "aram", true, false, false},
		{"classic semantics on a new queue", 2600, 12, "ARAM_MAYHEM_CLASSIC", "aram", true, false, false},
		{"arena by shape", 0, 30, "CHERRY", "arena", false, false, false},
		{"aram by shape", 0, 12, "ARAM", "aram", false, false, false},
		{"urf queue", 900, 11, "URF", "urf", false, false, false},
		{"urf by shape", 0, 11, "ARURF", "urf", false, false, false},
		{"nexus by shape", 0, 21, "NEXUSBLITZ", "nexus-blitz", false, false, false},
		{"unknown fallback", 9999, 99, "UNKNOWN", "unsupported", true, false, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := resolveGameplayRecommendationMode(test.queueID, test.gameMode, test.mapID)
			if got.InternalMode != test.wantMode || got.IsFallback != test.wantFallback {
				t.Fatalf("resolution = %#v, want mode=%q fallback=%v", got, test.wantMode, test.wantFallback)
			}
			if hasTopPlayers := recommendationModeHasTopPlayers(got); hasTopPlayers != test.wantTopPlayers {
				t.Fatalf("top-player capability = %v, want %v for %#v", hasTopPlayers, test.wantTopPlayers, got)
			}
			if hasItemDepths := opggModeSpecs[got.InternalMode].SupportsItemDepths; hasItemDepths != test.wantItemDepths {
				t.Fatalf("item-depth capability = %v, want %v for %#v", hasItemDepths, test.wantItemDepths, got)
			}
		})
	}
}

func TestR66LivePremadeAssignmentsUseSharedGameIDsAndUnionFind(t *testing.T) {
	matches := func(ids ...int64) []gameplayMatch {
		result := make([]gameplayMatch, 0, len(ids))
		for _, id := range ids {
			result = append(result, gameplayMatch{GameID: id})
		}
		return result
	}
	inputs := []livePremadeInput{
		{TeamID: 100, TeamParticipantID: 7, Matches: matches(1, 2, 3, 4, 5)},
		{TeamID: 100, TeamParticipantID: 7, Matches: matches(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)},
		{TeamID: 100, TeamParticipantID: 8, Matches: matches(6, 7, 8, 9, 10)},
		{TeamID: 100, TeamParticipantID: 99, Matches: matches(21)},
		{TeamID: 100, TeamParticipantID: 99, Matches: matches(22)},
	}
	got := livePremadeAssignments(inputs, 5)
	for index := 0; index < 3; index++ {
		if got[index].Group != "1" || got[index].Size != 3 {
			t.Fatalf("history-backed transitive group[%d] = %#v", index, got[index])
		}
	}
	if !got[0].SessionSignal || !got[1].SessionSignal || got[2].SessionSignal {
		t.Fatalf("session corroboration = %#v", got[:3])
	}
	if got[3].Group == "" || got[3].Group != got[4].Group || got[3].Source != "session" || got[4].Source != "session" {
		t.Fatalf("teamParticipantId direct group = %#v", got[3:])
	}
}

func TestR66LivePremadeAssignmentsFailClosedWhenHistoryCoverageIsLow(t *testing.T) {
	shared := []gameplayMatch{{GameID: 1}, {GameID: 2}, {GameID: 3}, {GameID: 4}, {GameID: 5}}
	got := livePremadeAssignments([]livePremadeInput{
		{TeamID: 100, Matches: shared},
		{TeamID: 100, Matches: shared},
		{TeamID: 100}, {TeamID: 100}, {TeamID: 100}, {TeamID: 100},
	}, 5)
	for index, assignment := range got {
		if assignment.Group != "" || assignment.Size != 0 {
			t.Fatalf("low-coverage assignment[%d] = %#v", index, assignment)
		}
	}
}

func TestR68ArenaNeverAppliesSessionPremadeGroups(t *testing.T) {
	shared := []gameplayMatch{{GameID: 1}, {GameID: 2}, {GameID: 3}, {GameID: 4}, {GameID: 5}}
	players := make([]gameplayLivePlayer, 2)
	inputs := []livePremadeInput{
		{TeamID: 100, TeamParticipantID: 7, Matches: shared},
		{TeamID: 100, TeamParticipantID: 7, Matches: shared},
	}
	applyLivePremadeAssignments(players, inputs, "InProgress", true)
	for index, player := range players {
		if player.PremadeGroup != "" || player.PremadeSize != 0 || player.PremadeSessionSignal {
			t.Fatalf("CHERRY player %d received a session premade label: %#v", index, player)
		}
	}
}

func TestR69SessionPremadesSurviveEmptyHistoryCoverage(t *testing.T) {
	inputs := make([]livePremadeInput, 10)
	for index := range inputs {
		inputs[index].TeamID = 100
		inputs[index].TeamParticipantID = int64(index + 1)
	}
	inputs[5].TeamID, inputs[6].TeamID = 200, 200
	inputs[5].TeamParticipantID, inputs[6].TeamParticipantID = 6, 6
	inputs[7].TeamID, inputs[8].TeamID, inputs[9].TeamID = 200, 200, 200
	inputs[7].TeamParticipantID, inputs[8].TeamParticipantID, inputs[9].TeamParticipantID = 7, 7, 8
	got := livePremadeAssignments(inputs, livePremadeMinSharedGames)
	for _, index := range []int{5, 6, 7, 8} {
		if got[index].Size != 2 || got[index].Source != "session" {
			t.Fatalf("session assignment[%d] = %#v", index, got[index])
		}
	}
	if got[5].Group == got[7].Group || got[9].Group != "" {
		t.Fatalf("independent session groups = %#v", got[5:])
	}
}

func TestR69HistoryOnlyPremadeIsMarkedInferred(t *testing.T) {
	shared := []gameplayMatch{{GameID: 1}, {GameID: 2}, {GameID: 3}, {GameID: 4}, {GameID: 5}, {GameID: 6}}
	got := livePremadeAssignments([]livePremadeInput{
		{TeamID: 100, Matches: shared}, {TeamID: 100, Matches: shared},
		{TeamID: 100, Matches: []gameplayMatch{{GameID: 10}}},
	}, livePremadeMinSharedGames)
	if got[0].Size != 2 || got[1].Size != 2 || got[0].Source != "inferred" || got[1].Source != "inferred" {
		t.Fatalf("inferred assignments = %#v", got)
	}
}

func TestR69SessionPremadesNeverCrossTeams(t *testing.T) {
	got := livePremadeAssignments([]livePremadeInput{
		{TeamID: 100, TeamParticipantID: 9},
		{TeamID: 200, TeamParticipantID: 9},
	}, livePremadeMinSharedGames)
	if got[0].Group != "" || got[1].Group != "" {
		t.Fatalf("cross-team session signal formed a group: %#v", got)
	}
}

func TestR69LiveHistoryStatesDistinguishUnavailableEmptyAndFailed(t *testing.T) {
	if got := liveHistoryState(false, livePlayerMatchesResult{State: "ok"}); got != "unavailable" {
		t.Fatalf("invalid player reference state = %q", got)
	}
	for _, test := range []struct {
		name       string
		status     int
		body       string
		want       string
		wantCached bool
	}{
		{name: "empty", status: http.StatusOK, body: `{"games":{"gameCount":0,"games":[]}}`, want: "empty", wantCached: true},
		{name: "failed", status: http.StatusInternalServerError, body: `{"message":"temporary"}`, want: "failed", wantCached: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
			a := &app{}
			result := a.livePlayerMatches(context.Background(), client, gameplayReference{PlayerRef: "r69-history-ref-01"}, "r69-history-ref-01", false, nil)
			if got := liveHistoryState(true, result); got != test.want {
				t.Fatalf("history state = %q, want %q (%#v)", got, test.want, result)
			}
			a.livePlayerMatchesMu.Lock()
			_, cached := a.livePlayerMatchCache["r69-history-ref-01\x00false"]
			a.livePlayerMatchesMu.Unlock()
			if cached != test.wantCached {
				t.Fatalf("cached=%v, want %v", cached, test.wantCached)
			}
		})
	}
	t.Run("failed SGP fallback is not cached as empty", func(t *testing.T) {
		playerRef := strings.Repeat("h", 48)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/match-history-query/") {
				http.Error(w, "temporary", http.StatusTeapot)
				return
			}
			_, _ = io.WriteString(w, `{"games":{"gameCount":0,"games":[]}}`)
		}))
		defer server.Close()
		client := &LCUClient{
			baseURL: server.URL, token: "test-token", http: server.Client(),
			region: "TENCENT", rsoPlatform: "HN1", platformProbe: true,
		}
		provider := newSGPProvider()
		provider.http = server.Client()
		provider.serverBases["HN1"] = server.URL
		provider.token, provider.tokenAt, provider.tokenClient = "entitlements", time.Now(), client
		provider.sessionToken, provider.sessionAt, provider.sessionOwner = "league-session", time.Now(), client
		a := &app{sgp: provider}
		result := a.livePlayerMatches(context.Background(), client, gameplayReference{PlayerRef: playerRef, ServerID: "HN1"}, playerRef, false, nil)
		if result.State != "failed" {
			t.Fatalf("SGP fallback failure state = %q, want failed", result.State)
		}
		a.livePlayerMatchesMu.Lock()
		_, cached := a.livePlayerMatchCache[playerRef+"\x00false"]
		a.livePlayerMatchesMu.Unlock()
		if cached {
			t.Fatal("failed SGP fallback was cached")
		}
	})
}

func TestR69LivePositionSnapshotDiagnosticsExposeHitAndMiss(t *testing.T) {
	a := &app{}
	a.rememberLivePositionSnapshot(6901, []gameplayLivePlayer{
		{gameplayPlayer: gameplayPlayer{PlayerRef: "player_1"}, Position: "top"},
		{gameplayPlayer: gameplayPlayer{PlayerRef: "player_2"}, Position: "jungle"},
	})
	hitPlayers := []gameplayLivePlayer{{gameplayPlayer: gameplayPlayer{PlayerRef: "player_1"}}}
	hit := a.applyLivePositionSnapshot(6901, hitPlayers)
	if hit.GameID != 6901 || hit.Size != 2 || hit.AppliedCount != 1 || hitPlayers[0].Position != "top" {
		t.Fatalf("snapshot hit = %#v players=%#v", hit, hitPlayers)
	}
	a.rememberLivePositionSnapshot(6901, []gameplayLivePlayer{
		{gameplayPlayer: gameplayPlayer{PlayerRef: "player_1"}, Position: "top"},
		{gameplayPlayer: gameplayPlayer{PlayerRef: "player_2"}, Position: "jungle"},
	})
	miss := a.applyLivePositionSnapshot(6902, []gameplayLivePlayer{{gameplayPlayer: gameplayPlayer{PlayerRef: "player_1"}}})
	if miss.GameID != 6901 || miss.Size != 2 || miss.AppliedCount != 0 {
		t.Fatalf("snapshot miss = %#v", miss)
	}
	diagnostic := livePositionShapeDiagnostic(gameplayLiveResponse{}, lcuGameflowSession{}, lcuChampSelectSession{}, Summoner{}, miss)
	if diagnostic["snapshot_game_id"] != int64(6901) || diagnostic["snapshot_size"] != 2 || diagnostic["snapshot_applied_count"] != 0 {
		t.Fatalf("snapshot diagnostic = %#v", diagnostic)
	}
}

func TestR69GameflowShapeCountsOnlyNonzeroFields(t *testing.T) {
	team := make([]map[string]any, 10)
	for index := range team {
		team[index] = map[string]any{"summonerName": "", "selectedPosition": ""}
		if index < 5 {
			team[index]["summonerName"] = fmt.Sprintf("private-%d", index)
			team[index]["selectedPosition"] = "TOP"
		}
	}
	raw, err := json.Marshal(map[string]any{"gameData": map[string]any{"teamOne": team[:5], "teamTwo": team[5:]}})
	if err != nil {
		t.Fatal(err)
	}
	payload := lcuGameflowSessionShapePayload(raw, "InProgress", "CLASSIC", 420)
	teamOne, _ := payload["team_one_nonzero_counts"].(map[string]int)
	teamTwo, _ := payload["team_two_nonzero_counts"].(map[string]int)
	if teamOne["summonerName"] != 5 || teamOne["selectedPosition"] != 5 || teamTwo["summonerName"] != 0 || teamTwo["selectedPosition"] != 0 {
		t.Fatalf("nonzero counts = teamOne:%#v teamTwo:%#v", teamOne, teamTwo)
	}
}

func TestR69LiveClientPositionsUseEnrichedRiotIDs(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	positions := []string{"TOP", "JUNGLE", "MIDDLE", "BOTTOM", "UTILITY", "TOP", "JUNGLE", "MIDDLE", "BOTTOM", "UTILITY"}
	gameflowPlayers := make([]map[string]any, 10)
	livePlayers := make([]map[string]any, 10)
	renderNames := make(map[string]string, 10)
	for index, position := range positions {
		playerRef := fmt.Sprintf("r69-player-ref-%02d", index)
		gameName := fmt.Sprintf("RiotOnly%02d", index)
		renderNames[playerRef] = gameName
		gameflowPlayers[index] = map[string]any{"puuid": playerRef, "summonerName": "", "championId": index + 1, "selectedPosition": "TOP"}
		livePlayers[index] = map[string]any{"riotId": gameName + "#CN1", "position": position, "team": []string{"ORDER", "CHAOS"}[index/5]}
	}
	liveRaw, err := json.Marshal(livePlayers)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/lol-gameflow/v1/gameflow-phase":
			_, _ = w.Write([]byte(`"InProgress"`))
		case request.URL.Path == "/lol-gameflow/v1/session":
			_ = json.NewEncoder(w).Encode(map[string]any{"gameData": map[string]any{"gameId": int64(6903), "queue": map[string]any{"id": 420, "mapId": 11, "gameMode": "CLASSIC"}, "teamOne": gameflowPlayers[:5], "teamTwo": gameflowPlayers[5:]}})
		case strings.HasPrefix(request.URL.Path, "/lol-summoner/v2/summoners/puuid/"):
			playerRef, _ := url.PathUnescape(strings.TrimPrefix(request.URL.Path, "/lol-summoner/v2/summoners/puuid/"))
			_ = json.NewEncoder(w).Encode(Summoner{PUUID: playerRef, GameName: renderNames[playerRef], TagLine: "CN1"})
		case strings.HasPrefix(request.URL.Path, "/lol-ranked/v1/ranked-stats/"):
			_, _ = w.Write([]byte(`{"queues":[]}`))
		case strings.HasPrefix(request.URL.Path, "/lol-match-history/v1/products/lol/"):
			_, _ = w.Write([]byte(`{"games":{"gameCount":0,"games":[]}}`))
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()
	a := &app{
		connected: true, lcu: &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}, storage: trackTestStore(t, &localStore{root: root}),
		liveClientPlayerList: func(context.Context) ([]byte, int, error) { return liveRaw, http.StatusOK, nil },
	}
	recorder := httptest.NewRecorder()
	a.handleGameplayLive(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/live", nil))
	var response gameplayLiveResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	for index, player := range response.Players {
		if player.Position != strings.ToLower(positions[index]) {
			t.Fatalf("enriched position[%d] = %q, want %q", index, player.Position, strings.ToLower(positions[index]))
		}
	}
	diagnostics, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	text := string(diagnostics)
	if !strings.Contains(text, `"position_matched_count":10`) || !strings.Contains(text, `"riotId":10`) || !strings.Contains(text, `"summonerName":0`) {
		t.Fatalf("enriched match diagnostics = %s", text)
	}
	for _, field := range []string{`"event":"live_load_cost"`, `"players":10`, `"concurrency":6`, `"ranks_ms":`, `"matches_ms":`, `"total_ms":`} {
		if !strings.Contains(text, field) {
			t.Fatalf("live load cost diagnostic missing %s: %s", field, text)
		}
	}
	for _, name := range renderNames {
		if strings.Contains(text, name) {
			t.Fatalf("diagnostic leaked Riot ID %q: %s", name, text)
		}
	}
}

func TestR69MeasureLiveRosterConcurrency(t *testing.T) {
	players := make([]map[string]any, 10)
	for index := range players {
		players[index] = map[string]any{"puuid": fmt.Sprintf("r69-cost-player-%02d", index), "summonerName": "", "championId": 0}
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/lol-gameflow/v1/gameflow-phase":
			_, _ = w.Write([]byte(`"InProgress"`))
		case request.URL.Path == "/lol-gameflow/v1/session":
			_ = json.NewEncoder(w).Encode(map[string]any{"gameData": map[string]any{"gameId": int64(6904), "queue": map[string]any{"id": 420, "mapId": 11, "gameMode": "CLASSIC"}, "teamOne": players[:5], "teamTwo": players[5:]}})
		case strings.HasPrefix(request.URL.Path, "/lol-summoner/v2/summoners/puuid/"):
			requests.Add(1)
			time.Sleep(40 * time.Millisecond)
			playerRef, _ := url.PathUnescape(strings.TrimPrefix(request.URL.Path, "/lol-summoner/v2/summoners/puuid/"))
			_ = json.NewEncoder(w).Encode(Summoner{PUUID: playerRef, GameName: "Cost", TagLine: "CN1"})
		case strings.HasPrefix(request.URL.Path, "/lol-ranked/v1/ranked-stats/"):
			requests.Add(1)
			time.Sleep(40 * time.Millisecond)
			_, _ = w.Write([]byte(`{"queues":[]}`))
		case strings.HasPrefix(request.URL.Path, "/lol-match-history/v1/products/lol/"):
			requests.Add(1)
			time.Sleep(40 * time.Millisecond)
			_, _ = w.Write([]byte(`{"games":{"gameCount":0,"games":[]}}`))
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()
	for _, concurrency := range []int{4, 6, 8} {
		a := &app{
			connected: true,
			lcu:       &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()},
			liveClientPlayerList: func(context.Context) ([]byte, int, error) {
				return nil, 0, errors.New("measurement fixture has no Live Client Data")
			},
			liveRosterConcurrencyOverride: concurrency,
		}
		requests.Store(0)
		started := time.Now()
		recorder := httptest.NewRecorder()
		a.handleGameplayLive(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/live", nil))
		coldDuration, coldRequests := time.Since(started), requests.Load()
		requests.Store(0)
		started = time.Now()
		recorder = httptest.NewRecorder()
		a.handleGameplayLive(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/live", nil))
		warmDuration, warmRequests := time.Since(started), requests.Load()
		if recorder.Code != http.StatusOK || coldRequests != 30 || warmRequests != 0 {
			t.Fatalf("concurrency=%d status=%d cold_requests=%d warm_requests=%d", concurrency, recorder.Code, coldRequests, warmRequests)
		}
		t.Logf("live concurrency=%d cold_ms=%d cold_lcu_requests=%d cold_req_per_s=%.1f warm_ms=%d warm_lcu_requests=%d warm_req_per_s=%.1f", concurrency, coldDuration.Milliseconds(), coldRequests, float64(coldRequests)/coldDuration.Seconds(), warmDuration.Milliseconds(), warmRequests, float64(warmRequests)/warmDuration.Seconds())
	}
}

func TestR66LivePlayerMatchesCachePreventsRepeatedLCUReads(t *testing.T) {
	var requests atomic.Int32
	var history lcuMatchHistory
	history.Games.Games = []lcuGame{{GameID: 701, QueueID: 420, GameMode: "CLASSIC"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/lol-match-history/v1/products/lol/cache-player/matches" {
			http.NotFound(w, request)
			return
		}
		requests.Add(1)
		_ = json.NewEncoder(w).Encode(history)
	}))
	t.Cleanup(server.Close)
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{}
	reference := gameplayReference{PlayerRef: "cache-player"}
	first := a.livePlayerMatches(context.Background(), client, reference, "cache-player", false, nil)
	second := a.livePlayerMatches(context.Background(), client, reference, "cache-player", false, nil)
	if len(first.Matches) != 1 || len(second.Matches) != 1 || requests.Load() != 1 {
		t.Fatalf("cache calls=%d first=%#v second=%#v", requests.Load(), first, second)
	}
	first.Matches[0].GameID = 999
	if second.Matches[0].GameID != 701 {
		t.Fatalf("cache returned the shared slice: %#v", second)
	}

	key := "cache-player\x00false"
	a.livePlayerMatchesMu.Lock()
	stale := a.livePlayerMatchCache[key]
	stale.FetchedAt = time.Now().Add(-livePlayerMatchesCacheTTL - time.Second)
	a.livePlayerMatchCache[key] = stale
	a.livePlayerMatchesMu.Unlock()
	a.livePlayerMatches(context.Background(), client, reference, "cache-player", false, nil)
	if requests.Load() != 2 {
		t.Fatalf("expired cache did not reload: calls=%d", requests.Load())
	}
}

func TestGameplayLiveUnsupportedModeUsesSemanticTFTField(t *testing.T) {
	for _, test := range []struct {
		gameMode    string
		unsupported bool
	}{{"TFT", true}, {" tft ", true}, {"CLASSIC", false}, {"CHERRY", false}} {
		reason, unsupported := gameplayLiveUnsupportedReason(test.gameMode)
		if unsupported != test.unsupported {
			t.Fatalf("game mode %q unsupported=%v, want %v", test.gameMode, unsupported, test.unsupported)
		}
		if unsupported && reason != "暂时不支持此模式，敬请期待" {
			t.Fatalf("game mode %q reason=%q", test.gameMode, reason)
		}
	}
}

func TestGameplayLiveReturnsExplicitTFTUnsupportedState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-gameflow/v1/gameflow-phase":
			_, _ = w.Write([]byte(`"InProgress"`))
		case "/lol-gameflow/v1/session":
			_, _ = w.Write([]byte(`{"gameData":{"gameId":123,"queue":{"id":1100,"name":"TFT","gameMode":"TFT","mapId":22},"teamOne":[{"puuid":"should-not-be-enriched"}]},"map":{"id":22,"gameMode":"TFT"}}`))
		default:
			t.Fatalf("TFT unsupported response requested unexpected endpoint %q", r.URL.Path)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{connected: true, lcu: client}
	recorder := httptest.NewRecorder()
	a.handleGameplayLive(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/live", nil))
	var response gameplayLiveResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Available || !response.Unsupported || response.UnsupportedReason != "暂时不支持此模式，敬请期待" || response.GameMode != "TFT" || len(response.Players) != 0 {
		t.Fatalf("TFT live response = %#v", response)
	}
}

func TestGameplayRecommendationBundleEncodesEmptyAugmentsAsArray(t *testing.T) {
	bundle := gameplayRecommendationsFromChampionDetail(64, "mid", championDetailResponse{Mode: "aram"})
	data, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"augments":[]`)) || bytes.Contains(data, []byte(`"augments":null`)) {
		t.Fatalf("empty augment contract = %s", data)
	}
}

func TestRecommendationModeDiagnosticIsRecordedOncePerGame(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	a := &app{storage: trackTestStore(t, &localStore{root: root})}
	resolution := resolveGameplayRecommendationMode(-1, "CLASSIC", 11)
	a.recordRecommendationModeResolution(987, -1, "CLASSIC", 11, resolution)
	a.recordRecommendationModeResolution(987, -1, "CLASSIC", 11, resolution)
	data, err := a.storage.readDiagnosticLog()
	if err != nil || strings.Count(string(data), `"event":"recommendation_mode_resolved"`) != 1 || !strings.Contains(string(data), `"queue_id":-1`) || !strings.Contains(string(data), `"has_top_players":true`) {
		t.Fatalf("recommendation mode diagnostic = %s err=%v", data, err)
	}
}

func TestUnknownQueueDiagnosticIsRecordedOncePerQueueID(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	a := &app{storage: trackTestStore(t, &localStore{root: root})}
	a.recordUnknownQueue(9999, "UNKNOWN", 99)
	a.recordUnknownQueue(9999, "CHANGED", 11)
	a.recordUnknownQueue(3270, "KIWI", 12)
	data, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), `"event":"unknown_queue_observed"`) != 1 || !strings.Contains(string(data), `"queue_id":9999`) || !strings.Contains(string(data), `"game_mode":"UNKNOWN"`) || !strings.Contains(string(data), `"map_id":99`) {
		t.Fatalf("unknown queue diagnostic = %s", data)
	}
}

func TestRecommendationRequestDiagnosticRecordsInvalidArrival(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	a := &app{storage: trackTestStore(t, &localStore{root: root})}
	recorder := httptest.NewRecorder()
	a.handleGameplayRecommendations(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/recommendations?championId=0&queueId=1750&position=TOP", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	data, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	events := map[string]map[string]any{}
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		var event map[string]any
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatalf("decode diagnostic %q: %v", line, err)
		}
		if name, _ := event["event"].(string); name != "" {
			events[name] = event
		}
	}
	requestEvent := events["live_recommendations_request"]
	responseEvent := events["live_recommendations_response"]
	requestTrace, _ := requestEvent["trace_id"].(string)
	responseTrace, _ := responseEvent["trace_id"].(string)
	if requestTrace == "" || responseTrace != requestTrace || requestEvent["queue_id"] != float64(1750) || requestEvent["raw_position"] != "TOP" {
		t.Fatalf("request/response trace mismatch: request=%#v response=%#v", requestEvent, responseEvent)
	}
	if responseEvent["diagnostic_schema"] != float64(2) || responseEvent["status"] != "failed" || responseEvent["stage"] != "champion-validation" || responseEvent["requested_position"] != "top" || responseEvent["queue_id"] != float64(1750) {
		t.Fatalf("failed response diagnostic = %#v", responseEvent)
	}
}

func TestDiagnosticDeduplicationResetsAfterLogRotation(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	a := &app{storage: store}
	store.onDiagnosticRotation = a.resetDiagnosticDeduplication
	resolution := resolveGameplayRecommendationMode(420, "CLASSIC", 11)
	a.recordRecommendationModeResolution(99, 420, "CLASSIC", 11, resolution)
	path := filepath.Join(root, "logs", "diagnostics.jsonl")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 2*1024*1024+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.appendDiagnostic(map[string]any{"event": "rotation-trigger"}); err != nil {
		t.Fatal(err)
	}
	a.recordRecommendationModeResolution(99, 420, "CLASSIC", 11, resolution)
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), `"event":"recommendation_mode_resolved"`) != 1 {
		t.Fatalf("post-rotation recommendation count = %d, bytes = %d", strings.Count(string(data), `"event":"recommendation_mode_resolved"`), len(data))
	}
}

func TestDiagnosticRotationCallbackRunsOutsideGeneralStorageMutex(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	callbackBlocked := false
	store.onDiagnosticRotation = func() {
		if !store.mu.TryLock() {
			callbackBlocked = true
			return
		}
		store.mu.Unlock()
	}
	path := filepath.Join(root, "logs", "diagnostics.jsonl")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 2*1024*1024+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.appendDiagnostic(map[string]any{"event": "rotation-trigger"}); err != nil {
		t.Fatal(err)
	}
	if callbackBlocked {
		t.Fatal("diagnostic rotation callback ran while the general storage mutex was held")
	}
}

func TestRankedWinrateDiagnosticsAggregateByMinuteAndSource(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	a := &app{storage: store}
	for index := 0; index < 1000; index++ {
		a.recordDiagnostic(map[string]any{
			"event": "ranked_winrate_resolved", "source": "lcu", "complete": index%2 == 0,
			"suppressed": index%2 != 0, "player_ref_hash": fmt.Sprintf("player-%d", index),
		})
	}
	a.flushRankedWinrateDiagnostics()
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), `"event":"ranked_winrate_resolved"`) != 1 || !strings.Contains(string(data), `"count":1000`) || !strings.Contains(string(data), `"complete_count":500`) || !strings.Contains(string(data), `"suppressed_count":500`) || strings.Contains(string(data), "player_ref_hash") {
		t.Fatalf("ranked win-rate aggregation = %s", data)
	}
}

func TestRankedWinrateDiagnosticAggregationKeepsSourceDimensionBounded(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	a := &app{storage: trackTestStore(t, &localStore{root: root})}
	for index := 0; index < diagnosticDeduplicationLimit+73; index++ {
		source := "lcu"
		if index%2 == 0 {
			source = "sgp"
		}
		a.recordDiagnostic(map[string]any{"event": "ranked_winrate_resolved", "source": source})
	}
	a.rankedWinrateDiagnosticMu.Lock()
	size := len(a.rankedWinrateDiagnosticBuckets)
	a.rankedWinrateDiagnosticMu.Unlock()
	if size != 2 {
		t.Fatalf("ranked win-rate aggregation sources = %d, want 2", size)
	}
}

func TestDiagnosticEventShareStaysBelowThirtyPercent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	a := &app{storage: store}
	noisy := map[string]any{"event": "ranked_winrate_resolved", "source": "lcu", "queue": "RANKED_SOLO_5x5", "wins": 0, "losses": 0}
	for range 100 {
		a.recordDiagnostic(noisy)
	}
	a.flushRankedWinrateDiagnostics()
	for index := 0; index < 10; index++ {
		a.recordDiagnostic(map[string]any{"event": fmt.Sprintf("diagnostic_share_probe_%d", index)})
	}
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	counts := make(map[string]int)
	total := 0
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var record struct {
			Event string `json:"event"`
		}
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("diagnostic line is not JSON: %v", err)
		}
		counts[record.Event]++
		total++
	}
	if total == 0 {
		t.Fatal("diagnostic share fixture is empty")
	}
	for event, count := range counts {
		if count*100 > total*30 {
			t.Fatalf("diagnostic event %q occupies %d/%d records, over 30%%", event, count, total)
		}
	}
}

func TestDiagnosticEventShareAggregatesDistinctRankedPayloadsBelowThirtyPercent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	a := &app{storage: store}
	// These payloads intentionally differ on every call, so H-1's exact-byte
	// deduplication cannot hide an event-level share-limit blind spot.
	for index := 0; index < 200; index++ {
		a.recordDiagnostic(map[string]any{
			"event": "ranked_winrate_resolved", "source": "lcu", "player_index": index,
		})
	}
	for index := 0; index < 10; index++ {
		a.recordDiagnostic(map[string]any{"event": fmt.Sprintf("diagnostic_distinct_share_probe_%d", index)})
	}
	a.flushRankedWinrateDiagnostics()
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	counts := make(map[string]int)
	total := 0
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var record struct {
			Event string `json:"event"`
		}
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("distinct-payload diagnostic line is not JSON: %v", err)
		}
		counts[record.Event]++
		total++
	}
	if total != 11 || counts["ranked_winrate_resolved"] != 1 {
		t.Fatalf("distinct-payload diagnostic counts = %#v total=%d", counts, total)
	}
	overLimit := counts["ranked_winrate_resolved"]*100 > total*30
	if overLimit {
		t.Fatalf("aggregated ranked diagnostic share exceeded 30%%: %d/%d", counts["ranked_winrate_resolved"], total)
	}
}

func TestNoisyDiagnosticDeduplicationResetsAfterLogRotation(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	a := &app{storage: store}
	store.onDiagnosticRotation = a.resetDiagnosticDeduplication
	payload := map[string]any{"event": "ranked_data_source_decision", "selected": "lcu", "reason": "local-client-connected"}
	a.recordDiagnostic(payload)
	store.diagnosticMu.Lock()
	if err := store.releaseDiagnosticLocked(); err != nil {
		t.Fatal(err)
	}
	store.diagnosticMu.Unlock()
	path := filepath.Join(root, "logs", "diagnostics.jsonl")
	prefix := []byte(`{"event":"ranked_data_source_decision","selected":"lcu","reason":"local-client-connected"}` + "\n")
	if err := os.WriteFile(path, append(prefix, bytes.Repeat([]byte("x"), 2*1024*1024+1)...), 0o600); err != nil {
		t.Fatal(err)
	}
	a.recordDiagnostic(map[string]any{"event": "rotation-trigger"})
	a.recordDiagnostic(payload)
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), `"event":"ranked_data_source_decision"`) != 1 {
		t.Fatalf("post-rotation noisy diagnostic count = %d, data = %s", strings.Count(string(data), `"event":"ranked_data_source_decision"`), data)
	}
	backup, err := os.ReadFile(filepath.Join(root, "logs", "diagnostics.1.jsonl"))
	if err != nil || !strings.Contains(string(backup), `"event":"ranked_data_source_decision"`) {
		t.Fatalf("rotated noisy diagnostic missing from backup: err=%v data=%s", err, backup)
	}
}

func TestLiveRosterShapeDiagnosticDeduplicatesByFingerprint(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	a := &app{storage: store}
	response := gameplayLiveResponse{Phase: "ChampSelect", GameID: 123, RawCount: 4, MergeAppended: 1, Players: []gameplayLivePlayer{
		{TeamID: 100, gameplayPlayer: gameplayPlayer{PlayerRef: "one"}, ModeStats: gameplayAggregate{Games: 2}},
		{TeamID: 200, gameplayPlayer: gameplayPlayer{PlayerRef: "two"}, Rank: &gameplayRank{Tier: "gold"}},
		{TeamID: 200, gameplayPlayer: gameplayPlayer{PlayerRef: "two"}},
		{TeamID: 200, ChampionID: 64},
	}}
	a.recordLiveRosterShape(response)
	a.recordLiveRosterShape(response)
	intentOnly := response
	intentOnly.Players = append([]gameplayLivePlayer(nil), response.Players...)
	intentOnly.Players[3].ChampionPickIntent = 103
	a.recordLiveRosterShape(intentOnly)
	changed := response
	changed.Players = append([]gameplayLivePlayer(nil), response.Players...)
	changed.Players[3].ChampionID = 103
	a.recordLiveRosterShape(changed)
	progress := changed
	progress.Players = append([]gameplayLivePlayer(nil), changed.Players...)
	progress.Players[2].Rank = &gameplayRank{Tier: "silver"}
	a.recordLiveRosterShape(progress)
	complete := progress
	complete.Players = append([]gameplayLivePlayer(nil), progress.Players...)
	complete.Players[3].Rank = &gameplayRank{Tier: "bronze"}
	a.recordLiveRosterShape(complete)
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), `"event":"live_roster_shape"`) != 4 || !strings.Contains(string(data), `"players":4`) || !strings.Contains(string(data), `"raw_count":4`) || !strings.Contains(string(data), `"with_stats":2`) || !strings.Contains(string(data), `"with_stats":3`) || !strings.Contains(string(data), `"with_stats":4`) || !strings.Contains(string(data), `"duplicate_player_refs":1`) || !strings.Contains(string(data), `"empty_ref_count":1`) || !strings.Contains(string(data), `"merge_appended":1`) {
		t.Fatalf("roster diagnostic = %s", data)
	}
}

func TestR68LiveRosterShapeRecordsStatsProgressFiveSevenTen(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	a := &app{storage: trackTestStore(t, &localStore{root: root})}
	base := gameplayLiveResponse{Phase: "InProgress", GameID: 6810, Players: make([]gameplayLivePlayer, 10)}
	for index := range base.Players {
		base.Players[index].TeamID = []int64{100, 200}[index/5]
		base.Players[index].ChampionID = int64(index + 1)
		base.Players[index].gameplayPlayer.PlayerRef = fmt.Sprintf("player_%02d", index)
	}
	for _, withStats := range []int{5, 7, 10} {
		frame := base
		frame.Players = append([]gameplayLivePlayer(nil), base.Players...)
		for index := 0; index < withStats; index++ {
			frame.Players[index].Rank = &gameplayRank{Tier: "gold"}
		}
		a.recordLiveRosterShape(frame)
	}
	data, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Count(text, `"event":"live_roster_shape"`) != 3 || !strings.Contains(text, `"with_stats":5`) || !strings.Contains(text, `"with_stats":7`) || !strings.Contains(text, `"with_stats":10`) {
		t.Fatalf("stats progress diagnostic = %s", text)
	}
}

func TestLCUSessionShapePayloadsContainOnlyStructuralStatistics(t *testing.T) {
	gameflowRaw := []byte(`{
		"phase":"ChampSelect",
		"gameData":{
			"teamOne":[
				{"puuid":"gameflow-secret-one","summonerName":"Alice","teamParticipantId":100},
				{"puuid":"","summonerName":"","teamParticipantId":100}
			],
			"teamTwo":[{"puuid":"gameflow-secret-two","teamParticipantId":200}]
		}
	}`)
	gameflow := lcuGameflowSessionShapePayload(gameflowRaw, "ChampSelect", "cherry", 1750)
	if gameflow["phase"] != "ChampSelect" || gameflow["game_mode"] != "CHERRY" || gameflow["queue_id"] != int64(1750) {
		t.Fatalf("gameflow context = %#v", gameflow)
	}
	if got := strings.Join(gameflow["team_one_player_keys"].([]string), ","); got != "puuid,summonerName,teamParticipantId" {
		t.Fatalf("gameflow team keys = %q", got)
	}
	if counts := gameflow["team_one_team_participant_id_counts"].(map[string]int); counts["100"] != 2 || len(counts) != 1 {
		t.Fatalf("gameflow team-one participant distribution = %#v", counts)
	}
	if counts := gameflow["team_two_team_participant_id_counts"].(map[string]int); counts["200"] != 1 || len(counts) != 1 {
		t.Fatalf("gameflow team-two participant distribution = %#v", counts)
	}

	champSelectRaw := []byte(`{
		"actions":[
			[{"actorCellId":0,"championId":64,"completed":true,"type":"pick","puuid":"action-secret-one"}],
			[{"actorCellId":1,"championId":0,"completed":false,"type":"ban","obfuscatedPuuid":"action-secret-two"}]
		],
		"benchChampions":[17,{"championId":64}],
		"localPlayerCellId":0,
		"myTeam":[
			{"cellId":0,"championId":64,"puuid":"champ-secret-one","gameName":"","team":100,"nameVisibilityType":"VISIBLE","obfuscatedPuuid":"obfuscated-private-one","ready":true,"meta":{},"actions":[]},
			{"cellId":1,"championId":0,"puuid":"","gameName":"Private Alice","team":100,"nameVisibilityType":"HIDDEN","obfuscatedPuuid":"","ready":false,"nullable":null,"meta":{"present":true},"actions":[1]}
		],
		"theirTeam":[{"cellId":5,"championId":0,"puuid":null}]
	}`)
	champSelect := lcuChampSelectSessionShapePayload(champSelectRaw, "ChampSelect", "CHERRY", 1750)
	if champSelect["my_team_length"] != 2 || champSelect["their_team_length"] != 1 {
		t.Fatalf("champ-select lengths = %#v", champSelect)
	}
	if got := strings.Join(champSelect["top_level_keys"].([]string), ","); got != "actions,benchChampions,localPlayerCellId,myTeam,theirTeam" {
		t.Fatalf("champ-select top-level keys = %q", got)
	}
	if champSelect["champion_progress_bucket"] != "1-2" || champSelect["actions_group_count"] != 2 || champSelect["actions_flat_count"] != 2 {
		t.Fatalf("champ-select action shape counts = %#v", champSelect)
	}
	if got := strings.Join(champSelect["actions_element_keys"].([]string), ","); got != "actorCellId,championId,completed,obfuscatedPuuid,puuid,type" {
		t.Fatalf("champ-select action keys = %q", got)
	}
	if counts := champSelect["actions_type_counts"].(map[string]int); counts["PICK"] != 1 || counts["BAN"] != 1 {
		t.Fatalf("champ-select action types = %#v", counts)
	}
	if counts := champSelect["actions_actor_cell_id_counts"].(map[string]int); counts["0"] != 1 || counts["1"] != 1 {
		t.Fatalf("champ-select action actor cells = %#v", counts)
	}
	if counts := champSelect["actions_champion_id_counts"].(map[string]int); counts["64"] != 1 || counts["0"] != 1 {
		t.Fatalf("champ-select action champions = %#v", counts)
	}
	if counts := champSelect["actions_completed_counts"].(map[string]int); counts["boolean"] != 2 {
		t.Fatalf("champ-select action completion shapes = %#v", counts)
	}
	if counts := champSelect["my_team_team_counts"].(map[string]int); counts["100"] != 2 || len(counts) != 1 {
		t.Fatalf("champ-select team distribution = %#v", counts)
	}
	if counts := champSelect["my_team_name_visibility_type_counts"].(map[string]int); counts["VISIBLE"] != 1 || counts["HIDDEN"] != 1 {
		t.Fatalf("champ-select visibility distribution = %#v", counts)
	}
	if shapes := champSelect["my_team_obfuscated_puuid_shapes"].(map[string]int); shapes["string_nonempty_length_22"] != 1 || shapes["string_empty"] != 1 {
		t.Fatalf("champ-select obfuscated PUUID shapes = %#v", shapes)
	}
	if champSelect["bench_champions_length"] != 2 {
		t.Fatalf("bench champion length = %#v", champSelect["bench_champions_length"])
	}
	if shapes := champSelect["bench_champions_element_shapes"].(map[string]int); shapes["number"] != 1 || shapes["object{championId}"] != 1 {
		t.Fatalf("bench champion shapes = %#v", shapes)
	}
	myCounts := champSelect["my_team_nonzero_counts"].(map[string]int)
	for key, want := range map[string]int{"cellId": 1, "championId": 1, "puuid": 1, "gameName": 1, "ready": 1, "meta": 1, "actions": 1} {
		if myCounts[key] != want {
			t.Fatalf("champ-select nonzero count %s = %d, want %d: %#v", key, myCounts[key], want, myCounts)
		}
	}
	for _, key := range []string{"nullable"} {
		if myCounts[key] != 0 {
			t.Fatalf("zero champ-select key %s was counted: %#v", key, myCounts)
		}
	}

	encodedGameflow, _ := json.Marshal(gameflow)
	encodedChampSelect, _ := json.Marshal(champSelect)
	encoded := string(append(encodedGameflow, encodedChampSelect...))
	for _, secret := range []string{"gameflow-secret-one", "gameflow-secret-two", "champ-secret-one", "obfuscated-private-one", "action-secret-one", "action-secret-two", "Alice", "Private Alice"} {
		if strings.Contains(encoded, secret) {
			t.Fatalf("shape diagnostic leaked private value %q: %s", secret, encoded)
		}
	}
}

func TestLCUSessionShapeDiagnosticsDeduplicateByModeAndRotate(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	a := &app{storage: store}
	gameflowRaw := []byte(`{"gameData":{"teamOne":[],"teamTwo":[]}}`)
	champSelectRaw := []byte(`{"myTeam":[],"theirTeam":[]}`)
	cherryOne := []byte(`{"myTeam":[{"championId":64}],"theirTeam":[]}`)
	cherryThree := []byte(`{"myTeam":[{"championId":64},{"championId":103},{"championId":222}],"theirTeam":[]}`)

	a.recordLCUGameflowSessionShape(gameflowRaw, "ChampSelect", "KIWI", 2400)
	a.recordLCUGameflowSessionShape(gameflowRaw, "ChampSelect", "KIWI", 2400)
	a.recordLCUGameflowSessionShape(gameflowRaw, "ChampSelect", "CHERRY", 1750)
	a.recordLCUChampSelectSessionShape(champSelectRaw, "ChampSelect", "KIWI", 2400)
	a.recordLCUChampSelectSessionShape(champSelectRaw, "ChampSelect", "KIWI", 2400)
	a.recordLCUChampSelectSessionShape(champSelectRaw, "ChampSelect", "CHERRY", 1750)
	a.recordLCUChampSelectSessionShape(cherryOne, "ChampSelect", "CHERRY", 1750)
	a.recordLCUChampSelectSessionShape(cherryThree, "ChampSelect", "CHERRY", 1750)
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), `"event":"lcu_gameflow_session_shape"`) != 2 || strings.Count(string(data), `"event":"lcu_champ_select_session_shape"`) != 4 {
		t.Fatalf("mode-scoped shape diagnostics = %s", data)
	}
	for _, bucket := range []string{`"champion_progress_bucket":"0"`, `"champion_progress_bucket":"1-2"`, `"champion_progress_bucket":"3"`} {
		if !strings.Contains(string(data), bucket) {
			t.Fatalf("missing CHERRY progress bucket %s: %s", bucket, data)
		}
	}
	if !strings.Contains(string(data), `"game_mode":"KIWI"`) || !strings.Contains(string(data), `"game_mode":"CHERRY"`) || !strings.Contains(string(data), `"queue_id":1750`) {
		t.Fatalf("shape diagnostic context missing: %s", data)
	}

	a.lcuGameflowShapeDiagnosticKeys = make(map[string]struct{}, lcuSessionShapeDiagnosticLimit)
	a.lcuChampSelectShapeDiagnosticKeys = make(map[string]struct{}, lcuSessionShapeDiagnosticLimit)
	for index := 0; index < lcuSessionShapeDiagnosticLimit; index++ {
		key := fmt.Sprintf("old-%d", index)
		a.lcuGameflowShapeDiagnosticKeys[key] = struct{}{}
		a.lcuChampSelectShapeDiagnosticKeys[key] = struct{}{}
	}
	a.recordLCUGameflowSessionShape(gameflowRaw, "InProgress", "CLASSIC", 420)
	a.recordLCUChampSelectSessionShape(champSelectRaw, "ChampSelect", "CLASSIC", 420)
	if len(a.lcuGameflowShapeDiagnosticKeys) != 1 || len(a.lcuChampSelectShapeDiagnosticKeys) != 1 {
		t.Fatalf("shape diagnostic maps did not rotate: gameflow=%d champ-select=%d", len(a.lcuGameflowShapeDiagnosticKeys), len(a.lcuChampSelectShapeDiagnosticKeys))
	}
}

func TestGameplayQueueModeClassificationMatrix(t *testing.T) {
	tests := []struct {
		name            string
		queueID, mapID  int64
		gameMode, label string
		wantGroup       string
	}{
		{"ordinary hextech 2300", 2300, 12, "KIWI", "海克斯大乱斗", "hextech-aram"},
		{"ordinary hextech 2400", 2400, 12, "KIWI", "海克斯大乱斗", "hextech-aram"},
		{"qualifier by Chinese label", 2600, 12, "KIWI", "海克斯大乱斗 海选赛", "hextech-qualifier"},
		{"classic by game mode", 2600, 12, "ARAM_MAYHEM_CLASSIC", "海克斯大乱斗", "hextech-classic"},
		{"classic by label", 2600, 12, "KIWI", "海克斯大乱斗 经典模式版", "hextech-classic"},
		{"arena 1700", 1700, 30, "CHERRY", "斗魂竞技场", "arena"},
		{"arena 1710", 1710, 30, "CHERRY", "斗魂竞技场", "arena"},
		{"aram 450", 450, 12, "ARAM", "极地大乱斗", "aram"},
		{"swiftplay 480", 480, 11, "CLASSIC", "快速模式", "match"},
		{"aram 930", 930, 12, "ARAM", "极地大乱斗", "aram"},
		{"match 400", 400, 11, "CLASSIC", "匹配模式", "match"},
		{"match 430", 430, 11, "CLASSIC", "匹配模式", "match"},
		{"match 490", 490, 11, "CLASSIC", "匹配模式", "match"},
		{"urf 1900", 1900, 11, "ARURF", "无限火力", "urf"},
		{"nexus blitz", 1300, 21, "NEXUSBLITZ", "极限闪击", "nexus-blitz"},
		{"bots", 850, 11, "BOT", "人机对战", "bots"},
		{"clash", 700, 11, "CLASSIC", "冠军杯赛", "clash"},
		{"doombots", 950, 12, "DOOMBOTS", "末日人工智能", "doombots"},
		{"unknown special", 1400, 22, "SPECIAL", "特殊模式", "other"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := queueModeGroupForLabel(test.queueID, test.gameMode, test.mapID, test.label); got != test.wantGroup {
				t.Fatalf("group = %q, want %q", got, test.wantGroup)
			}
		})
	}

	for _, test := range []struct {
		queueID int64
		want    string
	}{{480, "快速模式"}, {930, "极地大乱斗冠军杯赛"}, {1900, "无限火力"}, {490, "匹配模式（快速）"}, {1300, "极限闪击"}, {2400, "海克斯大乱斗"}} {
		if got := queueLabel(test.queueID, "", nil); got != test.want {
			t.Errorf("queueLabel(%d) = %q, want %q", test.queueID, got, test.want)
		}
	}
}

func TestGameplayRecommendationStatsDistinguishZeroFromMissing(t *testing.T) {
	encodedZero, err := json.Marshal(recommendationStats(0, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	encodedMissing, err := json.Marshal(gameplayRecommendationStats{})
	if err != nil {
		t.Fatal(err)
	}
	if string(encodedZero) != `{"pickRate":null,"winRate":null,"games":null}` {
		t.Fatalf("zero stats should be missing JSON = %s", encodedZero)
	}
	if string(encodedMissing) != `{"pickRate":null,"winRate":null,"games":null}` {
		t.Fatalf("missing stats JSON = %s", encodedMissing)
	}
}

func TestRecentRankedRecordDoesNotOverwriteSeasonRank(t *testing.T) {
	rank := gameplayRank{Wins: 80, Losses: 40, WinRate: 67}
	record := recentRankedRecord([]gameplayRecentGame{{Win: true}, {Win: false}, {Win: true}})
	if record == nil || record.Games != 3 || record.Wins != 2 || record.Losses != 1 {
		t.Fatalf("recent ranked record = %#v", record)
	}
	if rank.Wins != 80 || rank.Losses != 40 || rank.WinRate != 67 {
		t.Fatalf("season rank was polluted by recent matches: %#v", rank)
	}
}

func TestItemSetApplyPreservesUserSetsAndStoresIdempotently(t *testing.T) {
	playerRef := strings.Repeat("p", 48)
	document := lcuItemSetDocument{
		"accountId":   json.RawMessage(`456`),
		"timestamp":   json.RawMessage(`123456`),
		"futureField": json.RawMessage(`{"preserved":true}`),
		"itemSets":    json.RawMessage(`[{"uid":"user-owned","title":"我的装备","type":"custom","map":"any","mode":"any","associatedChampions":[],"associatedMaps":[],"blocks":[],"preferredItemSlots":[],"unknown":"keep-me"}]`),
	}
	putCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /lol-gameflow/v1/gameflow-phase":
			_, _ = w.Write([]byte(`"ChampSelect"`))
		case "GET /lol-champ-select/v1/session":
			_, _ = w.Write([]byte(`{"myTeam":[{"summonerId":123,"puuid":"` + playerRef + `","championId":64}]}`))
		case "GET /lol-game-data/assets/v1/items.json":
			_, _ = w.Write([]byte(`[{"id":1055,"priceTotal":450},{"id":3071,"priceTotal":3000},{"id":6630,"priceTotal":3300}]`))
		case "GET /lol-item-sets/v1/item-sets/123/sets":
			_ = json.NewEncoder(w).Encode(document)
		case "PUT /lol-item-sets/v1/item-sets/123/sets":
			putCount++
			var updated lcuItemSetDocument
			if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
				t.Fatalf("decode item set PUT: %v", err)
			}
			document = updated
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	provider := newChampionProvider()
	provider.itemPurchasable[9999] = false
	a := &app{connected: true, lcu: client, summoner: Summoner{SummonerID: 123, AccountID: 456, PUUID: playerRef}, storage: trackTestStore(t, &localStore{root: root}), champions: provider}
	body := `{"title":"李青 · 打野","championId":64,"mapId":11,"position":"jungle","selfPosition":"middle","blocks":[{"type":"出门装","items":[{"id":1055,"count":1}]},{"type":"出装路线","items":[{"id":6630,"count":1},{"id":3071,"count":1},{"id":9999,"count":1}]}]}`
	for attempt := 0; attempt < 2; attempt++ {
		recorder := httptest.NewRecorder()
		a.handleGameplayItemSetApply(recorder, httptest.NewRequest(http.MethodPost, "/api/gameplay/item-sets/apply", strings.NewReader(body)))
		if recorder.Code != http.StatusOK {
			t.Fatalf("attempt %d status = %d: %s", attempt, recorder.Code, recorder.Body.String())
		}
		var response struct {
			Applied bool `json:"applied"`
			Stored  bool `json:"stored"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || !response.Applied || !response.Stored {
			t.Fatalf("attempt %d response = %#v, %v", attempt, response, err)
		}
		if strings.Contains(recorder.Body.String(), `"verified"`) {
			t.Fatalf("attempt %d response overstates round-trip semantics: %s", attempt, recorder.Body.String())
		}
	}
	if putCount != 2 {
		t.Fatalf("PUT count = %d, want 2", putCount)
	}
	if string(document["futureField"]) != `{"preserved":true}` {
		t.Fatalf("top-level unknown field changed: %s", document["futureField"])
	}
	var sets []json.RawMessage
	if err := json.Unmarshal(document["itemSets"], &sets); err != nil || len(sets) != 2 {
		t.Fatalf("item sets = %d, %v", len(sets), err)
	}
	var userSet map[string]json.RawMessage
	if err := json.Unmarshal(sets[0], &userSet); err != nil || string(userSet["unknown"]) != `"keep-me"` {
		t.Fatalf("user set was not preserved: %#v, %v", userSet, err)
	}
	created, found, err := findLCUItemSet(document, itemSetUID(64, "jungle"))
	if err != nil || !found || created.Title != "DL · 打野" || created.StartedFrom != "DL" || created.SortRank != 100 || len(created.Blocks) != 2 || len(created.AssociatedChampions) != 1 || created.AssociatedChampions[0] != 64 {
		t.Fatalf("created item set = %#v, %v, %v", created, found, err)
	}
	if got := []string{created.Blocks[1].Items[0].ID, created.Blocks[1].Items[1].ID}; !reflect.DeepEqual(got, []string{"3071", "6630"}) {
		t.Fatalf("item set price order = %v", got)
	}
	data, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	var diagnostic map[string]any
	if err := json.Unmarshal(lines[len(lines)-1], &diagnostic); err != nil {
		t.Fatal(err)
	}
	if diagnostic["event"] != "item_set_apply" || diagnostic["stored"] != true || diagnostic["position"] != "jungle" || diagnostic["self_position"] != "middle" || diagnostic["block_count"] != float64(2) || diagnostic["item_count"] != float64(4) || diagnostic["normalized_count"] != float64(0) || diagnostic["unpurchasable_count"] != float64(1) {
		t.Fatalf("item-set diagnostic = %#v", diagnostic)
	}
	if diagnostic["requested_item_count"] != float64(4) || diagnostic["removed_stale_count"] != float64(0) {
		t.Fatalf("item-set count diagnostics = %#v", diagnostic)
	}
	blocks, ok := diagnostic["blocks"].([]any)
	if !ok || len(blocks) != 2 || blocks[0].(map[string]any)["count"] != float64(1) || blocks[1].(map[string]any)["count"] != float64(3) {
		t.Fatalf("written block diagnostics = %#v", diagnostic["blocks"])
	}
	if _, exists := diagnostic["verified"]; exists {
		t.Fatalf("item-set diagnostic still exposes verified: %#v", diagnostic)
	}
	if !bytes.Contains(data, []byte(`"event":"item_set_unmapped_item"`)) || !bytes.Contains(data, []byte(`"id":9999`)) {
		t.Fatalf("unmapped item diagnostic missing: %s", data)
	}
}

func TestNewLCUItemSetNormalizesExplicitUnpurchasableIDsAndDeduplicates(t *testing.T) {
	request := gameplayItemSetApplyRequest{
		Title: "测试", ChampionID: 13, MapID: 11, Position: "middle",
		Blocks: []gameplayItemSetBlockRequest{{Type: "核心装", Items: []gameplayItemSetItemRequest{
			{ID: 3040, Count: 1}, {ID: 3003, Count: 1}, {ID: 3042, Count: 1}, {ID: 3157, Count: 1}, {ID: 9999, Count: 1},
		}}},
	}
	itemSet, normalization := newLCUItemSet(request, map[int64]bool{3040: false, 3042: false, 3157: true, 9999: false})
	if len(itemSet.Blocks) != 1 {
		t.Fatalf("normalized blocks = %#v", itemSet.Blocks)
	}
	ids := make([]string, 0, len(itemSet.Blocks[0].Items))
	for _, item := range itemSet.Blocks[0].Items {
		ids = append(ids, item.ID)
	}
	if !reflect.DeepEqual(ids, []string{"3003", "3004", "3157", "9999"}) {
		t.Fatalf("normalized item IDs = %#v", ids)
	}
	if normalization.NormalizedCount != 2 || normalization.UnpurchasableCount != 3 || !reflect.DeepEqual(normalization.UnmappedItemIDs, []int64{9999}) {
		t.Fatalf("normalization diagnostics = %#v", normalization)
	}
}

func TestItemSetNameUsesCompactPositionLabel(t *testing.T) {
	for position, want := range map[string]string{"top": "DL · 上路", "jungle": "DL · 打野", "middle": "DL · 中路", "bottom": "DL · 下路", "utility": "DL · 辅助", "other": "DL · 通用", "unknown": "DL · 通用"} {
		if got := itemSetName(position); got != want || !utf8.ValidString(got) {
			t.Fatalf("item-set title for %q = %q, want %q", position, got, want)
		}
	}
}

func TestR58ItemSetUsesNonZeroSortRankAndRuneSafeTitleLimit(t *testing.T) {
	request := gameplayItemSetApplyRequest{
		Title: strings.Repeat("中", 80), ChampionID: 13, MapID: 11, Position: "middle",
		Blocks: []gameplayItemSetBlockRequest{{Type: "核心装", Items: []gameplayItemSetItemRequest{{ID: 3003, Count: 1}}}},
	}
	itemSet, _ := newLCUItemSet(request, nil)
	if itemSet.SortRank != 100 {
		t.Fatalf("SortRank = %d, want 100", itemSet.SortRank)
	}
	if itemSet.Title != "DL · 中路" || !utf8.ValidString(itemSet.Title) {
		t.Fatalf("item-set title must stay compact and ignore the request title: %q", itemSet.Title)
	}
}

func TestItemSetApplyDecodeFailuresRecordStage(t *testing.T) {
	cases := map[string]string{
		"malformed":      `{`,
		"unknown-field":  `{"future":true}`,
		"trailing-value": `{}` + "\n{}",
		"oversized":      `{"title":"` + strings.Repeat("x", 33*1024) + `"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
				t.Fatal(err)
			}
			a := &app{storage: trackTestStore(t, &localStore{root: root})}
			recorder := httptest.NewRecorder()
			a.handleGameplayItemSetApply(recorder, httptest.NewRequest(http.MethodPost, "/api/gameplay/item-sets/apply", strings.NewReader(body)))
			if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "装备方案无效") {
				t.Fatalf("status/body = %d %q", recorder.Code, recorder.Body.String())
			}
			data, err := a.storage.readDiagnosticLog()
			if err != nil {
				t.Fatal(err)
			}
			var event map[string]any
			if err := json.Unmarshal(bytes.TrimSpace(data), &event); err != nil {
				t.Fatalf("decode diagnostic: %v: %s", err, data)
			}
			traceID, _ := event["trace_id"].(string)
			if event["event"] != "item_set_apply" || event["diagnostic_schema"] != float64(2) || traceID == "" || event["stored"] != false || event["apply_stage"] != "decode" || event["game_render_geometry"] != "unobservable" || event["game_loaded_recommendation"] != "unknown" {
				t.Fatalf("decode failure diagnostic = %#v", event)
			}
		})
	}
}

func TestItemSetNormalizationRecordsSortedReplacementAndDropOrder(t *testing.T) {
	request := gameplayItemSetApplyRequest{
		Title: "测试", ChampionID: 13, MapID: 11, Position: "middle",
		Blocks: []gameplayItemSetBlockRequest{{Type: "核心装", Items: []gameplayItemSetItemRequest{
			{ID: 3040, Count: 1}, {ID: 3003, Count: 1}, {ID: 6630, Count: 1}, {ID: 3071, Count: 1},
		}}},
	}
	requested := diagnosticBlocksFromRequest(request.Blocks)
	sortGameplayItemSetBlocksByPrice(request.Blocks, map[int64]int64{3040: 3000, 3003: 2600, 6630: 3300, 3071: 3100})
	sorted := diagnosticBlocksFromRequest(request.Blocks)
	itemSet, normalization := newLCUItemSet(request, nil)
	normalization.RequestedBlocks = requested
	normalization.SortedBlocks = sorted
	normalization.RequestedOrderDigest = itemSetOrderDigest(requested)
	normalization.SortedOrderDigest = itemSetOrderDigest(sorted)
	if got := []string{itemSet.Blocks[0].Items[0].ID, itemSet.Blocks[0].Items[1].ID, itemSet.Blocks[0].Items[2].ID}; !reflect.DeepEqual(got, []string{"3003", "3071", "6630"}) {
		t.Fatalf("written order = %v", got)
	}
	if normalization.RequestedOrderDigest == normalization.SortedOrderDigest || normalization.WrittenOrderDigest == "" {
		t.Fatalf("missing order evidence: %+v", normalization)
	}
	if len(normalization.ItemMapping) != 4 {
		t.Fatalf("mapping count = %d", len(normalization.ItemMapping))
	}
	var replacement, dropped *itemSetItemMapping
	for index := range normalization.ItemMapping {
		mapping := &normalization.ItemMapping[index]
		if mapping.RequestedID == 3040 {
			replacement = mapping
		}
		if mapping.Result == "duplicate-dropped" {
			dropped = mapping
		}
	}
	if replacement == nil || replacement.NormalizedID != 3003 || replacement.Result != "duplicate-dropped" || replacement.Reason != "replacement-duplicate" || replacement.WrittenIndex != -1 {
		t.Fatalf("upgrade mapping = %+v; all=%+v", replacement, normalization.ItemMapping)
	}
	if dropped != replacement {
		t.Fatalf("unexpected dropped mapping = %+v", dropped)
	}
}

func TestItemSetNormalizationReportsWrittenCountsAfterUpgradeCollapse(t *testing.T) {
	request := gameplayItemSetApplyRequest{
		Title: "测试", ChampionID: 13, MapID: 11, Position: "middle",
		Blocks: []gameplayItemSetBlockRequest{{Type: "核心装", Items: []gameplayItemSetItemRequest{
			{ID: 3003, Count: 1}, {ID: 3040, Count: 1}, {ID: 3071, Count: 1}, {ID: 6630, Count: 1}, {ID: 3157, Count: 1},
		}}},
	}
	itemSet, normalization := newLCUItemSet(request, nil)
	stats, written := itemSetStats(itemSet)
	if normalization.RequestedItemCount != 5 {
		t.Fatalf("requested count = %d, want 5", normalization.RequestedItemCount)
	}
	if written != 4 || len(stats) != 1 || stats[0]["count"] != 4 {
		t.Fatalf("written counts = %d/%#v, want 4/one block of 4", written, stats)
	}
	if !normalization.UpgradeReplacementCollapsed {
		t.Fatal("upgrade replacement collapse was not reported")
	}
}

func TestItemSetApplyHandlerReportsWrittenCountsAfterUpgradeCollapse(t *testing.T) {
	playerRef := strings.Repeat("p", 48)
	document := lcuItemSetDocument{
		"accountId": json.RawMessage(`456`),
		"itemSets":  json.RawMessage(`[]`),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /lol-gameflow/v1/gameflow-phase":
			_, _ = w.Write([]byte(`"ChampSelect"`))
		case "GET /lol-champ-select/v1/session":
			_, _ = w.Write([]byte(`{"myTeam":[{"summonerId":123,"puuid":"` + playerRef + `","championId":13}]}`))
		case "GET /lol-game-data/assets/v1/items.json":
			_, _ = w.Write([]byte(`[{"id":3003,"priceTotal":3000},{"id":3040,"priceTotal":3000},{"id":3071,"priceTotal":3000},{"id":6630,"priceTotal":3300},{"id":3157,"priceTotal":3250}]`))
		case "GET /lol-item-sets/v1/item-sets/123/sets":
			_ = json.NewEncoder(w).Encode(document)
		case "PUT /lol-item-sets/v1/item-sets/123/sets":
			if err := json.NewDecoder(r.Body).Decode(&document); err != nil {
				t.Fatalf("decode item set PUT: %v", err)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{
		connected: true,
		lcu:       client,
		summoner:  Summoner{SummonerID: 123, AccountID: 456, PUUID: playerRef},
		storage:   trackTestStore(t, &localStore{root: root}),
	}
	body := `{"title":"瑞兹 · 中路","championId":13,"mapId":11,"position":"middle","blocks":[{"type":"核心装","items":[{"id":3003,"count":1},{"id":3040,"count":1},{"id":3071,"count":1},{"id":6630,"count":1},{"id":3157,"count":1}]}]}`
	recorder := httptest.NewRecorder()
	a.handleGameplayItemSetApply(recorder, httptest.NewRequest(http.MethodPost, "/api/gameplay/item-sets/apply", strings.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Applied bool   `json:"applied"`
		Stored  bool   `json:"stored"`
		Notice  string `json:"notice"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || !response.Applied || !response.Stored || response.Notice == "" {
		t.Fatalf("handler response = %#v, %v", response, err)
	}

	data, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	var diagnostic map[string]any
	if err := json.Unmarshal(lines[len(lines)-1], &diagnostic); err != nil {
		t.Fatal(err)
	}
	if diagnostic["event"] != "item_set_apply" || diagnostic["stored"] != true || diagnostic["item_count"] != float64(4) || diagnostic["requested_item_count"] != float64(5) {
		t.Fatalf("handler count diagnostic = %#v", diagnostic)
	}
	blocks, ok := diagnostic["blocks"].([]any)
	if !ok || len(blocks) != 1 {
		t.Fatalf("handler block diagnostics = %#v", diagnostic["blocks"])
	}
	block, ok := blocks[0].(map[string]any)
	if !ok || block["type"] != "核心装" || block["count"] != float64(4) {
		t.Fatalf("handler core block diagnostic = %#v", blocks[0])
	}
}

func TestUpsertLCUItemSetRemovesOnlyStaleDeepLegendsSets(t *testing.T) {
	item := func(uid, title, startedFrom string) map[string]any {
		return map[string]any{"uid": uid, "title": title, "type": "custom", "startedFrom": startedFrom, "blocks": []any{}}
	}
	old := []any{
		item("deep-legends-v1-64-top", "旧上路", "DL"),
		item("user-alpha", "用户方案 A", ""),
		item("legacy-r58-64-other", "DL · 瑞兹 · 位置未知", "DL"),
		item("deep-legends-v1-64-middle", "旧中路", "DL"),
		item("user-beta", "用户方案 B", "OTHER"),
		item("deep-legends-v1-103-jungle", "旧打野", "DL"),
	}
	raw, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	document := lcuItemSetDocument{"itemSets": raw}
	target := lcuItemSet{UID: itemSetUID(64, "jungle"), Title: "DL · 新方案", Type: "custom", Map: "any", Mode: "any", StartedFrom: "DL", AssociatedChampions: []int64{64}, Blocks: []lcuItemSetBlock{}, PreferredItemSlots: []lcuPreferredItemSlot{}}
	removed, err := upsertLCUItemSetWithCount(document, target)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 4 {
		t.Fatalf("removed stale count = %d, want 4", removed)
	}
	var sets []json.RawMessage
	if err := json.Unmarshal(document["itemSets"], &sets); err != nil {
		t.Fatal(err)
	}
	if len(sets) != 3 {
		t.Fatalf("result set count = %d, want 3", len(sets))
	}
	var got []struct {
		UID   string `json:"uid"`
		Title string `json:"title"`
	}
	for _, encoded := range sets {
		var value struct {
			UID   string `json:"uid"`
			Title string `json:"title"`
		}
		if err := json.Unmarshal(encoded, &value); err != nil {
			t.Fatal(err)
		}
		got = append(got, value)
	}
	if got[0].UID != "user-alpha" || got[1].UID != "user-beta" || got[2].UID != target.UID || got[2].Title != target.Title {
		t.Fatalf("set order/content = %#v", got)
	}
}

func TestSortGameplayItemSetBlocksByPriceKeepsUnknownSlotsAndStableTies(t *testing.T) {
	blocks := []gameplayItemSetBlockRequest{{Items: []gameplayItemSetItemRequest{
		{ID: 1, Count: 1}, {ID: 90, Count: 1}, {ID: 2, Count: 1}, {ID: 3, Count: 1}, {ID: 91, Count: 1},
	}}}
	sortGameplayItemSetBlocksByPrice(blocks, map[int64]int64{1: 400, 2: 50, 3: 400})
	got := make([]int64, 0, len(blocks[0].Items))
	for _, item := range blocks[0].Items {
		got = append(got, item.ID)
	}
	if !reflect.DeepEqual(got, []int64{2, 90, 1, 3, 91}) {
		t.Fatalf("stable price order with unknown slots = %v", got)
	}
}

func TestItemSetApplyNeverReadsOrWritesSetsOutsideChampionSelect(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet || r.URL.Path != "/lol-gameflow/v1/gameflow-phase" {
			t.Fatalf("unexpected item-set access outside champion select: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`"Lobby"`))
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{connected: true, lcu: client, summoner: Summoner{SummonerID: 123, PUUID: strings.Repeat("p", 48)}}
	body := `{"title":"测试","championId":64,"mapId":11,"position":"jungle","blocks":[{"type":"出门装","items":[{"id":1055,"count":1}]}]}`
	recorder := httptest.NewRecorder()
	a.handleGameplayItemSetApply(recorder, httptest.NewRequest(http.MethodPost, "/api/gameplay/item-sets/apply", strings.NewReader(body)))
	if recorder.Code != http.StatusConflict || requests != 1 {
		t.Fatalf("status/requests = %d/%d, want %d/1", recorder.Code, requests, http.StatusConflict)
	}
}

func TestSecurityHeadersRemainStrictForGameplayPages(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/live", nil))
	wantCSP := "default-src 'self'; img-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'"
	if recorder.Header().Get("Content-Security-Policy") != wantCSP || recorder.Header().Get("Referrer-Policy") != "no-referrer" || recorder.Header().Get("Cache-Control") != "no-store" || recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("security headers changed: %#v", recorder.Header())
	}
}

// A manually fired deadline, not a shortened sleep. Child contexts observe the
// same DeadlineExceeded value as context.WithTimeout in production.
type r86DeadlineContext struct{ context.Context }

func (c r86DeadlineContext) Err() error {
	if c.Context.Err() != nil {
		return context.DeadlineExceeded
	}
	return nil
}
