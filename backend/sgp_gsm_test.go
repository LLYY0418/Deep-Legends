package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const gsmHiddenRef = "11111111-2222-3333-4444-555555555555"

func gsmFixtureResponse() sgpGSMResponse {
	var response sgpGSMResponse
	response.Game.ID, response.Game.QueueID, response.Game.MapID, response.Game.GameMode, response.Game.GameState = 9025638917, 420, 11, "CLASSIC", "IN_PROGRESS"
	for i := 0; i < 10; i++ {
		p := lcuLivePlayer{PUUID: r161Ref(i), ChampionID: 1, SelectedPosition: []string{"TOP", "JUNGLE", "MIDDLE", "BOTTOM", "UTILITY"}[i%5], TeamParticipantID: int64(i + 1)}
		if i == 2 {
			p.PUUID, p.ChampionID, p.SelectedPosition = gsmHiddenRef, 117, "UTILITY"
		}
		if i < 5 {
			response.Game.TeamOne = append(response.Game.TeamOne, p)
		} else {
			response.Game.TeamTwo = append(response.Game.TeamTwo, p)
		}
	}
	response.Game.Selections = append(response.Game.Selections, struct {
		PUUID      string `json:"puuid"`
		ChampionID int64  `json:"championId"`
		Spell1ID   int64  `json:"spell1Id"`
		Spell2ID   int64  `json:"spell2Id"`
	}{gsmHiddenRef, 117, 4, 14})
	return response
}

func TestGSMValidationAndCurrentGameMerge(t *testing.T) {
	for _, tc := range []struct {
		name, reason string
		change       func(*sgpGSMResponse)
	}{
		{"valid", "verified", func(*sgpGSMResponse) {}},
		{"wrong-game", "game-mismatch", func(r *sgpGSMResponse) { r.Game.ID++ }},
		{"wrong-queue", "queue-mismatch", func(r *sgpGSMResponse) { r.Game.QueueID = 440 }},
		{"ended", "game-ended", func(r *sgpGSMResponse) { r.Game.GameState = "TERMINATED" }},
		{"no-self", "current-player-missing", func(r *sgpGSMResponse) { r.Game.TeamTwo = r.Game.TeamTwo[:1] }},
		{"duplicate", "identity-invalid", func(r *sgpGSMResponse) { r.Game.TeamTwo[0].PUUID = r.Game.TeamOne[0].PUUID }},
		{"zero", "identity-invalid", func(r *sgpGSMResponse) { r.Game.TeamOne[2].PUUID = emptyLCUPlayerPUUID }},
		{"overfilled", "roster-invalid", func(r *sgpGSMResponse) { r.Game.TeamOne = append(r.Game.TeamOne, lcuLivePlayer{}) }},
		{"selection-conflict", "champion-conflict", func(r *sgpGSMResponse) { r.Game.Selections[0].ChampionID = 99 }},
		{"unknown-selection", "selection-invalid", func(r *sgpGSMResponse) { r.Game.Selections[0].PUUID = r161Ref(15) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gsmFixtureResponse()
			tc.change(&r)
			got := validateGSMRoster(r, r161Ref(6), 9025638917, 420)
			if got.Reason != tc.reason {
				t.Fatalf("%+v", got)
			}
			if tc.reason != "verified" && (got.State != capabilityFailed || len(got.Players) > 0) {
				t.Fatal("invalid roster was accepted")
			}
			if tc.reason == "verified" {
				p := got.Players[2].player
				if p.PUUID != gsmHiddenRef || p.ObfuscatedPUUID != "" || p.ChampionID != 117 || p.Spell1ID != 4 || p.Spell2ID != 14 {
					t.Fatal(p)
				}
			}
		})
	}
	verified := validateGSMRoster(gsmFixtureResponse(), r161Ref(6), 9025638917, 420).Players
	complete := append([]liveRosterEntry(nil), verified...)
	for i := range complete {
		complete[i].player.ChampionID = 0
	}
	if liveGSMRosterNeeded(complete, 420) {
		t.Fatal("complete identity roster should not request GSM for missing champions")
	}
	complete[2].player.PUUID = ""
	if !liveGSMRosterNeeded(complete, 420) {
		t.Fatal("anonymous roster still needs an identity supplement")
	}
	for _, kind := range []string{"missing", "anonymous", "team-conflict", "champion-conflict", "complete"} {
		t.Run(kind, func(t *testing.T) {
			raw := append([]liveRosterEntry(nil), verified...)
			want := 0
			switch kind {
			case "missing":
				raw = append(raw[:2], raw[3:]...)
				want = 1
			case "anonymous":
				raw[2].player.PUUID = ""
				raw[2].player.NameVisibilityType = "HIDDEN"
			case "team-conflict":
				raw[0].team = 200
			case "champion-conflict":
				raw[2].player.ChampionID = 99
			}
			got, appended, restored, reason := mergeGSMLiveRoster(raw, verified)
			if strings.Contains(kind, "conflict") {
				if reason != "roster-conflict" || appended != 0 || restored != 0 || len(got) != len(raw) {
					t.Fatal(got, reason)
				}
				return
			}
			if len(got) != 10 || appended != want || reason != "merged" {
				t.Fatal(got, appended, restored, reason)
			}
			found := false
			for _, r := range got {
				if r.player.PUUID == gsmHiddenRef {
					found = true
				}
			}
			if !found {
				t.Fatal("Lulu identity not restored")
			}
			if kind == "complete" && restored != 0 {
				t.Fatal("existing roster duplicated")
			}
		})
	}
}

func TestGSMRoutesCacheFailuresAndScopes(t *testing.T) {
	for _, status := range []int{200, 404, 429, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			client := &LCUClient{}
			p := newSGPProvider()
			p.sessionOwner, p.sessionToken, p.sessionAt = client, "test-league-token", time.Now()
			p.retryWait = func(context.Context, time.Duration) error { return nil }
			var requests atomic.Int64
			var observationsMu sync.Mutex
			var observations []map[string]any
			p.observe = func(e map[string]any) {
				observationsMu.Lock()
				defer observationsMu.Unlock()
				observations = append(observations, e)
			}
			p.http = &http.Client{Transport: sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				requests.Add(1)
				if r.Method != "GET" || r.URL.Host != "hn1-k8s-sgp.lol.qq.com:21019" || r.URL.Path != "/gsm/v1/ledge/region/HN1/puuid/"+r161Ref(6) || r.Header.Get("Authorization") != "Bearer test-league-token" {
					t.Errorf("wrong GSM route or auth")
				}
				data, _ := json.Marshal(gsmFixtureResponse())
				if status == 200 {
					data = []byte(strings.TrimSuffix(string(data), "}") + `,"playerCredentials":{"encryptionKey":"secret-key","serverIp":"secret-ip"}}`)
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(data))), Header: make(http.Header)}, nil
			})}
			got := p.liveGameRoster(context.Background(), client, "HN1", r161Ref(6), 9025638917, 420)
			want := capabilityFailed
			if status == 200 {
				want = capabilityAvailable
			}
			if status == 404 {
				want = capabilityUnsupported
			}
			if got.State != want {
				t.Fatal(got)
			}
			before := requests.Load()
			cached := p.liveGameRoster(context.Background(), client, "HN1", r161Ref(6), 9025638917, 420)
			if !cached.Cached || cached.State != want || requests.Load() != before {
				t.Fatal("second read did not respect cache/negative backoff")
			}
			if len(cached.Players) > 0 {
				cached.Players[0].player.PUUID = "changed"
				if p.liveGameRoster(context.Background(), client, "HN1", r161Ref(6), 9025638917, 420).Players[0].player.PUUID == "changed" {
					t.Fatal("cache was mutated")
				}
			}
			observationsMu.Lock()
			encoded, _ := json.Marshal(observations)
			observationsMu.Unlock()
			for _, secret := range []string{r161Ref(6), gsmHiddenRef, "test-league-token", "secret-key", "secret-ip"} {
				if strings.Contains(string(encoded), secret) {
					t.Fatal("diagnostic identity/credential leak")
				}
			}
			key := sgpGSMKey{client, "HN1", r161Ref(6), 9025638917, 420}
			p.gsm.mu.Lock()
			entry := p.gsm.entries[key]
			entry.At = time.Now().Add(-time.Minute)
			p.gsm.entries[key] = entry
			p.gsm.mu.Unlock()
			p.liveGameRoster(context.Background(), client, "HN1", r161Ref(6), 9025638917, 420)
			if requests.Load() == before {
				t.Fatal("expired backoff was permanent")
			}
			before = requests.Load()
			p.liveGameRoster(context.Background(), client, "HN1", r161Ref(6), 9025638918, 420)
			if requests.Load() == before {
				t.Fatal("new game reused old cached roster")
			}
		})
	}
}

func TestGSMConcurrentFlightAndCancellation(t *testing.T) {
	p := newSGPProvider()
	client := &LCUClient{}
	p.sessionOwner, p.sessionToken, p.sessionAt = client, "test-token", time.Now()
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int64
	p.http = &http.Client{Transport: sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		data, _ := json.Marshal(gsmFixtureResponse())
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data))), Header: make(http.Header)}, nil
	})}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got := p.liveGameRoster(context.Background(), client, "HN1", r161Ref(6), 9025638917, 420); got.State != capabilityAvailable {
				t.Errorf("%+v", got)
			}
		}()
	}
	<-entered
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("GSM singleflight failed", calls.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := p.liveGameRoster(ctx, client, "HN1", r161Ref(6), 9025638918, 420)
	if got.State != capabilityCanceled {
		t.Fatal(got)
	}
	p.gsm.mu.Lock()
	n := len(p.gsm.entries)
	p.gsm.mu.Unlock()
	if n != 1 {
		t.Fatal("cancellation cached as empty")
	}
}

func TestGSMRefreshFailureRetainsVerifiedRosterTTL(t *testing.T) {
	client := &LCUClient{}
	p := newSGPProvider()
	p.sessionOwner, p.sessionToken, p.sessionAt = client, "test-token", time.Now()
	p.retryWait = func(context.Context, time.Duration) error { return nil }
	status := 200
	var calls atomic.Int64
	p.http = &http.Client{Transport: sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		data, _ := json.Marshal(gsmFixtureResponse())
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(data))), Header: make(http.Header)}, nil
	})}
	key := sgpGSMKey{client, "HN1", r161Ref(6), 9025638917, 420}
	if got := p.liveGameRoster(context.Background(), client, key.Server, key.Player, key.GameID, key.QueueID); got.State != capabilityAvailable {
		t.Fatal(got)
	}
	p.gsm.mu.Lock()
	entry := p.gsm.entries[key]
	entry.At = time.Now().Add(-time.Minute)
	p.gsm.entries[key] = entry
	p.gsm.mu.Unlock()
	status = 503
	got := p.liveGameRoster(context.Background(), client, key.Server, key.Player, key.GameID, key.QueueID)
	if got.State != capabilityAvailable || !got.Stale || len(got.Players) != 10 {
		t.Fatal("verified roster was erased by failure", got)
	}
	p.gsm.mu.Lock()
	retained := p.gsm.entries[key]
	p.gsm.mu.Unlock()
	if !retained.At.Equal(entry.At) || retained.TTL != entry.TTL {
		t.Fatal("failure refreshed successful TTL")
	}
	before := calls.Load()
	if got := p.liveGameRoster(context.Background(), client, key.Server, key.Player, key.GameID, key.QueueID); !got.Stale || calls.Load() != before {
		t.Fatal("stale retry backoff failed")
	}
}

func TestGSMCacheSeparatesClientAccountAndServer(t *testing.T) {
	client := &LCUClient{}
	p := newSGPProvider()
	p.sessionOwner, p.sessionToken, p.sessionAt = client, "test-token", time.Now()
	var calls atomic.Int64
	p.http = &http.Client{Transport: sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		data, _ := json.Marshal(gsmFixtureResponse())
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data))), Header: make(http.Header)}, nil
	})}
	for _, account := range []string{r161Ref(6), r161Ref(7)} {
		for _, server := range []string{"HN1", "HN10"} {
			got := p.liveGameRoster(context.Background(), client, server, account, 9025638917, 420)
			if got.State != capabilityAvailable || got.Cached {
				t.Fatal("account/server inherited an unrelated roster")
			}
		}
	}
	client = &LCUClient{}
	p.sessionOwner, p.sessionToken, p.sessionAt = client, "test-token", time.Now()
	if got := p.liveGameRoster(context.Background(), client, "HN1", r161Ref(6), 9025638917, 420); got.Cached || got.State != capabilityAvailable {
		t.Fatal("new client inherited roster")
	}
	if calls.Load() != 5 {
		t.Fatal(calls.Load())
	}
}

func TestGSMRefreshesLeagueTokenAndNeverForwardsRedirect(t *testing.T) {
	client := &LCUClient{baseURL: "http://127.0.0.1", token: "lcu-secret"}
	var tokenReads atomic.Int64
	client.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/lol-league-session/v1/league-session-token" {
			t.Error("wrong token source")
		}
		tokenReads.Add(1)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`"fresh-league-token"`)), Header: make(http.Header)}, nil
	})}
	p := newSGPProvider()
	p.sessionOwner, p.sessionToken, p.sessionAt = client, "expired-league-token", time.Now()
	var calls atomic.Int64
	p.http = &http.Client{Transport: sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if calls.Load() == 1 {
			return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
		}
		if r.Header.Get("Authorization") != "Bearer fresh-league-token" {
			t.Error("league token was not refreshed")
		}
		data, _ := json.Marshal(gsmFixtureResponse())
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data))), Header: make(http.Header)}, nil
	})}
	if got := p.liveGameRoster(context.Background(), client, "HN1", r161Ref(6), 9025638917, 420); got.State != capabilityAvailable || calls.Load() != 2 || tokenReads.Load() != 1 {
		t.Fatal(got, calls.Load(), tokenReads.Load())
	}
	p.http = &http.Client{Transport: sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "hn1-k8s-sgp.lol.qq.com:21019" {
			t.Error("credential sent to redirect target")
		}
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://example.com/credentials"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	if got := p.liveGameRoster(context.Background(), client, "HN1", r161Ref(6), 9025638918, 420); got.State != capabilityFailed {
		t.Fatal("redirect accepted", got)
	}
}

func TestGSMLuluEndToEndWithoutChampSelect(t *testing.T) {
	for _, red := range []bool{false, true} {
		for _, placeholder := range []bool{false, true} {
			t.Run(fmt.Sprintf("red=%v/placeholder=%v", red, placeholder), func(t *testing.T) {
				f := newR183Fixture(t)
				f.current = Summoner{PUUID: r161Ref(0), GameName: "Player0", TagLine: "CN1"}
				f.app.allSkins = []Skin{{ChampionID: 117, ChampionName: "璐璐"}, {ChampionID: 1, ChampionName: "安妮"}}
				gsm := gsmFixtureResponse()
				if red {
					gsm.Game.TeamOne, gsm.Game.TeamTwo = gsm.Game.TeamTwo, gsm.Game.TeamOne
					for i := range f.raw {
						f.raw[i].team = 300 - f.raw[i].team
					}
				}
				if placeholder {
					team := int64(100)
					if red {
						team = 200
					}
					f.raw = append(f.raw, liveRosterEntry{lcuLivePlayer{ChampionID: 117, NameVisibilityType: "HIDDEN"}, team})
				}
				base := f.client.http.Transport
				var game lcuGame
				game.GameID, game.GameCreation, game.GameDuration, game.QueueID, game.GameMode = 123, 1786300000000, 1800, 420, "CLASSIC"
				identity := lcuParticipantIdentity{ParticipantID: 1}
				identity.Player.PUUID = gsmHiddenRef
				participant := lcuParticipant{ParticipantID: 1, TeamID: 100, ChampionID: 117}
				participant.Stats.Kills, participant.Stats.Deaths, participant.Stats.Assists, participant.Stats.Win = 1, 5, 29, true
				game.ParticipantIdentities, game.Participants = []lcuParticipantIdentity{identity}, []lcuParticipant{participant}
				var history lcuMatchHistory
				history.Games.GameCount, history.Games.Games = 1, []lcuGame{game}
				var requestsMu *sync.Mutex = &f.mu
				f.client.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
					var value any
					switch r.URL.Path {
					case "/lol-gameflow/v1/session":
						var flow lcuGameflowSession
						flow.GameData.GameID, flow.GameData.Queue.ID, flow.GameData.Queue.GameMode = 9025638917, 420, "CLASSIC"
						for _, row := range f.raw {
							if row.team == 100 {
								flow.GameData.TeamOne = append(flow.GameData.TeamOne, row.player)
							} else {
								flow.GameData.TeamTwo = append(flow.GameData.TeamTwo, row.player)
							}
						}
						value = flow
					case "/lol-summoner/v2/summoners/puuid/" + gsmHiddenRef:
						value = Summoner{PUUID: gsmHiddenRef, GameName: "RecoveredLulu", TagLine: "26732"}
					case "/lol-ranked/v1/ranked-stats/" + gsmHiddenRef:
						value = map[string]any{"queues": []any{map[string]any{"queueType": "RANKED_SOLO_5x5", "tier": "PLATINUM", "division": "II", "wins": 28, "losses": 21}}}
					case "/lol-match-history/v1/products/lol/" + gsmHiddenRef + "/matches":
						value = history
					default:
						return base.RoundTrip(r)
					}
					requestsMu.Lock()
					f.requests[r.URL.Path]++
					requestsMu.Unlock()
					data, _ := json.Marshal(value)
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data))), Header: make(http.Header)}, nil
				})
				p := newSGPProvider()
				f.app.sgp = p
				p.sessionOwner, p.sessionToken, p.sessionAt = f.client, "test-token", time.Now()
				p.observe = f.app.recordDiagnostic
				var gsmCalls atomic.Int64
				p.http = &http.Client{Transport: sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
					status := 404
					body := []byte(`{}`)
					if strings.HasPrefix(r.URL.Path, "/gsm/") {
						if !strings.HasSuffix(r.URL.Path, "/puuid/"+f.current.PUUID) {
							t.Error("GSM queried another player")
						}
						gsmCalls.Add(1)
						status = 200
						body, _ = json.Marshal(gsm)
					}
					return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
				})}
				f.app.liveClientPlayerList = func(context.Context) ([]byte, int, error) { return []byte(`[]`), 200, nil }
				for attempt := 0; attempt < 2; attempt++ {
					response := f.app.loadGameplayLive(context.Background(), f.client, f.current, "InProgress")
					if len(response.Players) != 10 {
						t.Fatal("GSM did not recover ten players", response)
					}
					found := false
					ownTeam := int64(0)
					for _, player := range response.Players {
						if player.IsCurrent {
							ownTeam = player.TeamID
						}
					}
					for _, player := range response.Players {
						if player.reference.PlayerRef != gsmHiddenRef {
							continue
						}
						found = true
						team := int64(100)
						if red {
							team = 200
						}
						if player.Hidden || ownTeam != team || player.TeamID != team || player.GameName != "RecoveredLulu" || player.TagLine != "26732" || player.Rank == nil || len(player.RecentGames) != 1 || player.Spell1ID != 4 || player.Spell2ID != 14 || player.HistoryState != "ok" {
							t.Fatalf("Lulu %+v", player)
						}
					}
					if !found {
						t.Fatal("Lulu not queryable")
					}
					encoded, _ := json.Marshal(response)
					if strings.Contains(string(encoded), gsmHiddenRef) {
						t.Fatal("renderer leaked stable PUUID")
					}
				}
				if gsmCalls.Load() != 1 {
					t.Fatal("GSM reread amplified requests")
				}
			})
		}
	}
}
