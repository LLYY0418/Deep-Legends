# WORKLIST-R192：外部战绩列表在补全详情后卡片摘要不更新、「重试」按钮失效（R191 回归）

诊断人：Claude（复核 R191：读账本 + 只读审 diff + 重跑测试）。执行人：GPT。日期：2026-10-02。
基线：R191 工作区（0.12.55，未提交）。先把 R191 提交，再在其上做本工单；完成后版本升到 0.12.56。

## R191 复核结论

R191 的 P1–P5 都按工单实现了，本次复核没有发现需要返工的地方：

- P1：Riot / SGP / LCU 的 var 都改成指针，任一已选符文三个变量全缺时整组不输出；SGP 诊断传的 `playerRef` 就是 PUUID，主体匹配正确。
- P2：`perk_effect_sample` 只记主体，会话内每个符文最多 3 条，修正表仍为空。
- P4：hexdata 目录失败、校验失败、找不到 ID 都会记 `augment-directory`，然后走 CommunityDragon 回退。
- P5：`layout.json` 里 1280px 下左右为 892.5 / 297.5（3:1）；符文板在左栏里垂直居中（板 y=475，栏 383–848）；碎片图标 22×22；820px 下碎片横排。
- Node：`r190` + `r191` 共 14 项通过。`desktop/overview-render.test.cjs` 在复核环境里跑不完（进程被系统杀掉，和代码无关），以账本里的全量结果为准。Go 这次没能重跑：拷贝源码时被要求用户重新登录桌面端。

**但 P3 的改动引入了一个回归**，见下文。

---

## P1　外部战绩列表：补全详情后摘要不刷新，失败后「重试」点不动

### 影响范围

「外部战绩列表」就是 `deepLegendsMatchCards.mount(...)` 挂出来的列表（`gameplay.js` 约 7630 行起）。目前在用的是英雄页斗魂竞技场的两个列表（`champions.js` 约 1740–1755 行）：

- `pros`（韩服第一名对局）：带 `loadMatchDetails`，点开时才去 `/api/champions/arena/match/KR_{gameId}` 补全整场数据；
- `mine`（我的对局）。

### 原因

R191 为了「只重绘相关对局」，把外部视图 `view.render(id)` 的单场分支改成调用 `replaceMatchEntry(entry, tab, () => {})` 然后直接 `return`（约 7699–7701 行）。`replaceMatchEntry` 的设计是**保留 `.match-summary`**，只替换展开详情部分，并且只给新节点绑定 `bindMatchDetailControls` / `bindPlayerLinks`。R191 之前，这里是整张卡片替换，然后在 7704–7709 行绑定切换、重试、回放、玩家链接。

这样出了两个问题：

1. **补全详情后摘要还是旧的。** `toggleMatch` 里 `loadMatchDetails` 成功后会 `matches[index] = loaded`（`hydratedExternalMatch` 的结果，参与者完整、带 `subjectParticipantId`），然后 `view.render(id)`。但摘要被保留了，所以卡片右侧名单、斗魂的伤害 / 承伤行、平均段位、R190 的当前玩家高亮，都停在补全之前的样子（轻量样本缺的项会一直是占位）。
2. **「重试」按钮没有绑定。** 补全失败时，`renderMatchDetailFailure` 渲染的 `[data-retry-match-detail]` 按钮在 `.match-summary` **外面**（renderMatch 约 3305 行 `${detailFailure}`），会被当作详情部分换成新节点。新节点只经过 `bindMatchDetailControls`，而那里不处理 `data-retry-match-detail`；唯一的绑定在 7704 行，现在被 `return` 跳过了。结果是点「重试」没有任何反应。

R191 改过的测试 `R86 external match expansion preserves unrelated cards` 只测了普通展开（没有 `loadMatchDetails`），所以没发现。

### 修改

1. 外部视图 `render(id, options)` 分两种情况：
   - **只是展开、收起、切换详情页签**（数据没变）：继续用 `replaceMatchEntry`，保留摘要（R191 的目的不变）；
   - **这场的数据或详情状态变了**（`loadMatchDetails` 成功替换了 `matches[index]`，或者进入 / 离开 `failed` / `loading` 状态）：整张卡片替换，然后像 R191 之前一样完整绑定：切换、重试、回放、`bindMatchDetailControls`、`bindPlayerLinks`、`applyRenderedMetricStyles`、`prepareImages`。
   - 实现方式：`toggleMatch` 里涉及 `detailStates` 变化和 `matches[index] = loaded` 的那几次 `view.render(id)` 改成 `view.render(id, { full: true })`。不要用比较 HTML 字符串的办法判断。
2. 即使走 `replaceMatchEntry` 分支，也要给替换进来的节点里的 `[data-retry-match-detail]` 绑定 `view.toggleMatch`（防止以后别的路径渲染出失败态时又漏绑）。绑定只做一次，不能重复绑定导致一次点击触发两次请求。
3. 主总览、浮层里的 `rerenderMatch` / `replaceMatchEntry` 行为保持 R191 的样子，不改。R191 的 P3 测试（9、10）必须继续通过。

### 测试（Node，新建 `backend/web/r192.test.cjs` 或加进 `desktop/overview-render.test.cjs`）

1. 外部列表挂 3 场，`loadMatchDetails` 返回的完整数据里，右侧名单的名字和轻量样本不同、带 `subjectParticipantId`：点开第 1 场后，第 1 张卡片的名单显示补全后的名字，并且主体名字带 `is-current-player`；第 2、3 张卡片的 DOM 节点引用不变。
2. `loadMatchDetails` 第一次 reject：出现「完整详情未加载」和「重试」；点「重试」后 `loadMatchDetails` 被调用第 2 次（计数 = 2），第 2 次成功后详情展开。
3. 连续点两次「重试」（第一次还在 loading）：`loadMatchDetails` 只多调用 1 次（沿用 `loading` 状态的拦截）。
4. 没有 `loadMatchDetails` 的外部列表：展开 / 收起时摘要节点引用不变（即 R191 改过的那条 R86 测试，保留）。
5. 斗魂第一名列表补全后，伤害 / 承伤行显示真实数值，不再是占位。

变异：
- 去掉 `{ full: true }`（补全后也走 `replaceMatchEntry`）→ 测试 1、5 FAIL；
- 去掉重试按钮绑定 → 测试 2 FAIL。

---

## 执行步骤（GPT）

1. 提交 R191；新建 `docs/history/ledgers/r192-execution-ledger.md`。
2. 实现 P1；跑本工单 5 项测试和 2 个变异；再跑 R190 / R191 专项、`desktop/overview-render.test.cjs`，然后 Node 与 Go 全量。
3. Chromium：打开英雄页 → 斗魂竞技场 → 第一名对局列表，用夹具让第一次补全失败、第二次成功，截两张图（失败态、补全后）放进 `docs/history/reports/r192/`。
4. 版本 0.12.56；完整 public 构建（产物带 `-public` 后缀，账本写 key mode）。
5. `docs/WORKLIST-INDEX.md` 加 R192 一行。
6. Windows 待验沿用 R191 的三项（国服符文不是一列 0、致命节奏日志样本、海斗构建页其他卡片不闪），另加一项：英雄页斗魂第一名对局点开后，卡片右侧名单和伤害行正确；断网时点开失败，联网后点「重试」能加载出来。
