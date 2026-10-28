package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestR314ReadyWarmupVersionIdentityAndNonBlocking(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Setup.exe")
	if err := os.WriteFile(path, []byte("setup"), 0600); err != nil {
		t.Fatal(err)
	}
	started, release, finished := make(chan string, 4), make(chan struct{}), make(chan map[string]any, 4)
	u := &updateManager{directory: dir, ctx: context.Background(), diagnostic: func(event map[string]any) { finished <- event }, warmInstaller: func(ctx context.Context, path string) error {
		started <- path
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
	manifest := updateManifest{Version: "0.13.2", Asset: updateAsset{Name: "Setup.exe", Size: 5}}
	u.warmReadyInstaller(manifest)
	if len(started) != 0 || u.warmedInstaller != nil {
		t.Fatal("legacy setup was warmed")
	}
	manifest.Version = "0.13.3"
	u.warmReadyInstaller(manifest)
	select {
	case got := <-started:
		if got != path {
			t.Fatal(got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("warmup not launched")
	}
	// Calls return while warmup is still blocked, and never wait behind it.
	u.warmReadyInstaller(manifest)
	u.warmReadyInstaller(manifest)
	close(release)
	event := <-finished
	if event["result"] != "ok" || event["event"] != "update_warmup" || len(event) != 3 {
		t.Fatal(event)
	}
	if len(started) != 0 {
		t.Fatal("same immutable file warmed twice")
	}
	// Same size+mtime but a replacement file is a different identity.
	info, _ := os.Stat(path)
	replacement := filepath.Join(dir, "replacement")
	os.WriteFile(replacement, []byte("other"), 0600)
	os.Chtimes(replacement, info.ModTime(), info.ModTime())
	os.Rename(replacement, path)
	u.warmReadyInstaller(manifest)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("replacement not warmed")
	}
	<-finished
}

func TestR314WarmupFailureIsPrivateAndDoesNotAffectReadyState(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Setup.exe"), []byte("setup"), 0600)
	done := make(chan map[string]any, 1)
	u := &updateManager{directory: dir, ctx: context.Background(), status: updateStatus{State: "ready"}, diagnostic: func(event map[string]any) { done <- event }, warmInstaller: func(context.Context, string) error { return os.ErrPermission }}
	u.warmReadyInstaller(updateManifest{Version: "0.13.3", Asset: updateAsset{Name: "Setup.exe", Size: 5}})
	event := <-done
	if event["result"] != "failed" || len(event) != 3 || u.status.State != "ready" {
		t.Fatal(event, u.status)
	}
}

func TestR314ApplyNeverWaitsForWarmupProcess(t *testing.T) {
	data := []byte("verified setup fixture")
	u := updateTestManager(t, data)
	file := filepath.Join(u.directory, u.manifest.Asset.Name)
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	started, release, reported := make(chan struct{}), make(chan struct{}), make(chan map[string]any, 1)
	u.warmInstaller = func(context.Context, string) error { close(started); <-release; return nil }
	u.diagnostic = func(event map[string]any) {
		if event["event"] == "update_warmup" {
			reported <- event
		}
	}
	manifest := *u.manifest
	manifest.Version = "0.13.3"
	u.warmReadyInstaller(manifest)
	<-started
	defer func() { close(release); <-reported }()
	u.status.State = "ready"
	u.launch = func(string, string) error { return nil }
	done := make(chan error, 1)
	go func() { done <- u.Apply() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("apply waited for blocked warmup")
	}
}

func TestR314WarmupDeadlineReportsTimeout(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Setup.exe"), []byte("setup"), 0600)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	done := make(chan map[string]any, 1)
	u := &updateManager{directory: dir, ctx: ctx, warmInstaller: func(ctx context.Context, _ string) error { return ctx.Err() }, diagnostic: func(event map[string]any) { done <- event }}
	u.warmReadyInstaller(updateManifest{Version: "0.13.3", Asset: updateAsset{Name: "Setup.exe", Size: 5}})
	event := <-done
	if event["result"] != "timeout" || len(event) != 3 {
		t.Fatal(event)
	}
}
