package main

import (
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestR301NotesStartupRetryAndBroadcast(t *testing.T) {
	a := &app{storage: newFlowDiagnosticStore(t)}
	c := a.patchNotesState()
	c.mu.Lock()
	c.documents = c.documents[1:]
	c.mu.Unlock()
	events := make(chan string, 8)
	a.eventSubscribers = map[chan string]struct{}{events: {}}
	var count atomic.Int64
	client := &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "patch-notes.json") {
			return updateResponse(200, bundledPatchNotes), nil
		}
		return updateResponse(503, nil), nil
	})}
	rows := a.loadPatchReleaseState().Releases
	now := releaseTestTime("2026-10-24 12:00:00")
	plan := patchNotesRetryPlan{Next: now.Add(15 * time.Second)}
	refresh := func(trigger string) (string, bool) {
		count.Add(1)
		if count.Load() == 1 {
			return "26.20", false
		}
		if !a.beginPatchNotesRefresh(false) {
			t.Fatal("first real fetch blocked")
		}
		a.finishPatchNotesRefreshUsing(context.Background(), client, trigger)
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.documents[0].Version, !c.failed
	}
	if next := plan.step(now, rows, "26.20", time.Time{}, refresh); !next.Equal(now.Add(15*time.Second)) || count.Load() != 0 {
		t.Fatal(next, count.Load())
	}
	first := now.Add(15 * time.Second)
	next := plan.step(first, rows, "26.20", time.Time{}, refresh)
	if !next.Equal(first.Add(time.Minute)) || count.Load() != 1 {
		t.Fatal(next, count.Load())
	}
	plan.step(next.Add(-time.Second), rows, "26.20", time.Time{}, refresh)
	if count.Load() != 1 {
		t.Fatal("tight retry")
	}
	plan.step(next, rows, "26.20", time.Time{}, refresh)
	if count.Load() != 2 || !plan.Complete {
		t.Fatal(plan, count.Load())
	}
	select {
	case event := <-events:
		if !strings.Contains(event, "patch-notes-changed") {
			t.Fatal(event)
		}
	default:
		t.Fatal("missing update broadcast")
	}
	diagnostics := readDiagnosticEvents(t, a.storage, "patch_notes_refresh")
	if len(diagnostics) < 2 {
		t.Fatal(diagnostics)
	}
	found := false
	for _, d := range diagnostics {
		if d["source"] == "notices" && d["outcome"] == "ok" && d["latest_version"] == "26.21" && d["trigger"] == "retry" {
			found = true
		}
		raw, _ := json.Marshal(d)
		if strings.Contains(string(raw), "https://") {
			t.Fatal("diagnostic leaked URL", string(raw))
		}
	}
	if !found {
		t.Fatal(diagnostics)
	}
	plan.step(next.Add(time.Hour), rows, "26.21", time.Time{}, refresh)
	if count.Load() != 2 {
		t.Fatal("successful target retried")
	}
	restart := patchNotesRetryPlan{Next: next.Add(15 * time.Second)}
	restart.step(next, rows, "26.21", time.Time{}, refresh)
	if restart.Started {
		t.Fatal("startup delay lost")
	}
}

func TestR301ColdStartupAndBackoffBoundaries(t *testing.T) {
	rows := (&app{}).loadPatchReleaseState().Releases
	now := releaseTestTime("2026-10-24 12:00:00")
	p := patchNotesRetryPlan{Next: now.Add(15 * time.Second)}
	var calls atomic.Int64
	success := func(trigger string) (string, bool) {
		calls.Add(1)
		if trigger != "startup" {
			t.Fatal(trigger)
		}
		return "26.21", true
	}
	p.step(now.Add(14*time.Second), rows, "26.20", time.Time{}, success)
	if calls.Load() != 0 {
		t.Fatal("startup too early")
	}
	p.step(now.Add(15*time.Second), rows, "26.20", time.Time{}, success)
	if calls.Load() != 1 || !p.Complete {
		t.Fatal(p, calls.Load())
	}
	p = patchNotesRetryPlan{Next: now}
	for i, delay := range []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, time.Hour} {
		at := now
		next := p.step(at, rows, "26.20", time.Time{}, func(string) (string, bool) { return "26.20", false })
		if !next.Equal(at.Add(delay)) || p.Failures != i+1 {
			t.Fatal(i, next, at, p)
		}
		now = next
	}
	c := (&app{}).patchNotesState()
	c.attemptedAt = time.Now().Add(-59 * time.Second)
	a := &app{patchNotes: c}
	a.patchNotesOnce.Do(func() {})
	if a.beginPatchNotesRefresh(true) {
		t.Fatal("dialog bypassed one-minute gate")
	}
}

func TestR301AnnouncementIndependentMirrorBudget(t *testing.T) {
	var callsMu sync.Mutex
	calls := []string{}
	client := &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) < 3500*time.Millisecond || time.Until(deadline) > 4*time.Second {
			t.Fatal("mirror lost its four-second budget", time.Until(deadline))
		}
		callsMu.Lock()
		calls = append(calls, r.URL.String())
		callsMu.Unlock()
		if strings.HasPrefix(r.URL.String(), announcementBranchURL) {
			return nil, context.DeadlineExceeded
		}
		if strings.HasSuffix(r.URL.Path, "announcements.v2.json") {
			return updateResponse(404, nil), nil
		}
		return updateResponse(200, bundledAnnouncements), nil
	})}
	started := time.Now()
	_, list, err := fetchAnnouncementDocument(context.Background(), client)
	if err != nil || len(list) == 0 || len(calls) != 4 || !strings.HasSuffix(calls[2], "announcements.v2.json") || !strings.HasSuffix(calls[3], "announcements.json") || time.Since(started) > 20*time.Second {
		t.Fatal(calls, err)
	}
	_, _, err = fetchAnnouncementDocument(context.Background(), &http.Client{Transport: updateRoundTrip(func(*http.Request) (*http.Response, error) { return updateResponse(404, nil), nil })})
	var failure *announcementFetchError
	if !errors.As(err, &failure) || len(failure.Outcomes) != 2*len(updateMirrors) || failure.MirrorIndex != len(updateMirrors)-1 {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(failure.Outcomes)
	if strings.Contains(string(raw), "http://") || strings.Contains(string(raw), "https://") {
		t.Fatal(string(raw))
	}
}

func TestR301CanceledSelectionCleansOnlyEditableDLPages(t *testing.T) {
	for _, phase := range []string{"Lobby", "Matchmaking", "None", "ChampSelect", "InProgress"} {
		t.Run(phase, func(t *testing.T) {
			no := false
			var mu sync.Mutex
			pages := map[int64]lcuRunePage{1: {ID: 1, Name: "用户"}, 2: {ID: 2, Name: "[DL] 安妮"}, 3: {ID: 3, Name: "[DL] 只读", IsEditable: &no}, 4: {ID: 4, Name: "[DL] 受保护", IsDeletable: &no}}
			client := &LCUClient{baseURL: "https://127.0.0.1", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				mu.Lock()
				defer mu.Unlock()
				switch r.Method + " " + r.URL.Path {
				case "GET /lol-perks/v1/pages":
					list := []lcuRunePage{}
					for _, p := range pages {
						list = append(list, p)
					}
					raw, _ := json.Marshal(list)
					return updateResponse(200, raw), nil
				case "GET /lol-gameflow/v1/gameflow-phase":
					raw, _ := json.Marshal(phase)
					return updateResponse(200, raw), nil
				case "DELETE /lol-perks/v1/pages/2":
					delete(pages, 2)
					return updateResponse(204, nil), nil
				default:
					t.Errorf("user/protected page touched: %s %s", r.Method, r.URL.Path)
					return updateResponse(500, nil), nil
				}
			})}}
			a := &app{}
			a.cleanupDLRunePages(context.Background(), client, true)
			if (pages[2].ID == 0) != canceledChampSelectPhase(phase) || len(pages) < 3 || pages[1].Name != "用户" || pages[3].ID != 3 || pages[4].ID != 4 {
				t.Fatal(pages)
			}
		})
	}
	// Exercise the actual phase event trigger, rather than only the helper.
	done := make(chan struct{}, 1)
	client := &LCUClient{baseURL: "https://127.0.0.1", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/lol-perks/v1/pages" {
			done <- struct{}{}
			return updateResponse(200, []byte(`[]`)), nil
		}
		return updateResponse(500, nil), nil
	})}}
	a := &app{}
	a.autoConfig.phase = "ChampSelect"
	a.observeAutoConfigEvent(context.Background(), client, LCUEvent{URI: "/lol-gameflow/v1/gameflow-phase", Data: json.RawMessage(`"Lobby"`)})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cancel event never cleaned pages")
	}
}
func TestR301ManualRefreshDiagnosticsAllowReasonsAndSafeStage(t *testing.T) {
	a := &app{storage: newFlowDiagnosticStore(t)}
	for _, reason := range []string{"timeout", "released-no-client", "released-deadline"} {
		r := httptest.NewRequest(http.MethodPost, "/api/diagnostics/client", strings.NewReader(`{"event":"manual_refresh_client","reason":"`+reason+`","stage":"hard-refresh-wait","busy_ms":8000}`))
		w := httptest.NewRecorder()
		a.handleClientDiagnostic(w, r)
		if w.Code != 204 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	events := readDiagnosticEvents(t, a.storage, "manual_refresh_client")
	if len(events) != 3 {
		t.Fatal(events)
	}
	for _, e := range events {
		if e["stage"] != "hard-refresh-wait" {
			t.Fatal(e)
		}
	}
}

func TestR301StartupLoopIsWired(t *testing.T) {
	file := os.Getenv("R301_PATCH_NOTES_SOURCE")
	if file == "" {
		file = "patch_notes.go"
	}
	tree, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	wired := false
	for _, decl := range tree.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "startPatchNotes" {
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "runPatchNotesRetry" {
						wired = true
					}
				}
				return true
			})
		}
	}
	if !wired {
		t.Fatal("startup notes loop not registered")
	}
}
