package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestProSupplementIdentityAllKRAccountsAndRank(t *testing.T) {
	_, player, _ := proSupplementPlayer("TheShy")
	body := []byte(`<h1><a><small>Old Team</small></a> TheShy</h1><table><tr><td>Name</td><td>Kang Seung-lok (강승록)</td></tr></table><div><h4>Accounts</h4><table><tr><td>[KR] one#KR1</td><td>GM 1,234LP</td></tr><tr class="inactive_account"><td>[KR] two#KR2</td><td>Diamond II</td></tr><tr><td>[EUW] not-kr#EUW</td><td>Challenger 900LP</td></tr><tr><td>[KR] three#KR1</td><td>Unranked</td></tr><tr><td colspan="2">Show Inactive</td></tr></table></div>`)
	member, err := parseProSupplement(body, player)
	if err != nil || len(member.Summoners) != 3 {
		t.Fatalf("accounts=%d err=%v", len(member.Summoners), err)
	}
	one, _ := normalizeProAccount(member.Summoners[0])
	two, _ := normalizeProAccount(member.Summoners[1])
	three, _ := normalizeProAccount(member.Summoners[2])
	if one.Tier != "GRANDMASTER" || one.LP != 1234 || !one.LPKnown || one.Source != "TrackingThePros" {
		t.Fatalf("rank=%+v", one)
	}
	if !two.Inactive || two.Tier != "DIAMOND" || two.Division != 2 || two.LPKnown || two.UpdatedAt != "" {
		t.Fatalf("inactive/missing LP=%+v", two)
	}
	if one.Dormant || one.UpdatedAt != "" || three.Dormant || !two.Dormant {
		t.Fatal("unknown timestamp must not imply dormant; only explicit inactive rows fold")
	}
	if three.RankStatus != "unranked" {
		t.Fatalf("rank=%+v", three)
	}
	for _, bad := range [][]byte{[]byte("challenge"), bytes.ReplaceAll(body, []byte("TheShy"), []byte("Other")), bytes.ReplaceAll(body, []byte("Kang Seung-lok (강승록)"), []byte("Different Person")), bytes.ReplaceAll(body, []byte("Accounts"), []byte("Other")), bytes.Repeat([]byte("x"), 1<<20+1)} {
		if _, err := parseProSupplement(bad, player); err == nil {
			t.Fatal("unverified/invalid source accepted")
		}
	}
}

func TestProSupplementFailureDoesNotEraseOPGG(t *testing.T) {
	p := newChampionProvider()
	p.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme != "https" || r.URL.Host != proSupplementHost || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("X-Riot-Token") != "" {
			t.Error("unsafe identity request")
		}
		return proHTTPBody([]byte("bad page")), nil
	})}
	supplements := loadProSupplements(context.Background(), p)
	result := (&app{}).buildProPlayers(supplements, proRoster)
	if result.PlayerCount != 33 || len(result.Warnings) == 0 {
		t.Fatal("failure lost roster/warnings")
	}
	for _, player := range result.Teams[1].Players {
		if player.Name == "TheShy" && player.Status != "partial" {
			t.Fatal("missing failure status")
		}
	}
	if _, err := fetchProSupplement(context.Background(), p, "../../arbitrary"); err == nil {
		t.Fatal("arbitrary identity accepted")
	}
}

func TestProCapturedSupplements(t *testing.T) {
	directory := os.Getenv("PRO_PLAYERS_SUPPLEMENT_CAPTURE")
	if directory == "" {
		t.Skip("set PRO_PLAYERS_SUPPLEMENT_CAPTURE to directory containing locally captured identity HTML")
	}
	var source []opggProTeam
	if capture := os.Getenv("PRO_PLAYERS_CAPTURE"); capture != "" {
		body, err := os.ReadFile(capture)
		if err != nil {
			t.Fatal(err)
		}
		source, err = parseOPGGProPlayers(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range proSupplementPlayers {
		team, player, _ := proSupplementPlayer(name)
		body, err := os.ReadFile(directory + "/pro-ttp-" + strings.ToLower(name) + ".html")
		if err != nil {
			t.Fatal(err)
		}
		member, err := parseProSupplement(body, player)
		if err != nil {
			t.Fatal(err)
		}
		member.TeamID = team.OPGGID
		source = append(source, opggProTeam{ID: team.OPGGID, Name: team.Name, Members: []opggProMember{member}})
		t.Logf("%s: %d verified KR accounts", name, len(member.Summoners))
	}
	result := (&app{}).buildProPlayers(source, proRoster)
	t.Logf("%d players / %d accounts / %d missing", result.PlayerCount, result.AccountCount, result.MissingCount)
	if result.AccountCount < 14 {
		t.Fatal("supplement accounts lost")
	}
	if out := os.Getenv("PRO_PLAYERS_PUBLIC_OUTPUT"); out != "" {
		writeProPublicCapture(t, result, out)
	}
}

func TestProIdentityGraphDeduplicatesRenameChainsAndTransitiveConflicts(t *testing.T) {
	roster := []proRosterTeam{{Code: "TEST", OPGGID: 1, Players: []proRosterPlayer{{Name: "A", Position: "top", Names: []string{"Real A"}}, {Name: "B", Position: "top", Names: []string{"Real B"}}}}}
	raws := []opggProAccount{proFixtureAccount("latest", "one", "MASTER", 1, 50), proFixtureAccount("old", "one", "MASTER", 1, 40), proFixtureAccount("old", "two", "MASTER", 1, 30)}
	for i := range raws {
		raws[i].UpdatedAt = fmt.Sprintf("2026-09-08T0%d:00:00Z", 9-i)
	}
	source := []opggProTeam{{ID: 1, Name: "Test", Members: []opggProMember{proFixtureMember(0, "A", "Real A", raws...)}}}
	result := (&app{}).buildProPlayers(source, roster)
	if result.AccountCount != 1 || result.Teams[0].Players[0].Accounts[0].GameName != "latest" {
		t.Fatalf("rename chain duplicate: %+v", result)
	}
	source[0].Members = append(source[0].Members, proFixtureMember(1, "B", "Real B", proFixtureAccount("old", "three", "MASTER", 1, 10)))
	result = (&app{}).buildProPlayers(source, roster)
	if result.AccountCount != 0 || result.Teams[0].Players[0].Status != "partial" || result.Teams[0].Players[1].Status != "partial" {
		t.Fatal("transitive conflict accepted")
	}
}

func TestProSupplementRetainsOnlyBoundedIdentitySnapshot(t *testing.T) {
	now := time.Now()
	team, player, _ := proSupplementPlayer("TheShy")
	previous := []opggProTeam{{ID: team.OPGGID, Members: []opggProMember{{TeamID: team.OPGGID, Nickname: player.Name, RealName: "Kang Seung-lok (강승록)", Authority: "PROGAMER", Supplement: true, FetchedAt: now.Add(-time.Hour), Summoners: []opggProAccount{{GameName: "verified", TagLine: "KR1", Source: "TrackingThePros"}}}}}}
	failed := func() []opggProTeam {
		return []opggProTeam{{ID: team.OPGGID, Members: []opggProMember{{TeamID: team.OPGGID, Nickname: player.Name, RealName: player.Names[0], Authority: "PROGAMER", Supplement: true, Incomplete: true}}}}
	}
	retained := retainProSupplements(failed(), previous, now)
	member := retained[0].Members[0]
	if len(member.Summoners) != 1 || !member.Summoners[0].Stale || !member.FetchedAt.Equal(previous[0].Members[0].FetchedAt) || !proSupplementsIncomplete(retained) {
		t.Fatal("failed identity erased or stale timestamp advanced")
	}
	if previous[0].Members[0].Summoners[0].Stale {
		t.Fatal("mutated immutable cache")
	}
	result := (&app{}).buildProPlayers(retained, proRoster)
	if result.AccountCount != 1 || !result.Partial || !result.Teams[1].Players[0].Accounts[0].Stale {
		t.Fatal("stale marker lost in public response")
	}
	if len(retainProSupplements(failed(), previous, now.Add(24*time.Hour))[0].Members[0].Summoners) != 0 {
		t.Fatal("expired supplemental cache retained")
	}
	previous[0].Members[0].RealName = "Unrelated person"
	if len(retainProSupplements(failed(), previous, now)[0].Members[0].Summoners) != 0 {
		t.Fatal("different identity retained")
	}
}

func TestProPublicFetchRejectsUnsafeRedirectsAndCookies(t *testing.T) {
	for _, source := range []string{"directory", "supplement"} {
		for _, target := range []string{"https://op.gg:8443/", "https://example.com/", "http://op.gg/", "https://op.gg/other", "https://op.gg" + proPlayersPath + "?region=na", "https://www.trackingthepros.com/player/Other", "https://www.trackingthepros.com:8443/player/TheShy"} {
			t.Run(source+target, func(t *testing.T) {
				calls := 0
				jar, _ := cookiejar.New(nil)
				for _, host := range []string{opggPageHost, proSupplementHost} {
					u, _ := url.Parse("https://" + host)
					jar.SetCookies(u, []*http.Cookie{{Name: "private", Value: "secret"}})
				}
				provider := newChampionProvider()
				provider.client = &http.Client{Jar: jar, Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("X-Riot-Token") != "" {
						t.Fatal("local credentials attached")
					}
					response := proHTTPBody(nil)
					response.StatusCode = http.StatusFound
					response.Header.Set("Location", target)
					return response, nil
				})}
				var err error
				if source == "directory" {
					_, err = fetchProDirectory(context.Background(), provider)
				} else {
					_, err = fetchProSupplement(context.Background(), provider, "TheShy")
				}
				if err == nil || calls != 1 {
					t.Fatalf("redirect accepted: calls=%d err=%v", calls, err)
				}
			})
		}
	}
}
