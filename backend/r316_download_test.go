package main

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Real Range servers exercise cancellation while a body is still in flight.
func r316Source(t *testing.T, data []byte, mode string) *httptest.Server {
	t.Helper()
	var sent atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var start, end int64
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
			t.Errorf("range: %v", err)
			return
		}
		if mode == "503" {
			w.WriteHeader(503)
			return
		}
		if start < 0 || end >= int64(len(data)) || start > end {
			w.WriteHeader(416)
			return
		}
		cr := fmt.Sprintf("bytes %d-%d/%d", start, end, len(data))
		if mode == "range" {
			cr = "bytes 1-2/3"
		}
		w.Header().Set("Content-Range", cr)
		w.Header().Set("Content-Length", fmt.Sprint(end-start+1))
		w.WriteHeader(206)
		for at := start; at <= end; {
			size := min(int64(16<<10), end-at+1)
			rate := float64(6 << 20)
			if mode == "slow" && sent.Load() >= 2<<20 {
				rate = 120 << 10
			}
			if mode == "equal" {
				rate = 200 << 10
			}
			if mode == "hang" && sent.Load() >= 64<<10 {
				<-r.Context().Done()
				return
			}
			timer := time.NewTimer(time.Duration(float64(time.Second) * float64(size) / rate))
			select {
			case <-r.Context().Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			if _, err := w.Write(data[at : at+size]); err != nil {
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			sent.Add(size)
			at += size
		}
	}))
	t.Cleanup(server.Close)
	return server
}
func TestR316SharedQueueEscapesTricklingSource(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprint(reverse), func(t *testing.T) {
			data := bytes.Repeat([]byte{37}, 24<<20)
			a, b := r316Source(t, data, "fast"), r316Source(t, data, "slow")
			sources := []string{a.URL + "/", b.URL + "/"}
			if reverse {
				sources[0], sources[1] = sources[1], sources[0]
			}
			u := &updateManager{client: a.Client()}
			race := &updateSourceRace{first: sources[0], sources: sources, results: map[string]updateSourceProbe{sources[0]: {speed: 6 << 20}, sources[1]: {speed: 6 << 20}}}
			limit := 7 * time.Second
			ctx, cancel := context.WithTimeout(context.Background(), limit)
			defer cancel()
			started := time.Now()
			err := u.downloadSegments(ctx, r278Asset(data), filepath.Join(t.TempDir(), "asset.part"), race)
			if err != nil {
				t.Fatalf("slow source held shared work beyond %s: %v", limit, err)
			}
			if elapsed := time.Since(started); elapsed > limit {
				t.Fatalf("elapsed %s > %s", elapsed, limit)
			}
			if got := u.Status().Progress.ReceivedBytes; got != int64(len(data)) {
				t.Fatalf("unique received=%d want %d", got, len(data))
			}
			t.Logf("24MiB reverse=%v elapsed=%s", reverse, time.Since(started))
		})
	}
}
func TestR316BrokenSourcesAndSingleSource(t *testing.T) {
	for _, mode := range []string{"hang", "503", "range", "single", "equal"} {
		t.Run(mode, func(t *testing.T) {
			data := bytes.Repeat([]byte{61}, 4<<20)
			a := r316Source(t, data, "fast")
			sources := []string{a.URL + "/"}
			if mode == "equal" {
				a = r316Source(t, data, "equal")
				sources = []string{a.URL + "/"}
			}
			if mode != "single" {
				b := r316Source(t, data, mode)
				sources = append(sources, b.URL+"/")
			}
			var mu sync.Mutex
			var last map[string]any
			var stolen bool
			u := &updateManager{client: a.Client(), diagnostic: func(e map[string]any) {
				mu.Lock()
				defer mu.Unlock()
				if e["event"] == "update_download_segments_finished" {
					last = e
				}
				if e["event"] == "update_download_chunk_stolen" {
					stolen = true
				}
			}}
			race := &updateSourceRace{first: sources[0], sources: sources, results: map[string]updateSourceProbe{}}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			part := filepath.Join(t.TempDir(), "asset.part")
			started := time.Now()
			if err := u.downloadSegments(ctx, r278Asset(data), part, race); err != nil {
				t.Fatal(err)
			}
			if err := verifyUpdateFile(ctx, part, r278Asset(data)); err != nil {
				t.Fatal(err)
			}
			if got := u.Status().Progress.ReceivedBytes; got != int64(len(data)) {
				t.Fatalf("count %d", got)
			}
			mu.Lock()
			defer mu.Unlock()
			if last == nil || last["per_source"] == nil {
				t.Fatal("missing source accounting")
			}
			if mode == "equal" && last["aborted_slow_chunks"] != 0 {
				t.Fatalf("equal sources churn: %v", last)
			}
			if mode == "hang" && (!stolen || time.Since(started) > 3*time.Second) {
				t.Fatalf("hung tail not rescued promptly: stolen=%v elapsed=%s", stolen, time.Since(started))
			}
			if mode == "equal" && time.Since(started) > 16*time.Second {
				t.Fatalf("equal slow sources exceeded aggregate throughput budget: %s", time.Since(started))
			}
			t.Logf("mode=%s elapsed=%s steals=%v aborted=%v", mode, time.Since(started), last["steals"], last["aborted_slow_chunks"])
		})
	}
}
func TestR316FiveSecondSpeedAndETA(t *testing.T) {
	start := time.Unix(0, 0)
	w := updateSpeedWindow{}
	var total int64
	w.speed(start, 0)
	for second := 1; second <= 30; second++ {
		rate := int64(3 << 20)
		if second > 10 {
			rate = 50 << 10
		}
		total += rate
		speed := w.speed(start.Add(time.Duration(second)*time.Second), total)
		if second == 30 {
			if math.Abs(speed/(50<<10)-1) > .3 {
				t.Fatalf("speed %f", speed)
			}
		}
	}
}

func TestR316MonitorDemotesSustainedSlowButDoesNotChurnEqualSources(t *testing.T) {
	for _, bestSpeed := range []float64{6 << 20, 200 << 10} {
		begin := time.Unix(100, 0)
		u := &updateManager{}
		slow := &updateRoute{prefix: "https://slow.test/", speed: 120 << 10}
		fast := &updateRoute{prefix: "https://fast.test/", speed: bestSpeed}
		c := &updateChunk{end: 100 << 20, flights: map[*updateFlight]bool{}}
		var cancelled atomic.Bool
		f := &updateFlight{chunk: c, source: slow, started: begin, cancel: func() { cancelled.Store(true) }}
		f.window.speed(begin, 0)
		c.flights[f] = true
		s := &updateScheduler{manager: u, asset: updateAsset{Size: 100 << 20}, started: begin, chunks: []*updateChunk{c}, routes: []*updateRoute{fast, slow}}
		s.progress.speed(begin, 0)
		for second := 1; second <= 5; second++ {
			f.bytes += 120 << 10
			s.received += 120 << 10
			s.tick(begin.Add(time.Duration(second) * time.Second))
			if second < 4 && cancelled.Load() {
				t.Fatal("switched before three consecutive slow seconds")
			}
		}
		if bestSpeed > 300<<10 {
			if !cancelled.Load() || s.aborted != 1 || slow.demotedUntil.IsZero() {
				t.Fatalf("slow source not demoted %+v", s)
			}
		} else if cancelled.Load() || s.aborted != 0 {
			t.Fatal("similar slow sources churned")
		}
	}
}
func TestR316ProgressIsFiveSecondRateWithFreshETA(t *testing.T) {
	begin := time.Unix(100, 0)
	u := &updateManager{}
	s := &updateScheduler{manager: u, asset: updateAsset{Size: 100 << 20}, started: begin}
	s.progress.speed(begin, 0)
	for second := 1; second <= 30; second++ {
		rate := int64(3 << 20)
		if second > 10 {
			rate = 50 << 10
		}
		s.received += rate
		s.tick(begin.Add(time.Duration(second) * time.Second))
	}
	p := u.Status().Progress
	want := float64(s.asset.Size-s.received) / (50 << 10)
	if math.Abs(p.BytesPerSecond/(50<<10)-1) > .3 || math.Abs(float64(p.ETASeconds)/want-1) > .3 {
		t.Fatalf("stale progress %+v, ETA want %f", p, want)
	}
	s.tick(begin.Add(40 * time.Second))
	if p := u.Status().Progress; p.BytesPerSecond >= 1024 || p.ETASeconds != 0 {
		t.Fatalf("stalled ETA must calculate %+v", p)
	}
}
