package main

import (
	"bytes"
	"net/http"
	"sync"
	"testing"
	"time"
)

// Download completion is not completion of canceled probe diagnostics. Join
// those fixture producers under the original single ten-second watchdog.
func r276WaitUpdateProbeDiagnostics(t *testing.T, events *r196Events, watchdog <-chan time.Time) {
	t.Helper()
	if events.probeWaitStarted != nil {
		close(events.probeWaitStarted)
	}
	for completed := 0; completed < 3; completed++ {
		select {
		case <-events.probeCompleted:
		case <-watchdog:
			t.Fatal("probe diagnostics did not complete within original watchdog")
		}
	}
}

func TestR276UpdateProbeDiagnosticsWaitForCanceledSource(t *testing.T) {
	data := bytes.Repeat([]byte("installer"), 256*1024)
	u := updateTestManager(t, data)
	u.mirrors = []string{"", "https://fast.test/", "https://html.test/"}
	release := make(chan struct{})
	blocked := make(chan struct{})
	abort := make(chan struct{})
	released := sync.Once{}
	events := &r196Events{probeCompleted: make(chan struct{}, 3), probeWaitStarted: make(chan struct{})}
	u.diagnostic = func(row map[string]any) {
		if row["event"] == "update_download_probe" && row["mirror_prefix"] == "direct" {
			close(blocked)
			<-release
		}
		events.record(row)
	}
	u.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Range") == "bytes=0-1048575" {
			if r.URL.Host == "github.com" {
				<-r.Context().Done()
				return nil, r.Context().Err()
			}
			if r.URL.Host == "html.test" {
				return r196Response([]byte("<html>error</html>")), nil
			}
		}
		return r196Response(data), nil
	})}
	var aborted sync.Once
	cleanup := func() { aborted.Do(func() { close(abort) }); released.Do(func() { close(release) }) }
	t.Cleanup(cleanup)
	if err := u.Download(); err != nil {
		t.Fatal(err)
	}
	u.mu.Lock()
	done := u.downloadDone
	u.mu.Unlock()
	watchdog := time.After(10 * time.Second)
	select {
	case <-done:
	case <-watchdog:
		t.Fatal("download did not complete")
	}
	select {
	case <-blocked:
	case <-watchdog:
		t.Fatal("canceled probe did not reach controlled diagnostic gate")
	}
	if before := len(events.named("update_download_probe")); before != 2 {
		t.Fatal("old completion-only diagnostic assertion not reproduced", before)
	}
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		select {
		case <-events.probeWaitStarted:
		case <-abort:
		}
		released.Do(func() { close(release) })
	}()
	t.Cleanup(func() { cleanup(); <-joined })
	r276WaitUpdateProbeDiagnostics(t, events, watchdog)
	if after := len(events.named("update_download_probe")); after != 3 {
		t.Fatalf("probe diagnostic fixture has %d events; want all 3", after)
	}
	if u.Status().State != "ready" {
		t.Fatal("download state changed", u.Status())
	}
}
