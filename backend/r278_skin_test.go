package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestR278P9DirectSkinEventDuringFallback(t *testing.T) {
	a, c, updates := r258ViewFixture()
	a.snapshotReady = true
	a.clientSessionEpoch = 1
	a.allSkins = []Skin{{ID: 1001, ChampionID: 1}, {ID: 1002, ChampionID: 1}}
	c.baseURL = "https://fixture.test"
	c.token = "fixture"
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var requests atomic.Int32
	c.http = &http.Client{Transport: r278Transport(func(r *http.Request) (*http.Response, error) {
		n := requests.Add(1)
		if n == 1 {
			close(started)
			select {
			case <-release:
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
		}
		if n == 2 {
			close(finished)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`[{"itemId":1001,"inventoryType":"CHAMPION_SKIN"}]`))}, nil
	})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.observeCollectionSkinEvent(ctx, c, LCUEvent{URI: "/lol-loot/v1/player-loot"})
	<-started
	a.observeCollectionSkinEvent(ctx, c, LCUEvent{URI: "/lol-inventory/v2/inventory/CHAMPION_SKIN", Data: json.RawMessage(`[{"itemId":1002,"inventoryType":"CHAMPION_SKIN"}]`)})
	select {
	case raw := <-updates:
		var row map[string]any
		if json.Unmarshal([]byte(raw), &row) != nil || row["type"] != "collection-new-skins" || row["count"] != float64(1) {
			t.Fatalf("unexpected notification %s", raw)
		}
	default:
		t.Fatal("direct skin notification waited for full inventory refresh")
	}
	a.observeCollectionSkinEvent(ctx, c, LCUEvent{URI: "/lol-loot/v1/player-loot"})
	close(release)
	<-finished
	<-updates // fallback publishes the other skin after the gate opens.
	if requests.Load() != 2 {
		t.Fatal("pending ownership event was dropped")
	}
}
func TestR278P13FastestSourceSwitchNeedsTwoTimesSpeed(t *testing.T) {
	r := &updateSourceRace{first: "first", sources: []string{"first", "second", "third"}, results: map[string]updateSourceProbe{"first": {ok: true, speed: 100}, "second": {ok: true, speed: 199}, "third": {ok: true, speed: 50}}}
	if got := r.ordered()[0]; got != "first" {
		t.Fatal("route switched before a two-times advantage")
	}
	r.results["second"] = updateSourceProbe{ok: true, speed: 201}
	if got := r.ordered()[0]; got != "second" {
		t.Fatal("faster route was not selected after segment boundary")
	}
}
func TestR278P13PredownloadYieldsToForeground(t *testing.T) {
	u := &updateManager{predownloading: true, foregroundBusy: func() bool { return true }}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if u.yieldPredownload(ctx) == nil {
		t.Fatal("background read did not yield")
	}
	u.predownloading = false
	if u.yieldPredownload(ctx) != nil {
		t.Fatal("foreground read throttled")
	}
}
