package main

import (
	"context"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// R318 P8：海克斯元数据统一目录。
//
// 后端把所有来源按编号合并成一份元数据：名称、描述、稀有度、图标各自取最佳
// 来源，而不是「某个来源有这条记录就整条覆盖」。图标返回有序候选链：
// 本机 LCU → CommunityDragon 当前版本 → Hexdata（当前版本已无可用的海克斯
// 元数据来源，保留为空位）→ OP.GG → 内置快照。
// 前端按顺序逐个尝试直到加载成功，全部失败才显示统一占位图形。

type gameplayAugmentIcon struct {
	Source string `json:"source"`
	Path   string `json:"path,omitempty"`
	URL    string `json:"url"`
}

const (
	augmentCatalogRecheckAfter = 30 * time.Minute
	augmentUnknownRetryAfter   = 30 * time.Minute
)

// augmentIconURL 把不同来源的路径翻译成前端可以直接加载的同源地址。
func augmentIconURL(source, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "builtin:") {
		return strings.TrimPrefix(path, "builtin:")
	}
	if strings.HasPrefix(path, "opgg:") {
		source, path = "opgg", strings.TrimPrefix(path, "opgg:")
	}
	if strings.HasPrefix(path, "ddragon:") {
		source, path = "ddragon", strings.TrimPrefix(path, "ddragon:")
	}
	lower := strings.ToLower(path)
	if strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") {
		if parsed, err := url.Parse(path); err == nil && strings.EqualFold(parsed.Hostname(), opggAssetHost) {
			source, path = "opgg", parsed.EscapedPath()
		} else {
			return ""
		}
	}
	switch source {
	case "opgg", "communitydragon", "ddragon", "aramkit":
		if source == "communitydragon" && strings.HasPrefix(path, "/lol-game-data/assets/") {
			if converted := communityDragonGameAssetPath(path); converted != "" {
				path = converted
			}
		}
		if path == "" || path[0] != '/' {
			return ""
		}
		return "/api/champion-asset?source=" + url.QueryEscape(source) + "&path=" + url.QueryEscape(path)
	case "builtin":
		if id, ok := bundledAugmentID(path); ok {
			return bundledAugmentPath(id)
		}
		return ""
	default:
		// 本机客户端资源走 /api/image（与其它 LCU 图标一致）。
		if strings.HasPrefix(path, "/lol-game-data/") {
			return "/api/image?path=" + url.QueryEscape(path)
		}
		return ""
	}
}

func appendAugmentIcon(list []gameplayAugmentIcon, source, path string) []gameplayAugmentIcon {
	candidateURL := augmentIconURL(source, path)
	if candidateURL == "" {
		return list
	}
	for _, existing := range list {
		if existing.URL == candidateURL {
			return list
		}
	}
	return append(list, gameplayAugmentIcon{Source: source, Path: path, URL: candidateURL})
}

func (p *championProvider) setOnlineAugments(rows []gameplayAugment) {
	p.remoteAugmentMu.Lock()
	p.onlineAugments = append([]gameplayAugment(nil), rows...)
	p.remoteAugmentMu.Unlock()
}

func (p *championProvider) onlineAugmentRows() []gameplayAugment {
	p.remoteAugmentMu.RLock()
	defer p.remoteAugmentMu.RUnlock()
	return append([]gameplayAugment(nil), p.onlineAugments...)
}

func (p *championProvider) remoteAugmentRows() []gameplayAugment {
	p.remoteAugmentMu.RLock()
	defer p.remoteAugmentMu.RUnlock()
	return append([]gameplayAugment(nil), p.remoteAugments...)
}

func augmentRowsFromCatalog(rows []championAugment) []gameplayAugment {
	result := make([]gameplayAugment, 0, len(rows))
	for _, row := range rows {
		if row.ID <= 0 || strings.TrimSpace(row.Name) == "" {
			continue
		}
		path := strings.TrimSpace(row.ImagePath)
		source := strings.TrimSpace(row.ImageSource)
		if source != "" && path != "" {
			path = source + ":" + path
		}
		result = append(result, gameplayAugment{ID: int64(row.ID), Name: row.Name, Description: row.Description, Rarity: row.Rarity, IconPath: path})
	}
	return result
}

// unifiedAugmentCatalog 合并所有本地/缓存来源。extra 通常是本机 LCU 目录。
func (p *championProvider) unifiedAugmentCatalog(extra []gameplayAugment) []gameplayAugment {
	lcu := extra
	cdragon := p.remoteAugmentRows()
	mayhem := currentMayhemMetadataRows()
	opgg := p.onlineAugmentRows()
	bundled := bundledAugmentCatalog()

	index := map[int64]*gameplayAugment{}
	order := []int64{}
	upsert := func(id int64) *gameplayAugment {
		if row, ok := index[id]; ok {
			return row
		}
		row := &gameplayAugment{ID: id}
		index[id] = row
		order = append(order, id)
		return row
	}

	// 名称优先级：本机 LCU → 当前版本快照 → CommunityDragon → OP.GG → 内置快照。
	for _, source := range [][]gameplayAugment{lcu, mayhem, cdragon, opgg, bundled} {
		for _, item := range source {
			if item.ID <= 0 {
				continue
			}
			row := upsert(item.ID)
			if strings.TrimSpace(row.Name) == "" && strings.TrimSpace(item.Name) != "" {
				row.Name = item.Name
			}
		}
	}
	// 描述优先级：CommunityDragon → 当前版本快照 → OP.GG → 本机 LCU → 内置快照。
	for _, source := range [][]gameplayAugment{cdragon, mayhem, opgg, lcu, bundled} {
		for _, item := range source {
			if row, ok := index[item.ID]; ok && strings.TrimSpace(row.Description) == "" && strings.TrimSpace(item.Description) != "" {
				row.Description = item.Description
			}
		}
	}
	// 稀有度优先级：本机 LCU → 当前版本快照 → CommunityDragon → 内置快照 → OP.GG。
	for _, source := range [][]gameplayAugment{lcu, mayhem, cdragon, bundled, opgg} {
		for _, item := range source {
			if row, ok := index[item.ID]; ok && strings.TrimSpace(row.Rarity) == "" && strings.TrimSpace(item.Rarity) != "" {
				row.Rarity = normalizeAugmentRarity(item.Rarity)
			}
		}
	}
	// 图标候选链：本机 LCU → CommunityDragon → (Hexdata 空位) → OP.GG → 内置快照。
	for _, source := range []struct {
		name string
		rows []gameplayAugment
	}{{"lcu", lcu}, {"communitydragon", cdragon}, {"opgg", opgg}, {"bundled", bundled}} {
		for _, item := range source.rows {
			row, ok := index[item.ID]
			if !ok {
				continue
			}
			path := item.IconPath
			// 兼容既有消费方：保留首个来源的 iconPath，新增字段只是候选链。
			if strings.TrimSpace(row.IconPath) == "" && strings.TrimSpace(path) != "" {
				row.IconPath = path
			}
			if strings.HasPrefix(path, "opgg:") {
				path = strings.TrimPrefix(path, "opgg:")
				row.IconCandidates = appendAugmentIcon(row.IconCandidates, "opgg", path)
			} else if strings.HasPrefix(path, "builtin:") || source.name == "bundled" {
				row.IconCandidates = appendAugmentIcon(row.IconCandidates, "builtin", strings.TrimPrefix(path, "builtin:"))
			} else if strings.HasPrefix(path, "/lol-game-data/") && source.name == "lcu" {
				row.IconCandidates = appendAugmentIcon(row.IconCandidates, "lcu", path)
			} else if strings.HasPrefix(path, "/lol-game-data/") {
				// CommunityDragon 提供的仍是客户端资源路径，翻译成公共 CDN 地址。
				row.IconCandidates = appendAugmentIcon(row.IconCandidates, "communitydragon", communityDragonGameAssetPath(path))
			} else if path != "" {
				row.IconCandidates = appendAugmentIcon(row.IconCandidates, source.name, path)
			}
			if strings.TrimSpace(row.FallbackIconPath) == "" && strings.TrimSpace(item.FallbackIconPath) != "" {
				row.FallbackIconPath = item.FallbackIconPath
			}
			if strings.TrimSpace(item.FallbackIconPath) != "" && strings.HasPrefix(item.FallbackIconPath, "/lol-game-data/") {
				row.IconCandidates = appendAugmentIcon(row.IconCandidates, "communitydragon", communityDragonGameAssetPath(item.FallbackIconPath))
			}
		}
	}

	result := make([]gameplayAugment, 0, len(order))
	for _, id := range order {
		row := index[id]
		// 没有名称的行无法展示，保留编号交给前端占位；统一目录仍会返回它，
		// 便于诊断「哪些编号各来源都没有」。
		if strings.TrimSpace(row.Name) == "" {
			row.Name = "海克斯 " + strconv.FormatInt(id, 10)
			row.Unknown = true
		}
		result = append(result, *row)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (p *championProvider) augmentCatalogVersion(rows []gameplayAugment) string {
	patch := ""
	if snapshot := readCurrentMayhem(); snapshot.Patch != "" {
		patch = snapshot.Patch
	}
	return patch + "/" + strconv.Itoa(len(rows))
}

// warmAugmentCatalog 启动后在后台补齐在线目录（不阻塞首屏、不依赖英雄页）。
func (a *app) warmAugmentCatalog(ctx context.Context) {
	provider := a.championDataProvider()
	if provider == nil {
		return
	}
	warmCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if result, err := provider.loadAugments(warmCtx); err == nil {
		provider.setOnlineAugments(augmentRowsFromCatalog(result.Rows))
	}
	if remote, err := provider.loadCommunityDragonAugments(warmCtx); err == nil && len(remote) > 0 {
		provider.rememberCommunityDragonAugments(remote)
	}
	rows := provider.unifiedAugmentCatalog(nil)
	unknown := 0
	for _, row := range rows {
		if row.Unknown {
			unknown++
		}
	}
	a.recordDiagnostic(map[string]any{"event": "augment_catalog_state", "version": provider.augmentCatalogVersion(rows), "rows": len(rows), "online": len(provider.onlineAugmentRows()), "communitydragon": len(provider.remoteAugmentRows()), "bundled": len(bundledAugmentCatalog()), "unknown": unknown})
}

// resolveAugmentIDs 为新编号做一次按需补全；查不到的编号短期缓存，之后可再查。
func (a *app) resolveAugmentIDs(ctx context.Context, provider *championProvider, ids []int64) {
	if provider == nil || len(ids) == 0 {
		return
	}
	now := time.Now()
	a.augmentResolveMu.Lock()
	if a.augmentResolvedAt == nil {
		a.augmentResolvedAt = map[int64]time.Time{}
	}
	retry := false
	for _, id := range ids {
		if at, ok := a.augmentResolvedAt[id]; ok && now.Sub(at) < augmentUnknownRetryAfter {
			continue
		}
		retry = true
		a.augmentResolvedAt[id] = now
	}
	a.augmentResolveMu.Unlock()
	if !retry {
		return
	}
	a.augmentResolveMu.Lock()
	last := a.augmentOnlineProbeAt
	if !last.IsZero() && now.Sub(last) < time.Minute {
		a.augmentResolveMu.Unlock()
		return
	}
	a.augmentOnlineProbeAt = now
	a.augmentResolveMu.Unlock()
	probeCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	if result, err := provider.loadAugments(probeCtx); err == nil {
		provider.setOnlineAugments(augmentRowsFromCatalog(result.Rows))
	}
	if remote, err := provider.loadCommunityDragonAugments(probeCtx); err == nil && len(remote) > 0 {
		provider.rememberCommunityDragonAugments(remote)
	}
}
