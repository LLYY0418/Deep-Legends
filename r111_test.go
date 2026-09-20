package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestR111DetectEndpointNeverWritesOrResetsClientPreferences(t *testing.T) {
	var reads, writes atomic.Int32
	client := r99Client(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writes.Add(1)
		}
		reads.Add(1)
		fmt.Fprint(w, `{}`)
	})
	a := &app{connected: true, lcu: client}
	w := httptest.NewRecorder()
	a.handleFacadeProbe(w, httptest.NewRequest(http.MethodPost, "/api/facade/probe", strings.NewReader(`{}`)))
	var out struct {
		Mode        string `json:"mode"`
		WriteTested bool   `json:"writeTested"`
	}
	if json.Unmarshal(w.Body.Bytes(), &out) != nil || w.Code != 200 || out.Mode != "read-only" || out.WriteTested || reads.Load() != 8 || writes.Load() != 0 {
		t.Fatalf("%d %s reads=%d writes=%d", w.Code, w.Body.String(), reads.Load(), writes.Load())
	}
}
func TestR111QuotaDiagnosticsDistinguishLocalBudgetAndObservedLimits(t *testing.T) {
	var events []map[string]any
	p := &riotProvider{champions: &championProvider{diag: func(e map[string]any) { events = append(events, e) }}}
	ctx := r110RateContext("asia", "/lol/match/v5/matches/KR_private")
	if e := p.riotQuotaDiagnostic(ctx, 21); e["source"] != "fallback" || e["upstream_429"] != false {
		t.Fatal(e)
	}
	r110Advertise(p, "asia", "/lol/match/v5/matches/KR_private", "20:1,100:120", "1:1,99:120")
	r110Advertise(p, "asia", "/lol/match/v5/matches/KR_private", "20:1,100:120", "2:1,100:120")
	if len(events) != 1 {
		t.Fatal("unchanged rate policy repeated", events)
	}
	e := p.riotQuotaDiagnostic(ctx, 21)
	if e["source"] != "response-headers" || e["upstream_429"] != false || len(e["windows"].([]map[string]any)) != 2 {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(e)
	if strings.Contains(string(raw), "private") {
		t.Fatal("private match ID leaked", string(raw))
	}
	for _, path := range []string{"/lol/match/v5/matches/EUW1_private", "/lol/match/v5/matches/NA1_private/timeline"} {
		if strings.Contains(riotRequestRateScope("fixture", path).method, "private") {
			t.Fatal("match ID leaked", path)
		}
	}
}

func TestR111Headerless429DoesNotInventAnObservedPolicy(t *testing.T) {
	var events []map[string]any
	p := &riotProvider{champions: &championProvider{diag: func(e map[string]any) { events = append(events, e) }}}
	scope := riotRequestRateScope("asia", "/lol/match/v5/matches/KR_private")
	p.observeRiotRate(scope, http.Header{"Retry-After": {"3"}}, http.StatusTooManyRequests)
	if len(events) != 0 {
		t.Fatal("fallback was reported as a response policy", events)
	}
	if e := p.riotQuotaDiagnostic(r110RateContext(scope.host, scope.method), 3); e["source"] != "fallback" {
		t.Fatal(e)
	}
}
