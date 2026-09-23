package main

import (
	"encoding/json"
	"math"
	"testing"
)

func TestR115AbilityPairsExactlyTheSameGames(t *testing.T) {
	matches := []gameplayMatch{abilityTestMatch(1), abilityTestMatch(2), abilityTestMatch(3)}
	missing := abilityTestMatch(4)
	missing.Participants = missing.Participants[:1]
	missing.Participants[0].Kills = 100
	ambiguous := abilityTestMatch(5)
	ambiguous.Participants = append(ambiguous.Participants, ambiguous.Participants[1])
	matches = append(matches, missing, ambiguous)
	p, b, _ := gameplayAbilityStatsForQueue(matches, "subject", 420)
	if p.games != 3 || b.games != 3 || p.kills != 12 {
		t.Fatalf("unpaired stats: %#v / %#v", p, b)
	}
	cached := []seasonRankedMatch{seasonAbilityTestMatch(1, 420), seasonAbilityTestMatch(2, 420), seasonAbilityTestMatch(3, 420), seasonAbilityTestMatch(4, 420)}
	cached[3].Ability.Opponent = nil
	p, b, _ = seasonAbilityStatsForQueue(cached, 420)
	if p.games != 3 || b.games != 3 {
		t.Fatal("snapshot admitted unpaired sample")
	}
}

func TestR115AbilityMissingMetricDoesNotEraseOtherMetrics(t *testing.T) {
	matches := []gameplayMatch{abilityTestMatch(1), abilityTestMatch(2), abilityTestMatch(3)}
	for i := range matches {
		matches[i].Participants[1].VisionScore = 0
		matches[i].Teams[0].Kills = 0
	}
	profile := buildGameplayAbilityProfile(matches, "subject", nil, "")
	if profile == nil || len(profile.Metrics) != 7 {
		t.Fatal("missing dimension erased profile")
	}
	for _, m := range profile.Metrics {
		missing := m.Key == "vspm" || m.Key == "killParticipation"
		if m.Unavailable != missing {
			t.Fatalf("availability %s: %#v", m.Key, m)
		}
		if math.IsNaN(m.PlayerScore) || math.IsInf(m.PlayerScore, 0) {
			t.Fatal("nonfinite score")
		}
	}
	if _, err := json.Marshal(profile); err != nil {
		t.Fatal(err)
	}
}

func TestR115AbilityKDAIsInvariantWhenZeroDeathGamesRepeat(t *testing.T) {
	var p, b gameplayAbilityAccumulator
	subject := abilityTestMatch(1).Participants[0]
	subject.Deaths = 0
	opponent := abilityTestMatch(1).Participants[1]
	opponent.Deaths = 0
	team := gameplayTeam{Kills: 20, Damage: 20000}
	p.add(subject, team, 1200)
	b.add(opponent, team, 1200)
	first := abilityMetrics(p, b)[0]
	for i := 0; i < 4; i++ {
		p.add(subject, team, 1200)
		b.add(opponent, team, 1200)
	}
	repeated := abilityMetrics(p, b)[0]
	if first.Player != repeated.Player || first.PlayerScore != repeated.PlayerScore {
		t.Fatalf("sample count changed KDA: %#v / %#v", first, repeated)
	}
}

func TestR115AbilityBoundedSymmetricScaleAndTrueZero(t *testing.T) {
	var p, b gameplayAbilityAccumulator
	item := abilityTestMatch(1).Participants[0]
	team := gameplayTeam{Kills: 20, Damage: 20000}
	p.add(item, team, 1200)
	b.add(item, team, 1200)
	for _, m := range abilityMetrics(p, b) {
		if m.PlayerScore != 50 {
			t.Fatalf("equal sample should be 50: %#v", m)
		}
	}
	p.damage = 0
	if m := abilityMetrics(p, b)[3]; m.PlayerScore != 0 || m.Unavailable {
		t.Fatalf("real zero needs no artificial floor: %#v", m)
	}
	p.damage = 1_000_000
	forward := abilityMetrics(p, b)[3].PlayerScore
	reverse := abilityMetrics(b, p)[3].PlayerScore
	if math.Abs(forward+reverse-100) > 0.11 || forward > 100 || reverse < 0 {
		t.Fatalf("unbounded/asymmetric: %v,%v", forward, reverse)
	}
}

func TestR115SeasonAbilityRejectsAmbiguousOpponents(t *testing.T) {
	info := &riotMatchInfo{GameDuration: 1200, Participants: []riotParticipant{
		{TeamID: 100, TeamPosition: "TOP"}, {TeamID: 200, TeamPosition: "TOP"}, {TeamID: 200, TeamPosition: "TOP"},
	}}
	if got := seasonRankedAbilitySampleFor(info, info.Participants[0], "top"); got != nil {
		t.Fatal("ambiguous role accepted")
	}
}
