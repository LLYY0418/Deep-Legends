package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var user32 = windows.NewLazySystemDLL("user32.dll")

func visible(pid uint32) bool {
	found := false
	callback := syscall.NewCallback(func(hwnd, unused uintptr) uintptr {
		var owner uint32
		user32.NewProc("GetWindowThreadProcessId").Call(hwnd, uintptr(unsafe.Pointer(&owner)))
		if owner == pid {
			shown, _, _ := user32.NewProc("IsWindowVisible").Call(hwnd)
			if shown != 0 {
				found = true
			}
		}
		return 1
	})
	user32.NewProc("EnumWindows").Call(callback, 0)
	return found
}

type probe struct {
	cmd   *exec.Cmd
	lines chan string
	acks  atomic.Int32
	done  chan struct{}
}

func launch(exe string, args []string, dir string) (*probe, error) {
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Env = append(os.Environ(), "LOL_LOOT_DATA_DIR="+dir)
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	p := &probe{cmd: cmd, lines: make(chan string, 8), done: make(chan struct{})}
	go func() {
		s := bufio.NewScanner(pipe)
		for s.Scan() {
			line := s.Text()
			if line == "DEEP_LEGENDS_UPDATE_STARTED" {
				p.acks.Add(1)
			}
			select {
			case p.lines <- line:
			default:
			}
		}
		_ = cmd.Wait()
		close(p.done)
	}()
	return p, nil
}
func (p *probe) stop() {
	_ = p.cmd.Process.Kill()
	select {
	case <-p.done:
	case <-time.After(10 * time.Second):
		panic("owned installer did not exit")
	}
}
func (p *probe) wait(line string) error {
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case got := <-p.lines:
			if got == line {
				return nil
			}
		case <-p.done:
			return errors.New("installer exited before " + line)
		case <-deadline.C:
			return errors.New("timeout " + line)
		}
	}
}
func signal() (string, windows.Handle, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", 0, err
	}
	token := hex.EncodeToString(b[:])
	name, _ := windows.UTF16PtrFromString(`Local\DeepLegends.Update.Standby.` + token)
	event, err := windows.CreateEvent(nil, 1, 0, name)
	return token, event, err
}
func args(token, dest string, parent, owner int) []string {
	return []string{"--update", "--parent-first", "--parent-pid", strconv.Itoa(parent), "--owner-pid", strconv.Itoa(owner), "--dest", dest, "--standby", "--standby-token", token}
}
func main() {
	if len(os.Args) > 1 && os.Args[1] == "--owner-fixture" {
		fmt.Println("READY")
		time.Sleep(time.Minute)
		return
	}
	if len(os.Args) != 3 {
		panic("setup output")
	}
	setup, out := os.Args[1], os.Args[2]
	if err := os.MkdirAll(out, 0700); err != nil {
		panic(err)
	}
	rows := []map[string]any{}
	var maxMS int64
	for i := 0; i < 10; i++ {
		func() {
			dir := filepath.Join(out, fmt.Sprintf("trial-%d", i+1))
			dest := filepath.Join(dir, "installed")
			if err := os.MkdirAll(dest, 0700); err != nil {
				panic(err)
			}
			token, event, err := signal()
			if err != nil {
				panic(err)
			}
			defer windows.CloseHandle(event)
			p, err := launch(setup, args(token, dest, os.Getpid(), os.Getpid()), dir)
			if err != nil {
				panic(err)
			}
			defer p.stop()
			if err = p.wait("DEEP_LEGENDS_UPDATE_STANDBY_READY"); err != nil {
				panic(err)
			}
			before := visible(uint32(p.cmd.Process.Pid))
			if before {
				panic("standby window became visible before user start")
			}
			files, _ := os.ReadDir(dest)
			if len(files) != 0 {
				panic("standby modified install directory")
			}
			started := time.Now()
			if err = windows.SetEvent(event); err != nil {
				panic(err)
			}
			if err = p.wait("DEEP_LEGENDS_UPDATE_STARTED"); err != nil {
				panic(err)
			}
			ms := time.Since(started).Milliseconds()
			maxMS = max(maxMS, ms)
			after := visible(uint32(p.cmd.Process.Pid))
			if !after {
				panic("ACK before visible progress page")
			}
			if err = windows.SetEvent(event); err != nil {
				panic(err)
			}
			time.Sleep(100 * time.Millisecond)
			if p.acks.Load() != 1 {
				panic("duplicate signal produced duplicate ACK")
			}
			raw, err := os.ReadFile(filepath.Join(dir, "update-install-timing.json"))
			if err != nil {
				panic(err)
			}
			var timing map[string]int64
			if err = json.Unmarshal(raw, &timing); err != nil {
				panic(err)
			}
			if timing["page_visible"] <= 0 || timing["page_visible"] < timing["standby_signal_received"] || timing["ack_sent"] < timing["page_visible"] {
				panic("invalid actual visible/ACK timing")
			}
			row := map[string]any{"sample": i + 1, "signal_to_ack_ms": ms, "visible_before": before, "visible_after": after, "acks": p.acks.Load(), "timing": timing}
			rows = append(rows, row)
			data, _ := json.MarshalIndent(rows, "", "  ")
			_ = os.WriteFile(filepath.Join(out, "samples.json"), data, 0600)
		}()
	}
	// Invalid token has no matching local event, so it must silently exit.
	for _, kind := range []string{"wrong-token", "parent-exit", "owner-exit", "cancel"} {
		func() {
			dir := filepath.Join(out, kind)
			dest := filepath.Join(dir, "installed")
			_ = os.MkdirAll(dest, 0700)
			token, event, err := signal()
			if err != nil {
				panic(err)
			}
			defer windows.CloseHandle(event)
			parent, owner := os.Getpid(), os.Getpid()
			var fixture *probe
			if kind == "parent-exit" || kind == "owner-exit" {
				fixture, err = launch(os.Args[0], []string{"--owner-fixture"}, dir)
				if err != nil {
					panic(err)
				}
				defer fixture.stop()
				if err = fixture.wait("READY"); err != nil {
					panic(err)
				}
				if kind == "parent-exit" {
					parent = fixture.cmd.Process.Pid
				} else {
					owner = fixture.cmd.Process.Pid
				}
			}
			if kind == "wrong-token" {
				if token[0] == 'a' {
					token = "b" + token[1:]
				} else {
					token = "a" + token[1:]
				}
			}
			p, err := launch(setup, args(token, dest, parent, owner), dir)
			if err != nil {
				panic(err)
			}
			defer p.stop()
			if kind != "wrong-token" {
				if err = p.wait("DEEP_LEGENDS_UPDATE_STANDBY_READY"); err != nil {
					panic(err)
				}
			}
			if fixture != nil {
				fixture.stop()
			}
			if kind == "cancel" {
				p.stop()
			}
			select {
			case <-p.done:
			case <-time.After(5 * time.Second):
				panic(kind + " left a standby process")
			}
			if p.acks.Load() != 0 || visible(uint32(p.cmd.Process.Pid)) {
				panic(kind + " unexpectedly activated")
			}
			files, _ := os.ReadDir(dest)
			if len(files) != 0 {
				panic(kind + " modified install directory")
			}
		}()
	}
	result := map[string]any{"count": len(rows), "max_signal_to_ack_ms": maxMS, "limit_ms": 400, "rows": rows, "exit_paths": []string{"wrong-token", "parent-exit", "owner-exit", "cancel"}}
	raw, _ := json.MarshalIndent(result, "", "  ")
	_ = os.WriteFile(filepath.Join(out, "result.json"), raw, 0600)
	fmt.Println(string(raw))
	if maxMS > 400 {
		panic("standby exceeds original 400ms goal")
	}
}
