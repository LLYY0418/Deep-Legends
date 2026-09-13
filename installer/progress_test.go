package main

import (
	"testing"
	"time"
)

func TestSetupCommandLine(t *testing.T) {
	for _, tc := range []struct {
		dest     string
		shortcut bool
		want     string
	}{
		{`D:\Games\Deep Legends`, true, `"C:\Temp Space\Deep Legends Setup.exe" /S /D=D:\Games\Deep Legends`},
		{`C:\Program Files\Deep Legends`, false, `"C:\Temp Space\Deep Legends Setup.exe" /S --no-desktop-shortcut /D=C:\Program Files\Deep Legends`},
		{`D:\中文\Deep Legends\\`, false, `"C:\Temp Space\Deep Legends Setup.exe" /S --no-desktop-shortcut /D=D:\中文\Deep Legends`},
	} {
		if got := setupCommandLine(`C:\Temp Space\Deep Legends Setup.exe`, tc.dest, tc.shortcut); got != tc.want {
			t.Errorf("got %q; want %q", got, tc.want)
		}
	}
}

func TestProgressMeasuresBothDiskPassesAndCapsBeforeExit(t *testing.T) {
	m := progressModel{installedBytes: 1000}
	for _, tc := range []struct {
		extracted, copied int64
		elapsed           time.Duration
		want              int
	}{
		{0, 0, 0, 4}, {1000, 0, 0, 50}, {1000, 500, 0, 73},
		{1000, 1000, 0, 97}, {10000, 10000, 0, 97},
		{0, 0, 30 * time.Second, 54}, {0, 0, 45 * time.Second, 80},
		{0, 0, 10 * time.Minute, 80}, {-100, -100, -time.Second, 4},
		{1<<63 - 1, 1<<63 - 1, time.Hour, 97},
	} {
		if got := m.percent(tc.extracted, tc.copied, tc.elapsed); got != tc.want {
			t.Errorf("percent(%d,%d,%v) = %d; want %d", tc.extracted, tc.copied, tc.elapsed, got, tc.want)
		}
	}
	if got := (progressModel{}).percent(0, 0, time.Minute); got != 80 {
		t.Fatalf("missing metadata must not produce invalid progress: %d", got)
	}
}

func TestStagesAndSpace(t *testing.T) {
	if progressStage(0, 0) != "正在校验安装包…" || progressStage(1, 0) != "正在解压程序文件…" || progressStage(1, 1) != "正在写入程序文件…" {
		t.Fatal("incorrect disk stage")
	}
	if requiredSpace(1000) != 1150 || requiredSpace(1) != 2 {
		t.Fatal("space must include a rounded-up 15% reserve")
	}
}

func TestInstallStateRejectsDuplicateAttemptsAndClose(t *testing.T) {
	var s installState
	if s.busy() || !s.begin() || !s.busy() || s.begin() {
		t.Fatal("attempt must be reserved synchronously")
	}
	s.phase = phaseFinishing
	if !s.busy() || s.begin() {
		t.Fatal("launch delay must remain protected")
	}
	s.phase = phaseFailed
	if s.busy() || !s.begin() {
		t.Fatal("failed attempts must allow closing or retrying")
	}
}
