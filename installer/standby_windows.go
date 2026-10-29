package main

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"sync"
	"time"
)

type installerStandbySignal struct {
	cancel               windows.Handle
	mu                   sync.Mutex
	closed               bool
	waiters              sync.WaitGroup
	event, parent, owner windows.Handle
	started              time.Time
}

func openInstallerStandby(options installerOptions) (*installerStandbySignal, error) {
	if !options.Standby || !validStandbyToken(options.StandbyToken) || options.OwnerPID == 0 || options.ParentPID == 0 {
		return nil, errors.New("invalid standby")
	}
	name, _ := windows.UTF16PtrFromString(`Local\DeepLegends.Update.Standby.` + options.StandbyToken)
	event, err := windows.OpenEvent(windows.SYNCHRONIZE, false, name)
	if err != nil {
		return nil, err
	}
	parent, err := windows.OpenProcess(windows.SYNCHRONIZE, false, options.ParentPID)
	if err != nil {
		windows.CloseHandle(event)
		return nil, err
	}
	owner, err := windows.OpenProcess(windows.SYNCHRONIZE, false, options.OwnerPID)
	if err != nil {
		windows.CloseHandle(event)
		windows.CloseHandle(parent)
		return nil, err
	}
	cancel, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		windows.CloseHandle(event)
		windows.CloseHandle(parent)
		windows.CloseHandle(owner)
		return nil, err
	}
	return &installerStandbySignal{event: event, parent: parent, owner: owner, cancel: cancel, started: time.Now()}, nil
}
func (s *installerStandbySignal) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	_ = windows.SetEvent(s.cancel)
	s.mu.Unlock()
	s.waiters.Wait()
	windows.CloseHandle(s.event)
	windows.CloseHandle(s.parent)
	windows.CloseHandle(s.owner)
	windows.CloseHandle(s.cancel)
}
func (s *installerStandbySignal) wait() bool {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return false
	}
	s.waiters.Add(1)
	s.mu.Unlock()
	defer s.waiters.Done()
	remain := 10*time.Minute - time.Since(s.started)
	if remain <= 0 {
		return false
	}
	result, err := windows.WaitForMultipleObjects([]windows.Handle{s.parent, s.owner, s.cancel, s.event}, false, uint32(remain/time.Millisecond))
	return err == nil && result == windows.WAIT_OBJECT_0+3
}
func (a *installerApp) awaitStandby() {
	a.timing.mark("standby_ready")
	fmt.Fprintln(os.Stdout, "DEEP_LEGENDS_UPDATE_STANDBY_READY")
	go func() {
		start := a.standbySignal.wait()
		a.window.dispatch(func() {
			if !start || a.window.closed || !a.options.Standby {
				destroyWindow.Call(a.window.hwnd)
				return
			}
			mutex, first, err := acquireSingleInstance()
			if err != nil || !first {
				if mutex != 0 {
					windows.CloseHandle(mutex)
				}
				destroyWindow.Call(a.window.hwnd)
				return
			}
			a.standbyMutex = mutex
			a.options.Standby = false
			a.timing.mark("standby_signal_received")
			a.window.surface.ShowWindow = a.window.show
			if err := a.window.surface.Show(); err != nil {
				destroyWindow.Call(a.window.hwnd)
				return
			}
			windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmFlush").Call()
			a.timing.mark("first_frame")
			a.timing.mark("webview_ready")
			a.ackSent, err = acknowledgeUpdateShell(a.options, a.window.shown && a.pageReady, func(line string) error { _, err := fmt.Fprintln(os.Stdout, line); return err }, a.timing.mark)
			if err != nil {
				a.fail(failureMessage{Message: upgradeFailureMessage})
				return
			}
			a.onMessage(`{"type":"install"}`)
		})
	}()
}
