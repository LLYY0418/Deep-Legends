# WORKLIST-R175：对局补位恢复后数组越界 panic，导致我方隐藏玩家显示不出、后台进程崩溃；补 panic 兜底与崩溃证据导出

诊断人：Claude（只读核对 1001-1347 日志 + 源码，未改源码）。执行人：GPT。日期：2026-10-01。基线 0.12.39（指纹 4d2e848cbe8c）。

## 用户现象

1. 海克斯大乱斗（queue 2400 / KIWI）对局中，详情页**我方只有 4 人**，少的那个是隐藏战绩玩家；对方的隐藏玩家能正常显示（截图一）。
2. 点刷新后整页变成「实时对局读取失败 / Failed to fetch」（截图二），左下角「本地助手无响应」。
3. 用户反馈「刚安装的软件没有了」（具体是窗口消失还是安装被删，待用户确认，见 P3）。

## 结论

**这是 R161 / R163 引入的致命 bug，不是数据问题。** `loadGameplayLive` 在补位恢复之前就按旧人数分配了一个切片，恢复补进玩家后，后面的循环按新人数写这个切片，越界 panic：

- `backend/gameplay.go:7588`：`liveClientPositions := make([]string, len(rawPlayers))`，此时 `rawPlayers` 只有 9 人（gameflow 漏掉了我方隐藏玩家）。
- `backend/gameplay.go:7599-7605`：`recoverClassicLiveRoster(...)` 补进 1 个匿名占位（R163 的 `anonymous-placeholder`），`rawPlayers` 变成 10 人。
- `backend/gameplay.go:7715-7719`：`for index, raw := range rawPlayers { … liveClientPositions[index] = position }`，index 9 写入长度 9 的切片 → `panic: runtime error: index out of range [9] with length 9`。

R161 的具名恢复（排位 420/440，`reason: resolved/partial`）走的是同一段代码，**只要补进 ≥1 人就会 panic**。也就是说 R161/R163 的补位恢复从上线起在真机上一次都没成功过。R161/R163 的测试只直接调用了 `recoverClassicLiveRoster`，没有经过 `loadGameplayLive`，所以没测出来。

### 日志证据（run 77661f22…，对局 9009093994）

| 时间 (UTC) | 事件 | 说明 |
|---|---|---|
| 05:21:33 | `lcu_gameflow_session_shape` teamOne=4, teamTwo=5 | LCU gameflow 漏掉我方 1 人 |
| 05:21:33 / 05:21:42 | `live_roster_recovery` reason=`playerlist-incomplete`, appended=0 | 游戏内 Live Client 还没起来，不补人，不越界 |
| 05:21:37 | `live_prewarm` players=9；`live_roster_shape` players=9, team 100=4 | **这是用户最后看到的 9 人数据，就是截图一** |
| 05:21:48 – 05:21:58 | `live_roster_recovery` reason=`anonymous-placeholder`, appended=1，连续 15 次 | 每次补人后都在 HTTP 请求里 panic。net/http 兜住了 panic，进程没死，但这些请求全部失败，之后**再也没有任何 `live_roster_shape`** |
| 05:22 – 05:38 | 每 50 秒一次 `live_refresh_client` load，后端无任何 live 产出 | panic 时 `cachedGameplayLive` 的 `flight` 没关闭（见下），之后的请求都卡在等它 |
| 05:38:55.53 | gameflow 切到 `Reconnect` → `invalidate()` + 启动预热 goroutine | `gameplay_refresh.go:83-89` 是裸 `go func()`，没有 recover |
| 05:38:55.85 | `live_roster_recovery` appended=1 | 这次在后台 goroutine 里越界 |
| 05:38:56 | **本次进程最后一行日志** | 后台 goroutine 里没被捕获的 panic 直接杀掉整个 Go 进程 → 「Failed to fetch」「本地助手无响应」 |
| 05:44:59 | 新进程启动（同版本 0.12.39，指纹相同） | 用户重新打开 |

### 附带缺陷（同一条链路）

- `gameplay_refresh.go:196-222`（`cachedGameplayLive`）：`a.loadGameplayLive(...)` 如果 panic，后面的 `c.flight = nil`、`close(flight.done)` 都不会执行。同一代际里所有后续请求都在 `<-flight.done` 上等到自己的 ctx 超时，拿到的是空响应。这就是 05:22–05:38 页面一直停在 9 人的原因。只有阶段变化触发 `invalidate()` 才能解开。
- 整个 `backend/` 没有任何 `recover()`，有 161 处裸 `go func()`。任意后台 goroutine 一次 panic 就会让整个应用失效。
- 桌面端会把后端 stderr（也就是 panic 堆栈）写进 `%APPDATA%\Deep Legends\logs\desktop.log`，但导出时出于隐私会整段丢弃（本次 `desktop_export_redacted omittedLines 463`），「本地数据服务已退出（退出码）」这一行也被丢弃。所以诊断日志里看不到崩溃证据，只能靠时间断档去推断。

---

## P1　修越界（必须）

1. 把 `liveClientPositions` 的分配移到补位恢复之后，和 `response.Players / premadeInputs / ranksFinishedAt / matchesFinishedAt`（7606-7609）放在一起，都用恢复后的 `len(rawPlayers)`。
2. 逐行核对 `loadGameplayLive` 里所有按 `len(rawPlayers)` 或按下标对齐 `rawPlayers` 的切片和 map，确认没有其他在恢复之前分配、恢复之后按下标访问的。在账本里列出核对清单：变量名、分配行、访问行、结论。
3. 匿名占位玩家（`NameVisibilityType: "HIDDEN"`，没有 PUUID）后续经过 `loadGameplaySummoner / livePlayerIdentityValues / liveClientPositionForIdentities / sessionPlayers（7738）/ 组队推断` 时，不得再触发其他 panic 或 nil 解引用。逐个函数确认空 PUUID / 空名字的分支，结论写进账本。

### 测试（端到端，不能只测 `recoverClassicLiveRoster`）

用假 LCU + 假 Live Client 快照，从 `loadGameplayLive`（更好是从 `cachedGameplayLive`）入口跑：

- **用例 A（本次真机场景）**：queue 2400，gameflow 我方 4 人、敌方 5 人；playerlist 10 人，我方缺的那人没有 Riot ID。断言：不 panic；返回 10 人；补进的那个在我方（TeamID 与当前玩家一致），`Hidden=true`，没有 PlayerRef，不发起战绩请求；`live_roster_shape` 正常记录 players=10。
- **用例 B（R161 具名路径）**：queue 420，gameflow 9 人，playerlist 10 人，缺的那人有 Riot ID，假的腾讯 Riot ID 查询能解析出 PUUID。断言：不 panic；返回 10 人；补进的玩家走正常战绩加载。
- **用例 C**：恢复返回 `partial`（补进 1 人，仍缺 1 人），不 panic，人数对得上。
- **前端**：用用例 A 的返回体渲染详情页，我方 5 张卡，占位玩家显示为隐藏玩家卡（沿用现有隐藏样式，不加任何新说明文字）。

变异（各自必须是断言 FAIL 或测试捕获到的 panic，不能是编译错误）：

| 变异 | 期望 |
|---|---|
| 把 `liveClientPositions` 分配挪回恢复之前 | 用例 A / B 捕获 panic，FAIL |
| 占位玩家的 TeamID 写成敌方 | 用例 A 队伍断言 FAIL |
| 占位玩家仍去请求战绩 | 用例 A 请求计数断言 FAIL |

## P2　panic 兜底：不能再让一个 bug 杀掉整个进程或卡死实时页（必须）

1. **`cachedGameplayLive`**：给 `loadGameplayLive` 的调用加 `defer`。发生 panic 时也要清掉 `c.flight`、关闭 `flight.done`，返回空响应 `gameplayLiveResponse{Phase: phase}`，并记录诊断。等待中的请求要立刻返回，不能卡到超时。
2. **通用兜底 helper**：新增一个小函数，比如 `a.goSafe(site string, fn func())`，以及同步版 `a.recoverPanic(site)`。panic 时记录一条 `backend_panic` 诊断，进程继续运行。诊断字段**只允许**：
   - `site`（固定字符串）
   - `panic_kind`：从固定白名单里取，例如 `index-out-of-range / nil-pointer / slice-bounds / type-assertion / closed-channel / other`，用 `runtime.Error` 的类型和固定前缀匹配得到
   - `frames`：堆栈里前 8 个 `main.` 开头的函数名，只要函数名，不要参数、文件路径、行号以外的内容（行号可以保留）
   - **不得**输出 panic 值原文、变量内容、玩家名、PUUID
3. **接入点**（本工单必须覆盖）：
   - `gameplay_refresh.go` 里的所有 `go func()`（预热、`finishArenaGroupTruth` 等）。
   - HTTP 层：在 `a.authorized(...)` 包装里加 recover。panic 时记录 `backend_panic`（site=请求路径模板，比如 `/api/gameplay/live`，不带查询参数），返回 500 JSON，不让 net/http 默默断开连接。
4. **其余裸 goroutine**：全仓 `grep -n "go func()"` 列清单，按「长期后台循环 / 一次性后台任务 / 请求内并发」分类写进账本。长期后台循环（轮询、SSE、看门狗、采样器）本工单一并换成 `goSafe`。请求内并发（例如 `loadGameplayLive` 里按玩家开的 goroutine）至少在外层用 `recoverPanic` 包住，保证 panic 不跨 goroutine 杀进程（Go 的 panic 不会传播到父 goroutine，所以每个 goroutine 自己的 defer 都要有）。
5. 兜底只是保命，不能掩盖 bug：测试里要断言 panic 被记录成 `backend_panic`，而不是被吞掉。

### 测试

- 注入一个必 panic 的 `loadGameplayLive` 替身（测试钩子），并发 3 个 `cachedGameplayLive` 请求：都在 1 秒内返回空响应，不卡住；之后恢复正常替身，下一次请求能正常加载（flight 已清掉）。
- 预热 goroutine 里 panic：进程不退出（测试进程还活着），诊断里有 `backend_panic` site=预热，`panic_kind=index-out-of-range`。
- HTTP 路由 panic：返回 500，诊断有记录，下一个请求正常。
- 隐私：`backend_panic` 的字段集合严格等于白名单；panic 值里放一个假 PUUID 字符串，断言它不出现在诊断输出里。
- 变异：去掉 `cachedGameplayLive` 的 defer → 并发请求卡住测试 FAIL；去掉预热的 `goSafe` → 测试进程崩溃，记为 FAIL，账本里写明怎么判定的。

## P3　让下次崩溃能从导出日志里直接看到（必须）

用户本次说「刚安装的软件没有了」，日志里只能看到 05:38:56 到 05:44:59 之间 6 分钟的空白，分不清是用户点了「重启软件」后没拉起来、窗口直接消失，还是安装目录被删或被杀软处理。现在的导出把这些证据全丢了。补结构化事件，**不导出原始 stderr**：

1. `desktop/main.cjs` 的 `onBackendClosed`：写一条结构化日志，导出时转成 `desktop_backend_exit`，字段为 `code`（整数）、`signal`（白名单）、`uptime_ms`、`panic_kind`。`panic_kind` 通过扫描该进程最后 64 KB stderr，匹配 `^panic: runtime error: (index out of range|invalid memory address or nil pointer dereference|slice bounds out of range)` 这类固定前缀，映射到 P2 同一白名单；匹配不到写 `none` 或 `other`。另外加 `frames`：只取 stderr 里形如 `^main\.[A-Za-z0-9_.()*]+\(` 的函数名，前 8 个，去掉参数。
2. 用户点「重启软件」（`desktop-backend-restart`）：记录 `desktop_relaunch_requested`。新进程启动时，如果检测到上一个进程是因为 relaunch 退出的，记录 `desktop_relaunch_completed`，带两次之间的间隔毫秒数（用 userData 下一个小标记文件实现，读到即删）。这样下次就能看出「重启没拉起来」还是「用户自己重开」。
3. `desktopLogForExport` 增加对上述几类结构化行的解析，字段严格白名单，其余行照旧丢弃。
4. **本工单不改 relaunch 行为**（`app.relaunch()` + 单实例锁的时序目前没有证据说明有问题）。只加证据，等用户确认现象后再决定。

### 测试

- 用一段真实格式的 Go panic stderr 夹具（包含假的玩家名和 PUUID 字符串）喂给退出处理，导出结果只含白名单字段，假玩家名和 PUUID 不出现。
- 重启标记：模拟写入、新进程读取、计算间隔、删除标记；标记文件不存在或损坏时不报错。

## 不动的东西

R161/R163 的恢复判定规则（队伍校验、Riot ID 精确查询、匿名占位只限海克斯大乱斗）、R170/R173/R174 的所有功能、界面文字。界面红线：不新增任何说明文字或 tooltip。

## 收尾

- 版本 0.12.40，构建方式同 R174（public key mode，两平台后端 + 指纹校验 + self-test）。
- 全量 `go test ./backend -count=1`、`go vet ./backend`、`node --test backend/web/*.test.cjs`、`node --test desktop/*.test.cjs`、`git diff --check` 全绿。
- `docs/WORKLIST-INDEX.md` 加 R175。在 R161 / R163 的行里补一句「端到端补人会越界 panic，见 R175」。账本写 `docs/r175-execution-ledger.md`，P1 的切片核对清单和 P2 的 goroutine 清单都要写进去。

## 真机验收（用户，Windows）

1. 打一局海克斯大乱斗（或排位），如果再碰到我方有隐藏玩家：我方应显示 5 人，隐藏玩家以隐藏卡片出现；点刷新正常，不会报「Failed to fetch」，左下角不会变成「本地助手无响应」。
2. 导出日志，确认没有 `backend_panic`。如果有，把日志发给 Claude，里面会有 site 和函数名。
3. 如果再出现软件消失，导出日志里会有 `desktop_backend_exit` 和重启相关事件，据此判断是哪一种。
