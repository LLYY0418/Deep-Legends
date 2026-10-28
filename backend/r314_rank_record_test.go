package main

import "testing"

func TestR314MissingSeasonLossesUseEvenFill(t *testing.T) {
	for _, c := range []struct {
		official, w, l, wantW, wantL, rate int
		unknown                            bool
	}{
		{49, 20, 31, 49, 60, 45, false}, {154, 85, 98, 154, 167, 48, false},
		{178, 177, 180, 177, 180, 50, false}, {11, 11, 15, 11, 15, 42, false},
		{49, 3, 2, 49, 0, -1, true}, {49, 0, 12, 49, 61, 45, false},
	} {
		v := estimateSeasonRankRecord(c.official, gameplayAggregate{Games: c.w + c.l, Wins: c.w, Losses: c.l, WinRate: c.rate}, c.w+c.l)
		if v.wins != c.wantW || v.losses != c.wantL || v.winRate != c.rate || v.lossUnknown != c.unknown {
			t.Errorf("%+v: got %+v", c, v)
		}
	}
}
