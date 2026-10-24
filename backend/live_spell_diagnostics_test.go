package main

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLiveSpellProbeChangesPrivacyAndMissingValues(t *testing.T) {
	p := newLiveSpellProbe()
	raw := []byte(`[{"riotId":"PRIVATE#123","summonerName":"PRIVATE","team":"CHAOS","nameVisibilityType":"HIDDEN","summonerSpells":{"summonerSpellOne":{"displayName":"PRIVATE","rawDisplayName":"GeneratedTip_SummonerSpell_SummonerFlash_DisplayName","rawDescription":"技能描述","PRIVATE-key":123},"summonerSpellTwo":{"rawDisplayName":"SummonerDot"}}}]`)
	allRows, ok := p.playerRows(raw)
	rows := spellSummaryRows(allRows, "live_spell_player")
	if !ok || len(rows) != 1 {
		t.Fatalf("baseline %v %v", rows, ok)
	}
	encoded, _ := json.Marshal(allRows)
	for _, secret := range []string{"PRIVATE", "SECRET", "HIDDEN"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("identity leaked: %s", encoded)
		}
	}
	state := rows[0]["state"].(map[string]any)
	spell := state["summonerSpellOne"].(map[string]any)
	if spell["spell"] != "SummonerFlash" || spell["cooldownRemaining"] != nil || state["isVisible"] != nil {
		t.Fatalf("inferred missing state: %v", state)
	}
	if rows, _ := p.playerRows(raw); len(rows) != 0 {
		t.Fatal("unchanged snapshot logged again")
	}
	changed := []byte(strings.Replace(string(raw), `"PRIVATE-key":123`, `"cooldownRemaining":299,"isVisible":false`, 1))
	allRows, ok = p.playerRows(changed)
	rows = spellSummaryRows(allRows, "live_spell_player")
	if !ok || len(rows) != 1 || rows[0]["baseline"] != false {
		t.Fatal("missed change")
	}
	spell = rows[0]["state"].(map[string]any)["summonerSpellOne"].(map[string]any)
	if spell["cooldownRemaining"] != float64(299) || spell["isVisible"] != false {
		t.Fatal("lost raw numeric/boolean observation")
	}
	line, err := marshalDiagnosticRecord(rows[0], time.Now())
	if err != nil || strings.Contains(string(line), `"truncated":true`) {
		t.Fatalf("diagnostic lost details: %s %v", line, err)
	}
}

func TestLiveSpellProbeEventsResetAndMalformed(t *testing.T) {
	p := newLiveSpellProbe()
	if !p.gameTime([]byte(`{"gameTime":120}`)) {
		t.Fatal("game time")
	}
	raw := []byte(`{"Events":[{"EventID":1,"EventName":"SummonerSpellCast","EventTime":120,"CasterName":"PRIVATE","SpellID":4},{"EventID":2,"EventName":"PRIVATE","PRIVATE":123}]}`)
	allRows, ok := p.eventRows(raw)
	rows := spellSummaryRows(allRows, "live_spell_event")
	if !ok || len(rows) != 2 || rows[0]["name"] != "SummonerSpellCast" || rows[1]["name"] != "unknown" {
		t.Fatalf("events: %v", rows)
	}
	encoded, _ := json.Marshal(allRows)
	if strings.Contains(string(encoded), "PRIVATE") {
		t.Fatal("event identity leaked")
	}
	if rows, _ := p.eventRows(raw); len(rows) != 0 {
		t.Fatal("duplicate events")
	}
	p.gameTime([]byte(`{"gameTime":1}`))
	if rows, _ := p.eventRows(raw); p.epoch != 1 || len(spellSummaryRows(rows, "live_spell_event")) != 2 {
		t.Fatal("new game reused event IDs")
	}
	for _, raw := range []string{`null`, `{}`, `{"Events":null}`, `{"Events":"error"}`} {
		if _, ok := p.eventRows([]byte(raw)); ok {
			t.Fatalf("invalid events accepted: %s", raw)
		}
	}
	for _, raw := range []string{`null`, `{}`, `[{"summonerSpells":`} {
		if _, ok := p.playerRows([]byte(raw)); ok {
			t.Fatalf("invalid players accepted: %s", raw)
		}
	}
}

func TestLiveSpellProbeContinuesAfterSuccessAndCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls, waits := 0, 0
	var rows []map[string]any
	var mu sync.Mutex
	runLiveSpellProbe(ctx, func(ctx context.Context, endpoint string) ([]byte, int, error) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		switch endpoint {
		case "gamestats":
			return []byte(`{"gameTime":1}`), 200, nil
		case "playerlist":
			return []byte(`[{"riotId":"PRIVATE#1","summonerSpells":{}}]`), 200, nil
		default:
			return []byte(`{"Events":[]}`), 200, nil
		}
	}, func(row map[string]any) {
		mu.Lock()
		defer mu.Unlock()
		rows = append(rows, row)
	}, func(ctx context.Context, d time.Duration) bool {
		mu.Lock()
		defer mu.Unlock()
		if d != 2*time.Second {
			t.Fatalf("interval: %v", d)
		}
		waits++
		if waits == 3 {
			cancel()
			return false
		}
		return true
	})
	mu.Lock()
	defer mu.Unlock()
	if calls != 9 || rows[len(rows)-1]["event"] != "live_spell_stop" {
		t.Fatalf("lifecycle calls=%d rows=%v", calls, rows)
	}
	playerCount := 0
	for _, row := range rows {
		if row["event"] == "live_spell_player" {
			playerCount++
		}
	}
	if playerCount != 1 {
		t.Fatalf("unchanged spam: %d", playerCount)
	}
}

func TestLiveSpellProbeFailureIsNotEmptySuccess(t *testing.T) {
	var rows []map[string]any
	var mu sync.Mutex
	runLiveSpellProbe(context.Background(), func(context.Context, string) ([]byte, int, error) {
		return []byte(`{}`), 404, nil
	}, func(row map[string]any) {
		mu.Lock()
		defer mu.Unlock()
		rows = append(rows, row)
	}, func(_ context.Context, delay time.Duration) bool {
		if delay != 10*time.Second {
			t.Fatalf("failure backoff %v", delay)
		}
		return false
	})
	mu.Lock()
	defer mu.Unlock()
	health := spellSummaryRows(rows, "live_spell_health")
	if len(health) != 1 || health[0]["endpoints"].(map[string]any)["playerlist"] != "http-404" || len(spellSummaryRows(rows, "live_spell_sample")) != 1 {
		t.Fatalf("failure evidence %v", rows)
	}
}

func TestLiveSpellProbeMaximumRecordKeepsDetails(t *testing.T) {
	fields := map[string]any{}
	for _, key := range strings.Fields("cooldown cooldownRemaining remainingCooldown cooldownExpires cooldownEndTime cooldownStartTime castTime lastCastTime charges ammo spellId spellID isReady isOnCooldown isVisible visible isInFogOfWar isDead EventID EventTime SpellID SpellSlot") {
		fields[key] = 123456789.123456
	}
	fields["rawDisplayName"] = "SummonerFlash"
	player := map[string]any{}
	for key, value := range fields {
		player[key] = value
	}
	player["summonerSpells"] = map[string]any{"summonerSpellOne": fields, "summonerSpellTwo": fields}
	raw, _ := json.Marshal([]any{player})
	rows, _ := newLiveSpellProbe().playerRows(raw)
	line, err := marshalDiagnosticRecord(rows[0], time.Now())
	if err != nil || strings.Contains(string(line), `"truncated":true`) {
		t.Fatalf("max state lost (%d bytes): %s", len(line), line)
	}
}

func spellSummaryRows(rows []map[string]any, event string) []map[string]any {
	var result []map[string]any
	for _, row := range rows {
		if row["event"] == event {
			result = append(result, row)
		}
	}
	return result
}
