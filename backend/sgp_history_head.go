package main

import (
	"context"
	"encoding/json"
	"time"
)

// Expiration means revalidate the mutable index, not discard immutable games.
func (p *sgpProvider) refreshExpiredHistoryPage(ctx context.Context, client *LCUClient, serverID, puuid string, count int) (sgpHistoryPage, bool) {
	key := sgpHistoryPageCacheKey(serverID, puuid, 0, count, nil)
	p.mu.Lock()
	entry, found := p.historyCache[key]
	old, complete := p.materializeHistoryPageLocked(serverID, puuid, entry)
	found = found && complete
	generation := p.historyGeneration
	p.mu.Unlock()
	if !found || time.Since(old.at) < sgpCacheTTL || old.decodeFailed != 0 || count > 20 {
		return sgpHistoryPage{}, false
	}
	cost := overviewCostFromContext(ctx)
	_, before, _, _ := cost.snapshot()
	known := map[int64]bool{}
	for _, game := range old.games {
		if game != nil {
			known[game.GameID] = true
		}
	}
	newGames, verified, hit := 0, false, false
	defer func() {
		_, after, _, _ := cost.snapshot()
		p.recordObservation(map[string]any{"event": "overview_head_probe", "hit": hit, "new_games": newGames, "bytes": after - before, "boundary_verified": verified})
	}()
	for offset := 0; offset < count; offset++ {
		games, consumed, _, err := p.matchHistoryFilteredOn(ctx, client, serverID, puuid, offset, 1, nil, false)
		if err != nil || len(games) != consumed || consumed == 0 && len(old.games) > 0 {
			return sgpHistoryPage{}, false
		}
		if consumed == 0 || games[0] != nil && known[games[0].GameID] {
			verified, hit = true, offset == 0
			break
		}
		newGames++
	}
	if hit {
		p.cacheHistoryPage(serverID, puuid, 0, count, nil, old, generation)
		return old, true
	}
	if newGames == 0 {
		return sgpHistoryPage{}, false
	}
	fresh, consumed, more, err := p.matchHistoryFilteredOn(ctx, client, serverID, puuid, 0, newGames, nil, false)
	if err != nil || len(fresh) != consumed {
		return sgpHistoryPage{}, false
	}
	merged := append(append([]*riotMatchInfo(nil), fresh...), old.games...)
	ids := map[int64]bool{}
	games := merged[:0]
	for _, game := range merged {
		if game != nil && !ids[game.GameID] {
			ids[game.GameID] = true
			games = append(games, game)
		}
	}
	if len(games) > count {
		games = games[:count]
		more = true
	} else if verified {
		more = old.more
	}
	bytes := 0
	for _, game := range games {
		raw, _ := json.Marshal(game)
		bytes += len(raw)
	}
	result := sgpHistoryPage{games: games, consumed: len(games), more: more, bytes: bytes}
	p.cacheHistoryPage(serverID, puuid, 0, count, nil, result, generation)
	return result, true
}
