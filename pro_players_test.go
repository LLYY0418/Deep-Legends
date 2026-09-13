package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func proFixtureAccount(name, puuid, tier string, division, lp int) opggProAccount {
	rank := json.RawMessage("null")
	if tier != "" {
		rank = json.RawMessage(fmt.Sprintf(`{"tier":%q,"division":%d,"lp":%d}`, tier, division, lp))
	}
	return opggProAccount{PUUID: puuid, GameName: name, TagLine: "KR1", UpdatedAt: "2026-09-08T10:00:00+09:00", Rank: rank}
}
func proFixtureMember(team int, name, realName string, accounts ...opggProAccount) opggProMember {
	return opggProMember{TeamID: team, Nickname: name, RealName: realName, Authority: "PROGAMER", Position: "MID", Summoners: accounts}
}
func proFixtureHTML(region string, overrides ...opggProTeam) []byte {
	teams := map[int]opggProTeam{}
	for _, roster := range proRoster {
		teams[roster.OPGGID] = opggProTeam{ID: roster.OPGGID, Name: roster.Name, ShortName: roster.Code, Members: []opggProMember{}}
	}
	for _, team := range overrides {
		teams[team.ID] = team
	}
	var flight strings.Builder
	index := 1
	for _, team := range teams {
		data, _ := json.Marshal([]any{"$", "$L1", fmt.Sprint(team.ID), map[string]any{"region": region, "team": team}})
		fmt.Fprintf(&flight, "%x:%s\n", index, data)
		index++
	}
	// Split in the middle of JSON as real Next.js does.
	text := flight.String()
	split := len(text) / 2
	var page strings.Builder
	for _, part := range []string{text[:split], text[split:]} {
		value, _ := json.Marshal(part)
		fmt.Fprintf(&page, "<script>self.__next_f.push([1,%s])</script>", value)
	}
	return []byte(page.String())
}

func TestProDirectoryParsesAllFlightRecordsAndRequiresKR(t *testing.T) {
	team := opggProTeam{ID: 632, Name: "Bilibili Gaming", ShortName: "BLG", Members: []opggProMember{proFixtureMember(632, "knight", "Zhuo Ding (卓定)", proFixtureAccount("one", "puuid1", "CHALLENGER", 1, 1000))}}
	rows, err := parseOPGGProPlayers(proFixtureHTML("kr", team))
	if err != nil || len(rows) != 6 {
		t.Fatalf("teams=%d err=%v", len(rows), err)
	}
	result := (&app{token: "test"}).buildProPlayers(rows, proRoster)
	if result.PlayerCount != 33 || result.AccountCount != 1 || result.MissingCount != 32 {
		t.Fatalf("unexpected counts: players=%d accounts=%d missing=%d", result.PlayerCount, result.AccountCount, result.MissingCount)
	}
	for _, body := range [][]byte{nil, []byte("<html>challenge</html>"), proFixtureHTML("na", team), bytes.Repeat([]byte("x"), proPlayersMaxBytes+1)} {
		if _, err := parseOPGGProPlayers(body); err == nil {
			t.Fatal("invalid/NA directory accepted")
		}
	}
}

func TestProRosterIdentityMergeExcludesAcademyCoachesHistoricalAndCollisions(t *testing.T) {
	roster := []proRosterTeam{{Code: "BLG", OPGGID: 632, Players: []proRosterPlayer{{Name: "knight", Position: "middle", Names: []string{"Zhuo Ding", "卓定"}}, {Name: "ON", Position: "utility", Names: []string{"Luo Wen-Jun", "骆文俊"}}, {Name: "Missing", Position: "top", Names: []string{"No Data"}}}}, {Code: "IG", OPGGID: 371, Players: []proRosterPlayer{{AllowTeams: []int{858}, Name: "Rookie", Position: "middle", Names: []string{"Song Eui-jin", "송의진"}}}}}
	one := proFixtureAccount("old name", "stable-one", "MASTER", 1, 200)
	newer := proFixtureAccount("BLG 온", "stable-one", "CHALLENGER", 1, 700)
	newer.UpdatedAt = "2026-09-08T12:00:00+09:00"
	conflict := proFixtureAccount("conflict", "shared", "GOLD", 1, 10)
	coachAuthority := proFixtureMember(632, "knight", "Zhuo Ding", proFixtureAccount("coach1", "coach1", "", 0, 0))
	coachAuthority.Authority = "COACH"
	coachPosition := proFixtureMember(632, "knight", "Zhuo Ding", proFixtureAccount("coach2", "coach2", "", 0, 0))
	coachPosition.Position = "COACH"
	source := []opggProTeam{
		{ID: 632, Name: "Bilibili Gaming", Members: []opggProMember{
			proFixtureMember(632, "knight", "Zhuo Ding (卓定)", one, conflict),
			proFixtureMember(632, "Knight", "Zhuo Ding (卓定)", newer, proFixtureAccount("other", "two", "DIAMOND", 2, 99)),
			proFixtureMember(632, "knight", "Different Person", proFixtureAccount("impostor", "bad", "CHALLENGER", 1, 2000)),
			proFixtureMember(632, "ON", "Luo Wen-Jun (骆文俊)", conflict, proFixtureAccount("support", "support", "GOLD", 1, 10)),
			proFixtureMember(632, "Biubiu", "Yu Lei-Xin", proFixtureAccount("former", "former", "", 0, 0)), coachAuthority, coachPosition}},
		{ID: 101, Name: "Bilibili Gaming Junior", Members: []opggProMember{proFixtureMember(101, "Knight", "Zhuo Ding", proFixtureAccount("academy", "academy", "CHALLENGER", 1, 3000))}},
		{ID: 858, Name: "Ninjas in Pyjamas", Members: []opggProMember{proFixtureMember(858, "Rookie", "Song Eui-jin (송의진)", proFixtureAccount("rookie", "rookie", "MASTER", 1, 900))}},
	}
	a := &app{token: "private-session"}
	result := a.buildProPlayers(source, roster)
	knight := result.Teams[0].Players[0]
	if len(knight.Accounts) != 2 || knight.Accounts[0].GameName != "BLG 온" || knight.Status != "partial" {
		t.Fatalf("bad knight merge: %+v", knight)
	}
	if len(result.Teams[0].Players[1].Accounts) != 1 || result.Teams[0].Players[1].Accounts[0].GameName != "support" {
		t.Fatal("account nickname incorrectly interpreted as owner")
	}
	if result.Teams[0].Players[2].Status != "missing" || result.Teams[1].Players[0].Accounts[0].GameName != "rookie" {
		t.Fatal("missing player or historical team reconciliation lost")
	}
	encoded, _ := json.Marshal(result)
	for _, secret := range []string{"stable-one", "puuid", "acct_id", "summoner_id", "private-session", "playerRef", "academy", "impostor", "former", "coach1", "coach2"} {
		if bytes.Contains(encoded, []byte(secret)) {
			t.Fatalf("excluded/private data leaked: %s", secret)
		}
	}
}

func TestProAccountRankingAndMissingRank(t *testing.T) {
	cases := []struct {
		tier         string
		division, lp int
		want         string
	}{
		{"CHALLENGER", 1, 500, "ranked"}, {"MASTER", 0, 1900, "ranked"}, {"DIAMOND", 1, 0, "ranked"}, {"DIAMOND", 2, 99, "ranked"}, {"EMERALD", 1, 100, "ranked"}, {"UNKNOWN", 1, 40, "unavailable"}, {"DIAMOND", 0, 0, "unavailable"}, {"CHALLENGER", 1, -1, "unavailable"}, {"", 0, 0, "unavailable"}, {"UNRANKED", 0, 0, "unranked"},
	}
	accounts := []proAccount{}
	for i, tc := range cases {
		raw := proFixtureAccount(fmt.Sprint(i), "id", tc.tier, tc.division, tc.lp)
		account, valid := normalizeProAccount(raw)
		if !valid || account.RankStatus != tc.want {
			t.Fatalf("%+v => %+v valid=%v", tc, account, valid)
		}
		accounts = append(accounts, account)
	}
	sort.Slice(accounts, func(i, j int) bool { return proAccountLess(accounts[i], accounts[j]) })
	for i, name := range []string{"0", "1", "2", "3", "4", "9"} {
		if accounts[i].GameName != name {
			t.Fatalf("unexpected order: %+v", accounts)
		}
	}
	raw := proFixtureAccount("NA", "na", "CHALLENGER", 1, 999)
	raw.Region = "na"
	if _, valid := normalizeProAccount(raw); valid {
		t.Fatal("NA account accepted")
	}
	raw.Region = "kr"
	raw.Rank = nil
	account, valid := normalizeProAccount(raw)
	if !valid || account.RankStatus != "unavailable" {
		t.Fatal("missing rank reported as unranked")
	}
	raw = proFixtureAccount("ladder", "ladder", "MASTER", 1, 500)
	raw.LadderRank, raw.LadderRankKnown = 3714, true
	account, valid = normalizeProAccount(raw)
	if !valid || !account.LadderRankKnown || account.LadderRank != 3714 {
		t.Fatalf("ladder rank was not preserved: %+v", account)
	}
}

func TestParseOPGGLadderRankUsesRequestedHighlightedRow(t *testing.T) {
	body := []byte(`<table><tbody><tr id="other-KR1"><td>1</td></tr><tr id="빈 스토리-KR1" class="highlight"><td class="text-gray-400">3,714</td><td>빈 스토리#KR1</td></tr></tbody></table>`)
	rank, err := parseOPGGLadderRank(body, "빈 스토리", "KR1")
	if err != nil || rank != 3714 {
		t.Fatalf("rank=%d err=%v", rank, err)
	}
	for _, invalid := range [][]byte{
		nil,
		[]byte(`<tr id="other-KR1"><td>3714</td></tr>`),
		[]byte(`<tr id="빈 스토리-KR1"><td>unknown</td></tr>`),
		[]byte(`<tr id="빈 스토리-KR1"><td>0</td></tr>`),
		bytes.Repeat([]byte("x"), proLadderMaxBytes+1),
	} {
		if _, err := parseOPGGLadderRank(invalid, "빈 스토리", "KR1"); err == nil {
			t.Fatal("invalid ladder response accepted")
		}
	}
}

func TestFetchAndEnrichProLadderRankUsesFixedKRLeaderboard(t *testing.T) {
	var calls atomic.Int32
	provider := newChampionProvider()
	provider.clientMu.Lock()
	provider.client = &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		if !validProLadderURL(request.URL) || request.URL.Query().Get("summoner") != "fixtureaccount-KR1" {
			t.Fatalf("unexpected ladder request: %s", request.URL)
		}
		if request.Header.Get("Accept") != "text/html,application/xhtml+xml" || request.Header.Get("Cookie") != "" || request.Header.Get("Authorization") != "" || request.Header.Get("X-Riot-Token") != "" {
			t.Fatalf("unexpected ladder headers: %v", request.Header)
		}
		return proHTTPBody([]byte(`<table><tr id="fixtureaccount-KR1"><td>42</td><td>fixtureaccount</td></tr></table>`)), nil
	})}
	provider.clientMu.Unlock()

	account := proFixtureAccount("fixtureaccount", "fixture", "CHALLENGER", 1, 1200)
	source := []opggProTeam{{ID: 632, Name: "Bilibili Gaming", Members: []opggProMember{proFixtureMember(632, "Bin", "Chen Ze-Bin (陈泽彬)", account)}}}
	enrichProLadderRanks(context.Background(), provider, source)
	result := new(app).buildProPlayers(source, proRoster)
	got := result.Teams[0].Players[0].Accounts
	if calls.Load() != 1 || len(got) != 1 || !got[0].LadderRankKnown || got[0].LadderRank != 42 || result.LadderRankPartial {
		t.Fatalf("calls=%d account=%+v partial=%v", calls.Load(), got, result.LadderRankPartial)
	}
	if _, err := fetchOPGGLadderRank(context.Background(), provider, "bad#name", "KR1"); err == nil {
		t.Fatal("invalid Riot ID accepted")
	}
}

func newProMockApp(t *testing.T, fetch func(*http.Request) (*http.Response, error)) *app {
	t.Helper()
	p := newChampionProvider()
	p.clientMu.Lock()
	p.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == proSupplementHost {
			name := strings.TrimPrefix(r.URL.Path, "/player/")
			_, player, ok := proSupplementPlayer(name)
			if !ok {
				t.Errorf("unreviewed supplement: %s", name)
			}
			return proHTTPBody([]byte(fmt.Sprintf("<h1>%s</h1><table><tr><td>Name</td><td>%s</td></tr></table><div><h4>Accounts</h4><table></table></div>", player.Name, player.Names[0]))), nil
		}
		return fetch(r)
	})}
	p.clientMu.Unlock()
	return &app{token: "test-session", champions: p}
}
func proHTTPBody(body []byte) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header), ContentLength: int64(len(body))}
}

func TestProCacheSingleflightRefreshBackoffAndStale(t *testing.T) {
	var calls atomic.Int32
	var fail atomic.Bool
	a := newProMockApp(t, func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.URL.Host != opggPageHost || r.URL.Path != proPlayersPath || r.URL.Query().Get("region") != "kr" {
			t.Errorf("unexpected upstream: %s", r.URL)
		}
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("X-Riot-Token") != "" {
			t.Error("private credentials sent")
		}
		if fail.Load() {
			return nil, errors.New("private upstream error")
		}
		time.Sleep(15 * time.Millisecond)
		return proHTTPBody(proFixtureHTML("kr")), nil
	})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			teams, _, err := a.loadProPlayers(context.Background(), true)
			if err != nil || len(teams) != 9 {
				t.Errorf("load=%d err=%v", len(teams), err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("expected one load, got %d", calls.Load())
	}
	_, _, _ = a.loadProPlayers(context.Background(), true)
	if calls.Load() != 1 {
		t.Fatal("refresh bypasses cooldown")
	}
	waitProEnrichment(t, a)
	fail.Store(true)
	a.proPlayers.mu.Lock()
	a.proPlayers.attemptedAt = time.Now().Add(-proPlayersRetry - time.Second)
	a.proPlayers.fetchedAt = time.Now().Add(-proPlayersTTL - time.Minute)
	a.proPlayers.mu.Unlock()
	teams, at, err := a.loadProPlayers(context.Background(), false)
	if err == nil || len(teams) != 9 || time.Since(at) < proPlayersTTL {
		t.Fatal("failed load didn't retain explicitly stale data")
	}
	w := httptest.NewRecorder()
	a.handleProPlayers(w, httptest.NewRequest("GET", "/api/pro-players", nil))
	var response proPlayersResponse
	_ = json.Unmarshal(w.Body.Bytes(), &response)
	if w.Code != 200 || !response.Stale || response.Unavailable || strings.Contains(w.Body.String(), "private upstream error") {
		t.Fatalf("bad stale response: %s", w.Body.String())
	}
	a.proPlayers.mu.Lock()
	a.proPlayers.fetchedAt = time.Now().Add(-proPlayersMaxStale - time.Hour)
	a.proPlayers.mu.Unlock()
	teams, _, _ = a.loadProPlayers(context.Background(), false)
	if len(teams) != 0 {
		t.Fatal("unbounded stale accounts")
	}
	if calls.Load() != 2 {
		t.Fatal("failed requests did not back off")
	}
}

func waitProEnrichment(t *testing.T, a *app) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		a.proPlayers.mu.Lock()
		updating := a.proPlayers.updating
		a.proPlayers.mu.Unlock()
		if !updating {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("background directory enrichment did not finish")
}

func TestProDirectoryPublishesBeforeSlowSupplements(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	a := &app{champions: newChampionProvider()}
	var directoryCalls atomic.Int32
	a.champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == proSupplementHost {
			select {
			case <-release:
			case <-r.Context().Done():
			}
			return nil, errors.New("supplement unavailable")
		}
		directoryCalls.Add(1)
		return proHTTPBody(proFixtureHTML("kr")), nil
	})}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	teams, _, err := a.loadProPlayers(ctx, false)
	if err != nil || len(teams) != 9 {
		t.Fatalf("directory waited for supplement: %v", err)
	}
	a.proPlayers.mu.Lock()
	updating := a.proPlayers.updating
	a.proPlayers.mu.Unlock()
	if !updating {
		t.Fatal("no progressive update marker")
	}
	for i := 0; i < 3; i++ {
		if _, _, err := a.loadProPlayers(ctx, false); err != nil {
			t.Fatal(err)
		}
	}
	if directoryCalls.Load() != 1 {
		t.Fatal("poll refetched directory")
	}
}

func TestProDirectoryFailureRetainsRosterAndAuth(t *testing.T) {
	a := newProMockApp(t, func(*http.Request) (*http.Response, error) { return proHTTPBody([]byte("not a directory")), nil })
	w := httptest.NewRecorder()
	a.handleProPlayers(w, httptest.NewRequest("GET", "/api/pro-players", nil))
	var result proPlayersResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Unavailable || result.PlayerCount != 33 || len(result.Teams) != 6 || result.Teams[0].Players[0].Status != "unavailable" {
		t.Fatal("failed source erased roster")
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("private response browser-cached")
	}
	for _, query := range []string{"?region=na", "?refresh=2", "?refresh=1&refresh=1", "?url=https://evil.invalid"} {
		w = httptest.NewRecorder()
		a.handleProPlayers(w, httptest.NewRequest("GET", "/api/pro-players"+query, nil))
		if w.Code != 400 {
			t.Fatalf("query %s accepted", query)
		}
	}
	w = httptest.NewRecorder()
	a.authorized(a.handleProPlayers)(w, httptest.NewRequest("GET", "/api/pro-players", nil))
	if w.Code != 401 {
		t.Fatal("unauthenticated directory accepted")
	}
}

// Opt-in local verification uses the same parser + normalization as production;
// never check real stable player IDs into the repository or test output.
func TestProCapturedDirectory(t *testing.T) {
	file := os.Getenv("PRO_PLAYERS_CAPTURE")
	if file == "" {
		t.Skip("set PRO_PLAYERS_CAPTURE to a locally captured OP.GG KR HTML file")
	}
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	source, err := parseOPGGProPlayers(body)
	if err != nil {
		t.Fatal(err)
	}
	result := (&app{token: "capture-session"}).buildProPlayers(source, proRoster)
	for _, team := range result.Teams {
		for _, player := range team.Players {
			t.Logf("%s %s: %d accounts (%s)", team.Code, player.Name, len(player.Accounts), player.Status)
		}
	}
	t.Logf("%d teams, %d roster players, %d accounts, %d without accounts", len(result.Teams), result.PlayerCount, result.AccountCount, result.MissingCount)
	if len(result.Teams) != 6 || result.AccountCount < 40 {
		t.Fatal("unexpectedly incomplete live source")
	}
	if out := os.Getenv("PRO_PLAYERS_PUBLIC_OUTPUT"); out != "" && os.Getenv("PRO_PLAYERS_SUPPLEMENT_CAPTURE") == "" {
		writeProPublicCapture(t, result, out)
	}
}

func TestProCapturedLadder(t *testing.T) {
	file := os.Getenv("PRO_LADDER_CAPTURE")
	gameName := os.Getenv("PRO_LADDER_GAME_NAME")
	tagLine := os.Getenv("PRO_LADDER_TAG_LINE")
	if file == "" || gameName == "" || tagLine == "" {
		t.Skip("set PRO_LADDER_CAPTURE, PRO_LADDER_GAME_NAME and PRO_LADDER_TAG_LINE to a local OP.GG leaderboard capture")
	}
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	rank, err := parseOPGGLadderRank(body, gameName, tagLine)
	if err != nil || rank <= 0 {
		t.Fatalf("live ladder parser failed: rank=%d err=%v", rank, err)
	}
	t.Log("live ladder rank parsed")
}

func writeProPublicCapture(t *testing.T, result proPlayersResponse, out string) {
	t.Helper()
	result.RosterVerifiedAt = proRosterVerifiedAt
	result.FetchedAt = time.Now()
	data, _ := json.MarshalIndent(result, "", "  ")
	if err := os.WriteFile(out, data, 0600); err != nil {
		t.Fatal(err)
	}
}
