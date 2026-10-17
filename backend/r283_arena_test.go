package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestR283ArenaHelpFormats(t *testing.T) {
	for _, raw := range []string{`{"functions":{"GetLolCherryV1Foo":"说明"},"events":{"OnJsonApiEvent_lol-cherry_v1_foo":"说明"}}`, `[{"url":"/lol-end-of-game/v1/eog-stats-block","http_method":"GET","arguments":[]}]`} {
		ops := arenaDiscoveryHelp(json.RawMessage(raw))
		if len(ops) == 0 {
			t.Fatal("help format ignored", raw)
		}
	}
	ops := arenaDiscoveryHelp(json.RawMessage(`{"functions":{"GetLolCherryV1Foo":"说明"},"events":{"OnJsonApiEvent_lol-cherry_v1_foo":"说明"}}`))
	if len(ops) != 2 {
		t.Fatal(ops)
	}
}
func TestR283ArenaPostGameWindow(t *testing.T) {
	a := r175App(t)
	a.summoner = Summoner{PUUID: "PRIVATE-PUUID", SummonerID: 12345}
	c := &LCUClient{baseURL: "https://fixture", token: "test", sessionContext: context.Background()}
	calls := map[string]int{}
	var callsMu, timersMu sync.Mutex
	c.http = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		callsMu.Lock()
		defer callsMu.Unlock()
		calls[r.URL.Path]++
		if r.URL.Path == "/help" {
			if r.URL.RawQuery != "format=Full" {
				t.Error("Full help not preferred")
			}
			return r206RelayResponse(200, []byte(`[{"url":"/lol-cherry/v1/fame","http_method":"GET","arguments":[]}]`)), nil
		}
		if r.URL.Path == "/lol-cherry/v1/fame" {
			return r206RelayResponse(200, []byte(`{"fame":`+map[bool]string{true: "20", false: "10"}[calls[r.URL.Path] > 1]+`,"puuid":"PRIVATE-PUUID"}`)), nil
		}
		return r206RelayResponse(404, nil), nil
	})}
	s := c.arenaDiscoveryState()
	timers := map[time.Duration]func(){}
	s.schedule = func(delay time.Duration, fn func()) func() {
		timersMu.Lock()
		defer timersMu.Unlock()
		timers[delay] = fn
		return func() {}
	}
	a.discoverArenaFame(c)
	<-s.done
	a.observeArenaDiscoveryPhase(c, "InProgress")
	a.observeArenaDiscoveryPhase(c, "PreEndOfGame")
	if len(readDiagnosticEvents(t, a.storage, "arena_fame_discovery")) != 0 {
		t.Fatal("premature end emission")
	}
	a.observeArenaDiscoveryPhase(c, "Lobby")
	c.observeArenaDiscoveryEvent(LCUEvent{URI: "/lol-end-of-game/v1/eog-stats-block/12345"})
	timersMu.Lock()
	postGame, finish := timers[20*time.Second], timers[120*time.Second]
	timersMu.Unlock()
	if postGame == nil || finish == nil {
		t.Fatal("missing post-game windows")
	}
	postGame()
	callsMu.Lock()
	if calls["/lol-cherry/v1/fame"] != 2 || calls["/lol-pre-end-of-game/v1/currentSequenceEvent"] != 1 {
		t.Fatal(calls)
	}
	callsMu.Unlock()
	if len(readDiagnosticEvents(t, a.storage, "arena_fame_discovery")) != 0 {
		t.Fatal("emitted before120s")
	}
	finish()
	events := readDiagnosticEvents(t, a.storage, "arena_fame_discovery")
	if len(events) != 1 {
		t.Fatal(events)
	}
	raw, _ := json.Marshal(events[0])
	if len(raw) > 8192 || strings.Contains(string(raw), "PRIVATE-PUUID") || strings.Contains(string(raw), "12345") {
		t.Fatal("privacy/budget", string(raw))
	}
	if !strings.Contains(string(raw), "post_game") || !strings.Contains(string(raw), "difference") || !strings.Contains(string(raw), "lol-end-of-game") {
		t.Fatal(string(raw))
	}
}
