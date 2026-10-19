package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const esportsOPGGHost = "esports.op.gg"
const proOPGGPageSize = 100
const proOPGGMaxPages = 20

// Exact IDs from the public esports site's league metadata. Keep R75's scope.
var proOPGGLeagues = []struct{ ID, Name string }{{"98", "LPL"}, {"99", "LCK"}, {"141", "First Stand"}, {"100", "MSI"}, {"96", "Worlds"}}

const proOPGGBuildQuery = `query ListBuilds($leagueId:ID,$championId:ID,$page:Int,$limit:Int){builds(leagueId:$leagueId,championId:$championId,page:$page,limit:$limit){id championId position gameVersion items spells perk{primary{style selections}sub{style selections}stats{offense flex defense}}skillLevelUps{skillSlot levelUpType}player{id nickName}team{id acronym}opponentGamePlayer{championId player{nickName}team{acronym}}game{id length beginAt set winner{id}match{id homeTeam{acronym}awayTeam{acronym}}}}}`

type proOPGGTeam struct{ ID, Acronym string }
type proOPGGPlayer struct{ ID, NickName string }
type proOPGGBuild struct {
	ID, ChampionID, Position, GameVersion string
	Items, Spells                         []int64
	Perk                                  *struct {
		Primary, Sub struct {
			Style      int64
			Selections []int64
		}
		Stats struct{ Offense, Flex, Defense int64 }
	}
	SkillLevelUps []struct {
		SkillSlot   int
		LevelUpType string
	}
	Player             proOPGGPlayer
	Team               proOPGGTeam
	OpponentGamePlayer *struct {
		ChampionID string
		Player     proOPGGPlayer
		Team       proOPGGTeam
	}
	Game struct {
		ID      string
		Length  int
		BeginAt time.Time
		Set     int
		Winner  *proOPGGTeam
		Match   struct {
			ID                 string
			HomeTeam, AwayTeam proOPGGTeam
		}
	}
	League string // Set from the exact requested league, never a roster lookup.
}
type proOPGGCache struct {
	Revision          uint64
	ChampionID        int
	ReadAt, Attempted time.Time
	Rows              []proOPGGBuild
	FailedLeagues     int
	OverallFailed     bool
}

func (p *proRuneProvider) fetchOPGGBuilds(ctx context.Context, championID int, league string, page int) ([]proOPGGBuild, error) {
	if !p.provider.featureGates.enabled(featureGateOPGG) {
		return nil, errors.New("OP.GG source disabled")
	}
	select {
	case p.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-p.gate }()
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	body, _ := json.Marshal(map[string]any{"operationName": "ListBuilds", "query": proOPGGBuildQuery, "variables": map[string]any{"leagueId": league, "championId": strconv.Itoa(championID), "page": page, "limit": proOPGGPageSize}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+esportsOPGGHost+"/matches/graphql/__query__ListBuilds", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Apollo-Require-Preflight", "true")
	client := *p.provider.httpClient()
	client.Jar = nil
	client.Timeout = 8 * time.Second
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("esports redirect rejected") }
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OP.GG esports HTTP %d", res.StatusCode)
	}
	data, err := readLimited(res.Body, 2<<20)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Data   *struct{ Builds []proOPGGBuild }
		Errors []json.RawMessage
	}
	if err = json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	if payload.Data == nil || payload.Data.Builds == nil || len(payload.Errors) > 0 || len(payload.Data.Builds) > proOPGGPageSize {
		return nil, errors.New("invalid OP.GG builds response")
	}
	return payload.Data.Builds, nil
}

func (p *proRuneProvider) opggLeagueBuilds(ctx context.Context, championID int, league, label string, now time.Time) ([]proOPGGBuild, error) {
	rows := []proOPGGBuild{}
	seen := map[string]bool{}
	previous := now.Add(24 * time.Hour)
	cutoff := now.Add(-30 * 24 * time.Hour)
	for page := 0; page < proOPGGMaxPages; page++ {
		batch, err := p.fetchOPGGBuilds(ctx, championID, league, page)
		if err != nil {
			return nil, err
		}
		past := false
		for _, row := range batch {
			if row.Game.BeginAt.IsZero() || row.Game.BeginAt.After(previous) {
				return nil, errors.New("unordered OP.GG build dates")
			}
			previous = row.Game.BeginAt
			if !row.Game.BeginAt.After(cutoff) {
				past = true
				continue
			}
			if row.Game.BeginAt.After(now) || seen[row.ID] {
				continue
			}
			seen[row.ID] = true
			row.League = label
			rows = append(rows, row)
		}
		if past || len(batch) < proOPGGPageSize {
			return rows, nil
		}
	}
	return nil, errors.New("OP.GG builds pagination limit")
}

func (p *proRuneProvider) opggRecommend(ctx context.Context, championID int, position string, force bool) (proRuneResponse, error) {
	response := proRuneResponse{Pros: []gameplayRecommendationRune{}}
	meta, err := p.provider.championMetadataByID(ctx, championID)
	if err != nil {
		return response, err
	}
	now := time.Now()
	if p.now != nil {
		now = p.now()
	}
	key := "opgg-builds-" + strconv.Itoa(championID)
	p.mu.Lock()
	initialRevision := p.opggBuilds[championID].Revision
	p.mu.Unlock()
	err = p.gameResource(ctx, key, func() error {
		p.mu.Lock()
		old, exists := p.opggBuilds[championID]
		p.mu.Unlock()
		if !exists {
			p.readDisk(key, &old)
			if old.ChampionID != championID || old.Attempted.After(now) {
				old = proOPGGCache{}
			}
		}
		if force && exists && old.Revision != initialRevision || !force && !old.Attempted.IsZero() && now.Sub(old.Attempted) < proRuneTTL {
			p.mu.Lock()
			if p.opggBuilds == nil {
				p.opggBuilds = map[int]proOPGGCache{}
			}
			p.opggBuilds[championID] = old
			p.mu.Unlock()
			return nil
		}
		next := proOPGGCache{Revision: old.Revision + 1, ChampionID: championID, Attempted: now, ReadAt: now, Rows: []proOPGGBuild{}}
		var mu sync.Mutex
		failed := map[string]bool{}
		proParallelLimit(ctx, proOPGGLeagues, 3, func(league struct{ ID, Name string }) {
			rows, loadErr := p.opggLeagueBuilds(ctx, championID, league.ID, league.Name, now)
			mu.Lock()
			defer mu.Unlock()
			if loadErr != nil {
				failed[league.Name] = true
				return
			}
			next.Rows = append(next.Rows, rows...)
		})
		// Cancelled, unscheduled leagues count as failed, too.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		next.FailedLeagues = len(failed)
		next.OverallFailed = len(failed) == len(proOPGGLeagues)
		for _, row := range old.Rows {
			if failed[row.League] {
				next.Rows = append(next.Rows, row)
			}
		}
		if next.FailedLeagues > 0 {
			next.ReadAt = old.ReadAt
		}
		p.writeDisk(key, next)
		p.mu.Lock()
		if p.opggBuilds == nil {
			p.opggBuilds = map[int]proOPGGCache{}
		}
		p.opggBuilds[championID] = next
		p.mu.Unlock()
		return nil
	})
	p.mu.Lock()
	cached := p.opggBuilds[championID]
	writeFailed := p.cacheWriteFailed
	p.mu.Unlock()
	response.ReadAt = cached.ReadAt
	response.Stale = cached.FailedLeagues > 0 || writeFailed || cached.ReadAt.IsZero() || now.Sub(cached.ReadAt) > proRuneTTL
	if cached.FailedLeagues > 0 || err != nil {
		response.Reason = "network-unavailable"
	} else if writeFailed {
		response.Reason = "cache-write-failed"
	}
	response.Pros, response.TotalGames, response.Failed = proOPGGRecommendations(cached.Rows, championID, position, meta.NameZH, now, cached.ReadAt, response.Stale)
	if response.Failed > 0 {
		response.Stale = true
		if response.Reason == "" {
			response.Reason = "upstream-error"
		}
		for i := range response.Pros {
			response.Pros[i].RecordPartial = true
		}
	}
	if len(response.Pros) == 0 && response.Reason == "" {
		response.Reason = "no-sample"
	}
	if err != nil {
		return response, err
	}
	if cached.OverallFailed {
		return response, errors.New("OP.GG esports unavailable")
	}
	return response, nil
}

func proOPGGRecommendations(rows []proOPGGBuild, championID int, position, name string, now, readAt time.Time, stale bool) ([]gameplayRecommendationRune, int, int) {
	selected := []proOPGGBuild{}
	seen := map[string]bool{}
	failed := 0
	for _, row := range rows {
		if row.ChampionID != strconv.Itoa(championID) {
			continue
		}
		if !row.Game.BeginAt.After(now.Add(-30*24*time.Hour)) || row.Game.BeginAt.After(now) {
			continue
		}
		if row.ID == "" || seen[row.ID] {
			continue
		}
		seen[row.ID] = true
		if !proNumericID(row.Game.ID) || !proNumericID(row.Player.ID) || !proNumericID(row.Team.ID) || row.Perk == nil || len(row.Perk.Primary.Selections) != 4 || len(row.Perk.Sub.Selections) != 2 || row.Perk.Primary.Style <= 0 || row.Perk.Sub.Style <= 0 || row.Perk.Stats.Offense <= 0 || row.Perk.Stats.Flex <= 0 || row.Perk.Stats.Defense <= 0 || len(row.Items) == 0 || row.Player.NickName == "" || row.Team.Acronym == "" {
			failed++
			continue
		}
		selected = append(selected, row)
	}
	sort.SliceStable(selected, func(i, j int) bool {
		a, b := selected[i], selected[j]
		// Region belongs to the team, including at international events.
		ap, bp := proOPGGTeamRegion(a.Team.ID) != "", proOPGGTeamRegion(b.Team.ID) != ""
		if ap != bp {
			return ap
		}
		if !a.Game.BeginAt.Equal(b.Game.BeginAt) {
			return a.Game.BeginAt.After(b.Game.BeginAt)
		}
		al, bl := canonicalSpecialistPosition(a.Position) == canonicalSpecialistPosition(position), canonicalSpecialistPosition(b.Position) == canonicalSpecialistPosition(position)
		if al != bl {
			return al
		}
		return a.ID < b.ID
	})
	type record struct {
		games, wins int
		partial     bool
	}
	records := map[string]record{}
	games := map[string]bool{}
	for _, r := range selected {
		games[r.Game.ID] = true
		key := r.Team.ID + "/" + r.Player.ID
		v := records[key]
		if r.Game.Winner == nil || r.Game.Winner.ID == "" {
			v.partial = true
		} else {
			v.games++
			if r.Game.Winner.ID == r.Team.ID {
				v.wins++
			}
		}
		records[key] = v
	}
	result := []gameplayRecommendationRune{}
	counts := map[string]int{}
	for _, r := range selected {
		if len(result) >= 5 {
			break
		}
		key := r.Team.ID + "/" + r.Player.ID
		if counts[key] >= 5 {
			continue
		}
		counts[key]++
		complete, known := true, r.Game.Winner != nil && r.Game.Winner.ID != ""
		v := records[key]
		shards := []int64{r.Perk.Stats.Offense, r.Perk.Stats.Flex, r.Perk.Stats.Defense}
		perks := append(append(append([]int64{}, r.Perk.Primary.Selections...), r.Perk.Sub.Selections...), shards...)
		config := gameplayRecommendationRune{Key: "pro-opgg-" + r.ID, SourceKey: "pro", Title: r.Team.Acronym + " " + r.Player.NickName, ChampionID: int64(championID), ChampionName: name, PlayerName: r.Player.NickName, Position: map[string]string{"top": "上路", "jun": "打野", "mid": "中路", "adc": "下路", "sup": "辅助"}[r.Position], PrimaryStyleID: r.Perk.Primary.Style, SubStyleID: r.Perk.Sub.Style, SelectedPerkIDs: perks, StatModIDs: shards, ShardResolution: "explicit", SelectedComplete: &complete, ItemIDs: r.Items, PlayedAt: r.Game.BeginAt.UnixMilli(), WinKnown: &known, RecordGames: v.games, RecordWins: v.wins, RecordPartial: v.partial || stale, EventLabel: fmt.Sprintf("%s · %s · %s vs %s 第 %d 局", r.League, r.Game.BeginAt.UTC().Format("2006-01-02"), r.Game.Match.HomeTeam.Acronym, r.Game.Match.AwayTeam.Acronym, r.Game.Set)}
		if !readAt.IsZero() {
			config.CacheReadAt = readAt.UTC().Format(time.RFC3339)
		}
		if len(r.Spells) == 2 {
			config.Spell1ID, config.Spell2ID = r.Spells[0], r.Spells[1]
		}
		if known {
			win := r.Game.Winner.ID == r.Team.ID
			config.Win = &win
			if win {
				config.Result = "win"
			} else {
				config.Result = "loss"
			}
		}
		if r.OpponentGamePlayer != nil {
			o := r.OpponentGamePlayer
			config.OpponentChampionID, _ = strconv.ParseInt(o.ChampionID, 10, 64)
			config.OpponentPlayerName = strings.TrimSpace(o.Team.Acronym + " " + o.Player.NickName)
		}
		order := []string{}
		for _, skill := range r.SkillLevelUps {
			if skill.LevelUpType != "NORMAL" {
				continue
			}
			if skill.SkillSlot < 1 || skill.SkillSlot > 4 {
				order = nil
				break
			}
			order = append(order, []string{"Q", "W", "E", "R"}[skill.SkillSlot-1])
		}
		if len(order) > 0 && len(order) <= 18 {
			config.Build = &gameplayRecommendationBuild{SkillOrder: order}
		}
		result = append(result, config)
	}
	return result, len(games), failed
}
