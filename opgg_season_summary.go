package main

// OP.GG's player-summary sidebar queries a pre-aggregated ranked season. This
// never calls Riot, paginates match history or sums the sidebar's top champions.
import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Observed in the public JUGKlNG-kr summary component on 2026-09-08.
// If OP.GG changes this action, fail closed to the labelled recent sample.
const opggSeasonSummaryAction = "4028494596c44675d8e9f617b8f659312f3b678072"
const opggSeasonSummaryTTL = 10 * time.Minute

type opggSeasonSummary struct {
	Source    string                 `json:"source"`
	Season    string                 `json:"season"`
	SeasonID  int                    `json:"seasonId"`
	Queue     string                 `json:"queue"`
	Overall   gameplayAggregate      `json:"overall"`
	Champions []gameplayChampionStat `json:"champions"`
}
type opggSeasonEntry struct {
	at    time.Time
	value *opggSeasonSummary
	err   error
}
type opggSeasonFlight struct {
	done  chan struct{}
	value *opggSeasonSummary
	err   error
}
type opggSummaryMetadata struct {
	PUUID  string `json:"puuid"`
	Season struct {
		Label string `json:"display_value"`
		ID    int    `json:"season_id"`
	} `json:"season"`
	Queue string `json:"queueType"`
}

var opggFlightScript = regexp.MustCompile(`self\.__next_f\.push\((\[.*?\])\)</script>`)

func parseOPGGSummaryMetadata(data []byte, puuid string) (opggSummaryMetadata, error) {
	var stream strings.Builder
	for _, m := range opggFlightScript.FindAllSubmatch(data, -1) {
		var parts []json.RawMessage
		if json.Unmarshal(m[1], &parts) != nil || len(parts) != 2 {
			continue
		}
		var text string
		if json.Unmarshal(parts[1], &text) == nil {
			stream.WriteString(text)
		}
	}
	// Only the summary component's props carry all three fields together. Inspect
	// nested elements too, and reject conflicting seasons instead of taking the
	// first match in a Flight stream whose record order is not guaranteed.
	var selected opggSummaryMetadata
	var invalid bool
	for _, line := range strings.Split(stream.String(), "\n") {
		_, raw, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		var value any
		if json.Unmarshal([]byte(raw), &value) != nil {
			continue
		}
		walkOPGGPage(value, 0, func(node map[string]any) {
			if node["puuid"] != puuid || node["queueType"] == nil || node["season"] == nil {
				return
			}
			encoded, _ := json.Marshal(node)
			var props opggSummaryMetadata
			if json.Unmarshal(encoded, &props) != nil || props.Queue == "" || props.Season.ID <= 0 || !opggSeasonLabel.MatchString(props.Season.Label) {
				invalid = true
				return
			}
			if selected.PUUID != "" && selected.Season != props.Season {
				invalid = true
			}
			selected = props
		})
	}
	if !invalid && selected.PUUID != "" {
		return selected, nil
	}
	return opggSummaryMetadata{}, errors.New("OP.GG 未提供可确认的当前赛季")
}

var opggSeasonLabel = regexp.MustCompile(`^S20[0-9]{2}(?: S[1-3])?$`)

type opggSummaryRow struct {
	All         bool     `json:"is_all_champions"`
	Name        string   `json:"name"`
	Key         string   `json:"key"`
	Play        *int     `json:"play"`
	Win         *int     `json:"win"`
	Lose        *int     `json:"lose"`
	WinRate     *int     `json:"win_rate"`
	CS          *float64 `json:"cs"`
	CSPerMinute *float64 `json:"cs_per_min"`
	KDA         *struct {
		Ratio   *float64 `json:"kda"`
		Kills   *float64 `json:"avg_kill"`
		Deaths  *float64 `json:"avg_death"`
		Assists *float64 `json:"avg_assist"`
	} `json:"kda"`
}

func parseOPGGSeasonSummary(data []byte, meta opggSummaryMetadata, champions map[string]championMetadata) (*opggSeasonSummary, error) {
	// Follow the action result reference in row 0; do not search arbitrary rows
	// containing champion stats (the RSC response may include 20 match rosters).
	records := map[string]json.RawMessage{}
	for _, line := range bytes.Split(data, []byte("\n")) {
		id, raw, ok := bytes.Cut(line, []byte(":"))
		if ok {
			records[string(id)] = raw
		}
	}
	var root struct {
		Action string `json:"a"`
	}
	if json.Unmarshal(records["0"], &root) != nil || !strings.HasPrefix(root.Action, "$@") {
		return nil, errors.New("OP.GG 汇总响应结构已变化")
	}
	var rows []opggSummaryRow
	if json.Unmarshal(records[strings.TrimPrefix(root.Action, "$@")], &rows) != nil || len(rows) == 0 || len(rows) > 201 {
		return nil, errors.New("OP.GG 汇总暂不可用")
	}
	result := &opggSeasonSummary{Source: "OP.GG", Season: meta.Season.Label, SeasonID: meta.Season.ID, Queue: "RANKED", Champions: []gameplayChampionStat{}}
	allCount, topGames := 0, 0
	seen := map[int64]bool{}
	for _, row := range rows {
		if row.Play == nil || row.Win == nil || row.Lose == nil || row.WinRate == nil || row.CS == nil || row.CSPerMinute == nil || row.KDA == nil || row.KDA.Ratio == nil || row.KDA.Kills == nil || row.KDA.Deaths == nil || row.KDA.Assists == nil {
			return nil, errors.New("OP.GG 汇总字段不完整")
		}
		if *row.Play < 0 || *row.Win < 0 || *row.Lose < 0 || *row.Win+*row.Lose != *row.Play || *row.WinRate < 0 || *row.WinRate > 100 {
			return nil, errors.New("OP.GG 汇总场次无法核验")
		}
		aggregate := gameplayAggregate{Games: *row.Play, Wins: *row.Win, Losses: *row.Lose, WinRate: *row.WinRate, Kills: *row.KDA.Kills, Deaths: *row.KDA.Deaths, Assists: *row.KDA.Assists, KDA: *row.KDA.Ratio, CS: *row.CS, CSPerMinute: *row.CSPerMinute}
		if row.All {
			allCount++
			result.Overall = aggregate
			continue
		}
		champion, ok := champions[strings.ToLower(row.Key)]
		if !ok || champion.ID <= 0 || seen[int64(champion.ID)] {
			return nil, errors.New("OP.GG 英雄身份无法核验")
		}
		seen[int64(champion.ID)] = true
		name := champion.NameZH
		if name == "" {
			name = row.Name
		}
		result.Champions = append(result.Champions, gameplayChampionStat{ChampionID: int64(champion.ID), ChampionName: name, Games: aggregate.Games, Wins: aggregate.Wins, WinRate: aggregate.WinRate, Kills: aggregate.Kills, Deaths: aggregate.Deaths, Assists: aggregate.Assists, KDA: aggregate.KDA, CS: aggregate.CS, CSPerMinute: aggregate.CSPerMinute})
		topGames += aggregate.Games
	}
	if allCount != 1 || topGames > result.Overall.Games {
		return nil, errors.New("OP.GG 全部英雄总计无法核验")
	}
	return result, nil
}

func (a *app) fetchOPGGSeasonSummary(ctx context.Context, ref gameplayReference) (result *opggSeasonSummary, resultErr error) {
	started := time.Now()
	var pageMS, summaryMS int64
	stage := "page-request"
	defer func() {
		a.recordDiagnostic(map[string]any{"event": "opgg_season_summary_cost", "duration_ms": time.Since(started).Milliseconds(), "page_ms": pageMS, "summary_ms": summaryMS, "stage": stage, "success": resultErr == nil, "failure": supplementFailureCode(resultErr)})
	}()
	page, err := a.readOPGGPlayerPage(ctx, ref, http.MethodGet, "", nil)
	pageMS = time.Since(started).Milliseconds()
	if err != nil {
		return nil, err
	}
	stage = "page-identity-season"
	profile, err := parseOPGGPlayerPage(page, ref)
	if err != nil {
		return nil, err
	}
	meta, err := parseOPGGSummaryMetadata(page, profile.puuid)
	if err != nil {
		return nil, err
	}
	body, _ := json.Marshal([]map[string]any{{"locale": "zh-cn", "region": "kr", "puuid": meta.PUUID, "season_id": meta.Season.ID, "game_type": "RANKED"}})
	stage = "aggregate-request"
	payload, err := a.readOPGGPlayerPage(ctx, ref, http.MethodPost, opggSeasonSummaryAction, body)
	summaryMS = time.Since(started).Milliseconds() - pageMS
	if err != nil {
		return nil, err
	}
	stage = "champion-catalog"
	if err := a.champions.ensureChampionMetadata(ctx); err != nil {
		return nil, err
	}
	byKey := map[string]championMetadata{}
	a.champions.mu.Lock()
	for _, m := range a.champions.championMeta {
		byKey[strings.ToLower(m.Key)] = m
		byKey[strings.ToLower(m.Slug)] = m
	}
	a.champions.mu.Unlock()
	stage = "aggregate-validation"
	result, resultErr = parseOPGGSeasonSummary(payload, meta, byKey)
	if resultErr == nil {
		stage = "complete"
	}
	return result, resultErr
}

func (a *app) opggSeasonSummary(ctx context.Context, ref gameplayReference, force bool) (*opggSeasonSummary, error) {
	if a.opgg == nil || a.champions == nil || !a.champions.featureGates.enabled(featureGateOPGG) || strings.EqualFold(ref.Privacy, "PRIVATE") || !strings.EqualFold(ref.Region, riotRegionKR) || !validPlayerReference(ref.PlayerRef) || ref.GameName == "" || ref.TagLine == "" {
		return nil, errors.New("OP.GG 赛季汇总不可用")
	}
	key := sourceScopedKey(dataSourceOPGG, "season-summary:kr:"+ref.PlayerRef)
	a.opgg.mu.Lock()
	if a.opgg.seasons == nil {
		a.opgg.seasons = map[string]opggSeasonEntry{}
		a.opgg.seasonFlights = map[string]*opggSeasonFlight{}
	}
	cached, ok := a.opgg.seasons[key]
	ttl := opggSeasonSummaryTTL
	if cached.err != nil {
		ttl = 30 * time.Second
	}
	if ok && time.Since(cached.at) < ttl && (!force || time.Since(cached.at) < 15*time.Second) {
		a.opgg.mu.Unlock()
		return cached.value, cached.err
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
	// Bound unrelated accounts without making them wait behind slow upstreams.
	if len(a.opgg.seasonFlights) >= 4 {
		a.opgg.mu.Unlock()
		return nil, errors.New("OP.GG 赛季查询繁忙，请稍后重试")
	}
	flight := &opggSeasonFlight{done: make(chan struct{})}
	a.opgg.seasonFlights[key] = flight
	a.opgg.mu.Unlock()
	value, err := a.fetchOPGGSeasonSummary(ctx, ref)
	a.opgg.mu.Lock()
	if len(a.opgg.seasons) >= 64 {
		oldest := ""
		at := time.Now()
		for k, e := range a.opgg.seasons {
			if e.at.Before(at) {
				oldest, at = k, e.at
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

func (a *app) handleOPGGSeasonSummary(w http.ResponseWriter, r *http.Request) {
	var request struct {
		PlayerRef string `json:"playerRef"`
		Force     bool   `json:"force"`
	}
	if decodeJSONRequest(r, &request, 4<<10) != nil {
		http.Error(w, "请求无效", 400)
		return
	}
	ref, ok := a.resolveGameplayReferenceDetails(request.PlayerRef)
	if !ok || !strings.EqualFold(ref.Region, riotRegionKR) {
		http.Error(w, "玩家引用无效或已过期", 404)
		return
	}
	if strings.EqualFold(strings.TrimSpace(ref.Privacy), "PRIVATE") {
		http.Error(w, "私密玩家不读取第三方汇总", 403)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	value, err := a.opggSeasonSummary(ctx, ref, request.Force)
	if err != nil {
		http.Error(w, "OP.GG 本赛季汇总暂不可用，请用刷新战绩重试", 502)
		return
	}
	respondJSON(w, value)
}
