package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const riotMatchDiskEntries = 2000
const riotMatchDiskBytes = 50 << 20

func configuredRiotMatchConcurrency() int {
	n, err := strconv.Atoi(os.Getenv("DEEP_LEGENDS_RIOT_MATCH_CONCURRENCY"))
	if err != nil || n < 1 {
		return 8
	}
	// Raising request parallelism never raises the independent shared limiter.
	// Larger values require an explicit production-key deployment configuration.
	limit := 8
	if os.Getenv("DEEP_LEGENDS_RIOT_KEY_TIER") == "production" {
		limit = 20
	}
	return min(n, limit)
}

func (p *riotProvider) detailConcurrency() int {
	if p.matchConcurrency > 0 {
		return min(p.matchConcurrency, 20)
	}
	return 8
}

func newRiotMatchDiskCache(p *championProvider) *championDataCache {
	c := newPublicBinaryCache(nil, "", riotMatchDiskEntries, riotMatchDiskBytes)
	if p != nil && p.cache != nil && p.cache.dir != "" {
		c = newPublicBinaryCache(&localStore{root: filepath.Dir(p.cache.dir)}, "riot-matches", riotMatchDiskEntries, riotMatchDiskBytes)
	}
	return c
}

func validRiotMatchID(id string) bool {
	parts := strings.SplitN(id, "_", 2)
	if len(parts) != 2 || !isRiotRegion(parts[0]) || parts[0] != strings.ToUpper(riotPlatform(parts[0])) || len(id) > 32 {
		return false
	}
	n, err := strconv.ParseUint(parts[1], 10, 64)
	return err == nil && n > 0 && strconv.FormatUint(n, 10) == parts[1]
}

func (p *riotProvider) persistRiotMatch(key string, match *riotMatch) {
	data, err := json.Marshal(match)
	if err != nil {
		return
	}
	hash := sha256.Sum256(data)
	now := time.Now()
	_ = p.matchDisk.writeDisk(championCacheEnvelope{Schema: championCacheSchema, Key: key,
		FetchedAt: now,
		Hash:      hex.EncodeToString(hash[:]), Data: data})
}

func validRiotMatchContents(match *riotMatch, id string) bool {
	return match != nil && match.Metadata.MatchID == id && len(match.Info.Participants) > 0
}

// The immutable match cache uses access order; other binary caches keep their
// existing policy. Memory hits touch disk too, so frequently used matches stay.
func (p *riotProvider) touchRiotMatchDisk(key string) {
	c := p.matchDisk
	if c == nil || c.dir == "" {
		return
	}
	c.diskMu.Lock()
	defer c.diskMu.Unlock()
	file := c.pathFor(key)
	info, err := os.Lstat(file)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return
	}
	c.initBinaryDiskIndexLocked()
	now := time.Now()
	if os.Chtimes(file, now, now) == nil {
		c.strictEntries[file] = binaryDiskEntry{info.Size(), now}
	}
}

func (p *riotProvider) discardRiotMatchDisk(key string) {
	c := p.matchDisk
	if c == nil || c.dir == "" {
		return
	}
	c.diskMu.Lock()
	defer c.diskMu.Unlock()
	file := c.pathFor(key)
	if os.Remove(file) == nil || c.strictEntries[file].size > 0 {
		c.strictBytes -= c.strictEntries[file].size
		delete(c.strictEntries, file)
	}
}

func (p *riotProvider) clearRiotMatchCaches() error {
	if p == nil {
		return nil
	}
	p.platformMu.Lock()
	defer p.platformMu.Unlock()
	providers := []*riotProvider{p}
	for _, scoped := range p.platforms {
		providers = append(providers, scoped)
	}
	for _, scoped := range providers {
		scoped.cacheMu.Lock()
	}
	defer func() {
		for i := len(providers) - 1; i >= 0; i-- {
			providers[i].cacheMu.Unlock()
		}
	}()
	for _, scoped := range providers {
		scoped.matchGeneration++
		scoped.matchCache = make(map[string]*riotMatch)
		scoped.matchOrder = nil
	}
	c := p.matchDisk
	if c == nil || c.dir == "" {
		return nil
	}
	c.diskMu.Lock()
	defer c.diskMu.Unlock()
	info, err := os.Lstat(c.dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("untrusted match cache directory")
	}
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if len(name) != 69 || !strings.HasSuffix(name, ".json") {
			continue
		}
		if _, err := hex.DecodeString(strings.TrimSuffix(name, ".json")); err != nil {
			continue
		}
		if err := os.Remove(filepath.Join(c.dir, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	c.strictEntries, c.strictBytes = nil, 0
	return nil
}

func (a *app) handleClearRiotMatchCache(w http.ResponseWriter, r *http.Request) {
	if err := a.riot.clearRiotMatchCaches(); err != nil {
		http.Error(w, "缓存清理失败，请重试", http.StatusInternalServerError)
		return
	}
	respondJSON(w, map[string]bool{"cleared": true})
}

func (t *riotOverviewCostTracker) detailInFlight(delta int) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.inFlight += delta
	t.peakInFlight = max(t.peakInFlight, t.inFlight)
	t.mu.Unlock()
}
