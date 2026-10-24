package main

// This probe records observations, never infers casts, readiness or fog of war.
// The phase context owns its lifetime, independently of live-page caching.
import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const liveSpellInterval = 2 * time.Second
const liveSpellDiagnosticSchema = 2

type liveSpellProbe struct {
	capture               string
	sample                int
	epoch                 int
	lastTime              float64
	timeSample            int
	players               map[string]int // Raw identities live only in this phase-local map.
	aliases               map[string]int
	roster                []int
	playerCount           int
	previous              map[int]string
	events                map[int64]bool
	rejectedEvents        map[[32]byte]bool
	rejectedLimitReported bool
	lastRejectedEvents    int
	lastLimit             string
	detailPrevious        map[string]liveSpellDetailState
	privateTerms          map[string]bool
	observation           int
}

func newLiveSpellProbe() *liveSpellProbe {
	return &liveSpellProbe{capture: strconv.FormatInt(time.Now().UnixNano(), 36), lastTime: -1,
		players: map[string]int{}, previous: map[int]string{}, events: map[int64]bool{},
		aliases:        map[string]int{},
		rejectedEvents: map[[32]byte]bool{},
		detailPrevious: map[string]liveSpellDetailState{}, privateTerms: map[string]bool{}}
}

func (p *liveSpellProbe) row(kind string) map[string]any {
	return map[string]any{"event": "live_spell_" + kind, "diagnostic_schema": liveSpellDiagnosticSchema, "capture": p.capture,
		"sample": p.sample, "epoch": p.epoch, "game_time": p.lastTime, "game_time_sample": p.timeSample,
		"clock_fresh": p.lastTime >= 0 && p.timeSample == p.sample}
}

// Retain the compatibility summary. Schema 2 records complete field shapes and
// controlled spell/state text separately in chunked detail observations.
func liveSpellFields(object map[string]any) map[string]any {
	result := map[string]any{}
	shapes := map[string]string{}
	for _, key := range []string{"cooldown", "cooldownRemaining", "remainingCooldown", "cooldownExpires",
		"cooldownEndTime", "cooldownStartTime", "castTime", "lastCastTime", "charges", "ammo", "spellId", "spellID",
		"isReady", "isOnCooldown", "isVisible", "visible", "isInFogOfWar", "isDead", "EventID", "EventTime", "SpellID", "SpellSlot"} {
		switch value := object[key].(type) {
		case float64, bool:
			result[key] = value
		default:
			if value, exists := object[key]; exists {
				shapes[key] = liveSpellValueType(value)
			}
		}
	}
	for _, key := range []string{"displayName", "rawDisplayName", "rawDescription"} {
		if value, exists := object[key]; exists {
			shapes[key] = liveSpellValueType(value)
		}
	}
	result["field_shapes"] = shapes
	result["field_count"] = len(object)
	return result
}

func liveSpellValueType(value any) string {
	switch value.(type) {
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case nil:
		return "null"
	case []any:
		return "array"
	default:
		return "object"
	}
}

func liveSpellDescriptor(object map[string]any) string {
	value, _ := object["rawDisplayName"].(string)
	for _, name := range []string{"SummonerFlash", "SummonerDot", "SummonerHeal", "SummonerBarrier", "SummonerExhaust",
		"SummonerHaste", "SummonerBoost", "SummonerTeleport", "SummonerSmite", "SummonerSnowball", "SummonerMana",
		"SummonerPoroRecall", "SummonerPoroThrow", "Summoner_UltBookPlaceholder", "Summoner_UltBookSmitePlaceholder"} {
		if value == name || value == "GeneratedTip_SummonerSpell_"+name+"_DisplayName" {
			return name
		}
	}
	return "unknown"
}

func liveSpellEventName(value any) string {
	name, _ := value.(string)
	const allowed = "|GameStart|MinionsSpawning|Ace|ChampionKill|FirstBlood|TurretKilled|FirstBrick|InhibKilled|InhibRespawningSoon|InhibRespawned|DragonKill|HeraldKill|BaronKill|Multikill|GameEnd|SpellCast|SummonerSpellCast|SummonerSpellUsed|AbilityCast|"
	if name != "" && strings.Contains(allowed, "|"+name+"|") {
		return name
	}
	return "unknown"
}

func (p *liveSpellProbe) gameTime(raw []byte) bool {
	var data map[string]any
	if json.Unmarshal(raw, &data) != nil {
		return false
	}
	value, ok := data["gameTime"].(float64)
	if !ok || value < 0 {
		return false
	}
	if p.lastTime >= 0 && value < p.lastTime {
		p.epoch++
		p.players, p.previous, p.events = map[string]int{}, map[int]string{}, map[int64]bool{}
		p.detailPrevious = map[string]liveSpellDetailState{}
		p.aliases, p.roster = map[string]int{}, nil
		p.rejectedEvents, p.rejectedLimitReported = map[[32]byte]bool{}, false
	}
	p.lastTime = value
	p.timeSample = p.sample
	return true
}

func (p *liveSpellProbe) playerRows(raw []byte) ([]map[string]any, bool) {
	p.lastLimit = ""
	p.roster, p.playerCount = nil, 0
	var players []map[string]any
	if json.Unmarshal(raw, &players) != nil || players == nil {
		return nil, false
	}
	p.playerCount = len(players)
	if len(players) > 32 {
		return p.limitRows("playerlist-size", 32), true
	}
	rows := []map[string]any{}
	p.roster = []int{}
	// Actor aliases describe the current successful roster, not prior ambiguity
	// or names that have disappeared since the previous sample.
	p.aliases = map[string]int{}
	identities := map[string]int{}
	// Collect the whole roster before emitting the first player's skill text.
	for _, player := range players {
		p.rememberPrivateFields(player)
		identities[liveSpellPlayerIdentity(player)]++
	}
	for index, player := range players {
		identity := liveSpellPlayerIdentity(player)
		stable := identity != "" && identities[identity] == 1
		key := "identity:" + identity
		if !stable {
			key = "index:" + strconv.Itoa(index)
		}
		slot, exists := p.players[key]
		if !exists {
			if len(p.players) >= 64 {
				return append(rows, p.limitRows("tracked-players", 64)...), true
			}
			slot = len(p.players) + 1
			p.players[key] = slot
		}
		subject := "player:" + strconv.Itoa(slot)
		scope := "capture"
		if !stable {
			// List position is only meaningful within this sample. Never turn a
			// reordered anonymous roster into a cross-player state change.
			scope = "sample"
			delete(p.previous, slot)
			delete(p.detailPrevious, "playerlist:"+subject)
		}
		p.roster = append(p.roster, slot)
		for _, key := range []string{"riotId", "summonerName", "riotIdGameName"} {
			if alias, ok := player[key].(string); ok && alias != "" {
				if previous, exists := p.aliases[alias]; !stable || exists && previous != slot {
					p.aliases[alias] = 0
				} else {
					p.aliases[alias] = slot
				}
			}
		}
		state := liveSpellFields(player)
		state["identity_matched"] = stable
		team, _ := player["team"].(string)
		if team != "ORDER" && team != "CHAOS" {
			team = "unknown"
		}
		state["team"] = team
		spells, present := player["summonerSpells"].(map[string]any)
		state["spells_present"] = present
		for _, key := range []string{"summonerSpellOne", "summonerSpellTwo"} {
			spell, ok := spells[key].(map[string]any)
			fields := liveSpellFields(spell)
			fields["present"], fields["spell"] = ok, liveSpellDescriptor(spell)
			state[key] = fields
		}
		encoded, _ := json.Marshal(state)
		details := p.detailRows("playerlist", subject, player, !stable)
		for _, detail := range details {
			detail["identity_scope"], detail["identity_matched"] = scope, stable
		}
		if p.previous[slot] == string(encoded) {
			rows = append(rows, details...)
			continue
		}
		row := p.row("player")
		row["player_slot"], row["list_index"], row["state"] = slot, index, state
		row["identity_scope"] = scope
		row["baseline"] = p.previous[slot] == ""
		p.previous[slot] = string(encoded)
		rows = append(rows, row)
		rows = append(rows, details...)
	}
	return rows, true
}

func liveSpellPlayerIdentity(player map[string]any) string {
	identity, _ := player["riotId"].(string)
	if identity == "" {
		identity, _ = player["summonerName"].(string)
	}
	return identity
}

func (p *liveSpellProbe) limitRows(reason string, limit int) []map[string]any {
	p.lastLimit = reason
	row := p.row("limit")
	row["reason"], row["limit"] = reason, limit
	return []map[string]any{row}
}

func (p *liveSpellProbe) eventRows(raw []byte) ([]map[string]any, bool) {
	p.lastRejectedEvents = 0
	p.lastLimit = ""
	var data struct {
		Events []map[string]any `json:"Events"`
	}
	if json.Unmarshal(raw, &data) != nil || data.Events == nil {
		return nil, false
	}
	if len(data.Events) > 4096 {
		return p.limitRows("event-list-size", 4096), true
	}
	rows := []map[string]any{}
	processed := 0
	var metadata map[string]any
	_ = json.Unmarshal(raw, &metadata)
	delete(metadata, "Events")
	if len(metadata) > 0 {
		rows = append(rows, p.detailRows("eventdata", "metadata", metadata, false)...)
	}
	for index, event := range data.Events {
		p.rememberPrivateFields(event)
		id, ok := event["EventID"].(float64)
		if !ok || id < 0 || id > 1<<53 || id != float64(int64(id)) {
			p.lastRejectedEvents++
			rows = append(rows, p.rejectedEventRows(index, event)...)
			continue
		}
		if p.events[int64(id)] {
			continue
		}
		if len(p.events) >= 4096 {
			return append(rows, p.limitRows("tracked-events", 4096)...), true
		}
		row := p.row("event")
		row["name"], row["fields"] = liveSpellEventName(event["EventName"]), liveSpellFields(event)
		// Resolve actors only against this capture's roster; never persist names.
		for _, key := range []string{"KillerName", "VictimName", "CasterName", "PlayerName", "SummonerName"} {
			if name, ok := event[key].(string); ok && p.aliases[name] != 0 {
				row[key+"_slot"] = p.aliases[name]
			}
		}
		p.events[int64(id)] = true
		rows = append(rows, row)
		rows = append(rows, p.detailRows("eventdata", "event:"+strconv.FormatInt(int64(id), 10), event, true)...)
		processed++
		if processed == 128 {
			break
		} // Remaining events are picked up next poll.
	}
	return rows, true
}

func fetchLiveSpellEndpoint(ctx context.Context, endpoint string) ([]byte, int, error) {
	switch endpoint {
	case "gamestats", "playerlist", "eventdata":
	default:
		return nil, 0, errors.New("invalid endpoint")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://127.0.0.1:2999/liveclientdata/"+endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	client := *liveClientDataHTTPClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if len(raw) > 1<<20 {
		return nil, response.StatusCode, errors.New("response too large")
	}
	return raw, response.StatusCode, err
}

func (a *app) runLiveSpellDiagnostics(ctx context.Context) {
	runLiveSpellProbe(ctx, fetchLiveSpellEndpoint, a.recordDiagnostic, mayhemWait)
}

func runLiveSpellProbe(parent context.Context, fetch func(context.Context, string) ([]byte, int, error), emit func(map[string]any), wait func(context.Context, time.Duration) bool) {
	ctx, cancel := context.WithTimeout(parent, 90*time.Minute)
	defer cancel()
	p := newLiveSpellProbe()
	successes := map[string]int{}
	failures := map[string]int{}
	start := p.row("start")
	start["interval_ms"], start["failure_interval_ms"] = liveSpellInterval.Milliseconds(), 10000
	start["detail_chunk_bytes"], start["detail_max_fields"] = liveSpellDetailChunkBytes, liveSpellDetailMaxFields
	start["detail_max_depth"], start["detail_max_array"], start["detail_max_text_bytes"] = liveSpellDetailMaxDepth, liveSpellDetailMaxArray, liveSpellDetailMaxText
	start["detail_values"] = "spell-objects-and-cooldown-cast-visibility-candidates;other-player-fields-schema-only"
	start["sources"] = []string{"gamestats", "playerlist", "eventdata"}
	start["response_max_bytes"], start["request_timeout_ms"], start["budget_seconds"] = 1<<20, 1500, 5400
	start["max_players"], start["max_tracked_players"], start["max_events"], start["event_batch"] = 32, 64, 4096, 128
	start["max_rejected_event_details"] = 128
	start["anonymous_identity_scope"] = "sample"
	emit(start)
	defer func() {
		row := p.row("stop")
		row["reason"] = "phase-ended"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			row["reason"] = "budget"
		}
		row["successes"], row["failures"] = cloneSpellCounts(successes), cloneSpellCounts(failures)
		row["tracked_players"], row["tracked_events"], row["observations"] = len(p.players), len(p.events), p.observation
		emit(row)
	}()
	lastHealth := ""
	for ctx.Err() == nil {
		p.sample++
		health := map[string]any{}
		requests := map[string]any{}
		sampleStarted := time.Now()
		allOK := true
		for _, endpoint := range []string{"gamestats", "playerlist", "eventdata"} {
			p.lastLimit = ""
			started := time.Now()
			requestCtx, done := context.WithTimeout(ctx, 1500*time.Millisecond)
			raw, status, err := fetch(requestCtx, endpoint)
			done()
			if ctx.Err() != nil {
				requests[endpoint] = map[string]any{"result": "cancelled", "http_status": status, "response_bytes": len(raw), "started_at": started.UTC(), "finished_at": time.Now().UTC()}
				sampleRow := p.row("sample")
				sampleRow["requests"], sampleRow["completed"], sampleRow["all_ok"] = requests, false, false
				emit(sampleRow)
				return
			}
			valid := false
			var rows []map[string]any
			if err == nil && status == http.StatusOK {
				switch endpoint {
				case "gamestats":
					valid = p.gameTime(raw)
					if valid {
						var object map[string]any
						_ = json.Unmarshal(raw, &object)
						rows = p.detailRows(endpoint, "game", object, false)
					}
				case "playerlist":
					rows, valid = p.playerRows(raw)
				case "eventdata":
					rows, valid = p.eventRows(raw)
				}
			}
			kind := "ok"
			if !valid {
				kind = mayhemSampleErrorKind(err, status, false)
				if err == nil && status == http.StatusOK {
					kind = "invalid-payload"
					var invalid any
					if json.Unmarshal(raw, &invalid) == nil {
						rows = append(rows, p.detailRows(endpoint, "invalid-payload", map[string]any{"response": invalid}, false)...)
					}
				}
				if err != nil && err.Error() == "response too large" {
					kind = "response-too-large"
				}
				failures[endpoint]++
				allOK = false
			} else if p.lastLimit != "" {
				kind = "capture-limit"
				failures[endpoint]++
				allOK = false
			} else {
				successes[endpoint]++
			}
			health[endpoint] = kind
			request := map[string]any{"result": kind, "parse_valid": valid, "http_status": status, "response_bytes": len(raw), "started_at": started.UTC(), "finished_at": time.Now().UTC(), "duration_ms": time.Since(started).Milliseconds(), "rows": len(rows)}
			if p.lastLimit != "" {
				request["coverage_limit"] = p.lastLimit
			}
			if endpoint == "playerlist" && valid {
				request["player_count"], request["captured_player_count"] = p.playerCount, len(p.roster)
				request["player_slots"] = append([]int{}, p.roster...)
			}
			if endpoint == "eventdata" {
				request["rejected_events"] = p.lastRejectedEvents
			}
			requests[endpoint] = request
			for _, row := range rows {
				emit(row)
			}
		}
		sampleRow := p.row("sample")
		sampleRow["requests"], sampleRow["all_ok"], sampleRow["duration_ms"] = requests, allOK, time.Since(sampleStarted).Milliseconds()
		sampleRow["completed"] = true
		emit(sampleRow)
		encoded, _ := json.Marshal(health)
		if string(encoded) != lastHealth || p.sample%15 == 0 {
			row := p.row("health")
			row["endpoints"], row["successes"], row["failures"] = health, cloneSpellCounts(successes), cloneSpellCounts(failures)
			row["tracked_players"], row["tracked_events"] = len(p.players), len(p.events)
			emit(row)
			lastHealth = string(encoded)
		}
		delay := liveSpellInterval
		if !allOK {
			delay = 10 * time.Second
		}
		if !wait(ctx, delay) {
			return
		}
	}
}

func cloneSpellCounts(source map[string]int) map[string]int {
	copy := make(map[string]int, len(source))
	for k, v := range source {
		copy[k] = v
	}
	return copy
}
