package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestR313VerifiedInstallerIdentityAndTamper(t *testing.T) {
	for _, kind := range []string{"unchanged", "size", "mtime", "identity", "digest"} {
		t.Run(kind, func(t *testing.T) {
			data := []byte("verified fixture package")
			u := updateTestManager(t, data)
			asset := u.manifest.Asset
			file := filepath.Join(u.directory, asset.Name)
			os.WriteFile(file, data, 0600)
			skipped, err := u.verifyReadyUpdateFile(context.Background(), file, asset)
			if skipped || err != nil {
				t.Fatal(skipped, err)
			}
			info, _ := os.Stat(file)
			switch kind {
			case "size":
				os.WriteFile(file, append(data, 'x'), 0600)
			case "mtime":
				os.Chtimes(file, info.ModTime().Add(time.Second), info.ModTime().Add(time.Second))
			case "identity":
				os.Rename(file, file+".old")
				os.WriteFile(file, []byte("tampered fixture package"), 0600)
				os.Chtimes(file, info.ModTime(), info.ModTime())
			case "digest":
				asset.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
			}
			skipped, err = u.verifyReadyUpdateFile(context.Background(), file, asset)
			switch kind {
			case "unchanged":
				if !skipped || err != nil {
					t.Fatal(skipped, err)
				}
			case "mtime":
				if skipped || err != nil {
					t.Fatal(skipped, err)
				}
			default:
				if skipped || err == nil {
					t.Fatal("tamper accepted", skipped, err)
				}
			}
		})
	}
}
func TestR313ApplyTimingConsumesOnlyElapsedAllowlistedFields(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "update-apply-timing.json")
	os.WriteFile(path, []byte(`{"apply_click":0,"apply_received":15,"verify_skipped":16,"ack_received":20,"windows_hidden":21,"secret":123,"spawn_begin":-1}`), 0600)
	var rows []map[string]any
	for i := 0; i < 2; i++ {
		consumeUpdateApplyTiming(root, func(row map[string]any) { rows = append(rows, row) })
	}
	if len(rows) != 1 || rows[0]["result"] != "ok" {
		t.Fatal(rows)
	}
	b, _ := json.Marshal(rows)
	if string(b) == "" {
		t.Fatal("no timing")
	}
	stages := rows[0]["stages_ms"].(map[string]int64)
	if len(stages) != 5 || stages["windows_hidden"] != 21 {
		t.Fatal(stages)
	}
}

// Invoked only by the Windows diagnostic with the actual published 112MB
// Setup, rather than a synthetic byte buffer. Ordinary unit runs use the small
// identity/tamper tests above and do not create a large throwaway executable.
func TestR313RealPackageApplyEndpointTiming(t *testing.T) {
	source := os.Getenv("R313_SETUP_FILE")
	if source == "" {
		return
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 100000000 {
		t.Fatal("real setup payload is missing")
	}
	u := updateTestManager(t, data)
	u.launch = func(string, string) error { return nil }
	asset := u.manifest.Asset
	file := filepath.Join(u.directory, asset.Name)
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	a := &app{updates: u}
	a.quitOnce.Do(func() {}) // Measure the real handler without exiting this diagnostic host.
	result := map[string]any{"setup_bytes": len(data)}
	for _, mode := range []string{"hash", "cached"} {
		var values []float64
		for i := 0; i < 15; i++ {
			u.mu.Lock()
			u.status.State = "ready"
			u.verifiedFile = nil
			u.mu.Unlock()
			if mode == "cached" {
				if _, err := u.verifyReadyUpdateFile(context.Background(), file, asset); err != nil {
					t.Fatal(err)
				}
			}
			start := time.Now()
			w := httptest.NewRecorder()
			a.handleUpdateAction(w, httptest.NewRequest("POST", "/api/update/apply", strings.NewReader(`{}`)))
			elapsed := float64(time.Since(start)) / float64(time.Millisecond)
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			values = append(values, elapsed)
		}
		sorted := append([]float64(nil), values...)
		sort.Float64s(sorted)
		result[mode] = map[string]any{"samples_ms": values, "median_ms": sorted[7], "p95_ms": sorted[14], "max_ms": sorted[14]}
		if mode == "cached" && sorted[14] > 50 {
			t.Errorf("cached apply exceeds 50ms: %.3f", sorted[14])
		}
	}
	b, _ := json.Marshal(result)
	t.Log("R313_APPLY_TIMING " + string(b))
}
