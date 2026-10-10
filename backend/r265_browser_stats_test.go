package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// Browser sample payloads use the production Riot derivation, rather than
// truncating a demo list while retaining its complete statistics.
func TestR265BrowserLoadedStatsFixture(t *testing.T) {
	const subject = "player_r265_fixture"
	matches := make([]gameplayMatch, 10)
	for i := range matches {
		position := "top"
		if i >= 6 {
			position = "middle"
		}
		win := i%3 != 1
		result := "loss"
		if win {
			result = "win"
		}
		matches[i] = gameplayMatch{
			GameID: int64(265000 + i), QueueID: 420, QueueLabel: "单双排", GameMode: "CLASSIC", MapID: 11,
			CreatedAt: time.Now().Add(-time.Duration(i) * time.Hour).UnixMilli(), Duration: 1800, Result: result, SubjectParticipantID: 1,
		}
		roles := []string{"top", "jungle", "middle", "bottom", "utility"}
		if position == "middle" {
			roles[0], roles[2] = roles[2], roles[0]
		}
		for team := 0; team < 2; team++ {
			for slot, role := range roles {
				id := int64(team*5 + slot + 1)
				player := gameplayParticipant{ParticipantID: id, TeamID: int64(100 + team*100), PlayerRef: "r265-peer-" + strconv.FormatInt(id, 10), DisplayName: "同场测试" + strconv.FormatInt(id, 10), Position: role, Win: win != (team == 1), ChampionID: int64(64 + slot), ChampionName: "测试英雄", ChampionLevel: 17, Kills: 3 + slot, Deaths: 3, Assists: 7, CS: 180 + slot, Gold: 12000 + slot*100, Damage: 20000 + slot*100, VisionScore: 20, Spell1ID: 4, Spell2ID: 12, PrimaryStyleID: 8000, SubStyleID: 8400}
				if team == 0 && slot == 0 {
					player.PlayerRef, player.GameName, player.TagLine, player.DisplayName = subject, "外服测试", "KR1", "外服测试#KR1"
					player.ChampionID, player.ChampionName, player.Kills, player.CS = 164, "卡蜜尔", 5+i, 180+i
				}
				player.KDA = round2(float64(player.Kills+player.Assists) / float64(player.Deaths))
				player.CSPerMinute = round1(float64(player.CS) / 30)
				matches[i].Participants = append(matches[i].Participants, player)
			}
		}
		for _, teamID := range []int64{100, 200} {
			team := abilityTeam(matches[i], teamID)
			team.Win = win != (teamID == 200)
			matches[i].Teams = append(matches[i].Teams, team)
		}
	}
	fixtures := make(map[string]gameplayOverview)
	for _, k := range []int{0, 1, 3, 6, 7, 9, 10} {
		response := gameplayOverview{Matches: matches[:k]}
		deriveRiotOverviewStats(&response, subject, map[int64]string{164: "卡蜜尔", 64: "李青", 103: "阿狸"}, "kr")
		if response.RecentRanked.Wins+response.RecentRanked.Losses != k {
			t.Fatalf("k=%d wins+losses=%d", k, response.RecentRanked.Wins+response.RecentRanked.Losses)
		}
		for _, player := range response.RecentPlayers {
			if player.Games > k {
				t.Fatalf("k=%d common games=%d", k, player.Games)
			}
		}
		for _, stat := range response.RankedQueues["420"].Positions {
			count := 0
			for _, match := range matches[:k] {
				if match.Participants[0].Position == stat.Position {
					count++
				}
			}
			if stat.Games != count || stat.Share != int(math.Round(float64(count)*100/float64(k))) {
				t.Fatalf("k=%d position=%+v count=%d", k, stat, count)
			}
		}
		fixtures[strconv.Itoa(k)] = response
	}
	if target := os.Getenv("R265_STATS_FIXTURE_OUT"); target != "" {
		data, err := json.Marshal(fixtures)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
