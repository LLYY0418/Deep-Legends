package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
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
