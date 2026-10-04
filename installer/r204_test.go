package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestR204UpgradeProgressStages(t *testing.T) {
	base := time.Unix(100, 0)
	m := upgradeProgressModel{installedBytes: 1000}
	s := map[string]int64{"installer_start": base.UnixMilli()}
	cases := []struct {
		stage, text string
		low, high   int
		bytes       int64
	}{{"installer_start", "正在等待旧程序退出…", 0, 5, 0}, {"parent_exited", "正在移除旧版本…", 5, 25, 0}, {"uninstall_old_start", "正在移除旧版本…", 5, 25, 0}, {"uninstall_old_done", "正在解压程序文件…", 25, 60, 0}, {"extract_start", "正在解压程序文件…", 25, 60, 500}, {"extract_done", "正在写入程序文件…", 60, 95, 500}, {"copy_done", "正在完成升级…", 95, 100, 0}}
	for i, c := range cases {
		s[c.stage] = base.Add(time.Duration(i) * time.Minute).UnixMilli()
		last := m.last
		for second := 0; second <= 20; second++ {
			v := m.update(s, c.bytes, c.bytes, base.Add(time.Duration(i)*time.Minute+time.Duration(second)*time.Second))
			if v.Percent < c.low || v.Percent >= c.high || v.Percent < last || v.Stage != c.text {
				t.Fatal(c, v, last)
			}
			last = v.Percent
		}
	}
}
func TestR204UninstallTwentySecondsSmooth(t *testing.T) {
	now := time.Unix(100, 0)
	m := upgradeProgressModel{}
	s := map[string]int64{"uninstall_old_start": now.UnixMilli()}
	seen := map[int]bool{}
	for i := 0; i <= 20; i++ {
		v := m.update(s, 9999999, 9999999, now.Add(time.Duration(i)*time.Second))
		if v.Percent < 5 || v.Percent >= 25 {
			t.Fatal(v)
		}
		seen[v.Percent] = true
	}
	if len(seen) < 10 {
		t.Fatal("uninstall progress frozen", seen)
	}
}
func TestR204TimingImportAndNull(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOL_LOOT_DATA_DIR", root)
	timing := newUpdateInstallTiming()
	timing.mark("installer_start")
	now := time.Now().UnixMilli()
	os.WriteFile(filepath.Join(root, "update-install-stages.txt"), []byte("uninstall_old_start="+formatTimingMS(now)+"\nuninstall_old_done="+formatTimingMS(now)+"\nextract_start="+formatTimingMS(now)), 0600)
	timing.importNSIS(root)
	raw, err := os.ReadFile(timing.path)
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]any
	json.Unmarshal(raw, &values)
	if values["parent_exited"] != nil || values["uninstall_old_start"] == nil || values["extract_start"] == nil || len(values) != 8 {
		t.Fatal(values)
	}
	info, _ := os.Stat(timing.path)
	timing.importNSIS(root)
	after, _ := os.Stat(timing.path)
	if !after.ModTime().Equal(info.ModTime()) {
		t.Fatal("unchanged polling rewrote timing file")
	}
}
