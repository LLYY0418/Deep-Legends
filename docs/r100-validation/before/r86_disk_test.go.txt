package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR86EmptyProcessDiscoverySkipsDiskButErrorsRetainFallback(t *testing.T) {
	for _, tc := range []struct {
		name      string
		query     processQueryResult
		err       error
		wantCalls int
	}{
		{name: "empty"},
		{name: "failed", err: errors.New("access denied"), wantCalls: 1},
		{name: "unreadable", query: processQueryResult{ProcessCount: 1, Unreadable: 1}, wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			_, report, err := discoverLCUFromProcesses(tc.query, tc.err, func([]string) []string { calls++; return nil })
			if calls != tc.wantCalls || err == nil {
				t.Fatalf("calls=%d report=%+v err=%v", calls, report, err)
			}
			if tc.name == "empty" && (report.Result != "process-not-found" || report.LockfilesChecked != 0) {
				t.Fatal(report)
			}
		})
	}
}

func TestR86ChampionDiskMigrationPruneAndThrottle(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, championDataCacheDirectory)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for i := 0; i < 3; i++ {
		key := fmt.Sprintf("hexdata-old|hero|%d", i)
		keys = append(keys, key)
		data := []byte("fixture")
		sum := sha256.Sum256(data)
		entry := championCacheEnvelope{Schema: championCacheSchema, Key: key, Data: data, Hash: hex.EncodeToString(sum[:])}
		payload, _ := json.Marshal(entry)
		hash := sha256.Sum256([]byte(key))
		if err := os.WriteFile(filepath.Join(dir, hex.EncodeToString(hash[:])+".json"), payload, 0600); err != nil {
			t.Fatal(err)
		}
	}
	// A interrupted prior migration may leave a corrupt new-name target.
	firstHash := sha256.Sum256([]byte(keys[0]))
	if err := os.WriteFile(filepath.Join(dir, "hexdata-"+hex.EncodeToString(firstHash[:])+".json"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	c := newChampionDataCache(trackTestStore(t, &localStore{root: root}))
	if c.migrationErr != nil {
		t.Fatal(c.migrationErr)
	}
	for i, key := range keys {
		if _, err := c.readDisk(key); err != nil {
			t.Fatal(err)
		}
		// Explicit, widely separated timestamps make the victim order deterministic:
		// this assertion does not depend on filesystem mtime creation order or scheduling.
		// Without the protection every recovery file MUST precede ordinary LRU victims.
		old := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Hour)
		if err := os.Chtimes(c.pathFor(key), old, old); err != nil {
			t.Fatal(err)
		}
	}
	// 200 files, above the byte budget, including unreadable-as-JSON payloads.
	for i := 0; i < 200; i++ {
		f, err := os.Create(filepath.Join(dir, fmt.Sprintf("fixture-%03d.json", i)))
		if err != nil {
			t.Fatal(err)
		}
		if err = f.Truncate(400 << 10); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		newer := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Hour)
		if err := os.Chtimes(f.Name(), newer, newer); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.pruneDisk(); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	var total int64
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "hexdata-") {
			info, _ := entry.Info()
			total += info.Size()
		}
	}
	if total > championCacheMaxBytes {
		t.Fatal(total)
	}
	for _, key := range keys {
		if info, err := os.Lstat(c.pathFor(key)); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("prune deleted recovery data at its migrated path: %v", err)
		}
		if _, err := c.readDisk(key); err != nil {
			t.Fatal("prune deleted recovery data", err)
		}
	}
	var reads atomic.Int32
	c.readFile = func(path string) ([]byte, error) { reads.Add(1); return os.ReadFile(path) }
	c.pruneMu.Lock()
	c.lastPrune = time.Now()
	c.pruneMu.Unlock()
	if err := c.writeDisk(championCacheEnvelope{Key: "bootstrap|fixture", Data: []byte("x")}); err != nil {
		t.Fatal(err)
	}
	c.pruneMu.Lock()
	running := c.pruneDone
	written := c.writtenSincePrune
	c.pruneMu.Unlock()
	if running != nil || written == 0 {
		t.Fatalf("not throttled: running=%v written=%d", running, written)
	}
	c.scheduleDiskPrune(8 << 20)
	c.pruneMu.Lock()
	done := c.pruneDone
	c.pruneMu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("async prune stuck")
		}
	}
	if reads.Load() != 0 {
		t.Fatalf("write/prune read %d payloads", reads.Load())
	}

}
