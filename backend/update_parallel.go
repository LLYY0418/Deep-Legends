package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type updateSourceRace struct {
	mu      sync.Mutex
	first   string
	sources []string
	results map[string]updateSourceProbe
	cancel  context.CancelFunc
}

func (r *updateSourceRace) resultsReady() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.results[r.first].speed > 0
}
func (r *updateSourceRace) ordered() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := append([]string(nil), r.sources...)
	sort.SliceStable(out, func(i, j int) bool { return r.results[out[i]].speed > r.results[out[j]].speed })
	firstSpeed := r.results[r.first].speed
	if firstSpeed == 0 || len(out) > 0 && r.results[out[0]].speed < 2*firstSpeed {
		return updatePreferredSources(out, r.first)
	}
	return out
}
func (u *updateManager) startUpdateSourceRace(parent context.Context, asset updateAsset, sources []string) *updateSourceRace {
	ctx, cancel := context.WithCancel(parent)
	r := &updateSourceRace{sources: append([]string(nil), sources...), results: map[string]updateSourceProbe{}, cancel: cancel}
	first := make(chan string, len(sources))
	done := make(chan struct{}, len(sources))
	started := time.Now()
	for _, prefix := range sources {
		prefix := prefix
		goSafe("update-first-byte", func() {
			defer func() { done <- struct{}{} }()
			probeCtx, stop := context.WithTimeout(ctx, updateProbeTimeout)
			defer stop()
			req, err := http.NewRequestWithContext(probeCtx, "GET", prefix+asset.URL, nil)
			if err != nil {
				return
			}
			req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", min(asset.Size, updateProbeBytes)-1))
			req.Header.Set("Accept-Encoding", "identity")
			resp, err := u.client.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 && resp.StatusCode != 206 {
				return
			}
			buf := make([]byte, 32<<10)
			n, err := resp.Body.Read(buf)
			if n <= 0 {
				return
			}
			first <- prefix
			count := int64(n)
			if err == nil {
				remaining := min(asset.Size, updateProbeBytes) - count
				more, _ := io.Copy(io.Discard, io.LimitReader(resp.Body, max(int64(0), remaining)))
				count += more
			}
			duration := time.Since(started)
			probe := updateSourceProbe{prefix: prefix, ok: true, speed: float64(count) / max(.001, duration.Seconds())}
			r.mu.Lock()
			r.results[prefix] = probe
			r.mu.Unlock()
			u.recordUpdateCheck(map[string]any{"event": "update_download_probe", "mirror_prefix": updateDiagnosticMirrorPrefix(prefix), "bytes": count, "duration_ms": duration.Milliseconds(), "speed_kbps": probe.speed / 1024, "ok": true})
		})
	}
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	completed := 0
	for {
		select {
		case prefix := <-first:
			r.first = prefix
			u.recordUpdateCheck(map[string]any{"event": "update_download_first_byte", "first_byte_ms": time.Since(started).Milliseconds(), "predownload": u.predownloadActive(), "mirror_prefix": updateDiagnosticMirrorPrefix(prefix)})
			return r
		case <-done:
			completed++
			if completed == len(sources) {
				if len(sources) > 0 {
					r.first = sources[0]
				}
				return r
			}
		case <-timer.C:
			if len(sources) > 0 {
				r.first = sources[0]
			}
			return r
		case <-parent.Done():
			return r
		}
	}
}
func (u *updateManager) predownloadActive() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.predownloading
}
func (u *updateManager) yieldPredownload(ctx context.Context) error {
	if !u.predownloadActive() || u.foregroundBusy == nil || !u.foregroundBusy() {
		return nil
	}
	return waitContext(ctx, 100*time.Millisecond)
}

// Each worker owns disjoint offsets. A failed segment resumes at its last verified byte on another route.
func (u *updateManager) downloadSegments(ctx context.Context, asset updateAsset, part string, race *updateSourceRace) error {
	file, err := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err = file.Truncate(asset.Size); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var received atomic.Int64
	var switches atomic.Int64
	var wg sync.WaitGroup
	fail := make(chan error, 4)
	started := time.Now()
	piece := (asset.Size + 3) / 4
	for index := 0; index < 4; index++ {
		start := int64(index) * piece
		end := min(asset.Size-1, start+piece-1)
		if start > end {
			continue
		}
		wg.Add(1)
		goSafe("update-range-segment", func() {
			defer wg.Done()
			position := start
			sourceIndex := 0
			previous := ""
			for position <= end {
				if ctx.Err() != nil {
					return
				}
				sources := race.ordered()
				if len(sources) == 0 {
					fail <- errors.New("no download source")
					cancel()
					return
				}
				routeIndex := sourceIndex % len(sources)
				if sourceIndex == 0 && len(sources) > 1 && race.resultsReady() {
					routeIndex = index % min(2, len(sources))
				}
				prefix := sources[routeIndex]
				if sourceIndex > 0 && prefix == previous && len(sources) > 1 {
					prefix = sources[(routeIndex+1)%len(sources)]
				}
				if previous != "" && previous != prefix {
					switches.Add(1)
				}
				previous = prefix
				err := u.downloadSegment(ctx, file, asset, prefix, position, min(end, position+(4<<20)-1), func(n int64) {
					position += n
					total := received.Add(n)
					u.mu.Lock()
					elapsed := time.Since(started).Seconds()
					speed := float64(total) / max(.001, elapsed)
					u.status.Progress = updateProgress{ReceivedBytes: total, TotalBytes: asset.Size, BytesPerSecond: speed, ETASeconds: int64(float64(asset.Size-total) / max(1, speed))}
					u.mu.Unlock()
				})
				if err == nil {
					u.recordUpdateCheck(map[string]any{"event": "update_download_segment", "segment": index, "avg_kbps": float64(position-start) / max(.001, time.Since(started).Seconds()) / 1024, "switches": switches.Load(), "predownload": u.predownloadActive()})
					if position <= end {
						sourceIndex = 0
						continue
					}
					return
				}
				sourceIndex++
				if sourceIndex >= len(sources)*2 {
					fail <- err
					cancel()
					return
				}
			}
		})
	}
	ticks := time.NewTicker(250 * time.Millisecond)
	defer ticks.Stop()
	done := make(chan struct{})
	goSafe("update-segment-join", func() { wg.Wait(); close(done) })
	for {
		select {
		case <-ticks.C:
			u.publish()
		case <-done:
			select {
			case err := <-fail:
				return err
			default:
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			u.recordUpdateCheck(map[string]any{"event": "update_download_segments_finished", "segments": 4, "switches": switches.Load(), "predownload": u.predownloadActive()})
			return verifyUpdateFile(ctx, part, asset)
		}
	}
}
func (u *updateManager) downloadSegment(parent context.Context, file *os.File, asset updateAsset, prefix string, start, end int64, progress func(int64)) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	idle := time.AfterFunc(8*time.Second, cancel)
	defer idle.Stop()
	req, err := http.NewRequestWithContext(ctx, "GET", prefix+asset.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := u.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	expected := fmt.Sprintf("bytes %d-%d/%d", start, end, asset.Size)
	if resp.StatusCode != 206 || resp.Header.Get("Content-Range") != expected || resp.ContentLength >= 0 && resp.ContentLength != end-start+1 {
		return errors.New("range unavailable")
	}
	position := start
	buf := make([]byte, 64<<10)
	for position <= end {
		if err = u.yieldPredownload(ctx); err != nil {
			return err
		}
		n, readErr := resp.Body.Read(buf[:min(int64(len(buf)), end-position+1)])
		if n > 0 {
			written, e := file.WriteAt(buf[:n], position)
			if e != nil {
				return e
			}
			position += int64(written)
			progress(int64(written))
			idle.Reset(8 * time.Second)
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) && position > end {
				return nil
			}
			return readErr
		}
		if n == 0 {
			return io.ErrNoProgress
		}
	}
	return nil
}
func (u *updateManager) downloadAssetQuick(ctx context.Context, asset updateAsset, mirrors []string) error {
	race := u.startUpdateSourceRace(ctx, asset, mirrors)
	defer race.cancel()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	u.mu.Lock()
	u.status.Progress.SelectingSource = false
	u.mu.Unlock()
	u.publish()
	part := filepath.Join(u.directory, asset.Name+".part")
	started := time.Now()
	if asset.Size >= 4<<20 {
		if err := u.downloadSegments(ctx, asset, part, race); err == nil {
			if err = os.Rename(part, filepath.Join(u.directory, asset.Name)); err == nil {
				u.rememberVerifiedUpdateFile(filepath.Join(u.directory, asset.Name), asset)
				u.rememberUpdateSource(race.first)
				return nil
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_ = os.Remove(part)
		u.recordUpdateCheck(map[string]any{"event": "update_download_segment_fallback", "segments": 4, "predownload": u.predownloadActive()})
	}
	sources := updatePreferredSources(race.ordered(), race.first)
	var last error
	for attempt := 0; attempt < 2; attempt++ {
		for _, prefix := range sources {
			transfer := updateSourceTransfer{}
			last = u.downloadSource(ctx, asset, prefix+asset.URL, part, len(sources) > 1, &transfer)
			if last == nil {
				if last = os.Rename(part, filepath.Join(u.directory, asset.Name)); last == nil {
					u.rememberVerifiedUpdateFile(filepath.Join(u.directory, asset.Name), asset)
					u.rememberUpdateSource(prefix)
					u.recordUpdateCheck(map[string]any{"event": "update_download_finished", "duration_ms": time.Since(started).Milliseconds(), "segments": 1, "predownload": u.predownloadActive()})
					return nil
				}
			}
			if errors.Is(last, errUpdateChecksum) {
				_ = os.Remove(part)
				break
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
		}
	}
	if last == nil {
		last = errors.New("no download source")
	}
	return last
}

type updatePredownloadReader struct {
	manager *updateManager
	ctx     context.Context
	body    io.Reader
}

func (r updatePredownloadReader) Read(p []byte) (int, error) {
	if err := r.manager.yieldPredownload(r.ctx); err != nil {
		return 0, err
	}
	return r.body.Read(p[:min(len(p), 64<<10)])
}
