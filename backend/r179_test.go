package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestR179SpecialistPrefetchCoalescesVisibleRows(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "fixture-only")
	started := make(chan string, 8)
	release := make(chan struct{})
	var closeOnce sync.Once
	unblock := func() { closeOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	var timelines atomic.Int32
	p := specialistTestProvider(gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.URL.Host == opggPageHost:
			return specialistTestResponse(r, 200, specialistLeaderboardBody(championTopPlayer{Name: "First", Tagline: "KR1"}, championTopPlayer{Name: "Second", Tagline: "KR1"}))
		case strings.Contains(r.URL.Path, "/accounts/by-riot-id/"):
			ref := "first"
			if strings.Contains(r.URL.Path, "Second") {
				ref = "second"
			}
			return specialistTestResponse(r, 200, fmt.Sprintf(`{"puuid":%q}`, ref))
		case strings.HasSuffix(r.URL.Path, "/ids"):
			if strings.Contains(r.URL.Path, "second") {
				return specialistTestResponse(r, 200, `["KR_4","KR_5","KR_6"]`)
			}
			return specialistTestResponse(r, 200, `["KR_1","KR_2","KR_3"]`)
		case strings.HasSuffix(r.URL.Path, "/timeline"):
			timelines.Add(1)
			started <- r.URL.Path
			select {
			case <-release:
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
			return specialistTestResponse(r, 200, r177Timeline)
		case strings.Contains(r.URL.Path, "/matches/KR_"):
			id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			ref := "first"
			if id == "KR_4" || id == "KR_5" || id == "KR_6" {
				ref = "second"
			}
			return specialistTestResponse(r, 200, strings.Replace(specialistMatchBody(id, ref, 64, "complete", true), `"puuid":"`+ref+`"`, `"participantId":1,"puuid":"`+ref+`"`, 1))
		default:
			return specialistTestResponse(r, 404, `{}`)
		}
	}))
	a := r175App(t)
	a.riot = p
	p.champions.diag = a.recordDiagnostic
	rows, outcome := p.specialistRunes(context.Background(), 64, "leesin", "李青", "mid")
	if outcome != specialistOutcomeSuccess || len(rows) != 6 {
		t.Fatalf("rows=%d outcome=%s", len(rows), outcome)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("first two prefetch workers not started")
		}
	}
	// The async timeline calls remain blocked; the response cache lookup must return.
	returned := make(chan struct{})
	go func() { p.attachReadySpecialistStarters(rows); close(returned) }()
	select {
	case <-returned:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("response waited for unfinished timeline")
	}
	for _, row := range rows {
		if len(row.StarterItemIDs) > 0 {
			t.Fatal("uncached starter fabricated")
		}
	}
	wanted := map[string]int64{}
	for _, row := range rows[:3] {
		wanted[row.Key] = row.PlayedAt
	}
	got := make(chan []runeStarterRow, 1)
	go func() { got <- p.specialistStarterRows(context.Background(), 64, "mid", wanted) }()
	// Await actual joining of the existing flight rather than timing a request.
	time.Sleep(20 * time.Millisecond)
	if timelines.Load() != 2 {
		t.Fatalf("preflight/visible duplicate calls=%d", timelines.Load())
	}
	unblock()
	select {
	case result := <-got:
		if len(result) != 3 {
			t.Fatal(result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("merged request blocked")
	}
	deadline := time.Now().Add(time.Second)
	for timelines.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	p.attachReadySpecialistStarters(rows)
	for i, row := range rows {
		if i < 3 && !reflect.DeepEqual(row.StarterItemIDs, []int64{1055}) {
			t.Fatal("prefetch cache not attached", row)
		}
		if i >= 3 && len(row.StarterItemIDs) > 0 {
			t.Fatal("other specialist prefetched", row)
		}
	}
	if timelines.Load() != 3 {
		t.Fatal("more than first three prefetched", timelines.Load())
	}
	done := r175Events(t, a, "specialist_runes_done")
	if len(done) != 1 || done[0]["prefetch_started"] != float64(3) {
		t.Fatal(done)
	}
	encoded, _ := json.Marshal(runeStarterRequest{Source: "specialist", ChampionID: 64, Position: "mid", Rows: []runeStarterRowRequest{{Key: rows[0].Key, PlayedAt: rows[0].PlayedAt}}})
	w := httptest.NewRecorder()
	a.handleGameplayRuneStarters(w, httptest.NewRequest(http.MethodPost, "/api/gameplay/rune-starters", strings.NewReader(string(encoded))))
	batch := r175Events(t, a, "rune_starter_batch")
	if w.Code != 200 || len(batch) != 1 || batch[0]["prefetched"] != float64(1) || batch[0]["duration_ms"] == nil {
		t.Fatal(w.Code, batch)
	}
}

func TestR179ChampselectLobbyInferenceAndStableGroups(t *testing.T) {
	for _, tc := range []struct {
		name      string
		members   []int
		failed    bool
		wantLobby bool
	}{{"lobby-two", []int{0, 1}, false, true}, {"self-only", []int{0}, false, false}, {"lobby-failed", nil, true, false}} {
		t.Run(tc.name, func(t *testing.T) {
			a := r175App(t)
			client := &LCUClient{baseURL: "http://lcu.invalid", token: "test", region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
			teamOne, teamTwo := []any{}, []any{}
			for i := 0; i < 10; i++ {
				raw := map[string]any{"cellId": i, "puuid": r161Ref(i), "gameName": fmt.Sprintf("P%d", i), "tagLine": "CN1", "assignedPosition": "TOP"}
				if i < 5 {
					teamOne = append(teamOne, raw)
				} else {
					teamTwo = append(teamTwo, raw)
				}
			}
			lobby := []any{}
			for _, i := range tc.members {
				lobby = append(lobby, map[string]any{"puuid": r161Ref(i)})
			}
			client.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch {
				case r.URL.Path == "/lol-gameflow/v1/session":
					return r178JSON(map[string]any{"gameData": map[string]any{"gameId": 179, "queue": map[string]any{"id": 440, "mapId": 11, "gameMode": "CLASSIC"}}}, 200), nil
				case r.URL.Path == "/lol-champ-select/v1/session":
					return r178JSON(map[string]any{"gameId": 179, "localPlayerCellId": 0, "myTeam": teamOne, "theirTeam": teamTwo}, 200), nil
				case r.URL.Path == "/lol-lobby/v2/lobby":
					if tc.failed {
						return r178JSON(map[string]any{}, 503), nil
					}
					return r178JSON(map[string]any{"members": lobby}, 200), nil
				case strings.Contains(r.URL.Path, "/lol-match-history/"):
					index := 0
					for i := 0; i < 10; i++ {
						if strings.Contains(r.URL.Path, r161Ref(i)) {
							index = i
						}
					}
					games := []any{}
					for j := int64(1); j <= 5; j++ {
						id := int64(index*100) + j
						if index == 2 || index == 3 {
							id = 1000 + j
						}
						if index >= 5 {
							id = 2000 + j
						}
						games = append(games, map[string]any{"gameId": id, "queueId": 440, "gameCreation": time.Now().UnixMilli()})
					}
					return r178JSON(map[string]any{"games": map[string]any{"games": games}}, 200), nil
				default:
					return r178JSON(map[string]any{}, 404), nil
				}
			})}
			response := a.loadGameplayLive(context.Background(), client, Summoner{PUUID: r161Ref(0), GameName: "P0", TagLine: "CN1"}, "ChampSelect")
			if len(response.Players) != 10 {
				t.Fatal(len(response.Players))
			}
			players := response.Players
			if tc.wantLobby {
				if players[0].PremadeGroup == "" || players[0].PremadeGroup != players[1].PremadeGroup || players[0].PremadeSource != "lobby" || !players[0].PremadeSessionSignal {
					t.Fatal("lobby lost", players[:2])
				}
			} else if players[0].PremadeGroup != "" {
				t.Fatal("solo self grouped", players[0])
			}
			if players[2].PremadeGroup == "" || players[2].PremadeGroup != players[3].PremadeGroup || players[2].PremadeSource != "inferred" {
				t.Fatal("inference lost", players[2:4])
			}
			for _, p := range players[5:] {
				if p.PremadeGroup != "" {
					t.Fatal("enemy champselect grouped", p)
				}
			}
			// Reorder roster on phase transition; labels follow the same member set.
			before := players[2].PremadeGroup
			next := []gameplayLivePlayer{players[3], players[2], players[0], players[1]}
			inputs := []livePremadeInput{{TeamID: 100, TeamParticipantID: 2}, {TeamID: 100, TeamParticipantID: 2}, {TeamID: 100, TeamParticipantID: 1}, {TeamID: 100, TeamParticipantID: 1}}
			a.applyLivePremades(179, next, inputs, "InProgress", false, nil)
			if next[0].PremadeGroup != before || next[1].PremadeGroup != before {
				t.Fatal("label jumped", before, next)
			}
			arena := make([]gameplayLivePlayer, 2)
			a.applyLivePremades(179, arena, inputs[:2], "ChampSelect", true, []lcuLobbyMember{{PUUID: r161Ref(0)}})
			if arena[0].PremadeGroup != "" {
				t.Fatal("arena changed")
			}
		})
	}
}

func TestR179ClientDiagnosticsTransportToLog(t *testing.T) {
	// Static fixtures are emitted by the real runtime and direct lane sender in
	// the Node test. Both then pass through the actual Go allowlist and storage.
	for _, filename := range []string{"testdata/r179-runtime-events.json", "testdata/r179-lane-events.json"} {
		data, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		var payloads []json.RawMessage
		if err = json.Unmarshal(data, &payloads); err != nil {
			t.Fatal(err)
		}
		a := r175App(t)
		for _, payload := range payloads {
			w := httptest.NewRecorder()
			a.handleClientDiagnostic(w, httptest.NewRequest(http.MethodPost, "/api/diagnostics/client", strings.NewReader(string(payload))))
			if w.Code != 204 {
				t.Fatal(w.Code, w.Body.String())
			}
		}
		lanes := r175Events(t, a, "lane_matchup_candidate_fetch")
		if len(lanes) != 1 || lanes[0]["self_position"] != "mid" || lanes[0]["enemy_locked_count"] != float64(4) || lanes[0]["enemy_position_known_count"] != float64(2) || lanes[0]["ally_position_known_count"] != float64(5) {
			t.Fatal(lanes)
		}
		if strings.Contains(filename, "runtime") {
			events := r175Events(t, a, "live_render_rebuild")
			if len(events) != 1 || events[0]["rows_replaced"] != float64(10) || events[0]["images_recreated"] != float64(0) || events[0]["phase"] != "ChampSelect" {
				t.Fatal(events)
			}
		}
	}
}

func TestR179PrefetchedCountRequiresSuccessfulPrefetchLoader(t *testing.T) {
	a, _ := r177StarterApp(t, 0, 200)
	// A failed prefetch claim must not label a later foreground loader as a hit.
	a.riot.starterPrefetches = map[string]specialistStarterPrefetch{"KR_1": {at: time.Now(), done: true, success: false}}
	r177StarterRequest(t, a, 1)
	events := r175Events(t, a, "rune_starter_batch")
	if len(events) != 1 || events[0]["prefetched"] != float64(0) {
		t.Fatal(events)
	}
}
