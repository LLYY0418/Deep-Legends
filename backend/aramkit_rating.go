package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	aramkitRatingHost          = "api.aramkit.com"
	aramkitRatingPath          = "/rating"
	aramkitRatingResponseMax   = 2 << 20
	aramkitRatingCacheLimit    = 128
	aramkitRatingSuccessTTL    = 10 * time.Minute
	aramkitRatingFailureTTL    = 5 * time.Minute
	aramkitRatingRequestDelay  = 300 * time.Millisecond
	aramkitRatingRequestJitter = 100 * time.Millisecond
	aramkitRatingTimeout       = 9 * time.Second
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
	response   mayhemRatingResponse
	code       int
	httpStatus int
	result     string
	reason     string
}

type aramkitRatingCacheEntry struct {
	outcome   aramkitRatingOutcome
	expiresAt time.Time
}

type aramkitRatingFlight struct {
	done    chan struct{}
	outcome aramkitRatingOutcome
}

type aramkitRatingClient struct {
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
	timeout       time.Duration
	minimumDelay  time.Duration
	maximumJitter time.Duration
}

func newAramkitRatingClient(provider *championProvider, observe func(map[string]any), identityHash func(string) string) *aramkitRatingClient {
	client := &aramkitRatingClient{
		cache:         make(map[string]aramkitRatingCacheEntry),
		flights:       make(map[string]*aramkitRatingFlight),
		featureGates:  provider.featureGates,
		httpClient:    provider.httpClient,
		observe:       observe,
		identityHash:  identityHash,
		now:           time.Now,
		sleep:         sleepWithContext,
		jitter:        aramkitRandomJitter,
		baseURL:       "https://" + aramkitRatingHost,
		timeout:       aramkitRatingTimeout,
		minimumDelay:  aramkitRatingRequestDelay,
		maximumJitter: aramkitRatingRequestJitter,
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
		c.record(key, false, false, 0, aramkitRatingOutcome{response: response, result: "disabled", reason: "feature-disabled"})
		return response
	}

	started := c.now()
	c.mu.Lock()
	if cached, ok := c.cache[key]; ok && started.Before(cached.expiresAt) {
		c.mu.Unlock()
		c.record(key, true, false, c.now().Sub(started), cached.outcome)
		return cached.outcome.response
	}
	if flight := c.flights[key]; flight != nil {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return unavailableMayhemRating("查询已取消", dataSourceFailed)
		case <-flight.done:
			c.record(key, true, true, c.now().Sub(started), flight.outcome)
			return flight.outcome.response
		}
	}
	flight := &aramkitRatingFlight{done: make(chan struct{})}
	c.flights[key] = flight
	c.mu.Unlock()

	outcome := c.fetch(ctx, gameName, tagLine)
	flight.outcome = outcome
	ttl := aramkitRatingFailureTTL
	if outcome.response.Available {
		ttl = aramkitRatingSuccessTTL
	}
	c.mu.Lock()
	delete(c.flights, key)
	c.pruneCacheLocked(c.now())
	c.cache[key] = aramkitRatingCacheEntry{outcome: outcome, expiresAt: c.now().Add(ttl)}
	close(flight.done)
	c.mu.Unlock()
	c.record(key, false, false, c.now().Sub(started), outcome)
	return outcome.response
}

func (c *aramkitRatingClient) pruneCacheLocked(now time.Time) {
	for key, entry := range c.cache {
		if !now.Before(entry.expiresAt) {
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

func (c *aramkitRatingClient) fetch(ctx context.Context, gameName, tagLine string) aramkitRatingOutcome {
	failed := func(reason, message string, code, status int) aramkitRatingOutcome {
		response := unavailableMayhemRating(message, dataSourceFailed)
		response.FetchedAt = c.now().UTC()
		return aramkitRatingOutcome{response: response, code: code, httpStatus: status, result: "unavailable", reason: reason}
	}
	if err := c.waitForPace(ctx); err != nil {
		return failed("cancelled", "查询已取消", 0, 0)
	}
	requestContext, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	endpoint, err := url.Parse(c.baseURL)
	if err != nil {
		return failed("invalid-endpoint", "暂不可用", 0, 0)
	}
	endpoint.Path = aramkitRatingPath
	query := endpoint.Query()
	query.Set("gameName", gameName)
	query.Set("tagLine", tagLine)
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return failed("request-build", "暂不可用", 0, 0)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Deep-Legends/"+version)
	baseClient := c.httpClient()
	if baseClient == nil {
		baseClient = http.DefaultClient
	}
	client := *baseClient
	client.Timeout = c.timeout
	response, err := client.Do(request)
	if err != nil {
		reason := "network"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(requestContext.Err(), context.DeadlineExceeded) {
			reason = "timeout"
		}
		return failed(reason, "暂不可用", 0, 0)
	}
	defer response.Body.Close()
	data, readErr := readLimited(response.Body, aramkitRatingResponseMax)
	if readErr != nil {
		return failed("response-read", "暂不可用", 0, response.StatusCode)
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
	return aramkitRatingOutcome{response: result, code: envelope.Code, httpStatus: response.StatusCode, result: "success"}
}

func unavailableMayhemRating(message, outcome string) mayhemRatingResponse {
	return mayhemRatingResponse{
		Available: false, Source: dataSourceARAMKit, RatingType: -1, FetchedAt: time.Now().UTC(),
		UnavailableReason: message, Sources: []DataSourceAttempt{{Source: dataSourceARAMKit, Outcome: outcome, Message: message}},
	}
}

func (c *aramkitRatingClient) record(key string, cacheHit, coalesced bool, duration time.Duration, outcome aramkitRatingOutcome) {
	if c == nil || c.observe == nil || outcome.reason == "cancelled" {
		return
	}
	identity := ""
	if c.identityHash != nil {
		identity = c.identityHash(key)
	}
	c.observe(map[string]any{
		"event": "mayhem_rating_lookup", "source": dataSourceARAMKit, "player_hash": identity,
		"cache_hit": cacheHit, "coalesced": coalesced, "duration_ms": duration.Milliseconds(),
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
	tagLine := strings.TrimSpace(query.Get("tagLine"))
	if publicRef := strings.TrimSpace(query.Get("playerRef")); publicRef != "" {
		reference, ok := a.resolveGameplayReferenceDetails(publicRef)
		if !ok {
			http.Error(w, "player reference expired", http.StatusNotFound)
			return
		}
		if strings.EqualFold(strings.TrimSpace(reference.Region), riotRegionKR) {
			response := unavailableMayhemRating("仅支持国服", dataSourceModeUnsupported)
			respondJSON(w, response)
			return
		}
		gameName, tagLine = reference.GameName, reference.TagLine
	}
	if _, _, _, err := normalizeAramkitIdentity(gameName, tagLine); err != nil {
		http.Error(w, "player identity unavailable", http.StatusBadRequest)
		return
	}
	if a.mayhemRatings == nil {
		http.Error(w, "rating service unavailable", http.StatusServiceUnavailable)
		return
	}
	respondJSON(w, a.mayhemRatings.lookup(r.Context(), gameName, tagLine))
}
