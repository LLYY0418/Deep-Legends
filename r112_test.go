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

func r112ProfilePage(name, tag, id string) string {
	data, _ := json.Marshal(map[string]any{"region": "kr", "data": map[string]any{"gameName": name, "tagline": tag, "puuid": id}})
	fragment, _ := json.Marshal("1:" + string(data) + "\n")
	return "<script>self.__next_f.push([1," + string(fragment) + "])</script>"
}

func TestR112AverageTierMixedPublicResponse(t *testing.T) {
	data, err := os.ReadFile("testdata/r112-opgg-average-tiers.txt")
	if err != nil {
		t.Fatal(err)
	}
	games, cursor, err := parseOPGGGamesPage(data)
	if err != nil || len(games) != 20 || cursor == "" {
		t.Fatalf("page rows=%d cursor=%q err=%v", len(games), cursor, err)
	}
	found := 0
	for _, game := range games {
		tier := matchOPGGAverageTier(game.createdAt, game.duration, games)
		if tier != nil {
			found++
			if (tier.Tier != "CHALLENGER" && tier.Tier != "GRANDMASTER") || tier.Division != "" {
				t.Fatalf("tier %#v", tier)
			}
		}
	}
	if found != 7 {
		t.Fatalf("mixed page lost valid tiers: %d", found)
	}
}

func TestR112AverageTierActionReferenceAndMissingShapes(t *testing.T) {
	stream := `9:{"data":[{"average_tier":{"tier":"WRONG"}}]}
0:{"a":"$@2"}
2:{"data":[{"created_at":"2026-09-18T01:00:00Z","game_length":1800,"average_tier":"$3"},{"created_at":"2026-09-18T00:00:00Z","game_length":1900,"average_tier":null},{"created_at":"2026-09-17T23:00:00Z","game_length":1600,"average_tier":"$undefined"}]}
3:{"tier":"diamond","division":2,"lp":10}`
	games, cursor, err := parseOPGGGamesPage([]byte(stream))
	if err != nil || len(games) != 3 || cursor != "" || games[0].tier.Tier != "DIAMOND" || games[1].tier.Tier != "" || games[2].tier.Tier != "" {
		t.Fatalf("games=%#v cursor=%s err=%v", games, cursor, err)
	}
	for _, invalid := range []string{`0:{"data":[]}`, `0:{"a":"$@1"}` + "\n" + `1:{"error":"unavailable"}`, strings.Replace(stream, `"$3"`, `"$missing"`, 1)} {
		if _, _, err := parseOPGGGamesPage([]byte(invalid)); err == nil {
			t.Fatal("invalid action reported as confirmed no tier")
		}
	}
	empty, _, err := parseOPGGGamesPage([]byte("0:{\"a\":\"$@1\"}\n1:{\"data\":[]}"))
	if err != nil || len(empty) != 0 {
		t.Fatalf("valid empty page: %v", err)
	}
}

func TestR112AverageTierIdentityMismatchNeverPosts(t *testing.T) {
	p := newChampionProvider()
	calls := 0
	p.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != http.MethodGet {
			t.Fatal("unverified identity sent to action")
		}
		return testHTTPResponse(r, 200, r112ProfilePage("Wrong", "KR1", strings.Repeat("o", 48))), nil
	})}
	a := &app{champions: p, opgg: newOPGGInsights()}
	for range 2 {
		_, err := a.opggGameTiers(context.Background(), "Right", "KR1", strings.Repeat("r", 48), time.Now().UnixMilli())
		if err == nil {
			t.Fatal("identity mismatch must remain retryable")
		}
	}
	if calls != 1 {
		t.Fatalf("failure wasn't deduplicated: %d", calls)
	}
}

// Optional local capture verifies the complete public Flight stream, including
// unrelated records; the checked-in fixture contains no account identifiers.
func TestR112FullPublicCapture(t *testing.T) {
	path := os.Getenv("R112_OPGG_CAPTURE")
	if path == "" {
		t.Skip("optional public response capture")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	games, _, err := parseOPGGGamesPage(data)
	count := 0
	for _, game := range games {
		if game.tier.Tier != "" {
			count++
		}
	}
	if err != nil || len(games) != 20 || count != 7 {
		t.Fatalf("raw capture: %d rows %d tiers %v", len(games), count, err)
	}
}

func TestR112EmptyAverageTierPageIsCached(t *testing.T) {
	p := newChampionProvider()
	calls := 0
	p.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method == http.MethodGet {
			return testHTTPResponse(r, 200, r112ProfilePage("Empty", "KR1", strings.Repeat("o", 48))), nil
		}
		return testHTTPResponse(r, 200, "0:{\"a\":\"$@1\"}\n1:{\"data\":[]}"), nil
	})}
	a := &app{champions: p, opgg: newOPGGInsights()}
	for range 2 {
		games, err := a.opggGameTiers(context.Background(), "Empty", "KR1", strings.Repeat("r", 48), time.Now().UnixMilli())
		if err != nil || len(games) != 0 {
			t.Fatalf("empty %v %v", games, err)
		}
	}
	if calls != 2 {
		t.Fatalf("empty page fetched twice: %d", calls)
	}
}

func TestR112AverageTierFailureExpiresAndRecovers(t *testing.T) {
	p := newChampionProvider()
	calls := 0
	p.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return testHTTPResponse(r, 503, "unavailable"), nil
		}
		if r.Method == http.MethodGet {
			return testHTTPResponse(r, 200, r112ProfilePage("Retry", "KR1", strings.Repeat("o", 48))), nil
		}
		return testHTTPResponse(r, 200, "0:{\"a\":\"$@1\"}\n1:{\"data\":[{\"created_at\":\"2026-09-18T01:00:00Z\",\"game_length\":1800,\"average_tier\":{\"tier\":\"challenger\",\"division\":1,\"lp\":2000}}]}"), nil
	})}
	a := &app{champions: p, opgg: newOPGGInsights()}
	load := func() ([]opggGameTier, error) {
		return a.opggGameTiers(context.Background(), "Retry", "KR1", strings.Repeat("r", 48), time.Now().UnixMilli())
	}
	if _, err := load(); err == nil {
		t.Fatal("expected transient error")
	}
	for k, entry := range a.opgg.tiers {
		entry.attemptedAt = time.Now().Add(-opggTierFailureTTL - time.Second)
		a.opgg.tiers[k] = entry
	}
	games, err := load()
	if err != nil || len(games) != 1 || games[0].tier.Tier != "CHALLENGER" {
		t.Fatalf("recovery failed: %v %v", games, err)
	}
	if _, err := load(); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("recovered cache repeated request: %d", calls)
	}
}
