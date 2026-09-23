package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	stdhtml "html"
	"math"
	"net"
	"net/http"
	"net/url"
	pathpkg "path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/mozillazg/go-pinyin"
	xhtml "golang.org/x/net/html"
)

const (
	opggChampionHost                = "lol-api-champion.op.gg"
	opggWebAPIHost                  = "lol-web-api.op.gg"
	opggPageHost                    = "op.gg"
	qq101Host                       = "mlol.qt.qq.com"
	communityDragonHost             = "raw.communitydragon.org"
	yourGGArenaHost                 = "api.your.gg"
	opggAssetHost                   = "opgg-static.akamaized.net"
	dataDragonHost                  = "ddragon.leagueoflegends.com"
	championJSONMax                 = 6 << 20
	championHTMLMax                 = 5 << 20
	championImageMax                = 4 << 20
	championCoreRecommendationLimit = 15

	gtimgChampionIconPrefix = "/images/lol/act/img/champion/"
	gtimgSkinArtworkPrefix  = "/images/lol/act/img/skin/big"
)

var (
	championSlugPattern  = regexp.MustCompile(`^[a-z0-9]+$`)
	numberPattern        = regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?`)
	gamesPattern         = regexp.MustCompile(`([0-9][0-9,]*)\s*场`)
	markupPattern        = regexp.MustCompile(`<[^>]+>`)
	flightPattern        = regexp.MustCompile(`self\.__next_f\.push\(\[1,("(?:\\.|[^"\\])*")\]\)`)
	allowedChampionTiers = map[string]bool{
		"all": true, "challenger": true, "grandmaster": true, "master_plus": true, "master": true,
		"diamond_plus": true, "diamond": true, "emerald_plus": true, "emerald": true,
		"platinum_plus": true, "platinum": true, "gold_plus": true, "gold": true,
		"silver": true, "bronze": true, "iron": true,
	}
	championTierLabels = []championFilterOption{
		{Value: "all", Label: "全部段位"},
		{Value: "challenger", Label: "最强王者"},
		{Value: "grandmaster", Label: "傲世宗师"},
		{Value: "master_plus", Label: "超凡大师以上"},
		{Value: "master", Label: "超凡大师"},
		{Value: "diamond_plus", Label: "钻石以上"},
		{Value: "diamond", Label: "钻石"},
		{Value: "emerald_plus", Label: "翡翠以上"},
		{Value: "emerald", Label: "翡翠"},
		{Value: "platinum_plus", Label: "铂金以上"},
		{Value: "platinum", Label: "铂金"},
		{Value: "gold_plus", Label: "黄金以上"},
		{Value: "gold", Label: "黄金"},
		{Value: "silver", Label: "白银"},
		{Value: "bronze", Label: "青铜"},
		{Value: "iron", Label: "黑铁"},
	}
	championPositionNames = map[string]string{"all": "", "top": "TOP", "jungle": "JUNGLE", "mid": "MID", "adc": "ADC", "support": "SUPPORT"}
)

type championProvider struct {
	imageCache            *championDataCache
	communityImageCache   *championDataCache
	assetStats            assetFetchStats
	imageWarm             imageWarmState
	clientMu              sync.RWMutex
	client                *http.Client
	network               championNetworkSettings
	activeProxy           string
	cache                 *championDataCache
	diag                  func(event map[string]any)
	mu                    sync.Mutex
	patch                 string
	static                map[string]championAssetDescription
	itemPurchasable       map[int64]bool
	championKeys          map[string]string
	championIDs           map[string]int
	championMeta          map[int]championMetadata
	catalogNamesFailUntil time.Time
	catalogNamesFlight    chan struct{}
	abilities             map[string]map[string]championAssetDescription
	diagLast              map[string]time.Time
	yourGGMu              sync.Mutex
	yourGGLast            time.Time
	arenaItemsAt          time.Time
	arenaItems            []gameplayItem
	hexdata               *hexdataClient
	featureGates          *featureGates
	qq101ProbeMu          sync.Mutex
	qq101ProbeRunning     map[string]bool
	qq101ProbeAt          map[string]time.Time
	qq101Wait             time.Duration
	// Keep the last valid augment catalog in memory so a transient empty
	// Hexdata/OP.GG response cannot blank the atlas during refresh.
	augmentCatalogMu    sync.Mutex
	augmentCatalog      championAugmentResponse
	augmentCatalogReady bool
	// Injected by main to read the authenticated LCU augment catalog.
	// Keeping it as a callback makes the provider testable and preserves the
	// CommunityDragon fallback when the client is offline.
	gameplayAugments func(context.Context) ([]gameplayAugment, error)
}

type championFilterOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type championCatalogResponse struct {
	Source    string                 `json:"source"`
	Region    string                 `json:"region"`
	Patch     string                 `json:"patch"`
	FetchedAt time.Time              `json:"fetchedAt"`
	Tiers     []championFilterOption `json:"tiers"`
	Champions []championMetadata     `json:"champions"`
}

type championMetadata struct {
	ID          int      `json:"id"`
	Key         string   `json:"key"`
	Slug        string   `json:"slug"`
	NameZH      string   `json:"nameZh"`
	TitleZH     string   `json:"titleZh"`
	NameEN      string   `json:"nameEn"`
	TitleEN     string   `json:"titleEn"`
	ImageSource string   `json:"imageSource"`
	ImagePath   string   `json:"imagePath"`
	SearchTerms []string `json:"searchTerms"`
}

type championRankingResponse struct {
	Mode                 string                  `json:"mode"`
	Region               string                  `json:"region"`
	Tier                 string                  `json:"tier,omitempty"`
	Position             string                  `json:"position,omitempty"`
	Patch                string                  `json:"patch,omitempty"`
	Source               string                  `json:"source"`
	FetchedAt            time.Time               `json:"fetchedAt"`
	EntertainmentSample  bool                    `json:"entertainmentSample,omitempty"`
	Citation             *championSourceCitation `json:"citation,omitempty"`
	// R128 §2.3：前端不展示（界面上不再出现口径/方法论说明）；字段保留供诊断与既有后端测试使用。
	MeasurementTechnique string                  `json:"measurementTechnique,omitempty"`
	Rows                 []championRankingRow    `json:"rows"`
	TeamCompositions     []arenaTeamComposition  `json:"teamCompositions,omitempty"`
	TierBands            []championTierBand      `json:"tierBands,omitempty"`
}

type championRankingRow struct {
	ChampionID  int    `json:"championId"`
	Key         string `json:"key,omitempty"`
	Name        string `json:"name,omitempty"`
	ImageSource string `json:"imageSource,omitempty"`
	ImagePath   string `json:"imagePath,omitempty"`
	Rank        int    `json:"rank"`
	// Tier：R116-B P0-5-2 起优先取 hextech-insights.heroes[].tier（官方 T1-T5）。
	// 只有官方档位取不到时才回退到「按返回顺序本地分档」，此时
	// TierLocallyCalculated 必须为 true，前端据此显示「本地估算」小字。
	Tier                  int      `json:"tier"`
	Grade                 string   `json:"grade,omitempty"`
	Position              string   `json:"position,omitempty"`
	Positions             []string `json:"positions,omitempty"`
	Play                  int      `json:"play,omitempty"`
	WinRate               float64  `json:"winRate,omitempty"`
	PickRate              float64  `json:"pickRate,omitempty"`
	BanRate               float64  `json:"banRate,omitempty"`
	KDA                   float64  `json:"kda,omitempty"`
	AveragePlacement      float64  `json:"averagePlacement,omitempty"`
	FirstPlaceRate        float64  `json:"firstPlaceRate,omitempty"`
	TierLocallyCalculated bool     `json:"tierLocallyCalculated,omitempty"`
}

// championTierBand 是 /api/hexdata/meta 的 tierBands 原样透传（实测 5 条：
// 1=前15%、2=15%-35%、3=35%-65%、4=65%-85%、5=后15%）。R116-B P0-5 要求官方
// 档位的文案用上游给的标签，不在前端自己造一套「T1=超强」之类的解释。
// meta 拿不到时留空，前端整块不渲染。
type championTierBand struct {
	Tier  int    `json:"tier"`
	Label string `json:"label"`
}

type arenaTeamChampion struct {
	ID          int    `json:"id"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	ImageSource string `json:"imageSource"`
	ImagePath   string `json:"imagePath"`
}

type arenaTeamComposition struct {
	Champions        []arenaTeamChampion `json:"champions"`
	AveragePlacement float64             `json:"averagePlacement,omitempty"`
	FirstPlaceRate   float64             `json:"firstPlaceRate,omitempty"`
	PickRate         float64             `json:"pickRate,omitempty"`
	WinRate          float64             `json:"winRate,omitempty"`
	Games            int                 `json:"games,omitempty"`
}

type arenaChampionStats struct {
	Tier             *int    `json:"tier,omitempty"`
	Rank             int     `json:"rank,omitempty"`
	RankPrevPatch    int     `json:"rankPrevPatch,omitempty"`
	Games            int     `json:"games,omitempty"`
	KDA              float64 `json:"kda,omitempty"`
	AveragePlacement float64 `json:"averagePlacement,omitempty"`
	FirstPlaceRate   float64 `json:"firstPlaceRate,omitempty"`
	PickRate         float64 `json:"pickRate,omitempty"`
	WinRate          float64 `json:"winRate,omitempty"`
	BanRate          float64 `json:"banRate,omitempty"`
}

type championAsset struct {
	ID           int       `json:"id,omitempty"`
	Kind         string    `json:"kind"`
	Name         string    `json:"name"`
	Description  string    `json:"description,omitempty"`
	Source       string    `json:"source"`
	Path         string    `json:"path"`
	FallbackPath string    `json:"fallbackPath,omitempty"`
	Active       bool      `json:"active,omitempty"`
	CostType     string    `json:"costType,omitempty"`
	Costs        []float64 `json:"costs,omitempty"`
	Cooldowns    []float64 `json:"cooldowns,omitempty"`
	Ranges       []float64 `json:"ranges,omitempty"`
}

type championAssetDescription struct {
	Name        string
	Description string
	Source      string
	Path        string
	BuildsInto  bool
	Price       int64
	CostType    string
	Costs       []float64
	Cooldowns   []float64
	Ranges      []float64
}

type championMetricRow struct {
	Assets           []championAsset `json:"assets"`
	Ultimate         *championAsset  `json:"ultimate,omitempty"`
	Rarity           string          `json:"rarity,omitempty"`
	Tier             string          `json:"tier,omitempty"`
	Grade            string          `json:"grade,omitempty"`
	Score            float64         `json:"score,omitempty"`
	PickRate         float64         `json:"pickRate,omitempty"`
	WinRate          float64         `json:"winRate,omitempty"`
	Games            int             `json:"games,omitempty"`
	GamesUnavailable bool            `json:"gamesUnavailable,omitempty"`
	AveragePlacement float64         `json:"averagePlacement,omitempty"`
	FirstPlaceRate   float64         `json:"firstPlaceRate,omitempty"`
	SkillPriority    []string        `json:"skillPriority,omitempty"`
	SkillOrder       []string        `json:"skillOrder,omitempty"`
	// R116-A：以下字段来自 Hexdata JSON API 的官方口径，解析层原样保留。
	// R116-B P0-5-1 起 applyLocalAugmentGrades 改为「官方优先、本地兜底」：
	// HexLabel/HexTier 非空的行直接采用官方档位，本地分位只服务官方缺失的行。
	// 官方口径与本地估算靠字段是否非空区分，不混成一个字段。
	DeltaWinRate       float64 `json:"deltaWinRate,omitempty"`
	WilsonLowerWinRate float64 `json:"wilsonLowerWinRate,omitempty"`
	HexTier            string  `json:"hexTier,omitempty"`
	HexLabel           string  `json:"hexLabel,omitempty"`
	HexTierColor       string  `json:"hexTierColor,omitempty"`
	OfficialTier       int     `json:"officialTier,omitempty"`
	// 装备行专属：coreDelta = 带该装备胜率 − 不带该装备胜率；averageIndex 是
	// 平均出装顺位；withoutItemWinRate 是不出这件装备时的胜率（百分数）。
	CoreDelta          float64 `json:"coreDelta,omitempty"`
	AverageIndex       float64 `json:"averageIndex,omitempty"`
	WithoutItemWinRate float64 `json:"withoutItemWinRate,omitempty"`
	// R116-B P0-2：样本分档（"low"/"medium"/"high"），阈值从
	// /api/hexdata/meta 的 samplePolicy 读（实测 low 1..249、medium 250..999、
	// high ≥1000），后端算好直出，前端不重复实现阈值逻辑。meta 不可用或
	// samplePolicy 缺失时留空 → 前端整块不渲染（评审 6.1「取不到就整块隐藏」）。
	SampleTier string `json:"sampleTier,omitempty"`
	// R116-B P0-6：海克斯阶段（stage 1..4）明细。工单要求点阶段 chip 后
	// 「前端本地」重渲染、不重新发请求，所以四个阶段必须随详情页一次下发。
	// 只有海斗的海克斯行会有值；装备行与召唤师技能行留空（omitempty → 不下发）。
	// 上游缺某个阶段时这里也不补零行（实测阶段 1 只有 121/126 条 augment 有
	// stage 行）：前端拿不到 stage 就在该阶段视图里不渲染这条 augment，绝不用 0
	// 顶替——0 会被渲染成「胜率 0%」，那是显示不存在的数据。
	Stages []championMetricStageRow `json:"stages,omitempty"`
	// R116-F P2：负向推荐（慎选）判据命中标记，以及判据①用到的「该英雄同稀有度
	// 选取率中位数」（百分数，与 PickRate 同单位）。三条判据全在后端算：中位数的
	// 池子是「该英雄全部同稀有度海克斯」，而前端只看得到每品质前三张卡（实测英雄
	// 157 是 9/126），前端自己算出来的中位数必然是错的。判据与降级理由见
	// hexdata.go 的 markMayhemNegativeRecommendations。
	Caution               bool    `json:"caution,omitempty"`
	CautionMedianPickRate float64 `json:"cautionMedianPickRate,omitempty"`
}

// championMetricStageRow 是 augments[].stages[] 的裁剪下发版本（R116-B P0-6）。
// 上游每条 stage 行有 14 个字段，这里只保留「阶段视图要显示的 + 判断置信度要用
// 的」9 个：实测单英雄 126 条 augment × 4 阶段的 JSON 体积从 140,138 B 降到
// 102,135 B（-27%）。丢掉的 wins / tier / hexTier / hexScore /
// recommendationScore 前端一个都不渲染——hexTier 是内部枚举名（实测 "hang"），
// 给用户看的档位文案一律是 hexLabel（"夯"），所以只下发 hexLabel。
//
// R116-B 独立评审整改（B4 / B6）又调了一次这 9 个字段，一进一出体积反而更小
// （同一套静态核算：102,008 B → 97,033 B，净减 4,975 B/英雄；砍 pickRate 省
// 10,915 B，加 grade 花 5,940 B）：
//   - 去掉 PickRate：前端零渲染（阶段视图的卡片只有胜率/样本/综合评分/较基准/
//     官方档位/置信度/阶段基准七格，选取率不在其中）。R116-F 的「阶段 × 稀有度」
//     概率分布读的是上游解析结构 hexdataAugmentRowV2 的 Stages[].PickRate，不走
//     这个下发结构，所以砍掉它不影响 F。
//   - 加上 Grade：阶段视图的字母徽章必须与同一张卡上的「官方档位」同口径，否则
//     会出现「徽章 S（英雄级 hang）+ 官方档位 顶级（阶段级 top）」这种自相矛盾
//     （实测英雄 157 的 499 条阶段行里有 115 条 hexTier 与父行不同）。字母档位
//     仍然只由后端那一个官方档位映射函数算，前端不复制 hexTier/hexLabel → 字母
//     的对照表；上游给 insufficient 时映射返回空串，前端据此整块隐藏徽章。
//
// 单位与父行 championMetricRow 逐字一致，前端不需要再判断该不该换算：
//   - WinRate / PickRate / StageBaselineWinRate 是百分数（上游 0..1 已 ×100）；
//   - DeltaWinRate / WilsonLowerWinRate 保持上游 0..1 原值，展示时前端自己 ×100。
//
// SampleTier 与父行同源：由 applyHexdataSampleTiers 按 meta.samplePolicy 的阈值
// 算好直出（阶段行不按 hexdataMinimumSample 过滤，低样本阶段靠「样本极少」徽记
// 披露而不是悄悄消失，否则 P0-2 就白做了）。
//
// 展示字段一律带 omitempty，所以「这个阶段没有这个值」在 JSON 里表现为键不存在。
// 前端 mayhemAugmentStageItem 据此只覆盖阶段行真的带着的键（评审整改 A3），缺失
// 的键置 undefined 走各自的隐藏分支，绝不静默沿用父行的英雄级数值。
type championMetricStageRow struct {
	Stage                int     `json:"stage"`
	WinRate              float64 `json:"winRate,omitempty"`
	PickRate             float64 `json:"pickRate,omitempty"`
	DeltaWinRate         float64 `json:"deltaWinRate,omitempty"`
	WilsonLowerWinRate   float64 `json:"wilsonLowerWinRate,omitempty"`
	StageBaselineWinRate float64 `json:"stageBaselineWinRate,omitempty"`
	Games                int     `json:"games,omitempty"`
	HexLabel             string  `json:"hexLabel,omitempty"`
	Grade                string  `json:"grade,omitempty"`
	SampleTier           string  `json:"sampleTier,omitempty"`
}

// hexdataOfficialGrade 把 Hexdata 官方 hexTier 枚举映射到站内既有的字母档位，
// 只用于 .augment-grade.is-X 这套样式类；给用户看的文案一律是官方 hexLabel
// （中文「夯」「顶级」…），绝不展示内部枚举名 hexTier。
// 实测枚举只有六个取值：hang(夯) / top(顶级) / elite(人上人) / npc(NPC) /
// trap(拉完了) / insufficient(样本过少)。insufficient 故意不给字母档位——它是
// 上游自己的「样本不足」标记，硬套一个 S/A/B 等于替上游编一个强度评价。
func hexdataOfficialGrade(hexTier string) string {
	switch hexTier {
	case "hang":
		return "S"
	case "top":
		return "A"
	case "elite":
		return "B"
	case "npc":
		return "C"
	case "trap":
		return "F"
	default:
		return ""
	}
}

// hexdataRowHasOfficialGrade 判断这一行是否带可用的官方档位。只要官方给了
// hexLabel 或可映射的 hexTier，本地分位算法就完全不参与这一行。
func hexdataRowHasOfficialGrade(row championMetricRow) bool {
	return strings.TrimSpace(row.HexLabel) != "" || hexdataOfficialGrade(row.HexTier) != ""
}

// applyLocalAugmentGrades：R116-B P0-5-1 起是「官方优先、本地兜底」。
// 官方 hexTier/hexLabel 存在 → 直接用官方档位，不跑本地分位、也不把这行放进
// 本地分位样本池（否则官方行会抬高/压低本地行的分位）。官方字段缺失（上游
// 波动、OP.GG RSC 回退行）→ 回退到原来的本地分位算法。本地算法整段保留，
// 一条都没删：它仍是唯一的兜底。
func applyLocalAugmentGrades(rows []championMetricRow) {
	scores := make([]float64, 0, len(rows))
	for index := range rows {
		if hexdataRowHasOfficialGrade(rows[index]) {
			continue
		}
		if rows[index].Score <= 0 {
			rows[index].Score = localAugmentScore(rows[index])
		}
		if rows[index].Score > 0 {
			scores = append(scores, rows[index].Score)
		}
	}
	for index := range rows {
		if hexdataRowHasOfficialGrade(rows[index]) {
			rows[index].Grade = hexdataOfficialGrade(rows[index].HexTier)
			continue
		}
		percentile := float64(len(rows)-index) / float64(max(1, len(rows)))
		if len(scores) > 0 {
			percentile = 0
			for _, score := range scores {
				if score <= rows[index].Score {
					percentile++
				}
			}
			percentile /= float64(len(scores))
		}
		switch {
		case percentile >= 0.9:
			rows[index].Grade = "S"
		case percentile >= 0.7:
			rows[index].Grade = "A"
		default:
			rows[index].Grade = "B"
		}
	}
}

func localAugmentScore(row championMetricRow) float64 {
	if row.Score > 0 {
		return row.Score
	}
	if row.Games <= 0 || row.WinRate <= 0 || row.WinRate > 100 {
		return 0
	}
	p := row.WinRate / 100
	n := float64(row.Games)
	const z = 1.96
	return 100 * (p + z*z/(2*n) - z*math.Sqrt((p*(1-p)+z*z/(4*n))/n)) / (1 + z*z/n)
}

func appendMeasurementTechnique(value, note string) string {
	value, note = strings.TrimSpace(value), strings.TrimSpace(note)
	if value == "" {
		return note
	}
	if note == "" || strings.Contains(value, note) {
		return value
	}
	return value + "；" + note
}

type arenaAugmentGroup struct {
	Rarity int                 `json:"rarity"`
	Rows   []championMetricRow `json:"rows"`
}

type championBuildSections struct {
	SummonerSpells  []championMetricRow `json:"summonerSpells,omitempty"`
	Skills          []championMetricRow `json:"skills,omitempty"`
	StarterItems    []championMetricRow `json:"starterItems,omitempty"`
	Boots           []championMetricRow `json:"boots,omitempty"`
	CoreItems       []championMetricRow `json:"coreItems,omitempty"`
	FourthItems     []championMetricRow `json:"fourthItems,omitempty"`
	FifthItems      []championMetricRow `json:"fifthItems,omitempty"`
	SixthItems      []championMetricRow `json:"sixthItems,omitempty"`
	PrismItems      []championMetricRow `json:"prismItems,omitempty"`
	ItemSource      string              `json:"itemSource,omitempty"`
	ItemWindow      string              `json:"itemWindow,omitempty"`
	ItemChainStatus string              `json:"itemChainStatus,omitempty"`
	ItemAttempts    []DataSourceAttempt `json:"itemAttempts,omitempty"`
	ItemFallback    string              `json:"itemFallbackReason,omitempty"`
	FourthSample    int                 `json:"fourthSample,omitempty"`
	FifthSample     int                 `json:"fifthSample,omitempty"`
	SixthSample     int                 `json:"sixthSample,omitempty"`
}

type championRunePage struct {
	PrimaryStyle championAsset     `json:"primaryStyle"`
	SubStyle     championAsset     `json:"subStyle"`
	Selected     []championAsset   `json:"selected"`
	PrimarySlots [][]championAsset `json:"primarySlots,omitempty"`
	SubSlots     [][]championAsset `json:"subSlots,omitempty"`
	ShardSlots   [][]championAsset `json:"shardSlots,omitempty"`
	PickRate     float64           `json:"pickRate"`
	WinRate      float64           `json:"winRate"`
	Games        int               `json:"games"`
}

type championCounterRow struct {
	ChampionID  int     `json:"championId,omitempty"`
	Key         string  `json:"key"`
	Name        string  `json:"name"`
	ImageSource string  `json:"imageSource"`
	ImagePath   string  `json:"imagePath"`
	WinRate     float64 `json:"winRate,omitempty"`
	Games       int     `json:"games,omitempty"`
}

type championCounterSections struct {
	WeakAgainst   []championCounterRow `json:"weakAgainst,omitempty"`
	StrongAgainst []championCounterRow `json:"strongAgainst,omitempty"`
}

type championDetailStats struct {
	Tier     *int    `json:"tier,omitempty"`
	WinRate  float64 `json:"winRate,omitempty"`
	PickRate float64 `json:"pickRate,omitempty"`
	BanRate  float64 `json:"banRate,omitempty"`
}

type championPositionOption struct {
	Position string  `json:"position"`
	WinRate  float64 `json:"winRate"`
	PickRate float64 `json:"pickRate"`
	BanRate  float64 `json:"banRate"`
	RoleRate float64 `json:"roleRate"`
	Play     int     `json:"play"`
	Tier     int     `json:"tier"`
	Rank     int     `json:"rank"`
}

type championDetailResponse struct {
	Mode                 string                   `json:"mode"`
	Region               string                   `json:"region"`
	Tier                 string                   `json:"tier,omitempty"`
	Position             string                   `json:"position,omitempty"`
	Positions            []championPositionOption `json:"positions,omitempty"`
	PositionsSource      string                   `json:"positionsSource,omitempty"`
	Patch                string                   `json:"patch,omitempty"`
	CurrentPatch         string                   `json:"currentPatch,omitempty"`
	IsStale              bool                     `json:"isStale,omitempty"`
	Source               string                   `json:"source"`
	FetchedAt            time.Time                `json:"fetchedAt"`
	EntertainmentSample  bool                     `json:"entertainmentSample,omitempty"`
	Citation             *championSourceCitation  `json:"citation,omitempty"`
	BuildCitation        *championSourceCitation  `json:"buildCitation,omitempty"`
	// R128 §2.3：前端不展示；字段保留供诊断与既有后端测试使用。
	MeasurementTechnique string                   `json:"measurementTechnique,omitempty"`
	Stats                championDetailStats      `json:"stats,omitempty"`
	Runes                []championRunePage       `json:"runes,omitempty"`
	Counters             championCounterSections  `json:"counters,omitempty"`
	// SampleTier / CountersTier：所选段位样本不足时实际使用的回退段位。
	SampleTier          string                 `json:"sampleTier,omitempty"`
	CountersTier        string                 `json:"countersTier,omitempty"`
	TopPlayers          []championTopPlayer    `json:"topPlayers,omitempty"`
	RecommendedAugments []championMetricRow    `json:"recommendedAugments,omitempty"`
	ItemRanking         []championMetricRow    `json:"itemRanking,omitempty"`
	ArenaStats          arenaChampionStats     `json:"arenaStats,omitempty"`
	TeamCompositions    []arenaTeamComposition `json:"teamCompositions,omitempty"`
	ArenaAugments       []championMetricRow    `json:"arenaAugments,omitempty"`
	ArenaAugmentGroups  []arenaAugmentGroup    `json:"arenaAugmentGroups,omitempty"`
	Build               championBuildSections  `json:"build"`
	// R116-B P1-6：海斗赛后 22 项表现指标（含后端算好的「较全英雄平均」）。
	// postmatch 取不到时留 nil → 前端整个「表现」tab 不渲染。
	Performance *championPerformancePanel `json:"performance,omitempty"`
	// R116-F P1：该英雄专属的「阶段 × 稀有度」概率分布（英雄口径），由 hero-json
	// 的 augments[].stages[].pickRate 本地聚合而来，不发新请求。与全服口径的
	// /api/champions/augment-rarity（hexdataRarityStage）并存、互不替换：全服口径
	// 回答「整体分布」，这份回答「这个英雄」。取不到时留 nil → 前端整块不渲染。
	HeroStageRarity []hexdataHeroStageRarityRow `json:"heroStageRarity,omitempty"`
}

// championPerformanceMetric 是 R116-B P1-6「表现」tab 的一项赛后指标。
// 数值与「较全英雄平均」的差值全部在后端算好（工单 P1-6-2：不在前端算均值，
// 避免把 173 英雄完整表下发到前端），前端只做展示。
//
// 量纲警告（R116 实测）：/api/hexdata/postmatch 的 22 项不是同一种量纲。
//   - 18 项是每局均值（11 个 avg*）或比率/评分（damageShare、goldEfficiency、
//     kda、killParticipation、multiKillRate、survivability、utilityScore），
//     跨英雄可比 → 带 DeltaPercent。
//   - doubleKills/tripleKills/quadraKills/pentaKills 是「该英雄全部对局的累计
//     次数」，实测 pearson(总场次, doubleKills)=0.8117，对它们做 ±% 量到的是
//     人气不是强度 → Cumulative=true 且 DeltaPercent 恒为 0，前端必须去掉
//     ±% 并标注「累计次数（受出场场次影响，不可跨英雄直接比较）」。
//
// DeltaPercent=0 有两种含义（真的等于平均 / 无法计算），所以用 HasDelta 显式
// 区分；HasDelta=false 时前端不渲染「较平均」标签（评审 6.1：取不到就隐藏）。
type championPerformanceMetric struct {
	Key          string  `json:"key"`
	Label        string  `json:"label"`
	Group        string  `json:"group"`
	Value        float64 `json:"value"`
	Format       string  `json:"format,omitempty"`
	DeltaPercent float64 `json:"deltaPercent,omitempty"`
	HasDelta     bool    `json:"hasDelta,omitempty"`
	Cumulative   bool    `json:"cumulative,omitempty"`
}

// championPerformancePanel 是「表现」tab 的整块数据。HeroCount 是参与均值计算
// 的英雄数（实测 173），Averages 只在后端用于算 DeltaPercent，不下发。
type championPerformancePanel struct {
	Metrics   []championPerformanceMetric `json:"metrics"`
	HeroCount int                         `json:"heroCount,omitempty"`
	// Groups 是四组的展示顺序；某一组的全部指标都缺失时该组整块不下发，
	// 前端不留空态。
	Groups               []string                `json:"groups,omitempty"`
	Citation             *championSourceCitation `json:"citation,omitempty"`
	// R128 §2.3：前端不展示；字段保留供诊断与既有后端测试使用。
	MeasurementTechnique string                  `json:"measurementTechnique,omitempty"`
}

type championAugmentResponse struct {
	Source               string                  `json:"source"`
	Mode                 string                  `json:"mode"`
	FetchedAt            time.Time               `json:"fetchedAt"`
	EntertainmentSample  bool                    `json:"entertainmentSample"`
	Citation             *championSourceCitation `json:"citation,omitempty"`
	// R128 §2.3：前端不展示；字段保留供诊断与既有后端测试使用。
	MeasurementTechnique string                  `json:"measurementTechnique,omitempty"`
	Rows                 []championAugment       `json:"rows"`
}

type championAugment struct {
	ID                     int                       `json:"id"`
	Key                    string                    `json:"key"`
	Name                   string                    `json:"name"`
	Tier                   int                       `json:"tier"`
	Rarity                 string                    `json:"rarity"`
	Performance            float64                   `json:"performance,omitempty"`
	PerformanceUnavailable bool                      `json:"performanceUnavailable,omitempty"`
	Popularity             float64                   `json:"popularity,omitempty"`
	WinRate                float64                   `json:"winRate,omitempty"`
	Description            string                    `json:"description"`
	Tooltip                string                    `json:"tooltip,omitempty"`
	ImageSource            string                    `json:"imageSource"`
	ImagePath              string                    `json:"imagePath"`
	ImageFallbackPath      string                    `json:"imageFallbackPath,omitempty"`
	Champions              []championAugmentChampion `json:"champions,omitempty"`
}

type championAugmentChampion struct {
	ID          int     `json:"id"`
	Name        string  `json:"name,omitempty"`
	Key         string  `json:"key,omitempty"`
	ImageSource string  `json:"imageSource,omitempty"`
	ImagePath   string  `json:"imagePath,omitempty"`
	Performance float64 `json:"performance,omitempty"`
	Popularity  float64 `json:"popularity,omitempty"`
	Score       float64 `json:"score,omitempty"`
	WinRate     float64 `json:"winRate,omitempty"`
	Games       int     `json:"games,omitempty"`
}

func newChampionProvider() *championProvider {
	provider := &championProvider{
		static: make(map[string]championAssetDescription), championKeys: make(map[string]string), championIDs: make(map[string]int), championMeta: make(map[int]championMetadata),
		itemPurchasable: make(map[int64]bool), abilities: make(map[string]map[string]championAssetDescription), featureGates: newFeatureGates(), qq101Wait: qq101DetailWait,
	}
	provider.hexdata = newHexdataClient(provider, nil)
	if err := provider.setNetworkSettings(defaultChampionNetworkSettings()); err != nil {
		panic(err)
	}
	return provider
}

func newChampionHTTPClient(proxy func(*http.Request) (*url.URL, error)) *http.Client {
	dialer := &net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		MaxIdleConns:        32,
		MaxIdleConnsPerHost: 8,
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   true,
		Proxy:               proxy,
		DialContext:         dialer.DialContext,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
	}
	return &http.Client{
		Timeout:   12 * time.Second,
		Transport: transport,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) == 0 || !strings.EqualFold(request.URL.Scheme, "https") || !strings.EqualFold(request.URL.Hostname(), via[0].URL.Hostname()) {
				return errors.New("champion provider redirect rejected")
			}
			if len(via) > 3 {
				return errors.New("champion provider redirect limit exceeded")
			}
			return nil
		},
	}
}

func (p *championProvider) setNetworkSettings(settings championNetworkSettings) error {
	proxy, active, err := championProxyFor(settings)
	if err != nil {
		return err
	}
	client := newChampionHTTPClient(proxy)
	p.clientMu.Lock()
	previous := p.client
	p.client, p.network, p.activeProxy = client, settings, active
	p.clientMu.Unlock()
	if previous != nil {
		previous.CloseIdleConnections()
	}
	return nil
}

func (p *championProvider) networkStatus() championNetworkStatus {
	p.clientMu.RLock()
	defer p.clientMu.RUnlock()
	return championNetworkStatus{championNetworkSettings: p.network, Active: p.activeProxy}
}

func (a *app) championDataProvider() *championProvider {
	if a.champions != nil {
		return a.champions
	}
	return newChampionProvider()
}

func (p *championProvider) fetch(ctx context.Context, host, requestPath string, query url.Values, maxBytes int64, accept string) ([]byte, error) {
	data, _, err := p.fetchWithMetadata(ctx, host, requestPath, query, maxBytes, accept)
	return data, err
}

func (p *championProvider) fetchWithMetadata(ctx context.Context, host, requestPath string, query url.Values, maxBytes int64, accept string) ([]byte, time.Time, error) {
	return p.fetchWithMetadataCacheKey(ctx, host, requestPath, query, maxBytes, accept, "")
}

func (p *championProvider) fetchWithMetadataCacheKey(ctx context.Context, host, requestPath string, query url.Values, maxBytes int64, accept, explicitKey string) ([]byte, time.Time, error) {
	return p.fetchWithMetadataCacheKeyLoader(ctx, host, requestPath, query, maxBytes, accept, explicitKey, func(loadContext context.Context) ([]byte, error) {
		return p.fetchDirect(loadContext, host, requestPath, query, maxBytes, accept)
	})
}

func (p *championProvider) fetchWithMetadataCacheKeyLoader(ctx context.Context, host, requestPath string, query url.Values, maxBytes int64, accept, explicitKey string, loader func(context.Context) ([]byte, error)) ([]byte, time.Time, error) {
	started := time.Now()
	ttl, staleFor, persistDisk := championCachePolicy(host, requestPath, accept)
	cache := p.cache
	if strings.HasPrefix(accept, "image/") && p.imageCache != nil {
		cache = p.imageCache
	}
	if ttl <= 0 || cache == nil {
		data, err := loader(ctx)
		fetchedAt := time.Time{}
		cacheState := championCacheStateMiss
		if err != nil {
			cacheState = championCacheStateError
		} else {
			fetchedAt = started
		}
		recordAssetCacheState(ctx, cacheState)
		p.reportChampionUpstream(host, accept, data, err, cacheState, started)
		return data, fetchedAt, err
	}
	key := explicitKey
	if key == "" {
		key = championCacheKey(host, requestPath, query.Encode(), accept)
	}
	result, err := cache.loadWithStatus(ctx, key, ttl, staleFor, persistDisk, loader)
	recordAssetCacheState(ctx, result.state)
	observedErr := err
	if observedErr == nil && result.upstreamErr != nil {
		observedErr = result.upstreamErr
	}
	p.reportChampionUpstream(host, accept, result.data, observedErr, result.state, started)
	return result.data, result.fetchedAt, err
}

func (p *championProvider) reportChampionUpstream(host, accept string, data []byte, err error, cacheState string, started time.Time) {
	if p.diag == nil || isCancellation(err) || (accept != "application/json" && accept != "text/html,application/xhtml+xml") {
		return
	}
	p.diag(map[string]any{
		"event": "champion_upstream", "host": host, "status": championUpstreamHTTPStatus(err),
		"duration_ms": time.Since(started).Milliseconds(), "bytes": len(data), "cache": cacheState,
	})
}

func championUpstreamHTTPStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	var status int
	if _, scanErr := fmt.Sscanf(err.Error(), "champion provider returned HTTP %d", &status); scanErr == nil && status >= 100 && status <= 599 {
		return status
	}
	return 0
}

func championProviderErrorKind(err error) string {
	if status := championUpstreamHTTPStatus(err); status > 0 {
		return "http-" + strconv.Itoa(status)
	}
	value := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(value, "timeout") || strings.Contains(value, "deadline"):
		return "timeout"
	case strings.Contains(value, "empty response"):
		return "empty-response"
	case strings.Contains(value, "too large"):
		return "too-large"
	default:
		return "unavailable"
	}
}

func (p *championProvider) fetchDirect(ctx context.Context, host, requestPath string, query url.Values, maxBytes int64, accept string) ([]byte, error) {
	if strings.HasPrefix(accept, "image/") {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, remoteImageBudget(requestPath))
		defer cancel()
	}
	if gate := featureGateForChampionHost(host); gate != "" && !p.featureGates.enabled(gate) {
		return nil, fmt.Errorf("%s data source disabled by feature gate", gate)
	}
	if !allowedChampionHost(host) || !strings.HasPrefix(requestPath, "/") || strings.Contains(requestPath, "\\") {
		return nil, errors.New("champion provider request rejected")
	}
	cleaned := pathpkg.Clean(requestPath)
	if cleaned != requestPath || strings.Contains(cleaned, "..") {
		return nil, errors.New("champion provider path rejected")
	}
	u := url.URL{Scheme: "https", Host: host, Path: requestPath, RawQuery: query.Encode()}
	if host == yourGGArenaHost {
		if err := p.waitForYourGG(ctx); err != nil {
			return nil, err
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", accept)
	request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.6")
	request.Header.Set("User-Agent", "Deep-Legends/"+version)
	p.clientMu.RLock()
	client := p.client
	p.clientMu.RUnlock()
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("champion provider unavailable: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("champion provider returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maxBytes {
		return nil, errors.New("champion provider response is too large")
	}
	data, err := readLimited(response.Body, maxBytes)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("champion provider returned an empty response")
	}
	return data, nil
}

func allowedChampionHost(host string) bool {
	return host == opggChampionHost || host == opggWebAPIHost || host == opggPageHost || host == opggAssetHost || host == dataDragonHost || host == prestigeArtworkHost || host == communityDragonHost || host == yourGGArenaHost || host == hexdataHost || host == qq101Host
}

func (a *app) handleChampionCatalog(w http.ResponseWriter, r *http.Request) {
	response, err := a.championDataProvider().loadCatalog(r.Context())
	writeChampionResponse(w, response, err)
}

func (a *app) handleChampionRankings(w http.ResponseWriter, r *http.Request) {
	mode := normalizeInternalChampionMode(r.URL.Query().Get("mode"))
	provider := a.championDataProvider()
	var response championRankingResponse
	var err error
	switch mode {
	case "ranked":
		tier := strings.TrimSpace(r.URL.Query().Get("tier"))
		if tier == "" {
			tier = "emerald_plus"
		}
		position := strings.TrimSpace(r.URL.Query().Get("position"))
		if position == "" {
			position = "all"
		}
		if !allowedChampionTiers[tier] || championPositionNames[position] == "" && position != "all" {
			http.Error(w, "invalid champion ranking filter", http.StatusBadRequest)
			return
		}
		response, err = provider.loadRanked(r.Context(), tier, position)
	case "hextech-aram":
		response, err = provider.loadARAMRankings(r.Context())
	case "arena":
		response, err = provider.loadArenaRankings(r.Context())
	case "aram", "urf", "nexus-blitz":
		response, err = provider.loadStructuredModeRankings(r.Context(), mode)
	default:
		http.Error(w, "invalid champion mode", http.StatusBadRequest)
		return
	}
	writeChampionResponse(w, response, err)
}

func (a *app) handleChampionAugments(w http.ResponseWriter, r *http.Request) {
	response, err := a.championDataProvider().loadAugments(r.Context())
	writeChampionResponse(w, response, err)
}

func (a *app) handleChampionAugmentDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("id")))
	slug := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("slug")))
	if err != nil || id <= 0 || id > 10000 {
		http.Error(w, "invalid augment", http.StatusBadRequest)
		return
	}
	if !regexp.MustCompile(`^[a-z0-9-]+$`).MatchString(slug) {
		// OP.GG keys use a different namespace (for example ARAM_MagicMissile).
		// Let the provider resolve the entry by ID instead of sending a
		// non-canonical slug to Hexdata.
		slug = ""
	}
	response, err := a.championDataProvider().loadHexdataAugmentDetail(r.Context(), id, slug)
	writeChampionResponse(w, response, err)
}

func sanitizeAugmentSlug(value string) string {
	value = strings.TrimSpace(value)
	value = regexp.MustCompile(`(?i)^aram[_-]`).ReplaceAllString(value, "")
	value = regexp.MustCompile(`([a-z0-9])([A-Z])`).ReplaceAllString(value, `${1}-${2}`)
	value = strings.NewReplacer("_", "-", " ", "-").Replace(value)
	value = strings.ToLower(value)
	value = regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(value, "-")
	return strings.Trim(regexp.MustCompile(`-+`).ReplaceAllString(value, "-"), "-")
}

func (a *app) handleChampionAugmentRarity(w http.ResponseWriter, r *http.Request) {
	response, err := a.championDataProvider().loadHexdataRarity(r.Context())
	writeChampionResponse(w, response, err)
}

func (a *app) handleChampionDetail(w http.ResponseWriter, r *http.Request) {
	mode := normalizeInternalChampionMode(r.URL.Query().Get("mode"))
	champion := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("champion")))
	position := strings.TrimSpace(r.URL.Query().Get("position"))
	tier := strings.TrimSpace(r.URL.Query().Get("tier"))
	if !championSlugPattern.MatchString(champion) {
		http.Error(w, "invalid champion", http.StatusBadRequest)
		return
	}
	spec, supported := opggModeSpecs[mode]
	if mode == "ranked" {
		if !allowedChampionTiers[tier] || position == "all" || championPositionNames[position] == "" {
			http.Error(w, "invalid champion detail filter", http.StatusBadRequest)
			return
		}
	} else if !supported {
		http.Error(w, "invalid champion mode", http.StatusBadRequest)
		return
	} else {
		position = spec.requestPosition("")
		tier = ""
	}
	response, err := a.championDataProvider().loadDetail(r.Context(), mode, champion, position, tier)
	writeChampionResponse(w, response, err)
}

func (a *app) handleArenaFirstPlaces(w http.ResponseWriter, r *http.Request) {
	championID, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("championId")))
	if err != nil || championID <= 0 || championID > 10000 {
		http.Error(w, "invalid champion", http.StatusBadRequest)
		return
	}
	limit := 5
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 10 {
			http.Error(w, "invalid limit", http.StatusBadRequest)
			return
		}
	}
	response, err := a.championDataProvider().loadArenaFirstPlaces(r.Context(), championID, limit)
	if err != nil {
		writeArenaFirstPlacesError(w, err)
		return
	}
	writeChampionResponse(w, response, err)
}

func writeArenaFirstPlacesError(w http.ResponseWriter, err error) {
	message := "高手对局读取失败，请稍后重试"
	status := http.StatusBadGateway
	value := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(value, "timeout") || strings.Contains(value, "deadline"):
		message, status = "YOUR.GG 请求超时，请稍后重试", http.StatusGatewayTimeout
	case strings.Contains(value, "http 403"):
		message = "YOUR.GG 拒绝了本次请求（HTTP 403）"
	case strings.Contains(value, "http 404"):
		message = "YOUR.GG 暂未提供该英雄的数据（HTTP 404）"
	case strings.Contains(value, "empty response"):
		message = "YOUR.GG 返回了空响应，请稍后重试"
	case strings.Contains(value, "数据格式"):
		message = "YOUR.GG 返回的数据格式暂时无法识别"
	}
	http.Error(w, message, status)
}

func (a *app) handleChampionAsset(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), publicImageTimeout)
	defer cancel()
	r = r.WithContext(ctx)
	a.scheduleItemIconWarmup(false)
	source := strings.TrimSpace(r.URL.Query().Get("source"))
	requestPath := strings.TrimSpace(r.URL.Query().Get("path"))
	host, ok := validateChampionAssetPath(source, requestPath)
	if !ok {
		http.Error(w, "invalid champion asset", http.StatusBadRequest)
		return
	}
	provider := a.championDataProvider()
	if source == "communitydragon" {
		candidates := communityDragonChampionAssetCandidates(requestPath)
		// R127 P1-a.4：逐候选记下本机客户端的真实响应状态，出网时一并落日志，
		// 这样才能判断「本机能取到的就不该出网」到底有没有做到。
		lcuStatuses := make([]int, len(candidates))
		// Exhaust cheap local choices first; remote variants race under ONE deadline.
		for index, candidatePath := range candidates {
			data, lcuStatus, ok := a.clientAssetStatus(ctx, provider, source, candidatePath)
			lcuStatuses[index] = lcuStatus
			if ok {
				provider.reportAugmentIconFetch(requestPath, http.StatusOK, index, index > 0, lcuStatus)
				writeChampionAssetImage(w, data)
				return
			}
		}
		type assetResult struct {
			data  []byte
			index int
		}
		// Keep colored large/unsuffixed artwork ahead of small monochrome icons.
		// Race equivalent variants in each quality tier within the same deadline.
		for _, small := range []bool{false, true} {
			results := make(chan assetResult, len(candidates))
			count := 0
			for index, candidatePath := range candidates {
				if strings.HasSuffix(candidatePath, "_small.png") != small {
					continue
				}
				count++
				go func(index int, candidatePath string) {
					data, _ := a.loadChampionRemoteAsset(ctx, provider, source, host, candidatePath)
					results <- assetResult{data, index}
				}(index, candidatePath)
			}
			for n := 0; n < count; n++ {
				select {
				case result := <-results:
					if len(result.data) > 0 && writeChampionAssetImage(w, result.data) {
						provider.reportAugmentIconFetch(requestPath, http.StatusOK, result.index, result.index > 0, lcuStatusAt(lcuStatuses, result.index))
						return
					}
				case <-ctx.Done():
					http.NotFound(w, r)
					return
				}
			}
		}
		provider.reportAugmentIconFetch(requestPath, http.StatusNotFound, -1, len(candidates) > 1, lcuStatusAt(lcuStatuses, 0))
		http.NotFound(w, r)
		return
	}
	if data, ok := a.loadChampionAssetFromClient(r.Context(), provider, source, requestPath); ok {
		writeChampionAssetImage(w, data)
		return
	}
	data, err := a.loadChampionRemoteAsset(r.Context(), provider, source, host, requestPath)
	if err != nil {
		if fallbackHost, fallbackPath, ok := provider.championAssetFallback(source, requestPath); ok {
			data, err = a.loadChampionRemoteAsset(r.Context(), provider, "fallback", fallbackHost, fallbackPath)
		}
	}
	if err != nil || !writeChampionAssetImage(w, data) {
		http.NotFound(w, r)
	}
}

func communityDragonChampionAssetCandidates(requestPath string) []string {
	lower := strings.ToLower(strings.TrimSpace(requestPath))
	if !strings.HasSuffix(lower, "_large.png") {
		return []string{requestPath}
	}
	var relative string
	switch {
	case strings.HasPrefix(lower, "/latest/game/"):
		relative = strings.TrimPrefix(lower, "/latest/game/")
	case strings.HasPrefix(lower, "/latest/plugins/rcp-be-lol-game-data/global/default/"):
		relative = strings.TrimPrefix(lower, "/latest/plugins/rcp-be-lol-game-data/global/default/")
	default:
		return []string{requestPath}
	}
	if candidates := communityDragonImagePaths("/lol-game-data/assets/" + relative); len(candidates) > 0 {
		return candidates
	}
	return []string{requestPath}
}

func (p *championProvider) reportAugmentIconFetch(requestPath string, status, candidateIndex int, fellBack bool, lcuStatus int) {
	pathTemplate, ok := augmentIconPathTemplate(requestPath)
	if p == nil || p.diag == nil || !ok {
		return
	}
	p.diag(map[string]any{
		"event": "augment_icon_fetch", "source": "communitydragon", "path_template": pathTemplate,
		"status": status, "candidate_index": candidateIndex, "fell_back": fellBack,
		// R127 P1-a.4：本机客户端对这张图的真实回应。0 = 没有 LCU 映射或未连接
		// 客户端（根本没问）；-1 = 问了但没有 HTTP 状态；其它 = LCU 状态码，
		// 400/404 就说明客户端确实没打包这张图，出网是必要的。
		"lcu_status": lcuStatus,
	})
}

func augmentIconPathTemplate(requestPath string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(requestPath))
	delivery := ""
	switch {
	case strings.HasPrefix(lower, "/latest/game/"):
		delivery = "game"
	case strings.HasPrefix(lower, "/latest/plugins/rcp-be-lol-game-data/global/default/"):
		delivery = "plugin"
	case strings.HasPrefix(lower, "/lol-game-data/assets/assets/"):
		// 本机客户端（LCU）路径里的游戏侧资源，对应 CommunityDragon 的 /latest/game/。
		delivery = "client-game"
	case strings.HasPrefix(lower, "/lol-game-data/assets/"):
		delivery = "client"
	default:
		return "", false
	}
	family := ""
	switch {
	case strings.Contains(lower, "/ux/cherry/augments/"):
		family = "cherry"
	case strings.Contains(lower, "/ux/kiwi/augments/"):
		family = "kiwi"
	default:
		return "", false
	}
	suffix := ""
	switch {
	case strings.HasSuffix(lower, "_large.png"):
		suffix = "_large"
	case strings.HasSuffix(lower, "_small.png"):
		suffix = "_small"
	case strings.HasSuffix(lower, ".png"):
	default:
		return "", false
	}
	return delivery + "/ux/" + family + "/augments/icons/{icon}" + suffix + ".png", true
}

func writeChampionAssetImage(w http.ResponseWriter, data []byte) bool {
	contentType := http.DetectContentType(data)
	if !strings.HasPrefix(contentType, "image/") {
		return false
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
	return true
}

// loadChampionAssetFromClient serves champion icons and skin artwork from the
// logged-in client first, so a connected session never leaves the machine for
// assets the client already ships with its own version.
func (a *app) loadChampionAssetFromClient(ctx context.Context, provider *championProvider, source, requestPath string) ([]byte, bool) {
	data, _, ok := a.clientAssetStatus(ctx, provider, source, requestPath)
	return data, ok
}

// clientAssetStatus 与 loadChampionAssetFromClient 相同，但额外带出本机客户端的
// 真实响应状态（R127 P1-a.4）。没有这个状态就分不清「客户端没打包这张图」
// （LCU 400/404）和「我们根本没去问」（没有路径映射或未连接客户端），
// Kiwi/Augments/Icons 为什么出网也就永远查不清。
func (a *app) clientAssetStatus(ctx context.Context, provider *championProvider, source, requestPath string) ([]byte, int, bool) {
	lcuPath, ok := provider.championAssetLCUPath(source, requestPath)
	if !ok {
		return nil, 0, false
	}
	a.mu.RLock()
	client := a.lcu
	connected := a.connected
	a.mu.RUnlock()
	if client == nil || !connected {
		return nil, 0, false
	}
	data, err := a.loadAsset(ctx, lcuPath, championImageMax, 0, func(loadContext context.Context) ([]byte, error) {
		return client.GetBytesContext(loadContext, lcuPath)
	})
	if err != nil {
		return nil, lcuFailureStatus(err), false
	}
	if !strings.HasPrefix(http.DetectContentType(data), "image/") {
		return nil, -1, false
	}
	return data, http.StatusOK, true
}

// lcuFailureStatus 把 LCU 错误折算成可落日志的状态码：有 HTTP 状态就用它，
// 否则 -1（超时、内容不是图片、命中失败缓存等都没有状态码）。
func lcuFailureStatus(err error) int {
	var httpError *LCUHTTPError
	if errors.As(err, &httpError) {
		return httpError.StatusCode
	}
	return -1
}

func lcuStatusAt(statuses []int, index int) int {
	if index < 0 || index >= len(statuses) {
		return 0
	}
	return statuses[index]
}

func validateChampionAssetPath(source, requestPath string) (string, bool) {
	if requestPath == "" || strings.Contains(requestPath, "\\") || strings.Contains(requestPath, "?") || pathpkg.Clean(requestPath) != requestPath {
		return "", false
	}
	lower := strings.ToLower(requestPath)
	if !strings.HasSuffix(lower, ".png") && !strings.HasSuffix(lower, ".jpg") && !strings.HasSuffix(lower, ".jpeg") && !strings.HasSuffix(lower, ".webp") {
		return "", false
	}
	switch source {
	case "opgg":
		return opggAssetHost, strings.HasPrefix(requestPath, "/meta/images/")
	case "ddragon":
		return dataDragonHost, strings.HasPrefix(requestPath, "/cdn/")
	case "communitydragon":
		const masteryCrestPrefix = "/latest/plugins/rcp-fe-lol-collections/global/default/images/item-element/crest-and-banner-mastery-"
		return communityDragonHost, strings.HasPrefix(requestPath, "/latest/game/") || strings.HasPrefix(requestPath, "/latest/plugins/rcp-be-lol-game-data/global/default/") || strings.HasPrefix(requestPath, masteryCrestPrefix)
	case "gtimg":
		return prestigeArtworkHost, strings.HasPrefix(requestPath, gtimgChampionIconPrefix) || strings.HasPrefix(requestPath, gtimgSkinArtworkPrefix)
	default:
		return "", false
	}
}

// parseGtimgAssetRef recognises the two Tencent CDN layouts used by the
// champion page: champion square icons keyed by English champion key, and
// horizontal skin artwork keyed by numeric skin ID (championID*1000+num).
func parseGtimgAssetRef(source, requestPath string) (string, int, bool) {
	if source != "gtimg" {
		return "", 0, false
	}
	if name, ok := strings.CutPrefix(requestPath, gtimgChampionIconPrefix); ok {
		key, ok := strings.CutSuffix(name, ".png")
		if !ok || key == "" || strings.Contains(key, "/") {
			return "", 0, false
		}
		return key, 0, true
	}
	if raw, ok := strings.CutPrefix(requestPath, gtimgSkinArtworkPrefix); ok {
		digits, ok := strings.CutSuffix(raw, ".jpg")
		if !ok {
			return "", 0, false
		}
		skinID, err := strconv.Atoi(digits)
		if err != nil || skinID < 1000 {
			return "", 0, false
		}
		return "", skinID, true
	}
	return "", 0, false
}

// championAssetFallback maps a failed Tencent CDN request onto the matching
// Data Dragon asset so champion images stay available when gtimg is
// unreachable or does not know a champion key.
func (p *championProvider) championAssetFallback(source, requestPath string) (string, string, bool) {
	key, skinID, ok := parseGtimgAssetRef(source, requestPath)
	if !ok {
		return "", "", false
	}
	p.mu.Lock()
	patch := p.patch
	meta := p.championMeta[skinID/1000]
	p.mu.Unlock()
	if key != "" {
		if patch == "" {
			return "", "", false
		}
		return dataDragonHost, "/cdn/" + patch + "/img/champion/" + key + ".png", true
	}
	if meta.Key == "" {
		return "", "", false
	}
	return dataDragonHost, "/cdn/img/champion/splash/" + meta.Key + "_" + strconv.Itoa(skinID%1000) + ".jpg", true
}

func (p *championProvider) championAssetLCUPath(source, requestPath string) (string, bool) {
	if source == "communitydragon" {
		if relative, ok := strings.CutPrefix(requestPath, "/latest/game/assets/"); ok && relative != "" {
			return "/lol-game-data/assets/ASSETS/" + relative, true
		}
		if relative, ok := strings.CutPrefix(requestPath, "/latest/plugins/rcp-be-lol-game-data/global/default/"); ok && relative != "" {
			return "/lol-game-data/assets/" + relative, true
		}
		return "", false
	}
	key, skinID, ok := parseGtimgAssetRef(source, requestPath)
	if !ok {
		return "", false
	}
	if key != "" {
		p.mu.Lock()
		id := p.championIDs[strings.ToLower(key)]
		p.mu.Unlock()
		if id <= 0 {
			return "", false
		}
		return "/lol-game-data/assets/v1/champion-icons/" + strconv.Itoa(id) + ".png", true
	}
	return "/lol-game-data/assets/v1/champion-splashes/" + strconv.Itoa(skinID/1000) + "/" + strconv.Itoa(skinID) + ".jpg", true
}

func writeChampionResponse(w http.ResponseWriter, value any, err error) {
	if err != nil {
		http.Error(w, "英雄数据暂时不可用，请稍后重试", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(value)
}

type ddragonChampionList struct {
	Version string                     `json:"version"`
	Data    map[string]ddragonChampion `json:"data"`
}

type ddragonChampion struct {
	ID    string `json:"id"`
	Key   string `json:"key"`
	Name  string `json:"name"`
	Title string `json:"title"`
	Image struct {
		Full string `json:"full"`
	} `json:"image"`
}

type ddragonAssetList struct {
	Data map[string]ddragonAsset `json:"data"`
}

type ddragonAsset struct {
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Tooltip     string    `json:"tooltip"`
	Cooldown    []float64 `json:"cooldown"`
	Cost        []float64 `json:"cost"`
	Range       []float64 `json:"range"`
	Into        []string  `json:"into"`
	InStore     *bool     `json:"inStore"`
	HideFromAll *bool     `json:"hideFromAll"`
	Gold        struct {
		Total       int64 `json:"total"`
		Purchasable *bool `json:"purchasable"`
	} `json:"gold"`
	Image struct {
		Full string `json:"full"`
	} `json:"image"`
}

type ddragonChampionDetail struct {
	Data map[string]struct {
		Partype string `json:"partype"`
		Spells  []struct {
			ID          string    `json:"id"`
			Name        string    `json:"name"`
			Description string    `json:"description"`
			Tooltip     string    `json:"tooltip"`
			CostType    string    `json:"costType"`
			Cost        []float64 `json:"cost"`
			Cooldown    []float64 `json:"cooldown"`
			Range       []float64 `json:"range"`
			Image       struct {
				Full string `json:"full"`
			} `json:"image"`
		} `json:"spells"`
	} `json:"data"`
}

type ddragonRuneStyle struct {
	ID    int    `json:"id"`
	Key   string `json:"key"`
	Name  string `json:"name"`
	Icon  string `json:"icon"`
	Slots []struct {
		Runes []struct {
			ID        int    `json:"id"`
			Name      string `json:"name"`
			ShortDesc string `json:"shortDesc"`
			LongDesc  string `json:"longDesc"`
			Icon      string `json:"icon"`
		} `json:"runes"`
	} `json:"slots"`
}

// Supplementary overview APIs may run without a successful catalog request.
// Keep identity verification strict, but recover a cold/failed initial load.
func (p *championProvider) ensureChampionMetadata(ctx context.Context) error {
	p.mu.Lock()
	ready := len(p.championMeta) > 0
	p.mu.Unlock()
	if ready {
		return nil
	}
	_, err := p.loadCatalog(ctx)
	return err
}

func (p *championProvider) loadCatalog(ctx context.Context) (championCatalogResponse, error) {
	versionData, err := p.fetch(ctx, dataDragonHost, "/api/versions.json", nil, 1<<20, "application/json")
	if err != nil {
		return championCatalogResponse{}, err
	}
	var versions []string
	if err := json.Unmarshal(versionData, &versions); err != nil || len(versions) == 0 || !validDDragonVersion(versions[0]) {
		return championCatalogResponse{}, errors.New("Data Dragon version response changed")
	}
	patch := versions[0]
	p.mu.Lock()
	p.patch = patch
	p.mu.Unlock()
	zhBytes, err := p.fetch(ctx, dataDragonHost, "/cdn/"+patch+"/data/zh_CN/champion.json", nil, championJSONMax, "application/json")
	if err != nil {
		return championCatalogResponse{}, err
	}
	enBytes, err := p.fetch(ctx, dataDragonHost, "/cdn/"+patch+"/data/en_US/champion.json", nil, championJSONMax, "application/json")
	if err != nil {
		return championCatalogResponse{}, err
	}
	var zh, en ddragonChampionList
	if json.Unmarshal(zhBytes, &zh) != nil || json.Unmarshal(enBytes, &en) != nil || len(zh.Data) < 100 || len(en.Data) < 100 {
		return championCatalogResponse{}, errors.New("Data Dragon champion response changed")
	}
	champions := make([]championMetadata, 0, len(zh.Data))
	championKeys := make(map[string]string, len(zh.Data)*2)
	championIDs := make(map[string]int, len(zh.Data)*2)
	championMeta := make(map[int]championMetadata, len(zh.Data))
	for key, item := range zh.Data {
		id, parseErr := strconv.Atoi(item.Key)
		if parseErr != nil || id <= 0 || item.ID == "" || item.Name == "" {
			continue
		}
		english := en.Data[key]
		terms := []string{item.Name, item.Title, english.Name, english.Title, item.ID, strings.ToLower(item.ID)}
		terms = append(terms, championPinyinTerms(item.Name)...)
		terms = append(terms, championPinyinTerms(item.Title)...)
		terms = append(terms, championAliases[strings.ToLower(item.ID)]...)
		metadata := championMetadata{
			ID: id, Key: item.ID, Slug: championOPGGSlug(item.ID), NameZH: item.Name, TitleZH: item.Title,
			NameEN: english.Name, TitleEN: english.Title, ImageSource: "gtimg",
			ImagePath:   gtimgChampionIconPrefix + item.ID + ".png",
			SearchTerms: uniqueStrings(terms),
		}
		champions = append(champions, metadata)
		championKeys[strings.ToLower(item.ID)] = item.ID
		championKeys[championOPGGSlug(item.ID)] = item.ID
		championIDs[strings.ToLower(item.ID)] = id
		championIDs[championOPGGSlug(item.ID)] = id
		championMeta[id] = metadata
	}
	sort.Slice(champions, func(i, j int) bool { return champions[i].NameZH < champions[j].NameZH })
	p.mu.Lock()
	p.championKeys = championKeys
	p.championIDs = championIDs
	p.championMeta = championMeta
	p.mu.Unlock()
	return championCatalogResponse{Source: "Riot Data Dragon", Region: "KR", Patch: patch, FetchedAt: time.Now(), Tiers: championTierLabels, Champions: champions}, nil
}

func (p *championProvider) decorateDetailAssets(ctx context.Context, champion string, response *championDetailResponse) {
	descriptions, err := p.loadStaticDescriptions(ctx)
	if err == nil && len(descriptions) > 0 {
		sections := []*[]championMetricRow{
			&response.Build.SummonerSpells, &response.Build.Skills, &response.Build.StarterItems,
			&response.Build.Boots, &response.Build.CoreItems, &response.Build.FourthItems,
			&response.Build.FifthItems, &response.Build.SixthItems, &response.Build.PrismItems,
			&response.ArenaAugments,
		}
		for _, section := range sections {
			for rowIndex := range *section {
				for assetIndex := range (*section)[rowIndex].Assets {
					decorateChampionAsset(&(*section)[rowIndex].Assets[assetIndex], descriptions)
				}
			}
		}
		response.Build.CoreItems = filterChampionItemComponents(response.Build.CoreItems, descriptions)
		for pageIndex := range response.Runes {
			page := &response.Runes[pageIndex]
			decorateChampionAsset(&page.PrimaryStyle, descriptions)
			decorateChampionAsset(&page.SubStyle, descriptions)
			for assetIndex := range page.Selected {
				decorateChampionAsset(&page.Selected[assetIndex], descriptions)
			}
			for _, slots := range [][][]championAsset{page.PrimarySlots, page.SubSlots, page.ShardSlots} {
				for slotIndex := range slots {
					for assetIndex := range slots[slotIndex] {
						decorateChampionAsset(&slots[slotIndex][assetIndex], descriptions)
					}
				}
			}
		}
	}
	abilities, abilityErr := p.loadChampionAbilityDescriptions(ctx, champion)
	if abilityErr == nil {
		decorateChampionSkills(response.Build.Skills, abilities)
	}
}

func (p *championProvider) loadStaticDescriptions(ctx context.Context) (map[string]championAssetDescription, error) {
	p.mu.Lock()
	if len(p.static) > 0 {
		result := cloneAssetDescriptions(p.static)
		p.mu.Unlock()
		return result, nil
	}
	patch := p.patch
	p.mu.Unlock()
	if !validDDragonVersion(patch) {
		versionsData, err := p.fetch(ctx, dataDragonHost, "/api/versions.json", nil, 1<<20, "application/json")
		if err != nil {
			return nil, err
		}
		var versions []string
		if json.Unmarshal(versionsData, &versions) != nil || len(versions) == 0 || !validDDragonVersion(versions[0]) {
			return nil, errors.New("Data Dragon version response changed")
		}
		patch = versions[0]
	}
	descriptions := make(map[string]championAssetDescription)
	itemPurchasable := make(map[int64]bool)
	for _, endpoint := range []struct {
		path string
		kind string
	}{{"/cdn/" + patch + "/data/zh_CN/item.json", "item"}, {"/cdn/" + patch + "/data/zh_CN/summoner.json", "spell"}} {
		data, err := p.fetch(ctx, dataDragonHost, endpoint.path, nil, championJSONMax, "application/json")
		if err != nil {
			continue
		}
		var catalog ddragonAssetList
		if json.Unmarshal(data, &catalog) != nil {
			continue
		}
		if endpoint.kind == "item" {
			goldPurchasablePresent, inStorePresent, hideFromAllPresent := 0, 0, 0
			for _, item := range catalog.Data {
				if item.Gold.Purchasable != nil {
					goldPurchasablePresent++
				}
				if item.InStore != nil {
					inStorePresent++
				}
				if item.HideFromAll != nil {
					hideFromAllPresent++
				}
			}
			if p.diag != nil {
				p.diag(map[string]any{
					"event": "ddragon_item_purchasability_shape", "items": len(catalog.Data),
					"gold_purchasable_present": goldPurchasablePresent, "in_store_present": inStorePresent,
					"hide_from_all_present": hideFromAllPresent,
				})
			}
		}
		for id, item := range catalog.Data {
			if endpoint.kind == "item" {
				numericID, parseErr := strconv.ParseInt(strings.TrimSpace(id), 10, 64)
				if purchasable, known := ddragonItemPurchasability(item); parseErr == nil && numericID > 0 && known {
					itemPurchasable[numericID] = purchasable
				}
			}
			if item.Image.Full == "" {
				continue
			}
			descriptions[endpoint.kind+"/"+strings.ToLower(item.Image.Full)] = championAssetDescription{
				Name: item.Name, Description: cleanMarkup(firstNonEmpty(item.Description, item.Tooltip)),
				Source: "ddragon", Path: "/cdn/" + patch + "/img/" + endpoint.kind + "/" + item.Image.Full,
				BuildsInto: len(item.Into) > 0, Price: item.Gold.Total,
				Costs: cloneNumbers(item.Cost), Cooldowns: cloneNumbers(item.Cooldown), Ranges: cloneNumbers(item.Range),
			}
			if endpoint.kind == "spell" {
				// summoner.json 的 map 键是 "SummonerFlash" 这类英文名，数字 ID 在
				// 条目的 key 字段里；OP.GG 结构化数据按数字 ID 引用召唤师技能。
				numericID := strings.TrimSpace(item.Key)
				if numericID == "" {
					numericID = id
				}
				descriptions["spell-id/"+numericID] = descriptions[endpoint.kind+"/"+strings.ToLower(item.Image.Full)]
			}
		}
	}
	runeData, err := p.fetch(ctx, dataDragonHost, "/cdn/"+patch+"/data/zh_CN/runesReforged.json", nil, championJSONMax, "application/json")
	if err == nil {
		var styles []ddragonRuneStyle
		if json.Unmarshal(runeData, &styles) == nil {
			for _, style := range styles {
				descriptions["perk-style/"+strconv.Itoa(style.ID)] = championAssetDescription{Name: style.Name, Source: "ddragon", Path: "/cdn/img/" + strings.TrimPrefix(style.Icon, "/")}
				for _, slot := range style.Slots {
					for _, rune := range slot.Runes {
						descriptions["perk/"+strconv.Itoa(rune.ID)] = championAssetDescription{
							Name: rune.Name, Description: cleanMarkup(firstNonEmpty(rune.LongDesc, rune.ShortDesc)), Source: "ddragon", Path: "/cdn/img/" + strings.TrimPrefix(rune.Icon, "/"),
						}
					}
				}
			}
		}
	}
	if len(descriptions) == 0 {
		return nil, errors.New("Data Dragon static descriptions unavailable")
	}
	p.mu.Lock()
	p.patch = patch
	p.static = cloneAssetDescriptions(descriptions)
	p.itemPurchasable = itemPurchasable
	p.mu.Unlock()
	return descriptions, nil
}

func ddragonItemPurchasability(item ddragonAsset) (bool, bool) {
	// Data Dragon's explicit gold.purchasable flag is the authoritative store
	// decision. Older payloads may also expose inStore/hideFromAll, but those
	// fields can describe visibility rather than whether the item is buyable.
	if item.Gold.Purchasable != nil {
		return *item.Gold.Purchasable, true
	}
	purchasable, known := true, false
	if item.InStore != nil {
		purchasable, known = purchasable && *item.InStore, true
	}
	if item.HideFromAll != nil {
		purchasable, known = purchasable && !*item.HideFromAll, true
	}
	return purchasable, known
}

func (p *championProvider) itemPurchasabilitySnapshot() map[int64]bool {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	result := make(map[int64]bool, len(p.itemPurchasable))
	for id, purchasable := range p.itemPurchasable {
		result[id] = purchasable
	}
	return result
}

func (p *championProvider) currentPatch() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.patch
}

func cloneAssetDescriptions(source map[string]championAssetDescription) map[string]championAssetDescription {
	result := make(map[string]championAssetDescription, len(source))
	for key, value := range source {
		result[key] = cloneAssetDescription(value)
	}
	return result
}

func cloneAssetDescription(value championAssetDescription) championAssetDescription {
	value.Costs = cloneNumbers(value.Costs)
	value.Cooldowns = cloneNumbers(value.Cooldowns)
	value.Ranges = cloneNumbers(value.Ranges)
	return value
}

func cloneNumbers(values []float64) []float64 {
	return append([]float64(nil), values...)
}

func decorateChampionAsset(asset *championAsset, descriptions map[string]championAssetDescription) {
	if asset == nil {
		return
	}
	key := asset.Kind + "/" + strings.ToLower(pathpkg.Base(asset.Path))
	if asset.Kind == "perk" && asset.ID > 0 {
		key = "perk/" + strconv.Itoa(asset.ID)
	} else if asset.Kind == "spell" && asset.ID > 0 {
		key = "spell-id/" + strconv.Itoa(asset.ID)
	}
	if item, ok := descriptions[key]; ok {
		if item.Name != "" {
			asset.Name = item.Name
		}
		asset.Description = item.Description
		if item.Source != "" && item.Path != "" {
			asset.Source, asset.Path = item.Source, item.Path
		}
		asset.CostType = item.CostType
		asset.Costs = cloneNumbers(item.Costs)
		asset.Cooldowns = cloneNumbers(item.Cooldowns)
		asset.Ranges = cloneNumbers(item.Ranges)
	}
}

// OP.GG occasionally includes a component such as Tear as the first step of a
// core route. Data Dragon's `into` relation is the authoritative signal that
// an item still builds into another item; unknown assets are retained.
func filterChampionItemComponents(rows []championMetricRow, descriptions map[string]championAssetDescription) []championMetricRow {
	result := make([]championMetricRow, 0, len(rows))
	for _, row := range rows {
		assets := make([]championAsset, 0, len(row.Assets))
		for _, asset := range row.Assets {
			key := asset.Kind + "/" + strings.ToLower(pathpkg.Base(asset.Path))
			if item, known := descriptions[key]; asset.Kind == "item" && known && item.BuildsInto {
				continue
			}
			assets = append(assets, asset)
		}
		if len(assets) == 0 {
			continue
		}
		row.Assets = assets
		result = append(result, row)
	}
	return result
}

func (p *championProvider) loadChampionAbilityDescriptions(ctx context.Context, champion string) (map[string]championAssetDescription, error) {
	lookup := strings.ToLower(strings.TrimSpace(champion))
	p.mu.Lock()
	key := p.championKeys[lookup]
	patch := p.patch
	cacheKey := patch + "/" + strings.ToLower(key)
	if cached := p.abilities[cacheKey]; key != "" && len(cached) > 0 {
		result := cloneAssetDescriptions(cached)
		p.mu.Unlock()
		return result, nil
	}
	p.mu.Unlock()
	if key == "" {
		key = map[string]string{"wukong": "MonkeyKing"}[lookup]
	}
	if key == "" || !validDDragonVersion(patch) {
		return nil, errors.New("Data Dragon champion metadata unavailable")
	}
	// fetchDirect 会把整个 Path 转义一次，这里必须传原始 key，
	// 预先 PathEscape 会造成双重编码。
	data, err := p.fetch(ctx, dataDragonHost, "/cdn/"+patch+"/data/zh_CN/champion/"+key+".json", nil, championJSONMax, "application/json")
	if err != nil {
		return nil, err
	}
	result, err := parseChampionAbilityDescriptions(data, key, patch)
	if err != nil {
		return nil, err
	}
	cacheKey = patch + "/" + strings.ToLower(key)
	p.mu.Lock()
	p.abilities[cacheKey] = cloneAssetDescriptions(result)
	p.mu.Unlock()
	return result, nil
}

func parseChampionAbilityDescriptions(data []byte, key string, patchValues ...string) (map[string]championAssetDescription, error) {
	patch := ""
	if len(patchValues) > 0 {
		patch = patchValues[0]
	}
	var payload ddragonChampionDetail
	if json.Unmarshal(data, &payload) != nil {
		return nil, errors.New("Data Dragon champion detail response changed")
	}
	detail, ok := payload.Data[key]
	if !ok {
		for _, candidate := range payload.Data {
			detail, ok = candidate, true
			break
		}
	}
	if !ok || len(detail.Spells) < 3 {
		return nil, errors.New("Data Dragon champion abilities unavailable")
	}
	result := make(map[string]championAssetDescription, len(detail.Spells))
	for index, spell := range detail.Spells {
		if index >= len([]string{"Q", "W", "E", "R"}) {
			break
		}
		slot := []string{"Q", "W", "E", "R"}[index]
		costType := cleanMarkup(spell.CostType)
		if costType == "" || strings.Contains(costType, "{{") {
			costType = cleanMarkup(detail.Partype)
		}
		result[slot] = championAssetDescription{
			Name: spell.Name, Description: cleanMarkup(firstNonEmpty(spell.Description, spell.Tooltip)), CostType: costType,
			Source: "ddragon", Path: "/cdn/" + patch + "/img/spell/" + spell.Image.Full,
			Costs: cloneNumbers(spell.Cost), Cooldowns: cloneNumbers(spell.Cooldown), Ranges: cloneNumbers(spell.Range),
		}
	}
	return result, nil
}

func decorateChampionSkills(rows []championMetricRow, abilities map[string]championAssetDescription) {
	for rowIndex := range rows {
		for assetIndex := range rows[rowIndex].Assets {
			slot := ""
			if assetIndex < len(rows[rowIndex].SkillPriority) {
				slot = strings.ToUpper(rows[rowIndex].SkillPriority[assetIndex])
			} else if assetIndex < 3 {
				slot = []string{"Q", "W", "E"}[assetIndex]
			}
			if item, ok := abilities[slot]; ok {
				asset := &rows[rowIndex].Assets[assetIndex]
				asset.Name, asset.Description, asset.CostType = item.Name, item.Description, item.CostType
				if item.Source != "" && item.Path != "" {
					asset.Source, asset.Path = item.Source, item.Path
				}
				asset.Costs, asset.Cooldowns, asset.Ranges = cloneNumbers(item.Costs), cloneNumbers(item.Cooldowns), cloneNumbers(item.Ranges)
			}
		}
		// 技能加点始终附带大招介绍（R 不参与加点顺序统计，但需要展示）。
		if ability, ok := abilities["R"]; ok && rows[rowIndex].Ultimate == nil {
			rows[rowIndex].Ultimate = &championAsset{
				Kind: "ability", Name: ability.Name, Description: ability.Description, CostType: ability.CostType,
				Source: ability.Source, Path: ability.Path,
				Costs: cloneNumbers(ability.Costs), Cooldowns: cloneNumbers(ability.Cooldowns), Ranges: cloneNumbers(ability.Ranges),
			}
		}
	}
}

func championOPGGSlug(key string) string {
	if value, ok := map[string]string{"monkeyking": "wukong"}[strings.ToLower(key)]; ok {
		return value
	}
	return strings.ToLower(key)
}

func validDDragonVersion(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if _, err := strconv.Atoi(part); err != nil {
			return false
		}
	}
	return true
}

func championPinyinTerms(value string) []string {
	args := pinyin.NewArgs()
	args.Style = pinyin.Normal
	parts := pinyin.LazyPinyin(value, args)
	if len(parts) == 0 {
		return nil
	}
	initials := strings.Builder{}
	for _, part := range parts {
		if part != "" {
			initials.WriteByte(part[0])
		}
	}
	return []string{strings.Join(parts, ""), strings.Join(parts, " "), initials.String()}
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

var championAliases = map[string][]string{
	"ahri": {"狐狸"}, "akali": {"阿卡丽"}, "alistar": {"牛头"}, "amumu": {"木木"},
	"aurelionsol": {"龙王"}, "belveth": {"卑尔维斯"}, "blitzcrank": {"机器人"}, "brand": {"火男"},
	"aatrox": {"剑魔", "theshy"}, "caitlyn": {"女警"}, "chogath": {"大虫子"}, "darius": {"诺手"}, "drmundo": {"蒙多", "bin"},
	"draven": {"德莱文"}, "fiddlesticks": {"稻草人"}, "gangplank": {"船长"}, "garen": {"盖伦"},
	"jarvaniv": {"皇子", "j4"}, "jax": {"武器"}, "jayce": {"杰斯"}, "khazix": {"螳螂"},
	"kindred": {"千珏"}, "kogmaw": {"大嘴"}, "leesin": {"盲僧", "瞎子"}, "leblanc": {"妖姬"},
	"malphite": {"石头人"}, "masteryi": {"剑圣"}, "missfortune": {"女枪", "mf"}, "monkeyking": {"猴子", "悟空"},
	"mordekaiser": {"铁男"}, "nautilus": {"泰坦"}, "nocturne": {"梦魇"}, "renata": {"烈娜塔"},
	"renekton": {"鳄鱼"}, "ryze": {"瑞兹", "faker"}, "shaco": {"小丑"}, "singed": {"炼金"}, "tahmkench": {"蛤蟆"},
	"tristana": {"小炮"}, "twistedfate": {"卡牌", "tf"}, "twitch": {"老鼠"}, "vayne": {"薇恩", "vn", "uzi"},
	"veigar": {"小法"}, "velkoz": {"大眼"}, "vladimir": {"吸血鬼"}, "volibear": {"狗熊"},
	"warwick": {"狼人"}, "xinzhao": {"赵信"}, "yasuo": {"亚索", "托儿索"}, "yone": {"永恩"},
	"zac": {"扎克"}, "zed": {"劫"}, "zilean": {"时光"}, "zyra": {"婕拉"},
}

type opggRankedResponse struct {
	Data []struct {
		ID           int             `json:"id"`
		AverageStats opggRankedStats `json:"average_stats"`
		Positions    []struct {
			Name  string          `json:"name"`
			Stats opggRankedStats `json:"stats"`
		} `json:"positions"`
	} `json:"data"`
}

type opggRankedStats struct {
	Play       int     `json:"play"`
	Win        int     `json:"win"`
	TotalPlace int     `json:"total_place"`
	FirstPlace int     `json:"first_place"`
	WinRate    float64 `json:"win_rate"`
	PickRate   float64 `json:"pick_rate"`
	BanRate    float64 `json:"ban_rate"`
	RoleRate   float64 `json:"role_rate"`
	KDA        float64 `json:"kda"`
	Tier       *int    `json:"tier"`
	Rank       int     `json:"rank"`
	TierData   struct {
		Tier          int `json:"tier"`
		Rank          int `json:"rank"`
		RankPrev      int `json:"rank_prev"`
		RankPrevPatch int `json:"rank_prev_patch"`
	} `json:"tier_data"`
}

func opggTierRank(stats opggRankedStats) (int, int) {
	tierValue, rankValue := stats.TierData.Tier, stats.TierData.Rank
	if tierValue == 0 && stats.Tier != nil && *stats.Tier > 0 {
		tierValue = *stats.Tier
	}
	if rankValue == 0 {
		rankValue = stats.Rank
	}
	return tierValue, rankValue
}

func (p *championProvider) loadRanked(ctx context.Context, tier, position string) (championRankingResponse, error) {
	data, err := p.fetch(ctx, opggChampionHost, "/api/KR/champions/ranked", url.Values{"tier": {tier}}, championJSONMax, "application/json")
	if err != nil {
		return championRankingResponse{}, err
	}
	var raw opggRankedResponse
	if json.Unmarshal(data, &raw) != nil || len(raw.Data) < 50 {
		return championRankingResponse{}, errors.New("OP.GG ranked response changed")
	}
	position = strings.ToLower(strings.TrimSpace(position))
	wanted, validPosition := championPositionNames[position]
	if !validPosition {
		return championRankingResponse{}, errors.New("OP.GG ranked position is invalid")
	}
	rows := make([]championRankingRow, 0, len(raw.Data))
	for _, item := range raw.Data {
		positions := make([]string, 0, len(item.Positions))
		for _, candidate := range item.Positions {
			positionName := strings.ToLower(strings.TrimSpace(candidate.Name))
			upstreamName, ok := championPositionNames[positionName]
			if !ok || upstreamName == "" {
				continue
			}
			positions = append(positions, positionName)
		}
		stats := item.AverageStats
		rowPosition := ""
		if wanted == "" {
			if len(positions) == 0 {
				continue
			}
			rowPosition = positions[0]
			for _, candidate := range item.Positions {
				if strings.EqualFold(strings.TrimSpace(candidate.Name), championPositionNames[rowPosition]) {
					stats = candidate.Stats
					break
				}
			}
		} else {
			found := false
			for _, candidate := range item.Positions {
				if strings.EqualFold(strings.TrimSpace(candidate.Name), wanted) {
					stats, rowPosition, found = candidate.Stats, strings.ToLower(strings.TrimSpace(candidate.Name)), true
					break
				}
			}
			if !found {
				continue
			}
		}
		tierValue, rankValue := opggTierRank(stats)
		rows = append(rows, championRankingRow{ChampionID: item.ID, Rank: rankValue, Tier: tierValue, Position: rowPosition, Positions: positions, Play: stats.Play, WinRate: stats.WinRate * 100, PickRate: stats.PickRate * 100, BanRate: stats.BanRate * 100, KDA: stats.KDA})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Rank == rows[j].Rank {
			return rows[i].ChampionID < rows[j].ChampionID
		}
		if rows[i].Rank == 0 {
			return false
		}
		if rows[j].Rank == 0 {
			return true
		}
		return rows[i].Rank < rows[j].Rank
	})
	return championRankingResponse{Mode: "ranked", Region: "KR", Tier: tier, Position: position, Source: "OP.GG", FetchedAt: time.Now(), Rows: rows}, nil
}

type aramChampionRaw struct {
	Key        string  `json:"key"`
	Name       string  `json:"name"`
	ImageURL   string  `json:"image_url"`
	ChampionID int     `json:"champion_id"`
	ID         int     `json:"id"`
	Tier       int     `json:"tier"`
	Rank       int     `json:"rank"`
	WinRate    float64 `json:"win_rate"`
	PickRate   float64 `json:"pick_rate"`
}

type arenaTeamChampionRaw struct {
	ID       int    `json:"id"`
	Key      string `json:"key"`
	Name     string `json:"name"`
	ImageURL string `json:"image_url"`
}

type arenaTeamRaw struct {
	ChampionIDs       []int                  `json:"champion_ids"`
	ChampionID        int                    `json:"champion_id"`
	Champions         []arenaTeamChampionRaw `json:"champions"`
	TeammateChampions []arenaTeamChampionRaw `json:"teammate_champions"`
	CombinationSize   int                    `json:"combination_size"`
	Play              json.RawMessage        `json:"play"`
	WinRate           float64                `json:"win_rate"`
	FirstPlace        float64                `json:"first_place"`
	FirstPlaceRate    float64                `json:"first_place_rate"`
	AvgPlace          float64                `json:"avg_place"`
	AveragePlace      float64                `json:"average_place"`
	PickRate          float64                `json:"pick_rate"`
}

type arenaStatsRaw struct {
	WinRate    float64 `json:"win_rate"`
	PickRate   float64 `json:"pick_rate"`
	BanRate    float64 `json:"ban_rate"`
	FirstPlace float64 `json:"first_place"`
	AvgPlace   float64 `json:"avg_place"`
}

type arenaAugmentRaw struct {
	ID          int             `json:"id"`
	Name        string          `json:"name"`
	ImageURL    string          `json:"image_url"`
	PickRate    float64         `json:"pick_rate"`
	WinRate     float64         `json:"win_rate"`
	Play        json.RawMessage `json:"play"`
	Description string          `json:"desc"`
}

func (p *championProvider) loadARAMPage(ctx context.Context) ([]byte, string, error) {
	data, err := p.fetch(ctx, opggPageHost, "/zh-cn/lol/modes/aram-mayhem", nil, championHTMLMax, "text/html,application/xhtml+xml")
	if err != nil {
		return nil, "", err
	}
	decoded := decodeNextFlight(data)
	if decoded == "" {
		return nil, "", errors.New("OP.GG page data changed")
	}
	return data, decoded, nil
}

func (p *championProvider) loadARAMRankings(ctx context.Context) (championRankingResponse, error) {
	if response, err := p.loadHexdataRankings(ctx); err == nil {
		return response, nil
	}
	data, err := p.fetch(ctx, opggChampionHost, "/api/contents/tiers", url.Values{"type": {"aram_mayhem"}}, championJSONMax, "application/json")
	if err != nil {
		return championRankingResponse{}, err
	}
	var payload struct {
		Data []aramChampionRaw `json:"data"`
		Meta struct {
			Version string `json:"version"`
		} `json:"meta"`
	}
	if json.Unmarshal(data, &payload) != nil || len(payload.Data) < 100 {
		return championRankingResponse{}, errors.New("OP.GG ARAM champion response changed")
	}
	rows := make([]championRankingRow, 0, len(payload.Data))
	for _, item := range payload.Data {
		id := item.ChampionID
		if id == 0 {
			id = item.ID
		}
		if id == 0 || item.Rank <= 0 {
			continue
		}
		rows = append(rows, championRankingRow{ChampionID: id, Key: item.Key, Name: item.Name, Rank: item.Rank, Tier: item.Tier})
	}
	if len(rows) < 100 {
		return championRankingResponse{}, errors.New("OP.GG ARAM champion response changed")
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Rank < rows[j].Rank })
	return championRankingResponse{Mode: "hextech-aram", Region: "KR", Patch: payload.Meta.Version, Source: "OP.GG JSON · Hexdata fallback", FetchedAt: time.Now(), EntertainmentSample: true, Rows: rows}, nil
}

func (p *championProvider) loadStructuredModeRankings(ctx context.Context, mode string) (championRankingResponse, error) {
	spec, ok := opggModeSpecs[mode]
	if !ok || mode == "ranked" || mode == "arena" || mode == "hextech-aram" {
		return championRankingResponse{}, errors.New("unsupported structured ranking mode")
	}
	requestPath := "/api/" + spec.Region + "/champions/" + spec.APIMode
	data, err := p.fetch(ctx, opggChampionHost, requestPath, nil, championJSONMax, "application/json")
	if err != nil {
		return championRankingResponse{}, err
	}
	var payload struct {
		Data []struct {
			ID           int             `json:"id"`
			AverageStats opggRankedStats `json:"average_stats"`
		} `json:"data"`
		Meta struct {
			Version string `json:"version"`
		} `json:"meta"`
	}
	if json.Unmarshal(data, &payload) != nil || len(payload.Data) < 50 {
		return championRankingResponse{}, errors.New("OP.GG champion ranking response changed")
	}
	rows := make([]championRankingRow, 0, len(payload.Data))
	for _, item := range payload.Data {
		stats := item.AverageStats
		if item.ID <= 0 || stats.Play <= 0 {
			continue
		}
		rows = append(rows, championRankingRow{
			ChampionID: item.ID, Rank: firstPositiveInt(stats.TierData.Rank, stats.Rank), Tier: stats.TierData.Tier,
			Play: stats.Play, WinRate: firstPositive(fractionToPercent(stats.WinRate), percentOf(stats.Win, stats.Play)),
			PickRate: fractionToPercent(stats.PickRate), BanRate: fractionToPercent(stats.BanRate), KDA: stats.KDA,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Rank == rows[j].Rank {
			return rows[i].ChampionID < rows[j].ChampionID
		}
		if rows[i].Rank == 0 {
			return false
		}
		if rows[j].Rank == 0 {
			return true
		}
		return rows[i].Rank < rows[j].Rank
	})
	return championRankingResponse{Mode: mode, Region: spec.Region, Patch: payload.Meta.Version, Source: "OP.GG JSON", FetchedAt: time.Now(), Rows: rows}, nil
}

// Arena's list must use the same provider and region as the YOUR.GG page,
// not OP.GG's global sample with a locally re-sorted ranking.
func (p *championProvider) loadArenaRankings(ctx context.Context) (championRankingResponse, error) {
	data, at, err := p.fetchWithMetadata(ctx, yourGGArenaHost, "/kr/api/arena/champions", nil, championJSONMax, "application/json")
	if err != nil {
		return championRankingResponse{}, err
	}
	return parseYourGGArenaRankings(data, at, p.diag)
}

// fractionToPercent is used only for structured OP.GG fields whose contract is
// explicitly a 0..1 fraction. Point-valued sources must use their own parser.
func fractionToPercent(value float64) float64 {
	return value * 100
}

func percentOf(value, total int) float64 {
	if value <= 0 || total <= 0 {
		return 0
	}
	return float64(value) / float64(total) * 100
}

// parseArenaTeamCompositions keeps the requested subject in the first slot while
// preserving the upstream order of every teammate.
func parseArenaTeamCompositions(decoded, marker string, subjectID, limit int) []arenaTeamComposition {
	var raw []arenaTeamRaw
	if !extractBestArray(decoded, marker, func(candidate []json.RawMessage) int {
		score := 0
		for _, entry := range candidate {
			var item arenaTeamRaw
			if json.Unmarshal(entry, &item) == nil && item.CombinationSize == 3 && len(item.Champions) >= 3 {
				score++
			}
		}
		return score
	}, &raw) {
		return nil
	}
	result := make([]arenaTeamComposition, 0, min(limit, len(raw)))
	seen := make(map[string]bool)
	for _, item := range raw {
		champions := item.Champions
		if len(champions) < 3 {
			champions = item.TeammateChampions
		}
		team := arenaTeamComposition{
			AveragePlacement: firstPositive(item.AveragePlace, item.AvgPlace),
			FirstPlaceRate:   firstPositive(item.FirstPlaceRate, item.FirstPlace),
			PickRate:         item.PickRate,
			WinRate:          item.WinRate,
			Games:            flexibleJSONInt(item.Play),
		}
		ids := make([]string, 0, 3)
		for _, champion := range champions {
			asset, ok := remoteAsset(champion.ImageURL, champion.Name)
			if !ok || champion.ID <= 0 || champion.Key == "" {
				continue
			}
			team.Champions = append(team.Champions, arenaTeamChampion{
				ID: champion.ID, Key: champion.Key, Name: champion.Name,
				ImageSource: asset.Source, ImagePath: asset.Path,
			})
			ids = append(ids, strconv.Itoa(champion.ID))
			if len(team.Champions) == 3 {
				break
			}
		}
		wantedID := subjectID
		if wantedID <= 0 {
			wantedID = item.ChampionID
		}
		for index, champion := range team.Champions {
			if index > 0 && champion.ID == wantedID {
				copy(team.Champions[1:index+1], team.Champions[0:index])
				team.Champions[0] = champion
				break
			}
		}
		ids = ids[:0]
		for _, champion := range team.Champions {
			ids = append(ids, strconv.Itoa(champion.ID))
		}
		signature := strings.Join(ids, ",")
		if len(team.Champions) != 3 || signature == "" || seen[signature] {
			continue
		}
		seen[signature] = true
		result = append(result, team)
		if limit > 0 && len(result) == limit {
			break
		}
	}
	return result
}

func parseArenaStats(decoded string) arenaChampionStats {
	for offset := 0; ; {
		index := strings.Index(decoded[offset:], `"average_stats":`)
		if index < 0 {
			return arenaChampionStats{}
		}
		index += offset + len(`"average_stats":`)
		object, end, ok := balancedJSONObject(decoded, index)
		if ok {
			var raw arenaStatsRaw
			if json.Unmarshal(object, &raw) == nil && (raw.WinRate > 0 || raw.PickRate > 0) {
				return arenaChampionStats{
					AveragePlacement: raw.AvgPlace, FirstPlaceRate: raw.FirstPlace,
					PickRate: raw.PickRate, WinRate: raw.WinRate, BanRate: raw.BanRate,
				}
			}
			offset = end
		} else {
			offset = index + 1
		}
	}
}

func parseArenaAugments(decoded string) []championMetricRow {
	rows := make([]championMetricRow, 0, 12)
	seen := make(map[string]bool)
	for offset := 0; ; {
		index := strings.Index(decoded[offset:], `"data":`)
		if index < 0 {
			break
		}
		index += offset + len(`"data":`)
		object, end, ok := balancedJSONObject(decoded, index)
		if !ok {
			offset = index + 1
			continue
		}
		offset = end
		var raw arenaAugmentRaw
		if json.Unmarshal(object, &raw) != nil || raw.ID <= 0 || raw.Name == "" || !strings.Contains(raw.ImageURL, "/augment/") {
			continue
		}
		asset, valid := remoteAsset(raw.ImageURL, raw.Name)
		if !valid || seen[asset.Path] {
			continue
		}
		seen[asset.Path] = true
		asset.ID = raw.ID
		asset.Kind = "augment"
		asset.Description = cleanMarkup(raw.Description)
		rows = append(rows, championMetricRow{
			Assets: []championAsset{asset}, PickRate: raw.PickRate,
			WinRate: raw.WinRate, Games: flexibleJSONInt(raw.Play),
		})
		if len(rows) == 12 {
			break
		}
	}
	return rows
}

func firstPositive(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func flexibleJSONInt(value json.RawMessage) int {
	if len(value) == 0 {
		return 0
	}
	var text string
	if json.Unmarshal(value, &text) == nil {
		parsed, _ := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(text), ",", ""))
		return parsed
	}
	var numberValue float64
	if json.Unmarshal(value, &numberValue) == nil {
		return int(numberValue)
	}
	return 0
}

type augmentRaw struct {
	ID          int             `json:"id"`
	Key         string          `json:"key"`
	Name        string          `json:"name"`
	Tier        int             `json:"tier"`
	Rarity      int             `json:"rarity"`
	Performance float64         `json:"performance"`
	Popular     float64         `json:"popular"`
	LargeIcon   string          `json:"largeIcon"`
	Desc        string          `json:"desc"`
	Tooltip     string          `json:"tooltip"`
	ChampionIDs json.RawMessage `json:"champion_ids"`
	Champions   []struct {
		ID          int     `json:"id"`
		Performance float64 `json:"performance"`
		Popular     float64 `json:"popular"`
	} `json:"champions"`
}

type augmentChampionRaw struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Key      string `json:"key"`
	ImageURL string `json:"image_url"`
}

func (p *championProvider) loadAugments(ctx context.Context) (championAugmentResponse, error) {
	if p.hexdata != nil {
		if page, pageErr := p.hexdata.load(ctx, "augments", "all", "/augments", "text/html,application/xhtml+xml", false); pageErr == nil {
			if rows, citation, parseErr := parseHexdataAugments(page.Data); parseErr == nil && hexdataCitationComplete(citation) && (page.Cache == championCacheStateStale || p.hexdata.adoptCitation(citation)) {
				catalog := p.loadAugmentMetadataCatalog(ctx)
				byID := gameplayAugmentIndexAll(catalog)
				for index := range rows {
					if meta, ok := byID[rows[index].ID]; ok {
						if strings.TrimSpace(rows[index].Description) == "" && strings.TrimSpace(meta.Description) != "" {
							rows[index].Description = meta.Description
						}
						if rarity := normalizeAugmentRarity(meta.Rarity); rarity != "unknown" {
							rows[index].Rarity = rarity
						}
						if path := communityDragonGameAssetPath(meta.IconPath); path != "" {
							rows[index].ImageSource, rows[index].ImagePath = "communitydragon", path
							rows[index].ImageFallbackPath = communityDragonGameAssetPath(meta.FallbackIconPath)
						}
					}
					rows[index].Description = augmentDescriptionWithOfflineGuidance(rows[index].ID, rows[index].Description)
				}
				p.hexdata.promote("augments", "all", citation.BuildID, page.Data, page.FetchedAt)
				p.hexdata.recordSuccess("augments", "/augments")
				p.reportHexdataShape("augments", len(rows), hexdataTableFieldCount(page.Data), citation.BuildID)
				response := championAugmentResponse{Source: "Hexdata", Mode: "aram-mayhem", FetchedAt: page.FetchedAt, EntertainmentSample: true, Citation: &citation, MeasurementTechnique: p.loadHexdataMeasurementTechnique(ctx), Rows: rows}
				p.rememberAugmentCatalog(response)
				return response, nil
			} else {
				p.hexdata.recordShapeFailure("augments", len(rows), hexdataTableFieldCount(page.Data), citation.BuildID)
			}
		} else {
			p.reportHexdataFallback("augments", pageErr)
		}
	}
	_, decoded, err := p.loadARAMPage(ctx)
	if err != nil {
		if cached, ok := p.rememberedAugmentCatalog(); ok {
			cached.MeasurementTechnique = appendMeasurementTechnique(cached.MeasurementTechnique, "本次海克斯目录刷新失败，暂显示上次成功读取的缓存")
			cached.Source += "（缓存）"
			return cached, nil
		}
		return championAugmentResponse{}, err
	}
	var raw []augmentRaw
	if !extractBestArray(decoded, `"data":`, func(candidate []json.RawMessage) int {
		if len(candidate) < 50 {
			return 0
		}
		score := 0
		for _, entry := range candidate {
			var item augmentRaw
			if json.Unmarshal(entry, &item) == nil && item.Name != "" && item.LargeIcon != "" {
				score++
			}
		}
		return score
	}, &raw) {
		if cached, ok := p.rememberedAugmentCatalog(); ok {
			cached.MeasurementTechnique = appendMeasurementTechnique(cached.MeasurementTechnique, "本次海克斯目录解析失败，暂显示上次成功读取的缓存")
			cached.Source += "（缓存）"
			return cached, nil
		}
		return championAugmentResponse{}, errors.New("OP.GG augment response changed")
	}
	rows := make([]championAugment, 0, len(raw))
	for _, item := range raw {
		asset, ok := remoteAsset(item.LargeIcon, item.Name)
		if !ok || item.ID == 0 || item.Name == "" {
			continue
		}
		metadata := make(map[int]augmentChampionRaw)
		var rich []augmentChampionRaw
		if json.Unmarshal(item.ChampionIDs, &rich) == nil {
			for _, champion := range rich {
				metadata[champion.ID] = champion
			}
		}
		champions := make([]championAugmentChampion, 0, len(item.Champions))
		for _, stats := range item.Champions {
			entry := championAugmentChampion{ID: stats.ID, Performance: stats.Performance, Popularity: stats.Popular}
			if meta, exists := metadata[stats.ID]; exists {
				entry.Name, entry.Key = meta.Name, meta.Key
				if icon, valid := remoteAsset(meta.ImageURL, meta.Name); valid {
					entry.ImageSource, entry.ImagePath = icon.Source, icon.Path
				}
			}
			champions = append(champions, entry)
		}
		rows = append(rows, championAugment{ID: item.ID, Key: item.Key, Name: item.Name, Tier: item.Tier, Rarity: augmentRarity(item.Rarity), Performance: item.Performance, Popularity: item.Popular, Description: cleanMarkup(item.Desc), Tooltip: cleanMarkup(item.Tooltip), ImageSource: asset.Source, ImagePath: asset.Path, Champions: champions})
	}
	for index := range rows {
		rows[index].Description = augmentDescriptionWithOfflineGuidance(rows[index].ID, rows[index].Description)
	}
	if len(rows) < 40 {
		if cached, ok := p.rememberedAugmentCatalog(); ok {
			cached.MeasurementTechnique = appendMeasurementTechnique(cached.MeasurementTechnique, "本次海克斯目录样本不足，暂显示上次成功读取的缓存")
			cached.Source += "（缓存）"
			return cached, nil
		}
		return championAugmentResponse{}, errors.New("OP.GG augment catalog is incomplete")
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Tier == rows[j].Tier {
			if augmentRarityOrder(rows[i].Rarity) != augmentRarityOrder(rows[j].Rarity) {
				return augmentRarityOrder(rows[i].Rarity) < augmentRarityOrder(rows[j].Rarity)
			}
			return rows[i].Name < rows[j].Name
		}
		return rows[i].Tier < rows[j].Tier
	})
	response := championAugmentResponse{Source: "OP.GG", Mode: "aram-mayhem", FetchedAt: time.Now(), EntertainmentSample: true, Rows: rows}
	p.rememberAugmentCatalog(response)
	return response, nil
}

func cloneChampionAugmentResponse(value championAugmentResponse) championAugmentResponse {
	value.Rows = append([]championAugment(nil), value.Rows...)
	for index := range value.Rows {
		value.Rows[index].Champions = append([]championAugmentChampion(nil), value.Rows[index].Champions...)
	}
	if value.Citation != nil {
		citation := *value.Citation
		value.Citation = &citation
	}
	return value
}

func (p *championProvider) rememberAugmentCatalog(value championAugmentResponse) {
	p.augmentCatalogMu.Lock()
	p.augmentCatalog = cloneChampionAugmentResponse(value)
	p.augmentCatalogReady = true
	p.augmentCatalogMu.Unlock()
}

func (p *championProvider) rememberedAugmentCatalog() (championAugmentResponse, bool) {
	p.augmentCatalogMu.Lock()
	defer p.augmentCatalogMu.Unlock()
	if !p.augmentCatalogReady || len(p.augmentCatalog.Rows) == 0 {
		return championAugmentResponse{}, false
	}
	return cloneChampionAugmentResponse(p.augmentCatalog), true
}

// loadAugmentMetadataCatalog gives the authenticated client the first chance
// to describe an augment by its real ID. CommunityDragon is merged afterward
// for icons, rarity, and entries absent from the local catalog.
func (p *championProvider) loadAugmentMetadataCatalog(ctx context.Context) []gameplayAugment {
	var catalog []gameplayAugment
	if p.gameplayAugments != nil {
		if local, err := p.gameplayAugments(ctx); err == nil {
			catalog = append(catalog, local...)
		}
	}
	if remote, err := p.loadCommunityDragonAugments(ctx); err == nil {
		catalog = mergeGameplayAugmentMetadata(catalog, remote)
	}
	return catalog
}

// R116-B 更正（原注释的前提已被实测证伪）：这里以前写的是「Riot 数据里到处
// 都拿不到海克斯描述，唯一能渲染的中文描述来自 Hexdata 的海克斯页面，所以
// 按需抓取」。自 R116-A 起海斗详情走 /api/hexdata/heroes/{id}，响应里的
// augments[].augmentDescription 实测覆盖率 126/126 = 100%，描述已经随主响应
// 一次拿全，per-augment 的页面扇出（mayhemAugmentCopy/augmentSlugs 那条链路）
// 已整条删除。
// 这个占位串现在只服务两条仍然存在的回退路径：① 海克斯图鉴目录（OP.GG /
// CommunityDragon 元数据，ID ≥ 1000 的海斗海克斯确实没有描述字段）；
// ② 图鉴里点开单个海克斯时按需读取的 Hexdata 页面解析失败。
// 文案保持「海克斯图鉴中可读取说明」而不是「客户端可查看」——它指向的是站内
// 图鉴入口，不是 League 客户端。
const augmentOfflineDescription = "海克斯图鉴中可读取说明"

func augmentDescriptionWithOfflineGuidance(id int, description string) string {
	if strings.TrimSpace(description) != "" {
		return description
	}
	if id >= 1000 {
		return augmentOfflineDescription
	}
	return ""
}

func augmentRarityOrder(value string) int {
	switch value {
	case "prismatic":
		return 0
	case "gold":
		return 1
	case "silver":
		return 2
	default:
		return 3
	}
}

func augmentRarity(value int) string {
	// OP.GG's legacy 1/4/8 scale must be translated before this enters the
	// CommunityDragon 0/1/2/4 normalization scale, where 1 and 4 differ.
	switch value {
	case 1:
		return normalizeAugmentRarity(0)
	case 4:
		return normalizeAugmentRarity(1)
	case 8:
		return normalizeAugmentRarity(2)
	default:
		return "unknown"
	}
}

func cleanMarkup(value string) string {
	value = strings.ReplaceAll(value, "<br />", "\n")
	value = strings.ReplaceAll(value, "<br/>", "\n")
	value = strings.ReplaceAll(value, "<br>", "\n")
	value = markupPattern.ReplaceAllString(value, "")
	value = stdhtml.UnescapeString(value)
	lines := strings.FieldsFunc(value, func(r rune) bool { return r == '\n' || r == '\r' })
	for index := range lines {
		lines[index] = strings.Join(strings.Fields(lines[index]), " ")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func decodeNextFlight(page []byte) string {
	matches := flightPattern.FindAllSubmatch(page, -1)
	var result strings.Builder
	for _, match := range matches {
		var fragment string
		if len(match) == 2 && json.Unmarshal(match[1], &fragment) == nil {
			result.WriteString(fragment)
		}
	}
	return result.String()
}

func extractBestArray(source, marker string, score func([]json.RawMessage) int, target any) bool {
	bestScore := 0
	var best []byte
	for offset := 0; ; {
		index := strings.Index(source[offset:], marker)
		if index < 0 {
			break
		}
		index += offset + len(marker)
		array, end, ok := balancedJSONArray(source, index)
		if ok {
			var candidate []json.RawMessage
			if json.Unmarshal(array, &candidate) == nil {
				if current := score(candidate); current > bestScore {
					bestScore, best = current, append(best[:0], array...)
				}
			}
			offset = end
		} else {
			offset = index + 1
		}
	}
	return bestScore > 0 && json.Unmarshal(best, target) == nil
}

func balancedJSONArray(source string, start int) ([]byte, int, bool) {
	for start < len(source) && unicode.IsSpace(rune(source[start])) {
		start++
	}
	if start >= len(source) || source[start] != '[' {
		return nil, start, false
	}
	depth, quoted, escaped := 0, false, false
	for index := start; index < len(source); index++ {
		character := source[index]
		if quoted {
			if escaped {
				escaped = false
			} else if character == '\\' {
				escaped = true
			} else if character == '"' {
				quoted = false
			}
			continue
		}
		if character == '"' {
			quoted = true
			continue
		}
		if character == '[' {
			depth++
		} else if character == ']' {
			depth--
			if depth == 0 {
				return []byte(source[start : index+1]), index + 1, true
			}
		}
	}
	return nil, start, false
}

func balancedJSONObject(source string, start int) ([]byte, int, bool) {
	for start < len(source) && unicode.IsSpace(rune(source[start])) {
		start++
	}
	if start >= len(source) || source[start] != '{' {
		return nil, start, false
	}
	depth, quoted, escaped := 0, false, false
	for index := start; index < len(source); index++ {
		character := source[index]
		if quoted {
			if escaped {
				escaped = false
			} else if character == '\\' {
				escaped = true
			} else if character == '"' {
				quoted = false
			}
			continue
		}
		if character == '"' {
			quoted = true
			continue
		}
		if character == '{' {
			depth++
		} else if character == '}' {
			depth--
			if depth == 0 {
				return []byte(source[start : index+1]), index + 1, true
			}
		}
	}
	return nil, start, false
}

func normalizeArenaBuild(sections championBuildSections) championBuildSections {
	core := make([]championMetricRow, 0, len(sections.CoreItems))
	for _, row := range sections.CoreItems {
		assets := row.Assets[:0]
		for _, asset := range row.Assets {
			if asset.ID == 220007 || strings.TrimSpace(asset.Name) == "棱彩装备" || strings.TrimSpace(asset.Name) == "棱镜装备" {
				continue
			}
			assets = append(assets, asset)
		}
		row.Assets = assets
		if len(row.Assets) == 0 {
			continue
		}
		if len(row.Assets) == 1 && isArenaBoot(row.Assets[0]) {
			sections.Boots = appendUniqueMetricRows(sections.Boots, row)
			continue
		}
		core = appendUniqueMetricRows(core, row)
	}
	sections.CoreItems = core
	return sections
}

func isArenaBoot(asset championAsset) bool {
	name := strings.TrimSpace(asset.Name)
	for _, marker := range []string{"鞋", "靴", "胫甲", "钢盖"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

func parseChampionCounters(document *xhtml.Node) championCounterSections {
	return championCounterSections{
		WeakAgainst:   parseChampionCounterGroups(document, "劣势对抗", "对线劣势的英雄", "对线劣势", "劣势对线"),
		StrongAgainst: parseChampionCounterGroups(document, "优势对抗", "强烈对抗", "对线优势的英雄", "对线优势", "优势对线"),
	}
}

func parseChampionCounterGroups(document *xhtml.Node, labels ...string) []championCounterRow {
	for _, label := range labels {
		if rows := parseChampionCounterGroup(document, label); len(rows) > 0 {
			return rows
		}
	}
	return nil
}

func parseChampionCounterGroup(document *xhtml.Node, label string) []championCounterRow {
	var heading *xhtml.Node
	for _, node := range allDescendantElements(document) {
		if strings.TrimSpace(nodeText(node)) == label {
			heading = node
			break
		}
	}
	if heading == nil {
		return nil
	}
	var listContainer *xhtml.Node
	// OP.GG has used several wrapper depths for this section. Search a small
	// sibling window at each ancestor instead of assuming the list is the
	// immediate next sibling; this keeps the grouped direction labels intact.
	for container, depth := heading, 0; container != nil && depth < 8 && listContainer == nil; container, depth = container.Parent, depth+1 {
		for candidate, siblings := nextElementSibling(container), 0; candidate != nil && siblings < 5; candidate, siblings = nextElementSibling(candidate), siblings+1 {
			if len(descendantElements(candidate, "li")) > 0 {
				listContainer = candidate
				break
			}
		}
	}
	if listContainer == nil {
		return nil
	}
	rows := make([]championCounterRow, 0, 5)
	seen := make(map[string]bool)
	for _, item := range descendantElements(listContainer, "li") {
		anchor := firstDescendant(item, "a")
		image := firstDescendant(item, "img")
		strong := firstDescendant(item, "strong")
		parsed, err := url.Parse(stdhtml.UnescapeString(attribute(anchor, "href")))
		if err != nil {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parsed.Query().Get("target_champion")))
		asset, ok := remoteAsset(attribute(image, "src"), attribute(image, "alt"))
		if !ok || !championSlugPattern.MatchString(key) || seen[key] {
			continue
		}
		seen[key] = true
		rows = append(rows, championCounterRow{
			Key: key, Name: strings.TrimSpace(attribute(image, "alt")), ImageSource: asset.Source, ImagePath: asset.Path,
			WinRate: firstNumber(nodeText(strong)), Games: firstGames(nodeText(item)),
		})
		if len(rows) == 5 {
			break
		}
	}
	return rows
}

// OPGG has rendered counter section labels as div, h3, and span elements
// across different page builds. Keep the grouping parser independent of that
// presentational choice while still requiring an exact text match.
func allDescendantElements(node *xhtml.Node) []*xhtml.Node {
	result := make([]*xhtml.Node, 0)
	var walk func(*xhtml.Node)
	walk = func(current *xhtml.Node) {
		if current.Type == xhtml.ElementNode {
			result = append(result, current)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	if node != nil {
		walk(node)
	}
	return result
}

func appendUniqueMetricRows(existing []championMetricRow, rows ...championMetricRow) []championMetricRow {
	seen := make(map[string]bool, len(existing)+len(rows))
	for _, row := range existing {
		seen[metricRowSignature(row)] = true
	}
	for _, row := range rows {
		signature := metricRowSignature(row)
		if signature != "" && !seen[signature] {
			seen[signature] = true
			existing = append(existing, row)
		}
	}
	return existing
}

func metricRowSignature(row championMetricRow) string {
	parts := make([]string, 0, len(row.Assets)+len(row.SkillOrder))
	for _, asset := range row.Assets {
		parts = append(parts, asset.Path)
	}
	parts = append(parts, row.SkillOrder...)
	return strings.Join(parts, "|")
}

type runePageRaw struct {
	Play     int     `json:"play"`
	PickRate float64 `json:"pick_rate"`
	WinRate  float64 `json:"win_rate"`
	Builds   []struct {
		PrimaryStyle runeAssetRaw     `json:"primary_perk_style"`
		SubStyle     runeAssetRaw     `json:"perk_sub_style"`
		MainRunes    [][]runeAssetRaw `json:"main_runes"`
		SubRunes     [][]runeAssetRaw `json:"sub_runes"`
		Shards       [][]runeAssetRaw `json:"shards"`
	} `json:"builds"`
}

type runeAssetRaw struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ShortDesc   string `json:"short_desc"`
	LongDesc    string `json:"long_desc"`
	ImageURL    string `json:"image_url"`
	Active      bool   `json:"isActive"`
}

func parseChampionRunes(decoded string) []championRunePage {
	// R60 cleanup marker: this legacy HTML parser is retained for its tests only;
	// production champion details use the structured OP.GG response.
	var raw []runePageRaw
	if !extractBestArray(decoded, `"rune_pages":`, func(candidate []json.RawMessage) int {
		if len(candidate) == 0 {
			return 0
		}
		var page runePageRaw
		if json.Unmarshal(candidate[0], &page) != nil || len(page.Builds) == 0 {
			return 0
		}
		return len(candidate)
	}, &raw) {
		return nil
	}
	result := make([]championRunePage, 0, minInt(2, len(raw)))
	for _, page := range raw {
		if len(page.Builds) == 0 || len(result) == 2 {
			break
		}
		build := page.Builds[0]
		primary, okPrimary := runeAsset(build.PrimaryStyle)
		sub, okSub := runeAsset(build.SubStyle)
		if !okPrimary || !okSub {
			continue
		}
		primarySlots := runeAssetGroups(build.MainRunes)
		subSlots := runeAssetGroups(build.SubRunes)
		shardSlots := runeAssetGroups(build.Shards)
		selected := make([]championAsset, 0, 9)
		for _, groups := range [][][]championAsset{primarySlots, subSlots, shardSlots} {
			for _, group := range groups {
				for _, asset := range group {
					if asset.Active {
						selected = append(selected, asset)
					}
				}
			}
		}
		result = append(result, championRunePage{
			PrimaryStyle: primary, SubStyle: sub, Selected: selected,
			PrimarySlots: primarySlots, SubSlots: subSlots, ShardSlots: shardSlots,
			PickRate: page.PickRate * 100, WinRate: page.WinRate * 100, Games: page.Play,
		})
	}
	return result
}

func runeAssetGroups(groups [][]runeAssetRaw) [][]championAsset {
	result := make([][]championAsset, 0, len(groups))
	for _, group := range groups {
		row := make([]championAsset, 0, len(group))
		for _, item := range group {
			if asset, ok := runeAsset(item); ok {
				row = append(row, asset)
			}
		}
		if len(row) > 0 {
			result = append(result, row)
		}
	}
	return result
}

func runeAsset(item runeAssetRaw) (championAsset, bool) {
	asset, ok := remoteAsset(item.ImageURL, item.Name)
	if ok {
		asset.ID, asset.Active = item.ID, item.Active
		asset.Description = cleanMarkup(firstNonEmpty(item.LongDesc, item.Description, item.ShortDesc))
		if shardPath, exists := dataDragonRuneShardPath(item.ID); exists {
			asset.Kind, asset.Source, asset.Path = "perkShard", "ddragon", shardPath
			if description := runeShardDescription(item.ID); description != "" {
				asset.Description = description
			}
		}
	}
	return asset, ok
}

func runeShardDescription(id int) string {
	return map[int]string{
		5001: "获得10至180额外生命值（基于等级）。",
		5005: "获得10%攻击速度。",
		5007: "获得8技能急速。",
		5008: "获得9适应之力（5.4攻击力或9法术强度）。",
		5010: "获得2.5%移动速度。",
		5011: "获得65生命值。",
		5013: "获得10%韧性和减速抗性。",
	}[id]
}

func dataDragonRuneShardPath(id int) (string, bool) {
	name, ok := map[int]string{
		5001: "StatModsHealthPlusIcon.png",
		5005: "StatModsAttackSpeedIcon.png",
		5007: "StatModsCDRScalingIcon.png",
		5008: "StatModsAdaptiveForceIcon.png",
		5010: "StatModsMovementSpeedIcon.png",
		5011: "StatModsHealthScalingIcon.png",
		5013: "StatModsTenacityIcon.png",
	}[id]
	if !ok {
		return "", false
	}
	return "/cdn/img/perk-images/StatMods/" + name, true
}

func remoteAsset(rawURL, name string) (championAsset, bool) {
	parsed, err := url.Parse(stdhtml.UnescapeString(rawURL))
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || !strings.EqualFold(parsed.Hostname(), opggAssetHost) {
		return championAsset{}, false
	}
	path := parsed.EscapedPath()
	if decoded, err := url.PathUnescape(path); err == nil {
		path = decoded
	}
	if _, ok := validateChampionAssetPath("opgg", path); !ok {
		return championAsset{}, false
	}
	kind := "asset"
	for _, candidate := range []string{"champion", "item", "spell", "perkStyle", "perkShard", "perk", "aram-augment", "augment"} {
		if strings.Contains(path, "/"+candidate+"/") {
			kind = candidate
			break
		}
	}
	asset := championAsset{Kind: kind, Name: strings.TrimSpace(name), Source: "opgg", Path: path}
	if kind == "item" {
		asset.ID, _ = strconv.Atoi(strings.TrimSuffix(pathpkg.Base(path), pathpkg.Ext(path)))
	}
	return asset, true
}

func descendantElements(node *xhtml.Node, name string) []*xhtml.Node {
	result := make([]*xhtml.Node, 0)
	var walk func(*xhtml.Node)
	walk = func(current *xhtml.Node) {
		if current.Type == xhtml.ElementNode && current.Data == name {
			result = append(result, current)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	if node != nil {
		walk(node)
	}
	return result
}

func firstDescendant(node *xhtml.Node, name string) *xhtml.Node {
	items := descendantElements(node, name)
	if len(items) == 0 {
		return nil
	}
	return items[0]
}

func directElements(node *xhtml.Node, name string) []*xhtml.Node {
	result := make([]*xhtml.Node, 0)
	if node == nil {
		return result
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == xhtml.ElementNode && child.Data == name {
			result = append(result, child)
		}
	}
	return result
}

func nextElementSibling(node *xhtml.Node) *xhtml.Node {
	if node == nil {
		return nil
	}
	for sibling := node.NextSibling; sibling != nil; sibling = sibling.NextSibling {
		if sibling.Type == xhtml.ElementNode {
			return sibling
		}
	}
	return nil
}

func attribute(node *xhtml.Node, name string) string {
	if node == nil {
		return ""
	}
	for _, attribute := range node.Attr {
		if attribute.Key == name {
			return attribute.Val
		}
	}
	return ""
}

func nodeText(node *xhtml.Node) string {
	if node == nil {
		return ""
	}
	var builder strings.Builder
	var walk func(*xhtml.Node)
	walk = func(current *xhtml.Node) {
		if current.Type == xhtml.TextNode {
			builder.WriteString(current.Data)
			builder.WriteByte(' ')
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return strings.Join(strings.Fields(builder.String()), " ")
}

func firstNumber(value string) float64 {
	match := numberPattern.FindString(strings.ReplaceAll(value, ",", ""))
	parsed, _ := strconv.ParseFloat(match, 64)
	return parsed
}

func firstGames(value string) int {
	match := gamesPattern.FindStringSubmatch(value)
	if len(match) != 2 {
		return 0
	}
	parsed, _ := strconv.Atoi(strings.ReplaceAll(match[1], ",", ""))
	return parsed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
