package main

import (
	"os"
	"strings"
	"testing"
)

func TestR316StandbyRequiresRandomTokenOwnerAndExplicitUpdate(t *testing.T) {
	token := strings.Repeat("a1", 32)
	good := []string{"--update", "--parent-first", "--parent-pid", "123", "--dest", "C:\\Deep Legends", "--standby", "--standby-token", token, "--owner-pid", "456"}
	if o := parseInstallerOptions(good); o.Error != nil || !o.Standby || o.OwnerPID != 456 {
		t.Fatal(o)
	}
	for _, bad := range []string{"", strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("z", 64), strings.Repeat("A", 64), "../" + strings.Repeat("a", 61)} {
		args := append([]string(nil), good...)
		args[8] = bad
		if parseInstallerOptions(args).Error == nil {
			t.Fatal("bad token accepted")
		}
	}
	for _, args := range [][]string{{"--standby"}, {"--standby", "--standby-token", token}, {"--update", "--standby", "--standby-token", token, "--dest", "C:\\x"}} {
		if parseInstallerOptions(args).Error == nil {
			t.Fatal("incomplete standby accepted", args)
		}
	}
}
func TestR316StandbyCannotInstallBeforeSignalOrRevealBlankWindow(t *testing.T) {
	raw, err := os.ReadFile("standby_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	show, ack, install := strings.Index(s, "a.window.surface.Show()"), strings.Index(s, "acknowledgeUpdateShell("), strings.Index(s, `a.onMessage(`)
	if show < 0 || ack < show || install < ack {
		t.Fatal("activation order must be page visible, ACK, install")
	}
	if !strings.Contains(s, "[]windows.Handle{s.parent, s.owner, s.event}") || !strings.Contains(s, "10*time.Minute-time.Since") && !strings.Contains(s, "10*time.Minute - time.Since") {
		t.Fatal("standby must expire on parent/owner exit or ten minutes")
	}
}
