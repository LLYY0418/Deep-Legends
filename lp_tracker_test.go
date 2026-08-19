package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

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

func TestLPTrackerObserveAnnotatePersist(t *testing.T) {
	root := t.TempDir()
	store := &localStore{root: root, salt: bytes.Repeat([]byte{7}, 32)}
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
	store := &localStore{root: root, salt: bytes.Repeat([]byte{3}, 32)}
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
