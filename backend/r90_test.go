package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR90RealShapeGroups(t *testing.T) {
	players, raw := r62ArenaFixturePlayers(t, 18)
	phase, id := "InProgress", int64(90001)
	server := r62ArenaGameflowServer(t, &phase, &id, players)
	a := &app{connected: true, lcu: &LCUClient{baseURL: server.URL, token: "test", http: server.Client()}, liveClientPlayerList: func(context.Context) ([]byte, int, error) { return raw, http.StatusOK, nil }}
	got := r62GameplayLiveResponse(t, a)
	if got.ArenaGrouped || got.ArenaMascotMapping || !got.ArenaGroupingUnavailable {
		t.Fatal("order is not squad evidence")
	}
	for _, p := range got.Players {
		if p.ArenaGroup != "" {
			t.Fatal("guessed group", p.ArenaGroup)
		}
	}
}

func TestR90OrderInferenceNeverEnablesMascotMapping(t *testing.T) {
	for _, field := range []string{"subteamId", "playerSubteamId"} {
		t.Run(field, func(t *testing.T) {
			// Deliberately satisfy the downstream mascot predicate. Real order
			// fixtures currently use Field="order", which masks a removed !order guard.
			grouping := liveClientArenaGrouping{
				Field: field,
				ByIdentity: map[string]string{
					"player-1": "1", "player-2": "1", "player-3": "1",
					"player-4": "6", "player-5": "6", "player-6": "6",
				},
			}
			if !arenaLiveClientMascotMapping(grouping) {
				t.Fatal("fixture must pass mascot metadata checks so the order guard is exercised")
			}
			for _, tc := range []struct {
				name           string
				grouped, order bool
				want           bool
			}{
				{"inferred-blocks", true, true, false},
				{"explicit-client-subteams", true, false, true},
				{"ungrouped-client-subteams", false, false, false},
			} {
				t.Run(tc.name, func(t *testing.T) {
					if got := arenaLiveMascotMappingAllowed(tc.grouped, tc.order, grouping); got != tc.want {
						t.Fatalf("mascot mapping = %v, want %v (grouped=%v order=%v)", got, tc.want, tc.grouped, tc.order)
					}
				})
			}
		})
	}
}

func r90RememberAllies(a *app, start, size int) {
	players := make([]struct {
		player lcuLivePlayer
		team   int64
	}, size)
	for i := range players {
		players[i].player = lcuLivePlayer{PUUID: fmt.Sprintf("r90-player-identity-%02d", start+i+1), SummonerID: int64(start + i + 1)}
	}
	a.rememberArenaAllies(players)
}

func r90Fixture(t *testing.T, players []map[string]any, raw []byte) *app {
	t.Helper()
	phase, id := "InProgress", int64(90001)
	server := r62ArenaGameflowServer(t, &phase, &id, players)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0700); err != nil {
		t.Fatal(err)
	}
	return &app{connected: true, lcu: &LCUClient{baseURL: server.URL, token: "test", http: server.Client()}, summoner: Summoner{PUUID: "r90-player-identity-01"}, storage: trackTestStore(t, &localStore{root: root}), liveClientPlayerList: func(context.Context) ([]byte, int, error) { return raw, http.StatusOK, nil }}
}

func r90Events(t *testing.T, a *app, event string) []map[string]any {
	t.Helper()
	raw, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	events := []map[string]any{}
	for _, line := range bytes.Split(raw, []byte("\n")) {
		var e map[string]any
		if json.Unmarshal(line, &e) == nil && e["event"] == event {
			events = append(events, e)
		}
	}
	return events
}

func TestR90FixtureMatchesObservedFields(t *testing.T) {
	players, raw := r62ArenaFixturePlayers(t, 18)
	var entries []map[string]any
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	keys := strings.Fields("championName isBot isDead items level position rawChampionName rawSkinName respawnTimer riotId riotIdGameName riotIdTagLine runes scores skinID skinName summonerName summonerSpells team")
	for _, entry := range entries {
		if len(entry) != 19 {
			t.Fatal(entry)
		}
		for _, key := range keys {
			if _, ok := entry[key]; !ok {
				t.Fatal(key)
			}
		}
	}
	counts := map[int]int{}
	for _, p := range players {
		if p["summonerName"] != "" || len(p) != 8 {
			t.Fatal(p)
		}
		counts[p["teamParticipantId"].(int)]++
	}
	if !reflect.DeepEqual(counts, map[int]int{1: 2, 2: 2, 3: 1, 4: 2, 5: 6, 6: 1, 7: 1, 8: 1, 9: 1, 10: 1}) {
		t.Fatal(counts)
	}
}

func TestR90AlliesVerifyAndRejectScrambledPlayerlist(t *testing.T) {
	for _, scrambled := range []bool{false, true} {
		t.Run(fmt.Sprint(scrambled), func(t *testing.T) {
			players, raw := r62ArenaFixturePlayers(t, 18)
			if scrambled {
				var entries []map[string]any
				_ = json.Unmarshal(raw, &entries)
				entries[2], entries[3] = entries[3], entries[2]
				raw, _ = json.Marshal(entries)
			}
			a := r90Fixture(t, players, raw)
			r90RememberAllies(a, 0, 3)
			got := r62GameplayLiveResponse(t, a)
			if got.ArenaGrouped || !got.ArenaGroupingUnavailable {
				t.Fatal("order inference must stay disabled")
			}
			marked := 0
			for _, p := range got.Players {
				if p.MySquad {
					marked++
				}
				if p.ArenaGroup != "" {
					t.Fatal("guessed group")
				}
			}
			if marked != 3 {
				t.Fatal("known squad lost", marked)
			}
			events := r90Events(t, a, "arena_group_order_rejected")
			if len(events) != 1 || events[0]["reason"] != "order-inference-disabled" {
				t.Fatal(events)
			}
		})
	}
}

func TestR90SeventeenGameflowUsesEighteenPlayerlistBoundaries(t *testing.T) {
	for _, missing := range []int{1, 7, 17} {
		t.Run(fmt.Sprint(missing), func(t *testing.T) {
			players, raw := r62ArenaFixturePlayers(t, 18)
			players = append(players[:missing], players[missing+1:]...)
			a := r90Fixture(t, players, raw)
			r90RememberAllies(a, 0, 3)
			got := r62GameplayLiveResponse(t, a)
			if got.ArenaGrouped || got.ArenaMascotMapping {
				t.Fatal("partial roster guessed groups")
			}
			for _, p := range got.Players {
				if p.ArenaGroup != "" {
					t.Fatal("guessed group", p.ArenaGroup)
				}
			}
		})
	}
}

func TestR90QueueSquadSizesAndSessionFallback(t *testing.T) {
	for _, tc := range []struct {
		queue       int64
		count, size int
	}{{1700, 8, 2}, {1710, 16, 2}, {1750, 18, 3}, {1750, 12, 3}} {
		if arenaSquadSize(tc.queue) != tc.size {
			t.Fatal(tc)
		}
		raw := make([]lcuLivePlayer, tc.count)
		for i := range raw {
			raw[i].TeamParticipantID = 5
		}
		groups, ok := arenaSessionOrderGroups(tc.queue, raw)
		if !ok {
			t.Fatal(tc)
		}
		for i, g := range groups {
			if g != fmt.Sprint(i/tc.size+1) {
				t.Fatal(groups)
			}
		}
		if _, ok := arenaSessionOrderGroups(tc.queue, raw[:len(raw)-1]); ok {
			t.Fatal("partial block accepted")
		}
		if liveClientArenaDistribution(map[string]int{"1": 3, "2": 3}, 6, 2) {
			t.Fatal("three in a duo squad")
		}
	}
	players, raw := r62ArenaFixturePlayers(t, 18)
	a := r90Fixture(t, players, raw)
	r90RememberAllies(a, 0, 3)
	a.liveClientPlayerList = func(context.Context) ([]byte, int, error) { return nil, 0, errors.New("offline") }
	got := r62GameplayLiveResponse(t, a)
	if got.ArenaGrouped || !got.ArenaGroupingUnavailable || got.ArenaMascotMapping {
		t.Fatal("fallback", got.ArenaGroupSource)
	}
	players[2], players[3] = players[3], players[2]
	a.liveSnapshots.invalidate()
	got = r62GameplayLiveResponse(t, a)
	if got.ArenaGrouped || !got.ArenaGroupingUnavailable {
		t.Fatal("session fallback bypassed allies")
	}
}

func TestR90LiveSnapshotCacheLayersAndManualRefresh(t *testing.T) {
	for _, complete := range []bool{true, false} {
		t.Run(fmt.Sprint(complete), func(t *testing.T) {
			players, raw := r62ArenaFixturePlayers(t, 18)
			if !complete {
				players = players[:17]
			}
			a := r90Fixture(t, players, raw)
			// This test exercises complete/partial roster caching, not a failed
			// history read. Transient errors now correctly keep a snapshot retryable.
			transport := a.lcu.http.Transport
			if transport == nil {
				transport = http.DefaultTransport
			}
			a.lcu.http.Transport = gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if strings.Contains(req.URL.Path, "/lol-match-history/") {
					return response2351(map[string]any{"games": map[string]any{"gameCount": 0, "games": []any{}}}), nil
				}
				return transport.RoundTrip(req)
			})
			var probes atomic.Int32
			a.liveClientPlayerList = func(context.Context) ([]byte, int, error) {
				probes.Add(1)
				if !complete {
					return nil, 0, errors.New("not up")
				}
				return raw, 200, nil
			}
			first := r62GameplayLiveResponse(t, a)
			if first.ArenaGrouped {
				t.Fatal("unexpected grouping")
			}
			r62GameplayLiveResponse(t, a)
			if len(r90Events(t, a, "live_load_cost")) != 1 {
				t.Fatal("second HTTP request reassembled roster")
			}
			a.liveSnapshots.mu.Lock()
			a.liveSnapshots.at = time.Now().Add(-time.Hour)
			a.liveSnapshots.mu.Unlock()
			r62GameplayLiveResponse(t, a)
			want := 1
			if !complete {
				want = 2
			}
			if len(r90Events(t, a, "live_load_cost")) != want {
				t.Fatal("TTL layer", complete)
			}
			recorder := httptest.NewRecorder()
			a.handleGameplayLive(recorder, httptest.NewRequest("GET", "/api/gameplay/live?refresh=1", nil))
			if recorder.Code != 200 {
				t.Fatal(recorder.Code)
			}
			if len(r90Events(t, a, "live_load_cost")) != want+1 {
				t.Fatal("manual refresh used whole-game cache")
			}
			if complete && probes.Load() != 2 {
				t.Fatal("successful order payload probe was not cached", probes.Load())
			}
		})
	}
}

func r90TruthInfo() *riotMatchInfo {
	info := &riotMatchInfo{GameID: 90001, QueueID: 1750, GameMode: "CHERRY", MapID: 30}
	for i := 0; i < 18; i++ {
		info.Participants = append(info.Participants, riotParticipant{ParticipantID: int64(i + 1), PUUID: fmt.Sprintf("r90-player-identity-%02d", i+1), RiotIDGameName: fmt.Sprintf("R62Arena%02d", i+1), RiotIDTagline: "CN1", PlayerSubteamID: int64(i/3 + 1)})
	}
	return info
}

func TestR90TruthDiagnosticSeparatesPartitionAndBlockIdentity(t *testing.T) {
	for _, mode := range []string{"identity", "permuted", "wrong", "partial"} {
		t.Run(mode, func(t *testing.T) {
			players, raw := r62ArenaFixturePlayers(t, 17)
			a := r90Fixture(t, players, raw)
			r90RememberAllies(a, 0, 3)
			r62GameplayLiveResponse(t, a)
			info := r90TruthInfo()
			if mode == "permuted" {
				for i := range info.Participants {
					info.Participants[i].PlayerSubteamID = 7 - info.Participants[i].PlayerSubteamID
				}
			}
			if mode == "wrong" {
				info.Participants[2].PlayerSubteamID, info.Participants[3].PlayerSubteamID = info.Participants[3].PlayerSubteamID, info.Participants[2].PlayerSubteamID
			}
			a.checkArenaGroupTruth(&LCUClient{}, "", info)
			a.checkArenaGroupTruth(a.lcu, "different-server", info)
			if len(r90Events(t, a, "arena_group_truth_check")) != 0 {
				t.Fatal("cross client/server truth")
			}
			if mode == "partial" {
				partial := *info
				partial.Participants = info.Participants[:17]
				a.checkArenaGroupTruth(a.lcu, "", &partial)
				if events := r90Events(t, a, "arena_group_truth_check"); len(events) != 1 || events[0]["conclusive"] != false {
					t.Fatal("partial stats must be inconclusive")
				}
			}
			a.checkArenaGroupTruth(a.lcu, "", info)
			a.checkArenaGroupTruth(a.lcu, "", info)
			events := r90Events(t, a, "arena_group_truth_check")
			wantEvents := 1
			if mode == "partial" {
				wantEvents = 2
			}
			if len(events) != wantEvents {
				t.Fatal(events)
			}
			e := events[len(events)-1]
			if e["partition_match"] != (mode != "wrong") || e["block_to_subteam_identity"] != (mode == "identity" || mode == "partial") || e["player_count"] != float64(18) || e["verified_by_allies"] != false || e["my_squad_correct"] != (mode != "wrong") {
				t.Fatal(e)
			}
			if e["build_fingerprint"] != buildFingerprint {
				t.Fatal("fingerprint", e)
			}
			data, _ := json.Marshal(e)
			if strings.Contains(string(data), "r90-player") || strings.Contains(string(data), "R62Arena") {
				t.Fatal("identity leak")
			}
		})
	}
}

func TestR90QueueGuardScansNewGroupingModule(t *testing.T) {
	files, err := filepath.Glob("arena_live*.go")
	if err != nil || len(files) == 0 {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		var source any
		if replacement := os.Getenv("R90_QUEUE_GUARD_SOURCE"); replacement != "" && file == "arena_live_grouping.go" {
			raw, err := os.ReadFile(replacement)
			if err != nil {
				t.Fatal(err)
			}
			source = raw
		}
		if got := arenaLiteralComparisons(file, source); len(got) > 0 {
			t.Errorf("%s: queue literals at %v", file, got)
		}
	}
	if len(arenaLiteralComparisons("arena_live_mutant.go", `package main;func orderGroupsFromLiveClient(queueID int64)bool{return queueID==1750}`)) != 1 {
		t.Fatal("new grouping function escaped guard")
	}
}

func TestR90EndPhaseReadsSGPTruthOnce(t *testing.T) {
	players, raw := r62ArenaFixturePlayers(t, 18)
	a := r90Fixture(t, players, raw)
	a.lcu.platformProbe = true
	a.lcu.region = "TENCENT"
	a.lcu.rsoPlatform = "HN1"
	r90RememberAllies(a, 0, 3)
	r62GameplayLiveResponse(t, a)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/match-history-query/v1/products/lol/player/r90-player-identity-01/SUMMARY" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"games": []map[string]any{{"json": r90TruthInfo()}}})
	}))
	defer server.Close()
	provider := newSGPProvider()
	provider.http = server.Client()
	provider.serverBases["HN1"] = server.URL
	provider.token = "fixture"
	provider.tokenAt = time.Now()
	provider.tokenClient = a.lcu
	a.sgp = provider
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.observeGameplayPhase(ctx, a.lcu, "EndOfGame")
	deadline := time.Now().Add(2 * time.Second)
	hasTruth := func() bool {
		for _, e := range r90Events(t, a, "arena_group_truth_check") {
			if e["truth_source"] == "sgp" && e["conclusive"] == true {
				return true
			}
		}
		return false
	}
	for !hasTruth() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(r90Events(t, a, "arena_group_truth_check")) != 1 {
		t.Fatal("end phase did not compare SGP truth")
	}
	a.observeGameplayPhase(ctx, a.lcu, "EndOfGame")
	a.finishArenaGroupTruth(ctx, a.lcu)
	if requests.Load() != 1 {
		t.Fatal("completed truth check requested again", requests.Load())
	}
	a.clearGameplayReferences()
	a.arenaTruth.mu.Lock()
	count := len(a.arenaTruth.records)
	a.arenaTruth.mu.Unlock()
	if count != 0 {
		t.Fatal("account state not cleared")
	}
}

func TestR90SnapshotCompleteness(t *testing.T) {
	full := gameplayLiveResponse{Available: true, QueueID: 1750, GameMode: "CHERRY", ArenaGrouped: true, Players: []gameplayLivePlayer{{HistoryState: "ok"}}}
	if !gameplayLiveSnapshotComplete(full) {
		t.Fatal("complete grouped")
	}
	full.ArenaGrouped = false
	if gameplayLiveSnapshotComplete(full) {
		t.Fatal("unknown grouping")
	}
	full.ArenaGroupingUnavailable = true
	if !gameplayLiveSnapshotComplete(full) {
		t.Fatal("terminal grouping verdict")
	}
	full.Players[0].HistoryState = "pending"
	if gameplayLiveSnapshotComplete(full) {
		t.Fatal("pending history")
	}
	full.Players = nil
	if gameplayLiveSnapshotComplete(full) {
		t.Fatal("empty players")
	}
	full.Players = []gameplayLivePlayer{{}}
	full.Available = false
	if gameplayLiveSnapshotComplete(full) {
		t.Fatal("unavailable")
	}
}

func TestR90EndPhaseTransitionRestartsCanceledTruthRead(t *testing.T) {
	players, raw := r62ArenaFixturePlayers(t, 18)
	a := r90Fixture(t, players, raw)
	a.lcu.platformProbe = true
	a.lcu.region = "TENCENT"
	a.lcu.rsoPlatform = "HN1"
	r90RememberAllies(a, 0, 3)
	r62GameplayLiveResponse(t, a)
	started := make(chan struct{})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			close(started)
			<-r.Context().Done()
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"games": []map[string]any{{"json": r90TruthInfo()}}})
	}))
	defer server.Close()
	provider := newSGPProvider()
	provider.http = server.Client()
	provider.serverBases["HN1"] = server.URL
	provider.token = "fixture"
	provider.tokenAt = time.Now()
	provider.tokenClient = a.lcu
	a.sgp = provider
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.observeGameplayPhase(ctx, a.lcu, "WaitingForStats")
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first truth read did not start")
	}
	a.observeGameplayPhase(ctx, a.lcu, "EndOfGame")
	deadline := time.Now().Add(2 * time.Second)
	hasTruth := func() bool {
		for _, e := range r90Events(t, a, "arena_group_truth_check") {
			if e["truth_source"] == "sgp" && e["conclusive"] == true {
				return true
			}
		}
		return false
	}
	for !hasTruth() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !hasTruth() || requests.Load() != 2 {
		t.Fatal("end phase canceled truth without resuming", requests.Load())
	}
}
