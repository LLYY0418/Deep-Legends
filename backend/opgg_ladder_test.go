package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func ladderFixture(t *testing.T) []byte {
	t.Helper()
	body, err := os.ReadFile("testdata/opgg_kr_solo_ladder.html")
	if err != nil || len(body) > 100<<10 {
		t.Fatalf("ladder fixture: %d bytes, %v", len(body), err)
	}
	return body
}

func TestR320LadderParse(t *testing.T) {
	body := ladderFixture(t)
	page, skipped, err := parseOPGGLadderPage(body, nil)
	if err != nil || skipped != 0 || len(page.Rows) != 100 {
		t.Fatalf("page: %d rows, %d skipped, %v", len(page.Rows), skipped, err)
	}
	if page.Page != 1 || page.ChallengerCutoffLP != 1862 || page.ChallengerCount != 300 ||
		page.GrandmasterCutoffLP != 1371 || page.GrandmasterCount != 700 || page.TotalSummoners != 3288698 {
		t.Fatalf("metadata: %+v", page.ladderMetadata)
	}
	want := ladderSourceRow{ladderRow: ladderRow{
		Rank: 1, GameName: "抖音阿甘爱跑步", TagLine: "xtz小弟", Tier: "challenger",
		ProfileIconID: 7233, LP: 2700, Level: 213, Wins: 502, Losses: 407, WinRate: 55,
		Champions: []ladderChampion{{107, "傲之追猎者"}, {91, "刀锋之影"}, {134, "暗黑元首"}},
	}}
	if !reflect.DeepEqual(page.Rows[0], want) || !page.Rows[1].OPGGPro {
		t.Fatalf("first two rows: %+v / %+v", page.Rows[0], page.Rows[1])
	}
	division := strings.Replace(string(body), "<div>王者</div>", "<div>钻石 1</div>", 1)
	page, skipped, err = parseOPGGLadderPage([]byte(division), nil)
	if err != nil || skipped != 0 || page.Rows[0].Tier != "diamond" || page.Rows[0].Division != 1 {
		t.Fatalf("tier with division: %d skipped, %v", skipped, err)
	}
	broken := strings.Replace(string(body), "<td>1</td>", "<td>bad</td>", 1)
	page, skipped, err = parseOPGGLadderPage([]byte(broken), nil)
	if err != nil || skipped != 1 || len(page.Rows) != 99 || page.Rows[0].Rank != 2 {
		t.Fatalf("broken row: %d parsed, %d skipped, %v", len(page.Rows), skipped, err)
	}
}

func TestR320LadderURLAndRedirect(t *testing.T) {
	base := "https://op.gg/zh-cn/lol/leaderboards/tier?type=ladder&region=kr"
	for _, test := range []struct {
		url string
		ok  bool
	}{
		{base, true}, {base + "&page=100&tier=grandmaster", true}, {base + "&summoner=T1+%EA%B5%AC%EB%A6%84-2007", true},
		{base + "&page=101", false}, {base + "&tier=all", false}, {base + "&tier=bogus", false},
		{base + "&page=1&page=2", false}, {base + "&region=kr", false}, {base + "&unlisted=1", false},
		{base + "&summoner=Alpha", false}, {base + "&summoner=%2Ftag-KR1", false}, {base + "#fragment", false},
		{strings.Replace(base, "region=kr", "region=na", 1), false}, {strings.Replace(base, "type=ladder", "type=flex", 1), false},
		{strings.Replace(base, "&region=kr", "", 1), false}, {strings.Replace(base, "op.gg", "other.test", 1), false},
	} {
		u, err := url.Parse(test.url)
		if err != nil || validOPGGLadderURL(u) != test.ok {
			t.Errorf("URL %q want %v (%v)", test.url, test.ok, err)
		}
	}
	cp := proTestProvider()
	calls := 0
	cp.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Cookie") != "" || r.URL.Query().Get("region") != "kr" {
			t.Errorf("unexpected ladder request: %v", r.URL)
		}
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": {base + "&page=101"}}, Body: http.NoBody, Request: r}, nil
	})}
	if _, err := fetchOPGGLadderPage(context.Background(), cp, ladderKey{page: 1}, ""); err == nil || calls != 1 {
		t.Fatalf("invalid redirect: %v, requests %d", err, calls)
	}
}

func TestR320LadderCacheAndSingleflight(t *testing.T) {
	body := ladderFixture(t)
	a := &app{storage: &localStore{root: t.TempDir()}}
	p := newLadderProvider(a)
	var clock atomic.Int64
	now := time.Now().UTC()
	clock.Store(now.UnixNano())
	p.now = func() time.Time { return time.Unix(0, clock.Load()) }
	key := ladderKey{page: 1}
	updated := make(chan ladderKey, 1)
	p.updated = func(key ladderKey) { updated <- key }
	var calls atomic.Int32
	var diagnosticsMu sync.Mutex
	cacheStates := make(map[string]int)
	p.observe = func(event map[string]any) {
		diagnosticsMu.Lock()
		defer diagnosticsMu.Unlock()
		state, _ := event["cache"].(string)
		cacheStates[state]++
		if text, _ := json.Marshal(event); strings.Contains(string(text), "xtz") {
			t.Error("diagnostic contains account ID")
		}
	}
	release := make(chan struct{})
	started := make(chan struct{}, 3)
	p.fetch = func(ctx context.Context, _ ladderKey, _ string) ([]byte, error) {
		calls.Add(1)
		started <- struct{}{}
		select {
		case <-release:
			return body, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	waiters := make(chan error, 10)
	for i := 0; i < cap(waiters); i++ {
		go func() {
			page, err := p.load(ctx, key, false, false)
			if err == nil && len(page.Rows) != 100 {
				err = errors.New("page is incomplete")
			}
			waiters <- err
		}()
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("initial flight never started")
	}
	close(release)
	for i := 0; i < cap(waiters); i++ {
		if err := <-waiters; err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("initial flight made %d requests", calls.Load())
	}
	reopened := newLadderProvider(a)
	reopened.now = p.now
	reopened.fetch = func(context.Context, ladderKey, string) ([]byte, error) {
		return nil, errors.New("disk hit downloaded")
	}
	if fresh, err := reopened.load(ctx, key, false, false); err != nil || !fresh.FetchedAt.Equal(now) {
		t.Fatalf("disk hit: %v %v", fresh.FetchedAt, err)
	}
	if location := reopened.cachedRank("사모예드", "1021"); location.Rank != 2 {
		t.Fatalf("cached rank: %+v", location)
	}

	staleTime := now.Add(ladderFreshFor)
	clock.Store(staleTime.UnixNano())
	refreshRelease := make(chan struct{})
	p.fetch = func(ctx context.Context, _ ladderKey, _ string) ([]byte, error) {
		calls.Add(1)
		select {
		case <-refreshRelease:
			return body, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	for i := 0; i < 3; i++ {
		stale, err := p.load(ctx, key, false, true)
		if err != nil || !stale.FetchedAt.Equal(now) {
			t.Fatalf("stale return: %v %v", stale.FetchedAt, err)
		}
	}
	if _, err := p.load(ctx, ladderKey{page: 2}, false, true); !errors.Is(err, errLadderPrefetchBusy) {
		t.Fatalf("stale prefetch should hold the global slot: %v", err)
	}
	close(refreshRelease)
	select {
	case eventKey := <-updated:
		if eventKey != key {
			t.Fatalf("SSE key = %+v", eventKey)
		}
	case <-ctx.Done():
		t.Fatal("stale refresh did not notify")
	}
	if data, err := p.load(ctx, key, false, false); err != nil || !data.FetchedAt.Equal(staleTime) || calls.Load() != 2 {
		t.Fatalf("stale refresh: %v %v %d requests", data.FetchedAt, err, calls.Load())
	}
	expiredTime := staleTime.Add(ladderMaxAge)
	clock.Store(expiredTime.UnixNano())
	if data, err := p.load(ctx, key, false, false); err != nil || !data.FetchedAt.Equal(expiredTime) || calls.Load() != 3 {
		t.Fatalf("expired refresh: %v %v %d requests", data.FetchedAt, err, calls.Load())
	}
	diagnosticsMu.Lock()
	defer diagnosticsMu.Unlock()
	if cacheStates["miss"] != 3 || cacheStates["stale"] != 3 || cacheStates["hit"] < 1 {
		t.Fatalf("diagnostic cache states: %v", cacheStates)
	}
}

func TestR320LadderExactProMarks(t *testing.T) {
	a := new(app)
	a.ladder = newLadderProvider(a)
	a.ladder.fetch = func(context.Context, ladderKey, string) ([]byte, error) { return ladderFixture(t), nil }
	a.ladder.index = func() proIdentityIndex {
		return proIdentityIndex{byKey: map[string]*proIdentityBadge{
			"name:抖音阿甘爱跑步#xtz小弟": {TeamCode: "T1", PlayerName: "Reviewed"},
			"name:phan#korea":    {TeamCode: "GEN", PlayerName: "Other"},
			"name:phantasm#kr1":  {TeamCode: "T1", PlayerName: "Different tag"},
		}}
	}
	rec := httptest.NewRecorder()
	a.handleLadder(rec, httptest.NewRequest(http.MethodGet, "/api/ladder", nil))
	var got struct{ Rows []ladderDisplayRow }
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil || len(got.Rows) != 100 {
		t.Fatalf("ladder response: %d, %d rows", rec.Code, len(got.Rows))
	}
	if row := got.Rows[0]; !row.ProMark || row.ProPlayer == nil || row.ProPlayer.Name != "Reviewed" || row.ProPlayer.Team != "T1" {
		t.Fatalf("reviewed roster row: %+v", row)
	}
	if row := got.Rows[1]; !row.ProMark || row.ProPlayer != nil {
		t.Fatalf("upstream-only pro row: %+v", row)
	}
	if row := got.Rows[5]; row.GameName != "Phantasm" || row.ProMark || row.ProPlayer != nil {
		t.Fatalf("ordinary row with similar identities: %+v", row)
	}
	if strings.Contains(rec.Body.String(), `"opggPro"`) {
		t.Fatal("internal marker leaked into renderer payload")
	}
}

func TestR320LadderFindReusesDownloadedPage(t *testing.T) {
	body := string(ladderFixture(t))
	body = strings.Replace(body, "#1 ~ #100", "#101 ~ #200", 1)
	body = mapLadderFixtureRows(body, func(i int, row string) string {
		return replaceLadderFixtureCell(row, 0, strconv.Itoa(101+i))
	})
	a := new(app)
	a.ladder = newLadderProvider(a)
	calls := 0
	a.ladder.fetch = func(_ context.Context, key ladderKey, summoner string) ([]byte, error) {
		calls++
		if key != (ladderKey{}) || summoner != "抖音阿甘爱跑步-xtz小弟" {
			return nil, errors.New("unexpected find request")
		}
		return []byte(body), nil
	}
	rec := httptest.NewRecorder()
	target := "/api/ladder/find?riotId=" + url.QueryEscape("抖音阿甘爱跑步#xtz小弟")
	a.handleLadderFind(rec, httptest.NewRequest(http.MethodGet, target, nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"page":2,"rank":101`) {
		t.Fatalf("find response: %d %s", rec.Code, rec.Body.String())
	}
	data, err := a.ladder.load(context.Background(), ladderKey{page: 2}, false, false)
	if err != nil || data.Page != 2 || calls != 1 {
		t.Fatalf("lookup re-fetch: page %d, %d network calls, %v", data.Page, calls, err)
	}
}

func TestR320LadderFindCanOpenPageBeyondBrowseLimit(t *testing.T) {
	body := string(ladderFixture(t))
	start, afterFirstRow := strings.Index(body, "<tr"), strings.Index(body, "</tr>")+len("</tr>")
	body = body[:afterFirstRow] + body[strings.Index(body, "</tbody>"):]
	body = strings.Replace(body, "#1 ~ #100", "#47,501 ~ #47,600", 1)
	body = body[:start] + strings.Replace(body[start:], "<td>1</td>", "<td>47,501</td>", 1)
	a := new(app)
	a.ladder = newLadderProvider(a)
	a.ladder.fetch = func(_ context.Context, key ladderKey, summoner string) ([]byte, error) {
		if key != (ladderKey{}) || summoner == "" {
			return nil, errors.New("lookup paged through an unsupported URL")
		}
		return []byte(body), nil
	}
	rec := httptest.NewRecorder()
	target := "/api/ladder/find?riotId=" + url.QueryEscape("抖音阿甘爱跑步#xtz小弟")
	a.handleLadderFind(rec, httptest.NewRequest(http.MethodGet, target, nil))
	var found ladderLocation
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &found) != nil || found.Page != 476 ||
		found.Rank != 47501 || found.Data == nil || found.Data.Page != found.Page || len(found.Data.Rows) != 1 ||
		found.Data.Rows[0].Rank != found.Rank {
		t.Fatalf("distant find response: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	a.handleLadder(rec, httptest.NewRequest(http.MethodGet, "/api/ladder?page=476", nil))
	if rec.Code != 400 {
		t.Fatalf("direct page above cap: %d", rec.Code)
	}
}

func TestR320LadderStartupDoesNotFetch(t *testing.T) {
	var calls atomic.Int32
	cp := proTestProvider()
	cp.client = &http.Client{Transport: gameplayRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("startup must stay offline")
	})}
	a := &app{storage: &localStore{root: t.TempDir()}, champions: cp}
	a.ladder = newLadderProvider(a)
	a.warmProPlayersCaches()
	a.ladder.mu.Lock()
	defer a.ladder.mu.Unlock()
	if calls.Load() != 0 || len(a.ladder.flights) != 0 || len(a.ladder.pages) != 0 {
		t.Fatal("startup prewarmed the ladder")
	}
}

func TestR320LadderPrewarmUsesKRScopeAndPublicReference(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	a := &app{storage: &localStore{root: t.TempDir()}, champions: newChampionProvider()}
	a.champions.cache = newChampionDataCache(a.storage)
	a.riot = newRiotProvider(a.champions)
	a.ladder = newLadderProvider(a)

	var paths []string
	a.champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.Host+r.URL.Path)
		body := `{"puuid":"ladder-fixture-puuid","gameName":"Riot Fixture","tagLine":"KR1"}`
		if strings.Contains(r.URL.Path, "/league/") {
			body = `[{"queueType":"RANKED_SOLO_5x5","tier":"MASTER","leaguePoints":123}]`
		}
		return specialistTestResponse(r, 200, body)
	})}
	rec := httptest.NewRecorder()
	a.handleLadderPrewarm(rec, httptest.NewRequest(http.MethodGet, "/api/ladder/prewarm?riotId=Riot+Fixture%23KR1", nil))
	var wire map[string]string
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &wire) != nil || strings.Contains(rec.Body.String(), "ladder-fixture-puuid") {
		t.Fatalf("prewarm wire response: %d %s", rec.Code, rec.Body.String())
	}
	ref, ok := a.resolveGameplayReferenceDetails(wire["playerRef"])
	ctx := withRiotPlatform(context.Background(), "kr")
	if !ok || ref.Region != "kr" || ref.PlayerRef != "ladder-fixture-puuid" || ref.RiotCredentialScope != riotCredentialScope(ctx) {
		t.Fatalf("warm reference not reusable: %+v", ref)
	}
	ctx = context.WithValue(ctx, riotLookupIdentityKey{}, riotLookupIdentity{ref.GameName, ref.TagLine, ref.PlayerRef, ref.RiotCredentialScope})
	entries, err := a.riot.forPlatform("kr").leagueEntries(ctx, ref.PlayerRef)
	if err != nil || len(entries) != 1 || entries[0].LeaguePoints != 123 {
		t.Fatalf("ranks cache: %+v %v", entries, err)
	}
	want := []string{"asia.api.riotgames.com/riot/account/v1/accounts/by-riot-id/Riot Fixture/KR1", "kr.api.riotgames.com/lol/league/v4/entries/by-puuid/ladder-fixture-puuid"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("Riot calls: %v", paths)
	}
	r274Flush(t, a.riot.identityDisk)
}

func TestLadderRetryBackoff(t *testing.T) {
	body := ladderFixture(t)
	upstreamErr := errors.New("OP.GG unavailable")
	key := ladderKey{page: 1}

	t.Run("stale and manual refresh", func(t *testing.T) {
		p := newLadderProvider(new(app))
		now := time.Now()
		p.now = func() time.Time { return now }
		old := ladderSnapshot{ladderMetadata: ladderMetadata{Page: 1, FetchedAt: now.Add(-ladderFreshFor)}}
		p.save(key, old)
		var calls atomic.Int32
		release := make(chan struct{})
		defer func() {
			select {
			case <-release:
			default:
				close(release)
			}
		}()
		p.fetch = func(ctx context.Context, _ ladderKey, _ string) ([]byte, error) {
			if calls.Add(1) == 1 {
				select {
				case <-release:
					return nil, upstreamErr
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return body, nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		requireStale := func() {
			t.Helper()
			data, err := p.load(ctx, key, false, true)
			if err != nil || !data.FetchedAt.Equal(old.FetchedAt) {
				t.Fatalf("stale page: %v, %v", data.FetchedAt, err)
			}
		}
		requireStale()
		p.mu.Lock()
		finished := p.flights[key].done
		p.mu.Unlock()
		close(release)
		select {
		case <-finished:
		case <-ctx.Done():
			t.Fatal("background failure did not complete")
		}
		for i := 0; i < 3; i++ {
			requireStale()
		}
		p.mu.Lock()
		retried := p.flights[key] != nil
		p.mu.Unlock()
		if calls.Load() != 1 || retried {
			t.Fatal("stale request retried during backoff")
		}
		data, err := p.load(ctx, key, true, false)
		if err != nil || calls.Load() != 2 || !data.FetchedAt.Equal(now) {
			t.Fatalf("manual refresh: %v, %v, %d calls", data.FetchedAt, err, calls.Load())
		}
		if _, failed := p.failures[key]; failed {
			t.Fatal("successful refresh left the failure record")
		}
		if _, err := p.load(ctx, key, false, false); err != nil || calls.Load() != 2 {
			t.Fatalf("fresh after recovery: %v, %d calls", err, calls.Load())
		}
	})

	t.Run("cold error and retry boundary", func(t *testing.T) {
		p := newLadderProvider(new(app))
		now := time.Now()
		var clock atomic.Int64
		clock.Store(now.UnixNano())
		p.now = func() time.Time { return time.Unix(0, clock.Load()) }
		var calls atomic.Int32
		p.fetch = func(context.Context, ladderKey, string) ([]byte, error) {
			if calls.Add(1) == 1 {
				return nil, upstreamErr
			}
			return body, nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := p.load(ctx, key, false, false); !errors.Is(err, upstreamErr) {
			t.Fatalf("cold failure: %v", err)
		}
		clock.Store(now.Add(ladderRetryAfter - time.Nanosecond).UnixNano())
		if _, err := p.load(ctx, key, false, false); !errors.Is(err, upstreamErr) || calls.Load() != 1 {
			t.Fatalf("cold backoff: %v, %d calls", err, calls.Load())
		}
		clock.Store(now.Add(ladderRetryAfter).UnixNano())
		if data, err := p.load(ctx, key, false, false); err != nil || calls.Load() != 2 || len(data.Rows) != 100 {
			t.Fatalf("retry boundary: %v, %d calls, %d rows", err, calls.Load(), len(data.Rows))
		}
	})
}

func TestLadderMemoryPageLimit(t *testing.T) {
	p := newLadderProvider(&app{storage: &localStore{root: t.TempDir()}})
	now := time.Now()
	p.now = func() time.Time { return now }
	p.fetch = func(context.Context, ladderKey, string) ([]byte, error) {
		return nil, errors.New("disk recovery must not fetch")
	}
	tiers := []string{"", "challenger", "grandmaster"}
	for i := 0; i <= ladderMaxCachedPages; i++ {
		key := ladderKey{page: i%ladderMaxPage + 1, tier: tiers[i/ladderMaxPage]}
		data := ladderSnapshot{ladderMetadata: ladderMetadata{
			Page: key.page, Tier: key.tier, FetchedAt: now.Add(time.Duration(i-ladderMaxCachedPages) * time.Millisecond),
		}}
		data.Rows = []ladderSourceRow{{ladderRow: ladderRow{GameName: fmt.Sprintf("fixture%d", i), TagLine: "KR1", Rank: i + 1}}}
		p.save(key, data)
		if i == 0 {
			p.flights[key] = &ladderFlight{done: make(chan struct{})}
		}
		if len(p.pages) > ladderMaxCachedPages {
			t.Fatalf("%d memory pages exceed limit %d", len(p.pages), ladderMaxCachedPages)
		}
	}
	if _, retained := p.pages[ladderKey{page: 1}]; !retained {
		t.Fatal("in-flight page was evicted")
	}
	evicted := ladderKey{page: 2}
	if _, retained := p.pages[evicted]; retained {
		t.Fatal("oldest eligible page was not evicted")
	}
	if _, retained := p.pages[ladderKey{page: 3}]; !retained {
		t.Fatal("newer page was evicted")
	}
	if got := p.cachedRank("fixture1", "KR1"); got.Rank != 2 {
		t.Fatalf("evicted but fresh rank: %+v", got)
	}
	if data, err := p.load(context.Background(), evicted, false, false); err != nil ||
		len(data.Rows) != 1 || data.Rows[0].Rank != 2 || len(p.pages) > ladderMaxCachedPages {
		t.Fatalf("evicted page did not recover from disk: page=%d, rows=%d, memory=%d, err=%v", data.Page, len(data.Rows), len(p.pages), err)
	}
}

func TestLadderCachedRankIndex(t *testing.T) {
	a := &app{storage: &localStore{root: t.TempDir()}}
	p := newLadderProvider(a)
	now := time.Now().UTC()
	p.now = func() time.Time { return now }
	row := func(name string, rank int) ladderSourceRow {
		return ladderSourceRow{ladderRow: ladderRow{GameName: name, TagLine: "KR1", Rank: rank}}
	}
	save := func(provider *ladderProvider, page int, tier string, age time.Duration, rows ...ladderSourceRow) {
		provider.save(ladderKey{page: page, tier: tier}, ladderSnapshot{
			ladderMetadata: ladderMetadata{Page: page, Tier: tier, FetchedAt: now.Add(-age)}, Rows: rows,
		})
	}
	save(p, 2, "", time.Minute, row("Shared", 104), row("Removed", 105))
	save(p, 1, "", 2*time.Minute, row("Shared", 8))
	save(p, 3, "", ladderMaxAge, row("Expired", 215))
	save(p, 1, "master", 0, row("Shared", 1), row("Filtered", 2))

	for _, candidate := range []*ladderProvider{p, newLadderProvider(a)} {
		candidate.now = p.now
		if got := candidate.cachedRank(" shared ", "kr1"); got != (ladderLocation{Page: 2, Rank: 104}) {
			t.Fatalf("latest exact rank: %+v", got)
		}
		for _, missing := range []string{"Expired", "Filtered", "Share"} {
			if got := candidate.cachedRank(missing, "KR1"); got.Rank != 0 {
				t.Fatalf("%q incorrectly indexed: %+v", missing, got)
			}
		}
	}
	save(p, 2, "", 0, row("New", 111))
	if got := p.cachedRank("Shared", "KR1"); got != (ladderLocation{Page: 1, Rank: 8}) {
		t.Fatalf("replacement did not fall back to the other page: %+v", got)
	}
	if got := p.cachedRank("Removed", "KR1"); got.Rank != 0 {
		t.Fatalf("replacement left a removed Riot ID: %+v", got)
	}
	if got := p.cachedRank("New", "KR1"); got.Rank != 111 {
		t.Fatalf("replacement did not index the new row: %+v", got)
	}
}

func BenchmarkCachedRank(b *testing.B) {
	for _, count := range []int{100, 10000} {
		b.Run(fmt.Sprintf("rows=%d", count), func(b *testing.B) {
			p := newLadderProvider(new(app))
			now := time.Now()
			for first := 0; first < count; first += ladderPageSize {
				page := first/ladderPageSize + 1
				data := ladderSnapshot{ladderMetadata: ladderMetadata{Page: page, FetchedAt: now}}
				for i := first; i < first+ladderPageSize; i++ {
					data.Rows = append(data.Rows, ladderSourceRow{ladderRow: ladderRow{
						GameName: fmt.Sprintf("Player%d", i), TagLine: "KR1", Rank: i + 1,
					}})
				}
				p.save(ladderKey{page: page}, data)
			}
			name := fmt.Sprintf("Player%d", count-1)
			if got := p.cachedRank(name, "KR1").Rank; got != count {
				b.Fatalf("rank = %d, want %d", got, count)
			}
			b.ReportAllocs()
			for b.Loop() {
				p.cachedRank(name, "KR1")
			}
		})
	}
}

var ladderFixtureRowPattern = regexp.MustCompile(`(?s)<tr\b.*?</tr>`)
var ladderFixtureCellPattern = regexp.MustCompile(`(?s)<td>.*?</td>`)

func mapLadderFixtureRows(body string, update func(int, string) string) string {
	i := 0
	return ladderFixtureRowPattern.ReplaceAllStringFunc(body, func(row string) string {
		mapped := update(i, row)
		i++
		return mapped
	})
}

func replaceLadderFixtureCell(row string, column int, html string) string {
	cell := ladderFixtureCellPattern.FindAllStringIndex(row, -1)[column]
	return row[:cell[0]] + "<td>" + html + "</td>" + row[cell[1]:]
}

func TestLadderParseLocaleAndTierBadge(t *testing.T) {
	body := ladderFixture(t)
	english := string(body)
	for key, label := range ladderTiers {
		english = strings.ReplaceAll(english, "<div>"+label+"</div>", "<div>"+key+"</div>")
	}
	english = strings.ReplaceAll(english, "名召唤师", " ranked players")
	want, skipped, err := parseOPGGLadderPage(body, nil)
	if err != nil || skipped != 0 {
		t.Fatalf("original fixture: skipped=%d err=%v", skipped, err)
	}
	got, skipped, err := parseOPGGLadderPage([]byte(english), nil)
	if err != nil || skipped != 0 || !reflect.DeepEqual(got, want) {
		t.Fatalf("translation changed the snapshot: rows=%d skipped=%d err=%v", len(got.Rows), skipped, err)
	}

	// Check both badge attributes, including disagreement with visible text.
	badges := mapLadderFixtureRows(english, func(i int, row string) string {
		switch i {
		case 0:
			return replaceLadderFixtureCell(row, 2, `<img src="https://opgg-static.akamaized.net/images/medals_mini/rank-challenger.png?v=1"/><div>Grandmaster</div>`)
		case 1:
			return replaceLadderFixtureCell(row, 2, `<img alt="Challenger rank"/><div>untranslated</div>`)
		case 2:
			return replaceLadderFixtureCell(row, 2, `<img alt="Diamond"/><div>untranslated</div><span>D1</span>`)
		default:
			return row
		}
	})
	got, skipped, err = parseOPGGLadderPage([]byte(badges), nil)
	want.Rows[2].Tier, want.Rows[2].Division = "diamond", 1
	if err != nil || skipped != 0 || !reflect.DeepEqual(got, want) {
		t.Fatalf("badge tier or division was not applied: rows=%d skipped=%d err=%v", len(got.Rows), skipped, err)
	}
}

func TestLadderValidationKeepsGoodCache(t *testing.T) {
	body := string(ladderFixture(t))
	editFirst := func(n int, update func(int, string) string) string {
		return mapLadderFixtureRows(body, func(i int, row string) string {
			if i < n {
				return update(i, row)
			}
			return row
		})
	}
	cell := func(n, column int, html string) string {
		return editFirst(n, func(_ int, row string) string { return replaceLadderFixtureCell(row, column, html) })
	}
	tests := []struct {
		name, reason, tier, broken string
		rows, skipped, fieldRows   int
	}{
		{"missing full-page rows", "row_count", "", editFirst(11, func(_ int, _ string) string { return "" }), 89, 0, 0},
		{"missing final-page rows", "row_count", "", strings.Replace(editFirst(6, func(_ int, _ string) string { return "" }), "3,288,698", "100", 1), 94, 0, 0},
		{"rank order", "rank_order", "", cell(1, 0, "3"), 100, 0, 0},
		{"global rank outside page", "rank_order", "", mapLadderFixtureRows(body, func(i int, row string) string {
			if i == 99 {
				return replaceLadderFixtureCell(row, 0, "101")
			}
			return row
		}), 100, 0, 0},
		{"filtered rank far from page start", "rank_order", "challenger", mapLadderFixtureRows(body, func(i int, row string) string {
			return replaceLadderFixtureCell(row, 0, strconv.Itoa(i+7))
		}), 100, 0, 0},
		{"invalid LP", "field_sanity", "", cell(6, 3, "5,001"), 94, 6, 6},
		{"invalid level", "field_sanity", "", cell(6, 5, "5,001"), 94, 6, 6},
		{"invalid win rate", "field_sanity", "", cell(6, 6, "60胜40败10%"), 94, 6, 6},
		{"win rate above one hundred", "field_sanity", "", cell(6, 6, "100胜0败101%"), 94, 6, 6},
		{"invalid top tier division", "field_sanity", "", cell(6, 2, "<div>王者 1</div>"), 94, 6, 6},
		{"missing lower tier division", "field_sanity", "", cell(6, 2, "<div>钻石</div>"), 94, 6, 6},
		{"too many skipped", "skipped_ratio", "", cell(6, 0, "broken"), 94, 6, 0},
		{"missing footer", "footer", "", body[:strings.Index(body, "</tbody>")+len("</tbody>")], 100, 0, 0},
		{"missing cutoff", "cutoff", "", strings.Replace(body, "700 名召唤师", "", 1), 100, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			root := t.TempDir()
			a := &app{storage: &localStore{root: root}}
			p := newLadderProvider(a)
			a.ladder = p
			key := ladderKey{page: 1, tier: tt.tier}
			now := time.Now().UTC()
			p.now = func() time.Time { return now }
			var events []map[string]any
			p.observe = func(event map[string]any) { events = append(events, event) }
			var calls atomic.Int32
			p.fetch = func(context.Context, ladderKey, string) ([]byte, error) {
				if calls.Add(1) == 1 {
					return []byte(body), nil
				}
				return []byte(tt.broken), nil
			}
			old, err := p.load(ctx, key, false, false)
			if err != nil || len(old.Rows) != 100 {
				t.Fatalf("seed good page: rows=%d err=%v", len(old.Rows), err)
			}
			file := filepath.Join(root, key.file())
			diskBefore, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}

			now = now.Add(ladderFreshFor)
			requestPage := func(refresh string) {
				t.Helper()
				rec := httptest.NewRecorder()
				target := "/api/ladder?page=1&tier=" + tt.tier + refresh
				a.handleLadder(rec, httptest.NewRequest(http.MethodGet, target, nil).WithContext(ctx))
				var got ladderDisplayPage
				if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil ||
					!got.FetchedAt.Equal(old.FetchedAt) || len(got.Rows) != 100 ||
					!got.Stale || got.StaleReason != "upstream_unavailable" {
					t.Fatalf("did not serve the old page as stale: status=%d rows=%d stale=%v reason=%q", rec.Code, len(got.Rows), got.Stale, got.StaleReason)
				}
			}
			requestPage("&refresh=1")
			p.mu.Lock()
			cached, failure := p.pages[key], p.failures[key]
			p.mu.Unlock()
			if !errors.Is(failure.err, ladderParseRejection(tt.reason)) || !reflect.DeepEqual(cached, old) {
				t.Fatalf("failure=%v, memory preserved=%v", failure.err, reflect.DeepEqual(cached, old))
			}
			diskAfter, err := os.ReadFile(file)
			if err != nil || string(diskAfter) != string(diskBefore) {
				t.Fatalf("old disk cache changed: %v", err)
			}
			requestPage("")
			p.mu.Lock()
			inFlight := p.flights[key] != nil
			p.mu.Unlock()
			if inFlight || calls.Load() != 2 {
				t.Fatalf("backoff started another request: calls=%d in-flight=%v", calls.Load(), inFlight)
			}
			want := map[string]any{"event": "ladder_parse_rejected", "page": 1, "tier": tt.tier,
				"reason": tt.reason, "rows": tt.rows, "skipped": tt.skipped, "bytes": len(tt.broken)}
			rejections := 0
			for _, event := range events {
				if event["event"] == "ladder_parse_rejected" {
					rejections++
					if !reflect.DeepEqual(event, want) {
						t.Fatalf("rejection diagnostic=%v want=%v", event, want)
					}
				}
				if event["event"] == "ladder_fetch" && event["outcome"] == "failed" {
					if event["parse_version"] != 1 || event["rows_skipped"] != tt.skipped || event["field_invalid_rows"] != tt.fieldRows {
						t.Fatalf("parse drift diagnostic=%v", event)
					}
				}
			}
			if rejections != 1 {
				t.Fatalf("got %d rejection diagnostics, want one", rejections)
			}
		})
	}
}

func TestLadderValidationAcceptsFixtureAndSmallGaps(t *testing.T) {
	body := string(ladderFixture(t))
	drop := func(n int) string {
		return mapLadderFixtureRows(body, func(i int, row string) string {
			if i >= 78 && i < 78+n {
				return ""
			}
			return row
		})
	}
	outliers := mapLadderFixtureRows(body, func(i int, row string) string {
		if i >= 78 && i < 83 {
			return replaceLadderFixtureCell(row, 3, "5,001")
		}
		return row
	})
	gm := strings.ReplaceAll(drop(1), "<div>王者</div><span>C</span>", "<div>宗师</div><span>GM</span>")
	nearby := mapLadderFixtureRows(body, func(i int, row string) string {
		return replaceLadderFixtureCell(row, 0, strconv.Itoa(6+i))
	})
	for _, tt := range []struct {
		name, tier, html string
		rows, fieldRows  int
	}{
		{"original", "", body, 100, 0},
		{"global with one missing", "", drop(1), 99, 0},
		{"grandmaster with one missing", "grandmaster", gm, 99, 0},
		{"interior lower bound", "", drop(10), 90, 0},
		{"last-page tolerance", "", strings.Replace(drop(5), "3,288,698", "100", 1), 95, 0},
		{"isolated field outliers at five percent", "", outliers, 95, 5},
		{"filtered starts five ranks late", "challenger", nearby, 100, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := newLadderProvider(new(app))
			p.fetch = func(context.Context, ladderKey, string) ([]byte, error) { return []byte(tt.html), nil }
			var report map[string]any
			p.observe = func(event map[string]any) { report = event }
			got, err := p.fetchPage(context.Background(), ladderKey{page: 1, tier: tt.tier}, "")
			if err != nil || len(got.Rows) != tt.rows {
				t.Fatalf("valid page: rows=%d err=%v", len(got.Rows), err)
			}
			if report["event"] != "ladder_fetch" || report["parse_version"] != 1 || report["rows"] != tt.rows ||
				report["rows_skipped"] != tt.fieldRows || report["field_invalid_rows"] != tt.fieldRows {
				t.Fatalf("successful parse diagnostic: %v", report)
			}
		})
	}
}

func TestLadderStaleAPIAfterBackgroundFailure(t *testing.T) {
	body := ladderFixture(t)
	a := new(app)
	p := newLadderProvider(a)
	a.ladder = p
	var clock atomic.Int64
	now := time.Now().UTC()
	clock.Store(now.UnixNano())
	p.now = func() time.Time { return time.Unix(0, clock.Load()) }
	key := ladderKey{page: 1}
	const target = "/api/ladder?page=1"
	updated := make(chan ladderKey, 2)
	p.updated = func(key ladderKey) { updated <- key }
	var calls atomic.Int32
	gate := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p.fetch = func(ctx context.Context, _ ladderKey, _ string) ([]byte, error) {
		if calls.Add(1) == 2 {
			select {
			case <-gate:
				return nil, errors.New("upstream unavailable")
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return body, nil
	}
	request := func(path string, stale bool, fetchedAt time.Time) {
		t.Helper()
		rec := httptest.NewRecorder()
		a.handleLadder(rec, httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx))
		var page ladderDisplayPage
		reason := ""
		if stale {
			reason = "upstream_unavailable"
		}
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &page) != nil || !page.FetchedAt.Equal(fetchedAt) ||
			page.Stale != stale || page.StaleReason != reason || len(page.Rows) != 100 {
			t.Fatalf("response: status=%d stamp=%v stale=%v reason=%q rows=%d", rec.Code, page.FetchedAt, page.Stale, page.StaleReason, len(page.Rows))
		}
	}
	expectUpdate := func() {
		t.Helper()
		select {
		case got := <-updated:
			if got != key {
				t.Fatalf("notified wrong page: %+v", got)
			}
		case <-ctx.Done():
			t.Fatal("stale status change did not notify the frontend")
		}
	}
	request(target, false, now)
	later := now.Add(ladderFreshFor)
	clock.Store(later.UnixNano())
	request(target, false, now)
	close(gate)
	expectUpdate()
	request(target, true, now)
	if calls.Load() != 2 {
		t.Fatal("reading stale status bypassed backoff")
	}
	request(target+"&refresh=1", false, later)
	expectUpdate()
	request(target, false, later)
}

func TestLadderLiveOPGGShape(t *testing.T) {
	if os.Getenv("DEEP_LEGENDS_LIVE_OPGG") != "1" {
		t.Skip("set DEEP_LEGENDS_LIVE_OPGG=1 for a manual upstream shape check")
	}
	key := ladderKey{page: 1}
	body, err := fetchOPGGLadderPage(context.Background(), newChampionProvider(), key, "")
	if err != nil {
		t.Fatal("rows=0 reason=upstream_unavailable")
	}
	page, skipped, err := parseOPGGLadderPage(body, nil)
	if err != nil {
		t.Fatalf("rows=%d reason=footer", len(page.Rows))
	}
	if err := validateLadderPage(key, &page, skipped); err != nil {
		t.Fatalf("rows=%d reason=%s", len(page.Rows), string(err.(ladderParseRejection)))
	}
	t.Logf("rows=%d reason=ok", len(page.Rows))
}
