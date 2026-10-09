package main

import (
	"testing"
	"time"
)

func TestR264OfficialOutageRequiresBothSourcesOrCooling(t *testing.T) {
	for _, tc := range []struct {
		attempts []DataSourceAttempt
		cooling  int
		want     bool
	}{
		{[]DataSourceAttempt{{Source: dataSourceSGP, StatusCode: 503}}, 0, false},
		{[]DataSourceAttempt{{Source: dataSourceLCU, StatusCode: 500}}, 0, false},
		{[]DataSourceAttempt{{Source: dataSourceSGP, StatusCode: 503}, {Source: dataSourceLCU, StatusCode: 404}}, 0, false},
		{[]DataSourceAttempt{{Source: dataSourceSGP, StatusCode: 503}, {Source: dataSourceLCU, StatusCode: 500}}, 0, true},
		{nil, 42, true},
	} {
		if got := historyOfficialOutage(tc.attempts, tc.cooling); got != tc.want {
			t.Fatalf("attempts=%v cooling=%d got=%v", tc.attempts, tc.cooling, got)
		}
	}
	now := time.Now()
	p := &sgpProvider{historyClock: func() time.Time { return now }, historyCircuits: map[string]*sgpHistoryCircuit{"HN1": {until: now.Add(17 * time.Second)}}}
	if got := p.historyRetryAfter("HN1"); got != 17 {
		t.Fatalf("remaining=%d", got)
	}
	now = now.Add(18 * time.Second)
	if got := p.historyRetryAfter("HN1"); got != 0 {
		t.Fatalf("expired=%d", got)
	}
	if got := p.historyRetryAfter("HN2"); got != 0 {
		t.Fatalf("other server=%d", got)
	}
}
