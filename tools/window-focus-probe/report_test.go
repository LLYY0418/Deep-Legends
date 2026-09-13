package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestReportRoundTripAndArchiveAllowlist(t *testing.T) {
	r, err := newRecorder(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				r.event("test", "sample", sampleOK())
			}
		}()
	}
	wg.Wait()
	if err := os.WriteFile(filepath.Join(r.dir, "do-not-include.txt"), []byte("not in allowlist"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := r.finish(candidateTrials(), true, nil, map[string]any{"build": 1})
	if err != nil {
		t.Fatal(err)
	}
	if f.Code != "candidate_only" {
		t.Fatal(f)
	}
	b, err := os.ReadFile(filepath.Join(r.dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report finalReport
	if err = json.Unmarshal(b, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Trials) != 4 || report.Schema != 1 || len(report.Limits) < 5 {
		t.Fatal("missing evidence")
	}
	b, _ = os.ReadFile(filepath.Join(r.dir, "trace.jsonl"))
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 61 {
		t.Fatalf("lost events: %d", len(lines))
	}
	for _, line := range lines {
		if !json.Valid([]byte(line)) {
			t.Fatal("invalid JSONL")
		}
	}
	z, err := zip.OpenReader(filepath.Join(r.dir, "WindowProbe-Report.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	if len(z.File) != 3 {
		t.Fatal("archive must only contain three report files")
	}
	for _, f := range z.File {
		if strings.Contains(f.Name, "/") || strings.Contains(f.Name, "do-not-") {
			t.Fatal("unsafe entry", f.Name)
		}
	}
}

func TestCancelledRunCannotPass(t *testing.T) {
	r, err := newRecorder(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f, err := r.finish(candidateTrials(), true, errors.New("cancelled"), nil)
	if err != nil || f.Code != "incomplete" {
		t.Fatal(f, err)
	}
}

func TestReportWriteFailureIsNotSuccess(t *testing.T) {
	r, err := newRecorder(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r.file.Close()
	r.event("test", "sample", nil)
	_, err = r.finish(nil, true, nil, nil)
	if err == nil {
		t.Fatal("must expose trace failure")
	}
}

func TestRunDirectoriesAreUnique(t *testing.T) {
	dir := t.TempDir()
	a, err := newRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer a.file.Close()
	b, err := newRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer b.file.Close()
	if a.dir == b.dir {
		t.Fatal("must not overwrite previous run")
	}
}
