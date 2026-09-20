package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Older LCU item sets also left files in the game's recommendation namespaces.
// Cleaning the account document alone does not remove those files. Archive only
// this champion's exact generated v1 UIDs, after the new recommendation has passed readback.
// Never use a display title/startedFrom marker as proof of ownership.
func archiveLegacyRecommendedFiles(ctx context.Context, location settingsLocation, championID int64, guard func() error) (int, error) {
	return archiveRecommendedFilesMatching(ctx, location, func(uid string) bool { return managedItemSetForChampion(uid, championID) }, guard)
}

func archiveRecommendedFilesMatching(ctx context.Context, location settingsLocation, matches func(string) bool, guard func() error) (int, error) {
	directory, err := recommendedDirectory(location)
	if err != nil {
		return 0, err
	}
	// recommendedDirectory returns canonical paths (e.g. /private/var on macOS).
	location.allowedRoot, err = filepath.EvalSymlinks(location.allowedRoot)
	if err != nil {
		return 0, err
	}
	config := filepath.Dir(filepath.Dir(directory))
	location.configRoot = config
	dirs := []string{directory}
	champions := filepath.Join(config, "Champions")
	if stat, err := os.Lstat(champions); err == nil && stat.IsDir() && stat.Mode()&os.ModeSymlink == 0 {
		entries, err := boundedRecommendationEntries(champions, 256)
		if err != nil {
			return 0, err
		}
		for _, entry := range entries {
			if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			dir := filepath.Join(champions, entry.Name(), "Recommended")
			if stat, err := os.Lstat(dir); err == nil && stat.IsDir() && stat.Mode()&os.ModeSymlink == 0 {
				dirs = append(dirs, dir)
			}
		}
	}
	archived, scanned := 0, 0
	for _, dir := range dirs {
		entries, err := boundedRecommendationEntries(dir, 256)
		if err != nil {
			return archived, err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return archived, err
			}
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
				continue
			}
			scanned++
			if scanned > 1024 {
				return archived, errors.New("旧推荐文件过多，已停止迁移")
			}
			file := filepath.Join(dir, entry.Name())
			original, statErr := os.Lstat(file)
			if statErr != nil || !original.Mode().IsRegular() {
				continue
			}
			data, status := readBoundedItemSetDiagnosticFile(location, file, 64<<10)
			if status != "ok" {
				continue
			}
			var meta struct {
				UID string `json:"uid"`
			}
			if json.Unmarshal(data, &meta) != nil || !matches(meta.UID) {
				continue
			}
			if guard != nil {
				if err := guard(); err != nil {
					return archived, err
				}
			}
			// This sibling is outside both namespaces scanned by the game. Reject an
			// existing symlink and use a unique directory so no previous backup is lost.
			backupRoot := filepath.Join(config, "DeepLegendsItemSetBackups")
			if err := os.Mkdir(backupRoot, 0755); err != nil && !os.IsExist(err) {
				return archived, err
			}
			stat, err := os.Lstat(backupRoot)
			if err != nil || !stat.IsDir() || stat.Mode()&os.ModeSymlink != 0 {
				return archived, errors.New("旧推荐备份目录不安全")
			}
			backup, err := os.MkdirTemp(backupRoot, "v1-")
			if err != nil {
				return archived, err
			}
			// Preserve concurrent edits instead of moving an unrelated replacement.
			latest, status := readBoundedItemSetDiagnosticFile(location, file, 64<<10)
			if status != "ok" || !bytes.Equal(data, latest) {
				return archived, errors.New("旧推荐文件已变化，已停止迁移")
			}
			if guard != nil {
				if err := guard(); err != nil {
					return archived, err
				}
			}
			finalInfo, finalErr := os.Lstat(file)
			resolvedDir, dirErr := filepath.EvalSymlinks(dir)
			resolvedBackup, backupErr := filepath.EvalSymlinks(backup)
			if finalErr != nil || !os.SameFile(original, finalInfo) || dirErr != nil || resolvedDir != dir || backupErr != nil || resolvedBackup != backup {
				return archived, errors.New("旧推荐目录或文件已变化，已停止迁移")
			}
			if err := os.Rename(file, filepath.Join(backup, entry.Name())); err != nil {
				return archived, err
			}
			archived++
		}
	}
	return archived, nil
}

func boundedRecommendationEntries(dir string, limit int) ([]os.DirEntry, error) {
	file, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	entries, err := file.ReadDir(limit + 1)
	if len(entries) > limit {
		return nil, errors.New("推荐目录文件过多，已停止迁移")
	}
	// ReadDir(n) returns io.EOF for an empty directory.
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return entries, nil
}
