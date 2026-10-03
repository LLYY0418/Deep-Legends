package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

var errUpdateChecksum = errors.New("安装包校验失败，请从发布页手动下载")

type updateSpeedSample struct {
	at    time.Time
	bytes int64
}
type updateSpeedWindow struct {
	samples  []updateSpeedSample
	duration time.Duration
}

func (w *updateSpeedWindow) speed(at time.Time, received int64) float64 {
	w.samples = append(w.samples, updateSpeedSample{at, received})
	duration := w.duration
	if duration == 0 {
		duration = 5 * time.Second
	}
	cutoff := at.Add(-duration)
	for len(w.samples) > 2 && !w.samples[1].at.After(cutoff) {
		w.samples = w.samples[1:]
	}
	first := w.samples[0]
	if first.at.Before(cutoff) && len(w.samples) > 1 {
		next := w.samples[1]
		span := next.at.Sub(first.at).Seconds()
		if span > 0 {
			first.bytes += int64(float64(next.bytes-first.bytes) * cutoff.Sub(first.at).Seconds() / span)
		}
		first.at = cutoff
	}
	elapsed := at.Sub(first.at).Seconds()
	if elapsed <= 0 {
		return 0
	}
	return math.Max(0, float64(received-first.bytes)/elapsed)
}

type updateContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r updateContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
func verifyUpdateFile(ctx context.Context, path string, asset updateAsset) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != asset.Size {
		return errUpdateChecksum
	}
	hash := sha256.New()
	if _, err = io.Copy(hash, updateContextReader{ctx, file}); err != nil {
		return err
	}
	return verifyUpdateDigest(hex.EncodeToString(hash.Sum(nil)), asset.SHA256)
}
func verifyUpdateDigest(actual, expected string) error {
	if !strings.EqualFold(actual, expected) {
		return errUpdateChecksum
	}
	return nil
}

func (u *updateManager) Download() error {
	u.mu.Lock()
	if !u.status.Supported {
		u.mu.Unlock()
		return errors.New("请从发布页下载完整安装包")
	}
	if u.busyLocked() || u.manifest == nil || compareVersions(u.manifest.Version, u.status.Current) <= 0 {
		u.mu.Unlock()
		return errors.New("当前没有可下载的更新")
	}
	manifest := *u.manifest
	free, err := u.freeBytes(u.directory)
	if err == nil && free < int64(math.Ceil(float64(manifest.Asset.Size)*1.2)) {
		err = errors.New("磁盘剩余空间不足，请至少预留安装包大小的 1.2 倍空间")
	}
	if err != nil {
		u.status.State = "failed"
		u.status.Error = err.Error()
		u.mu.Unlock()
		u.publish()
		return err
	}
	ctx, cancel := context.WithCancel(u.ctx)
	done := make(chan struct{})
	u.downloadCancel, u.downloadDone = cancel, done
	u.status.State, u.status.Error, u.status.Progress = "downloading", "", updateProgress{TotalBytes: manifest.Asset.Size, SelectingSource: true}
	mirrors := u.sourcesLocked()
	u.mu.Unlock()
	u.publish()
	go func() {
		defer recoverPanic("update_download.Download.1")

		defer close(done)
		defer cancel()
		err := u.downloadAsset(ctx, manifest.Asset, mirrors)
		if ctx.Err() != nil {
			_ = os.Remove(filepath.Join(u.directory, manifest.Asset.Name+".part"))
		}
		u.mu.Lock()
		u.downloadCancel = nil
		if ctx.Err() != nil {
			u.status.State, u.status.Error, u.status.Progress = "available", "", updateProgress{}
		} else if err != nil {
			u.status.State, u.status.Error = "failed", err.Error()
		} else {
			u.status.State, u.status.Error = "ready", ""
			u.status.Progress = updateProgress{ReceivedBytes: manifest.Asset.Size, TotalBytes: manifest.Asset.Size}
		}
		u.mu.Unlock()
		u.publish()
	}()
	return nil
}
func (u *updateManager) Cancel() error {
	u.mu.Lock()
	cancel, done := u.downloadCancel, u.downloadDone
	u.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	<-done
	return nil
}
func (u *updateManager) downloadAsset(ctx context.Context, asset updateAsset, mirrors []string) error {
	start := time.Now()
	part := filepath.Join(u.directory, asset.Name+".part")
	received := func() int64 {
		if info, err := os.Stat(part); err == nil {
			return info.Size()
		}
		return u.Status().Progress.ReceivedBytes
	}
	u.recordUpdateCheck(map[string]any{"event": "update_proxy_env", "http_proxy_set": updateProxyEnvSet("HTTP_PROXY"), "https_proxy_set": updateProxyEnvSet("HTTPS_PROXY")})
	probes := u.probeUpdateSources(ctx, asset, mirrors)
	if ctx.Err() == nil {
		u.mu.Lock()
		u.status.Progress.SelectingSource = false
		u.mu.Unlock()
		u.publish()
	}
	sources := make([]updateSourceProbe, 0, len(mirrors))
	sources = append(sources, probes...)
	// If every probe fails, keep the previous sequential fallback exactly.
	if len(sources) == 0 {
		for _, prefix := range mirrors {
			sources = append(sources, updateSourceProbe{prefix: prefix})
		}
	}
	var last error
	switches := 0
	var resumed int64
	var downloaded int64
	if info, err := os.Stat(part); err == nil && info.Size() <= asset.Size {
		resumed = info.Size()
	}
	prefix := ""
	defer func() {
		if ctx.Err() != nil {
			u.recordUpdateCheck(map[string]any{"event": "update_download_cancelled", "received_bytes": received()})
		}
	}()
	for attempt := 0; attempt < 2; attempt++ {
		checksumFailed := false
		for i, source := range sources {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			prefix = source.prefix
			offset := int64(0)
			if info, err := os.Stat(part); err == nil {
				offset = info.Size()
			}
			u.recordUpdateCheck(map[string]any{"event": "update_download_source_selected", "mirror_prefix": updateDiagnosticMirrorPrefix(prefix), "speed_kbps": source.speed / 1024, "probe_measured": source.ok, "resume_offset": offset})
			// All measured sources slow: keep the fastest, avoiding pointless churn.
			canSwitch := len(probes) > 0 && probes[0].speed >= updateSlowSpeed && i+1 < len(probes)
			transfer := updateSourceTransfer{}
			err := u.downloadSource(ctx, asset, prefix+asset.URL, part, canSwitch, &transfer)
			downloaded += transfer.received
			if err == nil {
				err = os.Rename(part, filepath.Join(u.directory, asset.Name))
				if err == nil {
					u.rememberUpdateSource(prefix)
					duration := time.Since(start)
					u.recordUpdateCheck(map[string]any{"event": "update_download_finished", "mirror_prefix": updateDiagnosticMirrorPrefix(prefix), "duration_ms": duration.Milliseconds(), "avg_kbps": float64(downloaded) / duration.Seconds() / 1024, "switches": switches, "resumed_bytes": resumed})
					return nil
				}
			}
			last = err
			if errors.Is(err, errUpdateChecksum) {
				_ = os.Remove(part)
				checksumFailed = true
				break
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if i+1 < len(sources) {
				reason := "error"
				if errors.Is(err, errUpdateSourceSlow) {
					reason = "slow"
				}
				switches++
				u.recordUpdateCheck(map[string]any{"event": "update_download_source_switched", "from": updateDiagnosticMirrorPrefix(prefix), "to": updateDiagnosticMirrorPrefix(sources[i+1].prefix), "reason": reason, "speed_kbps": transfer.speed / 1024, "received_bytes": received()})
			}
		}
		if !checksumFailed {
			break
		}
	}
	if last == nil {
		last = errors.New("没有可用下载线路")
	}
	u.recordUpdateCheck(map[string]any{"event": "update_download_failed", "mirror_prefix": updateDiagnosticMirrorPrefix(prefix), "error_kind": diagnosticErrorKind(last), "received_bytes": received()})
	return last
}

// The watchdog bounds inactivity, not the total transfer duration. A healthy
// download may take minutes; every received chunk resets the eight-second clock.
func (u *updateManager) downloadSource(parent context.Context, asset updateAsset, target, part string, canSwitch bool, transfer *updateSourceTransfer) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	file, err := os.OpenFile(part, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	offset := info.Size()
	if offset > asset.Size {
		offset = 0
	}
	if offset == asset.Size {
		return verifyUpdateFile(ctx, part, asset)
	}
	if offset > 0 {
		probeCtx, probeCancel := context.WithTimeout(ctx, updateSourceTimeout)
		req, reqErr := http.NewRequestWithContext(probeCtx, http.MethodHead, target, nil)
		if reqErr != nil {
			probeCancel()
			return reqErr
		}
		response, probeErr := u.client.Do(req)
		supports := false
		if probeErr == nil {
			supports = response.StatusCode == 200 && strings.EqualFold(strings.TrimSpace(response.Header.Get("Accept-Ranges")), "bytes")
			response.Body.Close()
		}
		probeCancel()
		if parent.Err() != nil {
			return parent.Err()
		}
		if !supports {
			offset = 0
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	req.Header.Set("Accept-Encoding", "identity")
	stopHeaderTimer := u.sourceTimer(updateSourceTimeout, cancel)
	response, err := u.client.Do(req)
	stopHeaderTimer()
	if err != nil {
		return err
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
		offset = 0
	case http.StatusPartialContent:
		expected := fmt.Sprintf("bytes %d-%d/%d", offset, asset.Size-1, asset.Size)
		if offset == 0 || response.Header.Get("Content-Range") != expected {
			return errors.New("下载线路返回了错误的续传范围")
		}
	default:
		return fmt.Errorf("下载线路：HTTP %d", response.StatusCode)
	}
	if err = file.Truncate(offset); err != nil {
		return err
	}
	hash := sha256.New()
	if offset > 0 {
		if _, err = io.Copy(hash, updateContextReader{ctx, io.LimitReader(file, offset)}); err != nil {
			return err
		}
	}
	if _, err = file.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	transfer.offset = offset
	var slow atomic.Bool
	started := u.downloadTime()
	var received atomic.Int64
	received.Store(offset)
	var activity atomic.Int64
	activity.Store(time.Now().UnixNano())
	pulses := make(chan struct{}, 1)
	watchDone := make(chan struct{})
	watchStopped := make(chan struct{})
	u.mu.Lock()
	u.status.State = "downloading"
	u.mu.Unlock()
	u.publish()
	goSafe("update_download.downloadSource.1", func() {
		defer close(watchStopped)
		ticks, stopTicks := u.downloadProgressTicks()
		defer stopTicks()
		window := updateSpeedWindow{}
		slowWindow := updateSpeedWindow{duration: 10 * time.Second}
		window.speed(started, offset)
		slowWindow.speed(started, offset)
		for {
			select {
			case <-watchDone:
				return
			case <-ctx.Done():
				return
			case <-ticks:
			case <-pulses:
			}
			if time.Since(time.Unix(0, activity.Load())) >= updateSourceTimeout {
				cancel()
				return
			}
			current := received.Load()
			now := u.downloadTime()
			speed := window.speed(now, current)
			slowSpeed := slowWindow.speed(now, current)
			transfer.speed = slowSpeed
			if canSwitch && now.Sub(started) >= 15*time.Second && slowSpeed < updateSlowSpeed {
				slow.Store(true)
				cancel()
				return
			}
			eta := int64(0)
			if speed > 0 {
				eta = int64(math.Ceil(float64(asset.Size-current) / speed))
			}
			progress := updateProgress{ReceivedBytes: current, TotalBytes: asset.Size, BytesPerSecond: speed, ETASeconds: eta}
			u.mu.Lock()
			u.status.Progress = progress
			u.mu.Unlock()
			if u.notify != nil {
				u.notify("update:progress", progress)
			}
		}
	})
	writer := &updateDownloadWriter{file: file, received: &received, activity: &activity, pulses: pulses, nextPulse: offset + 512*1024}
	count, copyErr := io.Copy(writer, io.TeeReader(io.LimitReader(response.Body, asset.Size-offset+1), hash))
	close(watchDone)
	<-watchStopped
	transfer.received = count
	if parent.Err() != nil {
		return parent.Err()
	}
	if slow.Load() {
		return errUpdateSourceSlow
	}
	if ctx.Err() != nil {
		return errors.New("下载线路超过 8 秒未响应")
	}
	if copyErr != nil {
		return copyErr
	}
	if count+offset != asset.Size {
		return fmt.Errorf("安装包长度不正确：收到 %s 字节", strconv.FormatInt(count+offset, 10))
	}
	finalProgress := updateProgress{ReceivedBytes: asset.Size, TotalBytes: asset.Size}
	u.mu.Lock()
	u.status.Progress = finalProgress
	u.mu.Unlock()
	if u.notify != nil {
		u.notify("update:progress", finalProgress)
	}
	if err = file.Sync(); err != nil {
		return err
	}
	u.mu.Lock()
	u.status.State = "verifying"
	u.mu.Unlock()
	u.publish()
	if err = verifyUpdateDigest(hex.EncodeToString(hash.Sum(nil)), asset.SHA256); err != nil {
		return err
	}
	return file.Close()
}

type updateDownloadWriter struct {
	file               *os.File
	received, activity *atomic.Int64
	pulses             chan struct{}
	nextPulse          int64
}

func (w *updateDownloadWriter) Write(p []byte) (int, error) {
	n, err := w.file.Write(p)
	current := w.received.Add(int64(n))
	if n > 0 {
		w.activity.Store(time.Now().UnixNano())
	}
	if current >= w.nextPulse {
		w.nextPulse = current + 512*1024
		select {
		case w.pulses <- struct{}{}:
		default:
		}
	}
	return n, err
}

func (u *updateManager) Apply() error { return u.apply(false, nil) }

// Portable startup stays asynchronous. The shell exits after the installer
// acknowledges readiness, before it starts replacing application files.
func (u *updateManager) ApplyAsync(success func()) error { return u.apply(true, success) }
func (u *updateManager) apply(async bool, success func()) error {
	u.mu.Lock()
	if !u.status.Supported {
		u.mu.Unlock()
		return errors.New("当前构建不支持更新")
	}
	if u.status.State != "ready" || u.manifest == nil || compareVersions(u.manifest.Version, u.status.Current) <= 0 {
		u.mu.Unlock()
		return errors.New("安装包尚未就绪")
	}
	asset, dest, portable := u.manifest.Asset, u.installDir, u.status.Portable

	u.status.State = "applying"
	u.status.Error = ""
	u.mu.Unlock()
	u.publish()
	path := filepath.Join(u.directory, asset.Name)
	fail := func(err error) error {
		if err != nil {
			u.mu.Lock()
			u.status.State = "failed"
			u.status.Error = err.Error()
			u.mu.Unlock()
			u.publish()
		}
		return err
	}
	if err := verifyUpdateFile(u.ctx, path, asset); err != nil {
		return fail(err)
	}
	if portable {
		var err error
		dest, err = u.portableDirectory()
		if err != nil {
			return fail(err)
		}
	}
	finish := func() error {
		if portable {
			if err := u.checkPortableData(); err != nil {
				return fail(err)
			}
		}
		// Finish migration before readiness acknowledgement starts the 15s parent wait.
		if portable && u.migrate != nil {
			if err := u.migrate(); err != nil {
				return fail(err)
			}
		}
		err := u.launch(path, dest)
		if err = fail(err); err == nil && success != nil && u.ctx.Err() == nil {
			success()
		}
		return err
	}
	if async {
		goSafe("update-portable-apply", func() { _ = finish() })
		return nil
	}
	return finish()
}

func (u *updateManager) sourceTimer(d time.Duration, expire func()) func() {
	if u.startSourceTimer != nil {
		return u.startSourceTimer(d, expire)
	}
	timer := time.AfterFunc(d, expire)
	return func() { timer.Stop() }
}
func (u *updateManager) downloadProgressTicks() (<-chan time.Time, func()) {
	if u.progressTicks != nil {
		return u.progressTicks()
	}
	ticker := time.NewTicker(200 * time.Millisecond)
	return ticker.C, ticker.Stop
}
