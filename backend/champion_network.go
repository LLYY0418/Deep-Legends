package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const championNetworkSettingsFile = "champion-network.json"

type championNetworkSettings struct {
	Mode string `json:"mode"`
	URL  string `json:"url,omitempty"`
}

type championNetworkStatus struct {
	championNetworkSettings
	Active string `json:"active"`
}

func defaultChampionNetworkSettings() championNetworkSettings {
	return championNetworkSettings{Mode: "auto"}
}

func validateChampionNetworkSettings(settings championNetworkSettings) (championNetworkSettings, error) {
	settings.Mode = strings.ToLower(strings.TrimSpace(settings.Mode))
	settings.URL = strings.TrimSpace(settings.URL)
	switch settings.Mode {
	case "auto", "direct":
		settings.URL = ""
		return settings, nil
	case "manual":
		parsed, err := url.Parse(settings.URL)
		if err != nil || parsed.Hostname() == "" || parsed.User != nil {
			return championNetworkSettings{}, errors.New("代理地址无效")
		}
		switch strings.ToLower(parsed.Scheme) {
		case "http", "https", "socks5", "socks5h":
		default:
			return championNetworkSettings{}, errors.New("代理仅支持 http、https 或 socks5")
		}
		if parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
			return championNetworkSettings{}, errors.New("代理地址不能包含路径、查询参数或账号密码")
		}
		return settings, nil
	default:
		return championNetworkSettings{}, errors.New("代理模式无效")
	}
}

func loadChampionNetworkSettings(store *localStore) championNetworkSettings {
	settings := defaultChampionNetworkSettings()
	if store == nil {
		return settings
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	data, err := os.ReadFile(filepath.Join(store.root, championNetworkSettingsFile))
	if err != nil || len(data) > 16<<10 || json.Unmarshal(data, &settings) != nil {
		return defaultChampionNetworkSettings()
	}
	validated, err := validateChampionNetworkSettings(settings)
	if err != nil {
		return defaultChampionNetworkSettings()
	}
	return validated
}

func saveChampionNetworkSettings(store *localStore, settings championNetworkSettings) error {
	if store == nil {
		return errors.New("本地存储不可用")
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return atomicWriteFile(filepath.Join(store.root, championNetworkSettingsFile), data, 0o600)
}

func getenvSystemProxy() string { return os.Getenv("DEEP_LEGENDS_SYSTEM_PROXY") }

func championProxyFor(settings championNetworkSettings) (func(*http.Request) (*url.URL, error), string, error) {
	settings, err := validateChampionNetworkSettings(settings)
	if err != nil {
		return nil, "", err
	}
	if settings.Mode == "direct" {
		return nil, "直连", nil
	}
	proxyAddress := settings.URL
	if settings.Mode == "auto" {
		proxyAddress = strings.TrimSpace(os.Getenv("DEEP_LEGENDS_SYSTEM_PROXY"))
		if proxyAddress == "" {
			return nil, "直连", nil
		}
	}
	parsed, err := url.Parse(proxyAddress)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil {
		return nil, "", errors.New("代理地址无效")
	}
	return http.ProxyURL(parsed), parsed.Scheme + "://" + parsed.Host, nil
}

// handleSystemProxy 接收桌面壳在后端启动后异步下发的系统代理地址。
// 桌面壳为了不阻塞启动流程，先拉起本服务再解析系统代理；
// championProxyFor 的 auto 模式每次请求都会重新读取该环境变量，因此晚到也能生效。
func (a *app) handleSystemProxy(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Proxy string `json:"proxy"`
	}
	if err := decodeJSONRequest(r, &request, 4<<10); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	proxy := strings.TrimSpace(request.Proxy)
	if a.licenseSideEffect(r.Context()) != nil {
		http.Error(w, "软件授权已失效", http.StatusForbidden)
		return
	}
	if proxy == "" {
		_ = os.Unsetenv("DEEP_LEGENDS_SYSTEM_PROXY")
		a.refreshAutomaticNetworkProxy("")
		respondJSON(w, map[string]any{"ok": true, "proxy": ""})
		return
	}
	if _, err := validateChampionNetworkSettings(championNetworkSettings{Mode: "manual", URL: proxy}); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = os.Setenv("DEEP_LEGENDS_SYSTEM_PROXY", proxy)
	a.refreshAutomaticNetworkProxy(proxy)
	respondJSON(w, map[string]any{"ok": true, "proxy": proxy})
}

func (a *app) handleChampionNetwork(w http.ResponseWriter, r *http.Request) {
	provider := a.championDataProvider()
	if r.Method == http.MethodGet {
		respondJSON(w, provider.networkStatus())
		return
	}
	var request championNetworkSettings
	if err := decodeJSONRequest(r, &request, 16<<10); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	settings, err := validateChampionNetworkSettings(request)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if a.licenseSideEffect(r.Context()) != nil {
		http.Error(w, "软件授权已失效", http.StatusForbidden)
		return
	}
	if err := saveChampionNetworkSettings(a.storage, settings); err != nil {
		http.Error(w, "代理设置无法保存", http.StatusServiceUnavailable)
		return
	}
	if err := provider.setNetworkSettings(settings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	respondJSON(w, provider.networkStatus())
}

func (a *app) refreshAutomaticNetworkProxy(raw string) {
	p := a.champions
	if p == nil {
		return
	}
	p.clientMu.RLock()
	client := p.client
	p.clientMu.RUnlock()
	if client != nil {
		if t, ok := client.Transport.(*automaticNetworkTransport); ok && t.settings.Mode == "auto" {
			t.refreshProxy(raw)
			for group := range networkProbeURLs {
				t.probeGroup(group)
			}
		}
	}
}
func (a *app) recoverExternalNetwork() {
	riotRelays.mu.Lock()
	riotRelays.nextTry = time.Time{}
	riotRelays.failures = 0
	riotRelays.networkFailures = 0
	manager := riotRelays.entryManager
	riotRelays.mu.Unlock()
	if manager != nil {
		manager.mu.Lock()
		for _, entry := range manager.runtime {
			entry.until = time.Time{}
			entry.consecutive = 0
			entry.backoffs = 0
		}
		manager.mu.Unlock()
	}
	riotUserKeys.mu.Lock()
	if riotUserKeys.networkFailures > 0 {
		riotUserKeys.networkFailures = 0
		riotUserKeys.routeUntil = time.Time{}
	}
	riotUserKeys.mu.Unlock()
	clearRiotAccountNetworkFailures(a.riot, map[*riotProvider]bool{})
	if p := a.champions; p != nil && p.hexdata != nil {
		h := p.hexdata
		h.mu.Lock()
		h.state.Circuits = map[string]hexdataCircuitState{}
		h.saveStateLocked()
		h.mu.Unlock()
	}
	if c := a.mayhemRatings; c != nil {
		c.mu.Lock()
		for key, item := range c.cache {
			if !item.outcome.response.Available && item.outcome.response.UnavailableReason != "未收录" {
				delete(c.cache, key)
			}
		}
		c.mu.Unlock()
	}
	a.recordDiagnostic(map[string]any{"event": "network_proxy_recovered"})
	a.broadcastEvent(`{"type":"riot-relay-recovered"}`)
}

func clearRiotAccountNetworkFailures(p *riotProvider, seen map[*riotProvider]bool) {
	if p == nil || seen[p] {
		return
	}
	seen[p] = true
	p.accountMu.Lock()
	for key, item := range p.accountCache {
		if item.err != nil {
			delete(p.accountCache, key)
		}
	}
	p.accountMu.Unlock()
	p.platformMu.Lock()
	children := []*riotProvider{p.relayProvider}
	for _, child := range p.platforms {
		children = append(children, child)
	}
	p.platformMu.Unlock()
	for _, child := range children {
		clearRiotAccountNetworkFailures(child, seen)
	}
}
