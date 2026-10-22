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

type liveSpellProbe struct {
	capture    string
	sample     int
	epoch      int
	lastTime   float64
	timeSample int
	players    map[string]int // Raw identities live only in this phase-local map.
	previous   map[int]string
	events     map[int64]bool
}

func newLiveSpellProbe() *liveSpellProbe {
	return &liveSpellProbe{capture: strconv.FormatInt(time.Now().UnixNano(), 36), lastTime: -1,
		players: map[string]int{}, previous: map[int]string{}, events: map[int64]bool{}}
}

func (p *liveSpellProbe) row(kind string) map[string]any {
	return map[string]any{"event": "live_spell_" + kind, "capture": p.capture,
		"sample": p.sample, "epoch": p.epoch, "game_time": p.lastTime, "game_time_sample": p.timeSample}
}

// Only enumerated protocol fields and numeric/boolean values leave the parser.
// Unknown keys can contain identities; record their count, not their spelling.
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
	}
	p.lastTime = value
	p.timeSample = p.sample
	return true
}

func (p *liveSpellProbe) playerRows(raw []byte) ([]map[string]any, bool) {
	var players []map[string]any
	if json.Unmarshal(raw, &players) != nil || players == nil || len(players) > 32 {
		return nil, false
	}
	rows := []map[string]any{}
	for index, player := range players {
		identity, _ := player["riotId"].(string)
		if identity == "" {
			identity, _ = player["summonerName"].(string)
		}
		stable := identity != ""
		if !stable {
			identity = "index:" + strconv.Itoa(index)
		}
		slot, exists := p.players[identity]
		if !exists {
			if len(p.players) >= 64 {
				return rows, false
			}
			slot = len(p.players) + 1
			p.players[identity] = slot
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
		if p.previous[slot] == string(encoded) {
			continue
		}
		row := p.row("player")
		row["player_slot"], row["list_index"], row["state"] = slot, index, state
		row["baseline"] = p.previous[slot] == ""
		p.previous[slot] = string(encoded)
		rows = append(rows, row)
	}
	return rows, true
}

func (p *liveSpellProbe) eventRows(raw []byte) ([]map[string]any, bool) {
	var data struct {
		Events []map[string]any `json:"Events"`
	}
	if json.Unmarshal(raw, &data) != nil || data.Events == nil || len(data.Events) > 4096 {
		return nil, false
	}
	rows := []map[string]any{}
	for _, event := range data.Events {
		id, ok := event["EventID"].(float64)
		if !ok || id < 0 || id > 1<<53 || id != float64(int64(id)) {
			continue
		}
		if p.events[int64(id)] {
			continue
		}
		if len(p.events) >= 4096 {
			return rows, false
		}
		row := p.row("event")
		row["name"], row["fields"] = liveSpellEventName(event["EventName"]), liveSpellFields(event)
		// Resolve actors only against this capture's roster; never persist names.
		for _, key := range []string{"KillerName", "VictimName", "CasterName", "PlayerName", "SummonerName"} {
			if name, ok := event[key].(string); ok && p.players[name] != 0 {
				row[key+"_slot"] = p.players[name]
			}
		}
		p.events[int64(id)] = true
		rows = append(rows, row)
		if len(rows) == 128 {
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
	emit(p.row("start"))
	defer func() {
		row := p.row("stop")
		row["reason"] = "phase-ended"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			row["reason"] = "budget"
		}
		emit(row)
	}()
	lastHealth := ""
	successes := map[string]int{}
	failures := map[string]int{}
	for ctx.Err() == nil {
		p.sample++
		health := map[string]any{}
		allOK := true
		for _, endpoint := range []string{"gamestats", "playerlist", "eventdata"} {
			requestCtx, done := context.WithTimeout(ctx, 1500*time.Millisecond)
			raw, status, err := fetch(requestCtx, endpoint)
			done()
			if ctx.Err() != nil {
				return
			}
			valid := false
			var rows []map[string]any
			if err == nil && status == http.StatusOK {
				switch endpoint {
				case "gamestats":
					valid = p.gameTime(raw)
				case "playerlist":
					rows, valid = p.playerRows(raw)
				case "eventdata":
					rows, valid = p.eventRows(raw)
				}
			}
			kind := "ok"
			if !valid {
				kind = mayhemSampleErrorKind(err, status, false)
				failures[endpoint]++
				allOK = false
			} else {
				successes[endpoint]++
			}
			health[endpoint] = kind
			for _, row := range rows {
				emit(row)
			}
		}
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
