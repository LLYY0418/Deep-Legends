package main

import (
	"context"
	"errors"
	"math"
	"os"
	"sync"
	"time"
)

const updateChunkBytes int64 = 2 << 20

type updateChunk struct {
	index         int
	position, end int64 // high water mark shared by all copies; never counts a byte twice
	done, stolen  bool
	attempts      int
	flights       map[*updateFlight]bool
}
type updateFlight struct {
	chunk           *updateChunk
	source          *updateRoute
	position, bytes int64
	started         time.Time
	window          updateSpeedWindow
	speed           float64
	slowSince       time.Time
	cancel          context.CancelFunc
	slow            bool
}
type updateRoute struct {
	prefix                  string
	initial, speed          float64
	demotedUntil, lastProbe time.Time
	bytes                   int64
	connectionTime          time.Duration
	connections, active     int
}
type updateScheduler struct {
	mu                                   sync.Mutex
	manager                              *updateManager
	race                                 *updateSourceRace
	asset                                updateAsset
	file                                 *os.File
	chunks                               []*updateChunk
	routes                               []*updateRoute
	received                             int64
	finished                             int
	started                              time.Time
	progress                             updateSpeedWindow
	demotions, steals, aborted, switches int
	failure                              error
}

// All shared accounting and route choices are protected by mu. Each request
// writes a contiguous Range; duplicate tail requests only advance its high water.
func (s *updateScheduler) best(now time.Time, except *updateRoute) *updateRoute {
	var best *updateRoute
	var score float64
	for _, r := range s.routes {
		if r == except || now.Before(r.demotedUntil) {
			continue
		}
		speed := r.speed
		if speed <= 0 {
			speed = r.initial
		}
		if speed <= 0 {
			speed = 1 << 20
		}
		candidate := speed
		if best == nil || candidate > score {
			best, score = r, candidate
		}
	}
	if best == nil {
		for _, r := range s.routes {
			if r != except {
				return r
			}
		}
	}
	return best
}
func (s *updateScheduler) claim(ctx context.Context) (*updateFlight, context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if s.finished == len(s.chunks) || s.failure != nil || ctx.Err() != nil {
		return nil, nil
	}
	var chunk *updateChunk
	for _, c := range s.chunks {
		if !c.done && len(c.flights) == 0 {
			chunk = c
			break
		}
	}
	if s.race != nil {
		s.race.mu.Lock()
		for _, r := range s.routes {
			if measured := s.race.results[r.prefix].speed; measured > 0 {
				r.initial = measured
			}
		}
		s.race.mu.Unlock()
	}
	source := s.best(now, nil)
	if source == nil {
		s.failure = errors.New("no download source")
		return nil, nil
	}
	if chunk == nil {
		// Spare capacity races the slowest remaining tail on the fastest route.
		var slowest *updateFlight
		for _, c := range s.chunks {
			if c.done || c.stolen || len(c.flights) != 1 {
				continue
			}
			for f := range c.flights {
				if now.Sub(f.started) < 250*time.Millisecond {
					continue
				}
				if slowest == nil || f.speed < slowest.speed {
					slowest = f
				}
			}
		}
		if slowest == nil {
			return nil, nil
		}
		chunk = slowest.chunk
		if source == slowest.source && source.speed == 0 {
			if alternate := s.best(now, slowest.source); alternate != nil && !now.Before(alternate.demotedUntil) && alternate.initial >= source.initial {
				source = alternate
			}
		}
		chunk.stolen = true
		s.steals++
		s.manager.recordUpdateCheck(map[string]any{"event": "update_download_chunk_stolen", "segment": chunk.index, "source": updateDiagnosticMirrorPrefix(source.prefix), "remaining_bytes": chunk.end - chunk.position + 1})
	} else {
		// Give each unmeasured route one connection, then use measured speed. A
		// degraded route gets at most one small reevaluation every twenty seconds.
		for _, r := range s.routes {
			if r.connections == 0 {
				source = r
				break
			}
		}
		for _, r := range s.routes {
			if !r.demotedUntil.IsZero() && now.Sub(r.lastProbe) >= 20*time.Second && r.active == 0 {
				source = r
				r.lastProbe = now
				break
			}
		}
	}
	if chunk.attempts >= max(4, len(s.routes)*2) && len(chunk.flights) == 0 {
		s.failure = errors.New("download chunk retry budget exhausted")
		return nil, nil
	}
	chunk.attempts++
	source.connections++
	source.active++
	attemptCtx, cancel := context.WithCancel(ctx)
	f := &updateFlight{chunk: chunk, source: source, position: chunk.position, started: now, cancel: cancel}
	f.window.speed(now, 0)
	chunk.flights[f] = true
	return f, attemptCtx
}
func (s *updateScheduler) add(f *updateFlight, n int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f.bytes += n
	f.position += n
	f.source.bytes += n
	if f.position > f.chunk.position {
		s.received += f.position - f.chunk.position
		f.chunk.position = f.position
	}
}
func (s *updateScheduler) finish(f *updateFlight, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	route, c := f.source, f.chunk
	route.active--
	route.connectionTime += now.Sub(f.started)
	delete(c.flights, f)
	if !c.done && f.position > c.end {
		c.done = true
		s.finished++
		for other := range c.flights {
			other.cancel()
		}
	}
	if err == nil && now.Sub(f.started) > 0 {
		speed := float64(f.bytes) / now.Sub(f.started).Seconds()
		if route.speed == 0 {
			route.speed = speed
		} else {
			route.speed = .5*route.speed + .5*speed
		}
		if !route.demotedUntil.IsZero() {
			best := s.best(now, route)
			bestSpeed := route.speed
			if best != nil {
				bestSpeed = best.speed
			}
			if route.speed >= 300<<10 || route.speed >= bestSpeed*.5 {
				route.demotedUntil = time.Time{}
				s.manager.recordUpdateCheck(map[string]any{"event": "update_download_source_restored", "source": updateDiagnosticMirrorPrefix(route.prefix), "reason": "reevaluated", "kbps": route.speed / 1024, "best_kbps": bestSpeed / 1024})
			}
		}
	}
	if err != nil && !f.slow && !c.done && ctxErrorFree(err) {
		// Errors quarantine the route too; a different route resumes verified bytes.
		route.demotedUntil = now.Add(30 * time.Second)
		route.lastProbe = now
		s.switches++
	}
	s.manager.recordUpdateCheck(map[string]any{"event": "update_download_segment", "segment": c.index, "source": updateDiagnosticMirrorPrefix(route.prefix), "chunk_bytes": f.bytes, "chunk_ms": now.Sub(f.started).Milliseconds(), "chunk_kbps": float64(f.bytes) / math.Max(.001, now.Sub(f.started).Seconds()) / 1024, "switches": s.switches, "predownload": s.manager.predownloadActive()})
}
func ctxErrorFree(err error) bool { return !errors.Is(err, context.Canceled) }
func (s *updateScheduler) tick(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Update every active connection's five-second rate, including idle periods.
	for _, c := range s.chunks {
		for f := range c.flights {
			f.speed = f.window.speed(now, f.bytes)
		}
	}
	for _, r := range s.routes {
		var total float64
		var count int
		for _, c := range s.chunks {
			for f := range c.flights {
				if f.source == r && now.Sub(f.started) >= 250*time.Millisecond {
					total += f.speed
					count++
				}
			}
		}
		if count > 0 {
			r.speed = total / float64(count)
		}
	}
	for _, c := range s.chunks {
		if c.done {
			continue
		}
		for f := range c.flights {
			best := s.best(now, f.source)
			if best == nil || f.slow {
				continue
			}
			bestSpeed := best.speed
			if bestSpeed <= 0 {
				bestSpeed = best.initial
			}
			if now.Sub(f.started) >= time.Second && f.speed < 300<<10 && f.speed < bestSpeed*.25 {
				if f.slowSince.IsZero() {
					f.slowSince = now
				}
				if now.Sub(f.slowSince) >= 3*time.Second {
					f.slow = true
					f.source.demotedUntil = now.Add(30 * time.Second)
					f.source.lastProbe = now
					s.demotions++
					s.aborted++
					s.switches++
					s.manager.recordUpdateCheck(map[string]any{"event": "update_download_source_demoted", "source": updateDiagnosticMirrorPrefix(f.source.prefix), "reason": "sustained-low-throughput", "kbps": f.speed / 1024, "best_kbps": bestSpeed / 1024})
					f.cancel()
				}
			} else {
				f.slowSince = time.Time{}
			}
		}
	}
	speed := s.progress.speed(now, s.received)
	eta := int64(0)
	if speed >= 1024 && now.Sub(s.started) >= time.Second {
		eta = int64(math.Ceil(float64(s.asset.Size-s.received) / speed))
	}
	s.manager.mu.Lock()
	s.manager.status.Progress = updateProgress{ReceivedBytes: s.received, TotalBytes: s.asset.Size, BytesPerSecond: speed, ETASeconds: eta}
	s.manager.mu.Unlock()
}
func (u *updateManager) downloadSharedChunks(parent context.Context, asset updateAsset, part string, race *updateSourceRace) error {
	file, err := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err = file.Truncate(asset.Size); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	s := &updateScheduler{manager: u, race: race, asset: asset, file: file, started: time.Now()}
	s.progress.speed(s.started, 0)
	for start := int64(0); start < asset.Size; start += updateChunkBytes {
		s.chunks = append(s.chunks, &updateChunk{index: len(s.chunks), position: start, end: min(asset.Size-1, start+updateChunkBytes-1), flights: map[*updateFlight]bool{}})
	}
	race.mu.Lock()
	for _, prefix := range race.sources {
		s.routes = append(s.routes, &updateRoute{prefix: prefix, initial: race.results[prefix].speed})
	}
	race.mu.Unlock()
	var workers sync.WaitGroup
	for i := 0; i < 4; i++ {
		workers.Add(1)
		goSafe("update-shared-chunk", func() {
			defer workers.Done()
			for ctx.Err() == nil {
				f, attemptCtx := s.claim(ctx)
				if f == nil {
					s.mu.Lock()
					done := s.finished == len(s.chunks) || s.failure != nil
					s.mu.Unlock()
					if done {
						return
					}
					if waitContext(ctx, 50*time.Millisecond) != nil {
						return
					}
					continue
				}
				// Demoted probes are only 512 KiB; leave the remainder in the shared queue.
				end := f.chunk.end
				s.mu.Lock()
				if !f.source.demotedUntil.IsZero() {
					end = min(end, f.position+(512<<10)-1)
				}
				s.mu.Unlock()
				err := u.downloadSegment(attemptCtx, file, asset, f.source.prefix, f.position, end, func(n int64) { s.add(f, n) })
				f.cancel()
				s.finish(f, err)
			}
		})
	}
	done := make(chan struct{})
	goSafe("update-shared-join", func() { workers.Wait(); close(done) })
	ticks := time.NewTicker(250 * time.Millisecond)
	defer ticks.Stop()
	for {
		select {
		case <-ticks.C:
			s.tick(time.Now())
			u.publish()
		case <-done:
			if parent.Err() != nil {
				return parent.Err()
			}
			s.tick(time.Now())
			if s.failure != nil {
				return s.failure
			}
			perSource := make([]map[string]any, 0, len(s.routes))
			for _, r := range s.routes {
				perSource = append(perSource, map[string]any{"source": updateDiagnosticMirrorPrefix(r.prefix), "bytes": r.bytes, "avg_kbps": float64(r.bytes) / math.Max(.001, r.connectionTime.Seconds()) / 1024, "connections": r.connections})
			}
			elapsed := time.Since(s.started)
			u.recordUpdateCheck(map[string]any{"event": "update_download_segments_finished", "segments": 4, "chunks": len(s.chunks), "switches": s.switches, "total_ms": elapsed.Milliseconds(), "avg_kbps": float64(s.received) / math.Max(.001, elapsed.Seconds()) / 1024, "per_source": perSource, "demotions": s.demotions, "steals": s.steals, "aborted_slow_chunks": s.aborted, "predownload": u.predownloadActive()})
			if err = file.Sync(); err != nil {
				return err
			}
			return verifyUpdateFile(parent, part, asset)
		}
	}
}
