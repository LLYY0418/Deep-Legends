# WORKLIST-R176：移除出装卡片「核心装」路线前面的出门装和竖线（回退 R172）

诊断人：Claude（只读核对源码）。执行人：GPT。日期：2026-10-01。基线 0.12.39（R174 之后）。

## 背景

用户明确要求：出装推荐里「核心装」每一行前面不该有出门装图标和竖线，直接去掉（截图：核心装每行最前面的「短剑 + 两瓶药」加一条橙色竖线）。

这个前缀是 R172 加的，R172 工单是 Claude 把用户的需求理解错了位置（用户要的是符文卡片「最终装备」行，那部分已在 R173 完成并保留）。本工单只回退 R172 的核心装前缀，**不动 R173 / R174**。

## 要改的（范围严格限定）

`backend/web/gameplay.js`：

1. 约 6457 行：`optionList(coreOptions, "route", "暂无核心装备样本", renderCoreStats, isMayhemBuild ? [] : starterOptions[0]?.ids || [])`，去掉第 5 个参数（`leadingIds`），恢复为 `optionList(coreOptions, "route", "暂无核心装备样本", renderCoreStats)`。
2. 约 6417 行 `optionList(...)` 和约 6556 行 `renderConfigOption(option, kind, statsRenderer, leadingIds)`：删掉 `leadingIds` 参数及其内部的 `leading`、`leadingIcons` 逻辑；`config-icons` 的 `data-icon-count` 恢复为 `ids.length`。
3. 顶部独立的「出门装」区块（`build-summary-starter`）**保留不动**。

## 不要动的（共用，不能误删）

- `gameplay.css` 里的 `.route-divider` 样式：**保留**，R173 符文卡片装备行（`renderRuneEquipment`，约 6257 行）仍在用。
- `renderRuneEquipment`、`patchRuneStarterEquipment`、`.specialist-game-spells` 等 R173 / R174 的全部代码。
- `.config-item` 等类名。

## 测试

- 把 `champions.test.cjs` 里 R172 的两条测试（约 3923 行「classic core routes prepend…」、约 3950 行「arena and hextech build routes do not receive…」）改为反向断言，不能直接删光：
  1. 经典模式 `starterOptions` 有数据时，核心装行里**没有**出门装图标、**没有** `route-divider`，核心装顺序和箭头与 R172 之前一致；顶部「出门装」区块仍然存在。
  2. 斗魂竞技场、海克斯大乱斗同样没有 `route-divider`。
  3. `renderConfigOption(option, "route")` 的输出里 `data-icon-count` 等于 `ids.length`。
- 确认 `gameplay.test.cjs` 里 R173 / R174 符文装备行的 `route-divider` 断言（约 984、1117 行）仍然通过。
- 变异：把 `leadingIds` 前缀加回核心装行 → 上面第 1 条测试必须 FAIL（断言失败，不是编译错误）。
- `node --test backend/web/*.test.cjs`、`go test ./backend -count=1`、`go vet ./backend`、`git diff --check` 全绿。

## 收尾

- 版本按仓库惯例递增（与 R175 同批则共用一个版本号，由执行时的实际顺序决定）。
- `docs/WORKLIST-INDEX.md` 加 R176；R172 那行状态改为「核心装路线前缀已由 R176 移除」。账本 `docs/history/ledgers/r176-execution-ledger.md`。
- 界面红线不变：不新增任何说明文字或 tooltip。

## 真机验收（用户）

出装推荐里「核心装」每一行从第一个核心装图标开始，前面没有出门装和竖线；顶部「出门装」区块还在；符文推荐的「绝活哥 / 职业选手」装备行里的出门装 + 竖线仍然在。
