package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestR298ModeMapping(t *testing.T) {
	cases := map[string][]int64{"ranked": {420, 440}, "normal": {400, 430, 480, 490, 700, 820, 830, 840, 850, 860, 870, 880, 890, 950, 960}, "aram": {450, 930, 3220, 720}, "hextech": {2300, 2400, 3270}, "urf": {900, 1900}, "event": {1300, 1400}, "": {1700, 1710, 1750}}
	for want, queues := range cases {
		for _, q := range queues {
			if got := autoConfigMode(q, "", 0); got != want {
				t.Errorf("queue %d=%s want %s", q, got, want)
			}
		}
	}
	for mode, want := range map[string]string{"KIWI": "hextech", "ARAM_MAYHEM": "hextech", "ULTBOOK": "event", "NEXUSBLITZ": "event", "CLASSIC": "normal", "CHERRY": ""} {
		if got := autoConfigMode(0, mode, 11); got != want {
			t.Errorf("mode %s=%s want %s", mode, got, want)
		}
	}
	if autoConfigMode(0, "", 11) != "normal" {
		t.Error("custom map 11 must use normal")
	}
}
func TestR298LockAndFlash(t *testing.T) {
	cell := int64(7)
	s := lcuChampSelectSession{LocalPlayerCellID: &cell, MyTeam: []lcuChampSelectPlayer{{CellID: &cell, ChampionPickIntent: 1}}}
	if autoConfigLocked(s, "ranked") {
		t.Fatal("intent triggered")
	}
	s.MyTeam[0].ChampionID = 1
	s.Actions = [][]lcuChampSelectAction{{{Type: "pick", ActorCellID: 7, ChampionID: 1}}}
	if autoConfigLocked(s, "ranked") {
		t.Fatal("hover triggered")
	}
	s.Actions[0][0].Completed = true
	if !autoConfigLocked(s, "ranked") {
		t.Fatal("locked champion did not trigger")
	}
	s.Actions = nil
	if !autoConfigLocked(s, "aram") {
		t.Fatal("ARAM should confirm champion")
	}
	for _, c := range []struct {
		a, b int64
		slot *int
		want [2]int64
	}{{4, 14, nil, [2]int64{4, 14}}, {4, 14, intPtrR298(1), [2]int64{14, 4}}, {14, 4, intPtrR298(0), [2]int64{4, 14}}, {14, 11, intPtrR298(0), [2]int64{14, 11}}} {
		a, b := reorderFlashSpells(c.a, c.b, c.slot)
		if [2]int64{a, b} != c.want {
			t.Fatal("flash order", a, b, c.want)
		}
	}
}
func intPtrR298(v int) *int { return &v }
func TestR298Bindings(t *testing.T) {
	for raw, want := range map[string]string{"[d]": "D", "[Shift][d]": "Shift+D", "[n],[1]": "N", "": "", "[<Unbound>]": ""} {
		if got := formatSummonerBinding(raw); got != want {
			t.Errorf("%s=%s want %s", raw, got, want)
		}
	}
	keys := summonerKeysFromJSON([]byte(`{"GameEvents":{"evtCastAvatarSpell1":"[d]","evtSmartCastAvatarSpell1":"[n],[1]","evtCastAvatarSpell2":"[m]"}}`))
	if keys != [2]string{"N", "M"} {
		t.Fatal(keys)
	}
	keys = summonerKeysFromValues(allSettingsFromINI([]byte("[GameEvents]\nevtCastAvatarSpell1=[Shift][d]\nevtCastAvatarSpell2=[f]"), false))
	if keys != [2]string{"Shift+D", "F"} {
		t.Fatal(keys)
	}
}
func TestR298StorageVersionCopyAndFallback(t *testing.T) {
	dir := t.TempDir()
	a := &app{storage: &localStore{root: dir}}
	s := a.autoConfigSnapshot()
	if s.Enabled || !s.Runes || !s.Spells || !s.Items {
		t.Fatal(s)
	}
	s.Enabled = true
	s.Champions["1"] = map[string]autoConfigEntry{"ranked": {Runes: &autoConfigRunes{PrimaryStyleID: 8000, SubStyleID: 8100, SelectedPerkIDs: []int64{1, 0}}}}
	put := func(s autoConfigSettings) *httptest.ResponseRecorder {
		b, _ := json.Marshal(s)
		w := httptest.NewRecorder()
		a.handleAutoConfigSettings(w, httptest.NewRequest("PUT", "/", strings.NewReader(string(b))))
		return w
	}
	if w := put(s); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := put(s); w.Code != 409 {
		t.Fatal("no conflict", w.Code)
	}
	w := httptest.NewRecorder()
	a.handleAutoConfigCopy(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"championId":1,"from":"ranked","targets":["hextech","normal"]}`)))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	next := a.autoConfigSnapshot()
	if next.Champions["1"]["hextech"].Runes != nil || next.Champions["1"]["normal"].Runes == nil {
		t.Fatal(next)
	}
	fresh := &app{storage: a.storage}
	if !fresh.autoConfigSnapshot().Enabled {
		t.Fatal("not persisted")
	}
	_ = os.WriteFile(filepath.Join(dir, "auto-config.json"), []byte("broken"), 0600)
	fresh = &app{storage: a.storage}
	if fresh.autoConfigSnapshot().Enabled {
		t.Fatal("bad file did not reset")
	}
	if validateAutoConfigEntry("hextech", s.Champions["1"]["ranked"]) == nil {
		t.Fatal("hextech accepted runes")
	}
	fallback, key, ok := autoConfigEntryFor(s, 1, "ranked", "middle")
	if !ok || key != "ranked" || fallback.Runes == nil {
		t.Fatal("ranked fallback")
	}
}
func TestR298RuneCompleteness(t *testing.T) {
	c := r298Catalog()
	r := &autoConfigRunes{PrimaryStyleID: 8000, SubStyleID: 8100, SelectedPerkIDs: []int64{1, 2, 3, 4, 12, 13, 5008, 5008, 5001}}
	if !autoConfigRunesComplete(r, c) {
		t.Fatal("valid page rejected")
	}
	r.SelectedPerkIDs[5] = 12
	if autoConfigRunesComplete(r, c) {
		t.Fatal("duplicate secondary row accepted")
	}
	r.SelectedPerkIDs[5] = 13
	r.SelectedPerkIDs[0] = 99
	if autoConfigRunesComplete(r, c) {
		t.Fatal("obsolete rune accepted")
	}
}
func r298Catalog() autoConfigCatalog {
	slot := func(ids ...int64) gameplayPerkSlot {
		s := gameplayPerkSlot{}
		for _, id := range ids {
			s.Perks = append(s.Perks, gameplayPerk{ID: id, Name: "符文"})
		}
		return s
	}
	return autoConfigCatalog{Styles: []gameplayPerkStyle{{ID: 8000, Slots: []gameplayPerkSlot{slot(1), slot(2), slot(3), slot(4)}}, {ID: 8100, Slots: []gameplayPerkSlot{slot(11), slot(12), slot(13), slot(14)}}}, StatModSlots: []gameplayPerkSlot{slot(5008), slot(5008), slot(5001)}, Spells: []autoConfigSpell{{ID: 4, GameModes: []string{"CLASSIC", "ARAM"}}, {ID: 14, GameModes: []string{"CLASSIC", "ARAM"}}}, Items: []autoConfigItem{{ID: 3000}}}
}
func TestR298LateRuneAckAndManualYield(t *testing.T) {
	a := &app{}
	expected := &autoConfigRunes{PrimaryStyleID: 8000, SubStyleID: 8100, SelectedPerkIDs: []int64{1, 2, 3}}
	a.autoConfig.runtime = autoConfigRuntime{PageID: 77, Results: map[string]string{"runes": "applied"}, expectedRunes: expected, done: true}
	send := func(uri, data string) {
		a.observeAutoConfigEvent(context.Background(), nil, LCUEvent{URI: uri, Data: json.RawMessage(data), EventType: "Update"})
	}
	send("/lol-perks/v1/pages/77", `{"id":77,"primaryStyleId":8000,"subStyleId":8100,"selectedPerkIds":[1,2,3]}`)
	if a.autoConfig.runtime.Yielded["runes"] {
		t.Fatal("own late ack yielded")
	}
	send("/lol-perks/v1/pages/88", `{"id":88}`)
	if a.autoConfig.runtime.Yielded["runes"] {
		t.Fatal("unselected page yielded")
	}
	send("/lol-perks/v1/currentpage", `88`)
	if !a.autoConfig.runtime.Yielded["runes"] {
		t.Fatal("manual page switch did not yield")
	}
}
func TestR298GuardAndApplyOnce(t *testing.T) {
	var mu sync.Mutex
	cell := int64(7)
	s := lcuChampSelectSession{GameID: 42, QueueID: 420, LocalPlayerCellID: &cell, MyTeam: []lcuChampSelectPlayer{{CellID: &cell, ChampionID: 1, Spell1ID: 14, Spell2ID: 4}}, Actions: [][]lcuChampSelectAction{{{ActorCellID: 7, Type: "pick", ChampionID: 1}}}}
	phase := "ChampSelect"
	writes := [][2]int64{}
	client := &LCUClient{baseURL: "https://127.0.0.1", token: "test", http: &http.Client{Transport: gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		var data any
		switch req.URL.Path {
		case "/lol-gameflow/v1/gameflow-phase":
			data = phase
		case "/lol-champ-select/v1/session":
			data = s
		case "/lol-gameflow/v1/session":
			data = lcuGameflowSession{}
		case "/lol-champ-select/v1/session/my-selection":
			var pair autoConfigSpells
			_ = json.NewDecoder(req.Body).Decode(&pair)
			writes = append(writes, [2]int64{pair.Spell1ID, pair.Spell2ID})
			s.MyTeam[0].Spell1ID = pair.Spell1ID
			s.MyTeam[0].Spell2ID = pair.Spell2ID
		default:
			t.Errorf("unexpected %s", req.URL.Path)
		}
		b, _ := json.Marshal(data)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(b)))}, nil
	})}}
	a := &app{connected: true, lcu: client}
	settings := defaultAutoConfig()
	settings.Enabled = true
	settings.Champions["1"] = map[string]autoConfigEntry{"ranked": {Spells: &autoConfigSpells{4, 14}}}
	a.autoConfig.settings = settings
	a.autoConfig.loaded = true
	c := r298Catalog()
	a.autoConfig.catalog = &c
	a.autoConfig.catalogClient = client
	a.autoConfig.catalogKey, _ = clientCatalogKey(client)
	a.autoConfig.catalogAt = time.Now()
	a.autoConfig.keys.FlashSlot = intPtrR298(1)
	wait := func() {
		t.Helper()
		for i := 0; i < 200; i++ {
			a.autoConfig.Lock()
			busy := a.autoConfig.processing
			a.autoConfig.Unlock()
			if !busy {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatal("did not finish")
	}
	a.triggerAutoConfig(context.Background(), client)
	wait()
	mu.Lock()
	if len(writes) != 0 {
		t.Fatal("hover wrote")
	}
	s.Actions[0][0].Completed = true
	mu.Unlock()
	a.triggerAutoConfig(context.Background(), client)
	wait()
	a.triggerAutoConfig(context.Background(), client)
	wait()
	mu.Lock()
	if !reflect.DeepEqual(writes, [][2]int64{{4, 14}}) {
		t.Fatal("automatic order or once", writes)
	}
	snapshot := s
	s.Actions[0][0].Completed = false
	mu.Unlock()
	if a.autoConfigGuard(context.Background(), client, snapshot, a.autoConfig.runtime.generation, 1) == nil {
		t.Fatal("revoked lock passed guard")
	}
	mu.Lock()
	s.Actions[0][0].Completed = true
	phase = "GameStart"
	mu.Unlock()
	if a.autoConfigGuard(context.Background(), client, snapshot, a.autoConfig.runtime.generation, 1) == nil {
		t.Fatal("phase changed passed guard")
	}
}

func TestR298ARAMDebounceLatestChampionAndYield(t *testing.T) {
	var mu sync.Mutex
	cell := int64(7)
	session := lcuChampSelectSession{GameID: 42, QueueID: 450, LocalPlayerCellID: &cell, MyTeam: []lcuChampSelectPlayer{{CellID: &cell, ChampionID: 1}}}
	var writes []time.Time
	client := &LCUClient{baseURL: "https://127.0.0.1", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		var data any
		switch req.URL.Path {
		case "/lol-champ-select/v1/session":
			data = session
		case "/lol-gameflow/v1/session":
			data = lcuGameflowSession{}
		case "/lol-gameflow/v1/gameflow-phase":
			data = "ChampSelect"
		case "/lol-champ-select/v1/session/my-selection":
			var pair autoConfigSpells
			_ = json.NewDecoder(req.Body).Decode(&pair)
			if session.MyTeam[0].ChampionID != 2 {
				t.Error("wrote stale champion")
			}
			session.MyTeam[0].Spell1ID = pair.Spell1ID
			session.MyTeam[0].Spell2ID = pair.Spell2ID
			writes = append(writes, time.Now())
		default:
			t.Errorf("unexpected %s", req.URL.Path)
		}
		b, _ := json.Marshal(data)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b)))}, nil
	})}}
	a := &app{connected: true, lcu: client}
	settings := defaultAutoConfig()
	settings.Enabled = true
	settings.Champions["1"] = map[string]autoConfigEntry{"aram": {Spells: &autoConfigSpells{4, 14}}}
	settings.Champions["2"] = settings.Champions["1"]
	a.autoConfig.settings = settings
	a.autoConfig.loaded = true
	c := r298Catalog()
	a.autoConfig.catalog = &c
	a.autoConfig.catalogClient = client
	a.autoConfig.catalogKey, _ = clientCatalogKey(client)
	a.autoConfig.catalogAt = time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.triggerAutoConfig(ctx, client)
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	session.MyTeam[0].ChampionID = 2
	raw, _ := json.Marshal(session)
	mu.Unlock()
	changed := time.Now()
	a.observeAutoConfigEvent(ctx, client, LCUEvent{URI: "/lol-champ-select/v1/session", Data: raw})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		done := len(writes) > 0
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	if len(writes) != 1 {
		t.Fatalf("writes=%d runtime=%+v", len(writes), a.autoConfig.runtime)
	}
	elapsed := writes[0].Sub(changed)
	mu.Unlock()
	if elapsed < 1490*time.Millisecond {
		t.Fatalf("did not debounce: %s", elapsed)
	}
	for i := 0; i < 100; i++ {
		a.autoConfig.Lock()
		busy := a.autoConfig.processing
		a.autoConfig.Unlock()
		if !busy {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	mu.Lock()
	session.MyTeam[0].Spell1ID = 14
	session.MyTeam[0].Spell2ID = 4
	mu.Unlock()
	a.triggerAutoConfig(ctx, client)
	for i := 0; i < 100; i++ {
		a.autoConfig.Lock()
		busy := a.autoConfig.processing
		yielded := a.autoConfig.runtime.Yielded["spells"]
		a.autoConfig.Unlock()
		if !busy {
			if !yielded {
				t.Fatal("manual spells did not yield")
			}
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	a.resetAutoConfig()
	if len(a.autoConfigYieldSnapshot()) != 0 {
		t.Fatal("new session did not restore categories")
	}
}

func TestR298AppliedStateSurvivesGameStart(t *testing.T) {
	a := &app{}
	a.autoConfig.runtime = autoConfigRuntime{Session: "session", ChampionID: 1, Results: map[string]string{"runes": "applied"}}
	a.observeAutoConfigEvent(context.Background(), nil, LCUEvent{URI: "/lol-gameflow/v1/gameflow-phase", Data: json.RawMessage(`"GameStart"`)})
	if a.autoConfig.runtime.Results["runes"] != "applied" || a.autoConfig.runtime.ChampionID != 1 {
		t.Fatal("applied state lost when entering game")
	}
	a.observeAutoConfigEvent(context.Background(), nil, LCUEvent{URI: "/lol-gameflow/v1/gameflow-phase", Data: json.RawMessage(`"Lobby"`)})
	if a.autoConfig.runtime.ChampionID != 0 {
		t.Fatal("old game state survived return to lobby")
	}
}
