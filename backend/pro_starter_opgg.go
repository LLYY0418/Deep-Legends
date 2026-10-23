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
	"time"
)

// Public GetGameByMatch/CoreGamePlayer schema verified against esports.op.gg
// on 2026-10-24. Purchase timestamps are milliseconds from game start.
const proOPGGOpeningQuery = `query GetGameByMatch($matchId:ID!,$set:Int){gameByMatch(matchId:$matchId,set:$set){id beginAt set players{id championId position team{id acronym} itemPurchases{type itemId timestamp}}}}`

type proOPGGOpeningGame struct {
	ID      string
	BeginAt time.Time
	Set     int
	Players []struct {
		ID, ChampionID, Position string
		Team                     proOPGGTeam
		ItemPurchases            []struct {
			Type      string
			ItemID    int64
			Timestamp int64
		}
	}
}
type proOPGGOpeningCache struct {
	Game   proOPGGOpeningGame
	At     time.Time
	Failed bool
}

func (p *proRuneProvider) opggOpeningGame(ctx context.Context, row proOPGGBuild) (proOPGGOpeningGame, error) {
	var game proOPGGOpeningGame
	if !proNumericID(row.Game.Match.ID) || row.Game.Set <= 0 {
		return game, errors.New("missing-match-metadata")
	}
	key := "opgg-opening-" + row.Game.Match.ID + "-" + strconv.Itoa(row.Game.Set)
	err := p.gameResource(ctx, key, func() (loadErr error) {
		p.mu.Lock()
		cached, ok := p.opggOpenings[key]
		p.mu.Unlock()
		if ok && cached.Failed && time.Since(cached.At) < 90*time.Second {
			return errors.New("opgg-opening-cooldown")
		}
		valid := func(g proOPGGOpeningGame) bool {
			return g.ID == row.Game.ID && g.Set == row.Game.Set && !g.BeginAt.IsZero() && g.BeginAt.Equal(row.Game.BeginAt) && len(g.Players) == 10
		}
		if ok && valid(cached.Game) {
			game = cached.Game
			return nil
		}
		if p.readDisk(key, &game) && valid(game) {
			p.mu.Lock()
			if p.opggOpenings == nil {
				p.opggOpenings = map[string]proOPGGOpeningCache{}
			}
			p.opggOpenings[key] = proOPGGOpeningCache{Game: game}
			p.mu.Unlock()
			return nil
		}
		defer func() {
			if errors.Is(loadErr, context.Canceled) {
				return
			}
			p.mu.Lock()
			defer p.mu.Unlock()
			if p.opggOpenings == nil {
				p.opggOpenings = map[string]proOPGGOpeningCache{}
			}
			if len(p.opggOpenings) >= 128 {
				for old := range p.opggOpenings {
					if old != key {
						delete(p.opggOpenings, old)
						break
					}
				}
			}
			p.opggOpenings[key] = proOPGGOpeningCache{Game: game, At: time.Now(), Failed: loadErr != nil}
		}()
		if !p.provider.featureGates.enabled(featureGateOPGG) {
			return errors.New("OP.GG source disabled")
		}
		select {
		case p.gate <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		defer func() { <-p.gate }()
		requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		body, _ := json.Marshal(map[string]any{"operationName": "GetGameByMatch", "query": proOPGGOpeningQuery, "variables": map[string]any{"matchId": row.Game.Match.ID, "set": row.Game.Set}})
		req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, "https://"+esportsOPGGHost+"/matches/graphql", bytes.NewReader(body))
		if err != nil {
			return err
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
			return err
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("OP.GG opening HTTP %d", res.StatusCode)
		}
		data, err := readLimited(res.Body, 2<<20)
		if err != nil {
			return err
		}
		var payload struct {
			Data   *struct{ GameByMatch *proOPGGOpeningGame }
			Errors []json.RawMessage
		}
		if err = json.Unmarshal(data, &payload); err != nil {
			return err
		}
		if payload.Data == nil || payload.Data.GameByMatch == nil || len(payload.Errors) > 0 || !valid(*payload.Data.GameByMatch) {
			return errors.New("invalid-opgg-opening-game")
		}
		game = *payload.Data.GameByMatch
		p.writeDisk(key, game)
		return nil
	})
	return game, err
}

func proOPGGOpeningItems(game proOPGGOpeningGame, row proOPGGBuild) ([]int64, error) {
	matched := -1
	for i, player := range game.Players {
		if player.ID != row.ID || player.ChampionID != row.ChampionID || proTeamCode(player.Team.Acronym) != proTeamCode(row.Team.Acronym) || canonicalSpecialistPosition(player.Position) != canonicalSpecialistPosition(row.Position) {
			continue
		}
		if matched >= 0 {
			return nil, errors.New("ambiguous-participant")
		}
		matched = i
	}
	if matched < 0 {
		return nil, errors.New("participant-not-found")
	}
	purchases := append(game.Players[matched].ItemPurchases[:0:0], game.Players[matched].ItemPurchases...)
	sort.SliceStable(purchases, func(i, j int) bool { return purchases[i].Timestamp < purchases[j].Timestamp })
	items := []int64{}
	for _, purchase := range purchases {
		if purchase.Type == "ITEM_PURCHASED" && purchase.Timestamp >= 0 && purchase.Timestamp <= 60000 && purchase.ItemID > 0 && !isRuneTrinket(purchase.ItemID) {
			items = append(items, purchase.ItemID)
			if len(items) == 6 {
				break
			}
		}
	}
	if len(items) == 0 {
		return nil, errors.New("opening-items-empty")
	}
	return items, nil
}
