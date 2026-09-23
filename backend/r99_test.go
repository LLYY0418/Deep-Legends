package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func r99Client(t *testing.T, handler http.HandlerFunc) *LCUClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &LCUClient{baseURL: server.URL, token: "fixture", http: server.Client()}
}
func TestR99ProbeReadMatrixAndPrivacy(t *testing.T) {
	var mu sync.Mutex
	paths := []string{}
	writes := 0
	client := r99Client(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		if r.Method != "GET" {
			writes++
		}
		mu.Unlock()
		if strings.Contains(r.URL.Path, "/inventory/") && !strings.Contains(r.URL.Path, "SUMMONER_ICON") {
			fmt.Fprint(w, `{"SECRET-INVENTORY-KEY":{"puuid":"SECRET","summonerId":4,"accountId":5,"uuid":"SECRET","contentId":"SECRET","itemId":3,"purchaseDate":"SECRET","gameName":"SECRET","isOwned":true}}`)
		} else {
			fmt.Fprint(w, `[{"title":"Title","puuid":"SECRET","summonerId":4,"accountId":5,"uuid":"SECRET","contentId":"SECRET","itemId":3,"purchaseDate":"SECRET","gameName":"SECRET","owned":true}]`)
		}
	})
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0700); err != nil {
		t.Fatal(err)
	}
	a := &app{storage: trackTestStore(t, &localStore{root: root})}
	a.recordR99SurfaceShape(context.Background(), client)
	want := []string{
		"/lol-game-data/assets/v1/summoner-icons.json", "/lol-game-data/assets/v1/summoner-icon-sets.json", "/lol-game-data/assets/v1/regalia.json", "/lol-inventory/v2/inventory/SUMMONER_ICON", "/lol-regalia/v3/inventory/REGALIA_BANNER", "/lol-regalia/v3/inventory/REGALIA_CREST", "/lol-regalia/v3/inventory/REGALIA_BORDER", "/lol-inventory/v2/inventory/REGALIA_BANNER", "/lol-loadouts/v4/loadouts/scope/account", "/lol-regalia/v2/current-summoner/regalia", "/lol-banners/v1/current-summoner/flags", "/lol-banners/v1/current-summoner/flags/equipped", "/lol-banners/v1/current-summoner/frames/equipped"}
	want = append(append([]string{}, want[:6]...), want[8:10]...)
	sort.Strings(want)
	sort.Strings(paths)
	if !reflect.DeepEqual(paths, want) || writes != 0 {
		t.Fatalf("matrix=%v writes=%d", paths, writes)
	}
	raw, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"puuid", "summonerId", "accountId", "uuid", "contentId", "itemId", "purchaseDate", "gameName", "SECRET"} {
		if strings.Contains(string(raw), bad) {
			t.Fatalf("probe leaked %s: %s", bad, raw)
		}
	}
	if strings.Count(string(raw), `"event":"r99_read_probe"`) != 8 {
		t.Fatalf("missing probe event: %s", raw)
	}
}
func TestR99ConnectionNeverWritesAndProbesOnce(t *testing.T) {
	var writes, reads atomic.Int32
	client := r99Client(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			writes.Add(1)
		}
		if r.URL.Path == r99ReadPaths[3] {
			reads.Add(1)
		}
		if r.URL.Path == "/lol-summoner/v1/current-summoner" {
			fmt.Fprint(w, `{"profileIconId":71}`)
		} else {
			fmt.Fprint(w, `{}`)
		}
	})
	a := &app{connected: true, lcu: client, summoner: Summoner{SummonerID: 1}}
	a.loadFacadeState(context.Background())
	a.loadFacadeState(context.Background())
	if writes.Load() != 0 || reads.Load() != 1 {
		t.Fatalf("writes=%d probes=%d", writes.Load(), reads.Load())
	}
}

// R101 replaces the no-op W1-W3 and rejected W4 probes with W5/W6 restoration tests.
const r99IconsFixture = `[{"id":71,"title":"星之守护者","yearReleased":2026,"imagePath":"/lol-game-data/assets/v1/profile-icons/71.jpg","rarities":[{"region":"tencent"}],"uuid":"SECRET","contentId":"SECRET"},{"id":72,"title":"停用","yearReleased":2020,"imagePath":"/lol-game-data/assets/v1/profile-icons/72.jpg","disabledRegions":["TENCENT"]},{"id":73,"title":"空图","imagePath":""}]`

func TestR99IconCatalogProjectionCachingAndClientSwitch(t *testing.T) {
	var gets, terms atomic.Int32
	handler := func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case r99ReadPaths[0]:
			gets.Add(1)
			fmt.Fprint(w, r99IconsFixture)
		case r99ReadPaths[1]:
			fmt.Fprint(w, `[{"displayName":"星之守护者","icons":[71]}]`)
		default:
			http.Error(w, `{"owned":true,"uuid":"SECRET","purchaseDate":"SECRET","ownershipType":"SECRET","contentId":"SECRET"}`, 500)
		}
	}
	client := r99Client(t, handler)
	a := &app{connected: true, lcu: client, summoner: Summoner{SummonerID: 1}}
	a.facadeIcons.terms = func(s string) []string { terms.Add(1); return championPinyinTerms(s) }
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		a.handleFacadeIcons(w, httptest.NewRequest("GET", "/api/facade/icons", nil))
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		for _, bad := range []string{"uuid", "purchaseDate", "ownershipType", "contentId", "imagePath", "SECRET"} {
			if strings.Contains(w.Body.String(), bad) {
				t.Fatalf("leaked %s", bad)
			}
		}
		var body struct {
			Icons       []facadeIcon `json:"icons"`
			Unavailable bool         `json:"iconOwnershipUnavailable"`
		}
		json.Unmarshal(w.Body.Bytes(), &body)
		if !body.Unavailable || len(body.Icons) != 2 || body.Icons[0].Year != 2026 || len(body.Icons[0].Sets) != 1 || !body.Icons[1].Disabled {
			t.Fatalf("projection=%+v", body)
		}
	}
	if gets.Load() != 1 || terms.Load() != 2 {
		t.Fatalf("uncached terms=%d requests=%d", terms.Load(), gets.Load())
	}
	a.lcu = r99Client(t, handler)
	a.loadIconCatalog(context.Background(), a.lcu)
	if gets.Load() != 2 || terms.Load() != 4 {
		t.Fatal("client switch reused cache")
	}
}
func TestR99IconAndBannerApplyActionsRetired(t *testing.T) {
	var calls atomic.Int32
	client := r99Client(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1) })
	a := &app{connected: true, lcu: client, summoner: Summoner{SummonerID: 1}}
	a.facadeIcons = facadeIconCache{client: client, at: time.Now(), catalog: facadeIconCatalog{Icons: []facadeIcon{{ID: 71}}}}
	for _, body := range []string{`{"action":"icon","iconId":71}`, `{"action":"banner","bannerId":"4"}`} {
		w := httptest.NewRecorder()
		a.handleFacadeApply(w, httptest.NewRequest("POST", "/api/facade/apply", strings.NewReader(body)))
		if w.Code != 400 {
			t.Fatal(body, w.Code)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("retired actions reached the client")
	}
}
func TestR99SummonerEventStillUpdatesIdentity(t *testing.T) {
	client := &LCUClient{}
	a := &app{connected: true, lcu: client, summoner: Summoner{SummonerID: 1, ProfileIconID: 5}}
	event := LCUEvent{URI: "/lol-summoner/v1/current-summoner", Data: json.RawMessage(`{"summonerId":1,"profileIconId":71}`)}
	if a.handleFacadeLCUEvent(event) {
		t.Fatal("facade swallowed summoner identity event")
	}
	if !a.handleSummonerIdentityEvent(client, event.Data) {
		t.Fatal("identity rejected event")
	}
	defer a.clearFacadeEventThrottle()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		a.mu.RLock()
		id := a.summoner.ProfileIconID
		a.mu.RUnlock()
		if id == 71 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("identity never updated")
}
func TestR99RankBannerPreservesCrestAndRejectsUnknown(t *testing.T) {
	calls := 0
	client := r99Client(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method == "GET" {
			fmt.Fprint(w, `{"preferredCrestType":"ranked","selectedPrestigeCrest":7}`)
			return
		}
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		if r.Method != "PUT" || b["preferredCrestType"] != "ranked" || b["selectedPrestigeCrest"] != float64(7) || b["preferredBannerType"] != "blank" {
			t.Errorf("crest overwritten: %v", b)
		}
		w.WriteHeader(204)
	})
	if err := writeFacadeRankBanner(context.Background(), client, "hextech"); !errors.Is(err, errFacadeInvalid) || calls != 0 {
		t.Fatal("invalid banner made request")
	}
	if err := writeFacadeRankBanner(context.Background(), client, "blank"); err != nil || calls != 2 {
		t.Fatalf("write=%v calls=%d", err, calls)
	}
}

func r99SeedProvider(t *testing.T, root string, handler func(*http.Request) (*http.Response, error)) *riotProvider {
	t.Helper()
	t.Setenv("RIOT_API_KEY", "RGAPI-fixture")
	cp := newChampionProvider()
	cp.cache = newChampionDataCache(&localStore{root: root})
	cp.client = &http.Client{Transport: gameplayRoundTripFunc(handler)}
	return newRiotProvider(cp)
}
func r99Response(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}
func r99Rookie(result proPlayersResponse) proPlayer {
	for _, team := range result.Teams {
		if team.Code == "IG" {
			for _, player := range team.Players {
				if player.Name == "Rookie" {
					return player
				}
			}
		}
	}
	return proPlayer{}
}
func TestR99SeedRenameSurvivesRestartAndTTL(t *testing.T) {
	root := t.TempDir()
	seed := r102RookieSeed()
	byName, byStable := 0, 0
	handler := func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "by-riot-id") {
			byName++
			if byName > 1 {
				return nil, errors.New("old name no longer exists")
			}
			return r99Response(`{"puuid":"fixture-stable","gameName":"모든일은같이","tagLine":"KR1"}`), nil
		}
		byStable++
		return r99Response(`{"puuid":"fixture-stable","gameName":"Renamed","tagLine":"KR2"}`), nil
	}
	p := r99SeedProvider(t, root, handler)
	account, err := p.resolveProSeed(context.Background(), seed, 0)
	if err != nil || account.PUUID != "fixture-stable" {
		t.Fatal(err)
	}
	p = r99SeedProvider(t, root, handler)
	clock := time.Now().Add(29 * 24 * time.Hour)
	p.identityDisk.now = func() time.Time { return clock }
	account, err = p.resolveProSeed(context.Background(), seed, 0)
	if err != nil || account.GameName != "Renamed" || byName != 1 || byStable != 1 {
		t.Fatalf("restart=%+v %v name=%d stable=%d", account, err, byName, byStable)
	}
	_, err = p.resolveProSeed(context.Background(), seed, 0)
	if err != nil || byStable != 1 {
		t.Fatal("6h cache not used")
	}
	clock = clock.Add(6*time.Hour + time.Second)
	_, err = p.resolveProSeed(context.Background(), seed, 0)
	if err != nil || byStable != 2 {
		t.Fatalf("6h cache not expired: %v %d", err, byStable)
	}
}
func TestR99SeedAnchorSurvivesLRUAndHasOnlyStableField(t *testing.T) {
	root := t.TempDir()
	p := r99SeedProvider(t, root, func(*http.Request) (*http.Response, error) {
		return r99Response(`{"puuid":"fixture-stable","gameName":"Fixture","tagLine":"KR1"}`), nil
	})
	_, err := p.resolveProSeed(context.Background(), r102RookieSeed(), 0)
	if err != nil {
		t.Fatal(err)
	}
	paths, _ := filepath.Glob(filepath.Join(root, "riot-identities", "proseed-*.json"))
	if len(paths) != 1 {
		t.Fatal(paths)
	}
	var envelope championCacheEnvelope
	raw, _ := os.ReadFile(paths[0])
	json.Unmarshal(raw, &envelope)
	var body map[string]any
	json.Unmarshal(envelope.Data, &body)
	if len(body) != 1 || body["puuid"] != "fixture-stable" {
		t.Fatal(body)
	}
	for i := 0; i < 1030; i++ {
		value := map[string]int{"n": i}
		if err := p.cachedPublicIdentity(context.Background(), fmt.Sprintf("other:%d", i), time.Hour, &value, func(context.Context) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(paths[0]); err != nil {
		t.Fatal("anchor evicted", err)
	}
	if len(p.identityDisk.strictEntries) > 1024 {
		t.Fatal("cache budget grew")
	}
}
func TestR99SeedAppearsMergesAndNeverLeaks(t *testing.T) {
	p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/league/") {
			return r99Response(`[]`), nil
		}
		return r99Response(`{"puuid":"fixture-stable","gameName":"Renamed","tagLine":"KR1"}`), nil
	})
	a := &app{riot: p}
	seeds := a.loadProSeedAccounts(context.Background(), nil, []proSeedAccount{r102RookieSeed()})
	rows := withProSeed(nil, seeds)
	result := a.buildProPlayers(rows, proRoster)
	rookie := r99Rookie(result)
	if len(rookie.Accounts) != 1 || rookie.Status == "missing" || rookie.Accounts[0].Source != "人工核对" || rookie.Accounts[0].Confidence != "" || result.Partial {
		t.Fatalf("seed missing: %+v", rookie)
	}
	upstream := cloneProTeams(seeds)
	upstream[0].Members[0].Summoners[0].GameName = "Old"
	upstream[0].Members[0].Summoners[0].Source = ""
	upstream[0].Members[0].Summoners[0].UpdatedAt = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	result = a.buildProPlayers(withProSeed(upstream, seeds), proRoster)
	rookie = r99Rookie(result)
	if len(rookie.Accounts) != 1 || rookie.Accounts[0].GameName != "Renamed" {
		t.Fatalf("merge: %+v", rookie)
	}
	a.proPlayers.teams = rows
	a.proPlayers.fetchedAt = time.Now()
	w := httptest.NewRecorder()
	a.handleProPlayers(w, httptest.NewRequest("GET", "/api/pro-players", nil))
	for _, bad := range []string{"puuid", "fixture-stable"} {
		if strings.Contains(w.Body.String(), bad) {
			t.Fatalf("leaked %s", bad)
		}
	}
}
func TestR99SeedRespectsQuotaBudget(t *testing.T) {
	var calls atomic.Int32
	p := r99SeedProvider(t, t.TempDir(), func(*http.Request) (*http.Response, error) { calls.Add(1); return nil, errors.New("must not request") })
	p.limitSleep = func(context.Context, time.Duration) error {
		t.Error("seed waited beyond queue budget")
		return context.DeadlineExceeded
	}
	for i := 0; i < 15; i++ {
		p.shortWindow = append(p.shortWindow, time.Now())
	}
	previous := []opggProTeam{{ID: 371, Members: []opggProMember{{Nickname: "Rookie", Summoners: []opggProAccount{{Source: "seed", PUUID: "fixture-stable", GameName: "Previous", TagLine: "KR1"}}}}}}
	started := time.Now()
	rows := (&app{riot: p}).loadProSeedAccounts(context.Background(), previous, []proSeedAccount{r102RookieSeed()})
	if time.Since(started) > 300*time.Millisecond || calls.Load() != 0 || len(rows) != 1 || rows[0].Members[0].Summoners[0].GameName != "Previous" {
		t.Fatal("quota blocked or lost snapshot")
	}
}
func TestR99SeedSourceHasNoCopiedStableID(t *testing.T) {
	source := os.Getenv("R99_SEED_SOURCE")
	if source == "" {
		source = "pro_seed_accounts.go"
	}
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`[A-Za-z0-9_-]{40,}`).Match(raw) {
		t.Fatal("copied opaque ID in seed source")
	}
}

func TestR99SeedPresentInEveryPipelinePublication(t *testing.T) {
	releases := map[string]chan struct{}{}
	for _, name := range proSupplementPlayers {
		releases[name] = make(chan struct{})
	}
	closedFirst := false
	var releaseOnce sync.Once
	releaseAll := func() {
		releaseOnce.Do(func() {
			for name, ch := range releases {
				if name != "TheShy" || !closedFirst {
					close(ch)
				}
			}
		})
	}
	defer releaseAll()
	p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/ids") {
			return r99Response(`[]`), nil
		}
		if r.URL.Host == riotClusterHost {
			return r102AccountResponse(r), nil
		}
		if strings.Contains(r.URL.Path, "/league/") {
			return r99Response(`[]`), nil
		}
		if r.URL.Host == proSupplementHost {
			name := strings.TrimPrefix(r.URL.Path, "/player/")
			select {
			case <-releases[name]:
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
			_, player, _ := proSupplementPlayer(name)
			return proHTTPBody([]byte(fmt.Sprintf(`<h1>%s</h1><table><tr><td>Name</td><td>%s</td></tr></table><div><h4>Accounts</h4><table><tr><td>[KR] fixture-%s#KR1</td><td>Unranked</td></tr></table></div>`, name, player.Names[0], name))), nil
		}
		if strings.Contains(r.URL.Path, "pro-gamer") {
			return proHTTPBody(proFixtureHTML("kr")), nil
		}
		return nil, errors.New("no fixture ladder")
	})
	a := &app{riot: p, champions: p.champions}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	rows, _, err := a.loadProPlayers(ctx, false)
	if err != nil || len(r99Rookie(a.buildProPlayers(rows, proRoster)).Accounts) != 4 {
		t.Fatal("directory seed missing", err)
	}
	// Release one source only, observing its partial publication before completion.
	close(releases["TheShy"])
	closedFirst = true
	deadline := time.Now().Add(2 * time.Second)
	seen := false
	for time.Now().Before(deadline) {
		a.proPlayers.mu.Lock()
		snapshot := cloneProTeams(a.proPlayers.teams)
		a.proPlayers.mu.Unlock()
		for _, team := range snapshot {
			for _, member := range team.Members {
				if member.Nickname == "TheShy" && member.Supplement && !member.Incomplete {
					seen = true
				}
			}
		}
		if seen {
			if len(r99Rookie(a.buildProPlayers(snapshot, proRoster)).Accounts) != 4 {
				t.Fatal("partial publication lost seed")
			}
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !seen {
		t.Fatal("partial fixture did not publish")
	}
	releaseAll()
	waitProEnrichment(t, a)
	a.proPlayers.mu.Lock()
	snapshot := cloneProTeams(a.proPlayers.teams)
	a.proPlayers.mu.Unlock()
	result := a.buildProPlayers(snapshot, proRoster)
	if result.Partial || len(r99Rookie(result).Accounts) != 4 {
		t.Fatal("completed seed missing or partial")
	}
}

func TestR99ProbeHandlesAlternativeInventoryShape(t *testing.T) {
	value := []any{map[string]any{"isOwned": true, "itemId": 3, "contentId": "secret"}}
	e := r99ReadShape(7, value)
	if e["json_type"] != "array" || e["count"] != 1 || e["owned_count"] != 1 {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(e)
	if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "itemId") {
		t.Fatal(string(raw))
	}
}

func TestR99SeedQuotaDiagnosticsNeverContainIdentity(t *testing.T) {
	p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{"60"}}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})
	var events []map[string]any
	p.champions.diag = func(e map[string]any) { events = append(events, e) }
	if _, err := p.resolveProSeed(context.Background(), r102RookieSeed(), 0); err == nil {
		t.Fatal("expected quota rejection")
	}
	if _, err := p.fetchAccountByPUUID(context.Background(), "SECRET-STABLE-ID"); err == nil {
		t.Fatal("expected quota rejection")
	}
	raw, _ := json.Marshal(events)
	if len(events) != 2 {
		t.Fatalf("missing 429 diagnostics: %s", raw)
	}
	for _, bad := range []string{"SECRET-STABLE-ID", r102RookieSeed().Accounts[0].GameName, url.PathEscape(r102RookieSeed().Accounts[0].GameName)} {
		if strings.Contains(string(raw), bad) {
			t.Fatalf("quota diagnostic leaked %s", bad)
		}
	}
}
