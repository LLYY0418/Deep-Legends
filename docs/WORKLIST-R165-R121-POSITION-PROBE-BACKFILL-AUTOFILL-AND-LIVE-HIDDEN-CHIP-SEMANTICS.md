# WORKLIST-R165：软件打不开（本地数据服务已退出）；R121 两份位置探测文件没有被处理；实时对局"隐藏战绩"标签挂错了字段

诊断人：Claude（只读：用户截图 + 上传的两份 R121 探测文件 + 读仓库代码与 R161–R164 账本，未改仓库代码；本会话无 Go 工具链，P0 的根因需 GPT 用真实崩溃日志确认，不能凭静态审查下结论）。
执行人：GPT。
日期：2026-09-26。
基线：0.12.29（R164 之后），仓库 HEAD `35788b9f`，工作树含 R144–R164 未提交改动。
触发：用户截图显示打开客户端立即报"本地数据服务已退出，自动重试无法恢复"；另转述"GPT 已根据两份 R121 探测文件做了修复，并做了实时对局隐藏玩家详情卡片显示和隐藏玩家标签"，请求核对。

**P0 是本单最高优先级，必须先做**——软件现在完全打不开，比 P1/P2 的功能对错更紧急。

---

## P0　软件打不开：先拿真实崩溃日志，不要凭猜测改代码

### 现状与我能确认到什么程度

- 这是"本地数据服务已退出"（`desktop/web/app.js:2823`），代表 Go 后端子进程被 Electron 检测到已经退出（`desktop/main.cjs` 的 `onBackendClosed`），不是网络或前端的问题。
- 我读了自 R160 以来新增的所有后端代码（`bundled_augments.go`、`bundled_champion_names.go`、`live_roster_recovery.go`、`hexdata_augment_icons.go`、`gameplay.go`/`gameplay_refresh.go`/`lp_tracker.go` 的改动），静态审查没有找到能在**刚打开软件、还没进对局**时就触发的明显 bug——`总览` 页在没有对局数据时也会崩，说明问题很可能出在总览加载路径上（比如 `overviewChampionNames` 新接的 `bundledChampionNames()` 分支，或 `handleGameplayPerks`/`handleGameplayAugments` 新接的离线增益图标合并逻辑），但静态审查定不了位，**没有编译器和真机就不能瞎猜着改**，改错了反而掩盖真正的问题。
- 好消息是：Go 后端的 `stderr`（崩溃时会带完整堆栈）已经被 Electron 原样写进了本机日志文件，**不需要用户自己复现或截图控制台**，直接要这个文件就有答案。

### 需要用户先做一步（现在就能做，不用等 GPT）

打开这个文件（`Deep Legends` 是产品名，`%APPDATA%` 展开后一般是 `C:\Users\<你的用户名>\AppData\Roaming`）：

```
%APPDATA%\Deep Legends\logs\desktop.log
```

如果这个文件很旧或者找不到崩溃记录，同目录下的 `desktop.1.log`、`desktop.2.log` 是更早的滚动备份，也翻一下。把最后几十行（尤其是带 `panic:` 或一大段 `goroutine ... [running]:` 堆栈的部分）发给我或者 GPT。

### 需要 GPT 做的事

1. 拿到用户发来的 `desktop.log` 崩溃片段，定位到具体是哪一行 `panic`（数组越界、空指针、并发 map 读写等）。
2. 根据崩溃行去改对应代码，**不要在没看到崩溃堆栈之前就动手改**；如果用户暂时拿不到日志，GPT 在自己电脑上用真实 League 客户端把从 R160 到现在新增的功能（离线英雄名/图标、离线海克斯图标、对局隐藏玩家、组队排序）逐个跑一遍复现，优先测"刚打开软件、总览页"这个最短路径。
3. 定位到根因后，写清楚：崩溃点、触发条件、修复方式，做对抗变异（改回崩溃前的写法，确认能复现崩溃；改成修复后的写法，确认不再崩）。
4. 修复后必须有一次**从 Windows 安装包冷启动到总览页正常显示**的真实验证，不能只靠 `go test` 通过就算数——这次的问题很可能是"编译能过、单测能过，但真实启动会崩"的类型。
5. 在 `r165-execution-ledger.md` 里写清楚根因和验证过程，这条比 P1/P2 优先记录。

### 临时缓解（如果用户等不及、想先用起来）

如果 GPT 一时定位不到，且用户着急使用，可以用上一个已知能打开的版本（R163 之前，0.12.28 或更早）先凑合用；但**不要把这个当正式修复**，P0 仍然要定位到根因。

---

证据文件（用户上传）：
- `position-lobby.jsonl`：`run_id=c6239426333587e7739615f7`，`trace_id=2f87778ded39cf7351b61bc6`，2026-09-25T14:29:21Z。
- `position-champselect.jsonl`：`run_id=d62ca525ccca7c1beb76178e`，`trace_id=c026ce0ad394c7207b3e25a6`，2026-09-25T14:29:31Z。

## 执行纪律（延续 R117–R164）

- 每项验收判据都要做**对抗变异**：改回原样或换等效写法，对应测试必须 FAIL；变异在隔离副本里做，还原用"整文件拷回 + `diff -q`"，禁止 `git checkout -- <file>`。
- 不记录任何玩家标识、召唤师名、位置明细到诊断日志；只记数量。
- 补位信息只保存在内存中（本局有效），**不落盘**。如果实现中发现必须落盘，停下来问用户——会牵涉 `features.go` 的 `stores` 隐私声明，文案由用户拍板。
- 界面不加说明性文案、tooltip 说明或统计口径文字。

---

## P1　R121 两份探测文件没有被处理，但它们已经回答了 R121 的问题

### 现状（为什么说"没处理"）

- 两份文件的 `trace_id`（`2f87778d…`、`c026ce0a…`）在 `docs/`、`backend/` 里**一次都没出现**。
- `docs/r121-position-probe-findings.md` 最后修改于 9 月 22 日（早于这次探测），§1 结论表仍全部是"待真机运行填写"。
- `backend/riot_api.go:1011` 的 `autofillLabelGate` 仍为 `false`；后端没有任何地方读取 `isAutofilled`。
- 探测之后的 R162/R163/R164 三张单的来源分别是其他截图与诊断日志（`0926-0046`、`0926-0105`、`0926-0214`），都不是这两份文件。R162 的"按位置排序"用的是游戏流程里已有的 `selectedPosition`，与 R121 无关。

### 两份文件说明了什么

两次运行都 `verdict=found`，三个关键端点均 HTTP 200：

| 端点 | 命中键（名称 · 类型 · 所在对象） | 意义 |
|---|---|---|
| `/lol-champ-select/v1/session` | `assignedPosition`（string）、`isAutofilled`（boolean），`myTeam[]` 5 组、`theirTeam[]` 5 组；另有 `positionSwaps`（array） | **选人阶段每个队友都有客户端直接给出的"是否补位"布尔值**——这是 R119/R121 一直想要、却只能靠战绩推断的东西 |
| `/lol-lobby/v2/lobby` | `localMember` 与 `members[]`（3 名成员）各有 `firstPositionPreference`、`secondPositionPreference`（string），`thirdPositionPreference`–`fifthPositionPreference`（null），`autoFillEligible`、`autoFillProtectedForPromos/Remedy/Soloing/Streaking`（boolean）；`gameConfig.showPositionSelector`、`scarcePositions` | 房间内成员的分路偏好与补位保护状态 |
| `/lol-gameflow/v1/session` | `gameData.teamOne[]`/`teamTwo[]` 各 5 组 `selectedPosition`、`selectedRole` | 已在使用（R162 的排序来源） |
| `/lol-match-history/…/matches` | 0 命中，但 `keys_truncated=true`、扫描深度上限 3 | 不构成否定证据，本单不需要 |

需要写进结论的三条限制：

1. **两次都是在选人阶段跑的**。"大厅"那份文件里 `/lol-champ-select/v1/session` 也返回了 200 和完整的 `myTeam`/`theirTeam`，两次间隔只有 10 秒。所以没有真正"进选人前"的快照；不过大厅端点在选人阶段仍返回同样 7060 字节的数据，偏好字段照样拿到了，不需要补跑。
2. **探测只记键名和类型，不记取值**（设计如此）。已知 `isAutofilled` 存在且是布尔值，但不知道对方队伍（`theirTeam`）的取值是否被客户端填充——排位里对方信息通常被隐藏，很可能恒为 `false`/空字符串。
3. openapi 两个契约来源仍是 404；本结论完全来自端点直探，这是 R121 整改后的既定方案。

### 需要 GPT 做的事

**P1-1 归档与回填**
1. 两份文件按字节复制到 `docs/r121-validation/`（新建），建议命名 `position-probe-a-0925.jsonl`（原 lobby，注明实际处于选人阶段）、`position-probe-b-0925.jsonl`（原 champselect）；README 写明 trace_id、时间、SHA-256。
2. 回填 `docs/r121-position-probe-findings.md` §1 结论表（按上表），写明上述三条限制，结论：**找到了——选人阶段己方 `isAutofilled` 可直接读取；对方取值未知**。
3. `WORKLIST-INDEX.md` 里 R121 行改为已关闭并指向本单。

**P1-2 用 `isAutofilled` 给实时对局的队友标"补位"**
1. 在读取 `/lol-champ-select/v1/session` 的结构（`backend/gameplay.go` 里 `lcuChampSelectSession` / `lcuLivePlayer`）中加入 `isAutofilled`、`assignedPosition`。
2. **只采用 `myTeam[]`**（含自己）。`theirTeam[]` 本轮不展示，只进诊断计数。
3. 选人阶段读到的值按"本局 + 会话级玩家引用"存在内存里，进入游戏（InProgress/Reconnect）后选人接口会 404，要能把补位信息带进游戏中的详情卡片；换局即清空。
4. 前端实时详情的队友卡片在 `autofill === true` 时显示已有样式的"补位"标签（文字沿用现有"补位"）。
5. 这是客户端直接给的事实字段，**不要**去动 `autofillLabelGate`；战绩表里那套推断标签（`riotAutofillCandidates`/`riotAutofillFlags`）保持现状。
6. 新增诊断事件（名字自定，如 `champ_select_autofill_shape`），只含数量：己方人数、己方 `isAutofilled=true` 人数、对方 `isAutofilled=true` 人数、对方 `assignedPosition` 非空人数。对方两项用来回答限制 2。
7. 测试与对抗变异：改成读 `theirTeam` → 测试失败；进入游戏后丢失补位标记 → 测试失败；换局不清空 → 测试失败。

**P1-3 清理 R121 探测代码**（R121 立项时约定"结论回填后整体删除"，与 R155 同理）
- 删除 `backend/position_contract_probe.go`、`backend/position_contract_probe_test.go`；删除 `dist/probes/r121-position-probe.exe`、`run-r121.ps1`，`README.txt` 里只保留 R153 `skinName` 复核那段。
- `go build`/`go vet`/`go test ./...`、Node 全量通过。

### 真机验收（用户操作，GPT 写进账本待办）

打一局单双排或灵活组排，在客户端选人界面看到己方某人头像旁有官方补位图标时：Deep Legends 实时详情里**只有这个人**带"补位"标签；导出诊断日志，新事件里己方计数与之吻合，同时记下对方两个计数。

---

## P2　R164 的"隐藏战绩"标签挂在"隐藏身份"字段上；真正隐藏战绩的玩家反而不显示

### 证据

- 后端有两个不同的字段，注释写得很清楚（`backend/gameplay.go:277-280`）：`hidden` = 身份不可见；`privateHistory` = 玩家在客户端开启了"隐藏战绩"。
- 实时对局构造玩家时（`backend/gameplay.go` 约 7548 行）：`hidden = nameVisibilityType == "HIDDEN" || 名字为空`——这是**身份隐藏**。`PrivateHistory` 在实时路径里**从来没被赋值**（全仓只在总览 `gameplay.go:1347` 和韩服 `riot_api.go:1593` 赋值），尽管这里已经拿到了 `summoner.Privacy`（同一段代码正用它算段位分）。
- R164 前端（`backend/web/gameplay.js:5421`）：`player.hidden === true` 时显示"隐藏战绩"。
- 结果：
  - 身份隐藏的玩家（包括 R163 补出来的匿名席位——`live_roster_recovery.go:168` 把它设成 `NameVisibilityType: "HIDDEN"`）被标成"隐藏战绩"，但我们并不知道他们是否隐藏了战绩；
  - 名字正常、但开启了隐藏战绩的玩家**不会**显示标签。我在本机用 R164 测试同样的方法渲染 `{hidden:false, privateHistory:true}`，结果没有标签。
- `backend/web/r164.test.cjs:63-65` 的测试用例本身就是 `{displayName:'隐藏玩家', hidden:true}` 断言出现"隐藏战绩"，把这个错位固化成了"正确行为"。
- 总览页（`gameplay.js:1974`）用 `hidden || privateHistory` 显示"隐藏战绩"，身份隐藏也被算进去，是更早就有的同类问题。

R163 本身（匿名席位补卡片）的判定条件是保守的：只在双方各 5 人、阵营和缺口都核实、缺口恰好等于空 Riot ID 席位数时才补，不猜身份不附战绩——这部分没问题，只是它被 R164 贴上了错误的标签。

### 需要 GPT 做的事

1. 后端实时路径按总览同一规则设置 `PrivateHistory`（`summoner.Privacy == "PRIVATE"`）。R163 的匿名席位不设。
2. 实时详情卡片拆成两个标签：`privateHistory === true` → "隐藏战绩"；`hidden === true` → "隐藏身份"。两者都为真时两个都显示。沿用现有标签样式，不加 tooltip 说明。
3. 总览页 `gameplay.js:1974` 同样拆开（"隐藏战绩"只看 `privateHistory`；身份隐藏已有"身份已隐藏"字样，不再额外挂"隐藏战绩"）；删除那条"该玩家在客户端里开启了隐藏战绩"的 tooltip。
4. `live_roster_rendered` 的 `hidden_rendered` 拆成 `hidden_identity_rendered`、`private_history_rendered` 两个计数。
5. 改正 `r164.test.cjs` 的用例，新增：`{privateHistory:true, hidden:false}` 必须出现"隐藏战绩"且不出现"隐藏身份"；`{hidden:true}` 必须出现"隐藏身份"且不出现"隐藏战绩"；后端测试：实时玩家 `Privacy=PRIVATE` → `privateHistory=true`，匿名席位 → `privateHistory=false`。
6. 对抗变异：前端改回只看 `hidden` → 失败；后端不赋 `PrivateHistory` → 失败。

### 真机验收（用户操作）

下一局遇到开了隐藏战绩的玩家（战绩区显示"客户端未公开该玩家"）时，名字旁出现"隐藏战绩"；遇到名字显示为隐藏的席位时出现"隐藏身份"。

---

## 验收总表

| 项 | 验收标准 |
|---|---|
| P0 | 拿到真实崩溃日志（`desktop.log` 里的 `panic:` 堆栈或用户复现）定位根因；修复后有一次 Windows 安装包冷启动到总览页正常显示的真实验证，不只靠单测 |
| P1-1 | 两份文件归档且 SHA-256 与上传件一致；findings §1 回填且写明三条限制；R121 在索引中关闭 |
| P1-2 | 仅己方 `isAutofilled=true` 的队友显示"补位"，进游戏后仍在、换局清空；诊断只含四个计数；三条对抗变异均打红 |
| P1-3 | R121 探测 Go 文件与 Windows 探针已删；构建与全量测试通过 |
| P2 | 实时与总览的"隐藏战绩"只看 `privateHistory`，身份隐藏显示"隐藏身份"；诊断计数拆分；测试用例纠正且两条对抗变异打红 |
| 真机 | P1-2 与 P2 各一局截图 + 诊断日志，GPT 在账本列为待办 |
