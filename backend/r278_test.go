package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestR278P3NeverReferenceEmptyRevision(t *testing.T) {
	seen := map[int64]string{1: ""}
	wire := overviewWireFrame(gameplayOverview{Matches: []gameplayMatch{{GameID: 1}}}, "complete", seen)
	b, _ := json.Marshal(wire)
	var v struct {
		Matches []map[string]any `json:"matches"`
	}
	json.Unmarshal(b, &v)
	if len(v.Matches[0]) <= 2 {
		t.Fatalf("empty revision reference: %s", b)
	}
}
func TestR278P2Official503AndOnlyBackgroundProbe(t *testing.T) {
	if !historyOfficialOutage([]DataSourceAttempt{{Source: dataSourceSGP, StatusCode: 503}}, 0) {
		t.Fatal("fresh SGP 503 must be official")
	}
	p := &sgpProvider{historyClock: func() time.Time { return time.Unix(100, 0) }, historyCircuits: map[string]*sgpHistoryCircuit{"HN10": {until: time.Unix(200, 0), nextProbe: time.Unix(50, 0)}}}
	_, err := p.acquireHistoryCircuit("HN10", "SUMMARY")
	if !errors.Is(err, errSGPHistoryCooling) {
		t.Fatal("overview must not become circuit probe")
	}
	_ = context.Background()
}
func TestR278P13ProbeBudget(t *testing.T) {
	if updateProbeTimeout > 2*time.Second {
		t.Fatalf("probe budget %s", updateProbeTimeout)
	}
}
func TestR278P7WholePageQuota(t *testing.T) {
	now := time.Unix(1000, 0)
	p := &riotProvider{limitNow: func() time.Time { return now }}
	for i := 0; i < 70; i++ {
		p.longWindow = append(p.longWindow, now)
	}
	if release, err := p.reserveOverviewPage(context.Background(), 20); err == nil {
		release()
		t.Fatal("remaining <21 must wait")
	}
	now = now.Add(2 * time.Minute)
	release, err := p.reserveOverviewPage(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	release()
}
