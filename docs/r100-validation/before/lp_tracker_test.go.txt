package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
		"capture_index": true, "attempt": true, "attempts": true, "capability_state": true,
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
