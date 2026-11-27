package main

import (
	"context"
	"errors"
	"net/url"
	"sort"
	"sync"
	"time"
)

type proOfficialTournament struct {
	id           string
	start, until time.Time
}

type proOfficialMatch struct {
	id, serie string
	start     time.Time
}

type proOfficialLeague struct {
	league      proRuneLeague
	tournaments []proOfficialTournament
	matches     []proOfficialMatch
}

func (l proOfficialLeague) headBefore(before time.Time) proRuneScope {
	var head proRuneScope
	for _, match := range l.matches {
		if match.start.Before(before) && match.start.After(head.Last) {
			head = proRuneScope{l.league, match.serie, match.start}
		}
	}
	return head
}

func (p *proRuneProvider) officialLeagueMatches(ctx context.Context, league proRuneLeague, now time.Time) (proOfficialLeague, error) {
	result := proOfficialLeague{league: league}
	query := url.Values{"hl": {"zh-CN"}, "leagueId": {league.ID}}
	var calendar struct {
		Data struct {
			Leagues []struct {
				Tournaments []struct{ ID, StartDate, EndDate string }
			}
		}
	}
	if err := p.fetch(ctx, false, "getTournamentsForLeague", query, &calendar); err != nil {
		return result, err
	}
	if len(calendar.Data.Leagues) != 1 {
		return result, errors.New("invalid official tournament calendar")
	}
	for _, tournament := range calendar.Data.Leagues[0].Tournaments {
		start, errStart := time.Parse(time.DateOnly, tournament.StartDate)
		end, errEnd := time.Parse(time.DateOnly, tournament.EndDate)
		if errStart == nil && errEnd == nil && proNumericID(tournament.ID) && !start.After(end) && !start.After(now) {
			result.tournaments = append(result.tournaments, proOfficialTournament{tournament.ID, start, end.AddDate(0, 0, 1)})
		}
	}
	sort.Slice(result.tournaments, func(i, j int) bool { return result.tournaments[i].start.After(result.tournaments[j].start) })
	if len(result.tournaments) == 0 {
		return result, nil
	}
	// One extra tournament covers the previous edition of the same league.
	if len(result.tournaments) > 2 {
		result.tournaments = result.tournaments[:2]
	}
	floor := result.tournaments[len(result.tournaments)-1].start
	seen := map[string]bool{}
	for page := 0; page < 20; page++ {
		var wire struct {
			Data struct {
				Schedule struct {
					Pages  struct{ Older string }
					Events []struct {
						Start time.Time `json:"startTime"`
						State string
						Match struct{ ID string }
					}
				}
			}
		}
		if err := p.fetch(ctx, false, "getSchedule", query, &wire); err != nil {
			return result, err
		}
		oldest := now
		for _, event := range wire.Data.Schedule.Events {
			if !event.Start.IsZero() && event.Start.Before(oldest) {
				oldest = event.Start
			}
			if event.State != "completed" || event.Start.After(now) || !proNumericID(event.Match.ID) {
				continue
			}
			for _, tournament := range result.tournaments {
				if !event.Start.Before(tournament.start) && event.Start.Before(tournament.until) {
					result.matches = append(result.matches, proOfficialMatch{event.Match.ID, tournament.id, event.Start})
					break
				}
			}
		}
		token := wire.Data.Schedule.Pages.Older
		if token == "" || oldest.Before(floor) {
			return result, nil
		}
		if seen[token] {
			return result, errors.New("official schedule pagination cycle")
		}
		seen[token] = true
		query.Set("pageToken", token)
	}
	return result, errors.New("official schedule pagination limit")
}

type proOfficialSelection struct {
	leagueID string
	period   proRunePeriod
	previous bool
}

// Schedule times cover whole best-of series. When two leagues have adjacent
// matches, compare the actual kickoff of the relevant map from the live feed.
func (p *proRuneProvider) officialMapStart(ctx context.Context, data []proOfficialLeague, scope proRuneScope, first bool) (time.Time, error) {
	for _, league := range data {
		if league.league.ID != scope.ID {
			continue
		}
		for _, match := range league.matches {
			if match.serie != scope.Serie || !match.start.Equal(scope.Last) {
				continue
			}
			event, err := p.event(ctx, match.id)
			if err != nil {
				return time.Time{}, err
			}
			var target proEventGame
			for _, game := range event.Match.Games {
				if game.State == "completed" && (target.ID == "" || first && game.Number < target.Number || !first && game.Number > target.Number) {
					target = game
				}
			}
			if event.League.ID != scope.ID || target.ID == "" || !proEventAllowed(event) {
				return time.Time{}, errors.New("invalid official tournament boundary match")
			}
			index, err := p.indexGame(ctx, event, target)
			return index.Start, err
		}
	}
	return time.Time{}, errors.New("missing official tournament boundary match")
}

func officialNearDomestic(data []proOfficialLeague, at time.Time) bool {
	for _, league := range data {
		if league.league.Domestic {
			for _, match := range league.matches {
				if match.start.Sub(at).Abs() <= 24*time.Hour {
					return true
				}
			}
		}
	}
	return false
}

func (p *proRuneProvider) officialLatestInternational(ctx context.Context, data []proOfficialLeague, heads []proRuneScope) (proRuneScope, error) {
	international := proLatestInternational(heads)
	if !international.Last.IsZero() && officialNearDomestic(data, international.Last) {
		cutoff, err := p.officialMapStart(ctx, data, international, false)
		if err != nil {
			return proRuneScope{}, err
		}
		for i, head := range heads {
			if head.ID == international.ID {
				heads[i].Last = cutoff
			} else if head.Domestic && head.Last.Sub(international.Last).Abs() <= 24*time.Hour {
				heads[i].Last, err = p.officialMapStart(ctx, data, head, false)
				if err != nil {
					return proRuneScope{}, err
				}
			}
		}
		international.Last = cutoff
	}
	return international, nil
}

func (p *proRuneProvider) officialSegments(ctx context.Context, now time.Time) (map[string]proOfficialSelection, error) {
	// Keep the order stable even when some leagues finish at the same time.
	leagues := make([]proRuneLeague, 0, len(proRuneLeagues))
	for _, league := range proOPGGLeagues {
		for id, name := range proRuneLeagues {
			if name == league.Name {
				leagues = append(leagues, proRuneLeague{id, name, league.Domestic})
			}
		}
	}
	data := make([]proOfficialLeague, len(leagues))
	indices := make([]int, len(leagues))
	for i := range indices {
		indices[i] = i
	}
	var mu sync.Mutex
	var readErr error
	proParallel(ctx, indices, func(i int) {
		result, err := p.officialLeagueMatches(ctx, leagues[i], now)
		if err != nil {
			mu.Lock()
			readErr = err
			mu.Unlock()
			return
		}
		data[i] = result
	})
	if readErr != nil {
		return nil, readErr
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	heads := make([]proRuneScope, len(data))
	for i, league := range data {
		heads[i] = league.headBefore(now.Add(time.Nanosecond))
	}
	international, err := p.officialLatestInternational(ctx, data, heads)
	if err != nil {
		return nil, err
	}
	current := proCurrentPeriod(heads)
	var previous proRunePeriod
	if len(current.Scopes) != 0 && current.Scopes[0].Domestic {
		if international.Serie != "" {
			previous.Scopes = []proRuneScope{international}
		}
	} else if international.Serie != "" {
		// The official schedule exposes one start per match, with the
		// tournament calendar identifying the edition across all stages.
		first := international
		for _, league := range data {
			if league.league.ID == international.ID {
				for _, match := range league.matches {
					if match.serie == international.Serie && match.start.Before(first.Last) {
						first.Last = match.start
					}
				}
			}
		}
		if officialNearDomestic(data, first.Last) {
			first.Last, err = p.officialMapStart(ctx, data, first, true)
			if err != nil {
				return nil, err
			}
		}
		for i, league := range data {
			heads[i] = league.headBefore(first.Last)
		}
		prior, err := p.officialLatestInternational(ctx, data, heads)
		if err != nil {
			return nil, err
		}
		previous.After, previous.Before = prior.Last, first.Last
		for _, scope := range heads {
			if scope.Domestic && scope.Last.After(previous.After) {
				previous.Scopes = append(previous.Scopes, scope)
			}
		}
	}
	matches := map[string]proOfficialSelection{}
	for i, period := range []proRunePeriod{current, previous} {
		for _, scope := range period.Scopes {
			for _, league := range data {
				if league.league.ID != scope.ID {
					continue
				}
				for _, match := range league.matches {
					// An official series can straddle a boundary. Its individual
					// games are checked against their actual starts after indexing.
					if match.serie == scope.Serie && (period.Before.IsZero() || match.start.Before(period.Before)) &&
						match.start.After(period.After.Add(-24*time.Hour)) {
						matches[match.id] = proOfficialSelection{scope.ID, period, i == 1}
					}
				}
			}
		}
	}
	return matches, nil
}
