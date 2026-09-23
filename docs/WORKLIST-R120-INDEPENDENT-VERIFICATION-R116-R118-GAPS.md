# WORKLIST-R120：R116 / R118 独立验收遗留的真实缺口

**撰写日期：** 2026-09-21　**基线版本：** 0.12.13（`desktop/package.json`；含 R119 的未提交改动）
**触发：** 独立会话补做了 R116（探测 / A / B / D / E / F）与 R118 的验收，证据与复现命令见 `docs/r116-independent-verification.md`、`docs/r118-independent-verification.md`。本工单只收敛其中**仍未修复**的条目。
**本轮性质：** 纯审查产出，未改动任何代码文件。每条都带证据、复现和验收判据。

**范围说明：** R116 六张工单的功能与数据准确性经复核成立（前端 9/9 变异被拦；F 的阶段×稀有度分布独立重算一致），**不需要返工**。下面全部是「护栏没拦住」「测试不稳」「一处既存生产竞争」「两处待你拍板」，不是功能没做。

## 0. 结论摘要

1. **R119 的 `gameplay.go` 没过 gofmt，CI 第一步会红**（一行对齐，最快的一条）。
2. **CI 真机护栏的「不执行 / 失败被吞」这一类绕过没有堵死**：R118 补了 job 级两种，我换 10 种形状测试全部仍通过，其中 5 种是真实绕过。
3. **两个真 Chromium 脚本会在打印 PASS 之后以退出码 1 结束**（清理阶段 `ENOTEMPTY`），会让 CI 在护栏其实通过时变红。
4. **`loadStructuredDetail` 有一处生产代码里的真实数据竞争 + goroutine 泄漏**（HEAD 上就有，非 R116 引入）。
5. **`go test -race ./backend` 整包不稳定为绿**：至少 3 个随机失败源。
6. **`promote` 那条测试的保护被夸大**，赛季缓存预算与内层队列门禁缺测试。
7. **两处需要你拍板**：`season-stats/` 的隐私声明；R116 真机观测值回填。

## 执行纪律（延续 R117 / R118）

- 每项的「验收判据」都必须做**对抗变异**：改回原样、或换一种同等效果但语法不同的写法，对应测试必须 FAIL。只跑现有测试 PASS 不算验收。**改回原样只证明没被这一种退化绕过，不等于堵住了这一类**，验收时主动设计与执行方自述不同形状的变异。
- 变异一律在隔离副本（非 git 工作目录）里做；还原用「整文件从基线拷回 + `diff -q`」，**禁止 `git checkout -- <file>`**。
- **整包 `go test -race ./backend` 至少跑 2 次**，把红的项归类为「已知随机失败（本工单 P2-2 列表内）/ 新失败」；单次绿不算稳定绿。
- 不动隐私文案：P1-4 的文案由你拍板后才写进 `features.go`（R92 教训）。

## 建议执行顺序

P1-1（一行）→ P1-3 → P1-2 → P2-1 → P2-2 → P2-3 / P2-4 → P3。P1-4 等你拍板，不阻塞其它条目。

---

# P1

## P1-1　`backend/gameplay.go` gofmt 未通过（R119）

**证据：** `gofmt -l backend/*.go` 只列出 `backend/gameplay.go`。`gofmt -d`：约 500 行，`Autofill bool \`json:"autofill,omitempty"\`` 需改成 `Autofill  bool ...`（与下一行 `reference gameplayReference` 对齐）。
**影响：** `.github/workflows/ci.yml` `quality` 第一步就是 gofmt 检查，推上去 CI 立刻红。
**修复方向：** 运行 `gofmt -w backend/gameplay.go`，不做其它改动。
**验收判据：** `gofmt -l backend/*.go` 无输出；`git diff --stat` 只涉及该文件且只有对齐空白的变化。

## P1-2　CI 真机护栏：黑名单式断言挡不住「同类不同形」的绕过

**证据：** `backend/web/r117.test.cjs:160–` 的测试是「已知坏字段黑名单 + 命令行精确白名单」。R118 补了 `jobs.quality.continue-on-error` / `if`，台账的 6 种变异复现全部 FAIL。但下面 5 种在隔离副本里测试仍 **PASS**（护栏实际不执行或失败被吞）：

| # | 变异（改 `.github/workflows/ci.yml`） | 为什么是真绕过 |
|---|---|---|
| N1 | 护栏步骤加 `shell: bash {0}` | 去掉默认 `-e`；r100 失败、r117 通过 → 步骤绿 |
| N2 | `jobs.quality.defaults.run.shell: bash {0}` | 同上，作用到整个 job |
| N5 | workflow 级 `defaults.run.shell: bash {0}` | 同上，作用到整个 workflow |
| N3 | `on:` 只保留 `workflow_dispatch` | push / PR 不再触发 workflow，与 P1-2 最初根因同构 |
| N4 | `on.push.branches` / `on.pull_request.branches` 设成永不匹配 | 同上 |

另有一条读代码确认的形状：`desktop/r100-browser.cjs` 读 `R100_APP_SOURCE` / `R100_QUEUE_SOURCE`；CI 给这两个变量指向桩文件，护栏就在测假的源码，测试同样拦不住。
**修复方向：** 不要继续追加黑名单条目，改成「护栏处于默认执行路径」的白名单/结构不变量，加进现有测试：
1. `workflow.on` 必须含 `push` 与 `pull_request`，且二者都没有 `branches` / `paths` 过滤；
2. `workflow.defaults`、`jobs.quality.defaults`、护栏步骤都不能有 `shell` 字段；
3. `jobs.quality` 除 `runs-on` / `steps`（以及既有的 `needs` 类合法字段）外不得有 `if` / `continue-on-error` / `strategy` / `defaults` / `timeout-minutes` 等控制流字段，用**显式允许清单**而不是禁止清单；
4. 护栏步骤只允许 `name` / `run`（`env` 需显式允许清单，且禁止出现 `R100_*_SOURCE` 之类覆盖变量）。
**验收判据：** N1 N2 N5 N3 N4 五种变异各自使 `R117 real Chromium guards are wired into CI and cannot silently skip` FAIL；再加一个「护栏步骤 env 里设 `R100_APP_SOURCE`」的变异也 FAIL；R118 的 6 种既有变异（`jobs.quality.continue-on-error` / `if`、`|| echo`、`|| :`、`; true`、`set +e`）保持 FAIL；未变异控制组 PASS。

## P1-3　真 Chromium 脚本打印 PASS 后退出码 1

**证据：** `desktop/r100-browser.cjs:88` 与 `desktop/r117-browser.cjs` 末尾：
```js
main().catch(e=>{console.error(e);process.exitCode=1;}).finally(()=>{ws?.close();proc?.kill();...;fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100});});
```
`proc.kill()` 之后 Chrome 子进程还在写 profile 目录，`rmSync` 重试 0.8 秒仍可能失败，抛出的 `ENOTEMPTY` 从 `.finally` 变成未处理拒绝，退出码 1。云端沙箱（非 root、隔离目录）实测：r100 6 次里 4 次、r117 5 次里 3 次「`... Chromium PASS {...}` 后 `Error: ENOTEMPTY: directory not empty, rmdir '.../Default'`，exit=1」。
**影响：** CI 步骤默认 `bash -e`，护栏实际通过时也可能变红；人会习惯性重跑或加 `|| true`，正是 P1-2 一直在防的事。GitHub runner 上的发生率未验证，**请在真 runner 上看一次实际日志**。
**修复方向：** 清理改为尽力而为——先 `await` Chrome 进程 `exit`（带超时）再删目录，或对 `rmSync` 包 `try/catch` 只吞清理错误；**不得吞掉主流程失败**（断言失败必须仍然非零退出）。只改这两个脚本的收尾，不改护栏断言。
**验收判据：**
- 沙箱里两个脚本各连续跑 20 次，全部 exit 0；
- 变异：让脚本内的一条真实断言失败（例如把期望 `peakImages` 改成不可能的值），脚本必须 **exit≠0**（证明没把失败一起吞掉）；
- 变异：让 `rmSync` 目标永远删不掉，PASS 仍然 exit 0 且打出一行清理警告。

## P1-4　`season-stats/` 不在隐私声明里（**需要你拍板**）

**证据：** `backend/features.go` `handlePrivacy` 的 `stores`（7 项）没有 `season-stats/`；`storage.go:118` 创建的目录还有 `updates`、`pools`、`snapshots`、`logs`、`prestige` 艺术图缓存、`championdata` 缓存，其中哪些已被声明需要一并核对。R116-E 之后 `season-stats/<source>/<accountHash>-<season>.json` 里多存了每场海斗的英雄 / 海克斯 / 装备（`AugmentSamples`）。台账已披露此项待拍板（R117 工单第 662 行、`r116-execution-ledger.md` 决策表第 10 条，建议条目原文见 `docs/r116e-execution-ledger.md` §9.2 的 A/B 两版）。
**需要你决定：** 采用 A 版还是 B 版文案；是否同时补齐其它未声明目录。**在你拍板前任何人不得改 `features.go`。**
**拍板后的执行与验收：**
1. 把选定条目写进 `stores`；
2. 加一条结构性测试：枚举 `storage.go` 里创建的每个目录，断言 `stores` 文本里有对应声明（允许有显式豁免清单，每个豁免要写理由）；
3. 变异：删掉 `season-stats` 那一条 → 测试必须 FAIL；新增一个未声明的目录 → 测试必须 FAIL。

---

# P2

## P2-1　`loadStructuredDetail`：生产代码里的数据竞争 + goroutine 泄漏

**证据：** `backend/champions_structured.go`，HEAD `4ca768ec` 上同样存在。
- 1178–1186：两个 goroutine 的闭包**直接引用外层变量** `qq101PositionsC` / `qq101BuildC`，在结果就绪时执行 `qq101PositionsC <- ...`；
- 1285–1286（以及 1270 附近）：父 goroutine 收完或超时后把这两个变量**置为 `nil`**。
- `go test -race -count=1 -run TestStructuredDetailFallsBackWhenQQ101BuildTimesOut ./backend`（整包跑时命中过 1 次，HEAD 与当前树都有同一处代码）报 `DATA RACE`：写 `1285/1286`，读 `1180/1185`。
- 后果不止是竞争：goroutine 晚到时读到 `nil` 通道，**向 `nil` 通道发送会永久阻塞**，goroutine 泄漏且结果丢失。
**修复方向：** goroutine 只捕获**局部变量**（在启动前 `positionsCh := make(...)`，闭包用 `positionsCh`，或作为参数传入），父侧用另一组「仍在等待」变量做置空逻辑，二者互不共享；通道保持带缓冲（cap 1）即可，不需要加锁。
**验收判据：**
- `go test -race -count=20 -run 'TestStructuredDetailFallsBackWhenQQ101BuildTimesOut' ./backend` 全部无 `DATA RACE`；
- 新增泄漏断言：超时路径返回后等结果晚到，`runtime.NumGoroutine()` 回到基线（允许有限重试窗口）；
- 变异：把闭包改回引用共享变量 → `-race` 或泄漏断言必须 FAIL。

## P2-2　随机失败测试要先归类、再收敛

**证据：**
- `TestProDirectoryPublishesBeforeSlowSupplements`（`backend/pro_players_test.go:317`，与 HEAD 文件一致）——整包 `-race` 红过 1 次；被测逻辑是 `pro_players.go loadProPlayers`（约 202–300）。
- `TestProDirectoryFailureRetainsRosterAndAuth`——整包负载下红过 1 次，单跑 40/40 通过。
- `TestStructuredDetailFallsBackWhenQQ101BuildTimesOut`——是 P2-1 的表现。
**要求：** 先量化复现率（在 `GOMAXPROCS=1` 与整包并行两种条件下各跑 `-race -count=100`），再判断是**测试对时序做了错误假设**还是**生产逻辑真的有竞争**；前者改测试（去掉 sleep 依赖，用同步点），后者按 P2-1 的方式修生产代码。不允许靠加大等待时间、重试或 `t.Skip` 收敛。
**验收判据：** 上述两个测试在两种条件下 `-race -count=100` 全绿；整包 `go test -race -count=1 ./backend` **连续跑 3 次**全绿。

## P2-3　`TestHexdataJSONKindsSurviveSoftTTLBecauseTheyArePromoted` 的保护被夸大

**证据：** `backend/hexdata_r116a_test.go:925–929` 注释「忘记 promote 时这里必然失败」不成立。逐个删除 `hexdata.go` 里 8 处 `p.hexdata.promote(`（1475 answer、1864 heroes、2105 meta、2654 postmatch、2871 hextech-insights、3148 augment、3180 rarity、3318 hero-json）各跑整包：只有 answer / meta 被相关测试打红；heroes 相关子集全过；postmatch / hextech-insights / augment / rarity / hero-json 整包 **1456 项全过**。原因：正常路径由 `load()` 的 `loadWithStatus(persistDisk=true)` 已写盘；`promote` 只在 `refreshBuild` 通道（`persistDisk=false`）才是唯一落盘者，而各 loader 先走 `loadHexdataMeta`，`adoptMeta`（`hexdata.go:1418`）会刷新 `BuildChecked`，该分支在这些 loader 里基本走不到（探针：把 `BuildChecked` 拨到 13 小时前，删掉 `promote` 数据仍落盘）。
**判断：** 这些 `promote` 是无害的冗余防御，不是功能缺陷。问题只是测试注释和名称承诺了它没做到的保护。
**修复方向（二选一，建议 A）：**
- **A：** 改注释与测试说明，如实写「只有 answer / meta 的 promote 由本测试与冷启动预算测试保护；其余为冗余防御」；
- **B：** 删除确认不可达的 `promote` 调用，并跑整包 + 探针确认无回归。
**验收判据：** A —— 注释里点名的「被保护项」与实测一致（answer、meta 删除仍 FAIL）；B —— 删除后整包全绿，且 `TestHexdataColdRankingBudgetAndDiskRestart`、`TestLoadMayhemDetailColdHeroCostsFourHexdataRequestsWithWarmSiteMetadata` 仍能被「删 answer/meta 的 promote」打红。

## P2-4　赛季缓存预算与内层队列门禁缺测试（R116-E）

**证据（整包均存活的变异）：**
- **GE2**：`season_stats_budget.go` 第二级处置（`augment_samples_dropped` 那一步）里顺手 `trimmed.Stats = nil` → 整包全过；
- **GE2b**：第二级一上来 `trimmed.AugmentSamples = nil`（而不是逐步减半）→ 整包全过；
- **GE4**：`season_stats.go` `seasonAugmentSampleFor` 去掉内层 `!seasonMayhemQueue(info.QueueID)` 门禁 → 整包全过（外层 `seasonRecordRankedMatch` 已过滤，内层是冗余第二道，但没有独立测试）。
**修复方向：** 补测试，不改生产逻辑：
1. 预算第二级处置后，`Stats` 与 `RankedMatches`（在 level 1 之后）必须与处置前逐项相等，只允许 `AugmentSamples` 变化；
2. 构造刚好超预算一点的输入，断言处置后 `AugmentSamples` 只被裁到刚好合规（尽量保留），不是清空；
3. 直接对 `seasonAugmentSampleFor` 传入非海斗队列（例如 420、450）的对局，断言返回 `nil`。
**验收判据：** GE2、GE2b、GE4 三种变异各自使新增测试 FAIL；`season_stats_budget.go` / `season_stats.go` 原文件不变。

---

# P3

## P3-1　`desktop/overview-render.test.cjs` 很慢

**证据：** 单独跑 54/54 通过、退出码 0、耗时 338 秒（约 5.6 分钟）；R118 台账所述「文件级 worker failure」未复现，「超过 4 分钟未结束」是慢而非挂。
**要求：** 只记录基线耗时到台账，并核对 CI job 是否有 `timeout-minutes`（若有，确认留有余量）；本条不要求改测试文件。
**验收判据：** 台账里有该文件在 CI runner 上的实测耗时；如发现 job 超时阈值 < 该耗时的 2 倍，另立条目。

## P3-2　设计 token 单一来源检查的扫描范围（R118 P3-9 补充）

**证据：** `backend/web/r117-style.test.cjs:6-11` 只枚举 `backend/web/` 且正则为 `/\.(?:js|html)$/`。`desktop/` 下是 `.cjs` 而不是 `.js`，工单原文「`desktop/*.js`」不准确；`desktop/main.cjs`、`overview-banner-layout.cjs`、`share-export.cjs` 有 hex 字面量，但目前都不在 `tokenHexes` 白名单内。
**要求：** 仍按 R118 的判断不主动扩范围；如果将来扩，需同时放行 `.cjs`，并先跑一遍确认不会撞上历史重复。本条只存档。

## P3-3　R116 真机回填（**需要你操作**）

R116 台账标注「待真机」共 12 项，其中最关键的是 R116-探测的三项观测值——至今为空，直接决定「海克斯三选一实时排序」做不做、出装联动能否接线。请打一局海斗，按 `docs/r116-probe-findings.md` 与 `docs/r116probe-execution-ledger.md` 的步骤回填（探测入口需 `R116_AUGMENT_PROBE=1`）。R119 的「补位」判定依赖 `individualPosition` 语义，同样只有手写测试桩，没有真实数据佐证，建议一并抽样核对。

---

## 已复核无需返工（不要重复排查）

- R116-B 前端：9 种变异（较基准符号 / ×100 / 样本档位 / Wilson 显示 / 前端重排 / 冒充官方档位等）全部被打红；
- R116-A：Referer、路径白名单（放开英雄列表）、旧构建剪枝（取反 / 去宽限期）变异全部被打红；
- R116-D：克制方向、置信区间上下限、排序方向、协同去封顶变异全部被打红；P1-4 阶段二函数无生产调用方是台账明示的有意状态；
- R116-E：schema 8→9、队列白名单加入 450 的变异被打红；
- R116-F：`stage.PickRate` 求和、慎选三判据（`Δ<0`、非 low、同稀有度中位数 `>`）的变异全部被打红；阶段 1 有 121 条、每阶段三组之和 100.000%，与测试期望一致；
- R118 P2-8：`hexdata_r116f_test.go` 加锁是承重的，修复到位；
- Node 全量 885 项 / 884 通过 / 0 失败 / 1 跳过；`go build` / `go vet` 通过。

## 复现环境备注

沙箱工具链（Go 1.24.7、Node 22、Playwright Chromium 1194；root 下需用 `--no-sandbox` 的薄包装脚本，不改仓库脚本）与复现命令见 `docs/r116-independent-verification.md` §7、`docs/r118-independent-verification.md` §6。变异脚本使用「基线整文件拷回 + `diff -q`」还原。
