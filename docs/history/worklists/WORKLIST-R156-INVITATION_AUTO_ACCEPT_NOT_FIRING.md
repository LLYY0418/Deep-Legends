# WORKLIST-R156：好友邀请「自动接受」没有效果

诊断人：Claude（只读：`lol-loot-diagnostics-0925-1427.jsonl`（run_id `ee2c19ab41d3669617cfd8b2`，覆盖 05:33:31Z–06:27:29Z）+ 读代码核对实现，未改仓库任何文件）。
执行人：GPT。
日期：2026-09-25。
基线：0.12.20（`desktop/package.json`），仓库 HEAD `35788b9f`，工作区含 R154/R155 一路带下来的未提交改动。本单行号均以当前工作区文件为准，执行前请先 `grep` 确认未被后续改动移动。
状态：代码与本机自动验证完成，待真实邀请及 Windows 真机复核；执行详情见 `r156-execution-ledger.md`。
触发：用户反馈「自动接受邀请没有效果，好友邀请了我，没有自动接受」，并附诊断日志一份。

## 结论摘要

这次日志**无法直接证明**邀请事件到底有没有触发处理流程——不是因为没查到线索，而是查到了两个会让「有效果的操作在日志里也显示为空白」的独立缺陷（P2、P3）。同时找到一处代码级的字段取值错误（P1），一旦命中会让「已经配置好的队列策略」systematically 失效。三处都值得直接修，且修完 P2 之后，下一份日志就能第一次真正看清这条链路发生了什么。

| 项 | 现象 | 根因（一句话） |
|---|---|---|
| P1 | 即使配置了 420/440/450 全部「接受」，真实邀请也可能一条都匹配不上队列策略 | `handleInvitations` 从邀请对象的**顶层**取 `queueId`，但本仓库自己的 `recordLobbyShape` 已经证明 LCU 把 `queueId` 放在嵌套的 `gameConfig` 里，两处取值方式不一致 |
| P2 | 整份 51 分钟日志里，「自动接受邀请」这条规则一次「已处理/失败」的记录都没有——不管它有没有真的执行过 | `handleInvitations` 是全部 8 条自动规则里唯一只调用 `r.emit`（仅界面提示）、从不调用 `r.record`（写入诊断日志）的一条，天生对日志不可见 |
| P3 | 就算 P2 修好，单次触发的请求也可能不出现在导出的诊断日志里 | LCU 请求诊断按 10 秒窗口分桶，只有「同一路径在更晚的窗口再来一次」或「连接关闭」才会落盘；本次日志里客户端连接从 05:36:36 到日志结束都没断开过，孤立的一次性请求永远等不到落盘的时机 |
| P4 | 用户提出「海克斯大乱斗算不算极地大乱斗的一种」，怀疑 450 应该已经覆盖——**实际不覆盖**，且界面上根本没有单独开关 | `backend/queue_groups.go:120-124` 里海克斯大乱斗是 `queueId 2300/2400/3270`，与极地大乱斗的 `450`（`:106`）是完全不同的队列；`backend/web/suite.js:309-312` 的邀请策略快捷开关列表里也没有海克斯大乱斗这一项，用户根本无法为它单独配置，只能靠范围过宽的「其他队列」兜底 |

补充：本次日志里「房间邀请处理」最终配置为 `enabled:true`，`policies:{"420":"accept","440":"accept","450":"accept"}`（单双排/灵活组排/极地大乱斗），没有配置「其他队列」兜底策略。日志里的 `watch_custom_classification` 事件多次出现 `queue_id:2400`（05:40:33、06:09:22、06:27:25，横跨整段配置和使用窗口）——`2400` 正是海克斯大乱斗（见 P4），说明用户这段时间确实反复处于海克斯大乱斗房间里，与用户描述的场景吻合。**如果好友的邀请就是海克斯大乱斗，那么“配置了 450 却没生效”是必然的**（450 和海克斯大乱斗本来就是两个队列，不是同一个队列的两种叫法），不需要等 P1 的取值错误也能解释；P1 仍然值得修，因为它会让「就算以后专门给海克斯大乱斗配置了策略」依然可能失效。

---

## P1　`queueId` 取值路径与仓库自己已验证过的 LCU 数据结构不一致

### 证据

`backend/watch_rules.go:1299`（`handleInvitations` 内）：
```go
queue := strconv.FormatInt(anyInt(invitation, "queueId", "queueID"), 10)
```
`anyInt`（`backend/watch_rules.go:1464` 起）只读传入 map 的**顶层键**，不做任何嵌套展开：
```go
func anyInt(value map[string]any, keys ...string) int64 {
	for _, key := range keys {
		switch typed := value[key].(type) {
		case float64: return int64(typed)
		...
```

而同一个文件里，`recordLobbyShape`（`backend/watch_rules.go:812-836`）早就替我们验证过 LCU 这类大厅/邀请相关对象的真实结构——`queueId` 不在对象顶层，而在嵌套的 `gameConfig` 里：
```go
func (r *watchRunner) recordLobbyShape(lobby map[string]any) {
	local, _ := lobby["localMember"].(map[string]any)
	config, _ := lobby["gameConfig"].(map[string]any)   // ← queueId 所在的子对象
	...
```
`shouldStartAutoMatchmaking`（`backend/watch_rules.go:798`）读队列 ID 时用的正是这个展开后的 `config`，不是 `lobby` 本身：
```go
if anyInt(config, "queueId", "queueID") <= 0 {
```
本次日志里唯一一条 `lcu_lobby_shape` 事件（05:40:33.3566462Z）也印证了这一点：`game_config_keys` 数组里包含 `queueId`，而 lobby 对象自己的 `top_level_keys` 里没有。

`/lol-lobby/v2/received-invitations` 返回的每一条邀请对象，按 LCU 通用约定同样会把队列/地图这类对局配置信息放进一个 `gameConfig` 子对象（与 `/lol-lobby/v2/lobby` 共用同一套 DTO 风格），而不是摊平到邀请对象顶层。仓库现有的唯一测试（`backend/r50_suite_test.go:144-146`）恰好只用了手写的**扁平**假数据（`{"invitationId": "accept-one", "queueId": 420}`），从未用嵌套结构验证过，所以这条路径从写下第一天起就没有被真实结构检验过——和 R127 P1-c 发现的「测试只核对了自己手写的假设，没人拿真实数据核对过」是同一类问题。

### 根因

`handleInvitations` 在解析每条邀请的队列 ID 时，只看 `invitation["queueId"]`/`invitation["queueID"]`，没有像 `shouldStartAutoMatchmaking` 那样先展开 `invitation["gameConfig"]`。如果 `/lol-lobby/v2/received-invitations` 真实返回的是嵌套结构（本仓库自己的证据强烈指向这一点），那么无论用户在界面上给哪个队列配置了「接受」，`anyInt` 拿到的永远是零值，`queue` 恒为 `"0"`，只会命中一个从未被配置过的 `"0"` 或落回未设置的 `"default"`——也就是说，**这条规则目前很可能对任何真实邀请都不会生效，即使配置完全正确**。

### 修复方向

1. `handleInvitations` 取队列 ID 时，比照 `shouldStartAutoMatchmaking` 的做法，先尝试展开 `invitation["gameConfig"]`（`map[string]any`），从其中取 `queueId`/`queueID`；展开失败或取不到时，再退回邀请对象顶层的 `queueId`/`queueID` 作为兜底（两种都尝试，不要只改成单一路径，防止将来 LCU 版本差异导致两头都不生效）。
2. 顺手加一条一次性的 shape 诊断：仿照 `recordLobbyShape`（`backend/watch_rules.go:812`）的写法，在 `handleInvitations` 第一次真正拿到非空 `invitations` 列表时，记一条新事件（例如 `lcu_invitation_shape`），只记键名（顶层 + `gameConfig` 子对象，如果存在），不要记邀请人 ID、召唤师名等任何可识别信息。这样下一份日志能直接、确定性地证明真实结构到底是不是嵌套的，不用再靠推断。
3. `r50_suite_test.go` 现有测试的假数据改成嵌套结构（`{"invitationId": "...", "gameConfig": {"queueId": 420}}`），同时**新增**一条断言顶层扁平结构也仍然兼容的用例（覆盖两种写法都要通过）。

### 验收

- 新增/修改测试：分别用「顶层 `queueId`」「嵌套 `gameConfig.queueId`」两种邀请数据跑 `handleInvitations`，两种都必须正确匹配到配置的策略并发起 accept/decline 请求。
- 对抗变异：去掉嵌套展开逻辑，只保留顶层读取，嵌套结构用例必须 FAIL。
- 真机验收：GPT 用真实邀请触发一次（或用新增的 `lcu_invitation_shape` 诊断从下一份真实日志里确认结构），把确认结果写进执行台账。

---

## P2　`handleInvitations` 是唯一不写诊断日志的自动规则，导致这次问题天生无法从日志复核

### 证据

其余全部自动规则的成功/失败/跳过都会调用 `r.record(...)` 写一条 `event:"watch_action"` 诊断，例如：
```
backend/watch_rules.go:656   r.record(map[string]any{"event": "watch_action", "action": action, "result": "armed"})
backend/watch_rules.go:741   r.record(map[string]any{"event": "watch_action", "action": action, "result": "fired"})
backend/watch_rules.go:762   r.record(map[string]any{"event": "watch_action", "action": "accept", "result": "skipped_custom"})
backend/watch_rules.go:906   r.record(map[string]any{"event": "watch_action", "action": "auto-matchmaking", "result": "armed"})
backend/watch_rules.go:1328  r.record(map[string]any{"event": "watch_action", "action": "position-broadcast", "result": result, "reason": reason})
```
本次日志里也确实能看到 `accept`/`auto-matchmaking`/`play-again`/`position-broadcast`/`promote-leader`/`reconnect` 六种 `watch_action` 事件（`jq 'select(.event=="watch_action")|.action'` 统计各有 2–15 条不等）。

唯一的例外是 `handleInvitations`（`backend/watch_rules.go:1283-1319`）：三个分支（GET 失败、POST 成功、POST 失败）分别只调用了 `r.emit("watch:failed:invitations")` / `r.emit("watch:fired:invitations")`——`emit`（`backend/watch_rules.go:1080`）只转发给界面 toast 提示，**不会**调用 `record`（`:1048`），不会进诊断日志。**跳过（没有匹配到 accept/decline 策略）的分支甚至连 `emit` 都没有**（`:1304` 的 `continue` 前后都没有任何上报）。

结果是：本次日志里搜不到任何 `/lol-lobby/v2/received-invitations` 的 GET/POST 记录（见 P3），也搜不到任何一条与「invitations」相关的 `watch_action`——两者叠加，让「这条规则这次到底有没有跑起来、跑起来之后是命中了策略、被策略过滤掉、还是请求失败」这四种可能性，在日志里完全无法区分。

### 根因

`handleInvitations` 从写下第一天起就没有接入项目里所有其他自动规则统一使用的诊断上报模式，是这一条规则相对其余七条的功能缺口，不是这次新出现的回归。

### 修复方向

比照 `schedule`（`:656-741`）、`handleLobby` 里 auto-matchmaking/promote-leader（`:762-991`）、`broadcastPositionContext`（`:1328`）的既有写法，给 `handleInvitations` 的每个分支补 `r.record(map[string]any{"event": "watch_action", "action": "invitations", ...})`：

- GET 失败（`:1291`）：`result:"failed"`，`reason:"list-read-failed"`。
- 每条邀请因 `safeLCUIdentifier`/`markRun` 被跳过（`:1297`）：不需要单独上报（属于去重/防抖，不是策略决策），除非后续发现有诊断价值。
- 命中策略前先解析出队列 ID 和最终 `policy`；`policy` 为空导致 `continue`（`:1305`）：`result:"skipped_no_policy"`，带 `queue_id`（数字即可，不含召唤师信息）。
- POST 成功（对应现有 `emit("watch:fired:invitations")`，`:1318`）：`result:"fired"`，带 `queue_id`、`policy`（accept/decline）。
- POST 失败（对应现有 `emit("watch:failed:invitations")`，`:1315`）：`result:"failed"`，带 `queue_id`、`policy`。
- `customPaused` 提前返回（`:1284`、`:1307`）：`result:"skipped_custom"`，与 `accept` 规则 `:762` 的既有写法一致。

`emit` 调用全部保留（界面提示不能丢），`record` 是新增的，两者并存。

### 验收

- 新增测试：仿照 `TestWatchReadFailuresBroadcastAndDoNotWrite`（`backend/r50_suite_test.go`）的风格，构造「GET 失败」「命中 accept」「命中 decline」「策略未配置」「POST 失败」「custom-paused」六种场景，断言每种都产生对应 `result` 值的 `watch_action` 诊断事件。
- 对抗变异：把新增的 `r.record` 调用去掉，只保留 `r.emit`，上面的断言必须 FAIL。
- 真机验收：GPT 触发一次真实或模拟的邀请处理，确认诊断日志里能看到对应 `watch_action` 事件，且字段不包含召唤师名称等可识别信息。

---

## P3　LCU 请求诊断按窗口分桶、只在断线时落盘，导致长连接会话里的孤立请求在导出日志中「不存在」

### 证据

`backend/lcu.go:93`：
```go
const lcuRequestDiagnosticWindow = 10 * time.Second
```
`recordRequestDiagnostic`（`:194-224`）把同一 `方法+路径` 的请求按 10 秒窗口分桶（`windowStart := now.Truncate(lcuRequestDiagnosticWindow)`），**只有当同一路径在更晚的窗口里再次被请求时**，才会把上一个窗口的桶转成 `lcu_request` 事件写出去（`:207-211` 的 `if bucket != nil && !bucket.windowStart.Equal(windowStart) { completed = ...; bucket = nil }`）。唯一的另一条落盘路径是 `flushRequestDiagnostics`（`:229-246`），而它只在 `Close()`（`:880-886`）里被调用一次。

本次日志里，LCU 连接从 05:36:36Z 建立后，`lcu_discovery`/`lcu_event_stream` 再没有出现第二次「connected」，一直持续到日志末尾（06:27:29Z）都没有断开迹象——即整段 51 分钟连接从未 `Close()`。同一份日志里能看到的所有 `lcu_request` 路径（`jq -r 'select(.event=="lcu_request')|.path' | sort -u`，35 种）全部是被反复请求过多次、跨过窗口边界的高频路径（`/lol-champ-select/...`、`/lol-summoner/...` 等）；完全没有 `/lol-lobby/v2/received-invitations` 或它的 accept/decline 子路径——但这**既可能是这条请求从未发生过，也可能是它发生过恰好一次、后面没有第二次同路径请求把桶推走，连接又一直没断，桶就一直挂在内存里，导出时自然什么都看不到**。

### 根因

诊断分桶的落盘时机设计成「窗口滚动」或「连接关闭」两种，唯独没有「定时兜底刷新」。对高频路径（几乎每次英雄选择都会打好几次的那些）这个设计没有问题，天然会被后续请求推着落盘；但对像「收到邀请」这种低频、不规律触发的路径，一旦这一整份日志覆盖期间只发生了一次（或几次但都在孤立的窗口里、后面再没有同路径请求），这条记录就会在连接持续在线期间永远拿不到落盘的机会——而用户通常正是在「还开着客户端、还没退出」的时候导出诊断日志的，这种情况下一次性事件被这套机制系统性地漏掉。

这不只是影响这次的邀请排查：`/lol-honor/v1/ballot`、`/lol-matchmaking/v1/ready-check/accept`、`/lol-lobby/v2/play-again` 这类同样低频的路径，本次日志里能看到是因为它们在这局对局周期里确实被命中了不止一次；换一份「全程只点赞过一次」的日志，同样会有这个盲区。

### 修复方向

给 `LCUClient` 加一个后台定时器（例如每 30 秒一次，不需要很密），扫描 `requestDiagnostics` 里「窗口已经结束但还没被推走」的桶（`now.Sub(bucket.windowStart) > lcuRequestDiagnosticWindow` 且尚有 `samples`），主动落盘并清空——不用等下一次同路径请求或连接关闭。定时器需要跟随连接生命周期启动/停止（借用 `runConnectedSession` 现有的 `sessionCtx`），避免连接关闭后残留 goroutine。

### 验收

- 新增测试（假时钟）：构造「某路径只被请求一次，之后经过一个窗口以上的时间、既没有同路径新请求也没有 `Close()`」的场景，断言仍然能收到一条 `lcu_request` 诊断事件。
- 对抗变异：去掉新增的定时兜底刷新，上面的断言必须 FAIL；同时确认现有「窗口滚动即时落盘」「`Close()` 时落盘」两条既有路径的测试不受影响、继续通过。
- 真机验收：不强制要求（这条改动主要影响诊断可见性，行为上不改变任何实际请求/写入），跑一次全量测试确认没有引入 goroutine 泄漏或竞态（`go test -race ./backend/...` 如果条件允许）。

---

---

## P4　海克斯大乱斗不是极地大乱斗，但邀请策略界面里没有它的位置

### 证据

`backend/queue_groups.go`：
```go
{450, "极地大乱斗", "aram", "more:aram", "aram", 0},                          // :106
...
{2300, "海克斯大乱斗", "hextech-aram", "hextech-aram", "hextech-aram", 0},   // :120
{2400, "海克斯大乱斗", "hextech-aram", "hextech-aram", "hextech-aram", 0},   // :121
{3270, "海克斯大乱斗", "hextech-aram", "hextech-aram", "hextech-aram", 0},   // :124
```
这是本仓库自己对 LCU 队列目录的权威登记：**极地大乱斗（450）和海克斯大乱斗（2300/2400/3270）是三个不同的 `queueId`，只是同属 `aram`/`hextech-aram` 这一大类玩法**，不是同一个队列的两种叫法。项目里其余引用 `hextech-aram` 的地方（`champions_structured.go:167`、`gameplay.go:1721` 注释「三个 ID 的中文名都是『海克斯大乱斗』」、`queue_groups.go` 本身）都在反复确认这一点。

而 `backend/web/suite.js:309-312` 的邀请策略快捷开关列表：
```js
const queuePolicies = [
  ["420", "单双排"], ["440", "灵活组排"], ["450", "极地大乱斗"],
  ["400", "匹配征召"], ["430", "匹配自选"], ["490", "快速模式"],
  ["1700", "斗魂竞技场"], ["1090", "云顶普通"], ["default", "其他队列"],
];
```
里面没有 `2300`/`2400`/`3270` 中的任何一个。用户想单独为海克斯大乱斗配置「自动接受」，界面上根本没有对应的开关，只能通过「其他队列」（`default`）兜底——但那是一个不区分具体队列的总开关，一旦打开会连带自定义房间邀请、`400`/`430`/`490`/`1090` 等所有未单独列出的队列邀请一起自动处理，用户很可能并不想要这种「全都接受」的粒度。

### 根因

`queuePolicies` 这份快捷列表是手写的固定队列集合，创建时没有覆盖海克斯大乱斗（可能是海克斯大乱斗上线/接入本项目的时间晚于这份列表最初编写的时间，没有跟着补）。结果是一个相当常见、用户明确会用到的模式，在「按队列类型分别设定」这个功能里没有专属入口。

### 修复方向

在 `queuePolicies` 数组里补上海克斯大乱斗一项。由于它对应三个 `queueId`（2300/2400/3270，`backend/queue_groups.go:120,121,124`），建议参照后端 `hextech-aram` 分组的做法，让前端这一项在存储上仍然写成三个独立的 `policies` 键（`"2300"`/`"2400"`/`"3270"` 三个都设成同一个策略值），但在界面上合并显示成一个「海克斯大乱斗」开关，点击时同时切换三个键——不要求这三个 queueId 里的判断从此在后端也合并成一组（那属于 P1 范围之外的重构，本单不做）。前端已有 `season_stats.go:55`、`gameplay.go:1721` 里「hextech-aram 组三个 ID 合并成一个 UI 项」的先例，照抄思路即可。

### 验收

- 界面核对：邀请策略设置里能看到「海克斯大乱斗」独立开关，切换后三个 queueId 对应的 `policies` 值同步变化（可以直接看 `watch_settings_saved` 诊断确认）。
- 新增测试：切换该开关后校验 `state.watch.rules.invitations.policies` 里 `"2300"`/`"2400"`/`"3270"` 三个键同时被设置为同一个值。
- 对抗变异：把「三个键同步」的逻辑去掉、只改其中一个键，断言必须 FAIL。

---

## 验收总表

| 项 | 判据 |
|---|---|
| P1 | 邀请队列 ID 优先从嵌套 `gameConfig` 读取、顶层兜底；新增一次性 shape 诊断；两种数据结构的测试用例及对抗变异 |
| P2 | `handleInvitations` 每个分支都产生对应 `watch_action` 诊断，字段不含可识别信息；六种场景测试及对抗变异 |
| P3 | 长连接会话中的孤立单次请求也能落盘成 `lcu_request` 诊断；假时钟测试及对抗变异；既有分桶/断线落盘行为不回退 |
| P4 | 邀请策略界面新增海克斯大乱斗独立开关，三个 queueId 同步；测试及对抗变异 |
| 全量 | `go build -o <tmp> ./backend`、`go vet ./...`、`go test ./...`（有条件的话 `-race`）、`node --test backend/web/*.test.cjs desktop/*.test.cjs`，与 R154/R155 基线一致或更好（既有失败不得增加）；改动后版本号递增（当前 0.12.20），更新 `docs/WORKLIST-INDEX.md` |

## 结论

好友的邀请如果是海克斯大乱斗，这次「配置了却没自动接受」基本就是 P4（界面根本没给这个队列开关，用户没法配置它）；配完 P4 之后，只要该邀请的真实 JSON 结构确实是嵌套的 `gameConfig`（P1 待验证），P1 的取值修复也得同时到位，否则新配出来的海克斯大乱斗策略一样会被取错的队列 ID 挡住。这两处加上 P2 的可诊断性缺口，是这次问题最合理的完整解释，不再需要额外向用户确认队列类型。
