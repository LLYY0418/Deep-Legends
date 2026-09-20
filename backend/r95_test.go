package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR95SquadSurvivesHiddenChampSelectAndScrambledLive(t *testing.T) {
	players, raw := r62ArenaFixturePlayers(t, 18)
	var entries []map[string]any
	_ = json.Unmarshal(raw, &entries)
	entries[2], entries[7] = entries[7], entries[2]
	raw, _ = json.Marshal(entries)
	a := r90Fixture(t, players, raw)
	base := a.lcu.http.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	team := make([]map[string]any, 18)
	for i := range team {
		team[i] = map[string]any{"cellId": i, "team": 1, "nameVisibilityType": "HIDDEN", "obfuscatedPuuid": ""}
		if i < 3 {
			team[i]["nameVisibilityType"] = "UNHIDDEN"
			team[i]["puuid"] = fmt.Sprintf("r90-player-identity-%02d", i+1)
			team[i]["summonerId"] = i + 1
			team[i]["gameName"] = fmt.Sprintf("R62Arena%02d", i+1)
			team[i]["tagLine"] = "CN1"
			team[i]["championId"] = i + 1
		}
	}
	a.lcu.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/lol-champ-select/v1/session":
			return response2351(map[string]any{"gameId": 90001, "queueId": 1750, "localPlayerCellId": 0, "myTeam": team}), nil
		case "/lol-lobby/v2/lobby":
			return response2351(map[string]any{"gameConfig": map[string]any{"queueId": 1750, "mapId": 30, "gameMode": "CHERRY"}}), nil
		}
		return base.RoundTrip(r)
	})
	a.liveClientAllGameData = func(context.Context) ([]byte, int, error) { return []byte(`{"allPlayers":[]}`), 200, nil }
	for _, phase := range []string{"ChampSelect", "GameStart", "InProgress", "Reconnect"} {
		got := a.loadGameplayLive(context.Background(), a.lcu, a.summoner, phase)
		marked := 0
		for _, p := range got.Players {
			if p.MySquad {
				marked++
			}
			if p.ArenaGroup != "" {
				t.Fatalf("%s guessed group %q", phase, p.ArenaGroup)
			}
		}
		if marked != 3 || got.ArenaGrouped {
			t.Fatalf("%s marked=%d grouped=%t", phase, marked, got.ArenaGrouped)
		}
		if phase == "ChampSelect" && len(got.Players) != 3 {
			t.Fatalf("hidden players leaked: %d", len(got.Players))
		}
	}
	shapes := r90Events(t, a, "lcu_champ_select_session_shape")
	if len(shapes) == 0 {
		t.Fatal("missing CS shape")
	}
	e := shapes[0]
	if !reflect.DeepEqual(e["my_team_unhidden_cell_ids"], []any{float64(0), float64(1), float64(2)}) || len(e["my_team_cell_id_sequence"].([]any)) != 18 {
		t.Fatal(e)
	}
	a.clearArenaAllies()
	got := a.loadGameplayLive(context.Background(), a.lcu, a.summoner, "Reconnect")
	for _, p := range got.Players {
		if p.MySquad {
			t.Fatal("stale squad after clear")
		}
	}
}

func TestR95MissingKnownSquadMemberAndTruth(t *testing.T) {
	for _, wrong := range []bool{false, true} {
		t.Run(fmt.Sprint(wrong), func(t *testing.T) {
			players, raw := r62ArenaFixturePlayers(t, 18)
			players = players[1:]
			a := r90Fixture(t, players, raw)
			a.summoner.GameName = "R62Arena01"
			a.summoner.TagLine = "CN1"
			r90RememberAllies(a, 0, 3)
			a.lcu.platformProbe = true
			a.lcu.region = "TENCENT"
			a.lcu.rsoPlatform = "HN1"
			got := a.loadGameplayLive(context.Background(), a.lcu, a.summoner, "InProgress")
			marked := 0
			for _, p := range got.Players {
				if p.MySquad {
					marked++
				}
			}
			if len(got.Players) != 18 || marked != 3 {
				t.Fatalf("restored roster=%d marked=%d", len(got.Players), marked)
			}
			missing := r90Events(t, a, "session_missing_from_playerlist")
			if len(missing) != 1 {
				t.Fatal(missing)
			}
			list := missing[0]["missing"].([]any)
			if len(list) != 1 || list[0].(map[string]any)["index"] != float64(0) || list[0].(map[string]any)["is_current"] != true {
				t.Fatal(list)
			}
			info := r90TruthInfo()
			if wrong {
				info.Participants[2].PlayerSubteamID = 2
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"games": []map[string]any{{"json": info}}})
			}))
			defer server.Close()
			provider := newSGPProvider()
			provider.http = server.Client()
			provider.serverBases["HN1"] = server.URL
			provider.token = "fixture"
			provider.tokenAt = time.Now()
			provider.tokenClient = a.lcu
			a.sgp = provider
			a.finishArenaGroupTruth(context.Background(), a.lcu)
			events := r90Events(t, a, "arena_group_truth_check")
			if len(events) != 1 {
				t.Fatal(events)
			}
			e := events[0]
			if e["truth_source"] != "sgp" || e["my_squad_correct"] != !wrong || e["conclusive"] != true || e["session_order_is_squad_order"] != false || len(e["subteam_id_by_index"].([]any)) != 18 {
				t.Fatal(e)
			}
		})
	}
}

func TestR95TruthNoneAndRiotSource(t *testing.T) {
	players, raw := r62ArenaFixturePlayers(t, 18)
	a := r90Fixture(t, players, raw)
	r90RememberAllies(a, 0, 3)
	a.loadGameplayLive(context.Background(), a.lcu, a.summoner, "InProgress")
	a.finishArenaGroupTruth(context.Background(), a.lcu)
	events := r90Events(t, a, "arena_group_truth_check")
	if len(events) != 1 || events[0]["truth_source"] != "none" || events[0]["my_squad_correct"] != false {
		t.Fatal(events)
	}
	a.arenaTruth.mu.Lock()
	a.arenaTruth.records[0].serverID = "KR"
	a.arenaTruth.mu.Unlock()
	match := &riotMatch{Info: *r90TruthInfo()}
	match.Metadata.MatchID = "KR_90001"
	a.checkArenaRiotMatchTruth(match)
	events = r90Events(t, a, "arena_group_truth_check")
	if len(events) != 2 || events[1]["truth_source"] != "riot-match-v5" || events[1]["my_squad_correct"] != true {
		t.Fatal(events)
	}
}

func TestR95AllGameDataOncePrivateAndCanceledParent(t *testing.T) {
	players, raw := r62ArenaFixturePlayers(t, 18)
	a := r90Fixture(t, players, raw)
	var calls atomic.Int32
	a.liveClientAllGameData = func(ctx context.Context) ([]byte, int, error) {
		calls.Add(1)
		if ctx.Err() != nil {
			t.Error("inherited cancellation")
		}
		return []byte(`{"allPlayers":[{"riotId":"Secret#123","subteamId":2,"SecretDynamicKey":5,"items":[{"itemID":1}]}],"activePlayer":{"summonerName":"Private"}}`), 200, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.sampleArenaAllGameData(ctx, 1)
	a.sampleArenaAllGameData(ctx, 1)
	a.sampleArenaAllGameData(ctx, 2)
	events := r90Events(t, a, "live_client_allgamedata_shape")
	if calls.Load() != 2 || len(events) != 2 {
		t.Fatal(calls.Load(), events)
	}
	bytes, _ := json.Marshal(events)
	for _, secret := range []string{"Secret", "Private"} {
		if strings.Contains(string(bytes), secret) {
			t.Fatal("identity leak", string(bytes))
		}
	}
	if !strings.Contains(string(bytes), "subteamId") || !strings.Contains(string(bytes), "itemID") {
		t.Fatal("lost array shape", string(bytes))
	}
	for n := 0; n < 6; n++ {
		if arenaChampOrderConclusive(n, 3) {
			t.Fatal("insufficient comparison", n)
		}
	}
	if !arenaChampOrderConclusive(6, 3) {
		t.Fatal("six should be conclusive")
	}
}

func TestR95OverviewEmptyArraysAreExplicit(t *testing.T) {
	for _, v := range []gameplayOverview{{}, {Ranks: []gameplayRank{}, Masteries: []gameplayMastery{}, RecentPlayers: []gameplayRecentPlayer{}}} {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(raw, &fields)
		for _, key := range []string{"ranks", "masteries", "recentPlayers"} {
			if _, ok := fields[key]; !ok {
				t.Fatal("omitempty loses explicit complete clearing", key)
			}
		}
	}
}

func TestR95ProExactIdentityPrivacyAndNoFetch(t *testing.T) {
	a := &app{}
	account := func(name, tag, puuid string) opggProAccount {
		p := proFixtureAccount(name, puuid, "", 0, 0)
		p.TagLine = tag
		return p
	}
	a.proPlayers.teams = []opggProTeam{
		{ID: 9991, Name: "T1 Esports Academy", ShortName: "T1 Academy", Members: []opggProMember{proFixtureMember(9991, "Guti", "문정환", account("Faker", "구라티", "secret-guti"))}},
		{ID: 385, Name: "T1", ShortName: "T1", Members: []opggProMember{proFixtureMember(385, "Faker", "이상혁", account("Hide on bush", "KR1", "secret-faker"))}},
		{ID: 416, Name: "Gen.G", ShortName: "GEN", Members: []opggProMember{proFixtureMember(416, "Chovy", "정지훈", account("ChovyAccount", "KR1", "secret-chovy"))}},
	}
	idx := a.proIdentitySnapshot()
	for _, tc := range []struct {
		name, tag, region, puuid, want string
		secondary                      bool
	}{{"Faker", "구라티", "kr", "", "", false}, {"hide ON BUSH", "kr1", "KR", "", "Faker", false}, {"Faker", "different", "kr", "", "", false}, {"Faker", "구라티", "cn", "", "", false}, {"Hide on bush", "KR1", "kr", "secret-chovy", "Chovy", false}} {
		badge := a.matchProIdentity(idx, "test", gameplayReference{GameName: tc.name, TagLine: tc.tag, Region: tc.region, PlayerRef: tc.puuid})
		if tc.want == "" {
			if badge != nil {
				t.Fatal("false positive", tc, badge)
			}
			continue
		}
		if badge == nil || badge.PlayerName != tc.want || badge.Secondary != tc.secondary {
			t.Fatal(tc, badge)
		}
		wantTeam := "T1"
		if tc.want == "Chovy" {
			wantTeam = "GEN"
		}
		if badge.TeamCode != wantTeam {
			t.Fatal("wrong team", badge)
		}
		raw, _ := json.Marshal(badge)
		if strings.Contains(strings.ToLower(string(raw)), "puuid") || strings.Contains(string(raw), "secret-") {
			t.Fatal("privacy", string(raw))
		}
	}
	// A match only reads the snapshot; an empty cache must never initiate HTTP.
	var calls atomic.Int32
	a.champions = newChampionProvider()
	a.champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(*http.Request) (*http.Response, error) { calls.Add(1); return response2351(nil), nil })}
	a.proPlayers.teams = nil
	if a.matchProIdentity(a.proIdentitySnapshot(), "live", gameplayReference{Region: "kr", GameName: "Faker", TagLine: "구라티"}) != nil || calls.Load() != 0 {
		t.Fatal("empty snapshot fetched")
	}
}

func TestR95LockStrategiesUseOnlyVisibleWait(t *testing.T) {
	for _, strategy := range []string{"lock-now", "show-only", "show-then-lock"} {
		t.Run(strategy, func(t *testing.T) {
			f := newExecutionFixture(t)
			f.session.Timer.Phase = "BAN_PICK"
			f.configure(true, false, false, strategy)
			s := f.r.currentWatch()
			g := s.ChampSelect.Groups["practice"]
			g.Ban.DelayMS = 2000
			wait := 120
			g.Ban.LockDelayMS = &wait
			s.ChampSelect.Groups["practice"] = g
			f.r.apply(s)
			if got := f.r.currentWatch().ChampSelect.Groups["practice"].Ban.LockDelayMS; got == nil || *got != 120 {
				t.Fatal("ban lock wait discarded", got)
			}
			// Inspect schedules before waiting: reverting to 2000ms must fail an assertion,
			// not merely time out in the fixture.
			f.r.evaluateChampSelect(f.c, f.r.currentWatch().ChampSelect)
			f.mu.Lock()
			events := append([]map[string]any(nil), f.events...)
			f.mu.Unlock()
			armed := 0
			for _, e := range events {
				if e["stage"] == "schedule" && e["reason"] == "armed" {
					armed++
					if e["delay_ms"] != 0 {
						t.Fatalf("first hover/lock delay must be zero: %+v", e)
					}
				}
			}
			if armed != 1 {
				t.Fatal("missing initial schedule", armed)
			}
			waitTakeoverIdle(t, f)
			if f.last().Body["completed"] != false {
				t.Fatal("wildcard must hover before lock")
			}
			if strategy == "show-only" {
				f.tick(t)
				if f.count() != 1 {
					t.Fatal("show-only locked")
				}
				return
			}
			if strategy == "show-then-lock" {
				f.mu.Lock()
				f.session.Timer.AdjustedTimeLeftInPhase = 50
				f.mu.Unlock()
			}
			f.r.evaluateChampSelect(f.c, f.r.currentWatch().ChampSelect)
			f.mu.Lock()
			events = append([]map[string]any(nil), f.events...)
			f.mu.Unlock()
			delays := []int{}
			for _, e := range events {
				if e["stage"] == "schedule" && e["reason"] == "armed" {
					delays = append(delays, e["delay_ms"].(int))
				}
			}
			if len(delays) != 2 {
				t.Fatal("two stages required", delays)
			}
			if strategy == "lock-now" && delays[1] != 0 {
				t.Fatal("lock-now waited", delays)
			}
			if strategy == "show-then-lock" && (delays[1] <= 0 || delays[1] > 50) {
				t.Fatal("lock wait must respect remaining turn", delays)
			}
			waitTakeoverIdle(t, f)
			if f.count() != 2 || f.last().Body["completed"] != true {
				t.Fatal("missing confirmed lock", f.patches)
			}
			sent := 0
			for _, record := range f.r.champSelectSnapshot().Records {
				if strings.HasPrefix(record.Message, "已发送") {
					sent++
					if record.Kind != "ok" {
						t.Fatal("happy path must be ok", record)
					}
				}
			}
			if sent != 2 {
				t.Fatal("missing sent records", sent)
			}
		})
	}
}

func TestR95LegacyAlliesCrossBlocksGuardRemainsIndependent(t *testing.T) {
	allies := []lcuLivePlayer{{PUUID: "a"}, {PUUID: "b"}, {PUUID: "c"}}
	players := []gameplayLivePlayer{}
	for i, a := range allies {
		players = append(players, gameplayLivePlayer{gameplayPlayer: gameplayPlayer{reference: gameplayReference{PlayerRef: a.PUUID}, IsCurrent: i == 0}, IsAlly: true})
	}
	verified, rejected := arenaAlliesCorroborate([]string{"1", "1", "2"}, players, allies, 3, liveClientArenaGrouping{}, "a")
	if verified || !rejected {
		t.Fatal("allies-cross-blocks must reject", verified, rejected)
	}
	verified, rejected = arenaAlliesCorroborate([]string{"1", "1", "1"}, players, allies, 3, liveClientArenaGrouping{}, "a")
	if !verified || rejected {
		t.Fatal("guard sanity", verified, rejected)
	}
}

func TestR95SquadGameAndAccountIsolation(t *testing.T) {
	a := &app{}
	r90RememberAllies(a, 0, 3)
	a.arenaAllyGameID = 90001
	response := gameplayLiveResponse{GameID: 90001, QueueID: 1750, GameMode: "CHERRY", Players: []gameplayLivePlayer{{gameplayPlayer: gameplayPlayer{reference: gameplayReference{PlayerRef: "r90-player-identity-01"}}}}}
	a.markRememberedArenaSquad(&response)
	if !response.Players[0].MySquad {
		t.Fatal("fixture")
	}
	for _, id := range []int64{0, 90002} {
		response.GameID = id
		a.markRememberedArenaSquad(&response)
		if response.Players[0].MySquad {
			t.Fatal("cross-game squad", id)
		}
	}
	response.GameID = 90001
	a.clearGameplayReferences()
	a.markRememberedArenaSquad(&response)
	if response.Players[0].MySquad {
		t.Fatal("account reset retained squad")
	}
}

func TestR95ProDirectoryBadgeConsistencyAndConflict(t *testing.T) {
	teams := []opggProTeam{
		{ID: 385, Name: "T1", ShortName: "T1", Members: []opggProMember{proFixtureMember(385, "Faker", "이상혁", proFixtureAccount("AccountOne", "private-one", "", 0, 0))}},
		{ID: 416, Name: "Gen.G", ShortName: "GEN", Members: []opggProMember{proFixtureMember(416, "Chovy", "정지훈", proFixtureAccount("AccountTwo", "private-two", "", 0, 0))}},
	}
	a := &app{}
	a.proPlayers.teams = teams
	index := a.proIdentitySnapshot()
	directory := a.buildProPlayers(teams, proRoster)
	found := 0
	for _, team := range directory.Teams {
		for _, player := range team.Players {
			for _, account := range player.Accounts {
				badge := a.matchProIdentity(index, "directory", gameplayReference{Region: "kr", GameName: account.GameName, TagLine: account.TagLine})
				if badge == nil || badge.TeamCode != team.Code || badge.PlayerName != player.Name || badge.Secondary != team.Secondary {
					t.Fatalf("directory/badge drift: %+v %+v", player, badge)
				}
				found++
			}
		}
	}
	if found != 2 {
		t.Fatal("both reviewed first-team identities must be included", found)
	}
	// An account-owner conflict poisons its PUUID and Riot ID keys, so the name
	// fallback cannot bypass an ambiguous stable identifier.
	teams[1].Members[0].Summoners = append(teams[1].Members[0].Summoners, proFixtureAccount("Alias", "private-one", "", 0, 0))
	a.proPlayers.teams = teams
	index = a.proIdentitySnapshot()
	for _, ref := range []gameplayReference{{Region: "kr", PlayerRef: "private-one", GameName: "AccountTwo", TagLine: "KR1"}, {Region: "kr", GameName: "AccountOne", TagLine: "KR1"}, {Region: "kr", GameName: "Alias", TagLine: "KR1"}} {
		if a.matchProIdentity(index, "conflict", ref) != nil {
			t.Fatal("conflict accepted", ref)
		}
	}
	a.proPlayers.fetchedAt = time.Now().Add(-25 * time.Hour)
	if len(a.proIdentitySnapshot().byKey) != 0 {
		t.Fatal("stale directory used")
	}
}
