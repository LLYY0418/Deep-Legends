package main

import (
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"sync"
)

const historyGameCacheMax = 512

// Completed games are account-independent immutable data. Keep the source and
// server in the key: numeric game IDs alone are not globally unique. Serialized
// values and copied missing-field masks prevent callers mutating cached rosters.
type historyGameValue struct {
	key            string
	data           []byte
	missing        []map[string]bool
	perkStatsStale bool
	complete       bool
}
type historyGameFlight struct {
	done chan struct{}
	err  error
}
type historyGameCache struct {
	mu      sync.Mutex
	entries map[string]*list.Element
	order   *list.List
	flights map[string]*historyGameFlight
}

func newHistoryGameCache() *historyGameCache {
	return &historyGameCache{entries: map[string]*list.Element{}, order: list.New(), flights: map[string]*historyGameFlight{}}
}
func historyGameKey(source, server string, id int64) string {
	return sourceScopedKey(source, fmt.Sprintf("%s|%d", strings.ToUpper(strings.TrimSpace(server)), id))
}
func (c *historyGameCache) get(key string, complete bool) (historyGameValue, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	element := c.entries[key]
	if element == nil {
		return historyGameValue{}, false
	}
	value := element.Value.(historyGameValue)
	if complete && !value.complete {
		return historyGameValue{}, false
	}
	c.order.MoveToFront(element)
	return value, true
}
func (c *historyGameCache) put(value historyGameValue) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if element := c.entries[value.key]; element != nil {
		previous := element.Value.(historyGameValue)
		if previous.complete {
			c.order.MoveToFront(element)
			return
		}
		element.Value = value
		c.order.MoveToFront(element)
	} else {
		c.entries[value.key] = c.order.PushFront(value)
	}
	for len(c.entries) > historyGameCacheMax {
		element := c.order.Back()
		delete(c.entries, element.Value.(historyGameValue).key)
		c.order.Remove(element)
	}
}
func (c *historyGameCache) putSGP(server string, game *riotMatchInfo) {
	if game == nil || game.GameID <= 0 {
		return
	}
	raw, err := json.Marshal(game)
	if err != nil {
		return
	}
	missing := make([]map[string]bool, len(game.Participants))
	for i, p := range game.Participants {
		missing[i] = maps.Clone(p.scoreMissing)
	}
	summary := summarizeRiotParticipants([]*riotMatchInfo{game})
	c.put(historyGameValue{key: historyGameKey(dataSourceSGP, server, game.GameID), data: raw, missing: missing, perkStatsStale: game.PerkStatsStale, complete: len(game.Participants) > 0 && summary.Incomplete == 0 && summary.MissingPlayerRefs == 0})
}
func (c *historyGameCache) sgp(server string, id int64, requireComplete ...bool) (*riotMatchInfo, bool) {
	value, ok := c.get(historyGameKey(dataSourceSGP, server, id), len(requireComplete) > 0 && requireComplete[0])
	if !ok {
		return nil, false
	}
	var game riotMatchInfo
	if json.Unmarshal(value.data, &game) != nil {
		return nil, false
	}
	game.PerkStatsStale = value.perkStatsStale
	for i := range game.Participants {
		if i < len(value.missing) {
			game.Participants[i].scoreMissing = maps.Clone(value.missing[i])
		}
	}
	return &game, true
}
func (c *historyGameCache) putLCU(scope string, game lcuGame) {
	if game.GameID <= 0 {
		return
	}
	raw, err := json.Marshal(game)
	if err != nil {
		return
	}
	missing := make([]map[string]bool, len(game.Participants))
	for i, p := range game.Participants {
		missing[i] = maps.Clone(p.scoreMissing)
	}
	c.put(historyGameValue{key: historyGameKey(dataSourceLCU, scope, game.GameID), data: raw, missing: missing, complete: !lcuRosterIncomplete(game)})
}
func (c *historyGameCache) lcu(scope string, id int64, complete bool) (lcuGame, bool) {
	value, ok := c.get(historyGameKey(dataSourceLCU, scope, id), complete)
	if !ok {
		return lcuGame{}, false
	}
	var game lcuGame
	if json.Unmarshal(value.data, &game) != nil {
		return lcuGame{}, false
	}
	for i := range game.Participants {
		if i < len(value.missing) {
			game.Participants[i].scoreMissing = maps.Clone(value.missing[i])
		}
	}
	return game, true
}
func (p *sgpProvider) historyGamesLocked() *historyGameCache {
	if p.historyGames == nil {
		p.historyGames = newHistoryGameCache()
	}
	return p.historyGames
}
func (a *app) bindHistoryGameCache(client *LCUClient) {
	if client == nil {
		return
	}
	a.historyGamesMu.Lock()
	defer a.historyGamesMu.Unlock()
	if a.historyGames == nil {
		if a.sgp != nil {
			a.sgp.mu.Lock()
			a.historyGames = a.sgp.historyGamesLocked()
			a.sgp.mu.Unlock()
		} else {
			a.historyGames = newHistoryGameCache()
		}
	}
	client.mu.Lock()
	client.historyGames = a.historyGames
	client.mu.Unlock()
}
func (client *LCUClient) historyGameStore() (*historyGameCache, string) {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.historyGames == nil {
		client.historyGames = newHistoryGameCache()
	}
	scope := client.region + "|" + client.rsoPlatform
	if client.region == "" {
		scope = fmt.Sprintf("unknown:%p", client)
	}
	return client.historyGames, scope
}
func (client *LCUClient) cachedHistoryDetail(ctx context.Context, id int64) (lcuGame, error) {
	cache, scope := client.historyGameStore()
	key := historyGameKey(dataSourceLCU, scope, id)
	for {
		if game, ok := cache.lcu(scope, id, true); ok {
			return game, nil
		}
		cache.mu.Lock()
		if flight := cache.flights[key]; flight != nil {
			cache.mu.Unlock()
			select {
			case <-ctx.Done():
				return lcuGame{}, ctx.Err()
			case <-flight.done:
				if ctx.Err() == nil && (errors.Is(flight.err, context.Canceled) || errors.Is(flight.err, context.DeadlineExceeded)) {
					continue
				}
				if flight.err != nil {
					return lcuGame{}, flight.err
				}
				if game, ok := cache.lcu(scope, id, false); ok {
					return game, nil
				}
				continue
			}
		}
		if element := cache.entries[key]; element != nil && element.Value.(historyGameValue).complete {
			cache.mu.Unlock()
			continue
		}
		flight := &historyGameFlight{done: make(chan struct{})}
		cache.flights[key] = flight
		cache.mu.Unlock()
		return func() (game lcuGame, resultErr error) {
			resultErr = errors.New("客户端详情读取未完成")
			defer func() {
				cache.mu.Lock()
				flight.err = resultErr
				delete(cache.flights, key)
				close(flight.done)
				cache.mu.Unlock()
			}()
			defer recoverPanic("historyGameCache.detail")
			err := client.GetJSONContext(ctx, fmt.Sprintf("/lol-match-history/v1/games/%d", id), &game)
			if err == nil && game.GameID == id {
				cache.putLCU(scope, game)
			} else if err == nil {
				err = fmt.Errorf("客户端详情的 gameId 与请求不一致")
			}
			resultErr = err
			return
		}()
	}
}
