package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const clientLaunchCooldown = 6 * time.Second

type clientLaunchState struct {
	mu            sync.Mutex
	inFlight      map[string]bool
	cooldownUntil map[string]time.Time
}

type clientInstallation struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Kind             string `json:"kind"`
	Description      string `json:"description"`
	Location         string `json:"location,omitempty"`
	Available        bool   `json:"available"`
	executable       string
	shortcut         string
	arguments        []string
	launchCandidates []clientLaunchCandidate
}

type clientLaunchCandidate struct {
	Source     string
	executable string
	shortcut   string
	arguments  []string
}

type clientLaunchFailure struct {
	Source    string
	Stage     string
	ErrorCode uint32
}

type clientLaunchResult struct {
	Cancelled             bool
	AlreadyOpen           bool
	PathClass             string
	ProductSettingsExists bool
	ProductInstallClass   string
	Source                string
	Failures              []clientLaunchFailure
}

func buildDetectedClientInstallations(gameRoots, riotExecutables []string, shortcuts []clientInstallation, regular func(string) bool) []clientInstallation {
	byID := make(map[string]clientInstallation)
	add := func(installation clientInstallation, source string) {
		validShortcut := installation.shortcut != "" && filepath.IsAbs(installation.shortcut) && regular(installation.shortcut)
		validExecutable := installation.executable != "" && filepath.IsAbs(installation.executable) && regular(installation.executable)
		if installation.ID == "" || (!validShortcut && !validExecutable) {
			return
		}
		if validShortcut {
			installation.Location = "Windows 快捷方式 · " + strings.TrimSuffix(filepath.Base(installation.shortcut), filepath.Ext(installation.shortcut))
		} else if installation.Location == "" {
			installation.Location = filepath.Dir(installation.executable)
		}
		mergeClientLaunchCandidate(byID, installation, clientLaunchCandidate{
			Source: source, executable: installation.executable, shortcut: installation.shortcut, arguments: append([]string(nil), installation.arguments...),
		})
	}

	for _, root := range gameRoots {
		for _, candidate := range []struct {
			source     string
			executable string
		}{
			{"launcher", filepath.Join(root, "Launcher", "Client.exe")},
			{"tcls", filepath.Join(root, "TCLS", "Client.exe")},
		} {
			add(clientInstallation{ID: "tcls", Name: "TCLS 客户端", Kind: "tcls", Description: "直接启动腾讯英雄联盟客户端", executable: candidate.executable}, candidate.source)
		}
	}
	for _, candidate := range riotExecutables {
		if isTencentInstallPath(candidate) {
			continue
		}
		add(clientInstallation{ID: "riot", Name: "Riot 客户端", Kind: "riot", Description: "启动 Riot 英雄联盟客户端", executable: candidate, arguments: []string{"--launch-product=league_of_legends", "--launch-patchline=live"}}, "riot")
	}
	for _, installation := range shortcuts {
		if installation.ID == "riot" && isTencentInstallPath(installation.shortcut) {
			continue
		}
		source := "shortcut"
		if installation.ID == "wegame" && installation.executable != "" {
			source = "registry"
		}
		add(installation, source)
	}

	order := map[string]int{"tcls": 0, "wegame": 1, "riot": 2}
	result := make([]clientInstallation, 0, len(byID))
	for _, installation := range byID {
		result = append(result, installation)
	}
	sort.Slice(result, func(left, right int) bool { return order[result[left].ID] < order[result[right].ID] })
	return result
}

type clientInstallationScan struct {
	RegistryWeGameFound       bool
	PlatformSupported         bool
	RegistryRootFound         bool
	RegistryLauncherFound     bool
	RegistryTCLSFound         bool
	RegistryLeagueClientFound bool
	ShortcutCandidates        int
}

func (a *app) handleClientInstallations(w http.ResponseWriter, r *http.Request) {
	detected, scan := a.detectedClientInstallationsWithScanCached(r.URL.Query().Get("force") == "1")
	a.recordClientInstallationScan(scan, detected)
	items := append([]clientInstallation(nil), detected...)
	for index := range items {
		items[index].Location = ""
		items[index].executable = ""
		items[index].shortcut = ""
		items[index].arguments = nil
		items[index].launchCandidates = nil
	}
	respondJSON(w, map[string]any{"items": items, "count": len(items)})
}

const clientInstallationScanCacheTTL = 60 * time.Second

func (a *app) detectedClientInstallationsWithScanCached(force bool) ([]clientInstallation, clientInstallationScan) {
	if a == nil {
		return detectClientInstallationsWithScan()
	}
	a.clientInstallationsMu.Lock()
	if !force && !a.clientInstallationsCacheAt.IsZero() && time.Since(a.clientInstallationsCacheAt) < clientInstallationScanCacheTTL {
		items := append([]clientInstallation(nil), a.clientInstallationsCache...)
		scan := a.clientInstallationsCacheScan
		a.clientInstallationsMu.Unlock()
		return items, scan
	}
	a.clientInstallationsMu.Unlock()
	detected, scan := a.detectedClientInstallationsWithScan()
	a.clientInstallationsMu.Lock()
	a.clientInstallationsCache = append([]clientInstallation(nil), detected...)
	a.clientInstallationsCacheScan = scan
	a.clientInstallationsCacheAt = time.Now()
	a.clientInstallationsMu.Unlock()
	return detected, scan
}

func (a *app) handleClientLaunch(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ID string `json:"id"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "启动请求格式无效", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		http.Error(w, "启动请求格式无效", http.StatusBadRequest)
		return
	}
	request.ID = strings.TrimSpace(request.ID)
	if request.ID == "" {
		http.Error(w, "请选择要启动的客户端", http.StatusBadRequest)
		return
	}
	installation, ok := a.findClientInstallation(request.ID)
	if !ok || !installation.Available || !installation.hasLaunchCandidates() {
		a.recordClientLaunch(request.ID, "unavailable")
		http.Error(w, "没有找到这个客户端，请先在设置中检查安装位置", http.StatusNotFound)
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	if !a.beginClientLaunch(installation.ID) {
		a.recordClientLaunch(installation.ID, "busy")
		http.Error(w, "客户端正在启动，请稍后再试", http.StatusTooManyRequests)
		return
	}
	launched := false
	defer func() { a.finishClientLaunch(installation.ID, launched) }()
	a.recordClientLaunch(installation.ID, "requested")
	result, err := a.launchDetectedClientInstallation(installation)
	for _, failure := range result.Failures {
		a.recordClientLaunchAttempt(installation.ID, failure)
	}
	if installation.ID == "riot" {
		a.recordDiagnostic(map[string]any{"event": "client_launch", "client_id": "riot", "result": "validated", "source": safeClientLaunchSource(result.PathClass), "is_tencent_path": false, "product_settings_exists": result.ProductSettingsExists, "product_install_class": result.ProductInstallClass})
	}
	if result.Cancelled {
		a.recordClientLaunch(installation.ID, "cancelled")
		respondJSON(w, map[string]any{"cancelled": true})
		return
	}
	if result.AlreadyOpen {
		a.recordClientLaunch(installation.ID, "already-open")
		respondJSON(w, map[string]any{"alreadyOpen": true})
		return
	}
	if err != nil {
		a.recordClientLaunch(installation.ID, "failed")
		message := "客户端启动失败，请尝试从桌面快捷方式启动"
		if errors.Is(err, errRiotProductUnavailable) {
			message = "Riot 客户端里没有可用的英雄联盟安装"
		}
		http.Error(w, message, http.StatusInternalServerError)
		return
	}
	launched = true
	a.recordClientLaunchStarted(installation.ID, result.Source)
	a.requestRefresh()
	w.WriteHeader(http.StatusAccepted)
}

func (a *app) beginClientLaunch(id string) bool {
	a.clientLaunch.mu.Lock()
	defer a.clientLaunch.mu.Unlock()
	if a.clientLaunch.inFlight == nil {
		a.clientLaunch.inFlight = map[string]bool{}
		a.clientLaunch.cooldownUntil = map[string]time.Time{}
	}
	if a.clientLaunch.inFlight[id] || time.Now().Before(a.clientLaunch.cooldownUntil[id]) {
		return false
	}
	a.clientLaunch.inFlight[id] = true
	return true
}
func (a *app) finishClientLaunch(id string, succeeded bool) {
	a.clientLaunch.mu.Lock()
	defer a.clientLaunch.mu.Unlock()
	delete(a.clientLaunch.inFlight, id)
	if succeeded {
		a.clientLaunch.cooldownUntil[id] = time.Now().Add(clientLaunchCooldown)
	} else {
		delete(a.clientLaunch.cooldownUntil, id)
	}
}

func (a *app) detectedClientInstallations() []clientInstallation {
	items, _ := a.detectedClientInstallationsWithScanCached(false)
	return items
}

func (a *app) detectedClientInstallationsWithScan() ([]clientInstallation, clientInstallationScan) {
	if a != nil && a.clientInstallations != nil {
		return a.clientInstallations(), clientInstallationScan{PlatformSupported: true}
	}
	return detectClientInstallationsWithScan()
}

func (a *app) launchDetectedClientInstallation(installation clientInstallation) (clientLaunchResult, error) {
	if a != nil && a.clientLauncher != nil {
		return a.clientLauncher(installation)
	}
	return launchClientInstallation(installation)
}

func (installation clientInstallation) hasLaunchCandidates() bool {
	return len(installation.launchCandidates) != 0 || installation.shortcut != "" || installation.executable != ""
}

func (installation clientInstallation) candidates() []clientLaunchCandidate {
	if len(installation.launchCandidates) != 0 {
		return append([]clientLaunchCandidate(nil), installation.launchCandidates...)
	}
	if installation.shortcut != "" {
		return []clientLaunchCandidate{{Source: "shortcut", shortcut: installation.shortcut}}
	}
	if installation.executable != "" {
		source := safeClientLaunchSource(installation.Kind)
		return []clientLaunchCandidate{{Source: source, executable: installation.executable, arguments: append([]string(nil), installation.arguments...)}}
	}
	return nil
}

func mergeClientLaunchCandidate(byID map[string]clientInstallation, installation clientInstallation, candidate clientLaunchCandidate) {
	candidate.Source = safeClientLaunchSource(candidate.Source)
	existing, exists := byID[installation.ID]
	if !exists {
		existing = installation
		existing.Available = true
		existing.launchCandidates = nil
		existing.executable = candidate.executable
		existing.shortcut = candidate.shortcut
		existing.arguments = append([]string(nil), candidate.arguments...)
	}
	for _, current := range existing.launchCandidates {
		if sameClientLaunchCandidate(current, candidate) {
			byID[installation.ID] = existing
			return
		}
	}
	existing.launchCandidates = append(existing.launchCandidates, candidate)
	byID[installation.ID] = existing
}

func sameClientLaunchCandidate(left, right clientLaunchCandidate) bool {
	if !strings.EqualFold(left.executable, right.executable) || !strings.EqualFold(left.shortcut, right.shortcut) || len(left.arguments) != len(right.arguments) {
		return false
	}
	for index := range left.arguments {
		if left.arguments[index] != right.arguments[index] {
			return false
		}
	}
	return true
}

func launchClientCandidates(installation clientInstallation, attempt func(clientLaunchCandidate) (clientLaunchFailure, error)) (clientLaunchResult, error) {
	result := clientLaunchResult{}
	candidates := installation.candidates()
	if len(candidates) == 0 {
		return result, errors.New("client installation has no launch candidates")
	}
	var lastErr error
	for _, candidate := range candidates {
		candidate.Source = safeClientLaunchSource(candidate.Source)
		failure, err := attempt(candidate)
		if err == nil {
			result.Source = candidate.Source
			return result, nil
		}
		failure.Source = candidate.Source
		failure.Stage = safeClientLaunchStage(failure.Stage)
		result.Failures = append(result.Failures, failure)
		if failure.ErrorCode == 1223 {
			result.Cancelled = true
			return result, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("all client launch candidates failed")
	}
	return result, lastErr
}

func (a *app) findClientInstallation(id string) (clientInstallation, bool) {
	for _, installation := range a.detectedClientInstallations() {
		if installation.ID == id {
			return installation, true
		}
	}
	return clientInstallation{}, false
}

func (a *app) recordClientLaunch(id, result string) {
	event, safeID := safeClientLaunchEvent(id)
	a.recordDiagnostic(map[string]any{
		"event": event, "client_id": safeID, "result": result,
	})
}

func (a *app) recordClientLaunchAttempt(id string, failure clientLaunchFailure) {
	event, safeID := safeClientLaunchEvent(id)
	a.recordDiagnostic(map[string]any{
		"event": event, "client_id": safeID, "result": "attempt_failed",
		"source": safeClientLaunchSource(failure.Source), "stage": safeClientLaunchStage(failure.Stage), "error_code": failure.ErrorCode,
	})
}

func (a *app) recordClientLaunchStarted(id, source string) {
	a.startClientLaunchTiming(id, time.Now())
	event, safeID := safeClientLaunchEvent(id)
	a.recordDiagnostic(map[string]any{
		"event": event, "client_id": safeID, "result": "started", "source": safeClientLaunchSource(source),
	})
}

func safeClientLaunchEvent(id string) (string, string) {
	event := "client_launch"
	if id == "tcls" {
		event = "official_login_launch"
	}
	switch id {
	case "tcls", "wegame", "riot":
		return event, id
	default:
		return event, "unknown"
	}
}

func safeClientLaunchSource(source string) string {
	switch source {
	case "registry", "shortcut", "launcher", "tcls", "riot", "rc_default", "rc_live", "installs-json-other", "drive-guess":
		return source
	default:
		return "unknown"
	}
}

func safeClientLaunchStage(stage string) string {
	switch stage {
	case "validate", "encode", "shell-execute":
		return stage
	default:
		return "unknown"
	}
}

func (a *app) recordClientInstallationScan(scan clientInstallationScan, items []clientInstallation) {
	detected := make(map[string]bool, len(items))
	for _, item := range items {
		switch item.ID {
		case "tcls", "wegame", "riot":
			detected[item.ID] = true
		}
	}
	ids := make([]string, 0, len(detected))
	for _, id := range []string{"tcls", "wegame", "riot"} {
		if detected[id] {
			ids = append(ids, id)
		}
	}
	result := "empty"
	if !scan.PlatformSupported {
		result = "unsupported"
	} else if len(ids) != 0 {
		result = "detected"
	}
	a.recordDiagnostic(map[string]any{
		"event":                        "client_installations_scan",
		"result":                       result,
		"platform_supported":           scan.PlatformSupported,
		"registry_wegame_found":        scan.RegistryWeGameFound,
		"registry_root_found":          scan.RegistryRootFound,
		"registry_launcher_found":      scan.RegistryLauncherFound,
		"registry_tcls_found":          scan.RegistryTCLSFound,
		"registry_league_client_found": scan.RegistryLeagueClientFound,
		"shortcut_candidates":          scan.ShortcutCandidates,
		"detected_ids":                 ids,
		"count":                        len(ids),
	})
}

func classifyClientShortcut(name string) (id, displayName, kind, description string) {
	name = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(name, ".lnk")))
	if strings.Contains(name, "卸载") || strings.Contains(name, "uninstall") || strings.Contains(name, "uninstaller") {
		return "", "", "", ""
	}
	if strings.Contains(name, "wegame") {
		return "wegame", "WeGame", "wegame", "打开 WeGame"
	}
	switch {
	case strings.Contains(name, "英雄联盟") || strings.Contains(name, "league of legends") || strings.Contains(name, "tcls"):
		return "tcls", "英雄联盟", "tcls", "通过 Windows 快捷方式启动国服客户端"
	case strings.Contains(name, "riot client") || strings.Contains(name, "riot客户端"):
		return "riot", "Riot 客户端", "riot", "通过 Windows 快捷方式启动 Riot 客户端"
	default:
		return "", "", "", ""
	}
}
