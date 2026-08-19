package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
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
	defaultMatchCount        = 20
	maximumMatchCount        = 50
	maximumSummaryMatchCount = 100
	maximumMatchStart        = 10000
)

var playerReferencePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)

// Riot's hidden champ-select UUID is reversible with the same fixed XOR mask
// used by LeagueAkari's MIT-licensed magic compatibility addon. Keeping the
// transform in Go avoids loading an opaque native binary and keeps the real
// PUUID inside the authenticated backend.
var hiddenPlayerUUIDMask = [16]byte{0x81, 0x70, 0x76, 0xa9, 0xf4, 0x51, 0x50, 0x9b, 0x95, 0x98, 0x68, 0x13, 0xce, 0x91, 0x17, 0xe7}

type gameplayOverviewRequest struct {
	PlayerRef string `json:"playerRef"`
	GameName  string `json:"gameName"`
	TagLine   string `json:"tagLine"`
	// Region 指定查询的服务器："kr" 走 Riot 官方 API 查询韩服；
	// 留空表示国服，继续通过本机客户端查询。
	Region string `json:"region"`
	// ServerID 是国服子服务器（例如 HN1）；韩服必须留空。
	ServerID string `json:"serverId"`
	Count    int    `json:"count"`
	BegIndex int    `json:"begIndex"`
}

type gameplayPagination struct {
	BegIndex int  `json:"begIndex"`
	Count    int  `json:"count"`
	Total    int  `json:"total,omitempty"`
	HasMore  bool `json:"hasMore"`
}

type gameplayOverview struct {
	Player        gameplayPlayer         `json:"player"`
	Ranks         []gameplayRank         `json:"ranks"`
	Overall       gameplayAggregate      `json:"overall"`
	SevenDayRank  gameplayAggregate      `json:"sevenDayRank"`
	ChampionStats []gameplayChampionStat `json:"championStats"`
	Positions     []gameplayPositionStat `json:"positions"`
	Masteries     []gameplayMastery      `json:"masteries"`
	RecentPlayers []gameplayRecentPlayer `json:"recentPlayers"`
	ActivityHours []int                  `json:"activityHours"`
	Matches       []gameplayMatch        `json:"matches"`
	Pagination    gameplayPagination     `json:"pagination"`
	Capabilities  []EndpointCapability   `json:"capabilities"`
}

type gameplayPlayer struct {
	PlayerRef     string `json:"playerRef,omitempty"`
	DisplayName   string `json:"displayName"`
	GameName      string `json:"gameName,omitempty"`
	TagLine       string `json:"tagLine,omitempty"`
	ProfileIconID int64  `json:"profileIconId,omitempty"`
	SummonerLevel int64  `json:"summonerLevel,omitempty"`
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

type gameplayAggregate struct {
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
	ParticipantID  int64   `json:"participantId"`
	TeamID         int64   `json:"teamId"`
	PlayerRef      string  `json:"playerRef,omitempty"`
	DisplayName    string  `json:"displayName"`
	GameName       string  `json:"gameName,omitempty"`
	TagLine        string  `json:"tagLine,omitempty"`
	ProfileIconID  int64   `json:"profileIconId,omitempty"`
	ChampionID     int64   `json:"championId"`
	ChampionName   string  `json:"championName"`
	ChampionLevel  int     `json:"championLevel"`
	Spell1ID       int64   `json:"spell1Id,omitempty"`
	Spell2ID       int64   `json:"spell2Id,omitempty"`
	PrimaryStyleID int64   `json:"primaryStyleId,omitempty"`
	SubStyleID     int64   `json:"subStyleId,omitempty"`
	PerkIDs        []int64 `json:"perkIds"`
	AugmentIDs     []int64 `json:"augmentIds,omitempty"`
	ItemIDs        []int64 `json:"itemIds"`
	Position       string  `json:"position,omitempty"`
	Kills          int     `json:"kills"`
	Deaths         int     `json:"deaths"`
	Assists        int     `json:"assists"`
	KDA            float64 `json:"kda"`
	CS             int     `json:"cs"`
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
	Win         bool    `json:"win"`
	Hidden      bool    `json:"hidden"`
	MultiKill   int     `json:"multiKill,omitempty"`
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
	QueueType     string `json:"queueType"`
	Tier          string `json:"tier"`
	Division      string `json:"division"`
	LeaguePoints  int    `json:"leaguePoints"`
	Wins          int    `json:"wins"`
	Losses        int    `json:"losses"`
	IsProvisional bool   `json:"isProvisional"`
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
	GameCreationDate      string                   `json:"gameCreationDate"`
	GameDuration          int64                    `json:"gameDuration"`
	GameID                int64                    `json:"gameId"`
	GameMode              string                   `json:"gameMode"`
	MapID                 int64                    `json:"mapId"`
	QueueID               int64                    `json:"queueId"`
	ParticipantIdentities []lcuParticipantIdentity `json:"participantIdentities"`
	Participants          []lcuParticipant         `json:"participants"`
	Teams                 []lcuTeam                `json:"teams"`
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
	request := gameplayOverviewRequest{Count: defaultMatchCount}
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
		response, err := a.loadRiotOverview(r.Context(), reference, request.BegIndex, request.Count)
		if err != nil {
			http.Error(w, err.Error(), riotErrorStatus(err))
			return
		}
		respondJSON(w, response)
		return
	}
	client, current, err := a.gameplayClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	// 按 Riot ID 打开国服玩家（顶部搜索）：Riot Client 的全局 aliases
	// 接口先解析 PUUID，再由所选子服务器的 SGP 确认召唤师归属。
	if request.PlayerRef == "" && request.GameName != "" {
		if request.ServerID == "" {
			_, platform := client.platformInfo()
			request.ServerID, _ = normalizeTencentServerID(platform)
		}
		if request.ServerID == "" {
			http.Error(w, "请选择要查询的国服服务器", http.StatusBadRequest)
			return
		}
		resolved, lookupErr := a.resolveTencentRiotID(r.Context(), client, request.GameName, request.TagLine, request.ServerID)
		if lookupErr != nil {
			switch {
			case errors.Is(lookupErr, errTencentPlayerNotFound):
				http.Error(w, "所选服务器没有找到该玩家，请核对名称、编号和服务器", http.StatusNotFound)
			case errors.Is(lookupErr, errRiotClientNotFound), errors.Is(lookupErr, errRiotClientCredentialsUnreadable):
				http.Error(w, "国服跨服搜索需要 Riot Client 服务正在运行，请从英雄联盟客户端登录后重试", http.StatusConflict)
			default:
				http.Error(w, "国服跨服查询暂时不可用，请确认客户端已登录后重试", http.StatusBadGateway)
			}
			return
		}
		reference = resolved
	}
	if reference.ServerID == "" {
		_, platform := client.platformInfo()
		reference.ServerID, _ = normalizeTencentServerID(platform)
	}
	response := a.loadGameplayOverview(r.Context(), client, current, reference, request.BegIndex, request.Count)
	respondJSON(w, response)
}

var errTencentPlayerNotFound = errors.New("Tencent player was not found on selected server")

func (a *app) resolveTencentRiotID(ctx context.Context, lcu *LCUClient, gameName, tagLine, serverID string) (gameplayReference, error) {
	serverID, ok := normalizeTencentServerID(serverID)
	if !ok {
		return gameplayReference{}, errTencentPlayerNotFound
	}
	discover := a.riotClientDiscovery
	if discover == nil {
		discover = discoverRiotClient
	}
	riotClient, err := discover()
	if err != nil {
		return gameplayReference{}, err
	}
	defer riotClient.Close()
	aliases, err := riotClient.aliasesByRiotID(ctx, gameName, tagLine)
	if err != nil {
		if errors.Is(err, errRiotClientAliasNotFound) {
			return gameplayReference{}, errTencentPlayerNotFound
		}
		return gameplayReference{}, err
	}
	for _, alias := range aliases {
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
			ProfileIconID: summoner.ProfileIconID, SummonerLevel: summoner.Level, ServerID: serverID,
		}), nil
	}
	return gameplayReference{}, errTencentPlayerNotFound
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

// recentWindowDays 是“最近排位 / 最近一起玩”等汇总统计的时间窗口（天）。
const recentWindowDays = 30

func (a *app) loadGameplayOverview(ctx context.Context, client *LCUClient, current Summoner, reference gameplayReference, begIndex, count int) gameplayOverview {
	reference = normalizeGameplayReference(reference)
	if reference.Region == "" && reference.ServerID == "" {
		reference.ServerID = clientTencentServerID(client)
	}
	playerRef := reference.PlayerRef
	isCurrent := (playerRef == "" || gameplayReferenceContains(reference, current.PUUID)) && !isRemoteTencentServer(client, reference.ServerID)
	player := current
	profileHidden := strings.TrimSpace(player.GameName) == "" && strings.TrimSpace(player.DisplayName) == ""
	capabilities := make([]EndpointCapability, 0, 5)
	if isCurrent {
		playerRef = current.PUUID
		reference = mergeGameplayReferences(reference, gameplayReferenceFromSummoner(current))
		capabilities = append(capabilities, EndpointCapability{Name: "summoner", Path: "/lol-summoner/v1/current-summoner", State: capabilityAvailable, Count: 1})
	} else if reference.ServerID != "" {
		loaded, loadErr := a.sgp.summonerByPUUIDOn(ctx, client, reference.ServerID, playerRef)
		capability := EndpointCapability{Name: "summoner", Path: "sgp: /summoner-ledge/v1/regions/{server}/summoners/puuids"}
		if loadErr != nil {
			capability.State = capabilityFailed
			capability.Detail = "所选服务器暂时无法读取召唤师资料"
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
	queueLabels := loadQueueLabels(client)
	names := a.overviewChampionNames(ctx)
	matches, historyCapabilities, pagination := a.loadDetailedMatches(ctx, client, reference, playerRef, isCurrent, begIndex, count, names, queueLabels)
	capabilities = append(capabilities, historyCapabilities...)
	// 标注本地追踪到的排位胜点变化（仅当前登录玩家的场次有记录）。
	if isCurrent {
		a.lpTracker.annotate(matches, playerRef)
	}
	response := gameplayOverview{
		Player: gameplayPlayer{
			PlayerRef: playerRef, DisplayName: gameplayDisplayName(player), GameName: player.GameName,
			TagLine: player.TagLine, ProfileIconID: player.ProfileIconID, SummonerLevel: player.SummonerLevel,
			Region: reference.Region, ServerID: reference.ServerID, ServerName: tencentServerName(reference.ServerID),
			Hidden: profileHidden, PrivateHistory: strings.EqualFold(strings.TrimSpace(player.Privacy), "PRIVATE"),
			IsCurrent: isCurrent, reference: reference,
		},
		Matches: matches, Capabilities: capabilities,
		Pagination: pagination,
	}
	if begIndex > 0 {
		a.publicizeOverviewReferences(&response)
		return response
	}
	ranks, rankCapability := a.loadRanksWithFallback(ctx, client, playerRef, isCurrent, reference.ServerID)
	capabilities = append(capabilities, rankCapability)
	// 刷新 LP 追踪基线：下一场结算时据此计算胜点变化。
	if isCurrent {
		a.lpTracker.observe(playerRef, ranks)
	}
	windowMatches := matches
	windowAvailable := false
	windowExhausted := false
	remoteServer := isRemoteTencentServer(client, reference.ServerID)
	if reference.ServerID != "" && validPlayerReference(playerRef) {
		infos, _, more, windowErr := a.sgp.matchHistoryOn(ctx, client, reference.ServerID, playerRef, 0, maximumSummaryMatchCount, false)
		windowCapability := EndpointCapability{Name: "seven-day-history", Path: "sgp: /match-history-query/v1/products/lol/{player}/SUMMARY", Detail: "用于过去 30 天排位与活跃时段统计"}
		if windowErr == nil {
			windowCapability.State = capabilityAvailable
			windowCapability.Count = len(infos)
			windowAvailable = true
			windowExhausted = !more
			if len(infos) > len(matches) {
				windowMatches = make([]gameplayMatch, 0, len(infos))
				for _, info := range infos {
					windowMatches = append(windowMatches, convertRiotMatchInfo(info, playerRef, names, queueLabels, "", reference.ServerID))
				}
			}
		} else {
			windowCapability.State = capabilityFailed
			windowCapability.Detail = "所选服务器暂时无法读取 30 天统计样本"
		}
		capabilities = append(capabilities, windowCapability)
	}
	if !windowAvailable && !remoteServer {
		windowGames, windowCapabilities, historyTotal := loadGameplayHistory(client, playerRef, isCurrent, 0, maximumSummaryMatchCount, false)
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
				windowMatches = append(windowMatches, normalizeGameplayMatch(game, reference, names, queueLabels))
			}
		}
		returnedAllHistory := historyTotal > 0 && len(windowGames) >= historyTotal
		shortHistoryPage := historyTotal == 0 && len(windowGames) < maximumSummaryMatchCount
		windowExhausted = windowAvailable && (returnedAllHistory || shortHistoryPage)
	}
	masteryMap := map[int64]ChampionMastery{}
	masteryCapability := EndpointCapability{Name: "champion-mastery", Path: "/lol-champion-mastery/v1/{player}/champion-mastery"}
	if remoteServer {
		masteryCapability.State = capabilityUnsupported
		masteryCapability.Detail = "所选服务器暂未提供可核验的跨服熟练度接口"
	} else {
		masteryMap, masteryCapability = NewChampionMasteryAPI(client).All(playerRef)
		masteryCapability.Path = "/lol-champion-mastery/v1/{player}/champion-mastery"
	}
	capabilities = append(capabilities, masteryCapability)
	masteries := normalizeMasteries(masteryMap, names, 6)
	response.Ranks = ranks
	response.Masteries = masteries
	response.Capabilities = capabilities
	response.Overall = aggregateMatches(matches, playerRef, nil)
	recentWindowAfter := time.Now().Add(-recentWindowDays * 24 * time.Hour).UnixMilli()
	response.SevenDayRank = aggregateMatches(windowMatches, playerRef, func(match gameplayMatch) bool {
		return match.CreatedAt >= recentWindowAfter && (match.QueueID == 420 || match.QueueID == 440)
	})
	windowReachedCutoff := len(windowMatches) > 0 && windowMatches[len(windowMatches)-1].CreatedAt < recentWindowAfter
	if response.SevenDayRank.Games > 0 && !windowExhausted && !windowReachedCutoff {
		response.SevenDayRank.Sampled = true
	}
	response.ChampionStats = championStats(matches, playerRef, names)
	response.Positions = positionStats(matches, playerRef)
	response.ActivityHours = activityHours(windowMatches)
	// “最近一起玩”需要每场的完整参与者名单，因此基于已读取的详情页
	// 战绩统计，并限定在最近 30 天内。
	response.RecentPlayers = recentPlayers(matches, playerRef, recentWindowAfter)
	a.publicizeOverviewReferences(&response)
	return response
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

// loadDetailedMatches 读取带全部参与者的战绩列表。国服客户端的
// match-details 接口新版本只返回本人数据，因此优先走 SGP 网关一次拿到
// 整页完整对局；SGP 不可用时回退到旧的“列表 + 逐场详情”方式。
// loadDetailedMatches 读取带完整参与者的战绩页，并返回分页信息。
// SGP 会跳过缺少 json 的对局，导致一页有效场次可能少于请求数；分页的
// Count 因此使用服务器侧实际消费的条目数，前端据此推进下一页偏移量，
// 避免“一页不满 20 场就误判没有更多”或偏移错位导致的重复。
func (a *app) loadDetailedMatches(ctx context.Context, client *LCUClient, reference gameplayReference, playerRef string, isCurrent bool, begIndex, count int, names map[int64]string, queueLabels map[int64]string) ([]gameplayMatch, []EndpointCapability, gameplayPagination) {
	sgpDetail := ""
	serverID := reference.ServerID
	if serverID == "" {
		serverID, _, _ = a.sgp.available(client)
	}
	if serverID != "" && validPlayerReference(playerRef) {
		if infos, consumed, more, err := a.sgp.matchHistoryOn(ctx, client, serverID, playerRef, begIndex, count, false); err == nil {
			matches := make([]gameplayMatch, 0, len(infos))
			for _, info := range infos {
				matches = append(matches, convertRiotMatchInfo(info, playerRef, names, queueLabels, "", serverID))
			}
			capabilities := []EndpointCapability{
				{Name: "match-history", Path: "sgp: /match-history-query/v1/products/lol/{player}/SUMMARY", State: capabilityAvailable, Count: len(matches)},
				{Name: "match-details", Path: "sgp: 同一请求返回全部参与者", State: capabilityAvailable, Count: len(matches), Detail: "通过官方 SGP 网关读取完整对局数据"},
			}
			return matches, capabilities, gameplayPagination{BegIndex: begIndex, Count: consumed, HasMore: more}
		} else {
			if isRemoteTencentServer(client, serverID) {
				detail := "所选服务器的 SGP 战绩暂时无法读取"
				capabilities := []EndpointCapability{
					{Name: "match-history", Path: "sgp: /match-history-query/v1/products/lol/{player}/SUMMARY", State: capabilityFailed, Detail: detail},
					{Name: "match-details", Path: "sgp: 同一请求返回全部参与者", State: capabilityFailed, Detail: detail},
				}
				return nil, capabilities, gameplayPagination{BegIndex: begIndex}
			}
			sgpDetail = "SGP 网关读取失败，已回退客户端接口（该接口可能只返回本人数据）：" + err.Error()
		}
	} else if region, platform := client.platformInfo(); strings.EqualFold(region, "TENCENT") {
		if platform == "" {
			sgpDetail = "SGP 不可用：未能从客户端识别所在子服务器（rso_platform_id）"
		} else if _, known := tencentSGPServers[strings.ToUpper(platform)]; !known {
			sgpDetail = "SGP 不可用：未收录的国服子服务器 " + platform
		} else {
			sgpDetail = "SGP 暂时不可用（近期请求失败，稍后自动重试）"
		}
	}
	rawGames, capabilities, total := loadGameplayHistory(client, playerRef, isCurrent, begIndex, count, true)
	matches := make([]gameplayMatch, 0, len(rawGames))
	for _, game := range rawGames {
		matches = append(matches, normalizeGameplayMatch(game, reference, names, queueLabels))
	}
	// 把 SGP 不可用的原因写进能力状态：设置页与错误排查都能看到，
	// 不至于只表现为“战绩里只有自己一个人”。
	if sgpDetail != "" {
		capabilities = append(capabilities, EndpointCapability{Name: "match-details", Path: "sgp: /match-history-query", State: capabilityFailed, Detail: sgpDetail})
	}
	pagination := gameplayPagination{
		BegIndex: begIndex, Count: len(rawGames), Total: total,
		HasMore: total > begIndex+len(rawGames) || total == 0 && len(rawGames) == count,
	}
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
	if value := strings.TrimSpace(player.PUUID); value != "" {
		return value
	}
	if strings.EqualFold(strings.TrimSpace(player.NameVisibilityType), "HIDDEN") {
		return deobfuscateHiddenPlayerReference(player.ObfuscatedPUUID)
	}
	return ""
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
	return Summoner{SummonerID: reference.SummonerID, PUUID: reference.PlayerRef, DisplayName: reference.DisplayName, GameName: reference.GameName, TagLine: reference.TagLine, ProfileIconID: reference.ProfileIconID, SummonerLevel: reference.SummonerLevel}
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

func loadGameplaySummoner(client *LCUClient, reference gameplayReference) (Summoner, EndpointCapability) {
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
	a.mu.Lock()
	if a.gameplayRefs == nil {
		a.gameplayRefs = make(map[string]string)
	}
	if a.gameplayRefDetails == nil {
		a.gameplayRefDetails = make(map[string]gameplayReference)
	}
	a.gameplayRefs[publicRef] = playerRef
	a.gameplayRefDetails[publicRef] = mergeGameplayReferences(reference, a.gameplayRefDetails[publicRef])
	a.mu.Unlock()
	return publicRef
}

func (a *app) resolveGameplayReference(publicRef string) (string, bool) {
	a.mu.RLock()
	playerRef, ok := a.gameplayRefs[strings.TrimSpace(publicRef)]
	a.mu.RUnlock()
	return playerRef, ok && validPlayerReference(playerRef)
}

func (a *app) resolveGameplayReferenceDetails(publicRef string) (gameplayReference, bool) {
	publicRef = strings.TrimSpace(publicRef)
	a.mu.RLock()
	playerRef, ok := a.gameplayRefs[publicRef]
	reference := a.gameplayRefDetails[publicRef]
	a.mu.RUnlock()
	if !ok || !validPlayerReference(playerRef) {
		return gameplayReference{}, false
	}
	if reference.PlayerRef == "" {
		reference.PlayerRef = playerRef
	}
	return normalizeGameplayReference(reference), true
}

func (a *app) publicizeOverviewReferences(response *gameplayOverview) {
	playerReference := mergeGameplayReferences(response.Player.reference, gameplayReference{PlayerRef: response.Player.PlayerRef, DisplayName: response.Player.DisplayName, GameName: response.Player.GameName, TagLine: response.Player.TagLine, ProfileIconID: response.Player.ProfileIconID, SummonerLevel: response.Player.SummonerLevel, Region: response.Player.Region, ServerID: response.Player.ServerID})
	response.Player.PlayerRef = a.registerGameplayReferenceDetails(playerReference)
	for matchIndex := range response.Matches {
		for playerIndex := range response.Matches[matchIndex].Participants {
			participant := &response.Matches[matchIndex].Participants[playerIndex]
			participant.PlayerRef = a.registerGameplayReferenceDetails(mergeGameplayReferences(participant.reference, gameplayReference{PlayerRef: participant.PlayerRef, DisplayName: participant.DisplayName, GameName: participant.GameName, TagLine: participant.TagLine, ProfileIconID: participant.ProfileIconID}))
		}
	}
	for index := range response.RecentPlayers {
		player := &response.RecentPlayers[index]
		player.PlayerRef = a.registerGameplayReferenceDetails(mergeGameplayReferences(player.reference, gameplayReference{PlayerRef: player.PlayerRef, DisplayName: player.DisplayName, ProfileIconID: player.ProfileIconID}))
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

// loadRanksWithFallback 优先通过 SGP 读取排位数据：新版国服客户端的
// ranked-stats 接口不再返回负场（losses 恒为 0，胜率显示成 100%），
// 而 SGP 的 leagues-ledge 仍然提供完整的当季胜负场次。SGP 不可用时
// 回退本机客户端数据，此时胜负口径以客户端返回为准。
func (a *app) loadRanksWithFallback(ctx context.Context, client *LCUClient, playerRef string, isCurrent bool, serverID string) ([]gameplayRank, EndpointCapability) {
	if validPlayerReference(playerRef) {
		if serverID == "" {
			serverID, _, _ = a.sgp.available(client)
		}
		if serverID != "" {
			if isRemoteTencentServer(client, serverID) {
				return nil, EndpointCapability{Name: "ranked-stats", Path: "sgp: /leagues-ledge/v2/rankedStats", State: capabilityUnsupported, Detail: "跨服暂不支持排位"}
			}
			if queues, err := a.sgp.rankedStatsOn(ctx, client, serverID, playerRef); err == nil && len(queues) > 0 {
				ranks := make([]gameplayRank, 0, 2)
				for _, entry := range queues {
					if entry.QueueType != "RANKED_SOLO_5x5" && entry.QueueType != "RANKED_FLEX_SR" {
						continue
					}
					total := entry.Wins + entry.Losses
					winRate := 0
					if total > 0 {
						winRate = int(math.Round(float64(entry.Wins) * 100 / float64(total)))
					}
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
				if len(ranks) > 0 {
					sort.SliceStable(ranks, func(i, j int) bool {
						return ranks[i].QueueType == "RANKED_SOLO_5x5" && ranks[j].QueueType != "RANKED_SOLO_5x5"
					})
					return ranks, EndpointCapability{Name: "ranked-stats", Path: "sgp: /leagues-ledge/v2/rankedStats", State: capabilityAvailable, Count: len(ranks)}
				}
			}
		}
	}
	return loadGameplayRanks(client, playerRef, isCurrent)
}

func loadGameplayRanks(client *LCUClient, playerRef string, current bool) ([]gameplayRank, EndpointCapability) {
	path := "/lol-ranked/v1/ranked-stats/" + url.PathEscape(playerRef)
	publicPath := "/lol-ranked/v1/ranked-stats/{player}"
	if current {
		path = "/lol-ranked/v1/current-ranked-stats"
		publicPath = path
	}
	capability := EndpointCapability{Name: "ranked-stats", Path: publicPath}
	var payload lcuRankedStats
	if err := client.GetJSON(path, &payload); err != nil {
		return nil, gameplayCapabilityError(capability.Name, capability.Path, err)
	}
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
	for _, entry := range entries {
		if entry.QueueType != "RANKED_SOLO_5x5" && entry.QueueType != "RANKED_FLEX_SR" {
			continue
		}
		total := entry.Wins + entry.Losses
		winRate := 0
		if total > 0 {
			winRate = int(math.Round(float64(entry.Wins) * 100 / float64(total)))
		}
		label := "单排/双排"
		if entry.QueueType == "RANKED_FLEX_SR" {
			label = "灵活组排"
		}
		ranks = append(ranks, gameplayRank{QueueType: entry.QueueType, QueueLabel: label, Tier: entry.Tier, Division: entry.Division, LeaguePoints: entry.LeaguePoints, Wins: entry.Wins, Losses: entry.Losses, WinRate: winRate, Provisional: entry.IsProvisional})
	}
	sort.SliceStable(ranks, func(i, j int) bool {
		return ranks[i].QueueType == "RANKED_SOLO_5x5" && ranks[j].QueueType != "RANKED_SOLO_5x5"
	})
	capability.State = capabilityAvailable
	capability.Count = len(ranks)
	return ranks, capability
}

func loadGameplayHistory(client *LCUClient, playerRef string, current bool, begIndex, count int, details bool) ([]lcuGame, []EndpointCapability, int) {
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
	if err := client.GetJSON(path, &payload); err != nil {
		return nil, []EndpointCapability{gameplayCapabilityError("match-history", publicPath, err)}, 0
	}
	games := append([]lcuGame(nil), payload.Games.Games...)
	if len(games) > count {
		games = games[:count]
	}
	capabilities := []EndpointCapability{{Name: "match-history", Path: publicPath, State: capabilityAvailable, Count: len(games)}}
	if !details || len(games) == 0 {
		return games, capabilities, payload.Games.GameCount
	}
	var wait sync.WaitGroup
	var countMu sync.Mutex
	loaded := 0
	semaphore := make(chan struct{}, 4)
	for index := range games {
		participantCount := len(games[index].Participants)
		identityCount := len(games[index].ParticipantIdentities)
		complete := participantCount > 0 && participantCount == identityCount && len(games[index].Teams) > 0
		if games[index].GameID <= 0 || complete {
			countMu.Lock()
			loaded++
			countMu.Unlock()
			continue
		}
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			var detail lcuGame
			if err := client.GetJSON(fmt.Sprintf("/lol-match-history/v1/games/%d", games[index].GameID), &detail); err != nil {
				return
			}
			games[index] = detail
			countMu.Lock()
			loaded++
			countMu.Unlock()
		}(index)
	}
	wait.Wait()
	detailCapability := EndpointCapability{Name: "match-details", Path: "/lol-match-history/v1/games/{gameId}", Count: loaded}
	if loaded == len(games) {
		detailCapability.State = capabilityAvailable
	} else if loaded > 0 {
		detailCapability.State = capabilityFailed
		detailCapability.Detail = "部分对局详情不可用，已保留可核验的摘要"
	} else {
		detailCapability.State = capabilityUnsupported
		detailCapability.Detail = "当前客户端未返回可展开的对局详情"
	}
	capabilities = append(capabilities, detailCapability)
	return games, capabilities, payload.Games.GameCount
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
	match := gameplayMatch{GameID: game.GameID, CreatedAt: createdAt, Duration: game.GameDuration, QueueID: game.QueueID, QueueLabel: queueLabel(game.QueueID, game.GameMode, queueLabels), ModeGroup: queueModeGroup(game.QueueID), GameMode: game.GameMode, MapID: game.MapID}
	for _, raw := range game.Participants {
		identity := identities[raw.ParticipantID]
		visiblePlayerRef := strings.TrimSpace(identity.Player.PUUID)
		identityHidden := strings.TrimSpace(identity.Player.GameName) == "" && strings.TrimSpace(identity.Player.SummonerName) == ""
		if visiblePlayerRef == "" && identityHidden {
			visiblePlayerRef = deobfuscateHiddenPlayerReference(identity.Player.ObfuscatedPUUID)
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
		perks := compactPositiveInt64(raw.Stats.Perk0, raw.Stats.Perk1, raw.Stats.Perk2, raw.Stats.Perk3, raw.Stats.Perk4, raw.Stats.Perk5)
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
			Win: raw.Stats.Win, Hidden: hidden, MultiKill: raw.Stats.LargestMultiKill,
			SubteamID: raw.Stats.PlayerSubteamID, Placement: raw.Stats.SubteamPlacement, reference: reference,
		}
		match.Participants = append(match.Participants, participant)
		if gameplayReferencesMatch(reference, subject) {
			match.SubjectParticipantID = raw.ParticipantID
			if raw.Stats.Win {
				match.Result = "win"
			} else {
				match.Result = "loss"
			}
		}
	}
	if match.SubjectParticipantID == 0 && len(match.Participants) == 1 {
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

func championStats(matches []gameplayMatch, playerRef string, names map[int64]string) []gameplayChampionStat {
	type counter struct {
		games, wins, kills, deaths, assists, cs int
		duration                                int64
	}
	counters := make(map[int64]*counter)
	for _, match := range matches {
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
	counts := map[string]int{}
	total := 0
	for _, match := range matches {
		participant, ok := matchSubject(match, playerRef)
		if !ok || participant.Position == "" {
			continue
		}
		counts[participant.Position]++
		total++
	}
	labels := map[string]string{"top": "上路", "jungle": "打野", "middle": "中路", "bottom": "下路", "utility": "辅助", "other": "其他"}
	order := []string{"top", "jungle", "middle", "bottom", "utility", "other"}
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
			if participant.PlayerRef == "" || participant.PlayerRef == subjectRef {
				continue
			}
			item := byRef[participant.PlayerRef]
			if item == nil {
				item = &gameplayRecentPlayer{PlayerRef: participant.PlayerRef, DisplayName: participant.DisplayName, ProfileIconID: participant.ProfileIconID, Hidden: participant.Hidden, reference: participant.reference}
				byRef[participant.PlayerRef] = item
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
	Phase                string                    `json:"phase"`
	Available            bool                      `json:"available"`
	GameID               int64                     `json:"gameId,omitempty"`
	QueueID              int64                     `json:"queueId,omitempty"`
	QueueLabel           string                    `json:"queueLabel,omitempty"`
	GameMode             string                    `json:"gameMode,omitempty"`
	MapID                int64                     `json:"mapId,omitempty"`
	Players              []gameplayLivePlayer      `json:"players"`
	ClientRecommendation *gameplayRecommendation   `json:"clientRecommendation,omitempty"`
	ChampionAbilities    []gameplayChampionAbility `json:"championAbilities,omitempty"`
	Capabilities         []EndpointCapability      `json:"capabilities"`
}

type gameplayRecommendationsResponse struct {
	ChampionID      int64                        `json:"championId"`
	Position        string                       `json:"position"`
	Recommendations gameplayRecommendationBundle `json:"recommendations"`
}

type gameplayRecommendationBundle struct {
	Hero  gameplayRecommendationHero  `json:"hero"`
	Runes gameplayRecommendationRunes `json:"runes"`
	Build gameplayRecommendationBuild `json:"build"`
}

type gameplayRecommendationHero struct {
	WinRate       float64                         `json:"winRate,omitempty"`
	PickRate      float64                         `json:"pickRate,omitempty"`
	BanRate       float64                         `json:"banRate,omitempty"`
	StrongAgainst []gameplayRecommendationMatchup `json:"strongAgainst,omitempty"`
	WeakAgainst   []gameplayRecommendationMatchup `json:"weakAgainst,omitempty"`
}

type gameplayRecommendationMatchup struct {
	ChampionID   int64  `json:"championId"`
	ChampionName string `json:"championName"`
}

type gameplayRecommendationRunes struct {
	OPGG        []gameplayRecommendationRune `json:"opgg"`
	Specialists []gameplayRecommendationRune `json:"specialists"`
	Pros        []gameplayRecommendationRune `json:"pros"`
}

type gameplayRecommendationRune struct {
	Key             string                      `json:"key"`
	Title           string                      `json:"title"`
	ChampionID      int64                       `json:"championId"`
	ChampionName    string                      `json:"championName,omitempty"`
	PrimaryStyleID  int64                       `json:"primaryStyleId"`
	SubStyleID      int64                       `json:"subStyleId"`
	SelectedPerkIDs []int64                     `json:"selectedPerkIds"`
	StatModIDs      []int64                     `json:"statModIds,omitempty"`
	Stats           gameplayRecommendationStats `json:"stats"`
	PlayerName      string                      `json:"playerName,omitempty"`
	TagLine         string                      `json:"tagLine,omitempty"`
	Tier            string                      `json:"tier,omitempty"`
	Division        string                      `json:"division,omitempty"`
	LeaguePoints    string                      `json:"leaguePoints,omitempty"`
	ChampionGames   int                         `json:"championGames,omitempty"`
	PlayedAt        int64                       `json:"playedAt,omitempty"`
	Result          string                      `json:"result,omitempty"`
	Region          string                      `json:"region,omitempty"`
}

type gameplayRecommendationStats struct {
	PickRate float64 `json:"pickRate,omitempty"`
	WinRate  float64 `json:"winRate,omitempty"`
	Games    int     `json:"games,omitempty"`
}

type gameplayRecommendationBuild struct {
	Position       string                         `json:"position"`
	SkillPriority  []string                       `json:"skillPriority,omitempty"`
	SkillOrder     []string                       `json:"skillOrder,omitempty"`
	SkillStats     gameplayRecommendationStats    `json:"skillStats"`
	SpellOptions   []gameplayRecommendationOption `json:"spellOptions"`
	StarterOptions []gameplayRecommendationOption `json:"starterOptions"`
	BootOptions    []gameplayRecommendationOption `json:"bootOptions"`
	ItemRoutes     []gameplayRecommendationOption `json:"itemRoutes"`
}

type gameplayRecommendationOption struct {
	IDs   []int64                     `json:"ids"`
	Stats gameplayRecommendationStats `json:"stats"`
}

func (a *app) handleGameplayRecommendations(w http.ResponseWriter, r *http.Request) {
	championID, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("championId")), 10, 64)
	if err != nil || championID <= 0 || championID > 10000 {
		http.Error(w, "推荐英雄无效", http.StatusBadRequest)
		return
	}
	position, err := normalizeOPGGPosition(r.URL.Query().Get("position"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	provider := a.championDataProvider()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	metadata, err := provider.championMetadataByID(ctx, int(championID))
	if err != nil {
		http.Error(w, "暂时无法识别当前英雄", http.StatusNotFound)
		return
	}
	detail, err := provider.loadDetail(ctx, "ranked", metadata.Slug, position, championCounterFallbackTier)
	if err != nil {
		http.Error(w, "OPGG 推荐数据暂不可用", http.StatusBadGateway)
		return
	}
	respondJSON(w, gameplayRecommendationsResponse{ChampionID: championID, Position: position, Recommendations: gameplayRecommendationsFromChampionDetail(championID, position, detail)})
}

func normalizeOPGGPosition(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "other", "middle", "mid":
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

func gameplayRecommendationsFromChampionDetail(championID int64, position string, detail championDetailResponse) gameplayRecommendationBundle {
	result := gameplayRecommendationBundle{
		Hero:  gameplayRecommendationHero{WinRate: detail.Stats.WinRate, PickRate: detail.Stats.PickRate, BanRate: detail.Stats.BanRate},
		Runes: gameplayRecommendationRunes{OPGG: []gameplayRecommendationRune{}, Specialists: []gameplayRecommendationRune{}, Pros: []gameplayRecommendationRune{}},
		Build: gameplayRecommendationBuild{Position: position, SpellOptions: []gameplayRecommendationOption{}, StarterOptions: []gameplayRecommendationOption{}, BootOptions: []gameplayRecommendationOption{}, ItemRoutes: []gameplayRecommendationOption{}},
	}
	for _, row := range detail.Counters.StrongAgainst {
		result.Hero.StrongAgainst = append(result.Hero.StrongAgainst, gameplayRecommendationMatchup{ChampionID: int64(row.ChampionID), ChampionName: row.Name})
	}
	for _, row := range detail.Counters.WeakAgainst {
		result.Hero.WeakAgainst = append(result.Hero.WeakAgainst, gameplayRecommendationMatchup{ChampionID: int64(row.ChampionID), ChampionName: row.Name})
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
	result.Build.ItemRoutes = recommendationOptions(detail.Build.CoreItems)
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
			result = append(result, gameplayRecommendationOption{IDs: ids, Stats: recommendationStats(row.PickRate, row.WinRate, row.Games)})
		}
	}
	return result
}

func recommendationStats(pickRate, winRate float64, games int) gameplayRecommendationStats {
	return gameplayRecommendationStats{PickRate: pickRate, WinRate: winRate, Games: games}
}

type gameplayLivePlayer struct {
	gameplayPlayer
	TeamID       int64                `json:"teamId"`
	ChampionID   int64                `json:"championId,omitempty"`
	ChampionName string               `json:"championName,omitempty"`
	Position     string               `json:"position,omitempty"`
	Rank         *gameplayRank        `json:"rank,omitempty"`
	ModeStats    gameplayAggregate    `json:"modeStats"`
	RecentGames  []gameplayRecentGame `json:"recentGames,omitempty"`
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
	ChampionID           int64  `json:"championId"`
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
}

type lcuChampSelectSession struct {
	MyTeam    []lcuChampSelectPlayer `json:"myTeam"`
	TheirTeam []lcuChampSelectPlayer `json:"theirTeam"`
}

type lcuChampSelectPlayer struct {
	AssignedPosition     string `json:"assignedPosition"`
	ChampionID           int64  `json:"championId"`
	ChampionPickIntent   int64  `json:"championPickIntent"`
	GameName             string `json:"gameName"`
	TagLine              string `json:"tagLine"`
	PUUID                string `json:"puuid"`
	ObfuscatedPUUID      string `json:"obfuscatedPuuid"`
	SummonerID           int64  `json:"summonerId"`
	ObfuscatedSummonerID int64  `json:"obfuscatedSummonerId"`
	NameVisibilityType   string `json:"nameVisibilityType"`
}

func (a *app) handleGameplayPhase(w http.ResponseWriter, _ *http.Request) {
	client, _, err := a.gameplayClient()
	if err != nil {
		respondJSON(w, map[string]any{"phase": "None", "connected": false})
		return
	}
	var phase string
	if err := client.GetJSON("/lol-gameflow/v1/gameflow-phase", &phase); err != nil {
		respondJSON(w, map[string]any{"phase": "None", "connected": true})
		return
	}
	respondJSON(w, map[string]any{"phase": phase, "connected": true})
}

func (a *app) handleGameplayLive(w http.ResponseWriter, r *http.Request) {
	client, current, err := a.gameplayClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	var phase string
	if err := client.GetJSON("/lol-gameflow/v1/gameflow-phase", &phase); err != nil {
		respondJSON(w, gameplayLiveResponse{Phase: "Unavailable", Capabilities: []EndpointCapability{gameplayCapabilityError("gameflow", "/lol-gameflow/v1/gameflow-phase", err)}})
		return
	}
	response := gameplayLiveResponse{Phase: phase, Capabilities: []EndpointCapability{{Name: "gameflow", Path: "/lol-gameflow/v1/gameflow-phase", State: capabilityAvailable, Count: 1}}}
	if phase != "ChampSelect" && phase != "InProgress" && phase != "GameStart" && phase != "Reconnect" {
		respondJSON(w, response)
		return
	}
	var session lcuGameflowSession
	sessionErr := client.GetJSON("/lol-gameflow/v1/session", &session)
	if sessionErr != nil {
		// 国服部分版本在英雄选择阶段不返回 gameflow session，
		// 继续尝试 champ-select 会话，不要直接放弃整页数据。
		response.Capabilities = append(response.Capabilities, gameplayCapabilityError("gameflow-session", "/lol-gameflow/v1/session", sessionErr))
		if phase != "ChampSelect" {
			respondJSON(w, response)
			return
		}
	}
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
	rawPlayers := make([]struct {
		player lcuLivePlayer
		team   int64
	}, 0, 10)
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
	if phase == "ChampSelect" {
		var champSelect lcuChampSelectSession
		if err := client.GetJSON("/lol-champ-select/v1/session", &champSelect); err == nil {
			rawPlayers = mergeChampSelectPlayers(rawPlayers, champSelect)
			response.Capabilities = append(response.Capabilities, EndpointCapability{Name: "champ-select", Path: "/lol-champ-select/v1/session", State: capabilityAvailable, Count: len(champSelect.MyTeam) + len(champSelect.TheirTeam)})
		} else {
			response.Capabilities = append(response.Capabilities, gameplayCapabilityError("champ-select", "/lol-champ-select/v1/session", err))
		}
	}
	if sessionErr != nil && len(rawPlayers) == 0 {
		respondJSON(w, response)
		return
	}
	response.Available = true
	names := a.overviewChampionNames(r.Context())
	response.Players = make([]gameplayLivePlayer, len(rawPlayers))
	var wait sync.WaitGroup
	semaphore := make(chan struct{}, 4)
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
				ProfileIconID: raw.player.ProfileIconID,
			})
			summoner := summonerFromGameplayReference(reference)
			if loaded, capability := loadGameplaySummoner(client, reference); capability.State == capabilityAvailable {
				summoner = mergeSummonerIdentity(loaded, summoner)
				reference = mergeGameplayReferences(gameplayReferenceFromSummoner(summoner), reference)
			}
			playerRef := reference.PlayerRef
			validRef := validPlayerReference(playerRef)
			isCurrent := gameplayReferenceContains(reference, current.PUUID)
			var ranks []gameplayRank
			if validRef {
				ranks, _ = loadGameplayRanks(client, playerRef, isCurrent)
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
			var matches []gameplayMatch
			if validRef {
				matches = a.livePlayerMatches(r.Context(), client, reference, playerRef, isCurrent, names)
			}
			modeStats := aggregateMatches(matches, playerRef, func(match gameplayMatch) bool { return response.QueueID == 0 || match.QueueID == response.QueueID })
			hidden := strings.EqualFold(raw.player.NameVisibilityType, "HIDDEN") || (strings.TrimSpace(summoner.GameName) == "" && strings.TrimSpace(summoner.DisplayName) == "")
			response.Players[index] = gameplayLivePlayer{gameplayPlayer: gameplayPlayer{PlayerRef: playerRef, DisplayName: gameplayDisplayName(summoner), GameName: summoner.GameName, TagLine: summoner.TagLine, ProfileIconID: summoner.ProfileIconID, SummonerLevel: summoner.SummonerLevel, Hidden: hidden, IsCurrent: isCurrent, reference: reference}, TeamID: raw.team, ChampionID: raw.player.ChampionID, ChampionName: championName(names, raw.player.ChampionID), Position: normalizePosition(raw.player.SelectedPosition, raw.player.SelectedRole), Rank: rank, ModeStats: modeStats, RecentGames: recentGamesFromMatches(matches, playerRef, 8, response.QueueID)}
		}(index)
	}
	wait.Wait()
	for index := range response.Players {
		player := &response.Players[index]
		player.PlayerRef = a.registerGameplayReferenceDetails(player.reference)
	}
	response.Capabilities = append(response.Capabilities, EndpointCapability{Name: "live-player-analysis", Path: "本机召唤师、排位与战绩接口", State: capabilityAvailable, Count: len(response.Players)})
	for _, player := range response.Players {
		if !player.IsCurrent || player.ChampionID <= 0 {
			continue
		}
		if recommendation, err := loadClientRecommendation(client, player.ChampionID, player.Position, response.MapID); err == nil {
			response.ClientRecommendation = recommendation
			response.Capabilities = append(response.Capabilities, EndpointCapability{Name: "client-rune-recommendation", Path: "/lol-perks/v1/recommended-pages/...", State: capabilityAvailable, Count: 1})
		} else {
			response.Capabilities = append(response.Capabilities, gameplayCapabilityError("client-rune-recommendation", "/lol-perks/v1/recommended-pages/...", err))
		}
		if abilities, err := loadChampionAbilities(client, player.ChampionID); err == nil {
			response.ChampionAbilities = abilities
			response.Capabilities = append(response.Capabilities, EndpointCapability{Name: "champion-abilities", Path: "/lol-game-data/assets/v1/champions/{id}.json", State: capabilityAvailable, Count: len(abilities)})
		} else {
			response.Capabilities = append(response.Capabilities, gameplayCapabilityError("champion-abilities", "/lol-game-data/assets/v1/champions/{id}.json", err))
		}
		break
	}
	respondJSON(w, response)
}

// livePlayerMatches 读取对局页单名玩家的最近战绩：先用本机客户端的
// 列表接口（轻量），拿不到数据时改走 SGP 网关（带短期缓存，避免自动
// 刷新反复请求）。
func (a *app) livePlayerMatches(ctx context.Context, client *LCUClient, reference gameplayReference, playerRef string, isCurrent bool, names map[int64]string) []gameplayMatch {
	history, capabilities, _ := loadGameplayHistory(client, playerRef, isCurrent, 0, 10, false)
	matches := make([]gameplayMatch, 0, len(history))
	for _, game := range history {
		matches = append(matches, normalizeGameplayMatch(game, reference, names, nil))
	}
	listFailed := len(capabilities) > 0 && capabilities[0].State != capabilityAvailable
	if len(matches) == 0 && (listFailed || len(history) == 0) {
		if _, _, ok := a.sgp.available(client); ok {
			if infos, _, _, err := a.sgp.matchHistory(ctx, client, playerRef, 0, 10, true); err == nil {
				for _, info := range infos {
					matches = append(matches, convertRiotMatchInfo(info, playerRef, names, nil, "", reference.ServerID))
				}
			}
		}
	}
	return matches
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

func mergeChampSelectPlayers(existing []struct {
	player lcuLivePlayer
	team   int64
}, session lcuChampSelectSession) []struct {
	player lcuLivePlayer
	team   int64
} {
	result := append([]struct {
		player lcuLivePlayer
		team   int64
	}(nil), existing...)
	merge := func(source []lcuChampSelectPlayer, team int64) {
		for _, selected := range source {
			visiblePlayerRef := visibleChampSelectPlayerReference(selected)
			// 尚未锁定时使用预选英雄，避免整页显示“尚未选择英雄”。
			selectedChampionID := selected.ChampionID
			if selectedChampionID <= 0 {
				selectedChampionID = selected.ChampionPickIntent
			}
			selectedReference := normalizeGameplayReference(gameplayReference{PlayerRef: visiblePlayerRef, AlternatePlayerRef: selected.ObfuscatedPUUID, SummonerID: selected.SummonerID, AlternateSummonerID: selected.ObfuscatedSummonerID, GameName: selected.GameName, DisplayName: selected.GameName, TagLine: selected.TagLine})
			index := -1
			for candidate := range result {
				candidateReference := gameplayReference{PlayerRef: result[candidate].player.PUUID, AlternatePlayerRef: result[candidate].player.ObfuscatedPUUID, SummonerID: result[candidate].player.SummonerID, AlternateSummonerID: result[candidate].player.ObfuscatedSummonerID}
				if gameplayReferencesMatch(selectedReference, candidateReference) {
					index = candidate
					break
				}
			}
			if index < 0 {
				result = append(result, struct {
					player lcuLivePlayer
					team   int64
				}{player: lcuLivePlayer{PUUID: visiblePlayerRef, ObfuscatedPUUID: selected.ObfuscatedPUUID, SummonerID: selected.SummonerID, ObfuscatedSummonerID: selected.ObfuscatedSummonerID, ChampionID: selectedChampionID, SummonerName: selected.GameName, GameName: selected.GameName, TagLine: selected.TagLine, NameVisibilityType: selected.NameVisibilityType, SelectedPosition: selected.AssignedPosition}, team: team})
				continue
			}
			if selectedChampionID > 0 {
				result[index].player.ChampionID = selectedChampionID
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
			if selected.AssignedPosition != "" {
				result[index].player.SelectedPosition = selected.AssignedPosition
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
		position = "middle"
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

const (
	runePagePrefix       = "[DL] "
	runePageRecycleLimit = 5
	runePageLimitMessage = "客户端符文页已达上限，无法新增推荐符文页；请删除一个自定义符文页后重试"
)

var errRunePageLimit = errors.New(runePageLimitMessage)

func (a *app) handleGameplayRuneApply(w http.ResponseWriter, r *http.Request) {
	var request gameplayRuneApplyRequest
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "符文配置无效", http.StatusBadRequest)
		return
	}
	if err := validateRuneApplyRequest(request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	client, _, err := a.gameplayClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	var phase string
	if err := client.GetJSON("/lol-gameflow/v1/gameflow-phase", &phase); err != nil || phase != "ChampSelect" {
		http.Error(w, "只能在英雄选择阶段应用符文", http.StatusConflict)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	pageID, err := applyRunePage(ctx, client, request)
	if err != nil {
		if errors.Is(err, errRunePageLimit) {
			http.Error(w, runePageLimitMessage, http.StatusConflict)
			return
		}
		http.Error(w, "符文应用失败："+safeGameplayError(err), http.StatusConflict)
		return
	}
	respondJSON(w, map[string]any{"applied": true, "pageId": pageID, "name": runePageName(request.ChampionName, request.Source)})
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
	var added struct {
		ID int64 `json:"id"`
	}
	if err := client.RequestJSON(ctx, http.MethodPost, "/lol-perks/v1/pages/", map[string]any{"name": name, "isEditable": true, "primaryStyleId": strconv.FormatInt(request.PrimaryStyleID, 10)}, &added); err != nil {
		return 0, err
	}
	pageID := added.ID
	if pageID <= 0 {
		return 0, errors.New("客户端没有返回符文页编号")
	}
	body := map[string]any{"id": pageID, "isRecommendationOverride": false, "isTemporary": false, "name": name, "primaryStyleId": request.PrimaryStyleID, "selectedPerkIds": request.SelectedPerkIDs, "subStyleId": request.SubStyleID, "recommendationChampionId": request.ChampionID}
	if err := client.RequestJSON(ctx, http.MethodPut, fmt.Sprintf("/lol-perks/v1/pages/%d", pageID), body, nil); err != nil {
		return 0, err
	}
	if err := client.RequestJSON(ctx, http.MethodPut, "/lol-perks/v1/currentpage", pageID, nil); err != nil {
		return 0, err
	}
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

type gameplayItemSetApplyRequest struct {
	Title      string                        `json:"title"`
	ChampionID int64                         `json:"championId"`
	MapID      int64                         `json:"mapId"`
	Position   string                        `json:"position"`
	Blocks     []gameplayItemSetBlockRequest `json:"blocks"`
}

type gameplayItemSetBlockRequest struct {
	Type  string                       `json:"type"`
	Items []gameplayItemSetItemRequest `json:"items"`
}

type gameplayItemSetItemRequest struct {
	ID    int64 `json:"id"`
	Count int   `json:"count"`
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
	var request gameplayItemSetApplyRequest
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "装备方案无效", http.StatusBadRequest)
		return
	}
	position, err := normalizeGameplayItemSetPosition(request.Position)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	request.Position = position
	if err := validateGameplayItemSetRequest(request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
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
	if err := validateGameplayItemSetContext(ctx, client, current, request.ChampionID); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	a.itemSetMu.Lock()
	defer a.itemSetMu.Unlock()
	uid, title, err := applyGameplayItemSet(ctx, client, current, request, func() error {
		latestClient, latestCurrent, latestErr := a.gameplayClient()
		if latestErr != nil || latestClient != client || gameplaySummonerChanged(current, latestCurrent) || (current.AccountID > 0 && latestCurrent.AccountID > 0 && current.AccountID != latestCurrent.AccountID) {
			return errors.New("客户端账号已发生变化，已停止应用装备方案")
		}
		return validateGameplayItemSetContext(ctx, client, latestCurrent, request.ChampionID)
	})
	if err != nil {
		http.Error(w, "装备方案应用失败："+safeGameplayError(err), http.StatusConflict)
		return
	}
	respondJSON(w, map[string]any{"applied": true, "verified": true, "uid": uid, "title": title})
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
	var phase string
	if err := client.RequestJSON(ctx, http.MethodGet, "/lol-gameflow/v1/gameflow-phase", nil, &phase); err != nil || phase != "ChampSelect" {
		return errors.New("只能在英雄选择阶段应用装备方案")
	}
	var session lcuChampSelectSession
	if err := client.RequestJSON(ctx, http.MethodGet, "/lol-champ-select/v1/session", nil, &session); err != nil {
		return errors.New("无法确认当前英雄选择")
	}
	currentReference := gameplayReferenceFromSummoner(current)
	for _, player := range session.MyTeam {
		playerReference := normalizeGameplayReference(gameplayReference{
			PlayerRef: visibleChampSelectPlayerReference(player), AlternatePlayerRef: player.ObfuscatedPUUID,
			SummonerID: player.SummonerID, AlternateSummonerID: player.ObfuscatedSummonerID,
		})
		if !gameplayReferencesMatch(currentReference, playerReference) {
			continue
		}
		selectedChampionID := player.ChampionID
		if selectedChampionID <= 0 {
			selectedChampionID = player.ChampionPickIntent
		}
		if selectedChampionID <= 0 || selectedChampionID != championID {
			return errors.New("当前选择的英雄已经变化，已停止应用装备方案")
		}
		return nil
	}
	return errors.New("英雄选择中没有找到当前账号")
}

func applyGameplayItemSet(ctx context.Context, client *LCUClient, current Summoner, request gameplayItemSetApplyRequest, beforeWrite func() error) (string, string, error) {
	path := fmt.Sprintf("/lol-item-sets/v1/item-sets/%d/sets", current.SummonerID)
	var document lcuItemSetDocument
	if err := client.RequestJSON(ctx, http.MethodGet, path, nil, &document); err != nil {
		return "", "", err
	}
	if current.AccountID > 0 {
		if rawAccountID, ok := document["accountId"]; ok {
			var accountID int64
			if err := json.Unmarshal(rawAccountID, &accountID); err != nil || (accountID > 0 && accountID != current.AccountID) {
				return "", "", errors.New("客户端装备方案属于另一个账号")
			}
		}
	}
	itemSet := newLCUItemSet(request)
	if err := upsertLCUItemSet(document, itemSet); err != nil {
		return "", "", err
	}
	if beforeWrite != nil {
		if err := beforeWrite(); err != nil {
			return "", "", err
		}
	}
	if err := client.RequestJSON(ctx, http.MethodPut, path, document, nil); err != nil {
		return "", "", err
	}
	var verifiedDocument lcuItemSetDocument
	if err := client.RequestJSON(ctx, http.MethodGet, path, nil, &verifiedDocument); err != nil {
		return "", "", errors.New("客户端没有返回写入后的装备方案")
	}
	verified, found, err := findLCUItemSet(verifiedDocument, itemSet.UID)
	if err != nil || !found || !sameLCUItemSet(itemSet, verified) {
		return "", "", errors.New("客户端未能确认装备方案写入结果")
	}
	return itemSet.UID, itemSet.Title, nil
}

func newLCUItemSet(request gameplayItemSetApplyRequest) lcuItemSet {
	blocks := make([]lcuItemSetBlock, 0, len(request.Blocks))
	for _, source := range request.Blocks {
		items := make([]lcuItemSetItem, 0, len(source.Items))
		for _, item := range source.Items {
			items = append(items, lcuItemSetItem{ID: strconv.FormatInt(item.ID, 10), Count: item.Count})
		}
		blocks = append(blocks, lcuItemSetBlock{Type: strings.TrimSpace(source.Type), Items: items})
	}
	associatedMaps := make([]int64, 0, 1)
	if request.MapID > 0 {
		associatedMaps = append(associatedMaps, request.MapID)
	}
	return lcuItemSet{
		UID: itemSetUID(request.ChampionID, request.Position), Title: itemSetName(request.Title), Type: "custom", Map: "any", Mode: "any",
		StartedFrom: "Deep Legends", AssociatedChampions: []int64{request.ChampionID}, AssociatedMaps: associatedMaps,
		Blocks: blocks, PreferredItemSlots: []lcuPreferredItemSlot{},
	}
}

func itemSetUID(championID int64, position string) string {
	return fmt.Sprintf("deep-legends-v1-%d-%s", championID, position)
}

func itemSetName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "推荐出装"
	}
	value = "Deep Legends · " + value
	runes := []rune(value)
	if len(runes) > 60 {
		value = string(runes[:60])
	}
	return value
}

func upsertLCUItemSet(document lcuItemSetDocument, itemSet lcuItemSet) error {
	rawSets, ok := document["itemSets"]
	if !ok {
		return errors.New("客户端装备方案结构不受支持")
	}
	var sets []json.RawMessage
	if err := json.Unmarshal(rawSets, &sets); err != nil || sets == nil {
		return errors.New("客户端装备方案结构不受支持")
	}
	encoded, err := json.Marshal(itemSet)
	if err != nil {
		return err
	}
	found := -1
	for index, rawSet := range sets {
		var metadata struct {
			UID string `json:"uid"`
		}
		if err := json.Unmarshal(rawSet, &metadata); err != nil || strings.TrimSpace(string(rawSet)) == "null" {
			return errors.New("客户端装备方案集合包含无法识别的数据")
		}
		if metadata.UID != itemSet.UID {
			continue
		}
		if found >= 0 {
			return errors.New("客户端存在重复的 Deep Legends 装备方案")
		}
		found = index
	}
	if found >= 0 {
		sets[found] = encoded
	} else {
		sets = append(sets, encoded)
	}
	encodedSets, err := json.Marshal(sets)
	if err != nil {
		return err
	}
	document["itemSets"] = encodedSets
	return nil
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

type gameplayItem struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IconPath    string `json:"iconPath,omitempty"`
}

type gameplaySummonerSpell struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IconPath    string `json:"iconPath,omitempty"`
}

type gameplayAugment struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Rarity      string `json:"rarity,omitempty"`
	IconPath    string `json:"iconPath,omitempty"`
}

type gameplayAugmentRaw struct {
	ID                   int64  `json:"id"`
	Name                 string `json:"name"`
	NameTRA              string `json:"nameTRA"`
	SimpleNameTRA        string `json:"simpleNameTRA"`
	Description          string `json:"description"`
	Tooltip              string `json:"tooltip"`
	Rarity               string `json:"rarity"`
	AugmentSmallIconPath string `json:"augmentSmallIconPath"`
	IconPath             string `json:"iconPath"`
}

func (a *app) handleGameplayPerks(w http.ResponseWriter, r *http.Request) {
	client, _, err := a.gameplayClient()
	if err != nil {
		// 客户端未连接（例如只查看韩服页签）时改用 Data Dragon 中文目录。
		styles, perks, fallbackErr := a.fallbackGameplayPerks(r.Context())
		if fallbackErr != nil {
			http.Error(w, "客户端未提供符文图标目录", http.StatusNotFound)
			return
		}
		augments, _ := a.fallbackGameplayAugments(r.Context())
		respondJSON(w, map[string]any{"styles": styles, "perks": perks, "augments": augments})
		return
	}
	var styles []gameplayPerkStyle
	styleErr := client.GetJSON("/lol-game-data/assets/v1/perkstyles.json", &styles)
	var perks []gameplayPerk
	perkErr := client.GetJSON("/lol-game-data/assets/v1/perks.json", &perks)
	if styleErr != nil && perkErr != nil {
		http.Error(w, "客户端未提供符文图标目录", http.StatusNotFound)
		return
	}
	for index := range styles {
		styles[index].IconPath = sanitizeAssetPath(styles[index].IconPath)
		for slotIndex := range styles[index].Slots {
			for perkIndex := range styles[index].Slots[slotIndex].Perks {
				styles[index].Slots[slotIndex].Perks[perkIndex].IconPath = sanitizeAssetPath(styles[index].Slots[slotIndex].Perks[perkIndex].IconPath)
			}
		}
	}
	for index := range perks {
		perks[index].IconPath = sanitizeAssetPath(perks[index].IconPath)
	}
	augments, augmentErr := loadGameplayAugmentsFromClient(client)
	if augmentErr != nil {
		augments, _ = a.fallbackGameplayAugments(r.Context())
	}
	respondJSON(w, map[string]any{"styles": styles, "perks": perks, "augments": augments})
}

func loadGameplayAugmentsFromClient(client *LCUClient) ([]gameplayAugment, error) {
	var raw []gameplayAugmentRaw
	if err := client.GetJSON("/lol-game-data/assets/v1/cherry-augments.json", &raw); err != nil {
		return nil, err
	}
	return normalizeGameplayAugments(raw), nil
}

func (a *app) fallbackGameplayAugments(ctx context.Context) ([]gameplayAugment, error) {
	loadCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	data, err := a.champions.fetch(loadCtx, communityDragonHost, "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/cherry-augments.json", nil, 1<<20, "application/json")
	if err != nil {
		return nil, err
	}
	var raw []gameplayAugmentRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return normalizeGameplayAugments(raw), nil
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
		description := cleanMarkup(source.Description)
		if description == "" {
			description = cleanMarkup(source.Tooltip)
		}
		iconPath := sanitizeAssetPath(source.AugmentSmallIconPath)
		if iconPath == "" {
			iconPath = sanitizeAssetPath(source.IconPath)
		}
		seen[source.ID] = true
		result = append(result, gameplayAugment{ID: source.ID, Name: name, Description: description, Rarity: strings.TrimSpace(source.Rarity), IconPath: iconPath})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
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
	}
	if err := client.GetJSON("/lol-game-data/assets/v1/items.json", &raw); err != nil {
		http.Error(w, "客户端未提供装备目录", http.StatusNotFound)
		return
	}
	items := make([]gameplayItem, 0, len(raw))
	for _, source := range raw {
		name := strings.TrimSpace(source.Name)
		if name == "" {
			name = strings.TrimSpace(source.DisplayName)
		}
		description := strings.TrimSpace(source.Description)
		if description == "" {
			description = strings.TrimSpace(source.ShortDescription)
		}
		iconPath := sanitizeAssetPath(source.IconPath)
		if iconPath == "" {
			iconPath = sanitizeAssetPath(source.ImagePath)
		}
		if source.ID > 0 {
			items = append(items, gameplayItem{ID: source.ID, Name: name, Description: description, IconPath: iconPath})
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
	if err := client.GetJSON("/lol-game-data/assets/v1/summoner-spells.json", &raw); err != nil {
		http.Error(w, "客户端未提供召唤师技能目录", http.StatusNotFound)
		return
	}
	spells := make([]gameplaySummonerSpell, 0, len(raw))
	for _, source := range raw {
		iconPath := sanitizeAssetPath(source.IconPath)
		if iconPath == "" {
			iconPath = sanitizeAssetPath(source.ImagePath)
		}
		if source.ID > 0 {
			spells = append(spells, gameplaySummonerSpell{ID: source.ID, Name: strings.TrimSpace(source.Name), Description: strings.TrimSpace(source.Description), IconPath: iconPath})
		}
	}
	respondJSON(w, map[string]any{"spells": spells})
}

// ---------- 未连接客户端时的 Data Dragon 目录兜底 ----------
// iconPath 以 "ddragon:" 前缀标记，前端会改走 /api/champion-asset 代理。

func (a *app) fallbackGameplayItems(ctx context.Context) ([]gameplayItem, error) {
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
		items = append(items, gameplayItem{ID: id, Name: description.Name, Description: description.Description, IconPath: "ddragon:" + description.Path})
	}
	if len(items) == 0 {
		return nil, errors.New("Data Dragon 装备目录为空")
	}
	return items, nil
}

func (a *app) fallbackGameplaySummonerSpells(ctx context.Context) ([]gameplaySummonerSpell, error) {
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
	var queues []struct {
		ID        int64  `json:"id"`
		Name      string `json:"name"`
		ShortName string `json:"shortName"`
	}
	result := make(map[int64]string)
	if client.GetJSON("/lol-game-queues/v1/queues", &queues) == nil {
		for _, queue := range queues {
			name := strings.TrimSpace(queue.ShortName)
			if name == "" {
				name = strings.TrimSpace(queue.Name)
			}
			if queue.ID > 0 && name != "" {
				result[queue.ID] = name
			}
		}
	}
	return result
}

func queueLabel(queueID int64, mode string, labels map[int64]string) string {
	if label := strings.TrimSpace(labels[queueID]); label != "" {
		return label
	}
	switch queueID {
	case 420:
		return "单排/双排"
	case 440:
		return "灵活组排"
	case 450:
		return "极地大乱斗"
	case 900:
		return "无限火力"
	case 1700, 1710:
		return "斗魂竞技场"
	case 2300, 2400:
		return "海克斯大乱斗"
	}
	if strings.TrimSpace(mode) != "" {
		return mode
	}
	if queueID > 0 {
		return fmt.Sprintf("模式 %d", queueID)
	}
	return "自定义对局"
}

func queueModeGroup(queueID int64) string {
	switch queueID {
	case 420:
		return "solo"
	case 440:
		return "flex"
	case 2300, 2400:
		return "hextech-aram"
	case 1700, 1710:
		return "arena"
	case 450, 480, 930:
		// 极地大乱斗（含活动变体队列）。
		return "aram"
	default:
		return "other"
	}
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
	return "未知英雄"
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
