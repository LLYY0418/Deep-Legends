package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestPrewarmDefaultsOnWithExplicitControl(t *testing.T) {
	t.Setenv("DEEP_LEGENDS_STARTUP_PREWARM", "")
	if err := os.Unsetenv("DEEP_LEGENDS_STARTUP_PREWARM"); err != nil {
		t.Fatal(err)
	}
	if !startupPrewarmEnabled(os.Getenv("DEEP_LEGENDS_STARTUP_PREWARM")) {
		t.Fatal("unset must enable prewarm")
	}
	for _, value := range []string{"", "1", "true", "yes", " "} {
		if !startupPrewarmEnabled(value) {
			t.Errorf("%q must enable prewarm", value)
		}
	}
	if startupPrewarmEnabled("0") {
		t.Fatal("explicit control must disable prewarm")
	}
}

func TestPrewarmSequentialAndFailureDoesNotStopNextFile(t *testing.T) {
	if startupPrewarmBudget != 8*time.Second {
		t.Fatal("prewarm budget changed")
	}
	paths := startupPrewarmPaths("destination")
	if !reflect.DeepEqual(paths, []string{filepath.Join("destination", "Deep Legends.exe"), filepath.Join("destination", "resources", "app.asar.unpacked", "backend", "loot-service.exe")}) {
		t.Fatal(paths)
	}
	var reads []string
	var progress []int
	result := runStartupPrewarm(paths, func(_ context.Context, path string) error {
		reads = append(reads, path)
		return errors.New("unreadable")
	}, time.Second, func(p int) { progress = append(progress, p) })
	if !reflect.DeepEqual(reads, paths) || result.Files != 2 || result.Failures != 2 || result.TimedOut || !reflect.DeepEqual(progress, []int{98, 100}) {
		t.Fatalf("%v %v %+v", reads, progress, result)
	}
}

func TestPrewarmBudgetDoesNotWaitForABlockedFileRead(t *testing.T) {
	blocked := make(chan struct{})
	defer close(blocked)
	started := time.Now()
	result := runStartupPrewarm([]string{"blocked", "must not read"}, func(_ context.Context, path string) error {
		if path != "blocked" {
			t.Error("read another file after deadline")
		}
		<-blocked
		return nil
	}, 10*time.Millisecond, func(int) { t.Error("progress after timeout") })
	if !result.TimedOut || time.Since(started) > time.Second {
		t.Fatal("blocked IO delayed application launch", result)
	}
}

func TestPrewarmReadsContentsWithoutChangingFiles(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sample.exe")
	content := make([]byte, 400000)
	if err := os.WriteFile(file, content, 0600); err != nil {
		t.Fatal(err)
	}
	if err := readPrewarmFile(context.Background(), file); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(file)
	if err != nil || info.Size() != int64(len(content)) {
		t.Fatal("prewarm changed file")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := readPrewarmFile(ctx, file); !errors.Is(err, context.Canceled) {
		t.Fatal("read ignored cancellation", err)
	}
}
