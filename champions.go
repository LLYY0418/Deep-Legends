package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	stdhtml "html"
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
	opggChampionHost    = "lol-api-champion.op.gg"
	opggWebAPIHost      = "lol-web-api.op.gg"
	opggPageHost        = "op.gg"
	communityDragonHost = "raw.communitydragon.org"
	opggAssetHost       = "opgg-static.akamaized.net"
	dataDragonHost      = "ddragon.leagueoflegends.com"
	championJSONMax     = 6 << 20
	championHTMLMax     = 5 << 20
	championImageMax    = 4 << 20

	gtimgChampionIconPrefix = "/images/lol/act/img/champion/"
	gtimgSkinArtworkPrefix  = "/images/lol/act/img/skin/big"
)

var (
	championSlugPattern  = regexp.MustCompile(`^[a-z0-9]+$`)
	championPatchPattern = regexp.MustCompile(`版本\s*([0-9]+(?:\.[0-9]+){1,2})`)
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
	clientMu     sync.RWMutex
	client       *http.Client
	network      championNetworkSettings
	activeProxy  string
	cache        *championDataCache
	diag         func(event map[string]any)
	mu           sync.Mutex
	patch        string
	static       map[string]championAssetDescription
	championKeys map[string]string
	championIDs  map[string]int
	championMeta map[int]championMetadata
	abilities    map[string]map[string]championAssetDescription
	diagLast     map[string]time.Time
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
	Mode                string                 `json:"mode"`
	Region              string                 `json:"region"`
	Tier                string                 `json:"tier,omitempty"`
	Position            string                 `json:"position,omitempty"`
	Patch               string                 `json:"patch,omitempty"`
	Source              string                 `json:"source"`
	FetchedAt           time.Time              `json:"fetchedAt"`
	EntertainmentSample bool                   `json:"entertainmentSample,omitempty"`
	Rows                []championRankingRow   `json:"rows"`
	TeamCompositions    []arenaTeamComposition `json:"teamCompositions,omitempty"`
}

type championRankingRow struct {
	ChampionID  int      `json:"championId"`
	Key         string   `json:"key,omitempty"`
	Name        string   `json:"name,omitempty"`
	ImageSource string   `json:"imageSource,omitempty"`
	ImagePath   string   `json:"imagePath,omitempty"`
	Rank        int      `json:"rank"`
	Tier        int      `json:"tier"`
	Position    string   `json:"position,omitempty"`
	Positions   []string `json:"positions,omitempty"`
	Play        int      `json:"play,omitempty"`
	WinRate     float64  `json:"winRate,omitempty"`
	PickRate    float64  `json:"pickRate,omitempty"`
	BanRate     float64  `json:"banRate,omitempty"`
	KDA         float64  `json:"kda,omitempty"`
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
	AveragePlacement float64 `json:"averagePlacement,omitempty"`
	FirstPlaceRate   float64 `json:"firstPlaceRate,omitempty"`
	PickRate         float64 `json:"pickRate,omitempty"`
	WinRate          float64 `json:"winRate,omitempty"`
	BanRate          float64 `json:"banRate,omitempty"`
}

type championAsset struct {
	ID          int       `json:"id,omitempty"`
	Kind        string    `json:"kind"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Source      string    `json:"source"`
	Path        string    `json:"path"`
	Active      bool      `json:"active,omitempty"`
	CostType    string    `json:"costType,omitempty"`
	Costs       []float64 `json:"costs,omitempty"`
	Cooldowns   []float64 `json:"cooldowns,omitempty"`
	Ranges      []float64 `json:"ranges,omitempty"`
}

type championAssetDescription struct {
	Name        string
	Description string
	Source      string
	Path        string
	CostType    string
	Costs       []float64
	Cooldowns   []float64
	Ranges      []float64
}

type championMetricRow struct {
	Assets        []championAsset `json:"assets"`
	Ultimate      *championAsset  `json:"ultimate,omitempty"`
	PickRate      float64         `json:"pickRate,omitempty"`
	WinRate       float64         `json:"winRate,omitempty"`
	Games         int             `json:"games,omitempty"`
	SkillPriority []string        `json:"skillPriority,omitempty"`
	SkillOrder    []string        `json:"skillOrder,omitempty"`
}

type championBuildSections struct {
	SummonerSpells []championMetricRow `json:"summonerSpells,omitempty"`
	Skills         []championMetricRow `json:"skills,omitempty"`
	StarterItems   []championMetricRow `json:"starterItems,omitempty"`
	Boots          []championMetricRow `json:"boots,omitempty"`
	CoreItems      []championMetricRow `json:"coreItems,omitempty"`
	FourthItems    []championMetricRow `json:"fourthItems,omitempty"`
	FifthItems     []championMetricRow `json:"fifthItems,omitempty"`
	SixthItems     []championMetricRow `json:"sixthItems,omitempty"`
	PrismItems     []championMetricRow `json:"prismItems,omitempty"`
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
	WinRate  float64 `json:"winRate,omitempty"`
	PickRate float64 `json:"pickRate,omitempty"`
	BanRate  float64 `json:"banRate,omitempty"`
}

type championDetailResponse struct {
	Mode                string                  `json:"mode"`
	Region              string                  `json:"region"`
	Tier                string                  `json:"tier,omitempty"`
	Position            string                  `json:"position,omitempty"`
	Patch               string                  `json:"patch,omitempty"`
	Source              string                  `json:"source"`
	FetchedAt           time.Time               `json:"fetchedAt"`
	EntertainmentSample bool                    `json:"entertainmentSample,omitempty"`
	Stats               championDetailStats     `json:"stats,omitempty"`
	Runes               []championRunePage      `json:"runes,omitempty"`
	Counters            championCounterSections `json:"counters,omitempty"`
	// SampleTier / CountersTier：所选段位样本不足时实际使用的回退段位。
	SampleTier          string                 `json:"sampleTier,omitempty"`
	CountersTier        string                 `json:"countersTier,omitempty"`
	TopPlayers          []championTopPlayer    `json:"topPlayers,omitempty"`
	RecommendedAugments []championAsset        `json:"recommendedAugments,omitempty"`
	ArenaStats          arenaChampionStats     `json:"arenaStats,omitempty"`
	TeamCompositions    []arenaTeamComposition `json:"teamCompositions,omitempty"`
	ArenaAugments       []championMetricRow    `json:"arenaAugments,omitempty"`
	Build               championBuildSections  `json:"build"`
}

type championAugmentResponse struct {
	Source              string            `json:"source"`
	Mode                string            `json:"mode"`
	FetchedAt           time.Time         `json:"fetchedAt"`
	EntertainmentSample bool              `json:"entertainmentSample"`
	Rows                []championAugment `json:"rows"`
}

type championAugment struct {
	ID          int                       `json:"id"`
	Key         string                    `json:"key"`
	Name        string                    `json:"name"`
	Tier        int                       `json:"tier"`
	Rarity      string                    `json:"rarity"`
	Performance float64                   `json:"performance,omitempty"`
	Popularity  float64                   `json:"popularity,omitempty"`
	Description string                    `json:"description"`
	Tooltip     string                    `json:"tooltip,omitempty"`
	ImageSource string                    `json:"imageSource"`
	ImagePath   string                    `json:"imagePath"`
	Champions   []championAugmentChampion `json:"champions,omitempty"`
}

type championAugmentChampion struct {
	ID          int     `json:"id"`
	Name        string  `json:"name,omitempty"`
	Key         string  `json:"key,omitempty"`
	ImageSource string  `json:"imageSource,omitempty"`
	ImagePath   string  `json:"imagePath,omitempty"`
	Performance float64 `json:"performance,omitempty"`
	Popularity  float64 `json:"popularity,omitempty"`
}

func newChampionProvider() *championProvider {
	provider := &championProvider{
		static: make(map[string]championAssetDescription), championKeys: make(map[string]string), championIDs: make(map[string]int), championMeta: make(map[int]championMetadata),
		abilities: make(map[string]map[string]championAssetDescription),
	}
	if err := provider.setNetworkSettings(defaultChampionNetworkSettings()); err != nil {
		panic(err)
	}
	return provider
}

func newChampionHTTPClient(proxy func(*http.Request) (*url.URL, error)) *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:           proxy,
		DialContext:     dialer.DialContext,
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
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
	ttl, staleFor := championCachePolicy(host, requestPath, accept)
	if ttl <= 0 || p.cache == nil {
		return p.fetchDirect(ctx, host, requestPath, query, maxBytes, accept)
	}
	key := championCacheKey(host, requestPath, query.Encode(), accept)
	return p.cache.load(ctx, key, ttl, staleFor, func(loadContext context.Context) ([]byte, error) {
		return p.fetchDirect(loadContext, host, requestPath, query, maxBytes, accept)
	})
}

func (p *championProvider) fetchDirect(ctx context.Context, host, requestPath string, query url.Values, maxBytes int64, accept string) ([]byte, error) {
	if !allowedChampionHost(host) || !strings.HasPrefix(requestPath, "/") || strings.Contains(requestPath, "\\") {
		return nil, errors.New("champion provider request rejected")
	}
	cleaned := pathpkg.Clean(requestPath)
	if cleaned != requestPath || strings.Contains(cleaned, "..") {
		return nil, errors.New("champion provider path rejected")
	}
	u := url.URL{Scheme: "https", Host: host, Path: requestPath, RawQuery: query.Encode()}
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
	return host == opggChampionHost || host == opggWebAPIHost || host == opggPageHost || host == opggAssetHost || host == dataDragonHost || host == prestigeArtworkHost || host == communityDragonHost
}

func (a *app) handleChampionCatalog(w http.ResponseWriter, r *http.Request) {
	response, err := a.championDataProvider().loadCatalog(r.Context())
	writeChampionResponse(w, response, err)
}

func (a *app) handleChampionRankings(w http.ResponseWriter, r *http.Request) {
	mode := strings.TrimSpace(r.URL.Query().Get("mode"))
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
	case "aram-mayhem":
		response, err = provider.loadARAMRankings(r.Context())
	case "arena":
		response, err = provider.loadArenaRankings(r.Context())
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

func (a *app) handleChampionDetail(w http.ResponseWriter, r *http.Request) {
	mode := strings.TrimSpace(r.URL.Query().Get("mode"))
	champion := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("champion")))
	position := strings.TrimSpace(r.URL.Query().Get("position"))
	tier := strings.TrimSpace(r.URL.Query().Get("tier"))
	if !championSlugPattern.MatchString(champion) {
		http.Error(w, "invalid champion", http.StatusBadRequest)
		return
	}
	if mode == "ranked" {
		if !allowedChampionTiers[tier] || position == "all" || championPositionNames[position] == "" {
			http.Error(w, "invalid champion detail filter", http.StatusBadRequest)
			return
		}
	} else if mode != "aram-mayhem" && mode != "arena" {
		http.Error(w, "invalid champion mode", http.StatusBadRequest)
		return
	}
	response, err := a.championDataProvider().loadDetail(r.Context(), mode, champion, position, tier)
	writeChampionResponse(w, response, err)
}

func (a *app) handleChampionAsset(w http.ResponseWriter, r *http.Request) {
	source := strings.TrimSpace(r.URL.Query().Get("source"))
	requestPath := strings.TrimSpace(r.URL.Query().Get("path"))
	host, ok := validateChampionAssetPath(source, requestPath)
	if !ok {
		http.Error(w, "invalid champion asset", http.StatusBadRequest)
		return
	}
	const acceptImages = "image/avif,image/webp,image/png,image/jpeg,image/*;q=0.8"
	provider := a.championDataProvider()
	if data, ok := a.loadChampionAssetFromClient(r.Context(), provider, source, requestPath); ok {
		writeChampionAssetImage(w, data)
		return
	}
	data, err := provider.fetch(r.Context(), host, requestPath, nil, championImageMax, acceptImages)
	if err != nil {
		if fallbackHost, fallbackPath, ok := provider.championAssetFallback(source, requestPath); ok {
			data, err = provider.fetch(r.Context(), fallbackHost, fallbackPath, nil, championImageMax, acceptImages)
		}
	}
	if err != nil || !writeChampionAssetImage(w, data) {
		http.NotFound(w, r)
	}
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
	lcuPath, ok := provider.championAssetLCUPath(source, requestPath)
	if !ok {
		return nil, false
	}
	a.mu.RLock()
	client := a.lcu
	connected := a.connected
	a.mu.RUnlock()
	if client == nil || !connected {
		return nil, false
	}
	data, err := a.loadAsset(ctx, lcuPath, championImageMax, 0, func(loadContext context.Context) ([]byte, error) {
		return client.GetBytesContext(loadContext, lcuPath)
	})
	if err != nil || !strings.HasPrefix(http.DetectContentType(data), "image/") {
		return nil, false
	}
	return data, true
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
	Data map[string]struct {
		Key         string    `json:"key"`
		Name        string    `json:"name"`
		Description string    `json:"description"`
		Tooltip     string    `json:"tooltip"`
		Cooldown    []float64 `json:"cooldown"`
		Cost        []float64 `json:"cost"`
		Range       []float64 `json:"range"`
		Image       struct {
			Full string `json:"full"`
		} `json:"image"`
	} `json:"data"`
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
		for id, item := range catalog.Data {
			if item.Image.Full == "" {
				continue
			}
			descriptions[endpoint.kind+"/"+strings.ToLower(item.Image.Full)] = championAssetDescription{
				Name: item.Name, Description: cleanMarkup(firstNonEmpty(item.Description, item.Tooltip)),
				Source: "ddragon", Path: "/cdn/" + patch + "/img/" + endpoint.kind + "/" + item.Image.Full,
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
	p.mu.Unlock()
	return descriptions, nil
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
	KDA        float64 `json:"kda"`
	Tier       int     `json:"tier"`
	Rank       int     `json:"rank"`
	TierData   struct {
		Tier int `json:"tier"`
		Rank int `json:"rank"`
	} `json:"tier_data"`
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
	wanted := championPositionNames[position]
	rows := make([]championRankingRow, 0, len(raw.Data))
	for _, item := range raw.Data {
		positions := make([]string, 0, len(item.Positions))
		for _, candidate := range item.Positions {
			positions = append(positions, strings.ToLower(candidate.Name))
		}
		stats := item.AverageStats
		rowPosition := ""
		if wanted == "" {
			if len(item.Positions) == 0 {
				continue
			}
			rowPosition = strings.ToLower(item.Positions[0].Name)
		} else {
			found := false
			for _, candidate := range item.Positions {
				if candidate.Name == wanted {
					stats, rowPosition, found = candidate.Stats, strings.ToLower(candidate.Name), true
					break
				}
			}
			if !found {
				continue
			}
		}
		tierValue, rankValue := stats.TierData.Tier, stats.TierData.Rank
		if tierValue == 0 && stats.Tier > 0 {
			tierValue = stats.Tier
		}
		if rankValue == 0 {
			rankValue = stats.Rank
		}
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
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Rank < rows[j].Rank })
	return championRankingResponse{Mode: "aram-mayhem", Region: "KR", Patch: payload.Meta.Version, Source: "OP.GG JSON", FetchedAt: time.Now(), EntertainmentSample: true, Rows: rows}, nil
}

func (p *championProvider) loadArenaPage(ctx context.Context) ([]byte, string, error) {
	data, err := p.fetch(ctx, opggPageHost, "/zh-cn/lol/modes/arena", nil, championHTMLMax, "text/html,application/xhtml+xml")
	if err != nil {
		return nil, "", err
	}
	decoded := decodeNextFlight(data)
	if decoded == "" {
		return nil, "", errors.New("OP.GG Arena page data changed")
	}
	return data, decoded, nil
}

func (p *championProvider) loadArenaRankings(ctx context.Context) (championRankingResponse, error) {
	data, err := p.fetch(ctx, opggChampionHost, "/api/global/champions/arena", nil, championJSONMax, "application/json")
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
	if json.Unmarshal(data, &payload) != nil || len(payload.Data) < 100 {
		return championRankingResponse{}, errors.New("OP.GG Arena champion response changed")
	}
	rows := make([]championRankingRow, 0, len(payload.Data))
	for _, item := range payload.Data {
		stats := item.AverageStats
		if item.ID == 0 || stats.Rank <= 0 {
			continue
		}
		rows = append(rows, championRankingRow{
			ChampionID: item.ID, Rank: stats.Rank, Tier: stats.Tier,
			Play: stats.Play, WinRate: firstPositive(ratePercent(stats.WinRate), percentOf(stats.Win, stats.Play)), PickRate: ratePercent(stats.PickRate), BanRate: ratePercent(stats.BanRate),
		})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Rank < rows[j].Rank })
	var teams []arenaTeamComposition
	if _, decoded, pageErr := p.loadArenaPage(ctx); pageErr == nil {
		teams = parseArenaTeamCompositions(decoded, `"teamData":`, 40)
	}
	return championRankingResponse{
		Mode: "arena", Region: "GLOBAL", Patch: payload.Meta.Version, Source: "OP.GG JSON", FetchedAt: time.Now(),
		EntertainmentSample: true, Rows: rows, TeamCompositions: teams,
	}, nil
}

func ratePercent(value float64) float64 {
	if value > 0 && value <= 1 {
		return value * 100
	}
	return value
}

func percentOf(value, total int) float64 {
	if value <= 0 || total <= 0 {
		return 0
	}
	return float64(value) / float64(total) * 100
}

func parseArenaTeamCompositions(decoded, marker string, limit int) []arenaTeamComposition {
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
	_, decoded, err := p.loadARAMPage(ctx)
	if err != nil {
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
	if len(rows) < 40 {
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
	return championAugmentResponse{Source: "OP.GG", Mode: "aram-mayhem", FetchedAt: time.Now(), EntertainmentSample: true, Rows: rows}, nil
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
	switch value {
	case 1:
		return "silver"
	case 4:
		return "gold"
	case 8:
		return "prismatic"
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

func (p *championProvider) loadDetailHTML(ctx context.Context, mode, champion, position, tier string) (championDetailResponse, error) {
	requestPath := "/zh-cn/lol/modes/aram-mayhem/" + champion + "/build"
	query := url.Values{}
	if mode == "ranked" {
		requestPath = "/zh-cn/lol/champions/" + champion + "/build/" + position
		query = url.Values{"region": {"kr"}, "type": {"ranked"}, "tier": {tier}}
	} else if mode == "arena" {
		requestPath = "/zh-cn/lol/modes/arena/" + champion + "/build"
	}
	page, err := p.fetch(ctx, opggPageHost, requestPath, query, championHTMLMax, "text/html,application/xhtml+xml")
	if err != nil {
		return championDetailResponse{}, err
	}
	document, err := xhtml.Parse(strings.NewReader(string(page)))
	if err != nil {
		return championDetailResponse{}, errors.New("OP.GG detail page could not be parsed")
	}
	region := "KR"
	if mode == "arena" {
		region = "GLOBAL"
	}
	response := championDetailResponse{Mode: mode, Region: region, Tier: tier, Position: position, Source: "OP.GG", FetchedAt: time.Now(), Build: parseChampionBuild(document)}
	if match := championPatchPattern.FindSubmatch(page); len(match) == 2 {
		response.Patch = string(match[1])
	}
	decoded := decodeNextFlight(page)
	if mode == "ranked" {
		response.Runes = parseChampionRunes(decoded)
		response.Counters = parseChampionCounters(document)
	} else if mode == "aram-mayhem" {
		response.EntertainmentSample = true
		response.RecommendedAugments = parseRecommendedAugments(document)
	} else {
		response.EntertainmentSample = true
		response.ArenaStats = parseArenaStats(decoded)
		response.TeamCompositions = parseArenaTeamCompositions(decoded, `"arenaCombinations":`, 5)
		response.ArenaAugments = parseArenaAugments(decoded)
		response.Build = normalizeArenaBuild(response.Build)
	}
	p.decorateDetailAssets(ctx, champion, &response)
	if len(response.Build.SummonerSpells) == 0 && len(response.Build.CoreItems) == 0 && len(response.Build.PrismItems) == 0 {
		return championDetailResponse{}, errors.New("OP.GG detail data changed")
	}
	return response, nil
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

func parseChampionBuild(document *xhtml.Node) championBuildSections {
	sections := championBuildSections{}
	for _, table := range descendantElements(document, "table") {
		caption := strings.ToLower(nodeText(firstDescendant(table, "caption")))
		heading := nodeText(firstDescendant(table, "th"))
		key := caption + " " + heading
		rows := parseMetricRows(table)
		if len(rows) == 0 {
			continue
		}
		switch {
		case strings.Contains(key, "summonerspells") || strings.Contains(key, "召唤师技能"):
			sections.SummonerSpells = appendUniqueMetricRows(sections.SummonerSpells, rows...)
		case strings.Contains(key, "skillorder") || strings.Contains(key, "技能加点"):
			sections.Skills = appendUniqueMetricRows(sections.Skills, rows...)
		case strings.Contains(key, "depth 4") || strings.Contains(key, "第四件"):
			rows = normalizeLateItemMetrics(rows)
			sections.FourthItems = appendUniqueMetricRows(sections.FourthItems, rows...)
		case strings.Contains(key, "depth 5") || strings.Contains(key, "第五件"):
			rows = normalizeLateItemMetrics(rows)
			sections.FifthItems = appendUniqueMetricRows(sections.FifthItems, rows...)
		case strings.Contains(key, "depth 6") || strings.Contains(key, "第六件"):
			rows = normalizeLateItemMetrics(rows)
			sections.SixthItems = appendUniqueMetricRows(sections.SixthItems, rows...)
		case strings.Contains(key, "boots") || strings.Contains(key, "鞋子"):
			sections.Boots = appendUniqueMetricRows(sections.Boots, rows...)
		case strings.Contains(key, "prismatic") || strings.Contains(key, "棱镜装备") || strings.Contains(key, "棱彩装备"):
			sections.PrismItems = appendUniqueMetricRows(sections.PrismItems, rows...)
		case strings.Contains(key, "builds") || strings.Contains(key, "核心装备"):
			sections.CoreItems = appendUniqueMetricRows(sections.CoreItems, rows...)
		case strings.Contains(key, "items") || strings.Contains(key, "出门装"):
			sections.StarterItems = appendUniqueMetricRows(sections.StarterItems, rows...)
		}
	}
	return sections
}

func parseChampionCounters(document *xhtml.Node) championCounterSections {
	return championCounterSections{
		WeakAgainst:   parseChampionCounterGroup(document, "对线劣势的英雄"),
		StrongAgainst: parseChampionCounterGroup(document, "强烈对抗"),
	}
}

func parseChampionCounterGroup(document *xhtml.Node, label string) []championCounterRow {
	var heading *xhtml.Node
	for _, node := range descendantElements(document, "div") {
		if strings.TrimSpace(nodeText(node)) == label {
			heading = node
			break
		}
	}
	if heading == nil {
		return nil
	}
	var listContainer *xhtml.Node
	for container, depth := heading, 0; container != nil && depth < 5; container, depth = container.Parent, depth+1 {
		candidate := nextElementSibling(container)
		if len(descendantElements(candidate, "li")) > 0 {
			listContainer = candidate
			break
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

func normalizeLateItemMetrics(rows []championMetricRow) []championMetricRow {
	for index := range rows {
		if rows[index].WinRate == 0 && rows[index].PickRate > 0 {
			rows[index].WinRate, rows[index].PickRate = rows[index].PickRate, 0
		}
	}
	return rows
}

func parseMetricRows(table *xhtml.Node) []championMetricRow {
	rows := make([]championMetricRow, 0)
	for _, body := range descendantElements(table, "tbody") {
		for _, rowNode := range directElements(body, "tr") {
			cells := directElements(rowNode, "td")
			if len(cells) == 0 {
				continue
			}
			row := championMetricRow{}
			for _, image := range descendantElements(cells[0], "img") {
				asset, ok := remoteAsset(attribute(image, "src"), attribute(image, "alt"))
				if ok {
					row.Assets = append(row.Assets, asset)
				}
			}
			if len(cells) > 1 {
				row.PickRate = firstNumber(nodeText(cells[1]))
				row.Games = firstGames(nodeText(cells[1]))
			}
			if len(cells) > 2 {
				row.WinRate = firstNumber(nodeText(cells[2]))
			}
			keys := make([]string, 0, 24)
			for _, strong := range descendantElements(cells[0], "strong") {
				value := strings.TrimSpace(nodeText(strong))
				if value == "Q" || value == "W" || value == "E" || value == "R" {
					keys = append(keys, value)
				}
			}
			if len(keys) >= 3 {
				row.SkillPriority = append([]string(nil), keys[:3]...)
				if len(keys) > 3 {
					row.SkillOrder = append([]string(nil), keys[3:]...)
				}
			}
			if len(row.Assets) > 0 || len(row.SkillOrder) > 0 {
				rows = append(rows, row)
			}
		}
	}
	return rows
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
		5001: "StatModsHealthScalingIcon.png",
		5005: "StatModsAttackSpeedIcon.png",
		5007: "StatModsCDRScalingIcon.png",
		5008: "StatModsAdaptiveForceIcon.png",
		5010: "StatModsMovementSpeedIcon.png",
		5011: "StatModsHealthPlusIcon.png",
		5013: "StatModsTenacityIcon.png",
	}[id]
	if !ok {
		return "", false
	}
	return "/cdn/img/perk-images/StatMods/" + name, true
}

func parseRecommendedAugments(document *xhtml.Node) []championAsset {
	result := make([]championAsset, 0, 12)
	seen := make(map[string]bool)
	for _, item := range descendantElements(document, "li") {
		for _, image := range descendantElements(item, "img") {
			source := attribute(image, "src")
			if !strings.Contains(source, "/aram-augment/") && !strings.Contains(source, "/augment/") {
				continue
			}
			asset, ok := remoteAsset(source, attribute(image, "alt"))
			if ok && !seen[asset.Path] {
				seen[asset.Path] = true
				result = append(result, asset)
			}
			break
		}
		if len(result) == 12 {
			break
		}
	}
	return result
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

func patchFromOPGGPath(value string) string {
	marker := "/meta/images/lol/"
	index := strings.Index(value, marker)
	if index < 0 {
		return ""
	}
	rest := value[index+len(marker):]
	if slash := strings.IndexByte(rest, '/'); slash >= 0 {
		return rest[:slash]
	}
	return ""
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
