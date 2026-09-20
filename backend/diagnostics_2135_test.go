package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func Test2135ObservedCurrentActionUndefinedEmptyIDPartialRoster(t *testing.T) {
	raw, err := os.ReadFile("testdata/diagnostics-2135/current-action.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		ChampionIDs map[string]int  `json:"championIds"`
		TargetRef   string          `json:"targetRef"`
		Response    json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	a := &app{champions: newChampionProvider()}
	a.champions.championMeta = map[int]championMetadata{}
	for key, id := range fixture.ChampionIDs {
		a.champions.championMeta[id] = championMetadata{ID: id, Key: key, NameZH: key}
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, fixture.Response); err != nil {
		t.Fatal(err)
	}
	data := append([]byte("0:{\"a\":\"$@1\"}\n1:"), compact.Bytes()...)
	ref := gameplayReference{Region: "kr", PlayerRef: fixture.TargetRef}
	now := time.Date(2026, 9, 9, 13, 49, 0, 0, time.UTC)
	game, err := a.parseOPGGCurrentGame(data, ref, now)
	if err != nil {
		t.Fatal(err)
	}
	if game.Status != "active" || len(game.Teams) != 2 || len(game.Teams[0].Players) != 3 || len(game.Teams[1].Players) != 5 || game.Teams[0].MissingPlayers != 2 || game.Teams[0].AverageLP != nil || game.Teams[0].AverageRank != nil {
		t.Fatalf("partial live response lost: %+v", game)
	}
	if game.GameID != "" {
		t.Fatal("invented spectator ID")
	}
	got, _ := json.Marshal(game)
	if bytes.Contains(got, []byte(fixture.TargetRef)) || bytes.Contains(got, []byte("puuid")) {
		t.Fatal("source identity leaked")
	}
	finished := bytes.Replace(data, []byte(`"is_finished":"$undefined"`), []byte(`"is_finished":true`), 1)
	game, err = a.parseOPGGCurrentGame(finished, ref, now)
	if err != nil || game.Status != "none" {
		t.Fatal("finished response must be none", err)
	}
	ref.PlayerRef = "unrelated-fixture-player-00000001"
	if _, err = a.parseOPGGCurrentGame(data, ref, now); supplementFailureCode(err) != "opgg-current-target-mismatch" {
		t.Fatal("partial roster lost target binding", err)
	}
}

func Test2135CurrentActionReferencesAndOptionalFields(t *testing.T) {
	a, data, ref, now := currentGameFixture(t)
	data = bytes.Replace(data, []byte(`"is_finished":false`), []byte(`"is_finished":"$undefined","windows_script":"$missing-private-code"`), 1)
	data = bytes.Replace(data, []byte(`"game_map":"SUMMONERS_RIFT"`), []byte(`"game_map":"$f:map"`), 1)
	data = append(data, []byte("\nf:{\"map\":\"SUMMONERS_RIFT\"}")...)
	game, err := a.parseOPGGCurrentGame(data, ref, now)
	if err != nil || game.Status != "active" {
		t.Fatal("valid referenced response failed", err)
	}
	normalized, err := normalizedCurrentActionResult(data)
	if err != nil || bytes.Contains(normalized, []byte("windows_script")) {
		t.Fatal("spectate code resolved", err)
	}
	bad := bytes.Replace(data, []byte(`"map":"SUMMONERS_RIFT"`), []byte(`"map":"$f:map"`), 1)
	if _, err = normalizedCurrentActionResult(bad); supplementFailureCode(err) != "opgg-flight-limit" {
		t.Fatal("cycle not bounded", err)
	}
	if supplementFailureCode(context.Canceled) != "canceled" {
		t.Fatal("cancellation lost")
	}
}

func Test2135LivePublicCurrentGame(t *testing.T) {
	if os.Getenv("OPGG_2135_PROBE") != "1" {
		t.Skip("opt-in public read-only request")
	}
	a := &app{champions: newChampionProvider()}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	if err := a.champions.ensureChampionMetadata(ctx); err != nil {
		t.Fatal("catalog unavailable")
	}
	result, err := a.fetchOPGGCurrentGame(ctx, gameplayReference{Region: "kr", GameName: "Maldives", TagLine: "0727"})
	if err != nil {
		t.Fatal("live request:", supplementFailureCode(err))
	}
	counts := []int{}
	for _, team := range result.Teams {
		counts = append(counts, len(team.Players))
	}
	t.Logf("Live public request: status=%s team sizes=%v", result.Status, counts)
}
