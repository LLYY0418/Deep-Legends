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
type updateSpeedWindow struct{ samples []updateSpeedSample }

func (w *updateSpeedWindow) speed(at time.Time, received int64) float64 {
	w.samples = append(w.samples, updateSpeedSample{at, received})
	cutoff := at.Add(-5 * time.Second)
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
	if !u.status.Supported || u.status.Portable || u.status.ManualOnly {
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
	u.status.State, u.status.Error, u.status.Progress = "downloading", "", updateProgress{TotalBytes: manifest.Asset.Size}
	mirrors := append([]string(nil), u.mirrors...)
	u.mu.Unlock()
	u.publish()
	go func() {
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
	part := filepath.Join(u.directory, asset.Name+".part")
	var last error
	for attempt := 0; attempt < 2; attempt++ {
		checksumFailed := false
		for _, prefix := range mirrors {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			err := u.downloadSource(ctx, asset, prefix+asset.URL, part)
			if err == nil {
				return os.Rename(part, filepath.Join(u.directory, asset.Name))
			}
			last = err
			if errors.Is(err, errUpdateChecksum) {
				_ = os.Remove(part)
				checksumFailed = true
				break
			}
		}
		if !checksumFailed {
			return last
		}
	}
	return errUpdateChecksum
}

// The watchdog bounds inactivity, not the total transfer duration. A healthy
// download may take minutes; every received chunk resets the eight-second clock.
func (u *updateManager) downloadSource(parent context.Context, asset updateAsset, target, part string) error {
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
		return fmt.Errorf("%s：HTTP %d", target, response.StatusCode)
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
	go func() {
		defer close(watchStopped)
		ticks, stopTicks := u.downloadProgressTicks()
		defer stopTicks()
		window := updateSpeedWindow{}
		window.speed(time.Now(), offset)
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
			speed := window.speed(time.Now(), current)
			eta := int64(0)
			if speed > 0 {
				eta = int64(math.Ceil(float64(asset.Size-current) / speed))
			}
			progress := updateProgress{current, asset.Size, speed, eta}
			u.mu.Lock()
			u.status.Progress = progress
			u.mu.Unlock()
			if u.notify != nil {
				u.notify("update:progress", progress)
			}
		}
	}()
	writer := &updateDownloadWriter{file: file, received: &received, activity: &activity, pulses: pulses, nextPulse: offset + 512*1024}
	count, copyErr := io.Copy(writer, io.TeeReader(io.LimitReader(response.Body, asset.Size-offset+1), hash))
	close(watchDone)
	<-watchStopped
	if parent.Err() != nil {
		return parent.Err()
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

func (u *updateManager) Apply() error {
	u.mu.Lock()
	if !u.status.Supported || u.status.Portable || u.status.ManualOnly || u.installDir == "" {
		u.mu.Unlock()
		return errors.New("便携版或旧版客户端请从发布页下载完整安装包")
	}
	if u.status.State != "ready" || u.manifest == nil || compareVersions(u.manifest.Version, u.status.Current) <= 0 {
		u.mu.Unlock()
		return errors.New("安装包尚未就绪")
	}
	asset, dest := u.manifest.Asset, u.installDir
	u.status.State = "applying"
	u.status.Error = ""
	u.mu.Unlock()
	u.publish()
	path := filepath.Join(u.directory, asset.Name)
	err := verifyUpdateFile(u.ctx, path, asset)
	if err == nil {
		err = u.launch(path, dest)
	}
	if err != nil {
		u.mu.Lock()
		u.status.State = "failed"
		u.status.Error = err.Error()
		u.mu.Unlock()
		u.publish()
	}
	return err
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
