package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func Test2055CredentialIDsIsolateOwnerKindRotationAndBoundMemory(t *testing.T) {
	p := newSGPProvider()
	client := &LCUClient{}
	id := p.credentialVersion(client, sgpTokenLeagueSession, "secret")
	if id != p.credentialVersion(client, sgpTokenLeagueSession, "secret") {
		t.Fatal("unstable")
	}
	for _, got := range []string{p.credentialVersion(client, sgpTokenLeagueSession, "rotated"), p.credentialVersion(&LCUClient{}, sgpTokenLeagueSession, "secret"), p.credentialVersion(client, sgpTokenEntitlements, "secret")} {
		if got == id {
			t.Fatal("scope collision")
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p.credentialVersion(client, sgpTokenLeagueSession, fmt.Sprintf("secret-%d", i))
		}(i)
	}
	wg.Wait()
	if len(p.credentialVersions) > 32 || len(p.credentialOrder) > 32 {
		t.Fatal("unbounded credential metadata")
	}
}

func Test2055AuthUnknownFieldsAndCodesAreCountedNotExported(t *testing.T) {
	fields := sgpAuthDiagnostic([]byte(`{"errorCode":"secret-new-code","secret-name":"secret-token","details":{"code":"INSUFFICIENT_SCOPE","private-user-field":"private-id","message":"privacy restricted"}}`), "")
	if fields["auth_unknown_field_count"] != 2 || fields["auth_unknown_code_count"] != 1 {
		t.Fatal(fields)
	}
	data, _ := json.Marshal(fields)
	for _, secret := range []string{"secret-new-code", "secret-name", "secret-token", "private-user-field", "private-id"} {
		if strings.Contains(string(data), secret) {
			t.Fatal("auth detail leaked")
		}
	}
	if !strings.Contains(string(data), "INSUFFICIENT_SCOPE") || !strings.Contains(string(data), `"privacy"`) {
		t.Fatal(string(data))
	}
}

func Test2055TokenReadFailureStagesAreExplicit(t *testing.T) {
	for _, tc := range []struct {
		body   string
		status int
		kind   string
	}{{`"secret"`, 200, "none"}, {`{broken`, 200, "decode"}, {`{}`, 200, "decode"}, {`"secret"`, 401, "http-401"}, {`""`, 200, "empty-token"}} {
		t.Run(tc.kind+tc.body, func(t *testing.T) {
			p := newSGPProvider()
			events := captureSGPObservations(p)
			client := &LCUClient{baseURL: "https://127.0.0.1:1234", token: "local-secret", http: &http.Client{Transport: sgpRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}}
			ctx := context.WithValue(context.Background(), currentGameTraceKey{}, "cg-1234567890123-2")
			_, _ = p.leagueSessionTokenContext(ctx, client, true)
			if len(*events) == 0 || (*events)[len(*events)-1]["error_kind"] != tc.kind {
				t.Fatal(*events)
			}
			data, _ := json.Marshal(events)
			if strings.Contains(string(data), "secret") {
				t.Fatal("token leaked")
			}
		})
	}
}

func Test2055ClientDiagnosticNewFieldsAreAllowlisted(t *testing.T) {
	a := &app{storage: newFlowDiagnosticStore(t)}
	for _, event := range []string{"current_game_client", "champ_select_filter_client", "diagnostic_delivery_client"} {
		r := httptest.NewRecorder()
		reason := "failed"
		if event == "diagnostic_delivery_client" {
			reason = "export"
		}
		body := fmt.Sprintf(`{"event":%q,"reason":%q,"requestedPosition":"middle","resolvedPosition":"mid","requestId":5,"itemCount":23,"errorKind":"timeout","transportFailed":2,"transportDropped":1,"transportPending":999,"transportSuppressed":9999999,"transportHTTPStatus":500,"transportErrorKind":"network","key":"secret","position":"secret"}`, event, reason)
		a.handleClientDiagnostic(r, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(body)))
		if r.Code != 204 {
			t.Fatal(r.Body.String())
		}
	}
	data, _ := a.storage.readDiagnosticLog()
	if !strings.Contains(string(data), `"transport_pending":97`) {
		t.Fatal("missing bounded export pending count")
	}
	if strings.Contains(string(data), "secret") || !strings.Contains(string(data), `"error_kind":"timeout"`) || !strings.Contains(string(data), `"transport_suppressed":1000000`) || !strings.Contains(string(data), `"resolved_position":"mid"`) {
		t.Fatal(string(data))
	}
}

func Test2055DiagnosticWriteRecoveryAndReadLimit(t *testing.T) {
	store := newFlowDiagnosticStore(t)
	a := &app{storage: store}
	target := filepath.Join(store.root, "logs", "diagnostics.jsonl")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	a.recordDiagnostic(map[string]any{"event": "unwritten"})
	a.recordDiagnostic(map[string]any{"event": "unwritten"})
	if a.diagnosticWriteFailures != 2 || a.diagnosticWritePending != 2 {
		t.Fatal("write loss not observable")
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	} // Own empty fixture directory only.
	a.recordDiagnostic(map[string]any{"event": "recovered"})
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"diagnostic_write_failures_total":2`) || a.diagnosticWritePending != 0 || a.diagnosticLogErr != "" {
		t.Fatal(string(data))
	}
	_, err = readLimited(strings.NewReader("1234"), 3)
	if !errors.Is(err, errResponseLimitExceeded) || diagnosticErrorKind(err) != "response-too-large" {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(strings.Repeat(" ", 2*1024*1024+64*1024+1)), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = store.readDiagnosticLogForExport()
	if diagnosticErrorKind(err) != "response-too-large" {
		t.Fatal(err)
	}
}

type shortDiagnosticWriter struct{ header http.Header }

func (w *shortDiagnosticWriter) Header() http.Header       { return w.header }
func (*shortDiagnosticWriter) WriteHeader(int)             {}
func (*shortDiagnosticWriter) Write(p []byte) (int, error) { return len(p) / 2, nil }
func Test2055DiagnosticExportWriteFailureIsRecorded(t *testing.T) {
	a := &app{storage: newFlowDiagnosticStore(t)}
	a.recordDiagnostic(map[string]any{"event": "fixture"})
	a.handleDiagnosticLog(&shortDiagnosticWriter{header: make(http.Header)}, httptest.NewRequest("GET", "/api/diagnostics/log", nil))
	data, _ := a.storage.readDiagnosticLog()
	if !strings.Contains(string(data), `"event":"diagnostic_export_failed"`) || !strings.Contains(string(data), `"stage":"response-write"`) {
		t.Fatal(string(data))
	}
}

func Test2055RankingHandlerAcceptsCanonicalFivePositions(t *testing.T) {
	rows := []map[string]any{}
	for i := 1; i <= 50; i++ {
		positions := []map[string]any{}
		for _, p := range []string{"top", "jungle", "mid", "adc", "support"} {
			positions = append(positions, map[string]any{"name": p, "stats": map[string]any{"play": 100, "win_rate": 0.5}})
		}
		rows = append(rows, map[string]any{"id": i, "positions": positions})
	}
	data, _ := json.Marshal(map[string]any{"data": rows})
	p := newChampionProvider()
	p.cache = newChampionDataCache(nil)
	p.client = &http.Client{Transport: championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != opggChampionHost || r.URL.Path != "/api/KR/champions/ranked" {
			t.Fatal("unexpected upstream path")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(data))), Request: r}, nil
	})}
	a := &app{champions: p}
	for _, position := range []string{"top", "jungle", "mid", "adc", "support"} {
		w := httptest.NewRecorder()
		a.handleChampionRankings(w, httptest.NewRequest("GET", "/api/champions/rankings?mode=ranked&tier=emerald_plus&position="+position, nil))
		if w.Code != 200 {
			t.Fatalf("%s status %d: %s", position, w.Code, w.Body.String())
		}
		var response championRankingResponse
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || len(response.Rows) != 50 {
			t.Fatalf("%s rows %d err %v", position, len(response.Rows), err)
		}
	}
}
