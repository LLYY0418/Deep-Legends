package main

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"testing"
	"time"
)

func TestR316StandbyWaitTimeoutAndCancelCloseHandles(t *testing.T) {
	token := fmt.Sprintf("%064x", os.Getpid())
	name, _ := windows.UTF16PtrFromString(`Local\DeepLegends.Update.Standby.` + token)
	event, err := windows.CreateEvent(nil, 1, 0, name)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(event)
	options := installerOptions{Standby: true, StandbyToken: token, ParentPID: uint32(os.Getpid()), OwnerPID: uint32(os.Getpid())}
	s, err := openInstallerStandby(options)
	if err != nil {
		t.Fatal(err)
	}
	s.started = time.Now().Add(-10*time.Minute - time.Second)
	if s.wait() {
		t.Fatal("expired standby activated")
	}
	s.started = time.Now()
	done := make(chan bool, 1)
	go func() { done <- s.wait() }()
	s.close()
	select {
	case active := <-done:
		if active {
			t.Fatal("cancel activated standby")
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not join waiter before handle close")
	}
	if s.wait() {
		t.Fatal("closed standby accepted another wait")
	}
}
