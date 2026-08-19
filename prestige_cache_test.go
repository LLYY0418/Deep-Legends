package main

import (
	"bytes"
	"os"
	"testing"
	"time"
)

func TestPrestigeArtworkDiskCacheSurvivesStoreRestart(t *testing.T) {
	t.Setenv("LOL_LOOT_DATA_DIR", t.TempDir())
	store, err := openLocalStore()
	if err != nil {
		t.Fatal(err)
	}
	instanceID := "11914b2b-f986-474e-b3f7-1e8cc41b72c9"
	jpeg := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01}
	if err := store.storePrestigeArtwork(instanceID, jpeg); err != nil {
		t.Fatal(err)
	}
	reopened, err := openLocalStore()
	if err != nil {
		t.Fatal(err)
	}
	got, ok := reopened.loadPrestigeArtwork(instanceID)
	if !ok || !bytes.Equal(got, jpeg) {
		t.Fatalf("disk cache hit = %v, data = %x", ok, got)
	}
}

func TestPrestigeArtworkDiskCacheRejectsExpiredAndCorruptFiles(t *testing.T) {
	t.Setenv("LOL_LOOT_DATA_DIR", t.TempDir())
	store, err := openLocalStore()
	if err != nil {
		t.Fatal(err)
	}
	instanceID := "11914b2b-f986-474e-b3f7-1e8cc41b72c9"
	path := store.prestigeArtworkPath(instanceID)
	if err := atomicWriteFile(path, []byte("not-an-image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.loadPrestigeArtwork(instanceID); ok {
		t.Fatal("corrupt prestige artwork was accepted")
	}
	jpeg := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01}
	if err := store.storePrestigeArtwork(instanceID, jpeg); err != nil {
		t.Fatal(err)
	}
	expired := time.Now().Add(-prestigeArtworkCacheMaxAge - time.Hour)
	if err := os.Chtimes(path, expired, expired); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.loadPrestigeArtwork(instanceID); ok {
		t.Fatal("expired prestige artwork was accepted")
	}
}
