package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"time"
)

type poolSummary struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Source     string    `json:"source"`
	Version    string    `json:"version"`
	UpdatedAt  time.Time `json:"updatedAt"`
	Hash       string    `json:"hash"`
	EntryCount int       `json:"entryCount"`
	BuiltIn    bool      `json:"builtIn"`
	Selected   bool      `json:"selected"`
}

type diagnosticsResponse struct {
	SchemaVersion      int                     `json:"schemaVersion"`
	Connected          bool                    `json:"connected"`
	SnapshotReady      bool                    `json:"snapshotReady"`
	ConnectionState    string                  `json:"connectionState"`
	EventStream        bool                    `json:"eventStream"`
	Syncing            bool                    `json:"syncing"`
	LastAttempt        time.Time               `json:"lastAttempt,omitempty"`
	LastSuccess        time.Time               `json:"lastSuccess,omitempty"`
	LastDurationMS     int64                   `json:"lastDurationMs"`
	LastError          string                  `json:"lastError,omitempty"`
	LCUSource          string                  `json:"lcuSource,omitempty"`
	Ownership          []OwnershipSourceStatus `json:"ownershipSources"`
	Catalog            CatalogStats            `json:"catalog"`
	PoolID             string                  `json:"poolId"`
	PoolHash           string                  `json:"poolHash"`
	PoolTotal          int                     `json:"poolTotal"`
	PoolMatched        int                     `json:"poolMatched"`
	PoolIssueCount     int                     `json:"poolIssueCount"`
	StorageReady       bool                    `json:"storageReady"`
	DiagnosticLogReady bool                    `json:"diagnosticLogReady"`
	DiagnosticLogError string                  `json:"diagnosticLogError,omitempty"`
	Capabilities       []EndpointCapability    `json:"capabilities"`
	Discovery          LCUDiscoveryStatus      `json:"discovery"`
}

func (a *app) handleDiagnostics(w http.ResponseWriter, _ *http.Request) {
	a.mu.RLock()
	response := diagnosticsResponse{
		SchemaVersion: 4, Connected: a.connected, SnapshotReady: a.snapshotReady, ConnectionState: a.connectionState, EventStream: a.eventStream,
		Syncing: a.syncing, LastAttempt: a.lastAttempt,
		LastSuccess: a.lastSync, LastDurationMS: a.lastDuration.Milliseconds(), LastError: a.lastError,
		Ownership: append([]OwnershipSourceStatus(nil), a.ownership...), Catalog: a.catalog,
		PoolID: a.poolID, PoolHash: a.poolHash, PoolTotal: a.poolTotal, PoolMatched: a.poolMatched,
		PoolIssueCount: len(a.poolIssues), StorageReady: a.storage != nil,
		Capabilities: append([]EndpointCapability(nil), a.account.Capabilities...),
		Discovery:    a.discovery,
	}
	if a.lcu != nil {
		response.LCUSource = a.lcu.source
	}
	a.mu.RUnlock()
	a.diagnosticLogMu.RLock()
	response.DiagnosticLogError = a.diagnosticLogErr
	a.diagnosticLogMu.RUnlock()
	response.DiagnosticLogReady = response.StorageReady && response.DiagnosticLogError == ""
	respondJSON(w, response)
}

func (a *app) handleDiagnosticLog(w http.ResponseWriter, _ *http.Request) {
	if a.storage == nil {
		http.Error(w, "本地诊断日志不可用", http.StatusServiceUnavailable)
		return
	}
	data, err := a.storage.readDiagnosticLog()
	if err != nil {
		http.Error(w, "诊断日志读取失败", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="lol-loot-diagnostics.jsonl"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}

func (a *app) handlePools(w http.ResponseWriter, _ *http.Request) {
	a.mu.RLock()
	pools := make([]poolSummary, 0, len(a.pools))
	for _, pool := range a.pools {
		pools = append(pools, poolSummary{
			ID: pool.ID, Name: pool.Name, Source: pool.Source, Version: pool.Version, UpdatedAt: pool.UpdatedAt,
			Hash: pool.Hash, EntryCount: len(pool.Names), BuiltIn: pool.BuiltIn, Selected: pool.ID == a.poolID,
		})
	}
	a.mu.RUnlock()
	sort.Slice(pools, func(i, j int) bool {
		if pools[i].BuiltIn != pools[j].BuiltIn {
			return pools[i].BuiltIn
		}
		return pools[i].Name < pools[j].Name
	})
	respondJSON(w, map[string]any{"items": pools})
}

func decodeJSONRequest(r *http.Request, target any, limit int64) error {
	if contentType := r.Header.Get("Content-Type"); contentType != "" && !strings.HasPrefix(strings.ToLower(contentType), "application/json") {
		return errors.New("Content-Type 必须为 application/json")
	}
	if r.ContentLength > limit {
		return errors.New("请求内容过大")
	}
	data, err := readLimited(r.Body, limit)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("无效 JSON：%w", err)
	}
	return nil
}

func (a *app) handlePoolImport(w http.ResponseWriter, r *http.Request) {
	if a.storage == nil {
		http.Error(w, "本地存储不可用，无法导入奖池", http.StatusServiceUnavailable)
		return
	}
	var request struct {
		Name    string   `json:"name"`
		Source  string   `json:"source"`
		Version string   `json:"version"`
		Content string   `json:"content"`
		Names   []string `json:"names"`
	}
	if err := decodeJSONRequest(r, &request, 1024*1024); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	names := request.Names
	if len(names) == 0 {
		names = parsePoolNames(request.Content)
	}
	manifest, err := validatePoolManifest(PoolManifest{Name: request.Name, Source: request.Source, Version: request.Version, Names: names})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.storage.savePool(manifest); err != nil {
		http.Error(w, "保存奖池失败："+err.Error(), http.StatusInternalServerError)
		return
	}
	a.mu.Lock()
	a.pools[manifest.ID] = manifest
	a.mu.Unlock()
	a.recordDiagnostic(map[string]any{"event": "pool_imported", "pool_id": manifest.ID, "pool_hash": manifest.Hash, "entries": len(manifest.Names)})
	respondJSON(w, manifest)
}

func (a *app) handlePoolSelect(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ID string `json:"id"`
	}
	if err := decodeJSONRequest(r, &request, 4096); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a.mu.Lock()
	pool, ok := a.pools[request.ID]
	if !ok {
		a.mu.Unlock()
		http.Error(w, "未知奖池", http.StatusNotFound)
		return
	}
	a.poolID, a.poolSource, a.poolVersion, a.poolHash = pool.ID, pool.Source, pool.Version, pool.Hash
	a.poolTotal = len(pool.Names)
	a.poolGeneration++
	a.clearSnapshotLocked("奖池已切换，正在重新核对当前客户端。")
	a.mu.Unlock()
	a.recordDiagnostic(map[string]any{"event": "pool_selected", "pool_id": pool.ID, "pool_hash": pool.Hash, "entries": len(pool.Names)})
	a.requestRefresh()
	w.WriteHeader(http.StatusAccepted)
}

func (a *app) selectedSkins(view string) ([]Skin, PoolManifest, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if !a.connected || !a.snapshotReady {
		return nil, PoolManifest{}, errors.New("当前没有可用的客户端快照")
	}
	pool := a.pools[a.poolID]
	var skins []Skin
	switch view {
	case "owned":
		skins = a.owned
	case "all":
		skins = a.allSkins
	case "remaining", "":
		if !a.calculationOKLocked() {
			return nil, PoolManifest{}, errors.New("奖池尚未完整映射")
		}
		skins = a.remaining
	default:
		return nil, PoolManifest{}, errors.New("未知导出视图")
	}
	return append([]Skin(nil), skins...), pool, nil
}

func (a *app) handleExport(w http.ResponseWriter, r *http.Request) {
	view := r.URL.Query().Get("view")
	format := strings.ToLower(r.URL.Query().Get("format"))
	skins, pool, err := a.selectedSkins(view)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if view == "" {
		view = "remaining"
	}
	filename := fmt.Sprintf("lol-loot-%s-%s", view, time.Now().Format("20060102-150405"))
	payload := map[string]any{"schemaVersion": 1, "exportedAt": time.Now().UTC(), "view": view, "pool": poolSummary{ID: pool.ID, Name: pool.Name, Source: pool.Source, Version: pool.Version, Hash: pool.Hash, EntryCount: len(pool.Names)}, "items": skins}
	writeSkinExport(w, format, filename, "Deep Legends 导出", view, pool, skins, payload, time.Time{})
}

func writeSkinExport(w http.ResponseWriter, format, filename, title, view string, pool PoolManifest, skins []Skin, payload any, capturedAt time.Time) {
	switch format {
	case "json":
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.json"`, filename))
		respondJSON(w, payload)
	case "csv":
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.csv"`, filename))
		_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
		writer := csv.NewWriter(w)
		_ = writer.Write([]string{"skin_id", "skin_name", "champion", "rarity", "owned", "pool_name"})
		for _, skin := range skins {
			_ = writer.Write([]string{fmt.Sprint(skin.ID), safeCSVCell(skin.Name), safeCSVCell(skin.ChampionName), safeCSVCell(skin.Rarity), fmt.Sprint(skin.Owned), safeCSVCell(skin.PoolName)})
		}
		writer.Flush()
	case "html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.html"`, filename))
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'")
		_ = exportHTMLTemplate.Execute(w, map[string]any{"Title": title, "Pool": pool, "View": view, "Items": skins, "CapturedAt": capturedAt, "ExportedAt": time.Now().Format("2006-01-02 15:04:05")})
	default:
		http.Error(w, "format 必须为 json、csv 或 html", http.StatusBadRequest)
	}
}

func safeCSVCell(value string) string {
	value = strings.ReplaceAll(value, "\x00", "")
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed != "" && strings.ContainsRune("=+-@", []rune(trimmed)[0]) {
		return "'" + value
	}
	return value
}

var exportHTMLTemplate = template.Must(template.New("export").Parse(`<!doctype html><html lang="zh-CN"><meta charset="utf-8"><title>{{.Title}}</title><style>body{font:14px/1.5 system-ui,sans-serif;color:#171717;margin:32px}h1{font-size:24px}p{color:#555}table{width:100%;border-collapse:collapse}th,td{padding:8px;border-bottom:1px solid #ddd;text-align:left}th{background:#f5f5f5}</style><h1>{{.Title}}</h1><p>奖池：{{.Pool.Name}} · {{.Pool.Version}}{{if not .CapturedAt.IsZero}} · 快照时间 {{.CapturedAt.Format "2006-01-02 15:04:05"}}{{end}} · 导出时间 {{.ExportedAt}}</p><table><thead><tr><th>ID</th><th>皮肤</th><th>英雄</th><th>品质</th><th>状态</th></tr></thead><tbody>{{range .Items}}<tr><td>{{.ID}}</td><td>{{.Name}}</td><td>{{.ChampionName}}</td><td>{{.RarityTier}}</td><td>{{if .Owned}}已拥有{{else}}三合一剩余{{end}}</td></tr>{{end}}</tbody></table></html>`))

func (a *app) handleSnapshots(w http.ResponseWriter, r *http.Request) {
	if a.storage == nil {
		http.Error(w, "本地历史不可用", http.StatusServiceUnavailable)
		return
	}
	if r.Method == http.MethodGet {
		respondJSON(w, map[string]any{"items": a.storage.listSnapshots()})
		return
	}
	a.mu.RLock()
	if !a.calculationOKLocked() {
		a.mu.RUnlock()
		http.Error(w, "只有完整核对后的结果才能保存快照", http.StatusConflict)
		return
	}
	snapshot := a.snapshotLocked()
	pool := a.pools[a.poolID]
	a.mu.RUnlock()
	record, err := a.storage.saveSnapshot(snapshot, pool)
	if err != nil {
		http.Error(w, "保存快照失败："+err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, snapshotSummary(record))
}

func (a *app) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if a.storage == nil {
		http.Error(w, "本地历史不可用", http.StatusServiceUnavailable)
		return
	}
	record, err := a.storage.loadSnapshot(r.PathValue("id"))
	if err != nil {
		http.Error(w, "快照不存在", http.StatusNotFound)
		return
	}
	respondJSON(w, record)
}

func (a *app) handleSnapshotExport(w http.ResponseWriter, r *http.Request) {
	if a.storage == nil {
		http.Error(w, "本地历史不可用", http.StatusServiceUnavailable)
		return
	}
	record, err := a.storage.loadSnapshot(r.PathValue("id"))
	if err != nil {
		http.Error(w, "快照不存在", http.StatusNotFound)
		return
	}
	skins := make([]Skin, 0, len(record.Owned)+len(record.Remaining))
	appendSkins := func(items []snapshotSkin, owned bool) {
		for _, item := range items {
			skins = append(skins, Skin{ID: item.ID, Name: item.Name, ChampionName: item.ChampionName, Rarity: item.Rarity, Owned: owned, PoolName: item.PoolName})
		}
	}
	appendSkins(record.Owned, true)
	appendSkins(record.Remaining, false)
	pool := PoolManifest{ID: record.PoolID, Name: record.PoolName, Version: record.PoolVersion, Hash: record.PoolHash}
	payload := map[string]any{
		"schemaVersion": 1,
		"exportedAt":    time.Now().UTC(),
		"snapshot":      snapshotSummary(record),
		"owned":         record.Owned,
		"remaining":     record.Remaining,
	}
	filename := fmt.Sprintf("lol-loot-snapshot-%s", record.CapturedAt.Local().Format("20060102-150405"))
	writeSkinExport(w, strings.ToLower(r.URL.Query().Get("format")), filename, "Deep Legends 历史快照", "snapshot", pool, skins, payload, record.CapturedAt.Local())
}

func (a *app) handleSnapshotDiff(w http.ResponseWriter, r *http.Request) {
	if a.storage == nil {
		http.Error(w, "本地历史不可用", http.StatusServiceUnavailable)
		return
	}
	from, err := a.storage.loadSnapshot(r.PathValue("id"))
	if err != nil {
		http.Error(w, "起始快照不存在", http.StatusNotFound)
		return
	}
	to, err := a.storage.loadSnapshot(r.URL.Query().Get("against"))
	if err != nil {
		http.Error(w, "对比快照不存在", http.StatusNotFound)
		return
	}
	diff, err := diffSnapshots(from, to)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	respondJSON(w, diff)
}

func (a *app) handlePrivacy(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, map[string]any{
		"localOnly": true, "requiresPassword": false, "uploadsData": false,
		"reads":           []string{"当前 League Client 中召唤师的公开显示信息、排位与英雄熟练度", "League Client 中的最近战绩、对局参与者与当前游戏流程", "国服跨服搜索时，向本机 RiotClientServices 的 /player-account/aliases/v1/lookup 提交被查询的 Riot ID 并读取 PUUID；它与 League Client 使用独立端口和令牌，结果只在内存中用于所选腾讯 SGP 查询", "League Client 中的好友分组、好友在线状态与所在对局信息（只读，不提供增删改）", "League Client 中的皮肤目录", "当前账号永久拥有的皮肤 ID", "本机战利品与待领取奖励", "皮肤、装备与符文图标资源"},
		"explicitWrites":  []string{"只有在英雄选择阶段点击“应用到客户端”后才新建一页带“[DL] ”前缀的可编辑符文并设为当前页；页数已满时会删除本工具此前创建的、带精确“[DL] ”前缀且未被客户端显式标记为不可删除的最旧符文页来腾位置，每次最多回收 5 页，绝不删除其它符文页，也不更新或覆盖任何已有页", "只有在英雄选择阶段点击“应用装备方案”后才创建或更新客户端装备方案", "只有点击“回放”后才让英雄联盟客户端下载或启动对应回放"},
		"automaticWrites": []string{"默认关闭；仅在设置中开启后，才会在 ReadyCheck 阶段自动接受对局", "默认关闭；仅在设置中开启后，才会在 EndOfGame 阶段自动发起再来一局", "默认关闭；仅在设置中开启后，才会在 Reconnect 阶段自动请求断线重连"},
		"externalReads":   []string{"展示臻彩时按炫彩 ID、皮肤原画本机读取失败时按皮肤 ID，从固定的腾讯官方图片域名读取公开原画；不会发送账号信息、客户端令牌或收藏数据", "“英雄”页统计与图标只向固定 OP.GG、腾讯官方图片与 Riot Data Dragon 公共地址请求，已连接客户端时图标优先直接读取本机客户端", "查询韩服玩家时，向固定的 Riot 官方接口域名发送该玩家的 Riot ID 与内嵌 API Key；只填名称时另向 op.gg 公开搜索发送名称以补全编号", "为生成绝活哥符文推荐，会把从 OP.GG 韩服专家榜取得的第三方玩家 Riot ID 发送给 Riot 官方接口，并读取其公开对局以提取该英雄符文；不携带本机账号、Cookie 或客户端令牌，结果在进程内缓存 6 小时", "打开韩服玩家总览时，向 op.gg 公开页发送该玩家的 Riot ID 与 PUUID，换取每场对局的平均段位（当前登录的国服服务器改为向本机客户端逐人查询，不外发；跨服不查询排位）；失败时该行显示“—”，不影响战绩本身。以上请求都不携带本机账号、Cookie 或客户端令牌"},
		"stores":          []string{"随机脱敏账号标识", "已拥有和三合一剩余的本地历史快照", "按对局 ID 与加盐脱敏账号标识记录的胜点变化", "用户导入的奖池清单", "不含令牌和账号名的诊断事件"},
		"neverStores":     []string{"QQ 账号或密码", "LCU 临时令牌（仅在当前进程内存中短暂使用）", "PUUID、AccountID、SummonerID 与战绩内容", "客户端完整命令行", "战利品与待领取奖励明细"},
	})
}
