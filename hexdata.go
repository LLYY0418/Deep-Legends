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
	// Mayhem augment IDs start at 1000; Arena keeps 1..405.
	mayhemAugmentIDFloor = 1000
	// Bounded so hydrating a nine-card recommendation grid stays inside the
	// client's own pacing budget instead of queueing behind itself.
	mayhemAugmentCopyConcurrency = 3
	mayhemAugmentSlugFailureTTL  = time.Minute
)

func hexdataMeasurementTechnique(answer hexdataAnswerCards) string {
	return strings.TrimSpace(answer.Methodology.MeasurementTechnique)
}

var (
	hexdataHeroPathPattern    = regexp.MustCompile(`^/hero/([0-9]+)-([a-z0-9-]+)$`)
	hexdataAugmentPathPattern = regexp.MustCompile(`^/augment/([0-9]+)-([a-z0-9-]+)$`)
	hexdataHeroMetricPattern  = regexp.MustCompile(`胜率\s*([0-9]+(?:\.[0-9]+)?)%\s*[·，,]\s*样本\s*([0-9,]+)`)
	hexdataChampionNickname   = regexp.MustCompile(`\s*[\(（][^()（）]*[\)）]\s*$`)
	hexdataPatchPattern       = regexp.MustCompile(`Patch\s+([0-9]+\.[0-9]+)`)
	hexdataDatePattern        = regexp.MustCompile(`数据日期\s*([0-9]{4}-[0-9]{2}-[0-9]{2})`)
	hexdataTierPattern        = regexp.MustCompile(`层级\s*T([1-5])`)
	hexdataGlobalMetric       = regexp.MustCompile(`globalHexScore\s*([0-9]+(?:\.[0-9]+)?)\s*[·，,]\s*胜率\s*([0-9]+(?:\.[0-9]+)?)%`)
	opggRSCVersionPattern     = regexp.MustCompile(`/meta/images/lol/([0-9]+\.[0-9]+(?:\.[0-9]+)?)/`)
	opggRSCLinePattern        = regexp.MustCompile(`(?m)^([0-9a-f]+):(.*)$`)
	opggRSCReferencePattern   = regexp.MustCompile(`"\$L([0-9a-f]+)"`)
	opggRSCMetaIDPattern      = regexp.MustCompile(`"metaId":([0-9]+)`)
	opggRSCMetaKindIDPattern  = regexp.MustCompile(`"metaType":"([^"]+)"\s*,\s*"metaId":([0-9]+)`)
	opggRSCMetaIDKindPattern  = regexp.MustCompile(`"metaId":([0-9]+)\s*,\s*"metaType":"([^"]+)"`)
	opggRSCAugmentIDPattern   = regexp.MustCompile(`"metaId":([0-9]+),"metaType":"aram-augment"`)
	opggRSCSkillPattern       = regexp.MustCompile(`"skill_[0-9]+"[\s\S]{0,600}?"extraData":"([QWER])"`)
	opggRSCSkillOrderPattern  = regexp.MustCompile(`"span","[0-9]+"[\s\S]{0,320}?"children":"([QWER])"`)
	opggRSCSpellPairPattern   = regexp.MustCompile(`(?m)^[0-9a-f]+:(.*"spell_0".*"spell_1".*)$`)
	opggRSCSpellMetaPattern   = regexp.MustCompile(`(?:"metaId":([0-9]+)\s*,\s*"metaType":"spell"|"metaType":"spell"\s*,\s*"metaId":([0-9]+))`)
	opggRSCMetaNamePattern    = regexp.MustCompile(`"(?:name|title)":"([^"]+)"`)
	opggRSCMetaIconPattern    = regexp.MustCompile(`"(?:src|icon|iconUrl|image_url)":"([^"]+\.(?:png|jpg|jpeg|webp))"`)
	hexdataGlobalGate         = make(chan struct{}, 3)
	hexdataGlobalPaceMu       sync.Mutex
	hexdataGlobalLastRequest  time.Time
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

type hexdataHeroDetail struct {
	Citation championSourceCitation
	WinRate  float64
	Games    int
	Tier     int
	Augments []championMetricRow
	Items    []championMetricRow
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
	ID                   int                       `json:"id"`
	Slug                 string                    `json:"slug"`
	Source               string                    `json:"source"`
	Description          string                    `json:"description,omitempty"`
	Citation             *championSourceCitation   `json:"citation,omitempty"`
	MeasurementTechnique string                    `json:"measurementTechnique,omitempty"`
	Champions            []championAugmentChampion `json:"champions"`
}

type championAugmentRarityResponse struct {
	Source               string                  `json:"source"`
	Citation             *championSourceCitation `json:"citation,omitempty"`
	MeasurementTechnique string                  `json:"measurementTechnique,omitempty"`
	Stages               []hexdataRarityStage    `json:"stages"`
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
		rows, citation, err := parseHexdataAugments(data)
		shape = hexdataPayloadShape{Rows: len(rows), Fields: hexdataTableFieldCount(data), BuildID: citation.BuildID}
		if err != nil || !hexdataCitationComplete(citation) {
			for _, report := range reporters {
				if report != nil {
					report(hexdataTableShapeDiagnostic("augments", data))
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
	return path == hexdataAnswerPath || path == "/heroes" || path == "/augments" || path == "/augment-rarity" || hexdataHeroPathPattern.MatchString(path) || hexdataAugmentPathPattern.MatchString(path)
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
	if kind == "answer" {
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
	if kind == "answer" {
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
	if requestPath == hexdataAnswerPath {
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
	if h.state.BuildID != "" && h.state.BuildID != citation.BuildID {
		h.state.PreviousBuildID = h.state.BuildID
	}
	h.state.BuildID = citation.BuildID
	h.state.ReportPatch = citation.Patch
	h.state.ReportDate = citation.ReportDate
	h.state.BuildChecked = h.now()
	h.saveStateLocked()
	h.mu.Unlock()
	return true
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

func (p *championProvider) reportHexdataHeroShape(detail hexdataHeroDetail) {
	p.reportHexdataShape("hero", len(detail.Augments), len(detail.Items), detail.Citation.BuildID)
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
		reportDate = match[1]
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
	result := make([]championRankingRow, 0, len(rows))
	for index, item := range rows {
		result = append(result, championRankingRow{ChampionID: item.ID, Key: item.Slug, Name: item.Name, Rank: index + 1, Tier: index*5/len(rows) + 1, Play: item.Games, WinRate: item.WinRate, TierLocallyCalculated: true})
	}
	p.reportHexdataShape("heroes", len(result), len(result)*5, citation.BuildID)
	return championRankingResponse{Mode: "hextech-aram", Region: "CN", Patch: citation.Patch, Source: "Hexdata", FetchedAt: heroesPage.FetchedAt, EntertainmentSample: true, Citation: &citation, MeasurementTechnique: hexdataMeasurementTechnique(answer), Rows: result}, nil
}

func firstError(values ...error) error {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func parseHexdataHeroDetail(data []byte, canonical string) (hexdataHeroDetail, error) {
	document, buildID, err := parseHexdataDocument(data)
	if err != nil {
		return hexdataHeroDetail{}, err
	}
	tables := primaryTables(document)
	result := hexdataHeroDetail{Citation: hexdataCitation(document, buildID, canonical)}
	text := hexdataNodeText(document)
	if match := hexdataHeroMetricPattern.FindStringSubmatch(text); len(match) == 3 {
		result.WinRate, result.Games = parseFloatText(match[1]), parseCountText(match[2])
	}
	if match := hexdataTierPattern.FindStringSubmatch(text); len(match) == 2 {
		result.Tier, _ = strconv.Atoi(match[1])
	}
	for _, table := range tables {
		augmentRows := make([]championMetricRow, 0)
		itemRows := make([]championMetricRow, 0)
		for _, cells := range tableRows(table) {
			if len(cells) != 4 {
				continue
			}
			href, name := firstLink(cells[0])
			if match := hexdataAugmentPathPattern.FindStringSubmatch(hexdataLinkPath(href)); len(match) == 3 {
				id, _ := strconv.Atoi(match[1])
				games := parseCountText(hexdataNodeText(cells[3]))
				augmentRows = append(augmentRows, championMetricRow{Assets: []championAsset{{ID: id, Kind: "augment", Name: name}}, Score: parseFloatText(hexdataNodeText(cells[1])), WinRate: parsePercentText(hexdataNodeText(cells[2])), Games: games})
				continue
			}
			name = hexdataNodeText(cells[0])
			games := parseCountText(hexdataNodeText(cells[3]))
			if name != "" && games > 0 {
				itemRows = append(itemRows, championMetricRow{Assets: []championAsset{{Kind: "item", Name: name}}, Score: parseFloatText(hexdataNodeText(cells[1])), WinRate: parsePercentText(hexdataNodeText(cells[2])), Games: games})
			}
		}
		if len(result.Augments) == 0 && len(augmentRows) >= 8 {
			result.Augments = augmentRows
		}
		if len(result.Items) == 0 && len(itemRows) >= 8 {
			result.Items = itemRows
		}
	}
	if len(tables) < 2 || len(result.Augments) < 8 || len(result.Items) < 8 || result.WinRate <= 0 || result.Games <= 0 {
		return result, errors.New("hexdata hero detail shape is incomplete")
	}
	filtered := result.Augments[:0]
	for _, row := range result.Augments {
		if row.Games >= hexdataMinimumSample {
			filtered = append(filtered, row)
		}
	}
	result.Augments = filtered
	return result, nil
}

func parseHexdataAugments(data []byte) ([]championAugment, championSourceCitation, error) {
	document, buildID, err := parseHexdataDocument(data)
	if err != nil {
		return nil, championSourceCitation{}, err
	}
	tables := primaryTables(document)
	if len(tables) != 1 {
		return nil, championSourceCitation{}, errors.New("hexdata augments table changed")
	}
	rows := make([]championAugment, 0, 220)
	for _, cells := range tableRows(tables[0]) {
		if len(cells) < 2 {
			continue
		}
		href, name := firstLink(cells[0])
		path := hexdataAugmentPathPattern.FindStringSubmatch(hexdataLinkPath(href))
		metrics := hexdataGlobalMetric.FindStringSubmatch(hexdataNodeText(cells[1]))
		if len(path) != 3 || len(metrics) != 3 {
			continue
		}
		id, _ := strconv.Atoi(path[1])
		if id <= 0 || name == "" {
			continue
		}
		rows = append(rows, championAugment{ID: id, Key: path[2], Name: name, Performance: parseFloatText(metrics[1]), WinRate: parseFloatText(metrics[2])})
	}
	citation := hexdataCitation(document, buildID, "/augments")
	if len(rows) < 150 {
		return rows, citation, errors.New("hexdata augments shape is incomplete")
	}
	return rows, citation, nil
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
	rows := make([]hexdataRarityStage, 0, 4)
	for _, cells := range tableRows(tables[0]) {
		if len(cells) != 3 {
			continue
		}
		stageMatch := regexp.MustCompile(`[1-4]`).FindString(hexdataNodeText(cells[0]))
		metrics := metricPattern.FindStringSubmatch(hexdataNodeText(cells[1]))
		if stageMatch == "" || len(metrics) != 4 {
			continue
		}
		stage, _ := strconv.Atoi(stageMatch)
		rows = append(rows, hexdataRarityStage{Stage: stage, Silver: parseFloatText(metrics[1]), Gold: parseFloatText(metrics[2]), Prismatic: parseFloatText(metrics[3]), Games: parseCountText(hexdataNodeText(cells[2]))})
	}
	citation := hexdataCitation(document, buildID, "/augment-rarity")
	if len(rows) != 4 {
		return rows, citation, errors.New("hexdata rarity shape is incomplete")
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

func (p *championProvider) decorateHexdataItems(ctx context.Context, rows []championMetricRow) {
	descriptions, err := p.loadStaticDescriptions(ctx)
	if err != nil {
		return
	}
	byName := make(map[string]championAsset, len(descriptions))
	for key, item := range descriptions {
		if !strings.HasPrefix(key, "item/") || item.Name == "" {
			continue
		}
		id, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(key, "item/"), filepath.Ext(key)))
		if id > 0 {
			byName[item.Name] = championAsset{ID: id, Kind: "item", Name: item.Name, Description: item.Description, Source: item.Source, Path: item.Path}
		}
	}
	matched := 0
	for index := range rows {
		if len(rows[index].Assets) == 0 {
			continue
		}
		name := rows[index].Assets[0].Name
		if asset, ok := byName[name]; ok {
			rows[index].Assets[0] = asset
			matched++
		}
	}
	if p.diag != nil {
		p.diag(map[string]any{"event": "hexdata_item_match", "rows": len(rows), "matched": matched})
	}
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
	p.hydrateMayhemAugmentCopy(ctx, rows)
	for rowIndex := range rows {
		if len(rows[rowIndex].Assets) == 0 {
			continue
		}
		asset := &rows[rowIndex].Assets[0]
		asset.Description = augmentDescriptionWithOfflineGuidance(asset.ID, asset.Description)
	}
}

// Mayhem augments have no description anywhere in Riot's data: cherry-augments
// .json ships six fields and none of them is a description (CommunityDragon's
// copy is byte-for-byte the client's own file), /latest/cdragon/ only exposes
// arena/, and arena/zh_cn.json stops at ID 405. The Hexdata augment page is the
// only place the rendered Chinese copy exists, so the rows the user is actually
// looking at are hydrated from it on demand and then cached.
func (p *championProvider) hydrateMayhemAugmentCopy(ctx context.Context, rows []championMetricRow) {
	if p.hexdata == nil {
		return
	}
	wanted := make([]int, 0, len(rows))
	seen := make(map[int]bool, len(rows))
	for rowIndex := range rows {
		if len(rows[rowIndex].Assets) == 0 {
			continue
		}
		asset := rows[rowIndex].Assets[0]
		if asset.ID < mayhemAugmentIDFloor || strings.TrimSpace(asset.Description) != "" || seen[asset.ID] {
			continue
		}
		seen[asset.ID] = true
		wanted = append(wanted, asset.ID)
	}
	if len(wanted) == 0 {
		return
	}
	resolved := p.mayhemAugmentCopy(ctx, wanted)
	if len(resolved) == 0 {
		return
	}
	for rowIndex := range rows {
		if len(rows[rowIndex].Assets) == 0 {
			continue
		}
		asset := &rows[rowIndex].Assets[0]
		if strings.TrimSpace(asset.Description) == "" && resolved[asset.ID] != "" {
			asset.Description = resolved[asset.ID]
		}
	}
	if p.diag != nil {
		p.diag(map[string]any{"event": "hexdata_augment_copy", "requested": len(wanted), "resolved": len(resolved)})
	}
}

func (p *championProvider) mayhemAugmentCopy(ctx context.Context, ids []int) map[int]string {
	result := make(map[int]string, len(ids))
	pending := make([]int, 0, len(ids))
	p.augmentCopyMu.Lock()
	for _, id := range ids {
		if text, ok := p.augmentCopy[id]; ok {
			if text != "" {
				result[id] = text
			}
			continue
		}
		pending = append(pending, id)
	}
	p.augmentCopyMu.Unlock()
	if len(pending) == 0 {
		return result
	}
	slugs := p.mayhemAugmentSlugs(ctx)
	if len(slugs) == 0 {
		return result
	}
	type outcome struct {
		id   int
		text string
		ok   bool
	}
	results := make(chan outcome, len(pending))
	gate := make(chan struct{}, mayhemAugmentCopyConcurrency)
	var group sync.WaitGroup
	for _, id := range pending {
		slug := slugs[id]
		if slug == "" {
			continue
		}
		group.Add(1)
		go func(id int, slug string) {
			defer group.Done()
			gate <- struct{}{}
			defer func() { <-gate }()
			text, err := p.fetchMayhemAugmentCopy(ctx, id, slug)
			results <- outcome{id: id, text: text, ok: err == nil}
		}(id, slug)
	}
	group.Wait()
	close(results)
	p.augmentCopyMu.Lock()
	if p.augmentCopy == nil {
		p.augmentCopy = make(map[int]string, len(ids))
	}
	for item := range results {
		// Only a successful parse is memoized. A transient failure must stay
		// retryable, otherwise one blip blanks the augment for the session.
		if !item.ok {
			continue
		}
		p.augmentCopy[item.id] = item.text
		if item.text != "" {
			result[item.id] = item.text
		}
	}
	p.augmentCopyMu.Unlock()
	return result
}

func (p *championProvider) fetchMayhemAugmentCopy(ctx context.Context, id int, slug string) (string, error) {
	canonical := "/augment/" + strconv.Itoa(id) + "-" + slug
	if !hexdataAugmentPathPattern.MatchString(canonical) {
		return "", errors.New("invalid hexdata augment path")
	}
	page, err := p.hexdata.load(ctx, "augment", strconv.Itoa(id), canonical, "text/html,application/xhtml+xml", false)
	if err != nil {
		return "", err
	}
	document, _, err := parseHexdataDocument(page.Data)
	if err != nil {
		p.hexdata.recordShapeFailure("augment", 0, hexdataTableFieldCount(page.Data), "")
		return "", err
	}
	if page.Cache != championCacheStateStale {
		p.hexdata.recordSuccess("augment", canonical)
	}
	return hexdataAugmentGuideDescription(document), nil
}

// The augment ranking page carries every ID/slug pair in one cached request,
// so resolving slugs never costs an extra round trip per augment.
func (p *championProvider) mayhemAugmentSlugs(ctx context.Context) map[int]string {
	p.augmentCopyMu.Lock()
	cached := p.augmentSlugs
	retryAt := p.augmentSlugsRetryAt
	p.augmentCopyMu.Unlock()
	if len(cached) > 0 {
		return cached
	}
	if !retryAt.IsZero() && time.Now().Before(retryAt) {
		return nil
	}
	page, err := p.hexdata.load(ctx, "augments", "all", "/augments", "text/html,application/xhtml+xml", false)
	if err != nil {
		p.rememberAugmentSlugFailure()
		return nil
	}
	rows, citation, err := parseHexdataAugments(page.Data)
	if err != nil && len(rows) == 0 {
		p.hexdata.recordShapeFailure("augments", len(rows), hexdataTableFieldCount(page.Data), citation.BuildID)
		p.rememberAugmentSlugFailure()
		return nil
	}
	slugs := make(map[int]string, len(rows))
	for _, row := range rows {
		if row.ID > 0 && row.Key != "" {
			slugs[row.ID] = row.Key
		}
	}
	if len(slugs) == 0 {
		p.hexdata.recordShapeFailure("augments", len(rows), hexdataTableFieldCount(page.Data), citation.BuildID)
		p.rememberAugmentSlugFailure()
		return nil
	}
	if page.Cache != championCacheStateStale {
		p.hexdata.recordSuccess("augments", "/augments")
	}
	p.augmentCopyMu.Lock()
	p.augmentSlugs = slugs
	p.augmentSlugsRetryAt = time.Time{}
	p.augmentCopyMu.Unlock()
	return slugs
}

func (p *championProvider) rememberAugmentSlugFailure() {
	p.augmentCopyMu.Lock()
	p.augmentSlugsRetryAt = time.Now().Add(mayhemAugmentSlugFailureTTL)
	p.augmentCopyMu.Unlock()
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
	canonical := "/hero/" + strconv.Itoa(id) + "-" + slug
	var primary hexdataHeroDetail
	var primaryErr error
	page, pageErr := p.hexdata.load(ctx, "hero", strconv.Itoa(id), canonical, "text/html,application/xhtml+xml", false)
	if pageErr == nil {
		primary, primaryErr = parseHexdataHeroDetail(page.Data, canonical)
		if primaryErr == nil && !hexdataCitationComplete(primary.Citation) {
			primaryErr = errors.New("hexdata hero citation metadata is incomplete")
		} else if primaryErr == nil && page.Cache != championCacheStateStale && !p.hexdata.adoptCitation(primary.Citation) {
			primaryErr = errors.New("hexdata hero citation metadata is incomplete")
		}
		if primaryErr == nil {
			p.hexdata.promote("hero", strconv.Itoa(id), primary.Citation.BuildID, page.Data, page.FetchedAt)
			if page.Cache != championCacheStateStale {
				p.hexdata.recordSuccess("hero", canonical)
			}
		} else {
			p.hexdata.recordShapeFailure("hero", len(primary.Augments), len(primary.Items), primary.Citation.BuildID)
		}
	} else {
		primaryErr = pageErr
		p.reportHexdataFallback("hero-detail", pageErr)
	}
	rsc, rscErr := p.loadMayhemRSC(ctx, metadata.Slug)
	if primaryErr != nil && rscErr != nil {
		return championDetailResponse{}, primaryErr
	}
	response := championDetailResponse{Mode: "hextech-aram", Region: "CN", Source: "Hexdata + OP.GG RSC", EntertainmentSample: true, MeasurementTechnique: p.loadHexdataMeasurementTechnique(ctx)}
	if primaryErr == nil {
		response.Patch, response.FetchedAt, response.Stats.WinRate = primary.Citation.Patch, page.FetchedAt, primary.WinRate
		if primary.Tier >= 1 && primary.Tier <= 5 {
			tier := primary.Tier
			response.Stats.Tier = &tier
		}
		response.Citation = &primary.Citation
		response.RecommendedAugments = primary.Augments
		response.ItemRanking = primary.Items
		p.decorateHexdataAugments(ctx, response.RecommendedAugments)
		p.decorateHexdataItems(ctx, response.ItemRanking)
		p.reportHexdataHeroShape(primary)
	}
	if rscErr == nil {
		response.Build = rsc.Build
		response.BuildCitation = &rsc.Citation
		if primaryErr != nil {
			response.Source, response.Patch, response.FetchedAt = "OP.GG RSC", rsc.Citation.Patch, rsc.FetchedAt
			response.Citation = &rsc.Citation
		}
		response.RecommendedAugments = mergeMayhemAugmentRows(response.RecommendedAugments, rsc.Augments, 9)
		p.decorateHexdataAugments(ctx, response.RecommendedAugments)
		p.decorateDetailAssets(ctx, metadata.Slug, &response)
	}
	applyLocalAugmentGrades(response.RecommendedAugments)
	response.MeasurementTechnique = appendMeasurementTechnique(response.MeasurementTechnique, "当前档位为本地综合评分分位计算，非官方等级；缺少统计指标时按推荐顺序回退")
	if len(response.RecommendedAugments) == 0 {
		return championDetailResponse{}, errors.New("mayhem recommendations are incomplete")
	}
	return response, nil
}

func mergeMayhemAugmentRows(primary, fallback []championMetricRow, limit int) []championMetricRow {
	result := make([]championMetricRow, 0, min(limit, len(primary)+len(fallback)))
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
		if len(result.Augments) == 12 {
			break
		}
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
