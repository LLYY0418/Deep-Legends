package main

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type gameplayPhaseIdentity struct {
	Phase     string `json:"phase"`
	Connected bool   `json:"connected"`
	GameID    int64  `json:"gameId,omitempty"`
}

func gameplayIdentityPhase(phase string) bool {
	return phase == "InProgress" || phase == "Reconnect" || phase == "GameStart"
}

// This probe reads identity only, never players/recommendations. A fresh gameId
// survives a missed phase window even after the full live roster stops polling.
func readGameplayPhaseIdentity(ctx context.Context, client *LCUClient) gameplayPhaseIdentity {
	result := gameplayPhaseIdentity{Phase: "None", Connected: true}
	if client.RequestJSON(ctx, http.MethodGet, "/lol-gameflow/v1/gameflow-phase", nil, &result.Phase) != nil {
		return result
	}
	if !gameplayIdentityPhase(result.Phase) {
		return result // ChampSelect gameData may still identify the previous match.
	}
	probeCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	var session lcuGameflowSession
	if client.RequestJSON(probeCtx, http.MethodGet, "/lol-gameflow/v1/session", nil, &session) == nil {
		if session.Phase != "" {
			result.Phase = session.Phase
		}
		if gameplayIdentityPhase(result.Phase) && session.GameData.GameID > 0 {
			result.GameID = session.GameData.GameID
		}
	}
	return result
}

// Reuse session event data; do not block the LCU event reader on an extra HTTP
// request. Legacy phase-only events remain available for all other consumers.
func (a *app) broadcastGameplayIdentity(event LCUEvent) {
	if !strings.EqualFold(event.URI, "/lol-gameflow/v1/session") {
		return
	}
	var session lcuGameflowSession
	if json.Unmarshal(event.Data, &session) != nil || !gameplayIdentityPhase(session.Phase) || session.GameData.GameID <= 0 {
		return
	}
	payload, _ := json.Marshal(gameplayPhaseIdentity{Phase: session.Phase, Connected: true, GameID: session.GameData.GameID})
	a.broadcastEvent("gameflow:" + string(payload))
}

type gameflowClientObservation struct {
	Reason           string `json:"reason"`
	Phase            string `json:"phase"`
	PreviousPhase    string `json:"previousPhase"`
	GameID           int64  `json:"gameId"`
	CachedGameID     int64  `json:"cachedGameId"`
	GameIDComparison string `json:"gameIdComparison"`
	Source           string `json:"source"`
	PhaseChanged     bool   `json:"phaseChanged"`
	Invalidated      bool   `json:"invalidated"`
	ReceivedAt       int64  `json:"receivedAt"`
}

var gameflowPhaseDiagnosticPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

func (a *app) recordGameflowClientObservations(request clientDiagnosticRequest) bool {
	observations := request.Observations
	if len(observations) == 0 || len(observations) > 8 {
		return false
	}
	for _, observation := range observations {
		if observation.Phase != "" && !gameflowPhaseDiagnosticPattern.MatchString(observation.Phase) || observation.PreviousPhase != "" && !gameflowPhaseDiagnosticPattern.MatchString(observation.PreviousPhase) {
			return false
		}
		switch observation.Source {
		case "direct", "event", "sse", "poll", "resync", "interval", "manual":
		default:
			return false
		}
		switch observation.Reason {
		case "received", "invalidate", "stale-response":
		default:
			return false
		}
		if observation.GameID < 0 || observation.CachedGameID < 0 || observation.GameID > 9007199254740991 || observation.CachedGameID > 9007199254740991 {
			return false
		}
	}
	for _, observation := range observations {
		comparison := "same"
		switch {
		case observation.GameID == 0:
			comparison = "unavailable"
		case observation.CachedGameID == 0:
			comparison = "first"
		case observation.GameID != observation.CachedGameID:
			comparison = "different"
		}
		a.recordDiagnostic(map[string]any{
			"event": "gameflow_phase_client", "reason": observation.Reason,
			"phase": observation.Phase, "previous_phase": observation.PreviousPhase, "source": observation.Source,
			"game_id": observation.GameID, "cached_game_id": observation.CachedGameID, "game_id_comparison": comparison,
			"phase_changed": observation.PhaseChanged, "invalidated": observation.Invalidated,
			"received_at":       min(int64(1e13), max(int64(0), observation.ReceivedAt)),
			"transport_failed":  min(1000000, max(0, request.TransportFailed)),
			"transport_dropped": min(1000000, max(0, request.TransportDropped)),
		})
	}
	return true
}
