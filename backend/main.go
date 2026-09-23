package main

import (
	"bytes"
	"container/list"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
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

var version = "dev"
var buildFingerprint = "dev"

const diagnosticDeduplicationLimit = 512

// 侧边栏字标用的 Beaufort for LOL Bold（英雄联盟官方定制字体，见
// web/beaufort-for-lol-notice.txt——这不是开源字体，字体自带 name 表里的版权/商标
// 声明和 EULA 链接原样转录在那份 notice 里；本项目是非官方、非商业的粉丝向 LOL
// 战绩查询工具，随 EXE 分发这份 notice 是为了让版权与授权来源可追溯）。
// 这里逐个文件写死而不是加 web/*.woff2、web/*.txt 两条通配：R14 就是因为 web/*
// 通配把 *.test.cjs、README 一起打进 EXE 还能被 HTTP 直接下载，通配符在这个目录下
// 已经被证明是不安全的默认。
//
//go:embed web/*.js web/*.css web/*.html web/*.png web/*.svg
//go:embed web/arena-team-icons/*.svg web/position-icons/*.svg web/tier-icons/*.svg
//go:embed web/rune-styles/*.svg web/loot-icons/*.svg web/loot-icons/*.png web/rank-crests/*.png
//go:embed web/beaufort-for-lol-bold.woff2 web/beaufort-for-lol-notice.txt
//go:embed data/reroll_pool_14_5.txt data/reroll_pool_14_5.json
var embedded embed.FS

type app struct {
	collectionDataRetry             *time.Timer
	collectionDataRetryCount        int
	collectionDataRetryClient       *LCUClient
	liveSnapshots                   liveSnapshotCache
	liveClientAllGameData           func(context.Context) ([]byte, int, error)
	liveClientAllGameDataMu         sync.Mutex
	liveClientAllGameDataKeys       map[string]struct{}
	gameplayFlow                    gameplayFlowState
	overviewTimeout                 func(context.Context, time.Duration) (context.Context, context.CancelFunc)
	updates                         *updateManager
	runtimeCancel                   context.CancelFunc
	proRefreshContext               context.Context
	proProfiles                     proProfileCache
	quitOnce                        sync.Once
	currentGames                    currentGameStore
	proPlayers                      proPlayersCache
	proRuneMu                       sync.Mutex
	proRunes                        *proRuneProvider
	mu                              sync.RWMutex
	token                           string
	startedAt                       time.Time
	lastSync                        time.Time
	lastAttempt                     time.Time
	lastDuration                    time.Duration
	lastError                       string
	snapshotRetryCount              int
	snapshotRetryStarted            time.Time
	snapshotRetryExhausted          bool
	snapshotFallback                bool
	snapshotFallbackAt              time.Time
	collectionDirty                 bool
	collectionDirtyAt               time.Time
	fallbackOwned                   []Skin
	fallbackRemaining               []Skin
	connected                       bool
	manualDisconnected              bool
	identityReady                   bool
	snapshotReady                   bool
	collectionRequested             bool
	connectionState                 string
	eventStream                     bool
	summoner                        Summoner
	summonerIdentityAt              time.Time
	account                         AccountData
	masteries                       map[int64]ChampionMastery
	masteryCapability               EndpointCapability
	allSkins                        []Skin
	allSkinsWithBase                []Skin
	chromas                         []Chroma
	chromaState                     EndpointCapability
	owned                           []Skin
	remaining                       []Skin
	poolTotal                       int
	poolMatched                     int
	poolIssues                      []PoolIssue
	lcu                             *LCUClient
	syncing                         bool
	poolSource                      string
	poolVersion                     string
	poolID                          string
	poolHash                        string
	poolGeneration                  uint64
	pools                           map[string]PoolManifest
	storage                         *localStore
	diagnosticLogMu                 sync.RWMutex
	diagnosticLogErr                string
	diagnosticWriteFailures         uint64
	diagnosticWritePending          uint64
	diagnosticWriteRecovered        uint64
	diagnosticWriteLastKind         string
	ownership                       []OwnershipSourceStatus
	catalog                         CatalogStats
	assetCacheMu                    sync.RWMutex
	assetCache                      map[string][]byte
	assetCacheOrder                 []string
	assetCacheBytes                 int
	assetCacheGeneration            uint64
	assetFlights                    map[string]*assetFlight
	assetFailureUntil               map[string]time.Time
	assetHostTimeouts               map[string]int
	assetHostBackoffUntil           map[string]time.Time
	matchTierCacheOnce              sync.Once
	matchTierCache                  *championDataCache
	mediaSlots                      chan struct{}
	collectionRefreshPending        bool
	refreshRequests                 chan struct{}
	discovery                       LCUDiscoveryStatus
	eventMu                         sync.Mutex
	eventSubscribers                map[chan string]struct{}
	summonerIdentityMu              sync.Mutex
	summonerIdentityFlight          *summonerIdentityFlight
	summonerIdentityEventMu         sync.Mutex
	summonerIdentityEventTimer      *time.Timer
	summonerIdentityEventGeneration uint64
	gameplayRefsMu                  sync.Mutex
	gameplayRefs                    map[string]string
	gameplayRefDetails              map[string]gameplayReference
	gameplayRefOrder                *list.List
	gameplayRefEntries              map[string]*list.Element
	riotClientDiscovery             func() (*RiotClientAPI, error)
	clientInstallations             func() []clientInstallation
	clientInstallationsMu           sync.Mutex
	clientInstallationsCache        []clientInstallation
	clientInstallationsCacheScan    clientInstallationScan
	clientInstallationsCacheAt      time.Time
	collectionProbe                 func(context.Context, *LCUClient, int64) (bool, int)
	collectionRefresh               func(*LCUClient) bool
	liveClientPlayerList            func(context.Context) ([]byte, int, error)
	liveClientPlayerListNow         func() time.Time
	clientLauncher                  func(clientInstallation) (clientLaunchResult, error)
	clientLaunch                    clientLaunchState
	itemSetMu                       sync.Mutex
	itemSetPriceMu                  sync.Mutex
	itemSetPrices                   map[int64]int64
	itemSetPricesAt                 time.Time
	champions                       *championProvider
	liveRecommendationPrewarmer     *liveRecommendationPrewarmer
	riot                            *riotProvider
	sgp                             *sgpProvider
	watch                           *watchRunner
	convenience                     *convenienceRunner // legacy alias; points at watch
	lpTracker                       *lpTracker
	rankScores                      *rankScoreCache
	rankScoresOnce                  sync.Once
	opgg                            *opggInsights
	overviewQueries                 *overviewQueryCache
	matchTimelines                  *matchTimelineCache
	mayhemRatings                   *aramkitRatingClient
	facadeMu                        sync.Mutex
	facadeManualVersion             uint64
	perkCatalogDisk                 *championDataCache
	perkAugmentJobs                 map[string]chan struct{}
	perkAugmentAttempts             map[string]time.Time
	perkCatalogMu                   sync.Mutex
	perkCatalog                     map[string]gameplayPerkCatalogCacheEntry
	// 赛季统计后台回补的单飞登记表：key 是 accountHash|season。
	seasonBackfillMu sync.Mutex
	seasonBackfills  map[string]struct{}
	// seasonQuerySnapshots is the first query-parameter deduplication gate.
	// Cache and single-flight remain the second and third gates.
	seasonQuerySnapshots                map[string]time.Time
	seasonQuerySnapshotOrder            *list.List
	seasonQuerySnapshotEntries          map[string]*list.Element
	queueFilterCapabilityMu             sync.RWMutex
	queueFilterCapabilities             map[string]string
	recentRankedSamplesMu               sync.Mutex
	recentRankedSamples                 map[string]recentRankedSampleCacheEntry
	recentRankedSampleOrder             *list.List
	recentRankedSampleEntries           map[string]*list.Element
	livePlayerMatchesMu                 sync.Mutex
	livePlayerMatchCache                map[string]livePlayerMatchesCacheEntry
	livePlayerMatchOrder                *list.List
	livePlayerMatchEntries              map[string]*list.Element
	livePlayerMatchFlights              map[string]*livePlayerMatchesFlight
	liveRosterConcurrencyOverride       int
	rankedSplitProbeOnce                sync.Once
	rankedSplitProbeEnabled             bool
	rankedLCUShapeOnce                  sync.Once
	rankedCompletionGateMu              sync.Mutex
	rankedCompletionRecent              []bool
	rankedCompletionClosed              bool
	recommendationModeDiagnosticMu      sync.Mutex
	recommendationModeDiagnosticKeys    map[string]struct{}
	matchModeDiagnosticMu               sync.Mutex
	matchModeDiagnosticKeys             map[int64]struct{}
	championDataDiagnosticMu            sync.Mutex
	championDataDiagnosticKeys          map[string]struct{}
	liveRosterDiagnosticMu              sync.Mutex
	liveRosterDiagnosticKeys            map[string]struct{}
	livePositionMu                      sync.Mutex
	livePositionGameID                  int64
	livePositionByRef                   map[string]string
	liveClientPlayerListDiagnosticMu    sync.Mutex
	liveClientPlayerListDiagnosticKeys  map[string]struct{}
	liveClientProbeMu                   sync.Mutex
	liveClientProbe                     liveClientProbeState
	arenaAlliesMu                       sync.RWMutex
	arenaAllyKeys                       map[string]struct{}
	arenaAllyPlayers                    []lcuLivePlayer
	arenaAllyGameID                     int64
	arenaTruth                          arenaTruthState
	unknownQueueDiagnosticMu            sync.Mutex
	unknownQueueDiagnosticIDs           map[int64]struct{}
	diagnosticDedupMu                   sync.Mutex
	diagnosticDedupCounts               map[string]int
	rankedWinrateDiagnosticMu           sync.Mutex
	rankedWinrateDiagnosticBuckets      map[string]*rankedWinrateDiagnosticBucket
	lcuGameflowShapeDiagnosticMu        sync.Mutex
	lcuGameflowShapeDiagnosticKeys      map[string]struct{}
	lcuChampSelectShapeDiagnosticMu     sync.Mutex
	lcuChampSelectShapeDiagnosticKeys   map[string]struct{}
	facadeIdentityShapeDiagnosticMu     sync.Mutex
	facadeIdentityShapeDiagnosticClient *LCUClient
	facadeProbeMu                       sync.Mutex
	facadeIcons                         facadeIconCache
	facadeView                          facadeViewCache
	profileIconImages                   *championDataCache
	facadeSkinCatalogMu                 sync.Mutex
	facadeSkinCatalogClient             *LCUClient
	facadeSkinCatalog                   []Skin
	facadeSkinCatalogAt                 time.Time
	facadeSkinCatalogErr                error
	facadeChallengeCatalogMu            sync.Mutex
	facadeChallengeCatalogClient        *LCUClient
	facadeChallengeCatalogAttempted     bool
	facadeChallengeCatalogBackoffUntil  time.Time
	facadeChallengeCatalogFlight        chan struct{}
	facadeChallengeCatalog              map[string]facadeChallenge
	facadeChallengeCatalogRaw           json.RawMessage
	facadeChallengeCatalogErr           error
	facadeEventMu                       sync.Mutex
	facadeEventLastBroadcast            time.Time
	facadeEventTimer                    *time.Timer
	facadeEventPending                  bool
	facadeEventGeneration               uint64
}

type statusResponse struct {
	Update                 updateStatus         `json:"update"`
	Version                string               `json:"version"`
	BuildFingerprint       string               `json:"buildFingerprint"`
	Connected              bool                 `json:"connected"`
	IdentityReady          bool                 `json:"identityReady"`
	SnapshotReady          bool                 `json:"snapshotReady"`
	ConnectionState        string               `json:"connectionState"`
	EventStream            bool                 `json:"eventStream"`
	Syncing                bool                 `json:"syncing"`
	LastSync               time.Time            `json:"lastSync,omitempty"`
	LastError              string               `json:"lastError,omitempty"`
	LastAttempt            time.Time            `json:"lastAttempt,omitempty"`
	LastDurationMS         int64                `json:"lastDurationMs"`
	SnapshotRetryCount     int                  `json:"snapshotRetryCount,omitempty"`
	SnapshotRetryElapsedMS int64                `json:"snapshotRetryElapsedMs,omitempty"`
	SnapshotRetryExhausted bool                 `json:"snapshotRetryExhausted,omitempty"`
	SnapshotFallback       bool                 `json:"snapshotFallback,omitempty"`
	SnapshotFallbackAt     time.Time            `json:"snapshotFallbackAt,omitempty"`
	CollectionDirty        bool                 `json:"collectionDirty,omitempty"`
	Summoner               publicSummoner       `json:"summoner"`
	OwnedCount             int                  `json:"ownedCount"`
	ChromaOwnedCount       int                  `json:"chromaOwnedCount"`
	PoolTotal              int                  `json:"poolTotal"`
	PoolMatched            int                  `json:"poolMatched"`
	Remaining              int                  `json:"remainingCount"`
	CalculationOK          bool                 `json:"calculationOK"`
	PoolIssues             []PoolIssue          `json:"poolIssues,omitempty"`
	PoolSource             string               `json:"poolSource"`
	PoolVersion            string               `json:"poolVersion"`
	PoolID                 string               `json:"poolId"`
	PoolHash               string               `json:"poolHash"`
	StorageReady           bool                 `json:"storageReady"`
	ServerID               string               `json:"serverId,omitempty"`
	ServerName             string               `json:"serverName,omitempty"`
	QueueGroups            []queueGroupResponse `json:"queueGroups"`
}

type publicSummoner struct {
	DisplayName          string `json:"displayName,omitempty"`
	GameName             string `json:"gameName,omitempty"`
	TagLine              string `json:"tagLine,omitempty"`
	ProfileIconID        int64  `json:"profileIconId,omitempty"`
	SummonerLevel        int64  `json:"summonerLevel,omitempty"`
	BackgroundSkinID     int64  `json:"backgroundSkinId"`
	BackgroundSkinName   string `json:"backgroundSkinName"`
	BackgroundSource     string `json:"backgroundSource"`
	BackgroundPath       string `json:"backgroundPath"`
	BackgroundPosterPath string `json:"backgroundPosterPath,omitempty"`
	BackgroundVideoPath  string `json:"backgroundVideoPath,omitempty"`
}

func main() {
	startupWarmup := flag.Bool("startup-warmup", false, "exit before services, networking or user storage initialization")
	selfTest := flag.Bool("self-test", false, "validate embedded resources and exit")
	selfCheckRiotKey := flag.Bool("self-check-riot-key", false, "validate the embedded Riot API key and exit")
	noBrowser := flag.Bool("no-browser", false, "do not open the default browser")
	desktopMode := flag.Bool("desktop", false, "emit a desktop-shell bootstrap event and do not open a browser")
	listenAddress := flag.String("listen", "127.0.0.1:0", "loopback address for the local UI")
	encryptRiotKeyFlag := flag.String("encrypt-riot-key", "", "encrypt a Riot API key for embedding in riot_api.go and exit")
	flag.Parse()
	// Execution-only warmup must precede every persistent/service initializer.
	// --self-test below also validates user/provider state and is not read-only.
	if *startupWarmup {
		return
	}
	if strings.TrimSpace(*encryptRiotKeyFlag) != "" {
		cipherText, err := encryptRiotKey(*encryptRiotKeyFlag)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(cipherText)
		return
	}
	if *selfCheckRiotKey {
		if !riotKeyConfigured() {
			log.Fatal("Riot API key is not embedded")
		}
		fmt.Println("embedded Riot API key is configured")
		if !*selfTest {
			return
		}
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
	defer closeDiagnosticStore(store)
	token, err := loadOrCreateSessionToken(store)
	if err != nil {
		closeDiagnosticStore(store)
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
	championProvider.imageCache = newPublicBinaryCache(store, "champion-images", 2048, 64<<20)
	championProvider.communityImageCache = newCommunityImageCache(store)
	championProvider.hexdata = newHexdataClient(championProvider, store)
	if store != nil {
		championProvider.diag = func(event map[string]any) { _ = store.appendDiagnostic(event) }
	}
	if err := championProvider.setNetworkSettings(loadChampionNetworkSettings(store)); err != nil {
		log.Printf("英雄数据代理设置无效，已使用自动模式：%v", err)
		_ = championProvider.setNetworkSettings(defaultChampionNetworkSettings())
	}
	if gateURL := strings.TrimSpace(os.Getenv("DEEP_LEGENDS_FEATURE_GATES_URL")); gateURL != "" {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			if gateErr := championProvider.featureGates.refresh(ctx, championProvider.httpClient(), gateURL); gateErr != nil {
				if championProvider.diag != nil {
					championProvider.diag(map[string]any{"event": "feature_gates_failed", "reason": safeDiagnosticReason(gateErr)})
				}
				return
			}
			if championProvider.diag != nil {
				championProvider.diag(map[string]any{"event": "feature_gates_loaded"})
			}
		}()
	}
	a := &app{
		token:                 token,
		startedAt:             time.Now(),
		poolTotal:             len(builtInPool.Names),
		poolSource:            builtInPool.Source,
		poolVersion:           builtInPool.Version,
		poolID:                builtInPool.ID,
		poolHash:              builtInPool.Hash,
		pools:                 pools,
		storage:               store,
		assetCache:            make(map[string][]byte),
		assetFlights:          make(map[string]*assetFlight),
		assetFailureUntil:     make(map[string]time.Time),
		assetHostTimeouts:     make(map[string]int),
		assetHostBackoffUntil: make(map[string]time.Time),
		mediaSlots:            make(chan struct{}, 2),
		connectionState:       "connecting",
		refreshRequests:       make(chan struct{}, 1),
		eventSubscribers:      make(map[chan string]struct{}),
		gameplayRefs:          make(map[string]string),
		gameplayRefDetails:    make(map[string]gameplayReference),
		gameplayRefOrder:      list.New(),
		gameplayRefEntries:    make(map[string]*list.Element),
		perkCatalog:           make(map[string]gameplayPerkCatalogCacheEntry),
		riotClientDiscovery:   discoverRiotClient,
		champions:             championProvider,
		riot:                  newRiotProvider(championProvider),
		sgp:                   newSGPProvider(),
	}
	if store != nil {
		store.onDiagnosticRotation = a.resetDiagnosticDeduplication
	}
	championProvider.gameplayAugments = func(ctx context.Context) ([]gameplayAugment, error) {
		client, _, err := a.gameplayClient()
		if err != nil {
			return nil, err
		}
		return loadGameplayAugmentsFromClient(client)
	}
	a.sgp.observe = a.recordDiagnostic
	a.liveRecommendationPrewarmer = newLiveRecommendationPrewarmer(championProvider, a.recordDiagnostic)
	a.sgp.gatewayAccount = func(client *LCUClient) string {
		a.mu.RLock()
		defer a.mu.RUnlock()
		if a.lcu != client || !a.connected {
			return ""
		}
		return a.summoner.PUUID
	}
	a.watch = newWatchRunner(store, a.broadcastEvent)
	a.watch.observe = a.recordDiagnostic
	a.watch.broadcastChampionNames = a.championNames
	a.convenience = a.watch
	a.lpTracker = newLPTracker(store)
	a.lpTracker.observeEvent = a.recordDiagnostic
	a.rankScores = newRankScoreCache()
	a.riot.opponentRankScore = func(ctx context.Context, puuid string) rankScoreEntry {
		return a.playerRankScore(ctx, nil, puuid, false, "KR", "")
	}
	a.rankedSplitProbeEnabled = true
	a.opgg = newOPGGInsights()
	a.overviewQueries = newOverviewQueryCache()
	a.matchTimelines = newMatchTimelineCache()
	a.mayhemRatings = newAramkitRatingClient(championProvider, a.recordDiagnostic, a.aramkitRatingIdentityHash)
	a.recordAppStartDiagnostic()
	a.enableDiagnosticRotationSnapshot()

	webFS, err := fs.Sub(embedded, "web")
	if err != nil {
		closeDiagnosticStore(store)
		log.Fatal(err)
	}
	if *selfTest {
		log.Printf("Deep Legends %s 自检通过：奖池 %d 条，哈希 %s", version, len(builtInPool.Names), builtInPool.Hash[:12])
		return
	}
	mux := http.NewServeMux()
	// Go 内置的 MIME 表里没有 .woff2，缺省会回落到按内容嗅探出的
	// application/octet-stream。我们对所有响应都加了 X-Content-Type-Options: nosniff，
	// 类型不对时浏览器有权拒绝加载字体，字标就会静默掉回退字体——而且这种问题
	// 只在打包后的真机上出现，本地未必复现。这里显式登记，跨平台结果一致。
	if err := mime.AddExtensionType(".woff2", "font/woff2"); err != nil {
		closeDiagnosticStore(store)
		log.Fatal(err)
	}
	// Content-based validators survive process restarts and change with the
	// embedded asset, including development builds with fingerprint "dev".
	staticFiles := newStaticAssetHandler(webFS)
	mux.HandleFunc("GET /{$}", a.handleBootstrap(staticFiles))
	mux.Handle("GET /", staticFiles)
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
	mux.HandleFunc("GET /api/gameplay/mayhem-rating", a.authorized(a.handleGameplayMayhemRating))
	// R116-E P2-6：本人海克斯大乱斗「选了某个海克斯之后通常出什么」静态查询。
	mux.HandleFunc("GET /api/gameplay/season-mayhem-builds", a.authorized(a.handleGameplaySeasonMayhemBuilds))
	mux.HandleFunc("GET /api/gameplay/recommendations", a.authorized(a.handleGameplayRecommendations))
	mux.HandleFunc("GET /api/gameplay/specialist-runes", a.authorized(a.handleGameplaySpecialistRunes))
	mux.HandleFunc("GET /api/gameplay/pro-runes", a.authorized(a.handleGameplayProRunes))
	mux.HandleFunc("POST /api/gameplay/match-tiers", a.authorized(a.handleGameplayMatchTiers))
	mux.HandleFunc("POST /api/gameplay/current-game", a.authorized(a.handleOverviewCurrentGame))
	mux.HandleFunc("POST /api/gameplay/season-summary", a.authorized(a.handleOPGGSeasonSummary))
	mux.HandleFunc("POST /api/gameplay/match-timeline", a.authorized(a.handleGameplayMatchTimeline))
	mux.HandleFunc("GET /api/gameplay/phase", a.authorized(a.handleGameplayPhase))
	mux.HandleFunc("GET /api/gameplay/convenience", a.authorized(a.handleGameplayConvenience))
	mux.HandleFunc("POST /api/gameplay/convenience", a.authorized(a.handleGameplayConvenience))
	mux.HandleFunc("GET /api/watch/rules", a.authorized(a.handleWatchRules))
	mux.HandleFunc("POST /api/watch/rules", a.authorized(a.handleWatchRules))
	mux.HandleFunc("GET /api/champselect/groups", a.authorized(a.handleChampSelectGroups))
	mux.HandleFunc("GET /api/champselect/state", a.authorized(a.handleChampSelectState))
	mux.HandleFunc("POST /api/champselect/pause", a.authorized(a.handleChampSelectPause))
	mux.HandleFunc("GET /api/rig/status", a.authorized(a.handleRigStatus))
	mux.HandleFunc("POST /api/rig/settings-lock", a.authorized(a.handleSettingsLock))
	mux.HandleFunc("POST /api/rig/maintenance", a.authorized(a.handleClientMaintenance))
	mux.HandleFunc("GET /api/facade/state", a.authorized(a.handleFacadeState))
	mux.HandleFunc("GET /api/facade/icons", a.authorized(a.handleFacadeIcons))
	mux.HandleFunc("GET /api/facade/banners", a.authorized(a.handleFacadeBanners))
	mux.HandleFunc("POST /api/facade/probe", a.authorized(a.handleFacadeProbe))
	mux.HandleFunc("POST /api/facade/apply", a.authorized(a.handleFacadeApply))
	mux.HandleFunc("GET /api/claim/scan", a.authorized(a.handleClaimScan))
	mux.HandleFunc("POST /api/claim/execute", a.authorized(a.handleClaimExecute))
	mux.HandleFunc("GET /api/gameplay/perks", a.authorized(a.handleGameplayPerks))
	mux.HandleFunc("GET /api/gameplay/items", a.authorized(a.handleGameplayItems))
	mux.HandleFunc("GET /api/gameplay/summoner-spells", a.authorized(a.handleGameplaySummonerSpells))
	mux.HandleFunc("POST /api/gameplay/runes/apply", a.authorized(a.handleGameplayRuneApply))
	mux.HandleFunc("POST /api/gameplay/item-sets/apply", a.authorized(a.handleGameplayItemSetApply))
	mux.HandleFunc("GET /api/gameplay/replay", a.authorized(a.handleGameplayReplayMetadata))
	mux.HandleFunc("POST /api/gameplay/replay", a.authorized(a.handleGameplayReplayAction))
	mux.HandleFunc("GET /api/pro-players", a.authorized(a.handleProPlayers))
	mux.HandleFunc("GET /api/champions/catalog", a.authorized(a.handleChampionCatalog))
	mux.HandleFunc("GET /api/champions/rankings", a.authorized(a.handleChampionRankings))
	mux.HandleFunc("GET /api/champions/augments", a.authorized(a.handleChampionAugments))
	mux.HandleFunc("GET /api/champions/augment-detail", a.authorized(a.handleChampionAugmentDetail))
	mux.HandleFunc("GET /api/champions/augment-rarity", a.authorized(a.handleChampionAugmentRarity))
	mux.HandleFunc("GET /api/champions/detail", a.authorized(a.handleChampionDetail))
	mux.HandleFunc("GET /api/champions/arena-first-places", a.authorized(a.handleArenaFirstPlaces))
	mux.HandleFunc("GET /api/champions/arena/match/{matchId}", a.authorized(a.handleArenaMatchDetail))
	mux.HandleFunc("GET /api/champions/network", a.authorized(a.handleChampionNetwork))
	mux.HandleFunc("POST /api/champions/network", a.authorized(a.handleChampionNetwork))
	mux.HandleFunc("GET /api/social/friends", a.authorized(a.handleSocialFriends))
	mux.HandleFunc("POST /api/system-proxy", a.authorized(a.handleSystemProxy))
	mux.HandleFunc("GET /api/champion-asset", a.authorized(a.handleChampionAsset))
	mux.HandleFunc("POST /api/refresh", a.authorized(a.handleRefresh))
	mux.HandleFunc("POST /api/collection/ensure", a.authorized(a.handleCollectionEnsure))
	mux.HandleFunc("GET /api/image", a.authorized(a.handleImage))
	mux.HandleFunc("GET /api/prestige-image", a.authorized(a.handlePrestigeImage))
	mux.HandleFunc("GET /api/skin-art", a.authorized(a.handleSkinArt))
	mux.HandleFunc("GET /api/media", a.authorized(a.handleMedia))
	mux.HandleFunc("GET /api/diagnostics", a.authorized(a.handleDiagnostics))
	mux.HandleFunc("GET /api/diagnostics/log", a.authorized(a.handleDiagnosticLog))
	mux.HandleFunc("POST /api/diagnostics/objectives", a.authorized(a.handleObjectiveDiagnostics))
	mux.HandleFunc("POST /api/diagnostics/client", a.authorized(a.handleClientDiagnostic))
	mux.HandleFunc("POST /api/diagnostics/startup", a.authorized(a.handleDesktopStartup))
	mux.HandleFunc("POST /api/diagnostics/startup-stage", a.authorized(a.handleDesktopStartupStage))
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
	a.registerUpdateRoutes(mux)
	a.updates = newUpdateManager(version, a.storage, a.broadcastUpdateEvent)

	listener, err := net.Listen("tcp", *listenAddress)
	if err != nil {
		closeDiagnosticStore(store)
		log.Fatal(err)
	}
	if tcpAddress, ok := listener.Addr().(*net.TCPAddr); !ok || !tcpAddress.IP.IsLoopback() {
		_ = listener.Close()
		closeDiagnosticStore(store)
		log.Fatal("本地界面只能监听回环地址")
	}
	baseAddress := "http://" + listener.Addr().String()
	address := baseAddress + "/?bootstrap=" + url.QueryEscape(token)
	if *desktopMode {
		if readyErr := writeDesktopReady(os.Stdout, baseAddress, address, token); readyErr != nil {
			_ = listener.Close()
			closeDiagnosticStore(store)
			log.Fatal(readyErr)
		}
	}

	runtimeContext, runtimeCancel := context.WithCancel(context.Background())
	a.runtimeCancel = runtimeCancel
	a.proRefreshContext = runtimeContext
	go a.runConnectionManager(runtimeContext)
	go func() { _, _, _ = a.loadProPlayers(runtimeContext, true) }()
	a.updates.Start()
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
		WriteTimeout:      30 * time.Second,
	}
	log.Printf("Deep Legends %s 正在运行：%s", version, baseAddress)
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		closeDiagnosticStore(store)
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
	identityReady := a.identityReady || (a.connected && a.summoner.SummonerID != 0)
	ownedCount := len(a.owned)
	remainingCount := len(a.remaining)
	if a.snapshotFallback && !a.snapshotReady {
		ownedCount = len(a.fallbackOwned)
		remainingCount = len(a.fallbackRemaining)
	}
	backgroundID, backgroundName, backgroundSource, backgroundPath := profileBackground(a.account.Profile)
	summoner := publicSummoner{DisplayName: a.summoner.DisplayName, GameName: a.summoner.GameName, TagLine: a.summoner.TagLine, ProfileIconID: a.summoner.ProfileIconID, SummonerLevel: a.summoner.SummonerLevel, BackgroundSkinID: backgroundID, BackgroundSkinName: backgroundName, BackgroundSource: backgroundSource, BackgroundPath: backgroundPath}
	summoner.BackgroundPosterPath, summoner.BackgroundVideoPath = overviewSkinMedia(a.allSkins, backgroundID)
	response := statusResponse{
		Version: version, BuildFingerprint: buildFingerprint, Connected: a.connected, IdentityReady: identityReady, SnapshotReady: a.snapshotReady, ConnectionState: a.connectionState, EventStream: a.eventStream,
		Syncing: a.syncing, LastSync: a.lastSync, LastAttempt: a.lastAttempt, LastDurationMS: a.lastDuration.Milliseconds(),
		LastError: a.lastError, Summoner: summoner, OwnedCount: ownedCount, ChromaOwnedCount: ownedChromaCount(a.chromas), PoolTotal: a.poolTotal,
		PoolMatched: a.poolMatched, Remaining: remainingCount, CalculationOK: a.calculationOKLocked(),
		PoolIssues: issues, PoolSource: a.poolSource, PoolVersion: a.poolVersion, PoolID: a.poolID,
		PoolHash: a.poolHash, StorageReady: a.storage != nil,
		SnapshotRetryCount: a.snapshotRetryCount, SnapshotRetryExhausted: a.snapshotRetryExhausted,
		SnapshotFallback: a.snapshotFallback, SnapshotFallbackAt: a.snapshotFallbackAt,
		CollectionDirty: a.collectionDirty,
		QueueGroups:     queueGroupsForClient(),
	}
	if !a.snapshotRetryStarted.IsZero() {
		response.SnapshotRetryElapsedMS = time.Since(a.snapshotRetryStarted).Milliseconds()
	}
	a.mu.RUnlock()
	response.Update = a.updates.Status()
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
	if !a.connected || (!a.snapshotReady && !a.snapshotFallback) {
		a.mu.RUnlock()
		http.Error(w, "当前没有可用的客户端快照", http.StatusConflict)
		return
	}
	stale := a.snapshotFallback && !a.snapshotReady
	capturedAt := a.snapshotFallbackAt
	var skins []Skin
	switch view {
	case "owned":
		if stale {
			skins = a.fallbackOwned
		} else {
			skins = a.owned
		}
	case "all":
		if stale {
			a.mu.RUnlock()
			http.Error(w, "历史快照不包含完整皮肤目录", http.StatusConflict)
			return
		}
		skins = a.allSkins
	case "remaining":
		if stale {
			skins = a.fallbackRemaining
		} else if !a.calculationOKLocked() {
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
	respondJSON(w, map[string]any{"items": skins, "count": len(skins), "stale": stale, "capturedAt": capturedAt})
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
	a.mu.RLock()
	client := a.lcu
	connected := a.connected
	a.mu.RUnlock()
	if connected && client != nil {
		if _, err := a.refreshSummonerIdentity(client); err != nil {
			a.recordDiagnostic(map[string]any{"event": "summoner_identity_manual_refresh_failed", "reason": safeDiagnosticReason(err)})
		}
	}
	a.requestCollectionRefresh()
	w.WriteHeader(http.StatusAccepted)
}

func (a *app) handleCollectionEnsure(w http.ResponseWriter, _ *http.Request) {
	a.mu.RLock()
	connected := a.connected && a.lcu != nil
	ready := a.snapshotReady
	a.mu.RUnlock()
	if !connected {
		http.Error(w, "当前没有已连接的英雄联盟客户端", http.StatusConflict)
		return
	}
	if !ready {
		a.requestCollectionRefresh()
	}
	w.WriteHeader(http.StatusAccepted)
}

func (a *app) handleQuit(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, map[string]bool{"ok": true})
	a.scheduleQuit(150*time.Millisecond, "user")
}

func (a *app) handleImage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), publicImageTimeout)
	defer cancel()
	r = r.WithContext(ctx)
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
		return a.loadClientIcon(ctx, client, assetPath)
	})
	if err != nil {
		// 客户端只随包发布 rcp-be-lol-game-data 里的那一份资源，海克斯的
		// _large.png 全彩大图只存在于游戏侧 (/latest/game/assets/…)，连着
		// 客户端时逐个 404。回落到 CommunityDragon 才能拿到大图，否则前端
		// 只能退化成品质色实心块。
		served := a.serveCommunityDragonImage(w, r, assetPath)
		// R127 P1-a.4：只有增强符文图标会落这条日志（augmentIconPathTemplate 只认
		// augments/icons 路径）。带上 LCU 的真实状态，才能判断本机到底有没有这张
		// 图、出网是不是必要；以前这里完全没有痕迹，只能看到一堆 8 秒超时。
		status := http.StatusNotFound
		if served {
			status = http.StatusOK
		}
		a.champions.reportAugmentIconFetch(assetPath, status, -1, true, lcuFailureStatus(err))
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
	writeImageResponse(w, r, data, contentType, "private, max-age=3600")
}

// serveCommunityDragonImage 用 CommunityDragon 兜底本机客户端没有的资源，并返回
// 是否真的写出了图片（R127 P1-a.4 需要据此上报最终状态）。
func (a *app) serveCommunityDragonImage(w http.ResponseWriter, r *http.Request, assetPath string) bool {
	remotePaths := communityDragonImagePaths(assetPath)
	if len(remotePaths) == 0 || a.champions == nil {
		http.NotFound(w, r)
		return false
	}
	// 「该 assetPath 全部候选均失败」单独记一条负缓存，键用 assetPath 而不是
	// remotePath：否则每次请求都要把整组扇出重放一遍。maxEntrySize=0 表示成功
	// 结果仍只由 cdragon:<remotePath> 那层持有，这里不重复占用缓存预算。
	// 这一层是「该 assetPath 的全部候选都失败」的聚合结论，不按主机归因：每个
	// 候选在 loadCommunityDragonAsset 里已经各自归因过主机超时了。若这里也走
	// loadAssetFromHost，聚合错误会把内层刚建立的主机退避立刻抹掉（R127 复审）。
	data, err := a.loadAsset(r.Context(), "cdragon-resolved:"+assetPath, 0, communityImageResolveNegativeTTL, func(ctx context.Context) ([]byte, error) {
		for _, remotePath := range remotePaths {
			loaded, loadErr := a.loadCommunityDragonAsset(ctx, remotePath)
			if loadErr == nil && strings.HasPrefix(http.DetectContentType(loaded), "image/") {
				return loaded, nil
			}
		}
		return nil, errCommunityImageCandidatesExhausted
	})
	if err != nil || len(data) == 0 {
		http.NotFound(w, r)
		return false
	}
	writeImageResponse(w, r, data, http.DetectContentType(data), "private, max-age=86400")
	return true
}

func writeImageResponse(w http.ResponseWriter, r *http.Request, data []byte, contentType, cacheControl string) {
	etag := fmt.Sprintf(`"%x"`, sha256.Sum256(data))
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", cacheControl)
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, _ = w.Write(data)
}

func communityDragonImagePaths(assetPath string) []string {
	const prefix = "/lol-game-data/assets/"
	value := strings.ToLower(strings.TrimSpace(assetPath))
	if !strings.HasPrefix(value, prefix) {
		return nil
	}
	relative := strings.TrimPrefix(value, prefix)
	if relative == "" || strings.Contains(relative, "..") {
		return nil
	}
	gameRelative := relative
	if !strings.HasPrefix(gameRelative, "assets/") {
		gameRelative = "assets/" + gameRelative
	}
	pluginPath := "/latest/plugins/rcp-be-lol-game-data/global/default/" + relative
	gamePath := "/latest/game/" + gameRelative
	if strings.HasSuffix(relative, "_large.png") {
		return []string{
			gamePath,
			pluginPath,
			strings.TrimSuffix(gamePath, "_large.png") + ".png",
			strings.TrimSuffix(pluginPath, "_large.png") + ".png",
			strings.TrimSuffix(gamePath, "_large.png") + "_small.png",
			strings.TrimSuffix(pluginPath, "_large.png") + "_small.png",
		}
	}
	return []string{
		pluginPath,
		gamePath,
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

func (a *app) refreshIdentityWithClient(client *LCUClient) bool {
	client.setDiagnosticObserver(a.recordDiagnostic)
	result, err := loadIdentitySnapshot(client)
	if err != nil {
		a.recordDiagnostic(map[string]any{"event": "identity_refresh_failed", "error": friendlyError(err), "load_phases_ms": result.LoadPhases, "phase_group": "identity"})
		return false
	}
	a.mu.Lock()
	if a.lcu != nil && a.lcu != client {
		a.mu.Unlock()
		return false
	}
	previous := a.summoner
	a.connected = true
	a.identityReady = result.Summoner.SummonerID != 0
	a.collectionRequested = false
	a.collectionRefreshPending = false
	a.lastAttempt = time.Now()
	a.lastDuration = time.Duration(result.LoadPhases["total"]) * time.Millisecond
	a.lastError = ""
	if gameplaySummonerChanged(previous, result.Summoner) {
		a.clearGameplayReferences()
	}
	a.summoner = result.Summoner
	a.summonerIdentityAt = a.lastAttempt
	if result.Account.Profile != (SummonerProfile{}) || result.Account.Capabilities != nil {
		if result.Account.Profile != (SummonerProfile{}) {
			a.account.Profile = result.Account.Profile
		}
		a.account.Capabilities = result.Account.Capabilities
	}
	a.masteries = result.Masteries
	a.masteryCapability = result.MasteryCapability
	a.lcu = client
	a.mu.Unlock()
	a.recordDiagnostic(map[string]any{"event": "identity_refresh_succeeded", "duration_ms": result.LoadPhases["total"], "load_phases_ms": result.LoadPhases, "phase_group": "identity"})
	a.broadcastEvent("summoner-updated")
	a.broadcastEvent("connection-state")
	if a.proRefreshContext != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_, _ = a.cachedFacadeState(ctx, false, "poll")
		}()
	}
	return true
}

func (a *app) requestCollectionRefresh() {
	a.mu.Lock()
	a.collectionRequested = true
	a.manualDisconnected = false
	coalesced := a.collectionRefreshPending
	if !coalesced && a.refreshRequests != nil {
		a.collectionRefreshPending = true
		select {
		case a.refreshRequests <- struct{}{}:
		default:
		}
	}
	a.mu.Unlock()
	a.recordDiagnostic(map[string]any{"event": "collection_refresh_request", "coalesced": coalesced})
}

func (a *app) refreshWithClient(client *LCUClient) bool {
	client.setDiagnosticObserver(a.recordDiagnostic)
	started := time.Now()
	a.mu.Lock()
	if a.syncing {
		a.mu.Unlock()
		a.recordDiagnostic(map[string]any{"event": "collection_refresh_lifecycle", "stage": "coalesced"})
		return true
	}
	a.syncing = true
	a.collectionRefreshPending = true
	generation := a.poolGeneration
	initialClient := a.lcu
	pool := a.pools[a.poolID]
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.collectionRefreshPending = false
		a.mu.Unlock()
	}()
	a.recordDiagnostic(map[string]any{"event": "collection_refresh_lifecycle", "stage": "begin"})
	a.broadcastEvent("refresh-started")

	result, err := loadSnapshotWithClientProvider(client, pool, a.champions, a.recordDiagnostic)
	clientAlive := false
	if err != nil {
		clientAlive = client.probe() == nil
	}
	var fallbackRecord SnapshotRecord
	hasFallback := false
	if err != nil && clientAlive && a.storage != nil {
		account := result.Summoner
		if account.PUUID == "" && account.SummonerID == 0 {
			a.mu.RLock()
			account = a.summoner
			a.mu.RUnlock()
		}
		poolForFallback := pool
		if account.PUUID != "" || account.SummonerID != 0 {
			fallbackRecord, hasFallback = a.storage.latestMatchingSnapshot(a.storage.accountHash(account), poolForFallback.ID, poolForFallback.Hash)
		}
	}
	a.mu.Lock()
	previousOwned := append([]Skin(nil), a.owned...)
	previousRemaining := append([]Skin(nil), a.remaining...)
	previousSnapshotAt := a.lastSync
	previousSnapshotUsable := a.snapshotReady && a.calculationOKLocked()
	if a.manualDisconnected || (a.lcu != initialClient && a.lcu != client) {
		a.syncing = false
		a.mu.Unlock()
		a.recordDiagnostic(map[string]any{"event": "collection_refresh_lifecycle", "stage": "canceled", "reason": "client-changed", "duration_ms": time.Since(started).Milliseconds()})
		return false
	}
	if generation != a.poolGeneration {
		a.syncing = false
		a.mu.Unlock()
		a.recordDiagnostic(map[string]any{"event": "collection_refresh_lifecycle", "stage": "canceled", "reason": "pool-changed", "duration_ms": time.Since(started).Milliseconds()})
		a.requestRefresh()
		return true
	}
	a.syncing = false
	a.lastAttempt = time.Now()
	a.lastDuration = time.Since(started)
	if err != nil {
		message := friendlyError(err)
		if a.snapshotRetryStarted.IsZero() {
			a.snapshotRetryStarted = a.lastAttempt
		}
		a.snapshotRetryCount++
		if a.snapshotRetryCount >= snapshotRetryFailureBudget || time.Since(a.snapshotRetryStarted) >= snapshotRetryBudgetDuration {
			a.snapshotRetryExhausted = true
		}
		if clientAlive {
			if result.Summoner.PUUID == "" && result.Summoner.SummonerID == 0 {
				result.Summoner = a.summoner
			}
			a.retainClientAfterSnapshotErrorLocked(client, result, message)
			if hasFallback {
				a.snapshotFallback = true
				a.snapshotFallbackAt = fallbackRecord.CapturedAt
				a.fallbackOwned = skinsFromSnapshot(fallbackRecord.Owned, true)
				a.fallbackRemaining = skinsFromSnapshot(fallbackRecord.Remaining, false)
			} else if previousSnapshotUsable && !previousSnapshotAt.IsZero() {
				a.snapshotFallback = true
				a.snapshotFallbackAt = previousSnapshotAt
				a.fallbackOwned = previousOwned
				a.fallbackRemaining = previousRemaining
			} else {
				a.snapshotFallback = false
				a.snapshotFallbackAt = time.Time{}
				a.fallbackOwned = nil
				a.fallbackRemaining = nil
			}
		} else {
			a.clearSnapshotLocked(message)
			a.ownership = append([]OwnershipSourceStatus(nil), result.Ownership...)
			a.catalog = result.Catalog
		}
		a.mu.Unlock()
		a.clearAssetCache()
		a.recordDiagnostic(map[string]any{"event": "collection_refresh_lifecycle", "stage": "failed", "error_kind": diagnosticErrorKind(err), "duration_ms": time.Since(started).Milliseconds()})
		a.recordDiagnostic(map[string]any{"event": "refresh_failed", "error": message, "client_alive": clientAlive, "duration_ms": time.Since(started).Milliseconds(), "load_phases_ms": result.LoadPhases, "phase_group": result.PhaseGroup, "pool_id": pool.ID, "pool_hash": pool.Hash, "catalog": result.Catalog, "ownership_sources": result.Ownership})
		a.broadcastEvent("refresh-failed")
		return clientAlive
	}
	a.lastSync = a.lastAttempt
	a.snapshotRetryCount = 0
	a.snapshotRetryStarted = time.Time{}
	a.snapshotRetryExhausted = false
	a.snapshotFallback = false
	a.snapshotFallbackAt = time.Time{}
	a.fallbackOwned = nil
	a.fallbackRemaining = nil
	a.connected = true
	a.identityReady = result.Summoner.SummonerID != 0
	a.snapshotReady = true
	a.collectionRequested = true
	a.clearCollectionDirtyThroughLocked(started)
	a.lastError = ""
	if gameplaySummonerChanged(a.summoner, result.Summoner) {
		a.clearGameplayReferences()
	}
	a.summoner = result.Summoner
	a.summonerIdentityAt = a.lastAttempt
	a.account = cloneAccountData(result.Account)
	a.masteries = result.Masteries
	a.masteryCapability = result.MasteryCapability
	a.allSkins = result.All
	a.allSkinsWithBase = result.AllWithBase
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
	a.recordDiagnostic(map[string]any{"event": "refresh_succeeded", "duration_ms": time.Since(started).Milliseconds(), "load_phases_ms": result.LoadPhases, "phase_group": result.PhaseGroup, "pool_id": pool.ID, "pool_hash": pool.Hash, "owned": len(result.Owned), "remaining": len(result.Remaining), "matched": result.PoolMatched, "catalog": result.Catalog, "ownership_sources": result.Ownership})
	if calculationOK && a.storage != nil {
		if _, saveErr := a.storage.saveSnapshot(savedSnapshot, pool); saveErr != nil {
			a.recordDiagnostic(map[string]any{"event": "snapshot_save_failed", "error": "local write failed"})
		}
	}
	a.scheduleCollectionDataRetry(client, result.Account)
	a.broadcastEvent("snapshot-updated")
	return true
}

func (a *app) refreshAccountWithClient(client *LCUClient) {
	client.setDiagnosticObserver(a.recordDiagnostic)
	a.mu.Lock()
	if a.syncing || !a.connected || a.lcu != client {
		a.mu.Unlock()
		return
	}
	a.syncing = true
	skins := append([]Skin(nil), a.allSkins...)
	a.mu.Unlock()
	profile, profileCapability := NewSummonerAPI(client).Profile()
	loot, lootCapability := NewObservedLootAPI(client, a.recordDiagnostic).PlayerLoot()
	metadataContext, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	metadata := loadLootMetadata(metadataContext, client, a.champions, a.recordDiagnostic)
	cancel()
	loot = enrichLootItemsWithMetadata(loot, skins, metadata, a.recordDiagnostic)
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

func (a *app) clearCollectionDirtyThroughLocked(refreshStartedAt time.Time) {
	if a.collectionDirtyAt.After(refreshStartedAt) {
		return
	}
	a.collectionDirty = false
	a.collectionDirtyAt = time.Time{}
}

func (a *app) clearSnapshotLocked(message string) {
	if a.collectionDataRetry != nil {
		a.collectionDataRetry.Stop()
		a.collectionDataRetry = nil
	}
	a.collectionDataRetryCount = 0
	a.collectionDataRetryClient = nil
	a.connected = false
	a.identityReady = false
	a.snapshotReady = false
	a.collectionRequested = false
	a.collectionRefreshPending = false
	a.lastError = message
	a.snapshotRetryCount = 0
	a.snapshotRetryStarted = time.Time{}
	a.snapshotRetryExhausted = false
	a.snapshotFallback = false
	a.snapshotFallbackAt = time.Time{}
	a.fallbackOwned = nil
	a.fallbackRemaining = nil
	a.summoner = Summoner{}
	a.summonerIdentityAt = time.Time{}
	a.account = AccountData{}
	a.masteries = nil
	a.masteryCapability = EndpointCapability{}
	a.allSkins = nil
	a.allSkinsWithBase = nil
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
	if result.Summoner.SummonerID == 0 {
		result.Summoner = a.summoner
	}
	a.connected = true
	a.identityReady = result.Summoner.SummonerID != 0
	a.snapshotReady = false
	a.collectionRequested = true
	a.lastError = message
	if gameplaySummonerChanged(a.summoner, result.Summoner) {
		a.clearGameplayReferences()
	}
	a.summoner = result.Summoner
	if result.Summoner.SummonerID != 0 {
		a.summonerIdentityAt = a.lastAttempt
	}
	profile := a.account.Profile
	a.account = AccountData{Profile: profile}
	if result.Account.Profile != (SummonerProfile{}) {
		a.account.Profile = result.Account.Profile
	}
	a.allSkins = nil
	a.allSkinsWithBase = nil
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

func skinsFromSnapshot(items []snapshotSkin, owned bool) []Skin {
	result := make([]Skin, 0, len(items))
	for _, item := range items {
		if item.ID <= 0 {
			continue
		}
		result = append(result, Skin{ID: item.ID, Name: item.Name, ChampionName: item.ChampionName, Rarity: item.Rarity, PoolName: item.PoolName, Owned: owned})
	}
	return result
}

func (a *app) clearAssetCache() {
	a.assetCacheMu.Lock()
	a.assetCache = make(map[string][]byte)
	a.assetCacheOrder = nil
	a.assetCacheBytes = 0
	a.assetCacheGeneration++
	a.assetFailureUntil = make(map[string]time.Time)
	a.assetHostTimeouts = make(map[string]int)
	a.assetHostBackoffUntil = make(map[string]time.Time)
	a.assetCacheMu.Unlock()
}

func (a *app) recordDiagnostic(event map[string]any) {
	if a.storage == nil {
		return
	}
	// Navigation cancellation is not an upstream failure. Keep it out of the
	// diagnostics stream even if a call site only retained the safe reason.
	for _, value := range event {
		if err, ok := value.(error); ok && isCancellation(err) {
			return
		}
		if text, ok := value.(string); ok && strings.EqualFold(strings.TrimSpace(text), context.Canceled.Error()) {
			return
		}
	}
	if eventName, _ := event["event"].(string); eventName == "ranked_winrate_resolved" {
		a.aggregateRankedWinrateDiagnostic(event, time.Now())
		return
	}
	if eventName, ok := event["event"].(string); ok && isNoisyDiagnosticEvent(eventName) {
		evicted := 0
		raw, _ := json.Marshal(event)
		key := eventName + "|" + string(raw)
		a.diagnosticDedupMu.Lock()
		if a.diagnosticDedupCounts == nil {
			a.diagnosticDedupCounts = make(map[string]int)
		}
		if _, exists := a.diagnosticDedupCounts[key]; !exists && len(a.diagnosticDedupCounts) >= diagnosticDeduplicationLimit {
			evicted = len(a.diagnosticDedupCounts)
			a.diagnosticDedupCounts = make(map[string]int)
		}
		a.diagnosticDedupCounts[key]++
		count := a.diagnosticDedupCounts[key]
		a.diagnosticDedupMu.Unlock()
		if evicted > 0 {
			a.appendDiagnosticEvent(map[string]any{"event": "diagnostic_dedup_reset", "keys_evicted": evicted})
		}
		if count > 1 {
			if count%100 != 0 {
				return
			}
			summary := map[string]any{"event": "diagnostic_dedup", "source_event": eventName, "repeat_count": count}
			a.appendDiagnosticEvent(summary)
			return
		}
	}
	a.appendDiagnosticEvent(event)
}

type rankedWinrateDiagnosticBucket struct {
	source          string
	windowStart     time.Time
	count           int
	completeCount   int
	suppressedCount int
}

func (a *app) aggregateRankedWinrateDiagnostic(event map[string]any, now time.Time) {
	source := strings.ToLower(strings.TrimSpace(fmt.Sprint(event["source"])))
	if source == "" {
		source = "unknown"
	}
	windowStart := now.UTC().Truncate(time.Minute)
	var completed *rankedWinrateDiagnosticBucket
	a.rankedWinrateDiagnosticMu.Lock()
	if a.rankedWinrateDiagnosticBuckets == nil {
		a.rankedWinrateDiagnosticBuckets = make(map[string]*rankedWinrateDiagnosticBucket)
	}
	bucket := a.rankedWinrateDiagnosticBuckets[source]
	if bucket != nil && !bucket.windowStart.Equal(windowStart) {
		copy := *bucket
		completed = &copy
		bucket = nil
	}
	if bucket == nil {
		bucket = &rankedWinrateDiagnosticBucket{source: source, windowStart: windowStart}
		a.rankedWinrateDiagnosticBuckets[source] = bucket
	}
	bucket.count++
	if complete, _ := event["complete"].(bool); complete {
		bucket.completeCount++
	}
	if suppressed, _ := event["suppressed"].(bool); suppressed {
		bucket.suppressedCount++
	}
	a.rankedWinrateDiagnosticMu.Unlock()
	if completed != nil {
		a.appendDiagnosticEvent(rankedWinrateDiagnosticEvent(completed))
	}
}

func rankedWinrateDiagnosticEvent(bucket *rankedWinrateDiagnosticBucket) map[string]any {
	return map[string]any{
		"event": "ranked_winrate_resolved", "source": bucket.source,
		"window_start": bucket.windowStart.Format(time.RFC3339), "count": bucket.count,
		"complete_count": bucket.completeCount, "suppressed_count": bucket.suppressedCount,
	}
}

func (a *app) flushRankedWinrateDiagnostics() {
	if a == nil || a.storage == nil {
		return
	}
	a.rankedWinrateDiagnosticMu.Lock()
	buckets := make([]*rankedWinrateDiagnosticBucket, 0, len(a.rankedWinrateDiagnosticBuckets))
	for _, bucket := range a.rankedWinrateDiagnosticBuckets {
		copy := *bucket
		buckets = append(buckets, &copy)
	}
	a.rankedWinrateDiagnosticBuckets = make(map[string]*rankedWinrateDiagnosticBucket)
	a.rankedWinrateDiagnosticMu.Unlock()
	for _, bucket := range buckets {
		a.appendDiagnosticEvent(rankedWinrateDiagnosticEvent(bucket))
	}
}

func (a *app) appendDiagnosticEvent(event map[string]any) {
	if a == nil || a.storage == nil {
		return
	}
	// Add write-failure context on a private copy; callers may reuse payloads.
	// The storage encoder supplies the timestamp and current build fingerprint.
	toWrite := make(map[string]any, len(event))
	for key, value := range event {
		toWrite[key] = value
	}
	a.diagnosticLogMu.RLock()
	pending := a.diagnosticWritePending
	observedFailures := a.diagnosticWriteFailures
	if a.diagnosticWriteFailures > 0 {
		toWrite["diagnostic_write_failures_total"] = a.diagnosticWriteFailures
		toWrite["diagnostic_write_pending"] = pending
		toWrite["diagnostic_write_last_kind"] = a.diagnosticWriteLastKind
	}
	a.diagnosticLogMu.RUnlock()
	err := a.storage.appendDiagnostic(toWrite)
	a.diagnosticLogMu.Lock()
	if err != nil {
		a.diagnosticLogErr = "诊断日志写入失败"
		a.diagnosticWriteFailures++
		a.diagnosticWritePending++
		a.diagnosticWriteLastKind = diagnosticErrorKind(err)
	} else {
		a.diagnosticWriteRecovered = max(observedFailures, a.diagnosticWriteRecovered)
		a.diagnosticWritePending = a.diagnosticWriteFailures - a.diagnosticWriteRecovered
		if a.diagnosticWritePending == 0 {
			a.diagnosticLogErr = ""
		}
	}
	a.diagnosticLogMu.Unlock()
}

func isNoisyDiagnosticEvent(event string) bool {
	switch event {
	case "pro_identity_match", "ranked_winrate_resolved", "ranked_data_source_decision", "specialist_runes_client_skip", "client_installations_scan", "lcu_discovery", "live_position_shape", "social_presence_resolved":
		return true
	default:
		return false
	}
}

func (a *app) resetDiagnosticDeduplication() {
	if a == nil {
		return
	}
	a.recommendationModeDiagnosticMu.Lock()
	a.recommendationModeDiagnosticKeys = make(map[string]struct{})
	a.recommendationModeDiagnosticMu.Unlock()
	a.championDataDiagnosticMu.Lock()
	a.championDataDiagnosticKeys = make(map[string]struct{})
	a.championDataDiagnosticMu.Unlock()
	a.matchModeDiagnosticMu.Lock()
	a.matchModeDiagnosticKeys = make(map[int64]struct{})
	a.matchModeDiagnosticMu.Unlock()
	a.liveRosterDiagnosticMu.Lock()
	a.liveRosterDiagnosticKeys = make(map[string]struct{})
	a.liveRosterDiagnosticMu.Unlock()
	a.liveClientPlayerListDiagnosticMu.Lock()
	a.liveClientPlayerListDiagnosticKeys = make(map[string]struct{})
	a.liveClientPlayerListDiagnosticMu.Unlock()
	a.unknownQueueDiagnosticMu.Lock()
	a.unknownQueueDiagnosticIDs = make(map[int64]struct{})
	a.unknownQueueDiagnosticMu.Unlock()
	a.lcuGameflowShapeDiagnosticMu.Lock()
	a.lcuGameflowShapeDiagnosticKeys = make(map[string]struct{})
	a.lcuGameflowShapeDiagnosticMu.Unlock()
	a.lcuChampSelectShapeDiagnosticMu.Lock()
	a.lcuChampSelectShapeDiagnosticKeys = make(map[string]struct{})
	a.lcuChampSelectShapeDiagnosticMu.Unlock()
	a.diagnosticDedupMu.Lock()
	a.diagnosticDedupCounts = make(map[string]int)
	a.diagnosticDedupMu.Unlock()
}

func (a *app) recordAppStartDiagnostic(rotation ...bool) {
	a.recordDiagnostic(map[string]any{
		"event": "app_start", "version": version,
		"build_fingerprint": buildFingerprint, "riot_key": riotKeyConfigured(),
		"log_rotation": len(rotation) > 0 && rotation[0],
	})
}

func (a *app) enableDiagnosticRotationSnapshot() {
	if a == nil || a.storage == nil {
		return
	}
	a.storage.onDiagnosticRotation = func() {
		a.resetDiagnosticDeduplication()
		a.recordAppStartDiagnostic(true)
	}
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
		Summoner: a.summoner, All: append([]Skin(nil), a.allSkins...), AllWithBase: append([]Skin(nil), a.allSkinsWithBase...), Owned: append([]Skin(nil), a.owned...),
		Remaining: append([]Skin(nil), a.remaining...), PoolTotal: a.poolTotal, PoolMatched: a.poolMatched,
		Issues: append([]PoolIssue(nil), a.poolIssues...), Client: a.lcu,
		Ownership: append([]OwnershipSourceStatus(nil), a.ownership...), Catalog: a.catalog,
		Chromas: append([]Chroma(nil), a.chromas...), ChromaState: a.chromaState,
		Masteries: a.masteries, MasteryCapability: a.masteryCapability,
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
	if err := atomicWriteFile(path, []byte(token+"\n"), 0o600); err != nil {
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
	if errors.Is(err, errCollectionNotSynced) {
		return errCollectionNotSynced.Error()
	}
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

var errResponseLimitExceeded = errors.New("response size limit exceeded")

func readLimited(body io.Reader, limit int64) ([]byte, error) {
	if limit < 0 {
		return nil, errors.New("invalid response limit")
	}
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes: %w", limit, errResponseLimitExceeded)
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

func closeDiagnosticStore(store *localStore) {
	if err := store.Close(); err != nil {
		log.Print("诊断日志关闭失败，最后一批事件可能未写入")
	}
}
