package main

import (
	"encoding/json"

	"net/http"
	"net/http/httptest"

	"testing"
	"time"
)

func TestR159OfflinePerksCatalogSeesHeroIconsAfterBaseCache(t *testing.T) {
	provider := newChampionProvider()
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
		t.Fatalf("cached base catalog did not see current offline metadata: %#v", byID[2095])
	}
}
