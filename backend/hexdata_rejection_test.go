package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// R146：上游拒绝请求时，诊断里要有上游给出的错误码，但不带响应正文的其他内容。
func TestHexdataUpstreamRejectionIsDiagnosedWithErrorCodeOnly(t *testing.T) {
	provider := newHexdataBudgetProvider(t, t.TempDir(), championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		header := make(http.Header)
		header.Set("Content-Type", "application/json")
		header.Set("Server", "test-edge")
		body := `{"error":"data_not_public","message":"SECRET-MESSAGE-BODY"}`
		return &http.Response{StatusCode: http.StatusForbidden, Header: header, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	}))
	var events []map[string]any
	provider.diag = func(event map[string]any) { events = append(events, event) }
	if _, err := provider.loadHexdataMeta(context.Background()); err == nil {
		t.Fatal("403 must fail the load")
	}
	var found map[string]any
	for _, event := range events {
		if event["event"] == "hexdata_upstream_rejected" {
			found = event
		}
	}
	if found == nil {
		t.Fatalf("no hexdata_upstream_rejected event in %v", events)
	}
	if found["status"] != http.StatusForbidden || found["error_code"] != "data_not_public" || found["path"] != hexdataMetaPath {
		t.Fatalf("event = %v", found)
	}
	for _, event := range events {
		for _, value := range event {
			if text, ok := value.(string); ok && strings.Contains(text, "SECRET-MESSAGE-BODY") {
				t.Fatalf("response body leaked into diagnostics: %v", event)
			}
		}
	}
}
