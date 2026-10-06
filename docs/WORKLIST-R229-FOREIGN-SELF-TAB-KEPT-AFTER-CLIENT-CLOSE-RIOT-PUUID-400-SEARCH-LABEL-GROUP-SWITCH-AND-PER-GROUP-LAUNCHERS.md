# WORKLIST-R229：外服客户端关闭后本人战绩不消失；本人总览 Riot 接口 400（用了客户端 PUUID）；搜索框文字、空分组切不过去、按分组显示启动入口；Riot 启动偏慢

诊断：Claude（1 张截图 + 日志 `lol-loot-diagnostics-1006-0042.jsonl` + 只读核对源码）。执行：GPT。日期：2026-10-06。
基线：**0.12.74**，`run_id b719f006df451835af827856`（北京时间 00:33～00:42）。

用户确认已正常：外服战绩能看到；国服纯净入口点「否」后显示「已取消」，没有再去直接启动 LeagueClient.exe（日志 `official_login_launch result=cancelled`，R223 P3 生效）。

---

## 日志时间线（北京时间）

| 时间 | 事件 |
|---|---|
| 00:33:30.10 | 点 Riot 入口：`client_launch requested` → 47ms 后 `validated source=rc_live product_install_class=riot` → `started` |
| 00:34:10.19 | 第一次发现客户端进程：`lcu_discovery probe-failed`（进程在，本地接口还没响应） |
| 00:34:27.24 | `lcu_discovery connected`（比上一次晚 17 秒） |
| 00:34:29.45 | `client_platform_resolved region=JP platform=JP1 source=command-line-query`（R223 P2 生效） |
| 00:34:35.69 | `match_history_data_source_decision client_region=jp1`：**riot failed「Riot 接口返回 HTTP 400」**，退回 LCU 成功；`detailed_matches=5975ms` |
| 00:35:15.63 | 用户关闭客户端：`lcu_event_stream dropped` → `lcu_discovery credentials-unreadable` → **`blocking_state_client show`**（又弹出「正在启动」遮罩）→ 00:35:18 `hide no-client-process` |
| 00:36:22 起 | 客户端已关闭，**仍每分钟 `overview_current_game region=jp1`**（00:36:22、00:37:10、00:38:24、00:42:51），说明本人日服页签还在、还在轮询 |

---

## P1　外服客户端关闭后，本人页签和战绩仍然保留（还在每分钟轮询）

### 根因（源码已核对）

- `gameplay.js` `playerGroupCount`：`!tab.current || connected() || riotTab(tab)`。本人页签只要是外服，**断开后仍被计数、仍显示**。这是 R221 执行时的设计（账本写着「外服本人断连后保留可读数据」），用户现在明确不要。
- 页签不消失，所以它的 OP.GG 当前对局轮询也一直在跑（R223 P6 只在「大区未知」时停，不管断开）。

### 修复要求

1. 客户端断开（`connected=false`）后，**本人页签不论国服还是外服都按国服现在的方式处理**：不再显示本人数据，不计入分组数量，对应分组显示空页面和启动入口（见 P4）。
2. 断开时取消本人页签的在途请求、OP.GG 当前对局轮询和自动重试计时器；不要再按本人页签发任何请求。
3. 用户手动搜索打开的外服玩家页签（非本人）不受影响，照常保留。
4. 重新连上同一个账号时，本人页签按正常流程重新加载，不复用断开前的过期数据。

### 验收

- 前端测试：日服本人页签已加载 → 状态变为未连接 → 本人页签不显示、外服分组数量减 1、`overview_current_game` 相关请求和计时器全部停止；搜索打开的日服玩家页签仍在。
- 真机：日服登录 → 关闭客户端 → 本人战绩立即消失，出现启动入口。

---

## P2　本人总览走 Riot 接口返回 400：直接用了客户端的 PUUID

### 根因（源码已核对）

- `riot_client_history.go` `loadClientRiotHistory(ctx, region, puuid, ...)` 直接把 LCU 返回的本人 PUUID 传给 `matchIDsForOverview`，即 `/lol/match/v5/matches/by-puuid/{puuid}/ids`。
- 拳头公开 API 的 PUUID 是**按 API Key 加密**的，和客户端（LCU）里的 PUUID 不是同一个值。拿客户端 PUUID 去调公开 API，会返回 400（错误体通常是 `Exception decrypting`）。
- 对比：用户在搜索框搜日服玩家时走的是 `account-v1 by-riot-id`，拿到的是公开 API 的 PUUID，所以 10 场全部成功（`riot_overview_cost matches_loaded=10`）。
- 现在每次本人总览都先白白等一个 400，再退回 LCU；LCU 详情很慢（`detailed_matches` 5975ms），而且只有本人能读到的数据。

### 修复要求

1. 本人走 Riot 接口前，先用客户端提供的 `gameName#tagLine` 调 `account-v1 by-riot-id`（按 R221 的 SEA→ASIA 账号集群规则）换成公开 API 的 PUUID，按「平台 + 客户端 PUUID」缓存（6 小时，换账号失效）。
2. 用换来的 PUUID 查对局 ID、对局详情、段位、熟练度；在对局里找「本人」时也要用这个 PUUID（`riotConvertMatch` 的 `puuid` 参数）。
3. 全仓检查：凡是把 LCU 的 PUUID 直接用于 Riot 公开 API 的地方（本人总览、对局里的玩家、实时对局玩家战绩），统一走同一个换算函数。对局里的其他玩家，如果 LCU 给了 Riot ID，就按 Riot ID 换算；拿不到时退回 LCU，不要拿客户端 PUUID 去碰公开 API。
4. 中转返回 400 时，在 `match_history_data_source_decision` 里区分原因：`riot-puuid-mismatch`（错误体含 decrypt）和其他 400，不记录错误体原文。
5. 换算失败（账号不存在、429、超时）就按原逻辑退回 LCU。

### 验收

- Go 测试：mock 中转对客户端 PUUID 返回 400 decrypt、对 by-riot-id 换来的 PUUID 返回 200 → 本人总览只走 Riot、不再出现 400、本人高亮正确；换算结果被缓存，第二次加载不再请求 account 接口。
- 真机：日服本人总览日志里 `selected=riot`，不再有「HTTP 400」。

---

## P3　搜索框按钮文字

截图里按钮显示「外服 · 跟随客…」，被截断，也不直观。

要求（**只改搜索框按钮上的文字**，下拉菜单的结构和选项都不变）：

- 选外服任一服务器，或外服「跟随客户端」：只显示区名，如「**韩服**」「**日服**」。
- 选国服具体大区，或国服「跟随客户端」：只显示大区名，如「**黑色玫瑰**」「**艾欧尼亚**」。国服跟随客户端但还没识别出大区时显示「国服」。
- 不再出现「外服 ·」「国服 ·」前缀和「跟随客户端」字样。

验收：前端测试覆盖四种情况（外服固定、外服跟随、国服固定、国服跟随已识别/未识别）。

---

## P4　分组切换：国服为 0 时切不过去；空分组显示空白页加对应的启动入口

### 根因（源码已核对）

- `gameplay.js` `selectPlayerGroup`：`group === "players" && !connected()` 时直接取 `state.tabs.find(tab => tab.current)` 作为目标页签。本人页签是日服时，它属于外服组，`selectPlayerTab` 又把分组切回外服，所以点「国服」看起来没反应。

### 修复要求

1. 三个分组都**始终可切换**，包括数量为 0 的。切过去后，如果组里没有页签，显示该分组的空白页。
2. 空白页内容（不加解释性文字）：
   - **国服**：显示国服的启动入口，即「**国服纯净入口**」和「**WeGame**」两个。
   - **外服**：只显示「**Riot 客户端**」入口。
   - **职业**：保持现有空状态（从职业选手目录打开账号）。
3. 当前的启动入口卡（`renderLaunchpad`）改成按分组过滤：国服组只出 TCLS 和 WeGame，外服组只出 Riot。客户端已连接时，入口按原逻辑收起。
4. **WeGame 入口**（新增）：
   - 检测：从注册表读取 WeGame 安装路径（`HKLM/HKCU\SOFTWARE\Tencent\WeGame` 的 `InstallPath`；具体键名在 Windows 上核实，写进账本），再加上桌面/开始菜单里名字含「WeGame」的快捷方式。`classifyClientShortcut` 目前把含 `wegame` 的快捷方式直接丢掉，改为归到新的 `wegame` 入口。
   - 启动：打开 WeGame 主程序（或快捷方式）。如果 WeGame 支持直接拉起英雄联盟的参数或协议，经核实可用就用上；不确定就只打开 WeGame，不要猜参数。
   - 诊断：沿用 `client_launch`，`client_id=wegame`，来源只记枚举（registry / shortcut）。1223 取消的处理与 R223 一致。
5. 本人页签断开后（P1）落在哪个分组：显示上次连接的那个分组的空白页和入口；首次启动时默认国服组。

### 验收

- 前端测试：未连接、国服 0 个页签 → 点国服 → 进入国服空白页，显示纯净入口和 WeGame；点外服 → 只显示 Riot；外服有搜索页签时显示页签，不显示入口。
- Go 测试：WeGame 快捷方式和注册表检测、启动候选、1223 取消。

---

## P5　Riot 入口启动偏慢：哪些是我们的问题

日志拆解，从点击到总览出数据共约 65 秒：

| 段 | 耗时 | 归属 |
|---|---|---|
| 点击 → Riot Client 启动 | 47ms | 我们，正常 |
| Riot Client 登录、启动英雄联盟、LeagueClientUx 进程出现 | ≤40 秒（00:33:30 → 00:34:10 首次发现） | 主要是拳头客户端自身；但**我们在客户端未运行时按 3～8 秒退避轮询**（`minimumDiscoveryBackoff`/`maximumDiscoveryBackoff`），最多再晚 8 秒才发现 |
| 进程出现 → 本地接口可用并连上 | 17 秒（probe-failed → connected） | 客户端自身启动 + 我们的退避（同上） |
| 连上 → 总览出数据 | 8 秒，其中先白等一个 Riot 400，再走慢的 LCU 详情 6 秒 | **我们的问题**（P2） |

修复要求：

1. 用户刚点过任一启动入口（返回 `started` 后的 120 秒内），以及发现结果是 `credentials-unreadable` / `probe-failed`（客户端正在启动）时，发现轮询改为每 1 秒一次；超过 120 秒或连上后，恢复原退避。不影响「没有点启动、客户端也没运行」时的轮询频率。
2. P2 修好后，连上到出数据应在 3 秒左右（Riot 接口路径 `riot_overview_cost` 本次约 2.5 秒）。
3. 增加诊断 `client_launch_to_connected`：`client_id`、`ms_to_process`（首次发现进程）、`ms_to_connected`、`ms_to_overview`（总览首屏完成），只记数字。这样下次能直接看出慢在哪段。

验收：Go 测试——启动后 120 秒窗口内发现间隔为 1 秒，窗口外恢复退避；诊断字段齐全且只含数字和枚举。

---

## P6　关闭客户端时短暂弹出「正在启动」遮罩

- 00:35:15 用户关闭客户端，退出过程中进程还在但凭据读不到（`credentials-unreadable`），R219 的规则把它当成「客户端正在启动」，弹出遮罩 3 秒。
- 要求：刚从「已连接」变成断开后的 15 秒内，`credentials-unreadable` / `probe-failed` 视为客户端正在退出，**不显示遮罩**；只有在这段时间之后，或用户刚点过启动入口时，才按 R219 显示「检测到客户端正在启动」。
- `blocking_state_client` 的 `hide_reason` 不变；跳过显示时记 `reason=skip`、`skip_reason=client-exiting`。

验收：前端测试——已连接 → 断开（credentials-unreadable）→ 不显示遮罩；冷启动时遇到 credentials-unreadable → 仍显示。

---

## 测试与记录

- Go：P2 PUUID 换算和缓存、P4 WeGame 检测与启动、P5 发现间隔与诊断。跑 `go test ./backend -count=1`、`go vet ./backend`。
- 前端：P1、P3、P4、P6。跑全量 `node --test backend/web/*.test.cjs` 和 `node scripts/test-renderers.cjs all`。
- 新建 `docs/r229-execution-ledger.md`，验证日志放 `docs/history/reports/r229/`；在 `docs/WORKLIST-INDEX.md` 登记。
- 真机（由用户执行）：日服登录后关闭客户端，战绩立即消失并出现 Riot 入口；日服总览不再有 400；国服 0 个页签时能切到国服并看到纯净入口和 WeGame；搜索框只显示区名或大区名；关闭客户端不再闪「正在启动」遮罩；Riot 入口启动后看 `client_launch_to_connected` 各段耗时。
