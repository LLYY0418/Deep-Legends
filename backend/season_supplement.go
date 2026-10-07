package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"time"
)

var seasonSupplementFlights sync.Map

// Career responses are accepted only as actual match-v5 games with a subject,
// queue, game ID and timestamp. Aggregate career totals cannot invent games.
func decodeSeasonSupplement(data []byte, puuid string) []*riotMatchInfo {
	var rows []riotMatchInfo
	if json.Unmarshal(data, &rows) != nil {
		var payload struct {
			Games json.RawMessage `json:"games"`
		}
		if json.Unmarshal(data, &payload) != nil {
			return nil
		}
		if json.Unmarshal(payload.Games, &rows) != nil {
			var nested struct {
				Games []riotMatchInfo `json:"games"`
			}
			if json.Unmarshal(payload.Games, &nested) != nil {
				return nil
			}
			rows = nested.Games
		}
	}
	out := []*riotMatchInfo{}
	for i := range rows {
		row := &rows[i]
		if row.GameID <= 0 || normalizeEpochMillis(row.GameCreation) <= 0 || !seasonStatsQueueAllowed(row.QueueID) {
			continue
		}
		for _, p := range row.Participants {
			if p.PUUID == puuid && p.ChampionID > 0 {
				out = append(out, row)
				break
			}
		}
	}
	if len(out) > 0 {
		return out
	}
	// LCU history uses participant identities plus nested stats. Convert only
	// when the actual subject identity can be matched; no aggregate fallback.
	var history lcuMatchHistory
	if json.Unmarshal(data, &history) != nil {
		return nil
	}
	for _, game := range history.Games.Games {
		match := normalizeGameplayMatch(game, gameplayReference{PlayerRef: puuid}, nil, nil)
		if match.SubjectParticipantID <= 0 || !seasonStatsQueueAllowed(match.QueueID) || match.CreatedAt <= 0 {
			continue
		}
		info := &riotMatchInfo{GameID: match.GameID, GameCreation: match.CreatedAt, GameDuration: match.Duration, QueueID: match.QueueID, GameMode: match.GameMode, GameType: match.GameType, MapID: match.MapID}
		for _, p := range match.Participants {
			info.Participants = append(info.Participants, riotParticipant{ParticipantID: p.ParticipantID, PUUID: p.PlayerRef, TeamID: p.TeamID, ChampionID: p.ChampionID, Kills: p.Kills, Deaths: p.Deaths, Assists: p.Assists, TotalMinionsKilled: p.LaneCS, NeutralMinionsKilled: p.JungleCS, GoldEarned: p.Gold, TotalDamageDealtToChampions: p.Damage, Win: p.Win, TeamPosition: p.Position})
		}
		out = append(out, info)
	}
	return out
}
func (a *app) startSelfSeasonSupplement(client *LCUClient, reference gameplayReference, playerRef string, names map[int64]string) {
	a.mu.RLock()
	self := a.lcu == client && a.summoner.PUUID == playerRef
	a.mu.RUnlock()
	if !self || a.storage == nil || !isTencentClient(client) {
		return
	}
	season, start := currentRankedSeason(time.Now())
	hash := a.storage.accountHash(Summoner{PUUID: playerRef})
	flightKey := fmt.Sprintf("%p|%s|%s", client, hash, season)
	if _, loaded := seasonSupplementFlights.LoadOrStore(flightKey, true); loaded {
		return
	}
	a.goSafe("season.selfSupplement", func() {
		// One completed probe per client session. Transport failures and panics
		// release the guard so a later refresh can retry the real source.
		completed := false
		defer func() {
			if !completed {
				seasonSupplementFlights.Delete(flightKey)
			}
		}()
		sameIdentity := func() bool {
			a.mu.RLock()
			defer a.mu.RUnlock()
			return a.lcu == client && a.summoner.PUUID == playerRef
		}
		failed := false
		cache, err := a.storage.loadSeasonStats(seasonStatsSource, hash, season)
		if err != nil || !cache.CappedByUpstream {
			seasonSupplementFlights.Delete(flightKey)
			return
		}
		base := "/lol-career-stats/v1/summoner-games/" + url.PathEscape(playerRef)
		paths := []string{base, base + "?season=" + fmt.Sprint(start.Year()), base + "?season=" + season, "/lol-match-history/v1/products/lol/" + url.PathEscape(playerRef) + "/matches?begIndex=1000&endIndex=1019"}
		scan := &seasonScanState{cache: cache, stats: map[int64]*gameplaySeasonChampionStat{}, queueStats: cache.QueueStats, seen: map[int64]bool{}, seasonStartMillis: start.UnixMilli(), playerRef: playerRef, newInfos: map[int64]*riotMatchInfo{}}
		if scan.queueStats == nil {
			scan.queueStats = map[int64]gameplayAggregate{}
		}
		for _, row := range cache.Stats {
			copy := row
			scan.stats[row.ChampionID] = &copy
		}
		for _, id := range cache.GameIDs {
			scan.seen[id] = true
		}
		for index, path := range paths {
			ctx, cancel := context.WithTimeout(a.licenseBusinessContext(), 10*time.Second)
			raw, err := client.GetBytesContext(ctx, path)
			cancel()
			if !sameIdentity() {
				return
			}
			if err != nil {
				failed = true
			}
			infos := decodeSeasonSupplement(raw, playerRef)
			added := 0
			oldest := int64(0)
			for _, info := range infos {
				created := normalizeEpochMillis(info.GameCreation)
				if oldest == 0 || created < oldest {
					oldest = created
				}
				if created < start.UnixMilli() || scan.seen[info.GameID] {
					continue
				}
				scan.seen[info.GameID] = true
				scan.cache.GameIDs = append(scan.cache.GameIDs, info.GameID)
				scan.newInfos[info.GameID] = info
				seasonStatsAccumulate(scan.stats, scan.queueStats, info, playerRef, start.UnixMilli())
				seasonAccumulateChampionTable(&scan.cache, info, playerRef, start.UnixMilli())
				seasonRecordRankedMatch(&scan.cache, info, playerRef)
				added++
			}
			category := "career"
			if index == 3 {
				category = "history-1000"
			}
			a.recordDiagnostic(map[string]any{"event": "season_self_source_probe", "source": category, "variant": index, "matches": len(infos), "added": added, "oldest_created_at": oldest, "bytes": len(raw), "failed": err != nil, "reason": safeDiagnosticReason(err)})
		}
		if !sameIdentity() {
			return
		}
		completed = !failed
		if len(scan.newInfos) > 0 {
			a.finishSeasonScan(scan, names, hash)
			a.publishSeasonSnapshot(reference, playerRef, scan.cache)
		}
	})
}
