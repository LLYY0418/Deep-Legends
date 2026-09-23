# R116 执行总账（六张工单汇总）

**执行日期：** 2026-09-20 ~ 2026-09-21
**基线版本：** 0.12.7　**最终版本：** **0.12.12**（`desktop/package.json` + `package-lock.json` 已同步，三方依赖 `unzipper@0.12.7` 未动）
**路线图：** `docs/r116-worklist-index.md`　**方案与评审：** `docs/r116-mayhem-data-dimensions-proposal.md`、`docs/r116-proposal-feasibility-review.md`
**本文性质：** 六张工单全部执行完毕后的**汇总总账**。逐张工单的细节在各自的 `docs/r116<x>-execution-ledger.md` 里，本文只记跨工单的事实、裁定、冲突与遗留，**不重复抄录**。

---

## 0. 三句话结论

1. **六张工单全部执行完毕，代码侧判据全部 PASS，三套测试全绿**：`go test ./backend/... -count=1` → **ok 221.658s**；`cd backend/web && node --test` → **622 pass / 0 fail**；`cd desktop && node --test` → **259 pass / 0 fail / 1 skipped**；`go build -o` / `go vet` / `gofmt -l` 全部干净。
2. **凡是需要 Windows 真机 + League 客户端 + 真实海斗对局的判据，一律如实标注 `待真机验证`，没有一条被伪造成 PASS**（汇总清单见第 6 节，共 12 项）。其中最关键的是 R116-探测的三项判据——它们的观测值至今**全部留空**，而这三项决定了 P1-1 的去留与 P1-4 阶段二能否接线。
3. **本轮最大的执行风险不是技术，是环境**：一个并行的 R117 会话在同一个工作树里反复回滚文件，销毁了 R116-B 第一轮 62 分钟的前端成果、三次回滚 `hexdata.go`、删掉过两个新测试文件。完整时间线与防护措施见第 5 节。

---

## 1. 六张工单的交付摘要

| 工单 | 版本 | 交付 | 判据状态 | 账本 |
|---|---|---|---|---|
| **R116-探测** | 不升版（无产品功能） | `backend/augment_contract_probe.go`（429 行，独立谓词 `(?i)cherry\|augment\|mayhem` + openapi 全量扫描 + `/help` 格式无关兜底 + 判据护栏字段）+ 652 行测试；`gameplay.go` 新增**一个** `aramMode && !arenaMode` 分支把既有 `sampleArenaAllGameData` 诊断埋点扩到海斗 | 代码侧全 PASS；**三项真机判据观测值全部留空**（`docs/r116-probe-findings.md` 写成可回填的表格 + 用户操作步骤） | `docs/r116probe-execution-ledger.md`（278 行） |
| **R116-A** | 0.12.8 | Hexdata 英雄详情链路由 HTML 抓取换成 `/api/hexdata/heroes/{id}`；新增 `meta`/`hero-json`/`postmatch`/`hextech-insights` 四个 kind 与真实结构校验；`hexdataMetaSnapshot` 统一 citation；`pruneStaleHexdataBuilds` 按 buildID 回收落盘文件；删掉 10 个因换源而失效的函数 | P0~P3 全部判据 + 4 处对抗变异 PASS；真机清单第 3 项已自动化，前 2 项待真机 | `docs/r116a-execution-ledger.md`（514 行） |
| **R116-B** | 0.12.9 | 英雄详情页 6 段线性 → **概览/构筑/表现三 tab**；P0-1 收益率与「不出这件时胜率」、P0-2 `sampleTier`+Wilson 下界+排序护栏、P0-3 `shared.js` 共享统计口径 + 推荐页常驻页脚、P0-4 召唤师技能数据源切 Hexdata（带胜率）、P0-5 官方档位替代本地自算（行级 + 英雄级 + 「本地估算」小字）、P0-6 阶段 1-4 chips（纯前端、不发请求）、P1-6 22 项表现指标面板（后端算均值） | 全部自动化判据 PASS；真机 4 项 + 真 Chromium 截图待真机 | `docs/r116b-execution-ledger.md`（696 行，含第 13 节评审整改与第 14 节主控裁定） |
| **R116-D** | 0.12.10 | P1-2/P1-3 克制与协同**行内小条**（命中才出现、没命中零 DOM、只拉本人英雄一次）；P1-5 队伍画像缺口标签（纯函数、无魔法数字）；P1-4 **阶段一**（`playerlist.items` 解析 + `live_client_items_parsed` 诊断 + 克隆点深拷贝）；P1-4 **阶段二只交付纯函数 + 单测、不接线** | P1-2/3/5 判据全 PASS + 3 处对抗变异实跑；P1-4 阶段一 PASS、阶段二按工单要求**明确不接线**；真机 2 项待真机 | `docs/r116d-execution-ledger.md`（619 行） |
| **R116-E** | 0.12.11→0.12.12 | 放开队列过滤（420/440 + 海斗 2300/2400/3270，两处判据收敛到同一函数）+ schema **8→9**；新建 `season_stats_budget.go`（4 MiB 单文件预算、三级处置、一次性目录体检）；`seasonAugmentSample`（只存必要字段、只记海斗）；P2-6 静态查询（纯函数 + HTTP 出口 + 构筑 tab 渲染）；**用户裁决的额外范围**：队列切换器第三个「海克斯大乱斗」tab + 文案同步 + 闭合 `rankedQueueLabel` 未定义的既存生产缺陷 | P0~P3 判据全 PASS + 5 处对抗变异实跑；真机 2 项待真机；隐私声明按四步流程核实完毕（**未改 `features.go`**，见第 4 节） | `docs/r116e-execution-ledger.md`（979 行） |
| **R116-F** | 工单标注 0.12.12，实际执行序在 E 之前 | P1 英雄专属「阶段 × 稀有度」概率（`computeHeroStageRarityDistribution`，签名与工单逐字一致）；P2 负向推荐慎选判据（同稀有度中位数 + `deltaWinRate<0` + `sampleTier!="low"`，客观陈述文案、禁用「陷阱」）；P3 平衡参数探测 → **明确放弃** | **全部判据自动化闭合，无一条待真机**（这张工单本来就没有真机清单） | `docs/r116f-execution-ledger.md`（500 行）+ `docs/r116f-balance-param-findings.md`（222 行） |

### 规模
- **新建代码/测试 16 个文件、8,635 行**：`augment_contract_probe.go`、`season_stats_budget.go`、`shared.js` 三个生产文件 + 10 个 Go 测试文件 + 4 个前端测试文件（`r116b/d/e/f.test.cjs`）
- **新建夹具 5 份**（`backend/testdata/r116/`，全部由真实上游响应裁剪，共约 200 KB）
- **R116 专属测试：Go 101 个测试函数 + 前端 72 个用例**
- 主要文件增长：`hexdata.go` 2418→3934、`gameplay.go` 8779→10100、`season_stats.go` 754→1215、`champions.js` 2420→3074、`champions.go` 2879→3048、`gameplay.js` 6964→7299、`champion_cache.go` 528→603、`champions.css` 933→988、`gameplay.css` 1877→1899、`index.html` 382→383
  （`gameplay.go`/`season_stats.go`/`champions.js`/`champions.go` 的增长里**混有 R117 的改动**，不全是 R116 的）
- **文档 8 份共 4,984 行**（7 份工单账本/findings + 本总账）

---

## 2. 执行顺序与版本号重映射

工单内标注的版本号假设「六张按序执行、彼此不冲突」。实际执行序与并行度如下，版本号按**实际完成顺序**重映射（`r116-worklist-index.md` 第 5 节明写「实际以最后完成的那张为准」）：

| 实际序 | 工单 | 工单标注 | 实际升到 | 说明 |
|---|---|---|---|---|
| 1 | R116-探测 | 不升版 | — | 与 A 并行 |
| 1 | R116-A | 0.12.8 | **0.12.8** | 与探测并行（文件不相交） |
| 2 | R116-B | 0.12.9 | **0.12.9** | 必须等 A（前端消费新数据结构） |
| — | B 的独立评审 + 整改 | 无 | 不升版 | 整改属 B 范围 |
| 3 | R116-D | 0.12.10 | **0.12.10** | 与 F 并行（D=`gameplay.go`+`gameplay.js`，F=`hexdata.go`+`champions.js`，不相交） |
| 3 | R116-F | 0.12.12 | **0.12.11** | 与 D 并行；版本号下调以匹配实际执行序 |
| 4 | R116-E | 0.12.11 | **0.12.12** | 押最后：它要改 `season_stats.go`（R117 也在改）与 `gameplay.go`/`gameplay.js`/`champions.js`（D/F 刚改完） |

**D 与 F 的版本号由主控统一收口**（两个子任务都被明确要求不改 `desktop/package.json`，避免并行写冲突），最终停在 **0.12.12**。

---

## 3. 本轮实测推翻或修正的文档结论（14 条）

工单、评审、方案、以及**我自己写给子任务的契约**都有被真实数据/真实代码推翻的地方。逐条留档，后续引用时以这里为准：

| # | 原文断言 | 实测/核实结论 | 发现者 |
|---|---|---|---|
| 1 | 评审 §3.1：协同覆盖率 **18.8%** | **≈15.4%**。18.8% 是按 n=5 算的（把本人也算进抽样），但 `teammateSynergies` 是「我与队友」的表，本人不可能命中自己 → n=4。克制那条 n=5 是对的（复算 34.8% ≈ 评审 34.6%） | R116-D |
| 2 | **主控契约**：「阶段 1 棱彩 47% 明显高于阶段 2-4 的 ~27%，说明英雄专属口径比全服口径多出信息量」 | **错**。抓真实全服 `/augment-rarity`（patch 16.18，200 / 9,757 B）：全服阶段 1 自己就是白银 8.4 / 黄金 46.6 / **棱彩 45.0**，阶段 2-4 是 ~28.6/43.8/27.6 —— 那个偏斜是**模式级现象**。4 个英雄逐阶段作差：阶段 1 最大仅 +1.98pp，阶段 2-4 最大 0.38pp。两个视图按工单都保留，但 UI 与注释**不再宣称量级差别** | R116-F（推翻主控） |
| 3 | 工单 B P1-6：「22 项指标每项都要带『较全英雄平均 ±%』」 | **数据上不成立，只有 18 项可比**。`doubleKills`/`tripleKills`/`quadraKills`/`pentaKills` 是**累计次数**，实测 `pearson(英雄总场次, doubleKills)=0.8117` → 对它们做 ±% 量到的是人气不是强度。处置：后端加 `Cumulative` 标志、`DeltaPercent` 恒 0，前端去掉 ±% 并标注「累计次数（受出场场次影响，不可跨英雄直接比较）」。`multiKillRate` 的口径**无法从公开字段反推**（实测 ≠ (d+t+q+p)/games，比值 1.16~1.25 不恒定），故原样展示、不加解释性文案 | 主控实测 |
| 4 | 工单 B：详情页是「6 段线性堆叠」且含海克斯图鉴 | 实测详情面板只拼 **5 段**；图鉴与稀有度分布早已是 `state.mayhemView === "atlas"` 的**平行视图**。本轮补的是工单真正缺的那一半——详情面板顶部的独立入口按钮（复用既有状态机、无新路由） | R116-B |
| 5 | 工单 D 的结构体示例 `json:"confidenceLow,confidenceHigh"` | **笔误**。Go 的 tag 会把它解析成「名字 `confidenceLow` + 选项 `confidenceHigh`」，两个字段共用一个键、`confidenceHigh` 永远不出现在响应里。已按两个独立字段各带 tag 实现，有回归钉住 | 主控 + R116-D |
| 6 | **主控契约附五**：`gameplay.go:3447` 是「对位/counter 统计」 | **错**，那是 `championStats`（英雄胜率卡片的兜底），不被任何队列页签消费，所以「海斗 tab 对位为空」这个后果不成立。已按实测处置（**刻意保留** 420/440 门禁 + 理由注释） | R116-E（推翻主控） |
| 7 | **主控契约附一**：R117「删掉了 `CSPerMinute`」 | **错**。实测 `gameplaySeasonChampionStat` 仍有该字段、`seasonStatsFinalize` 仍在算。未动（禁触） | R116-E（推翻主控） |
| 8 | 工单 E P3：augment 样本「约 300 B/场」 | 300 B 是把伤害/经济算进去的口径；按 Anti-scope 第 4 条只存必要字段后**实测 126 B/场** | R116-E |
| 9 | 工单 E P2：单文件「约 60 KB → 120 KB」 | 那是**不含 augment 样本**的口径；含样本后**实测 305.2 KB**（2000 场），对 4 MiB 上限余量 **13.4×**。两组数字都写进了 E 的账本 | R116-E |
| 10 | 工单 E P1 引用的注释「每页 20 场」 | **注释本身是错的**：实际 `sgpPageSize = 50`，还有向 `sgpFallbackPageSize = 20` 降级的路径 → 真实单页 20~50 场波动，前台 2 页 = 40~100 场、后台 12 页 = 240~600 场。**注释已改对** | 工单自己指出，E 落实 |
| 11 | 工单 A P1 判据 1：「改动后冷启动应为 **2** 条请求」 | 真·全冷启动是 **3** 条（`meta` + `hero-json` + `answer-cards`；`answer-cards` 是 `measurementTechnique` 的唯一来源，删掉它会让统计口径说明消失＝触数据准确性红线，故保留）。B 接入官方档位与表现面板后，**详情页与榜单页的冷启动都是 4 条**，均已在测试里显式记账（含「重启后 0 条追加」与「官方档位确实生效」两道断言，避免多付的请求变成白付） | R116-A / 主控 |
| 12 | 工单 F P3：「`yasuo.bin.json`（200，**无**）」 | 需**精确化**为「200，**含 ARAM 逐技能覆盖但不含英雄级平衡乘数**」。亚索文件里 `"ARAM"` 出现 2 处（hash 键 `{f9c2333e}` 下的 `SpellEffectAmount` 覆盖 `[27,27,25,23,21,19,19]`；`ItemRecommendationOverride` 的 `mMapID:12`）；提莫对照样本同样 2 处，`DamageDealt` 命中 0、`Modifier` 命中 0。写清区别，否则会误导后来人以为 CommunityDragon 完全没有 ARAM 维度 | 主控实测 |
| 13 | 工单探测 P0 判据 1：「`count == 0` → **确证**客户端没有任何海克斯端点」 | **有假阴性漏洞**：`docs/history/DIAGNOSIS-R82-CURRENT-GAME-MATRIX-RESULT.md` §4 记录本机 `/swagger/v3/openapi.json` 与 v2 **双双 404**、只有 `/help?format=Full` 200。404 时 `count` 也是 0，照原文会写成「确证无端点」。已加 `contract_read`/`negative_conclusive` 护栏（**只有真读到契约时 0 才算否定证据**）+ `/help` 格式无关兜底，判据分流扩成 A/B/C/D 四种 | R116-探测 |
| 14 | 工单探测第 19 行：「`/lol-cherry*`、`/lol-mayhem*`、`/lol-augment*` 命名空间从未出现在代码里」 | 前半句成立，但**谓词必然有一个无关命中**：`/lol-game-data/assets/v1/cherry-augments.json` 在代码里（`gameplay.go` 的 `loadGameplayAugmentsFromClient`、`perk_catalog_async.go:42`），同时含 cherry+augment。已在 findings 里列为「已知无关命中」，要求判读时先排除 | R116-探测 |

---

## 4. 裁定记录（主控作出的判断，含一处已批准的越界）

| # | 事项 | 裁定 | 依据 |
|---|---|---|---|
| 1 | **`backend/main.go` 越界 2 行**（E 报告）：加了 1 句注释 + 1 条路由 `GET /api/gameplay/season-mayhem-builds`（`a.authorized` 包裹） | **批准保留** | 工单 E 交付物第 3 条明写「P2-6 的静态查询函数**与前端渲染**」。路由注册只可能在 `main.go`；没有它前端拿不到数据，P2-6 就成了「算了但取不到」的死数据——那正是用户裁决「让新数据有入口」要避免的缺陷类型。纯追加、未改任何既有行、沿用既有命名与鉴权惯例。**回退方式：删掉这 2 行即可，其余改动不受影响**（`git diff backend/main.go` 里其余部分是 R117 的图片 ETag/负缓存改动，与本条无关） |
| 2 | **评审整改 B6 把阶段行的 `pickRate` 裁掉** | **推翻，已恢复** | 工单 B P0-6「实现要求」第 1 条明写点击 chip 后要重渲染该阶段的 `winRate`/`deltaWinRate`/**`pickRate`**。B6 只是独立评审的**建议项**（归在「建议修但不阻塞」），理由是「前端零渲染」——但正确解法是把这一格**渲染出来**，而不是让工单点名的字段少一个。二者冲突时**工单字面优先**。B6 的「进」那一半（`stageBaselineWinRate` 可见一格 + 装备排行 tooltip）**保留**。代价 +10.9 KB/英雄（详情页），而整改 B5 已为局内链路省掉 97 KB，净账仍为正。两侧对抗变异都实跑过：前端删掉「选取率」格 → 2 条测试 FAIL；后端删掉 `PickRate: row.PickRate * 100` → 2 条 Go 测试 FAIL。详见 `docs/r116b-execution-ledger.md` 第 14 节 |
| 3 | **P1-4 阶段二是否接线** | **不接线** | 工单 D「对抗变异」最后一条明写「若阶段二判定为不可行，本工单只交付阶段一的诊断解析，不强行实现阶段二——**不接受『猜测字段结构强行实现』的交付**」。探测的三项真机判据观测值至今全部留空，即「可行」**未被证实**。处置：阶段二的纯计算（`terminalItemTrios` 前缀匹配）已实现并有完整单测，但**零生产调用方**，且用 `TestR116DStageTwoStaysUnwired` 钉住。接线条件与两处接线点写在 D 的账本 §4 |
| 4 | **ChampSelect 阶段是否展示「对面 5 人」的克制** | **不展示** | 工单 D Anti-scope 第 4 条 + 探测未证实 `their_team_length > 0`（仓库里 `theirTeam` 的 fixture 无一例外是 `[]`，docs 里零真实观测）。P1-2 协同只需我方，不受影响。这是**基于「探测未证实」采取的保守分支**，真机证实后放开的具体位置已写在 D 的账本里 |
| 5 | **P1-5 的 `avgHealShield` 是否参与判定** | **不参与**（对工单的收窄，已显式记录） | 工单列了 5 项指标，但三个标签（缺前排/缺控制/缺持续输出）没有一个能给 `avgHealShield` 一个可辩护的判据位置，硬塞就是编造口径。它照常算进均值与诊断路径。D 的账本 §8 第 3 点显式记录，未静默处理 |
| 6 | **`seasonAugmentSample` 用切片还是定长数组** | **切片 + `omitempty`** | 工单字面写 `AugmentIDs[6]`/`ItemIDs[7]`。定长数组会把空槽位序列化成 `[0,0,5001,0,0,0]`，与工单自己要求的「精简 0.6 MB 路线」相悖。槽位上限由 `seasonAugmentSlots=6`/`seasonItemSlots=7` 两个具名常量表达并有测试断言 |
| 7 | **`gameplay.go` 的 `championStats`（英雄胜率卡兜底）是否放开海斗** | **刻意保留 420/440 门禁** | 放开会把海斗混进一张写着「本赛季 · N 场排位」的卡＝口径伪造。海斗的英雄级统计走赛季 `Stats`（P0 已放开），不会因此取不到。连带把前端兜底标签从「场排位」改成「场对局」 |
| 8 | **`positionStatsForQueue` 对海斗的早退** | **从「回退全量」改成 `return nil`** | 海斗是 ARAM 系、**没有分路**。原来的早退会 `return positionStats(matches, ...)`（不分队列的全量统计），那等于把单双排的分路数据挂在「海克斯大乱斗」标签下。第二道闸：海斗页签根本不调用它、也不下发 `Positions`/`Ability`。对抗变异实跑 FAIL（`positionStatsForQueue(2400)` 返回 `top Games:3`，正是那 3 场 420） |
| 9 | **海斗按钮出现在哪些 scope** | **只出现在「近期战绩」** | 给 ability/position 也加海斗按钮会导致「整块隐藏后切换器也消失、再也点不回来」的死胡同（与整改 A1 同类）。代价是三个切换器按钮数 3/2/2，并在海斗卡里写明「海克斯大乱斗没有分路，不提供位置胜率与能力雷达口径」 |
| 10 | **隐私声明文案是否修改** | **不改 `features.go`，只核实 + 给建议条目原文** | R117 工单第 662 行已把「`season-stats/` 未出现在 `stores` 清单里」标记为「需要用户拍板，按项目惯例（R92 教训）不代为决定」。核实结论：本轮改动**不让任何现有文案变成假的**；产品文档侧（`README.md`/`DESIGN.md`/`PRODUCT.md`）**没有任何一处陈述过「赛季统计只含 420/440」**，故无需改动；唯一缺口是既存的 `stores` 清单。建议条目原文（A/B 两版，推荐 A）在 E 的账本 §9.2，标明「待用户/R117 侧拍板后写入 `features.go:752`」 |
| 11 | **`CHANGELOG.md` 是否更新** | **不更新** | 不在任何工单的交付物清单里，且紧邻的 R117 那轮也没更新它（该文件按轮次记条目，但看起来是在发布/打包时统一补，而「用户通常自己打包」）。按「超范围改动零容忍」不动，仅在此列为建议后续项 |
| 12 | **D 报告的两处技术债**（`renderLiveInsights` 的 helper 定义在函数体内、`ensureLiveRecommendations` 的 `typeof` 护栏） | **已在评审整改中清掉** | 它们是 D 为避开与 F 的并行冲突而做的妥协（`champions.test.cjs` 用 `compileFunctions` 单独编译、依赖列表固定）。F 收尾后 helper 已提到模块作用域、护栏已删，三处测试的注入列表同步补上（传递编译**真实实现**，不打桩） |
| 13 | **D 提请裁决：阶段二接线可能要改 `main.go`** | **本轮不决定** | 阶段二本来就不接线（裁定 3），这个选择等探测的真机判据回填后再做。两条路（走查询参数 vs 在 `app` 上开快照读取口）都记在 D 的账本 §11.3 |

---

## 5. 并行会话（R117）冲突的完整记录

**这是本轮最需要留档的执行事实。** 用户在本轮期间同时运行着另一个会话执行 `docs/WORKLIST-R117-FULL-PROJECT-OPTIMIZATION.md`（33 条全项目优化），两个会话共用同一个工作树。

### 时间线（全部有 mtime / 内容证据）
| 时间 | 事件 | 损失 | 处置 |
|---|---|---|---|
| 18:22 | 首次发现 `season_stats.go` 被改（schema 7→8、K/D/A 改 float64 逐场均值）、`README.md`/`DESIGN.md` 被改、新建 `readme_privacy_test.go` | 无（识别为 R117 的 P0-2/P0-6/沉淀要求） | 上报用户 → 裁决「A/B/D/F 照跑，E 押最后，schema 顺延到 9」 |
| 18:48 / 19:08 | `hexdata.go`、`hexdata_test.go` 被回滚成 HEAD（**两次**） | A 的执行方自己从 Edit 快照恢复 | A 在报告里如实记录 |
| ~20:00 | **删掉** `augment_contract_probe_test.go` 与 `champion_cache_r116a_test.go` | 2 个测试文件消失 | 主控从 `r116-snapshot-phase1` 恢复，重跑 `-run R116` → ok |
| **20:42** | `hexdata.go` **完整回滚到 HEAD**（2418 行），A 的 1416 行 + B 的追加全丢；`hexdata_test.go` 同步回滚 | A 的成果从工作树消失 | 主控从 B 的 `hexdata.go.b-work`（3631 行，含 A+B）与 guard 快照恢复 |
| **20:46→20:54** | `champions.js` 从 2752 行退回 2428 行（11 处 `mayhem-detail-tab` 归零）、`shared.js` 被删、`index.html` 的引入被删 | **B 第一轮 62 分钟的前端成果全部丢失，且无任何备份** | 无法恢复 → 停掉 B、上报用户 |
| 21:0x | 用户裁决「继续，主控负责守护和恢复」 | — | 建立 `r116-guard`/`r116-full-backup` 快照 + 完整性核对脚本 |
| ~21:1x | 又一次删除 6 个 R116 文件（3 个测试 + 3 份账本/findings） | 再次消失 | 全部从 guard 恢复，编译 + 测试复核通过 |
| 10:22（次日） | 用户确认 R117 已停（连续 8 分钟零改动） | — | 重做 B（第二轮，要求**每写完一个文件立刻增量备份**） |
| 15:10 / 15:45-15:47 | R117 侧再次活动（改自己的 `r117*.test.cjs`、`docs/r117-execution-ledger.md`、跑浏览器验证） | **无 R116 损失** | 主控启用高频完整性核对（`r116-integrity.sh`，19 个检查点），全程绿 |

### 结论与教训
1. **损失全部集中在 B 的第一轮**：62 分钟的前端成果因没有增量备份而彻底丢失，重做花了约 90 分钟。A/D/E/F 与 B 第二轮**零损失**——差别只在于有没有「每写完一个文件立刻 `cp` 到 scratch」这条纪律。
2. **恢复能力来自三层快照**：`r116-snapshot-phase1`（阶段收口）、`r116-guard`（滚动守护）、各子任务自己的 `r116<x>-backup`（增量）。子任务自己的增量备份最关键——`hexdata.go` 能从 20:42 的完整回滚里救回来，靠的正是 B 在 20:21 存的 `.b-work`。
3. **`git` 帮不上忙**：R116 的全部成果都是未提交的新增/修改，`git reflog` 与 `git stash list` 干净，对方的回滚是直接 `git checkout -- <file>` 或用自己的备份覆盖，事后无法从 git 追溯。
4. **建议（写给后续轮次）**：多会话共用一个工作树时，要么串行、要么各自 `git worktree`。本轮用户选择「串行化 R116-E + 主控守护」是有效的折中，但代价是一次 62 分钟的返工。

---

## 6. 待真机验证项总清单（12 项，全部未伪造）

以下判据**需要 Windows 真机 + League 客户端**（多数还需要真实海斗对局），本环境无法执行。每项的操作步骤、要看哪个诊断事件/文件的哪个字段、期望值，都写在对应账本里。

| # | 工单 | 待验证项 | 期望 | 出处 |
|---|---|---|---|---|
| 1 | 探测 | 客户端登录态触发 `augment_contract_probe`，检查 `count` 与 `contract_read`/`negative_conclusive` | 按 findings §2.3 的 A/B/C/D 四种分流判读；**只有 `contract_read=true` 时 `count==0` 才算否定证据** | findings §2 |
| 2 | 探测 | 打一局海斗（KIWI/ARAM_MAYHEM），完整走完 ChampSelect → InProgress → 正常结束，导出日志核对三个事件 | `live_client_playerlist_shape.element_keys` 是否含 `items`；`lcu_champ_select_session_shape.their_team_length` 是否 >0；`live_client_allgamedata_shape.top_level_keys`（本轮新增触发后才会出现） | findings §1/§3 |
| 3 | 探测 | 真机诊断日志导出文件存档 | 建议路径 `docs/r116-validation/augment-contract-probe.jsonl` 与 `mayhem-live-shapes.jsonl`（该目录本轮**未创建**） | findings |
| 4 | A | 打开一个此前从未访问过的海斗英雄详情页，核对 hexdata 请求数 | 真·全冷启动 **4** 条（`meta` + `hero-json` + `hextech-insights` + `postmatch`，见第 3 节第 11 条），`answer-cards` 命中缓存后不再计 | A 账本 §7 |
| 5 | A | 连续打开 5 个不同英雄详情页 | 诊断里**没有** `hexdata_circuit_open` | A 账本 §7 |
| 6 | B | 打开至少 5 个不同英雄详情页，三个 tab 都能切换且内容不串 | 每个 tab 只渲染自己的内容；隐藏 tab 不占布局高度 | B 账本 §8 |
| 7 | B | 阶段 chip 点击后数字正确变化且不产生新请求 | jsdom 已自动化覆盖（fetch 全量记账）；真机复核真实网络面板 | B 账本 §8 |
| 8 | B | 召唤师技能区块显示胜率；出门装/鞋子仍只显示选用率 | 混合来源的口径说明同时出现在页脚 | B 账本 §8 |
| 9 | B | 真 Chromium 截图核对：tab 切换、页脚常驻不闪烁、**首屏高度不超过一屏** | 阶段视图现在最多 **8 格**（恢复 pickRate 后），首屏高度需重点看 | B 账本 §8 + §14.6 |
| 10 | D | ≥3 局海斗，记录克制/协同提示的实际出现次数，与覆盖率量级比对 | 克制 ≈**34.8%**、协同 ≈**15.4%**（均为理论推算，见第 3 节第 1 条），不要求精确，用于发现设计误差 | D 账本 |
| 11 | E | 打若干局海斗后，下次总览页扫描出现个人海斗统计 | 「近期战绩」出现第三个「海克斯大乱斗」页签且有数据；位置胜率与能力雷达**整块不出现**（海斗无分路，属正确降级） | E 账本 §2.6 |
| 12 | E | `season-stats/sgp/<accountHash>-s26.json` 文件大小 | 普通玩家 120~180 KB、2000 场高频玩家 **305 KB** 量级，**绝不该到几 MB**（4 MiB 预算有三级处置兜底） | E 账本 §2.6 |

**R116-F 没有任何待真机项**——它本来就没有真机清单，P1/P2 是纯本地计算 + 已有数据，P3 是网络探测，全部判据自动化闭合。

---

## 7. 建议的后续工单（本轮发现但未做，全部有证据）

| # | 事项 | 性质 | 证据位置 |
|---|---|---|---|
| 1 | **删除探测代码**：`backend/augment_contract_probe.go` + 其测试 + 临时入口。工单明写「探测完成、结论落盘后必须整体删除，不进入正式版本——这是一次性侦察代码」。**前提是第 6 节第 1-3 项真机判据先回填**，否则删掉就要重写 | 工单要求的收尾动作，**当前被真机判据阻塞** | 探测账本 §4 |
| 2 | **`gameplay.go:6106` 的海斗诊断分支去留**：工单探测 P1 第 4 条要求按结论评估——若 P1-4/P1-2/P1-3 判定要做则保留为正式埋点，否则删除（回退成本一行） | 同上，被真机判据阻塞 | 探测账本 |
| 3 | **P1-4 阶段二接线**：纯函数与单测已就位、零生产调用方。接线条件＝findings 的 `element_keys` 判据回填为「含 `items` 且结构与斗魂一致」。**接线前必须先用真机日志核对 `self_item_ids` 里有没有合成组件 ID**——`items[]` 没有任何字段能区分成品件与组件（`canUse` 是「能否主动使用」），否则前缀匹配会假命中 | 被真机判据阻塞 | D 账本 §4/§11.2 |
| 4 | **`renderOptionStats` 两处既存缺陷**：症状是「胜率 —」/「选用率 —」**破折号占位格**（不是台账原先写的「0.00%」——`omitempty` 让 Go 的 0 不进 JSON，前端拿到 `undefined` 后 `percent()` 走「—」）。`champions.js:2475` 的默认 statsRenderer（调用点 2319/2321/2322/2323）与 `gameplay.js:6058` 的同形独立实现；后者还有**反向问题**：只有真实 `games` 时整块不渲染，把真数据藏了 | **既存缺陷，非本轮引入**，两处一起改成逐格判断 | B 账本 §11.3（已按实测重写） |
| 5 | **英雄级汇总视图的「较基准」缺可见基准值**：英雄级 `heroWinRate` 没有逐行下发。拒绝用 `winRate − deltaWinRate` 前端反推（＝造数）。正解是详情响应加**一个英雄级标量**（约 30 B） | 需后端小改 | B 账本 §13.6 |
| 6 | **成装三件套 `terminalItemTrios` 未在构筑 tab 消费**：R116-A 已解析并按 `games` 降序裁剪到 top-50，但响应未下发。工单 B 的 tab 内容表明写「若本工单未消费则留作后续」，豁免成立 | 工单允许的遗留 | B 账本 |
| 7 | **`meta.tierBands` 官方中文标签未接**（`前15%`/`15%-35%`/`35%-65%`/`65%-85%`/`后15%`）：需扩后端字段。这是主控实测笔记里的建议，**不是工单要求** | 增强项 | 实测笔记 §4 |
| 8 | **`features.go:752` 的 `stores` 清单补 `season-stats/` 条目**：既存缺口，R117 已提交用户拍板。建议条目原文（A/B 两版，推荐 A：一条覆盖整个 `season-stats/` 并写明 4 MiB 上限与截断顺序）已备好 | **待用户拍板** | E 账本 §9.2 |
| 9 | **`README.md:31` 的总览卡片枚举不完整**：只列「单排/双排、灵活组排」，本轮加了第三个页签后该枚举不再完整（同句里还留着早已删除的「过去 30 天排位」）。改法已写好，**未改 README**（禁触 + 属 R117 地盘） | 由用户裁决的 UI 范围造成，非 P0 造成 | E 账本 §9.3 |
| 10 | **`CHANGELOG.md` 未更新**（本轮与 R117 都没更新） | 见第 4 节裁定 11 | 本总账 |
| 11 | **`seasonAugmentSample` 无 `createdAt`** → 追加顺序 ≠ 时间顺序（后台回补往回翻旧页、追加在末尾），所以超限裁剪只能按 GameID 降序**近似**「新在前」。该假设只决定超限时先丢哪些、不参与任何对外数字。要精确就得加 `createdAt`（+40 KB，仍在预算内），但那要动 Anti-scope 第 4 条的字段清单 | **待用户拍板** | E 账本 |
| 12 | **海斗页签 key 是 `"2300"` 但内容是三个队列（2300/2400/3270）合并**：前端 `rankedQueueData` 用 `String(tab[stateKey])` 做键、键必须是数字串，没有更好的办法。已在 Go/JS 两侧注释 + 测试三处钉住语义 | 已知妥协，留档 | E 账本 |

---

## 8. 最终验证数字（主控亲自复跑，2026-09-21 17:1x）

```
go build -o "$PI_SCRATCH_DIR/dl-build" ./backend   → BUILD OK
go vet ./backend                                    → 无输出
gofmt -l backend/*.go                               → 无输出
go test ./backend/... -count=1                      → ok  lol-loot-assistant/backend  221.658s
cd backend/web && node --test                       → tests 622 / pass 622 / fail 0
cd desktop    && node --test                        → tests 260 / pass 259 / fail 0 / skipped 1
desktop/package.json                                → "version": "0.12.12"
desktop/package-lock.json                           → 顶层与 packages[""] 均 0.12.12；unzipper@0.12.7 未动
R117 样式棘轮（六个预算）                            → 全部未变差
```

**对照基线（本轮开工前，2026-09-20 17:5x）**：`go test ./backend/...` ok 203.331s、前端 530 pass / 0 fail、版本 0.12.7。
→ 净增：**Go 测试时长 203s→222s（新增 101 个 R116 测试函数）、前端 530→622 用例（新增 72 个 R116 用例 + R117 的增量）、版本 0.12.7→0.12.12**，全程**零回归**。

R116 成果完整性核对（19 个检查点，`r116-integrity.sh`）：**全部完好**。
