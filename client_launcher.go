package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

type clientInstallation struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
	Location    string `json:"location,omitempty"`
	Available   bool   `json:"available"`
	executable  string
	shortcut    string
	arguments   []string
}

func (a *app) handleClientInstallations(w http.ResponseWriter, _ *http.Request) {
	items := detectClientInstallations()
	for index := range items {
		items[index].executable = ""
		items[index].shortcut = ""
		items[index].arguments = nil
	}
	respondJSON(w, map[string]any{"items": items, "count": len(items)})
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
	request.ID = strings.TrimSpace(request.ID)
	if request.ID == "" {
		http.Error(w, "请选择要启动的客户端", http.StatusBadRequest)
		return
	}
	installation, ok := findClientInstallation(request.ID)
	if !ok || !installation.Available || (installation.executable == "" && installation.shortcut == "") {
		http.Error(w, "没有找到这个客户端，请先在设置中检查安装位置", http.StatusNotFound)
		return
	}
	if err := launchClientInstallation(installation); err != nil {
		http.Error(w, "客户端启动失败，请尝试从桌面快捷方式启动", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func classifyClientShortcut(name string) (id, displayName, kind, description string) {
	name = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(name, ".lnk")))
	if strings.Contains(name, "卸载") || strings.Contains(name, "uninstall") || strings.Contains(name, "uninstaller") {
		return "", "", "", ""
	}
	switch {
	case strings.Contains(name, "wegame"):
		return "wegame", "WeGame", "wegame", "通过 Windows 快捷方式打开 WeGame"
	case strings.Contains(name, "英雄联盟") || strings.Contains(name, "league of legends") || strings.Contains(name, "tcls"):
		return "tcls", "英雄联盟", "tcls", "通过 Windows 快捷方式启动国服客户端"
	case strings.Contains(name, "riot client") || strings.Contains(name, "riot客户端"):
		return "riot", "Riot 客户端", "riot", "通过 Windows 快捷方式启动 Riot 客户端"
	default:
		return "", "", "", ""
	}
}

func findClientInstallation(id string) (clientInstallation, bool) {
	for _, installation := range detectClientInstallations() {
		if installation.ID == id {
			return installation, true
		}
	}
	return clientInstallation{}, false
}
