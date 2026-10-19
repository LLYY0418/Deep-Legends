package main

import (
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func r292Build(t *testing.T, id int, team string, hours int) proOPGGBuild {
	t.Helper()
	r := proOPGGFixture(t)[0]
	r.ID = strconv.Itoa(id)
	r.Game.ID = strconv.Itoa(id)
	r.Team = proOPGGTeam{ID: team, Acronym: "fixture"}
	r.Player = proOPGGPlayer{ID: strconv.Itoa(id), NickName: "player" + strconv.Itoa(id)}
	r.Game.Winner = &proOPGGTeam{ID: team}
	r.Game.BeginAt = proOPGGNow().Add(-time.Duration(hours) * time.Hour)
	return r
}

func r292Keys(rows []gameplayRecommendationRune) []string {
	keys := []string{}
	for _, row := range rows {
		keys = append(keys, row.Key)
	}
	return keys
}

func TestR292PriorityUsesTeamRegionInWorldsAndFillsFive(t *testing.T) {
	rows := []proOPGGBuild{
		r292Build(t, 1, "563", 1), r292Build(t, 2, "1751", 2),
		r292Build(t, 3, "385", 10), r292Build(t, 4, "632", 8),
		r292Build(t, 5, "563", 3), r292Build(t, 6, "1751", 4), r292Build(t, 7, "999999", 5),
	}
	// All rows are Worlds. The unknown ID intentionally borrows T1's acronym.
	rows[6].Team.Acronym = "T1"
	got, games, failed := proOPGGRecommendations(rows, 103, "mid", "阿狸", proOPGGNow(), proOPGGNow(), false)
	want := []string{"pro-opgg-4", "pro-opgg-3", "pro-opgg-1", "pro-opgg-2", "pro-opgg-5"}
	if !reflect.DeepEqual(r292Keys(got), want) || games != 7 || failed != 0 {
		t.Fatalf("team region/time/whole-list cap: got %v, games=%d failed=%d", r292Keys(got), games, failed)
	}
	for _, row := range got {
		if row.EventLabel[:6] != "Worlds" {
			t.Fatal("event label was changed to a team region")
		}
	}
}

func TestR292FivePriorityGamesExcludeOtherRegions(t *testing.T) {
	rows := []proOPGGBuild{r292Build(t, 1, "563", 1)}
	for i, team := range []string{"385", "632", "416", "698", "1755", "645"} {
		rows = append(rows, r292Build(t, i+2, team, i+10))
	}
	got, games, _ := proOPGGRecommendations(rows, 103, "mid", "阿狸", proOPGGNow(), proOPGGNow(), false)
	if want := []string{"pro-opgg-2", "pro-opgg-3", "pro-opgg-4", "pro-opgg-5", "pro-opgg-6"}; !reflect.DeepEqual(r292Keys(got), want) || games != 7 {
		t.Fatal("priority group must fill the whole five-game list", r292Keys(got), games)
	}
}

func TestR292OtherRegionsKeepLatestFiveSnapshotAndUnknowns(t *testing.T) {
	rows := []proOPGGBuild{}
	for i := 7; i >= 1; i-- {
		r := r292Build(t, i, "999999", i)
		r.League = "" // Missing event/region fields must not clear the list.
		rows = append(rows, r)
	}
	got, games, _ := proOPGGRecommendations(rows, 103, "mid", "阿狸", proOPGGNow(), proOPGGNow(), false)
	// Captures the old chronological order, with only the new global cap applied.
	if want := []string{"pro-opgg-1", "pro-opgg-2", "pro-opgg-3", "pro-opgg-4", "pro-opgg-5"}; !reflect.DeepEqual(r292Keys(got), want) || games != 7 {
		t.Fatal("unknown/other teams changed order or disappeared", r292Keys(got), games)
	}
}

func TestR292RecordCountsAllGamesBeforeDisplayCap(t *testing.T) {
	rows := []proOPGGBuild{}
	for i := 1; i <= 7; i++ {
		r := r292Build(t, i, "385", i)
		r.Player.ID = "10"
		if i > 3 {
			r.Game.Winner = &proOPGGTeam{ID: "563"}
		}
		if i == 7 {
			r.Game.Winner = nil
		}
		rows = append(rows, r)
	}
	got, games, _ := proOPGGRecommendations(rows, 103, "mid", "阿狸", proOPGGNow(), proOPGGNow(), false)
	if len(got) != 5 || games != 7 {
		t.Fatal("display cap changed totalGames", len(got), games)
	}
	for _, row := range got {
		if row.RecordGames != 6 || row.RecordWins != 3 || !row.RecordPartial {
			t.Fatal("hidden games lost from the confirmed record", row)
		}
	}
}

func TestR292LegacyCacheUsesTeamIDsWithoutExtraRequests(t *testing.T) {
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	p := newProRuneProvider(proTestProvider(), store, nil)
	now := proOPGGNow()
	// Exact pre-R292 cache shape: no team-region field, only the original IDs.
	p.writeDisk("opgg-builds-103", proOPGGCache{ChampionID: 103, ReadAt: now, Attempted: now, Rows: []proOPGGBuild{r292Build(t, 1, "563", 1), r292Build(t, 2, "385", 2)}})
	restarted := newProRuneProvider(proTestProvider(), store, nil)
	restarted.now = proOPGGNow
	var requests atomic.Int32
	restarted.provider.client = &http.Client{Transport: championRoundTripFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return proOPGGReply([]proOPGGBuild{}), nil
	})}
	got, err := restarted.opggRecommend(t.Context(), 103, "mid", false)
	if err != nil || requests.Load() != 0 || !reflect.DeepEqual(r292Keys(got.Pros), []string{"pro-opgg-2", "pro-opgg-1"}) {
		t.Fatal("old cache downgraded a Worlds LCK team or added requests", r292Keys(got.Pros), requests.Load(), err)
	}
}

func TestR292VerifiedTeamRegionTable(t *testing.T) {
	b, err := os.ReadFile("testdata/opgg-team-regions-2026-10-19.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Teams []struct{ ID, Region, Acronym, Name string }
	}
	if err := json.Unmarshal(b, &fixture); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fixture.Teams, proOPGGPriorityTeams) {
		t.Fatal("production region table differs from verified public responses")
	}
	// Call recommendations so the source evidence also verifies real behavior.
	seen := map[string]string{}
	counts := map[string]int{}
	for _, team := range fixture.Teams {
		if old := seen[team.ID]; old != "" {
			t.Fatalf("duplicate/cross-region ID %s: %s/%s", team.ID, old, team.Region)
		}
		seen[team.ID] = team.Region
		counts[team.Region]++
		got, _, _ := proOPGGRecommendations([]proOPGGBuild{r292Build(t, 1, "999999", 1), r292Build(t, 2, team.ID, 2)}, 103, "mid", "阿狸", proOPGGNow(), proOPGGNow(), false)
		if got[0].Key != "pro-opgg-2" {
			t.Fatalf("verified %s %s was downgraded", team.Region, team.Acronym)
		}
	}
	if counts["LPL"] != 12 || counts["LCK"] != 10 || len(seen) != 22 {
		t.Fatal("incomplete current-season directory", counts)
	}
}
