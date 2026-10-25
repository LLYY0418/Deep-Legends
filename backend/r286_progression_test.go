package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

type r286ProgressionFixture struct {
	Help           json.RawMessage
	Configuration  json.RawMessage
	Before, After  struct{ Instance, Inventory json.RawMessage }
	UnresolvedHelp json.RawMessage
}

func r286ProgressionFixtureData(t *testing.T) r286ProgressionFixture {
	t.Helper()
	b, err := os.ReadFile("testdata/r286-progression.json")
	if err != nil {
		t.Fatal(err)
	}
	var f r286ProgressionFixture
	if err = json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	return f
}
func TestR286ProgressionBeforeAfterReadOnlyResolvedFromFullHelp(t *testing.T) {
	for _, kind := range []string{"changed", "unchanged", "unresolved"} {
		t.Run(kind, func(t *testing.T) {
			f := r286ProgressionFixtureData(t)
			a := r175App(t)
			a.summoner = Summoner{PUUID: "PRIVATE-SELF", SummonerID: 123456}
			c := &LCUClient{baseURL: "https://fixture", token: "test", sessionContext: context.Background()}
			var mu sync.Mutex
			calls := map[string]int{}
			post := false
			c.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				mu.Lock()
				defer mu.Unlock()
				calls[r.URL.Path]++
				if r.Method != "GET" {
					t.Error("non-GET probe", r.Method, r.URL.Path)
				}
				switch r.URL.Path {
				case "/help":
					if r.URL.Query().Get("format") != "Full" {
						t.Error("must resolve Full help")
					}
					if kind == "unresolved" {
						return r206RelayResponse(200, f.UnresolvedHelp), nil
					}
					return r206RelayResponse(200, f.Help), nil
				case "/lol-progression/v3/catalog/groups/configuration":
					return r206RelayResponse(200, f.Configuration), nil
				case "/lol-progression/v3/catalog/groups/cherry-season/instance-data":
					if post && kind == "changed" {
						return r206RelayResponse(200, f.After.Instance), nil
					}
					return r206RelayResponse(200, f.Before.Instance), nil
				case "/lol-inventory/v6/bags/cherry":
					if post && kind == "changed" {
						return r206RelayResponse(200, f.After.Inventory), nil
					}
					return r206RelayResponse(200, f.Before.Inventory), nil
				default:
					return r206RelayResponse(404, nil), nil
				}
			})}
			s := c.arenaDiscoveryState()
			var timersMu sync.Mutex
			timers := []struct {
				delay time.Duration
				fn    func()
			}{}
			s.schedule = func(delay time.Duration, fn func()) func() {
				timersMu.Lock()
				defer timersMu.Unlock()
				timers = append(timers, struct {
					delay time.Duration
					fn    func()
				}{delay, fn})
				return func() {}
			}
			a.startArenaProgressionDiscovery(c, "InProgress")
			if s.started {
				t.Fatal("midgame cannot create before snapshot")
			}
			a.startArenaProgressionDiscovery(c, "ChampSelect")
			<-s.done
			<-s.progression.done
			a.startArenaProgressionDiscovery(c, "GameStart") // Already armed: no duplicate before.
			a.observeArenaDiscoveryPhase(c, "InProgress")
			mu.Lock()
			post = true
			mu.Unlock()
			a.observeArenaDiscoveryPhase(c, "Lobby")
			a.observeArenaDiscoveryPhase(c, "None")
			timersMu.Lock()
			scheduled := append([]struct {
				delay time.Duration
				fn    func()
			}{}, timers...)
			timersMu.Unlock()
			for _, timer := range scheduled {
				if timer.delay == 20*time.Second || timer.delay == 2*time.Minute {
					timer.fn()
				}
			}
			events := readDiagnosticEvents(t, a.storage, "arena_fame_progression_diff_v3")
			if len(events) != 1 {
				t.Fatal(events)
			}
			event := events[0]
			changes, ok := event["changes"].(map[string]any)
			if !ok {
				t.Fatal(event)
			}
			if kind == "changed" {
				if len(changes) != 3 {
					t.Fatal(changes)
				}
				v := changes["group/cherry-season/counters.fame"].(map[string]any)
				if v["before"] != float64(10) || v["after"] != float64(25) {
					t.Fatal(v)
				}
			} else if len(changes) != 0 {
				t.Fatal(changes)
			}
			raw, _ := json.Marshal(event)
			for _, secret := range []string{"PRIVATE", "123456", "654321", "playerId", "puuid", "accountId", "unchanged"} {
				if strings.Contains(string(raw), secret) {
					t.Fatal("private or unchanged field", string(raw))
				}
			}
			mu.Lock()
			defer mu.Unlock()
			if kind == "unresolved" {
				if calls["/lol-progression/v1/groups/configuration"] != 1 || calls["/lol-inventory/v1/cherry-inventory"] != 1 || event["before_status"].(map[string]any)["configuration"] != "http-404" {
					t.Fatal(calls, event)
				}
			} else {
				for _, path := range []string{"/lol-progression/v3/catalog/groups/configuration", "/lol-progression/v3/catalog/groups/cherry-season/instance-data", "/lol-inventory/v6/bags/cherry"} {
					if calls[path] != 2 {
						t.Fatalf("must read exactly before and after: %s=%d all=%v", path, calls[path], calls)
					}
				}
			}
			a.probeArenaProgressionAfter(c, s)
			if len(readDiagnosticEvents(t, a.storage, "arena_fame_progression_diff_v3")) != 1 {
				t.Fatal("duplicate diff")
			}
		})
	}
}
func TestR286ProgressionPartsReconstructWithin4KB(t *testing.T) {
	a := r175App(t)
	before := arenaProgressionSnapshot{Values: map[string]float64{}}
	after := arenaProgressionSnapshot{Values: map[string]float64{}}
	for i := 0; i < 400; i++ {
		key := fmt.Sprintf("ArenaSeason/counters.fame%d", i)
		before.Values[key] = float64(i)
		after.Values[key] = float64(i + 20)
	}
	changes := arenaProgressionDiff(before, after)
	event := map[string]any{"event": "arena_fame_progression_diff_v3", "changes": changes}
	parts := arenaFameDiscoveryParts(event)
	if len(parts) < 2 {
		t.Fatal("oversized diff was not split")
	}
	for _, part := range parts {
		a.recordDiagnostic(part)
	}
	events := readDiagnosticEvents(t, a.storage, "arena_fame_progression_diff_v3")
	rebuilt := ""
	for i, e := range events {
		raw, _ := json.Marshal(e)
		if len(raw) >= 4096 || e["truncated"] != nil {
			t.Fatalf("part over budget %d", len(raw))
		}
		if e["part_index"] != float64(i+1) || e["part_count"] != float64(len(parts)) {
			t.Fatal(e)
		}
		rebuilt += e["data"].(string)
	}
	var decoded map[string]arenaProgressionDelta
	if err := json.Unmarshal([]byte(rebuilt), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 400 || decoded["ArenaSeason/counters.fame399"].After != 419 {
		t.Fatal(len(decoded))
	}
}
func TestR286ProgressionDoesNotGuessMissingHTTPMethodsOrQueryArguments(t *testing.T) {
	for _, raw := range []string{`{"name":"GetLolProgressionV1GroupsConfiguration","path":"/lol-progression/v1/groups/configuration"}`, `{"name":"GetLolProgressionV1GroupsConfiguration","method":"POST","path":"/lol-progression/v1/groups/configuration"}`, `{"name":"GetLolProgressionV1GroupsConfiguration","method":"GET","path":"/lol-progression/v1/groups/configuration","arguments":[{"name":"unknown","required":true}]}`, `{"name":"GetLolProgressionV1GroupsConfiguration","method":"GET","path":"/lol-progression/v1/groups/configuration","arguments":{"unknown":{"required":true}}}`} {
		ops := arenaDiscoveryHelpAll(json.RawMessage(raw))
		if len(ops) != 1 || arenaProgressionPath(ops[0], Summoner{}, "") != "" {
			t.Fatal(ops)
		}
	}
}

func TestR286ProgressionExplicitGroupIDsAndExistingDiscovery(t *testing.T) {
	if groups := arenaProgressionGroups(json.RawMessage(`{"groups":{"metadata":{"name":"Wrong"}}}`)); len(groups) != 0 {
		t.Fatal("guessed group ID", groups)
	}
	a := r175App(t)
	c := &LCUClient{baseURL: "https://fixture", token: "test", sessionContext: context.Background()}
	c.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) { return r206RelayResponse(200, []byte(`{}`)), nil })}
	a.discoverArenaFame(c)
	s := c.arenaDiscoveryState()
	<-s.done
	a.startArenaProgressionDiscovery(c, "ChampSelect")
	s.mu.Lock()
	p := s.progression
	s.mu.Unlock()
	if p == nil {
		t.Fatal("existing discovery swallowed pregame progression")
	}
	<-p.done
	if p.before.Status["configuration"] != "empty" {
		t.Fatal(p.before)
	}
	a.observeArenaDiscoveryPhase(c, "Lobby")
	if p.scheduled {
		t.Fatal("canceled champion select is not post-game")
	}
}
