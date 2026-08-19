package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

var version = "0.11.0"

//go:embed web/* data/reroll_pool_14_5.txt data/reroll_pool_14_5.json
var embedded embed.FS

type app struct {
	mu                   sync.RWMutex
	token                string
	startedAt            time.Time
	lastSync             time.Time
	lastAttempt          time.Time
	lastDuration         time.Duration
	lastError            string
	connected            bool
	snapshotReady        bool
	connectionState      string
	eventStream          bool
	summoner             Summoner
	account              AccountData
	allSkins             []Skin
	chromas              []Chroma
	chromaState          EndpointCapability
	owned                []Skin
	remaining            []Skin
	poolTotal            int
	poolMatched          int
	poolIssues           []PoolIssue
	lcu                  *LCUClient
	syncing              bool
	poolSource           string
	poolVersion          string
	poolID               string
	poolHash             string
	poolGeneration       uint64
	pools                map[string]PoolManifest
	storage              *localStore
	diagnosticLogMu      sync.RWMutex
	diagnosticLogErr     string
	ownership            []OwnershipSourceStatus
	catalog              CatalogStats
	assetCacheMu         sync.RWMutex
	assetCache           map[string][]byte
	assetCacheOrder      []string
	assetCacheBytes      int
	assetCacheGeneration uint64
	assetFlights         map[string]*assetFlight
	assetFailureUntil    map[string]time.Time
	mediaSlots           chan struct{}
	refreshRequests      chan struct{}
	discovery            LCUDiscoveryStatus
	eventMu              sync.Mutex
	eventSubscribers     map[chan string]struct{}
	gameplayRefs         map[string]string
	gameplayRefDetails   map[string]gameplayReference
	riotClientDiscovery  func() (*RiotClientAPI, error)
	itemSetMu            sync.Mutex
	champions            *championProvider
	riot                 *riotProvider
	sgp                  *sgpProvider
	convenience          *convenienceRunner
	lpTracker            *lpTracker
	rankScores           *rankScoreCache
	opgg                 *opggInsights
	matchTimelines       *matchTimelineCache
}

type statusResponse struct {
	Version          string         `json:"version"`
	Connected        bool           `json:"connected"`
	SnapshotReady    bool           `json:"snapshotReady"`
	ConnectionState  string         `json:"connectionState"`
	EventStream      bool           `json:"eventStream"`
	Syncing          bool           `json:"syncing"`
	LastSync         time.Time      `json:"lastSync,omitempty"`
	LastError        string         `json:"lastError,omitempty"`
	LastAttempt      time.Time      `json:"lastAttempt,omitempty"`
	Summoner         publicSummoner `json:"summoner"`
	OwnedCount       int            `json:"ownedCount"`
	ChromaOwnedCount int            `json:"chromaOwnedCount"`
	PoolTotal        int            `json:"poolTotal"`
	PoolMatched      int            `json:"poolMatched"`
	Remaining        int            `json:"remainingCount"`
	CalculationOK    bool           `json:"calculationOK"`
	PoolIssues       []PoolIssue    `json:"poolIssues,omitempty"`
	PoolSource       string         `json:"poolSource"`
	PoolVersion      string         `json:"poolVersion"`
	PoolID           string         `json:"poolId"`
	PoolHash         string         `json:"poolHash"`
	StorageReady     bool           `json:"storageReady"`
	ServerID         string         `json:"serverId,omitempty"`
	ServerName       string         `json:"serverName,omitempty"`
}

type publicSummoner struct {
	DisplayName   string `json:"displayName,omitempty"`
	GameName      string `json:"gameName,omitempty"`
	TagLine       string `json:"tagLine,omitempty"`
	ProfileIconID int64  `json:"profileIconId,omitempty"`
	SummonerLevel int64  `json:"summonerLevel,omitempty"`
}

func main() {
	selfTest := flag.Bool("self-test", false, "validate embedded resources and exit")
	noBrowser := flag.Bool("no-browser", false, "do not open the default browser")
	desktopMode := flag.Bool("desktop", false, "emit a desktop-shell bootstrap event and do not open a browser")
	listenAddress := flag.String("listen", "127.0.0.1:0", "loopback address for the local UI")
	encryptRiotKeyFlag := flag.String("encrypt-riot-key", "", "encrypt a Riot API key for embedding in riot_api.go and exit")
	flag.Parse()
	if strings.TrimSpace(*encryptRiotKeyFlag) != "" {
		cipherText, err := encryptRiotKey(*encryptRiotKeyFlag)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(cipherText)
		return
	}
	poolBytes, err := embedded.ReadFile("data/reroll_pool_14_5.json")
	if err != nil {
		log.Fatal(err)
	}
	var embeddedPool struct {
		Entries []PoolEntry `json:"entries"`
	}
	if err := json.Unmarshal(poolBytes, &embeddedPool); err != nil {
		log.Fatal(err)
	}
	builtInPool, err := validatePoolManifest(PoolManifest{
		ID: "cn-14.5-2024-02-29", Name: "国服 14.5 三合一奖池", Source: "https://lol.qq.com/news/detail.shtml?docid=12008689032502035596",
		Version: "14.5 / 2024-02-29", UpdatedAt: time.Date(2024, 2, 29, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60)), Entries: embeddedPool.Entries, BuiltIn: true,
	})
	if err != nil {
		log.Fatal(err)
	}
	store, storageErr := openLocalStore()
	if storageErr != nil {
		log.Printf("本地历史与自定义奖池不可用：%v", storageErr)
	}
	token, err := loadOrCreateSessionToken(store)
	if err != nil {
		log.Fatal(err)
	}
	pools := map[string]PoolManifest{builtInPool.ID: builtInPool}
	if store != nil {
		for _, pool := range store.loadPools() {
			pools[pool.ID] = pool
		}
	}
	championProvider := newChampionProvider()
	championProvider.cache = newChampionDataCache(store)
	if store != nil {
		championProvider.diag = func(event map[string]any) { _ = store.appendDiagnostic(event) }
	}
	if err := championProvider.setNetworkSettings(loadChampionNetworkSettings(store)); err != nil {
		log.Printf("英雄数据代理设置无效，已使用自动模式：%v", err)
		_ = championProvider.setNetworkSettings(defaultChampionNetworkSettings())
	}
	a := &app{
		token:               token,
		startedAt:           time.Now(),
		poolTotal:           len(builtInPool.Names),
		poolSource:          builtInPool.Source,
		poolVersion:         builtInPool.Version,
		poolID:              builtInPool.ID,
		poolHash:            builtInPool.Hash,
		pools:               pools,
		storage:             store,
		assetCache:          make(map[string][]byte),
		assetFlights:        make(map[string]*assetFlight),
		assetFailureUntil:   make(map[string]time.Time),
		mediaSlots:          make(chan struct{}, 2),
		connectionState:     "connecting",
		refreshRequests:     make(chan struct{}, 1),
		eventSubscribers:    make(map[chan string]struct{}),
		gameplayRefs:        make(map[string]string),
		gameplayRefDetails:  make(map[string]gameplayReference),
		riotClientDiscovery: discoverRiotClient,
		champions:           championProvider,
		riot:                newRiotProvider(championProvider),
		sgp:                 newSGPProvider(),
	}
	a.convenience = newConvenienceRunner(store, a.broadcastEvent)
	a.lpTracker = newLPTracker(store)
	a.rankScores = newRankScoreCache()
	a.opgg = newOPGGInsights()
	a.matchTimelines = newMatchTimelineCache()

	webFS, err := fs.Sub(embedded, "web")
	if err != nil {
		log.Fatal(err)
	}
	if *selfTest {
		log.Printf("Deep Legends %s 自检通过：奖池 %d 条，哈希 %s", version, len(builtInPool.Names), builtInPool.Hash[:12])
		return
	}
	mux := http.NewServeMux()
	fileServer := http.FileServer(http.FS(webFS))
	// 内嵌静态资源没有修改时间等校验信息，强制浏览器每次校验，
	// 确保重新编译后前端样式与脚本立即生效（本机服务，无网络成本）。
	staticFiles := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		fileServer.ServeHTTP(w, r)
	}
	mux.HandleFunc("GET /{$}", a.handleBootstrap(http.HandlerFunc(staticFiles)))
	mux.HandleFunc("GET /", staticFiles)
	mux.HandleFunc("GET /api/status", a.authorized(a.handleStatus))
	mux.HandleFunc("GET /api/events", a.authorized(a.handleEvents))
	mux.HandleFunc("GET /api/skins", a.authorized(a.handleSkins))
	mux.HandleFunc("GET /api/pool-skins", a.authorized(a.handlePoolSkins))
	mux.HandleFunc("GET /api/chromas", a.authorized(a.handleChromas))
	mux.HandleFunc("GET /api/skin-details", a.authorized(a.handleSkinDetails))
	mux.HandleFunc("GET /api/account", a.authorized(a.handleAccount))
	mux.HandleFunc("GET /api/gameplay/overview", a.authorized(a.handleGameplayOverview))
	mux.HandleFunc("POST /api/gameplay/overview", a.authorized(a.handleGameplayOverview))
	mux.HandleFunc("GET /api/gameplay/live", a.authorized(a.handleGameplayLive))
	mux.HandleFunc("GET /api/gameplay/recommendations", a.authorized(a.handleGameplayRecommendations))
	mux.HandleFunc("GET /api/gameplay/specialist-runes", a.authorized(a.handleGameplaySpecialistRunes))
	mux.HandleFunc("POST /api/gameplay/match-tiers", a.authorized(a.handleGameplayMatchTiers))
	mux.HandleFunc("POST /api/gameplay/match-timeline", a.authorized(a.handleGameplayMatchTimeline))
	mux.HandleFunc("GET /api/gameplay/phase", a.authorized(a.handleGameplayPhase))
	mux.HandleFunc("GET /api/gameplay/convenience", a.authorized(a.handleGameplayConvenience))
	mux.HandleFunc("POST /api/gameplay/convenience", a.authorized(a.handleGameplayConvenience))
	mux.HandleFunc("GET /api/gameplay/perks", a.authorized(a.handleGameplayPerks))
	mux.HandleFunc("GET /api/gameplay/items", a.authorized(a.handleGameplayItems))
	mux.HandleFunc("GET /api/gameplay/summoner-spells", a.authorized(a.handleGameplaySummonerSpells))
	mux.HandleFunc("POST /api/gameplay/runes/apply", a.authorized(a.handleGameplayRuneApply))
	mux.HandleFunc("POST /api/gameplay/item-sets/apply", a.authorized(a.handleGameplayItemSetApply))
	mux.HandleFunc("GET /api/gameplay/replay", a.authorized(a.handleGameplayReplayMetadata))
	mux.HandleFunc("POST /api/gameplay/replay", a.authorized(a.handleGameplayReplayAction))
	mux.HandleFunc("GET /api/champions/catalog", a.authorized(a.handleChampionCatalog))
	mux.HandleFunc("GET /api/champions/rankings", a.authorized(a.handleChampionRankings))
	mux.HandleFunc("GET /api/champions/augments", a.authorized(a.handleChampionAugments))
	mux.HandleFunc("GET /api/champions/detail", a.authorized(a.handleChampionDetail))
	mux.HandleFunc("GET /api/champions/network", a.authorized(a.handleChampionNetwork))
	mux.HandleFunc("POST /api/champions/network", a.authorized(a.handleChampionNetwork))
	mux.HandleFunc("GET /api/social/friends", a.authorized(a.handleSocialFriends))
	mux.HandleFunc("POST /api/system-proxy", a.authorized(a.handleSystemProxy))
	mux.HandleFunc("GET /api/champion-asset", a.authorized(a.handleChampionAsset))
	mux.HandleFunc("POST /api/refresh", a.authorized(a.handleRefresh))
	mux.HandleFunc("GET /api/image", a.authorized(a.handleImage))
	mux.HandleFunc("GET /api/prestige-image", a.authorized(a.handlePrestigeImage))
	mux.HandleFunc("GET /api/skin-art", a.authorized(a.handleSkinArt))
	mux.HandleFunc("GET /api/media", a.authorized(a.handleMedia))
	mux.HandleFunc("GET /api/diagnostics", a.authorized(a.handleDiagnostics))
	mux.HandleFunc("GET /api/diagnostics/log", a.authorized(a.handleDiagnosticLog))
	mux.HandleFunc("GET /api/pools", a.authorized(a.handlePools))
	mux.HandleFunc("POST /api/pools/import", a.authorized(a.handlePoolImport))
	mux.HandleFunc("POST /api/pools/select", a.authorized(a.handlePoolSelect))
	mux.HandleFunc("GET /api/export", a.authorized(a.handleExport))
	mux.HandleFunc("GET /api/snapshots", a.authorized(a.handleSnapshots))
	mux.HandleFunc("POST /api/snapshots", a.authorized(a.handleSnapshots))
	mux.HandleFunc("GET /api/snapshots/{id}", a.authorized(a.handleSnapshot))
	mux.HandleFunc("GET /api/snapshots/{id}/export", a.authorized(a.handleSnapshotExport))
	mux.HandleFunc("GET /api/snapshots/{id}/diff", a.authorized(a.handleSnapshotDiff))
	mux.HandleFunc("GET /api/privacy", a.authorized(a.handlePrivacy))
	mux.HandleFunc("GET /api/client-installations", a.authorized(a.handleClientInstallations))
	mux.HandleFunc("POST /api/client-launch", a.authorized(a.handleClientLaunch))
	mux.HandleFunc("POST /api/quit", a.authorized(a.handleQuit))

	listener, err := net.Listen("tcp", *listenAddress)
	if err != nil {
		log.Fatal(err)
	}
	if tcpAddress, ok := listener.Addr().(*net.TCPAddr); !ok || !tcpAddress.IP.IsLoopback() {
		_ = listener.Close()
		log.Fatal("本地界面只能监听回环地址")
	}
	baseAddress := "http://" + listener.Addr().String()
	address := baseAddress + "/?bootstrap=" + url.QueryEscape(token)
	if *desktopMode {
		if readyErr := writeDesktopReady(os.Stdout, baseAddress, address, token); readyErr != nil {
			_ = listener.Close()
			log.Fatal(readyErr)
		}
	}

	go a.runConnectionManager(context.Background())
	if !*noBrowser && !*desktopMode {
		go func() {
			time.Sleep(250 * time.Millisecond)
			if err := openBrowser(address); err != nil {
				log.Printf("浏览器未能自动打开，请访问本地助手首页：%s (%v)", baseAddress, err)
			}
		}()
	}

	server := &http.Server{
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       45 * time.Second,
	}
	log.Printf("Deep Legends %s 正在运行：%s", version, baseAddress)
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func (a *app) authorized(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provided := r.Header.Get("X-Local-Token")
		if cookie, err := r.Cookie("lol_loot_token"); err == nil && provided == "" {
			provided = cookie.Value
		}
		if provided != a.token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (a *app) handleBootstrap(fileServer http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bootstrap := r.URL.Query().Get("bootstrap")
		if bootstrap != "" {
			if bootstrap != a.token {
				http.Error(w, "invalid bootstrap token", http.StatusUnauthorized)
				return
			}
			a.setSessionCookie(w)
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		// 在浏览器地址栏直接打开或刷新首页时自动续发会话 cookie：服务
		// 重启后旧页面刷新一次即可恢复，不必依赖带 bootstrap 的链接。
		// 仅对浏览器的顶级导航发放（Sec-Fetch-* 由浏览器强制设置，页面
		// 脚本无法伪造）；跨站发起的导航与普通 fetch 都拿不到 cookie。
		if cookie, err := r.Cookie("lol_loot_token"); err != nil || cookie.Value != a.token {
			if isTrustedNavigation(r) {
				a.setSessionCookie(w)
			}
		}
		fileServer.ServeHTTP(w, r)
	}
}

func (a *app) setSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "lol_loot_token",
		Value:    a.token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int((365 * 24 * time.Hour).Seconds()),
	})
}

// isTrustedNavigation 判断请求是否为浏览器发起的同站或用户直接输入的
// 顶级文档导航。Sec-Fetch 头由浏览器自身设置，网页脚本无法覆盖，因此
// 跨站页面既不能通过导航拿到 cookie，也不能通过 fetch 读取任何接口。
func isTrustedNavigation(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Mode") != "navigate" || r.Header.Get("Sec-Fetch-Dest") != "document" {
		return false
	}
	site := r.Header.Get("Sec-Fetch-Site")
	return site == "none" || site == "same-origin"
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func (a *app) handleStatus(w http.ResponseWriter, _ *http.Request) {
	a.mu.RLock()
	issues := append([]PoolIssue(nil), a.poolIssues...)
	if len(issues) > 100 {
		issues = issues[:100]
	}
	client := a.lcu
	summoner := publicSummoner{DisplayName: a.summoner.DisplayName, GameName: a.summoner.GameName, TagLine: a.summoner.TagLine, ProfileIconID: a.summoner.ProfileIconID, SummonerLevel: a.summoner.SummonerLevel}
	response := statusResponse{
		Version: version, Connected: a.connected, SnapshotReady: a.snapshotReady, ConnectionState: a.connectionState, EventStream: a.eventStream,
		Syncing: a.syncing, LastSync: a.lastSync, LastAttempt: a.lastAttempt,
		LastError: a.lastError, Summoner: summoner, OwnedCount: len(a.owned), ChromaOwnedCount: ownedChromaCount(a.chromas), PoolTotal: a.poolTotal,
		PoolMatched: a.poolMatched, Remaining: len(a.remaining), CalculationOK: a.calculationOKLocked(),
		PoolIssues: issues, PoolSource: a.poolSource, PoolVersion: a.poolVersion, PoolID: a.poolID,
		PoolHash: a.poolHash, StorageReady: a.storage != nil,
	}
	a.mu.RUnlock()
	if response.Connected {
		response.ServerID = clientTencentServerID(client)
		response.ServerName = tencentServerName(response.ServerID)
	}
	respondJSON(w, response)
}

func (a *app) handleChromas(w http.ResponseWriter, _ *http.Request) {
	a.mu.RLock()
	if !a.connected || !a.snapshotReady {
		a.mu.RUnlock()
		http.Error(w, "当前没有可用的客户端快照", http.StatusConflict)
		return
	}
	items := append([]Chroma(nil), a.chromas...)
	capability := a.chromaState
	a.mu.RUnlock()
	respondJSON(w, map[string]any{"items": items, "count": len(items), "ownedCount": ownedChromaCount(items), "capability": capability})
}

func ownedChromaCount(items []Chroma) int {
	count := 0
	for _, item := range items {
		if item.Owned {
			count++
		}
	}
	return count
}

func (a *app) handleSkins(w http.ResponseWriter, r *http.Request) {
	view := r.URL.Query().Get("view")
	a.mu.RLock()
	if !a.connected || !a.snapshotReady {
		a.mu.RUnlock()
		http.Error(w, "当前没有可用的客户端快照", http.StatusConflict)
		return
	}
	var skins []Skin
	switch view {
	case "owned":
		skins = a.owned
	case "all":
		skins = a.allSkins
	case "remaining":
		if !a.calculationOKLocked() {
			a.mu.RUnlock()
			http.Error(w, "奖池数据尚未完整映射，已停止剩余计算", http.StatusConflict)
			return
		}
		skins = a.remaining
	default:
		a.mu.RUnlock()
		http.Error(w, "未知皮肤视图", http.StatusBadRequest)
		return
	}
	skins = append([]Skin(nil), skins...)
	a.mu.RUnlock()
	respondJSON(w, map[string]any{"items": skins, "count": len(skins)})
}

func (a *app) handlePoolSkins(w http.ResponseWriter, _ *http.Request) {
	a.mu.RLock()
	if !a.calculationOKLocked() {
		a.mu.RUnlock()
		http.Error(w, "当前奖池还没有准备好", http.StatusConflict)
		return
	}
	items := make([]Skin, 0, a.poolTotal)
	for _, skin := range a.allSkins {
		if skin.PoolName != "" {
			items = append(items, skin)
		}
	}
	poolID := a.poolID
	poolName := a.pools[a.poolID].Name
	a.mu.RUnlock()
	respondJSON(w, map[string]any{"items": items, "count": len(items), "poolId": poolID, "poolName": poolName})
}

func (a *app) handleSkinDetails(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "无效的皮肤 ID", http.StatusBadRequest)
		return
	}
	a.mu.RLock()
	client := a.lcu
	connected := a.connected && a.snapshotReady
	var skin Skin
	for _, candidate := range a.allSkins {
		if candidate.ID == id {
			skin = candidate
			break
		}
	}
	a.mu.RUnlock()
	if !connected || client == nil {
		http.Error(w, "当前没有可用的客户端快照", http.StatusConflict)
		return
	}
	if skin.ID == 0 {
		http.Error(w, "当前目录中没有这款皮肤", http.StatusNotFound)
		return
	}
	type priceResult struct {
		value int
		known bool
	}
	type borderResult struct {
		hasBorder bool
		known     bool
		owned     bool
	}
	priceResults := make(chan priceResult, 1)
	borderResults := make(chan borderResult, 1)
	go func() {
		value, known := NewStoreAPI(client).SkinPrice(skin.ID)
		priceResults <- priceResult{value: value, known: known}
	}()
	go func() {
		hasBorder, known, owned := NewSkinAppearanceAPI(client).BorderStatus(skin)
		borderResults <- borderResult{hasBorder: hasBorder, known: known, owned: owned}
	}()
	price := <-priceResults
	border := <-borderResults
	detail := SkinDetailData{
		PriceRP: price.value, PriceKnown: price.known,
		HasBorder: border.hasBorder, BorderOwnershipKnown: border.known, OwnsBorder: border.owned,
	}
	respondJSON(w, map[string]any{"skin": skin, "details": detail})
}

func (a *app) handleRefresh(w http.ResponseWriter, _ *http.Request) {
	a.requestRefresh()
	w.WriteHeader(http.StatusAccepted)
}

func (a *app) handleQuit(w http.ResponseWriter, _ *http.Request) {
	a.mu.Lock()
	client := a.lcu
	a.lcu = nil
	a.mu.Unlock()
	if client != nil {
		client.Close()
	}
	respondJSON(w, map[string]bool{"ok": true})
	go func() {
		time.Sleep(150 * time.Millisecond)
		os.Exit(0)
	}()
}

func (a *app) handleImage(w http.ResponseWriter, r *http.Request) {
	assetPath := r.URL.Query().Get("path")
	cleanPath := pathpkg.Clean(assetPath)
	if cleanPath != assetPath || sanitizeClientImagePath(assetPath) == "" {
		http.Error(w, "invalid asset path", http.StatusBadRequest)
		return
	}
	a.mu.RLock()
	client := a.lcu
	connected := a.connected
	a.mu.RUnlock()
	if client == nil || !connected {
		// 未连接客户端（例如只查看韩服页签）时改用 CommunityDragon：
		// 其目录结构与客户端的 lol-game-data 资源路径完全一致。
		a.serveCommunityDragonImage(w, r, assetPath)
		return
	}
	data, err := a.loadAsset(r.Context(), assetPath, 2*1024*1024, 0, func(ctx context.Context) ([]byte, error) {
		return client.GetBytesContext(ctx, assetPath)
	})
	if err != nil {
		http.NotFound(w, r)
		return
	}
	contentType := http.DetectContentType(data)
	if strings.EqualFold(pathpkg.Ext(assetPath), ".svg") && bytes.Contains(data, []byte("<svg")) && !bytes.Contains(bytes.ToLower(data), []byte("<script")) {
		contentType = "image/svg+xml"
	}
	if !strings.HasPrefix(contentType, "image/") {
		http.Error(w, "not an image", http.StatusUnsupportedMediaType)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(data)
}

func (a *app) serveCommunityDragonImage(w http.ResponseWriter, r *http.Request, assetPath string) {
	remotePaths := communityDragonImagePaths(assetPath)
	if len(remotePaths) == 0 || a.champions == nil {
		http.NotFound(w, r)
		return
	}
	var data []byte
	for _, remotePath := range remotePaths {
		loaded, err := a.loadAsset(r.Context(), "cdragon:"+remotePath, 2*1024*1024, 0, func(ctx context.Context) ([]byte, error) {
			return a.champions.fetchDirect(ctx, communityDragonHost, remotePath, nil, 2*1024*1024, "image/avif,image/webp,image/png,image/jpeg,image/*;q=0.8")
		})
		if err == nil && strings.HasPrefix(http.DetectContentType(loaded), "image/") {
			data = loaded
			break
		}
	}
	if len(data) == 0 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", http.DetectContentType(data))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, _ = w.Write(data)
}

func communityDragonImagePaths(assetPath string) []string {
	trimmed, ok := strings.CutPrefix(strings.ToLower(assetPath), "/lol-game-data/assets")
	if !ok || trimmed == "" || trimmed[0] != '/' {
		return nil
	}
	return []string{
		"/latest/plugins/rcp-be-lol-game-data/global/default" + trimmed,
		"/latest/game/assets" + trimmed,
	}
}

func (a *app) handleMedia(w http.ResponseWriter, r *http.Request) {
	assetPath := r.URL.Query().Get("path")
	cleanPath := pathpkg.Clean(assetPath)
	extension := strings.ToLower(pathpkg.Ext(assetPath))
	if cleanPath != assetPath || strings.Contains(assetPath, "..") || !strings.HasPrefix(strings.ToLower(assetPath), "/lol-game-data/assets/") || (extension != ".webm" && extension != ".mp4") {
		http.Error(w, "invalid media path", http.StatusBadRequest)
		return
	}
	a.mu.RLock()
	client := a.lcu
	connected := a.connected
	a.mu.RUnlock()
	if client == nil || !connected {
		http.NotFound(w, r)
		return
	}
	a.mu.Lock()
	if a.mediaSlots == nil {
		a.mediaSlots = make(chan struct{}, 2)
	}
	mediaSlots := a.mediaSlots
	a.mu.Unlock()
	select {
	case mediaSlots <- struct{}{}:
		defer func() { <-mediaSlots }()
	case <-r.Context().Done():
		return
	}
	data, err := a.loadAsset(r.Context(), "media:"+assetPath, 0, 0, func(ctx context.Context) ([]byte, error) {
		return client.GetMediaBytesContext(ctx, assetPath)
	})
	if err != nil {
		http.NotFound(w, r)
		return
	}
	contentType := "video/webm"
	if extension == ".mp4" {
		contentType = "video/mp4"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, pathpkg.Base(assetPath), time.Time{}, bytes.NewReader(data))
}

func (a *app) refresh() {
	a.mu.RLock()
	client := a.lcu
	a.mu.RUnlock()
	if client == nil {
		a.requestRefresh()
		return
	}
	_ = a.refreshWithClient(client)
}

func (a *app) refreshWithClient(client *LCUClient) bool {
	started := time.Now()
	a.mu.Lock()
	if a.syncing {
		a.mu.Unlock()
		return true
	}
	a.syncing = true
	generation := a.poolGeneration
	pool := a.pools[a.poolID]
	a.mu.Unlock()
	a.broadcastEvent("refresh-started")

	result, err := loadSnapshotWithClient(client, pool)
	clientAlive := false
	if err != nil {
		clientAlive = client.probe() == nil
	}
	a.mu.Lock()
	if generation != a.poolGeneration {
		a.syncing = false
		a.mu.Unlock()
		a.requestRefresh()
		return true
	}
	a.syncing = false
	a.lastAttempt = time.Now()
	a.lastDuration = time.Since(started)
	if err != nil {
		message := friendlyError(err)
		if clientAlive {
			a.retainClientAfterSnapshotErrorLocked(client, result, message)
		} else {
			a.clearSnapshotLocked(message)
			a.ownership = append([]OwnershipSourceStatus(nil), result.Ownership...)
			a.catalog = result.Catalog
		}
		a.mu.Unlock()
		a.clearAssetCache()
		a.recordDiagnostic(map[string]any{"event": "refresh_failed", "error": message, "client_alive": clientAlive, "duration_ms": time.Since(started).Milliseconds(), "pool_id": pool.ID, "pool_hash": pool.Hash, "catalog": result.Catalog, "ownership_sources": result.Ownership})
		a.broadcastEvent("refresh-failed")
		return clientAlive
	}
	a.lastSync = a.lastAttempt
	a.connected = true
	a.snapshotReady = true
	a.lastError = ""
	if gameplaySummonerChanged(a.summoner, result.Summoner) {
		a.gameplayRefs = make(map[string]string)
		a.gameplayRefDetails = make(map[string]gameplayReference)
	}
	a.summoner = result.Summoner
	a.account = cloneAccountData(result.Account)
	a.allSkins = result.All
	a.chromas = append([]Chroma(nil), result.Chromas...)
	a.chromaState = result.ChromaState
	a.owned = result.Owned
	a.remaining = result.Remaining
	a.poolTotal = result.PoolTotal
	a.poolMatched = result.PoolMatched
	a.poolIssues = result.Issues
	a.lcu = result.Client
	a.ownership = append([]OwnershipSourceStatus(nil), result.Ownership...)
	a.catalog = result.Catalog
	savedSnapshot := a.snapshotLocked()
	calculationOK := a.calculationOKLocked()
	a.mu.Unlock()
	a.recordDiagnostic(map[string]any{"event": "refresh_succeeded", "duration_ms": time.Since(started).Milliseconds(), "pool_id": pool.ID, "pool_hash": pool.Hash, "owned": len(result.Owned), "remaining": len(result.Remaining), "matched": result.PoolMatched, "catalog": result.Catalog, "ownership_sources": result.Ownership})
	if calculationOK && a.storage != nil {
		if _, saveErr := a.storage.saveSnapshot(savedSnapshot, pool); saveErr != nil {
			a.recordDiagnostic(map[string]any{"event": "snapshot_save_failed", "error": "local write failed"})
		}
	}
	a.broadcastEvent("snapshot-updated")
	return true
}

func (a *app) refreshAccountWithClient(client *LCUClient) {
	a.mu.Lock()
	if a.syncing || !a.connected || a.lcu != client {
		a.mu.Unlock()
		return
	}
	a.syncing = true
	skins := append([]Skin(nil), a.allSkins...)
	a.mu.Unlock()
	profile, profileCapability := NewSummonerAPI(client).Profile()
	loot, lootCapability := NewLootAPI(client).PlayerLoot()
	loot = enrichLootItems(loot, skins)
	sanctumSparks, sanctumCapability := NewLootAPI(client).SanctumSparks()
	rewards, rewardsCapability := NewRewardsAPI(client).PendingGrants()
	account := AccountData{Profile: profile, Loot: loot, Rewards: rewards, SanctumSparks: sanctumSparks, SanctumSparksKnown: sanctumCapability.State == capabilityAvailable, Capabilities: []EndpointCapability{profileCapability, lootCapability, sanctumCapability, rewardsCapability}}
	a.mu.Lock()
	if a.connected && a.lcu == client {
		a.account = cloneAccountData(account)
	}
	a.syncing = false
	a.mu.Unlock()
	a.broadcastEvent("account-updated")
}

func (a *app) calculationOKLocked() bool {
	return a.connected && a.snapshotReady && a.poolTotal > 0 && a.poolMatched == a.poolTotal && len(a.poolIssues) == 0
}

func (a *app) clearSnapshotLocked(message string) {
	a.connected = false
	a.snapshotReady = false
	a.lastError = message
	a.summoner = Summoner{}
	a.account = AccountData{}
	a.allSkins = nil
	a.chromas = nil
	a.chromaState = EndpointCapability{}
	a.owned = nil
	a.remaining = nil
	a.poolMatched = 0
	a.poolIssues = nil
	a.lcu = nil
	a.ownership = nil
	a.catalog = CatalogStats{}
	a.eventStream = false
	// 匿名玩家引用表刻意保留：客户端短暂断线、快照失败都不应让已打开的
	// 玩家页签（尤其是不依赖客户端的韩服页签）失效；引用只在召唤师真正
	// 更换（gameplaySummonerChanged）时清空，防止跨账号复用。
}

func (a *app) retainClientAfterSnapshotErrorLocked(client *LCUClient, result Snapshot, message string) {
	a.connected = true
	a.snapshotReady = false
	a.lastError = message
	if gameplaySummonerChanged(a.summoner, result.Summoner) {
		a.gameplayRefs = make(map[string]string)
		a.gameplayRefDetails = make(map[string]gameplayReference)
	}
	a.summoner = result.Summoner
	a.account = AccountData{}
	a.allSkins = nil
	a.chromas = nil
	a.chromaState = result.ChromaState
	a.owned = nil
	a.remaining = nil
	a.poolMatched = 0
	a.poolIssues = nil
	a.lcu = client
	a.ownership = append([]OwnershipSourceStatus(nil), result.Ownership...)
	a.catalog = result.Catalog
	a.eventStream = false
}

func (a *app) clearAssetCache() {
	a.assetCacheMu.Lock()
	a.assetCache = make(map[string][]byte)
	a.assetCacheOrder = nil
	a.assetCacheBytes = 0
	a.assetCacheGeneration++
	a.assetFailureUntil = make(map[string]time.Time)
	a.assetCacheMu.Unlock()
}

func (a *app) recordDiagnostic(event map[string]any) {
	if a.storage == nil {
		return
	}
	err := a.storage.appendDiagnostic(event)
	a.diagnosticLogMu.Lock()
	if err != nil {
		a.diagnosticLogErr = "诊断日志写入失败"
	} else {
		a.diagnosticLogErr = ""
	}
	a.diagnosticLogMu.Unlock()
}

func (a *app) updateDiscovery(report LCUDiscoveryStatus) {
	a.mu.Lock()
	a.discovery = report
	a.mu.Unlock()
	a.recordDiagnostic(map[string]any{
		"event":                 "lcu_discovery",
		"method":                report.Method,
		"result":                report.Result,
		"detail":                report.Detail,
		"process_count":         report.ProcessCount,
		"unreadable_processes":  report.UnreadableProcesses,
		"command_line_count":    report.CommandLineCount,
		"credential_candidates": report.CredentialCandidates,
		"lockfiles_checked":     report.LockfilesChecked,
		"lockfiles_found":       report.LockfilesFound,
		"probe_failures":        report.ProbeFailures,
	})
}

func (a *app) snapshotLocked() Snapshot {
	return Snapshot{
		Summoner: a.summoner, All: append([]Skin(nil), a.allSkins...), Owned: append([]Skin(nil), a.owned...),
		Remaining: append([]Skin(nil), a.remaining...), PoolTotal: a.poolTotal, PoolMatched: a.poolMatched,
		Issues: append([]PoolIssue(nil), a.poolIssues...), Client: a.lcu,
		Ownership: append([]OwnershipSourceStatus(nil), a.ownership...), Catalog: a.catalog,
		Chromas: append([]Chroma(nil), a.chromas...), ChromaState: a.chromaState,
		Account: cloneAccountData(a.account),
	}
}

func randomToken(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// sessionTokenFileName 保存本地界面的会话令牌。跨重启复用同一令牌，
// 浏览器里已授权的页面在服务重启后依然有效，不会整页退回 401。
const sessionTokenFileName = "session-token"

func loadOrCreateSessionToken(store *localStore) (string, error) {
	if store == nil {
		return randomToken(24)
	}
	path := filepath.Join(store.root, sessionTokenFileName)
	if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
		if raw, readErr := os.ReadFile(path); readErr == nil {
			candidate := strings.TrimSpace(string(raw))
			if isSessionToken(candidate) {
				return candidate, nil
			}
		}
	}
	token, err := randomToken(24)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		log.Printf("会话令牌未能持久化，服务重启后需要重新打开页面：%v", err)
	}
	return token, nil
}

func isSessionToken(value string) bool {
	if len(value) != 48 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func writeDesktopReady(writer io.Writer, baseAddress, bootstrapAddress, token string) error {
	ready, err := json.Marshal(map[string]string{"baseUrl": baseAddress, "bootstrapUrl": bootstrapAddress, "token": token})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, "LOOT_READY %s\n", ready)
	return err
}

func respondJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}

func openBrowser(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	case "darwin":
		cmd = exec.Command("open", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	return cmd.Start()
}

func friendlyError(err error) string {
	if errors.Is(err, errLCUCredentialsUnreadable) {
		return "已检测到英雄联盟客户端进程，但无法读取连接凭据。请确认客户端已进入大厅；如果英雄联盟以管理员身份运行，请也以管理员身份启动本助手。"
	}
	if errors.Is(err, errLCUProbeFailed) {
		return "已找到英雄联盟客户端连接凭据，但本地接口尚未就绪。请进入客户端大厅后重新读取；脱敏诊断会保存在本机日志目录。"
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "leagueclient") || strings.Contains(message, "lockfile") {
		return "尚未发现已登录的英雄联盟客户端。请先登录国服客户端并停留在大厅，然后点击重新读取。"
	}
	if strings.Contains(message, "summoner") {
		return "已连接客户端，但尚未进入可读取账号信息的大厅。"
	}
	if strings.Contains(message, "owned skin inventory") || strings.Contains(message, "ownership sources") {
		return "库存证据未达到一致性要求，已停止计算。脱敏来源状态已写入本机日志。"
	}
	if strings.Contains(message, "skin catalog") {
		return "客户端皮肤目录不完整或格式已变化，已停止计算。"
	}
	if strings.Contains(message, "response exceeds") {
		return "客户端返回的数据超过安全大小限制，已停止读取。"
	}
	if strings.Contains(message, "pool") {
		return "奖池清单无效或未能完整映射，已停止计算。"
	}
	return "读取失败。为避免泄露本机路径或客户端细节，只记录了本机脱敏日志。"
}

func readLimited(body io.Reader, limit int64) ([]byte, error) {
	if limit < 0 {
		return nil, errors.New("invalid response limit")
	}
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	return data, nil
}

func parsePoolNames(raw string) []string {
	var out []string
	seen := map[string]bool{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		key := normalizeName(line)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, line)
	}
	return out
}

func ensureWindows() error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("LeagueClient discovery is only available on Windows")
	}
	return nil
}
