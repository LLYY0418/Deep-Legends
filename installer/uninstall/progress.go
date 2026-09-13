package main

import (
	"io/fs"
	"os"
	"time"
)

const (
	stageStopping     = "正在关闭正在运行的客户端…"
	stageDeleting     = "正在删除程序文件…"
	stageCleaning     = "正在清理快捷方式与注册表…"
	stageCacheWarning = "本机缓存未能完全清理，可以稍后手动删除"
)

type directorySnapshot struct {
	Bytes, Files int64
	Missing      bool
}

func snapshotDirectory(directory string) directorySnapshot {
	var result directorySnapshot
	_, err := os.Lstat(directory)
	if os.IsNotExist(err) {
		result.Missing = true
		return result
	}
	// WalkDir does not follow junctions/symlinks into unrelated directories.
	_ = fs.WalkDir(os.DirFS(directory), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if !entry.IsDir() {
			if info, err := entry.Info(); err == nil && info.Mode().IsRegular() {
				result.Bytes += info.Size()
				result.Files++
			}
		}
		return nil
	})
	return result
}

type uninstallProgressModel struct {
	baselineBytes, baselineFiles int64
	shown                        int
	deleting                     bool
}

func (m *uninstallProgressModel) update(remaining directorySnapshot, elapsed time.Duration) progressMessage {
	stage := stageStopping
	percent := int(2 + 8*min(1, max(0, float64(elapsed)/float64(3*time.Second))))
	if remaining.Bytes < m.baselineBytes || remaining.Files < m.baselineFiles {
		m.deleting = true
	}
	if m.deleting {
		stage = stageDeleting
		ratios := []float64{}
		if m.baselineBytes > 0 {
			ratios = append(ratios, 1-float64(remaining.Bytes)/float64(m.baselineBytes))
		}
		if m.baselineFiles > 0 {
			ratios = append(ratios, 1-float64(remaining.Files)/float64(m.baselineFiles))
		}
		var total float64
		for _, ratio := range ratios {
			total += min(1, max(0, ratio))
		}
		if len(ratios) > 0 {
			percent = 10 + int(84*total/float64(len(ratios)))
		}
	}
	if remaining.Missing {
		percent, stage = 97, stageCleaning
	}
	m.shown = max(m.shown, min(percent, 97))
	return progressMessage{Percent: m.shown, Stage: stage}
}
