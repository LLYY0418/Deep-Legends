package main

import (
	"testing"
)

func TestR303AutoConfigSkipDeduplicatesPerSession(t *testing.T) {
	a := r175App(t)
	a.autoConfig.phase = "ChampSelect"
	for _, reason := range []string{"disabled", "category_off", "no_mode", "no_config", "not_locked", "yielded", "guard"} {
		for i := 0; i < 3; i++ {
			a.recordAutoConfigSkip(reason, "normal")
		}
	}
	rows := readDiagnosticEvents(t, a.storage, "auto_config_skip")
	if len(rows) != 7 {
		t.Fatal(rows)
	}
	for _, row := range rows {
		if row["modeKey"] != "normal" || row["phase"] != "ChampSelect" {
			t.Fatal(row)
		}
	}
	a.resetAutoConfig()
	a.recordAutoConfigSkip("disabled", "normal")
	if len(readDiagnosticEvents(t, a.storage, "auto_config_skip")) != 8 {
		t.Fatal("new session must emit again")
	}
}
func TestR303ParticipantSelfUsesLiveIdentityPredicate(t *testing.T) {
	a := r175App(t)
	a.summoner.PUUID = r161Ref(0)
	match := gameplayMatch{Participants: []gameplayParticipant{{PlayerRef: r161Ref(0), GameName: "本人"}, {PlayerRef: r161Ref(1), GameName: "别人"}}}
	a.publicizeMatchReferences(&match)
	if !match.Participants[0].IsCurrent || match.Participants[1].IsCurrent {
		t.Fatal(match.Participants)
	}
}
