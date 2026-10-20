package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type r298ReviewLCU struct {
	mu      sync.Mutex
	session lcuChampSelectSession
	flow    lcuGameflowSession
	writes  []autoConfigSpells
}

func r298ReviewFixture(t *testing.T) (autoConfigCatalog, map[string][]int64) {
	t.Helper()
	data, err := os.ReadFile("testdata/r298-review-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Catalog            autoConfigCatalog  `json:"catalog"`
		ExpectedSpellPools map[string][]int64 `json:"expectedSpellPools"`
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture.Catalog, fixture.ExpectedSpellPools
}

func r298ReviewApp(t *testing.T, state *r298ReviewLCU) (*app, *LCUClient) {
	t.Helper()
	client := &LCUClient{baseURL: "https://127.0.0.1", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		state.mu.Lock()
		defer state.mu.Unlock()
		var data any
		switch req.URL.Path {
		case "/lol-gameflow/v1/gameflow-phase":
			data = "ChampSelect"
		case "/lol-gameflow/v1/session":
			data = state.flow
		case "/lol-champ-select/v1/session":
			data = state.session
		case "/lol-champ-select/v1/session/my-selection":
			if req.Method != http.MethodPatch {
				t.Errorf("selection write method = %s, want PATCH", req.Method)
			}
			var pair autoConfigSpells
			if err := json.NewDecoder(req.Body).Decode(&pair); err != nil {
				t.Error(err)
			}
			state.writes = append(state.writes, pair)
			state.session.MyTeam[0].Spell1ID, state.session.MyTeam[0].Spell2ID = pair.Spell1ID, pair.Spell2ID
		default:
			t.Errorf("unexpected %s %s", req.Method, req.URL.Path)
		}
		body, _ := json.Marshal(data)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})}}
	a := &app{connected: true, lcu: client}
	a.autoConfig.settings = defaultAutoConfig()
	a.autoConfig.loaded = true
	catalog, _ := r298ReviewFixture(t)
	a.autoConfig.catalog, a.autoConfig.catalogClient = &catalog, client
	a.autoConfig.catalogKey, _ = clientCatalogKey(client)
	a.autoConfig.catalogAt = time.Now()
	return a, client
}

func TestR298ReviewGameflowModeAppliesThroughTrigger(t *testing.T) {
	for _, c := range []struct {
		name, gameMode, want string
		queue                int64
	}{
		{"flow queue ARAM", "", "aram", 450},
		{"flow queue hextech", "", "hextech", 2400},
		{"flow map ARAM", "ARAM", "aram", 0},
		{"flow map KIWI", "KIWI", "hextech", 0},
		{"flow map ARAM_MAYHEM", "ARAM_MAYHEM", "hextech", 0},
		{"flow queue gameMode", "KIWI", "hextech", 0},
		{"flow ranked hover", "CLASSIC", "", 420},
		{"flow normal hover", "CLASSIC", "", 430},
	} {
		t.Run(c.name, func(t *testing.T) {
			cell := int64(7)
			state := &r298ReviewLCU{session: lcuChampSelectSession{GameID: 298, LocalPlayerCellID: &cell, MyTeam: []lcuChampSelectPlayer{{CellID: &cell, ChampionID: 1}}}}
			state.flow.GameData.Queue.ID = c.queue
			state.flow.Map.ID, state.flow.Map.GameMode = 12, c.gameMode
			if c.name == "flow queue gameMode" {
				state.flow.GameData.Queue.MapID, state.flow.GameData.Queue.GameMode = 12, c.gameMode
				state.flow.Map.ID, state.flow.Map.GameMode = 0, ""
			}
			a, client := r298ReviewApp(t, state)
			a.autoConfig.settings.Enabled = true
			a.autoConfig.settings.Champions["1"] = map[string]autoConfigEntry{}
			for _, mode := range []string{"ranked", "normal", "aram", "hextech"} {
				a.autoConfig.settings.Champions["1"][mode] = autoConfigEntry{Spells: &autoConfigSpells{4, 14}}
			}
			a.autoConfig.keys.FlashSlot = intPtrR298(1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			a.triggerAutoConfig(ctx, client)
			deadline := time.Now().Add(4 * time.Second)
			for {
				a.autoConfig.Lock()
				busy := a.autoConfig.processing
				a.autoConfig.Unlock()
				if !busy {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("automatic configuration did not finish")
				}
				time.Sleep(5 * time.Millisecond)
			}
			state.mu.Lock()
			writes := append([]autoConfigSpells(nil), state.writes...)
			state.mu.Unlock()
			if c.want == "" {
				if len(writes) != 0 {
					t.Fatal("ordinary hover applied spells", writes)
				}
				return
			}
			if !reflect.DeepEqual(writes, []autoConfigSpells{{4, 14}}) {
				t.Fatalf("gameflow-only %s did not apply configured order: %v", c.want, writes)
			}
			if a.autoConfig.runtime.ModeKey != c.want {
				t.Fatal("wrong applied mode", a.autoConfig.runtime.ModeKey)
			}
		})
	}
}

func r298ReviewSave(a *app, mode string, entry autoConfigEntry) *httptest.ResponseRecorder {
	body, _ := json.Marshal(entry)
	req := httptest.NewRequest("PUT", "/api/auto-config/champions/1/"+mode, strings.NewReader(string(body)))
	req.SetPathValue("id", "1")
	req.SetPathValue("modeKey", mode)
	w := httptest.NewRecorder()
	a.handleAutoConfigChampion(w, req)
	return w
}

func TestR298ReviewChangedGameflowModeCancelsPendingWrite(t *testing.T) {
	cell := int64(7)
	state := &r298ReviewLCU{session: lcuChampSelectSession{GameID: 298, LocalPlayerCellID: &cell, MyTeam: []lcuChampSelectPlayer{{CellID: &cell, ChampionID: 1}}, Actions: [][]lcuChampSelectAction{{{ActorCellID: cell, ChampionID: 1, Type: "pick", Completed: true}}}}}
	state.flow.GameData.Queue.ID = 450
	a, client := r298ReviewApp(t, state)
	a.autoConfig.settings.Enabled = true
	a.autoConfig.settings.Champions["1"] = map[string]autoConfigEntry{"aram": {Spells: &autoConfigSpells{4, 14}}, "ranked": {Spells: &autoConfigSpells{14, 4}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.triggerAutoConfig(ctx, client)
	deadline := time.Now().Add(4 * time.Second)
	for {
		a.autoConfig.Lock()
		observed := a.autoConfig.runtime.observed
		a.autoConfig.Unlock()
		if observed == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("initial gameflow was not observed")
		}
		time.Sleep(5 * time.Millisecond)
	}
	state.mu.Lock()
	state.flow.GameData.Queue.ID = 420
	state.flow.Map.ID, state.flow.Map.GameMode = 11, "CLASSIC"
	state.mu.Unlock()
	for {
		a.autoConfig.Lock()
		busy := a.autoConfig.processing
		a.autoConfig.Unlock()
		if !busy {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("mode change cancellation did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.writes) != 0 {
		t.Fatal("old mode configuration was written after gameflow mode changed", state.writes)
	}
}

func TestR298ReviewActiveEventDoesNotChangeOtherSavedSpellPools(t *testing.T) {
	state := &r298ReviewLCU{}
	state.flow.Map.ID, state.flow.Map.GameMode = 11, "ULTBOOK"
	a, _ := r298ReviewApp(t, state)
	catalog, expected := r298ReviewFixture(t)
	for mode, allowed := range expected {
		for _, spell := range catalog.Spells {
			second := int64(14)
			if spell.ID == second {
				second = 4
			}
			w := r298ReviewSave(a, mode, autoConfigEntry{Spells: &autoConfigSpells{spell.ID, second}})
			want := false
			for _, id := range allowed {
				want = want || id == spell.ID
			}
			if (w.Code == http.StatusOK) != want {
				t.Errorf("%s spell %d: status=%d expected allowed=%v: %s", mode, spell.ID, w.Code, want, w.Body.String())
			}
		}
	}
}

func TestR298ReviewUnchangedObsoleteCategoriesDoNotBlockRuneEdits(t *testing.T) {
	a, _ := r298ReviewApp(t, &r298ReviewLCU{})
	old := autoConfigEntry{Spells: &autoConfigSpells{999, 998}, Items: &autoConfigItems{Core: []int64{999999}}}
	a.autoConfig.settings.Champions["1"] = map[string]autoConfigEntry{"ranked": old}
	edited := old
	edited.Runes = &autoConfigRunes{PrimaryStyleID: 8000, SubStyleID: 8100, SelectedPerkIDs: []int64{1, 2, 3, 4, 12, 13, 5008, 5008, 5001}}
	if w := r298ReviewSave(a, "ranked", edited); w.Code != http.StatusOK {
		t.Fatalf("unchanged obsolete spells/items blocked a rune edit: %d %s", w.Code, w.Body.String())
	}
	got := a.autoConfigSnapshot().Champions["1"]["ranked"]
	if !reflect.DeepEqual(got.Spells, old.Spells) || !reflect.DeepEqual(got.Items, old.Items) || !reflect.DeepEqual(got.Runes, edited.Runes) {
		t.Fatal("saving another category lost the obsolete values", got)
	}
	newInvalid := edited
	newInvalid.Spells = &autoConfigSpells{997, 996}
	if w := r298ReviewSave(a, "ranked", newInvalid); w.Code != http.StatusBadRequest {
		t.Fatal("new invalid spell pair was accepted", w.Code)
	}
	newInvalid = edited
	newInvalid.Items = &autoConfigItems{Core: []int64{999998}}
	if w := r298ReviewSave(a, "ranked", newInvalid); w.Code != http.StatusBadRequest {
		t.Fatal("new invalid items were accepted", w.Code)
	}
	// Repairing one obsolete category must not require repairing the other first.
	edited.Spells = &autoConfigSpells{4, 14}
	if w := r298ReviewSave(a, "ranked", edited); w.Code != http.StatusOK {
		t.Fatal("repairing spells was blocked by unchanged obsolete equipment", w.Code)
	}
}
