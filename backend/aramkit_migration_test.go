package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
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

func TestARAMKitColdRankingBudgetAndDiskRestart(t *testing.T) {
	root := t.TempDir()
	p, calls := newARAMKitFixtureProvider(t, root)
	if _, e := p.loadARAMRankings(t.Context()); e != nil {
		t.Fatal(e)
	}
	var versions aramkitVersions
	r307Fixture(t, "versions", &versions)
	v, _ := selectARAMKitVersion(versions)
	expected := map[string]int{"/data/versions.json": 1, "/" + v.ResourcePath + "/zh-CN/resources/champions.json": 1, "/" + v.DataPath + "/stats/all/champion-rankings.json": 1}
	if !reflect.DeepEqual(calls, expected) {
		t.Fatalf("cold ranking request inventory: got %v want %v", calls, expected)
	}
	restarted, again := newARAMKitFixtureProvider(t, root)
	if _, e := restarted.loadARAMRankings(t.Context()); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(again, map[string]int{"/data/versions.json": 1}) {
		t.Fatalf("restart request inventory: %v", again)
	}
}
func TestARAMKitSameHeroTenConcurrentLoadsUseOneRequestAndRestartUsesNoImmutableRequests(t *testing.T) {
	root := t.TempDir()
	p, calls := newARAMKitFixtureProvider(t, root)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, e := p.loadMayhemDetail(t.Context(), "157")
			if e != nil || d.Source != "ARAMKit" {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	var versions aramkitVersions
	r307Fixture(t, "versions", &versions)
	v, _ := selectARAMKitVersion(versions)
	expected := map[string]int{"/data/versions.json": 1, "/" + v.DataPath + "/stats/all/champion-details/157.json": 1}
	for _, name := range []string{"champions", "augments", "items", "summoner-spells", "archetypes"} {
		expected["/"+v.ResourcePath+"/zh-CN/resources/"+name+".json"] = 1
	}
	if !reflect.DeepEqual(calls, expected) {
		t.Fatalf("ten concurrent requests: got %v want %v", calls, expected)
	}
	p, calls = newARAMKitFixtureProvider(t, root)
	if _, e := p.loadMayhemDetail(t.Context(), "157"); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(calls, map[string]int{"/data/versions.json": 1}) {
		t.Fatalf("restart request inventory: %v", calls)
	}
}
func TestARAMKitRequestUsesHonestUserAgent(t *testing.T) {
	p := newChampionProvider()
	p.client = &http.Client{Transport: r307Transport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("User-Agent") != "Deep-Legends/"+version || strings.Contains(r.Header.Get("User-Agent"), "Mozilla") {
			t.Fatal("dishonest UA")
		}
		return publicDataResponse(r, []byte(`{}`)), nil
	})}
	if _, e := p.aramkitClient().immutable(t.Context(), "data/test/fixture.json"); e != nil {
		t.Fatal(e)
	}
}
func TestARAMKitDisabledSourceMakesNoRequests(t *testing.T) {
	p := newChampionProvider()
	p.featureGates.apply(map[string]bool{featureGateAramkit: false})
	p.client = &http.Client{Transport: r307Transport(func(*http.Request) (*http.Response, error) {
		t.Fatal("disabled source requested network")
		return nil, nil
	})}
	if _, e := p.aramkitClient().immutable(t.Context(), "data/test/fixture.json"); e == nil {
		t.Fatal("disabled source accepted")
	}
}
func TestR307RetiredCacheCleanupIsScopedAndReportsCounts(t *testing.T) {
	root := t.TempDir()
	store := trackTestStore(t, &localStore{root: root})
	if e := os.MkdirAll(filepath.Join(root, "logs"), 0700); e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(root, championDataCacheDirectory)
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	oldKey := retiredMayhemCacheTag + "-old|hero|157"
	payload, _ := json.Marshal(championCacheEnvelope{Schema: 1, Key: oldKey, Data: []byte("old")})
	sum := sha256.Sum256([]byte(oldKey))
	hash := hex.EncodeToString(sum[:]) + ".json"
	for _, name := range []string{hash, retiredMayhemCacheTag + "-" + hash, retiredMayhemCacheTag + "-state.json"} {
		if e := os.WriteFile(filepath.Join(dir, name), payload, 0600); e != nil {
			t.Fatal(e)
		}
	}
	kept := filepath.Join(dir, "unrelated.json")
	if e := os.WriteFile(kept, []byte(`{"key":"unrelated"}`), 0600); e != nil {
		t.Fatal(e)
	}
	outside := filepath.Join(root, "outside.json")
	if e := os.WriteFile(outside, payload, 0600); e != nil {
		t.Fatal(e)
	}
	removeRetiredMayhemCache(store)
	for _, name := range []string{hash, retiredMayhemCacheTag + "-" + hash, retiredMayhemCacheTag + "-state.json"} {
		if _, e := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(e) {
			t.Fatal("retired file retained")
		}
	}
	for _, name := range []string{kept, outside} {
		if _, e := os.Stat(name); e != nil {
			t.Fatal("unrelated file deleted", e)
		}
	}
	text := r116dDiagnosticText(t, store)
	if !strings.Contains(text, retiredMayhemCacheTag+"_cache_removed") || !strings.Contains(text, `"files":3`) {
		t.Fatal("cleanup diagnostic missing", text)
	}
}

func TestARAMKitCorruptDiskRefetchesAndCanceledOrDisabledReadsDoNotPoisonRetry(t *testing.T) {
	p := newChampionProvider()
	c := p.aramkitClient()
	c.dir = t.TempDir()
	name := "data/build/corrupt.json"
	if e := os.MkdirAll(filepath.Join(c.dir, "data/build"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(c.dir, filepath.FromSlash(name)), []byte(`{broken`), 0600); e != nil {
		t.Fatal(e)
	}
	var calls atomic.Int32
	p.client = &http.Client{Transport: r307Transport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if e := r.Context().Err(); e != nil {
			return nil, e
		}
		return publicDataResponse(r, []byte(`{"valid":true}`)), nil
	})}
	b, e := c.immutable(t.Context(), name)
	if e != nil || string(b) != `{"valid":true}` || calls.Load() != 1 {
		t.Fatal("corrupt disk did not refetch", string(b), e, calls.Load())
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _ = c.immutable(ctx, "data/build/canceled.json")
	if !c.retry["data/build/canceled.json"].IsZero() {
		t.Fatal("caller cancellation poisoned retry")
	}
	p.featureGates.apply(map[string]bool{featureGateAramkit: false})
	_, _ = c.immutable(t.Context(), "data/build/disabled.json")
	if !c.retry["data/build/disabled.json"].IsZero() {
		t.Fatal("disabled gate poisoned retry")
	}
	p.featureGates.apply(map[string]bool{featureGateAramkit: true})
	if _, e := c.immutable(t.Context(), "data/build/disabled.json"); e != nil {
		t.Fatal("reenabled source remained blocked", e)
	}
}
func TestARAMKitBackoffHitsDoNotExtendRetryDeadline(t *testing.T) {
	p := newChampionProvider()
	c := p.aramkitClient()
	c.dir = t.TempDir()
	var calls atomic.Int32
	p.client = &http.Client{Transport: r307Transport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		response := publicDataResponse(r, []byte(`{}`))
		response.StatusCode = 503
		return response, nil
	})}
	name := "data/build/failure.json"
	if _, e := c.immutable(t.Context(), name); e == nil {
		t.Fatal("failure accepted")
	}
	deadline := c.retry[name]
	if deadline.IsZero() {
		t.Fatal("missing failure backoff")
	}
	before := calls.Load()
	if _, e := c.immutable(t.Context(), name); e == nil {
		t.Fatal("backoff accepted")
	}
	if calls.Load() != before || c.retry[name] != deadline {
		t.Fatal("backoff extended or made requests")
	}
	c.retry[name] = time.Now().Add(-time.Second)
	_, _ = c.immutable(t.Context(), name)
	if calls.Load() != before+2 {
		t.Fatal("expired backoff did not retry both static mirrors")
	}
}

func TestR148MayhemHeroFailureStillAttemptsOPGGRSC(t *testing.T) {
	for _, failure := range []struct {
		name string
		err  error
	}{{name: "http_504"}, {name: "timeout", err: context.DeadlineExceeded}} {
		for _, available := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/rsc_%t", failure.name, available), func(t *testing.T) {
				p, _ := newARAMKitFixtureProvider(t, "")
				base := p.client.Transport
				var rscCalls atomic.Int32
				p.static["item/3032.png"] = championAssetDescription{Name: "测试装备"}
				p.client.Transport = r307Transport(func(req *http.Request) (*http.Response, error) {
					if strings.Contains(req.URL.Path, "/champion-details/157.json") {
						if failure.err != nil {
							return nil, failure.err
						}
						resp := publicDataResponse(req, []byte(`{}`))
						resp.StatusCode = 504
						return resp, nil
					}
					if req.URL.Host == opggPageHost && strings.HasSuffix(req.URL.Path, "/yasuo/build") {
						rscCalls.Add(1)
						if available {
							return publicDataResponse(req, []byte(r157MayhemRSC)), nil
						}
						return nil, fmt.Errorf("fixture offline")
					}
					if req.URL.Host == aramkitDataCN || req.URL.Host == aramkitDataCOM {
						return base.RoundTrip(req)
					}
					return nil, fmt.Errorf("fixture offline")
				})
				d, e := p.loadMayhemDetail(t.Context(), "157")
				if e != nil {
					t.Fatal(e)
				}
				if rscCalls.Load() != 1 {
					t.Fatal("RSC fallback not attempted exactly once", rscCalls.Load())
				}
				if available {
					if d.Source != "OP.GG RSC" || len(d.Build.CoreItems) == 0 {
						t.Fatal("RSC build lost", d)
					}
				} else if d.Source != "none" || len(d.Build.CoreItems) != 0 || len(d.ItemRanking) != 0 || len(d.RecommendedAugments) != 0 {
					t.Fatal("failed sources left fake cards", d)
				}
			})
		}
	}
}

func TestLoadMayhemDetailItemIDsSurviveDuplicateAndSpecialCharacterNames(t *testing.T) {
	r := r307Resources(t)
	var raw aramkitRawDetail
	r307Fixture(t, "champion-157", &raw)
	ids := []int{123430, 994403, 3031}
	names := []string{"重复名称（改）", "重复名称（改）", "特殊/字符\\ · <>&\""}
	rows := make([]aramkitStat, 0, len(ids))
	for i, id := range ids {
		key := strconv.Itoa(id)
		item, ok := r.Items[key]
		if !ok {
			t.Fatal("real item fixture missing", id)
		}
		item.Name = names[i]
		r.Items[key] = item
		rows = append(rows, aramkitStat{ID: id, Rank: i + 1, WinRate: .6, PickRate: .2, SampleCount: 1000})
	}
	scope := raw.Items["unfiltered"]
	scope.All = rows
	raw.Items["unfiltered"] = scope
	b, e := json.Marshal(raw)
	if e != nil {
		t.Fatal(e)
	}
	d, _, e := parseARAMKitDetail(b, 157, r)
	if e != nil || len(d.ItemRanking) != 3 {
		t.Fatal(e, d.ItemRanking)
	}
	for i, id := range ids {
		if d.ItemRanking[i].Assets[0].ID != id || d.ItemRanking[i].Assets[0].Name != names[i] {
			t.Fatal("ID or verbatim name lost", d.ItemRanking[i])
		}
	}
}
func TestARAMKitAppDiskCacheStaysWithinDeclaredChampionData(t *testing.T) {
	root := t.TempDir()
	p := newPublicDataTestProvider(t, root, nil)
	if got, want := p.aramkitClient().dir, filepath.Join(root, championDataCacheDirectory, "aramkit"); got != want {
		t.Fatal(got, want)
	}
}
