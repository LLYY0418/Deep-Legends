package main

import (
	"bytes"
	"container/list"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultMatchCount         = 20
	maximumMatchCount         = 50
	maximumSummaryMatchCount  = 100
	maximumMatchStart         = 10000
	sgpCompletionRecentLimit  = 20
	sgpCompletionFailureLimit = 10
	gameplayReferenceCacheMax = 4096
	livePlayerMatchesCacheMax = 256
	livePremadeMinSharedGames = 5
	livePlayerMatchesCacheTTL = 45 * time.Second
	liveRosterConcurrency     = 6
	liveClientPlayerListURL   = "https://127.0.0.1:2999/liveclientdata/playerlist"
	liveClientAllGameDataURL  = "https://127.0.0.1:2999/liveclientdata/allgamedata"
	liveClientRetryDelay      = 3 * time.Second
)

var liveClientDataHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy: nil,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // Live Client Data uses a self-signed loopback certificate.
			MinVersion:         tls.VersionTLS12,
		},
		DisableKeepAlives: true,
	},
	Timeout: 1500 * time.Millisecond,
}

var playerReferencePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)

var (
	diagnosticURLPattern        = regexp.MustCompile(`https?://[^\s"']+`)
	diagnosticIdentifierPattern = regexp.MustCompile(`(?i)(?:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|[A-Za-z0-9_-]{40,128})`)
)

// Riot's hidden champ-select UUID is reversible with the same fixed XOR mask
// used by LeagueAkari's MIT-licensed magic compatibility addon. Keeping the
// transform in Go avoids loading an opaque native binary and keeps the real
// PUUID inside the authenticated backend.
var hiddenPlayerUUIDMask = [16]byte{0x81, 0x70, 0x76, 0xa9, 0xf4, 0x51, 0x50, 0x9b, 0x95, 0x98, 0x68, 0x13, 0xce, 0x91, 0x17, 0xe7}

type gameplayOverviewRequest struct {
	ExpectedTier string `json:"expectedTier,omitempty"`
	PlayerRef    string `json:"playerRef"`
	GameName     string `json:"gameName"`
	TagLine      string `json:"tagLine"`
	// Region 指定查询的服务器："kr" 走 Riot 官方 API 查询韩服；
	// 留空表示国服，继续通过本机客户端查询。
	Region string `json:"region"`
	// ServerID 是国服子服务器（例如 HN1）；韩服必须留空。
	ServerID string `json:"serverId"`
	Count    int    `json:"count"`
	BegIndex int    `json:"begIndex"`
	Force    bool   `json:"force,omitempty"`
	// MatchFilter is normalized server-side and mapped to the documented SGP
	// tag contract. Raw upstream query parameters are never accepted.
	MatchFilter string `json:"matchFilter,omitempty"`
}

type gameplayPagination struct {
	BegIndex             int    `json:"begIndex"`
	Count                int    `json:"count"`
	Total                int    `json:"total,omitempty"`
	HasMore              bool   `json:"hasMore"`
	Partial              bool   `json:"partial,omitempty"`
	BudgetExceeded       bool   `json:"budgetExceeded,omitempty"`
	Filter               string `json:"filter,omitempty"`
	ServerFiltered       bool   `json:"serverFiltered,omitempty"`
	FilterFallback       bool   `json:"filterFallback,omitempty"`
	FilterFallbackReason string `json:"filterFallbackReason,omitempty"`
}

var overviewSoftBudget = 18 * time.Second

type overviewPhasesContextKey struct{}

type overviewPhaseTimings struct {
	mu      sync.Mutex
	started time.Time
	last    time.Time
	values  map[string]int64
	spans   map[string][]overviewPhaseSpan
}

type overviewPhaseSpan struct {
	started  time.Time
	finished time.Time
}

var overviewPhaseNames = []string{
	"identity", "queue_labels", "champion_names", "detailed_matches", "season_snapshot",
	"ranks", "mastery", "recent_ranked", "recent_players", "serialize",
	"account", "summoner", "matchIDs", "details", "opgg-historical",
}

func newOverviewPhaseTimings(started time.Time) *overviewPhaseTimings {
	values := make(map[string]int64, len(overviewPhaseNames)+1)
	for _, name := range overviewPhaseNames {
		values[name] = 0
	}
	return &overviewPhaseTimings{started: started, last: started, values: values, spans: make(map[string][]overviewPhaseSpan)}
}

func (p *overviewPhaseTimings) mark(name string) {
	if p == nil {
		return
	}
	now := time.Now()
	p.mu.Lock()
	p.values[name] += now.Sub(p.last).Milliseconds()
	p.last = now
	p.mu.Unlock()
}

// markSpan records an independently measured phase. Unlike mark, it is safe to
// call from concurrent overview workers and therefore does not pretend that
// overlapping upstream requests were serialized.
func (p *overviewPhaseTimings) markSpan(name string, started, finished time.Time) {
	if p == nil || started.IsZero() || finished.Before(started) {
		return
	}
	p.mu.Lock()
	p.values[name] += finished.Sub(started).Milliseconds()
	p.spans[name] = append(p.spans[name], overviewPhaseSpan{started: started, finished: finished})
	p.mu.Unlock()
}

func (p *overviewPhaseTimings) snapshot(now time.Time) map[string]any {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	// Attribute the tiny tail after the last explicit mark to serialization so
	// every millisecond is accounted for and total stays directly comparable.
	p.values["serialize"] += now.Sub(p.last).Milliseconds()
	p.last = now
	result := make(map[string]any, len(p.values)+2)
	for _, name := range overviewPhaseNames {
		result[name] = p.values[name]
	}
	result["total"] = now.Sub(p.started).Milliseconds()
	spans := make(map[string][][2]int64, len(p.spans))
	for name, entries := range p.spans {
		for _, entry := range entries {
			spans[name] = append(spans[name], [2]int64{
				entry.started.Sub(p.started).Milliseconds(),
				entry.finished.Sub(p.started).Milliseconds(),
			})
		}
	}
	result["spans"] = spans
	return result
}

func overviewPhasesFromContext(ctx context.Context) *overviewPhaseTimings {
	if ctx == nil {
		return nil
	}
	phases, _ := ctx.Value(overviewPhasesContextKey{}).(*overviewPhaseTimings)
	return phases
}

type participantCompletenessSummary struct {
	Incomplete            int
	SingleParticipant     int
	MinParticipants       int
	MaxParticipants       int
	MissingPlayerRefs     int
	GamesMissingPlayerRef int
	Counts                map[int]int
}

type gameplayOverview struct {
	ProfilePending  bool                     `json:"profilePending,omitempty"`
	Player          gameplayPlayer           `json:"player"`
	Ranks           []gameplayRank           `json:"ranks"`
	HistoricalRanks []gameplayHistoricalRank `json:"historicalRanks,omitempty"`
	// 国服 SGP 只提供上赛季和历史最高，不等同于韩服的多赛段 HistoricalRanks。
	RankMilestones      *gameplayRankMilestones             `json:"rankMilestones,omitempty"`
	RecentRanked        gameplayRecentRankedSummary         `json:"recentRanked"`
	Ability             *gameplayAbilityProfile             `json:"ability,omitempty"`
	Overall             gameplayAggregate                   `json:"overall"`
	ChampionStats       []gameplayChampionStat              `json:"championStats"`
	SeasonChampionStats []gameplaySeasonChampionStat        `json:"seasonChampionStats,omitempty"`
	SeasonStatsProgress seasonStatsProgress                 `json:"seasonStatsProgress,omitempty"`
	SeasonOverall       gameplayAggregate                   `json:"seasonOverall,omitempty"`
	Positions           []gameplayPositionStat              `json:"positions"`
	RankedQueues        map[string]gameplayRankedQueueStats `json:"rankedQueues,omitempty"`
	Masteries           []gameplayMastery                   `json:"masteries"`
	RecentPlayers       []gameplayRecentPlayer              `json:"recentPlayers"`
	ActivityHours       []int                               `json:"activityHours"`
	Matches             []gameplayMatch                     `json:"matches"`
	Pagination          gameplayPagination                  `json:"pagination"`
	Capabilities        []EndpointCapability                `json:"capabilities"`
}

type gameplayRankedQueueStats struct {
	RecentRanked       *gameplayRecentRankedSummary `json:"recentRanked,omitempty"`
	Ability            *gameplayAbilityProfile      `json:"ability,omitempty"`
	AbilitySampleGames int                          `json:"abilitySampleGames"`
	Positions          []gameplayPositionStat       `json:"positions"`
	PositionQueueID    int64                        `json:"positionQueueId,omitempty"`
	PositionQueueLabel string                       `json:"positionQueueLabel,omitempty"`
}

type gameplayPlayer struct {
	ProPlayer            *proIdentityBadge `json:"proPlayer,omitempty"`
	PlayerRef            string            `json:"playerRef,omitempty"`
	DisplayName          string            `json:"displayName"`
	GameName             string            `json:"gameName,omitempty"`
	TagLine              string            `json:"tagLine,omitempty"`
	ProfileIconID        int64             `json:"profileIconId,omitempty"`
	SummonerLevel        int64             `json:"summonerLevel,omitempty"`
	BackgroundSkinID     int64             `json:"backgroundSkinId,omitempty"`
	BackgroundSkinName   string            `json:"backgroundSkinName,omitempty"`
	BackgroundSource     string            `json:"backgroundSource,omitempty"`
	BackgroundPath       string            `json:"backgroundPath,omitempty"`
	BackgroundPosterPath string            `json:"backgroundPosterPath,omitempty"`
	BackgroundVideoPath  string            `json:"backgroundVideoPath,omitempty"`
	// Region 标注该玩家所属服务器："kr" 表示韩服（Riot 官方 API），
	// 空值表示国服（本机客户端）。两个服务器的玩家互不相通。
	Region string `json:"region,omitempty"`
	// ServerID / ServerName 明确标注国服子服务器。稳定账号标识仍只在
	// 后端保存，渲染层拿到的 PlayerRef 是会话级匿名引用。
	ServerID   string `json:"serverId,omitempty"`
	ServerName string `json:"serverName,omitempty"`
	Hidden     bool   `json:"hidden"`
	// PrivateHistory 表示玩家在客户端里开启了“隐藏战绩”；身份正常展示，
	// 界面在名称旁标注“隐藏战绩”标签。
	PrivateHistory bool `json:"privateHistory,omitempty"`
	IsCurrent      bool `json:"isCurrent"`
	reference      gameplayReference
}

// gameplayReference is retained only inside the authenticated backend. It
// keeps enough identity hints to continue loading match data when the client
// hides a player's visible name or exposes an obfuscated champ-select ID.
type gameplayReference struct {
	OPGGIdentity        bool // resolve by Riot ID before sending to our Riot API project
	PlayerRef           string
	AlternatePlayerRef  string
	SummonerID          int64
	AlternateSummonerID int64
	DisplayName         string
	GameName            string
	TagLine             string
	ProfileIconID       int64
	SummonerLevel       int64
	// Region 标记玩家所属服务器；"kr" 表示该引用来自 Riot 官方 API 的韩服数据，
	// 后续点击继续查询时无需本机客户端。空值表示国服（本机客户端）。
	Region   string
	ServerID string
	Privacy  string
}

type gameplayRank struct {
	QueueType    string `json:"queueType"`
	QueueLabel   string `json:"queueLabel"`
	Tier         string `json:"tier,omitempty"`
	Division     string `json:"division,omitempty"`
	LeaguePoints int    `json:"leaguePoints"`
	Wins         int    `json:"wins"`
	Losses       int    `json:"losses"`
	WinRate      int    `json:"winRate"`
	Provisional  bool   `json:"provisional"`
}

type gameplayHistoricalRank struct {
	Season       string `json:"season"`
	QueueType    string `json:"queueType"`
	Tier         string `json:"tier"`
	Division     string `json:"division,omitempty"`
	LeaguePoints *int   `json:"leaguePoints,omitempty"`
	WinRate      *int   `json:"winRate,omitempty"`
}

type gameplayRankMilestones struct {
	PeakTier       string                       `json:"peakTier,omitempty"`
	PeakDivision   string                       `json:"peakDivision,omitempty"`
	PreviousSeason []gameplayPreviousSeasonRank `json:"previousSeason,omitempty"`
}

type gameplayPreviousSeasonRank struct {
	QueueType   string `json:"queueType"`
	Tier        string `json:"tier"`
	Division    string `json:"division,omitempty"`
	HighestTier string `json:"highestTier,omitempty"`
	HighestDiv  string `json:"highestDivision,omitempty"`
}

type gameplayAbilityProfile struct {
	SampleGames   int                     `json:"sampleGames"`
	BaselineGames int                     `json:"baselineGames"`
	QueueID       int64                   `json:"queueId,omitempty"`
	QueueLabel    string                  `json:"queueLabel,omitempty"`
	Position      string                  `json:"position,omitempty"`
	PositionLabel string                  `json:"positionLabel,omitempty"`
	BaselineLabel string                  `json:"baselineLabel"`
	SourceLabel   string                  `json:"sourceLabel"`
	Metrics       []gameplayAbilityMetric `json:"metrics"`
}

type gameplayAbilityMetric struct {
	Unavailable bool    `json:"unavailable,omitempty"`
	Key         string  `json:"key"`
	Label       string  `json:"label"`
	Description string  `json:"description"`
	Unit        string  `json:"unit,omitempty"`
	Player      float64 `json:"player"`
	Baseline    float64 `json:"baseline"`
	PlayerScore float64 `json:"playerScore"`
	Grade       string  `json:"grade"`
}

type gameplayAggregate struct {
	QueueID     int64   `json:"queueId,omitempty"`
	QueueLabel  string  `json:"queueLabel,omitempty"`
	Games       int     `json:"games"`
	Wins        int     `json:"wins"`
	Losses      int     `json:"losses"`
	WinRate     int     `json:"winRate"`
	Kills       float64 `json:"kills"`
	Deaths      float64 `json:"deaths"`
	Assists     float64 `json:"assists"`
	KDA         float64 `json:"kda"`
	CS          float64 `json:"cs"`
	CSPerMinute float64 `json:"csPerMinute"`
	Sampled     bool    `json:"sampled,omitempty"`
}

type gameplayRecentRankedSummary struct {
	QueueID                int64                     `json:"queueId,omitempty"`
	QueueLabel             string                    `json:"queueLabel,omitempty"`
	Games                  int                       `json:"games"`
	Wins                   int                       `json:"wins"`
	Losses                 int                       `json:"losses"`
	WinRate                int                       `json:"winRate"`
	Kills                  float64                   `json:"kills"`
	Deaths                 float64                   `json:"deaths"`
	Assists                float64                   `json:"assists"`
	KDA                    float64                   `json:"kda"`
	KillParticipation      float64                   `json:"killParticipation"`
	KillParticipationGames int                       `json:"killParticipationGames"`
	Positions              []gameplayPositionWinRate `json:"positions"`
}

type gameplayPositionWinRate struct {
	Position string `json:"position"`
	Label    string `json:"label"`
	Games    int    `json:"games"`
	Wins     int    `json:"wins"`
	WinRate  int    `json:"winRate"`
}

type gameplayChampionStat struct {
	ChampionID   int64   `json:"championId"`
	ChampionName string  `json:"championName"`
	Games        int     `json:"games"`
	Wins         int     `json:"wins"`
	WinRate      int     `json:"winRate"`
	Kills        float64 `json:"kills"`
	Deaths       float64 `json:"deaths"`
	Assists      float64 `json:"assists"`
	KDA          float64 `json:"kda"`
	CS           float64 `json:"cs"`
	CSPerMinute  float64 `json:"csPerMinute"`
}

type gameplayPositionStat struct {
	Position string `json:"position"`
	Label    string `json:"label"`
	Games    int    `json:"games"`
	Share    int    `json:"share"`
}

type gameplayMastery struct {
	ChampionID     int64  `json:"championId"`
	ChampionName   string `json:"championName"`
	ChampionLevel  int64  `json:"championLevel"`
	ChampionPoints int64  `json:"championPoints"`
}

type gameplayRecentPlayer struct {
	PlayerRef     string `json:"playerRef,omitempty"`
	DisplayName   string `json:"displayName"`
	ProfileIconID int64  `json:"profileIconId,omitempty"`
	Games         int    `json:"games"`
	Hidden        bool   `json:"hidden"`
	reference     gameplayReference
}

type gameplayMatch struct {
	GameID               int64  `json:"gameId"`
	CreatedAt            int64  `json:"createdAt"`
	Duration             int64  `json:"duration"`
	QueueID              int64  `json:"queueId"`
	QueueLabel           string `json:"queueLabel"`
	ModeGroup            string `json:"modeGroup"`
	GameMode             string `json:"gameMode,omitempty"`
	GameType             string `json:"gameType,omitempty"`
	MapID                int64  `json:"mapId,omitempty"`
	Result               string `json:"result"`
	SubjectParticipantID int64  `json:"subjectParticipantId,omitempty"`
	// LpDelta 是当前登录玩家这场排位的胜点变化（由本地 LP 追踪器在
	// 对局结束时记录）；历史对局或未记录到的场次没有该字段。
	LpDelta *int `json:"lpDelta,omitempty"`
	// AverageTier 供演示数据和向后兼容使用；生产数据由前端在首屏之后
	// 调用 match-tiers 异步回填，避免慢速外部请求阻塞总览。
	AverageTier  *matchTiersResponse   `json:"averageTier,omitempty"`
	Participants []gameplayParticipant `json:"participants"`
	Teams        []gameplayTeam        `json:"teams"`
}

type gameplayParticipant struct {
	ProPlayer      *proIdentityBadge `json:"proPlayer,omitempty"`
	ParticipantID  int64             `json:"participantId"`
	TeamID         int64             `json:"teamId"`
	PlayerRef      string            `json:"playerRef,omitempty"`
	DisplayName    string            `json:"displayName"`
	GameName       string            `json:"gameName,omitempty"`
	TagLine        string            `json:"tagLine,omitempty"`
	ProfileIconID  int64             `json:"profileIconId,omitempty"`
	ChampionID     int64             `json:"championId"`
	ChampionName   string            `json:"championName"`
	ChampionLevel  int               `json:"championLevel"`
	Spell1ID       int64             `json:"spell1Id,omitempty"`
	Spell2ID       int64             `json:"spell2Id,omitempty"`
	PrimaryStyleID int64             `json:"primaryStyleId,omitempty"`
	SubStyleID     int64             `json:"subStyleId,omitempty"`
	PerkIDs        []int64           `json:"perkIds"`
	AugmentIDs     []int64           `json:"augmentIds,omitempty"`
	ItemIDs        []int64           `json:"itemIds"`
	Position       string            `json:"position,omitempty"`
	Kills          int               `json:"kills"`
	Deaths         int               `json:"deaths"`
	Assists        int               `json:"assists"`
	KDA            float64           `json:"kda"`
	CS             int               `json:"cs"`
	// LaneCS / JungleCS 把补刀拆成小兵与野怪，供“分均补刀”提示展示。
	LaneCS      int     `json:"laneCs"`
	JungleCS    int     `json:"jungleCs"`
	CSPerMinute float64 `json:"csPerMinute"`
	Gold        int     `json:"gold"`
	Damage      int     `json:"damage"`
	DamageTaken int     `json:"damageTaken"`
	VisionScore int     `json:"visionScore"`
	WardsPlaced int     `json:"wardsPlaced"`
	WardsKilled int     `json:"wardsKilled"`
	// Pointer preserves the difference between a real zero and older sources
	// that do not return visionWardsBoughtInGame at all.
	ControlWardsBought *int `json:"controlWardsBought,omitempty"`
	Win                bool `json:"win"`
	Hidden             bool `json:"hidden"`
	MultiKill          int  `json:"multiKill,omitempty"`
	// 斗魂竞技场等多小队模式：SubteamID 是所属小队编号，
	// Placement 是该小队的最终名次（1 为冠军）。
	SubteamID int64 `json:"subteamId,omitempty"`
	Placement int   `json:"placement,omitempty"`
	reference gameplayReference
}

type gameplayTeam struct {
	TeamID         int64 `json:"teamId"`
	Win            bool  `json:"win"`
	Kills          int   `json:"kills"`
	Gold           int   `json:"gold"`
	Damage         int   `json:"damage"`
	DamageTaken    int   `json:"damageTaken"`
	VisionScore    int   `json:"visionScore"`
	CS             int   `json:"cs"`
	TowerKills     int   `json:"towerKills"`
	DragonKills    int   `json:"dragonKills"`
	BaronKills     int   `json:"baronKills"`
	InhibitorKills int   `json:"inhibitorKills"`
}

type lcuRankedStats struct {
	Queues   []lcuRankedEntry          `json:"queues"`
	QueueMap map[string]lcuRankedEntry `json:"queueMap"`
}

type lcuRankedEntry struct {
	QueueType                     string `json:"queueType"`
	Tier                          string `json:"tier"`
	Division                      string `json:"division"`
	LeaguePoints                  int    `json:"leaguePoints"`
	Wins                          int    `json:"wins"`
	Losses                        int    `json:"losses"`
	IsProvisional                 bool   `json:"isProvisional"`
	PreviousSeasonEndTier         string `json:"previousSeasonEndTier"`
	PreviousSeasonEndDivision     string `json:"previousSeasonEndDivision"`
	PreviousSeasonHighestTier     string `json:"previousSeasonHighestTier"`
	PreviousSeasonHighestDivision string `json:"previousSeasonHighestDivision"`
	HighestTier                   string `json:"highestTier"`
	HighestDivision               string `json:"highestDivision"`
}

type lcuMatchHistory struct {
	Games struct {
		GameCount int       `json:"gameCount"`
		Games     []lcuGame `json:"games"`
	} `json:"games"`
}

type lcuGame struct {
	GameCreation int64 `json:"gameCreation"`
	// GameCreationDate 是部分客户端版本提供的 ISO 时间字符串；
	// gameCreation 缺失或为 0 时用它兜底，避免界面出现“时间未知”。
	GameCreationDate          string                   `json:"gameCreationDate"`
	GameDuration              int64                    `json:"gameDuration"`
	GameEndedInEarlySurrender bool                     `json:"gameEndedInEarlySurrender"`
	GameEndedInSurrender      bool                     `json:"gameEndedInSurrender"`
	GameID                    int64                    `json:"gameId"`
	GameMode                  string                   `json:"gameMode"`
	GameType                  string                   `json:"gameType"`
	MapID                     int64                    `json:"mapId"`
	QueueID                   int64                    `json:"queueId"`
	ParticipantIdentities     []lcuParticipantIdentity `json:"participantIdentities"`
	Participants              []lcuParticipant         `json:"participants"`
	Teams                     []lcuTeam                `json:"teams"`
}

type lcuParticipantIdentity struct {
	ParticipantID int64 `json:"participantId"`
	Player        struct {
		PUUID                string `json:"puuid"`
		ObfuscatedPUUID      string `json:"obfuscatedPuuid"`
		GameName             string `json:"gameName"`
		TagLine              string `json:"tagLine"`
		SummonerName         string `json:"summonerName"`
		ProfileIcon          int64  `json:"profileIcon"`
		SummonerID           int64  `json:"summonerId"`
		ObfuscatedSummonerID int64  `json:"obfuscatedSummonerId"`
	} `json:"player"`
}

type lcuParticipant struct {
	ChampionID    int64 `json:"championId"`
	ParticipantID int64 `json:"participantId"`
	Spell1ID      int64 `json:"spell1Id"`
	Spell2ID      int64 `json:"spell2Id"`
	TeamID        int64 `json:"teamId"`
	Stats         struct {
		Assists                     int   `json:"assists"`
		ChampLevel                  int   `json:"champLevel"`
		Deaths                      int   `json:"deaths"`
		GoldEarned                  int   `json:"goldEarned"`
		GameEndedInEarlySurrender   bool  `json:"gameEndedInEarlySurrender"`
		GameEndedInSurrender        bool  `json:"gameEndedInSurrender"`
		Item0                       int64 `json:"item0"`
		Item1                       int64 `json:"item1"`
		Item2                       int64 `json:"item2"`
		Item3                       int64 `json:"item3"`
		Item4                       int64 `json:"item4"`
		Item5                       int64 `json:"item5"`
		Item6                       int64 `json:"item6"`
		Kills                       int   `json:"kills"`
		LargestMultiKill            int   `json:"largestMultiKill"`
		NeutralMinionsKilled        int   `json:"neutralMinionsKilled"`
		Perk0                       int64 `json:"perk0"`
		Perk1                       int64 `json:"perk1"`
		Perk2                       int64 `json:"perk2"`
		Perk3                       int64 `json:"perk3"`
		Perk4                       int64 `json:"perk4"`
		Perk5                       int64 `json:"perk5"`
		PerkPrimaryStyle            int64 `json:"perkPrimaryStyle"`
		PerkSubStyle                int64 `json:"perkSubStyle"`
		StatPerk0                   int64 `json:"statPerk0"`
		StatPerk1                   int64 `json:"statPerk1"`
		StatPerk2                   int64 `json:"statPerk2"`
		PlayerAugment1              int64 `json:"playerAugment1"`
		PlayerAugment2              int64 `json:"playerAugment2"`
		PlayerAugment3              int64 `json:"playerAugment3"`
		PlayerAugment4              int64 `json:"playerAugment4"`
		PlayerAugment5              int64 `json:"playerAugment5"`
		PlayerAugment6              int64 `json:"playerAugment6"`
		PlayerSubteamID             int64 `json:"playerSubteamId"`
		SubteamPlacement            int   `json:"subteamPlacement"`
		TotalDamageDealtToChampions int   `json:"totalDamageDealtToChampions"`
		TotalDamageTaken            int   `json:"totalDamageTaken"`
		TotalMinionsKilled          int   `json:"totalMinionsKilled"`
		VisionScore                 int   `json:"visionScore"`
		WardsKilled                 int   `json:"wardsKilled"`
		WardsPlaced                 int   `json:"wardsPlaced"`
		VisionWardsBoughtInGame     *int  `json:"visionWardsBoughtInGame"`
		Win                         bool  `json:"win"`
	} `json:"stats"`
	Timeline struct {
		Lane string `json:"lane"`
		Role string `json:"role"`
	} `json:"timeline"`
}

type lcuTeam struct {
	TeamID         int64  `json:"teamId"`
	Win            string `json:"win"`
	TowerKills     int    `json:"towerKills"`
	DragonKills    int    `json:"dragonKills"`
	BaronKills     int    `json:"baronKills"`
	InhibitorKills int    `json:"inhibitorKills"`
}

func (a *app) handleGameplayOverview(w http.ResponseWriter, r *http.Request) {
	a.scheduleItemIconWarmup(true)
	loadStarted := time.Now()
	request := gameplayOverviewRequest{Count: defaultMatchCount}
	stream := false
	loadCost := &overviewLoadCost{}
	phases := newOverviewPhaseTimings(loadStarted)
	timeout := a.overviewTimeout
	if timeout == nil {
		timeout = context.WithTimeout
	}
	budget := overviewSoftBudget
	if strings.Contains(r.Header.Get("Accept"), "application/x-ndjson") {
		budget = 180 * time.Second
		_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	}
	budgetContext, cancelBudget := timeout(r.Context(), budget)
	budgetContext = context.WithValue(budgetContext, overviewRequestContextKey{}, r.Context())
	defer cancelBudget()
	requestContext := context.WithValue(budgetContext, overviewLoadCostContextKey{}, loadCost)
	requestContext = context.WithValue(requestContext, overviewPhasesContextKey{}, phases)
	r = r.WithContext(requestContext)
	defer func() {
		finishedAt := time.Now()
		requests, bytes, historyCalls, historyCacheHits := loadCost.snapshot()
		a.recordDiagnostic(map[string]any{
			"event": "overview_load_cost", "beg_index": request.BegIndex, "stream": stream, "sgp_requests": requests, "sgp_bytes": bytes,
			"sgp_history_calls": historyCalls, "sgp_history_cache_hits": historyCacheHits,
			"duration_ms": finishedAt.Sub(loadStarted).Milliseconds(),
		})
		a.recordDiagnostic(map[string]any{"event": "overview_phases_ms", "beg_index": request.BegIndex, "stream": stream, "load_phases_ms": phases.snapshot(finishedAt)})
	}()
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			http.Error(w, "查询参数无效", http.StatusBadRequest)
			return
		}
	} else {
		if raw := r.URL.Query().Get("count"); raw != "" {
			request.Count, _ = strconv.Atoi(raw)
		}
		if raw := r.URL.Query().Get("begIndex"); raw != "" {
			request.BegIndex, _ = strconv.Atoi(raw)
		}
		request.MatchFilter = r.URL.Query().Get("matchFilter")
		request.Force = r.URL.Query().Get("force") == "1" || strings.EqualFold(r.URL.Query().Get("force"), "true")
	}
	request.PlayerRef = strings.TrimSpace(request.PlayerRef)
	if request.PlayerRef != "" && !validPlayerReference(request.PlayerRef) {
		http.Error(w, "玩家标识无效", http.StatusBadRequest)
		return
	}
	reference := gameplayReference{PlayerRef: request.PlayerRef}
	if request.PlayerRef != "" {
		resolved, ok := a.resolveGameplayReferenceDetails(request.PlayerRef)
		if !ok {
			http.Error(w, "玩家引用已失效，请从对局列表重新打开", http.StatusNotFound)
			return
		}
		reference = resolved
	}
	request.Count = clampMatchCount(request.Count)
	request.BegIndex = clampMatchStart(request.BegIndex)
	request.MatchFilter = normalizeGameplayMatchFilter(request.MatchFilter)
	request.GameName = strings.TrimSpace(request.GameName)
	request.TagLine = strings.TrimSpace(strings.TrimPrefix(request.TagLine, "#"))
	request.Region = strings.ToLower(strings.TrimSpace(request.Region))
	rawServerID := strings.TrimSpace(request.ServerID)
	request.ServerID = strings.ToUpper(rawServerID)
	if rawServerID != "" {
		if normalized, ok := normalizeTencentServerID(rawServerID); ok {
			request.ServerID = normalized
		} else {
			http.Error(w, "国服服务器无效", http.StatusBadRequest)
			return
		}
	}
	if reference.ServerID != "" && request.ServerID != "" && !strings.EqualFold(reference.ServerID, request.ServerID) {
		http.Error(w, "玩家引用与所选服务器不一致", http.StatusBadRequest)
		return
	}
	if reference.ServerID == "" && request.ServerID != "" {
		reference.ServerID = request.ServerID
	}
	if request.PlayerRef == "" && request.GameName != "" {
		if len([]rune(request.GameName)) > 40 || len([]rune(request.TagLine)) > 12 {
			http.Error(w, "玩家名称无效", http.StatusBadRequest)
			return
		}
	}
	// 韩服玩家（英雄榜单点击、顶部搜索选择韩服、或此前打开的韩服页签）：
	// 直接走 Riot 官方 API，不依赖本机客户端。
	if strings.EqualFold(reference.Region, riotRegionKR) || (request.PlayerRef == "" && request.GameName != "" && request.Region == riotRegionKR) {
		if request.ServerID != "" {
			http.Error(w, "韩服查询不能指定国服服务器", http.StatusBadRequest)
			return
		}
		if reference.Region == "" {
			reference = gameplayReference{GameName: request.GameName, TagLine: request.TagLine, Region: riotRegionKR}
		}
		phases.mark("identity")
		stream = strings.Contains(r.Header.Get("Accept"), "application/x-ndjson")
		streamStarted := false
		emit := func(value any) {
			if !streamStarted {
				w.Header().Set("Content-Type", "application/x-ndjson")
				w.Header().Set("Cache-Control", "no-store")
				streamStarted = true
			}
			_ = json.NewEncoder(w).Encode(value)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
		if stream {
			r = r.WithContext(context.WithValue(r.Context(), riotOverviewProgressKey{}, func(partial gameplayOverview) {
				partial.Pagination.Filter = request.MatchFilter
				emit(map[string]any{"type": "progress", "overview": partial})
			}))
		}
		response, err := a.loadRiotOverviewDeduplicated(r.Context(), reference, request.BegIndex, request.Count, request.Force, request.MatchFilter)
		phases.mu.Lock()
		phases.last = time.Now()
		phases.mu.Unlock()
		if err != nil {
			if streamStarted {
				emit(struct {
					riotHTTPError
					Type   string `json:"type"`
					Status int    `json:"status"`
				}{riotHTTPErrorBody(err), "error", riotErrorStatus(err)})
			} else {
				writeRiotHTTPError(w, err)
			}
			return
		}
		response.Pagination.Filter = request.MatchFilter
		// Comparison is request-local: never mutate the shared overview cache.
		payload := struct {
			gameplayOverview
			ProMismatch bool `json:"proMismatch,omitempty"`
		}{response, proOverviewMismatch(request.ExpectedTier, response.Ranks)}
		if stream {
			emit(map[string]any{"type": "complete", "overview": payload})
		} else {
			respondJSON(w, payload)
		}
		phases.mark("serialize")
		return
	}
	client, current, err := a.gameplayClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	// 按 Riot ID 打开国服玩家（顶部搜索）：当前服务器优先使用 LCU
	// aliases；只有真正跨服时才发现 Riot Client 并通过 SGP 确认归属。
	if request.PlayerRef == "" && request.GameName != "" {
		if request.ServerID == "" {
			_, platform := client.platformInfo()
			request.ServerID, _ = normalizeTencentServerID(platform)
		}
		if request.ServerID == "" {
			http.Error(w, "请选择要查询的国服服务器", http.StatusBadRequest)
			return
		}
		remoteLookup := isRemoteTencentServer(client, request.ServerID)
		resolved, lookupErr := a.resolveTencentRiotID(r.Context(), client, request.GameName, request.TagLine, request.ServerID)
		if lookupErr != nil {
			switch {
			case errors.Is(lookupErr, errTencentPlayerNotFound):
				http.Error(w, "所选服务器没有找到该玩家，请核对名称、编号和服务器", http.StatusNotFound)
			case remoteLookup && (errors.Is(lookupErr, errRiotClientNotFound) || errors.Is(lookupErr, errRiotClientCredentialsUnreadable)):
				http.Error(w, "跨服搜索需要 Riot Client 服务正在运行；查询当前服务器玩家不受影响", http.StatusConflict)
			case remoteLookup:
				http.Error(w, "国服跨服查询暂时不可用，请确认客户端已登录后重试", http.StatusBadGateway)
			case errors.Is(lookupErr, context.DeadlineExceeded):
				http.Error(w, "客户端响应超时，请稍后重试", http.StatusBadGateway)
			default:
				var lcuErr *LCUHTTPError
				if errors.As(lookupErr, &lcuErr) && lcuErr.StatusCode != http.StatusNotFound {
					http.Error(w, fmt.Sprintf("客户端返回异常（HTTP %d）", lcuErr.StatusCode), http.StatusBadGateway)
				} else {
					http.Error(w, "当前服务器玩家查询暂时不可用，请确认客户端已登录后重试", http.StatusBadGateway)
				}
			}
			return
		}
		reference = resolved
	}
	if reference.ServerID == "" {
		_, platform := client.platformInfo()
		reference.ServerID, _ = normalizeTencentServerID(platform)
	}
	phases.mark("identity")
	response := a.loadGameplayOverviewDeduplicated(r.Context(), client, current, reference, request.BegIndex, request.Count, request.MatchFilter, request.Force)
	respondJSON(w, response)
	phases.mark("serialize")
}

var errTencentPlayerNotFound = errors.New("Tencent player was not found on selected server")

type lcuSummonerAlias struct {
	PUUID string `json:"puuid"`
}

type lcuSummonerAliasResponse []lcuSummonerAlias

func (aliases *lcuSummonerAliasResponse) UnmarshalJSON(data []byte) error {
	// alias/lookup returns one object on Tencent clients. Preserve support for
	// array-shaped responses without mistaking a successful HTTP 200 for failure.
	*aliases = nil
	data = bytes.TrimSpace(data)
	if len(data) > 0 && data[0] == '{' {
		var alias lcuSummonerAlias
		if err := json.Unmarshal(data, &alias); err != nil {
			return err
		}
		*aliases = []lcuSummonerAlias{alias}
		return nil
	}
	var rows []lcuSummonerAlias
	if err := json.Unmarshal(data, &rows); err != nil {
		return err
	}
	*aliases = rows
	return nil
}

func (a *app) resolveTencentRiotID(ctx context.Context, lcu *LCUClient, gameName, tagLine, serverID string) (resolved gameplayReference, resultErr error) {
	started := time.Now()
	serverID, ok := normalizeTencentServerID(serverID)
	remoteLookup := ok && isRemoteTencentServer(lcu, serverID)
	stage := "server-scope"
	aliasCount, attempts := 0, 0
	defer func() {
		outcome := "success"
		httpStatus := 0
		if resultErr != nil {
			switch {
			case errors.Is(resultErr, errTencentPlayerNotFound):
				outcome = "not-found"
			case errors.Is(resultErr, context.DeadlineExceeded):
				outcome = "timeout"
			default:
				var shapeErr *json.UnmarshalTypeError
				var httpErr *LCUHTTPError
				if errors.As(resultErr, &shapeErr) {
					outcome = "response-shape-error"
				} else if errors.As(resultErr, &httpErr) {
					outcome = "http-error"
					httpStatus = httpErr.StatusCode
				} else {
					outcome = "client-error"
				}
			}
		}
		a.recordDiagnostic(map[string]any{
			"event": "tencent_riot_id_lookup", "outcome": outcome, "http_status": httpStatus,
			"duration_ms": time.Since(started).Milliseconds(), "remote_lookup": remoteLookup, "server_id": serverID,
			"stage": stage, "alias_count": aliasCount, "attempts": attempts, "error_kind": diagnosticErrorKind(resultErr),
		})
	}()
	if !ok {
		return gameplayReference{}, errTencentPlayerNotFound
	}
	if serverID == clientTencentServerID(lcu) {
		stage = "local-alias"
		query := url.Values{"gameName": {strings.TrimSpace(gameName)}, "tagLine": {strings.TrimSpace(tagLine)}}
		var aliases lcuSummonerAliasResponse
		requestPath := "/lol-summoner/v1/alias/lookup?" + query.Encode()
		attempts++
		err := lcu.RequestJSON(ctx, http.MethodGet, requestPath, nil, &aliases)
		if retryableLCUAliasLookupError(err) {
			timer := time.NewTimer(500 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return gameplayReference{}, ctx.Err()
			case <-timer.C:
			}
			attempts++
			err = lcu.RequestJSON(ctx, http.MethodGet, requestPath, nil, &aliases)
		}
		if err != nil {
			var httpErr *LCUHTTPError
			if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
				return gameplayReference{}, errTencentPlayerNotFound
			}
			return gameplayReference{}, err
		}
		var matched string
		aliasCount = len(aliases)
		stage = "local-identity-validation"
		for _, alias := range aliases {
			if validPlayerReference(alias.PUUID) {
				if matched != "" && matched != alias.PUUID {
					stage = "ambiguous-local-identity"
					return gameplayReference{}, errors.New("客户端返回多个不同玩家，无法确认身份")
				}
				matched = alias.PUUID
			}
		}
		if matched != "" {
			return normalizeGameplayReference(gameplayReference{
				PlayerRef: matched, GameName: gameName, TagLine: tagLine, ServerID: serverID,
			}), nil
		}
		return gameplayReference{}, errTencentPlayerNotFound
	}
	discover := a.riotClientDiscovery
	stage = "riot-client-discovery"
	if discover == nil {
		discover = discoverRiotClient
	}
	riotClient, err := discover()
	if err != nil {
		return gameplayReference{}, err
	}
	defer riotClient.Close()
	stage = "remote-alias"
	attempts++
	aliases, err := riotClient.aliasesByRiotID(ctx, gameName, tagLine)
	aliasCount = len(aliases)
	if err != nil {
		if errors.Is(err, errRiotClientAliasNotFound) {
			return gameplayReference{}, errTencentPlayerNotFound
		}
		return gameplayReference{}, err
	}
	for _, alias := range aliases {
		stage = "remote-summoner"
		summoner, lookupErr := a.sgp.summonerByPUUIDOn(ctx, lcu, serverID, alias.PUUID)
		if errors.Is(lookupErr, errSGPSummonerNotFound) {
			continue
		}
		if lookupErr != nil {
			return gameplayReference{}, lookupErr
		}
		resolvedGameName := strings.TrimSpace(alias.Alias.GameName)
		resolvedTagLine := strings.TrimSpace(alias.Alias.TagLine)
		if resolvedGameName == "" {
			resolvedGameName = gameName
		}
		if resolvedTagLine == "" {
			resolvedTagLine = tagLine
		}
		return normalizeGameplayReference(gameplayReference{
			PlayerRef: alias.PUUID, DisplayName: summoner.Name, GameName: resolvedGameName, TagLine: resolvedTagLine,
			ProfileIconID: summoner.ProfileIconID, SummonerLevel: summoner.Level, ServerID: serverID, Privacy: summoner.Privacy,
		}), nil
	}
	return gameplayReference{}, errTencentPlayerNotFound
}

func retryableLCUAliasLookupError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	var httpErr *LCUHTTPError
	if errors.As(err, &httpErr) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var networkError net.Error
	return errors.As(err, &networkError)
}

func (a *app) gameplayClient() (*LCUClient, Summoner, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if !a.connected || a.lcu == nil {
		return nil, Summoner{}, errors.New("当前没有已连接的英雄联盟客户端")
	}
	return a.lcu, a.summoner, nil
}

func (a *app) currentPlayerRef() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.summoner.PUUID
}

func clientTencentServerID(client *LCUClient) string {
	if client == nil {
		return ""
	}
	region, platform := client.platformInfo()
	if !strings.EqualFold(region, "TENCENT") {
		return ""
	}
	serverID, _ := normalizeTencentServerID(platform)
	return serverID
}

func isRemoteTencentServer(client *LCUClient, serverID string) bool {
	serverID, ok := normalizeTencentServerID(serverID)
	return ok && serverID != clientTencentServerID(client)
}

func (a *app) championNames() map[int64]string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := make(map[int64]string)
	for _, skin := range a.allSkins {
		if skin.ChampionID > 0 && strings.TrimSpace(skin.ChampionName) != "" {
			result[skin.ChampionID] = skin.ChampionName
		}
	}
	return result
}

func (a *app) currentProfileBackground() (int64, string, string, string) {
	a.mu.RLock()
	profile := a.account.Profile
	a.mu.RUnlock()
	return profileBackground(profile)
}

func profileBackground(profile SummonerProfile) (int64, string, string, string) {
	if profile.BackgroundSkinID <= 0 {
		return 0, "", "", ""
	}
	return profile.BackgroundSkinID, profile.BackgroundSkinName, "gtimg",
		fmt.Sprintf("%s%d.jpg", gtimgSkinArtworkPrefix, profile.BackgroundSkinID)
}

// applyMasteryBackgroundFallback 补齐召唤师条的背景原画。
//
// 个人主页背景只存在于本机登录客户端的 /lol-summoner/v1/current-summoner/summoner-profile，
// Riot 官方 API 没有对应字段，SGP 也不返回别人的背景设置。因此韩服、查看别人资料、
// 以及本人压根没设过背景这三种情况下 BackgroundPath 都是空的，界面会退化成纯色卡片。
// 这里统一退回到该玩家最高熟练度英雄的默认皮肤原画（皮肤编号 = 英雄 ID × 1000），
// 让背景和旁边的“最高熟练度”徽章指向同一个英雄。
//
// 走 gtimg 是因为 handleChampionAsset 对 gtimg 皮肤原画配了 Data Dragon 回退
// （见 championAssetFallback），腾讯 CDN 缺图或不可达时会自动换源，无需前端感知。
func applyMasteryBackgroundFallback(player *gameplayPlayer, masteries []gameplayMastery) {
	if player == nil || player.BackgroundPath != "" {
		return
	}
	best := gameplayMastery{}
	for _, item := range masteries {
		if item.ChampionID > 0 && item.ChampionPoints > best.ChampionPoints {
			best = item
		}
	}
	if best.ChampionID <= 0 {
		return
	}
	skinID := best.ChampionID * 1000
	player.BackgroundSkinID = skinID
	player.BackgroundSkinName = best.ChampionName
	player.BackgroundSource = "gtimg"
	player.BackgroundPath = fmt.Sprintf("%s%d.jpg", gtimgSkinArtworkPrefix, skinID)
}

// recentWindowDays 是“最近排位 / 最近一起玩”等汇总统计的时间窗口（天）。
const recentWindowDays = 30

const (
	recentRankedSampleCacheTTL = 10 * time.Minute
	recentRankedSampleCacheMax = 128
)

type recentRankedSampleCacheEntry struct {
	at      time.Time
	matches []gameplayMatch
}

type recentRankedSampleSet struct {
	ByQueue map[int64][]gameplayMatch
}

type recentRankedResult struct {
	value          recentRankedSampleSet
	started, ended time.Time
}

func (a *app) loadGameplayOverview(ctx context.Context, client *LCUClient, current Summoner, reference gameplayReference, begIndex, count int, matchFilter string, force bool) gameplayOverview {
	reference = normalizeGameplayReference(reference)
	if reference.Region == "" && reference.ServerID == "" {
		reference.ServerID = clientTencentServerID(client)
	}
	playerRef := reference.PlayerRef
	isCurrent := (playerRef == "" || gameplayReferenceContains(reference, current.PUUID)) && !isRemoteTencentServer(client, reference.ServerID)
	if force && a.sgp != nil {
		historyRef := playerRef
		if isCurrent {
			historyRef = current.PUUID
		}
		a.sgp.invalidatePlayerHistory(reference.ServerID, historyRef)
	}
	player := current
	profileHidden := strings.TrimSpace(player.GameName) == "" && strings.TrimSpace(player.DisplayName) == ""
	capabilities := make([]EndpointCapability, 0, 5)
	if isCurrent {
		summonerCapability := EndpointCapability{Name: "summoner", Path: "/lol-summoner/v1/current-summoner"}
		if force {
			if _, refreshErr := a.refreshSummonerIdentity(client); refreshErr != nil {
				summonerCapability.State = capabilityFailed
				summonerCapability.Detail = a.summonerCapabilityDetail("身份刷新失败，沿用上次读取的身份")
			} else {
				a.mu.RLock()
				if a.lcu == client {
					current = a.summoner
				}
				a.mu.RUnlock()
				player = current
				profileHidden = strings.TrimSpace(player.GameName) == "" && strings.TrimSpace(player.DisplayName) == ""
				summonerCapability.State = capabilityAvailable
				summonerCapability.Count = 1
			}
		} else {
			summonerCapability.State = capabilityAvailable
			summonerCapability.Detail = a.summonerCapabilityDetail("使用缓存身份，本次未请求客户端")
		}
		playerRef = current.PUUID
		reference = mergeGameplayReferences(reference, gameplayReferenceFromSummoner(current))
		a.startRankedSplitProbe(client, playerRef, current.SummonerID)
		capabilities = append(capabilities, summonerCapability)
	} else if reference.ServerID != "" {
		var loaded sgpSummoner
		var loadErr error
		if a.sgp == nil {
			loadErr = errors.New("SGP 召唤师数据源不可用")
		} else {
			loaded, loadErr = a.sgp.summonerByPUUIDOn(ctx, client, reference.ServerID, playerRef)
		}
		capability := EndpointCapability{Name: "summoner", Path: "sgp: /summoner-ledge/v1/regions/{server}/summoners/puuids"}
		if loadErr != nil {
			if isCancellation(loadErr) {
				capability.State = capabilityCanceled
			} else if a.sgp == nil {
				capability.State = capabilityUnsupported
				capability.Detail = "当前未启用跨服召唤师数据源"
			} else {
				capability.State = capabilityFailed
				capability.Detail = "所选服务器暂时无法读取召唤师资料"
			}
			player = summonerFromGameplayReference(reference)
			if strings.TrimSpace(player.DisplayName) == "" && strings.TrimSpace(player.GameName) == "" {
				player.DisplayName = "隐藏玩家"
			}
			profileHidden = true
		} else {
			capability.State = capabilityAvailable
			capability.Count = 1
			player = mergeSummonerIdentity(Summoner{
				PUUID: loaded.PUUID, DisplayName: loaded.Name, ProfileIconID: loaded.ProfileIconID,
				SummonerLevel: loaded.Level, Privacy: loaded.Privacy,
			}, summonerFromGameplayReference(reference))
			reference = mergeGameplayReferences(gameplayReferenceFromSummoner(player), reference)
			profileHidden = strings.TrimSpace(player.GameName) == "" && strings.TrimSpace(player.DisplayName) == ""
		}
		capabilities = append(capabilities, capability)
	} else {
		loaded, capability := loadGameplaySummoner(client, reference)
		if capability.State != capabilityAvailable {
			player = summonerFromGameplayReference(reference)
			if strings.TrimSpace(player.DisplayName) == "" && strings.TrimSpace(player.GameName) == "" {
				player.DisplayName = "隐藏玩家"
			}
			profileHidden = true
		} else {
			player = mergeSummonerIdentity(loaded, summonerFromGameplayReference(reference))
			reference = mergeGameplayReferences(gameplayReferenceFromSummoner(player), reference)
			profileHidden = strings.TrimSpace(player.GameName) == "" && strings.TrimSpace(player.DisplayName) == ""
		}
		capabilities = append(capabilities, capability)
	}
	if player.PUUID == "" {
		player.PUUID = playerRef
	}
	if validPlayerReference(player.PUUID) {
		playerRef = player.PUUID
		reference = mergeGameplayReferences(gameplayReferenceFromSummoner(player), reference)
	}
	phases := overviewPhasesFromContext(ctx)
	type queueResult struct {
		value             map[int64]string
		started, finished time.Time
	}
	type namesResult struct {
		value             map[int64]string
		started, finished time.Time
	}
	type seasonResult struct {
		stats             []gameplaySeasonChampionStat
		progress          seasonStatsProgress
		byQueue           map[int64]gameplayAggregate
		started, finished time.Time
	}
	queueCh := make(chan queueResult, 1)
	namesCh := make(chan namesResult, 1)
	seasonCh := make(chan seasonResult, 1)
	go func() {
		started := time.Now()
		value := loadQueueLabelsContext(ctx, client)
		queueCh <- queueResult{value, started, time.Now()}
	}()
	go func() {
		started := time.Now()
		value := a.overviewChampionNames(ctx)
		namesCh <- namesResult{value, started, time.Now()}
	}()
	go func() {
		started := time.Now()
		stats, progress, _, byQueue := a.loadSeasonChampionStatsSnapshot(reference, player, playerRef)
		seasonCh <- seasonResult{stats, progress, byQueue, started, time.Now()}
	}()
	type rankResult struct {
		value          rankScoreEntry
		started, ended time.Time
	}
	type masteryResult struct {
		value          map[int64]ChampionMastery
		capability     EndpointCapability
		started, ended time.Time
	}
	var rankCh chan rankResult
	var masteryCh chan masteryResult
	if begIndex == 0 && ctx.Err() == nil {
		rankCh = make(chan rankResult, 1)
		go func() {
			started := time.Now()
			value := a.playerRankScore(ctx, client, playerRef, isCurrent, reference.ServerID, reference.Privacy)
			rankCh <- rankResult{value: value, started: started, ended: time.Now()}
		}()
		masteryCh = make(chan masteryResult, 1)
		go func() {
			started := time.Now()
			capability := EndpointCapability{Name: "champion-mastery", Path: "/lol-champion-mastery/v1/{player}/champion-mastery"}
			value := map[int64]ChampionMastery{}
			if isRemoteTencentServer(client, reference.ServerID) {
				capability.State = capabilityUnsupported
				capability.Detail = "所选服务器暂未提供可核验的跨服熟练度接口"
			} else {
				value, capability = NewChampionMasteryAPI(client).AllContext(ctx, playerRef)
				capability.Path = "/lol-champion-mastery/v1/{player}/champion-mastery"
			}
			masteryCh <- masteryResult{value: value, capability: capability, started: started, ended: time.Now()}
		}()
	}
	queue := <-queueCh
	namesResultValue := <-namesCh
	queueLabels, names := queue.value, namesResultValue.value
	phases.markSpan("queue_labels", queue.started, queue.finished)
	phases.markSpan("champion_names", namesResultValue.started, namesResultValue.finished)
	detailedStarted := time.Now()
	matches, historyCapabilities, pagination := a.loadDetailedMatches(ctx, client, reference, playerRef, isCurrent, begIndex, count, matchFilter, names, queueLabels)
	detailedFinished := time.Now()
	phases.markSpan("detailed_matches", detailedStarted, detailedFinished)
	capabilities = append(capabilities, historyCapabilities...)
	var rankedCh chan recentRankedResult
	if begIndex == 0 && ctx.Err() == nil {
		rankedCh = make(chan recentRankedResult, 1)
		go func() {
			started := time.Now()
			value := a.loadRecentRankedSamples(ctx, client, reference, playerRef, matchFilter, pagination, matches, names, queueLabels)
			rankedCh <- recentRankedResult{value: value, started: started, ended: time.Now()}
		}()
	}
	a.recordMatchModeClassifications(matches)
	// 标注本地追踪到的排位胜点变化（仅当前登录玩家的场次有记录）。
	if isCurrent {
		a.lpTracker.annotate(matches, playerRef)
	}
	playerData := gameplayPlayer{
		PlayerRef: playerRef, DisplayName: gameplayDisplayName(player), GameName: player.GameName,
		TagLine: player.TagLine, ProfileIconID: player.ProfileIconID, SummonerLevel: player.SummonerLevel,
		Region: reference.Region, ServerID: reference.ServerID, ServerName: tencentServerName(reference.ServerID),
		Hidden: profileHidden, PrivateHistory: strings.EqualFold(strings.TrimSpace(reference.Privacy), "PRIVATE"),
		IsCurrent: isCurrent, reference: reference,
	}
	if isCurrent {
		playerData.BackgroundSkinID, playerData.BackgroundSkinName, playerData.BackgroundSource, playerData.BackgroundPath = a.currentProfileBackground()
	}
	response := gameplayOverview{
		Player:  playerData,
		Matches: matches, Capabilities: capabilities,
		Pagination: pagination,
	}
	season := <-seasonCh
	phases.markSpan("season_snapshot", season.started, season.finished)
	seasonStats, seasonProgress, seasonByQueue := season.stats, season.progress, season.byQueue
	response.SeasonChampionStats = seasonStats
	response.SeasonStatsProgress = seasonProgress
	response.SeasonOverall = seasonStatsOverall(seasonStats)
	if begIndex > 0 {
		phases.mark("ranks")
		phases.mark("mastery")
		phases.mark("recent_ranked")
		phases.mark("recent_players")
		a.publicizeOverviewReferences(&response)
		return response
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		response.Pagination.BudgetExceeded = true
		response.Pagination.Partial = true
		response.Pagination.HasMore = true
		// A slow match page must not discard profile requests that already
		// finished. Drain only ready results, without extending the budget.
		ready := map[string]bool{}
		select {
		case result := <-rankCh:
			response.Ranks, result.value.capability = a.applySeasonRankWinRateFallback(result.value.ranks, result.value.capability, seasonByQueue)
			response.RankMilestones = rankMilestonesForRegion(reference.Region, result.value.milestones)
			response.Capabilities = append(response.Capabilities, result.value.capability)
			ready["ranked-stats"] = true
		default:
		}
		select {
		case result := <-masteryCh:
			response.Masteries = normalizeMasteries(result.value, names, 6)
			response.Capabilities = append(response.Capabilities, result.capability)
			ready["champion-mastery"] = true
		default:
		}
		for _, capability := range overviewBudgetCapabilities() {
			if !ready[capability.Name] {
				response.Capabilities = append(response.Capabilities, capability)
			}
		}
		a.completeOverviewBackground(&response, playerRef)
		phases.mark("ranks")
		phases.mark("mastery")
		phases.mark("recent_ranked")
		phases.mark("recent_players")
		a.publicizeOverviewReferences(&response)
		return response
	}
	// Season history is an incremental state slice. It must never block matches,
	// ranks or player identity in the core overview response.
	a.startSeasonStatsRefresh(client, reference, player, playerRef, names)

	rankResultValue := <-rankCh
	phases.markSpan("ranks", rankResultValue.started, rankResultValue.ended)
	rankEntry := rankResultValue.value
	ranks := append([]gameplayRank(nil), rankEntry.ranks...)
	rankMilestones, rankCapability := rankEntry.milestones, rankEntry.capability
	ranks, rankCapability = a.applySeasonRankWinRateFallback(ranks, rankCapability, seasonByQueue)
	capabilities = append(capabilities, rankCapability)
	response.RankMilestones = rankMilestonesForRegion(reference.Region, rankMilestones)
	if reference.Region == riotRegionKR && !strings.EqualFold(strings.TrimSpace(reference.Privacy), "PRIVATE") {
		response.HistoricalRanks = a.cachedOPGGHistoricalRanks(playerRef)
		a.startOPGGHistoricalRanks(reference, player.GameName, player.TagLine, playerRef, reference.Privacy)
	}
	// 刷新 LP 追踪基线：下一场结算时据此计算胜点变化。
	if isCurrent {
		a.lpTracker.observe(playerRef, ranks)
	}
	masteryResultValue := <-masteryCh
	phases.markSpan("mastery", masteryResultValue.started, masteryResultValue.ended)
	masteryMap, masteryCapability := masteryResultValue.value, masteryResultValue.capability
	windowMatches := matches
	windowAvailable := len(matches) > 0
	windowExhausted := false
	remoteServer := isRemoteTencentServer(client, reference.ServerID)
	shouldLoadWindow := false
	if shouldLoadOverviewHistory(reference, playerRef, matches) {
		shouldLoadWindow = true
	}
	if a.sgp != nil && shouldLoadWindow {
		infos, _, more, windowErr := a.sgp.matchHistoryOn(ctx, client, reference.ServerID, playerRef, 0, maximumSummaryMatchCount, true)
		windowCapability := EndpointCapability{Name: "seven-day-history", Path: "sgp: /match-history-query/v1/products/lol/player/{player}/SUMMARY", Detail: "用于过去 30 天排位与活跃时段统计"}
		var windowPartial *sgpPartialHistoryError
		if errors.As(windowErr, &windowPartial) && isCancellation(windowPartial) {
			windowCapability.State = capabilityCanceled
			windowCapability.Detail = ""
			windowAvailable = true
		} else if windowErr == nil || errors.As(windowErr, &windowPartial) && len(infos) > 0 {
			participantSummary := summarizeRiotParticipants(infos)
			filterSummary := summarizeRiotGameplayFilters(infos)
			windowCapability.State = capabilityAvailable
			details := make([]string, 0, 2)
			if participantSummary.Incomplete > 0 {
				windowCapability.State = capabilityFailed
				details = append(details, fmt.Sprintf("SGP 返回的 %d 场 30 天样本参与者不完整，最近一起玩可能缺失", participantSummary.Incomplete))
			}
			if windowPartial != nil {
				windowCapability.State = capabilityFailed
				details = append(details, "SGP 只返回了部分 30 天统计样本")
			}
			if len(details) > 0 {
				windowCapability.Detail = strings.Join(details, "；")
			}
			windowCapability.Count = len(infos)
			windowAvailable = true
			windowExhausted = !more
			windowEvent := "sgp_summary_history_succeeded"
			if windowPartial != nil {
				windowEvent = "sgp_summary_history_partial"
			}
			windowDiagnostic := map[string]any{
				"event": windowEvent, "returned": len(infos),
				"visible": filterSummary.Visible, "skipped_empty_participants": filterSummary.SkippedEmptyParticipants,
				"filtered_custom": filterSummary.FilteredCustom, "custom_reasons": filterSummary.CustomReasons,
				"incomplete": participantSummary.Incomplete, "single_participant": participantSummary.SingleParticipant,
				"min_participants": participantSummary.MinParticipants, "max_participants": participantSummary.MaxParticipants,
				"participant_counts": participantSummary.Counts, "missing_player_refs": participantSummary.MissingPlayerRefs,
				"games_missing_player_refs": participantSummary.GamesMissingPlayerRef,
			}
			if windowPartial != nil {
				windowDiagnostic["reason"] = safeDiagnosticReason(windowPartial)
			}
			a.recordDiagnostic(windowDiagnostic)
			if len(infos) > len(matches) {
				windowMatches = make([]gameplayMatch, 0, len(infos))
				for _, info := range infos {
					if info == nil || len(info.Participants) == 0 {
						continue
					}
					match := convertRiotMatchInfo(info, playerRef, names, queueLabels, "", reference.ServerID)
					if !isCustomGameplayMatch(match) {
						windowMatches = append(windowMatches, match)
					}
				}
			}
		} else {
			if isCancellation(windowErr) {
				windowCapability.State = capabilityCanceled
				windowAvailable = true
			} else {
				windowCapability.State = capabilityFailed
				windowCapability.Detail = "所选服务器暂时无法读取 30 天统计样本"
				a.recordDiagnostic(map[string]any{"event": "sgp_summary_history_failed", "reason": safeDiagnosticReason(windowErr)})
			}
		}
		capabilities = append(capabilities, windowCapability)
	} else if !shouldLoadWindow && reference.ServerID != "" && validPlayerReference(playerRef) {
		capabilities = append(capabilities, EndpointCapability{
			Name: "seven-day-history", Path: "sgp: /match-history-query/v1/products/lol/player/{player}/SUMMARY",
			State: capabilityAvailable, Count: len(matches), Detail: "复用首屏完整战绩样本，避免重复下载",
		})
		a.recordDiagnostic(map[string]any{"event": "sgp_summary_history_skipped", "reason": "reuse_detail_matches", "reused": len(matches)})
	}
	if !windowAvailable && !remoteServer {
		windowGames, windowCapabilities, _ := loadGameplayHistoryContext(ctx, client, playerRef, isCurrent, 0, maximumSummaryMatchCount, false)
		windowAvailable = len(windowCapabilities) > 0 && windowCapabilities[0].State == capabilityAvailable
		if len(windowCapabilities) > 0 {
			windowCapability := windowCapabilities[0]
			windowCapability.Name = "seven-day-history"
			windowCapability.Detail = "用于过去 30 天排位与活跃时段统计"
			capabilities = append(capabilities, windowCapability)
		}
		if len(windowGames) > len(matches) {
			windowMatches = make([]gameplayMatch, 0, len(windowGames))
			for _, game := range windowGames {
				match := normalizeGameplayMatch(game, reference, names, queueLabels)
				if !isCustomGameplayMatch(match) {
					windowMatches = append(windowMatches, match)
				}
			}
		}
		windowExhausted = windowAvailable && len(windowGames) < maximumSummaryMatchCount
		if windowAvailable {
			filterSummary := summarizeLCUGameplayFilters(windowGames)
			a.recordDiagnostic(map[string]any{
				"event": "lcu_summary_history_succeeded", "returned": len(windowGames), "visible": filterSummary.Visible,
				"skipped_empty_participants": filterSummary.SkippedEmptyParticipants, "filtered_custom": filterSummary.FilteredCustom,
				"custom_reasons": filterSummary.CustomReasons, "empty_participant_payloads": filterSummary.EmptyParticipantPayloads,
			})
		}
	}
	capabilities = append(capabilities, masteryCapability)
	masteries := normalizeMasteries(masteryMap, names, 6)
	response.Ranks = ranks
	response.Masteries = masteries
	a.completeOverviewBackground(&response, playerRef)
	response.Capabilities = capabilities
	response.Overall = aggregateMatches(matches, playerRef, nil)
	recentWindowAfter := time.Now().Add(-recentWindowDays * 24 * time.Hour).UnixMilli()
	rankedResult := <-rankedCh
	phases.markSpan("recent_ranked", rankedResult.started, rankedResult.ended)
	rankedSamples := rankedResult.value
	rankedSampleMatches := append(append([]gameplayMatch(nil), rankedSamples.ByQueue[420]...), rankedSamples.ByQueue[440]...)
	response.RecentRanked = recentRankedSummary(rankedSampleMatches, playerRef, nil)
	windowReachedCutoff := len(windowMatches) > 0 && windowMatches[len(windowMatches)-1].CreatedAt < recentWindowAfter
	response.ChampionStats = championStats(matches, playerRef, names)
	defaultRankedMatches := recentRankedMatchesForQueue(rankedSampleMatches, response.RecentRanked.QueueID, defaultMatchCount)
	response.Positions = positionStats(defaultRankedMatches, playerRef)
	response.Ability = buildGameplayAbilityProfile(defaultRankedMatches, playerRef, ranks, reference.Region)
	response.RankedQueues = buildGameplayRankedQueues(rankedSamples.ByQueue[420], rankedSamples.ByQueue[440], playerRef, ranks, reference.Region)
	response.ActivityHours = activityHours(windowMatches)
	// “最近一起玩”需要每场的完整参与者名单，因此基于已读取的详情页
	// 战绩统计，并限定在最近 30 天内。
	response.RecentPlayers = recentPlayers(windowMatches, playerRef, recentWindowAfter)
	usableRecentMatches := 0
	for _, match := range windowMatches {
		if match.CreatedAt > 0 && match.CreatedAt < recentWindowAfter {
			continue
		}
		if len(match.Participants) >= 2 {
			usableRecentMatches++
		}
	}
	a.recordDiagnostic(map[string]any{
		"event": "recent_players_resolved", "window_matches": len(windowMatches), "usable_matches": usableRecentMatches,
		"players": len(response.RecentPlayers), "window_exhausted": windowExhausted, "window_reached_cutoff": windowReachedCutoff,
	})
	spell1Present, spell2Present := 0, 0
	for _, match := range matches {
		if subject, ok := matchSubject(match, playerRef); ok {
			if subject.Spell1ID > 0 {
				spell1Present++
			}
			if subject.Spell2ID > 0 {
				spell2Present++
			}
		}
	}
	a.recordDiagnostic(map[string]any{"event": "overview_loadout_shape", "matches": len(matches), "spell1_present": spell1Present, "spell2_present": spell2Present})
	phases.mark("recent_players")
	a.publicizeOverviewReferences(&response)
	return response
}

func overviewBudgetCapabilities() []EndpointCapability {
	detail := "overview 18 秒软预算已用尽；保留已读取结果，点击继续可补齐"
	return []EndpointCapability{
		{Name: "ranked-stats", State: capabilityFailed, Detail: detail},
		{Name: "champion-mastery", State: capabilityFailed, Detail: detail},
		{Name: "recent-ranked", State: capabilityFailed, Detail: detail},
		{Name: "recent-players", State: capabilityFailed, Detail: detail},
	}
}

func (a *app) loadRecentRankedSamples(ctx context.Context, client *LCUClient, reference gameplayReference, playerRef, matchFilter string, pagination gameplayPagination, matches []gameplayMatch, names map[int64]string, queueLabels map[int64]string) recentRankedSampleSet {
	// matchFilter/pagination belong to the right-side match list. Keep them in
	// the signature for callers, but deliberately ignore them so career samples
	// never inherit the list's current filter or page state.
	_ = matchFilter
	_ = pagination
	result := recentRankedSampleSet{ByQueue: map[int64][]gameplayMatch{
		420: recentRankedMatchesForQueue(matches, 420, defaultMatchCount),
		440: recentRankedMatchesForQueue(matches, 440, defaultMatchCount),
	}}
	if a == nil || a.sgp == nil || reference.Region != "" || reference.ServerID == "" || !validPlayerReference(playerRef) {
		return result
	}
	type queueResult struct {
		queueID int64
		matches []gameplayMatch
	}
	queueResults := make(chan queueResult, 2)
	var group sync.WaitGroup
	for _, queueID := range []int64{420, 440} {
		queueID := queueID
		group.Add(1)
		go func() {
			defer group.Done()
			queueResults <- queueResult{queueID: queueID, matches: a.loadRecentRankedSampleQueue(ctx, client, reference, playerRef, queueID, names, queueLabels, result.ByQueue[queueID])}
		}()
	}
	group.Wait()
	close(queueResults)
	for item := range queueResults {
		result.ByQueue[item.queueID] = item.matches
	}
	return result
}

func (a *app) loadRecentRankedSampleQueue(ctx context.Context, client *LCUClient, reference gameplayReference, playerRef string, queueID int64, names map[int64]string, queueLabels map[int64]string, fallback []gameplayMatch) []gameplayMatch {
	cacheKey := strings.ToUpper(strings.TrimSpace(reference.ServerID)) + "|" + strings.TrimSpace(playerRef) + "|" + strconv.FormatInt(queueID, 10)
	if result, ok := a.recentRankedSample(cacheKey, time.Now()); ok {
		a.recordDiagnostic(map[string]any{"event": "recent_ranked_sample_resolved", "source": "cache", "queue_id": queueID, "matches": len(result)})
		return result
	}
	filter := "solo"
	if queueID == 440 {
		filter = "flex"
	}
	infos, _, _, resolution, err := a.loadSGPMatchHistoryPage(ctx, client, reference.ServerID, playerRef, 0, defaultMatchCount, filter)
	if err != nil || !resolution.ServerFiltered {
		reason := resolution.FallbackReason
		if err != nil {
			reason = safeDiagnosticReason(err)
		}
		a.recordDiagnostic(map[string]any{"event": "recent_ranked_sample_fallback", "queue_id": queueID, "filter": filter, "reason": reason})
		return fallback
	}
	result := make([]gameplayMatch, 0, len(infos))
	for _, info := range infos {
		if info == nil || len(info.Participants) == 0 {
			continue
		}
		match := convertRiotMatchInfo(info, playerRef, names, queueLabels, "", reference.ServerID)
		if !isCustomGameplayMatch(match) {
			result = append(result, match)
		}
	}
	a.cacheRecentRankedSample(cacheKey, recentRankedSampleCacheEntry{at: time.Now(), matches: append([]gameplayMatch(nil), result...)})
	a.recordDiagnostic(map[string]any{"event": "recent_ranked_sample_resolved", "source": "sgp-tag", "queue_id": queueID, "filter": filter, "tag": "q_" + strconv.FormatInt(queueID, 10), "matches": len(result)})
	return result
}

func (a *app) recentRankedSample(key string, now time.Time) ([]gameplayMatch, bool) {
	a.recentRankedSamplesMu.Lock()
	defer a.recentRankedSamplesMu.Unlock()
	cached, ok := a.recentRankedSamples[key]
	if !ok {
		return nil, false
	}
	if now.Sub(cached.at) >= recentRankedSampleCacheTTL {
		a.removeRecentRankedSampleLocked(key)
		return nil, false
	}
	if element := a.recentRankedSampleEntries[key]; element != nil {
		a.recentRankedSampleOrder.MoveToFront(element)
	}
	return append([]gameplayMatch(nil), cached.matches...), true
}

func (a *app) cacheRecentRankedSample(key string, entry recentRankedSampleCacheEntry) {
	a.recentRankedSamplesMu.Lock()
	defer a.recentRankedSamplesMu.Unlock()
	if a.recentRankedSamples == nil {
		a.recentRankedSamples = make(map[string]recentRankedSampleCacheEntry)
	}
	if a.recentRankedSampleOrder == nil {
		a.recentRankedSampleOrder = list.New()
	}
	if a.recentRankedSampleEntries == nil {
		a.recentRankedSampleEntries = make(map[string]*list.Element)
	}
	a.recentRankedSamples[key] = entry
	if element := a.recentRankedSampleEntries[key]; element != nil {
		a.recentRankedSampleOrder.MoveToFront(element)
	} else {
		a.recentRankedSampleEntries[key] = a.recentRankedSampleOrder.PushFront(key)
	}
	for len(a.recentRankedSamples) > recentRankedSampleCacheMax {
		back := a.recentRankedSampleOrder.Back()
		if back == nil {
			break
		}
		a.removeRecentRankedSampleLocked(back.Value.(string))
	}
}

func (a *app) removeRecentRankedSampleLocked(key string) {
	delete(a.recentRankedSamples, key)
	if element := a.recentRankedSampleEntries[key]; element != nil {
		a.recentRankedSampleOrder.Remove(element)
		delete(a.recentRankedSampleEntries, key)
	}
}

func buildGameplayRankedQueues(soloMatches, flexMatches []gameplayMatch, playerRef string, ranks []gameplayRank, region string) map[string]gameplayRankedQueueStats {
	queues := make(map[string]gameplayRankedQueueStats, 2)
	for _, item := range []struct {
		queueID int64
		matches []gameplayMatch
	}{
		{420, soloMatches},
		{440, flexMatches},
	} {
		queueID := item.queueID
		samples := recentRankedMatchesForQueue(item.matches, queueID, defaultMatchCount)
		recent := recentRankedSummaryForQueue(samples, playerRef, nil, queueID)
		ability := buildGameplayAbilityProfileForQueue(samples, playerRef, ranks, region, queueID)
		abilitySampleGames := gameplayAbilitySampleGamesForQueue(samples, playerRef, queueID)
		positions := positionStatsForQueue(samples, playerRef, queueID)
		positionQueueID := queueID
		positionQueueLabel := rankedQueueLabel(queueID)
		queues[strconv.FormatInt(queueID, 10)] = gameplayRankedQueueStats{
			RecentRanked: &recent, Ability: ability, AbilitySampleGames: abilitySampleGames, Positions: positions,
			PositionQueueID: positionQueueID, PositionQueueLabel: positionQueueLabel,
		}
	}
	return queues
}

func recentRankedMatchesForQueue(matches []gameplayMatch, queueID int64, limit int) []gameplayMatch {
	filtered := make([]gameplayMatch, 0, min(len(matches), limit))
	for _, match := range matches {
		if (queueID != 420 && queueID != 440 || match.QueueID == queueID) && (match.Result == "win" || match.Result == "loss") {
			filtered = append(filtered, match)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].CreatedAt > filtered[j].CreatedAt })
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered
}

func positionStatsGames(rows []gameplayPositionStat) int {
	total := 0
	for _, row := range rows {
		total += row.Games
	}
	return total
}

// overviewChampionNames 合并本机客户端目录与 Data Dragon 中文目录：
// 客户端目录在启动初期可能尚未加载完成，导致首次渲染出现“英雄 35”
// 这类占位名，合并线上目录后首屏即可显示正确名称。
func (a *app) overviewChampionNames(ctx context.Context) map[int64]string {
	if a.riot == nil || a.riot.champions == nil {
		return a.championNames()
	}
	boundedCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	return a.riotChampionNames(boundedCtx)
}

func summarizeRiotParticipants(infos []*riotMatchInfo) participantCompletenessSummary {
	summary := participantCompletenessSummary{Counts: make(map[int]int)}
	for _, info := range infos {
		if info == nil {
			continue
		}
		participantCount := len(info.Participants)
		summary.Counts[participantCount]++
		if summary.MinParticipants == 0 || participantCount < summary.MinParticipants {
			summary.MinParticipants = participantCount
		}
		if participantCount > summary.MaxParticipants {
			summary.MaxParticipants = participantCount
		}
		if participantCount == 1 {
			summary.SingleParticipant++
		}
		if riotRosterIncomplete(*info) {
			summary.Incomplete++
		}
		missingInGame := 0
		for _, participant := range info.Participants {
			if strings.TrimSpace(participant.PUUID) == "" {
				summary.MissingPlayerRefs++
				missingInGame++
			}
		}
		if missingInGame > 0 {
			summary.GamesMissingPlayerRef++
		}
	}
	return summary
}

func riotRosterIncomplete(info riotMatchInfo) bool {
	if isCustomGameplayMatch(gameplayMatch{QueueID: info.QueueID, GameMode: info.GameMode, GameType: info.GameType}) {
		return false
	}
	participantCount := len(info.Participants)
	// 斗魂竞技场的队伍和总人数会随版本变化，只把“仅返回本人”视为
	// 明确不完整；其余常规英雄联盟模式应返回完整的十名参与者。
	if isArenaQueue(info.QueueID, info.GameMode) {
		return participantCount <= 1
	}
	return participantCount < 10
}

// loadDetailedMatches 读取带全部参与者的战绩列表。国服客户端的
// match-details 接口新版本只返回本人数据，因此优先走 SGP 网关一次拿到
// 整页完整对局；SGP 不可用时回退到旧的“列表 + 逐场详情”方式。
// loadDetailedMatches 读取带完整参与者的战绩页，并返回分页信息。
// SGP 会跳过缺少 json 的对局，导致一页有效场次可能少于请求数；分页的
// Count 因此使用服务器侧实际消费的条目数，前端据此推进下一页偏移量，
// 避免“一页不满 20 场就误判没有更多”或偏移错位导致的重复。
type matchHistoryFilterResolution struct {
	Filter         string
	ServerFiltered bool
	Fallback       bool
	FallbackReason string
}

func (resolution matchHistoryFilterResolution) pagination(begIndex, count int, more bool) gameplayPagination {
	return gameplayPagination{
		BegIndex: begIndex, Count: count, HasMore: more, Filter: resolution.Filter,
		ServerFiltered: resolution.ServerFiltered, FilterFallback: resolution.Fallback,
		FilterFallbackReason: resolution.FallbackReason,
	}
}

func (a *app) loadSGPMatchHistoryPage(ctx context.Context, client *LCUClient, serverID, playerRef string, begIndex, count int, matchFilter string) ([]*riotMatchInfo, int, bool, matchHistoryFilterResolution, error) {
	matchFilter = normalizeGameplayMatchFilter(matchFilter)
	spec := matchHistoryFilterFor(matchFilter)
	resolution := matchHistoryFilterResolution{Filter: matchFilter}
	if !spec.available() {
		if matchFilter != "all" {
			resolution.Fallback = true
			resolution.FallbackReason = "当前模式没有可验证的单一 SGP tag，已使用客户端筛选"
		}
		infos, consumed, more, err := a.sgp.matchHistoryOn(ctx, client, serverID, playerRef, begIndex, count, true)
		return infos, consumed, more, resolution, err
	}
	state := a.queueFilterCapability(serverID, spec.Key)
	if state != queueFilterCapabilityUnsupported {
		infos, consumed, more, err := a.sgp.matchHistoryFilteredOn(ctx, client, serverID, playerRef, begIndex, count, spec.Tags, true)
		var partialErr *sgpPartialHistoryError
		usablePartial := errors.As(err, &partialErr) && len(infos) > 0
		if (err == nil || usablePartial) && spec.acceptsAll(infos) {
			a.setQueueFilterCapability(serverID, spec.Key, queueFilterCapabilitySupported)
			resolution.ServerFiltered = true
			if begIndex == 0 {
				a.recordDiagnostic(map[string]any{
					"event":  "sgp_match_filter_resolved",
					"filter": matchFilter, "tags": spec.Tags, "state": queueFilterCapabilitySupported,
					"returned": len(infos),
				})
			}
			return infos, consumed, more, resolution, err
		}
		resolution.Fallback = true
		if err != nil {
			resolution.FallbackReason = "SGP tag 请求失败，已使用客户端筛选：" + safeDiagnosticReason(err)
		} else {
			resolution.FallbackReason = "SGP 网关未按 tag 过滤返回结果，已使用客户端筛选"
			a.setQueueFilterCapability(serverID, spec.Key, queueFilterCapabilityUnsupported)
		}
	} else {
		resolution.Fallback = true
		resolution.FallbackReason = "当前 SGP 网关已确认不支持该 tag，已使用客户端筛选"
	}
	if begIndex == 0 {
		a.recordDiagnostic(map[string]any{
			"event":  "sgp_match_filter_fallback",
			"filter": matchFilter, "tags": spec.Tags, "reason": resolution.FallbackReason,
		})
	}
	infos, consumed, more, err := a.sgp.matchHistoryOn(ctx, client, serverID, playerRef, begIndex, count, true)
	return infos, consumed, more, resolution, err
}

func (a *app) loadDetailedMatches(ctx context.Context, client *LCUClient, reference gameplayReference, playerRef string, isCurrent bool, begIndex, count int, matchFilter string, names map[int64]string, queueLabels map[int64]string) ([]gameplayMatch, []EndpointCapability, gameplayPagination) {
	sgpDetail := ""
	filterResolution := matchHistoryFilterResolution{Filter: normalizeGameplayMatchFilter(matchFilter)}
	serverID := reference.ServerID
	if serverID == "" && a.sgp != nil {
		serverID, _, _ = a.sgp.available(client)
	}
	remoteServer := isRemoteTencentServer(client, serverID)
	decision := resolveMatchHistoryDataSources(matchHistoryDataSourceInput{
		PlayerReferenceValid: validPlayerReference(playerRef),
		LCUConnected:         client != nil,
		RemoteServer:         remoteServer,
		SGPAvailable:         a.sgp != nil && serverID != "",
	})
	attempts := make([]DataSourceAttempt, 0, len(decision.Sources)+1)
	fallbackReason := ""
	if len(decision.Sources) == 0 || decision.Sources[0] != dataSourceSGP {
		attempts = append(attempts, DataSourceAttempt{Source: dataSourceSGP, Outcome: dataSourceDisabled, Message: "当前查询没有可用的 SGP 路由"})
		fallbackReason = decision.Reason
	}
	if len(decision.Sources) > 0 && decision.Sources[0] == dataSourceSGP {
		infos, consumed, more, resolvedFilter, historyErr := a.loadSGPMatchHistoryPage(ctx, client, serverID, playerRef, begIndex, count, matchFilter)
		filterResolution = resolvedFilter
		var partialErr *sgpPartialHistoryError
		if errors.As(historyErr, &partialErr) && isCancellation(partialErr) {
			return nil, []EndpointCapability{{Name: "match-history", Path: "sgp: /match-history-query/v1/products/lol/player/{player}/SUMMARY", State: capabilityCanceled, Attempts: attempts}}, filterResolution.pagination(begIndex, 0, false)
		}
		if historyErr == nil || errors.As(historyErr, &partialErr) && len(infos) > 0 {
			attempt := DataSourceAttempt{Source: dataSourceSGP, Outcome: dataSourceSuccess}
			if partialErr != nil {
				attempt.Outcome = dataSourceFailed
				attempt.Message = safeDiagnosticReason(partialErr)
				fallbackReason = "sgp-partial-failed"
			}
			attempts = append(attempts, attempt)
			matches := make([]gameplayMatch, 0, len(infos))
			participantSummary := summarizeRiotParticipants(infos)
			filterSummary := summarizeRiotGameplayFilters(infos)
			for _, info := range infos {
				if info == nil || len(info.Participants) == 0 {
					continue
				}
				a.checkArenaGroupTruth(client, serverID, info)
				match := convertRiotMatchInfo(info, playerRef, names, queueLabels, "", serverID)
				if !isCustomGameplayMatch(match) {
					matches = append(matches, match)
				}
			}
			detailState := capabilityAvailable
			detailText := "通过官方 SGP 网关读取完整对局数据"
			if participantSummary.Incomplete > 0 {
				detailState = capabilityFailed
				detailText = fmt.Sprintf("SGP 返回的 %d 场对局参与者不完整，当前只能展示接口实际返回的数据", participantSummary.Incomplete)
			}
			if partialErr != nil {
				detailState = capabilityFailed
				detailText += "；SGP 分页读取中途失败，本页仅展示已成功返回的对局"
			}
			historyState := capabilityAvailable
			historyDetail := ""
			if partialErr != nil {
				historyState = capabilityFailed
				historyDetail = "SGP 分页读取中途失败，本页仅展示已成功返回的对局"
			}
			capabilities := []EndpointCapability{
				{Name: "match-history", Path: "sgp: /match-history-query/v1/products/lol/player/{player}/SUMMARY", State: historyState, Count: len(matches), Detail: historyDetail, Attempts: attempts, FallbackReason: fallbackReason},
				{Name: "match-details", Path: "sgp: 同一请求返回全部参与者", State: detailState, Count: len(matches), Detail: detailText, Attempts: attempts, FallbackReason: fallbackReason},
			}
			event := "sgp_match_history_succeeded"
			if partialErr != nil {
				event = "sgp_match_history_partial"
			}
			diagnostic := map[string]any{
				"event":    event,
				"returned": len(infos), "visible": len(matches), "consumed": consumed,
				"filter": filterResolution.Filter, "server_filtered": filterResolution.ServerFiltered,
				"skipped_empty_participants": filterSummary.SkippedEmptyParticipants,
				"filtered_custom":            filterSummary.FilteredCustom, "custom_reasons": filterSummary.CustomReasons,
				"incomplete": participantSummary.Incomplete, "single_participant": participantSummary.SingleParticipant,
				"min_participants": participantSummary.MinParticipants, "max_participants": participantSummary.MaxParticipants,
				"participant_counts": participantSummary.Counts, "missing_player_refs": participantSummary.MissingPlayerRefs,
				"games_missing_player_refs": participantSummary.GamesMissingPlayerRef,
			}
			if partialErr != nil {
				diagnostic["reason"] = safeDiagnosticReason(partialErr)
			}
			a.recordDiagnostic(diagnostic)
			pagination := filterResolution.pagination(begIndex, consumed, more)
			pagination.Partial = partialErr != nil
			pagination.BudgetExceeded = partialErr != nil && errors.Is(ctx.Err(), context.DeadlineExceeded)
			if pagination.BudgetExceeded {
				pagination.HasMore = true
			}
			return matches, capabilities, pagination
		} else {
			if isCancellation(historyErr) {
				return nil, []EndpointCapability{{Name: "match-history", Path: "sgp: /match-history-query/v1/products/lol/player/{player}/SUMMARY", State: capabilityCanceled, Attempts: attempts}}, filterResolution.pagination(begIndex, 0, false)
			}
			reason := safeDiagnosticReason(historyErr)
			attempts = append(attempts, DataSourceAttempt{Source: dataSourceSGP, Outcome: dataSourceFailed, Message: reason})
			fallbackReason = "sgp-failed"
			a.recordDiagnostic(map[string]any{
				"event":    "sgp_match_history_failed",
				"fallback": map[bool]string{true: "none", false: "lcu"}[remoteServer],
				"reason":   reason,
			})
			if remoteServer {
				detail := "所选服务器的 SGP 战绩暂时无法读取"
				capabilities := []EndpointCapability{
					{Name: "match-history", Path: "sgp: /match-history-query/v1/products/lol/player/{player}/SUMMARY", State: capabilityFailed, Detail: detail, Attempts: attempts, FallbackReason: fallbackReason},
					{Name: "match-details", Path: "sgp: 同一请求返回全部参与者", State: capabilityFailed, Detail: detail, Attempts: attempts, FallbackReason: fallbackReason},
				}
				return nil, capabilities, filterResolution.pagination(begIndex, 0, false)
			}
			sgpDetail = "SGP 网关读取失败，已回退客户端接口（该接口可能只返回本人数据）：" + reason
		}
	} else if region, platform := client.platformInfo(); strings.EqualFold(region, "TENCENT") {
		if platform == "" {
			sgpDetail = "SGP 不可用：未能从客户端识别所在子服务器（rso_platform_id）"
		} else if _, known := tencentSGPServers[strings.ToUpper(platform)]; !known {
			sgpDetail = "SGP 不可用：未收录的国服子服务器 " + platform
		} else {
			sgpDetail = "SGP 暂时不可用（近期请求失败，稍后自动重试）"
		}
		if begIndex == 0 {
			a.recordDiagnostic(map[string]any{"event": "sgp_match_history_unavailable", "region": strings.ToUpper(region), "reason": sgpDetail})
		}
	}
	if filterResolution.Filter != "all" && !filterResolution.ServerFiltered && !filterResolution.Fallback {
		filterResolution.Fallback = true
		filterResolution.FallbackReason = "SGP 服务端筛选不可用，已使用客户端筛选"
	}
	rawGames, capabilities, total := loadGameplayHistoryContext(ctx, client, playerRef, isCurrent, begIndex, count, true)
	lcuAttempt := DataSourceAttempt{Source: dataSourceLCU, Outcome: dataSourceModeUnsupported, Message: "客户端未返回兼容的战绩列表"}
	lcuCanceled := false
	for index := range capabilities {
		if capabilities[index].Name != "match-history" {
			continue
		}
		switch capabilities[index].State {
		case capabilityAvailable:
			lcuAttempt.Outcome, lcuAttempt.Message = dataSourceSuccess, ""
		case capabilityFailed:
			lcuAttempt.Outcome, lcuAttempt.Message = dataSourceFailed, capabilities[index].Detail
		case capabilityCanceled:
			lcuCanceled = true
		}
		break
	}
	if lcuCanceled {
		for index := range capabilities {
			if capabilities[index].Name == "match-history" || capabilities[index].Name == "match-details" {
				capabilities[index].Attempts = attempts
				capabilities[index].FallbackReason = fallbackReason
			}
		}
		return nil, capabilities, filterResolution.pagination(begIndex, 0, false)
	}
	attempts = append(attempts, lcuAttempt)
	matches := make([]gameplayMatch, 0, len(rawGames))
	for _, game := range rawGames {
		match := normalizeGameplayMatch(game, reference, names, queueLabels)
		if !isCustomGameplayMatch(match) {
			matches = append(matches, match)
		}
	}
	filterSummary := summarizeLCUGameplayFilters(rawGames)
	if lcuAttempt.Outcome == dataSourceSuccess {
		a.recordDiagnostic(map[string]any{
			"event": "lcu_match_history_succeeded", "returned": len(rawGames), "visible": len(matches),
			"skipped_empty_participants": filterSummary.SkippedEmptyParticipants, "filtered_custom": filterSummary.FilteredCustom,
			"custom_reasons": filterSummary.CustomReasons, "empty_participant_payloads": filterSummary.EmptyParticipantPayloads,
		})
	} else {
		a.recordDiagnostic(map[string]any{"event": "lcu_match_history_failed", "reason": lcuAttempt.Message})
	}
	// 把 SGP 不可用的原因写进能力状态：设置页与错误排查都能看到，
	// 不至于只表现为“战绩里只有自己一个人”。
	if sgpDetail != "" {
		updated := false
		for index := range capabilities {
			if capabilities[index].Name != "match-details" {
				continue
			}
			capabilities[index].State = capabilityFailed
			capabilities[index].Path = "sgp: /match-history-query"
			capabilities[index].Detail = sgpDetail
			updated = true
			break
		}
		if !updated {
			capabilities = append(capabilities, EndpointCapability{Name: "match-details", Path: "sgp: /match-history-query", State: capabilityFailed, Detail: sgpDetail})
		}
	}
	for index := range capabilities {
		if capabilities[index].Name == "match-history" || capabilities[index].Name == "match-details" {
			capabilities[index].Attempts = attempts
			capabilities[index].FallbackReason = fallbackReason
		}
	}
	a.recordDiagnostic(map[string]any{
		"event": "match_history_data_source_decision", "reason": decision.Reason,
		"selected": dataSourceLCU, "fallback_reason": fallbackReason, "attempts": attempts,
	})
	pagination := filterResolution.pagination(begIndex, len(rawGames), len(rawGames) == count)
	pagination.Total = total
	return matches, capabilities, pagination
}

func normalizeGameplayReference(reference gameplayReference) gameplayReference {
	reference.PlayerRef = strings.TrimSpace(reference.PlayerRef)
	reference.AlternatePlayerRef = strings.TrimSpace(reference.AlternatePlayerRef)
	if !validPlayerReference(reference.PlayerRef) {
		reference.PlayerRef = ""
	}
	if !validPlayerReference(reference.AlternatePlayerRef) || reference.AlternatePlayerRef == reference.PlayerRef {
		reference.AlternatePlayerRef = ""
	}
	if reference.PlayerRef == "" && reference.AlternatePlayerRef != "" {
		reference.PlayerRef, reference.AlternatePlayerRef = reference.AlternatePlayerRef, ""
	}
	reference.Region = strings.ToLower(strings.TrimSpace(reference.Region))
	reference.Privacy = strings.ToUpper(strings.TrimSpace(reference.Privacy))
	if reference.Privacy != "PUBLIC" && reference.Privacy != "PRIVATE" {
		reference.Privacy = ""
	}
	if reference.Region == riotRegionKR {
		reference.ServerID = ""
	} else if serverID, ok := normalizeTencentServerID(reference.ServerID); ok {
		reference.ServerID = serverID
	} else {
		reference.ServerID = ""
	}
	return reference
}

func deobfuscateHiddenPlayerReference(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return ""
	}
	compact := strings.ReplaceAll(value, "-", "")
	decoded, err := hex.DecodeString(compact)
	if err != nil || len(decoded) != len(hiddenPlayerUUIDMask) {
		return ""
	}
	for index := range decoded {
		decoded[index] ^= hiddenPlayerUUIDMask[index]
	}
	encoded := hex.EncodeToString(decoded)
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
}

func visibleChampSelectPlayerReference(player lcuChampSelectPlayer) string {
	if strings.EqualFold(strings.TrimSpace(player.NameVisibilityType), "HIDDEN") {
		if decoded := deobfuscateHiddenPlayerReference(player.ObfuscatedPUUID); decoded != "" {
			return decoded
		}
	}
	return strings.TrimSpace(player.PUUID)
}

func visibleLivePlayerReference(player lcuLivePlayer) string {
	if strings.EqualFold(strings.TrimSpace(player.NameVisibilityType), "HIDDEN") {
		if decoded := deobfuscateHiddenPlayerReference(player.ObfuscatedPUUID); decoded != "" {
			return decoded
		}
	}
	if value := strings.TrimSpace(player.PUUID); value != "" {
		return value
	}
	return ""
}

func safeDiagnosticReason(err error) string {
	if err == nil {
		return "未知错误"
	}
	message := strings.TrimSpace(err.Error())
	message = diagnosticURLPattern.ReplaceAllStringFunc(message, func(raw string) string {
		parsed, parseErr := url.Parse(raw)
		if parseErr != nil || parsed.Host == "" {
			return "[上游地址已隐藏]"
		}
		return parsed.Scheme + "://" + parsed.Host + "/…"
	})
	message = diagnosticIdentifierPattern.ReplaceAllString(message, "[玩家标识已隐藏]")
	if len([]rune(message)) > 360 {
		message = string([]rune(message)[:360]) + "…"
	}
	if message == "" {
		return "未知错误"
	}
	return message
}

func mergeGameplayReferences(preferred, fallback gameplayReference) gameplayReference {
	preferred = normalizeGameplayReference(preferred)
	fallback = normalizeGameplayReference(fallback)
	if preferred.PlayerRef == "" {
		preferred.PlayerRef = fallback.PlayerRef
	}
	for _, candidate := range []string{fallback.PlayerRef, fallback.AlternatePlayerRef} {
		if preferred.AlternatePlayerRef == "" && candidate != "" && candidate != preferred.PlayerRef {
			preferred.AlternatePlayerRef = candidate
		}
	}
	if preferred.SummonerID == 0 {
		preferred.SummonerID = fallback.SummonerID
	}
	if preferred.AlternateSummonerID == 0 && fallback.AlternateSummonerID != preferred.SummonerID {
		preferred.AlternateSummonerID = fallback.AlternateSummonerID
	}
	if preferred.DisplayName == "" {
		preferred.DisplayName = fallback.DisplayName
	}
	if preferred.GameName == "" {
		preferred.GameName = fallback.GameName
	}
	if preferred.TagLine == "" {
		preferred.TagLine = fallback.TagLine
	}
	if preferred.ProfileIconID == 0 {
		preferred.ProfileIconID = fallback.ProfileIconID
	}
	if preferred.SummonerLevel == 0 {
		preferred.SummonerLevel = fallback.SummonerLevel
	}
	if preferred.Region == "" {
		preferred.Region = fallback.Region
	}
	if preferred.ServerID == "" {
		preferred.ServerID = fallback.ServerID
	}
	if preferred.Privacy == "" {
		preferred.Privacy = fallback.Privacy
	}
	return normalizeGameplayReference(preferred)
}

func gameplayReferenceContains(reference gameplayReference, playerRef string) bool {
	playerRef = strings.TrimSpace(playerRef)
	reference = normalizeGameplayReference(reference)
	return playerRef != "" && (reference.PlayerRef == playerRef || reference.AlternatePlayerRef == playerRef)
}

func gameplayReferencesMatch(left, right gameplayReference) bool {
	left = normalizeGameplayReference(left)
	right = normalizeGameplayReference(right)
	if left.Region != "" && right.Region != "" && left.Region != right.Region {
		return false
	}
	if left.ServerID != "" && right.ServerID != "" && left.ServerID != right.ServerID {
		return false
	}
	if gameplayReferenceContains(left, right.PlayerRef) || gameplayReferenceContains(left, right.AlternatePlayerRef) {
		return true
	}
	for _, leftID := range []int64{left.SummonerID, left.AlternateSummonerID} {
		for _, rightID := range []int64{right.SummonerID, right.AlternateSummonerID} {
			if leftID > 0 && leftID == rightID {
				return true
			}
		}
	}
	return false
}

func gameplayReferenceFromSummoner(summoner Summoner) gameplayReference {
	return normalizeGameplayReference(gameplayReference{
		PlayerRef: summoner.PUUID, SummonerID: summoner.SummonerID,
		DisplayName: summoner.DisplayName, GameName: summoner.GameName, TagLine: summoner.TagLine,
		ProfileIconID: summoner.ProfileIconID, SummonerLevel: summoner.SummonerLevel,
		Privacy: summoner.Privacy,
	})
}

func gameplaySummonerChanged(previous, next Summoner) bool {
	previousPUUID := strings.TrimSpace(previous.PUUID)
	nextPUUID := strings.TrimSpace(next.PUUID)
	if previousPUUID != nextPUUID && (previousPUUID != "" || nextPUUID != "") {
		return true
	}
	return previous.SummonerID != next.SummonerID && (previous.SummonerID != 0 || next.SummonerID != 0)
}

func summonerFromGameplayReference(reference gameplayReference) Summoner {
	reference = normalizeGameplayReference(reference)
	return Summoner{SummonerID: reference.SummonerID, PUUID: reference.PlayerRef, DisplayName: reference.DisplayName, GameName: reference.GameName, TagLine: reference.TagLine, ProfileIconID: reference.ProfileIconID, SummonerLevel: reference.SummonerLevel, Privacy: reference.Privacy}
}

func mergeSummonerIdentity(preferred, fallback Summoner) Summoner {
	if preferred.SummonerID == 0 {
		preferred.SummonerID = fallback.SummonerID
	}
	if preferred.AccountID == 0 {
		preferred.AccountID = fallback.AccountID
	}
	if preferred.PUUID == "" {
		preferred.PUUID = fallback.PUUID
	}
	if preferred.DisplayName == "" {
		preferred.DisplayName = fallback.DisplayName
	}
	if preferred.GameName == "" {
		preferred.GameName = fallback.GameName
	}
	if preferred.TagLine == "" {
		preferred.TagLine = fallback.TagLine
	}
	if preferred.ProfileIconID == 0 {
		preferred.ProfileIconID = fallback.ProfileIconID
	}
	if preferred.SummonerLevel == 0 {
		preferred.SummonerLevel = fallback.SummonerLevel
	}
	return preferred
}

func loadGameplaySummonerUncached(client *LCUClient, reference gameplayReference) (Summoner, EndpointCapability) {
	reference = normalizeGameplayReference(reference)
	capability := EndpointCapability{Name: "summoner", Path: "/lol-summoner/v2/summoners/puuid/{player} 或 /lol-summoner/v1/summoners/{id}"}
	var lastErr error
	for _, playerRef := range []string{reference.PlayerRef, reference.AlternatePlayerRef} {
		if !validPlayerReference(playerRef) {
			continue
		}
		var summoner Summoner
		if err := client.GetJSON("/lol-summoner/v2/summoners/puuid/"+url.PathEscape(playerRef), &summoner); err == nil {
			if summoner.PUUID == "" {
				summoner.PUUID = playerRef
			}
			capability.State = capabilityAvailable
			capability.Count = 1
			return summoner, capability
		} else {
			lastErr = err
		}
	}
	seenIDs := map[int64]bool{}
	for _, summonerID := range []int64{reference.SummonerID, reference.AlternateSummonerID} {
		if summonerID <= 0 || seenIDs[summonerID] {
			continue
		}
		seenIDs[summonerID] = true
		var summoner Summoner
		if err := client.GetJSON(fmt.Sprintf("/lol-summoner/v1/summoners/%d", summonerID), &summoner); err == nil {
			capability.State = capabilityAvailable
			capability.Count = 1
			return summoner, capability
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		capability.State = capabilityUnsupported
		capability.Detail = "当前客户端未提供可查询的玩家身份"
		return Summoner{}, capability
	}
	return Summoner{}, gameplayCapabilityError(capability.Name, capability.Path, lastErr)
}

// Renderer-facing player references are session-scoped aliases. Stable PUUIDs
// and identity hints remain inside the authenticated backend while hidden or
// streamer-mode players use the same ranked and match-history loaders.
type gameplayReferenceCacheItem struct {
	publicRef string
}

func (a *app) registerGameplayReference(playerRef string) string {
	return a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: playerRef})
}

func (a *app) registerGameplayReferenceDetails(reference gameplayReference) string {
	reference = normalizeGameplayReference(reference)
	playerRef := reference.PlayerRef
	if playerRef == "" {
		return ""
	}
	// 同一 Riot 账号可以在多个国服子服务器拥有召唤师资料；服务器身份
	// 必须进入别名作用域，避免同一 PUUID 的两个页签互相覆盖详情。
	digest := sha256.Sum256([]byte(a.token + "\x00" + reference.Region + "\x00" + reference.ServerID + "\x00" + playerRef))
	publicRef := fmt.Sprintf("player_%x", digest[:16])
	a.gameplayRefsMu.Lock()
	a.ensureGameplayReferenceCacheLocked()
	if a.gameplayRefs == nil {
		a.gameplayRefs = make(map[string]string)
	}
	if a.gameplayRefDetails == nil {
		a.gameplayRefDetails = make(map[string]gameplayReference)
	}
	a.gameplayRefs[publicRef] = playerRef
	a.gameplayRefDetails[publicRef] = mergeGameplayReferences(reference, a.gameplayRefDetails[publicRef])
	if element := a.gameplayRefEntries[publicRef]; element != nil {
		a.gameplayRefOrder.MoveToFront(element)
	} else {
		a.gameplayRefEntries[publicRef] = a.gameplayRefOrder.PushFront(gameplayReferenceCacheItem{publicRef: publicRef})
	}
	for len(a.gameplayRefs) > gameplayReferenceCacheMax {
		a.removeGameplayReferenceElementLocked(a.gameplayRefOrder.Back())
	}
	a.gameplayRefsMu.Unlock()
	return publicRef
}

func (a *app) resolveGameplayReference(publicRef string) (string, bool) {
	publicRef = strings.TrimSpace(publicRef)
	a.gameplayRefsMu.Lock()
	playerRef, ok := a.gameplayRefs[publicRef]
	if element := a.gameplayRefEntries[publicRef]; element != nil {
		a.gameplayRefOrder.MoveToFront(element)
	}
	a.gameplayRefsMu.Unlock()
	return playerRef, ok && validPlayerReference(playerRef)
}

func (a *app) resolveGameplayReferenceDetails(publicRef string) (gameplayReference, bool) {
	publicRef = strings.TrimSpace(publicRef)
	a.gameplayRefsMu.Lock()
	playerRef, ok := a.gameplayRefs[publicRef]
	reference := a.gameplayRefDetails[publicRef]
	if element := a.gameplayRefEntries[publicRef]; element != nil {
		a.gameplayRefOrder.MoveToFront(element)
	}
	a.gameplayRefsMu.Unlock()
	if !ok || !validPlayerReference(playerRef) {
		return gameplayReference{}, false
	}
	if reference.PlayerRef == "" {
		reference.PlayerRef = playerRef
	}
	return normalizeGameplayReference(reference), true
}

func (a *app) ensureGameplayReferenceCacheLocked() {
	if a.gameplayRefs == nil {
		a.gameplayRefs = make(map[string]string)
	}
	if a.gameplayRefDetails == nil {
		a.gameplayRefDetails = make(map[string]gameplayReference)
	}
	if a.gameplayRefOrder == nil {
		a.gameplayRefOrder = list.New()
	}
	if a.gameplayRefEntries == nil {
		a.gameplayRefEntries = make(map[string]*list.Element)
	}
}

func (a *app) removeGameplayReferenceElementLocked(element *list.Element) {
	if element == nil {
		return
	}
	item := element.Value.(gameplayReferenceCacheItem)
	delete(a.gameplayRefs, item.publicRef)
	delete(a.gameplayRefDetails, item.publicRef)
	delete(a.gameplayRefEntries, item.publicRef)
	a.gameplayRefOrder.Remove(element)
}

func (a *app) clearGameplayReferences() {
	a.arenaTruth.mu.Lock()
	a.arenaTruth.records = nil
	a.arenaTruth.mu.Unlock()
	a.clearArenaAllies()
	a.clearLiveClientProbe()
	a.gameplayRefsMu.Lock()
	a.gameplayRefs = make(map[string]string)
	a.gameplayRefDetails = make(map[string]gameplayReference)
	a.gameplayRefOrder = list.New()
	a.gameplayRefEntries = make(map[string]*list.Element)
	a.gameplayRefsMu.Unlock()
}

func (a *app) publicizeOverviewReferences(response *gameplayOverview) {
	playerReference := mergeGameplayReferences(response.Player.reference, gameplayReference{PlayerRef: response.Player.PlayerRef, DisplayName: response.Player.DisplayName, GameName: response.Player.GameName, TagLine: response.Player.TagLine, ProfileIconID: response.Player.ProfileIconID, SummonerLevel: response.Player.SummonerLevel, Region: response.Player.Region, ServerID: response.Player.ServerID})
	index := a.proIdentitySnapshot()
	response.Player.ProPlayer = a.matchProIdentity(index, "overview", playerReference)
	response.Player.PlayerRef = a.registerGameplayReferenceDetails(playerReference)
	for matchIndex := range response.Matches {
		a.publicizeMatchReferencesWithProIndex(&response.Matches[matchIndex], index)
	}
	for index := range response.RecentPlayers {
		player := &response.RecentPlayers[index]
		player.PlayerRef = a.registerGameplayReferenceDetails(mergeGameplayReferences(player.reference, gameplayReference{PlayerRef: player.PlayerRef, DisplayName: player.DisplayName, ProfileIconID: player.ProfileIconID}))
	}
}

func (a *app) publicizeMatchReferences(match *gameplayMatch) {
	a.publicizeMatchReferencesWithProIndex(match, a.proIdentitySnapshot())
}

func (a *app) publicizeMatchReferencesWithProIndex(match *gameplayMatch, index proIdentityIndex) {
	if match == nil {
		return
	}
	for playerIndex := range match.Participants {
		participant := &match.Participants[playerIndex]
		reference := mergeGameplayReferences(participant.reference, gameplayReference{
			PlayerRef: participant.PlayerRef, DisplayName: participant.DisplayName, GameName: participant.GameName,
			TagLine: participant.TagLine, ProfileIconID: participant.ProfileIconID,
		})
		participant.ProPlayer = a.matchProIdentity(index, "match", reference)
		participant.PlayerRef = a.registerGameplayReferenceDetails(reference)
	}
}

func gameplayDisplayName(summoner Summoner) string {
	if value := strings.TrimSpace(summoner.GameName); value != "" {
		return value
	}
	if value := strings.TrimSpace(summoner.DisplayName); value != "" {
		return value
	}
	return "隐藏玩家"
}

// loadRanksWithFallback makes cross-source replacement an explicit compatibility
// decision. Local LCU ranked-stats is preferred because it is fast and normally
// returns verified wins/losses; SGP is attempted only when LCU is unavailable,
// incompatible with a remote server, or returns an unverified win/loss pair.
func (a *app) loadRanksWithFallback(ctx context.Context, client *LCUClient, playerRef string, isCurrent bool, serverID, privacy string) ([]gameplayRank, *gameplayRankMilestones, EndpointCapability) {
	if serverID == "" && client != nil {
		serverID = clientTencentServerID(client)
	}
	decision := resolveRankDataSources(rankDataSourceInput{
		PlayerReferenceValid: validPlayerReference(playerRef),
		LCUConnected:         client != nil,
		RemoteServer:         isRemoteTencentServer(client, serverID),
		SGPAvailable:         a.sgp != nil && serverID != "",
	})
	attempts := make([]DataSourceAttempt, 0, len(decision.Sources))
	fallbackReason := ""
	if len(decision.Sources) > 0 && decision.Sources[0] == dataSourceSGP {
		fallbackReason = decision.Reason
	}
	var lcuRanks []gameplayRank
	var lcuMilestones *gameplayRankMilestones
	var lcuCapability EndpointCapability
	for _, source := range decision.Sources {
		switch source {
		case dataSourceLCU:
			ranks, milestones, capability, err := a.loadGameplayRanksContext(ctx, client, playerRef, isCurrent)
			if err != nil {
				if isCancellation(err) {
					canceled := gameplayCapabilityError("ranked-stats", capability.Path, err)
					canceled.Attempts = attempts
					canceled.FallbackReason = fallbackReason
					return nil, nil, canceled
				}
				attempts = append(attempts, DataSourceAttempt{Source: source, Outcome: dataSourceFailed, Message: safeDiagnosticReason(err)})
				fallbackReason = "lcu-failed"
				continue
			}
			if len(ranks) == 0 {
				attempts = append(attempts, DataSourceAttempt{Source: source, Outcome: dataSourceModeUnsupported, Message: "客户端未返回单双排或灵活组排"})
				fallbackReason = "lcu-mode-unsupported"
				continue
			}
			lcuRanks, lcuMilestones, lcuCapability = ranks, milestones, capability
			if !ranksHaveUnverifiedWinRate(ranks) {
				attempts = append(attempts, DataSourceAttempt{Source: source, Outcome: dataSourceSuccess})
				capability.Attempts = attempts
				a.recordRankDataSourceDecision(decision, capability, attempts)
				return ranks, milestones, capability
			}
			attempts = append(attempts, DataSourceAttempt{Source: source, Outcome: dataSourceSuccess, Message: "胜负场未完整验证"})
			fallbackReason = "lcu-unverified-win-loss"
		case dataSourceSGP:
			completionAttempt := lcuRanks != nil && fallbackReason == "lcu-unverified-win-loss"
			if completionAttempt && !a.sgpCompletionGateAllows() {
				fallbackReason = "sgp-completion-gate-closed"
				continue
			}
			ranks, milestones, capability, err := a.loadSGPRanks(ctx, client, serverID, playerRef, isCurrent, privacy)
			if err != nil {
				if isCancellation(err) {
					canceled := gameplayCapabilityError("ranked-stats", capability.Path, err)
					canceled.Attempts = attempts
					canceled.FallbackReason = fallbackReason
					return nil, nil, canceled
				}
				if completionAttempt {
					a.recordSGPCompletion(false)
				}
				attempts = append(attempts, DataSourceAttempt{Source: source, Outcome: dataSourceFailed, Message: safeDiagnosticReason(err)})
				if fallbackReason == "" {
					fallbackReason = "sgp-failed"
				}
				continue
			}
			if len(ranks) == 0 {
				if completionAttempt {
					a.recordSGPCompletion(false)
				}
				attempts = append(attempts, DataSourceAttempt{Source: source, Outcome: dataSourceModeUnsupported, Message: "SGP 未返回单双排或灵活组排"})
				continue
			}
			if completionAttempt {
				a.recordSGPCompletion(!ranksHaveUnverifiedWinRate(ranks))
			}
			attempts = append(attempts, DataSourceAttempt{Source: source, Outcome: dataSourceSuccess})
			if lcuRanks != nil && ranksHaveUnverifiedWinRate(ranks) {
				continue
			}
			capability.Attempts = attempts
			capability.FallbackReason = fallbackReason
			if lcuMilestones != nil {
				milestones = lcuMilestones
			}
			a.recordRankDataSourceDecision(decision, capability, attempts)
			return ranks, milestones, capability
		}
	}
	if lcuRanks != nil {
		lcuCapability.Attempts = attempts
		lcuCapability.FallbackReason = fallbackReason
		a.recordRankDataSourceDecision(decision, lcuCapability, attempts)
		return lcuRanks, lcuMilestones, lcuCapability
	}
	capability := EndpointCapability{Name: "ranked-stats", State: capabilityUnsupported, Detail: "当前没有兼容的排位数据源", Attempts: attempts, FallbackReason: fallbackReason}
	if len(attempts) > 0 {
		capability.State = capabilityFailed
		capability.Detail = "排位数据源均读取失败"
	}
	a.recordRankDataSourceDecision(decision, capability, attempts)
	return nil, nil, capability
}

func (a *app) recordRankDataSourceDecision(decision dataSourceDecision, capability EndpointCapability, attempts []DataSourceAttempt) {
	a.recordDiagnostic(map[string]any{
		"event": "ranked_data_source_decision", "reason": decision.Reason,
		"selected": capabilitySource(capability), "fallback_reason": capability.FallbackReason,
		"attempts": attempts,
	})
}

func (a *app) sgpCompletionGateAllows() bool {
	if a == nil {
		return false
	}
	a.rankedCompletionGateMu.Lock()
	defer a.rankedCompletionGateMu.Unlock()
	return !a.rankedCompletionClosed
}

func (a *app) recordSGPCompletion(success bool) {
	if a == nil {
		return
	}
	a.rankedCompletionGateMu.Lock()
	a.rankedCompletionRecent = append(a.rankedCompletionRecent, success)
	if len(a.rankedCompletionRecent) > sgpCompletionRecentLimit {
		a.rankedCompletionRecent = append([]bool(nil), a.rankedCompletionRecent[len(a.rankedCompletionRecent)-sgpCompletionRecentLimit:]...)
	}
	recentSuccess := 0
	consecutiveFailures := 0
	for index, completed := range a.rankedCompletionRecent {
		if completed {
			recentSuccess++
		}
		if index == len(a.rankedCompletionRecent)-1 {
			for cursor := index; cursor >= 0 && !a.rankedCompletionRecent[cursor]; cursor-- {
				consecutiveFailures++
			}
		}
	}
	wasClosed := a.rankedCompletionClosed
	if consecutiveFailures >= sgpCompletionFailureLimit {
		a.rankedCompletionClosed = true
	}
	closed := a.rankedCompletionClosed
	recentAttempts := len(a.rankedCompletionRecent)
	a.rankedCompletionGateMu.Unlock()

	if recentAttempts == 1 || success || (!wasClosed && closed) {
		state := "open"
		if closed {
			state = "closed"
		}
		a.recordDiagnostic(map[string]any{
			"event": "sgp_completion_gate", "state": state,
			"recent_success": recentSuccess, "recent_attempts": recentAttempts,
		})
	}
}

func ranksHaveUnverifiedWinRate(ranks []gameplayRank) bool {
	for _, rank := range ranks {
		if rank.WinRate < 0 {
			return true
		}
	}
	return false
}

const rankedWinRateDiagnosticHashDomain = "ranked-winrate-player-v1"

// rankedWinRateDiagnosticPlayerHash lets diagnostics distinguish the canonical
// player references fanned out by match-tiers without emitting a raw PUUID.
// The match-tiers path deduplicates canonical refs and playerRankScore keys its
// cache/single-flight by source, server, and ref; this salted hash makes that
// existing fan-out observable when reviewing high-frequency events.
func (a *app) rankedWinRateDiagnosticPlayerHash(playerRef string) string {
	playerRef = strings.TrimSpace(playerRef)
	if playerRef == "" {
		return ""
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(rankedWinRateDiagnosticHashDomain))
	_, _ = hash.Write([]byte{0})
	if a != nil && a.storage != nil && len(a.storage.salt) > 0 {
		_, _ = hash.Write(a.storage.salt)
	} else {
		// Tests and transient startup states can lack a local store. Keep a
		// domain-separated deterministic fallback rather than hashing bare refs.
		_, _ = hash.Write([]byte("no-install-salt"))
	}
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(playerRef))
	digest := hash.Sum(nil)
	return hex.EncodeToString(digest[:8])
}

func (a *app) loadSGPRanks(ctx context.Context, client *LCUClient, serverID, playerRef string, isCurrent bool, privacy string) ([]gameplayRank, *gameplayRankMilestones, EndpointCapability, error) {
	capability := EndpointCapability{Name: "ranked-stats", Path: "sgp: /leagues-ledge/v2/rankedStats"}
	stats, err := a.sgp.rankedStatsOn(ctx, client, serverID, playerRef, isCurrent, privacy)
	if err != nil {
		if !isCancellation(err) {
			a.recordDiagnostic(map[string]any{"event": "sgp_ranked_stats_failed", "fallback": "none", "reason": safeDiagnosticReason(err)})
		}
		return nil, nil, capability, err
	}
	milestones := gameplayRankMilestonesFromSGP(stats)
	ranks := make([]gameplayRank, 0, 2)
	incompleteRecord := false
	for _, entry := range stats.Queues {
		if entry.QueueType != "RANKED_SOLO_5x5" && entry.QueueType != "RANKED_FLEX_SR" {
			continue
		}
		winRate, complete := verifiedRankWinRate(entry.Wins, entry.Losses)
		a.recordDiagnostic(map[string]any{
			"event": "ranked_winrate_resolved", "source": dataSourceSGP, "queue": entry.QueueType,
			"complete": complete, "suppressed": !complete, "wins": entry.Wins, "losses": entry.Losses,
			"tier_empty": strings.TrimSpace(entry.Tier) == "", "player_ref_hash": a.rankedWinRateDiagnosticPlayerHash(playerRef),
		})
		incompleteRecord = incompleteRecord || !complete
		label := "单排/双排"
		if entry.QueueType == "RANKED_FLEX_SR" {
			label = "灵活组排"
		}
		ranks = append(ranks, gameplayRank{
			QueueType: entry.QueueType, QueueLabel: label, Tier: entry.Tier, Division: entry.Rank,
			LeaguePoints: entry.LeaguePoints, Wins: entry.Wins, Losses: entry.Losses, WinRate: winRate,
			Provisional: entry.ProvisionalGamesRemaining > 0,
		})
	}
	sortGameplayRanks(ranks)
	capability.State = capabilityAvailable
	capability.Count = len(ranks)
	if incompleteRecord {
		capability.Detail = "SGP 未返回排位负场，胜率暂不展示"
		a.recordDiagnostic(map[string]any{"event": "sgp_ranked_stats_incomplete", "queue_count": len(ranks)})
	}
	return ranks, milestones, capability, nil
}

func gameplayRankMilestonesFromSGP(stats sgpRankedStats) *gameplayRankMilestones {
	tier := strings.TrimSpace(stats.HighestPreviousSeasonAchievedTier)
	division := strings.TrimSpace(stats.HighestPreviousSeasonAchievedRank)
	if tier == "" {
		tier = strings.TrimSpace(stats.HighestPreviousSeasonEndTier)
		division = strings.TrimSpace(stats.HighestPreviousSeasonEndRank)
	}
	milestones := gameplayRankMilestones{
		PeakTier:     strings.ToLower(tier),
		PeakDivision: division,
	}
	for _, queue := range stats.Queues {
		if queue.QueueType != "RANKED_SOLO_5x5" || strings.TrimSpace(queue.PreviousSeasonEndTier) == "" {
			continue
		}
		milestones.PreviousSeason = append(milestones.PreviousSeason, gameplayPreviousSeasonRank{
			QueueType:   queue.QueueType,
			Tier:        strings.ToLower(strings.TrimSpace(queue.PreviousSeasonEndTier)),
			Division:    strings.TrimSpace(queue.PreviousSeasonEndRank),
			HighestTier: strings.ToLower(strings.TrimSpace(queue.PreviousSeasonHighestTier)),
			HighestDiv:  strings.TrimSpace(queue.PreviousSeasonHighestRank),
		})
		break
	}
	if milestones.PeakTier == "" && len(milestones.PreviousSeason) == 0 {
		return nil
	}
	return &milestones
}

func gameplayRankMilestonesFromLCU(entries []lcuRankedEntry) *gameplayRankMilestones {
	milestones := gameplayRankMilestones{}
	for _, entry := range entries {
		if entry.QueueType != "RANKED_SOLO_5x5" && entry.QueueType != "RANKED_FLEX_SR" {
			continue
		}
		if milestones.PeakTier == "" && strings.TrimSpace(entry.HighestTier) != "" {
			milestones.PeakTier = strings.ToLower(strings.TrimSpace(entry.HighestTier))
			milestones.PeakDivision = strings.TrimSpace(entry.HighestDivision)
		}
		if strings.TrimSpace(entry.PreviousSeasonEndTier) == "" {
			continue
		}
		milestones.PreviousSeason = append(milestones.PreviousSeason, gameplayPreviousSeasonRank{
			QueueType:   entry.QueueType,
			Tier:        strings.ToLower(strings.TrimSpace(entry.PreviousSeasonEndTier)),
			Division:    strings.TrimSpace(entry.PreviousSeasonEndDivision),
			HighestTier: strings.ToLower(strings.TrimSpace(entry.PreviousSeasonHighestTier)),
			HighestDiv:  strings.TrimSpace(entry.PreviousSeasonHighestDivision),
		})
	}
	if milestones.PeakTier == "" && len(milestones.PreviousSeason) == 0 {
		return nil
	}
	return &milestones
}

func rankMilestonesForRegion(region string, milestones *gameplayRankMilestones) *gameplayRankMilestones {
	if strings.TrimSpace(region) != "" {
		return nil
	}
	return milestones
}

// applySeasonRankWinRateFallback fills only records with an incomplete
// win/loss pair, using the already-loaded aggregate for that exact queue.
func (a *app) applySeasonRankWinRateFallback(ranks []gameplayRank, capability EndpointCapability, seasonByQueue map[int64]gameplayAggregate) ([]gameplayRank, EndpointCapability) {
	filled := 0
	filledGames, filledWins, filledLosses := 0, 0, 0
	for index := range ranks {
		if ranks[index].WinRate >= 0 {
			continue
		}
		queueID := int64(0)
		switch ranks[index].QueueType {
		case "RANKED_SOLO_5x5":
			queueID = 420
		case "RANKED_FLEX_SR":
			queueID = 440
		}
		season, ok := seasonByQueue[queueID]
		if !ok || season.Games <= 0 {
			continue
		}
		ranks[index].Wins = season.Wins
		ranks[index].Losses = season.Losses
		ranks[index].WinRate = season.WinRate
		filled++
		filledGames += season.Games
		filledWins += season.Wins
		filledLosses += season.Losses
	}
	if filled == 0 {
		return ranks, capability
	}
	capability.Detail = "上游未返回排位负场，已按赛季战绩聚合补全胜率"
	a.recordDiagnostic(map[string]any{
		"event": "sgp_ranked_stats_season_fallback", "ranks_filled": filled,
		"games": filledGames, "wins": filledWins, "losses": filledLosses,
	})
	return ranks, capability
}

func (a *app) loadGameplayRanks(client *LCUClient, playerRef string, current bool) ([]gameplayRank, EndpointCapability) {
	ranks, _, capability, _ := a.loadGameplayRanksContext(context.Background(), client, playerRef, current)
	return ranks, capability
}

func (a *app) loadGameplayRanksContext(ctx context.Context, client *LCUClient, playerRef string, current bool) ([]gameplayRank, *gameplayRankMilestones, EndpointCapability, error) {
	path := "/lol-ranked/v1/ranked-stats/" + url.PathEscape(playerRef)
	publicPath := "/lol-ranked/v1/ranked-stats/{player}"
	if current {
		path = "/lol-ranked/v1/current-ranked-stats"
		publicPath = path
	}
	capability := EndpointCapability{Name: "ranked-stats", Path: publicPath}
	if client == nil {
		err := errors.New("LCU client unavailable")
		return nil, nil, gameplayCapabilityError(capability.Name, capability.Path, err), err
	}
	var raw json.RawMessage
	if err := client.RequestJSON(ctx, http.MethodGet, path, nil, &raw); err != nil {
		return nil, nil, gameplayCapabilityError(capability.Name, capability.Path, err), err
	}
	var payload lcuRankedStats
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, gameplayCapabilityError(capability.Name, capability.Path, err), err
	}
	queueEntries := rankedQueueEntries(raw)
	a.rankedLCUShapeOnce.Do(func() {
		a.recordDiagnostic(map[string]any{
			"event": "lcu_ranked_stats_shape", "is_current": current, "body_bytes": len(raw),
			"queue_count": len(queueEntries), "top_level_keys": diagnosticKeySet(raw),
			"queue_keys": diagnosticKeyUnion(queueEntries), "sample_queue_values": diagnosticRankedQueueSamples(raw),
		})
	})
	entries := append([]lcuRankedEntry(nil), payload.Queues...)
	if len(entries) == 0 {
		for key, entry := range payload.QueueMap {
			if entry.QueueType == "" {
				entry.QueueType = key
			}
			entries = append(entries, entry)
		}
	}
	ranks := make([]gameplayRank, 0, len(entries))
	incompleteRecord := false
	for _, entry := range entries {
		if entry.QueueType != "RANKED_SOLO_5x5" && entry.QueueType != "RANKED_FLEX_SR" {
			continue
		}
		winRate, complete := verifiedRankWinRate(entry.Wins, entry.Losses)
		a.recordDiagnostic(map[string]any{
			"event": "ranked_winrate_resolved", "source": "lcu", "queue": entry.QueueType,
			"complete": complete, "suppressed": !complete, "wins": entry.Wins, "losses": entry.Losses,
			"tier_empty": strings.TrimSpace(entry.Tier) == "", "player_ref_hash": a.rankedWinRateDiagnosticPlayerHash(playerRef),
		})
		incompleteRecord = incompleteRecord || !complete
		label := "单排/双排"
		if entry.QueueType == "RANKED_FLEX_SR" {
			label = "灵活组排"
		}
		ranks = append(ranks, gameplayRank{QueueType: entry.QueueType, QueueLabel: label, Tier: entry.Tier, Division: entry.Division, LeaguePoints: entry.LeaguePoints, Wins: entry.Wins, Losses: entry.Losses, WinRate: winRate, Provisional: entry.IsProvisional})
	}
	sortGameplayRanks(ranks)
	capability.State = capabilityAvailable
	capability.Count = len(ranks)
	if incompleteRecord {
		capability.Detail = "客户端未返回排位负场，胜率暂不展示"
	}
	return ranks, gameplayRankMilestonesFromLCU(entries), capability, nil
}

func sortGameplayRanks(ranks []gameplayRank) {
	sort.SliceStable(ranks, func(i, j int) bool {
		return ranks[i].QueueType == "RANKED_SOLO_5x5" && ranks[j].QueueType != "RANKED_SOLO_5x5"
	})
}

func verifiedRankWinRate(wins, losses int) (int, bool) {
	// 新版国服 LCU 与部分 leagues-ledge 响应只返回 wins，把 losses
	// 固定成 0。此时不能把不完整记录展示成 100% 胜率。
	if wins > 0 && losses == 0 {
		return -1, false
	}
	total := wins + losses
	if total <= 0 {
		return 0, true
	}
	return int(math.Round(float64(wins) * 100 / float64(total))), true
}

func loadGameplayHistory(client *LCUClient, playerRef string, current bool, begIndex, count int, details bool) ([]lcuGame, []EndpointCapability, int) {
	return loadGameplayHistoryContext(context.Background(), client, playerRef, current, begIndex, count, details)
}

func loadGameplayHistoryContext(ctx context.Context, client *LCUClient, playerRef string, current bool, begIndex, count int, details bool) ([]lcuGame, []EndpointCapability, int) {
	if details {
		count = clampMatchCount(count)
	} else {
		count = clampSummaryMatchCount(count)
	}
	begIndex = clampMatchStart(begIndex)
	endIndex := begIndex + count - 1
	path := fmt.Sprintf("/lol-match-history/v1/products/lol/%s/matches?begIndex=%d&endIndex=%d", url.PathEscape(playerRef), begIndex, endIndex)
	publicPath := "/lol-match-history/v1/products/lol/{player}/matches"
	if current {
		path = fmt.Sprintf("/lol-match-history/v1/products/lol/current-summoner/matches?begIndex=%d&endIndex=%d", begIndex, endIndex)
		publicPath = "/lol-match-history/v1/products/lol/current-summoner/matches"
	}
	var payload lcuMatchHistory
	if err := client.GetJSONContext(ctx, path, &payload); err != nil {
		return nil, []EndpointCapability{gameplayCapabilityError("match-history", publicPath, err)}, 0
	}
	games := append([]lcuGame(nil), payload.Games.Games...)
	if len(games) > count {
		games = games[:count]
	}
	total := payload.Games.GameCount
	if total <= begIndex+len(games) {
		total = 0
	}
	capabilities := []EndpointCapability{{Name: "match-history", Path: publicPath, State: capabilityAvailable, Count: len(games)}}
	if !details || len(games) == 0 {
		return games, capabilities, total
	}
	var wait sync.WaitGroup
	var countMu sync.Mutex
	loaded := 0
	incomplete := 0
	semaphore := make(chan struct{}, 4)
	for index := range games {
		complete := !lcuRosterIncomplete(games[index])
		if games[index].GameID <= 0 || complete {
			countMu.Lock()
			if complete {
				loaded++
			} else {
				incomplete++
			}
			countMu.Unlock()
			continue
		}
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			select {
			case semaphore <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-semaphore }()
			var detail lcuGame
			if err := client.GetJSONContext(ctx, fmt.Sprintf("/lol-match-history/v1/games/%d", games[index].GameID), &detail); err != nil {
				return
			}
			games[index] = detail
			countMu.Lock()
			if lcuRosterIncomplete(detail) {
				incomplete++
			} else {
				loaded++
			}
			countMu.Unlock()
		}(index)
	}
	wait.Wait()
	detailCapability := EndpointCapability{Name: "match-details", Path: "/lol-match-history/v1/games/{gameId}", Count: loaded}
	if isCancellation(ctx.Err()) {
		detailCapability.State = capabilityCanceled
	} else if incomplete > 0 {
		detailCapability.State = capabilityFailed
		detailCapability.Detail = fmt.Sprintf("%d 场对局参与者不完整，当前只能展示客户端实际返回的数据", incomplete)
	} else if loaded == len(games) {
		detailCapability.State = capabilityAvailable
	} else if loaded > 0 {
		detailCapability.State = capabilityFailed
		detailCapability.Detail = "部分对局详情不可用，已保留可核验的摘要"
	} else {
		detailCapability.State = capabilityUnsupported
		detailCapability.Detail = "当前客户端未返回可展开的对局详情"
	}
	capabilities = append(capabilities, detailCapability)
	return games, capabilities, total
}

func lcuRosterIncomplete(game lcuGame) bool {
	if isCustomGameplayMatch(gameplayMatch{QueueID: game.QueueID, GameMode: game.GameMode, GameType: game.GameType}) {
		return false
	}
	participantCount := len(game.Participants)
	identityCount := len(game.ParticipantIdentities)
	if participantCount == 0 || identityCount < participantCount || len(game.Teams) == 0 {
		return true
	}
	if isArenaQueue(game.QueueID, game.GameMode) {
		return participantCount <= 1
	}
	return participantCount < 10
}

const remakeFallbackMaxDurationSeconds int64 = 5 * 60

// Riot 的 gameEndedInEarlySurrender 是重开的主信号；普通投降字段会明确排除重开。
// 部分旧版 LCU/SGP 摘要不带这些字段时，只对极短、无人获胜的匹配对局做保守兜底。
func isRemakeGame(explicit, surrendered bool, duration, queueID int64, gameMode, gameType string, hasWinner bool) bool {
	if explicit {
		return true
	}
	if surrendered || duration <= 0 || duration > remakeFallbackMaxDurationSeconds || hasWinner || queueID <= 0 {
		return false
	}
	mode := strings.ToUpper(strings.TrimSpace(gameMode))
	kind := strings.ToUpper(strings.TrimSpace(gameType))
	if isArenaQueue(queueID, mode) || strings.Contains(kind, "CUSTOM") {
		return false
	}
	// 人机对局可能合法地在五分钟内结束，不能仅凭短时长标记重开。
	if queueID >= 820 && queueID <= 890 {
		return false
	}
	return true
}

func normalizeGameplayMatch(game lcuGame, subject gameplayReference, names map[int64]string, queueLabels map[int64]string) gameplayMatch {
	identities := make(map[int64]lcuParticipantIdentity, len(game.ParticipantIdentities))
	for _, identity := range game.ParticipantIdentities {
		identities[identity.ParticipantID] = identity
	}
	createdAt := normalizeEpochMillis(game.GameCreation)
	if createdAt <= 0 {
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(game.GameCreationDate)); err == nil {
			createdAt = parsed.UnixMilli()
		}
	}
	explicitRemake := game.GameEndedInEarlySurrender
	surrendered := game.GameEndedInSurrender
	hasWinner := false
	for _, raw := range game.Participants {
		explicitRemake = explicitRemake || raw.Stats.GameEndedInEarlySurrender
		surrendered = surrendered || raw.Stats.GameEndedInSurrender
		hasWinner = hasWinner || raw.Stats.Win
	}
	for _, team := range game.Teams {
		hasWinner = hasWinner || strings.EqualFold(strings.TrimSpace(team.Win), "win")
	}
	remake := isRemakeGame(explicitRemake, surrendered, game.GameDuration, game.QueueID, game.GameMode, game.GameType, hasWinner)
	label := queueLabel(game.QueueID, game.GameMode, queueLabels)
	match := gameplayMatch{GameID: game.GameID, CreatedAt: createdAt, Duration: game.GameDuration, QueueID: game.QueueID, QueueLabel: label, ModeGroup: queueModeGroupForLabel(game.QueueID, game.GameMode, game.MapID, label), GameMode: game.GameMode, GameType: game.GameType, MapID: game.MapID}
	if remake {
		match.Result = "remake"
	}
	for _, raw := range game.Participants {
		identity := identities[raw.ParticipantID]
		visiblePlayerRef := strings.TrimSpace(identity.Player.PUUID)
		identityHidden := strings.TrimSpace(identity.Player.GameName) == "" && strings.TrimSpace(identity.Player.SummonerName) == ""
		if identityHidden {
			if decoded := deobfuscateHiddenPlayerReference(identity.Player.ObfuscatedPUUID); decoded != "" {
				visiblePlayerRef = decoded
			}
		}
		reference := normalizeGameplayReference(gameplayReference{
			PlayerRef: visiblePlayerRef, AlternatePlayerRef: identity.Player.ObfuscatedPUUID,
			SummonerID: identity.Player.SummonerID, AlternateSummonerID: identity.Player.ObfuscatedSummonerID,
			GameName: identity.Player.GameName, TagLine: identity.Player.TagLine,
			DisplayName: identity.Player.SummonerName, ProfileIconID: identity.Player.ProfileIcon,
		})
		playerRef := reference.PlayerRef
		name := strings.TrimSpace(identity.Player.GameName)
		if name == "" {
			name = strings.TrimSpace(identity.Player.SummonerName)
		}
		hidden := name == ""
		if hidden {
			name = "隐藏玩家"
		}
		items := itemSlots(raw.Stats.Item0, raw.Stats.Item1, raw.Stats.Item2, raw.Stats.Item3, raw.Stats.Item4, raw.Stats.Item5, raw.Stats.Item6)
		perks := compactPositiveInt64(raw.Stats.Perk0, raw.Stats.Perk1, raw.Stats.Perk2, raw.Stats.Perk3, raw.Stats.Perk4, raw.Stats.Perk5, raw.Stats.StatPerk0, raw.Stats.StatPerk1, raw.Stats.StatPerk2)
		augments := compactPositiveInt64(raw.Stats.PlayerAugment1, raw.Stats.PlayerAugment2, raw.Stats.PlayerAugment3, raw.Stats.PlayerAugment4, raw.Stats.PlayerAugment5, raw.Stats.PlayerAugment6)
		cs := raw.Stats.TotalMinionsKilled + raw.Stats.NeutralMinionsKilled
		participant := gameplayParticipant{
			ParticipantID: raw.ParticipantID, TeamID: raw.TeamID, PlayerRef: playerRef, DisplayName: name,
			GameName: identity.Player.GameName, TagLine: identity.Player.TagLine, ProfileIconID: identity.Player.ProfileIcon,
			ChampionID: raw.ChampionID, ChampionName: championName(names, raw.ChampionID), ChampionLevel: raw.Stats.ChampLevel,
			Spell1ID: raw.Spell1ID, Spell2ID: raw.Spell2ID, PrimaryStyleID: raw.Stats.PerkPrimaryStyle, SubStyleID: raw.Stats.PerkSubStyle,
			PerkIDs: perks, AugmentIDs: augments, ItemIDs: items, Position: normalizePosition(raw.Timeline.Lane, raw.Timeline.Role),
			Kills: raw.Stats.Kills, Deaths: raw.Stats.Deaths, Assists: raw.Stats.Assists,
			KDA: ratio(raw.Stats.Kills+raw.Stats.Assists, raw.Stats.Deaths), CS: cs,
			LaneCS: raw.Stats.TotalMinionsKilled, JungleCS: raw.Stats.NeutralMinionsKilled, CSPerMinute: perMinute(cs, game.GameDuration),
			Gold: raw.Stats.GoldEarned, Damage: raw.Stats.TotalDamageDealtToChampions, DamageTaken: raw.Stats.TotalDamageTaken,
			VisionScore: raw.Stats.VisionScore, WardsPlaced: raw.Stats.WardsPlaced, WardsKilled: raw.Stats.WardsKilled,
			ControlWardsBought: raw.Stats.VisionWardsBoughtInGame,
			Win:                raw.Stats.Win, Hidden: hidden, MultiKill: raw.Stats.LargestMultiKill,
			SubteamID: raw.Stats.PlayerSubteamID, Placement: raw.Stats.SubteamPlacement, reference: reference,
		}
		match.Participants = append(match.Participants, participant)
		if gameplayReferencesMatch(reference, subject) {
			match.SubjectParticipantID = raw.ParticipantID
			if remake {
				continue
			} else if raw.Stats.Win {
				match.Result = "win"
			} else {
				match.Result = "loss"
			}
		}
	}
	if !remake && match.SubjectParticipantID == 0 && len(match.Participants) == 1 {
		match.SubjectParticipantID = match.Participants[0].ParticipantID
		if match.Participants[0].Win {
			match.Result = "win"
		} else {
			match.Result = "loss"
		}
	}
	if match.Result == "" {
		match.Result = "unknown"
	}
	teamByID := make(map[int64]gameplayTeam)
	for _, raw := range game.Teams {
		teamByID[raw.TeamID] = gameplayTeam{TeamID: raw.TeamID, Win: strings.EqualFold(raw.Win, "win"), TowerKills: raw.TowerKills, DragonKills: raw.DragonKills, BaronKills: raw.BaronKills, InhibitorKills: raw.InhibitorKills}
	}
	for _, participant := range match.Participants {
		team := teamByID[participant.TeamID]
		team.TeamID = participant.TeamID
		team.Kills += participant.Kills
		team.Gold += participant.Gold
		team.Damage += participant.Damage
		team.DamageTaken += participant.DamageTaken
		team.VisionScore += participant.VisionScore
		team.CS += participant.CS
		teamByID[participant.TeamID] = team
	}
	for _, team := range teamByID {
		match.Teams = append(match.Teams, team)
	}
	sort.Slice(match.Teams, func(i, j int) bool { return match.Teams[i].TeamID < match.Teams[j].TeamID })
	return match
}

func normalizeMasteries(source map[int64]ChampionMastery, names map[int64]string, limit int) []gameplayMastery {
	items := make([]gameplayMastery, 0, len(source))
	for id, mastery := range source {
		items = append(items, gameplayMastery{ChampionID: id, ChampionName: championName(names, id), ChampionLevel: mastery.ChampionLevel, ChampionPoints: mastery.ChampionPoints})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ChampionPoints > items[j].ChampionPoints })
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func aggregateMatches(matches []gameplayMatch, playerRef string, include func(gameplayMatch) bool) gameplayAggregate {
	var result gameplayAggregate
	var kills, deaths, assists, cs int
	var duration int64
	for _, match := range matches {
		if match.Result != "win" && match.Result != "loss" {
			continue
		}
		if include != nil && !include(match) {
			continue
		}
		participant, ok := matchSubject(match, playerRef)
		if !ok {
			continue
		}
		result.Games++
		if participant.Win {
			result.Wins++
		}
		kills += participant.Kills
		deaths += participant.Deaths
		assists += participant.Assists
		cs += participant.CS
		duration += match.Duration
	}
	result.Losses = result.Games - result.Wins
	if result.Games > 0 {
		result.WinRate = int(math.Round(float64(result.Wins) * 100 / float64(result.Games)))
		result.Kills = round1(float64(kills) / float64(result.Games))
		result.Deaths = round1(float64(deaths) / float64(result.Games))
		result.Assists = round1(float64(assists) / float64(result.Games))
		result.CS = round1(float64(cs) / float64(result.Games))
		result.KDA = round2(ratio(kills+assists, deaths))
		result.CSPerMinute = round1(perMinute(cs, duration))
	}
	return result
}

func recentRankedTeamKills(match gameplayMatch, subject gameplayParticipant) (int, bool) {
	for _, team := range match.Teams {
		if team.TeamID == subject.TeamID && team.Kills > 0 && team.Kills >= subject.Kills {
			return team.Kills, true
		}
	}
	teamPlayers, teamKills := 0, 0
	for _, participant := range match.Participants {
		if participant.TeamID != subject.TeamID {
			continue
		}
		teamPlayers++
		teamKills += participant.Kills
	}
	if teamPlayers < 5 || teamKills <= 0 || teamKills < subject.Kills {
		return 0, false
	}
	return teamKills, true
}

// rankedSample 把两个来源的排位单场归一成同一种形状：首屏那批完整战绩，
// 以及赛季扫描顺带留下的快照（seasonRankedMatch）。首屏样本只有 20 条，
// 玩家如果主玩大乱斗/斗魂，这 20 条里可能只有几场排位——实测日志里就出现过
// 「近 20 场排位 实际 8 场」。赛季缓存能把样本补到真正的最近 20 场排位，
// 而且是零额外网络开销（那些对局赛季统计本来就要下载解析一遍）。
type rankedSample struct {
	gameID    int64
	createdAt int64
	queueID   int64
	win       bool
	kills     int
	deaths    int
	assists   int
	teamKills int
	hasTeam   bool
	position  string
}

func recentRankedSummary(matches []gameplayMatch, playerRef string, cached []seasonRankedMatch) gameplayRecentRankedSummary {
	samples := make([]rankedSample, 0, len(matches)+len(cached))
	seen := make(map[int64]bool, len(matches)+len(cached))
	for _, match := range matches {
		if match.QueueID != 420 && match.QueueID != 440 || match.Result != "win" && match.Result != "loss" {
			continue
		}
		subject, ok := matchSubject(match, playerRef)
		if !ok {
			continue
		}
		sample := rankedSample{
			gameID: match.GameID, createdAt: match.CreatedAt, queueID: match.QueueID, win: match.Result == "win",
			kills: subject.Kills, deaths: subject.Deaths, assists: subject.Assists,
			position: strings.ToLower(strings.TrimSpace(subject.Position)),
		}
		if teamKills, ok := recentRankedTeamKills(match, subject); ok {
			sample.teamKills, sample.hasTeam = teamKills, true
		}
		if sample.gameID > 0 {
			if seen[sample.gameID] {
				continue
			}
			seen[sample.gameID] = true
		}
		samples = append(samples, sample)
	}
	for _, item := range cached {
		if item.QueueID != 420 && item.QueueID != 440 {
			continue
		}
		if item.GameID > 0 && seen[item.GameID] {
			continue
		}
		if item.GameID > 0 {
			seen[item.GameID] = true
		}
		samples = append(samples, rankedSample{
			gameID: item.GameID, createdAt: item.CreatedAt, queueID: item.QueueID, win: item.Win,
			kills: item.Kills, deaths: item.Deaths, assists: item.Assists,
			teamKills: item.TeamKills, hasTeam: item.TeamKills > 0,
			position: strings.ToLower(strings.TrimSpace(item.Position)),
		})
	}
	selectedQueue := int64(0)
	for _, sample := range samples {
		if sample.queueID == 420 {
			selectedQueue = 420
			break
		}
	}
	if selectedQueue == 0 {
		for _, sample := range samples {
			if sample.queueID == 440 {
				selectedQueue = 440
				break
			}
		}
	}
	filtered := samples[:0]
	for _, sample := range samples {
		if selectedQueue == 0 || sample.queueID == selectedQueue {
			filtered = append(filtered, sample)
		}
	}
	samples = filtered
	sort.SliceStable(samples, func(i, j int) bool { return samples[i].createdAt > samples[j].createdAt })
	if len(samples) > defaultMatchCount {
		samples = samples[:defaultMatchCount]
	}

	type positionCounter struct{ games, wins int }
	positionCounters := make(map[string]*positionCounter)
	result := gameplayRecentRankedSummary{QueueID: selectedQueue, QueueLabel: rankedQueueLabel(selectedQueue)}
	var kills, deaths, assists int
	var participationKills, participationTeamKills int
	for _, sample := range samples {
		result.Games++
		if sample.win {
			result.Wins++
		}
		kills += sample.kills
		deaths += sample.deaths
		assists += sample.assists
		if sample.hasTeam && sample.teamKills > 0 {
			result.KillParticipationGames++
			participationKills += sample.kills + sample.assists
			participationTeamKills += sample.teamKills
		}

		position := sample.position
		if position == "" || position == "other" {
			continue
		}
		counter := positionCounters[position]
		if counter == nil {
			counter = &positionCounter{}
			positionCounters[position] = counter
		}
		counter.games++
		if sample.win {
			counter.wins++
		}
	}
	result.Losses = result.Games - result.Wins
	if result.Games > 0 {
		result.WinRate = int(math.Round(float64(result.Wins) * 100 / float64(result.Games)))
		result.Kills = round1(float64(kills) / float64(result.Games))
		result.Deaths = round1(float64(deaths) / float64(result.Games))
		result.Assists = round1(float64(assists) / float64(result.Games))
		result.KDA = round2(ratio(kills+assists, deaths))
	}
	if participationTeamKills > 0 {
		result.KillParticipation = round1(float64(participationKills) * 100 / float64(participationTeamKills))
	}

	labels := map[string]string{"top": "上路", "jungle": "打野", "middle": "中路", "bottom": "下路", "utility": "辅助"}
	positionOrder := map[string]int{"top": 0, "jungle": 1, "middle": 2, "bottom": 3, "utility": 4}
	for position, counter := range positionCounters {
		label, ok := labels[position]
		if !ok || counter.games == 0 {
			continue
		}
		result.Positions = append(result.Positions, gameplayPositionWinRate{
			Position: position, Label: label, Games: counter.games, Wins: counter.wins,
			WinRate: int(math.Round(float64(counter.wins) * 100 / float64(counter.games))),
		})
	}
	sort.Slice(result.Positions, func(i, j int) bool {
		if result.Positions[i].Games != result.Positions[j].Games {
			return result.Positions[i].Games > result.Positions[j].Games
		}
		return positionOrder[result.Positions[i].Position] < positionOrder[result.Positions[j].Position]
	})
	if len(result.Positions) > 2 {
		result.Positions = result.Positions[:2]
	}
	return result
}

func recentRankedSummaryForQueue(matches []gameplayMatch, playerRef string, cached []seasonRankedMatch, queueID int64) gameplayRecentRankedSummary {
	if queueID != 420 && queueID != 440 {
		return recentRankedSummary(matches, playerRef, cached)
	}
	filteredMatches := make([]gameplayMatch, 0, len(matches))
	for _, match := range matches {
		if match.QueueID == queueID {
			filteredMatches = append(filteredMatches, match)
		}
	}
	filteredCached := make([]seasonRankedMatch, 0, len(cached))
	for _, item := range cached {
		if item.QueueID == queueID {
			filteredCached = append(filteredCached, item)
		}
	}
	result := recentRankedSummary(filteredMatches, playerRef, filteredCached)
	if result.QueueID == 0 {
		result.QueueID = queueID
		result.QueueLabel = rankedQueueLabel(queueID)
	}
	return result
}

func rankedQueueLabel(queueID int64) string {
	if queueID == 440 {
		return "灵活组排"
	}
	if queueID == 420 {
		return "单双排"
	}
	return ""
}

func championStats(matches []gameplayMatch, playerRef string, names map[int64]string) []gameplayChampionStat {
	type counter struct {
		games, wins, kills, deaths, assists, cs int
		duration                                int64
	}
	counters := make(map[int64]*counter)
	for _, match := range matches {
		if match.Result != "win" && match.Result != "loss" {
			continue
		}
		if match.QueueID != 420 && match.QueueID != 440 {
			continue
		}
		participant, ok := matchSubject(match, playerRef)
		if !ok || participant.ChampionID <= 0 {
			continue
		}
		item := counters[participant.ChampionID]
		if item == nil {
			item = &counter{}
			counters[participant.ChampionID] = item
		}
		item.games++
		if participant.Win {
			item.wins++
		}
		item.kills += participant.Kills
		item.deaths += participant.Deaths
		item.assists += participant.Assists
		item.cs += participant.CS
		item.duration += match.Duration
	}
	result := make([]gameplayChampionStat, 0, len(counters))
	for id, item := range counters {
		result = append(result, gameplayChampionStat{ChampionID: id, ChampionName: championName(names, id), Games: item.games, Wins: item.wins, WinRate: int(math.Round(float64(item.wins) * 100 / float64(item.games))), Kills: round1(float64(item.kills) / float64(item.games)), Deaths: round1(float64(item.deaths) / float64(item.games)), Assists: round1(float64(item.assists) / float64(item.games)), KDA: round2(ratio(item.kills+item.assists, item.deaths)), CS: round1(float64(item.cs) / float64(item.games)), CSPerMinute: round1(perMinute(item.cs, item.duration))})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Games != result[j].Games {
			return result[i].Games > result[j].Games
		}
		return result[i].WinRate > result[j].WinRate
	})
	return result
}

func positionStats(matches []gameplayMatch, playerRef string) []gameplayPositionStat {
	labels := map[string]string{"top": "上单", "jungle": "打野", "middle": "中单", "bottom": "下路", "utility": "辅助"}
	counts := map[string]int{}
	total := 0
	for _, match := range matches {
		participant, ok := matchSubject(match, playerRef)
		position := strings.ToLower(strings.TrimSpace(participant.Position))
		if !ok || labels[position] == "" {
			continue
		}
		counts[position]++
		total++
	}
	order := []string{"top", "jungle", "middle", "bottom", "utility"}
	result := make([]gameplayPositionStat, 0, len(order))
	for _, position := range order {
		games := counts[position]
		share := 0
		if total > 0 {
			share = int(math.Round(float64(games) * 100 / float64(total)))
		}
		result = append(result, gameplayPositionStat{Position: position, Label: labels[position], Games: games, Share: share})
	}
	return result
}

func positionStatsForQueue(matches []gameplayMatch, playerRef string, queueID int64) []gameplayPositionStat {
	if queueID != 420 && queueID != 440 {
		return positionStats(matches, playerRef)
	}
	filtered := make([]gameplayMatch, 0, len(matches))
	for _, match := range matches {
		if match.QueueID == queueID {
			filtered = append(filtered, match)
		}
	}
	return positionStats(filtered, playerRef)
}

// Reuse the already-fetched latest ten games; never infer a role from a champion.
func liveRecentPositions(matches []gameplayMatch, playerRef string, queueID int64) []gameplayPositionStat {
	if queueID != 420 && queueID != 440 {
		return nil
	}
	if len(matches) > 10 {
		matches = matches[:10]
	}
	var result []gameplayPositionStat
	for _, stat := range positionStatsForQueue(matches, playerRef, queueID) {
		if stat.Games > 0 {
			result = append(result, stat)
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Games > result[j].Games })
	return result
}

func activityHours(matches []gameplayMatch) []int {
	hours := make([]int, 24)
	for _, match := range matches {
		if match.CreatedAt <= 0 {
			continue
		}
		hours[time.UnixMilli(match.CreatedAt).Local().Hour()]++
	}
	return hours
}

func recentPlayers(matches []gameplayMatch, subjectRef string, sinceMillis int64) []gameplayRecentPlayer {
	byRef := map[string]*gameplayRecentPlayer{}
	for _, match := range matches {
		if sinceMillis > 0 && match.CreatedAt > 0 && match.CreatedAt < sinceMillis {
			continue
		}
		for _, participant := range match.Participants {
			if participant.ParticipantID == match.SubjectParticipantID || participant.PlayerRef != "" && participant.PlayerRef == subjectRef {
				continue
			}
			key := participant.PlayerRef
			if key == "" {
				gameName := strings.TrimSpace(participant.GameName)
				tagLine := strings.TrimSpace(participant.TagLine)
				displayName := strings.TrimSpace(participant.DisplayName)
				switch {
				case gameName != "":
					key = "riot:" + strings.ToLower(gameName) + "\x00" + strings.ToLower(tagLine)
				case displayName != "":
					key = "display:" + strings.ToLower(displayName)
				default:
					continue
				}
			}
			item := byRef[key]
			if item == nil {
				item = &gameplayRecentPlayer{PlayerRef: participant.PlayerRef, DisplayName: participant.DisplayName, ProfileIconID: participant.ProfileIconID, Hidden: participant.Hidden, reference: participant.reference}
				byRef[key] = item
			}
			item.Games++
		}
	}
	items := make([]gameplayRecentPlayer, 0, len(byRef))
	for _, item := range byRef {
		items = append(items, *item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Games > items[j].Games })
	if len(items) > 6 {
		items = items[:6]
	}
	return items
}

func matchSubject(match gameplayMatch, playerRef string) (gameplayParticipant, bool) {
	for _, participant := range match.Participants {
		if participant.ParticipantID == match.SubjectParticipantID || (playerRef != "" && participant.PlayerRef == playerRef) {
			return participant, true
		}
	}
	return gameplayParticipant{}, false
}

type gameplayLiveResponse struct {
	Phase                    string                    `json:"phase"`
	Available                bool                      `json:"available"`
	Unsupported              bool                      `json:"unsupported"`
	UnsupportedReason        string                    `json:"unsupportedReason,omitempty"`
	GameID                   int64                     `json:"gameId,omitempty"`
	QueueID                  int64                     `json:"queueId,omitempty"`
	QueueLabel               string                    `json:"queueLabel,omitempty"`
	GameMode                 string                    `json:"gameMode,omitempty"`
	MapID                    int64                     `json:"mapId,omitempty"`
	CurrentChampionID        int64                     `json:"currentChampionId,omitempty"`
	ResolvedChampionID       int64                     `json:"resolvedChampionId,omitempty"`
	ChampSelectNotice        string                    `json:"champSelectNotice,omitempty"`
	ArenaGroupingRetryable   bool                      `json:"arenaGroupingRetryable,omitempty"`
	ArenaGroupingUnavailable bool                      `json:"arenaGroupingUnavailable,omitempty"`
	ArenaMySquadNotice       string                    `json:"arenaMySquadNotice,omitempty"`
	ArenaGrouped             bool                      `json:"arenaGrouped,omitempty"`
	ArenaMascotMapping       bool                      `json:"arenaMascotMapping,omitempty"`
	ArenaGroupSource         string                    `json:"arenaGroupSource,omitempty"`
	ArenaGroupNames          map[string]string         `json:"arenaGroupNames,omitempty"`
	DroppedStaleGameData     bool                      `json:"droppedStaleGameData,omitempty"`
	Players                  []gameplayLivePlayer      `json:"players"`
	ClientRecommendation     *gameplayRecommendation   `json:"clientRecommendation,omitempty"`
	ChampionAbilities        []gameplayChampionAbility `json:"championAbilities,omitempty"`
	Capabilities             []EndpointCapability      `json:"capabilities"`
	RawCount                 int                       `json:"-"`
	MergeAppended            int                       `json:"-"`
}

type gameplayRecommendationsResponse struct {
	ChampionID      int64                        `json:"championId"`
	Position        string                       `json:"position"`
	Recommendations gameplayRecommendationBundle `json:"recommendations"`
}

type gameplayRecommendationBundle struct {
	TraceID              string                      `json:"traceId,omitempty"`
	RecommendationKey    string                      `json:"recommendationKey,omitempty"`
	RequestedPosition    string                      `json:"requestedPosition,omitempty"`
	QueueID              int64                       `json:"queueId,omitempty"`
	MapID                int64                       `json:"mapId,omitempty"`
	GameID               int64                       `json:"gameId,omitempty"`
	GameMode             string                      `json:"gameMode,omitempty"`
	Tier                 string                      `json:"tier,omitempty"`
	Source               string                      `json:"source"`
	ResolvedMode         string                      `json:"resolvedMode"`
	ResolvedRegion       string                      `json:"resolvedRegion"`
	DataVersion          string                      `json:"dataVersion,omitempty"`
	CurrentVersion       string                      `json:"currentVersion,omitempty"`
	IsFallback           bool                        `json:"isFallback"`
	IsStale              bool                        `json:"isStale"`
	HasRunes             bool                        `json:"hasRunes"`
	HasAugments          bool                        `json:"hasAugments"`
	HasCounters          bool                        `json:"hasCounters"`
	HasBanRate           bool                        `json:"hasBanRate"`
	HasTopPlayers        bool                        `json:"hasTopPlayers"`
	HasItemDepths        bool                        `json:"hasItemDepths"`
	Citation             *championSourceCitation     `json:"citation,omitempty"`
	MeasurementTechnique string                      `json:"measurementTechnique,omitempty"`
	Positions            []championPositionOption    `json:"positions,omitempty"`
	ResolvedPosition     string                      `json:"resolvedPosition"`
	PositionSource       string                      `json:"positionSource"`
	Hero                 gameplayRecommendationHero  `json:"hero"`
	Runes                gameplayRecommendationRunes `json:"runes"`
	Augments             []championMetricRow         `json:"augments"`
	ItemRanking          []championMetricRow         `json:"itemRanking,omitempty"`
	Build                gameplayRecommendationBuild `json:"build"`
}

type gameplayRecommendationHero struct {
	Tier          *int                            `json:"tier,omitempty"`
	WinRate       float64                         `json:"winRate,omitempty"`
	PickRate      float64                         `json:"pickRate,omitempty"`
	BanRate       float64                         `json:"banRate,omitempty"`
	EmptyReason   string                          `json:"emptyReason,omitempty"`
	StrongAgainst []gameplayRecommendationMatchup `json:"strongAgainst,omitempty"`
	WeakAgainst   []gameplayRecommendationMatchup `json:"weakAgainst,omitempty"`
}

type gameplayRecommendationMatchup struct {
	ChampionID   int64   `json:"championId"`
	ChampionName string  `json:"championName"`
	WinRate      float64 `json:"winRate,omitempty"`
	Games        int     `json:"games,omitempty"`
}

type gameplayRecommendationRunes struct {
	OPGG        []gameplayRecommendationRune `json:"opgg"`
	Specialists []gameplayRecommendationRune `json:"specialists"`
	Pros        []gameplayRecommendationRune `json:"pros"`
}

type gameplayRecommendationRune struct {
	Key                  string                      `json:"key"`
	Title                string                      `json:"title"`
	ChampionID           int64                       `json:"championId"`
	ChampionName         string                      `json:"championName,omitempty"`
	PrimaryStyleID       int64                       `json:"primaryStyleId"`
	SubStyleID           int64                       `json:"subStyleId"`
	SelectedPerkIDs      []int64                     `json:"selectedPerkIds"`
	StatModIDs           []int64                     `json:"statModIds,omitempty"`
	ShardResolution      string                      `json:"shardResolution,omitempty"`
	ItemIDs              []int64                     `json:"itemIds,omitempty"`
	Stats                gameplayRecommendationStats `json:"stats"`
	PlayerName           string                      `json:"playerName,omitempty"`
	TagLine              string                      `json:"tagLine,omitempty"`
	Tier                 string                      `json:"tier,omitempty"`
	Division             string                      `json:"division,omitempty"`
	LeaguePoints         string                      `json:"leaguePoints,omitempty"`
	ChampionGames        int                         `json:"championGames,omitempty"`
	PlayedAt             int64                       `json:"playedAt,omitempty"`
	Result               string                      `json:"result,omitempty"`
	Region               string                      `json:"region,omitempty"`
	OpponentChampionID   int64                       `json:"opponentChampionId,omitempty"`
	OpponentChampionName string                      `json:"opponentChampionName,omitempty"`
	OpponentPlayerName   string                      `json:"opponentPlayerName,omitempty"`
	OpponentTagLine      string                      `json:"opponentTagLine,omitempty"`
	OpponentTier         string                      `json:"opponentTier,omitempty"`
	OpponentDivision     string                      `json:"opponentDivision,omitempty"`
	OpponentWinRate      *int                        `json:"opponentWinRate,omitempty"`
	Position             string                      `json:"position,omitempty"`
	SourceKey            string                      `json:"sourceKey,omitempty"`
	EventLabel           string                      `json:"eventLabel,omitempty"`
	WinKnown             *bool                       `json:"winKnown,omitempty"`
	Win                  *bool                       `json:"win,omitempty"`
	SelectedComplete     *bool                       `json:"selectedComplete,omitempty"`
	RecordGames          int                         `json:"recordGames,omitempty"`
	RecordWins           int                         `json:"recordWins,omitempty"`
	RecordPartial        bool                        `json:"recordPartial,omitempty"`
	CacheReadAt          string                      `json:"cacheReadAt,omitempty"`
	opponentPUUID        string
}

type gameplayRecommendationStats struct {
	PickRate         *float64 `json:"pickRate"`
	WinRate          *float64 `json:"winRate"`
	Games            *int     `json:"games"`
	AveragePlacement *float64 `json:"averagePlacement,omitempty"`
	FirstPlaceRate   *float64 `json:"firstPlaceRate,omitempty"`
}

type gameplayRecommendationBuild struct {
	Position        string                         `json:"position"`
	SkillPriority   []string                       `json:"skillPriority,omitempty"`
	SkillOrder      []string                       `json:"skillOrder,omitempty"`
	SkillStats      gameplayRecommendationStats    `json:"skillStats"`
	SpellOptions    []gameplayRecommendationOption `json:"spellOptions"`
	StarterOptions  []gameplayRecommendationOption `json:"starterOptions"`
	BootOptions     []gameplayRecommendationOption `json:"bootOptions"`
	CoreOptions     []gameplayRecommendationOption `json:"coreOptions"`
	FourthOptions   []gameplayRecommendationOption `json:"fourthOptions,omitempty"`
	FifthOptions    []gameplayRecommendationOption `json:"fifthOptions,omitempty"`
	SixthOptions    []gameplayRecommendationOption `json:"sixthOptions,omitempty"`
	PrismOptions    []gameplayRecommendationOption `json:"prismOptions,omitempty"`
	ItemSource      string                         `json:"itemSource,omitempty"`
	ItemWindow      string                         `json:"itemWindow,omitempty"`
	ItemChainStatus string                         `json:"itemChainStatus,omitempty"`
	FourthSample    int                            `json:"fourthSample,omitempty"`
	FifthSample     int                            `json:"fifthSample,omitempty"`
	SixthSample     int                            `json:"sixthSample,omitempty"`
}

type gameplayRecommendationOption struct {
	IDs              []int64                     `json:"ids"`
	Grade            string                      `json:"grade,omitempty"`
	GamesUnavailable bool                        `json:"gamesUnavailable,omitempty"`
	Stats            gameplayRecommendationStats `json:"stats"`
}

type gameplayRecommendationModeResolution struct {
	InternalMode string
	IsFallback   bool
	QueueID      int64
}

const (
	maxRecommendationSmallID  int64 = 100000
	maxRecommendationGameID   int64 = 1 << 62
	gameModeARAM                    = "ARAM"
	gameModeKIWI                    = "KIWI"
	gameModeARAMMayhem              = "ARAM_MAYHEM"
	gameModeARAMMayhemClassic       = "ARAM_MAYHEM_CLASSIC"
)

func isARAMFamilyGameMode(gameMode string) bool {
	switch strings.ToUpper(strings.TrimSpace(gameMode)) {
	case gameModeARAM, gameModeKIWI, gameModeARAMMayhem, gameModeARAMMayhemClassic:
		return true
	default:
		return false
	}
}

func recommendationModeHasTopPlayers(resolution gameplayRecommendationModeResolution) bool {
	// Top-player data belongs to the semantic Summoner's Rift classic mode.
	// Queue IDs are deliberately not enumerated: Tencent custom games have
	// already appeared as -1, 0 and 3100.
	return resolution.InternalMode == "ranked"
}

func resolveGameplayRecommendationMode(queueID int64, gameMode string, mapID int64) gameplayRecommendationModeResolution {
	resolved := gameplayRecommendationModeResolution{QueueID: queueID}
	mode := strings.ToUpper(strings.TrimSpace(gameMode))
	// Classic Mayhem has no dedicated OP.GG dataset. Resolve it to ordinary
	// ARAM recommendations before queue-ID fallbacks so queue 2400 is not
	// mistaken for the augment-enabled mode when the semantic field is present.
	if mode == gameModeARAMMayhemClassic && (mapID == 0 || mapID == 12) {
		resolved.InternalMode, resolved.IsFallback = "aram", true
		return resolved
	}
	if definition, ok := supportedQueueDefinition(queueID); ok {
		resolved.InternalMode = definition.RecommendationMode
	}
	if resolved.InternalMode != "" {
		return resolved
	}
	switch {
	case (mode == gameModeKIWI || mode == gameModeARAMMayhem) && mapID == 12:
		resolved.InternalMode = "hextech-aram"
	case mode == "CHERRY" && mapID == 30:
		resolved.InternalMode = "arena"
	case mode == gameModeARAM && mapID == 12:
		resolved.InternalMode = "aram"
	case (mode == "URF" || mode == "ARURF") && mapID == 11:
		resolved.InternalMode = "urf"
	case mode == "NEXUSBLITZ" && mapID == 21:
		resolved.InternalMode = "nexus-blitz"
	case mode == "CLASSIC" && mapID == 11:
		resolved.InternalMode = "ranked"
	default:
		resolved.InternalMode, resolved.IsFallback = "unsupported", true
	}
	return resolved
}

func (a *app) recordRecommendationModeResolution(gameID, queueID int64, gameMode string, mapID int64, resolution gameplayRecommendationModeResolution) {
	if a == nil {
		return
	}
	key := fmt.Sprintf("shape:%d:%s:%d", queueID, strings.ToUpper(strings.TrimSpace(gameMode)), mapID)
	if gameID > 0 {
		key = "game:" + strconv.FormatInt(gameID, 10)
	}
	a.recommendationModeDiagnosticMu.Lock()
	if a.recommendationModeDiagnosticKeys == nil {
		a.recommendationModeDiagnosticKeys = make(map[string]struct{})
	}
	if _, recorded := a.recommendationModeDiagnosticKeys[key]; recorded {
		a.recommendationModeDiagnosticMu.Unlock()
		return
	}
	if len(a.recommendationModeDiagnosticKeys) >= 64 {
		a.recommendationModeDiagnosticKeys = make(map[string]struct{})
	}
	a.recommendationModeDiagnosticKeys[key] = struct{}{}
	a.recommendationModeDiagnosticMu.Unlock()
	a.recordDiagnostic(map[string]any{
		"event": "recommendation_mode_resolved", "queue_id": queueID,
		"game_mode": strings.ToUpper(strings.TrimSpace(gameMode)), "map_id": mapID,
		"internal_mode": resolution.InternalMode, "has_top_players": recommendationModeHasTopPlayers(resolution),
		"has_item_depths": opggModeSpecs[resolution.InternalMode].SupportsItemDepths,
	})
}

func (a *app) handleGameplayRecommendations(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	query := r.URL.Query()
	queueIDForDiagnostic, _ := parseOptionalRecommendationInt(query.Get("queueId"))
	championIDForDiagnostic, _ := parseOptionalRecommendationInt(query.Get("championId"))
	mapIDForDiagnostic, _ := parseOptionalRecommendationInt(query.Get("mapId"))
	gameIDForDiagnostic, _ := parseOptionalGameID(query.Get("gameId"))
	traceID := normalizeItemSetTraceID(query.Get("traceId"))
	if traceID == "" {
		traceID = newItemSetTraceID("recommendation")
	}
	recommendationKey := truncateClientDiagnosticText(query.Get("recommendationKey"))
	diagnosticStage := "request-validation"
	diagnosticRecorded := false
	diagnosticInternalMode := ""
	diagnosticTier := truncateClientDiagnosticText(query.Get("tier"))
	diagnosticRequestedPosition := strings.ToLower(strings.TrimSpace(query.Get("position")))
	diagnosticResolvedPosition := ""
	diagnosticPositionSource := ""
	defer func() {
		if diagnosticRecorded {
			return
		}
		a.recordDiagnostic(map[string]any{
			"event": "live_recommendations_response", "diagnostic_schema": 2, "trace_id": traceID,
			"recommendation_key": recommendationKey, "champion_id": championIDForDiagnostic,
			"queue_id": queueIDForDiagnostic, "map_id": mapIDForDiagnostic, "game_id": gameIDForDiagnostic,
			"game_mode": strings.ToUpper(strings.TrimSpace(query.Get("gameMode"))), "internal_mode": diagnosticInternalMode,
			"tier": diagnosticTier, "requested_position": diagnosticRequestedPosition,
			"resolved_position": diagnosticResolvedPosition, "position_source": diagnosticPositionSource,
			"status": "failed", "stage": diagnosticStage, "duration_ms": time.Since(started).Milliseconds(),
		})
	}()
	a.recordDiagnostic(map[string]any{
		"event": "live_recommendations_request", "diagnostic_schema": 2, "trace_id": traceID,
		"recommendation_key": recommendationKey, "champion_id": championIDForDiagnostic,
		"queue_id": queueIDForDiagnostic, "map_id": diagnosticInt64(query.Get("mapId")),
		"game_id": diagnosticInt64(query.Get("gameId")), "game_mode": strings.ToUpper(strings.TrimSpace(query.Get("gameMode"))),
		"tier": truncateClientDiagnosticText(query.Get("tier")), "raw_position": query.Get("position"),
		"requested_position": strings.ToLower(strings.TrimSpace(query.Get("position"))), "status": "received",
	})
	diagnosticStage = "champion-validation"
	championID, err := strconv.ParseInt(strings.TrimSpace(query.Get("championId")), 10, 64)
	if err != nil || championID <= 0 || championID > 10000 {
		a.recordLiveRecommendationRejection("championId", query.Get("championId"))
		http.Error(w, "推荐英雄无效", http.StatusBadRequest)
		return
	}
	diagnosticStage = "queue-validation"
	queueID, err := parseOptionalRecommendationInt(query.Get("queueId"))
	if err != nil {
		a.recordLiveRecommendationRejection("queueId", query.Get("queueId"))
		http.Error(w, "推荐队列无效", http.StatusBadRequest)
		return
	}
	diagnosticStage = "map-validation"
	mapID, err := parseOptionalRecommendationInt(query.Get("mapId"))
	if err != nil {
		a.recordLiveRecommendationRejection("mapId", query.Get("mapId"))
		http.Error(w, "推荐地图无效", http.StatusBadRequest)
		return
	}
	diagnosticStage = "game-validation"
	gameID, err := parseOptionalGameID(query.Get("gameId"))
	if err != nil {
		a.recordLiveRecommendationRejection("gameId", query.Get("gameId"))
		gameID = 0
	}
	diagnosticStage = "mode-resolution"
	resolution := resolveGameplayRecommendationMode(queueID, query.Get("gameMode"), mapID)
	diagnosticInternalMode = resolution.InternalMode
	a.recordUnknownQueue(queueID, query.Get("gameMode"), mapID)
	a.recordRecommendationModeResolution(gameID, queueID, query.Get("gameMode"), mapID, resolution)
	requestedSource := strings.ToLower(strings.TrimSpace(query.Get("source")))
	if requestedSource != "" && (requestedSource != "mayhem" || resolution.InternalMode != "hextech-aram") {
		a.recordLiveRecommendationRejection("source", query.Get("source"))
		http.Error(w, "推荐来源无效", http.StatusBadRequest)
		return
	}
	spec := opggModeSpecs[resolution.InternalMode]
	diagnosticStage = "tier-validation"
	tier, err := gameplayRecommendationTier(spec, query.Get("tier"))
	if err != nil {
		a.recordLiveRecommendationRejection("tier", query.Get("tier"))
		http.Error(w, "推荐段位无效", http.StatusBadRequest)
		return
	}
	diagnosticTier = tier
	diagnosticStage = "position-validation"
	position := ""
	positionSource := "fallback"
	requestedPosition := ""
	if spec.PositionMode == opggPositionRequired {
		normalized, normalizeErr := normalizeOPGGPosition(query.Get("position"))
		if normalizeErr != nil {
			a.recordLiveRecommendationRejection("position", query.Get("position"))
			http.Error(w, "推荐位置无效", http.StatusBadRequest)
			return
		}
		requestedPosition = normalized
	} else {
		position = spec.requestPosition(query.Get("position"))
	}
	spell1ID, err := parseOptionalRecommendationInt(query.Get("spell1Id"))
	if err != nil {
		a.recordLiveRecommendationRejection("spell1Id", query.Get("spell1Id"))
		http.Error(w, "推荐召唤师技能无效", http.StatusBadRequest)
		return
	}
	spell2ID, err := parseOptionalRecommendationInt(query.Get("spell2Id"))
	if err != nil {
		a.recordLiveRecommendationRejection("spell2Id", query.Get("spell2Id"))
		http.Error(w, "推荐召唤师技能无效", http.StatusBadRequest)
		return
	}
	if requestedPosition == "" && hasSmiteSpell(spell1ID, spell2ID) {
		requestedPosition = "jungle"
	}
	diagnosticRequestedPosition = requestedPosition
	diagnosticStage = "metadata-load"
	provider := a.championDataProvider()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	metadata, err := provider.championMetadataByID(ctx, int(championID))
	if err != nil {
		http.Error(w, "暂时无法识别当前英雄", http.StatusNotFound)
		return
	}
	diagnosticStage = "detail-load"
	var detail championDetailResponse
	if spec.PositionMode == opggPositionRequired {
		probePosition := requestedPosition
		if probePosition == "" {
			probePosition = "mid"
		}
		detail, err = provider.loadDetail(ctx, resolution.InternalMode, metadata.Slug, probePosition, tier)
		if err == nil {
			diagnosticStage = "position-resolution"
			position, positionSource, err = resolveGameplayRecommendationPosition(requestedPosition, detail.Positions)
			if err != nil {
				http.Error(w, "推荐位置无法推断", http.StatusBadGateway)
				return
			}
			if position != probePosition {
				detail, err = provider.loadDetail(ctx, resolution.InternalMode, metadata.Slug, position, tier)
			}
		}
	} else {
		positionSource = "requested"
		detail, err = provider.loadDetail(ctx, resolution.InternalMode, metadata.Slug, position, tier)
	}
	if err != nil {
		http.Error(w, "推荐数据暂不可用", http.StatusBadGateway)
		return
	}
	diagnosticStage = "response-build"
	diagnosticResolvedPosition = strings.ToLower(strings.TrimSpace(position))
	diagnosticPositionSource = positionSource
	if spec.PositionMode == opggPositionRequired {
		a.recordPositionResolution(traceID, championID, requestedPosition, position, positionSource, detail.Positions)
	}
	a.recordChampionDataResolution(traceID, gameID, championID, queueID, resolution.InternalMode, position, positionSource, detail)
	if !recommendationModeHasTopPlayers(resolution) {
		detail.TopPlayers = nil
	}
	bundle := gameplayRecommendationsFromResolvedDetail(championID, strings.ToLower(position), detail, resolution)
	bundle.TraceID = traceID
	bundle.RecommendationKey = recommendationKey
	bundle.RequestedPosition = requestedPosition
	bundle.ResolvedPosition = strings.ToLower(position)
	bundle.PositionSource = positionSource
	bundle.QueueID = queueID
	bundle.MapID = mapID
	bundle.GameID = gameID
	bundle.GameMode = strings.ToUpper(strings.TrimSpace(query.Get("gameMode")))
	bundle.Tier = tier
	coreIn := len(detail.Build.CoreItems)
	bundle.Build.CoreOptions = capGameplayCoreOptions(bundle.Build.CoreOptions)
	a.recordDiagnostic(map[string]any{"event": "live_build_shape", "diagnostic_schema": 2, "trace_id": traceID, "recommendation_key": recommendationKey, "core_in": coreIn, "core_out": len(bundle.Build.CoreOptions)})
	a.recordDiagnostic(map[string]any{
		"event": "live_recommendations_response", "diagnostic_schema": 2, "trace_id": traceID, "recommendation_key": recommendationKey,
		"champion_id": championID, "queue_id": queueID, "map_id": mapID, "game_id": gameID, "game_mode": bundle.GameMode,
		"internal_mode": resolution.InternalMode, "tier": tier, "requested_position": requestedPosition,
		"resolved_position": bundle.ResolvedPosition, "position_source": positionSource, "build_position": strings.ToLower(strings.TrimSpace(bundle.Build.Position)),
		"build_options": recommendationBuildOrderDiagnostic(bundle.Build), "status": "success", "stage": "response", "duration_ms": time.Since(started).Milliseconds(),
	})
	diagnosticRecorded = true
	respondJSON(w, gameplayRecommendationsResponse{ChampionID: championID, Position: strings.ToLower(position), Recommendations: bundle})
}

func recommendationBuildOrderDiagnostic(build gameplayRecommendationBuild) map[string]any {
	options := func(rows []gameplayRecommendationOption) [][]int64 {
		out := make([][]int64, 0, len(rows))
		for _, row := range rows {
			out = append(out, append([]int64(nil), row.IDs...))
		}
		return out
	}
	return map[string]any{
		"starter": options(build.StarterOptions), "boots": options(build.BootOptions), "core": options(build.CoreOptions),
		"fourth": options(build.FourthOptions), "fifth": options(build.FifthOptions), "sixth": options(build.SixthOptions), "prism": options(build.PrismOptions),
	}
}

func (a *app) recordUnknownQueue(queueID int64, gameMode string, mapID int64) {
	if a == nil || queueID <= 0 {
		return
	}
	if _, known := supportedQueueDefinition(queueID); known {
		return
	}
	a.unknownQueueDiagnosticMu.Lock()
	if a.unknownQueueDiagnosticIDs == nil {
		a.unknownQueueDiagnosticIDs = make(map[int64]struct{})
	}
	if _, recorded := a.unknownQueueDiagnosticIDs[queueID]; recorded {
		a.unknownQueueDiagnosticMu.Unlock()
		return
	}
	a.unknownQueueDiagnosticIDs[queueID] = struct{}{}
	a.unknownQueueDiagnosticMu.Unlock()
	a.recordDiagnostic(map[string]any{
		"event": "unknown_queue_observed", "queue_id": queueID,
		"game_mode": strings.ToUpper(strings.TrimSpace(gameMode)), "map_id": mapID,
	})
}

func (a *app) recordLiveRecommendationRejection(stage, raw string) {
	rawLength, firstCharacterClass := recommendationInputShape(raw)
	a.recordDiagnostic(map[string]any{
		"event": "live_recommendations_rejected", "stage": stage, "status": http.StatusBadRequest,
		"raw_length": rawLength, "raw_first_character_class": firstCharacterClass,
	})
}

func recommendationInputShape(value string) (int, string) {
	if value == "" {
		return 0, "empty"
	}
	class := "other"
	switch first := value[0]; {
	case first >= '0' && first <= '9':
		class = "digit"
	case first == '-':
		class = "minus"
	case first == '+':
		class = "plus"
	case first == ' ' || first == '\t' || first == '\r' || first == '\n':
		class = "whitespace"
	case first >= 'A' && first <= 'Z' || first >= 'a' && first <= 'z':
		class = "letter"
	}
	return len(value), class
}

func (a *app) recordPositionResolution(traceID string, championID int64, requested, resolved, source string, positions []championPositionOption) {
	candidates := make([]string, 0, len(positions))
	for _, option := range positions {
		candidate := strings.ToLower(strings.TrimSpace(option.Position))
		if _, ok := championPositionNames[candidate]; ok {
			candidates = append(candidates, candidate)
		}
	}
	a.recordDiagnostic(map[string]any{
		"event": "position_resolved", "diagnostic_schema": 2, "trace_id": traceID, "champion_id": championID, "requested": requested,
		"resolved": resolved, "source": source, "candidates": candidates,
	})
}

func (a *app) recordChampionDataResolution(traceID string, gameID, championID, queueID int64, mode, position, positionSource string, detail championDetailResponse) {
	if a == nil {
		return
	}
	key := fmt.Sprintf("%d:%d:%s:%s", queueID, championID, strings.ToLower(strings.TrimSpace(mode)), strings.ToLower(strings.TrimSpace(position)))
	if gameID > 0 {
		key = "game:" + strconv.FormatInt(gameID, 10)
	}
	a.championDataDiagnosticMu.Lock()
	if a.championDataDiagnosticKeys == nil {
		a.championDataDiagnosticKeys = make(map[string]struct{})
	}
	if _, recorded := a.championDataDiagnosticKeys[key]; recorded {
		a.championDataDiagnosticMu.Unlock()
		return
	}
	if len(a.championDataDiagnosticKeys) >= 128 {
		a.championDataDiagnosticKeys = make(map[string]struct{})
	}
	a.championDataDiagnosticKeys[key] = struct{}{}
	a.championDataDiagnosticMu.Unlock()
	a.recordDiagnostic(map[string]any{
		"event": "champion_data_resolved", "diagnostic_schema": 2, "trace_id": traceID, "champion_id": championID,
		"position": strings.ToLower(strings.TrimSpace(position)), "position_source": positionSource,
		"mode": strings.ToLower(strings.TrimSpace(mode)), "queue_id": queueID,
		"win_rate_present": detail.Stats.WinRate > 0,
		"counters_n":       len(detail.Counters.WeakAgainst) + len(detail.Counters.StrongAgainst),
		"runes_n":          len(detail.Runes),
	})
}

func (a *app) recordLiveRosterShape(response gameplayLiveResponse) {
	if a == nil {
		return
	}
	teamCounts := make(map[string]int)
	arenaGroupCounts := make(map[string]int)
	identities := make(map[string]struct{})
	duplicates := 0
	emptyRefs := 0
	withStats := 0
	for _, player := range response.Players {
		teamKey := strconv.FormatInt(player.TeamID, 10)
		teamCounts[teamKey]++
		if group := strings.TrimSpace(player.ArenaGroup); group != "" {
			arenaGroupCounts[group]++
		}
		identity := liveRosterDiagnosticIdentity(player)
		if identity != "" {
			if _, exists := identities[identity]; exists {
				duplicates++
			} else {
				identities[identity] = struct{}{}
			}
		} else {
			emptyRefs++
		}
		if player.Rank != nil || player.ModeStats.Games > 0 || len(player.RecentGames) > 0 {
			withStats++
		}
	}
	parts := []string{
		strconv.FormatInt(response.GameID, 10),
		strings.ToUpper(strings.TrimSpace(response.Phase)),
		strconv.FormatBool(response.DroppedStaleGameData),
		"with-stats:" + strconv.Itoa(withStats),
		"empty-refs:" + strconv.Itoa(emptyRefs),
		"arena-source:" + response.ArenaGroupSource,
	}
	for _, player := range response.Players {
		identity := liveRosterDiagnosticIdentity(player)
		parts = append(parts, fmt.Sprintf("%d:%s:%d:%s", player.TeamID, identity, player.ChampionID, player.ArenaGroup))
	}
	fingerprint := sha256.Sum256([]byte(strings.Join(parts, "|")))
	key := "roster:" + hex.EncodeToString(fingerprint[:])
	a.liveRosterDiagnosticMu.Lock()
	if a.liveRosterDiagnosticKeys == nil {
		a.liveRosterDiagnosticKeys = make(map[string]struct{})
	}
	if len(a.liveRosterDiagnosticKeys) >= 512 {
		a.liveRosterDiagnosticKeys = make(map[string]struct{})
	}
	if _, recorded := a.liveRosterDiagnosticKeys[key]; recorded {
		a.liveRosterDiagnosticMu.Unlock()
		return
	}
	a.liveRosterDiagnosticKeys[key] = struct{}{}
	a.liveRosterDiagnosticMu.Unlock()
	a.recordDiagnostic(map[string]any{
		"event": "live_roster_shape", "game_id": response.GameID, "phase": response.Phase,
		"players": len(response.Players), "raw_count": response.RawCount, "with_stats": withStats, "team_counts": teamCounts,
		"arena_grouped": response.ArenaGrouped, "arena_group_source": response.ArenaGroupSource, "arena_group_counts": arenaGroupCounts,
		"duplicate_player_refs": duplicates, "empty_ref_count": emptyRefs, "merge_appended": response.MergeAppended,
		"dropped_stale_gamedata": response.DroppedStaleGameData,
		"fingerprint":            hex.EncodeToString(fingerprint[:8]),
	})
}

func (a *app) clearLivePositionSnapshot() {
	if a == nil {
		return
	}
	a.livePositionMu.Lock()
	a.livePositionGameID = 0
	a.livePositionByRef = nil
	a.livePositionMu.Unlock()
}

func (a *app) rememberLivePositionSnapshot(gameID int64, players []gameplayLivePlayer) {
	if a == nil || gameID <= 0 {
		return
	}
	next := make(map[string]string)
	for _, player := range players {
		key := strings.TrimSpace(player.PlayerRef)
		position := normalizePosition(player.Position, "")
		// Snapshot keys are renderer-safe aliases, never upstream identity values.
		if strings.HasPrefix(key, "player_") && position != "" {
			next[key] = position
		}
	}
	a.livePositionMu.Lock()
	if a.livePositionGameID != 0 && a.livePositionGameID != gameID {
		a.livePositionByRef = nil
	}
	a.livePositionGameID = gameID
	a.livePositionByRef = next
	a.livePositionMu.Unlock()
}

type livePositionSnapshotStats struct {
	GameID       int64
	Size         int
	AppliedCount int
}

func (a *app) livePositionSnapshotStats() livePositionSnapshotStats {
	if a == nil {
		return livePositionSnapshotStats{}
	}
	a.livePositionMu.Lock()
	defer a.livePositionMu.Unlock()
	return livePositionSnapshotStats{GameID: a.livePositionGameID, Size: len(a.livePositionByRef)}
}

func (a *app) applyLivePositionSnapshot(gameID int64, players []gameplayLivePlayer) livePositionSnapshotStats {
	if a == nil || gameID <= 0 {
		return livePositionSnapshotStats{}
	}
	a.livePositionMu.Lock()
	stats := livePositionSnapshotStats{GameID: a.livePositionGameID, Size: len(a.livePositionByRef)}
	if a.livePositionGameID != gameID {
		a.livePositionGameID = 0
		a.livePositionByRef = nil
		a.livePositionMu.Unlock()
		return stats
	}
	positions := make(map[string]string, len(a.livePositionByRef))
	for key, position := range a.livePositionByRef {
		positions[key] = position
	}
	a.livePositionMu.Unlock()
	for index := range players {
		if position := positions[players[index].PlayerRef]; position != "" {
			players[index].Position = position
			stats.AppliedCount++
		}
	}
	return stats
}

func livePositionShapeDiagnostic(response gameplayLiveResponse, session lcuGameflowSession, champSelect lcuChampSelectSession, current Summoner, snapshot livePositionSnapshotStats) map[string]any {
	selectedPositions := map[string]int{}
	selectedRoles := map[string]int{}
	assignedPositions := map[string]int{}
	normalizedPositions := map[string]int{}
	selfSelected := ""
	selfAssigned := ""
	countSelected := func(player lcuLivePlayer) {
		position := strings.ToUpper(strings.TrimSpace(player.SelectedPosition))
		role := strings.ToUpper(strings.TrimSpace(player.SelectedRole))
		selectedPositions[position]++
		selectedRoles[role]++
		if selfSelected == "" && strings.TrimSpace(current.PUUID) != "" && player.PUUID == current.PUUID {
			selfSelected = position
		}
	}
	for _, player := range session.GameData.TeamOne {
		countSelected(player)
	}
	for _, player := range session.GameData.TeamTwo {
		countSelected(player)
	}
	for _, player := range champSelect.MyTeam {
		position := strings.ToUpper(strings.TrimSpace(player.AssignedPosition))
		assignedPositions[position]++
		isSelf := champSelect.LocalPlayerCellID != nil && player.CellID != nil && *champSelect.LocalPlayerCellID == *player.CellID
		if !isSelf && strings.TrimSpace(current.PUUID) != "" {
			isSelf = player.PUUID == current.PUUID
		}
		if isSelf {
			selfAssigned = position
		}
	}
	smiteCount := 0
	for _, player := range response.Players {
		normalizedPositions[normalizePosition(player.Position, "")]++
		if player.Spell1ID == 11 || player.Spell2ID == 11 {
			smiteCount++
		}
	}
	positionSwapKeys := map[string]struct{}{}
	for _, swap := range champSelect.PositionSwaps {
		for key := range swap {
			positionSwapKeys[key] = struct{}{}
		}
	}
	keys := make([]string, 0, len(positionSwapKeys))
	for key := range positionSwapKeys {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return map[string]any{
		"event": "live_position_shape", "phase": response.Phase, "queue_id": response.QueueID,
		"game_mode": response.GameMode, "game_id": response.GameID,
		"selected_position_counts": selectedPositions, "selected_role_counts": selectedRoles,
		"assigned_position_counts": assignedPositions, "normalized_position_counts": normalizedPositions,
		"smite_count": smiteCount, "self_selected_position": selfSelected, "self_assigned_position": selfAssigned,
		"position_swaps_length": len(champSelect.PositionSwaps), "position_swaps_element_keys": keys,
		"snapshot_game_id": snapshot.GameID, "snapshot_size": snapshot.Size, "snapshot_applied_count": snapshot.AppliedCount,
	}
}

const lcuSessionShapeDiagnosticLimit = 128

func claimBoundedDiagnosticKey(mu *sync.Mutex, keys *map[string]struct{}, key string, limit int) bool {
	mu.Lock()
	defer mu.Unlock()
	if *keys == nil {
		*keys = make(map[string]struct{})
	}
	if _, recorded := (*keys)[key]; recorded {
		return false
	}
	if len(*keys) >= limit {
		*keys = make(map[string]struct{})
	}
	(*keys)[key] = struct{}{}
	return true
}

func lcuSessionShapeDiagnosticKey(phase, gameMode string, queueID int64) string {
	return strings.ToUpper(strings.TrimSpace(phase)) + ":" + strings.ToUpper(strings.TrimSpace(gameMode)) + ":" + strconv.FormatInt(queueID, 10)
}

type liveClientArenaGrouping struct {
	Field             string
	ByIdentity        map[string]string
	NameByGroup       map[string]string
	IdentifiedPlayers int
}

type liveClientSnapshot struct {
	Grouping           liveClientArenaGrouping
	OrderedIdentities  [][]string
	PositionByIdentity map[string]string
}

type liveClientProbeState struct {
	GameID        int64
	Snapshot      liveClientSnapshot
	MascotMapping bool
	Diagnostic    *liveClientProbeDiagnostic
	Succeeded     bool
	InFlight      bool
	Attempt       int
	FirstAttempt  time.Time
	LastAttempt   time.Time
}

type liveClientPlayerListShape struct {
	PlayerCount          int
	ElementKeys          []string
	TeamValues           map[string]int
	SubteamValues        map[string]map[string]int
	PositionValues       map[string]int
	PositionMatchedCount int
	PositionMatchSources map[string]int
	GroupField           string
	Grouped              bool
}

type liveClientProbeDiagnostic struct {
	Status  int
	Shape   liveClientPlayerListShape
	Result  string
	Attempt int
	Phase   string
}

func (a *app) loadLiveClientPlayerList(ctx context.Context) ([]byte, int, error) {
	if a != nil && a.liveClientPlayerList != nil {
		return a.liveClientPlayerList(ctx)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, liveClientPlayerListURL, nil)
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := liveClientDataHTTPClient.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, response.StatusCode, fmt.Errorf("live client player list returned HTTP %d", response.StatusCode)
	}
	data, err := readLimited(response.Body, 4*1024*1024)
	if err != nil {
		return nil, response.StatusCode, err
	}
	return data, response.StatusCode, nil
}

func liveClientMapValue(entry map[string]any, name string) (any, bool) {
	for key, value := range entry {
		if strings.EqualFold(strings.TrimSpace(key), name) {
			return value, true
		}
	}
	return nil, false
}

func liveClientDistributionValue(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		value := strings.TrimSpace(typed)
		if value == "" || len(value) > 64 {
			return "", false
		}
		return value, true
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return "", false
		}
		if typed == math.Trunc(typed) {
			return strconv.FormatInt(int64(typed), 10), true
		}
		return strconv.FormatFloat(typed, 'f', -1, 64), true
	case bool:
		return strconv.FormatBool(typed), true
	default:
		return "", false
	}
}

func liveClientFieldDistribution(entries []map[string]any, field string) map[string]int {
	distribution := make(map[string]int)
	for _, entry := range entries {
		value, exists := liveClientMapValue(entry, field)
		if !exists {
			continue
		}
		if normalized, ok := liveClientDistributionValue(value); ok {
			distribution[normalized]++
		}
	}
	if len(distribution) == 0 {
		return nil
	}
	return distribution
}

func normalizeLiveClientPlayerName(value string) []string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return nil
	}
	keys := []string{value}
	if base, _, found := strings.Cut(value, "#"); found && strings.TrimSpace(base) != "" {
		keys = append(keys, strings.TrimSpace(base))
	}
	return keys
}

func liveClientEntryIdentityKeys(entry map[string]any) []string {
	set := make(map[string]struct{})
	for _, field := range []string{"summonerName", "riotId", "displayName"} {
		if value, exists := liveClientMapValue(entry, field); exists {
			if text, ok := value.(string); ok {
				for _, key := range normalizeLiveClientPlayerName(text) {
					set[key] = struct{}{}
				}
			}
		}
	}
	gameNameValue, _ := liveClientMapValue(entry, "gameName")
	tagLineValue, _ := liveClientMapValue(entry, "tagLine")
	gameName, _ := gameNameValue.(string)
	tagLine, _ := tagLineValue.(string)
	for _, key := range normalizeLiveClientPlayerName(gameName) {
		set[key] = struct{}{}
	}
	if strings.TrimSpace(gameName) != "" && strings.TrimSpace(tagLine) != "" {
		for _, key := range normalizeLiveClientPlayerName(gameName + "#" + tagLine) {
			set[key] = struct{}{}
		}
	}
	riotGameNameValue, _ := liveClientMapValue(entry, "riotIdGameName")
	riotTagLineValue, _ := liveClientMapValue(entry, "riotIdTagLine")
	riotGameName, _ := riotGameNameValue.(string)
	riotTagLine, _ := riotTagLineValue.(string)
	for _, key := range normalizeLiveClientPlayerName(riotGameName) {
		set[key] = struct{}{}
	}
	if strings.TrimSpace(riotGameName) != "" && strings.TrimSpace(riotTagLine) != "" {
		for _, key := range normalizeLiveClientPlayerName(riotGameName + "#" + riotTagLine) {
			set[key] = struct{}{}
		}
	}
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	return keys
}

func parseLiveClientPlayerList(raw []byte, sizes ...int) (liveClientSnapshot, liveClientPlayerListShape, error) {
	squadSize := 3
	if len(sizes) > 0 {
		squadSize = sizes[0]
	}
	var rawEntries []json.RawMessage
	if err := json.Unmarshal(raw, &rawEntries); err != nil {
		return liveClientSnapshot{}, liveClientPlayerListShape{}, err
	}
	entries := make([]map[string]any, 0, len(rawEntries))
	subteamFieldSet := make(map[string]struct{})
	for _, rawEntry := range rawEntries {
		var entry map[string]any
		if err := json.Unmarshal(rawEntry, &entry); err != nil {
			return liveClientSnapshot{}, liveClientPlayerListShape{}, err
		}
		entries = append(entries, entry)
		for key := range entry {
			if strings.Contains(strings.ToLower(key), "subteam") {
				subteamFieldSet[key] = struct{}{}
			}
		}
	}
	shape := liveClientPlayerListShape{
		PlayerCount:    len(entries),
		ElementKeys:    diagnosticKeyUnion(rawEntries),
		TeamValues:     liveClientFieldDistribution(entries, "team"),
		PositionValues: make(map[string]int),
	}
	snapshot := liveClientSnapshot{PositionByIdentity: make(map[string]string)}
	for _, entry := range entries {
		snapshot.OrderedIdentities = append(snapshot.OrderedIdentities, liveClientEntryIdentityKeys(entry))
		value, _ := liveClientMapValue(entry, "position")
		position, _ := value.(string)
		position = normalizePosition(position, "")
		shape.PositionValues[position]++
		if position == "" {
			continue
		}
		for _, identity := range liveClientEntryIdentityKeys(entry) {
			snapshot.PositionByIdentity[identity] = position
		}
	}
	if len(shape.PositionValues) == 0 {
		shape.PositionValues = nil
	}
	subteamFields := make([]string, 0, len(subteamFieldSet))
	for field := range subteamFieldSet {
		subteamFields = append(subteamFields, field)
	}
	sort.Strings(subteamFields)
	if len(subteamFields) > 0 {
		shape.SubteamValues = make(map[string]map[string]int, len(subteamFields))
		for _, field := range subteamFields {
			shape.SubteamValues[field] = liveClientFieldDistribution(entries, field)
		}
	}
	groupField := ""
	for _, field := range subteamFields {
		if !strings.EqualFold(field, "subteamId") && !strings.EqualFold(field, "playerSubteamId") {
			continue
		}
		if liveClientArenaDistribution(shape.SubteamValues[field], len(entries), squadSize) {
			groupField = field
			break
		}
	}
	if groupField == "" {
		return snapshot, shape, nil
	}
	grouping := liveClientArenaGrouping{Field: groupField, ByIdentity: make(map[string]string), NameByGroup: make(map[string]string)}
	conflictingNames := make(map[string]bool)
	for _, entry := range entries {
		value, _ := liveClientMapValue(entry, groupField)
		group, ok := liveClientDistributionValue(value)
		if !ok {
			return snapshot, shape, nil
		}
		identities := liveClientEntryIdentityKeys(entry)
		if len(identities) == 0 {
			return snapshot, shape, nil
		}
		grouping.IdentifiedPlayers++
		for _, identity := range identities {
			grouping.ByIdentity[identity] = group
		}
		for _, field := range []string{"subteamName", "teamName"} {
			rawName, exists := liveClientMapValue(entry, field)
			name, _ := rawName.(string)
			name = strings.TrimSpace(name)
			if !exists || name == "" || len([]rune(name)) > 64 || conflictingNames[group] {
				continue
			}
			if previous := grouping.NameByGroup[group]; previous != "" && previous != name {
				delete(grouping.NameByGroup, group)
				conflictingNames[group] = true
			} else {
				grouping.NameByGroup[group] = name
			}
			break
		}
	}
	shape.GroupField = groupField
	shape.Grouped = grouping.IdentifiedPlayers == len(entries) && len(grouping.ByIdentity) > 0
	if !shape.Grouped {
		return snapshot, shape, nil
	}
	snapshot.Grouping = grouping
	return snapshot, shape, nil
}

func (a *app) recordLiveClientPlayerListShape(gameID int64, status int, shape liveClientPlayerListShape, result string, attempt int, phase string) {
	if a == nil {
		return
	}
	result = strings.ToLower(strings.TrimSpace(result))
	key := "game:" + strconv.FormatInt(gameID, 10) + ":" + result
	if !claimBoundedDiagnosticKey(&a.liveClientPlayerListDiagnosticMu, &a.liveClientPlayerListDiagnosticKeys, key, lcuSessionShapeDiagnosticLimit) {
		return
	}
	a.recordDiagnostic(map[string]any{
		"event": "live_client_playerlist_shape", "result": result,
		"attempt": attempt, "phase": phase,
		"http_status": status, "player_count": shape.PlayerCount, "element_keys": shape.ElementKeys,
		"team_values": liveClientDiagnosticDistribution(shape.TeamValues), "subteam_field_values": liveClientDiagnosticNestedDistribution(shape.SubteamValues),
		"position_values": liveClientDiagnosticDistribution(shape.PositionValues), "position_matched_count": shape.PositionMatchedCount,
		"position_match_source_counts": liveClientPositionMatchSourceCounts(shape.PositionMatchSources),
		"group_field":                  shape.GroupField, "grouped": shape.Grouped,
	})
}

func liveClientPositionMatchSourceCounts(counts map[string]int) map[string]int {
	return map[string]int{
		"summonerName": counts["summonerName"],
		"riotId":       counts["riotId"],
	}
}

func cloneLiveClientArenaGrouping(grouping liveClientArenaGrouping) liveClientArenaGrouping {
	return liveClientArenaGrouping{Field: grouping.Field, ByIdentity: maps.Clone(grouping.ByIdentity), NameByGroup: maps.Clone(grouping.NameByGroup), IdentifiedPlayers: grouping.IdentifiedPlayers}
}

func cloneLiveClientSnapshot(snapshot liveClientSnapshot) liveClientSnapshot {
	cloned := liveClientSnapshot{Grouping: cloneLiveClientArenaGrouping(snapshot.Grouping), PositionByIdentity: maps.Clone(snapshot.PositionByIdentity)}
	for _, keys := range snapshot.OrderedIdentities {
		cloned.OrderedIdentities = append(cloned.OrderedIdentities, append([]string(nil), keys...))
	}
	return cloned
}

func (a *app) liveClientPlayerListTime() time.Time {
	if a != nil && a.liveClientPlayerListNow != nil {
		return a.liveClientPlayerListNow()
	}
	return time.Now()
}

func (a *app) clearLiveClientProbe() {
	if a == nil {
		return
	}
	a.liveClientProbeMu.Lock()
	a.liveClientProbe = liveClientProbeState{}
	a.liveClientProbeMu.Unlock()
}

func (a *app) liveClientSnapshotForGame(ctx context.Context, gameID int64, phase string, expectedArenaPlayers int, queues ...int64) (liveClientSnapshot, bool) {
	if a == nil {
		return liveClientSnapshot{}, false
	}
	now := a.liveClientPlayerListTime()
	a.liveClientProbeMu.Lock()
	if a.liveClientProbe.GameID != gameID {
		a.liveClientProbe = liveClientProbeState{GameID: gameID}
	}
	if a.liveClientProbe.Succeeded {
		snapshot := cloneLiveClientSnapshot(a.liveClientProbe.Snapshot)
		mascotMapping := a.liveClientProbe.MascotMapping
		a.liveClientProbeMu.Unlock()
		return snapshot, mascotMapping
	}
	if a.liveClientProbe.InFlight || (!a.liveClientProbe.LastAttempt.IsZero() && now.Sub(a.liveClientProbe.LastAttempt) < liveClientRetryDelay) {
		a.liveClientProbeMu.Unlock()
		return liveClientSnapshot{}, false
	}
	a.liveClientProbe.InFlight = true
	if a.liveClientProbe.FirstAttempt.IsZero() {
		a.liveClientProbe.FirstAttempt = now
	}
	a.liveClientProbe.Attempt++
	a.liveClientProbe.LastAttempt = now
	attempt := a.liveClientProbe.Attempt
	firstAttempt := a.liveClientProbe.FirstAttempt
	a.liveClientProbeMu.Unlock()

	raw, status, fetchErr := a.loadLiveClientPlayerList(ctx)
	var snapshot liveClientSnapshot
	var shape liveClientPlayerListShape
	var parseErr error
	if fetchErr == nil {
		squadSize := 3
		if len(queues) > 0 && expectedArenaPlayers > 0 {
			squadSize = arenaSquadSize(queues[0])
		}
		snapshot, shape, parseErr = parseLiveClientPlayerList(raw, squadSize)
	}
	result := "grouped"
	if fetchErr != nil {
		result = "unavailable"
	} else if parseErr != nil {
		result = "invalid"
	} else if !shape.Grouped {
		result = "ungrouped"
	}
	usable := shape.Grouped || len(snapshot.PositionByIdentity) > 0
	if expectedArenaPlayers > 0 {
		usable = shape.PlayerCount >= expectedArenaPlayers
	}
	succeeded := fetchErr == nil && parseErr == nil && usable
	mascotMapping := succeeded && arenaLiveClientMascotMapping(snapshot.Grouping)
	a.appendDiagnosticEvent(map[string]any{"event": "live_client_probe_timing", "game_id": gameID, "phase": phase, "attempt": attempt, "success": succeeded, "elapsed_ms": now.Sub(firstAttempt).Milliseconds(), "player_count": shape.PlayerCount})
	if !succeeded {
		a.recordLiveClientPlayerListShape(gameID, status, shape, result, attempt, phase)
	}

	a.liveClientProbeMu.Lock()
	if a.liveClientProbe.GameID == gameID {
		a.liveClientProbe.InFlight = false
		if succeeded {
			a.liveClientProbe.Snapshot = cloneLiveClientSnapshot(snapshot)
			a.liveClientProbe.MascotMapping = mascotMapping
			a.liveClientProbe.Diagnostic = &liveClientProbeDiagnostic{Status: status, Shape: shape, Result: result, Attempt: attempt, Phase: phase}
			a.liveClientProbe.Succeeded = true
		}
	}
	a.liveClientProbeMu.Unlock()
	if !succeeded {
		return liveClientSnapshot{}, false
	}
	return snapshot, mascotMapping
}

func (a *app) finalizeLiveClientPlayerListShape(gameID int64, matchedCount int, sourceCounts map[string]int) {
	if a == nil {
		return
	}
	a.liveClientProbeMu.Lock()
	if a.liveClientProbe.GameID != gameID || a.liveClientProbe.Diagnostic == nil {
		a.liveClientProbeMu.Unlock()
		return
	}
	diagnostic := *a.liveClientProbe.Diagnostic
	a.liveClientProbe.Diagnostic = nil
	a.liveClientProbeMu.Unlock()
	diagnostic.Shape.PositionMatchedCount = matchedCount
	diagnostic.Shape.PositionMatchSources = liveClientPositionMatchSourceCounts(sourceCounts)
	a.recordLiveClientPlayerListShape(gameID, diagnostic.Status, diagnostic.Shape, diagnostic.Result, diagnostic.Attempt, diagnostic.Phase)
}

func liveClientDiagnosticDistribution(distribution map[string]int) map[string]int {
	if len(distribution) == 0 {
		return nil
	}
	result := make(map[string]int, len(distribution))
	for value, count := range distribution {
		safe := strings.ToUpper(strings.TrimSpace(value))
		for _, char := range safe {
			if (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '_' && char != '-' && char != '.' {
				safe = ""
				break
			}
		}
		if safe == "" || len(safe) > 32 {
			safe = "text_length_" + strconv.Itoa(len([]rune(value)))
		}
		result[safe] += count
	}
	return result
}

func liveClientDiagnosticNestedDistribution(distributions map[string]map[string]int) map[string]map[string]int {
	if len(distributions) == 0 {
		return nil
	}
	result := make(map[string]map[string]int, len(distributions))
	for field, distribution := range distributions {
		result[field] = liveClientDiagnosticDistribution(distribution)
	}
	return result
}

func liveClientPositionForIdentities(snapshot liveClientSnapshot, summonerNames, riotIDs []string) (string, string) {
	for _, candidate := range []struct {
		source string
		values []string
	}{{source: "riotId", values: riotIDs}, {source: "summonerName", values: summonerNames}} {
		for _, value := range candidate.values {
			for _, key := range normalizeLiveClientPlayerName(value) {
				if position := snapshot.PositionByIdentity[key]; position != "" {
					return position, candidate.source
				}
			}
		}
	}
	return "", ""
}

func arenaAllyIdentityKeys(player lcuLivePlayer) []string {
	set := make(map[string]struct{})
	for _, value := range []string{player.PUUID, player.ObfuscatedPUUID} {
		if value = strings.ToLower(strings.TrimSpace(value)); value != "" {
			set["puuid:"+value] = struct{}{}
		}
	}
	for _, value := range []int64{player.SummonerID, player.ObfuscatedSummonerID} {
		if value > 0 {
			set["summoner:"+strconv.FormatInt(value, 10)] = struct{}{}
		}
	}
	nameValues := []string{player.SummonerName, player.GameName}
	if strings.TrimSpace(player.GameName) != "" && strings.TrimSpace(player.TagLine) != "" {
		nameValues = append(nameValues, strings.TrimSpace(player.GameName)+"#"+strings.TrimSpace(player.TagLine))
	}
	for _, value := range nameValues {
		for _, key := range normalizeLiveClientPlayerName(value) {
			set["name:"+key] = struct{}{}
		}
	}
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	return keys
}

func (a *app) rememberArenaAllies(players []struct {
	player lcuLivePlayer
	team   int64
}) {
	keys := make(map[string]struct{})
	for _, player := range players {
		for _, key := range arenaAllyIdentityKeys(player.player) {
			keys[key] = struct{}{}
		}
	}
	a.arenaAlliesMu.Lock()
	a.arenaAllyKeys = keys
	a.arenaAllyPlayers = nil
	for _, p := range players {
		a.arenaAllyPlayers = append(a.arenaAllyPlayers, p.player)
	}
	a.arenaAlliesMu.Unlock()
}

func (a *app) clearArenaAllies() {
	if a == nil {
		return
	}
	a.arenaAlliesMu.Lock()
	a.arenaAllyKeys = nil
	a.arenaAllyPlayers = nil
	a.arenaAllyGameID = 0
	a.arenaAlliesMu.Unlock()
}

func (a *app) isRememberedArenaAlly(player lcuLivePlayer) bool {
	if a == nil {
		return false
	}
	a.arenaAlliesMu.RLock()
	defer a.arenaAlliesMu.RUnlock()
	for _, key := range arenaAllyIdentityKeys(player) {
		if _, exists := a.arenaAllyKeys[key]; exists {
			return true
		}
	}
	return false
}

func arenaLiveClientMascotMapping(grouping liveClientArenaGrouping) bool {
	if !strings.EqualFold(grouping.Field, "subteamId") && !strings.EqualFold(grouping.Field, "playerSubteamId") {
		return false
	}
	groups := make(map[string]struct{})
	for _, group := range grouping.ByIdentity {
		groups[group] = struct{}{}
	}
	if len(groups) < 2 || len(groups) > 8 {
		return false
	}
	for group := range groups {
		id, err := strconv.Atoi(group)
		if err != nil || id < 1 || id > 8 {
			return false
		}
	}
	return true
}

func (a *app) recordLCUGameflowSessionShape(raw []byte, phase, gameMode string, queueID int64) {
	if a != nil && phase == "GameStart" {
		var session lcuGameflowSession
		if json.Unmarshal(raw, &session) == nil {
			key := "GameStart-first:" + strconv.FormatInt(session.GameData.GameID, 10)
			if claimBoundedDiagnosticKey(&a.lcuGameflowShapeDiagnosticMu, &a.lcuGameflowShapeDiagnosticKeys, key, lcuSessionShapeDiagnosticLimit) {
				event := lcuGameflowSessionShapePayload(raw, phase, gameMode, queueID)
				event["game_id"], event["forced_sample"] = session.GameData.GameID, true
				a.appendDiagnosticEvent(event)
			}
		}
		return
	}
	if a == nil || !claimBoundedDiagnosticKey(&a.lcuGameflowShapeDiagnosticMu, &a.lcuGameflowShapeDiagnosticKeys, lcuSessionShapeDiagnosticKey(phase, gameMode, queueID), lcuSessionShapeDiagnosticLimit) {
		return
	}
	a.recordDiagnostic(lcuGameflowSessionShapePayload(raw, phase, gameMode, queueID))
}

func (a *app) recordLCUChampSelectSessionShape(raw []byte, phase, gameMode string, queueID int64) {
	key := lcuSessionShapeDiagnosticKey(phase, gameMode, queueID)
	if strings.ToUpper(strings.TrimSpace(gameMode)) == "CHERRY" {
		key += ":champions-" + lcuChampSelectProgressBucket(raw)
	}
	if a == nil || !claimBoundedDiagnosticKey(&a.lcuChampSelectShapeDiagnosticMu, &a.lcuChampSelectShapeDiagnosticKeys, key, lcuSessionShapeDiagnosticLimit) {
		return
	}
	a.recordDiagnostic(lcuChampSelectSessionShapePayload(raw, phase, gameMode, queueID))
}

func lcuChampSelectProgressBucket(raw []byte) string {
	var object map[string]json.RawMessage
	_ = json.Unmarshal(raw, &object)
	nonzero := 0
	for value, count := range diagnosticInt64FieldCounts(rawArrayField(object["myTeam"]), "championId") {
		if value != "0" {
			nonzero += count
		}
	}
	switch {
	case nonzero == 0:
		return "0"
	case nonzero <= 2:
		return "1-2"
	case nonzero == 3:
		return "3"
	default:
		return "4+"
	}
}

func lcuGameflowSessionShapePayload(raw []byte, phase, gameMode string, queueID int64) map[string]any {
	var object map[string]json.RawMessage
	_ = json.Unmarshal(raw, &object)
	var gameData map[string]json.RawMessage
	_ = json.Unmarshal(object["gameData"], &gameData)
	teamOne := rawArrayField(gameData["teamOne"])
	teamTwo := rawArrayField(gameData["teamTwo"])
	return map[string]any{
		"event":           "lcu_gameflow_session_shape",
		"phase":           strings.TrimSpace(phase),
		"game_mode":       strings.ToUpper(strings.TrimSpace(gameMode)),
		"queue_id":        queueID,
		"top_level_keys":  diagnosticKeySet(json.RawMessage(raw)),
		"team_one_length": len(teamOne), "team_two_length": len(teamTwo),
		"team_one_player_keys":                diagnosticKeyUnion(teamOne),
		"team_two_player_keys":                diagnosticKeyUnion(teamTwo),
		"team_one_nonzero_counts":             diagnosticNonZeroKeyCounts(teamOne),
		"team_two_nonzero_counts":             diagnosticNonZeroKeyCounts(teamTwo),
		"team_one_team_participant_id_counts": diagnosticInt64FieldCounts(teamOne, "teamParticipantId"),
		"team_two_team_participant_id_counts": diagnosticInt64FieldCounts(teamTwo, "teamParticipantId"),
	}
}

func lcuChampSelectSessionShapePayload(raw []byte, phase, gameMode string, queueID int64) map[string]any {
	var object map[string]json.RawMessage
	_ = json.Unmarshal(raw, &object)
	myTeam := rawArrayField(object["myTeam"])
	theirTeam := rawArrayField(object["theirTeam"])
	benchChampions := rawArrayField(object["benchChampions"])
	actionGroups := rawArrayField(object["actions"])
	actions := make([]json.RawMessage, 0)
	for _, group := range actionGroups {
		actions = append(actions, rawArrayField(group)...)
	}
	return map[string]any{
		"event":                               "lcu_champ_select_session_shape",
		"phase":                               strings.TrimSpace(phase),
		"game_mode":                           strings.ToUpper(strings.TrimSpace(gameMode)),
		"queue_id":                            queueID,
		"top_level_keys":                      diagnosticKeySet(json.RawMessage(raw)),
		"my_team_keys":                        diagnosticKeyUnion(myTeam),
		"their_team_keys":                     diagnosticKeyUnion(theirTeam),
		"my_team_length":                      len(myTeam),
		"my_team_unhidden_cell_ids":           diagnosticCellIDs(myTeam, true),
		"my_team_cell_id_sequence":            diagnosticCellIDs(myTeam, false),
		"their_team_length":                   len(theirTeam),
		"my_team_nonzero_counts":              diagnosticNonZeroKeyCounts(myTeam),
		"their_team_nonzero_counts":           diagnosticNonZeroKeyCounts(theirTeam),
		"my_team_team_counts":                 diagnosticInt64FieldCounts(myTeam, "team"),
		"my_team_name_visibility_type_counts": diagnosticEnumFieldCounts(myTeam, "nameVisibilityType"),
		"my_team_obfuscated_puuid_shapes":     diagnosticFieldShapeCounts(myTeam, "obfuscatedPuuid"),
		"bench_champions_length":              len(benchChampions),
		"bench_champions_element_shapes":      diagnosticElementShapeCounts(benchChampions),
		"champion_progress_bucket":            lcuChampSelectProgressBucket(raw),
		"actions_group_count":                 len(actionGroups),
		"actions_flat_count":                  len(actions),
		"actions_element_keys":                diagnosticKeyUnion(actions),
		"actions_nonzero_counts":              diagnosticNonZeroKeyCounts(actions),
		"actions_type_counts":                 diagnosticEnumFieldCounts(actions, "type"),
		"actions_actor_cell_id_counts":        diagnosticInt64FieldCounts(actions, "actorCellId"),
		"actions_champion_id_counts":          diagnosticInt64FieldCounts(actions, "championId"),
		"actions_completed_counts":            diagnosticFieldShapeCounts(actions, "completed"),
	}
}

func diagnosticInt64FieldCounts(entries []json.RawMessage, field string) map[string]int {
	counts := make(map[string]int)
	for _, entry := range entries {
		var object map[string]json.RawMessage
		if json.Unmarshal(entry, &object) != nil {
			continue
		}
		raw, exists := object[field]
		if !exists || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			continue
		}
		var value int64
		if json.Unmarshal(raw, &value) != nil {
			continue
		}
		counts[strconv.FormatInt(value, 10)]++
	}
	return counts
}

func diagnosticEnumFieldCounts(entries []json.RawMessage, field string) map[string]int {
	counts := make(map[string]int)
	for _, entry := range entries {
		var object map[string]json.RawMessage
		if json.Unmarshal(entry, &object) != nil {
			continue
		}
		raw, exists := object[field]
		if !exists {
			counts["missing"]++
			continue
		}
		var value string
		if json.Unmarshal(raw, &value) != nil {
			counts["non_string"]++
			continue
		}
		value = strings.ToUpper(strings.TrimSpace(value))
		if value == "" {
			counts["empty"]++
			continue
		}
		safe := len(value) <= 32
		for _, char := range value {
			if (char < 'A' || char > 'Z') && char != '_' && char != '-' {
				safe = false
				break
			}
		}
		if !safe {
			value = "other"
		}
		counts[value]++
	}
	return counts
}

func diagnosticFieldShapeCounts(entries []json.RawMessage, field string) map[string]int {
	counts := make(map[string]int)
	for _, entry := range entries {
		var object map[string]json.RawMessage
		if json.Unmarshal(entry, &object) != nil {
			continue
		}
		raw, exists := object[field]
		if !exists {
			counts["missing"]++
			continue
		}
		counts[diagnosticJSONShape(raw, false)]++
	}
	return counts
}

func diagnosticElementShapeCounts(entries []json.RawMessage) map[string]int {
	counts := make(map[string]int)
	for _, entry := range entries {
		counts[diagnosticJSONShape(entry, true)]++
	}
	return counts
}

func diagnosticJSONShape(raw json.RawMessage, includeObjectKeys bool) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return "invalid"
	}
	switch raw[0] {
	case 'n':
		return "null"
	case '"':
		var value string
		if json.Unmarshal(raw, &value) != nil {
			return "invalid_string"
		}
		if value == "" {
			return "string_empty"
		}
		return "string_nonempty_length_" + strconv.Itoa(len(value))
	case '{':
		if includeObjectKeys {
			return "object{" + strings.Join(diagnosticKeySet(raw), ",") + "}"
		}
		return "object"
	case '[':
		return "array"
	case 't', 'f':
		return "boolean"
	default:
		var number json.Number
		if json.Unmarshal(raw, &number) == nil {
			return "number"
		}
		return "invalid"
	}
}

func diagnosticNonZeroKeyCounts(entries []json.RawMessage) map[string]int {
	counts := make(map[string]int)
	for _, entry := range entries {
		var object map[string]json.RawMessage
		if json.Unmarshal(entry, &object) != nil {
			continue
		}
		for key, value := range object {
			if diagnosticJSONValueNonZero(value) {
				counts[key]++
			}
		}
	}
	return counts
}

func diagnosticJSONValueNonZero(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return false
	}
	switch raw[0] {
	case '"':
		var value string
		return json.Unmarshal(raw, &value) == nil && strings.TrimSpace(value) != ""
	case 't', 'f':
		var value bool
		return json.Unmarshal(raw, &value) == nil && value
	case '[':
		var value []json.RawMessage
		return json.Unmarshal(raw, &value) == nil && len(value) > 0
	case '{':
		var value map[string]json.RawMessage
		return json.Unmarshal(raw, &value) == nil && len(value) > 0
	default:
		var value float64
		return json.Unmarshal(raw, &value) == nil && value != 0
	}
}

// liveRosterDiagnosticIdentity returns an in-memory identity key for roster
// comparison. Only its hash is emitted in diagnostics; raw PUUID values never
// leave the process.
func liveRosterDiagnosticIdentity(player gameplayLivePlayer) string {
	for _, value := range []string{
		strings.TrimSpace(player.PlayerRef),
		strings.TrimSpace(player.reference.PlayerRef),
		strings.TrimSpace(player.reference.AlternatePlayerRef),
	} {
		if value != "" {
			return value
		}
	}
	return ""
}

func rawArrayField(raw json.RawMessage) []json.RawMessage {
	var values []json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &values) != nil {
		return nil
	}
	return values
}

func shouldLoadOverviewHistory(reference gameplayReference, playerRef string, matches []gameplayMatch) bool {
	return reference.ServerID != "" && validPlayerReference(playerRef) && len(matches) == 0
}

func capGameplayCoreOptions(options []gameplayRecommendationOption) []gameplayRecommendationOption {
	if len(options) > championCoreRecommendationLimit {
		return options[:championCoreRecommendationLimit]
	}
	return options
}

func parseOptionalRecommendationInt(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 || parsed > maxRecommendationSmallID {
		return 0, errors.New("invalid recommendation number")
	}
	return parsed, nil
}

func parseOptionalGameID(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 || parsed > maxRecommendationGameID {
		return 0, errors.New("invalid game id")
	}
	return parsed, nil
}

func normalizeOPGGPosition(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "other":
		return "", nil
	case "middle", "mid":
		return "mid", nil
	case "top", "jungle", "adc", "support":
		return strings.ToLower(strings.TrimSpace(value)), nil
	case "bottom":
		return "adc", nil
	case "utility":
		return "support", nil
	default:
		return "", errors.New("推荐位置无效")
	}
}

func hasSmiteSpell(spell1ID, spell2ID int64) bool {
	return spell1ID == 11 || spell2ID == 11
}

func gameplayRecommendationTier(spec opggModeSpec, value string) (string, error) {
	if !spec.UsesTier {
		return "", nil
	}
	tier := strings.ToLower(strings.TrimSpace(value))
	if tier == "" {
		tier = championCounterFallbackTier
	}
	if !allowedChampionTiers[tier] {
		return "", errors.New("invalid recommendation tier")
	}
	return tier, nil
}

func resolveGameplayRecommendationPosition(value string, positions []championPositionOption) (string, string, error) {
	position, err := normalizeOPGGPosition(value)
	if err != nil {
		return "", "", err
	}
	selected := ""
	selectedRate := -1.0
	for _, option := range positions {
		candidate := strings.ToLower(strings.TrimSpace(option.Position))
		upstream, ok := championPositionNames[candidate]
		if !ok || upstream == "" {
			continue
		}
		if candidate == position {
			return position, "requested", nil
		}
		if option.RoleRate <= selectedRate {
			continue
		}
		selected, selectedRate = candidate, option.RoleRate
	}
	if selected != "" {
		return selected, "opgg-primary", nil
	}
	if position != "" {
		return position, "fallback", nil
	}
	return "mid", "fallback", nil
}

func gameplayRecommendationsFromChampionDetail(championID int64, position string, detail championDetailResponse) gameplayRecommendationBundle {
	mode := normalizeInternalChampionMode(detail.Mode)
	if _, ok := opggModeSpecs[mode]; !ok {
		mode = "ranked"
	}
	result := gameplayRecommendationsFromResolvedDetail(championID, position, detail, gameplayRecommendationModeResolution{InternalMode: mode, QueueID: 420})
	result.PositionSource = "client"
	return result
}

func gameplayRecommendationsFromResolvedDetail(championID int64, position string, detail championDetailResponse, resolution gameplayRecommendationModeResolution) gameplayRecommendationBundle {
	spec := opggModeSpecs[resolution.InternalMode]
	hasTopPlayers := recommendationModeHasTopPlayers(resolution)
	heroStats := detail.Stats
	if detail.Mode == "arena" {
		heroStats = championDetailStats{Tier: detail.ArenaStats.Tier, WinRate: detail.ArenaStats.WinRate, PickRate: detail.ArenaStats.PickRate, BanRate: detail.ArenaStats.BanRate}
	}
	emptyReason := ""
	if heroStats.WinRate <= 0 && heroStats.PickRate <= 0 && heroStats.BanRate <= 0 {
		emptyReason = "该英雄在这个位置没有统计样本"
	}
	result := gameplayRecommendationBundle{
		Source: resolution.InternalMode, ResolvedMode: spec.APIMode, ResolvedRegion: spec.Region,
		DataVersion: detail.Patch, CurrentVersion: detail.CurrentPatch, IsFallback: resolution.IsFallback, IsStale: detail.IsStale,
		HasRunes: spec.HasRunes, HasAugments: spec.HasAugments, HasCounters: spec.HasCounters, HasBanRate: spec.HasBanRate, HasTopPlayers: hasTopPlayers, HasItemDepths: spec.SupportsItemDepths,
		Citation: detail.Citation, MeasurementTechnique: detail.MeasurementTechnique,
		Positions: append([]championPositionOption(nil), detail.Positions...), ResolvedPosition: position,
		Hero:     gameplayRecommendationHero{Tier: heroStats.Tier, WinRate: heroStats.WinRate, PickRate: heroStats.PickRate, BanRate: heroStats.BanRate, EmptyReason: emptyReason},
		Runes:    gameplayRecommendationRunes{OPGG: []gameplayRecommendationRune{}, Specialists: []gameplayRecommendationRune{}, Pros: []gameplayRecommendationRune{}},
		Build:    gameplayRecommendationBuild{Position: position, SpellOptions: []gameplayRecommendationOption{}, StarterOptions: []gameplayRecommendationOption{}, BootOptions: []gameplayRecommendationOption{}, CoreOptions: []gameplayRecommendationOption{}},
		Augments: []championMetricRow{},
	}
	if spec.HasAugments {
		result.Augments = append([]championMetricRow(nil), detail.RecommendedAugments...)
	}
	result.ItemRanking = append([]championMetricRow(nil), detail.ItemRanking...)
	if detail.Mode == "arena" {
		result.Augments = append([]championMetricRow(nil), detail.ArenaAugments...)
	}
	if result.Augments == nil {
		result.Augments = []championMetricRow{}
	}
	if spec.HasCounters {
		for _, row := range detail.Counters.StrongAgainst {
			result.Hero.StrongAgainst = append(result.Hero.StrongAgainst, gameplayRecommendationMatchup{ChampionID: int64(row.ChampionID), ChampionName: row.Name, WinRate: row.WinRate, Games: row.Games})
		}
		for _, row := range detail.Counters.WeakAgainst {
			result.Hero.WeakAgainst = append(result.Hero.WeakAgainst, gameplayRecommendationMatchup{ChampionID: int64(row.ChampionID), ChampionName: row.Name, WinRate: row.WinRate, Games: row.Games})
		}
	}
	for index, page := range detail.Runes {
		selected := make([]int64, 0, len(page.Selected))
		for _, asset := range page.Selected {
			if asset.ID > 0 {
				selected = append(selected, int64(asset.ID))
			}
		}
		statMods := make([]int64, 0, len(page.ShardSlots))
		for _, slot := range page.ShardSlots {
			for _, asset := range slot {
				if asset.Active && asset.ID > 0 {
					statMods = append(statMods, int64(asset.ID))
					break
				}
			}
		}
		if page.PrimaryStyle.ID <= 0 || page.SubStyle.ID <= 0 || len(selected) < 6 {
			continue
		}
		key := "opgg"
		if index > 0 {
			key += "-" + strconv.Itoa(index)
		}
		title := strings.TrimSpace(page.PrimaryStyle.Name + " · " + page.SubStyle.Name)
		result.Runes.OPGG = append(result.Runes.OPGG, gameplayRecommendationRune{
			Key: key, Title: title, ChampionID: championID, PrimaryStyleID: int64(page.PrimaryStyle.ID), SubStyleID: int64(page.SubStyle.ID),
			SelectedPerkIDs: selected, StatModIDs: statMods, Stats: recommendationStats(page.PickRate, page.WinRate, page.Games),
		})
	}
	result.Build.SpellOptions = recommendationOptions(detail.Build.SummonerSpells)
	result.Build.StarterOptions = recommendationOptions(detail.Build.StarterItems)
	result.Build.BootOptions = recommendationOptions(detail.Build.Boots)
	result.Build.CoreOptions = recommendationOptions(detail.Build.CoreItems)
	result.Build.FourthOptions = recommendationOptions(detail.Build.FourthItems)
	result.Build.FifthOptions = recommendationOptions(detail.Build.FifthItems)
	result.Build.SixthOptions = recommendationOptions(detail.Build.SixthItems)
	result.Build.PrismOptions = recommendationOptions(detail.Build.PrismItems)
	result.Build.ItemSource = detail.Build.ItemSource
	result.Build.ItemWindow = detail.Build.ItemWindow
	result.Build.ItemChainStatus = detail.Build.ItemChainStatus
	result.Build.FourthSample = detail.Build.FourthSample
	result.Build.FifthSample = detail.Build.FifthSample
	result.Build.SixthSample = detail.Build.SixthSample
	if len(detail.Build.Skills) > 0 {
		skills := detail.Build.Skills[0]
		result.Build.SkillPriority = append([]string(nil), skills.SkillPriority...)
		result.Build.SkillOrder = append([]string(nil), skills.SkillOrder...)
		result.Build.SkillStats = recommendationStats(skills.PickRate, skills.WinRate, skills.Games)
	}
	return result
}

func recommendationOptions(rows []championMetricRow) []gameplayRecommendationOption {
	result := make([]gameplayRecommendationOption, 0, len(rows))
	for _, row := range rows {
		ids := make([]int64, 0, len(row.Assets))
		for _, asset := range row.Assets {
			if asset.ID > 0 {
				ids = append(ids, int64(asset.ID))
			}
		}
		if len(ids) > 0 {
			stats := recommendationStats(row.PickRate, row.WinRate, row.Games)
			if row.AveragePlacement > 0 || row.FirstPlaceRate > 0 {
				averagePlacement, firstPlaceRate := row.AveragePlacement, row.FirstPlaceRate
				stats.AveragePlacement = &averagePlacement
				stats.FirstPlaceRate = &firstPlaceRate
			}
			grade := strings.ToUpper(strings.TrimSpace(firstNonEmpty(row.Grade, row.Tier)))
			result = append(result, gameplayRecommendationOption{IDs: ids, Grade: grade, GamesUnavailable: row.GamesUnavailable, Stats: stats})
		}
	}
	return result
}

func recommendationStats(pickRate, winRate float64, games int) gameplayRecommendationStats {
	if pickRate == 0 && winRate == 0 && games == 0 {
		return gameplayRecommendationStats{}
	}
	return gameplayRecommendationStats{PickRate: &pickRate, WinRate: &winRate, Games: &games}
}

type gameplayLivePlayer struct {
	gameplayPlayer
	TeamID               int64  `json:"teamId"`
	ArenaGroup           string `json:"arenaGroup,omitempty"`
	MySquad              bool   `json:"mySquad,omitempty"`
	PremadeGroup         string `json:"premadeGroup,omitempty"`
	PremadeSize          int    `json:"premadeSize,omitempty"`
	PremadeSource        string `json:"premadeSource,omitempty"`
	PremadeSessionSignal bool   `json:"premadeSessionSignal,omitempty"`
	IsAlly               bool   `json:"isAlly,omitempty"`
	// ChampionID is the locked champion. During champion select the client can
	// expose a separate pick intent before the player locks it in.
	ChampionID          int64                  `json:"championId,omitempty"`
	ChampionPickIntent  int64                  `json:"championPickIntent,omitempty"`
	ChampionPickPending bool                   `json:"championPickPending,omitempty"`
	ChampionLocked      bool                   `json:"championLocked"`
	ChampionName        string                 `json:"championName,omitempty"`
	Position            string                 `json:"position,omitempty"`
	Spell1ID            int64                  `json:"spell1Id,omitempty"`
	Spell2ID            int64                  `json:"spell2Id,omitempty"`
	Rank                *gameplayRank          `json:"rank,omitempty"`
	ModeStats           gameplayAggregate      `json:"modeStats"`
	HistoryState        string                 `json:"historyState,omitempty"`
	RecentGames         []gameplayRecentGame   `json:"recentGames,omitempty"`
	RecentRankedRecord  *gameplayRecentRecord  `json:"recentRankedRecord,omitempty"`
	RecentPositions     []gameplayPositionStat `json:"recentPositions,omitempty"`
}

// gameplayRecentGame 是对局页“详情”页签使用的单场极简摘要，
// 数据来自计算 modeStats 时已经读取的最近战绩，不产生额外请求；
// 只保留当前队列的对局（与 modeStats 同口径），因此界面无需再标注游戏模式。
type gameplayRecentGame struct {
	ChampionID   int64  `json:"championId"`
	ChampionName string `json:"championName,omitempty"`
	Win          bool   `json:"win"`
	Kills        int    `json:"kills"`
	Deaths       int    `json:"deaths"`
	Assists      int    `json:"assists"`
	CS           int    `json:"cs,omitempty"`
	QueueLabel   string `json:"queueLabel,omitempty"`
	CreatedAt    int64  `json:"createdAt,omitempty"`
}

type gameplayRecentRecord struct {
	Games  int `json:"games"`
	Wins   int `json:"wins"`
	Losses int `json:"losses"`
}

func recentRankedRecord(games []gameplayRecentGame) *gameplayRecentRecord {
	if len(games) == 0 {
		return nil
	}
	record := &gameplayRecentRecord{Games: len(games)}
	for _, game := range games {
		if game.Win {
			record.Wins++
		} else {
			record.Losses++
		}
	}
	return record
}

func recentGamesFromMatches(matches []gameplayMatch, playerRef string, limit int, queueID int64) []gameplayRecentGame {
	result := make([]gameplayRecentGame, 0, limit)
	for _, match := range matches {
		if match.Result != "win" && match.Result != "loss" {
			continue
		}
		if queueID > 0 && match.QueueID != queueID {
			continue
		}
		var subject *gameplayParticipant
		for index := range match.Participants {
			participant := &match.Participants[index]
			if match.SubjectParticipantID > 0 && participant.ParticipantID == match.SubjectParticipantID {
				subject = participant
				break
			}
			if playerRef != "" && participant.PlayerRef == playerRef {
				subject = participant
			}
		}
		if subject == nil {
			continue
		}
		result = append(result, gameplayRecentGame{
			ChampionID: subject.ChampionID, ChampionName: subject.ChampionName,
			Win: match.Result == "win", Kills: subject.Kills, Deaths: subject.Deaths, Assists: subject.Assists,
			CS: subject.CS, QueueLabel: match.QueueLabel, CreatedAt: match.CreatedAt,
		})
		if len(result) == limit {
			break
		}
	}
	return result
}

type gameplayRecommendation struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Source          string  `json:"source"`
	ChampionID      int64   `json:"championId"`
	Position        string  `json:"position"`
	PrimaryStyleID  int64   `json:"primaryStyleId"`
	SubStyleID      int64   `json:"subStyleId"`
	SelectedPerkIDs []int64 `json:"selectedPerkIds"`
}

type gameplayChampionAbility struct {
	Slot        string    `json:"slot"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	IconPath    string    `json:"iconPath,omitempty"`
	Costs       []float64 `json:"costs,omitempty"`
	Cooldowns   []float64 `json:"cooldowns,omitempty"`
	Ranges      []float64 `json:"ranges,omitempty"`
}

type lcuGameflowSession struct {
	Phase    string `json:"phase"`
	GameData struct {
		GameID int64 `json:"gameId"`
		Queue  struct {
			ID        int64  `json:"id"`
			Name      string `json:"name"`
			ShortName string `json:"shortName"`
			GameMode  string `json:"gameMode"`
			MapID     int64  `json:"mapId"`
			Type      string `json:"type"`
		} `json:"queue"`
		TeamOne []lcuLivePlayer `json:"teamOne"`
		TeamTwo []lcuLivePlayer `json:"teamTwo"`
	} `json:"gameData"`
	Map struct {
		ID       int64  `json:"id"`
		GameMode string `json:"gameMode"`
		Name     string `json:"name"`
	} `json:"map"`
}

type lcuLivePlayer struct {
	CellID               *int64 `json:"cellId"`
	ChampionID           int64  `json:"championId"`
	ChampionPickIntent   int64  `json:"championPickIntent"`
	ChampionPickPending  bool   `json:"-"`
	ChampionLocked       bool   `json:"-"`
	ProfileIconID        int64  `json:"profileIconId"`
	PUUID                string `json:"puuid"`
	ObfuscatedPUUID      string `json:"obfuscatedPuuid"`
	SelectedPosition     string `json:"selectedPosition"`
	SelectedRole         string `json:"selectedRole"`
	SummonerID           int64  `json:"summonerId"`
	ObfuscatedSummonerID int64  `json:"obfuscatedSummonerId"`
	SummonerName         string `json:"summonerName"`
	GameName             string `json:"gameName"`
	TagLine              string `json:"tagLine"`
	NameVisibilityType   string `json:"nameVisibilityType"`
	Spell1ID             int64  `json:"spell1Id"`
	Spell2ID             int64  `json:"spell2Id"`
	TeamParticipantID    int64  `json:"teamParticipantId"`
}

type lcuChampSelectSession struct {
	GameID            int64                         `json:"gameId"`
	QueueID           int64                         `json:"queueId"`
	LocalPlayerCellID *int64                        `json:"localPlayerCellId"`
	Actions           [][]lcuChampSelectAction      `json:"actions"`
	MyTeam            []lcuChampSelectPlayer        `json:"myTeam"`
	TheirTeam         []lcuChampSelectPlayer        `json:"theirTeam"`
	PositionSwaps     []map[string]any              `json:"positionSwaps"`
	BenchEnabled      bool                          `json:"benchEnabled"`
	BenchChampions    []lcuChampSelectBenchChampion `json:"benchChampions"`
	Timer             lcuChampSelectTimer           `json:"timer"`
}

type lcuChampSelectBenchChampion struct {
	ChampionID int64 `json:"championId"`
	IsPriority bool  `json:"isPriority"`
}

type lcuChampSelectTimer struct {
	AdjustedTimeLeftInPhase int64  `json:"adjustedTimeLeftInPhase"`
	Phase                   string `json:"phase"`
	TotalTimeInPhase        int64  `json:"totalTimeInPhase"`
}

type lcuChampSelectAction struct {
	ActorCellID  int64  `json:"actorCellId"`
	ChampionID   int64  `json:"championId"`
	Completed    bool   `json:"completed"`
	Duration     int64  `json:"duration"`
	ID           int64  `json:"id"`
	IsAllyAction bool   `json:"isAllyAction"`
	IsInProgress bool   `json:"isInProgress"`
	PickTurn     int64  `json:"pickTurn"`
	Type         string `json:"type"`
}

type lcuLobby struct {
	GameConfig struct {
		QueueID  int64  `json:"queueId"`
		MapID    int64  `json:"mapId"`
		GameMode string `json:"gameMode"`
	} `json:"gameConfig"`
}

type lcuChampSelectPlayer struct {
	// Champ-select's upstream `team` is only the blue/red binary used by
	// LeagueAkari (`100`/`1` => TEAM-100, everything else => TEAM-200). It is
	// not an Arena squad ID. In gameflow, real `teamParticipantId` samples had
	// 12 distinct values (5/1/1/1/1/1/2/1/1/1/2/1), not six groups of three.
	// Arena squads therefore cannot be recovered during champion select; the
	// established client projects cannot recover them either. Do not retry.
	AssignedPosition     string `json:"assignedPosition"`
	CellID               *int64 `json:"cellId"`
	ChampionID           int64  `json:"championId"`
	ChampionPickIntent   int64  `json:"championPickIntent"`
	GameName             string `json:"gameName"`
	TagLine              string `json:"tagLine"`
	PUUID                string `json:"puuid"`
	ObfuscatedPUUID      string `json:"obfuscatedPuuid"`
	SummonerID           int64  `json:"summonerId"`
	ObfuscatedSummonerID int64  `json:"obfuscatedSummonerId"`
	NameVisibilityType   string `json:"nameVisibilityType"`
	Spell1ID             int64  `json:"spell1Id"`
	Spell2ID             int64  `json:"spell2Id"`
}

const (
	arenaChampSelectNotice = "我的小队：英雄选择阶段客户端只提供本小队信息，其余小队进入对局后仍不提供小队归属"
	emptyLCUPlayerPUUID    = "00000000-0000-0000-0000-000000000000"
)

func isArenaChampSelectMode(gameMode string) bool {
	return strings.EqualFold(strings.TrimSpace(gameMode), "CHERRY")
}

func filterArenaChampSelectPlayers(players []struct {
	player lcuLivePlayer
	team   int64
}) ([]struct {
	player lcuLivePlayer
	team   int64
}, int) {
	visible := make([]struct {
		player lcuLivePlayer
		team   int64
	}, 0, len(players))
	filtered := 0
	for _, player := range players {
		puuid := strings.TrimSpace(player.player.PUUID)
		if puuid == "" || strings.EqualFold(puuid, emptyLCUPlayerPUUID) {
			filtered++
			continue
		}
		visible = append(visible, player)
	}
	return visible, filtered
}

func (a *app) handleGameplayPhase(w http.ResponseWriter, r *http.Request) {
	client, _, err := a.gameplayClient()
	if err != nil {
		respondJSON(w, gameplayPhaseIdentity{Phase: "None", Connected: false})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	respondJSON(w, readGameplayPhaseIdentity(ctx, client))
}

func (a *app) handleGameplayLive(w http.ResponseWriter, r *http.Request) {
	client, current, err := a.gameplayClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	var phase string
	if err := client.RequestJSON(r.Context(), http.MethodGet, "/lol-gameflow/v1/gameflow-phase", nil, &phase); err != nil {
		respondJSON(w, gameplayLiveResponse{Phase: "Unavailable", Capabilities: []EndpointCapability{gameplayCapabilityError("gameflow", "/lol-gameflow/v1/gameflow-phase", err)}})
		return
	}
	if r.URL.Query().Get("refresh") == "1" {
		a.liveSnapshots.invalidate()
		a.clearLiveClientProbe()
	}
	respondJSON(w, a.cachedGameplayLive(r.Context(), client, current, phase))
}

func (a *app) loadGameplayLive(ctx context.Context, client *LCUClient, current Summoner, phase string) gameplayLiveResponse {
	liveLoadStarted := time.Now()
	response := gameplayLiveResponse{Phase: phase, Capabilities: []EndpointCapability{{Name: "gameflow", Path: "/lol-gameflow/v1/gameflow-phase", State: capabilityAvailable, Count: 1}}}
	if phase != "ChampSelect" && phase != "InProgress" && phase != "GameStart" && phase != "Reconnect" {
		a.clearArenaAllies()
		a.clearLiveClientProbe()
		a.clearLivePositionSnapshot()
		return response
	}
	if phase == "ChampSelect" {
		a.clearLiveClientProbe()
	}
	var session lcuGameflowSession
	var sessionRaw []byte
	sessionRaw, sessionErr := client.GetBytesContext(ctx, "/lol-gameflow/v1/session")
	if sessionErr == nil {
		sessionErr = json.Unmarshal(sessionRaw, &session)
	}
	if sessionErr == nil && phase == "GameStart" {
		mode := session.GameData.Queue.GameMode
		if mode == "" {
			mode = session.Map.GameMode
		}
		a.recordLCUGameflowSessionShape(sessionRaw, phase, mode, session.GameData.Queue.ID)
	}
	if sessionErr != nil {
		// 国服部分版本在英雄选择阶段不返回 gameflow session，
		// 继续尝试 champ-select 会话，不要直接放弃整页数据。
		response.Capabilities = append(response.Capabilities, gameplayCapabilityError("gameflow-session", "/lol-gameflow/v1/session", sessionErr))
		if phase != "ChampSelect" {
			return response
		}
	}
	rawPlayers := make([]struct {
		player lcuLivePlayer
		team   int64
	}, 0, 10)
	if phase == "ChampSelect" {
		// gameData remains populated with the previous match until the new game
		// starts. Rebuild ChampSelect solely from its own session and lobby.
		response.DroppedStaleGameData = sessionErr == nil
	} else {
		response.GameID = session.GameData.GameID
		response.QueueID = session.GameData.Queue.ID
		response.MapID = session.GameData.Queue.MapID
		if response.MapID == 0 {
			response.MapID = session.Map.ID
		}
		response.GameMode = session.GameData.Queue.GameMode
		if response.GameMode == "" {
			response.GameMode = session.Map.GameMode
		}
		response.QueueLabel = strings.TrimSpace(session.GameData.Queue.ShortName)
		if response.QueueLabel == "" {
			response.QueueLabel = strings.TrimSpace(session.GameData.Queue.Name)
		}
		if response.QueueLabel == "" {
			response.QueueLabel = queueLabel(response.QueueID, response.GameMode, nil)
		}
		for _, player := range session.GameData.TeamOne {
			rawPlayers = append(rawPlayers, struct {
				player lcuLivePlayer
				team   int64
			}{player, 100})
		}
		for _, player := range session.GameData.TeamTwo {
			rawPlayers = append(rawPlayers, struct {
				player lcuLivePlayer
				team   int64
			}{player, 200})
		}
	}
	response.RawCount = len(rawPlayers)
	mergeAppended := 0
	var localPlayerCellID *int64
	var champSelect lcuChampSelectSession
	if phase == "ChampSelect" {
		champSelectRaw, champSelectErr := client.GetBytesContext(ctx, "/lol-champ-select/v1/session")
		if champSelectErr == nil {
			champSelectErr = json.Unmarshal(champSelectRaw, &champSelect)
		}
		if champSelectErr == nil {
			localPlayerCellID = champSelect.LocalPlayerCellID
			beforeMerge := len(rawPlayers)
			rawPlayers = mergeChampSelectPlayers(rawPlayers, champSelect, response.RawCount)
			mergeAppended = len(rawPlayers) - beforeMerge
			response.MergeAppended = mergeAppended
			if response.GameID == 0 {
				response.GameID = champSelect.GameID
			}
			if response.QueueID == 0 {
				response.QueueID = champSelect.QueueID
			}
			if response.QueueID == 0 || response.MapID == 0 || response.GameMode == "" {
				var lobby lcuLobby
				if err := client.GetJSON("/lol-lobby/v2/lobby", &lobby); err == nil {
					if response.QueueID == 0 {
						response.QueueID = lobby.GameConfig.QueueID
					}
					if response.MapID == 0 {
						response.MapID = lobby.GameConfig.MapID
					}
					if response.GameMode == "" {
						response.GameMode = lobby.GameConfig.GameMode
					}
					response.Capabilities = append(response.Capabilities, EndpointCapability{Name: "lobby", Path: "/lol-lobby/v2/lobby", State: capabilityAvailable, Count: 1})
				} else {
					response.Capabilities = append(response.Capabilities, gameplayCapabilityError("lobby", "/lol-lobby/v2/lobby", err))
				}
			}
			if response.QueueLabel == "" {
				response.QueueLabel = queueLabel(response.QueueID, response.GameMode, nil)
			}
			if isArenaChampSelectMode(response.GameMode) {
				a.rememberArenaChampOrder(client, champSelect)
				rawPlayers, _ = filterArenaChampSelectPlayers(rawPlayers)
				response.ChampSelectNotice = arenaChampSelectNotice
				a.rememberArenaAllies(rawPlayers)
				a.arenaAlliesMu.Lock()
				a.arenaAllyGameID = response.GameID
				a.arenaAlliesMu.Unlock()
			} else {
				a.clearArenaAllies()
			}
			a.recordLCUChampSelectSessionShape(champSelectRaw, phase, response.GameMode, response.QueueID)
			response.Capabilities = append(response.Capabilities, EndpointCapability{Name: "champ-select", Path: "/lol-champ-select/v1/session", State: capabilityAvailable, Count: len(champSelect.MyTeam) + len(champSelect.TheirTeam)})
		} else {
			response.Capabilities = append(response.Capabilities, gameplayCapabilityError("champ-select", "/lol-champ-select/v1/session", champSelectErr))
		}
		if currentChampionID, err := loadCurrentChampionID(client); err == nil {
			response.CurrentChampionID = currentChampionID
			count := 0
			if currentChampionID > 0 {
				count = 1
			}
			response.Capabilities = append(response.Capabilities, EndpointCapability{Name: "current-champion", Path: "/lol-champ-select/v1/current-champion", State: capabilityAvailable, Count: count})
		} else {
			response.Capabilities = append(response.Capabilities, gameplayCapabilityError("current-champion", "/lol-champ-select/v1/current-champion", err))
		}
	}
	if sessionErr == nil {
		a.recordLCUGameflowSessionShape(sessionRaw, phase, response.GameMode, response.QueueID)
	}
	if reason, unsupported := gameplayLiveUnsupportedReason(response.GameMode); unsupported {
		response.Available = true
		response.Unsupported = true
		response.UnsupportedReason = reason
		response.Players = []gameplayLivePlayer{}
		return response
	}
	if sessionErr != nil && len(rawPlayers) == 0 && response.CurrentChampionID <= 0 {
		return response
	}
	response.Available = true
	// Begin recommendations before the ten-player identity/history fan-out.
	seed := liveRecommendationSeed{ChampionID: response.CurrentChampionID, QueueID: response.QueueID, MapID: response.MapID, GameMode: response.GameMode}
	for _, raw := range rawPlayers {
		ref := gameplayReference{PlayerRef: visibleLivePlayerReference(raw.player), SummonerID: raw.player.SummonerID}
		if gameplayLivePlayerIsCurrent(ref, current.PUUID, raw.player.CellID, localPlayerCellID) {
			if seed.ChampionID <= 0 {
				seed.ChampionID = raw.player.ChampionID
			}
			seed.Position = normalizePosition(raw.player.SelectedPosition, raw.player.SelectedRole)
			break
		}
	}
	a.liveRecommendationPrewarmer.warm(seed)
	arenaMode := isArenaChampSelectMode(response.GameMode)
	aramMode := isARAMFamilyGameMode(response.GameMode)
	switch queueModeGroupFor(response.QueueID, response.GameMode, response.MapID) {
	case "aram", "hextech-aram", "hextech-classic":
		aramMode = true
	}
	if arenaMode {
		a.noteArenaPhaseRoster(client, &response)
	}
	a.arenaAlliesMu.RLock()
	staleAllies := a.arenaAllyGameID != 0 && a.arenaAllyGameID != response.GameID
	a.arenaAlliesMu.RUnlock()
	if staleAllies {
		a.clearArenaAllies()
	}
	liveClientPositions := make([]string, len(rawPlayers))
	var liveClientSnapshotValue liveClientSnapshot
	if phase == "InProgress" || phase == "Reconnect" {
		expectedArenaPlayers := 0
		if arenaMode {
			expectedArenaPlayers = max(6, len(rawPlayers))
		}
		liveClientSnapshotValue, _ = a.liveClientSnapshotForGame(ctx, response.GameID, phase, expectedArenaPlayers, response.QueueID)
	}

	names := a.overviewChampionNames(ctx)
	response.Players = make([]gameplayLivePlayer, len(rawPlayers))
	premadeInputs := make([]livePremadeInput, len(rawPlayers))
	ranksFinishedAt := make([]time.Time, len(rawPlayers))
	matchesFinishedAt := make([]time.Time, len(rawPlayers))
	enrichmentStarted := time.Now()
	var wait sync.WaitGroup
	concurrency := liveRosterConcurrency
	if a.liveRosterConcurrencyOverride > 0 {
		concurrency = a.liveRosterConcurrencyOverride
	}
	semaphore := make(chan struct{}, concurrency)
	for index := range rawPlayers {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			raw := rawPlayers[index]
			visiblePlayerRef := visibleLivePlayerReference(raw.player)
			reference := normalizeGameplayReference(gameplayReference{
				PlayerRef: visiblePlayerRef, AlternatePlayerRef: raw.player.ObfuscatedPUUID,
				SummonerID: raw.player.SummonerID, AlternateSummonerID: raw.player.ObfuscatedSummonerID,
				DisplayName: raw.player.SummonerName, GameName: raw.player.GameName, TagLine: raw.player.TagLine,
				ProfileIconID: raw.player.ProfileIconID, ServerID: clientTencentServerID(client),
			})
			summoner := summonerFromGameplayReference(reference)
			if loaded, capability := loadGameplaySummoner(client, reference); capability.State == capabilityAvailable {
				summoner = mergeSummonerIdentity(loaded, summoner)
				reference = mergeGameplayReferences(gameplayReferenceFromSummoner(summoner), reference)
			}
			playerRef := reference.PlayerRef
			validRef := validPlayerReference(playerRef)
			isCurrent := gameplayLivePlayerIsCurrent(reference, current.PUUID, raw.player.CellID, localPlayerCellID)
			isAlly := isCurrent || (arenaMode && (phase == "ChampSelect" || a.isRememberedArenaAlly(raw.player)))
			var ranks []gameplayRank
			var matches []gameplayMatch
			historyResult := livePlayerMatchesResult{}
			if validRef {
				// Rank and recent-history reads are independent once the player identity
				// is resolved. Running them together shortens the live-page critical path.
				var enrichment sync.WaitGroup
				enrichment.Add(2)
				go func() {
					defer enrichment.Done()
					if aramMode {
						return // ARAM details do not display ranked tiers.
					}
					ranks = append([]gameplayRank(nil), a.playerRankScore(ctx, client, playerRef, isCurrent, reference.ServerID, summoner.Privacy).ranks...)
					ranksFinishedAt[index] = time.Now()
				}()
				go func() {
					defer enrichment.Done()
					historyResult = a.livePlayerMatches(ctx, client, reference, playerRef, isCurrent, names)
					matches = historyResult.Matches
					matchesFinishedAt[index] = time.Now()
				}()
				enrichment.Wait()
			}
			var rank *gameplayRank
			for rankIndex := range ranks {
				if response.QueueID == 440 && ranks[rankIndex].QueueType == "RANKED_FLEX_SR" {
					rank = &ranks[rankIndex]
					break
				}
				if response.QueueID != 440 && ranks[rankIndex].QueueType == "RANKED_SOLO_5x5" {
					rank = &ranks[rankIndex]
					break
				}
			}
			modeStats := aggregateMatches(matches, playerRef, func(match gameplayMatch) bool { return response.QueueID == 0 || match.QueueID == response.QueueID })
			hidden := strings.EqualFold(raw.player.NameVisibilityType, "HIDDEN") || (strings.TrimSpace(summoner.GameName) == "" && strings.TrimSpace(summoner.DisplayName) == "")
			displayChampionID := raw.player.ChampionID
			if displayChampionID <= 0 {
				displayChampionID = raw.player.ChampionPickIntent
			}
			if raw.player.ChampionPickPending || raw.player.ChampionPickIntent < 0 {
				displayChampionID = -3
			}
			locked := raw.player.ChampionLocked || raw.player.ChampionID > 0
			recentGames := recentGamesFromMatches(matches, playerRef, 8, response.QueueID)
			var rankedRecord *gameplayRecentRecord
			if response.QueueID == 420 || response.QueueID == 440 {
				rankedRecord = recentRankedRecord(recentGames)
			}
			response.Players[index] = gameplayLivePlayer{gameplayPlayer: gameplayPlayer{PlayerRef: playerRef, DisplayName: gameplayDisplayName(summoner), GameName: summoner.GameName, TagLine: summoner.TagLine, ProfileIconID: summoner.ProfileIconID, SummonerLevel: summoner.SummonerLevel, Hidden: hidden, IsCurrent: isCurrent, reference: reference}, TeamID: raw.team, IsAlly: isAlly, ChampionID: raw.player.ChampionID, ChampionPickIntent: positiveChampionPickIntent(raw.player.ChampionPickIntent), ChampionPickPending: raw.player.ChampionPickPending || raw.player.ChampionPickIntent < 0, ChampionLocked: locked, ChampionName: championName(names, displayChampionID), Position: normalizePosition(raw.player.SelectedPosition, raw.player.SelectedRole), Spell1ID: raw.player.Spell1ID, Spell2ID: raw.player.Spell2ID, Rank: rank, ModeStats: modeStats, HistoryState: liveHistoryState(validRef, historyResult), RecentGames: recentGames, RecentRankedRecord: rankedRecord, RecentPositions: liveRecentPositions(matches, playerRef, response.QueueID)}
			premadeInputs[index] = livePremadeInput{TeamID: raw.team, TeamParticipantID: raw.player.TeamParticipantID, Matches: cloneGameplayMatches(matches)}
		}(index)
	}
	wait.Wait()
	phaseMilliseconds := func(finished []time.Time) int64 {
		var latest time.Time
		for _, value := range finished {
			if value.After(latest) {
				latest = value
			}
		}
		if latest.IsZero() {
			return 0
		}
		return latest.Sub(enrichmentStarted).Milliseconds()
	}
	ranksMS, matchesMS := phaseMilliseconds(ranksFinishedAt), phaseMilliseconds(matchesFinishedAt)
	positionMatchSources := liveClientPositionMatchSourceCounts(nil)
	for index, raw := range rawPlayers {
		player := response.Players[index]
		summonerNames, riotIDs := livePlayerIdentityValues(raw.player, player)
		position, source := liveClientPositionForIdentities(liveClientSnapshotValue, summonerNames, riotIDs)
		liveClientPositions[index] = position
		if source != "" {
			positionMatchSources[source]++
		}
	}
	a.finalizeLiveClientPlayerListShape(response.GameID, positionMatchSources["summonerName"]+positionMatchSources["riotId"], positionMatchSources)
	if arenaMode {
		response.ArenaMySquadNotice = arenaChampSelectNotice
		a.markRememberedArenaSquad(&response)
	}
	applyLivePremadeAssignments(response.Players, premadeInputs, phase, arenaMode)
	if arenaMode && (phase == "GameStart" || phase == "InProgress" || phase == "Reconnect") {
		sessionPlayers := make([]lcuLivePlayer, len(rawPlayers))
		for i := range rawPlayers {
			sessionPlayers[i] = rawPlayers[i].player
		}
		a.markArenaGroupingAttempt(client, &response)
		a.compareArenaChampOrder(client, &response, sessionPlayers, liveClientSnapshotValue)
		a.recordArenaMissingSession(current, &response, liveClientSnapshotValue)
		a.applyArenaLiveGrouping(client, current, &response, sessionPlayers, liveClientSnapshotValue)
		go a.sampleArenaAllGameData(ctx, response.GameID)
	}

	proIndex := a.proIdentitySnapshot()
	for index := range response.Players {
		player := &response.Players[index]
		player.ProPlayer = a.matchProIdentity(proIndex, "live", player.reference)
		player.PlayerRef = a.registerGameplayReferenceDetails(player.reference)
	}
	snapshotStats := livePositionSnapshotStats{}
	if phase == "ChampSelect" {
		a.rememberLivePositionSnapshot(response.GameID, response.Players)
		snapshotStats = a.livePositionSnapshotStats()
	} else if phase == "InProgress" || phase == "Reconnect" {
		snapshotStats = a.applyLivePositionSnapshot(response.GameID, response.Players)
		// Position source priority is deliberate: Live Client Data is the only
		// in-game source covering self, allies, and opponents. Champ-select's
		// snapshot remains the fallback, followed by gameflow selectedPosition.
		for index, position := range liveClientPositions {
			if position != "" {
				response.Players[index].Position = position
			}
		}
	}
	response.Capabilities = append(response.Capabilities, EndpointCapability{Name: "live-player-analysis", Path: "本机召唤师、排位与战绩接口", State: capabilityAvailable, Count: len(response.Players)})
	a.recordLiveRosterShape(response)
	a.recordDiagnostic(livePositionShapeDiagnostic(response, session, champSelect, current, snapshotStats))
	recommendationChampionID, recommendationPosition := gameplayLiveRecommendationTargetWithChampSelect(response.Players, response.CurrentChampionID, champSelect)
	response.ResolvedChampionID = recommendationChampionID
	if recommendationChampionID > 0 {
		var recommendation *gameplayRecommendation
		var abilities []gameplayChampionAbility
		var recommendationErr, abilitiesErr error
		var recommendationWait sync.WaitGroup
		recommendationWait.Add(2)
		go func() {
			defer recommendationWait.Done()
			recommendation, recommendationErr = loadClientRecommendation(client, recommendationChampionID, recommendationPosition, response.MapID)
		}()
		go func() {
			defer recommendationWait.Done()
			abilities, abilitiesErr = a.loadChampionAbilitiesWithFallback(ctx, client, recommendationChampionID)
		}()
		recommendationWait.Wait()
		if recommendationErr == nil {
			response.ClientRecommendation = recommendation
			response.Capabilities = append(response.Capabilities, EndpointCapability{Name: "client-rune-recommendation", Path: "/lol-perks/v1/recommended-pages/...", State: capabilityAvailable, Count: 1})
		} else {
			response.Capabilities = append(response.Capabilities, gameplayCapabilityError("client-rune-recommendation", "/lol-perks/v1/recommended-pages/...", recommendationErr))
		}
		if abilitiesErr == nil {
			response.ChampionAbilities = abilities
			response.Capabilities = append(response.Capabilities, EndpointCapability{Name: "champion-abilities", Path: "/lol-game-data/assets/v1/champions/{id}.json", State: capabilityAvailable, Count: len(abilities)})
		} else {
			response.Capabilities = append(response.Capabilities, gameplayCapabilityError("champion-abilities", "/lol-game-data/assets/v1/champions/{id}.json", abilitiesErr))
		}
	}
	a.recordDiagnostic(map[string]any{
		"event": "live_load_cost", "players": len(response.Players), "concurrency": concurrency,
		"ranks_ms": ranksMS, "matches_ms": matchesMS, "total_ms": time.Since(liveLoadStarted).Milliseconds(),
	})
	return response
}

func gameplayLiveUnsupportedReason(gameMode string) (string, bool) {
	if strings.EqualFold(strings.TrimSpace(gameMode), "TFT") {
		return "暂时不支持此模式，敬请期待", true
	}
	return "", false
}

func loadCurrentChampionID(client *LCUClient) (int64, error) {
	var championID int64
	if err := client.GetJSON("/lol-champ-select/v1/current-champion", &championID); err != nil {
		return 0, err
	}
	if championID <= 0 {
		return 0, nil
	}
	return championID, nil
}

func gameplayLiveRecommendationTarget(players []gameplayLivePlayer, currentChampionID int64) (int64, string) {
	return gameplayLiveRecommendationTargetWithChampSelect(players, currentChampionID, lcuChampSelectSession{})
}

func gameplayLiveRecommendationTargetWithChampSelect(players []gameplayLivePlayer, currentChampionID int64, champSelect lcuChampSelectSession) (int64, string) {
	position := ""
	for _, player := range players {
		if !player.IsCurrent {
			continue
		}
		position = player.Position
		championID := player.ChampionID
		if championID <= 0 {
			championID = player.ChampionPickIntent
		}
		if championID > 0 {
			return championID, position
		}
		break
	}
	if currentChampionID > 0 {
		return currentChampionID, position
	}
	if champSelect.LocalPlayerCellID != nil {
		for _, group := range champSelect.Actions {
			for _, action := range group {
				if action.ActorCellID == *champSelect.LocalPlayerCellID && strings.EqualFold(strings.TrimSpace(action.Type), "PICK") && action.ChampionID > 0 {
					return action.ChampionID, position
				}
			}
		}
	}
	return 0, position
}

func gameplayLivePlayerIsCurrent(reference gameplayReference, currentPUUID string, playerCellID, localPlayerCellID *int64) bool {
	if gameplayReferenceContains(reference, currentPUUID) {
		return true
	}
	return playerCellID != nil && localPlayerCellID != nil && *playerCellID == *localPlayerCellID
}

type livePremadeInput struct {
	TeamID            int64
	TeamParticipantID int64
	Matches           []gameplayMatch
}

type livePremadeAssignment struct {
	Group         string
	Size          int
	Source        string
	SessionSignal bool
}

func applyLivePremadeAssignments(players []gameplayLivePlayer, inputs []livePremadeInput, phase string, arenaMode bool) {
	if arenaMode || (phase != "InProgress" && phase != "Reconnect") {
		return
	}
	for index, assignment := range livePremadeAssignments(inputs, livePremadeMinSharedGames) {
		if index >= len(players) {
			break
		}
		players[index].PremadeGroup = assignment.Group
		players[index].PremadeSize = assignment.Size
		players[index].PremadeSource = assignment.Source
		players[index].PremadeSessionSignal = assignment.SessionSignal
	}
}

// livePremadeAssignments combines the client's direct teamParticipantId signal
// with history inference. The caller excludes Arena before reaching this code:
// teamParticipantId has different semantics there. History coverage can disable
// inference, but must never suppress a direct same-team session grouping.
func livePremadeAssignments(inputs []livePremadeInput, threshold int) []livePremadeAssignment {
	assignments := make([]livePremadeAssignment, len(inputs))
	if len(inputs) < 2 {
		return assignments
	}
	if threshold <= 0 {
		threshold = livePremadeMinSharedGames
	}
	gameIDs := make([]map[int64]struct{}, len(inputs))
	covered := 0
	for index, input := range inputs {
		ids := make(map[int64]struct{}, len(input.Matches))
		for _, match := range input.Matches {
			if match.GameID > 0 {
				ids[match.GameID] = struct{}{}
			}
		}
		gameIDs[index] = ids
		if len(ids) > 0 {
			covered++
		}
	}
	parent := make([]int, len(inputs))
	rank := make([]int, len(inputs))
	for index := range parent {
		parent[index] = index
	}
	var find func(int) int
	find = func(value int) int {
		if parent[value] != value {
			parent[value] = find(parent[value])
		}
		return parent[value]
	}
	union := func(left, right int) {
		leftRoot, rightRoot := find(left), find(right)
		if leftRoot == rightRoot {
			return
		}
		if rank[leftRoot] < rank[rightRoot] {
			parent[leftRoot] = rightRoot
			return
		}
		parent[rightRoot] = leftRoot
		if rank[leftRoot] == rank[rightRoot] {
			rank[leftRoot]++
		}
	}
	sessionEdges := make([][]bool, len(inputs))
	inferredEdges := make([][]bool, len(inputs))
	for index := range inputs {
		sessionEdges[index] = make([]bool, len(inputs))
		inferredEdges[index] = make([]bool, len(inputs))
	}
	// Direct session groups are deterministic and intentionally precede the
	// history coverage gate.
	for left := 0; left < len(inputs); left++ {
		for right := left + 1; right < len(inputs); right++ {
			if inputs[left].TeamID != inputs[right].TeamID {
				continue
			}
			signal := inputs[left].TeamParticipantID
			if signal > 0 && signal == inputs[right].TeamParticipantID {
				sessionEdges[left][right] = true
				union(left, right)
			}
		}
	}
	// Fail closed for inference when a majority of the roster did not yield
	// history. Direct session groups above remain valid.
	inferenceEnabled := covered > len(inputs)/2
	for left := 0; left < len(inputs); left++ {
		for right := left + 1; right < len(inputs); right++ {
			if !inferenceEnabled {
				continue
			}
			if inputs[left].TeamID != 0 && inputs[right].TeamID != 0 && inputs[left].TeamID != inputs[right].TeamID {
				continue
			}
			shared := 0
			for gameID := range gameIDs[left] {
				if _, ok := gameIDs[right][gameID]; ok {
					shared++
					if shared >= threshold {
						inferredEdges[left][right] = true
						union(left, right)
						break
					}
				}
			}
		}
	}

	components := make(map[int][]int)
	for index := range inputs {
		root := find(index)
		components[root] = append(components[root], index)
	}
	groupNumber := 0
	for first := range inputs {
		root := find(first)
		members := components[root]
		if len(members) < 2 || members[0] != first {
			continue
		}
		groupNumber++
		hasSession, hasInference := false, false
		for leftIndex, left := range members {
			for _, right := range members[leftIndex+1:] {
				hasSession = hasSession || sessionEdges[left][right]
				hasInference = hasInference || inferredEdges[left][right]
			}
		}
		source := "inferred"
		if hasSession {
			source = "session"
		}
		if hasSession && hasInference {
			source = "both"
		}
		sessionCounts := make(map[int64]int)
		for _, member := range members {
			if signal := inputs[member].TeamParticipantID; signal > 0 {
				sessionCounts[signal]++
			}
		}
		for _, member := range members {
			signal := inputs[member].TeamParticipantID
			assignments[member] = livePremadeAssignment{
				Group: strconv.Itoa(groupNumber), Size: len(members),
				Source: source, SessionSignal: signal > 0 && sessionCounts[signal] > 1,
			}
		}
	}
	return assignments
}

type livePlayerMatchesCacheEntry struct {
	Matches   []gameplayMatch
	State     string
	FetchedAt time.Time
}

type livePlayerMatchesFlight struct {
	Done    chan struct{}
	Matches []gameplayMatch
	State   string
}

type livePlayerMatchesResult struct {
	Matches []gameplayMatch
	State   string
}

func liveHistoryState(validRef bool, result livePlayerMatchesResult) string {
	if !validRef {
		return "unavailable"
	}
	switch result.State {
	case "ok", "empty", "failed":
		return result.State
	default:
		return "failed"
	}
}

func cloneGameplayMatches(matches []gameplayMatch) []gameplayMatch {
	return append([]gameplayMatch(nil), matches...)
}

func (a *app) cachedLivePlayerMatches(ctx context.Context, key string, loader func(context.Context) livePlayerMatchesResult) livePlayerMatchesResult {
	now := time.Now()
	a.livePlayerMatchesMu.Lock()
	if cached, ok := a.livePlayerMatchCache[key]; ok {
		if now.Sub(cached.FetchedAt) < livePlayerMatchesCacheTTL {
			if element := a.livePlayerMatchEntries[key]; element != nil {
				a.livePlayerMatchOrder.MoveToFront(element)
			}
			a.livePlayerMatchesMu.Unlock()
			return livePlayerMatchesResult{Matches: cloneGameplayMatches(cached.Matches), State: cached.State}
		}
		delete(a.livePlayerMatchCache, key)
		if element := a.livePlayerMatchEntries[key]; element != nil {
			a.livePlayerMatchOrder.Remove(element)
			delete(a.livePlayerMatchEntries, key)
		}
	}
	if flight := a.livePlayerMatchFlights[key]; flight != nil {
		a.livePlayerMatchesMu.Unlock()
		select {
		case <-ctx.Done():
			return livePlayerMatchesResult{State: "failed"}
		case <-flight.Done:
			return livePlayerMatchesResult{Matches: cloneGameplayMatches(flight.Matches), State: flight.State}
		}
	}
	if a.livePlayerMatchCache == nil {
		a.livePlayerMatchCache = make(map[string]livePlayerMatchesCacheEntry)
		a.livePlayerMatchOrder = list.New()
		a.livePlayerMatchEntries = make(map[string]*list.Element)
		a.livePlayerMatchFlights = make(map[string]*livePlayerMatchesFlight)
	}
	flight := &livePlayerMatchesFlight{Done: make(chan struct{})}
	a.livePlayerMatchFlights[key] = flight
	a.livePlayerMatchesMu.Unlock()

	loaded := loader(ctx)
	stored := cloneGameplayMatches(loaded.Matches)
	a.livePlayerMatchesMu.Lock()
	// A transient failure is not evidence of an empty history and must not poison
	// subsequent refreshes for the full cache TTL.
	if loaded.State != "failed" {
		a.livePlayerMatchCache[key] = livePlayerMatchesCacheEntry{Matches: stored, State: loaded.State, FetchedAt: time.Now()}
		if element := a.livePlayerMatchEntries[key]; element != nil {
			a.livePlayerMatchOrder.MoveToFront(element)
		} else {
			a.livePlayerMatchEntries[key] = a.livePlayerMatchOrder.PushFront(key)
		}
	}
	for len(a.livePlayerMatchCache) > livePlayerMatchesCacheMax {
		oldest := a.livePlayerMatchOrder.Back()
		if oldest == nil {
			break
		}
		oldestKey, _ := oldest.Value.(string)
		delete(a.livePlayerMatchCache, oldestKey)
		delete(a.livePlayerMatchEntries, oldestKey)
		a.livePlayerMatchOrder.Remove(oldest)
	}
	flight.Matches = cloneGameplayMatches(stored)
	flight.State = loaded.State
	delete(a.livePlayerMatchFlights, key)
	close(flight.Done)
	a.livePlayerMatchesMu.Unlock()
	return livePlayerMatchesResult{Matches: cloneGameplayMatches(stored), State: loaded.State}
}

// livePlayerMatches 读取对局页单名玩家的最近战绩：先用本机客户端的
// 列表接口（轻量），拿不到数据时改走 SGP 网关。最终规范化结果按
// playerRef + isCurrent 短暂缓存，避免 20 秒自动刷新重复请求。
func (a *app) livePlayerMatches(ctx context.Context, client *LCUClient, reference gameplayReference, playerRef string, isCurrent bool, names map[int64]string) livePlayerMatchesResult {
	key := playerRef + "\x00" + strconv.FormatBool(isCurrent)
	return a.cachedLivePlayerMatches(ctx, key, func(loadCtx context.Context) livePlayerMatchesResult {
		return a.loadLivePlayerMatches(loadCtx, client, reference, playerRef, isCurrent, names)
	})
}

func (a *app) loadLivePlayerMatches(ctx context.Context, client *LCUClient, reference gameplayReference, playerRef string, isCurrent bool, names map[int64]string) livePlayerMatchesResult {
	history, capabilities, _ := loadGameplayHistoryContext(ctx, client, playerRef, isCurrent, 0, 10, false)
	matches := make([]gameplayMatch, 0, len(history))
	for _, game := range history {
		matches = append(matches, normalizeGameplayMatch(game, reference, names, nil))
	}
	listFailed := len(capabilities) == 0 || capabilities[0].State != capabilityAvailable
	fallbackFailed := false
	if len(matches) == 0 && (listFailed || len(history) == 0) {
		if a.sgp != nil {
			_, _, ok := a.sgp.available(client)
			if ok {
				infos, _, _, err := a.sgp.matchHistory(ctx, client, playerRef, 0, 10, true)
				if err == nil {
					for _, info := range infos {
						a.checkArenaGroupTruth(client, reference.ServerID, info)
						matches = append(matches, convertRiotMatchInfo(info, playerRef, names, nil, "", reference.ServerID))
					}
					if len(matches) > 0 {
						return livePlayerMatchesResult{Matches: matches, State: "ok"}
					}
					return livePlayerMatchesResult{Matches: matches, State: "empty"}
				}
				fallbackFailed = true
			}
		}
	}
	if len(matches) > 0 {
		return livePlayerMatchesResult{Matches: matches, State: "ok"}
	}
	if listFailed || fallbackFailed {
		return livePlayerMatchesResult{State: "failed"}
	}
	return livePlayerMatchesResult{State: "empty"}
}

func loadChampionAbilities(client *LCUClient, championID int64) ([]gameplayChampionAbility, error) {
	if championID <= 0 {
		return nil, errors.New("尚未选定英雄")
	}
	var detail struct {
		Spells []struct {
			Name               string    `json:"name"`
			Description        string    `json:"description"`
			Tooltip            string    `json:"tooltip"`
			DynamicDescription string    `json:"dynamicDescription"`
			IconPath           string    `json:"iconPath"`
			ImagePath          string    `json:"imagePath"`
			AbilityIconPath    string    `json:"abilityIconPath"`
			CostCoefficients   []float64 `json:"costCoefficients"`
			Cooldowns          []float64 `json:"cooldownCoefficients"`
			Ranges             []float64 `json:"range"`
		} `json:"spells"`
	}
	path := fmt.Sprintf("/lol-game-data/assets/v1/champions/%d.json", championID)
	if err := client.GetJSON(path, &detail); err != nil {
		return nil, err
	}
	result := make([]gameplayChampionAbility, 0, len(detail.Spells))
	for index, spell := range detail.Spells {
		if index >= 4 {
			break
		}
		description := strings.TrimSpace(spell.Description)
		if description == "" {
			description = strings.TrimSpace(spell.Tooltip)
		}
		if description == "" {
			description = strings.TrimSpace(spell.DynamicDescription)
		}
		iconPath := sanitizeAssetPath(spell.IconPath)
		if iconPath == "" {
			iconPath = sanitizeAssetPath(spell.ImagePath)
		}
		if iconPath == "" {
			iconPath = sanitizeAssetPath(spell.AbilityIconPath)
		}
		result = append(result, gameplayChampionAbility{
			Slot:        []string{"Q", "W", "E", "R"}[index],
			Name:        strings.TrimSpace(spell.Name),
			Description: description,
			IconPath:    iconPath,
			Costs:       spell.CostCoefficients,
			Cooldowns:   spell.Cooldowns,
			Ranges:      spell.Ranges,
		})
	}
	if len(result) == 0 {
		return nil, errors.New("客户端没有返回英雄技能")
	}
	return result, nil
}

func (a *app) loadChampionAbilitiesWithFallback(ctx context.Context, client *LCUClient, championID int64) ([]gameplayChampionAbility, error) {
	abilities, clientErr := loadChampionAbilities(client, championID)
	if clientErr == nil && len(abilities) > 0 {
		return abilities, nil
	}
	provider := a.championDataProvider()
	metadata, metadataErr := provider.championMetadataByID(ctx, int(championID))
	if metadataErr != nil {
		return nil, errors.Join(clientErr, metadataErr)
	}
	descriptions, fallbackErr := provider.loadChampionAbilityDescriptions(ctx, metadata.Key)
	if fallbackErr != nil {
		return nil, errors.Join(clientErr, fallbackErr)
	}
	result := make([]gameplayChampionAbility, 0, 4)
	for _, slot := range []string{"Q", "W", "E", "R"} {
		description, ok := descriptions[slot]
		if !ok {
			continue
		}
		result = append(result, gameplayChampionAbility{
			Slot: slot, Name: description.Name, Description: description.Description,
			IconPath: "ddragon:" + description.Path, Costs: cloneNumbers(description.Costs),
			Cooldowns: cloneNumbers(description.Cooldowns), Ranges: cloneNumbers(description.Ranges),
		})
	}
	if len(result) == 0 {
		return nil, errors.Join(clientErr, errors.New("Data Dragon 没有返回英雄技能"))
	}
	return result, nil
}

func mergeChampSelectPlayers(existing []struct {
	player lcuLivePlayer
	team   int64
}, session lcuChampSelectSession, rawCount int) []struct {
	player lcuLivePlayer
	team   int64
} {
	result := append([]struct {
		player lcuLivePlayer
		team   int64
	}(nil), existing...)
	// The gameflow session only has one champion field. Treat a non-zero value
	// from it as locked, then let champ-select provide the separate pick intent.
	for index := range result {
		if result[index].player.ChampionID > 0 {
			result[index].player.ChampionLocked = true
		}
	}
	merge := func(source []lcuChampSelectPlayer, team int64) {
		for _, selected := range source {
			visiblePlayerRef := visibleChampSelectPlayerReference(selected)
			selectedReference := normalizeGameplayReference(gameplayReference{PlayerRef: visiblePlayerRef, AlternatePlayerRef: selected.ObfuscatedPUUID, SummonerID: selected.SummonerID, AlternateSummonerID: selected.ObfuscatedSummonerID, GameName: selected.GameName, DisplayName: selected.GameName, TagLine: selected.TagLine})
			index := -1
			for candidate := range result {
				candidateVisibleRef := visibleLivePlayerReference(result[candidate].player)
				candidateReference := normalizeGameplayReference(gameplayReference{PlayerRef: candidateVisibleRef, AlternatePlayerRef: result[candidate].player.ObfuscatedPUUID, SummonerID: result[candidate].player.SummonerID, AlternateSummonerID: result[candidate].player.ObfuscatedSummonerID, GameName: result[candidate].player.GameName, DisplayName: result[candidate].player.SummonerName, TagLine: result[candidate].player.TagLine})
				if gameplayReferencesMatch(selectedReference, candidateReference) {
					index = candidate
					break
				}
			}
			if index < 0 {
				teamCount := 0
				for _, candidate := range result {
					if candidate.team == team {
						teamCount++
					}
				}
				// Only apply the five-player duplicate guard when gameflow supplied
				// the baseline roster. Arena champ-select can be the sole roster
				// source, in which case every returned cell must be retained.
				if rawCount > 0 && teamCount >= 5 {
					continue
				}
				pickIntent := positiveChampionPickIntent(selected.ChampionPickIntent)
				result = append(result, struct {
					player lcuLivePlayer
					team   int64
				}{player: lcuLivePlayer{CellID: selected.CellID, PUUID: visiblePlayerRef, ObfuscatedPUUID: selected.ObfuscatedPUUID, SummonerID: selected.SummonerID, ObfuscatedSummonerID: selected.ObfuscatedSummonerID, ChampionID: selected.ChampionID, ChampionPickIntent: pickIntent, ChampionPickPending: selected.ChampionPickIntent < 0, ChampionLocked: selected.ChampionID > 0, SummonerName: selected.GameName, GameName: selected.GameName, TagLine: selected.TagLine, NameVisibilityType: selected.NameVisibilityType, SelectedPosition: selected.AssignedPosition, Spell1ID: selected.Spell1ID, Spell2ID: selected.Spell2ID}, team: team})
				continue
			}
			result[index].player.ChampionPickIntent = positiveChampionPickIntent(selected.ChampionPickIntent)
			result[index].player.ChampionPickPending = selected.ChampionPickIntent < 0
			if selected.ChampionID > 0 {
				result[index].player.ChampionID = selected.ChampionID
				result[index].player.ChampionLocked = true
			} else if selected.ChampionPickIntent > 0 {
				// Champ-select is authoritative while the player is hovering a new
				// champion; clear a stale gameflow champion ID from the prior frame.
				result[index].player.ChampionID = 0
				result[index].player.ChampionLocked = false
			}
			if selected.GameName != "" {
				result[index].player.SummonerName = selected.GameName
				result[index].player.GameName = selected.GameName
			}
			if selected.TagLine != "" {
				result[index].player.TagLine = selected.TagLine
			}
			if selected.NameVisibilityType != "" {
				result[index].player.NameVisibilityType = selected.NameVisibilityType
			}
			if selected.Spell1ID > 0 {
				result[index].player.Spell1ID = selected.Spell1ID
			}
			if selected.Spell2ID > 0 {
				result[index].player.Spell2ID = selected.Spell2ID
			}
			if selected.AssignedPosition != "" {
				result[index].player.SelectedPosition = selected.AssignedPosition
			}
			if selected.CellID != nil {
				result[index].player.CellID = selected.CellID
			}
			if result[index].player.PUUID == "" {
				result[index].player.PUUID = visiblePlayerRef
			}
			if result[index].player.ObfuscatedPUUID == "" {
				result[index].player.ObfuscatedPUUID = selected.ObfuscatedPUUID
			}
			if result[index].player.SummonerID == 0 {
				result[index].player.SummonerID = selected.SummonerID
			}
			if result[index].player.ObfuscatedSummonerID == 0 {
				result[index].player.ObfuscatedSummonerID = selected.ObfuscatedSummonerID
			}
		}
	}
	merge(session.MyTeam, 100)
	merge(session.TheirTeam, 200)
	return result
}

type lcuRecommendedPage struct {
	RecommendationID         string `json:"recommendationId"`
	RecommendationChampionID int64  `json:"recommendationChampionId"`
	Position                 string `json:"position"`
	PrimaryPerkStyleID       int64  `json:"primaryPerkStyleId"`
	SecondaryPerkStyleID     int64  `json:"secondaryPerkStyleId"`
	Keystone                 struct {
		ID int64 `json:"id"`
	} `json:"keystone"`
	Perks []struct {
		ID int64 `json:"id"`
	} `json:"perks"`
}

func loadClientRecommendation(client *LCUClient, championID int64, position string, mapID int64) (*gameplayRecommendation, error) {
	if championID <= 0 {
		return nil, errors.New("尚未选定英雄")
	}
	position = strings.ToLower(strings.TrimSpace(position))
	if position == "" || position == "other" {
		return nil, errors.New("客户端位置未知")
	}
	if mapID <= 0 {
		mapID = 11
	}
	path := fmt.Sprintf("/lol-perks/v1/recommended-pages/champion/%d/position/%s/map/%d", championID, url.PathEscape(position), mapID)
	var pages []lcuRecommendedPage
	if err := client.GetJSON(path, &pages); err != nil {
		return nil, err
	}
	if len(pages) == 0 {
		return nil, errors.New("客户端没有返回推荐符文")
	}
	page := pages[0]
	selected := make([]int64, 0, len(page.Perks)+1)
	seen := map[int64]bool{}
	if page.Keystone.ID > 0 {
		selected = append(selected, page.Keystone.ID)
		seen[page.Keystone.ID] = true
	}
	for _, perk := range page.Perks {
		if perk.ID > 0 && !seen[perk.ID] {
			selected = append(selected, perk.ID)
			seen[perk.ID] = true
		}
	}
	if len(selected) < 6 {
		return nil, errors.New("客户端推荐符文不完整")
	}
	return &gameplayRecommendation{ID: page.RecommendationID, Name: "客户端内置推荐", Source: "client", ChampionID: championID, Position: position, PrimaryStyleID: page.PrimaryPerkStyleID, SubStyleID: page.SecondaryPerkStyleID, SelectedPerkIDs: selected}, nil
}

type gameplayRuneApplyRequest struct {
	ChampionName    string  `json:"championName"`
	Source          string  `json:"source"`
	ChampionID      int64   `json:"championId"`
	PrimaryStyleID  int64   `json:"primaryStyleId"`
	SubStyleID      int64   `json:"subStyleId"`
	SelectedPerkIDs []int64 `json:"selectedPerkIds"`
}

type runeApplyTrace struct {
	Responses map[string]string
}

type runeApplyTraceContextKey struct{}

const runeApplyResponseLimit = 4096

func runeApplyTraceFromContext(ctx context.Context) *runeApplyTrace {
	if ctx == nil {
		return nil
	}
	trace, _ := ctx.Value(runeApplyTraceContextKey{}).(*runeApplyTrace)
	return trace
}

func recordRuneApplyResponse(trace *runeApplyTrace, name string, raw json.RawMessage) {
	if trace == nil || len(bytes.TrimSpace(raw)) == 0 {
		return
	}
	if trace.Responses == nil {
		trace.Responses = make(map[string]string)
	}
	data := bytes.TrimSpace(raw)
	if len(data) > runeApplyResponseLimit {
		data = data[:runeApplyResponseLimit]
	}
	trace.Responses[name] = string(data)
}

const (
	runePagePrefix       = "[DL] "
	runePageRecycleLimit = 5
	runePageLimitMessage = "客户端符文页已达上限，无法新增推荐符文页；请删除一个自定义符文页后重试"
)

var errRunePageLimit = errors.New(runePageLimitMessage)

func (a *app) handleGameplayRuneApply(w http.ResponseWriter, r *http.Request) {
	phase := ""
	outcome := "rejected"
	reason := "unknown"
	var request gameplayRuneApplyRequest
	trace := &runeApplyTrace{}
	defer func() {
		event := map[string]any{"event": "perk_apply_attempt", "phase": phase, "outcome": outcome, "reason": truncateClientDiagnosticText(reason)}
		if len(request.SelectedPerkIDs) > 0 {
			event["perk_ids"] = append([]int64(nil), request.SelectedPerkIDs...)
		}
		if len(trace.Responses) > 0 {
			event["lcu_response"] = trace.Responses
		}
		a.recordDiagnostic(event)
	}()
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		reason = "decode-error"
		http.Error(w, "符文配置无效", http.StatusBadRequest)
		return
	}
	if err := validateRuneApplyRequest(request); err != nil {
		reason = "invalid-request: " + safeDiagnosticReason(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	client, _, err := a.gameplayClient()
	if err != nil {
		reason = "client-unavailable"
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err := client.GetJSON("/lol-gameflow/v1/gameflow-phase", &phase); err != nil {
		outcome = "failed"
		reason = "phase-read-failed"
		http.Error(w, "只能在英雄选择阶段应用符文", http.StatusConflict)
		return
	}
	if phase != "ChampSelect" {
		reason = "phase-not-champ-select"
		http.Error(w, "只能在英雄选择阶段应用符文", http.StatusConflict)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, runeApplyTraceContextKey{}, trace)
	pageID, err := applyRunePage(ctx, client, request)
	if err != nil {
		outcome = "failed"
		if errors.Is(err, errRunePageLimit) {
			reason = "rune-page-limit"
			http.Error(w, runePageLimitMessage, http.StatusConflict)
			return
		}
		reason = "lcu-write-failed"
		http.Error(w, "符文应用失败："+safeGameplayError(err), http.StatusConflict)
		return
	}
	outcome = "success"
	reason = "applied"
	respondJSON(w, map[string]any{"applied": true, "pageId": pageID, "name": runePageName(request.ChampionName, request.Source), "selectedPerkIds": request.SelectedPerkIDs})
}

func validateRuneApplyRequest(request gameplayRuneApplyRequest) error {
	if strings.TrimSpace(request.ChampionName) == "" {
		return errors.New("当前英雄名称无效")
	}
	if strings.TrimSpace(request.Source) == "" {
		return errors.New("符文推荐来源无效")
	}
	if len([]rune(runePageName(request.ChampionName, request.Source))) > 60 {
		return errors.New("英雄名称与符文推荐来源过长")
	}
	if request.PrimaryStyleID <= 0 || request.SubStyleID <= 0 || request.PrimaryStyleID == request.SubStyleID {
		return errors.New("主系或副系符文无效")
	}
	if len(request.SelectedPerkIDs) < 6 || len(request.SelectedPerkIDs) > 12 {
		return errors.New("符文数量不完整")
	}
	seen := map[int64]int{}
	shardIDs := map[int64]bool{5001: true, 5005: true, 5007: true, 5008: true, 5010: true, 5011: true, 5013: true}
	for _, id := range request.SelectedPerkIDs {
		seen[id]++
		if id <= 0 || id > 100000 || (seen[id] > 1 && !shardIDs[id]) || seen[id] > 2 {
			return errors.New("符文编号无效或重复")
		}
	}
	return nil
}

func applyRunePage(ctx context.Context, client *LCUClient, request gameplayRuneApplyRequest) (int64, error) {
	trace := runeApplyTraceFromContext(ctx)
	canAdd, err := canAddRunePage(ctx, client)
	if err != nil {
		return 0, err
	}
	if !canAdd {
		var pages []lcuRunePage
		if err := client.RequestJSON(ctx, http.MethodGet, "/lol-perks/v1/pages", nil, &pages); err != nil {
			return 0, err
		}
		candidates := make([]lcuRunePage, 0, len(pages))
		for _, page := range pages {
			if page.ID <= 0 || !strings.HasPrefix(page.Name, runePagePrefix) || (page.IsDeletable != nil && !*page.IsDeletable) {
				continue
			}
			candidates = append(candidates, page)
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			left, right := candidates[i], candidates[j]
			leftMissing, rightMissing := left.LastModified <= 0, right.LastModified <= 0
			if leftMissing || rightMissing {
				if leftMissing != rightMissing {
					return leftMissing
				}
				return left.ID < right.ID
			}
			if left.LastModified != right.LastModified {
				return left.LastModified < right.LastModified
			}
			return left.ID < right.ID
		})
		for index, page := range candidates {
			if index >= runePageRecycleLimit {
				break
			}
			if err := client.RequestJSON(ctx, http.MethodDelete, fmt.Sprintf("/lol-perks/v1/pages/%d", page.ID), nil, nil); err != nil {
				return 0, err
			}
			canAdd, err = canAddRunePage(ctx, client)
			if err != nil {
				return 0, err
			}
			if canAdd {
				break
			}
		}
		if !canAdd {
			return 0, errRunePageLimit
		}
	}
	name := runePageName(request.ChampionName, request.Source)
	var addedRaw json.RawMessage
	if err := client.RequestJSON(ctx, http.MethodPost, "/lol-perks/v1/pages/", map[string]any{"name": name, "isEditable": true, "primaryStyleId": strconv.FormatInt(request.PrimaryStyleID, 10)}, &addedRaw); err != nil {
		return 0, err
	}
	recordRuneApplyResponse(trace, "create-page", addedRaw)
	var added struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(addedRaw, &added); err != nil {
		return 0, fmt.Errorf("decode rune page response: %w", err)
	}
	pageID := added.ID
	if pageID <= 0 {
		return 0, errors.New("客户端没有返回符文页编号")
	}
	body := map[string]any{"id": pageID, "isRecommendationOverride": false, "isTemporary": false, "name": name, "primaryStyleId": request.PrimaryStyleID, "selectedPerkIds": request.SelectedPerkIDs, "subStyleId": request.SubStyleID, "recommendationChampionId": request.ChampionID}
	var updatedRaw json.RawMessage
	if err := client.RequestJSON(ctx, http.MethodPut, fmt.Sprintf("/lol-perks/v1/pages/%d", pageID), body, &updatedRaw); err != nil {
		return 0, err
	}
	recordRuneApplyResponse(trace, "update-page", updatedRaw)
	var currentRaw json.RawMessage
	if err := client.RequestJSON(ctx, http.MethodPut, "/lol-perks/v1/currentpage", pageID, &currentRaw); err != nil {
		return 0, err
	}
	recordRuneApplyResponse(trace, "set-current-page", currentRaw)
	return pageID, nil
}

type lcuRunePage struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	IsDeletable  *bool  `json:"isDeletable"`
	LastModified int64  `json:"lastModified"`
}

func canAddRunePage(ctx context.Context, client *LCUClient) (bool, error) {
	var inventory struct {
		CanAddCustomPage bool `json:"canAddCustomPage"`
	}
	if err := client.RequestJSON(ctx, http.MethodGet, "/lol-perks/v1/inventory", nil, &inventory); err != nil {
		return false, err
	}
	return inventory.CanAddCustomPage, nil
}

func runePageName(championName, source string) string {
	championName = strings.Join(strings.Fields(championName), " ")
	source = strings.Join(strings.Fields(source), " ")
	return runePagePrefix + championName + " · " + source
}

var itemSetTraceIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

func normalizeItemSetTraceID(value string) string {
	value = strings.TrimSpace(value)
	if !itemSetTraceIDPattern.MatchString(value) {
		return ""
	}
	return value
}

func newItemSetTraceID(prefix string) string {
	prefix = strings.Trim(strings.ToLower(strings.TrimSpace(prefix)), "-_")
	if prefix == "" {
		prefix = "trace"
	}
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func diagnosticInt64(value string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return n
}

type gameplayItemSetApplyRequest struct {
	Storage           string                        `json:"storage,omitempty"`
	Title             string                        `json:"title"`
	ChampionID        int64                         `json:"championId"`
	MapID             int64                         `json:"mapId"`
	Position          string                        `json:"position"`
	SelfPosition      string                        `json:"selfPosition,omitempty"`
	TraceID           string                        `json:"traceId,omitempty"`
	RecommendationKey string                        `json:"recommendationKey,omitempty"`
	RequestedPosition string                        `json:"requestedPosition,omitempty"`
	ResolvedPosition  string                        `json:"resolvedPosition,omitempty"`
	PositionSource    string                        `json:"positionSource,omitempty"`
	GameID            int64                         `json:"gameId,omitempty"`
	QueueID           int64                         `json:"queueId,omitempty"`
	GameMode          string                        `json:"gameMode,omitempty"`
	Tier              string                        `json:"tier,omitempty"`
	Blocks            []gameplayItemSetBlockRequest `json:"blocks"`
}

type gameplayItemSetBlockRequest struct {
	Type  string                       `json:"type"`
	Items []gameplayItemSetItemRequest `json:"items"`
}

type gameplayItemSetItemRequest struct {
	ID            int64 `json:"id"`
	Count         int   `json:"count"`
	OriginalIndex int   `json:"-"`
	PriceTotal    int64 `json:"-"`
}

var gameplayItemSetPurchasableReplacements = map[int64]int64{
	3040: 3003,
	3042: 3004,
}

type gameplayItemSetNormalization struct {
	WriteTrace                  *itemSetWriteTrace
	RequestedBlocks             []itemSetDiagnosticBlock
	SortedBlocks                []itemSetDiagnosticBlock
	WrittenBlocks               []itemSetDiagnosticBlock
	ReadbackBlocks              []itemSetDiagnosticBlock
	ItemMapping                 []itemSetItemMapping
	RequestedOrderDigest        string
	SortedOrderDigest           string
	WrittenOrderDigest          string
	ReadbackOrderDigest         string
	ReadbackOrderMatch          bool
	Storage                     string
	LegacyCleanupFailed         bool
	NormalizedCount             int
	UnpurchasableCount          int
	UnmappedItemIDs             []int64
	RequestedItemCount          int
	WrittenItemCount            int
	WrittenBlockStats           []map[string]any
	RemovedStaleCount           int
	UpgradeReplacementCollapsed bool
}

type lcuItemSetDocument map[string]json.RawMessage

type lcuItemSet struct {
	UID                 string                 `json:"uid"`
	Title               string                 `json:"title"`
	Type                string                 `json:"type"`
	Map                 string                 `json:"map"`
	Mode                string                 `json:"mode"`
	SortRank            int                    `json:"sortrank"`
	StartedFrom         string                 `json:"startedFrom"`
	AssociatedChampions []int64                `json:"associatedChampions"`
	AssociatedMaps      []int64                `json:"associatedMaps"`
	Blocks              []lcuItemSetBlock      `json:"blocks"`
	PreferredItemSlots  []lcuPreferredItemSlot `json:"preferredItemSlots"`
}

type lcuItemSetBlock struct {
	Type                string           `json:"type"`
	HideIfSummonerSpell string           `json:"hideIfSummonerSpell"`
	ShowIfSummonerSpell string           `json:"showIfSummonerSpell"`
	Items               []lcuItemSetItem `json:"items"`
}

type lcuItemSetItem struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

type lcuPreferredItemSlot struct {
	ID                string `json:"id"`
	PreferredItemSlot int    `json:"preferredItemSlot"`
}

func (a *app) handleGameplayItemSetApply(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	request := gameplayItemSetApplyRequest{}
	normalization := gameplayItemSetNormalization{}
	traceID := newItemSetTraceID("apply")
	applyStage := "decode"
	phase := ""
	uid := ""
	title := ""
	stored := false
	requestedItemCount := 0
	defer func() {
		event := map[string]any{
			"event": "item_set_apply", "diagnostic_schema": 2, "trace_id": traceID,
			"recommendation_key": truncateClientDiagnosticText(request.RecommendationKey),
			"champion_id":        request.ChampionID, "uid": uid, "position": request.Position,
			"requested_position": strings.ToLower(strings.TrimSpace(request.RequestedPosition)),
			"resolved_position":  strings.ToLower(strings.TrimSpace(request.ResolvedPosition)),
			"position_source":    truncateClientDiagnosticText(request.PositionSource),
			"self_position":      strings.ToLower(strings.TrimSpace(request.SelfPosition)),
			"game_id":            request.GameID, "queue_id": request.QueueID, "map_id": request.MapID,
			"game_mode": strings.ToUpper(strings.TrimSpace(request.GameMode)), "tier": truncateClientDiagnosticText(request.Tier),
			"requested_blocks": normalization.RequestedBlocks, "sorted_blocks": normalization.SortedBlocks,
			"written_blocks": normalization.WrittenBlocks, "readback_blocks": normalization.ReadbackBlocks,
			"requested_order_digest": normalization.RequestedOrderDigest, "sorted_order_digest": normalization.SortedOrderDigest,
			"written_order_digest": normalization.WrittenOrderDigest, "readback_order_digest": normalization.ReadbackOrderDigest,
			"readback_order_match": normalization.ReadbackOrderMatch, "item_mapping": normalization.ItemMapping,
			"blocks": normalization.WrittenBlockStats, "block_count": len(normalization.WrittenBlocks), "item_count": normalization.WrittenItemCount,
			"requested_item_count": requestedItemCount, "normalized_count": normalization.NormalizedCount,
			"unpurchasable_count": normalization.UnpurchasableCount, "removed_stale_count": normalization.RemovedStaleCount,
			"storage": normalization.Storage, "legacy_cleanup_failed": normalization.LegacyCleanupFailed,
			"phase": phase, "duration_ms": time.Since(started).Milliseconds(), "stored": stored,
			"apply_stage": applyStage, "write_trace": normalization.WriteTrace, "requested_storage": request.Storage,
			"game_render_geometry": "unobservable", "game_loaded_recommendation": "unknown",
		}
		a.recordDiagnostic(event)
	}()

	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "装备方案无效", http.StatusBadRequest)
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		http.Error(w, "装备方案无效", http.StatusBadRequest)
		return
	}
	if normalized := normalizeItemSetTraceID(request.TraceID); normalized != "" {
		traceID = normalized
	}
	request.TraceID = traceID
	request.RecommendationKey = truncateClientDiagnosticText(request.RecommendationKey)
	request.RequestedPosition = truncateClientDiagnosticText(request.RequestedPosition)
	request.ResolvedPosition = truncateClientDiagnosticText(request.ResolvedPosition)
	request.PositionSource = truncateClientDiagnosticText(request.PositionSource)
	request.GameMode = truncateClientDiagnosticText(request.GameMode)
	request.Tier = truncateClientDiagnosticText(request.Tier)
	applyStage = "validate-storage"
	if request.Storage != "" && request.Storage != "recommended" {
		http.Error(w, "装备方案写入方式无效", http.StatusBadRequest)
		return
	}
	applyStage = "validate-position"
	position, err := normalizeGameplayItemSetPosition(request.Position)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	request.Position = position
	uid = itemSetUID(request.ChampionID, request.Position)
	applyStage = "validate-payload"
	if err := validateGameplayItemSetRequest(request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	for _, block := range request.Blocks {
		requestedItemCount += len(block.Items)
	}
	normalization.RequestedBlocks = diagnosticBlocksFromRequest(request.Blocks)
	normalization.RequestedOrderDigest = itemSetOrderDigest(normalization.RequestedBlocks)

	applyStage = "client-context"
	client, current, err := a.gameplayClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if current.SummonerID <= 0 {
		http.Error(w, "客户端没有提供当前账号编号", http.StatusConflict)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
	defer cancel()
	applyStage = "champ-select-guard"
	observedPhase, err := validateGameplayItemSetContextPhase(ctx, client, current, request.ChampionID)
	phase = observedPhase
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	a.itemSetMu.Lock()
	defer a.itemSetMu.Unlock()
	prices := a.gameplayItemPrices(ctx, client)
	sortGameplayItemSetBlocksByPrice(request.Blocks, prices)
	normalization.SortedBlocks = diagnosticBlocksFromRequest(request.Blocks)
	normalization.SortedOrderDigest = itemSetOrderDigest(normalization.SortedBlocks)
	var itemPurchasable map[int64]bool
	if a.champions != nil {
		itemPurchasable = a.champions.itemPurchasabilitySnapshot()
	}
	apply := applyGameplayItemSet
	if request.Storage == "recommended" {
		apply = applyRecommendedItemSet
	}
	applyStage = "apply"
	uid, title, appliedNormalization, err := apply(ctx, client, current, request, itemPurchasable, func() error {
		latestClient, latestCurrent, latestErr := a.gameplayClient()
		if latestErr != nil || latestClient != client || gameplaySummonerChanged(current, latestCurrent) || (current.AccountID > 0 && latestCurrent.AccountID > 0 && current.AccountID != latestCurrent.AccountID) {
			return errors.New("客户端账号已发生变化，已停止应用装备方案")
		}
		return validateGameplayItemSetContext(ctx, client, latestCurrent, request.ChampionID)
	})
	appliedNormalization.RequestedBlocks = normalization.RequestedBlocks
	appliedNormalization.SortedBlocks = normalization.SortedBlocks
	appliedNormalization.RequestedOrderDigest = normalization.RequestedOrderDigest
	appliedNormalization.SortedOrderDigest = normalization.SortedOrderDigest
	normalization = appliedNormalization
	for _, itemID := range normalization.UnmappedItemIDs {
		a.recordDiagnostic(map[string]any{"event": "item_set_unmapped_item", "diagnostic_schema": 2, "trace_id": traceID, "id": itemID, "champion_id": request.ChampionID})
	}
	if err != nil {
		applyStage = itemSetFailureStage(err, "apply-failed")
		if normalization.WriteTrace != nil {
			applyStage = normalization.WriteTrace.Stage
		}
		http.Error(w, "装备方案应用失败："+safeGameplayError(err), http.StatusConflict)
		return
	}
	stored = true
	applyStage = "completed"
	if normalization.LegacyCleanupFailed {
		applyStage = "completed-with-legacy-warning"
	}
	result := map[string]any{"applied": true, "stored": true, "uid": uid, "title": title, "storage": normalization.Storage, "traceId": traceID}
	if normalization.LegacyCleanupFailed {
		result["notice"] = "推荐文件已写入，旧 DL 方案未能清理"
	}
	if normalization.UpgradeReplacementCollapsed && !normalization.LegacyCleanupFailed {
		result["notice"] = "炽天使之拥等升级形态已替换为可购买的原始装备（大天使之杖），游戏内叠满后自动升级"
	}
	respondJSON(w, result)
}

func normalizeGameplayItemSetPosition(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "fill", "other":
		return "other", nil
	case "top", "jungle", "middle", "bottom", "utility":
		return strings.ToLower(strings.TrimSpace(value)), nil
	case "mid":
		return "middle", nil
	case "adc":
		return "bottom", nil
	case "support":
		return "utility", nil
	default:
		return "", errors.New("装备方案位置无效")
	}
}

func validateGameplayItemSetRequest(request gameplayItemSetApplyRequest) error {
	if request.ChampionID <= 0 || request.ChampionID > 10000 {
		return errors.New("装备方案英雄无效")
	}
	if request.MapID < 0 || request.MapID > 100000 {
		return errors.New("装备方案地图无效")
	}
	if len(request.Blocks) == 0 || len(request.Blocks) > 20 {
		return errors.New("装备方案分组数量无效")
	}
	for _, block := range request.Blocks {
		blockType := strings.TrimSpace(block.Type)
		if blockType == "" || len([]rune(blockType)) > 64 || len(block.Items) == 0 || len(block.Items) > 20 {
			return errors.New("装备方案分组无效")
		}
		seen := make(map[int64]bool, len(block.Items))
		for _, item := range block.Items {
			if item.ID <= 0 || item.ID > 1000000 || item.Count <= 0 || item.Count > 6 || seen[item.ID] {
				return errors.New("装备方案包含无效或重复的装备")
			}
			seen[item.ID] = true
		}
	}
	return nil
}

func validateGameplayItemSetContext(ctx context.Context, client *LCUClient, current Summoner, championID int64) error {
	_, err := validateGameplayItemSetContextPhase(ctx, client, current, championID)
	return err
}

// Report the gameflow phase actually observed alongside the guard result. The
// apply diagnostic used to record a hardcoded "ChampSelect" after this guard
// passed, which made the field tautological: it could never show the phase a
// rejected apply was really in. Callers log the returned value instead. An
// empty string means the endpoint could not be read at all.
func validateGameplayItemSetContextPhase(ctx context.Context, client *LCUClient, current Summoner, championID int64) (string, error) {
	var phase string
	if err := client.RequestJSON(ctx, http.MethodGet, "/lol-gameflow/v1/gameflow-phase", nil, &phase); err != nil {
		return "", errors.New("只能在英雄选择阶段应用装备方案")
	}
	observed := strings.TrimSpace(phase)
	if observed != "ChampSelect" {
		return observed, errors.New("只能在英雄选择阶段应用装备方案")
	}
	var session lcuChampSelectSession
	if err := client.RequestJSON(ctx, http.MethodGet, "/lol-champ-select/v1/session", nil, &session); err != nil {
		return observed, errors.New("无法确认当前英雄选择")
	}
	currentReference := gameplayReferenceFromSummoner(current)
	for _, player := range session.MyTeam {
		playerReference := normalizeGameplayReference(gameplayReference{
			PlayerRef: visibleChampSelectPlayerReference(player), AlternatePlayerRef: player.ObfuscatedPUUID,
			SummonerID: player.SummonerID, AlternateSummonerID: player.ObfuscatedSummonerID,
		})
		if !gameplayReferencesMatch(currentReference, playerReference) &&
			!gameplayLivePlayerIsCurrent(playerReference, current.PUUID, player.CellID, session.LocalPlayerCellID) {
			continue
		}
		selectedChampionID := player.ChampionID
		if selectedChampionID <= 0 {
			selectedChampionID = player.ChampionPickIntent
		}
		if selectedChampionID <= 0 || selectedChampionID != championID {
			return observed, errors.New("当前选择的英雄已经变化，已停止应用装备方案")
		}
		return observed, nil
	}
	return observed, errors.New("英雄选择中没有找到当前账号")
}

func (a *app) gameplayItemPrices(ctx context.Context, client *LCUClient) map[int64]int64 {
	if a == nil || client == nil {
		return nil
	}
	now := time.Now()
	a.itemSetPriceMu.Lock()
	if len(a.itemSetPrices) > 0 && now.Sub(a.itemSetPricesAt) < 30*time.Minute {
		prices := maps.Clone(a.itemSetPrices)
		a.itemSetPriceMu.Unlock()
		return prices
	}
	a.itemSetPriceMu.Unlock()
	var raw []struct {
		ID         int64 `json:"id"`
		PriceTotal int64 `json:"priceTotal"`
	}
	if err := client.RequestJSON(ctx, http.MethodGet, "/lol-game-data/assets/v1/items.json", nil, &raw); err != nil {
		return nil
	}
	prices := make(map[int64]int64, len(raw))
	for _, item := range raw {
		if item.ID > 0 && item.PriceTotal > 0 {
			prices[item.ID] = item.PriceTotal
		}
	}
	if len(prices) == 0 {
		return nil
	}
	a.itemSetPriceMu.Lock()
	a.itemSetPrices, a.itemSetPricesAt = maps.Clone(prices), now
	a.itemSetPriceMu.Unlock()
	return prices
}

func sortGameplayItemSetBlocksByPrice(blocks []gameplayItemSetBlockRequest, prices map[int64]int64) {
	for blockIndex := range blocks {
		items := blocks[blockIndex].Items
		knownIndexes := make([]int, 0, len(items))
		knownItems := make([]gameplayItemSetItemRequest, 0, len(items))
		for itemIndex := range items {
			items[itemIndex].OriginalIndex = itemIndex
			items[itemIndex].PriceTotal = prices[items[itemIndex].ID]
			if items[itemIndex].PriceTotal <= 0 {
				continue
			}
			knownIndexes = append(knownIndexes, itemIndex)
			knownItems = append(knownItems, items[itemIndex])
		}
		sort.SliceStable(knownItems, func(left, right int) bool {
			return knownItems[left].PriceTotal < knownItems[right].PriceTotal
		})
		for index, itemIndex := range knownIndexes {
			items[itemIndex] = knownItems[index]
		}
	}
}

func applyGameplayItemSet(ctx context.Context, client *LCUClient, current Summoner, request gameplayItemSetApplyRequest, itemPurchasable map[int64]bool, beforeWrite func() error) (string, string, gameplayItemSetNormalization, error) {
	path := fmt.Sprintf("/lol-item-sets/v1/item-sets/%d/sets", current.SummonerID)
	var document lcuItemSetDocument
	if err := client.RequestJSON(ctx, http.MethodGet, path, nil, &document); err != nil {
		return "", "", gameplayItemSetNormalization{}, err
	}
	if current.AccountID > 0 {
		if rawAccountID, ok := document["accountId"]; ok {
			var accountID int64
			if err := json.Unmarshal(rawAccountID, &accountID); err != nil || (accountID > 0 && accountID != current.AccountID) {
				return "", "", gameplayItemSetNormalization{}, errors.New("客户端装备方案属于另一个账号")
			}
		}
	}
	itemSet, normalization := newLCUItemSet(request, itemPurchasable)
	normalization.Storage = "lcu"
	normalization.WrittenBlockStats, normalization.WrittenItemCount = itemSetStats(itemSet)
	removedStaleCount, err := upsertLCUItemSetWithCount(document, itemSet)
	normalization.RemovedStaleCount = removedStaleCount
	if err != nil {
		return "", "", normalization, err
	}
	if beforeWrite != nil {
		if err := beforeWrite(); err != nil {
			return "", "", normalization, err
		}
	}
	if err := client.RequestJSON(ctx, http.MethodPut, path, document, nil); err != nil {
		return "", "", normalization, err
	}
	var verifiedDocument lcuItemSetDocument
	if err := client.RequestJSON(ctx, http.MethodGet, path, nil, &verifiedDocument); err != nil {
		return "", "", normalization, errors.New("客户端没有返回写入后的装备方案")
	}
	verified, found, err := findLCUItemSet(verifiedDocument, itemSet.UID)
	if found {
		normalization.ReadbackBlocks = diagnosticBlocksFromLCU(verified.Blocks)
		normalization.ReadbackOrderDigest = itemSetOrderDigest(normalization.ReadbackBlocks)
		normalization.ReadbackOrderMatch = normalization.WrittenOrderDigest == normalization.ReadbackOrderDigest
	}
	if err != nil || !found || !sameLCUItemSet(itemSet, verified) || !normalization.ReadbackOrderMatch {
		return "", "", normalization, errors.New("客户端未能确认装备方案写入结果")
	}
	return itemSet.UID, itemSet.Title, normalization, nil
}

func newLCUItemSet(request gameplayItemSetApplyRequest, itemPurchasable map[int64]bool) (lcuItemSet, gameplayItemSetNormalization) {
	blocks := make([]lcuItemSetBlock, 0, len(request.Blocks))
	normalization := gameplayItemSetNormalization{}
	for _, source := range request.Blocks {
		normalization.RequestedItemCount += len(source.Items)
	}
	unmapped := make(map[int64]struct{})
	for blockIndex, source := range request.Blocks {
		items := make([]lcuItemSetItem, 0, len(source.Items))
		seen := make(map[int64]struct{}, len(source.Items))
		for sortedIndex, item := range source.Items {
			itemID := item.ID
			mapping := itemSetItemMapping{
				BlockIndex: blockIndex, RequestedIndex: item.OriginalIndex, SortedIndex: sortedIndex, WrittenIndex: -1,
				RequestedID: item.ID, NormalizedID: item.ID, Count: item.Count, Price: item.PriceTotal, PriceKnown: item.PriceTotal > 0,
				Result: "written",
			}
			if replacement, ok := gameplayItemSetPurchasableReplacements[itemID]; ok {
				itemID = replacement
				mapping.NormalizedID = replacement
				mapping.Result = "replaced-written"
				mapping.Reason = "upgrade-replacement"
				normalization.NormalizedCount++
				normalization.UnpurchasableCount++
			} else if purchasable, known := itemPurchasable[itemID]; known && !purchasable {
				mapping.Result = "written-unpurchasable"
				mapping.Reason = "no-safe-replacement"
				normalization.UnpurchasableCount++
				if _, recorded := unmapped[itemID]; !recorded {
					unmapped[itemID] = struct{}{}
					normalization.UnmappedItemIDs = append(normalization.UnmappedItemIDs, itemID)
				}
			}
			if _, duplicate := seen[itemID]; duplicate {
				mapping.Result = "duplicate-dropped"
				if itemID != item.ID {
					mapping.Reason = "replacement-duplicate"
				} else {
					mapping.Reason = "normalized-duplicate"
				}
				if itemID != item.ID && strings.Contains(strings.TrimSpace(source.Type), "核心") {
					normalization.UpgradeReplacementCollapsed = true
				}
				normalization.ItemMapping = append(normalization.ItemMapping, mapping)
				continue
			}
			seen[itemID] = struct{}{}
			mapping.WrittenIndex = len(items)
			normalization.ItemMapping = append(normalization.ItemMapping, mapping)
			items = append(items, lcuItemSetItem{ID: strconv.FormatInt(itemID, 10), Count: item.Count})
		}
		blocks = append(blocks, lcuItemSetBlock{Type: strings.TrimSpace(source.Type), Items: items})
	}
	associatedMaps := make([]int64, 0, 1)
	if request.MapID > 0 {
		associatedMaps = append(associatedMaps, request.MapID)
	}
	set := lcuItemSet{
		UID: itemSetUID(request.ChampionID, request.Position), Title: itemSetName(request.Position), Type: "custom", Map: "any", Mode: "any",
		StartedFrom: "DL", SortRank: 100, AssociatedChampions: []int64{request.ChampionID}, AssociatedMaps: associatedMaps,
		Blocks: blocks, PreferredItemSlots: []lcuPreferredItemSlot{},
	}
	normalization.WrittenBlocks = diagnosticBlocksFromLCU(set.Blocks)
	normalization.WrittenOrderDigest = itemSetOrderDigest(normalization.WrittenBlocks)
	return set, normalization
}

func itemSetStats(itemSet lcuItemSet) ([]map[string]any, int) {
	stats := make([]map[string]any, 0, len(itemSet.Blocks))
	count := 0
	for _, block := range itemSet.Blocks {
		count += len(block.Items)
		stats = append(stats, map[string]any{"type": strings.TrimSpace(block.Type), "count": len(block.Items)})
	}
	return stats, count
}

func itemSetUID(championID int64, position string) string {
	return fmt.Sprintf("deep-legends-v1-%d-%s", championID, position)
}

func itemSetName(position string) string {
	label := map[string]string{
		"top": "上路", "jungle": "打野", "middle": "中路", "bottom": "下路", "utility": "辅助", "other": "通用",
	}[strings.ToLower(strings.TrimSpace(position))]
	if label == "" {
		label = "通用"
	}
	return "DL · " + label
}

func upsertLCUItemSetWithCount(document lcuItemSetDocument, itemSet lcuItemSet) (int, error) {
	rawSets, ok := document["itemSets"]
	if !ok {
		return 0, errors.New("客户端装备方案结构不受支持")
	}
	var sets []json.RawMessage
	if err := json.Unmarshal(rawSets, &sets); err != nil || sets == nil {
		return 0, errors.New("客户端装备方案结构不受支持")
	}
	encoded, err := json.Marshal(itemSet)
	if err != nil {
		return 0, err
	}
	cleaned := make([]json.RawMessage, 0, len(sets)+1)
	found := false
	removed := 0
	for _, rawSet := range sets {
		var metadata struct {
			UID         string `json:"uid"`
			StartedFrom string `json:"startedFrom"`
		}
		if err := json.Unmarshal(rawSet, &metadata); err != nil || strings.TrimSpace(string(rawSet)) == "null" {
			return 0, errors.New("客户端装备方案集合包含无法识别的数据")
		}
		ownedByDeepLegends := strings.HasPrefix(metadata.UID, "deep-legends-v1-") || strings.EqualFold(strings.TrimSpace(metadata.StartedFrom), "DL")
		if !ownedByDeepLegends {
			cleaned = append(cleaned, rawSet)
			continue
		}
		if metadata.UID == itemSet.UID && !found {
			cleaned = append(cleaned, encoded)
			found = true
			continue
		}
		removed++
	}
	if !found {
		cleaned = append(cleaned, encoded)
	}
	encodedSets, err := json.Marshal(cleaned)
	if err != nil {
		return 0, err
	}
	document["itemSets"] = encodedSets
	return removed, nil
}

func findLCUItemSet(document lcuItemSetDocument, uid string) (lcuItemSet, bool, error) {
	rawSets, ok := document["itemSets"]
	if !ok {
		return lcuItemSet{}, false, errors.New("客户端装备方案结构不受支持")
	}
	var sets []json.RawMessage
	if err := json.Unmarshal(rawSets, &sets); err != nil {
		return lcuItemSet{}, false, err
	}
	var result lcuItemSet
	found := false
	for _, rawSet := range sets {
		var candidate lcuItemSet
		if err := json.Unmarshal(rawSet, &candidate); err != nil {
			return lcuItemSet{}, false, err
		}
		if candidate.UID != uid {
			continue
		}
		if found {
			return lcuItemSet{}, false, errors.New("客户端返回了重复的装备方案")
		}
		result, found = candidate, true
	}
	return result, found, nil
}

func sameLCUItemSet(expected, actual lcuItemSet) bool {
	if expected.UID != actual.UID || expected.Title != actual.Title || expected.Type != actual.Type || expected.Map != actual.Map || expected.Mode != actual.Mode || len(expected.AssociatedChampions) != len(actual.AssociatedChampions) || len(expected.AssociatedMaps) != len(actual.AssociatedMaps) || len(expected.Blocks) != len(actual.Blocks) {
		return false
	}
	for index := range expected.AssociatedChampions {
		if expected.AssociatedChampions[index] != actual.AssociatedChampions[index] {
			return false
		}
	}
	for index := range expected.AssociatedMaps {
		if expected.AssociatedMaps[index] != actual.AssociatedMaps[index] {
			return false
		}
	}
	for blockIndex := range expected.Blocks {
		left, right := expected.Blocks[blockIndex], actual.Blocks[blockIndex]
		if left.Type != right.Type || len(left.Items) != len(right.Items) {
			return false
		}
		for itemIndex := range left.Items {
			if left.Items[itemIndex] != right.Items[itemIndex] {
				return false
			}
		}
	}
	return true
}

type gameplayReplayRequest struct {
	GameID int64  `json:"gameId"`
	Action string `json:"action"`
}
type gameplayReplayMetadata struct {
	DownloadProgress float64 `json:"downloadProgress"`
	GameID           int64   `json:"gameId"`
	State            string  `json:"state"`
}

func (a *app) handleGameplayReplayMetadata(w http.ResponseWriter, r *http.Request) {
	gameID, err := strconv.ParseInt(r.URL.Query().Get("gameId"), 10, 64)
	if err != nil || gameID <= 0 {
		http.Error(w, "对局编号无效", http.StatusBadRequest)
		return
	}
	client, _, err := a.gameplayClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	var metadata gameplayReplayMetadata
	path := fmt.Sprintf("/lol-replays/v1/metadata/%d", gameID)
	if err := client.GetJSON(path, &metadata); err != nil {
		var httpErr *LCUHTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			respondJSON(w, map[string]any{"available": false, "state": "unavailable"})
			return
		}
		http.Error(w, "回放状态读取失败", http.StatusConflict)
		return
	}
	respondJSON(w, map[string]any{"available": true, "state": metadata.State, "downloadProgress": metadata.DownloadProgress})
}

func (a *app) handleGameplayReplayAction(w http.ResponseWriter, r *http.Request) {
	var request gameplayReplayRequest
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || request.GameID <= 0 {
		http.Error(w, "回放参数无效", http.StatusBadRequest)
		return
	}
	if request.Action == "" {
		request.Action = "auto"
	}
	if request.Action != "auto" && request.Action != "download" && request.Action != "watch" {
		http.Error(w, "未知回放操作", http.StatusBadRequest)
		return
	}
	client, _, err := a.gameplayClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	action := request.Action
	state := ""
	if action == "auto" {
		var metadata gameplayReplayMetadata
		if err := client.RequestJSON(ctx, http.MethodGet, fmt.Sprintf("/lol-replays/v1/metadata/%d", request.GameID), nil, &metadata); err != nil {
			// 元数据在首次请求下载前通常是 404，这不代表回放不存在：
			// 直接发起下载，让客户端开始拉取回放文件。
			action = "download"
		} else {
			state = metadata.State
			switch metadata.State {
			case "incompatible":
				http.Error(w, "该回放与当前客户端版本不兼容（版本更新后旧对局无法回放）", http.StatusConflict)
				return
			case "watch":
				action = "watch"
			case "downloading", "checking":
				// 已在下载中：不重复触发，交给前端轮询等待。
				respondJSON(w, map[string]any{"accepted": true, "action": "download", "state": metadata.State, "downloadProgress": metadata.DownloadProgress})
				return
			default:
				action = "download"
			}
		}
	}
	path := fmt.Sprintf("/lol-replays/v1/rofls/%d/%s", request.GameID, action)
	if err := client.RequestJSON(ctx, http.MethodPost, path, map[string]string{"componentType": "replay-button_match-history"}, nil); err != nil {
		if action == "download" {
			http.Error(w, "客户端拒绝下载这场回放：官方回放只保留当前版本的对局", http.StatusConflict)
			return
		}
		http.Error(w, "回放操作失败："+safeGameplayError(err), http.StatusConflict)
		return
	}
	respondJSON(w, map[string]any{"accepted": true, "action": action, "state": state})
}

type gameplayPerk struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	IconPath         string `json:"iconPath"`
	StyleID          int64  `json:"styleId,omitempty"`
	ShortDescription string `json:"shortDesc,omitempty"`
	LongDescription  string `json:"longDesc,omitempty"`
}

// Riot Client's perkstyles.json has shipped both full perk objects and the
// compact integer-ID form. Keep the public response uniform so the browser can
// always render slot entries as objects, then enrich ID-only entries from
// perks.json in handleGameplayPerks.
func (perk *gameplayPerk) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*perk = gameplayPerk{}
		return nil
	}
	if first := trimmed[0]; (first >= '0' && first <= '9') || first == '-' {
		var id int64
		if err := json.Unmarshal([]byte(trimmed), &id); err != nil {
			return err
		}
		*perk = gameplayPerk{ID: id}
		return nil
	}
	type gameplayPerkAlias gameplayPerk
	var decoded gameplayPerkAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*perk = gameplayPerk(decoded)
	return nil
}

type gameplayPerkSlot struct {
	Type  string         `json:"type,omitempty"`
	Perks []gameplayPerk `json:"perks"`
}
type gameplayPerkStyle struct {
	ID       int64              `json:"id"`
	Name     string             `json:"name"`
	IconPath string             `json:"iconPath"`
	Slots    []gameplayPerkSlot `json:"slots"`
}

func parseGameplayPerkStyles(data []byte) ([]gameplayPerkStyle, error) {
	var wrapped struct {
		Styles []gameplayPerkStyle `json:"styles"`
	}
	wrappedErr := json.Unmarshal(data, &wrapped)
	if wrappedErr == nil && len(wrapped.Styles) > 0 {
		return wrapped.Styles, nil
	}
	var styles []gameplayPerkStyle
	arrayErr := json.Unmarshal(data, &styles)
	if arrayErr == nil && len(styles) > 0 {
		return styles, nil
	}
	if wrappedErr != nil && arrayErr != nil {
		return nil, fmt.Errorf("解析符文树目录失败（包装对象: %v；裸数组: %w）", wrappedErr, arrayErr)
	}
	return nil, errors.New("客户端符文树目录为空")
}

type gameplayItem struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IconPath    string `json:"iconPath,omitempty"`
	Price       int64  `json:"price,omitempty"`
}

type gameplaySummonerSpell struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IconPath    string `json:"iconPath,omitempty"`
}

type gameplayAugment struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	Description      string `json:"description,omitempty"`
	Rarity           string `json:"rarity,omitempty"`
	IconPath         string `json:"iconPath,omitempty"`
	FallbackIconPath string `json:"fallbackIconPath,omitempty"`
}

type gameplayAugmentRaw struct {
	ID                   int64  `json:"id"`
	Name                 string `json:"name"`
	NameTRA              string `json:"nameTRA"`
	SimpleNameTRA        string `json:"simpleNameTRA"`
	Description          string `json:"description"`
	Desc                 string `json:"desc"`
	Tooltip              string `json:"tooltip"`
	Rarity               string `json:"rarity"`
	AugmentSmallIconPath string `json:"augmentSmallIconPath"`
	IconPath             string `json:"iconPath"`
}

const gameplayPerkCatalogTTL = 30 * time.Minute

type gameplayPerkCatalogResponse struct {
	AugmentsPending bool `json:"augmentsPending,omitempty"`
	source          string
	Styles          []gameplayPerkStyle `json:"styles"`
	StatModSlots    []gameplayPerkSlot  `json:"statModSlots"`
	Perks           []gameplayPerk      `json:"perks"`
	Augments        []gameplayAugment   `json:"augments"`
}

type gameplayPerkCatalogCacheEntry struct {
	loadedAt time.Time
	payload  gameplayPerkCatalogResponse
}

func (a *app) handleGameplayPerks(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	client, _, clientErr := a.gameplayClient()
	cacheKey := "lcu"
	if clientErr != nil {
		client = nil
		cacheKey = "ddragon"
	}
	payload, err := a.cachedGameplayPerkCatalog(r.Context(), cacheKey, func() (gameplayPerkCatalogResponse, error) {
		return a.loadGameplayPerkCatalog(r.Context(), client)
	})
	if err == nil {
		payload = a.enrichPerkAugments(cacheKey, client, payload)
	}
	source := payload.source
	if source == "" {
		source = cacheKey
	}
	a.reportCatalogLoad("perks", source, len(payload.Perks), started, err)
	if err != nil {
		http.Error(w, "客户端未提供符文图标目录", http.StatusNotFound)
		return
	}
	respondJSON(w, payload)
}

func (a *app) cachedGameplayPerkCatalog(ctx context.Context, key string, loader func() (gameplayPerkCatalogResponse, error)) (gameplayPerkCatalogResponse, error) {
	a.perkCatalogMu.Lock()
	if entry, ok := a.perkCatalog[key]; ok && time.Since(entry.loadedAt) < gameplayPerkCatalogTTL {
		a.perkCatalogMu.Unlock()
		return entry.payload, nil
	}
	if a.perkCatalogDisk == nil {
		a.perkCatalogDisk = newPublicBinaryCache(a.storage, "perk-catalog", 8, 4<<20)
	}
	cache := a.perkCatalogDisk
	a.perkCatalogMu.Unlock()
	data, err := cache.load(ctx, "normalized-perks-v1|"+key, gameplayPerkCatalogTTL, 24*time.Hour, true, func(context.Context) ([]byte, error) {
		payload, err := loader()
		if err != nil {
			return nil, err
		}
		return json.Marshal(payload)
	})
	if err != nil {
		return gameplayPerkCatalogResponse{}, err
	}
	var payload gameplayPerkCatalogResponse
	if err = json.Unmarshal(data, &payload); err != nil {
		return payload, err
	}
	payload.source = key
	a.perkCatalogMu.Lock()
	defer a.perkCatalogMu.Unlock()
	if a.perkCatalog == nil {
		a.perkCatalog = make(map[string]gameplayPerkCatalogCacheEntry)
	}
	// Do not overwrite enrichment already published by an overlapping request.
	if entry, ok := a.perkCatalog[key]; ok && time.Since(entry.loadedAt) < gameplayPerkCatalogTTL {
		return entry.payload, nil
	}
	a.perkCatalog[key] = gameplayPerkCatalogCacheEntry{loadedAt: time.Now(), payload: payload}
	return payload, nil
}

func (a *app) loadGameplayPerkCatalog(ctx context.Context, client *LCUClient) (gameplayPerkCatalogResponse, error) {
	if client == nil {
		// 客户端未连接（例如只查看韩服页签）时改用 Data Dragon 中文目录。
		styles, perks, fallbackErr := a.fallbackGameplayPerks(ctx)
		if fallbackErr != nil {
			a.recordDiagnostic(map[string]any{
				"event": "perk_catalog_failed", "source": "ddragon", "reason": safeDiagnosticReason(fallbackErr),
				"payload_bytes": 0, "payload_prefix_shape": "unavailable",
			})
			return gameplayPerkCatalogResponse{}, fallbackErr
		}
		styles, perks, enriched, missingIcons := normalizeGameplayPerkCatalog(styles, perks)
		styles, statModSlots := splitGameplayStatModSlots(styles)
		if len(statModSlots) == 0 {
			statModSlots = defaultGameplayStatModSlots()
		}
		a.recordDiagnostic(map[string]any{
			"event": "perk_catalog_loaded", "source": "ddragon", "styles": len(styles), "perks": len(perks),
			"enriched_from_perks_json": enriched, "missing_icon_path": missingIcons,
		})
		return gameplayPerkCatalogResponse{source: "ddragon", Styles: styles, StatModSlots: statModSlots, Perks: perks}, nil
	}
	styleData, styleErr := client.GetBytesContext(ctx, "/lol-game-data/assets/v1/perkstyles.json")
	var styles []gameplayPerkStyle
	if styleErr == nil {
		styles, styleErr = parseGameplayPerkStyles(styleData)
		if styleErr != nil {
			a.recordDiagnostic(map[string]any{
				"event": "perk_catalog_failed", "source": "lcu", "reason": safeDiagnosticReason(styleErr),
				"payload_bytes": len(styleData), "payload_prefix_shape": diagnosticPayloadPrefixShape(styleData),
			})
		}
	} else {
		a.recordDiagnostic(map[string]any{
			"event": "perk_catalog_failed", "source": "lcu", "reason": safeDiagnosticReason(styleErr),
			"payload_bytes": 0, "payload_prefix_shape": "unavailable",
		})
	}
	var perks []gameplayPerk
	perkErr := client.GetJSON("/lol-game-data/assets/v1/perks.json", &perks)
	if perkErr != nil {
		a.recordDiagnostic(map[string]any{
			"event": "perk_catalog_failed", "source": "lcu", "catalog": "perks.json", "reason": safeDiagnosticReason(perkErr),
			"payload_bytes": 0, "payload_prefix_shape": "unavailable",
		})
	} else if len(perks) == 0 {
		a.recordDiagnostic(map[string]any{
			"event": "perk_catalog_failed", "source": "lcu", "catalog": "perks.json", "reason": "符文条目目录为空",
			"payload_bytes": 0, "payload_prefix_shape": "json_array",
		})
	}
	source := "lcu"
	if styleErr != nil || len(styles) == 0 || perkErr != nil || len(perks) == 0 {
		fallbackStyles, fallbackPerks, fallbackErr := a.fallbackGameplayPerks(ctx)
		if fallbackErr == nil {
			if styleErr != nil || len(styles) == 0 {
				styles = fallbackStyles
				source = "ddragon"
			}
			if perkErr != nil || len(perks) == 0 {
				perks = fallbackPerks
				source = "ddragon"
			}
		} else if styleErr != nil || len(styles) == 0 {
			a.recordDiagnostic(map[string]any{
				"event": "perk_catalog_failed", "source": "ddragon", "reason": safeDiagnosticReason(fallbackErr),
				"payload_bytes": 0, "payload_prefix_shape": "unavailable",
			})
			return gameplayPerkCatalogResponse{}, fallbackErr
		}
	}
	styles, perks, enriched, missingIcons := normalizeGameplayPerkCatalog(styles, perks)
	styles, statModSlots := splitGameplayStatModSlots(styles)
	if len(statModSlots) == 0 {
		statModSlots = defaultGameplayStatModSlots()
	}
	a.recordDiagnostic(map[string]any{
		"event": "perk_catalog_loaded", "source": source, "styles": len(styles), "perks": len(perks),
		"enriched_from_perks_json": enriched, "missing_icon_path": missingIcons,
	})
	return gameplayPerkCatalogResponse{source: source, Styles: styles, StatModSlots: statModSlots, Perks: perks}, nil
}

func normalizeGameplayPerkCatalog(styles []gameplayPerkStyle, perks []gameplayPerk) ([]gameplayPerkStyle, []gameplayPerk, int, int) {
	perkByID := make(map[int64]gameplayPerk, len(perks))
	for _, perk := range perks {
		perkByID[perk.ID] = perk
	}
	enriched := 0
	for styleIndex := range styles {
		for slotIndex := range styles[styleIndex].Slots {
			for perkIndex := range styles[styleIndex].Slots[slotIndex].Perks {
				entry := &styles[styleIndex].Slots[slotIndex].Perks[perkIndex]
				catalogEntry, ok := perkByID[entry.ID]
				if !ok {
					continue
				}
				idOnly := entry.Name == "" && entry.IconPath == "" && entry.ShortDescription == "" && entry.LongDescription == "" && entry.StyleID == 0
				if entry.Name == "" {
					entry.Name = catalogEntry.Name
				}
				if entry.IconPath == "" {
					entry.IconPath = catalogEntry.IconPath
				}
				if entry.ShortDescription == "" {
					entry.ShortDescription = catalogEntry.ShortDescription
				}
				if entry.LongDescription == "" {
					entry.LongDescription = catalogEntry.LongDescription
				}
				if entry.StyleID == 0 {
					entry.StyleID = catalogEntry.StyleID
				}
				if idOnly {
					enriched++
				}
			}
		}
	}
	missingIcons := 0
	for index := range styles {
		styles[index].IconPath = sanitizeAssetPath(styles[index].IconPath)
		if styles[index].IconPath == "" {
			missingIcons++
		}
		for slotIndex := range styles[index].Slots {
			for perkIndex := range styles[index].Slots[slotIndex].Perks {
				styles[index].Slots[slotIndex].Perks[perkIndex].IconPath = sanitizeAssetPath(styles[index].Slots[slotIndex].Perks[perkIndex].IconPath)
				if styles[index].Slots[slotIndex].Perks[perkIndex].IconPath == "" {
					missingIcons++
				}
			}
		}
	}
	for index := range perks {
		perks[index].IconPath = sanitizeAssetPath(perks[index].IconPath)
		if perks[index].IconPath == "" {
			missingIcons++
		}
	}
	return styles, perks, enriched, missingIcons
}

func splitGameplayStatModSlots(styles []gameplayPerkStyle) ([]gameplayPerkStyle, []gameplayPerkSlot) {
	var statModSlots []gameplayPerkSlot
	for styleIndex := range styles {
		kept := make([]gameplayPerkSlot, 0, len(styles[styleIndex].Slots))
		styleStatMods := make([]gameplayPerkSlot, 0, 3)
		for _, slot := range styles[styleIndex].Slots {
			if strings.EqualFold(strings.TrimSpace(slot.Type), "kStatMod") {
				styleStatMods = append(styleStatMods, slot)
				continue
			}
			kept = append(kept, slot)
		}
		styles[styleIndex].Slots = kept
		if len(statModSlots) == 0 && len(styleStatMods) > 0 {
			statModSlots = styleStatMods
		}
	}
	return styles, statModSlots
}

func defaultGameplayStatModSlots() []gameplayPerkSlot {
	perk := func(id int64, name, icon string) gameplayPerk {
		return gameplayPerk{ID: id, Name: name, IconPath: "ddragon:/cdn/img/perk-images/StatMods/" + icon}
	}
	adaptive := perk(5008, "适应之力", "StatModsAdaptiveForceIcon.png")
	healthScaling := perk(5001, "成长生命值", "StatModsHealthPlusIcon.png")
	return []gameplayPerkSlot{
		{Type: "kStatMod", Perks: []gameplayPerk{
			perk(5005, "攻击速度", "StatModsAttackSpeedIcon.png"), adaptive, perk(5007, "技能急速", "StatModsCDRScalingIcon.png"),
		}},
		{Type: "kStatMod", Perks: []gameplayPerk{
			adaptive, perk(5010, "移动速度", "StatModsMovementSpeedIcon.png"), healthScaling,
		}},
		{Type: "kStatMod", Perks: []gameplayPerk{
			perk(5011, "生命值", "StatModsHealthScalingIcon.png"), perk(5013, "韧性", "StatModsTenacityIcon.png"), healthScaling,
		}},
	}
}

func loadGameplayAugmentsFromClient(client *LCUClient) ([]gameplayAugment, error) {
	var raw []gameplayAugmentRaw
	if err := client.GetJSON("/lol-game-data/assets/v1/cherry-augments.json", &raw); err != nil {
		return nil, err
	}
	return normalizeGameplayAugments(raw), nil
}

func (a *app) fallbackGameplayAugments(ctx context.Context) ([]gameplayAugment, error) {
	loadCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	catalog, err := a.championDataProvider().loadCommunityDragonAugments(loadCtx)
	// This catalog backs ID-to-icon lookups, so it must retain both Kiwi and
	// Cherry namespaces just like the LCU catalog path.
	return catalog, err
}

func normalizeGameplayAugments(raw []gameplayAugmentRaw) []gameplayAugment {
	result := make([]gameplayAugment, 0, len(raw))
	seen := make(map[int64]bool, len(raw))
	for _, source := range raw {
		if source.ID <= 0 || seen[source.ID] {
			continue
		}
		name := strings.TrimSpace(source.NameTRA)
		if name == "" {
			name = strings.TrimSpace(source.SimpleNameTRA)
		}
		if name == "" {
			name = strings.TrimSpace(source.Name)
		}
		if name == "" {
			continue
		}
		description := cleanMarkup(firstNonEmpty(source.Description, source.Desc))
		if description == "" {
			description = cleanMarkup(source.Tooltip)
		}
		iconPath := sanitizeAssetPath(source.AugmentSmallIconPath)
		if iconPath == "" {
			iconPath = sanitizeAssetPath(source.IconPath)
		}
		iconPath, fallbackIconPath := normalizeAugmentIconPaths(iconPath)
		seen[source.ID] = true
		result = append(result, gameplayAugment{ID: source.ID, Name: name, Description: description, Rarity: strings.TrimSpace(source.Rarity), IconPath: iconPath, FallbackIconPath: fallbackIconPath})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

// normalizeAugmentIconPaths promotes an explicitly named small icon to the
// official large artwork while retaining the small asset for load failures.
// Other filenames are left untouched because not every catalog entry follows
// the *_small.png naming convention.
func normalizeAugmentIconPaths(path string) (string, string) {
	path = sanitizeAssetPath(path)
	if path == "" {
		return "", ""
	}
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, "/cherry/augments/icons/drop_bear_small.png") || strings.HasSuffix(lower, "/cherry/augments/icons/drop_bear_large.png") {
		// Verified in the actual game asset directory: no _large file; the
		// unsuffixed artwork is colored, whereas _small is a monochrome glyph.
		directory := path[:strings.LastIndex(path, "/")+1]
		return directory + "drop_bear.png", directory + "drop_bear_small.png"
	}
	const suffix = "_small.png"
	if !strings.HasSuffix(lower, suffix) {
		return path, ""
	}
	large := path[:len(path)-len(suffix)] + "_large.png"
	return large, path
}

func (a *app) handleGameplayItems(w http.ResponseWriter, r *http.Request) {
	client, _, err := a.gameplayClient()
	if err != nil {
		items, fallbackErr := a.fallbackGameplayItems(r.Context())
		if fallbackErr != nil {
			http.Error(w, "客户端未提供装备目录", http.StatusNotFound)
			return
		}
		respondJSON(w, map[string]any{"items": items})
		return
	}
	var raw []struct {
		ID               int64  `json:"id"`
		Name             string `json:"name"`
		DisplayName      string `json:"displayName"`
		Description      string `json:"description"`
		ShortDescription string `json:"shortDescription"`
		IconPath         string `json:"iconPath"`
		ImagePath        string `json:"imagePath"`
		PriceTotal       int64  `json:"priceTotal"`
	}
	catalogStarted := time.Now()
	catalogErr := client.GetJSONContext(r.Context(), "/lol-game-data/assets/v1/items.json", &raw)
	a.reportCatalogLoad("items", "lcu", len(raw), catalogStarted, catalogErr)
	if catalogErr != nil || len(raw) == 0 {
		items, fallbackErr := a.fallbackGameplayItems(r.Context())
		if fallbackErr != nil {
			http.Error(w, "客户端未提供装备目录", http.StatusNotFound)
			return
		}
		respondJSON(w, map[string]any{"items": items})
		return
	}
	items := make([]gameplayItem, 0, len(raw))
	for _, source := range raw {
		name := strings.TrimSpace(source.Name)
		if name == "" {
			name = strings.TrimSpace(source.DisplayName)
		}
		description := cleanMarkup(source.Description)
		if description == "" {
			description = cleanMarkup(source.ShortDescription)
		}
		iconPath := sanitizeAssetPath(source.IconPath)
		if iconPath == "" {
			iconPath = sanitizeAssetPath(source.ImagePath)
		}
		if source.ID > 0 {
			items = append(items, gameplayItem{ID: source.ID, Name: name, Description: description, IconPath: iconPath, Price: source.PriceTotal})
		}
	}
	if len(items) == 0 {
		items, err = a.fallbackGameplayItems(r.Context())
		if err != nil {
			http.Error(w, "客户端未提供有效的装备目录", http.StatusNotFound)
			return
		}
	}
	respondJSON(w, map[string]any{"items": items})
}

func (a *app) handleGameplaySummonerSpells(w http.ResponseWriter, r *http.Request) {
	client, _, err := a.gameplayClient()
	if err != nil {
		spells, fallbackErr := a.fallbackGameplaySummonerSpells(r.Context())
		if fallbackErr != nil {
			http.Error(w, "客户端未提供召唤师技能目录", http.StatusNotFound)
			return
		}
		respondJSON(w, map[string]any{"spells": spells})
		return
	}
	var raw []struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		IconPath    string `json:"iconPath"`
		ImagePath   string `json:"imagePath"`
	}
	catalogStarted := time.Now()
	catalogErr := client.GetJSONContext(r.Context(), "/lol-game-data/assets/v1/summoner-spells.json", &raw)
	a.reportCatalogLoad("summoner-spells", "lcu", len(raw), catalogStarted, catalogErr)
	if catalogErr != nil || len(raw) == 0 {
		spells, fallbackErr := a.fallbackGameplaySummonerSpells(r.Context())
		if fallbackErr != nil {
			http.Error(w, "客户端未提供召唤师技能目录", http.StatusNotFound)
			return
		}
		respondJSON(w, map[string]any{"spells": spells})
		return
	}
	spells := make([]gameplaySummonerSpell, 0, len(raw))
	for _, source := range raw {
		iconPath := sanitizeAssetPath(source.IconPath)
		if iconPath == "" {
			iconPath = sanitizeAssetPath(source.ImagePath)
		}
		if source.ID > 0 {
			spells = append(spells, gameplaySummonerSpell{ID: source.ID, Name: strings.TrimSpace(source.Name), Description: cleanMarkup(source.Description), IconPath: iconPath})
		}
	}
	if len(spells) == 0 {
		spells, err = a.fallbackGameplaySummonerSpells(r.Context())
		if err != nil {
			http.Error(w, "客户端未提供有效的召唤师技能目录", http.StatusNotFound)
			return
		}
	}
	respondJSON(w, map[string]any{"spells": spells})
}

// ---------- 未连接客户端时的 Data Dragon 目录兜底 ----------
// iconPath 以 "ddragon:" 前缀标记，前端会改走 /api/champion-asset 代理。

func (a *app) fallbackGameplayItems(ctx context.Context) (result []gameplayItem, resultErr error) {
	started := time.Now()
	defer func() { a.reportCatalogLoad("items", "ddragon", len(result), started, resultErr) }()
	loadCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	descriptions, err := a.champions.loadStaticDescriptions(loadCtx)
	if err != nil {
		return nil, err
	}
	items := make([]gameplayItem, 0, len(descriptions))
	for key, description := range descriptions {
		file, ok := strings.CutPrefix(key, "item/")
		if !ok {
			continue
		}
		id, parseErr := strconv.ParseInt(strings.TrimSuffix(file, ".png"), 10, 64)
		if parseErr != nil || id <= 0 {
			continue
		}
		items = append(items, gameplayItem{ID: id, Name: description.Name, Description: description.Description, IconPath: "ddragon:" + description.Path, Price: description.Price})
	}
	if len(items) == 0 {
		return nil, errors.New("Data Dragon 装备目录为空")
	}
	sort.Slice(items, func(left, right int) bool { return items[left].ID < items[right].ID })
	return items, nil
}

func (a *app) fallbackGameplaySummonerSpells(ctx context.Context) (result []gameplaySummonerSpell, resultErr error) {
	started := time.Now()
	defer func() { a.reportCatalogLoad("summoner-spells", "ddragon", len(result), started, resultErr) }()
	loadCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	descriptions, err := a.champions.loadStaticDescriptions(loadCtx)
	if err != nil {
		return nil, err
	}
	spells := make([]gameplaySummonerSpell, 0, 24)
	for key, description := range descriptions {
		rawID, ok := strings.CutPrefix(key, "spell-id/")
		if !ok {
			continue
		}
		id, parseErr := strconv.ParseInt(rawID, 10, 64)
		if parseErr != nil || id <= 0 {
			continue
		}
		spells = append(spells, gameplaySummonerSpell{ID: id, Name: description.Name, Description: description.Description, IconPath: "ddragon:" + description.Path})
	}
	if len(spells) == 0 {
		return nil, errors.New("Data Dragon 召唤师技能目录为空")
	}
	return spells, nil
}

func (a *app) fallbackGameplayPerks(ctx context.Context) ([]gameplayPerkStyle, []gameplayPerk, error) {
	loadCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	// loadStaticDescriptions 会顺带解析并缓存当前补丁号。
	if _, err := a.champions.loadStaticDescriptions(loadCtx); err != nil {
		return nil, nil, err
	}
	patch := a.champions.currentPatch()
	if !validDDragonVersion(patch) {
		return nil, nil, errors.New("Data Dragon 版本不可用")
	}
	data, err := a.champions.fetch(loadCtx, dataDragonHost, "/cdn/"+patch+"/data/zh_CN/runesReforged.json", nil, championJSONMax, "application/json")
	if err != nil {
		return nil, nil, err
	}
	var rawStyles []ddragonRuneStyle
	if err := json.Unmarshal(data, &rawStyles); err != nil {
		return nil, nil, err
	}
	styles := make([]gameplayPerkStyle, 0, len(rawStyles))
	perks := make([]gameplayPerk, 0, 64)
	for _, raw := range rawStyles {
		style := gameplayPerkStyle{ID: int64(raw.ID), Name: raw.Name, IconPath: "ddragon:/cdn/img/" + strings.TrimPrefix(raw.Icon, "/")}
		for _, slot := range raw.Slots {
			perkSlot := gameplayPerkSlot{}
			for _, item := range slot.Runes {
				perk := gameplayPerk{ID: int64(item.ID), Name: item.Name, IconPath: "ddragon:/cdn/img/" + strings.TrimPrefix(item.Icon, "/"), StyleID: int64(raw.ID), ShortDescription: item.ShortDesc, LongDescription: item.LongDesc}
				perkSlot.Perks = append(perkSlot.Perks, perk)
				perks = append(perks, perk)
			}
			style.Slots = append(style.Slots, perkSlot)
		}
		styles = append(styles, style)
	}
	if len(styles) == 0 {
		return nil, nil, errors.New("Data Dragon 符文目录为空")
	}
	return styles, perks, nil
}

func loadQueueLabels(client *LCUClient) map[int64]string {
	return loadQueueLabelsContext(context.Background(), client)
}

func loadQueueLabelsContext(ctx context.Context, client *LCUClient) map[int64]string {
	if client == nil {
		return map[int64]string{}
	}
	client.queueLabelsMu.Lock()
	if client.queueLabelsLoaded {
		result := cloneQueueLabels(client.queueLabels)
		client.queueLabelsMu.Unlock()
		return result
	}
	client.queueLabelsMu.Unlock()
	var queues []struct {
		ID        int64  `json:"id"`
		Name      string `json:"name"`
		ShortName string `json:"shortName"`
	}
	result := make(map[int64]string)
	if client.GetJSONContext(ctx, "/lol-game-queues/v1/queues", &queues) == nil {
		for _, queue := range queues {
			name := strings.TrimSpace(queue.ShortName)
			if name == "" {
				name = strings.TrimSpace(queue.Name)
			}
			if queue.ID > 0 && name != "" {
				result[queue.ID] = name
			}
		}
		client.queueLabelsMu.Lock()
		if !client.queueLabelsLoaded {
			client.queueLabels = cloneQueueLabels(result)
			client.queueLabelsLoaded = true
		} else {
			result = cloneQueueLabels(client.queueLabels)
		}
		client.queueLabelsMu.Unlock()
	}
	return result
}

func cloneQueueLabels(labels map[int64]string) map[int64]string {
	result := make(map[int64]string, len(labels))
	for queueID, label := range labels {
		result[queueID] = label
	}
	return result
}

func queueLabel(queueID int64, mode string, labels map[int64]string) string {
	if label := strings.TrimSpace(labels[queueID]); containsChineseQueueLabel(label) {
		return label
	}
	if definition, ok := supportedQueueDefinition(queueID); ok && containsChineseQueueLabel(definition.Name) {
		return definition.Name
	}
	if label := gameModeLabelsZH[strings.ToUpper(strings.TrimSpace(mode))]; label != "" {
		return label
	}
	if queueID == 0 && strings.TrimSpace(mode) == "" {
		return "自定义对局"
	}
	return "其他模式"
}

func containsChineseQueueLabel(label string) bool {
	for _, r := range label {
		if r >= '一' && r <= '鿿' {
			return true
		}
	}
	return false
}

var gameModeLabelsZH = map[string]string{
	"CHERRY": "斗魂竞技场", "ARENA": "斗魂竞技场", "CLASSIC": "召唤师峡谷", "ARAM": "极地大乱斗",
	"KIWI": "海克斯大乱斗", "ARAM_MAYHEM": "海克斯大乱斗", "ARAM_MAYHEM_CLASSIC": "海克斯大乱斗（经典模式版）",
	"STRAWBERRY": "无尽狂潮", "ONEFORALL": "克隆大作战", "ULTBOOK": "终极魔典", "NEXUSBLITZ": "极限闪击",
	"KINGPORO": "魄罗大乱斗", "TUTORIAL": "新手教程", "TFT": "云顶之弈", "URF": "无限火力", "DOOMBOTSTEEMO": "末日人工智能",
}

func queueModeGroupFor(queueID int64, gameMode string, mapID int64) string {
	return queueModeGroupForLabel(queueID, gameMode, mapID, "")
}

func queueModeGroupForLabel(queueID int64, gameMode string, mapID int64, label string) string {
	mode := strings.ToUpper(strings.TrimSpace(gameMode))
	label = strings.TrimSpace(label)
	labelLower := strings.ToLower(label)
	switch {
	case mode == gameModeARAMMayhemClassic || strings.Contains(label, "经典模式版") || (strings.Contains(labelLower, "classic") && (strings.Contains(labelLower, "mayhem") || strings.Contains(labelLower, "hextech aram"))):
		return "hextech-classic"
	case strings.Contains(label, "海选赛") || (strings.Contains(labelLower, "qualifier") && (strings.Contains(labelLower, "mayhem") || strings.Contains(labelLower, "hextech aram"))):
		return "hextech-qualifier"
	case (mode == gameModeKIWI || mode == gameModeARAMMayhem) && (mapID == 0 || mapID == 12):
		return "hextech-aram"
	}
	if definition, ok := supportedQueueDefinition(queueID); ok {
		return definition.ModeGroup
	}
	resolved := resolveGameplayRecommendationMode(queueID, gameMode, mapID)
	switch resolved.InternalMode {
	case "hextech-aram", "arena", "aram", "urf", "nexus-blitz":
		return resolved.InternalMode
	default:
		return "other"
	}
}

func (a *app) recordMatchModeClassifications(matches []gameplayMatch) {
	if a == nil {
		return
	}
	for _, match := range matches {
		a.matchModeDiagnosticMu.Lock()
		if a.matchModeDiagnosticKeys == nil {
			a.matchModeDiagnosticKeys = make(map[int64]struct{})
		}
		if _, recorded := a.matchModeDiagnosticKeys[match.QueueID]; recorded {
			a.matchModeDiagnosticMu.Unlock()
			continue
		}
		if len(a.matchModeDiagnosticKeys) >= 128 {
			a.matchModeDiagnosticKeys = make(map[int64]struct{})
		}
		a.matchModeDiagnosticKeys[match.QueueID] = struct{}{}
		a.matchModeDiagnosticMu.Unlock()
		kind := match.ModeGroup
		if definition, ok := supportedQueueDefinition(match.QueueID); ok {
			kind = definition.Filter
		}
		a.recordDiagnostic(map[string]any{
			"event": "match_mode_classified", "queue_id": match.QueueID,
			"game_mode": match.GameMode, "map_id": match.MapID,
			"queue_label": match.QueueLabel, "mode_group": match.ModeGroup, "kind": kind,
		})
	}
}

type gameplayFilterSummary struct {
	Visible                  int
	SkippedEmptyParticipants int
	FilteredCustom           int
	EmptyParticipantPayloads int
	CustomReasons            map[string]int
}

func customGameplayMatchReasons(match gameplayMatch) []string {
	gameType := strings.ToUpper(strings.TrimSpace(match.GameType))
	gameMode := strings.ToUpper(strings.TrimSpace(match.GameMode))
	modeGroup := strings.ToLower(strings.TrimSpace(match.ModeGroup))
	reasons := make([]string, 0, 4)
	if match.QueueID == 0 {
		reasons = append(reasons, "queue_id_zero")
	}
	if gameType == "CUSTOM" || gameType == "CUSTOM_GAME" {
		reasons = append(reasons, "game_type")
	}
	if gameMode == "CUSTOM" || gameMode == "CUSTOM_GAME" {
		reasons = append(reasons, "game_mode")
	}
	if modeGroup == "custom" {
		reasons = append(reasons, "mode_group")
	}
	return reasons
}

func isCustomGameplayMatch(match gameplayMatch) bool {
	return len(customGameplayMatchReasons(match)) > 0
}

func addCustomReasonCounts(counts map[string]int, reasons []string) {
	for _, reason := range reasons {
		counts[reason]++
	}
}

func summarizeRiotGameplayFilters(infos []*riotMatchInfo) gameplayFilterSummary {
	summary := gameplayFilterSummary{CustomReasons: make(map[string]int)}
	for _, info := range infos {
		if info == nil || len(info.Participants) == 0 {
			summary.SkippedEmptyParticipants++
			continue
		}
		reasons := customGameplayMatchReasons(gameplayMatch{QueueID: info.QueueID, GameMode: info.GameMode, GameType: info.GameType, ModeGroup: queueModeGroupFor(info.QueueID, info.GameMode, info.MapID)})
		if len(reasons) > 0 {
			summary.FilteredCustom++
			addCustomReasonCounts(summary.CustomReasons, reasons)
			continue
		}
		summary.Visible++
	}
	return summary
}

func summarizeLCUGameplayFilters(games []lcuGame) gameplayFilterSummary {
	summary := gameplayFilterSummary{CustomReasons: make(map[string]int)}
	for _, game := range games {
		if len(game.Participants) == 0 {
			summary.EmptyParticipantPayloads++
		}
		reasons := customGameplayMatchReasons(gameplayMatch{QueueID: game.QueueID, GameMode: game.GameMode, GameType: game.GameType, ModeGroup: queueModeGroupFor(game.QueueID, game.GameMode, game.MapID)})
		if len(reasons) > 0 {
			summary.FilteredCustom++
			addCustomReasonCounts(summary.CustomReasons, reasons)
			continue
		}
		summary.Visible++
	}
	return summary
}

func validPlayerReference(value string) bool {
	return playerReferencePattern.MatchString(strings.TrimSpace(value))
}
func clampMatchCount(value int) int {
	if value <= 0 {
		return defaultMatchCount
	}
	if value > maximumMatchCount {
		return maximumMatchCount
	}
	if value < 5 {
		return 5
	}
	return value
}
func clampMatchStart(value int) int {
	if value < 0 {
		return 0
	}
	if value > maximumMatchStart {
		return maximumMatchStart
	}
	return value
}
func clampSummaryMatchCount(value int) int {
	if value <= 0 {
		return defaultMatchCount
	}
	if value > maximumSummaryMatchCount {
		return maximumSummaryMatchCount
	}
	if value < 5 {
		return 5
	}
	return value
}
func normalizeEpochMillis(value int64) int64 {
	if value > 0 && value < 100000000000 {
		return value * 1000
	}
	return value
}
func compactPositiveInt64(values ...int64) []int64 {
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value > 0 {
			result = append(result, value)
		}
	}
	return result
}

// itemSlots 保留装备栏原始槽位（含空位），负数归零；
// 第 7 个槽位固定是饰品（守卫/扫描等），前端按槽位渲染。
func itemSlots(values ...int64) []int64 {
	result := make([]int64, len(values))
	for index, value := range values {
		if value > 0 {
			result[index] = value
		}
	}
	return result
}
func championName(names map[int64]string, id int64) string {
	if value := strings.TrimSpace(names[id]); value != "" {
		return value
	}
	if id > 0 {
		return fmt.Sprintf("英雄 %d", id)
	}
	if id < 0 {
		return "随机待定"
	}
	return "未知英雄"
}

func positiveChampionPickIntent(value int64) int64 {
	if value > 0 {
		return value
	}
	return 0
}
func ratio(numerator, denominator int) float64 {
	if denominator <= 0 {
		denominator = 1
	}
	return float64(numerator) / float64(denominator)
}
func perMinute(value int, seconds int64) float64 {
	if seconds <= 0 {
		return 0
	}
	return float64(value) / (float64(seconds) / 60)
}
func round1(value float64) float64 { return math.Round(value*10) / 10 }
func round2(value float64) float64 { return math.Round(value*100) / 100 }

func normalizePosition(lane, role string) string {
	value := strings.ToUpper(strings.TrimSpace(lane + " " + role))
	switch {
	case strings.Contains(value, "JUNGLE"):
		return "jungle"
	case strings.Contains(value, "MIDDLE"), strings.Contains(value, "MID"):
		return "middle"
	case strings.Contains(value, "TOP"):
		return "top"
	case strings.Contains(value, "UTILITY"), strings.Contains(value, "SUPPORT"):
		return "utility"
	case strings.Contains(value, "BOTTOM"), strings.Contains(value, "BOT"), strings.Contains(value, "CARRY"):
		return "bottom"
	case strings.TrimSpace(value) == "":
		return ""
	default:
		return "other"
	}
}

func gameplayCapabilityError(name, path string, err error) EndpointCapability {
	if isCancellation(err) {
		return EndpointCapability{Name: name, Path: path, State: capabilityCanceled}
	}
	capability := EndpointCapability{Name: name, Path: path, State: capabilityFailed, Detail: "读取失败，界面已保留可核验数据"}
	var httpErr *LCUHTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		capability.State = capabilityUnsupported
		capability.Detail = "当前客户端版本未提供此接口"
	}
	return capability
}

func safeGameplayError(err error) string {
	var httpErr *LCUHTTPError
	if errors.As(err, &httpErr) {
		if httpErr.StatusCode == http.StatusNotFound {
			return "当前客户端版本不支持"
		}
		if httpErr.StatusCode == http.StatusConflict {
			return "客户端当前状态不允许此操作"
		}
	}
	return "请确认客户端仍在英雄选择或大厅阶段"
}
