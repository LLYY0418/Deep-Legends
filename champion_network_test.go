package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestChampionNetworkSettingsValidation(t *testing.T) {
	for _, mode := range []string{"auto", "direct"} {
		settings, err := validateChampionNetworkSettings(championNetworkSettings{Mode: mode, URL: "http://ignored:7890"})
		if err != nil || settings.URL != "" {
			t.Fatalf("%s should be valid and clear URL: %#v %v", mode, settings, err)
		}
	}
	settings, err := validateChampionNetworkSettings(championNetworkSettings{Mode: "manual", URL: "http://127.0.0.1:7890"})
	if err != nil || settings.URL == "" {
		t.Fatalf("manual proxy should be valid: %#v %v", settings, err)
	}
	for _, value := range []string{"", "ftp://proxy.example:21", "http://user:secret@proxy.example:8080", "http://proxy.example:8080/path"} {
		if _, err := validateChampionNetworkSettings(championNetworkSettings{Mode: "manual", URL: value}); err == nil {
			t.Fatalf("manual proxy %q should be rejected", value)
		}
	}
}

func TestChampionAutoProxyUsesDesktopResolution(t *testing.T) {
	previous, existed := os.LookupEnv("DEEP_LEGENDS_SYSTEM_PROXY")
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv("DEEP_LEGENDS_SYSTEM_PROXY", previous)
		} else {
			_ = os.Unsetenv("DEEP_LEGENDS_SYSTEM_PROXY")
		}
	})
	if err := os.Setenv("DEEP_LEGENDS_SYSTEM_PROXY", "http://127.0.0.1:7890"); err != nil {
		t.Fatal(err)
	}
	proxy, active, err := championProxyFor(defaultChampionNetworkSettings())
	if err != nil || proxy == nil || active != "http://127.0.0.1:7890" {
		t.Fatalf("unexpected auto proxy: %q %v", active, err)
	}
	request, _ := http.NewRequest(http.MethodGet, "https://lol-api-champion.op.gg/", nil)
	resolved, err := proxy(request)
	if err != nil || resolved.String() != "http://127.0.0.1:7890" {
		t.Fatalf("unexpected resolved proxy: %v %v", resolved, err)
	}
}

func TestSystemProxyEndpoint(t *testing.T) {
	previous, existed := os.LookupEnv("DEEP_LEGENDS_SYSTEM_PROXY")
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv("DEEP_LEGENDS_SYSTEM_PROXY", previous)
		} else {
			_ = os.Unsetenv("DEEP_LEGENDS_SYSTEM_PROXY")
		}
	})
	a := &app{}
	post := func(body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/system-proxy", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		a.handleSystemProxy(recorder, request)
		return recorder
	}
	if recorder := post(`{"proxy":"http://127.0.0.1:7890"}`); recorder.Code != http.StatusOK {
		t.Fatalf("valid proxy rejected: %d %s", recorder.Code, recorder.Body.String())
	}
	if value := os.Getenv("DEEP_LEGENDS_SYSTEM_PROXY"); value != "http://127.0.0.1:7890" {
		t.Fatalf("proxy env not applied: %q", value)
	}
	for _, body := range []string{`{"proxy":"ftp://proxy.example:21"}`, `{"proxy":"http://user:secret@proxy.example:8080"}`, `{"proxy":"not a url"}`} {
		if recorder := post(body); recorder.Code != http.StatusBadRequest {
			t.Fatalf("invalid proxy %s accepted: %d", body, recorder.Code)
		}
	}
	if value := os.Getenv("DEEP_LEGENDS_SYSTEM_PROXY"); value != "http://127.0.0.1:7890" {
		t.Fatalf("invalid submissions must not change env: %q", value)
	}
	if recorder := post(`{"proxy":""}`); recorder.Code != http.StatusOK {
		t.Fatalf("empty proxy rejected: %d", recorder.Code)
	}
	if _, present := os.LookupEnv("DEEP_LEGENDS_SYSTEM_PROXY"); present {
		t.Fatal("empty proxy should unset env")
	}
}

func TestChampionDataCacheSingleflightAndStaleFallback(t *testing.T) {
	cache := newChampionDataCache(nil)
	var calls atomic.Int32
	loader := func(context.Context) ([]byte, error) {
		calls.Add(1)
		time.Sleep(15 * time.Millisecond)
		return []byte(`{"ok":true}`), nil
	}
	const clients = 20
	var group sync.WaitGroup
	group.Add(clients)
	for range clients {
		go func() {
			defer group.Done()
			data, err := cache.load(context.Background(), "same", time.Minute, time.Hour, loader)
			if err != nil || string(data) != `{"ok":true}` {
				t.Errorf("load failed: %s %v", data, err)
			}
		}()
	}
	group.Wait()
	if calls.Load() != 1 {
		t.Fatalf("loader called %d times", calls.Load())
	}

	cache.mu.Lock()
	entry := cache.entries["same"]
	entry.ExpiresAt = time.Now().Add(-time.Minute)
	entry.StaleUntil = time.Now().Add(time.Hour)
	cache.entries["same"] = entry
	cache.mu.Unlock()
	data, err := cache.load(context.Background(), "same", time.Minute, time.Hour, func(context.Context) ([]byte, error) { return nil, errors.New("upstream down") })
	if err != nil || string(data) != `{"ok":true}` {
		t.Fatalf("stale fallback failed: %s %v", data, err)
	}
}

func TestChampionDataCachePersistsAcrossInstances(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, championDataCacheDirectory), 0o700); err != nil {
		t.Fatal(err)
	}
	store := &localStore{root: root}
	first := newChampionDataCache(store)
	data, err := first.load(context.Background(), "disk", time.Hour, time.Hour, func(context.Context) ([]byte, error) { return []byte(`{"cached":true}`), nil })
	if err != nil || len(data) == 0 {
		t.Fatalf("initial cache write failed: %v", err)
	}
	second := newChampionDataCache(store)
	var calls atomic.Int32
	data, err = second.load(context.Background(), "disk", time.Hour, time.Hour, func(context.Context) ([]byte, error) { calls.Add(1); return nil, errors.New("should not load") })
	if err != nil || string(data) != `{"cached":true}` || calls.Load() != 0 {
		t.Fatalf("disk cache miss: %s calls=%d err=%v", data, calls.Load(), err)
	}
}

func TestStructuredOPGGDetailSchema(t *testing.T) {
	fixture := []byte(`{"data":{"summary":{"id":67,"average_stats":{"play":100,"win":55,"total_place":320,"first_place":20,"pick_rate":0.14,"ban_rate":0.39}},"summoner_spells":[{"ids":[4,21],"play":80,"win":44,"pick_rate":0.8}],"skill_masteries":[{"ids":["Q","W","E"],"play":90,"win":50,"pick_rate":0.9,"builds":[{"order":["Q","W","E","Q","Q","R"],"play":70,"win":42,"pick_rate":0.77}]}],"core_items":[{"ids":[3153,3124,3302],"play":60,"win":36,"pick_rate":0.3}],"augment_group":[{"rarity":8,"augments":[{"id":225,"play":50,"win":35,"pick_rate":0.18,"win_rate":0.7}]}],"synergies":[{"champion_id":350,"play":30,"win":24,"total_place":70,"first_place":12,"pick_rate":0.03}]},"meta":{"version":"16.15","cached_at":"2026-08-11 21:53:48"}}`)
	var payload opggStructuredDetail
	if err := json.Unmarshal(fixture, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Summary.ID != 67 || len(payload.Data.SkillMasteries) != 1 || payload.Data.SkillMasteries[0].IDs[0] != "Q" {
		t.Fatalf("skill schema mismatch: %#v", payload.Data.SkillMasteries)
	}
	if payload.Data.AugmentGroup[0].Augments[0].ID != 225 || payload.Data.Synergies[0].ChampionID != 350 {
		t.Fatal("arena schema mismatch")
	}
	provider := newChampionProvider()
	provider.patch = "16.15.1"
	rows := provider.structuredMetrics(payload.Data.CoreItems, "item", 5)
	if len(rows) != 1 || len(rows[0].Assets) != 3 || rows[0].WinRate != 60 {
		t.Fatalf("metric conversion mismatch: %#v", rows)
	}
	skills := provider.structuredSkills(payload.Data.SkillMasteries)
	if len(skills) != 1 || len(skills[0].SkillOrder) != 6 || skills[0].WinRate != 60 {
		t.Fatalf("skill conversion mismatch: %#v", skills)
	}
}
