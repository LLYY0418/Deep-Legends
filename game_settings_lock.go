package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type rigStatus struct {
	Connected      bool   `json:"connected"`
	Region         string `json:"region,omitempty"`
	Platform       string `json:"platform,omitempty"`
	InstallRoot    string `json:"installRoot,omitempty"`
	ConfigRoot     string `json:"configRoot,omitempty"`
	SettingsFile   string `json:"settingsFile,omitempty"`
	SettingsKnown  bool   `json:"settingsKnown"`
	SettingsLocked bool   `json:"settingsLocked"`
	UXState        string `json:"uxState,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

type settingsLocation struct {
	installRoot string
	configRoot  string
	file        string
	allowedRoot string
}

var errUnsafeSettingsFile = errors.New("设置文件不可安全操作")

func locateGameSettings(ctx context.Context, client *LCUClient) (settingsLocation, error) {
	if client == nil {
		return settingsLocation{}, errors.New("未连接英雄联盟客户端")
	}
	var payload any
	if err := client.RequestJSON(ctx, http.MethodGet, "/data-store/v1/install-dir", nil, &payload); err != nil {
		return settingsLocation{}, errors.New("无法定位安装目录")
	}
	installRoot := ""
	switch typed := payload.(type) {
	case string:
		installRoot = strings.TrimSpace(typed)
	case map[string]any:
		installRoot = anyString(typed, "installDir", "path", "directory")
	}
	if installRoot == "" {
		return settingsLocation{}, errors.New("无法定位安装目录")
	}
	installRoot = filepath.Clean(installRoot)
	region, _ := client.platformInfo()
	configRoot := filepath.Join(installRoot, "Config")
	allowedRoot := installRoot
	if strings.EqualFold(region, "TENCENT") {
		allowedRoot = filepath.Clean(filepath.Join(installRoot, ".."))
		configRoot = filepath.Join(allowedRoot, "Game", "Config")
	}
	location := settingsLocation{
		installRoot: installRoot,
		configRoot:  filepath.Clean(configRoot),
		file:        filepath.Clean(filepath.Join(configRoot, "PersistedSettings.json")),
		allowedRoot: filepath.Clean(allowedRoot),
	}
	if !pathWithin(location.allowedRoot, location.file) {
		return settingsLocation{}, errors.New("设置文件路径超出客户端安装目录")
	}
	return location, nil
}

func pathWithin(root, target string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	if err != nil || relative == ".." || filepath.IsAbs(relative) {
		return false
	}
	return !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func safeSettingsFile(location settingsLocation) (string, os.FileInfo, error) {
	if !pathWithin(location.allowedRoot, location.file) {
		return "", nil, errUnsafeSettingsFile
	}
	entry, err := os.Lstat(location.file)
	if err != nil {
		return "", nil, err
	}
	if !entry.Mode().IsRegular() || entry.Mode()&os.ModeSymlink != 0 {
		return "", nil, errUnsafeSettingsFile
	}
	resolvedRoot, err := filepath.EvalSymlinks(location.allowedRoot)
	if err != nil {
		return "", nil, err
	}
	resolvedFile, err := filepath.EvalSymlinks(location.file)
	if err != nil {
		return "", nil, err
	}
	if !pathWithin(resolvedRoot, resolvedFile) {
		return "", nil, errUnsafeSettingsFile
	}
	info, err := os.Stat(resolvedFile)
	if err != nil {
		return "", nil, err
	}
	if !info.Mode().IsRegular() {
		return "", nil, errUnsafeSettingsFile
	}
	return resolvedFile, info, nil
}

func readRigStatus(ctx context.Context, client *LCUClient) rigStatus {
	status := rigStatus{}
	if client == nil {
		status.Reason = "未连接英雄联盟客户端"
		return status
	}
	status.Connected = true
	status.Region, status.Platform = client.platformInfo()
	location, err := locateGameSettings(ctx, client)
	if err != nil {
		status.Reason = err.Error()
	} else {
		status.InstallRoot = location.installRoot
		status.ConfigRoot = location.configRoot
		status.SettingsFile = location.file
		_, info, statErr := safeSettingsFile(location)
		if statErr == nil {
			status.SettingsKnown = true
			status.SettingsLocked = info.Mode().Perm()&0o222 == 0
		} else if !errors.Is(statErr, errUnsafeSettingsFile) {
			status.Reason = "设置文件尚未生成或无法读取"
		} else {
			status.Reason = "设置文件不是可操作的普通文件"
		}
	}
	var ux any
	if err := client.RequestJSON(ctx, http.MethodGet, "/riotclient/ux-state", nil, &ux); err == nil {
		switch typed := ux.(type) {
		case string:
			status.UXState = strings.TrimSpace(typed)
		case map[string]any:
			status.UXState = anyString(typed, "state", "phase")
		}
	}
	return status
}

func (a *app) handleRigStatus(w http.ResponseWriter, r *http.Request) {
	client, _, err := a.gameplayClient()
	if err != nil {
		a.recordDiagnostic(map[string]any{"event": "rig_status_read", "result": "not-connected"})
		respondJSON(w, rigStatus{Reason: "未连接英雄联盟客户端"})
		return
	}
	status := readRigStatus(r.Context(), client)
	result := "ok"
	if status.InstallRoot == "" {
		result = "locate-failed"
	}
	a.recordDiagnostic(map[string]any{"event": "rig_status_read", "result": result})
	respondJSON(w, status)
}

func (a *app) handleSettingsLock(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Locked bool `json:"locked"`
	}
	if err := decodeJSONRequest(r, &request, 4096); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	client, _, err := a.gameplayClient()
	if err != nil {
		a.recordDiagnostic(map[string]any{"event": "rig_maintenance", "action": "settings-lock", "result": "not-connected"})
		http.Error(w, "未连接英雄联盟客户端", http.StatusConflict)
		return
	}
	location, err := locateGameSettings(r.Context(), client)
	if err != nil {
		a.recordDiagnostic(map[string]any{"event": "rig_maintenance", "action": "settings-lock", "result": "locate-failed"})
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	resolvedFile, _, err := safeSettingsFile(location)
	if err != nil {
		a.recordDiagnostic(map[string]any{"event": "rig_maintenance", "action": "settings-lock", "result": "unsafe-file"})
		http.Error(w, "设置文件不可安全操作", http.StatusUnprocessableEntity)
		return
	}
	mode := os.FileMode(0o644)
	if request.Locked {
		mode = 0o444
	}
	if err := os.Chmod(resolvedFile, mode); err != nil {
		a.recordDiagnostic(map[string]any{"event": "rig_maintenance", "action": "settings-lock", "result": "failed"})
		http.Error(w, "无法修改设置文件只读状态", http.StatusServiceUnavailable)
		return
	}
	a.recordDiagnostic(map[string]any{"event": "rig_maintenance", "action": "settings-lock", "result": "ok"})
	respondJSON(w, readRigStatus(r.Context(), client))
}
