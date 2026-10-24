package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestR308RankingPreservesRankAndFirstRouteOpening(t *testing.T) {
	r := r307Resources(t)
	var raw aramkitRawDetail
	b := r307Fixture(t, "champion-157", &raw)
	detail, _, err := parseARAMKitDetail(b, 157, r)
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"unfiltered", "filtered"} {
		view := detail.ARAMKit.Scopes[scope]
		if view.All[0].Rank != raw.Items[scope].All[0].Rank {
			t.Fatal("upstream rank lost")
		}
		for i, a := range raw.Builds[scope].Archetypes {
			route := a.Profiles[0].Routes[0]
			got := view.Archetypes[i]
			if len(got.Starters) != len(route.Starters) || len(got.Boots) != len(route.Boots) {
				t.Fatalf("opening merged across routes: %s", scope)
			}
			if got.Starters[0].Games != route.Starters[0].SampleCount {
				t.Fatal("not first route")
			}
		}
	}
}

func TestR308CaseDifferenceOncePerDataPath(t *testing.T) {
	p := newChampionProvider()
	r := r307Resources(t)
	p.championMeta = r.Metadata
	c := p.aramkitClient()
	b := r307Fixture(t, "champion-identities", nil)
	c.memory = map[string][]byte{"resources/fixture/zh-CN/resources/champions.json": b}
	var mu sync.Mutex
	count := 0
	p.diag = func(fields map[string]any) {
		mu.Lock()
		defer mu.Unlock()
		if fields["event"] == "aramkit_champion_key_case_diff" {
			count++
		}
	}
	for _, dataPath := range []string{"data/one", "data/one", "data/two", "data/two"} {
		if _, err := p.loadARAMKitChampionResources(context.Background(), aramkitVersion{DataPath: dataPath, ResourcePath: "resources/fixture"}); err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if count != 2 {
		t.Fatalf("case differences logged %d times", count)
	}
}

func TestR308HedgeDiagnosticsBothWinners(t *testing.T) {
	for _, winner := range []string{"primary.test", "backup.test"} {
		t.Run(winner, func(t *testing.T) {
			both := make(chan struct{})
			cancelled := make(chan struct{})
			client := newAramkitTestClient(r307Transport(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host == "backup.test" {
					close(both)
				}
				if req.URL.Host == winner {
					<-both
					return aramkitJSONResponse(200, `{"code":200,"data":{"rating":2000}}`), nil
				}
				<-req.Context().Done()
				close(cancelled)
				return nil, req.Context().Err()
			}))
			client.baseURL = "https://primary.test"
			client.fallbackURL = "https://backup.test"
			client.hedgeDelay = time.Millisecond
			client.totalTimeout = time.Second
			var mu sync.Mutex
			var fields map[string]any
			client.observe = func(f map[string]any) { mu.Lock(); defer mu.Unlock(); fields = f }
			if !client.lookup(t.Context(), "synthetic-fixture", "0000").Available {
				t.Fatal("lookup failed")
			}
			select {
			case <-cancelled:
			case <-time.After(time.Second):
				t.Fatal("loser not cancelled")
			}
			mu.Lock()
			defer mu.Unlock()
			other := "primary.test"
			if winner == other {
				other = "backup.test"
			}
			if fields["hedged"] != true || fields["host"] != winner || fields["other_host"] != other || fields["other_result"] != "被取消" {
				t.Fatalf("wrong diagnostics: %v", fields)
			}
			if ms, ok := fields["other_ms"].(int64); !ok || ms < 0 || ms > 1000 {
				t.Fatal("bad other_ms")
			}
			payload, _ := json.Marshal(fields)
			if strings.Contains(string(payload), "synthetic-fixture") {
				t.Fatal("identity leak")
			}
		})
	}
}

func TestR308HedgeFailureIsReported(t *testing.T) {
	c := newAramkitTestClient(r307Transport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "primary.test" {
			return nil, fmt.Errorf("offline")
		}
		return aramkitJSONResponse(200, `{"code":200,"data":{"rating":2000}}`), nil
	}))
	c.baseURL = "https://primary.test"
	c.fallbackURL = "https://backup.test"
	out := c.fetch(t.Context(), "synthetic-fixture", "0000")
	if !out.hedged || out.otherHost != "primary.test" || out.otherResult != "失败" {
		t.Fatalf("lost failure: %+v", out)
	}
}

func TestR308HedgeDeadlineKeepsOtherDiagnostics(t *testing.T) {
	c := newAramkitTestClient(r307Transport(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	}))
	c.baseURL = "https://primary.test"
	c.fallbackURL = "https://backup.test"
	c.hedgeDelay = time.Millisecond
	c.totalTimeout = 30 * time.Millisecond
	out := c.fetch(t.Context(), "synthetic-fixture", "0000")
	if !out.hedged || out.otherHost == "" || out.otherResult == "" || out.otherMS < 0 {
		t.Fatalf("deadline lost hedge diagnostics: %+v", out)
	}
}
