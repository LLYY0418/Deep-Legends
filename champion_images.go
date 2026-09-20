package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

const publicImageTimeout = 8 * time.Second

const championImageAccept = "image/avif,image/webp,image/png,image/jpeg,image/*;q=0.8"

func newPublicBinaryCache(store *localStore, directory string, entries int, bytes int64) *championDataCache {
	c := newChampionDataCache(nil)
	c.strictDisk, c.diskMaxEntries, c.diskMaxBytes = true, entries, bytes
	if store != nil {
		c.dir = filepath.Join(store.root, directory)
		if err := os.MkdirAll(c.dir, 0o700); err != nil {
			c.migrationErr = err
		} else if info, err := os.Lstat(c.dir); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			c.migrationErr = errors.New("untrusted binary cache directory")
			c.dir = ""
		}
	}
	return c
}

type assetCacheObservationKey struct{}
type assetCacheObservation struct{ state string }

func recordAssetCacheState(ctx context.Context, state string) {
	if observation, _ := ctx.Value(assetCacheObservationKey{}).(*assetCacheObservation); observation != nil {
		observation.state = state
	}
}

func (a *app) loadChampionRemoteAsset(ctx context.Context, p *championProvider, source, host, path string) ([]byte, error) {
	started := time.Now()
	observation := &assetCacheObservation{state: "memory"}
	ctx = context.WithValue(ctx, assetCacheObservationKey{}, observation)
	data, err := a.loadAsset(ctx, "champion-asset:"+source+":"+host+":"+path, championImageMax, 5*time.Second, func(ctx context.Context) ([]byte, error) {
		observation.state = "miss"
		return p.fetch(ctx, host, path, nil, championImageMax, championImageAccept)
	})
	if err != nil && observation.state == "memory" {
		observation.state = "negative"
	}
	p.observeAssetFetch(host, observation.state, time.Since(started), err)
	return data, err
}

type assetFetchWindow struct {
	durations       []int64
	hits            map[string]int
	count, failures int
}
type assetFetchStats struct {
	mu      sync.Mutex
	windows map[string]*assetFetchWindow
	timer   *time.Timer
}

// At most 512 latency samples per host/window; counts remain exact even during
// image-heavy scrolling. Export flushes the window, including a lone request.
func (p *championProvider) observeAssetFetch(host, state string, elapsed time.Duration, err error) {
	if p.diag == nil || isCancellation(err) {
		return
	}
	s := &p.assetStats
	s.mu.Lock()
	if s.windows == nil {
		s.windows = make(map[string]*assetFetchWindow)
	}
	w := s.windows[host]
	if w == nil {
		w = &assetFetchWindow{hits: make(map[string]int)}
		s.windows[host] = w
	}
	w.count++
	w.hits[state]++
	if err != nil {
		w.failures++
	}
	if len(w.durations) < 512 {
		w.durations = append(w.durations, elapsed.Microseconds())
	}
	if s.timer == nil {
		s.timer = time.AfterFunc(10*time.Second, p.flushAssetFetch)
	}
	s.mu.Unlock()
}

func (p *championProvider) flushAssetFetch() {
	s := &p.assetStats
	s.mu.Lock()
	windows := s.windows
	s.windows = nil
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.mu.Unlock()
	for host, w := range windows {
		sort.Slice(w.durations, func(i, j int) bool { return w.durations[i] < w.durations[j] })
		p.diag(map[string]any{"event": "asset_fetch", "host": host, "requests": w.count,
			"cache": w.hits, "hit_rate": float64(w.hits["memory"]+w.hits["disk"]+w.hits["shared"]) / float64(w.count),
			"p50_ms": float64(w.durations[(len(w.durations)-1)*50/100]) / 1000,
			"p90_ms": float64(w.durations[(len(w.durations)-1)*90/100]) / 1000, "failures": w.failures})
	}
}

type imageWarmState struct {
	mu         sync.Mutex
	started    bool
	cancel     context.CancelFunc
	timer      *time.Timer
	generation uint64
	complete   bool
}

// Foreground requests cancel an active prefetch and restart the idle delay.
// Prefetches use exactly the same final versioned URL/cache as visible icons.
func (a *app) scheduleItemIconWarmup(enable bool) {
	p := a.champions
	if p == nil || p.imageCache == nil || p.imageCache.dir == "" || os.Getenv("DEEP_LEGENDS_ITEM_PREWARM") == "0" {
		return
	}
	w := &p.imageWarm
	w.mu.Lock()
	defer w.mu.Unlock()
	w.started = w.started || enable
	if !w.started || w.complete {
		return
	}
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	if w.timer != nil {
		w.timer.Stop()
	}
	w.generation++
	generation := w.generation
	w.timer = time.AfterFunc(3*time.Second, func() {
		w.mu.Lock()
		if generation != w.generation {
			w.mu.Unlock()
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		w.cancel = cancel
		w.mu.Unlock()
		err := a.warmRankedItemIcons(ctx)
		w.mu.Lock()
		if generation == w.generation {
			w.cancel = nil
			w.complete = err == nil
		}
		w.mu.Unlock()
		cancel()
	})
}

func rankedItemWarmPaths(descriptions map[string]championAssetDescription, scores map[int]int) []string {
	ids := make([]int, 0, len(scores))
	for id := range scores {
		if descriptions["item/"+strconv.Itoa(id)+".png"].Path != "" {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		if scores[ids[i]] == scores[ids[j]] {
			return ids[i] < ids[j]
		}
		return scores[ids[i]] > scores[ids[j]]
	})
	paths := make([]string, 0, min(120, len(ids)))
	for _, id := range ids[:min(120, len(ids))] {
		paths = append(paths, descriptions["item/"+strconv.Itoa(id)+".png"].Path)
	}
	return paths
}

func (a *app) warmRankedItemIcons(ctx context.Context) error {
	p := a.champions
	descriptions, err := p.loadStaticDescriptions(ctx)
	if err != nil {
		return err
	}
	ranking, err := p.loadRanked(ctx, "emerald_plus", "all")
	if err != nil {
		return err
	}
	sort.SliceStable(ranking.Rows, func(i, j int) bool { return ranking.Rows[i].Play > ranking.Rows[j].Play })
	// A bounded sample of the twenty most played ranked champions. Sum observed
	// item appearances (not arbitrary item IDs); no whole-roster build crawl.
	scores := map[int]int{}
	var mu sync.Mutex
	var wait sync.WaitGroup
	slots := make(chan struct{}, 6)
	for _, row := range ranking.Rows[:min(20, len(ranking.Rows))] {
		if ctx.Err() != nil {
			break
		}
		wait.Add(1)
		go func(row championRankingRow) {
			defer wait.Done()
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-slots }()
			_, path, query, err := opggDetailRequest("ranked", row.ChampionID, row.Position, "emerald_plus")
			if err != nil {
				return
			}
			data, err := p.fetch(ctx, opggChampionHost, path, query, championJSONMax, "application/json")
			if err != nil {
				return
			}
			var detail opggStructuredDetail
			if json.Unmarshal(data, &detail) != nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, rows := range [][]opggMetric{detail.Data.StarterItems, detail.Data.Boots, detail.Data.CoreItems} {
				for _, metric := range rows {
					for _, id := range metric.IDs {
						scores[id] += metric.Play
					}
				}
			}
		}(row)
	}
	wait.Wait()
	for _, path := range rankedItemWarmPaths(descriptions, scores) {
		if ctx.Err() != nil {
			break
		}
		wait.Add(1)
		go func(path string) {
			defer wait.Done()
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-slots }()
			_, _ = a.loadChampionRemoteAsset(ctx, p, "ddragon", dataDragonHost, path)
		}(path)
	}
	wait.Wait()
	return ctx.Err()
}
