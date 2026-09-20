package main

// Public tournament data is isolated from the Riot/account providers. The IDs
// below were rechecked against getLeagues/getTeams on 2026-09-09 (R75).
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var proRuneLeagues = map[string]string{
	"98767991314006698": "LPL", "98767991310872058": "LCK",
	"113464388705111224": "First Stand", "98767991325878492": "MSI", "98767975604431411": "Worlds",
}
var proRuneTeams = map[string]string{
	"99566404853854212": "BLG", "99566404848691211": "IG", "98767991853197861": "T1",
	"100205573496804586": "HLE", "100205573495116443": "GEN", "100725845018863243": "DK",
}

const proRuneAPIKey = "0TvQnueqKa5mxJntVWt0w4LpLfEkrV1Ta8rQBb9Z" // published website key, NOT a Riot developer key
const proRuneTTL = 5 * time.Minute
const proRuneBackgroundWorkers = 3

type proEventTeam struct {
	ID     string `json:"id"`
	Code   string `json:"code"`
	Result struct {
		GameWins int `json:"gameWins"`
	} `json:"result"`
}
type proEventGame struct {
	ID     string `json:"id"`
	Number int    `json:"number"`
	State  string `json:"state"`
} // deliberately no side field
type proRuneEvent struct {
	ID     string `json:"id"`
	League struct {
		ID string `json:"id"`
	} `json:"league"`
	Match struct {
		Teams []proEventTeam `json:"teams"`
		Games []proEventGame `json:"games"`
	} `json:"match"`
}
type proParticipant struct {
	ParticipantID int    `json:"participantId"`
	PlayerID      string `json:"esportsPlayerId"`
	SummonerName  string `json:"summonerName"`
	ChampionID    string `json:"championId"`
	Role          string `json:"role"`
}
type proTeamMetadata struct {
	TeamID       string           `json:"esportsTeamId"`
	Participants []proParticipant `json:"participantMetadata"`
}
type proGameMetadata struct {
	Patch string          `json:"patchVersion"`
	Blue  proTeamMetadata `json:"blueTeamMetadata"`
	Red   proTeamMetadata `json:"redTeamMetadata"`
}
type proFrameTeam struct {
	Inhibitors int `json:"inhibitors"`
	TotalGold  int `json:"totalGold"`
}
type proFrame struct {
	Timestamp time.Time    `json:"rfc460Timestamp"`
	State     string       `json:"gameState"`
	Blue      proFrameTeam `json:"blueTeam"`
	Red       proFrameTeam `json:"redTeam"`
}
type proWindow struct {
	Metadata proGameMetadata `json:"gameMetadata"`
	Frames   []proFrame      `json:"frames"`
}
type proPerks struct {
	StyleID    int64   `json:"styleId"`
	SubStyleID int64   `json:"subStyleId"`
	Perks      []int64 `json:"perks"`
}
type proDetailParticipant struct {
	ID    int      `json:"participantId"`
	Items []int64  `json:"items"`
	Perks proPerks `json:"perkMetadata"`
}
type proDetails struct {
	Frames []struct {
		Timestamp    time.Time              `json:"rfc460Timestamp"`
		Participants []proDetailParticipant `json:"participants"`
	} `json:"frames"`
}
type proRuneIndexGame struct {
	ID       string
	MatchID  string
	LeagueID string
	Number   int
	Start    time.Time
	Metadata proGameMetadata
}
type proRuneSnapshot struct {
	Games  map[string]proRuneIndexGame
	Events map[string]proRuneEvent
	ReadAt time.Time
}
type proRuneProvider struct {
	provider         *championProvider
	root             string
	notify           func()
	mu               sync.Mutex
	snapshot         proRuneSnapshot
	identities       map[string]string
	ends             map[string]proWindow
	details          map[string]proDetails
	flights          map[string]chan struct{}
	gate             chan struct{}
	preparing        bool
	outcomesLoading  bool
	attempted        time.Time
	reason           string
	failed           int // Request failures for diagnostics, not the public game count.
	failedGameIDs    map[string]bool
	sourceIncomplete bool
	diskMu           sync.Mutex
	cacheWriteFailed bool
}

func newProRuneProvider(provider *championProvider, store *localStore, notify func()) *proRuneProvider {
	p := &proRuneProvider{provider: provider, notify: notify, identities: map[string]string{}, ends: map[string]proWindow{}, details: map[string]proDetails{}, flights: map[string]chan struct{}{}, gate: make(chan struct{}, 4), snapshot: proRuneSnapshot{Games: map[string]proRuneIndexGame{}, Events: map[string]proRuneEvent{}}}
	if store != nil {
		p.root = filepath.Join(store.root, "pro-runes-v1")
	}
	return p
}
func (a *app) proRuneSource() *proRuneProvider {
	a.proRuneMu.Lock()
	defer a.proRuneMu.Unlock()
	if a.proRunes == nil {
		a.proRunes = newProRuneProvider(a.championDataProvider(), a.storage, func() { a.broadcastEvent("pro-runes") })
	}
	return a.proRunes
}
func proNumericID(id string) bool {
	if len(id) == 0 || len(id) > 24 {
		return false
	}
	for _, c := range id {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
func (p *proRuneProvider) readDisk(key string, into any) bool {
	if p.root == "" {
		return false
	}
	f, err := os.Open(filepath.Join(p.root, key+".json"))
	if err != nil {
		return false
	}
	defer f.Close()
	data, err := readLimited(f, 16<<20)
	return err == nil && json.Unmarshal(data, into) == nil
}
func (p *proRuneProvider) writeDisk(key string, value any) {
	if p.root == "" {
		return
	}
	data, err := json.Marshal(value)
	if err != nil || len(data) > 16<<20 {
		return
	}
	p.diskMu.Lock()
	defer p.diskMu.Unlock()
	err = os.MkdirAll(p.root, 0700)
	if err == nil {
		err = atomicWriteFile(filepath.Join(p.root, key+".json"), data, 0600)
	}
	if err != nil {
		p.mu.Lock()
		p.cacheWriteFailed = true
		p.mu.Unlock()
	}
}
func (p *proRuneProvider) fetch(ctx context.Context, feed bool, endpoint string, query url.Values, into any) error {
	host := "esports-api.lolesports.com"
	path := "/persisted/gw/" + endpoint
	if feed {
		host = "feed.lolesports.com"
		path = "/livestats/v1/" + endpoint
	}
	// Only caller-constructed public query fields. Never forward incoming headers/query.
	for key := range query {
		if key != "hl" && key != "leagueId" && key != "id" && key != "pageToken" && key != "startingTime" {
			return errors.New("invalid public query")
		}
	}
	select {
	case p.gate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-p.gate }()
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	client := *p.provider.httpClient()
	client.Jar = nil
	client.Timeout = 8 * time.Second
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return errors.New("esports redirect rejected") }
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+host+path+"?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if !feed {
		req.Header.Set("x-api-key", proRuneAPIKey)
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("esports HTTP %d", res.StatusCode)
	}
	data, err := readLimited(res.Body, 2<<20)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, into)
}
func proStartingTime(t time.Time) string {
	return t.UTC().Truncate(10 * time.Second).Format(time.RFC3339)
}
func proEventAllowed(e proRuneEvent) bool {
	if proRuneLeagues[e.League.ID] == "" || len(e.Match.Teams) != 2 || e.Match.Teams[0].ID == e.Match.Teams[1].ID {
		return false
	}
	for _, t := range e.Match.Teams {
		if proRuneTeams[t.ID] != "" {
			return true
		}
	}
	return false
}
func (p *proRuneProvider) event(ctx context.Context, id string) (proRuneEvent, error) {
	var e proRuneEvent
	if !proNumericID(id) {
		return e, errors.New("invalid match ID")
	}
	if p.readDisk("event-"+id, &e) && e.ID == id {
		return e, nil
	}
	var payload struct {
		Data struct {
			Event proRuneEvent `json:"event"`
		} `json:"data"`
	}
	err := p.fetch(ctx, false, "getEventDetails", url.Values{"hl": {"zh-CN"}, "id": {id}}, &payload)
	e = payload.Data.Event
	if err == nil && e.ID != id {
		err = errors.New("match identity mismatch")
	}
	if err == nil {
		p.writeDisk("event-"+id, e)
	}
	return e, err
}
func (p *proRuneProvider) indexGame(ctx context.Context, e proRuneEvent, g proEventGame) (proRuneIndexGame, error) {
	var row proRuneIndexGame
	if !proNumericID(g.ID) {
		return row, errors.New("invalid game ID")
	}
	validTeams := func(m proGameMetadata) bool {
		return len(e.Match.Teams) == 2 && m.Blue.TeamID != m.Red.TeamID && ((m.Blue.TeamID == e.Match.Teams[0].ID && m.Red.TeamID == e.Match.Teams[1].ID) || (m.Blue.TeamID == e.Match.Teams[1].ID && m.Red.TeamID == e.Match.Teams[0].ID))
	}
	if p.readDisk("index-"+g.ID, &row) && row.ID == g.ID && row.MatchID == e.ID && row.LeagueID == e.League.ID && !row.Start.IsZero() && validTeams(row.Metadata) {
		return row, nil
	}
	var w proWindow
	err := p.fetch(ctx, true, "window/"+g.ID, nil, &w)
	if err != nil {
		return row, err
	}
	if len(w.Frames) == 0 || w.Frames[0].Timestamp.IsZero() {
		return row, errors.New("missing start time")
	}
	teams := map[string]bool{}
	for _, t := range e.Match.Teams {
		teams[t.ID] = true
	}
	if !teams[w.Metadata.Blue.TeamID] || !teams[w.Metadata.Red.TeamID] || w.Metadata.Blue.TeamID == w.Metadata.Red.TeamID {
		return row, errors.New("livestats team mismatch")
	}
	row = proRuneIndexGame{ID: g.ID, MatchID: e.ID, LeagueID: e.League.ID, Number: g.Number, Start: w.Frames[0].Timestamp, Metadata: w.Metadata}
	p.writeDisk("index-"+g.ID, row)
	return row, nil
}
func proParallel[T any](ctx context.Context, values []T, fn func(T)) {
	proParallelLimit(ctx, values, 4, fn)
}

func proParallelLimit[T any](ctx context.Context, values []T, workers int, fn func(T)) {
	var wg sync.WaitGroup
	jobs := make(chan T)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for value := range jobs {
				if ctx.Err() == nil {
					fn(value)
				}
			}
		}()
	}
	for _, value := range values {
		select {
		case jobs <- value:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return
		}
	}
	close(jobs)
	wg.Wait()
}

// kick never performs disk or network I/O on the first-screen request.
func (p *proRuneProvider) kick(force bool) {
	p.mu.Lock()
	now := time.Now()
	if p.preparing || p.outcomesLoading || (!force && !p.attempted.IsZero() && now.Sub(p.attempted) < proRuneTTL) {
		p.mu.Unlock()
		return
	}
	p.preparing = true
	p.attempted = now
	p.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		p.refresh(ctx, now)
		if p.notify != nil {
			p.notify()
		}
		// Outcome validation runs separately after publication, never in the index
		// critical path. Rune/item details remain strictly champion-hit-only.
		p.mu.Lock()
		p.outcomesLoading = true
		games := make([]proRuneIndexGame, 0, len(p.snapshot.Games))
		for _, g := range p.snapshot.Games {
			games = append(games, g)
		}
		p.mu.Unlock()
		endCtx, endCancel := context.WithTimeout(context.Background(), 60*time.Second)
		var resultMu sync.Mutex
		finished := map[string]bool{}
		// Leave one of the four public-request slots available for selected rune details.
		proParallelLimit(endCtx, games, proRuneBackgroundWorkers, func(g proRuneIndexGame) {
			if _, err := p.terminal(endCtx, g); err == nil {
				resultMu.Lock()
				finished[g.ID] = true
				resultMu.Unlock()
			}
		})
		p.mu.Lock()
		if p.failedGameIDs == nil {
			p.failedGameIDs = map[string]bool{}
		}
		for _, g := range games {
			if !finished[g.ID] {
				p.failedGameIDs[g.ID] = true
			}
		}
		if missing := len(games) - len(finished); missing > 0 {
			p.failed += missing
			p.reason = "network-unavailable"
			if endCtx.Err() != nil {
				p.reason = "upstream-timeout"
			}
		}
		p.mu.Unlock()
		endCancel()
		p.mu.Lock()
		p.outcomesLoading = false
		p.mu.Unlock()
		if p.notify != nil {
			p.notify()
		}
	}()
}
func (p *proRuneProvider) refresh(ctx context.Context, now time.Time) {
	cutoff := now.Add(-30 * 24 * time.Hour)
	var old proRuneSnapshot
	p.mu.Lock()
	p.cacheWriteFailed = false
	old = p.snapshot
	p.mu.Unlock()
	if len(old.Games) == 0 {
		p.readDisk("snapshot", &old)
	}
	identities := map[string]string{}
	p.readDisk("identities", &identities)
	p.mu.Lock()
	for id, key := range identities {
		p.identities[id] = key
	}
	p.mu.Unlock()
	next := proRuneSnapshot{Games: map[string]proRuneIndexGame{}, Events: map[string]proRuneEvent{}, ReadAt: old.ReadAt}
	// Preserve partial/offline index without extending its last successful time.
	for id, g := range old.Games {
		if g.Start.After(cutoff) && !g.Start.After(now) && proEventAllowed(old.Events[g.MatchID]) {
			next.Games[id] = g
			next.Events[g.MatchID] = old.Events[g.MatchID]
		}
	}
	p.mu.Lock()
	cached := proRuneSnapshot{Games: map[string]proRuneIndexGame{}, Events: map[string]proRuneEvent{}, ReadAt: next.ReadAt}
	for id, g := range next.Games {
		cached.Games[id] = g
	}
	for id, e := range next.Events {
		cached.Events[id] = e
	}
	p.snapshot = cached
	p.mu.Unlock()
	if len(cached.Games) > 0 && p.notify != nil {
		p.notify()
	}
	var mu sync.Mutex
	failed := 0
	failedGames := map[string]bool{}
	sourceIncomplete := false
	reason := ""
	matches := map[string]bool{}
	ids := make([]string, 0, 5)
	for id := range proRuneLeagues {
		ids = append(ids, id)
	}
	proParallel(ctx, ids, func(leagueID string) {
		token := ""
		seen := map[string]bool{}
		for page := 0; page < 20; page++ {
			q := url.Values{"hl": {"zh-CN"}, "leagueId": {leagueID}}
			if token != "" {
				q.Set("pageToken", token)
			}
			var response struct {
				Data struct {
					Schedule struct {
						Pages struct {
							Older string `json:"older"`
						} `json:"pages"`
						Events []struct {
							Start time.Time `json:"startTime"`
							State string    `json:"state"`
							Match struct {
								ID string `json:"id"`
							} `json:"match"`
						} `json:"events"`
					} `json:"schedule"`
				} `json:"data"`
			}
			if err := p.fetch(ctx, false, "getSchedule", q, &response); err != nil {
				mu.Lock()
				failed++
				sourceIncomplete = true
				reason = proRuneErrorReason(err)
				mu.Unlock()
				return
			}
			s := response.Data.Schedule
			oldest := now
			mu.Lock()
			for _, e := range s.Events {
				if e.Start.Before(oldest) {
					oldest = e.Start
				}
				if e.State == "completed" && !e.Start.After(now) && e.Start.After(cutoff.Add(-24*time.Hour)) && proNumericID(e.Match.ID) {
					matches[e.Match.ID] = true
				}
			}
			mu.Unlock()
			// Include series straddling the UTC window; individual games are filtered below.
			token = s.Pages.Older
			if token == "" || oldest.Before(cutoff.Add(-24*time.Hour)) {
				return
			}
			if seen[token] {
				mu.Lock()
				failed++
				sourceIncomplete = true
				reason = "upstream-error"
				mu.Unlock()
				return
			}
			seen[token] = true
		}
		mu.Lock()
		failed++
		sourceIncomplete = true
		reason = "upstream-error"
		mu.Unlock()
	})
	matchIDs := make([]string, 0, len(matches))
	for id := range matches {
		matchIDs = append(matchIDs, id)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(matchIDs)))
	proParallel(ctx, matchIDs, func(id string) {
		e, err := p.event(ctx, id)
		if err != nil {
			mu.Lock()
			failed++
			sourceIncomplete = true
			reason = proRuneErrorReason(err)
			mu.Unlock()
			return
		}
		if !proEventAllowed(e) {
			return
		}
		mu.Lock()
		next.Events[id] = e
		mu.Unlock()
		for _, g := range e.Match.Games {
			if g.State != "completed" {
				continue
			}
			row, err := p.indexGame(ctx, e, g)
			mu.Lock()
			if err != nil {
				failed++
				failedGames[g.ID] = true
				reason = proRuneErrorReason(err)
			} else if row.Start.After(cutoff) && !row.Start.After(now) {
				next.Games[row.ID] = row
			}
			mu.Unlock()
		}
	})
	if ctx.Err() != nil {
		failed++
		sourceIncomplete = true
		reason = "upstream-timeout"
	}
	// A failed game does not make the successfully refreshed index an old cache.
	if !sourceIncomplete {
		next.ReadAt = now
	}
	p.mu.Lock()
	p.snapshot = next
	p.failed = failed
	p.failedGameIDs = failedGames
	p.sourceIncomplete = sourceIncomplete
	p.reason = reason
	p.preparing = false
	p.outcomesLoading = true
	p.mu.Unlock()
	// Learn reviewed identities at index time, even before any hero is selected.
	p.rows(next, now)
	p.saveIdentities()
	p.mu.Lock()
	for id := range p.ends {
		if _, ok := next.Games[id]; !ok {
			delete(p.ends, id)
		}
	}
	for id := range p.details {
		if _, ok := next.Games[id]; !ok {
			delete(p.details, id)
		}
	}
	p.mu.Unlock()
	p.writeDisk("snapshot", next)
}
func proRuneErrorReason(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "upstream-timeout"
	}
	var timeout interface{ Timeout() bool }
	if errors.As(err, &timeout) && timeout.Timeout() {
		return "upstream-timeout"
	}
	return "network-unavailable"
}

// ID mapping is learned only after a team-ID and exact reviewed-name match.
func (p *proRuneProvider) player(teamID string, part proParticipant) string {
	code := proRuneTeams[teamID]
	if code == "" {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	name := strings.TrimSpace(part.SummonerName)
	if len(name) > len(code) && strings.EqualFold(name[:len(code)], code) {
		name = strings.TrimSpace(name[len(code):])
	}
	known := p.identities[part.PlayerID]
	for _, t := range proRoster {
		if t.Code != code {
			continue
		}
		for _, player := range t.Players {
			key := teamID + "/" + player.Name
			if known != "" {
				if known == key {
					return player.Name
				}
				continue
			}
			if strings.EqualFold(name, player.Name) {
				if proNumericID(part.PlayerID) {
					p.identities[part.PlayerID] = key
				}
				return player.Name
			}
		}
	}
	return ""
}
func (p *proRuneProvider) saveIdentities() {
	p.mu.Lock()
	copy := map[string]string{}
	for k, v := range p.identities {
		copy[k] = v
	}
	p.mu.Unlock()
	p.writeDisk("identities", copy)
}

func (a *app) handleGameplayProRunes(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.URL.Query().Get("championId"))
	if err != nil || id <= 0 || id > 10000 {
		http.Error(w, "推荐英雄无效", 400)
		return
	}
	position, err := normalizeOPGGPosition(r.URL.Query().Get("position"))
	if err != nil {
		http.Error(w, "推荐分路无效", 400)
		return
	}
	p := a.proRuneSource()
	p.kick(r.URL.Query().Get("refresh") == "1")
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	respondJSON(w, p.recommend(ctx, id, position))
}
