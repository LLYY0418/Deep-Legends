package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
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
)

func lpTestClient(t *testing.T, gameID, queueID int64) *LCUClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/lol-gameflow/v1/session" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"gameData": map[string]any{"gameId": gameID, "queue": map[string]any{"id": queueID}},
		})
	}))
	t.Cleanup(server.Close)
	return &LCUClient{baseURL: server.URL, token: "test", http: server.Client()}
}

func captureLPObservations(tracker *lpTracker) *[]map[string]any {
	events := make([]map[string]any, 0, lpCaptureAttempts+3)
	tracker.observeEvent = func(event map[string]any) {
		copy := make(map[string]any, len(event))
		for key, value := range event {
			copy[key] = value
		}
		events = append(events, copy)
	}
	return &events
}

func lpObservation(events []map[string]any, name string) (map[string]any, bool) {
	for _, event := range events {
		if event["event"] == name {
			return event, true
		}
	}
	return nil, false
}

func lpObservationCount(events []map[string]any, name string) int {
	count := 0
	for _, event := range events {
		if event["event"] == name {
			count++
		}
	}
	return count
}

func assertLPObservationPrivacy(t *testing.T, events []map[string]any, forbiddenValues ...string) {
	t.Helper()
	allowedKeys := map[string]bool{
		"event": true, "reason": true, "stage": true, "has_baseline": true,
		"capture_index": true, "attempt": true, "attempts": true, "capability_state": true, "games_gap": true,
		"wins_delta": true, "losses_delta": true, "score_delta": true, "baseline_source": true, "baseline_age_s": true, "snapshot_source": true, "writer": true, "season_fallback": true,
	}
	for _, event := range events {
		for key := range event {
			if !allowedKeys[key] {
				t.Fatalf("LP diagnostic used unreviewed field %q: %#v", key, event)
			}
		}
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{`"game_id"`, `"queue_type"`, `"playerRef"`, `"puuid"`, `"accountHash"`, `"tier"`, `"division"`, `"league_points"`, `"wins"`, `"losses"`}
	forbidden = append(forbidden, forbiddenValues...)
	for _, value := range forbidden {
		if value != "" && strings.Contains(string(encoded), value) {
			t.Fatalf("LP diagnostic leaked %q: %s", value, encoded)
		}
	}
}

func seededLPTracker(t *testing.T, playerRef, queueType string, baseline lpSnapshot) (*lpTracker, string) {
	t.Helper()
	store := trackTestStore(t, &localStore{root: t.TempDir(), salt: bytes.Repeat([]byte{9}, 32)})
	tracker := newLPTracker(store)
	tracker.sleep = func(time.Duration) {}
	accountHash := tracker.accountHash(playerRef)
	tracker.history.Baselines[accountHash] = map[string]lpSnapshot{queueType: baseline}
	return tracker, accountHash
}

func TestRankAbsoluteScore(t *testing.T) {
	cases := []struct {
		tier     string
		division string
		lp       int
		want     int
		ok       bool
	}{
		{"IRON", "IV", 0, 0, true},
		{"iron", "III", 40, 140, true},
		{"GOLD", "II", 55, 1455, true},
		{"EMERALD", "I", 99, 2399, true},
		{"DIAMOND", "IV", 0, 2400, true},
		// 大师及以上不细分小段，胜点直接累加。
		{"MASTER", "I", 120, 2920, true},
		{"CHALLENGER", "", 1043, 3843, true},
		{"", "", 10, 0, false},
		{"UNRANKED", "IV", 10, 0, false},
	}
	for _, item := range cases {
		got, ok := rankAbsoluteScore(item.tier, item.division, item.lp)
		if ok != item.ok || got != item.want {
			t.Fatalf("rankAbsoluteScore(%q,%q,%d) = %d,%v want %d,%v", item.tier, item.division, item.lp, got, ok, item.want, item.ok)
		}
	}
}

func TestRankFromScoreRoundTrip(t *testing.T) {
	tier, division := rankFromScore(1455)
	if tier != "GOLD" || division != "II" {
		t.Fatalf("rankFromScore(1455) = %s %s", tier, division)
	}
	tier, division = rankFromScore(3200)
	if tier != "MASTER" || division != "" {
		t.Fatalf("rankFromScore(3200) = %s %s", tier, division)
	}
	tier, division = rankFromScore(0)
	if tier != "IRON" || division != "IV" {
		t.Fatalf("rankFromScore(0) = %s %s", tier, division)
	}
}

func TestLPSnapshotTrustworthy(t *testing.T) {
	for _, test := range []struct {
		name     string
		snapshot lpSnapshot
		want     bool
	}{
		{name: "complete", snapshot: lpSnapshot{Wins: 12, Losses: 8}, want: true},
		{name: "missing losses", snapshot: lpSnapshot{Wins: 107, Losses: 0}, want: false},
		{name: "unplayed", snapshot: lpSnapshot{}, want: true},
		{name: "losses only", snapshot: lpSnapshot{Losses: 1}, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := test.snapshot.trustworthy(); got != test.want {
				t.Fatalf("trustworthy() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestLPTrackerObserveRejectsIncompleteSnapshot(t *testing.T) {
	playerRef := strings.Repeat("o", 48)
	queueType := "RANKED_SOLO_5x5"
	baseline := lpSnapshot{Tier: "EMERALD", Division: "II", LeaguePoints: 47, Wins: 60, Losses: 40}
	tracker, accountHash := seededLPTracker(t, playerRef, queueType, baseline)
	events := captureLPObservations(tracker)

	tracker.observe(playerRef, []gameplayRank{{QueueType: queueType, Tier: "EMERALD", Division: "II", LeaguePoints: 71, Wins: 107, Losses: 0}})
	if got := tracker.history.Baselines[accountHash][queueType]; got != baseline {
		t.Fatalf("incomplete snapshot replaced baseline: %#v", got)
	}
	rejected, ok := lpObservation(*events, "lp_snapshot_rejected")
	if !ok || rejected["stage"] != "observe" || rejected["reason"] != "missing_losses" {
		t.Fatalf("rejection observation = %#v", rejected)
	}
	assertLPObservationPrivacy(t, *events, playerRef, accountHash, queueType)
}

func TestLPTrackerCaptureRecordsDiagnostics(t *testing.T) {
	playerRef := strings.Repeat("c", 48)
	queueType := "RANKED_SOLO_5x5"
	baseline := lpSnapshot{Tier: "EMERALD", Division: "II", LeaguePoints: 47, Wins: 60, Losses: 40}
	tracker, accountHash := seededLPTracker(t, playerRef, queueType, baseline)
	events := captureLPObservations(tracker)
	client := lpTestClient(t, 90003, 420)

	tracker.capture(client, playerRef, func() ([]gameplayRank, EndpointCapability) {
		return []gameplayRank{{QueueType: queueType, Tier: "EMERALD", Division: "II", LeaguePoints: 71, Wins: 61, Losses: 40}}, EndpointCapability{State: capabilityAvailable}
	})
	if got := tracker.history.Games["90003"].Delta; got != 24 {
		t.Fatalf("recorded delta = %d, want 24", got)
	}
	started, startedOK := lpObservation(*events, "lp_capture_started")
	poll, pollOK := lpObservation(*events, "lp_capture_poll")
	recorded, recordedOK := lpObservation(*events, "lp_capture_recorded")
	captureIndex, captureIndexOK := started["capture_index"].(uint64)
	if !startedOK || !captureIndexOK || captureIndex != 1 || started["has_baseline"] != true {
		t.Fatalf("started observation = %#v", started)
	}
	if !pollOK || poll["capture_index"] != captureIndex || poll["attempt"] != 1 || poll["capability_state"] != capabilityAvailable {
		t.Fatalf("poll observation = %#v", poll)
	}
	if !recordedOK || recorded["capture_index"] != captureIndex {
		t.Fatalf("recorded observation = %#v", recorded)
	}
	if _, skipped := lpObservation(*events, "lp_capture_skipped"); skipped {
		t.Fatalf("recorded capture was also skipped: %#v", *events)
	}
	assertLPObservationPrivacy(t, *events, playerRef, accountHash, queueType, "90003")
}

func TestLPTrackerCaptureTimeoutDiagnostics(t *testing.T) {
	playerRef := strings.Repeat("t", 48)
	queueType := "RANKED_FLEX_SR"
	baseline := lpSnapshot{Tier: "GOLD", Division: "I", LeaguePoints: 20, Wins: 30, Losses: 20}
	tracker, _ := seededLPTracker(t, playerRef, queueType, baseline)
	events := captureLPObservations(tracker)
	calls := 0

	tracker.capture(lpTestClient(t, 90004, 440), playerRef, func() ([]gameplayRank, EndpointCapability) {
		calls++
		return []gameplayRank{{QueueType: queueType, Tier: "GOLD", Division: "I", LeaguePoints: 20, Wins: 30, Losses: 20}}, EndpointCapability{State: capabilityAvailable}
	})
	if calls != lpCaptureAttempts || lpObservationCount(*events, "lp_capture_poll") != lpCaptureAttempts {
		t.Fatalf("calls=%d polls=%d, want %d", calls, lpObservationCount(*events, "lp_capture_poll"), lpCaptureAttempts)
	}
	started, startedOK := lpObservation(*events, "lp_capture_started")
	captureIndex, captureIndexOK := started["capture_index"].(uint64)
	if !startedOK || !captureIndexOK || captureIndex != 1 {
		t.Fatalf("started observation = %#v", started)
	}
	for _, event := range *events {
		if event["event"] == "lp_capture_poll" && event["capture_index"] != captureIndex {
			t.Fatalf("poll lacks capture identity: %#v", event)
		}
	}
	timedOut, ok := lpObservation(*events, "lp_capture_timeout")
	if !ok || timedOut["capture_index"] != captureIndex || timedOut["attempts"] != lpCaptureAttempts {
		t.Fatalf("timeout observation = %#v", timedOut)
	}
	if _, recorded := lpObservation(*events, "lp_capture_recorded"); recorded {
		t.Fatalf("unchanged snapshot was recorded: %#v", *events)
	}
	if _, skipped := lpObservation(*events, "lp_capture_skipped"); skipped {
		t.Fatalf("unchanged snapshot was skipped instead of timing out: %#v", *events)
	}
	assertLPObservationPrivacy(t, *events, playerRef, queueType, "90004")
}

func TestLPTrackerCaptureRejectsIncompleteSnapshot(t *testing.T) {
	playerRef := strings.Repeat("r", 48)
	queueType := "RANKED_SOLO_5x5"
	baseline := lpSnapshot{Tier: "PLATINUM", Division: "I", LeaguePoints: 80, Wins: 50, Losses: 50}
	tracker, accountHash := seededLPTracker(t, playerRef, queueType, baseline)
	events := captureLPObservations(tracker)

	tracker.capture(lpTestClient(t, 90005, 420), playerRef, func() ([]gameplayRank, EndpointCapability) {
		return []gameplayRank{{QueueType: queueType, Tier: "EMERALD", Division: "IV", LeaguePoints: 10, Wins: 101, Losses: 0}}, EndpointCapability{State: capabilityAvailable}
	})
	if got := tracker.history.Baselines[accountHash][queueType]; got != baseline {
		t.Fatalf("rejected capture replaced baseline: %#v", got)
	}
	if lpObservationCount(*events, "lp_snapshot_rejected") != lpCaptureAttempts {
		t.Fatalf("rejected observations = %d, want %d", lpObservationCount(*events, "lp_snapshot_rejected"), lpCaptureAttempts)
	}
	if _, recorded := tracker.history.Games["90005"]; recorded {
		t.Fatal("incomplete snapshot produced an LP record")
	}
	if _, timedOut := lpObservation(*events, "lp_capture_timeout"); !timedOut {
		t.Fatalf("rejected capture did not time out: %#v", *events)
	}
	if _, skipped := lpObservation(*events, "lp_capture_skipped"); skipped {
		t.Fatalf("rejected snapshot was treated as a terminal skip: %#v", *events)
	}
	assertLPObservationPrivacy(t, *events, playerRef, accountHash, queueType, "90005")
}

func TestLPTrackerCaptureReportsSkippedReasons(t *testing.T) {
	queueType := "RANKED_SOLO_5x5"
	tests := []struct {
		name           string
		playerChar     string
		gameID         int64
		baseline       lpSnapshot
		removeBaseline bool
		current        gameplayRank
		reason         string
		baselineGames  any
		currentGames   int
	}{
		{
			name: "no baseline", playerChar: "n", gameID: 90101, removeBaseline: true,
			current: gameplayRank{QueueType: queueType, Tier: "GOLD", Division: "II", LeaguePoints: 30, Wins: 12, Losses: 8},
			reason:  "no_baseline", currentGames: 20,
		},
		{
			name: "games jumped", playerChar: "j", gameID: 90102,
			baseline: lpSnapshot{Tier: "EMERALD", Division: "II", LeaguePoints: 47, Wins: 60, Losses: 40},
			current:  gameplayRank{QueueType: queueType, Tier: "EMERALD", Division: "I", LeaguePoints: 2, Wins: 62, Losses: 40},
			reason:   "games_jumped", baselineGames: 100, currentGames: 102,
		},
		{
			name: "score unresolved", playerChar: "s", gameID: 90103,
			baseline: lpSnapshot{Tier: "EMERALD", Division: "II", LeaguePoints: 47, Wins: 60, Losses: 40},
			current:  gameplayRank{QueueType: queueType, Tier: "UNKNOWN", Division: "I", LeaguePoints: 71, Wins: 61, Losses: 40},
			reason:   "score_unresolved", baselineGames: 100, currentGames: 101,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			playerRef := strings.Repeat(test.playerChar, 48)
			tracker, accountHash := seededLPTracker(t, playerRef, queueType, test.baseline)
			if test.removeBaseline {
				delete(tracker.history.Baselines, accountHash)
			}
			events := captureLPObservations(tracker)
			tracker.capture(lpTestClient(t, test.gameID, 420), playerRef, func() ([]gameplayRank, EndpointCapability) {
				return []gameplayRank{test.current}, EndpointCapability{State: capabilityAvailable}
			})

			skipped, ok := lpObservation(*events, "lp_capture_skipped")
			started, startedOK := lpObservation(*events, "lp_capture_started")
			captureIndex, captureIndexOK := started["capture_index"].(uint64)
			if !startedOK || !captureIndexOK || captureIndex == 0 || !ok || lpObservationCount(*events, "lp_capture_skipped") != 1 || skipped["reason"] != test.reason || skipped["capture_index"] != captureIndex {
				t.Fatalf("skipped observation = %#v", skipped)
			}
			if test.reason == "games_jumped" && skipped["games_gap"] != 2 {
				t.Fatalf("games gap = %#v, want 2", skipped["games_gap"])
			}
			if _, recorded := lpObservation(*events, "lp_capture_recorded"); recorded {
				t.Fatalf("skipped capture was recorded: %#v", *events)
			}
			if _, timedOut := lpObservation(*events, "lp_capture_timeout"); timedOut {
				t.Fatalf("terminal skip also timed out: %#v", *events)
			}
			if got, want := tracker.history.Baselines[accountHash][queueType], lpSnapshotFromRank(test.current); got != want {
				t.Fatalf("refreshed baseline = %#v, want %#v", got, want)
			}
			if _, recorded := tracker.history.Games[strconv.FormatInt(test.gameID, 10)]; recorded {
				t.Fatal("skipped capture created an LP game record")
			}
			assertLPObservationPrivacy(t, *events, playerRef, accountHash, queueType, strconv.FormatInt(test.gameID, 10))
		})
	}
}

func TestLPTrackerCaptureReportsEarlyExitReasons(t *testing.T) {
	tracker := newLPTracker(nil)
	events := captureLPObservations(tracker)
	tracker.capture(nil, "", func() ([]gameplayRank, EndpointCapability) { return nil, EndpointCapability{} })
	ignored, ok := lpObservation(*events, "lp_capture_ignored")
	if !ok || ignored["reason"] != "invalid_player_reference" {
		t.Fatalf("invalid reference observation = %#v", ignored)
	}

	playerRef := strings.Repeat("e", 48)
	tracker, _ = seededLPTracker(t, playerRef, "RANKED_SOLO_5x5", lpSnapshot{Tier: "GOLD", Division: "I", LeaguePoints: 20, Wins: 20, Losses: 20})
	events = captureLPObservations(tracker)
	tracker.capture(lpTestClient(t, 90201, 450), playerRef, func() ([]gameplayRank, EndpointCapability) { return nil, EndpointCapability{} })
	ignored, ok = lpObservation(*events, "lp_capture_ignored")
	if !ok || ignored["reason"] != "unranked_queue" {
		t.Fatalf("unranked observation = %#v", ignored)
	}
	assertLPObservationPrivacy(t, *events, playerRef, "RANKED_SOLO_5x5", "90201")
}

func TestLPTrackerObserveAnnotatePersist(t *testing.T) {
	root := t.TempDir()
	store := trackTestStore(t, &localStore{root: root, salt: bytes.Repeat([]byte{7}, 32)})
	tracker := newLPTracker(store)
	playerRef := strings.Repeat("a", 48)
	accountHash := store.accountHash(Summoner{PUUID: playerRef})

	// 基线：翡翠 II 47 LP，共 100 场。
	tracker.observe(playerRef, []gameplayRank{{QueueType: "RANKED_SOLO_5x5", Tier: "EMERALD", Division: "II", LeaguePoints: 47, Wins: 60, Losses: 40}})

	// 模拟一场结算后的差值记录（+24 LP），并验证标注与持久化。
	tracker.mu.Lock()
	tracker.history.Games["90001"] = lpGameRecord{AccountHash: accountHash, QueueType: "RANKED_SOLO_5x5", Delta: 24, RecordedAt: 1}
	tracker.persistLocked()
	tracker.mu.Unlock()

	matches := []gameplayMatch{{GameID: 90001, QueueID: 420}, {GameID: 90002, QueueID: 420}}
	tracker.annotate(matches, playerRef)
	if matches[0].LpDelta == nil || *matches[0].LpDelta != 24 {
		t.Fatalf("match 90001 lpDelta = %v, want 24", matches[0].LpDelta)
	}
	if matches[1].LpDelta != nil {
		t.Fatalf("match 90002 lpDelta should be nil")
	}

	// 其他玩家的战绩不能被标注。
	other := []gameplayMatch{{GameID: 90001, QueueID: 420}}
	tracker.annotate(other, strings.Repeat("b", 48))
	if other[0].LpDelta != nil {
		t.Fatalf("lpDelta should not leak to other players")
	}

	historyPath := filepath.Join(root, lpHistoryFile)
	if _, err := os.Stat(historyPath); err != nil {
		t.Fatalf("history file missing: %v", err)
	}
	data, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(playerRef)) || bytes.Contains(data, []byte(`"playerRef"`)) {
		t.Fatalf("history leaked the stable player reference: %s", data)
	}
	if !bytes.Contains(data, []byte(accountHash)) {
		t.Fatalf("history is missing the salted account hash: %s", data)
	}
	reloaded := newLPTracker(store)
	if got := reloaded.history.Games["90001"].Delta; got != 24 {
		t.Fatalf("reloaded delta = %d, want 24", got)
	}
	if got := reloaded.history.Baselines[accountHash]["RANKED_SOLO_5x5"].LeaguePoints; got != 47 {
		t.Fatalf("reloaded baseline lp = %d, want 47", got)
	}
}

func TestLPTrackerInvalidatesLegacyPlaintextHistory(t *testing.T) {
	root := t.TempDir()
	store := trackTestStore(t, &localStore{root: root, salt: bytes.Repeat([]byte{3}, 32)})
	playerRef := strings.Repeat("p", 48)
	legacy := []byte(`{"schemaVersion":1,"baselines":{"` + playerRef + `":{"RANKED_SOLO_5x5":{"tier":"GOLD","division":"I","leaguePoints":20,"wins":10,"losses":8}}},"games":{"42":{"accountHash":"` + playerRef + `","queueType":"RANKED_SOLO_5x5","delta":18,"recordedAt":1}}}`)
	historyPath := filepath.Join(root, lpHistoryFile)
	if err := os.WriteFile(historyPath, legacy, 0o600); err != nil {
		t.Fatal(err)
	}

	tracker := newLPTracker(store)
	if len(tracker.history.Baselines) != 0 || len(tracker.history.Games) != 0 {
		t.Fatalf("legacy history was retained: %#v", tracker.history)
	}
	data, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(playerRef)) || bytes.Contains(data, []byte(`"playerRef"`)) {
		t.Fatalf("legacy stable player reference remains on disk: %s", data)
	}
	var rewritten lpHistoryData
	if err := json.Unmarshal(data, &rewritten); err != nil || rewritten.SchemaVersion != lpHistorySchemaVersion {
		t.Fatalf("rewritten history = %#v, %v", rewritten, err)
	}
}

func TestLPHistoryRejectsMoreThanTheRetentionLimit(t *testing.T) {
	history := lpHistoryData{
		SchemaVersion: lpHistorySchemaVersion,
		Baselines:     make(map[string]map[string]lpSnapshot),
		Games:         make(map[string]lpGameRecord, lpHistoryLimit+1),
	}
	for index := 0; index < lpHistoryLimit; index++ {
		history.Games[strconv.Itoa(index)] = lpGameRecord{AccountHash: "0123456789abcdef"}
	}
	if !validLPHistory(history) {
		t.Fatal("history at the retention limit was rejected")
	}
	history.Games["overflow"] = lpGameRecord{AccountHash: "0123456789abcdef"}
	if validLPHistory(history) {
		t.Fatal("history above the retention limit was accepted")
	}
}

func TestItemSlotsKeepPositions(t *testing.T) {
	got := itemSlots(6630, 0, 3053, -1, 0, 0, 3340)
	want := []int64{6630, 0, 3053, 0, 0, 0, 3340}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("slot %d = %d, want %d", index, got[index], want[index])
		}
	}
}

func r182Rank(snapshot lpSnapshot, queue string) gameplayRank {
	return gameplayRank{QueueType: queue, Tier: snapshot.Tier, Division: snapshot.Division, LeaguePoints: snapshot.LeaguePoints, Wins: snapshot.Wins, Losses: snapshot.Losses}
}
func r182Start(t *testing.T) (*lpTracker, string, string, lpSnapshot, *LCUClient) {
	t.Helper()
	ref := strings.Repeat("b", 48)
	queue := "RANKED_SOLO_5x5"
	baseline := lpSnapshot{Tier: "GOLD", Division: "II", LeaguePoints: 80, Wins: 10, Losses: 10}
	tracker, hash := seededLPTracker(t, ref, queue, baseline)
	client := lpTestClient(t, 182001, 420)
	tracker.takeGameStart(client, ref, func() ([]gameplayRank, EndpointCapability) {
		return []gameplayRank{r182Rank(baseline, queue)}, EndpointCapability{State: capabilityAvailable, Path: "/lol-ranked/v1/current-ranked-stats"}
	})
	if tracker.history.GameStarts[hash][queue].GameID != 182001 {
		t.Fatal("start not saved")
	}
	return tracker, ref, queue, baseline, client
}
func r182Capture(tracker *lpTracker, client *LCUClient, ref, queue string, snapshots []lpSnapshot, sources []string) int {
	calls := 0
	tracker.capture(client, ref, func() ([]gameplayRank, EndpointCapability) {
		i := min(calls, len(snapshots)-1)
		source := "lcu"
		if len(sources) > 0 {
			source = sources[min(calls, len(sources)-1)]
		}
		calls++
		return []gameplayRank{r182Rank(snapshots[i], queue)}, EndpointCapability{State: capabilityAvailable, Path: source + ":ranked"}
	})
	return calls
}
func TestR182LossGapTwoUsesGameStartScore(t *testing.T) {
	tracker, ref, queue, baseline, client := r182Start(t)
	events := captureLPObservations(tracker)
	after := baseline
	after.Losses += 2
	after.LeaguePoints -= 18
	calls := r182Capture(tracker, client, ref, queue, []lpSnapshot{after}, nil)
	recorded, ok := lpObservation(*events, "lp_capture_recorded")
	if !ok || tracker.history.Games["182001"].Delta != -18 || recorded["games_gap"] != 2 || recorded["baseline_source"] != "game_start" || recorded["snapshot_source"] != "lcu" || recorded["wins_delta"] != 0 || recorded["losses_delta"] != 2 || recorded["score_delta"] != -18 || calls != 3 || lpObservationCount(*events, "lp_capture_settle") != 2 {
		t.Fatal(tracker.history.Games, calls, *events)
	}
	hash := tracker.accountHash(ref)
	if _, ok := tracker.history.GameStarts[hash][queue]; ok {
		t.Fatal("finished start baseline retained")
	}
	assertLPObservationPrivacy(t, *events, ref, hash, queue, "182001")
}
func TestR182ProjectedLossWinWithUnchangedGames(t *testing.T) {
	tracker, ref, queue, baseline, client := r182Start(t)
	events := captureLPObservations(tracker)
	after := baseline
	after.Wins++
	after.Losses--
	after.LeaguePoints += 20
	r182Capture(tracker, client, ref, queue, []lpSnapshot{after}, nil)
	event, ok := lpObservation(*events, "lp_capture_recorded")
	if !ok || tracker.history.Games["182001"].Delta != 20 || event["games_gap"] != 0 || event["wins_delta"] != 1 || event["losses_delta"] != -1 {
		t.Fatal(tracker.history.Games, *events)
	}
}
func TestR182ScoreSettlesBeforeDelayedGameCount(t *testing.T) {
	for _, test := range []struct {
		name  string
		lp    []int
		delta int
		calls int
	}{{"score-first", []int{62, 62, 62}, -18, 3}, {"changes-twice", []int{62, 60, 58, 58}, -22, 4}, {"never-stable", []int{62, 60, 58, 56}, 0, 4}} {
		t.Run(test.name, func(t *testing.T) {
			tracker, ref, queue, baseline, client := r182Start(t)
			events := captureLPObservations(tracker)
			snaps := []lpSnapshot{}
			for i, lp := range test.lp {
				s := baseline
				s.LeaguePoints = lp
				if i > 0 {
					s.Losses += 2
				}
				snaps = append(snaps, s)
			}
			calls := r182Capture(tracker, client, ref, queue, snaps, nil)
			record, recorded := tracker.history.Games["182001"]
			if calls != test.calls {
				t.Fatal(calls, *events)
			}
			if test.delta == 0 {
				event, ok := lpObservation(*events, "lp_capture_skipped")
				if recorded || !ok || event["reason"] != "score_unstable" {
					t.Fatal(*events)
				}
			} else if !recorded || record.Delta != test.delta {
				t.Fatal(record, *events)
			}
		})
	}
}
func TestR182ProtectionTimeoutAndSourceMismatch(t *testing.T) {
	t.Run("unchanged", func(t *testing.T) {
		tracker, ref, queue, baseline, client := r182Start(t)
		events := captureLPObservations(tracker)
		calls := r182Capture(tracker, client, ref, queue, []lpSnapshot{baseline}, nil)
		if _, ok := tracker.history.Games["182001"]; ok {
			t.Fatal("unchanged recorded")
		}
		if calls != 18 || lpObservationCount(*events, "lp_capture_timeout") != 1 || lpObservationCount(*events, "lp_capture_settle") != 0 {
			t.Fatal(calls, *events)
		}
	})
	t.Run("same-source-unchanged-after-fallback", func(t *testing.T) {
		tracker, ref, queue, baseline, client := r182Start(t)
		events := captureLPObservations(tracker)
		calls := r182Capture(tracker, client, ref, queue, []lpSnapshot{baseline}, []string{"sgp", "lcu"})
		if calls != 18 || lpObservationCount(*events, "lp_capture_timeout") != 1 || lpObservationCount(*events, "lp_capture_skipped") != 0 || len(tracker.history.Games) != 0 {
			t.Fatal(calls, tracker.history.Games, *events)
		}
	})

	for _, allMismatch := range []bool{false, true} {
		t.Run(strconv.FormatBool(allMismatch), func(t *testing.T) {
			tracker, ref, queue, baseline, client := r182Start(t)
			events := captureLPObservations(tracker)
			after := baseline
			after.Losses += 2
			after.LeaguePoints -= 18
			sources := []string{"sgp", "sgp", "lcu"}
			if allMismatch {
				sources = []string{"sgp"}
			}
			calls := r182Capture(tracker, client, ref, queue, []lpSnapshot{after}, sources)
			if allMismatch {
				event, ok := lpObservation(*events, "lp_capture_skipped")
				if !ok || event["reason"] != "source_mismatch" || event["score_delta"] != nil || calls != 18 {
					t.Fatal(calls, *events)
				}
				if _, ok := tracker.history.Games["182001"]; ok {
					t.Fatal("cross-source score recorded")
				}
			} else if calls != 5 || tracker.history.Games["182001"].Delta != -18 {
				t.Fatal(calls, tracker.history.Games, *events)
			}
		})
	}
}
func TestR182ObserveCannotReplaceGameStart(t *testing.T) {
	tracker, ref, queue, baseline, client := r182Start(t)
	hash := tracker.accountHash(ref)
	before := tracker.history.GameStarts[hash][queue]
	observed := baseline
	observed.LeaguePoints = 99
	observed.Wins++
	tracker.observe(ref, []gameplayRank{r182Rank(observed, queue)}, EndpointCapability{Path: "lcu:ranked"})
	if tracker.history.GameStarts[hash][queue] != before || tracker.history.Baselines[hash][queue] != observed {
		t.Fatal("observe replaced immutable game baseline", tracker.history)
	}
	after := baseline
	after.LeaguePoints -= 18
	after.Losses += 2
	r182Capture(tracker, client, ref, queue, []lpSnapshot{after}, nil)
	if tracker.history.Games["182001"].Delta != -18 {
		t.Fatal(tracker.history.Games)
	}
}
func TestR182SeasonFallbackSkippedAndDiagnosticsDedup(t *testing.T) {
	tracker, ref, queue, baseline, _ := r182Start(t)
	events := captureLPObservations(tracker)
	hash := tracker.accountHash(ref)
	clock := time.Unix(1820000000, 0)
	tracker.now = func() time.Time { return clock }
	raw := r182Rank(baseline, queue)
	raw.Wins = 99
	raw.Losses = 0
	raw.WinRate = -1
	a := &app{}
	filled, _ := a.applySeasonRankWinRateFallback([]gameplayRank{raw}, EndpointCapability{}, seasonStatsProgress{Complete: true}, map[int64]gameplayAggregate{420: {Games: 25, Wins: 12, Losses: 13, WinRate: 48}})
	if !filled[0].seasonFallback {
		t.Fatal("fallback provenance missing")
	}
	for i := 0; i < 3; i++ {
		tracker.observe(ref, filled)
	}
	if tracker.history.Baselines[hash][queue] != baseline {
		t.Fatal("season counts wrote baseline", tracker.history)
	}
	event, ok := lpObservation(*events, "lp_baseline_written")
	if !ok || event["writer"] != "observe" || event["season_fallback"] != true || event["reason"] != "season_fallback" || lpObservationCount(*events, "lp_baseline_written") != 1 {
		t.Fatal(*events)
	}
	clock = clock.Add(61 * time.Second)
	tracker.observe(ref, filled)
	if lpObservationCount(*events, "lp_baseline_written") != 2 {
		t.Fatal(*events)
	}
	// A trustworthy untouched queue can still write, and unchanged writes are bounded.
	flex := r182Rank(baseline, "RANKED_FLEX_SR")
	for i := 0; i < 3; i++ {
		tracker.observe(ref, []gameplayRank{flex})
	}
	if tracker.history.Baselines[hash][flex.QueueType] != baseline || lpObservationCount(*events, "lp_baseline_written") != 3 {
		t.Fatal(*events, tracker.history)
	}
	assertLPObservationPrivacy(t, *events, ref, hash, queue)
}
func TestR182HistorySchemaOneCompatibilityAndStartRoundTrip(t *testing.T) {
	ref := strings.Repeat("v", 48)
	tracker, hash := seededLPTracker(t, ref, "RANKED_SOLO_5x5", lpSnapshot{Tier: "GOLD", Division: "II", LeaguePoints: 80, Wins: 10, Losses: 10})
	old := tracker.history
	old.SchemaVersion = 1
	old.Games["181001"] = lpGameRecord{AccountHash: hash, QueueType: "RANKED_SOLO_5x5", Delta: -18, RecordedAt: 1}
	old.GameStarts = nil
	old.BaselineInfo = nil
	data, _ := json.Marshal(old)
	if err := os.WriteFile(filepath.Join(tracker.store.root, lpHistoryFile), data, 0600); err != nil {
		t.Fatal(err)
	}
	reloaded := newLPTracker(tracker.store)
	reloaded.sleep = func(time.Duration) {}
	if reloaded.history.Games["181001"].Delta != -18 || len(reloaded.history.GameStarts) != 0 || reloaded.history.SchemaVersion != 2 {
		t.Fatal(reloaded.history)
	}
	snapshot := old.Baselines[hash]["RANKED_SOLO_5x5"]
	reloaded.takeGameStart(lpTestClient(t, 182001, 420), ref, func() ([]gameplayRank, EndpointCapability) {
		return []gameplayRank{r182Rank(snapshot, "RANKED_SOLO_5x5")}, EndpointCapability{State: capabilityAvailable, Path: "sgp:ranked"}
	})
	final := newLPTracker(tracker.store)
	if !reflect.DeepEqual(final.history, reloaded.history) || final.history.GameStarts[hash]["RANKED_SOLO_5x5"].Source != "sgp" || final.history.Games["181001"].Delta != -18 {
		t.Fatal(final.history, reloaded.history)
	}
	data, _ = os.ReadFile(filepath.Join(tracker.store.root, lpHistoryFile))
	if bytes.Contains(data, []byte(ref)) {
		t.Fatal("persistent raw player reference")
	}
}
func TestR182StartRetryBudgetAndPhaseDedup(t *testing.T) {
	tracker, ref, queue, baseline, _ := r182Start(t)
	hash := tracker.accountHash(ref)
	delete(tracker.history.GameStarts[hash], queue)
	tracker.startSeen = map[string]bool{}
	events := captureLPObservations(tracker)
	calls := 0
	waits := []time.Duration{}
	tracker.sleep = func(d time.Duration) { waits = append(waits, d) }
	client := lpTestClient(t, 182002, 420)
	tracker.takeGameStart(client, ref, func() ([]gameplayRank, EndpointCapability) {
		calls++
		if calls < 3 {
			return nil, EndpointCapability{State: capabilityFailed}
		}
		return []gameplayRank{r182Rank(baseline, queue)}, EndpointCapability{State: capabilityAvailable, Path: "lcu:ranked"}
	})
	tracker.takeGameStart(client, ref, func() ([]gameplayRank, EndpointCapability) {
		t.Error("duplicated start read")
		return nil, EndpointCapability{}
	})
	if calls != 3 || !reflect.DeepEqual(waits, []time.Duration{5 * time.Second, 5 * time.Second}) || tracker.history.GameStarts[hash][queue].GameID != 182002 {
		t.Fatal(calls, waits, tracker.history)
	}
	delete(tracker.history.GameStarts[hash], queue)
	calls = 0
	tracker.takeGameStart(lpTestClient(t, 182003, 420), ref, func() ([]gameplayRank, EndpointCapability) {
		calls++
		return nil, EndpointCapability{State: capabilityFailed}
	})
	if calls != 3 || len(tracker.history.GameStarts[hash]) != 0 {
		t.Fatal(calls, tracker.history)
	}
	event, ok := lpObservation(*events, "lp_baseline_written")
	if !ok {
		t.Fatal(*events)
	}
	_ = event
	if _, ok := lpObservation(*events, "lp_game_start_poll"); !ok {
		t.Fatal(*events)
	}
}
func TestR182DiagnosticPrivacyAndBaselineAge(t *testing.T) {
	ref := strings.Repeat("z", 48)
	queue := "RANKED_SOLO_5x5"
	baseline := lpSnapshot{Tier: "CHALLENGER", LeaguePoints: 98765, Wins: 123456, Losses: 654321}
	tracker, hash := seededLPTracker(t, ref, queue, baseline)
	events := captureLPObservations(tracker)
	clock := time.Unix(1820000000, 0)
	tracker.now = func() time.Time { return clock }
	client := lpTestClient(t, 182001, 420)
	tracker.takeGameStart(client, ref, func() ([]gameplayRank, EndpointCapability) {
		return []gameplayRank{r182Rank(baseline, queue)}, EndpointCapability{State: capabilityAvailable, Path: "lcu:ranked"}
	})
	clock = clock.Add(120 * time.Second)
	after := baseline
	after.Losses += 2
	after.LeaguePoints -= 18
	r182Capture(tracker, client, ref, queue, []lpSnapshot{after}, nil)
	event, ok := lpObservation(*events, "lp_capture_recorded")
	if !ok || event["baseline_age_s"] != int64(120) {
		t.Fatal(*events)
	}
	assertLPObservationPrivacy(t, *events, ref, hash, queue, "CHALLENGER", "98765", "123456", "654321", "182001")
}

func TestR182GameflowEventsStartThenSettlement(t *testing.T) {
	ref := strings.Repeat("e", 48)
	queue := "RANKED_SOLO_5x5"
	baseline := lpSnapshot{Tier: "GOLD", Division: "II", LeaguePoints: 80, Wins: 10, Losses: 10}
	tracker, hash := seededLPTracker(t, ref, queue, baseline)
	var settled atomic.Bool
	var rankReads atomic.Int32
	endPhase := make(chan struct{})
	started := make(chan struct{}, 1)
	recorded := make(chan struct{}, 1)
	var eventMu sync.Mutex
	events := []map[string]any{}
	tracker.observeEvent = func(event map[string]any) {
		eventMu.Lock()
		events = append(events, event)
		eventMu.Unlock()
		if event["event"] == "lp_baseline_written" && event["writer"] == "game_start" {
			started <- struct{}{}
		}
		if event["event"] == "lp_capture_recorded" {
			recorded <- struct{}{}
		}
	}
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if websocket.IsWebSocketUpgrade(r) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			if _, _, err = conn.ReadMessage(); err != nil {
				return
			}
			send := func(phase string) {
				_ = conn.WriteJSON([]any{8, "OnJsonApiEvent", map[string]any{"uri": "/lol-gameflow/v1/gameflow-phase", "eventType": "Update", "data": phase}})
			}
			send("InProgress")
			<-endPhase
			send("WaitingForStats")
			for {
				if _, _, err = conn.ReadMessage(); err != nil {
					return
				}
			}
		}
		switch r.URL.Path {
		case "/lol-gameflow/v1/session":
			_ = json.NewEncoder(w).Encode(map[string]any{"gameData": map[string]any{"gameId": 182001, "queue": map[string]any{"id": 420, "mapId": 11, "gameMode": "CLASSIC"}}})
		case "/lol-ranked/v1/current-ranked-stats":
			rankReads.Add(1)
			s := baseline
			if settled.Load() {
				s.Losses += 2
				s.LeaguePoints -= 18
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"queues": []any{r182Rank(s, queue)}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	client := newLCUClient(port, "fixture")
	defer client.Close()
	p := newChampionProvider()
	p.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) { return r178JSON(map[string]any{}, 404), nil })}
	a := &app{lcu: client, connected: true, summoner: Summoner{PUUID: ref}, lpTracker: tracker, champions: p, refreshRequests: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.runConnectedSession(ctx, client) }()
	var endOnce sync.Once
	finish := func() { endOnce.Do(func() { close(endPhase) }) }
	defer func() {
		finish()
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("session did not stop")
		}
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("InProgress did not write a game-start baseline")
	}
	tracker.mu.Lock()
	start := tracker.history.GameStarts[hash][queue]
	tracker.mu.Unlock()
	if start.GameID != 182001 || start.Source != "lcu" || start.Snapshot != baseline {
		t.Fatal(start)
	}
	settled.Store(true)
	finish()
	select {
	case <-recorded:
	case <-time.After(3 * time.Second):
		t.Fatal("WaitingForStats did not record loss")
	}
	tracker.mu.Lock()
	record := tracker.history.Games["182001"]
	tracker.mu.Unlock()
	if record.Delta != -18 || rankReads.Load() < 4 {
		t.Fatal(record, rankReads.Load())
	}
	eventMu.Lock()
	defer eventMu.Unlock()
	event, ok := lpObservation(events, "lp_capture_recorded")
	if !ok || event["baseline_source"] != "game_start" {
		t.Fatal(events)
	}
	assertLPObservationPrivacy(t, events, ref, hash, queue, "182001")
}

func TestR182NoStartKeepsFirstEligibleDecision(t *testing.T) {
	ref := strings.Repeat("f", 48)
	queue := "RANKED_SOLO_5x5"
	baseline := lpSnapshot{Tier: "GOLD", Division: "II", LeaguePoints: 80, Wins: 10, Losses: 10}
	tracker, _ := seededLPTracker(t, ref, queue, baseline)
	events := captureLPObservations(tracker)
	first := baseline
	first.Wins++
	first.LeaguePoints += 24
	followup := first
	followup.Wins++
	followup.LeaguePoints += 20
	calls := r182Capture(tracker, lpTestClient(t, 182001, 420), ref, queue, []lpSnapshot{first, followup}, nil)
	if calls != 3 || tracker.history.Games["182001"].Delta != 24 || tracker.history.Baselines[tracker.accountHash(ref)][queue] != followup || lpObservationCount(*events, "lp_capture_settle") != 2 {
		t.Fatal(tracker.history, *events)
	}
	// Diagnostic sampling failure cannot erase a decision supported by the old +1 guard.
	tracker, _ = seededLPTracker(t, ref, queue, baseline)
	calls = 0
	tracker.capture(lpTestClient(t, 182002, 420), ref, func() ([]gameplayRank, EndpointCapability) {
		calls++
		if calls == 1 {
			return []gameplayRank{r182Rank(first, queue)}, EndpointCapability{State: capabilityAvailable, Path: "lcu:ranked"}
		}
		return nil, EndpointCapability{State: capabilityFailed}
	})
	if calls != 3 || tracker.history.Games["182002"].Delta != 24 {
		t.Fatal(calls, tracker.history)
	}
}
