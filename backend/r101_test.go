package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func r101IconCatalogFixture(t *testing.T, fail bool) facadeIconCatalog {
	t.Helper()
	client := r99Client(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case r99ReadPaths[0]:
			rows := []map[string]any{}
			for i := 1; i <= 5099; i++ {
				rows = append(rows, map[string]any{"id": i, "title": fmt.Sprint(i), "imagePath": "image"})
			}
			json.NewEncoder(w).Encode(rows)
		case r99ReadPaths[1]:
			fmt.Fprint(w, `[]`)
		case r99ReadPaths[3]:
			if fail {
				w.WriteHeader(500)
				return
			}
			rows := []map[string]any{}
			for i := 1; i <= 508; i++ {
				rows = append(rows, map[string]any{"itemId": i, "owned": i <= 478, "uuid": "SECRET", "purchaseDate": "SECRET", "contentId": "SECRET", "ownershipType": "OWNED"})
			}
			json.NewEncoder(w).Encode(rows)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	})
	a := &app{connected: true, lcu: client, summoner: Summoner{SummonerID: 1}}
	w := httptest.NewRecorder()
	a.handleFacadeIcons(w, httptest.NewRequest("GET", "/api/facade/icons", nil))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	for _, bad := range []string{"SECRET", "uuid", "purchaseDate", "contentId", "ownershipType"} {
		if strings.Contains(w.Body.String(), bad) {
			t.Fatalf("leaked %s", bad)
		}
	}
	var catalog facadeIconCatalog
	json.Unmarshal(w.Body.Bytes(), &catalog)
	return catalog
}
func TestR101IconOwnershipParsedFromInventory(t *testing.T) {
	c := r101IconCatalogFixture(t, false)
	owned := 0
	for _, icon := range c.Icons {
		if icon.Owned {
			owned++
		}
	}
	if owned != 478 || c.OwnedCount != 478 || c.Total != 5099 || c.OwnershipUnavailable {
		t.Fatalf("owned=%d counts=%d/%d unknown=%v", owned, c.OwnedCount, c.Total, c.OwnershipUnavailable)
	}
}
func TestR101IconOwnershipStillFallsBackOpen(t *testing.T) {
	c := r101IconCatalogFixture(t, true)
	if !c.OwnershipUnavailable || len(c.Icons) != 5099 {
		t.Fatalf("unknown=%v len=%d", c.OwnershipUnavailable, len(c.Icons))
	}
}
func r101BannerCatalogFixture(t *testing.T) []facadeBanner {
	t.Helper()
	client := r99Client(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-game-data/assets/v1/regalia.json":
			rows := []facadeRegalia{{ID: "1", Type: "kBanner", Selectable: true}}
			for i := 0; i < 11; i++ {
				rows = append(rows, facadeRegalia{ID: "2", IDSecondary: fmt.Sprint(i), Type: "kBanner", Selectable: true})
			}
			for i := 3; i < 38; i++ {
				rows = append(rows, facadeRegalia{ID: fmt.Sprint(i), IDSecondary: "0", Type: "kBanner", Selectable: true, Name: fmt.Sprintf("旗帜%02d", i), TencentOnly: i < 6})
			}
			rows = append(rows, facadeRegalia{ID: "999", Type: "kNone", Selectable: true}, facadeRegalia{ID: "998", Type: "kBanner"})
			json.NewEncoder(w).Encode(rows)
		case "/lol-regalia/v3/inventory/REGALIA_BANNER":
			fmt.Fprint(w, `{"3":{"isOwned":true},"4":{"isOwned":true},"5":{"isOwned":true},"6":{"isOwned":true}}`)
		case "/lol-inventory/v2/inventory/REGALIA_BANNER":
			t.Error("R8 is not authoritative")
			fmt.Fprint(w, `[{"owned":false}]`)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	})
	b, unknown, err := loadFacadeBanners(context.Background(), client)
	if err != nil || unknown {
		t.Fatal(err, unknown)
	}
	return b
}
func TestR101BannerCatalogIs35AndIDsAreStrings(t *testing.T) {
	b := r101BannerCatalogFixture(t)
	if len(b) != 35 {
		t.Fatal(len(b))
	}
	raw, _ := json.Marshal(b)
	if !strings.Contains(string(raw), `"id":"3"`) {
		t.Fatal(string(raw))
	}
	local := 0
	for _, v := range b {
		if v.TencentOnly {
			local++
		}
	}
	if local != 3 {
		t.Fatal(local)
	}
}
func TestR101BannerOwnershipUsesRegaliaV3Only(t *testing.T) {
	n := 0
	for _, v := range r101BannerCatalogFixture(t) {
		if v.Owned {
			n++
		}
	}
	if n != 4 {
		t.Fatal(n)
	}
}
func TestR101PreviousBannerActionRemoved(t *testing.T) {
	calls := 0
	client := r99Client(t, func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(200) })
	a := &app{connected: true, lcu: client, summoner: Summoner{SummonerID: 1}}
	w := httptest.NewRecorder()
	a.handleFacadeApply(w, httptest.NewRequest("POST", "/api/facade/apply", strings.NewReader(`{"action":"previous-banner"}`)))
	if w.Code != 400 || calls != 0 || facadeDiagnosticAction("previous-banner") != "unknown" {
		t.Fatal(w.Code, calls)
	}
}
func TestR101ClearChallengesStillPreservesTitle(t *testing.T) {
	TestR64ClearChallengesPreservesTitle(t)
}

func TestR101ChampSelectDefaultsOn(t *testing.T) {
	if !defaultChampSelectSettings().Enabled {
		t.Fatal("fresh default is off")
	}
}
func TestR101ExistingProfileKeepsItsOwnSetting(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, convenienceSettingsFile), []byte(`{"schemaVersion":3,"champSelect":{"enabled":false}}`), 0600)
	if loadWatchSettings(trackTestStore(t, &localStore{root: root})).ChampSelect.Enabled {
		t.Fatal("saved false overwritten")
	}
}
func TestR101EnabledWithEmptySequencesSendsNothing(t *testing.T) {
	for _, group := range []string{"normal", "ranked", "aram", "arena"} {
		for _, side := range []string{"ban", "pick"} {
			t.Run(group+side, func(t *testing.T) {
				f := newR78ChampSelectFixture(t, group, side, 0, []int64{1, 2}, map[int64]champSelectGridSelectionStatus{1: {}, 2: {}})
				if group == "aram" {
					f.session.BenchEnabled = true
					f.session.BenchChampions = []lcuChampSelectBenchChampion{{ChampionID: 1}}
					f.runner.mu.Lock()
					f.runner.champSelect.benchFirstSeen[1] = time.Now().Add(-2 * time.Second)
					f.runner.mu.Unlock()
				}
				for _, enabled := range []bool{false, true} {
					settings := defaultWatchSettings()
					g := settings.ChampSelect.Groups[group]
					g.Ban.Enabled = enabled
					g.Pick.Enabled = enabled
					settings.ChampSelect.Groups[group] = g
					f.runner.apply(settings)
					var writes atomic.Int32
					transport := f.client.http.Transport
					f.client.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
						if r.Method == "PATCH" || r.Method == "POST" {
							writes.Add(1)
						}
						if r.URL.Path == "/lol-gameflow/v1/session" {
							return r99Response(`{}`), nil
						}
						if r.URL.Path == "/lol-gameflow/v1/gameflow-phase" {
							return r99Response(`"ChampSelect"`), nil
						}
						return transport.RoundTrip(r)
					})
					a := &app{watch: f.runner}
					a.primeWatchState(f.client)
					for _, phase := range []string{"PLANNING", "BAN_PICK", "FINALIZATION"} {
						f.session.Timer.Phase = phase
						f.runner.evaluateChampSelect(f.client, settings.ChampSelect)
					}
					f.runner.handleChampSelectPhase("InProgress")
					time.Sleep(25 * time.Millisecond)
					if writes.Load() != 0 || f.patches.Load() != 0 || f.benchSwaps.Load() != 0 {
						t.Fatal("empty pool wrote to LCU")
					}
					f.runner.handleChampSelectPhase("ChampSelect")
					f.client.http.Transport = transport
				}
			})
		}
	}
}

func r101SeedProvider(t *testing.T, entries string, count *int) *riotProvider {
	return r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) {
		*count++
		if strings.Contains(r.URL.Path, "/league/") {
			return r99Response(entries), nil
		}
		return r99Response(`{"puuid":"fixture-seed","gameName":"Seed","tagLine":"KR1"}`), nil
	})
}
func r101SeedAccount(t *testing.T, entries string) proAccount {
	t.Helper()
	count := 0
	p := r101SeedProvider(t, entries, &count)
	a := &app{riot: p}
	seeds := a.loadProSeedAccounts(context.Background(), nil, []proSeedAccount{r102RookieSeed()})
	rookie := r99Rookie(a.buildProPlayers(seeds, proRoster))
	if len(rookie.Accounts) != 1 {
		t.Fatalf("missing seed %+v", rookie)
	}
	return rookie.Accounts[0]
}

const r101SoloEntries = `[{"queueType":"RANKED_FLEX_SR","tier":"IRON","rank":"IV","leaguePoints":1},{"queueType":"RANKED_SOLO_5x5","tier":"CHALLENGER","rank":"I","leaguePoints":2490}]`

func TestR101SeedResolvesSoloQueueRank(t *testing.T) {
	a := r101SeedAccount(t, r101SoloEntries)
	if a.RankStatus != "ranked" || a.Tier != "CHALLENGER" || a.LP != 2490 || a.Division != 1 {
		t.Fatalf("%+v", a)
	}
}
func TestR101SeedRecentActivityBeforeOlderUpstream(t *testing.T) {
	count := 0
	a := &app{riot: r101SeedProvider(t, r101SoloEntries, &count)}
	seeds := a.loadProSeedAccounts(context.Background(), nil, []proSeedAccount{r102RookieSeed()})
	seeds[0].Members[0].Summoners[0].LastMatchAt = "2026-09-17T08:00:00Z"
	seeds[0].Members[0].Summoners[0].LastMatchAtKnown = true
	upstream := cloneProTeams(seeds)
	upstream[0].Members[0].Summoners = []opggProAccount{{LastMatchAt: "2026-09-16T08:00:00Z", LastMatchAtKnown: true, GameName: "Lower", TagLine: "KR1", Region: "kr", Rank: json.RawMessage(`{"tier":"DIAMOND","division":2,"lp":75}`)}}
	rookie := r99Rookie(a.buildProPlayers(withProSeed(upstream, seeds), proRoster))
	if len(rookie.Accounts) != 2 || rookie.Accounts[0].GameName != "Seed" {
		t.Fatalf("%+v", rookie)
	}
}
func TestR101SeedRankMapsRomanDivision(t *testing.T) {
	for _, tier := range []string{"DIAMOND", "MASTER", "GRANDMASTER", "CHALLENGER"} {
		a := r101SeedAccount(t, fmt.Sprintf(`[{"queueType":"RANKED_SOLO_5x5","tier":%q,"rank":"II","leaguePoints":75}]`, tier))
		want := 1
		if tier == "DIAMOND" {
			want = 2
		}
		if a.Division != want {
			t.Fatal(tier, a.Division)
		}
	}
}
func TestR101SeedWithoutSoloQueueStaysUnavailable(t *testing.T) {
	a := r101SeedAccount(t, `[{"queueType":"RANKED_FLEX_SR","tier":"MASTER","rank":"I","leaguePoints":999}]`)
	if a.RankStatus != "unavailable" {
		t.Fatal(a)
	}
}
func TestR104SeedRank24HourCacheSeparateFromOverview(t *testing.T) {
	count := 0
	p := r101SeedProvider(t, r101SoloEntries, &count)
	now := time.Now()
	p.identityDisk.now = func() time.Time { return now }
	_, err := p.proSeedRank(context.Background(), "seed")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(4 * time.Minute)
	p.proSeedRank(context.Background(), "seed")
	if count != 1 {
		t.Fatalf("seed cache expired after overview TTL: %d", count)
	}
	p.leagueEntries(context.Background(), "seed")
	if count != 2 {
		t.Fatal("overview cache namespace collided")
	}
	// R104 extends only the seed TTL; the overview cache stays independent.
	now = now.Add(23 * time.Hour)
	if _, err := p.proSeedRank(context.Background(), "seed"); err != nil || count != 2 {
		t.Fatalf("seed cache expired before 24 hours: %d %v", count, err)
	}
	now = now.Add(time.Hour)
	p.proSeedRank(context.Background(), "seed")
	if count != 3 {
		t.Fatal("seed cache did not expire")
	}
}
func TestR101SeedRankRespectsQuotaBudget(t *testing.T) {
	count := 0
	p := r101SeedProvider(t, r101SoloEntries, &count)
	a := &app{riot: p}
	previous := a.loadProSeedAccounts(context.Background(), nil, []proSeedAccount{r102RookieSeed()})
	p.resolveProSeed(context.Background(), r102RookieSeed(), 0)
	now := time.Now().Add(24*time.Hour + time.Second)
	p.identityDisk.now = func() time.Time { return now }
	p.fetchAccountByPUUID(context.Background(), "fixture-seed")
	p.shortWindow = nil
	for i := 0; i < 15; i++ {
		p.shortWindow = append(p.shortWindow, time.Now())
	}
	before := count
	started := time.Now()
	rows := a.loadProSeedAccounts(context.Background(), previous, []proSeedAccount{r102RookieSeed()})
	if count != before || time.Since(started) > time.Second || !reflect.DeepEqual(rows, previous) {
		t.Fatal("quota failure lost seed snapshot or issued HTTP", count, before)
	}
}

func TestR101IconOwnershipCacheIsClientScoped(t *testing.T) {
	client := func(owned int) *LCUClient {
		return r99Client(t, func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case r99ReadPaths[0]:
				fmt.Fprint(w, r99IconsFixture)
			case r99ReadPaths[1]:
				fmt.Fprint(w, `[]`)
			case r99ReadPaths[3]:
				fmt.Fprintf(w, `[{"itemId":%d,"owned":true}]`, owned)
			default:
				t.Errorf("unexpected %s", r.URL.Path)
			}
		})
	}
	a := &app{}
	one, err := a.loadIconCatalogResult(context.Background(), client(71))
	if err != nil {
		t.Fatal(err)
	}
	two, err := a.loadIconCatalogResult(context.Background(), client(72))
	if err != nil {
		t.Fatal(err)
	}
	if !one.Icons[0].Owned || one.Icons[1].Owned || two.Icons[0].Owned || !two.Icons[1].Owned {
		t.Fatal("ownership carried across clients")
	}
}
func TestR123FacadeIconAndBannerActionsRemoved(t *testing.T) {
	calls := 0
	client := r99Client(t, func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(200) })
	a := &app{connected: true, lcu: client, summoner: Summoner{SummonerID: 1}}
	for _, body := range []string{`{"action":"icon","iconId":72}`, `{"action":"banner","bannerId":"4"}`} {
		w := httptest.NewRecorder()
		a.handleFacadeApply(w, httptest.NewRequest("POST", "/api/facade/apply", strings.NewReader(body)))
		if w.Code != 400 || calls != 0 {
			t.Fatal(body, w.Code, calls)
		}
	}
	if facadeDiagnosticAction("icon") != "unknown" || facadeDiagnosticAction("banner") != "unknown" {
		t.Fatal("diagnostic still classifies retired actions")
	}
}
