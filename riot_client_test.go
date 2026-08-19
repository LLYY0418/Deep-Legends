package main

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRiotClientCommandLineUsesIndependentCredentials(t *testing.T) {
	commandLine := `"C:\\Riot Games\\Riot Client\\RiotClientServices.exe" --riotclient-app-port=51234 --riotclient-auth-token="riot-client-secret" --app-port=61234 --remoting-auth-token=lcu-secret`
	client, ok := riotClientFromCommandLine(commandLine)
	if !ok || client == nil || client.local.port != 51234 {
		t.Fatalf("Riot Client command line was not parsed: client=%#v ok=%v", client, ok)
	}
	if token, _ := client.local.credentials(); token != "riot-client-secret" {
		t.Fatal("Riot Client parser reused the LCU credential")
	}
	client.Close()
	spaced, spacedOK := riotClientFromCommandLine(`C:\\RIOTCLIENTSERVICES.EXE --RIOTCLIENT-APP-PORT "54321" --RIOTCLIENT-AUTH-TOKEN "token.with-special_chars-1"`)
	if !spacedOK || spaced.local.port != 54321 {
		t.Fatalf("spaced/case-insensitive command line was not parsed: client=%#v ok=%v", spaced, spacedOK)
	}
	if token, _ := spaced.local.credentials(); token != "token.with-special_chars-1" {
		t.Fatalf("special-character token = %q", token)
	}
	spaced.Close()
	for _, invalid := range []string{
		`C:\\LeagueClientUx.exe --riotclient-app-port=51234 --riotclient-auth-token=x`,
		`C:\\RiotClientServices.exe --app-port=51234 --remoting-auth-token=x`,
		`C:\\RiotClientServices.exe --riotclient-app-port=70000 --riotclient-auth-token=x`,
	} {
		if parsed, accepted := riotClientFromCommandLine(invalid); accepted || parsed != nil {
			t.Fatalf("invalid Riot Client command line was accepted: %s", invalid)
		}
	}
}

func TestPowerShellProcessOutputParserRejectsCorruption(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(`C:\\RiotClientServices.exe --riotclient-app-port=1 --riotclient-auth-token=x`))
	result, err := parsePowerShellProcessOutput([]byte("COUNT:2\nCMD:"+encoded+"\nUNREADABLE\n"), "test")
	if err != nil || result.ProcessCount != 2 || len(result.CommandLines) != 1 || result.Unreadable != 1 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	for name, output := range map[string]string{
		"missing count":   "CMD:" + encoded,
		"invalid count":   "COUNT:nope\nCMD:" + encoded,
		"duplicate count": "COUNT:1\nCOUNT:1\nCMD:" + encoded,
		"invalid base64":  "COUNT:1\nCMD:not-base64!",
		"inconsistent":    "COUNT:0\nCMD:" + encoded,
	} {
		if _, parseErr := parsePowerShellProcessOutput([]byte(output), "test"); parseErr == nil {
			t.Fatalf("%s output was accepted", name)
		}
	}
}

func TestRiotClientAliasLookupIsExactAndAuthenticated(t *testing.T) {
	puuid := strings.Repeat("p", 48)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/player-account/aliases/v1/lookup" || r.URL.Query().Get("gameName") != "测试 玩家" || r.URL.Query().Get("tagLine") != "12345" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("riot:riot-secret"))
		if r.Header.Get("Authorization") != wantAuth {
			http.Error(w, "bad auth", http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, `[{"alias":{"game_name":"测试 玩家","tag_line":"12345"},"puuid":"`+puuid+`"},{"alias":{"game_name":"其他玩家","tag_line":"12345"},"puuid":"`+strings.Repeat("q", 48)+`"}]`)
	}))
	defer server.Close()
	client := &RiotClientAPI{local: &LCUClient{baseURL: server.URL, token: "riot-secret", http: server.Client()}}
	aliases, err := client.aliasesByRiotID(context.Background(), "测试 玩家", "12345")
	if err != nil || len(aliases) != 1 || aliases[0].PUUID != puuid {
		t.Fatalf("aliases = %#v, err=%v", aliases, err)
	}
}

func TestRiotClientAliasLookupFailurePaths(t *testing.T) {
	notFound := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))
	defer notFound.Close()
	client := &RiotClientAPI{local: &LCUClient{baseURL: notFound.URL, token: "secret", http: notFound.Client()}}
	if _, err := client.aliasesByRiotID(context.Background(), "missing", "123"); !errors.Is(err, errRiotClientAliasNotFound) {
		t.Fatalf("404 error = %v", err)
	}
	if _, err := (&RiotClientAPI{}).aliasesByRiotID(context.Background(), "missing", "123"); !errors.Is(err, errRiotClientNotFound) {
		t.Fatalf("nil local client error = %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.aliasesByRiotID(cancelled, "missing", "123"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled lookup error = %v", err)
	}

	invalid := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `[{"alias":{"game_name":"wrong","tag_line":"999"},"puuid":"short"}]`)
	}))
	defer invalid.Close()
	invalidClient := &RiotClientAPI{local: &LCUClient{baseURL: invalid.URL, token: "secret", http: invalid.Client()}}
	if _, err := invalidClient.aliasesByRiotID(context.Background(), "expected", "123"); !errors.Is(err, errRiotClientAliasNotFound) {
		t.Fatalf("filtered-empty error = %v", err)
	}
}

func TestResolveTencentRiotIDKeepsRiotAndLeagueTokensSeparate(t *testing.T) {
	puuid := strings.Repeat("r", 48)
	riotServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("riot:riot-secret"))
		if r.Header.Get("Authorization") != wantAuth {
			http.Error(w, "bad Riot Client auth", http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, `[{"alias":{"game_name":"跨服玩家","tag_line":"9988"},"puuid":"`+puuid+`"}]`)
	}))
	defer riotServer.Close()

	lcuServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/lol-league-session/v1/league-session-token" {
			http.Error(w, "unexpected LCU endpoint", http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, `"league-session-secret"`)
	}))
	defer lcuServer.Close()

	sgpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/summoner-ledge/v1/regions/hn1/summoners/puuids" || r.Header.Get("Authorization") != "Bearer league-session-secret" {
			http.Error(w, "bad SGP request", http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), puuid) || strings.Contains(string(body), "riot-secret") {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, `[{"puuid":"`+puuid+`","name":"跨服玩家","profileIconId":27,"level":301,"privacy":"PUBLIC"}]`)
	}))
	defer sgpServer.Close()
	riotAPI := &RiotClientAPI{local: &LCUClient{baseURL: riotServer.URL, token: "riot-secret", http: riotServer.Client()}}
	provider := newSGPProvider()
	provider.http = sgpServer.Client()
	provider.serverBases["HN1"] = sgpServer.URL
	a := &app{sgp: provider, riotClientDiscovery: func() (*RiotClientAPI, error) { return riotAPI, nil }}
	lcu := &LCUClient{baseURL: lcuServer.URL, token: "lcu-secret", http: lcuServer.Client()}
	reference, err := a.resolveTencentRiotID(context.Background(), lcu, "跨服玩家", "9988", "HN1")
	if err != nil {
		t.Fatal(err)
	}
	if reference.PlayerRef != puuid || reference.ServerID != "HN1" || reference.GameName != "跨服玩家" || reference.TagLine != "9988" || reference.ProfileIconID != 27 {
		t.Fatalf("reference = %#v", reference)
	}
	if token, ok := riotAPI.local.credentials(); ok || token != "" {
		t.Fatal("short-lived Riot Client credential was not cleared")
	}
}

func TestRemoteMatchHistoryFailureNeverFallsBackToCurrentLCU(t *testing.T) {
	puuid := strings.Repeat("m", 48)
	var lcuHistoryCalls int
	lcuServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/entitlements/v1/token":
			_, _ = io.WriteString(w, `{"accessToken":"entitlements-secret"}`)
		default:
			if strings.Contains(r.URL.Path, "/lol-match-history/") {
				lcuHistoryCalls++
			}
			http.Error(w, "unexpected LCU fallback", http.StatusNotFound)
		}
	}))
	defer lcuServer.Close()
	sgpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "temporary failure", http.StatusInternalServerError)
	}))
	defer sgpServer.Close()
	client := &LCUClient{baseURL: lcuServer.URL, token: "lcu-secret", http: lcuServer.Client(), region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	provider := newSGPProvider()
	provider.http = sgpServer.Client()
	provider.serverBases["HN10"] = sgpServer.URL
	a := &app{sgp: provider}
	matches, capabilities, _ := a.loadDetailedMatches(context.Background(), client, gameplayReference{PlayerRef: puuid, ServerID: "HN10"}, puuid, false, 0, 20, nil, nil)
	if len(matches) != 0 || lcuHistoryCalls != 0 {
		t.Fatalf("remote failure fell back to current LCU: matches=%d calls=%d", len(matches), lcuHistoryCalls)
	}
	if len(capabilities) < 2 || capabilities[0].State != capabilityFailed {
		t.Fatalf("remote failure was not surfaced: %#v", capabilities)
	}
}

func TestRemoteRankedStatsIsExplicitlyUnsupported(t *testing.T) {
	puuid := strings.Repeat("q", 48)
	var sgpCalls int
	sgpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sgpCalls++
		http.Error(w, "remote ranked endpoint must not be called", http.StatusInternalServerError)
	}))
	defer sgpServer.Close()
	provider := newSGPProvider()
	provider.http = sgpServer.Client()
	provider.serverBases["HN10"] = sgpServer.URL
	client := &LCUClient{region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	a := &app{sgp: provider}

	ranks, capability := a.loadRanksWithFallback(context.Background(), client, puuid, false, "HN10")
	if len(ranks) != 0 || sgpCalls != 0 {
		t.Fatalf("remote ranked query ran: ranks=%#v calls=%d", ranks, sgpCalls)
	}
	if capability.Name != "ranked-stats" || capability.State != capabilityUnsupported || capability.Detail != "跨服暂不支持排位" {
		t.Fatalf("remote ranked capability = %#v", capability)
	}
}

func TestTencentReferenceAliasesAreScopedByServer(t *testing.T) {
	puuid := strings.Repeat("s", 48)
	a := &app{token: "session-secret", gameplayRefs: make(map[string]string), gameplayRefDetails: make(map[string]gameplayReference)}
	hn1 := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: puuid, ServerID: "HN1"})
	hn10 := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: puuid, ServerID: "HN10"})
	if hn1 == "" || hn10 == "" || hn1 == hn10 || strings.Contains(hn1+hn10, puuid) {
		t.Fatalf("server-scoped aliases are invalid: HN1=%q HN10=%q", hn1, hn10)
	}
	first, firstOK := a.resolveGameplayReferenceDetails(hn1)
	second, secondOK := a.resolveGameplayReferenceDetails(hn10)
	if !firstOK || !secondOK || first.ServerID != "HN1" || second.ServerID != "HN10" {
		t.Fatalf("server details were mixed: first=%#v second=%#v", first, second)
	}
}

func TestMatchTimelineRejectsServerRebinding(t *testing.T) {
	puuid := strings.Repeat("t", 48)
	a := &app{token: "session-secret", gameplayRefs: make(map[string]string), gameplayRefDetails: make(map[string]gameplayReference), matchTimelines: newMatchTimelineCache()}
	publicRef := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: puuid, ServerID: "HN1"})
	body := `{"gameId":123,"participantId":1,"serverId":"HN10","playerRef":"` + publicRef + `"}`
	recorder := httptest.NewRecorder()
	a.handleGameplayMatchTimeline(recorder, httptest.NewRequest(http.MethodPost, "/api/gameplay/match-timeline", strings.NewReader(body)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", recorder.Code, recorder.Body.String())
	}
}

func TestMatchTimelineRestoresServerFromRegisteredReference(t *testing.T) {
	puuid := strings.Repeat("u", 48)
	var localTimelineCalls int
	lcuServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/entitlements/v1/token":
			_, _ = io.WriteString(w, `{"accessToken":"entitlements-secret"}`)
		default:
			if strings.Contains(r.URL.Path, "game-timelines") {
				localTimelineCalls++
			}
			http.Error(w, "unexpected local request", http.StatusNotFound)
		}
	}))
	defer lcuServer.Close()
	sgpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/match-history-query/v1/products/lol/HN10_123/DETAILS" {
			http.Error(w, "wrong server", http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, `{"json":{"frames":[{"timestamp":0,"events":[{"type":"ITEM_PURCHASED","timestamp":10000,"participantId":1,"itemId":1055}]}]}}`)
	}))
	defer sgpServer.Close()
	client := &LCUClient{baseURL: lcuServer.URL, token: "lcu-secret", http: lcuServer.Client(), region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	provider := newSGPProvider()
	provider.http = sgpServer.Client()
	provider.serverBases["HN10"] = sgpServer.URL
	a := &app{
		token: "session-secret", connected: true, lcu: client, sgp: provider, matchTimelines: newMatchTimelineCache(),
		gameplayRefs: make(map[string]string), gameplayRefDetails: make(map[string]gameplayReference),
	}
	publicRef := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: puuid, ServerID: "HN10"})
	body := `{"gameId":123,"participantId":1,"playerRef":"` + publicRef + `"}`
	recorder := httptest.NewRecorder()
	a.handleGameplayMatchTimeline(recorder, httptest.NewRequest(http.MethodPost, "/api/gameplay/match-timeline", strings.NewReader(body)))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"source":"sgp"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if localTimelineCalls != 0 {
		t.Fatalf("remote timeline fell back to current LCU %d times", localTimelineCalls)
	}
}
