# WORKLIST-R123：生涯头像 / 旗帜切换功能退场，迁移为收藏页只读浏览子页

诊断人：Claude（只读设计，未改仓库任何代码文件；本轮新增 `docs/design-mockups/r123-favorites-icon-banner-mockup.png` 一张设计稿截图，真 Chromium 渲染，1240 宽 / devicePixelRatio=2，直接复用仓库真实 `app.css`/`champions.css`/`suite.css`，占位图用色块代替真实 Riot 素材）。
执行人：GPT。
日期：2026-09-22。
基线版本：0.12.15（`desktop/package.json`）。
状态：**设计方案，待执行**。

设计稿：`docs/design-mockups/r123-favorites-icon-banner-mockup.png` —— 收藏页新导航（新增「头像与旗帜」子页）+ 头像视图（筛选/网格/详情）+ 旗帜视图（筛选/网格/详情）一次性截全页。

---

## 0. 三句话结论

1. **写入功能已确认退场**：`docs/r107-execution-ledger.md` §「生涯背景、头像、旗帜」已记录头像 `PUT` 返回 401（真实拒绝），旗帜写入的「未拥有旗帜是否被接受」当时标记「尚未证实」；用户在本轮对话中进一步确认——两条切换功能目前**均无法在客户端生效**，不再等待补充验证，直接按「做不到」处理。
2. **迁移方向**：把生涯页（`suite.js` 里 `data-suite-tab="facade"`）现有的「选择头像」「选择旗帜」两个弹窗，改造成收藏页（`favorites-panel`）下的**第四个子页**「头像与旗帜」，视觉与交互**完全复用皮肤/炫彩收藏已有的组件族**（`view-tabs` / `filters` / `skin-grid` / `skin-card-template` / `list-meta`），只读浏览，不提供任何切换/应用按钮。
3. **后端零新增**：目录数据继续用现成的只读接口 `GET /api/facade/icons`、`GET /api/facade/banners`（`backend/facade_icons.go`、`backend/facade_banners.go`），本轮不新增、不修改这两个接口的读取逻辑；工作量集中在前端迁移 + 写入代码的清理。

---

## 口径决定（不要自行更改）

1. **位置：收藏页新增第 4 个子页，而不是塞进现有「皮肤与炫彩」视图 tab。**
   现有 `皮肤与炫彩` 子页的 `view-tabs`（`index.html:190-193`）已经承载 `已拥有`/`全部皮肤`/`全部炫彩` 三个视图，配套的品质筛选（`rarity-menu`）、臻彩开关是皮肤/炫彩专属维度，头像/旗帜的筛选维度是「系列」「国服专属」，和品质无关。硬塞进同一行 tab 会让这一行从 3 个涨到 5 个，且筛选控件要按 view 互相隐藏/显示（现在 `chroma-unowned-control`/`chroma-prestige-control` 已经在做这种切换，再加两套会让 `app.js` 的 `renderSkinGrid`/`applyFilters` 分支进一步膨胀）。开新子页能让「头像/旗帜」拥有独立的、更简单的状态机，不与皮肤/炫彩收藏的现有回归测试（r99/r101/r105 相关用例）共用渲染路径，改动面更小、回归风险更低。
2. **数据源：完全复用 `GET /api/facade/icons`、`GET /api/facade/banners`，本轮不改后端读取代码。**
   `facadeIcon`（`backend/facade_icons.go:15`）字段：`id, title, year, legacy, disabled, owned, imagePath, sets[], searchTerms[]`；`facadeIconCatalog`（`facade_icons.go:26`）额外带 `ownershipUnavailable, total, ownedCount`。
   `facadeBanner`（`backend/facade_banners.go:32`）字段：`id, idSecondary, name, tencentOnly, owned, imagePath, searchTerms[]`；`handleFacadeBanners`（`facade_banners.go:64`）响应还带 `bannerOwnershipUnavailable`（见 `backend/web/demo-data.js:1003` 的 mock 形状）。
   这两个接口已经在生涯页的选择弹窗里稳定使用（`suite.js` 的 `openFacadeIconPicker`/`openFacadeBannerPicker`），迁移只是换一个消费方，不动生产接口。
3. **写入功能范围：彻底移除，不是隐藏。**
   删除 UI 触发入口与调用链，不要只是把按钮 `hidden`：
   - `suite.js` 里 `openFacadeIconPicker`（约 1355 行起）尾部的 `apply` 按钮点击处理与 `applyFacade({ action: "icon", iconId })` 调用（约 1450 行）；
   - `suite.js` 里 `openFacadeBannerPicker`（约 1462 行起）的点击立即更改逻辑与 `applyFacade({ action: "banner", bannerId })` 调用（约 1494 行）；
   - 两个弹窗函数本身（`openFacadeIconPicker`、`openFacadeBannerPicker`）以及它们专属的 DOM 结构/样式（`.facade-picker`、`.facade-icon-*`、`.facade-banner-*` 等，`suite.css` 里对应规则一并清理，先确认没有其它调用方在用同名类）；
   - 生涯页头像卡/旗帜卡上的「选择头像」「选择旗帜」按钮（`suite.js` 约 1624-1625 行的模板：`<button ... data-facade-icons>选择头像</button>`、`<button ... data-facade-banners>选择旗帜</button>`）；
   - 后端 `backend/profile_facade.go:819`（`a.writeFacadeIconResult(ctx, client, request.IconID)`）与 `:822`（`writeFacadeBanner(ctx, client, request.BannerID)`）这两个调用点，以及它们所在的 `action=="icon"`/`action=="banner"` 分支（连带 `facade_icons.go` 里的 `writeFacadeIcon`/`writeFacadeIconResult`、`facade_banners.go` 里的 `writeFacadeBanner`/`writeFacadeBannerPreferences`/`writeFacadeBannerPreferencesResult` 这几个纯写入函数，如果确认没有其它调用方，一并删除而不是留作死代码——本项目对「没有测试盯着的死代码」是红线，见 9/14 用户原话「靠代码结构的巧合成立，没有一条测试真正盯着它」）。
   - **执行前必须重新 `grep` 一次当前 HEAD 的准确行号**，不要照抄本工单列出的历史行号盲改——本工单撰写时未修改仓库任何文件，行号可能已被后续工单（R124 及以后）改变。
   - 生涯页头像卡/旗帜卡保留「当前头像/当前旗帜」的**只读**展示（数据来自 `summoner.profileIconId` 与生涯草稿，这部分本来就是读，没有失效），把原来的按钮替换成一行文字链接，例如「在收藏页浏览头像与旗帜 →」，点击后切换 `state.section = "favorites"` 并激活新子页（对应 R99 设计稿里「头像/旗帜」两张卡的位置不变，只是卡片底部的交互出口换掉）。
   - **不在本次范围内、保持不动**：段位旗（`preferredBannerType`：`lastSeasonHighestRank`/`blank`，`suite.js:1625` 那个 `suite-segment` 控件）、头衔/挑战勋章清空按钮、挑战配色 `bannerAccent`——这些是独立的数据面（R99 文档里的类别 B/C），用户这次点名的是「切换头像和旗帜」，对应的是生涯头像与生涯旗帜（`REGALIA_BANNER`）选择器，不含这几项。
   - 隐私声明（`features.go` 的 `stores` 列表）如果曾把「头像/旗帜写入」列为数据用途，需要同步去掉对应条目，**文案由用户拍板后才落地**，执行方不得代写（项目红线，`CLAUDE.md`/对话摘要反复强调「声明准确性有红线」）。
4. **未拥有条目默认展示，视觉语言照抄炫彩收藏。**
   默认「显示未拥有」开启（对应现有 `show-unowned-chromas` 默认 `checked`，`index.html:210`），未拥有卡片套用现成的 `.skin-card.is-locked` 样式（`app.css:561-567`：灰阶 + `saturate(.25)` + 圆形锁形图标居中 + 右上角 `.skin-state` 徽标文案改成「未拥有」）。用户关闭该开关后，网格套用现成的 `.skin-grid.hide-unowned .is-locked { display:none }`（`app.css:608`）即可隐藏未拥有卡片，不需要新写 CSS。
5. **卡片与详情弹窗只读，不含任何写操作按钮。**
   复用 `<template id="skin-card-template">`（`index.html:381`）原样结构（`.skin-art` / `.skin-lock` / `.skin-state` / `.skin-copy > strong/.skin-hero/.skin-meta`），字段映射：
   - 头像卡：`strong` = `title`；`.skin-hero` = `sets.join(" · ")`（无系列则显示「未分类」）+ `year`；`.skin-meta` = `ID ${id}`；`.skin-state` = `已拥有`/`未拥有`。
   - 旗帜卡：`strong` = `name`；`.skin-hero` = `tencentOnly ? "国服专属" : "通用"`；`.skin-meta` = `ID ${id}`（`idSecondary` 非空时追加）；`.skin-state` = `已拥有`/`未拥有`。
   点击卡片弹出详情，复用现有 `skin-dialog` 结构（大图 + 名称 + 关键信息列表），**去掉「设为头像」/「立即更改」相关按钮与其事件绑定**，仅保留关闭。

---

## P0　执行前回归确认（只读，快速）

不需要重新跑 R99 的 13 条探测矩阵——那一轮已经把只读目录接口的字段形状钉死并有测试覆盖（`backend/r99_test.go`、`r101_test.go` 等）。执行前只需要：

1. `go test ./backend -run 'Facade|R99|R101|R105'`，确认 `handleFacadeIcons`/`handleFacadeBanners`/`loadFacadeBanners`/`loadIconCatalog` 相关测试仍然全绿（本工单不改这些函数，理论上不受影响，跑一遍是防止基线漂移）。
2. `grep -n "writeFacadeIcon\|writeFacadeBanner\|applyFacade" backend/*.go backend/web/*.js` 摸一遍当前所有调用点，核对本工单列出的行号是否需要更新，避免执行时按旧行号误删无关代码。

---

## P1　前端：收藏页新增「头像与旗帜」子页

### P1-1　导航（`backend/web/index.html`）

在 `favorites-nav`（约 121-124 行）追加第 4 个 `favorites-nav-item`，`data-favorites-page="facade-collection"`，对应新增面板 `favorites-facade-panel`（挂在 `favorites-account-panel`/`favorites-pools-panel` 同级）。图标、文案参考设计稿：标题「头像与旗帜」，副标题「头像旗帜与拥有状态」——直接点名页面内容（头像、旗帜）并带出这一页区别于其它三项的核心信息（拥有状态），不出现「只读」「展示」这类实现细节词；与另外三项（「收藏浏览与筛选」「战利品与奖励」「目录、清单与历史」）同为「名词+与+名词」的措辞风格，不重复使用「目录」这个词——「三合一奖池」的副标题已经用过，避免两个 tab 都在讲「目录」造成混淆。

### P1-2　面板骨架

新面板内部结构完全比照 `favorites-collection-panel`（`index.html:187-218`）：
- 内层 `view-tabs`：`头像` / `旗帜` 两个 tab（`data-view="icons"` / `data-view="banners"`），默认打开 `icons`。
- 筛选行（`.filters`）按视图切换显隐，比照现有 `chroma-unowned-control` 用 `hidden` 属性切换的做法：
  - 头像视图：搜索框（名称/拼音/首字母，复用 `suite.js` 里已有的 `window.deepLegendsChampionSearch.scoreOption` 模糊搜索）、系列下拉（`sets` 去重排序）、快捷分类（全部头像/近三年新增，比照原弹窗的 `all`/`recent` 分组）、显示未拥有开关、排序（最新在前/最早在前/按名称/未拥有优先）、排序方向按钮。
  - 旗帜视图：搜索框（名称/编号）、快捷分类（全部/国服专属，比照原弹窗 `all`/`tencent` 两组）、显示未拥有开关、排序（按编号/按名称/未拥有优先）、排序方向按钮。
  - **已拥有/未拥有不做成快捷分类按钮**：这两个状态已经由「显示未拥有」开关覆盖（开=全部都看，关=只看已拥有），快捷分类里再放一组「已拥有/未拥有」是同一维度的重复表达，会让用户不清楚该用哪一个、两者冲突时以谁为准。快捷分类只保留和显示未拥有开关**正交**的维度（头像的「近三年新增」、旗帜的「国服专属」）。
- `list-meta` 行：文案模板「共 N 款{头像/旗帜} · 已拥有 M · 已显示 K」，与现有皮肤页 `list-meta` 文案风格一致（`app.js` 里 `heroCount`/`formatNumber` 一类的拼接方式）。
- 可选：一条小型统计条（设计稿里的 `metrics-mini`：目录总数/已拥有/当前显示），复用 `app.css` 的 `.metrics` 规则（`app.css:427-433`），非必需，P2 视觉打磨时再定。
- 网格：`<div class="skin-grid" ...>`，复用 `skin-card-template`。

### P1-3　状态与渲染逻辑

建议**不要**塞进现有 `app.js` 里皮肤/炫彩那一整套 `state.items`/`applyFilters`/`renderSkinGrid`（`app.js:605-1200` 一带），那一套已经承载皮肤分组、炫彩堆叠、臻彩独立原画等复杂度。新开一个轻量模块（例如 `backend/web/favorites-facade.js`，随 `index.html` 一起 `<script>` 引入，与 `suite.js`/`champions.js` 同级），只做：拉取 `/api/facade/icons` 或 `/api/facade/banners` → 按 视图/搜索/分类/系列/排序 过滤排序 → clone `skin-card-template` 渲染 → 点击弹详情。逻辑量级参考 `suite.js` 里 `openFacadeIconPicker` 的 `render()`/`chunk()` 分帧渲染（5099 条数据不能一次性同步渲染，照抄它按 `requestAnimationFrame` 分片插入 DOM 的写法，`suite.js` 约 1418-1436 行）。

未连接客户端 / 拥有状态不可读取（`iconOwnershipUnavailable`/`bannerOwnershipUnavailable`）时的占位文案，比照现有皮肤页两种降级提示：
- 未连接：复用 `app.js:1066` 那条「等待英雄联盟客户端」空状态；
- 已连接但拥有状态未知：提示语气比照弹窗原文「拥有状态未读取；点击后核对客户端结果」，收藏页语境改成静态说明，例如「本次未能读取拥有状态，以下按目录顺序展示，暂不区分已拥有/未拥有」，并隐藏「显示未拥有」开关（此时开关无意义）。

### P1-4　详情弹窗

复用 `skin-dialog` 相关 DOM（`el.skinDialog` 等，`app.js:122-123` 一带列出的 id），或者新开一个更轻的只读弹窗（考虑到 `skin-dialog` 目前耦合了炫彩原画切换、视频播放等皮肤专属逻辑，新开一个 `facade-detail-dialog` 可能比裁剪 `skin-dialog` 更省事，两种做法都可以，执行时按改动量小的来，不强制）。展示内容：大图、名称、系列/国服专属、ID、拥有状态；**不含任何按钮**（对比原弹窗底部「设为头像」「点击旗帜立即更改」，这里全部去掉）。

### P1-5　生涯页清理（`backend/web/suite.js` + `suite.css`）

- 移除 `openFacadeIconPicker`、`openFacadeBannerPicker` 两个函数体，及触发它们的 `[data-facade-icons]`/`[data-facade-banners]` 按钮事件绑定。
- 生涯页「头像」卡（`suite.js` 约 1624 行模板里的 `.facade-icon-card`）与「旗帜」卡（约 1625 行 `.facade-banner-card`）保留当前值只读展示（当前头像缩略图、当前生涯旗帜名称——若现有代码能读到具体名称；读不到就保持现状的占位文案，不新增读取逻辑），把「选择头像」「选择旗帜」按钮替换为文字链接，点击后 `state.section = "favorites"` + 激活 P1-1 新增的子页 tab（`data-favorites-page="facade-collection"`）+ 默认打开对应 `data-view`（从头像卡进去开 `icons`，从旗帜卡进去开 `banners`）。
- `suite.css` 里专属于两个弹窗的规则（`.facade-picker`、`.facade-icon-*`、`.facade-banner-picker` 等）确认无其它引用后删除；`.facade-icon-card`/`.facade-banner-card` 本身的卡片外观样式保留（只读展示还要用）。

---

## P2　后端：清理写入代码

- `backend/profile_facade.go`：删除 `action=="icon"` 分支里对 `a.writeFacadeIconResult(ctx, client, request.IconID)` 的调用（当前约 819 行）与 `action=="banner"` 分支里对 `writeFacadeBanner(ctx, client, request.BannerID)` 的调用（约 822 行）；如果 `applyFacade`/`facadeApplyResult` 相关的请求结构体、路由分支只为这两个 action 存在，一并清掉，避免留下「永远返回 400/未知 action」的死路由。
- `backend/facade_icons.go`：`writeFacadeIcon`（165 行）、`writeFacadeIconResult`（170 行）——确认没有其它调用方后删除；`loadIconCatalog`/`fetchIconCatalog`/`handleFacadeIcons`（只读部分）保留，收藏新子页继续用。
- `backend/facade_banners.go`：`writeFacadeBanner`（87 行）、`facadeBannerPreferences`/`writeFacadeBannerPreferences`/`writeFacadeBannerPreferencesResult`（112-196 行）——同样确认无其它调用方后删除；`loadFacadeBanners`/`handleFacadeBanners` 保留。
  **注意**：`facadeBannerPreferences`/`writeFacadeBannerPreferences` 函数名里的「preferences」容易和段位旗 `preferredBannerType` 的读写混淆——删除前务必确认这组函数只服务于本次要退场的「生涯旗帜」写入，不要连段位旗的写入路径一起删掉（段位旗按口径决定第 3 条不在本次范围）。
- 对应的 Go 测试（`r101_test.go`、`r105_test.go`、`r107_test.go`、`r108_test.go` 里断言 `writeFacadeBanner`/`writeFacadeIconResult` 行为的用例）需要同步删除或改写成「确认该写入路径已不存在」的负向测试，不能留着测一个已删除的函数（编译都过不了）。

---

## 验收判据

1. **旧写入路径确实清零**：`grep -rn "writeFacadeIcon\b\|writeFacadeIconResult\|writeFacadeBanner\b\|writeFacadeBannerPreferences" backend/*.go` 应无匹配（`loadFacadeBanners`/`loadIconCatalog`/`handleFacade*` 等只读函数除外）；`grep -rn "openFacadeIconPicker\|openFacadeBannerPicker\|设为头像\|旗帜已应用\|立即更改" backend/web/*.js` 应无匹配。
2. **新子页数据正确**：新子页头像/旗帜数量、已拥有数量与 `GET /api/facade/icons`/`GET /api/facade/banners` 原始响应的 `total`/`ownedCount`（或按 `owned` 字段现算）逐一致；对抗变异——故意把 `owned` 判定条件取反，断言前端展示的「已拥有 N」随之改变（证明数字不是写死的）。
3. **未拥有展示默认值正确**：新子页首次打开时「显示未拥有」为开启状态，未拥有卡片可见且套用 `.is-locked` 灰阶+锁形样式；关闭开关后未拥有卡片从 DOM/视图中隐藏；变异——把默认值改成 `false`，断言相关端到端测试 FAIL。
4. **未连接客户端时占位正确**：断连状态下打开新子页展示与皮肤收藏页一致的「等待英雄联盟客户端」空状态，不发起 `/api/facade/icons`/`/api/facade/banners` 请求（比照现有皮肤页断连时不拉取 `/api/skins` 的做法）。
5. **生涯页无死链**：生涯页头像卡/旗帜卡的新文字链接能正确跳转到收藏页新子页并定位到对应 `头像`/`旗帜` 视图；生涯页不再出现「选择头像」「选择旗帜」按钮或其弹窗 DOM。
6. **构建与既有测试不回归**：`go build ./backend`、`go vet ./backend`、`go test ./backend` 全绿；皮肤/炫彩收藏（`view-owned`/`view-all`/`view-chromas`）现有行为逐项人工核对未受影响（本工单口径决定第 1 条要求新子页独立状态机，理论上零交叉，仍建议执行后手动过一遍）。
7. **隐私声明同步**：若 `features.go` 的 `stores` 曾声明头像/旗帜写入用途，本轮改动后声明与实际代码一致；文案改动需要用户确认后再落地，不能执行方自行代写。

---

## 已知边界 / 不确定项

- 段位旗（`lastSeasonHighestRank`/`blank`）、头衔与挑战勋章清空、挑战配色 `bannerAccent` 三项不在本次范围，维持现状；如果后续验证这几项也存在类似的「无法生效」问题，另开工单处理，不要顺手在本工单里一并改掉。
- 新子页是否需要在「收藏」整体统计（现有 `owned-count`/`chroma-count` 那一排顶部数字）里追加头像/旗帜的已拥有计数，属于可选的视觉打磨（P2 之后再议），本工单不作为验收项。
- 设计稿使用色块占位，不含真实 Riot 素材；真机截图（浅色/深色主题、常见窗口宽度）留给执行方按项目惯例在验收阶段补（比照 R119/R120 的截图证据流程）。
