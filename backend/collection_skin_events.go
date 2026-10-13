package main

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

func (a *app) observeCollectionSkinEvent(ctx context.Context, client *LCUClient, event LCUEvent) {
	uri := strings.ToLower(event.URI)
	if (!strings.Contains(uri, "inventory") && !strings.Contains(uri, "skins") && !strings.Contains(uri, "loot")) || strings.Contains(uri, "chroma") {
		return
	}
	a.mu.RLock()
	valid := a.lcu == client && a.snapshotReady && a.clientSessionConnectedLocked()
	epoch := a.clientSessionEpoch
	owned := append([]Skin(nil), a.owned...)
	catalog := append([]Skin(nil), a.allSkins...)
	a.mu.RUnlock()
	if !valid {
		return
	}
	started := time.Now()
	a.collectionSkinEventMu.Lock()
	if a.collectionSkinEventEpoch != epoch || a.collectionSkinEventSeen == nil {
		a.collectionSkinEventEpoch = epoch
		a.collectionSkinEventSeen = map[int64]bool{}
		a.collectionSkinEventRunning = false
		a.collectionSkinEventPending = false
		for _, s := range owned {
			a.collectionSkinEventSeen[s.ID] = true
		}
	}
	a.collectionSkinEventMu.Unlock()
	// Ownership payloads can be compared immediately even while a fallback read is running.
	if strings.Contains(uri, "champion_skin") || strings.Contains(uri, "skins-minimal") {
		a.publishCollectionSkinIDs(client, epoch, catalog, event.Data, strings.Contains(uri, "champion_skin"), started)
		return
	}
	a.collectionSkinEventMu.Lock()
	if a.collectionSkinEventRunning {
		a.collectionSkinEventPending = true
		a.collectionSkinEventMu.Unlock()
		return
	}
	a.collectionSkinEventRunning = true
	a.collectionSkinEventMu.Unlock()
	a.goSafe("collection-new-skins", func() {
		for {
			readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			data, err := client.GetBytesContext(readCtx, "/lol-inventory/v2/inventory/CHAMPION_SKIN")
			cancel()
			if err == nil {
				a.publishCollectionSkinIDs(client, epoch, catalog, data, true, started)
			}
			a.collectionSkinEventMu.Lock()
			if a.collectionSkinEventEpoch != epoch {
				a.collectionSkinEventMu.Unlock()
				return
			}
			again := a.collectionSkinEventPending && ctx.Err() == nil
			a.collectionSkinEventPending = false
			if !again {
				a.collectionSkinEventRunning = false
			}
			a.collectionSkinEventMu.Unlock()
			if !again {
				return
			}
			started = time.Now()
		}
	})
}

func (a *app) publishCollectionSkinIDs(client *LCUClient, epoch uint64, catalog []Skin, data []byte, presence bool, started time.Time) {
	var root any
	if json.Unmarshal(data, &root) != nil {
		return
	}
	evidence := extractOwnedEvidence(root, presence)
	validIDs := map[int64]bool{}
	for _, s := range catalog {
		if !isBaseSkin(s) {
			validIDs[s.ID] = true
		}
	}
	a.mu.RLock()
	current := a.lcu == client && a.clientSessionEpoch == epoch && a.clientSessionConnectedLocked()
	a.mu.RUnlock()
	if !current {
		return
	}
	count := 0
	a.collectionSkinEventMu.Lock()
	if a.collectionSkinEventEpoch == epoch {
		for id := range evidence.OwnedIDs {
			if validIDs[id] && !a.collectionSkinEventSeen[id] {
				count++
				a.collectionSkinEventSeen[id] = true
			}
		}
	}
	a.collectionSkinEventMu.Unlock()
	if count > 0 {
		raw, _ := json.Marshal(map[string]any{"type": "collection-new-skins", "count": count, "at": started.UnixMilli()})
		a.broadcastEvent(string(raw))
		a.recordDiagnostic(map[string]any{"event": "collection_new_marker", "kind": "skin", "source": "event", "latency_ms": time.Since(started).Milliseconds()})
	}
}
