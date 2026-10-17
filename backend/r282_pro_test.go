package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestR282WorldsOutsideRosterAndCrossLaneRows(t *testing.T) {
	p := newProRuneProvider(proTestProvider(), nil, nil)
	event := proTestEvent()
	event.League.ID = "98767975604431411"
	event.Match.Teams[0].ID = "100000001"
	event.Match.Teams[0].Code = "PNG"
	event.Match.Teams[1].ID = "100000002"
	event.Match.Teams[1].Code = "PSG"
	if !proEventAllowed(event) {
		t.Fatal("Worlds non-six-team match was discarded")
	}
	now := time.Now()
	g := proTestGame()
	g.LeagueID = event.League.ID
	g.Metadata.Blue.TeamID = event.Match.Teams[0].ID
	g.Metadata.Red.TeamID = event.Match.Teams[1].ID
	g.Metadata.Blue.Participants[0].SummonerName = "PNG Player"
	g.Metadata.Blue.Participants[0].Role = "top"
	snapshot := proRuneSnapshot{Games: map[string]proRuneIndexGame{g.ID: g}, Events: map[string]proRuneEvent{event.ID: event}}
	rows := p.rows(snapshot, now)
	selected := proNewestRows(rows, "Cassiopeia", "middle")
	if len(selected) != 1 || selected[0].Player != "Player" || selected[0].Participant.Role != "top" {
		t.Fatalf("Worlds cross lane row missing: %+v", selected)
	}
	same := selected[0]
	same.Participant.Role = "mid"
	same.Participant.PlayerID = "111"
	other := selected[0]
	other.Participant.PlayerID = "000"
	same.Player = "Preferred"
	tied := proNewestRows([]proRuneRow{other, same}, "Cassiopeia", "middle")
	if tied[0].Player != "Preferred" {
		t.Fatal("lane must break time ties")
	}
	other.Game.Start = now.Add(-time.Minute)
	tied = proNewestRows([]proRuneRow{same, other}, "Cassiopeia", "middle")
	if tied[0].Participant.Role != "top" {
		t.Fatal("time must precede lane")
	}
	if proEventTeamCode(event, event.Match.Teams[0].ID) != "PNG" {
		t.Fatal("event code ignored")
	}
}

func TestR282RecordedWorldsMetadataIndexesEveryTeamAndReportsLeagueCounts(t *testing.T) {
	for _, prefix := range []string{"worlds", "playin"} {
		t.Run(prefix, func(t *testing.T) { r282RecordedWorlds(t, prefix) })
	}
}
func r282RecordedWorlds(t *testing.T, prefix string) {
	eventBytes, err := os.ReadFile("testdata/r282/" + prefix + "-event.json")
	if err != nil {
		t.Fatal(err)
	}
	windowBytes, err := os.ReadFile("testdata/r282/" + prefix + "-window.json")
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Data struct {
			Event proRuneEvent `json:"event"`
		} `json:"data"`
	}
	if err = json.Unmarshal(eventBytes, &wire); err != nil {
		t.Fatal(err)
	}
	var window proWindow
	if err = json.Unmarshal(windowBytes, &window); err != nil || len(window.Frames) == 0 {
		t.Fatal(err)
	}
	event := wire.Data.Event
	// Replay only the game for which this fixture captured a real window.
	var identity struct {
		GameID string `json:"esportsGameId"`
	}
	json.Unmarshal(windowBytes, &identity)
	if identity.GameID == "" {
		identity.GameID = event.Match.Games[0].ID
	}
	games := event.Match.Games[:0]
	for _, game := range event.Match.Games {
		if game.ID == identity.GameID {
			games = append(games, game)
		}
	}
	event.Match.Games = games
	wire.Data.Event = event
	eventBytes, _ = json.Marshal(wire)
	if prefix == "playin" {
		raw, _ := os.ReadFile("testdata/r282/playin-schedule.json")
		if !strings.Contains(string(raw), "Play-Ins") {
			t.Fatal("recorded stage missing")
		}
	}
	for _, team := range event.Match.Teams {
		if proRuneTeams[team.ID] != "" {
			t.Fatal("fixture must cover teams outside original six")
		}
	}
	now := window.Frames[0].Timestamp.Add(time.Hour)
	provider := proTestProvider()
	var eventsMu sync.Mutex
	events := []map[string]any{}
	provider.diag = func(e map[string]any) {
		eventsMu.Lock()
		defer eventsMu.Unlock()
		if e["event"] == "pro_rune_index" {
			events = append(events, e)
		}
	}
	provider.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		var body []byte
		switch {
		case strings.HasSuffix(r.URL.Path, "getSchedule"):
			schedule := map[string]any{"data": map[string]any{"schedule": map[string]any{"events": []any{}, "pages": map[string]string{}}}}
			if r.URL.Query().Get("leagueId") == event.League.ID {
				schedule["data"].(map[string]any)["schedule"].(map[string]any)["events"] = []any{map[string]any{"startTime": window.Frames[0].Timestamp, "state": "completed", "match": map[string]string{"id": event.ID}}}
			}
			body, _ = json.Marshal(schedule)
		case strings.HasSuffix(r.URL.Path, "getEventDetails"):
			body = eventBytes
		case strings.Contains(r.URL.Path, "/window/"):
			body = windowBytes
		default:
			t.Fatalf("unexpected fixture request %s", r.URL.Path)
		}
		return r206RelayResponse(200, body), nil
	})}
	p := newProRuneProvider(provider, nil, nil)
	p.refresh(context.Background(), now)
	rows := p.rows(p.snapshot, now)
	if len(rows) < 10 {
		t.Fatalf("official Worlds participants missing: %d", len(rows))
	}
	champion := window.Metadata.Blue.Participants[0].ChampionID
	selected := proNewestRows(rows, champion, "middle")
	if len(selected) == 0 || selected[0].Participant.Role != "top" {
		t.Fatal("recorded top lane must be available for mid request")
	}
	if len(events) != 5 {
		t.Fatal("one index diagnostic per league required", events)
	}
	for _, e := range events {
		if e["league"] == "Worlds" && e["included"].(int) == 0 {
			t.Fatal("Worlds inclusion counter missing", e)
		}
	}
}

func TestR282RecordedPlayinProducesCrossLaneRuneCard(t *testing.T) {
	read := func(name string) []byte {
		b, err := os.ReadFile("testdata/r282/playin-" + name + ".json")
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	var wire struct {
		Data struct {
			Event proRuneEvent `json:"event"`
		} `json:"data"`
	}
	json.Unmarshal(read("event"), &wire)
	var start, end proWindow
	var details proDetails
	json.Unmarshal(read("window"), &start)
	json.Unmarshal(read("end"), &end)
	json.Unmarshal(read("details"), &details)
	var id struct {
		GameID string `json:"esportsGameId"`
	}
	json.Unmarshal(read("window"), &id)
	provider := proTestProvider()
	var champions map[string]struct{ ID, Key, Name string }
	json.Unmarshal(read("champions"), &champions)
	for _, meta := range champions {
		n, err := strconv.Atoi(meta.Key)
		if err != nil {
			t.Fatal(err)
		}
		provider.championMeta[n] = championMetadata{ID: n, Key: meta.ID, Slug: strings.ToLower(meta.ID), NameZH: meta.Name}
		provider.championIDs[strings.ToLower(meta.ID)] = n
	}
	p := newProRuneProvider(provider, nil, nil)
	now := start.Frames[0].Timestamp.Add(time.Hour)
	p.now = func() time.Time { return now }
	event := wire.Data.Event
	g := proRuneIndexGame{ID: id.GameID, MatchID: event.ID, LeagueID: event.League.ID, Number: 1, Start: start.Frames[0].Timestamp, Metadata: start.Metadata}
	p.snapshot = proRuneSnapshot{ReadAt: now, Games: map[string]proRuneIndexGame{g.ID: g}, Events: map[string]proRuneEvent{event.ID: event}}
	p.ends[g.ID] = end
	p.details[g.ID] = details
	champion := provider.championIDs[strings.ToLower(g.Metadata.Blue.Participants[0].ChampionID)]
	response := p.recommend(context.Background(), champion, "middle")
	if len(response.Pros) != 1 || response.Pros[0].Position != "上路" || !strings.Contains(response.Pros[0].EventLabel, "Worlds") {
		t.Fatalf("recorded play-in card missing: %+v", response)
	}
	if response.Pros[0].PrimaryStyleID <= 0 || len(response.Pros[0].SelectedPerkIDs) < 6 {
		t.Fatal("recorded runes missing")
	}
	if output := os.Getenv("R282_PRO_UI_OUTPUT"); output != "" {
		raw, _ := json.MarshalIndent(response, "", "  ")
		if err := os.WriteFile(output, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
