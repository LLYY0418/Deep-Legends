package main

// riot_api.go 通过 Riot Games 官方开发者 API 查询韩服（KR）玩家的
// 生涯、段位、熟练度与最近战绩，并复用 gameplay.go 的聚合与匿名引用
// 机制，让韩服玩家在总览页获得与国服玩家一致的展示。
//
// 官方 API 不覆盖国服（腾讯运营的服务器不在 Riot 公开平台列表中），
// 国服玩家仍然只能通过本机已登录的客户端查询。
//
// 请求只发往固定的 Riot 官方域名，只携带 API Key 与被查询的 Riot ID，
// 不附带本机账号、Cookie 或客户端令牌；网络代理设置与“英雄数据网络”共用。

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// Riot API Key 配置（三种方式按优先级从高到低）：
//
//  1. 环境变量 RIOT_API_KEY —— 仅本机临时调试用，不需要重新构建。
//  2. riotAPIKeyCipher —— 推荐：加密后的 key（AES-256-GCM + Base64），
//     在构建时通过 -ldflags "-X main.riotAPIKeyCipher=<密文>" 注入，
//     源码里这个变量必须永远留空字符串，绝不能把真实密文写死提交到仓库。
//     生成密文：go run . -encrypt-riot-key "RGAPI-你的key"
//     构建示例：./build-desktop-windows.ps1 -RiotAPIKeyCipher "<密文>"
//     （或设置环境变量 RIOT_API_KEY_CIPHER，构建脚本会自动读取）
//  3. riotAPIKey —— 明文变量，同样只能通过构建时注入，不能写死提交。
//
// 重要边界说明：密文与解密逻辑都在程序里，这只是混淆——能防止用
// strings 等工具从 EXE 中直接扫出明文 key，但挡不住有心人抓包或逆向。
// key 属于开发者个人凭据，泄露可到 developer.riotgames.com 重置后重新生成密文。
// ============================================================================
var riotAPIKeyCipher = ""

var riotAPIKey = ""

// riotCipherKey 由分散的固定片段派生解密密钥；仅用于混淆，见上方说明。
func riotCipherKey() []byte {
	parts := []string{"deep", "legends", "hexcore", "loot", "kr-riot-channel", "v1"}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return sum[:]
}

func encryptRiotKey(plain string) (string, error) {
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return "", errors.New("key 不能为空")
	}
	block, err := aes.NewCipher(riotCipherKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func decryptRiotKeyCipher(encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(riotCipherKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) <= gcm.NonceSize() {
		return "", errors.New("riot key 密文长度无效")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", errors.New("riot key 密文无法解密，请用 -encrypt-riot-key 重新生成")
	}
	return string(plain), nil
}

var riotEmbeddedKey = sync.OnceValue(func() string {
	if riotAPIKeyCipher != "" {
		if plain, err := decryptRiotKeyCipher(riotAPIKeyCipher); err == nil {
			return strings.TrimSpace(plain)
		}
	}
	return strings.TrimSpace(riotAPIKey)
})

func riotKey() string {
	if value := strings.TrimSpace(os.Getenv("RIOT_API_KEY")); value != "" {
		return value
	}
	return riotEmbeddedKey()
}

const (
	riotRegionKR = "kr"
	// Riot ID 解析与 Match-V5 战绩使用大区集群主机（韩服属于 asia）；
	// 召唤师资料、段位、熟练度使用具体平台主机。
	riotClusterHost  = "asia.api.riotgames.com"
	riotPlatformHost = "kr.api.riotgames.com"
	riotResponseMax  = 4 << 20
	// 对局时间线包含逐帧事件（击杀伤害明细等），体积远大于常规接口。
	riotTimelineResponseMax = 16 << 20
	riotMatchCacheMax       = 600
)

var errRiotNotFound = errors.New("riot: not found")

type riotProvider struct {
	champions *championProvider

	limitMu     sync.Mutex
	shortWindow []time.Time
	longWindow  []time.Time

	cacheMu    sync.Mutex
	matchCache map[string]*riotMatch
	matchOrder []string

	specialistMu      sync.Mutex
	specialistCache   map[int64]specialistRuneCacheEntry
	specialistFlights map[int64]*specialistRuneFlight
	specialistSlots   chan struct{}
}

func newRiotProvider(champions *championProvider) *riotProvider {
	return &riotProvider{
		champions: champions, matchCache: make(map[string]*riotMatch),
		specialistCache: make(map[int64]specialistRuneCacheEntry), specialistFlights: make(map[int64]*specialistRuneFlight), specialistSlots: make(chan struct{}, 1),
	}
}

func riotKeyConfigured() bool { return riotKey() != "" }

// wait 在本地执行保守限速（Personal Key 上限 20 次/秒、100 次/2 分钟，
// 这里各留出安全余量），超过预算时阻塞等待而不是直接失败。
func (p *riotProvider) wait(ctx context.Context) error {
	const (
		shortLimit  = 15
		shortPeriod = time.Second
		longLimit   = 90
		longPeriod  = 2 * time.Minute
	)
	for {
		p.limitMu.Lock()
		now := time.Now()
		p.shortWindow = pruneTimestamps(p.shortWindow, now.Add(-shortPeriod))
		p.longWindow = pruneTimestamps(p.longWindow, now.Add(-longPeriod))
		if len(p.shortWindow) < shortLimit && len(p.longWindow) < longLimit {
			p.shortWindow = append(p.shortWindow, now)
			p.longWindow = append(p.longWindow, now)
			p.limitMu.Unlock()
			return nil
		}
		var sleep time.Duration
		if len(p.shortWindow) >= shortLimit {
			sleep = p.shortWindow[0].Add(shortPeriod).Sub(now)
		}
		if len(p.longWindow) >= longLimit {
			if wait := p.longWindow[0].Add(longPeriod).Sub(now); wait > sleep {
				sleep = wait
			}
		}
		p.limitMu.Unlock()
		if sleep < 50*time.Millisecond {
			sleep = 50 * time.Millisecond
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleep):
		}
	}
}

func pruneTimestamps(window []time.Time, cutoff time.Time) []time.Time {
	index := 0
	for index < len(window) && window[index].Before(cutoff) {
		index++
	}
	return append(window[:0], window[index:]...)
}

func (p *riotProvider) get(ctx context.Context, host, requestPath string, query url.Values, out any) error {
	return p.getLimited(ctx, host, requestPath, query, out, riotResponseMax)
}

// getLimited 与 get 相同，但允许调用方指定响应大小上限；对局时间线
// （timeline）包含逐帧事件，体积可达数 MB，需要比常规接口更大的额度。
func (p *riotProvider) getLimited(ctx context.Context, host, requestPath string, query url.Values, out any, responseMax int64) error {
	if !riotKeyConfigured() {
		return errors.New("尚未配置 Riot API Key：请用 -encrypt-riot-key 生成密文，构建时通过 -ldflags \"-X main.riotAPIKeyCipher=<密文>\" 注入（临时调试可用环境变量 RIOT_API_KEY）")
	}
	for attempt := 0; attempt < 3; attempt++ {
		if err := p.wait(ctx); err != nil {
			return err
		}
		// requestPath 已由调用方用 url.PathEscape 逐段转义，这里必须按
		// 字符串拼接后交给 http.NewRequest 解析；若赋值给 url.URL.Path，
		// 序列化时 % 会被二次转义，带空格或韩文的 Riot ID 会全部 404。
		endpoint := "https://" + host + requestPath
		if encoded := query.Encode(); encoded != "" {
			endpoint += "?" + encoded
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		request.Header.Set("X-Riot-Token", riotKey())
		request.Header.Set("Accept", "application/json")
		request.Header.Set("User-Agent", "Deep-Legends/"+version)
		response, err := p.champions.httpClient().Do(request)
		if err != nil {
			return fmt.Errorf("无法连接 Riot 官方接口（可在设置中调整“英雄数据网络”代理）：%w", err)
		}
		body, readErr := readLimited(response.Body, responseMax)
		response.Body.Close()
		switch response.StatusCode {
		case http.StatusOK:
			if readErr != nil {
				return readErr
			}
			if err := json.Unmarshal(body, out); err != nil {
				return errors.New("Riot 接口返回的数据无法解析，可能接口已变更")
			}
			return nil
		case http.StatusNotFound:
			return errRiotNotFound
		case http.StatusUnauthorized, http.StatusForbidden:
			return errors.New("Riot API Key 无效、过期或无权访问：请到 developer.riotgames.com 检查后重新生成密文并在构建时注入")
		case http.StatusTooManyRequests:
			retryAfter := time.Duration(3) * time.Second
			if seconds, parseErr := strconv.Atoi(response.Header.Get("Retry-After")); parseErr == nil && seconds > 0 && seconds <= 30 {
				retryAfter = time.Duration(seconds) * time.Second
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryAfter):
			}
		default:
			return fmt.Errorf("Riot 接口返回 HTTP %d", response.StatusCode)
		}
	}
	return errors.New("Riot 接口限流中（HTTP 429），请稍后重试")
}

// riotStatusError 携带面向用户的中文提示与对应的 HTTP 状态码。
type riotStatusError struct {
	message string
	status  int
}

func (e *riotStatusError) Error() string { return e.message }

func riotNotFoundError(format string, args ...any) error {
	return &riotStatusError{message: fmt.Sprintf(format, args...), status: http.StatusNotFound}
}

func riotErrorStatus(err error) int {
	var statusErr *riotStatusError
	if errors.As(err, &statusErr) {
		return statusErr.status
	}
	if errors.Is(err, errRiotNotFound) {
		return http.StatusNotFound
	}
	return http.StatusBadGateway
}

/* ---------- Riot API 数据结构（只保留项目需要的字段） ---------- */

type riotAccount struct {
	PUUID    string `json:"puuid"`
	GameName string `json:"gameName"`
	TagLine  string `json:"tagLine"`
}

type riotSummoner struct {
	PUUID         string `json:"puuid"`
	ProfileIconID int64  `json:"profileIconId"`
	SummonerLevel int64  `json:"summonerLevel"`
}

type riotLeagueEntry struct {
	QueueType    string `json:"queueType"`
	Tier         string `json:"tier"`
	Rank         string `json:"rank"`
	LeaguePoints int    `json:"leaguePoints"`
	Wins         int    `json:"wins"`
	Losses       int    `json:"losses"`
}

type riotMasteryEntry struct {
	ChampionID     int64 `json:"championId"`
	ChampionLevel  int64 `json:"championLevel"`
	ChampionPoints int64 `json:"championPoints"`
	LastPlayTime   int64 `json:"lastPlayTime"`
}

type riotPerkSelections struct {
	Description string `json:"description"`
	Style       int64  `json:"style"`
	Selections  []struct {
		Perk int64 `json:"perk"`
	} `json:"selections"`
}

type riotParticipant struct {
	ParticipantID               int64  `json:"participantId"`
	TeamID                      int64  `json:"teamId"`
	PUUID                       string `json:"puuid"`
	RiotIDGameName              string `json:"riotIdGameName"`
	RiotIDTagline               string `json:"riotIdTagline"`
	SummonerName                string `json:"summonerName"`
	ProfileIcon                 int64  `json:"profileIcon"`
	ChampionID                  int64  `json:"championId"`
	ChampLevel                  int    `json:"champLevel"`
	Summoner1ID                 int64  `json:"summoner1Id"`
	Summoner2ID                 int64  `json:"summoner2Id"`
	Item0                       int64  `json:"item0"`
	Item1                       int64  `json:"item1"`
	Item2                       int64  `json:"item2"`
	Item3                       int64  `json:"item3"`
	Item4                       int64  `json:"item4"`
	Item5                       int64  `json:"item5"`
	Item6                       int64  `json:"item6"`
	TeamPosition                string `json:"teamPosition"`
	IndividualPosition          string `json:"individualPosition"`
	Kills                       int    `json:"kills"`
	Deaths                      int    `json:"deaths"`
	Assists                     int    `json:"assists"`
	TotalMinionsKilled          int    `json:"totalMinionsKilled"`
	NeutralMinionsKilled        int    `json:"neutralMinionsKilled"`
	GoldEarned                  int    `json:"goldEarned"`
	TotalDamageDealtToChampions int    `json:"totalDamageDealtToChampions"`
	TotalDamageTaken            int    `json:"totalDamageTaken"`
	VisionScore                 int    `json:"visionScore"`
	WardsPlaced                 int    `json:"wardsPlaced"`
	WardsKilled                 int    `json:"wardsKilled"`
	Win                         bool   `json:"win"`
	LargestMultiKill            int    `json:"largestMultiKill"`
	// 斗魂竞技场：多支小队同场，playerSubteamId 标记所属小队，
	// subteamPlacement 是该小队的最终名次。
	PlayerSubteamID  int64 `json:"playerSubteamId"`
	SubteamPlacement int   `json:"subteamPlacement"`
	// 斗魂竞技场的海克斯强化（最多 6 个，0 表示空位）。
	PlayerAugment1 int64 `json:"playerAugment1"`
	PlayerAugment2 int64 `json:"playerAugment2"`
	PlayerAugment3 int64 `json:"playerAugment3"`
	PlayerAugment4 int64 `json:"playerAugment4"`
	PlayerAugment5 int64 `json:"playerAugment5"`
	PlayerAugment6 int64 `json:"playerAugment6"`
	Perks          struct {
		StatPerks struct {
			Offense int64 `json:"offense"`
			Flex    int64 `json:"flex"`
			Defense int64 `json:"defense"`
		} `json:"statPerks"`
		Styles []riotPerkSelections `json:"styles"`
	} `json:"perks"`
}

type riotTeam struct {
	TeamID     int64 `json:"teamId"`
	Win        bool  `json:"win"`
	Objectives struct {
		Baron struct {
			Kills int `json:"kills"`
		} `json:"baron"`
		Dragon struct {
			Kills int `json:"kills"`
		} `json:"dragon"`
		Inhibitor struct {
			Kills int `json:"kills"`
		} `json:"inhibitor"`
		Tower struct {
			Kills int `json:"kills"`
		} `json:"tower"`
	} `json:"objectives"`
}

// riotMatchInfo 是 Match-V5 风格的单场对局数据；Riot 官方接口与
// 国服 SGP 网关（见 sgp_api.go）返回的结构一致，双方共用同一套转换逻辑。
type riotMatchInfo struct {
	GameID           int64             `json:"gameId"`
	GameCreation     int64             `json:"gameCreation"`
	GameDuration     int64             `json:"gameDuration"`
	GameEndTimestamp int64             `json:"gameEndTimestamp"`
	QueueID          int64             `json:"queueId"`
	GameMode         string            `json:"gameMode"`
	MapID            int64             `json:"mapId"`
	Participants     []riotParticipant `json:"participants"`
	Teams            []riotTeam        `json:"teams"`
}

type riotMatch struct {
	Metadata struct {
		MatchID string `json:"matchId"`
	} `json:"metadata"`
	Info riotMatchInfo `json:"info"`
}

/* ---------- 端点封装 ---------- */

func (p *riotProvider) accountByRiotID(ctx context.Context, gameName, tagLine string) (riotAccount, error) {
	var account riotAccount
	path := "/riot/account/v1/accounts/by-riot-id/" + url.PathEscape(gameName) + "/" + url.PathEscape(tagLine)
	if err := p.get(ctx, riotClusterHost, path, nil, &account); err != nil {
		return riotAccount{}, err
	}
	if account.PUUID == "" {
		return riotAccount{}, errRiotNotFound
	}
	return account, nil
}

func (p *riotProvider) summonerByPUUID(ctx context.Context, puuid string) (riotSummoner, error) {
	var summoner riotSummoner
	err := p.get(ctx, riotPlatformHost, "/lol/summoner/v4/summoners/by-puuid/"+url.PathEscape(puuid), nil, &summoner)
	return summoner, err
}

func (p *riotProvider) leagueEntries(ctx context.Context, puuid string) ([]riotLeagueEntry, error) {
	var entries []riotLeagueEntry
	err := p.get(ctx, riotPlatformHost, "/lol/league/v4/entries/by-puuid/"+url.PathEscape(puuid), nil, &entries)
	return entries, err
}

func (p *riotProvider) topMasteries(ctx context.Context, puuid string, count int) ([]riotMasteryEntry, error) {
	var entries []riotMasteryEntry
	query := url.Values{"count": {strconv.Itoa(count)}}
	err := p.get(ctx, riotPlatformHost, "/lol/champion-mastery/v4/champion-masteries/by-puuid/"+url.PathEscape(puuid)+"/top", query, &entries)
	return entries, err
}

func (p *riotProvider) matchIDs(ctx context.Context, puuid string, start, count int) ([]string, error) {
	var ids []string
	query := url.Values{"start": {strconv.Itoa(start)}, "count": {strconv.Itoa(count)}}
	err := p.get(ctx, riotClusterHost, "/lol/match/v5/matches/by-puuid/"+url.PathEscape(puuid)+"/ids", query, &ids)
	return ids, err
}

// matchByID 带进程内缓存：对局 JSON 一旦生成就不会变化，翻页或
// 查看同场其他玩家时直接命中，显著节省官方接口配额。
func (p *riotProvider) matchByID(ctx context.Context, matchID string) (*riotMatch, error) {
	p.cacheMu.Lock()
	if cached, ok := p.matchCache[matchID]; ok {
		p.cacheMu.Unlock()
		return cached, nil
	}
	p.cacheMu.Unlock()
	var match riotMatch
	if err := p.get(ctx, riotClusterHost, "/lol/match/v5/matches/"+url.PathEscape(matchID), nil, &match); err != nil {
		return nil, err
	}
	if match.Metadata.MatchID == "" || len(match.Info.Participants) == 0 {
		return nil, errors.New("Riot 战绩详情缺少必要字段，可能接口已变更")
	}
	p.cacheMu.Lock()
	if _, exists := p.matchCache[matchID]; !exists {
		p.matchCache[matchID] = &match
		p.matchOrder = append(p.matchOrder, matchID)
		for len(p.matchOrder) > riotMatchCacheMax {
			delete(p.matchCache, p.matchOrder[0])
			p.matchOrder = p.matchOrder[1:]
		}
	}
	p.cacheMu.Unlock()
	return &match, nil
}

// riotTimeline 只保留时间线里项目需要的帧结构（事件形状与 SGP 同构）。
type riotTimeline struct {
	Info struct {
		Frames []timelineFrame `json:"frames"`
	} `json:"info"`
}

// matchTimeline 读取单场对局的时间线（装备路线与技能加点用）。
// 韩服对局的 matchID 由 "KR_" + gameId 构成。
func (p *riotProvider) matchTimeline(ctx context.Context, matchID string) ([]timelineFrame, error) {
	var timeline riotTimeline
	if err := p.getLimited(ctx, riotClusterHost, "/lol/match/v5/matches/"+url.PathEscape(matchID)+"/timeline", nil, &timeline, riotTimelineResponseMax); err != nil {
		return nil, err
	}
	if len(timeline.Info.Frames) == 0 {
		return nil, errors.New("Riot 未返回该对局的时间线")
	}
	return timeline.Info.Frames, nil
}

/* ---------- 数据映射 ---------- */

func riotPositionKey(participant riotParticipant) string {
	position := strings.ToUpper(strings.TrimSpace(participant.TeamPosition))
	if position == "" {
		position = strings.ToUpper(strings.TrimSpace(participant.IndividualPosition))
	}
	switch position {
	case "TOP":
		return "top"
	case "JUNGLE":
		return "jungle"
	case "MIDDLE", "MID":
		return "middle"
	case "BOTTOM":
		return "bottom"
	case "UTILITY", "SUPPORT":
		return "utility"
	}
	return ""
}

func riotMatchDurationSeconds(info *riotMatchInfo) int64 {
	duration := info.GameDuration
	// 2021 年中之前的对局 gameDuration 单位是毫秒（无 gameEndTimestamp 字段）。
	if info.GameEndTimestamp == 0 && duration > 20000 {
		duration /= 1000
	}
	return duration
}

func riotConvertMatch(match *riotMatch, subjectPUUID string, names map[int64]string) gameplayMatch {
	return convertRiotMatchInfo(&match.Info, subjectPUUID, names, nil, riotRegionKR, "")
}

// convertRiotMatchInfo 把 Match-V5 风格的对局转换为界面模型；region 标注
// 韩服，serverID 标注国服子服务器，两者共同进入后端匿名引用作用域。
func convertRiotMatchInfo(info *riotMatchInfo, subjectPUUID string, names map[int64]string, queueLabels map[int64]string, region, serverID string) gameplayMatch {
	duration := riotMatchDurationSeconds(info)
	createdAt := normalizeEpochMillis(info.GameCreation)
	if createdAt <= 0 && info.GameEndTimestamp > 0 {
		// 个别对局缺少 gameCreation：用结束时间倒推开局时间兜底。
		createdAt = normalizeEpochMillis(info.GameEndTimestamp) - duration*1000
	}
	result := gameplayMatch{
		GameID:     info.GameID,
		CreatedAt:  createdAt,
		Duration:   duration,
		QueueID:    info.QueueID,
		QueueLabel: queueLabel(info.QueueID, info.GameMode, queueLabels),
		ModeGroup:  queueModeGroup(info.QueueID),
		GameMode:   info.GameMode,
		MapID:      info.MapID,
	}
	for _, raw := range info.Participants {
		name := strings.TrimSpace(raw.RiotIDGameName)
		if name == "" {
			name = strings.TrimSpace(raw.SummonerName)
		}
		hidden := name == ""
		if hidden {
			name = "隐藏玩家"
		}
		perkIDs := make([]int64, 0, 9)
		var primaryStyle, subStyle int64
		for index, style := range raw.Perks.Styles {
			if index == 0 || strings.EqualFold(style.Description, "primaryStyle") {
				if primaryStyle == 0 {
					primaryStyle = style.Style
				}
			} else if subStyle == 0 {
				subStyle = style.Style
			}
			for _, selection := range style.Selections {
				if selection.Perk > 0 {
					perkIDs = append(perkIDs, selection.Perk)
				}
			}
		}
		for _, statPerk := range []int64{raw.Perks.StatPerks.Offense, raw.Perks.StatPerks.Flex, raw.Perks.StatPerks.Defense} {
			if statPerk > 0 {
				perkIDs = append(perkIDs, statPerk)
			}
		}
		cs := raw.TotalMinionsKilled + raw.NeutralMinionsKilled
		augments := make([]int64, 0, 6)
		for _, augment := range []int64{raw.PlayerAugment1, raw.PlayerAugment2, raw.PlayerAugment3, raw.PlayerAugment4, raw.PlayerAugment5, raw.PlayerAugment6} {
			if augment > 0 {
				augments = append(augments, augment)
			}
		}
		reference := normalizeGameplayReference(gameplayReference{
			PlayerRef: raw.PUUID, GameName: raw.RiotIDGameName, TagLine: raw.RiotIDTagline,
			DisplayName: raw.SummonerName, ProfileIconID: raw.ProfileIcon, Region: region, ServerID: serverID,
		})
		participant := gameplayParticipant{
			ParticipantID: raw.ParticipantID, TeamID: raw.TeamID, PlayerRef: reference.PlayerRef,
			DisplayName: name, GameName: raw.RiotIDGameName, TagLine: raw.RiotIDTagline, ProfileIconID: raw.ProfileIcon,
			ChampionID: raw.ChampionID, ChampionName: championName(names, raw.ChampionID), ChampionLevel: raw.ChampLevel,
			Spell1ID: raw.Summoner1ID, Spell2ID: raw.Summoner2ID, PrimaryStyleID: primaryStyle, SubStyleID: subStyle,
			PerkIDs: perkIDs, ItemIDs: itemSlots(raw.Item0, raw.Item1, raw.Item2, raw.Item3, raw.Item4, raw.Item5, raw.Item6),
			Position: riotPositionKey(raw),
			Kills:    raw.Kills, Deaths: raw.Deaths, Assists: raw.Assists,
			KDA: ratio(raw.Kills+raw.Assists, raw.Deaths), CS: cs,
			LaneCS: raw.TotalMinionsKilled, JungleCS: raw.NeutralMinionsKilled, CSPerMinute: perMinute(cs, duration),
			Gold: raw.GoldEarned, Damage: raw.TotalDamageDealtToChampions, DamageTaken: raw.TotalDamageTaken,
			VisionScore: raw.VisionScore, WardsPlaced: raw.WardsPlaced, WardsKilled: raw.WardsKilled,
			Win: raw.Win, Hidden: hidden, MultiKill: raw.LargestMultiKill,
			SubteamID: raw.PlayerSubteamID, Placement: raw.SubteamPlacement, AugmentIDs: augments, reference: reference,
		}
		result.Participants = append(result.Participants, participant)
		if subjectPUUID != "" && raw.PUUID == subjectPUUID {
			result.SubjectParticipantID = raw.ParticipantID
			if raw.Win {
				result.Result = "win"
			} else {
				result.Result = "loss"
			}
		}
	}
	if result.Result == "" {
		result.Result = "unknown"
	}
	teamByID := make(map[int64]gameplayTeam)
	for _, raw := range info.Teams {
		teamByID[raw.TeamID] = gameplayTeam{
			TeamID: raw.TeamID, Win: raw.Win,
			TowerKills: raw.Objectives.Tower.Kills, DragonKills: raw.Objectives.Dragon.Kills,
			BaronKills: raw.Objectives.Baron.Kills, InhibitorKills: raw.Objectives.Inhibitor.Kills,
		}
	}
	for _, participant := range result.Participants {
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
		result.Teams = append(result.Teams, team)
	}
	sort.Slice(result.Teams, func(i, j int) bool { return result.Teams[i].TeamID < result.Teams[j].TeamID })
	return result
}

/* ---------- 总览装配 ---------- */

// riotChampionNames 合并 Data Dragon 中文英雄目录与本机客户端目录，
// 未连接客户端时仍能显示中文英雄名。
func (a *app) riotChampionNames(ctx context.Context) map[int64]string {
	names := a.riot.champions.championNamesZH(ctx)
	for id, name := range a.championNames() {
		names[id] = name
	}
	return names
}

func (a *app) loadRiotOverview(ctx context.Context, reference gameplayReference, begIndex, count int) (gameplayOverview, error) {
	if a.riot == nil {
		return gameplayOverview{}, errors.New("Riot 查询通道未初始化")
	}
	provider := a.riot
	puuid := strings.TrimSpace(reference.PlayerRef)
	gameName := strings.TrimSpace(reference.GameName)
	tagLine := strings.TrimSpace(reference.TagLine)
	if puuid == "" {
		var account riotAccount
		err := errRiotNotFound
		if tagLine != "" {
			account, err = provider.accountByRiotID(ctx, gameName, tagLine)
		}
		if errors.Is(err, errRiotNotFound) {
			// 编号错误或玩家改过名时，Riot 的精确查询会直接 404，而 OP.GG
			// 按名称模糊搜索仍能找到人。这里用 OP.GG 自动补全接口纠正
			// 编号后重查一次，让搜索体验与 OP.GG 一致。
			if corrected, ok := a.opggResolveRiotID(ctx, gameName); ok && !strings.EqualFold(corrected.TagLine, tagLine) {
				account, err = provider.accountByRiotID(ctx, corrected.GameName, corrected.TagLine)
			}
		}
		if errors.Is(err, errRiotNotFound) {
			if tagLine == "" {
				return gameplayOverview{}, riotNotFoundError("没有找到玩家「%s」：请补全 # 后的编号，或核对名称拼写", gameName)
			}
			return gameplayOverview{}, riotNotFoundError("没有找到 Riot ID「%s#%s」：编号可能不对（并非所有玩家都是 KR1），请核对后重试", gameName, tagLine)
		}
		if err != nil {
			return gameplayOverview{}, err
		}
		puuid = account.PUUID
		gameName = account.GameName
		tagLine = account.TagLine
	}
	capabilities := make([]EndpointCapability, 0, 5)
	summoner, err := provider.summonerByPUUID(ctx, puuid)
	if errors.Is(err, errRiotNotFound) {
		return gameplayOverview{}, riotNotFoundError("「%s#%s」不在韩服（该 Riot ID 属于其他大区）", gameName, tagLine)
	}
	if err != nil {
		return gameplayOverview{}, err
	}
	capabilities = append(capabilities, EndpointCapability{Name: "summoner", Path: "riot: /lol/summoner/v4/summoners/by-puuid", State: capabilityAvailable, Count: 1})
	reference = mergeGameplayReferences(gameplayReference{
		PlayerRef: puuid, GameName: gameName, TagLine: tagLine,
		ProfileIconID: summoner.ProfileIconID, SummonerLevel: summoner.SummonerLevel, Region: riotRegionKR,
	}, reference)
	names := a.riotChampionNames(ctx)
	ids, err := provider.matchIDs(ctx, puuid, begIndex, count)
	if err != nil && !errors.Is(err, errRiotNotFound) {
		return gameplayOverview{}, err
	}
	// 段位与熟练度和对局详情并行读取：串行时一次搜索要等 20 多个请求
	// 依次返回，是“韩服搜索慢”的主要来源之一。
	var ranks []gameplayRank
	var rankCapability EndpointCapability
	var masteries []gameplayMastery
	var masteryCapability EndpointCapability
	var profileWait sync.WaitGroup
	if begIndex == 0 {
		profileWait.Add(2)
		go func() {
			defer profileWait.Done()
			ranks, rankCapability = provider.loadRiotRanks(ctx, puuid)
		}()
		go func() {
			defer profileWait.Done()
			masteries, masteryCapability = provider.loadRiotMasteries(ctx, puuid, names)
		}()
	}
	matches := make([]gameplayMatch, 0, len(ids))
	loadedDetails := 0
	var loadMu sync.Mutex
	var wait sync.WaitGroup
	semaphore := make(chan struct{}, 8)
	details := make([]*riotMatch, len(ids))
	for index := range ids {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			detail, detailErr := provider.matchByID(ctx, ids[index])
			if detailErr != nil {
				return
			}
			loadMu.Lock()
			details[index] = detail
			loadedDetails++
			loadMu.Unlock()
		}(index)
	}
	wait.Wait()
	for _, detail := range details {
		if detail == nil {
			continue
		}
		matches = append(matches, riotConvertMatch(detail, puuid, names))
	}
	historyCapability := EndpointCapability{Name: "match-history", Path: "riot: /lol/match/v5/matches/by-puuid", State: capabilityAvailable, Count: len(ids)}
	if err != nil {
		historyCapability.State = capabilityUnsupported
		historyCapability.Detail = "Riot 未返回该玩家的战绩列表"
	}
	capabilities = append(capabilities, historyCapability)
	detailCapability := EndpointCapability{Name: "match-details", Path: "riot: /lol/match/v5/matches/{matchId}", Count: loadedDetails}
	switch {
	case len(ids) == 0 || loadedDetails == len(ids):
		detailCapability.State = capabilityAvailable
	case loadedDetails > 0:
		detailCapability.State = capabilityFailed
		detailCapability.Detail = "部分对局详情暂不可用，已保留可核验的场次"
	default:
		detailCapability.State = capabilityFailed
		detailCapability.Detail = "对局详情读取失败，请稍后重试"
	}
	capabilities = append(capabilities, detailCapability)
	hasMore := len(ids) == count
	response := gameplayOverview{
		Player: gameplayPlayer{
			PlayerRef: puuid, DisplayName: gameName, GameName: gameName, TagLine: tagLine,
			ProfileIconID: summoner.ProfileIconID, SummonerLevel: summoner.SummonerLevel,
			Region: riotRegionKR, Hidden: gameName == "", IsCurrent: false, reference: reference,
		},
		Matches:      matches,
		Capabilities: capabilities,
		// Count 使用服务器返回的 ID 数（个别对局详情读取失败时 matches 会
		// 少于 ids），保证前端推进下一页偏移量时不会与本页重叠。
		Pagination: gameplayPagination{BegIndex: begIndex, Count: len(ids), HasMore: hasMore},
	}
	if begIndex > 0 {
		a.publicizeOverviewReferences(&response)
		return response, nil
	}
	profileWait.Wait()
	capabilities = append(capabilities, rankCapability)
	capabilities = append(capabilities, masteryCapability)
	// 汇总统计基于当前已读取的这一页战绩：Riot Personal Key 配额有限，
	// 不像本机客户端那样一次拉取 100 场样本。
	capabilities = append(capabilities, EndpointCapability{Name: "seven-day-history", Path: "riot: 复用本页战绩样本", State: capabilityAvailable, Count: len(matches), Detail: fmt.Sprintf("基于最近 %d 场计算 30 天排位与活跃时段", len(matches))})
	response.Ranks = ranks
	response.Masteries = masteries
	response.Capabilities = capabilities
	response.Overall = aggregateMatches(matches, puuid, nil)
	recentWindowAfter := time.Now().Add(-recentWindowDays * 24 * time.Hour).UnixMilli()
	response.SevenDayRank = aggregateMatches(matches, puuid, func(match gameplayMatch) bool {
		return match.CreatedAt >= recentWindowAfter && (match.QueueID == 420 || match.QueueID == 440)
	})
	if response.SevenDayRank.Games > 0 && hasMore {
		response.SevenDayRank.Sampled = true
	}
	response.ChampionStats = championStats(matches, puuid, names)
	response.Positions = positionStats(matches, puuid)
	response.ActivityHours = activityHours(matches)
	response.RecentPlayers = recentPlayers(matches, puuid, recentWindowAfter)
	a.publicizeOverviewReferences(&response)
	return response, nil
}

// opggSearchAction 是 OP.GG 站内搜索（Next.js Server Action）的动作标识，
// 抓包自 op.gg 首页搜索框。该值随 OP.GG 前端发版可能变化；失效时下方
// 解析不到结果会静默降级，只影响“编号纠错”这一增强能力，不影响精确查询。
const opggSearchAction = "402c9587dc35c9a189a48efae20bebb24826369a95"

// opggResolveRiotID 用 OP.GG 的召唤师自动补全按名称查找韩服玩家，
// 返回其当前的完整 Riot ID（真实编号）。旧的 lol-web-api.op.gg 域名在
// 部分网络环境不可达，这里改走 op.gg 主域名的 Server Action（与网页
// 搜索框同源），失败时静默降级，不影响原有的精确查询流程。
func (a *app) opggResolveRiotID(ctx context.Context, gameName string) (riotAccount, bool) {
	if a.champions == nil || strings.TrimSpace(gameName) == "" {
		return riotAccount{}, false
	}
	searchCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	body, err := json.Marshal([]map[string]string{{"region": "kr", "value": gameName, "locale": "zh-cn"}})
	if err != nil {
		return riotAccount{}, false
	}
	request, err := http.NewRequestWithContext(searchCtx, http.MethodPost, "https://op.gg/zh-cn", bytes.NewReader(body))
	if err != nil {
		return riotAccount{}, false
	}
	request.Header.Set("Next-Action", opggSearchAction)
	request.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	request.Header.Set("Accept", "text/x-component")
	request.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36")
	response, err := a.champions.httpClient().Do(request)
	if err != nil {
		return riotAccount{}, false
	}
	data, readErr := readLimited(response.Body, 1<<20)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || readErr != nil {
		return riotAccount{}, false
	}
	candidates := opggParseSearchResult(data)
	if len(candidates) == 0 {
		return riotAccount{}, false
	}
	normalized := strings.ToLower(strings.TrimSpace(gameName))
	for _, entry := range candidates {
		if strings.ToLower(strings.TrimSpace(entry.GameName)) == normalized && strings.TrimSpace(entry.TagLine) != "" {
			return riotAccount{GameName: strings.TrimSpace(entry.GameName), TagLine: strings.TrimSpace(entry.TagLine)}, true
		}
	}
	// 没有完全同名的结果时，若只返回一名候选则采用（OP.GG 已做前缀匹配）。
	if len(candidates) == 1 && strings.TrimSpace(candidates[0].TagLine) != "" {
		return riotAccount{GameName: strings.TrimSpace(candidates[0].GameName), TagLine: strings.TrimSpace(candidates[0].TagLine)}, true
	}
	return riotAccount{}, false
}

// opggParseSearchResult 解析 Server Action 的 text/x-component 流：
// 每行形如 “<id>:<JSON>”，召唤师结果在带 "summoners" 键的那一行。
func opggParseSearchResult(data []byte) []riotAccount {
	for _, line := range strings.Split(string(data), "\n") {
		colon := strings.Index(line, ":")
		if colon <= 0 || !strings.Contains(line, `"summoners"`) {
			continue
		}
		var payload struct {
			Summoners []struct {
				GameName string `json:"gameName"`
				Tagline  string `json:"tagline"`
			} `json:"summoners"`
		}
		if json.Unmarshal([]byte(line[colon+1:]), &payload) != nil {
			continue
		}
		accounts := make([]riotAccount, 0, len(payload.Summoners))
		for _, entry := range payload.Summoners {
			accounts = append(accounts, riotAccount{GameName: entry.GameName, TagLine: entry.Tagline})
		}
		return accounts
	}
	return nil
}

func (p *riotProvider) loadRiotRanks(ctx context.Context, puuid string) ([]gameplayRank, EndpointCapability) {
	capability := EndpointCapability{Name: "ranked-stats", Path: "riot: /lol/league/v4/entries/by-puuid"}
	entries, err := p.leagueEntries(ctx, puuid)
	if err != nil && !errors.Is(err, errRiotNotFound) {
		capability.State = capabilityFailed
		capability.Detail = err.Error()
		return nil, capability
	}
	ranks := make([]gameplayRank, 0, 2)
	for _, entry := range entries {
		if entry.QueueType != "RANKED_SOLO_5x5" && entry.QueueType != "RANKED_FLEX_SR" {
			continue
		}
		label := "单排/双排"
		if entry.QueueType == "RANKED_FLEX_SR" {
			label = "灵活组排"
		}
		total := entry.Wins + entry.Losses
		winRate := 0
		if total > 0 {
			winRate = int(math.Round(float64(entry.Wins) * 100 / float64(total)))
		}
		ranks = append(ranks, gameplayRank{
			QueueType: entry.QueueType, QueueLabel: label,
			Tier: strings.ToLower(entry.Tier), Division: entry.Rank,
			LeaguePoints: entry.LeaguePoints, Wins: entry.Wins, Losses: entry.Losses, WinRate: winRate,
		})
	}
	capability.State = capabilityAvailable
	capability.Count = len(ranks)
	return ranks, capability
}

func (p *riotProvider) loadRiotMasteries(ctx context.Context, puuid string, names map[int64]string) ([]gameplayMastery, EndpointCapability) {
	capability := EndpointCapability{Name: "champion-mastery", Path: "riot: /lol/champion-mastery/v4/by-puuid/top"}
	entries, err := p.topMasteries(ctx, puuid, 6)
	if err != nil && !errors.Is(err, errRiotNotFound) {
		capability.State = capabilityFailed
		capability.Detail = err.Error()
		return nil, capability
	}
	source := make(map[int64]ChampionMastery, len(entries))
	for _, entry := range entries {
		source[entry.ChampionID] = ChampionMastery{ChampionID: entry.ChampionID, ChampionLevel: entry.ChampionLevel, ChampionPoints: entry.ChampionPoints, LastPlayTime: entry.LastPlayTime}
	}
	capability.State = capabilityAvailable
	capability.Count = len(source)
	return normalizeMasteries(source, names, 6), capability
}

/* ---------- championProvider 复用访问器 ---------- */

func (p *championProvider) httpClient() *http.Client {
	p.clientMu.RLock()
	defer p.clientMu.RUnlock()
	return p.client
}

// championNamesZH 返回 championId → 中文名映射；目录未加载时尝试
// 加载一次（带磁盘缓存），失败则返回空映射并由调用方降级显示。
func (p *championProvider) championNamesZH(ctx context.Context) map[int64]string {
	p.mu.Lock()
	loaded := len(p.championMeta) > 0
	p.mu.Unlock()
	if !loaded {
		loadContext, cancel := context.WithTimeout(ctx, 10*time.Second)
		_, _ = p.loadCatalog(loadContext)
		cancel()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	result := make(map[int64]string, len(p.championMeta))
	for id, meta := range p.championMeta {
		if meta.NameZH != "" {
			result[int64(id)] = meta.NameZH
		}
	}
	return result
}
