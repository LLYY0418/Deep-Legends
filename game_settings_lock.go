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
		info, statErr := os.Lstat(location.file)
		if statErr == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			status.SettingsKnown = true
			status.SettingsLocked = info.Mode().Perm()&0o222 == 0
		} else if statErr != nil {
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
		respondJSON(w, rigStatus{Reason: "未连接英雄联盟客户端"})
		return
	}
	respondJSON(w, readRigStatus(r.Context(), client))
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
		http.Error(w, "未连接英雄联盟客户端", http.StatusConflict)
		return
	}
	location, err := locateGameSettings(r.Context(), client)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	info, err := os.Lstat(location.file)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || !pathWithin(location.allowedRoot, location.file) {
		http.Error(w, "设置文件不可安全操作", http.StatusUnprocessableEntity)
		return
	}
	mode := os.FileMode(0o644)
	if request.Locked {
		mode = 0o444
	}
	if err := os.Chmod(location.file, mode); err != nil {
		http.Error(w, "无法修改设置文件只读状态", http.StatusServiceUnavailable)
		return
	}
	respondJSON(w, readRigStatus(r.Context(), client))
}
