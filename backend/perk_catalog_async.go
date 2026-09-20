package main

import (
	"context"
	"encoding/json"
	"time"
)

// Augments are optional and independent of rune styles. Never hold the catalog
// mutex over network I/O; publish a new value, not mutations of a response slice.
func (a *app) enrichPerkAugments(key string, client *LCUClient, payload gameplayPerkCatalogResponse) gameplayPerkCatalogResponse {
	a.perkCatalogMu.Lock()
	defer a.perkCatalogMu.Unlock()
	if len(payload.Augments) > 0 {
		return payload
	}
	if a.perkAugmentJobs == nil {
		a.perkAugmentJobs = make(map[string]chan struct{})
		a.perkAugmentAttempts = make(map[string]time.Time)
	}
	if a.perkAugmentJobs[key] != nil {
		payload.AugmentsPending = true
		return payload
	}
	if time.Since(a.perkAugmentAttempts[key]) < time.Minute {
		return payload
	}
	a.perkAugmentAttempts[key] = time.Now()
	done := make(chan struct{})
	a.perkAugmentJobs[key] = done
	cache := a.perkCatalogDisk
	generation := a.perkCatalog[key].loadedAt
	payload.AugmentsPending = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		data, err := cache.load(ctx, "normalized-augments-v1|"+key, gameplayPerkCatalogTTL, 24*time.Hour, true, func(ctx context.Context) ([]byte, error) {
			var augments []gameplayAugment
			var err error
			if client != nil {
				var raw []gameplayAugmentRaw
				err = client.GetJSONContext(ctx, "/lol-game-data/assets/v1/cherry-augments.json", &raw)
				if err == nil {
					augments = normalizeGameplayAugments(raw)
				}
			}
			if client == nil || err != nil {
				augments, err = a.fallbackGameplayAugments(ctx)
			}
			if err != nil {
				return nil, err
			}
			return json.Marshal(augments)
		})
		var augments []gameplayAugment
		if err == nil {
			err = json.Unmarshal(data, &augments)
		}
		a.perkCatalogMu.Lock()
		if entry, ok := a.perkCatalog[key]; ok && entry.loadedAt == generation && err == nil {
			entry.payload.Augments = augments
			a.perkCatalog[key] = entry
		}
		delete(a.perkAugmentJobs, key)
		close(done)
		a.perkCatalogMu.Unlock()
	}()
	return payload
}
