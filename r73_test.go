package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestR73TeamGate(t *testing.T) {
	member := proFixtureMember(385, "knight", "Zhuo Ding", proFixtureAccount("excluded-one", "secret-one", "MASTER", 1, 20), proFixtureAccount("excluded-two", "secret-two", "MASTER", 1, 10), proFixtureAccount("excluded-three", "secret-three", "MASTER", 1, 5))
	result := new(app).buildProPlayers([]opggProTeam{{ID: 385, Members: []opggProMember{member}}}, proRoster[:1])
	var found bool
	for _, p := range result.Teams[0].Players {
		if p.Name == "knight" {
			found = true
			if len(p.Accounts) != 0 || p.Status != "partial" {
				t.Fatalf("cross-team identity accepted: %+v", p)
			}
		}
	}
	if !found {
		t.Fatal("player missing")
	}
	body, _ := json.Marshal(result)
	for _, forbidden := range []string{"puuid", "secret-one", "excluded-one", "private-session"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatal("private or rejected identity exported")
		}
	}
}

func TestR73StableIdentityAnchor(t *testing.T) {
	old := proFixtureAccount("old", "stable-secret", "MASTER", 1, 1)
	recent := old
	recent.GameName = "new"
	recent.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	// Union connectivity is unchanged by anchor order, so test the actual key
	// contract too: a rename must preserve the primary internal identity key.
	if proAccountKeys(old)[0] != proAccountKeys(recent)[0] {
		t.Fatal("mutable Riot ID used as identity anchor")
	}
	src := []opggProTeam{{ID: 632, Members: []opggProMember{proFixtureMember(632, "Bin", "Chen Ze-Bin", old, recent)}}}
	result := new(app).buildProPlayers(src, proRoster[:1])
	accounts := result.Teams[0].Players[0].Accounts
	if len(accounts) != 1 || accounts[0].GameName != "new" {
		t.Fatalf("rename merge failed: %+v", accounts)
	}
	body, _ := json.Marshal(result)
	if strings.Contains(string(body), "stable-secret") || strings.Contains(string(body), "puuid") {
		t.Fatal("stable ID leaked")
	}
}

func TestR73DormantPrimary(t *testing.T) {
	old := proFixtureAccount("old-high", "old-id", "CHALLENGER", 1, 1000)
	old.UpdatedAt = time.Now().Add(-200 * 24 * time.Hour).UTC().Format(time.RFC3339)
	fresh := proFixtureAccount("fresh", "fresh-id", "MASTER", 1, 10)
	fresh.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	src := []opggProTeam{{ID: 632, Members: []opggProMember{proFixtureMember(632, "Bin", "Chen Ze-Bin", old, fresh)}}}
	result := new(app).buildProPlayers(src, proRoster[:1])
	accounts := result.Teams[0].Players[0].Accounts
	if len(accounts) != 2 || accounts[0].GameName != "fresh" || !accounts[0].Primary || accounts[0].Dormant || !accounts[1].Dormant || accounts[1].Primary || result.DormantCount != 1 {
		t.Fatalf("freshness ordering/primary wrong: %+v", accounts)
	}
	old.Source = "TrackingThePros"
	old.PUUID = ""
	old.UpdatedAt = "invalid"
	a, ok := normalizeProAccount(old)
	if !ok || a.Dormant || a.Confidence != "low" {
		t.Fatal("undated supplement must remain visible and low confidence")
	}
}

func TestR73AmbiguousLadderNoRequest(t *testing.T) {
	provider := &championProvider{client: &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Error("ambiguous name requested")
		return nil, context.Canceled
	})}}
	for _, name := range []string{"has space", "has-hyphen"} {
		raw := proFixtureAccount(name, "secret", "MASTER", 1, 10)
		src := []opggProTeam{{ID: 632, Members: []opggProMember{proFixtureMember(632, "Bin", "Chen Ze-Bin", raw)}}}
		enrichProLadderRanks(context.Background(), provider, src)
		if src[0].Members[0].Summoners[0].LadderRankKnown {
			t.Fatal("ambiguous ladder known")
		}
	}
}

func TestR73LazyTierComparison(t *testing.T) {
	for _, tc := range []struct {
		expected, actual string
		want             bool
	}{{"CHALLENGER", "DIAMOND", true}, {"CHALLENGER", "MASTER", false}, {"IRON", "GOLD", true}, {"CHALLENGER", "UNRANKED", false}, {"", "IRON", false}} {
		if got := proOverviewMismatch(tc.expected, []gameplayRank{{QueueType: "RANKED_SOLO_5x5", Tier: tc.actual}}); got != tc.want {
			t.Fatalf("%+v got %v", tc, got)
		}
	}
	if proOverviewMismatch("CHALLENGER", []gameplayRank{{QueueType: "RANKED_FLEX_SR", Tier: "IRON"}}) {
		t.Fatal("flex used as solo")
	}
}

func TestR73OverviewComparisonDoesNotPolluteSnapshot(t *testing.T) {
	a := &app{overviewQueries: newOverviewQueryCache()}
	ref := gameplayReference{GameName: "Fixture", TagLine: "KR1", Region: "kr"}
	key := riotOverviewQuerySnapshotKey(ref, 0, defaultMatchCount)
	snapshot := gameplayOverview{Ranks: []gameplayRank{{QueueType: "RANKED_SOLO_5x5", Tier: "DIAMOND"}}}
	a.overviewQueries.putLocked(key, overviewQueryCacheEntry{at: time.Now(), response: snapshot})
	for _, tc := range []struct {
		tier     string
		mismatch bool
	}{{"CHALLENGER", true}, {"DIAMOND", false}, {"", false}} {
		body := `{"gameName":"Fixture","tagLine":"KR1","region":"kr","expectedTier":"` + tc.tier + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/gameplay/overview", strings.NewReader(body))
		w := httptest.NewRecorder()
		a.handleGameplayOverview(w, req)
		if w.Code != 200 {
			t.Fatalf("status=%d %s", w.Code, w.Body.String())
		}
		var result struct {
			ProMismatch bool `json:"proMismatch"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.ProMismatch != tc.mismatch {
			t.Fatalf("request-local comparison %s: %+v", tc.tier, result)
		}
		if strings.Contains(w.Body.String(), "puuid") || strings.Contains(w.Body.String(), "expectedTier") {
			t.Fatal("private/request field exposed")
		}
	}
	encoded, _ := json.Marshal(a.overviewQueries.entries[key].Value.(overviewQueryCacheItem).entry.response)
	if strings.Contains(string(encoded), "proMismatch") || strings.Contains(string(encoded), "expectedTier") {
		t.Fatal("comparison entered shared snapshot")
	}
}

func TestR73Riot404AndPublicOverview(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-fixture")
	for _, notFound := range []bool{false, true} {
		champions := newChampionProvider()
		champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			body := "[]"
			code := 200
			switch {
			case strings.Contains(r.URL.Path, "/riot/account/"):
				if notFound {
					code = 404
					body = `{}`
				} else {
					body = `{"puuid":"stable-sensitive","gameName":"Fixture","tagLine":"KR1"}`
				}
			case strings.Contains(r.URL.Path, "/lol/summoner/"):
				body = `{"puuid":"stable-sensitive","summonerLevel":100}`
			case strings.Contains(r.URL.Path, "/lol/league/"):
				body = `[{"queueType":"RANKED_SOLO_5x5","tier":"DIAMOND","rank":"I","leaguePoints":10}]`
			}
			return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		})}
		a := &app{riot: newRiotProvider(champions), token: "private-session-token"}
		req := httptest.NewRequest(http.MethodPost, "/api/gameplay/overview", strings.NewReader(`{"gameName":"Fixture","tagLine":"KR1","region":"kr","expectedTier":"CHALLENGER"}`))
		w := httptest.NewRecorder()
		a.handleGameplayOverview(w, req)
		want := 200
		if notFound {
			want = 404
		}
		if w.Code != want {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		for _, secret := range []string{"stable-sensitive", `"puuid"`, "private-session-token"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("private identity in overview response")
			}
		}
		if !notFound && !strings.Contains(w.Body.String(), `"proMismatch":true`) {
			t.Fatal("real Riot rank not compared")
		}
	}
}

func TestR73CapturedTeamGateAudit(t *testing.T) {
	file := os.Getenv("R73_DIRECTORY_CAPTURE")
	if file == "" {
		t.Skip("optional local source capture")
	}
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	teams, err := parseOPGGProPlayers(body)
	if err != nil {
		t.Fatal(err)
	}
	for _, team := range proRoster {
		for _, player := range team.Players {
			for _, source := range teams {
				if source.ID == team.OPGGID || proSecondaryTeam(source) {
					continue
				}
				for _, member := range source.Members {
					if strings.EqualFold(member.Nickname, player.Name) && proIdentityMatches(member, player) {
						t.Logf("review %s/%s: upstream team %d %s", team.Code, player.Name, source.ID, source.Name)
					}
				}
			}
		}
	}
}
