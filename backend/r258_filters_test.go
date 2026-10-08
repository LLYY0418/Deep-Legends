package main

import (
	"encoding/json"
	"testing"
)

func TestR258OverviewExportsExistingSeasonBoundary(t *testing.T) {
	a, _, _ := r258ViewFixture()
	response := gameplayOverview{}
	a.publicizeOverviewReferences(&response)
	if !response.SeasonStart.Equal(seasonStartS26) {
		t.Fatal("history filter has a different season boundary")
	}
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		SeasonStart string `json:"seasonStart"`
	}
	if err = json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SeasonStart != "2026-01-08T00:00:00Z" {
		t.Fatalf("season start not available in overview: %s", raw)
	}
}
