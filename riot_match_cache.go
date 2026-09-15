package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

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
	c := newPublicBinaryCache(nil, "", riotMatchCacheMax, 128<<20)
	if p != nil && p.cache != nil && p.cache.dir != "" {
		c = newPublicBinaryCache(&localStore{root: filepath.Dir(p.cache.dir)}, "riot-matches", riotMatchCacheMax, 128<<20)
	}
	return c
}

func validRiotMatchID(id string) bool {
	if !strings.HasPrefix(id, "KR_") || len(id) > 32 {
		return false
	}
	n, err := strconv.ParseUint(strings.TrimPrefix(id, "KR_"), 10, 64)
	return err == nil && n > 0
}

func (p *riotProvider) persistRiotMatch(key string, match *riotMatch) {
	data, err := json.Marshal(match)
	if err != nil {
		return
	}
	hash := sha256.Sum256(data)
	now := time.Now()
	_ = p.matchDisk.writeDisk(championCacheEnvelope{Schema: championCacheSchema, Key: key,
		FetchedAt: now, ExpiresAt: now.Add(365 * 24 * time.Hour), StaleUntil: now.Add(365 * 24 * time.Hour),
		Hash: hex.EncodeToString(hash[:]), Data: data})
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
