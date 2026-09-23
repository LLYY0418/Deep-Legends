package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	xhtml "golang.org/x/net/html"
)

const (
	hexdataHost                 = "hexdata.com.cn"
	hexdataAnswerPath           = "/api/hexdata/answer-cards"
	hexdataCacheTTL             = 10 * 365 * 24 * time.Hour
	hexdataBuildSoftTTL         = 12 * time.Hour
	hexdataCircuitDuration      = 30 * time.Minute
	hexdataCircuitProbeInterval = 5 * time.Minute
	hexdataCircuitFailureLimit  = 3
	hexdataCircuitFailureDecay  = 30 * time.Minute
	hexdataCircuitFailureMax    = 6
	hexdataMinimumInterval      = 300 * time.Millisecond
	hexdataMaximumJitter        = 800 * time.Millisecond
	hexdataRetryDelay           = 2 * time.Second
	hexdataMinimumSample        = 250
	// R116-A：JSON API 的三个数据端点与站点元数据端点。响应体本身不带 buildId，
	// citation 统一由 /api/hexdata/meta 的快照提供。
	hexdataMetaPath            = "/api/hexdata/meta"
	hexdataPostmatchPath       = "/api/hexdata/postmatch"
	hexdataHextechInsightsPath = "/api/hexdata/hextech-insights"
	hexdataHeroJSONPathPrefix  = "/api/hexdata/heroes/"
	// 两个目录型聚合文件与 /heroes 榜单同性质：每个 buildID 只取一次，id 固定 all。
	hexdataAggregateID = "all"
	// citation 的 CanonicalURL 必须是给人看的 HTML 页面，不能指向 JSON 端点。
	// /heroes 是站内已在用的真实榜单页；工单举例的 /hero-analysis 未经验证存在。
	hexdataAggregateCanonical = "/heroes"
	// 上游 trios / terminalItemTrios 各 1000 条（meta.heroTrioLimitPerHero 是硬上限，
	// 不是采样），解析层按 games 降序只保留前 50 条，压掉单英雄的内存膨胀。
	hexdataTrioKeepLimit = 50
	// 聚合文件规模护栏：实测 173 英雄 / 211 海克斯，留出英雄上下架的正常波动。
	hexdataAggregateHeroMin    = 150
	hexdataAggregateHeroMax    = 200
	hexdataAggregateAugmentMin = 190
	hexdataAggregateAugmentMax = 230
)

func hexdataMeasurementTechnique(answer hexdataAnswerCards) string {
	return strings.TrimSpace(answer.Methodology.MeasurementTechnique)
}

var (
	hexdataHeroPathPattern    = regexp.MustCompile(`^/hero/([0-9]+)-([a-z0-9-]+)$`)
	hexdataAugmentPathPattern = regexp.MustCompile(`^/augment/([0-9]+)-([a-z0-9-]+)$`)
	// R116-A：JSON API 的单英雄端点。只放行带数字 ID 的详情页，
	// /api/hexdata/heroes（无 ID 的列表接口）上游已 403 拦截并明文警告，绝不放行。
	hexdataHeroJSONPathPattern = regexp.MustCompile(`^/api/hexdata/heroes/([0-9]+)$`)
	hexdataHeroMetricPattern   = regexp.MustCompile(`胜率\s*([0-9]+(?:\.[0-9]+)?)%\s*[·，,]\s*样本\s*([0-9,]+)`)
	hexdataChampionNickname    = regexp.MustCompile(`\s*[\(（][^()（）]*[\)）]\s*$`)
	hexdataPatchPattern        = regexp.MustCompile(`Patch\s+([0-9]+\.[0-9]+)`)
	hexdataDatePattern         = regexp.MustCompile(`数据日期\s*([0-9]{4}-[0-9]{2}-[0-9]{2})`)
	hexdataGlobalMetric        = regexp.MustCompile(`globalHexScore\s*([0-9]+(?:\.[0-9]+)?)\s*[·，,]\s*胜率\s*([0-9]+(?:\.[0-9]+)?)%`)
	hexdataAugmentWinRate      = regexp.MustCompile(`胜率\s*([0-9]+(?:\.[0-9]+)?)%`)
	opggRSCVersionPattern      = regexp.MustCompile(`/meta/images/lol/([0-9]+\.[0-9]+(?:\.[0-9]+)?)/`)
	opggRSCLinePattern         = regexp.MustCompile(`(?m)^([0-9a-f]+):(.*)$`)
	opggRSCReferencePattern    = regexp.MustCompile(`"\$L([0-9a-f]+)"`)
	opggRSCMetaIDPattern       = regexp.MustCompile(`"metaId":([0-9]+)`)
	opggRSCMetaKindIDPattern   = regexp.MustCompile(`"metaType":"([^"]+)"\s*,\s*"metaId":([0-9]+)`)
	opggRSCMetaIDKindPattern   = regexp.MustCompile(`"metaId":([0-9]+)\s*,\s*"metaType":"([^"]+)"`)
	opggRSCAugmentIDPattern    = regexp.MustCompile(`"metaId":([0-9]+),"metaType":"aram-augment"`)
	opggRSCSkillPattern        = regexp.MustCompile(`"skill_[0-9]+"[\s\S]{0,600}?"extraData":"([QWER])"`)
	opggRSCSkillOrderPattern   = regexp.MustCompile(`"span","[0-9]+"[\s\S]{0,320}?"children":"([QWER])"`)
	opggRSCSpellPairPattern    = regexp.MustCompile(`(?m)^[0-9a-f]+:(.*"spell_0".*"spell_1".*)$`)
	opggRSCSpellMetaPattern    = regexp.MustCompile(`(?:"metaId":([0-9]+)\s*,\s*"metaType":"spell"|"metaType":"spell"\s*,\s*"metaId":([0-9]+))`)
	opggRSCMetaNamePattern     = regexp.MustCompile(`"(?:name|title)":"([^"]+)"`)
	opggRSCMetaIconPattern     = regexp.MustCompile(`"(?:src|icon|iconUrl|image_url)":"([^"]+\.(?:png|jpg|jpeg|webp))"`)
	hexdataGlobalGate          = make(chan struct{}, 3)
	hexdataGlobalPaceMu        sync.Mutex
	hexdataGlobalLastRequest   time.Time
)

type championSourceCitation struct {
	Source       string `json:"source"`
	Patch        string `json:"patch,omitempty"`
	ReportDate   string `json:"reportDate,omitempty"`
	BuildID      string `json:"buildId,omitempty"`
	CanonicalURL string `json:"canonicalUrl"`
}

type hexdataMethodology struct {
	MeasurementTechnique string `json:"measurementTechnique,omitempty"`
	SamplePolicy         struct {
		Low struct {
			Min          int `json:"min"`
			MaxExclusive int `json:"maxExclusive"`
		} `json:"low"`
		Medium struct {
			Min          int `json:"min"`
			MaxExclusive int `json:"maxExclusive"`
		} `json:"medium"`
		High struct {
			Min int `json:"min"`
		} `json:"high"`
	} `json:"samplePolicy"`
	RecommendationPolicy struct {
		Ranking                string  `json:"ranking"`
		WilsonZ                float64 `json:"wilsonZ"`
		MinRecommendationGames int     `json:"minRecommendationGames"`
	} `json:"recommendationPolicy"`
}

type hexdataAnswerCards struct {
	BuildID     string             `json:"buildId"`
	ReportPatch string             `json:"reportPatch"`
	ReportDate  string             `json:"reportDate"`
	Methodology hexdataMethodology `json:"methodology"`
	HeroAliases []struct {
		ID             string   `json:"id"`
		URL            string   `json:"url"`
		Name           string   `json:"name"`
		AlternateNames []string `json:"alternateNames"`
	} `json:"heroAliases"`
}

type hexdataState struct {
	BuildID         string                         `json:"buildId,omitempty"`
	PreviousBuildID string                         `json:"previousBuildId,omitempty"`
	ReportPatch     string                         `json:"reportPatch,omitempty"`
	ReportDate      string                         `json:"reportDate,omitempty"`
	BuildChecked    time.Time                      `json:"buildChecked,omitempty"`
	CircuitUntil    time.Time                      `json:"circuitUntil,omitempty"`
	CircuitProbeAt  time.Time                      `json:"circuitProbeAt,omitempty"`
	Failures        int                            `json:"failures,omitempty"`
	Circuits        map[string]hexdataCircuitState `json:"circuits,omitempty"`
	ETags           map[string]string              `json:"etags,omitempty"`
	LastModified    map[string]string              `json:"lastModified,omitempty"`
}

type hexdataCircuitState struct {
	Until       time.Time `json:"until,omitempty"`
	ProbeAt     time.Time `json:"probeAt,omitempty"`
	LastFailure time.Time `json:"lastFailure,omitempty"`
	Failures    int       `json:"failures,omitempty"`
}

type hexdataPayloadShape struct {
	Rows    int
	Fields  int
	BuildID string
}

type hexdataShapeValidationError struct {
	shape hexdataPayloadShape
	err   error
}

func (e *hexdataShapeValidationError) Error() string {
	return "hexdata shape validation failed: " + e.err.Error()
}

func (e *hexdataShapeValidationError) Unwrap() error { return e.err }

var errHexdataEmptyPayload = errors.New("hexdata returned an empty response")

type hexdataClient struct {
	provider      *championProvider
	mu            sync.Mutex
	state         hexdataState
	statePath     string
	minInterval   time.Duration
	maximumJitter time.Duration
	retryDelay    time.Duration
	now           func() time.Time
	sleep         func(context.Context, time.Duration) error
	jitter        func(time.Duration) time.Duration
	probeInFlight map[string]bool
	// R116-A P3：拿到新 buildID 后异步回收旧 buildID 的 hexdata- 落盘文件。
	// prunedBuildID 保证每个 (进程, buildID) 只扫一次盘，pruneWait 供测试等待。
	pruneMu       sync.Mutex
	prunedBuildID string
	pruneWait     sync.WaitGroup
}

type hexdataPage struct {
	Data      []byte
	FetchedAt time.Time
	Cache     string
}

type hexdataHeroRow struct {
	ID       int
	Slug     string
	Name     string
	WinRate  float64
	Games    int
	Wilson   float64
	TierBand int
}

// R116-A：JSON API 的响应体不含 buildId/citation，站点元数据由
// /api/hexdata/meta 单独提供。三个 JSON kind（hero-json / postmatch /
// hextech-insights）的 citation 全部从这份快照拼，不从各自 payload 现掰。
type hexdataMetaSnapshot struct {
	BuildID      string `json:"buildId"`
	ReportPatch  string `json:"reportPatch"`
	ReportDate   string `json:"reportDate"`
	HeroCount    int    `json:"heroCount"`
	AugmentCount int    `json:"augmentCount"`
	// R116-B P0-2：samplePolicy 的三档阈值必须从 meta 读，不许硬编码
	// 250/1000（实测 low{min:1,maxExclusive:250}、medium{min:250,
	// maxExclusive:1000}、high{min:1000}，上游改口径时这里自动跟随）。
	SamplePolicy hexdataSamplePolicy `json:"samplePolicy"`
	// R116-B P0-5：官方 tier 1~5 的中文标签（前15% / 15%-35% / …），
	// 前端渲染官方档位时用它，不自己造文案。
	TierBands []hexdataTierBand `json:"tierBands"`
}

// hexdataSamplePolicy 对应 meta.samplePolicy。三个档位的 min/maxExclusive 全部
// 原样解析：maxExclusive 缺失（high 档就没有）表示上不封顶。
type hexdataSamplePolicy struct {
	Low    hexdataSampleBand `json:"low"`
	Medium hexdataSampleBand `json:"medium"`
	High   hexdataSampleBand `json:"high"`
}

type hexdataSampleBand struct {
	Min          int `json:"min"`
	MaxExclusive int `json:"maxExclusive"`
}

// hexdataTierBand 对应 meta.tierBands[]。
type hexdataTierBand struct {
	Tier  int    `json:"tier"`
	Label string `json:"label"`
}

// usable 判断这份 samplePolicy 能不能用来分档。上游把字段改名或漏发时阈值会是
// 0，此时宁可返回「不可用」让 sampleTier 留空、前端整块隐藏，也不能拿 0 当门槛
// 把所有行都判成 high。
func (policy hexdataSamplePolicy) usable() bool {
	return policy.Medium.Min > 0 && policy.High.Min > 0 && policy.High.Min >= policy.Medium.Min
}

// sampleTier 按 meta.samplePolicy 给一行样本分档。返回 "" 表示「阈值不可用」，
// 调用方必须把它当成「这个维度取不到」处理（championMetricRow.SampleTier 留空
// → omitempty → 前端不渲染任何样本分档元素）。games<=0 同样返回 ""：没有样本
// 就谈不上样本分档，编一个 "low" 也是伪造。
func (policy hexdataSamplePolicy) sampleTier(games int) string {
	if !policy.usable() || games <= 0 {
		return ""
	}
	switch {
	case games >= policy.High.Min:
		return "high"
	case games >= policy.Medium.Min:
		return "medium"
	default:
		return "low"
	}
}

// tierBands 把 meta 的官方档位标签搬进站内响应；阈值缺失时返回 nil，
// 前端据此整块不渲染。
func (snapshot hexdataMetaSnapshot) tierBands() []championTierBand {
	if len(snapshot.TierBands) == 0 {
		return nil
	}
	result := make([]championTierBand, 0, len(snapshot.TierBands))
	for _, band := range snapshot.TierBands {
		if strings.TrimSpace(band.Label) == "" {
			continue
		}
		result = append(result, championTierBand{Tier: band.Tier, Label: band.Label})
	}
	return result
}

// applyHexdataSampleTiers 给一批响应行补 sampleTier。阈值只来自 meta 快照，
// 前端不重复实现分档逻辑（工单 P0-2-1）。
// R116-B P0-6 起同时给行内的 stages[] 补 sampleTier：阶段视图里的低样本徽记
// 与父行同源，否则「阶段 3 只有 373 场」这种行会被当成正常样本展示。
// 阶段行没有 GamesUnavailable 这个回退标记（它只属于 QQ101 装备行），所以
// 一律按 games 分档。
func applyHexdataSampleTiers(rows []championMetricRow, snapshot hexdataMetaSnapshot) {
	if !snapshot.SamplePolicy.usable() {
		return
	}
	for index := range rows {
		for stageIndex := range rows[index].Stages {
			rows[index].Stages[stageIndex].SampleTier = snapshot.SamplePolicy.sampleTier(rows[index].Stages[stageIndex].Games)
		}
		if rows[index].GamesUnavailable {
			continue
		}
		rows[index].SampleTier = snapshot.SamplePolicy.sampleTier(rows[index].Games)
	}
}

// hexdataHeroDetailV2 是 /api/hexdata/heroes/{id} 的解析结果（R116-A P1-1）。
// 上游把所有 ID 写成字符串，这里统一转成 int；转换失败的整条记录被丢弃，
// 绝不用 0 顶替——0 会指向另一个真实存在的英雄/装备/海克斯，属于伪造数据。
type hexdataHeroDetailV2 struct {
	Items              []hexdataItemRow      `json:"items"`
	Augments           []hexdataAugmentRowV2 `json:"augments"`
	Trios              []hexdataTrioRow      `json:"trios"`
	WeakAgainst        []hexdataMatchupRow   `json:"weakAgainst"`
	StrongAgainst      []hexdataMatchupRow   `json:"strongAgainst"`
	TeammateSynergies  []hexdataSynergyRow   `json:"teammateSynergies"`
	TerminalItemTrios  []hexdataTrioRow      `json:"terminalItemTrios"`
	SummonerSpellPairs []hexdataSpellPairRow `json:"summonerSpellPairs"`
	// HeroWinRate 是官方 heroWinRate（augments[]/trios[] 每条都带，0..1；items[]
	// 没有这个字段），已换算成百分数。旧 HTML 页的「胜率 57.8%」在 JSON 里没有
	// 英雄级等价字段，这是唯一真实来源，取第一个非零值，不做任何推算。
	HeroWinRate float64 `json:"heroWinRate,omitempty"`
}

// hexdataItemRow 对应 items[]：itemId 是真实资产 ID，不再靠中文名反查。
type hexdataItemRow struct {
	ItemID             int     `json:"itemId"`
	ItemName           string  `json:"itemName"`
	WinRate            float64 `json:"winRate"`
	PickRate           float64 `json:"pickRate"`
	Games              int     `json:"games"`
	Tier               int     `json:"tier"`
	HexTier            string  `json:"hexTier"`
	HexLabel           string  `json:"hexLabel"`
	HexTierColor       string  `json:"hexTierColor"`
	HexScore           float64 `json:"hexScore"`
	DeltaWinRate       float64 `json:"deltaWinRate"`
	CoreDelta          float64 `json:"coreDelta"`
	AverageIndex       float64 `json:"averageIndex"`
	WilsonLowerWinRate float64 `json:"wilsonLowerWinRate"`
	WithoutItemWinRate float64 `json:"withoutItemWinRate"`
	WithoutItemGames   int     `json:"withoutItemGames"`
}

// hexdataAugmentRowV2 对应 augments[]：augmentDescription 实测 126/126 满覆盖，
// 所以旧的 per-augment 描述扇出（fetchMayhemAugmentCopy）整条链路已删除。
// Rarity 是上游的中文串（棱彩/黄金/白银），原样保留；站内响应字段用的是
// silver/gold/prismatic，映射在 hexdataRarityLabel 里做。
type hexdataAugmentRowV2 struct {
	AugmentID           int                      `json:"augmentId"`
	AugmentName         string                   `json:"augmentName"`
	AugmentDescription  string                   `json:"augmentDescription"`
	Rarity              string                   `json:"rarity"`
	Tier                int                      `json:"tier"`
	Score               float64                  `json:"score"`
	HexTier             string                   `json:"hexTier"`
	HexLabel            string                   `json:"hexLabel"`
	HexTierColor        string                   `json:"hexTierColor"`
	HexScore            float64                  `json:"hexScore"`
	PickRate            float64                  `json:"pickRate"`
	PairWinRate         float64                  `json:"pairWinRate"`
	HeroWinRate         float64                  `json:"heroWinRate"`
	DeltaWinRate        float64                  `json:"deltaWinRate"`
	WilsonLowerWinRate  float64                  `json:"wilsonLowerWinRate"`
	RecommendationScore float64                  `json:"recommendationScore"`
	Wins                int                      `json:"wins"`
	Games               int                      `json:"games"`
	Stages              []hexdataAugmentStageRow `json:"stages"`
}

// hexdataAugmentStageRow 对应 augments[].stages[]（恒 4 条，stage = 1..4）。
// R116-F 的阶段×稀有度概率分布直接消费这个结构。
type hexdataAugmentStageRow struct {
	Stage                int     `json:"stage"`
	Tier                 int     `json:"tier"`
	HexTier              string  `json:"hexTier"`
	HexLabel             string  `json:"hexLabel"`
	HexTierColor         string  `json:"hexTierColor"`
	HexScore             float64 `json:"hexScore"`
	WinRate              float64 `json:"winRate"`
	PickRate             float64 `json:"pickRate"`
	DeltaWinRate         float64 `json:"deltaWinRate"`
	WilsonLowerWinRate   float64 `json:"wilsonLowerWinRate"`
	RecommendationScore  float64 `json:"recommendationScore"`
	StageBaselineWinRate float64 `json:"stageBaselineWinRate"`
	Wins                 int     `json:"wins"`
	Games                int     `json:"games"`
}

// hexdataTrioRow 同时承载 trios[]（海克斯三件套）与 terminalItemTrios[]
// （成型装备三件套）：上游两者只差 ID/名称字段名，形状一致。
// 解析后各自按 games 降序裁剪到 hexdataTrioKeepLimit 条。
type hexdataTrioRow struct {
	TrioKey            string   `json:"trioKey"`
	AugmentIDs         []int    `json:"augmentIds,omitempty"`
	AugmentNames       []string `json:"augmentNames,omitempty"`
	ItemIDs            []int    `json:"itemIds,omitempty"`
	ItemNames          []string `json:"itemNames,omitempty"`
	WinRate            float64  `json:"winRate"`
	PickRate           float64  `json:"pickRate"`
	DeltaWinRate       float64  `json:"deltaWinRate"`
	HeroWinRate        float64  `json:"heroWinRate"`
	WinRateTier        int      `json:"winRateTier,omitempty"`
	PickRateTier       int      `json:"pickRateTier,omitempty"`
	Tier               int      `json:"tier,omitempty"`
	WilsonLowerWinRate float64  `json:"wilsonLowerWinRate,omitempty"`
	Wins               int      `json:"wins"`
	Games              int      `json:"games"`
}

// hexdataMatchupRow 对应 weakAgainst[] / strongAgainst[]。上游只发布通过多重
// 比较校正的显著项（实测各 7 条、evidence 全为 supported、qValue 全为 0），
// 我们不放宽阈值，也不补算不存在的效果量。
type hexdataMatchupRow struct {
	OpponentChampionID     int     `json:"opponentChampionId"`
	OpponentName           string  `json:"opponentName"`
	OpponentHref           string  `json:"opponentHref"`
	CounterDelta           float64 `json:"counterDelta"`
	Evidence               string  `json:"evidence"`
	ConfidenceLow          float64 `json:"confidenceLow"`
	ConfidenceHigh         float64 `json:"confidenceHigh"`
	QValue                 float64 `json:"qValue"`
	AdjustedWinRate        float64 `json:"adjustedWinRate"`
	ExpectedWinRate        float64 `json:"expectedWinRate"`
	ObservedWinRate        float64 `json:"observedWinRate"`
	TargetWinRateDrop      float64 `json:"targetWinRateDrop"`
	ExpectedTargetWinRate  float64 `json:"expectedTargetWinRate"`
	TargetAdjustedWinRate  float64 `json:"targetAdjustedWinRate"`
	TargetBaselineWinRate  float64 `json:"targetBaselineWinRate"`
	CounterBaselineWinRate float64 `json:"counterBaselineWinRate"`
	Wins                   int     `json:"wins"`
	Games                  int     `json:"games"`
}

// hexdataSynergyRow 对应 teammateSynergies[]：形状与对位一致，键名换成
// teammate*，效果量是 synergyDelta，另带 pValue 与双方基线胜率。
type hexdataSynergyRow struct {
	TeammateChampionID    int     `json:"teammateChampionId"`
	TeammateName          string  `json:"teammateName"`
	TeammateHref          string  `json:"teammateHref"`
	SynergyDelta          float64 `json:"synergyDelta"`
	Evidence              string  `json:"evidence"`
	ConfidenceLow         float64 `json:"confidenceLow"`
	ConfidenceHigh        float64 `json:"confidenceHigh"`
	PValue                float64 `json:"pValue"`
	QValue                float64 `json:"qValue"`
	AdjustedWinRate       float64 `json:"adjustedWinRate"`
	ExpectedWinRate       float64 `json:"expectedWinRate"`
	ObservedWinRate       float64 `json:"observedWinRate"`
	FirstBaselineWinRate  float64 `json:"firstBaselineWinRate"`
	SecondBaselineWinRate float64 `json:"secondBaselineWinRate"`
	Wins                  int     `json:"wins"`
	Games                 int     `json:"games"`
}

// hexdataSpellPairRow 对应 summonerSpellPairs[]（实测 28 条）。
type hexdataSpellPairRow struct {
	SpellIDs           []int    `json:"spellIds"`
	SpellNames         []string `json:"spellNames"`
	SpellKey           string   `json:"spellKey"`
	WinRate            float64  `json:"winRate"`
	PickRate           float64  `json:"pickRate"`
	DeltaWinRate       float64  `json:"deltaWinRate"`
	HeroWinRate        float64  `json:"heroWinRate"`
	WilsonLowerWinRate float64  `json:"wilsonLowerWinRate"`
	Tier               int      `json:"tier"`
	Wins               int      `json:"wins"`
	Games              int      `json:"games"`
}

// hexdataHeroJSONStats 记录解析层丢了多少条、裁了多少条，供诊断事件使用：
// 上游改字段名时这里会先于用户可见的空白暴露出来。
type hexdataHeroJSONStats struct {
	DroppedItems             int
	DroppedAugments          int
	DroppedTrios             int
	DroppedTerminalItemTrios int
	DroppedMatchups          int
	DroppedSynergies         int
	DroppedSpellPairs        int
	TrimmedTrios             int
	TrimmedTerminalItemTrios int
}

func (s hexdataHeroJSONStats) dropped() int {
	return s.DroppedItems + s.DroppedAugments + s.DroppedTrios + s.DroppedTerminalItemTrios +
		s.DroppedMatchups + s.DroppedSynergies + s.DroppedSpellPairs
}

func (s hexdataHeroJSONStats) addToDiagnostic(event map[string]any) {
	event["dropped_items"] = s.DroppedItems
	event["dropped_augments"] = s.DroppedAugments
	event["dropped_trios"] = s.DroppedTrios
	event["dropped_terminal_item_trios"] = s.DroppedTerminalItemTrios
	event["dropped_matchups"] = s.DroppedMatchups
	event["dropped_synergies"] = s.DroppedSynergies
	event["dropped_spell_pairs"] = s.DroppedSpellPairs
	event["trimmed_trios"] = s.TrimmedTrios
	event["trimmed_terminal_item_trios"] = s.TrimmedTerminalItemTrios
}

// 以下 *Raw 结构只用于反序列化：把上游的字符串 ID 接住，再安全转换成 int。
// 嵌入的目标结构与 Raw 字段带同一个 JSON tag，encoding/json 取嵌套更浅的那个，
// 所以 ID 字段只会落进 Raw，转换成功后才写回目标结构。
type hexdataHeroJSONPayload struct {
	Items              []hexdataItemRowRaw      `json:"items"`
	Trios              []hexdataTrioRowRaw      `json:"trios"`
	Augments           []hexdataAugmentRowRaw   `json:"augments"`
	WeakAgainst        []hexdataMatchupRowRaw   `json:"weakAgainst"`
	StrongAgainst      []hexdataMatchupRowRaw   `json:"strongAgainst"`
	TeammateSynergies  []hexdataSynergyRowRaw   `json:"teammateSynergies"`
	TerminalItemTrios  []hexdataTrioRowRaw      `json:"terminalItemTrios"`
	SummonerSpellPairs []hexdataSpellPairRowRaw `json:"summonerSpellPairs"`
}

type hexdataItemRowRaw struct {
	hexdataItemRow
	RawItemID string `json:"itemId"`
}

type hexdataAugmentRowRaw struct {
	hexdataAugmentRowV2
	RawAugmentID string `json:"augmentId"`
}

type hexdataTrioRowRaw struct {
	hexdataTrioRow
	RawAugmentIDs []string `json:"augmentIds"`
	RawItemIDs    []string `json:"itemIds"`
}

type hexdataMatchupRowRaw struct {
	hexdataMatchupRow
	RawOpponentChampionID string `json:"opponentChampionId"`
}

type hexdataSynergyRowRaw struct {
	hexdataSynergyRow
	RawTeammateChampionID string `json:"teammateChampionId"`
}

type hexdataSpellPairRowRaw struct {
	hexdataSpellPairRow
	RawSpellIDs []string `json:"spellIds"`
}

// hexdataPostmatchRow 是 /api/hexdata/postmatch 里一个英雄的 22 项赛后指标，
// 字段与上游一一对应，不做裁剪也不做换算。
type hexdataPostmatchRow struct {
	KDA               float64 `json:"kda"`
	AvgKills          float64 `json:"avgKills"`
	AvgDeaths         float64 `json:"avgDeaths"`
	AvgAssists        float64 `json:"avgAssists"`
	AvgCs             float64 `json:"avgCs"`
	AvgGold           float64 `json:"avgGold"`
	AvgDamage         float64 `json:"avgDamage"`
	AvgDamageTaken    float64 `json:"avgDamageTaken"`
	AvgDmgMitigated   float64 `json:"avgDmgMitigated"`
	AvgHealShield     float64 `json:"avgHealShield"`
	AvgTurretDmg      float64 `json:"avgTurretDmg"`
	AvgCcTime         float64 `json:"avgCcTime"`
	DamageShare       float64 `json:"damageShare"`
	GoldEfficiency    float64 `json:"goldEfficiency"`
	KillParticipation float64 `json:"killParticipation"`
	MultiKillRate     float64 `json:"multiKillRate"`
	DoubleKills       int     `json:"doubleKills"`
	TripleKills       int     `json:"tripleKills"`
	QuadraKills       int     `json:"quadraKills"`
	PentaKills        int     `json:"pentaKills"`
	Survivability     float64 `json:"survivability"`
	UtilityScore      float64 `json:"utilityScore"`
}

// hexdataPostmatch 是交给上层消费的形状：英雄 ID → 22 项指标，外加 citation。
type hexdataPostmatch struct {
	Citation  championSourceCitation
	FetchedAt time.Time
	Heroes    map[int]hexdataPostmatchRow
	// R116-B P1-6：英雄 ID → 该英雄在 JSON 里实际出现过的字段名集合。
	// 用途见 parseHexdataPostmatchWithPresence 的注释：没有它就无法区分
	// 「上游给了 0」和「上游没给这个字段」，而后者绝不能渲染成 0。
	Present map[int]map[string]bool
}

// hexdataInsightHero / hexdataInsightAugment 是 /api/hexdata/hextech-insights
// 的两个数组元素。topItems / topAugments 各 5 条，带 confidence，
// R116-D 的推荐页概览直接消费，不需要再逐个英雄调 /heroes/{id}。
type hexdataInsightEntry struct {
	// AssetID 是 ID 的安全 int 转换结果（装备 ID 或海克斯 ID）；转换失败的条目
	// 在解析层保留字符串原样、AssetID 留 0，调用方不得把 0 当成真实资产。
	AssetID      int     `json:"-"`
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Games        int     `json:"games"`
	WinRate      float64 `json:"winRate"`
	PickRate     float64 `json:"pickRate"`
	Confidence   string  `json:"confidence"`
	Rarity       string  `json:"rarity,omitempty"`
	Reason       string  `json:"reason,omitempty"`
	DetailURL    string  `json:"detailUrl,omitempty"`
	PairWinRate  float64 `json:"pairWinRate,omitempty"`
	DeltaWinRate float64 `json:"deltaWinRate,omitempty"`
}

type hexdataInsightHero struct {
	ID string `json:"id"`
	// ChampionID 是 ID 的安全 int 转换结果；ID 非法的整条在解析层被丢弃。
	ChampionID  int                   `json:"-"`
	Name        string                `json:"name"`
	Tier        int                   `json:"tier"`
	Games       int                   `json:"games"`
	WinRate     float64               `json:"winRate"`
	PickRate    float64               `json:"pickRate"`
	DetailURL   string                `json:"detailUrl"`
	Confidence  string                `json:"confidence"`
	TopItems    []hexdataInsightEntry `json:"topItems"`
	TopAugments []hexdataInsightEntry `json:"topAugments"`
}

type hexdataInsightAugment struct {
	ID string `json:"id"`
	// AugmentID 是 ID 的安全 int 转换结果；ID 非法的整条在解析层被丢弃。
	AugmentID         int     `json:"-"`
	Name              string  `json:"name"`
	Tier              int     `json:"tier"`
	Games             int     `json:"games"`
	Rarity            string  `json:"rarity"`
	WinRate           float64 `json:"winRate"`
	PickRate          float64 `json:"pickRate"`
	DetailURL         string  `json:"detailUrl"`
	Confidence        string  `json:"confidence"`
	AvgDeltaWinRate   float64 `json:"avgDeltaWinRate"`
	CoverageHeroCount int     `json:"coverageHeroCount"`
}

type hexdataHextechInsightsPayload struct {
	Heroes        []hexdataInsightHero    `json:"heroes"`
	Augments      []hexdataInsightAugment `json:"augments"`
	ReportPatch   string                  `json:"reportPatch"`
	ReportDate    string                  `json:"reportDate"`
	HeroCount     int                     `json:"heroCount"`
	AugmentCount  int                     `json:"augmentCount"`
	GeneratedAt   string                  `json:"generatedAt"`
	SchemaVersion string                  `json:"schemaVersion"`
}

// hexdataHextechInsights 是交给上层消费的形状：payload 内嵌（可直接
// insights.Heroes）+ meta 快照拼出的 citation。
type hexdataHextechInsights struct {
	hexdataHextechInsightsPayload
	Citation  championSourceCitation
	FetchedAt time.Time
}

type hexdataAugmentParseStats struct {
	SkipShortCells       int
	SkipPathUnmatched    int
	SkipMetricsUnmatched int
	SkipEmptyName        int
	SkipBadID            int
	Col0LinkTextLen      int
	Col0TextLen          int
	Col2LinkTextLen      int
	Col2TextLen          int
}

func (s hexdataAugmentParseStats) addToDiagnostic(event map[string]any) {
	event["skip_short_cells"] = s.SkipShortCells
	event["skip_path_unmatched"] = s.SkipPathUnmatched
	event["skip_metrics_unmatched"] = s.SkipMetricsUnmatched
	event["skip_empty_name"] = s.SkipEmptyName
	event["skip_bad_id"] = s.SkipBadID
	event["col0_link_text_len"] = s.Col0LinkTextLen
	event["col0_text_len"] = s.Col0TextLen
	event["col2_link_text_len"] = s.Col2LinkTextLen
	event["col2_text_len"] = s.Col2TextLen
}

type hexdataAugmentDetail struct {
	Citation    championSourceCitation
	Description string
	Rows        []championAugmentChampion
}

type hexdataRarityStage struct {
	Stage     int     `json:"stage"`
	Silver    float64 `json:"silver"`
	Gold      float64 `json:"gold"`
	Prismatic float64 `json:"prismatic"`
	Games     int     `json:"games"`
}

type championAugmentDetailResponse struct {
	ID          int                     `json:"id"`
	Slug        string                  `json:"slug"`
	Source      string                  `json:"source"`
	Description string                  `json:"description,omitempty"`
	Citation    *championSourceCitation `json:"citation,omitempty"`
	// R128 §2.3：前端不展示（海克斯图鉴详情不再渲染口径说明）；字段保留供诊断与既有后端测试使用。
	MeasurementTechnique string                    `json:"measurementTechnique,omitempty"`
	Champions            []championAugmentChampion `json:"champions"`
}

type championAugmentRarityResponse struct {
	Source   string                  `json:"source"`
	Citation *championSourceCitation `json:"citation,omitempty"`
	// R128 §2.3：前端不展示（全服品质分布面板不再渲染口径说明）；字段保留供诊断与既有后端测试使用。
	MeasurementTechnique string               `json:"measurementTechnique,omitempty"`
	Stages               []hexdataRarityStage `json:"stages"`
}

func newHexdataClient(provider *championProvider, store *localStore) *hexdataClient {
	client := &hexdataClient{
		provider: provider, minInterval: hexdataMinimumInterval, maximumJitter: hexdataMaximumJitter,
		retryDelay: hexdataRetryDelay, now: time.Now,
		state: hexdataState{
			Circuits:     make(map[string]hexdataCircuitState),
			ETags:        make(map[string]string),
			LastModified: make(map[string]string),
		},
		probeInFlight: make(map[string]bool),
		jitter: func(maximum time.Duration) time.Duration {
			return time.Duration(rand.Int64N(int64(maximum) + 1))
		},
		sleep: func(ctx context.Context, duration time.Duration) error {
			timer := time.NewTimer(duration)
			defer timer.Stop()
			select {
			case <-timer.C:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}
	if store != nil {
		client.statePath = filepath.Join(store.root, championDataCacheDirectory, "hexdata-state.json")
		client.loadState()
	}
	return client
}

func (h *hexdataClient) loadState() {
	data, err := os.ReadFile(h.statePath)
	if err != nil || len(data) > 64<<10 || json.Unmarshal(data, &h.state) != nil {
		return
	}
	if h.state.ETags == nil {
		h.state.ETags = make(map[string]string)
	}
	if h.state.LastModified == nil {
		h.state.LastModified = make(map[string]string)
	}
	if h.state.Circuits == nil {
		h.state.Circuits = make(map[string]hexdataCircuitState)
	}
	// Old releases persisted one global circuit without the failing data kind.
	// It cannot be migrated safely because an augments failure must not block
	// heroes or hero-detail, so clear it once during the state-schema upgrade.
	if !h.state.CircuitUntil.IsZero() || !h.state.CircuitProbeAt.IsZero() || h.state.Failures != 0 {
		h.state.CircuitUntil = time.Time{}
		h.state.CircuitProbeAt = time.Time{}
		h.state.Failures = 0
		h.saveStateLocked()
		if h.provider != nil && h.provider.diag != nil {
			h.provider.diag(map[string]any{"event": "hexdata_circuit_reset", "reason": "legacy global circuit removed"})
		}
	}
	// A local restart is an explicit retry signal. Do not carry a stale
	// circuit lock across restarts once it has outlived a normal work session.
	for kind, circuit := range h.state.Circuits {
		remaining := circuit.Until.Sub(h.now())
		if !circuit.Until.IsZero() && (remaining > 6*time.Hour || remaining < -6*time.Hour) {
			delete(h.state.Circuits, kind)
			continue
		}
		if !circuit.Until.IsZero() && circuit.ProbeAt.IsZero() {
			circuit.ProbeAt = h.now()
			h.state.Circuits[kind] = circuit
		}
	}
}

func (h *hexdataClient) saveStateLocked() {
	if h.statePath == "" {
		return
	}
	data, err := json.Marshal(h.state)
	if err == nil {
		_ = atomicWriteFile(h.statePath, data, 0o600)
	}
}

func (h *hexdataClient) snapshot() hexdataState {
	h.mu.Lock()
	defer h.mu.Unlock()
	state := h.state
	state.Circuits = make(map[string]hexdataCircuitState, len(h.state.Circuits))
	for key, value := range h.state.Circuits {
		state.Circuits[key] = value
	}
	state.ETags = make(map[string]string, len(h.state.ETags))
	for key, value := range h.state.ETags {
		state.ETags[key] = value
	}
	state.LastModified = make(map[string]string, len(h.state.LastModified))
	for key, value := range h.state.LastModified {
		state.LastModified[key] = value
	}
	return state
}

func (h *hexdataClient) circuitSnapshot(kind string) hexdataCircuitState {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state.Circuits[kind]
}

func inspectHexdataPayload(kind, requestPath string, data []byte, reporters ...func(map[string]any)) (hexdataPayloadShape, error) {
	shape := hexdataPayloadShape{}
	if len(strings.TrimSpace(string(data))) == 0 {
		return shape, errHexdataEmptyPayload
	}
	switch kind {
	case "answer":
		var answer hexdataAnswerCards
		if err := json.Unmarshal(data, &answer); err != nil {
			return shape, err
		}
		shape = hexdataPayloadShape{Rows: len(answer.HeroAliases), Fields: len(answer.HeroAliases), BuildID: answer.BuildID}
		if !validHexdataAnswer(answer) {
			return shape, errors.New("hexdata answer cards changed")
		}
	case "heroes":
		rows, citation, err := parseHexdataHeroes(data, hexdataAnswerCards{})
		shape = hexdataPayloadShape{Rows: len(rows), Fields: hexdataTableFieldCount(data), BuildID: citation.BuildID}
		if err != nil || !hexdataCitationComplete(citation) {
			return shape, firstError(err, errors.New("hexdata heroes citation is incomplete"))
		}
	case "augments":
		rows, citation, stats, err := parseHexdataAugmentsWithStats(data)
		shape = hexdataPayloadShape{Rows: len(rows), Fields: hexdataTableFieldCount(data), BuildID: citation.BuildID}
		if err != nil || !hexdataCitationComplete(citation) {
			for _, report := range reporters {
				if report != nil {
					event := hexdataTableShapeDiagnostic("augments", data)
					stats.addToDiagnostic(event)
					report(event)
				}
			}
			return shape, firstError(err, errors.New("hexdata augments citation is incomplete"))
		}
	case "augment":
		detail, err := parseHexdataAugmentDetail(data, requestPath)
		shape = hexdataPayloadShape{Rows: len(detail.Rows), Fields: hexdataTableFieldCount(data), BuildID: detail.Citation.BuildID}
		if err != nil || !hexdataCitationComplete(detail.Citation) {
			return shape, firstError(err, errors.New("hexdata augment citation is incomplete"))
		}
	case "rarity":
		rows, citation, err := parseHexdataRarity(data)
		shape = hexdataPayloadShape{Rows: len(rows), Fields: hexdataTableFieldCount(data), BuildID: citation.BuildID}
		if err != nil || !hexdataCitationComplete(citation) {
			return shape, firstError(err, errors.New("hexdata rarity citation is incomplete"))
		}
	case "meta":
		// /api/hexdata/meta 是三个 JSON kind 唯一的 citation 来源，字段缺失就等于
		// buildId 快照不可用，必须报错而不是静默降级。
		snapshot, err := parseHexdataMeta(data)
		shape = hexdataPayloadShape{Rows: snapshot.HeroCount, Fields: snapshot.AugmentCount, BuildID: snapshot.BuildID}
		if err != nil {
			return shape, firstError(err, errors.New("hexdata meta snapshot is incomplete"))
		}
	case "hero-json":
		// 真实结构校验：items/augments/trios 三个数组都必须非空。冷门英雄没有出装
		// 数据是可能的，但 augments 为空基本等于上游改了字段名，必须被发现。
		// JSON 响应体不带 buildId，所以 shape.BuildID 保持空值，不伪造。
		var payload hexdataHeroJSONPayload
		err := json.Unmarshal(data, &payload)
		shape = hexdataPayloadShape{Rows: len(payload.Items), Fields: len(payload.Augments)}
		if err != nil {
			return shape, err
		}
		if len(payload.Items) == 0 || len(payload.Augments) == 0 || len(payload.Trios) == 0 {
			return shape, firstError(fmt.Errorf("hexdata hero-json shape is incomplete: items=%d augments=%d trios=%d", len(payload.Items), len(payload.Augments), len(payload.Trios)))
		}
	case "postmatch":
		// 顶层是 map[英雄ID字符串]22 项指标，实测 173 条。逐条校验 kda/avgDamage
		// 存在（不是只抽查一条），map 遍历顺序随机才不会让校验结果抖动。
		var raw map[string]map[string]json.RawMessage
		err := json.Unmarshal(data, &raw)
		shape = hexdataPayloadShape{Rows: len(raw), Fields: len(raw)}
		if err != nil {
			return shape, err
		}
		if len(raw) < hexdataAggregateHeroMin || len(raw) > hexdataAggregateHeroMax {
			return shape, firstError(fmt.Errorf("hexdata postmatch covers %d heroes, want %d..%d", len(raw), hexdataAggregateHeroMin, hexdataAggregateHeroMax))
		}
		for id, metrics := range raw {
			if _, ok := metrics["kda"]; !ok {
				return shape, firstError(fmt.Errorf("hexdata postmatch hero %s has no kda", id))
			}
			if _, ok := metrics["avgDamage"]; !ok {
				return shape, firstError(fmt.Errorf("hexdata postmatch hero %s has no avgDamage", id))
			}
		}
	case "hextech-insights":
		// heroes 实测 173 条、augments 实测 211 条，超出护栏说明上游改了口径。
		var insights hexdataHextechInsightsPayload
		err := json.Unmarshal(data, &insights)
		shape = hexdataPayloadShape{Rows: len(insights.Heroes), Fields: len(insights.Augments)}
		if err != nil {
			return shape, err
		}
		if len(insights.Heroes) < hexdataAggregateHeroMin || len(insights.Heroes) > hexdataAggregateHeroMax {
			return shape, firstError(fmt.Errorf("hexdata hextech-insights covers %d heroes, want %d..%d", len(insights.Heroes), hexdataAggregateHeroMin, hexdataAggregateHeroMax))
		}
		if len(insights.Augments) < hexdataAggregateAugmentMin || len(insights.Augments) > hexdataAggregateAugmentMax {
			return shape, firstError(fmt.Errorf("hexdata hextech-insights covers %d augments, want %d..%d", len(insights.Augments), hexdataAggregateAugmentMin, hexdataAggregateAugmentMax))
		}
	case "hero":
		_, buildID, err := parseHexdataDocument(data)
		shape.BuildID = buildID
		if err != nil {
			return shape, err
		}
	default:
		_, buildID, err := parseHexdataDocument(data)
		shape.BuildID = buildID
		if err != nil {
			return shape, err
		}
	}
	return shape, nil
}

func (h *hexdataClient) checkedPage(kind, id, requestPath string, page hexdataPage, err error) (hexdataPage, error) {
	if err != nil {
		return page, err
	}
	shape, shapeErr := inspectHexdataPayload(kind, requestPath, page.Data)
	if shapeErr == nil {
		return page, nil
	}
	h.recordLoadFailure(kind, &hexdataShapeValidationError{shape: shape, err: shapeErr})
	state := h.snapshot()
	if h.provider != nil && h.provider.cache != nil && state.PreviousBuildID != "" {
		previousKey := strings.Join([]string{state.PreviousBuildID, kind, id}, "|")
		if previous, previousErr := h.provider.cache.readDisk(previousKey); previousErr == nil && len(previous.Data) > 0 {
			if _, previousShapeErr := inspectHexdataPayload(kind, requestPath, previous.Data); previousShapeErr == nil {
				return hexdataPage{Data: previous.Data, FetchedAt: previous.FetchedAt, Cache: championCacheStateStale}, nil
			}
		}
	}
	return hexdataPage{}, shapeErr
}

func (h *hexdataClient) allowedPath(path string) bool {
	// R116-A：新增 JSON API 的四个端点。/api/hexdata/heroes（无 ID 的列表接口）
	// 不在白名单里——上游已 403 拦截并明文警告「请停止未经许可的自动化访问」，
	// 且 /heroes 榜单页与 /hextech-insights 已经覆盖全量英雄概览，本站不需要它。
	// 两条 HTML 正则保留：hexdataHeroPathPattern 仍被 parseHexdataHeroes /
	// parseHexdataAugmentDetail 用于解析站内链接，hexdataAugmentPathPattern 对应的
	// /augment/{id}-{slug} 详情页仍由 loadHexdataAugmentDetail 抓取。
	return path == hexdataAnswerPath || path == hexdataMetaPath || path == hexdataPostmatchPath || path == hexdataHextechInsightsPath || hexdataHeroJSONPathPattern.MatchString(path) ||
		path == "/heroes" || path == "/augments" || path == "/augment-rarity" || hexdataHeroPathPattern.MatchString(path) || hexdataAugmentPathPattern.MatchString(path)
}

func (h *hexdataClient) cacheKey(kind, id string) string {
	state := h.snapshot()
	buildID := strings.TrimSpace(state.BuildID)
	if buildID == "" {
		buildID = "bootstrap"
	}
	return strings.Join([]string{buildID, kind, id}, "|")
}

func (h *hexdataClient) load(ctx context.Context, kind, id, requestPath, accept string, forceBuildCheck bool) (hexdataPage, error) {
	if h.provider != nil && !h.provider.featureGates.enabled(featureGateHexdata) {
		return hexdataPage{}, errors.New("hexdata data source disabled by feature gate")
	}
	if !h.allowedPath(requestPath) {
		return hexdataPage{}, errors.New("hexdata request path rejected")
	}
	fetchChecked := func(loadCtx context.Context, stale []byte, probe bool) ([]byte, error) {
		data, fetchErr := h.fetch(loadCtx, kind, requestPath, accept, stale, probe)
		if fetchErr == nil {
			var report func(map[string]any)
			if h.provider != nil {
				report = h.provider.diag
			}
			shape, shapeErr := inspectHexdataPayload(kind, requestPath, data, report)
			if shapeErr != nil {
				fetchErr = &hexdataShapeValidationError{shape: shape, err: shapeErr}
			}
		}
		if fetchErr != nil {
			h.recordLoadFailure(kind, fetchErr)
		}
		return data, fetchErr
	}
	state := h.snapshot()
	circuit := state.Circuits[kind]
	probe := false
	if h.now().Before(circuit.Until) {
		now := h.now()
		h.mu.Lock()
		current := h.state.Circuits[kind]
		if current.Until.IsZero() || !now.Before(current.Until) {
			h.mu.Unlock()
		} else if now.Before(current.ProbeAt) || h.probeInFlight[kind] {
			h.mu.Unlock()
			if h.provider != nil && h.provider.diag != nil {
				module := kind
				if kind == "answer" || kind == "heroes" {
					module = "rankings"
				}
				h.provider.diag(map[string]any{"event": "hexdata_circuit_open", "module": module, "kind": kind, "until": current.Until, "probeAt": current.ProbeAt})
				h.provider.diag(map[string]any{"event": "hexdata_fallback", "module": module, "reason": "hexdata circuit is open"})
			}
			return hexdataPage{}, errors.New("hexdata circuit is open")
		} else {
			// A single periodic half-open request is allowed. Mark it before
			// releasing the lock so concurrent callers cannot stampede upstream.
			current.ProbeAt = now.Add(hexdataCircuitProbeInterval)
			h.state.Circuits[kind] = current
			h.probeInFlight[kind] = true
			h.saveStateLocked()
			h.mu.Unlock()
			probe = true
			if h.provider != nil && h.provider.diag != nil {
				module := kind
				if kind == "answer" || kind == "heroes" {
					module = "rankings"
				}
				h.provider.diag(map[string]any{"event": "hexdata_circuit_probe", "outcome": "start", "module": module, "kind": kind})
			}
		}
	}
	if probe {
		defer func() {
			h.mu.Lock()
			delete(h.probeInFlight, kind)
			h.mu.Unlock()
		}()
	}
	key := h.cacheKey(kind, id)
	ttl := hexdataCacheTTL
	refreshBuild := false
	if kind == "answer" || kind == "meta" {
		ttl = hexdataBuildSoftTTL
	} else if forceBuildCheck || state.BuildChecked.IsZero() || h.now().Sub(state.BuildChecked) >= hexdataBuildSoftTTL {
		// The requested page doubles as the build check. Never issue a separate
		// probe after the soft TTL expires.
		refreshBuild = true
	}
	var stale championCacheEnvelope
	if h.provider.cache != nil {
		stale, _ = h.provider.cache.readDisk(key)
		if probe {
			data, err := fetchChecked(ctx, stale.Data, true)
			if err != nil {
				if h.provider != nil && h.provider.diag != nil {
					h.provider.diag(map[string]any{"event": "hexdata_circuit_probe", "outcome": "failure", "kind": kind, "status": championUpstreamHTTPStatus(err), "error": err.Error()})
				}
				return hexdataPage{}, err
			}
			if h.provider != nil && h.provider.diag != nil {
				h.provider.diag(map[string]any{"event": "hexdata_circuit_probe", "outcome": "success", "kind": kind})
			}
			h.recordSuccess(kind, requestPath)
			return h.checkedPage(kind, id, requestPath, hexdataPage{Data: data, FetchedAt: h.now(), Cache: championCacheStateMiss}, nil)
		}
		if refreshBuild {
			refreshKey := key + "|refresh"
			result, err := h.provider.cache.loadWithStatus(ctx, refreshKey, 0, 0, false, func(loadCtx context.Context) ([]byte, error) {
				return fetchChecked(loadCtx, stale.Data, probe)
			})
			if err == nil {
				return h.checkedPage(kind, id, requestPath, hexdataPage{Data: result.data, FetchedAt: result.fetchedAt, Cache: result.state}, nil)
			}
			if len(stale.Data) > 0 {
				return h.checkedPage(kind, id, requestPath, hexdataPage{Data: stale.Data, FetchedAt: stale.FetchedAt, Cache: championCacheStateStale}, nil)
			}
			if state.PreviousBuildID != "" {
				previousKey := strings.Join([]string{state.PreviousBuildID, kind, id}, "|")
				if previous, previousErr := h.provider.cache.readDisk(previousKey); previousErr == nil && len(previous.Data) > 0 {
					return h.checkedPage(kind, id, requestPath, hexdataPage{Data: previous.Data, FetchedAt: previous.FetchedAt, Cache: championCacheStateStale}, nil)
				}
			}
			return hexdataPage{}, err
		}
		result, err := h.provider.cache.loadWithStatus(ctx, key, ttl, hexdataCacheTTL, true, func(loadCtx context.Context) ([]byte, error) {
			return fetchChecked(loadCtx, stale.Data, probe)
		})
		if err != nil {
			if state.PreviousBuildID != "" {
				previousKey := strings.Join([]string{state.PreviousBuildID, kind, id}, "|")
				if previous, previousErr := h.provider.cache.readDisk(previousKey); previousErr == nil && len(previous.Data) > 0 {
					return h.checkedPage(kind, id, requestPath, hexdataPage{Data: previous.Data, FetchedAt: previous.FetchedAt, Cache: championCacheStateStale}, nil)
				}
			}
			return hexdataPage{}, err
		}
		return h.checkedPage(kind, id, requestPath, hexdataPage{Data: result.data, FetchedAt: result.fetchedAt, Cache: result.state}, nil)
	}
	data, err := fetchChecked(ctx, nil, probe)
	page, checkedErr := h.checkedPage(kind, id, requestPath, hexdataPage{Data: data, FetchedAt: h.now(), Cache: championCacheStateMiss}, err)
	if probe && checkedErr == nil {
		h.recordSuccess(kind, requestPath)
		if h.provider != nil && h.provider.diag != nil {
			h.provider.diag(map[string]any{"event": "hexdata_circuit_probe", "outcome": "success", "kind": kind})
		}
	}
	return page, checkedErr
}

func (h *hexdataClient) promote(kind, id, buildID string, data []byte, fetchedAt time.Time) {
	if h.provider.cache == nil || strings.TrimSpace(buildID) == "" || len(data) == 0 {
		return
	}
	key := strings.Join([]string{buildID, kind, id}, "|")
	hash := sha256Bytes(data)
	ttl := hexdataCacheTTL
	if kind == "answer" || kind == "meta" {
		ttl = hexdataBuildSoftTTL
	}
	entry := championCacheEnvelope{Schema: championCacheSchema, Key: key, FetchedAt: fetchedAt, ExpiresAt: fetchedAt.Add(ttl), StaleUntil: fetchedAt.Add(ttl + hexdataCacheTTL), Hash: hash, Data: append([]byte(nil), data...)}
	h.provider.cache.mu.Lock()
	h.provider.cache.storeMemoryLocked(key, entry)
	h.provider.cache.mu.Unlock()
	_ = h.provider.cache.writeDisk(entry)
}

func sha256Bytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (h *hexdataClient) fetch(ctx context.Context, kind, requestPath, accept string, stale []byte, probe bool) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			if status := championUpstreamHTTPStatus(lastErr); status >= 400 && status < 500 {
				break
			}
			if err := h.sleep(ctx, h.retryDelay); err != nil {
				return nil, err
			}
		}
		data, status, err := h.fetchOnce(ctx, requestPath, accept, stale)
		if err == nil {
			return data, nil
		}
		lastErr = err
		if status == http.StatusNotModified && len(stale) == 0 {
			// A validator without a reusable body is not a cache hit. The
			// fetchOnce path clears it; retry once without conditions.
			stale = nil
			continue
		}
		if status == http.StatusForbidden || status == http.StatusTooManyRequests {
			break
		}
	}
	return nil, lastErr
}

func (h *hexdataClient) fetchOnce(ctx context.Context, requestPath, accept string, stale []byte) ([]byte, int, error) {
	select {
	case hexdataGlobalGate <- struct{}{}:
		defer func() { <-hexdataGlobalGate }()
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	}
	hexdataGlobalPaceMu.Lock()
	now := h.now()
	lastRequest := hexdataGlobalLastRequest
	if lastRequest.After(now) {
		// A clock adjustment must not turn a 300 ms pacing rule into a long stall.
		lastRequest = time.Time{}
	}
	wait := time.Duration(0)
	if !lastRequest.IsZero() {
		wait = h.minInterval - now.Sub(lastRequest)
		if wait < 0 {
			wait = 0
		}
	}
	if h.maximumJitter > 0 && h.jitter != nil {
		wait += h.jitter(h.maximumJitter)
	}
	if wait > 0 {
		if err := h.sleep(ctx, wait); err != nil {
			hexdataGlobalPaceMu.Unlock()
			return nil, 0, err
		}
	}
	hexdataGlobalLastRequest = h.now()
	hexdataGlobalPaceMu.Unlock()

	requestContext, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	u := url.URL{Scheme: "https", Host: hexdataHost, Path: requestPath}
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set("Accept", accept)
	request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	// R116-A：上游对 /api/hexdata/* 校验 Referer。实测（2026-09-20）不带
	// Referer 请求 /api/hexdata/heroes/157 返回 403 data_not_public，而 fetch
	// 对 4xx 立即 break 不重试并计失败，三次就把新 kind 熔断——第一次上线就自爆。
	request.Header.Set("Referer", "https://"+hexdataHost+"/")
	request.Header.Set("User-Agent", "DeepLegends/"+version+" (+https://github.com/LLYY0418/Deep-Legends; LoL 本地助手; 用户触发查询)")
	h.mu.Lock()
	if len(stale) > 0 {
		if validator := h.state.ETags[requestPath]; validator != "" {
			request.Header.Set("If-None-Match", validator)
		}
		if validator := h.state.LastModified[requestPath]; validator != "" {
			request.Header.Set("If-Modified-Since", validator)
		}
	}
	h.mu.Unlock()
	h.provider.clientMu.RLock()
	baseClient := h.provider.client
	h.provider.clientMu.RUnlock()
	client := *baseClient
	client.Timeout = 8 * time.Second
	response, err := client.Do(request)
	if err != nil {
		return nil, 0, fmt.Errorf("hexdata unavailable: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotModified && len(stale) > 0 {
		return append([]byte(nil), stale...), response.StatusCode, nil
	}
	if response.StatusCode == http.StatusNotModified {
		h.mu.Lock()
		delete(h.state.ETags, requestPath)
		delete(h.state.LastModified, requestPath)
		h.saveStateLocked()
		h.mu.Unlock()
		return nil, response.StatusCode, errors.New("hexdata validator has no cached body")
	}
	if response.StatusCode != http.StatusOK {
		return nil, response.StatusCode, fmt.Errorf("champion provider returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > championHTMLMax {
		return nil, response.StatusCode, errors.New("hexdata response is too large")
	}
	data, err := readLimited(response.Body, championHTMLMax)
	if err != nil || len(data) == 0 {
		if err == nil {
			err = errHexdataEmptyPayload
		}
		return nil, response.StatusCode, err
	}
	h.mu.Lock()
	if value := response.Header.Get("ETag"); value != "" {
		if h.state.ETags == nil {
			h.state.ETags = make(map[string]string)
		}
		h.state.ETags[requestPath] = value
	}
	if value := response.Header.Get("Last-Modified"); value != "" {
		if h.state.LastModified == nil {
			h.state.LastModified = make(map[string]string)
		}
		h.state.LastModified[requestPath] = value
	}
	h.saveStateLocked()
	h.mu.Unlock()
	return data, response.StatusCode, nil
}

func (h *hexdataClient) recordSuccess(kind, requestPath string) {
	h.mu.Lock()
	delete(h.state.Circuits, kind)
	if requestPath == hexdataAnswerPath || requestPath == hexdataMetaPath {
		h.state.BuildChecked = h.now()
	}
	h.saveStateLocked()
	h.mu.Unlock()
}

func (h *hexdataClient) recordFailure(kind string, status int, immediate bool) {
	_ = immediate
	reason := "upstream unavailable"
	if status > 0 {
		reason = fmt.Sprintf("upstream HTTP %d", status)
	}
	h.recordCircuitFailure(kind, reason, status, hexdataPayloadShape{}, false, false)
}

func (h *hexdataClient) recordShapeFailure(kind string, rows, fields int, buildID string) {
	// Zero parsed rows can still mean a non-empty page whose markup changed.
	// Only errHexdataEmptyPayload is classified as a genuinely empty response.
	h.recordCircuitFailure(kind, "shape validation failed", 0, hexdataPayloadShape{Rows: rows, Fields: fields, BuildID: buildID}, true, false)
}

func (h *hexdataClient) recordLoadFailure(kind string, err error) {
	if err == nil || isCancellation(err) {
		return
	}
	var shapeErr *hexdataShapeValidationError
	if errors.As(err, &shapeErr) {
		empty := errors.Is(err, errHexdataEmptyPayload)
		h.recordCircuitFailure(kind, "shape validation failed", 0, shapeErr.shape, true, empty)
		return
	}
	if errors.Is(err, errHexdataEmptyPayload) {
		h.recordCircuitFailure(kind, "empty payload", 0, hexdataPayloadShape{}, true, true)
		return
	}
	h.recordFailure(kind, championUpstreamHTTPStatus(err), false)
}

func (h *hexdataClient) recordCircuitFailure(kind, reason string, status int, shape hexdataPayloadShape, shapeFailure, empty bool) {
	h.mu.Lock()
	circuit := h.state.Circuits[kind]
	now := h.now()
	if circuit.LastFailure.IsZero() || now.Sub(circuit.LastFailure) >= hexdataCircuitFailureDecay || now.Before(circuit.LastFailure) {
		circuit = hexdataCircuitState{}
	}
	circuit.Failures = min(circuit.Failures+1, hexdataCircuitFailureMax)
	circuit.LastFailure = now
	failures := circuit.Failures
	trip := failures >= hexdataCircuitFailureLimit
	if trip {
		duration := hexdataCircuitBackoff(failures, empty)
		circuit.Until = now.Add(duration)
		probeDelay := min(duration, hexdataCircuitProbeInterval)
		circuit.ProbeAt = now.Add(probeDelay)
	}
	h.state.Circuits[kind] = circuit
	h.saveStateLocked()
	h.mu.Unlock()
	if h.provider != nil && h.provider.diag != nil {
		if shapeFailure {
			h.provider.diag(map[string]any{"event": "hexdata_shape_invalid", "kind": kind, "rows": shape.Rows, "fields": shape.Fields, "buildId": shape.BuildID, "failure_kind": map[bool]string{true: "empty", false: "parse"}[empty]})
		}
		if trip {
			event := map[string]any{"event": "hexdata_circuit_trip", "reason": reason, "kind": kind, "failures": failures, "until": circuit.Until, "probeAt": circuit.ProbeAt}
			if status > 0 {
				event["status"] = status
			}
			if shapeFailure {
				event["id"], event["rows"], event["fields"] = shape.BuildID, shape.Rows, shape.Fields
			}
			h.provider.diag(event)
		}
	}
}

func hexdataCircuitBackoff(failures int, empty bool) time.Duration {
	durations := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, hexdataCircuitDuration}
	if empty {
		durations = []time.Duration{30 * time.Second, time.Minute, 5 * time.Minute, 15 * time.Minute}
	}
	index := max(0, failures-hexdataCircuitFailureLimit)
	return durations[min(index, len(durations)-1)]
}

func (h *hexdataClient) adoptAnswer(answer hexdataAnswerCards) {
	h.mu.Lock()
	if h.state.BuildID != "" && h.state.BuildID != answer.BuildID {
		h.state.PreviousBuildID = h.state.BuildID
	}
	h.state.BuildID = answer.BuildID
	h.state.ReportPatch = answer.ReportPatch
	h.state.ReportDate = answer.ReportDate
	h.state.BuildChecked = h.now()
	h.saveStateLocked()
	h.mu.Unlock()
}

func (h *hexdataClient) adoptCitation(citation championSourceCitation) bool {
	if !hexdataCitationComplete(citation) {
		return false
	}
	h.mu.Lock()
	changed := h.state.BuildID != citation.BuildID
	if h.state.BuildID != "" && changed {
		h.state.PreviousBuildID = h.state.BuildID
	}
	h.state.BuildID = citation.BuildID
	h.state.ReportPatch = citation.Patch
	h.state.ReportDate = citation.ReportDate
	h.state.BuildChecked = h.now()
	h.saveStateLocked()
	h.mu.Unlock()
	if changed {
		h.pruneStaleBuildsAsync(citation.BuildID)
	}
	return true
}

// adoptMeta 把 /api/hexdata/meta 的快照写进客户端状态。JSON API 的响应体没有
// citation，buildId/patch/reportDate 只能从这里来；cacheKey 也依赖 state.BuildID，
// 不 adopt 就会退回 "bootstrap|" 前缀，磁盘缓存永远命不中。
func (h *hexdataClient) adoptMeta(snapshot hexdataMetaSnapshot) bool {
	if !strings.HasPrefix(snapshot.BuildID, "hexdata-") || snapshot.ReportPatch == "" || snapshot.ReportDate == "" {
		return false
	}
	h.mu.Lock()
	changed := h.state.BuildID != snapshot.BuildID
	if h.state.BuildID != "" && changed {
		h.state.PreviousBuildID = h.state.BuildID
	}
	h.state.BuildID = snapshot.BuildID
	h.state.ReportPatch = snapshot.ReportPatch
	h.state.ReportDate = snapshot.ReportDate
	h.state.BuildChecked = h.now()
	h.saveStateLocked()
	h.mu.Unlock()
	if changed {
		h.pruneStaleBuildsAsync(snapshot.BuildID)
	}
	return true
}

// pruneStaleBuildsAsync 是 R116-A P3 的触发点：确认拿到新 buildID 之后异步回收
// 上一个 buildID 的 hexdata- 落盘文件。跑在 goroutine 里，不阻塞请求主路径。
func (h *hexdataClient) pruneStaleBuildsAsync(buildID string) {
	if h == nil || h.provider == nil || h.provider.cache == nil || !strings.HasPrefix(buildID, "hexdata-") {
		return
	}
	h.pruneMu.Lock()
	if h.prunedBuildID == buildID {
		h.pruneMu.Unlock()
		return
	}
	h.prunedBuildID = buildID
	h.pruneMu.Unlock()
	h.pruneWait.Add(1)
	go func() {
		defer h.pruneWait.Done()
		removed, err := h.provider.cache.pruneStaleHexdataBuildsCount(buildID)
		if h.provider.diag == nil {
			return
		}
		event := map[string]any{"event": "hexdata_build_prune", "buildId": buildID, "removed": removed}
		if err != nil {
			event["error"] = err.Error()
		}
		h.provider.diag(event)
	}()
}

func hexdataCitationComplete(citation championSourceCitation) bool {
	return strings.HasPrefix(citation.BuildID, "hexdata-") && citation.Patch != "" && citation.ReportDate != "" && strings.HasPrefix(citation.CanonicalURL, "https://"+hexdataHost+"/")
}

func (p *championProvider) loadHexdataAnswer(ctx context.Context) (hexdataAnswerCards, hexdataPage, error) {
	page, err := p.hexdata.load(ctx, "answer", "site", hexdataAnswerPath, "application/json", false)
	if err != nil {
		return hexdataAnswerCards{}, page, err
	}
	var answer hexdataAnswerCards
	err = json.Unmarshal(page.Data, &answer)
	valid := err == nil && validHexdataAnswer(answer)
	if !valid {
		p.hexdata.recordShapeFailure("answer", len(answer.HeroAliases), 0, answer.BuildID)
		return hexdataAnswerCards{}, page, errors.New("hexdata answer cards changed")
	}
	p.hexdata.adoptAnswer(answer)
	if page.Cache != championCacheStateStale {
		p.hexdata.recordSuccess("answer", hexdataAnswerPath)
	}
	p.hexdata.promote("answer", "site", answer.BuildID, page.Data, page.FetchedAt)
	p.reportHexdataShape("answer", len(answer.HeroAliases), len(answer.HeroAliases), answer.BuildID)
	return answer, page, nil
}

func validHexdataAnswer(answer hexdataAnswerCards) bool {
	return strings.HasPrefix(answer.BuildID, "hexdata-") && answer.ReportPatch != "" && answer.ReportDate != "" && len(answer.HeroAliases) >= 150 && answer.Methodology.RecommendationPolicy.Ranking == "sample_tier_then_wilson_lower_bound_v1" && answer.Methodology.RecommendationPolicy.WilsonZ > 0 && answer.Methodology.SamplePolicy.Medium.Min > 0 && answer.Methodology.SamplePolicy.High.Min > 0
}

func (p *championProvider) loadHexdataMeasurementTechnique(ctx context.Context) string {
	answer, _, err := p.loadHexdataAnswer(ctx)
	if err != nil {
		return ""
	}
	return hexdataMeasurementTechnique(answer)
}

func (p *championProvider) reportHexdataShape(kind string, rows, fields int, buildID string) {
	if p.diag != nil {
		p.diag(map[string]any{"event": "hexdata_shape", "kind": kind, "rows": rows, "fields": fields, "buildId": buildID})
	}
}

func (p *championProvider) reportHexdataFallback(module string, err error) {
	if p.diag == nil || err == nil {
		return
	}
	p.diag(map[string]any{"event": "hexdata_fallback", "module": module, "reason": err.Error()})
}

func (p *championProvider) reportHexdataHeroShape(detail hexdataHeroDetailV2, citation championSourceCitation) {
	p.reportHexdataShape("hero-json", len(detail.Augments), len(detail.Items), citation.BuildID)
}

// reportHexdataHeroJSONStats 把解析层的丢弃/裁剪计数写成一条诊断事件：上游改
// 字段名时，这里会先于用户看到的空白卡片暴露出来（日志完备红线）。
func (p *championProvider) reportHexdataHeroJSONStats(championID int, stats hexdataHeroJSONStats) {
	if p.diag == nil {
		return
	}
	event := map[string]any{"event": "hexdata_hero_json", "championId": championID, "dropped": stats.dropped()}
	stats.addToDiagnostic(event)
	p.diag(event)
}

func parseHexdataDocument(data []byte) (*xhtml.Node, string, error) {
	document, err := xhtml.Parse(strings.NewReader(string(data)))
	if err != nil {
		return nil, "", err
	}
	buildID := ""
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if buildID == "" && node.Type == xhtml.ElementNode {
			if node.Data == "meta" && attribute(node, "name") == "hexdata-build-id" {
				buildID = attribute(node, "content")
			}
			if value := attribute(node, "data-hexdata-build-id"); value != "" {
				buildID = value
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(document)
	if !strings.HasPrefix(buildID, "hexdata-") {
		return nil, "", errors.New("hexdata build id missing")
	}
	return document, buildID, nil
}

func hexdataNodeText(node *xhtml.Node) string {
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

func hexdataRawText(node *xhtml.Node) string {
	var builder strings.Builder
	var walk func(*xhtml.Node)
	walk = func(current *xhtml.Node) {
		if current.Type == xhtml.TextNode {
			builder.WriteString(current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return strings.TrimSpace(builder.String())
}

func primaryTables(document *xhtml.Node) []*xhtml.Node {
	var primary *xhtml.Node
	var find func(*xhtml.Node)
	find = func(node *xhtml.Node) {
		if primary == nil && node.Type == xhtml.ElementNode && hasAttribute(node, "data-primary-content") {
			primary = node
			return
		}
		for child := node.FirstChild; child != nil && primary == nil; child = child.NextSibling {
			find(child)
		}
	}
	find(document)
	if primary == nil {
		return nil
	}
	return descendantElements(primary, "table")
}

func hasAttribute(node *xhtml.Node, key string) bool {
	for _, item := range node.Attr {
		if item.Key == key {
			return true
		}
	}
	return false
}

func tableRows(table *xhtml.Node) [][]*xhtml.Node {
	result := make([][]*xhtml.Node, 0)
	for _, row := range descendantElements(table, "tr") {
		cells := descendantElements(row, "td")
		if len(cells) > 0 {
			result = append(result, cells)
		}
	}
	return result
}

func hexdataTableFieldCount(data []byte) int {
	document, err := xhtml.Parse(strings.NewReader(string(data)))
	if err != nil {
		return 0
	}
	fields := 0
	for _, table := range primaryTables(document) {
		for _, cells := range tableRows(table) {
			fields += len(cells)
		}
	}
	return fields
}

func hexdataTableShapeDiagnostic(kind string, data []byte) map[string]any {
	document, err := xhtml.Parse(strings.NewReader(string(data)))
	if err != nil {
		return map[string]any{"event": "hexdata_table_shape", "kind": kind, "parse_error": true}
	}
	tables := primaryTables(document)
	rowCellCounts := make(map[string]int)
	columns := make([]map[string]any, 0)
	for _, table := range tables {
		for _, cells := range tableRows(table) {
			rowCellCounts[strconv.Itoa(len(cells))]++
			for index, cell := range cells {
				for len(columns) <= index {
					columns = append(columns, map[string]any{
						"index": len(columns), "link_present_counts": map[string]int{},
						"link_path_head_counts": map[string]int{}, "augment_path_match_counts": map[string]int{},
						"text_class_counts": map[string]int{},
					})
				}
				linkCounts := columns[index]["link_present_counts"].(map[string]int)
				pathHeads := columns[index]["link_path_head_counts"].(map[string]int)
				pathMatches := columns[index]["augment_path_match_counts"].(map[string]int)
				textClasses := columns[index]["text_class_counts"].(map[string]int)
				href, _ := firstLink(cell)
				if href == "" {
					linkCounts["absent"]++
					pathHeads["none"]++
					pathMatches["unmatched"]++
				} else {
					linkCounts["present"]++
					path := hexdataLinkPath(href)
					pathHeads[hexdataSafePathHead(path)]++
					if hexdataAugmentPathPattern.MatchString(path) {
						pathMatches["matched"]++
					} else {
						pathMatches["unmatched"]++
					}
				}
				textClasses[hexdataTextClass(hexdataNodeText(cell))]++
			}
		}
	}
	return map[string]any{
		"event": "hexdata_table_shape", "kind": kind, "table_count": len(tables),
		"row_cell_count_counts": rowCellCounts, "columns": columns,
	}
}

func hexdataSafePathHead(path string) string {
	head := strings.ToLower(strings.SplitN(strings.Trim(hexdataLinkPath(path), "/"), "/", 2)[0])
	if head == "" {
		return "none"
	}
	for _, char := range head {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' && char != '_' {
			return "other"
		}
	}
	return "/" + head
}

func hexdataTextClass(value string) string {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	classes := make([]string, 0, 3)
	if strings.Contains(lower, "globalhexscore") {
		classes = append(classes, "has_globalhexscore")
	}
	if strings.Contains(value, "胜率") {
		classes = append(classes, "has_胜率")
	}
	if strings.Contains(value, "%") {
		classes = append(classes, "has_percent_sign")
	}
	if len(classes) > 0 {
		return strings.Join(classes, "+")
	}
	if _, err := strconv.ParseFloat(strings.ReplaceAll(value, ",", ""), 64); err == nil && value != "" {
		return "pure_number"
	}
	return "other"
}

func firstLink(node *xhtml.Node) (string, string) {
	links := descendantElements(node, "a")
	if len(links) == 0 {
		return "", ""
	}
	return attribute(links[0], "href"), hexdataNodeText(links[0])
}

func parsePercentText(value string) float64 {
	parsed, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(value), "%"), 64)
	return parsed
}

func parseCountText(value string) int {
	parsed, _ := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(value), ",", ""))
	return parsed
}

func parseFloatText(value string) float64 {
	parsed, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return parsed
}

func hexdataCitation(document *xhtml.Node, buildID, canonical string) championSourceCitation {
	text := hexdataNodeText(document)
	patch, reportDate := "", ""
	if match := hexdataPatchPattern.FindStringSubmatch(text); len(match) == 2 {
		patch = match[1]
	}
	if match := hexdataDatePattern.FindStringSubmatch(text); len(match) == 2 {
		if _, err := time.Parse("2006-01-02", match[1]); err == nil {
			reportDate = match[1]
		}
	}
	// Current pages publish the date on their canonical Dataset instead of a
	// visible “数据日期” label. Do not borrow dates from breadcrumbs/other datasets.
	if reportDate == "" {
		for _, script := range descendantElements(document, "script") {
			if attribute(script, "type") != "application/ld+json" {
				continue
			}
			var dataset struct {
				Type         string `json:"@type"`
				URL          string `json:"url"`
				DateModified string `json:"dateModified"`
			}
			if json.Unmarshal([]byte(hexdataRawText(script)), &dataset) != nil || dataset.Type != "Dataset" || dataset.URL != "https://"+hexdataHost+canonical {
				continue
			}
			if _, err := time.Parse("2006-01-02", dataset.DateModified); err == nil {
				reportDate = dataset.DateModified
				break
			}
		}
	}
	return championSourceCitation{Source: "Hexdata", Patch: patch, ReportDate: reportDate, BuildID: buildID, CanonicalURL: "https://" + hexdataHost + canonical}
}

func parseHexdataHeroes(data []byte, answer hexdataAnswerCards) ([]hexdataHeroRow, championSourceCitation, error) {
	document, buildID, err := parseHexdataDocument(data)
	if err != nil {
		return nil, championSourceCitation{}, err
	}
	tables := primaryTables(document)
	if len(tables) != 1 {
		return nil, championSourceCitation{}, errors.New("hexdata heroes table changed")
	}
	rows := make([]hexdataHeroRow, 0, 180)
	for _, cells := range tableRows(tables[0]) {
		if len(cells) < 2 {
			continue
		}
		href, name := firstLink(cells[0])
		name = strings.TrimSpace(hexdataChampionNickname.ReplaceAllString(name, ""))
		path := hexdataHeroPathPattern.FindStringSubmatch(hexdataLinkPath(href))
		metrics := hexdataHeroMetricPattern.FindStringSubmatch(hexdataNodeText(cells[1]))
		if len(path) != 3 || len(metrics) != 3 {
			continue
		}
		id, _ := strconv.Atoi(path[1])
		winRate := parseFloatText(metrics[1])
		games := parseCountText(metrics[2])
		if id <= 0 || name == "" || winRate <= 0 || games <= 0 {
			continue
		}
		rows = append(rows, hexdataHeroRow{ID: id, Slug: path[2], Name: name, WinRate: winRate, Games: games})
	}
	if len(rows) < 150 || len(answer.HeroAliases) >= 150 && len(rows) != len(answer.HeroAliases) {
		return rows, championSourceCitation{}, errors.New("hexdata heroes shape is incomplete")
	}
	applyHexdataLocalTiers(rows, answer.Methodology)
	citation := hexdataCitation(document, buildID, "/heroes")
	return rows, citation, nil
}

func applyHexdataLocalTiers(rows []hexdataHeroRow, methodology hexdataMethodology) {
	z := methodology.RecommendationPolicy.WilsonZ
	if z <= 0 {
		return
	}
	highMin := methodology.SamplePolicy.High.Min
	mediumMin := methodology.SamplePolicy.Medium.Min
	if highMin <= 0 || mediumMin <= 0 {
		return
	}
	for index := range rows {
		row := &rows[index]
		switch {
		case row.Games >= highMin:
			row.TierBand = 2
		case row.Games >= mediumMin:
			row.TierBand = 1
		default:
			row.TierBand = 0
		}
		p := row.WinRate / 100
		n := float64(row.Games)
		row.Wilson = (p + z*z/(2*n) - z*math.Sqrt((p*(1-p)+z*z/(4*n))/n)) / (1 + z*z/n)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].TierBand != rows[j].TierBand {
			return rows[i].TierBand > rows[j].TierBand
		}
		if rows[i].Wilson != rows[j].Wilson {
			return rows[i].Wilson > rows[j].Wilson
		}
		return rows[i].ID < rows[j].ID
	})
}

func (p *championProvider) loadHexdataRankings(ctx context.Context) (championRankingResponse, error) {
	answer, _, answerErr := p.loadHexdataAnswer(ctx)
	if answerErr != nil {
		p.reportHexdataFallback("rankings", answerErr)
		return championRankingResponse{}, answerErr
	}
	heroesPage, heroesErr := p.hexdata.load(ctx, "heroes", "all", "/heroes", "text/html,application/xhtml+xml", false)
	if heroesErr != nil {
		p.reportHexdataFallback("rankings", heroesErr)
		return championRankingResponse{}, heroesErr
	}
	rows, citation, err := parseHexdataHeroes(heroesPage.Data, answer)
	if err != nil {
		p.hexdata.recordShapeFailure("heroes", len(rows), hexdataTableFieldCount(heroesPage.Data), citation.BuildID)
		return championRankingResponse{}, err
	}
	if (citation.BuildID != answer.BuildID || citation.Patch != answer.ReportPatch || citation.ReportDate != answer.ReportDate) && heroesPage.Cache != championCacheStateStale {
		p.hexdata.recordShapeFailure("heroes", len(rows), hexdataTableFieldCount(heroesPage.Data), citation.BuildID)
		return championRankingResponse{}, errors.New("hexdata heroes build metadata mismatch")
	}
	p.hexdata.promote("heroes", "all", citation.BuildID, heroesPage.Data, heroesPage.FetchedAt)
	if heroesPage.Cache != championCacheStateStale {
		p.hexdata.recordSuccess("heroes", "/heroes")
	}
	// R116-B P0-5-2：官方英雄档位优先。meta 快照这一趟同时给出 tierBands
	// （官方 T1-T5 的中文标签）与 samplePolicy，所以只加载一次；insights 复用
	// 同一份快照，冷启动最多多 2 条 hexdata 请求（meta + hextech-insights），
	// 两者都按 buildID 长期缓存，同一版本内只回源一次。
	snapshot, snapshotErr := p.loadHexdataMeta(ctx)
	if snapshotErr != nil {
		// meta 不可用不是榜单的失败条件：citation 已经从 answer-cards 拼好了。
		p.reportHexdataFallback("hero-tiers", snapshotErr)
	}
	officialTiers := p.hexdataOfficialHeroTiers(ctx, snapshot, snapshotErr)
	result := make([]championRankingRow, 0, len(rows))
	localTierRows := 0
	for index, item := range rows {
		row := championRankingRow{ChampionID: item.ID, Key: item.Slug, Name: item.Name, Rank: index + 1, Play: item.Games, WinRate: item.WinRate}
		if tier, ok := officialTiers[item.ID]; ok {
			row.Tier, row.TierLocallyCalculated = tier, false
		} else {
			// 官方档位缺失（insights 熔断/降级，或该英雄不在 insights 里）：
			// 沿用旧的「按返回顺序本地分档」，并如实标记为本地估算，前端据此
			// 显示「本地估算」小字。绝不把本地值伪装成官方值。
			row.Tier, row.TierLocallyCalculated = index*5/len(rows)+1, true
			localTierRows++
		}
		result = append(result, row)
	}
	p.reportHexdataShape("heroes", len(result), len(result)*5, citation.BuildID)
	if p.diag != nil {
		p.diag(map[string]any{
			"event": "hexdata_hero_tier_source", "rows": len(result),
			"officialTierRows": len(result) - localTierRows, "localTierRows": localTierRows,
			"insightsAvailable": officialTiers != nil, "metaAvailable": snapshotErr == nil,
			"tierBands": len(snapshot.TierBands),
		})
	}
	response := championRankingResponse{Mode: "hextech-aram", Region: "CN", Patch: citation.Patch, Source: "Hexdata", FetchedAt: heroesPage.FetchedAt, EntertainmentSample: true, Citation: &citation, MeasurementTechnique: hexdataMeasurementTechnique(answer), Rows: result, TierBands: snapshot.tierBands()}
	if localTierRows > 0 {
		response.MeasurementTechnique = appendMeasurementTechnique(response.MeasurementTechnique, "部分英雄的梯度档位为本地按排名顺序估算（官方档位数据不可用），已在界面标注「本地估算」")
	}
	return response, nil
}

func firstError(values ...error) error {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// R116-A：JSON API 数据源。四个端点（meta / heroes/{id} / postmatch /
// hextech-insights）都是 application/json，响应体不带 citation，buildId、patch、
// reportDate 一律取自 /api/hexdata/meta 的快照。
// ---------------------------------------------------------------------------

// hexdataParseID 把上游的字符串 ID 转成 int。空串、非数字、<=0 一律判非法，
// 调用方必须把整条记录丢弃：用 0 顶替会指向另一个真实存在的英雄/装备/海克斯，
// 属于伪造数据（项目红线：证据不足明确降级）。
func hexdataParseID(value string) (int, bool) {
	text := strings.TrimSpace(value)
	if text == "" {
		return 0, false
	}
	parsed, err := strconv.Atoi(text)
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return parsed, true
}

// hexdataParseIDs 转换 ID 数组（trios 的 augmentIds、装备三件套的 itemIds、
// 召唤师技能对的 spellIds）。任意一个非法就整条判废：半个组合会误导用户。
func hexdataParseIDs(values []string) ([]int, bool) {
	if len(values) == 0 {
		return nil, false
	}
	result := make([]int, 0, len(values))
	for _, value := range values {
		id, ok := hexdataParseID(value)
		if !ok {
			return nil, false
		}
		result = append(result, id)
	}
	return result, true
}

func (row hexdataItemRowRaw) convert() (hexdataItemRow, bool) {
	id, ok := hexdataParseID(row.RawItemID)
	if !ok {
		return hexdataItemRow{}, false
	}
	converted := row.hexdataItemRow
	converted.ItemID = id
	return converted, true
}

func (row hexdataAugmentRowRaw) convert() (hexdataAugmentRowV2, bool) {
	id, ok := hexdataParseID(row.RawAugmentID)
	if !ok {
		return hexdataAugmentRowV2{}, false
	}
	converted := row.hexdataAugmentRowV2
	converted.AugmentID = id
	return converted, true
}

func (row hexdataTrioRowRaw) convert() (hexdataTrioRow, bool) {
	converted := row.hexdataTrioRow
	// trios 用 augmentIds，terminalItemTrios 用 itemIds；两个都缺就是形状变了。
	if len(row.RawAugmentIDs) > 0 {
		ids, ok := hexdataParseIDs(row.RawAugmentIDs)
		if !ok {
			return hexdataTrioRow{}, false
		}
		converted.AugmentIDs = ids
	}
	if len(row.RawItemIDs) > 0 {
		ids, ok := hexdataParseIDs(row.RawItemIDs)
		if !ok {
			return hexdataTrioRow{}, false
		}
		converted.ItemIDs = ids
	}
	if len(converted.AugmentIDs) == 0 && len(converted.ItemIDs) == 0 {
		return hexdataTrioRow{}, false
	}
	return converted, true
}

func (row hexdataMatchupRowRaw) convert() (hexdataMatchupRow, bool) {
	id, ok := hexdataParseID(row.RawOpponentChampionID)
	if !ok {
		return hexdataMatchupRow{}, false
	}
	converted := row.hexdataMatchupRow
	converted.OpponentChampionID = id
	return converted, true
}

func (row hexdataSynergyRowRaw) convert() (hexdataSynergyRow, bool) {
	id, ok := hexdataParseID(row.RawTeammateChampionID)
	if !ok {
		return hexdataSynergyRow{}, false
	}
	converted := row.hexdataSynergyRow
	converted.TeammateChampionID = id
	return converted, true
}

func (row hexdataSpellPairRowRaw) convert() (hexdataSpellPairRow, bool) {
	ids, ok := hexdataParseIDs(row.RawSpellIDs)
	if !ok {
		return hexdataSpellPairRow{}, false
	}
	converted := row.hexdataSpellPairRow
	converted.SpellIDs = ids
	return converted, true
}

// hexdataTrimTrios 按 games 降序裁剪到前 hexdataTrioKeepLimit 条，返回裁掉的条数。
// 上游 trios / terminalItemTrios 各 1000 条是硬上限（meta.heroTrioLimitPerHero），
// 不是采样；terminalItemTrios 的最小 games 实测已有 1248，前 50 条足够覆盖所有会被
// 展示的组合，同时把单英雄解析后的内存占用从「各 1000 条」压到「各 50 条」。
// 排序键带 TrioKey 兜底，保证同一份 payload 每次裁出同一批。
func hexdataTrimTrios(rows []hexdataTrioRow) ([]hexdataTrioRow, int) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Games != rows[j].Games {
			return rows[i].Games > rows[j].Games
		}
		if rows[i].WinRate != rows[j].WinRate {
			return rows[i].WinRate > rows[j].WinRate
		}
		return rows[i].TrioKey < rows[j].TrioKey
	})
	if len(rows) <= hexdataTrioKeepLimit {
		return rows, 0
	}
	return rows[:hexdataTrioKeepLimit], len(rows) - hexdataTrioKeepLimit
}

// parseHexdataMeta 解析 /api/hexdata/meta。这是三个 JSON kind 唯一的 citation
// 来源，字段不完整就报错，绝不返回半成品快照让下游拼出假的 buildId。
func parseHexdataMeta(data []byte) (hexdataMetaSnapshot, error) {
	var snapshot hexdataMetaSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return hexdataMetaSnapshot{}, err
	}
	if !strings.HasPrefix(snapshot.BuildID, "hexdata-") {
		return snapshot, errors.New("hexdata meta buildId is missing")
	}
	if snapshot.ReportPatch == "" {
		return snapshot, errors.New("hexdata meta reportPatch is missing")
	}
	if _, err := time.Parse("2006-01-02", snapshot.ReportDate); err != nil {
		return snapshot, errors.New("hexdata meta reportDate is not an ISO date")
	}
	if snapshot.HeroCount < hexdataAggregateHeroMin || snapshot.HeroCount > hexdataAggregateHeroMax {
		return snapshot, fmt.Errorf("hexdata meta heroCount %d is outside %d..%d", snapshot.HeroCount, hexdataAggregateHeroMin, hexdataAggregateHeroMax)
	}
	if snapshot.AugmentCount < hexdataAggregateAugmentMin || snapshot.AugmentCount > hexdataAggregateAugmentMax {
		return snapshot, fmt.Errorf("hexdata meta augmentCount %d is outside %d..%d", snapshot.AugmentCount, hexdataAggregateAugmentMin, hexdataAggregateAugmentMax)
	}
	return snapshot, nil
}

// hexdataJSONCitation 用 meta 快照拼 citation。canonical 必须是站内给人看的
// HTML 页面路径（/hero/{id}-{slug}、/heroes），不许指向 JSON 端点。
func hexdataJSONCitation(snapshot hexdataMetaSnapshot, canonical string) championSourceCitation {
	return championSourceCitation{
		Source:       "Hexdata",
		Patch:        snapshot.ReportPatch,
		ReportDate:   snapshot.ReportDate,
		BuildID:      snapshot.BuildID,
		CanonicalURL: "https://" + hexdataHost + canonical,
	}
}

// loadHexdataMeta 取站点元数据快照：kind="meta"、12h 软 TTL，缓存策略与 answer 一致。
func (p *championProvider) loadHexdataMeta(ctx context.Context) (hexdataMetaSnapshot, error) {
	page, err := p.hexdata.load(ctx, "meta", "site", hexdataMetaPath, "application/json", false)
	if err != nil {
		p.reportHexdataFallback("meta", err)
		return hexdataMetaSnapshot{}, err
	}
	snapshot, parseErr := parseHexdataMeta(page.Data)
	if parseErr != nil {
		p.hexdata.recordShapeFailure("meta", snapshot.HeroCount, snapshot.AugmentCount, snapshot.BuildID)
		return hexdataMetaSnapshot{}, parseErr
	}
	if !p.hexdata.adoptMeta(snapshot) {
		p.hexdata.recordShapeFailure("meta", snapshot.HeroCount, snapshot.AugmentCount, snapshot.BuildID)
		return hexdataMetaSnapshot{}, errors.New("hexdata meta snapshot was rejected")
	}
	// P0-5：成功路径必须 promote，否则 refreshBuild 那条 persistDisk=false 的
	// 通道永远不落盘，每 12 小时第一次访问都回源。
	p.hexdata.promote("meta", "site", snapshot.BuildID, page.Data, page.FetchedAt)
	if page.Cache != championCacheStateStale {
		p.hexdata.recordSuccess("meta", hexdataMetaPath)
	}
	p.reportHexdataShape("meta", snapshot.HeroCount, snapshot.AugmentCount, snapshot.BuildID)
	return snapshot, nil
}

// parseHexdataHeroJSON 解析 /api/hexdata/heroes/{id}（R116-A P1-1）。
func parseHexdataHeroJSON(data []byte) (hexdataHeroDetailV2, error) {
	detail, _, err := parseHexdataHeroJSONWithStats(data)
	return detail, err
}

// parseHexdataHeroJSONWithStats 与 parseHexdataHeroJSON 相同，另外返回解析层的
// 丢弃/裁剪计数。ID 转换失败的记录整条丢弃并计数，绝不用 0 顶替。
func parseHexdataHeroJSONWithStats(data []byte) (hexdataHeroDetailV2, hexdataHeroJSONStats, error) {
	var payload hexdataHeroJSONPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return hexdataHeroDetailV2{}, hexdataHeroJSONStats{}, err
	}
	detail, stats := hexdataHeroDetailV2{}, hexdataHeroJSONStats{}
	heroWinRate := func(value float64) {
		if detail.HeroWinRate == 0 && value > 0 {
			detail.HeroWinRate = value * 100
		}
	}
	for _, row := range payload.Items {
		converted, ok := row.convert()
		if !ok {
			stats.DroppedItems++
			continue
		}
		detail.Items = append(detail.Items, converted)
	}
	for _, row := range payload.Augments {
		converted, ok := row.convert()
		if !ok {
			stats.DroppedAugments++
			continue
		}
		detail.Augments = append(detail.Augments, converted)
		heroWinRate(converted.HeroWinRate)
	}
	for _, row := range payload.Trios {
		converted, ok := row.convert()
		if !ok {
			stats.DroppedTrios++
			continue
		}
		detail.Trios = append(detail.Trios, converted)
		heroWinRate(converted.HeroWinRate)
	}
	for _, row := range payload.TerminalItemTrios {
		converted, ok := row.convert()
		if !ok {
			stats.DroppedTerminalItemTrios++
			continue
		}
		detail.TerminalItemTrios = append(detail.TerminalItemTrios, converted)
	}
	for _, row := range payload.WeakAgainst {
		converted, ok := row.convert()
		if !ok {
			stats.DroppedMatchups++
			continue
		}
		detail.WeakAgainst = append(detail.WeakAgainst, converted)
	}
	for _, row := range payload.StrongAgainst {
		converted, ok := row.convert()
		if !ok {
			stats.DroppedMatchups++
			continue
		}
		detail.StrongAgainst = append(detail.StrongAgainst, converted)
	}
	for _, row := range payload.TeammateSynergies {
		converted, ok := row.convert()
		if !ok {
			stats.DroppedSynergies++
			continue
		}
		detail.TeammateSynergies = append(detail.TeammateSynergies, converted)
	}
	for _, row := range payload.SummonerSpellPairs {
		converted, ok := row.convert()
		if !ok {
			stats.DroppedSpellPairs++
			continue
		}
		detail.SummonerSpellPairs = append(detail.SummonerSpellPairs, converted)
	}
	detail.Trios, stats.TrimmedTrios = hexdataTrimTrios(detail.Trios)
	detail.TerminalItemTrios, stats.TrimmedTerminalItemTrios = hexdataTrimTrios(detail.TerminalItemTrios)
	return detail, stats, nil
}

// hexdataRarityLabel 把上游的中文稀有度（实测只有 棱彩/黄金/白银 三个取值）映射
// 成站内一直在用的英文 token（前端按它上色分组）。未知取值降级为空，交给
// CommunityDragon 目录补，不猜。
func hexdataRarityLabel(value string) string {
	switch strings.TrimSpace(value) {
	case "棱彩":
		return "prismatic"
	case "黄金":
		return "gold"
	case "白银":
		return "silver"
	default:
		return ""
	}
}

// hexdataAugmentMetricRows 把官方字段搬到响应行：Score 用上游 hexScore，胜率用
// pairWinRate（换算成百分数，与站内既有单位一致），档位/收益率/威尔逊下界原样
// 保留官方口径。低样本行沿用旧的 hexdataMinimumSample（250 场）门槛过滤。
func hexdataAugmentMetricRows(rows []hexdataAugmentRowV2) []championMetricRow {
	result := make([]championMetricRow, 0, len(rows))
	for _, row := range rows {
		if row.Games < hexdataMinimumSample {
			continue
		}
		result = append(result, championMetricRow{
			Assets:             []championAsset{{ID: row.AugmentID, Kind: "augment", Name: row.AugmentName, Description: row.AugmentDescription, Source: "hexdata"}},
			Rarity:             hexdataRarityLabel(row.Rarity),
			Score:              row.HexScore,
			WinRate:            row.PairWinRate * 100,
			PickRate:           row.PickRate * 100,
			Games:              row.Games,
			DeltaWinRate:       row.DeltaWinRate,
			WilsonLowerWinRate: row.WilsonLowerWinRate,
			HexTier:            row.HexTier,
			HexLabel:           row.HexLabel,
			HexTierColor:       row.HexTierColor,
			OfficialTier:       row.Tier,
			Stages:             hexdataAugmentStageMetricRows(row.Stages),
		})
	}
	return result
}

// hexdataAugmentStageMetricRows 把上游 augments[].stages[] 裁剪成下发结构
// （R116-B P0-6）。单位换算与 hexdataAugmentMetricRows 逐字一致：胜率/阶段基准
// 胜率 ×100 变成百分数，deltaWinRate 与 wilsonLowerWinRate 保持上游 0..1 原值
// （前端展示时自己 ×100）。
//
// 三条与父行不同的取舍，都是有意为之：
//  1. 不按 hexdataMinimumSample 过滤。阶段 3/4 的样本天然小（实测某条棱彩
//     augment 阶段 3 只有 373 场、阶段 4 只有 250 场），把它们整条丢掉等于
//     让阶段视图凭空少一行；保留并用 sampleTier="low" + 「样本极少」徽记披露，
//     才是 P0-2 要的口径。
//  2. stage 号非法（<=0）的整条丢弃。前端按 stage 号取行，一个 0 号 stage 会
//     被当成「阶段 0」渲染出来，那是编造一个不存在的阶段。
//  3. 带 PickRate：工单 P0-6「实现要求」第 1 条明写点击阶段 chip 后要重新渲染该
//     阶段的 winRate/deltaWinRate/pickRate 三个值。评审整改 B6 曾以「前端零渲染」
//     为由把它裁掉，主控判定工单的字面要求优先于省体积建议，已恢复下发并在阶段
//     视图里占一格（约 +10.9 KB/英雄，局内链路的体积另由 B5 清 Stages 抵掉）。同时带 Grade——阶段
//     视图的字母徽章必须与同一张卡上的官方档位 hexLabel 同口径，否则同一张卡会
//     同时出现「徽章 S（父行 hang）」与「官方档位 顶级（阶段 top）」这种自相矛盾
//     的表述（评审整改 B4）。字母档位复用父行那一个 hexdataOfficialGrade，映射
//     只有一份实现；上游给 insufficient 时它返回空串 → omitempty → 键不存在 →
//     前端整块隐藏徽章，绝不退回父行的英雄级字母（那正是 B4 要治的口径混用）。
func hexdataAugmentStageMetricRows(rows []hexdataAugmentStageRow) []championMetricStageRow {
	if len(rows) == 0 {
		return nil
	}
	result := make([]championMetricStageRow, 0, len(rows))
	for _, row := range rows {
		if row.Stage <= 0 {
			continue
		}
		result = append(result, championMetricStageRow{
			Stage:                row.Stage,
			WinRate:              row.WinRate * 100,
			PickRate:             row.PickRate * 100,
			DeltaWinRate:         row.DeltaWinRate,
			WilsonLowerWinRate:   row.WilsonLowerWinRate,
			StageBaselineWinRate: row.StageBaselineWinRate * 100,
			Games:                row.Games,
			HexLabel:             row.HexLabel,
			Grade:                hexdataOfficialGrade(row.HexTier),
		})
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// ---- R116-F P1：该英雄专属的「阶段 × 稀有度」概率分布 ----

// hexdataHeroStageRarityRow 是详情页下发的单个阶段的稀有度概率（英雄专属口径）。
// 单位与全服口径的 hexdataRarityStage 逐字一致：百分数（上游 0..1 已 ×100），
// 前端两处都能直接用 percent() 渲染，不需要再判断该不该换算。
//
// Augments 是这一阶段真的有 stage 行的海克斯条数（实测英雄 157 的阶段 1 只有
// 121 条，不是 126），Total 是三组概率之和（实测 100.0001%~100.0003%）。两个
// 字段都下发，是为了让「阶段 1 少 5 条」与「三组之和确实是 1.0」这两件事在响应
// 里可见，而不是只写在文档里。
type hexdataHeroStageRarityRow struct {
	Stage     int     `json:"stage"`
	Silver    float64 `json:"silver"`
	Gold      float64 `json:"gold"`
	Prismatic float64 `json:"prismatic"`
	Augments  int     `json:"augments"`
	Total     float64 `json:"total,omitempty"`
}

// computeHeroStageRarityDistribution 按阶段把该英雄全部海克斯的 stages[].pickRate
// 按稀有度分组求和，返回 map[阶段]map[稀有度]概率（工单 R116-F P1 实现要求第 1 条，
// 签名与工单逐字一致）。概率保持上游 0..1 原值，不做任何归一化：实测英雄 157 全量
// 126 条 augment 的四个阶段求和分别是 1.000003 / 1.000003 / 1.000001 / 1.000001，
// 上游本身就是阶段内归一化的，我们再除一次只会平白引入误差。
//
// 两条实测钉住的口径（证据见 docs/r116f-execution-ledger.md）：
//  1. 只用 stages[].pickRate。augment 顶层的 PickRate 是跨阶段选取率，实测 126 条
//     顶层值求和 = 3.791613（棱彩 1.235358 / 黄金 1.671008 / 白银 0.885247），
//     拿它分组会得到「三组之和 3.79」这种一眼假的结果。
//  2. 缺失的 stage 行直接跳过，绝不补 pickRate=0 的零行。实测阶段 1 只有 121/126
//     条 augment 有 stage 行（缺的 5 条：男爵之手 / 属性！/ 闪光弹 / 闪闪现现 /
//     面包和果酱），补 0 既稀释概率、又让「三组之和 = 1.0」在阶段 1 失效。
//
// 稀有度取自父级 augment 的 Rarity（stage 行里没有这个字段），经 hexdataRarityLabel
// 映射成站内口径的 silver/gold/prismatic；映射不出来的行整条跳过——上游新增第四种
// 稀有度时，这里表现为该阶段 Total 明显小于 100%（响应里可见的降级），而不是多出
// 一个空字符串键被前端渲染成「未知 0.00%」。
func computeHeroStageRarityDistribution(augments []hexdataAugmentRowV2) map[int]map[string]float64 {
	distribution := make(map[int]map[string]float64)
	for _, augment := range augments {
		rarity := hexdataRarityLabel(augment.Rarity)
		if rarity == "" {
			continue
		}
		for _, stage := range augment.Stages {
			if stage.Stage <= 0 {
				continue
			}
			groups, ok := distribution[stage.Stage]
			if !ok {
				groups = make(map[string]float64, 3)
				distribution[stage.Stage] = groups
			}
			groups[rarity] += stage.PickRate
		}
	}
	return distribution
}

// hexdataHeroStageRarityRows 把分布换算成下发结构：百分数、阶段号升序、每阶段带上
// 条数与三组之和。没有任何可用阶段时返回 nil（前端整块不渲染，不留一个空壳）。
func hexdataHeroStageRarityRows(augments []hexdataAugmentRowV2) []hexdataHeroStageRarityRow {
	distribution := computeHeroStageRarityDistribution(augments)
	if len(distribution) == 0 {
		return nil
	}
	counts := make(map[int]int, len(distribution))
	for _, augment := range augments {
		if hexdataRarityLabel(augment.Rarity) == "" {
			continue
		}
		for _, stage := range augment.Stages {
			if stage.Stage > 0 {
				counts[stage.Stage]++
			}
		}
	}
	stages := make([]int, 0, len(distribution))
	for stage := range distribution {
		stages = append(stages, stage)
	}
	sort.Ints(stages)
	rows := make([]hexdataHeroStageRarityRow, 0, len(stages))
	for _, stage := range stages {
		groups := distribution[stage]
		row := hexdataHeroStageRarityRow{
			Stage:     stage,
			Silver:    groups["silver"] * 100,
			Gold:      groups["gold"] * 100,
			Prismatic: groups["prismatic"] * 100,
			Augments:  counts[stage],
		}
		row.Total = row.Silver + row.Gold + row.Prismatic
		// 三组全 0 说明这个阶段一条有效样本都没有，下发一行「0.00% / 0.00% / 0.00%」
		// 等于显示不存在的数据（评审 6.1：取不到就整块隐藏）。
		if row.Total <= 0 {
			continue
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil
	}
	return rows
}

// reportMayhemStageRarity 把英雄专属阶段概率的自检值写成一条诊断事件（日志完备
// 红线）：工单 P1 的验证判据是「三组之和 = 1.0 ± 0.01」，上游哪天改了 pickRate
// 口径，这里会先于用户看到一个明显不对的百分比就报出来。deviatingStages 是偏离
// 超过 ±1 个百分点（= ±0.01）的阶段数，正常恒为 0。
func (p *championProvider) reportMayhemStageRarity(championID int, rows []hexdataHeroStageRarityRow) {
	if p.diag == nil || len(rows) == 0 {
		return
	}
	stages := make([]map[string]any, 0, len(rows))
	deviating := 0
	for _, row := range rows {
		if math.Abs(row.Total-100) > 1 {
			deviating++
		}
		stages = append(stages, map[string]any{"stage": row.Stage, "augments": row.Augments, "total": row.Total})
	}
	p.diag(map[string]any{"event": "mayhem_stage_rarity", "championId": championID, "stages": stages, "deviatingStages": deviating})
}

// ---- R116-F P2：负向推荐（慎选）判据 ----

// mayhemPickRateMedianByRarity 按稀有度分组算选取率中位数。必须先分组：棱彩/黄金/
// 白银的天然选取率量级不同（实测英雄 157 全量 126 条：棱彩中位数 1.6190%、黄金
// 2.3460%、白银 1.4201%），混在一起算一个中位数会让判据①对低量级稀有度几乎恒成立、
// 对高量级稀有度几乎恒不成立，判据直接失效。Rarity 为空的行（上游缺字段、OP.GG RSC
// 回退行）既不进池子也不参与判定：没有「同稀有度」就谈不上同稀有度中位数。
func mayhemPickRateMedianByRarity(rows []championMetricRow) map[string]float64 {
	pools := make(map[string][]float64)
	for index := range rows {
		rarity := strings.TrimSpace(rows[index].Rarity)
		if rarity == "" {
			continue
		}
		pools[rarity] = append(pools[rarity], rows[index].PickRate)
	}
	medians := make(map[string]float64, len(pools))
	for rarity, values := range pools {
		medians[rarity] = mayhemMedian(values)
	}
	return medians
}

// mayhemMedian 是标准定义的中位数：奇数个取中间那个，偶数个取中间两个的平均
// （工单判据①没有另给定义，就不自造口径）。空池子返回 0。入参不被修改。
func mayhemMedian(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}

// markMayhemNegativeRecommendations 给命中「负向推荐（慎选）」判据的海克斯行打
// Caution 标记，并把判据①用到的同稀有度中位数一并写进行里（前端文案要展示它，
// 免得再算一遍）。返回命中条数，供诊断事件使用。
//
// 三条判据必须同时成立（工单 R116-F P2 实现要求第 1 条）：
//
//	① PickRate 严格超过该英雄同稀有度海克斯的中位数；
//	② DeltaWinRate < 0；
//	③ SampleTier != "low"（R116-B P0-2 已落地的分档，阈值来自 meta.samplePolicy，
//	   这里直接复用它的产出，不另算一套阈值）。
//
// 调用方必须在 samplePolicy 可用时才调它：策略不可用时所有行的 SampleTier 都是空
// 串，判据③会对每一行都成立——那是把「不知道样本量」当成「不是低样本」，属于用
// 推断值代替真实值。宁可一条慎选提示都不给。
// 函数是幂等的：每次先把两个字段清零再判定，重复调用不会累积。
func markMayhemNegativeRecommendations(rows []championMetricRow) int {
	medians := mayhemPickRateMedianByRarity(rows)
	if len(medians) == 0 {
		return 0
	}
	flagged := 0
	for index := range rows {
		row := &rows[index]
		row.Caution, row.CautionMedianPickRate = false, 0
		median, ok := medians[strings.TrimSpace(row.Rarity)]
		if !ok {
			continue
		}
		if row.PickRate <= median || row.DeltaWinRate >= 0 || row.SampleTier == "low" {
			continue
		}
		row.Caution, row.CautionMedianPickRate = true, median
		flagged++
	}
	return flagged
}

// reportMayhemCautionFlags 把慎选判据的执行情况写成一条诊断事件（日志完备红线）：
// 三条判据里任何一条的上游字段改名，表现都是「一条提示都没有」，用户看不出区别，
// 所以把每条判据各自过滤掉多少行都记下来。
func (p *championProvider) reportMayhemCautionFlags(championID int, rows []championMetricRow, flagged int, samplePolicyUsable bool) {
	if p.diag == nil {
		return
	}
	pools := make(map[string]int, 4)
	lowSample, negativeDelta := 0, 0
	for index := range rows {
		pools[rows[index].Rarity]++
		if rows[index].SampleTier == "low" {
			lowSample++
		}
		if rows[index].DeltaWinRate < 0 {
			negativeDelta++
		}
	}
	p.diag(map[string]any{
		"event": "mayhem_caution", "championId": championID, "rows": len(rows), "flagged": flagged,
		"samplePolicyUsable": samplePolicyUsable, "rarityPools": pools,
		"lowSampleRows": lowSample, "negativeDeltaRows": negativeDelta,
	})
}

// hexdataItemMetricRows 同上，装备行的资产 ID 直接来自 JSON 的 itemId。
func hexdataItemMetricRows(rows []hexdataItemRow) []championMetricRow {
	result := make([]championMetricRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, championMetricRow{
			Assets:             []championAsset{{ID: row.ItemID, Kind: "item", Name: row.ItemName, Source: "hexdata"}},
			Score:              row.HexScore,
			WinRate:            row.WinRate * 100,
			PickRate:           row.PickRate * 100,
			Games:              row.Games,
			DeltaWinRate:       row.DeltaWinRate,
			WilsonLowerWinRate: row.WilsonLowerWinRate,
			HexTier:            row.HexTier,
			HexLabel:           row.HexLabel,
			HexTierColor:       row.HexTierColor,
			OfficialTier:       row.Tier,
			CoreDelta:          row.CoreDelta,
			AverageIndex:       row.AverageIndex,
			WithoutItemWinRate: row.WithoutItemWinRate * 100,
		})
	}
	return result
}

// decorateHexdataItemAssets 按 JSON 给的真实 itemId 装饰装备资产（图标、说明、
// Data Dragon 路径）。旧 decorateHexdataItems 是按中文名反查目录，重名或带特殊
// 字符就会走错装备，已随 HTML 数据源一起删除；这里只做精确 ID 查表。
func (p *championProvider) decorateHexdataItemAssets(ctx context.Context, rows []championMetricRow) {
	descriptions, err := p.loadStaticDescriptions(ctx)
	if err != nil {
		// 目录不可用时保留上游给的真实 ID 与中文名，只缺图标，绝不做名称猜测。
		if p.diag != nil {
			p.diag(map[string]any{"event": "hexdata_item_asset_catalog_unavailable", "rows": len(rows), "error": err.Error()})
		}
		return
	}
	matched := 0
	missing := make([]int, 0, 4)
	for index := range rows {
		for assetIndex := range rows[index].Assets {
			asset := &rows[index].Assets[assetIndex]
			if asset.Kind != "item" || asset.ID <= 0 {
				continue
			}
			if asset.Path == "" {
				asset.Path = p.ddragonAssetPath("item", asset.ID)
			}
			if asset.Source == "" {
				asset.Source = "ddragon"
			}
			// 目录键约定与 decorateChampionAsset 一致：item/<id>.png。
			if _, ok := descriptions["item/"+strconv.Itoa(asset.ID)+".png"]; ok {
				matched++
			} else if len(missing) < 8 {
				missing = append(missing, asset.ID)
			}
			decorateChampionAsset(asset, descriptions)
		}
	}
	if p.diag != nil {
		event := map[string]any{"event": "hexdata_item_asset", "rows": len(rows), "matched": matched}
		if len(missing) > 0 {
			event["missingIds"] = missing
		}
		p.diag(event)
	}
}

// parseHexdataPostmatch 解析 /api/hexdata/postmatch：顶层 map 的 key 是英雄 ID
// 字符串。key 转不成合法 ID 的条目整条丢弃并计数，不塞进 map 的 0 键。
// R116-A 的签名与语义保持不变；字段存在性核对拆到了下面的 WithPresence 版本。
func parseHexdataPostmatch(data []byte) (map[int]hexdataPostmatchRow, int, error) {
	rows, _, dropped, err := parseHexdataPostmatchWithPresence(data)
	return rows, dropped, err
}

// parseHexdataPostmatchWithPresence 在上面的基础上额外返回「每个英雄实际在 JSON
// 里出现过哪些字段名」。
// 为什么需要它：encoding/json 对缺失字段静默填 0，光看 hexdataPostmatchRow 无法
// 区分「上游给了 0」和「上游根本没给这个字段」。把「没给」渲染成 0.0 就是显示
// 不存在的数据（项目数据准确性红线 + 评审 6.1「取不到就整块隐藏」）。实测
// 173 英雄 × 22 字段无任何缺失，所以这份集合平时全是 22 项；它是给上游改字段名
// 或漏发字段时兜底的，届时对应指标会被整条隐去而不是显示成 0。
func parseHexdataPostmatchWithPresence(data []byte) (map[int]hexdataPostmatchRow, map[int]map[string]bool, int, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, 0, err
	}
	rows := make(map[int]hexdataPostmatchRow, len(raw))
	present := make(map[int]map[string]bool, len(raw))
	dropped := 0
	for key, body := range raw {
		id, ok := hexdataParseID(key)
		if !ok {
			dropped++
			continue
		}
		var row hexdataPostmatchRow
		if err := json.Unmarshal(body, &row); err != nil {
			dropped++
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(body, &fields); err != nil {
			dropped++
			continue
		}
		names := make(map[string]bool, len(fields))
		for name := range fields {
			names[name] = true
		}
		rows[id] = row
		present[id] = names
	}
	return rows, present, dropped, nil
}

// loadHexdataPostmatch 取全量赛后指标：一次请求覆盖全部英雄，按 buildID 缓存。
func (p *championProvider) loadHexdataPostmatch(ctx context.Context) (hexdataPostmatch, error) {
	snapshot, err := p.loadHexdataMeta(ctx)
	if err != nil {
		return hexdataPostmatch{}, err
	}
	page, err := p.hexdata.load(ctx, "postmatch", hexdataAggregateID, hexdataPostmatchPath, "application/json", false)
	if err != nil {
		p.reportHexdataFallback("postmatch", err)
		return hexdataPostmatch{}, err
	}
	rows, present, dropped, parseErr := parseHexdataPostmatchWithPresence(page.Data)
	citation := hexdataJSONCitation(snapshot, hexdataAggregateCanonical)
	if parseErr != nil || len(rows) < hexdataAggregateHeroMin || !hexdataCitationComplete(citation) {
		p.hexdata.recordShapeFailure("postmatch", len(rows), len(rows), citation.BuildID)
		return hexdataPostmatch{}, firstError(parseErr, errors.New("hexdata postmatch payload is incomplete"))
	}
	p.hexdata.promote("postmatch", hexdataAggregateID, citation.BuildID, page.Data, page.FetchedAt)
	if page.Cache != championCacheStateStale {
		p.hexdata.recordSuccess("postmatch", hexdataPostmatchPath)
	}
	p.reportHexdataShape("postmatch", len(rows), len(rows), citation.BuildID)
	if dropped > 0 && p.diag != nil {
		p.diag(map[string]any{"event": "hexdata_postmatch_dropped", "rows": dropped})
	}
	return hexdataPostmatch{Citation: citation, FetchedAt: page.FetchedAt, Heroes: rows, Present: present}, nil
}

// hexdataPerformanceField 描述 /api/hexdata/postmatch 22 项里的一项：展示标签、
// 所属分组、数值格式、以及能不能做「较全英雄平均 ±%」。
//
// 量纲（R116 实测，173 英雄逐字段核算）分三类：
//  1. 每局均值（11 个 avg*）——跨英雄可比，带 ±%；
//  2. 比率/评分（damageShare、goldEfficiency、kda、killParticipation、
//     multiKillRate、survivability、utilityScore）——跨英雄可比，带 ±%；
//  3. 原始累计计数（doubleKills/tripleKills/quadraKills/pentaKills）——是「该
//     英雄全部对局的总次数」，实测 pearson(总场次, doubleKills)=0.8117，对它
//     们做 ±% 量到的是人气不是强度 → Cumulative=true，永不计算 DeltaPercent，
//     前端必须去掉 ±% 并标注「累计次数（受出场场次影响，不可跨英雄直接比较）」。
//
// multiKillRate 的口径无法从公开字段反推（实测 ≠ (d+t+q+p)/games，比值
// 1.16~1.25 不恒定），所以这里不给它任何解释性文案，只原样展示数值 + ±%。
type hexdataPerformanceField struct {
	Key        string
	Label      string
	Group      string
	Format     string
	Cumulative bool
	value      func(hexdataPostmatchRow) float64
}

// hexdataPerformanceFields 是「表现」tab 的四组展示顺序（工单 P1-6-1：战斗 KDA
// / 伤害输出 / 生存辅助 / 经济节奏）。22 项按这个顺序归组，10+4+5+3=22。
var hexdataPerformanceFields = []hexdataPerformanceField{
	{Key: "kda", Label: "KDA", Group: "战斗 KDA", Format: "decimal2", value: func(row hexdataPostmatchRow) float64 { return row.KDA }},
	{Key: "avgKills", Label: "平均击杀", Group: "战斗 KDA", Format: "decimal2", value: func(row hexdataPostmatchRow) float64 { return row.AvgKills }},
	{Key: "avgDeaths", Label: "平均死亡", Group: "战斗 KDA", Format: "decimal2", value: func(row hexdataPostmatchRow) float64 { return row.AvgDeaths }},
	{Key: "avgAssists", Label: "平均助攻", Group: "战斗 KDA", Format: "decimal2", value: func(row hexdataPostmatchRow) float64 { return row.AvgAssists }},
	{Key: "killParticipation", Label: "击杀参与率", Group: "战斗 KDA", Format: "percent", value: func(row hexdataPostmatchRow) float64 { return row.KillParticipation }},
	{Key: "multiKillRate", Label: "多杀指标", Group: "战斗 KDA", Format: "decimal2", value: func(row hexdataPostmatchRow) float64 { return row.MultiKillRate }},
	{Key: "doubleKills", Label: "双杀累计次数", Group: "战斗 KDA", Format: "count", Cumulative: true, value: func(row hexdataPostmatchRow) float64 { return float64(row.DoubleKills) }},
	{Key: "tripleKills", Label: "三杀累计次数", Group: "战斗 KDA", Format: "count", Cumulative: true, value: func(row hexdataPostmatchRow) float64 { return float64(row.TripleKills) }},
	{Key: "quadraKills", Label: "四杀累计次数", Group: "战斗 KDA", Format: "count", Cumulative: true, value: func(row hexdataPostmatchRow) float64 { return float64(row.QuadraKills) }},
	{Key: "pentaKills", Label: "五杀累计次数", Group: "战斗 KDA", Format: "count", Cumulative: true, value: func(row hexdataPostmatchRow) float64 { return float64(row.PentaKills) }},

	{Key: "avgDamage", Label: "平均伤害", Group: "伤害输出", Format: "int", value: func(row hexdataPostmatchRow) float64 { return row.AvgDamage }},
	{Key: "damageShare", Label: "团队伤害占比", Group: "伤害输出", Format: "percent", value: func(row hexdataPostmatchRow) float64 { return row.DamageShare }},
	{Key: "avgTurretDmg", Label: "平均推塔伤害", Group: "伤害输出", Format: "int", value: func(row hexdataPostmatchRow) float64 { return row.AvgTurretDmg }},
	{Key: "avgDmgMitigated", Label: "平均减免伤害", Group: "伤害输出", Format: "int", value: func(row hexdataPostmatchRow) float64 { return row.AvgDmgMitigated }},

	{Key: "survivability", Label: "生存评分", Group: "生存辅助", Format: "decimal3", value: func(row hexdataPostmatchRow) float64 { return row.Survivability }},
	{Key: "avgDamageTaken", Label: "平均承受伤害", Group: "生存辅助", Format: "int", value: func(row hexdataPostmatchRow) float64 { return row.AvgDamageTaken }},
	{Key: "avgHealShield", Label: "平均治疗与护盾", Group: "生存辅助", Format: "int", value: func(row hexdataPostmatchRow) float64 { return row.AvgHealShield }},
	{Key: "avgCcTime", Label: "平均控制时长", Group: "生存辅助", Format: "decimal2", value: func(row hexdataPostmatchRow) float64 { return row.AvgCcTime }},
	{Key: "utilityScore", Label: "功能性评分", Group: "生存辅助", Format: "decimal3", value: func(row hexdataPostmatchRow) float64 { return row.UtilityScore }},

	{Key: "avgGold", Label: "平均金币", Group: "经济节奏", Format: "int", value: func(row hexdataPostmatchRow) float64 { return row.AvgGold }},
	{Key: "goldEfficiency", Label: "金币效率", Group: "经济节奏", Format: "decimal2", value: func(row hexdataPostmatchRow) float64 { return row.GoldEfficiency }},
	{Key: "avgCs", Label: "平均补刀", Group: "经济节奏", Format: "decimal2", value: func(row hexdataPostmatchRow) float64 { return row.AvgCs }},
}

// mayhemPerformancePanel 组装「表现」tab 的数据（R116-B P1-6）。
// 均值在后端算：工单 P1-6-2 明确要求不在前端算，避免把 173 英雄的完整表下发。
// 任何一步取不到都返回 nil → 前端整个「表现」tab 不渲染，不留空态。
func (p *championProvider) mayhemPerformancePanel(ctx context.Context, id int) *championPerformancePanel {
	postmatch, err := p.loadHexdataPostmatch(ctx)
	if err != nil {
		if p.diag != nil {
			p.diag(map[string]any{"event": "hexdata_performance_unavailable", "championId": id, "reason": err.Error()})
		}
		return nil
	}
	row, ok := postmatch.Heroes[id]
	if !ok {
		// 这个英雄不在 postmatch 里：整块隐藏。绝不拿 0 顶替 22 项指标。
		if p.diag != nil {
			p.diag(map[string]any{"event": "hexdata_performance_hero_missing", "championId": id, "heroes": len(postmatch.Heroes)})
		}
		return nil
	}
	present := postmatch.Present[id]
	// 均值池：只统计「字段确实出现过」的英雄；累计计数类字段不进池，因为它们
	// 没有可比的均值口径（量到的是人气）。
	sums := make([]float64, len(hexdataPerformanceFields))
	counts := make([]int, len(hexdataPerformanceFields))
	for heroID, hero := range postmatch.Heroes {
		heroPresent := postmatch.Present[heroID]
		for index, field := range hexdataPerformanceFields {
			if field.Cumulative || (len(heroPresent) > 0 && !heroPresent[field.Key]) {
				continue
			}
			sums[index] += field.value(hero)
			counts[index]++
		}
	}
	metrics := make([]championPerformanceMetric, 0, len(hexdataPerformanceFields))
	groups := make([]string, 0, 4)
	seenGroup := make(map[string]bool, 4)
	cumulative := 0
	for index, field := range hexdataPerformanceFields {
		// 上游没给这个字段 → 整条指标不下发（前端因此不会出现 0 / N/A）。
		if len(present) > 0 && !present[field.Key] {
			continue
		}
		metric := championPerformanceMetric{
			Key: field.Key, Label: field.Label, Group: field.Group,
			Value: field.value(row), Format: field.Format, Cumulative: field.Cumulative,
		}
		if field.Cumulative {
			cumulative++
		} else if counts[index] > 0 {
			average := sums[index] / float64(counts[index])
			// 均值为 0 时百分比差没有意义（除零），按「取不到」处理：
			// HasDelta=false，前端不渲染「较平均」标签。
			if average != 0 {
				metric.DeltaPercent = (metric.Value - average) / average * 100
				metric.HasDelta = true
			}
		}
		if !seenGroup[field.Group] {
			seenGroup[field.Group] = true
			groups = append(groups, field.Group)
		}
		metrics = append(metrics, metric)
	}
	if len(metrics) == 0 {
		return nil
	}
	citation := postmatch.Citation
	if p.diag != nil {
		p.diag(map[string]any{
			"event": "hexdata_performance_panel", "championId": id,
			"metrics": len(metrics), "heroCount": len(postmatch.Heroes),
			"cumulativeMetrics": cumulative, "deltaMetrics": len(metrics) - cumulative,
		})
	}
	return &championPerformancePanel{
		Metrics: metrics, HeroCount: len(postmatch.Heroes), Groups: groups, Citation: &citation,
		MeasurementTechnique: "赛后表现指标来自 Hexdata 全量聚合（/api/hexdata/postmatch），「较平均」是相对全部英雄同一项算术均值的百分比差",
	}
}

// parseHexdataHextechInsights 解析 /api/hexdata/hextech-insights，并把英雄/海克斯
// 的字符串 ID 安全转换成 int（转换失败的条目丢弃并计数）。
func parseHexdataHextechInsights(data []byte) (hexdataHextechInsightsPayload, int, error) {
	var payload hexdataHextechInsightsPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return payload, 0, err
	}
	dropped := 0
	heroes := make([]hexdataInsightHero, 0, len(payload.Heroes))
	for _, hero := range payload.Heroes {
		id, ok := hexdataParseID(hero.ID)
		if !ok {
			dropped++
			continue
		}
		hero.ChampionID = id
		for index := range hero.TopItems {
			if assetID, ok := hexdataParseID(hero.TopItems[index].ID); ok {
				hero.TopItems[index].AssetID = assetID
			}
		}
		for index := range hero.TopAugments {
			if assetID, ok := hexdataParseID(hero.TopAugments[index].ID); ok {
				hero.TopAugments[index].AssetID = assetID
			}
		}
		heroes = append(heroes, hero)
	}
	augments := make([]hexdataInsightAugment, 0, len(payload.Augments))
	for _, augment := range payload.Augments {
		id, ok := hexdataParseID(augment.ID)
		if !ok {
			dropped++
			continue
		}
		augment.AugmentID = id
		augments = append(augments, augment)
	}
	payload.Heroes, payload.Augments = heroes, augments
	if len(payload.Heroes) == 0 || len(payload.Augments) == 0 {
		return payload, dropped, errors.New("hexdata hextech-insights payload is empty")
	}
	return payload, dropped, nil
}

// loadHexdataHextechInsights 取全量 tier/topItems/topAugments 目录：与 postmatch
// 同性质，每个 buildID 只取一次，长期缓存。R116-D 的推荐页概览优先从这里取数，
// 不需要对每个英雄单独调 /heroes/{id}。
func (p *championProvider) loadHexdataHextechInsights(ctx context.Context) (hexdataHextechInsights, error) {
	snapshot, err := p.loadHexdataMeta(ctx)
	if err != nil {
		return hexdataHextechInsights{}, err
	}
	return p.loadHexdataHextechInsightsWithSnapshot(ctx, snapshot)
}

// loadHexdataHextechInsightsWithSnapshot 是上面那个入口的实现体，单独拆出来只为
// 一件事：R116-B P0-5-2 的英雄榜单与海斗详情页已经自己取过 meta 快照（要它的
// tierBands 与 samplePolicy），复用同一份快照就不会为了官方档位再打一次
// /api/hexdata/meta。冷启动的 hexdata 请求数是预算项，有测试钉住，不能悄悄翻倍。
func (p *championProvider) loadHexdataHextechInsightsWithSnapshot(ctx context.Context, snapshot hexdataMetaSnapshot) (hexdataHextechInsights, error) {
	page, err := p.hexdata.load(ctx, "hextech-insights", hexdataAggregateID, hexdataHextechInsightsPath, "application/json", false)
	if err != nil {
		p.reportHexdataFallback("hextech-insights", err)
		return hexdataHextechInsights{}, err
	}
	payload, dropped, parseErr := parseHexdataHextechInsights(page.Data)
	citation := hexdataJSONCitation(snapshot, hexdataAggregateCanonical)
	if parseErr != nil || len(payload.Heroes) < hexdataAggregateHeroMin || !hexdataCitationComplete(citation) {
		p.hexdata.recordShapeFailure("hextech-insights", len(payload.Heroes), len(payload.Augments), citation.BuildID)
		return hexdataHextechInsights{}, firstError(parseErr, errors.New("hexdata hextech-insights payload is incomplete"))
	}
	p.hexdata.promote("hextech-insights", hexdataAggregateID, citation.BuildID, page.Data, page.FetchedAt)
	if page.Cache != championCacheStateStale {
		p.hexdata.recordSuccess("hextech-insights", hexdataHextechInsightsPath)
	}
	p.reportHexdataShape("hextech-insights", len(payload.Heroes), len(payload.Augments), citation.BuildID)
	if dropped > 0 && p.diag != nil {
		p.diag(map[string]any{"event": "hexdata_hextech_insights_dropped", "rows": dropped})
	}
	return hexdataHextechInsights{hexdataHextechInsightsPayload: payload, Citation: citation, FetchedAt: page.FetchedAt}, nil
}

// hexdataOfficialHeroTiers 取官方的英雄级档位（hextech-insights.heroes[].tier，
// 实测 T1-T5 整数），供 R116-B P0-5-2 替换 hexdata.go 里那句
// 「Tier: index*5/len(rows)+1」的本地顺序分档。
//
// 这是尽力而为的一条支线：insights 熔断/降级/形状校验失败时返回 nil，调用方
// 必须回退到本地分档并把 TierLocallyCalculated 置 true。整张英雄榜单不能因为
// 少一个装饰性的官方档位而拉不出来。meta 都取不到时连 insights 请求都不发——
// loadHexdataHextechInsightsWithSnapshot 的 citation 要靠 meta 快照才拼得出来。
func (p *championProvider) hexdataOfficialHeroTiers(ctx context.Context, snapshot hexdataMetaSnapshot, metaErr error) map[int]int {
	if metaErr != nil {
		return nil
	}
	insights, err := p.loadHexdataHextechInsightsWithSnapshot(ctx, snapshot)
	if err != nil {
		// loadHexdataHextechInsightsWithSnapshot 内部已经报过 hextech-insights
		// 的 fallback，这里补一条带用途的，方便排障时区分「谁在要官方档位」。
		if p.diag != nil {
			p.diag(map[string]any{"event": "hexdata_official_hero_tiers_unavailable", "reason": err.Error()})
		}
		return nil
	}
	tiers := make(map[int]int, len(insights.Heroes))
	for _, hero := range insights.Heroes {
		// tier<=0 或 ID 非法的条目不进 map：留空让调用方走本地兜底，
		// 比塞一个 0 进去（前端会渲染成「OP」档）安全得多。
		if hero.ChampionID > 0 && hero.Tier > 0 {
			tiers[hero.ChampionID] = hero.Tier
		}
	}
	if len(tiers) == 0 {
		return nil
	}
	return tiers
}

func parseHexdataAugments(data []byte) ([]championAugment, championSourceCitation, error) {
	rows, citation, _, err := parseHexdataAugmentsWithStats(data)
	return rows, citation, err
}

func parseHexdataAugmentsWithStats(data []byte) ([]championAugment, championSourceCitation, hexdataAugmentParseStats, error) {
	document, buildID, err := parseHexdataDocument(data)
	if err != nil {
		return nil, championSourceCitation{}, hexdataAugmentParseStats{}, err
	}
	tables := primaryTables(document)
	if len(tables) != 1 {
		return nil, championSourceCitation{}, hexdataAugmentParseStats{}, errors.New("hexdata augments table changed")
	}
	rows := make([]championAugment, 0, 220)
	stats := hexdataAugmentParseStats{}
	for rowIndex, cells := range tableRows(tables[0]) {
		if rowIndex == 0 {
			stats.Col0LinkTextLen, stats.Col0TextLen = hexdataCellTextLengths(cells, 0)
			stats.Col2LinkTextLen, stats.Col2TextLen = hexdataCellTextLengths(cells, 2)
		}
		if len(cells) < 2 {
			stats.SkipShortCells++
			continue
		}
		href, name := firstLink(cells[0])
		path := hexdataAugmentPathPattern.FindStringSubmatch(hexdataLinkPath(href))
		if len(path) != 3 {
			stats.SkipPathUnmatched++
			continue
		}
		performance, winRate, performanceUnavailable, ok := parseHexdataAugmentMetrics(hexdataNodeText(cells[1]))
		if !ok {
			stats.SkipMetricsUnmatched++
			continue
		}
		if name == "" && len(cells) > 2 {
			_, name = firstLink(cells[2])
		}
		if name == "" {
			stats.SkipEmptyName++
			continue
		}
		id, _ := strconv.Atoi(path[1])
		if id <= 0 {
			stats.SkipBadID++
			continue
		}
		rows = append(rows, championAugment{ID: id, Key: path[2], Name: name, Performance: performance, PerformanceUnavailable: performanceUnavailable, WinRate: winRate})
	}
	citation := hexdataCitation(document, buildID, "/augments")
	if len(rows) < 150 {
		return rows, citation, stats, errors.New("hexdata augments shape is incomplete")
	}
	return rows, citation, stats, nil
}

func parseHexdataAugmentMetrics(value string) (performance, winRate float64, performanceUnavailable, ok bool) {
	if metrics := hexdataGlobalMetric.FindStringSubmatch(value); len(metrics) == 3 {
		return parseFloatText(metrics[1]), parseFloatText(metrics[2]), false, true
	}
	if metrics := hexdataAugmentWinRate.FindStringSubmatch(value); len(metrics) == 2 {
		return 0, parseFloatText(metrics[1]), true, true
	}
	return 0, 0, false, false
}

func hexdataCellTextLengths(cells []*xhtml.Node, index int) (linkTextLen, textLen int) {
	if index < 0 || index >= len(cells) {
		return 0, 0
	}
	_, linkText := firstLink(cells[index])
	return utf8.RuneCountInString(linkText), utf8.RuneCountInString(hexdataNodeText(cells[index]))
}

func hexdataLinkPath(href string) string {
	href = strings.TrimSpace(href)
	parsed, err := url.Parse(href)
	if err != nil || parsed.Path == "" {
		return href
	}
	return parsed.Path
}

func parseHexdataAugmentDetail(data []byte, canonical string) (hexdataAugmentDetail, error) {
	document, buildID, err := parseHexdataDocument(data)
	if err != nil {
		return hexdataAugmentDetail{}, err
	}
	tables := primaryTables(document)
	if len(tables) < 1 {
		return hexdataAugmentDetail{}, errors.New("hexdata augment detail table changed")
	}
	result := hexdataAugmentDetail{Citation: hexdataCitation(document, buildID, canonical), Description: hexdataAugmentGuideDescription(document)}
	for _, cells := range tableRows(tables[0]) {
		if len(cells) != 4 {
			continue
		}
		href, name := firstLink(cells[0])
		match := hexdataHeroPathPattern.FindStringSubmatch(hexdataLinkPath(href))
		if len(match) != 3 {
			continue
		}
		id, _ := strconv.Atoi(match[1])
		games := parseCountText(hexdataNodeText(cells[3]))
		result.Rows = append(result.Rows, championAugmentChampion{ID: id, Name: name, Key: match[2], Score: parseFloatText(hexdataNodeText(cells[1])), WinRate: parsePercentText(hexdataNodeText(cells[2])), Games: games})
	}
	if len(result.Rows) != 12 {
		return result, errors.New("hexdata augment detail shape is incomplete")
	}
	return result, nil
}

// hexdataAugmentGuideDescription pulls the augment's own gameplay text out of
// the page's guide paragraph. Riot ships no description for Mayhem augments —
// cherry-augments.json (both the client copy and its CommunityDragon mirror)
// has no description field, /latest/cdragon/ only exposes arena/, and
// arena/zh_cn.json stops at ID 405 — so this rendered sentence is the only
// Chinese copy available for ID >= 1000.
//
// Shape (verified on 1225-dual-wield and 1116-flashy):
//
//	<section class="seo-guide-section" data-seo-guide><h2>…</h2>
//	<p>{名称}更适合先在A、B、C这类高分高样本英雄上考虑。{说明}如果你的英雄机制能…</p>
//
// Both anchors must be present; the page's <meta name="description"> is a
// win-rate blurb and must never be used as the augment description.
func hexdataAugmentGuideDescription(document *xhtml.Node) string {
	const (
		prefixAnchor = "这类高分高样本英雄上考虑。"
		suffixAnchor = "如果你的英雄机制"
	)
	var guide *xhtml.Node
	var find func(*xhtml.Node)
	find = func(node *xhtml.Node) {
		if guide == nil && node.Type == xhtml.ElementNode && hasAttribute(node, "data-seo-guide") {
			guide = node
			return
		}
		for child := node.FirstChild; child != nil && guide == nil; child = child.NextSibling {
			find(child)
		}
	}
	find(document)
	if guide == nil {
		return ""
	}
	for _, paragraph := range descendantElements(guide, "p") {
		text := hexdataNodeText(paragraph)
		start := strings.Index(text, prefixAnchor)
		if start < 0 {
			continue
		}
		text = text[start+len(prefixAnchor):]
		end := strings.Index(text, suffixAnchor)
		if end <= 0 {
			continue
		}
		if description := strings.TrimSpace(text[:end]); description != "" {
			return description
		}
	}
	return ""
}

func parseHexdataRarity(data []byte) ([]hexdataRarityStage, championSourceCitation, error) {
	document, buildID, err := parseHexdataDocument(data)
	if err != nil {
		return nil, championSourceCitation{}, err
	}
	tables := primaryTables(document)
	if len(tables) != 1 {
		return nil, championSourceCitation{}, errors.New("hexdata rarity table changed")
	}
	metricPattern := regexp.MustCompile(`白银\s*([0-9.]+)%\s*[·，,]\s*黄金\s*([0-9.]+)%\s*[·，,]\s*棱彩\s*([0-9.]+)%`)
	stagePattern := regexp.MustCompile(`^第\s*([1-4])\s*阶段$`)
	rows := make([]hexdataRarityStage, 0, 4)
	for _, cells := range tableRows(tables[0]) {
		if len(cells) != 3 {
			continue
		}
		stageMatch := stagePattern.FindStringSubmatch(hexdataNodeText(cells[0]))
		metrics := metricPattern.FindStringSubmatch(hexdataNodeText(cells[1]))
		if len(stageMatch) != 2 || len(metrics) != 4 {
			continue
		}
		stage, _ := strconv.Atoi(stageMatch[1])
		rows = append(rows, hexdataRarityStage{Stage: stage, Silver: parseFloatText(metrics[1]), Gold: parseFloatText(metrics[2]), Prismatic: parseFloatText(metrics[3]), Games: parseCountText(hexdataNodeText(cells[2]))})
	}
	citation := hexdataCitation(document, buildID, "/augment-rarity")
	if len(rows) != 4 {
		return rows, citation, errors.New("hexdata rarity shape is incomplete")
	}
	for index, row := range rows {
		if row.Stage != index+1 || row.Games <= 0 || row.Silver < 0 || row.Gold < 0 || row.Prismatic < 0 || row.Silver > 100 || row.Gold > 100 || row.Prismatic > 100 || math.Abs(row.Silver+row.Gold+row.Prismatic-100) > 0.21 {
			return rows, citation, errors.New("hexdata rarity distribution is invalid")
		}
	}
	return rows, citation, nil
}

func (p *championProvider) loadHexdataAugmentDetail(ctx context.Context, id int, slug string) (championAugmentDetailResponse, error) {
	if id <= 0 {
		return championAugmentDetailResponse{}, errors.New("invalid hexdata augment")
	}
	slug = sanitizeAugmentSlug(slug)
	if slug == "" {
		catalog, catalogErr := p.loadCommunityDragonAugments(ctx)
		if catalogErr != nil {
			return championAugmentDetailResponse{}, catalogErr
		}
		meta, ok := gameplayAugmentIndexAll(catalog)[id]
		if !ok {
			return championAugmentDetailResponse{}, errors.New("hexdata augment metadata unavailable")
		}
		return championAugmentDetailResponse{ID: id, Slug: "", Source: "CommunityDragon fallback", Description: meta.Description, Champions: []championAugmentChampion{}}, nil
	}
	canonical := "/augment/" + strconv.Itoa(id) + "-" + slug
	if !hexdataAugmentPathPattern.MatchString(canonical) {
		return championAugmentDetailResponse{}, errors.New("invalid hexdata augment")
	}
	page, err := p.hexdata.load(ctx, "augment", strconv.Itoa(id), canonical, "text/html,application/xhtml+xml", false)
	if err != nil {
		p.reportHexdataFallback("augment-detail", err)
		return championAugmentDetailResponse{}, err
	}
	detail, err := parseHexdataAugmentDetail(page.Data, canonical)
	if err != nil || !hexdataCitationComplete(detail.Citation) || page.Cache != championCacheStateStale && !p.hexdata.adoptCitation(detail.Citation) {
		p.hexdata.recordShapeFailure("augment", len(detail.Rows), hexdataTableFieldCount(page.Data), detail.Citation.BuildID)
		return championAugmentDetailResponse{}, errors.New("hexdata augment detail changed")
	}
	p.hexdata.promote("augment", strconv.Itoa(id), detail.Citation.BuildID, page.Data, page.FetchedAt)
	if page.Cache != championCacheStateStale {
		p.hexdata.recordSuccess("augment", canonical)
	}
	for index := range detail.Rows {
		if meta, metaErr := p.championMetadataByID(ctx, detail.Rows[index].ID); metaErr == nil {
			detail.Rows[index].ImageSource, detail.Rows[index].ImagePath = meta.ImageSource, meta.ImagePath
		}
	}
	p.reportHexdataShape("augment", len(detail.Rows), hexdataTableFieldCount(page.Data), detail.Citation.BuildID)
	description := detail.Description
	if strings.TrimSpace(description) == "" {
		if catalog, catalogErr := p.loadCommunityDragonAugments(ctx); catalogErr == nil {
			if meta, ok := gameplayAugmentIndexAll(catalog)[id]; ok {
				description = strings.TrimSpace(meta.Description)
			}
		}
	}
	return championAugmentDetailResponse{ID: id, Slug: slug, Source: "Hexdata", Description: description, Citation: &detail.Citation, MeasurementTechnique: p.loadHexdataMeasurementTechnique(ctx), Champions: detail.Rows}, nil
}

func (p *championProvider) loadHexdataRarity(ctx context.Context) (championAugmentRarityResponse, error) {
	page, err := p.hexdata.load(ctx, "rarity", "stages", "/augment-rarity", "text/html,application/xhtml+xml", false)
	if err != nil {
		p.reportHexdataFallback("rarity", err)
		return championAugmentRarityResponse{}, err
	}
	rows, citation, err := parseHexdataRarity(page.Data)
	if err != nil || !hexdataCitationComplete(citation) || page.Cache != championCacheStateStale && !p.hexdata.adoptCitation(citation) {
		p.hexdata.recordShapeFailure("rarity", len(rows), hexdataTableFieldCount(page.Data), citation.BuildID)
		return championAugmentRarityResponse{}, errors.New("hexdata rarity page changed")
	}
	p.hexdata.promote("rarity", "stages", citation.BuildID, page.Data, page.FetchedAt)
	if page.Cache != championCacheStateStale {
		p.hexdata.recordSuccess("rarity", "/augment-rarity")
	}
	return championAugmentRarityResponse{Source: "Hexdata", Citation: &citation, MeasurementTechnique: p.loadHexdataMeasurementTechnique(ctx), Stages: rows}, nil
}

func (p *championProvider) decorateHexdataAugments(ctx context.Context, rows []championMetricRow) {
	catalog := p.loadAugmentMetadataCatalog(ctx)
	byID := gameplayAugmentIndexAll(catalog)
	for rowIndex := range rows {
		if len(rows[rowIndex].Assets) == 0 {
			continue
		}
		asset := &rows[rowIndex].Assets[0]
		meta, ok := byID[asset.ID]
		if !ok {
			if p.diag != nil {
				p.diag(map[string]any{"event": "hexdata_augment_metadata_missing", "id": asset.ID})
			}
			continue
		}
		if strings.TrimSpace(asset.Description) == "" && strings.TrimSpace(meta.Description) != "" {
			asset.Description = meta.Description
		}
		if strings.TrimSpace(meta.Name) != "" {
			asset.Name = meta.Name
		}
		if rarity := normalizeAugmentRarity(meta.Rarity); rarity != "unknown" {
			rows[rowIndex].Rarity = rarity
		}
		if path := communityDragonGameAssetPath(meta.IconPath); path != "" {
			asset.Source, asset.Path = "communitydragon", path
			asset.FallbackPath = communityDragonGameAssetPath(meta.FallbackIconPath)
		}
	}
	for rowIndex := range rows {
		if len(rows[rowIndex].Assets) == 0 {
			continue
		}
		asset := &rows[rowIndex].Assets[0]
		asset.Description = augmentDescriptionWithOfflineGuidance(asset.ID, asset.Description)
		// Drop Bear's colored 256px artwork has no size suffix. Its _small
		// resource is a monochrome 64px HUD glyph, not prismatic card artwork.
		if asset.ID == 2031 {
			asset.Source = "communitydragon"
			asset.Path = "/latest/game/assets/ux/cherry/augments/icons/drop_bear.png"
			asset.FallbackPath = "/latest/game/assets/ux/cherry/augments/icons/drop_bear_small.png"
		}
	}
}

// Augment IDs are unique across the whole CommunityDragon catalog (655 entries,
// zero collisions between the Cherry and ARAM_ namespaces), so metadata is
// always looked up by ID. The icon directory (/Cherry/ vs /Kiwi/) looks like a
// mode boundary but is not one: 217 of the 370 Mayhem augments ship /Cherry/
// icons and Arena's 393 艾卡西亚的陷落 ships a /Kiwi/ icon. Filtering on it used
// to drop names (leaving raw IDs like "1022"), icons and rarities.
func gameplayAugmentIndexAll(catalog []gameplayAugment) map[int]gameplayAugment {
	result := make(map[int]gameplayAugment, len(catalog))
	for _, item := range catalog {
		if item.ID > 0 {
			result[int(item.ID)] = item
		}
	}
	return result
}

func normalizeAugmentRarity(value any) string {
	switch typed := value.(type) {
	case int:
		return map[int]string{0: "silver", 1: "gold", 2: "prismatic", 4: "event"}[typed]
	case string:
		key := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(typed), "k"))
		if key == "eventchoice" {
			return "event"
		}
		if key == "silver" || key == "gold" || key == "prismatic" || key == "event" {
			return key
		}
	}
	return "unknown"
}

func (p *championProvider) loadMayhemDetail(ctx context.Context, champion string) (championDetailResponse, error) {
	id, metadata, err := p.resolveChampionID(ctx, champion)
	if err != nil {
		return championDetailResponse{}, err
	}
	slug := strings.ToLower(strings.TrimSpace(champion))
	if _, numericErr := strconv.Atoi(slug); numericErr == nil || !regexp.MustCompile(`^[a-z0-9-]+$`).MatchString(slug) {
		slug = metadata.Slug
	}
	// CanonicalURL 依旧指向给人看的 HTML 详情页；数据本身改走 JSON API。
	canonical := "/hero/" + strconv.Itoa(id) + "-" + slug
	requestPath := hexdataHeroJSONPathPrefix + strconv.Itoa(id)
	// Independent providers used to run serially on the recommendation path.
	type rscResult struct {
		detail mayhemRSCDetail
		err    error
	}
	rscReady := make(chan rscResult, 1)
	go func() {
		detail, err := p.loadMayhemRSC(ctx, metadata.Slug)
		rscReady <- rscResult{detail, err}
	}()
	var primary hexdataHeroDetailV2
	var citation championSourceCitation
	var fetchedAt time.Time
	var primaryErr error
	// JSON 响应体不带 buildId，必须先拿 meta 快照：它既提供 citation，也把
	// state.BuildID 落地，否则 hero-json 的 cache key 会退回 "bootstrap|" 前缀，
	// 磁盘缓存永远命不中。冷启动因此固定是 2 条 hexdata 请求（meta + hero-json），
	// 不再有旧链路的 per-augment 描述扇出。
	snapshot, snapshotErr := p.loadHexdataMeta(ctx)
	if snapshotErr != nil {
		primaryErr = snapshotErr
	} else {
		citation = hexdataJSONCitation(snapshot, canonical)
		page, pageErr := p.hexdata.load(ctx, "hero-json", strconv.Itoa(id), requestPath, "application/json", false)
		if pageErr != nil {
			primaryErr = pageErr
			p.reportHexdataFallback("hero-detail", pageErr)
		} else {
			detail, stats, parseErr := parseHexdataHeroJSONWithStats(page.Data)
			primary, fetchedAt = detail, page.FetchedAt
			p.reportHexdataHeroJSONStats(id, stats)
			switch {
			case parseErr != nil:
				primaryErr = parseErr
			case len(detail.Items) == 0 || len(detail.Augments) == 0:
				primaryErr = errors.New("hexdata hero json is incomplete")
			case !hexdataCitationComplete(citation):
				primaryErr = errors.New("hexdata hero citation metadata is incomplete")
			default:
				// P0-5：成功路径必须 promote。refreshBuild 那条通道用的是
				// key+"|refresh" 且 persistDisk=false，忘了 promote 就永远不落盘，
				// 每 12 小时第一次访问都强制回源。
				p.hexdata.promote("hero-json", strconv.Itoa(id), citation.BuildID, page.Data, page.FetchedAt)
				if page.Cache != championCacheStateStale {
					p.hexdata.recordSuccess("hero-json", requestPath)
				}
			}
			if primaryErr != nil {
				p.hexdata.recordShapeFailure("hero-json", len(detail.Items), len(detail.Augments), citation.BuildID)
			}
		}
	}
	loadedRSC := <-rscReady
	rsc, rscErr := loadedRSC.detail, loadedRSC.err
	if primaryErr != nil && rscErr != nil {
		return championDetailResponse{}, primaryErr
	}
	response := championDetailResponse{Mode: "hextech-aram", Region: "CN", Source: "Hexdata + OP.GG RSC", EntertainmentSample: true, MeasurementTechnique: p.loadHexdataMeasurementTechnique(ctx)}
	// R116-B：P0-2 的 sampleTier 阈值、P0-5 的 tierBands、P0-5-2 的官方英雄
	// 档位都要 meta 快照里的东西。meta 在上面已经取过一次，这里全部复用，
	// 冷启动不会为详情页多打一条 /api/hexdata/meta。
	var officialTiers map[int]int
	if snapshotErr == nil {
		officialTiers = p.hexdataOfficialHeroTiers(ctx, snapshot, nil)
	}
	if primaryErr == nil {
		response.Patch, response.FetchedAt, response.Stats.WinRate = citation.Patch, fetchedAt, primary.HeroWinRate
		// R116-B P0-5-2：英雄级梯度接 hextech-insights.heroes[].tier（官方 T1-T5）。
		// /api/hexdata/heroes/{id} 顶层只有 8 个数组、没有英雄级 tier，而
		// items[].tier / augments[].tier 都是行级档位——拿它冒充英雄梯度就是
		// 伪造数据（这正是 R116-A 把 Stats.Tier 明确降级成 nil 的原因）。
		// insights 熔断/降级时继续留 nil：推荐页的梯度徽章整块不显示，
		// 好过显示一个编出来的档位。
		if tier, ok := officialTiers[id]; ok {
			response.Stats.Tier = &tier
		}
		response.Citation = &citation
		response.RecommendedAugments = hexdataAugmentMetricRows(primary.Augments)
		response.ItemRanking = hexdataItemMetricRows(primary.Items)
		p.decorateHexdataAugments(ctx, response.RecommendedAugments)
		p.decorateHexdataItemAssets(ctx, response.ItemRanking)
		p.reportHexdataHeroShape(primary, citation)
		// R116-F P1：英雄专属的「阶段 × 稀有度」概率分布，从 hero-json 的
		// augments[].stages[] 本地聚合，不发任何新请求。用 primary.Augments 全量
		// （实测 126 条）而不是 response.RecommendedAugments：后者按
		// hexdataMinimumSample 过滤过、还可能被 RSC 合并改写，缺行会让三组之和
		// 小于 100%，判据「三组之和 = 1.0 ± 0.01」就不成立了。
		response.HeroStageRarity = hexdataHeroStageRarityRows(primary.Augments)
		p.reportMayhemStageRarity(id, response.HeroStageRarity)
	}
	// R116-B P0-2：sampleTier 由后端按 meta.samplePolicy 算好直出，前端不重复
	// 实现阈值逻辑。meta 不可用时全部留空 → 前端整块不渲染样本分档元素。
	applyHexdataSampleTiers(response.RecommendedAugments, snapshot)
	applyHexdataSampleTiers(response.ItemRanking, snapshot)
	if rscErr == nil {
		response.Build = rsc.Build
		response.BuildCitation = &rsc.Citation
		if primaryErr != nil {
			response.Source, response.Patch, response.FetchedAt = "OP.GG RSC", rsc.Citation.Patch, rsc.FetchedAt
			response.Citation = &rsc.Citation
		}
		// Keep the source rows until rarity metadata has been applied. A global
		// top-nine cut can discard every silver recommendation before grouping.
		response.RecommendedAugments = mergeMayhemAugmentRows(response.RecommendedAugments, rsc.Augments, 0)
		applyHexdataSampleTiers(response.RecommendedAugments, snapshot)
		p.decorateHexdataAugments(ctx, response.RecommendedAugments)
		// 必须在 decorateDetailAssets 之前替换召唤师技能，让 hexdata 的行也走
		// 同一套资产装饰。
		p.applyMayhemSummonerSpellPairs(&response, primary, primaryErr, snapshot)
		p.decorateDetailAssets(ctx, metadata.Slug, &response)
	} else {
		p.applyMayhemSummonerSpellPairs(&response, primary, primaryErr, snapshot)
	}
	applyLocalAugmentGrades(response.RecommendedAugments)
	// R116-B P0-5-1：档位改成官方优先之后，这句口径说明不能再无条件挂上去，
	// 否则官方档位（hexLabel「夯」「顶级」…）也会被说成「本地分位计算」。
	// 只在确实有行回退到本地算法时才披露，并给出回退行数。
	if local := countLocallyGradedRows(response.RecommendedAugments); local > 0 {
		response.MeasurementTechnique = appendMeasurementTechnique(response.MeasurementTechnique,
			fmt.Sprintf("其中 %d 项缺少官方档位，其档位为本地综合评分分位计算，非官方等级；缺少统计指标时按推荐顺序回退", local))
	}
	// R116-F P2：负向推荐（慎选）判据。放在 RSC 合并与档位回填之后，是因为它要看的
	// 是用户实际看到的那一份行（hexdata 行 + RSC 合并行），判据①的中位数池子必须
	// 与卡片同源。samplePolicy 不可用时整块跳过：那时所有行的 SampleTier 都是空串，
	// 判据③「!= low」会对每一行都成立，等于把「不知道样本量」当成「不是低样本」。
	samplePolicyUsable := snapshot.SamplePolicy.usable()
	cautioned := 0
	if samplePolicyUsable {
		cautioned = markMayhemNegativeRecommendations(response.RecommendedAugments)
	}
	p.reportMayhemCautionFlags(id, response.RecommendedAugments, cautioned, samplePolicyUsable)
	// R116-F P1：英雄专属阶段概率的口径说明，与既有说明一起落在常驻页脚。
	if len(response.HeroStageRarity) > 0 {
		response.MeasurementTechnique = appendMeasurementTechnique(response.MeasurementTechnique,
			"「该英雄专属品质概率」由该英雄 hero-json 的 augments[].stages[].pickRate 按稀有度分组求和得到，是阶段内选取率口径，不代表抽取、刷新或保底概率")
	}
	// R116-B P1-6：22 项赛后表现指标 + 后端算好的「较全英雄平均」。
	// postmatch 取不到时返回 nil → 前端整个「表现」tab 不渲染。
	response.Performance = p.mayhemPerformancePanel(ctx, id)
	if response.Performance != nil {
		response.MeasurementTechnique = appendMeasurementTechnique(response.MeasurementTechnique,
			fmt.Sprintf("表现指标为 %d 位英雄赛后数据的每局均值，「较平均」是相对全部英雄同一项均值的百分比差", response.Performance.HeroCount))
	}
	if len(response.RecommendedAugments) == 0 {
		return championDetailResponse{}, errors.New("mayhem recommendations are incomplete")
	}
	return response, nil
}

// countLocallyGradedRows 数出有多少行最终落到了本地分位算法（官方 hexLabel 与
// 可映射的 hexTier 都缺失）。只用于口径披露文案，不参与任何排序或档位计算。
func countLocallyGradedRows(rows []championMetricRow) int {
	count := 0
	for _, row := range rows {
		if !hexdataRowHasOfficialGrade(row) {
			count++
		}
	}
	return count
}

// hexdataSpellPairMetricRows 把 hero-json 的 summonerSpellPairs[] 搬成站内 build
// 行（R116-B P0-4）。上游给的是真实技能 ID（实测 spellIds:["4","32"]、
// winRate:0.569432、pickRate:0.907758、deltaWinRate:0.001923），前端既有的
// summonerSpellAsset() 已经能把 ID 解析成图标与中文名，所以这里不新建映射表，
// 只把 ID 与上游给的中文名一起塞进 championAsset。
// 单位约定与 hexdataItemMetricRows 完全一致（A 已有测试钉住）：
// winRate/pickRate ×100，deltaWinRate 与 wilsonLowerWinRate 保持上游 0..1 原值。
func hexdataSpellPairMetricRows(rows []hexdataSpellPairRow) []championMetricRow {
	result := make([]championMetricRow, 0, len(rows))
	for _, row := range rows {
		if len(row.SpellIDs) == 0 {
			continue
		}
		assets := make([]championAsset, 0, len(row.SpellIDs))
		usable := true
		for index, id := range row.SpellIDs {
			// ID 非法的组合整条丢弃：0 不是合法技能 ID，塞进去前端会渲染成
			// 一个错误的图标，比少一行更糟。
			if id <= 0 {
				usable = false
				break
			}
			name := ""
			if index < len(row.SpellNames) {
				name = row.SpellNames[index]
			}
			assets = append(assets, championAsset{ID: id, Kind: "spell", Name: name, Source: "hexdata"})
		}
		if !usable {
			continue
		}
		result = append(result, championMetricRow{
			Assets:             assets,
			WinRate:            row.WinRate * 100,
			PickRate:           row.PickRate * 100,
			Games:              row.Games,
			DeltaWinRate:       row.DeltaWinRate,
			WilsonLowerWinRate: row.WilsonLowerWinRate,
			OfficialTier:       row.Tier,
		})
	}
	return result
}

// applyMayhemSummonerSpellPairs 是 P0-4 的数据源切换点：召唤师技能改用 Hexdata
// hero-json 的 summonerSpellPairs（同时带胜率与选用率）。OP.GG RSC 那份只有
// 选用率，从此退化成兜底。
// 上游波动 / 字段改名导致 summonerSpellPairs 为空时，保留已有的 OP.GG
// pickRate-only 行：不整块清空、不用 0 胜率顶替（0 会被前端渲染成「0.0%」，
// 那是显示不存在的数据）。
func (p *championProvider) applyMayhemSummonerSpellPairs(response *championDetailResponse, primary hexdataHeroDetailV2, primaryErr error, snapshot hexdataMetaSnapshot) {
	if primaryErr != nil {
		return
	}
	pairs := hexdataSpellPairMetricRows(primary.SummonerSpellPairs)
	if len(pairs) == 0 {
		if p.diag != nil {
			p.diag(map[string]any{"event": "hexdata_spell_pairs_unavailable", "rawRows": len(primary.SummonerSpellPairs), "opggFallbackRows": len(response.Build.SummonerSpells)})
		}
		return
	}
	response.Build.SummonerSpells = pairs
	applyHexdataSampleTiers(response.Build.SummonerSpells, snapshot)
	// 口径必须与实际数据来源一致：开局配置现在是混合来源，说清楚哪块来自谁。
	response.MeasurementTechnique = appendMeasurementTechnique(response.MeasurementTechnique,
		"召唤师技能的胜率与选用率来自 Hexdata 英雄数据，出门装与鞋子仍来自 OP.GG（只有选用率，没有胜率）")
	if p.diag != nil {
		p.diag(map[string]any{"event": "hexdata_spell_pairs", "rows": len(pairs)})
	}
}

func mergeMayhemAugmentRows(primary, fallback []championMetricRow, limit int) []championMetricRow {
	capacity := len(primary) + len(fallback)
	if limit > 0 {
		capacity = min(limit, capacity)
	}
	result := make([]championMetricRow, 0, capacity)
	seen := make(map[int]bool, len(primary)+len(fallback))
	for _, rows := range [][]championMetricRow{primary, fallback} {
		for _, row := range rows {
			if len(row.Assets) == 0 || row.Assets[0].ID <= 0 || seen[row.Assets[0].ID] {
				continue
			}
			seen[row.Assets[0].ID] = true
			result = append(result, row)
			if limit > 0 && len(result) == limit {
				return result
			}
		}
	}
	return result
}

type mayhemRSCDetail struct {
	Build             championBuildSections
	Augments          []championMetricRow
	Citation          championSourceCitation
	FetchedAt         time.Time
	RejectedCoreItems []rscRejectedItem
}

type rscRejectedItem struct {
	ID     int
	Reason string
}

func (p *championProvider) reportRSCRejectedItems(rows []rscRejectedItem) {
	if p.diag == nil {
		return
	}
	for _, row := range rows {
		p.diag(map[string]any{"event": "rsc_core_item_rejected", "id": row.ID, "reason": row.Reason})
	}
}

func (p *championProvider) loadMayhemRSC(ctx context.Context, slug string) (mayhemRSCDetail, error) {
	requestPath := "/lol/modes/aram-mayhem/" + slug + "/build"
	key := strings.Join([]string{"v2", "opgg-rsc", "aram-mayhem", slug}, "|")
	loader := func(loadCtx context.Context) ([]byte, error) { return p.fetchOPGGRSCDirect(loadCtx, requestPath) }
	var data []byte
	var fetchedAt time.Time
	var err error
	if p.cache != nil {
		result, loadErr := p.cache.loadWithStatus(ctx, key, 6*time.Hour, 24*time.Hour, true, loader)
		data, fetchedAt, err = result.data, result.fetchedAt, loadErr
	} else {
		data, err = loader(ctx)
		fetchedAt = time.Now()
	}
	if err != nil {
		return mayhemRSCDetail{}, err
	}
	result, err := parseMayhemRSC(data, slug)
	if err != nil {
		return mayhemRSCDetail{}, err
	}
	result.Citation.Source, result.Citation.CanonicalURL = "OP.GG RSC", "https://op.gg"+requestPath
	result.FetchedAt = fetchedAt
	p.reportRSCRejectedItems(result.RejectedCoreItems)
	descriptions, descriptionErr := p.loadStaticDescriptions(ctx)
	if descriptionErr != nil {
		return mayhemRSCDetail{}, fmt.Errorf("load Data Dragon item catalog: %w", descriptionErr)
	}
	validItemIDs := make(map[int]bool)
	for key := range descriptions {
		if !strings.HasPrefix(key, "item/") {
			continue
		}
		id, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(key, "item/"), filepath.Ext(key)))
		if id > 0 {
			validItemIDs[id] = true
		}
	}
	filteredRows := result.Build.CoreItems[:0]
	for _, row := range result.Build.CoreItems {
		assets := row.Assets[:0]
		for _, asset := range row.Assets {
			if asset.Kind == "item" && validItemIDs[asset.ID] {
				assets = append(assets, asset)
				continue
			}
			if p.diag != nil {
				p.diag(map[string]any{"event": "rsc_core_item_rejected", "id": asset.ID, "reason": "not_in_ddragon_item_catalog"})
			}
		}
		row.Assets = assets
		if len(row.Assets) > 0 {
			filteredRows = append(filteredRows, row)
		}
	}
	result.Build.CoreItems = filteredRows
	if len(result.Build.CoreItems) == 0 {
		return mayhemRSCDetail{}, errors.New("OP.GG mayhem RSC core items are invalid")
	}
	for _, section := range []*[]championMetricRow{&result.Build.StarterItems, &result.Build.Boots, &result.Build.CoreItems, &result.Build.SummonerSpells} {
		for rowIndex := range *section {
			for assetIndex := range (*section)[rowIndex].Assets {
				asset := &(*section)[rowIndex].Assets[assetIndex]
				if asset.Path == "" {
					asset.Path = p.ddragonAssetPath(asset.Kind, asset.ID)
				}
			}
		}
	}
	return result, nil
}

func (p *championProvider) fetchOPGGRSCDirect(ctx context.Context, requestPath string) ([]byte, error) {
	requestContext, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, "https://"+opggPageHost+requestPath, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "text/x-component")
	request.Header.Set("RSC", "1")
	request.Header.Set("User-Agent", "DeepLegends/"+version)
	p.clientMu.RLock()
	client := p.client
	p.clientMu.RUnlock()
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("champion provider returned HTTP %d", response.StatusCode)
	}
	return readLimited(response.Body, championHTMLMax)
}

func expandRSCReferences(source string) string {
	lines := make(map[string]string)
	for _, match := range opggRSCLinePattern.FindAllStringSubmatch(source, -1) {
		// RSC uses the same $Lxx syntax for delayed JSON values and imported
		// component descriptors. Only delayed JSON rows belong in build data.
		if strings.HasPrefix(match[2], "[") {
			lines[match[1]] = match[2]
		}
	}
	result := source
	for range 4 {
		changed := false
		result = opggRSCReferencePattern.ReplaceAllStringFunc(result, func(reference string) string {
			match := opggRSCReferencePattern.FindStringSubmatch(reference)
			if len(match) != 2 {
				return reference
			}
			if line := lines[match[1]]; line != "" {
				changed = true
				return line
			}
			return reference
		})
		if !changed {
			break
		}
	}
	return result
}

func rscRowSegments(source, prefix string, limit int) []string {
	pattern := regexp.MustCompile(`"(` + regexp.QuoteMeta(prefix) + `_[0-9]+)"`)
	indexes := pattern.FindAllStringSubmatchIndex(source, -1)
	// A Next.js RSC response can expose the same logical row more than once:
	// an expanded mobile-table reference and the complete desktop-table row.
	// The old code counted occurrences, so a duplicate item_0 consumed one of
	// the five slots and item_4 was never inspected. Keep one segment per row
	// key and prefer the compact, self-contained occurrence.
	order := make([]string, 0, min(limit, len(indexes)))
	segments := make(map[string]string, min(limit, len(indexes)))
	for index, bounds := range indexes {
		end := len(source)
		if index+1 < len(indexes) {
			end = indexes[index+1][0]
		}
		if nextRow := strings.Index(source[bounds[1]:end], `["$","tr"`); nextRow >= 0 {
			end = bounds[1] + nextRow
		}
		key := source[bounds[2]:bounds[3]]
		segment := source[bounds[0]:end]
		current, exists := segments[key]
		if !exists {
			if limit > 0 && len(order) >= limit {
				continue
			}
			order = append(order, key)
		}
		if !exists || len(segment) < len(current) {
			segments[key] = segment
		}
	}
	result := make([]string, 0, len(order))
	for _, key := range order {
		result = append(result, segments[key])
	}
	return result
}

func rscRows(source, prefix string, limit int) [][]int {
	segments := rscRowSegments(source, prefix, limit)
	result := make([][]int, 0, len(segments))
	for _, segment := range segments {
		seen := make(map[int]bool)
		ids := make([]int, 0, 6)
		for _, match := range opggRSCMetaIDPattern.FindAllStringSubmatch(segment, -1) {
			id, _ := strconv.Atoi(match[1])
			if id > 0 && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			result = append(result, ids)
		}
	}
	return result
}

func rscCoreItemRows(source string, limit int) ([][]int, []rscRejectedItem) {
	segments := rscRowSegments(source, "core_items", limit)
	rows := make([][]int, 0, len(segments))
	rejected := make([]rscRejectedItem, 0)
	for _, segment := range segments {
		kinds := make(map[int]string)
		for _, match := range opggRSCMetaKindIDPattern.FindAllStringSubmatch(segment, -1) {
			id, _ := strconv.Atoi(match[2])
			kinds[id] = strings.ToLower(match[1])
		}
		for _, match := range opggRSCMetaIDKindPattern.FindAllStringSubmatch(segment, -1) {
			id, _ := strconv.Atoi(match[1])
			kinds[id] = strings.ToLower(match[2])
		}
		seen := make(map[int]bool)
		ids := make([]int, 0, 6)
		for _, match := range opggRSCMetaIDPattern.FindAllStringSubmatch(segment, -1) {
			id, _ := strconv.Atoi(match[1])
			if seen[id] {
				continue
			}
			seen[id] = true
			reason := ""
			kind := kinds[id]
			if kind == "" {
				reason = "missing_meta_type"
			} else if kind != "item" {
				reason = "kind_" + kind
			}
			if reason != "" {
				rejected = append(rejected, rscRejectedItem{ID: id, Reason: reason})
				continue
			}
			ids = append(ids, id)
		}
		if len(ids) > 0 {
			rows = append(rows, ids)
		}
	}
	return rows, rejected
}

func metricRowsFromIDs(values [][]int, kind string) []championMetricRow {
	rows := make([]championMetricRow, 0, len(values))
	for _, ids := range values {
		assets := make([]championAsset, 0, len(ids))
		for _, id := range ids {
			assets = append(assets, championAsset{ID: id, Kind: kind, Name: strconv.Itoa(id), Source: "ddragon"})
		}
		rows = append(rows, championMetricRow{Assets: assets})
	}
	return rows
}

func rscAssetMetadata(source string, id int, kind string) championAsset {
	asset := championAsset{ID: id, Kind: kind, Name: strconv.Itoa(id), Source: "ddragon"}
	needle := fmt.Sprintf(`"metaId":%d`, id)
	start := 0
	for {
		index := strings.Index(source[start:], needle)
		if index < 0 {
			break
		}
		index += start
		left, right := max(0, index-700), min(len(source), index+900)
		window := source[left:right]
		if match := opggRSCMetaNamePattern.FindStringSubmatch(window); len(match) == 2 && strings.TrimSpace(match[1]) != "" {
			asset.Name = match[1]
		}
		if match := opggRSCMetaIconPattern.FindStringSubmatch(window); len(match) == 2 {
			iconURL := strings.ReplaceAll(match[1], `\/`, "/")
			if parsed, err := url.Parse(iconURL); err == nil && parsed.Host == opggAssetHost && strings.HasPrefix(parsed.Path, "/meta/images/") {
				asset.Source, asset.Path = "opgg", parsed.Path
			}
		}
		break
	}
	return asset
}

func rscAssetRarity(source string, id int) string {
	needle := fmt.Sprintf(`"metaId":%d`, id)
	index := strings.Index(source, needle)
	if index < 0 {
		return "unknown"
	}
	window := strings.ToLower(source[max(0, index-700):min(len(source), index+900)])
	switch {
	case strings.Contains(window, "prismatic") || strings.Contains(window, "棱彩"):
		return "prismatic"
	case strings.Contains(window, "gold") || strings.Contains(window, "黄金"):
		return "gold"
	case strings.Contains(window, "silver") || strings.Contains(window, "白银"):
		return "silver"
	default:
		return "unknown"
	}
}

func rscSpellRows(source string) [][]int {
	result := make([][]int, 0, 2)
	for _, match := range opggRSCSpellPairPattern.FindAllStringSubmatch(source, -1) {
		if len(match) != 2 {
			continue
		}
		ids := make([]int, 0, 2)
		seen := make(map[int]bool)
		for _, idMatch := range opggRSCSpellMetaPattern.FindAllStringSubmatch(match[1], -1) {
			value := idMatch[1]
			if value == "" {
				value = idMatch[2]
			}
			id, _ := strconv.Atoi(value)
			if id > 0 && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
			if len(ids) == 2 {
				break
			}
		}
		if len(ids) == 2 {
			result = append(result, ids)
		}
		if len(result) == 2 {
			break
		}
	}
	return result
}

func parseMayhemRSC(data []byte, slug string) (mayhemRSCDetail, error) {
	expanded := expandRSCReferences(decodeNextFlight(data))
	if expanded == "" {
		expanded = expandRSCReferences(string(data))
	}
	result := mayhemRSCDetail{}
	if match := opggRSCVersionPattern.FindStringSubmatch(expanded); len(match) == 2 {
		result.Citation.Patch = match[1]
	}
	result.Build.StarterItems = metricRowsFromIDs(rscRows(expanded, "starter_items", 2), "item")
	result.Build.Boots = metricRowsFromIDs(rscRows(expanded, "boots", 2), "item")
	coreItems, rejectedCoreItems := rscCoreItemRows(expanded, 5)
	result.Build.CoreItems = metricRowsFromIDs(coreItems, "item")
	result.RejectedCoreItems = rejectedCoreItems
	result.Build.SummonerSpells = metricRowsFromIDs(rscSpellRows(expanded), "spell")
	priority := make([]string, 0, 3)
	for _, match := range opggRSCSkillPattern.FindAllStringSubmatch(expanded, -1) {
		if len(match) == 2 && !containsString(priority, match[1]) {
			priority = append(priority, match[1])
		}
		if len(priority) == 3 {
			break
		}
	}
	allOrder := opggRSCSkillOrderPattern.FindAllStringSubmatch(expanded, -1)
	if len(allOrder) > 15 {
		allOrder = allOrder[len(allOrder)-15:]
	}
	order := make([]string, 0, 15)
	for _, match := range allOrder {
		if len(match) == 2 {
			order = append(order, match[1])
		}
		if len(order) == 15 {
			break
		}
	}
	if len(priority) > 0 {
		assets := make([]championAsset, len(priority))
		for index, key := range priority {
			assets[index] = championAsset{Kind: "ability", Name: key}
		}
		result.Build.Skills = []championMetricRow{{Assets: assets, SkillPriority: priority, SkillOrder: order}}
	}
	seen := make(map[int]bool)
	for _, match := range opggRSCAugmentIDPattern.FindAllStringSubmatch(expanded, -1) {
		id, _ := strconv.Atoi(match[1])
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		asset := rscAssetMetadata(expanded, id, "augment")
		result.Augments = append(result.Augments, championMetricRow{Assets: []championAsset{asset}, Rarity: rscAssetRarity(expanded, id)})
	}
	if len(result.Build.CoreItems) == 0 || len(result.Build.StarterItems) == 0 || len(result.Build.Boots) == 0 || len(result.Build.Skills) == 0 || len(result.Build.SummonerSpells) == 0 {
		return result, errors.New("OP.GG mayhem RSC is incomplete")
	}
	return result, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
