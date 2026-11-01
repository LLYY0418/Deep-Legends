package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestR317BootsValidationAndBlocks(t *testing.T) {
	a, c := r298ReviewApp(t, &r298ReviewLCU{})
	catalog := autoConfigCatalog{}
	for _, id := range []int64{1001, 3006, 3047, 1055} {
		item := autoConfigItem{ID: id, InStore: true, DisplayInItemSets: true, Price: 3000, Maps: map[string]bool{"11": true, "12": true}}
		if id != 1055 {
			item.Categories = []string{"Boots"}
		}
		catalog.Items = append(catalog.Items, item)
	}
	a.autoConfig.catalog = &catalog
	a.autoConfig.catalogClient = c
	a.autoConfig.catalogKey, _ = clientCatalogKey(c)
	a.autoConfig.catalogAt = time.Now()
	for _, tc := range []struct {
		ids   []int64
		valid bool
	}{{[]int64{1001, 3006, 3047}, true}, {[]int64{1001, 1001}, false}, {[]int64{1055}, false}, {[]int64{1001, 3006, 3047, 3111}, false}, {[]int64{-1}, false}} {
		e := autoConfigEntry{Items: &autoConfigItems{Boots: tc.ids}}
		err := validateAutoConfigEntry("normal", e)
		if err == nil {
			err = a.validateAutoConfigCatalog(context.Background(), "normal", e, false)
		}
		if (err == nil) != tc.valid {
			t.Fatalf("boots %v err=%v", tc.ids, err)
		}
	}
	e := autoConfigEntry{Items: &autoConfigItems{Starter: []autoConfigStarterItem{{ID: 1055, Count: 1}}, Boots: []int64{1001, 3006}, Core: []int64{4000}, Situational: []int64{4001}}}
	blocks := autoConfigItemBlocks(e.Items)
	var names []string
	for _, b := range blocks {
		names = append(names, b.Type)
	}
	if !reflect.DeepEqual(names, []string{"出门装", "鞋子", "核心出装", "备选"}) {
		t.Fatal(names)
	}
	if len(blocks[1].Items) != 2 || blocks[1].Items[1].ID != 3006 || blocks[1].Items[1].Count != 1 {
		t.Fatal(blocks)
	}
	e.Items.Boots = nil
	if len(autoConfigItemBlocks(e.Items)) != 3 {
		t.Fatal("empty boots block written")
	}
	for _, mode := range []string{"normal", "aram", "hextech"} {
		if !autoConfigStarterAllowed(catalog.Items[3], mode) {
			t.Fatal("price cap survived", mode)
		}
	}
}

func TestR317RetiredArenaFilesRemovedOnceWithoutSquadLoss(t *testing.T) {
	s := &localStore{root: t.TempDir()}
	names := []string{"arena-" + "fa" + "me-events.json", "arena-progression-before.json"}
	for _, name := range append(names, "arena-squad.json") {
		if err := os.WriteFile(filepath.Join(s.root, name), []byte("old"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	removeRetiredArenaCache(s)
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(s.root, name)); !os.IsNotExist(err) {
			t.Fatal(name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(s.root, "arena-squad.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.root, names[0]), []byte("sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
	removeRetiredArenaCache(s)
	if _, err := os.Stat(filepath.Join(s.root, names[0])); err != nil {
		t.Fatal("migration repeated")
	}
}

func TestR317RetiredArenaSourceAudit(t *testing.T) {
	forbidden := []string{"arena_" + "fa" + "me", "arena" + "Fa" + "me", "arena-" + "fa" + "me"}
	for _, dir := range []string{".", "../scripts"} {
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, ".js") && !strings.HasSuffix(path, ".cjs") && !strings.HasSuffix(path, ".css") && !strings.HasSuffix(path, ".svg") {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, word := range forbidden {
				if bytes.Contains(raw, []byte(word)) {
					t.Errorf("retired identifier %s in %s", word, path)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestR317TwoSecondStallAndRepeatDemotion(t *testing.T) {
	now := time.Now()
	r := &updateRoute{prefix: "slow"}
	other := &updateRoute{prefix: "fast", lastProgress: now.Add(2100 * time.Millisecond)}
	var cancelled atomic.Int64
	c := &updateChunk{flights: map[*updateFlight]bool{}}
	f := &updateFlight{chunk: c, source: r, started: now, lastProgress: now, cancel: func() { cancelled.Add(1) }}
	c.flights[f] = true
	s := &updateScheduler{manager: &updateManager{}, routes: []*updateRoute{r, other}, chunks: []*updateChunk{c}, started: now, lastProgress: now}
	s.tick(now.Add(1999 * time.Millisecond))
	if cancelled.Load() != 0 {
		t.Fatal("early abort")
	}
	s.tick(now.Add(2250 * time.Millisecond))
	if cancelled.Load() != 1 || s.stalledAborts != 1 || s.demotions != 0 {
		t.Fatal(cancelled.Load(), s.stalledAborts, s.demotions)
	}
	f.stalled = false
	f.started = now.Add(3 * time.Second)
	f.lastProgress = f.started
	other.lastProgress = now.Add(5 * time.Second)
	s.tick(now.Add(5250 * time.Millisecond))
	if cancelled.Load() != 2 || s.demotions != 1 {
		t.Fatal("second stall not demoted", cancelled.Load(), s.demotions)
	}
}

func TestR317RangeStallResumesAndPreservesSHA(t *testing.T) {
	for _, single := range []bool{false, true} {
		t.Run(fmt.Sprint(single), func(t *testing.T) {
			data := bytes.Repeat([]byte{37}, 12<<20)
			var first atomic.Bool
			stalled := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var start, end int64
				fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end)
				w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
				w.Header().Set("Content-Length", fmt.Sprint(end-start+1))
				w.WriteHeader(206)
				if first.CompareAndSwap(false, true) {
					w.Write(data[start : start+64<<10])
					w.(http.Flusher).Flush()
					<-r.Context().Done()
					return
				}
				for at := start; at <= end; {
					n := min(int64(32<<10), end-at+1)
					select {
					case <-r.Context().Done():
						return
					case <-time.After(20 * time.Millisecond):
					}
					if _, err := w.Write(data[at : at+n]); err != nil {
						return
					}
					w.(http.Flusher).Flush()
					at += n
				}
			}))
			defer stalled.Close()
			sources := []string{stalled.URL + "/"}
			if !single {
				fast := r316Source(t, data, "equal")
				sources = append(sources, fast.URL+"/")
			}
			var mu sync.Mutex
			var final map[string]any
			u := &updateManager{client: stalled.Client(), diagnostic: func(e map[string]any) {
				if e["event"] == "update_download_segments_finished" {
					mu.Lock()
					final = e
					mu.Unlock()
				}
			}}
			race := &updateSourceRace{first: sources[0], sources: sources, results: map[string]updateSourceProbe{}}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			part := filepath.Join(t.TempDir(), "asset.part")
			if err := u.downloadSegments(ctx, r278Asset(data), part, race); err != nil {
				t.Fatal(err)
			}
			if err := verifyUpdateFile(ctx, part, r278Asset(data)); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			defer mu.Unlock()
			if final == nil {
				t.Fatal("missing completion diagnostic")
			}
			if final["max_progress_gap_ms"].(int64) > 2500 {
				t.Fatal(final)
			}
			if final["stalled_aborts"].(int) < 1 {
				t.Fatal("silent connection was not migrated", final)
			}
			t.Log(final)
		})
	}
}

func TestR317ArenaLockGateAndChampionChange(t *testing.T) {
	cell := int64(7)
	session := lcuChampSelectSession{QueueID: 1750, LocalPlayerCellID: &cell, MyTeam: []lcuChampSelectPlayer{{CellID: &cell, ChampionID: 154}}, Actions: [][]lcuChampSelectAction{{{ActorCellID: cell, Type: "pick", ChampionID: 154}}}}
	if lockedArenaChampion(session) != 0 {
		t.Fatal("hover treated as locked")
	}
	session.Actions[0][0].Completed = true
	if lockedArenaChampion(session) != 154 {
		t.Fatal("completed lock lost")
	}
	a := &app{}
	loaded := make(chan int64, 2)
	a.liveRecommendationPrewarmer = &liveRecommendationPrewarmer{load: func(_ context.Context, s liveRecommendationSeed) error { loaded <- s.ChampionID; return nil }}
	a.prewarmLockedArena(session, 1750, "CHERRY")
	session.MyTeam[0].ChampionID = 69
	session.Actions[0][0].ChampionID = 69
	a.prewarmLockedArena(session, 1750, "CHERRY")
	seen := map[int64]bool{}
	for range 2 {
		select {
		case id := <-loaded:
			seen[id] = true
		case <-time.After(time.Second):
			t.Fatal("prewarm did not start")
		}
	}
	if !seen[69] || !seen[154] {
		t.Fatal(seen)
	}
}

func TestR317SoloFlexJSONHasNoRetiredField(t *testing.T) {
	p := gameplayLivePlayer{}
	p.SoloRank = &gameplayRank{Tier: "GOLD"}
	p.FlexRank = &gameplayRank{Tier: "EMERALD"}
	p.ArenaRecord = &gameplayArenaRecord{Games: 3}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"soloRank", "flexRank", "arenaRecord"} {
		if !bytes.Contains(raw, []byte(field)) {
			t.Fatal(string(raw))
		}
	}
	if bytes.Contains(raw, []byte("arena"+"Fa"+"me")) {
		t.Fatal(string(raw))
	}
}

func r317ArenaProvider(t *testing.T, your func(*http.Request, int) (*http.Response, error), opFailed bool) *championProvider {
	t.Helper()
	p := newChampionProvider()
	p.cache = newChampionDataCache(nil)
	p.patch = "16.19.1"
	p.arenaVersion = "16.19"
	p.arenaVersionAt = time.Now()
	for i := 0; i < 6; i++ {
		p.remoteAugments = append(p.remoteAugments, gameplayAugment{ID: int64(9901 + i), Name: fmt.Sprint("夹具", i), Rarity: []string{"kSilver", "kGold", "kPrismatic"}[i%3]})
	}
	var count atomic.Int64
	p.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == yourGGArenaHost {
			if strings.HasSuffix(r.URL.Path, "/champions") {
				return updateResponse(200, []byte(`{"response":[]}`)), nil
			}
			return your(r, int(count.Add(1)))
		}
		if opFailed {
			return nil, context.DeadlineExceeded
		}
		return updateResponse(200, []byte(`{"data":{"summary":{"id":67},"core_items":[{"ids":[3153],"play":100,"win":60}],"augment_group":[{"rarity":1,"augments":[{"id":9901,"play":100,"win":55}]},{"rarity":2,"augments":[{"id":9902,"play":100,"win":55}]},{"rarity":3,"augments":[{"id":9903,"play":100,"win":55}]}]},"meta":{"version":"16.19"}}`)), nil
	})}
	return p
}
func r317YourBody() []byte {
	rows := []map[string]any{}
	for i := 0; i < 6; i++ {
		rows = append(rows, map[string]any{"augmentId": 9901 + i, "tier": "A", "score": 80, "winRate": .6, "averagePlacement": 3, "firstPlacementRate": .2, "matches": 300})
	}
	raw, _ := json.Marshal(map[string]any{"response": map[string]any{"version": "16.19", "coreItems": []map[string]any{{"itemId": 3153, "tier": "A", "score": 80, "winRate": .6, "matches": 300}}, "prismaticItems": []any{}, "augments": rows}})
	return raw
}
func TestR317ArenaFastSourceThenRicherCacheWithoutHistory(t *testing.T) {
	release := make(chan struct{})
	p := r317ArenaProvider(t, func(r *http.Request, _ int) (*http.Response, error) {
		select {
		case <-r.Context().Done():
			return nil, r.Context().Err()
		case <-release:
			return updateResponse(200, r317YourBody()), nil
		}
	}, false)
	start := time.Now()
	first, err := p.loadLiveArenaDetail(context.Background(), 67, "")
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("fast source waited for optional work")
	}
	if !first.ArenaPending || len(first.ArenaAugments) != 3 || first.ArenaCacheHit {
		t.Fatalf("first %+v", first)
	}
	close(release)
	deadline := time.Now().Add(3 * time.Second)
	for {
		next, err := p.loadLiveArenaDetail(context.Background(), 67, "")
		if err != nil {
			t.Fatal(err)
		}
		if !next.ArenaPending {
			if len(next.ArenaAugments) != 6 || !next.ArenaCacheHit {
				t.Fatal("richer source lost", next)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cache never converged")
		}
		time.Sleep(time.Millisecond)
	}
}
func TestR317ArenaHedgeSucceedsWhenPrimaryTimesOutAndOPFails(t *testing.T) {
	p := r317ArenaProvider(t, func(r *http.Request, attempt int) (*http.Response, error) {
		if attempt == 1 {
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return updateResponse(200, r317YourBody()), nil
	}, true)
	started := time.Now()
	detail, err := p.loadLiveArenaDetail(context.Background(), 67, "")
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(started)
	if elapsed < 2400*time.Millisecond || elapsed > 3*time.Second || !detail.ArenaHedged || len(detail.ArenaAugments) != 6 {
		t.Fatal("hedge timing/result", elapsed, detail.ArenaHedged, len(detail.ArenaAugments))
	}
	t.Log("hedge ready", elapsed)
}
func TestR317ArenaLockedPrewarmReusedImmediatelyOnEntry(t *testing.T) {
	p := r317ArenaProvider(t, func(_ *http.Request, _ int) (*http.Response, error) { return updateResponse(200, r317YourBody()), nil }, true)
	a := &app{}
	ready := make(chan struct{})
	a.liveRecommendationPrewarmer = newLiveRecommendationPrewarmer(p, nil)
	a.liveRecommendationPrewarmer.onReady = func(s liveRecommendationSeed) { close(ready) }
	cell := int64(7)
	s := lcuChampSelectSession{QueueID: 1750, LocalPlayerCellID: &cell, MyTeam: []lcuChampSelectPlayer{{CellID: &cell, ChampionID: 67}}, Actions: [][]lcuChampSelectAction{{{ActorCellID: 7, Type: "pick", ChampionID: 67, Completed: true}}}}
	a.prewarmLockedArena(s, 1750, "CHERRY")
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("lock prewarm late")
	}
	started := time.Now()
	result, err := p.loadLiveArenaDetail(context.Background(), 67, "")
	if err != nil || !result.ArenaCacheHit || time.Since(started) > time.Second {
		t.Fatal("entry missed ready cache", err, time.Since(started))
	}
}

func TestR317ArenaAllPlayersUseSoloAndFlexPipeline(t *testing.T) {
	for _, queue := range []int{1700, 1750} {
		t.Run(strconv.Itoa(queue), func(t *testing.T) {
			var calls atomic.Int64
			ref := strings.Repeat("r", 48)
			client := &LCUClient{baseURL: "https://fixture", token: "test", platformProbe: true}
			client.http = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
				body := ""
				switch r.URL.Path {
				case "/lol-gameflow/v1/gameflow-phase":
					body = `"ChampSelect"`
				case "/lol-gameflow/v1/session":
					body = fmt.Sprintf(`{"gameData":{"gameId":777,"queue":{"id":%d,"mapId":30,"gameMode":"CHERRY"},"teamOne":[],"teamTwo":[]},"map":{"id":30,"gameMode":"CHERRY"}}`, queue)
				case "/lol-champ-select/v1/session":
					body = fmt.Sprintf(`{"localPlayerCellId":7,"myTeam":[{"cellId":7,"puuid":%q,"summonerId":123,"summonerName":"Player","championId":22},{"cellId":8,"puuid":"r317-other-reference-000000000000000000000000000000","summonerId":124,"summonerName":"Other","championId":69}],"theirTeam":[],"actions":[]}`, ref)
				case "/lol-champ-select/v1/current-champion":
					body = `22`
				case "/lol-ranked/v1/current-ranked-stats", "/lol-ranked/v1/ranked-stats/r317-other-reference-000000000000000000000000000000":
					calls.Add(1)
					body = `{"queues":[{"queueType":"RANKED_SOLO_5x5","tier":"DIAMOND","division":"II"},{"queueType":"RANKED_FLEX_SR","tier":"GOLD","division":"IV"}]}`
				default:
					return r206RelayResponse(404, []byte(`{}`)), nil
				}
				return r206RelayResponse(200, []byte(body)), nil
			})}
			a := &app{connected: true, lcu: client, summoner: Summoner{SummonerID: 123, PUUID: ref}}
			for range 2 {
				rec := httptest.NewRecorder()
				a.handleGameplayLive(rec, httptest.NewRequest(http.MethodGet, "/api/gameplay/live", nil))
				var response gameplayLiveResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if len(response.Players) != 2 {
					t.Fatal(rec.Body.String())
				}
				for _, player := range response.Players {
					if player.SoloRank == nil || player.FlexRank == nil || player.Rank == nil || player.Rank.Tier != "DIAMOND" {
						t.Fatalf("Arena solo/flex missing: %s", rec.Body.String())
					}
				}
			}
			if calls.Load() != 2 {
				t.Fatalf("rank reads=%d", calls.Load())
			}
		})
	}
}
