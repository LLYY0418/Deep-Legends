package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"syscall"
	"time"
)

const (
	mayhemSampleInterval = 10 * time.Second
	mayhemSampleBudget   = 5 * time.Minute
	mayhemRequestBudget  = 2 * time.Second
)

func mayhemWait(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func mayhemSampleErrorKind(err error, status int, valid bool) string {
	if errors.Is(err, syscall.ECONNREFUSED) {
		return "connection-refused"
	}
	var networkError net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &networkError) && networkError.Timeout() {
		return "timeout"
	}
	if status != http.StatusOK && status != 0 {
		return "http-" + strconv.Itoa(status)
	}
	if status == http.StatusOK && !valid {
		return "invalid-json"
	}
	return "unavailable"
}

// A Mayhem game owns one diagnostic sampler. The request context is never reused:
// the first frontend request normally ends before the live client opens port 2999.
func (a *app) startMayhemSampler(client *LCUClient, gameID int64, phase string) {
	if a == nil || gameID == 0 {
		return
	}
	a.mayhemSamplerMu.Lock()
	if a.mayhemSamplerGameID == gameID && (a.mayhemSamplerCancel != nil || a.mayhemSamplerDoneGameID == gameID) {
		a.mayhemSamplerMu.Unlock()
		return
	}
	if a.mayhemSamplerCancel != nil {
		a.mayhemSamplerCancel(errors.New("new-game"))
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	a.mayhemSamplerGameID = gameID
	a.mayhemSamplerClient = client
	a.mayhemSamplerCancel = cancel
	a.mayhemSamplerMu.Unlock()
	go func() {
		a.runMayhemSampler(ctx, gameID, phase)
		a.mayhemSamplerMu.Lock()
		if a.mayhemSamplerGameID == gameID {
			a.mayhemSamplerDoneGameID = gameID
			a.mayhemSamplerCancel = nil
		}
		a.mayhemSamplerMu.Unlock()
	}()
}

func (a *app) stopMayhemSampler(reason string) {
	if a == nil {
		return
	}
	a.mayhemSamplerMu.Lock()
	if a.mayhemSamplerCancel != nil {
		a.mayhemSamplerDoneGameID = a.mayhemSamplerGameID
		a.mayhemSamplerCancel(errors.New(reason))
		a.mayhemSamplerCancel = nil
	}
	a.mayhemSamplerMu.Unlock()
}

func (a *app) stopMayhemSamplerForClient(reason string, client *LCUClient) {
	if a == nil {
		return
	}
	a.mayhemSamplerMu.Lock()
	if a.mayhemSamplerClient == client && a.mayhemSamplerCancel != nil {
		a.mayhemSamplerDoneGameID = a.mayhemSamplerGameID
		a.mayhemSamplerCancel(errors.New(reason))
		a.mayhemSamplerCancel = nil
	}
	a.mayhemSamplerMu.Unlock()
}

func (a *app) runMayhemSampler(ctx context.Context, gameID int64, phase string) {
	now := a.mayhemSamplerNow
	if now == nil {
		now = time.Now
	}
	wait := a.mayhemSamplerWait
	if wait == nil {
		wait = mayhemWait
	}
	started := now()
	deadline := started.Add(mayhemSampleBudget)
	allSucceeded, playersSucceeded := false, false
	allAttempts, playerAttempts := 0, 0
	allError, playerError := "", ""
	allElapsed, playerElapsed := int64(-1), int64(-1)
	reason := "timeout"
	defer func() {
		a.recordDiagnostic(map[string]any{
			"event": "live_client_mayhem_sample_summary", "game_id": gameID, "phase": phase,
			"allgamedata_success": allSucceeded, "playerlist_success": playersSucceeded,
			"allgamedata_attempts": allAttempts, "playerlist_attempts": playerAttempts,
			"allgamedata_error_kind": allError, "playerlist_error_kind": playerError,
			"allgamedata_elapsed_ms": allElapsed, "playerlist_elapsed_ms": playerElapsed,
			"end_reason": reason,
		})
	}()
	for {
		if ctx.Err() != nil {
			reason = "gameflow-left"
			if cause := context.Cause(ctx); cause != nil {
				reason = cause.Error()
			}
			return
		}
		if !now().Before(deadline) {
			return
		}
		if !allSucceeded {
			allAttempts++
			requestCtx, cancel := context.WithTimeout(ctx, mayhemRequestBudget)
			raw, status, err := a.fetchLiveClientAllGameData(requestCtx)
			cancel()
			if ctx.Err() != nil {
				continue
			}
			valid := json.Valid(raw)
			if err == nil && status == http.StatusOK && valid {
				allSucceeded = true
				allElapsed = now().Sub(started).Milliseconds()
				payload := liveClientAllGameDataShape(raw)
				payload["event"], payload["game_id"] = "live_client_allgamedata_shape", gameID
				payload["http_status"], payload["success"] = status, true
				payload["attempt"], payload["elapsed_ms"] = allAttempts, allElapsed
				a.recordDiagnostic(payload)
			} else {
				allError = mayhemSampleErrorKind(err, status, valid)
			}
		}
		if !playersSucceeded {
			playerAttempts++
			requestCtx, cancel := context.WithTimeout(ctx, mayhemRequestBudget)
			raw, status, err := a.loadLiveClientPlayerList(requestCtx)
			cancel()
			if ctx.Err() != nil {
				continue
			}
			valid := json.Valid(raw)
			var shape liveClientPlayerListShape
			if err == nil && status == http.StatusOK && valid {
				_, shape, err = parseLiveClientPlayerList(raw, 3)
				valid = err == nil
			}
			if err == nil && status == http.StatusOK && valid {
				playersSucceeded = true
				playerElapsed = now().Sub(started).Milliseconds()
				result := "ungrouped"
				if shape.Grouped {
					result = "grouped"
				}
				a.recordDiagnostic(map[string]any{
					"event": "live_client_playerlist_shape", "game_id": gameID, "phase": phase,
					"result": result, "http_status": status, "success": true,
					"attempt": playerAttempts, "elapsed_ms": playerElapsed,
					"player_count": shape.PlayerCount, "element_keys": shape.ElementKeys,
					"group_field": shape.GroupField, "grouped": shape.Grouped,
				})
			} else {
				playerError = mayhemSampleErrorKind(err, status, valid)
			}
		}
		if allSucceeded && playersSucceeded {
			reason = "success"
			return
		}
		remaining := mayhemSampleInterval
		if until := deadline.Sub(now()); until < remaining {
			remaining = until
		}
		if remaining <= 0 {
			return
		}
		if !wait(ctx, remaining) {
			reason = "gameflow-left"
			if cause := context.Cause(ctx); cause != nil {
				reason = cause.Error()
			}
			return
		}
	}
}
