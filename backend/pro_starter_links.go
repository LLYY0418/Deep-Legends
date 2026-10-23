package main

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type proStarterLink struct {
	Game   proRuneIndexGame
	Event  proRuneEvent
	At     time.Time
	Reason string
}

func proTeamCode(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }
func proSameTeams(a, b string, teams []proEventTeam) bool {
	if len(teams) != 2 || a == "" || b == "" || a == b {
		return false
	}
	x, y := proTeamCode(teams[0].Code), proTeamCode(teams[1].Code)
	return a == x && b == y || a == y && b == x
}
func proStarterLinkMatches(link proStarterLink, row proOPGGBuild) bool {
	return proEventAllowed(link.Event) && proRuneLeagues[link.Event.League.ID] == row.League && link.Game.MatchID == link.Event.ID && link.Game.Number == row.Game.Set && proNumericID(link.Game.ID) && !link.Game.Start.IsZero() && link.Game.Start.Sub(row.Game.BeginAt).Abs() <= 24*time.Hour && proSameTeams(proTeamCode(row.Game.Match.HomeTeam.Acronym), proTeamCode(row.Game.Match.AwayTeam.Acronym), link.Event.Match.Teams)
}

// Fallback when the public OP.GG purchase timeline cannot be read. Resolve only
// a unique official series by both teams, date and set; never infer opening items.
func (p *proRuneProvider) officialStarterLink(ctx context.Context, row proOPGGBuild) (proStarterLink, error) {
	var link proStarterLink
	if !proNumericID(row.Game.Match.ID) || row.Game.Set <= 0 || row.Game.BeginAt.IsZero() {
		return link, errors.New("missing-match-metadata")
	}
	key := "starter-link-" + row.Game.Match.ID + "-" + strconv.Itoa(row.Game.Set)
	err := p.gameResource(ctx, key, func() (loadErr error) {
		cacheResult := true
		defer func() {
			if !cacheResult || errors.Is(loadErr, context.Canceled) {
				return
			}
			p.mu.Lock()
			if p.starterLinks == nil {
				p.starterLinks = map[string]proStarterLink{}
			}
			if len(p.starterLinks) >= 128 {
				for old := range p.starterLinks {
					if old != key {
						delete(p.starterLinks, old)
						break
					}
				}
			}
			if loadErr == nil {
				p.starterLinks[key] = link
			} else {
				p.starterLinks[key] = proStarterLink{At: time.Now(), Reason: proStarterFailureReason(loadErr)}
			}
			p.mu.Unlock()
			if loadErr == nil {
				p.writeDisk(key, link)
			}
		}()

		p.mu.Lock()
		cached, ok := p.starterLinks[key]
		p.mu.Unlock()
		if ok && cached.Reason != "" && time.Since(cached.At) < 90*time.Second {
			cacheResult = false
			return errors.New(cached.Reason)
		}
		if ok && proStarterLinkMatches(cached, row) {
			link = cached
			return nil
		}
		if p.readDisk(key, &cached) && proStarterLinkMatches(cached, row) {
			link = cached
			return nil
		}
		// An already indexed series needs no extra schedule requests.
		p.mu.Lock()
		snapshot := p.snapshot
		p.mu.Unlock()
		matches := []proStarterLink{}
		for _, game := range snapshot.Games {
			candidate := proStarterLink{Game: game, Event: snapshot.Events[game.MatchID]}
			if proStarterLinkMatches(candidate, row) {
				matches = append(matches, candidate)
			}
		}
		if len(matches) == 1 {
			link = matches[0]
			return nil
		}
		if len(matches) > 1 {
			return errors.New("ambiguous-official-game")
		}
		var leagueID string
		for id, name := range proRuneLeagues {
			if name == row.League {
				leagueID = id
			}
		}
		if leagueID == "" {
			return errors.New("unsupported-league")
		}
		ids := map[string]bool{}
		token := ""
		seen := map[string]bool{}
		home, away := proTeamCode(row.Game.Match.HomeTeam.Acronym), proTeamCode(row.Game.Match.AwayTeam.Acronym)
		for page := 0; page < 20; page++ {
			q := url.Values{"hl": {"zh-CN"}, "leagueId": {leagueID}}
			if token != "" {
				q.Set("pageToken", token)
			}
			var response struct {
				Data struct {
					Schedule struct {
						Pages  struct{ Older string }
						Events []struct {
							Start time.Time `json:"startTime"`
							State string
							Match struct {
								ID    string
								Teams []proEventTeam
							}
						}
					}
				}
			}
			if err := p.fetch(withProBackground(ctx), false, "getSchedule", q, &response); err != nil {
				return err
			}
			oldest := time.Now()
			for _, event := range response.Data.Schedule.Events {
				if event.Start.Before(oldest) {
					oldest = event.Start
				}
				if event.State == "completed" && event.Start.Sub(row.Game.BeginAt).Abs() <= 24*time.Hour && proSameTeams(home, away, event.Match.Teams) && proNumericID(event.Match.ID) {
					ids[event.Match.ID] = true
				}
			}
			token = response.Data.Schedule.Pages.Older
			if token == "" || oldest.Before(row.Game.BeginAt.Add(-24*time.Hour)) {
				break
			}
			if seen[token] {
				return errors.New("schedule-pagination-cycle")
			}
			seen[token] = true
			if page == 19 {
				return errors.New("schedule-pagination-limit")
			}
		}
		if len(ids) == 0 {
			return errors.New("official-match-not-found")
		}
		if len(ids) != 1 {
			return errors.New("ambiguous-official-match")
		}
		var id string
		for candidate := range ids {
			id = candidate
		}
		event, err := p.event(ctx, id)
		if err != nil {
			return err
		}
		if !proEventAllowed(event) || event.League.ID != leagueID || !proSameTeams(home, away, event.Match.Teams) {
			return errors.New("official-team-mismatch")
		}
		var games []proEventGame
		for _, game := range event.Match.Games {
			if game.Number == row.Game.Set && game.State == "completed" {
				games = append(games, game)
			}
		}
		if len(games) != 1 {
			return errors.New("official-set-not-found")
		}
		game, err := p.indexGame(ctx, event, games[0])
		if err != nil {
			return err
		}
		link = proStarterLink{Game: game, Event: event, At: time.Now()}
		if !proStarterLinkMatches(link, row) {
			return errors.New("official-date-mismatch")
		}
		return nil
	})
	return link, err
}
func proStarterParticipant(link proStarterLink, row proOPGGBuild, championKey string) (int, error) {
	teamID := ""
	for _, team := range link.Event.Match.Teams {
		if proTeamCode(team.Code) == proTeamCode(row.Team.Acronym) {
			if teamID != "" {
				return 0, errors.New("ambiguous-team")
			}
			teamID = team.ID
		}
	}
	if teamID == "" {
		return 0, errors.New("participant-team-not-found")
	}
	id := 0
	for _, team := range []proTeamMetadata{link.Game.Metadata.Blue, link.Game.Metadata.Red} {
		if team.TeamID != teamID {
			continue
		}
		for _, participant := range team.Participants {
			if !strings.EqualFold(participant.ChampionID, championKey) {
				continue
			}
			if row.Position != "" && canonicalSpecialistPosition(participant.Role) != canonicalSpecialistPosition(row.Position) {
				continue
			}
			if id != 0 {
				return 0, errors.New("ambiguous-participant")
			}
			id = participant.ParticipantID
		}
	}
	if id <= 0 {
		return 0, errors.New("participant-not-found")
	}
	return id, nil
}
func proStarterFailureReason(err error) string {
	for _, reason := range []string{"official-match-not-found", "ambiguous-official-match", "ambiguous-official-game", "ambiguous-participant", "ambiguous-team", "participant-not-found", "participant-team-not-found", "official-set-not-found", "opening-items-empty", "missing-match-metadata", "official-team-mismatch", "official-date-mismatch", "opgg-row-not-found", "schedule-pagination-cycle", "schedule-pagination-limit", "unsupported-league"} {
		if strings.Contains(err.Error(), reason) {
			return reason
		}
	}
	return proRuneErrorReason(err)
}

func (p *proRuneProvider) opggStarterRows(ctx context.Context, championID int, championKey string, wanted map[string]int64) []runeStarterRow {
	p.mu.Lock()
	cached, exists := p.opggBuilds[championID]
	p.mu.Unlock()
	if !exists {
		p.readDisk("opgg-builds-"+strconv.Itoa(championID), &cached)
	}
	selected := []proOPGGBuild{}
	matched := map[string]bool{}
	for _, row := range cached.Rows {
		key := "pro-opgg-" + row.ID
		if at, ok := wanted[key]; !ok || at != row.Game.BeginAt.UnixMilli() || row.ChampionID != strconv.Itoa(championID) || matched[key] {
			continue
		}
		matched[key] = true
		selected = append(selected, row)
		if len(selected) >= runeStarterRowLimit {
			break
		}
	}
	for key := range wanted {
		if strings.HasPrefix(key, "pro-opgg-") && !matched[key] && p.provider.diag != nil {
			p.provider.diag(map[string]any{"event": "rune_starter_items", "source": "pro", "outcome": "failed", "reason": "opgg-row-not-found", "item_count": 0})
		}
	}
	result := make([]runeStarterRow, len(selected))
	runeStarterParallel(ctx, selected, 2, func(index int, row proOPGGBuild) {
		started := time.Now()
		game, directErr := p.opggOpeningGame(ctx, row)
		if directErr == nil {
			items, itemErr := proOPGGOpeningItems(game, row)
			if itemErr == nil {
				result[index] = runeStarterRow{Key: "pro-opgg-" + row.ID, PlayedAt: row.Game.BeginAt.UnixMilli(), StarterItemIDs: items}
				if p.provider.diag != nil {
					p.provider.diag(map[string]any{"event": "rune_starter_items", "source": "pro", "outcome": "success", "provider": "opgg-purchases", "item_count": len(items), "duration_ms": time.Since(started).Milliseconds()})
				}
				return
			}
			directErr = itemErr
		}
		link, err := p.officialStarterLink(ctx, row)
		id := 0
		if err == nil {
			id, err = proStarterParticipant(link, row, championKey)
		}
		if err == nil {
			var details proDetails
			details, err = p.openingDetails(ctx, link.Game, []int{id})
			if err == nil {
				items, _ := proStarterItems(details, id, link.Game.Start)
				if len(items) == 0 {
					err = errors.New("opening-items-empty")
				} else {
					result[index] = runeStarterRow{Key: "pro-opgg-" + row.ID, PlayedAt: row.Game.BeginAt.UnixMilli(), StarterItemIDs: items}
				}
			}
		}
		recordRuneStarterBatchError(ctx, err)
		if p.provider.diag != nil {
			outcome, reason := "success", ""
			if err != nil {
				outcome, reason = "failed", proStarterFailureReason(err)
				for _, r := range []string{"official-match-not-found", "ambiguous-official-match", "ambiguous-official-game", "participant-not-found", "participant-team-not-found", "official-set-not-found", "opening-items-empty", "missing-match-metadata"} {
					if strings.Contains(err.Error(), r) {
						reason = r
						break
					}
				}
			}
			p.provider.diag(map[string]any{"event": "rune_starter_items", "source": "pro", "outcome": outcome, "reason": reason, "provider": "official-fallback", "opgg_reason": proStarterFailureReason(directErr), "item_count": len(result[index].StarterItemIDs), "duration_ms": time.Since(started).Milliseconds()})
		}
	})
	return nonemptyStarterRows(result)
}
