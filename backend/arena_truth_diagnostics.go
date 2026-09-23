package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// These identities never leave backend memory. No array-order inference feeds UI.
type arenaObservedIdentity struct {
	PUUID string
	Keys  []string
}

func arenaObservedPlayer(p lcuLivePlayer) arenaObservedIdentity {
	keys := []string{}
	for _, key := range arenaAllyIdentityKeys(p) {
		if strings.HasPrefix(key, "name:") {
			keys = append(keys, strings.TrimPrefix(key, "name:"))
		}
	}
	return arenaObservedIdentity{PUUID: p.PUUID, Keys: arenaExactNameKeys(keys)}
}

// When a Riot ID is available, never also match its ambiguous base name.
func arenaExactNameKeys(keys []string) []string {
	full := []string{}
	normalized := []string{}
	for _, key := range keys {
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" {
			continue
		}
		normalized = append(normalized, key)
		if name, tag, ok := strings.Cut(key, "#"); ok && name != "" && tag != "" {
			full = append(full, key)
		}
	}
	if len(full) > 0 {
		return full
	}
	return normalized
}
func arenaSameIdentity(a, b arenaObservedIdentity) bool {
	if a.PUUID != "" && b.PUUID != "" {
		return a.PUUID == b.PUUID
	}
	for _, left := range a.Keys {
		if slices.Contains(b.Keys, left) {
			return true
		}
	}
	return false
}

func (a *app) rememberArenaObservation(client *LCUClient, current Summoner, response *gameplayLiveResponse, allies []lcuLivePlayer, snapshot liveClientSnapshot) {
	r := arenaInference{client: client, gameID: response.GameID, queueID: response.QueueID, serverID: clientTencentServerID(client), selfPUUID: current.PUUID, source: "observed-roster", squadSize: arenaSquadSize(response.QueueID)}
	if r.gameID == 0 {
		return
	}
	for _, p := range response.Players {
		r.sessionIdentities = append(r.sessionIdentities, arenaObservedPlayer(lcuLivePlayer{PUUID: p.reference.PlayerRef, SummonerName: p.DisplayName, GameName: p.GameName, TagLine: p.TagLine}))
	}
	for _, keys := range snapshot.OrderedIdentities {
		r.playerlistIdentities = append(r.playerlistIdentities, arenaExactNameKeys(keys))
	}
	for _, p := range allies {
		r.mySquadIdentities = append(r.mySquadIdentities, arenaObservedPlayer(p))
	}
	r.playerCount = len(r.sessionIdentities)
	a.arenaTruth.mu.Lock()
	defer a.arenaTruth.mu.Unlock()
	for i, old := range a.arenaTruth.records {
		if old.client == client && old.gameID == r.gameID {
			if len(old.sessionIdentities) > len(r.sessionIdentities) {
				r.sessionIdentities = old.sessionIdentities
			}
			if len(old.playerlistIdentities) > len(r.playerlistIdentities) {
				r.playerlistIdentities = old.playerlistIdentities
			}
			r.checked, r.noneReported, r.truthObserved = old.checked, old.noneReported, old.truthObserved
			a.arenaTruth.records[i] = r
			return
		}
	}
	a.arenaTruth.records = append(a.arenaTruth.records, r)
	if len(a.arenaTruth.records) > 5 {
		a.arenaTruth.records = slices.Clone(a.arenaTruth.records[len(a.arenaTruth.records)-5:])
	}
}

func arenaTruthIdentity(p arenaGroupTruthParticipant) arenaObservedIdentity {
	keys := []string{p.SummonerName}
	if name, tag, ok := strings.Cut(p.RiotID, "#"); ok && name != "" && tag != "" {
		keys = append(keys, p.RiotID)
	}
	return arenaObservedIdentity{PUUID: p.PUUID, Keys: arenaExactNameKeys(keys)}
}
func arenaTruthIndex(identity arenaObservedIdentity, participants []arenaGroupTruthParticipant) int {
	found := -1
	for i, p := range participants {
		if arenaSameIdentity(identity, arenaTruthIdentity(p)) {
			if found >= 0 {
				return -1
			}
			found = i
		}
	}
	return found
}
func arenaTruthSequence(identities []arenaObservedIdentity, participants []arenaGroupTruthParticipant) ([]int64, bool) {
	ids := make([]int64, len(identities))
	seen := map[int]bool{}
	complete := len(ids) > 0
	for i, identity := range identities {
		index := arenaTruthIndex(identity, participants)
		if index < 0 || seen[index] {
			complete = false
			continue
		}
		seen[index] = true
		ids[i] = participants[index].SubteamID
		complete = complete && ids[i] > 0
	}
	return ids, complete
}

// This is a post-game hypothesis check, never an assignment for live players.
func arenaTruthOrder(ids []int64, size int) bool {
	if size < 2 || len(ids) < size*2 || len(ids)%size != 0 {
		return false
	}
	seen := map[int64]bool{}
	for i, id := range ids {
		if id <= 0 {
			return false
		}
		if i%size == 0 {
			if seen[id] {
				return false
			}
			seen[id] = true
		} else if ids[i-1] != id {
			return false
		}
	}
	return true
}
func (a *app) emitArenaObservationTruth(r *arenaInference, truth *arenaGroupTruthInput) {
	if truth.Source == "none" && (r.noneReported || r.truthObserved) {
		return
	}
	if truth.Source != "none" && len(truth.Participants) > 0 {
		r.truthObserved = true
	}
	session, sessionComplete := arenaTruthSequence(r.sessionIdentities, truth.Participants)
	playerlistIDs := make([]arenaObservedIdentity, len(r.playerlistIdentities))
	for i, keys := range r.playerlistIdentities {
		playerlistIDs[i] = arenaObservedIdentity{Keys: keys}
	}
	playerlist, playerlistComplete := arenaTruthSequence(playerlistIDs, truth.Participants)
	selfSubteam := int64(0)
	for _, p := range truth.Participants {
		if p.PUUID != "" && p.PUUID == r.selfPUUID {
			selfSubteam = p.SubteamID
		}
	}
	marked := map[int]bool{}
	correct := selfSubteam > 0 && len(r.mySquadIdentities) == r.squadSize
	for _, identity := range r.mySquadIdentities {
		index := arenaTruthIndex(identity, truth.Participants)
		if index < 0 || marked[index] {
			correct = false
			continue
		}
		marked[index] = true
		if truth.Participants[index].SubteamID != selfSubteam {
			correct = false
		}
	}
	actual := 0
	for i, p := range truth.Participants {
		if selfSubteam > 0 && p.SubteamID == selfSubteam {
			actual++
			if !marked[i] {
				correct = false
			}
		}
	}
	correct = correct && actual == r.squadSize
	complete := len(truth.Participants) >= r.squadSize*2 && len(truth.Participants) >= max(len(session), len(playerlist)) && len(truth.Participants)%r.squadSize == 0 && selfSubteam > 0
	for _, p := range truth.Participants {
		complete = complete && p.SubteamID > 0
	}
	// Missing identities are inconclusive, not a successful verification.
	complete = complete && ((sessionComplete && len(session) == len(truth.Participants)) || (playerlistComplete && len(playerlist) == len(truth.Participants)))
	ids := session
	if len(playerlist) > 0 {
		ids = playerlist
	}
	// Retain the old block diagnostics, strictly as a post-game hypothesis check.
	blocks := []int64{}
	identityBlocks, selfBlock := true, 0
	if r.squadSize > 0 {
		for start := 0; start+r.squadSize <= len(ids); start += r.squadSize {
			id := ids[start]
			for _, other := range ids[start : start+r.squadSize] {
				if other != id {
					id = 0
					break
				}
			}
			blocks = append(blocks, id)
			identityBlocks = identityBlocks && id == int64(len(blocks))
			if id > 0 && id == selfSubteam {
				selfBlock = len(blocks)
			}
		}
	}
	a.appendDiagnosticEvent(map[string]any{"partition_match": arenaTruthOrder(ids, r.squadSize), "block_to_subteam": blocks, "block_to_subteam_identity": arenaTruthOrder(ids, r.squadSize) && identityBlocks, "self_block": selfBlock, "verified_by_allies": r.verified, "event": "arena_group_truth_check", "truth_source": truth.Source, "game_id": r.gameID, "queue_id": r.queueID, "source": r.source, "player_count": len(ids), "squad_size": r.squadSize, "subteam_id_by_index": ids, "session_subteam_id_by_index": session, "playerlist_subteam_id_by_index": playerlist, "playerlist_order_is_squad_order": playerlistComplete && arenaTruthOrder(playerlist, r.squadSize), "session_order_is_squad_order": sessionComplete && arenaTruthOrder(session, r.squadSize), "my_squad_correct": correct, "self_subteam_id": selfSubteam, "conclusive": truth.Source != "none" && complete})
	if truth.Source == "none" {
		r.noneReported = true
	} else if complete {
		r.checked = true
	}
}

func diagnosticCellIDs(entries []json.RawMessage, unhiddenOnly bool) []int64 {
	ids := []int64{}
	for _, raw := range entries {
		var p struct {
			CellID     *int64 `json:"cellId"`
			Visibility string `json:"nameVisibilityType"`
		}
		if json.Unmarshal(raw, &p) == nil && p.CellID != nil && (!unhiddenOnly || strings.EqualFold(p.Visibility, "UNHIDDEN")) {
			ids = append(ids, *p.CellID)
		}
	}
	return ids
}

// Unknown object keys can themselves contain player identities. Only protocol
// field names are logged; unknown keys are collapsed, never copied into paths.
func arenaShapeKey(key string) string {
	const fields = "|activePlayer|allPlayers|events|Events|gameData|abilities|fullRunes|runes|items|scores|championStats|E|Q|R|W|passive|displayName|gameName|tagLine|riotId|riotIdGameName|riotIdTagLine|summonerName|championName|rawChampionName|rawSkinName|skinID|isBot|isDead|respawnTimer|team|position|level|itemID|slot|count|canUse|consumable|price|rawDescription|kills|deaths|assists|creepScore|wardScore|currentGold|resourceType|abilityPower|armor|attackDamage|attackRange|attackSpeed|critChance|critDamage|currentHealth|maxHealth|healthRegenRate|lifeSteal|magicLethality|magicPenetrationFlat|magicPenetrationPercent|magicResist|moveSpeed|physicalLethality|physicalPenetrationFlat|physicalPenetrationPercent|resourceMax|resourceRegenRate|resourceValue|spellVamp|id|rawDisplayName|keystone|primaryRuneTree|secondaryRuneTree|statRunes|generalRunes|EventID|EventName|EventTime|KillerName|VictimName|Assisters|TurretKilled|InhibKilled|DragonType|Stolen|Acer|AcingTeam|Result|gameMode|gameTime|mapName|mapNumber|mapTerrain|summonerSpells|summonerSpellOne|summonerSpellTwo|subteamId|subTeamId|playerSubteamId|squadId|squadName|teamId|teamName|"
	if strings.Contains(fields, "|"+key+"|") {
		return key
	}
	return "<unknown-key>"
}
func liveClientAllGameDataShape(raw []byte) map[string]any {
	arrays := map[string]any{}
	var walk func(json.RawMessage, string, int)
	walk = func(raw json.RawMessage, path string, depth int) {
		if depth > 8 {
			return
		}
		var object map[string]json.RawMessage
		if json.Unmarshal(raw, &object) == nil && object != nil {
			for key, value := range object {
				walk(value, path+"."+arenaShapeKey(key), depth+1)
			}
			return
		}
		var entries []json.RawMessage
		if json.Unmarshal(raw, &entries) != nil || entries == nil {
			return
		}
		fields := map[string]map[string]int{}
		keys := []string{}
		for _, key := range diagnosticKeyUnion(entries) {
			safe := arenaShapeKey(key)
			if fields[safe] == nil {
				fields[safe] = map[string]int{}
				keys = append(keys, safe)
			}
			for shape, count := range diagnosticFieldShapeCounts(entries, key) {
				fields[safe][shape] += count
			}
		}
		slices.Sort(keys)
		// Multiple nested arrays sharing a wildcard path are retained, not overwritten.
		elementShapes := map[string]int{}
		for _, entry := range entries {
			elementShapes[diagnosticJSONShape(entry, false)]++
		}
		array := map[string]any{"length": len(entries), "element_keys": keys, "element_shapes": elementShapes, "field_shapes": fields}
		previous, _ := arrays[path].([]map[string]any)
		arrays[path] = append(previous, array)
		for _, entry := range entries {
			walk(entry, path+"[]", depth+1)
		}
	}
	walk(raw, "$", 0)
	top := []string{}
	for _, key := range diagnosticKeySet(raw) {
		top = append(top, arenaShapeKey(key))
	}
	slices.Sort(top)
	return map[string]any{"top_level_keys": slices.Compact(top), "arrays": arrays}
}

func (a *app) fetchLiveClientAllGameData(ctx context.Context) ([]byte, int, error) {
	if a.liveClientAllGameData != nil {
		return a.liveClientAllGameData(ctx)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, liveClientAllGameDataURL, nil)
	if err != nil {
		return nil, 0, err
	}
	response, err := liveClientDataHTTPClient.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	return raw, response.StatusCode, err
}

func (a *app) sampleArenaAllGameData(ctx context.Context, gameID int64) {
	if gameID == 0 || !claimBoundedDiagnosticKey(&a.liveClientAllGameDataMu, &a.liveClientAllGameDataKeys, strconv.FormatInt(gameID, 10), 32) {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	raw, status, err := a.fetchLiveClientAllGameData(ctx)
	payload := liveClientAllGameDataShape(raw)
	payload["event"] = "live_client_allgamedata_shape"
	payload["game_id"] = gameID
	payload["http_status"] = status
	payload["success"] = err == nil && status == http.StatusOK && json.Valid(raw)
	a.appendDiagnosticEvent(payload)
}
func (a *app) recordArenaMissingSession(current Summoner, response *gameplayLiveResponse, snapshot liveClientSnapshot) {
	session := []arenaObservedIdentity{}
	for _, p := range response.Players {
		session = append(session, arenaObservedPlayer(lcuLivePlayer{PUUID: p.reference.PlayerRef, GameName: p.GameName, TagLine: p.TagLine, SummonerName: p.DisplayName}))
	}
	self := arenaObservedPlayer(lcuLivePlayer{PUUID: current.PUUID, GameName: current.GameName, TagLine: current.TagLine, SummonerName: current.DisplayName})
	missing := []map[string]any{}
	for index, keys := range snapshot.OrderedIdentities {
		identity := arenaObservedIdentity{Keys: arenaExactNameKeys(keys)}
		found := false
		for _, p := range session {
			found = found || arenaSameIdentity(identity, p)
		}
		if !found {
			missing = append(missing, map[string]any{"index": index, "is_current": arenaSameIdentity(identity, self)})
		}
	}
	a.recordDiagnostic(map[string]any{"event": "session_missing_from_playerlist", "game_id": response.GameID, "session_count": len(session), "playerlist_count": len(snapshot.OrderedIdentities), "missing": missing})
}

// Match-v5 consumers already fetched this detail. Never add a network request,
// and never compare a KR match to a CN observation with the same numeric ID.
func (a *app) checkArenaRiotMatchTruth(match *riotMatch) {
	if match == nil || !strings.HasPrefix(match.Metadata.MatchID, "KR_") {
		return
	}
	truth := arenaGroupTruthFromRiot(&match.Info, "riot-match-v5")
	if !isArenaQueue(truth.QueueID, truth.GameMode) {
		return
	}
	a.arenaTruth.mu.Lock()
	defer a.arenaTruth.mu.Unlock()
	for i := range a.arenaTruth.records {
		r := &a.arenaTruth.records[i]
		if !r.checked && strings.EqualFold(r.serverID, "KR") && r.gameID == truth.GameID && r.queueID == truth.QueueID {
			a.emitArenaObservationTruth(r, truth)
		}
	}
}

func arenaChampOrderConclusive(comparable, squadSize int) bool { return comparable >= squadSize*2 }
