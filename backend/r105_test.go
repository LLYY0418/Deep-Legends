package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func r105PlayerAccounts(name string) []proSeedAccountRef {
	for _, seed := range proSeedAccounts {
		if seed.Player == name {
			return seed.Accounts
		}
	}
	return nil
}

func TestR105_EmbeddedAccountCounts(t *testing.T) {
	total := 0
	for _, seed := range proSeedAccounts {
		total += len(seed.Accounts)
	}
	if total != 53 {
		t.Fatalf("total accounts = %d, want 53", total)
	}
	TestR102SeedAccountsMatchVerificationDoc(t)
}

func TestR105_PlayerAccountCounts(t *testing.T) {
	want := map[string]int{"Bin": 1, "Wenbo": 1, "Flandre": 1, "Xun": 2, "knight": 2, "Viper": 1, "ON": 1, "TheShy": 5, "Wei": 2, "Rookie": 4, "Assum": 1, "JiaQi": 2, "Meiko": 1, "Doran": 1, "Oner": 1, "Faker": 1, "Peyz": 1, "Keria": 1, "Zeus": 1, "Kanavi": 2, "Zeka": 2, "Gumayusi": 2, "Delight": 1, "Kiin": 1, "Canyon": 1, "Chovy": 1, "Ruler": 1, "Duro": 1, "Siwoo": 2, "Lucid": 2, "ShowMaker": 2, "Smash": 3, "Career": 2}
	if len(proSeedAccounts) != 33 {
		t.Fatal("want 33 players")
	}
	for name, count := range want {
		if got := len(r105PlayerAccounts(name)); got != count {
			t.Errorf("%s: got %d accounts, want %d", name, got, count)
		}
	}
}

func TestR105_CriticalAccountFixes(t *testing.T) {
	want := map[string][]proSeedAccountRef{"Wei": {{"dyjkbysb", "KR1"}}, "Rookie": {{"벼락식혜", "0070"}, {"EmberKnight", "KR0"}}, "Xun": {{"我累铜泥丸", "小重o"}}, "knight": {{"BLG 온", "KR1"}}, "TheShy": {{"은여하", "1103"}}, "Smash": {{"Smash", "KR2"}}}
	for player, refs := range want {
		for _, ref := range refs {
			found := false
			for _, actual := range r105PlayerAccounts(player) {
				found = found || actual == ref
			}
			if !found {
				t.Errorf("%s missing %s#%s", player, ref.GameName, ref.TagLine)
			}
		}
	}
	for _, ref := range r105PlayerAccounts("Rookie") {
		if ref.GameName == "dyjkbysb" {
			t.Error("dyjkbysb belongs to Wei")
		}
	}
}

func TestR105_SpecialCharacters(t *testing.T) {
	for name, ref := range map[string]proSeedAccountRef{"Canyon": {"JUGKlNG", "kr"}, "Ruler": {"강 철", "샤 넬"}} {
		if !reflect.DeepEqual(r105PlayerAccounts(name), []proSeedAccountRef{ref}) {
			t.Fatalf("%s spelling/case/space mismatch", name)
		}
	}
}

func r105AssertPage(t *testing.T, result proPlayersResponse) {
	t.Helper()
	if result.PlayerCount != 33 || result.AccountCount != 53 || len(result.Teams) != 6 {
		t.Fatalf("page totals: %d/%d/%d", result.PlayerCount, result.AccountCount, len(result.Teams))
	}
	for _, team := range result.Teams {
		for _, player := range team.Players {
			want := r105PlayerAccounts(player.Name)
			if len(player.Accounts) != len(want) {
				t.Errorf("%s page count %d want %d", player.Name, len(player.Accounts), len(want))
			}
			for _, ref := range want {
				found := false
				for _, account := range player.Accounts {
					found = found || account.Reviewed && account.GameName == ref.GameName && account.TagLine == ref.TagLine
				}
				if !found {
					t.Errorf("%s page missing %s#%s", player.Name, ref.GameName, ref.TagLine)
				}
			}
		}
	}
}

func TestR105_ReviewedPageSurvivesOldSnapshotAndWrongOwner(t *testing.T) {
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	if err := os.MkdirAll(filepath.Join(store.root, "logs"), 0700); err != nil {
		t.Fatal(err)
	}
	a := &app{storage: store}
	a.proPlayers.mu.Lock()
	a.restoreProSnapshotLocked()
	// Historical index and PUUID claim must not steal Wei or remove Rookie.
	wrong := opggProAccount{GameName: "dyjkbysb", TagLine: "KR1", PUUID: "old-puuid", Source: "seed", SeedKey: "proseed:v1:IG/rookie/0", UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	a.proPlayers.teams = []opggProTeam{{ID: 28, Members: []opggProMember{{Nickname: "Rookie", Authority: "PROGAMER", Summoners: []opggProAccount{wrong}}}}}
	a.proPlayers.fetchedAt = time.Now().Add(-time.Hour)
	a.persistProSnapshotLocked()
	a.proPlayers.mu.Unlock()
	reboot := &app{storage: store}
	w := httptest.NewRecorder()
	reboot.handleProPlayers(w, httptest.NewRequest("GET", "/api/pro-players", nil))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var result proPlayersResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	r105AssertPage(t, result)
	if output := os.Getenv("R105_PUBLIC_OUTPUT"); output != "" {
		if err := os.WriteFile(output, w.Body.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	log, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	var event struct {
		Event    string    `json:"event"`
		Teams    []proTeam `json:"teams"`
		Players  int       `json:"playerCount"`
		Accounts int       `json:"accountCount"`
	}
	for _, line := range strings.Split(string(log), "\n") {
		var e = event
		if json.Unmarshal([]byte(line), &e) == nil && e.Event == "pro_players" {
			event = e
		}
	}
	r105AssertPage(t, proPlayersResponse{Teams: event.Teams, PlayerCount: event.Players, AccountCount: event.Accounts})
	if strings.Contains(string(log), "old-puuid") {
		t.Fatal("private upstream PUUID leaked")
	}
}

func TestR105_AllPublicationsAndOfflineKeepReviewedAccounts(t *testing.T) {
	directory, err := parseOPGGProPlayers(r104FixtureFlight(t))
	if err != nil {
		t.Fatal(err)
	}
	a := new(app)
	seeds := a.loadProSeeds(context.Background(), nil, directory)
	for _, source := range [][]opggProTeam{nil, directory, withProSeed(directory, seeds), withProSeed(append(cloneProTeams(directory), pendingProSupplements()...), seeds)} {
		r105AssertPage(t, a.buildReviewedProPlayers(source))
	}
	// Unknown activity preserves document order, independent of LP. Known
	// lower-ranked Smash#KR2 wins once a valid activity time is supplied.
	result := a.buildReviewedProPlayers(nil)
	for _, team := range result.Teams {
		for _, player := range team.Players {
			for i, ref := range r105PlayerAccounts(player.Name) {
				if player.Accounts[i].GameName != ref.GameName || player.Accounts[i].TagLine != ref.TagLine {
					t.Fatal("document order changed", player.Name)
				}
			}
		}
	}
	rows := []opggProTeam{{Members: []opggProMember{{Summoners: []opggProAccount{{GameName: "Smash", TagLine: "KR2", LastMatchAtKnown: true, LastMatchAt: "2026-09-17T12:00:00Z"}, {GameName: "DK Smash", TagLine: "KR7", LastMatchAtKnown: true, LastMatchAt: "2026-09-16T12:00:00Z", Rank: json.RawMessage(`{"tier":"CHALLENGER","division":1,"lp":999}`)}}}}}}
	for _, team := range a.buildReviewedProPlayers(rows).Teams {
		for _, p := range team.Players {
			if p.Name == "Smash" && p.Accounts[0].GameName != "Smash" {
				t.Fatal("LP displaced activity ordering")
			}
		}
	}
}

// Run the real app handler and LCU RequestJSON with an in-memory transport.
// No localhost listener or network dependency is needed for these assertions.
func r105LCU(handler http.HandlerFunc) *LCUClient {
	return &LCUClient{baseURL: "https://fixture.invalid", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		w := httptest.NewRecorder()
		handler(w, r)
		return w.Result(), nil
	})}}
}
