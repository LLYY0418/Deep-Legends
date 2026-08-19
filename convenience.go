package main

// convenience.go 迁移自 LeagueAkari 的“便捷设置”：由本地服务监听客户端
// gameflow 阶段变化，自动执行接受对局、结算后再来一局与断线重连。
// 所有动作只调用本机客户端接口（127.0.0.1），可随时在设置页关闭；
// 设置保存在本地数据目录 convenience.json，不上传任何数据。

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const convenienceSettingsFile = "convenience.json"

type convenienceSettings struct {
	// AutoAccept：匹配到对局（ReadyCheck）时自动点击“接受”。
	AutoAccept bool `json:"autoAccept"`
	// AutoPlayAgain：对局结算（EndOfGame）后自动点击“再来一局”。
	AutoPlayAgain bool `json:"autoPlayAgain"`
	// AutoReconnect：检测到掉线（Reconnect 阶段）时自动重连对局。
	AutoReconnect bool `json:"autoReconnect"`
}

type convenienceRunner struct {
	mu       sync.Mutex
	settings convenienceSettings
	lastRun  map[string]time.Time
	notify   func(event string)
}

func newConvenienceRunner(store *localStore, notify func(event string)) *convenienceRunner {
	return &convenienceRunner{settings: loadConvenienceSettings(store), lastRun: make(map[string]time.Time), notify: notify}
}

func loadConvenienceSettings(store *localStore) convenienceSettings {
	settings := convenienceSettings{}
	if store == nil {
		return settings
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	data, err := os.ReadFile(filepath.Join(store.root, convenienceSettingsFile))
	if err != nil || len(data) > 4<<10 || json.Unmarshal(data, &settings) != nil {
		return convenienceSettings{}
	}
	return settings
}

func saveConvenienceSettings(store *localStore, settings convenienceSettings) error {
	if store == nil {
		return errors.New("本地存储不可用")
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return atomicWriteFile(filepath.Join(store.root, convenienceSettingsFile), data, 0o600)
}

func (r *convenienceRunner) current() convenienceSettings {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.settings
}

func (r *convenienceRunner) apply(settings convenienceSettings) {
	r.mu.Lock()
	r.settings = settings
	r.mu.Unlock()
}

// handlePhase 由 LCU 事件流的 gameflow 阶段变化触发。动作在独立
// goroutine 中执行并做 3 秒去重，避免客户端重复推送同一阶段时连点。
func (r *convenienceRunner) handlePhase(client *LCUClient, phase string) {
	if r == nil || client == nil {
		return
	}
	settings := r.current()
	switch phase {
	case "ReadyCheck":
		if settings.AutoAccept {
			r.run(client, "accept", http.MethodPost, "/lol-matchmaking/v1/ready-check/accept")
		}
	case "EndOfGame":
		if settings.AutoPlayAgain {
			r.run(client, "play-again", http.MethodPost, "/lol-lobby/v2/play-again")
		}
	case "Reconnect":
		if settings.AutoReconnect {
			r.run(client, "reconnect", http.MethodPost, "/lol-gameflow/v1/reconnect")
		}
	}
}

func (r *convenienceRunner) run(client *LCUClient, action, method, path string) {
	r.mu.Lock()
	if last, ok := r.lastRun[action]; ok && time.Since(last) < 3*time.Second {
		r.mu.Unlock()
		return
	}
	r.lastRun[action] = time.Now()
	r.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		// 结算面板刚出现时“再来一局”接口偶尔尚未就绪，稍等片刻更稳。
		if action == "play-again" {
			time.Sleep(1200 * time.Millisecond)
		}
		if err := client.RequestJSON(ctx, method, path, nil, nil); err == nil && r.notify != nil {
			r.notify("convenience:" + action)
		}
	}()
}

func (a *app) handleGameplayConvenience(w http.ResponseWriter, r *http.Request) {
	if a.convenience == nil {
		if r.Method == http.MethodGet {
			respondJSON(w, convenienceSettings{})
			return
		}
		http.Error(w, "便捷设置不可用", http.StatusServiceUnavailable)
		return
	}
	if r.Method == http.MethodGet {
		respondJSON(w, a.convenience.current())
		return
	}
	var request convenienceSettings
	if err := decodeJSONRequest(r, &request, 4<<10); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := saveConvenienceSettings(a.storage, request); err != nil {
		http.Error(w, "便捷设置无法保存", http.StatusServiceUnavailable)
		return
	}
	a.convenience.apply(request)
	respondJSON(w, request)
}
