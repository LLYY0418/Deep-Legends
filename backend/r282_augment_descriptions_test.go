package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR282AugmentDescriptionFallbacks(t *testing.T) {
	var calls atomic.Int32
	p := newHexdataBudgetProvider(t, t.TempDir(), championRoundTripFunc(func(r *http.Request) (*http.Response, error) { calls.Add(1); return nil, context.DeadlineExceeded }))
	local := func(context.Context) []gameplayAugment {
		return []gameplayAugment{{ID: 93, Description: "本机读取的真实结构说明"}}
	}
	item := p.resolveAugmentDescription(context.Background(), 93, local)
	if item.Source != "lcu" || item.Status != "ok" || calls.Load() != 0 {
		t.Fatalf("LCU should bypass offline CDN: %#v calls=%d", item, calls.Load())
	}
	cached := p.resolveAugmentDescription(context.Background(), 93, nil)
	if cached.Source != "cache" || cached.Description != item.Description {
		t.Fatalf("cache %#v", cached)
	}
	key := augmentDescriptionKey(93)
	p.cache.mu.Lock()
	entry := p.cache.entries[key]
	entry.ExpiresAt = time.Now().Add(-time.Hour)
	p.cache.mu.Unlock()
	// Force the genuine disk envelope through a second provider, as after restart.
	if err := p.cache.writeDisk(entry); err != nil {
		t.Fatal(err)
	}
	p2 := newHexdataBudgetProvider(t, t.TempDir(), p.client.Transport)
	p2.cache.dir = p.cache.dir
	stale := p2.resolveAugmentDescription(context.Background(), 93, nil)
	if stale.Source != "stale_cache" || stale.Description != item.Description {
		t.Fatalf("restart stale %#v", stale)
	}
	waitR282AugmentRefresh(t, p2)
	again := p2.resolveAugmentDescription(context.Background(), 93, nil)
	if again.Description != item.Description {
		t.Fatal("failed refresh discarded stale cache")
	}
	waitR282AugmentRefresh(t, p2)
	bundled := p.resolveAugmentDescription(context.Background(), 89, nil)
	if bundled.Source != "bundled" || bundled.Status != "ok" {
		t.Fatalf("bundled %#v", bundled)
	}
	waitR282AugmentRefresh(t, p)
	for _, tc := range []struct {
		err  error
		kind string
	}{{context.DeadlineExceeded, "timeout"}, {errCommunityDragonAugmentBackoff, "backoff_skipped"}, {errors.New("offline"), "no_response"}} {
		got := augmentDescriptionFailure(900, tc.err)
		if got.Status != "retry" || got.ErrorKind != tc.kind {
			t.Fatalf("transient %#v", got)
		}
	}
	healthy := newHexdataBudgetProvider(t, t.TempDir(), championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return hexdataResponse(r, []byte(`{"augments":[{"id":900,"name":"no description"}]}`)), nil
	}))
	missing := healthy.resolveAugmentDescription(context.Background(), 900, nil)
	if missing.Status != "unavailable" || missing.ErrorKind != "not_found" {
		t.Fatalf("healthy absence %#v", missing)
	}
}
func waitR282AugmentRefresh(t *testing.T, p *championProvider) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		p.augmentDescriptions.Lock()
		n := len(p.augmentDescriptions.active)
		p.augmentDescriptions.Unlock()
		if n == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("refresh did not settle")
		}
		time.Sleep(time.Millisecond)
	}
}
func TestR282BundledDescriptionSnapshot(t *testing.T) {
	rows := bundledAugmentDescriptions()
	if len(rows) != 225 {
		t.Fatalf("snapshot count %d", len(rows))
	}
	for _, id := range []int64{89, 93, 347, 359} {
		item := augmentDescriptionFromRows(id, rows, "bundled")
		if item.Status != "ok" {
			t.Fatalf("ID %d missing: %#v", id, item)
		}
	}
	data, err := bundledArenaData.ReadFile("data/augment_descriptions_20261017.json")
	if err != nil || !json.Valid(data) {
		t.Fatal("snapshot not embedded")
	}
}

func TestR282LCUDescriptionCandidateFallback(t *testing.T) {
	for _, tc := range []struct{ body, source, kind string }{{`[]`, "bundled", "invalid_shape"}, {`{"wrong":true}`, "bundled", "invalid_shape"}, {`[{"id":93,"name":"本机","description":"本地说明"}]`, "lcu", ""}, {`[{"id":1,"name":"其他","description":"其他说明"}]`, "bundled", "not_found"}} {
		t.Run(tc.kind+tc.source, func(t *testing.T) {
			p := newHexdataBudgetProvider(t, t.TempDir(), championRoundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded }))
			client := &LCUClient{baseURL: "https://fixture", token: "fixture", http: &http.Client{Transport: championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != "/lol-game-data/assets/v1/cherry-augments.json" {
					t.Errorf("unexpected LCU path %s", r.URL.Path)
				}
				return hexdataResponse(r, []byte(tc.body)), nil
			})}}
			a := &app{connected: true, lcu: client, champions: p}
			w := httptest.NewRecorder()
			a.handleGameplayAugmentDescriptions(w, httptest.NewRequest("GET", "/api/gameplay/augment-descriptions?ids=93", nil))
			var response struct {
				Items []gameplayAugmentDescription `json:"items"`
			}
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &response) != nil || len(response.Items) != 1 {
				t.Fatalf("response %d %s", w.Code, w.Body.String())
			}
			got := response.Items[0]
			if got.Source != tc.source || got.LCUErrorKind != tc.kind {
				t.Fatalf("candidate fallback %#v", got)
			}
			waitR282AugmentRefresh(t, p)
		})
	}
}

func TestR282StaleCatalogMissIsStillRetryable(t *testing.T) {
	p := newHexdataBudgetProvider(t, t.TempDir(), championRoundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, errors.New("offline") }))
	key := championCacheKey(communityDragonHost, "/latest/cdragon/arena/zh_cn.json", "", "application/json")
	_, err := p.cache.load(context.Background(), key, -time.Hour, 24*time.Hour, false, func(context.Context) ([]byte, error) { return []byte(`{"augments":[]}`), nil })
	if err != nil {
		t.Fatal(err)
	}
	got := p.onlineAugmentDescription(context.Background(), 900)
	if got.Status != "retry" || got.ErrorKind != "no_response" {
		t.Fatalf("stale catalog cannot prove not_found: %#v", got)
	}
}

func TestR282NewClientDiagnosticsPassCompleteHTTPHandler(t *testing.T) {
	tests := []struct {
		event, reason, fields string
		status                int
	}{
		{"build_spells_client", "render", `{"mode":"arena","spell1Id_missing":1,"spell1Id_zero":2,"spell1Id_value":13,"spell2Id_missing":1,"spell2Id_zero":2,"spell2Id_value":13}`, 204},
		{"augment_description_client", "ok", `{"augment_id":93,"retry_count":1,"source":"bundled","error_kind":"none","gave_up":false}`, 204},
		{"augment_description_client", "retry", `{"augment_id":93,"retry_count":3,"source":"none","error_kind":"backoff_skipped","gave_up":true}`, 204},
		{"augment_description_client", "unavailable", `{"augment_id":93,"retry_count":0,"source":"none","error_kind":"not_found","gave_up":false}`, 204},
		{"build_spells_client", "render", `{"mode":"arena","playerRef":"private"}`, 400},
		{"augment_description_client", "unknown", `{}`, 400},
	}
	for _, tc := range tests {
		t.Run(tc.event+tc.reason+strconv.Itoa(tc.status), func(t *testing.T) {
			w := httptest.NewRecorder()
			body := `{"event":"` + tc.event + `","reason":"` + tc.reason + `","r278":` + tc.fields + `}`
			(&app{}).handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(body)))
			if w.Code != tc.status {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
}
