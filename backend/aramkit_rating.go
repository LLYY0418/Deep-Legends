package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	aramkitRatingHost          = "api.aramkit.com"
	aramkitRatingCNHost        = "api.aramkit.cn"
	aramkitRatingPath          = "/rating"
	aramkitRatingResponseMax   = 2 << 20
	aramkitRatingCacheLimit    = 128
	aramkitRatingSuccessTTL    = 24 * time.Hour
	aramkitRatingFailureTTL    = 2 * time.Minute
	aramkitRatingRequestDelay  = 300 * time.Millisecond
	aramkitRatingRequestJitter = 100 * time.Millisecond
	aramkitRatingTimeout       = 25 * time.Second
	aramkitRatingBodyTimeout   = 8 * time.Second
	aramkitRatingTotalTimeout  = 25 * time.Second
	aramkitDiagnosticDomain    = "aramkit-rating-player-v1"
)

type aramkitRatingEnvelope struct {
	Code    int                `json:"code"`
	Message string             `json:"message"`
	Data    *aramkitRatingData `json:"data"`
}

type aramkitRatingData struct {
	GameName           string               `json:"gameName"`
	TagLine            string               `json:"tagLine"`
	PlatformID         string               `json:"platformId"`
	Rating             int                  `json:"rating"`
	RatingType         int                  `json:"ratingType"`
	MarginOfError      int                  `json:"marginOfError"`
	PerformanceScore   int                  `json:"performanceScore"`
	PerformanceComment []string             `json:"performanceComment"`
	Matches            []aramkitRatingMatch `json:"matches"`
}

type aramkitRatingMatch struct {
	GameCreationTime string                     `json:"gameCreationTime"`
	MatchResult      int                        `json:"matchResult"`
	ChampionID       int                        `json:"championId"`
	DurationSeconds  int                        `json:"durationSeconds"`
	Kills            int                        `json:"kills"`
	Deaths           int                        `json:"deaths"`
	Assists          int                        `json:"assists"`
	AverageRating    int                        `json:"avgRating"`
	BlueAverage      int                        `json:"blueAvgRating"`
	RedAverage       int                        `json:"redAvgRating"`
	Participants     []aramkitRatingParticipant `json:"participants"`
}

type aramkitRatingParticipant struct {
	GameName          string  `json:"gameName"`
	TagLine           string  `json:"tagLine"`
	TeamID            int     `json:"teamId"`
	IsSelf            bool    `json:"isSelf"`
	ChampionID        int     `json:"championId"`
	Kills             int     `json:"kills"`
	Deaths            int     `json:"deaths"`
	Assists           int     `json:"assists"`
	KillParticipation float64 `json:"killParticipation"`
	Rating            int     `json:"rating"`
	PerformanceScore  int     `json:"performanceScore"`
}

// mayhemRatingMatch intentionally omits participant identities. The overview
// only needs enough context to explain the estimate; third-party roster data
// is neither persisted nor returned to the renderer.
type mayhemRatingMatch struct {
	GameCreationTime string `json:"gameCreationTime,omitempty"`
	MatchResult      int    `json:"matchResult,omitempty"`
	ChampionID       int    `json:"championId,omitempty"`
	DurationSeconds  int    `json:"durationSeconds,omitempty"`
	Kills            int    `json:"kills,omitempty"`
	Deaths           int    `json:"deaths,omitempty"`
	Assists          int    `json:"assists,omitempty"`
	AverageRating    int    `json:"avgRating,omitempty"`
	BlueAverage      int    `json:"blueAvgRating,omitempty"`
	RedAverage       int    `json:"redAvgRating,omitempty"`
}

type mayhemRatingResponse struct {
	Available         bool                `json:"available"`
	Source            string              `json:"source"`
	Rating            int                 `json:"rating,omitempty"`
	RatingType        int                 `json:"ratingType"`
	MarginOfError     int                 `json:"marginOfError,omitempty"`
	PlatformID        string              `json:"platformId,omitempty"`
	FetchedAt         time.Time           `json:"fetchedAt"`
	UnavailableReason string              `json:"unavailableReason,omitempty"`
	Matches           []mayhemRatingMatch `json:"matches,omitempty"`
	Sources           []DataSourceAttempt `json:"sources"`
}

type aramkitRatingOutcome struct {
	host              string
	dns, connect, tls time.Duration
	ttfb              time.Duration
	bytes             int
	response          mayhemRatingResponse
	code              int
	httpStatus        int
	result            string
	reason            string
}

type aramkitRatingCacheEntry struct {
	outcome   aramkitRatingOutcome
	expiresAt time.Time
}

type aramkitRatingFlight struct {
	done    chan struct{}
	outcome aramkitRatingOutcome
}

type aramkitRatingPrefetchKey struct{}
type aramkitRatingClient struct {
	disk        *championDataCache
	refreshes   map[string]bool
	mu          sync.Mutex
	cache       map[string]aramkitRatingCacheEntry
	flights     map[string]*aramkitRatingFlight
	paceMu      sync.Mutex
	lastRequest time.Time

	featureGates  *featureGates
	httpClient    func() *http.Client
	observe       func(map[string]any)
	identityHash  func(string) string
	now           func() time.Time
	sleep         func(context.Context, time.Duration) error
	jitter        func(time.Duration) time.Duration
	baseURL       string
	fallbackURL   string
	hedgeDelay    time.Duration
	bodyTimeout   time.Duration
	totalTimeout  time.Duration
	timeout       time.Duration
	minimumDelay  time.Duration
	maximumJitter time.Duration
}

func newAramkitRatingClient(provider *championProvider, observe func(map[string]any), identityHash func(string) string) *aramkitRatingClient {
	client := &aramkitRatingClient{
		cache:         make(map[string]aramkitRatingCacheEntry),
		refreshes:     make(map[string]bool),
		flights:       make(map[string]*aramkitRatingFlight),
		featureGates:  provider.featureGates,
		httpClient:    provider.httpClient,
		observe:       observe,
		identityHash:  identityHash,
		now:           time.Now,
		sleep:         sleepWithContext,
		jitter:        aramkitRandomJitter,
		baseURL:       "https://" + aramkitRatingCNHost,
		fallbackURL:   "https://" + aramkitRatingHost,
		hedgeDelay:    3 * time.Second,
		timeout:       aramkitRatingTimeout,
		totalTimeout:  aramkitRatingTotalTimeout,
		bodyTimeout:   aramkitRatingBodyTimeout,
		minimumDelay:  aramkitRatingRequestDelay,
		maximumJitter: aramkitRatingRequestJitter,
	}
	if provider.cache != nil && provider.cache.dir != "" {
		client.disk = newPublicBinaryCache(&localStore{root: filepath.Dir(provider.cache.dir)}, "mayhem-ratings", 128, 4<<20)
	}
	return client
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func aramkitRandomJitter(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return 0
	}
	return time.Duration(binary.LittleEndian.Uint64(raw[:]) % uint64(max+1))
}

func normalizeAramkitIdentity(gameName, tagLine string) (string, string, string, error) {
	gameName = strings.TrimSpace(gameName)
	tagLine = strings.TrimSpace(tagLine)
	if gameName == "" || tagLine == "" || strings.Contains(tagLine, "#") || len([]rune(gameName)) > 64 || len([]rune(tagLine)) > 16 {
		return "", "", "", errors.New("invalid player identity")
	}
	for _, value := range gameName + tagLine {
		if unicode.IsControl(value) {
			return "", "", "", errors.New("invalid player identity")
		}
	}
	key := strings.ToLower(gameName) + "|" + strings.ToLower(tagLine)
	return gameName, tagLine, key, nil
}

func (c *aramkitRatingClient) lookup(ctx context.Context, gameName, tagLine string) mayhemRatingResponse {
	gameName, tagLine, key, err := normalizeAramkitIdentity(gameName, tagLine)
	if err != nil {
		return unavailableMayhemRating("查询身份无效", dataSourceFailed)
	}
	if c == nil || c.featureGates == nil || !c.featureGates.enabled(featureGateAramkit) {
		response := unavailableMayhemRating("功能暂不可用", dataSourceDisabled)
		c.recordContext(ctx, key, false, false, 0, aramkitRatingOutcome{response: response, result: "disabled", reason: "feature-disabled"})
		return response
	}

	started := c.now()
	diskKey := riotIdentityKey("mayhem:" + key)
	source := "memory"
	c.mu.Lock()
	_, present := c.cache[key]
	c.mu.Unlock()
	if !present && c.disk != nil {
		if entry, e := c.disk.readDisk(diskKey); e == nil {
			var response mayhemRatingResponse
			if json.Unmarshal(entry.Data, &response) == nil && (response.Available || response.UnavailableReason == "未收录") {
				c.mu.Lock()
				c.cache[key] = aramkitRatingCacheEntry{aramkitRatingOutcome{response: response, result: "disk"}, entry.ExpiresAt}
				c.mu.Unlock()
				source = "disk"
			}
		}
	}
	ctx = context.WithValue(ctx, aramkitRatingCacheSourceKey{}, source)
	c.mu.Lock()
	if cached, ok := c.cache[key]; ok && (started.Before(cached.expiresAt) || cached.outcome.response.Available) && ctx.Value(aramkitRatingRefreshKey{}) != true {
		if c.refreshes == nil {
			c.refreshes = make(map[string]bool)
		}
		refresh := cached.outcome.response.Available && !started.Before(cached.expiresAt) && !c.refreshes[key] && len(c.refreshes) < 128
		if refresh {
			c.refreshes[key] = true
		}
		c.mu.Unlock()
		c.recordContext(ctx, key, true, false, c.now().Sub(started), cached.outcome)
		if refresh {
			go func() {
				defer recoverPanic("aramkit.refresh")
				defer func() { c.mu.Lock(); delete(c.refreshes, key); c.mu.Unlock() }()
				background, cancel := context.WithTimeout(context.WithoutCancel(ctx), aramkitRatingTotalTimeout)
				defer cancel()
				c.lookup(context.WithValue(background, aramkitRatingRefreshKey{}, true), gameName, tagLine)
			}()
		}
		return cached.outcome.response
	}
	if flight := c.flights[key]; flight != nil {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return unavailableMayhemRating("查询已取消", dataSourceFailed)
		case <-flight.done:
			c.recordContext(ctx, key, true, true, c.now().Sub(started), flight.outcome)
			return flight.outcome.response
		}
	}
	flight := &aramkitRatingFlight{done: make(chan struct{})}
	c.flights[key] = flight
	c.mu.Unlock()

	outcome := c.fetch(ctx, gameName, tagLine)
	flight.outcome = outcome
	ttl := aramkitRatingFailureTTL
	if outcome.reason == "cancelled" {
		ttl = 0
	}
	if outcome.response.Available {
		ttl = aramkitRatingSuccessTTL
	}
	c.mu.Lock()
	delete(c.flights, key)
	c.pruneCacheLocked(c.now())
	if ttl > 0 {
		if prior, ok := c.cache[key]; ok && prior.outcome.response.Available && !outcome.response.Available {
			prior.expiresAt = c.now().Add(ttl)
			c.cache[key] = prior
		} else {
			c.cache[key] = aramkitRatingCacheEntry{outcome: outcome, expiresAt: c.now().Add(ttl)}
		}
	}
	close(flight.done)
	c.mu.Unlock()
	if (outcome.response.Available || outcome.response.UnavailableReason == "未收录") && c.disk != nil {
		data, _ := json.Marshal(outcome.response)
		sum := sha256.Sum256(data)
		now := c.now()
		c.disk.enqueueDiskWrite(championCacheEnvelope{Schema: championCacheSchema, Key: diskKey, FetchedAt: now, ExpiresAt: now.Add(ttl), StaleUntil: now.Add(ttl), Hash: hex.EncodeToString(sum[:]), Data: data})
	}
	c.recordContext(ctx, key, false, false, c.now().Sub(started), outcome)
	return outcome.response
}

func (c *aramkitRatingClient) pruneCacheLocked(now time.Time) {
	for key, entry := range c.cache {
		if !now.Before(entry.expiresAt) && !entry.outcome.response.Available {
			delete(c.cache, key)
		}
	}
	for len(c.cache) >= aramkitRatingCacheLimit {
		var oldestKey string
		var oldest time.Time
		for key, entry := range c.cache {
			if oldestKey == "" || entry.expiresAt.Before(oldest) {
				oldestKey, oldest = key, entry.expiresAt
			}
		}
		delete(c.cache, oldestKey)
	}
}

func (c *aramkitRatingClient) waitForPace(ctx context.Context) error {
	c.paceMu.Lock()
	defer c.paceMu.Unlock()
	now := c.now()
	delay := c.minimumDelay + c.jitter(c.maximumJitter) - now.Sub(c.lastRequest)
	if !c.lastRequest.IsZero() && delay > 0 {
		if err := c.sleep(ctx, delay); err != nil {
			return err
		}
	}
	c.lastRequest = c.now()
	return nil
}

func (c *aramkitRatingClient) fetchEndpoint(ctx context.Context, baseURL, gameName, tagLine string) (outcome aramkitRatingOutcome) {
	trace := newARAMKitRatingTrace()
	defer func() {
		outcome.dns, outcome.connect, outcome.tls, outcome.ttfb = trace.snapshot(outcome.ttfb)
		if u, e := url.Parse(baseURL); e == nil {
			outcome.host = u.Hostname()
		}
	}()
	var ttfb time.Duration
	var received bytes.Buffer
	started := time.Now()
	failed := func(reason, message string, code, status int) aramkitRatingOutcome {
		response := unavailableMayhemRating(message, dataSourceFailed)
		response.FetchedAt = c.now().UTC()
		return aramkitRatingOutcome{response: response, code: code, httpStatus: status, result: "unavailable", reason: reason, ttfb: ttfb, bytes: received.Len()}
	}

	total := c.totalTimeout
	if total <= 0 {
		total = aramkitRatingTotalTimeout
	}
	requestContext, cancel := context.WithTimeout(ctx, total)
	defer cancel()
	timer := time.AfterFunc(c.timeout, cancel)
	defer timer.Stop()
	endpoint, err := url.Parse(baseURL)
	if err != nil {
		return failed("invalid-endpoint", "暂不可用", 0, 0)
	}
	endpoint.Path = aramkitRatingPath
	query := endpoint.Query()
	query.Set("gameName", gameName)
	query.Set("tagLine", tagLine)
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(httptrace.WithClientTrace(requestContext, trace.hooks()), http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return failed("request-build", "暂不可用", 0, 0)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Accept-Encoding", "gzip")
	request.Header.Set("User-Agent", "Deep-Legends/"+version)
	baseClient := c.httpClient()
	if baseClient == nil {
		baseClient = http.DefaultClient
	}
	client := *baseClient
	client.Timeout = 0
	response, err := client.Do(request)
	if err != nil {
		reason := "network"
		if errors.Is(err, context.DeadlineExceeded) || requestContext.Err() != nil && ctx.Err() == nil {
			reason = "timeout"
		}
		return failed(reason, "暂不可用", 0, 0)
	}
	defer response.Body.Close()
	ttfb = time.Since(started)
	if !timer.Stop() {
		return failed("timeout", "暂不可用", 0, response.StatusCode)
	}
	bodyTimeout := c.bodyTimeout
	if bodyTimeout <= 0 {
		bodyTimeout = aramkitRatingBodyTimeout
	}
	timer.Reset(bodyTimeout)
	var reader io.Reader = &aramkitIdleReader{Reader: io.TeeReader(response.Body, &received), timer: timer, idle: bodyTimeout}
	if response.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(reader)
		if err != nil {
			return failed("response-read", "暂不可用", 0, response.StatusCode)
		}
		defer gz.Close()
		reader = gz
	}
	data, readErr := readLimited(reader, aramkitRatingResponseMax)
	if readErr != nil {
		reason := "response-read"
		if requestContext.Err() != nil && ctx.Err() == nil {
			reason = "timeout"
		}
		return failed(reason, "暂不可用", 0, response.StatusCode)
	}
	if response.StatusCode != http.StatusOK {
		return failed("http-status", "暂不可用", 0, response.StatusCode)
	}
	var envelope aramkitRatingEnvelope
	if json.Unmarshal(data, &envelope) != nil {
		return failed("invalid-json", "暂不可用", 0, response.StatusCode)
	}
	if envelope.Code == http.StatusBadRequest {
		return failed("not-recorded", "未收录", envelope.Code, response.StatusCode)
	}
	if envelope.Code != http.StatusOK || envelope.Data == nil || envelope.Data.Rating <= 0 {
		return failed("invalid-data", "暂不可用", envelope.Code, response.StatusCode)
	}
	result := mayhemRatingResponse{
		Available:     true,
		Source:        dataSourceARAMKit,
		Rating:        envelope.Data.Rating,
		RatingType:    envelope.Data.RatingType,
		MarginOfError: envelope.Data.MarginOfError,
		PlatformID:    strings.TrimSpace(envelope.Data.PlatformID),
		FetchedAt:     c.now().UTC(),
		Sources:       []DataSourceAttempt{{Source: dataSourceARAMKit, Outcome: dataSourceSuccess}},
		Matches:       make([]mayhemRatingMatch, 0, len(envelope.Data.Matches)),
	}
	for _, match := range envelope.Data.Matches {
		result.Matches = append(result.Matches, mayhemRatingMatch{
			GameCreationTime: match.GameCreationTime, MatchResult: match.MatchResult, ChampionID: match.ChampionID,
			DurationSeconds: match.DurationSeconds, Kills: match.Kills, Deaths: match.Deaths, Assists: match.Assists,
			AverageRating: match.AverageRating, BlueAverage: match.BlueAverage, RedAverage: match.RedAverage,
		})
	}
	return aramkitRatingOutcome{response: result, code: envelope.Code, httpStatus: response.StatusCode, result: "success", ttfb: ttfb, bytes: received.Len()}
}

func unavailableMayhemRating(message, outcome string) mayhemRatingResponse {
	return mayhemRatingResponse{
		Available: false, Source: dataSourceARAMKit, RatingType: -1, FetchedAt: time.Now().UTC(),
		UnavailableReason: message, Sources: []DataSourceAttempt{{Source: dataSourceARAMKit, Outcome: outcome, Message: message}},
	}
}

type aramkitIdleReader struct {
	io.Reader
	timer *time.Timer
	idle  time.Duration
}

func (r *aramkitIdleReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if n > 0 {
		r.timer.Reset(r.idle)
	}
	return n, err
}

type aramkitRatingCacheSourceKey struct{}
type aramkitRatingRefreshKey struct{}

func (c *aramkitRatingClient) recordContext(ctx context.Context, key string, cacheHit, coalesced bool, duration time.Duration, outcome aramkitRatingOutcome) {
	if c == nil || c.observe == nil || outcome.reason == "cancelled" {
		return
	}
	identity := ""
	if c.identityHash != nil {
		identity = c.identityHash(key)
	}
	c.observe(map[string]any{
		"event": "mayhem_rating_lookup", "source": dataSourceARAMKit, "player_hash": identity,
		"cache_hit": cacheHit, "cache_source": ctx.Value(aramkitRatingCacheSourceKey{}), "prefetch": ctx.Value(aramkitRatingPrefetchKey{}) == true, "coalesced": coalesced, "duration_ms": duration.Milliseconds(),
		"host": outcome.host, "dns_ms": outcome.dns.Milliseconds(), "connect_ms": outcome.connect.Milliseconds(), "tls_ms": outcome.tls.Milliseconds(),
		"ttfb_ms": outcome.ttfb.Milliseconds(), "bytes": outcome.bytes,
		"code": outcome.code, "http_status": outcome.httpStatus, "result": outcome.result, "reason": outcome.reason,
	})
}

func (a *app) aramkitRatingIdentityHash(identity string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(aramkitDiagnosticDomain))
	_, _ = hash.Write([]byte{0})
	if a != nil && a.storage != nil && len(a.storage.salt) > 0 {
		_, _ = hash.Write(a.storage.salt)
	} else {
		_, _ = hash.Write([]byte("no-install-salt"))
	}
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(strings.ToLower(strings.TrimSpace(identity))))
	digest := hash.Sum(nil)
	return hex.EncodeToString(digest[:8])
}

func (a *app) handleGameplayMayhemRating(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if len(query["playerRef"]) > 1 || len(query["gameName"]) > 1 || len(query["tagLine"]) > 1 {
		http.Error(w, "invalid player query", http.StatusBadRequest)
		return
	}
	gameName := strings.TrimSpace(query.Get("gameName"))
	a.mu.RLock()
	client := a.lcu
	a.mu.RUnlock()
	if !isTencentClient(client) {
		respondJSON(w, unavailableMayhemRating("", dataSourceModeUnsupported))
		return
	}
	knownTencent := clientTencentServerID(client) != ""
	tagLine := strings.TrimSpace(query.Get("tagLine"))
	if publicRef := strings.TrimSpace(query.Get("playerRef")); publicRef != "" {
		reference, ok := a.resolveGameplayReferenceDetails(publicRef)
		if !ok {
			http.Error(w, "player reference expired", http.StatusNotFound)
			return
		}
		if isRiotRegion(reference.Region) {
			response := unavailableMayhemRating("仅支持国服", dataSourceModeUnsupported)
			respondJSON(w, response)
			return
		}
		knownTencent = reference.ServerID != ""
		gameName, tagLine = reference.GameName, reference.TagLine
	}
	if !knownTencent {
		respondJSON(w, unavailableMayhemRating("", dataSourceModeUnsupported))
		return
	}
	if _, _, _, err := normalizeAramkitIdentity(gameName, tagLine); err != nil {
		http.Error(w, "player identity unavailable", http.StatusBadRequest)
		return
	}
	if a.mayhemRatings == nil {
		http.Error(w, "rating service unavailable", http.StatusServiceUnavailable)
		return
	}
	ctx := context.WithValue(r.Context(), aramkitRatingPrefetchKey{}, query.Get("prefetch") == "1")
	if query.Get("retry") == "1" {
		ctx = context.WithValue(ctx, aramkitRatingRefreshKey{}, true)
	}
	respondJSON(w, a.mayhemRatings.lookup(ctx, gameName, tagLine))
}

// Only the primary starts immediately. The alternate begins after three seconds,
// or immediately after a failed primary. Both share one bounded lookup context.
func (c *aramkitRatingClient) fetch(ctx context.Context, gameName, tagLine string) aramkitRatingOutcome {
	if e := c.waitForPace(ctx); e != nil {
		return aramkitRatingOutcome{reason: "cancelled", result: "unavailable", response: unavailableMayhemRating("查询已取消", dataSourceFailed)}
	}
	parent := ctx
	total := c.totalTimeout
	if total <= 0 || total > 25*time.Second {
		total = 25 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, total)
	defer cancel()
	fallback := c.fallbackURL
	// Existing injected endpoints remain self contained; tests can supply two local endpoints.
	if c.baseURL != "https://"+aramkitRatingCNHost && fallback == "https://"+aramkitRatingHost {
		fallback = ""
	}
	if fallback == "" || fallback == c.baseURL {
		return c.fetchEndpoint(ctx, c.baseURL, gameName, tagLine)
	}
	results := make(chan aramkitRatingOutcome, 2)
	launch := func(endpoint string) {
		go func() {
			outcome := aramkitRatingOutcome{reason: "network_error", result: "unavailable", response: unavailableMayhemRating("暂不可用", dataSourceFailed)}
			defer func() { results <- outcome }()
			defer recoverPanic("aramkit.rating.endpoint")
			outcome = c.fetchEndpoint(ctx, endpoint, gameName, tagLine)
		}()
	}
	launch(c.baseURL)
	delay := c.hedgeDelay
	if delay <= 0 {
		delay = 3 * time.Second
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	alternate := false
	completed := 0
	var failure aramkitRatingOutcome
	for {
		select {
		case result := <-results:
			completed++
			if result.response.Available {
				cancel()
				return result
			}
			failure = result
			if !alternate {
				alternate = true
				timer.Stop()
				launch(fallback)
			}
			if completed == 2 {
				return failure
			}
		case <-timer.C:
			if !alternate {
				alternate = true
				launch(fallback)
			}
		case <-ctx.Done():
			if parent.Err() != nil {
				return aramkitRatingOutcome{reason: "cancelled", result: "unavailable", response: unavailableMayhemRating("查询已取消", dataSourceFailed)}
			}
			if failure.reason != "" {
				return failure
			}
			return aramkitRatingOutcome{reason: "timeout", result: "unavailable", response: unavailableMayhemRating("暂不可用", dataSourceFailed)}
		}
	}
}

type aramkitRatingTrace struct {
	mu                          sync.Mutex
	started, dnsStart, tlsStart time.Time
	connectStarts               map[string]time.Time
	dns, connect, tls, ttfb     time.Duration
}

func newARAMKitRatingTrace() *aramkitRatingTrace {
	return &aramkitRatingTrace{started: time.Now(), connectStarts: map[string]time.Time{}}
}
func (t *aramkitRatingTrace) hooks() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) { t.mu.Lock(); t.dnsStart = time.Now(); t.mu.Unlock() }, DNSDone: func(httptrace.DNSDoneInfo) {
			t.mu.Lock()
			if !t.dnsStart.IsZero() {
				t.dns += time.Since(t.dnsStart)
			}
			t.mu.Unlock()
		},
		ConnectStart: func(network, addr string) { t.mu.Lock(); t.connectStarts[network+addr] = time.Now(); t.mu.Unlock() }, ConnectDone: func(network, addr string, _ error) {
			t.mu.Lock()
			if start, ok := t.connectStarts[network+addr]; ok {
				t.connect += time.Since(start)
				delete(t.connectStarts, network+addr)
			}
			t.mu.Unlock()
		},
		TLSHandshakeStart: func() { t.mu.Lock(); t.tlsStart = time.Now(); t.mu.Unlock() }, TLSHandshakeDone: func(tls.ConnectionState, error) {
			t.mu.Lock()
			if !t.tlsStart.IsZero() {
				t.tls += time.Since(t.tlsStart)
			}
			t.mu.Unlock()
		},
		GotFirstResponseByte: func() { t.mu.Lock(); t.ttfb = time.Since(t.started); t.mu.Unlock() },
	}
}
func (t *aramkitRatingTrace) snapshot(fallback time.Duration) (time.Duration, time.Duration, time.Duration, time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	ttfb := t.ttfb
	if ttfb == 0 {
		ttfb = fallback
	}
	return t.dns, t.connect, t.tls, ttfb
}
