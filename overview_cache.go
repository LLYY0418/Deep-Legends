package main

import (
	"container/list"
	"context"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	overviewQuerySnapshotTTL = 2 * time.Second
	overviewQueryCacheMax    = 256
)

type overviewQueryCacheEntry struct {
	at       time.Time
	response gameplayOverview
}

type overviewQueryCacheItem struct {
	key   string
	entry overviewQueryCacheEntry
}

type overviewQueryFlight struct {
	done     chan struct{}
	response gameplayOverview
	err      error
}

func riotOverviewQuerySnapshotKey(reference gameplayReference, begIndex, count int) string {
	identity := strings.TrimSpace(reference.PlayerRef)
	if identity == "" {
		identity = strings.ToLower(strings.TrimSpace(reference.GameName)) + "#" + strings.ToLower(strings.TrimSpace(reference.TagLine))
	}
	return sourceScopedKey(dataSourceRiot, strings.Join([]string{identity, strconv.Itoa(begIndex), strconv.Itoa(count), strings.ToUpper(strings.TrimSpace(reference.Privacy))}, "|"))
}

func (a *app) loadRiotOverviewDeduplicated(ctx context.Context, reference gameplayReference, begIndex, count int, force bool) (gameplayOverview, error) {
	if force || a.overviewQueries == nil {
		return a.loadRiotOverview(ctx, reference, begIndex, count)
	}
	key := riotOverviewQuerySnapshotKey(reference, begIndex, count)
	a.overviewQueries.mu.Lock()
	if cached, ok := a.overviewQueries.getLocked(key, time.Now()); ok {
		response := cached.response
		a.overviewQueries.mu.Unlock()
		return response, nil
	}
	if flight := a.overviewQueries.flights[key]; flight != nil {
		done := flight.done
		a.overviewQueries.mu.Unlock()
		select {
		case <-done:
			return flight.response, flight.err
		case <-ctx.Done():
			return gameplayOverview{}, ctx.Err()
		}
	}
	flight := &overviewQueryFlight{done: make(chan struct{})}
	a.overviewQueries.flights[key] = flight
	a.overviewQueries.mu.Unlock()

	response, err := a.loadRiotOverview(ctx, reference, begIndex, count)
	a.overviewQueries.mu.Lock()
	flight.response, flight.err = response, err
	if err == nil {
		a.overviewQueries.putLocked(key, overviewQueryCacheEntry{at: time.Now(), response: response})
	}
	delete(a.overviewQueries.flights, key)
	close(flight.done)
	a.overviewQueries.mu.Unlock()
	return response, err
}

type overviewQueryCache struct {
	mu      sync.Mutex
	entries map[string]*list.Element
	recent  *list.List
	flights map[string]*overviewQueryFlight
}

func newOverviewQueryCache() *overviewQueryCache {
	return &overviewQueryCache{entries: make(map[string]*list.Element), recent: list.New(), flights: make(map[string]*overviewQueryFlight)}
}

func (cache *overviewQueryCache) getLocked(key string, now time.Time) (overviewQueryCacheEntry, bool) {
	element, ok := cache.entries[key]
	if !ok {
		return overviewQueryCacheEntry{}, false
	}
	item := element.Value.(overviewQueryCacheItem)
	if now.Sub(item.entry.at) >= overviewQuerySnapshotTTL {
		cache.removeElementLocked(element)
		return overviewQueryCacheEntry{}, false
	}
	cache.recent.MoveToFront(element)
	return item.entry, true
}

func (cache *overviewQueryCache) putLocked(key string, entry overviewQueryCacheEntry) {
	cache.removeExpiredLocked(entry.at)
	if element, ok := cache.entries[key]; ok {
		element.Value = overviewQueryCacheItem{key: key, entry: entry}
		cache.recent.MoveToFront(element)
		return
	}
	cache.entries[key] = cache.recent.PushFront(overviewQueryCacheItem{key: key, entry: entry})
	for len(cache.entries) > overviewQueryCacheMax {
		cache.removeElementLocked(cache.recent.Back())
	}
}

func (cache *overviewQueryCache) removeExpiredLocked(now time.Time) {
	for element := cache.recent.Back(); element != nil; {
		previous := element.Prev()
		item := element.Value.(overviewQueryCacheItem)
		if now.Sub(item.entry.at) >= overviewQuerySnapshotTTL {
			cache.removeElementLocked(element)
		}
		element = previous
	}
}

func (cache *overviewQueryCache) removeElementLocked(element *list.Element) {
	if element == nil {
		return
	}
	item := element.Value.(overviewQueryCacheItem)
	delete(cache.entries, item.key)
	cache.recent.Remove(element)
}

func (cache *overviewQueryCache) complete(key string, flight *overviewQueryFlight, response gameplayOverview, err error) {
	cache.mu.Lock()
	flight.response, flight.err = response, err
	if err == nil {
		cache.putLocked(key, overviewQueryCacheEntry{at: time.Now(), response: response})
	}
	delete(cache.flights, key)
	close(flight.done)
	cache.mu.Unlock()
}

func overviewQuerySnapshotKey(client *LCUClient, reference gameplayReference, playerRef string, isCurrent bool, begIndex, count int, matchFilter string) string {
	source := dataSourceSGP + "+" + dataSourceLCU
	if isRemoteTencentServer(client, reference.ServerID) {
		source = dataSourceSGP
	}
	key := strings.Join([]string{
		strings.ToUpper(strings.TrimSpace(reference.ServerID)), strings.TrimSpace(playerRef),
		strconv.FormatBool(isCurrent), strconv.Itoa(begIndex), strconv.Itoa(count), normalizeGameplayMatchFilter(matchFilter), strings.ToUpper(strings.TrimSpace(reference.Privacy)),
	}, "|")
	return sourceScopedKey(source, key)
}

func (a *app) loadGameplayOverviewDeduplicated(ctx context.Context, client *LCUClient, current Summoner, reference gameplayReference, begIndex, count int, matchFilter string, force bool) gameplayOverview {
	if force || a.overviewQueries == nil {
		return a.loadGameplayOverview(ctx, client, current, reference, begIndex, count, matchFilter)
	}
	playerRef := strings.TrimSpace(reference.PlayerRef)
	isCurrent := (playerRef == "" || gameplayReferenceContains(reference, current.PUUID)) && !isRemoteTencentServer(client, reference.ServerID)
	if playerRef == "" {
		playerRef = current.PUUID
	}
	key := overviewQuerySnapshotKey(client, reference, playerRef, isCurrent, begIndex, count, matchFilter)
	a.overviewQueries.mu.Lock()
	if cached, ok := a.overviewQueries.getLocked(key, time.Now()); ok {
		response := cached.response
		a.overviewQueries.mu.Unlock()
		return response
	}
	if flight := a.overviewQueries.flights[key]; flight != nil {
		done := flight.done
		a.overviewQueries.mu.Unlock()
		select {
		case <-done:
			if flight.err != nil {
				if ctx.Err() != nil {
					return gameplayOverview{}
				}
				return a.loadGameplayOverviewDeduplicated(ctx, client, current, reference, begIndex, count, matchFilter, false)
			}
			return flight.response
		case <-ctx.Done():
			return gameplayOverview{}
		}
	}
	flight := &overviewQueryFlight{done: make(chan struct{})}
	a.overviewQueries.flights[key] = flight
	a.overviewQueries.mu.Unlock()

	response := a.loadGameplayOverview(ctx, client, current, reference, begIndex, count, matchFilter)
	a.overviewQueries.complete(key, flight, response, ctx.Err())
	return response
}

func (a *app) clearOverviewQuerySnapshots() {
	if a.overviewQueries == nil {
		return
	}
	a.overviewQueries.mu.Lock()
	a.overviewQueries.entries = make(map[string]*list.Element)
	a.overviewQueries.recent.Init()
	a.overviewQueries.mu.Unlock()
}
