package main

import (
	"context"
	"strconv"
	"strings"
)

// Official API reference /api-details/summoner-v4 and match-v5 (2026-10-05).
// Account-V1 has only three clusters: SEA accounts use ASIA, while their
// Match-V5 histories use SEA. Pro directories and specialist leaderboards
// intentionally continue to select KR explicitly.
type riotPlatformRoute struct{ Cluster, Label string }

var riotPlatforms = map[string]riotPlatformRoute{
	"br1": {"americas", "巴西"}, "eun1": {"europe", "欧北东"}, "euw1": {"europe", "欧西"},
	"jp1": {"asia", "日服"}, "kr": {"asia", "韩服"}, "la1": {"americas", "拉北"}, "la2": {"americas", "拉南"},
	"me1": {"europe", "中东"}, "na1": {"americas", "美服"}, "oc1": {"sea", "大洋洲"}, "ru": {"europe", "俄服"},
	"sg2": {"sea", "东南亚"}, "tr1": {"europe", "土耳其"}, "tw2": {"sea", "台服"}, "vn2": {"sea", "越南"},
}

func riotPlatform(region string) string {
	region = strings.ToLower(strings.TrimSpace(region))
	if _, ok := riotPlatforms[region]; ok {
		return region
	}
	return ""
}
func isRiotRegion(region string) bool      { return riotPlatform(region) != "" }
func riotRegionLabel(region string) string { return riotPlatforms[riotPlatform(region)].Label }
func riotHostRoute(host string) string {
	const suffix = ".api.riotgames.com"
	if !strings.HasSuffix(host, suffix) {
		return ""
	}
	route := strings.TrimSuffix(host, suffix)
	switch route {
	case "americas", "asia", "europe", "sea":
		return route
	}
	if route == riotPlatform(route) {
		return route
	}
	return ""
}
func (p *riotProvider) region() string {
	if region := riotPlatform(p.platform); region != "" {
		return region
	}
	// The root provider serves the explicitly Korean pro/specialist directory.
	return riotRegionKR
}
func (p *riotProvider) platformHost() string { return p.region() + ".api.riotgames.com" }
func (p *riotProvider) clusterHost() string {
	return riotPlatforms[p.region()].Cluster + ".api.riotgames.com"
}
func (p *riotProvider) accountHost() string {
	cluster := riotPlatforms[p.region()].Cluster
	if cluster == "sea" {
		cluster = "asia"
	}
	return cluster + ".api.riotgames.com"
}

// Separate providers isolate memory identity caches, flights and conservative
// Personal-Key rate windows. Disk identity keys additionally carry platform.
func (p *riotProvider) forPlatform(region string) *riotProvider {
	region = riotPlatform(region)
	if region == "" || region == p.region() {
		return p
	}
	p.platformMu.Lock()
	defer p.platformMu.Unlock()
	if p.platforms == nil {
		p.platforms = make(map[string]*riotProvider)
	}
	if scoped := p.platforms[region]; scoped != nil {
		return scoped
	}
	scoped := newRiotProvider(p.champions)
	scoped.platform, scoped.matchDisk, scoped.identityDisk = region, p.matchDisk, p.identityDisk
	scoped.limitNow, scoped.limitSleep = p.limitNow, p.limitSleep
	p.platforms[region] = scoped
	return scoped
}
func clientRiotPlatform(client *LCUClient) string {
	if client == nil {
		return ""
	}
	region, platform := client.platformInfo()
	if strings.EqualFold(region, "TENCENT") {
		return ""
	}
	if value := riotPlatform(platform); value != "" {
		return value
	}
	return riotPlatform(region)
}
func clientRegionInfo(client *LCUClient) (string, string) {
	if client == nil {
		return "", ""
	}
	region, _ := client.platformInfo()
	if strings.EqualFold(region, "TENCENT") {
		return "TENCENT", "国服"
	}
	platform := clientRiotPlatform(client)
	return platform, riotRegionLabel(platform)
}
func riotMatchID(region string, gameID int64) string {
	return strings.ToUpper(riotPlatform(region)) + "_" + strconv.FormatInt(gameID, 10)
}

func clientPlatformDiagnostic(client *LCUClient) map[string]any {
	region, platform := client.platformInfo()
	if !isRiotRegion(platform) && !strings.EqualFold(region, "TENCENT") {
		region, platform = "", ""
	}
	if strings.EqualFold(region, "TENCENT") {
		if _, known := normalizeTencentServerID(platform); !known {
			platform = ""
		}
	} else if platform != "" {
		// Region is an enum too; do not retain arbitrary command-line values.
		allowed := map[string]bool{"JP": true, "KR": true, "NA": true, "EUW": true, "EUNE": true, "BR": true, "LAN": true, "LAS": true, "OCE": true, "RU": true, "TR": true, "SG": true, "TW": true, "VN": true, "ME": true, "SEA": true}
		if !allowed[region] && !isRiotRegion(region) {
			region = ""
		}
	}
	client.mu.RLock()
	source := client.platformSource
	client.mu.RUnlock()
	if source != "startup-args" && source != "command-line-query" {
		source = "unknown"
	}
	return map[string]any{"event": "client_platform_resolved", "region": region, "platform": platform, "source": source}
}

type riotPlatformContextKey struct{}

func withRiotPlatform(ctx context.Context, region string) context.Context {
	return context.WithValue(ctx, riotPlatformContextKey{}, riotPlatform(region))
}
func riotContextPlatform(ctx context.Context) string {
	if region, _ := ctx.Value(riotPlatformContextKey{}).(string); isRiotRegion(region) {
		return region
	}
	return riotRegionKR
}
func opggPlatform(region string) string {
	if region == "" {
		region = riotRegionKR
	} // Legacy explicitly Korean directory callers.
	return map[string]string{"kr": "kr", "jp1": "jp", "na1": "na", "euw1": "euw", "eun1": "eune", "br1": "br", "la1": "lan", "la2": "las", "oc1": "oce", "ru": "ru", "tr1": "tr", "sg2": "sg", "tw2": "tw", "vn2": "vn", "me1": "me"}[riotPlatform(region)]
}
func opggRegionCacheIdentity(region, identity string) string {
	if region == "" || region == riotRegionKR {
		return identity
	}
	return riotPlatform(region) + ":" + identity
}
