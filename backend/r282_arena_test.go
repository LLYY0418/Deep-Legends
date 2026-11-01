package main

import (
	"testing"
)

func TestR282BraveryActionAndIntentPreservedForOwnTeam(t *testing.T) {
	cell := int64(7)
	for _, mode := range []string{"action", "champion", "intent"} {
		s := lcuChampSelectSession{MyTeam: []lcuChampSelectPlayer{{CellID: &cell, PUUID: "r282-private-reference"}}}
		switch mode {
		case "action":
			s.Actions = [][]lcuChampSelectAction{{{ActorCellID: cell, Type: "pick", ChampionID: -3}}}
		case "champion":
			s.MyTeam[0].ChampionID = -3
		case "intent":
			s.MyTeam[0].ChampionPickIntent = -3
		}
		rows := mergeChampSelectPlayers(nil, s, 0)
		if len(rows) != 1 || !rows[0].player.Bravery {
			t.Fatalf("%s lost bravery: %+v", mode, rows)
		}
	}
}
