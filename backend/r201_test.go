package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestR201ReadOnlyCameraAndRelockRetry(t *testing.T) {
	for _, failures := range []int{0, 1, 2} {
		t.Run(string(rune('0'+failures)), func(t *testing.T) {
			v := newR198Fixture(t)
			if err := os.Chmod(v.f.location.file, 0444); err != nil {
				t.Fatal(err)
			}
			attempts := 0
			v.f.a.cameraFilePermissions = func(path string, info os.FileInfo) (func(bool) error, error) {
				original, err := cameraFilePermissions(path, info)
				return func(locked bool) error {
					if locked {
						attempts++
						if attempts <= failures {
							return errors.New("injected relock failure")
						}
					}
					return original(locked)
				}, err
			}
			v.apply("champselect")
			e := cameraEvent(t, v)
			expected := strings.Replace(r198JSON, `"value":"2"`, `"value":"0"`, 1)
			if r198Read(t, v.f.location.file) != expected {
				t.Fatal("CameraMode bytes not preserved", e)
			}
			info, _ := os.Stat(v.f.location.file)
			locked := gameSettingsReadOnly(v.f.location.file, info)
			if failures == 2 {
				if locked || e["file_result"] != "relock_failed" || e["relocked"] != false {
					t.Fatal(e, locked)
				}
				if status := readRigStatus(context.Background(), v.f.client); status.SettingsLocked {
					t.Fatal("maintenance lies about failed relock")
				}
			} else if !locked || e["file_result"] != "ok_relocked" || e["relocked"] != true {
				t.Fatal(e, locked)
			}
			if attempts != min(failures+1, 2) {
				t.Fatal("incorrect retry count", attempts)
			}
			// Windows cannot delete a read-only file during TempDir cleanup.
			os.Chmod(v.f.location.file, 0600)
		})
	}
}
func TestR201MissingLCUCameraIsNotPresent(t *testing.T) {
	v := newR198Fixture(t)
	delete(v.f.lcu["General"], "CameraMode")
	v.apply("champselect")
	if e := cameraEvent(t, v); e["lcu_result"] != "not_present" || len(v.f.patches) != 0 {
		t.Fatal(e, v.f.patches)
	}
}
func TestR201InGameAndNoneNeverUnlock(t *testing.T) {
	for _, kind := range []string{"InProgress", "running", "none", "settings_changed"} {
		t.Run(kind, func(t *testing.T) {
			v := newR198Fixture(t)
			os.Chmod(v.f.location.file, 0444)
			v.f.a.cameraFilePermissions = func(string, os.FileInfo) (func(bool) error, error) { t.Fatal("unsafe unlock"); return nil, nil }
			stage := "champselect"
			switch kind {
			case "InProgress":
				v.phase.Store(kind)
			case "running":
				v.phase.Store("GameStart")
				v.running.Store(true)
				stage = "game_start"
			case "none":
				v.f.a.saveGameSettingsPreference("none")
			case "settings_changed":
				v.phase.Store("Lobby")
				stage = kind
			}
			v.apply(stage)
			info, _ := os.Stat(v.f.location.file)
			if !gameSettingsReadOnly(v.f.location.file, info) || r198Read(t, v.f.location.file) != r198JSON {
				t.Fatal("file/attributes touched")
			}
			os.Chmod(v.f.location.file, 0600)
		})
	}
}
func TestR201TimingConsumedOnceAndInvalidDeleted(t *testing.T) {
	for _, raw := range []string{`{`, `{"installer_start":1}`, `{"installer_start":1000,"parent_exited":1100,"extract_start":1200,"extract_done":1400,"copy_done":1500,"relaunch":1600}`} {
		root := t.TempDir()
		path := filepath.Join(root, "update-install-timing.json")
		os.WriteFile(path, []byte(raw), 0600)
		rows := []map[string]any{}
		record := func(row map[string]any) { rows = append(rows, row) }
		consumeUpdateInstallTiming(root, record)
		consumeUpdateInstallTiming(root, record)
		if len(rows) != 1 {
			t.Fatal(rows)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("timing retained")
		}
		expected := "partial"
		if raw == `{` {
			expected = "invalid"
		}
		if strings.Contains(raw, "relaunch") {
			expected = "partial"
		}
		if rows[0]["result"] != expected {
			t.Fatal(rows)
		}
		encoded, _ := json.Marshal(rows)
		if bytes.Contains(encoded, []byte(root)) {
			t.Fatal("path exposed")
		}
		if strings.Contains(raw, "relaunch") && rows[0]["total_ms"] != int64(600) {
			t.Fatal(rows)
		}
	}
}
func TestR201ProbeWindowCancelsSlowRoutes(t *testing.T) {
	t.Parallel()
	data := bytes.Repeat([]byte("x"), int(updateProbeBytes))
	for _, second := range []bool{false, true} {
		u := updateTestManager(t, data)
		slowCanceled := make(chan error, 1)
		var wg sync.WaitGroup
		wg.Add(3)
		u.client = &http.Client{Transport: r196RoundTrip(func(req *http.Request) (*http.Response, error) {
			defer wg.Done()
			delay := 5 * time.Second
			if req.URL.Host == "first.test" {
				delay = 490 * time.Millisecond
			}
			if second && req.URL.Host == "second.test" {
				delay = 200 * time.Millisecond
			}
			select {
			case <-req.Context().Done():
				if req.URL.Host == "slow.test" {
					slowCanceled <- req.Context().Err()
				}
				return nil, req.Context().Err()
			case <-time.After(delay):
				return r196Response(data), nil
			}
		})}
		started := time.Now()
		got := u.probeUpdateSources(context.Background(), u.manifest.Asset, []string{"https://first.test/", "https://second.test/", "https://slow.test/"})
		elapsed := time.Since(started)
		// Allow scheduling margin around the 490ms first response plus the
		// production 1s comparison window, while still excluding a 5s wait.
		if elapsed >= 3*time.Second {
			t.Fatal("waited slowest probe", elapsed)
		}
		want := "https://first.test/"
		if second {
			want = "https://second.test/"
		}
		measured := 0
		for _, probe := range got {
			if probe.ok {
				measured++
			}
		}
		if second && measured != 2 {
			t.Fatal("comparison window missed completed route", got)
		}
		if len(got) == 0 || got[0].prefix != want {
			t.Fatal(got)
		}
		exited := make(chan struct{})
		go func() { wg.Wait(); close(exited) }()
		select {
		case <-exited:
		case <-time.After(time.Second):
			t.Fatal("unfinished probes not canceled")
		}
		select {
		case err := <-slowCanceled:
			if !errors.Is(err, context.Canceled) {
				t.Fatal("slow probe did not exit by cancellation", second, err)
			}
		default:
			t.Fatal("slow probe returned without context cancellation", second)
		}
		t.Logf("R260 probe round second=%v elapsed=%s measured=%d slow_canceled=true", second, elapsed, measured)
	}
}
func TestR201BothUpdatePathsPassParent(t *testing.T) {
	for _, cmd := range []string{updateCommandLine("setup.exe", "dest", 42), portableUpdateCommandLine("setup.exe", "dest", 42)} {
		if !strings.Contains(cmd, "--parent-pid 42") || !strings.Contains(cmd, "--parent-first") {
			t.Fatal(cmd)
		}
	}
}
func TestR204SubsetWithoutPoolChampionNeverSelects(t *testing.T) {
	for _, strategy := range []string{"show-only", "show-then-lock", "lock-now"} {
		v := newR200Fixture(t, `[107,141,75]`)
		settings := v.r.currentWatch()
		group := settings.ChampSelect.Groups["aram"]
		group.Pick.Strategy = strategy
		settings.ChampSelect.Groups["aram"] = group
		v.r.apply(settings)
		v.tick(t)
		v.tick(t)
		if v.count() != 0 {
			t.Fatal("picked outside configured pool", v.patches)
		}
		v.requireTrace(t, "subset", "no-pool-champion")
	}
}

func TestR201SubsetConfiguredStrategies(t *testing.T) {
	for _, strategy := range []string{"show-only", "show-then-lock", "lock-now"} {
		t.Run(strategy, func(t *testing.T) {
			v := newR200Fixture(t, `[136,141,75]`)
			v.grid = append(v.grid, champSelectGridChampion{ID: 107, Owned: true}, champSelectGridChampion{ID: 141, Owned: true})
			s := v.r.currentWatch()
			g := s.ChampSelect.Groups["aram"]
			g.Pick.Strategy = strategy
			delay := 30
			g.Pick.LockDelayMS = &delay
			s.ChampSelect.Groups["aram"] = g
			v.r.apply(s)
			v.tick(t)
			if v.count() != 1 || v.last().Body["completed"] != (strategy == "lock-now") {
				t.Fatal(v.patches)
			}
			v.tick(t)
			if strategy == "show-then-lock" && (v.count() != 2 || v.last().Body["completed"] != true) {
				t.Fatal(v.patches)
			}
			if strategy == "show-only" && v.count() != 1 {
				t.Fatal("show-only locked", v.patches)
			}
			v.tick(t)
			v.requireTrace(t, "postflight", "applied")
		})
	}
}

func TestR201CanceledProbeStillFallsBackAfterDownloadError(t *testing.T) {
	data := bytes.Repeat([]byte("x"), int(updateProbeBytes))
	u := updateTestManager(t, data)
	u.mirrors = []string{"https://first.test/", "https://late.test/"}
	events := &r196Events{}
	u.diagnostic = events.record
	u.client = &http.Client{Transport: r196RoundTrip(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Range") == "bytes=0-1048575" {
			if req.URL.Host == "first.test" {
				return r196Response(data), nil
			}
			<-req.Context().Done()
			return nil, req.Context().Err()
		}
		if req.URL.Host == "first.test" {
			return nil, errors.New("download unavailable after fast probe")
		}
		return r196Response(data), nil
	})}
	if err := u.downloadAsset(context.Background(), u.manifest.Asset, u.mirrors); err != nil {
		t.Fatal("canceled fallback route lost", err)
	}
	selected := events.named("update_download_source_selected")
	if len(selected) != 2 || selected[1]["probe_measured"] != false {
		t.Fatal(selected)
	}
}
