package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func Test2351PickSkipsTeammateChampionEvenWithoutIntentFlag(t *testing.T) {
	for _, avoid := range []bool{true, false} {
		f := newR78ChampSelectFixture(t, "practice", "pick", 0, []int64{5, 804}, map[int64]champSelectGridSelectionStatus{5: {}, 804: {}})
		ally := int64(8)
		f.session.MyTeam = append(f.session.MyTeam, lcuChampSelectPlayer{CellID: &ally, ChampionID: 5})
		f.enable("practice", "pick", []int64{5, 804})
		settings := f.runner.currentWatch()
		group := settings.ChampSelect.Groups["practice"]
		group.Pick.AvoidTeammateIntent = avoid
		settings.ChampSelect.Groups["practice"] = group
		f.runner.apply(settings)
		if got := f.waitAfterEvaluate(t); got.ChampionID != 804 {
			t.Fatalf("avoid=%v picked teammate champion: %+v", avoid, got)
		}
		wait2253Submitted(t, f.runner, 42, false)
		if got := f.runner.champSelectSnapshot().PickStates["5"]; got != "teammate-picked" {
			t.Fatalf("UI state=%q", got)
		}
	}
}

func Test2351OwnHoverFallsThroughWhenTeammateTakesChampion(t *testing.T) {
	f := newR78ChampSelectFixture(t, "practice", "pick", 0, []int64{5, 804}, map[int64]champSelectGridSelectionStatus{5: {}, 804: {}})
	f.enable("practice", "pick", []int64{5, 804})
	if first := f.waitAfterEvaluate(t); first.ChampionID != 5 || first.Completed {
		t.Fatalf("hover=%+v", first)
	}
	wait2253Submitted(t, f.runner, 42, false)
	f.session.Actions[0][0].ChampionID = 5
	ally := int64(8)
	f.session.MyTeam = append(f.session.MyTeam, lcuChampSelectPlayer{CellID: &ally, ChampionID: 5})
	if next := f.waitAfterEvaluate(t); next.ChampionID != 804 || next.Completed {
		t.Fatalf("fallback hover=%+v", next)
	}
	wait2253Submitted(t, f.runner, 42, false)
	f.session.Actions[0][0].ChampionID = 804
	if lock := f.waitAfterEvaluate(t); lock.ChampionID != 804 || !lock.Completed {
		t.Fatalf("lock=%+v", lock)
	}
	wait2253Submitted(t, f.runner, 42, true)
}

func Test2351PreflightRejectsTeammatePickArrivingDuringDelay(t *testing.T) {
	f := newR78ChampSelectFixture(t, "practice", "pick", 0, []int64{5, 804}, map[int64]champSelectGridSelectionStatus{5: {}, 804: {}})
	f.enable("practice", "pick", []int64{5, 804})
	ally := int64(8)
	f.session.MyTeam = append(f.session.MyTeam, lcuChampSelectPlayer{CellID: &ally, ChampionID: 5})
	decision := champSelectDecision{Action: champSelectActionPick, ActionID: 42, ChampionID: 5, SessionAPI: champSelectAPI, LocalCellID: 7, QueueID: 3100}
	f.runner.mu.Lock()
	f.runner.champSelect.groupID = "practice"
	f.runner.mu.Unlock()
	if f.runner.champSelectRequestStillCurrent(context.Background(), f.client, decision) {
		t.Fatal("stale pick authorized")
	}
}

func Test2351BanSentinelDoesNotAuthorizeConfiguredHeroes(t *testing.T) {
	f := newR78ChampSelectFixture(t, "practice", "ban", 0, []int64{-1}, map[int64]champSelectGridSelectionStatus{141: {}})
	f.enable("practice", "ban", []int64{141})
	f.runner.evaluateChampSelect(f.client, f.runner.currentWatch().ChampSelect)
	if f.patches.Load() != 0 {
		t.Fatal("empty-ban sentinel used as a wildcard")
	}
}

// Keep the real transport/session preflight in the test, with no live LCU writes.
func response2351(value any) *http.Response {
	data, _ := json.Marshal(value)
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(data)))}
}

func Test2351AuthDiagnosticNeverExportsSecrets(t *testing.T) {
	claims, _ := json.Marshal(map[string]any{"exp": time.Now().Add(-time.Hour).Unix(), "sub": "private-player", "aud": "private-audience"})
	token := "header." + base64.RawURLEncoding.EncodeToString(claims) + ".secret-signature"
	diagnostic := sgpAuthDiagnostic([]byte(`{"message":"expired token private-player secret-token","credentials":"private-key"}`), token)
	if diagnostic["token_expired"] != true || diagnostic["auth_error_class"] != "expired" {
		t.Fatalf("diagnostic=%+v", diagnostic)
	}
	encoded, _ := json.Marshal(diagnostic)
	for _, secret := range []string{"private", "secret", token} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("secret leaked: %s", encoded)
		}
	}
}

func Test2351SourceDiagnosticIncludesRawBanIDsAndTeamWithoutIdentity(t *testing.T) {
	f := newR78ChampSelectFixture(t, "practice", "ban", 0, []int64{-1}, map[int64]champSelectGridSelectionStatus{141: {}})
	f.session.MyTeam = append(f.session.MyTeam, lcuChampSelectPlayer{ChampionID: 5, PUUID: "private-player", GameName: "private-name"})
	var event map[string]any
	f.runner.observe = func(value map[string]any) {
		if value["event"] == "champselect_source" {
			event = value
		}
	}
	f.enable("practice", "ban", []int64{141})
	f.runner.evaluateChampSelect(f.client, f.runner.currentWatch().ChampSelect)
	ids, ok := event["bannable_raw_ids"].([]int64)
	if !ok || len(ids) != 1 || ids[0] != -1 || event["bannable_count"] != 0 {
		t.Fatalf("source=%+v", event)
	}
	encoded, _ := json.Marshal(event)
	if strings.Contains(string(encoded), "private") || !strings.Contains(string(encoded), `"champion_id":5`) {
		t.Fatalf("source=%s", encoded)
	}
}

func Test2351DelayedPickCancelsThenAdvancesOnFreshSession(t *testing.T) {
	f := newR78ChampSelectFixture(t, "practice", "pick", 0, []int64{5, 804}, map[int64]champSelectGridSelectionStatus{5: {}, 804: {}})
	f.enable("practice", "pick", []int64{5, 804})
	settings := f.runner.currentWatch()
	g := settings.ChampSelect.Groups["practice"]
	g.Pick.DelayMS, g.Pick.Strategy = 20, "lock-now"
	settings.ChampSelect.Groups["practice"] = g
	f.runner.apply(settings)
	var reads atomic.Int32
	transport := f.client.http.Transport
	f.client.http.Transport = gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == champSelectAPI+"/session" && reads.Add(1) > 1 {
			copy := *f.session
			ally := int64(8)
			copy.MyTeam = append(append([]lcuChampSelectPlayer{}, copy.MyTeam...), lcuChampSelectPlayer{CellID: &ally, ChampionID: 5})
			return response2351(copy), nil
		}
		return transport.RoundTrip(req)
	})
	f.runner.evaluateChampSelect(f.client, f.runner.currentWatch().ChampSelect)
	deadline := time.Now().Add(time.Second)
	for {
		f.runner.mu.Lock()
		pending := len(f.runner.pending)
		f.runner.mu.Unlock()
		if pending == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("pending request did not cancel")
		}
		time.Sleep(time.Millisecond)
	}
	if f.patches.Load() != 0 {
		t.Fatal("stale candidate submitted")
	}
	if got := f.waitAfterEvaluate(t); got.ChampionID != 804 || !got.Completed {
		t.Fatalf("fresh pick=%+v", got)
	}
	wait2253Submitted(t, f.runner, 42, true)
}

func Test2351AuthRequestLogsRefreshWithoutChangingTokenContract(t *testing.T) {
	var events []map[string]any
	var tokenReads int
	client := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "local-fixture"}
	client.http = &http.Client{Transport: gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/lol-league-session/v1/league-session-token" {
			t.Fatalf("unexpected token endpoint %s", req.URL.Path)
		}
		tokenReads++
		return response2351("private-session-token"), nil
	})}
	p := newSGPProvider()
	p.observe = func(value map[string]any) {
		if value["event"] == "sgp_request" {
			events = append(events, value)
		}
	}
	p.http = &http.Client{Transport: sgpRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "Bearer private-session-token" {
			t.Fatal("wrong auth contract")
		}
		return &http.Response{StatusCode: 401, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"message":"Unauthorized private-session-token"}`))}, nil
	})}
	var out any
	err := p.getJSONWithToken(context.Background(), client, sgpTokenLeagueSession, "HN10", "RANKED", "/ranked/{player}", "https://hn10-k8s-sgp.lol.qq.com:21019/ranked/private-player", &out)
	var upstream *sgpHTTPError
	if !errors.As(err, &upstream) || upstream.StatusCode != 401 || tokenReads != 2 {
		t.Fatalf("err=%v reads=%d", err, tokenReads)
	}
	if len(events) != 2 || events[0]["auth_error_class"] != "unauthorized" {
		t.Fatalf("events=%+v", events)
	}
	encoded, _ := json.Marshal(events)
	if strings.Contains(string(encoded), "private") {
		t.Fatalf("private data leaked: %s", encoded)
	}
}
