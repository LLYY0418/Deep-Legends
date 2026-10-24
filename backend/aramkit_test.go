package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func r307Fixture(t *testing.T, name string, out any) []byte {
	t.Helper()
	b, e := os.ReadFile("testdata/r307/" + name + ".json")
	if e != nil {
		t.Fatal(e)
	}
	if out != nil {
		if e = json.Unmarshal(b, out); e != nil {
			t.Fatal(e)
		}
	}
	return b
}
func r307Resources(t *testing.T) aramkitResources {
	t.Helper()
	r := aramkitResources{Version: aramkitVersion{Version: "16.21"}, Metadata: map[int]championMetadata{}}
	var directory map[string]championMetadata
	r307Fixture(t, "champion-directory", &directory)
	for key, m := range directory {
		var id int
		fmt.Sscan(key, &id)
		m.ID = id
		r.Metadata[id] = m
	}
	r307Fixture(t, "champion-identities", &r.Champions)
	r307Fixture(t, "augment-resources", &r.Augments)
	r307Fixture(t, "item-resources", &r.Items)
	r307Fixture(t, "spell-resources", &r.Spells)
	r307Fixture(t, "archetypes", &r.Archetypes)
	return r
}
func TestR307ARAMKitChampionIdentity(t *testing.T) {
	r := r307Resources(t)
	valid, cases, bad := verifyARAMKitChampions(r.Champions, r.Metadata)
	if len(valid) != 173 || len(bad) != 0 || len(cases) != 1 || cases[0] != 9 {
		t.Fatalf("valid=%d case=%v mismatch=%v", len(valid), cases, bad)
	}
	if valid["9"].RiotID != "FiddleSticks" || r.Metadata[9].Key != "Fiddlesticks" {
		t.Fatal("real ID 9 case fixture missing")
	}
	row := r.Champions["9"]
	row.RiotID = "Annie"
	r.Champions["9"] = row
	valid, _, bad = verifyARAMKitChampions(r.Champions, r.Metadata)
	if _, ok := valid["9"]; ok || len(bad) != 1 || bad[0] != 9 {
		t.Fatal("genuine mismatch retained")
	}
}
func TestR307ARAMKitRealDetails(t *testing.T) {
	r := r307Resources(t)
	for _, id := range []int{157, 421} {
		b := r307Fixture(t, fmt.Sprintf("champion-%d", id), nil)
		detail, counts, e := parseARAMKitDetail(b, id, r)
		if e != nil {
			t.Fatal(e)
		}
		if detail.Patch != "26.21" || detail.Stats.Games <= 0 || len(detail.ItemRanking) == 0 || len(detail.RecommendedAugments) == 0 || len(detail.Build.SummonerSpells) == 0 || len(detail.Build.Skills) == 0 || len(detail.Build.CoreItems) == 0 {
			t.Fatalf("incomplete %d counts=%v", id, counts)
		}
		if detail.ARAMKit.Scopes["filtered"].Slots["1"] == nil || detail.ARAMKit.Scopes["unfiltered"].Slots["6"] == nil {
			t.Fatal("scopes/slots lost")
		}
		if id == 157 {
			if detail.Stats.WinRate != 58.48 || detail.AugmentCombinations[0].Games != 12105 || detail.AugmentCombinations[0].WinRate != 74.88 {
				t.Fatal("real values changed")
			}
		}
	}
}
func TestR307ARAMKitMissingAndUnknownFields(t *testing.T) {
	r := r307Resources(t)
	for _, body := range []string{`{}`, `{"champion":{"id":157,"stats":{"sampleCount":0}}}`, `{"champion":{"id":421,"stats":{"sampleCount":1}}}`} {
		if _, _, e := parseARAMKitDetail([]byte(body), 157, r); e == nil {
			t.Fatal("accepted missing or mismatched champion")
		}
	}
	body := []byte(`{"extra":true,"champion":{"id":157,"tier":"S","stats":{"sampleCount":2,"winRate":0.5}},"augments":{"all":[]}}`)
	d, _, e := parseARAMKitDetail(body, 157, r)
	if e != nil || len(d.RecommendedAugments) != 0 || d.Stats.WinRate != 50 {
		t.Fatal("unknown field/empty rows handling")
	}
}
func TestR307ARAMKitCombos(t *testing.T) {
	r := r307Resources(t)
	var raw aramkitRawDetail
	r307Fixture(t, "champion-157", &raw)
	good := raw.AugmentCombinations[0]
	bad := good
	bad.AugmentIDs = []int{good.AugmentIDs[0], 999999}
	short := good
	short.AugmentIDs = short.AugmentIDs[:1]
	low := good
	low.SampleCount = 199
	rows, dropped := r.combos([]aramkitRawCombo{good, bad, short, low})
	if len(rows) != 1 || dropped != 2 || len(rows[0].Assets) != 2 {
		t.Fatal(rows, dropped)
	}
}
func TestR307ARAMKitPatchAndBalance(t *testing.T) {
	if aramkitPatch("16.21") != "26.21" || aramkitPatch("16.20") != "26.20" || aramkitPatch("bad") != "" {
		t.Fatal("patch mapping")
	}
	line := "冷却时间改为 14 / 13 / 12 / 11 / 10（原为 10 / 9.5 / 9 / 8.5 / 8）。"
	b := aramkitBalance{AbilityChanges: []aramkitAbilityChange{{Key: "Q", Lines: []string{line}}}}
	for _, key := range []string{"damageDealt", "damageTaken", "abilityHaste", "tenacity", "healing", "attackSpeed", "shielding", "energyRegen", "unknown"} {
		b.Modifiers = append(b.Modifiers, aramkitModifier{Key: key, Unit: "percent", Value: -.1})
	}
	result, dropped := filteredARAMKitBalance(b)
	if len(result.Modifiers) != 8 || dropped != 1 || result.AbilityChanges[0].Lines[0] != line {
		t.Fatal("balance dropped or rewritten")
	}
}

type r307Transport func(*http.Request) (*http.Response, error)

func (f r307Transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestR307ARAMKitImmutableCacheAndConcurrency(t *testing.T) {
	p := newChampionProvider()
	var mu sync.Mutex
	calls, active, peak := 0, 0, 0
	p.client = &http.Client{Transport: r307Transport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("User-Agent") != "Deep-Legends/"+version {
			t.Error("missing UA")
		}
		mu.Lock()
		calls++
		active++
		if active > peak {
			peak = active
		}
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
		mu.Lock()
		active--
		mu.Unlock()
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
	})}
	c := p.aramkitClient()
	c.dir = t.TempDir()
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := c.immutable(context.Background(), fmt.Sprintf("data/build-%d/test.json", i%3))
			if e != nil {
				t.Error(e)
			}
		}(i)
	}
	wg.Wait()
	if calls != 3 || peak > 2 {
		t.Fatalf("calls=%d peak=%d", calls, peak)
	}
	c.memory = map[string][]byte{}
	if _, e := c.immutable(context.Background(), "data/build-0/test.json"); e != nil || calls != 3 {
		t.Fatal("disk hit requested network")
	}
	if _, e := c.immutable(context.Background(), "data/new-build/test.json"); e != nil || calls != 4 {
		t.Fatal("new dataPath not read")
	}
}

func TestR307ARAMKitVersionRevalidationOfflineAndBackoff(t *testing.T) {
	p := newChampionProvider()
	calls := 0
	offline := false
	build := "first"
	p.client = &http.Client{Transport: r307Transport(func(req *http.Request) (*http.Response, error) {
		calls++
		if offline {
			return nil, fmt.Errorf("offline")
		}
		if req.Header.Get("If-None-Match") == `"first"` && build == "first" {
			return &http.Response{StatusCode: 304, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		payload := fmt.Sprintf(`{"latest":"16.21","versions":[{"version":"16.21","dataPath":"data/%s","resourcePath":"resources/meta"}]}`, build)
		return &http.Response{StatusCode: 200, Header: http.Header{"Etag": {`"` + build + `"`}}, Body: io.NopCloser(strings.NewReader(payload))}, nil
	})}
	c := p.aramkitClient()
	c.dir = t.TempDir()
	v, e := c.current(t.Context())
	if e != nil || v.DataPath != "data/first" {
		t.Fatal(v, e)
	}
	_, e = c.current(t.Context())
	if e != nil || calls != 1 {
		t.Fatal("3-hour refresh policy")
	}
	c.checked = time.Time{}
	_, e = c.current(t.Context())
	if e != nil || calls != 2 {
		t.Fatal("conditional validation")
	}
	build = "second"
	c.checked = time.Time{}
	v, e = c.current(t.Context())
	if e != nil || v.DataPath != "data/second" {
		t.Fatal("dataPath switch", v, e)
	}
	offline = true
	c.checked = time.Time{}
	v, e = c.current(t.Context())
	if e != nil || v.DataPath != "data/second" {
		t.Fatal("offline last valid versions", v, e)
	}
	before := calls
	_, _ = c.current(t.Context())
	if calls != before {
		t.Fatal("failure backoff ignored")
	}
	fresh := newChampionProvider().aramkitClient()
	fresh.dir = c.dir
	data, e := os.ReadFile(c.dir + "/versions-cache.json")
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(data, &fresh.versions); e != nil {
		t.Fatal(e)
	}
	fresh.retry["versions"] = time.Now().Add(time.Minute)
	v, e = fresh.current(t.Context())
	if e != nil || v.DataPath != "data/second" {
		t.Fatal("disk versions lost")
	}
}
func TestR307ARAMKitRatingHedgeDelayCancelAndRetry(t *testing.T) {
	primaryStarted := make(chan struct{})
	cancelled := make(chan struct{})
	var mu sync.Mutex
	var requests []string
	started := time.Now()
	client := newAramkitTestClient(r307Transport(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		requests = append(requests, req.URL.Host)
		mu.Unlock()
		if req.URL.Host == "primary.test" {
			close(primaryStarted)
			<-req.Context().Done()
			close(cancelled)
			return nil, req.Context().Err()
		}
		if time.Since(started) < 25*time.Millisecond {
			t.Error("backup started before hedge delay")
		}
		return aramkitJSONResponse(200, `{"code":200,"data":{"rating":2000}}`), nil
	}))
	client.baseURL = "https://primary.test"
	client.fallbackURL = "https://backup.test"
	client.hedgeDelay = 30 * time.Millisecond
	client.totalTimeout = time.Second
	result := client.lookup(t.Context(), "synthetic-fixture", "0000")
	if !result.Available {
		t.Fatal("backup did not win")
	}
	<-primaryStarted
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("loser not cancelled")
	}
	mu.Lock()
	count := len(requests)
	mu.Unlock()
	client.lookup(t.Context(), "synthetic-fixture", "0000")
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != count || count != 2 {
		t.Fatal("cache requested network", requests)
	}
	if aramkitRatingFailureTTL != 2*time.Minute || aramkitRatingSuccessTTL != 24*time.Hour || aramkitRatingTotalTimeout > 25*time.Second {
		t.Fatal("cache/timeout budget")
	}
}
func TestR307ARAMKitRatingImmediateFailureFallsBack(t *testing.T) {
	client := newAramkitTestClient(r307Transport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "primary.test" {
			return nil, fmt.Errorf("offline")
		}
		return aramkitJSONResponse(200, `{"code":200,"data":{"rating":2100}}`), nil
	}))
	client.baseURL = "https://primary.test"
	client.fallbackURL = "https://backup.test"
	client.hedgeDelay = time.Hour
	client.totalTimeout = time.Second
	if !client.lookup(t.Context(), "synthetic-fixture", "0000").Available {
		t.Fatal("failure did not trigger fallback")
	}
}
func TestR307ARAMKitRatingTracePrivacyAndRetryWindow(t *testing.T) {
	now := time.Now()
	calls := 0
	var event map[string]any
	client := newAramkitTestClient(r307Transport(func(req *http.Request) (*http.Response, error) {
		calls++
		trace := httptrace.ContextClientTrace(req.Context())
		trace.DNSStart(httptrace.DNSStartInfo{})
		trace.DNSDone(httptrace.DNSDoneInfo{})
		trace.ConnectStart("tcp", "fixture")
		trace.ConnectDone("tcp", "fixture", nil)
		trace.TLSHandshakeStart()
		trace.TLSHandshakeDone(tls.ConnectionState{}, nil)
		trace.GotFirstResponseByte()
		return aramkitJSONResponse(503, `{}`), nil
	}))
	client.now = func() time.Time { return now }
	client.observe = func(e map[string]any) { event = e }
	client.lookup(t.Context(), "synthetic-fixture", "0000")
	client.lookup(t.Context(), "synthetic-fixture", "0000")
	if calls != 1 {
		t.Fatal("negative cache missed")
	}
	now = now.Add(2*time.Minute + time.Millisecond)
	client.lookup(t.Context(), "synthetic-fixture", "0000")
	if calls != 2 {
		t.Fatal("automatic retry still blocked")
	}
	for _, key := range []string{"host", "dns_ms", "connect_ms", "tls_ms", "ttfb_ms", "player_hash"} {
		if _, ok := event[key]; !ok {
			t.Fatal("missing diagnostic", key)
		}
	}
	b, _ := json.Marshal(event)
	if strings.Contains(string(b), "synthetic-fixture") {
		t.Fatal("identity leaked")
	}
}

func TestR307ARAMKitCompletePipelineReusesFilesAndNoRetiredRequests(t *testing.T) {
	p := newChampionProvider()
	r := r307Resources(t)
	p.championMeta = r.Metadata
	p.championIDs = map[string]int{"yasuo": 157}
	var versions aramkitVersions
	r307Fixture(t, "versions", &versions)
	v, _ := selectARAMKitVersion(versions)
	files := map[string][]byte{"/data/versions.json": r307Fixture(t, "versions", nil)}
	for _, item := range []struct{ remote, fixture string }{{"champions", "champion-identities"}, {"augments", "augment-resources"}, {"items", "item-resources"}, {"summoner-spells", "spell-resources"}, {"archetypes", "archetypes"}} {
		files["/"+v.ResourcePath+"/zh-CN/resources/"+item.remote+".json"] = r307Fixture(t, item.fixture, nil)
	}
	for _, item := range []struct{ remote, fixture string }{{"champion-rankings", "champion-rankings"}, {"augment-rankings", "augment-rankings"}, {"augment-rarity", "augment-rarity"}, {"champion-details/157", "champion-157"}, {"augment-details/1134", "augment-1134"}} {
		files["/"+v.DataPath+"/stats/all/"+item.remote+".json"] = r307Fixture(t, item.fixture, nil)
	}
	calls := map[string]int{}
	p.client = &http.Client{Transport: r307Transport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != aramkitDataCN {
			t.Fatalf("unexpected host: %s", req.URL.Host)
		}
		b, ok := files[req.URL.Path]
		if !ok {
			t.Fatalf("unexpected request: %s", req.URL.Path)
		}
		calls[req.URL.Path]++
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(b)))}, nil
	})}
	p.aramkitClient().dir = t.TempDir()
	ranking, e := p.loadARAMKitRankings(t.Context())
	if e != nil || len(ranking.Rows) != 10 || ranking.Rows[0].ChampionID != 157 || ranking.Rows[0].Grade != "S" {
		t.Fatal(ranking, e)
	}
	detail, e := p.loadMayhemDetail(t.Context(), "yasuo")
	if e != nil || detail.Source != "ARAMKit" {
		t.Fatal(e)
	}
	resolution := gameplayRecommendationModeResolution{InternalMode: "hextech-aram"}
	bundle := gameplayRecommendationsFromResolvedDetail(157, "", detail, resolution)
	if len(bundle.AugmentCombinations) == 0 || len(bundle.Build.CoreOptions) == 0 || len(bundle.Build.CoreOptions[0].IDs) != 3 {
		t.Fatal("live data lost")
	}
	catalog, e := p.loadAugments(t.Context())
	if e != nil || catalog.Source != "ARAMKit" || len(catalog.Rows) == 0 {
		t.Fatal(catalog, e)
	}
	augment, e := p.loadCurrentAugmentDetail(t.Context(), 1134)
	if e != nil || len(augment.Champions) == 0 || augment.Champions[0].ID != 360 || augment.Champions[0].Name != aramkitChampionDisplayName(r.Metadata[360]) {
		t.Fatal(augment, e)
	}
	if delta := augment.Champions[0].DeltaWinRate; delta < 17.06 || delta > 17.08 {
		t.Fatal("delta not percentage points", delta)
	}
	rarity, e := p.loadARAMKitRarity(t.Context())
	if e != nil || len(rarity.Stages) != 4 {
		t.Fatal(rarity, e)
	}
	_, _ = p.loadMayhemDetail(t.Context(), "yasuo")
	for name, n := range calls {
		if n != 1 {
			t.Fatalf("repeated immutable read %s x%d", name, n)
		}
	}
}

func TestR307ARAMKitOfflineWithoutCacheHidesCards(t *testing.T) {
	p := newChampionProvider()
	r := r307Resources(t)
	p.championMeta = r.Metadata
	p.championIDs = map[string]int{"yasuo": 157}
	p.client = &http.Client{Transport: r307Transport(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Host, "hex"+"data") {
			t.Fatal("retired request")
		}
		return nil, fmt.Errorf("offline")
	})}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	detail, e := p.loadMayhemDetail(ctx, "yasuo")
	if e != nil || detail.Source != "none" || len(detail.RecommendedAugments) != 0 || len(detail.ItemRanking) != 0 || len(detail.Build.CoreItems) != 0 {
		t.Fatal("offline cards not hidden", detail, e)
	}
}

func TestR307ARAMKitNetworkTargetsAndAssets(t *testing.T) {
	targets := systemProxyTargets()
	for _, host := range []string{aramkitDataCN, aramkitDataCOM, aramkitRatingCNHost, aramkitRatingHost} {
		if targets[host] != "https://"+host+"/" || networkGroup(host) != "aramkit" {
			t.Fatal("missing independent target", host)
		}
	}
	for _, host := range []string{aramkitDataCN, aramkitDataCOM} {
		if !allowedChampionHost(host) {
			t.Fatal("host rejected", host)
		}
	}
	host, ok := validateChampionAssetPath("aramkit", "/assets/augment/1134-hash.webp")
	if !ok || host != aramkitDataCN {
		t.Fatal("asset proxy path")
	}
	for _, bad := range []string{"/assets/../secret.webp", "/other/icon.webp", "/assets/icon.webp?key=secret"} {
		if _, ok := validateChampionAssetPath("aramkit", bad); ok {
			t.Fatal("unsafe asset accepted")
		}
	}
}
