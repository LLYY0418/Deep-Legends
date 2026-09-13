# 2026-09-12 战利品名称修复

## 结论与数据依据

本次七个缺名条目的原因是名称目录覆盖不全：原来只读取 `rcp-be-lol-game-data/global/zh_cn/v1/loot.json`，该目录未包含这七个 ID。旧宝箱/材料名称在战利品前端翻译文件中；守卫、召唤师图标分别有独立目录。

| 截图 ID | 已核实的中文名称 | 名称字段 |
| --- | --- | --- |
| `CHEST_128` | 英雄魔法引擎 | `loot_name_chest_128` |
| `MATERIAL_clashtickets` | 冠军杯赛挑战券 | `loot_name_material_clashtickets` |
| `WARD_SKIN_RENTAL_12` | 光辉守卫 | ward `id=12, name` |
| `WARD_SKIN_RENTAL_23` | 金球守卫 | ward `id=23, name` |
| `WARD_SKIN_RENTAL_44` | 光学增强器 守卫 | ward `id=44, name` |
| `WARD_SKIN_RENTAL_63` | 星之守护者 守卫 | ward `id=63, name` |
| `SUMMONER_ICON_782` | 心形爆破 | icon `id=782, title` |

上述名称直接来自客户端资源的中文提取数据：[战利品翻译](https://raw.communitydragon.org/latest/plugins/rcp-fe-lol-loot/global/zh_cn/trans.json)、[守卫目录](https://raw.communitydragon.org/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/ward-skins.json)、[召唤师图标目录](https://raw.communitydragon.org/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/summoner-icons.json)。

`CHEST_128` 的描述字段说明它是等级提升奖励，包含英雄碎片且不需要钥匙。其[原始图标](https://raw.communitydragon.org/latest/plugins/rcp-fe-lol-loot/global/default/assets/loot_item_icons/chest_128.png)与第二张截图客户端左下角的蓝金色胶囊一致。挑战券的描述为可用于进入冠军杯赛，对应的[资源图标](https://raw.communitydragon.org/latest/plugins/rcp-fe-lol-loot/global/default/assets/loot_item_icons/material_clashtickets.png)也存在。名称及用途来自上述翻译字段，没有按照图片外观猜名称。

## 用户日志能证明什么

输入为 `lol-loot-diagnostics-0912-2220.jsonl`（551 行）。原日志没有逐条完整 loot ID，因此不能只靠它确认这七条各自的数量，或认定挑战券应该在客户端当前截图的哪个位置显示。

- `log_seq=376` 的 `loot_map_shape`：原始 42 条，保留 41 条，零数量丢弃 0 条；类型统计含 CHEST 5、MATERIAL 3、WARDSKIN_RENTAL 4、SUMMONERICON 1，和截图相关分组吻合。
- `log_seq=370` 唯一的 `loot_name_fallback` 是没有类型、没有键和名称的空壳，不能对应到截图中的七条。
- 原实现只把空名称或“未命名战利品”计为缺名；当名称已经退回原始 ID 时，就漏掉了缺名诊断。这解释了截图缺名但日志缺少对应记录的现象。
- `log_seq=369` 的 `/lol-inventory/v1/inventory` 400 属于通用拥有权库存读取。实际战利品来自独立的 `/lol-loot/v1/player-loot-map`；不能把该 400 当作本次名称缺失的根因。

未因名称缺失删除真实库存条目，也未推断它们是过期、隐藏或错误库存。

## 实现

- `loot_metadata.go` 并发读取四类目录，优先本机客户端，失败时读取公共中文目录并复用现有缓存。所有读取共用调用方原有的 8 秒预算；本机单次目录读取上限 1 秒。请求均为固定目录 GET，不向公共服务发送玩家库存 ID。
- 守卫与图标按各自前缀和目录 ID 映射，避免和英雄皮肤或 `storeItemId` 混淆。翻译目录按 `loot_name_*` 和 `loot_description_*` 补全历史物品。
- 两个已核实的旧物品另有中文及图标兜底，目录不可用时仍可显示。客户端已有有效名称优先，公共目录仅补缺。
- 首次收藏快照和账户刷新都使用同一加载器。
- 缺名统计现在覆盖原始 ID 回退；日志仅记录前缀和类型等信息。新增四类目录的来源/可用性诊断，不记录完整库存 ID 或数量。
- 没有调整收藏页面布局，没有构建安装包。

## 可复现数据

`testdata/loot-names-0912/` 保存三个公开目录的必要字段子集，未复制用户账号日志。以下 SHA-256 对应下载时的完整源文件，可用于确认本次取样依据；`latest` 后续可能变化。

| 源文件 | SHA-256 |
| --- | --- |
| `trans.json` | `2d2c0365f738df9d172741ac1a32d1500e00277156a9d47acdcd0ca4f18e6719` |
| `ward-skins.json` | `64fd9064233f7561a2ecf5f9e8473c1ceedb06221d8fdbe6d571d63561043d5b` |
| `summoner-icons.json` | `7a40781cc8077ebe3c5b4e408d9ff9decbe93e2055b36af404ba4068e214c362` |

## 验证

- Go 相关回归通过：真实字段子集经首次快照及账户刷新显示全部七个名称和图标，数量/ID 不变；覆盖公共目录失败退化、缓存、共享截止时间、保留本地名称、未知条目和诊断隐私。
- Node 相关回归 10 项通过：包含七条卡片渲染、名称待补全提示消失、未知条目保留和既有 R61 合约。
- 新增目录加载、首次快照和账户刷新相关 Go 竞态检测通过。
- 本机没有连接用户的 Windows 游戏客户端；运行时结果仍需在用户下次自行构建后核对。
