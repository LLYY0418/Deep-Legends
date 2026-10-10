package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
)

func r263Ref(index int) string { return fmt.Sprintf("r263-player-reference-%04d", index) }

// R95 真机分布（局 8980574903 InProgress）：{1:2, 2:2, 3:1, 4:2, 5:6, 6:1, 7:1, 8:1, 9:1, 10:1}。
// 我的小队 = 编号 1 的两人 + 编号 3 的一人。
func r263RealInProgressInputs() []arenaPremadeInput {
	signals := []int64{1, 1, 3, 2, 2, 4, 4, 5, 5, 5, 5, 5, 5, 6, 7, 8, 9, 10}
	inputs := make([]arenaPremadeInput, len(signals))
	for index, signal := range signals {
		inputs[index] = arenaPremadeInput{Ref: r263Ref(index), TeamParticipantID: signal, Mine: index < 3}
	}
	return inputs
}

func TestR263ArenaPremadeGroupsUseRealDistribution(t *testing.T) {
	groups, dropped := arenaPremadeGroups(r263RealInProgressInputs(), 3, 5)
	if len(groups) != 2 {
		t.Fatalf("groups = %#v, want the two 2-player lobbies", groups)
	}
	for _, group := range groups {
		if len(group.Members) != 2 || group.Source != "session" {
			t.Fatalf("group = %#v", group)
		}
	}
	if !reflect.DeepEqual(groups[0].Members, []int{3, 4}) || !reflect.DeepEqual(groups[1].Members, []int{5, 6}) {
		t.Fatalf("group order = %#v", groups)
	}
	if len(dropped) != 1 || dropped[0].Size != 6 || dropped[0].Reason != "exceeds_squad" {
		t.Fatalf("dropped = %#v, the six-player id must be rejected", dropped)
	}
}

func TestR263ArenaPremadeGroupsRejectFourPlayerAndCrossSquadSignals(t *testing.T) {
	inputs := []arenaPremadeInput{
		{Ref: r263Ref(0), TeamParticipantID: 8, Mine: true},
		{Ref: r263Ref(1), TeamParticipantID: 8},
		{Ref: r263Ref(2), TeamParticipantID: 3},
		{Ref: r263Ref(3), TeamParticipantID: 3},
		{Ref: r263Ref(4), TeamParticipantID: 3},
		{Ref: r263Ref(5), TeamParticipantID: 3},
	}
	groups, dropped := arenaPremadeGroups(inputs, 3, 5)
	if len(groups) != 0 {
		t.Fatalf("groups = %#v", groups)
	}
	reasons := []string{}
	for _, drop := range dropped {
		reasons = append(reasons, fmt.Sprintf("%s:%d", drop.Reason, drop.Size))
	}
	if strings.Join(reasons, ",") != "crosses_my_squad:2,exceeds_squad:4" {
		t.Fatalf("dropped = %v", reasons)
	}
}

func r263SharedArenaGames(left, right string, count int, firstGameID int64) []gameplayMatch {
	matches := make([]gameplayMatch, 0, count)
	for index := 0; index < count; index++ {
		matches = append(matches, gameplayMatch{GameID: firstGameID + int64(index), QueueID: 1750, Result: "win", SubjectParticipantID: 1, Participants: []gameplayParticipant{
			{ParticipantID: 1, PlayerRef: left, SubteamID: 2, Placement: 1},
			{ParticipantID: 2, PlayerRef: right, SubteamID: 2, Placement: 1},
		}})
	}
	return matches
}

func TestR263ArenaPremadeInferenceCompletesTrioAndRejectsOversize(t *testing.T) {
	inputs := []arenaPremadeInput{
		{Ref: r263Ref(0), Mine: true},
		{Ref: r263Ref(1), TeamParticipantID: 4},
		{Ref: r263Ref(2), TeamParticipantID: 4},
		{Ref: r263Ref(3)},
	}
	inputs[2].Matches = r263SharedArenaGames(r263Ref(2), r263Ref(3), 5, 100)
	groups, dropped := arenaPremadeGroups(inputs, 3, 5)
	if len(groups) != 1 || !reflect.DeepEqual(groups[0].Members, []int{1, 2, 3}) || groups[0].Source != "both" || len(dropped) != 0 {
		t.Fatalf("groups = %#v dropped = %#v", groups, dropped)
	}
	// Four shared games stay below the threshold.
	inputs[2].Matches = r263SharedArenaGames(r263Ref(2), r263Ref(3), 4, 100)
	groups, _ = arenaPremadeGroups(inputs, 3, 5)
	if len(groups) != 1 || len(groups[0].Members) != 2 || groups[0].Source != "session" {
		t.Fatalf("below threshold groups = %#v", groups)
	}
	// Same games on different subteams are opponents, not a lobby.
	opponents := r263SharedArenaGames(r263Ref(2), r263Ref(3), 6, 200)
	for index := range opponents {
		opponents[index].Participants[1].SubteamID = 5
	}
	inputs[2].Matches = opponents
	groups, _ = arenaPremadeGroups(inputs, 3, 5)
	if len(groups) != 1 || len(groups[0].Members) != 2 {
		t.Fatalf("opponents must not merge: %#v", groups)
	}
	// Inference that grows a lobby past the squad size returns it to unknown.
	inputs = append(inputs, arenaPremadeInput{Ref: r263Ref(4)})
	inputs[2].Matches = append(r263SharedArenaGames(r263Ref(2), r263Ref(3), 5, 300), r263SharedArenaGames(r263Ref(2), r263Ref(4), 5, 400)...)
	groups, dropped = arenaPremadeGroups(inputs, 3, 5)
	if len(groups) != 0 || len(dropped) != 1 || dropped[0].Reason != "inferred_exceeds_squad" || dropped[0].Size != 4 {
		t.Fatalf("oversize inference groups = %#v dropped = %#v", groups, dropped)
	}
}

func r263ArenaResponse(order []int, signals map[int]int64, mine map[int]bool) (gameplayLiveResponse, map[string]arenaPremadeInput) {
	response := gameplayLiveResponse{Phase: "InProgress", GameID: 263, QueueID: 1750, GameMode: "CHERRY"}
	inputs := map[string]arenaPremadeInput{}
	for _, index := range order {
		ref := r263Ref(index)
		player := gameplayLivePlayer{MySquad: mine[index]}
		player.reference.PlayerRef = ref
		player.IsCurrent = index == 0
		response.Players = append(response.Players, player)
		inputs[ref] = arenaPremadeInput{TeamParticipantID: signals[index]}
	}
	return response, inputs
}

func TestR263ApplyArenaPremadeSquadsKeepsLettersStable(t *testing.T) {
	a := &app{}
	signals := map[int]int64{0: 1, 1: 1, 2: 3, 3: 2, 4: 2, 5: 4, 6: 4, 7: 9}
	mine := map[int]bool{0: true, 1: true, 2: true}
	response, inputs := r263ArenaResponse([]int{0, 1, 2, 3, 4, 5, 6, 7}, signals, mine)
	a.applyArenaPremadeSquads(&response, inputs)
	if response.ArenaSquadSize != 3 {
		t.Fatalf("squad size = %d", response.ArenaSquadSize)
	}
	first := map[string]string{}
	for _, player := range response.Players {
		first[player.reference.PlayerRef] = player.ArenaSquadKey
	}
	if first[r263Ref(0)] != "mine" || first[r263Ref(3)] != "premade:A" || first[r263Ref(5)] != "premade:B" || first[r263Ref(7)] != "" {
		t.Fatalf("keys = %#v", first)
	}
	// A later refresh lists the roster in another order: letters must not swap.
	response, inputs = r263ArenaResponse([]int{6, 5, 7, 4, 3, 2, 1, 0}, signals, mine)
	a.applyArenaPremadeSquads(&response, inputs)
	for _, player := range response.Players {
		if got, want := player.ArenaSquadKey, first[player.reference.PlayerRef]; got != want {
			t.Fatalf("%s key = %q, want %q", player.reference.PlayerRef, got, want)
		}
	}
}

func TestR263ChampSelectSignalPreferredOverInProgress(t *testing.T) {
	a := &app{}
	a.rememberArenaChampSelectSignals(263, []liveRosterEntry{
		{player: lcuLivePlayer{PUUID: r263Ref(3), TeamParticipantID: 7}},
		{player: lcuLivePlayer{PUUID: r263Ref(4), TeamParticipantID: 7}},
	})
	// InProgress lumps 3, 4, 5, 6 under one id (the R95 shape); champ select had a duo.
	signals := map[int]int64{0: 1, 3: 5, 4: 5, 5: 5, 6: 5}
	response, inputs := r263ArenaResponse([]int{0, 3, 4, 5, 6}, signals, map[int]bool{0: true})
	a.applyArenaPremadeSquads(&response, inputs)
	keys := map[string]string{}
	for _, player := range response.Players {
		keys[player.reference.PlayerRef] = player.ArenaSquadKey
	}
	if keys[r263Ref(3)] != "premade:A" || keys[r263Ref(4)] != "premade:A" || keys[r263Ref(5)] != "" || keys[r263Ref(6)] != "" {
		t.Fatalf("keys = %#v", keys)
	}
}

func TestR263ArenaFameLevelsAndTierCrossCheck(t *testing.T) {
	cases := []struct {
		fame  int
		level int
	}{{99, 0}, {100, 1}, {1599, 1}, {1600, 2}, {47599, 11}, {47600, 12}, {87510, 12}}
	for _, item := range cases {
		if got := arenaLevelForFame(item.fame); got != item.level {
			t.Fatalf("level(%d) = %d, want %d", item.fame, got, item.level)
		}
	}
	if fame, reason := arenaFameFromRated("GLADIATOR", 87510); fame == nil || fame.Level != 12 || fame.Fame != 87510 || reason != "" {
		t.Fatalf("gladiator = %#v %q", fame, reason)
	}
	if fame, reason := arenaFameFromRated("silver", 12600.4); fame == nil || fame.Level != 6 || fame.Tier != "SILVER" || reason != "" {
		t.Fatalf("silver = %#v %q", fame, reason)
	}
	// A legacy hidden rating (e.g. 1500 with GOLD) is not Fame: do not guess.
	for _, item := range []struct {
		tier   string
		rating float64
		reason string
	}{{"GOLD", 1500, "tier_mismatch"}, {"", 9000, "no_tier"}, {"NONE", 9000, "no_tier"}, {"WOOD", 0, "no_rating"}, {"WOOD", 50, "below_level_one"}} {
		if fame, reason := arenaFameFromRated(item.tier, item.rating); fame != nil || reason != item.reason {
			t.Fatalf("%s/%v = %#v %q, want %q", item.tier, item.rating, fame, reason, item.reason)
		}
	}
}

func TestR263SGPRankedQueueKeepsCherryRatedFields(t *testing.T) {
	var queue sgpRankedQueue
	if err := json.Unmarshal([]byte(`{"queueType":"CHERRY","ratedTier":"GLADIATOR","ratedRating":87510}`), &queue); err != nil {
		t.Fatal(err)
	}
	if queue.RatedTier != "GLADIATOR" || queue.RatedRating != 87510 {
		t.Fatalf("queue = %#v", queue)
	}
	// Fractional ratings must not break decoding of the entry.
	if err := json.Unmarshal([]byte(`{"queueType":"RANKED_SOLO_5x5","tier":"GOLD","ratedRating":0.5}`), &queue); err != nil || queue.Tier != "GOLD" {
		t.Fatalf("solo decode = %#v %v", queue, err)
	}
}

func r263ArenaMatch(gameID int64, placement int, ref string) gameplayMatch {
	match := gameplayMatch{GameID: gameID, QueueID: 1750, GameMode: "CHERRY", ModeGroup: "arena", CreatedAt: 1_800_000_000_000 - gameID, SubjectParticipantID: 1}
	for team := int64(1); team <= 6; team++ {
		participant := gameplayParticipant{ParticipantID: team, SubteamID: team, Placement: int(team)}
		if team == 1 {
			participant.PlayerRef, participant.Placement = ref, placement
		} else if int(team) == placement {
			participant.Placement = 1
		}
		match.Participants = append(match.Participants, participant)
	}
	applyArenaResults(&match)
	return match
}

func TestR263LiveArenaRecordTopHalfAndFirst(t *testing.T) {
	ref := r263Ref(1)
	matches := []gameplayMatch{r263ArenaMatch(1, 1, ref), r263ArenaMatch(2, 3, ref), r263ArenaMatch(3, 4, ref), r263ArenaMatch(4, 6, ref)}
	stats := liveArenaRecord(matches, ref, 1750)
	if stats == nil || *stats != (gameplayArenaRecord{Games: 4, TopHalf: 2, Top1: 1}) {
		t.Fatalf("stats = %#v", stats)
	}
	many := []gameplayMatch{}
	for index := int64(1); index <= 35; index++ {
		many = append(many, r263ArenaMatch(index, 1, ref))
	}
	if stats := liveArenaRecord(many, ref, 1750); stats == nil || stats.Games != arenaLiveStatsGames {
		t.Fatalf("thirty-game window = %#v", stats)
	}
	if stats := liveArenaRecord(nil, ref, 1750); stats != nil {
		t.Fatalf("empty = %#v", stats)
	}
}

func TestR263RecentGamesCarryGameIDAndPlacement(t *testing.T) {
	ref := r263Ref(2)
	games := recentGamesFromSelectedMatches([]gameplayMatch{r263ArenaMatch(91, 3, ref)}, ref)
	if len(games) != 1 || games[0].GameID != 91 || games[0].Placement != 3 || games[0].QueueID != 1750 || !games[0].Win {
		t.Fatalf("games = %#v", games)
	}
	raw, _ := json.Marshal(games[0])
	if !strings.Contains(string(raw), `"gameId":91`) || !strings.Contains(string(raw), `"placement":3`) {
		t.Fatalf("json = %s", raw)
	}
}

func TestR263LivePlayerJSONExposesNewFieldsFlat(t *testing.T) {
	player := gameplayLivePlayer{}
	player.ArenaSquadKey = "premade:A"
	player.SoloRank = &gameplayRank{QueueType: "RANKED_SOLO_5x5", Tier: "EMERALD", Division: "IV", LeaguePoints: 48}
	player.ArenaFame = &gameplayArenaFame{Level: 12, Fame: 87510, Tier: "GLADIATOR"}
	raw, err := json.Marshal(player)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"arenaSquadKey":"premade:A"`, `"soloRank":{`, `"arenaFame":{"level":12,"fame":87510,"tier":"GLADIATOR"}`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("missing %s in %s", want, raw)
		}
	}
}

func TestR263LiveArenaExtensionSkipsUnfilteredOrShortHistory(t *testing.T) {
	a := &app{}
	ref := r263Ref(3)
	base := livePlayerMatchesResult{Matches: []gameplayMatch{r263ArenaMatch(1, 2, ref)}, Evidence: &liveHistoryEvidence{QueueFiltered: true, StopReason: "enough"}}
	if got := a.extendLiveArenaHistory(t.Context(), &LCUClient{}, gameplayReference{}, ref, nil, 1, 1750, base); len(got) != 1 {
		t.Fatalf("no SGP provider must return base: %d", len(got))
	}
	a.sgp = newSGPProvider()
	base.Evidence.QueueFiltered = false
	if got := a.extendLiveArenaHistory(t.Context(), &LCUClient{}, gameplayReference{}, ref, nil, 1, 1750, base); len(got) != 1 {
		t.Fatalf("unfiltered page must not extend: %d", len(got))
	}
	base.Evidence.QueueFiltered = true
	if got := a.extendLiveArenaHistory(t.Context(), &LCUClient{}, gameplayReference{}, ref, nil, 1, 1750, base); len(got) != 1 {
		t.Fatalf("fewer than ten games means the queue is exhausted: %d", len(got))
	}
	if got := a.extendLiveArenaHistory(t.Context(), &LCUClient{}, gameplayReference{}, ref, nil, 1, 420, base); len(got) != 1 {
		t.Fatalf("non-arena queue must not extend: %d", len(got))
	}
}

func TestR263LiveMatchEndpointServesIndexedMatchWithSubject(t *testing.T) {
	a := &app{token: "test-token", gameplayRefs: map[string]string{}, gameplayRefDetails: map[string]gameplayReference{}}
	subject := r263Ref(5)
	other := r263Ref(6)
	match := r263ArenaMatch(777, 2, other) // indexed from another player's history
	match.Participants[1].PlayerRef = subject
	a.rememberLiveMatches(263, other, []gameplayMatch{match})
	publicRef := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: subject})

	recorder := httptest.NewRecorder()
	a.handleGameplayLiveMatch(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/live/match?gameId=777&player="+publicRef, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var got gameplayMatch
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.GameID != 777 || got.SubjectParticipantID != 2 || len(got.Participants) != 6 {
		t.Fatalf("match = %#v", got)
	}
	// Subteam 2 finished first in the fixture: the subject's own result wins.
	if got.Result != "win" {
		t.Fatalf("result = %q", got.Result)
	}
	for _, participant := range got.Participants {
		if strings.HasPrefix(participant.PlayerRef, "r263-") {
			t.Fatalf("raw reference leaked: %q", participant.PlayerRef)
		}
	}

	for _, target := range []string{"/api/gameplay/live/match?gameId=0&player=" + publicRef, "/api/gameplay/live/match?gameId=777"} {
		recorder = httptest.NewRecorder()
		a.handleGameplayLiveMatch(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d", target, recorder.Code)
		}
	}
	recorder = httptest.NewRecorder()
	a.handleGameplayLiveMatch(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/live/match?gameId=778&player="+publicRef, nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("missing match status = %d", recorder.Code)
	}
}

func TestR263LiveMatchIndexKeepsTwoGames(t *testing.T) {
	a := &app{}
	ref := r263Ref(8)
	for gameID := int64(1); gameID <= 3; gameID++ {
		a.rememberLiveMatches(gameID, ref, []gameplayMatch{r263ArenaMatch(gameID*10, 1, ref)})
	}
	if _, ok := a.lookupLiveMatch(ref, 10); ok {
		t.Fatal("oldest live game must be evicted")
	}
	for _, id := range []int64{20, 30} {
		if _, ok := a.lookupLiveMatch(ref, id); !ok {
			t.Fatalf("game %d missing", id)
		}
	}
}

func TestR263RouteRegistered(t *testing.T) {
	source := readR263Source(t, "main.go")
	if !strings.Contains(source, `mux.HandleFunc("GET /api/gameplay/live/match", a.authorized(a.handleGameplayLiveMatch))`) {
		t.Fatal("live match route must be registered behind authorization")
	}
}

func readR263Source(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
