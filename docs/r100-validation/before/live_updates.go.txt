package main

import (
	"encoding/json"
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
	backgroundID, backgroundName, backgroundSource, backgroundPath := profileBackground(a.account.Profile)
	response := struct {
		Summoner publicSummoner `json:"summoner"`
		Account  AccountData    `json:"account"`
	}{
		Summoner: publicSummoner{DisplayName: a.summoner.DisplayName, GameName: a.summoner.GameName, TagLine: a.summoner.TagLine, ProfileIconID: a.summoner.ProfileIconID, SummonerLevel: a.summoner.SummonerLevel, BackgroundSkinID: backgroundID, BackgroundSkinName: backgroundName, BackgroundSource: backgroundSource, BackgroundPath: backgroundPath},
		Account:  cloneAccountData(a.account),
	}
	response.Summoner.BackgroundPosterPath, response.Summoner.BackgroundVideoPath = overviewSkinMedia(a.allSkins, backgroundID)
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
	updates := make(chan string, 32)
	a.eventMu.Lock()
	a.eventSubscribers[updates] = struct{}{}
	a.eventMu.Unlock()
	defer func() {
		a.eventMu.Lock()
		delete(a.eventSubscribers, updates)
		a.eventMu.Unlock()
	}()
	_, _ = fmt.Fprint(w, "data: ready\n\n")
	// Include an updater snapshot on initial subscribe as well as reconnect;
	// a check may have finished between the renderer's status fetch and SSE.
	if a.updates != nil {
		data, _ := json.Marshal(a.updates.Status())
		if err := writeLiveEvent(w, "update:status\n"+string(data)); err != nil {
			return
		}
	}
	flusher.Flush()
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-updates:
			if err := writeLiveEvent(w, event); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\nevent: heartbeat\ndata: ready\n\n"); err != nil {
				return
			}
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
			// A slow renderer must recover *all* dirty slices, not silently lose
			// the only connection/claim/roster notification. Coalesce overflow.
			draining := true
			for draining {
				select {
				case <-subscriber:
				default:
					draining = false
				}
			}
			select {
			case subscriber <- "resync-required":
			default: // Only possible for an unbuffered test subscriber.
			}
		}
	}
}
