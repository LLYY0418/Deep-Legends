package main

// Read-only current-game adapters. No account refresh, recording or spectate
// commands are sent to OP.GG or the local client.
import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

const opggCurrentGameAction = "40f052435807841afcefdd3e993b4a019b4a1bf970" // getInGameInfo, public player page 2026-09-09

type currentGame struct {
	Status    string            `json:"status"` // active (KR roster), none, unsupported
	Source    string            `json:"source"`
	CheckedAt time.Time         `json:"checkedAt"`
	StartedAt *time.Time        `json:"startedAt,omitempty"`
	GameID    string            `json:"gameId,omitempty"`
	Queue     string            `json:"queue,omitempty"`
	Map       string            `json:"map,omitempty"`
	Teams     []currentGameTeam `json:"teams,omitempty"`
}
type currentGameTeam struct {
	MissingPlayers int                 `json:"missingPlayers,omitempty"`
	AverageLP      *int                `json:"averageLP,omitempty"`
	Side           string              `json:"side"`
	AverageRank    *gameplayRank       `json:"averageRank,omitempty"`
	Players        []currentGamePlayer `json:"players"`
}
type currentGamePlayer struct {
	gameplayPlayer
	ChampionID        int64               `json:"championId,omitempty"`
	ChampionName      string              `json:"championName,omitempty"`
	Spells            []int64             `json:"spells,omitempty"`
	Runes             []int64             `json:"runes,omitempty"`
	Rank              *gameplayRank       `json:"rank,omitempty"`
	PreferredPosition string              `json:"preferredPosition,omitempty"`
	Streak            string              `json:"streak,omitempty"`
	Recent            []currentGameRecent `json:"recent,omitempty"`
}
type currentGameRecent struct {
	ChampionID   int64   `json:"championId"`
	ChampionName string  `json:"championName"`
	Win          *bool   `json:"win,omitempty"`
	Spells       []int64 `json:"spells,omitempty"`
}
type currentGameEntry struct {
	playerRef string
	traceID   string
	at        time.Time
	value     *currentGame
	err       error
}
type currentGameFlight struct {
	traceID string
	done    chan struct{}
	value   *currentGame
	err     error
}
type currentGameStore struct {
	mu      sync.Mutex
	entries map[string]currentGameEntry
	flights map[string]*currentGameFlight
}

type opggCurrentIcon struct {
	ID int64 `json:"id"`
}
type opggCurrentTier struct {
	Tier     string `json:"tier"`
	Division int    `json:"division"`
	LP       *int   `json:"lp"`
}
type opggCurrentParticipant struct {
	PUUID        string            `json:"puuid"`
	GameName     string            `json:"game_name"`
	TagLine      string            `json:"tagline"`
	Level        int64             `json:"level"`
	ChampionKey  string            `json:"champion_key"`
	ChampionName string            `json:"champion_name"`
	Spells       []opggCurrentIcon `json:"spells"`
	Runes        struct {
		Primary   opggCurrentIcon `json:"primary_rune"`
		Secondary opggCurrentIcon `json:"primary_perk_style"`
	} `json:"runes"`
	League struct {
		Tier opggCurrentTier `json:"tier_info"`
	} `json:"league_stats"`
	Summary struct {
		Badges        []string `json:"badges"`
		RecentSummary struct {
			Position string `json:"most_position"`
		} `json:"recent_summary"`
		Recent []struct {
			Champion struct {
				Key  string `json:"key"`
				Name string `json:"name"`
			} `json:"champion"`
			Win    *bool             `json:"is_win"`
			Spells []opggCurrentIcon `json:"spells"`
		} `json:"recent_matches"`
	} `json:"summary"`
}
type opggCurrentTeam struct {
	Average opggCurrentTier          `json:"average_tier_info"`
	Players []opggCurrentParticipant `json:"participants"`
}

func currentRank(t opggCurrentTier, requireLP bool) *gameplayRank {
	tier := strings.ToUpper(t.Tier)
	if !strings.Contains("|IRON|BRONZE|SILVER|GOLD|PLATINUM|EMERALD|DIAMOND|MASTER|GRANDMASTER|CHALLENGER|", "|"+tier+"|") || tier == "" || requireLP && t.LP == nil {
		return nil
	}
	rank := &gameplayRank{Tier: tier}
	if t.Division >= 1 && t.Division <= 4 {
		rank.Division = []string{"", "I", "II", "III", "IV"}[t.Division]
	}
	if t.LP != nil {
		rank.LeaguePoints = *t.LP
	}
	return rank
}
func currentPosition(raw string) string {
	switch strings.ToUpper(raw) {
	case "TOP":
		return "top"
	case "JUNGLE":
		return "jungle"
	case "MID", "MIDDLE":
		return "middle"
	case "ADC", "BOTTOM":
		return "bottom"
	case "SUPPORT", "UTILITY":
		return "utility"
	}
	return ""
}
func currentActionResult(data []byte) (json.RawMessage, error) {
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
		return nil, errors.New("OP.GG 当前对局契约已变化")
	}
	value := records[strings.TrimPrefix(root.Action, "$@")]
	if len(value) == 0 {
		return nil, errors.New("OP.GG 当前对局结果缺失")
	}
	return value, nil
}
func (a *app) parseOPGGCurrentGame(data []byte, ref gameplayReference, now time.Time) (*currentGame, error) {
	raw, err := normalizedCurrentActionResult(data)
	if err != nil {
		return nil, err
	}
	return a.parseOPGGCurrentGameValue(raw, ref, now)
}

// Server Actions use the same Flight references/undefined values as page
// snapshots. Normalize only approved game fields; never resolve spectate code.
func normalizedCurrentActionResult(data []byte) (json.RawMessage, error) {
	raw, err := currentActionResult(data)
	if err != nil {
		return nil, err
	}
	p := &opggPlayerPage{rows: map[string]any{}}
	for _, line := range bytes.Split(data, []byte("\n")) {
		id, row, ok := bytes.Cut(line, []byte(":"))
		var value any
		if ok && json.Unmarshal(row, &value) == nil {
			p.rows[string(id)] = value
		}
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, errors.New("opgg-current-json")
	}
	if game, ok := value.(map[string]any); ok {
		selected := map[string]any{}
		for _, key := range []string{"game_id", "created_at", "game_map", "game_type", "summaryTeamsData"} {
			selected[key] = game[key]
		}
		if record, ok := game["record_info"].(map[string]any); ok {
			selected["record_info"] = map[string]any{"is_finished": record["is_finished"]}
		}
		value = selected
	}
	budget := 50000
	value, err = p.resolve(value, 0, &budget)
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func (a *app) parseOPGGCurrentGameValue(raw json.RawMessage, ref gameplayReference, now time.Time) (*currentGame, error) {
	result := &currentGame{Status: "none", Source: "OP.GG", CheckedAt: now}
	// Next Flight encodes the observed no-game undefined result as this exact string.
	if string(raw) == "null" || string(raw) == `"$undefined"` {
		return result, nil
	}
	var input struct {
		GameID    string    `json:"game_id"`
		CreatedAt time.Time `json:"created_at"`
		Map       string    `json:"game_map"`
		Type      struct {
			Key   string `json:"game_type"`
			Label string `json:"game_translate"`
		} `json:"game_type"`
		Teams struct {
			Blue opggCurrentTeam `json:"blueTeam"`
			Red  opggCurrentTeam `json:"redTeam"`
		} `json:"summaryTeamsData"`
		Record struct {
			Finished bool `json:"is_finished"`
		} `json:"record_info"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, errors.New("opgg-current-json")
	}
	if input.Record.Finished {
		return result, nil
	}
	// OP.GG may omit the encrypted spectator game ID while still returning a
	// fresh live roster. We navigate to the player's page, not that ID.
	if input.CreatedAt.IsZero() || input.Type.Key == "" || input.Map == "" {
		return nil, errors.New("opgg-current-required-fields")
	}
	if now.Sub(input.CreatedAt) > 6*time.Hour || input.CreatedAt.After(now.Add(time.Minute)) {
		return nil, errors.New("opgg-current-time")
	}
	if input.Map != "SUMMONERS_RIFT" && input.Map != "HOWLING_ABYSS" {
		result.Status = "unsupported"
		return result, nil
	}
	teams := []opggCurrentTeam{input.Teams.Blue, input.Teams.Red}
	// Exact queried PUUID must occur once in the current roster, never in a
	// historical match or another RSC component. An observed live response may
	// omit participants while their profiles load; keep verified rows, never invent
	// players or present a partial average as the whole team's average.
	seen := map[string]bool{}
	matched := 0
	for _, team := range teams {
		if len(team.Players) > 5 {
			return nil, errors.New("opgg-current-roster-size")
		}
		for _, p := range team.Players {
			if !validPlayerReference(p.PUUID) || seen[p.PUUID] {
				return nil, errors.New("opgg-current-roster-identity")
			}
			seen[p.PUUID] = true
			if p.PUUID == ref.PlayerRef {
				matched++
			}
		}
	}
	if matched != 1 {
		return nil, errors.New("opgg-current-target-mismatch")
	}
	byKey := map[string]championMetadata{}
	a.champions.mu.Lock()
	for _, m := range a.champions.championMeta {
		byKey[strings.ToLower(m.Key)] = m
		byKey[strings.ToLower(m.Slug)] = m
	}
	a.champions.mu.Unlock()
	proIndex := a.proIdentitySnapshot()
	for index, team := range teams {
		output := currentGameTeam{Side: []string{"blue", "red"}[index], AverageRank: currentRank(team.Average, false)}
		output.MissingPlayers = 5 - len(team.Players)
		if output.MissingPlayers > 0 {
			output.AverageRank = nil
		}
		for _, p := range team.Players {
			meta := byKey[strings.ToLower(p.ChampionKey)]
			if meta.ID <= 0 {
				return nil, errors.New("opgg-current-champion-identity")
			}
			name := meta.NameZH
			if name == "" {
				name = p.ChampionName
			}
			identity := gameplayReference{PlayerRef: p.PUUID, GameName: p.GameName, TagLine: p.TagLine, Region: "kr", OPGGIdentity: true}
			player := currentGamePlayer{gameplayPlayer: gameplayPlayer{PlayerRef: a.registerGameplayReferenceDetails(identity), GameName: p.GameName, TagLine: p.TagLine, DisplayName: p.GameName, SummonerLevel: p.Level, Region: "kr"}, ChampionID: int64(meta.ID), ChampionName: name, Rank: currentRank(p.League.Tier, true), PreferredPosition: currentPosition(p.Summary.RecentSummary.Position)}
			player.ProPlayer = a.matchProIdentity(proIndex, "current-game", identity)
			for _, spell := range p.Spells {
				if spell.ID > 0 && len(player.Spells) < 2 {
					player.Spells = append(player.Spells, spell.ID)
				}
			}
			for _, rune := range []opggCurrentIcon{p.Runes.Primary, p.Runes.Secondary} {
				if rune.ID > 0 {
					player.Runes = append(player.Runes, rune.ID)
				}
			}
			for _, badge := range p.Summary.Badges {
				if badge == "WIN_STREAK_BADGE" {
					player.Streak = "win"
				}
				if badge == "LOSE_STREAK_BADGE" {
					player.Streak = "loss"
				}
			}
			for _, recent := range p.Summary.Recent {
				if len(player.Recent) >= 10 {
					break
				}
				m := byKey[strings.ToLower(recent.Champion.Key)]
				if m.ID <= 0 {
					continue
				}
				game := currentGameRecent{ChampionID: int64(m.ID), ChampionName: m.NameZH, Win: recent.Win}
				for _, spell := range recent.Spells {
					if spell.ID > 0 && len(game.Spells) < 2 {
						game.Spells = append(game.Spells, spell.ID)
					}
				}
				player.Recent = append(player.Recent, game)
			}
			output.Players = append(output.Players, player)
		}
		// LP is comparable across the three apex tiers. Lower tiers reset LP
		// per division, so a raw LP mean there would misrepresent team strength.
		total, known := 0, len(output.Players) == 5
		for _, p := range output.Players {
			if p.Rank == nil || (p.Rank.Tier != "MASTER" && p.Rank.Tier != "GRANDMASTER" && p.Rank.Tier != "CHALLENGER") {
				known = false
				break
			}
			total += p.Rank.LeaguePoints
		}
		if known {
			average := (total + len(output.Players)/2) / len(output.Players)
			output.AverageLP = &average
		}
		result.Teams = append(result.Teams, output)
	}
	result.Status = "active"
	result.GameID = input.GameID
	result.Queue = input.Type.Label
	result.Map = map[string]string{"SUMMONERS_RIFT": "召唤师峡谷", "HOWLING_ABYSS": "嚎哭深渊"}[input.Map]
	result.StartedAt = &input.CreatedAt
	return result, nil
}
func (a *app) fetchOPGGCurrentGame(ctx context.Context, ref gameplayReference) (result *currentGame, resultErr error) {
	started := time.Now()
	stage := "source-gate"
	responseSource := "none"
	defer func() {
		status := "unavailable"
		rosterCounts := []int{}
		if result != nil {
			status = result.Status
			for _, team := range result.Teams {
				rosterCounts = append(rosterCounts, len(team.Players))
			}
		}
		a.recordDiagnostic(map[string]any{"event": "opgg_current_game_cost", "duration_ms": time.Since(started).Milliseconds(), "stage": stage, "status": status, "response_source": responseSource, "roster_counts": rosterCounts, "failure": supplementFailureCode(resultErr)})
	}()
	if a.champions == nil || !a.champions.featureGates.enabled(featureGateOPGG) {
		return nil, errors.New("OP.GG 不可用")
	}
	stage = "page-request"
	pageData, err := a.readOPGGPlayerPage(ctx, ref, http.MethodGet, "", nil)
	if err != nil {
		return nil, err
	}
	stage = "page-identity"
	page, err := parseOPGGPlayerPage(pageData, ref)
	if err != nil {
		return nil, err
	}
	ref.PlayerRef = page.puuid
	stage = "page-current-game"
	data, fresh, err := page.currentGame(time.Now())
	if err != nil {
		return nil, err
	}
	if !fresh {
		responseSource = "action"
		body, _ := json.Marshal([]map[string]string{{"locale": "zh-cn", "region": "kr", "puuid": page.puuid}})
		stage = "current-game-request"
		data, err = a.readOPGGPlayerPage(ctx, ref, http.MethodPost, opggCurrentGameAction, body)
		if err != nil {
			return nil, err
		}
	} else {
		responseSource = "page"
	}
	stage = "action-response"
	value, err := currentActionResult(data)
	if !fresh {
		value, err = normalizedCurrentActionResult(data)
	}
	if err != nil {
		return nil, err
	}
	if string(value) != `"$undefined"` && string(value) != "null" {
		stage = "champion-catalog"
		if err := a.champions.ensureChampionMetadata(ctx); err != nil {
			return nil, err
		}
	}
	stage = "roster-validation"
	result, resultErr = a.parseOPGGCurrentGameValue(value, ref, time.Now())
	if resultErr == nil {
		stage = "complete"
	}
	return result, resultErr
}

func (a *app) loadCurrentGame(ctx context.Context, ref gameplayReference) (*currentGame, error) {
	if !strings.EqualFold(ref.Region, "kr") || ref.ServerID != "" || a.champions == nil || !a.champions.featureGates.enabled(featureGateOPGG) {
		return nil, errors.New("韩服当前对局来源不可用")
	}
	key := sourceScopedKey("current-game", overviewSupplementCacheIdentity(ref))
	cache := &a.currentGames
	cache.mu.Lock()
	if cache.entries == nil {
		cache.entries = map[string]currentGameEntry{}
		cache.flights = map[string]*currentGameFlight{}
	}
	// Roster hydration may refresh the spelling of this same stable player.
	// Keep the original ten-second identity cache valid without a second page
	// read; name-only prefetches still join via the canonical Riot-ID key above.
	for cachedKey, entry := range cache.entries {
		if entry.playerRef == ref.PlayerRef && entry.playerRef != "" && time.Since(entry.at) < 10*time.Second {
			key = cachedKey
			break
		}
	}
	if entry, ok := cache.entries[key]; ok && time.Since(entry.at) < 10*time.Second && !currentGameManualRefresh(ctx) {
		cache.mu.Unlock()
		a.currentGameDiagnostic(ctx, "cache", "hit", map[string]any{"age_ms": time.Since(entry.at).Milliseconds(), "cached_error": diagnosticErrorKind(entry.err), "origin_trace": entry.traceID})
		return entry.value, entry.err
	}
	if f := cache.flights[key]; f != nil {
		cache.mu.Unlock()
		a.currentGameDiagnostic(ctx, "cache", "join-in-flight", map[string]any{"origin_trace": f.traceID})
		select {
		case <-ctx.Done():
			a.currentGameDiagnostic(ctx, "cache", "join-canceled", map[string]any{"origin_trace": f.traceID})
			return nil, ctx.Err()
		case <-f.done:
			a.currentGameDiagnostic(ctx, "cache", "join-completed", mergeDiagnosticFields(currentGameSummary(f.value), map[string]any{"origin_trace": f.traceID, "error_kind": diagnosticErrorKind(f.err)}))
			return f.value, f.err
		}
	}
	if len(cache.flights) >= 2 {
		cache.mu.Unlock()
		a.currentGameDiagnostic(ctx, "cache", "concurrency-limit", nil)
		return nil, errors.New("当前对局查询繁忙")
	}
	flight := &currentGameFlight{done: make(chan struct{}), traceID: currentGameTrace(ctx)}
	cache.flights[key] = flight
	cache.mu.Unlock()
	a.currentGameDiagnostic(ctx, "cache", "miss", map[string]any{"manual_refresh": currentGameManualRefresh(ctx)})
	value, err := a.fetchOPGGCurrentGame(ctx, ref)
	cache.mu.Lock()
	if len(cache.entries) >= 64 {
		for k := range cache.entries {
			delete(cache.entries, k)
			break
		}
	}
	if ctx.Err() != nil {
		value, err = nil, ctx.Err()
	}
	cacheable := !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
	if cacheable {
		cache.entries[key] = currentGameEntry{at: time.Now(), value: value, err: err, traceID: currentGameTrace(ctx), playerRef: ref.PlayerRef}
	}
	flight.value, flight.err = value, err
	delete(cache.flights, key)
	close(flight.done)
	cache.mu.Unlock()
	a.currentGameDiagnostic(ctx, "cache", "store-decision", mergeDiagnosticFields(currentGameSummary(value), map[string]any{"cacheable": cacheable, "error_kind": diagnosticErrorKind(err)}))
	return value, err
}
func (a *app) handleOverviewCurrentGame(w http.ResponseWriter, r *http.Request) {
	var input struct {
		GameName     string `json:"gameName"`
		TagLine      string `json:"tagLine"`
		Region       string `json:"region"`
		PlayerRef    string `json:"playerRef"`
		TraceID      string `json:"traceId"`
		ForceRefresh bool   `json:"forceRefresh"`
	}
	decodeErr := decodeJSONRequest(r, &input, 4096)
	traceID := input.TraceID
	if decodeErr != nil || !validFlowClientTrace(traceID) {
		traceID = newDiagnosticTrace("cg")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, currentGameTraceKey{}, traceID)
	ctx = context.WithValue(ctx, currentGameRefreshKey{}, input.ForceRefresh)
	w.Header().Set("X-Diagnostic-Trace", traceID)
	if decodeErr != nil {
		a.currentGameDiagnostic(ctx, "request", "invalid-payload", nil)
		http.Error(w, "请求无效", 400)
		return
	}
	ref, status := a.resolveOverviewSupplement(input.PlayerRef, input.GameName, input.TagLine, input.Region)
	if status != 0 {
		a.currentGameDiagnostic(ctx, "request", "reference-expired", nil)
		http.Error(w, "玩家引用已失效或查询参数不完整", status)
		return
	}
	if !strings.EqualFold(ref.Region, "kr") {
		http.Error(w, "仅支持韩服玩家当前对局", http.StatusBadRequest)
		return
	}
	if strings.EqualFold(strings.TrimSpace(ref.Privacy), "PRIVATE") {
		a.currentGameDiagnostic(ctx, "request", "privacy-blocked", nil)
		http.Error(w, "私密玩家不读取当前对局", 403)
		return
	}
	started := time.Now()
	a.currentGameDiagnostic(ctx, "request", "started", map[string]any{"region": ref.Region, "server_id": ref.ServerID, "manual_refresh": input.ForceRefresh, "target_private": strings.EqualFold(ref.Privacy, "PRIVATE")})
	result, err := a.loadCurrentGame(ctx, ref)
	if err == nil && result == nil {
		err = errors.New("当前对局来源返回空结果")
	}
	a.currentGameDiagnostic(ctx, "response", diagnosticStageError(err, "success"), mergeDiagnosticFields(currentGameSummary(result), map[string]any{"duration_ms": time.Since(started).Milliseconds()}))
	if err != nil {
		a.recordDiagnostic(map[string]any{"event": "overview_current_game", "region": ref.Region, "state": "failed"})
		http.Error(w, "当前对局暂时无法确认，请稍后重试", 502)
		return
	}
	rosterCounts := make([]int, 0, len(result.Teams))
	for _, team := range result.Teams {
		rosterCounts = append(rosterCounts, len(team.Players))
	}
	a.recordDiagnostic(map[string]any{"event": "overview_current_game", "diagnostic_schema": 4, "trace_id": traceID, "region": ref.Region, "state": result.Status, "source": result.Source, "roster_counts": rosterCounts})
	respondJSON(w, result)
}
