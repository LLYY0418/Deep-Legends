# WORKLIST-R126：生涯页左侧「当前背景」与右侧「生涯背景」选择器对不上（右侧落到黑暗之女）

诊断人：Claude（只读：日志 `lol-loot-diagnostics-0922-1630.jsonl` + 代码，未改仓库代码）。
执行人：GPT。
日期：2026-09-22。
状态：P1 已执行并完成自动化验收；P2 为非阻塞诊断建议，本轮未扩做。真机“先收藏后生涯”流程待 Windows + LCU 环境复核。

---

## 0. 结论

用户没有手动指定生涯背景，客户端按「最高熟练度英雄」自动显示李青的**原皮**（backdrop 类型 `highest-mastery`）。后端要把这张图对应到一个皮肤 ID，靠的是在皮肤列表里找 `championId*1000`（原皮 ID）。但只要收藏页的皮肤目录加载过，生涯页就改用收藏目录，而**收藏目录是去掉了原皮的**，于是永远匹配不到，`backgroundSkinId` 变成 0，名字变成「客户端当前背景」。前端拿到 0 后，英雄下拉框兜底选列表第一个英雄——黑暗之女。左边显示李青原画、右边选中安妮，就是这么来的。

## 1. 证据

### 1.1 日志：同一个会话里，前后两种结果

| 时间 | 皮肤来源 `source` | 皮肤数 | 匹配结果 |
|---|---|---|---|
| 07:41:49（第 1 次读取，收藏目录还没加载） | `lcu-catalog` | 2121 | `background_skin_id: 64000`（李青原皮），`catalog_matched: true` |
| 07:45:43 起之后 728 次 | `collection` | 1948 | `background_skin_id: 0`，`catalog_matched: false` |

两次的 `backdrop_type` 都是 `highest-mastery`。2121 − 1948 = 173，正好约等于英雄数（每个英雄一个原皮 + 1），说明差的就是原皮。

### 1.2 代码

- `backend/profile_facade.go` `loadFacadeSkins()`（约 1180 行）：只要 `a.allSkins` 非空就直接返回它，来源标 `collection`。
- `backend/catalog.go:376-380, 394`：`a.allSkins` 来自 `Snapshot.All = displayAll`，而 `displayAll` 明确跳过 `isBaseSkin(skin)`。
- `backend/facade_backdrop.go` `applyFacadeBackdrop()`：`highest-mastery` / `recently-played` 时在 `state.Skins` 里找 `ID == backdrop.ChampionID*1000`，找不到就 `BackgroundSkinID = 0`、名字写成「客户端当前背景」（截图三左上角正是这几个字）。
- `backend/web/suite.js:1359-1362` `hydrateFacadeDraft()`：`backgroundSkinId` 为 0 时 `hero` 取 `skins[0]?.championId` → 列表第一个英雄（黑暗之女）。而且这个草稿只在第一次或 `force` 时生成，后面即使数据对了也不会自动改回来。

## 2. 修复方向

### P1-a 后端：生涯页的皮肤目录必须包含原皮

`loadFacadeSkins()` 不要直接复用收藏页的 `a.allSkins`（那是给收藏展示用的、去掉原皮的列表）。两种做法任选其一，推荐第一种：

1. 在 `catalog.go` 生成快照时，把未过滤的完整目录（含原皮）另存一份（例如 `a.allSkinsWithBase`），`loadFacadeSkins()` 用这份；
2. 或者保持现状，但在 `loadFacadeSkins()` 里把 `lcu-catalog` 里的原皮补进去再返回。

原皮的拥有状态沿用 `applySkinOwnership` 的规则（拥有该英雄即拥有原皮），不要另造规则。

### P1-b 后端：不依赖目录也要知道是哪个英雄

`applyFacadeBackdrop()` 已经从客户端拿到了 `backdrop.ChampionID`，但没有往前端传。给 `facadeProfile` 加一个 `backgroundChampionId` 字段：`highest-mastery` / `recently-played` 时直接等于 `backdrop.ChampionID`；`specified-skin` 或匹配到具体皮肤时等于 `matched.ChampionID`。这样即使将来目录又缺了某个皮肤，英雄也不会选错。

### P1-c 前端：兜底顺序

`hydrateFacadeDraft()` 的 `hero` 改为：匹配到的皮肤的英雄 → `profile.backgroundChampionId` → `floor(backgroundSkinId/1000)` → 空（不选任何英雄，显示「请选择英雄」）。**不要再兜底到 `skins[0]`**——宁可不选，也不能选一个跟左边对不上的英雄。

另外：用户还没动过选择器（草稿未修改）时，新的一次读取如果得到了不同的背景，应当用新值重建草稿；用户动过就保留草稿（沿用现有的 `facadeDraftDirty()` 判断）。

### P1-d 右侧高亮

原皮进入目录后，右侧皮肤网格里要能看到并高亮这张原皮（它就是当前背景）。「应用背景」按钮对当前已生效的皮肤仍然隐藏（沿用 `backgroundApplied` 逻辑）。

## 3. 验收

- 用日志里的真实形状写后端测试：backdrop `{type: "highest-mastery", championId: 64}`，`a.allSkins` 为去掉原皮的列表 → `state.Profile.BackgroundSkinID == 64000`、`BackgroundChampionID == 64`、名字是李青原皮的名字而不是「客户端当前背景」。**对抗变异**：把 P1-a 撤掉，该测试必须 FAIL。
- 前端测试（抽函数跑）：`backgroundSkinId: 0, backgroundChampionId: 64` → `draft.hero === "64"`；两者都没有 → `draft.hero === ""`，不能是 `skins[0].championId`。
- 手动：先打开收藏页让皮肤目录加载，再进生涯页，右侧英雄是李青，网格里原皮被高亮，左右一致。
- 诊断：`facade_skin_state` 在收藏目录加载后仍应是 `background_in_catalog: true`。

---

## P2　生涯页打开期间每 2 秒整页重读一次

同一份日志里 `facade_load_cost` 共 730 次，其中 728 次 `trigger: "sse"`，从 07:45:43 到 08:10:06 几乎每 2 秒一次（`facadeEventThrottleInterval = 2s`，`connection_manager.go:22`），恰好在用户离开生涯页时停止。每次重读都会请求 `/lol-chat/v1/me`、`/lol-regalia/...`、`/lol-challenges/v1/summary-player-data/local-player`（最慢 1.1 秒）、`/lol-collections/.../backdrop`、召唤师资料，25 分钟里对客户端多打了约 3,600 次请求，而 728 次读到的结果完全一样。

触发源是 `isFacadeLCUEvent()` 监听的四类推送之一（最可疑的是 `/lol-chat/v1/me`，客户端会频繁推送聊天状态），但日志没有记录具体是哪个 URI，没法定论。

建议（不阻塞 P1）：
1. `handleFacadeLCUEvent` 记一个按 URI 聚合的诊断（每 60 秒一条：各 URI 触发次数），先把触发源坐实；
2. 前端 `refreshFacadeFromEvent()` 拿到新数据后先比对内容（背景、头像、头衔、签名、段位展示等关键字段），没变就不重渲染；
3. 后端对 `/lol-chat/v1/me` 推送只在关心的字段（`statusMessage`、`lol` 里的段位展示、`availability`）变化时才广播 `facade:changed`。
