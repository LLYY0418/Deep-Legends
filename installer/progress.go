package main

import (
	"strings"
	"time"
)

const (
	progressPrepare      = 4
	progressBeforeExit   = 97
	progressTimeFloorCap = 80
)

type progressModel struct{ installedBytes int64 }

func (m progressModel) percent(extracted, copied int64, elapsed time.Duration) int {
	// Convert before adding/multiplying so malformed counters cannot overflow.
	total := 2 * float64(m.installedBytes)
	done := max(0, float64(extracted)) + max(0, float64(copied))
	measured := float64(progressPrepare)
	if total > 0 {
		measured += float64(progressBeforeExit-progressPrepare) * done / total
	}
	floor := progressPrepare + float64(progressTimeFloorCap-progressPrepare)*min(1, max(0, float64(elapsed)/float64(45*time.Second)))
	// Applying the ceiling here also covers pre-existing files during upgrades.
	return int(min(max(measured, floor), progressBeforeExit))
}

func progressStage(extracted, copied int64) string {
	if copied > 0 {
		return "正在写入程序文件…"
	}
	if extracted > 0 {
		return "正在解压程序文件…"
	}
	return "正在校验安装包…"
}

// Upgrade progress follows real NSIS milestones. Pre-existing target bytes
// are ignored until removal/extraction finish. Stages without byte counters
// advance asymptotically inside their interval, never pretending to complete.
type upgradeProgressModel struct {
	installedBytes int64
	last           int
}

func (m *upgradeProgressModel) update(stages map[string]int64, extracted, copied int64, now time.Time) progressMessage {
	low, high, stage, at, bytes := 0, 5, "正在等待旧程序退出…", stages["installer_start"], int64(0)
	switch {
	case stages["copy_done"] > 0:
		low, high, stage, at = 95, 100, "正在完成升级…", stages["copy_done"]
	case stages["extract_done"] > 0:
		low, high, stage, at, bytes = 60, 95, "正在写入程序文件…", stages["extract_done"], copied
	case stages["uninstall_old_done"] > 0 && stages["extract_start"] == 0:
		low, high, stage, at = 25, 60, "正在解压程序文件…", stages["uninstall_old_done"]
	case stages["extract_start"] > 0:
		low, high, stage, at, bytes = 25, 60, "正在解压程序文件…", stages["extract_start"], extracted
	case stages["parent_exited"] > 0 || stages["uninstall_old_start"] > 0:
		low, high, stage, at = 5, 25, "正在移除旧版本…", stages["uninstall_old_start"]
		if at == 0 {
			at = stages["parent_exited"]
		}
	}
	elapsed := max(0, float64(now.UnixMilli()-at)/1000)
	if at == 0 {
		elapsed = 0
	}
	fraction := elapsed / (elapsed + 12)
	if bytes > 0 && m.installedBytes > 0 {
		fraction = float64(bytes) / float64(m.installedBytes)
	}
	percent := low + int(float64(high-low)*min(.99, max(0, fraction)))
	m.last = min(99, max(m.last, percent))
	return progressMessage{Percent: m.last, Stage: stage}
}

func setupCommandLine(setupPath, destDir string, desktopShortcut bool) string {
	var b strings.Builder
	b.WriteString(`"`)
	b.WriteString(setupPath)
	b.WriteString(`" /S`)
	if !desktopShortcut {
		b.WriteString(" --no-desktop-shortcut")
	}
	b.WriteString(" /D=")
	b.WriteString(strings.TrimRight(destDir, `\`))
	return b.String()
}

func requiredSpace(installedBytes int64) int64 {
	return installedBytes + (installedBytes*15+99)/100
}

// The existing parent wait is bounded by the handoff protocol. Report its
// otherwise byte-less interval while it runs; manual installs do not use this.
func waitUpgradeParent(wait func() bool, emit func(progressMessage)) bool {
	if wait == nil {
		return false
	}
	done := make(chan bool, 1)
	go func() { done <- wait() }()
	started := time.Now()
	model := upgradeProgressModel{}
	stages := map[string]int64{"installer_start": started.UnixMilli()}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case ok := <-done:
			return ok
		case now := <-ticker.C:
			emit(model.update(stages, 0, 0, now))
		}
	}
}
