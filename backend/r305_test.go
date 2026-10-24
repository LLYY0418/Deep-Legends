package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestR305ModeResolutionSharedFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/r305-mode-resolution.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Name, Mode, Position, Want string
		Entries                    []string
		Exists                     bool
	}
	if json.Unmarshal(raw, &rows) != nil {
		t.Fatal("fixture")
	}
	for _, row := range rows {
		t.Run(row.Name, func(t *testing.T) {
			m := map[string]autoConfigEntry{}
			for _, k := range row.Entries {
				m[k] = autoConfigEntry{Spells: &autoConfigSpells{Spell1ID: 4, Spell2ID: 14}}
			}
			s := defaultAutoConfig()
			s.Champions = map[string]map[string]autoConfigEntry{"84": m}
			_, key, ok := autoConfigEntryFor(s, 84, row.Mode, row.Position)
			if key != row.Want || ok != row.Exists {
				t.Fatal(key, ok, row)
			}
		})
	}
}
func TestR305HiddenReferenceOnlyFreshPublicLiveClearsBlock(t *testing.T) {
	a := &app{storage: &localStore{root: t.TempDir()}, token: "fixture"}
	hidden := gameplayReference{PlayerRef: "11111111-1111-4111-8111-111111111111", TagBlocked: true, TagVisibilityKnown: true, TagSource: "live"}
	public := a.registerGameplayReferenceDetails(hidden)
	request := func() int {
		w := httptest.NewRecorder()
		a.handlePlayerTags(w, httptest.NewRequest(http.MethodPost, "/api/player-tags", bytes.NewBufferString(`{"op":"update","playerRef":"`+public+`","tags":["preset-1"]}`)))
		return w.Code
	}
	if request() != 404 {
		t.Fatal("hidden editable")
	}
	a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: hidden.PlayerRef, GameName: "historical"}, "history")
	if request() != 404 {
		t.Fatal("history unblocked hidden")
	}
	a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: hidden.PlayerRef, GameName: "public", TagVisibilityKnown: true}, "live")
	if request() != 200 {
		t.Fatal("public live blocked")
	}
	row := gameplayOverview{Player: gameplayPlayer{PlayerRef: hidden.PlayerRef, GameName: "public"}, RecentPlayers: []gameplayRecentPlayer{{PlayerRef: hidden.PlayerRef, DisplayName: "public"}}}
	a.publicizeOverviewReferences(&row)
	if !row.Player.Taggable || !row.RecentPlayers[0].Taggable {
		t.Fatal("taggable missing")
	}
}
func TestR305ArenaPartialPremadesIncludeSelfAndExcludeUnknown(t *testing.T) {
	signals := []int64{1, 1, 1, 2, 2, 2, 3, 3, 3, 4, 4, 5, 5, 6, 6, 0, 0, 0}
	players := []gameplayLivePlayer{}
	inputs := map[string]arenaPremadeInput{}
	for i, signal := range signals {
		ref := r263Ref(i)
		players = append(players, gameplayLivePlayer{gameplayPlayer: gameplayPlayer{PlayerRef: ref, IsCurrent: i == 0, reference: gameplayReference{PlayerRef: ref}}, MySquad: i < 3})
		inputs[ref] = arenaPremadeInput{TeamParticipantID: signal}
	}
	response := gameplayLiveResponse{GameID: 305, QueueID: 1750, Phase: "InProgress", Players: players}
	a := &app{}
	a.applyArenaPremadeSquads(&response, inputs)
	for i, p := range response.Players {
		if i < 15 {
			if p.PremadeGroup == "" || p.PremadeSize < 2 {
				t.Fatal(i, p)
			}
		} else if p.PremadeGroup != "" {
			t.Fatal("unknown tagged", i)
		}
	}
	if response.Players[9].PremadeGroup != response.Players[10].PremadeGroup || response.Players[0].ArenaSquadKey != "mine" {
		t.Fatal("group/mine")
	}
}
func TestR305ProgressionRestartExpiryAndConditionalDelete(t *testing.T) {
	a := r175App(t)
	a.summoner = Summoner{PUUID: "self"}
	c := &LCUClient{}
	p := &arenaProgressionProbe{account: a.storage.accountHash(a.summoner), platform: clientRiotPlatform(c) + "/" + clientTencentServerID(c), gameID: 305, at: time.Now(), lobbyAt: time.Now().Add(-3 * time.Minute), before: arenaProgressionSnapshot{Values: map[string]float64{"fame": 10}}}
	a.persistArenaProgression(p)
	restarted := &app{storage: a.storage, summoner: a.summoner}
	restored := restarted.restoreArenaProgression(c)
	if restored == nil || restored.before.Values["fame"] != 10 {
		t.Fatal("restart lost before")
	}
	s := c.arenaDiscoveryState()
	var delay time.Duration
	called := false
	s.schedule = func(d time.Duration, fn func()) func() { delay = d; called = true; return func() {} }
	restarted.observeArenaProgressionPhase(c, "Lobby")
	if !called || delay != 0 {
		t.Fatal("overdue lobby not immediate", delay)
	}
	newer := *p
	newer.gameID = 306
	newer.at = p.at.Add(time.Second)
	a.persistArenaProgression(&newer)
	a.clearArenaProgression(p)
	if a.restoreArenaProgression(c) == nil {
		t.Fatal("old callback deleted new snapshot")
	}
	newer.at = time.Now().Add(-31 * time.Minute)
	a.persistArenaProgression(&newer)
	if a.restoreArenaProgression(c) != nil {
		t.Fatal("expired restored")
	}
	if _, err := readLocalStoreFile(a.storage, "arena-progression-before.json"); !os.IsNotExist(err) {
		t.Fatal("expired file remains")
	}
	none := &LCUClient{}
	called = false
	none.arenaDiscoveryState().schedule = s.schedule
	a.observeArenaProgressionPhase(none, "Lobby")
	if called {
		t.Fatal("no snapshot scheduled")
	}
}
func TestR305ModeBoundariesPersistAcrossIndependentCompleteSaves(t *testing.T) {
	s := &localStore{root: t.TempDir()}
	base := seasonStatsCache{SchemaVersion: seasonStatsCacheSchemaVersion, Source: seasonStatsSource, Season: "S26", AccountHash: "account", Streams: map[string]seasonStatsStream{"ranked": {Complete: true, OldestCreatedAt: 12}, "flex": {Complete: true, CappedByUpstream: true, OldestCreatedAt: 40}, "mayhem": {Complete: true, CappedByUpstream: true, OldestCreatedAt: 30}}}
	if _, err := s.saveSeasonStatsReported(base); err != nil {
		t.Fatal(err)
	}
	base.Streams = map[string]seasonStatsStream{"ranked": {Complete: true, OldestCreatedAt: 20}, "flex": {Complete: true}, "mayhem": {Complete: true}}
	if _, err := s.saveSeasonStatsReported(base); err != nil {
		t.Fatal(err)
	}
	disk, err := s.loadSeasonStats(base.Source, base.AccountHash, base.Season)
	if err != nil {
		t.Fatal(err)
	}
	p := seasonStatsProgress{}
	p.applyUpstreamBoundary(disk)
	if p.SoloCapped || !p.FlexCapped || !p.MayhemCapped || p.SoloOldestAt != 12 || p.FlexOldestAt != 40 || p.MayhemOldestAt != 30 {
		t.Fatal(p)
	}
}

func TestR305PracticeLocksAndAppliesRankedFallbackSpells(t *testing.T) {
	var mu sync.Mutex
	cell := int64(7)
	session := lcuChampSelectSession{GameID: 305, QueueID: 3100, LocalPlayerCellID: &cell, MyTeam: []lcuChampSelectPlayer{{CellID: &cell, ChampionID: 84, Spell1ID: 14, Spell2ID: 4}}, Actions: [][]lcuChampSelectAction{{{ActorCellID: 7, Type: "pick", ChampionID: 84, Completed: true}}}}
	writes := 0
	c := &LCUClient{baseURL: "https://127.0.0.1", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		var value any
		switch r.URL.Path {
		case "/lol-gameflow/v1/gameflow-phase":
			value = "ChampSelect"
		case "/lol-champ-select/v1/session":
			value = session
		case "/lol-gameflow/v1/session":
			value = lcuGameflowSession{}
		case "/lol-champ-select/v1/session/my-selection":
			var pair autoConfigSpells
			if json.NewDecoder(r.Body).Decode(&pair) != nil {
				t.Error("pair")
			}
			session.MyTeam[0].Spell1ID, session.MyTeam[0].Spell2ID = pair.Spell1ID, pair.Spell2ID
			writes++
		default:
			t.Error("unexpected", r.URL.Path)
		}
		raw, _ := json.Marshal(value)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})}}
	a := &app{connected: true, lcu: c}
	settings := defaultAutoConfig()
	settings.Enabled = true
	settings.Champions["84"] = map[string]autoConfigEntry{"ranked": {Spells: &autoConfigSpells{4, 14}}}
	a.autoConfig.settings = settings
	a.autoConfig.loaded = true
	catalog := r298Catalog()
	a.autoConfig.catalog = &catalog
	a.autoConfig.catalogClient = c
	a.autoConfig.catalogKey, _ = clientCatalogKey(c)
	a.autoConfig.catalogAt = time.Now()
	a.triggerAutoConfig(context.Background(), c)
	deadline := time.Now().Add(time.Second)
	for {
		a.autoConfig.Lock()
		busy := a.autoConfig.processing
		result := a.autoConfig.runtime.Results["spells"]
		key := a.autoConfig.runtime.ModeKey
		a.autoConfig.Unlock()
		if !busy {
			if result != "applied" || key != "ranked" {
				t.Fatal(result, key)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("apply unfinished")
		}
		time.Sleep(time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if writes != 1 {
		t.Fatal("writes", writes)
	}
}

func TestR305TwelveOfficialSoloGamesAreNotAnUpstreamCap(t *testing.T) {
	a := &app{}
	c := &LCUClient{}
	a.rememberOfficialRankWins(c, "player", "HN1", []gameplayRank{{QueueType: "RANKED_SOLO_5x5", Wins: 6, Losses: 6}, {QueueType: "RANKED_FLEX_SR", Wins: 50, Losses: 50}})
	cache := seasonStatsCache{Streams: map[string]seasonStatsStream{"ranked": {Complete: true, CappedByUpstream: true, OldestCreatedAt: 12}, "flex": {Complete: true, CappedByUpstream: true, OldestCreatedAt: 40}, "mayhem": {Complete: true, CappedByUpstream: true, OldestCreatedAt: 30}}, QueueStats: map[int64]gameplayAggregate{420: {Games: 12}, 440: {Games: 40}}}
	a.captureSeasonOfficialTotals(&cache, c, "HN1", "player")
	p := seasonStatsProgress{}
	p.applyUpstreamBoundary(cache)
	if p.SoloCapped || !p.FlexCapped || !p.MayhemCapped {
		t.Fatal(p)
	}
}
