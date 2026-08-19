package main

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoadAssetDeduplicatesConcurrentFetches(t *testing.T) {
	a := &app{assetCache: make(map[string][]byte)}
	var calls atomic.Int32
	start := make(chan struct{})
	loaderStarted := make(chan struct{}, 1)
	release := make(chan struct{})
	loader := func(context.Context) ([]byte, error) {
		calls.Add(1)
		select {
		case loaderStarted <- struct{}{}:
		default:
		}
		<-release
		return []byte("shared-image"), nil
	}

	const workers = 24
	var wait sync.WaitGroup
	wait.Add(workers)
	errorsSeen := make(chan error, workers)
	for range workers {
		go func() {
			defer wait.Done()
			<-start
			data, err := a.loadAsset(context.Background(), "same", 1024, 0, loader)
			if err != nil {
				errorsSeen <- err
				return
			}
			if string(data) != "shared-image" {
				errorsSeen <- errors.New("unexpected asset data")
			}
		}()
	}
	close(start)
	<-loaderStarted
	time.Sleep(10 * time.Millisecond)
	close(release)
	wait.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("loader calls = %d, want 1", got)
	}
}

func TestLoadAssetNegativeCacheAvoidsImmediateRetry(t *testing.T) {
	a := &app{assetCache: make(map[string][]byte)}
	var calls atomic.Int32
	loader := func(context.Context) ([]byte, error) {
		calls.Add(1)
		return nil, errors.New("upstream unavailable")
	}
	if _, err := a.loadAsset(context.Background(), "missing", 1024, time.Minute, loader); err == nil {
		t.Fatal("first load unexpectedly succeeded")
	}
	if _, err := a.loadAsset(context.Background(), "missing", 1024, time.Minute, loader); !errors.Is(err, errCachedAssetFailure) {
		t.Fatalf("second load error = %v, want cached failure", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("loader calls = %d, want 1", got)
	}
}

func TestAssetCacheEvictsIncrementally(t *testing.T) {
	a := &app{assetCache: make(map[string][]byte)}
	a.assetCacheMu.Lock()
	for index := 0; index < assetCacheMaxEntries+20; index++ {
		a.storeAssetLocked(string(rune(index+1)), []byte{byte(index)})
	}
	a.assetCacheMu.Unlock()
	if got := len(a.assetCache); got > assetCacheMaxEntries {
		t.Fatalf("cache entries = %d, want at most %d", got, assetCacheMaxEntries)
	}
	if _, ok := a.assetCache[string(rune(assetCacheMaxEntries+20))]; !ok {
		t.Fatal("newest cache entry was discarded")
	}
}
