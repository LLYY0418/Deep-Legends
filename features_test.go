package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClientDiagnosticAcceptsMultipleEventWhitelists(t *testing.T) {
	for event, reasons := range clientDiagnosticEvents {
		for reason := range reasons {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/diagnostics/client", strings.NewReader(`{"event":"`+event+`","reason":"`+reason+`","key":"13:middle:CLASSIC:11:4-12","championId":13,"queueId":420,"position":"MID"}`))
			(&app{}).handleClientDiagnostic(recorder, request)
			if recorder.Code != http.StatusNoContent {
				t.Fatalf("whitelisted event=%q reason=%q returned %d: %s", event, reason, recorder.Code, recorder.Body.String())
			}
		}
	}

	for name, body := range map[string]string{
		"unknown event":  `{"event":"other","reason":"cached"}`,
		"unknown reason": `{"event":"specialist_runes_client_skip","reason":"key-missing-permanent"}`,
		"unknown field":  `{"event":"specialist_runes_client_skip","reason":"cached","playerRef":"private"}`,
	} {
		recorder := httptest.NewRecorder()
		(&app{}).handleClientDiagnostic(recorder, httptest.NewRequest(http.MethodPost, "/api/diagnostics/client", strings.NewReader(body)))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s returned %d, want 400", name, recorder.Code)
		}
	}
}

func TestClientDiagnosticRejectsAndLogsUnknownEvent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	a := &app{storage: store}
	recorder := httptest.NewRecorder()
	a.handleClientDiagnostic(recorder, httptest.NewRequest(http.MethodPost, "/api/diagnostics/client", strings.NewReader(`{"event":"future_event","reason":"cached"}`)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unknown event status = %d, want 400", recorder.Code)
	}
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if event["event"] != "client_diagnostic_rejected" || event["reason"] != "unknown-event" || event["raw_event"] != "future_event" {
		t.Fatalf("rejection event = %#v", event)
	}
}

func TestClientDiagnosticTruncatesKeyBeforeWriting(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	a := &app{storage: store}
	key := strings.Repeat("k", clientDiagnosticTextLimit+20)
	recorder := httptest.NewRecorder()
	a.handleClientDiagnostic(recorder, httptest.NewRequest(http.MethodPost, "/api/diagnostics/client", strings.NewReader(`{"event":"live_recommendations_skip","reason":"cached","key":"`+key+`"}`)))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if got := event["key"].(string); len([]rune(got)) != clientDiagnosticTextLimit {
		t.Fatalf("key length = %d, want %d", len([]rune(got)), clientDiagnosticTextLimit)
	}
}

func TestClientDiagnosticWritesSanitizedEvent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	a := &app{storage: store}
	recorder := httptest.NewRecorder()
	body := `{"event":"specialist_runes_client_skip","reason":"in-flight","championId":13,"queueId":0,"position":"MIDDLE"}`
	a.handleClientDiagnostic(recorder, httptest.NewRequest(http.MethodPost, "/api/diagnostics/client", strings.NewReader(body)))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("diagnostic status = %d: %s", recorder.Code, recorder.Body.String())
	}
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if event["event"] != "specialist_runes_client_skip" || event["reason"] != "in-flight" || event["position"] != "middle" || event["champion_id"] != float64(13) || event["queue_id"] != float64(0) {
		t.Fatalf("client diagnostic = %#v", event)
	}
	for _, privateField := range []string{"playerRef", "puuid", "account"} {
		if _, ok := event[privateField]; ok {
			t.Fatalf("client diagnostic leaked %s: %#v", privateField, event)
		}
	}
}

func TestMatchTierOverviewDiagnosticKeepsOnlyAggregateCounts(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	a := &app{storage: trackTestStore(t, &localStore{root: root})}
	recorder := httptest.NewRecorder()
	body := `{"event":"match_tiers_overview_batch","reason":"complete","totalRefs":36,"uniqueRefs":24,"cacheHits":19}`
	a.handleClientDiagnostic(recorder, httptest.NewRequest(http.MethodPost, "/api/diagnostics/client", strings.NewReader(body)))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	data, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if event["total_refs"] != float64(36) || event["unique_refs"] != float64(24) || event["cache_hits"] != float64(19) {
		t.Fatalf("aggregate diagnostic = %#v", event)
	}
	for _, forbidden := range []string{"player_ref", "puuid", "account_id", "summoner_id"} {
		if _, ok := event[forbidden]; ok {
			t.Fatalf("aggregate diagnostic leaked %s: %#v", forbidden, event)
		}
	}
}
