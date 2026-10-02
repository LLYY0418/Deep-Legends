package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const r177Timeline = `{"info":{"frames":[{"events":[{"type":"ITEM_PURCHASED","participantId":1,"timestamp":5000,"itemId":1055}]}]}}`

func r177StarterApp(t *testing.T, used int, status int) (*app, *atomic.Int32) {
	t.Helper()
	t.Setenv("RIOT_API_KEY", "fixture-only")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if !strings.HasSuffix(r.URL.Path, "/timeline") {
			t.Errorf("unexpected upstream %s", r.URL.Path)
		}
		if status == 429 {
			w.Header().Set("Retry-After", "80")
		}
		w.WriteHeader(status)
		if status == 200 {
			_, _ = w.Write([]byte(r177Timeline))
		}
	}))
	t.Cleanup(server.Close)
	endpoint, _ := url.Parse(server.URL)
	p := specialistTestProvider(gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		// Keep real rate admission on the Riot host, redirect only HTTP transport.
		if r.URL.Host != riotClusterHost {
			t.Errorf("unexpected domain %s", r.URL.Host)
		}
		if deadline, ok := r.Context().Value(riotQueueDeadlineKey{}).(time.Time); !ok || time.Until(deadline) > 6*time.Second {
			t.Errorf("missing six-second queue limit")
		}
		clone := r.Clone(r.Context())
		clone.URL.Scheme, clone.URL.Host = endpoint.Scheme, endpoint.Host
		return server.Client().Transport.RoundTrip(clone)
	}))
	now := time.Unix(1000, 0)
	p.limitNow = func() time.Time { return now }
	r110Advertise(p, riotClusterHost, "/lol/match/v5/matches/KR_1/timeline", "20:1,100:120", fmt.Sprintf("%d:120", used))
	p.specialistCache[specialistRuneKey(64, "mid")] = specialistRuneCacheEntry{runes: []gameplayRecommendationRune{
		{Key: "row-1", PlayedAt: 1000, runeMatchID: "KR_1", runeParticipantID: 1},
		{Key: "row-2", PlayedAt: 2000, runeMatchID: "KR_2", runeParticipantID: 1},
		{Key: "row-3", PlayedAt: 3000, runeMatchID: "KR_3", runeParticipantID: 1},
	}}
	a := r175App(t)
	a.riot, a.champions = p, p.champions
	return a, &calls
}

func r177StarterRequest(t *testing.T, a *app, keys ...int) map[string]json.RawMessage {
	t.Helper()
	rows := []runeStarterRowRequest{}
	for _, key := range keys {
		rows = append(rows, runeStarterRowRequest{fmt.Sprintf("row-%d", key), int64(key * 1000)})
	}
	body, _ := json.Marshal(runeStarterRequest{Source: "specialist", ChampionID: 64, Position: "mid", Rows: rows})
	rec := httptest.NewRecorder()
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()
	a.handleGameplayRuneStarters(rec, httptest.NewRequest("POST", "/api/gameplay/rune-starters", strings.NewReader(string(body))).WithContext(ctx))
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var response map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func TestR177StarterForegroundReserve(t *testing.T) {
	a, calls := r177StarterApp(t, 40, 200)
	ctx := withRiotBackground(r110RateContext(riotClusterHost, "/lol/match/v5/matches/KR_1/timeline"))
	if err := a.riot.wait(ctx); !errors.Is(err, errThrottled) {
		t.Fatalf("background reserve changed: %v", err)
	}
	rows := a.riot.specialistStarterRows(context.Background(), 64, "mid", map[string]int64{"row-1": 1000})
	if len(rows) != 1 || !reflect.DeepEqual(rows[0].StarterItemIDs, []int64{1055}) || calls.Load() != 1 {
		t.Fatalf("foreground reserve: rows=%+v calls=%d", rows, calls.Load())
	}
	response := r177StarterRequest(t, a, 2)
	var got []runeStarterRow
	_ = json.Unmarshal(response["starters"], &got)
	if len(got) != 1 || calls.Load() != 2 {
		t.Fatalf("handler still background: %+v calls=%d", response, calls.Load())
	}
	if _, ok := response["retryAfterSeconds"]; ok {
		t.Fatal("successful response has retry hint")
	}
}

func TestR177StarterFullQuotaHasRetryWithoutFailureCooldown(t *testing.T) {
	a, calls := r177StarterApp(t, 100, 200)
	started := time.Now()
	response := r177StarterRequest(t, a, 1, 2)
	if time.Since(started) >= 6*time.Second {
		t.Fatal("queue bound exceeded")
	}
	var retry int
	_ = json.Unmarshal(response["retryAfterSeconds"], &retry)
	if retry != 60 || calls.Load() != 0 {
		t.Fatalf("retry=%d calls=%d response=%s", retry, calls.Load(), response)
	}
	if len(a.riot.starterFailures) != 0 {
		t.Fatal("rate limit entered match failure cooldown", a.riot.starterFailures)
	}
	events := r175Events(t, a, "rune_starter_batch")
	if len(events) != 1 || events[0]["rows_requested"] != float64(2) || events[0]["success"] != float64(0) || events[0]["rate_limit"] != float64(2) || events[0]["other_failed"] != float64(0) || events[0]["retry_after_s"] != float64(60) {
		t.Fatal("batch counts", events)
	}
}

func TestR177StarterOtherErrorsKeepCooldown(t *testing.T) {
	for _, status := range []int{404, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			a, calls := r177StarterApp(t, 0, status)
			first := r177StarterRequest(t, a, 1)
			if _, ok := first["retryAfterSeconds"]; ok {
				t.Fatal("non-quota failure has retry")
			}
			if a.riot.starterFailures["KR_1"].IsZero() {
				t.Fatal("missing failure cooldown")
			}
			count := calls.Load()
			if _, err := a.riot.specialistMatchStarters(context.Background(), "KR_1", 1); err == nil || err.Error() != "starter retry cooldown" {
				t.Fatal("cooldown not applied", err)
			}
			r177StarterRequest(t, a, 1)
			if calls.Load() != count {
				t.Fatal("cooldown sent another upstream request")
			}
			for _, event := range r175Events(t, a, "rune_starter_batch") {
				if event["other_failed"] != float64(1) || event["rate_limit"] != float64(0) {
					t.Fatal(event)
				}
			}
		})
	}
}

func TestR177StarterBatchMixedCountsAndPrivacy(t *testing.T) {
	a, _ := r177StarterApp(t, 0, 200)
	if _, err := a.riot.specialistMatchStarters(context.Background(), "KR_1", 1); err != nil {
		t.Fatal(err)
	}
	a.riot.starterFailures["KR_3"] = time.Now()
	a.riot.rateHosts[riotClusterHost].app.windows[120*time.Second].used = 100
	r177StarterRequest(t, a, 1, 2, 3)
	events := r175Events(t, a, "rune_starter_batch")
	if len(events) != 1 {
		t.Fatal(events)
	}
	event := events[0]
	for key, want := range map[string]float64{"rows_requested": 3, "success": 1, "rate_limit": 1, "other_failed": 1, "retry_after_s": 60, "queue_wait_ms": 0} {
		if event[key] != want {
			t.Fatalf("%s: %v", key, event)
		}
	}
	raw, _ := json.Marshal(event)
	if strings.Contains(string(raw), "KR_") || strings.Contains(string(raw), "row-") {
		t.Fatal("batch contains match identity", string(raw))
	}
}

func TestR177ChampionLanesExistingProviderCacheAndValidation(t *testing.T) {
	cp := newChampionProvider()
	cp.cache = newChampionDataCache(trackTestStore(t, &localStore{root: t.TempDir()}))
	var calls atomic.Int32
	cp.client = &http.Client{Transport: championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		switch {
		case r.URL.Host == qq101Host && r.URL.Path == qq101VersionPath:
			return qq101HTTPResponse(r, 200, []byte(`{"code":0,"data":[{"id":"226","name":"16.19"}]}`)), nil
		case r.URL.Host == qq101Host && r.URL.Path == qq101RiftPath+"_newlane":
			if r.URL.Query().Get("itier") != "255" || r.URL.Query().Has("lane") {
				t.Errorf("QQ101 wrong query %s", r.URL.RawQuery)
			}
			return qq101HTTPResponse(r, 200, qq101FieldFixture(t, "18122", `{"lane_details":"MIDDLE:1.92_46.12_0.31_T4_52_70#TOP:0.53_51.5_0.3_T4_21_30"}`)), nil
		case r.URL.Host == opggChampionHost && strings.HasSuffix(r.URL.Path, "/versions"):
			return qq101HTTPResponse(r, 200, []byte(`{"data":["16.19"]}`)), nil
		case r.URL.Host == opggChampionHost && r.URL.Path == "/api/KR/champions/ranked/103/MID":
			if r.URL.Query().Get("tier") != "gold" {
				t.Errorf("tier substituted: %s", r.URL.RawQuery)
			}
			return qq101HTTPResponse(r, 200, []byte(`{"data":{"summary":{"id":103,"average_stats":{"play":100},"positions":[{"name":"MIDDLE","stats":{"role_rate":0.7,"play":70}},{"name":"TOP","stats":{"play":30}}]}}}`)), nil
		default:
			t.Errorf("new upstream request: %s", r.URL)
			return qq101HTTPResponse(r, 404, []byte(`{}`)), nil
		}
	})}
	a := &app{champions: cp}
	request := func(query string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		a.handleGameplayChampionLanes(rec, httptest.NewRequest("GET", "/api/gameplay/champion-lanes?"+query, nil))
		return rec
	}
	for _, tier := range []string{"all", "gold"} {
		query := "champions=103&tier=" + tier
		rec := request(query)
		if rec.Code != 200 {
			t.Fatal(rec.Code, rec.Body.String())
		}
		var result map[string][]championLaneShare
		if json.Unmarshal(rec.Body.Bytes(), &result) != nil || !reflect.DeepEqual(result["103"], []championLaneShare{{"mid", 0.7}, {"top", 0.3}}) {
			t.Fatal(rec.Body.String())
		}
		before := calls.Load()
		request(query)
		if calls.Load() != before {
			t.Fatal("cache sent upstream again")
		}
	}
	before := calls.Load()
	for _, query := range []string{"", "champions=0", "champions=-1", "champions=x", "champions=10001", "champions=1,", "champions=1,1", "champions=1,2,3,4,5,6", "champions=103&tier=unknown"} {
		if rec := request(query); rec.Code != 400 {
			t.Fatalf("invalid %q accepted: %d", query, rec.Code)
		}
	}
	if calls.Load() != before {
		t.Fatal("invalid requests reached provider")
	}
	if rec := request("champions=1,2,3,4,5"); rec.Code != 200 {
		t.Fatal("five champions rejected", rec.Code)
	}
}

func TestR177LaneDiagnosticShapeAllowlist(t *testing.T) {
	a := r175App(t)
	for _, reason := range []string{"context-unavailable", "lane-ambiguous"} {
		body := fmt.Sprintf(`{"event":"lane_matchup_candidate_fetch","reason":%q,"selfPosition":"mid","enemyLockedCount":3,"enemyPositionKnownCount":1,"allyPositionKnownCount":5,"key":"SECRET_PLAYER"}`, reason)
		rec := httptest.NewRecorder()
		a.handleClientDiagnostic(rec, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(body)))
		if rec.Code != 204 {
			t.Fatal(rec.Code, rec.Body.String())
		}
	}
	for _, row := range r175Events(t, a, "lane_matchup_candidate_fetch") {
		if row["self_position"] != "mid" || row["enemy_locked_count"] != float64(3) || row["enemy_position_known_count"] != float64(1) || row["ally_position_known_count"] != float64(5) {
			t.Fatal(row)
		}
		raw, _ := json.Marshal(row)
		if strings.Contains(string(raw), "SECRET_PLAYER") {
			t.Fatal("identity leaked")
		}
	}
}

func TestR177StarterShortQueueWaitIsObserved(t *testing.T) {
	a, _ := r177StarterApp(t, 40, 200)
	p := a.riot
	now := p.limitNow()
	p.limitNow = func() time.Time { return now }
	p.rateHosts[riotClusterHost].app.windows[time.Second].used = 20
	p.limitSleep = func(ctx context.Context, delay time.Duration) error {
		if delay > 6*time.Second {
			t.Fatalf("wait exceeds queue limit: %s", delay)
		}
		time.Sleep(2 * time.Millisecond)
		now = now.Add(delay)
		return nil
	}
	response := r177StarterRequest(t, a, 1)
	var rows []runeStarterRow
	_ = json.Unmarshal(response["starters"], &rows)
	if len(rows) != 1 {
		t.Fatal("short queue did not recover", response)
	}
	event := r175Events(t, a, "rune_starter_batch")[0]
	if event["queue_wait_ms"].(float64) < 1 {
		t.Fatal("queue wait not observed", event)
	}
}

func TestR177StarterUpstream429IsNotMatchFailure(t *testing.T) {
	a, _ := r177StarterApp(t, 0, 429)
	response := r177StarterRequest(t, a, 1)
	var retry int
	_ = json.Unmarshal(response["retryAfterSeconds"], &retry)
	if retry != 60 || len(a.riot.starterFailures) != 0 {
		t.Fatal("upstream throttle cached as match failure", response, a.riot.starterFailures)
	}
}
