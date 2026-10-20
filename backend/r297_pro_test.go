package main

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestR297ProOpponentNamesAndLayoutFixture(t *testing.T) {
	rows, _, _ := proOPGGRecommendations(proOPGGFixture(t), 103, "mid", "阿狸", proOPGGNow(), time.Now(), false)
	if len(rows) == 0 {
		t.Fatal("fixture empty")
	}
	for _, row := range rows {
		if row.OpponentChampionID > 0 && row.OpponentChampionName == "" {
			t.Fatalf("missing localized opponent champion %d", row.OpponentChampionID)
		}
	}
	if path := os.Getenv("R297_PRO_RESPONSE"); path != "" {
		raw, _ := json.Marshal(map[string]any{"pros": rows})
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
