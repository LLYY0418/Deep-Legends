package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestOPGGProfileSeparatesProviderIDs(t *testing.T) {
	ref := gameplayReference{Region: "kr", GameName: "Fixture", TagLine: "KR1", PlayerRef: "riot-project-id-000001"}
	page := summaryFixtureHTML("opgg-project-id-000001", 33)
	parsed, err := parseOPGGPlayerPage(page, ref)
	if err != nil || parsed.puuid == ref.PlayerRef {
		t.Fatalf("provider boundary: %v", err)
	}
	for _, bad := range []gameplayReference{{GameName: "Other", TagLine: "KR1"}, {GameName: "Fixture", TagLine: "other"}} {
		if _, err := parseOPGGPlayerPage(page, bad); err == nil {
			t.Fatal("wrong profile accepted")
		}
	}
	a := &app{champions: newChampionProvider()}
	a.champions.championMeta = map[int]championMetadata{126: {ID: 126, Key: "jayce"}}
	a.champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == "GET" {
			return proHTTPBody(page), nil
		}
		var input []map[string]any
		if json.NewDecoder(r.Body).Decode(&input) != nil || input[0]["puuid"] != parsed.puuid {
			t.Fatal("Riot ID sent to OP.GG")
		}
		return proHTTPBody(summaryFixtureResponse()), nil
	})}
	result, err := a.fetchOPGGSeasonSummary(context.Background(), ref)
	if err != nil || result.Overall.Games != 573 {
		t.Fatalf("aggregate %v %v", result, err)
	}
	identity := gameplayReference{PlayerRef: parsed.puuid, Region: "kr", GameName: ref.GameName, TagLine: ref.TagLine, OPGGIdentity: true}
	resolved, ok := a.resolveGameplayReferenceDetails(a.registerGameplayReferenceDetails(identity))
	if !ok || !resolved.OPGGIdentity {
		t.Fatal("OP.GG reference provenance lost during registration")
	}
}

func TestOPGGCurrentPageFreshnessAndReferences(t *testing.T) {
	a, action, ref, now := currentGameFixture(t)
	value, err := currentActionResult(action)
	if err != nil {
		t.Fatal(err)
	}
	var game map[string]any
	_ = json.Unmarshal(value, &game)
	page := &opggPlayerPage{puuid: ref.PlayerRef, rows: map[string]any{}}
	page.rows["55"] = []any{"$", "fixture", nil, map[string]any{"type": game["game_type"]}}
	game["game_type"] = "$55:props:type"
	initial := map[string]any{"data": game, "fetchedAt": float64(now.UnixMilli())}
	page.rows["35"] = []any{[]any{"$", "fixture", nil, map[string]any{"region": "kr", "puuid": ref.PlayerRef, "initialResult": initial}}}
	data, fresh, err := page.currentGame(now)
	if err != nil || !fresh {
		t.Fatal(err)
	}
	result, err := a.parseOPGGCurrentGame(data, ref, now)
	if err != nil || result.Status != "active" {
		t.Fatalf("%v %v", result, err)
	}
	if _, fresh, _ := page.currentGame(now.Add(11 * time.Second)); fresh {
		t.Fatal("stale server page accepted")
	}
	if _, fresh, _ := page.currentGame(now.Add(-2 * time.Minute)); fresh {
		t.Fatal("future fetchedAt accepted")
	}
	page.rows["55"] = "$55"
	if _, _, err := page.currentGame(now); err == nil {
		t.Fatal("cyclic reference accepted")
	}
}

func TestOPGGSeasonRejectsConflictingMetadata(t *testing.T) {
	page := append(summaryFixtureHTML("profile-id-000001", 33), summaryFixtureHTML("profile-id-000001", 32)...)
	if _, err := parseOPGGSummaryMetadata(page, "profile-id-000001"); err == nil {
		t.Fatal("ambiguous season accepted")
	}
}

// Opt-in exact user-report capture. No player identifiers or scripts are added
// to the repository; this tests the original, unmodified Flight references.
func TestOPGGUserReportCapture(t *testing.T) {
	if os.Getenv("OPGG_REPORT_CAPTURE") != "1" {
		t.Skip("local user-report capture")
	}
	raw, err := os.ReadFile("/private/tmp/opgg-eclipse-0909.html")
	if err != nil {
		t.Fatal(err)
	}
	ref := gameplayReference{Region: "kr", GameName: "T1 Eclipse", TagLine: "2009", PlayerRef: "different-riot-project-identity"}
	page, err := parseOPGGPlayerPage(raw, ref)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := parseOPGGSummaryMetadata(raw, page.puuid)
	if err != nil {
		t.Fatal(err)
	}
	var catalog map[string]struct {
		ID        int
		Key, Name string
	}
	b, _ := os.ReadFile("/private/tmp/meraki-champions-20260909.json")
	if json.Unmarshal(b, &catalog) != nil {
		t.Fatal("catalog")
	}
	a := &app{champions: newChampionProvider()}
	a.champions.championMeta = map[int]championMetadata{}
	byKey := map[string]championMetadata{}
	for key, v := range catalog {
		m := championMetadata{ID: v.ID, Key: key, NameZH: v.Name}
		a.champions.championMeta[v.ID] = m
		byKey[strings.ToLower(key)] = m
	}
	stamp := time.UnixMilli(1788947884100)
	data, fresh, err := page.currentGame(stamp.Add(time.Second))
	if err != nil || !fresh {
		t.Fatalf("page current: %v %v", fresh, err)
	}
	ref.PlayerRef = page.puuid
	game, err := a.parseOPGGCurrentGame(data, ref, stamp)
	if err != nil || game.Status != "active" {
		t.Fatalf("roster: %v %v", game, err)
	}
	seasonRaw, _ := os.ReadFile("/private/tmp/opgg-eclipse-season-action.txt")
	season, err := parseOPGGSeasonSummary(seasonRaw, meta, byKey)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("user capture: active teams=%d; %s games=%d wins=%d losses=%d champions=%d", len(game.Teams), season.Season, season.Overall.Games, season.Overall.Wins, season.Overall.Losses, len(season.Champions))
	if os.Getenv("OPGG_REPORT_LIVE") == "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		ref.PlayerRef = "different-riot-project-identity"
		live, err := a.fetchOPGGSeasonSummary(ctx, ref)
		if err != nil {
			t.Fatal("live summary", err)
		}
		current, err := a.fetchOPGGCurrentGame(ctx, ref)
		if err != nil {
			t.Fatal("live game", err)
		}
		t.Logf("LIVE production adapters: %s %d games, %d/%d, champions=%d; current=%s teams=%d", live.Season, live.Overall.Games, live.Overall.Wins, live.Overall.Losses, len(live.Champions), current.Status, len(current.Teams))
	}
}
