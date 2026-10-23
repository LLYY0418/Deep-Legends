package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestR300SingleDLPageAndCleanup(t *testing.T) {
	for _, legacy := range []int{0, 4} {
		t.Run(fmt.Sprint(legacy), func(t *testing.T) {
			var mu sync.Mutex
			pages := map[int64]lcuRunePage{10: {ID: 10, Name: "我的符文页"}}
			for i := 0; i < legacy; i++ {
				id := int64(i + 20)
				pages[id] = lcuRunePage{ID: id, Name: "[DL] 旧页"}
			}
			created, updated, deleted := 0, 0, 0
			current := int64(0)
			phase := "ChampSelect"
			client := &LCUClient{baseURL: "https://127.0.0.1", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				mu.Lock()
				defer mu.Unlock()
				var out any
				switch req.Method + " " + req.URL.Path {
				case "GET /lol-perks/v1/pages":
					list := []lcuRunePage{}
					for _, p := range pages {
						list = append(list, p)
					}
					out = list
				case "GET /lol-perks/v1/inventory":
					out = map[string]any{"canAddCustomPage": true}
				case "GET /lol-gameflow/v1/gameflow-phase":
					out = phase
				case "POST /lol-perks/v1/pages/":
					created++
					pages[100] = lcuRunePage{ID: 100, Name: "[DL] 新页"}
					out = map[string]any{"id": 100}
				case "PUT /lol-perks/v1/currentpage":
					_ = json.NewDecoder(req.Body).Decode(&current)
				default:
					id, _ := strconv.ParseInt(strings.TrimPrefix(req.URL.Path, "/lol-perks/v1/pages/"), 10, 64)
					if id == 10 {
						t.Errorf("user page touched: %s", req.Method)
					}
					switch req.Method {
					case "PUT":
						updated++
						var body lcuRunePage
						_ = json.NewDecoder(req.Body).Decode(&body)
						pages[id] = body
					case "DELETE":
						deleted++
						delete(pages, id)
					default:
						t.Errorf("unexpected %s %s", req.Method, req.URL.Path)
					}
				}
				raw, _ := json.Marshal(out)
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
			})}}
			request := gameplayRuneApplyRequest{ChampionName: "阿狸", Source: "OPGG", ChampionID: 103, PrimaryStyleID: 8000, SubStyleID: 8100, SelectedPerkIDs: []int64{1, 2, 3, 4, 5, 6, 5008, 5008, 5001}}
			for i := 0; i < 5; i++ {
				trace := &runeApplyTrace{}
				ctx := context.WithValue(context.Background(), runeApplyTraceContextKey{}, trace)
				id, err := applyRunePage(ctx, client, request)
				if err != nil || id != current {
					t.Fatal(id, err, current)
				}
				if len(pages) != 2 || pages[10].Name != "我的符文页" || trace.DLPagesAfter != 1 {
					t.Fatal(pages, trace)
				}
				if i == 0 && legacy == 4 && (trace.DLPagesBefore != 4 || trace.Action != "deleted_extra") {
					t.Fatal(trace)
				}
				if i > 0 && trace.Action != "updated" {
					t.Fatal(trace)
				}
			}
			if updated != 5 || legacy == 0 && created != 1 || legacy > 0 && created != 0 || deleted != max(0, legacy-1) {
				t.Fatal(created, updated, deleted)
			}
			no := false
			pages[90] = lcuRunePage{ID: 90, Name: "[DL] 只读", IsEditable: &no}
			pages[91] = lcuRunePage{ID: 91, Name: "[DL] 受保护", IsDeletable: &no}
			phase = "EndOfGame"
			a := &app{}
			a.cleanupDLRunePages(context.Background(), client)
			if len(pages) != 3 || pages[10].Name != "我的符文页" || pages[90].ID != 90 || pages[91].ID != 91 {
				t.Fatal("cleanup touched protected pages", pages)
			}
		})
	}
}
func TestR300NonEditableDLPageNeverUpdatedOrDeleted(t *testing.T) {
	no := false
	for _, p := range []lcuRunePage{{ID: 1, Name: "用户"}, {ID: 2, Name: "[DL] 只读", IsEditable: &no}, {ID: 3, Name: "[DL] 受保护", IsDeletable: &no}} {
		if editableDLPage(p) {
			t.Fatal(p)
		}
	}
}
func r300WaitAuto(t *testing.T, a *app) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		a.autoConfig.Lock()
		busy := a.autoConfig.processing
		a.autoConfig.Unlock()
		if !busy {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("auto config did not settle")
}
func TestR300ManualYieldFollowsChampion(t *testing.T) {
	for _, same := range []bool{false, true} {
		t.Run(fmt.Sprint(same), func(t *testing.T) {
			cell := int64(7)
			state := &r298ReviewLCU{session: lcuChampSelectSession{GameID: 42, QueueID: 420, LocalPlayerCellID: &cell, MyTeam: []lcuChampSelectPlayer{{CellID: &cell, ChampionID: 1}}}}
			state.flow.Map.ID = 11
			a, client := r298ReviewApp(t, state)
			a.autoConfig.settings.Enabled = true
			for _, id := range []string{"1", "2"} {
				a.autoConfig.settings.Champions[id] = map[string]autoConfigEntry{"ranked": {Spells: &autoConfigSpells{4, 14}}}
			}
			a.yieldAutoConfigForChampion(context.Background(), client, 1, "runes", "spells")
			target := int64(2)
			if same {
				target = 1
			}
			state.mu.Lock()
			state.session.MyTeam[0].ChampionID = target
			state.session.Actions = [][]lcuChampSelectAction{{{ActorCellID: cell, ChampionID: target, Type: "pick"}}}
			state.mu.Unlock()
			a.triggerAutoConfig(context.Background(), client)
			r300WaitAuto(t, a)
			state.mu.Lock()
			if len(state.writes) != 0 {
				t.Error("hover wrote client")
			}
			state.session.Actions[0][0].Completed = true
			state.mu.Unlock()
			a.triggerAutoConfig(context.Background(), client)
			r300WaitAuto(t, a)
			state.mu.Lock()
			count := len(state.writes)
			state.mu.Unlock()
			if same && count != 0 || !same && count != 1 {
				t.Fatal("wrong hero yield", same, count, a.autoConfigYieldSnapshot())
			}
			a.autoConfig.Lock()
			a.autoConfig.runtime.selectChampion(1)
			preserved := a.autoConfig.runtime.Yielded["runes"] && a.autoConfig.runtime.Yielded["spells"]
			a.autoConfig.Unlock()
			if !preserved {
				t.Fatal("returning to manually configured hero lost yield")
			}
		})
	}
}
func TestR300ManualCustomApplyDuringHover(t *testing.T) {
	cell := int64(7)
	state := &r298ReviewLCU{session: lcuChampSelectSession{GameID: 42, QueueID: 420, LocalPlayerCellID: &cell, MyTeam: []lcuChampSelectPlayer{{CellID: &cell, ChampionPickIntent: 1}}}}
	state.flow.Map.ID = 11
	a, _ := r298ReviewApp(t, state)
	a.autoConfig.settings.Champions["1"] = map[string]autoConfigEntry{"ranked": {Spells: &autoConfigSpells{4, 14}}}
	w := httptest.NewRecorder()
	a.handleAutoConfigApply(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"championId":1}`)))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"spells":"applied"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.writes) != 1 || state.writes[0] != (autoConfigSpells{4, 14}) {
		t.Fatal(state.writes)
	}
}
func TestR300StarterValidationAndBackupCompatibility(t *testing.T) {
	a, client := r298ReviewApp(t, &r298ReviewLCU{})
	catalog := autoConfigCatalog{Items: []autoConfigItem{{ID: 2003, Price: 50, InStore: true, DisplayInItemSets: true, Maps: map[string]bool{"11": true, "12": true}, Categories: []string{"Consumable"}}, {ID: 1055, Price: 450, InStore: true, DisplayInItemSets: true, Maps: map[string]bool{"11": true}}}}
	// Offline catalog cache still authorizes the exact fixture pool.
	a.autoConfig.catalog = &catalog
	a.autoConfig.catalogClient = client
	a.autoConfig.catalogKey, _ = clientCatalogKey(client)
	a.autoConfig.catalogAt = time.Now()
	for _, tc := range []struct {
		mode  string
		items []autoConfigStarterItem
		valid bool
	}{{"normal", []autoConfigStarterItem{{2003, 10}}, true}, {"normal", []autoConfigStarterItem{{2003, 11}}, false}, {"aram", []autoConfigStarterItem{{2003, 28}}, true}, {"normal", []autoConfigStarterItem{{1055, 2}}, false}, {"aram", []autoConfigStarterItem{{1055, 1}}, false}, {"urf", []autoConfigStarterItem{{2003, 10}}, true}} {
		e := autoConfigEntry{Items: &autoConfigItems{Starter: tc.items}}
		err := a.validateAutoConfigCatalog(context.Background(), tc.mode, e, false)
		if (err == nil) != tc.valid {
			t.Errorf("%+v: %v", tc, err)
		}
	}
	var old autoConfigEntry
	if err := json.Unmarshal([]byte(`{"items":{"core":[3000],"situational":[]}}`), &old); err != nil || len(old.Items.Starter) != 0 {
		t.Fatal(old, err)
	}
	entry := autoConfigEntry{Items: &autoConfigItems{Starter: []autoConfigStarterItem{{2003, 10}}, Core: []int64{3000}}}
	raw, _ := json.Marshal(entry)
	var roundtrip autoConfigEntry
	_ = json.Unmarshal(raw, &roundtrip)
	if !reflect.DeepEqual(entry, roundtrip) {
		t.Fatal(string(raw))
	}
}
func TestR300ProOPGGStarterUsesUniqueOfficialGameAndCaches(t *testing.T) {
	cp := proTestProvider()
	game := proTestGame()
	game.Start = game.Start.Truncate(10 * time.Second)
	event := proTestEvent()
	row := proOPGGBuild{ID: "999", ChampionID: "69", Position: "mid", League: "LCK", Team: proOPGGTeam{Acronym: "T1"}}
	row.Game.ID = "444"
	row.Game.Match.ID = "555"
	row.Game.BeginAt = game.Start
	row.Game.Set = 1
	row.Game.Match.HomeTeam.Acronym = "T1"
	row.Game.Match.AwayTeam.Acronym = "GEN"
	var mu sync.Mutex
	requests := []string{}
	diagnostics := []map[string]any{}
	cp.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		requests = append(requests, r.URL.Path)
		var body any
		switch {
		case r.URL.Host == esportsOPGGHost:
			return specialistTestResponse(r, 503, `unavailable`)
		case strings.HasSuffix(r.URL.Path, "getSchedule"):
			body = map[string]any{"data": map[string]any{"schedule": map[string]any{"events": []any{map[string]any{"startTime": game.Start, "state": "completed", "match": map[string]any{"id": event.ID, "teams": event.Match.Teams}}}}}}
		case strings.HasSuffix(r.URL.Path, "getEventDetails"):
			body = map[string]any{"data": map[string]any{"event": event}}
		case strings.Contains(r.URL.Path, "window/"):
			body = proWindow{Metadata: game.Metadata, Frames: []proFrame{{Timestamp: game.Start}}}
		case strings.Contains(r.URL.Path, "details/"):
			return specialistTestResponse(r, 200, string(r173ProOpeningBody(game)))
		default:
			t.Errorf("unexpected %s", r.URL.String())
		}
		raw, _ := json.Marshal(body)
		return specialistTestResponse(r, 200, string(raw))
	})}
	cp.diag = func(e map[string]any) { mu.Lock(); defer mu.Unlock(); diagnostics = append(diagnostics, e) }
	p := newProRuneProvider(cp, nil, nil)
	p.opggBuilds = map[int]proOPGGCache{69: {ChampionID: 69, Rows: []proOPGGBuild{row}}}
	wanted := map[string]int64{"pro-opgg-999": game.Start.UnixMilli()}
	for i := 0; i < 2; i++ {
		got := p.starterRows(context.Background(), 69, "mid", wanted)
		if len(got) != 1 || got[0].Key != "pro-opgg-999" || !reflect.DeepEqual(got[0].StarterItemIDs, []int64{1055, 2003}) {
			t.Fatal(got)
		}
	}
	if len(requests) != 5 {
		t.Fatal("mapping or opening cache missed", requests)
	}
	found := false
	for _, e := range diagnostics {
		if e["event"] == "rune_starter_items" && e["source"] == "pro" && e["outcome"] == "success" && e["item_count"] == 2 {
			found = true
		}
	}
	if !found {
		t.Fatal(diagnostics)
	}
	link := proStarterLink{Game: game, Event: event}
	if !proStarterLinkMatches(link, row) {
		t.Fatal("valid mapping rejected")
	}
	row.Game.Set = 2
	if proStarterLinkMatches(link, row) {
		t.Fatal("wrong set accepted")
	}
	row.Game.Set = 1
	row.Game.Match.HomeTeam.Acronym = "OTHER"
	if proStarterLinkMatches(link, row) {
		t.Fatal("wrong teams accepted")
	}
}
func TestR300AnnouncementHTTPStatus(t *testing.T) {
	client := &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) { return specialistTestResponse(r, 404, `not found`) })}
	_, _, err := fetchAnnouncementDocument(context.Background(), client)
	failure, ok := err.(*announcementFetchError)
	if !ok || failure.Status != 404 {
		t.Fatal(err)
	}
}

func TestR300OPGGActualOpeningPurchasesAndCache(t *testing.T) {
	raw, err := os.ReadFile("testdata/r300-opgg-opening.json")
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Data struct{ GameByMatch proOPGGOpeningGame }
	}
	if err = json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	game := payload.Data.GameByMatch
	row := proOPGGBuild{ID: "460494", ChampionID: "221", Position: "adc", Team: proOPGGTeam{Acronym: "KC"}}
	row.Game.ID = game.ID
	row.Game.Match.ID = "32715"
	row.Game.Set = 1
	row.Game.BeginAt = game.BeginAt
	cp := proTestProvider()
	requests := 0
	cp.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.Method != "POST" || r.URL.String() != "https://esports.op.gg/matches/graphql" {
			t.Error(r.Method, r.URL)
		}
		var q struct {
			Query     string
			Variables struct {
				MatchID string
				Set     int
			}
		}
		_ = json.NewDecoder(r.Body).Decode(&q)
		if q.Query != proOPGGOpeningQuery || q.Variables.MatchID != "32715" || q.Variables.Set != 1 {
			t.Error(q)
		}
		return specialistTestResponse(r, 200, string(raw))
	})}
	p := newProRuneProvider(cp, nil, nil)
	p.opggBuilds = map[int]proOPGGCache{221: {Rows: []proOPGGBuild{row}}}
	for i := 0; i < 2; i++ {
		got := p.opggStarterRows(context.Background(), 221, "Smolder", map[string]int64{"pro-opgg-460494": game.BeginAt.UnixMilli()})
		if len(got) != 1 || !reflect.DeepEqual(got[0].StarterItemIDs, []int64{1086, 2003, 2003}) {
			t.Fatal(got)
		}
	}
	if requests != 1 {
		t.Fatal("cache missed", requests)
	}
	// The same game must never supply another player's opening inventory.
	row.ID = "another"
	if _, err = proOPGGOpeningItems(game, row); err == nil {
		t.Fatal("foreign player accepted")
	}
	row.ID = "460494"
	row.Team.Acronym = "OTHER"
	if _, err = proOPGGOpeningItems(game, row); err == nil {
		t.Fatal("foreign team accepted")
	}
}
func TestR300FailedSessionDoesNotYieldOldChampion(t *testing.T) {
	a := &app{}
	a.autoConfig.runtime.selectChampion(1)
	client := &LCUClient{baseURL: "https://127.0.0.1", http: &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) { return specialistTestResponse(r, 503, `unavailable`) })}}
	a.yieldAutoConfigForChampion(context.Background(), client, 2, "runes")
	if a.autoConfig.runtime.ChampionID != 1 || len(a.autoConfig.runtime.Yielded) != 0 || len(a.autoConfig.runtime.yieldedByChampion) != 1 {
		t.Fatal(a.autoConfig.runtime)
	}
	trace := &runeApplyTrace{}
	recordRuneApplyResponse(trace, "update", json.RawMessage(`{"name":"private name","updated":true}`))
	if strings.Contains(trace.Responses["update"], "private name") {
		t.Fatal(trace.Responses)
	}
}

func TestR300StarterCountsSurviveClientRecommendedFile(t *testing.T) {
	request := gameplayItemSetApplyRequest{ChampionID: 1, MapID: 11, Position: "other", Blocks: []gameplayItemSetBlockRequest{
		{Type: "出门装", Items: []gameplayItemSetItemRequest{{ID: 2003, Count: 10}}},
		{Type: "核心出装", Items: []gameplayItemSetItemRequest{{ID: 3000, Count: 1}, {ID: 3001, Count: 1}}},
		{Type: "备选", Items: []gameplayItemSetItemRequest{{ID: 3002, Count: 1}}},
	}}
	if err := validateGameplayItemSetRequest(request); err != nil {
		t.Fatal(err)
	}
	set, _ := newLCUItemSet(request, nil)
	location := r71GameLocation(t)
	trace := &itemSetWriteTrace{}
	if err := writeRecommendedItemSetTraced(location, asRecommendedItemSet(set), nil, trace); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(location.configRoot, "Global", "Recommended", recommendedItemSetUID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Blocks []struct {
			Type  string
			Items []lcuItemSetItem
		}
	}
	if err = json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Blocks) != 3 || doc.Blocks[0].Type != "出门装" || doc.Blocks[0].Items[0].Count != 10 || doc.Blocks[0].Items[0].ID != "2003" || doc.Blocks[1].Type != "核心出装" || doc.Blocks[1].Items[0].ID != "3000" || doc.Blocks[1].Items[1].ID != "3001" || doc.Blocks[2].Type != "备选" {
		t.Fatal(string(raw))
	}
	request.Blocks[0].Type = "核心出装"
	if validateGameplayItemSetRequest(request) == nil {
		t.Fatal("large count escaped starter block")
	}
}
