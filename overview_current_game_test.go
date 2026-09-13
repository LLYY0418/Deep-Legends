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
	"sync/atomic"
	"testing"
	"time"
)

func currentGameFixture(t *testing.T) (*app, []byte, gameplayReference, time.Time) {
	t.Helper()
	raw, err := os.ReadFile("testdata/opgg-current-game.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		ChampionIDs map[string]int  `json:"championIds"`
		Response    json.RawMessage `json:"response"`
	}
	if json.Unmarshal(raw, &fixture) != nil {
		t.Fatal("fixture malformed")
	}
	c := newChampionProvider()
	c.championMeta = map[int]championMetadata{}
	for key, id := range fixture.ChampionIDs {
		c.championMeta[id] = championMetadata{ID: id, Key: key, NameZH: key}
	}
	a := &app{champions: c, token: "fixture-token"}
	var compact bytes.Buffer
	if err := json.Compact(&compact, fixture.Response); err != nil {
		t.Fatal(err)
	}
	return a, append([]byte("0:{\"a\":\"$@7\"}\n7:"), compact.Bytes()...), gameplayReference{PlayerRef: "current-game-fixture-0-0-0000000001", Region: "kr", GameName: "Fixture", TagLine: "KR1"}, time.Date(2026, 9, 8, 16, 0, 0, 0, time.UTC)
}
func TestOverviewCurrentGameObservedContract(t *testing.T) {
	a, data, ref, now := currentGameFixture(t)
	game, err := a.parseOPGGCurrentGame(data, ref, now)
	if err != nil {
		t.Fatal(err)
	}
	if game.Status != "active" || len(game.Teams) != 2 || len(game.Teams[0].Players) != 5 || game.Map != "召唤师峡谷" {
		t.Fatal("incorrect roster")
	}
	p := game.Teams[0].Players[0]
	if p.ChampionID != 777 || p.Rank.LeaguePoints != 1705 || p.PreferredPosition != "middle" || len(p.Spells) != 2 || len(p.Runes) != 2 {
		t.Fatalf("incorrect mapped fields: %+v", p)
	}
	if game.Teams[1].Players[0].Streak != "loss" || p.Streak != "" {
		t.Fatal("unsupported badges included or lose streak lost")
	}
	payload, _ := json.Marshal(game)
	if bytes.Contains(payload, []byte("puuid")) || bytes.Contains(payload, []byte(ref.PlayerRef)) || bytes.Contains(payload, []byte("op_score")) || bytes.Contains(payload, []byte("lane_score")) || bytes.Contains(payload, []byte("windows_script")) {
		t.Fatal("unapproved fields leaked")
	}
	for _, team := range game.Teams {
		for _, p := range team.Players {
			if _, ok := a.resolveGameplayReferenceDetails(p.PlayerRef); !ok {
				t.Fatal("not an anonymous reference")
			}
		}
	}
	badRef := ref
	badRef.PlayerRef = "unrelated-player-00000000001"
	if _, err = a.parseOPGGCurrentGame(data, badRef, now); err == nil {
		t.Fatal("accepted unrelated match")
	}
	if _, err = a.parseOPGGCurrentGame(data, ref, now.Add(7*time.Hour)); err == nil {
		t.Fatal("accepted stale game")
	}
	duplicate := bytes.Replace(data, []byte("current-game-fixture-1-0-0000000001"), []byte(ref.PlayerRef), 1)
	if _, err = a.parseOPGGCurrentGame(duplicate, ref, now); err == nil {
		t.Fatal("accepted duplicate identity")
	}
	finished := bytes.Replace(data, []byte(`"is_finished":false`), []byte(`"is_finished":true`), 1)
	if game, err = a.parseOPGGCurrentGame(finished, ref, now); err != nil || game.Status != "none" {
		t.Fatal("finished game shown live")
	}
	if game, err = a.parseOPGGCurrentGame([]byte("0:{\"a\":\"$@1\"}\n1:\"$undefined\""), ref, now); err != nil || game.Status != "none" {
		t.Fatal("explicit no-game failed")
	}
	if _, err = a.parseOPGGCurrentGame([]byte("0:{\"a\":\"$@1\"}\n1:{}"), ref, now); err == nil {
		t.Fatal("invalid result treated as idle")
	}
}
func TestOverviewCurrentGameReadOnlyCacheAndPrivacy(t *testing.T) {
	a, data, ref, _ := currentGameFixture(t)
	data = bytes.Replace(data, []byte("2026-09-09T00:56:11+09:00"), []byte(time.Now().Add(-time.Minute).Format(time.RFC3339)), 1)
	var calls atomic.Int32
	a.champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.Method == http.MethodGet {
			return proHTTPBody(summaryFixtureHTML(ref.PlayerRef, 33)), nil
		}
		if r.Method != "POST" || r.URL.Host != "op.gg" || r.Header.Get("Next-Action") != opggCurrentGameAction || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("unexpected request")
		}
		body, _ := io.ReadAll(r.Body)
		if !bytes.Contains(body, []byte(ref.PlayerRef)) || bytes.Contains(body, []byte("fixture-token")) {
			t.Error("wrong scope")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), Header: http.Header{}}, nil
	})}
	opaque := a.registerGameplayReferenceDetails(ref)
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/gameplay/current-game", strings.NewReader(`{"playerRef":"`+opaque+`"}`))
		a.handleOverviewCurrentGame(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if calls.Load() != 2 {
		t.Fatal("did not cache")
	}
	private := ref
	private.Privacy = "PRIVATE"
	opaque = a.registerGameplayReferenceDetails(private)
	w := httptest.NewRecorder()
	a.handleOverviewCurrentGame(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"playerRef":"`+opaque+`"}`)))
	if w.Code != 403 || calls.Load() != 2 {
		t.Fatal("privacy guard")
	}
	w = httptest.NewRecorder()
	a.handleOverviewCurrentGame(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"playerRef":"`+ref.PlayerRef+`"}`)))
	if w.Code != 404 {
		t.Fatal("accepted raw identity")
	}
	_, err := a.loadCurrentGame(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
}

func TestOverviewCurrentGameRejectsCNWithoutRequests(t *testing.T) {
	a, _, ref, _ := currentGameFixture(t)
	ref.Region = "cn"
	a.champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Fatal("CN current-game request must not reach any transport")
		return nil, nil
	})}
	if _, err := a.loadCurrentGame(context.Background(), ref); err == nil {
		t.Fatal("CN adapter was not disabled before cache or network access")
	}
	opaque := a.registerGameplayReferenceDetails(ref)
	w := httptest.NewRecorder()
	a.handleOverviewCurrentGame(w, httptest.NewRequest("POST", "/api/gameplay/current-game", strings.NewReader(`{"playerRef":"`+opaque+`"}`)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("CN must be explicitly rejected, got %d: %s", w.Code, w.Body.String())
	}
}

func TestOverviewCurrentGamePublicProbe(t *testing.T) {
	if os.Getenv("OPGG_CURRENT_GAME_PROBE") != "1" {
		t.Skip("opt-in public-network verification")
	}
	a, _, _, _ := currentGameFixture(t)
	input, err := os.ReadFile("/private/tmp/opgg-ingame-request.json")
	if err != nil {
		t.Fatal(err)
	}
	var request []struct {
		PUUID string `json:"puuid"`
	}
	if json.Unmarshal(input, &request) != nil || len(request) != 1 {
		t.Fatal("probe input malformed")
	}
	ref := gameplayReference{Region: "kr", PlayerRef: request[0].PUUID, GameName: "JUGKlNG", TagLine: "kr"}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	if _, err = a.champions.loadCatalog(ctx); err != nil {
		t.Fatal("public champion catalogue:", err)
	}
	if data, err := os.ReadFile("/private/tmp/opgg-ingame-response.txt"); err == nil {
		result, err := a.parseOPGGCurrentGame(data, ref, time.Date(2026, 9, 8, 16, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatal("observed response:", err)
		}
		t.Logf("observed public response: status=%s teams=%d recent=%d", result.Status, len(result.Teams), len(result.Teams[0].Players[0].Recent))
	}
	start := time.Now()
	result, err := a.fetchOPGGCurrentGame(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("production public fetch: status=%s teams=%d elapsed=%s", result.Status, len(result.Teams), time.Since(start).Round(time.Millisecond))
}
