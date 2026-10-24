package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLiveSpellProbeStartupCapabilitySurvivesRotation(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	a := &app{storage: trackTestStore(t, &localStore{root: root})}
	a.recordAppStartDiagnostic()
	a.enableDiagnosticRotationSnapshot()
	a.storage.onDiagnosticRotation()
	wire, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(wire)), "\n")
	if len(lines) != 2 {
		t.Fatalf("startup snapshots: %s", wire)
	}
	for i, line := range lines {
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatal(err)
		}
		if row["event"] != "app_start" || row["live_spell_diagnostic_schema"] != float64(2) || row["log_rotation"] != (i == 1) {
			t.Fatalf("startup/rotation capability missing: %v", row)
		}
	}
}

func TestLiveSpellProbeAnonymousReorderingStartsNewBaselines(t *testing.T) {
	p := newLiveSpellProbe()
	first := `[{"summonerSpells":{"summonerSpellOne":{"rawDisplayName":"FirstSpell"}}},{"summonerSpells":{"summonerSpellOne":{"rawDisplayName":"SecondSpell"}}}]`
	second := `[{"summonerSpells":{"summonerSpellOne":{"rawDisplayName":"SecondSpell"}}},{"summonerSpells":{"summonerSpellOne":{"rawDisplayName":"FirstSpell"}}}]`
	mustSpellPlayers(t, p, first)
	p.sample++
	rows := mustSpellPlayers(t, p, second)
	if len(spellSummaryRows(rows, "live_spell_change")) != 0 {
		t.Fatal("anonymous list positions were treated as stable player identities")
	}
	for _, row := range rows {
		if row["identity_scope"] != "sample" {
			t.Fatalf("sample-only identity scope missing: %v", row)
		}
		if row["event"] == "live_spell_player" && row["baseline"] != true {
			t.Fatal("anonymous player reused prior baseline")
		}
	}
	// A duplicated name (for example a hidden champion alias) is not stable.
	p = newLiveSpellProbe()
	rows = mustSpellPlayers(t, p, `[{"summonerName":"DUPLICATE"},{"summonerName":"DUPLICATE"}]`)
	players := spellSummaryRows(rows, "live_spell_player")
	if len(players) != 2 || players[0]["player_slot"] == players[1]["player_slot"] {
		t.Fatal("duplicate names collapsed two players into one slot")
	}
}

func TestLiveSpellProbeActorAliasesFollowCurrentUniqueRoster(t *testing.T) {
	p := newLiveSpellProbe()
	mustSpellPlayers(t, p, `[{"summonerName":"DUPLICATE"},{"summonerName":"DUPLICATE"}]`)
	players := spellSummaryRows(mustSpellPlayers(t, p, `[{"summonerName":"DUPLICATE"}]`), "live_spell_player")
	rows, ok := p.eventRows([]byte(`{"Events":[{"EventID":1,"EventName":"NewCast","CasterName":"DUPLICATE"}]}`))
	events := spellSummaryRows(rows, "live_spell_event")
	if !ok || len(events) != 1 || events[0]["CasterName_slot"] != players[0]["player_slot"] {
		t.Fatal("a formerly ambiguous alias did not recover after becoming unique")
	}
	mustSpellPlayers(t, p, `[{"riotId":"OTHER#1"}]`)
	rows, _ = p.eventRows([]byte(`{"Events":[{"EventID":2,"EventName":"NewCast","CasterName":"DUPLICATE"}]}`))
	if spellSummaryRows(rows, "live_spell_event")[0]["CasterName_slot"] != nil {
		t.Fatal("a departed alias still points at a prior roster")
	}
}

func TestLiveSpellProbeCaptureBudgetsAreExplicit(t *testing.T) {
	for _, kind := range []string{"tracked-players", "tracked-events", "playerlist-size", "event-list-size"} {
		t.Run(kind, func(t *testing.T) {
			p := newLiveSpellProbe()
			var rows []map[string]any
			var ok bool
			switch kind {
			case "tracked-players":
				for i := 0; i < 64; i++ {
					p.players["existing:"+strconv.Itoa(i)] = i + 1
				}
				rows, ok = p.playerRows([]byte(`[{"riotId":"NEW#1"}]`))
			case "tracked-events":
				for i := int64(0); i < 4096; i++ {
					p.events[i] = true
				}
				rows, ok = p.eventRows([]byte(`{"Events":[{"EventID":4096}]}`))
			case "playerlist-size":
				raw, _ := json.Marshal(make([]map[string]any, 33))
				rows, ok = p.playerRows(raw)
			case "event-list-size":
				raw, _ := json.Marshal(map[string]any{"Events": make([]map[string]any, 4097)})
				rows, ok = p.eventRows(raw)
			}
			limits := spellSummaryRows(rows, "live_spell_limit")
			if !ok || len(limits) != 1 || limits[0]["reason"] != kind {
				t.Fatalf("capture budget mistaken for malformed data: %v %v", ok, rows)
			}
		})
	}
}

func TestLiveSpellProbeLimitSampleRetainsParseSuccess(t *testing.T) {
	var mu sync.Mutex
	var rows []map[string]any
	runLiveSpellProbe(context.Background(), func(_ context.Context, endpoint string) ([]byte, int, error) {
		switch endpoint {
		case "gamestats":
			return []byte(`{"gameTime":1}`), 200, nil
		case "playerlist":
			raw, _ := json.Marshal(make([]map[string]any, 33))
			return raw, 200, nil
		default:
			return []byte(`{"Events":[]}`), 200, nil
		}
	}, func(row map[string]any) { mu.Lock(); defer mu.Unlock(); rows = append(rows, row) }, func(_ context.Context, delay time.Duration) bool {
		if delay != 10*time.Second {
			t.Fatalf("limited capture backoff: %v", delay)
		}
		return false
	})
	mu.Lock()
	defer mu.Unlock()
	sample := spellSummaryRows(rows, "live_spell_sample")[0]
	if spellSummaryRows(rows, "live_spell_start")[0]["diagnostic_schema"] != 2 {
		t.Fatal("started probe does not use the advertised schema")
	}
	request := sample["requests"].(map[string]any)["playerlist"].(map[string]any)
	if sample["all_ok"] != false || request["parse_valid"] != true || request["result"] != "capture-limit" {
		t.Fatalf("limit sample not explicit: %v", sample)
	}
	if request["player_count"] != 33 || request["captured_player_count"] != 0 {
		t.Fatalf("limited roster counts are misleading: %v", request)
	}
}
