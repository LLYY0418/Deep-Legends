package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR288ProFreshnessAndFailureBackoff(t *testing.T) {
	for i, want := range []time.Duration{5 * time.Minute, 15 * time.Minute, time.Hour, 3 * time.Hour, 3 * time.Hour} {
		if got := proProfileTTL(opggProAccount{CheckFailed: true, Failures: i + 1}); got != want {
			t.Fatalf("failure %d TTL %s", i+1, got)
		}
	}
	if proProfileTTL(opggProAccount{}) != 3*time.Hour {
		t.Fatal("successful freshness not 3 hours")
	}
	now := time.Now()
	if proProfileDue(opggProAccount{CheckedAt: now.Add(-2 * time.Hour).Format(time.RFC3339Nano)}, now) {
		t.Fatal("fresh account reloaded")
	}
}
func TestR288ProFirstScreenIsLocalAndScopedPlanStaysVisible(t *testing.T) {
	a := r175App(t)
	started := time.Now()
	teams, _, _ := a.proPlayersFirstScreen(false)
	response := a.buildReviewedProPlayers(teams)
	if response.AccountCount != 64 {
		t.Fatal(response.AccountCount)
	}
	if time.Since(started) > 300*time.Millisecond {
		t.Fatal("local roster held by upstream")
	}
	a.initializeProProfileCache()
	now := time.Now()
	var selected string
	for _, team := range response.Teams {
		for _, player := range team.Players {
			for _, account := range player.Accounts {
				key := proLadderAccountKey(account.GameName, account.TagLine)
				a.proProfiles.entries["pro-profile-v2|"+key] = opggProAccount{GameName: account.GameName, TagLine: account.TagLine, CheckedAt: now.Format(time.RFC3339Nano)}
				if selected == "" {
					selected = key
				}
			}
		}
	}
	body, _ := json.Marshal(map[string]any{"team": "all", "accounts": []string{selected}, "manual": false})
	w := httptest.NewRecorder()
	a.handleProScopedRefresh(w, httptest.NewRequest("POST", "/api/pro-players/refresh", strings.NewReader(string(body))))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	events := r175Events(t, a, "pro_refresh_plan")
	if len(events) != 1 || events[0]["visible"] != float64(1) || events[0]["due"] != float64(0) {
		t.Fatal(events)
	}
}
func TestR288ProRiotCacheKeepsLastSuccessOnFailure(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	a := r175App(t)
	cp := newChampionProvider()
	a.champions = cp
	var calls atomic.Int32
	fail := false
	matchRaw, readErr := os.ReadFile("testdata/r272-synthetic-match.json")
	if readErr != nil {
		t.Fatal(readErr)
	}
	var matchDocument map[string]any
	json.Unmarshal(matchRaw, &matchDocument)
	matchDocument["metadata"].(map[string]any)["matchId"] = "KR_2881"
	matchDocument["info"].(map[string]any)["gameId"] = 2881
	matchDocument["info"].(map[string]any)["gameStartTimestamp"] = int64(1792000000000)
	matchRaw, _ = json.Marshal(matchDocument)
	cp.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if fail {
			return updateResponse(503, []byte(`{}`)), nil
		}
		switch {
		case strings.Contains(r.URL.Path, "accounts/by-riot-id"):
			fields := strings.Split(r.URL.Path, "/")
			name, tag := fields[len(fields)-2], fields[len(fields)-1]
			hash := sha256.Sum256([]byte(name + tag))
			raw, _ := json.Marshal(map[string]any{"puuid": fmt.Sprintf("%x", hash), "gameName": name, "tagLine": tag})
			return updateResponse(200, raw), nil
		case strings.HasSuffix(r.URL.Path, "/ids"):
			return updateResponse(200, []byte(`["KR_2881"]`)), nil
		case strings.Contains(r.URL.Path, "/entries/"):
			return updateResponse(200, []byte(`[]`)), nil
		case strings.Contains(r.URL.Path, "/matches/"):
			return updateResponse(200, matchRaw), nil
		default:
			t.Fatalf("unexpected endpoint %s", r.URL.Path)
			return nil, nil
		}
	})}
	a.riot = newRiotProvider(cp)
	now := time.Now()
	a.proProfiles.now = func() time.Time { return now }
	old := opggProAccount{GameName: "Fixture", TagLine: "KR1", Region: "kr"}
	first := a.readProProfile(context.Background(), old)
	if first.CheckFailed || !first.LastMatchAtKnown {
		t.Fatalf("first %+v", first)
	}
	count := calls.Load()
	second := a.readProProfile(context.Background(), old)
	if calls.Load() != count || second.LastMatchAt != first.LastMatchAt {
		t.Fatal("fresh reread hit upstream")
	}
	now = now.Add(4 * time.Hour)
	fail = true
	// A new provider has no in-memory request cache; successful facts and the
	// permanent Riot-ID mapping belong to the application's persisted cache.
	a.riot = newRiotProvider(cp)
	failed := a.readProProfile(context.Background(), old)
	if !failed.CheckFailed || failed.LastMatchAt != first.LastMatchAt || string(failed.Rank) != string(first.Rank) {
		t.Fatalf("failure erased successful facts %+v", failed)
	}
}

func TestR288ProTenAccountsTwoWorkersWithinTenSeconds(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-fixture")
	a := r175App(t)
	cp := newChampionProvider()
	a.champions = cp
	var active, maximum, requests atomic.Int32
	raw, err := os.ReadFile("testdata/r272-synthetic-match.json")
	if err != nil {
		t.Fatal(err)
	}
	cp.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		n := active.Add(1)
		defer active.Add(-1)
		for old := maximum.Load(); n > old; old = maximum.Load() {
			if maximum.CompareAndSwap(old, n) {
				break
			}
		}
		select {
		case <-r.Context().Done():
			return nil, r.Context().Err()
		case <-time.After(200 * time.Millisecond):
		}
		switch {
		case strings.Contains(r.URL.Path, "accounts/by-riot-id"):
			parts := strings.Split(r.URL.Path, "/")
			name := parts[len(parts)-2]
			body, _ := json.Marshal(map[string]any{"puuid": strings.Repeat(name, 6), "gameName": name, "tagLine": "KR1"})
			return updateResponse(200, body), nil
		case strings.HasSuffix(r.URL.Path, "/ids"):
			return updateResponse(200, []byte(`["KR_288"]`)), nil
		case strings.Contains(r.URL.Path, "/entries/"):
			return updateResponse(200, []byte(`[]`)), nil
		case strings.Contains(r.URL.Path, "/matches/"):
			var document map[string]any
			json.Unmarshal(raw, &document)
			document["metadata"].(map[string]any)["matchId"] = "KR_288"
			body, _ := json.Marshal(document)
			return updateResponse(200, body), nil
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			return updateResponse(500, []byte(`{}`)), nil
		}
	})}
	a.riot = newRiotProvider(cp)
	a.initializeProProfileCache()
	rows := map[string]opggProAccount{}
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("Fixture%d", i)
		key := "pro-profile-v2|" + proLadderAccountKey(name, "KR1")
		rows[key] = opggProAccount{GameName: name, TagLine: "KR1", Region: "kr"}
		a.proProfiles.inflight[key] = true
	}
	started := time.Now()
	a.refreshProScope(rows, false)
	if elapsed := time.Since(started); elapsed >= 10*time.Second {
		t.Fatalf("team refresh %s", elapsed)
	}
	if maximum.Load() > 2 || maximum.Load() < 2 {
		t.Fatalf("concurrency %d", maximum.Load())
	}
	if len(a.proProfiles.inflight) != 0 {
		t.Fatal("inflight entries leaked")
	}
	for key := range rows {
		row := a.proProfiles.entries[key]
		if row.CheckFailed || !row.LastMatchAtKnown || row.LastSuccessAt == "" {
			t.Fatalf("row not refreshed: %+v", row)
		}
	}
	t.Logf("10 accounts in %s, requests=%d, max=%d", time.Since(started), requests.Load(), maximum.Load())
}

func TestR288ProThrottleStopsBatchWithoutInflightLeak(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-fixture")
	a := r175App(t)
	a.champions = newChampionProvider()
	a.riot = newRiotProvider(a.champions)
	a.riot.limitQueue = []*riotLimitWaiter{{turn: make(chan struct{})}}
	a.initializeProProfileCache()
	rows := map[string]opggProAccount{}
	for i := 0; i < 10; i++ {
		key := fmt.Sprint(i)
		rows[key] = opggProAccount{GameName: fmt.Sprintf("Fixture%d", i), TagLine: "KR1"}
		a.proProfiles.inflight[key] = true
	}
	done := make(chan struct{})
	go func() { defer close(done); a.refreshProScope(rows, false) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("quota yield blocked job queue")
	}
	if len(a.proProfiles.inflight) != 0 {
		t.Fatal("unstarted inflight entries leaked")
	}
}

func TestR288ProSuccessfulFactsExpireAfterSevenDays(t *testing.T) {
	now := time.Now()
	old := opggProAccount{LastSuccessAt: now.Add(-8 * 24 * time.Hour).Format(time.RFC3339Nano), CheckedAt: now.Format(time.RFC3339Nano), CheckFailed: true, Rank: json.RawMessage(`{"tier":"MASTER","lp":100}`), LastMatchAt: "2026-10-01T00:00:00Z", LastMatchAtKnown: true, LadderRank: 1, LadderRankKnown: true}
	expired := expireProProfileFacts(old, now)
	if len(expired.Rank) > 0 || expired.LastMatchAtKnown || expired.LadderRankKnown {
		t.Fatal("failure extended old successful facts beyond seven days")
	}
	old.LastSuccessAt = now.Add(-6 * 24 * time.Hour).Format(time.RFC3339Nano)
	kept := expireProProfileFacts(old, now)
	if !kept.LastMatchAtKnown || string(kept.Rank) != string(old.Rank) {
		t.Fatal("fresh successful facts erased by failure")
	}
}
