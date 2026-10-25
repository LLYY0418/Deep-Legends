package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

var r309Thresholds = []int{100, 900, 1800, 2900, 4200, 5700, 7400, 9300, 11400, 13800, 16800, 20800}

func r309Event(fame int, thresholds []int) LCUEvent {
	milestones := []map[string]int{}
	for _, v := range thresholds {
		milestones = append(milestones, map[string]int{"triggerValue": v})
	}
	raw, _ := json.Marshal(map[string]any{"counters": []map[string]int{{"counterValue": fame}}, "milestones": milestones})
	return LCUEvent{URI: "/lol-progression/v1/groups/" + arenaFameEventGroup, Data: raw}
}
func TestR309FameMilestonesPersistSelfAndDelta(t *testing.T) {
	a := r175App(t)
	a.summoner = Summoner{PUUID: "r309-own-private"}
	c := &LCUClient{region: "TENCENT", rsoPlatform: "HN1"}
	a.bindArenaFameEvents(c)
	for _, tc := range []struct{ fame, level int }{{0, 0}, {4199, 4}, {5000, 5}, {94122, 12}, {94252, 12}} {
		c.observeArenaProgressionEvent(r309Event(tc.fame, r309Thresholds))
		got := a.ownArenaFame(c)
		if got == nil || got.Fame != tc.fame || got.Level != tc.level || got.Tier != arenaTierForLevel(tc.level) {
			t.Fatalf("%+v want %+v", got, tc)
		}
	}
	records := readDiagnosticEvents(t, a.storage, "arena_fame_delta")
	if len(records) != 4 {
		t.Fatalf("delta count %d", len(records))
	}
	last := records[len(records)-1]
	if fmt.Sprint(last["delta"]) != "130" || last["baseline_source"] != "previous_event" {
		t.Fatal(last)
	}
	restart := &app{storage: a.storage, summoner: a.summoner}
	got := restart.ownArenaFame(c)
	if got == nil || got.Level != 12 || got.LastDelta == nil || *got.LastDelta != 130 {
		t.Fatal(got)
	}
	if v := a.loadLiveArenaFame(context.Background(), nil, gameplayReference{}, "r309-other-private", false, ""); v != nil {
		t.Fatal("other consumed self fame")
	}
	restart.summoner.PUUID = "different-account"
	if restart.ownArenaFame(c) != nil {
		t.Fatal("cross-account fame")
	}
	c.observeArenaProgressionEvent(r309Event(99999, nil))
	if got := a.ownArenaFame(c); got.Fame != 94252 {
		t.Fatal("missing thresholds accepted")
	}
}
func TestR309OtherScanOnlyKeysAndChallengesFailureOnce(t *testing.T) {
	raw := json.RawMessage(`{"puuid":"SECRET-ID","summonerId":987654321,"arena":{"prestige":94252,"fameId":"PRIVATE-VALUE","progression":{"level":12}},"123456789":{"fame":8},"abc123":{"fame":9},"challenge-12345":{"prestige":10}}`)
	paths := arenaFameOtherPaths(raw)
	encoded, _ := json.Marshal(paths)
	for _, secret := range []string{"SECRET-ID", "987654321", "94252", "PRIVATE-VALUE", "123456789", "fameId", "abc123", "challenge-12345"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatal(string(encoded))
		}
	}
	if !strings.Contains(string(encoded), "arena.progression") {
		t.Fatal(paths)
	}
	if !strings.Contains(string(encoded), "{key}.fame") {
		t.Fatal("dynamic maps must remain searchable without their IDs", paths)
	}
	a := r175App(t)
	c := &LCUClient{baseURL: "https://fixture", token: "test"}
	var calls atomic.Int64
	c.http = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.Method != "GET" {
			t.Fatal(r.Method)
		}
		return r206RelayResponse(404, nil), nil
	})}
	a.scanOtherArenaFame(c, "PRIVATE-REF", "lcu", raw)
	a.scanOtherArenaFame(c, "SECOND-REF", "lcu", raw)
	a.probeOtherArenaChallenges(context.Background(), c, "PRIVATE-REF")
	a.probeOtherArenaChallenges(context.Background(), c, "PRIVATE-REF")
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
	events := readDiagnosticEvents(t, a.storage, "arena_fame_other_scan")
	if len(events) != 1 {
		t.Fatal(len(events))
	}
	logged, _ := json.Marshal(events)
	for _, secret := range []string{"PRIVATE-REF", "SECRET-ID", "94252", "987654321"} {
		if strings.Contains(string(logged), secret) {
			t.Fatal(string(logged))
		}
	}
}

func TestR309OtherLegacyShapeCannotLogResponseValues(t *testing.T) {
	a := r175App(t)
	a.recordArenaFameShape("lcu", false, true, json.RawMessage(`{"ratedRating":94252,"puuid":"SECRET-OTHER"}`), "source_unverified")
	if events := readDiagnosticEvents(t, a.storage, "arena_fame_source_shape"); len(events) != 0 {
		t.Fatal("other-player scalar diagnostics must be absent", events)
	}
}
func TestR309SelfLiveFallbackUsesPersistedEventOnly(t *testing.T) {
	a := r175App(t)
	a.summoner = Summoner{PUUID: r263Ref(1)}
	c := &LCUClient{baseURL: "https://fixture", token: "test", region: "TENCENT", rsoPlatform: "HN1", sessionContext: context.Background()}
	c.arenaDiscoveryState().started = true
	c.http = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		return r206RelayResponse(200, []byte(`{"queues":[]}`)), nil
	})}
	if got := a.loadLiveArenaFame(context.Background(), c, gameplayReference{}, r263Ref(1), true, ""); got != nil {
		t.Fatal("invented fame without an event", got)
	}
	a.observeOwnArenaFame(c, r309Event(94252, r309Thresholds))
	got := a.loadLiveArenaFame(context.Background(), c, gameplayReference{}, r263Ref(1), true, "")
	if got == nil || got.Fame != 94252 || got.Level != 12 || got.Source != "progression_event" || got.ObservedAt.IsZero() || got.LastDelta != nil {
		t.Fatal(got)
	}
	other := &LCUClient{region: "TENCENT", rsoPlatform: "HN2"}
	if a.ownArenaFame(other) != nil {
		t.Fatal("cross-server fame")
	}
}
func TestR309Progression404CacheIsVersionAndPathScoped(t *testing.T) {
	a := r175App(t)
	c := &LCUClient{baseURL: "https://fixture", token: "test", gameVersion: "26.21"}
	calls := map[string]int{}
	var callsMu sync.Mutex
	snapshot := func() map[string]int {
		callsMu.Lock()
		defer callsMu.Unlock()
		copy := map[string]int{}
		for k, v := range calls {
			copy[k] = v
		}
		return copy
	}
	c.http = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		callsMu.Lock()
		calls[r.URL.Path]++
		callsMu.Unlock()
		return r206RelayResponse(404, nil), nil
	})}
	ops := []lcuHelpOperation{{Name: "configuration", Method: http.MethodGet, Path: "/lol-progression/v1/groups/configuration"}, {Name: "instance-data", Method: http.MethodGet, Path: "/lol-progression/v1/groups/{groupId}/instance-data"}, {Name: "cherry-inventory", Method: http.MethodGet, Path: "/lol-inventory/v1/cherry-inventory"}}
	for range 2 {
		a.readArenaProgressionSnapshot(context.Background(), c, ops, Summoner{})
	}
	if snapshot()[ops[0].Path] != 1 || snapshot()[ops[2].Path] != 1 {
		t.Fatal(snapshot())
	}
	c.gameVersion = "26.22"
	a.readArenaProgressionSnapshot(context.Background(), c, ops, Summoner{})
	if snapshot()[ops[0].Path] != 2 || snapshot()[ops[2].Path] != 2 {
		t.Fatal("new version must probe again", snapshot())
	}
	ops[0].Path = "/lol-progression/v2/groups/configuration"
	a.readArenaProgressionSnapshot(context.Background(), c, ops, Summoner{})
	if snapshot()[ops[0].Path] != 1 {
		t.Fatal("new path must probe", snapshot())
	}
}

func TestR309UnknownProgressionGroupOnlyReportsOnceAndKeepsSource(t *testing.T) {
	a := r175App(t)
	a.summoner = Summoner{PUUID: r263Ref(2)}
	c := &LCUClient{region: "TENCENT", rsoPlatform: "HN1"}
	a.observeOwnArenaFame(c, r309Event(5000, r309Thresholds))
	changed := r309Event(94252, r309Thresholds)
	changed.URI = "/lol-progression/v1/groups/00000000-1111-2222-3333-444444444444"
	for range 2 {
		a.observeOwnArenaFame(c, changed)
	}
	if got := a.ownArenaFame(c); got == nil || got.Fame != 5000 {
		t.Fatal("unknown group replaced verified source", got)
	}
	events := readDiagnosticEvents(t, a.storage, "arena_fame_group_changed")
	if len(events) != 1 {
		t.Fatal(events)
	}
	raw, _ := json.Marshal(events)
	if strings.Contains(string(raw), "94252") || strings.Contains(string(raw), "00000000") {
		t.Fatal(string(raw))
	}
	mismatch := readDiagnosticEvents(t, a.storage, "arena_fame_threshold_mismatch")
	if len(mismatch) != 1 {
		t.Fatal(mismatch)
	}
}
