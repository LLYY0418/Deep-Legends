package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestR186PortableInstallerHandshakeAndCurrentUserCommand(t *testing.T) {
	options := parseInstallerOptions([]string{"--update", "--fresh-install", "--parent-pid", "42", "--dest", `C:\Users\test\AppData\Local\Programs\Deep Legends`})
	if options.Error != nil || !options.FreshInstall || options.ParentPID != 42 {
		t.Fatal(options)
	}
	cmd := portableSetupCommandLine(`C:\Temp\setup.exe`, options.Destination)
	if !strings.Contains(cmd, `/S /NCRC /currentuser --portable-upgrade --updated /D=C:\Users\test\AppData\Local\Programs\Deep Legends`) {
		t.Fatal(cmd)
	}
	for _, fail := range []bool{true, false} {
		events := []string{}
		started := false
		done := portableUpdateHandoff(func(value string) error { events = append(events, value); return nil }, func() bool { events = append(events, "wait-parent"); return !fail }, func() { started = true; events = append(events, "start") })
		if done == fail || started == fail {
			t.Fatal(events)
		}
		want := []string{"DEEP_LEGENDS_UPDATE_INSTALLED", "wait-parent"}
		if !fail {
			want = append(want, "start")
		}
		if !reflect.DeepEqual(events, want) {
			t.Fatal(events)
		}
	}
	called := false
	if portableUpdateHandoff(func(string) error { return errors.New("pipe closed") }, func() bool { called = true; return true }, func() { called = true }) || called {
		t.Fatal("failed report still relaunched")
	}
	for _, exit := range []int{1, 2, 1223} {
		reported := false
		completeInstallation(options, installationResult{ExitCode: exit}, installationCompletionHooks{Failed: func(f failureMessage) { reported = true }, Cleanup: func() { t.Fatal("failed cleanup") }, Handoff: func() { t.Fatal("failed handoff") }})
		if !reported {
			t.Fatal(exit)
		}
	}
}
