package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestR210SnapshotRejectsIdlePhasesAndClearsFinishedReveal(t *testing.T) {
	for _, phase := range []string{"Lobby", "None", "Matchmaking", "ReadyCheck", "ChampSelect"} {
		t.Run(phase, func(t *testing.T) {
			f := newR197Fixture(t, 2400)
			f.run(t)
			if snapshot, ok := f.a.postGameSnapshot(f.client, "EndOfGame", 0); !ok || len(snapshot.Players) != 10 || snapshot.Players[9].Hidden {
				t.Fatal("end-game reveal lost")
			}
			if _, ok := f.a.postGameSnapshot(f.client, phase, 0); ok {
				t.Fatal("old roster returned in", phase)
			}
			if f.a.postGameReveal.client != nil || f.a.postGameReveal.snapshot.GameID != 0 || f.a.postGameReveal.expiryTimer != nil {
				t.Fatal("reveal not cleared")
			}
			if phase != "ChampSelect" {
				rows := r175Events(t, f.a, "live_roster_post_game_reveal")
				if len(rows) != 2 || rows[1]["reason"] != "left_end_of_game" {
					t.Fatal(rows)
				}
			}
		})
	}
}

func TestR210ObserveLeavingEndStopsWaitingAndFinishedReveal(t *testing.T) {
	for _, finished := range []bool{false, true} {
		f := newR197Fixture(t, 2400)
		if finished {
			f.run(t)
		} else {
			f.a.observePostGameReveal(context.Background(), f.client, "WaitingForStats")
		}
		f.a.observeGameplayPhase(context.Background(), f.client, "Lobby")
		f.a.observeGameplayPhase(context.Background(), f.client, "None")
		if f.a.postGameReveal.client != nil || f.a.postGameReveal.snapshot.GameID != 0 {
			t.Fatal("retained roster survived phase observation")
		}
		if snapshot := f.a.cachedGameplayLive(context.Background(), f.client, f.a.summoner, "None"); snapshot.Available || len(snapshot.Players) != 0 || snapshot.GameID != 0 {
			t.Fatal("normal live cache republished old roster", snapshot)
		}
		count := 0
		for _, row := range r175Events(t, f.a, "live_roster_post_game_reveal") {
			if row["reason"] == "left_end_of_game" {
				count++
			}
		}
		if count != 1 {
			t.Fatal("leaving diagnostic count", count)
		}
	}
}

func TestR210RetentionExpiresTwoMinutesAfterCompletionWithoutRestart(t *testing.T) {
	for _, failures := range []int{0, 3} {
		f := newR197Fixture(t, 2400)
		f.failDetails = failures
		f.run(t)
		finishedAt := f.now
		f.now = finishedAt.Add(2*time.Minute - time.Millisecond)
		if _, ok := f.a.postGameSnapshot(f.client, "EndOfGame", 0); !ok {
			t.Fatal("expired before two minutes after completion")
		}
		f.now = finishedAt.Add(2 * time.Minute)
		if _, ok := f.a.postGameSnapshot(f.client, "EndOfGame", 0); ok {
			t.Fatal("retention exceeded two minutes after completion")
		}
		f.a.observePostGameReveal(context.Background(), f.client, "EndOfGame")
		if _, ok := f.a.postGameSnapshot(f.client, "EndOfGame", 0); ok {
			t.Fatal("duplicate EndOfGame resurrected expired game")
		}
		count := 0
		for _, row := range r175Events(t, f.a, "live_roster_post_game_reveal") {
			if row["reason"] == "expired" {
				count++
			}
		}
		if count != 1 {
			t.Fatal("expiry diagnostic count", count)
		}
	}
}

func TestR210ExpiryTimerClearsWithoutSnapshotReadsAndFencesOldGeneration(t *testing.T) {
	f := newR197Fixture(t, 2400)
	f.run(t)
	s := &f.a.postGameReveal
	var clockMu sync.Mutex
	now := f.now.Add(2 * time.Minute)
	s.mu.Lock()
	oldGeneration := s.generation
	s.now = func() time.Time { clockMu.Lock(); defer clockMu.Unlock(); return now }
	s.expiryTimer.Reset(time.Millisecond)
	s.mu.Unlock()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		cleared := s.client == nil
		s.mu.Unlock()
		if cleared {
			break
		}
		time.Sleep(time.Millisecond)
	}
	s.mu.Lock()
	cleared := s.client == nil
	s.mu.Unlock()
	if !cleared {
		t.Fatal("timer did not automatically expire roster")
	}
	f.a.observePostGameReveal(context.Background(), f.client, "ChampSelect")
	f.a.observePostGameReveal(context.Background(), f.client, "WaitingForStats")
	f.a.expirePostGameReveal(f.client, oldGeneration)
	if _, ok := f.a.postGameSnapshot(f.client, "WaitingForStats", 0); !ok {
		t.Fatal("old timer cleared replacement generation")
	}
}

func TestR210LiveScopeResetLeftEndDiagnosticAccepted(t *testing.T) {
	a := r175App(t)
	w := httptest.NewRecorder()
	a.handleClientDiagnostic(w, httptest.NewRequest(http.MethodPost, "/api/diagnostics/client", strings.NewReader(`{"event":"live_scope_reset","reason":"left_end_of_game","source":"sse","previousGameId":9014822625,"gameId":0,"previousPhase":"EndOfGame","phase":"Lobby","clearedRecommendations":1}`)))
	if w.Code != http.StatusNoContent {
		t.Fatal(w.Code, w.Body.String())
	}
	rows := r175Events(t, a, "live_scope_reset")
	if len(rows) != 1 || rows[0]["reason"] != "left_end_of_game" || rows[0]["previous_game_id"] != float64(9014822625) || rows[0]["phase"] != "Lobby" {
		t.Fatal(rows)
	}
}
