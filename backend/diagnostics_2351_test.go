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

// P2-1（R120 复测）：现行契约是「通配符先悬停、经会话确认后才允许锁定」
// （r91_addendum_test.go「wildcard must hover first」、r95_test.go「wildcard must
// hover before lock」）。[-1] 哨兵在本地进行中的 ban 回合里会从选人网格挑候选，
// 走 wildcard-grid-hover，forceHover（champselect.go:985）压掉 lock-now 的直接锁定，
// 由 scheduleChampSelectRequest **异步**发出一条 completed:false 的悬停 PATCH。
//
// 旧版本在这里断言 patches==0（「哨兵不得当通配符」，R78/2351 时代的语义，已被上述
// 契约取代），而且在 evaluateChampSelect 返回后立刻看计数——PATCH 大多数时候还没
// 发出去，「碰巧为 0」；调度快一点就红（审查方实测 -race 全新进程单跑 18% 失败，
// 断言前加 300ms sleep 则 20/20 全红；本机 120 次全新进程 0 红 + sleep 探针 20/20 红，
// 同一根因）。现在改为：显式等那一次悬停 PATCH，断言载荷，再证明「确认之前不会锁」。
// 策略必须钉成 lock-now：这是 forceHover 唯一承重的场景（show-then-lock 下未确认
// 会话本来就不会锁，forceHover 改没改都一样，测不出退化）。
func Test2351BanSentinelHoversOnceWithoutLocking(t *testing.T) {
	f := newR78ChampSelectFixture(t, "practice", "ban", 0, []int64{-1}, map[int64]champSelectGridSelectionStatus{141: {}})
	f.enable("practice", "ban", []int64{141})
	settings := f.runner.currentWatch()
	group := settings.ChampSelect.Groups["practice"]
	group.Ban.Strategy = "lock-now"
	settings.ChampSelect.Groups["practice"] = group
	f.runner.apply(settings)
	f.runner.evaluateChampSelect(f.client, f.runner.currentWatch().ChampSelect)
	// 1) 悬停 PATCH 由调度器异步发出：用 patchCh 等它，而不是立刻看计数赌调度快慢。
	var hover r78PatchRequest
	select {
	case hover = <-f.patchCh:
	case <-time.After(5 * time.Second):
		t.Fatalf("[-1] sentinel never hovered: no PATCH within 5s (patches=%d)", f.patches.Load())
	}
	// 2) 载荷必须是「ban / 配置内且未被禁的 141 / 未锁定」。
	if hover.Type != "ban" || hover.ChampionID != 141 || hover.Completed {
		t.Fatalf("sentinel hover payload = %+v, want {Type:ban ChampionID:141 Completed:false}", hover)
	}
	wait2351RunnerIdle(t, f)
	// 3) 会话还没有回显悬停（action.ChampionID 仍是 0）：再评估一次也必须被
	//    「同键去重」挡住，不产生任何 completed:true 的锁定 PATCH。评估里的去重
	//    return 在调度之前同步发生，所以 idle 之后的计数检查是确定性的，不是赌窗口。
	f.runner.evaluateChampSelect(f.client, f.runner.currentWatch().ChampSelect)
	wait2351RunnerIdle(t, f)
	for {
		select {
		case extra := <-f.patchCh:
			if extra.Completed {
				t.Fatalf("sentinel locked before hover confirmation: %+v", extra)
			}
			continue
		default:
		}
		break
	}
	if got := f.patches.Load(); got != 1 {
		t.Fatalf("sentinel produced %d PATCHes, want exactly one hover (completed:false)", got)
	}
}

// wait2351RunnerIdle 与 champselect_takeover_test.go 的 waitTakeoverIdle 同一口径：
// 等到 runner 没有待调度也没有在飞的请求。r78 fixture 与 executionFixture 字段名
// 不同，所以单独写一份而不是改共享 helper 的签名。
func wait2351RunnerIdle(t *testing.T, f *r78ChampSelectFixture) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		f.runner.mu.Lock()
		idle := len(f.runner.pending) == 0 && len(f.runner.champSelect.inFlight) == 0
		f.runner.mu.Unlock()
		if idle {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("champselect request did not finish within 5s")
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
