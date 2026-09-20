package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func r103WriteByteFile(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte{'x'}, 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestR103ProseedEntriesNeverSelectedForEviction(t *testing.T) {
	dir := t.TempDir()
	c := &championDataCache{
		strictDisk:     true,
		dir:            dir,
		diskMaxEntries: 3,
		diskMaxBytes:   1 << 60,
		strictEntries:  map[string]binaryDiskEntry{},
	}

	proseedA := r103WriteByteFile(t, dir, "proseed-a.json")
	proseedB := r103WriteByteFile(t, dir, "proseed-b.json")
	ordinaryOld := r103WriteByteFile(t, dir, "ordinary-old.json")
	ordinaryMid := r103WriteByteFile(t, dir, "ordinary-mid.json")
	ordinaryKeep := r103WriteByteFile(t, dir, "ordinary-keep.json")
	c.strictEntries[proseedA] = binaryDiskEntry{size: 1, modified: time.Unix(946684800, 0)}
	c.strictEntries[proseedB] = binaryDiskEntry{size: 1, modified: time.Unix(946684801, 0)}
	c.strictEntries[ordinaryOld] = binaryDiskEntry{size: 1, modified: time.Unix(1577836800, 0)}
	c.strictEntries[ordinaryMid] = binaryDiskEntry{size: 1, modified: time.Unix(1577836801, 0)}
	c.strictEntries[ordinaryKeep] = binaryDiskEntry{size: 1, modified: time.Unix(1577836802, 0)}
	c.strictBytes = 5

	// Rewrite an existing ordinary item so the write itself does not add a
	// sixth entry; the budget loop must evict the two oldest ordinary entries.
	if err := c.accountBinaryDiskWriteLocked(ordinaryKeep, 1); err != nil {
		t.Fatalf("account existing ordinary entry: %v", err)
	}

	if len(c.strictEntries) != 3 {
		t.Fatalf("strict entry count = %d, want 3", len(c.strictEntries))
	}
	for _, path := range []string{proseedA, proseedB, ordinaryKeep} {
		if _, ok := c.strictEntries[path]; !ok {
			t.Errorf("protected/retained entry missing from index: %s", filepath.Base(path))
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("retained file %s: %v", filepath.Base(path), err)
		}
	}
	for _, path := range []string{ordinaryOld, ordinaryMid} {
		if _, ok := c.strictEntries[path]; ok {
			t.Errorf("evicted entry remains in index: %s", filepath.Base(path))
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("evicted file %s stat error = %v, want not-exist", filepath.Base(path), err)
		}
	}
	if c.strictBytes != 3 {
		t.Fatalf("strict bytes = %d, want 3", c.strictBytes)
	}
}

func TestR103AllCandidatesProtectedReturnsErrorNotEviction(t *testing.T) {
	dir := t.TempDir()
	c := &championDataCache{
		strictDisk:     true,
		dir:            dir,
		diskMaxEntries: 1,
		diskMaxBytes:   1 << 60,
		strictEntries:  map[string]binaryDiskEntry{},
	}

	proseedA := r103WriteByteFile(t, dir, "proseed-a.json")
	proseedB := r103WriteByteFile(t, dir, "proseed-b.json")
	c.strictEntries[proseedA] = binaryDiskEntry{size: 1, modified: time.Unix(946684800, 0)}
	c.strictEntries[proseedB] = binaryDiskEntry{size: 1, modified: time.Unix(946684801, 0)}
	c.strictBytes = 2

	err := c.accountBinaryDiskWriteLocked(proseedA, 1)
	if err == nil || err.Error() != "protected cache entries exceed disk budget" {
		t.Fatalf("error = %v, want protected cache budget error", err)
	}
	if len(c.strictEntries) != 2 {
		t.Fatalf("strict entry count = %d, want 2", len(c.strictEntries))
	}
	for _, path := range []string{proseedA, proseedB} {
		if _, ok := c.strictEntries[path]; !ok {
			t.Errorf("protected entry missing from index: %s", filepath.Base(path))
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("protected file %s: %v", filepath.Base(path), err)
		}
	}
	if c.strictBytes != 2 {
		t.Fatalf("strict bytes = %d, want 2", c.strictBytes)
	}
}
