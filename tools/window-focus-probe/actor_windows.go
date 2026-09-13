//go:build windows

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"syscall"
	"time"
)

type actorState struct {
	hwnd    uintptr
	owner   uint32
	anchor  uintptr
	inbox   chan command
	encoder *json.Encoder
}

func runActor() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &actorState{inbox: make(chan command, 8), encoder: json.NewEncoder(os.Stdout)}
	cb := syscall.NewCallback(func(h, msg, wp, lp uintptr) uintptr {
		switch msg {
		case wmApp:
			select {
			case c := <-a.inbox:
				a.handle(c)
			default:
			}
			return 0
		case wmClose:
			// Only our own window. Cleanup remains available even if the controller died.
			showWindow.Call(h, 0) // Hide before removing a cloak, including watchdog/EOF cleanup.
			cloak(h, uint32(os.Getpid()), false)
			destroyWindow.Call(h)
			return 0
		case wmDestroy:
			postQuit.Call(0)
			return 0
		}
		r, _, _ := defaultProc.Call(h, msg, wp, lp)
		return r
	})
	h, err := createTopWindow(fmt.Sprintf("DLProbeActor-%d", os.Getpid()), "模拟客户端 · 这是测试窗口，不是 LOL", cb, 520, 260, 35)
	if err != nil {
		return err
	}
	a.hwnd = h
	defer func() {
		if targetOK(h, uint32(os.Getpid())) {
			destroyWindow.Call(h)
		}
	}()
	control(h, "STATIC", "这是独立测试进程。\r\n\r\n基线和恢复阶段会故意弹出，用来验证测试有效。\r\n它不会连接游戏，也不会读取任何客户端文件。\r\n\r\n按 Esc 或关闭此窗口可中止测试。", 0, 24, 24, 465, 180, 0)
	if err := a.encoder.Encode(response{OK: true, PID: uint32(os.Getpid()), HWND: uint64(h)}); err != nil {
		return err
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		scan := bufio.NewScanner(os.Stdin)
		scan.Buffer(make([]byte, 1024), 8192)
		for scan.Scan() {
			var c command
			if json.Unmarshal(scan.Bytes(), &c) != nil {
				break
			}
			select {
			case a.inbox <- c:
			case <-done:
				return
			}
			postMessage.Call(h, wmApp, 0, 0)
		}
		// EOF also happens if the controller is killed; no permanent hidden window.
		postMessage.Call(h, wmClose, 0, 0)
	}()
	go func() {
		select {
		case <-done:
		case <-time.After(60 * time.Second):
			postMessage.Call(h, wmClose, 0, 0)
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				os.Exit(3)
			}
		}
	}()
	return messageLoop()
}

func (a *actorState) event(api, result, meaning string) apiEvent {
	return apiEvent{At: nowUTC(), API: api, Result: result, Meaning: meaning,
		Snapshot: sample(a.hwnd, uint32(os.Getpid()), a.owner, a.anchor)}
}

func (a *actorState) handle(c command) {
	r := response{ID: c.ID, OK: true}
	switch c.Op {
	case "init":
		if c.OwnerPID == uint32(os.Getpid()) || !targetOK(uintptr(c.Anchor), c.OwnerPID) {
			r.OK, r.Error = false, "invalid controller anchor"
		} else {
			a.owner, a.anchor = c.OwnerPID, uintptr(c.Anchor)
		}
	case "reset":
		// The synthetic window starts every trial hidden and below the controller.
		// v1.0.0 uncloaked first, briefly exposing the previous self-cloak trial.
		showWindow.Call(a.hwnd, 0)
		ok, hr := cloak(a.hwnd, uint32(os.Getpid()), false)
		r.HR = hr
		r.Events = append(r.Events, a.event("DwmSetWindowAttribute(CLOAK=false)", hr, "cleanup, HRESULT"))
		setWindowPos.Call(a.hwnd, 1, 0, 0, 0, 0, 0x13) // HWND_BOTTOM, no activate/resize/move.
		allowForeground.Call(uintptr(a.owner))
		r.Events = append(r.Events, a.event("reset_hidden_bottom", fmt.Sprint(ok), "reset only our test window"))
	case "self_cloak", "release":
		r.OK, r.HR = cloak(a.hwnd, uint32(os.Getpid()), c.Op == "self_cloak")
		r.Events = append(r.Events, a.event("DwmSetWindowAttribute("+c.Op+")", r.HR, "HRESULT; own-process control, not an external solution"))
	case "burst":
		if a.owner == 0 {
			r.OK, r.Error = false, "not initialized"
			break
		}
		// This is the verified LoL native API sequence, but ONLY on this synthetic HWND.
		for _, mode := range []uintptr{4, 5} {
			v, _, _ := showWindow.Call(a.hwnd, mode)
			r.Events = append(r.Events, a.event(fmt.Sprintf("ShowWindow(%d)", mode), fmt.Sprint(v != 0), "previously visible; NOT a success boolean"))
		}
		v, _, _ := setActive.Call(a.hwnd)
		r.Events = append(r.Events, a.event("SetActiveWindow", fmt.Sprint(v != 0), "previous active handle was non-null; handle omitted"))
		v, _, _ = setForeground.Call(a.hwnd)
		r.Events = append(r.Events, a.event("SetForegroundWindow", fmt.Sprint(v != 0), "nonzero means foreground operation succeeded"))
		for _, position := range []int{-1, -2} {
			v, _, err := setWindowPos.Call(a.hwnd, uintptr(position), 0, 0, 0, 0, 3)
			result := fmt.Sprint(v != 0)
			if v == 0 {
				result += "; " + err.Error()
			}
			r.Events = append(r.Events, a.event(fmt.Sprintf("SetWindowPos(%d,flags=3)", position), result, "success boolean; TOPMOST then NOTOPMOST"))
		}
	case "shutdown":
		showWindow.Call(a.hwnd, 0)
		r.OK, r.HR = cloak(a.hwnd, uint32(os.Getpid()), false)
		_ = a.encoder.Encode(r)
		postMessage.Call(a.hwnd, wmClose, 0, 0)
		return
	default:
		r.OK, r.Error = false, "unknown operation"
	}
	if err := a.encoder.Encode(r); err != nil {
		postMessage.Call(a.hwnd, wmClose, 0, 0)
	}
}
