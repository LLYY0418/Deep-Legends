# R120 复测工单执行台账：测试强度与随机红（TEST-POWER-AND-FLAKES）

执行时间：2026-09-22 08:58 – 10:30（本机 macOS darwin/arm64，8 核，Go 1.24.5，Node v24.14.1）
工单原文：`docs/WORKLIST-R120-TEST-POWER-AND-FLAKES.md`
基线：`desktop/package.json` 0.12.14（并行会话在 R121/R122 里升的版，本轮未动）
仓库 HEAD：`4ca768ec`

---

## 0. 一句话结论

工单 4 条全部闭合：**P1-1 / P2-1 / P2-2 三条测试问题全部修复**（只改测试文件，生产代码零改动），**P3-1 可选加固全部采纳**（CI 三条 + 隐私钉死清单两条）。共执行 **17 组新对抗变异**（P1-1 3 组、P2-1 2 组、P2-2 2 组、P3-1 7 组 + 3 组探针/负对照）全部按预期表现，另把 R120 的 **27 组旧变异全量重跑**确认加固没有回归。整包 `go test -race` 连续 3 次全绿，Node 全量 885/884/0/1 与基线一致，`gofmt`/`go vet`/`go build` 干净。**P3-2（真机回填 + GitHub runner 实测）仍待用户操作。**

执行中发现并如实上报的两处偏差（都不是放过，是抓到了、但形状与工单预期不同）：
1. **P1-1 的 M2 变异**（去掉 `c.updating ||` 与 `|| backoff`）实际让**第一次** `loadProPlayers` 就进入无限重拉循环、被 60 秒看门狗翻译成 `directory waited for supplement: context canceled`，而不是走到 `poll refetched directory` 断言——变异被拦下（测试 FAIL），但拦截点更早。分析见 §1 P1-1。
2. **P2-2 在本机有第二个随机红源**：`-count=20000` 会创建 2 万个真实 httptest socket，macOS 临时端口耗尽导致 lookup 偶发退化为传输层错误（原版实测 28/20000、哨兵改版 15/20000，全部是 `当前服务器玩家查询暂时不可用` 形状，与哨兵无关）。已一并修掉（改进程内桩），否则工单验收判据「count=20000 全绿」在本机不可能达成。见 §1 P2-2。

---

## 1. 逐条执行结果

### P1-1　`TestProDirectoryPublishesBeforeSlowSupplements` 测不出「发布等补充源」　→　已修复

**改动**：`backend/pro_players_test.go`（+14/-4，纯测试）。按工单修复方向实现「不依赖墙钟的完成计数」：

1. 补充源 handler 在 `return` 之前 `supplementDone.Add(1)`（`release` / ctx 取消之后）；
2. 第一次 `loadProPlayers` 返回后**立即**断言 `supplementDone.Load() == 0`——发布只要等过任何一次补充源请求的完成（哪怕等到它 12s×2 自己超时），这里必然非 0；
3. 原有断言全部保留：42 队、`updating` 标记、三次轮询后目录请求恰好 1 次、60 秒看门狗、`refreshCtx` 结束即取消；
4. 测试注释同步更正：原来那句「release 不关它就永远不返回」对「补充源自带 12 秒超时」不成立，已改写为准确的同步点描述。

**为什么这个计数是确定性判据**：`release` 在测试体内不关闭，未变异时补充源 handler 只能阻塞在 select 上，`supplementDone` 在测试体内恒为 0；变异后发布路径同步等补充源，handler 经由请求级 12 秒超时返回并抬计数。两边都不赌调度。

**对抗变异**（隔离副本 `mut120b/go`，基线拷回 + `diff -q` 还原，全部一致）：

| 变异 | 期望 | 实测 |
|---|---|---|
| **M1**（工单原文）：发布行后插 `_ = loadProSupplements(loadCtx, provider, func([]opggProTeam) {})` | FAIL | **FAIL（24.3s）**——`pro_players_test.go:375: directory returned only after 6 supplement requests had finished（发布等过补充源）`，与工单预期报错一致（6 = 3 名选手 × 2 次尝试）。**旧断言在同一变异下确认存活**（工单证据复现：测试 25.4s 后 ok） |
| **M2**（工单验收第三条）：`if c.updating || (…) || backoff` 去掉 `c.updating \|\|` 与 `\|\| backoff` | FAIL | **FAIL（60.4s）**——但拦截点是看门狗：`directory waited for supplement: context canceled (teams=0)`，见下方偏差说明 |
| CTRL 未变异 | PASS | **PASS（2.1s）** |

**M2 偏差说明（如实记录）**：工单预期 `poll refetched directory` 断言 FAIL，实测该断言在此变异下**不可达**。原因：发布的快照永远带 `Incomplete` 的 pending 补充源成员（`pendingProSupplements` → `proSupplementsIncomplete(c.teams)` 恒真），去掉 `c.updating`/`backoff` 两个短路后，**第一次**调用的 for 循环每轮都判定「缓存不可用」→ 无限重拉目录，永远不返回，60 秒看门狗取消 ctx 后在第一条断言处失败（`directoryCalls` 此时已远超 1，语义上正是「轮询在重拉目录」，只是轮次发生在第一次调用内部）。结论：**变异被拦下（这是验收的本质要求），拦截提前且由 R120 特意设计的「看门狗把挂死翻译成可读失败」机制完成**——该机制在这次变异下第一次实战生效。另验证过：只去掉两个子句之一时测试仍 PASS（另一个短路兜底），所以工单给的「两个都去掉」是能触发重拉的最小变异，没有更精细的、能让第一次调用正常返回的重拉变异形状。

**验收判据核对**：
- M1 变异 FAIL ✅；
- `GOMAXPROCS=1` 与默认并行 `-race -count=100` 全绿 ✅（8.96s / 3.36s，最终状态 10:08 复跑；R120 时的「27 次假红」没有回来）；
- M2 变异 FAIL ✅（拦截点偏差如上，已附分析与证据）。

### P2-1　`Test2351BanSentinelDoesNotAuthorizeConfiguredHeroes` 随机红 + 名实不符　→　已重写为现行契约测试

**根因复核（先复现再动手）**：
- 旧测试在 `evaluateChampSelect` 返回后**立刻**断言 `f.patches.Load() != 0`，而悬停 PATCH 由 `scheduleChampSelectRequest` 异步发出（延迟 0）——断言赌调度快慢；
- 本机复现率与工单环境不同：`-race` 全新进程单跑 **120 次 0 红**、单进程 `-race -count=400` 也 0 红（工单环境 18%）；但**决定性探针**与工单一致：在断言前加 `time.Sleep(300ms)`，`-race -count=20` **20/20 全红**——证明 PATCH 确实每次都发出，旧断言只是「大多数时候跑得比调度快」才碰巧通过。随机红的机器依赖性与「断言得太早」的根因判断都被证实；
- 契约冲突复核成立：`champselect_execution.go:86-103` 明确把 `[-1]`+进行中 ban 当通配符（`wildcard-grid-hover`），`champselect.go:985` `forceHover` 压掉直接锁定；`r91_addendum_test.go`（wildcard must hover first）与 `r95_test.go`（wildcard must hover before lock）钉住现行契约。旧测试名与失败信息（`empty-ban sentinel used as a wildcard`）断言的是 R78/2351 时代被取代的语义。

**改动**：`backend/diagnostics_2351_test.go`（+67/-3，纯测试）。测试重命名为 **`Test2351BanSentinelHoversOnceWithoutLocking`**（`-run 'Test2351BanSentinel'` 前缀兼容工单验收命令；旧名仅工单文档引用，无代码引用），按工单四步实现：
1. 用 `f.patchCh` + 5 秒超时**等**那一次悬停 PATCH（不再立刻看计数）；
2. 断言载荷 `Type:"ban"`、`ChampionID:141`（配置内且未被禁）、**`Completed:false`**；
3. 等 runner idle（新增 `wait2351RunnerIdle`，与 `waitTakeoverIdle` 同口径：`pending==0 && inFlight==0`，r78 fixture 字段不同所以单独实现）后**再评估一次**（模拟会话未回显悬停时的 watch tick），断言仍无 `Completed:true`、总 PATCH 数恰为 1——去重 `return` 在调度前同步发生，idle 后的计数检查是确定性的，不是赌窗口；
4. **策略显式钉成 `lock-now`**：这是 `forceHover` 唯一承重的场景（默认 `show-then-lock` 下未确认会话本来就不锁，M-A 变异将不可检出——这是实现时发现的关键点，工单未提及）。

**旧语义的取舍依据（工单要求写明）**：「哨兵不得授权任何英雄」已被「哨兵=通配符，先悬停、经会话回显确认后才允许锁定」取代（生产代码 + r91/r95 双测试钉住）。「不直接授权」的核心语义**保留**在新测试第 3 步（确认前不得出现 `Completed:true`），只是不再假装悬停本身不该发生。

**对抗变异**（隔离副本，还原 `diff -q` 一致）：

| 变异 | 期望 | 实测 |
|---|---|---|
| **M-A**：`champselect.go:985` `forceHover := …` → `false`（通配符直接锁定） | FAIL | **FAIL（5.0s）**——`[-1] sentinel never hovered: no PATCH within 5s (patches=0)`。形状说明：lock-now 下去掉 forceHover 后 `completed=true`，但生产的**第二道闸**（锁定前必须经会话回显的 preflight，见 `champselect_execution.go` 注释 "must survive fresh preflight and be echoed by the session before a lock is permitted"）拦下了未确认的锁 → 一条 PATCH 都没有 → 第 1 步超时抓住。变异被拦，拦截消息如实反映「悬停没有发生」 |
| **M-B**：`champselect_execution.go:86` 通配候选条件恒假 | FAIL | **FAIL（5.0s）**——同一条超时断言（无候选 → 无悬停） |
| 探针（旧断言 + 300ms sleep） | FAIL | **FAIL 20/20**——证实旧测试「断言太早」的根因（见上） |

**验收判据核对**：
- `-race -count=200` 默认并行与 `GOMAXPROCS=1` 全绿 ✅（3.05s / 3.36s，最终状态 10:08 复跑；改动后首跑 3.6s/4.3s 亦绿）；
- M-A、M-B 变异均 FAIL ✅；
- 整包 `-race -count=1` 连续 3 次全绿 ✅（见 §2）。

### P2-2　纯数字哨兵被时间戳偶然包含　→　已修复，并顺带闭合本机独有的第二随机红源

**改动 1（工单原文）**：哨兵值 `12345` → **`ZQ7XK`**（`backend/gameplay_test.go:225/240`）、`WARD_SKIN_RENTAL_99999` → **`WARD_SKIN_RENTAL_ZQ7XK`**（`backend/loot_metadata_test.go:189/201`，断言同步）。`ZQ7XK` 含 a–f 之外的字母：诊断行的数字只来自时间戳与计数、`run_id` 只含十六进制字符，都不可能拼出它。`lootIDPrefix` 的 `WARD_` 前缀匹配与回退语义不受影响（改名后测试断言逐项通过）。`gameplay_test.go:249` 另一处 `"12345"` 是**入参**不是日志扫描哨兵（`TestTencentLookupRetriesOneTransportFailure` 无日志子串断言），按范围纪律未动。

**改动 2（执行中发现，超出工单文字但为达成验收所必需）**：`TestGameplayOverviewTencentLookupHTTPErrorIsExplainedAndRecorded` 的 lookup fixture 从 `httptest.NewServer`（真 socket）改为 `gameplayRoundTripFunc` 进程内桩（与本文件 `TestTencentLookupRetriesOneTransportFailure` 同一既有模式；alias/lookup 路径返回 500、其余 404，语义与原 handler 逐分支一致）。

- **为什么必须改**：`-count=20000` 在单进程里创建/关闭 2 万个 httptest 监听器与连接，macOS 临时端口 + TIME_WAIT 压力下 alias lookup 偶发退化为**传输层错误**，落进 `当前服务器玩家查询暂时不可用` 的泛化分支，`HTTP 500` 映射断言失败。**原版（未改哨兵）实测 28/20000 失败、哨兵改版 15/20000 失败，全部是这个形状**（失败迭代耗时 0.51s ≈ 传输重试，正常迭代 3.5ms）——与哨兵无关的既存环境型随机红，工单作者的环境（Linux）未观察到。不改掉它，「count=20000 全绿」的验收判据在本机不可能达成。
- **没有削弱测试**：断言目标（500 → `LCUHTTPError` 分类 → 502+「HTTP 500」文案 → 诊断事件字段 → 不泄漏）全部在 `http.Client.Do` 之上，桩替换的只是 socket 层；真实 socket 行为由同文件其它 httptest 测试覆盖。附带收益：单轮从 71s 降到 17.7s。
- 这不是「加大等待/重试/t.Skip」，是移除环境依赖。

**实测**：
| 命令 | 结果 |
|---|---|
| `go test -count=20000 -run TestGameplayOverviewTencentLookupHTTPErrorIsExplainedAndRecorded ./backend` | **ok 17.7s，0 失败**（改桩前：哨兵版 15 红、原版 28 红） |
| `go test -count=20000 -run TestLootNamingPreservesLocalNamesAndReportsUnresolvedRawIDs ./backend` | **ok 3.1s，0 失败** |

**对抗变异**（工单验收第二条，隔离副本，还原一致）：
| 变异 | 期望 | 实测 |
|---|---|---|
| **L1**：`gameplay.go:911` 诊断事件加 `"tag": tagLine` | FAIL | **FAIL**——`lookup diagnostic leaked Riot ID: {…}`（ZQ7XK 被抓到） |
| **L2**：`lcu_api.go:238` 回退事件加 `"loot_id": item.LootID` | FAIL | **FAIL**——`fallback diagnostics leaked full ID` |

**验收判据核对**：两条测试 `-count=20000`（不带 -race）全绿 ✅；故意泄漏 tagLine / 完整 LootID 的变异仍 FAIL ✅。

### P3-1　可选加固　→　**五条全部采纳**，逐条「做 + 理由」与变异证据

**CI 侧（`backend/web/r117.test.cjs`，+17 行，追加在既有护栏测试内；`.github/workflows/ci.yml` 未动）**：

| # | 采纳 | 做法 | 堵住的绕过 | 变异实测 |
|---|---|---|---|---|
| 1 | **做**（低成本、纯追加） | `assert.equal(quality.needs, undefined, …)`——needs 在 job 允许清单里，必须单独钉死 | `needs` 指向 `if: false` 的新 job → quality 整体被跳过 | **B5 FAIL** ✅ |
| 2 | **做** | quality job 所有步骤的 `run` 文本 `doesNotMatch(/GITHUB_ENV\|GITHUB_PATH/)` | 前置步骤 `echo "R100_APP_SOURCE=/dev/null" >> "$GITHUB_ENV"`（或 NODE_OPTIONS）给护栏注入桩源码——env 映射断言管不到 run 文本 | **B6 FAIL** ✅ |
| 3 | **做** | `Test frontend and desktop renderers` 步骤必须存在且 `if`/`continue-on-error` 均为 undefined | 给运行护栏测试自己的步骤加 `if: false` → 护栏自身永不执行 | **B7 FAIL** ✅ |

**隐私侧（`backend/quality_test.go`，+66 行，新测试 `TestPrivacyStoreDirectoryCreationCallSitesArePinned` + 两个钉死表）**：

| # | 采纳 | 做法 | 变异实测 |
|---|---|---|---|
| 4 | **做** | 生产源码 `os.Mkdir(All)?(` 调用点按**文件+次数**钉死：`champion_images.go:1, item_set_authorized_cleanup.go:1, item_set_migration.go:1, item_set_recommended.go:1, pro_runes.go:1, storage.go:2, update.go:1`（比工单示例多钉了 3 个 `item_set_*`——它们是游戏安装目录侧的创建点，属 explicitWrites 范畴，但钉进同一张清单才能保证「任何新目录创建点都先红」）；每处带归类注释 | **B1**（行内 `MkdirAll(Join(root,"r122-inline"))`）**FAIL** ✅；**B2**（目录名走常量）**FAIL** ✅；**B4**（根变量改名 `base := store.root`）**FAIL** ✅ |
| 5 | **做** | `writeLocalStoreFile(` 调用点同样钉死（扣除定义）：`season_stats.go:1, update.go:3` | **B3**（`writeLocalStoreFile(s, "r122-sub/probe.json", …)` 隐式建子目录）**FAIL** ✅ |

钉死失败信息里写明处置流程：「先在覆盖表（privacyStoreDirectoryCoverage）或豁免清单里归类，再同步更新钉死值」——新增落盘点必须被作者显式分类，不可能悄悄通过。
**已知残留（如实记录）**：直接调 `atomicWriteFile(filepath.Join(store.root, "新文件"))` 写**根级新文件**不新增任何被钉死的调用点，测试抓不到；根级文件目前由 stores 第 11/12 条按名声明，兜底靠人工评审。工单的低成本方案未包含 atomicWriteFile 钉死，本轮按工单范围执行；若要堵死，把 `atomicWriteFile(` 调用点也钉一张表即可（当前分布：champion_cache/champion_network/hexdata/lp_tracker/main/prestige_cache/storage/watch_rules/pro_runes/item_set_recommended 各 1–3 处）。

**回归确认**：加固后把 R120 的全部旧变异重跑——CI 21 组（20 变异 FAIL + CTRL PASS，CTRL 同时证明真实 ci.yml 满足三条新不变量）、Go 6 组（P2-1/GE2/GE2b/GE4/P1-4a/P1-4b 全 FAIL）——**零回归**。

### P3-2　真机回填 + GitHub runner 实测　→　**未做，需要用户操作**

本轮无真机与 GitHub runner 可用：R116 三项探测观测值、R119/R121 补位 `individualPosition` 对照仍为空；CI「Real Chromium…」步骤在 `ubuntu-latest` 上的实际表现（含 Chrome 用户命名空间）与 quality job 总耗时（`overview-render.test.cjs` 单文件约 250–340s，R120 台账 P3-1 已记本机基线）仍待第一次 push 后核对。

---

## 2. 全量回归（真树，10:08–10:27）

| 检查 | 结果 |
|---|---|
| `gofmt -l backend/*.go` | 无输出 |
| `go vet ./backend` | rc=0 |
| `go build -o $PI_SCRATCH_DIR/dl-backend-r120b ./backend` | OK |
| 整包 `go test -race -count=1 ./backend` **连续 3 次** | **ok 266.5s / 271.1s / 271.9s，0 个 `--- FAIL`**（P2-1 验收第四条） |
| Node 全量 `node --test backend/web/*.test.cjs desktop/*.test.cjs` | **885 / 884 pass / 0 fail / 1 skipped**，228s，exit 0——与两轮工单记录的基线逐项一致 |
| 定向：P1-1 `-race -count=100` ×2 条件 | ok 8.96s / 3.36s |
| 定向：P2-1 `-race -count=200` ×2 条件 | ok 3.05s / 3.36s |
| 定向：P2-2 `-count=20000` ×2 条 | ok 17.7s / 3.1s |

变异总账（全部隔离副本、基线拷回 + `diff -q` 还原核对一致、未用过 `git checkout`）：本轮新增 **17 组**（M1/M2/CTRL、M-A/M-B、L1/L2、B1–B7）+ 探针 2 组（sleep 300ms、旧断言存活确认）+ 旧变异重跑 **27 组**（CI 21 + Go 6）= **44 组，异常 0 组**。

---

## 3. 并行会话状态

R121/R122 会话在本轮执行期间仍活跃：09:03 改 `gameplay.go`（`AutofillTagged` → `PositionMismatch` 中性命名）与 `r119_autofill_test.go`，随后新增 `r122_autofill_diagnostic_test.go`。对本轮的影响与处置：
- 09:50 前后隔离快照与仓库一度版本错位（快照 `r119_autofill_test.go` 是旧版，构建失败）——**整快照重新 rsync（09:51:09）+ `go vet` 确认后重跑**，L1/L2 变异 evidence 均为真实断言失败而非构建失败；
- 本轮改动的 7 个文件与对方无交集；10:08 起对方静默（10 分钟内无文件变动），最终全量验收全部在真树完成；
- `desktop/package.json` 仍是对方升的 **0.12.14**，本轮未动（是否随本轮改动升版请用户定）。

## 4. 本轮改动文件清单（7 个，全部只改测试/护栏，生产代码零改动）

| 文件 | 变化 | 属于 |
|---|---|---|
| `backend/pro_players_test.go` | +14/-4（supplementDone 计数 + 注释更正） | P1-1 |
| `backend/diagnostics_2351_test.go` | +67/-3（测试重写并更名 + wait2351RunnerIdle） | P2-1 |
| `backend/gameplay_test.go` | +16/-11（哨兵 ZQ7XK + fixture 改进程内桩） | P2-2 |
| `backend/loot_metadata_test.go` | +5/-2（哨兵 ZQ7XK） | P2-2 |
| `backend/web/r117.test.cjs` | +17（needs / GITHUB_ENV / 前端步骤三条不变量） | P3-1 |
| `backend/quality_test.go` | +66（调用点钉死测试 + 两张钉死表） | P3-1 |
| `docs/r120-retest-execution-ledger.md` | 新增（本文件） | — |

**刻意未改**：`backend/pro_players.go`、`backend/champselect.go`、`backend/champselect_execution.go`、`backend/gameplay.go`、`backend/lcu_api.go`、`.github/workflows/ci.yml`、`desktop/package.json`、以及并行会话的全部文件（变异只发生在隔离副本，均已还原核对）。

## 5. 复现命令

```bash
cd /Users/ly/personal/personal-work/deep-legends
# P1-1
GOMAXPROCS=1 go test -race -count=100 -run 'TestProDirectoryPublishesBeforeSlowSupplements' ./backend
go test -race -count=100 -run 'TestProDirectoryPublishesBeforeSlowSupplements' ./backend
# P2-1
go test -race -count=200 -run 'Test2351BanSentinel' ./backend
GOMAXPROCS=1 go test -race -count=200 -run 'Test2351BanSentinel' ./backend
# P2-2
go test -count=20000 -run 'TestGameplayOverviewTencentLookupHTTPErrorIsExplainedAndRecorded' ./backend
go test -count=20000 -run 'TestLootNamingPreservesLocalNamesAndReportsUnresolvedRawIDs' ./backend
# P3-1
go test -count=1 -run 'TestPrivacyStore' ./backend
node --test backend/web/r117.test.cjs
# 全量
go build -o /tmp/dl-backend ./backend && go vet ./backend && go test -race -count=1 ./backend
node --test backend/web/*.test.cjs desktop/*.test.cjs
```

变异脚本（`$PI_SCRATCH_DIR/mut120b/`，隔离副本 + `diff -q` 还原）：
- `p11-mutations.cjs`（M1/M2/CTRL）、`p21-mutations.cjs`（M-A/M-B）、`p22-mutations.cjs`（L1/L2）、`p31-mutations.cjs`（B1–B7）
- 旧套件：`mut120/ci-guard-mutations.cjs`（21 组）、`mut120b/go-mutations.cjs`（6 组）
