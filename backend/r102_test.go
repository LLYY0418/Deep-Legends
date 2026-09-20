package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// Older worklists exercise one original Rookie account, independent of the
// complete R102 catalog and its extra aliases.
func r102RookieSeed() proSeedAccount {
	for _, seed := range proSeedAccounts {
		if seed.Player == "Rookie" {
			seed.Accounts = append([]proSeedAccountRef(nil), seed.Accounts[:1]...)
			return seed
		}
	}
	panic("missing Rookie seed")
}

func r102FixtureSeed(count int) proSeedAccount {
	seed := r102RookieSeed()
	seed.Accounts = nil
	for i := 0; i < count; i++ {
		seed.Accounts = append(seed.Accounts, proSeedAccountRef{fmt.Sprintf("fixture%d", i), "KR1"})
	}
	return seed
}
func r102AccountResponse(r *http.Request) *http.Response {
	name := path.Base(path.Dir(r.URL.Path))
	if strings.Contains(r.URL.Path, "/by-puuid/") {
		name = strings.TrimPrefix(path.Base(r.URL.Path), "id-")
	}
	body, _ := json.Marshal(riotAccount{PUUID: "id-" + name, GameName: name, TagLine: "KR1"})
	return r99Response(string(body))
}
func r102Match(id string, start int64) *http.Response {
	return r99Response(fmt.Sprintf(`{"metadata":{"matchId":%q},"info":{"gameStartTimestamp":%d,"gameCreation":1000000000000,"participants":[{"puuid":"private-fixture"}]}}`, id, start))
}
func TestR102SeedAccountSupportsMultipleAccountsPerPlayer(t *testing.T) {
	seed := r102FixtureSeed(3)
	p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) { return r102AccountResponse(r), nil })
	seen := map[string]bool{}
	for i := range seed.Accounts {
		a, err := p.resolveProSeed(context.Background(), seed, i)
		if err != nil || a.PUUID != "id-"+seed.Accounts[i].GameName || seen[a.PUUID] {
			t.Fatalf("independent identity %d: %+v %v", i, a, err)
		}
		seen[a.PUUID] = true
	}
}
func TestR102SeedAnchorKeysAreUniquePerAccount(t *testing.T) {
	seed := r102FixtureSeed(2)
	root := t.TempDir()
	calls := 0
	handler := func(r *http.Request) (*http.Response, error) { calls++; return r102AccountResponse(r), nil }
	p := r99SeedProvider(t, root, handler)
	for i := range seed.Accounts {
		if _, err := p.resolveProSeed(context.Background(), seed, i); err != nil {
			t.Fatal(err)
		}
	}
	keys := map[string]bool{}
	for key := range p.identityDisk.entries {
		if strings.Contains(key, "|proseed:") {
			keys[key] = true
		}
	}
	if len(keys) != 2 {
		t.Fatalf("anchor count = %d", len(keys))
	}
	p = r99SeedProvider(t, root, handler)
	for i := range seed.Accounts {
		a, err := p.resolveProSeed(context.Background(), seed, i)
		if err != nil || a.PUUID != "id-"+seed.Accounts[i].GameName {
			t.Fatalf("restart anchor %d: %+v %v", i, a, err)
		}
	}
	if calls != 4 {
		t.Fatalf("want two initial names and two stable lookups: %d", calls)
	}
}
func TestR102SeedRealNameAndPositionComeFromRoster(t *testing.T) {
	rows := new(app).loadProSeeds(context.Background(), nil)
	for _, team := range proRoster {
		for _, player := range team.Players {
			found := false
			for _, row := range rows {
				if row.ID == team.OPGGID && row.Members[0].Nickname == player.Name {
					found = true
					m := row.Members[0]
					if m.RealName != player.Names[0] || m.Position != player.Position || m.TeamID != team.OPGGID {
						t.Fatalf("roster metadata mismatch %s: %+v", player.Name, m)
					}
				}
			}
			if !found {
				t.Fatalf("missing roster player %s", player.Name)
			}
		}
	}
}
func TestR102AllThirtyThreePlayersHaveSeeds(t *testing.T) {
	if len(proSeedAccounts) != 33 {
		t.Fatalf("players: %d", len(proSeedAccounts))
	}
	got := map[string]int{}
	seen := map[string]bool{}
	for _, s := range proSeedAccounts {
		got[s.TeamCode]++
		key := s.TeamCode + "/" + s.Player
		if seen[key] {
			t.Fatal("duplicate player", key)
		}
		seen[key] = true
	}
	want := map[string]int{"BLG": 7, "IG": 6, "T1": 5, "HLE": 5, "GEN": 5, "DK": 5}
	if !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
}
func TestR102DyjkbysbBelongsToWei(t *testing.T) {
	count := 0
	for _, s := range proSeedAccounts {
		for _, a := range s.Accounts {
			if a.GameName == "dyjkbysb" && a.TagLine == "KR1" {
				count++
				if s.Player != "Wei" || s.TeamCode != "IG" {
					t.Fatal("wrong owner", s.Player)
				}
			}
		}
	}
	if count != 1 {
		t.Fatal("expected one Wei account", count)
	}
}
func TestR102SeedAccountsMatchVerificationDoc(t *testing.T) {
	// Compare each owner and its ordered account list to the user-approved source,
	// independently of the production catalog. Counts alone miss swapped owners.
	doc, err := os.ReadFile("../docs/pro-accounts-verification-2026-09-17.md")
	if err != nil {
		t.Fatal(err)
	}
	documented := map[string][]string{}
	team := ""
	for _, line := range strings.Split(string(doc), "\n") {
		if strings.HasPrefix(line, "## ") {
			team = ""
			for _, code := range []string{"BLG", "IG", "T1", "HLE", "GEN", "DK"} {
				if strings.HasPrefix(line, "## "+code+"（") {
					team = code
				}
			}
		}
		columns := strings.Split(line, "|")
		if team == "" || len(columns) < 5 {
			continue
		}
		account := strings.Split(columns[3], "`")
		if len(account) < 3 {
			continue
		}
		owner := team + "/" + strings.Trim(strings.TrimSpace(columns[1]), "*")
		documented[owner] = append(documented[owner], account[1])
	}
	actual := map[string][]string{}
	for _, seed := range proSeedAccounts {
		owner := seed.TeamCode + "/" + seed.Player
		for _, account := range seed.Accounts {
			actual[owner] = append(actual[owner], account.GameName+"#"+account.TagLine)
		}
	}
	if len(documented) != 33 || !reflect.DeepEqual(actual, documented) {
		t.Fatalf("seed owners/account order differ from approved document: actual=%v documented=%v", actual, documented)
	}
	want := map[string]int{"BLG": 9, "IG": 15, "T1": 5, "HLE": 8, "GEN": 5, "DK": 11}
	got := map[string]int{}
	seen := map[string]bool{}
	for _, s := range proSeedAccounts {
		for _, a := range s.Accounts {
			got[s.TeamCode]++
			key := a.GameName + "#" + a.TagLine
			if seen[key] {
				t.Fatal("duplicate account", key)
			}
			seen[key] = true
		}
	}
	if len(seen) != 53 || !reflect.DeepEqual(got, want) {
		t.Fatalf("accounts: %d %v", len(seen), got)
	}
	for _, id := range []string{"JUGKlNG#kr", "강 철#샤 넬", "14小孩幻想赢对线#4453", "키보드에음료쏟아서고장내는이도윤#KR1"} {
		if !seen[id] {
			t.Fatal("case/Unicode mismatch", id)
		}
	}
}
func TestR102LastMatchStartParsesGameStartTimestamp(t *testing.T) {
	calls := 0
	p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) {
		calls++
		if strings.HasSuffix(r.URL.Path, "/ids") {
			q := r.URL.Query()
			if q.Get("start") != "0" || q.Get("count") != "1" || q.Has("queue") || q.Has("type") {
				t.Fatal("filtered activity", q)
			}
			return r99Response(`["KR_102"]`), nil
		}
		return r102Match("KR_102", 1757980800000), nil
	})
	at, ok, err := p.lastMatchStart(context.Background(), "private")
	if err != nil || !ok || !at.Equal(time.Date(2025, 9, 16, 0, 0, 0, 0, time.UTC)) || at.Location() != time.UTC || calls != 2 {
		t.Fatalf("wrong start %s %v %v calls=%d", at, ok, err, calls)
	}
}
func TestR102LastMatchStartHandlesNoMatches(t *testing.T) {
	calls := 0
	p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) {
		calls++
		if !strings.HasSuffix(r.URL.Path, "/ids") {
			t.Fatal("detail request for empty list")
		}
		return r99Response(`[]`), nil
	})
	at, ok, err := p.lastMatchStart(context.Background(), "private")
	if err != nil || ok || !at.IsZero() || calls != 1 {
		t.Fatalf("%s %v %v calls=%d", at, ok, err, calls)
	}
}
func TestR102LastMatchStartCachedSevenDays(t *testing.T) {
	calls, ids := 0, 0
	root := t.TempDir()
	now := time.Now()
	handler := func(r *http.Request) (*http.Response, error) {
		calls++
		if strings.HasSuffix(r.URL.Path, "/ids") {
			ids++
			return r99Response(fmt.Sprintf(`["KR_%d"]`, ids)), nil
		}
		return r102Match(path.Base(r.URL.Path), 1757980800123), nil
	}
	p := r99SeedProvider(t, root, handler)
	p.identityDisk.now = func() time.Time { return now }
	read := func() {
		at, ok, err := p.lastMatchStart(context.Background(), "private")
		if err != nil || !ok || at.UnixMilli() != 1757980800123 {
			t.Fatalf("%s %v %v", at, ok, err)
		}
	}
	read()
	if calls != 2 {
		t.Fatal(calls)
	}
	now = now.Add(7*24*time.Hour - time.Second)
	read()
	if calls != 2 {
		t.Fatal("early expiry", calls)
	}
	p = r99SeedProvider(t, root, handler)
	p.identityDisk.now = func() time.Time { return now }
	read()
	if calls != 2 {
		t.Fatal("disk TTL lost", calls)
	}
	now = now.Add(2 * time.Second)
	read()
	if calls != 4 {
		t.Fatal("missing seven-day refresh", calls)
	}
}
func TestR102LastMatchErrorsRemainUnknownAndRetry(t *testing.T) {
	for _, stage := range []string{"ids", "detail", "missing-start"} {
		t.Run(stage, func(t *testing.T) {
			calls := 0
			sentinel := errors.New("fixture transport failure")
			p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) {
				calls++
				if strings.HasSuffix(r.URL.Path, "/ids") {
					if stage == "ids" {
						return nil, sentinel
					}
					return r99Response(`["KR_1"]`), nil
				}
				if stage == "detail" {
					return nil, sentinel
				}
				return r102Match("KR_1", 0), nil
			})
			at, ok, err := p.lastMatchStart(context.Background(), "private")
			if ok || !at.IsZero() {
				t.Fatal("fabricated timestamp", at, ok)
			}
			if stage == "missing-start" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if !errors.Is(err, sentinel) {
				t.Fatal("lost error", err)
			}
			before := calls
			p.lastMatchStart(context.Background(), "private")
			if calls != before+1 {
				t.Fatal("error cached", calls, before)
			}
		})
	}
}
func TestR104SeedTotalBudgetIsBoundedRegardlessOfAccountCount(t *testing.T) {
	budget := func(n int) time.Duration {
		ctx, cancel := proSeedContext(context.Background(), n)
		defer cancel()
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("missing deadline")
		}
		return time.Until(deadline)
	}
	// R104 supersedes R102's linear budget; test both small and full-catalog calls.
	for _, count := range []int{1, 5, 53, 1000} {
		got := budget(count)
		if got < 19900*time.Millisecond || got > 20*time.Second {
			t.Fatalf("count=%d total budget=%s, want fixed 20s", count, got)
		}
	}
}
func TestR102SeedOneAccountFailureDoesNotBlockOthers(t *testing.T) {
	seed := r102FixtureSeed(3)
	timedOut := false
	p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/by-riot-id/fixture1/") {
			<-r.Context().Done()
			timedOut = true
			return nil, r.Context().Err()
		}
		if strings.Contains(r.URL.Path, "/league/") {
			return r99Response(r101SoloEntries), nil
		}
		if strings.HasSuffix(r.URL.Path, "/ids") {
			return r99Response(`[]`), nil
		}
		return r102AccountResponse(r), nil
	})
	rows := (&app{riot: p}).loadProSeedAccounts(context.Background(), nil, []proSeedAccount{seed})
	if len(rows) != 1 || len(rows[0].Members[0].Summoners) != 3 {
		t.Fatal("failed account discarded", rows)
	}
	accounts := rows[0].Members[0].Summoners
	if !timedOut || accounts[0].PUUID != "id-fixture0" || accounts[2].PUUID != "id-fixture2" || len(accounts[0].Rank) == 0 || len(accounts[2].Rank) == 0 || accounts[1].LastMatchAtKnown {
		t.Fatalf("account failure contaminated others: %+v", accounts)
	}
}
func TestR102SeedKeepsPerAccountLastSnapshotOnQuotaExhausted(t *testing.T) {
	seed := r102FixtureSeed(3)
	previous := new(app).loadProSeedAccounts(context.Background(), nil, []proSeedAccount{seed})
	for i := range previous[0].Members[0].Summoners {
		row := &previous[0].Members[0].Summoners[i]
		row.PUUID = fmt.Sprintf("private%d", i)
		row.GameName = fmt.Sprintf("renamed%d", i)
		row.Rank = json.RawMessage(fmt.Sprintf(`{"tier":"MASTER","lp":%d}`, 100+i))
		setProLastMatch(row, time.Unix(int64(100+i), 0), true)
	}
	calls := 0
	p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) { calls++; return nil, errors.New("quota guard failed") })
	for i := 0; i < 90; i++ {
		p.longWindow = append(p.longWindow, time.Now())
	}
	rows := (&app{riot: p}).loadProSeedAccounts(context.Background(), previous, []proSeedAccount{seed})
	if calls != 0 || !reflect.DeepEqual(rows, previous) {
		t.Fatalf("per-account fallback lost: calls=%d rows=%+v", calls, rows)
	}
}
func TestR102SortsByLastMatchTimeDescending(t *testing.T) {
	rows := []proAccount{
		{GameName: "old", Tier: "CHALLENGER", LastMatchAtKnown: true, LastMatchAt: "2026-09-16T00:00:00Z"},
		{GameName: "new", Tier: "IRON", Dormant: true, LastMatchAtKnown: true, LastMatchAt: "2026-09-17T00:00:00.123Z"},
		{GameName: "middle", Tier: "MASTER", LastMatchAtKnown: true, LastMatchAt: "2026-09-17T08:00:00.122+08:00"},
	}
	sort.Slice(rows, func(i, j int) bool { return proAccountLess(rows[i], rows[j]) })
	for i, want := range []string{"new", "middle", "old"} {
		if rows[i].GameName != want {
			t.Fatal(rows)
		}
	}
}
func TestR102KnownAlwaysBeforeUnknown(t *testing.T) {
	known := proAccount{GameName: "low", Tier: "IRON", Dormant: true, LastMatchAtKnown: true, LastMatchAt: "2025-01-01T00:00:00Z"}
	unknown := proAccount{GameName: "high", Tier: "CHALLENGER"}
	if !proAccountLess(known, unknown) || proAccountLess(unknown, known) {
		t.Fatal("unknown sorted before known")
	}
}
func TestR102UnknownLastMatchFallsBackToRankOrder(t *testing.T) {
	// Every adjacent pair isolates a legacy ordering key; both directions are checked.
	pairs := [][2]proAccount{
		{{Dormant: false, Tier: "IRON"}, {Dormant: true, Tier: "CHALLENGER"}},
		{{Tier: "CHALLENGER", Division: 4}, {Tier: "MASTER", Division: 1}},
		{{Tier: "DIAMOND", Division: 1, LP: 1}, {Tier: "DIAMOND", Division: 2, LP: 99}},
		{{Tier: "MASTER", LPKnown: true, LP: 1}, {Tier: "MASTER", LPKnown: false, LP: 99}},
		{{Tier: "MASTER", LPKnown: true, LP: 99}, {Tier: "MASTER", LPKnown: true, LP: 1}},
		{{RankStatus: "unranked", GameName: "z"}, {RankStatus: "unavailable", GameName: "a"}},
		{{GameName: "Alpha", TagLine: "Z"}, {GameName: "beta", TagLine: "A"}},
		{{GameName: "same", TagLine: "A"}, {GameName: "same", TagLine: "B"}},
	}
	for i, pair := range pairs {
		if !proAccountLess(pair[0], pair[1]) || proAccountLess(pair[1], pair[0]) {
			t.Fatalf("legacy key %d changed: %+v", i, pair)
		}
	}
}
func TestR102PrimaryFieldRemoved(t *testing.T) {
	if _, ok := reflect.TypeOf(proAccount{}).FieldByName("Primary"); ok {
		t.Fatal("Primary still exported")
	}
	raw, _ := json.Marshal(proAccount{})
	if strings.Contains(string(raw), `"primary"`) {
		t.Fatal(string(raw))
	}
}

func TestR102UnrankedAccountBacksOffRankQuery(t *testing.T) {
	calls := 0
	now := time.Now()
	start := now
	root := t.TempDir()
	handler := func(r *http.Request) (*http.Response, error) {
		calls++
		return r99Response(`[{"queueType":"RANKED_FLEX_SR","tier":"MASTER","rank":"I","leaguePoints":50}]`), nil
	}
	p := r99SeedProvider(t, root, handler)
	p.identityDisk.now = func() time.Time { return now }
	for _, hour := range []int{0, 1, 70, 73} {
		now = start.Add(time.Duration(hour) * time.Hour)
		rank, err := p.proSeedRank(context.Background(), "private")
		if err != nil || string(rank) != "null" {
			t.Fatalf("rank=%s err=%v", rank, err)
		}
		want := 1
		if hour == 73 {
			want = 2
		}
		if calls != want {
			t.Fatalf("hour %d: requests %d want %d", hour, calls, want)
		}
		if hour == 1 {
			p = r99SeedProvider(t, root, handler)
			p.identityDisk.now = func() time.Time { return now }
		}
	}
}
func TestR102FailedQueryDoesNotTriggerBackoff(t *testing.T) {
	calls := 0
	now := time.Now()
	sentinel := errors.New("rank request failed")
	p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) { calls++; return nil, sentinel })
	p.identityDisk.now = func() time.Time { return now }
	for i := 0; i < 2; i++ {
		_, err := p.proSeedRank(context.Background(), "private")
		if !errors.Is(err, sentinel) {
			t.Fatal("rank failure swallowed", err)
		}
		now = now.Add(time.Hour)
	}
	if calls != 2 {
		t.Fatal("failed query backed off", calls)
	}
}
func TestR102UnrankedBackoffDoesNotAffectLastMatchQuery(t *testing.T) {
	rankCalls, matchCalls := 0, 0
	now := time.Now()
	p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/league/") {
			rankCalls++
			return r99Response(`[]`), nil
		}
		matchCalls++
		return r99Response(`[]`), nil
	})
	p.identityDisk.now = func() time.Time { return now }
	for i := 0; i < 2; i++ {
		if _, err := p.proSeedRank(context.Background(), "private"); err != nil {
			t.Fatal(err)
		}
		if _, _, err := p.lastMatchStart(context.Background(), "private"); err != nil {
			t.Fatal(err)
		}
		now = now.Add(73 * time.Hour)
	}
	if rankCalls != 2 || matchCalls != 1 {
		t.Fatalf("rank/activity backoff coupled: %d/%d", rankCalls, matchCalls)
	}
}
func TestR102NewlyRankedAccountResetsToNormalTTL(t *testing.T) {
	calls := 0
	now := time.Now()
	root := t.TempDir()
	handler := func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return r99Response(`[]`), nil
		}
		return r99Response(r101SoloEntries), nil
	}
	p := r99SeedProvider(t, root, handler)
	p.identityDisk.now = func() time.Time { return now }
	if _, err := p.proSeedRank(context.Background(), "private"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	if _, err := p.leagueEntries(context.Background(), "private"); err != nil {
		t.Fatal(err)
	}
	// A restart must see the early observation too.
	p = r99SeedProvider(t, root, handler)
	p.identityDisk.now = func() time.Time { return now }
	rank, err := p.proSeedRank(context.Background(), "private")
	if err != nil || !strings.Contains(string(rank), "CHALLENGER") || calls != 2 {
		t.Fatalf("early rank ignored %s %v %d", rank, err, calls)
	}
	now = now.Add(24*time.Hour - time.Second)
	if _, err = p.proSeedRank(context.Background(), "private"); err != nil || calls != 2 {
		t.Fatal("expired before 24 hours", err, calls)
	}
	// A newer overview response must not extend an already-ranked cache.
	if _, err = p.leagueEntries(context.Background(), "private"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Second)
	if _, err = p.proSeedRank(context.Background(), "private"); err != nil || calls != 4 {
		t.Fatal("rank kept 72h backoff or sliding TTL", err, calls)
	}
}
func TestR102UpstreamActivityAndSnapshotRemainPrivate(t *testing.T) {
	seed := r102FixtureSeed(3)
	rows := new(app).loadProSeedAccounts(context.Background(), nil, []proSeedAccount{seed})
	for i := range rows[0].Members[0].Summoners {
		row := &rows[0].Members[0].Summoners[i]
		row.Source = "OP.GG"
		row.PUUID = fmt.Sprintf("private%d", i)
		row.SeedKey = ""
	}
	previous := cloneProTeams(rows)
	setProLastMatch(&previous[0].Members[0].Summoners[1], time.UnixMilli(1757980800001), true)
	calls := 0
	p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) {
		calls++
		if strings.Contains(r.URL.Path, "private1") {
			return nil, errors.New("activity unavailable")
		}
		if strings.Contains(r.URL.Path, "private2") {
			return r99Response(`[]`), nil
		}
		if strings.HasSuffix(r.URL.Path, "/ids") {
			return r99Response(`["KR_102"]`), nil
		}
		return r102Match("KR_102", 1757980800123), nil
	})
	a := &app{riot: p, storage: &localStore{root: t.TempDir()}}
	a.enrichProActivity(context.Background(), rows, previous)
	accounts := rows[0].Members[0].Summoners
	if !accounts[0].LastMatchAtKnown || accounts[0].LastMatchAt != "2025-09-16T00:00:00.123Z" || accounts[1].LastMatchAt != previous[0].Members[0].Summoners[1].LastMatchAt || accounts[2].LastMatchAtKnown {
		t.Fatalf("enrichment/fallback %+v", accounts)
	}
	before := calls
	result := a.buildProPlayers(rows, proRoster)
	raw, _ := json.Marshal(result)
	if calls != before || strings.Contains(string(raw), "private") || strings.Contains(string(raw), "SeedKey") || !strings.Contains(string(raw), "lastMatchAtKnown") {
		t.Fatalf("projection leaked identity or queried HTTP: %s", raw)
	}
	rows[0].Members[0].Summoners[2].SeedKey = "proseed:v1:fixture/2"
	a.restoreProSnapshotLocked()
	a.proPlayers.teams = rows
	a.proPlayers.fetchedAt = time.Now()
	a.persistProSnapshotLocked()
	restarted := &app{storage: a.storage}
	if !restarted.restoreProSnapshotLocked() {
		t.Fatal("snapshot not restored")
	}
	wantDisk, _ := json.Marshal(newProDiskSnapshot(rows))
	gotDisk, _ := json.Marshal(newProDiskSnapshot(restarted.proPlayers.teams))
	if string(wantDisk) != string(gotDisk) {
		t.Fatal("restart lost activity/anchor metadata")
	}

	var forged opggProAccount
	if err := json.Unmarshal([]byte(`{"lastMatchAt":"2026-09-17T00:00:00Z","lastMatchAtKnown":true,"SeedKey":"fake"}`), &forged); err != nil {
		t.Fatal(err)
	}
	if forged.LastMatchAtKnown || forged.LastMatchAt != "" || forged.SeedKey != "" {
		t.Fatal("upstream spoofed private activity")
	}
}
func TestR102ColdIdentityFailurePreservesReviewedAccounts(t *testing.T) {
	calls := 0
	p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) { calls++; return nil, errors.New("offline") })
	seed := r102FixtureSeed(3)
	rows := (&app{riot: p}).loadProSeedAccounts(context.Background(), nil, []proSeedAccount{seed})
	accounts := r99Rookie(new(app).buildProPlayers(rows, proRoster)).Accounts
	if len(accounts) != 3 || calls != 3 {
		t.Fatalf("cold identities lost %d %d", len(accounts), calls)
	}
	for _, row := range accounts {
		if row.LastMatchAtKnown || row.LastMatchAt != "" || row.RankStatus != "unavailable" {
			t.Fatal("invented fallback", row)
		}
	}
}
