# WORKLIST 0823-R11B（R11 收尾：1 项位置修正 + 6 个护栏缺口 + 3 项未完成）

诊断人：Claude（只读验收，未改仓库文件）。执行人：GPT。
前置：`design/WORKLIST-0823-R11.md` 的 13 项**功能上基本都已落地**，本单只列**还没做完的部分**。

验收基线：Go `go test ./...` 绿；JS `champions.test.cjs` 85 项绿；变异测试 16 杀 6 活。
所有行号是本次验收时的实测值，改前先 `grep` 确认锚点。

---

## 一、必改：详情页四五六件的位置（用户本轮明确要求）

### W1 把 `depthGroups` 挪进核心装那一列，与出装路线一起竖向排列

用户原话：**"详情页的四五六件应该和放在之前的核心装备那块一起竖向排列展示"**。

现在两个渲染函数都把它放在了**整块的最底部（全宽条）**，不在核心装那一列里：

**排位详情页** `web/champions.js:1433`
```js
return `<section class="recommendation-section champion-build-board build-workspace"><header><h3>出装</h3></header><div class="build-split"><div class="build-side">${group("出门装", starters)}${group("鞋子", boots)}</div><div class="build-routes">${routeRows || '<p class="muted">暂无出装路线样本</p>'}</div></div>${depthGroups}</section>`;
//                                                                                                                                                                                                                                    ^^^^^^^^^^^^^ 在 build-split 外面
```
→ 改成放进 `.build-routes` 内部，跟在 `routeRows` 后面：
```js
<div class="build-routes">${routeRows || '<p class="muted">暂无出装路线样本</p>'}${depthGroups}</div>
```

**非排位详情页** `web/champions.js:1418`
```js
...<div class="item-option-groups">${itemSections.join("")}</div>${depthGroups}</section>
```
→ `depthGroups` 应并入 `.route-options` 那个 section（`champions.js:1417` 拼的那条），让它跟"出装路线"同列同栏：
```js
itemSections.push(`<section class="route-options"><h3>出装路线</h3><div class="config-option-list">${routes.map(...).join("")}</div>${depthGroups}</section>`);
```
并把 `:1418` 结尾的 `${depthGroups}` 删掉（否则会渲染两遍）。

**CSS 配套**（`web/champions.css`）：
- `.build-routes`（`:476`）现在是 `grid-auto-rows: minmax(84px,1fr)`，`depthGroups` 进去后会被当成等高行拉伸。给它加 `grid-row: auto` / 或把 `.champion-item-depth-columns` 设成 `grid-auto-rows: auto`，避免被强行撑到 84px 一行。
- `.champion-item-depth-columns`（`:506`）的 `border-top: 1px solid var(--line)` 保留，作为与出装路线的分隔线。
- 保持单列竖排（`grid-auto-rows: minmax(0,1fr)` 改成 `auto` 后仍是单列），**不要**改成横向。

**验收**：排位与非排位两个详情页，第四/第五/第六件都出现在核心装（出装路线）那一列的下方、竖向堆叠；页面底部不再有全宽的 depth 条。

---

## 二、护栏缺口：6 个变异体存活（改完必须能被杀掉）

我用真副本做了变异测试，下面 6 处**改坏了测试也不报**。每条都给出了我用的变异体，写完测试请用同样的变异体自验一次。

### W2【最高危】四五六件的后端接线没有护栏

**变异体**：`champions_structured.go:465`
```go
response.Build.FourthItems, response.Build.FifthItems, response.Build.SixthItems = depths[4], depths[5], depths[6]
// 改成 →  _ = depths
```
→ 整个四五六件功能作废，`go test ./...` 依然全绿。

`parseOPGGItemDepths` 本身有真护栏（`TestParseOPGGItemDepths`，我实测能杀 3 个变异体），但"解析完有没有真的落进响应"没人管。

**要补**：一个覆盖 `loadDetail` → `response.Build.FourthItems/FifthItems/SixthItems` 的测试。用假 transport 喂一份含 `depth_4_item_0` / `depth_5_item_0` / `depth_6_item_0` 的页面，断言三个字段各自非空且 ID 对得上。

### W3 非排位详情页的 depth 分组没有护栏

**变异体**：`web/champions.js:1411`（`renderBuildBoard` 里的那一处，**不是 1427**）
```js
const depthGroups = renderBuildDepthGroups(build);
// 改成 →  const depthGroups = "";
```
→ 非排位模式（斗魂/海斗/大乱斗）静默丢掉分组，85 项测试全绿。

现有的 `champion detail keeps fourth/fifth/sixth items out of core routes`（`champions.test.cjs:1666`）只断言了 `renderRankedBuild`（`:1685` 那行 `functionSource(script, "renderRankedBuild")`）。

**要补**：同样断言 `renderBuildBoard` 的函数体里含 `renderBuildDepthGroups(build)`，且 W1 改完后断言它在 `route-options` section 内。

### W4 详情页 depth 的竖向布局没有 CSS 断言

**变异体**：`web/champions.css:506`
```css
.champion-item-depth-columns { display: grid; min-width: 0; grid-auto-rows: minmax(0,1fr); ... }
/* 改成 →  grid-auto-flow: column; align-content: start; */
```
→ 变成横向并排，测试全绿。

对局内那一版（`.item-depth-columns`）已经有断言了（`live build recommendations keep core items left and depth groups vertical on the right`），详情页这版漏了。

### W5 "胜率前二"没有护栏

**变异体**：`web/gameplay.js:2332` 附近
```js
const spellOptions = byWinRate(build.spellOptions || ..., 2);
// 改成 →  const spellOptions = ((o)=>o)(build.spellOptions || ..., 2);
```
→ 退回上游的选取率序，测试全绿。

**要补**：编译出 `renderBuildRecommendation`，喂一组胜率与选取率**顺序相反**的 option，断言渲染出来的前两条是胜率最高的两条（三个分区都要覆盖：召唤师技能 / 出门装 / 鞋子）。

### W6 重进页面的 20s 节流没有护栏

**变异体**：`web/gameplay.js:184`
```js
if (!tab.data || Date.now() - loadedAt >= 20_000) loadOverview(tab, true);
// 改成 →  if (!tab.data) loadOverview(tab, true);
```
→ 退回"永不重载"（这正是 R11 A4 修的那个 bug），测试全绿。

**要补**：源码级断言 `activateSection` 里同时含 `loadedAt` 与 `loadOverview(tab, true)`；更好的是把节流判断抽成一个纯函数（如 `shouldReloadOverview(tab, now)`）再做行为测试。

### W7 `-self-check-riot-key` 没有护栏

**变异体**：`main.go:150`
```go
if !riotKeyConfigured() { log.Fatal("Riot API key is not embedded") }
// 改成 →  if false { ... }
```
→ 打包硬校验形同虚设，测试全绿。

**要补**：`riotKeyConfigured()` 的单测（有密文 / 无密文 / 密文损坏三种输入）。self-check 分支本身跑的是 `log.Fatal`，不好直接测，测底层判定函数即可。

---

## 三、原工单里还没做完的三项

### W8 赛季化英雄胜率（R11 的 D2 第二步）

**第一步已完成**：`gameplay.go:1757` 加了 `if match.QueueID != 420 && match.QueueID != 440 { continue }`，前端文案也改成了"最近 N 场排位"（`web/gameplay.js:786`）。

**第二步未开始**：全仓 `*.go` 搜 `season|splitId|seasonId` 仍然零命中。

需要做的（沿用 R11 里的方案，不重复展开）：
- 新增赛季起始时间戳常量表（S26 起始；明年 S27 按年递增）。
- 后端做**按赛季分页扫描 + 落盘缓存**：SGP 每页 20（`sgp_api.go:76`）翻到赛季起点；结果按 `accountHash` 落盘（**用 storage.go 的加盐哈希，不要存 PUUID**）；后续只增量拉到撞上已缓存 gameId 为止。
- 输出 `{championId, games, wins, kills, deaths, assists}`，前端展示 胜率 / KDA / 场次。
- **必须有进度反馈**（"已统计 N 场"），首次要拉几百场。

### W9 打包硬校验只覆盖了 Windows 一侧

现状：
- `build-desktop.sh` ✅ 已新建，缺 key 文件或密文为空时 `exit 1`。
- `build-desktop-windows.ps1:44` ✅ 无 key 时 `throw`；`:83-84` 会跑 `--self-check-riot-key`。
- ❌ `desktop/package.json` **没有 `beforePack` 钩子**（grep 无命中）。

缺口：`build-desktop.sh` 是在 macOS 上交叉编译 Windows exe，**没法执行自检**，所以"密文确实注入进二进制了"这件事在 macOS 链路上完全没有验证。而 R11 查出来的原始事故正是发生在 macOS 手敲构建上。

**要做**：给 `desktop/package.json` 加 `beforePack` 钩子，在打包前对 `desktop/backend/loot-service.exe` 做一次**静态校验**（不需要执行）——比如用 Node 读文件、按 `riot_api.go:56-60` 的派生方式尝试解密，扫不到可解密密文就让打包失败。这样 macOS / Windows 两条链路都被兜住。

### W10 两项等真机日志，**不是 GPT 现在能做的**

这两条埋点已经落地，需要用户跑一次真机把日志导出来再定，请**不要**凭猜改代码：

| 埋点 | 位置 | 等什么 |
|---|---|---|
| `sgp_ranked_stats_shape` | `sgp_api.go:615-620` | 国服他人胜率：看 `queue_keys` 是否含 `losses`、`privacy` 取值 |
| `overview_loadout_shape` | `gameplay.go:855` | 排位召唤师技能：`spell1_present` 计数决定是"目录失败"还是"字段缺失" |
| `counters_shape` | `champions_structured.go:694` | 优劣势对抗为"—"：看 `in/out/dropped_no_meta/dropped_no_play` |
| `load_top_players_shape` | `champions_structured.go:342` | "场次最多的玩家"：看 `parsed` 实际几行（**前后端上限本来就是 5，不要去改常量**） |

---

## 验收清单

- [ ] W1 两个详情页的第四/五/六件都在核心装那一列内、竖向堆叠；底部无全宽 depth 条；无重复渲染
- [ ] W2~W7 六个变异体逐个复现，确认现在**都能被测试杀掉**
- [ ] W8 赛季聚合可用且有进度反馈；缓存不含 PUUID
- [ ] W9 在 macOS 上跑 `./build-desktop.sh`，若人为清空密文，打包必须失败
- [ ] `go test ./...` + `node --test web/champions.test.cjs` 全绿，`gofmt -l` 只剩 `yourgg_arena.go`

**测试纪律**（本项目反复踩过）：
1. 前端正则型断言很容易是假护栏，**每条新断言都要用上面给的变异体自验一次**。
2. 变异脚本要能指定**第 N 处**匹配——`const depthGroups = renderBuildDepthGroups(build);` 在 `champions.js` 里就有两处（1411 非排位 / 1427 排位），只替换第一处会得出错误结论。
3. 跑测试用 `node --test web/champions.test.cjs`，**不能用 `node --test web/`**。
4. 变异测试拷副本时除 `*.go`/`go.mod`/`go.sum` 外必须带 `prestige_chromas.json`（`prestige.go:20` 有 `go:embed`），否则 baseline 就编译失败、所有变异体会被误判成 killed。
