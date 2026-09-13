//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type boundedStderr struct {
	sync.Mutex
	data bytes.Buffer
}

func (b *boundedStderr) Write(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	n := len(p)
	if left := 8192 - b.data.Len(); left > 0 {
		b.data.Write(p[:min(left, len(p))])
	}
	return n, nil
}
func (b *boundedStderr) text() string { b.Lock(); defer b.Unlock(); return b.data.String() }

type actorClient struct {
	cmd           *exec.Cmd
	in            io.WriteCloser
	out           io.ReadCloser
	replies       chan response
	readerDone    chan struct{}
	exited        chan struct{}
	waitErr       error // Read only after exited has closed.
	stderr        boundedStderr
	nextID        int
	pid           uint32
	processHandle uintptr
	hwnd          uintptr
	anchor        uintptr
	recorder      *recorder
}

func startActor(ctx context.Context, anchor uintptr, rec *recorder) (*actorClient, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	c := &actorClient{cmd: exec.Command(exe, "--internal-probe-child"), replies: make(chan response, 32), readerDone: make(chan struct{}), exited: make(chan struct{}), anchor: anchor, recorder: rec}
	c.cmd.Stderr = &c.stderr
	c.in, err = c.cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	c.out, err = c.cmd.StdoutPipe()
	if err != nil {
		c.in.Close()
		return nil, err
	}
	if err := c.cmd.Start(); err != nil {
		c.in.Close()
		c.out.Close()
		return nil, err
	}
	c.pid = uint32(c.cmd.Process.Pid)
	// Drain stdout before Wait closes StdoutPipe; otherwise the final response can be lost.
	go func() { <-c.readerDone; c.waitErr = c.cmd.Wait(); close(c.exited) }()
	go func() {
		defer close(c.readerDone)
		defer close(c.replies)
		decoder := json.NewDecoder(io.LimitReader(c.out, 2<<20))
		for {
			var r response
			if err := decoder.Decode(&r); err != nil {
				rec.event("child", "pipe_reader_ended", err.Error())
				return
			}
			rec.event("child", "response", r)
			select {
			case c.replies <- r:
			default:
				rec.event("child", "protocol_overflow", true)
				return
			}
		}
	}()
	// Retain a kernel handle for the process identity, not merely its reusable PID.
	processHandle, _, openErr := kernel.NewProc("OpenProcess").Call(0x00100000, 0, uintptr(c.pid)) // SYNCHRONIZE only.
	if processHandle == 0 {
		c.close()
		return nil, fmt.Errorf("无法固定模拟进程身份: %v", openErr)
	}
	c.processHandle = processHandle
	select {
	case r, ok := <-c.replies:
		if !ok || !r.OK || r.ID != 0 || r.PID != c.pid || !targetOK(uintptr(r.HWND), c.pid) {
			c.close()
			return nil, errors.New("模拟窗口未建立或PID/HWND校验失败")
		}
		c.hwnd = uintptr(r.HWND)
	case <-ctx.Done():
		c.close()
		return nil, ctx.Err()
	case <-time.After(5 * time.Second):
		c.close()
		return nil, errors.New("启动模拟进程超时")
	}
	if _, err := c.call(ctx, "startup", command{Op: "init", OwnerPID: uint32(os.Getpid()), Anchor: uint64(anchor)}); err != nil {
		c.close()
		return nil, err
	}
	return c, nil
}

func (c *actorClient) call(ctx context.Context, stage string, req command) (response, error) {
	if ctx.Err() != nil {
		return response{}, ctx.Err()
	}
	c.nextID++
	req.ID = c.nextID
	c.recorder.event(stage, "command", req)
	if err := json.NewEncoder(c.in).Encode(req); err != nil {
		return response{}, err
	}
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case r, ok := <-c.replies:
			if !ok {
				return r, errors.New("模拟进程已退出或通信中断")
			}
			if r.ID < req.ID {
				continue
			} // Late response after cancellation, already in trace.
			if r.ID != req.ID {
				return r, errors.New("管道响应序号不匹配")
			}
			if !r.OK {
				return r, fmt.Errorf("%s失败: %s %s", req.Op, r.Error, r.HR)
			}
			return r, nil
		case <-ctx.Done():
			return response{}, ctx.Err()
		case <-timeout.C:
			return response{}, errors.New("模拟窗口响应超时")
		}
	}
}

func (c *actorClient) alive() bool {
	if c.processHandle == 0 {
		return false
	}
	state, _, _ := kernel.NewProc("WaitForSingleObject").Call(c.processHandle, 0)
	if state != 258 {
		return false
	} // Only WAIT_TIMEOUT means the captured process is still running.
	select {
	case <-c.exited:
		return false
	default:
		return targetOK(c.hwnd, c.pid)
	}
}

func (c *actorClient) close() bool {
	defer func() {
		if c.processHandle != 0 {
			kernel.NewProc("CloseHandle").Call(c.processHandle)
			c.processHandle = 0
		}
	}()
	// Closing the anonymous pipe makes the child restore/destroy its OWN window.
	// A bounded fallback kills ONLY the process handle created by cmd.Start.
	if c.hwnd != 0 && c.alive() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		r, err := c.call(ctx, "cleanup", command{Op: "shutdown"})
		cancel()
		c.recorder.event("cleanup", "shutdown_reply", map[string]any{"reply": r, "error": fmt.Sprint(err)})
	}
	_ = c.in.Close()
	forced := false
	select {
	case <-c.exited:
	case <-time.After(2 * time.Second):
		forced = true
		_ = c.cmd.Process.Kill()
		select {
		case <-c.exited:
		case <-time.After(2 * time.Second):
			c.recorder.event("cleanup", "child_exit_timeout", true)
			return false
		}
	}
	_ = c.out.Close()
	c.recorder.event("cleanup", "child_exited", map[string]any{"forced": forced, "error": fmt.Sprint(c.waitErr), "stderr": c.stderr.text()})
	// Destruction also removes DWM attributes; no third-party window was altered.
	return true
}

func pause(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *actorClient) observe(ctx context.Context, t *trial, duration time.Duration) error {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(duration)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.exited:
			return errors.New("模拟进程提前退出")
		case <-ticker.C:
			s := sample(c.hwnd, c.pid, uint32(os.Getpid()), c.anchor)
			t.Samples = append(t.Samples, s)
			c.recorder.event(t.Name, "external_sample", s)
		case <-timer.C:
			return nil
		}
	}
}

func (c *actorClient) runTrial(ctx context.Context, name string) (t trial, err error) {
	t.Name = name
	defer func() {
		if err != nil {
			t.Error = err.Error()
		}
		c.recorder.event(name, "trial_finished", map[string]any{"complete": t.Complete, "bursts": t.Bursts, "error": t.Error})
	}()
	if !c.alive() {
		return t, errors.New("模拟窗口已关闭")
	}
	if _, err = c.call(ctx, name, command{Op: "reset"}); err != nil {
		return
	}
	if !targetOK(c.anchor, uint32(os.Getpid())) {
		return t, errors.New("控制窗口已关闭")
	}
	v, _, _ := setForeground.Call(c.anchor)
	c.recorder.event(name, "prepare_anchor_foreground", v != 0)
	if err = pause(ctx, 250*time.Millisecond); err != nil {
		return
	}
	if name == "external_cloak" {
		if !c.alive() {
			return t, errors.New("目标PID已失效")
		}
		_, t.CloakAttempt = cloak(c.hwnd, c.pid, true)
		c.recorder.event(name, "cross_process_cloak", t.CloakAttempt)
	} else if name == "self_cloak" {
		// Deliberately separate positive control, never used as a silent fallback.
		var r response
		r, err = c.call(ctx, name, command{Op: "self_cloak"})
		t.CloakAttempt = r.HR
		if err != nil && r.ID != 0 && r.HR != "" {
			// A reported unsupported/rejected HRESULT is a result, not a transport failure.
			err = nil
		} else if err != nil {
			return
		}
	}
	if err = pause(ctx, 150*time.Millisecond); err != nil {
		return
	}
	t.Before = sample(c.hwnd, c.pid, uint32(os.Getpid()), c.anchor)
	t.AnchorPrepared = t.Before.ForegroundAnchor
	t.CloakApplied = t.Before.DWMOK && t.Before.Cloaked&1 != 0
	c.recorder.event(name, "before_burst", t.Before)
	if (name == "external_cloak" || name == "self_cloak") && (t.CloakAttempt != "0x00000000" || !t.CloakApplied) {
		t.Complete = true
		t.SkippedReason = "遮蔽未应用，跳过无意义的重复弹窗；不判作成功"
		c.recorder.event(name, "burst_skipped", t.SkippedReason)
		return
	}
	bursts, hold := 1, 1000*time.Millisecond
	if name == "external_cloak" {
		bursts, hold = 3, 650*time.Millisecond
	}
	for i := 0; i < bursts; i++ {
		// Give ONLY our synthetic child foreground eligibility to avoid a weak baseline.
		v, _, _ := allowForeground.Call(uintptr(c.pid))
		c.recorder.event(name, "allow_owned_child_foreground", v != 0)
		var r response
		r, err = c.call(ctx, name, command{Op: "burst"})
		if err != nil {
			return
		}
		for _, e := range r.Events {
			t.Samples = append(t.Samples, e.Snapshot)
		}
		t.Bursts++
		if err = c.observe(ctx, &t, hold); err != nil {
			return
		}
	}
	t.Complete = true
	return
}

func runExperiment(ctx context.Context, anchor uintptr, status func(string), done func(string, string)) {
	exe, err := os.Executable()
	if err != nil {
		done("无法定位测试程序："+err.Error(), "")
		return
	}
	rec, err := newRecorder(filepath.Dir(exe))
	if err != nil {
		done("无法创建报告目录，未运行测试："+err.Error(), "")
		return
	}
	var trials []trial
	var c *actorClient
	cleanup := true
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("测试异常: %v", p)
		}
		if c != nil {
			cleanup = c.close()
		}
		f, saveErr := rec.finish(trials, cleanup, err, windowsVersion())
		message := f.Summary
		if saveErr != nil {
			message += "\r\n报告未完整保存：" + saveErr.Error()
		}
		done(message, rec.dir)
	}()
	rec.event("startup", "scope", "synthetic processes only; no game files, network, registry or injection")
	c, err = startActor(ctx, anchor, rec)
	if err != nil {
		return
	}
	stages := []struct{ name, label string }{
		{"baseline", "1/4 基线对照：模拟窗口会故意弹出。请暂时不要操作鼠标键盘。"},
		{"external_cloak", "2/4 外部遮蔽：由另一个进程提前请求遮蔽，再模拟 ShowMain。"},
		{"self_cloak", "3/4 自身遮蔽对照：仅用于区分权限限制，不是可交付修复。"},
		{"release_show", "4/4 解除并恢复：模拟进入选人，测试窗口应重新正常显示。"},
	}
	for _, stage := range stages {
		status(stage.label)
		var t trial
		t, err = c.runTrial(ctx, stage.name)
		trials = append(trials, t)
		if err != nil {
			return
		}
	}
}
