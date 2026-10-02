package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func r191Riot(t *testing.T, vars bool, missing int) riotMatchInfo {
	t.Helper()
	selections := []map[string]any{}
	for i, id := range []int64{8008, 9111, 9103, 8017, 8304, 8347} {
		row := map[string]any{"perk": id}
		if vars && i != missing {
			row["var1"], row["var2"], row["var3"] = 0, 0, 0
		}
		selections = append(selections, row)
	}
	data, _ := json.Marshal(map[string]any{"gameId": 191, "queueId": 420, "gameDuration": 1800, "participants": []any{map[string]any{"participantId": 4, "puuid": "r191-subject-puuid", "perks": map[string]any{"styles": []any{map[string]any{"style": 8000, "selections": selections}}}}}})
	var info riotMatchInfo
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatal(err)
	}
	return info
}
func r191LCU(t *testing.T, vars bool, missing int) lcuGame {
	t.Helper()
	stats := map[string]any{}
	for i, id := range []int64{8008, 9111, 9103, 8017, 8304, 8347} {
		stats[fmt.Sprintf("perk%d", i)] = id
		if vars && i != missing {
			for j := 1; j <= 3; j++ {
				stats[fmt.Sprintf("perk%dVar%d", i, j)] = 0
			}
		}
	}
	data, _ := json.Marshal(map[string]any{"gameId": 191, "queueId": 420, "gameDuration": 1800, "participants": []any{map[string]any{"participantId": 4, "stats": stats}}, "participantIdentities": []any{map[string]any{"participantId": 4, "player": map[string]any{"puuid": "r191-subject-puuid"}}}})
	var game lcuGame
	if err := json.Unmarshal(data, &game); err != nil {
		t.Fatal(err)
	}
	return game
}
func r191Events(t *testing.T, a *app, name string) []map[string]any {
	t.Helper()
	data, err := a.storage.readDiagnosticLogForExport()
	if err != nil {
		t.Fatal(err)
	}
	var events []map[string]any
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		var row map[string]any
		if json.Unmarshal([]byte(line), &row) != nil {
			t.Fatal(line)
		}
		if row["event"] == name {
			events = append(events, row)
		}
	}
	return events
}
func TestR191_01RiotSGPPresence(t *testing.T) {
	for _, source := range []string{"riot", "sgp"} {
		for _, present := range []bool{false, true} {
			info := r191Riot(t, present, -1)
			match := convertRiotMatchInfo(&info, "r191-subject-puuid", nil, nil, source, "")
			stats := match.Participants[0].PerkStats
			if present {
				if len(stats) != 6 || stats[0].Vars != [3]int64{} {
					t.Fatalf("%s true zeros lost: %+v", source, stats)
				}
			} else {
				data, _ := json.Marshal(match.Participants[0])
				if len(stats) != 0 || strings.Contains(string(data), "perkStats") {
					t.Fatalf("%s missing synthesized: %s", source, data)
				}
			}
		}
	}
}
func TestR191_02LCUPresence(t *testing.T) {
	for _, present := range []bool{false, true} {
		game := r191LCU(t, present, -1)
		stats := normalizeGameplayMatch(game, gameplayReference{PlayerRef: "r191-subject-puuid"}, nil, nil).Participants[0].PerkStats
		if present && (len(stats) != 6 || stats[0].Vars != [3]int64{}) || !present && len(stats) != 0 {
			t.Fatalf("present=%v stats=%+v", present, stats)
		}
	}
}
func TestR191_03OneMissingDropsGroup(t *testing.T) {
	for i := 0; i < 6; i++ {
		info := r191Riot(t, true, i)
		if len(convertRiotMatchInfo(&info, "r191-subject-puuid", nil, nil, "", "").Participants[0].PerkStats) != 0 {
			t.Fatalf("Riot missing %d", i)
		}
		if len(lcuPerkStats(r191LCU(t, true, i).Participants[0])) != 0 {
			t.Fatalf("LCU missing %d", i)
		}
	}
	// One provided var is enough to establish presence; remaining absent vars use zero.
	info := r191Riot(t, true, -1)
	info.Participants[0].Perks.Styles[0].Selections[0].Var2 = nil
	info.Participants[0].Perks.Styles[0].Selections[0].Var3 = nil
	if len(riotParticipantPerkStats(info.Participants[0], false)) != 6 {
		t.Fatal("partially supplied vars mistaken for fully absent")
	}
}
func TestR191_04V2CacheRoundtrip(t *testing.T) {
	p := newHexdataBudgetProvider(t, t.TempDir(), championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Fatal("v2 should avoid network")
		return nil, errors.New("unexpected")
	}))
	for i, present := range []bool{false, true} {
		info := r191Riot(t, present, -1)
		raw := &riotMatch{Info: info}
		id := fmt.Sprintf("KR_%d", 191+i)
		raw.Metadata.MatchID = id
		newRiotProvider(p).persistRiotMatch("riot-match-v2|"+id, raw)
		got, err := newRiotProvider(p).matchByID(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		stats := riotConvertMatch(got, "r191-subject-puuid", nil).Participants[0].PerkStats
		if present && len(stats) != 6 || !present && len(stats) != 0 {
			t.Fatalf("cache presence=%v %+v", present, stats)
		}
		data, _ := json.Marshal(got)
		if !present && !strings.Contains(string(data), `"var1":null`) {
			t.Fatal("cache did not retain null")
		}
	}
}
func TestR191_05PresenceCountsAndCaps(t *testing.T) {
	a := &app{storage: newFlowDiagnosticStore(t)}
	yes, no := r191Riot(t, true, -1), r191Riot(t, false, -1)
	yes.Participants = append(yes.Participants, no.Participants...)
	for i := 0; i < 4; i++ {
		for _, source := range []string{"sgp", "riot"} {
			a.recordRiotPerkDiagnostics(source, []*riotMatchInfo{&yes}, "", 0)
		}
		games := []lcuGame{r191LCU(t, true, -1), r191LCU(t, false, -1)}
		a.recordLCUPerkDiagnostics(games, gameplayReference{})
	}
	rows := r191Events(t, a, "perk_stats_presence")
	if len(rows) != 9 {
		t.Fatalf("caps=%+v", rows)
	}
	counts := map[string]int{}
	for _, row := range rows {
		counts[row["source"].(string)]++
		if row["participants"] != float64(2) || row["with_vars"] != float64(1) || row["without_vars"] != float64(1) {
			t.Fatalf("counts=%+v", row)
		}
		for _, key := range []string{"game_id", "gameId", "name", "puuid"} {
			if _, ok := row[key]; ok {
				t.Fatal("identity in presence")
			}
		}
	}
	for source, n := range counts {
		if n != 3 {
			t.Fatalf("%s %d", source, n)
		}
	}
	a.resetDiagnosticDeduplication()
	a.recordRiotPerkDiagnostics("riot", []*riotMatchInfo{&yes}, "", 0)
	if len(r191Events(t, a, "perk_stats_presence")) != 9 {
		t.Fatal("rotation reset session caps")
	}
}
func TestR191_06SubjectSamples(t *testing.T) {
	for _, source := range []string{"riot", "sgp", "lcu"} {
		t.Run(source, func(t *testing.T) {
			a := &app{storage: newFlowDiagnosticStore(t)}
			values := [3]int64{3, 1028, 7}
			if source == "lcu" {
				game := r191LCU(t, true, -1)
				game.Participants[0].Stats.Perk0Var1 = &values[0]
				game.Participants[0].Stats.Perk0Var2 = &values[1]
				game.Participants[0].Stats.Perk0Var3 = &values[2]
				game.Participants = append(game.Participants, game.Participants[0])
				game.Participants[1].ParticipantID = 5
				a.recordLCUPerkDiagnostics([]lcuGame{game}, gameplayReference{PlayerRef: "r191-subject-puuid"})
			} else {
				info := r191Riot(t, true, -1)
				selection := &info.Participants[0].Perks.Styles[0].Selections[0]
				selection.Var1, selection.Var2, selection.Var3 = &values[0], &values[1], &values[2]
				other := info.Participants[0]
				other.PUUID = "other"
				other.ParticipantID = 5
				info.Participants = append(info.Participants, other)
				a.recordRiotPerkDiagnostics(source, []*riotMatchInfo{&info}, "r191-subject-puuid", 0)
			}
			rows := r191Events(t, a, "perk_effect_sample")
			if len(rows) != 2 {
				t.Fatalf("non-subject sample: %+v", rows)
			}
			for _, row := range rows {
				expectedVars := "[0 0 0]"
				if row["perk_id"] == float64(8008) {
					expectedVars = "[3 1028 7]"
				}
				if row["source"] != source || row["queue_id"] != float64(420) || row["game_duration"] != float64(1800) || fmt.Sprint(row["vars"]) != expectedVars {
					t.Fatalf("sample=%+v", row)
				}
				for _, key := range []string{"name", "game_id", "gameId", "puuid"} {
					if _, ok := row[key]; ok {
						t.Fatal("identity in sample")
					}
				}
			}
		})
	}
	// A non-subject carrying either target rune produces no samples.
	a := &app{storage: newFlowDiagnosticStore(t)}
	info := r191Riot(t, true, -1)
	a.recordRiotPerkDiagnostics("sgp", []*riotMatchInfo{&info}, "someone-else", 0)
	if len(r191Events(t, a, "perk_effect_sample")) != 0 {
		t.Fatal("sampled non-subject")
	}
}
func TestR191_07SampleCap(t *testing.T) {
	a := &app{storage: newFlowDiagnosticStore(t)}
	info := r191Riot(t, true, -1)
	for i := 0; i < 4; i++ {
		info.GameID = int64(191 + i)
		a.recordRiotPerkDiagnostics("sgp", []*riotMatchInfo{&info}, "r191-subject-puuid", 0)
	}
	rows := r191Events(t, a, "perk_effect_sample")
	if len(rows) != 6 {
		t.Fatalf("fourth sample accepted: %d", len(rows))
	}
}
func TestR191_08DiagnosticExport(t *testing.T) {
	a := &app{storage: newFlowDiagnosticStore(t)}
	info := r191Riot(t, true, -1)
	a.recordRiotPerkDiagnostics("riot", []*riotMatchInfo{&info}, "r191-subject-puuid", 0)
	r := httptest.NewRecorder()
	a.handleDiagnosticLog(r, httptest.NewRequest("GET", "/api/diagnostics/log", nil))
	if r.Code != 200 || !strings.Contains(r.Body.String(), `"event":"perk_effect_sample"`) {
		t.Fatalf("export %d %s", r.Code, r.Body)
	}
}

func r191AugmentProvider(t *testing.T, mode string) *championProvider {
	t.Helper()
	p := newHexdataBudgetProvider(t, t.TempDir(), championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/augments":
			if mode == "error" {
				return nil, errors.New("directory fixture failure")
			}
			if mode == "invalid" {
				return hexdataResponse(r, []byte("changed directory")), nil
			}
			catalog := string(hexdataAugmentsTestPage())
			if mode != "not-found" {
				catalog = strings.Replace(catalog, "/augment/1000-augment1000", "/augment/1225-dual-wield", 1)
			}
			return hexdataResponse(r, []byte(catalog)), nil
		case "/augment/1225-dual-wield":
			return hexdataResponse(r, r190DetailPage("攻击时施加攻击特效。")), nil
		case "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/cherry-augments.json":
			return hexdataResponse(r, []byte(`[{"id":1225,"name":"元数据","description":"禁止补入的目录效果。"}]`)), nil
		case "/latest/cdragon/arena/zh_cn.json":
			return hexdataResponse(r, []byte(`{"augments":[]}`)), nil
		default:
			return hexdataResponse(r, hexdataTestPage("<p>fixture</p>")), nil
		}
	}))
	p.hexdata.adoptCitation(championSourceCitation{Source: "Hexdata", Patch: "16.16", ReportDate: "2026-08-19", BuildID: hexdataTestBuild, CanonicalURL: "https://hexdata.com.cn/augments"})
	return p
}
func TestR191_11DirectoryFallbackHTTP(t *testing.T) {
	for _, mode := range []string{"error", "invalid", "not-found"} {
		t.Run(mode, func(t *testing.T) {
			p := r191AugmentProvider(t, mode)
			a := &app{champions: p}
			r := httptest.NewRecorder()
			a.handleChampionAugmentDetail(r, httptest.NewRequest("GET", "/api/champions/augment-detail?id=1225&slug=ARAM_MagicMissile", nil))
			var got championAugmentDetailResponse
			_ = json.Unmarshal(r.Body.Bytes(), &got)
			if r.Code != 200 || got.Source != "CommunityDragon fallback" {
				t.Fatalf("%d %s", r.Code, r.Body)
			}
		})
	}
}
func TestR191_12FallbackBuildUnavailable(t *testing.T) {
	a := &app{champions: r191AugmentProvider(t, "error")}
	r := httptest.NewRecorder()
	a.handleGameplayAugmentDescriptions(r, httptest.NewRequest("GET", "/api/gameplay/augment-descriptions?ids=1225", nil))
	if r.Code != 200 || !strings.Contains(r.Body.String(), `"status":"unavailable"`) || strings.Contains(r.Body.String(), "禁止补入") {
		t.Fatalf("%d %s", r.Code, r.Body)
	}
}
func TestR191_13NormalGuide(t *testing.T) {
	p := r191AugmentProvider(t, "normal")
	detail, err := p.loadHexdataAugmentDetail(context.Background(), 1225, "")
	if err != nil || detail.Source != "Hexdata" || detail.effectDescription != "攻击时施加攻击特效。" {
		t.Fatalf("%+v %v", detail, err)
	}
	items := loadGameplayAugmentDescriptions(context.Background(), []int64{1225}, p.gameplayAugmentDescription)
	if items[0].Status != "ok" || items[0].Description != detail.effectDescription {
		t.Fatal(items)
	}
}
