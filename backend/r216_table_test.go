package main

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

func TestR216ChampionTableThreeGamesAndMissing(t *testing.T) {
	cache := seasonStatsCache{}
	for i, enemy := range []int64{99, 99, 64} {
		ps := []riotParticipant{}
		for j := 0; j < 10; j++ {
			p := riotParticipant{ParticipantID: int64(j + 1), PUUID: "other", ChampionID: 22, TeamID: 100, Kills: 2, Deaths: 1, Assists: 3, GoldEarned: 10000, TotalDamageDealtToChampions: 10000, TotalMinionsKilled: 100, VisionScore: 10, Win: j < 5, TeamPosition: []string{"TOP", "JUNGLE", "MIDDLE", "BOTTOM", "UTILITY"}[j%5]}
			if j >= 5 {
				p.TeamID = 200
			}
			if j == 0 {
				p.PUUID = "subject"
				p.ChampionID = 13
				p.Kills = 4 + i
				p.Deaths = 2
				p.Assists = 6
				p.TotalDamageDealtToChampions = 20000
				p.GoldEarned = 12000
				p.TotalMinionsKilled = 120
				p.Win = i != 2
				two := 2
				p.DoubleKills = &two
				zero := 0
				p.VisionWardsBoughtInGame = &zero
			}
			if j == 5 {
				p.ChampionID = enemy
			}
			taken := 10000
			p.TotalDamageTaken = &taken
			ps = append(ps, p)
		}
		info := &riotMatchInfo{GameID: int64(216 + i), GameCreation: 1700000000000, GameDuration: 1200, QueueID: 420, Participants: ps}
		seasonAccumulateChampionTable(&cache, info, "subject", 0)
	}
	rows, total := championTableRows(cache, "ranked", nil)
	if len(rows) != 1 || total.Games != 3 || total.Wins != 2 || len(rows[0].Opponents) != 2 || rows[0].Opponents[0].Games != 2 {
		t.Fatal(rows, total)
	}
	r := rows[0]
	if *r.Kills != 5 || *r.Deaths != 2 || *r.Assists != 6 || *r.KDA != 5.5 || *r.DamagePerMinute != 1000 || math.Abs(*r.DamageShare-1.0/3) > 1e-12 || math.Abs(*r.TankShare-.2) > 1e-12 || *r.CS != 120 || *r.Gold != 12000 || *r.DoubleKills != 6 || *r.ControlWards != 0 || r.TripleKills != nil {
		t.Fatalf("row=%+v", r)
	}
	if r.Score == nil || r.Rank == nil {
		t.Fatal("Go score missing")
	}
	before, _ := json.Marshal(total)
	seasonTrimChampionTable(&cache, 1)
	_, after := championTableRows(cache, "ranked", nil)
	got, _ := json.Marshal(after)
	if string(before) != string(got) {
		t.Fatal("opponent budget changed totals")
	}
}
func TestR216ChampionTableOpponentEvidence(t *testing.T) {
	for _, kind := range []string{"missing", "ambiguous", "mayhem", "flex"} {
		cache := seasonStatsCache{}
		info := &riotMatchInfo{GameCreation: 1700000000000, GameDuration: 1200, QueueID: 420, Participants: []riotParticipant{{ParticipantID: 1, TeamID: 100, PUUID: "subject", ChampionID: 13, TeamPosition: "TOP", Win: true, Kills: 1}, {ParticipantID: 2, TeamID: 200, ChampionID: 99, TeamPosition: "TOP", Kills: 1}}}
		switch kind {
		case "missing":
			info.Participants[0].TeamPosition = ""
		case "ambiguous":
			info.Participants = append(info.Participants, riotParticipant{ParticipantID: 3, TeamID: 200, ChampionID: 64, TeamPosition: "TOP"})
		case "mayhem":
			info.QueueID = 2400
		case "flex":
			info.QueueID = 440
		}
		seasonAccumulateChampionTable(&cache, info, "subject", 0)
		if len(cache.ChampionTable) != 1 {
			t.Fatal(kind)
		}
		want := 0
		if kind == "flex" {
			want = 1
		}
		if len(cache.ChampionTable[0].Opponents) != want {
			t.Fatal(kind, cache)
		}
	}
	var p gameplayParticipant
	_ = json.Unmarshal([]byte(`{"participantId":1,"teamId":100,"win":true}`), &p)
	tots := tableTotalsForMatch(r216Game(p, r216Person(2, 200)), p)
	r := tableRow(13, tots, nil)
	if r.Score != nil || r.Kills != nil || r.Gold != nil || r.CS != nil || r.DamageShare != nil || r.ControlWards != nil || r.WardsPlaced != nil {
		t.Fatal("missing invented", r)
	}
}

func TestR216OPGGChampionTableObservedPage(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../docs/history/reports/r216/kr-champion-table-probe/champions.html")
	if err != nil {
		t.Fatal(err)
	}
	ref := gameplayReference{Region: "kr", GameName: "TOPKING", TagLine: "asd"}
	table, err := parseOPGGChampionTable(raw, ref, "RANKED", nil)
	if err != nil {
		t.Fatal(err)
	}
	if table.Overall.Games != 1383 || len(table.TableRows) == 0 || !table.TableSupported {
		t.Fatal(table)
	}
	if table.TableOverall.Score != nil || table.TableOverall.Rank != nil || table.TableOverall.TankShare != nil {
		t.Fatal("substituted OP score or absolute tank value")
	}
	if table.TableOverall.DoubleKills == nil || *table.TableOverall.DoubleKills != 622 {
		t.Fatal(table.TableOverall)
	}
	opponents := 0
	for _, r := range table.TableRows {
		opponents += len(r.Opponents)
	}
	if opponents == 0 {
		t.Fatal("missing matchup aggregates")
	}
	if _, err := parseOPGGChampionTable(raw, gameplayReference{Region: "kr", GameName: "Wrong", TagLine: "asd"}, "RANKED", nil); err == nil {
		t.Fatal("unbound player")
	}
	solo, err := os.ReadFile("../docs/history/reports/r216/kr-champion-table-probe/solo-queue-type.html")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseOPGGChampionTable(solo, ref, "SOLORANKED", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := parseOPGGChampionTable(solo, ref, "FLEXRANKED", nil); err == nil {
		t.Fatal("wrong queue accepted")
	}
}
