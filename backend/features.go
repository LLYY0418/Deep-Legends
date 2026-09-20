package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

type poolSummary struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Source     string    `json:"source"`
	Version    string    `json:"version"`
	UpdatedAt  time.Time `json:"updatedAt"`
	Hash       string    `json:"hash"`
	EntryCount int       `json:"entryCount"`
	BuiltIn    bool      `json:"builtIn"`
	Selected   bool      `json:"selected"`
}

type diagnosticsResponse struct {
	SchemaVersion           int                     `json:"schemaVersion"`
	Connected               bool                    `json:"connected"`
	IdentityReady           bool                    `json:"identityReady"`
	SnapshotReady           bool                    `json:"snapshotReady"`
	ConnectionState         string                  `json:"connectionState"`
	EventStream             bool                    `json:"eventStream"`
	Syncing                 bool                    `json:"syncing"`
	LastAttempt             time.Time               `json:"lastAttempt,omitempty"`
	LastSuccess             time.Time               `json:"lastSuccess,omitempty"`
	LastDurationMS          int64                   `json:"lastDurationMs"`
	SnapshotRetryCount      int                     `json:"snapshotRetryCount,omitempty"`
	SnapshotRetryElapsedMS  int64                   `json:"snapshotRetryElapsedMs,omitempty"`
	SnapshotRetryExhausted  bool                    `json:"snapshotRetryExhausted,omitempty"`
	SnapshotFallback        bool                    `json:"snapshotFallback,omitempty"`
	SnapshotFallbackAt      time.Time               `json:"snapshotFallbackAt,omitempty"`
	LastError               string                  `json:"lastError,omitempty"`
	LCUSource               string                  `json:"lcuSource,omitempty"`
	Ownership               []OwnershipSourceStatus `json:"ownershipSources"`
	Catalog                 CatalogStats            `json:"catalog"`
	PoolID                  string                  `json:"poolId"`
	PoolHash                string                  `json:"poolHash"`
	PoolTotal               int                     `json:"poolTotal"`
	PoolMatched             int                     `json:"poolMatched"`
	PoolIssueCount          int                     `json:"poolIssueCount"`
	StorageReady            bool                    `json:"storageReady"`
	DiagnosticLogReady      bool                    `json:"diagnosticLogReady"`
	DiagnosticLogError      string                  `json:"diagnosticLogError,omitempty"`
	DiagnosticLogBytes      int64                   `json:"diagnosticLogBytes"`
	DiagnosticLogEvents     int                     `json:"diagnosticLogEvents"`
	DiagnosticWriteFailures uint64                  `json:"diagnosticWriteFailures"`
	DiagnosticWritePending  uint64                  `json:"diagnosticWritePending"`
	Capabilities            []EndpointCapability    `json:"capabilities"`
	Discovery               LCUDiscoveryStatus      `json:"discovery"`
}

type clientDiagnosticRequest struct {
	Observations           []gameflowClientObservation `json:"observations,omitempty"`
	ItemID                 int64                       `json:"itemId,omitempty"`
	Endpoint               string                      `json:"endpoint,omitempty"`
	StartedAt              int64                       `json:"startedAt,omitempty"`
	CompletedAt            int64                       `json:"completedAt,omitempty"`
	Section                string                      `json:"section,omitempty"`
	PhaseChanged           bool                        `json:"phaseChanged,omitempty"`
	LiveRefreshQueued      bool                        `json:"liveRefreshQueued,omitempty"`
	Claiming               bool                        `json:"claiming,omitempty"`
	Done                   int                         `json:"done,omitempty"`
	Total                  int                         `json:"total,omitempty"`
	Event                  string                      `json:"event"`
	Reason                 string                      `json:"reason"`
	Key                    string                      `json:"key,omitempty"`
	TraceID                string                      `json:"traceId,omitempty"`
	RequestedPosition      string                      `json:"requestedPosition,omitempty"`
	ResolvedPosition       string                      `json:"resolvedPosition,omitempty"`
	PositionSource         string                      `json:"positionSource,omitempty"`
	GameMode               string                      `json:"gameMode,omitempty"`
	Tier                   string                      `json:"tier,omitempty"`
	MapID                  int64                       `json:"mapId,omitempty"`
	BlockCount             int                         `json:"blockCount,omitempty"`
	ItemCount              int                         `json:"itemCount,omitempty"`
	ChampionID             int64                       `json:"championId,omitempty"`
	QueueID                int64                       `json:"queueId,omitempty"`
	Position               string                      `json:"position,omitempty"`
	Phase                  string                      `json:"phase,omitempty"`
	GameID                 int64                       `json:"gameId,omitempty"`
	PlayersReceived        int                         `json:"playersReceived,omitempty"`
	Rendered100            int                         `json:"rendered100,omitempty"`
	Rendered200            int                         `json:"rendered200,omitempty"`
	TotalRefs              int                         `json:"totalRefs,omitempty"`
	UniqueRefs             int                         `json:"uniqueRefs,omitempty"`
	CacheHits              int                         `json:"cacheHits,omitempty"`
	Revision               int                         `json:"revision,omitempty"`
	DurationMS             int                         `json:"durationMs,omitempty"`
	MasterEnabled          bool                        `json:"masterEnabled,omitempty"`
	CustomPaused           bool                        `json:"customPaused,omitempty"`
	ChampSelectEnabled     bool                        `json:"champSelectEnabled,omitempty"`
	AutoMatchmakingEnabled bool                        `json:"autoMatchmakingEnabled,omitempty"`
	PendingSaves           int                         `json:"pendingSaves,omitempty"`
	ForceRefresh           bool                        `json:"forceRefresh,omitempty"`
	Hidden                 bool                        `json:"hidden,omitempty"`
	TeamsReceived          int                         `json:"teamsReceived,omitempty"`
	CacheAgeMS             int                         `json:"cacheAgeMs,omitempty"`
	HTTPStatus             int                         `json:"httpStatus,omitempty"`
	Source                 string                      `json:"source,omitempty"`
	Gate                   string                      `json:"gate,omitempty"`
	ErrorKind              string                      `json:"errorKind,omitempty"`
	RequestID              int                         `json:"requestId,omitempty"`
	TransportFailed        int                         `json:"transportFailed,omitempty"`
	TransportDropped       int                         `json:"transportDropped,omitempty"`
	TransportSuppressed    int                         `json:"transportSuppressed,omitempty"`
	TransportPending       int                         `json:"transportPending,omitempty"`
	TransportHTTPStatus    int                         `json:"transportHTTPStatus,omitempty"`
	TransportErrorKind     string                      `json:"transportErrorKind,omitempty"`
}

var specialistRuneClientReasons = map[string]bool{
	"no-target": true, "no-top-players": true, "embedded": true,
	"cached": true, "cached-empty": true, "in-flight": true,
	"riot-key-missing-cooldown": true, "recent-failure-cooldown": true,
}

var clientDiagnosticEvents = map[string]map[string]bool{
	"gameflow_phase_client":        {"batch": true},
	"catalog_client":               {"failed": true, "loaded": true},
	"item_id_not_in_catalog":       {"missing": true},
	"champselect_dialog_client":    {"open": true, "rerender-while-open": true, "close": true},
	"live_refresh_client":          {"load": true, "queue": true, "phase": true},
	"local_request_client":         {"complete": true},
	"claim_progress_client":        {"begin": true, "heartbeat": true, "end": true, "item-timeout": true},
	"diagnostic_delivery_client":   {"export": true},
	"champ_select_filter_client":   {"request": true, "all": true, "cached": true, "received": true, "stale": true, "failed": true},
	"current_game_client":          {"request": true, "received": true, "rendered": true, "failed": true, "canceled": true, "stale": true, "cached": true, "in-flight": true, "gated": true, "render-failed": true, "render-no-root": true, "render-scope-mismatch": true, "invalid-response": true},
	"watch_settings_client":        {"save-queued": true, "save-succeeded": true, "save-failed": true, "load-started": true, "load-applied": true, "load-stale": true, "load-failed": true, "load-skipped": true, "custom-event": true, "rendered": true},
	"match_timeline_client":        {"missing-participant": true, "unavailable": true, "request-failed": true},
	"specialist_runes_client_skip": specialistRuneClientReasons,
	"live_recommendations_skip": {
		"no-target": true, "has-payload": true, "cached": true, "in-flight": true, "backoff": true,
	},
	"live_roster_rendered":        {"render": true},
	"live_recommendations_client": {"received": true, "rendered": true, "failed": true},
	"item_set_apply_request":      {"submitted": true, "succeeded": true, "failed": true},
	"item_set_apply":              {"success": true, "failed": true},
	"match_tiers_overview_batch":  {"complete": true},
}

const clientDiagnosticTextLimit = 128

func truncateClientDiagnosticText(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > clientDiagnosticTextLimit {
		runes = runes[:clientDiagnosticTextLimit]
	}
	return string(runes)
}

func (a *app) recordClientDiagnosticRejected(reason, rawEvent string) {
	a.recordDiagnostic(map[string]any{
		"event": "client_diagnostic_rejected", "reason": reason,
		"raw_event": truncateClientDiagnosticText(rawEvent),
	})
}

func (a *app) handleClientDiagnostic(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	defer r.Body.Close()
	var request clientDiagnosticRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		a.recordClientDiagnosticRejected("decode-error", request.Event)
		http.Error(w, "invalid client diagnostic", http.StatusBadRequest)
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		a.recordClientDiagnosticRejected("decode-error", request.Event)
		http.Error(w, "invalid client diagnostic", http.StatusBadRequest)
		return
	}
	reasons, knownEvent := clientDiagnosticEvents[request.Event]
	if !knownEvent {
		a.recordClientDiagnosticRejected("unknown-event", request.Event)
		http.Error(w, "invalid client diagnostic", http.StatusBadRequest)
		return
	}
	if !reasons[request.Reason] {
		a.recordClientDiagnosticRejected("unknown-reason", request.Event)
		http.Error(w, "invalid client diagnostic", http.StatusBadRequest)
		return
	}
	if request.Event == "gameflow_phase_client" {
		if !a.recordGameflowClientObservations(request) {
			a.recordClientDiagnosticRejected("invalid-observations", request.Event)
			http.Error(w, "invalid gameflow observations", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	event := map[string]any{"event": request.Event, "reason": request.Reason}
	if request.Event == "catalog_client" || request.Event == "item_id_not_in_catalog" {
		switch request.Endpoint {
		case "items", "perks", "summoner-spells":
			event["endpoint"] = request.Endpoint
		}
		event["http_status"] = min(599, max(0, request.HTTPStatus))
		event["items"] = min(10000, max(0, request.ItemCount))
		event["id"] = min(int64(1000000), max(int64(0), request.ItemID))
		switch request.ErrorKind {
		case "none", "http", "timeout", "network", "decode", "canceled", "empty":
			event["error_kind"] = request.ErrorKind
		}
		a.recordDiagnostic(event)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if request.Event == "champselect_dialog_client" {
		event["revision"] = min(1000000, max(0, request.Revision))
		a.recordDiagnostic(event)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if request.Event == "local_request_client" {
		switch request.Endpoint {
		case "status", "gameplay", "champions", "collection", "other":
			event["endpoint"] = request.Endpoint
		}
		event["started_at"] = min(int64(1e13), max(0, request.StartedAt))
		event["completed_at"] = min(int64(1e13), max(0, request.CompletedAt))
		event["duration_ms"] = min(1000000, max(0, request.DurationMS))
		event["http_status"] = min(599, max(0, request.HTTPStatus))
		switch request.ErrorKind {
		case "none", "http", "timeout", "network", "decode", "canceled":
			event["error_kind"] = request.ErrorKind
		}
		a.recordDiagnostic(event)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if request.Event == "live_refresh_client" {
		event["hidden"], event["phase_changed"], event["live_refresh_queued"] = request.Hidden, request.PhaseChanged, request.LiveRefreshQueued
		switch request.Section {
		case "overview", "live", "champions", "favorites", "suite", "collection", "tools":
			event["section"] = request.Section
		}
		switch request.Source {
		case "direct", "event", "sse", "poll", "resync", "interval":
			event["source"] = request.Source
		}
		event["transport_suppressed"] = min(1000000, max(0, request.TransportSuppressed))
		a.recordDiagnostic(event)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if request.Event == "claim_progress_client" {
		event["hidden"] = request.Hidden
		event["claiming"], event["done"], event["total"] = request.Claiming, min(100, max(0, request.Done)), min(100, max(0, request.Total))
		event["duration_ms"] = min(3600000, max(0, request.DurationMS))
		a.recordDiagnostic(event)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if request.Event == "current_game_client" || request.Event == "watch_settings_client" || request.Event == "champ_select_filter_client" || request.Event == "diagnostic_delivery_client" {
		// Dedicated allowlist: ignore every general-purpose text/identity field.
		event["diagnostic_schema"] = 4
		if validFlowClientTrace(request.TraceID) {
			event["trace_id"] = request.TraceID
		}
		event["duration_ms"] = min(120000, max(0, request.DurationMS))
		for key, value := range map[string]string{"error_kind": request.ErrorKind, "transport_error_kind": request.TransportErrorKind} {
			switch value {
			case "none", "http", "timeout", "network", "decode", "read", "canceled", "invalid-response", "other":
				event[key] = value
			}
		}
		for key, value := range map[string]int{"transport_failed": request.TransportFailed, "transport_dropped": request.TransportDropped, "transport_suppressed": request.TransportSuppressed} {
			event[key] = min(1000000, max(0, value))
		}
		if request.Event == "diagnostic_delivery_client" {
			event["transport_pending"] = min(97, max(0, request.TransportPending))
			event["request_id"] = min(1000000, max(0, request.RequestID))
		}
		if request.TransportHTTPStatus >= 100 && request.TransportHTTPStatus <= 599 {
			event["transport_http_status"] = request.TransportHTTPStatus
		}
		if request.Event == "champ_select_filter_client" {
			event["request_id"] = min(1000000, max(0, request.RequestID))
			event["item_count"] = min(1000, max(0, request.ItemCount))
			for key, value := range map[string]string{"requested_position": request.RequestedPosition, "resolved_position": request.ResolvedPosition} {
				switch value {
				case "all", "top", "jungle", "middle", "bottom", "utility", "mid", "adc", "support":
					event[key] = value
				}
			}
		}
		if request.Event == "watch_settings_client" {
			event["revision"] = min(1000000, max(0, request.Revision))
			event["master_enabled"], event["custom_paused"], event["champselect_enabled"] = request.MasterEnabled, request.CustomPaused, request.ChampSelectEnabled
			event["auto_matchmaking_enabled"] = request.AutoMatchmakingEnabled
			event["pending_saves"] = min(1000, max(0, request.PendingSaves))
		} else {
			event["manual_refresh"], event["hidden"] = request.ForceRefresh, request.Hidden
			event["teams_received"], event["cache_age_ms"] = min(100, max(0, request.TeamsReceived)), min(1000000, max(0, request.CacheAgeMS))
			if request.HTTPStatus >= 100 && request.HTTPStatus <= 599 {
				event["http_status"] = request.HTTPStatus
			}
			switch request.Source {
			case "OP.GG":
				event["source"] = request.Source
			}
			switch request.Gate {
			case "no-reference", "destroyed", "external-render", "private-kr", "reference-changed":
				event["gate"] = request.Gate
			}
		}
		event["players_received"] = min(100, max(0, request.PlayersReceived))
		event["rendered_100"], event["rendered_200"] = min(100, max(0, request.Rendered100)), min(100, max(0, request.Rendered200))
		if request.Phase == "active" || request.Phase == "none" || request.Phase == "unsupported" || request.Phase == "error" {
			event["state"] = request.Phase
		}
		a.recordDiagnostic(event)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if request.Event == "live_recommendations_client" || request.Event == "item_set_apply_request" {
		event["diagnostic_schema"] = 2
		if traceID := normalizeItemSetTraceID(request.TraceID); traceID != "" {
			event["trace_id"] = traceID
		}
		for key, value := range map[string]string{
			"requested_position": request.RequestedPosition, "resolved_position": request.ResolvedPosition,
			"position_source": request.PositionSource, "game_mode": request.GameMode, "tier": request.Tier,
		} {
			if value = truncateClientDiagnosticText(value); value != "" {
				event[key] = strings.ToLower(value)
			}
		}
		if request.GameID > 0 {
			event["game_id"] = request.GameID
		}
		if request.MapID > 0 {
			event["map_id"] = request.MapID
		}
		event["block_count"] = max(0, request.BlockCount)
		event["item_count"] = max(0, request.ItemCount)
	}
	if key := truncateClientDiagnosticText(request.Key); key != "" {
		event["key"] = key
	}
	if request.ChampionID > 0 {
		event["champion_id"] = request.ChampionID
	}
	if request.QueueID >= 0 {
		event["queue_id"] = request.QueueID
	}
	if position := strings.ToLower(strings.TrimSpace(request.Position)); position != "" && len(position) <= 16 {
		event["position"] = position
	}
	if phase := strings.TrimSpace(request.Phase); phase != "" && len(phase) <= 32 {
		event["phase"] = phase
	}
	if request.Event == "live_roster_rendered" {
		if request.GameID > 0 {
			event["game_id"] = request.GameID
		}
		event["players_received"] = request.PlayersReceived
		event["rendered_100"] = request.Rendered100
		event["rendered_200"] = request.Rendered200
	}
	if request.Event == "match_tiers_overview_batch" {
		event["total_refs"] = max(0, request.TotalRefs)
		event["unique_refs"] = max(0, request.UniqueRefs)
		event["cache_hits"] = max(0, request.CacheHits)
	}
	a.recordDiagnostic(event)
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) handleDiagnostics(w http.ResponseWriter, _ *http.Request) {
	a.flushRankedWinrateDiagnostics()
	a.mu.RLock()
	identityReady := a.identityReady || (a.connected && a.summoner.SummonerID != 0)
	response := diagnosticsResponse{
		SchemaVersion: 5, Connected: a.connected, IdentityReady: identityReady, SnapshotReady: a.snapshotReady, ConnectionState: a.connectionState, EventStream: a.eventStream,
		Syncing: a.syncing, LastAttempt: a.lastAttempt,
		LastSuccess: a.lastSync, LastDurationMS: a.lastDuration.Milliseconds(), LastError: a.lastError,
		SnapshotRetryCount: a.snapshotRetryCount, SnapshotRetryExhausted: a.snapshotRetryExhausted,
		SnapshotFallback: a.snapshotFallback, SnapshotFallbackAt: a.snapshotFallbackAt,
		Ownership: append([]OwnershipSourceStatus(nil), a.ownership...), Catalog: a.catalog,
		PoolID: a.poolID, PoolHash: a.poolHash, PoolTotal: a.poolTotal, PoolMatched: a.poolMatched,
		PoolIssueCount: len(a.poolIssues), StorageReady: a.storage != nil,
		Capabilities: append([]EndpointCapability(nil), a.account.Capabilities...),
		Discovery:    a.discovery,
	}
	if !a.snapshotRetryStarted.IsZero() {
		response.SnapshotRetryElapsedMS = time.Since(a.snapshotRetryStarted).Milliseconds()
	}
	if a.lcu != nil {
		response.LCUSource = a.lcu.source
	}
	a.mu.RUnlock()
	a.diagnosticLogMu.RLock()
	response.DiagnosticLogError = a.diagnosticLogErr
	response.DiagnosticWriteFailures, response.DiagnosticWritePending = a.diagnosticWriteFailures, a.diagnosticWritePending
	a.diagnosticLogMu.RUnlock()
	if response.StorageReady {
		data, err := a.storage.readDiagnosticLog()
		if err != nil {
			response.DiagnosticLogError = "诊断日志读取失败"
		} else {
			response.DiagnosticLogBytes = int64(len(data))
			response.DiagnosticLogEvents = bytes.Count(data, []byte{'\n'})
			if len(bytes.TrimSpace(data)) > 0 && data[len(data)-1] != '\n' {
				response.DiagnosticLogEvents++
			}
		}
	}
	response.DiagnosticLogReady = response.StorageReady && response.DiagnosticLogError == ""
	respondJSON(w, response)
}

func (a *app) handleDiagnosticLog(w http.ResponseWriter, r *http.Request) {
	if a.champions != nil {
		a.champions.flushAssetFetch()
	}
	if a.storage == nil {
		http.Error(w, "本地诊断日志不可用", http.StatusServiceUnavailable)
		return
	}
	a.flushRankedWinrateDiagnostics()
	a.recordItemSetExportSnapshot(r.Context())
	if runner := a.activeWatch(); runner != nil {
		runner.recordFlowExportSnapshot()
	}
	a.mu.RLock()
	client := a.lcu
	a.mu.RUnlock()
	a.collectObjectiveDiagnostics(r.Context(), client, "export")
	a.collectAcceptFocusInspection(r.Context(), client)
	data, err := a.storage.readDiagnosticLogForExport()
	if err != nil {
		a.recordDiagnostic(map[string]any{"event": "diagnostic_export_failed", "stage": "read", "error_kind": diagnosticErrorKind(err)})
		http.Error(w, "诊断日志读取失败", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, diagnosticLogExportFilename(time.Now())))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	written, err := w.Write(data)
	if err == nil && written != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		a.recordDiagnostic(map[string]any{"event": "diagnostic_export_failed", "stage": "response-write", "error_kind": diagnosticErrorKind(err), "expected_bytes": len(data), "written_bytes": written})
	}
}

// diagnosticLogExportFilename 把导出时间戳进文件名（如 lol-loot-diagnostics-0827-1343.jsonl）。
// 同名文件在用户重复上传给第三方（比如粘贴进聊天工具）时容易被按内容或按文件名去重，
// 导致明明是新导出的日志却被误判成旧文件；带上日期时间后每次导出都是不同文件名。
func diagnosticLogExportFilename(at time.Time) string {
	return fmt.Sprintf("lol-loot-diagnostics-%s.jsonl", at.Format("0102-1504"))
}

func (a *app) handlePools(w http.ResponseWriter, _ *http.Request) {
	a.mu.RLock()
	pools := make([]poolSummary, 0, len(a.pools))
	for _, pool := range a.pools {
		pools = append(pools, poolSummary{
			ID: pool.ID, Name: pool.Name, Source: pool.Source, Version: pool.Version, UpdatedAt: pool.UpdatedAt,
			Hash: pool.Hash, EntryCount: len(pool.Names), BuiltIn: pool.BuiltIn, Selected: pool.ID == a.poolID,
		})
	}
	a.mu.RUnlock()
	sort.Slice(pools, func(i, j int) bool {
		if pools[i].BuiltIn != pools[j].BuiltIn {
			return pools[i].BuiltIn
		}
		return pools[i].Name < pools[j].Name
	})
	respondJSON(w, map[string]any{"items": pools})
}

func decodeJSONRequest(r *http.Request, target any, limit int64) error {
	if contentType := r.Header.Get("Content-Type"); contentType != "" && !strings.HasPrefix(strings.ToLower(contentType), "application/json") {
		return errors.New("Content-Type 必须为 application/json")
	}
	if r.ContentLength > limit {
		return errors.New("请求内容过大")
	}
	data, err := readLimited(r.Body, limit)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("无效 JSON：%w", err)
	}
	return nil
}

func (a *app) handlePoolImport(w http.ResponseWriter, r *http.Request) {
	if a.storage == nil {
		http.Error(w, "本地存储不可用，无法导入奖池", http.StatusServiceUnavailable)
		return
	}
	var request struct {
		Name    string   `json:"name"`
		Source  string   `json:"source"`
		Version string   `json:"version"`
		Content string   `json:"content"`
		Names   []string `json:"names"`
	}
	if err := decodeJSONRequest(r, &request, 1024*1024); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	names := request.Names
	if len(names) == 0 {
		names = parsePoolNames(request.Content)
	}
	manifest, err := validatePoolManifest(PoolManifest{Name: request.Name, Source: request.Source, Version: request.Version, Names: names})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.storage.savePool(manifest); err != nil {
		http.Error(w, "保存奖池失败："+err.Error(), http.StatusInternalServerError)
		return
	}
	a.mu.Lock()
	a.pools[manifest.ID] = manifest
	a.mu.Unlock()
	a.recordDiagnostic(map[string]any{"event": "pool_imported", "pool_id": manifest.ID, "pool_hash": manifest.Hash, "entries": len(manifest.Names)})
	respondJSON(w, manifest)
}

func (a *app) handlePoolSelect(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ID string `json:"id"`
	}
	if err := decodeJSONRequest(r, &request, 4096); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a.mu.Lock()
	pool, ok := a.pools[request.ID]
	if !ok {
		a.mu.Unlock()
		http.Error(w, "未知奖池", http.StatusNotFound)
		return
	}
	a.poolID, a.poolSource, a.poolVersion, a.poolHash = pool.ID, pool.Source, pool.Version, pool.Hash
	a.poolTotal = len(pool.Names)
	a.poolGeneration++
	a.clearSnapshotLocked("奖池已切换，正在重新核对当前客户端。")
	a.mu.Unlock()
	a.recordDiagnostic(map[string]any{"event": "pool_selected", "pool_id": pool.ID, "pool_hash": pool.Hash, "entries": len(pool.Names)})
	a.requestRefresh()
	w.WriteHeader(http.StatusAccepted)
}

func (a *app) selectedSkins(view string) ([]Skin, PoolManifest, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if !a.connected || !a.snapshotReady {
		return nil, PoolManifest{}, errors.New("当前没有可用的客户端快照")
	}
	pool := a.pools[a.poolID]
	var skins []Skin
	switch view {
	case "owned":
		skins = a.owned
	case "all":
		skins = a.allSkins
	case "remaining", "":
		if !a.calculationOKLocked() {
			return nil, PoolManifest{}, errors.New("奖池尚未完整映射")
		}
		skins = a.remaining
	default:
		return nil, PoolManifest{}, errors.New("未知导出视图")
	}
	return append([]Skin(nil), skins...), pool, nil
}

func (a *app) handleExport(w http.ResponseWriter, r *http.Request) {
	view := r.URL.Query().Get("view")
	format := strings.ToLower(r.URL.Query().Get("format"))
	skins, pool, err := a.selectedSkins(view)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if view == "" {
		view = "remaining"
	}
	filename := fmt.Sprintf("lol-loot-%s-%s", view, time.Now().Format("20060102-150405"))
	payload := map[string]any{"schemaVersion": 1, "exportedAt": time.Now().UTC(), "view": view, "pool": poolSummary{ID: pool.ID, Name: pool.Name, Source: pool.Source, Version: pool.Version, Hash: pool.Hash, EntryCount: len(pool.Names)}, "items": skins}
	writeSkinExport(w, format, filename, "Deep Legends 导出", view, pool, skins, payload, time.Time{})
}

func writeSkinExport(w http.ResponseWriter, format, filename, title, view string, pool PoolManifest, skins []Skin, payload any, capturedAt time.Time) {
	switch format {
	case "json":
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.json"`, filename))
		respondJSON(w, payload)
	case "csv":
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.csv"`, filename))
		_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
		writer := csv.NewWriter(w)
		_ = writer.Write([]string{"skin_id", "skin_name", "champion", "rarity", "owned", "pool_name"})
		for _, skin := range skins {
			_ = writer.Write([]string{fmt.Sprint(skin.ID), safeCSVCell(skin.Name), safeCSVCell(skin.ChampionName), safeCSVCell(skin.Rarity), fmt.Sprint(skin.Owned), safeCSVCell(skin.PoolName)})
		}
		writer.Flush()
	case "html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.html"`, filename))
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'")
		_ = exportHTMLTemplate.Execute(w, map[string]any{"Title": title, "Pool": pool, "View": view, "Items": skins, "CapturedAt": capturedAt, "ExportedAt": time.Now().Format("2006-01-02 15:04:05")})
	default:
		http.Error(w, "format 必须为 json、csv 或 html", http.StatusBadRequest)
	}
}

func safeCSVCell(value string) string {
	value = strings.ReplaceAll(value, "\x00", "")
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed != "" && strings.ContainsRune("=+-@", []rune(trimmed)[0]) {
		return "'" + value
	}
	return value
}

var exportHTMLTemplate = template.Must(template.New("export").Parse(`<!doctype html><html lang="zh-CN"><meta charset="utf-8"><title>{{.Title}}</title><style>body{font:14px/1.5 system-ui,sans-serif;color:#171717;margin:32px}h1{font-size:24px}p{color:#555}table{width:100%;border-collapse:collapse}th,td{padding:8px;border-bottom:1px solid #ddd;text-align:left}th{background:#f5f5f5}</style><h1>{{.Title}}</h1><p>奖池：{{.Pool.Name}} · {{.Pool.Version}}{{if not .CapturedAt.IsZero}} · 快照时间 {{.CapturedAt.Format "2006-01-02 15:04:05"}}{{end}} · 导出时间 {{.ExportedAt}}</p><table><thead><tr><th>ID</th><th>皮肤</th><th>英雄</th><th>品质</th><th>状态</th></tr></thead><tbody>{{range .Items}}<tr><td>{{.ID}}</td><td>{{.Name}}</td><td>{{.ChampionName}}</td><td>{{.RarityTier}}</td><td>{{if .Owned}}已拥有{{else}}三合一剩余{{end}}</td></tr>{{end}}</tbody></table></html>`))

func (a *app) handleSnapshots(w http.ResponseWriter, r *http.Request) {
	if a.storage == nil {
		http.Error(w, "本地历史不可用", http.StatusServiceUnavailable)
		return
	}
	if r.Method == http.MethodGet {
		respondJSON(w, map[string]any{"items": a.storage.listSnapshots()})
		return
	}
	a.mu.RLock()
	if !a.calculationOKLocked() {
		a.mu.RUnlock()
		http.Error(w, "只有完整核对后的结果才能保存快照", http.StatusConflict)
		return
	}
	snapshot := a.snapshotLocked()
	pool := a.pools[a.poolID]
	a.mu.RUnlock()
	record, err := a.storage.saveSnapshot(snapshot, pool)
	if err != nil {
		http.Error(w, "保存快照失败："+err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, snapshotSummary(record))
}

func (a *app) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if a.storage == nil {
		http.Error(w, "本地历史不可用", http.StatusServiceUnavailable)
		return
	}
	record, err := a.storage.loadSnapshot(r.PathValue("id"))
	if err != nil {
		http.Error(w, "快照不存在", http.StatusNotFound)
		return
	}
	respondJSON(w, record)
}

func (a *app) handleSnapshotExport(w http.ResponseWriter, r *http.Request) {
	if a.storage == nil {
		http.Error(w, "本地历史不可用", http.StatusServiceUnavailable)
		return
	}
	record, err := a.storage.loadSnapshot(r.PathValue("id"))
	if err != nil {
		http.Error(w, "快照不存在", http.StatusNotFound)
		return
	}
	skins := make([]Skin, 0, len(record.Owned)+len(record.Remaining))
	appendSkins := func(items []snapshotSkin, owned bool) {
		for _, item := range items {
			skins = append(skins, Skin{ID: item.ID, Name: item.Name, ChampionName: item.ChampionName, Rarity: item.Rarity, Owned: owned, PoolName: item.PoolName})
		}
	}
	appendSkins(record.Owned, true)
	appendSkins(record.Remaining, false)
	pool := PoolManifest{ID: record.PoolID, Name: record.PoolName, Version: record.PoolVersion, Hash: record.PoolHash}
	payload := map[string]any{
		"schemaVersion": 1,
		"exportedAt":    time.Now().UTC(),
		"snapshot":      snapshotSummary(record),
		"owned":         record.Owned,
		"remaining":     record.Remaining,
	}
	filename := fmt.Sprintf("lol-loot-snapshot-%s", record.CapturedAt.Local().Format("20060102-150405"))
	writeSkinExport(w, strings.ToLower(r.URL.Query().Get("format")), filename, "Deep Legends 历史快照", "snapshot", pool, skins, payload, record.CapturedAt.Local())
}

func (a *app) handleSnapshotDiff(w http.ResponseWriter, r *http.Request) {
	if a.storage == nil {
		http.Error(w, "本地历史不可用", http.StatusServiceUnavailable)
		return
	}
	from, err := a.storage.loadSnapshot(r.PathValue("id"))
	if err != nil {
		http.Error(w, "起始快照不存在", http.StatusNotFound)
		return
	}
	to, err := a.storage.loadSnapshot(r.URL.Query().Get("against"))
	if err != nil {
		http.Error(w, "对比快照不存在", http.StatusNotFound)
		return
	}
	diff, err := diffSnapshots(from, to)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	respondJSON(w, diff)
}

func (a *app) handlePrivacy(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, map[string]any{
		"localOnly": false, "requiresPassword": false, "uploadsData": false,
		"mayhemRatingDisclosure": "打开国服玩家总览时，会把该玩家的昵称与 Tag 发送给 ARAMKit 查询海克斯大乱斗第三方估算分；不携带本机账号凭据、Cookie 或客户端令牌，结果只在内存缓存 10 分钟，韩服不发送。",
		"reads":                  []string{"当前 League Client 中召唤师的公开显示信息、排位与英雄熟练度", "League Client 中的最近战绩、对局参与者与当前游戏流程", "国服跨服搜索时，向本机 RiotClientServices 的 /player-account/aliases/v1/lookup 提交被查询的 Riot ID 并读取 PUUID；它与 League Client 使用独立端口和令牌，结果只在内存中用于所选腾讯 SGP 查询", "League Client 中的好友分组、好友在线状态与所在对局信息（只读，不提供增删改）", "League Client 中的皮肤目录", "当前账号永久拥有的皮肤 ID", "本机战利品与待领取奖励", "皮肤、装备与符文图标资源"},
		"explicitWrites":         []string{"设置生涯头像：只写 profileIconId 一个字段，不改其它资料", "设置段位旗：保留头像框偏好，不动表情、守卫皮肤和其它槽位。生涯旗帜目前只读浏览；点击检测客户端写入支持后会短暂切换到另一个已拥有头像和旗帜并恢复原值，旗帜保留槽位其它字段；头像失败时依次探测资料接口与签名库存来源，诊断不记录库存令牌", "只有在英雄选择阶段点击“应用到客户端”后才新建一页带“[DL] ”前缀的可编辑符文并设为当前页；页数已满时会删除本工具此前创建的、带精确“[DL] ”前缀且未被客户端显式标记为不可删除的最旧符文页来腾位置，每次最多回收 5 页，绝不删除其它符文页，也不更新或覆盖任何已有页", "只有在英雄选择阶段点击“应用装备方案”后才创建或更新客户端装备方案", "只有点击“回放”后才让英雄联盟客户端下载或启动对应回放", "只有在工具生涯页点击应用后，才修改生涯背景皮肤", "只有在工具生涯页点击应用后，才修改聊天在线状态、个性签名与展示段位", "只有在工具生涯页逐项二次确认后，才修改头像框、挑战勋章、头衔或表情轮盘", "只有在工具领奖页明确勾选并开始领取后，才逐项领取奖励账本、任务或活动奖励；失败不会中断其它条目", "只有在工具维护页明确点击后，才重启、结束或启动客户端界面、关闭客户端或主动断开助手连接", "只有在工具维护页明确点击后，才修改当前客户端设置文件的只读属性；写前校验安装目录与符号链接"},
		"automaticWrites":        []string{"默认关闭；仅在工具自动页逐条开启后，才会在 ReadyCheck 阶段自动接受对局", "默认关闭；仅在工具自动页逐条开启后，才会在 EndOfGame 阶段自动发起再来一局", "默认关闭；仅在工具自动页逐条开启后，才会在 Reconnect 阶段自动请求断线重连", "默认关闭；仅在工具自动页逐条开启后，才会在结算阶段按所选策略自动点赞，且不会投给敌方", "默认关闭；仅在工具自动页逐条开启后，才会跳过任务庆祝", "默认关闭；仅在工具自动页逐条开启后，才会在大乱斗类英雄选择中播报阵营位置", "默认关闭；仅在工具自动页逐条开启后，才会把房主随机转交给其他房间成员", "默认关闭；仅在工具自动页逐条开启并配置队列策略后，才会接受或拒绝房间邀请", "默认关闭；仅在工具自动页逐条开启后，才会在满足人数时开始一次匹配；取消后不会自动重排", "征召托管默认开启，但禁用/选用序列为空时不发送任何写请求；仅在工具征召页启用对应序列并配置英雄后，才会在英雄选择阶段自动禁用、提前预选、自动选用或从备战席交换英雄；提前预选随自动选用生效"},
		"externalReads":          []string{"为了让职业选手账号在改名后仍能认出来，会按已解析的稳定标识向 Riot 官方接口反查当前 Riot ID；只针对内置名单里的职业选手，不针对你和你的好友", "展示臻彩时按炫彩 ID、皮肤原画本机读取失败时按皮肤 ID，从固定的腾讯官方图片域名读取公开原画；不会发送账号信息、客户端令牌或收藏数据", "“英雄”页统计与图标只向固定 OP.GG、腾讯官方图片与 Riot Data Dragon 公共地址请求，已连接客户端时图标优先直接读取本机客户端", "查询韩服玩家时，向固定的 Riot 官方接口域名发送该玩家的 Riot ID 与内嵌 API Key；只填名称时另向 op.gg 公开搜索发送名称以补全编号", "为生成绝活哥符文推荐，会把从 OP.GG 韩服专家榜取得的第三方玩家 Riot ID 发送给 Riot 官方接口，并读取其公开对局以提取该英雄符文；不携带本机账号、Cookie 或客户端令牌，结果在进程内缓存 6 小时", "打开韩服玩家总览时，向 op.gg 公开页发送该玩家的 Riot ID 与 PUUID，换取每场对局的平均段位（当前登录的国服服务器改为向本机客户端逐人查询，不外发；跨服不查询排位）；失败时该行显示“—”，不影响战绩本身。以上请求都不携带本机账号、Cookie 或客户端令牌", "打开国服玩家总览时，向 ARAMKit 发送该玩家的昵称与 Tag，读取海克斯大乱斗第三方估算分；不携带本机账号凭据、Cookie 或客户端令牌，成功结果只在进程内缓存 10 分钟，失败缓存 5 分钟，韩服不发送"},
		"stores":                 []string{"内置职业选手名单里人工核对账号的稳定标识锚点（本地缓存，30 天），按稳定标识反查的当前 Riot ID 与种子单双排段位分别缓存 6 小时", "公开韩服查询的身份解析与对局内容（含被查询玩家及公开对局参与者的 Riot ID/PUUID），保存在有界本地缓存，文件名哈希但正文不加密", "随机脱敏账号标识", "已拥有和三合一剩余的本地历史快照", "按对局 ID 与加盐脱敏账号标识记录的胜点变化", "用户导入的奖池清单", "不含令牌和账号名的诊断事件"},
		"neverStores":            []string{"QQ 账号或密码", "LCU 临时令牌（仅在当前进程内存中短暂使用）", "本机当前账号的 PUUID、AccountID、SummonerID 不外发给第三方，也不出现在前端和诊断日志；公开韩服查询解析出的 PUUID 会在本机缓存目录保存复用（见存储声明），查询目标可按外部读取声明发送至 Riot 官方和 OP.GG", "客户端完整命令行", "战利品、待领取奖励、任务与活动奖励明细"},
	})
}
