package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func r277CNScoreFixture(t *testing.T) (*app, *LCUClient, lcuGame, *riotMatchInfo) {
	t.Helper()
	a, client, _ := r258ViewFixture()
	client.baseURL = "https://127.0.0.1:27700"
	client.token = "fixture"
	t.Cleanup(client.Close)
	a.storage = newFlowDiagnosticStore(t)
	a.sgp = newSGPProvider()
	a.bindHistoryGameCache(client)
	raw, err := os.ReadFile("testdata/r272-synthetic-match.json")
	if err != nil {
		t.Fatal(err)
	}
	var riot riotMatch
	if err = json.Unmarshal(raw, &riot); err != nil {
		t.Fatal(err)
	}
	for i := range riot.Info.Participants {
		riot.Info.Participants[i].PUUID = fmt.Sprintf("cn-player-%d-%s", i, strings.Repeat("x", 40))
	}
	riot.Info.Participants[0].PUUID = a.summoner.PUUID
	var game lcuGame
	// LCU stats intentionally have their own wire shape, not Riot JSON fields.
	participants := make([]map[string]any, 10)
	identities := make([]map[string]any, 10)
	for i, p := range riot.Info.Participants {
		participants[i] = map[string]any{"participantId": p.ParticipantID, "teamId": p.TeamID, "championId": p.ChampionID, "stats": map[string]any{"kills": 4 + i, "deaths": 5, "assists": 7, "win": p.Win, "champLevel": 18, "goldEarned": 12000, "totalDamageDealtToChampions": 20000 + i*1000, "totalMinionsKilled": 180, "neutralMinionsKilled": 10, "visionScore": 20, "wardsPlaced": 6, "wardsKilled": 3}, "timeline": map[string]any{"lane": "TOP", "role": "SOLO"}}
		identities[i] = map[string]any{"participantId": p.ParticipantID, "player": map[string]any{"puuid": p.PUUID, "summonerId": i + 1, "gameName": fmt.Sprintf("CN%d", i)}}
	}
	encoded, _ := json.Marshal(map[string]any{"gameId": riot.Info.GameID, "gameDuration": 1800, "gameCreation": 1700000000000, "queueId": 420, "mapId": 11, "gameMode": "CLASSIC", "gameType": "MATCHED_GAME", "participants": participants, "participantIdentities": identities, "teams": []map[string]any{{"teamId": 100, "win": "Win"}, {"teamId": 200, "win": "Fail"}}})
	if err = json.Unmarshal(encoded, &game); err != nil {
		t.Fatal(err)
	}
	return a, client, game, &riot.Info
}
func r277ScoreRequest(a *app, body map[string]any) *httptest.ResponseRecorder {
	data, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	a.handleGameplayMatch(w, httptest.NewRequest("POST", "/api/gameplay/match", strings.NewReader(string(data))))
	return w
}
func r277CNBody(a *app, player, server string, game int64) map[string]any {
	ref := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: player, ServerID: server, ClientIdentity: true})
	return map[string]any{"gameId": game, "participantId": 1, "region": "cn", "serverId": server, "playerRef": ref, "refresh": "score-parts"}
}
func r277AssertScoreParts(t *testing.T, w *httptest.ResponseRecorder, game int64) gameplayMatch {
	t.Helper()
	if w.Code != 200 {
		t.Fatalf("lazy score endpoint HTTP %d: %s", w.Code, w.Body)
	}
	var match gameplayMatch
	if err := json.Unmarshal(w.Body.Bytes(), &match); err != nil {
		t.Fatal(err)
	}
	if match.GameID != game || match.SubjectParticipantID != 1 || len(match.Participants) != 10 || match.Participants[0].Score == nil || len(match.Participants[0].Score.Parts) == 0 {
		t.Fatalf("score parts missing or wrong game/participant: %+v", match)
	}
	return match
}
func TestR277CNScoreLocalCacheAndDetail(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(fmt.Sprint("cached=", cached), func(t *testing.T) {
			a, client, game, _ := r277CNScoreFixture(t)
			var calls atomic.Int32
			client.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				if r.URL.Path != fmt.Sprintf("/lol-match-history/v1/games/%d", game.GameID) {
					t.Errorf("unexpected endpoint %s", r.URL.Path)
				}
				data, _ := json.Marshal(game)
				return hexdataResponse(r, data), nil
			})}
			cache, scope := client.historyGameStore()
			if cached {
				cache.putLCU(scope, game)
			}
			body := r277CNBody(a, a.summoner.PUUID, "HN10", game.GameID)
			for i := 0; i < 2; i++ {
				match := r277AssertScoreParts(t, r277ScoreRequest(a, body), game.GameID)
				if match.Participants[0].PlayerRef != body["playerRef"] {
					t.Fatal("anonymous identity changed")
				}
			}
			expected := int32(1)
			if cached {
				expected = 0
			}
			if calls.Load() != expected {
				t.Fatalf("detail calls=%d want %d", calls.Load(), expected)
			}
		})
	}
}
func TestR277CNScoreSGPCacheOtherPlayerAndServer(t *testing.T) {
	a, client, _, info := r277CNScoreFixture(t)
	// Remote CN player on another server must use the SGP complete cache, not LCU.
	info.Participants[0].PUUID = strings.Repeat("other", 12)
	a.sgp.historyGamesLocked().putSGP("HN1", info)
	client.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("cached SGP details must not query LCU: %s", r.URL)
		return nil, fmt.Errorf("unexpected upstream")
	})}
	body := r277CNBody(a, info.Participants[0].PUUID, "HN1", info.GameID)
	for i := 0; i < 2; i++ {
		r277AssertScoreParts(t, r277ScoreRequest(a, body), info.GameID)
	}
	// List contract is untouched: details parts never leak back into overview wire.
	full := convertRiotMatchInfo(info, info.Participants[0].PUUID, nil, nil, "", "HN1")
	applyMatchScores(&full)
	wire, _ := json.Marshal(overviewWireFrame(gameplayOverview{Matches: []gameplayMatch{full}}, "complete", map[int64]string{}))
	if strings.Contains(string(wire), `"parts"`) {
		t.Fatal("list emitted score parts")
	}
}
func TestR277CNScoreRejectsWrongScopeAndParticipant(t *testing.T) {
	for _, kind := range []string{"missing-ref", "unknown-ref", "wrong-server", "wrong-account", "participant", "game", "riot-ref", "unknown-region", "remote-lcu", "incomplete-sgp"} {
		t.Run(kind, func(t *testing.T) {
			a, client, game, info := r277CNScoreFixture(t)
			cache, scope := client.historyGameStore()
			cache.putLCU(scope, game)
			body := r277CNBody(a, a.summoner.PUUID, "HN10", game.GameID)
			switch kind {
			case "missing-ref":
				delete(body, "playerRef")
			case "unknown-ref":
				body["playerRef"] = "not-registered"
			case "wrong-server":
				body["serverId"] = "HN1"
			case "wrong-account":
				body["playerRef"] = a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: strings.Repeat("z", 48), ServerID: "HN10"})
			case "participant":
				body["participantId"] = 2
			case "game":
				body["gameId"] = game.GameID + 1
			case "riot-ref":
				body["playerRef"] = a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: a.summoner.PUUID, Region: "kr"})
			case "unknown-region":
				body["region"] = "garbage"
			case "remote-lcu":
				body = r277CNBody(a, a.summoner.PUUID, "HN1", game.GameID)
			case "incomplete-sgp":
				info.Participants = info.Participants[:1]
				a.sgp.historyGamesLocked().putSGP("HN1", info)
				body = r277CNBody(a, a.summoner.PUUID, "HN1", game.GameID)
			}
			client.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) { return hexdataResponse(r, []byte(`{"gameId":1}`)), nil })}
			w := r277ScoreRequest(a, body)
			if w.Code == 200 {
				t.Fatalf("%s exposed another scope/participant's cached score", kind)
			}
		})
	}
}

// A real HTTP handler fixture for the existing Chromium harness. Only immutable
// upstream data is synthetic; the browser's POST body reaches handleGameplayMatch.
func TestR277CNScoreBrowserContract(t *testing.T) {
	readyFile := os.Getenv("R277_CN_SCORE_SERVER")
	a, client, game, info := r277CNScoreFixture(t)
	cache, scope := client.historyGameStore()
	cache.putLCU(scope, game)
	other := strings.Repeat("remote-other-", 5)
	info.Participants[0].PUUID = other
	a.sgp.historyGamesLocked().putSGP("HN1", info)
	// No accidental CN cache miss may produce an upstream network call.
	client.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("browser fixture unexpected LCU %s", r.URL)
	})}
	selfRef := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: a.summoner.PUUID, ServerID: "HN10", ClientIdentity: true})
	otherRef := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: other, ServerID: "HN1", ClientIdentity: true})
	local := normalizeGameplayMatch(game, gameplayReference{PlayerRef: a.summoner.PUUID, ServerID: "HN10", ClientIdentity: true}, nil, nil)
	a.publicizeMatchReferences(&local)
	remote := convertRiotMatchInfo(info, other, nil, nil, "", "HN1")
	a.publicizeMatchReferences(&remote)
	var reads atomic.Int32
	var mu sync.Mutex
	var bodies []map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("GET /fixture", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, map[string]any{"self": map[string]any{"playerRef": selfRef, "serverId": "HN10", "matches": overviewListMatches([]gameplayMatch{local})}, "cn": map[string]any{"playerRef": otherRef, "serverId": "HN1", "matches": overviewListMatches([]gameplayMatch{remote})}})
	})
	mux.HandleFunc("POST /api/gameplay/match", func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "fixture read", 400)
			return
		}
		var body map[string]any
		if json.Unmarshal(data, &body) != nil {
			http.Error(w, "fixture JSON", 400)
			return
		}
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		reads.Add(1)
		r.Body = io.NopCloser(strings.NewReader(string(data)))
		a.handleGameplayMatch(w, r)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	if readyFile == "" {
		// Normal/native Go proof executes both real HTTP contracts; the external
		// browser mode below additionally exercises renderer transport and hover.
		for _, subject := range []struct {
			ref, server string
			match       gameplayMatch
		}{
			{selfRef, "HN10", local}, {otherRef, "HN1", remote},
		} {
			body, err := json.Marshal(map[string]any{"gameId": subject.match.GameID, "participantId": subject.match.SubjectParticipantID, "region": "", "serverId": subject.server, "playerRef": subject.ref, "refresh": "score-parts"})
			if err != nil {
				t.Fatal(err)
			}
			response, err := server.Client().Post(server.URL+"/api/gameplay/match", "application/json", strings.NewReader(string(body)))
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusOK {
				t.Fatalf("normal HTTP %s: %d %s", subject.server, response.StatusCode, raw)
			}
			var result gameplayMatch
			if err := json.Unmarshal(raw, &result); err != nil {
				t.Fatal(err)
			}
			if result.GameID != subject.match.GameID {
				t.Fatal("wrong score game")
			}
			found := false
			for _, participant := range result.Participants {
				if participant.ParticipantID == result.SubjectParticipantID && participant.PlayerRef == subject.ref && participant.Score != nil && len(participant.Score.Parts) > 0 {
					found = true
				}
			}
			if !found {
				t.Fatalf("normal HTTP %s lost scoped parts: %s", subject.server, raw)
			}
		}
		if reads.Load() != 2 {
			t.Fatalf("normal HTTP POSTs=%d want 2", reads.Load())
		}
		t.Log("normal HTTP self + remote contracts PASS; no optional skip")
		return
	}
	ready, _ := json.Marshal(map[string]any{"url": server.URL})
	if err := os.WriteFile(readyFile, ready, 0600); err != nil {
		t.Fatal(err)
	}
	deadlineR287Timing := r287NewDeadline(t, "backend/r277_cn_score_test.go:270 deadline", 90*time.Second)
	defer deadlineR287Timing.Stop()
	deadline := deadlineR287Timing.C
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("browser did not finish score fixture")
		case <-ticker.C:
			if _, err := os.Stat(readyFile + ".done"); err == nil {
				if reads.Load() != 2 {
					t.Fatalf("browser POST count=%d want two scoped games", reads.Load())
				}
				mu.Lock()
				t.Logf("actual browser POSTs: %v", bodies)
				mu.Unlock()
				return
			}
		}
	}

}

func TestR277CNScoreSameServerOtherPlayer(t *testing.T) {
	a, client, game, _ := r277CNScoreFixture(t)
	player := strings.Repeat("local-other-", 5)
	game.ParticipantIdentities[0].Player.PUUID = player
	cache, scope := client.historyGameStore()
	cache.putLCU(scope, game)
	body := r277CNBody(a, player, "HN10", game.GameID)
	delete(body, "serverId") // anonymous reference, never current-server guessing
	match := r277AssertScoreParts(t, r277ScoreRequest(a, body), game.GameID)
	if match.Participants[0].PlayerRef != body["playerRef"] {
		t.Fatal("other-player subject changed")
	}
	wire, _ := json.Marshal(overviewWireFrame(gameplayOverview{Matches: []gameplayMatch{match}}, "complete", map[int64]string{}))
	if strings.Contains(string(wire), `"parts"`) {
		t.Fatal("local list emitted score parts")
	}
}
func TestR277CNScoreRejectsSessionChangeDuringDetail(t *testing.T) {
	for _, change := range []string{"account", "client"} {
		t.Run(change, func(t *testing.T) {
			a, client, game, _ := r277CNScoreFixture(t)
			body := r277CNBody(a, a.summoner.PUUID, "HN10", game.GameID)
			// Explicit type lets the syntactic capture audit identify the actual app lock.
			var sessionMu *sync.RWMutex = &a.mu
			client.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				sessionMu.Lock()
				if change == "account" {
					a.summoner.PUUID = "different-account"
				} else {
					a.lcu = nil
				}
				sessionMu.Unlock()
				data, _ := json.Marshal(game)
				return hexdataResponse(r, data), nil
			})}
			w := r277ScoreRequest(a, body)
			if w.Code != http.StatusConflict {
				t.Fatalf("%s changed: HTTP %d want 409: %s", change, w.Code, w.Body)
			}
		})
	}
}
func TestR277CNScoreSGPServerIsolation(t *testing.T) {
	a, client, _, info := r277CNScoreFixture(t)
	a.sgp.historyGamesLocked().putSGP("HN1", info)
	// Identical numeric game ID in another server must not hit that cached roster.
	body := r277CNBody(a, info.Participants[0].PUUID, "NJ100", info.GameID)
	var calls atomic.Int32
	client.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, fmt.Errorf("wrong-region fetch")
	})}
	w := r277ScoreRequest(a, body)
	if w.Code != 404 || calls.Load() != 0 {
		t.Fatalf("cross-server cache fallback HTTP=%d LCU=%d", w.Code, calls.Load())
	}
}

func TestR277CNScoreRiotScopedContractUnchanged(t *testing.T) {
	f := r98OverviewFixture(t, false)
	data, err := os.ReadFile("testdata/r272-synthetic-match.json")
	if err != nil {
		t.Fatal(err)
	}
	var raw riotMatch
	if err = json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	original := f.a.riot.champions.client.Transport
	f.a.riot.champions.client.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/matches/KR_272000") {
			calls.Add(1)
			return r206RelayResponse(200, data), nil
		}
		return original.RoundTrip(r)
	})
	public := f.a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: raw.Info.Participants[0].PUUID, Region: "kr"})
	body := map[string]any{"gameId": raw.Info.GameID, "participantId": 1, "region": "kr", "serverId": "", "playerRef": public, "refresh": "score-parts"}
	for i := 0; i < 2; i++ {
		match := r277AssertScoreParts(t, r277ScoreRequest(f.a, body), raw.Info.GameID)
		if match.Participants[0].PlayerRef != public {
			t.Fatal("Riot public subject changed")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("Riot cache calls=%d", calls.Load())
	}
	for _, kind := range []string{"server", "participant", "account", "region"} {
		copyBody := map[string]any{}
		for k, v := range body {
			copyBody[k] = v
		}
		switch kind {
		case "server":
			copyBody["serverId"] = "HN10"
		case "participant":
			copyBody["participantId"] = 2
		case "account":
			copyBody["playerRef"] = f.a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: "different-account", Region: "kr"})
		case "region":
			copyBody["playerRef"] = f.a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: raw.Info.Participants[0].PUUID, Region: "jp1"})
		}
		w := r277ScoreRequest(f.a, copyBody)
		if w.Code != 400 {
			t.Fatalf("Riot %s: HTTP %d want 400", kind, w.Code)
		}
	}
}
