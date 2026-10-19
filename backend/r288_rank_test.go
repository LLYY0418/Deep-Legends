package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func r288RankFixture(t *testing.T, rankedGames int, rankedDelay, mayhemDelay time.Duration) (*app, *LCUClient, string, chan string, *atomic.Int32) {
	t.Helper()
	a := r175App(t)
	client := &LCUClient{region: "TENCENT", rsoPlatform: "HN1"}
	ref := strings.Repeat("r", 48)
	templateRaw, err := os.ReadFile("testdata/r272-synthetic-match.json")
	if err != nil {
		t.Fatal(err)
	}
	var template map[string]any
	if err := json.Unmarshal(templateRaw, &template); err != nil {
		t.Fatal(err)
	}
	fullInfo := template["info"].(map[string]any)
	mayhemCalls := &atomic.Int32{}
	a.sgp = newSGPProvider()
	a.sgp.token, a.sgp.tokenAt, a.sgp.tokenClient = "test", time.Now(), client
	a.sgp.serverBases["HN1"] = "https://fixture.invalid"
	payloads := map[bool]map[int][]byte{}
	for _, ranked := range []bool{true, false} {
		payloads[ranked] = map[int][]byte{}
		count := rankedGames
		if !ranked {
			count = 100
		}
		for offset := 0; offset <= count; offset += 50 {
			games := []any{}
			for i := offset; i < offset+50 && i < count; i++ {
				id, q := i+1, 420
				if !ranked {
					id, q = i+10000, 2400
				}
				info := make(map[string]any, len(fullInfo))
				for key, value := range fullInfo {
					info[key] = value
				}
				info["gameId"], info["queueId"] = id, q
				info["gameCreation"] = seasonStartS26.Add(48 * time.Hour).UnixMilli()
				participants := append([]any(nil), fullInfo["participants"].([]any)...)
				first := make(map[string]any)
				for key, value := range participants[0].(map[string]any) {
					first[key] = value
				}
				first["puuid"], first["win"] = ref, i%2 == 0
				participants[0] = first
				info["participants"] = participants
				games = append(games, map[string]any{"json": info})
			}
			raw, _ := json.Marshal(map[string]any{"games": games})
			payloads[ranked][offset] = raw
		}
	}
	a.sgp.http = &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		ranked := strings.Contains(r.URL.RawQuery, "q_420")
		delay := rankedDelay
		if !ranked {
			delay = mayhemDelay
			mayhemCalls.Add(1)
		}
		select {
		case <-r.Context().Done():
			return nil, r.Context().Err()
		case <-time.After(delay):
		}
		offset, _ := strconv.Atoi(r.URL.Query().Get("startIndex"))
		return updateResponse(200, payloads[ranked][offset]), nil
	})}
	events := make(chan string, 128)
	a.eventSubscribers = map[chan string]struct{}{events: {}}
	return a, client, ref, events, mayhemCalls
}

func r288WaitRanked(t *testing.T, events chan string, timeout time.Duration) map[string]any {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case raw := <-events:
			var event map[string]any
			if json.Unmarshal([]byte(raw), &event) == nil && event["type"] == "season-progress" && event["rankedComplete"] == true {
				return event
			}
		case <-timer.C:
			t.Fatal("ranked snapshot waited for mayhem")
			return nil
		}
	}
}

func TestR288BackfillPublishesRankedBeforeSlowMayhem(t *testing.T) {
	a, c, ref, events, mayhem := r288RankFixture(t, 1, 0, 2*time.Second)
	scan := r231Scan(ref)
	hash := a.storage.accountHash(Summoner{PUUID: ref})
	scan.cache.AccountHash = hash
	a.finishSeasonScan(scan, nil, hash)
	a.startSeasonBackfill(c, gameplayReference{ServerID: "HN1"}, Summoner{PUUID: ref}, ref, nil, "HN1", hash, "S26", seasonStartS26)
	event := r288WaitRanked(t, events, time.Second)
	if event["complete"] == true || mayhem.Load() > 1 {
		t.Fatalf("late publication: %#v calls=%d", event, mayhem.Load())
	}
	a.recordRankWindow(c, "HN1", ref, &seasonScanState{cache: seasonStatsCache{Streams: map[string]seasonStatsStream{"ranked": {Complete: true}}}})
	diagnostics := r175Events(t, a, "rank_window_check")
	if len(diagnostics) < 2 {
		t.Fatal(diagnostics)
	}
	for _, event := range diagnostics {
		if event["queue"] != "solo" && event["queue"] != "flex" {
			t.Fatal("queue outside diagnostic whitelist", event)
		}
		for _, key := range []string{"official_wins", "aggregated_wins", "aggregated_losses", "capped", "oldest_month"} {
			if _, ok := event[key]; !ok {
				t.Fatal("missing diagnostic field", key)
			}
		}
		raw, _ := json.Marshal(event)
		for _, private := range []string{ref, "puuid", "gameName", "tagLine", "playerRef", "account_hash"} {
			if strings.Contains(string(raw), private) {
				t.Fatal("identity entered rank-window diagnostics")
			}
		}
	}

}

func TestR290RankedPublicationFitsDecodeAndNetworkBudget(t *testing.T) {
	// Measure the production build, whose JSON/CPU costs do not include TSAN.
	// The separate integrity test below exercises all 500 matches in the parent
	// process as well, including when the suite is built with -race.
	if os.Getenv("R288_RANK_MEASURE_CHILD") != "1" {
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
		defer cancel()
		name := "rank-production-test"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		binary := filepath.Join(t.TempDir(), name)
		build := exec.CommandContext(ctx, "go", "test", "-c", "-race=false", "-o", binary, ".")
		if output, err := build.CombinedOutput(); err != nil {
			t.Fatalf("production fixture build: %v\n%s", err, output)
		}
		command := exec.CommandContext(ctx, binary, "-test.run=^TestR290RankedPublicationFitsDecodeAndNetworkBudget$", "-test.count=1", "-test.v", "-test.timeout=90s")
		command.Env = append(os.Environ(), "R288_RANK_MEASURE_CHILD=1")
		output, err := command.CombinedOutput()
		t.Logf("production measurement:\n%s", output)
		if err != nil {
			t.Fatalf("production latency gate: %v", err)
		}
		return
	}
	// Same-process CPU calibration uses the identical full payload and decoder /
	// immutable cache, with no network delay, aggregation, disk save or publish.
	baselineApp, client, ref, _, mayhem := r288RankFixture(t, 500, 0, 0)
	began := time.Now()
	decoded, pages := 0, 0
	for offset := 0; ; offset += sgpPageSize {
		infos, consumed, more, err := baselineApp.sgp.matchHistoryFilteredWithDecodeOn(t.Context(), client, "HN1", ref, offset, sgpPageSize, seasonStreamTags["ranked"], false, nil)
		if err != nil {
			t.Fatal(err)
		}
		decoded += len(infos)
		pages++
		if !more {
			break
		}
		if consumed != sgpPageSize {
			t.Fatal("unexpected calibration page", consumed)
		}
	}
	decodeBaseline := time.Since(began)
	if decoded != 500 || mayhem.Load() != 0 {
		t.Fatal("invalid decode baseline", decoded, mayhem.Load())
	}
	networkBudget := time.Duration(pages) * 300 * time.Millisecond
	const publicationMargin = time.Second
	bound := decodeBaseline + networkBudget + publicationMargin
	t.Logf("relative rank budget: decode_baseline=%s ranked_pages=%d network_budget=%s fixed_margin=%s bound=%s", decodeBaseline, pages, networkBudget, publicationMargin, bound)
	r288FiveHundredRanked(t, bound)
}

func r288FiveHundredRanked(t *testing.T, deadline time.Duration) chan string {
	t.Helper()
	a, c, ref, events, mayhem := r288RankFixture(t, 500, 300*time.Millisecond, 2*time.Second)
	scan := r231Scan(ref)
	hash := a.storage.accountHash(Summoner{PUUID: ref})
	scan.cache.AccountHash = hash
	a.finishSeasonScan(scan, nil, hash)
	started := time.Now()
	a.startSeasonBackfill(c, gameplayReference{ServerID: "HN1"}, Summoner{PUUID: ref}, ref, nil, "HN1", hash, "S26", seasonStartS26)
	event := r288WaitRanked(t, events, deadline)
	queues := event["snapshot"].(map[string]any)["seasonQueueStats"].(map[string]any)
	if queues["420"].(map[string]any)["games"] != float64(500) {
		t.Fatal(queues)
	}
	if event["complete"] == true || mayhem.Load() > 1 {
		t.Fatal("ranked snapshot was delayed until mayhem", event)
	}
	t.Logf("500 real-shaped matches: ranked snapshot %s", time.Since(started))
	return events
}

func TestR288FiveHundredRanksRetainRaceIntegrityAndFinalSnapshot(t *testing.T) {
	events := r288FiveHundredRanked(t, 30*time.Second)
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	for {
		select {
		case raw := <-events:
			var event map[string]any
			if json.Unmarshal([]byte(raw), &event) != nil || event["type"] != "season-progress" || event["complete"] != true {
				continue
			}
			queues := event["snapshot"].(map[string]any)["seasonQueueStats"].(map[string]any)
			if event["rankedComplete"] != true || queues["420"].(map[string]any)["games"] != float64(500) || queues["2400"].(map[string]any)["games"] != float64(100) {
				t.Fatal("final snapshot lost ranked results", event)
			}
			return
		case <-timer.C:
			t.Fatal("final mayhem snapshot did not complete")
		}
	}
}

func TestR288HeadPublishesRankedBeforeMayhemRequests(t *testing.T) {
	a, c, ref, events, mayhem := r288RankFixture(t, 1, 0, 2*time.Second)
	scan := r231Scan(ref)
	scan.headOnly = true
	scan.onPage = func(s *seasonScanState) {
		a.finishSeasonScan(s, nil, "")
		a.publishSeasonSnapshot(gameplayReference{ServerID: "HN1"}, ref, s.cache)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); a.seasonScanPagesWithHistoryCache(ctx, c, "HN1", ref, scan, 4, false) }()
	event := r288WaitRanked(t, events, time.Second)
	if event["complete"] == true || mayhem.Load() > 1 {
		t.Fatal("head did not prioritize ranked")
	}
	cancel()
	<-done
}

func TestR288ScorePresenceOnePassRetainsMissingAndZero(t *testing.T) {
	for _, body := range []string{`{}`, `{"kills":0,"deaths":0,"assists":0,"win":false}`, `{"kills":null,"deaths":0,"assists":1,"totalDamageDealtToChampions":0,"goldEarned":0,"totalMinionsKilled":0,"neutralMinionsKilled":0,"visionScore":0,"wardsPlaced":0,"wardsKilled":0}`} {
		var raw map[string]json.RawMessage
		json.Unmarshal([]byte(body), &raw)
		var participant riotParticipant
		if err := json.Unmarshal([]byte(body), &participant); err != nil {
			t.Fatal(err)
		}
		want, _ := json.Marshal(scoreMissingFields(raw, true))
		got, _ := json.Marshal(participant.scoreMissing)
		if string(want) != string(got) {
			t.Fatalf("presence %s: %s != %s", body, got, want)
		}
	}
	var bad riotParticipant
	if json.Unmarshal([]byte(`{"kills":"bad"}`), &bad) == nil {
		t.Fatal("invalid core field silently accepted")
	}
}
