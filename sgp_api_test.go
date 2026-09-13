package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

type sgpRoundTripFunc func(*http.Request) (*http.Response, error)

type sgpErrorReader struct{}

func (sgpErrorReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func (fn sgpRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func captureSGPObservations(provider *sgpProvider) *[]map[string]any {
	events := make([]map[string]any, 0, 4)
	provider.observe = func(event map[string]any) {
		copy := make(map[string]any, len(event))
		for key, value := range event {
			copy[key] = value
		}
		events = append(events, copy)
	}
	return &events
}

func captureSGPRequestObservations(provider *sgpProvider) *[]map[string]any {
	events := make([]map[string]any, 0, 4)
	provider.observe = func(event map[string]any) {
		if event["event"] == "sgp_request" {
			events = append(events, event)
		}
	}
	return &events
}

func assertSGPRequestObservation(t *testing.T, event map[string]any, route string, status int) {
	t.Helper()
	if event["event"] != "sgp_request" || event["route"] != route || event["http_status"] != status {
		t.Fatalf("unexpected SGP observation: %#v", event)
	}
	allowedKeys := map[string]bool{
		"event": true, "method": true, "route": true, "path": true,
		"http_status": true, "duration_ms": true, "retried": true, "token_kind": true, "body_bytes": true,
		"error_kind": true, "read_failed": true, "parse_failed": true, "payload_prefix_shape": true, "payload_sample_bytes": true,
		"start_index": true, "count": true, "tags": true,
		"credential_id":    true,
		"auth_error_class": true, "auth_error_classes": true, "auth_body_shape": true, "auth_body_truncated": true,
		"auth_json_valid": true, "auth_known_fields": true, "auth_field_details": true,
		"auth_unknown_field_count": true, "auth_unknown_code_count": true, "auth_depth_limited_count": true,
		"auth_unknown_field_shapes": true, "auth_unknown_field_size_buckets": true,
		"token_claims_verified": true, "token_has_outer_whitespace": true, "token_jwt_shape": true, "token_present": true,
	}
	for key := range event {
		if !allowedKeys[key] {
			t.Fatalf("SGP request observation used unreviewed field %q: %#v", key, event)
		}
	}
	for _, key := range []string{"duration_ms", "retried", "token_kind", "body_bytes", "path"} {
		if _, ok := event[key]; !ok {
			t.Fatalf("SGP observation missing %q: %#v", key, event)
		}
	}
}

func TestDiagnosticKeySetIsSortedAndValueFree(t *testing.T) {
	got := diagnosticKeySet(json.RawMessage(`{"z":"private","a":12,"m":{"player":"hidden"}}`))
	want := []string{"a", "m", "z"}
	if len(got) != len(want) {
		t.Fatalf("keys = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("keys = %#v, want %#v", got, want)
		}
	}
	if diagnosticKeySet(json.RawMessage(`[]`)) != nil || diagnosticKeySet(json.RawMessage(`invalid`)) != nil {
		t.Fatal("non-object JSON produced diagnostic keys")
	}
}

func TestSGPConnectionErrorsHaveStableKinds(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want string
	}{
		{name: "timeout", err: context.DeadlineExceeded, want: "timeout"},
		{name: "dns", err: &net.DNSError{Err: "private lookup detail", Name: "private.example"}, want: "dns"},
		{name: "connection refused", err: syscall.ECONNREFUSED, want: "connection_refused"},
		{name: "other", err: errors.New("private transport detail"), want: "other"},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &LCUClient{}
			provider := newSGPProvider()
			var waits []time.Duration
			provider.retryWait = func(ctx context.Context, d time.Duration) error { waits = append(waits, d); return ctx.Err() }
			provider.http = &http.Client{Transport: sgpRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, test.err
			})}
			provider.token = "test-entitlements"
			provider.tokenAt = time.Now()
			provider.tokenClient = client
			observed := captureSGPObservations(provider)
			var payload map[string]any
			err := provider.getJSON(context.Background(), client, "HN1", "RANKED", "/ranked/{player}", "https://ranked.invalid/private-player", &payload)
			if len(waits) != 2 || waits[0] != 250*time.Millisecond || waits[1] != 750*time.Millisecond {
				t.Fatalf("retry delays=%v", waits)
			}
			if err == nil || len(*observed) != 3 {
				t.Fatalf("error=%v observations=%#v", err, *observed)
			}
			for index, event := range *observed {
				assertSGPRequestObservation(t, event, "RANKED", 0)
				if event["error_kind"] != test.want || event["retried"] != (index > 0) {
					t.Fatalf("observation %d = %#v, want kind %q", index, event, test.want)
				}
			}
			encoded, _ := json.Marshal(observed)
			if strings.Contains(string(encoded), "private") {
				t.Fatalf("connection diagnostic leaked transport details: %s", encoded)
			}
		})
	}
}

func TestSGPCanceledRequestIsNotRetriedAndHasCanceledKind(t *testing.T) {
	client := &LCUClient{}
	provider := newSGPProvider()
	provider.http = &http.Client{Transport: sgpRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, context.Canceled
	})}
	provider.token, provider.tokenAt, provider.tokenClient = "test-entitlements", time.Now(), client
	observed := captureSGPObservations(provider)
	var payload map[string]any
	err := provider.getJSON(context.Background(), client, "HN1", "RANKED", "/ranked/{player}", "https://ranked.invalid/player", &payload)
	if !errors.Is(err, context.Canceled) || len(*observed) != 1 || (*observed)[0]["error_kind"] != "canceled" || (*observed)[0]["retried"] != false {
		t.Fatalf("canceled request = error:%v observations:%#v", err, *observed)
	}
}

func TestEntitlementsTokenErrorsDistinguishStatusAndEmptyField(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "missing endpoint", status: http.StatusNotFound, body: `{"error":"missing"}`, want: "HTTP 404"},
		{name: "empty token", status: http.StatusOK, body: `{}`, want: "accessToken 字段为空"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
			_, err := newSGPProvider().entitlementsToken(client, false)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestMatchHistoryReturnsPartialErrorWhenLaterPageFails(t *testing.T) {
	requests := 0
	puuid := strings.Repeat("p", 48)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/match-history-query/v1/products/lol/player/"+puuid+"/SUMMARY" {
			t.Fatalf("path = %q, want player history route", r.URL.Path)
		}
		if requests > 1 {
			http.Error(w, "temporary failure", http.StatusBadGateway)
			return
		}
		games := make([]map[string]any, 0, sgpPageSize)
		for index := 0; index < sgpPageSize; index++ {
			games = append(games, map[string]any{"json": map[string]any{"gameId": index + 1, "participants": []map[string]any{{"participantId": 1}}}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"games": games})
	}))
	defer server.Close()

	client := &LCUClient{}
	provider := newSGPProvider()
	provider.http = server.Client()
	provider.serverBases["HN1"] = server.URL
	provider.token = "test-entitlements"
	provider.tokenAt = time.Now()
	provider.tokenClient = client
	observed := captureSGPObservations(provider)

	games, consumed, more, err := provider.matchHistoryOn(context.Background(), client, "HN1", puuid, 0, sgpPageSize+1, false)
	var partial *sgpPartialHistoryError
	if !errors.As(err, &partial) || len(games) != sgpPageSize || consumed != sgpPageSize || !more || requests != 4 {
		t.Fatalf("games=%d consumed=%d more=%v requests=%d error=%v", len(games), consumed, more, requests, err)
	}
	if len(*observed) != 5 {
		t.Fatalf("observations = %#v", *observed)
	}
	assertSGPRequestObservation(t, (*observed)[0], "SUMMARY", http.StatusOK)
	if (*observed)[1]["event"] != "sgp_participant_keys" || strings.Join((*observed)[1]["keys"].([]string), ",") != "participantId" {
		t.Fatalf("participant keys = %#v", (*observed)[1])
	}
	for _, event := range (*observed)[2:] {
		assertSGPRequestObservation(t, event, "SUMMARY", http.StatusBadGateway)
	}
	encoded, _ := json.Marshal(observed)
	if strings.Contains(string(encoded), puuid) || strings.Contains(string(encoded), "temporary failure") {
		t.Fatalf("SGP diagnostics leaked an identifier or response body: %s", encoded)
	}
}

func TestMatchHistoryParsesFullRosterFromPlayerRoute(t *testing.T) {
	puuid := strings.Repeat("r", 48)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/match-history-query/v1/products/lol/player/"+puuid+"/SUMMARY" {
			t.Fatalf("path = %q, want player history route", r.URL.Path)
		}
		participants := make([]map[string]any, 10)
		for index := range participants {
			participants[index] = map[string]any{"participantId": index + 1, "puuid": strings.Repeat(string(rune('a'+index)), 48)}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"games": []map[string]any{{"json": map[string]any{
			"gameId": 71, "queueId": 420, "gameMode": "CLASSIC", "participants": participants,
		}}}})
	}))
	defer server.Close()

	client := &LCUClient{}
	provider := newSGPProvider()
	provider.http = server.Client()
	provider.serverBases["HN1"] = server.URL
	provider.token = "test-entitlements"
	provider.tokenAt = time.Now()
	provider.tokenClient = client
	observed := captureSGPObservations(provider)

	games, consumed, more, err := provider.matchHistoryOn(context.Background(), client, "HN1", puuid, 0, 20, false)
	if err != nil || len(games) != 1 || len(games[0].Participants) != 10 || consumed != 1 || more {
		t.Fatalf("games=%#v consumed=%d more=%v error=%v", games, consumed, more, err)
	}
	if len(*observed) != 2 {
		t.Fatalf("observations = %#v", *observed)
	}
	assertSGPRequestObservation(t, (*observed)[0], "SUMMARY", http.StatusOK)
	if (*observed)[1]["event"] != "sgp_participant_keys" || strings.Join((*observed)[1]["keys"].([]string), ",") != "participantId,puuid" {
		t.Fatalf("participant keys = %#v", (*observed)[1])
	}
}

func TestMatchHistoryCacheAvoidsDuplicateSummaryAndReportsHit(t *testing.T) {
	requests := 0
	puuid := strings.Repeat("h", 48)
	responseBody := `{"games":[{"json":{"gameId":91,"queueId":420,"participants":[{"participantId":1}]}}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(responseBody))
	}))
	defer server.Close()
	client := &LCUClient{}
	provider := newSGPProvider()
	provider.http = server.Client()
	provider.serverBases["HN1"] = server.URL
	provider.token = "test-entitlements"
	provider.tokenAt = time.Now()
	provider.tokenClient = client
	cost := &overviewLoadCost{}
	ctx := context.WithValue(context.Background(), overviewLoadCostContextKey{}, cost)
	if _, _, _, err := provider.matchHistoryOn(ctx, client, "HN1", puuid, 0, 20, true); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := provider.matchHistoryOn(ctx, client, "HN1", puuid, 0, 20, true); err != nil {
		t.Fatal(err)
	}
	requestsMade, bytesReceived, historyCalls, historyCacheHits := cost.snapshot()
	if requests != 1 || requestsMade != 1 || bytesReceived != len(responseBody) || historyCalls != 2 || historyCacheHits != 1 {
		t.Fatalf("requests=%d tracked=%d bytes=%d historyCalls=%d historyCacheHits=%d", requests, requestsMade, bytesReceived, historyCalls, historyCacheHits)
	}
}

func TestSGPRetriesTransient503TwiceThenSucceeds(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		if requests < 3 {
			http.Error(w, "temporary", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	client := &LCUClient{}
	provider := newSGPProvider()
	provider.http = server.Client()
	provider.token, provider.tokenAt, provider.tokenClient = "test-entitlements", time.Now(), client
	observed := captureSGPObservations(provider)
	var payload map[string]any
	if err := provider.getJSON(context.Background(), client, "HN1", "SUMMARY", "/match-history-query/v1/products/lol/player/{puuid}/SUMMARY", server.URL+"?startIndex=40&count=10&tag=q_420", &payload); err != nil {
		t.Fatalf("retry should recover from SGP 网关返回 HTTP 503: %v", err)
	}
	if requests != 3 || payload["ok"] != true || len(*observed) != 3 {
		t.Fatalf("requests=%d payload=%#v observations=%#v", requests, payload, *observed)
	}
	for index, event := range *observed {
		wantStatus := http.StatusServiceUnavailable
		if index == 2 {
			wantStatus = http.StatusOK
		}
		assertSGPRequestObservation(t, event, "SUMMARY", wantStatus)
		if event["retried"] != (index > 0) || event["start_index"] != 40 || event["count"] != 10 || event["tags"] != "q_420" {
			t.Fatalf("retry/page diagnostic %d = %#v", index, event)
		}
	}
}

func TestSGPRetryBudgetDoesNotConsumeAuthRefresh(t *testing.T) {
	tokenRequests := 0
	lcuServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		tokenRequests++
		_, _ = io.WriteString(w, `{"accessToken":"refreshed-entitlements"}`)
	}))
	defer lcuServer.Close()
	requests := 0
	sgpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		switch requests {
		case 1, 2:
			http.Error(w, "temporary", http.StatusServiceUnavailable)
		case 3:
			http.Error(w, "expired", http.StatusUnauthorized)
		default:
			_, _ = io.WriteString(w, `{"ok":true}`)
		}
	}))
	defer sgpServer.Close()
	client := &LCUClient{baseURL: lcuServer.URL, token: "lcu-token", http: lcuServer.Client()}
	provider := newSGPProvider()
	provider.http = sgpServer.Client()
	provider.token, provider.tokenAt, provider.tokenClient = "expired-entitlements", time.Now(), client
	observed := captureSGPRequestObservations(provider)
	var payload map[string]any
	if err := provider.getJSON(context.Background(), client, "HN1", "RANKED", "/ranked/{player}", sgpServer.URL, &payload); err != nil {
		t.Fatal(err)
	}
	if requests != 4 || tokenRequests != 1 || payload["ok"] != true || len(*observed) != 4 || (*observed)[3]["retried"] != true {
		t.Fatalf("independent budgets = requests:%d tokenRequests:%d payload:%#v observations:%#v", requests, tokenRequests, payload, *observed)
	}
}

func TestSGPRetriesReadFailureTwiceThenSucceeds(t *testing.T) {
	requests := 0
	client := &LCUClient{}
	provider := newSGPProvider()
	provider.http = &http.Client{Transport: sgpRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		body := io.ReadCloser(io.NopCloser(strings.NewReader(`{"ok":true}`)))
		if requests < 3 {
			body = io.NopCloser(sgpErrorReader{})
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body, Request: request}, nil
	})}
	provider.token, provider.tokenAt, provider.tokenClient = "test-entitlements", time.Now(), client
	observed := captureSGPObservations(provider)
	var payload map[string]any
	if err := provider.getJSON(context.Background(), client, "HN1", "RANKED", "/ranked/{player}", "https://ranked.invalid/player", &payload); err != nil {
		t.Fatal(err)
	}
	if requests != 3 || payload["ok"] != true || len(*observed) != 3 || (*observed)[0]["read_failed"] != true || (*observed)[1]["read_failed"] != true || (*observed)[2]["retried"] != true {
		t.Fatalf("read retries = requests:%d payload:%#v observations:%#v", requests, payload, *observed)
	}
}

func TestMatchHistoryPhysicalPageCacheReusesTwentyBeforeFifty(t *testing.T) {
	requests := make([]string, 0, 3)
	puuid := strings.Repeat("c", 48)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start, _ := strconv.Atoi(r.URL.Query().Get("startIndex"))
		count, _ := strconv.Atoi(r.URL.Query().Get("count"))
		requests = append(requests, fmt.Sprintf("%d:%d", start, count))
		games := make([]map[string]any, 0, count)
		for index := 0; index < count; index++ {
			games = append(games, map[string]any{"json": map[string]any{"gameId": start + index + 1, "participants": []map[string]any{{"participantId": 1}}}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"games": games})
	}))
	defer server.Close()
	client := &LCUClient{}
	provider := newSGPProvider()
	provider.http = server.Client()
	provider.serverBases["HN1"] = server.URL
	provider.token, provider.tokenAt, provider.tokenClient = "test-entitlements", time.Now(), client
	if _, _, _, err := provider.matchHistoryOn(context.Background(), client, "HN1", puuid, 0, 20, true); err != nil {
		t.Fatal(err)
	}
	games, consumed, _, err := provider.matchHistoryOn(context.Background(), client, "HN1", puuid, 0, 50, true)
	if err != nil || len(games) != 50 || consumed != 50 {
		t.Fatalf("games=%d consumed=%d err=%v", len(games), consumed, err)
	}
	want := []string{"0:20", "20:30"}
	if strings.Join(requests, ",") != strings.Join(want, ",") {
		t.Fatalf("physical page requests = %v, want %v (second logical request must add exactly 1 upstream call)", requests, want)
	}
}

func TestMatchHistoryPhysicalPageCacheSeparatesPageSizesAtSameStart(t *testing.T) {
	provider := newSGPProvider()
	puuid := strings.Repeat("p", 48)
	tags := []string{"q_420"}
	provider.cacheHistoryPage("HN1", puuid, 0, 20, tags, sgpHistoryCacheEntry{consumed: 20, more: true})
	provider.cacheHistoryPage("HN1", puuid, 0, 50, tags, sgpHistoryCacheEntry{consumed: 50, more: false})

	if len(provider.historyCache) != 2 {
		t.Fatalf("physical cache entries = %d, want 2 (pageSize must be part of the key)", len(provider.historyCache))
	}
	small, smallPageSize, ok := provider.cachedHistoryPage("HN1", puuid, 0, 20, tags)
	if !ok || smallPageSize != 20 || small.consumed != 20 || !small.more {
		t.Fatalf("20-row physical page = entry:%#v pageSize:%d ok:%t", small, smallPageSize, ok)
	}
	large, largePageSize, ok := provider.cachedHistoryPage("HN1", puuid, 0, 50, tags)
	if !ok || largePageSize != 50 || large.consumed != 50 || large.more {
		t.Fatalf("50-row physical page = entry:%#v pageSize:%d ok:%t", large, largePageSize, ok)
	}
}

func TestMatchHistoryColdFiftyUsesOneUpstreamRequestAndOneShapeSample(t *testing.T) {
	requests := 0
	puuid := strings.Repeat("f", 48)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Get("startIndex") != "0" || r.URL.Query().Get("count") != "50" {
			t.Fatalf("query = %q", r.URL.RawQuery)
		}
		games := make([]map[string]any, 0, 50)
		for index := 0; index < 50; index++ {
			games = append(games, map[string]any{"json": map[string]any{"gameId": index + 1, "participants": []map[string]any{{"participantId": 1, "teamId": 100}}}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"games": games})
	}))
	defer server.Close()
	client := &LCUClient{}
	provider := newSGPProvider()
	provider.http = server.Client()
	provider.serverBases["HN1"] = server.URL
	provider.token, provider.tokenAt, provider.tokenClient = "test-entitlements", time.Now(), client
	observed := captureSGPObservations(provider)
	if games, _, _, err := provider.matchHistoryOn(context.Background(), client, "HN1", puuid, 0, 50, false); err != nil || len(games) != 50 || requests != 1 {
		t.Fatalf("games=%d requests=%d err=%v", len(games), requests, err)
	}
	shapeEvents := 0
	for _, event := range *observed {
		if event["event"] == "sgp_participant_keys" {
			shapeEvents++
			if event["sampled"] != true {
				t.Fatalf("shape sample missing sampled=true: %#v", event)
			}
		}
	}
	if shapeEvents != 1 {
		t.Fatalf("sgp_participant_keys events=%d, want exactly 1 for 50 matches: %#v", shapeEvents, *observed)
	}
}

func TestMatchHistoryCacheEvictsLeastRecentlyUsed(t *testing.T) {
	puuid := strings.Repeat("l", 48)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"games":[]}`))
	}))
	defer server.Close()
	client := &LCUClient{}
	provider := newSGPProvider()
	provider.http = server.Client()
	provider.serverBases["HN1"] = server.URL
	provider.token, provider.tokenAt, provider.tokenClient = "test-entitlements", time.Now(), client
	insertedAt := time.Now().Add(-time.Minute)
	cacheKey := func(start int) string {
		return sgpHistoryPageCacheKey("HN1", puuid, start, 20, nil)
	}
	provider.mu.Lock()
	for start := 0; start < sgpCacheMax; start++ {
		provider.historyCache[cacheKey(start)] = sgpHistoryCacheEntry{
			at: insertedAt, lastUsed: insertedAt.Add(time.Duration(start) * time.Millisecond), consumed: 1,
		}
	}
	provider.mu.Unlock()

	if _, _, _, err := provider.matchHistoryOn(context.Background(), client, "HN1", puuid, 0, 20, true); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := provider.matchHistoryOn(context.Background(), client, "HN1", puuid, sgpCacheMax, 20, true); err != nil {
		t.Fatal(err)
	}
	provider.mu.Lock()
	_, touchedPresent := provider.historyCache[cacheKey(0)]
	_, leastRecentlyUsedPresent := provider.historyCache[cacheKey(1)]
	_, insertedPresent := provider.historyCache[cacheKey(sgpCacheMax)]
	cacheSize := len(provider.historyCache)
	provider.mu.Unlock()
	if !touchedPresent || leastRecentlyUsedPresent || !insertedPresent || cacheSize != sgpCacheMax {
		t.Fatalf("LRU eviction failed: touched=%v oldest=%v inserted=%v size=%d", touchedPresent, leastRecentlyUsedPresent, insertedPresent, cacheSize)
	}
}

func TestMatchHistoryFilteredOnSendsRepeatedTagsWithOR(t *testing.T) {
	requests := 0
	puuid := strings.Repeat("m", 48)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/match-history-query/v1/products/lol/player/"+puuid+"/SUMMARY" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		query := r.URL.Query()
		if got := strings.Join(query["tag"], ","); got != "q_2300,q_2400" {
			t.Fatalf("tags = %q", got)
		}
		if query.Get("tagsQueryType") != "OR" || query.Get("startIndex") != "0" || query.Get("count") != "20" {
			t.Fatalf("query = %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"games":[{"json":{"gameId":81,"queueId":2300,"participants":[]}}]}`))
	}))
	defer server.Close()
	client := &LCUClient{}
	provider := newSGPProvider()
	provider.http = server.Client()
	provider.serverBases["HN1"] = server.URL
	provider.token, provider.tokenAt, provider.tokenClient = "test-entitlements", time.Now(), client
	games, _, _, err := provider.matchHistoryFilteredOn(context.Background(), client, "HN1", puuid, 0, 20, []string{"q_2300", "q_2400"}, false)
	if err != nil || len(games) != 1 || requests != 1 {
		t.Fatalf("games=%d requests=%d error=%v", len(games), requests, err)
	}
}

func TestMatchHistoryFilteredOnDoesNotRefillMalformedTaggedPage(t *testing.T) {
	requests := 0
	puuid := strings.Repeat("v", 48)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Get("startIndex") != "0" || r.URL.Query().Get("count") != "20" {
			t.Fatalf("query = %q", r.URL.RawQuery)
		}
		games := make([]map[string]any, 20)
		for index := range games {
			games[index] = map[string]any{"json": "not-an-object"}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"games": games})
	}))
	defer server.Close()
	client := &LCUClient{}
	provider := newSGPProvider()
	provider.http = server.Client()
	provider.serverBases["HN1"] = server.URL
	provider.token, provider.tokenAt, provider.tokenClient = "test-entitlements", time.Now(), client
	games, consumed, more, err := provider.matchHistoryFilteredOn(context.Background(), client, "HN1", puuid, 0, 20, []string{"q_420"}, false)
	if err != nil || requests != 1 || len(games) != 0 || consumed != 20 || !more {
		t.Fatalf("requests=%d games=%d consumed=%d more=%v error=%v", requests, len(games), consumed, more, err)
	}
}

func TestMatchHistoryKeepsEmptyRosterForDiagnostics(t *testing.T) {
	puuid := strings.Repeat("e", 48)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/match-history-query/v1/products/lol/player/"+puuid+"/SUMMARY" {
			t.Fatalf("path = %q, want player history route", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"games": []map[string]any{{"json": map[string]any{
			"gameId": 72, "queueId": 420, "gameMode": "CLASSIC", "participants": []any{},
		}}}})
	}))
	defer server.Close()

	client := &LCUClient{}
	provider := newSGPProvider()
	provider.http = server.Client()
	provider.serverBases["HN1"] = server.URL
	provider.token = "test-entitlements"
	provider.tokenAt = time.Now()
	provider.tokenClient = client
	observed := captureSGPRequestObservations(provider)

	games, consumed, more, err := provider.matchHistoryOn(context.Background(), client, "HN1", puuid, 0, 20, false)
	if err != nil || len(games) != 1 || len(games[0].Participants) != 0 || consumed != 1 || more {
		t.Fatalf("games=%#v consumed=%d more=%v error=%v", games, consumed, more, err)
	}
	if len(*observed) != 1 {
		t.Fatalf("observations = %#v", *observed)
	}
	assertSGPRequestObservation(t, (*observed)[0], "SUMMARY", http.StatusOK)
}

func TestSGPRequestObservationTracksRetryAndSafeParseShape(t *testing.T) {
	lcuServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/entitlements/v1/token" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"accessToken":"refreshed-entitlements"}`))
	}))
	defer lcuServer.Close()
	requests := 0
	sgpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		if requests == 1 {
			http.Error(w, "expired secret", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html>private upstream message</html>"))
	}))
	defer sgpServer.Close()
	client := &LCUClient{baseURL: lcuServer.URL, token: "lcu-secret", http: lcuServer.Client()}
	provider := newSGPProvider()
	provider.http = sgpServer.Client()
	provider.serverBases["HN1"] = sgpServer.URL
	provider.token = "expired-entitlements"
	provider.tokenAt = time.Now()
	provider.tokenClient = client
	observed := captureSGPRequestObservations(provider)

	_, _, _, err := provider.matchHistoryOn(context.Background(), client, "HN1", strings.Repeat("z", 48), 0, 20, false)
	if err == nil || len(*observed) != 2 {
		t.Fatalf("error=%v observations=%#v", err, *observed)
	}
	assertSGPRequestObservation(t, (*observed)[0], "SUMMARY", http.StatusUnauthorized)
	assertSGPRequestObservation(t, (*observed)[1], "SUMMARY", http.StatusOK)
	if (*observed)[0]["retried"] != false || (*observed)[1]["retried"] != true || (*observed)[1]["parse_failed"] != true || (*observed)[1]["payload_prefix_shape"] != "html" {
		t.Fatalf("retry/parse fields = %#v", *observed)
	}
	encoded, _ := json.Marshal(observed)
	if strings.Contains(string(encoded), "private upstream message") || strings.Contains(string(encoded), "expired-entitlements") {
		t.Fatalf("SGP diagnostics leaked a response or token: %s", encoded)
	}
}

func TestSGPTimelinePayloadReportsWrapperAndEventTypes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/match-history-query/v1/products/lol/HN1_73/DETAILS" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"json":{"frames":[{"events":[{"eventType":"ITEM_PURCHASED"},{"type":"SKILL_LEVEL_UP"},{"type":"ITEM_PURCHASED"}]}]}}`))
	}))
	defer server.Close()
	client := &LCUClient{}
	provider := newSGPProvider()
	provider.http = server.Client()
	provider.serverBases["HN1"] = server.URL
	provider.token = "test-entitlements"
	provider.tokenAt = time.Now()
	provider.tokenClient = client
	observed := captureSGPObservations(provider)

	frames, err := provider.gameDetailsOn(context.Background(), client, "HN1", 73)
	if err != nil || len(frames) != 1 || len(*observed) != 2 {
		t.Fatalf("frames=%#v err=%v observations=%#v", frames, err, *observed)
	}
	assertSGPRequestObservation(t, (*observed)[0], "DETAILS", http.StatusOK)
	payload := (*observed)[1]
	if payload["event"] != "sgp_timeline_payload" || payload["frames_wrapped"] != "JSON.Frames" || payload["frame_count"] != 1 || payload["event_count"] != 3 {
		t.Fatalf("timeline payload observation = %#v", payload)
	}
	allowedKeys := map[string]bool{
		"event": true, "route": true, "http_status": true, "frames_wrapped": true,
		"frame_count": true, "event_count": true, "event_types": true,
	}
	if len(payload) != len(allowedKeys) {
		t.Fatalf("timeline payload observation has unreviewed fields: %#v", payload)
	}
	for key := range payload {
		if !allowedKeys[key] {
			t.Fatalf("timeline payload observation used unreviewed field %q: %#v", key, payload)
		}
	}
	types, ok := payload["event_types"].([]string)
	if !ok || len(types) != 2 || types[0] != "ITEM_PURCHASED" || types[1] != "SKILL_LEVEL_UP" {
		t.Fatalf("timeline event types = %#v", payload["event_types"])
	}
}

func TestSGPRankedAndSummonerRoutesReportSessionToken(t *testing.T) {
	puuid := strings.Repeat("q", 48)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/leagues-ledge/"):
			_, _ = w.Write([]byte(`{"owner":"private-player-name","queues":[{"queueType":"RANKED_SOLO_5x5","tier":"DIAMOND","wins":12,"losses":8,"opaque":"private-ranked-value"},{"queueType":"RANKED_FLEX_SR","division":"I","leaguePoints":42}]}`))
		case strings.HasPrefix(r.URL.Path, "/summoner-ledge/"):
			_, _ = w.Write([]byte(`[{"name":"public"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &LCUClient{}
	provider := newSGPProvider()
	provider.http = server.Client()
	provider.serverBases["HN1"] = server.URL
	provider.sessionToken = "test-session"
	provider.sessionAt = time.Now()
	provider.sessionOwner = client
	observed := captureSGPObservations(provider)

	if _, err := provider.rankedStatsOn(context.Background(), client, "HN1", puuid, true, "PUBLIC"); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.summonerByPUUIDOn(context.Background(), client, "HN1", puuid); err != nil {
		t.Fatal(err)
	}
	if len(*observed) != 3 {
		t.Fatalf("observations = %#v", *observed)
	}
	assertSGPRequestObservation(t, (*observed)[0], "RANKED", http.StatusOK)
	shape := (*observed)[1]
	bodyBytes, bodyBytesOK := shape["body_bytes"].(int)
	if shape["event"] != "sgp_ranked_stats_shape" || shape["is_self"] != true || shape["privacy"] != "PUBLIC" || shape["queue_count"] != 2 || !bodyBytesOK || bodyBytes <= 0 {
		t.Fatalf("ranked shape observation = %#v", shape)
	}
	if _, leaked := shape["server_id"]; leaked {
		t.Fatalf("ranked shape observation leaked server identity: %#v", shape)
	}
	allowedShapeKeys := []string{"event", "is_self", "privacy", "body_bytes", "queue_count", "top_level_keys", "queue_keys", "sample_queue_values"}
	if len(shape) != len(allowedShapeKeys) {
		t.Fatalf("ranked shape observation has unreviewed fields: %#v", shape)
	}
	for _, key := range allowedShapeKeys {
		if _, ok := shape[key]; !ok {
			t.Fatalf("ranked shape observation lacks %q: %#v", key, shape)
		}
	}
	if got, ok := shape["top_level_keys"].([]string); !ok || len(got) != 2 || got[0] != "owner" || got[1] != "queues" {
		t.Fatalf("top-level keys = %#v", shape["top_level_keys"])
	}
	if got, ok := shape["queue_keys"].([]string); !ok || strings.Join(got, ",") != "division,leaguePoints,losses,opaque,queueType,tier,wins" {
		t.Fatalf("queue keys = %#v", shape["queue_keys"])
	}
	samples, ok := shape["sample_queue_values"].(map[string]any)
	if !ok {
		t.Fatalf("ranked queue sample = %#v", shape["sample_queue_values"])
	}
	queueSample, ok := samples["queues"].(map[string]any)
	if !ok || queueSample["wins"] != float64(12) || queueSample["losses"] != float64(8) || queueSample["tier"] != "DIAMOND" {
		t.Fatalf("ranked queue sample = %#v", samples)
	}
	assertSGPRequestObservation(t, (*observed)[2], "SUMMONER", http.StatusOK)
	if (*observed)[0]["token_kind"] != "session" || (*observed)[2]["token_kind"] != "session" {
		t.Fatalf("token kinds = %#v", *observed)
	}
	encoded, _ := json.Marshal(observed)
	if strings.Contains(string(encoded), puuid) || strings.Contains(string(encoded), "private-player-name") || strings.Contains(string(encoded), "private-ranked-value") {
		t.Fatalf("ranked shape diagnostic leaked payload values: %s", encoded)
	}
}

func TestSGPRankedShapeObservationIsProcessScoped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"queues":[{"queueType":"RANKED_SOLO_5x5","tier":"GOLD","division":"II","wins":12,"losses":8}]}`))
	}))
	defer server.Close()
	client := &LCUClient{}
	provider := newSGPProvider()
	provider.http = server.Client()
	provider.serverBases["HN1"] = server.URL
	provider.sessionToken, provider.sessionAt, provider.sessionOwner = "session", time.Now(), client
	observed := captureSGPObservations(provider)
	for index := 0; index < 2; index++ {
		if _, err := provider.rankedStatsOn(context.Background(), client, "HN1", strings.Repeat("p", 48), false, "PUBLIC"); err != nil {
			t.Fatal(err)
		}
	}
	shapes := 0
	for _, event := range *observed {
		if event["event"] == "sgp_ranked_stats_shape" {
			shapes++
		}
	}
	if shapes != 1 {
		t.Fatalf("SGP ranked shape observations = %d, want 1: %#v", shapes, *observed)
	}
}

func TestSGPRankedStatsOnParsesObservedHistoricalShapes(t *testing.T) {
	fixtures := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "historical fields present",
			body: `{
				"highestPreviousSeasonEndTier":"PLATINUM",
				"highestPreviousSeasonEndRank":"I",
				"highestPreviousSeasonAchievedTier":"DIAMOND",
				"highestPreviousSeasonAchievedRank":"IV",
				"queues":[{
					"queueType":"RANKED_SOLO_5x5","tier":"EMERALD","rank":"II",
					"highestTier":"MASTER","highestRank":"I",
					"previousSeasonAchievedTier":"EMERALD","previousSeasonAchievedRank":"II",
					"previousSeasonEndTier":"EMERALD","previousSeasonEndRank":"I",
					"previousSeasonHighestTier":"DIAMOND","previousSeasonHighestRank":"IV"
				}]
			}`,
			want: true,
		},
		{
			name: "historical fields omitted",
			body: `{"queues":[{"queueType":"RANKED_SOLO_5x5","tier":"","rank":""}]}`,
		},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(fixture.body))
			}))
			defer server.Close()
			client := &LCUClient{}
			provider := newSGPProvider()
			provider.http = server.Client()
			provider.serverBases["HN1"] = server.URL
			provider.sessionToken = "test-session"
			provider.sessionAt = time.Now()
			provider.sessionOwner = client

			stats, err := provider.rankedStatsOn(context.Background(), client, "HN1", strings.Repeat("h", 48), false, "PUBLIC")
			if err != nil {
				t.Fatal(err)
			}
			milestones := gameplayRankMilestonesFromSGP(stats)
			if !fixture.want {
				if milestones != nil {
					t.Fatalf("omitted historical fields produced milestones: %#v", milestones)
				}
				return
			}
			if len(stats.Queues) != 1 || stats.Queues[0].HighestTier != "MASTER" || stats.Queues[0].PreviousSeasonAchievedTier != "EMERALD" || stats.Queues[0].PreviousSeasonAchievedRank != "II" || stats.Queues[0].PreviousSeasonEndTier != "EMERALD" || stats.Queues[0].PreviousSeasonHighestTier != "DIAMOND" {
				t.Fatalf("parsed ranked stats = %#v", stats)
			}
			if milestones == nil || milestones.PeakTier != "diamond" || len(milestones.PreviousSeason) != 1 || milestones.PreviousSeason[0].Tier != "emerald" || milestones.PreviousSeason[0].HighestTier != "diamond" {
				t.Fatalf("parsed milestones = %#v", milestones)
			}
		})
	}
}

func TestSGPSummonerSuccessIsCached(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(`[{"puuid":"cached","name":"public"}]`))
	}))
	defer server.Close()
	client := &LCUClient{}
	provider := newSGPProvider()
	provider.http = server.Client()
	provider.serverBases["HN1"] = server.URL
	provider.sessionToken = "test-session"
	provider.sessionAt = time.Now()
	provider.sessionOwner = client

	for range 2 {
		got, err := provider.summonerByPUUIDOn(context.Background(), client, "HN1", strings.Repeat("q", 48))
		if err != nil || got.Name != "public" {
			t.Fatalf("summoner=%#v err=%v", got, err)
		}
	}
	if requests != 1 {
		t.Fatalf("SUMMONER requests = %d, want 1", requests)
	}
}

func TestMatchHistoryCacheEvictsByBytes(t *testing.T) {
	provider := newSGPProvider()
	puuid := strings.Repeat("b", 48)
	for start := 0; start < 3; start++ {
		provider.cacheHistoryPage("HN1", puuid, start, 50, nil, sgpHistoryCacheEntry{
			consumed: 50,
			bytes:    10 << 20,
		})
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if len(provider.historyCache) != 2 {
		t.Fatalf("history cache entries = %d, want 2", len(provider.historyCache))
	}
	if provider.historyBytes > sgpCacheMaxBytes {
		t.Fatalf("history cache bytes = %d, want <= %d", provider.historyBytes, sgpCacheMaxBytes)
	}
	if _, ok := provider.historyCache[sgpHistoryPageCacheKey("HN1", puuid, 0, 50, nil)]; ok {
		t.Fatal("oldest byte-heavy history page was not evicted")
	}
}
