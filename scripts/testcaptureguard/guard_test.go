package testcaptureguard

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

func TestCapturedWrites(t *testing.T) {
	cases := []struct {
		name, body string
		unsafe     int
	}{
		{"counter", `n:=0; handler:=func(){n++}; register(handler)`, 1},
		{"slice", `var rows []int; register(func(){rows=append(rows,1)})`, 1},
		{"map", `rows:=map[int]int{}; register(func(){rows[1]=2; delete(rows,2)})`, 2},
		{"flag goroutine", `ready:=false; go func(){ready=true}()`, 1},
		{"atomic", `var n atomic.Int64; register(func(){n.Add(1)})`, 0},
		{"mutex", `var mu sync.Mutex; n:=0; register(func(){mu.Lock(); defer mu.Unlock(); n++})`, 0},
		{"unlocked", `var mu sync.Mutex; n:=0; register(func(){mu.Lock(); mu.Unlock(); n++})`, 1},
		{"read lock", `var mu sync.RWMutex; n:=0; register(func(){mu.RLock(); defer mu.RUnlock(); n++})`, 1},
		{"branch lock", `var mu sync.Mutex; n:=0; register(func(){if true {mu.Lock()}; n++})`, 1},
		{"local", `register(func(){n:=0;n++})`, 0},
		{"explained", "n:=0; register(func(){\n// r276-capture-safe: synchronous callback runs only before the caller returns\nn++\n})", 0},
		{"empty exemption", "n:=0; register(func(){\n// r276-capture-safe: safe\nn++\n})", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report, err := Analyze("fixture_test.go", []byte("package fixture\nimport (\"sync\";\"sync/atomic\")\nfunc test(){"+tc.body+"}"))
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Issues) != tc.unsafe {
				t.Fatalf("issues=%+v want=%d", report.Issues, tc.unsafe)
			}
		})
	}
}

func TestReviewedExceptionIsSourceBound(t *testing.T) {
	source := []byte("package fixture\nfunc test(){n:=0;register(func(){n++})}")
	report, err := Analyze("fixture_test.go", source)
	if err != nil || len(report.Issues) != 1 {
		t.Fatalf("%+v %v", report, err)
	}
	issue := report.Issues[0]
	entry := ReviewedExemption{File: issue.File, Line: issue.Line, Target: issue.Target, SourceSHA256: fmt.Sprintf("%x", sha256.Sum256(source)), Comment: "r276-capture-safe: register is synchronous and its only caller joins before reading n"}
	if err := ApplyReviewed(&report, source, []ReviewedExemption{entry}); err != nil || len(report.Issues) != 0 || len(report.Whitelists) != 1 {
		t.Fatalf("%+v %v", report, err)
	}
	changed := append(append([]byte{}, source...), []byte("\n// changed")...)
	report, _ = Analyze("fixture_test.go", changed)
	if err := ApplyReviewed(&report, changed, []ReviewedExemption{entry}); err == nil {
		t.Fatal("stale exception accepted")
	}
	report, _ = Analyze("fixture_test.go", source)
	entry.Comment = "safe"
	if err := ApplyReviewed(&report, source, []ReviewedExemption{entry}); err == nil {
		t.Fatal("unexplained exception accepted")
	}
}
