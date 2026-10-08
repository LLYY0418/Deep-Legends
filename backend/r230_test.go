package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestR230SeasonParseLossRetriesSamePage(t *testing.T) {
	a, ref, _ := newGameplayOverviewSGPFixture(t, false)
	a.storage = r175App(t).storage
	reference, _ := a.resolveGameplayReferenceDetails(ref)
	info := &riotMatchInfo{GameID: 1, QueueID: 420, GameCreation: seasonStartS26.Add(time.Hour).UnixMilli()}
	a.sgp.cacheHistoryPage("HN1", reference.PlayerRef, 0, 50, seasonStreamTags["ranked"], sgpHistoryPage{games: []*riotMatchInfo{info}, consumed: 50, more: false})
	scan := &seasonScanState{cache: seasonStatsCache{}, stats: map[int64]*gameplaySeasonChampionStat{}, queueStats: map[int64]gameplayAggregate{}, seen: map[int64]bool{}, seasonStartMillis: seasonStartS26.UnixMilli()}
	a.seasonScanPagesWithHistoryCache(context.Background(), a.lcu, "HN1", reference.PlayerRef, scan, 1, true)
	if scan.cache.Complete || scan.cache.ResumeIndex != 0 || !scan.interrupted {
		t.Fatal("parse loss marked complete/skipped the dropped page", scan)
	}
	events := r175Events(t, a, "season_scan_parse_dropped")
	if len(events) != 1 || events[0]["returned"] != float64(50) || events[0]["parsed"] != float64(1) || events[0]["resume_index"] != float64(0) {
		t.Fatal(events)
	}
}
func TestR230SeasonCompletionRequiresBoundaryOrCleanTerminalPage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		created int64
		more    bool
		want    bool
	}{
		{"season-boundary", seasonStartS26.Add(-time.Second).UnixMilli(), true, true},
		{"clean-terminal", seasonStartS26.Add(time.Hour).UnixMilli(), false, true},
		{"more-upstream", seasonStartS26.Add(time.Hour).UnixMilli(), true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, ref, _ := newGameplayOverviewSGPFixture(t, false)
			reference, _ := a.resolveGameplayReferenceDetails(ref)
			infos := []*riotMatchInfo{{GameID: 2, GameCreation: tc.created}}
			if tc.more {
				for len(infos) < 50 {
					infos = append(infos, infos[0])
				}
			}
			a.sgp.cacheHistoryPage("HN1", reference.PlayerRef, 0, 50, seasonStreamTags["ranked"], sgpHistoryPage{games: infos, consumed: len(infos), more: tc.more})
			scan := &seasonScanState{stats: map[int64]*gameplaySeasonChampionStat{}, queueStats: map[int64]gameplayAggregate{}, seen: map[int64]bool{}, seasonStartMillis: seasonStartS26.UnixMilli()}
			a.seasonScanPagesWithHistoryCache(context.Background(), a.lcu, "HN1", reference.PlayerRef, scan, 1, true)
			if scan.cache.Complete != tc.want {
				t.Fatal(scan.cache)
			}
		})
	}
}
func TestR230Schema12FalseCompleteTriggersBackfill(t *testing.T) {
	a, ref, _ := newGameplayOverviewSGPFixture(t, true)
	a.storage = r175App(t).storage
	reference, _ := a.resolveGameplayReferenceDetails(ref)
	player := Summoner{PUUID: reference.PlayerRef}
	season, _ := currentRankedSeason(time.Now())
	hash := a.storage.accountHash(player)
	if _, err := a.storage.saveSeasonStatsReported(seasonStatsCache{SchemaVersion: 12, Source: seasonStatsSource, AccountHash: hash, Season: season, Complete: true, GameIDs: []int64{999}, Stats: []gameplaySeasonChampionStat{{ChampionID: 1, Games: 1}}}); err != nil {
		t.Fatal(err)
	}
	if loaded, err := a.storage.loadSeasonStats(seasonStatsSource, hash, season); err != nil || len(loaded.GameIDs) != 1 || len(loaded.Stats) != 1 {
		t.Fatal("schema12 history lost")
	}
	// Two full current-season pages fill the foreground budget; backfill must
	// fetch the terminal third page. Completion is awaited before store cleanup.
	raw := fmt.Sprintf(`{"gameId":420,"queueId":420,"gameCreation":%d,"gameDuration":1800,"participants":[{"puuid":"%s","championId":103}]}`, time.Now().UnixMilli(), reference.PlayerRef)
	a.sgp.http.Transport = sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := `{"games":[]}`
		start := r.URL.Query().Get("startIndex")
		if start == "0" || start == "50" {
			offset := 0
			if start == "50" {
				offset = 50
			}
			rows := make([]string, 50)
			for i := range rows {
				rows[i] = `{"json":` + strings.Replace(raw, `"gameId":420`, fmt.Sprintf(`"gameId":%d`, 420+offset+i), 1) + `}`
			}
			body = `{"games":[` + strings.Join(rows, ",") + `]}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	_, progress, _, _ := a.loadSeasonChampionStatsWithHistoryCache(context.Background(), a.lcu, reference, player, reference.PlayerRef, bundledChampionNames(), true)
	if progress.Complete {
		t.Fatal("old completeness was reused", progress)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		a.seasonBackfillMu.Lock()
		n := len(a.seasonBackfills)
		a.seasonBackfillMu.Unlock()
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("backfill timeout")
		}
		time.Sleep(time.Millisecond)
	}
	if len(r175Events(t, a, "season_backfill_round")) == 0 {
		t.Fatal("schema upgrade failed to trigger backfill")
	}
}
func TestR230NamesWithoutCollectionOnBothEndpoints(t *testing.T) {
	a, ref, _ := newGameplayOverviewSGPFixture(t, false)
	a.storage = r175App(t).storage
	reference, _ := a.resolveGameplayReferenceDetails(ref)
	season, _ := currentRankedSeason(time.Now())
	cache := seasonStatsCache{SchemaVersion: seasonStatsCacheSchemaVersion, Source: seasonStatsSource, AccountHash: a.storage.accountHash(Summoner{PUUID: reference.PlayerRef}), Season: season, Complete: true, ChampionTable: []seasonTableBucket{{ChampionID: 804, QueueID: 420, Totals: seasonTableTotals{Games: 20, Wins: 12}}}}
	if _, err := a.storage.saveSeasonStatsReported(cache); err != nil {
		t.Fatal(err)
	}
	a.lcu.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`[{"championId":804,"championLevel":9,"championPoints":10000}]`)), Request: r}, nil
	})
	for _, endpoint := range []string{"champion-table", "masteries"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/gameplay/"+endpoint+"?playerRef="+ref+"&queue=420", nil)
		if endpoint == "masteries" {
			a.handleGameplayMasteries(w, r)
		} else {
			a.handleGameplayChampionTable(w, r)
		}
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"championName":"`+bundledChampionNames()[804]+`"`) {
			t.Fatal(endpoint, w.Code, w.Body.String())
		}
		if endpoint == "masteries" {
			var response masteryDetailsResponse
			_ = json.Unmarshal(w.Body.Bytes(), &response)
			if response.TotalChampions <= 0 {
				t.Fatal(response)
			}
		}
	}
}
func TestR230ArenaSquadRestoresSameGameAndExpires(t *testing.T) {
	for _, tc := range []struct {
		name   string
		gameID int64
		age    time.Duration
		want   int
	}{{"same", 230, 0, 3}, {"different", 231, 0, 0}, {"expired", 230, 3 * time.Hour, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			a := r175App(t)
			a.summoner = Summoner{PUUID: "member0"}
			a.arenaAllyGameID = 230
			for i := 0; i < 3; i++ {
				a.arenaAllyPlayers = append(a.arenaAllyPlayers, lcuLivePlayer{PUUID: fmt.Sprintf("member%d", i), SummonerID: int64(i + 1), GameName: fmt.Sprintf("玩家%d", i), TagLine: "测试"})
			}
			a.persistArenaSquad(230)
			if tc.age > 0 {
				data, _ := readLocalStoreFile(a.storage, arenaSquadFile)
				var record arenaSquadRecord
				_ = json.Unmarshal(data, &record)
				record.RecordedAt = time.Now().Add(-tc.age)
				data, _ = json.Marshal(record)
				if err := writeLocalStoreFile(a.storage, arenaSquadFile, data); err != nil {
					t.Fatal(err)
				}
			}
			restarted := &app{storage: a.storage, summoner: a.summoner}
			response := gameplayLiveResponse{GameID: tc.gameID, QueueID: 1750, GameMode: "CHERRY"}
			for i := 0; i < 4; i++ {
				response.Players = append(response.Players, gameplayLivePlayer{gameplayPlayer: gameplayPlayer{reference: gameplayReference{PlayerRef: fmt.Sprintf("member%d", i), SummonerID: int64(i + 1)}}})
			}
			restarted.markRememberedArenaSquad(&response)
			n := 0
			for _, p := range response.Players {
				if p.MySquad {
					n++
				}
			}
			if n != tc.want {
				t.Fatal(n, response)
			}
			if tc.want == 0 {
				if _, err := os.Stat(filepath.Join(a.storage.root, arenaSquadFile)); !os.IsNotExist(err) {
					t.Fatal("stale file remains", err)
				}
			} else {
				events := r175Events(t, a, "arena_squad_restored")
				if len(events) != 1 || events[0]["source"] != "disk" || events[0]["members"] != float64(3) {
					t.Fatal(events)
				}
			}
		})
	}
}

func TestR230OverviewNavigationReusesHistoryAndManualRefreshBypasses(t *testing.T) {
	a, ref, requests := newGameplayOverviewSGPFixture(t, true)
	call := func(fresh bool) {
		w := httptest.NewRecorder()
		body := fmt.Sprintf(`{"playerRef":"%s","force":true,"freshHistory":%t,"count":20}`, ref, fresh)
		a.handleGameplayOverview(w, httptest.NewRequest("POST", "/api/gameplay/overview", strings.NewReader(body)))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	call(false)
	first := requests.Load()
	if first == 0 {
		t.Fatal("cold request made no history reads")
	}
	call(false)
	if (requests.Load()-first)*2 >= first {
		t.Fatal("navigation failed to halve history requests", first, requests.Load()-first)
	}
	warm := requests.Load()
	call(true)
	if requests.Load() <= warm {
		t.Fatal("manual refresh reused stale history")
	}
}

func TestR230ArenaSquadDoesNotRestoreAcrossAccounts(t *testing.T) {
	a := r175App(t)
	a.summoner = Summoner{PUUID: "owner-a"}
	a.arenaAllyGameID = 230
	a.arenaAllyPlayers = []lcuLivePlayer{{PUUID: "owner-a"}, {PUUID: "mate"}}
	a.persistArenaSquad(230)
	b := &app{storage: a.storage, summoner: Summoner{PUUID: "owner-b"}}
	b.restoreArenaSquad(230)
	if len(b.arenaAllyPlayers) != 0 {
		t.Fatal("cross-account squad restored")
	}
	if _, err := readLocalStoreFile(a.storage, arenaSquadFile); !os.IsNotExist(err) {
		t.Fatal("foreign record not removed", err)
	}
}

func TestR230RepeatedOverviewHasCacheHitsAndHalfBytes(t *testing.T) {
	a, ref, _ := newGameplayOverviewSGPFixture(t, true)
	reference, _ := a.resolveGameplayReferenceDetails(ref)
	load := func() *overviewLoadCost {
		cost := &overviewLoadCost{}
		ctx := context.WithValue(context.Background(), overviewLoadCostContextKey{}, cost)
		ctx = context.WithValue(ctx, overviewFreshHistoryKey{}, false)
		a.loadGameplayOverview(ctx, a.lcu, a.summoner, reference, 0, 20, "all", true)
		return cost
	}
	first, second := load(), load()
	_, firstBytes, _, _ := first.snapshot()
	_, secondBytes, _, hits := second.snapshot()
	t.Logf("first=%+v second=%+v", first, second)
	if hits <= 0 || firstBytes <= 0 || secondBytes*2 >= firstBytes {
		t.Fatal("cache traffic target failed", firstBytes, secondBytes, hits)
	}
}

func TestR230ClientDiagnosticsUseOnlySafeFields(t *testing.T) {
	a := r175App(t)
	for _, body := range []string{`{"event":"summoner_copy","reason":"failed","ok":false,"method":"electron","error_name":"私密名字#编号"}`, `{"event":"overview_card_ready","reason":"ready","card":"champions","source":"snapshot","durationMs":140}`} {
		w := httptest.NewRecorder()
		a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(body)))
		if w.Code != 204 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	events := r175Events(t, a, "summoner_copy")
	if len(events) != 1 || events[0]["error_name"] != "" || events[0]["method"] != "electron" {
		t.Fatal(events)
	}
	events = r175Events(t, a, "overview_card_ready")
	if len(events) != 1 || events[0]["duration_ms"] != float64(140) || events[0]["source"] != "snapshot" {
		t.Fatal(events)
	}
}

func TestR230IncompleteHeadWithoutResumeAdvancesAcrossKnownGames(t *testing.T) {
	a, ref, _ := newGameplayOverviewSGPFixture(t, false)
	reference, _ := a.resolveGameplayReferenceDetails(ref)
	infos := make([]*riotMatchInfo, 50)
	for i := range infos {
		infos[i] = &riotMatchInfo{GameID: int64(i + 1), GameCreation: seasonStartS26.Add(time.Hour).UnixMilli()}
	}
	a.sgp.cacheHistoryPage("HN1", reference.PlayerRef, 0, 50, seasonStreamTags["ranked"], sgpHistoryPage{games: infos, consumed: 50, more: true})
	scan := &seasonScanState{stats: map[int64]*gameplaySeasonChampionStat{}, queueStats: map[int64]gameplayAggregate{}, seen: map[int64]bool{1: true}, seasonStartMillis: seasonStartS26.UnixMilli()}
	a.seasonScanPagesWithHistoryCache(context.Background(), a.lcu, "HN1", reference.PlayerRef, scan, 1, true)
	if scan.cache.Complete || scan.cache.ResumeIndex != 50 || scan.cache.PendingIndex != 50 {
		t.Fatal("retry stalled at the head instead of continuing", scan.cache)
	}
}
