package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestR86DiagnosticBufferBudgetAndImmediateRead(t *testing.T) {
	s := newFlowDiagnosticStore(t)
	stats := 0
	s.diagnosticLstat = func(path string) (os.FileInfo, error) { stats++; return os.Lstat(path) }
	for i := 0; i < 10000; i++ {
		if err := s.appendDiagnostic(map[string]any{"event": "budget"}); err != nil {
			t.Fatal(err)
		}
	}
	data, err := s.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	if stats > 200 {
		t.Fatalf("Lstat=%d want <=200", stats)
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte{'\n'})
	if len(lines) != 10000 {
		t.Fatalf("records=%d", len(lines))
	}
	for i, line := range lines {
		var row struct {
			Seq int `json:"log_seq"`
		}
		if err := json.Unmarshal(line, &row); err != nil {
			t.Fatal(err)
		}
		if row.Seq != i+1 {
			t.Fatalf("record %d sequence %d", i, row.Seq)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.appendDiagnostic(map[string]any{"event": "after-close"}); err == nil {
		t.Fatal("closed writer accepted event")
	}
}

func TestR86DiagnosticTimerFlushAndAsyncFailureVisible(t *testing.T) {
	s := newFlowDiagnosticStore(t)
	if err := s.appendDiagnostic(map[string]any{"event": "timer-flushed"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		s.diagnosticMu.Lock()
		closed := s.diagnosticFile == nil
		s.diagnosticMu.Unlock()
		if closed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("200ms timer did not flush/release handle")
		}
		time.Sleep(5 * time.Millisecond)
	}
	data, err := os.ReadFile(filepath.Join(s.root, "logs", "diagnostics.jsonl"))
	if err != nil || !bytes.Contains(data, []byte("timer-flushed")) {
		t.Fatalf("timer flush: %v", err)
	}
	s.diagnosticMu.Lock()
	s.diagnosticAsyncErr = errors.New("fixture-flush-failure")
	s.diagnosticMu.Unlock()
	if _, err := s.readDiagnosticLogForExport(); err == nil {
		t.Fatal("async write failure hidden from export")
	}
	if err := s.appendDiagnostic(map[string]any{"event": "observes-failure"}); err == nil {
		t.Fatal("async failure hidden from caller")
	}
	if err := s.appendDiagnostic(map[string]any{"event": "recovered"}); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkAppendDiagnostic(b *testing.B) {
	root := b.TempDir()
	if err := os.Mkdir(filepath.Join(root, "logs"), 0700); err != nil {
		b.Fatal(err)
	}
	s := &localStore{root: root}
	defer s.Close()
	stats := 0
	s.diagnosticLstat = func(p string) (os.FileInfo, error) { stats++; return os.Lstat(p) }
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := s.appendDiagnostic(map[string]any{"event": "benchmark"}); err != nil {
			b.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		b.Fatal(err)
	}
	b.ReportMetric(float64(stats), "lstat-total")
}

// Release descriptors before TempDir cleanup on Windows as well as Unix.
func trackTestStore(t testing.TB, s *localStore) *localStore {
	t.Helper()
	t.Cleanup(func() { _ = s.Close() })
	return s
}
