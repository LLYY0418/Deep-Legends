package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"deeplegends/installer/payload"
)

func releasePayload() (setup, directory string, err error) {
	source, err := payload.Open()
	if err != nil {
		return "", "", err
	}
	defer source.Close()
	for attempt := 0; attempt < 8; attempt++ {
		var random [4]byte
		if _, err := rand.Read(random[:]); err != nil {
			return "", "", err
		}
		directory = filepath.Join(os.TempDir(), "DeepLegendsSetup-"+hex.EncodeToString(random[:]))
		if err = os.Mkdir(directory, 0700); os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", "", err
		}
		setup = filepath.Join(directory, "Deep Legends Setup.exe")
		file, err := os.OpenFile(setup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err == nil {
			_, err = io.Copy(file, source)
			closeErr := file.Close()
			if err == nil {
				err = closeErr
			}
		}
		if err != nil {
			_ = os.RemoveAll(directory)
			return "", "", err
		}
		return setup, directory, nil
	}
	return "", "", fmt.Errorf("could not allocate installer temporary directory")
}

// Remove only empty directories created by this validation attempt, never an
// existing installation or unrelated files. Used on preflight failures only.
func prepareDestination(input string, installedBytes int64) (string, *failureMessage) {
	dest, err := normalizeInstallDir(input)
	if err != nil {
		return "", &failureMessage{Message: err.Error()}
	}
	var missing []string
	for directory := dest; ; directory = filepath.Dir(directory) {
		if _, err := os.Stat(directory); !os.IsNotExist(err) {
			break
		}
		missing = append(missing, directory)
		if parent := filepath.Dir(directory); parent == directory {
			break
		}
	}
	fail := func(message string) (string, *failureMessage) {
		for _, directory := range missing {
			_ = os.Remove(directory)
		}
		return "", &failureMessage{Message: message, Detail: dest}
	}
	if err := os.MkdirAll(dest, 0755); err != nil {
		if os.IsPermission(err) {
			return fail("这个位置不允许写入，请换一个位置")
		}
		return fail("无法创建安装文件夹，请换一个位置")
	}
	probe, err := os.CreateTemp(dest, ".deeplegends-write-*")
	if err != nil {
		return fail("这个位置不允许写入，请换一个位置")
	}
	_, writeErr := probe.Write([]byte("Deep Legends setup write probe"))
	closeErr := probe.Close()
	removeErr := os.Remove(probe.Name())
	if writeErr != nil || closeErr != nil || removeErr != nil {
		return fail("这个位置不允许写入，请换一个位置")
	}
	free, err := diskFreeBytes(dest)
	if err != nil {
		return fail("无法读取磁盘空间，请选择可用的本机磁盘")
	}
	if free < requiredSpace(installedBytes) {
		return fail("磁盘空间不足，请换一个安装位置")
	}
	return dest, nil
}

func (a *installerApp) install(message uiMessage) {
	installationStarted := time.Now()
	w := a.window
	reportFailure := func(failure failureMessage) {
		if a.options.Update {
			failure.Message = upgradeFailureMessage
		}
		w.dispatch(func() { a.fail(failure) })
	}
	if a.options.Update {
		if err := validateUpgradeDestination(message.Path); err != nil {
			reportFailure(failureMessage{Message: upgradeFailureMessage})
			return
		}
	}
	dest, failure := prepareDestination(message.Path, a.meta.InstalledBytes)
	if failure != nil {
		reportFailure(*failure)
		return
	}
	w.dispatch(func() {
		a.emit("installing", struct {
			Path string `json:"path"`
		}{dest})
	})
	setup, temporaryDir, err := releasePayload()
	if err != nil {
		reportFailure(failureMessage{Message: "无法释放安装包，请检查临时磁盘空间后重试", Detail: dest})
		return
	}
	defer os.RemoveAll(temporaryDir)
	cmd := exec.Command(setup)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CmdLine: installerCommandLine(setup, dest, message.DesktopShortcut, a.options.Update)}
	// Give this child its own TEMP so parallel installers cannot inflate progress.
	// NSIS still uses its normal ns*.tmp/7z-out layout inside this directory.
	cmd.Env = installerEnvironment(temporaryDir)
	a.startupWarm = newConfiguredExecutionWarmup(dest, installationStarted, executeWarmHooks{})
	defer a.startupWarm.Finish()
	started := time.Now()
	if err := cmd.Start(); err != nil {
		reportFailure(failureMessage{Message: "无法启动安装，请重新下载安装包后重试", Detail: "退出码：未启动；目标路径：" + dest})
		return
	}
	finished := make(chan struct{})
	var waitErr error
	go func() { waitErr = cmd.Wait(); close(finished) }()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	model := progressModel{installedBytes: a.meta.InstalledBytes}
	for {
		select {
		case <-finished:
			completeInstallation(a.options, installationResult{ExitCode: cmd.ProcessState.ExitCode(), WaitError: waitErr, Destination: dest}, installationCompletionHooks{
				Failed:  reportFailure,
				Cleanup: func() { _ = os.RemoveAll(temporaryDir) },
				Handoff: func() { a.handoffApplication(filepath.Join(dest, a.meta.ExeName), dest) },
			})
			return
		case <-ticker.C:
			a.startupWarm.Poll(time.Now(), finished)
			extracted := extractedBytes(temporaryDir, started, finished)
			copied := directoryBytes(dest, finished)
			update := progressMessage{Percent: model.percent(extracted, copied, time.Since(started)), Stage: progressStage(extracted, copied)}
			w.dispatch(func() { a.emit("progress", update) })
		}
	}
}

func installerEnvironment(temp string) []string {
	env := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(name, "TEMP") && !strings.EqualFold(name, "TMP") {
			env = append(env, entry)
		}
	}
	return append(env, "TEMP="+temp, "TMP="+temp)
}

func directoryBytes(root string, stopped <-chan struct{}) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		select {
		case <-stopped:
			return fs.SkipAll
		default:
		}
		if err != nil || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if !entry.IsDir() {
			if info, err := entry.Info(); err == nil && info.Mode().IsRegular() {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

// The NSIS extraction directory is used only to report installation progress.
func extractedBytes(temp string, started time.Time, stopped <-chan struct{}) int64 {
	entries, _ := os.ReadDir(temp)
	var total int64
	for _, entry := range entries {
		name := strings.ToLower(entry.Name())
		if !entry.IsDir() || !strings.HasPrefix(name, "ns") || !strings.HasSuffix(name, ".tmp") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		attributes, ok := info.Sys().(*syscall.Win32FileAttributeData)
		if !ok || attributes.CreationTime.Nanoseconds() < started.UnixNano() {
			continue
		}
		total += directoryBytes(filepath.Join(temp, entry.Name(), "7z-out"), stopped)
	}
	return total
}
