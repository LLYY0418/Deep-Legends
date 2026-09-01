# 工单：国服排位胜率缺失 / 每局 LP 变化未展示

来源：2026-08-20 真机反馈 + `diagnostics-0d59fad4.jsonl`、`diagnostics.1.jsonl`。

两个问题性质完全不同：

- **A（胜率）**：链路没坏，坏的是代码注释里的前提假设。**根因未定，先加埋点，不要直接改抓取逻辑。**
- **B（LP 变化）**：不是 bug，是机制天然限制。需要补的是护栏、埋点和文案，不是新数据源。
- **C（附带发现）**：RANKED 路由 23% 的请求连接都没建立。

本文件只描述改动，**未改动任何仓库代码**。

---

## 一、工单 A：国服搜索玩家胜率显示「负场未提供」

### A0. 现状与证据

截图里那个玩家（单排 45 胜 / 灵活 139 胜）在日志里能精确对上：

```
05:28:48.0330053  {"event":"ranked_winrate_resolved","source":"sgp","queue":"RANKED_FLEX_SR",
                   "wins":139,"losses":0,"suppressed":true}
05:28:48.0330053  {"event":"ranked_winrate_resolved","source":"sgp","queue":"RANKED_SOLO_5x5",
                   "wins":45,"losses":0,"suppressed":true}
```

对应请求 `/leagues-ledge/v2/rankedStats/puuid/{puuid}`，`http_status:200`，`body_bytes:3346`。

链路每一环都在按设计执行：SGP 返回 wins、losses 为 0 → `verifiedRankWinRate`（`gameplay.go:1469-1479`）判定记录不完整、返回 `-1, false` → `loadRanksWithFallback` 置 `capability.Detail`（`gameplay.go:1405`）→ 前端 `web/gameplay.js:765-766` 落到「负场未提供 / 胜率暂不可用」。

**不要去改这几处。它们没错。**

### A1. 被推翻的前提

`gameplay.go:1360-1362` 与 `sgp_api.go:509-510` 两处注释都写着：SGP 的 leagues-ledge「仍然提供完整的当季胜负场次」。真机数据不支持这个说法。

两份日志共 2460 条非空排位记录：

| | 条数 |
| --- | --- |
| 带负场（完整） | 42 |
| `wins > 0 && losses == 0`（被抑制） | 2418 |

带负场的 42 条涉及 **10 组不同的 (wins, losses)**：

```
37/30 ×14   131/138 ×14   ← 当前登录玩家
310/224 ×2  115/4 ×2  234/178 ×2  223/201 ×2  410/372 ×2  108/101 ×2
97/90 ×1    472/429 ×1
```

472/429、410/372、310/224 显然不是当前登录玩家。**同一个接口、同一个 HN10、同一个 league-session token，少数玩家返回负场，绝大多数不返回。**时间上也交错：05:28:48.03 那个玩家被抑制，3.6 秒后 05:28:51.64 另一个玩家两个队列都完整。

所以以下两条结论都是错的，别按它们改：

- ~~「SGP 对非本人一律不返回负场」~~ —— 有 42 条反例。
- ~~「代码把 losses 解析漏了」~~ —— 同一个结构体、同一段代码，有时能解析出来。

### A2. 目前最硬的线索：响应体大小

同一批次（05:28:47.95 – 05:28:48.12，10 个请求串行）：

| 类型 | body_bytes |
| --- | --- |
| 完整（37/30 + 131/138） | 3548 |
| 被抑制（45/139、2/108、91/7、58/0） | 3253 / 3332 / 3335 / 3346 |
| 完全空档（0 胜 0 负，未定级） | 2755 / 2826 / 2829 |

完整响应比同批次被抑制的大 **200–290 字节**。两个队列的 `"losses":NNN` 加起来撑死 30 字节。

**多出来的两百多字节是结构，不是数字。**被抑制的响应里少的不止 `losses` 一个键，而是一整块字段。

### A3. 两个待区分的假设

| | 假设 | 支持 | 反对 |
| --- | --- | --- | --- |
| H1 | 接口对非本人返回精简 payload | 解释 200 字节结构差 | 解释不了那 42 条完整记录 |
| H2 | 玩家侧隐私 / 生涯可见性设置决定返回完整还是精简 | 同时解释「少数完整」和「结构性缺失」 | 未验证 |

倾向 H2。仓库里其实已经有隐私信号 —— `sgpSummoner.Privacy`（`sgp_api.go:553`）和 `gameplayPlayer.PrivateHistory`（`gameplay.go:692`）——**但 rankedStats 这条链路从来没读过它，也没记进任何埋点。**

### A4. 【本轮要做的】只加埋点，不改抓取逻辑

现在 RANKED 路由的 `sgp_request` 只记 `body_bytes`（`sgp_api.go:340-343`），拿不到任何字段信息。补一条**只记键名、不记值**的埋点，一次真机运行即可定死 H1 还是 H2。

**A4-1. 新增键名提取工具**（放在 `sgp_api.go:280` 的 `diagnosticPayloadPrefixShape` 旁边，风格保持一致）

```go
// diagnosticKeySet 返回 JSON 对象的键名列表。只记键名，不记任何值 ——
// 排位响应里含远端玩家的段位与场次，值一律不落盘。
func diagnosticKeySet(raw json.RawMessage) []string {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return nil
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
```

**A4-2. 改 `rankedStatsOn`**（`sgp_api.go:531-546`）先取原始字节再解析

`getJSONWithToken` 内部是 `json.Unmarshal(body, out)`，所以传 `*json.RawMessage` 就能拿到整个响应体，不需要改它的签名：

```go
func (p *sgpProvider) rankedStatsOn(ctx context.Context, client *LCUClient, serverID, puuid string, isSelf bool) ([]sgpRankedQueue, error) {
	serverID = strings.ToUpper(strings.TrimSpace(serverID))
	base, ok := p.serverBase(serverID)
	if !ok {
		return nil, fmt.Errorf("未收录的国服子服务器：%s", serverID)
	}
	endpoint := base + "/leagues-ledge/v2/rankedStats/puuid/" + url.PathEscape(puuid)
	var raw json.RawMessage
	if err := p.getJSONWithToken(ctx, client, sgpTokenLeagueSession, serverID, "RANKED", "/leagues-ledge/v2/rankedStats/puuid/{puuid}", endpoint, &raw); err != nil {
		return nil, err
	}
	var payload struct {
		Queues []json.RawMessage `json:"queues"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	queueKeys := []string(nil)
	if len(payload.Queues) > 0 {
		queueKeys = diagnosticKeySet(payload.Queues[0])
	}
	p.recordObservation(map[string]any{
		"event": "sgp_ranked_stats_shape", "server_id": serverID, "is_self": isSelf,
		"body_bytes": len(raw), "queue_count": len(payload.Queues),
		"top_level_keys": diagnosticKeySet(raw), "queue_keys": queueKeys,
	})
	queues := make([]sgpRankedQueue, 0, len(payload.Queues))
	for _, entry := range payload.Queues {
		var queue sgpRankedQueue
		if json.Unmarshal(entry, &queue) == nil {
			queues = append(queues, queue)
		}
	}
	return queues, nil
}
```

**A4-3. 传入 `isSelf`**

- `loadRanksWithFallback`（`gameplay.go:1377`）已经有 `isCurrent` 参数，直接透传。
- `rankedStats`（`sgp_api.go:522-528`）是自查封装，传 `true`。
- **`sgp_api_test.go:273` 调用的是 4 参数版本，需要同步补一个实参。**这是本工单唯一一处必须改动的测试。

**A4-4. 顺带把 privacy 带上**

`summonerByPUUIDOn`（`sgp_api.go:557`）已经拿到了 `sgpSummoner.Privacy`，而 `gameplay.go:692` 也已经在用它算 `PrivateHistory`。在 `loadRanksWithFallback` 里把当前玩家的 privacy 值一并记进 `sgp_ranked_stats_shape`（或另发一条能按时间对上的事件）。`privacy` 是枚举字符串（PUBLIC / PRIVATE），不是隐私内容本身，可以落盘。

> **隐私红线**：这条埋点**只允许记键名、布尔、计数和 privacy 枚举**。禁止记 puuid、昵称、段位、场次数值，禁止记 payload 片段。现有 `payload_sample_bytes`（`sgp_api.go:355`）记的是长度不是内容，保持这个尺度。

### A5. 【拿到日志后再做，本轮不要动】

跑一次真机、搜 3–5 个不同玩家（最好包含一个已知「生涯公开」和一个「生涯隐藏」的），然后按 `sgp_ranked_stats_shape` 的 `queue_keys` 分组：

- **若被抑制的响应里 `queue_keys` 不含 `losses`** → 结构性缺失坐实。再看 `is_self` 与 privacy 是否能完全区分两组：
  - 能区分 → H2 成立，走 A6 文案方案，国服他人赛季负场判定为**不可得**，不再尝试其它接口。
  - 不能区分 → H1 成立，走 A7 降级方案。
- **若被抑制的响应里 `queue_keys` 含 `losses`** → 那就是服务端确实回了 0，结论同 H1，走 A7。

### A6. 若确认是隐私门控（H2）

文案要有信息量，不能让用户以为是 bug。`web/gameplay.js:765-766` 的 `胜率暂不可用` 改成能表达原因的说法，并补一条提示框说明。**注意 A8 的测试约束和 `escapeHTML` 用法，别引入未转义的插值。**

### A7. 若确认是接口门控（H1）

唯一现实的备选是用 app 已经在拉的 SGP 战绩反推 —— `gameplay.go:713` 的 `matchHistoryOn` 已经取了窗口内的对局，统计其中 queueId 420/440 的胜负即可。

**但必须与赛季胜率明确区分**，标成「近 N 局 x 胜 y 负」，不能填进 `gameplayRank.WinRate`、不能冒充赛季数据。`verifiedRankWinRate` 的抑制语义保持不变。

### A8. 本工单不能碰的测试约束

| 测试 | 约束 |
| --- | --- |
| `gameplay_test.go:333` | `verifiedRankWinRate(107, 0)` 必须返回 `-1, false` |
| `gameplay_test.go:336` | `verifiedRankWinRate(12, 8)` 必须返回 `60, true` |
| `gameplay_test.go:360` | 日志里必须出现 `"event":"ranked_winrate_resolved"` + `"source":"lcu"` + `"suppressed":true` |
| `gameplay_test.go:380` | 日志里必须出现 `"event":"ranked_winrate_resolved"` + `"source":"sgp"` + `"suppressed":false` |

新埋点是**追加**事件，不替换 `ranked_winrate_resolved`。

---

## 二、工单 B：每把排位的分数变化没出现

### B0. 这不是 bug

`lp_tracker.go:3-14` 的文件头已经写清楚：官方接口（LCU / Riot / SGP）都不提供单场 LP 增减，OP.GG 是靠服务器 7×24 轮询自己攒的；本地助手改成监听 gameflow 事件，在对局结算后轮询排位数据，等场次 +1 时用前后 LP 绝对差反推。

于是有四道闸门，任意一道关上就没有 LP：

1. **只有当前登录玩家** —— `gameplay.go:686-688` `if isCurrent { a.lpTracker.annotate(matches, playerRef) }`。搜索别人 100% 为空。
2. **只有助手运行期间打完的那一场** —— `connection_manager.go:105-110` 由 `EndOfGame` 事件触发。`lp_tracker.go:14` 自己写了「助手关闭期间进行的对局无法回溯」。
3. **只有 420 / 440** —— `lp_tracker.go:36` `lpRankedQueues`。
4. **只有国服本机客户端** —— 依赖 LCU gameflow 事件流。

前端 `web/gameplay.js:1158-1161` 的渲染没问题：`ranked && Number.isFinite(lpValue) && match.lpDelta != null` 才出 chip，数据没来就不出。

**所以「历史战绩全空、搜索别人全空」是符合当前设计的预期行为，不要试图去「修」它。**

### B1. 关于韩服

用户提出「韩服应该好拿到数据」，方向相反：

- 韩服排位走 Riot API `/lol/league/v4/entries/by-puuid/{puuid}`（`riot_api.go:452-455`），`riotLeagueEntry`（`riot_api.go:312-319`）只有 `tier / rank / leaguePoints / wins / losses` 五个字段，**是当前快照，没有任何按对局拆分的维度**。
- Riot 官方 API 从来没有 per-match LP。
- 韩服玩家没有本机 LCU，gameflow 事件不存在，差分机制根本起不来。

**结论：韩服每局 LP 在本地助手架构下做不到。不要开这个坑。**需要在 UI 或文档里明确说明，避免用户反复提。

### B2. 【要做】这条链路零埋点

```
$ grep -c recordDiagnostic lp_tracker.go
0
$ grep -c "lp_" diagnostics-0d59fad4.jsonl diagnostics.1.jsonl
0
0
```

追踪器有没有被触发、有没有拿到 baseline、18 次轮询有没有等到场次 +1、结果有没有落盘 —— **在日志里完全是黑箱**。现在无法区分「机制受限所以没有」和「机制本身也坏了」。

建议在 `lpTracker` 上挂一个和 `sgpProvider.observe`（`sgp_api.go:304-308`）同构的回调，至少补四个点：

| 位置 | 事件 | 字段（**不得含 playerRef / puuid / accountHash**） |
| --- | --- | --- |
| `capture` 入口（`lp_tracker.go:264` 后） | `lp_capture_started` | `game_id`、`queue_type`、`has_baseline` |
| 轮询每一轮（`lp_tracker.go:313` 附近） | `lp_capture_poll` | `attempt`、`capability_state`、`baseline_games`、`current_games` |
| 命中（`lp_tracker.go:329` 附近） | `lp_capture_recorded` | `game_id`、`queue_type`、`delta` |
| 18 次耗尽 | `lp_capture_timeout` | `game_id`、`queue_type`、`attempts`、`last_games` |

`accountHash` 虽然已加盐，仍属稳定玩家标识，**不要写进日志**（`lp_tracker.go:11` 的原则：稳定玩家标识不写文件）。

### B3. 【要做】A 和 B 是耦合的 —— 这是真实缺陷

`capture` 用 `snapshot.games() = Wins + Losses`（`lp_tracker.go:46`）判断场次 +1：

```go
// lp_tracker.go:314
if hasBaseline && snapshot.games() <= baseline.games() { continue }
// lp_tracker.go:322
if hasBaseline && snapshot.games() == baseline.games()+1 { ... }
```

`lp_tracker.go:243-244` 的注释也写着「输掉的排位靠它才能察觉场次 +1」。

**但工单 A 已经证明国服 SGP 大量返回 `losses = 0`。**一旦当前登录玩家某次也落进精简分支，输掉的那一把 `games()` 完全不变 → 轮询 18 次全部 `continue` → 超时 → **输的场次永远记不上，只有赢的能记上**，而且下一场的 baseline 也是脏的。

而 `lpSnapshotFromRank`（`lp_tracker.go:175-177`）无条件接收，没有任何拒绝逻辑。

建议加一道和 `verifiedRankWinRate` 同口径的护栏：

```go
// trustworthy 判断快照是否可用于差分。wins>0 而 losses==0 的响应
// 是不完整记录（见 verifiedRankWinRate），拿它当基线或终值都会算错。
func (s lpSnapshot) trustworthy() bool {
	return !(s.Wins > 0 && s.Losses == 0)
}
```

用在两处：`observe`（`lp_tracker.go:181`）不完整快照不写基线；`capture`（`lp_tracker.go:313` 附近）不完整快照当作「数据未同步」`continue`，并记一条 `lp_snapshot_rejected` 埋点。

> **已知边界**：全新账号首胜确实是 `1 胜 0 负`，会被这条规则挡掉一次。可以接受 —— 与 `verifiedRankWinRate` 的口径保持一致比这个边角更重要。

目前日志显示当前登录玩家 14 次全都拿到了完整的 37/30、131/138，这条**暂时**是稳的，所以这是加护栏不是救火。

### B4. 【要做】UI 区分「没记录」和「助手当时没开」

现在两者都是一片空白，用户无法分辨。建议在排位对局（420/440）且 `lpDelta` 为空时给一个极淡的占位，提示框说明「助手未运行期间的对局无法回溯」。

**约束**：提示框正在改版（见 `design/tooltip-redesign/`），新增的 `data-tooltip` 内容要遵守「首行即标题」的约定，否则分层后首行会被误加粗。

### B5. 本工单不能碰的测试约束

| 测试 | 约束 |
| --- | --- |
| `lp_tracker_test.go:73-74` | 命中的对局 `LpDelta` 必须等于 24 |
| `lp_tracker_test.go:76-77` | 未命中的对局 `LpDelta` 必须是 `nil` |
| `lp_tracker_test.go:83-84` | `LpDelta` 不得泄漏到其他玩家的战绩 |

加护栏时注意别让测试里的构造数据落进 `trustworthy() == false` 分支。

---

## 三、工单 C（附带发现）：RANKED 路由 23% 请求连接失败

两份日志里 `route:"RANKED"` 共 2065 次请求：

| http_status | 次数 | 占比 |
| --- | --- | --- |
| 200 | 1592 | 77.1% |
| 0（连接未建立，`body_bytes:0`） | 473 | 22.9% |

`http_status:0` 是 `p.http.Do` 直接失败（`sgp_api.go:331-335` 那条埋点路径），不是 HTTP 错误码。四分之一的排位请求打不出去，这会放大工单 A 的观感，也可能拖慢队伍列表加载。

需要单独排查：是并发批量打爆了连接池、还是网关限流、还是 token 刷新期间的竞态。**本轮不改，只记录。**建议在 `sgp_request` 的 `http_status:0` 分支上补记 `error_kind`（超时 / 连接拒绝 / DNS），用 `safeDiagnosticReason`（`gameplay.go:1098`）脱敏后落盘。

---

## 四、验收清单

1. `go vet ./...` / `go test ./...` / `go test -race ./...` / `go build ./...` 全绿。
2. `node --test web/*.test.cjs desktop/*.test.cjs` 仍是 59 pass / 0 fail。
3. `sgp_api_test.go:273` 已同步为新签名（A4-3），且断言未被削弱。
4. 真机搜 3–5 个玩家后，日志里出现 `sgp_ranked_stats_shape`，且 `top_level_keys` / `queue_keys` 非空。
5. **日志里不得出现 puuid、玩家昵称、accountHash，或任何排位数值以外的 payload 内容。**用 `grep` 抽查一遍新事件。
6. 打完一把排位后，日志里出现完整的 `lp_capture_started` → `lp_capture_poll` × N → `lp_capture_recorded`（或 `lp_capture_timeout`）。
7. `web/` 是 `go:embed` 的，改了前端**必须重新 `go build`** 才能在桌面壳里看到效果。
8. A6 / A7 **本轮不实现**，等第 4 步的日志回来再定。
