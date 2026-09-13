package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

type recorder struct {
	mu       sync.Mutex
	dir      string
	file     *os.File
	start    time.Time
	writeErr error
}

func newRecorder(exeDir string) (*recorder, error) {
	prefix := "WindowProbe-" + time.Now().Format("20060102-150405") + "-"
	dir, err := os.MkdirTemp(exeDir, prefix)
	if err != nil {
		dir, err = os.MkdirTemp("", prefix)
	}
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "trace.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	return &recorder{dir: dir, file: f, start: time.Now()}, nil
}

func (r *recorder) event(stage, kind string, data any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.writeErr != nil {
		return
	}
	r.writeErr = json.NewEncoder(r.file).Encode(map[string]any{
		"at_utc":     time.Now().UTC().Format(time.RFC3339Nano),
		"elapsed_ms": time.Since(r.start).Milliseconds(), "stage": stage, "kind": kind, "data": data,
	})
}

type finalReport struct {
	Schema     int            `json:"schema"`
	Version    string         `json:"version"`
	Platform   string         `json:"platform"`
	Windows    map[string]any `json:"windows"`
	StartedUTC string         `json:"started_utc"`
	DurationMS int64          `json:"duration_ms"`
	CleanupOK  bool           `json:"cleanup_ok"`
	Error      string         `json:"error,omitempty"`
	Finding    finding        `json:"finding"`
	Trials     []trial        `json:"trials"`
	Limits     []string       `json:"limits"`
}

func (r *recorder) finish(trials []trial, cleanup bool, runErr error, windows map[string]any) (finding, error) {
	f := assess(trials, cleanup)
	errText := ""
	if runErr != nil {
		errText = runErr.Error()
		f = finding{"incomplete", "测试中止或发生错误；详见报告，不能视为抑制成功。"}
	}
	r.event("final", "assessment", f)
	r.mu.Lock()
	defer r.mu.Unlock()
	defer r.file.Close()
	if r.writeErr != nil {
		return f, fmt.Errorf("诊断记录写入失败: %w", r.writeErr)
	}
	if err := r.file.Sync(); err != nil {
		return f, err
	}
	report := finalReport{Schema: 1, Version: version, Platform: runtime.GOOS + "/" + runtime.GOARCH,
		Windows: windows, StartedUTC: r.start.UTC().Format(time.RFC3339Nano), DurationMS: time.Since(r.start).Milliseconds(),
		CleanupOK: cleanup, Error: errText, Finding: f, Trials: trials,
		Limits: []string{
			"只操作本工具创建的模拟进程/窗口；不连接LOL，不读取客户端或账号，不注入、不写注册表。",
			"self_cloak 是模拟窗口在自身进程内遮蔽的对照，不证明外部进程拥有同样权限。",
			"跨进程通信仅匿名管道，无网络监听。外部窗口只分类为 other，不记录标题、内容、进程名或截图。",
			"20ms定时采样不是显示器逐帧捕获；逐API读回也会改变模拟的时间特性。未观察到不等于零闪烁。",
			"ShowWindow返回值表示之前是否可见，不是成功/失败；HRESULT与状态读回分别记录。",
			"visible不等于实际可见：同时考虑最小化、DWM遮蔽；读回失败记为未知。",
		}}
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return f, err
	}
	if err := os.WriteFile(filepath.Join(r.dir, "report.json"), b, 0600); err != nil {
		return f, err
	}
	text := "Deep Legends 独立窗口实验 " + version + "\r\n\r\n" + f.Summary + "\r\n结果代码: " + f.Code +
		"\r\n\r\n这不是游戏修复程序。基线和恢复阶段故意允许模拟窗口弹出。\r\n请发送本目录的 WindowProbe-Report.zip；如实际看见闪烁，也请说明。\r\n"
	if errText != "" {
		text += "\r\n错误: " + errText + "\r\n"
	}
	if err := os.WriteFile(filepath.Join(r.dir, "结果说明.txt"), []byte("\xef\xbb\xbf"+text), 0600); err != nil {
		return f, err
	}
	return f, zipReport(r.dir)
}

func zipReport(dir string) (err error) {
	out := filepath.Join(dir, "WindowProbe-Report.zip")
	f, err := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	w := zip.NewWriter(f)
	defer func() {
		if e := w.Close(); err == nil {
			err = e
		}
		if e := f.Close(); err == nil {
			err = e
		}
		if err != nil {
			_ = os.Remove(out)
		}
	}()
	for _, name := range []string{"report.json", "结果说明.txt", "trace.jsonl"} {
		src, e := os.Open(filepath.Join(dir, name))
		if e != nil {
			return e
		}
		dst, e := w.Create(name)
		if e == nil {
			_, e = io.Copy(dst, src)
		}
		_ = src.Close()
		if e != nil {
			return e
		}
	}
	return nil
}
