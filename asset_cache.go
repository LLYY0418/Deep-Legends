package main

import (
	"context"
	"errors"
	"time"
)

const (
	assetCacheMaxEntries = 1200
	assetCacheMaxBytes   = 256 * 1024 * 1024
)

var errCachedAssetFailure = errors.New("asset temporarily unavailable")

type assetFlight struct {
	done       chan struct{}
	data       []byte
	err        error
	generation uint64
}

func (a *app) loadAsset(ctx context.Context, key string, maxEntrySize int, negativeTTL time.Duration, loader func(context.Context) ([]byte, error)) ([]byte, error) {
	a.assetCacheMu.Lock()
	if data, ok := a.assetCache[key]; ok {
		a.assetCacheMu.Unlock()
		return data, nil
	}
	if until := a.assetFailureUntil[key]; !until.IsZero() {
		if time.Now().Before(until) {
			a.assetCacheMu.Unlock()
			return nil, errCachedAssetFailure
		}
		delete(a.assetFailureUntil, key)
	}
	if flight := a.assetFlights[key]; flight != nil {
		done := flight.done
		a.assetCacheMu.Unlock()
		select {
		case <-done:
			return flight.data, flight.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if a.assetFlights == nil {
		a.assetFlights = make(map[string]*assetFlight)
	}
	flight := &assetFlight{done: make(chan struct{}), generation: a.assetCacheGeneration}
	a.assetFlights[key] = flight
	a.assetCacheMu.Unlock()

	data, err := loader(ctx)

	a.assetCacheMu.Lock()
	flight.data = data
	flight.err = err
	cacheIsCurrent := flight.generation == a.assetCacheGeneration
	if err == nil && cacheIsCurrent {
		delete(a.assetFailureUntil, key)
		if maxEntrySize > 0 && len(data) <= maxEntrySize {
			a.storeAssetLocked(key, data)
		}
	} else if err != nil && cacheIsCurrent && negativeTTL > 0 && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		if a.assetFailureUntil == nil {
			a.assetFailureUntil = make(map[string]time.Time)
		}
		a.assetFailureUntil[key] = time.Now().Add(negativeTTL)
	}
	delete(a.assetFlights, key)
	close(flight.done)
	a.assetCacheMu.Unlock()
	return data, err
}

func (a *app) storeAssetLocked(key string, data []byte) {
	if a.assetCache == nil {
		a.assetCache = make(map[string][]byte)
	}
	if previous, ok := a.assetCache[key]; ok {
		a.assetCacheBytes -= len(previous)
	} else {
		a.assetCacheOrder = append(a.assetCacheOrder, key)
	}
	for len(a.assetCache) >= assetCacheMaxEntries || a.assetCacheBytes+len(data) > assetCacheMaxBytes {
		if len(a.assetCacheOrder) > 0 {
			oldest := a.assetCacheOrder[0]
			a.assetCacheOrder = a.assetCacheOrder[1:]
			if previous, ok := a.assetCache[oldest]; ok {
				a.assetCacheBytes -= len(previous)
				delete(a.assetCache, oldest)
			}
			continue
		}
		removed := false
		for existingKey, previous := range a.assetCache {
			a.assetCacheBytes -= len(previous)
			delete(a.assetCache, existingKey)
			removed = true
			break
		}
		if !removed {
			break
		}
	}
	a.assetCache[key] = data
	a.assetCacheBytes += len(data)
}
