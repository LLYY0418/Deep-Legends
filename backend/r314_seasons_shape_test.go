package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestR314RankedSeasonsShapeHasOnlyStructuralAndNumericData(t *testing.T) {
	raw := json.RawMessage(`{"puuid":"private-id","seasons":[{"seasonId":2026,"split":1,"wins":28,"losses":17,"gamesPlayed":45,"puuid":"private-id","name":"private-name","rating":999,"nested":{"totalWins":48,"token":"secret"}}]}`)
	b, _ := json.Marshal(diagnosticRankedSeasonsShape(raw, "lcu"))
	s := string(b)
	for _, secret := range []string{"private-id", "private-name", "secret", "999"} {
		if strings.Contains(s, secret) {
			t.Fatal(s)
		}
	}
	for _, want := range []string{`"length":1`, `"wins":28`, `"losses":17`, `"seasonId":2026`, `"totalWins":48`} {
		if !strings.Contains(s, want) {
			t.Fatal(s)
		}
	}
}
