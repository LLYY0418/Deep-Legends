package main

// OP.GG provides season champion/matchup aggregates. Its OP scores are NOT our
// Go scores; absent Go scores and tank-share stay nil instead of substituting
// OP scores or interpreting absolute damage_taken as a percentage.
import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type opggChampionRow struct {
	ID      int64    `json:"id"`
	Name    string   `json:"name"`
	Play    int      `json:"play"`
	Win     int      `json:"win"`
	Lose    int      `json:"lose"`
	WinRate float64  `json:"win_rate"`
	KP      *float64 `json:"kill_participation"`
	KDA     struct {
		Ratio   *float64 `json:"kda"`
		Kills   *float64 `json:"avg_kill"`
		Deaths  *float64 `json:"avg_death"`
		Assists *float64 `json:"avg_assist"`
	} `json:"kda"`
	DamagePerMinute *float64          `json:"damage_dealt_per_min"`
	DamageShare     *float64          `json:"damage_dealt_share"`
	ControlWards    *float64          `json:"vision_ward"`
	WardsPlaced     *float64          `json:"ward_placed"`
	WardsKilled     *float64          `json:"ward_kill"`
	CS              *float64          `json:"cs"`
	CSPerMinute     *float64          `json:"cs_per_min"`
	Gold            *float64          `json:"gold"`
	GoldPerMinute   *float64          `json:"gold_per_min"`
	DoubleKills     *float64          `json:"double_kill"`
	TripleKills     *float64          `json:"triple_kill"`
	QuadraKills     *float64          `json:"quadra_kill"`
	PentaKills      *float64          `json:"penta_kill"`
	Opponents       []opggChampionRow `json:"match_up_stats"`
}

func opggTableRow(raw opggChampionRow, names map[int64]string) (championTableRow, error) {
	if raw.Play < 0 || raw.Win < 0 || raw.Lose < 0 || raw.Win+raw.Lose != raw.Play || raw.WinRate < 0 || raw.WinRate > 100 {
		return championTableRow{}, errors.New("OP.GG 英雄场次无法核验")
	}
	r := championTableRow{ChampionID: raw.ID, ChampionName: championName(names, raw.ID), Games: raw.Play, Wins: raw.Win, Losses: raw.Lose, WinRate: raw.WinRate, Kills: raw.KDA.Kills, Deaths: raw.KDA.Deaths, Assists: raw.KDA.Assists, KDA: raw.KDA.Ratio, DamagePerMinute: raw.DamagePerMinute, ControlWards: raw.ControlWards, WardsPlaced: raw.WardsPlaced, WardsKilled: raw.WardsKilled, CS: raw.CS, CSPerMinute: raw.CSPerMinute, Gold: raw.Gold, GoldPerMinute: raw.GoldPerMinute, DoubleKills: raw.DoubleKills, TripleKills: raw.TripleKills, QuadraKills: raw.QuadraKills, PentaKills: raw.PentaKills}
	if raw.ID == 0 {
		r.ChampionName = "所有英雄"
	} else if raw.Name != "" && names[raw.ID] == "" {
		r.ChampionName = raw.Name
	}
	if raw.KP != nil {
		v := *raw.KP / 100
		r.KP = &v
	}
	if raw.DamageShare != nil {
		v := *raw.DamageShare / 100
		r.DamageShare = &v
	}
	seen := map[int64]bool{}
	for _, opp := range raw.Opponents {
		if opp.ID <= 0 || seen[opp.ID] {
			return r, errors.New("OP.GG 对位身份无法核验")
		}
		seen[opp.ID] = true
		row, err := opggTableRow(opp, names)
		if err != nil {
			return r, err
		}
		row.Opponents = nil
		r.Opponents = append(r.Opponents, row)
	}
	sort.Slice(r.Opponents, func(i, j int) bool {
		if r.Opponents[i].Games == r.Opponents[j].Games {
			return r.Opponents[i].ChampionID < r.Opponents[j].ChampionID
		}
		return r.Opponents[i].Games > r.Opponents[j].Games
	})
	if len(r.Opponents) > 40 {
		r.Opponents = r.Opponents[:40]
	}
	return r, nil
}
func parseOPGGChampionTable(page []byte, ref gameplayReference, queue string, names map[int64]string) (*opggSeasonSummary, error) {
	profile, err := parseOPGGPlayerPage(page, ref)
	if err != nil {
		return nil, err
	}
	var selected *opggSeasonSummary
	var parseErr error
	for _, value := range profile.rows {
		walkOPGGPage(value, 0, func(node map[string]any) {
			ctx, ok := node["recommendationRequestContext"].(map[string]any)
			if !ok || ctx["puuid"] != profile.puuid || node["gameType"] != queue {
				return
			}
			raw, _ := json.Marshal(node)
			var props struct {
				Year int
				Data struct {
					GameType string `json:"game_type"`
					SeasonID int    `json:"season_id"`
					Play     int
					Win      int
					Lose     int
					Rows     []opggChampionRow `json:"my_champion_stats"`
				}
			}
			if json.Unmarshal(raw, &props) != nil || props.Year != time.Now().Year() || props.Data.GameType != queue || props.Data.SeasonID <= 0 || len(props.Data.Rows) == 0 {
				parseErr = errors.New("OP.GG 当前赛季英雄统计不可核验")
				return
			}
			result := &opggSeasonSummary{Source: "OP.GG", Season: "S" + strconv.Itoa(props.Year), SeasonID: props.Data.SeasonID, Queue: queue, TableSupported: true}
			all := 0
			seen := map[int64]bool{}
			for _, row := range props.Data.Rows {
				r, e := opggTableRow(row, names)
				if e != nil {
					parseErr = e
					return
				}
				if seen[r.ChampionID] {
					parseErr = errors.New("OP.GG 英雄重复")
					return
				}
				seen[r.ChampionID] = true
				if r.ChampionID == 0 {
					all++
					result.TableOverall = r
					result.Overall = gameplayAggregate{Games: r.Games, Wins: r.Wins, Losses: r.Losses, WinRate: int(r.WinRate)}
					if r.Kills != nil {
						result.Overall.Kills = *r.Kills
					}
					if r.Deaths != nil {
						result.Overall.Deaths = *r.Deaths
					}
					if r.Assists != nil {
						result.Overall.Assists = *r.Assists
					}
					if r.KDA != nil {
						result.Overall.KDA = *r.KDA
					}
					continue
				}
				result.TableRows = append(result.TableRows, r)
				c := gameplayChampionStat{ChampionID: r.ChampionID, ChampionName: r.ChampionName, Games: r.Games, Wins: r.Wins, WinRate: int(r.WinRate)}
				if r.Kills != nil {
					c.Kills = *r.Kills
				}
				if r.Deaths != nil {
					c.Deaths = *r.Deaths
				}
				if r.Assists != nil {
					c.Assists = *r.Assists
				}
				if r.KDA != nil {
					c.KDA = *r.KDA
				}
				if r.CS != nil {
					c.CS = *r.CS
				}
				if r.CSPerMinute != nil {
					c.CSPerMinute = *r.CSPerMinute
				}
				result.Champions = append(result.Champions, c)
			}
			if all != 1 || result.Overall.Games != props.Data.Play || props.Data.Win != result.Overall.Wins || props.Data.Lose != result.Overall.Losses {
				parseErr = errors.New("OP.GG 全部英雄总计无法核验")
				return
			}
			sort.Slice(result.TableRows, func(i, j int) bool { return result.TableRows[i].Games > result.TableRows[j].Games })
			selected = result
		})
	}
	if parseErr != nil {
		return nil, parseErr
	}
	if selected == nil {
		return nil, errors.New("OP.GG 未提供英雄聚合")
	}
	return selected, nil
}
func (a *app) fetchOPGGChampionTable(ctx context.Context, ref gameplayReference, queue string) (*opggSeasonSummary, error) {
	if a.opgg == nil || a.champions == nil || !a.champions.featureGates.enabled(featureGateOPGG) || strings.EqualFold(ref.Privacy, "PRIVATE") || !isRiotRegion(ref.Region) || ref.GameName == "" || ref.TagLine == "" {
		return nil, errors.New("OP.GG 英雄统计不可用")
	}
	endpoint := "https://op.gg/zh-cn/lol/summoners/" + opggPlatform(ref.Region) + "/" + url.PathEscape(ref.GameName+"-"+ref.TagLine) + "/champions?queue_type=" + url.QueryEscape(queue)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, bytes.NewReader(nil))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	client := *a.champions.httpClient()
	client.Jar = nil
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return errors.New("opgg-champion-redirect") }
	started := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	a.recordDiagnostic(map[string]any{"event": "opgg_champion_table_cost", "duration_ms": time.Since(started).Milliseconds(), "http_status": resp.StatusCode, "queue": queue})
	if resp.StatusCode != 200 {
		return nil, errors.New("OP.GG 英雄统计请求失败")
	}
	data, err := readLimited(resp.Body, opggGamesResponseMax)
	if err != nil {
		return nil, err
	}
	return parseOPGGChampionTable(data, ref, queue, a.championNames())
}

// Reuse OP.GG's bounded singleflight cache; separate keys from sidebar summaries.
func (a *app) opggChampionTable(ctx context.Context, ref gameplayReference, queue string) (*opggSeasonSummary, error) {
	if a.opgg == nil {
		return nil, errors.New("OP.GG 不可用")
	}
	key := sourceScopedKey(dataSourceOPGG, "champion-table:"+queue+":"+overviewSupplementCacheIdentity(ref))
	a.opgg.mu.Lock()
	if a.opgg.seasons == nil {
		a.opgg.seasons = map[string]opggSeasonEntry{}
		a.opgg.seasonFlights = map[string]*opggSeasonFlight{}
	}
	entry, ok := a.opgg.seasons[key]
	ttl := opggSeasonSummaryTTL
	if entry.err != nil {
		ttl = 30 * time.Second
	}
	if ok && time.Since(entry.at) < ttl {
		a.opgg.mu.Unlock()
		return entry.value, entry.err
	}
	if flight := a.opgg.seasonFlights[key]; flight != nil {
		a.opgg.mu.Unlock()
		select {
		case <-flight.done:
			return flight.value, flight.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if len(a.opgg.seasonFlights) >= 4 {
		a.opgg.mu.Unlock()
		return nil, errors.New("OP.GG 查询繁忙")
	}
	flight := &opggSeasonFlight{done: make(chan struct{})}
	a.opgg.seasonFlights[key] = flight
	a.opgg.mu.Unlock()
	value, err := a.fetchOPGGChampionTable(ctx, ref, queue)
	a.opgg.mu.Lock()
	if len(a.opgg.seasons) >= 64 {
		oldest := ""
		at := time.Now()
		for k, e := range a.opgg.seasons {
			if e.at.Before(at) {
				oldest = k
				at = e.at
			}
		}
		delete(a.opgg.seasons, oldest)
	}
	if !errors.Is(err, context.Canceled) {
		a.opgg.seasons[key] = opggSeasonEntry{at: time.Now(), value: value, err: err}
	}
	flight.value, flight.err = value, err
	delete(a.opgg.seasonFlights, key)
	close(flight.done)
	a.opgg.mu.Unlock()
	return value, err
}
