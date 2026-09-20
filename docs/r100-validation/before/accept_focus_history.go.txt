package main

import (
	"encoding/json"
	"strings"
	"sync"
	"time"
)

// Only fixed event names and whitelisted scalar states enter this bounded ring.
// The websocket callback never writes to disk or requests additional endpoints.
type acceptFocusHistory struct {
	mu      sync.Mutex
	rows    []map[string]any
	dropped int
}

func focusPhase(phase string) string {
	switch phase {
	case "None", "Lobby", "Matchmaking", "ReadyCheck", "ChampSelect", "InProgress", "Reconnect", "WaitingForStats", "PreEndOfGame", "EndOfGame":
		return phase
	default:
		return "unknown"
	}
}

func (c *LCUClient) rememberAcceptFocusEvent(event LCUEvent) {
	path := strings.ToLower(event.URI)
	switch path {
	case "/lol-gameflow/v1/gameflow-phase", "/lol-matchmaking/v1/ready-check", "/lol-champ-select/v1/session":
	default:
		return
	}
	row := map[string]any{"observed_at_ms": time.Now().UnixMilli(), "path": path, "foreground": nativeAcceptWindowState().category}
	if path == "/lol-gameflow/v1/gameflow-phase" {
		var phase string
		row["parsed"] = json.Unmarshal(event.Data, &phase) == nil
		row["phase"] = focusPhase(phase)
	} else if path == "/lol-matchmaking/v1/ready-check" {
		var ready struct {
			State          string `json:"state"`
			PlayerResponse string `json:"playerResponse"`
		}
		row["parsed"] = json.Unmarshal(event.Data, &ready) == nil
		for key, value := range map[string]string{"state": ready.State, "player_response": ready.PlayerResponse} {
			switch value {
			case "InProgress", "EveryoneReady", "Done", "Invalid", "Accepted", "Declined", "None":
				row[key] = value
			default:
				row[key] = "unknown"
			}
		}
	}
	c.acceptFocusHistory.mu.Lock()
	defer c.acceptFocusHistory.mu.Unlock()
	if len(c.acceptFocusHistory.rows) == 128 {
		copy(c.acceptFocusHistory.rows, c.acceptFocusHistory.rows[1:])
		c.acceptFocusHistory.rows = c.acceptFocusHistory.rows[:127]
		c.acceptFocusHistory.dropped++
	}
	c.acceptFocusHistory.rows = append(c.acceptFocusHistory.rows, row)
}

func (c *LCUClient) acceptFocusHistorySnapshot() map[string]any {
	c.acceptFocusHistory.mu.Lock()
	defer c.acceptFocusHistory.mu.Unlock()
	rows := append([]map[string]any(nil), c.acceptFocusHistory.rows...)
	return map[string]any{"event": "accept_focus_event_history", "entries": rows, "dropped": c.acceptFocusHistory.dropped, "capacity": 128, "source": "websocket-before-routing"}
}
