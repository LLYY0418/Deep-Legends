package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestR229ClientHistoryUsesPublicIdentityAndHighlightsSelf(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-r229-fixture")
	local, public := strings.Repeat("l", 48), strings.Repeat("p", 48)
	var accounts atomic.Int32
	var bad atomic.Int32
	champs := newChampionProvider()
	champs.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		path := r.URL.Path
		if strings.Contains(path, local) {
			bad.Add(1)
			return &http.Response{StatusCode: 400, Body: http.NoBody, Header: http.Header{}}, nil
		}
		if strings.Contains(path, "/by-riot-id/") {
			accounts.Add(1)
			if r.URL.Host != "asia.api.riotgames.com" {
				t.Error(r.URL.Host)
			}
			return proHTTPBody([]byte(`{"puuid":"` + public + `","gameName":"Fixture","tagLine":"JP1"}`)), nil
		}
		if strings.HasSuffix(path, "/ids") {
			return proHTTPBody([]byte(`["JP1_229"]`)), nil
		}
		if strings.Contains(path, "/matches/") {
			return proHTTPBody([]byte(`{"metadata":{"matchId":"JP1_229"},"info":{"gameId":229,"queueId":420,"gameDuration":1800,"gameType":"MATCHED_GAME","participants":[{"participantId":1,"teamId":100,"puuid":"` + public + `","riotIdGameName":"Fixture","riotIdTagline":"JP1","championId":1,"kills":4,"win":true}]}}`)), nil
		}
		if strings.Contains(path, "league/") {
			return proHTTPBody([]byte(`[{"queueType":"RANKED_SOLO_5x5","tier":"GOLD","rank":"I","leaguePoints":20,"wins":10,"losses":10}]`)), nil
		}
		if strings.Contains(path, "champion-masteries") {
			return proHTTPBody([]byte(`[{"championId":1,"championPoints":1234}]`)), nil
		}
		t.Error("unexpected public path", path)
		return proHTTPBody([]byte(`{}`)), nil
	})}
	a := &app{riot: newRiotProvider(champs), summoner: Summoner{PUUID: local, GameName: "Fixture", TagLine: "JP1"}}
	ref := gameplayReferenceFromSummoner(a.summoner)
	client := newLCUClient(1, "fixture")
	client.region = "JP"
	client.rsoPlatform = "JP1"
	client.http = &http.Client{Transport: gameplayRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("successful Riot history requested LCU")
		return r206RelayResponse(404, []byte(`{}`)), nil
	})}
	a.lcu = client
	clientRef := ref
	clientRef.Region = "jp1"
	a.registerGameplayReferenceDetails(clientRef)
	if _, known := a.riotReferenceForPlayer(local); known {
		t.Fatal("client PUUID marked public")
	}
	for i := 0; i < 2; i++ {
		matches, caps, _ := a.loadDetailedMatches(t.Context(), client, ref, local, false, 0, 10, "all", nil, nil)
		var err error
		if len(caps) == 0 || len(caps[0].Attempts) != 1 || caps[0].Attempts[0].Source != dataSourceRiot {
			t.Fatal(caps)
		}
		if err != nil || len(matches) != 1 || matches[0].SubjectParticipantID != 1 || matches[0].Participants[0].PlayerRef != local || matches[0].Participants[0].Kills != 4 {
			t.Fatal(matches, err)
		}
	}
	if accounts.Load() != 1 || bad.Load() != 0 {
		t.Fatal("identity requests", accounts.Load(), bad.Load())
	}
	c := &a.clientRiotIdentities
	c.mu.Lock()
	entry := c.entries["jp1\x00"+local]
	c.mu.Unlock()
	if d := time.Until(entry.until); d < 5*time.Hour+59*time.Minute || d > 6*time.Hour {
		t.Fatal("TTL", d)
	}
	resolved, err := a.resolveClientRiotPUUID(t.Context(), nil, "jp1", local, ref)
	if err != nil || resolved != public {
		t.Fatal(resolved, err)
	}
	ranks, capability := a.riot.forPlatform("jp1").loadRiotRanks(t.Context(), resolved)
	if len(ranks) != 1 || capability.State != capabilityAvailable {
		t.Fatal(ranks, capability)
	}
	mastery, err := a.riot.forPlatform("jp1").clientMasteries(t.Context(), resolved)
	if err != nil || len(mastery) != 1 || mastery[0].ChampionPoints != 1234 {
		t.Fatal(mastery, err)
	}
	ranks, _, capability = a.loadRanksWithFallback(t.Context(), client, local, false, "jp1", "")
	if len(ranks) != 1 || capability.State != capabilityAvailable || bad.Load() != 0 || accounts.Load() != 1 {
		t.Fatal("legacy rank path bypassed identity conversion", ranks, capability, bad.Load(), accounts.Load())
	}
	a.clearGameplayReferences()
	if len(c.entries) != 0 {
		t.Fatal("switch retained identity")
	}
}
func TestR229IdentityFlightCannotRepopulateAfterAccountSwitch(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-r229-fixture")
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	ch := newChampionProvider()
	ch.client = &http.Client{Transport: gameplayRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		close(started)
		<-release
		return proHTTPBody([]byte(`{"puuid":"public-fixture"}`)), nil
	})}
	a := &app{riot: newRiotProvider(ch)}
	ref := gameplayReference{GameName: "Fixture", TagLine: "JP1"}
	var wg sync.WaitGroup
	wg.Add(1)
	var err error
	go func() {
		defer wg.Done()
		_, err = a.resolveClientRiotPUUID(context.Background(), nil, "jp1", "local-fixture", ref)
	}()
	<-started
	waitCtx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, waitErr := a.resolveClientRiotPUUID(waitCtx, nil, "jp1", "local-fixture", ref); !errors.Is(waitErr, context.Canceled) {
		t.Fatal(waitErr)
	}
	a.clearClientRiotIdentities()
	close(release)
	wg.Wait()
	if !errors.Is(err, context.Canceled) || len(a.clientRiotIdentities.entries) != 0 || calls.Load() != 1 {
		t.Fatal(err, calls.Load())
	}
}
func TestR229MissingRiotIDFallsBackWithoutPublicRequest(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-r229-fixture")
	client := newLCUClient(1, "fixture")
	client.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/matches") {
			return proHTTPBody([]byte(`{"games":{"games":[]}}`)), nil
		}
		return r206RelayResponse(404, []byte(`{}`)), nil
	})}
	client.rsoPlatform = "JP1"
	ch := newChampionProvider()
	ch.client = &http.Client{Transport: gameplayRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("sent client identity to public API")
		return nil, errors.New("unexpected")
	})}
	a := &app{riot: newRiotProvider(ch), lcu: client, summoner: Summoner{PUUID: strings.Repeat("l", 48)}}
	_, caps, _ := a.loadDetailedMatches(t.Context(), client, gameplayReference{}, a.summoner.PUUID, true, 0, 20, "all", nil, nil)
	if len(caps) == 0 || len(caps[0].Attempts) != 1 || caps[0].Attempts[0].Source != dataSourceLCU {
		t.Fatal(caps)
	}
}
func TestR229Decrypt400IsAnEnumAndDoesNotRetainBody(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-r229-fixture")
	for _, body := range []string{`{"message":"Exception decrypting private-account"}`, `{"message":"different private-account"}`} {
		ch := newChampionProvider()
		ch.client = &http.Client{Transport: gameplayRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 400, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}
		p := newRiotProvider(ch)
		var out any
		err := p.get(t.Context(), "asia.api.riotgames.com", "/fixture", nil, &out)
		want := "riot-http-400"
		if strings.Contains(body, "decrypt") {
			want = "riot-puuid-mismatch"
		}
		if riotHistoryFailureCategory(err) != want || strings.Contains(err.Error(), "private") {
			t.Fatal(err, want)
		}
	}
}
func TestR229WeGameRegistryShortcutAndCancel(t *testing.T) {
	root := t.TempDir()
	var keys []string
	roots := wegameRegistryRoots(func(key, value string) []string {
		keys = append(keys, key)
		if value != "InstallPath" {
			t.Fatal(value)
		}
		return []string{root}
	})
	if len(keys) != 2 || len(roots) != 1 {
		t.Fatal(keys, roots)
	}
	path := filepath.Join(root, "WeGame.exe")
	items := wegameRegistryInstallations(roots, func(p string) bool { return p == path })
	shortcut := filepath.Join(root, "WeGame.lnk")
	items = append(items, clientInstallation{ID: "wegame", Name: "WeGame", Kind: "wegame", shortcut: shortcut})
	got := buildDetectedClientInstallations(nil, nil, items, func(string) bool { return true })
	if len(got) != 1 || got[0].ID != "wegame" || len(got[0].candidates()) != 2 || got[0].candidates()[0].Source != "registry" || got[0].candidates()[1].Source != "shortcut" || len(got[0].candidates()[0].arguments) != 0 {
		t.Fatal(got)
	}
	id, _, _, _ := classifyClientShortcut("WeGame.lnk")
	if id != "wegame" {
		t.Fatal(id)
	}
	calls := 0
	r, err := launchClientCandidates(got[0], func(clientLaunchCandidate) (clientLaunchFailure, error) {
		calls++
		return clientLaunchFailure{ErrorCode: 1223}, errors.New("cancel")
	})
	if err != nil || !r.Cancelled || calls != 1 {
		t.Fatal(r, err, calls)
	}
}
func TestR229FastDiscoveryWindowAndNumericMilestones(t *testing.T) {
	now := time.Now()
	a := &app{storage: &localStore{root: t.TempDir()}}
	if err := os.MkdirAll(filepath.Join(a.storage.root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.storage.Close() })
	normal := 8 * time.Second
	if got := a.clientDiscoveryInterval(normal, LCUDiscoveryStatus{Result: "process-not-found"}, now); got != normal {
		t.Fatal(got)
	}
	a.startClientLaunchTiming("wegame", now)
	for _, seconds := range []int{0, 119, 120, 121} {
		want := time.Second
		if seconds >= 120 {
			want = normal
		}
		if got := a.clientDiscoveryInterval(normal, LCUDiscoveryStatus{Result: "process-not-found"}, now.Add(time.Duration(seconds)*time.Second)); got != want {
			t.Fatal(seconds, got)
		}
	}
	a.startClientLaunchTiming("wegame", now.Add(-10*time.Second))
	a.observeClientLaunchDiscovery(LCUDiscoveryStatus{ProcessCount: 1}, now.Add(-9*time.Second))
	a.observeClientLaunchDiscovery(LCUDiscoveryStatus{Result: "connected", ProcessCount: 1}, now.Add(-7*time.Second))
	if got := a.clientDiscoveryInterval(normal, LCUDiscoveryStatus{Result: "connected"}, now.Add(4*time.Second)); got != normal {
		t.Fatal(got)
	}
	a.clientLaunchTiming.mu.Lock()
	event := a.clientLaunchTiming.eventLocked()
	a.clientLaunchTiming.mu.Unlock()
	if event["ms_to_process"] != int64(1000) || event["ms_to_connected"] != int64(3000) || event["ms_to_overview"] != int64(-1) {
		t.Fatal(event)
	}
	event["puuid"] = "private"
	safe := allowClientLaunchTimingDiagnostic(event)
	encoded, _ := json.Marshal(safe)
	if len(safe) != 5 || strings.Contains(string(encoded), "private") {
		t.Fatal(safe)
	}
	a.connected = true
	w := httptest.NewRecorder()
	a.handleClientLaunchOverviewReady(w, httptest.NewRequest("POST", "/api/client-launch-overview-ready", nil))
	if w.Code != 200 || a.clientLaunchTiming.overview.IsZero() {
		t.Fatal(w.Code)
	}
	a.storage.diagnosticMu.Lock()
	flushErr := a.storage.flushDiagnosticLocked()
	a.storage.diagnosticMu.Unlock()
	if flushErr != nil {
		t.Fatal(flushErr)
	}
	bytes, readErr := os.ReadFile(filepath.Join(a.storage.root, "logs", "diagnostics.jsonl"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if strings.Contains(string(bytes), "private") || !strings.Contains(string(bytes), "ms_to_overview") {
		t.Fatal("timing log", string(bytes))
	}
	var count int
	for _, line := range strings.Split(strings.TrimSpace(string(bytes)), "\n") {
		var record map[string]any
		if json.Unmarshal([]byte(line), &record) == nil && record["event"] == "client_launch_to_connected" {
			count++
			if record["client_id"] != "wegame" {
				t.Fatal(record)
			}
			for _, k := range []string{"ms_to_process", "ms_to_connected", "ms_to_overview"} {
				if _, ok := record[k].(float64); !ok {
					t.Fatal(k, record)
				}
			}
		}
	}
	if count != 3 {
		t.Fatal("milestones logged", count)
	}

	gone := &app{}
	gone.clientDiscoveryInterval(normal, LCUDiscoveryStatus{Result: "probe-failed"}, now)
	if gone.clientDiscoveryInterval(normal, LCUDiscoveryStatus{Result: "process-not-found"}, now.Add(time.Second)) != normal {
		t.Fatal("passive client exit sped up idle polling")
	}
	cold := &app{}
	probe := LCUDiscoveryStatus{Result: "probe-failed"}
	if cold.clientDiscoveryInterval(normal, probe, now) != time.Second || cold.clientDiscoveryInterval(normal, probe, now.Add(121*time.Second)) != normal {
		t.Fatal("cold window")
	}
}
