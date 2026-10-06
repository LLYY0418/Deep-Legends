package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const arenaSquadFile = "arena-squad.json"
const arenaSquadMaxAge = 2 * time.Hour

// Only identity fields are persisted; no client token, roster order or inferred
// opponent grouping. A game ID and a two-hour expiry bound the record to one game.
type arenaSquadMember struct {
	PUUID        string `json:"puuid,omitempty"`
	SummonerID   int64  `json:"summonerId,omitempty"`
	SummonerName string `json:"summonerName,omitempty"`
	GameName     string `json:"gameName,omitempty"`
	TagLine      string `json:"tagLine,omitempty"`
}
type arenaSquadRecord struct {
	Scope      string             `json:"scope"`
	GameID     int64              `json:"gameId"`
	RecordedAt time.Time          `json:"recordedAt"`
	Members    []arenaSquadMember `json:"members"`
}

func (a *app) arenaSquadScope() string {
	a.mu.RLock()
	current, client := a.summoner, a.lcu
	a.mu.RUnlock()
	if current.PUUID == "" && current.SummonerID <= 0 {
		return ""
	}
	return a.storage.accountHash(current) + ":" + clientRiotPlatform(client) + ":" + clientTencentServerID(client)
}

func (a *app) persistArenaSquad(gameID int64) {
	if a == nil || a.storage == nil || gameID <= 0 {
		return
	}
	scope := a.arenaSquadScope()
	if scope == "" {
		return
	}
	a.arenaAlliesMu.Lock()
	defer a.arenaAlliesMu.Unlock()
	if len(a.arenaAllyPlayers) == 0 || len(a.arenaAllyPlayers) > 3 {
		return
	}
	record := arenaSquadRecord{Scope: scope, GameID: gameID, RecordedAt: time.Now().UTC()}
	for _, p := range a.arenaAllyPlayers {
		record.Members = append(record.Members, arenaSquadMember{p.PUUID, p.SummonerID, p.SummonerName, p.GameName, p.TagLine})
	}
	data, err := json.Marshal(record)
	if err == nil {
		err = writeLocalStoreFile(a.storage, arenaSquadFile, data)
	}
	if err != nil {
		a.recordDiagnostic(map[string]any{"event": "arena_squad_store_failed", "game_id": gameID, "operation": "write"})
	}
}

func (a *app) removeArenaSquadFile() {
	if err := os.Remove(filepath.Join(a.storage.root, arenaSquadFile)); err != nil && !os.IsNotExist(err) {
		a.recordDiagnostic(map[string]any{"event": "arena_squad_store_failed", "operation": "delete"})
	}
}
func (a *app) restoreArenaSquad(gameID int64) {
	if a == nil || a.storage == nil || gameID <= 0 {
		return
	}
	scope := a.arenaSquadScope()
	if scope == "" {
		return
	}
	a.arenaAlliesMu.Lock()
	defer a.arenaAlliesMu.Unlock()
	if a.arenaAllyGameID == gameID && len(a.arenaAllyPlayers) > 0 {
		return
	}
	data, err := readLocalStoreFile(a.storage, arenaSquadFile)
	if os.IsNotExist(err) {
		return
	}
	var record arenaSquadRecord
	if err != nil || len(data) > 16<<10 || json.Unmarshal(data, &record) != nil || record.GameID != gameID || record.Scope != scope || time.Since(record.RecordedAt) > arenaSquadMaxAge || record.RecordedAt.After(time.Now().Add(time.Minute)) || len(record.Members) == 0 || len(record.Members) > 3 {
		a.removeArenaSquadFile()
		return
	}
	a.arenaAllyKeys = make(map[string]struct{})
	a.arenaAllyPlayers = nil
	for _, p := range record.Members {
		player := lcuLivePlayer{PUUID: p.PUUID, SummonerID: p.SummonerID, SummonerName: p.SummonerName, GameName: p.GameName, TagLine: p.TagLine}
		a.arenaAllyPlayers = append(a.arenaAllyPlayers, player)
		for _, key := range arenaAllyIdentityKeys(player) {
			a.arenaAllyKeys[key] = struct{}{}
		}
	}
	a.arenaAllyGameID = gameID
	a.recordDiagnostic(map[string]any{"event": "arena_squad_restored", "game_id": gameID, "members": len(record.Members), "source": "disk"})
}
