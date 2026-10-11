package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type r274DiskGate struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	open    sync.Once
}

func r274Gate(t *testing.T, c *championDataCache, operation string) *r274DiskGate {
	t.Helper()
	g := &r274DiskGate{entered: make(chan struct{}), release: make(chan struct{})}
	c.beforeDiskIO = func(kind string) {
		if kind == operation {
			g.once.Do(func() { close(g.entered) })
			<-g.release
		}
	}
	t.Cleanup(func() {
		g.unblock()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := c.flushDisk(ctx); err != nil {
			t.Error("writer cleanup", err)
		}
	})
	return g
}

func (g *r274DiskGate) unblock() { g.open.Do(func() { close(g.release) }) }

func r274Wait[T any](t *testing.T, ch <-chan T, failure string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(2 * time.Second):
		t.Fatal(failure)
		var zero T
		return zero
	}
}

func r274Flush(t *testing.T, c *championDataCache) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.flushDisk(ctx); err != nil {
		t.Fatal("durability barrier", err)
	}
}

func r274AuxiliaryStore(t *testing.T) *localStore {
	t.Helper()
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	for _, directory := range []string{championDataCacheDirectory, prestigeArtworkCacheDirectory} {
		if err := os.MkdirAll(filepath.Join(store.root, directory), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return store
}

func TestR274HexdataStateReturnsBeforeDisk(t *testing.T) {
	store := r274AuxiliaryStore(t)
	h := newHexdataClient(newChampionProvider(), store)
	g := r274Gate(t, h.stateDisk, "hexdata-state")
	done := make(chan struct{})
	go func() { h.recordFailure("heroes", 429, true); close(done) }()
	r274Wait(t, g.entered, "Hexdata state writer not reached")
	r274Wait(t, done, "Hexdata foreground waited for state write")
	if !h.mu.TryLock() {
		t.Fatal("Hexdata global lock held during state write")
	}
	h.mu.Unlock()
	for range 4 {
		h.recordFailure("heroes", 429, true)
	}
	g.unblock()
	r274Flush(t, h.stateDisk)
	data, err := os.ReadFile(h.statePath)
	var saved hexdataState
	if err != nil || json.Unmarshal(data, &saved) != nil || saved.Circuits["heroes"].Failures != 5 {
		t.Fatal("latest coalesced Hexdata state not persisted", err, string(data))
	}
}

func TestR274PrestigeReadAndWriteDoNotHoldStoreLock(t *testing.T) {
	for _, operation := range []string{"prestige-read", "prestige-write"} {
		t.Run(operation, func(t *testing.T) {
			store := r274AuxiliaryStore(t)
			id := "11914b2b-f986-474e-b3f7-1e8cc41b72c9"
			jpeg := []byte{0xff, 0xd8, 0xff, 0xe0, 0, 0x10, 'J', 'F', 'I', 'F', 0, 1}
			if err := store.storePrestigeArtwork(id, jpeg); err != nil {
				t.Fatal(err)
			}
			g := r274Gate(t, store.auxiliaryCacheDisk(), operation)
			done := make(chan error, 1)
			go func() {
				if operation == "prestige-write" {
					done <- store.storePrestigeArtwork(id, jpeg)
					return
				}
				got, ok := store.loadPrestigeArtwork(id)
				if !ok || string(got) != string(jpeg) {
					done <- fmt.Errorf("artwork missing or altered")
					return
				}
				done <- nil
			}()
			r274Wait(t, g.entered, "prestige I/O gate not reached")
			if !store.mu.TryLock() {
				g.unblock()
				r274Wait(t, done, "blocked writer cleanup")
				t.Fatal("localStore global lock held during artwork I/O")
			}
			store.mu.Unlock()
			g.unblock()
			if err := r274Wait(t, done, "artwork I/O did not finish"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestR274PrestigeTouchDoesNotBlockForeground(t *testing.T) {
	store := r274AuxiliaryStore(t)
	id := "11914b2b-f986-474e-b3f7-1e8cc41b72c9"
	jpeg := []byte{0xff, 0xd8, 0xff, 0xe0, 0, 0x10, 'J', 'F', 'I', 'F', 0, 1}
	if err := store.storePrestigeArtwork(id, jpeg); err != nil {
		t.Fatal(err)
	}
	g := r274Gate(t, store.auxiliaryCacheDisk(), "prestige-touch")
	done := make(chan bool, 1)
	go func() { _, ok := store.loadPrestigeArtwork(id); done <- ok }()
	r274Wait(t, g.entered, "artwork touch not reached")
	if !r274Wait(t, done, "foreground waited for artwork touch") {
		t.Fatal("artwork cache miss")
	}
	if _, ok := store.loadPrestigeArtwork(id); !ok {
		t.Fatal("blocked touch prevented second cache hit")
	}
	g.unblock()
	r274Flush(t, store.auxiliaryCacheDisk())
}

func TestR274ConcurrentDetailsReturnBeforeDisk(t *testing.T) {
	var calls atomic.Int32
	p := r265CacheProvider(t, t.TempDir(), &calls)
	g := r274Gate(t, p.matchDisk, "write")
	done := make(chan error, 10)
	for i := 1; i <= 10; i++ {
		go func(id string) { _, _, err := p.matchByIDWithCache(context.Background(), id); done <- err }(fmt.Sprintf("KR_%d", i))
	}
	r274Wait(t, g.entered, "disk gate not reached")
	for i := 0; i < 10; i++ {
		if err := r274Wait(t, done, "foreground detail waited for blocked disk write"); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 10 {
		t.Fatal("details duplicated or omitted", calls.Load())
	}
	g.unblock()
	r274Flush(t, p.matchDisk)
	for i := 1; i <= 10; i++ {
		id := fmt.Sprintf("KR_%d", i)
		entry, err := p.matchDisk.readDisk("riot-match-v4|" + id)
		if err != nil || !strings.Contains(string(entry.Data), id) {
			t.Fatal("released write missing or wrong", id, err)
		}
	}
}

func TestR274MemoryHitDuringBlockedWrite(t *testing.T) {
	var calls atomic.Int32
	p := r265CacheProvider(t, t.TempDir(), &calls)
	p.cacheMu.Lock()
	p.storeMatchLocked("KR_999", r265CachedMatch(t, "KR_999"))
	p.cacheMu.Unlock()
	g := r274Gate(t, p.matchDisk, "write")
	download := make(chan error, 1)
	go func() { _, _, err := p.matchByIDWithCache(context.Background(), "KR_274"); download <- err }()
	r274Wait(t, g.entered, "disk gate not reached")
	hit := make(chan string, 1)
	go func() { _, source, _ := p.matchByIDWithCache(context.Background(), "KR_999"); hit <- source }()
	if source := r274Wait(t, hit, "memory hit blocked behind disk write/cacheMu"); source != "hit" {
		t.Fatal("memory entry lost", source)
	}
	g.unblock()
	if err := r274Wait(t, download, "download did not finish"); err != nil {
		t.Fatal(err)
	}
	r274Flush(t, p.matchDisk)
}

func TestR274ClearFencesBlockedAndQueuedWrites(t *testing.T) {
	var calls atomic.Int32
	p := r265CacheProvider(t, t.TempDir(), &calls)
	g := r274Gate(t, p.matchDisk, "write")
	done := make(chan error, 1)
	go func() { _, _, err := p.matchByIDWithCache(context.Background(), "KR_274"); done <- err }()
	r274Wait(t, g.entered, "disk gate not reached")
	queued := make(chan error, 2)
	for _, id := range []string{"KR_276", "KR_277"} {
		go func(id string) { _, _, err := p.matchByIDWithCache(context.Background(), id); queued <- err }(id)
	}
	for i := 0; i < 2; i++ {
		if err := r274Wait(t, queued, "queued detail waited for disk"); err != nil {
			t.Fatal(err)
		}
	}
	cleared := make(chan error, 1)
	go func() { cleared <- p.clearRiotMatchCaches() }()
	if err := r274Wait(t, cleared, "clear blocked on writer holding cacheMu"); err != nil {
		t.Fatal(err)
	}
	g.unblock()
	if err := r274Wait(t, done, "download did not finish"); err != nil {
		t.Fatal(err)
	}
	r274Flush(t, p.matchDisk)
	files, err := os.ReadDir(p.matchDisk.dir)
	if err != nil || len(files) != 0 {
		t.Fatal("old generation resurrected cleared disk", len(files), err)
	}
	p.cacheMu.Lock()
	defer p.cacheMu.Unlock()
	if len(p.matchCache) != 0 {
		t.Fatal("old generation resurrected memory")
	}
}

func r274Envelope(key, data string) championCacheEnvelope {
	bytes := []byte(data)
	hash := sha256.Sum256(bytes)
	return championCacheEnvelope{Schema: championCacheSchema, Key: key, FetchedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour), StaleUntil: time.Now().Add(time.Hour), Hash: hex.EncodeToString(hash[:]), Data: bytes}
}

func TestR274IdentityReturnsBeforeDiskWrite(t *testing.T) {
	var calls atomic.Int32
	p := r265CacheProvider(t, t.TempDir(), &calls)
	g := r274Gate(t, p.identityDisk, "write")
	done := make(chan error, 1)
	go func() {
		var out map[string]int
		done <- p.cachedPublicIdentity(context.Background(), "anonymous-identity", time.Hour, &out, func(context.Context) error { out = map[string]int{"value": 274}; return nil })
	}()
	r274Wait(t, g.entered, "identity writer not reached")
	if err := r274Wait(t, done, "identity foreground waited for diskMu write"); err != nil {
		t.Fatal(err)
	}
	var out map[string]int
	if err := p.cachedPublicIdentity(context.Background(), "anonymous-identity", time.Hour, &out, func(context.Context) error { t.Error("identity memory hit reloaded"); return nil }); err != nil || out["value"] != 274 {
		t.Fatal("identity memory lost", out, err)
	}
	g.unblock()
	r274Flush(t, p.identityDisk)
	if _, err := p.identityDisk.readDisk(p.identityKey("anonymous-identity")); err != nil {
		t.Fatal("identity persistence lost", err)
	}
}

func TestR274MatchTierReturnsBeforeDiskWrite(t *testing.T) {
	a := &app{storage: &localStore{root: t.TempDir()}}
	c := a.matchTierDiskCache()
	g := r274Gate(t, c, "write")
	done := make(chan struct{}, 1)
	go func() {
		a.writeMatchTierCache(274, &matchTiersResponse{Tier: "GOLD", Score: 1050, Samples: 3})
		done <- struct{}{}
	}()
	r274Wait(t, g.entered, "tier writer not reached")
	r274Wait(t, done, "tier foreground waited for diskMu write")
	value, ok := a.readMatchTierCache(274)
	if !ok || value.Tier != "GOLD" || value.Score != 1050 || value.Samples != 3 {
		t.Fatal("tier memory lost", value, ok)
	}
	g.unblock()
	r274Flush(t, c)
	restarted := &app{storage: a.storage}
	value, ok = restarted.readMatchTierCache(274)
	if !ok || value.Tier != "GOLD" || value.Score != 1050 {
		t.Fatal("tier restart lost", value, ok)
	}
}

func TestR274InvalidationHidesDiskWhileWriterBlocked(t *testing.T) {
	c := newPublicBinaryCache(&localStore{root: t.TempDir()}, "fixture", 200, 2<<20)
	key := "bootstrap|r274"
	if err := c.writeDisk(r274Envelope(key, "old")); err != nil {
		t.Fatal(err)
	}
	g := r274Gate(t, c, "write")
	c.enqueueDiskWrite(r274Envelope(key, "obsolete"))
	r274Wait(t, g.entered, "writer not reached")
	c.invalidate(key)
	if _, err := c.readDisk(key); err == nil {
		t.Fatal("invalidated identity disk still readable while delete queued")
	}
	c.enqueueDiskWrite(r274Envelope(key, "new"))
	g.unblock()
	r274Flush(t, c)
	entry, err := c.readDisk(key)
	if err != nil || string(entry.Data) != "new" {
		t.Fatal("invalidated write replaced newer data", string(entry.Data), err)
	}
}

func TestR274DiskQueueBoundedAndCoalesced(t *testing.T) {
	c := newPublicBinaryCache(&localStore{root: t.TempDir()}, "fixture", 200, 2<<20)
	g := r274Gate(t, c, "write")
	c.enqueueDiskWrite(r274Envelope("bootstrap|0", "active"))
	r274Wait(t, g.entered, "writer not reached")
	for i := 1; i <= cacheDiskQueueLimit; i++ {
		c.enqueueDiskWrite(r274Envelope(fmt.Sprintf("bootstrap|%d", i), "queued"))
	}
	c.enqueueDiskWrite(r274Envelope("bootstrap|1", "latest"))
	c.enqueueDiskWrite(r274Envelope("bootstrap|overflow", "must-drop"))
	c.diskQueue.mu.Lock()
	count := len(c.diskQueue.tasks)
	c.diskQueue.mu.Unlock()
	if count != cacheDiskQueueLimit {
		t.Fatal("queue unbounded or duplicate work not coalesced", count)
	}
	g.unblock()
	r274Flush(t, c)
	files, err := os.ReadDir(c.dir)
	if err != nil || len(files) != cacheDiskQueueLimit+1 {
		t.Fatal("bounded queue durability wrong", len(files), err)
	}
	entry, err := c.readDisk("bootstrap|1")
	if err != nil || string(entry.Data) != "latest" {
		t.Fatal("coalesced newest write lost", string(entry.Data), err)
	}
	if _, err := c.readDisk("bootstrap|overflow"); err == nil {
		t.Fatal("queue overflow unexpectedly accepted")
	}
}

func TestR274ShutdownDropsPendingKeepsExistingFiles(t *testing.T) {
	c := newPublicBinaryCache(&localStore{root: t.TempDir()}, "fixture", 200, 2<<20)
	if err := c.writeDisk(r274Envelope("bootstrap|preserved", "valid-existing")); err != nil {
		t.Fatal(err)
	}
	g := r274Gate(t, c, "write")
	c.enqueueDiskWrite(r274Envelope("bootstrap|active", "active"))
	r274Wait(t, g.entered, "writer not reached")
	c.enqueueDiskWrite(r274Envelope("bootstrap|pending", "pending"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.closeDisk(ctx); err != context.Canceled {
		t.Fatal("shutdown did not respect canceled grace", err)
	}
	c.enqueueDiskWrite(r274Envelope("bootstrap|after-close", "drop"))
	g.unblock()
	r274Flush(t, c)
	entry, err := c.readDisk("bootstrap|preserved")
	if err != nil || string(entry.Data) != "valid-existing" {
		t.Fatal("shutdown damaged existing file", err)
	}
	for _, key := range []string{"bootstrap|pending", "bootstrap|after-close"} {
		if _, err := c.readDisk(key); err == nil {
			t.Fatal("closed queue retained pending work")
		}
	}
}

func TestR274PromotedLeaderDoesNotPersist(t *testing.T) {
	var calls atomic.Int32
	p := r265CacheProvider(t, t.TempDir(), &calls)
	transport := p.champions.client.Transport
	var priority *riotFlightPriority
	p.champions.client.Transport = r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		response, err := transport.RoundTrip(r)
		p.cacheMu.Lock()
		priority = p.matchFlights["KR_274"].priority
		if !promoteRiotFlight(context.Background(), priority) {
			t.Error("promotion fixture not applied")
		}
		p.cacheMu.Unlock()
		return response, err
	})
	_, _, _ = p.loadRiotMatchFlight(withRiotBackground(context.Background()), "KR_274", false)
	if priority == nil {
		t.Fatal("background flight not created")
	}
	close(priority.done)
	r274Flush(t, p.matchDisk)
	p.cacheMu.Lock()
	count := len(p.matchCache)
	p.cacheMu.Unlock()
	if count != 0 {
		t.Fatal("promoted leader cached result")
	}
	if _, err := p.matchDisk.readDisk("riot-match-v4|KR_274"); err == nil {
		t.Fatal("promoted leader persisted result")
	}
}

func TestR274DiskWriteDiagnosticsRetained(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		t.Run(fmt.Sprint(disabled), func(t *testing.T) {
			var calls atomic.Int32
			root := t.TempDir()
			p := r265CacheProvider(t, root, &calls)
			events := make(chan map[string]any, 8)
			p.champions.diag = func(event map[string]any) {
				if event["event"] == "riot_match_disk_write" {
					events <- event
				}
			}
			if disabled {
				p.matchDisk.dir = ""
			} else {
				blocker := filepath.Join(root, "not-a-directory")
				if err := os.WriteFile(blocker, []byte("fixture"), 0600); err != nil {
					t.Fatal(err)
				}
				p.matchDisk.dir = filepath.Join(blocker, "cache")
			}
			if _, _, err := p.matchByIDWithCache(context.Background(), "KR_274"); err != nil {
				t.Fatal(err)
			}
			r274Flush(t, p.matchDisk)
			event := r274Wait(t, events, "write diagnostic missing")
			result := "failed"
			if disabled {
				result = "disabled"
			}
			raw, _ := json.Marshal(event)
			if event["result"] != result || len(event) != 3 || strings.Contains(string(raw), "KR_274") || strings.Contains(string(raw), root) {
				t.Fatal("diagnostic changed or leaked", string(raw))
			}
		})
	}
}

func TestR274SuccessfulCanceledCallerStillPersists(t *testing.T) {
	var calls atomic.Int32
	p := r265CacheProvider(t, t.TempDir(), &calls)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	transport := p.champions.client.Transport
	p.champions.client.Transport = r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		response, err := transport.RoundTrip(r)
		if err == nil {
			body, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if readErr != nil {
				return nil, readErr
			}
			response.Body = io.NopCloser(strings.NewReader(string(body)))
			cancel()
		}
		return response, err
	})
	g := r274Gate(t, p.matchDisk, "write")
	done := make(chan error, 1)
	go func() { _, _, err := p.matchByIDWithCache(ctx, "KR_274"); done <- err }()
	r274Wait(t, g.entered, "disk gate not reached")
	if err := r274Wait(t, done, "paid canceled detail waited for disk"); err != nil {
		t.Fatal("successful response discarded on cancellation", err)
	}
	p.cacheMu.Lock()
	cached := p.matchCache["KR_274"]
	p.cacheMu.Unlock()
	if cached == nil {
		t.Fatal("paid canceled detail lost memory")
	}
	g.unblock()
	r274Flush(t, p.matchDisk)
	if _, err := p.matchDisk.readDisk("riot-match-v4|KR_274"); err != nil {
		t.Fatal("paid canceled detail lost disk", err)
	}
}

func TestR274MemoryHitDoesNotWaitForTouch(t *testing.T) {
	var calls atomic.Int32
	p := r265CacheProvider(t, t.TempDir(), &calls)
	p.persistRiotMatch("riot-match-v4|KR_274", r265CachedMatch(t, "KR_274"))
	p.cacheMu.Lock()
	p.storeMatchLocked("KR_274", r265CachedMatch(t, "KR_274"))
	p.cacheMu.Unlock()
	g := r274Gate(t, p.matchDisk, "touch")
	done := make(chan string, 1)
	go func() { _, source, _ := p.matchByIDWithCache(context.Background(), "KR_274"); done <- source }()
	r274Wait(t, g.entered, "touch gate not reached")
	if source := r274Wait(t, done, "memory hit waited for blocked disk touch"); source != "hit" {
		t.Fatal(source)
	}
	g.unblock()
	r274Flush(t, p.matchDisk)
}

func TestR274InvalidationsRetainedAtQueueCapacity(t *testing.T) {
	c := newPublicBinaryCache(&localStore{root: t.TempDir()}, "fixture", 200, 2<<20)
	for _, key := range []string{"bootstrap|delete-a", "bootstrap|delete-b"} {
		if err := c.writeDisk(r274Envelope(key, "old")); err != nil {
			t.Fatal(err)
		}
	}
	g := r274Gate(t, c, "write")
	c.enqueueDiskWrite(r274Envelope("bootstrap|active", "active"))
	r274Wait(t, g.entered, "writer not reached")
	c.invalidate("bootstrap|delete-a")
	for i := 0; i < cacheDiskQueueLimit-1; i++ {
		c.enqueueDiskWrite(r274Envelope(fmt.Sprintf("bootstrap|pending-%d", i), "queued"))
	}
	c.invalidate("bootstrap|delete-b")
	c.diskQueue.mu.Lock()
	pending := len(c.diskQueue.tasks)
	c.diskQueue.mu.Unlock()
	if pending > cacheDiskQueueLimit {
		t.Fatal("invalidation exceeded bounded queue", pending)
	}
	g.unblock()
	r274Flush(t, c)
	restarted := newPublicBinaryCache(&localStore{root: filepath.Dir(c.dir)}, "fixture", 200, 2<<20)
	for _, key := range []string{"bootstrap|delete-a", "bootstrap|delete-b"} {
		if _, err := restarted.readDisk(key); err == nil {
			t.Fatal("invalidated file resurrected after restart", key)
		}
	}
}

func TestR274CorruptCleanupPreservesNewerAtomicWrite(t *testing.T) {
	c := newPublicBinaryCache(&localStore{root: t.TempDir()}, "fixture", 200, 2<<20)
	key := "bootstrap|corrupt"
	old := []byte("invalid JSON")
	if err := os.WriteFile(c.pathFor(key), old, 0600); err != nil {
		t.Fatal(err)
	}
	g := r274Gate(t, c, "write")
	c.enqueueDiskWrite(r274Envelope(key, "new-valid"))
	r274Wait(t, g.entered, "writer not reached")
	if _, err := c.readDisk(key); err == nil {
		t.Fatal("corrupt data accepted")
	}
	g.unblock()
	r274Flush(t, c)
	entry, err := c.readDisk(key)
	if err != nil || string(entry.Data) != "new-valid" {
		t.Fatal("stale cleanup removed newer atomic file", err)
	}
}

func TestR274ShutdownDrainsInitializedOwnersWithoutCreatingCaches(t *testing.T) {
	store := &localStore{root: t.TempDir()}
	c := newPublicBinaryCache(store, "fixture", 200, 2<<20)
	a := &app{storage: store, clientCatalogCache: c, proProfiles: proProfileCache{disk: c}}
	c.enqueueDiskWrite(r274Envelope("bootstrap|shutdown", "persisted"))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	a.closePublicCacheDisks(ctx)
	if _, err := c.readDisk("bootstrap|shutdown"); err != nil {
		t.Fatal("shutdown dropped completed public cache", err)
	}
	if a.matchTierCache != nil || a.perkCatalogDisk != nil || a.profileIconImages != nil || a.proPlayers.disk != nil {
		t.Fatal("shutdown created unused caches")
	}
	c.diskQueue.mu.Lock()
	closed := c.diskQueue.closed
	c.diskQueue.mu.Unlock()
	if !closed {
		t.Fatal("shutdown did not close initialized owner")
	}
}

func TestR274ObservedAugmentsReadOutsideProviderLock(t *testing.T) {
	p := newChampionProvider()
	p.cache = newPublicBinaryCache(&localStore{root: t.TempDir()}, "fixture", 200, 2<<20)
	p.observedWriteActive = true // Isolate startup read from unrelated background persistence.
	path := "/assets/augments/icons/16.19/Fixture.png"
	data, err := json.Marshal([]gameplayAugment{{ID: 1, Name: "old", IconPath: "hexdata:" + path}, {ID: 2, Name: "disk-only", IconPath: "hexdata:" + path}})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.cache.writeDisk(r274Envelope(observedHexdataAugmentsKey, string(data))); err != nil {
		t.Fatal(err)
	}
	g := r274Gate(t, p.cache, "read")
	done := make(chan []gameplayAugment, 1)
	go func() { done <- p.observedHexdataAugments() }()
	r274Wait(t, g.entered, "observed disk read not reached")
	if !p.observedAugmentMu.TryLock() {
		t.Fatal("observed disk read held provider lock")
	}
	p.observedAugmentMu.Unlock()
	observed := make(chan struct{}, 1)
	go func() {
		p.observeHexdataAugments([]hexdataAugmentRowV2{{AugmentID: 1, AugmentName: "new", AugmentIconURL: path}})
		observed <- struct{}{}
	}()
	r274Wait(t, observed, "live observation waited for startup disk read")
	g.unblock()
	items := r274Wait(t, done, "startup read did not finish")
	if len(items) != 2 || items[0].Name != "new" || items[1].Name != "disk-only" {
		t.Fatal("startup disk value overwrote live observation", items)
	}
}
