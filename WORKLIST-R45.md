# WORKLIST-R45（给 GPT 执行）

> R44 主工单（A~I）和 ADDENDUM（A~D）验收结论：**全部真实现，没有一处"假装做了"，
> 绝大多数变异体被杀，这是迄今质量最好的一轮。** Go 523 通过、JS 217/217 全绿。
>
> 这份工单只收尾验收挖出的遗留问题，量不大。

---

## A 组（P0，唯一的真功能缺陷）：斗魂"一键应用装备"仍然会报错

ADDENDUM 的根因段里点名过这条连带影响，但这轮没改到：

`validateGameplayItemSetContext`（现 `gameplay.go:4907-4934`）仍然只用
`gameplayReferencesMatch(currentReference, playerReference)`（`:4922`）逐条比 PUUID/summonerId，
**没有 cellId 兜底**。斗魂英雄选择阶段 puuid 全空时必然走到 `:4934` 返回
"英雄选择中没有找到当前账号"。

也就是说：ADDENDUM 把"出装页能出数据"修好了，但用户真去点"应用装备方案"仍然会失败。

**改法现成**：复用这轮刚做好的 `gameplayLivePlayerIsCurrent`（`gameplay.go:4279-4284`），
把 `session.LocalPlayerCellID` 和 `player.CellID` 也纳入判据。

**补测试**：构造 puuid 全空 + 有 cellId 的 champ-select 上下文，断言校验能通过；
变异测试去掉 cellId 判据后必须 FAIL。

> 注：斗魂/海斗现在按 E-3 已经不显示"应用装备方案"按钮了，
> 所以这条对**斗魂**的实际影响是"按钮本来就没有"。但这个校验函数是所有模式共用的，
> 峡谷自定义局等场景一旦出现 puuid 为空同样会挂，仍然要修。

---

## B 组（P1）：一处假护栏 —— C 组桌面端主规则可被静默删除

R44-C 组（战绩卡斗魂两行）功能是真做对了，但护栏有个定位歧义：

测试里用的 `cssBlockAfter(gameplayStyles, ".match-stats.is-arena")` 是 **`indexOf` 首次匹配**。
而 `.match-stats.is-arena` 这条规则在文件里有两份——
基础层 `web/gameplay.css:641` 和窄屏媒体查询里 `:761`，**同名同值**。

于是变异测试实测：**只删掉 `:641`（正是用户投诉的桌面场景）→ 测试依旧全绿**，
因为 `indexOf` 会顺位命中媒体查询里那条。同时删两条才会 FAIL。

**改法**：把断言改成在基础层定位，例如先切片再找：
```js
const baseLayer = gameplayStyles.slice(0, gameplayStyles.indexOf("@container"));
```
然后在 `baseLayer` 里断言这条规则存在。

**这类"测试写对了但定位有歧义"的假护栏比"根本没写测试"更隐蔽**，
请顺手检查一下 `cssBlockAfter` 的其他调用点有没有同样的同名规则歧义问题。

---

## C 组（P2）：补几处纯样式/接线的回归护栏

以下**功能都已验证做对了**，只是没有回归保护，被人手滑改回去不会有测试报警。
逐条补断言即可，不需要改功能代码。

1. **B-2 的标题分隔线**：`web/gameplay.css:1279-1282`（`.live-augment-recommendations +
   .build-recommendation` 的 margin-top:20px + `::before` 1px 线）删掉后测试全绿。
2. **B-3 技能加点跨行**：`web/gameplay.css:1289` 的
   `.build-summary-skill{grid-column:1/-1}`（让技能加点在海斗布局里独占一行）删掉后测试全绿。
3. **B-4 棱彩/核心装顺序**：把 `${lateBands}${itemBuildLayout}` 对调后测试全绿。
4. **B-4 斗魂卡片样式**：`web/gameplay.css:1292-1293`（对齐 champions 那套的
   padding/background/border）删掉后测试全绿。
5. **ADDENDUM-B 的接线层**：`gameplay.go:4163`（把 cellId 参数传成 nil）和
   `:4097`（删掉 `localPlayerCellID = champSelect.LocalPlayerCellID` 赋值）
   两处变异都存活。根因是唯一走 `handleGameplayLive` 的测试
   （`gameplay_test.go:1870`）用的是 `{"myTeam":[],"theirTeam":[]}` 空名单，走不到这段。
   **建议补一条 handler 级测试**：champ-select 返回 `localPlayerCellId:0` +
   两条 puuid 全空的 cell，断言 `response.Players[0].IsCurrent == true`。

---

## D 组（P2）：几处口径不一致（用户暂未反馈，但迟早会问）

这几条都不是缺陷，是"对局页和详情页还差最后一步一致"：

1. **核心装条数**：后端现在按 `championCoreRecommendationLimit=15` 给足了，但
   - 对局页 `LIVE_CORE_OPTION_LIMIT = 6` 且**没有"展开全部"按钮**（斗魂详情页
     `web/champions.js:1139` 是折叠 6 + `renderArenaExpand` 可展开到全部）
   - **更要注意**：排位/大乱斗详情页 `web/champions.js:1705 buildItemRoutes` 还是 `slice(0,5)`、
     海斗详情页 `:932 renderMayhemItemRoutes` 也是 `slice(0,5)`——
     **后端提到 15 之后详情页自己没跟上，现在对局页(6) 反而比排位详情页(5) 多一条**。
     建议详情页也提到同一常量口径，并给对局页加"展开全部"。
2. **A-4 零值的显示形态**：后端给 null 之后，对局页
   `web/gameplay.js:3815-3819 renderCoreStats` / `:3807-3811 renderOptionStats`
   没有 `if (!hasPick && !hasWin) return ""` 守卫（只有 `:3821 renderDepthStats` 有），
   `rate(null)` 走 `:3882` 返回 `"—"`。所以海斗核心装行是"胜率 — 场次 —"，
   而详情页 `web/champions.js:1792` 是整块 `return ""` 什么都不画。
   观感缺陷（假装有 0% 数据）已经消除，但要"和详情页一致"还差这一步。
3. **海克斯排序口径**：两边确实是同一份 `detail.RecommendedAugments`，但
   详情页 `web/champions.js:1470-1471` 是按后端顺序 `slice(0,9)`，
   对局页 `web/gameplay.js:3395-3397` 是本地按 S/A/B/C 档位重排再按 score/games 排、且不截断。
   **数据源一致但呈现不一致**——如果用户说的"对局页海克斯和详情页不一样"包含顺序，这条要统一。
   建议以详情页口径为准（用户是拿详情页当基准在对比的）。

---

## E 组（P3）：两处遗留的旧名称字符串

E-2 把装备方案名称改成 `DL · 兰博 · 上路` 之后还有两处没跟：
1. `web/demo-data.js:719` 演示模式的 mock 响应仍返回 `Deep Legends · ...`（只影响演示态）。
2. `lcuItemSet.StartedFrom` 仍是 `"Deep Legends"`（该字段不进游戏内下拉框，不影响 E-2 目标）。

第 1 条建议改（演示态和真实态显示不一致会让人困惑），第 2 条可改可不改。

---

## 交付要求

- `go build`/`go vet`/`go test ./...` 全绿；`node --test web/*.test.cjs desktop/*.test.cjs` 全绿。
- **A 组必须补测试 + 变异测试**（这是唯一的真功能缺陷）。
- **B 组改完后请自己验证一次**：只删 `web/gameplay.css:641` 那一条，测试必须 FAIL。
- C 组五条都是补断言，补完后逐条自测"删掉对应实现 → 测试 FAIL"。
- **另外提醒**：斗魂的"我方栏 5 个空壳玩家"和"对方栏空"这两个现象，
  ADDENDUM 已说明修不掉（需要 champ-select 结构探测回传才能定位身份藏在哪个键）。
  用户下次打斗魂导出日志后，请重点看新事件 `lcu_champ_select_session_shape` 的
  `my_team_keys` / `my_team_nonzero_counts` / `their_team_length`。
