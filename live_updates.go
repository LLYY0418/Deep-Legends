package main

import (
	"fmt"
	"net/http"
	"time"
)

func cloneAccountData(source AccountData) AccountData {
	result := source
	result.Loot = append([]LootItem(nil), source.Loot...)
	result.Rewards = make([]RewardGrant, len(source.Rewards))
	for i, grant := range source.Rewards {
		result.Rewards[i] = grant
		result.Rewards[i].Items = append([]RewardItem(nil), grant.Items...)
	}
	result.Capabilities = append([]EndpointCapability(nil), source.Capabilities...)
	return result
}

func (a *app) handleAccount(w http.ResponseWriter, _ *http.Request) {
	a.mu.RLock()
	if !a.connected || !a.snapshotReady {
		a.mu.RUnlock()
		http.Error(w, "当前没有可用的客户端快照", http.StatusConflict)
		return
	}
	response := struct {
		Summoner publicSummoner `json:"summoner"`
		Account  AccountData    `json:"account"`
	}{
		Summoner: publicSummoner{DisplayName: a.summoner.DisplayName, GameName: a.summoner.GameName, TagLine: a.summoner.TagLine, ProfileIconID: a.summoner.ProfileIconID, SummonerLevel: a.summoner.SummonerLevel},
		Account:  cloneAccountData(a.account),
	}
	a.mu.RUnlock()
	respondJSON(w, response)
}

func (a *app) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "event streaming unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	updates := make(chan string, 4)
	a.eventMu.Lock()
	a.eventSubscribers[updates] = struct{}{}
	a.eventMu.Unlock()
	defer func() {
		a.eventMu.Lock()
		delete(a.eventSubscribers, updates)
		a.eventMu.Unlock()
	}()
	_, _ = fmt.Fprint(w, "data: ready\n\n")
	flusher.Flush()
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-updates:
			_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
			flusher.Flush()
		case <-heartbeat.C:
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func (a *app) broadcastEvent(event string) {
	a.eventMu.Lock()
	defer a.eventMu.Unlock()
	for subscriber := range a.eventSubscribers {
		select {
		case subscriber <- event:
		default:
		}
	}
}
