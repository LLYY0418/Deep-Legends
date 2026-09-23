# R123 执行记录：生涯头像 / 旗帜切换退场，迁移为收藏页只读浏览子页

> 证据已于 R135 移出工作区，见提交 `c2b678d07938cc39b5c15ccc49e5a09c931c17e3`。

日期：2026-09-22（执行轮）。
工单：`docs/history/worklists/WORKLIST-R123-FACADE-ICON-BANNER-COLLECTION-MIGRATION.md`。
设计稿：`docs/design-mockups/r123-favorites-icon-banner-mockup.png`。
基线版本：0.12.15（`desktop/package.json`）。

## 0. 结论

- P0 回归：执行前 `go test ./backend -run 'Facade|R99|R101|R105'` 全绿；`grep` 核对写入调用点，确认工单行号基本有效，另发现 `r101_probe.go`（W5/W6 真机写入探针）是 `facadeBannerPreferences`/`writeFacadeBannerPreferencesResult` 的唯一其它调用方（见 §2 决定）。
- P1 前端：收藏页新增第 4 个子页「头像与旗帜」（`data-favorites-page="facade-collection"`），独立模块 `backend/web/favorites-facade.js`，复用 `view-tabs / filters / skin-grid / skin-card-template / list-meta / metrics` 组件族；详情弹窗 `facade-detail-dialog` 只读，无任何写入按钮；生涯页两个选择弹窗与按钮全部删除，改为文字链接「在收藏页浏览头像与旗帜 →」。
- P2 后端：`writeFacadeIcon*`、`writeFacadeChatIcon`、`writeFacadeBanner*`、`facadeBannerPreferences*`、`applyFacadeActionResultDetails` 的 `icon`/`banner` 分支、`facadeApplyRequest.IconID/BannerID`、`facadeApplyResult.IconScope`、`facadeState.IconApplyScope` 全部删除；W5/W6 写入探针与 `runR99WriteProbe` 一并退场；`GET /api/facade/icons`、`GET /api/facade/banners` 读取逻辑未动，仅从 banners 响应移除已失效的 `writeSupported`/`writeStatus` 字段（demo mock 同步）。
- 测试：Go 全量 `go test .` 与前端 `node --test *.test.cjs`（623 项）全绿；新增 `backend/web/r123.test.cjs` 与 Go 侧负向测试（`TestR123FacadeIconAndBannerActionsRemoved`、`TestR99IconAndBannerApplyActionsRetired`）。

## 1. 文件改动清单

后端：
- `backend/facade_icons.go`：删除 `knownFacadeIcon`、`writeFacadeIcon`、`writeFacadeIconResult`、`writeFacadeChatIcon`；`facadeIconInventoryEntry` 从探针文件迁入。
- `backend/facade_banners.go`：删除 `writeFacadeBanner`、`facadeBannerPreferences`、`writeFacadeBannerPreferences`、`writeFacadeBannerPreferencesResult` 与死常量 `facadeChallengePreferencesPath`；`handleFacadeBanners` 响应去掉 `writeSupported`/`writeStatus`；保留 `facadeBannerIdentity`/`facadeBannerItemID`（生涯状态读取仍用）。
- `backend/r101_probe.go`：整文件删除（W5/W6 写入探针）。
- `backend/r99_probe.go`：删除 `runR99WriteProbe`、`r99ProbeErrorCode`；`handleFacadeProbe` 维持只读。
- `backend/profile_facade.go`：删除 `icon`/`banner` 动作分支、请求/响应/状态里的 icon/banner 字段与诊断分支；`facadeDiagnosticAction` 不再识别 `icon`/`banner`（落入 `unknown`）。
- 测试：`r101_test.go`、`r105_test.go`、`r107_test.go`、`r108_test.go`、`r99_test.go` 删除写入/探针用例，补负向用例。

前端：
- `backend/web/index.html`：新增导航项、`favorites-facade-panel` 面板（头像/旗帜 view-tabs、筛选行、metrics-mini、list-meta、skin-grid）、`facade-detail-dialog`、`favorites-facade.js` 脚本引用。
- `backend/web/favorites-facade.js`（新）：拉取 `/api/facade/icons|banners` → 过滤/排序（rAF 分帧渲染）→ `skin-card-template` 卡片 → 只读详情；断连时展示「等待英雄联盟客户端」且不发请求；拥有状态不可读时隐藏「显示未拥有」开关并降级徽标文案。
- `backend/web/app.js`：`el.viewTabs` 作用域收敛到皮肤面板（避免与新子页 `data-view` 冲突）；`activateFavoritesPage` 支持 `facade-collection`；新增 `window.deepLegendsOpenFacadeCollection(view)` 供生涯页跳转。
- `backend/web/suite.js`：删除 `openFacadeIconPicker`/`openFacadeBannerPicker`/`showFacadePickerError` 及全部弹窗状态引用；头像卡/旗帜卡按钮替换为 `data-facade-browse` 文字链接；`applyFacade` 回读逻辑去掉 icon 分支。
- `backend/web/suite.css`：删除 `.facade-picker*`、`.facade-banner-picker/.facade-banner-grid/.facade-banner-option*` 规则；卡片只读样式保留。
- `backend/web/app.css`：新增子页样式（导航「新」徽标、快捷分类、metrics、旗帜网格尺寸、详情弹窗）。
- `backend/web/demo-data.js`：banners mock 去掉 `writeSupported`/`writeStatus`。
- 测试：`r101.test.cjs`、`r105.test.cjs`、`suite.test.cjs` 删除弹窗用例；新增 `r123.test.cjs`。

## 2. 口径决定（执行期补充，均记录备查）

1. **W5/W6 真机写入探针退场**：工单 P2 要求删除 `facadeBannerPreferences` 组函数，而 `r101_probe.go` 是其唯一其它调用方；探针本身即「向真实客户端写入头像/旗帜再恢复」的验证工具，与「写入功能已确认退场」直接冲突，且验收 grep 要求 `writeFacadeBannerPreferences` 在 `backend/*.go` 零匹配。故连同 `runR99WriteProbe` 与对应测试一并删除，`handleFacadeProbe` 保持只读检测不变。
2. **`writeSupported`/`writeStatus` 移除**：写入路径删除后该字段恒为谎言，且无生产消费方、无测试钉住，故从响应与 demo mock 移除；目录读取逻辑（catalog、拥有状态判定）未改。
3. **`data-view` 作用域**：新子页 tab 沿用工单的 `data-view="icons|banners"`，但把 app.js 的 `el.viewTabs` 选择器收敛到 `#favorites-collection-panel`，保证皮肤/炫彩既有状态机零交叉。
4. **未拥有展示**：关闭「显示未拥有」时在 JS 侧过滤（未拥有卡片不进 DOM），同时保留 `.is-locked` 灰阶+锁形样式用于开启状态；拥有状态不可读时不套用锁形、徽标显示「拥有状态未知」、metrics「已拥有」显示「—」。
5. **隐私声明**：`features.go` 的 `explicitWrites` 仍含「设置生涯头像…」与探针描述句，按工单红线**未擅自改文案**，待用户拍板后落地（见 §4）。

## 3. 验收判据核对

1. 旧写入路径清零：`grep -rn "writeFacadeIcon\b\|writeFacadeIconResult\|writeFacadeBanner\b\|writeFacadeBannerPreferences" backend/*.go` 无匹配；`grep -rn "openFacadeIconPicker\|openFacadeBannerPicker\|设为头像\|旗帜已应用\|立即更改" backend/web/*.js` 无匹配。✔
2. 数据正确：`r123.test.cjs` 断言已拥有数由目录 `owned` 现算（含 owned 取反变异断言）；metrics/list-meta 文案函数单测钉住「共 N 款 · 已拥有 M · 已显示 K」。✔
3. 未拥有默认展示：模块默认 `showUnowned: true`（源码断言）；关闭后未拥有行被过滤；开启时 `.is-locked` 生效。✔
4. 断连占位：`activate()/render()` 在 `connected=false` 时只渲染「等待英雄联盟客户端」空状态，`load()` 直接返回不发请求；status 事件断连时清空目录缓存。✔
5. 生涯页无死链：`data-facade-browse` 链接经 `window.deepLegendsOpenFacadeCollection` 切到收藏页并 `setView(icons|banners)`；旧按钮与弹窗 DOM 已无。✔
6. 构建与测试：`go build .`、`go vet .`、`go test .` 全绿；前端 `node --test *.test.cjs` 623 项全绿（含 R117 样式审计）。皮肤/炫彩收藏渲染路径未改（仅 viewTabs 选择器收敛，行为等价）。✔
7. 隐私声明：待用户确认文案（§4）。⏳

## 4. 待办

- 隐私声明 `explicitWrites` 文案（用户拍板后落地）：需删除「设置生涯头像：只写 profileIconId 一个字段，不改其它资料」条目，并修订段位旗条目中的探针描述句。
- 真机/截图证据：按 R119/R120 惯例的浅色/深色主题与常见窗口宽度截图留给验收阶段补。

## 5. 验收证据（2026-09-22 执行轮补充）

- 浏览器实测（demo 模式，127.0.0.1:8731）：头像视图渲染 5,099 条目录、`共 5,099 款头像 · 已拥有 1,700 · 已显示 5,099`（已拥有数由目录 `owned` 现算）；快捷分类「全部头像 / 近三年新增」、排序「最新在前 / 最早在前 / 按名称 / 未拥有优先」、「显示未拥有」默认开启，均与设计稿一致；未拥有卡片带 `.is-locked` 锁形徽标。
- 详情弹窗实测：仅 1 个按钮（关闭），字段为 名称 / 系列 / 拥有状态 / ID + 只读说明文案，无任何写入按钮。
- 截图证据（headless Chrome + CDP 落盘）：`docs/r123-validation/facade-icons-dark-1280.png`、`facade-banners-dark-1280.png`、`facade-icons-light-1280.png`、`facade-detail-light-1280.png`。
- 静态资源：`favorites-facade.js` 已登记进 `static_assets_test.go` 的嵌入清单，`go:embed web/*.js` 自动覆盖。

## 6. 未决项处理口径

- 隐私声明文案与版本号/CHANGELOG 两项在执行轮以问题卡征询用户，未获选择；按项目红线保守处理：隐私声明一字未改（待用户拍板），版本保持 0.12.15、CHANGELOG 不动（用户自行打包，近期轮次惯例）。

## 7. 用户反馈轮（2026-09-22 第二轮）

- 导航「新」徽标移除（含 `.favorites-nav-flag` 样式）；副标题改为「目录与拥有状态」。换行问题根因：徽标 `<em>` 使文本 span 不再是 `span:last-child`，丢失 `display: grid` 的标题/副标题分行样式；移除后 1280px 实测 `stacked: true`，与其余三项一致。
- 隐私声明按用户确认文案落地：删除「设置生涯头像…」整条；段位旗条目探针句改为「……生涯旗帜目前只读浏览，目录与拥有状态在收藏页查看；点击“检测客户端支持”只读查询头像与旗帜相关接口和拥有状态，不切换、不修改头像、旗帜或背景，结果只写入诊断日志，不记录库存或身份标识」；`quality_test.go` 计数 11→10 同步。
- 截图证据已用去除徽标后的界面重拍（同 §5 四个文件）。

## 8. 用户反馈轮（2026-09-22 第三轮）

- 断连态与收藏页其它子页对齐：`renderDisconnected()` 现在隐藏筛选工具栏、metrics 统计条与 list-meta 行，网格只保留「等待英雄联盟客户端」空状态；重连后自动恢复。浏览器实测（status 事件模拟断连/重连）：`toolbar/metrics/listRow hidden=true`、meta 空、仅空状态卡；重连后目录与统计恢复。
- 「等待客户端数据」文案彻底移除：HTML 初始 list-meta 为空，JS 不再写入该文案。
- 导航副标题改为「头像旗帜与拥有状态」。
- `r123.test.cjs` 增补断言：副标题文案、list-meta 初始为空、断连隐藏逻辑存在；截图证据重拍。

## 9. R124 执行（2026-09-22）：拥有状态不可用 + 显示未拥有关闭的滤空缺口

- `facadeCollectionIconRows`/`facadeCollectionBannerRows` 新增 `ownershipUnavailable` 参数，过滤条件改为 `!ownershipUnavailable && !filters.showUnowned && !item.owned`；`currentRows()` 传入 `ownershipUnavailable()`。
- `syncControls()` 双重保险：`unavailable` 为真时强制 `filters.showUnowned = true`（控件隐藏但记忆值复位），为拥有状态恢复后的下一次渲染准备正确状态。
- 验收核对：工单证据数据（owned 全 false + `showUnowned:false`）改后返回全量 3/2；变异证明——内存中去掉 `!ownershipUnavailable &&` 后同数据返回 0，新测试 `R124 ownership-unavailable catalog survives a previously closed unowned toggle` 会 FAIL；浏览器端到端（demo + fetch 拦截 `iconOwnershipUnavailable:true` + 重连事件）：关过开关后 `checked` 自动复位 true、控件隐藏、网格 30/30 不被滤空、meta 显示「拥有状态未读取」。
- 测试：`r123.test.cjs` 7/7、前端全量 624 项全绿。
- P2 两项按工单「不要求本轮处理」保持不动：版本号仍 0.12.15（待用户打包前决定）；生涯页跳旗帜视图多一次 icons 请求（`load()` 有视图守卫，不影响正确性）。

## 10. R125 执行（2026-09-22）：头像视图格子改原生尺寸

- 新增 `.facade-grid.is-icons` 样式块（与 `is-banners` 互斥，`syncControls()` 里切换）：固定列宽 `repeat(auto-fill, 92px)`、正方形 `aspect-ratio: 1/1`、`object-fit: contain` 不裁剪、文字移到图下只留名称一行（完整名称进 `title`）、锁与角标等比缩小；`.is-locked` 灰度逻辑不变。
- 尺寸微调说明：工单建议 104px，但实测 1240px 窗口下网格内容宽仅 920px，104px 只能排 8 列，不满足「一行不少于 9 个」的验收；按工单「具体数值可以微调」授权改为 92px（9 列=9×92+8×10=908≤920），1900px 下 15 列且格子尺寸不变。
- R117 预算兼容：新规则的 padding 复用既有值（`3px 7px`、`0`），gap 用 `12px 10px` 替换原 `14px 12px`（distinct 计数不变），padding/border-radius/gap 三个棘轮均不超基线。
- 验收（headless Chrome + 静态资源服务 + `/api/image` SVG 桩，因 Go 编译被并行工作流破坏，见下）：icons@1240 = 9 列 / 92×92 / contain / 锁 30px / 「未拥有」角标 / title 完整名称 / 5,099 卡；icons@1900 = 15 列、格子仍 92×92；banners@1240 = 174×174 正方形（本轮未改旗帜 CSS）；skins@1240 = 298×168（16:9，未改）。截图：`docs/r123-validation/facade-icons-1240.png`、`facade-icons-1900.png`。
- 测试：`r123.test.cjs` 新增 R125 断言（92px 固定列宽、正方形、contain、is-icons 切换、title 赋值），前端全量 625 项全绿。
- **环境警示（非本轮引入）**：工作区存在另一并行工作流的未提交产物（untracked `backend/position_contract_probe.go`、`augment_contract_probe.go` 等），其中 `position_contract_probe.go:583` 引用了磁盘上不存在的 `positionProbeTextPaths`，导致当前 `go build ./backend` 失败。R125 未改任何 Go 文件；本轮验收改用静态服务完成。该编译破坏需由对应工作流补齐函数后自行恢复，执行方未代改。
