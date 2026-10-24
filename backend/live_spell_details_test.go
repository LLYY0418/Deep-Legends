package main

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Reassemble the actual JSONL wire format, including the shared storage encoder.
func spellDetailPayloads(t *testing.T, rows []map[string]any) []map[string]any {
	t.Helper()
	var result []map[string]any
	parts := map[int]string{}
	for _, row := range rows {
		name, _ := row["event"].(string)
		if name != "live_spell_snapshot" && name != "live_spell_change" {
			continue
		}
		wire, err := marshalDiagnosticRecord(row, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		var decoded map[string]any
		if err = json.Unmarshal(wire, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded["truncated"] == true {
			t.Fatalf("details lost by encoder: %s", wire)
		}
		id := int(decoded["observation"].(float64))
		parts[id] += decoded["payload"].(string)
		if decoded["part_index"] == decoded["part_count"] {
			var payload map[string]any
			if err = json.Unmarshal([]byte(parts[id]), &payload); err != nil {
				t.Fatalf("invalid chunked payload: %v", err)
			}
			payload["wire_event"] = name
			result = append(result, payload)
		}
	}
	return result
}

func TestLiveSpellProbeDetailsPreserveUnknownNamesTextAndDiffs(t *testing.T) {
	p := newLiveSpellProbe()
	p.sample = 1
	raw := `[{"riotId":"PRIVATE#123","summonerName":"PRIVATE","team":"CHAOS","summonerSpells":{"summonerSpellOne":{"displayName":"雪球","rawDisplayName":"SummonerSnowball","rawDescription":"命中后再次施放"},"summonerSpellTwo":{"rawDisplayName":"GeneratedTip_SummonerSpell_ArenaSpecial_DisplayName","displayName":"特殊技能","rawDescription":"原始说明"}},"newState":{"cooldownRemaining":"12.50","isVisible":false},"nameVisibilityType":"HIDDEN"}]`
	rows, ok := p.playerRows([]byte(raw))
	if !ok {
		t.Fatal("parse")
	}
	payloads := spellDetailPayloads(t, rows)
	if len(payloads) == 0 {
		t.Fatal("missing complete field snapshots")
	}
	data, _ := json.Marshal(payloads)
	for _, want := range []string{"SummonerSnowball", "ArenaSpecial", "命中后再次施放", "/summonerSpells/summonerSpellOne/rawDisplayName", "/newState/cooldownRemaining", "12.50", "nameVisibilityType"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("missing %q in %s", want, data)
		}
	}
	for _, secret := range []string{"PRIVATE", "HIDDEN"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("identity/value leak %q", secret)
		}
	}
	if repeat, _ := p.playerRows([]byte(raw)); len(repeat) != 0 {
		t.Fatal("unchanged snapshot repeated")
	}
	p.sample = 2
	changed := strings.Replace(raw, `"rawDisplayName":"SummonerSnowball"`, `"rawDisplayName":"SummonerSnowballFollowup"`, 1)
	changed = strings.Replace(changed, `"rawDescription":"命中后再次施放"`, `"rawDescription":"再次施放的说明"`, 1)
	payloads = spellDetailPayloads(t, mustSpellPlayers(t, p, changed))
	data, _ = json.Marshal(payloads)
	for _, want := range []string{"before", "after", "SummonerSnowball", "SummonerSnowballFollowup", "命中后再次施放", "再次施放的说明"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("missing diff %q: %s", want, data)
		}
	}
}

func mustSpellPlayers(t *testing.T, p *liveSpellProbe, raw string) []map[string]any {
	t.Helper()
	rows, ok := p.playerRows([]byte(raw))
	if !ok {
		t.Fatal("invalid players")
	}
	return rows
}

func TestLiveSpellProbeDetailsFieldRemovalUnknownEventAndLongText(t *testing.T) {
	p := newLiveSpellProbe()
	description := strings.Repeat("完整说明<>&\"\\中文", 800)
	spell := map[string]any{"displayName": "未知技能", "rawDisplayName": "NewSpellVariant", "rawDescription": description, "future": map[string]any{"remaining": nil, "nested": []any{true, float64(2)}}}
	raw, _ := json.Marshal([]any{map[string]any{"riotId": "PRIVATE#1", "summonerSpells": map[string]any{"summonerSpellOne": spell}}})
	rows, ok := p.playerRows(raw)
	if !ok {
		t.Fatal("parse")
	}
	payloads := spellDetailPayloads(t, rows)
	encoded, _ := json.Marshal(payloads)
	if !strings.Contains(string(encoded), "NewSpellVariant") {
		t.Fatal("unknown raw name lost")
	}
	// Unmarshal once more to compare the entire original long description, not a prefix.
	found := false
	for _, payload := range payloads {
		fields, _ := payload["fields"].(map[string]any)
		field, _ := fields["/summonerSpells/summonerSpellOne/rawDescription"].(map[string]any)
		if field["value"] == description {
			found = true
		}
	}
	if !found {
		t.Fatal("long text did not survive encoder and chunk reassembly")
	}
	delete(spell, "future")
	raw, _ = json.Marshal([]any{map[string]any{"riotId": "PRIVATE#1", "summonerSpells": map[string]any{"summonerSpellOne": spell}}})
	payloads = spellDetailPayloads(t, mustSpellPlayers(t, p, string(raw)))
	encoded, _ = json.Marshal(payloads)
	if !strings.Contains(string(encoded), `"after_present":false`) || !strings.Contains(string(encoded), "/future/nested/0") {
		t.Fatalf("field removal lost: %s", encoded)
	}
	events, ok := p.eventRows([]byte(`{"Events":[{"EventID":1,"EventName":"FutureSummonerUsed","EventTime":12,"SpellSlot":2,"CasterName":"PRIVATE","future":{"castClock":"12.0"}}]}`))
	if !ok {
		t.Fatal("events")
	}
	payloads = spellDetailPayloads(t, events)
	encoded, _ = json.Marshal(payloads)
	for _, want := range []string{"FutureSummonerUsed", "/future/castClock", "12.0"} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("unknown event lost %q: %s", want, encoded)
		}
	}
	if strings.Contains(string(encoded), "PRIVATE") {
		t.Fatal("actor leaked")
	}
}

func TestLiveSpellProbeDetailsLimitsAreExplicitAndEpochRestartsBaseline(t *testing.T) {
	p := newLiveSpellProbe()
	huge := strings.Repeat("说", liveSpellDetailMaxText/3+20)
	values := make([]any, liveSpellDetailMaxArray+5)
	for i := range values {
		values[i] = true
	}
	object := map[string]any{"summonerSpells": map[string]any{"summonerSpellOne": map[string]any{"rawDescription": huge, "future": values}}}
	raw, _ := json.Marshal([]any{object})
	rows := mustSpellPlayers(t, p, string(raw))
	payloads := spellDetailPayloads(t, rows)
	if len(payloads) != 1 || len(payloads[0]["limits"].([]any)) != 2 {
		t.Fatalf("missing text/array coverage limits: %v", payloads)
	}
	for _, row := range rows {
		if row["event"] == "live_spell_snapshot" && row["coverage_limited"] != true {
			t.Fatal("limit not visible on every fragment")
		}
	}
	// A same-length change beyond the retained prefix must still be detected.
	slot := object["summonerSpells"].(map[string]any)["summonerSpellOne"].(map[string]any)
	slot["rawDescription"] = huge[:len(huge)-3] + "改"
	raw, _ = json.Marshal([]any{object})
	if len(spellDetailPayloads(t, mustSpellPlayers(t, p, string(raw)))) == 0 {
		t.Fatal("change after text limit went undetected")
	}
	p.gameTime([]byte(`{"gameTime":100}`))
	p.gameTime([]byte(`{"gameTime":1}`))
	payloads = spellDetailPayloads(t, mustSpellPlayers(t, p, string(raw)))
	if p.epoch != 1 || payloads[0]["wire_event"] != "live_spell_snapshot" {
		t.Fatal("epoch reused prior snapshot")
	}
	// Field-count and recursion limits also report incomplete coverage.
	many := map[string]any{}
	for i := 0; i < liveSpellDetailMaxFields+5; i++ {
		many[strconv.Itoa(i)] = true
	}
	_, limits := p.detailFields("playerlist", many)
	if len(limits) == 0 {
		t.Fatal("field-count limit hidden")
	}
	deep := map[string]any{}
	at := deep
	for i := 0; i < liveSpellDetailMaxDepth+3; i++ {
		next := map[string]any{}
		at["nested"] = next
		at = next
	}
	_, limits = p.detailFields("playerlist", deep)
	if len(limits) == 0 {
		t.Fatal("depth limit hidden")
	}
}

func TestLiveSpellProbeDetailsTypeChangesActorsAndEventBatch(t *testing.T) {
	p := newLiveSpellProbe()
	first := `[{"riotId":"PRIVATE#1","summonerName":"PRIVATE","summonerSpells":{"summonerSpellOne":{"rawDisplayName":"UnknownVariant","cooldownRemaining":null}}}]`
	mustSpellPlayers(t, p, first)
	changed := strings.Replace(first, `"cooldownRemaining":null`, `"cooldownRemaining":"12 seconds"`, 1)
	payloads := spellDetailPayloads(t, mustSpellPlayers(t, p, changed))
	data, _ := json.Marshal(payloads)
	if !strings.Contains(string(data), `"type":"null"`) || !strings.Contains(string(data), `"value":"12 seconds"`) {
		t.Fatalf("type change lost: %s", data)
	}
	event := map[string]any{"EventID": float64(1), "EventName": "NewSpellState", "CasterName": "PRIVATE", "EventTime": 12.0}
	events := []any{}
	for i := 0; i < 140; i++ {
		clone := map[string]any{}
		for k, v := range event {
			clone[k] = v
		}
		clone["EventID"] = i
		events = append(events, clone)
	}
	raw, _ := json.Marshal(map[string]any{"Events": events})
	rows, ok := p.eventRows(raw)
	summaries := spellSummaryRows(rows, "live_spell_event")
	if !ok || len(summaries) != 128 || summaries[0]["CasterName_slot"] != 1 {
		t.Fatalf("event budget/actor mapping: %d", len(summaries))
	}
	rows, ok = p.eventRows(raw)
	if !ok || len(spellSummaryRows(rows, "live_spell_event")) != 12 {
		t.Fatal("backlog not drained")
	}
	// Array indices, numeric values and later disappearance all remain distinct.
	fields, _ := p.detailFields("playerlist", map[string]any{"summonerSpells": []any{float64(0), false, nil}})
	if fields["/summonerSpells/0"]["value"] != float64(0) || fields["/summonerSpells/1"]["value"] != false || fields["/summonerSpells/2"]["type"] != "null" {
		t.Fatal("zero/false/null confused")
	}
}

func TestLiveSpellProbeDetailsSampleCoverageAndStaleClock(t *testing.T) {
	var mu sync.Mutex
	var rows []map[string]any
	sample := 1
	runLiveSpellProbe(context.Background(), func(_ context.Context, endpoint string) ([]byte, int, error) {
		mu.Lock()
		defer mu.Unlock()
		if endpoint == "gamestats" {
			if sample == 2 {
				return []byte(`{}`), 404, nil
			}
			return []byte(`{"gameTime":` + strconv.Itoa(sample) + `}`), 200, nil
		}
		if endpoint == "playerlist" {
			return []byte(`[{"riotId":"PRIVATE#1","summonerSpells":{"summonerSpellOne":{"rawDisplayName":"UnknownVariant"}}}]`), 200, nil
		}
		return []byte(`{"Events":[]}`), 200, nil
	}, func(row map[string]any) { mu.Lock(); defer mu.Unlock(); rows = append(rows, row) }, func(_ context.Context, _ time.Duration) bool {
		mu.Lock()
		defer mu.Unlock()
		sample++
		return sample <= 3
	})
	mu.Lock()
	defer mu.Unlock()
	samples := spellSummaryRows(rows, "live_spell_sample")
	if len(samples) != 3 || samples[0]["clock_fresh"] != true || samples[1]["clock_fresh"] != false || samples[1]["all_ok"] != false {
		t.Fatalf("clock/failed sample hidden: %v", samples)
	}
	stop := spellSummaryRows(rows, "live_spell_stop")[0]
	if stop["successes"].(map[string]int)["gamestats"] != 2 || stop["failures"].(map[string]int)["gamestats"] != 1 {
		t.Fatal("final totals missing")
	}
	request := samples[0]["requests"].(map[string]any)["playerlist"].(map[string]any)
	if request["player_count"] != 1 || request["parse_valid"] != true {
		t.Fatal("roster coverage missing")
	}
	if len(spellDetailPayloads(t, rows)) == 0 {
		t.Fatal("no detail data")
	}
	// Optional evidence destination for an end-to-end decoder check, never needed
	// for a successful test and never used to skip assertions.
	if target := os.Getenv("LIVE_SPELL_WIRE_FIXTURE"); target != "" {
		var wire []byte
		for _, row := range rows {
			line, err := marshalDiagnosticRecord(row, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			wire = append(wire, line...)
		}
		if err := os.WriteFile(target, wire, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLiveSpellProbeDetailsRetainRejectedEventShapeAndMetadata(t *testing.T) {
	p := newLiveSpellProbe()
	raw := []byte(`{"futureCastState":{"remaining":"3 seconds"},"Events":[{"EventID":"new-format","EventName":"NewSummonerCast","CasterName":"PRIVATE","newCooldown":5}]}`)
	rows, ok := p.eventRows(raw)
	if !ok || p.lastRejectedEvents != 1 {
		t.Fatal("rejected event coverage")
	}
	payloads := spellDetailPayloads(t, rows)
	encoded, _ := json.Marshal(payloads)
	for _, want := range []string{"NewSummonerCast", "/newCooldown", "/futureCastState/remaining", "3 seconds"} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("rejected event detail missing %s: %s", want, encoded)
		}
	}
	if strings.Contains(string(encoded), "PRIVATE") {
		t.Fatal("rejected actor leaked")
	}
	again, _ := p.eventRows(raw)
	if len(again) != 0 {
		t.Fatal("rejected event or metadata repeated")
	}
	for i := 0; i < 130; i++ {
		raw, _ := json.Marshal(map[string]any{"Events": []any{map[string]any{"EventName": "NewCast", "EventID": "bad-" + strconv.Itoa(i)}}})
		rows, _ = p.eventRows(raw)
	}
	if !p.rejectedLimitReported || len(p.rejectedEvents) != 128 {
		t.Fatal("rejected-event limit was hidden")
	}
}

func TestLiveSpellProbeDetailsCancellationStillLogsPartialSample(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	var rows []map[string]any
	runLiveSpellProbe(ctx, func(context.Context, string) ([]byte, int, error) { cancel(); return nil, 0, context.Canceled }, func(row map[string]any) { mu.Lock(); defer mu.Unlock(); rows = append(rows, row) }, func(context.Context, time.Duration) bool { return true })
	mu.Lock()
	defer mu.Unlock()
	samples := spellSummaryRows(rows, "live_spell_sample")
	if len(samples) != 1 || samples[0]["completed"] != false || samples[0]["all_ok"] != false || len(spellSummaryRows(rows, "live_spell_stop")) != 1 {
		t.Fatalf("cancelled sample vanished: %v", rows)
	}
}
