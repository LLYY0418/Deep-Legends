# R120 执行台账：R116 / R118 独立验收遗留缺口

执行时间：2026-09-21 21:20 – 23:20（本机 macOS darwin/arm64，8 核，Go 1.24.5，Node v24.14.1，Chrome 稳定版本机可用）
工单原文：`docs/WORKLIST-R120-INDEPENDENT-VERIFICATION-R116-R118-GAPS.md`
基线：`desktop/package.json` 开工时 0.12.13；**执行途中被并行会话改成 0.12.14**（见 §3）
仓库 HEAD：`4ca768ec`（未提交改动含 R116–R121 全部成果）

---

## 0. 一句话结论

工单 11 条里 **10 条已闭合**（P1-1 复核即成立；P1-2 / P1-3 / P2-1 / P2-2 / P2-3 / P2-4 全部修复并通过对抗变异；P1-4 已由用户拍板并落地；P3-1 / P3-2 按要求只记录不改代码），**只剩 P3-3 需要用户打一局海斗回填真机观测值**。共执行 **39 组对抗变异**（P1-2 21 组 + P2-3 8 组 + Go 侧 6 组 + P1-3 4 组）另加 1 组「修复前」负对照，全部按预期表现、无一存活；真树 `go build` / `go vet` / `gofmt` 干净，整包 `go test -race` 连续 4 次全绿，前端全量 885/884/0/1 与基线一致。

**必须上报的两件事**：
1. **同一工作树里有另一个会话在并行执行 R121**（补位标签），21:51–22:47 之间持续改动 `gameplay.go` / `riot_api.go` / `web/gameplay.js` / `r119_autofill_test.go` / 新建 `position_contract_probe.go`、`augment_contract_probe.go`，其中 22:29 前后整包一度**编译不过**（`position_contract_probe.go:214 declared and not used: paths`）。本轮所有变异与长测都在隔离快照里做，仓库只用编辑工具改了自己的 11 个文件；23:10 复查时对方文件已自行修好，`go vet ./backend` 恢复干净。详见 §3。
2. **版本号已被对方改成 0.12.14，本轮未再动 `desktop/package.json`**。是否要为本轮单独出 0.12.15，请用户定（见 §4）。

---

## 1. 逐条执行结果

### P1-1　`backend/gameplay.go` gofmt　→　复核即成立，本轮未改动

- 开工第一件事就是跑工单给的证据命令：`gofmt -l backend/*.go` **无输出**；再跑 CI `quality` 第一步的精确命令
  （`find . -type d \( -name '.*' ! -name . -o -name node_modules -o -name vendor -o -name dist \) -prune -o -type f -name '*.go' -print0 | xargs -0 gofmt -l`）同样**无输出**。
- `backend/gameplay.go:501` 已是 `Autofill  bool \`json:"autofill,omitempty"\``（两空格，与下一行 `reference gameplayReference` 对齐），文件 mtime **21:53**，晚于工单撰写时间——即 R119/R121 侧已经自己修掉了。
- 收工前（23:10）再复核一次：`gofmt -l backend/*.go` 仍无输出。
- **本轮没有改 `gameplay.go` 一个字节**（该文件是并行会话的地盘，且已满足判据）。

**验收判据核对**：`gofmt -l backend/*.go` 无输出 ✅；「diff 只涉及该文件的对齐空白」不适用（改动非本轮所为）。

### P1-2　CI 真机护栏改成结构不变量　→　已修复，21 组变异全数拦下

**改动**：只改 `backend/web/r117.test.cjs`（+62 行，全部在既有测试 `R117 real Chromium guards are wired into CI and cannot silently skip` 内部追加），**`.github/workflows/ci.yml` 一个字没改**。按工单要求从黑名单改成白名单/结构不变量：

| 新增不变量 | 拦住的形状 |
|---|---|
| `workflow.on` 必须含 `push` 与 `pull_request`（用 `hasOwnProperty`，`on: push:` 解析成 `null` 也算存在） | N3 |
| 两个触发器都不得有 `branches` / `branches-ignore` / `paths` / `paths-ignore` / `tags` / `tags-ignore` | N4、X1 |
| `workflow.defaults` / `jobs.quality.defaults` / 护栏步骤**递归**不得出现任何 `shell` 键 | N1、N2、N5、X6 |
| `jobs.quality` 每个步骤若显式写 `shell`，只允许 GitHub 内建名（`bash`/`sh`/`pwsh`/`powershell`/`python`，Linux 上自带 `-e`） | `shell: "bash {0}"` 一类自定义模板 |
| `jobs.quality` 的键走**显式允许清单** `name/needs/runs-on/steps/permissions/environment/container/services/outputs`；`env`、`defaults`、`if`、`continue-on-error`、`strategy`、`timeout-minutes`、`concurrency` 一律不在清单内 | X2、X4、X5 + R118 的两种 |
| 护栏步骤的键只允许 `name`/`run`/`env`；`env` 再走允许清单 `CHROME_BIN`/`R100_BROWSER_OUTPUT`/`R117_BROWSER_OUTPUT` | ENV、X7、X8 |
| `workflow.env` / `jobs.quality.env` / 护栏步骤 `env` 三层都禁止任何 `*_SOURCE` 键 | 「CI 把 `R100_APP_SOURCE` 指向桩文件」这一形状 |

R118 既有的断言（job 级 `continue-on-error`/`if`、`run` 块精确命令行、`|| echo` / `|| :` / `; true` / `set +e` 的 shell 级软化正则）**全部保留未动**。

**对抗变异（21 组，全部在隔离副本 `$PI_SCRATCH_DIR/mut120/ci/sandbox` 里做，还原用「基线整文件拷回 + `diff -q`」，未使用 `git checkout`）**：

| 组 | 变异 | 期望 | 实测 | 触发的断言 |
|---|---|---|---|---|
| R118-1 | `jobs.quality.continue-on-error: true` | FAIL | **FAIL** | job 级 continue-on-error |
| R118-2 | `jobs.quality.if: github.event_name == 'workflow_dispatch'` | FAIL | **FAIL** | job 级 if |
| R118-3 | r100 命令行加 `\|\| echo skipped` | FAIL | **FAIL** | run 块精确命令行 |
| R118-4 | r100 命令行加 `\|\| :` | FAIL | **FAIL** | 同上 |
| R118-5 | r100 命令行加 `; true` | FAIL | **FAIL** | 同上 |
| R118-6 | run 块首行插 `set +e` | FAIL | **FAIL** | 同上 |
| **N1** | 护栏步骤加 `shell: "bash {0}"` | FAIL | **FAIL** | 护栏步骤不得出现 shell 字段 |
| **N2** | `jobs.quality.defaults.run.shell: "bash {0}"` | FAIL | **FAIL** | jobs.quality.defaults.run 不得出现 shell |
| **N5** | workflow 级 `defaults.run.shell: "bash {0}"` | FAIL | **FAIL** | workflow.defaults.run 不得出现 shell |
| **N3** | `on:` 只留 `workflow_dispatch` | FAIL | **FAIL** | workflow.on 必须包含 push |
| **N4** | `on.push.branches` / `on.pull_request.branches` 设成永不匹配 | FAIL | **FAIL** | on.push.branches 会让护栏整轮不触发 |
| **ENV** | 护栏步骤 `env: R100_APP_SOURCE: /tmp/stub` | FAIL | **FAIL** | env 允许清单多出 R100_APP_SOURCE |
| X1 | `on.push.paths: ["never/**"]`（我自设） | FAIL | **FAIL** | on.push.paths |
| X2 | job 级 `env: R100_QUEUE_SOURCE`（我自设） | FAIL | **FAIL** | jobs.quality 出现未审计字段 env |
| X3 | workflow 级 `env: R117_APP_SOURCE`（我自设） | FAIL | **FAIL** | workflow.env 不得定义 *_SOURCE |
| X4 | `jobs.quality.timeout-minutes: 1`（我自设） | FAIL | **FAIL** | 未审计字段 timeout-minutes |
| X5 | `jobs.quality.strategy.fail-fast: false`（我自设） | FAIL | **FAIL** | 未审计字段 strategy |
| X6 | 护栏步骤 `shell: "sh {0}"`（换 shell 名，我自设） | FAIL | **FAIL** | 护栏步骤不得出现 shell 字段 |
| X7 | 护栏步骤级 `continue-on-error: true`（我自设） | FAIL | **FAIL** | continue-on-error 会吞掉失败 |
| X8 | 护栏步骤 `if: github.event_name == 'workflow_dispatch'`（我自设） | FAIL | **FAIL** | 条件执行会整步跳过 |
| CTRL | 未变异 | PASS | **PASS** | — |

合计 21 组，异常 0 组。副本还原 `diff -q` 一致；`.github/workflows/ci.yml` mtime 仍是 **15:55**（本轮开工之前），确认未被本轮触碰。

**验收判据核对**：N1 N2 N5 N3 N4 五种各自使该测试 FAIL ✅；「护栏步骤 env 里设 `R100_APP_SOURCE`」FAIL ✅；R118 六种保持 FAIL ✅；未变异控制组 PASS ✅。另外自设 8 种不同形状也全部 FAIL（工单要求「主动设计与执行方自述不同形状的变异」）。

### P1-3　真 Chromium 脚本打印 PASS 后退出码 1　→　已修复，并做出比工单更强的负对照

**改动**：`desktop/r100-browser.cjs` 与 `desktop/r117-browser.cjs` **各自的收尾**（每个文件 +20/-1 行），护栏断言一行未动。原来是一行 `.finally()` 里同步 `proc.kill()` 后立刻 `fs.rmSync(temp, {maxRetries:8, retryDelay:100})`；现在改成 `async function cleanup()`：

1. `ws.close()`（吞异常）；
2. `proc.kill()` 后 **`await` 进程真正 `exit`**（`exit`/`error` 事件 + 5 秒超时兜底）；
3. `server.closeAllConnections()` + `server.close()`（2 秒超时兜底）；
4. 删目录最多重试 8 次、每次间隔 150 ms，**全部失败也只打一行 `cleanup warning: <temp> left behind (<原因>)`**；
5. `cleanup()` 的拒绝被 `.catch` 接住并降级成同一行警告，不可能变成未处理拒绝。

主流程失败的退出码路径完全没变：`main().catch(e=>{console.error(e);process.exitCode=1;})` 仍在 `.finally` 之前把 `exitCode` 置 1，清理路径不参与退出码。

**实测（本机 Chrome，`CHROME_BIN=/Applications/Google Chrome.app/...`，输出目录一律指向 scratch，仓库 `docs/r100-validation/`、`docs/r117-validation/` 的 mtime 仍是 17:11，未被本轮写入）**：

| 场景 | 结果 |
|---|---|
| 修复后 r100 连跑 **20 次** | exit≠0 **0 次**；缺 PASS 行 0 次；日志出现 ENOTEMPTY **0 次** |
| 修复后 r117 连跑 **20 次** | exit≠0 **0 次**；20/20 有 `R117 Chromium PASS`；ENOTEMPTY **0 次** |
| **负对照**：把收尾还原成修复前那一行，各跑 10 次 | r100 **10/10 exit≠0 且 10/10 ENOTEMPTY**；r117 **10/10 exit≠0 且 10/10 ENOTEMPTY** |
| 变异 A：把真实断言改成不可能成立（r100 `peakImages<=limit` → `peakImages>999999`；r117 首屏样式表清单 → `['/never-loaded.css']`） | 两个脚本都 **exit=1**，失败没被清理路径吞掉 |
| 变异 B：把 `fs.rmSync` 换成永远抛 `ENOTEMPTY` | 两个脚本都 **exit=0**、`Chromium PASS` 行 1 条、`cleanup warning` 行**恰好 1 条** |

变异全部在隔离副本 `$PI_SCRATCH_DIR/mut120/browser/copy` 里做（副本自带 `backend/web` 与两个脚本），收尾用 `diff -q` 与仓库文件核对**一致**。

> 负对照这一条比工单要求的更强：工单说云端沙箱里 r100 6 次中 4 次、r117 5 次中 3 次复现；本机是 **20/20 全部复现**。也就是说这个缺陷在真 runner 上不是「偶发变红」而是「几乎必然变红」，工单要求的「请在真 runner 上看一次实际日志」仍建议做，但修复的必要性已无需等真机确认。

**验收判据核对**：两个脚本各连续 20 次全部 exit 0 ✅；断言失败必须 exit≠0 ✅；`rmSync` 永远删不掉时 PASS 仍 exit 0 且打出一行清理警告 ✅。

### P1-4　`season-stats/` 与其它未声明目录的隐私声明　→　用户拍板后已落地

**用户裁决（本轮 asktool 原文选项）**：① 采用 **A 版**文案（一条覆盖整个 `season-stats/`，同时闭合 R117 第 662 行的既存缺口）；② **是**，核对后把确实未声明的都补进 `stores`；③ **加**结构性测试，并带豁免清单机制。

**改动 1：`backend/features.go:752`** —— `stores` 从 7 条扩到 **12 条**，前 7 条**逐字未动**，新增 5 条：

| # | 覆盖的目录/文件 | 新增条目要点 |
|---|---|---|
| 8 | `season-stats/`（含 `sgp/` 子目录） | A 版原文照录：逐场对局 ID、逐英雄胜负与 K/D/A/补刀、420/440 与海斗逐场快照、海斗逐场海克斯与装备 ID 和胜负；单文件上限 4 MiB、超限先截断逐场快照、绝不丢英雄统计；不含原始账号标识、不外发 |
| 9 | `champion-images` / `community-images` / `profile-icons` / `prestige-artwork` / `perk-catalog` / `champion-data` | 按内容哈希命名的公开游戏资源与数据缓存：英雄与皮肤图标、玩家头像、臻彩与皮肤原画、符文与海克斯目录、英雄统计数据的公开信封与构建状态；不含账号标识、有条数与体积上限并按最近使用自动回收、不外发 |
| 10 | `pro-runes-v1` / `pro-directory` / `pro-profiles` | 职业赛事与职业账号的公开数据缓存：赛事日程与对局详情、职业选手的符文与装备明细、职业名单与账号快照（Riot ID/昵称/段位）；来自公开赛事接口与 OP.GG，不含你的账号数据、不外发 |
| 11 | `updates` + 根级 `update-settings.json` / `update-manifest.json` | 自动更新用到的本地文件：下载的安装包与校验文件、更新设置与最新版本清单；不含账号数据 |
| 12 | 根级 `session-token` / `convenience.json` / `champion-network.json` | 本机偏好与界面令牌：随机生成并跨重启复用的本地界面会话令牌、工具页的自动化规则与偏好开关、英雄关系网络数据源设置；不含 Riot 账号凭据、不外发 |

> 第 11、12 条覆盖的是**根级散文件**，不是目录，严格说超出「未声明目录」的字面范围。按用户「确实未声明的都补进」的口径一并声明；如需收窄，删掉这两条即可，但届时 `session-token`（本机 UI 令牌，`main.go:1676/1695`）会重新变成未声明项。
> 每条文案都对着实现核过：目录清单来自 `storage.go:118` 的 MkdirAll 循环 + `champion_images.go:24`（`newPublicBinaryCache`）+ `pro_runes.go:140/185`；条数/体积上限取自各调用点实参（`champion-images` 2048/64 MiB、`community-images` 512/16 MiB、`profile-icons` 2048/64 MiB、`perk-catalog` 8/4 MiB、`pro-profiles` 64/4 MiB、`pro-directory` 2/16 MiB、`prestige-artwork` 512/192 MiB/45 天、`champion-data` 256/64 MiB）；淘汰逻辑见 `champion_cache.go:418-475 pruneDiskLocked`。

**改动 2：`backend/quality_test.go`** —— 新增 `TestPrivacyStoresDeclareEveryLocalDirectory`（+176 行，含覆盖表与豁免表）。目录清单**从源码解析**，不写死在测试里：

- (a) 解析 `storage.go` 里 `for _, path := range []string{...}` 的启动期清单，按括号深度切分（`filepath.Join(root, "x")` 自带逗号），常量项（`prestigeArtworkCacheDirectory` / `championDataCacheDirectory`）在整个包文本里回查字符串字面量；
- (b) 全包扫 `newPublicBinaryCache(<store>, "<dir>", ...)`；
- (c) 全包扫 `变量 = filepath.Join(<...>root, "<dir>")` 且**同文件里对该变量调用 `MkdirAll`**（这条把 `pro-runes-v1`、`updates` 抓出来，同时天然排除 `account-salt`、`lp-history.json` 这类根级**文件**）；
- 解析失效防护：枚举结果 `< 14` 个直接 `t.Fatalf`，避免「什么都没扫到 → 全绿」这种静默失效；
- 覆盖表反向核对：表里登记但源码里已找不到创建点的目录也报错（防止表变成陈旧装饰）；
- `privacyStoreDirectoryExemptions` 现在是**空表**，机制保留，任何豁免必须写非空理由，空理由判失败。

实测枚举到 **16 个目录**：`champion-data champion-images community-images logs perk-catalog pools prestige-artwork pro-directory pro-profiles pro-runes-v1 profile-icons riot-identities riot-matches season-stats snapshots updates`（`-v` 下会打印这一行，便于日后核对）。

**对抗变异**（隔离副本，`diff -q` 还原核对一致）：

| 变异 | 期望 | 实测 |
|---|---|---|
| P1-4a：从 `stores` 里删掉 season-stats 那一条 | FAIL | **FAIL** —「目录 "season-stats" 的存储声明缺少关键词 "本赛季个人对局统计"」 |
| P1-4b：`storage.go` 清单里新增未声明目录 `r120-undeclared` | FAIL | **FAIL** —「目录 "r120-undeclared"（创建于 storage.go）没有登记进隐私声明覆盖表」，枚举个数从 16 变 17 |

既有的 `TestPrivacyListsEveryClientWrite`（`quality_test.go:403`）未改，仍然 PASS（它只对 `explicitWrites`=11、`automaticWrites`=10 断言条数，`stores` 无条数断言，所以扩条不会打破它）。

**顺带核实（未改，仅记录）**：子代理审计发现 `externalReads` 第 5 条「绝活哥符文……结果在进程内缓存 6 小时」曾被怀疑与 `pro-runes-v1/` 落盘冲突。**逐个查过后确认不冲突**：`specialist_runes.go` 的 `specialistRuneCacheTTL = 6 * time.Hour`（:15，用在 :225）确实只有进程内缓存、全文件没有任何写盘调用；落盘的 `pro-runes-v1/` 属于**职业赛事符文**（`pro_runes.go`，数据来自公开赛事接口 `/persisted/gw/`、`/livestats/v1/`），是另一条链路，现已由新增的第 10 条声明覆盖。`neverStores` 五条逐条核对未被本轮改动变成假话（`session-token` 是本工具自己的 UI 令牌，不是 LCU 令牌）。

### P2-1　`loadStructuredDetail` 数据竞争 + goroutine 泄漏　→　已修复

**改动**：`backend/champions_structured.go:1177-1194`（+18/-10，含 6 行说明注释）。按工单的修复方向，goroutine 只捕获**局部**通道变量：先 `positionsCh := make(chan qq101PositionsResult, 1)` / `buildCh := make(chan qq101BuildResult, 1)`，再赋给外层的 `qq101PositionsC` / `qq101BuildC`，两个闭包分别只引用 `positionsCh` / `buildCh`。父侧的置空逻辑（`select` 各分支与 `timer.C` 分支）**一行未改**，从此只作用于它自己的等待状态。通道保持 cap 1，没加锁。

后果闭合：晚到的 goroutine 往自己的带缓冲通道发送，**永远不需要接收方在场**，既没有竞争也不会阻塞泄漏。

**新增测试**：`backend/qq101_test.go` `TestStructuredDetailQQ101TimeoutDoesNotLeakGoroutines`（+92 行，含 `r120SettledGoroutines` / `r120GoroutineDump` 两个辅助函数）。QQ101 上游在非 version 路径上 `<-request.Context().Done()` 死等，只有父协程放弃并 `cancelQQ101()` 之后才返回，**保证结果必然晚到**；随后在 10 秒有限窗口内断言 `runtime.NumGoroutine()` 回到基线（基线先等 goroutine 数连续 5 次采样不变，避免把前一个测试的 HTTP/2 余波算进来）。

**实测**：

| 命令 | 结果 |
|---|---|
| `go test -race -count=20 -run 'TestStructuredDetailFallsBackWhenQQ101BuildTimesOut\|TestStructuredDetailQQ101TimeoutDoesNotLeakGoroutines' ./backend`（真树，23:16） | `ok 7.510s`，无 DATA RACE |
| 同上（隔离快照，22:0x） | `ok 8.041s` |
| **变异**：把闭包改回引用共享的 `qq101PositionsC`/`qq101BuildC`（`-race -count=2`） | **FAIL** —「QQ101 超时路径泄漏了 goroutine：baseline=4 now=6 qq101Hits=2（晚到的结果发进了 nil 通道）」，两次迭代各泄漏 2 个 |

**验收判据核对**：`-race -count=20` 全部无 DATA RACE ✅；泄漏断言在超时路径晚到结果后回到基线 ✅；改回共享变量的变异被泄漏断言打红 ✅。

### P2-2　随机失败测试　→　先量化、定位到两条真实成因，再按「改测试用同步点」收敛

**量化（修复前，隔离快照）**：

| 条件 | 命令 | 结果 |
|---|---|---|
| GOMAXPROCS=1 | `GOMAXPROCS=1 go test -race -count=100 -run 'TestProDirectoryPublishesBeforeSlowSupplements\|TestProDirectoryFailureRetainsRosterAndAuth' ./backend` | **27/100 失败**，全部是 `pro_players_test.go:343 poll refetched directory` |
| 整包并行 | 同上去掉 `GOMAXPROCS=1` | 第一次跑出现多条同样失败，第二次跑 **0 失败**（不稳定） |
| 单跑 | `-count=1` 连跑 10 次 | 0/10 失败（与工单「单跑 40/40 通过」一致） |

**定位（不是猜的，是打探针打出来的）**：在快照里给 mock transport 加一行 `t.Logf` 打印每个请求的 host/path，`GOMAXPROCS=1 -count=6` 当场抓到两条事实：

1. 除目录请求 `op.gg /zh-cn/lol/spectate/list/pro-gamer` 之外，**后台补种子/阶梯还会打 `op.gg /zh-cn/lol/summoners/kr/<账号>`**；而原测试的计数器是「凡不是补充源 host 的请求都算一次目录请求」，于是断言 `directoryCalls == 1` 实际在赌「后台请求有没有恰好在三次轮询之间落地」——**测试对时序做了错误假设**，与生产逻辑无关。
2. 同一次运行直接 **panic**：`Log in goroutine after TestProDirectoryPublishesBeforeSlowSupplements has completed: ... host=op.gg path=/zh-cn/lol/summoners/kr/...`。即 `loadProPlayers` 发布目录后的后台 goroutine 会**活过测试结束**再回调 transport；生产上这是**有意设计**（`pro_players.go:246` 注释「One caller leaving a page must not cancel another caller's shared load」，后台用 `a.proRefreshContext`，预算 90 秒），但测试侧任何 `t.Log/t.Error` 都会把整个测试二进制 panic 掉，**失败会被记在当时正在跑的另一个测试头上**。这正是 `TestProDirectoryFailureRetainsRosterAndAuth`「单跑 40/40、整包负载下偶发红」的形状（它自己用 `newProMockApp`，该 helper 的 transport 里有 `t.Errorf`）。

**修复（`backend/pro_players_test.go`，+44/-7；生产代码一行未改）**：

- `newProMockApp`：加 `var finished atomic.Bool` + `t.Cleanup(finished.Store(true))`，测试结束后不再调用 `t.Errorf`；未复核名字直接返回错误而不是继续走 `player.Names[0]`（原写法在 `!ok` 时会二次 panic）。**这一条消灭的是「整包被 panic 打红并归错测试」这一类**，对三个使用方都生效。
- `TestProDirectoryPublishesBeforeSlowSupplements`：
  - 计数器改成**只数目录请求**（`r.URL.Path == proPlayersPath`），另设 `supplementCalls` / `otherCalls` 两个计数，失败信息里三个数都打出来（排障时一眼看出是谁多了）；
  - 给 `a.proRefreshContext` 挂一个可取消 ctx 并 `defer cancelRefresh()`，测试结束时后台补种子/阶梯立刻收工，不再带着 90 秒预算活到下一个测试；
  - 去掉 1 秒墙钟预算：改成 `context.WithCancel` + 一个 60 秒看门狗。看门狗**只负责把挂死翻译成可读失败**——取消之后 `loadProPlayers` 只能返回 ctx 错误，下面的断言必然失败，所以它不可能把挂死伪装成通过。「目录先发布」这件事本身由**同步点**证明：`release` 只在测试结束时关闭，整个测试体里补充源不可能返回，既然如此能拿到 42 支队伍就已经证明没等补充源，不需要任何时间断言。
  - 没有加大任何等待时间、没有重试、没有 `t.Skip`。

**修复后实测**：

| 条件 | 命令 | 结果 |
|---|---|---|
| GOMAXPROCS=1 | `-race -count=100` | `ok 23.588s`，**0 失败**（快照 23.475s / 0 失败，两侧一致） |
| 整包并行 | `-race -count=100` | `ok 17.127s`，**0 失败** |
| 整包 `-race -count=1` **连续 3 次**（完整快照 go3，含 `docs/`、`README.md`、`desktop/`） | | `ok 266.929s` / `ok 272.795s` / `ok 264.070s`，**3 次全绿，0 个 `--- FAIL`** |
| 整包 `-race -count=1` **真树**（23:10–23:15） | | `ok 266.298s`，**0 失败** |

`TestStructuredDetailFallsBackWhenQQ101BuildTimesOut` 是 P2-1 的表现，已随 P2-1 修复，`-race -count=20` 干净。

**验收判据核对**：两个测试在两种条件下 `-race -count=100` 全绿 ✅；整包 `-race -count=1` 连续 3 次全绿 ✅（另加真树 1 次全绿）。

### P2-3　`promote` 测试的保护被夸大　→　选 A（改注释），并按实测把话说得更准

**改动**：`backend/hexdata_r116a_test.go:931-952` 的注释（+22/-9），`hexdata.go` 一个字没改（8 处 `promote` 全部保留）。

**8 组变异整包实测**（子代理在隔离副本里做，每处用 `if false { ... }` 包住该条语句——比删行更安全，参数变量仍被使用，不会触发 Go 的 declared-and-not-used；每次单变异、跑完整包、`cp pristine` + `diff -q` 还原，全部 RESTORED_OK）：

| # | kind | 行号 | 相对基线的失败增量 | 被打红的测试 |
|---|---|---|---|---|
| 1 | answer | 1475 | **+2** | `TestHexdataColdRankingBudgetAndDiskRestart`（`hexdata_test.go:478`）、`TestLoadMayhemDetailColdHeroCostsFourHexdataRequestsWithWarmSiteMetadata`（`hexdata_r116a_test.go:780`） |
| 2 | heroes | 1864 | 0（存活） | — |
| 3 | meta | 2105 | **+1** | `TestLoadMayhemDetailRequestsHeroJSONAndMetaOnly`（`hexdata_r116a_test.go:651`） |
| 4 | postmatch | 2654 | 0（存活） | — |
| 5 | hextech-insights | 2871 | 0（存活） | — |
| 6 | augment | 3148 | 0（存活） | — |
| 7 | rarity | 3180 | 0（存活）※ | — |
| 8 | hero-json | 3318 | 0（存活） | — |

※ rarity 首跑出现过 +1，失败项是 `TestProDirectoryPublishesBeforeSlowSupplements`（`pro_players_test.go:343 poll refetched directory`），同一变异复跑消失、单跑 5 次全绿——**这正是 P2-2 修掉的那条 flake**（子代理用的快照里还是修复前的 `pro_players_test.go`）。两条工单条目在同一次实验里互相印证。

**比工单更准的一点**：`TestHexdataJSONKindsSurviveSoftTTLBecauseTheyArePromoted` **自身对 8 处删除全都不敏感**（8 次全 PASS，包括 answer 与 meta）——它断言的「13 小时后仍能从磁盘兜底」由正常路径 `loadWithStatus(persistDisk=true)`（`hexdata.go:1096`）提供。真正钉住 promote 的是上表那三条**别的**测试，且只覆盖 answer 与 meta。新注释已按这个实测结果写，不再让「本测试」背这个承诺；测试名保持不动（多份历史台账按名字引用它），名字里的 `BecauseTheyArePromoted` 在注释中被明确限定为「只对 answer / meta 成立」。

**验收判据核对（A 版）**：注释里点名的被保护项与实测一致 ✅（answer 删除仍 FAIL 于 2 条测试、meta 删除仍 FAIL 于 1 条测试，且注释如实写明本测试自身不敏感、其余 6 处无覆盖）。

### P2-4　赛季缓存预算与内层队列门禁缺测试　→　已补 3 条，生产文件未改

**新增测试**（`backend/season_stats_budget_test.go` +219 行含 3 个辅助函数、`backend/season_stats_test.go` +44 行；`season_stats_budget.go` 与 `season_stats.go` **一个字没改**）：

1. `TestSeasonStatsBudgetThirdStageOnlyDropsAugmentSamples` —— 工单要求 1：基座（173 条 Stats + 40 万条 GameIDs，两者都不可截断）单独就撑到 5,747,687 B > 4 MiB，**逼处置走到第三级**；断言写出的载荷里 `Stats` / `GameIDs` / `QueueStats` 与缓存元数据（schema、source、season、accountHash、complete、resume/pending、updatedAt）**逐项 `reflect.DeepEqual`**，`RankedMatches` 必须等于「第一级裁剪后的那 40 条」，只有 `AugmentSamples` 归零；`Steps` 必须含 `ranked_matches_capped` / `augment_samples_dropped` / `written_over_budget`，`OverBudget` 必须为真；顺带断言调用方手里的 cache 没被就地改坏。
2. `TestSeasonStatsBudgetKeepsAsManyAugmentSamplesAsFit` —— 工单要求 2：不写死魔法数字，先量出「单条样本 115.5 B、单条 GameID 14.00 B」的实际开销，用浮点解出「全量样本刚好超预算、砍一半就合规」的窗口（实测 `gameIDs=283275`，`raw=4,252,137`，一半样本 `4,136,637`，预算 `4,194,304`），再带 64 次实测校正兜底。断言处置**停在第一个合规档位**：保留 1000/2000 条（非空、非全量），写出 4,125,237 B 在预算内，且**反证**「上一档 2000 条必然超限」——即真的「尽量保留」，既不是清空也不是过度截断。
   > 这条第一版翻过一次车：整数除法把 GameID 的 14 B 算成 13 B，外推 28 万条冲过预算约 250 KB，fixture 自身不成立。改成浮点 + 实测校正窗口后一次命中（校正 0 次）。
3. `TestSeasonAugmentSampleForRejectsEveryNonMayhemQueue` —— 工单要求 3：直接对 `seasonAugmentSampleFor` 传 420 / 440 / **450** / **3220** / 0 / -1 / 2299 / 2301 / 3271，全部必须返回 `nil`；反向退化同样拦（`seasonMayhemQueueIDs` 三个队列必须照记，且 `seasonMayhemQueue` 与该清单不得自相矛盾，样本内容与零槽位丢弃一并核对）；另外三个 `nil` 分支（本人不在参与者名单、`info == nil`、这一场没有任何海克斯）也各断言一次。

**对抗变异**（隔离副本，`diff -q` 还原核对一致）：

| 变异 | 期望 | 实测 |
|---|---|---|
| **GE2**：第二级处置里顺手 `trimmed.Stats = nil` | FAIL | **FAIL** —「Stats 在第二/三级处置里被改动了：173 -> 0 条」 |
| **GE2b**：第二级一上来 `trimmed.AugmentSamples = nil` | FAIL | **FAIL** —「处置步骤里没有 "augment_samples_trimmed"：[ranked_matches_capped augment_samples_dropped]」 |
| **GE4**：`seasonAugmentSampleFor` 去掉内层 `!seasonMayhemQueue(info.QueueID)` | FAIL | **FAIL** —「队列 420 不是海克斯大乱斗，却记下了样本：&{GameID:91 ChampionID:157 ...}」 |

**验收判据核对**：GE2 / GE2b / GE4 三种变异各自使新增测试 FAIL ✅；`season_stats_budget.go` / `season_stats.go` 原文件不变 ✅（`git diff` 里这两个文件不含本轮改动）。

### P3-1　`desktop/overview-render.test.cjs` 很慢　→　只记录，未改测试文件

- 本机基线（子代理前台连跑 2 次，8 核，node v24.14.1）：
  | | real | user | sys | 退出码 | `# duration_ms` | 开跑时 load average |
  |---|---|---|---|---|---|---|
  | Run 1 | **4m9.372s（249.4s）** | 2m36.054s | 0m9.004s | 0 | 248980.455 | 2.85 3.03 3.19 |
  | Run 2 | **4m19.283s（259.3s）** | 2m51.597s | 0m9.151s | 0 | 259177.673 | 5.15 3.85 3.46 |
  两次都是 `tests 54 / pass 54 / fail 0 / skipped 0 / cancelled 0`，**无 worker failure、无超时、无非零退出**（R118 台账所述「文件级 worker failure」在本机同样未复现，「超过 4 分钟未结束」确实是慢而非挂）。Run 2 比 Run 1 慢 4.0%，与 load average 上升一致。
- 54 个子测试耗时之和 247,958 ms / 258,161 ms，与 `duration_ms` 只差约 1.0 s → runner 开销可忽略，基线就是测试体本身。单条 `R86 ADD-1 card detail, metric, legacy damage controls and timeline stay local at 200 matches` 占 **66.2s / 68.1s（≈27%）**，是将来拆分或并行化收益最大的一条；前 12 条合计 ≈64%。
- **CI 超时阈值核对**：`.github/workflows/ci.yml` 里 `jobs.quality` 与 `jobs.windows-build` **都没有 `timeout-minutes`**，走 GitHub 默认 **360 分钟**，远大于该文件耗时（250 s）的 2 倍 → 按工单判据**不需要另立条目**。
- ⚠️ 一个必须记录的相互作用：本轮 P1-2 给 `jobs.quality` 上了**显式键允许清单**，`timeout-minutes` 刻意不在清单内（工单 P1-2 第 3 条点名要求）。所以将来若真要给 quality job 设超时，`R117 real Chromium guards...` 这条测试会红（变异 X4 已验证），必须**同时**改测试里的允许清单——这是有意的显式决策点，不是漏配。
- 台账里的数字是**本机**实测；工单要求的是「CI runner 上的实测耗时」，本轮**没有 GitHub runner 可用**，这一项如实标注 `待 CI 实测`（判据本身只要求「台账里有实测耗时 + 阈值有余量」，两条都满足）。

### P3-2　设计 token 单一来源检查的扫描范围　→　按要求只存档，未改代码

- 复核成立：`backend/web/r117-style.test.cjs:6-11` 只枚举 `backend/web/`，正则 `/\.(?:js|html)$/`；`desktop/` 下是 `.cjs`，不在扫描范围内，工单原文「`desktop/*.js`」的表述不准确。
- 本轮**没有扩大扫描范围**（延续 R118 P3-9 的判断）。存档提示：将来若扩，需同时放行 `.cjs`，并先跑一遍确认不会撞上 `desktop/main.cjs`、`overview-banner-layout.cjs`、`share-export.cjs` 里既有的 hex 字面量（它们目前都不在 `tokenHexes` 白名单内，一扩就会红）。

### P3-3　R116 真机回填　→　**未做，需要用户操作**

本轮没有任何真机数据可用，12 项「待真机」一项都没动，也没有把任何观测值当成已成立。最关键的是 R116-探测三项观测值至今为空（直接决定「海克斯三选一实时排序」做不做、出装联动能否接线），需要用户打一局海斗、带 `R116_AUGMENT_PROBE=1` 走 `docs/r116-probe-findings.md` 与 `docs/r116probe-execution-ledger.md` 的步骤回填；R119/R121 的「补位」判定依赖 `individualPosition` 语义，同样只有手写测试桩，建议一并抽样核对。

---

## 2. 全量回归（收工前，真树，23:05–23:16）

| 检查 | 命令 | 结果 |
|---|---|---|
| gofmt | `gofmt -l backend/*.go` | 无输出 |
| CI 的 gofmt 命令 | `find ... -name '*.go' -print0 \| xargs -0 gofmt -l` | 无输出 |
| 构建 | `go build -o $PI_SCRATCH_DIR/dl-backend-r120 ./backend` | OK |
| vet | `go vet ./backend` | 无输出（rc=0） |
| 整包 race | `go test -race -count=1 ./backend` | **`ok 266.298s`，0 个 `--- FAIL`** |
| 前端全量（CI 同一条命令） | `node --test backend/web/*.test.cjs desktop/*.test.cjs` | **885 tests / 884 pass / 0 fail / 1 skipped**，`duration_ms 264931.5`，exit 0 —— 与工单「已复核无需返工」记录的基线逐项一致 |
| 语法 | `node --check` × 3（`r117.test.cjs`、两个 browser 脚本） | OK |

隔离快照里另跑过整包 `-race` 连续 3 次全绿（266.9 / 272.8 / 264.1 s），与真树结果一致。

---

## 3. 并行会话冲突记录（**必读**）

本轮执行期间，同一工作树里有另一个会话在做 **R121（补位标签）**。时间线（全部来自 `ls -la` 与 `git status` 实测，非推测）：

| 时间 | 对方动作 | 对本轮的影响 |
|---|---|---|
| 21:51 | `backend/riot_api.go` 改动 | 无 |
| 21:53 | `backend/gameplay.go` 改动：`participantCompletenessSummary.AutofillSwapped` → `PositionMismatch`（注释里写明「R121 P1-2」） | 顺手修掉了 P1-1 的 gofmt 问题；同时让 `backend/r119_autofill_test.go:179` 编译不过 |
| 21:54 | `backend/web/gameplay.js`、`backend/web/r119.test.cjs` 改动 | 无（前端全量 23:05 跑时已稳定，885/884/0/1） |
| 21:57 | 我在**仓库**里第一次跑 `go test` → `summary.AutofillSwapped undefined`，整包**不能编译** | 从这里开始，本轮所有 Go 长测与全部变异改在隔离快照里做 |
| 22:17 | 对方更新 `backend/r119_autofill_test.go` | — |
| 22:27 | 对方新建 `backend/position_contract_probe.go`（未跟踪），`gofmt` 不干净 | — |
| 22:29 | 仓库 `go vet ./backend` 失败：`position_contract_probe.go:214: declared and not used: paths` | 仓库整包再次不能编译 |
| 22:47 | 对方修好 `position_contract_probe.go`（19,635 B） | — |
| 23:10 | 复查：`gofmt -l` 无输出、`go vet ./backend` rc=0、`go build` OK | 于是整包 race 与前端全量都改在**真树**里重跑，结果见 §2 |
| 23:1x | 对方新建 `backend/augment_contract_probe.go`、`augment_contract_probe_test.go`；`desktop/package.json` 版本 0.12.13 → **0.12.14** | 见 §4 |

**本轮的隔离纪律**：
- 快照两次：`$PI_SCRATCH_DIR/mut120/go`（21:55:59，仅 `go.mod`/`go.sum`/`backend`）与 `$PI_SCRATCH_DIR/mut120/go3`（22:48:49，rsync 全仓，排除 `.git`/缓存/`node_modules`/`dist`/`output`，`desktop/node_modules` 用软链）。P2-3 的 8 组变异在 `go2`（`go` 的副本）里做。
- **所有变异只在快照里做**，还原一律「基线整文件拷回 + `diff -q`」，全程没有执行过 `git checkout -- <file>` / `git stash` / `git clean`。
- 快照里为了让包能编译，就地改过两处**对方文件**（`r119_autofill_test.go` 的 `AutofillSwapped`→`PositionMismatch`；无其它）——只存在于快照，仓库里这两个文件本轮**一个字节都没动**。
- 第一个快照缺 `../docs`、`../README.md`、`../desktop`，导致 4 条路径依赖测试（`TestDesktopStartupStageShell*` ×2、`TestR102SeedAccountsMatchVerificationDoc`、`TestR105_EmbeddedAccountCounts`、`TestSessionTokenPrivacyDocumentationMatchesDiskPersistence`）在快照里假红；`go3` 补齐兄弟目录后这些全部转绿，**§2 的整包 3 次全绿是在补齐后的快照与真树上分别拿到的**。

---

## 4. 需要用户决定的遗留项

1. **版本号**：`desktop/package.json` 已被并行会话改成 **0.12.14**。本轮**没有再动它**（避免与 R121 撞车）。R120 的改动是否要单独出 0.12.15、还是随 R121 一起发 0.12.14，请定；按项目惯例「改动后必须重新构建并确认版本号最新」，打包前需要确认这一条。
2. **P1-4 的第 11、12 条声明**覆盖的是根级散文件（`session-token` / `convenience.json` / `champion-network.json` / `update-*.json`），超出「未声明目录」的字面范围。按你「确实未声明的都补进」的口径写进去了；若要收窄，删这两条即可，但 `session-token` 会重新变成未声明项。
3. **`stores` 是否要加条数棘轮**：既有 `TestPrivacyListsEveryClientWrite` 对 `explicitWrites`=11、`automaticWrites`=10 有条数断言，对 `stores` 没有。本轮新测试用「目录 → 关键词」的结构性对照，比条数棘轮更强，所以**没有**另加条数断言；若你想要双保险，说一声。
4. **P3-3 真机回填**：只有你能做，见 §1 P3-3。
5. **P3-1 的 CI runner 实测**：本机基线已入账，GitHub runner 上的实际耗时仍 `待 CI 实测`（下一次 push 之后看一眼 quality job 的 `Test frontend and desktop renderers` 步骤耗时即可；默认 360 分钟阈值余量充足）。

---

## 5. 本轮改动文件清单（11 个源文件 + 本台账，全部由编辑工具直改）

> 下表行数是**本轮的改动量**，不是 `git diff` 对 HEAD 的量——这些文件里还叠着 R116–R121 的未提交改动，`git diff --numstat` 的数字会大得多。

| 文件 | 变化 | 属于 |
|---|---|---|
| `backend/web/r117.test.cjs` | +62 | P1-2 |
| `desktop/r100-browser.cjs` | +20 / −1 | P1-3 |
| `desktop/r117-browser.cjs` | +20 / −1 | P1-3 |
| `backend/features.go` | 1 行替换（`stores` 7→12 条） | P1-4 |
| `backend/quality_test.go` | +176（新测试 + 覆盖表 + 豁免表 + `sort` import） | P1-4 |
| `backend/champions_structured.go` | +18 / −10（1177–1194） | P2-1 |
| `backend/qq101_test.go` | +92（泄漏断言测试 + 2 个辅助函数 + `runtime` import） | P2-1 |
| `backend/pro_players_test.go` | +44 / −7 | P2-2 |
| `backend/hexdata_r116a_test.go` | +22 / −3（仅注释） | P2-3 |
| `backend/season_stats_budget_test.go` | +238 / −17（2 条测试 + 3 个辅助函数 + `math`/`reflect`/`time` import） | P2-4 |
| `backend/season_stats_test.go` | +44（1 条测试） | P2-4 |

**刻意未改**：`.github/workflows/ci.yml`（本轮只加测试护栏，不动 workflow）、`backend/hexdata.go`（P2-3 选 A）、`backend/season_stats.go`、`backend/season_stats_budget.go`、`backend/storage.go`、`backend/gameplay.go`、`backend/riot_api.go`、`backend/position_contract_probe.go`、`backend/r119_autofill_test.go`、`desktop/package.json`、`desktop/overview-render.test.cjs`、`backend/web/r117-style.test.cjs`、所有产品文档（`README.md` / `DESIGN.md` / `PRODUCT.md`）。

---

## 6. 复现命令

```bash
cd /Users/ly/personal/personal-work/deep-legends

# P1-1
gofmt -l backend/*.go
find . -type d \( -name '.*' ! -name . -o -name node_modules -o -name vendor -o -name dist \) -prune \
  -o -type f -name '*.go' -print0 | xargs -0 gofmt -l

# P1-3（输出目录指向 scratch，避免写进 docs/r100-validation）
export CHROME_BIN="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
for i in $(seq 1 20); do R100_BROWSER_OUTPUT=/tmp/r100 node desktop/r100-browser.cjs; echo "exit=$?"; done
for i in $(seq 1 20); do R117_BROWSER_OUTPUT=/tmp/r117 node desktop/r117-browser.cjs; echo "exit=$?"; done

# P1-4 / P2-1 / P2-2 / P2-4
go test -count=1 -v -run 'TestPrivacyStoresDeclareEveryLocalDirectory' ./backend
go test -race -count=20 -run 'TestStructuredDetailFallsBackWhenQQ101BuildTimesOut|TestStructuredDetailQQ101TimeoutDoesNotLeakGoroutines' ./backend
GOMAXPROCS=1 go test -race -count=100 -run 'TestProDirectoryPublishesBeforeSlowSupplements|TestProDirectoryFailureRetainsRosterAndAuth' ./backend
go test -race -count=100 -run 'TestProDirectoryPublishesBeforeSlowSupplements|TestProDirectoryFailureRetainsRosterAndAuth' ./backend
go test -count=1 -v -run 'TestSeasonStatsBudgetThirdStageOnlyDropsAugmentSamples|TestSeasonStatsBudgetKeepsAsManyAugmentSamplesAsFit|TestSeasonAugmentSampleForRejectsEveryNonMayhemQueue' ./backend

# 全量
go build -o /tmp/dl-backend ./backend && go vet ./backend && go test -race -count=1 ./backend
node --test backend/web/*.test.cjs desktop/*.test.cjs
```

变异脚本（隔离副本、`diff -q` 还原，全部在 `$PI_SCRATCH_DIR/mut120/` 下）：
- `ci-guard-mutations.cjs` —— P1-2 的 21 组（用法：`REPO=<仓库路径> node ci-guard-mutations.cjs`）
- `go-mutations.cjs` —— P2-1 / GE2 / GE2b / GE4 / P1-4a / P1-4b 共 6 组
- `p13-browser-verification.sh [mut|loops]` —— P1-3 的 20 次循环与 A/B 两种变异（含修复前负对照的做法见 §1 P1-3）
