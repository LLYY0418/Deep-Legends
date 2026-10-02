package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestR160BundledAugmentArtMatchesScreenshotIDs(t *testing.T) {
	want := map[int64]string{
		110: "晾衣绳", 30: "尤里卡", 48: "珠光护手",
		75: "慢炖", 215: "黑暗赐福", 18: "巨像的勇气",
		41: "歌利亚巨人", 218: "亵渎者", 45: "炼狱导管",
		2095: "掷骰狂人", 2031: "空投熊",
	}
	index := gameplayAugmentIndexAll(bundledAugmentCatalog())
	if len(index) != 554 {
		t.Fatalf("bundled catalog rows = %d, want 554", len(index))
	}
	for id, name := range want {
		row := index[int(id)]
		if row.Name != name || row.IconPath != "builtin:"+bundledAugmentPath(id) {
			t.Fatalf("id %d metadata = %#v", id, row)
		}
		data, ok := bundledAugmentImage(bundledAugmentPath(id))
		if !ok {
			t.Fatalf("id %d missing bundled art", id)
		}
		config, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil || config.Width != 256 || config.Height != 256 {
			t.Fatalf("id %d art = %dx%d, err %v", id, config.Width, config.Height, err)
		}
	}
}

func TestR160ArenaAugmentsResolveWithoutCommunityDragon(t *testing.T) {
	provider := newChampionProvider()
	groups := []opggArenaAugmentGroup{{Rarity: 1, Augments: []opggAugmentMetric{{ID: 110, Play: 100, Win: 50}, {ID: 70001, Play: 10, Win: 5}}}}
	rows := provider.structuredArenaAugmentGroupsWithCatalog(groups, provider.arenaAugmentCatalogFast(), nil)
	if len(rows) != 1 || len(rows[0].Rows) != 2 {
		t.Fatalf("arena rows = %#v", rows)
	}
	known, unknown := rows[0].Rows[0].Assets[0], rows[0].Rows[1].Assets[0]
	if known.Name != "晾衣绳" || known.Source != "builtin" || known.Path != "/augments/110.png" {
		t.Fatalf("known arena icon = %#v", known)
	}
	if unknown.Name != "海克斯 70001" || unknown.Source != "" || unknown.Path != "" {
		t.Fatalf("unknown arena ID was guessed: %#v", unknown)
	}
}

func TestR160OverviewAugmentCatalogDoesNotWaitForPerks(t *testing.T) {
	a := &app{}
	w := httptest.NewRecorder()
	a.handleGameplayAugments(w, httptest.NewRequest(http.MethodGet, "/api/gameplay/augments", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("local augment index status = %d", w.Code)
	}
	var response struct {
		Augments []gameplayAugment `json:"augments"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Augments) != 554 || response.Augments[0].IconPath != "builtin:/augments/1.png" {
		t.Fatalf("local augment index: count=%d first=%#v", len(response.Augments), response.Augments[0])
	}
}

func TestR160ArenaAggregateHasBoundedDetailBudget(t *testing.T) {
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(nil)
	called := false
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		called = true
		deadline, ok := request.Context().Deadline()
		remaining := time.Until(deadline)
		if !ok || remaining < 3*time.Second || remaining > 4*time.Second {
			t.Errorf("YOUR.GG aggregate budget = %s, want up to 4 seconds", remaining)
		}
		return nil, context.DeadlineExceeded
	})}
	if _, _, _, err := provider.loadArenaChampionAggregate(context.Background(), 67); err == nil || !called {
		t.Fatalf("aggregate request result: called=%v err=%v", called, err)
	}
}

func TestR160MayhemUsesBundledArtAndStrictProxy(t *testing.T) {
	payload, _, err := parseHexdataHeroJSONWithStats(r116aFixture(t, "hexdata-hero-157.json"))
	if err != nil {
		t.Fatal(err)
	}
	rows := hexdataAugmentMetricRows(payload.Augments)
	if len(rows) == 0 || rows[0].Assets[0].ID != 2095 || rows[0].Assets[0].Source != "builtin" || rows[0].Assets[0].Path != "/augments/2095.png" {
		t.Fatalf("mayhem artwork = %#v", rows)
	}
	for _, path := range []string{"/augments/0.png", "/augments/0110.png", "/augments/999999.png", "/augments/../110.png", "/augments/110.svg"} {
		if _, ok := validateChampionAssetPath("builtin", path); ok {
			t.Fatalf("invalid built-in path accepted: %q", path)
		}
	}
	app := &app{}
	w := httptest.NewRecorder()
	app.handleChampionAsset(w, httptest.NewRequest(http.MethodGet, "/api/champion-asset?source=builtin&path=%2Faugments%2F2095.png", nil).WithContext(context.Background()))
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("bundled image response = %d %q", w.Code, w.Header().Get("Content-Type"))
	}
}

func TestR160OverviewChampionNamesColdStart(t *testing.T) {
	provider := newChampionProvider()
	a := &app{riot: newRiotProvider(provider)}
	names := a.overviewChampionNames(context.Background())
	if len(names) != 173 || names[3] != "正义巨像" || names[157] != "疾风剑豪" {
		t.Fatalf("bundled overview names: count=%d, galio=%q, yasuo=%q", len(names), names[3], names[157])
	}
	provider.mu.Lock()
	provider.patch = "16.20.1"
	provider.catalogNamesFailUntil = time.Now().Add(time.Minute)
	provider.mu.Unlock()
	if names := a.overviewChampionNames(context.Background()); len(names) != 0 {
		t.Fatalf("newer patch reused stale bundled names: count=%d", len(names))
	}
}

func TestR160OfflineOverviewChampionIcon(t *testing.T) {
	a := &app{}
	w := httptest.NewRecorder()
	a.handleImage(w, httptest.NewRequest(http.MethodGet, "/api/image?path=%2Flol-game-data%2Fassets%2Fv1%2Fchampion-icons%2F3.png", nil))
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("offline overview icon = %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	config, err := png.DecodeConfig(bytes.NewReader(w.Body.Bytes()))
	if err != nil || config.Width == 0 || config.Height == 0 {
		t.Fatalf("offline overview icon invalid: %#v %v", config, err)
	}
	if _, ok := bundledChampionIcon("/lol-game-data/assets/v1/champion-icons/03.png"); ok {
		t.Fatal("noncanonical champion icon path was accepted")
	}
}

func TestR160SuccessfulSlowImageDiagnosticReachesLog(t *testing.T) {
	instance := newR127DiagnosticApp(t)
	body := `{"event":"image_queue_slow","reason":"loaded","queueWaitMs":4200,"loadMs":220,` +
		`"activeSlowCount":2,"imageSource":"builtin"}`
	if code := r127PostDiagnostic(t, instance, body); code != http.StatusNoContent {
		t.Fatalf("slow image diagnostic status = %d", code)
	}
	events := r127DiagnosticEvents(t, instance)
	r127AssertNoRejection(t, events)
	if len(events) != 1 || events[0]["event"] != "image_queue_slow" || events[0]["image_source"] != "builtin" || events[0]["queue_wait_ms"] != float64(4200) {
		t.Fatalf("slow image diagnostic = %#v", events)
	}
}
