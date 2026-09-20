package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// One flight per immutable game/resource; all flights share the four-request
// public-network semaphore, independently of the specialist/Riot limiter.
func (p *proRuneProvider) gameResource(ctx context.Context, key string, load func() error) error {
	for {
		p.mu.Lock()
		flight := p.flights[key]
		if flight == nil {
			flight = make(chan struct{})
			p.flights[key] = flight
			p.mu.Unlock()
			err := load()
			p.mu.Lock()
			delete(p.flights, key)
			close(flight)
			p.mu.Unlock()
			return err
		}
		p.mu.Unlock()
		select {
		case <-flight:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
func (p *proRuneProvider) terminal(ctx context.Context, g proRuneIndexGame) (proWindow, error) {
	var result proWindow
	err := p.gameResource(ctx, "end-"+g.ID, func() error {
		p.mu.Lock()
		cached, ok := p.ends[g.ID]
		p.mu.Unlock()
		if ok {
			result = cached
			return nil
		}
		valid := func(w proWindow) bool {
			if len(w.Frames) == 0 {
				return false
			}
			last := w.Frames[len(w.Frames)-1]
			return last.State == "finished" && !last.Timestamp.Before(g.Start) && w.Metadata.Blue.TeamID == g.Metadata.Blue.TeamID && w.Metadata.Red.TeamID == g.Metadata.Red.TeamID
		}
		if !p.readDisk("end-"+g.ID, &result) || !valid(result) {
			if err := p.fetch(ctx, true, "window/"+g.ID, url.Values{"startingTime": {proStartingTime(g.Start.Add(3 * time.Hour))}}, &result); err != nil {
				return err
			}
		}
		if !valid(result) {
			return errors.New("invalid terminal frame or team identity")
		}
		last := result.Frames[len(result.Frames)-1]
		// Store only terminal frame, never the 10-frame upstream response.
		result.Frames = []proFrame{last}
		p.writeDisk("end-"+g.ID, result)
		p.mu.Lock()
		p.ends[g.ID] = result
		p.mu.Unlock()
		return nil
	})
	return result, err
}
func (p *proRuneProvider) supplement(ctx context.Context, g proRuneIndexGame) (proDetails, error) {
	var result proDetails
	err := p.gameResource(ctx, "details-"+g.ID, func() error {
		p.mu.Lock()
		cached, ok := p.details[g.ID]
		p.mu.Unlock()
		if ok {
			result = cached
			return nil
		}
		end, err := p.terminal(ctx, g)
		if err != nil {
			return err
		}
		last := end.Frames[len(end.Frames)-1]
		valid := func(d proDetails) bool {
			if len(d.Frames) == 0 {
				return false
			}
			f := d.Frames[len(d.Frames)-1]
			if f.Timestamp.Before(last.Timestamp.UTC().Truncate(10*time.Second)) || len(f.Participants) == 0 {
				return false
			}
			allowed := map[int]bool{}
			for _, team := range []proTeamMetadata{g.Metadata.Blue, g.Metadata.Red} {
				for _, p := range team.Participants {
					allowed[p.ParticipantID] = true
				}
			}
			seen := map[int]bool{}
			for _, p := range f.Participants {
				if !allowed[p.ID] || seen[p.ID] {
					return false
				}
				seen[p.ID] = true
			}
			return true
		}
		if !p.readDisk("details-"+g.ID, &result) || !valid(result) {
			if err = p.fetch(ctx, true, "details/"+g.ID, url.Values{"startingTime": {proStartingTime(last.Timestamp)}}, &result); err != nil {
				return err
			}
		}
		if !valid(result) {
			return errors.New("invalid perk frame or participants")
		}
		result.Frames = result.Frames[len(result.Frames)-1:]
		p.writeDisk("details-"+g.ID, result)
		p.mu.Lock()
		p.details[g.ID] = result
		p.mu.Unlock()
		return nil
	})
	return result, err
}

func proInhibitorWinner(g proRuneIndexGame, w proWindow) string {
	if len(w.Frames) == 0 {
		return ""
	}
	f := w.Frames[len(w.Frames)-1]
	if f.State != "finished" {
		return ""
	}
	if f.Blue.Inhibitors >= 1 && f.Red.Inhibitors == 0 {
		return g.Metadata.Blue.TeamID
	}
	if f.Red.Inhibitors >= 1 && f.Blue.Inhibitors == 0 {
		return g.Metadata.Red.TeamID
	}
	return ""
}

// Unknowns are solved only when the remaining official score has one solution.
// A missing game's terminal frame must not turn partial series into proof.
func proSeriesWinners(e proRuneEvent, games map[string]proRuneIndexGame, ends map[string]proWindow) (map[string]string, bool) {
	winners := map[string]string{}
	if !proEventAllowed(e) {
		return winners, false
	}
	scores := map[string]int{}
	for _, t := range e.Match.Teams {
		if t.Result.GameWins < 0 {
			return winners, false
		}
		scores[t.ID] = t.Result.GameWins
	}
	unknown := []string{}
	complete := true
	count := 0
	seen := map[string]bool{}
	for _, game := range e.Match.Games {
		if game.State != "completed" {
			continue
		}
		if seen[game.ID] {
			return nil, false
		}
		seen[game.ID] = true
		count++
		g, ok := games[game.ID]
		w, found := ends[game.ID]
		if !ok || !found || len(w.Frames) == 0 || w.Frames[len(w.Frames)-1].State != "finished" {
			complete = false
			unknown = append(unknown, game.ID)
			continue
		}
		winner := proInhibitorWinner(g, w)
		if winner == "" {
			unknown = append(unknown, game.ID)
			continue
		}
		if _, valid := scores[winner]; !valid {
			return nil, false
		}
		scores[winner]--
		winners[game.ID] = winner
	}
	for _, left := range scores {
		if left < 0 {
			return nil, false
		}
	}
	total := 0
	for _, t := range e.Match.Teams {
		total += t.Result.GameWins
	}
	if total != count {
		return nil, false
	}
	if complete && len(unknown) > 0 {
		for id, left := range scores {
			if left == len(unknown) {
				for _, gameID := range unknown {
					winners[gameID] = id
				}
				break
			}
		}
	}
	return winners, true
}

type proRuneRow struct {
	Game           proRuneIndexGame
	Participant    proParticipant
	Player         string
	TeamID         string
	Opponent       proParticipant
	OpponentTeamID string
}

func (p *proRuneProvider) rows(snapshot proRuneSnapshot, now time.Time) []proRuneRow {
	result := []proRuneRow{}
	for _, g := range snapshot.Games {
		if !g.Start.After(now.Add(-30*24*time.Hour)) || g.Start.After(now) || !proEventAllowed(snapshot.Events[g.MatchID]) {
			continue
		}
		for _, pair := range [][2]proTeamMetadata{{g.Metadata.Blue, g.Metadata.Red}, {g.Metadata.Red, g.Metadata.Blue}} {
			for _, part := range pair[0].Participants {
				name := p.player(pair[0].TeamID, part)
				if name == "" {
					continue
				}
				var opponent proParticipant
				matches := 0
				for _, other := range pair[1].Participants {
					if other.Role != "" && other.Role == part.Role {
						opponent = other
						matches++
					}
				}
				if matches != 1 {
					opponent = proParticipant{}
				}
				result = append(result, proRuneRow{Game: g, Participant: part, Player: name, TeamID: pair[0].TeamID, Opponent: opponent, OpponentTeamID: pair[1].TeamID})
			}
		}
	}
	return result
}

// Match the requested lane strictly; never borrow another lane's rune page.
// The cap is per roster identity, not per champion or number of player tabs.
func proNewestRows(rows []proRuneRow, champion, position string) []proRuneRow {
	result := []proRuneRow{}
	lane := canonicalSpecialistPosition(position)
	if lane == "" || lane == "other" {
		return result
	}
	for _, r := range rows {
		if strings.EqualFold(r.Participant.ChampionID, champion) && canonicalSpecialistPosition(r.Participant.Role) == lane {
			result = append(result, r)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if !result[i].Game.Start.Equal(result[j].Game.Start) {
			return result[i].Game.Start.After(result[j].Game.Start)
		}
		return result[i].Game.ID+result[i].Participant.PlayerID < result[j].Game.ID+result[j].Participant.PlayerID
	})
	counts := map[string]int{}
	selected := []proRuneRow{}
	for _, r := range result {
		key := r.TeamID + "/" + r.Player
		if counts[key] >= 5 {
			continue
		}
		counts[key]++
		selected = append(selected, r)
	}
	return selected
}

// Official perkMetadata is sometimes a set: repeated shard IDs occur only
// once. Solve against all three legal slots, preserving explicit multiplicity
// for nine-ID records. Only a unique assignment is complete; ambiguous slots
// remain zero. Never fill using a popular/recommended rune page.
func proRuneShards(perks []int64) ([]int64, bool) {
	slots := [][]int64{{5008, 5005, 5007}, {5008, 5010, 5001}, {5001, 5013, 5011}}
	empty := []int64{0, 0, 0}
	if len(perks) < 8 || len(perks) > 9 {
		return empty, false
	}
	counts := map[int64]int{}
	for _, id := range perks[6:] {
		counts[id]++
	}
	// An eight-ID response must contain two distinct IDs to be a deduped set.
	if len(perks) == 8 && len(counts) != 2 {
		return empty, false
	}
	solutions := [][]int64{}
	for _, a := range slots[0] {
		for _, b := range slots[1] {
			for _, c := range slots[2] {
				choice := []int64{a, b, c}
				actual := map[int64]int{}
				for _, id := range choice {
					actual[id]++
				}
				valid := len(actual) == len(counts)
				for id, count := range counts {
					if actual[id] == 0 || (len(perks) == 9 && actual[id] != count) {
						valid = false
					}
				}
				if valid {
					solutions = append(solutions, choice)
				}
			}
		}
	}
	if len(solutions) == 0 {
		return empty, false
	}
	known := append([]int64(nil), solutions[0]...)
	for _, choice := range solutions[1:] {
		for i, id := range choice {
			if known[i] != id {
				known[i] = 0
			}
		}
	}
	return known, len(solutions) == 1
}

type proRuneResponse struct {
	Pros            []gameplayRecommendationRune `json:"pros"`
	Reason          string                       `json:"reason,omitempty"`
	Preparing       bool                         `json:"preparing"`
	OutcomesLoading bool                         `json:"outcomesLoading"`
	Stale           bool                         `json:"stale"`
	ReadAt          time.Time                    `json:"readAt"`
	Failed          int                          `json:"failedGames"`
	TotalGames      int                          `json:"totalGames"`
}

func (p *proRuneProvider) recommend(ctx context.Context, championID int, position string) proRuneResponse {
	p.mu.Lock()
	snapshot := p.snapshot
	failedGames := map[string]bool{}
	totalGames := len(snapshot.Games)
	for id := range p.failedGameIDs {
		failedGames[id] = true
		if _, ok := snapshot.Games[id]; !ok {
			totalGames++
		}
	}
	response := proRuneResponse{Pros: []gameplayRecommendationRune{}, Reason: p.reason, Preparing: p.preparing, OutcomesLoading: p.outcomesLoading, ReadAt: snapshot.ReadAt, Failed: len(failedGames), TotalGames: totalGames}
	sourceIncomplete := p.sourceIncomplete || p.cacheWriteFailed
	if p.cacheWriteFailed && response.Reason == "" {
		response.Reason = "cache-write-failed"
	}
	p.mu.Unlock()
	now := time.Now()
	response.Stale = sourceIncomplete || (response.Reason != "" && response.Failed == 0) || snapshot.ReadAt.IsZero() || now.Sub(snapshot.ReadAt) > proRuneTTL
	if len(snapshot.Games) == 0 {
		if response.Preparing {
			response.Reason = "preparing"
		} else if response.Reason == "" {
			response.Reason = "no-sample"
		}
		return response
	}
	meta, err := p.provider.championMetadataByID(ctx, championID)
	if err != nil {
		response.Reason = proRuneErrorReason(err)
		response.Stale = true
		return response
	}
	rows := p.rows(snapshot, now)
	selected := proNewestRows(rows, meta.Key, position)
	if len(selected) == 0 {
		if response.Reason == "" {
			response.Reason = "no-sample"
		}
		return response
	}
	byGame := map[string]proDetails{}
	var mu sync.Mutex
	proHitParallel(ctx, selected, func(r proRuneRow) {
		d, err := p.supplement(ctx, r.Game)
		mu.Lock()
		defer mu.Unlock()
		if err == nil {
			byGame[r.Game.ID] = d
		} else {
			response.Reason = proRuneErrorReason(err)
			// Index, terminal and detail failures for one game count only once.
			failedGames[r.Game.ID] = true
			response.Failed = len(failedGames)
		}
	})
	p.mu.Lock()
	ends := map[string]proWindow{}
	for id, w := range p.ends {
		ends[id] = w
	}
	p.mu.Unlock()
	outcomes := map[string]string{}
	rejected := map[string]bool{}
	for id, event := range snapshot.Events {
		winners, ok := proSeriesWinners(event, snapshot.Games, ends)
		if !ok {
			rejected[id] = true
			continue
		}
		for game, winner := range winners {
			outcomes[game] = winner
		}
	}
	type record struct {
		games, wins int
		partial     bool
	}
	records := map[string]record{}
	for _, r := range rows {
		if rejected[r.Game.MatchID] {
			continue
		}
		key := r.TeamID + "/" + r.Player + "/" + r.Participant.ChampionID
		rec := records[key]
		if winner := outcomes[r.Game.ID]; winner != "" {
			rec.games++
			if winner == r.TeamID {
				rec.wins++
			}
		} else {
			rec.partial = true
		}
		records[key] = rec
	}
	opponents, opponentErr := p.opponentMetadata(ctx, selected)
	if opponentErr != nil {
		response.Reason = proRuneErrorReason(opponentErr)
		response.Stale = true
	}
	for _, r := range selected {
		if rejected[r.Game.MatchID] {
			continue
		}
		details, ok := byGame[r.Game.ID]
		if !ok || len(details.Frames) == 0 {
			continue
		}
		var participant *proDetailParticipant
		for _, part := range details.Frames[0].Participants {
			if part.ID == r.Participant.ParticipantID {
				copy := part
				participant = &copy
				break
			}
		}
		if participant == nil || len(participant.Perks.Perks) < 6 || participant.Perks.StyleID <= 0 || participant.Perks.SubStyleID <= 0 {
			continue
		}
		shards, complete := proRuneShards(participant.Perks.Perks)
		perkIDs := append([]int64(nil), participant.Perks.Perks...)
		if complete {
			perkIDs = append(perkIDs[:6:6], shards...)
		}
		shardResolution := "unresolved"
		if complete {
			shardResolution = "explicit-unique"
			if len(participant.Perks.Perks) == 8 {
				shardResolution = "deduplicated-unique"
			}
		}
		known := outcomes[r.Game.ID] != ""
		win := outcomes[r.Game.ID] == r.TeamID
		rec := records[r.TeamID+"/"+r.Player+"/"+r.Participant.ChampionID]
		e := snapshot.Events[r.Game.MatchID]
		teams := []string{}
		for _, t := range e.Match.Teams {
			code := proRuneTeams[t.ID]
			if code == "" {
				code = t.Code
			}
			teams = append(teams, code)
		}
		config := gameplayRecommendationRune{Key: fmt.Sprintf("pro-%s-%d", r.Game.ID, r.Participant.ParticipantID), SourceKey: "pro", Title: proRuneTeams[r.TeamID] + " " + r.Player,
			ChampionID: int64(championID), ChampionName: meta.NameZH, PlayerName: r.Player, Position: map[string]string{"top": "上路", "jungle": "打野", "mid": "中路", "bottom": "下路", "support": "辅助"}[r.Participant.Role],
			PrimaryStyleID: participant.Perks.StyleID, SubStyleID: participant.Perks.SubStyleID, SelectedPerkIDs: perkIDs, StatModIDs: shards, ShardResolution: shardResolution, SelectedComplete: &complete, ItemIDs: participant.Items,
			PlayedAt: r.Game.Start.UnixMilli(), WinKnown: &known, RecordGames: rec.games, RecordWins: rec.wins, RecordPartial: rec.partial || response.Stale,
			OpponentPlayerName: proOpponentDisplayName(r, e), EventLabel: fmt.Sprintf("%s · %s · %s 第 %d 局", proRuneLeagues[r.Game.LeagueID], r.Game.Start.UTC().Format("2006-01-02"), strings.Join(teams, " vs "), r.Game.Number)}
		if !snapshot.ReadAt.IsZero() {
			config.CacheReadAt = snapshot.ReadAt.UTC().Format(time.RFC3339)
		}
		if m, ok := opponents[strings.ToLower(r.Opponent.ChampionID)]; ok {
			config.OpponentChampionID = int64(m.ID)
			config.OpponentChampionName = m.NameZH
		}
		if known {
			config.Win = &win
			if win {
				config.Result = "win"
			} else {
				config.Result = "loss"
			}
		}
		response.Pros = append(response.Pros, config)
	}
	p.saveIdentities()
	if len(response.Pros) == 0 && response.Reason == "" {
		response.Reason = "upstream-error"
	}
	return response
}

// Process every selected game with bounded workers. Five is a per-player
// display cap, never a cap on the amount of work scheduled for a response.
// The shared public-network gate still limits actual upstream concurrency.
func proHitParallel[T any](ctx context.Context, values []T, fn func(T)) {
	var wg sync.WaitGroup
	jobs := make(chan T)
	for i := 0; i < min(5, len(values)); i++ {
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

// Resolve all opponent IDs with at most one bounded catalog load per response.
// Catalog failures must not trigger a fresh multi-request load for every row.
func (p *proRuneProvider) opponentMetadata(ctx context.Context, rows []proRuneRow) (map[string]championMetadata, error) {
	result := map[string]championMetadata{}
	collect := func() bool {
		p.provider.mu.Lock()
		defer p.provider.mu.Unlock()
		missing := false
		for _, row := range rows {
			key := strings.ToLower(row.Opponent.ChampionID)
			if key == "" {
				continue
			}
			meta := p.provider.championMeta[p.provider.championIDs[key]]
			if meta.ID > 0 {
				result[key] = meta
			} else {
				missing = true
			}
		}
		return missing
	}
	if !collect() {
		return result, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	select {
	case p.gate <- struct{}{}:
	case <-ctx.Done():
		return result, ctx.Err()
	}
	defer func() { <-p.gate }()
	if _, err := p.provider.loadCatalog(ctx); err != nil {
		return result, err
	}
	if collect() {
		return result, errors.New("unknown opponent champion")
	}
	return result, nil
}

// Only split the team code supplied by this match's official team ID.
// Account names from other sources are never rewritten.
func proOpponentDisplayName(row proRuneRow, event proRuneEvent) string {
	name := strings.TrimSpace(row.Opponent.SummonerName)
	if name == "" {
		return ""
	}
	for _, team := range event.Match.Teams {
		if team.ID != row.OpponentTeamID {
			continue
		}
		code := strings.TrimSpace(team.Code)
		if known := proRuneTeams[team.ID]; known != "" {
			code = known
		}
		if code == "" {
			return name
		}
		if len(name) > len(code) && strings.EqualFold(name[:len(code)], code) {
			name = strings.TrimSpace(name[len(code):])
		}
		return code + " " + name
	}
	return name
}
