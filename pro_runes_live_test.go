package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// Opt-in, public read-only integration. Raw captures remain outside the repo.
func TestProRuneLiveCaptureReplay(t *testing.T) {
	root := os.Getenv("R75_CAPTURE")
	if root == "" {
		t.Skip("set R75_CAPTURE to the public capture directory")
	}
	cp := proTestProvider()
	// Captured public catalog makes offline replay include real opponent IDs.
	if data, err := os.ReadFile(filepath.Join(root, "champion-metadata.json")); err == nil {
		var metadata map[int]championMetadata
		if json.Unmarshal(data, &metadata) == nil {
			cp.championMeta = metadata
			for id, m := range metadata {
				cp.championIDs[strings.ToLower(m.Key)] = id
			}
		}
	}
	var mu sync.Mutex
	requests := []string{}
	cp.client = &http.Client{Transport: championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		name := "schedule"
		if r.URL.Query().Get("pageToken") != "" {
			name = "schedule-80"
		}
		if strings.HasSuffix(r.URL.Path, "getEventDetails") {
			name = "event-" + r.URL.Query().Get("id")
		}
		if strings.Contains(r.URL.Path, "/window/") {
			name = "window-" + filepath.Base(r.URL.Path)
			if r.URL.Query().Get("startingTime") != "" {
				name = "end-" + filepath.Base(r.URL.Path)
			}
		}
		if strings.Contains(r.URL.Path, "/details/") {
			name = "details-" + filepath.Base(r.URL.Path)
		}
		if r.Header.Get("Cookie") != "" || r.Header.Get("x-riot-token") != "" || r.Header.Get("Authorization") != "" {
			t.Error("V8 private headers")
		}
		mu.Lock()
		requests = append(requests, name)
		mu.Unlock()
		data, err := os.ReadFile(filepath.Join(root, name+".json"))
		if err != nil || len(data) == 0 {
			return nil, fmt.Errorf("capture missing %s", name)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data))), Header: http.Header{}}, nil
	})}
	p := newProRuneProvider(cp, &localStore{root: t.TempDir()}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	p.refresh(ctx, time.Date(2026, 9, 9, 4, 16, 0, 0, time.UTC))
	for _, req := range requests {
		if strings.HasPrefix(req, "details-") || strings.HasPrefix(req, "end-") {
			t.Fatal("index eagerly supplements", req)
		}
	}
	if len(p.snapshot.Games) < 90 {
		t.Fatal("missing full index", len(p.snapshot.Games), p.failed)
	}
	games := []proRuneIndexGame{}
	for _, g := range p.snapshot.Games {
		games = append(games, g)
	}
	proParallel(ctx, games, func(g proRuneIndexGame) { p.terminal(ctx, g) })
	total, unknown, rejected := 0, 0, 0
	for _, e := range p.snapshot.Events {
		winners, ok := proSeriesWinners(e, p.snapshot.Games, p.ends)
		if !ok {
			rejected++
			continue
		}
		for _, g := range e.Match.Games {
			if _, ok := p.snapshot.Games[g.ID]; ok {
				total++
				if winners[g.ID] == "" {
					unknown++
				}
			}
		}
	}
	t.Logf("UTC window 2026-08-10T04:16Z..2026-09-09T04:16Z: games=%d unknown=%d rate=%.2f%% rejected=%d indexFailures=%d", total, unknown, 100*float64(unknown)/float64(total), rejected, p.failed)
	start := time.Now()
	res := p.recommend(ctx, 69, "mid")
	cold := time.Since(start)
	start = time.Now()
	hot := p.recommend(ctx, 69, "mid")
	warm := time.Since(start)
	if len(res.Pros) == 0 || len(hot.Pros) == 0 {
		t.Fatal("production replay no champion hit", res)
	}
	counts := map[string]int{}
	for _, row := range hot.Pros {
		counts[row.Title]++
		if counts[row.Title] > 5 || row.Position != "中路" {
			t.Fatalf("per-player or selected-position contract violated: %+v", row)
		}
	}
	if warm >= 100*time.Millisecond {
		t.Fatal("V7 replay warm exceeds budget", warm)
	}
	t.Logf("Production capture replay rows=%d fill=%s hot=%s", len(res.Pros), cold, warm)
	data, _ := json.MarshalIndent(res, "", "  ")
	os.WriteFile(filepath.Join(root, "production-response.json"), data, 0600)
}

func TestProRuneLiveSupplementLatency(t *testing.T) {
	if os.Getenv("R75_LIVE") != "1" {
		t.Skip("set R75_LIVE=1 for public network requests")
	}
	var candidates []proRuneIndexGame
	for _, series := range proCaptured(t) {
		for _, g := range series.Event.Match.Games {
			w, ok := series.Windows[g.ID]
			if ok {
				candidates = append(candidates, proRuneIndexGame{ID: g.ID, MatchID: series.Event.ID, LeagueID: series.Event.League.ID, Start: w.Frames[0].Timestamp, Metadata: w.Metadata})
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Start.After(candidates[j].Start) })
	cp := proTestProvider()
	base := cp.httpClient().Transport
	if base == nil {
		base = http.DefaultTransport
	}
	var mu sync.Mutex
	audit := []string{}
	cp.client = &http.Client{Transport: championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("x-riot-token") != "" || r.Header.Get("Authorization") != "" || strings.Contains(strings.ToLower(r.URL.String()), "puuid") {
			t.Error("V8 leak")
		}
		mu.Lock()
		audit = append(audit, r.Method+" "+r.URL.String()+" headers="+fmt.Sprint(r.Header))
		mu.Unlock()
		return base.RoundTrip(r)
	})}
	times := []float64{}
	hotTimes := []float64{}
	for batch := 0; batch < 5; batch++ {
		p := newProRuneProvider(cp, nil, nil)
		games := candidates[batch*5 : batch*5+5]
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		failures := 0
		start := time.Now()
		proHitParallel(ctx, games, func(g proRuneIndexGame) {
			if _, err := p.supplement(ctx, g); err != nil {
				mu.Lock()
				failures++
				mu.Unlock()
				t.Log("fill error", g.ID, err)
			}
		})
		elapsed := float64(time.Since(start).Microseconds()) / 1000
		if failures == 0 {
			times = append(times, elapsed)
		}
		start = time.Now()
		proHitParallel(ctx, games, func(g proRuneIndexGame) { p.supplement(ctx, g) })
		hotTimes = append(hotTimes, float64(time.Since(start).Microseconds())/1000)
		cancel()
		t.Logf("batch %d 5-game two-stage fill %.3fms failed=%d hot=%.3fms", batch+1, elapsed, failures, hotTimes[len(hotTimes)-1])
	}
	sort.Float64s(times)
	sort.Float64s(hotTimes)
	if len(times) == 0 {
		t.Fatal("no completed live fills")
	}
	t.Logf("V7 measured successful batches=%d P50=%.3fms hotP50=%.3fms targetMet=%v", len(times), times[len(times)/2], hotTimes[len(hotTimes)/2], times[len(times)/2] < 1500)
	if path := os.Getenv("R75_CAPTURE"); path != "" {
		os.WriteFile(filepath.Join(path, "runtime-request-audit.txt"), []byte(strings.Join(audit, "\n")), 0600)
	}
}

func TestProRuneLiveIndex(t *testing.T) {
	if os.Getenv("R75_LIVE") != "1" {
		t.Skip("set R75_LIVE=1 for public index validation")
	}
	cp := proTestProvider()
	base := cp.httpClient().Transport
	if base == nil {
		base = http.DefaultTransport
	}
	var mu sync.Mutex
	audit := []string{}
	cp.client = &http.Client{Transport: championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("x-riot-token") != "" || r.Header.Get("Authorization") != "" || strings.Contains(strings.ToLower(r.URL.String()), "puuid") {
			t.Error("V8 credentials")
		}
		mu.Lock()
		audit = append(audit, r.Method+" "+r.URL.String()+" headers="+fmt.Sprint(r.Header))
		mu.Unlock()
		return base.RoundTrip(r)
	})}
	// Normal app already has a full champion catalog; this isolated provider does not.
	catalogCtx, catalogCancel := context.WithTimeout(context.Background(), 25*time.Second)
	if _, err := cp.loadCatalog(catalogCtx); err != nil {
		t.Log("catalog preload unavailable", err)
	}
	catalogCancel()
	if root := os.Getenv("R75_CAPTURE"); root != "" {
		data, _ := json.Marshal(cp.championMeta)
		os.WriteFile(filepath.Join(root, "champion-metadata.json"), data, 0600)
	}
	p := newProRuneProvider(cp, &localStore{root: filepath.Join(os.TempDir(), "r75-production-cache")}, nil)
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	p.refresh(ctx, time.Now())
	cancel()
	t.Logf("actual production index elapsed=%s games=%d matches=%d failures=%d reason=%s", time.Since(start), len(p.snapshot.Games), len(p.snapshot.Events), p.failed, p.reason)
	if len(p.snapshot.Games) == 0 {
		t.Fatal("no production games")
	}
	games := []proRuneIndexGame{}
	for _, g := range p.snapshot.Games {
		games = append(games, g)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	proParallel(ctx, games, func(g proRuneIndexGame) {
		if _, err := p.terminal(ctx, g); err != nil {
			t.Log("terminal unavailable", g.ID, err)
		}
	})
	total, unknown, rejected := 0, 0, 0
	for _, e := range p.snapshot.Events {
		winners, ok := proSeriesWinners(e, p.snapshot.Games, p.ends)
		if !ok {
			rejected++
			continue
		}
		for _, g := range e.Match.Games {
			if _, ok := p.snapshot.Games[g.ID]; ok {
				total++
				if winners[g.ID] == "" {
					unknown++
				}
			}
		}
	}
	t.Logf("actual UTC window games=%d unknown=%d rate=%.2f%% rejected=%d", total, unknown, 100*float64(unknown)/float64(total), rejected)
	for _, id := range []int{69, 103} {
		start = time.Now()
		recommendCtx, recommendCancel := context.WithTimeout(context.Background(), 25*time.Second)
		res := p.recommend(recommendCtx, id, "mid")
		recommendCancel()
		t.Logf("actual recommend hero=%d rows=%d elapsed=%s reason=%s", id, len(res.Pros), time.Since(start), res.Reason)
	}
	if root := os.Getenv("R75_CAPTURE"); root != "" {
		os.WriteFile(filepath.Join(root, "runtime-index-audit.txt"), []byte(strings.Join(audit, "\n")), 0600)
	}
}
