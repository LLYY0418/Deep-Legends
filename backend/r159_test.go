package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestR159HexdataIconProxyStaysScopedAndServesImage(t *testing.T) {
	const icon = "/assets/augments/icons/16.18/highroller_small.png"
	if host, ok := validateChampionAssetPath("hexdata", icon); !ok || host != hexdataAssetHost {
		t.Fatalf("real icon rejected: %q %v", host, ok)
	}
	for _, bad := range []string{
		"https://dl.hexdata.com.cn" + icon,
		"/assets/items/icons/16.18/highroller_small.png",
		"/assets/augments/icons/16.18/../secret.png",
		icon + "?x=1", "/assets/augments/icons/16.18/test.svg",
	} {
		if _, ok := validateChampionAssetPath("hexdata", bad); ok {
			t.Errorf("unrelated path accepted: %s", bad)
		}
	}
	image := []byte("\x89PNG\r\n\x1a\nR159-icon")
	provider := newChampionProvider()
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != hexdataAssetHost || request.URL.Path != icon {
			t.Errorf("wrong asset target: %s", request.URL.String())
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(image)), ContentLength: int64(len(image)), Request: request}, nil
	})}
	a := &app{champions: provider}
	w := httptest.NewRecorder()
	a.handleChampionAsset(w, httptest.NewRequest(http.MethodGet, "/api/champion-asset?source=hexdata&path="+icon, nil))
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/png" || !bytes.Equal(w.Body.Bytes(), image) {
		t.Fatalf("image response: status=%d type=%q body=%q", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
}

func TestR159HexdataHeroIconsAndObservedCatalogPersist(t *testing.T) {
	root := t.TempDir()
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(&localStore{root: root})
	if err := os.MkdirAll(provider.cache.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	payload, _, err := parseHexdataHeroJSONWithStats(r116aFixture(t, "hexdata-hero-157.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.Augments) < 2 {
		t.Fatal("hero fixture lacks distinct icons")
	}
	rows := hexdataAugmentMetricRows(payload.Augments)
	if len(rows) < 2 || rows[0].Assets[0].Path == "" || rows[0].Assets[0].Path == rows[1].Assets[0].Path {
		t.Fatalf("hero icons missing or collapsed: %#v", rows)
	}
	provider.observeHexdataAugments(payload.Augments)
	got := gameplayAugmentIndexAll(provider.observedHexdataAugments())
	first := payload.Augments[0]
	if got[first.AugmentID].IconPath != "hexdata:"+first.AugmentIconURL {
		t.Fatalf("observed catalog missing first icon: %#v", got[first.AugmentID])
	}
	if source, path := augmentMetadataImage(got[first.AugmentID].IconPath); source != "hexdata" || path != first.AugmentIconURL {
		t.Fatalf("atlas did not resolve observed icon: %q %q", source, path)
	}
	// The observed small icon stays on disk; the verified bundled full artwork
	// takes precedence at rendering time without a CommunityDragon call.
	provider.gameplayAugments = func(context.Context) ([]gameplayAugment, error) { return nil, nil }
	fast := gameplayAugmentIndexAll(provider.loadAugmentMetadataCatalogFast(context.Background()))
	fullIcon := "builtin:" + bundledAugmentPath(int64(first.AugmentID))
	if fast[first.AugmentID].IconPath != fullIcon {
		t.Fatalf("fast catalog did not prefer full artwork: %#v", fast[first.AugmentID])
	}
	// A cached remote catalog cannot replace verified offline artwork.
	provider.rememberCommunityDragonAugments([]gameplayAugment{{ID: int64(first.AugmentID), FallbackIconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/fallback.png"}})
	fast = gameplayAugmentIndexAll(provider.loadAugmentMetadataCatalogFast(context.Background()))
	if fast[first.AugmentID].IconPath != fullIcon || fast[first.AugmentID].FallbackIconPath != "" {
		t.Fatalf("cached enrichment lost icon or fallback: %#v", fast[first.AugmentID])
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(provider.cache.pathFor(observedHexdataAugmentsKey)); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("observed index was not persisted")
		}
		time.Sleep(10 * time.Millisecond)
	}
	restarted := newChampionProvider()
	restarted.cache = newChampionDataCache(&localStore{root: root})
	stored := gameplayAugmentIndexAll(restarted.observedHexdataAugments())
	if stored[first.AugmentID].IconPath != got[first.AugmentID].IconPath {
		t.Fatalf("restart lost observed icon: %#v", stored[first.AugmentID])
	}
	if _, err := os.Stat(filepath.Join(root, championDataCacheDirectory)); err != nil {
		t.Fatal(err)
	}
}

func TestR159HexdataIconSurvivesCatalogEnrichment(t *testing.T) {
	provider := newChampionProvider()
	rows := []championMetricRow{{Assets: []championAsset{{ID: 2095, Kind: "augment", Source: "hexdata", Path: "/assets/augments/icons/16.18/highroller_small.png"}}, Rarity: "prismatic"}}
	provider.decorateHexdataAugmentsWithCatalog(rows, map[int]gameplayAugment{2095: {
		ID: 2095, Description: "补充说明", IconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/highroller.png",
	}})
	asset := rows[0].Assets[0]
	if asset.Source != "hexdata" || !strings.HasSuffix(asset.Path, "highroller_small.png") || asset.Description != "补充说明" {
		t.Fatalf("enrichment replaced direct icon or lost description: %#v", asset)
	}
}

func TestR159OfflinePerksCatalogSeesHeroIconsAfterBaseCache(t *testing.T) {
	provider := newChampionProvider()
	provider.observeHexdataAugments([]hexdataAugmentRowV2{{
		AugmentID: 2095, AugmentName: "掷骰狂人", AugmentIconURL: "/assets/augments/icons/16.18/highroller_small.png",
	}})
	a := &app{champions: provider,
		perkCatalog: map[string]gameplayPerkCatalogCacheEntry{
			"ddragon": {loadedAt: time.Now(), payload: gameplayPerkCatalogResponse{}},
		},
		perkAugmentJobs:     make(map[string]chan struct{}),
		perkAugmentAttempts: map[string]time.Time{"ddragon": time.Now()},
	}
	w := httptest.NewRecorder()
	a.handleGameplayPerks(w, httptest.NewRequest(http.MethodGet, "/api/gameplay/perks", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("offline perk catalog status = %d", w.Code)
	}
	var body gameplayPerkCatalogResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	byID := gameplayAugmentIndexAll(body.Augments)
	if byID[2095].IconPath != "builtin:/augments/2095.png" {
		t.Fatalf("cached base catalog did not see observed icon: %#v", byID[2095])
	}
}
