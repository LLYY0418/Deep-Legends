package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestR70OverviewInvalidationDetachesOldFlights(t *testing.T) {
	cache := newOverviewQueryCache()
	old := &overviewQueryFlight{done: make(chan struct{})}
	cache.flights["player"] = old
	(&app{overviewQueries: cache}).clearOverviewQuerySnapshots()
	current := &overviewQueryFlight{done: make(chan struct{})}
	cache.flights["player"] = current
	cache.complete("player", old, gameplayOverview{}, nil)
	select {
	case <-old.done:
	default:
		t.Fatal("old waiters were stranded")
	}
	if cache.flights["player"] != current || len(cache.entries) != 0 {
		t.Fatal("stale completion replaced new flight or repopulated invalidated cache")
	}
	cache.complete("player", current, gameplayOverview{}, nil)
	if len(cache.entries) != 1 || len(cache.flights) != 0 {
		t.Fatal("current generation did not populate cache")
	}
}

func TestR70SSEOverflowRequestsResyncWithoutBlockingPeers(t *testing.T) {
	slow, fast := make(chan string, 32), make(chan string, 32)
	a := &app{eventSubscribers: map[chan string]struct{}{slow: {}, fast: {}}}
	for i := 0; i < 32; i++ {
		slow <- "old"
	}
	a.broadcastEvent("claim:changed")
	if got := <-slow; got != "resync-required" {
		t.Fatalf("overflow = %q", got)
	}
	if got := <-fast; got != "claim:changed" {
		t.Fatalf("fast subscriber = %q", got)
	}
	for i := 0; i < 10000; i++ {
		a.broadcastEvent("collection:changed")
	}
	found := false
	for len(slow) > 0 {
		if <-slow == "resync-required" {
			found = true
		}
	}
	if !found {
		t.Fatal("event flood lost resync marker")
	}
}

func TestR70ChestsClassifyContainerRatherThanContents(t *testing.T) {
	for _, item := range []LootItem{
		{LootID: "CHEST_224", Type: "CHEST"},
		{LootID: "CHEST_skin_token", IsSkinRelated: true},
		{LootName: "CHEST_EMOTE", DisplayCategories: "EMOTE"},
		{LootID: "CHEST_tft_companion", DisplayCategories: "COMPANION"},
		{LootID: "CHEST_promotion"},
	} {
		if got := lootCategory(item); got != "宝箱" {
			t.Errorf("%+v category = %s", item, got)
		}
	}
	if lootCategory(LootItem{LootID: "EMOTE_1", Type: "EMOTE"}) != "表情" {
		t.Fatal("actual emote misclassified")
	}
	if lootCategory(LootItem{LootID: "CHAMPION_SKIN_1000", IsSkinRelated: true}) != "皮肤" {
		t.Fatal("actual skin misclassified")
	}
}

func TestR70MalformedGrantsCannotWrite(t *testing.T) {
	good := claimEntry{Source: "grant", ID: "grant", RewardGroupID: "group", Items: []RewardItem{{ID: "one"}}, MinSelections: 1, MaxSelections: 1}
	cases := []claimEntry{good, good, good, good, good, good}
	cases[0].RewardGroupID = "../unsafe"
	cases[1].Items = nil
	cases[2].Items = []RewardItem{{}}
	cases[3].MaxSelections = 2
	cases[4].MinSelections = -1
	cases[5].Items = []RewardItem{{ID: "one"}, {ID: "one"}}
	for i, entry := range cases {
		if grantValidationReason(entry) == "" {
			t.Errorf("case %d advertised unsafe action", i)
		}
		// nil client deliberately proves failure occurs before any network write.
		if err := executeClaimEntry(context.Background(), nil, entry, nil); err == nil {
			t.Errorf("case %d permitted write", i)
		}
	}
}

func TestR70ScanPreservesUnsafeRewardEvidenceWithoutAdvertisingClaim(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/lol-rewards/v1/grants":
			_, _ = w.Write([]byte(`[{"info":{"id":"bad","status":"PENDING"},"rewardGroup":{"id":"","rewards":[{"id":"one"}]}},{"info":{"id":"range","status":"PENDING"},"rewardGroup":{"id":"g","selectionStrategyConfig":{"minSelectionsAllowed":2,"maxSelectionsAllowed":2},"rewards":[{"id":"one"}]}}]`))
		case "/lol-missions/v1/missions":
			_, _ = w.Write([]byte(`[{"id":"mission","status":"SELECT_REWARDS","rewardGroups":[]}]`))
		default:
			_ = json.NewEncoder(w).Encode([]any{})
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test", http: server.Client()}
	scan := scanClaims(context.Background(), client)
	if len(scan.Items) != 3 {
		t.Fatalf("evidence lost: %+v", scan.Items)
	}
	for _, item := range scan.Items {
		if item.Actionable || item.Detail == "" {
			t.Errorf("unsafe entry: %+v", item)
		}
	}
}

func TestR70ClaimBatchRejectsDuplicateWithoutBlockingOtherRewards(t *testing.T) {
	var writes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/lol-rewards/v1/grants":
			_, _ = w.Write([]byte(`[{"info":{"id":"one","status":"PENDING"},"rewardGroup":{"id":"g1","rewards":[{"id":"r1"}]}},{"info":{"id":"two","status":"PENDING"},"rewardGroup":{"id":"g2","rewards":[{"id":"r2"}]}}]`))
		case r.Method == http.MethodPost && (r.URL.Path == "/lol-rewards/v1/grants/one/select" || r.URL.Path == "/lol-rewards/v1/grants/two/select"):
			writes.Add(1)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected write: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	a := &app{connected: true, lcu: &LCUClient{baseURL: server.URL, token: "test", http: server.Client()}}
	recorder := httptest.NewRecorder()
	a.handleClaimExecute(recorder, httptest.NewRequest(http.MethodPost, "/api/claim/execute", strings.NewReader(`{"items":[{"key":"grant:one"},{"key":"grant:one"},{"key":"grant:two"}]}`)))
	var response claimExecuteResponse
	if recorder.Code != http.StatusOK || json.Unmarshal(recorder.Body.Bytes(), &response) != nil {
		t.Fatalf("invalid response: %d %s", recorder.Code, recorder.Body.String())
	}
	if writes.Load() != 2 || response.Succeeded != 2 || response.Failed != 1 || len(response.Results) != 3 {
		t.Fatalf("duplicate was executed or independent reward was blocked: writes=%d response=%+v", writes.Load(), response)
	}
	if response.Results[1].OK || !strings.Contains(response.Results[1].Message, "重复") || !response.Results[2].OK {
		t.Fatalf("unexpected per-item results: %+v", response.Results)
	}
}
