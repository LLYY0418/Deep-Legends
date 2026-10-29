package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

type r316StandbyFake struct {
	fail           atomic.Bool
	ready          atomic.Bool
	starts, closes atomic.Int32
}

func (s *r316StandbyFake) Ready() bool { return s.ready.Load() }
func (s *r316StandbyFake) Start(mark func(string)) error {
	s.starts.Add(1)
	if s.fail.Load() {
		return errors.New("failed signal")
	}
	mark("standby_signal")
	return nil
}
func (s *r316StandbyFake) Close() { s.closes.Add(1) }
func TestR316StandbyOnlyForVerifiedCapableReadyInstaller(t *testing.T) {
	for _, capable := range []bool{false, true} {
		data := bytes.Repeat([]byte{1}, 4096)
		u := updateTestManager(t, data)
		u.manifest.Version = "0.13.6"
		if capable {
			u.manifest.Capabilities = []string{updateStandbyCapability}
		}
		u.status.State = "ready"
		path := filepath.Join(u.directory, u.manifest.Asset.Name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		var calls atomic.Int32
		fake := &r316StandbyFake{}
		fake.ready.Store(true)
		u.startStandby = func(ctx context.Context, p, dest string) (updateStandbyProcess, error) {
			calls.Add(1)
			if p != path || dest != u.installDir {
				t.Error("wrong standby target")
			}
			return fake, nil
		}
		u.prepareStandby(context.Background(), *u.manifest, path)
		if capable {
			if calls.Load() != 1 {
				t.Fatal("capable installer not prepared")
			}
			used, err := u.activateStandby(path, u.installDir, u.manifest.Asset)
			if !used || err != nil || fake.starts.Load() != 1 {
				t.Fatal(used, err)
			}
			u.stopStandby()
			if fake.closes.Load() != 0 {
				t.Fatal("activated installer killed on old-app close")
			}
		}
		if !capable && calls.Load() != 0 {
			t.Fatal("old installer received standby")
		}
	}
}
func TestR316StandbyPendingCancellationAndIdentityMismatch(t *testing.T) {
	for _, reason := range []string{"cancel", "manifest", "not-ready", "wrong-path"} {
		t.Run(reason, func(t *testing.T) {
			data := bytes.Repeat([]byte{2}, 4096)
			u := updateTestManager(t, data)
			u.manifest.Version = "0.13.6"
			u.manifest.Capabilities = []string{updateStandbyCapability}
			u.status.State = "ready"
			path := filepath.Join(u.directory, u.manifest.Asset.Name)
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			fake := &r316StandbyFake{}
			fake.ready.Store(reason != "not-ready")
			u.startStandby = func(context.Context, string, string) (updateStandbyProcess, error) { return fake, nil }
			u.prepareStandby(context.Background(), *u.manifest, path)
			switch reason {
			case "cancel":
				_ = u.Cancel()
			case "manifest":
				u.stopStandby()
			default:
				target := path
				if reason == "wrong-path" {
					target += ".changed"
				}
				used, err := u.activateStandby(target, u.installDir, u.manifest.Asset)
				if used || err != nil {
					t.Fatal("invalid standby activated")
				}
			}
			if fake.starts.Load() != 0 || fake.closes.Load() != 1 {
				t.Fatalf("starts=%d closes=%d", fake.starts.Load(), fake.closes.Load())
			}
		})
	}
}

func TestR316StandbyFailedActivationFallsBackAndCancelledWarmupCannotRecreate(t *testing.T) {
	data := bytes.Repeat([]byte{7}, 4096)
	u := updateTestManager(t, data)
	u.manifest.Version = "0.13.6"
	u.manifest.Capabilities = []string{updateStandbyCapability}
	u.status.State = "ready"
	path := filepath.Join(u.directory, u.manifest.Asset.Name)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	fake := &r316StandbyFake{}
	fake.ready.Store(true)
	fake.fail.Store(true)
	var calls atomic.Int32
	u.startStandby = func(context.Context, string, string) (updateStandbyProcess, error) { calls.Add(1); return fake, nil }
	u.prepareStandby(context.Background(), *u.manifest, path)
	used, err := u.activateStandby(path, u.installDir, u.manifest.Asset)
	if used || err != nil || fake.closes.Load() != 1 {
		t.Fatal("failed activation must close then permit compatibility launch", used, err)
	}
	u.standby.mu.Lock()
	generation := u.standby.generation
	u.standby.mu.Unlock()
	u.stopStandby()
	u.prepareStandby(context.Background(), *u.manifest, path, generation)
	if calls.Load() != 1 {
		t.Fatal("cancelled generation created another process")
	}
}
