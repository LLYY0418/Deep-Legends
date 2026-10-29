package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

const updateStandbyCapability = "standby-v1"

type updateStandbyProcess interface {
	Ready() bool
	Start(func(string)) error
	Close()
}
type updateStandbySlot struct {
	path, destination string
	asset             updateAsset
	process           updateStandbyProcess
	cancel            context.CancelFunc
}
type updateStandbyState struct {
	mu         sync.Mutex
	slot       *updateStandbySlot
	generation uint64
}

func (u *updateManager) stopStandby() {
	u.standby.mu.Lock()
	slot := u.standby.slot
	u.standby.generation++
	u.standby.slot = nil
	u.standby.mu.Unlock()
	if slot != nil {
		slot.cancel()
		if slot.process != nil {
			slot.process.Close()
		}
	}
}
func (u *updateManager) prepareStandby(parent context.Context, manifest updateManifest, path string, generation ...uint64) {
	if !slices.Contains(manifest.Capabilities, updateStandbyCapability) || compareVersions(manifest.Version, "0.13.5") < 0 {
		return
	}
	// The caller reached this point only after SHA verification and warmup.
	if _, err := u.verifyReadyUpdateFile(parent, path, manifest.Asset); err != nil {
		return
	}
	u.mu.Lock()
	valid := u.status.State == "ready" && u.manifest != nil && u.manifest.Version == manifest.Version && u.manifest.Asset == manifest.Asset
	dest, portable := u.installDir, u.status.Portable
	u.mu.Unlock()
	if !valid {
		return
	}
	if portable {
		if u.portableDirectory == nil {
			return
		}
		var err error
		dest, err = u.portableDirectory()
		if err != nil {
			return
		}
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Minute)
	slot := &updateStandbySlot{path: path, destination: dest, asset: manifest.Asset, cancel: cancel}
	u.standby.mu.Lock()
	if len(generation) > 0 && u.standby.generation != generation[0] {
		u.standby.mu.Unlock()
		cancel()
		return
	}
	old := u.standby.slot
	u.standby.generation++
	u.standby.slot = slot
	u.standby.mu.Unlock()
	if old != nil {
		old.cancel()
		if old.process != nil {
			old.process.Close()
		}
	}
	start := u.startStandby
	if start == nil {
		start = startUpdateStandby
	}
	process, err := start(ctx, path, dest)
	u.standby.mu.Lock()
	accepted := u.standby.slot == slot && ctx.Err() == nil
	if err == nil && accepted {
		slot.process = process
	} else if u.standby.slot == slot {
		u.standby.slot = nil
	}
	u.standby.mu.Unlock()
	if err != nil || !accepted {
		cancel()
		if process != nil {
			process.Close()
		}
	}
}
func (u *updateManager) activateStandby(path, dest string, asset updateAsset) (bool, error) {
	u.standby.mu.Lock()
	slot := u.standby.slot
	u.standby.slot = nil
	u.standby.mu.Unlock()
	if slot == nil {
		return false, nil
	}
	if slot.process == nil || !slot.process.Ready() || slot.path != path || slot.destination != dest || slot.asset != asset {
		slot.cancel()
		if slot.process != nil {
			slot.process.Close()
		}
		return false, nil
	}
	// Recheck immutable identity even though apply() has already verified it.
	info, err := os.Stat(path)
	if err != nil || info.Size() != asset.Size || filepath.Clean(path) != filepath.Clean(slot.path) {
		slot.cancel()
		slot.process.Close()
		return false, nil
	}
	err = slot.process.Start(u.markApplyTiming)
	if err != nil {
		slot.cancel()
		slot.process.Close()
		u.recordUpdateCheck(map[string]any{"event": "update_standby_fallback", "reason": "activation-failed"})
		return false, nil
	}
	// An activated installer owns its lifetime; closing the old app must not kill it.
	return true, err
}

func updateManifestIdentity(manifest updateManifest) string {
	raw, _ := json.Marshal(manifest)
	return string(raw)
}
