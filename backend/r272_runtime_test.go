package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestR272CollectionEventAndPostgame(t *testing.T) {
	for _, phase := range []string{"Lobby", "InProgress"} {
		t.Run(phase, func(t *testing.T) {
			client := &LCUClient{}
			a := &app{connected: true, lcu: client, refreshRequests: make(chan struct{}, 1)}
			a.gameplayFlow.phase = phase
			a.gameplayFlow.client = client
			a.markCollectionDirty("inventory")
			if phase == "Lobby" {
				if !r287WaitSelect(t, "backend/r272_runtime_test.go:26 wait", 5*time.Second, func(r287Deadline <-chan time.Time) bool {
					select {
					case <-a.refreshRequests:
						return true
					case <-r287Deadline:
						return false
					}
				}) {
					t.Fatal("inventory did not refresh within5s")
				}
			} else {
				time.Sleep(collectionEventRefreshDelay + 100*time.Millisecond)
				select {
				case <-a.refreshRequests:
					t.Fatal("refreshed during game")
				default:
				}
				a.observeGameplayPhase(context.Background(), client, "EndOfGame")
				if !r287WaitSelect(t, "backend/r272_runtime_test.go:39 wait", time.Second, func(r287Deadline <-chan time.Time) bool {
					select {
					case <-a.refreshRequests:
						return true
					case <-r287Deadline:
						return false
					}
				}) {
					t.Fatal("postgame discarded dirty inventory")
				}
			}
		})
	}
}
func TestR272ArenaKnownTrioSurvivesMissingMember(t *testing.T) {
	for _, found := range []bool{false, true} {
		t.Run(fmt.Sprint(found), func(t *testing.T) {
			a := &app{arenaAllyGameID: 272, arenaAllyPlayers: []lcuLivePlayer{{PUUID: "self", SummonerID: 1, ChampionID: 1}, {PUUID: "friend", SummonerID: 2, ChampionID: 2}, {PUUID: emptyLCUPlayerPUUID, SummonerID: 3, ChampionID: 3, NameVisibilityType: "HIDDEN"}}}
			response := gameplayLiveResponse{GameID: 272, QueueID: 1750, GameMode: "CHERRY", Phase: "InProgress", Players: []gameplayLivePlayer{{gameplayPlayer: gameplayPlayer{IsCurrent: true, reference: gameplayReference{PlayerRef: "self", SummonerID: 1}}, ChampionID: 1}, {gameplayPlayer: gameplayPlayer{reference: gameplayReference{PlayerRef: "friend", SummonerID: 2}}, ChampionID: 2}}}
			if found {
				response.Players = append(response.Players, gameplayLivePlayer{ChampionID: 3, gameplayPlayer: gameplayPlayer{Hidden: true}})
			}
			a.applyArenaLiveGrouping(nil, Summoner{PUUID: "self"}, &response, nil, liveClientSnapshot{})
			count := 0
			for _, p := range response.Players {
				if p.MySquad {
					count++
				}
			}
			if count != 3 || len(response.Players) != 3 {
				t.Fatalf("my squad=%d roster=%d", count, len(response.Players))
			}
			if !response.Players[2].Hidden {
				t.Fatal("hidden identity was revealed")
			}
		})
	}
}
func TestR272ArenaFameRankedSourceRemainsUnverifiedForSelfAndOther(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"queues":[{"queueType":"CHERRY","ratedTier":"SILVER","ratedRating":12600.4}]}`)
	}))
	defer server.Close()
	client := &LCUClient{token: "fixture", http: server.Client(), baseURL: server.URL, region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	for _, self := range []bool{true, false} {
		a := &app{sgp: newSGPProvider()}
		value := a.loadLiveArenaFame(context.Background(), client, gameplayReference{ServerID: "HN1"}, strings.Repeat("a", 48), self, "")
		if value != nil {
			t.Fatal("unverified ranked Fame must remain hidden", self, value)
		}
	}
}
func TestR272MayhemContinuousAndIdleBody(t *testing.T) {
	for _, spec := range []struct {
		name        string
		header, gap time.Duration
		parts       int
		want        bool
	}{{"specified_3s_20KB_each_second", 3 * time.Second, time.Second, 7, true}, {"continuous_beyond_old15s", 0, 2500 * time.Millisecond, 7, true}, {"idle8s", 0, 9 * time.Second, 2, false}} {
		t.Run(spec.name, func(t *testing.T) {
			raw := `{"code":200,"data":{"rating":2001,"syntheticPadding":"` + strings.Repeat("x", 140000) + `"}}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case <-time.After(spec.header):
				case <-r.Context().Done():
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(200)
				w.(http.Flusher).Flush()
				size := (len(raw) + spec.parts - 1) / spec.parts
				for start := 0; start < len(raw); start += size {
					select {
					case <-time.After(spec.gap):
					case <-r.Context().Done():
						return
					}
					if _, err := io.WriteString(w, raw[start:min(len(raw), start+size)]); err != nil {
						return
					}
					w.(http.Flusher).Flush()
				}
			}))
			defer server.Close()
			c := newAramkitTestClient(server.Client().Transport)
			c.baseURL = server.URL
			c.timeout = 12 * time.Second
			c.bodyTimeout = 8 * time.Second
			c.totalTimeout = aramkitRatingTotalTimeout
			result := c.fetch(context.Background(), "Anonymous", "TEST")
			if result.response.Available != spec.want {
				t.Fatalf("available=%v reason=%s bytes=%d", result.response.Available, result.reason, result.bytes)
			}
			if !spec.want && result.reason != "timeout" {
				t.Fatal(result.reason)
			}
		})
	}
}
func TestR272DiagnosticsFieldsReachLog(t *testing.T) {
	a := r175App(t)
	for _, body := range []string{`{"event":"overview_scroll_reset","reason":"restored","tabType":"pro","renderReason":"opgg_season","scroll_before":1200,"scroll_after":0}`, `{"event":"overview_reference_expired","reason":"retry","tabType":"riot","append":true,"key_change_ms":12}`, `{"event":"history_search_client","reason":"no_more","beg_index":80,"new_unique":60,"exhausted":true,"time_range":"2026-09-11"}`, `{"event":"lp_chip_render","reason":"render","ranked_matches":4,"with_delta":4,"chips":4,"revision_skipped":0}`} {
		w := httptest.NewRecorder()
		a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(body)))
		if w.Code != 204 {
			t.Fatal(w.Code, w.Body.String())
		}
		var input map[string]any
		_ = json.Unmarshal([]byte(body), &input)
		rows := r175Events(t, a, input["event"].(string))
		if len(rows) != 1 {
			t.Fatal("diagnostic lost", input["event"])
		}
	}
}
func TestR272ArenaPlayerListRecoveryThenKnownSquad(t *testing.T) {
	for _, present := range []bool{true, false} {
		t.Run(fmt.Sprint(present), func(t *testing.T) {
			players, raw := r62ArenaFixturePlayers(t, 18)
			var list []map[string]any
			_ = json.Unmarshal(raw, &list)
			if !present {
				list = append(list[:2], list[3:]...)
			}
			raw, _ = json.Marshal(list)
			roster := append(append([]map[string]any{}, players[:2]...), players[3:]...)
			a := r90Fixture(t, roster, raw)
			a.arenaAllyGameID = 90001
			a.arenaAllyPlayers = []lcuLivePlayer{{PUUID: "r90-player-identity-01", SummonerID: 1, ChampionID: 1}, {PUUID: "r90-player-identity-02", SummonerID: 2, ChampionID: 2}, {PUUID: "r90-player-identity-03", SummonerID: 3, ChampionID: 3, NameVisibilityType: "HIDDEN"}}
			a.liveClientAllGameData = func(context.Context) ([]byte, int, error) { return []byte(`{"allPlayers":[]}`), 200, nil }
			result := a.loadGameplayLive(context.Background(), a.lcu, a.summoner, "InProgress")
			squad := 0
			for _, p := range result.Players {
				if p.MySquad {
					squad++
				}
			}
			if len(result.Players) != 18 || squad != 3 {
				t.Fatalf("playerlist_present=%v roster=%d known_squad=%d", present, len(result.Players), squad)
			}
			if present && result.MergeAppended != 1 {
				t.Fatal("playerlist not used")
			}
			if !present {
				found := false
				for _, p := range result.Players {
					found = found || p.MySquad && p.ChampionID == 3 && !p.Hidden && p.GameName == "R62Arena03" && p.reference.PlayerRef == "r90-player-identity-03"
				}
				if !found {
					t.Fatal("known identity did not resolve through the live profile pipeline")
				}
			}
		})
	}
}

func TestR272MayhemGzipNegotiation(t *testing.T) {
	raw := []byte(`{"code":200,"data":{"rating":2001,"syntheticPadding":"` + strings.Repeat("x", 140000) + `"}}`)
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, _ = writer.Write(raw)
	_ = writer.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept-Encoding") != "gzip" {
			t.Error("gzip not negotiated")
		}
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(compressed.Bytes())
	}))
	defer server.Close()
	c := newAramkitTestClient(server.Client().Transport)
	c.baseURL = server.URL
	result := c.fetch(context.Background(), "Anonymous", "TEST")
	if !result.response.Available || result.bytes != compressed.Len() || result.bytes >= len(raw)/10 {
		t.Fatal("gzip decoding/wire bytes", result.response.Available, result.bytes)
	}
	t.Logf("synthetic raw=%d gzip=%d", len(raw), result.bytes)
}

func TestR272CollectionTrackerLoadedByReleaseHTML(t *testing.T) {
	html, err := embedded.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(html), `<script src="/collection-new-items.js" defer></script>`) {
		t.Fatal("release HTML does not load collection ownership tracker")
	}
	script, err := embedded.ReadFile("web/collection-new-items.js")
	if err != nil || !strings.Contains(string(script), "deepLegendsCollectionNewItems") {
		t.Fatal("embedded collection tracker unavailable", err)
	}
}
