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
	"strconv"
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

	championCacheStateMemory = "memory"
	championCacheStateDisk   = "disk"
	championCacheStateMiss   = "miss"
	championCacheStateStale  = "stale"
	championCacheStateError  = "error"
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
	done        chan struct{}
	data        []byte
	fetchedAt   time.Time
	state       string
	upstreamErr error
	err         error
}

type championCacheLoadResult struct {
	data        []byte
	fetchedAt   time.Time
	state       string
	upstreamErr error
}

type championDataCache struct {
	now               func() time.Time
	strictDisk        bool
	diskMaxEntries    int
	diskMaxBytes      int64
	strictEntries     map[string]binaryDiskEntry
	strictBytes       int64
	readFile          func(string) ([]byte, error)
	dir               string
	mu                sync.Mutex
	entries           map[string]championCacheEnvelope
	order             []string
	bytes             int
	flights           map[string]*championCacheFlight
	diskMu            sync.Mutex
	pruneMu           sync.Mutex
	pruneDone         chan struct{}
	lastPrune         time.Time
	writtenSincePrune int64
	migrationErr      error
}

func newChampionDataCache(store *localStore) *championDataCache {
	cache := &championDataCache{entries: make(map[string]championCacheEnvelope), flights: make(map[string]*championCacheFlight)}
	if store != nil {
		cache.dir = filepath.Join(store.root, championDataCacheDirectory)
		cache.migrationErr = cache.purgeDiskHost(yourGGArenaHost)
	}
	return cache
}

func championCacheKey(host, requestPath, query, accept string) string {
	return "v1|" + host + "|" + requestPath + "|" + query + "|" + accept
}

func opggDetailCacheKey(mode, region string, championID int, position, tier, dataVersion string) string {
	return strings.Join([]string{"v3", "opgg-detail", mode, region, strconv.Itoa(championID), position, tier, dataVersion}, "|")
}

func (c *championDataCache) load(ctx context.Context, key string, ttl, staleFor time.Duration, persistDisk bool, loader func(context.Context) ([]byte, error)) ([]byte, error) {
	result, err := c.loadWithStatus(ctx, key, ttl, staleFor, persistDisk, loader)
	return result.data, err
}

func (c *championDataCache) loadWithStatus(ctx context.Context, key string, ttl, staleFor time.Duration, persistDisk bool, loader func(context.Context) ([]byte, error)) (championCacheLoadResult, error) {
	return c.loadWithResultTTL(ctx, key, ttl, staleFor, persistDisk, nil, loader)
}

func (c *championDataCache) loadWithResultTTL(ctx context.Context, key string, ttl, staleFor time.Duration, persistDisk bool, resultTTL func([]byte) time.Duration, loader func(context.Context) ([]byte, error)) (championCacheLoadResult, error) {
	now := c.cacheNow()
	c.mu.Lock()
	if entry, ok := c.entries[key]; ok {
		if now.Before(entry.ExpiresAt) {
			c.touchLocked(key)
			data := append([]byte(nil), entry.Data...)
			c.mu.Unlock()
			return championCacheLoadResult{data: data, fetchedAt: entry.FetchedAt, state: championCacheStateMemory}, nil
		}
	}
	if flight := c.flights[key]; flight != nil {
		done := flight.done
		c.mu.Unlock()
		select {
		case <-done:
			if ctx.Err() == nil && (errors.Is(flight.err, context.Canceled) || errors.Is(flight.err, context.DeadlineExceeded)) {
				return c.loadWithResultTTL(ctx, key, ttl, staleFor, persistDisk, resultTTL, loader)
			}
			return championCacheLoadResult{data: append([]byte(nil), flight.data...), fetchedAt: flight.fetchedAt, state: flight.state, upstreamErr: flight.upstreamErr}, flight.err
		case <-ctx.Done():
			return championCacheLoadResult{state: championCacheStateError}, ctx.Err()
		}
	}
	c.mu.Unlock()

	var diskEntry championCacheEnvelope
	if persistDisk && championCacheDiskAllowed(key) {
		diskEntry, _ = c.readDisk(key)
	}
	c.mu.Lock()
	if entry, ok := c.entries[key]; ok && c.cacheNow().Before(entry.ExpiresAt) {
		c.touchLocked(key)
		data := append([]byte(nil), entry.Data...)
		c.mu.Unlock()
		return championCacheLoadResult{data: data, fetchedAt: entry.FetchedAt, state: championCacheStateMemory}, nil
	}
	if existing := c.flights[key]; existing != nil {
		done := existing.done
		c.mu.Unlock()
		select {
		case <-done:
			if ctx.Err() == nil && (errors.Is(existing.err, context.Canceled) || errors.Is(existing.err, context.DeadlineExceeded)) {
				return c.loadWithResultTTL(ctx, key, ttl, staleFor, persistDisk, resultTTL, loader)
			}
			return championCacheLoadResult{data: append([]byte(nil), existing.data...), fetchedAt: existing.fetchedAt, state: existing.state, upstreamErr: existing.upstreamErr}, existing.err
		case <-ctx.Done():
			return championCacheLoadResult{state: championCacheStateError}, ctx.Err()
		}
	}
	if len(diskEntry.Data) > 0 {
		c.storeMemoryLocked(key, diskEntry)
		if now.Before(diskEntry.ExpiresAt) {
			data := append([]byte(nil), diskEntry.Data...)
			c.mu.Unlock()
			return championCacheLoadResult{data: data, fetchedAt: diskEntry.FetchedAt, state: championCacheStateDisk}, nil
		}
	}
	stale := c.entries[key]
	flight := &championCacheFlight{done: make(chan struct{})}
	c.flights[key] = flight
	c.mu.Unlock()

	data, err := loader(ctx)
	if err == nil && len(data) > 0 {
		if resultTTL != nil {
			ttl = resultTTL(data)
		}
		hash := sha256.Sum256(data)
		entry := championCacheEnvelope{
			Schema: championCacheSchema, Key: key, FetchedAt: now, ExpiresAt: now.Add(ttl),
			StaleUntil: now.Add(ttl + staleFor), Hash: hex.EncodeToString(hash[:]), Data: append([]byte(nil), data...),
		}
		c.mu.Lock()
		c.storeMemoryLocked(key, entry)
		flight.data = append([]byte(nil), data...)
		flight.fetchedAt = entry.FetchedAt
		flight.state = championCacheStateMiss
		delete(c.flights, key)
		close(flight.done)
		c.mu.Unlock()
		if persistDisk {
			_ = c.writeDisk(entry)
		}
		return championCacheLoadResult{data: data, fetchedAt: entry.FetchedAt, state: championCacheStateMiss}, nil
	}
	upstreamErr := err
	state := championCacheStateMiss
	fetchedAt := time.Time{}
	if err != nil {
		state = championCacheStateError
	}
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && len(stale.Data) > 0 && now.Before(stale.StaleUntil) {
		data = append([]byte(nil), stale.Data...)
		fetchedAt = stale.FetchedAt
		err = nil
		state = championCacheStateStale
	}
	c.mu.Lock()
	flight.data, flight.fetchedAt, flight.state, flight.upstreamErr, flight.err = append([]byte(nil), data...), fetchedAt, state, upstreamErr, err
	delete(c.flights, key)
	close(flight.done)
	c.mu.Unlock()
	return championCacheLoadResult{data: data, fetchedAt: fetchedAt, state: state, upstreamErr: upstreamErr}, err
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
	name := hex.EncodeToString(hash[:]) + ".json"
	if strings.HasPrefix(key, "hexdata-") {
		name = "hexdata-" + name
	}
	if strings.HasPrefix(key, "riot-identity-v1|proseed:") {
		name = "proseed-" + name
	}
	return filepath.Join(c.dir, name)
}

func (c *championDataCache) readDisk(key string) (championCacheEnvelope, error) {
	if c.dir == "" {
		return championCacheEnvelope{}, os.ErrNotExist
	}
	path := c.pathFor(key)
	info, err := os.Lstat(path)
	// Failed startup migration must not make existing recovery data invisible.
	if errors.Is(err, os.ErrNotExist) && strings.HasPrefix(key, "hexdata-") {
		path = filepath.Join(c.dir, strings.TrimPrefix(filepath.Base(path), "hexdata-"))
		info, err = os.Lstat(path)
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > championCacheMaxEntry*2 {
		return championCacheEnvelope{}, errors.New("cache miss")
	}
	data, err := c.readCacheFile(path)
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
	// Do not grow a cache whose recovery data could not be classified safely.
	if c.migrationErr != nil {
		return c.migrationErr
	}
	if c.dir == "" || len(entry.Data) > championCacheMaxEntry || !championCacheDiskAllowed(entry.Key) {
		return nil
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	c.diskMu.Lock()
	if c.strictDisk {
		c.initBinaryDiskIndexLocked()
	}
	err = atomicWriteFile(c.pathFor(entry.Key), data, 0o600)
	if err == nil && c.strictDisk {
		err = c.accountBinaryDiskWriteLocked(c.pathFor(entry.Key), int64(len(data)))
	}
	c.diskMu.Unlock()
	if err != nil {
		return err
	}
	if !c.strictDisk {
		c.scheduleDiskPrune(int64(len(data)))
	}
	return nil
}

// Keep directory maintenance off request latency and coalesce concurrent writers.
func (c *championDataCache) scheduleDiskPrune(written int64) {
	c.pruneMu.Lock()
	defer c.pruneMu.Unlock()
	c.writtenSincePrune += written
	if c.migrationErr != nil || c.pruneDone != nil || (c.writtenSincePrune < 8<<20 && time.Since(c.lastPrune) < 5*time.Minute) {
		return
	}
	c.lastPrune, c.writtenSincePrune = time.Now(), 0
	done := make(chan struct{})
	c.pruneDone = done
	go func() {
		_ = c.pruneDisk()
		c.pruneMu.Lock()
		c.pruneDone = nil
		close(done)
		c.pruneMu.Unlock()
	}()
}

func championCacheDiskAllowed(key string) bool {
	if strings.HasPrefix(key, "public-profile-icon|") || strings.HasPrefix(key, "pro-profile-v1|") || strings.HasPrefix(key, "pro-profile-v2|") {
		return true
	}
	if strings.HasPrefix(key, "public-pro-snapshot-v1|") || strings.HasPrefix(key, "normalized-perks-v1|") || strings.HasPrefix(key, "normalized-augments-v1|") || strings.HasPrefix(key, "riot-identity-v1|") || strings.HasPrefix(key, "riot-match-v1|KR_") || strings.HasPrefix(key, "kr-match-tier-v1|KR_") || strings.HasPrefix(key, "hexdata-") || strings.HasPrefix(key, "bootstrap|") || strings.HasPrefix(key, "v2|opgg-rsc|") || strings.HasPrefix(key, "v3|opgg-detail|") {
		return true
	}
	for _, host := range []string{dataDragonHost, communityDragonHost, opggChampionHost, opggPageHost, qq101Host} {
		if strings.HasPrefix(key, "v1|"+host+"|") {
			return true
		}
	}
	return false
}

func (c *championDataCache) purgeDiskHost(host string) error {
	if c.dir == "" || strings.TrimSpace(host) == "" {
		return nil
	}
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	prefix := "v1|" + host + "|"
	for _, item := range entries {
		info, infoErr := item.Info()
		if infoErr != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > championCacheMaxEntry*2 {
			continue
		}
		path := filepath.Join(c.dir, item.Name())
		data, readErr := c.readCacheFile(path)
		if readErr != nil {
			return readErr
		}
		var header struct {
			Key string `json:"key"`
		}
		if json.Unmarshal(data, &header) != nil {
			continue
		}
		if strings.HasPrefix(header.Key, prefix) {
			_ = os.Remove(path)
			continue
		}
		// This existing one-time startup scan also migrates old hashed Hexdata
		// names before any generic LRU pruning is allowed to run.
		if strings.HasPrefix(header.Key, "hexdata-") && !strings.HasPrefix(item.Name(), "hexdata-") {
			hash := sha256.Sum256([]byte(header.Key))
			if item.Name() != hex.EncodeToString(hash[:])+".json" {
				continue
			}
			target := c.pathFor(header.Key)
			if _, err := os.Lstat(target); err == nil {
				if _, validErr := c.readDisk(header.Key); validErr == nil {
					// A validated current-format value supersedes its duplicate.
					if err := os.Remove(path); err != nil {
						return err
					}
					continue
				}
				// readDisk removes malformed regular files; never replace an
				// untrusted target (e.g. a symlink) or discard the valid legacy copy.
				if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
					return errors.New("hexdata migration target is not replaceable")
				}
			}
			if _, err := os.Lstat(target); errors.Is(err, os.ErrNotExist) {
				if err := os.Rename(path, target); err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *championDataCache) pruneDisk() error {
	c.diskMu.Lock()
	defer c.diskMu.Unlock()
	return c.pruneDiskLocked()
}

func (c *championDataCache) pruneDiskLocked() error {
	if c.migrationErr != nil {
		return c.migrationErr
	}
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
		path := filepath.Join(c.dir, item.Name())
		// Hexdata正文 and its validator state are user-facing recovery data.
		// They must not be selected as generic LRU victims based on mtime.
		protected := strings.HasPrefix(item.Name(), "hexdata-") || strings.HasPrefix(item.Name(), "proseed-")
		if protected {
			continue
		}
		items = append(items, diskEntry{path: path, mod: info.ModTime(), size: info.Size()})
		total += info.Size()
	}
	sort.Slice(items, func(i, j int) bool { return items[i].mod.Before(items[j].mod) })
	maxEntries, maxBytes := c.diskMaxEntries, c.diskMaxBytes
	if maxEntries <= 0 {
		maxEntries = championCacheMaxEntries
	}
	if maxBytes <= 0 {
		maxBytes = championCacheMaxBytes
	}
	for len(items) > 0 && (len(items) > maxEntries || total > maxBytes) {
		oldest := items[0]
		items = items[1:]
		if os.Remove(oldest.path) == nil {
			total -= oldest.size
		}
	}
	return nil
}

// hexdataStaleBuildGrace 是旧 buildID 落盘文件的宽限期：2 个 buildID 周期
// （一个 patch 约 4 天）≈ 8 天。buildID 刚轮换时不能立刻删掉上一版数据，否则
// checkedPage 的 PreviousBuildID 回退路径会被抽掉，造成回源请求风暴。
const hexdataStaleBuildGrace = 8 * 24 * time.Hour

// pruneStaleHexdataBuilds 回收已经过期的 buildID 对应的 hexdata- 落盘文件，
// 补上「hexdata- 前缀永久豁免磁盘预算」留下的静默增长缺口。这是与
// pruneDiskLocked 里 protected 判断并行的第二条清理路径：protected 继续防止
// 「访问不频繁被普通 LRU 按 mtime 误杀」，这里只处理「buildID 已经过期」。
// 文件名是 key 的 sha256（不含 buildID），所以 buildID 只能从信封的 Key 字段
// （{buildID}|{kind}|{id}）里解出来。宽限期按 c.cacheNow() 比较，时间可注入。
func (c *championDataCache) pruneStaleHexdataBuilds(currentBuildID string) error {
	_, err := c.pruneStaleHexdataBuildsCount(currentBuildID)
	return err
}

// pruneStaleHexdataBuildsCount 与上同，另外返回删除的文件数，供诊断事件使用。
func (c *championDataCache) pruneStaleHexdataBuildsCount(currentBuildID string) (int, error) {
	currentBuildID = strings.TrimSpace(currentBuildID)
	if c.dir == "" || !strings.HasPrefix(currentBuildID, "hexdata-") {
		return 0, nil
	}
	if c.migrationErr != nil {
		return 0, c.migrationErr
	}
	c.diskMu.Lock()
	defer c.diskMu.Unlock()
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	now := c.cacheNow()
	removed := 0
	for _, item := range entries {
		name := item.Name()
		// hexdata-state.json 是熔断/validator 状态，不是缓存信封，绝不能碰。
		if !strings.HasPrefix(name, "hexdata-") || !strings.HasSuffix(name, ".json") || name == "hexdata-state.json" {
			continue
		}
		info, infoErr := item.Info()
		if infoErr != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > championCacheMaxEntry*2 {
			continue
		}
		path := filepath.Join(c.dir, name)
		data, readErr := c.readCacheFile(path)
		if readErr != nil {
			// 读不出来的文件一律不动：无法确认它属于哪个 buildID，删了就是猜。
			continue
		}
		var entry championCacheEnvelope
		if json.Unmarshal(data, &entry) != nil || strings.TrimSpace(entry.Key) == "" {
			continue
		}
		buildID := strings.SplitN(entry.Key, "|", 2)[0]
		if buildID == currentBuildID || !strings.HasPrefix(buildID, "hexdata-") {
			continue
		}
		fetchedAt := entry.FetchedAt
		if fetchedAt.IsZero() {
			// 信封缺时间戳时退回文件 mtime，宽限期语义不变。
			fetchedAt = info.ModTime()
		}
		if now.Sub(fetchedAt) < hexdataStaleBuildGrace {
			continue
		}
		if os.Remove(path) == nil {
			removed++
		}
	}
	return removed, nil
}

func championCachePolicy(host, requestPath, accept string) (time.Duration, time.Duration, bool) {
	if strings.HasPrefix(accept, "image/") && allowedChampionHost(host) {
		return 7 * 24 * time.Hour, 30 * 24 * time.Hour, true
	}
	if accept != "application/json" && accept != "text/html,application/xhtml+xml" {
		return 0, 0, false
	}
	if host == dataDragonHost {
		return 24 * time.Hour, 7 * 24 * time.Hour, true
	}
	if host == communityDragonHost {
		return 24 * time.Hour, 7 * 24 * time.Hour, true
	}
	if host == yourGGArenaHost {
		return 20 * time.Minute, 0, false
	}
	if host == qq101Host {
		return 30 * time.Minute, 24 * time.Hour, true
	}
	if host == opggPageHost && requestPath == proLadderPath {
		return 24 * time.Hour, 0, true
	}
	if host == opggPageHost {
		return 30 * time.Minute, 24 * time.Hour, true
	}
	if host != opggChampionHost {
		return 0, 0, false
	}
	if len(requestPath) > len("/api/KR/champions/ranked/") && (containsChampionDetailPath(requestPath)) {
		return 6 * time.Hour, 24 * time.Hour, true
	}
	return 20 * time.Minute, 24 * time.Hour, true
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

func (c *championDataCache) readCacheFile(path string) ([]byte, error) {
	if c.readFile != nil {
		return c.readFile(path)
	}
	return os.ReadFile(path)
}

func (c *championDataCache) cacheNow() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}
