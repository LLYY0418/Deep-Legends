package main

import (
	"encoding/json"
	"sort"
)

// Cache encoding deliberately excludes response-only averages and ratios.
type seasonChampionTotals struct {
	ChampionID   int64  `json:"championId"`
	ChampionName string `json:"championName,omitempty"`
	Games        int    `json:"games"`
	Wins         int    `json:"wins"`
	TotalKills   int    `json:"totalKills"`
	TotalDeaths  int    `json:"totalDeaths"`
	TotalAssists int    `json:"totalAssists"`
	TotalCS      int    `json:"totalCS,omitempty"`
	Duration     int64  `json:"duration,omitempty"`
}

func (cache seasonStatsCache) MarshalJSON() ([]byte, error) {
	type plain seasonStatsCache
	rows := make([]seasonChampionTotals, 0, len(cache.Stats))
	for _, row := range cache.Stats {
		rows = append(rows, seasonChampionTotals{row.ChampionID, row.ChampionName, row.Games, row.Wins, row.TotalKills, row.TotalDeaths, row.TotalAssists, row.TotalCS, row.Duration})
	}
	return json.Marshal(struct {
		plain
		Stats []seasonChampionTotals `json:"stats"`
	}{plain(cache), rows})
}

func seasonStatsRawRows(stats map[int64]*gameplaySeasonChampionStat, names map[int64]string) []gameplaySeasonChampionStat {
	rows := make([]gameplaySeasonChampionStat, 0, len(stats))
	for id, row := range stats {
		copy := *row
		copy.ChampionName = championName(names, id)
		copy.Kills, copy.Deaths, copy.Assists, copy.KDA, copy.CS, copy.CSPerMinute, copy.WinRate = 0, 0, 0, 0, 0, 0, 0
		rows = append(rows, copy)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ChampionID < rows[j].ChampionID })
	return rows
}

func seasonStatsResponse(rows []gameplaySeasonChampionStat) []gameplaySeasonChampionStat {
	stats := make(map[int64]*gameplaySeasonChampionStat, len(rows))
	for _, row := range rows {
		copy := row
		stats[row.ChampionID] = &copy
	}
	return seasonStatsFinalize(stats, nil)
}

func seasonStatsCount(rows []gameplaySeasonChampionStat) int {
	count := 0
	for _, row := range rows {
		count += row.Games
	}
	return count
}
