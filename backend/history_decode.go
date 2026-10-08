package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
)

// Optional observations use one tolerant type across Riot/SGP and LCU.
// Invalid shapes/overflow remain absent, never an invented zero. All sources
// round decimals identically (math.Round, halves away from zero).
type lenientInt struct{ value *int }

func (v *lenientInt) UnmarshalJSON(data []byte) error {
	v.value = nil
	number := strings.TrimSpace(string(data))
	if len(number) > 0 && number[0] == '"' {
		var text string
		if json.Unmarshal(data, &text) != nil {
			return nil
		}
		number = strings.TrimSpace(text)
	}
	numeric, err := strconv.ParseFloat(number, 64)
	numeric = math.Round(numeric)
	if err != nil || math.IsNaN(numeric) || math.IsInf(numeric, 0) || numeric < 0 || numeric >= math.Exp2(strconv.IntSize-1) {
		return nil
	}
	integer := int(numeric)
	v.value = &integer
	return nil
}
func (v lenientInt) MarshalJSON() ([]byte, error) { return json.Marshal(v.value) }
func historyIntValue(v *lenientInt) *int {
	if v == nil {
		return nil
	}
	return v.value
}

type lenientChallenges struct {
	SoloKills                    *lenientInt `json:"soloKills"`
	KnockEnemyIntoTeamAndKill    *lenientInt `json:"knockEnemyIntoTeamAndKill"`
	KillsNearEnemyTurret         *lenientInt `json:"killsNearEnemyTurret"`
	KillsUnderOwnTurret          *lenientInt `json:"killsUnderOwnTurret"`
	MaxCsAdvantageOnLaneOpponent *lenientInt `json:"maxCsAdvantageOnLaneOpponent"`

	DragonTakedowns     *lenientInt `json:"dragonTakedowns"`
	BaronTakedowns      *lenientInt `json:"baronTakedowns"`
	RiftHeraldTakedowns *lenientInt `json:"riftHeraldTakedowns"`
}

func (v *lenientChallenges) UnmarshalJSON(data []byte) error {
	*v = lenientChallenges{}
	if !strings.HasPrefix(strings.TrimSpace(string(data)), "{") {
		return nil
	}
	type plain lenientChallenges
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return nil
	}
	*v = lenientChallenges(value)
	return nil
}

func historyParticipantError(err error, prefix string) error {
	var field *json.UnmarshalTypeError
	if errors.As(err, &field) {
		copy := *field
		copy.Field = prefix + copy.Field
		return &copy
	}
	return err
}

// If an unrelated optional payload (e.g. perks) has changed shape, retry with
// the identity/combat fields only. Core damage is never coerced or dropped.
func decodeSGPHistoryGame(data []byte) (*riotMatchInfo, error) {
	var info riotMatchInfo
	if err := json.Unmarshal(data, &info); err == nil {
		return &info, nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	var participants []map[string]json.RawMessage
	if err := json.Unmarshal(raw["participants"], &participants); err != nil {
		return nil, historyParticipantError(err, "participants.")
	}
	core := map[string]bool{}
	for _, key := range strings.Fields("participantId teamId puuid riotIdGameName riotIdTagline summonerName profileIcon championId champLevel summoner1Id summoner2Id spell1Id spell2Id teamPosition individualPosition kills deaths assists win gameEndedInEarlySurrender gameEndedInSurrender playerSubteamId subteamPlacement") {
		core[key] = true
	}
	for _, participant := range participants {
		for key := range participant {
			if !core[key] {
				delete(participant, key)
			}
		}
	}
	raw["participants"], _ = json.Marshal(participants)
	// Probe optional top-level fields independently so one changed metadata
	// shape cannot turn a core-valid match into a skip.
	for _, key := range []string{"gameCreation", "gameStartTimestamp", "gameDuration", "gameEndTimestamp", "gameMode", "gameType", "mapId", "teams"} {
		if value, exists := raw[key]; exists {
			single, _ := json.Marshal(map[string]json.RawMessage{key: value})
			var metadata riotMatchInfo
			if json.Unmarshal(single, &metadata) != nil {
				delete(raw, key)
			}
		}
	}
	minimal, _ := json.Marshal(raw)
	info = riotMatchInfo{}
	if err := json.Unmarshal(minimal, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func sgpGameDecodeDiagnostic(err error, payload ...[]byte) map[string]any {
	field, valueType := "", "invalid-json"
	var typed *json.UnmarshalTypeError
	if errors.As(err, &typed) {
		field = typed.Field
		valueType, _, _ = strings.Cut(typed.Value, " ")
	}
	// Error.Value for string includes no contents, but keep only known type enums.
	switch valueType {
	case "number", "string", "bool", "array", "object", "null":
	default:
		valueType = "invalid-json"
	}
	event := map[string]any{"event": "sgp_game_decode_failed", "count": 1, "field": field, "value_type": valueType}
	if len(payload) > 0 {
		data := bytes.TrimSpace(payload[0])
		event["payload_bytes"] = len(payload[0])
		event["first_byte_kind"], event["last_byte_kind"] = "empty", "empty"
		if len(data) > 0 {
			event["first_byte_kind"] = diagnosticJSONByteKind(data[0])
			event["last_byte_kind"] = diagnosticJSONByteKind(data[len(data)-1])
		}
	}
	return allowSGPGameDecodeDiagnostic(event)
}

// Report categories only, never a byte, substring, match ID or account value.
func diagnosticJSONByteKind(value byte) string {
	switch value {
	case '{':
		return "object_open"
	case '}':
		return "object_close"
	case '[':
		return "array_open"
	case ']':
		return "array_close"
	case '"':
		return "quote"
	default:
		if value >= '0' && value <= '9' {
			return "digit"
		}
		if value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' {
			return "letter"
		}
		return "other"
	}
}
