package main

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

func TestR291AppendedRosterPreservesReviewedAnchorsAndDates(t *testing.T) {
	if len(proRoster) != 8 || len(proSeedAccounts) != 44 || proRosterVerifiedAt != "2026-10-19" {
		t.Fatal("roster totals/date changed")
	}
	h := sha256.New()
	for i, seed := range proSeedAccounts {
		wantDate := "2026-09-17"
		if i >= 33 {
			wantDate = "2026-10-19"
		}
		if seed.Reviewed != wantDate {
			t.Fatalf("review date %s/%s = %s", seed.TeamCode, seed.Player, seed.Reviewed)
		}
		if i < 33 {
			for _, a := range seed.Accounts {
				fmt.Fprintf(h, "%s/%s:%s#%s\n", seed.TeamCode, seed.Player, a.GameName, a.TagLine)
			}
		}
	}
	// Independently captured from f85daddf's ordered 53 accounts before append.
	if fmt.Sprintf("%x", h.Sum(nil)) != "ac56e60975f2475c5ed6bfd29654c6c96d2d4af72e45b32a3d5e7f693df43b76" {
		t.Fatal("old account order or spelling changed")
	}
	for i, want := range []struct {
		code, name  string
		id, players int
	}{{"AL", "Anyone's Legend", 698, 5}, {"TES", "Top Esports", 415, 6}} {
		got := proRoster[6+i]
		if got.Code != want.code || got.Name != want.name || got.League != "LPL" || got.OPGGID != want.id || len(got.Players) != want.players {
			t.Fatal("new team metadata", got)
		}
	}
}

func TestR291ALTESReviewedAccountsMatchInGameIdentity(t *testing.T) {
	a := new(app)
	a.proPlayers.teams = a.loadProSeeds(t.Context(), nil)
	index := a.proIdentitySnapshot()
	count := 0
	for _, seed := range proSeedAccounts {
		if seed.TeamCode != "AL" && seed.TeamCode != "TES" {
			continue
		}
		for _, account := range seed.Accounts {
			for _, surface := range []string{"match", "live", "overview", "current-game"} {
				badge := a.matchProIdentity(index, surface, gameplayReference{Region: "kr", GameName: account.GameName, TagLine: account.TagLine})
				if badge == nil || badge.TeamCode != seed.TeamCode || badge.PlayerName != seed.Player || badge.Secondary {
					t.Fatalf("wrong identity %s/%s on %s: %+v", seed.TeamCode, seed.Player, surface, badge)
				}
			}
			count++
		}
	}
	if count != 11 {
		t.Fatal("new account coverage", count)
	}
}
