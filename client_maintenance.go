package main

import (
	"context"
	"net/http"
	"time"
)

func (a *app) handleClientMaintenance(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Action string `json:"action"`
	}
	if err := decodeJSONRequest(r, &request, 4096); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if request.Action == "disconnect" {
		a.mu.Lock()
		client := a.lcu
		a.manualDisconnected = true
		a.mu.Unlock()
		if client != nil {
			a.disconnectClient(client, "已按你的要求断开；点击顶部刷新可重新连接")
		}
		respondJSON(w, map[string]bool{"ok": true})
		return
	}
	paths := map[string]string{
		"restart-ux":  "/riotclient/kill-and-restart-ux",
		"kill-ux":     "/riotclient/kill-ux",
		"launch-ux":   "/riotclient/launch-ux",
		"quit-client": "/process-control/v1/process/quit",
	}
	path, ok := paths[request.Action]
	if !ok {
		http.Error(w, "未知维护动作", http.StatusBadRequest)
		return
	}
	client, _, err := a.gameplayClient()
	if err != nil {
		http.Error(w, "未连接英雄联盟客户端", http.StatusConflict)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	if err := client.RequestJSON(ctx, http.MethodPost, path, nil, nil); err != nil {
		http.Error(w, "客户端维护动作执行失败", http.StatusBadGateway)
		return
	}
	respondJSON(w, map[string]bool{"ok": true})
}
