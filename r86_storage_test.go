package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestR86SnapshotsPruneByBytesAndCount(t *testing.T) {
	for _, size := range []int64{8 << 20, 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			store := trackTestStore(t, &localStore{root: t.TempDir()})
			directory := filepath.Join(store.root, "snapshots")
			if err := os.MkdirAll(directory, 0700); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 35; i++ {
				f, err := os.Create(filepath.Join(directory, fmt.Sprintf("%03d.json", i)))
				if err != nil {
					t.Fatal(err)
				}
				err = f.Truncate(size)
				f.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			store.pruneSnapshots()
			entries, err := os.ReadDir(directory)
			if err != nil {
				t.Fatal(err)
			}
			var total int64
			for _, entry := range entries {
				info, err := entry.Info()
				if err != nil {
					t.Fatal(err)
				}
				total += info.Size()
			}
			if len(entries) > maxSnapshots || total > maxSnapshotTotalBytes {
				t.Fatalf("count=%d bytes=%d", len(entries), total)
			}
			if _, err := os.Stat(filepath.Join(directory, "034.json")); err != nil {
				t.Fatal("newest snapshot lost", err)
			}
			if _, err := os.Stat(filepath.Join(directory, "000.json")); !os.IsNotExist(err) {
				t.Fatal("oldest snapshot survived")
			}
		})
	}
}
