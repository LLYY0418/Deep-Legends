package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func proTestProvider() *championProvider {
	p := newChampionProvider()
	p.championMeta = map[int]championMetadata{69: {ID: 69, Key: "Cassiopeia", Slug: "cassiopeia", NameZH: "卡西奥佩娅"}, 103: {ID: 103, Key: "Ahri", Slug: "ahri", NameZH: "阿狸"}}
	p.championIDs = map[string]int{"cassiopeia": 69, "ahri": 103}
	return p
}
func proTestEvent() proRuneEvent {
	var e proRuneEvent
	_ = json.Unmarshal([]byte(`{"id":"123","league":{"id":"98767991310872058"},"match":{"teams":[{"id":"98767991853197861","code":"T1","result":{"gameWins":1}},{"id":"100205573495116443","code":"GEN","result":{"gameWins":0}}],"games":[{"id":"456","number":1,"state":"completed","teams":[{"id":"98767991853197861","side":"red"},{"id":"100205573495116443","side":"blue"}]}]}}`), &e)
	return e
}
func proTestGame() proRuneIndexGame {
	return proRuneIndexGame{ID: "456", MatchID: "123", LeagueID: "98767991310872058", Number: 1, Start: time.Now().UTC().Add(-time.Hour), Metadata: proGameMetadata{Patch: "16.17", Blue: proTeamMetadata{TeamID: "98767991853197861", Participants: []proParticipant{{ParticipantID: 3, PlayerID: "98767991747728851", SummonerName: "T1 Faker", ChampionID: "Cassiopeia", Role: "mid"}}}, Red: proTeamMetadata{TeamID: "100205573495116443", Participants: []proParticipant{{ParticipantID: 8, PlayerID: "999", SummonerName: "GEN Chovy", ChampionID: "Ahri", Role: "mid"}}}}}
}
func proTestEnd(g proRuneIndexGame) proWindow {
	return proWindow{Metadata: g.Metadata, Frames: []proFrame{{Timestamp: g.Start.Add(30 * time.Minute), State: "finished", Blue: proFrameTeam{Inhibitors: 1, TotalGold: 40000}, Red: proFrameTeam{TotalGold: 50000}}}}
}
func TestProRuneWhiteLists(t *testing.T) {
	leagues := map[string]string{"98767991314006698": "LPL", "98767991310872058": "LCK", "113464388705111224": "First Stand", "98767991325878492": "MSI", "98767975604431411": "Worlds"}
	teams := map[string]string{"99566404853854212": "BLG", "99566404848691211": "IG", "98767991853197861": "T1", "100205573496804586": "HLE", "100205573495116443": "GEN", "100725845018863243": "DK"}
	if !reflect.DeepEqual(proRuneLeagues, leagues) || !reflect.DeepEqual(proRuneTeams, teams) {
		t.Fatal("V1/V2 exact official ID whitelist changed")
	}
	e := proTestEvent()
	if !proEventAllowed(e) {
		t.Fatal("first-team match rejected")
	}
	for _, id := range []string{"98767991335774713", "98767991355908944"} {
		e.League.ID = id
		if proEventAllowed(e) {
			t.Fatal("V1 secondary league accepted", id)
		}
	}
	e = proTestEvent()
	e.Match.Teams[0].ID = "105550026570060790"
	e.Match.Teams[0].Code = "DK"
	e.Match.Teams[1].ID = "outside"
	e.Match.Teams[1].Code = "GEN"
	if proEventAllowed(e) {
		t.Fatal("V2 code collision accepted DK Challengers")
	}
}
func TestProRuneExactIdentityAndPersistentMapping(t *testing.T) {
	p := newProRuneProvider(proTestProvider(), &localStore{root: t.TempDir()}, nil)
	for _, name := range []string{"NotFaker", "T1 Faker2", "T1 Test MID1", "FakeFaker", "T1 Painter"} {
		if got := p.player("98767991853197861", proParticipant{SummonerName: name}); got != "" {
			t.Fatal("V2/M8 non-roster name accepted", name, got)
		}
	}
	if got := p.player("105550026570060790", proParticipant{SummonerName: "DK ShowMaker"}); got != "" {
		t.Fatal("academy player admitted")
	}
	for _, name := range []string{"T1 Faker", "T1Faker", "faker"} {
		if got := p.player("98767991853197861", proParticipant{PlayerID: "98767991747728851", SummonerName: name}); got != "Faker" {
			t.Fatal("missing exact identity", name)
		}
	}
	p.saveIdentities()
	q := newProRuneProvider(proTestProvider(), &localStore{root: filepath.Dir(p.root)}, nil)
	q.readDisk("identities", &q.identities)
	if got := q.player("98767991853197861", proParticipant{PlayerID: "98767991747728851", SummonerName: "renamed"}); got != "Faker" {
		t.Fatal("learned esports ID lost", got)
	}
	if got := q.player("100205573495116443", proParticipant{PlayerID: "98767991747728851", SummonerName: "GEN Chovy"}); got != "" {
		t.Fatal("ID changed reviewed team")
	}
}
func TestProRuneNewestAndShards(t *testing.T) {
	rows := []proRuneRow{}
	for i := 0; i < 8; i++ {
		g := proTestGame()
		g.Start = g.Start.Add(time.Duration(i) * time.Minute)
		rows = append(rows, proRuneRow{Game: g, Participant: g.Metadata.Blue.Participants[0]})
	}
	selected := proNewestRows(rows, "Cassiopeia", "mid")
	if len(selected) != 5 || !selected[0].Game.Start.Equal(rows[7].Game.Start) {
		t.Fatal("V5 newest first and max five violated")
	}
	shards, complete := proRuneShards([]int64{8112, 8143, 8137, 8106, 8401, 8444, 5008, 5011})
	if !complete || !reflect.DeepEqual(shards, []int64{5008, 5008, 5011}) {
		t.Fatal("unique deduplicated shards not restored", shards, complete)
	}
	shards, complete = proRuneShards([]int64{8112, 8143, 8137, 8106, 8401, 8444, 5011, 5008, 5005})
	if !complete || !reflect.DeepEqual(shards, []int64{5005, 5008, 5011}) {
		t.Fatal("unique shard slot assignment failed", shards)
	}
	shards, complete = proRuneShards([]int64{8112, 8143, 8137, 8106, 8401, 8444, 5008, 5008, 5011})
	if !complete || !reflect.DeepEqual(shards, []int64{5008, 5008, 5011}) {
		t.Fatal("identical permutations of explicit duplicate shards are not ambiguous", shards)
	}
}
func TestProRuneWinnerConstraints(t *testing.T) {
	e := proTestEvent()
	g := proTestGame()
	games := map[string]proRuneIndexGame{g.ID: g}
	ends := map[string]proWindow{g.ID: proTestEnd(g)}
	winners, ok := proSeriesWinners(e, games, ends)
	if !ok || winners[g.ID] != g.Metadata.Blue.TeamID {
		t.Fatal("V3/V4 gold/side used instead of livestats inhibitor rule")
	}
	end := ends[g.ID]
	end.Frames[0].Blue.Inhibitors = 1
	end.Frames[0].Red.Inhibitors = 1
	ends[g.ID] = end
	winners, ok = proSeriesWinners(e, games, ends)
	if !ok || winners[g.ID] != g.Metadata.Blue.TeamID {
		t.Fatal("unique remaining official score not solved")
	}
	e.Match.Teams[0].Result.GameWins = 0
	e.Match.Teams[1].Result.GameWins = 1
	ends[g.ID] = proTestEnd(g)
	if _, ok = proSeriesWinners(e, games, ends); ok {
		t.Fatal("V4 overcalled series must be rejected")
	}
}
func TestProRunePublicTransportAndAlignedSupplement(t *testing.T) {
	p := proTestProvider()
	g := proTestGame()
	jar, _ := cookiejar.New(nil)
	u, _ := url.Parse("https://feed.lolesports.com")
	jar.SetCookies(u, []*http.Cookie{{Name: "secret", Value: "private"}})
	var calls atomic.Int32
	p.client = &http.Client{Jar: jar, Transport: championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.Header.Get("Cookie") != "" || r.Header.Get("x-riot-token") != "" || r.Header.Get("Authorization") != "" || strings.Contains(strings.ToLower(r.URL.String()), "puuid") {
			t.Error("V8 credential leak", r.Header, r.URL)
		}
		if r.URL.Host == "feed.lolesports.com" && r.Header.Get("x-api-key") != "" {
			t.Error("feed received API key")
		}
		stamp := r.URL.Query().Get("startingTime")
		parsed, err := time.Parse(time.RFC3339, stamp)
		status := 200
		body := ""
		if err != nil || parsed.Nanosecond() != 0 || parsed.Second()%10 != 0 {
			status = 400
			body = `{"error":"unaligned timestamp"}`
		} else if strings.Contains(r.URL.Path, "/window/") {
			data, _ := json.Marshal(proTestEnd(g))
			body = string(data)
		} else {
			body = `{"frames":[{"rfc460Timestamp":"` + proStartingTime(g.Start.Add(30*time.Minute)) + `","participants":[{"participantId":3,"items":[3364,2031,6692],"perkMetadata":{"styleId":8100,"subStyleId":8400,"perks":[8112,8143,8137,8106,8401,8444,5008,5011]}}]}]}`
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	source := newProRuneProvider(p, &localStore{root: t.TempDir()}, nil)
	if _, err := source.supplement(context.Background(), g); err != nil {
		t.Fatal("M5 supplement failed (HTTP 400 expected for unaligned mutation)", err)
	}
	if calls.Load() != 2 {
		t.Fatal("two-stage requests", calls.Load())
	}
	start := time.Now()
	if _, err := source.supplement(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || time.Since(start) >= 100*time.Millisecond {
		t.Fatal("V7 cached fill exceeded 100ms or fetched again")
	}
	reloaded := newProRuneProvider(p, &localStore{root: filepath.Dir(source.root)}, nil)
	if _, err := reloaded.supplement(context.Background(), g); err != nil || calls.Load() != 2 {
		t.Fatal("immutable disk cache not reused", err)
	}
	// An initial-frame disk entry must not masquerade as final equipment.
	corrupt := reloaded.details[g.ID]
	corrupt.Frames[0].Timestamp = g.Start
	reloaded.writeDisk("details-"+g.ID, corrupt)
	fresh := newProRuneProvider(p, &localStore{root: filepath.Dir(source.root)}, nil)
	if _, err := fresh.supplement(context.Background(), g); err != nil || calls.Load() != 3 {
		t.Fatal("nonterminal disk details were not refetched", err, calls.Load())
	}
}
func TestProRuneColdIndexNonblockingAndConcurrency(t *testing.T) {
	provider := proTestProvider()
	release := make(chan struct{})
	var active, peak atomic.Int32
	provider.client = &http.Client{Transport: championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for prev := peak.Load(); n > prev && !peak.CompareAndSwap(prev, n); prev = peak.Load() {
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":{"schedule":{"pages":{},"events":[]}}}`)), Header: http.Header{}}, nil
	})}
	done := make(chan struct{}, 4)
	p := newProRuneProvider(provider, nil, func() { done <- struct{}{} })
	start := time.Now()
	p.kick(false)
	res := p.recommend(context.Background(), 69, "mid")
	if time.Since(start) > 100*time.Millisecond || !res.Preparing || res.Reason != "preparing" {
		t.Fatal("V7 cold index blocked first response", res)
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); p.kick(false) }()
	}
	wg.Wait()
	close(release)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("index stuck")
	}
	if peak.Load() > 4 {
		t.Fatal("network concurrency exceeded 4")
	}
}
func TestProRuneRecommendationUnknownRecordAndOffline(t *testing.T) {
	p := newProRuneProvider(proTestProvider(), nil, nil)
	g := proTestGame()
	e := proTestEvent()
	p.snapshot = proRuneSnapshot{Games: map[string]proRuneIndexGame{g.ID: g}, Events: map[string]proRuneEvent{e.ID: e}, ReadAt: time.Now()}
	var d proDetails
	_ = json.Unmarshal([]byte(`{"frames":[{"participants":[{"participantId":3,"items":[3364],"perkMetadata":{"styleId":8100,"subStyleId":8400,"perks":[8112,8143,8137,8106,8401,8444,5008,5011]}}]}]}`), &d)
	p.details[g.ID] = d
	res := p.recommend(context.Background(), 69, "mid")
	if len(res.Pros) != 1 {
		t.Fatal("missing recommendation", res)
	}
	r := res.Pros[0]
	if *r.WinKnown || r.Win != nil || r.Result != "" || r.RecordGames != 0 || !*r.SelectedComplete || r.OpponentPlayerName != "GEN Chovy" || r.OpponentChampionID != 103 || r.EventLabel == "" {
		t.Fatal("V6/V9 unknown improperly counted/rendered", r)
	}
	data, _ := json.Marshal(r)
	if strings.Contains(string(data), `"win":`) {
		t.Fatal("unknown emitted win")
	}
	p.ends[g.ID] = proTestEnd(g)
	res = p.recommend(context.Background(), 69, "mid")
	if res.Pros[0].RecordGames != 1 || res.Pros[0].RecordWins != 1 || res.Pros[0].Stats.WinRate != nil {
		t.Fatal("V9 record sample")
	}
	p.reason = "network-unavailable"
	res = p.recommend(context.Background(), 69, "mid")
	if !res.Stale || res.ReadAt.IsZero() || res.Pros[0].CacheReadAt == "" {
		t.Fatal("V10 stale cache not disclosed")
	}
}

type proCapturedSeries struct {
	Event   proRuneEvent         `json:"event"`
	Windows map[string]proWindow `json:"windows"`
	Ends    map[string]proWindow `json:"ends"`
}

func proCaptured(t *testing.T) []proCapturedSeries {
	t.Helper()
	data, err := os.ReadFile("testdata/r75/official-series.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []proCapturedSeries
	if err = json.Unmarshal(data, &rows); err != nil {
		t.Fatal(err)
	}
	return rows
}
func TestProRuneCapturedAllSeries(t *testing.T) {
	total, unknown, missing, rejected := 0, 0, 0, 0
	for _, series := range proCaptured(t) {
		games := map[string]proRuneIndexGame{}
		for _, g := range series.Event.Match.Games {
			if g.State != "completed" {
				continue
			}
			w, ok := series.Windows[g.ID]
			if !ok {
				missing++
				continue
			}
			games[g.ID] = proRuneIndexGame{ID: g.ID, MatchID: series.Event.ID, LeagueID: series.Event.League.ID, Number: g.Number, Start: w.Frames[0].Timestamp, Metadata: w.Metadata}
		}
		winners, ok := proSeriesWinners(series.Event, games, series.Ends)
		if !ok {
			rejected++
			t.Errorf("V4 rejected captured series %s", series.Event.ID)
			continue
		}
		counts := map[string]int{}
		for _, winner := range winners {
			counts[winner]++
		}
		for _, team := range series.Event.Match.Teams {
			if counts[team.ID] > team.Result.GameWins {
				t.Errorf("V4 overcall match %s", series.Event.ID)
			}
		}
		for id := range games {
			total++
			if winners[id] == "" {
				unknown++
			}
		}
		if series.Event.ID == "117030752644841619" {
			if winners["117030752644841620"] != "" || winners["117030752644841621"] != "" {
				t.Error("HLE-GEN games 1/2 must remain unknown")
			}
		}
		if series.Event.ID == "117155436343202178" && winners["117155436343202181"] != "99566404848691211" {
			t.Error("WE-IG game3 unique score constraint failed")
		}
	}
	t.Logf("R75 official capture: indexed=%d unknown=%d missing=%d rejected=%d unknown-rate=%.2f%%", total, unknown, missing, rejected, 100*float64(unknown)/float64(total))
}
func TestProRuneCapturedLivestatsSides(t *testing.T) {
	for _, series := range proCaptured(t) {
		if series.Event.ID != "117155436343202172" {
			continue
		}
		for _, g := range series.Event.Match.Games {
			if g.State != "completed" {
				continue
			}
			w := series.Windows[g.ID]
			provider := proTestProvider()
			provider.client = &http.Client{Transport: championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				data, _ := json.Marshal(w)
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data)))}, nil
			})}
			p := newProRuneProvider(provider, nil, nil)
			row, err := p.indexGame(context.Background(), series.Event, g)
			if err != nil {
				t.Fatal(err)
			}
			if row.Metadata.Blue.TeamID != w.Metadata.Blue.TeamID || row.Metadata.Red.TeamID != w.Metadata.Red.TeamID {
				t.Fatal("V3 getEventDetails.side used instead of livestats")
			}
			if got := proInhibitorWinner(row, series.Ends[g.ID]); got != "99566404853854212" {
				t.Fatal("V3 BLG 3-0 AL corrupted by getEventDetails.side", g.ID, got)
			}
		}
		return
	}
	t.Fatal("missing BLG-AL fixture")
}
func TestProRuneHandlerValidation(t *testing.T) {
	a := &app{}
	for _, query := range []string{"championId=0", "championId=69&position=invalid"} {
		w := httptest.NewRecorder()
		a.handleGameplayProRunes(w, httptest.NewRequest("GET", "/api/gameplay/pro-runes?"+query, nil))
		if w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
}

func TestProRuneOpponentCatalogFailureIsBounded(t *testing.T) {
	cp := proTestProvider()
	var calls atomic.Int32
	cp.client = &http.Client{Transport: championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`[]`))}, nil
	})}
	p := newProRuneProvider(cp, nil, nil)
	rows := []proRuneRow{{Opponent: proParticipant{ChampionID: "Ahri"}}, {Opponent: proParticipant{ChampionID: "MissingA"}}, {Opponent: proParticipant{ChampionID: "MissingB"}}}
	known, err := p.opponentMetadata(context.Background(), rows)
	if err == nil || calls.Load() != 1 || known["ahri"].ID != 103 {
		t.Fatalf("catalog failure repeated or lost cached opponent: calls=%d known=%v err=%v", calls.Load(), known, err)
	}
}

func TestProRuneCacheWriteFailureRemainsVisible(t *testing.T) {
	p := newProRuneProvider(proTestProvider(), nil, nil)
	p.root = filepath.Join(t.TempDir(), "file-not-dir")
	if err := os.WriteFile(p.root, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	p.writeDisk("index-456", proTestGame())
	// A successful network refresh may clear its own reason, not a disk failure.
	p.reason = ""
	p.snapshot.ReadAt = time.Now()
	res := p.recommend(context.Background(), 69, "mid")
	if !res.Stale || res.Reason != "cache-write-failed" {
		t.Fatal("cache failure hidden", res)
	}
}

// MG7: exercise the network metadata guard itself, without a disk-cache shortcut.
func TestProRuneIndexGameRejectsLivestatsTeamMismatch(t *testing.T) {
	e := proTestEvent()
	a, b := e.Match.Teams[0].ID, e.Match.Teams[1].ID
	for _, tc := range []struct {
		name, blue, red string
		valid           bool
	}{
		{"valid", a, b, true}, {"valid reversed", b, a, true},
		{"unrelated blue", "99999999999999999", b, false},
		{"unrelated red", a, "99999999999999999", false},
		{"same A", a, a, false}, {"same B", b, b, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := proTestEnd(proTestGame())
			w.Metadata.Blue.TeamID, w.Metadata.Red.TeamID = tc.blue, tc.red
			body, _ := json.Marshal(w)
			provider := proTestProvider()
			provider.client.Transport = championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}}, nil
			})
			p := newProRuneProvider(provider, nil, nil)
			_, err := p.indexGame(context.Background(), e, e.Match.Games[0])
			if tc.valid {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || err.Error() != "livestats team mismatch" {
				t.Fatalf("want livestats team mismatch, got %v", err)
			}
		})
	}
}

// Count actual refresh results, not a hand-built response. Schedule/event
// failures have unknown game cardinality and must not become fictional games.
func TestProRunePartialFailureCounts(t *testing.T) {
	for _, tc := range []struct{ total, failed int }{{99, 1}, {30, 10}, {10, 1}, {0, 0}} {
		t.Run(fmt.Sprintf("%d-of-%d", tc.failed, tc.total), func(t *testing.T) {
			now := time.Now().UTC()
			e := proTestEvent()
			e.Match.Games = nil
			e.Match.Teams[0].Result.GameWins = tc.total
			failIDs := map[string]bool{}
			for i := 0; i < tc.total; i++ {
				id := strconv.Itoa(1000 + i)
				e.Match.Games = append(e.Match.Games, proEventGame{ID: id, Number: i + 1, State: "completed"})
				if i < tc.failed {
					failIDs[id] = true
				}
			}
			provider := proTestProvider()
			provider.client.Transport = championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				var data any
				switch {
				case strings.HasSuffix(r.URL.Path, "getSchedule"):
					data = map[string]any{"data": map[string]any{"schedule": map[string]any{"events": []any{map[string]any{"startTime": now.Add(-time.Hour), "state": "completed", "match": map[string]string{"id": e.ID}}}}}}
				case strings.HasSuffix(r.URL.Path, "getEventDetails"):
					data = map[string]any{"data": map[string]any{"event": e}}
				case strings.Contains(r.URL.Path, "/window/"):
					if failIDs[path.Base(r.URL.Path)] {
						return nil, errors.New("fixture unavailable")
					}
					data = proTestEnd(proTestGame())
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
					return nil, errors.New("unexpected request")
				}
				body, _ := json.Marshal(data)
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}}, nil
			})
			p := newProRuneProvider(provider, nil, nil)
			p.refresh(context.Background(), now)
			// Successful detail cache keeps the public recommendations available.
			var d proDetails
			_ = json.Unmarshal([]byte(`{"frames":[{"participants":[{"participantId":3,"items":[3364],"perkMetadata":{"styleId":8100,"subStyleId":8400,"perks":[8112,8143,8137,8106,8401,8444,5008,5011]}}]}]}`), &d)
			for id := range p.snapshot.Games {
				p.details[id] = d
			}
			res := p.recommend(context.Background(), 69, "mid")
			if tc.total > tc.failed && len(res.Pros) == 0 {
				t.Fatal("successful recommendations lost")
			}
			if res.TotalGames != tc.total || res.Failed != tc.failed || res.Stale || !res.ReadAt.Equal(now) {
				t.Fatalf("wrong refresh coverage: %+v", res)
			}
			body, _ := json.Marshal(res)
			var wire map[string]any
			_ = json.Unmarshal(body, &wire)
			if wire["totalGames"] != float64(tc.total) || wire["failedGames"] != float64(tc.failed) {
				t.Fatal(string(body))
			}
			// A failed schedule cannot claim a complete fresh source or fabricate a game count.
			provider.client.Transport = championRoundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, errors.New("offline") })
			p.refresh(context.Background(), now.Add(time.Minute))
			res = p.recommend(context.Background(), 69, "mid")
			if !res.Stale || res.Failed != 0 || res.TotalGames != tc.total-tc.failed || !res.ReadAt.Equal(now) {
				t.Fatalf("global failure: %+v", res)
			}
		})
	}
}

func TestProRuneFailureCountsDeduplicateDetailAndTerminal(t *testing.T) {
	p := newProRuneProvider(proTestProvider(), nil, nil)
	g := proTestGame()
	e := proTestEvent()
	p.snapshot = proRuneSnapshot{Games: map[string]proRuneIndexGame{g.ID: g}, Events: map[string]proRuneEvent{e.ID: e}, ReadAt: time.Now()}
	p.failedGameIDs = map[string]bool{g.ID: true, "789": true}
	p.reason = "network-unavailable"
	p.provider.client.Transport = championRoundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, errors.New("offline") })
	for i := 0; i < 2; i++ {
		res := p.recommend(context.Background(), 69, "mid")
		if res.Failed != 2 || res.TotalGames != 2 || res.Stale {
			t.Fatalf("duplicate game or falsely stale: %+v", res)
		}
	}
}

func TestProRuneFailureCountsAcrossIndexTerminalAndDetail(t *testing.T) {
	provider := proTestProvider()
	g, e := proTestGame(), proTestEvent()
	var recovered atomic.Bool
	var windows atomic.Int32
	provider.client.Transport = championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body []byte
		switch {
		case strings.HasSuffix(r.URL.Path, "getSchedule"):
			body, _ = json.Marshal(map[string]any{"data": map[string]any{"schedule": map[string]any{"events": []any{map[string]any{"startTime": g.Start, "state": "completed", "match": map[string]string{"id": e.ID}}}}}})
		case strings.HasSuffix(r.URL.Path, "getEventDetails"):
			body, _ = json.Marshal(map[string]any{"data": map[string]any{"event": e}})
		case strings.Contains(r.URL.Path, "/window/"):
			windows.Add(1)
			if !recovered.Load() {
				return nil, errors.New("single game unavailable")
			}
			body, _ = json.Marshal(proTestEnd(g))
		case strings.Contains(r.URL.Path, "/details/"):
			body = []byte(`{"frames":[{"rfc460Timestamp":"` + proStartingTime(g.Start.Add(30*time.Minute)) + `","participants":[{"participantId":3,"perkMetadata":{"styleId":8100,"subStyleId":8400,"perks":[8112,8143,8137,8106,8401,8444,5008,5011]}}]}]}`)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}}, nil
	})
	notified := make(chan struct{}, 10)
	p := newProRuneProvider(provider, nil, func() { notified <- struct{}{} })
	// An old valid index survives the failed re-index, so the same ID reaches
	// terminal and on-demand detail supplementation too.
	p.snapshot = proRuneSnapshot{Games: map[string]proRuneIndexGame{g.ID: g}, Events: map[string]proRuneEvent{e.ID: e}, ReadAt: time.Now().Add(-time.Hour)}
	refresh := func() {
		t.Helper()
		p.kick(true)
		deadline := time.NewTimer(3 * time.Second)
		defer deadline.Stop()
		for {
			select {
			case <-notified:
				p.mu.Lock()
				done := !p.preparing && !p.outcomesLoading
				p.mu.Unlock()
				if done {
					return
				}
			case <-deadline.C:
				t.Fatal("refresh and outcome check did not finish")
			}
		}
	}
	refresh()
	for i := 0; i < 2; i++ {
		res := p.recommend(context.Background(), 69, "mid")
		if res.TotalGames != 1 || res.Failed != 1 || res.Stale {
			t.Fatalf("same game counted repeatedly: %+v", res)
		}
	}
	if windows.Load() < 4 {
		t.Fatalf("did not exercise index, terminal and both detail attempts: %d", windows.Load())
	}
	recovered.Store(true)
	refresh()
	res := p.recommend(context.Background(), 69, "mid")
	if res.TotalGames != 1 || res.Failed != 0 || res.Stale || res.Reason != "" || len(res.Pros) != 1 {
		t.Fatalf("recovery kept an old failure: %+v", res)
	}
}

func TestProRunePerPlayerLimitAndStrictPosition(t *testing.T) {
	rows := []proRuneRow{}
	now := time.Now()
	for player := 0; player < 7; player++ {
		for game := 0; game < 8; game++ {
			g := proTestGame()
			g.ID = fmt.Sprintf("%d-%d", player, game)
			g.Start = now.Add(-time.Duration(game+1) * time.Hour)
			part := g.Metadata.Blue.Participants[0]
			part.PlayerID = fmt.Sprint(player)
			rows = append(rows, proRuneRow{Game: g, Participant: part, TeamID: "T1", Player: fmt.Sprint(player)})
		}
	}
	wrong := rows[0]
	wrong.Participant.Role = "top"
	wrong.Game.Start = now
	rows = append(rows, wrong)
	wrong.Participant.Role = "mid"
	wrong.Participant.ChampionID = "Ahri"
	rows = append(rows, wrong)
	selected := proNewestRows(rows, "Cassiopeia", "middle")
	if len(selected) != 35 {
		t.Fatalf("want 7 players x 5 games, got %d", len(selected))
	}
	counts := map[string]int{}
	for i, r := range selected {
		counts[r.Player]++
		if r.Participant.Role != "mid" || r.Participant.ChampionID != "Cassiopeia" {
			t.Fatal("wrong champion/lane", r)
		}
		if i > 0 && r.Game.Start.After(selected[i-1].Game.Start) {
			t.Fatal("not newest first")
		}
		if now.Sub(r.Game.Start) > 5*time.Hour {
			t.Fatal("not latest five per player")
		}
	}
	for player, count := range counts {
		if count != 5 {
			t.Fatalf("player %s: %d", player, count)
		}
	}
	for _, lane := range []string{"support", "", "other"} {
		if len(proNewestRows(rows, "Cassiopeia", lane)) != 0 {
			t.Fatal("must not fall back to another lane", lane)
		}
	}
	for _, pair := range [][2]string{{"bottom", "adc"}, {"utility", "support"}, {"middle", "mid"}} {
		row := rows[0]
		row.Participant.Role = pair[0]
		if len(proNewestRows([]proRuneRow{row}, "Cassiopeia", pair[1])) != 1 {
			t.Fatal("lane alias rejected", pair)
		}
	}
}

func TestProRuneMultiPlayerRecommendationKeepsFullRecord(t *testing.T) {
	p := newProRuneProvider(proTestProvider(), nil, nil)
	p.snapshot = proRuneSnapshot{Games: map[string]proRuneIndexGame{}, Events: map[string]proRuneEvent{}, ReadAt: time.Now()}
	for i := 0; i < 9; i++ {
		g := proTestGame()
		g.ID = fmt.Sprint(1000 + i)
		g.MatchID = fmt.Sprint(2000 + i)
		g.Start = time.Now().Add(-time.Duration(i+1) * time.Hour)
		// Both Faker and Chovy played the same hero in this fixture.
		g.Metadata.Red.Participants[0].ChampionID = "Cassiopeia"
		if i == 8 {
			g.Start = time.Now().Add(-31 * 24 * time.Hour)
		}
		e := proTestEvent()
		e.ID = g.MatchID
		e.Match.Games[0].ID = g.ID
		p.snapshot.Games[g.ID] = g
		p.snapshot.Events[e.ID] = e
		p.ends[g.ID] = proTestEnd(g)
		var d proDetails
		_ = json.Unmarshal([]byte(`{"frames":[{"participants":[{"participantId":3,"items":[3364,6657],"perkMetadata":{"styleId":8100,"subStyleId":8400,"perks":[8112,8143,8137,8106,8401,8444,5008,5011]}},{"participantId":8,"items":[3363,3116],"perkMetadata":{"styleId":8100,"subStyleId":8400,"perks":[8112,8143,8137,8106,8401,8444,5008,5011]}}]}]}`), &d)
		p.details[g.ID] = d
	}
	result := p.recommend(context.Background(), 69, "mid")
	if len(result.Pros) != 10 {
		t.Fatalf("want 5 games for each of 2 players, got %d: %+v", len(result.Pros), result)
	}
	counts := map[string]int{}
	for _, row := range result.Pros {
		counts[row.PlayerName]++
		if row.RecordGames != 8 {
			t.Fatalf("record must include all 8 in-window games, not 5 displayed or 9 all-time: %+v", row)
		}
		if len(row.ItemIDs) != 2 || len(row.SelectedPerkIDs) != 9 {
			t.Fatal("missing game loadout", row)
		}
		if row.PlayerName == "Faker" && row.RecordWins != 8 || row.PlayerName == "Chovy" && row.RecordWins != 0 {
			t.Fatal("incorrect win record", row)
		}
	}
	if counts["Faker"] != 5 || counts["Chovy"] != 5 {
		t.Fatal(counts)
	}
	empty := p.recommend(context.Background(), 69, "top")
	if len(empty.Pros) != 0 || empty.Reason != "no-sample" {
		t.Fatal("wrong-lane fallback", empty)
	}
}

func TestProHitParallelProcessesAllSelectedGames(t *testing.T) {
	values := make([]int, 35)
	for i := range values {
		values[i] = i
	}
	var count, active, maximum atomic.Int32
	proHitParallel(context.Background(), values, func(i int) {
		n := active.Add(1)
		for old := maximum.Load(); n > old && !maximum.CompareAndSwap(old, n); old = maximum.Load() {
		}
		time.Sleep(time.Millisecond)
		count.Add(1)
		active.Add(-1)
	})
	if count.Load() != 35 || maximum.Load() > 5 {
		t.Fatalf("processed=%d, concurrency=%d", count.Load(), maximum.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	proHitParallel(ctx, values, func(int) { t.Error("cancelled work executed") })
}

func TestProBackgroundWorkersLeaveForegroundSlot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gate := make(chan struct{}, 4)
	started := make(chan struct{}, 10)
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		proParallelLimit(ctx, []int{1, 2, 3, 4, 5, 6}, proRuneBackgroundWorkers, func(_ int) {
			gate <- struct{}{}
			started <- struct{}{}
			<-release
			<-gate
		})
	}()
	defer func() { close(release); <-done }()
	for i := 0; i < 3; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("background work did not start")
		}
	}
	select {
	case <-started:
		t.Fatal("background consumed foreground capacity")
	case <-time.After(20 * time.Millisecond):
	}
	select {
	case gate <- struct{}{}:
		<-gate
	case <-time.After(time.Second):
		t.Fatal("no foreground capacity")
	}
}
