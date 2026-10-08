package main

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

const clientShutdownGrace = 15 * time.Second

type clientCloseTimeline struct {
	client       *LCUClient
	started      time.Time
	tabRemovedMS int64
	restoredFlip int
	overlayShown int
}

func (a *app) beginClientCloseLocked(client *LCUClient, now time.Time) {
	if a.clientClose.client != client || a.clientClose.started.IsZero() || a.clientClose.restoredFlip > 0 {
		a.clientClose = clientCloseTimeline{client: client, started: now, tabRemovedMS: -1}
	}
}

func (a *app) recordClientTabRemoved(now time.Time) {
	a.mu.Lock()
	if a.clientClose.started.IsZero() || now.Before(a.clientClose.started) {
		a.mu.Unlock()
		return
	}
	a.clientClose.tabRemovedMS = max(0, now.Sub(a.clientClose.started).Milliseconds())
	event := a.clientClose.event()
	a.mu.Unlock()
	a.recordDiagnostic(event)
}

func (s clientCloseTimeline) event() map[string]any {
	return map[string]any{"event": "client_close_timeline", "shutdown_signal_ms": int64(0), "shutdown_signal_at": s.started.UnixMilli(), "tab_removed_ms": s.tabRemovedMS, "overlay_shown": s.overlayShown, "restored_flip": s.restoredFlip}
}

func (a *app) clientShutdownPending(client *LCUClient) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.shutdownClient == client && client != nil
}

// Keep only URI structure, never dynamic account/game parameters or payloads.
func shutdownDiagnosticURI(uri string) string {
	uri = strings.SplitN(uri, "?", 2)[0]
	// Preserve only known fixed signals. Other event URIs retain a known
	// namespace and version; every remaining segment may contain identity.
	switch uri {
	case "/riotclient/pre-shutdown/begin", "/process-control/v1/process", "/lol-chat/v1/session", "/lol-chat/v1/friends", "/lol-gameflow/v1/gameflow-phase", "/lol-champ-select/v1/session", "/lol-summoner/v1/current-summoner":
		return uri
	}
	parts := strings.Split(uri, "/")
	if len(parts) < 2 {
		return "/{route}"
	}
	switch parts[1] {
	case "riotclient", "process-control", "lol-chat", "lol-gameflow", "lol-champ-select", "lol-summoner", "lol-match-history", "lol-loot", "lol-inventory", "lol-ranked", "lol-login", "lol-platform-config":
		prefix := "/" + parts[1]
		if len(parts) > 2 && len(parts[2]) == 2 && parts[2][0] == 'v' && parts[2][1] >= '1' && parts[2][1] <= '9' {
			prefix += "/" + parts[2]
		}
		return prefix + "/{arg}"
	default:
		return "/{route}"
	}
}
func (a *app) observeClientShutdownEvent(client *LCUClient, event LCUEvent, now time.Time) {
	a.mu.Lock()
	if a.lcu != client {
		a.mu.Unlock()
		return
	}
	a.clientLastEventAt = now
	if a.shutdownEvents == nil {
		a.shutdownEvents = map[string]time.Time{}
	}
	for uri, at := range a.shutdownEvents {
		if now.Sub(at) > 10*time.Second {
			delete(a.shutdownEvents, uri)
		}
	}
	uri := shutdownDiagnosticURI(event.URI)
	if len(a.shutdownEvents) < 30 || !a.shutdownEvents[uri].IsZero() {
		a.shutdownEvents[uri] = now
	}
	reason := ""
	switch strings.ToLower(event.URI) {
	case "/riotclient/pre-shutdown/begin":
		reason = "pre-shutdown"
	case "/process-control/v1/process":
		var payload struct {
			Status string `json:"status"`
		}
		if json.Unmarshal(event.Data, &payload) == nil && strings.EqualFold(payload.Status, "Stopping") {
			reason = "process-stopping"
		}
	case "/lol-chat/v1/session":
		var payload struct {
			State        string `json:"state"`
			Status       string `json:"status"`
			Availability string `json:"availability"`
		}
		if json.Unmarshal(event.Data, &payload) == nil {
			a.shutdownChatOfflineAt = time.Time{}
			if strings.EqualFold(payload.State, "offline") || strings.EqualFold(payload.Status, "offline") || strings.EqualFold(payload.Availability, "offline") {
				a.shutdownChatOfflineAt = now
			}
		}
	case "/lol-chat/v1/friends":
		var rows []json.RawMessage
		if json.Unmarshal(event.Data, &rows) == nil {
			a.shutdownFriendsEmptyAt = time.Time{}
			if len(rows) == 0 {
				a.shutdownFriendsEmptyAt = now
			}
		}
	}
	if reason == "" && !a.shutdownChatOfflineAt.IsZero() && !a.shutdownFriendsEmptyAt.IsZero() && now.Sub(a.shutdownChatOfflineAt) <= 10*time.Second && now.Sub(a.shutdownFriendsEmptyAt) <= 10*time.Second {
		reason = "chat-offline-empty"
	}
	if reason == "" || a.shutdownClient == client {
		a.mu.Unlock()
		return
	}
	a.shutdownClient = client
	a.beginClientCloseLocked(client, now)
	a.shutdownAt = now
	a.connectionState = "client-exiting"
	if a.shutdownTimer != nil {
		a.shutdownTimer.Stop()
	}
	a.shutdownTimer = time.AfterFunc(clientShutdownGrace, func() { a.restoreClientShutdown(client, time.Now()) })
	a.mu.Unlock()
	a.recordDiagnostic(map[string]any{"event": "client_shutdown_trace", "reason": reason})
	a.publishClientView()
	a.signalGameplayPhase(client)
}
func (a *app) restoreClientShutdown(client *LCUClient, now time.Time) bool {
	a.mu.Lock()
	if a.lcu != client || !a.connected || a.shutdownClient != client || now.Sub(a.shutdownAt) < clientShutdownGrace || !a.eventStream || a.clientLastEventAt.IsZero() || now.Sub(a.clientLastEventAt) > 5*time.Second {
		a.mu.Unlock()
		return false
	}
	a.shutdownClient = nil
	a.shutdownChatOfflineAt = time.Time{}
	a.shutdownFriendsEmptyAt = time.Time{}
	a.shutdownTimer = nil
	a.connectionState = "connected"
	a.clientClose.restoredFlip++
	closeEvent := a.clientClose.event()
	a.mu.Unlock()
	a.recordDiagnostic(closeEvent)
	a.recordDiagnostic(map[string]any{"event": "client_shutdown_trace", "reason": "false-positive-restored"})
	a.publishClientView()
	a.signalGameplayPhase(client)
	return true
}
func (a *app) recordClientShutdownTrace(client *LCUClient, now time.Time) {
	a.mu.RLock()
	entries := []map[string]any{}
	for uri, at := range a.shutdownEvents {
		if now.Sub(at) <= 10*time.Second {
			entries = append(entries, map[string]any{"uri": uri, "relative_ms": at.Sub(now).Milliseconds()})
		}
	}
	a.mu.RUnlock()
	sort.Slice(entries, func(i, j int) bool { return entries[i]["relative_ms"].(int64) < entries[j]["relative_ms"].(int64) })
	a.recordDiagnostic(map[string]any{"event": "client_shutdown_trace", "reason": "event-stream-disconnected", "events": entries})
}
