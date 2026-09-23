# WORKLIST-R131：R128/R129 执行后 `node --test backend/web/*.test.cjs` 未全绿

诊断人：Claude（只读：跑测试 + 读代码，未改仓库代码）。核对对象：R128（`WORKLIST-R128-MAYHEM-DETAIL-OVERVIEW-MODULES-AND-AUGMENT-HEADER.md`）与 R129（`WORKLIST-R129-LIVE-RECOMMENDATION-FLICKER.md`）的执行结果，均已被用户确认"执行完成"。
执行人：GPT。
日期：2026-09-22。
状态：待执行。
关联：本单只处理"执行后测试没全绿"这一个新问题；R128/R129 本身的代码与 CSS 改动经核对**已经逐条对上各自工单**，不用再动，见下面 §0。

---

## 0. 结论

R128、R129 两份工单描述的源码/CSS 改动本身都已正确落地（tab 网格样式、表现指标重排、统计口径与说明文案全项目删除、AGENTS.md/CLAUDE.md 规则、`renderLive`/`loadLive` 的 P1/P2/P3 修复均核对通过），两份工单各自新增的专项测试（`r128.test.cjs` 10 条、`r129.test.cjs` 11 条，含对抗变异）也全绿。

但 `node --test backend/web/*.test.cjs` 整体跑下来是 **617/657**，不是两份工单各自 §3 验收要求的"全绿"。40 处失败分三类，没有一类是本单要新加功能，全是"改动本身对，但周边测试没跟着更新"：

1. **36 处**：R129 给 `renderLive`/`loadLive`/`bindLiveContent`/`renderLivePlayer` 新增的几个依赖函数，没有同步补进 7 份旧对局页回归测试各自独立维护的编译沙箱，全部报 `ReferenceError`（§1.1、§2.1）。
2. **3 处**：R128 删掉了两处 UI 文案后，`r116b.test.cjs`（2 处）、`r116d.test.cjs`（1 处）里还留着断言这些旧文案存在的行（§1.2、§2.2）。
3. **1 处**：与 R128/R129 都无关的既存测试 bug，顺手记录，本单不处理（§1.3）。

---

## 1. 证据

### 1.1 R129 相关：36 处 `ReferenceError`（`node --test backend/web/*.test.cjs`，2026-09-22）

| 文件 | 失败/总数 | 报错 |
|---|---|---|
| `r91.test.cjs` | 13/13 | `liveRenderTriggerLabel is not defined` |
| `r94.test.cjs` | 9/14 | 同上 |
| `r95.test.cjs` | 7/19 | 同上 |
| `r87.test.cjs` | 2/6 | 同上 |
| `r90.test.cjs` | 2/6 | 1 处 `liveRenderTriggerLabel is not defined`（`loadLive` 相关 compile 调用），1 处 `bindLivePanelScope is not defined`（`r90.test.cjs:78` 单独编译 `bindLiveContent` 那处） |
| `r91-addendum.test.cjs` | 2/15 | 1 处 `liveRenderTriggerLabel is not defined`，1 处 `liveHistorySettled is not defined` |
| `r113.test.cjs` | 1/8 | `liveHistoryStateOf is not defined`（`r113.test.cjs:78` 单独编译 `renderLivePlayer` 那处） |

**根因**：这 7 份文件不共享 `champions.test.cjs`/`r116d.test.cjs` 那套 `compileFunctions` helper，是各自手写沙箱——`vm.runInNewContext` 或 `Function(...)`，把 `renderLive`/`loadLive`/`bindLiveContent`/`renderLivePlayer` 等函数按名字从 `gameplay.js` 源码里抠出来单独跑，其余被调用的东西全部以 `noop`/桩函数形式塞进各自的 `context`/`deps` 对象（例：`r94.test.cjs:24-31` 的 `context = {...bindLiveContent: noop, applyRenderedMetricStyles: noop, prepareImages: noop}`；`r90.test.cjs:78` `compile(["bindLiveContent"], {...})`；`r113.test.cjs:78` `compile('gameplay.js',['renderLivePlayer'],{state:{},...})`）。

R129 P1 把 `renderLivePlayer`/`renderInsightMatches` 里内联的历史状态判断抽成了 `liveHistoryStateOf`/`liveHistorySettled`（`gameplay.js:143-151`）；P3 让 `loadLive` 每轮加载开始时记一次触发来源（`liveRenderTriggerLabel`，`gameplay.js:4389` 附近）、`renderLive`/`bindLiveContent` 渲染与绑定时调用 `recordLiveRenderRebuild`/`bindLivePanelScope`（`gameplay.js:5129-5245`）。这些新函数没有被任何旧文件的 `names`/`deps` 列表收录，沙箱里就是自由变量，一调用就抛 `ReferenceError`。

对照：R129 自己新增的 `r129.test.cjs` 走的是另一套写法——`compile(names, dependencies)` 把需要的依赖当参数显式传入（见该文件 54-58 行），新函数都在 `FUNCTION_NAMES` 列表里，所以不受影响，11 条全绿。

### 1.2 R128 相关：3 处断言残留旧文案

- `r116b.test.cjs:939`：`assert.match(markup, /2 个/, "英雄级汇总的两条都要渲染出来")`。断言的是海克斯推荐标题行右侧、已被 R128 §2.4 删掉的「N 个」徽章（`champions.js:1999` 现在的 header 里已经没有 `section-count` 了）。
- `r116b.test.cjs:1035`：同一个 `/2 个/` 断言，出现在"阶段筛选换英雄回到汇总"那条用例里。
- `r116d.test.cjs:363`：`assert.match(markup, /没有提示不代表阵容没有问题/)`，对应 R128 §2.3-B 表里已删除的 `gameplay.js:5662` 文案（现在 `gameplay.js` 里已经搜不到这句话，之前已核对过）。

### 1.3 与本单无关的既存问题（仅记录，不处理）

`r116b.test.cjs:1140` 断言 `.arena-option-card[data-metric-count="7"] dl\s*\{` 要求 `dl` 后面直接跟 `{`，但 `champions.css` 里这条规则实际写法是 `[data-metric-count="5"] dl,\n...[data-metric-count="8"] dl { ... }` 的合并选择器，`dl` 后面是逗号不是花括号，正则永远匹配不上。这段合并选择器是 R116-B 时期就有的写法，跟 R128/R129 都没有关系，是独立的既存测试 bug。要不要修交给用户决定，本单不处理，也不计入下面的验收范围。

---

## 2. 修复方向

### 2.1 §1.1 的 36 处：给旧测试的编译沙箱补齐新依赖

不改这 7 份旧测试原本要验证的逻辑（状态机、轮询、DOM 复用），只给各自的沙箱补齐新增依赖：

1. **`liveRenderTriggerLabel`**（7 个文件都要加）：纯函数、无外部依赖，可以直接加进各文件的 `names`/extract 列表按真实实现跑（如 `r94.test.cjs:39` 的 `names` 数组末尾加一项），或者桩成 `() => "direct"`——这几份测试都不断言触发来源文案，桩值不影响判据。
2. **`recordLiveRenderRebuild`**（凡真实抽取了 `renderLive` 的文件都要加）：桩成 `() => {}`。
3. **`updateLivePanels`**（同上，凡真实抽取了 `renderLive` 的文件）：桩成 `() => false`——这样 `renderLive` 会退回 R129 之前"标记没变就不动、变了就整块重建"的老路径，正好对上这批旧测试本来就在断言的"DOM 节点复用/整块替换"语义，不用把 R129 P2 的分面板增量替换逻辑一起引入这些旧测试。
4. **`bindLivePanelScope`**（`r90.test.cjs:78` 单独抽取 `bindLiveContent` 的地方）：跟同一个 deps 对象里已有的 `bindRuneWorkspaceControls`/`bindPlayerLinks` 一样桩成 `() => {}`。
5. **`liveHistoryStateOf` / `liveHistorySettled`**（`r113.test.cjs:78`、`r91-addendum.test.cjs` 里直接编译 `renderLivePlayer`/`renderInsightMatches` 的地方）：建议按真实实现加进编译列表，不要桩掉——这两个函数正是 R129 P1 修的那个 flicker bug 的核心判据，桩掉会让这几条旧测试以后测不出同类回归。

### 2.2 §1.2 的 3 处：删掉断言旧文案的行

- `r116b.test.cjs:939`：删掉这一行。同一条用例上面已经用 `assert.match(markup, /海克斯A/)` / `/海克斯B/` 验证了两条都渲染出来，不用换成别的判据。
- `r116b.test.cjs:1035`：删掉。紧邻的上一行已有 `assert.equal(cards().length, 2, "两条 augment 都以英雄级汇总渲染")` 覆盖同一件事。
- `r116d.test.cjs:363`：删掉这一行，连同上面第 362 行"覆盖率说明：没提示不等于阵容没问题"那条注释一起删，避免误导后人。

---

## 3. 验收

- `node --test backend/web/*.test.cjs` 全绿，应为 654/654（657 减去 §2.2 删掉的 3 条断言）。
- §2.1 补完依赖后，原有断言一律不改（`liveHistoryStateOf`/`liveHistorySettled` 除外——那两处按 §2.1-5 用真实实现，不桩），不允许为了让测试通过而放宽判据。
- §1.3 的既存问题不在本单验收范围内。
