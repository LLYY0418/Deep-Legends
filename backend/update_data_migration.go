package main

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func defaultUpdateDataDirectory() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "LOLLootAssistant"), nil
}

// Publish a complete copy only after every file has been read. Existing data is
// never replaced; a failed copy removes only the private staging directory.
func migrateUpdateData(source, destination string) (string, error) {
	return copyUpdateData(source, destination, true)
}
func copyUpdateData(source, destination string, publish bool) (string, error) {
	if strings.EqualFold(filepath.Clean(source), filepath.Clean(destination)) {
		return "shared", nil
	}
	if info, err := os.Lstat(destination); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "failed", errors.New("unsafe migration destination")
		}
		entries, err := os.ReadDir(destination)
		if err != nil {
			return "failed", err
		}
		if len(entries) > 0 {
			return "skipped_existing", nil
		}
	} else if !os.IsNotExist(err) {
		return "failed", err
	}
	if rel, err := filepath.Rel(source, destination); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "failed", errors.New("migration destination overlaps source")
	}
	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return "failed", err
	}
	staging, err := os.MkdirTemp(parent, ".deeplegends-data-migration-")
	if err != nil {
		return "failed", err
	}
	defer os.RemoveAll(staging)
	err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("unsafe migration source")
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(staging, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("unsafe migration file")
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, err = io.Copy(output, input)
		closeErr := output.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
	if err != nil {
		return "failed", err
	}
	if !publish {
		return "checked", nil
	}
	// Remove only an empty destination. A concurrent writer makes this fail,
	// preserving its new data rather than overwriting it.
	if err = os.Remove(destination); err != nil && !os.IsNotExist(err) {
		return "failed", err
	}
	if err = os.Rename(staging, destination); err != nil {
		return "failed", err
	}
	return "ok", nil
}
func (u *updateManager) preparePortableData() error { return u.copyPortableData(true) }
func (u *updateManager) checkPortableData() error   { return u.copyPortableData(false) }
func (u *updateManager) copyPortableData(publish bool) error {
	if u.store == nil {
		return nil
	}
	destination := u.migrationDirectory
	if destination == nil {
		destination = defaultUpdateDataDirectory
	}
	dest, err := destination()
	result := "failed"
	if err == nil {
		u.store.mu.Lock()
		u.store.diagnosticMu.Lock()
		err = u.store.flushDiagnosticLocked()
		if err == nil {
			result, err = copyUpdateData(u.store.root, dest, publish)
		}
		u.store.diagnosticMu.Unlock()
		u.store.mu.Unlock()
	}
	if u.diagnostic != nil {
		u.diagnostic(map[string]any{"event": "update_data_migration", "result": result, "stage": map[bool]string{false: "pre_install", true: "before_restart"}[publish]})
	}
	if err != nil {
		return errors.New("升级数据迁移失败，原数据已保留，请从发布页重试")
	}
	return nil
}
