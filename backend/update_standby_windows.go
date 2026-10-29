package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type windowsUpdateStandby struct {
	cmd              *exec.Cmd
	event            windows.Handle
	ready, activated atomic.Bool
	ack              chan error
	done             chan struct{}
	once             sync.Once
}

func startUpdateStandby(ctx context.Context, setup, dest string) (updateStandbyProcess, error) {
	var entropy [32]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(entropy[:])
	name, _ := windows.UTF16PtrFromString(`Local\DeepLegends.Update.Standby.` + token)
	event, err := windows.CreateEvent(nil, 1, 0, name)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(setup)
	command := updateRecoveryCommandLine(updateCommandLine(setup, dest), dest) + " --standby --standby-token " + token + " --owner-pid " + strconv.Itoa(os.Getpid())
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: command, HideWindow: true}
	cmd.Env = os.Environ()
	output, err := cmd.StdoutPipe()
	if err != nil {
		windows.CloseHandle(event)
		return nil, err
	}
	if err = ctx.Err(); err == nil {
		err = cmd.Start()
	}
	if err != nil {
		windows.CloseHandle(event)
		return nil, err
	}
	s := &windowsUpdateStandby{cmd: cmd, event: event, ack: make(chan error, 1), done: make(chan struct{})}
	goSafe("update-standby-output", func() {
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 256), 4096)
		for scanner.Scan() {
			switch scanner.Text() {
			case "DEEP_LEGENDS_UPDATE_STANDBY_READY":
				s.ready.Store(true)
			case "DEEP_LEGENDS_UPDATE_STARTED":
				select {
				case s.ack <- nil:
				default:
				}
			}
		}
		_ = cmd.Wait()
		s.ready.Store(false)
		select {
		case s.ack <- errors.New("待命安装程序已退出"):
		default:
		}
		_ = windows.CloseHandle(s.event)
		close(s.done)
	})
	goSafe("update-standby-lifetime", func() {
		select {
		case <-ctx.Done():
			if !s.activated.Load() {
				s.Close()
			}
		case <-s.done:
		}
	})
	return s, nil
}
func (s *windowsUpdateStandby) Ready() bool { return s.ready.Load() && !s.activated.Load() }
func (s *windowsUpdateStandby) Start(mark func(string)) error {
	if !s.ready.Load() || !s.activated.CompareAndSwap(false, true) {
		return errors.New("待命安装程序尚未就绪")
	}
	mark("standby_signal")
	if err := windows.SetEvent(s.event); err != nil {
		return err
	}
	select {
	case err := <-s.ack:
		if err == nil {
			mark("ack_received")
		}
		return err
	case <-time.After(30 * time.Second):
		s.Close()
		return errors.New("安装等待超时，请重试或打开发布页")
	}
}
func (s *windowsUpdateStandby) Close() {
	s.once.Do(func() { _ = s.cmd.Process.Kill(); <-s.done })
}
