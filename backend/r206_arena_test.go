package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestR206ArenaRetryPartialAndAllFailed(t *testing.T) {
	for _, scenario := range []string{"first-timeout", "yourgg-failed", "opgg-failed", "both-failed", "yourgg-404"} {
		t.Run(scenario, func(t *testing.T) {
			p := newChampionProvider()
			p.cache = newChampionDataCache(nil)
			p.patch = "16.19.1"
			p.arenaVersion, p.arenaVersionAt = "16.19", time.Now()
			p.remoteAugments = []gameplayAugment{{ID: 901, Name: "夹具强化", Rarity: "kSilver"}}
			p.static["item/3153.png"] = championAssetDescription{Name: "夹具装备"}
			var mu sync.Mutex
			calls := map[string]int{}
			attempt2 := map[string]bool{}
			p.diag = func(row map[string]any) {
				mu.Lock()
				defer mu.Unlock()
				if row["event"] == "champion_upstream" && row["attempt"] == 2 {
					attempt2[row["host"].(string)] = true
				}
			}
			p.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
				mu.Lock()
				calls[r.URL.Host]++
				count := calls[r.URL.Host]
				mu.Unlock()
				fail := scenario == "both-failed" || scenario == "yourgg-failed" && r.URL.Host == yourGGArenaHost || scenario == "opgg-failed" && r.URL.Host == opggChampionHost || scenario == "first-timeout" && count == 1
				if fail {
					return nil, context.DeadlineExceeded
				}
				if scenario == "yourgg-404" && r.URL.Host == yourGGArenaHost {
					return updateResponse(404, []byte(`{}`)), nil
				}
				body := `{"data":{"summary":{"id":67,"average_stats":{"play":1000,"win":550}},"core_items":[{"ids":[3153],"play":100,"win":60}]},"meta":{"version":"16.19"}}`
				if r.URL.Host == yourGGArenaHost {
					body = `{"success":true,"statusCode":200,"response":{"version":"16.19","coreItems":[{"itemId":3153,"tier":"A","score":80,"winRate":0.6,"pickRate":0.2,"matches":300}],"prismaticItems":[],"augments":[{"augmentId":901,"tier":"A","score":80,"winRate":0.6,"averagePlacement":3,"firstPlacementRate":0.2,"matches":300}]}}`
				}
				return updateResponse(200, []byte(body)), nil
			})}
			a := &app{champions: p}
			w := httptest.NewRecorder()
			a.handleChampionDetail(w, httptest.NewRequest("GET", "/api/champions/detail?mode=arena&champion=67", nil))
			if scenario == "both-failed" {
				if w.Code != 502 {
					t.Fatal("all failed must be 502", w.Code)
				}
				return
			}
			if w.Code != 200 {
				t.Fatal("partial/success not 200", w.Code, w.Body.String())
			}
			mu.Lock()
			defer mu.Unlock()
			if scenario == "first-timeout" && (!attempt2[opggChampionHost] || !attempt2[yourGGArenaHost] || calls[opggChampionHost] != 2 || calls[yourGGArenaHost] != 2) {
				t.Fatal("missing immediate retry", calls, attempt2)
			}
			if scenario == "yourgg-failed" && (!strings.Contains(w.Body.String(), `"failedBlocks":["items","augments"]`) || !strings.Contains(w.Body.String(), `"coreItems"`)) {
				t.Fatal("failed blocks lost successful build", w.Body.String())
			}
			if scenario == "opgg-failed" && !strings.Contains(w.Body.String(), `"synergies"`) {
				t.Fatal("successful aggregate lost", w.Body.String())
			}
			if scenario == "yourgg-404" && calls[yourGGArenaHost] != 1 {
				t.Fatal("4xx retried", calls)
			}
		})
	}
}

func TestR206ArenaAttemptBudgetsAndConnectionRetry(t *testing.T) {
	p := newChampionProvider()
	calls := 0
	p.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		deadline, ok := r.Context().Deadline()
		remaining := time.Until(deadline)
		want := 3 * time.Second
		if calls == 2 {
			want = 5 * time.Second
		}
		if !ok || remaining < want-100*time.Millisecond || remaining > want {
			t.Fatal("attempt budget", remaining)
		}
		if calls == 1 {
			return nil, errors.New("connection refused")
		}
		return updateResponse(200, []byte(`{}`)), nil
	})}
	if _, _, err := p.fetchArenaWithMetadata(context.Background(), opggChampionHost, "/api/fixture", nil, ""); err != nil || calls != 2 {
		t.Fatal(err, calls)
	}
}

func TestR206ArenaBlockRetryOnlyQueriesRequestedSource(t *testing.T) {
	p := newChampionProvider()
	calls := 0
	p.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != yourGGArenaHost {
			t.Fatal("item retry queried another source")
		}
		return updateResponse(200, []byte(`{"response":{"coreItems":[{"itemId":3153,"tier":"A","score":80,"matches":300}],"prismaticItems":[]}}`)), nil
	})}
	a := &app{champions: p}
	w := httptest.NewRecorder()
	a.handleChampionDetail(w, httptest.NewRequest("GET", "/api/champions/detail?mode=arena&champion=67&block=items", nil))
	if w.Code != 200 || calls != 1 {
		t.Fatal(w.Code, calls, w.Body.String())
	}
}
