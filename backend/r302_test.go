package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func r302PatchClient(mode string) *http.Client {
	return &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "apps.game.qq.com" && mode == "official" {
			return updateResponse(200, []byte(`callback({"data":{"result":[{"sTitle":"26.21版本更新公告","sRedirectURL":"https://lol.qq.com/gicp/news/410/37099805.html"}]}})`)), nil
		}
		if strings.HasSuffix(r.URL.Path, "patch-notes.json") && mode != "failed" {
			return updateResponse(200, bundledPatchNotes), nil
		}
		return updateResponse(503, nil), nil
	})}
}

type r302PatchResponse struct {
	Version           string    `json:"version"`
	UpdateStatus      string    `json:"updateStatus"`
	SuccessfulAt      time.Time `json:"successfulAt"`
	OfficialCheckedAt time.Time `json:"officialCheckedAt"`
}

func r302PatchStatus(t *testing.T, a *app) r302PatchResponse {
	t.Helper()
	w := httptest.NewRecorder()
	a.handlePatchNotes(w, httptest.NewRequest(http.MethodGet, "/api/patch-notes", nil))
	var data r302PatchResponse
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &data) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	return data
}

func TestR302PatchNotesFallbackDoesNotClaimOfficialCheck(t *testing.T) {
	a := &app{storage: newFlowDiagnosticStore(t)}
	c := a.patchNotesState()
	c.mu.Lock()
	c.documents = c.documents[1:]
	c.mu.Unlock()
	client := r302PatchClient("fallback")
	err := a.refreshPatchNotesWithClient(context.Background(), client)
	if !errors.Is(err, errPatchNotesOfficialUnchecked) || !strings.Contains(err.Error(), "503") {
		t.Fatal("fallback must wrap the sentinel and official error", err)
	}
	a.finishPatchNotesRefreshUsing(context.Background(), client, "startup")
	data := r302PatchStatus(t, a)
	if data.Version != "26.21" || data.UpdateStatus != "fallback" || data.SuccessfulAt.IsZero() || !data.OfficialCheckedAt.IsZero() {
		t.Fatal("fallback data must not masquerade as an official check", data)
	}
	c.mu.Lock()
	failed, fallback := c.failed, c.fallback
	c.fetching = true
	c.mu.Unlock()
	if failed || !fallback || r302PatchStatus(t, a).UpdateStatus != "checking" {
		t.Fatal("fallback success/checking precedence lost", failed, fallback)
	}
}

func TestR302PatchNotesOfficialAndFailureStates(t *testing.T) {
	a := &app{storage: newFlowDiagnosticStore(t)}
	if data := r302PatchStatus(t, a); data.UpdateStatus != "bundled" {
		t.Fatal(data)
	}
	a.finishPatchNotesRefreshUsing(context.Background(), r302PatchClient("official"), "dialog")
	official := r302PatchStatus(t, a)
	if official.UpdateStatus != "checked" || official.SuccessfulAt.IsZero() || official.OfficialCheckedAt.IsZero() {
		t.Fatal("successful official index check must be checked", official)
	}
	a.finishPatchNotesRefreshUsing(context.Background(), r302PatchClient("fallback"), "dialog")
	fallback := r302PatchStatus(t, a)
	if fallback.UpdateStatus != "fallback" || !fallback.OfficialCheckedAt.Equal(official.OfficialCheckedAt) {
		t.Fatal("fallback must preserve the last real official check time", fallback)
	}
	a.finishPatchNotesRefreshUsing(context.Background(), r302PatchClient("failed"), "dialog")
	failed := r302PatchStatus(t, a)
	if failed.UpdateStatus != "failed" || !failed.SuccessfulAt.Equal(fallback.SuccessfulAt) || !failed.OfficialCheckedAt.Equal(official.OfficialCheckedAt) || failed.Version != "26.21" {
		t.Fatal("total failure must retain data and successful timestamps", failed)
	}
}

func TestR302PatchNotesRetryUsesAvailableDataAtTarget(t *testing.T) {
	a := &app{storage: newFlowDiagnosticStore(t)}
	c := a.patchNotesState()
	now := releaseTestTime("2026-10-24 12:00:00")
	rows := a.loadPatchReleaseState().Releases
	var calls atomic.Int64
	refresh := func(string) (string, bool) {
		calls.Add(1)
		a.finishPatchNotesRefreshUsing(context.Background(), r302PatchClient("fallback"), "startup")
		return c.availableVersion()
	}
	plan := patchNotesRetryPlan{Next: now}
	plan.step(now, rows, "26.20", time.Time{}, refresh)
	if !plan.Complete || calls.Load() != 1 || r302PatchStatus(t, a).UpdateStatus != "fallback" {
		t.Fatal("fallback at target must complete retry plan", plan, calls.Load())
	}
	plan.step(now.Add(time.Hour), rows, "26.21", time.Time{}, refresh)
	if calls.Load() != 1 {
		t.Fatal("fallback target retried", calls.Load())
	}

	// A later transient failure cannot erase already acquired target data.
	a.finishPatchNotesRefreshUsing(context.Background(), r302PatchClient("failed"), "dialog")
	version, available := c.availableVersion()
	if version != "26.21" || !available || r302PatchStatus(t, a).UpdateStatus != "failed" {
		t.Fatal("available data and last fetch failure must be independent", version, available)
	}
	plan = patchNotesRetryPlan{Next: now}
	plan.step(now, rows, version, time.Time{}, func(string) (string, bool) { return c.availableVersion() })
	if !plan.Complete {
		t.Fatal("available target must stop retries despite later failure")
	}

	// Bundled data alone is not a successful network refresh.
	version, available = (&app{}).patchNotesState().availableVersion()
	if version != "26.21" || available {
		t.Fatal("bundled data must not count as a successful fetch", version, available)
	}
}
