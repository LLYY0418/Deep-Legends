package main

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"
)

// R216: accumulated while existing SGP season pages are scanned. Metrics retain
// independent observed sample counts, so missing data never becomes a zero.
type seasonTableMetric struct {
	Sum   float64 `json:"sum"`
	Count int     `json:"count"`
}

func (m *seasonTableMetric) add(value float64) { m.Sum += value; m.Count++ }
func (m seasonTableMetric) avg() *float64 {
	if m.Count == 0 {
		return nil
	}
	v := m.Sum / float64(m.Count)
	return &v
}

func (m seasonTableMetric) total() *float64 {
	if m.Count == 0 {
		return nil
	}
	v := m.Sum
	return &v
}

type seasonTableTotals struct {
	Games           int               `json:"games"`
	Wins            int               `json:"wins"`
	Kills           int               `json:"kills"`
	Deaths          int               `json:"deaths"`
	Assists         int               `json:"assists"`
	CombatGames     int               `json:"combatGames"`
	Score           seasonTableMetric `json:"score"`
	Rank            seasonTableMetric `json:"rank"`
	KP              seasonTableMetric `json:"kp"`
	DamagePerMinute seasonTableMetric `json:"damagePerMinute"`
	DamageShare     seasonTableMetric `json:"damageShare"`
	TankShare       seasonTableMetric `json:"tankShare"`
	ControlWards    seasonTableMetric `json:"controlWards"`
	WardsPlaced     seasonTableMetric `json:"wardsPlaced"`
	WardsKilled     seasonTableMetric `json:"wardsKilled"`
	CS              seasonTableMetric `json:"cs"`
	CSPerMinute     seasonTableMetric `json:"csPerMinute"`
	Gold            seasonTableMetric `json:"gold"`
	GoldPerMinute   seasonTableMetric `json:"goldPerMinute"`
	DoubleKills     seasonTableMetric `json:"doubleKills"`
	TripleKills     seasonTableMetric `json:"tripleKills"`
	QuadraKills     seasonTableMetric `json:"quadraKills"`
	PentaKills      seasonTableMetric `json:"pentaKills"`
}
type seasonTableOpponent struct {
	ChampionID int64             `json:"championId"`
	Totals     seasonTableTotals `json:"totals"`
}
type seasonTableBucket struct {
	QueueID    int64                 `json:"queueId"`
	ChampionID int64                 `json:"championId"`
	Totals     seasonTableTotals     `json:"totals"`
	Opponents  []seasonTableOpponent `json:"opponents,omitempty"`
}

func tableAddObserved(m *seasonTableMetric, p *int) {
	if p != nil && *p >= 0 {
		m.add(float64(*p))
	}
}
func tableTotalsForMatch(m gameplayMatch, p gameplayParticipant) seasonTableTotals {
	t := seasonTableTotals{Games: 1}
	if p.Win {
		t.Wins = 1
	}
	if !p.scoreMissing["combat"] {
		t.Kills = p.Kills
		t.Deaths = p.Deaths
		t.Assists = p.Assists
		t.CombatGames = 1
	}
	if p.Score != nil {
		t.Score.add(p.Score.RawScore)
		t.Rank.add(float64(p.Score.Rank))
	}
	kills, damage, tank := 0., 0., 0.
	combat, damageKnown, tankKnown := true, true, true
	for _, q := range m.Participants {
		if q.TeamID != p.TeamID {
			continue
		}
		kills += float64(q.Kills)
		damage += float64(q.Damage)
		combat = combat && !q.scoreMissing["combat"]
		damageKnown = damageKnown && !q.scoreMissing["damage"]
		if q.DamageTaken == nil {
			tankKnown = false
		} else {
			tank += float64(*q.DamageTaken)
		}
	}
	if combat && !p.scoreMissing["combat"] && kills > 0 {
		t.KP.add(float64(p.Kills+p.Assists) / kills)
	}
	if damageKnown && damage > 0 {
		t.DamageShare.add(float64(p.Damage) / damage)
	}
	if tankKnown && p.DamageTaken != nil && tank > 0 {
		t.TankShare.add(float64(*p.DamageTaken) / tank)
	}
	minutes := float64(m.Duration) / 60
	if !p.scoreMissing["damage"] && minutes > 0 {
		t.DamagePerMinute.add(float64(p.Damage) / minutes)
	}
	if !p.scoreMissing["cs"] {
		t.CS.add(float64(p.CS))
		if minutes > 0 {
			t.CSPerMinute.add(float64(p.CS) / minutes)
		}
	}
	if !p.scoreMissing["gold"] {
		t.Gold.add(float64(p.Gold))
		if minutes > 0 {
			t.GoldPerMinute.add(float64(p.Gold) / minutes)
		}
	}
	tableAddObserved(&t.ControlWards, p.ControlWardsBought)
	if !p.scoreMissing["wardsPlaced"] {
		t.WardsPlaced.add(float64(p.WardsPlaced))
	}
	if !p.scoreMissing["wardsKilled"] {
		t.WardsKilled.add(float64(p.WardsKilled))
	}
	tableAddObserved(&t.DoubleKills, p.DoubleKills)
	tableAddObserved(&t.TripleKills, p.TripleKills)
	tableAddObserved(&t.QuadraKills, p.QuadraKills)
	tableAddObserved(&t.PentaKills, p.PentaKills)
	return t
}
func tableMerge(a *seasonTableTotals, b seasonTableTotals) {
	a.Games += b.Games
	a.Wins += b.Wins
	a.Kills += b.Kills
	a.Deaths += b.Deaths
	a.Assists += b.Assists
	a.CombatGames += b.CombatGames
	left := []*seasonTableMetric{&a.Score, &a.Rank, &a.KP, &a.DamagePerMinute, &a.DamageShare, &a.TankShare, &a.ControlWards, &a.WardsPlaced, &a.WardsKilled, &a.CS, &a.CSPerMinute, &a.Gold, &a.GoldPerMinute, &a.DoubleKills, &a.TripleKills, &a.QuadraKills, &a.PentaKills}
	right := []seasonTableMetric{b.Score, b.Rank, b.KP, b.DamagePerMinute, b.DamageShare, b.TankShare, b.ControlWards, b.WardsPlaced, b.WardsKilled, b.CS, b.CSPerMinute, b.Gold, b.GoldPerMinute, b.DoubleKills, b.TripleKills, b.QuadraKills, b.PentaKills}
	for i, m := range left {
		m.Sum += right[i].Sum
		m.Count += right[i].Count
	}
}
func seasonAccumulateChampionTable(cache *seasonStatsCache, info *riotMatchInfo, playerRef string, seasonStart int64) {
	if info == nil || !seasonStatsQueueAllowed(info.QueueID) || normalizeEpochMillis(info.GameCreation) < seasonStart || riotMatchInfoIsRemake(info) {
		return
	}
	m := convertRiotMatchInfo(info, playerRef, nil, nil, "", "")
	var subject *gameplayParticipant
	for i := range m.Participants {
		if info.Participants[i].PUUID == playerRef {
			subject = &m.Participants[i]
			break
		}
	}
	if subject == nil || subject.ChampionID <= 0 {
		return
	}
	t := tableTotalsForMatch(m, *subject)
	index := -1
	for i, b := range cache.ChampionTable {
		if b.QueueID == m.QueueID && b.ChampionID == subject.ChampionID {
			index = i
			break
		}
	}
	if index < 0 {
		cache.ChampionTable = append(cache.ChampionTable, seasonTableBucket{QueueID: m.QueueID, ChampionID: subject.ChampionID})
		index = len(cache.ChampionTable) - 1
	}
	bucket := &cache.ChampionTable[index]
	tableMerge(&bucket.Totals, t)
	if m.QueueID != 420 && m.QueueID != 440 {
		return
	}
	position := scorePosition(*subject)
	if position == "" {
		return
	}
	enemy := int64(0)
	count := 0
	for _, p := range m.Participants {
		if p.TeamID != subject.TeamID && scorePosition(p) == position {
			enemy = p.ChampionID
			count++
		}
	}
	if count != 1 || enemy <= 0 {
		return
	}
	for i := range bucket.Opponents {
		if bucket.Opponents[i].ChampionID == enemy {
			tableMerge(&bucket.Opponents[i].Totals, t)
			return
		}
	}
	bucket.Opponents = append(bucket.Opponents, seasonTableOpponent{enemy, t})
}
func seasonTrimChampionTable(cache *seasonStatsCache, limit int) {
	// Copy before trimming: persisted-budget handling must not mutate scan state.
	buckets := append([]seasonTableBucket(nil), cache.ChampionTable...)
	for i := range buckets {
		opps := append([]seasonTableOpponent(nil), buckets[i].Opponents...)
		sort.Slice(opps, func(i, j int) bool {
			if opps[i].Totals.Games == opps[j].Totals.Games {
				return opps[i].ChampionID < opps[j].ChampionID
			}
			return opps[i].Totals.Games > opps[j].Totals.Games
		})
		if len(opps) > limit {
			opps = opps[:limit]
		}
		buckets[i].Opponents = opps
	}
	cache.ChampionTable = buckets
}

type championTableRow struct {
	ChampionID      int64              `json:"championId"`
	ChampionName    string             `json:"championName"`
	Games           int                `json:"games"`
	Wins            int                `json:"wins"`
	Losses          int                `json:"losses"`
	WinRate         float64            `json:"winRate"`
	Kills           *float64           `json:"kills"`
	Deaths          *float64           `json:"deaths"`
	Assists         *float64           `json:"assists"`
	KDA             *float64           `json:"kda"`
	KP              *float64           `json:"kp"`
	Score           *float64           `json:"score"`
	Rank            *float64           `json:"rank"`
	DamagePerMinute *float64           `json:"damagePerMinute"`
	DamageShare     *float64           `json:"damageShare"`
	TankShare       *float64           `json:"tankShare"`
	ControlWards    *float64           `json:"controlWards"`
	WardsPlaced     *float64           `json:"wardsPlaced"`
	WardsKilled     *float64           `json:"wardsKilled"`
	CS              *float64           `json:"cs"`
	CSPerMinute     *float64           `json:"csPerMinute"`
	Gold            *float64           `json:"gold"`
	GoldPerMinute   *float64           `json:"goldPerMinute"`
	DoubleKills     *float64           `json:"doubleKills"`
	TripleKills     *float64           `json:"tripleKills"`
	QuadraKills     *float64           `json:"quadraKills"`
	PentaKills      *float64           `json:"pentaKills"`
	Opponents       []championTableRow `json:"opponents,omitempty"`
}

func tableRow(id int64, t seasonTableTotals, names map[int64]string) championTableRow {
	r := championTableRow{ChampionID: id, ChampionName: championName(names, id), Games: t.Games, Wins: t.Wins, Losses: t.Games - t.Wins, Score: t.Score.avg(), Rank: t.Rank.avg(), KP: t.KP.avg(), DamagePerMinute: t.DamagePerMinute.avg(), DamageShare: t.DamageShare.avg(), TankShare: t.TankShare.avg(), ControlWards: t.ControlWards.avg(), WardsPlaced: t.WardsPlaced.avg(), WardsKilled: t.WardsKilled.avg(), CS: t.CS.avg(), CSPerMinute: t.CSPerMinute.avg(), Gold: t.Gold.avg(), GoldPerMinute: t.GoldPerMinute.avg(), DoubleKills: t.DoubleKills.total(), TripleKills: t.TripleKills.total(), QuadraKills: t.QuadraKills.total(), PentaKills: t.PentaKills.total()}
	if t.Games > 0 {
		r.WinRate = 100 * float64(t.Wins) / float64(t.Games)
	}
	if t.CombatGames > 0 {
		k, d, a := float64(t.Kills)/float64(t.CombatGames), float64(t.Deaths)/float64(t.CombatGames), float64(t.Assists)/float64(t.CombatGames)
		kda := ratioFloat(float64(t.Kills+t.Assists), float64(t.Deaths))
		r.Kills = &k
		r.Deaths = &d
		r.Assists = &a
		r.KDA = &kda
	}
	if id == 0 {
		r.ChampionName = "所有英雄"
	}
	return r
}
func championTableRows(cache seasonStatsCache, queue string, names map[int64]string) ([]championTableRow, championTableRow) {
	buckets := map[int64]*seasonTableBucket{}
	var total seasonTableTotals
	for _, b := range cache.ChampionTable {
		include := queue == "ranked" && (b.QueueID == 420 || b.QueueID == 440) || queue == "420" && b.QueueID == 420 || queue == "440" && b.QueueID == 440 || queue == "mayhem" && seasonMayhemQueue(b.QueueID)
		if !include {
			continue
		}
		bucket := buckets[b.ChampionID]
		if bucket == nil {
			bucket = &seasonTableBucket{ChampionID: b.ChampionID}
			buckets[b.ChampionID] = bucket
		}
		tableMerge(&bucket.Totals, b.Totals)
		tableMerge(&total, b.Totals)
		for _, opp := range b.Opponents {
			found := false
			for i := range bucket.Opponents {
				if bucket.Opponents[i].ChampionID == opp.ChampionID {
					tableMerge(&bucket.Opponents[i].Totals, opp.Totals)
					found = true
					break
				}
			}
			if !found {
				bucket.Opponents = append(bucket.Opponents, opp)
			}
		}
	}
	rows := []championTableRow{}
	for id, b := range buckets {
		row := tableRow(id, b.Totals, names)
		if queue != "mayhem" {
			for _, opp := range b.Opponents {
				row.Opponents = append(row.Opponents, tableRow(opp.ChampionID, opp.Totals, names))
			}
			sort.Slice(row.Opponents, func(i, j int) bool { return row.Opponents[i].Games > row.Opponents[j].Games })
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Games == rows[j].Games {
			return rows[i].ChampionID < rows[j].ChampionID
		}
		return rows[i].Games > rows[j].Games
	})
	return rows, tableRow(0, total, names)
}
func (a *app) handleGameplayChampionTable(w http.ResponseWriter, r *http.Request) {
	queue := r.URL.Query().Get("queue")
	if queue == "" {
		queue = "ranked"
	}
	if queue != "ranked" && queue != "420" && queue != "440" && queue != "mayhem" {
		http.Error(w, "队列无效", http.StatusBadRequest)
		return
	}
	ref := strings.TrimSpace(r.URL.Query().Get("playerRef"))
	reference, ok := a.resolveGameplayReferenceDetails(ref)
	if !ok {
		http.Error(w, "玩家引用已失效", http.StatusNotFound)
		return
	}
	if isRiotRegion(reference.Region) {
		if queue == "mayhem" {
			respondJSON(w, map[string]any{"available": false, "detail": "当前韩服数据源不支持海克斯大乱斗英雄统计"})
			return
		}
		gameType := map[string]string{"ranked": "RANKED", "420": "SOLORANKED", "440": "FLEXRANKED"}[queue]
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		table, err := a.opggChampionTable(ctx, reference, gameType)
		if err != nil {
			respondJSON(w, map[string]any{"available": false, "detail": "韩服英雄数据读取失败，请稍后重试"})
			return
		}
		respondJSON(w, map[string]any{"available": true, "queue": queue, "season": table.Season, "complete": true, "rows": table.TableRows, "overall": table.TableOverall})
		return
	}
	if a.storage == nil {
		http.Error(w, "赛季数据不可用", http.StatusConflict)
		return
	}
	season, _ := currentRankedSeason(time.Now())
	hash := a.storage.accountHash(Summoner{PUUID: reference.PlayerRef})
	cache, err := a.storage.loadSeasonStats(seasonStatsSource, hash, season)
	if err != nil {
		respondJSON(w, map[string]any{"available": false, "detail": "本赛季数据仍在读取，请稍后重试"})
		return
	}
	rows, total := championTableRows(cache, queue, a.displayChampionNames(r.Context(), "champion-table"))
	respondJSON(w, map[string]any{"available": true, "queue": queue, "season": season, "complete": cache.Complete, "rows": rows, "overall": total})
}
