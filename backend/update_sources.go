package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const updateProbeBytes int64 = 1048576
const updateProbeTimeout = 6 * time.Second
const updateSlowSpeed = 150 * 1024

var errUpdateSourceSlow = errors.New("下载线路速度过低")

type updateSourceProbe struct {
	prefix string
	speed  float64
	ok     bool
}
type updateSourceTransfer struct {
	offset   int64
	received int64
	speed    float64
}

func updateSourcePresent(sources []string, prefix string) bool {
	for _, source := range sources {
		if source == prefix {
			return true
		}
	}
	return false
}
func updatePreferredSources(sources []string, preferred string) []string {
	result := make([]string, 0, len(sources))
	if updateSourcePresent(sources, preferred) {
		result = append(result, preferred)
	}
	for _, source := range sources {
		if !updateSourcePresent(result, source) {
			result = append(result, source)
		}
	}
	return result
}
func (u *updateManager) defaultUpdateSources() []string {
	if u.sourceDefaults != nil {
		return u.sourceDefaults
	}
	return updateMirrors
}

// Built-in routes stay available when the settings add custom prefixes.
// Call under mu (or during construction); return a detached, deduplicated list.
func (u *updateManager) sourcesLocked() []string {
	return updatePreferredSources(append(append([]string(nil), u.mirrors...), u.defaultUpdateSources()...), u.preferredSource)
}
func (u *updateManager) rememberUpdateSource(prefix string) {
	u.mu.Lock()
	if !updateSourcePresent(u.sourcesLocked(), prefix) {
		u.mu.Unlock()
		return
	}
	u.preferredSource = prefix
	data, _ := json.Marshal(updateSettings{Mirrors: append([]string(nil), u.mirrors...), PreferredSource: prefix})
	err := writeLocalStoreFile(u.store, "update-settings.json", data)
	u.mu.Unlock()
	if err != nil {
		u.recordUpdateCheck(map[string]any{"event": "update_source_preference_failed", "error_kind": diagnosticErrorKind(err)})
	}
}
func (u *updateManager) downloadTime() time.Time {
	if u.downloadClock != nil {
		return u.downloadClock()
	}
	return time.Now()
}
func updateProxyEnvSet(name string) bool {
	return os.Getenv(name) != "" || os.Getenv(strings.ToLower(name)) != ""
}

// Probe bytes never enter the partial file. The first usable result starts a
// one-second comparison window; cancel unfinished requests instead of waiting
// for the slowest route. Only the receiver owns the result slice.
func (u *updateManager) probeUpdateSources(parent context.Context, asset updateAsset, sources []string) []updateSourceProbe {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	type completed struct {
		index int
		probe updateSourceProbe
	}
	done := make(chan completed, len(sources))
	for i, prefix := range sources {
		goSafe("update-download-probe", func() { done <- completed{i, u.probeUpdateSource(ctx, asset, prefix)} })
	}
	results := make([]completed, 0, len(sources))
	seen := make([]bool, len(sources))
	var timer *time.Timer
	var deadline <-chan time.Time
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
collect:
	for received := 0; received < len(sources); received++ {
		select {
		case result := <-done:
			seen[result.index] = true
			if result.probe.ok {
				results = append(results, result)
				if timer == nil {
					timer = time.NewTimer(time.Second)
					deadline = timer.C
				}
			}
		case <-deadline:
			break collect
		case <-parent.Done():
			return nil
		}
	}
	cancel()
	sort.Slice(results, func(i, j int) bool {
		if results[i].probe.speed == results[j].probe.speed {
			return results[i].index < results[j].index
		}
		return results[i].probe.speed > results[j].probe.speed
	})
	usable := make([]updateSourceProbe, 0, len(results))
	for _, result := range results {
		usable = append(usable, result.probe)
	}
	// Canceled probes are unmeasured, not failed routes. Preserve them after
	// the measured winners for download-error fallback and slow-source switching.
	for i, prefix := range sources {
		if !seen[i] {
			usable = append(usable, updateSourceProbe{prefix: prefix})
		}
	}
	return usable
}
func (u *updateManager) probeUpdateSource(parent context.Context, asset updateAsset, prefix string) updateSourceProbe {
	start := time.Now()
	ctx, cancel := context.WithTimeout(parent, updateProbeTimeout)
	defer cancel()
	result := updateSourceProbe{prefix: prefix}
	status := 0
	var count int64
	var failure error
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, prefix+asset.URL, nil)
	if err == nil {
		req.Header.Set("Range", "bytes=0-1048575")
		req.Header.Set("Accept-Encoding", "identity")
		var response *http.Response
		response, err = u.client.Do(req)
		if err == nil {
			status = response.StatusCode
			total, parseErr := strconv.ParseInt(response.Header.Get("Content-Range")[strings.LastIndex(response.Header.Get("Content-Range"), "/")+1:], 10, 64)
			valid := status == http.StatusPartialContent && strings.HasPrefix(response.Header.Get("Content-Range"), "bytes 0-") && parseErr == nil && total == asset.Size || status == http.StatusOK && response.ContentLength == asset.Size
			if valid {
				count, err = io.Copy(io.Discard, io.LimitReader(updateContextReader{ctx, response.Body}, min(updateProbeBytes, asset.Size)))
			} else {
				err = errors.New("invalid-probe-response")
			}
			response.Body.Close()
			result.ok = valid && err == nil && ctx.Err() == nil && count == min(updateProbeBytes, asset.Size)
			if !result.ok && err == nil {
				err = errors.New("incomplete-probe-response")
			}
		}
	}
	failure = err
	duration := time.Since(start)
	result.speed = float64(count) / duration.Seconds()
	kind := ""
	if !result.ok {
		kind = diagnosticErrorKind(failure)
		if ctx.Err() != nil {
			kind = diagnosticErrorKind(ctx.Err())
		}
	}
	u.recordUpdateCheck(map[string]any{"event": "update_download_probe", "mirror_prefix": updateDiagnosticMirrorPrefix(prefix), "http_status": status, "bytes": count, "duration_ms": duration.Milliseconds(), "speed_kbps": result.speed / 1024, "ok": result.ok, "error_kind": kind})
	return result
}
