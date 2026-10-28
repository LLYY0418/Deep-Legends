package main

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

type warmedUpdateInstaller struct {
	path string
	info os.FileInfo
}

// Called only after verification. Reserve the immutable file before dispatch so
// repeated ready notifications and an overlapping click cannot schedule twice.
func (u *updateManager) warmReadyInstaller(manifest updateManifest) {
	if _, valid := parseUpdateVersion(manifest.Version); !valid || compareVersions(manifest.Version, "0.13.3") < 0 {
		return
	}
	path := filepath.Join(u.directory, manifest.Asset.Name)
	info, err := verifiedUpdateFileInfo(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != manifest.Asset.Size {
		return
	}
	u.mu.Lock()
	if old := u.warmedInstaller; old != nil && old.path == path && old.info.Size() == info.Size() && old.info.ModTime().Equal(info.ModTime()) && os.SameFile(old.info, info) {
		u.mu.Unlock()
		return
	}
	u.warmedInstaller = &warmedUpdateInstaller{path: path, info: info}
	run, parent := u.warmInstaller, u.ctx
	u.mu.Unlock()
	if run == nil {
		run = warmUpdateInstaller
	}
	if parent == nil {
		parent = context.Background()
	}
	goSafe("update-installer-warmup", func() {
		ctx, cancel := context.WithTimeout(parent, 30*time.Second)
		defer cancel()
		started := time.Now()
		err := run(ctx, path)
		result := "ok"
		if ctx.Err() == context.DeadlineExceeded {
			result = "timeout"
		} else if err != nil || ctx.Err() != nil {
			result = "failed"
		}
		if u.diagnostic != nil {
			u.diagnostic(map[string]any{"event": "update_warmup", "result": result, "ms": time.Since(started).Milliseconds()})
		}
	})
}
