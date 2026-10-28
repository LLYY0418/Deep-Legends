package main

import (
	"os"
	"strings"
	"testing"
)

func TestR314WarmupBeforeEveryInstallerSideEffect(t *testing.T) {
	if !installerWarmup([]string{"--warmup"}) || installerWarmup([]string{"--update"}) {
		t.Fatal("warmup flag")
	}
	raw, _ := os.ReadFile("main_windows.go")
	s := string(raw)
	start := strings.Index(s, "func main() {")
	if !strings.HasPrefix(strings.TrimSpace(s[start+len("func main() {"):]), "if installerWarmup(os.Args[1:]) {\n\t\treturn\n\t}") {
		t.Fatal("warmup must exit before logging, mutex, payload, or windows")
	}
	raw, _ = os.ReadFile("window_windows.go")
	if strings.Contains(string(raw), "DrawTextW") || strings.Contains(string(raw), "正在启动升级程序") {
		t.Fatal("placeholder paint returned")
	}
	raw, _ = os.ReadFile("internal/webviewhost/surface_windows.go")
	s = string(raw)
	a := strings.Index(s, "func (s *WindowsSurface) Show()")
	b := strings.Index(s[a:], "s.ShowWindow()") + a
	if b < a || strings.Index(s[a:b], "s.Prepare()") < 0 || strings.Index(s[a:b], "controllerGetVisible") < 0 {
		t.Fatal("controller must be visible before parent reveal")
	}
}

func TestR314RelativeProcessTimingPersistsZeroAndNoOldShellStage(t *testing.T) {
	t.Setenv("LOL_LOOT_DATA_DIR", t.TempDir())
	timing := newUpdateInstallTiming()
	timing.mark("installer_start")
	timing.duration("process_created_to_main_ms", 0)
	timing.mark("page_visible")
	timing.mark("ack_sent")
	raw, err := os.ReadFile(timing.path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, `"process_created_to_main_ms":0`) || !strings.Contains(s, `"page_visible":`) || strings.Contains(s, "shell_window_shown") {
		t.Fatal(s)
	}
}
