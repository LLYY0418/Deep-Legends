# WORKLIST-R189：仓库里的表情显示成 `EMOTE_1468`，提示"客户端数据暂未同步"，也没有拥有状态

诊断人：Claude（看截图 + 读日志 + 只读核对源码）。执行人：GPT。日期：2026-10-02。基线：源码 0.12.52；如果 R188 已合入，以合入后为准，两者不冲突。

## 现象

截图：收藏页"表情"分组里有一张卡：标题 `EMOTE_1468`，下面一行"客户端数据暂未同步，可稍后重试"，图标是一个带问号的通用金色占位图。没有"已拥有 / 未拥有"之类的状态。

## 证据

日志 `lol-loot-diagnostics-1002-2016.jsonl`（0.12.49，运行 `0807172d…`）：

1. 12:12:43 开了一个宝箱（`known_chest_counts.other` 从 0 变 1，又在 12:14:42 消失）。12:14:42 仓库里第一次出现 `EMOTE_` 物品 2 个，`type_counts.EMOTE=2`。
2. 同一时刻两条 `loot_name_fallback`：`loot_id_prefix=EMOTE_`、`display_categories=EMOTE`、`has_loot_name=true`、`has_localized_name=false`。客户端给了 lootName（就是 `EMOTE_1468` 这类 ID），**没给本地化名字**。
3. `loot_metadata_source` 每次都只加载 4 份目录：`loot`（3703 条）、`wards`、`icons`、`translations`。**没有表情目录**。
4. 12:14:49 起表情剩 1 个（另一个被使用或分解），仍然是 `unnamed_type_counts.EMOTE=1`，后面每次刷新都一样。不是同步延迟，等多久都不会变。

## 根因（源码已核对）

1. `backend/loot_metadata.go` 第 22 行的 `lootMetadataCatalogs` 只有 `loot.json`、`ward-skins.json`、`summoner-icons.json`、`trans.json`。守卫皮肤和头像都有单独的目录解析（`parseLootWardCatalog`、`parseLootIconCatalog`），**表情没有**。`loot.json` 里不含 `EMOTE_<id>`，所以名字查不到。
2. 名字查不到时，`lcu_api.go` 约 233 行把 `DisplayName` 退回 `LootID`，也就是 `EMOTE_1468`。
3. 前端 `backend/web/app.js` 2927 行 `lootNamePending`：名字等于 lootId 就认为"未同步"，所以显示"客户端数据暂未同步，可稍后重试"。这句话在这里是**错的**：数据已经同步，只是软件没有表情目录。
4. 图标同理：没有目录里的表情图，只能用通用占位图。
5. 拥有状态：前端 3020 行的 `ownership` 只对 `category === "皮肤"` 生效。其他类型（表情、守卫皮肤、头像）即使客户端给了 `itemStatus`（`OWNED` / `NONE`），也不显示。

## P1　加表情目录：名字和图标

1. `lootMetadataCatalogs` 增加一项：`{"emotes", "/lol-game-data/assets/v1/summoner-emotes.json", "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/summoner-emotes.json", parseLootEmoteCatalog}`。走现有逻辑：先读客户端，失败再读 CommunityDragon 磁盘缓存。
2. `parseLootEmoteCatalog`：解析数组元素的 `id`、`name`、`description`、`inventoryIcon`。跳过 `id < 0` 和名字为空的条目。键写 `EMOTE_<id>`。图片走 `sanitizeClientImagePath(inventoryIcon)`。结果为空时按现有 `requireLootMetadataEntries` 处理。
3. 表情物品命中目录后：`DisplayName` 为目录里的名字，`LocalizedDescription` 为描述，`Asset` 为 `inventoryIcon`。`lootClientIcons` 里如果有 `EMOTE_` 的通用图，不能覆盖目录图（命中目录时优先目录图）。
4. 目录也没有这个 ID（例如国服客户端比 CommunityDragon 新）：名字显示 **`表情 1468`**（类别名 + 数字 ID），不显示原始 `EMOTE_1468`。图标用分类占位图。

## P2　"暂未同步"只用于真正没同步的情况

1. 前端 `lootNamePending` 改为只看后端的 `dataPending`，不再用"名字等于 lootId"来推断。
2. 后端只在 R144 定义的空白记录（没有 ID、名字、类型）且还在重试时设置 `dataPending`，这部分不改。
3. 其他类型名字查不到时，后端统一按 P1-4 的规则给"类别名 + 数字 ID"。没有数字 ID 的，用类别名本身（例如 `表情`）。不再把原始 lootId 当名字显示。

## P3　表情、守卫皮肤、头像显示拥有状态

1. 后端 `LootItem` 已有 `ItemStatus`。对表情、守卫皮肤、召唤师头像三类，按 `itemStatus` 输出 `ownedKnown` / `owned`：`OWNED` → 已拥有；`NONE` → 未拥有；其他值或空 → 不确定（不显示）。
2. 前端 `ownership` 扩展到这三类：已拥有显示现有 `is-owned` 样式的"已拥有"，未拥有显示"未拥有"（新样式类 `is-missing`，颜色用 `--muted`）。皮肤维持现有的"已拥有 / 可升级"，不改。
3. 不新增其他文字或说明。

## P4　诊断

1. `loot_metadata_source` 自动包含 `catalog=emotes`（沿用现有事件）。
2. 物品名查不到时，`loot_name_fallback` 对**所有**记录都加 `item_status`（目前只有空白记录才带），并加 `catalog_hit`（布尔，是否在任何目录中命中）。仍不记数量和账号。

## 测试

Go：

1. `parseLootEmoteCatalog`：构造含 `{id:1468, name:"…", inventoryIcon:"/lol-game-data/assets/ASSETS/Loadouts/SummonerEmotes/…png"}` 的 JSON → 键 `EMOTE_1468`，名字、图片正确；`id=-1` 和空名字被跳过。
2. `enrichLootItemsWithMetadata`：`EMOTE_1468` 物品 + 表情目录 → 名字为目录名、图片为目录图，`loot_name_fallback` 不出现。
3. 目录没有该 ID → 名字为 `表情 1468`，`dataPending=false`，`loot_name_fallback catalog_hit=false item_status=…`。
4. `itemStatus=OWNED` → `ownedKnown=true owned=true`；`NONE` → `ownedKnown=true owned=false`；空 → `ownedKnown=false`。守卫皮肤、头像同样。
5. 空白记录（R144）行为不变：仍 `dataPending=true`，重试耗尽后结算。
6. 客户端目录读取失败 → 回退 CommunityDragon；两边都失败 → 走测试 3 的规则，不报错。

Node：

7. 表情卡：显示目录名，没有"客户端数据暂未同步"。
8. 名字为 `表情 1468`、`dataPending=false` 的卡：没有"暂未同步"。
9. `dataPending=true` 的空白记录：仍显示"暂未同步"。
10. 表情 `owned=true` 显示"已拥有"，`owned=false` 显示"未拥有"，`ownedKnown=false` 不显示状态；皮肤卡的"已拥有 / 可升级"不变。

变异（overlay，必须断言 FAIL）：

| 变异 | 期望 |
|---|---|
| 不注册表情目录 | 测试 2 FAIL |
| `lootNamePending` 恢复"名字等于 ID 就算未同步" | 测试 8 FAIL |
| 前端拥有状态仍只看皮肤 | 测试 10 FAIL |
| 查不到名字时仍显示原始 lootId | 测试 3 FAIL |

`node --test backend/web/*.test.cjs desktop/*.test.cjs`、`go test ./backend -count=1`、`go vet ./backend`、`git diff --check` 全绿。

## 收尾

- 版本递增；`docs/WORKLIST-INDEX.md` 加 R189，R144 那行备注"非空白记录不再显示未同步由 R189 调整"；账本 `docs/history/ledgers/r189-execution-ledger.md`，附演示数据下表情卡改前/改后截图。
- 账本写明：表情 1468 的中文名没有在真机上核对，需要用户验收。

## 真机验收（用户）

1. 安装新版本后打开收藏页的"表情"分组：那张表情显示中文名字和表情图片，不再是 `EMOTE_1468`，也没有"客户端数据暂未同步"。
2. 卡片上显示"已拥有"或"未拥有"，和客户端里看到的一致。
3. 导出日志：`loot_metadata_source` 里有 `catalog=emotes`，表情不再出现 `loot_name_fallback`。
