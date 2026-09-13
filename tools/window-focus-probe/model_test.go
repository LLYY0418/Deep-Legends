package main

import "testing"

func sampleOK() snapshot {
	return snapshot{Valid: true, DWMOK: true, DWMResult: "0x00000000", Foreground: "controller", ForegroundAnchor: true, RelativeZ: "below", InputOK: true, InputTick: 100}
}

func candidateTrials() []trial {
	before := sampleOK()
	shown := before
	shown.Visible, shown.Foreground, shown.RelativeZ = true, "probe", "above"
	shown.ForegroundAnchor = false
	hidden := before
	hidden.Visible, hidden.Cloaked = true, 1
	return []trial{
		{Name: "baseline", Complete: true, AnchorPrepared: true, Before: before, Bursts: 1, Samples: []snapshot{shown}},
		{Name: "external_cloak", Complete: true, CloakAttempt: "0x00000000", CloakApplied: true, AnchorPrepared: true, Before: before, Bursts: 3, Samples: []snapshot{hidden}},
		{Name: "self_cloak", Complete: true, CloakAttempt: "0x00000000", CloakApplied: true, AnchorPrepared: true, Before: before, Bursts: 1, Samples: []snapshot{hidden}},
		{Name: "release_show", Complete: true, AnchorPrepared: true, Before: before, Bursts: 1, Samples: []snapshot{shown}},
	}
}

func TestAssessmentNeverOverclaims(t *testing.T) {
	tests := []struct {
		name, want string
		mutate     func([]trial)
	}{
		{"candidate is not proven", "candidate_only", func(ts []trial) {}},
		{"other controller window", "inconclusive", func(ts []trial) { ts[1].Samples[0].ForegroundAnchor = false }},
		{"self control missing", "incomplete", func(ts []trial) { ts[2].Name = "missing" }},
		{"self control incomplete", "incomplete", func(ts []trial) { ts[2].Complete = false }},
		{"self control skipped", "inconclusive", func(ts []trial) { ts[2].SkippedReason = "API rejected" }},
		{"self control rejected", "inconclusive", func(ts []trial) { ts[2].CloakAttempt = "0x80070005" }},
		{"self control not applied", "inconclusive", func(ts []trial) { ts[2].CloakApplied = false }},
		{"self control failed to hide", "inconclusive", func(ts []trial) { ts[2].Samples[0].Cloaked = 0 }},
		{"access denied", "external_cloak_rejected", func(ts []trial) { ts[1].CloakAttempt = "0x80070005"; ts[1].Samples = nil; ts[1].Bursts = 0 }},
		{"reported rejection with unknown hidden z", "external_cloak_rejected", func(ts []trial) {
			ts[1].CloakAttempt = "0x80070005"
			ts[1].CloakApplied = false
			ts[1].Before.RelativeZ = "unknown"
			ts[1].Samples = nil
			ts[1].Bursts = 0
			ts[1].SkippedReason = "cloak was not applied"
		}},
		{"missing api", "external_cloak_rejected", func(ts []trial) { ts[1].CloakAttempt = "0x80004001" }},
		{"success without readback", "external_cloak_not_applied", func(ts []trial) { ts[1].CloakApplied = false }},
		{"visible despite mask", "requirement_not_met", func(ts []trial) { ts[1].Samples[0].Cloaked = 0 }},
		{"hidden but stole focus", "requirement_not_met", func(ts []trial) { ts[1].Samples[0].Foreground = "probe" }},
		{"hidden but transient topmost", "requirement_not_met", func(ts []trial) {
			ts[1].Samples = append(ts[1].Samples, ts[1].Samples[0])
			ts[1].Samples[0].Topmost = true
		}},
		{"hidden but changed z", "requirement_not_met", func(ts []trial) { ts[1].Samples[0].RelativeZ = "above" }},
		{"read error not hidden", "inconclusive", func(ts []trial) { ts[1].Samples[0].DWMOK = false }},
		{"unknown foreground", "inconclusive", func(ts []trial) { ts[1].Samples[0].Foreground = "none" }},
		{"third party foreground", "inconclusive", func(ts []trial) { ts[1].Samples[0].Foreground = "other" }},
		{"unknown z", "inconclusive", func(ts []trial) { ts[1].Samples[0].RelativeZ = "unknown" }},
		{"empty observations", "inconclusive", func(ts []trial) { ts[1].Samples = nil }},
		{"weak baseline", "inconclusive", func(ts []trial) { ts[0].Samples[0].Foreground = "controller" }},
		{"no baseline display", "inconclusive", func(ts []trial) { ts[0].Samples[0].Visible = false }},
		{"anchor not ready", "inconclusive", func(ts []trial) { ts[1].AnchorPrepared = false }},
		{"input interference", "inconclusive", func(ts []trial) { ts[1].Samples[0].InputTick++ }},
		{"missing input information", "inconclusive", func(ts []trial) { ts[1].Samples[0].InputOK = false }},
		{"insufficient repeats", "inconclusive", func(ts []trial) { ts[1].Bursts = 1 }},
		{"failed release", "inconclusive", func(ts []trial) { ts[3].Samples[0].Cloaked = 1 }},
		{"incomplete release", "incomplete", func(ts []trial) { ts[3].Complete = false }},
		{"incomplete external", "incomplete", func(ts []trial) { ts[1].Complete = false }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := candidateTrials()
			tc.mutate(ts)
			if got := assess(ts, true); got.Code != tc.want {
				t.Fatalf("got %+v want %s", got, tc.want)
			}
		})
	}
	if assess(nil, true).Code != "incomplete" {
		t.Fatal("missing trials cannot pass")
	}
	if assess(candidateTrials(), false).Code != "cleanup_incomplete" {
		t.Fatal("cleanup failure cannot pass")
	}
}

func TestPresentationRequiresKnownDWMAndNotMinimized(t *testing.T) {
	s := sampleOK()
	s.Visible = true
	if !s.presentable() {
		t.Fatal("known visible window")
	}
	s.Cloaked = 1
	if s.presentable() {
		t.Fatal("cloak is not on-screen visibility")
	}
	s.Cloaked = 0
	s.DWMOK = false
	if s.presentable() {
		t.Fatal("unknown != visible")
	}
	s.DWMOK = true
	s.Minimized = true
	if s.presentable() {
		t.Fatal("minimized window")
	}
}
