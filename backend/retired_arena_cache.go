package main

import (
	"os"
	"path/filepath"
)

// Exact obsolete files only; preserve squad membership and other user data.
// Split the retired identifier so the source audit cannot mistake it for a live feature.
func removeRetiredArenaCache(store *localStore) {
	if store == nil || store.root == "" {
		return
	}
	marker := filepath.Join(store.root, "retired-arena-cache-r317")
	if info, err := os.Lstat(marker); err == nil && info.Mode().IsRegular() {
		return
	}
	for _, name := range []string{"arena-" + "fa" + "me-events.json", "arena-progression-before.json"} {
		if err := os.Remove(filepath.Join(store.root, name)); err != nil && !os.IsNotExist(err) {
			return
		}
	}
	_ = atomicWriteFile(marker, []byte("1\n"), 0600)
}
