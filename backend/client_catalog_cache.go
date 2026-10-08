package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func (client *LCUClient) cachedGameVersion() string {
	if client == nil {
		return ""
	}
	client.mu.RLock()
	defer client.mu.RUnlock()
	return client.gameVersion
}

func (a *app) warmClientCatalogVersion(ctx context.Context, client *LCUClient) {
	previous := client.cachedGameVersion()
	readCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	var version string
	err := client.GetJSONContext(readCtx, "/lol-patch/v1/game-version", &version)
	version = strings.TrimSpace(version)
	if err != nil || version == "" || len(version) > 80 {
		return
	}
	client.mu.Lock()
	if client.gameVersion != previous {
		client.mu.Unlock()
		return
	}
	client.gameVersion = version
	client.mu.Unlock()
	a.noteAssetClientVersion(client, version)
	a.publishClientView()
}

func (a *app) observeClientCatalogVersion(client *LCUClient, event LCUEvent) {
	if !strings.EqualFold(event.URI, "/lol-patch/v1/game-version") {
		return
	}
	var version string
	if json.Unmarshal(event.Data, &version) != nil || strings.TrimSpace(version) == "" || len(version) > 80 {
		return
	}
	client.mu.Lock()
	changed := client.gameVersion != version
	client.gameVersion = version
	client.mu.Unlock()
	if changed {
		a.noteAssetClientVersion(client, version)
		a.publishClientView()
	}
}

func clientCatalogKey(client *LCUClient) (string, bool) {
	if client == nil {
		return "ddragon", true
	}
	version := client.cachedGameVersion()
	client.mu.RLock()
	region, platform := client.region, client.rsoPlatform
	client.mu.RUnlock()
	if version == "" || region == "" {
		return fmt.Sprintf("lcu-unversioned:%p", client), false
	}
	return "lcu-versioned|" + version + "|" + strings.ToUpper(region) + "|" + strings.ToUpper(platform), true
}

// Only validated JSON is cached. A failed LCU read still follows the existing
// fallback path and cannot poison a versioned client entry with Data Dragon data.
func (a *app) clientCatalogBytes(ctx context.Context, client *LCUClient, path string) ([]byte, error) {
	key, known := clientCatalogKey(client)
	if !known {
		return client.GetBytesContext(ctx, path)
	}
	a.perkCatalogMu.Lock()
	if a.clientCatalogCache == nil {
		a.clientCatalogCache = newPublicBinaryCache(a.storage, "client-catalog", 24, 16<<20)
		a.clientCatalogCache.memoryMaxEntries, a.clientCatalogCache.memoryMaxBytes = 24, 16<<20
	}
	cache := a.clientCatalogCache
	a.perkCatalogMu.Unlock()
	return cache.load(ctx, "client-catalog-v1|"+key+"|"+path, 365*24*time.Hour, 0, true, func(ctx context.Context) ([]byte, error) {
		raw, err := client.GetBytesContext(ctx, path)
		if err == nil {
			var rows []json.RawMessage
			decodeErr := json.Unmarshal(raw, &rows)
			if strings.HasSuffix(path, "/perkstyles.json") && decodeErr != nil {
				var wrapped struct {
					Styles []json.RawMessage `json:"styles"`
				}
				decodeErr = json.Unmarshal(raw, &wrapped)
				rows = wrapped.Styles
			}
			if decodeErr != nil || len(rows) == 0 {
				return nil, fmt.Errorf("客户端目录 JSON 无效或为空")
			}
		}
		return raw, err
	})
}

func (a *app) clientCatalogJSON(ctx context.Context, client *LCUClient, path string, target any) error {
	raw, err := a.clientCatalogBytes(ctx, client, path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

func (a *app) noteAssetClientVersion(client *LCUClient, version string) {
	a.mu.Lock()
	if a.lcu != client || !a.connected {
		a.mu.Unlock()
		return
	}
	previous := a.assetClientVersion
	a.assetClientVersion = version
	a.mu.Unlock()
	if previous != "" && previous != version {
		a.clearAssetCache()
	}
}
