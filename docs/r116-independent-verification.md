# R116 独立验收：六张工单（探测 / A / B / D / E / F）全量复跑 + 换形状对抗变异

**验收日期：** 2026-09-21　**验收对象：** `docs/r116-execution-ledger.md` 及其六份子台账（`r116probe` / `r116a` / `r116b` / `r116d` / `r116e` / `r116f`）
**验收快照：** 用户工作树的一份隔离拷贝（`desktop/package.json` 版本 0.12.13，含 R117/R118/R119 的后续改动）；另用 HEAD `4ca768ec` 的 `backend/` 做 A/B 对照，用来区分「R116 引入」与「HEAD 上本来就有」。**没有触碰用户工作树里的任何代码文件**，所有变异都在隔离副本里做，每次事后整文件拷回基线并 `diff -q` 确认还原。
**验收方法：** 不信任台账自述；整包重跑；对每张工单的关键判据做「改回原样 / 换一种形状」的变异，看有没有测试变红；对 F 的核心数学用不依赖 Go 代码的方式独立重算。
**姊妹文档：** `docs/r118-independent-verification.md`。

---

## 0. 结论

1. **R116 的功能面基本站得住，没有发现「功能没落地」或「数据算错」的问题。** 构建、`go vet` 干净；Node 全量 885 项 / 884 通过 / 0 失败 / 1 跳过；Go 整包 1456 个测试函数，除下面第 4 条的随机失败外全部通过。F 的核心数学（阶段×稀有度概率、三组之和 = 100%）用夹具原始 JSON 独立重算，与代码和测试期望值一致。
2. **前端护栏很扎实**：对海斗页面 9 种不同形状的变异（较基准符号 / 放大倍数 / 样本档位 / Wilson 显示 / 前端重排 / 冒充官方档位等）**全部被打红**。
3. **后端护栏总体有效，但有 4 处台账声称的保护实际没有起作用**（都不是功能缺陷，是「测试没拦住」）：
   - `TestHexdataJSONKindsSurviveSoftTTLBecauseTheyArePromoted` 的注释声称「忘记 promote 时这里必然失败」，**实测 8 个 `promote(` 调用点里只有 2 个（answer / meta）真正被相关测试打红；`heroes` 那一处整包红的是一个无关的随机失败，跑相关子集是全过的；其余 5 个（postmatch / hextech-insights / augment / rarity / hero-json）删掉后整包 1456 个测试全过**（细节见 §3.2）。我进一步验证：当前调用图里这 6 处 `promote` 在生产上也几乎不可达（每个 loader 先走 `loadHexdataMeta`，`adoptMeta` 会刷新 `BuildChecked`，`refreshBuild` 通道用不上），属于「无害的冗余防御 + 一个夸大了的测试注释」。
   - 赛季缓存预算（`season_stats_budget.go`）第二级「裁 AugmentSamples」：把它改成顺手清空 `Stats`、或一上来就清空全部样本，**整包测试仍全过**——预算三级处置的「先动什么、不许动什么」没有被钉住。
   - `seasonAugmentSampleFor` 里的内层队列门禁（只记海斗）被删掉，整包测试仍全过；原因是外层 `seasonRecordRankedMatch` 已经过滤了，内层是冗余的第二道，没有独立的保护。
4. **整包 `go test -race` 目前不稳定为绿**：我整包跑了 2 次，各红 1 个、且不是同一个，都是**HEAD 上就存在**的问题（不是 R116 引入），详见 §4。
5. **隐私声明缺口仍然存在且更实了**：`features.go handlePrivacy` 的 `stores` 清单里没有 `season-stats/`。这是台账**明示留给你拍板**的既存缺口（R117 工单第 662 行、R116 台账决策表第 10 条），不是隐藏问题；但 R116-E 之后这个目录里多存了「每场海斗的英雄 / 海克斯 / 装备」（`AugmentSamples`），敏感度比之前高了一档。**建议这一条尽快拍板。**
6. **R119 引入一个会让 CI 第一步就红的格式问题**：`backend/gameplay.go` 新增字段 `Autofill` 未对齐，`gofmt -l` 列出该文件。

---

## 1. 全量测试复跑

环境：云端沙箱 Go 1.24.7（`go.mod` 要求 1.24；用文件 GOPROXY 离线构建）、Node 22；源码为工作树隔离拷贝。

| 项 | 结果 |
|---|---|
| `go build ./backend` | 通过 |
| `go vet ./backend` | 空 |
| `gofmt -l backend/*.go` | **`backend/gameplay.go`**（R119：`Autofill  bool` 需对齐，`gofmt -d` 只有这一处）|
| `go test -count=1 ./backend`（1456 个测试函数，非 race，重复多轮，含 8 个 promote 变异各一次整包）| 基线通过；约 100–150 秒/次 |
| `go test -race -count=1 ./backend`（整包，2 次）| 第 1 次红 `TestProDirectoryPublishesBeforeSlowSupplements`；第 2 次红 `TestStructuredDetailFallsBackWhenQQ101BuildTimesOut`（带 2 处 `DATA RACE`）。均为 HEAD 既有 |
| `node --test backend/web/*.test.cjs desktop/*.test.cjs` | 885 项 / 884 通过 / 0 失败 / 1 跳过（Windows 专属发布门禁）|
| `node --test desktop/overview-render.test.cjs`（单独）| 54/54 通过，338 秒 |
| `node desktop/r100-browser.cjs` / `r117-browser.cjs` | 均打印 PASS（`peakImages=5`、`unstyledFrames=0`）；但**退出码不稳定**，见 R118 文档 §4.1 |

未做（没有条件）：Windows 真机、真实海斗对局、GitHub runner。R116 台账里标注「待真机」的 12 项（尤其探测三项观测值）我**没有也无法**验证。

---

## 2. 六张工单逐项复核

| 工单 | 台账声称 | 我做了什么 | 结论 |
|---|---|---|---|
| **探测** | 探测入口 `collectAugmentContractProbe` 是临时入口；`R116_AUGMENT_PROBE=1` 才跑；观测值待真机 | 整包重跑探测相关测试全过；grep 确认该函数**没有生产调用方**（`augment_contract_probe.go:319` 之外无引用，台账已披露为「临时入口」）；环境变量门控的测试存在（`augment_contract_probe_test.go:590`）| 属实。真机三项观测值仍为空，决定「海克斯三选一实时排序做不做」，需要你打一局海斗回填 |
| **A**（hexdata 数据源）| JSON API 接线；Referer；路径白名单；旧构建剪枝（含宽限期、只剪 hexdata- 前缀、读不出的文件跳过）；promote 落盘 | 变异：去掉 Referer（GA1）、把白名单放开到英雄列表（GA2）、剪枝判定取反（GA3）、去掉宽限期（GA5）→ **全部被打红**。删除 `hextech-insights` 的 `promote`（GA6）→ 存活，见 §3.2 | 数据源接线、白名单、剪枝真实有效；**promote 的测试保护被夸大** |
| **B**（英雄页 tab / 较基准 / 样本档位 / 官方档位）| 前端收益率显示、样本极少徽记、不前端重排、官方档位缺失时不冒充 | 9 种前端变异（见 §3.1）→ **全部被打红** | 真实有效 |
| **D**（推荐页克制/协同提示、局内出装解析、队伍画像）| 克制/协同只用本地已缓存页、不发上游请求；按 \|Δ\| 降序封顶 5 条；P1-4 阶段二「已实现未接线」| 变异：克制方向对调（GD2）、置信区间上下限对调（GD3）、排序反向（GD4）、协同去掉封顶（GD5）→ **全部被打红**。P1-4 阶段二两个函数（`gameplayOwnedTerminalItemIDs` / `gameplayNextItemSuggestionFromTrios`）确认无生产调用方，台账已披露 | 真实有效；阶段二是有意未接线，非缺陷 |
| **E**（赛季缓存：放开队列、schema 8→9、预算、AugmentSamples、静态查询）| 420/440 + 2300/2400/3270；schema 9 拒绝所有旧文件；4 MiB 单文件预算三级处置；只存必要字段且只记海斗 | 变异：schema 改回 8（GE1）、队列白名单加入 450（GE3）→ **被打红**；预算第二级改成同时清空 `Stats`（GE2）、一上来清空全部样本（GE2b）、去掉内层队列门禁（GE4）→ **整包存活** | 队列和 schema 有效；**预算处置顺序与内层门禁没有测试保护**（§3.2）|
| **F**（阶段×稀有度分布、慎选判据）| 用 `stage.PickRate` 而不是顶层 `pickRate` 求和；慎选 = pickRate > 同稀有度中位数 且 Δ胜率 < 0 且样本档位非 low | 变异：改用顶层 `pickRate`（GF1）、`Δ<0` 改 `Δ<=0`（GF2）、去掉 low 判定（GF3）、中位数比较 `>` 改 `>=`（GF4）→ **全部被打红**。独立重算（§3.3）与期望值一致 | 真实有效，数据准确 |

---

## 3. 对抗变异明细

### 3.1 前端（`backend/web/*`，`node --test`）：9/9 被打红

| # | 变异 | 结果 |
|---|---|---|
| J1 | 较基准 ×100 去掉 | 红 9 项 |
| J2 | 样本档位 medium 当 low | 红 3 项 |
| J3 | low 样本也显示 Wilson 下界 | 红 4 项 |
| J4 | `cumulative` 口径也显示「较平均」 | 红 1 项 |
| J5 | 海斗路径重新做前端重排（`toSorted`）| 红 2 项 |
| J5b | 换形状：`reverse()` 重排 | 红 1 项 |
| J6 | `stats.tier` 缺失时用行级档位冒充 | 红 2 项 |
| J7 | 较基准符号取反 | 红 8 项 |
| J8 | 舍入后为 0 的小值仍显示「较基准」 | 红 1 项 |

### 3.2 后端（`go test ./backend`）

先跑「相关测试子集」（阶段 1），存活的再跑整包（阶段 2）。18 个变异，阶段 1 打红 14 个；存活 4 个（GA6 / GE2 / GE2b / GE4）都做了整包复核，**整包也存活**。另外把 `hexdata.go` 里 8 处 `p.hexdata.promote(` 逐个删除各跑一次整包：

| 调用点 | 删除后整包 | 说明 |
|---|---|---|
| `answer`（hexdata.go:1475）| 红（`TestLoadMayhemDetailColdHeroCostsFourHexdataRequestsWithWarmSiteMetadata` 等）| 有保护 |
| `heroes`（1864）| 整包红的是无关的 `TestProDirectoryFailureRetainsRosterAndAuth`（该测试单跑 40/40 通过，是负载相关的随机失败）；跑相关测试子集（Hexdata\|Mayhem\|Ranking\|Champion）**全过** → **实质存活** | 没有保护 |
| `meta`（2105）| 红（`TestLoadMayhemDetailRequestsHeroJSONAndMetaOnly`）| 有保护 |
| `postmatch`（2654）| **整包全过** | 没有保护 |
| `hextech-insights`（2871）| **整包全过** | 没有保护 |
| `augment`（3148）| **整包全过** | 没有保护 |
| `rarity`（3180）| **整包全过** | 没有保护 |
| `hero-json`（3318）| **整包全过**（连 `TestHexdataJSONKindsSurvive…` 里明确检查 `hero-json\|157` 落盘的断言都没红）| 没有保护 |

**为什么这个测试拦不住：** 测试断言的落盘键，在正常路径下由 `hexdataClient.load()` 里的 `loadWithStatus(..., persistDisk=true)` 已经写好；`promote()` 只有在 `refreshBuild` 通道（`persistDisk=false`）才是唯一的落盘者。而每个 JSON loader 一进来先调 `loadHexdataMeta`，`adoptMeta`（`hexdata.go:1418`）把 `BuildChecked` 刷成「现在」，于是 `refreshBuild` 分支在这些 loader 里基本走不到。我写了一个探针测试（人为把 `BuildChecked` 拨到 13 小时前再调用 loader）：**即使删掉 `promote`，数据仍然落盘**——印证「冗余」。所以：

- 这不是功能缺陷，promote 是无害的冗余防御；
- 但 `hexdata_r116a_test.go:925–929` 的注释「忘记 promote 时这里必然失败」**不成立**，属于夸大的护栏；下次有人真的重构 `load()` 让 `promote` 变得承重时，这条测试不会替你报警。

**GE2 / GE2b / GE4：**

| 变异 | 原因 | 判断 |
|---|---|---|
| GE2 预算第二级顺手清空 `Stats` | 预算测试只断言「不超预算」「AugmentSamples 被裁」，没断言 `Stats` 与 `RankedMatches` 不许被第二级动 | 真实测试缺口（保护的是「预算超了也不许丢用户的赛季战绩」）|
| GE2b 一上来清空全部样本而不是逐步减半 | 没有断言「尽可能保留样本、裁到刚好合规」 | 真实测试缺口（次要）|
| GE4 去掉 `seasonAugmentSampleFor` 内层队列门禁 | 外层 `seasonRecordRankedMatch` 已过滤队列，内层永远走不到 | 冗余的第二道门，不是缺陷；但它没有独立的保护 |

### 3.3 F 的数学独立重算

用 Python 直接读 `backend/testdata/r116/hexdata-hero-157-stage-rarity.json`（126 条 augments），不经过 Go 代码，按 `stages[].pickRate` 按稀有度求和：

| 阶段 | 有阶段行的条数 | 三组之和 |
|---|---|---|
| 1 | **121**（不是 126，5 条上游没有阶段 1 行）| 100.000% |
| 2 | 126 | 100.000% |
| 3 | 126 | 100.000% |
| 4 | 126 | 100.000% |

与测试期望（`1.000003 / 1.000003 / 1.000001 / 1.000001`、阶段 1 条数 121）一致；用顶层 `pickRate` 求和会得到 3.7916（测试 `hexdata_r116f_test.go:180` 也钉住了这个「错误值」），与 GF1 被打红相互印证。**局限：** 夹具是执行方 2026-09-20 抓取的「上游真实响应」，我没有出网条件，验证的是「计算对不对」，不是「夹具是否确实来自上游」。

---

## 4. 发现清单

按对你的影响排序。**都没有在验收里改动代码，需要修的建议另开工单。**

| # | 级别 | 发现 | 是否 R116 引入 | 证据 |
|---|---|---|---|---|
| 1 | 高 | R119 的 `gameplay.go` gofmt 未通过——CI `quality` 第一步就会红 | 否（R119）| `gofmt -d backend/gameplay.go`：`Autofill bool` → `Autofill  bool` |
| 2 | 中（拍板项）| `season-stats/` 不在隐私声明 `stores` 清单里；R116-E 新增 `AugmentSamples`（每场海斗的英雄/海克斯/装备）| 部分（目录既存；数据种类 R116-E 扩大）| `features.go:744–` `stores` 7 项无 `season-stats`；台账 §10 已披露待你拍板 |
| 3 | 中 | **`champions_structured.go` 生产代码里的真实数据竞争 + goroutine 泄漏**：`loadStructuredDetail` 里两个 goroutine 捕获 `qq101PositionsC` / `qq101BuildC`，父 goroutine 在超时后把它们置 nil，随后子 goroutine 读到 nil 通道，向 nil 通道发送会永久阻塞（泄漏）| 否（HEAD 同样存在）| `go test -race`：写 `champions_structured.go:1285/1286`，读 `1180/1185` |
| 4 | 中 | 随机失败测试：`TestProDirectoryPublishesBeforeSlowSupplements`（`pro_players_test.go:317`，测试文件与 HEAD 一致）；`TestProDirectoryFailureRetainsRosterAndAuth` 在整包负载下红过 1 次、单跑 40/40 通过 | 否 | 见 §1 |
| 5 | 中 | promote 冗余 + 测试注释夸大（§3.2）；预算三级处置顺序无保护（GE2/GE2b）| 是（A / E）| §3.2 |
| 6 | 低 | `seasonAugmentSampleFor` 内层队列门禁无独立保护（GE4）| 是（E）| §3.2 |
| 7 | 低 | 真 Chromium 脚本 PASS 后退出码 1（`rmSync` `ENOTEMPTY`）| 否（R117）| R118 文档 §4.1 |
| 8 | 信息 | 探测入口、P1-4 阶段二函数无生产调用方 | 是，均为台账明示的有意状态 | grep |

---

## 5. 没有验证的东西（避免把「没测」当成「没问题」）

- 台账里所有「待真机」项（12 项），包括探测三项观测值、Windows 真机行为、真实海斗对局下的显示；
- R116 的界面我只跑了 jsdom 测试与变异，**没有在真浏览器里逐页渲染 R116-B/E 的新 tab 看视觉效果**；
- 真实上游数据：无出网，未复抓 hexdata 接口，夹具真实性按台账自述；
- GitHub runner、Windows 打包产物；
- 我没有逐行审阅所有约 3 万行 diff，验收面是「六张工单的关键判据 + 对抗变异」，不是全量代码评审。

---

## 6. 方法论沉淀

1. **「测试断言的状态」和「测试保护的动作」要分开看。** `promote` 那条测试断言了落盘的结果，但落盘还有另一条独立路径，于是断言对被保护的动作（promote）是盲的。验收这类「重复落盘 / 冗余写入」的护栏，必须逐个删除动作本身看整包是否变红。
2. **阶段 1（相关测试子集）存活 ≠ 真的存活，阶段 2（整包）再判**；反过来，整包变红也要看红的是不是相关测试（`heroes` 那一次红的是无关的随机失败）。
3. **随机失败要单独归类。** 本项目 `-race` 整包至少 3 个随机失败源；不归类的话，任何一次红都会被误判成「刚改的东西坏了」，任何一次绿也不代表稳定。
4. **冗余的第二道防线（GE4 那种）不是缺陷，但它意味着「去掉外层过滤」时不会有测试报警**——只要一层被删，另一层就要承重，而它没有自己的测试。
5. 变异始终在隔离副本里做，还原用「整文件从基线拷回 + `diff -q`」，不使用 `git checkout`。

---

## 7. 复现命令

```bash
# 隔离拷贝：把工作树整个拷到一个非 git 目录（含 desktop/node_modules），再拷一份作为 mut/
export GOFLAGS=-mod=mod GOTOOLCHAIN=local          # 离线时另配 GOPROXY=file://<本地代理>
go build ./backend && go vet ./backend && gofmt -l backend/*.go
go test -count=1 ./backend                           # 整包，约 100-150 秒
go test -race -count=1 ./backend                     # 整包 -race，多跑 2 次并归类失败
node --test backend/web/*.test.cjs desktop/*.test.cjs

# promote 变异：在 mut/ 里逐个删除 hexdata.go 中 8 处 `p.hexdata.promote(` 所在行，
# 每次整包运行后从基线整文件拷回并 diff -q
go test -count=1 ./backend

# F 的独立重算：读 backend/testdata/r116/hexdata-hero-157-stage-rarity.json，
# 按 augments[].stages[] 的 stage 与父级 rarity 累加 pickRate，比较每阶段之和是否 ≈ 1
```
