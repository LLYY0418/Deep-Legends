package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func r265CacheProvider(t *testing.T, root string, calls *atomic.Int32) *riotProvider {
	t.Helper()
	t.Setenv("RIOT_API_KEY", "synthetic-r265-key")
	return newRiotProvider(&championProvider{
		cache: newChampionDataCache(&localStore{root: root}),
		client: &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
			calls.Add(1)
			id := filepath.Base(r.URL.Path)
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(fmt.Sprintf("{\"metadata\":{\"matchId\":%q},\"info\":{\"participants\":[{\"participantId\":1,\"puuid\":\"fixture\"}]}}", id)))}, nil
		})},
	})
}

func r265CachedMatch(t *testing.T, id string) *riotMatch {
	t.Helper()
	var match riotMatch
	if err := json.Unmarshal([]byte(fmt.Sprintf("{\"metadata\":{\"matchId\":%q},\"info\":{\"participants\":[{\"participantId\":1,\"puuid\":\"fixture\"}]}}", id)), &match); err != nil {
		t.Fatal(err)
	}
	return &match
}

func TestR265RiotDiskLimitsAndNoExpiryRestart(t *testing.T) {
	root := t.TempDir()
	var calls atomic.Int32
	p := r265CacheProvider(t, root, &calls)
	t.Run("limits", func(t *testing.T) {
		if p.matchDisk.diskMaxEntries != 2000 || p.matchDisk.diskMaxBytes != 50<<20 {
			t.Fatal("wrong permanent detail budget", p.matchDisk.diskMaxEntries, p.matchDisk.diskMaxBytes)
		}
	})
	key := "riot-match-v4|KR_265"
	p.persistRiotMatch(key, r265CachedMatch(t, "KR_265"))
	entry, err := p.matchDisk.readDisk(key)
	if err != nil {
		t.Fatal(err)
	}
	// Existing caches must also remain usable after their old TTL passes.
	entry.ExpiresAt, entry.StaleUntil = time.Unix(1, 0), time.Unix(1, 0)
	if err = p.matchDisk.writeDisk(entry); err != nil {
		t.Fatal(err)
	}
	restarted := r265CacheProvider(t, root, &calls)
	got, source, err := restarted.matchByIDWithCacheMode(context.Background(), "KR_265", false)
	if err != nil || got == nil || source != "disk" || calls.Load() != 0 {
		t.Fatal("restart lost immutable detail", source, err, calls.Load())
	}
}

func TestR265RiotDiskLRUAndCorruption(t *testing.T) {
	var calls atomic.Int32
	p := r265CacheProvider(t, t.TempDir(), &calls)
	p.matchDisk.diskMaxEntries = 3
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("KR_%d", i)
		p.persistRiotMatch("riot-match-v4|"+id, r265CachedMatch(t, id))
	}
	if _, _, err := p.matchByIDWithCache(context.Background(), "KR_1"); err != nil {
		t.Fatal(err)
	}
	p.persistRiotMatch("riot-match-v4|KR_4", r265CachedMatch(t, "KR_4"))
	if _, err := os.Stat(p.matchDisk.pathFor("riot-match-v4|KR_1")); err != nil {
		t.Fatal("recently used match was evicted", err)
	}
	if _, err := os.Stat(p.matchDisk.pathFor("riot-match-v4|KR_2")); !os.IsNotExist(err) {
		t.Fatal("least recently used match survived", err)
	}
	restarted := r265CacheProvider(t, filepath.Dir(p.matchDisk.dir), &calls)
	key := "riot-match-v4|KR_4"
	if err := os.WriteFile(restarted.matchDisk.pathFor(key), []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	got, source, err := restarted.matchByIDWithCache(context.Background(), "KR_4")
	if err != nil || got == nil || source != "miss" || calls.Load() != 1 {
		t.Fatal("corrupt match did not silently reload", source, err, calls.Load())
	}
	restarted.matchDisk.diskMaxEntries, restarted.matchDisk.diskMaxBytes = 100, 1200
	for i := 10; i < 20; i++ {
		id := fmt.Sprintf("KR_%d", i)
		restarted.persistRiotMatch("riot-match-v4|"+id, r265CachedMatch(t, id))
	}
	files, err := os.ReadDir(restarted.matchDisk.dir)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, file := range files {
		info, err := file.Info()
		if err != nil {
			t.Fatal(err)
		}
		total += info.Size()
	}
	if total > 1200 {
		t.Fatal("byte budget exceeded", total)
	}
}

func TestR265RiotClearAcrossRegionsAndExportIsolation(t *testing.T) {
	var calls atomic.Int32
	root := t.TempDir()
	p := r265CacheProvider(t, root, &calls)
	jp := p.forPlatform("jp1")
	for _, entry := range []struct {
		provider *riotProvider
		id       string
	}{{p, "KR_265"}, {jp, "JP1_265"}} {
		entry.provider.persistRiotMatch("riot-match-v4|"+entry.id, r265CachedMatch(t, entry.id))
		if _, _, err := entry.provider.matchByIDWithCache(context.Background(), entry.id); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 0 || len(p.matchCache) != 1 || len(jp.matchCache) != 1 {
		t.Fatal("regional disk keys collided", calls.Load())
	}
	a := &app{riot: p}
	result := httptest.NewRecorder()
	a.handleClearRiotMatchCache(result, httptest.NewRequest("POST", "/api/gameplay/cache/clear", nil))
	if result.Code != 200 || len(p.matchCache) != 0 || len(jp.matchCache) != 0 {
		t.Fatal("clear did not cover all providers", result.Code)
	}
	files, err := os.ReadDir(p.matchDisk.dir)
	if err != nil || len(files) != 0 {
		t.Fatal("details remain on disk", len(files), err)
	}
	// Cache payloads are not diagnostic log generations and cannot enter export.
	store := &localStore{root: root}
	p.persistRiotMatch("riot-match-v4|KR_265", r265CachedMatch(t, "KR_265"))
	data, err := store.readDiagnosticLogForExport()
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "fixture") || strings.Contains(string(data), "KR_265") {
		t.Fatal("match cache leaked into diagnostic export")
	}
}

func TestR265ClearFencesInflightCacheWrite(t *testing.T) {
	var calls atomic.Int32
	p := r265CacheProvider(t, t.TempDir(), &calls)
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	p.champions.client.Transport = r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		once.Do(func() { close(started) })
		<-release
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("{\"metadata\":{\"matchId\":\"KR_265\"},\"info\":{\"participants\":[{\"participantId\":1}]}}"))}, nil
	})
	done := make(chan error, 1)
	go func() { _, _, err := p.matchByIDWithCache(t.Context(), "KR_265"); done <- err }()
	<-started
	if err := p.clearRiotMatchCaches(); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	files, _ := os.ReadDir(p.matchDisk.dir)
	if len(files) != 0 || len(p.matchCache) != 0 {
		t.Fatal("old response resurrected cleared details", len(files), len(p.matchCache))
	}
}
