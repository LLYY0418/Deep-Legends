package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR92RiotMatchConcreteDiskBudget(t *testing.T) {
	p := newChampionProvider()
	p.cache = newChampionDataCache(&localStore{root: t.TempDir()})
	c := newRiotMatchDiskCache(p)
	if c.diskMaxEntries != 600 || c.diskMaxBytes != 128<<20 || !c.strictDisk {
		t.Fatalf("riot-matches instance budget: entries=%d bytes=%d strict=%v", c.diskMaxEntries, c.diskMaxBytes, c.strictDisk)
	}
	for i := 0; i < 605; i++ {
		if err := c.writeDisk(championCacheEnvelope{Key: fmt.Sprintf("riot-match-v1|KR_%d", i+1), Data: []byte(`{"info":{"participants":[]}}`)}); err != nil {
			t.Fatal(err)
		}
	}
	files, err := os.ReadDir(c.dir)
	if err != nil {
		t.Fatal(err)
	}
	var bytes int64
	for _, f := range files {
		info, err := f.Info()
		if err != nil {
			t.Fatal(err)
		}
		bytes += info.Size()
	}
	if len(files) != 600 || bytes > 128<<20 {
		t.Fatalf("concrete disk occupancy: entries=%d bytes=%d", len(files), bytes)
	}
	// Restart must rebuild the bounded disk index, not reset its accounting.
	c = newRiotMatchDiskCache(p)
	if err := c.writeDisk(championCacheEnvelope{Key: "riot-match-v1|KR_606", Data: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	files, _ = os.ReadDir(c.dir)
	if len(files) != 600 {
		t.Fatalf("restart budget: %d", len(files))
	}
}

func TestR92ProLadderStartsBeforeSupplementsFinish(t *testing.T) {
	p := newChampionProvider()
	baseStarted := make(chan struct{}, 1)
	var serial atomic.Bool
	base := opggProTeam{ID: 632, Name: "Bilibili Gaming", Members: []opggProMember{proFixtureMember(632, "Bin", "Chen Ze-Bin (陈泽彬)", proFixtureAccount("fixtureaccount", "fixture", "CHALLENGER", 1, 1200))}}
	p.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == proSupplementHost {
			select {
			case <-baseStarted:
				baseStarted <- struct{}{}
			case <-time.After(500 * time.Millisecond):
				serial.Store(true)
			}
			name := strings.TrimPrefix(r.URL.Path, "/player/")
			_, player, _ := proSupplementPlayer(name)
			return proHTTPBody([]byte(fmt.Sprintf("<h1>%s</h1><table><tr><td>Name</td><td>%s</td></tr></table><div><h4>Accounts</h4><table></table></div>", player.Name, player.Names[0]))), nil
		}
		if r.URL.Path == proLadderPath {
			baseStarted <- struct{}{}
			return proHTTPBody([]byte(`<table><tr id="fixtureaccount-KR1"><td>42</td></tr></table>`)), nil
		}
		return proHTTPBody(proFixtureHTML("kr", base)), nil
	})}
	a := &app{champions: p}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	snapshot, _, err := a.loadProPlayers(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		a.proPlayers.mu.Lock()
		done := !a.proPlayers.updating
		a.proPlayers.mu.Unlock()
		if done {
			break
		}
		time.Sleep(time.Millisecond)
	}
	a.proPlayers.mu.Lock()
	completed := a.proPlayers.teams
	updating := a.proPlayers.updating
	a.proPlayers.mu.Unlock()
	if serial.Load() || updating {
		t.Fatalf("ladder waited for supplements: serial=%v updating=%v", serial.Load(), updating)
	}
	accounts := proRankedLadderAccounts(completed)
	if len(accounts) != 1 || !accounts[0].LadderRankKnown || accounts[0].LadderRank != 42 {
		t.Fatalf("final ladder lost: %+v", accounts)
	}
	for _, a := range proRankedLadderAccounts(snapshot) {
		if a.LadderRankKnown {
			t.Fatal("published directory snapshot mutated")
		}
	}
}

func TestR92ProLadderNewAccountsAndDedup(t *testing.T) {
	p := newChampionProvider()
	var calls atomic.Int32
	p.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return proHTTPBody([]byte(fmt.Sprintf(`<table><tr id="%s"><td>7</td></tr></table>`, r.URL.Query().Get("summoner")))), nil
	})}
	source := []opggProTeam{{ID: 632, Name: "Bilibili Gaming", Members: []opggProMember{proFixtureMember(632, "Bin", "Chen Ze-Bin (陈泽彬)", proFixtureAccount("first", "first", "CHALLENGER", 1, 1200))}}}
	pipeline := newProLadderPipeline(context.Background(), p)
	pipeline.submit(source)
	source[0].Members[0].Summoners = append(source[0].Members[0].Summoners, proFixtureAccount("second", "second", "MASTER", 1, 200))
	pipeline.submit(source)
	pipeline.submit(source)
	pipeline.finish()
	pipeline.apply(source)
	if calls.Load() != 2 {
		t.Fatalf("new account lost or duplicate: %d", calls.Load())
	}
	for _, a := range proRankedLadderAccounts(source) {
		if !a.LadderRankKnown || a.LadderRank != 7 {
			t.Fatal("new account rank missing")
		}
	}
}

func TestR92CommunityImageIndependentDiskRestart(t *testing.T) {
	root := t.TempDir()
	var calls atomic.Int32
	makeApp := func() *app {
		p := newChampionProvider()
		p.communityImageCache = newCommunityImageCache(&localStore{root: root})
		p.imageCache = newPublicBinaryCache(&localStore{root: root}, "champion-images", 2048, 64<<20)
		p.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host != communityDragonHost {
				t.Errorf("unexpected/LCU image host: %s", r.URL.Host)
			}
			calls.Add(1)
			return proHTTPBody(r89PNG), nil
		})}
		return &app{champions: p}
	}
	load := func(a *app) {
		for _, path := range []string{"/lol-game-data/assets/v1/profile-icons/0.jpg", "/lol-game-data/assets/DATA/Spells/Icons2D/Summoner_flash.png", "/lol-game-data/assets/v1/perk-images/Styles/Precision/Conqueror/Conqueror.png"} {
			w := httptest.NewRecorder()
			a.serveCommunityDragonImage(w, httptest.NewRequest("GET", "/api/image", nil), path)
			if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "image/") {
				t.Fatalf("image %d", w.Code)
			}
		}
	}
	a := makeApp()
	load(a)
	load(a)
	if calls.Load() != 3 {
		t.Fatalf("memory/singleflight cache: %d", calls.Load())
	}
	load(makeApp())
	if calls.Load() != 3 {
		t.Fatalf("restart missed CommunityDragon disk: %d", calls.Load())
	}
	c := a.champions.communityImageCache
	if c.diskMaxEntries != 512 || c.diskMaxBytes != 16<<20 || !c.strictDisk || c.dir == a.champions.imageCache.dir {
		t.Fatal("independent image budget lost")
	}
	entries, _ := os.ReadDir(c.dir)
	equipment, _ := os.ReadDir(a.champions.imageCache.dir)
	if len(entries) != 3 || len(equipment) != 0 {
		t.Fatalf("wrong cache directory: community=%d equipment=%d", len(entries), len(equipment))
	}
}

func TestR92DefaultPersonalConcurrency(t *testing.T) {
	t.Setenv("DEEP_LEGENDS_RIOT_MATCH_CONCURRENCY", "")
	t.Setenv("DEEP_LEGENDS_RIOT_KEY_TIER", "")
	if configuredRiotMatchConcurrency() != 8 {
		t.Fatal("default personal concurrency must be eight")
	}
}

func TestR92RiotMatchByteBudgetAcrossRestart(t *testing.T) {
	p := newChampionProvider()
	p.cache = newChampionDataCache(&localStore{root: t.TempDir()})
	c := newRiotMatchDiskCache(p)
	// Base64 + envelope puts 48 two-MiB bodies just over the real 128-MiB
	// limit, well below 600 entries. This isolates byte eviction from count.
	data := bytes.Repeat([]byte("x"), 2<<20)
	check := func() {
		files, err := os.ReadDir(c.dir)
		if err != nil {
			t.Fatal(err)
		}
		var size int64
		for _, f := range files {
			info, err := f.Info()
			if err != nil {
				t.Fatal(err)
			}
			size += info.Size()
		}
		if size > 128<<20 {
			t.Fatalf("riot-match byte limit exceeded: %d", size)
		}
	}
	for i := 0; i < 48; i++ {
		if err := c.writeDisk(championCacheEnvelope{Key: fmt.Sprintf("riot-match-v1|KR_%d", i+1), Data: data}); err != nil {
			t.Fatal(err)
		}
	}
	check()
	c = newRiotMatchDiskCache(p)
	for i := 48; i < 50; i++ {
		if err := c.writeDisk(championCacheEnvelope{Key: fmt.Sprintf("riot-match-v1|KR_%d", i+1), Data: data}); err != nil {
			t.Fatal(err)
		}
	}
	check()
}
