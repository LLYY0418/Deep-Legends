package main

// Offline only: all loaders are in-memory; HTTP handlers use httptest recorders.
// These benchmarks must never contact Riot, LCU or any other network service.
import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

var r115BenchScope string

func BenchmarkR115LCURoute(b *testing.B) {
	for _, uri := range []string{"/lol-summoner/v1/current-summoner/summoner-profile", "/LOL-CHAMP-SELECT/v1/session/actions/42", "/lol-gameflow/v1/session"} {
		for _, tc := range []struct {
			name string
			fn   func(LCUEvent) string
		}{{"legacy", r115LegacyScope}, {"current", lcuEventRefreshScope}} {
			b.Run(uri+"/"+tc.name, func(b *testing.B) {
				event := LCUEvent{URI: uri}
				b.ReportAllocs()
				for b.Loop() {
					r115BenchScope = tc.fn(event)
				}
			})
		}
	}
}
func BenchmarkR115Asset(b *testing.B) {
	for _, mode := range []string{"hit", "miss", "negative", "singleflight"} {
		b.Run(mode, func(b *testing.B) {
			a := &app{}
			data := []byte("image fixture")
			ctx := context.Background()
			loader := func(context.Context) ([]byte, error) { return data, nil }
			maxSize := 64
			switch mode {
			case "hit":
				a.loadAsset(ctx, "image", maxSize, 0, loader)
			case "miss":
				maxSize = 0
			case "negative":
				a.assetFailureUntil = map[string]time.Time{"image": time.Now().Add(time.Hour)}
			case "singleflight":
				done := make(chan struct{})
				close(done)
				a.assetFlights = map[string]*assetFlight{"image": {done: done, data: data}}
			}
			b.ReportAllocs()
			for b.Loop() {
				_, err := a.loadAsset(ctx, "image", maxSize, 0, loader)
				if err != nil && !errors.Is(err, errCachedAssetFailure) {
					b.Fatal(err)
				}
			}
		})
	}
}
func BenchmarkR115Overview(b *testing.B) {
	for _, mode := range []string{"hit", "miss", "expired", "evict"} {
		b.Run(mode, func(b *testing.B) {
			cache := newOverviewQueryCache()
			now := time.Now()
			entry := overviewQueryCacheEntry{at: now}
			for i := range overviewQueryCacheMax {
				cache.putLocked(string(rune(i)), entry)
			}
			keys := make([]string, 257)
			for i := range keys {
				keys[i] = strconv.Itoa(i)
			}
			next := 0
			b.ReportAllocs()
			for b.Loop() {
				cache.mu.Lock()
				switch mode {
				case "hit":
					cache.getLocked("a", now)
				case "miss":
					cache.getLocked("absent", now)
				case "expired":
					cache.putLocked("expired", entry)
					cache.getLocked("expired", now.Add(time.Hour))
				case "evict":
					cache.putLocked(keys[next%len(keys)], entry)
					next++
				}
				cache.mu.Unlock()
			}
		})
	}
}
func BenchmarkR115RiotBuckets(b *testing.B) {
	scope := riotRateScope{"fixture", "match"}
	p := &riotProvider{}
	now := time.Now()
	p.limitNow = func() time.Time { return now }
	headers := http.Header{"X-App-Rate-Limit": []string{"15:1,90:120"}, "X-App-Rate-Limit-Count": []string{"0:1,0:120"}, "X-Method-Rate-Limit": []string{"15:1"}, "X-Method-Rate-Limit-Count": []string{"0:1"}}
	p.observeRiotRate(scope, headers, 200)
	ctx := context.WithValue(context.Background(), riotRateScopeKey{}, scope)
	b.ReportAllocs()
	for b.Loop() {
		now = now.Add(2 * time.Minute)
		p.limitMu.Lock()
		p.scopedRiotDelay(ctx, now, true, false)
		p.limitMu.Unlock()
		p.beginRiotRateRequest(scope)
		p.observeRiotRate(scope, headers, 200)
	}
}
func BenchmarkR115RiotQueue(b *testing.B) {
	for _, mode := range []string{"admit", "cancel-follower"} {
		b.Run(mode, func(b *testing.B) {
			p := &riotProvider{}
			ctx := context.Background()
			b.ReportAllocs()
			for b.Loop() {
				release, err := p.enterRiotLimitQueue(ctx)
				if err != nil {
					b.Fatal(err)
				}
				if mode == "cancel-follower" {
					child, cancel := context.WithCancel(ctx)
					done := make(chan error, 1)
					go func() {
						leave, err := p.enterRiotLimitQueue(child)
						if leave != nil {
							leave()
						}
						done <- err
					}()
					for {
						p.limitMu.Lock()
						queued := len(p.limitQueue) == 2
						p.limitMu.Unlock()
						if queued {
							break
						}
						runtime.Gosched()
					}
					cancel()
					if err := <-done; !errors.Is(err, context.Canceled) {
						b.Fatalf("follower not canceled: %v", err)
					}
				}
				release()
			}
		})
	}
}
func BenchmarkR115StaticAssets(b *testing.B) {
	files := fstest.MapFS{"app.js": {Data: []byte(strings.Repeat("const fixture = 'offline';\n", 1000))}}
	for _, mode := range []string{"cold-gzip", "warm-gzip", "etag"} {
		b.Run(mode, func(b *testing.B) {
			h := newStaticAssetHandler(files)
			req := httptest.NewRequest("GET", "/app.js", nil)
			req.Header.Set("Accept-Encoding", "gzip")
			warm := httptest.NewRecorder()
			h.ServeHTTP(warm, req)
			if mode == "etag" {
				req.Header.Set("If-None-Match", warm.Header().Get("ETag"))
			}
			b.ReportAllocs()
			for b.Loop() {
				if mode == "cold-gzip" {
					h = newStaticAssetHandler(files)
				}
				h.ServeHTTP(httptest.NewRecorder(), req)
			}
		})
	}
}
