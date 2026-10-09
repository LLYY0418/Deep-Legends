package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type diagnosticExportPart struct {
	file    *os.File
	size    int64
	newline bool
}
type diagnosticExportSnapshot struct {
	parts []diagnosticExportPart
	size  int64
}

func (s *diagnosticExportSnapshot) close() {
	for _, part := range s.parts {
		_ = part.file.Close()
	}
}
func (s *diagnosticExportSnapshot) writeTo(w io.Writer) (int64, error) {
	var written int64
	for _, part := range s.parts {
		n, err := io.Copy(w, io.NewSectionReader(part.file, 0, part.size))
		written += n
		if err != nil {
			return written, err
		}
		if n != part.size {
			return written, io.ErrUnexpectedEOF
		}
		if part.newline {
			n, err := w.Write([]byte{'\n'})
			written += int64(n)
			if err != nil {
				return written, err
			}
			if n != 1 {
				return written, io.ErrShortWrite
			}
		}
	}
	return written, nil
}

// Open bounded snapshots under the rotation lock, then stream exact lengths
// from stable handles. Newest records are sent first even on interrupted links.
func (s *localStore) snapshotDiagnosticExport() (snapshot *diagnosticExportSnapshot, err error) {
	if s == nil {
		return nil, errors.New("local storage unavailable")
	}
	s.diagnosticMu.Lock()
	defer s.diagnosticMu.Unlock()
	if err = s.flushDiagnosticLocked(); err != nil {
		return nil, err
	}
	snapshot = &diagnosticExportSnapshot{}
	defer func() {
		if err != nil {
			snapshot.close()
		}
	}()
	for _, name := range []string{"diagnostics.jsonl", "diagnostics.1.jsonl", "diagnostics.2.jsonl", "diagnostics.3.jsonl", "diagnostics.4.jsonl"} {
		path := filepath.Join(s.root, "logs", name)
		info, statErr := os.Lstat(path)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil {
			return snapshot, statErr
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return snapshot, errors.New("diagnostic log is not a trusted regular file")
		}
		if info.Size() > 2*1024*1024+64*1024 {
			return snapshot, errResponseLimitExceeded
		}
		file, openErr := os.Open(path)
		if openErr != nil {
			return snapshot, openErr
		}
		opened, openErr := file.Stat()
		if openErr != nil || !os.SameFile(info, opened) {
			file.Close()
			return snapshot, errors.New("diagnostic log changed")
		}
		part := diagnosticExportPart{file: file, size: info.Size()}
		snapshot.parts = append(snapshot.parts, part)
		if part.size > 0 {
			var last [1]byte
			if _, err = file.ReadAt(last[:], part.size-1); err != nil {
				return snapshot, err
			}
			part.newline = last[0] != '\n'
			snapshot.parts[len(snapshot.parts)-1] = part
		}
		snapshot.size += part.size
		if part.newline {
			snapshot.size++
		}
	}
	return snapshot, nil
}

// A probe with a broken cancellation path must not hold the download hostage.
func (a *app) collectDiagnosticExportProbes(parent context.Context, client *LCUClient) map[string]int64 {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	var mu sync.Mutex
	timings := map[string]int64{}
	done := make(chan struct{})
	probes := []struct {
		name string
		run  func(context.Context)
	}{
		{"item_sets", a.recordItemSetExportSnapshot},
		{"objective", func(ctx context.Context) { a.collectObjectiveDiagnostics(ctx, client, "export") }},
		{"accept_focus", func(ctx context.Context) { a.collectAcceptFocusInspection(ctx, client) }},
	}
	a.goSafe("diagnostic-export-probes", func() {
		defer close(done)
		for _, probe := range probes {
			if ctx.Err() != nil {
				return
			}
			started := time.Now()
			probe.run(ctx)
			mu.Lock()
			timings[probe.name] = time.Since(started).Milliseconds()
			mu.Unlock()
		}
	})
	select {
	case <-done:
	case <-ctx.Done():
		a.recordDiagnostic(map[string]any{"event": "diagnostic_export_probe_timeout", "budget_ms": 5000})
	}
	mu.Lock()
	defer mu.Unlock()
	result := make(map[string]int64, len(timings))
	for name, duration := range timings {
		result[name] = duration
	}
	return result
}
