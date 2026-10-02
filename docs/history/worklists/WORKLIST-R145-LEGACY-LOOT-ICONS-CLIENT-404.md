# WORKLIST-R145：英雄魔法引擎 / 冠军杯赛挑战券图标缺失——客户端 `/fe/` 路径 404，图片接口没有回落到 CommunityDragon

诊断人：Claude（读了 `lol-loot-diagnostics-0924-1121.jsonl`、用户截图、`backend/main.go` 图片接口）。
这一轮 Claude 已经把代码改完（§3），执行人 GPT 负责真实依赖下的复核、真机验证和发版（§5）。
日期：2026-09-24。
基线：0.12.19，HEAD `35788b9f`，工作区含 R144 的未提交改动和 R140 的未提交改动（`web/app.css`、`web/favorites-facade.js`、`web/r130.test.cjs`、`desktop/r140-banner-layout.cjs`，本单没碰）。
状态：代码已改，待 GPT 复核与真机验证。

用户的问题：账户页里「英雄魔法引擎」（宝箱）和「冠军杯赛挑战券」（材料）显示的是统一占位图，没有真实图标。

---

## 1. 证据（`lol-loot-diagnostics-0924-1121.jsonl`，构建指纹 `202ae4440cfa`）

- 打开账户页后，`lcu_request` 里出现两个 404，都在 `/fe/lol-loot/assets/loot_item_icons/` 下：
  - `chest_128.png`：03:19:07.099、03:19:47.348、03:19:57.371，共三次，全部 404。
  - 另一个被路径归一成 `{id}` 的文件：03:19:03.436、03:19:54.546、03:20:04.551，全部 404。按用户截图，它就是冠军杯赛挑战券的图标（`material_clashtickets.png`；这一步是推断，日志把文件名归一掉了）。
- 这两个图标在代码里的来源：`backend/lcu_api.go` 的 `lootClientIcons` 把 `CHEST_128`、`MATERIAL_CLASHTICKETS` 指到 `/fe/lol-loot/assets/loot_item_icons/…png`；前端 `lootImagePaths` 的固定本地图标（`/loot-icons/…`）里没有这两项，所以只剩这条 `/fe/` 路径。
- 你的客户端没有这两个文件：LCU 返回 404，卡片退化成占位图（截图里的宝箱轮廓和六边形）。`docs/loot-names-0912.md` 早已核实原图存在于 CommunityDragon 的 `rcp-fe-lol-loot/global/default/assets/loot_item_icons/` 下。
- 图片接口 `handleImage`（`backend/main.go`）在 LCU 失败后确实会回落到 CommunityDragon，但 `communityDragonImagePaths` 只认 `/lol-game-data/assets/` 开头的路径，`/fe/…` 路径直接返回空候选，所以回落什么也没取到，最后 404。这就是根因。
- 每一次 404 之后约 10 秒，都有一条 `local_request_client image failed`，`queue_wait_ms` 约 10 秒（03:19:03.439 → 10010，03:19:57.373 → 10000）。这一轮它们和这两个 404 的时间几乎重合。R144 §7.1 里留下的问题“三个 404 里哪个对应 10 秒等待”有了倾向：这一轮已经没有 `.png` 请求（R144 小修生效），10 秒等待仍在，说明它至少和这两个客户端没有的图标有关。这只是相关性，是否消失以修复后的日志为准。
- 同一份日志里 `collection_data_retry_exhausted{attempts:3, blank_entries:1}` 在 03:19:47.106 再次出现，`.png` 请求已不在，R144 的两项修改都在这个构建里生效。

## 2. 根因

客户端不一定带全 `/fe/lol-loot/assets/loot_item_icons/` 下的文件（这两个旧物品的图标你的客户端没有），而应用的 CommunityDragon 兜底只覆盖 `/lol-game-data/assets/`，没覆盖 `/fe/` 战利品图标路径。这两个图标本来就不在前端固定图标表里。

## 3. 已做的修改（Claude，未提交、未打包）

`backend/main.go`：`communityDragonImagePaths` 新增战利品前端图标映射（表 `communityDragonFrontendIcons`）：

- `/fe/lol-loot/assets/loot_item_icons/<文件>` → `/latest/plugins/rcp-fe-lol-loot/global/default/assets/loot_item_icons/<文件>`
- `/fe/lol-static-assets/images/currency/icons/<文件>` → `/latest/plugins/rcp-fe-lol-static-assets/global/default/images/currency/icons/<文件>`（`sanitizeClientImagePath` 本来就放行这条前缀，一并补上）

只接受目录下带主名的单个文件：空主名（`.png`）、子目录、`..`、`?`、`#`、`%` 都返回空候选，不向公共镜像发请求。文件名转小写（镜像里是小写）。客户端有图时仍然优先用客户端的，没有才回落，缓存沿用现有的 CommunityDragon 磁盘缓存（7 天）与负缓存。

新增 `backend/loot_icon_fallback_test.go`：

- `TestCommunityDragonImagePathsCoverFrontendLootIcons`：映射与拒绝的路径。
- `TestHandleImageFallsBackToCommunityDragonForMissingLegacyLootIcons`：LCU 对两个图标返回 404、镜像返回 PNG 时，`/api/image` 返回 200 图片，并且只请求了预期的一个镜像路径；`.png` 空主名返回 404 且不发任何上游请求。

前端 `app.js` **没有改**：卡片本来就会请求 `/api/image?path=/fe/…`，后端现在能取到图，卡片自然显示。界面没有新增任何文字。

## 4. Claude 这边的验证边界

- Go：云端沙箱访问不了 `proxy.golang.org`，第三方依赖用**空实现桩**替换后编译（同 R144）。`go vet ./backend` 通过；全量 `go test ./backend` 的失败集合和改动前一致（都是桩造成的），新增 2 条测试和相关旧测试（`TestCommunityDragonImagePathsCoverPluginAndGameAssets`、`TestR117CommunityImageFanout…`、`TestImagePathTraversalRejected`）通过。**这不是真实依赖下的验证。**
- 对抗变异 3 项，均让对应测试 FAIL，并已整文件还原：关掉映射循环；允许空主名；允许子目录。
- Node：在用户电脑上 `node --test backend/web/*.test.cjs` 693/693 通过。
- **没有实际访问过镜像**：Claude 的抓取工具不返回图片内容，只能确认镜像的这两个 URL 有图（抓取报的是“不支持图片内容”，不是 404），没有下载、没有看图。真机验证时以界面为准。

## 5. GPT 要做

1. **真实依赖下跑全量**：`go build -o <tmp> ./backend`、`go vet ./...`、`go test ./...`，与 R144 之后的基线比较，R135 记录的 5 项既有失败不得增加；确认 `loot_icon_fallback_test.go` 的 2 条通过。
2. **复核变更范围**：本单只动了 `backend/main.go` 和新文件 `backend/loot_icon_fallback_test.go`。
3. **和 R144 同一个发版**：版本号递增，`private` 模式打包（public 产物文件名带 `-public`），账本记 key mode、指纹、SHA256，写 `docs/history/ledgers/r145-execution-ledger.md`，更新 `docs/WORKLIST-INDEX.md`。

用户在真机上做：

- 装新包，联网，打开「账户与物品」：英雄魔法引擎和冠军杯赛挑战券应显示真实图标（第一次显示可能慢半秒，之后走缓存）。
- 导出诊断日志发回来，Claude 看：`/fe/lol-loot/assets/loot_item_icons/…` 的 404 是否变成 200 回落，以及 `image failed`、`queue_wait_ms≈10 秒` 是否消失；不消失就说明它另有原因，另开工单。

## 6. 可选的后续（不要求本轮做）

- **内置这两张图**：`web/loot-icons/README.md` 的既有做法是把这类图标下载后内置（运行时不依赖网络）。本单选了运行时回落，是因为 Claude 这边下不了二进制图。如果以后要求断网也能显示：GPT 从 CommunityDragon 下载 `chest_128.png` 和 `material_clashtickets.png` 放进 `backend/web/loot-icons/`，README 记来源与下载日期，并在 `app.js` 的 `lootImagePaths` 固定表里加 `CHEST_128`、`MATERIAL_CLASHTICKETS` 两行（本地图优先，就不再请求 `/fe/` 路径）。
- **已知边界**：完全断网且客户端也没有这两个文件时，仍然显示占位图。
