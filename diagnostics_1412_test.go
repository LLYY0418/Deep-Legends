package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func Test1412SavedBanSwitchReachesRunnerAndLCU(t *testing.T) {
	fixture := newR78ChampSelectFixture(t, "practice", "ban", 0, []int64{141}, map[int64]champSelectGridSelectionStatus{141: {}})
	fixture.session.QueueID = 3110
	settings := fixture.runner.currentWatch()
	settings.ChampSelect.Enabled = true
	group := settings.ChampSelect.Groups["practice"]
	group.Ban.Enabled = true
	group.Ban.DelayMS = 0
	group.Ban.Champions["default"] = []int64{141}
	settings.ChampSelect.Groups["practice"] = group
	payload, _ := json.Marshal(settings)
	application := &app{watch: fixture.runner, storage: &localStore{root: t.TempDir()}}
	response := httptest.NewRecorder()
	application.handleWatchRules(response, httptest.NewRequest(http.MethodPost, "/api/watch/rules", bytes.NewReader(payload)))
	if response.Code != http.StatusOK {
		t.Fatalf("settings save status=%d", response.Code)
	}
	var acknowledged watchSettings
	if err := json.Unmarshal(response.Body.Bytes(), &acknowledged); err != nil || !acknowledged.ChampSelect.Groups["practice"].Ban.Enabled {
		t.Fatalf("save did not acknowledge enabled ban: %v", err)
	}
	if !loadWatchSettings(application.storage).ChampSelect.Groups["practice"].Ban.Enabled {
		t.Fatal("enabled switch did not survive reload")
	}
	if request := fixture.waitAfterEvaluate(t); request.ChampionID != 141 {
		t.Fatalf("saved switch did not drive actual PATCH: %#v", request)
	}
}

func Test1412CustomQueueOverridesFalseFlagAndNeverSearches(t *testing.T) {
	for _, queue := range []int{3100, 3110, 3200} {
		t.Run(fmt.Sprint(queue), func(t *testing.T) {
			runner := newWatchRunner(nil, nil)
			var writes atomic.Int32
			client := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "test", http: &http.Client{Transport: gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != "GET" {
					writes.Add(1)
				}
				body := fmt.Sprintf(`{"gameConfig":{"queueId":%d,"isCustom":false},"localMember":{"isLeader":true},"members":[{}]}`, queue)
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}}
			rules := defaultWatchSettings().Rules
			rules.AutoMatchmaking = watchMatchmakingRule{Enabled: true, MinPartySize: 1}
			runner.handleLobby(client, Summoner{}, rules)
			if !runner.customPaused() {
				t.Fatal("custom queue falsely resumed")
			}
			if ready, terminal := runner.matchmakingPreflight(context.Background(), client); ready || !terminal {
				t.Fatal("preflight allowed custom search")
			}
			if runner.scheduleAutoMatchmaking(client, 0) {
				t.Fatal("custom search armed")
			}
			time.Sleep(10 * time.Millisecond)
			if writes.Load() != 0 {
				t.Fatalf("custom writes=%d", writes.Load())
			}
		})
	}
}

func Test1412DisabledBanPoolExplainsWhyThenEnabledBanSubmits(t *testing.T) {
	fixture := newR78ChampSelectFixture(t, "practice", "ban", 0, []int64{141}, map[int64]champSelectGridSelectionStatus{141: {}})
	fixture.session.QueueID = 3110
	fixture.enable("practice", "ban", []int64{141})
	settings := fixture.runner.currentWatch()
	group := settings.ChampSelect.Groups["practice"]
	group.Ban.Enabled = false
	settings.ChampSelect.Groups["practice"] = group
	fixture.runner.apply(settings)
	fixture.runner.evaluateChampSelect(fixture.client, fixture.runner.currentWatch().ChampSelect)
	if fixture.patches.Load() != 0 {
		t.Fatal("disabled rule submitted")
	}
	found := false
	for _, record := range fixture.runner.champSelectSnapshot().Records {
		if strings.Contains(record.Message, "自动禁用开关未开启") {
			found = true
		}
	}
	if !found {
		t.Fatal("disabled ban was silently skipped")
	}
	fixture.enable("practice", "ban", []int64{141})
	if req := fixture.waitAfterEvaluate(t); req.ChampionID != 141 {
		t.Fatalf("enabled rule: %#v", req)
	}
}

func Test1412GridReadFailureIsVisibleAndDeduplicated(t *testing.T) {
	fixture := newR78ChampSelectFixture(t, "practice", "ban", 0, nil, nil)
	fixture.enable("practice", "ban", []int64{141})
	original := fixture.client.http.Transport
	fixture.client.http.Transport = gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "all-grid-champions") {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"unexpected":"object"}`))}, nil
		}
		return original.RoundTrip(req)
	})
	diagnostics := []map[string]any{}
	fixture.runner.observe = func(event map[string]any) {
		if event["event"] == "champselect_read_failed" {
			diagnostics = append(diagnostics, event)
		}
	}
	for i := 0; i < 2; i++ {
		fixture.runner.evaluateChampSelect(fixture.client, fixture.runner.currentWatch().ChampSelect)
	}
	if len(diagnostics) != 1 || diagnostics[0]["endpoint"] != "all-grid-champions" {
		t.Fatalf("read failure lost: %#v", diagnostics)
	}
	snapshot := fixture.runner.champSelectSnapshot()
	if snapshot.GroupID != "practice" || len(snapshot.Records) == 0 || fixture.patches.Load() != 0 {
		t.Fatal("failed read looks idle or submitted a guess")
	}
}

func Test1412ChampSelectDiagnosticPathsStayDistinct(t *testing.T) {
	for _, name := range []string{"all-grid-champions", "pickable-champion-ids", "bannable-champion-ids"} {
		path := "/lol-champ-select/v1/" + name
		if got := lcuDiagnosticPath(path); got != path {
			t.Fatalf("%s => %s", path, got)
		}
	}
}
