# WORKLIST-R148：海斗页首次加载慢；梯度榜胜率与详情头部样本一直是「—」；换英雄后停留在上一个英雄的 tab

诊断人：Claude（读了 `lol-loot-diagnostics-0924-1453.jsonl`、用户截图、`backend/hexdata.go` / `champions.go` / `web/champions.js`）。
**本单只出工单，Claude 没有改任何代码**，全部由 GPT 实现、验证、发版。
日期：2026-09-24。基线：0.12.19，含 R144–R147 的改动。
状态：代码与全量验证通过，待真机；版本与打包按用户此前要求暂缓。执行记录见 `r148-execution-ledger.md`。

用户的三个问题：
1. 海斗页第一次请求很慢，能不能结合日志优化。
2. 梯度榜左侧的胜率一直出不来；英雄详情头部的「样本」有没有数据源，没有就删。
3. 切换到别的英雄后不会回到「概览」tab，而是停在上一个英雄的 tab，切换后应还原。

---

## P2 梯度榜胜率与头部样本（先说，因为它也是 P1 的一部分原因）

### 证据

- 截图：左侧梯度榜「胜率」列全是「—」，右上角详情头部「胜率 55.26%」有值、「样本」是「—」、「梯度 T1」有值。
- 日志（`lol-loot-diagnostics-0924-1453.jsonl`，run `54a0ad92…`，每次打开海斗页都重复）：
  - `hexdata_token_acquired kind=heroes ok=true status=200 duration_ms=592`（令牌已拿到，R147 生效）
  - 紧接着 `hexdata_shape_invalid kind=heroes failure_kind=parse fields=519 buildId=""`。
  - 随后一条 `lol-api-champion.op.gg`（8 KB）请求 = OP.GG 榜单回退。
- 代码对应关系：
  - `champions.go loadARAMRankings` 先调 `hexdata.go loadHexdataRankings`；后者靠解析 `/heroes` 的 HTML 表格（`parseHexdataHeroes`）得到每个英雄的胜率和场次。这个解析现在失败（`parseHexdataDocument` 取不到 `buildId`，页面结构或内嵌信息变了），于是整条链失败，回退到 OP.GG `/api/contents/tiers`，那个接口只有 `Rank` 和 `Tier`，没有 `WinRate` / `Play`（`championRankingRow` 里这两项为 0）。
  - 所以榜单胜率列是「—」；详情头部 `metric("样本", compactNumber(row.play))`（`champions.js` 约 1131 行）里的 `row.play` 同样来自这一行，所以「样本」也是「—」。胜率有值是因为它取的是 `detail.stats.winRate`（hero-json），梯度 T1 来自 insights。
- **样本有数据源，不需要删。** `hextech-insights` 的 `heroes[]` 里每个英雄都有 `games`、`winRate`、`pickRate`、`tier`（`hexdata.go` 的 `hexdataInsightHero`，第 588–665 行附近），这份数据本来就在每次详情加载里被取（日志里 `hexdata_shape kind=hextech-insights rows=173`），173 个英雄齐全。

### 要做

1. `loadHexdataRankings` 改成**以 hextech-insights 为主数据源**：`Play = games`、`WinRate = winRate`、`Tier = tier`（官方档位，`TierLocallyCalculated=false`），不再依赖 `/heroes` HTML 表格。这样榜单胜率、头部样本都有值，也去掉了每次打开海斗页多打的一次 `/heroes` 请求和它的令牌获取（见 P1）。
2. 排序（`Rank`）：原来是 `/heroes` 页面里的表格顺序（背后是 answer-cards 的 `sample_tier_then_wilson_lower_bound_v1` 推荐口径，见 `applyHexdataLocalTiers`）。**GPT 先确认现有顺序的来源**，再用 insights 的字段复刻同一口径（样本分档后按 Wilson 下界），并在浏览器里对照官方 `/heroes` 页面前 20 名，顺序一致才算过。做不到一致就在工单里回报，不要自己发明排序。
3. `hexdata_shape_invalid kind=heroes` 这条解析失败本身：不再触发（因为不再请求 `/heroes`）。`parseHexdataHeroes` 与 `/heroes` 相关代码若已无调用点，按 R135 的做法列出并删除死代码；若 `augments` 等其他 HTML 页面仍在用同一个 `parseHexdataDocument`，先确认它们是否同样取不到 `buildId`，把结论记进账本（不在本单修）。
4. 头部「样本」显示格式沿用 `compactNumber`（如「35万」）。insights 里没有该英雄时（极端情况）仍显示「—」，不新增说明文字。

## P1 首次加载慢（结合日志的时间线）

以下时间都是相对于 `lol-loot-diagnostics-0924-1453.jsonl` 里「点开海斗」的那一刻（约第 31 秒，`status` 请求之后）。

| 时间点 | 事件 | 耗时/说明 |
|---|---|---|
| +0.0 s | `hexdata_shape answer`（answer-cards） | 约 0.6 s |
| +0.6 s | `hexdata_token_acquired kind=heroes` + `/heroes` 解析失败 | 令牌 592 ms，**这一整步对榜单没有产出**（P2） |
| +1.0 s | OP.GG `/api/contents/tiers` 回退 | 423 ms，榜单到手（约 +1.3 s） |
| +1.9 s | meta | 榜单出来后才开始拉详情 |
| +4.7 s | hero-json 到手 | meta 之后隔了约 **2.8 s**（约 1.2 MB 的 JSON，含 pace 等待；详见诊断缺口） |
| +5.9 s | hextech-insights | 再隔约 1.2 s，**串行** |
| +6.5–6.9 s | CDragon 装备资产 | 再隔约 0.9 s，串行 |
| +7.8 s | postmatch | 再隔约 0.9 s，串行 |
| +7.9 s | 海克斯图标（CDragon）等收尾 | 详情接口在此之后才返回 |

第二个新英雄（首次打开）：meta 命中缓存后，hero-json 仍要约 **2.8 s**（`47.68 → 50.48`）；之后 insights / postmatch 命中缓存，整体约 3 s。已经打开过的英雄第二次是毫秒级（`55.14`、`61.80`，内存/磁盘缓存命中）。

### 归因（日志能证明的部分）

1. **详情接口是一条串行链**：`loadMayhemDetail`（`hexdata.go` 约 3490–3600 行）里 meta → hero-json → insights（官方英雄档位）→ CDragon 资产 → postmatch，逐个等待；只有 OP.GG RSC 那一路在 goroutine 里并行。详情接口要等最后一个才返回，前端 `loadMayhemDetail` 也是单次等待。
2. **两份全局聚合（insights 约 372 KB、postmatch 约 89 KB）是按 buildId 缓存的，但第一次仍然卡在第一个英雄的点击路径上**。它们对所有英雄都一样，本不需要等用户点了英雄才取。
3. **节流对用户触发的第一个请求也生效**：`waitForPace`（`hexdata.go` 约 1380 行）每个请求都加 300 ms 最小间隔外加 0–800 ms 随机抖动（`hexdataMaximumJitter`），并且整个等待期间持有全局互斥锁，并发请求的等待会**排队叠加**。一次详情有 4–5 个 hexdata 请求，仅节流就贡献约 2–4 s。
4. **多余的一步**：每次进入海斗页都会发一次 `/heroes`（令牌 + 解析失败），见 P2，约 0.6–1 s，且结果丢弃。
5. **日志不足以拆分「节流等待 / 网络 / 解析」**：hexdata 主机的请求不产生 `champion_upstream` 事件（该事件只覆盖 op.gg / ddragon / CDragon / your.gg），所以上面 hero-json 的 2.8 s 里多少是节流、多少是下载 1.2 MB、多少是解析和裁剪（日志里 `trimmed_trios=950`），**Claude 分不出来，不猜**。先补诊断（下面第 1 条）。

另有一个**观察到但没有分析**的现象：应用启动后约 2.5–15 s 内，`op.gg` 主站发出约 40 个 1.1 MB 的页面请求 + 约 25 个 `lol-api-champion.op.gg` 请求（共约 45 MB），不是海斗页触发的。本单不处理，请 GPT 在诊断里确认触发来源（哪个功能、是否每次启动都会做、能否延后），把结论写进账本，需要改另开工单。

### 要做（按顺序，前一项效果不够再做后一项）

1. **补诊断（先做，用它判定后面的收益）**：
   - `hexdata_request{kind, path_kind, status, bytes, pace_wait_ms, wire_ms, cache}`：每个 hexdata 请求一条（不记 cookie、不记 URL 查询串）。`pace_wait_ms` 是 `waitForPace` 里实际等待时间，`wire_ms` 是发出请求到读完响应体。
   - `mayhem_detail_phases_ms{meta, hero_json, insights, postmatch, rsc, decorate, total}`：详情接口各阶段耗时。
2. **榜单不再请求 `/heroes`**（P2 的第 1 条）：直接省掉令牌获取与解析失败的一整步，并让榜单和头部数据一次到位。
3. **详情接口内部并行**：meta 拿到后，hero-json、insights、postmatch 三个请求并发发出（`hexdataGlobalGate` 容量本来就是 3），OP.GG RSC 继续并行；CDragon 装备/海克斯资产装饰与 postmatch 并行做。不要提高 `hexdataGlobalGate` 的容量。
4. **节流只约束「连续请求」**：距离上一个 hexdata 请求已经超过一个窗口（建议 2 s，GPT 可按诊断调整）时，新的请求不再叠加随机抖动，也不等最小间隔；连发时仍保留 300 ms 最小间隔和抖动。**等待不要持有全局互斥锁**（改成先计算自己的发出时刻再释放锁，再等待），避免并发请求的等待叠加。请求速率的上限不变，仍遵守 R116 第 6 节的红线和 R147 的许可范围。
5. **两份全局聚合提前取**：进入海斗视图（不是点开某个英雄）时，在后台并发启动 meta、insights、postmatch 三个请求；结果按 buildId 落盘缓存，因此后续每次启动、每个英雄都命中缓存。**只预取这三份全局聚合；per-hero 的 hero-json 仍然只在用户点开该英雄时请求，禁止为 173 个英雄预取**（R116 第 6 节第 1 条不变）。
6. **hero-json 的体积**：确认请求带了压缩（Go 的 `http.Transport` 在不手写 `Accept-Encoding` 时自动 gzip，核实 `fetchOnce` 没有覆盖它）；`trimmed_trios=950` 说明上游返回了大量三元组，如果站点提供更小的等价数据，另议；本单不改数据范围。
7. **最后手段（仅当 1–6 做完仍不满意再评估，不要一开始就做）**：详情接口拆成「核心（hero-json + 官方档位）先返回、表现（postmatch）后台补齐」的两段。代价是「表现」tab 的出现要等第二段（现有设计是取不到就整个 tab 不出现），需要用户确认交互后再做。

### 验收目标（以 GPT 在真机上用新诊断量出来为准，不是承诺值）

- 点开海斗到榜单出现：不再有 `/heroes` 与 OP.GG 回退，数据全来自 hexdata；预期比现在少约 1.5–2 s。
- 第一个英雄详情完整出现：预期从约 8 s（加上榜单约 1.3 s）降到几秒（三个聚合已预取、hero-json 与其他并行）。
- 首次打开一个新英雄：只剩 hero-json 一个 hexdata 请求，且不再叠加抖动等待。
- 已打开过的英雄：保持毫秒级。
- 日志里 hexdata 请求数：冷启动最多 meta + insights + postmatch + answer + 每个被点开的英雄 1 个 hero-json（+ 令牌获取），**不得比现在多**；R147 里钉住的「冷启动 hexdata 请求预算」测试按新的预算更新并说明理由。

### 测试

- Go：榜单以 insights 为源的转换（胜率/场次/档位/顺序；insights 缺英雄时的行为）；详情并行后 hero-json 失败/超时的降级仍走 OP.GG RSC 路径；节流新语义（空闲后第一个请求零等待；连发仍 ≥300 ms；等待期间不持锁，`-race`）；预取只覆盖三份聚合（对抗：让预取遍历英雄 → 测试 FAIL）；诊断事件不含 cookie 与查询串。
- 变异（都要让对应测试 FAIL，并整文件还原）：榜单退回 `/heroes`；聚合改回串行；抖动改回无条件叠加；预取扩大到 per-hero。
- Node：`champions.test.cjs` 里榜单胜率列和头部样本用带 `play`/`winRate` 的行渲染出数值。

## P3 换英雄后没有回到「概览」tab

### 证据与定位

- 截图：当前英雄的 tab 条是「概览 / 构筑 / 表现」，用户描述换英雄后停在上一个英雄选中的 tab。
- 代码：`champions.js` 的 `primeMayhemDetail`（约 572–586 行）在换英雄时重置了 `state.mayhemStage = 0`（评审整改 A1 里写明了同类问题），**没有重置 `state.mayhemDetailTab`**；`state.mayhemDetailTab` 只在 `switchMayhemDetailTab` 里改（约 1207–1211 行，并写入设置 `champion-mayhem-detail-tab`）。`selectMayhemChampion`（约 609 行）和 `prepareMayhemSelection`（约 588 行）都经过 `primeMayhemDetail`。

### 要做

1. 在 `primeMayhemDetail` 里，**仅当英雄确实变了**（`Number(state.selected?.championId)` 与新选中的不同；重复点击同一个英雄、以及详情刷新时不能重置）把 `state.mayhemDetailTab = "overview"`，并同步写回设置 `champion-mayhem-detail-tab`，保证重启后不会又回到旧 tab。位置紧挨 `state.mayhemStage = 0`，同样加一段说明这是同一类「不让状态跨英雄存活」的问题。
2. 「表现」tab 只在 postmatch 有指标时才有；`normalizeMayhemDetailTab` 已有对不存在 tab 的兜底，保持不变。
3. 不改任何界面文字。

### 测试

- `champions.test.cjs`：选中英雄 A → 切到「构筑」→ 切换到英雄 B，断言 tab 为「概览」、面板内容是概览；再点击 B（同一英雄）不重置（先切到「构筑」再点同一英雄，仍是「构筑」）；重启（重新读设置）后为「概览」。变异：去掉重置 → FAIL，还原。

## GPT 的其他工作

真实依赖下 `go build` / `go vet ./...` / `go test -race ./...` 与 R147 之后的基线对比，R135 记录的既有失败不得增加；`node --test backend/web/*.test.cjs desktop/*.test.cjs`。发版（版本号递增，`private` 模式，账本记 key mode、指纹、SHA256），写 `docs/history/ledgers/r148-execution-ledger.md`，更新 `docs/WORKLIST-INDEX.md`。真机验证请用户导出日志发回来，Claude 对照 `hexdata_request` 和 `mayhem_detail_phases_ms` 做前后对比。

## 已知边界

- 时间线里的耗时来自单次会话的单份日志，网络波动会影响绝对值；判断收益要看 `hexdata_request` 拆出的三段，而不是总时长。
- 榜单排序改用 insights 后，如果与官方 `/heroes` 页面顺序不一致（口径差异），以 P2 第 2 条的对照结论为准，不要为了「有数据」牺牲排序正确性。
