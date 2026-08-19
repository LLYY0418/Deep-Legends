package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	championDataCacheDirectory = "champion-data"
	championCacheSchema        = 1
	championCacheMaxEntry      = 8 << 20
	championCacheMaxEntries    = 256
	championCacheMaxBytes      = 64 << 20
	championMemoryMaxEntries   = 128
	championMemoryMaxBytes     = 32 << 20
)

type championCacheEnvelope struct {
	Schema     int       `json:"schema"`
	Key        string    `json:"key"`
	FetchedAt  time.Time `json:"fetchedAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
	StaleUntil time.Time `json:"staleUntil"`
	Hash       string    `json:"hash"`
	Data       []byte    `json:"data"`
}

type championCacheFlight struct {
	done chan struct{}
	data []byte
	err  error
}

type championDataCache struct {
	dir     string
	mu      sync.Mutex
	entries map[string]championCacheEnvelope
	order   []string
	bytes   int
	flights map[string]*championCacheFlight
}

func newChampionDataCache(store *localStore) *championDataCache {
	cache := &championDataCache{entries: make(map[string]championCacheEnvelope), flights: make(map[string]*championCacheFlight)}
	if store != nil {
		cache.dir = filepath.Join(store.root, championDataCacheDirectory)
	}
	return cache
}

func championCacheKey(host, requestPath, query, accept string) string {
	return "v1|" + host + "|" + requestPath + "|" + query + "|" + accept
}

func (c *championDataCache) load(ctx context.Context, key string, ttl, staleFor time.Duration, loader func(context.Context) ([]byte, error)) ([]byte, error) {
	now := time.Now()
	c.mu.Lock()
	if entry, ok := c.entries[key]; ok {
		if now.Before(entry.ExpiresAt) {
			c.touchLocked(key)
			data := append([]byte(nil), entry.Data...)
			c.mu.Unlock()
			return data, nil
		}
	}
	if flight := c.flights[key]; flight != nil {
		done := flight.done
		c.mu.Unlock()
		select {
		case <-done:
			return append([]byte(nil), flight.data...), flight.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	c.mu.Unlock()

	diskEntry, _ := c.readDisk(key)
	c.mu.Lock()
	if entry, ok := c.entries[key]; ok && time.Now().Before(entry.ExpiresAt) {
		c.touchLocked(key)
		data := append([]byte(nil), entry.Data...)
		c.mu.Unlock()
		return data, nil
	}
	if existing := c.flights[key]; existing != nil {
		done := existing.done
		c.mu.Unlock()
		select {
		case <-done:
			return append([]byte(nil), existing.data...), existing.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if len(diskEntry.Data) > 0 {
		c.storeMemoryLocked(key, diskEntry)
		if now.Before(diskEntry.ExpiresAt) {
			data := append([]byte(nil), diskEntry.Data...)
			c.mu.Unlock()
			return data, nil
		}
	}
	stale := c.entries[key]
	flight := &championCacheFlight{done: make(chan struct{})}
	c.flights[key] = flight
	c.mu.Unlock()

	data, err := loader(ctx)
	if err == nil && len(data) > 0 {
		hash := sha256.Sum256(data)
		entry := championCacheEnvelope{
			Schema: championCacheSchema, Key: key, FetchedAt: now, ExpiresAt: now.Add(ttl),
			StaleUntil: now.Add(ttl + staleFor), Hash: hex.EncodeToString(hash[:]), Data: append([]byte(nil), data...),
		}
		c.mu.Lock()
		c.storeMemoryLocked(key, entry)
		flight.data = append([]byte(nil), data...)
		delete(c.flights, key)
		close(flight.done)
		c.mu.Unlock()
		_ = c.writeDisk(entry)
		return data, nil
	}
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && len(stale.Data) > 0 && now.Before(stale.StaleUntil) {
		data = append([]byte(nil), stale.Data...)
		err = nil
	}
	c.mu.Lock()
	flight.data, flight.err = append([]byte(nil), data...), err
	delete(c.flights, key)
	close(flight.done)
	c.mu.Unlock()
	return data, err
}

func (c *championDataCache) touchLocked(key string) {
	for index, current := range c.order {
		if current == key {
			c.order = append(append(c.order[:index], c.order[index+1:]...), key)
			return
		}
	}
	c.order = append(c.order, key)
}

func (c *championDataCache) storeMemoryLocked(key string, entry championCacheEnvelope) {
	if previous, ok := c.entries[key]; ok {
		c.bytes -= len(previous.Data)
	}
	c.entries[key] = entry
	c.bytes += len(entry.Data)
	c.touchLocked(key)
	for len(c.entries) > championMemoryMaxEntries || c.bytes > championMemoryMaxBytes {
		if len(c.order) == 0 {
			break
		}
		oldest := c.order[0]
		c.order = c.order[1:]
		if previous, ok := c.entries[oldest]; ok {
			c.bytes -= len(previous.Data)
			delete(c.entries, oldest)
		}
	}
}

func (c *championDataCache) pathFor(key string) string {
	hash := sha256.Sum256([]byte(key))
	return filepath.Join(c.dir, hex.EncodeToString(hash[:])+".json")
}

func (c *championDataCache) readDisk(key string) (championCacheEnvelope, error) {
	if c.dir == "" {
		return championCacheEnvelope{}, os.ErrNotExist
	}
	path := c.pathFor(key)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > championCacheMaxEntry*2 {
		return championCacheEnvelope{}, errors.New("cache miss")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return championCacheEnvelope{}, err
	}
	var entry championCacheEnvelope
	if json.Unmarshal(data, &entry) != nil || entry.Schema != championCacheSchema || entry.Key != key || len(entry.Data) == 0 || len(entry.Data) > championCacheMaxEntry {
		_ = os.Remove(path)
		return championCacheEnvelope{}, errors.New("invalid cache entry")
	}
	hash := sha256.Sum256(entry.Data)
	if entry.Hash != hex.EncodeToString(hash[:]) {
		_ = os.Remove(path)
		return championCacheEnvelope{}, errors.New("invalid cache hash")
	}
	return entry, nil
}

func (c *championDataCache) writeDisk(entry championCacheEnvelope) error {
	if c.dir == "" || len(entry.Data) > championCacheMaxEntry {
		return nil
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if err := atomicWriteFile(c.pathFor(entry.Key), data, 0o600); err != nil {
		return err
	}
	return c.pruneDisk()
}

func (c *championDataCache) pruneDisk() error {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return err
	}
	type diskEntry struct {
		path string
		mod  time.Time
		size int64
	}
	items := make([]diskEntry, 0, len(entries))
	var total int64
	for _, item := range entries {
		info, err := item.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		items = append(items, diskEntry{path: filepath.Join(c.dir, item.Name()), mod: info.ModTime(), size: info.Size()})
		total += info.Size()
	}
	sort.Slice(items, func(i, j int) bool { return items[i].mod.Before(items[j].mod) })
	for len(items) > championCacheMaxEntries || total > championCacheMaxBytes {
		oldest := items[0]
		items = items[1:]
		if os.Remove(oldest.path) == nil {
			total -= oldest.size
		}
	}
	return nil
}

func championCachePolicy(host, requestPath, accept string) (time.Duration, time.Duration) {
	if accept != "application/json" && accept != "text/html,application/xhtml+xml" {
		return 0, 0
	}
	if host == dataDragonHost {
		return 24 * time.Hour, 7 * 24 * time.Hour
	}
	if host == opggPageHost {
		return 30 * time.Minute, 24 * time.Hour
	}
	if host != opggChampionHost {
		return 0, 0
	}
	if len(requestPath) > len("/api/KR/champions/ranked/") && (containsChampionDetailPath(requestPath)) {
		return 6 * time.Hour, 24 * time.Hour
	}
	return 20 * time.Minute, 24 * time.Hour
}

func containsChampionDetailPath(requestPath string) bool {
	parts := 0
	for _, character := range requestPath {
		if character == '/' {
			parts++
		}
	}
	return parts >= 6 || (parts >= 5 && strings.HasPrefix(requestPath, "/api/global/champions/arena/"))
}
