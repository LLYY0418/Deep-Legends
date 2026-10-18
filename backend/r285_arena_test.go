package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestR285ArenaDiscoveryPartsRoundTrip(t *testing.T) {
	a := r175App(t)
	rows := []map[string]any{}
	for i := 0; i < 40; i++ {
		rows = append(rows, map[string]any{"path": fmt.Sprintf("/lol-cherry/v1/progression-%d", i), "method": "GET", "status": 200, "keys": []string{"progression", "fame"}, "numbers": map[string]float64{"progression.fame": float64(i), "progression.level": 12}})
	}
	// A single shape can also exceed a line, with Unicode and JSON escapes.
	rows = append(rows, map[string]any{"name": strings.Repeat("名望\"\\", 1100)})
	event := map[string]any{"event": "arena_fame_discovery", "help_status": 200, "interfaces": rows, "post_game": rows[:20], "event_uris": []string{"/lol-cherry/v1/fame/{id}", "/lol-end-of-game/v1/stats"}, "queue_types": map[string][]string{"lcu": {"RANKED_SOLO_5x5"}, "sgp": {"RANKED_FLEX_SR"}}}
	raw, _ := json.Marshal(event)
	if len(raw) < 5*1024 {
		t.Fatal("fixture too small")
	}
	s := &arenaFameDiscoveryState{started: true, event: event, uris: event["event_uris"].([]string), queues: map[string][]string{dataSourceLCU: {"RANKED_SOLO_5x5"}, dataSourceSGP: {"RANKED_FLEX_SR"}}}
	a.finishArenaFameDiscovery(s)
	events := readDiagnosticEvents(t, a.storage, "arena_fame_discovery")
	pieces := map[string]string{}
	for i, e := range events {
		b, _ := json.Marshal(e)
		if len(b) >= 4096 || e["truncated"] != nil {
			t.Fatalf("part truncated bytes=%d event=%v", len(b), e)
		}
		if e["part_index"] != float64(i+1) || e["part_count"] != float64(len(events)) {
			t.Fatalf("part sequence=%v", e)
		}
		part, _ := e["part"].(string)
		chunk, _ := e["data"].(string)
		pieces[part] += chunk
	}
	rebuilt := map[string]any{"event": "arena_fame_discovery"}
	for part, data := range pieces {
		var v any
		if err := json.Unmarshal([]byte(data), &v); err != nil {
			t.Fatal(err)
		}
		rebuilt[part] = v
	}
	var want any
	json.Unmarshal(raw, &want)
	if !reflect.DeepEqual(rebuilt, want) {
		t.Fatalf("incomplete reconstruction: parts=%v events=%d", pieces, len(events))
	}
}

func TestR285ArenaDiscoveryKeepsLargeProbeSets(t *testing.T) {
	a := r175App(t)
	ops := []map[string]any{}
	numbers := map[string]any{}
	for i := 0; i < 24; i++ {
		ops = append(ops, map[string]any{"method": "GET", "path": fmt.Sprintf("/lol-cherry/v1/progression-%d", i)})
	}
	for i := 0; i < 40; i++ {
		numbers[fmt.Sprintf("famePoints%d", i)] = i
	}
	help, _ := json.Marshal(ops)
	body, _ := json.Marshal(numbers)
	c := &LCUClient{baseURL: "https://fixture", token: "test", sessionContext: context.Background()}
	c.http = &http.Client{Transport: gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/help" {
			return r206RelayResponse(200, help), nil
		}
		return r206RelayResponse(200, body), nil
	})}
	a.discoverArenaFame(c)
	s := c.arenaDiscoveryState()
	<-s.done
	a.probeArenaFamePostGame(c, s)
	s.mu.Lock()
	defer s.mu.Unlock()
	if rows, _ := s.event["interfaces"].([]map[string]any); len(rows) != 24 {
		t.Fatalf("interfaces=%d", len(rows))
	}
	if rows, _ := s.event["post_game"].([]map[string]any); len(rows) != 26 {
		t.Fatalf("post_game=%d", len(rows))
	}
}
