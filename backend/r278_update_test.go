package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

type r278Transport func(*http.Request) (*http.Response, error)

func (f r278Transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func r278Asset(data []byte) updateAsset {
	sum := sha256.Sum256(data)
	return updateAsset{Name: "fixture.exe", Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), URL: "https://github.com/fixture/setup"}
}
func r278RangeResponse(r *http.Request, data []byte) (*http.Response, error) {
	start, end := int64(0), int64(len(data)-1)
	status := 200
	header := http.Header{}
	if value := r.Header.Get("Range"); value != "" {
		if _, err := fmt.Sscanf(value, "bytes=%d-%d", &start, &end); err != nil {
			return nil, err
		}
		status = 206
		header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
	}
	return &http.Response{StatusCode: status, Header: header, ContentLength: end - start + 1, Body: io.NopCloser(bytes.NewReader(data[start : end+1]))}, nil
}
func TestR278P13FirstByteDoesNotWaitForBlockedMirror(t *testing.T) {
	data := bytes.Repeat([]byte{7}, 2<<20)
	u := &updateManager{}
	started := make(chan struct{}, 1)
	u.client = &http.Client{Transport: r278Transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "blocked.test" {
			started <- struct{}{}
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return r278RangeResponse(r, data)
	})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	race := u.startUpdateSourceRace(ctx, r278Asset(data), []string{"https://blocked.test/", "https://fast.test/"})
	defer race.cancel()
	if race.first != "https://fast.test/" {
		t.Fatal("first byte did not start the ready route")
	}
	<-started
	cancel()
}

type r278BrokenBody struct {
	data []byte
	sent bool
}

func (b *r278BrokenBody) Read(p []byte) (int, error) {
	if b.sent {
		return 0, io.ErrUnexpectedEOF
	}
	b.sent = true
	n := copy(p, b.data)
	return n, io.ErrUnexpectedEOF
}
func (b *r278BrokenBody) Close() error { return nil }
func TestR278P13FailedSegmentResumesVerifiedBytes(t *testing.T) {
	data := bytes.Repeat([]byte{5}, 5<<20)
	asset := r278Asset(data)
	var broke atomic.Bool
	var resumed atomic.Bool
	u := &updateManager{}
	u.client = &http.Client{Transport: r278Transport(func(r *http.Request) (*http.Response, error) {
		resp, err := r278RangeResponse(r, data)
		if err != nil {
			return nil, err
		}
		var start, end int64
		fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end)
		if start == 0 && broke.CompareAndSwap(false, true) {
			resp.Body = &r278BrokenBody{data: data[:32<<10]}
		}
		if start == 32<<10 {
			resumed.Store(true)
		}
		return resp, nil
	})}
	race := &updateSourceRace{first: "https://one.test/", sources: []string{"https://one.test/", "https://two.test/"}, results: map[string]updateSourceProbe{}}
	part := filepath.Join(t.TempDir(), "asset.part")
	if err := u.downloadSegments(context.Background(), asset, part, race); err != nil {
		t.Fatal(err)
	}
	if !resumed.Load() {
		t.Fatal("failed segment restarted instead of resuming")
	}
	if err := verifyUpdateFile(context.Background(), part, asset); err != nil {
		t.Fatal(err)
	}
}
func TestR278P13ChecksumFailureFallsBackToOneSource(t *testing.T) {
	data := bytes.Repeat([]byte{9}, 5<<20)
	bad := bytes.Repeat([]byte{8}, len(data))
	asset := r278Asset(data)
	var serial atomic.Int64
	u := &updateManager{directory: t.TempDir()}
	u.client = &http.Client{Transport: r278Transport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Range") == "" {
			serial.Add(1)
			return r278RangeResponse(r, data)
		}
		return r278RangeResponse(r, bad)
	})}
	if err := u.downloadAssetQuick(context.Background(), asset, []string{"https://one.test/"}); err != nil {
		t.Fatal(err)
	}
	if serial.Load() == 0 {
		t.Fatal("checksum failure did not use a single source")
	}
	if err := verifyUpdateFile(context.Background(), filepath.Join(u.directory, asset.Name), asset); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(u.directory, asset.Name+".part")); !os.IsNotExist(err) {
		t.Fatal("partial file retained")
	}
}
func TestR278P13ReadyPredownloadDoesNotStartAnotherTransfer(t *testing.T) {
	u := &updateManager{status: updateStatus{Supported: true, State: "ready", Current: "0.12.84"}}
	if err := u.Download(); err != nil {
		t.Fatal(err)
	}
	if u.status.State != "ready" {
		t.Fatal("ready installer lost")
	}
}
func TestR278P12CachedAndInvalidCacheFallback(t *testing.T) {
	for _, valid := range []bool{true, false} {
		t.Run(fmt.Sprint(valid), func(t *testing.T) {
			store := trackTestStore(t, &localStore{root: t.TempDir()})
			raw := bundledAnnouncements
			if valid {
				list, err := decodeAnnouncements(raw)
				if err != nil {
					t.Fatal(err)
				}
				list[0].Revision = "cached"
				raw, _ = json.Marshal(list)
			} else {
				raw = []byte(`[{"popup":null}]`)
			}
			if err := writeLocalStoreFile(store, "announcements.json", raw); err != nil {
				t.Fatal(err)
			}
			a := &app{storage: store}
			cache := a.announcementState()
			if len(cache.notices) == 0 {
				t.Fatal("offline notice lost")
			}
			if valid && (cache.source != "cache" || cache.notices[0].Revision != "cached") {
				t.Fatal("valid cache ignored")
			}
			if !valid && cache.source != "bundled" {
				t.Fatal("invalid cache replaced bundled notice")
			}
		})
	}
}
func TestR278P13AutomaticPredownloadAndForegroundTakeover(t *testing.T) {
	for _, mode := range []string{"enabled", "disabled", "manual-only"} {
		t.Run(mode, func(t *testing.T) {
			data := []byte("verified installer fixture")
			u := updateTestManager(t, data)
			u.autoPredownload = mode != "disabled"
			manifest := *u.manifest
			if mode == "manual-only" {
				manifest.MinSupported = "9.0.0"
				manifest = signUpdateTestManifest(manifest)
			}
			u.cache = updateCache{CheckedAt: u.now(), Current: u.status.Current, Manifest: manifest}
			u.mirrors = []string{""}
			started, release := make(chan struct{}), make(chan struct{})
			var transfers atomic.Int32
			u.client = &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
				transfers.Add(1)
				close(started)
				select {
				case <-release:
				case <-r.Context().Done():
					return nil, r.Context().Err()
				}
				return updateResponse(200, data), nil
			})}
			if !u.Check(false) {
				t.Fatal("check did not start")
			}
			<-u.checkDone
			if mode != "enabled" {
				if transfers.Load() != 0 {
					t.Fatal("unexpected automatic download")
				}
				return
			}
			u.mu.Lock()
			pre := u.predownloading
			done := u.downloadDone
			u.mu.Unlock()
			if !pre {
				t.Fatal("automatic transfer is not background")
			}
			<-started
			if err := u.Download(); err != nil {
				t.Fatal(err)
			}
			close(release)
			<-done
			if transfers.Load() != 1 || u.Status().State != "ready" {
				t.Fatal("foreground takeover duplicated or lost verified download")
			}
		})
	}
}
