package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type r29RoundTripper func(*http.Request) (*http.Response, error)

func (f r29RoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestSnapshotRetryDelayBacksOffAndCaps(t *testing.T) {
	want := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second, 60 * time.Second, 60 * time.Second}
	for attempt, expected := range want {
		if got := nextSnapshotRetryDelay(attempt, false); got != expected {
			t.Fatalf("attempt %d delay = %s, want %s", attempt, got, expected)
		}
	}
	if got := nextSnapshotRetryDelay(1, true); got != 60*time.Second {
		t.Fatalf("exhausted retry delay = %s, want 60s", got)
	}
}

func TestStalePresenceChoosesSmallestSupportedAuthoritativeSet(t *testing.T) {
	authoritative := []ownershipResult{
		{ids: map[int64]bool{1: true, 2: true, 3: true, 4: true, 5: true}},
		{ids: map[int64]bool{1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true}},
	}
	presence := []ownershipResult{{ids: map[int64]bool{1: true, 2: true, 3: true}}}
	index, support, tied := bestCorroboratedAuthoritative(authoritative, presence)
	if index != 0 || support != 1 || tied {
		t.Fatalf("stale subset arbitration = (%d, %d, %v), want (0, 1, false)", index, support, tied)
	}
	if !idSetContains(presence[0].ids, authoritative[index].ids) {
		t.Fatal("selected authoritative set does not contain all cached presence IDs")
	}
}

func TestPresenceExtraIDCannotSupportEitherAuthoritativeSet(t *testing.T) {
	authoritative := []ownershipResult{
		{ids: map[int64]bool{1: true, 2: true, 3: true}},
		{ids: map[int64]bool{1: true, 2: true, 4: true}},
	}
	presence := []ownershipResult{{ids: map[int64]bool{1: true, 2: true, 9: true}}}
	index, support, tied := bestCorroboratedAuthoritative(authoritative, presence)
	if index != 0 || support != 0 || tied {
		t.Fatalf("extra presence ID arbitration = (%d, %d, %v), want no support", index, support, tied)
	}
}

func TestInventoryV1EndpointDisablesAfterRepeatedFailures(t *testing.T) {
	client := &LCUClient{}
	for attempt := 1; attempt <= inventoryV1FailureBudget; attempt++ {
		count, disabled := client.noteInventoryV1Failure()
		if count != attempt {
			t.Fatalf("failure count = %d, want %d", count, attempt)
		}
		if disabled != (attempt == inventoryV1FailureBudget) {
			t.Fatalf("disabled = %v at attempt %d", disabled, attempt)
		}
	}
	if !client.inventoryV1DisabledNow() {
		t.Fatal("endpoint should be disabled after failure budget")
	}
	client.resetInventoryV1Failures()
	if client.inventoryV1DisabledNow() {
		t.Fatal("successful refresh should reset endpoint degradation")
	}
}

func TestInventoryV1EndpointIsRemovedFromLiveArbitrationAfterFailures(t *testing.T) {
	v1Calls := 0
	client := &LCUClient{baseURL: "http://r29.test", token: "test-token", http: &http.Client{Transport: r29RoundTripper(func(request *http.Request) (*http.Response, error) {
		var body string
		status := http.StatusOK
		switch {
		case strings.Contains(request.URL.Path, "skins-minimal"), strings.HasSuffix(request.URL.Path, "/champions"):
			body = `[{"id":1001,"owned":true}]`
		case request.URL.Path == "/lol-inventory/v2/inventory/CHAMPION_SKIN":
			body = `[{"itemId":1001,"inventoryType":"CHAMPION_SKIN"}]`
		case strings.HasPrefix(request.URL.Path, "/lol-inventory/v1/"):
			v1Calls++
			status = http.StatusInternalServerError
			body = `temporary`
		default:
			status = http.StatusNotFound
			body = `not found`
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: request}, nil
	})}}
	catalog := []Skin{{ID: 1001, Name: "测试皮肤", ChampionID: 1}}
	for attempt := 0; attempt < inventoryV1FailureBudget+1; attempt++ {
		ids, _, err := loadOwnedSkinInventory(client, 42, catalog)
		if err != nil || !ids[1001] {
			t.Fatalf("repeated dead endpoint should not veto valid sources on attempt %d: ids=%#v err=%v", attempt+1, ids, err)
		}
	}
	if v1Calls != inventoryV1FailureBudget {
		t.Fatalf("dead v1 endpoint calls = %d, want %d before session degradation", v1Calls, inventoryV1FailureBudget)
	}
}

func TestSkinsFromSnapshotMarksHistoricalOwnership(t *testing.T) {
	owned := skinsFromSnapshot([]snapshotSkin{{ID: 1001, Name: "测试", ChampionName: "英雄"}}, true)
	remaining := skinsFromSnapshot([]snapshotSkin{{ID: 1002, Name: "剩余"}}, false)
	if len(owned) != 1 || !owned[0].Owned || len(remaining) != 1 || remaining[0].Owned {
		t.Fatalf("snapshot conversion lost historical ownership: owned=%#v remaining=%#v", owned, remaining)
	}
}

func TestHistoricalFallbackSkinsAreServedAsStaleWithoutSnapshotReady(t *testing.T) {
	a := &app{
		connected: true, snapshotReady: false, snapshotFallback: true,
		fallbackOwned:      []Skin{{ID: 1001, Name: "历史皮肤", Owned: true}},
		snapshotFallbackAt: time.Date(2026, 8, 27, 1, 2, 3, 0, time.UTC),
	}
	recorder := httptest.NewRecorder()
	a.handleSkins(recorder, httptest.NewRequest(http.MethodGet, "/api/skins?view=owned", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("historical fallback status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Stale bool   `json:"stale"`
		Items []Skin `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Stale || len(payload.Items) != 1 || payload.Items[0].ID != 1001 {
		t.Fatalf("historical fallback payload = %#v", payload)
	}
	if a.snapshotReady {
		t.Fatal("historical fallback must not mark the live snapshot ready")
	}
}

func TestStatusExposesRetryBudgetAndHistoricalFallback(t *testing.T) {
	a := &app{
		connected: true, snapshotFallback: true, snapshotRetryCount: 4,
		snapshotRetryStarted: time.Now().Add(-12 * time.Second),
		fallbackOwned:        []Skin{{ID: 1001}}, fallbackRemaining: []Skin{{ID: 1002}},
	}
	recorder := httptest.NewRecorder()
	a.handleStatus(recorder, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	var payload statusResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SnapshotRetryCount != 4 || payload.SnapshotRetryElapsedMS < 10_000 || !payload.SnapshotFallback || payload.OwnedCount != 1 || payload.Remaining != 1 {
		t.Fatalf("status omitted retry/fallback state: %#v", payload)
	}
	if payload.SnapshotReady {
		t.Fatal("historical fallback must remain distinct from a ready snapshot")
	}
}
