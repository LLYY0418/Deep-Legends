package main

import "strings"

const (
	queueFilterCapabilityUnknown     = "unknown"
	queueFilterCapabilitySupported   = "supported"
	queueFilterCapabilityUnsupported = "unsupported"
)

func (a *app) setQueueFilterCapability(serverID, filter, state string) {
	if a == nil {
		return
	}
	serverID = strings.ToUpper(strings.TrimSpace(serverID))
	filter = strings.ToLower(strings.TrimSpace(filter))
	if serverID == "" || filter == "" || state == queueFilterCapabilityUnknown {
		return
	}
	a.queueFilterCapabilityMu.Lock()
	if a.queueFilterCapabilities == nil {
		a.queueFilterCapabilities = make(map[string]string)
	}
	a.queueFilterCapabilities[serverID+"|"+filter] = state
	a.queueFilterCapabilityMu.Unlock()
}

func (a *app) queueFilterCapability(serverID, filter string) string {
	if a == nil {
		return queueFilterCapabilityUnknown
	}
	serverID = strings.ToUpper(strings.TrimSpace(serverID))
	filter = strings.ToLower(strings.TrimSpace(filter))
	a.queueFilterCapabilityMu.RLock()
	state := a.queueFilterCapabilities[serverID+"|"+filter]
	if state == "" {
		state = a.queueFilterCapabilities[serverID+"|tag"]
	}
	a.queueFilterCapabilityMu.RUnlock()
	if state == "" {
		return queueFilterCapabilityUnknown
	}
	return state
}

type queueGroupResponse struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	ModeGroup          string `json:"modeGroup"`
	Filter             string `json:"filter"`
	RecommendationMode string `json:"recommendationMode"`
	AugmentSource      string `json:"augmentSource"`
}

func queueGroupsForClient() []queueGroupResponse {
	groups := make([]queueGroupResponse, 0, len(supportedQueueDefinitions))
	for _, definition := range supportedQueueDefinitions {
		augmentSource := ""
		switch definition.ModeGroup {
		case "arena":
			augmentSource = "arena"
		case "hextech-aram":
			augmentSource = "hextech"
		}
		groups = append(groups, queueGroupResponse{
			ID: definition.ID, Name: definition.Name, ModeGroup: definition.ModeGroup,
			Filter: definition.Filter, RecommendationMode: definition.RecommendationMode,
			AugmentSource: augmentSource,
		})
	}
	return groups
}

// queueDefinition is the canonical queue classification used by the backend.
// Names and semantics come from Riot queue metadata plus LeagueAkari's 2026-07
// Tencent SGP queue snapshot. Unknown queues stay "other" instead of being
// guessed into ranked or ARAM.
type queueDefinition struct {
	ID                 int64
	Name               string
	ModeGroup          string
	Filter             string
	RecommendationMode string
	SquadSize          int
}

var supportedQueueDefinitions = []queueDefinition{
	{400, "匹配模式（征召）", "match", "more:match", "ranked", 0},
	{420, "单排/双排", "solo", "solo", "ranked", 0},
	{430, "匹配模式（盲选）", "match", "more:match", "ranked", 0},
	{440, "灵活组排", "flex", "flex", "ranked", 0},
	{450, "极地大乱斗", "aram", "more:aram", "aram", 0},
	{480, "快速模式", "match", "more:match", "ranked", 0},
	{490, "匹配模式（快速）", "match", "more:match", "ranked", 0},
	{700, "召唤师峡谷冠军杯赛", "clash", "more:clash", "ranked", 0},
	{720, "极地大乱斗冠军杯赛", "clash", "more:clash", "aram", 0},
	{820, "人机对战（新手）", "bots", "more:bots", "bots", 0},
	{830, "人机对战（入门）", "bots", "more:bots", "bots", 0},
	{840, "人机对战（新手）", "bots", "more:bots", "bots", 0},
	{850, "人机对战（一般）", "bots", "more:bots", "bots", 0},
	{860, "人机对战", "bots", "more:bots", "bots", 0},
	{870, "人机对战", "bots", "more:bots", "bots", 0},
	{880, "人机对战", "bots", "more:bots", "bots", 0},
	{890, "人机对战", "bots", "more:bots", "bots", 0},
	{900, "无限火力", "urf", "more:urf", "urf", 0},
	{930, "极地大乱斗冠军杯赛", "aram", "more:aram", "aram", 0},
	{950, "末日人工智能（投票）", "doombots", "more:doombots", "bots", 0},
	{960, "末日人工智能", "doombots", "more:doombots", "bots", 0},
	{1300, "极限闪击", "nexus-blitz", "more:nexus-blitz", "nexus-blitz", 0},
	{1700, "斗魂竞技场", "arena", "arena", "arena", 2},
	{1710, "斗魂竞技场", "arena", "arena", "arena", 2},
	{1750, "斗魂竞技场", "arena", "arena", "arena", 3},
	{1900, "无限火力", "urf", "more:urf", "urf", 0},
	{2300, "海克斯大乱斗", "hextech-aram", "hextech-aram", "hextech-aram", 0},
	{2400, "海克斯大乱斗", "hextech-aram", "hextech-aram", "hextech-aram", 0},
	{3100, "召唤师峡谷自选自定义", "custom", "excluded", "ranked", 0},
	{3220, "极地大乱斗", "aram", "more:aram", "aram", 0},
	{3270, "海克斯大乱斗", "hextech-aram", "hextech-aram", "hextech-aram", 0},
	// Tencent SGP snapshots identify these queues as the RUBY family. Riot's
	// localized queue catalog supplies the visible name when the client is
	// connected; keeping them explicit prevents CLASSIC/map-11 fallthrough.
	{4210, "RUBY", "other", "more:special", "unsupported", 0},
	{4220, "RUBY", "other", "more:special", "unsupported", 0},
	{4240, "RUBY 试炼 1", "other", "more:special", "unsupported", 0},
	{4250, "RUBY 试炼 2", "other", "more:special", "unsupported", 0},
	{4260, "RUBY 试炼 3", "other", "more:special", "unsupported", 0},
}

var supportedQueueDefinitionsByID = func() map[int64]queueDefinition {
	result := make(map[int64]queueDefinition, len(supportedQueueDefinitions))
	for _, definition := range supportedQueueDefinitions {
		result[definition.ID] = definition
	}
	return result
}()

func supportedQueueDefinition(queueID int64) (queueDefinition, bool) {
	definition, ok := supportedQueueDefinitionsByID[queueID]
	return definition, ok
}

type matchHistoryFilterSpec struct {
	Key           string
	Tags          []string
	AllowedQueues map[int64]struct{}
	AllowedGroups map[string]struct{}
}

func matchHistoryFilterFor(value string) matchHistoryFilterSpec {
	value = normalizeGameplayMatchFilter(value)
	spec := matchHistoryFilterSpec{Key: value}
	switch value {
	case "solo":
		spec.Tags = []string{"q_420"}
		spec.AllowedQueues = int64Set(420)
	case "flex":
		spec.Tags = []string{"q_440"}
		spec.AllowedQueues = int64Set(440)
	case "more:aram":
		spec.Tags = []string{"q_450", "q_930", "q_3220"}
		spec.AllowedQueues = int64Set(450, 930, 3220)
	case "more:match":
		spec.Tags = []string{"q_400", "q_430", "q_480", "q_490"}
		spec.AllowedGroups = stringSet("match")
	case "more:bots":
		spec.Tags = []string{"q_820", "q_830", "q_840", "q_850", "q_860", "q_870", "q_880", "q_890"}
		spec.AllowedGroups = stringSet("bots")
	case "more:urf":
		spec.Tags = []string{"q_900", "q_1900"}
		spec.AllowedGroups = stringSet("urf")
	case "more:clash":
		spec.Tags = []string{"q_700", "q_720"}
		spec.AllowedGroups = stringSet("clash")
	case "more:nexus-blitz":
		spec.Tags = []string{"q_1300"}
		spec.AllowedGroups = stringSet("nexus-blitz")
	case "more:doombots":
		spec.Tags = []string{"q_950", "q_960"}
		spec.AllowedGroups = stringSet("doombots")
	case "hextech-aram":
		spec.Tags = []string{"q_2300", "q_2400", "q_3270"}
		spec.AllowedQueues = int64Set(2300, 2400, 3270)
	case "arena":
		spec.Tags = []string{"q_1700", "q_1710", "q_1750"}
		spec.AllowedQueues = int64Set(1700, 1710, 1750)
	case "ranked":
		spec.Tags = []string{"ranked"}
		spec.AllowedQueues = int64Set(420, 440)
	}
	return spec
}

func normalizeGameplayMatchFilter(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "all", "solo", "flex", "ranked", "hextech-aram", "arena",
		"more:aram", "more:hextech-qualifier", "more:match", "more:hextech-classic",
		"more:bots", "more:urf", "more:clash", "more:nexus-blitz", "more:doombots", "more:special":
		return value
	default:
		return "all"
	}
}

func int64Set(values ...int64) map[int64]struct{} {
	result := make(map[int64]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func stringSet(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func (spec matchHistoryFilterSpec) available() bool {
	return len(spec.Tags) > 0
}

func (spec matchHistoryFilterSpec) accepts(info *riotMatchInfo) bool {
	if info == nil {
		return false
	}
	if len(spec.AllowedQueues) > 0 {
		_, ok := spec.AllowedQueues[info.QueueID]
		return ok
	}
	if len(spec.AllowedGroups) > 0 {
		_, ok := spec.AllowedGroups[queueModeGroupFor(info.QueueID, info.GameMode, info.MapID)]
		return ok
	}
	return true
}

func (spec matchHistoryFilterSpec) acceptsAll(infos []*riotMatchInfo) bool {
	for _, info := range infos {
		if !spec.accepts(info) {
			return false
		}
	}
	return true
}

// Arena queue IDs belong only in the shared registration table.
func isArenaQueue(queueID int64, gameMode string) bool {
	definition, ok := supportedQueueDefinition(queueID)
	return (ok && definition.ModeGroup == "arena") || strings.EqualFold(strings.TrimSpace(gameMode), "CHERRY") || strings.EqualFold(strings.TrimSpace(gameMode), "ARENA")
}

// Queue-specific champ-select compatibility belongs in the queue registry.
// Legacy probing is limited to the observed custom draft implementation.
var champSelectQueueCompatibility = map[int64]struct {
	group, mode string
	legacy      bool
}{
	3110: {group: "practice", legacy: true},
	3200: {group: "aram"},
	420:  {group: "ranked"}, 440: {group: "ranked"},
	400: {group: "normal", mode: "CLASSIC"}, 700: {group: "normal", mode: "CLASSIC"},
}

func champSelectQueueOverride(queueID int64, mode string) string {
	entry := champSelectQueueCompatibility[queueID]
	if entry.mode == "" || entry.mode == mode {
		return entry.group
	}
	return ""
}
func queueUsesLegacyChampSelect(queueID int64) bool {
	return champSelectQueueCompatibility[queueID].legacy
}
func missingQueueID(queueID int64) bool     { return queueID <= 0 }
func unspecifiedQueueID(queueID int64) bool { return queueID == 0 }

func arenaSquadSize(queueID int64) int {
	definition, _ := supportedQueueDefinition(queueID)
	return definition.SquadSize
}
