package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func proSegmentTime(value string) time.Time {
	at, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return at
}

func proSegmentBuild(base proOPGGBuild, id, serie, at string, championID int) proOPGGBuild {
	base.ID, base.Game.ID = id, id
	base.ChampionID = strconv.Itoa(championID)
	base.Game.Match.Tournament.Serie.ID = serie
	base.Game.BeginAt = proSegmentTime(at)
	return base
}

type proSegmentRequest struct {
	LeagueID, ChampionID string
	Page, Limit          int
}

func proSegmentFixtureTransport(t *testing.T, feed map[string][]proOPGGBuild, mu *sync.Mutex, requests *[]proSegmentRequest, failHead *string) http.RoundTripper {
	t.Helper()
	return championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		var request struct {
			Query     string
			Variables proSegmentRequest
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			return nil, err
		}
		v := request.Variables
		if !strings.Contains(request.Query, "serie{id year season}") ||
			(v.ChampionID == "" && strings.Contains(request.Query, "championId")) {
			t.Error("missing series metadata or champion-dependent discovery", v)
		}
		if v.LeagueID == "167" {
			t.Error("KeSPA must not be queried")
		}
		mu.Lock()
		*requests = append(*requests, v)
		mu.Unlock()
		if v.ChampionID == "" && v.Limit == 1 && v.LeagueID == *failHead {
			return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		rows := []proOPGGBuild{}
		for _, row := range feed[v.LeagueID] {
			if v.ChampionID == "" || row.ChampionID == v.ChampionID {
				rows = append(rows, row)
			}
		}
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].Game.BeginAt.After(rows[j].Game.BeginAt) })
		start := v.Page * v.Limit
		if start >= len(rows) {
			rows = rows[:0]
		} else {
			rows = rows[start:min(start+v.Limit, len(rows))]
		}
		return proOPGGReply(rows), nil
	})
}

func TestProOPGGCurrentAndPreviousSegments(t *testing.T) {
	base := proOPGGFixture(t)[0]
	build := func(id, serie, day string, championID int) proOPGGBuild {
		return proSegmentBuild(base, id, serie, day+"T12:00:00Z", championID)
	}
	worlds := []proOPGGBuild{
		build("11", "741", "2026-11-14", 103), build("12", "741", "2026-10-21", 103),
		build("13", "741", "2026-10-15", 103), build("14", "600", "2025-11-09", 103),
	}
	for i := range worlds[:3] {
		// The same serie spans playoffs, groups and play-ins.
		worlds[i].Game.Match.Tournament.ID = strconv.Itoa(1443 - i)
	}
	lpl := []proOPGGBuild{build("21", "736", "2026-09-19", 103), build("22", "735", "2026-06-01", 103)}
	lck := []proOPGGBuild{build("31", "716", "2026-09-13", 103), build("32", "716", "2026-06-10", 103)}
	msi := []proOPGGBuild{build("41", "730", "2026-07-12", 103), build("42", "730", "2026-06-29", 103)}
	ewc := []proOPGGBuild{build("51", "733", "2026-07-19", 103), build("52", "733", "2026-07-14", 103)}
	tests := []struct {
		name     string
		feed     map[string][]proOPGGBuild
		champion int
		want     []string
		queries  []string
	}{
		{"Worlds off-season includes play-ins and groups", map[string][]proOPGGBuild{"96": worlds, "98": lpl, "99": lck, "100": msi, "166": ewc}, 103, []string{"11", "12", "13"}, []string{"96"}},
		{"only LPL has begun a new split", map[string][]proOPGGBuild{"96": worlds, "98": {build("23", "800", "2026-11-23", 103), lpl[0]}, "99": lck}, 103, []string{"23"}, []string{"98"}},
		{"LCK same serie straddles MSI", map[string][]proOPGGBuild{"100": msi, "99": lck, "98": {lpl[1]}}, 103, []string{"31"}, []string{"99"}},
		{"international underway includes EWC but not later KeSPA", map[string][]proOPGGBuild{"100": msi, "166": ewc[1:], "98": {lpl[1]}, "167": {build("61", "888", "2026-11-26", 103)}}, 103, []string{"52"}, []string{"166"}},
		{"domestic no champion falls back to international", map[string][]proOPGGBuild{"96": worlds, "98": {build("23", "800", "2026-11-23", 69), lpl[0]}}, 103, []string{"11", "12", "13"}, []string{"96", "98"}},
		{"international no champion falls back to domestic after last international", map[string][]proOPGGBuild{"96": {build("11", "741", "2026-11-14", 69), build("13", "741", "2026-10-15", 69)}, "98": lpl, "99": lck, "100": msi, "166": ewc}, 103, []string{"21", "31"}, []string{"96", "98", "99"}},
		{"both segments empty", map[string][]proOPGGBuild{"96": worlds, "98": lpl, "99": lck, "100": msi, "166": ewc}, 69, nil, []string{"96", "98", "99"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newProRuneProvider(proTestProvider(), nil, nil)
			p.now = func() time.Time { return proSegmentTime("2026-11-27T01:00:00Z") }
			var mu sync.Mutex
			var requests []proSegmentRequest
			var failHead string
			p.provider.client = &http.Client{Transport: proSegmentFixtureTransport(t, tt.feed, &mu, &requests, &failHead)}
			got, err := p.opggRecommend(t.Context(), tt.champion, "mid", false)
			var keys []string
			for _, row := range got.Pros {
				keys = append(keys, strings.TrimPrefix(row.Key, "pro-opgg-"))
			}
			if err != nil || !reflect.DeepEqual(keys, tt.want) || got.TotalGames != len(tt.want) {
				t.Fatalf("got %v (%d games), want %v: %v", keys, got.TotalGames, tt.want, err)
			}
			if len(tt.want) == 0 && got.Reason != "no-sample" {
				t.Fatalf("empty segments: %s", got.Reason)
			}
			var heads, queries []string
			for _, request := range requests {
				switch {
				case request.ChampionID != "":
					queries = append(queries, request.LeagueID)
				case request.Limit == 1:
					if request.Page != 0 {
						t.Fatal("segment discovery did not request the first result")
					}
					heads = append(heads, request.LeagueID)
				}
			}
			sort.Strings(heads)
			sort.Strings(queries)
			if !reflect.DeepEqual(heads, []string{"100", "141", "166", "96", "98", "99"}) || !reflect.DeepEqual(queries, tt.queries) {
				t.Fatalf("head requests %v, champion requests %v", heads, queries)
			}
		})
	}
}

func TestProOPGGSegmentCacheFailureAndTransition(t *testing.T) {
	base := proOPGGFixture(t)[0]
	worlds := proSegmentBuild(base, "11", "741", "2026-11-14T12:00:00Z", 103)
	lpl := proSegmentBuild(base, "21", "800", "2026-11-27T01:15:00Z", 103)
	feed := map[string][]proOPGGBuild{"96": {worlds}}
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	p := newProRuneProvider(proTestProvider(), store, nil)
	now := proSegmentTime("2026-11-27T01:00:00Z")
	p.now = func() time.Time { return now }
	p.writeDisk("opgg-builds-103", proOPGGCache{ChampionID: 103, ReadAt: now, Attempted: now, Rows: []proOPGGBuild{lpl}})
	var mu sync.Mutex
	var requests []proSegmentRequest
	var failHead string
	p.provider.client = &http.Client{Transport: proSegmentFixtureTransport(t, feed, &mu, &requests, &failHead)}
	assertRow := func(id string, stale bool) {
		t.Helper()
		got, err := p.opggRecommend(t.Context(), 103, "mid", false)
		if err != nil || len(got.Pros) != 1 || got.Pros[0].Key != "pro-opgg-"+id || got.Stale != stale {
			t.Fatalf("expected %s stale=%v: %+v, %v", id, stale, got, err)
		}
	}
	assertRow("11", false)
	originalCacheKey := p.opggBuilds[103].Segment
	now = now.Add(29 * time.Minute)
	feed["98"] = []proOPGGBuild{lpl}
	assertRow("11", false)
	if len(requests) != 8 {
		t.Fatal("metadata refreshed before 30 minutes", requests)
	}
	now = now.Add(time.Minute)
	failHead = "99"
	assertRow("11", true)
	n := len(requests)
	assertRow("11", true)
	if len(requests) != n {
		t.Fatal("failed metadata discovery not cached")
	}
	now = now.Add(proOPGGSegmentTTL)
	failHead = ""
	assertRow("21", false)
	if next := p.opggBuilds[103].Segment; next == originalCacheKey || !p.readDisk(next, &proOPGGCache{}) {
		t.Fatal("changed segment reused the old disk key")
	}
}

func TestProRuneLiveOPGGSegments(t *testing.T) {
	if os.Getenv("DEEP_LEGENDS_LIVE_OPGG") != "1" {
		t.Skip("set DEEP_LEGENDS_LIVE_OPGG=1 for a manual upstream check")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	cp := newChampionProvider()
	cp.championMeta = map[int]championMetadata{
		799: {ID: 799, Slug: "ambessa", NameZH: "安蓓萨"}, 21: {ID: 21, Slug: "missfortune", NameZH: "厄运小姐"}, 51: {ID: 51, Slug: "caitlyn", NameZH: "凯特琳"},
	}
	now := time.Now()
	p := newProRuneProvider(cp, nil, nil)
	// The test uses an isolated provider with no disk store.
	state, err := p.opggSegmentState(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	period := proCurrentPeriod(state.Heads)
	if len(period.Scopes) == 0 {
		t.Fatal("OP.GG returned no active tournament")
	}
	for _, scope := range period.Scopes {
		games := map[string]bool{}
		var mu sync.Mutex
		err := p.walkOPGGSeries(ctx, scope.proRuneLeague, now, func(row proRuneScope, gameID string) bool {
			if row.Serie != scope.Serie || !row.Last.After(period.After) {
				return false
			}
			mu.Lock()
			games[gameID] = true
			mu.Unlock()
			return true
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("league=%s serie=%s games=%d", scope.Name, scope.Serie, len(games))
	}
	for _, id := range []int{799, 21, 51} {
		got, err := p.opggRecommend(ctx, id, "", false)
		if err != nil {
			t.Fatalf("champion=%d: %v (%s)", id, err, got.Reason)
		}
		t.Logf("champion=%d recommendations=%d eligible_games=%d", id, len(got.Pros), got.TotalGames)
	}
}

func TestProRuneOfficialSegmentsKeepWorldsAndPriorDomestic(t *testing.T) {
	const worlds = "98767975604431411"
	const ewc = "116838530616006090"
	const lpl = "98767991314006698"
	const lck = "98767991310872058"
	type event struct {
		serie, id, day string
	}
	fixtures := map[string][]event{
		worlds: {{"741", "11", "2026-11-14"}, {"741", "12", "2026-10-15"}, {"600", "13", "2025-11-09"}},
		ewc:    {{"733", "21", "2026-07-19"}},
		lpl:    {{"736", "31", "2026-09-19"}, {"735", "32", "2026-06-01"}},
		lck:    {{"716", "41", "2026-09-13"}, {"716", "42", "2026-06-10"}},
	}
	p := newProRuneProvider(proTestProvider(), nil, nil)
	p.provider.client = &http.Client{Transport: championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		events := fixtures[r.URL.Query().Get("leagueId")]
		tournaments := []map[string]string{}
		schedule := []map[string]any{}
		positions := map[string]int{}
		for _, event := range events {
			if pos, ok := positions[event.serie]; ok {
				tournaments[pos]["startDate"] = event.day
			} else {
				positions[event.serie] = len(tournaments)
				tournaments = append(tournaments, map[string]string{"id": event.serie, "startDate": event.day, "endDate": event.day})
			}
			schedule = append(schedule, map[string]any{
				"state": "completed", "startTime": event.day + "T12:00:00Z", "match": map[string]string{"id": event.id},
			})
		}
		wire := map[string]any{"schedule": map[string]any{"events": schedule}, "leagues": []any{map[string]any{"tournaments": tournaments}}}
		reply, err := json.Marshal(map[string]any{"data": wire})
		if err != nil {
			return nil, err
		}
		return proHTTPBody(reply), nil
	})}
	got, err := p.officialSegments(t.Context(), proSegmentTime("2026-11-27T12:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"11": false, "12": false, "31": true, "41": true}
	actual := map[string]bool{}
	for id, match := range got {
		actual[id] = match.previous
	}
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("official segments: %v, want %v", actual, want)
	}
}
