# WORKLIST-R136：韩服战绩读不出 / R116 探测结果无效 / 账户与物品闪烁 / 对局页签空隙 / 海斗表现字号

诊断人：Claude（只读：两份用户上传日志 + 截图 + 读代码，未改仓库代码）。
执行人：GPT。
日期：2026-09-23。
基线：0.12.18，HEAD `6558fe85`（R135 之后）。
状态：待执行。本单合并用户一次提出的全部问题：P1–P2 是缺陷与探测失效，P3–P5 是界面问题，P6 是 R130 真机压测结果回填。

证据文件（用户上传，本单引用的都是其中的原始字段）：
- `augment-probe.jsonl`：R116 判据一探测输出（`trace_id=d1f33afcde302756baefdc5e`，2026-09-23 03:35:18Z，Windows 真机 + 国服客户端）。
- `lol-loot-diagnostics-0923-1137.jsonl`：0.12.18 安装包的诊断日志，4,849 条，单次运行 `run_id=0514595efe88fc36a008f2c4`，`build_fingerprint=1de5cc3d1e8c`，03:00:14Z–03:37:06Z。
执行时请把这两份文件放到 `docs/r136-validation/`，账本引用这个路径。

## 执行纪律（延续 R117–R135）

- 每项验收判据都要做**对抗变异**：改回原样或换一种等效写法，对应测试必须 FAIL。
- 变异在隔离副本里做，还原用「整文件从基线拷回 + `diff -q`」，**禁止 `git checkout -- <file>`**。
- 界面文案红线（CLAUDE.md，R128 §2.5）：不加口径、方法论、免责说明。P1 改的是错误提示本身，属于允许范围。P5 **不删**表现面板底部那句多杀累计说明，也不删右上角「对比 N 位英雄」：R128 §2.2 写明这两处是用户指定保留的。
- 全部改完后版本号递增到 0.12.19，**用 private 模式**（默认，读 `riot_key.local.txt`）打包，把安装包交给用户，见 §7。

---

## P1　韩服战绩读取失败：用户装的是 R135 验证时打的无 Key 包

### 证据

- 截图一：韩服玩家页「战绩读取失败」，文案是 `backend/riot_api.go:420` 的原句「尚未配置 Riot API Key：请用 -encrypt-riot-key 生成密文，构建时通过 -ldflags … 注入」。
- 日志两条 `app_start`（log_seq 1、4061）都是 `"riot_key": false`。
- 同一运行里 13 条 `riot_overview_cost` 全是 `matches_requested=0`、`first_error_kind=""`：请求在账号查询这一步就返回了，诊断事件没有记录原因。
- `docs/r135-execution-ledger.md` 最后一节：R135 验收时执行的是 `DEEP_LEGENDS_KEY_MODE=public … bash build-desktop.sh 0.12.18`，产物是 `dist/desktop/Deep Legends Setup 0.12.18.exe`。用户安装目录 `D:\Download\Deep Legends` 里的文件时间是 2026-09-23 10:50（本地），和这次打包对得上。

### 根因

构建脚本本身没问题：`build-desktop.sh:22` 默认 `private`，public 必须显式指定。问题出在三处：

1. public 包和 private 包**文件名完全一样**，都叫 `Deep Legends Setup 0.12.18.exe`，放在同一个 `dist/desktop/` 目录。验证用的 public 包留在那里，用户拿去装了，没人察觉。
2. 缺 Key 时 UI 直接显示开发者指令（`-ldflags`、`-X main.riotAPIKeyCipher`），用户看不懂也没法自己处理。
3. `riot_overview_cost` 只统计单场对局失败（`riotOverviewCostTracker.matchFailureSnapshot`），账号查询阶段的失败不计入，日志里看起来是「什么都没请求、也没出错」。

### 修复方向

1. **区分 public 产物**：`DEEP_LEGENDS_KEY_MODE=public` 时，安装包、zip 和 `SHA256SUMS.txt` 的文件名一律带 `-public` 后缀（例如 `Deep Legends Setup 0.12.19-public.exe`）。`release-build.json` 本来就记录了 mode（`desktop/release-build.cjs:12`），确认它会写进 release receipt。private 产物文件名不变。
2. **缺 Key 的错误文案改成用户能看懂的一句**，例如「此安装包未内置 Riot API Key，韩服战绩暂不可用。」。开发者指令移到 `riot_api.go` 的注释里。「重试」按钮保留。
3. **诊断补上原因**：账号查询因缺 Key 失败时，`riot_overview_cost` 带上 `error_kind: "riot_key_missing"`（新增字段，或者让 `first_error_kind` 覆盖到账号查询阶段，二选一，写进账本）。其他账号查询失败按现有 `championProviderErrorKind` 归类。
4. 以后凡是验证性打包，账本里必须写明 key mode；public 产物验证完从 `dist/` 删掉，或者只保留带 `-public` 后缀的文件名。这条写进 AGENTS.md / CLAUDE.md 的构建说明。

### 验收

- `desktop/release-quality-gates.test.cjs` 已经有一条 public 模式用例，在它上面补断言：产物文件名含 `-public`，private 模式不含。变异：去掉后缀逻辑，测试 FAIL。
- Go 测试：`riotKeyConfigured()` 为 false 时，账号查询返回的错误文本不含 `ldflags`、`encrypt-riot-key`、`riotAPIKeyCipher`，`riot_overview_cost` 带 `riot_key_missing`。变异：还原旧文案，或者去掉 error_kind，测试 FAIL。
- 用 private 模式打出 0.12.19，启动后 `app_start.riot_key == true`，韩服玩家页能出战绩（§7 由用户确认）。

---

## P2　R116 海斗探测：三项判据这一轮一项都不能回填

**`docs/r116-probe-findings.md` 这一轮一个字都不要改**，下面三项都不是有效观测。

### 判据一（LCU 有没有海克斯三选一端点）：`negative_conclusive=true` 是假阴性

证据（`augment-probe.jsonl`）：openapi v3/v2 都是 404；`/help?format=Full` 返回 200、3,031,009 字节，`root_shape = {events: array:737, functions: array:1468, types: array:3578}`，但文本扫描只拿到 `paths_scanned=1`、`unique_paths=1`，然后判了 `contract_read=true`、`negative_conclusive=true`，summary 里 `negative_conclusive_all=true`。

根因：`backend/augment_contract_probe.go:361-370` 对 help-full 走的是 `augmentProbeTextPaths`，也就是在原始文本里找以 `/` 开头的路径 token，并且把 `contractRead = stats.Tokens > 0` 当成「契约已读」。可是 `/help?format=Full` 里是驼峰式函数名、事件名和类型名（上面三个数组），不是 REST 路径。**R121 在 `position_contract_probe.go`（第 21、102 行的注释）已经查明并删掉了同一个兜底，R116 这份探测没有同步。** 在 5,783 个条目里扫到 1 个 token 就判「确证没有」，是伪造结论。

修复：

1. help-full 改为**按 JSON 结构扫描**：解析根对象，遍历 `functions[]`、`events[]`、`types[]` 每个元素的 `name`，以及元素里任何名为 `url` / `path` / `uri` 的字符串字段，用原谓词 `(?i)cherry|augment|mayhem` 匹配。驼峰名 `GetLolCherry…` 和事件名 `OnJsonApiEvent_lol-cherry_…` 都能命中。
2. 事件里记录三个数组各自的长度、实际扫描的元素数，以及元素键名并集（`functions_element_keys` 等，只记键名，不记取值），这样下一轮能看清条目长什么样。
3. `contract_read` 只有在三个数组都存在、每个元素都扫到（扫描数等于数组长度）、没有触发任何上限时才为 true；`negative_conclusive` 要在此基础上再加命中数为 0。
4. 命中项全部列出（上限沿用现有常量，超出置 truncated）。`cherry-augments.json` 这类静态目录属于已知无关命中，按 findings §1.2 的规则单列。
5. 删掉 help-full 的 `augmentProbeTextPaths` 分支。若该函数不再有调用方，一并删除。

测试（对抗）：

- 夹具按真机形状构造：根对象含 `events`/`functions`/`types`，全文没有任何以 `/` 开头的路径，`functions` 里有一个 `name: "GetLolCherryV1AugmentOffer"`。结果必须 `count ≥ 1`、`negative_conclusive=false`。变异：恢复文本扫描，这条 FAIL。
- 同形状、没有任何命中名：`contract_read=true`、`negative_conclusive=true`，且扫描数等于三个数组长度之和。
- 根对象缺 `functions` 数组或触发上限：`contract_read=false`。
- `TestR116AugmentContractProbeMutation` 等既有用例保持通过。

### 判据二（海斗 playerlist 有没有装备字段）：8 局全部在加载画面采样，一次都没拿到

证据：8 局 `live_client_playerlist_shape` 全是 `attempt=1`、`http_status=0`、`result=unavailable`；8 条 `live_client_allgamedata_shape` 全是 `http_status=0`、`success=false`、`top_level_keys=[]`；每条的时间戳和该局 gameflow 进入 `InProgress` 的时间相同（例如 03:04:26.2085589Z）。

根因：

- `backend/gameplay.go:7514-7517` 在海斗首次出现 `InProgress` 时调用 `go a.sampleArenaAllGameData(...)`。`arena_truth_diagnostics.go:307-309` 用 `claimBoundedDiagnosticKey` 让**每局只采样一次**。gameflow 刚进 `InProgress` 时游戏还在加载画面，2999 端口没起来，这唯一一次必然失败，之后不再重试。
- playerlist 探测 `liveClientSnapshotForGame`（`gameplay.go:5968-6025`）虽然有 3 秒间隔，但只在对局页刷新时才会被调用。对局中前端不再自动刷新，所以每局也只有 attempt=1。
- `http_status=0` 分不出是连接被拒还是超时，日志里没有区分。

修复（只动诊断，不新增用户可见功能）：

1. 海斗（`aramMode && !arenaMode`）进入 `InProgress`/`Reconnect` 后，启动一个**有界的后台采样器**，不依赖前端刷新：每 10 秒请求一次 allgamedata 和 playerlist，直到两者各拿到一次 HTTP 200 且 JSON 合法，或者超过 5 分钟、或者 gameflow 离开对局为止。成功后各记录一次 shape 事件（只记键名和类型）。同一局最多一个采样器，并发安全。
2. 失败的尝试只记汇总：尝试次数、最后一次的 `error_kind`（connection-refused / timeout / http-非200 / invalid-json）、首次成功相对 InProgress 的 `elapsed_ms`。不要每次失败都写一条事件。
3. 采样器结束时写一条 `live_client_mayhem_sample_summary`，内容包括 `game_id`、两项是否成功、尝试次数、结束原因。

测试（假时钟 + 假 2999）：

- 前 3 次连接被拒、第 4 次成功：只有一条成功 shape 事件，`attempt=4`，采样器随即停止。
- 一直失败：5 分钟后停止，summary 为 `timeout`，总尝试次数有上限。
- gameflow 提前离开对局：采样器立即停止。
- 变异：恢复「每局只采一次」，第一条用例 FAIL。

### 判据三（海斗选人能否看到对方阵容）：这一轮是自定义单人局，无法判断

证据：两次 `lcu_champ_select_session_shape`（log_seq 753、4127）都是 `queue_id=3270`、`my_team_length=1`、`their_team_length=0`；对局中 `lcu_gameflow_session_shape` 是 `team_one_length=1`、`team_two_length=0`；`watch_custom_classification` 是 `custom=true, queue_id=3270`，8 次 `watch_session result=paused_custom`。也就是说这 8 局都是自定义房间、只有用户一个人，看不到对方阵容是因为根本没有对手，不能当作证据。

这一项不改代码。账本注明「本轮样本为自定义单人局，判据三不可判」；下一轮必须是**匹配进入的 5v5 海斗**（见 §7）。

### 交付：免装 Go 的探测程序

用户的 Windows 电脑上只有一份手工拷过去的源码，而且要换 `GOPROXY` 才能下依赖。改完后请交叉编译成独立 exe：

```bash
GOOS=windows GOARCH=amd64 go test -c -o dist/probes/r116-augment-probe.exe ./backend
GOOS=windows GOARCH=amd64 go test -c -o dist/probes/r121-position-probe.exe ./backend   # 同一个测试二进制，也可以只出一个
```

再写一份 `dist/probes/README.txt`（中文，面向用户），给出 PowerShell 里的完整命令：`$env:R116_AUGMENT_PROBE="1"`、`$env:R116_AUGMENT_PROBE_OUTPUT="augment-probe.jsonl"`、`.\r116-augment-probe.exe -test.run TestR116AugmentContractProbeLiveClient -test.v`；R121 同理，要写明两个阶段（大厅已开始匹配、英雄选择中）各跑一次。exe 不进 Git。

---

## P3　收藏「账户与物品」首次进入会闪一下

### 证据

日志 03:09:53–03:10:50：用户进入收藏页，前端 `POST /api/collection`（202）触发全量刷新，1.8 秒后 `refresh_succeeded`。紧接着 03:09:56.236 又完整读了一遍战利品（`loot_metadata_source` ×4、`loot_map_shape` 再次出现），这是刷新完成、`status.lastSync` 变化后 `loadAccount()` 被再次调用。之后 03:10:00、03:10:17、03:10:48 又有三次刷新，每次都会走同一路径。

### 根因（`backend/web/app.js`）

- `loadAccount()`（1951 行起）**每次调用都会先把整个面板换成骨架屏**（1962 行 `el.accountContent.innerHTML = '<div class="account-loading">…'`），拿到数据后再整块重写 `innerHTML`。
- 首次进入会调用一次（`activateFavoritesPage`，2418 行）。进入收藏页触发的刷新完成后，`refreshStatus` 里 `changed` 为真（457、476 行），在账户页时又调用一次。另外 `account` 实时切片（3478、3592 行）也会触发。
- 所以首次进入的实际画面是：骨架 → 内容 → 约 2 秒后又一次骨架 → 内容。整块重写还会让所有战利品图片（`loadNextLootImage` 链，2911 行）和顶部召唤师背景图重新加载，图片先消失再出现。
- 数据其实没有变：4 次刷新的 catalog 指纹、战利品数量（`kept: 42`）完全一致。

### 修复方向

1. **骨架只在面板还没有内容时出现**（首次加载，或断线后 `state.account` 被清空）。后台重载（状态变化、account 切片、claim 变化）保留现有 DOM，拿到数据后再处理。
2. **标记比较**：把拼好的 HTML 存到 `el.accountContent._accountMarkup`。新旧一致就**完全不写 DOM**（照 R129 `_recommendationMarkup` 的做法）；不一致才替换，替换时也不经过骨架。
3. **防乱序**：给 `loadAccount` 加请求序号，较早请求晚返回时直接丢弃。
4. 同样检查「三合一奖池」页（`loadPools`，476 行的同一路径）有没有先清空再重建的问题，有就一并按上述方式处理，并写进账本；没有也写明。

### 验收（jsdom，写进新的 `backend/web/r136.test.cjs`）

- 首次加载后，用**相同的 payload** 触发一次后台重载：`MutationObserver` 全程不能记录到 `.account-loading` 节点插入；重载前后战利品 `<img>` 是同一批对象（引用相等）。
- payload 变了（例如某个战利品数量 +1）：面板更新，期间同样不出现骨架。
- 两次重载乱序返回：最终渲染的是较新一次的数据。
- 变异：恢复「每次先放骨架」，第一条 FAIL；去掉标记比较，图片引用相等断言 FAIL。
- 手动：真机首次点进「账户与物品」，除第一次骨架外不再闪。

---

## P4　对局页「海克斯与出装 / 详情」页签和上方空隙过大（所有模式）

### 证据与根因

截图二：「英雄选择 · 海克斯大乱斗」概要卡和页签之间有一大段空白。这段空白由三部分叠加，大约 66px：

| 来源 | 位置 | 大小 |
|---|---|---|
| `.live-toolbar { margin-bottom: 10px }` | `gameplay.css:989` | 10px |
| `#live-content` 使用的 `.gameplay-content { padding-top: 16px }` | `gameplay.css:30`；`index.html:179` | 16px |
| **空的** `<div data-live-status>` 固定占位 `min-height: 40px` | `gameplay.css:1854`；`gameplay.js:5223` | 40px |

第三项是 R95 为了「状态条出现时内容不跳动」加的常驻占位（`docs/r95-execution-ledger.md:50`）。平时状态条没有内容，40px 就是纯空白。这个结构对所有模式都一样：经典 / 排位（符文、详情、出装与技能）、极地大乱斗、海斗、斗魂（前面多一条 `.arena-my-squad-notice`，之后 `.recommendation-area` 不是 first-child，还会再加 `margin-top: 14px`）、等待进入对局、不支持的模式、读取失败、正在识别新对局。

### 修复方向

1. **状态条挪进工具栏**：`renderLiveRefreshStatus` 的内容渲染进 `.live-toolbar`，放在对局概要和「刷新对局」按钮之间的一个槽位。内容区不再有 `[data-live-status]` 占位。这样状态出现或消失都不影响下方内容的位置（R95 的目标保留），也继续满足 R129 规则：状态不参与 `_recommendationMarkup` 比较。状态条自带的「刷新」按钮和工具栏已有的「刷新对局」按钮重复，只留一个。
2. `#live-content` 的 `padding-top` 改为 0，只对对局页生效，总览页共用的 `.gameplay-content` 不动。
3. 斗魂那条提示与页签之间按同样的间距处理，不再叠加 14px。
4. 目标：**工具栏下边缘到第一个内容块（页签行 / 数据提示 / 斗魂提示 / 空状态卡）的距离，所有模式都在 10–12px**。

### 验收

- 真 Chromium 布局脚本 `desktop/r136-live-gap-layout.cjs`，照 `desktop/current-game-layout.cjs` 的写法：在 1200px 和 960px 两个宽度下，对上面列出的每种模式和状态各渲染一次，量工具栏底部到第一个内容块顶部的距离，全部在 10–12px；另外量「有状态消息」和「无状态消息」两种情况下页签行顶部的 y 坐标，必须相同（不跳动）。输出截图到 `docs/r136-validation/live-gap/`。
- 静态测试：`gameplay.css` 里不再有 `[data-live-status] { min-height`；`#live-content` 里不再出现 `data-live-status`。
- 变异：恢复 40px 占位，距离断言 FAIL；把状态放回内容区，不跳动断言 FAIL。
- 既有测试里引用 `data-live-status` 的是 `backend/web/r129.test.cjs` 和 `backend/web/r91.test.cjs`，按新位置更新（断言的行为不能放宽），账本逐条列出改了哪些。

---

## P5　英雄页海斗详情「表现指标」字号太小

### 证据与根因

截图三。`backend/web/champions.css:946-959` 这个面板的字号是：分组标题 11px、指标名 10px、数值 12px、「较平均」9px、多杀小标题 9px、多杀数值 10px、底部说明 9px。DESIGN.md「Typography」规定固定字号层级只有 **12 / 13 / 14 / 16 / 20 / 28px**，这里有 5 处低于最小档。

### 修复方向（只改字号和随之需要的行高、间距，不改文字内容和结构）

| 元素 | 现在 | 改为 |
|---|---|---|
| `.mayhem-performance-group h4`（分组标题） | 11px | 13px |
| `dt`（指标名） | 10px | 13px |
| `dd`（数值） | 12px | 14px，保持 800 字重、tabular-nums |
| `.mayhem-delta`（较平均） | 9px | 12px |
| `.mayhem-multikill-values small` | 9px | 12px |
| `.mayhem-multikill-values b` | 10px | 13px |
| `.mayhem-performance-note`（底部说明，**保留文字**） | 9px | 12px |

行最小高度按新字号调整（例如 36px → 40px）。窄宽度（`champions.css:968` 的单列断点）下，多杀四个七位数必须完整显示，不能出现省略号；放不下就在该断点改成两列（已有 `repeat(2, …)`，确认生效）。

### 验收

- 在 `r128.test.cjs`（或新建 `r136.test.cjs`）加 CSS 断言：以上选择器的 `font-size` 都属于 {12, 13, 14, 16, 20, 28}px。变异：把任意一处改回 9/10/11px，测试 FAIL。
- `r116b.test.cjs:704-706`、`r128.test.cjs:131-138` 对说明文字「逐字一致」的断言保持通过（文字不变）。
- 真 Chromium 截图：1200px 和 960px 各一张，多杀数值没有省略号。

---

## P6　R130 真机 5 分钟压测：已跑，没有 stall（回填记录，不改代码）

证据：整份日志（03:00–03:37Z，包含 03:09–03:10 的收藏刷新和 03:18–03:20 的头像旗帜页）里：
- `card_image_stalled`：**0 条**；
- `local_request_client` `reason=failed`（任何 endpoint，包括 image）：**0 条**；
- `client_diagnostic_rejected`：0 条（前端诊断通道正常，R127 修复有效，所以「0 条 stall」是可信的，不是上报被拒）。

局限：日志里没有页面导航事件，只能从日志确认「这段时间没有任何卡片卡住或图片失败」，压测时长和操作以用户口述为准。

执行：
1. 在 `docs/history/ledgers/r130-execution-ledger.md` §8「未验证 / 边界」第 1 条补一句：「2026-09-23 用户在 Windows 真机按 R133 P2 步骤压测 5 分钟，诊断日志 `card_image_stalled`=0、图片请求失败=0，见 `docs/r136-validation/lol-loot-diagnostics-0923-1137.jsonl`。」
2. R133 P2 标记完成（写进 `docs/r133-execution-ledger.md`）。R133 如果因此所有条目都已完成，按 R135 规则归档，并更新 `docs/WORKLIST-INDEX.md`。

---

## 7. 发版与需要用户做的事

GPT 完成 P1–P6 后：

1. 版本号 0.12.19，**private 模式**打包（不要设置 `DEEP_LEGENDS_KEY_MODE=public`），确认 `dist/desktop/Deep Legends Setup 0.12.19.exe` 文件名不带 `-public`，账本记录 key mode、指纹和 SHA256。
2. 连同 `dist/probes/` 里的探测 exe 和 README 一起交给用户。

用户在真机上做（写进 README，也写进账本「待真机」栏）：

- 装 0.12.19，打开任意韩服玩家页，确认能出战绩；首次进入「账户与物品」不闪；看一下对局页页签上方的空隙和海斗「表现」字号。
- **匹配进入一局 5v5 海克斯大乱斗（不是自定义房间）**，进游戏后至少待 2 分钟（让 P2 的采样器拿到 2999 数据），结束后导出诊断日志。
- 同一局的英雄选择阶段，用 `r116-augment-probe.exe` 跑一次判据一；R121 位置探测按 README 在大厅和英雄选择两个阶段各跑一次。
- 把诊断日志和探测输出发回来，由 Claude 读取后回填 `docs/r116-probe-findings.md` 和 `docs/r121-position-probe-findings.md`。

## 验收总表

| 项 | 判据 |
|---|---|
| P1 | public 产物带后缀；缺 Key 文案无开发者指令；`riot_key_missing` 进诊断；private 0.12.19 的 `app_start.riot_key=true` |
| P2 | help-full 结构化扫描 + 4 条用例及变异；海斗有界采样器 + 3 条用例及变异；探测 exe 与 README 交付；findings 文档本轮不改 |
| P3 | 后台重载无骨架、图片节点不变、防乱序，变异 FAIL |
| P4 | 所有模式间距 10–12px、有无状态消息时页签位置不变，Chromium 截图 |
| P5 | 字号全部落在 DESIGN.md 层级，说明文字不变，窄屏多杀不截断 |
| P6 | r130 / r133 账本回填，索引更新 |
| 全量 | `go build -o <tmp> ./backend`、`go vet ./...`、`go test ./...`、installer 测试、`node --test backend/web/*.test.cjs desktop/*.test.cjs`、r100/r117 Chromium 护栏全部与 R135 基线一致或更好（R135 记录的 5 项既有失败不得增加） |
