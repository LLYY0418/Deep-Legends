package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func r104FixtureFlight(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/r104-opgg-pro-directory.json")
	if err != nil {
		t.Fatal(err)
	}
	var records any
	if err := json.Unmarshal(raw, &records); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal("1:" + string(payload) + "\n")
	return []byte("<script>self.__next_f.push([1," + string(encoded) + "])</script>")
}
func r104Rows(teams []opggProTeam) []opggProAccount {
	var result []opggProAccount
	for _, team := range teams {
		for _, member := range team.Members {
			result = append(result, member.Summoners...)
		}
	}
	return result
}
func r104RiotResponse(r *http.Request) (*http.Response, error) {
	switch {
	case strings.Contains(r.URL.Path, "/accounts/"):
		if strings.Contains(r.URL.Path, "subject") || strings.Contains(r.URL.Path, "Foreground") {
			return r99Response(`{"puuid":"subject","gameName":"Foreground","tagLine":"KR1"}`), nil
		}
		return r102AccountResponse(r), nil
	case strings.Contains(r.URL.Path, "/summoner/"):
		return r99Response(`{"puuid":"subject","profileIconId":1,"summonerLevel":760}`), nil
	case strings.HasSuffix(r.URL.Path, "/ids"):
		count, _ := strconv.Atoi(r.URL.Query().Get("count"))
		prefix := "KR_seed_"
		if strings.Contains(r.URL.Path, "/subject/") {
			prefix = "KR_"
		}
		ids := make([]string, count)
		for i := range ids {
			ids[i] = fmt.Sprintf("%s%d", prefix, i+1)
		}
		data, _ := json.Marshal(ids)
		return r99Response(string(data)), nil
	case strings.Contains(r.URL.Path, "/lol/match/v5/matches/"):
		id := path.Base(r.URL.Path)
		return r99Response(fmt.Sprintf(`{"metadata":{"matchId":%q},"info":{"gameId":1,"queueId":420,"gameDuration":1800,"gameStartTimestamp":1757980800123,"participants":[{"puuid":"subject","participantId":1,"teamId":100,"championId":1,"win":true}]}}`, id)), nil
	default:
		return r99Response(`[]`), nil
	}
}

func TestR104RealOPGGFixtureProvidesRevisionTimeAndZeroRiot(t *testing.T) {
	teams, err := parseOPGGProPlayers(r104FixtureFlight(t))
	if err != nil {
		t.Fatal(err)
	}
	var original opggProAccount
	for _, row := range r104Rows(teams) {
		if row.GameName == "Kimman" && row.TagLine == "zxfkk" {
			original = row
		}
	}
	if !original.LastMatchAtKnown || original.LastMatchAt != "2026-09-16T18:56:23Z" || original.RevisionAt != "2026-09-17T03:56:23+09:00" || original.Level != 415 || len(original.Rank) == 0 || original.PUUID == "" {
		t.Fatalf("real fixture fields/time lost: %+v", original)
	}
	var calls atomic.Int32
	p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) { calls.Add(1); return r104RiotResponse(r) })
	a := &app{riot: p}
	var seed proSeedAccount
	for _, candidate := range proSeedAccounts {
		if candidate.TeamCode == "IG" && candidate.Player == "Wei" {
			seed = candidate
			seed.Accounts = []proSeedAccountRef{{"  KIMMAN ", " ZXFKK "}}
		}
	}
	rows := r104Rows(a.loadProSeedAccounts(context.Background(), nil, []proSeedAccount{seed}, teams))
	if len(rows) != 1 || rows[0].PUUID != original.PUUID || rows[0].LastMatchAt != original.LastMatchAt || !rows[0].LastMatchAtKnown || string(rows[0].Rank) != string(original.Rank) || rows[0].Level != original.Level {
		t.Fatalf("directory projection lost: %+v", rows)
	}
	if calls.Load() != 0 {
		t.Fatalf("OPGG hit made %d Riot requests", calls.Load())
	}
	// Read the persisted 30-day anchor from disk, proving rename tracking can
	// survive restart without an account lookup during directory reuse.
	entry, err := p.identityDisk.readDisk(riotIdentityKey(proSeedAnchor(seed, 0)))
	if err != nil {
		t.Fatal(err)
	}
	var anchor struct {
		PUUID string `json:"puuid"`
	}
	if json.Unmarshal(entry.Data, &anchor) != nil || anchor.PUUID != original.PUUID || entry.ExpiresAt.Sub(entry.FetchedAt) != 30*24*time.Hour {
		t.Fatalf("directory anchor lost: %+v", entry)
	}
}

func TestR104ExactDirectoryMatchAvoidsRiotAndMissFallsBack(t *testing.T) {
	seed := r102FixtureSeed(2)
	hit := proFixtureAccount(" FIXTURE0 ", "directory-puuid", "MASTER", 1, 123)
	hit.TagLine = " kr1 "
	hit.RevisionAt = "2026-09-17T00:00:00Z"
	setProLastMatch(&hit, time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC), true)
	// Same gameName with a different tag is a deliberate near miss.
	near := hit
	near.GameName = "fixture1"
	near.TagLine = "KR2"
	directory := []opggProTeam{{Members: []opggProMember{{Summoners: []opggProAccount{hit, near}}}}}
	var calls []string
	p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) {
		calls = append(calls, r.URL.Path)
		return r104RiotResponse(r)
	})
	rows := r104Rows((&app{riot: p}).loadProSeedAccounts(context.Background(), nil, []proSeedAccount{seed}, directory))
	if len(rows) != 2 || rows[0].PUUID != "directory-puuid" || rows[1].PUUID != "id-fixture1" || !rows[1].LastMatchAtKnown {
		t.Fatalf("hit/miss fallback: %+v", rows)
	}
	if len(calls) != 4 || !strings.Contains(calls[0], "/by-riot-id/fixture1/KR1") {
		t.Fatalf("only exact misses may call Riot, calls=%v", calls)
	}
}

func TestR104BackgroundAdmissionYieldsAtThirtyOrForegroundQueue(t *testing.T) {
	now := time.Unix(1000, 0)
	p := &riotProvider{limitNow: func() time.Time { return now }}
	release, err := p.enterRiotLimitQueue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := p.wait(withRiotBackground(context.Background())); !errors.Is(err, errThrottled) {
		t.Fatalf("foreground queue error=%v", err)
	}
	release()
	for i := 0; i < 30; i++ {
		p.longWindow = append(p.longWindow, now)
	}
	if err := p.wait(withRiotBackground(context.Background())); !errors.Is(err, errThrottled) {
		t.Fatalf("shared occupancy 30 must yield: %v", err)
	}
	// Foreground can still use reserved long-window capacity.
	if err := p.wait(context.Background()); err != nil || len(p.longWindow) != 31 {
		t.Fatalf("foreground reserve: %v %d", err, len(p.longWindow))
	}
	p.longWindow = nil
	p.shortWindow = nil
	for i := 0; i < 15; i++ {
		p.shortWindow = append(p.shortWindow, now)
	}
	if err := p.wait(withRiotBackground(context.Background())); !errors.Is(err, errThrottled) {
		t.Fatalf("short-window guard: %v", err)
	}
	now = now.Add(3 * time.Minute)
	if err := p.wait(withRiotBackground(context.Background())); err != nil || len(p.longWindow) != 1 || len(p.shortWindow) != 1 {
		t.Fatalf("expired occupancy must allow background again: %v", err)
	}
}

func TestR104OldSnapshotWithoutRevisionRemainsUnknown(t *testing.T) {
	// Pre-R102 disk envelope, with neither revision_at nor activity extension.
	var disk proDiskSnapshot
	if err := json.Unmarshal([]byte(`{"Teams":[{"id":1,"members":[{"summoners":[{"game_name":"old","tagline":"KR1","puuid":"p","solo_tier_info":{"tier":"CHALLENGER","lp":2000}}]}]}]}`), &disk); err != nil {
		t.Fatal(err)
	}
	rows := r104Rows(disk.restore())
	if len(rows) != 1 {
		t.Fatal("old account dropped")
	}
	unknown, valid := normalizeProAccount(rows[0])
	if !valid || unknown.LastMatchAtKnown || unknown.LastMatchAt != "" || rows[0].RevisionAt != "" {
		t.Fatalf("old snapshot became epoch time: %+v", unknown)
	}
	known := proAccount{GameName: "known", Tier: "IRON", LastMatchAt: "2026-09-17T00:00:00Z", LastMatchAtKnown: true}
	sorted := []proAccount{unknown, known}
	sort.Slice(sorted, func(i, j int) bool { return proAccountLess(sorted[i], sorted[j]) })
	if sorted[0].GameName != "known" || sorted[1].GameName != "old" {
		t.Fatal("unknown high-rank account must sort last", sorted)
	}
}

func TestR104ColdStartupDripProtectsForegroundTwentyMatches(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body := r104FixtureFlight(t)
	var background, foreground, directoryCalls atomic.Int32
	p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == riotClusterHost || r.URL.Host == riotPlatformHost {
			if isRiotBackground(r.Context()) {
				background.Add(1)
			} else {
				foreground.Add(1)
			}
			return r104RiotResponse(r)
		}
		if r.URL.Path == proPlayersPath {
			directoryCalls.Add(1)
			return proHTTPBody(body), nil
		}
		return proHTTPBody([]byte(`<html></html>`)), nil
	})
	// No socket listener or real credentials: only clocks and transport are fake.
	// The real startup publisher, worker, caches, limiter, and overview execute.
	var nanos atomic.Int64
	start := time.Now()
	nanos.Store(start.UnixNano())
	now := func() time.Time { return time.Unix(0, nanos.Load()) }
	p.limitNow = now
	p.limitSleep = func(ctx context.Context, d time.Duration) error {
		nanos.Add(int64(d + time.Nanosecond))
		return ctx.Err()
	}
	p.champions.championMeta = map[int]championMetadata{1: {NameZH: "安妮"}}
	waits := make(chan time.Duration, 4)
	ticks := make(chan struct{})
	stopped := make(chan struct{})
	a := &app{riot: p, champions: p.champions, proRefreshContext: ctx}
	a.proPlayers.refreshNow = now
	a.proPlayers.refreshWait = func(ctx context.Context, d time.Duration) error {
		waits <- d
		select {
		case <-ticks:
			nanos.Add(int64(d))
			return nil
		case <-ctx.Done():
			close(stopped)
			return ctx.Err()
		}
	}
	awaitWait := func() {
		t.Helper()
		select {
		case d := <-waits:
			if d != time.Minute {
				t.Fatalf("drip interval=%s", d)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("startup/worker stuck")
		}
	}
	if _, _, err := a.loadProPlayers(ctx, false); err != nil {
		t.Fatal(err)
	}
	awaitWait() // final directory + supplements + ladder publication completed
	if background.Load() != 0 || foreground.Load() != 0 || directoryCalls.Load() != 1 {
		t.Fatalf("startup unexpectedly called Riot: bg=%d fg=%d directory=%d", background.Load(), foreground.Load(), directoryCalls.Load())
	}
	ticks <- struct{}{}
	awaitWait() // 60s, one cold fallback account
	if got := background.Load(); got != 4 {
		t.Fatalf("one-account minute must cost four cold Riot requests, got %d", got)
	}
	// Hold a foreground FIFO head while a real 20-match search joins behind it.
	release, err := p.enterRiotLimitQueue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	released := false
	defer func() {
		if !released {
			release()
		}
	}()
	type outcome struct {
		data gameplayOverview
		err  error
	}
	result := make(chan outcome, 1)
	go func() {
		data, err := a.loadRiotOverview(ctx, gameplayReference{GameName: "Foreground", TagLine: "KR1", Region: "kr", Privacy: "PRIVATE"}, 0, 20)
		result <- outcome{data, err}
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		p.limitMu.Lock()
		queued := len(p.limitQueue)
		p.limitMu.Unlock()
		if queued >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("foreground never queued")
		}
		time.Sleep(time.Millisecond)
	}
	before := background.Load()
	ticks <- struct{}{}
	awaitWait() // 120s, background must immediately yield
	if background.Load() != before {
		t.Errorf("background inserted requests ahead of waiting foreground: %d -> %d", before, background.Load())
	}
	release()
	released = true
	select {
	case got := <-result:
		if got.err != nil || len(got.data.Matches) != 20 {
			t.Fatalf("foreground must finish 20/20 without throttling: %d %v", len(got.data.Matches), got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("foreground search stuck")
	}
	if background.Load() > 12 || foreground.Load() != 25 {
		t.Fatalf("two-minute quota bg=%d fg=%d", background.Load(), foreground.Load())
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop")
	}
	t.Logf("cold startup: 0 Riot; first minute: 4 background; second minute yields; foreground: 20/20, %d requests", foreground.Load())
}

func TestR104RefreshDiagnosticsCountRequestsAndBudgetSkips(t *testing.T) {
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	if err := os.MkdirAll(filepath.Join(store.root, "logs"), 0700); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if strings.Contains(r.URL.Path, "/accounts/") {
			return r102AccountResponse(r), nil
		}
		return r99Response(`[]`), nil
	})
	a := &app{riot: p, storage: store}
	seed := r102FixtureSeed(1)
	a.loadProSeedAccounts(context.Background(), nil, []proSeedAccount{seed})
	if calls.Load() != 3 {
		t.Fatal("cold seed request fixture", calls.Load())
	}
	// A second, uncached identity must be denied without going to transport.
	now := time.Now()
	p.limitMu.Lock()
	for len(p.longWindow) < 30 {
		p.longWindow = append(p.longWindow, now)
	}
	p.limitMu.Unlock()
	other := seed
	other.Player = "other"
	other.Accounts = []proSeedAccountRef{{"uncached", "KR1"}}
	a.loadProSeedAccounts(context.Background(), nil, []proSeedAccount{other})
	if calls.Load() != 3 {
		t.Fatal("budget skip issued HTTP", calls.Load())
	}
	p.limitMu.Lock()
	p.longWindow = nil
	p.shortWindow = nil
	p.limitMu.Unlock()
	teams := []opggProTeam{{ID: seed.OPGGID, Members: []opggProMember{{TeamID: seed.OPGGID, Nickname: seed.Player, RealName: seed.RealName, Authority: "PROGAMER", Summoners: []opggProAccount{{PUUID: "upstream", GameName: "upstream", TagLine: "KR1"}}}}}}
	a.enrichProActivity(context.Background(), teams, nil)
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	var costs []map[string]any
	for _, line := range strings.Split(string(data), "\n") {
		var event map[string]any
		if json.Unmarshal([]byte(line), &event) == nil && (event["event"] == "pro_seed_cost" || event["event"] == "pro_activity_cost") {
			costs = append(costs, event)
		}
	}
	if len(costs) != 3 {
		t.Fatalf("missing seed/activity events: %s", data)
	}
	want := [][4]int{{1, 1, 3, 0}, {1, 0, 0, 1}, {1, 1, 1, 0}}
	for i, event := range costs {
		for j, key := range []string{"accounts_total", "accounts_refreshed", "riot_requests", "skipped_by_budget"} {
			if event[key] != float64(want[i][j]) {
				t.Fatalf("event %d %s=%v want %d", i, key, event[key], want[i][j])
			}
		}
	}
}
