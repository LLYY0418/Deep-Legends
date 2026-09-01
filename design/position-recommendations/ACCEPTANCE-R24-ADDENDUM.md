## R24 追加工单（D/E/F + P1 收尾）验收 — 2026-08-26 全部通过

对照 `ACCEPTANCE-R24.md` 第八节（D/E/F）与正文 P0/P1 清单逐条复测，全部关闭。

### D. 分路按钮挪到四格下方 —— ✅ 真机验证

Hero 改回单行网格，第三列变成"四格指标 + 分路按钮"纵向堆叠，按钮 `flex-wrap: wrap`
按 2×2 排列，不再受列宽挤压。真机截图 + 高度实测：

| 分路数 | hero 高度 | vs 基线(176px) |
|---|---|---|
| 1（莫甘娜，不显示按钮） | 176px | +0 |
| 2（卡蜜儿，一行两个） | 176px | +0 |
| 3/4（亚索/贾克斯，两行） | 218px | +42px |

1440~1920px 全宽度范围高度恒定（列宽由 `auto` track 锁定，不随视口拉伸），
不会出现"越宽越矮/越窄越高"的不稳定观感。4 视口 × 4 分路数截断审计**全部 0 处截断**
（上一轮 900px/1080px 出现的 `占87.…` 文字截断已消失）。

### E. 按钮内颜色复用全局约定 —— ✅

`champions.js` 把 `<small>` 拆成 `<span class="metric-win">`/`<span class="metric-pick">`，
CSS 把选择器并入既有共享规则（`champions.css:100-101`），没有新写颜色值。
截图确认：胜率绿、占比蓝，与四格指标同色系。

### F. 单套召唤师技能视觉打磨 —— ✅

`renderSpellsCard` 单条数据时加 `is-single` 类，卡片撑满整行（实测宽度 402px = 整个卡片列宽），
不再有另一半空白。同时确认了这不是数据问题——真实上游数据核实过，打野惩戒硬约束下
很多英雄确实只有一套主流召唤师技能组合。

### P0 三项 —— ✅ 全部关闭

- P0-1 图标恒灰：新增 hover/active 的 de-gray 规则，真机 computed style 确认选中态图标已变色。
- P0-2 克制点进 400：`openCounterDetail` 补了 `firstPositionOf(ranked) || state.selected?.position || "mid"`
  三级兜底 + 位置仍为空时前端直接给"请先选择分路"的可见态、不再发出畸形请求。
  真机复现：点击不在榜单里的诺手，现在请求带 `position=top`，正常渲染详情，无报错。
- P0-3 窄屏截断：随 D 项重排一并解决，0 处截断。

### P1 五项 —— ✅ 全部关闭（读码 + 部分真机确认）

1. `positions.length >= 1` 时也渲染切换条（原来 `> 1` 吞掉单分路推断态的"自动"角标）。
2. `resetLivePositionOverrides(previous,next)` 在每次 `loadLive()` 刷新时调用，
   `gameId` 变化或离开 ChampSelect 即清空手动覆盖。
3. `selectLivePosition` 改用 `selectedTarget.key`（覆盖生效后重新算出的新 key）删缓存/失败记录，
   不再误删旧位置的。
4. 缓存键新增 `spellKey`（召唤师技能排序拼接），中途换惩戒会重新触发推断。
5. 对线样本空态 tooltip 改用 `resolvedPosition`，不再用未推断前的原始 `self.position`。

### 护栏：两轮变异测试，17 个变异体全部被杀，0 存活

**Go（对比 /tmp/mut 快照）**：`handleGameplayRecommendations` 补了 3 组 `httptest` 端到端用例
（role-rate 探测两跳请求 / 惩戒跳过探测 / 显式 client 位置），针对性变异 7 个全部被杀
（PositionSource 整行删除、探测位改错、惩戒短路、needsPositionInference 失效、
role-rate 从"取第一个"改成真正的最大值比较、hasSmiteSpell 阈值改错、白名单 fail-closed）。
`TestStructuredOPGGDetailSchema` 不再自己抄生产逻辑测自己，改成真调
`loadStructuredDetail` 走 mock HTTP；`TestStructuredDetailOmitsPositionsOutsideRanked`
从只测 1 个模式扩到 arena/aram/urf/nexus-blitz 四个模式各自断言请求路径。

**JS（对比 /tmp/jsmut 快照）**：10 个变异体（D/E/F 三项各 1~2 个 + P1 五项）全部被杀。

**结论：这轮返工把上次验收指出的 3 个 P0、5 个 P1、以及两个测试假护栏全部关闭，
未发现新的回归。** 用户这次追加的两条布局/颜色反馈也已落地并真机核验。
