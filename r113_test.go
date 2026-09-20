package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR113ChatJIDAndPhaseCancellation(t *testing.T) {
	for _, id := range []string{"champ-select", "room-42@champselect.na1.pvp.net"} {
		if !safeLCUChatIdentifier(id) {
			t.Fatalf("valid chat id rejected: %s", id)
		}
	}
	for _, id := range []string{"../me", "x@y@z", "@room", "room@", "room%2fmessages", " room@host"} {
		if safeLCUChatIdentifier(id) {
			t.Fatalf("unsafe chat id accepted: %s", id)
		}
	}
	for _, cancelBeforeWrite := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelBeforeWrite), func(t *testing.T) {
			var writes atomic.Int32
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := &LCUClient{baseURL: "http://fixture", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if err := req.Context().Err(); err != nil {
					return nil, err
				}
				body := `{}`
				switch req.URL.Path {
				case "/lol-champ-select/v1/session":
					body = `{"queueId":450,"benchEnabled":true,"localPlayerCellId":0,"myTeam":[{"cellId":0,"team":100}]}`
				case "/lol-chat/v1/me":
					body = `{"id":"self"}`
				case "/lol-chat/v1/conversations":
					body = `[{"id":"room@champselect.pvp.net","type":"championSelect"}]`
					if cancelBeforeWrite {
						cancel()
					}
				case "/lol-chat/v1/conversations/room@champselect.pvp.net/messages":
					writes.Add(1)
				default:
					t.Errorf("unexpected path: %s", req.URL.Path)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})}}
			runner := newWatchRunner(nil, nil)
			result := runner.broadcastPositionContext(ctx, client, Summoner{}, watchBroadcastRule{})
			if cancelBeforeWrite && writes.Load() != 0 {
				t.Fatal("canceled selection wrote a chat message")
			}
			if !cancelBeforeWrite && (result != "fired" || writes.Load() != 1) {
				t.Fatalf("result=%s writes=%d", result, writes.Load())
			}
		})
	}
}

func TestR113BroadcastEventStormIsBoundedAndRearmsNextSession(t *testing.T) {
	var reads atomic.Int32
	client := &LCUClient{baseURL: "http://fixture", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("session not ready")
	})}}
	done := make(chan string, 10)
	runner := newWatchRunner(nil, func(event string) { done <- event })
	runner.observe = func(event map[string]any) {
		if event["event"] == "watch_action" && event["action"] == "position-broadcast" && event["result"] == "armed" {
			reads.Add(1)
		}
	}
	settings := defaultWatchSettings()
	settings.Rules.PositionBroadcast.Enabled = true
	runner.apply(settings)
	runner.handleChampSelectPhase("ChampSelect")
	for range 100 {
		runner.handleChampSelect(client, Summoner{})
	}
	deadline := time.After(6 * time.Second)
	for {
		select {
		case event := <-done:
			if event == "watch:skipped:position-broadcast:unavailable" {
				goto exhausted
			}
		case <-deadline:
			t.Fatal("bounded retries did not finish")
		}
	}
exhausted:
	for range 100 {
		runner.handleChampSelect(client, Summoner{})
	}
	if reads.Load() != 3 {
		t.Fatalf("event storm caused %d reads", reads.Load())
	}
	runner.handleChampSelectPhase("InProgress")
	runner.handleChampSelect(client, Summoner{})
	if reads.Load() != 3 {
		t.Fatal("broadcast resumed outside champion select")
	}
	runner.handleChampSelectPhase("ChampSelect")
	runner.mu.Lock()
	armed := runner.broadcastForSession
	runner.mu.Unlock()
	if armed {
		t.Fatal("next champion select was not reset")
	}
}

func TestR113PrewarmDeduplicatesAndDoesNotWaitForRoster(t *testing.T) {
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	p := &liveRecommendationPrewarmer{load: func(ctx context.Context, seed liveRecommendationSeed) error {
		close(started)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}, report: func(map[string]any) { close(finished) }}
	seed := liveRecommendationSeed{ChampionID: 13, QueueID: 450, MapID: 12, GameMode: "ARAM"}
	if !p.warm(seed) {
		t.Fatal("first warm not scheduled")
	}
	<-started
	for range 100 {
		if p.warm(seed) {
			t.Fatal("duplicate warm during flight")
		}
	}
	close(release)
	<-finished
	if p.warm(seed) {
		t.Fatal("fresh warm repeated provider work")
	}
	if p.warm(liveRecommendationSeed{ChampionID: -3, QueueID: 450}) {
		t.Fatal("unresolved random pick warmed")
	}
}

func TestR113ARAMSkipsRankRequestsAndStartsRecommendationsBeforeHistory(t *testing.T) {
	var rankReads atomic.Int32
	warmed := make(chan struct{})
	finished := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.URL.Path == "/lol-gameflow/v1/session":
			io.WriteString(w, `{"gameData":{"gameId":113,"queue":{"id":450,"mapId":12,"gameMode":"ARAM"},"teamOne":[{"puuid":"r113-player-reference","championId":13}],"teamTwo":[]}}`)
		case strings.HasPrefix(req.URL.Path, "/lol-summoner/v2/summoners/puuid/"):
			json.NewEncoder(w).Encode(Summoner{PUUID: "r113-player-reference", GameName: "Fixture", TagLine: "CN1"})
		case strings.HasPrefix(req.URL.Path, "/lol-ranked/"):
			rankReads.Add(1)
			io.WriteString(w, `{"queues":[]}`)
		case strings.HasPrefix(req.URL.Path, "/lol-match-history/"):
			select {
			case <-warmed:
			case <-time.After(time.Second):
				t.Error("recommendations waited for history")
			}
			io.WriteString(w, `{"games":{"gameCount":0,"games":[]}}`)
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()
	a := &app{liveClientPlayerList: func(context.Context) ([]byte, int, error) { return nil, 0, errors.New("fixture") }, liveRecommendationPrewarmer: &liveRecommendationPrewarmer{
		load: func(context.Context, liveRecommendationSeed) error { close(warmed); return nil }, report: func(map[string]any) { close(finished) },
	}}
	client := &LCUClient{baseURL: server.URL, token: "fixture", http: server.Client()}
	got := a.loadGameplayLive(context.Background(), client, Summoner{PUUID: "r113-player-reference"}, "InProgress")
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("prewarm never started")
	}
	if !got.Available || len(got.Players) != 1 || !got.Players[0].IsCurrent {
		t.Fatalf("unexpected roster: %#v", got.Players)
	}
	if rankReads.Load() != 0 || got.Players[0].Rank != nil {
		t.Fatal("ARAM still requested unused ranked data")
	}
}

func TestR113MayhemMergeKeepsSilverAfterFirstNine(t *testing.T) {
	primary := make([]championMetricRow, 9)
	for i := range primary {
		primary[i] = championMetricRow{Assets: []championAsset{{ID: i + 1}}, Rarity: "gold"}
	}
	fallback := []championMetricRow{{Assets: []championAsset{{ID: 10}}, Rarity: "silver"}}
	got := mergeMayhemAugmentRows(primary, fallback, 0)
	if len(got) != 10 || got[9].Rarity != "silver" {
		t.Fatal("silver discarded before grouping")
	}
}

func TestR113RSCDoesNotDiscardLaterRarityRows(t *testing.T) {
	var source strings.Builder
	for id := 1; id <= 18; id++ {
		fmt.Fprintf(&source, "%d:{\"metaId\":%d,\"metaType\":\"aram-augment\"}\n", id, id)
	}
	result, err := parseMayhemRSC([]byte(source.String()), "vayne")
	if err == nil {
		t.Fatal("fixture omits build sections and must remain incomplete")
	}
	if len(result.Augments) != 18 || result.Augments[17].Assets[0].ID != 18 {
		t.Fatal("later rarity rows were truncated before catalog enrichment")
	}
}

func TestR113FailedHistoryDoesNotFreezeForWholeGame(t *testing.T) {
	response := gameplayLiveResponse{Available: true, QueueID: 450, GameID: 113, Players: []gameplayLivePlayer{{HistoryState: "failed"}}}
	if gameplayLiveSnapshotComplete(response) {
		t.Fatal("transient history failure became immutable for the entire game")
	}
	for _, state := range []string{"empty", "unavailable", "ok"} {
		response.Players[0].HistoryState = state
		if !gameplayLiveSnapshotComplete(response) {
			t.Fatalf("terminal history state %s must not cause repeated reads", state)
		}
	}
}
