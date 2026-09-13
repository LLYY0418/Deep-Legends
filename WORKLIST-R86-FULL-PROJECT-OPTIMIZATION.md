# WORKLIST-R86 全项目优化（性能 / 体验 / 清理 / 流程）

本轮由 Claude 做了一次全仓深度遍历（后端数据链路、前端渲染、LCU 状态机、样式适配、存储与安全、死代码、测试与构建六个维度并行审查），
所有条目的 `file:line` 都经过**二次人工核实**（不是从历史工单或记忆里抄的），核实时的实测命令与输出见每条的"证据"。

## 执行须知（请逐条遵守）

1. **每条都要留护栏**。判据里写"可自动化"的，必须落成真实测试；落不成的要在报告里说明为什么。
2. **禁止用"加死代码 / 改内部逻辑"制造推进假象**。本项目历史上出现过静态护栏被"新增一条旁路函数"绕过的事故，
   所以凡是新增护栏，都要自己做一次**"新增一条独立路径"型变异**，确认护栏仍能杀掉。
3. **不要动已确认无问题的地方**。附录 A 列了本轮查过、确认已经做对的点，别在那里浪费时间，也别把它们"顺手重构"掉。
4. 本轮条目按 P0 → P1 → P2 排序，**可以在任意一档停下**。P0 全部是低风险或纯流程性修复；
   标了 `⚠️高风险` 的条目请先单独确认再动手，不要和别的条目混在同一次提交里。
5. 基线：本次审查时 `go test ./...` = 1414 pass / 0 fail / 14 skip（66.1s），`gofmt -l .` 与 `go vet ./...` 干净，
   `GOOS=windows go vet ./...`（根模块与 installer 模块）干净，`node --test web/*.test.cjs desktop/*.test.cjs` = 567 pass / **1 fail**（见 P0-5）。
   动手前先自己复现这个基线，否则后面所有"我改完是绿的"都不成立。

---

# P0 — 先做这些

## P0-1 git 账本和工作区严重脱节（最高优先级，不修会放大后面所有改动的风险）

**证据**（本次实测）：
```
untracked: 225   deleted-but-tracked: 260   modified: 86
git ls-files | grep -Ei 'deep-legends.exe|lol-loot-assistant-go-tmp'
  deep-legends.exe
  lol-loot-assistant-go-tmp-umask
```
`git log` 只有 5 个提交，最新一条是 9 月 1 日。而磁盘上**几乎全部当前生产代码**（`champselect.go`、`update.go`、`pro_players.go`、
全部 `desktop/*.cjs` 测试、`docs/` 下大部分文档，共 225 个文件）**从未被 git 跟踪过**；同时 260 个早已删除的旧文件仍记在 HEAD 索引里。

**影响**：现在任何一次 `git checkout` / `git stash` / 回滚都可能一次性抹掉大量未跟踪的生产代码，且无法恢复。
本轮后面所有改动都建立在这个不稳的地基上。

**改法**：
1. 先 `git status --porcelain > /tmp/r86-git-baseline.txt` 存档一份现状。
2. `git rm --cached deep-legends.exe lol-loot-assistant-go-tmp-umask`（文件已不在工作区，只是索引里还记着）。
3. `git add -A && git commit`，把 225 个新增、260 个删除、86 个修改一次性落进历史。
4. 提交后立刻 `go build ./... && go test ./...` 确认新纳入的文件能编译通过（已预先验证可以）。

**验收判据**：`git status --porcelain` 输出为空；`git ls-files | grep -Ei '\.exe$|go-tmp'` 为空；`go test ./...` 与基线同样绿。

**风险**：低（纯版本控制操作，不改代码）。但**必须在动任何代码之前做完**。

---

## P0-2 CI 从来没有运行过一次

**证据**：
```
ls .github/workflows/ → ci.yml（2238 字节，确实存在于磁盘）
git check-ignore -v .github/workflows/ci.yml → .gitignore:41:.github/workflows/
git ls-files .github | wc -l → 0
git log → f35d2b1 "Drop CI workflow from initial push (token lacks workflow scope)"
```
磁盘上那份 `ci.yml` 设计得其实不错（gofmt 检查、`go vet`、`go test -race`、JS 语法检查 + jsdom 测试、内嵌奖池检查，
外加一个 Windows 构建 + 校验和作业），但因为被 `.gitignore` 第 41 行排除，每次推送都会被静默丢掉，GitHub Actions 里从未有过一次运行记录。

**影响**：P0-3、P0-4、P0-5 三条的根因都是这个——所有测试门禁都只是"意图"，从未被强制执行过。

**改法**：
1. 从 `.gitignore` 删掉 `.github/workflows/` 这一行。
2. 用带 `workflow` scope 的凭据推送；如果本地 token 没有该 scope，直接在 GitHub 网页 UI 里新建这个文件（网页操作不需要该 scope）。
3. 顺带修 `ci.yml` 两处：(a) 第 58 行写死的 `-Version "0.11.1"` 改成从 `desktop/package.json` 读（见 P2-5）；
   (b) 补一步 `cd installer && go test ./... && go vet ./...` —— `installer/` 是独立 module，根目录的 `go test ./...` 压根覆盖不到它。

**验收判据**：`git ls-files .github/workflows/ci.yml` 能返回路径；下一次推送后 GitHub Actions 页面出现运行记录且为绿。

**风险**：低。

---

## P0-3 Windows 发布脚本里"跑测试"是假门禁，红着也能发版

**证据** `build-desktop-windows.ps1:70-88`（本次实测原文）：
```powershell
    go test ./...
    go vet ./...
    Push-Location (Join-Path $projectRoot "installer")
    try {
        go test ./...
        if ($LASTEXITCODE -ne 0) { throw "Installer tests failed" }
        go vet ./...
        if ($LASTEXITCODE -ne 0) { throw "Installer vet failed" }
    } finally { Pop-Location }
    node --check web/app.js
    ...
    node --test $webTests
    ...
    node --test $desktopTests
```
根模块的 `go test` / `go vet` 和**全部 4 个 JS 检查**都没有 `$LASTEXITCODE` 判断，
而同一个文件里 installer 的 `go test`/`go vet`（75/77 行）、指纹校验（108）、NSIS 打包（136）、外壳构建（134/139）、
发布校验（156）**全都正确判了退出码**。第 9 行的 `$ErrorActionPreference = "Stop"` 对原生进程的退出码无效，这是 PowerShell 的经典坑。

**影响**：Go 测试、`go vet`、整个 JS/desktop 测试套件全红的情况下，这个脚本照样会一路跑完并产出安装包。
P0-5 那条红测试就是在这个缺口下活到今天的。

**改法**：在 70、71、81、82、84、85、86、88 这 8 个调用后面各补一行
`if ($LASTEXITCODE -ne 0) { throw "<具体哪一步失败>" }`，照抄同文件里 installer 那几处的写法。

**验收判据**：临时把某个 Go 测试改成 `t.Fatal`，跑脚本必须在打包之前 `throw` 退出；恢复后正常构建。**这个负面验证必须真做一遍**。

**风险**：低。

---

## P0-4 macOS 发布脚本一个测试都不跑，而 README 说发版就用它

**证据**：`grep -n "go test\|go vet" build-desktop.sh` → **零命中**。脚本从 Riot key 加密直接跳到
`go build ... -o desktop/backend/loot-service.exe .`（第 30 行）。而 `README.md:237` 明确写着 macOS 上用
`bash build-desktop.sh 0.12.1` 出正式发布构建。

**影响**：实际用来发版的那个平台反而完全没有测试门禁，比 Windows 脚本（虽然门禁是坏的）还弱一档。

**改法**：在 `go build` 之前插入 `gofmt -l .`（非空则退出）、`go test ./...`、`go vet ./...`，
以及 `cd installer && go test ./... && go vet ./...`。脚本第 2 行已有 `set -euo pipefail`，
bash 下退出码会正确传播，**不需要**像 PowerShell 那样手写判断。

**验收判据**：同 P0-3，用一个故意失败的测试验证脚本会在构建前中止。

**风险**：中（会让发布流程变慢约 70 秒；如果当前有红测试会立刻挡住发版——但这正是目的）。

---

## P0-5 JS 测试套件现在就是红的：一条陈旧断言

**证据**（本次实测）：
```
node --test web/champions.test.cjs → 231 pass / 1 fail
  not ok 312 - player-name tooltips are gated by the actual ellipsized text
grep -c 'data-tooltip-overflow="\.player-tab-name"' web/gameplay.js → 0
```
`web/champions.test.cjs:3268-3270` 遍历 `[".player-tab-name", "self", ".recent-player-name", ...]`，
要求每个字面量都在 `gameplay.js` 里出现。但 `web/gameplay.js:940` 现在写的是
`<span class="player-tab-name" data-tooltip="…" data-tooltip-overflow="self" …>`，
而 `web/app.js:3519` 的 `if (selector === "self") return next;` 证明 `"self"` 是**被正式支持且功能等价**的写法。

**影响**：这是陈旧断言，不是产品回归。但它证明 P0-2/P0-3 不是理论问题——这条红测试已经活了一段时间没人发现。

**改法**：把 `champions.test.cjs:3268` 的清单里 `".player-tab-name"` 去掉（`"self"` 已在清单里，覆盖不丢），
或在循环里把 `"self"` 接受为该选择器的等价别名。**不要**为了让测试变绿去改 `gameplay.js` 回到旧写法。

**验收判据**：`node --test web/*.test.cjs desktop/*.test.cjs` → 0 fail。

**风险**：低（仅测试）。

---

## P0-6 总览接口四段完全串行，其中两段根本不依赖战绩（战绩加载速度的最大单点）

**证据** `gameplay.go`（本次实测行号）：
```
1124  phases := overviewPhasesFromContext(ctx)
1125  queueLabels := loadQueueLabels(client)
1127  names := a.overviewChampionNames(ctx)
1129  matches, historyCapabilities, pagination := a.loadDetailedMatches(...)
1181  rankEntry := a.playerRankScore(ctx, client, playerRef, isCurrent, ...)
1312  masteryMap, masteryCapability = NewChampionMasteryAPI(client).All(playerRef)
1325  rankedSamples := a.loadRecentRankedSamples(ctx, client, reference, playerRef, ...)
```
`playerRef` 在 1120~1123 行就已定稿。`playerRankScore`（SGP RANKED 或 LCU）和 `ChampionMasteryAPI.All`
（LCU 全英雄熟练度，170 个英雄的大 JSON）**只依赖 playerRef，与 matches 毫无关系**，却排在 `loadDetailedMatches` 之后。
`loadRecentRankedSamples` 在 1379~1411 行明确写了 `_ = matchFilter; _ = pagination`，形参里的 matches 只做兜底。
真正必须串行的只有 1205 行那个 30 天窗口（`shouldLoadOverviewHistory` 在 5093~5095 行要求 `len(matches)==0`）。

**影响**：一次冷总览至少 4 个串行 RTT 段。按国服 SGP 单请求 200~600ms、LCU 熟练度 100~300ms 估，
串行约 1.5~2.0s；改成"战绩 / 段位 / 熟练度 / 排位样本"并发后可压到约 0.6~0.8s。这是用户点开总览到看见内容最直接的等待。

**改法**：在 1124 行之后起一个 `sync.WaitGroup`（或 `errgroup`），把 `playerRankScore`、
`NewChampionMasteryAPI(client).All`、`loadRecentRankedSamples` 与 `loadDetailedMatches` 并发；1205 行的 30 天窗口保留在 matches 之后。
两个必须注意的点：
- `capabilities` 现在是串行 `append`，并发后**必须改成固定槽位再按顺序合并**，否则设置页里的能力列表顺序会随机抖动。
- 1165 行那个 `DeadlineExceeded` 早退分支要挪到 `Wait()` 之后，别把已经拿到的并发结果丢掉。
- 1185 行 `applySeasonRankWinRateFallback` 依赖 ranks，要留在合并之后。

**验收判据**：用 httptest 假 SGP + 假 LCU，给每个上游注入 150ms 固定延迟，断言 `loadGameplayOverview`
总耗时 < 400ms（串行实现必然 > 600ms）；同时断言 `overview_phases_ms` 里 `ranks` / `mastery` / `detailed_matches`
三个 mark 的时间区间**存在重叠**。变异测试：把并发改回顺序调用，该测试必须红。

**风险**：中。单独一次提交，不要和别的条目混。

---

## P0-7 展开任意一场战绩 = 整个总览页 innerHTML 全量重建（前端最大单点）

**证据** `web/gameplay.js:1476-1481` 有一个列表复用闸门：
```js
const preserveMatchList = Boolean(retainedMatchList
  && container._matchListMatches === rawMatches
  && container._matchListFilter === tab.matchFilter
  && container._matchListViewRevision === Number(tab.matchViewRevision || 0));
```
但 `web/gameplay.js:3394-3405` 的展开按钮处理器**先把 `matchViewRevision` 加一**，于是第四个条件必然为假，闸门永久失效：
```js
tab.openMatches.add(id);
tab.matchViewRevision = Number(tab.matchViewRevision || 0) + 1;
rerender();
```
`rerender` → `rerenderTab`（460~466 行）→ `renderOverview` → `container.innerHTML = ...`（1507~1527 行），
重建范围是召唤师条 + 全部生涯模块 + 全部战绩 + 分页哨兵。容器还是 `aria-live="polite"`（`web/index.html:173`），
整棵子树替换会让无障碍树整体重算。

**jsdom 实测规模曲线**（jsdom 比真 Chromium 慢一个量级，数字只用于横向对比）：

| 已加载战绩 | `.match-list` 节点数 | HTML 字节 | 单次展开点击（同步） | 期间 `createElement` 次数 |
|---|---|---|---|---|
| 17 | 2,357 | 190,769 | 465 ms | — |
| 68 | 9,428 | 763,328 | 1,126~1,341 ms | — |
| 204 | 28,284 | 2,290,320 | 3,326~3,541 ms | **19,202** |

`MAX_BROWSE_MATCHES = 1000`（`gameplay.js:133`）且分页是 `IntersectionObserver` 自动触发的（568~592 行），
所以"滚到两三百场"是正常使用路径，不是极端情况。

**影响**：用户往下滚一阵加载了 200 场后，点开任意一场详情要重新序列化 2.3MB HTML、重建 2.8 万个节点、
重新创建约 4000 个 `<img>`、重新绑定全部监听。即便真机 Chromium 快 10~30 倍，也是一次 100~350ms 的主线程停顿，
并伴随整列表图片/滚动/tooltip 状态复位。

**改法**：把"展开/收起某一场"降级为局部操作——只对目标 `.match-entry` 重渲染并 `replaceWith`，**不要碰 `matchViewRevision`**；
把 `matchViewRevision` 的语义收窄为"整列表可视形态变了（筛选/排序/口径）"。
注意 `matchViewRevision` 还被 `bindMatchDetailControls`、`bindRankedQueueControls`、`damageSorts` 共用，要逐个分流。
次优方案：让 `preserveMatchList` 只比较 `rawMatches + matchFilter`，另外维护一个 `dirtyMatchIds: Set`，渲染后只替换脏条目。

**验收判据**（两条都要，缺一条都能被"换个地方全量重建"绕过）：
用 `desktop/overview-render.test.cjs:83-131` 的 `bootDemoApp` 起真实页面注入 200 场，点击一次展开后断言
(a) 除目标 entry 外所有 `.match-entry` 的 DOM 节点是**同一个对象**（`===`）；
(b) 单次点击期间 `document.createElement` 调用数 < 500（当前 19,202）。

**风险**：中。

---

## P0-8 escapeHTML 有五份实现、三份用 createElement、两份不转义引号却被拼进属性

**证据**（本次实测五份原文）：
```
web/app.js:2662       createElement 版，不转义 " 和 '
web/champions.js:136  createElement 版，不转义 " 和 '
web/gameplay.js:125   createElement 版 + 额外 .replace(/"/g,"&quot;")
web/friends.js:39     纯字符串版，转义 & < > ' "
web/suite.js:94       纯字符串版，转义 & < > ' "
```
而 `app.js` / `champions.js` 的输出**确实被直接拼进了属性值**：
```
champions.js:941   aria-label="查看${escapeHTML(name)}详情"
champions.js:1506  aria-label="查看${escapeHTML(name)}详情"
champions.js:217   data-tooltip="${escapeHTML(tooltip)}" aria-label="${escapeHTML(...)}"
app.js:239         data-native-select-value="${escapeHTML(option.value)}"
app.js:799         data-client-id="${escapeHTML(item.id)}"
app.js:1240        aria-label="横向浏览${escapeHTML(parent.name)}炫彩"
```
名称来自上游目录（ddragon / op.gg / 皮肤名），一旦包含 `"` 就会提前闭合属性。

**性能侧实测**：jsdom 微基准 20 万次，`createElement` 版 600.3ms（3.00µs/次）vs 字符串版 37.3ms（0.19µs/次），
**16 倍**差距。真实路径上 P0-7 那次单击的 19,202 次 `createElement` 几乎全部来自这里。

**改法**：在 `web/runtime.js` 里导出一份权威实现（常量 map 提到函数外，避免每次匹配都新建对象字面量），
五个模块统一引用：
```js
const HTML_ESCAPES = { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" };
function escapeHTML(value) { return String(value ?? "").replace(/[&<>"']/g, (c) => HTML_ESCAPES[c]); }
```
**先补齐 `"` 和 `'`，再谈性能**——否则就是把一个性能改动变成注入面扩大。

**验收判据**：
(a) 等价性测试：对一组含 `& < > " ' 中文 emoji   undefined null` 的输入，新实现逐字节符合预期；
其中双引号一项应把 `app.js`/`champions.js` 的旧行为判为 bug 而非基线。
(b) 性能护栏：200 场战绩单次展开点击期间 `document.createElement` 调用数 < 500。
(c) 回归：现有全部 `*.test.cjs` 绿。

**风险**：中（是安全等价性风险，不是性能风险）。

---

# P1 — 值得做

## P1-1 每条诊断事件 = 2 次 Lstat + open + write + close，且全局串一把锁

**证据** `storage.go:578` `appendDiagnostic` → `582` `s.diagnosticMu.Lock()` → `597` `appendDiagnosticLocked`；
该函数内 `629` 和 `690` **两次** `os.Lstat(path)`，`696` `os.OpenFile(...O_APPEND)`，`712` `file.Close()`。
没有常驻 fd、没有 bufio、没有异步队列。调用密度：`grep -c 'recordDiagnostic(' gameplay.go` = **55 处**；
另外 `sgp_api.go:686-690` 每个 SGP 请求记一条，`champions.go:574` 每个上游请求记一条。

**影响**：一次总览保守 20~40 条事件 ×（2 stat + 1 open + 1 close）。Windows 带 Defender 实时扫描时
CreateFile/CloseHandle 通常 0.1~0.5ms，即 10~60ms 纯 syscall，全部挤在一把 `diagnosticMu` 上。
并发总览 + 赛季回补 + 对局轮询时互相阻塞，也解释了为什么埋点一密 `duration_ms` 就整体上浮。

**改法**：`localStore` 持有常驻 `*os.File` + `*bufio.Writer`；轮转检查改成维护内存里的 `bytesWritten` 计数器
（超过 2MB 才去 Lstat 一次做轮转，而不是每条 Lstat 两次）；后台 goroutine 每 200ms 或每 64KB Flush；
在 `handleDiagnosticLog` / 导出 / 退出前强制 Flush（`readDiagnosticLog` 已持同一把锁，加 Flush 即可保住"导出看得到刚写的"）。

**验收判据**：`BenchmarkAppendDiagnostic` 写 10000 条，打桩统计 Lstat 次数从 20000 降到 ≤ 200；
另加一条：写 100 条后立刻 `readDiagnosticLog()`，`log_seq` 必须是 1..100 连续无缺（防止用"丢弃未 flush"的方式作弊）。

**风险**：中——这条链路是全项目的证据来源，缓冲会引入"崩溃时丢尾部"。必须保留崩溃前 Flush 与 `log_seq` 连续性。

---

## P1-2 championDataCache 每次写盘都把整个缓存目录 ReadFile 一遍（最多 64MB），还是同步的

**证据** `champion_cache.go:257-260`：
```go
	if err := atomicWriteFile(c.pathFor(entry.Key), data, 0o600); err != nil { return err }
	return c.pruneDisk()
```
`pruneDisk`（318~336 行）为了判断"是不是 hexdata 条目"，对每个非保护文件做 `os.ReadFile(path)` 再 `json.Unmarshal` 取 header：
```go
		protected := item.Name() == "hexdata-state.json"
		if !protected {
			if data, readErr := os.ReadFile(path); readErr == nil {
				var header struct{ Key string `json:"key"` }
				if json.Unmarshal(data, &header) == nil && strings.HasPrefix(header.Key, "hexdata-") { protected = true }
```
上限是 256 条 / 64MB（18~23 行）。`writeDisk` 由 `loadWithStatus`（163~165 行）在 loader 成功后**同步**调用，
调用方是 `champions.go:561`，覆盖 ddragon / cdragon / op.gg / qq101 / hexdata 全部落盘路径。

**影响**：目录填满后，每一次缓存未命中的落盘都要读 255 个文件、最多 64MB 磁盘 I/O + 同量 JSON 解析，
并**阻塞发起该请求的 HTTP handler**（英雄详情、出装、符文、职业选手页）。
更糟的是 `reportChampionUpstream` 在 561 行之后，`duration_ms` 里混进了 prune 的磁盘时间，诊断会把人误导到上游去。

**改法**：(a) 把"是不是 hexdata 条目"改成**文件名可判**——落盘时给 hexdata 条目用独立子目录或固定文件名前缀，
`pruneDisk` 只看 `ReadDir` 的 name + `info.Size()`，零 ReadFile；
(b) prune 改成节流：累计写入超过 8MB 或距上次超过 5 分钟才跑，并放进 goroutine 不阻塞返回。
`champion_cache.go:325-327` 那段"hexdata 是用户可见恢复数据、不可作为通用 LRU victim"的保护语义**必须保留**（那是上一轮踩过的坑），
改子目录时注意 `purgeDiskHost` 与旧文件迁移。

**验收判据**：造 200 个假缓存文件（含 3 个 hexdata-*），打桩统计 `os.ReadFile` 调用次数，
断言一次 `writeDisk` 触发的 ReadFile 次数 == 0（当前约 197）；再断言 hexdata 条目在超限时不被删除
（变异：把保护判据删掉，测试必须红）。

**风险**：低。

---

## P1-3 三个 http.Transport 都没设 MaxIdleConnsPerHost（默认 2），championHTTPClient 还丢了 HTTP/2

**证据**：
```
champions.go:476-482  newChampionHTTPClient：设了自定义 TLSClientConfig，但没有 ForceAttemptHTTP2
lcu.go:660-667        LCU transport
sgp_api.go:199        http: &http.Client{Timeout: 20 * time.Second}   // 连 Transport 都没显式给
```
全仓 `grep MaxIdleConnsPerHost` **零命中** → 全部落到 Go 默认值 2。而实际并发是：
`pro_players_ladder.go:21 proLadderConcurrency = 8`（同一个 op.gg host）、
`riot_api.go:1030 semaphore = 4`（同一个 riot cluster host）、
`gameplay.go:2761 semaphore = 4`（LCU loopback）、
`rank_insights.go:278 matchTiersRankConcurrency = 4`（SGP 单机房）。
另外 `champions.go:481` 设了自定义 `TLSClientConfig` 却没设 `ForceAttemptHTTP2: true`，
这会让该 Transport 完全退回 HTTP/1.1，对 op.gg / hexdata / riot 这些 h2 服务失去多路复用。

**影响**：韩服搜一页 20 场，并发 4 / 池 2 ⇒ 约 10 次额外完整 TLS 握手；跨境 RTT 按 150ms、握手 2-RTT ≈ 300ms，
净增约 1.5s 握手时间。职业选手页 8 并发 × 几十个账号 ⇒ 每轮有 6 条连接被丢弃。

**改法**：三处统一加 `MaxIdleConns: 32, MaxIdleConnsPerHost: 8, IdleConnTimeout: 90 * time.Second`；
`newChampionHTTPClient` 再加 `ForceAttemptHTTP2: true`；
`sgp_api.go:199` 换成带显式 Transport 的 Client（顺带能配 `ResponseHeaderTimeout`）。
**不要**改动现有的 `CheckRedirect` 与 `InsecureSkipVerify` 语义。
特别注意：`sgp_api.go:663-665` 的 `copy := *p.http` 是浅拷贝只为改 `CheckRedirect`，**Transport 指针共享、连接池确实在复用**，
改动时别把它变成每次 new Transport。

**验收判据**：起 httptest TLS server，统计 `tls.Config.GetConfigForClient` 被调次数（= 握手次数），
8 并发发 40 个请求，断言握手次数 ≤ 10（当前约 34）；HTTP/2 用 `httptest.NewUnstartedServer` + `EnableHTTP2 = true`，
断言 `response.Proto == "HTTP/2.0"`。

**风险**：低。

---

## P1-4 熟练度与队列名用 context.Background()，18 秒软预算和请求取消都管不到

**证据**：
```
lcu_api.go:614-622  func (api ChampionMasteryAPI) All(puuid string) ... → api.client.GetBytes(path)
lcu.go:800-802      func (c *LCUClient) GetBytes(path string) ... { return c.GetBytesContext(context.Background(), path) }
gameplay.go:8216    client.queueLabelsMu.Lock(); defer Unlock()   // 锁内发 HTTP
gameplay.go:8227    client.GetJSON("/lol-game-queues/v1/queues", &queues)
```
调用点分别是 `gameplay.go:1312`（熟练度，总览主路径）和 `gameplay.go:1125`（队列名）。

**影响**：`gameplay.go:600` 那个 18 秒软预算（`overviewSoftBudget`）对这两步完全无效。
如果预算是在段位阶段耗尽的，熟练度仍会开始并最多再跑 8 秒（`lcu.go:674 Timeout: 8s`），
总览实际可以 26 秒以上才返回；用户切走页面（`r.Context()` 取消）也拦不住，白烧一次全英雄熟练度请求。
`loadQueueLabels` 在锁内发 HTTP，LCU 卡住时所有总览请求会串在这把锁上 8 秒。

**改法**：给 `ChampionMasteryAPI` 加 `AllContext(ctx, puuid)`，`loadQueueLabels` 加 `loadQueueLabelsContext(ctx, client)`，
总览传 `ctx`。顺手把 `loadQueueLabels` 的 HTTP 调用移出 `queueLabelsMu`（锁内取快照 → 锁外请求 → 加锁写回并双检）。

**验收判据**：构造永久 hang 的假 LCU，给总览传 50ms deadline 的 ctx，
断言 `loadGameplayOverview` 在 200ms 内返回（当前会等到 8s 的 client Timeout）。

**风险**：低。

---

## P1-5 SGP 战绩页缓存只按条目数淘汰（256 条），没有字节预算，估 80~130MB 常驻

**证据**：
```
sgp_api.go:82-83   sgpCacheTTL = 5 * time.Minute ; sgpCacheMax = 256
sgp_api.go:179-185 type sgpHistoryCacheEntry struct { ... games []*riotMatchInfo ... }
sgp_api.go:802     for len(p.historyCache) > sgpCacheMax {
```
写入点 `sgp_api.go:931-933`，由 `season_stats.go:626` 在赛季扫描/回补里以 `sgpPageSize = 50` 逐页调用。

**影响**：`riotMatchInfo` 每场含 10 名参与者的完整 Match-V5 字段，粗算 6~10KB/场 ⇒ 一条 50 场缓存项 300~500KB
⇒ 256 条上限约 **80~130MB 常驻 5 分钟**。这正对得上历史记录里"首次总览 24s / 159MB"的量级。
对照 `champion_cache.go:24-25` 同时设了条目数和字节数双上限，SGP 这边缺了字节维度。
更糟的是赛季回补那几十页写完就**永远不会再被读**（`seasonScanPagesWithHistoryCache` 靠 `cache.ResumeIndex` 推进），纯污染。

**改法**：(a) 给 `sgpProvider` 加 `historyBytes` 与 `sgpCacheMaxBytes`（建议 24MB），用 `len(body)` 累计最诚实，
在 `cacheHistoryPage` 的淘汰循环里同时按字节收敛；(b) `season_stats.go:737 startSeasonBackfill` 把 `useHistoryCache` 传 `false`，
只让首屏增量头部扫描用缓存。**(b) 风险更低，可以先只做 (b)。**

**验收判据**：连续灌 300 页 × 50 场假数据，断言 `len(p.historyCache)` 与累计字节都在上限内；
另一条：让赛季回补扫 10 页，断言回补结束后 `p.historyCache` 里没有 `startIndex >= 50` 的条目。

**风险**：低。

---

## P1-6 Riot 对局详情只有 600 条内存缓存，没有磁盘缓存也没有 singleflight —— 而对局是不可变数据

**证据**：
```
riot_api.go:131      riotMatchCacheMax = 600
riot_api.go:687-695  matchByIDWithCache：命中就返回，未命中直接 p.get(...)，无 flight
riot_api.go:254-260  shortLimit = 15/s ; longLimit = 90 / 2min   // 内嵌 personal key，全体用户共享
```

**影响**：一次韩服搜索 ≈ 1 account + 1 summoner + 1 matchIDs + 20 matchByID + 1 leagues + 1 masteries ≈ **25 个配额单位**。
90/2min 意味着 2 分钟内只能搜 3 个玩家，第 4 个直接撞 `riot_local_rate_limited` 弹"额度正在恢复"。
而对局详情永久不变，进程一重启 600 条内存缓存全丢，重开同一玩家又是 20 个配额单位。
项目里已有 `championDataCache`（内存 + 磁盘 + 陈旧兜底 + singleflight，`champion_cache.go:86`）这套现成设施却没用上。

**改法**：把 `matchByIDWithCache` 的 loader 包进 `championDataCache.load(ctx, "v1|riot-match|"+matchID, 365*24h, 0, true, loader)`，
并在 `championCacheDiskAllowed`（263~273 行）放行 `v1|riot-match|` 前缀。用它就顺带拿到 singleflight。
TTL 用超长值，靠 `pruneDisk` 的 LRU 收口。
**隐私注意**：落盘的是他人对局完整 JSON，含玩家 PUUID，与 `features.go:656` 的 `neverStores` 声明有冲突风险 ——
**建议只缓存转换后的 `gameplayMatch`，不要缓存原始 payload**，或先确认隐私声明口径。

**验收判据**：同一 matchID 经两个新建的 `riotProvider`（模拟重启，共享同一 cache dir）请求，断言假 Riot server 只收到 1 次；
并发 8 个同 matchID 请求，断言上游只收到 1 次。

**风险**：中（隐私口径需先确认）。

---

## P1-7 自动"再来一局"三个阶段共用一个 3 秒去重键，最多迟 8.4 秒才点

**证据** `watch_rules.go:429-432`：
```go
	case "WaitingForStats", "PreEndOfGame", "EndOfGame":
		if settings.Rules.AutoPlayAgain.Enabled {
			delay := map[string]int{"WaitingForStats": 10000, "PreEndOfGame": 3250, "EndOfGame": 1575}[phase]
			r.schedulePlayAgain(client, delay)
```
`watch_rules.go:547` → `r.schedule(client, "play-again", delayMS, ...)`，而 `watch_rules.go:550-555`：
```go
func (r *watchRunner) schedule(client *LCUClient, action string, delayMS int, method, path string, body any) bool {
	if !r.markRun(action, 3*time.Second) { return false }
	return r.scheduleMarked(client, action, delayMS, method, path, body)
}
```
`markRun`（983~994 行）在**同一 action 名 3 秒内**直接返回 false，而这个拦截发生在 `scheduleMarked`
（那里才做"取消上一个待发定时器并重新武装"，588~591 行）之前。三个阶段用的是同一个字面量 `"play-again"`。

**影响**：`WaitingForStats` 先到（武装 10000ms 定时器），若 `PreEndOfGame`/`EndOfGame` 在 3 秒内接着到，
去重闸门返回 false，更短更准确的 3250ms / 1575ms **永远不会被武装**，原来那个 10000ms 继续跑，
且没有任何日志提示"这次重排被跳过了"。净效果：最多迟约 8.4 秒。
这和历史上"快速下一把被自己 3 秒去重吃掉"是同一类 bug，只是这次发生在 play-again 而非 accept。

**改法**：把 `markRun`/`pending` 的键改成 `"play-again:" + phase`；或者在新调用的目标触发时刻
（`now + delayMS`）早于当前已武装定时器的触发时刻时，**绕过去重闸门**。后者更贴合语义。

**验收判据**：`handlePhase(client, "WaitingForStats")`，1 秒后 `handlePhase(client, "EndOfGame")`，
断言实际 POST `/lol-lobby/v2/play-again` 发生在 `armed_at + 1575ms`，而不是 `armed_at + 10000ms`。

**风险**：低。

---

## P1-8 切换战绩筛选、以及韩服/职业玩家的外部战绩视图，同样整列表重建

**证据**：
- 筛选：`web/gameplay.js:3295-3309` `list.innerHTML = matches.map(...).join("")` +
  `bindOverviewContent` + `applyRenderedMetricStyles` + `prepareImages`。jsdom 实测 17 场 demo 下单次筛选 104.7ms，规模按 P0-7 的表线性放大。
- 外部视图：`web/gameplay.js:5501-5513` 的 `view.render()` **连复用闸门都没有**，无条件
  `container.innerHTML = matches.map(...)`；且在"读取完整详情"路径上会被调用**两次**（5483 的 loading 态 + 5498 的结果态），
  即一次点击两次全量重建。

**改法**：筛选改成对已渲染的 `.match-entry` 按 `dataset.gameId` 打 `hidden`，
只对"筛选结果里存在但从未渲染过"的场次追加渲染（`appendOverviewMatches`，595~610 行，已经是正确的增量写法，直接复用）；
外部视图复用 P0-7 的局部替换函数，至少把 loading 态改成只改目标 entry 的 class。

**验收判据**：注入 200 场，"全部" → "斗魂竞技场" → "全部"，断言回到"全部"后所有 `.match-entry` 仍是切换前的同一批 DOM 对象，
且两次切换期间 `createElement` 调用数 < 200。外部视图路径走 `desktop/pro-players.test.cjs` 的 boot 设施同样断言。

**风险**：低。

---

## P1-9 英雄页搜索每敲一个字都整页重建，且 championMeta 是 O(N²)

**证据**：
```
champions.js:2257-2261  root.addEventListener("input", ...) → updateChampionSearch(...)，无防抖
champions.js:2270-2278  preserveSearchInput：update() → render() → 重新 querySelector 并手动恢复焦点与光标
champions.js:894        root.innerHTML = ...   // 整页：页头 + 段位下拉 + 模式页签 + 完整英雄表
champions.js:169/185    objectRows() 每次新建一个 170 元素数组；championMeta(id) 再线性 find
champions.js:1497       renderChampionRow 里每行调一次 championMeta
champions.js:2001-2003  filteredChampionRows 里每行再调一次，并对 8 个字段各跑一次 normalizeSearch
champions.js:2284       updateChampionSearch 每次按键 writeSetting(...) 同步写 localStorage
```
"输入框在每次按键时被销毁重建"这件事，从 `preserveSearchInput` 需要手动恢复焦点与光标这个事实本身就能证明。
jsdom 实测（170 行英雄表，3312 节点 / 343 个 `<img>`）连续按键同步耗时 `[158.4, 151.9, 27.9, 13.4, 14.8] ms`。
一次渲染 = 340 次 `championMeta` = 340 个临时 170 元素数组 + 约 58,000 次比较；
`normalizeSearch` 的 `.normalize("NFKC").toLocaleLowerCase("zh-CN")` 两个 ICU 调用被跑约 1400 次/渲染。

**影响**：中文 IME 有 `compositionstart/end` 兜住（2263~2268 行），但英文/拼音直打时每个字符都付一次整页重建 +
343 个 `<img>` 重建 + 焦点重置，真机上是可感知的输入延迟与光标抖动。

**改法**：(a) 搜索加 120~180ms 防抖，`writeSetting` 跟着防抖走；
(b) 更彻底：搜索只重渲染 `.champion-table-scroll` 内的 `<tbody>`，不碰页头/页签/输入框——这样 `preserveSearchInput`
那套焦点恢复体操可以整段删掉；
(c) `state.catalog` 落库时（`adoptCatalog`/`stampRankings`）顺手建 `Map<number, meta>` 和
`Map<championId, normalizedSearchTerms[]>`，`championMeta` / `normalizeSearch` 全部走 Map。

**验收判据**：(a) 连打 5 个字符，断言每次 `input` 后 `[data-champion-search]` 仍是同一个 DOM 对象（当前会变）；
(b) 5 次按键期间 `render()` 被调用 ≤ 2 次；(c) 渲染一次 170 行，断言 `objectRows` 调用次数 ≤ 5（当前 350+）。

**风险**：低。

---

## P1-10 `/api/gameplay/phase` 每秒常驻轮询，与当前在哪一页无关，而 SSE 已经在推同一件事

**证据** `web/gameplay.js:5845-5864`：
```js
  const BEACON_FAST_POLL_MS = 1_000;
  ...
  if (document.hidden) { scheduleBeaconPoll(BEACON_IDLE_POLL_MS); return; }
  if (!connected()) { scheduleBeaconPoll(BEACON_DISCONNECTED_POLL_MS); return; }
  api("/api/gameplay/phase", {}, "gameflow-phase", 8000) ...
```
后端每次真打一发 LCU HTTPS（`gameplay.go:5586-5598`）。而 SSE 已经在推 `gameflow:` 事件
（`app.js:3221-3225` 转发成 `deep-legends:gameflow`，`gameplay.js:5832` 消费并直接 `updateBeacon(phase)`），
所以这 1 秒轮询是纯兜底。

**影响**：客户端已连接且窗口可见时，无论用户在收藏页还是设置页，都是 **3600 次/小时**本地 HTTP + 3600 次 LCU 请求。
`document.hidden` 有保护，但"窗口在前台但在看别的页"没有。

**改法**：把兜底轮询做成 SSE 健康度的函数——`state.liveEventsReady === true` 且最近 N 秒收到过任意 SSE 帧时退到
`BEACON_IDLE_POLL_MS`；只有 SSE 断开（`onerror` / `live-disconnected`）才回到 1 秒。
**必须保留"SSE 不可用时 1s 兜底"这条路径**，R79 的自动接受链路对延迟敏感。

**验收判据**：jsdom + `overview-render.test.cjs` 里现成的 MockEventSource，统计 30 秒内对 `/api/gameplay/phase` 的 fetch 次数：
SSE 正常时 ≤ 4 次；手动触发 `source.onerror` + `readyState = CLOSED` 后 ≥ 20 次。**两个方向都要测**，
否则很容易改成"永远不兜底"。

**风险**：中。

---

## P1-11 进入"工具"页一次性拉全部 5 个子页签的数据

**证据** `web/suite.js:1828-1837` 的 section 事件里无条件 `loadAll()`；`suite.js:224-236`：
```js
    const tasks = [ loadWatch(force), loadRig(force), loadFacade(force), loadClaims(force), loadChampSelect(force) ];
    await Promise.allSettled(tasks);
```
其中 `loadChampSelect` 首次还会再拉 4 个接口含 `/api/champions/catalog`（753~756 行）。
**已有保护**：`loadWatch`(412)、`loadRig`(1143)、`loadClaims`(1793) 都有 `if (state.xxx && !force) { render(); return; }` 短路，
`loadFacade`(1516) 有 30 秒新鲜度窗口——所以这只在首次进入与缓存失效后付全额代价。
但 `/api/champselect/state` 每次进入无条件重拉。

**改法**：`loadAll` 只加载 `state.tab` 对应的那一个 + 影响顶栏/连接状态的 `loadRig`，其余交给 `activateTab` 按需加载
（`suite.js:205-206` 本来就是这个模式，只是被 `loadAll` 抢先了）。

**验收判据**：jsdom 点进"工具"（默认 watch 页签），断言首次进入期间的 fetch URL 集合**不含**
`/api/claim/scan`、`/api/facade/state`、`/api/champions/catalog`；随后点"领奖"，此时才出现 `/api/claim/scan`。

**风险**：低。

---

## P1-12 首屏同步阻塞 1.01MB JS + 418KB CSS，其中 84KB 是生产环境立即 return 的 demo 数据

**证据** `web/index.html:10-27` 有 8 个渲染阻塞 CSS + 10 个 `defer` 脚本；`web/demo-data.js:6-10` 生产环境第 10 行就 return，
但 84,485 字节已经下载并解析完了。本次实测（`gzip -1`，与 `static_assets.go:52-59` 的 `BestSpeed` 一致）：

| | 原始 | gzip |
|---|---|---|
| CSS × 8 | 418,309 B | 107,243 B |
| JS × 10 | 1,014,048 B | 327,026 B |
| 其中 demo-data.js | 84,485 B | 28,834 B |
| 其中 gameplay.js | 385,766 B | 124,145 B |

**已有保护**：`index.html:173` 内联了骨架屏，所以零 JS 状态下首帧**不是白屏**。这条说的是"首帧到可交互之间的解析税"，
叠在已知的 Electron 冷启动 1.7s 黑屏之后。

**改法**（按性价比排序，可以只做第 1 条）：
1. `demo-data.js` 改成只在 `?demo` / `#demo` / localStorage 命中时由 `runtime.js` 动态插 `<script>`，默认零字节。
2. `champions.js`(148KB) / `suite.js`(137KB) / `pro-players.js` / `friends.js` 改为首次进入对应 section 时按需加载。
   它们的 IIFE 都在模块末尾自初始化（如 `champions.js:2352-2354`），需要包一层 `init()` 导出。
3. `champions.css`(90KB) / `suite.css`(49KB) / `pro-players.css` 跟着各自 JS 一起动态插 `<link>`。

**⚠️ 第 2/3 条的关键风险**：`deep-legends:section` / `deep-legends:status` 这些事件是在模块解析期注册的，
晚加载的模块会**错过早期事件**——必须给晚加载模块补一次"当前状态重放"。

**验收判据**：(a) 生产 `index.html` 的同步 `<script>` 不含 `demo-data.js`，且 `?demo` 下 demo 角标仍出现
（现有 `overview-render.test.cjs` 全套 demo 用例必须全绿，这是防"为了瘦身把 demo 打死"的护栏）；
(b) 一个 node 脚本统计 `index.html` 所有同步 CSS+JS 的 gzip 字节总和并设阈值（当前 434KB），超了就红。

**风险**：第 1 条低，第 2/3 条中。

---

## P1-13 `--ui-zoom` 漏改四处（项目自己在 app.css:33 定了规则）

`web/app.css:33` 自己写着规则："只有 .app-frame（以及浮层）真正吃 zoom；所有 100vh/100dvh 必须除以它"。
以下四处违反了这条自定规则。参照组是三个做对了的原生 dialog：
`app.css:817 .skin-dialog`、`gameplay.css:261 .career-dialog`、`champions.css:277 .mayhem-tier-dialog`
（它们都自己声明了 `zoom: var(--ui-zoom, 1)`，因为 `.showModal()` 进顶层后是 `#app-frame` 的兄弟节点，不继承其 zoom）。

| # | 位置 | 症状 | 改法 |
|---|---|---|---|
| a | `suite.css:528 .cs-dialog` **无 zoom**，而 `:530 .cs-dialog-sheet` 却写了 `max-height: calc(100vh / var(--ui-zoom,1) - 32px)` | 弹窗本体不放大，内容区高度却按"已放大"去除法。zoom=2.5 时（`app.js:2264 UI_SCALE_STEPS` 含 2.5）内容区被压到真实视口 40% | 给 `.cs-dialog` 补 `zoom: var(--ui-zoom, 1)`，并把 width/max-height 改成 `calc(100% / var(--ui-zoom,1) - 32px)` 一类 |
| b | `app.css:1002-1008 .update-dialog / .update-sheet` 完全不吃 zoom | 高分屏放大后唯独"发现新版本"弹窗保持 1x，明显偏小 | 照 `.skin-dialog` 补 zoom + vw/dvh 除法 |
| c | `app.css:469 .app-select-menu` 的 `max-height: min(320px, 50vh)` 未除 zoom（同文件 `:330 .region-menu` 已正确除） | 它在 `.app-frame` 内会被整体放大，zoom>1 时真实高度达 `50vh × zoom`（最高 125% 视口），长下拉会顶出屏幕 | 改成 `min(320px, calc(50vh / var(--ui-zoom, 1)))` |
| d | `app.css:849 .toast` 纯固定 px 无补偿，而同为 body 级浮层的 `app.css:202 .global-tooltip` 已正确用 `calc(Npx * var(--ui-zoom,1))` | 放大后 toast 比界面明显偏小 | 照 tooltip 把 padding / font-size 改成 `calc(Npx * var(--ui-zoom,1))` |

另有一处低优先级：`app.css:924` 的 ≤620px 媒体查询把 `.skin-dialog` 的 max-height 覆盖回裸 `100dvh`，丢了 zoom 除法。

**验收判据**：把 `--ui-zoom` 设为 2 和 2.5，分别打开征召设置弹窗、更新弹窗、长选项 app-select 下拉、触发一次 toast，
断言四者的可视尺寸与放大后的界面成比例且不溢出视口。**注意：本条的全部结论来自静态代码比对，未跑真 Chromium 量像素
（沙箱装不上浏览器），落地时请在真机上截图核对。**

**风险**：中（仅非 1x 缩放时触发，但触发后 a 是布局级损坏）。

---

## P1-14 英雄目录拉取失败时没有负缓存，ddragon 不可达的机器每次总览白等 6 秒

**证据** `riot_api.go:1277-1285`：
```go
	p.mu.Lock(); loaded := len(p.championMeta) > 0; p.mu.Unlock()
	if !loaded {
		loadContext, cancel := context.WithTimeout(ctx, 10*time.Second)
		_, _ = p.loadCatalog(loadContext)
		cancel()
	}
```
调用链 `gameplay.go:1127 → gameplay.go:1554（6 秒上限）→ riot_api.go:916`；`loadCatalog` 是 3 个串行 ddragon 请求。
失败路径没有负缓存：`champion_cache.go:168-185` 在 loader 出错时只尝试返回陈旧数据，否则**什么都不存**，下次重来。
对比同仓 `asset_cache.go:23` 的签名里就带 `negativeTTL` 并在 64~69 行落盘失败窗口 —— 一个仓库两套缓存，一个有一个没有。

**影响**：首次安装 + ddragon 被墙/代理没配对的机器上，**每一次**总览都要先白等 6 秒才开始读战绩，且失败不留痕给用户。
成功过一次后 24h TTL + 7 天 stale 会兜住，所以只影响"从未成功过"和"离线超过 7 天"—— 但那正是首次启动的体验。

**改法**：给 `championDataCache.loadWithStatus` 加 `negativeTTL`（复用 `asset_cache.go` 的模式，建议 60 秒），
或在 `championProvider` 上加 `catalogFailUntil time.Time`（仿 `sgp_api.go:277-281 failUntil`）：
失败后 60 秒内 `championNamesZH` 直接返回空映射走降级显示。别把"目录可用但内容变了"也当失败缓存掉。

**验收判据**：让假 ddragon 一直返回 500，连续调 5 次 `overviewChampionNames`，
断言上游只收到 3 次请求（第一次的 3 个）而不是 15 次，且第 2~5 次调用在 10ms 内返回。

**风险**：低。

---

## P1-15 desktop.log 拿不到、也不会轮转，目录名还和产品名不一致

**证据**：
```
desktop/main.cjs:264-267   const directory = path.join(app.getPath("userData"), "logs"); fs.appendFileSync(... "desktop.log" ...)
desktop/package.json:2     "name": "deep-legends-desktop"      // main.cjs 全文无 app.setName()
desktop/package.json:22    "productName": "Deep Legends"
desktop/diagnostics-export.cjs:6-33  只拦截 /api/diagnostics/log 一个后端接口
desktop/main.cjs:258-269   appendDesktopLog 只对单行 slice(0,2000)，整个文件无大小检查、无轮转
desktop/main.cjs:316       backend.stderr.on("data", appendDesktopLog)
```
对照后端日志：`storage.go:621-689` 有 2MB 触发轮转 + 最多 5 代（总量硬顶约 10MB）。

**影响**：(a) 外壳阶段的全部证据（启动耗时、splash、代理下发、后端 stderr）只在 `desktop.log` 里，
用户点"导出诊断日志"拿到的文件**完全没有这部分**，排查启动卡死时会误判成"零证据 = 没发生"（这个误判已经真实发生过）；
(b) 后端崩溃重启循环或代理探测反复失败时，这个文件会无限增长；
(c) 实际路径是 `%APPDATA%\deep-legends-desktop\logs\`，不是用户直觉的 `%APPDATA%\Deep Legends`。

**改法**：(a) 在 `attachDiagnosticsExport` 里追加读取
`path.join(app.getPath("userData"), "logs", "desktop.log")` 并拼进导出产物；
(b) 给 `appendDesktopLog` 加同款大小触发轮转（复用 Go 侧 2MB/5 代思路）；
(c) 调 `app.setName("Deep Legends")` 让目录名与产品名一致 —— **⚠️ 注意这会改变 userData 路径，
需要处理旧目录迁移，否则用户的既有设置会"丢失"。如果没把握，(c) 可以先不做，只做 (a)(b)。**

**验收判据**：真机触发一次外壳阶段事件，点"导出诊断日志"，确认导出文件内含该事件文本；
构造反复写入到 10MB+ 的测试，确认文件不再无限增长。

**风险**：(a)(b) 低，(c) 中。

**不做 (c) 的影响（维持现状）**：纯体验问题，不影响功能。日志目录会继续叫
`%APPDATA%\deep-legends-desktop` 而不是用户直觉的 `%APPDATA%\Deep Legends`，用户手动去翻日志文件夹时会找错地方，
仅此而已——应用照样正常读写这个目录。**(a)(b) 与 (c) 无关，建议照做**；(c) 可以无限期搁置，
真要做的话务必先处理好 `userData` 路径迁移（旧目录下的设置/缓存/token 不会自动跟过去），否则会造成
"用户升级后设置像是丢了"的更差体验，那样反而不如不做。

---

## P1-16 riot key 自检护栏是字符串型的，一行就能绕过

**证据** `riot_key_test.go:25-33` `TestSelfCheckRiotKeyUsesConfiguredGuard` 做的是
`strings.Contains(string(mainGoSource), "if !riotKeyConfigured()")` —— 对**整个 main.go** 做子串匹配。
只要这个字符串在文件里任意位置存在（注释里、死代码里、挂在无关 flag 上）测试就绿，
完全不验证它是否真的位于 `-self-check-riot-key` 分支内（当前 `main.go:313-316` 是对的，但没有任何东西把两者绑在一起）。

**影响**：把真检查移到一个提前 return 之后、同时把那行字符串留在注释里，测试照样绿，而实际保护失效。
这是"防止公开构建在没有 key 的情况下静默通过"的安全相邻护栏。

**改法**：照抄 `installer/message_wiring_test.go` 与 `installer/completion_wiring_test.go` 已经验证有效的 AST 模式 ——
用 `go/parser` 解析 `main.go`，定位 `if *selfCheckRiotKey` 语句块，断言其 body 内容。
那两个文件的注释（`message_wiring_test.go:10-12`）明确写了它们是在历史绕过事故后重新设计的：
**守路由/边界，而不是守函数名清单**。这次照同样的思路做。

**验收判据**：做两次变异 ——
(1) 把检查移进注释同时加一个提前 bypass return，断言**新**测试红、**旧**测试绿（证明新护栏真的更强）；
(2) 新增一条旁路分支处理 `-self-check-riot-key`，断言新测试仍然红（这是本项目历史上真实发生过的绕过手法）。

**风险**：低（仅测试）。

---

## P1-17 一次缓存命中会把该轮后续所有页强制降到 20 条

**证据** `sgp_api.go:840-849`（本次实测原文）：
```go
	downgraded := false
	for len(games) < count && fetched < count+sgpPageSize {
		pageSize := count - len(games)
		if pageSize > sgpPageSize { pageSize = sgpPageSize }
		if cacheHit || downgraded { pageSize = min(pageSize, sgpFallbackPageSize) }
```
`sgpPageSize = 50`、`sgpFallbackPageSize = 20`（76~78 行）。`downgraded` 是服务端真的把 50 裁成 20 时设的
（928、936~939 行），那是合理的；`cacheHit` 没有这个理由。

**影响**：赛季扫描（`season_stats.go:626`）用 `count = 50` 请求。第 0 页会命中总览之前缓存的那条
`(startIndex=0, pageSize=20)` 条目（`cachedHistoryPage` 在 769 行从 maxPageSize 向下逐个试 key），
于是 `cacheHit=true`，剩下 30 场被拆成 20 + 10 两个请求 —— 本来 1 个请求能拿完。
即每次赛季扫描第 0 页多出 1 个 SGP 往返（约 200~600ms + 一次令牌校验）。

**改法**：849 行改成只有 `downgraded` 才降级。缓存写入侧（931 行）已按实际 pageSize 建键，
读取侧（769 行）的向下探测能兼容任意粒度，不需要为对齐键而降级。
**先做一步**：`git log -S "cacheHit || downgraded"` 查一下引入原因，确认它不是为绕某个已观测到的网关行为而加的，再改。

**验收判据**：预置一条 `(0, 20)` 缓存条目，再请求 `count=50`，
断言假 SGP 只收到 **1** 个请求且其 count 参数为 30（当前会收到 2 个，count 分别为 20 和 10）。

**风险**：低。

---

## P1-18 对一个永不为 nil 的字段取 app 级写锁

**证据** `rank_insights.go:189-194`：
```go
	a.mu.Lock()
	if a.rankScores == nil { a.rankScores = newRankScoreCache() }
	cache := a.rankScores
	a.mu.Unlock()
```
而 `main.go:429` 在 `main()` 的 setup 段（mux 注册之前）就无条件执行了 `a.rankScores = newRankScoreCache()`。
`a.mu` 是全局 `sync.RWMutex`（`main.go:60`），被 `gameplayClient()`、`currentPlayerRef()`、`championNames()` 等到处 RLock。

**影响**：`/api/gameplay/match-tiers` 一次最多 24 个 playerRef（`rank_insights.go:335`）、4 并发（388 行），
每个 goroutine 都抢一次 app 级**写**锁，写锁期间所有读者排队。单次只有微秒级，但这是纯粹的无谓竞争，改动成本几乎为零。

**改法**：删掉 189~194 行的加锁，直接用 `a.rankScores`；若担心测试里手搓 `app{}` 的场景，
改成 `sync.OnceValue` 或让 `rankScoreCache` 的方法 nil-safe，**不要动全局锁**。

**验收判据**：`go test -race` 下 100 个 goroutine 同时调 `playerRankScoreWithCacheStatus`，断言无 race 且结果一致；
再加一条 grep 型护栏断言 `rank_insights.go` 里不再出现 `a.mu.Lock()`（并对该护栏做变异：加回去必须红）。

**风险**：低。

---

## P1-19 韩服战绩详情的信号量获取不响应取消，取消后仍烧共享 Riot 配额

**证据** `riot_api.go:1032-1038`：
```go
	for index := range ids {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			detail, detailErr := provider.matchByID(ctx, ids[index])
```
同仓写对了的版本在 `gameplay.go:2777-2781`：
```go
		select {
		case semaphore <- struct{}{}:
		case <-ctx.Done():
			return
		}
```

**影响**：用户搜到一半切页面（`r.Context()` 取消），排队中的十几个 goroutine 仍会各走一遍。
`p.get` 内部的 `p.wait(ctx)` 会因取消提前返回（262~264 行），所以**实际不会真的发出请求**——
真正的残余问题是 `p.wait` 已经把时间戳记进了限速窗口（270~271 行在返回 nil 前 append），
即**被取消的请求仍占用了 90/2min 的配额预算**。

**改法**：1036 行改成与 `gameplay.go:2777-2781` 一致的 select；
并在 265~274 行的窗口记账后、发请求前再查一次 `ctx.Err()`，已取消就回滚刚 append 的时间戳
（或把记账挪到 `client.Do` 之前最后一刻）。

**验收判据**：给 30 个 matchID、并发 4，在第 5 个请求到达后取消 ctx，
断言假 Riot server 收到的请求数 ≤ 8，且 `len(p.longWindow)` ≤ 8。

**风险**：低。

---

# P2 — 清理与长期健康

## P2-1 死代码 20 处 + 孤儿资源 7 个

用 `deadcode -test ./...`（`go install golang.org/x/tools/cmd/deadcode@latest`，已验证可跑）得到 20 个不可达函数，
每一个都用 `grep -rn` 复核过引用计数（我本人抽查了 4 个，均只剩定义行本身）：

```
catalog.go:135            loadSnapshot(pool PoolManifest)        // 与 storage.go:483 的方法同名不同函数
lcu.go:394                discoverLCU()                          // 唯一调用方就是上面那个，链式死代码
client_launcher.go:147    detectClientInstallations()            // 与 (a *app) findClientInstallation 新旧并存
client_launcher.go:430    findClientInstallation(id)             // 同上，包级自由函数版本
champions.go:1883         championProvider.loadArenaPage
champions.go:1905         arenaTierValue
champions.go:2743         patchFromOPGGPath                      ✅已复核：1 次命中（仅定义行）
gameplay.go:4651          liveClientPositionForPlayer            ✅已复核：1 次命中
gameplay.go:7399          upsertLCUItemSet                       ✅已复核：1 次命中
gameplay.go:8267          queueModeGroup
main.go:1010              (a *app) refresh()
match_timeline.go:374     app.loadMatchTimelineCN
opgg_insights.go:439      app.annotateOPGGAverageTiers
player_ability.go:63      gameplayAbilityRank
player_ability.go:278     gameplayRankTitleZH
profile_facade.go:912     facadeTitleRestoreCandidates
profile_facade.go:1097    facadeImagePath                        ✅已复核：1 次命中
profile_facade.go:1104    facadeLevelLabel
qq101.go:307              findQQ101Position
sgp_api.go:337            (p *sgpProvider) leagueSessionToken
sgp_api.go:1042           (p *sgpProvider) rankedStats           // 注意：rankedStats 有 7 处命中，6 处是端点路径字符串
sgp_auth_diagnostics.go:136  sgpTokenIdentityDiagnostic
watch_rules.go:975        (r *watchRunner) run
watch_rules.go:1006       cancelAllPending
```
**删除顺序有讲究**：`catalog.go:135 loadSnapshot` 和 `lcu.go:394 discoverLCU` 互为唯一调用链，
删完一轮后**重跑 `deadcode -test ./...`** 看有没有级联出新的死代码。

孤儿资源（`grep` 全仓零命中，已实测）：
- `web/loot-icons/hextech-chest.png`（70,784 字节）—— 对照组 `hextech-chest-transparent.png` / `promotion-chest.png`
  在 `web/app.js:2594-2596` 确有引用。
- `web/tier-icons/{1,2,3,4,5,op}.svg`（共 3,015 字节）—— 已被 `yourgg-{s,a,b,c,d,f}.svg` 取代
  （新方案在 `champions.js:160`）。
  **⚠️ 联动**：`main_test.go:111` 用 `"web/tier-icons/1.svg"` 作为"目录被正确 embed"的抽样断言。
  删文件前必须把这行换成仍存在的文件（如 `web/tier-icons/yourgg-a.svg`），否则 `go test ./...` 必红。
  这正是"go:embed 引用在代码里是字符串"的实例。

**依赖**：`go.mod` 四个依赖全部在用，**无可删**。特别注意 `golang.org/x/sys` **只在 `*_windows.go` 里使用**
（`accept_window_windows.go` / `client_installations_windows.go` / `update_platform_windows.go`），
Linux 上看着"没人 import"但绝不能删。`desktop/package.json` 三个 devDependencies 均在用。

**验收判据**：`go build ./... && go vet ./... && go test ./...` 全绿；
`GOOS=windows go vet ./...`（根 + installer 两个模块）也要绿；重跑 `deadcode -test ./...` 确认无级联残留。

**风险**：低。

---

## P2-2 根目录 34 份历史文档归档

根目录有 34 份 `ACCEPTANCE-*.md` / `WORKLIST-*.md` / `DIAGNOSIS-*.md`（R80~R85 产物，共 320K）。
建议**整体一起**搬到 `docs/history/`，保持文件名不变即可维持它们之间的互链。

**⚠️ 硬约束**：`README.md` 第 122、136 行各有一处真实超链接指向
`ACCEPTANCE-REPORT-R83-READ-ONLY-DEFAULT.md` 和 `ACCEPTANCE-REPORT-R84-CLEANUP.md`；
`docs/r82-current-game-probe-matrix-results.md` 和 `docs/r85-manual-update-check.md` 也各链了一份根目录 WORKLIST。
搬移时必须同步改这 4 处路径。

**验收判据**：搬完后 `grep -rn 'ACCEPTANCE-REPORT-\|WORKLIST-R8[0-5]' README.md docs/` 无残留旧路径。

**风险**：低。

---

## P2-3 死 CSS

- 整块"资源统计卡"：`web/app.css:636-647`（`.resource-section` / `.resource-grid` / `.resource-card` /
  `.resource-card::after` / `.resource-glyph` / `.resource-blue|orange|gold|violet`），8 个类名在 `web/*.js` 与 `index.html` 全部零命中。
- 整套"领奖台"：`web/champions.css:570-604`（`.augment-podium` / `.podium-place` / `.podium-place.is-1|2|3` /
  `.arena-augment-podium .podium-place` 及 `--arena-augment-quality` 系列）+ 移动端覆盖 `champions.css:747-753`，约 40 行。
- 零散死类 14 个（均已逐个 grep 核对，**已排除** `is-color-${n}` / `is-slot-${n}` / `level-${n}` / `is-metric-${n}`
  这类模板字符串动态拼接的假阳性）：
  `pro-players.css:63,72 .pro-table-hint`、`suite.css:135 .suite-range-value`、`suite.css:434 .cs-cap-limit`、
  `suite.css:469-472 .cs-avoid-row`、`app.css:392-393,872 .intro/.intro-copy`、`app.css:845 .dialog-description`、
  `gameplay.css:837 .replay-button`、`gameplay.css:1329-1333 .force-no-subgrid`、
  `gameplay.css:1409 .item-set-manual-note`、`champions.css:44 .arena-synergy-panel`、
  `champions.css:100-101 .rune-page-card`、`champions.css:695 .counter-groups`、
  `champions.css:855 .arena-prism-section`、`champions.css:880-882 .arena-core-meta`。

**⚠️ 注意**：初筛共产出 137 个候选类名，上面只是逐一核对确认死亡的那部分。
**清理前请对全量 137 个做一次脚本化交叉核对**（含动态拼接模式），不要凭初筛清单批量删。

**验收判据**：删除后 `node --test web/*.test.cjs desktop/*.test.cjs` 全绿
（`champions.test.cjs` 里有对具体 CSS 变量值的断言，会兜住误删）。

**风险**：低。

---

## P2-4 build-windows.ps1 已经没人引用

`grep -rn "build-windows.ps1"` 在 README、`ci.yml`、`make-release.cjs` 等处**零命中**（只有文件自己）。
它产出的是独立便携版 `Deep Legends.exe` + `.zip`，没有 NSIS 安装器也没有 Go/WebView2 外壳 ——
而 `README.md:124` 明确写着当前发布流程只产出 `Deep Legends Setup <version>.exe`、不生成便携版。
它的默认版本号还是 `0.11.2`（第三个不同的值，见 P2-5）。

**改法**：删除，或明确标注 deprecated。**真实风险**是有人在发版时跑错脚本，发出一个和文档不符的未签名便携包。

**验收判据**：移除后全仓 grep 无引用。

**风险**：低。

---

## P2-5 版本号要手改 5 处，而且已经漂了

当前 `desktop/package.json` = `0.12.1`。其余各处：
```
build-desktop.sh:6              0.12.1   （且第 8 行会与 package.json 核对并在不一致时中止 ✅）
build-desktop-windows.ps1:2     0.12.1   （第 18-21 行同样有核对 ✅）
build-windows.ps1:2             0.11.2   ← 第三个值（该脚本已废弃，见 P2-4）
.github/workflows/ci.yml:58     0.11.1   ← 落后两个小版本，且无任何核对
CHANGELOG.md 标题                手动
```
**改法**：`ci.yml` 改成 `node -p "require('./desktop/package.json').version"` 读取，别再写死。

**验收判据**：把 `package.json` 版本 +1，确认 `ci.yml` 无需手改即能跟上。

**风险**：低。

---

## P2-6 五个用真实 sleep 的测试占了整套 Go 测试 58% 的时长

实测 `go test -json .`：总 66.1s / 1414 pass / 0 fail / 14 skip。其中：
```
TestGameplayOverviewReturnsPartialBeforeTwentySeconds   18.0s   gameplay_test.go:282（mock transport 真 time.After(8s)，被打 3 次）
TestUpdateProgressAndStalledSourceFallback               8.46s  update_test.go:617（64KB 分块，每块真延迟 220ms）
TestR56ReadyCheckAcceptIsOncePerWindowAndResetsForNext   4.56s  r56_test.go:36-41（9 次真 time.Sleep(500ms)）
TestSGPConnectionErrorsHaveStableKinds（4 子测试）        4.05s  真 backoff，sgp_api.go:596-601 的 250ms/750ms
TestConvenienceReconnectPostsAndNotifies                 3.01s
```
合计约 38s / 66s。`-race` 版本为 90s。

**改法**：给 `waitSGPRetry`、ready-check 去重窗口、总览预算这三处注入可替换的时钟/sleep 函数，
预计可把整套压到约 30s。**不要**用缩短真实 sleep 时长的方式糊过去 —— 那会让这些测试失去时序敏感性。

**验收判据**：重构后 `go test -json .` 的 pass 数不变，这 5 个测试的 `Elapsed` 显著下降。

**风险**：低。

---

## P2-7 Windows 专属文件的自动化覆盖缺口

根模块无对应 `*_windows_test.go` 的：`accept_window_windows.go`、`process_windows.go`、`update_platform_windows.go`。
`installer/` 模块 7 个 `*_windows.go` 里只有 `handoff_windows.go` 有测试；
`install_windows.go`（6 个函数）、`dialog_windows.go`（5 个）、`execute_command_windows.go`、
`main_windows.go`、`webview_windows.go`、`window_windows.go` 全无。
已确认这些文件交叉编译与 vet 都干净（不是编译风险，纯覆盖缺口）。
而且因为 P0-2，连**已存在**的那些测试也从未在任何地方执行过。

**改法**：先把 P0-2 的 CI 跑起来（`windows-latest` 作业），让现有测试至少有机会跑；
再照 `handoff_windows_test.go` 的模式给 `install_windows.go` 和 `dialog_windows.go` 补冒烟测试。
这两个文件正是历史上 AST 守卫绕过事故发生的区域。

**验收判据**：`GOOS=windows go test ./...`（需 Windows runner 或 CI）对这些文件显示非零测试数。

**风险**：低。

---

## P2-8 客户端没运行时，lockfile 扫描仍然每轮全量 stat

**证据** `lcu.go:406-427`：
```go
	query, commandErr := leagueProcessCommands()
	lines := query.CommandLines
	...
	lockfiles := lockfileCandidates(lines)
	report.LockfilesChecked = len(lockfiles)
	for _, path := range lockfiles { if info, err := os.Lstat(path); ... }
```
这个循环无条件执行，不看 `query.ProcessCount`。而 `leagueProcessCommands()` 自己**已经**对
`ProcessCount == 0` 做了短路（跳过慢速 PowerShell 兜底），lockfile 扫描却没跟上同样的判据。

**影响**：客户端完全关闭时，每 3~8 秒的发现重试都要对完整候选清单（跨盘符，数百条路径）做一遍文件 stat，
纯粹为了再确认一次"还是没开"。空闲机器上的低度浪费。

**改法**：`query.ProcessCount == 0` 时整段跳过 `lockfileCandidates` 与扫描循环。
**先确认**没有任何有效发现路径依赖"零匹配进程但 lockfile 存在"（理论上不存在——有 lockfile 就必有进程），再改。

**验收判据**：假 `leagueProcessCommands` 返回 `ProcessCount: 0` 时，断言 lockfile 扫描路径未被调用。

**风险**：低。

---

## P2-9 连接管理状态机没有任何行为测试

全仓对 `connection_manager|runConnectionManager|runConnectedSession|discoverLCU` 的测试文件检索只有一处命中：
`r55_test.go:308-314`，而它做的是
```go
	source, err := os.ReadFile("connection_manager.go")
	if !strings.Contains(string(source), "go a.primeWatchState(client)") { t.Fatal(...) }
```
—— 源码文本 grep，不是行为测试。`runConnectionManager`（`connection_manager.go:120-167`）、
`discoverLCUDetailed`（`lcu.go:399-462`）、`runConnectedSession`（182~487 行）的实际运行时行为零覆盖。

**影响**：退避逻辑回归（比如忘了在成功后把 `backoff` 重置回 `minimumDiscoveryBackoff`，`connection_manager.go:161`，
或破坏了 `waitForDiscovery` 经 `a.refreshRequests` 的提前唤醒）会静默通过全部测试。

**改法**：新增 `connection_manager_test.go`，用 httptest 假 LCU 覆盖三件事：
(a) 退避从 3s 翻到 8s，成功发现后重置回 3s；
(b) WS 断开但 `client.probe()` 仍成功时，只重试事件流、**不**把连接状态翻成"未连接"；
(c) `manualDisconnected` 暂停路径在 `refreshRequests` 触发前不做发现。

**验收判据**：新测试断言的是真实状态迁移与时序，不是字符串存在性。

**风险**：无（纯新增测试）。

---

## P2-10 隐私护栏的文件清单没有文档化，且是字面量匹配

`quality_test.go:521-561` `TestDiagnosticSourcesExcludeStableIdentifiers` 对 4 个文件
（`gameplay.go` / `lp_tracker.go` / `match_timeline.go` / `sgp_api.go`）做整文件字面量扫描，
禁止 `"puuid":`、`"accountHash":` 等出现。整文件、位置无关，已经比单函数检查强。
但有两个结构性盲区：(a) 字面量匹配，动态拼接（`"pu"+"uid"`）或走变量即可绕过而运行时行为不变；
(b) 范围是**枚举的文件清单**，同样的诊断发射模式出现在第 5 个文件里就是零覆盖。

**改法**：不急着重写，但至少在测试里加注释说明"新增任何发射诊断的文件都必须加进这个清单"（目前没有这句话）；
更好的是加一个伴随测试：遍历所有含 `recordDiagnostic(` 调用的 `.go` 文件，而不是读硬编码清单。

**验收判据**：给一个新文件加上带 `puuid` 字段的诊断发射，确认当前测试**抓不到**（演示盲区），再决定要不要补。

**风险**：低。

---

## P2-11 前端渲染的常数放大器（做完 P0-7 后收益会自动缩小，优先级排在它之后）

- `web/gameplay.js:5384-5402` `applyRenderedMetricStyles` 在整棵新子树上跑 **4 遍** `querySelectorAll`
  （`[data-bar-width]` / `[data-blue-share]` / `[data-win-rate]` / `[data-skill-count]`）。
  纯写不读，**不触发强制同步重排**，但可以合并成一次多选择器查询 + `switch`。
- `web/gameplay.js:5340-5377` `prepareImages` 是第 5 遍扫描，并给每个 `<img>` 挂 2 个监听。
  实测 17 场总览有 **356 个 `<img>`**（19.2 个/场），按 200 场推算约 4300 个 → 一次全量重建挂约 8600 个监听。
  监听是 `{once:true}` 且挂在会被替换的节点上，**不构成泄漏**。
  改法：`container` 上做一次捕获阶段委托（`load`/`error` 不冒泡但可捕获），去掉逐图绑定。
- `web/app.js:2019-2038` `restoreSectionScroll` 用 rAF 循环最长 2 秒、每帧读 `scrollTop`（强制 layout flush）再写 `scrollTo`。
  **已有保护**：`target <= 0` 时提前返回、四个监听都是 `{once:true}` 且三条退出路径都调 `stop()`、
  `state.section !== name` 会中断——**不是监听泄漏**。问题是内容高度还没到位时会满 2 秒跑完 120 帧，
  正好叠在切页面最卡的那一刻。改法：换成 `ResizeObserver` 驱动，只在高度变化时尝试一次。
  **必须保留**注释里写明的"加载态更矮导致 scrollTop 被夹回 0"这个语义，以及"用户一动滚轮就放弃"。
- `web/gameplay.js:5601-5603` 玩家页签的 scroll 监听每次都 `getComputedStyle` + 写 `hidden` 后又读
  `scrollLeft/clientWidth/scrollWidth`（975~988 行），构成写后读强制同步布局，且未节流。
  这是全仓**唯一**一处这种模式，改法照抄 `app.js:3482-3490` 已有的 rAF 节流范式。

**验收判据**：一次渲染里 `addEventListener` 调用数 < 1000（当前约 8600）；
切页面后 `restoreSectionScroll` 的 rAF 执行帧数 ≤ 3（当前可达 120），同时保留"位置真的被还原"的功能断言；
连派 20 个 scroll 事件，`getComputedStyle` 调用 ≤ 2（当前 20）。

**风险**：低。

---

## P2-12 零散一致性问题

- `web/gameplay.css:701` `.match-team-list button:disabled { cursor: not-allowed; }` —— 只改了鼠标指针，没有变暗。
  同文件其他所有 `:disabled` 都配了 opacity（`:61` 0.35、`:554` 0.78、`:740` 0.62、`:984` 0.62）。
  战绩卡里禁用的队友按钮对键盘/触屏用户完全没有视觉反馈。建议补 `opacity: .62` 与语境最近的 `.match-actions button:disabled` 对齐。
- `storage.go` 的快照清理 `pruneSnapshots`（453~469 行）只有**数量**上限（`maxSnapshots = 30`），没有总字节上限；
  单个快照读取上限 8MB、奖池条目上限 10000，理论上 30 份大快照能到几十 MB。建议顺手加一个总字节上限。
- `main.go:1601-1622 loadOrCreateSessionToken` 用裸 `os.WriteFile`，
  而全仓其他落盘路径都走 `atomicWriteFile`（`storage.go:161-192`，temp + fsync + rename + 目录 fsync）。
  **影响很小**：`isSessionToken`（1624~1634 行）会校验 48 位十六进制格式，格式不对就当"文件不存在"重新生成，
  不会导致启动失败，只是极端情况下用户要重走一次 bootstrap 链接。顺手改成 `atomicWriteFile` 保持一致即可。

**风险**：低。

---

## P2-13 ⚠️高风险：把总览首屏契约拆开（建议排在 P0-6 之后单独一轮）

`gameplay.go:1318-1338` 把熟练度、近 20 场排位样本、activityHours、recentPlayers 全塞进同一个 `respondJSON`。
而项目里**已有**"首屏返回后再补"的现成范式：`main.go:474-476` 的 `/api/gameplay/match-tiers`、
`current-game`、`season-summary` 三个独立端点，以及赛季统计走 `startSeasonStatsRefresh` + SSE `season-progress`
异步补齐（`season_stats.go:500/545-546`）。

熟练度（1 个 LCU 全英雄请求）和近 20 场排位样本（2 个 SGP 请求）都是侧栏/下方面板内容，
却挡在"右侧战绩列表"这个真首屏前面。**这部分收益与 P0-6 的并发化不重叠**（并发后总耗时取 max，移出后 max 变小），
预计再省 0.3~0.8s。

**改法**：新增 `POST /api/gameplay/profile-extras`（熟练度 + 近期排位样本 + positions/ability/rankedQueues），
首屏 overview 只返回 player/matches/pagination/ranks/capabilities。
复用现有 `overviewQueryCache`（`overview_cache.go:71`）给新端点做同样的 2 秒快照 + singleflight。
**注意** `buildGameplayRankedQueues` 与 `buildGameplayAbilityProfile` 依赖 ranks，所以 ranks 必须留在首屏
（它本来也是并发的一路，不额外增加成本）。

**为什么高风险**：改响应契约，前端渲染顺序、骨架屏、`Capabilities` 聚合（设置页读它）全部要跟着改。

**验收判据**：断言首屏 `/api/gameplay/overview` 的 `masteries`/`recentRanked` 为空且 `capabilities` 不含
`champion-mastery`；用假上游注入延迟，断言首屏只等战绩 + 段位两路（不含熟练度延迟）。

---

## P2-14 ⚠️需用户拍板：更新链路的信任根不完整

**证据**：
```
update.go:26            var updateMirrors = []string{"", "https://ghfast.top/", "https://gh-proxy.com/", "https://ghproxy.net/"}
update.go:401-417       fetchManifest 把 prefix+updateManifestPath 依次发给这些第三方镜像
update_download.go:158  downloadSource(ctx, asset, prefix+asset.URL, part) 下载资产同样套镜像前缀
update_download.go:80-85 verifyUpdateDigest 只比对 SHA256 —— 而这个 SHA256 本身来自同一个（可能被替换的）镜像返回的 latest.json
```
`validateUpdateManifest`（`update.go:248-267`）虽然强制 `asset.URL` 的 host 必须是 `github.com`、路径精确匹配，
但**请求实际是发给镜像服务器的**，由它决定回什么内容。所以恶意/被劫持的镜像可以同时伪造清单里的 SHA256
和被下载的字节，两者自洽，`verifyUpdateDigest` 无法识破。清单本身没有开发者签名，是纯自证。

**影响**：直连 GitHub 失败时（国内常见）会落到这三个第三方代理。一旦其中之一被劫持，就是完整的供应链投毒。

**改法（需拍板）**：给 `latest.json` 加开发者侧 Ed25519/GPG 签名、校验公钥内置在客户端，与传输通道解耦；
或至少拆分信任——清单必须直连（或走自己可控的 CDN），只有大文件下载才走镜像。
**这条涉及发布流程改造（要多一步签名），请用户先确认是否要做、以及愿意接受多大的流程复杂度。**

**验收判据**：构造一个恶意镜像（合法 URL 格式 + 自算 SHA256 + 同镜像上放篡改过的 exe），
确认当前代码会接受安装（先复现问题），加签名后同样构造必须被拒绝。

**不做的影响（维持现状）**：这是三个待拍板事项里**唯一涉及真实安全风险**的一条，但要分清"理论风险"和
"会真实发生的概率"——不做不代表现在就不安全，而是"依赖三个第三方镜像服务商不作恶、不被攻破"这个假设成立才安全。
除非 ghfast.top / gh-proxy.com / ghproxy.net 中有一个真的被劫持或主动作恶，否则日常使用不会触发任何问题；
一旦触发，后果是给走镜像更新的用户机器装上被篡改的安装包（供应链投毒），属于"低概率、高后果"类型。
这条本质是"要不要为了防一个不常发生但后果严重的事，给发布流程加一道签名步骤"的权衡，
如果应用用户量不大、且这三个镜像本身是业界常用的可信代理，现阶段搁置也是合理选择，
但建议**不要永久搁置**，等发布流程有精力改动时优先处理这条，而不是归为"不用管"。

---

## P2-15 ⚠️需用户拍板：.git 历史里有约 293MB 误提交的 .gomodcache

实测 `git rev-list --objects --all` + `git cat-file --batch-check`：全部历史 blob 共 436MB，
其中 **`.gomodcache/` 相关 blob 有 11,731 个、共 292,997,997 字节（约 293MB）**——
某次提交把完整 Go 工具链（含 79MB 的 toolchain zip、19MB 的 compile 二进制）连带 `git add` 进去了。
另有 `deep-legends.exe`（19.7MB）和一批 `deep-legends/__TEXT__rodata` 之类的 Mach-O 分段文件（约 15~20MB），
都在 `ff3cf27 Expand Deep Legends application features and content` 这个提交里。
这是 `.git` 目录 328MB 的主因。`.gitignore` 现在已经覆盖了这些路径，但 gitignore **不会追溯清除已提交的历史对象**。

**改法（需拍板）**：
```
git filter-repo --path .gomodcache --path deep-legends.exe --path deep-legends \
                --path lol-loot-assistant-go-tmp-umask --invert-paths
git gc --prune=now
git push --force
```
**为什么需要拍板**：这会重写全部 5 个提交的 SHA，而这 5 个提交已经推到了 `LLYY0418/Deep-Legends`
（0912 晚首次 Release 时推的），需要对 origin 强推。请用户确认。

**验收判据**：`du -sh .git` 从 328M 降到几 MB 量级；GitHub 上的仓库体积同步下降；`go build ./... && go test ./...` 仍绿。

**前置条件**：必须先做完 P0-1（把工作区落进 git），否则重写历史会把未跟踪的 225 个生产文件一起搞丢。

**不做的影响（维持现状）**：纯仓库体积问题，**不影响功能、不影响安全、不影响任何用户使用这个应用**。
`.git` 目录会一直带着这约 293MB 的历史包袱（总共 328MB），每次 clone、每次备份都要多传这部分数据。
如果这个仓库只有维护者自己在用、暂时不打算给别人 clone 或协作，可以无限期不处理，没有任何功能性代价。
真正会促使处理这条的场景是：仓库要给其他协作者 clone/fork、或者在意 GitHub 上显示的仓库体积、或者磁盘紧张。
**代价随时间推移会变大**——如果之后又有人 clone 或 fork 过这个仓库，再重写历史强推会让他们的本地历史和远端产生冲突；
现在（还没有其他协作者/fork）是操作成本最低的时机，越往后拖，强推的连带影响越大。

---

# 附录 A：本轮已确认**没有问题**的地方（别在这里花时间，也别顺手"优化"掉）

**后端并发与缓存（已做对）**
- 总览顶层请求去重：`overview_cache.go:41/154` 有 2 秒快照 + singleflight，
  且 `clearOverviewQuerySnapshots`（195 行）用"换 map 而非清 map"正确摘掉旧 flight。
- 段位分数：`rank_insights.go:160-177 + 209-226` LRU + singleflight，抢到 leader 后**又复查了一次缓存**，
  把 get/beginFlight 之间的竞态关好了；负结果也缓存。
- LCU 逐场详情：`gameplay.go:2757-2797` 4 并发 + 信号量 + ctx 取消，且只对 roster 不完整的场次补详情，不是 N+1。
- 韩服总览：`riot_api.go:1006-1025` 已把段位/熟练度/OP.GG 历史段位三路并发，1030 行又对 20 场做 4 并发。
- asset 缓存：`asset_cache.go:23-74` singleflight + 负缓存 TTL + generation 代际校验 + 字节/条目双上限。**全仓最完善的一层**。
- champion-data 缓存：`champion_cache.go:91-186` 内存 + 磁盘 + 陈旧兜底 + singleflight，flight 加入者会复查缓存。
- 当前对局（韩服）：`overview_current_game.go:421-479` 10 秒缓存 + singleflight + 并发上限 2 + "取消不入缓存"。
- 职业选手页：`pro_players.go:159-246` flight + TTL + 失败 backoff + 渐进式发布，
  且刻意用 `context.Background()` 避免一个用户离页取消掉共享加载。
- SGP 令牌：`sgp_api.go:287-315/341-363` 90 秒 TTL + 按 client 绑定。网关发现：`sgp_gateway.go:87-92` 1 分钟缓存。
- `copy := *p.http` 那 5 处（`sgp_api.go:663/1136`、`sgp_gateway.go:73`、`pro_players_supplement.go:50`、
  `pro_players_ladder.go:154`）都是浅拷贝只为改 CheckRedirect/Jar，**Transport 指针共享、连接池确实在复用**，别当缺陷。
- 赛季统计已经从同步响应里摘出去了（`season_stats.go:455-484` network-free + 三重去重闸门）。
- HTTP 路由层无全局锁：`main.go` 里所有 `a.mu.Lock()` 临界区都不含 I/O。
- 诊断噪声抑制：`main.go:1366-1392` 按内容去重 + 每 100 次才写一条汇总。

**前端（已做对）**
- fetch 竞态：五个模块的 `api()` 都有"同 key 先 abort 旧请求 + 超时 abort + 响应落地前二次校验 + finally 清理 Map"四件套，
  `gameplay.js:159-219` 是最完整的一份（188 行拦住迟到响应覆盖新状态，218 行防 controller Map 无限增长）。
- SSE：`app.js:3170-3175` 每次 setup 先 clearTimeout + close，所有 handler 有身份校验，1s→30s 指数退避。不会多条并存。
- 定时器/Observer 泄漏：`gameplay.js:5890-5909 disposeGameplay` 清 8 个 timer + per-tab timer + 全部 AbortController + 7 个 Observer；
  `suite.js:1881-1894`、`friends.js:297-298/401` 同样配对正确。
- 切页面不重建 DOM：`app.js:2063-2081 activateSection` 只切 `panel.hidden`，实测各 section 切换同步耗时 2.7~14.9ms。
  这是刻意选择（注释 2079~2080 行说明了原因）。
- 切页面不重拉已有数据：`loadSkins` 10 分钟缓存、`loadOverview` 在 `tab.data && !force` 时直接 rerender。
- 图片：总览 356 个 `<img>` 里 322 个 `loading="lazy"`、356 个 `decoding="async"`；34 个 eager 的经核查全部合理。
  虽无 `width`/`height` 属性，但 `gameplay.css:953/960/961` 给 `.game-icon` 硬写了尺寸，**不存在图标造成的布局抖动**。
  43 个唯一 URL 被复用 356 次，后端给了 `Cache-Control`（`main.go:905/930`、`champions.go:917`），浏览器会去重。
- 收藏页大列表是全仓最好的一块，可当范本：分批 `runRenderTasks` + `renderGeneration` 世代号 +
  离页 `cancelRenderFrames` + IntersectionObserver 620px 预取边界。
- 循环内强制同步重排：17 个 `getBoundingClientRect/offsetWidth` 调用点逐个看过，无一在大列表循环里。
  唯一一处循环内调用（`gameplay.js:2613-2618`）只在无 IntersectionObserver 的兜底分支，Electron/Chromium 永不走到。
- tooltip：700 个 `[data-tooltip]` 节点只有 `document` 上 6 个委托监听，零逐节点绑定，position 由 rAF 合并。
- 骨架屏永久卡住已有三层护栏（`gameplay.js:1446-1453` try/catch 转错误态、
  `app.js:522-529` 25 秒强制撤遮罩、champions loader 的 finally 复位）。
- `app.js:346-348` 的全局 MutationObserver：做过关闭对照实验，68 场下开/关耗时无差异（回调走微任务，
  不在点击的同步窗口内）；`enhanceNativeSelect` 有幂等闸门，旧 select 可被 GC，**不是泄漏**。不建议单独排期。

**LCU 状态机（已做对）**
- WS 订阅覆盖度：`lcu_events.go:82` 订阅的是 `[5, "OnJsonApiEvent"]` **全量 firehose**，
  gameflow/champ-select/ready-check/honor/lobby/matchmaking 全部经 `handleEvent`/`handlePhase` 直达。
  **不存在"本可订阅却在轮询"的状态。**
- 选人自动化实时性：ban/pick 由 WS 事件直接触发（`connection_manager.go:256-270`），
  **没有**被 300ms UI 广播防抖拖慢（那个 debounce 只用于 UI 推送）。
  1 秒 `champSelectPoll` 是倒计时数学必需的内存态兜底，选人阶段之外不做 HTTP。
  `champselect.go:521-529 champSelectDelay` 保证 `configuredMS` 永不超过 `remainingMS`。
- 身份切换级联：`summoner_identity.go:67-93` 正确检测换号并调 `clearGameplayReferences` + `requestCollectionRefresh`，
  历史上的"整包快照导致身份陈旧"**仍然是修好的状态**。
- 错误降级：`main.go` 的 `refreshWithClient` 用 `client.probe() == nil` 把断连判据和 4xx/5xx 分开；
  `connection_manager.go:458-467` 的 WS 错误分支同样只在 probe 也失败时才升级为整个会话结束。
- 重连追赶：每次 WS（重）连都会 `primeWatchState`（200~206 行，函数体 612~638 行）重读 gameflow-phase 并重跑 handlePhase，
  断线期间漏掉的阶段迁移会在恢复瞬间补上。
- 启动客户端后的重试节奏：`client_launcher.go:196` 成功启动后立即 `requestRefresh()` 唤醒 `waitForDiscovery`，
  实际落点约 0s/3s/9s/17s，对约 10~20 秒的 LCU 启动窗口是合理的。
- 安全卫生：LCU 令牌从不记日志且 Close 时清空（`lcu.go:863-871`）；
  `watch_rules.go:1211-1220 secureRandomMember` 用 `crypto/rand`；accept 焦点采样明确不阻塞实际 accept 请求。

**存储与安全（已做对）**
- 本地 HTTP 服务是教科书级分层防护：`main.go:297` 默认绑 `127.0.0.1:0`、
  `546-549` 启动时二次校验 `IsLoopback()` 否则 `log.Fatal`、
  `583-595` 要求 `X-Local-Token` 或 cookie 精确匹配 192bit 随机会话令牌、
  `622-631` cookie `HttpOnly + SameSite=Strict`、
  `636-641` 用 `Sec-Fetch-*` 防跨站骗取 bootstrap cookie、`644-651` CSP `default-src 'self'`、无 CORS。
- 静态资源零路径穿越：`static_assets.go:27-40` 启动时把 embed.FS 枚举进 map，请求时直接查表，
  **从不**把请求路径拼进文件系统路径。
- Riot key 闸门链路完整：`riot_api.go:62-113` AES-256-GCM + `sync.OnceValue`；
  `desktop/verify-embedded-riot-key.cjs:34-48` 同时扫密文特征与 `RGAPI-` 明文特征，public 模式两者任一出现即抛错；
  `desktop/release-build.cjs:39-53` 还要求 release-build.json 收据 + schema + mode + 指纹 + 二进制哈希全部吻合。
  **任何改动这条链路的提交都要重点复核。**
- 原子写入：`storage.go:161-192 atomicWriteFile` = temp + Write + Sync + Close + Rename + 父目录 Sync，
  两层 fsync；所有读取路径在 JSON 解析失败/哈希不符时都是"丢弃该条目或整体重置"，**不会 panic 也不会阻塞启动**。
- 诊断日志轮转：2MB 触发、5 代上限、读取时超 `2MB+64KB` 直接拒绝（`errResponseLimitExceeded`），总量硬顶约 10MB。
- 磁盘缓存总量：champion-data 64MB/256 条、prestige-artwork 192MB/512 条/45 天（还带
  `http.DetectContentType` 图片格式白名单）、asset cache 256MB **纯内存不落盘**。
  理论磁盘上限约 256MB，**不会把用户磁盘写满**。
- LP 历史隐私：`lp_tracker.go:54-59` 落盘的是 `AccountHash`，`storage.go:339-351` 是 `salt + PUUID` 的
  SHA-256 前 16 字节十六进制，单向不可逆；`lp_tracker_test.go:79` 有字段黑名单测试。
  与 `features.go:656` 的 `neverStores` 声明**一致**。
- 前端 localStorage：全部统一 `lol-loot-` 前缀，只存折叠状态/界面偏好/demo 开关，无敏感信息、无体积失控。
- `riot_key.local.txt`（根目录，43 字节）**从未被 git 跟踪**（`git log --all --` 空），
  `.gitignore` 的 `riot_key.local.*` 规则有效，全仓 `RGAPI-` 的其余命中都是测试里的假 key。**无需任何动作。**
- LCU 用 `InsecureSkipVerify` 连 `127.0.0.1` 自签证书是正常设计，不是漏洞。

**测试与构建（已做对）**
- **所有联网测试都正确门禁化了**：`pro_runes_live_test.go`、`overview_current_game_test.go:159-161`、
  `diagnostics_2135_test.go:84-86`、`diagnostics_1945_regression_test.go:233-234`、`opgg_player_page_test.go:139`、
  `pro_players_test.go`×2、`pro_players_supplement_test.go`、`hexdata_test.go:491`、
  `item_depth_completeness_test.go:188`、`opgg_season_summary_test.go:141`、`r73_test.go:192`
  全部在 `t.Skip` + 环境变量（`R75_LIVE` / `R75_CAPTURE` / `OPGG_CURRENT_GAME_PROBE` / `OPGG_2135_PROBE` 等）之后，默认跳过。
- 无死测试：无 `if false`、无被注释掉的 `func Test...`、无缺 reason 的孤立 skip。
- JS/desktop 测试无真实网络调用。
- `installer/` 的 AST 守卫（`message_wiring_test.go` / `completion_wiring_test.go` / `handoff_windows_test.go`）
  用 `go/ast` 解析真实源码并比对重排后的语句块，**守的是分派边界而不是函数名白名单** ——
  这是历史绕过事故之后重新设计的，确认仍然有效，**是本项目里护栏写法的正面范本**。
- **按轮次命名的测试没有冗余**：逐一读过 `r29_test.go`~`r79_test.go`（19 个）与
  `diagnostics_HHMM_test.go`（12 个）里约 150+ 个测试函数签名，指向的都是各不相同的具体历史场景，
  没有两份文件测同一件事，也没有测已删除行为的。命名不利于检索是另一回事，**不要以"冗余"为理由删它们**。

**样式（已做对）**
- `.live-player` 网格没有重演"图标被挤扁"：`gameplay.css:1021` 用 `minmax(100px,1fr)` 而非裸 `1fr`，
  且 `:1025-1027` 三个后代都设了 `min-width: 0`，长玩家名走 ellipsis 而不挤压相邻列。
- `.app-select-options` 正确修复了历史"grid auto 行不吃 max-height"：`app.css:471` 有 `flex: 1 1 auto; min-height: 0; overflow-y: auto`，
  父级是 `flex-direction: column` —— 这正是 R65 那个 bug 的正确修法。
- 暗色模式不是半成品：全仓 `light-dark(` 零命中，走的是显式 `[data-theme="dark"]` 整套变量覆盖 +
  `prefers-color-scheme` 兜底，且 `champions.test.cjs:3107` 有对具体变量值的回归断言。
- `focus-visible` 由 `app.css:197` 一条规则统一覆盖 button/input/select/summary/a/[tabindex]，无遗漏。
- 窄断点（700/620/480/380px）不是死代码：`desktop/main.cjs:420-421` 强制 `minWidth: 780`，
  这些更窄的断点是给"直接用浏览器访问后端端口"这个真实使用场景准备的。
- `select-menu-popover` / `quality-popover` / `mythic-submenu` 缺 `max-height` 不是遗漏：数据源是硬编码的有限枚举，长度有界。

---

# 附录 B：需要用户拍板的三件事

三条都不是"不做就会立刻出问题"的类型，都是"做了更好，不做也能正常运转"，但代价性质不同，按建议关注度排序：

1. **P2-14 更新清单签名**（安全相邻，唯一涉及真实风险）—— 不做不代表不安全，而是依赖三个第三方更新镜像
   （ghfast.top / gh-proxy.com / ghproxy.net）不作恶这一假设成立。日常不会触发任何问题，一旦镜像被劫持则是
   完整的供应链投毒（低概率、高后果）。建议**不要永久搁置**，列为长期待办，等发布流程有精力时优先处理这条。
2. **P2-15 重写 git 历史**（回收约 293MB）—— 纯体积问题，不影响功能/安全/任何用户使用。
   只有仓库要给别人 clone/fork、或在意仓库体积时才需要处理。**现在处理成本最低**（还没有其他协作者/fork），
   拖得越久，将来强推对协作者的连带影响越大。可以搁置，但不建议无限期搁置。
3. **P1-15(c) `app.setName("Deep Legends")`**（体验问题，影响最小）—— 只影响用户手动翻日志文件夹时找不找得对地方，
   不影响应用本身运行。P1-15 的 (a)(b) 两步（导出诊断日志含外壳日志、日志轮转）与此无关，建议照做；
   (c) 可以无限期搁置，真要做时必须先处理 userData 路径迁移，否则会造成"升级后设置像是丢了"的更差体验。


# 附录 C：记忆更正

`deep-legends-lp-history-puuid.md`（"待修：lp-history 存了 PUUID，与 neverStores 隐私声明冲突"）
**这条已经不成立了**：本次复核 `lp_tracker.go:54-59` + `storage.go:339-351`，落盘字段是 `AccountHash`
（`salt + PUUID` 的 SHA-256 前 16 字节），单向不可逆，且有 `lp_tracker_test.go:79` 的黑名单测试守护，
与 `features.go:656` 的声明一致。下次整理记忆时应标记为已解决。
