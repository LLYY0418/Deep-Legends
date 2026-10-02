package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestR165ChampSelectAutofillOnlyUsesMyTeam(t *testing.T) {
	session := lcuChampSelectSession{
		MyTeam:    []lcuChampSelectPlayer{{PUUID: "own", IsAutofilled: true}, {PUUID: "mate"}},
		TheirTeam: []lcuChampSelectPlayer{{PUUID: "foe", IsAutofilled: true}},
	}
	players := mergeChampSelectPlayers(nil, session, 0)
	if len(players) != 3 || !players[0].player.IsAutofilled || players[1].player.IsAutofilled || players[2].player.IsAutofilled {
		t.Fatalf("only myTeam true may be marked: %#v", players)
	}
}

func TestR165LiveAutofillSnapshotStaysInOneGame(t *testing.T) {
	a := &app{}
	players := []gameplayLivePlayer{{gameplayPlayer: gameplayPlayer{PlayerRef: "player_own", Autofill: true}}, {gameplayPlayer: gameplayPlayer{PlayerRef: "player_mate"}}, {gameplayPlayer: gameplayPlayer{PlayerRef: "raw-puuid", Autofill: true}}}
	a.rememberLiveAutofillSnapshot(1651, players)
	continuing := []gameplayLivePlayer{{gameplayPlayer: gameplayPlayer{PlayerRef: "player_own"}}, {gameplayPlayer: gameplayPlayer{PlayerRef: "player_mate"}}, {gameplayPlayer: gameplayPlayer{PlayerRef: "raw-puuid"}}}
	a.applyLiveAutofillSnapshot(1651, continuing)
	if !continuing[0].Autofill || continuing[1].Autofill || continuing[2].Autofill {
		t.Fatalf("same-game session aliases = %#v", continuing)
	}
	nextGame := []gameplayLivePlayer{{gameplayPlayer: gameplayPlayer{PlayerRef: "player_own"}}}
	a.applyLiveAutofillSnapshot(1652, nextGame)
	if nextGame[0].Autofill || a.liveAutofillGameID != 0 || len(a.liveAutofillByRef) != 0 {
		t.Fatalf("new game retained autofill: %#v", nextGame)
	}
	a.rememberLiveAutofillSnapshot(1653, players)
	a.clearLiveAutofillSnapshot()
	if a.liveAutofillGameID != 0 || len(a.liveAutofillByRef) != 0 {
		t.Fatal("leaving live phase retained autofill")
	}
}

func TestR165LivePrivacyAndAutofillAcrossPhases(t *testing.T) {
	const ownRef = "ownplayer0000001"
	const mateRef = "mateplayer000001"
	const foeRef = "foeplayer0000001"
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	var gameID atomic.Int64
	gameID.Store(1651)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-gameflow/v1/session":
			_ = json.NewEncoder(w).Encode(map[string]any{"gameData": map[string]any{"gameId": gameID.Load(), "queue": map[string]any{"id": 420, "mapId": 11, "gameMode": "CLASSIC"}, "teamOne": []any{map[string]any{"puuid": ownRef}, map[string]any{"puuid": mateRef}}, "teamTwo": []any{map[string]any{"puuid": foeRef}}}})
		case "/lol-champ-select/v1/session":
			_ = json.NewEncoder(w).Encode(map[string]any{"gameId": gameID.Load(), "queueId": 420, "localPlayerCellId": 1, "myTeam": []any{map[string]any{"cellId": 1, "puuid": ownRef, "isAutofilled": true}, map[string]any{"cellId": 2, "puuid": mateRef, "isAutofilled": false}}, "theirTeam": []any{map[string]any{"cellId": 3, "puuid": foeRef, "isAutofilled": true, "assignedPosition": "TOP"}, map[string]any{"cellId": 4, "nameVisibilityType": "HIDDEN", "isAutofilled": true}}})
		case "/lol-champ-select/v1/current-champion":
			_, _ = w.Write([]byte(`0`))
		case "/lol-lobby/v2/lobby":
			_, _ = w.Write([]byte(`{"gameConfig":{"queueId":420,"mapId":11,"gameMode":"CLASSIC"}}`))
		case "/lol-summoner/v2/summoners/puuid/" + ownRef, "/lol-summoner/v2/summoners/puuid/" + mateRef, "/lol-summoner/v2/summoners/puuid/" + foeRef:
			name := strings.TrimPrefix(r.URL.Path, "/lol-summoner/v2/summoners/puuid/")
			privacy := "PUBLIC"
			if name == mateRef {
				privacy = "PRIVATE"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"puuid": name, "gameName": name, "privacy": privacy})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{storage: trackTestStore(t, &localStore{root: root})}
	current := Summoner{PUUID: ownRef}
	selectResponse := a.loadGameplayLive(context.Background(), client, current, "ChampSelect")
	if len(selectResponse.Players) != 4 || !selectResponse.Players[0].Autofill || selectResponse.Players[1].Autofill || selectResponse.Players[2].Autofill || selectResponse.Players[3].Autofill {
		t.Fatalf("champ-select autofill: %#v", selectResponse.Players)
	}
	if !selectResponse.Players[1].PrivateHistory || selectResponse.Players[2].PrivateHistory || !selectResponse.Players[3].Hidden || selectResponse.Players[3].PrivateHistory {
		t.Fatalf("champ-select privacy: %#v", selectResponse.Players)
	}
	for _, player := range selectResponse.Players[:3] {
		if !strings.HasPrefix(player.PlayerRef, "player_") {
			t.Fatalf("missing safe session alias: %#v", player)
		}
	}
	gameResponse := a.loadGameplayLive(context.Background(), client, current, "InProgress")
	if len(gameResponse.Players) != 3 || !gameResponse.Players[0].Autofill || gameResponse.Players[1].Autofill || gameResponse.Players[2].Autofill || !gameResponse.Players[1].PrivateHistory {
		t.Fatalf("in-progress facts: %#v", gameResponse.Players)
	}
	gameID.Store(1652)
	nextResponse := a.loadGameplayLive(context.Background(), client, current, "InProgress")
	if len(nextResponse.Players) != 3 || nextResponse.Players[0].Autofill {
		t.Fatalf("new game inherited old autofill: %#v", nextResponse.Players)
	}
	logData, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	var shape map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(logData)), "\n") {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event["event"] == "champ_select_autofill_shape" {
			shape = event
			break
		}
	}
	if shape["ally_count"] != float64(2) || shape["ally_autofilled_count"] != float64(1) || shape["enemy_autofilled_count"] != float64(2) || shape["enemy_assigned_position_count"] != float64(1) {
		t.Fatalf("aggregate autofill shape = %#v", shape)
	}
	for _, forbidden := range []string{"puuid", "gameName", "summonerName", "playerRef"} {
		if _, found := shape[forbidden]; found {
			t.Fatalf("autofill diagnostic leaked %s", forbidden)
		}
	}
}
