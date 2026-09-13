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
