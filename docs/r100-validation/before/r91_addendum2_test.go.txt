package main

import "testing"

func TestR91Addendum2NonWildcardBanClearYields(t *testing.T) {
	for group, queue := range map[string]int64{"ranked": 420, "arena": 1750} {
		for _, entry := range []string{"poll", "preflight"} {
			t.Run(group+"/"+entry, func(t *testing.T) {
				f := newExecutionFixture(t)
				f.session.QueueID = queue
				f.session.Timer.Phase = "BAN_PICK"
				f.bans = []int64{141, 104} // Real IDs, no wildcard confirmation path.
				s := f.r.currentWatch()
				g := s.ChampSelect.Groups[group]
				g.Ban.Enabled, g.Ban.DelayMS, g.Ban.Strategy = true, 0, "show-then-lock"
				g.Ban.Champions["default"] = []int64{141, 104}
				s.ChampSelect.Groups[group] = g
				f.r.apply(s)
				f.tick(t)
				if f.count() != 1 || f.last().Body["completed"] != false {
					t.Fatal("initial non-wildcard hover missing")
				}
				s = f.r.currentWatch()
				g = s.ChampSelect.Groups[group]
				lockDelay := 120
				g.Ban.LockDelayMS = &lockDelay // R95: active lock uses lockDelayMs, not hidden hover delay.
				s.ChampSelect.Groups[group] = g
				f.r.apply(s)
				f.r.evaluateChampSelect(f.c, f.r.currentWatch().ChampSelect)
				f.r.mu.Lock()
				last, armed := f.r.champSelect.submitted[0], f.r.pending[champSelectActionBan] != nil
				f.r.mu.Unlock()
				if !last.Confirmed || last.Decision.ForceHover || !armed {
					t.Fatalf("fixture must confirm ordinary hover and arm lock: %+v armed=%t", last, armed)
				}
				f.mu.Lock()
				f.session.Actions[0][0].ChampionID = 0
				f.mu.Unlock()
				if entry == "poll" {
					f.r.evaluateChampSelect(f.c, f.r.currentWatch().ChampSelect)
				}
				// With no intervening poll, the actual delayed write must reject the clear.
				waitTakeoverIdle(t, f)
				state := f.r.champSelectSnapshot()
				if f.count() != 1 || state.BanStates["141"] != "manual-takeover" {
					t.Fatalf("cleared non-wildcard ban was not respected: patches=%d state=%+v", f.count(), state)
				}
				f.tick(t)
				if f.count() != 1 {
					t.Fatal("later poll rearmed the canceled ban")
				}
				f.mu.Lock()
				defer f.mu.Unlock()
				for _, event := range f.events {
					if event["reason"] == "hover-cleared-by-client" {
						t.Fatal("manual clear was mislabeled as client compatibility")
					}
				}
			})
		}
	}
}
