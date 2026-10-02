# WORKLIST-R160：海斗完整图、斗魂离线图标与详情等待、总览冷启动

日期：2026-09-25。基线：0.12.24。触发：用户提供 `lol-loot-diagnostics-0925-2049.jsonl` 与两张截图，要求按诊断修复。

## 验收项

- [x] 海斗英雄详情不再把 Hexdata 的 `*_small.png` 符号图当完整卡面。按官方目录中的同一增益 ID 使用本地完整图；没有验证过的 ID 保留 Hexdata 原图，不按文件名猜 ID。
- [x] 斗魂海克斯目录请求失败时，统计行仍按上游 ID 原样保留；有本地官方目录对应 ID 的行显示中文名、品质与图标，未知 ID 保留显式占位。
- [x] 斗魂详情不再同步等待 CommunityDragon 增益和装备目录的 4/12 秒网络预算；YOUR.GG 聚合最多等待 4 秒，超时按现有 OP.GG 规则回退并记录原因。
- [x] 战绩、海克斯图鉴等共享目录路径也能使用同一份本地图标，不要求先打开英雄详情或连接 LCU。
- [x] 玩家总览海克斯卡片先读取本地增益目录，不再等待整份符文目录；符文资料仍独立按原路径加载。
- [x] 玩家总览首次读取中文英雄名不再等待 Data Dragon 目录；离线查看韩服玩家时，173 个已核对英雄 ID 的方形图从本机交付。已知当前版本不是 16.19 系列时仍用现有在线目录路径。
- [x] 对成功但排队或加载超过 1.5 秒的图片，按 10 秒间隔记录一条无路径的 `image_queue_slow`；与失败日志分开，后续能核查总览图标的真实可见等待。
- [x] 不提高 Riot App Key 限额和详情并发，不延长比赛 ID 缓存 TTL；额外的 Match-V5 等待继续在诊断中如实保留。
- [x] 后端、前端全量自动测试及版本构建完成。
- [ ] Windows 真机复核海斗、斗魂、总览耗时。

## 资源来源与准确性边界

- 2026-09-24 版 [CommunityDragon 中文 Cherry 增益目录](https://raw.communitydragon.org/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/cherry-augments.json)：554 个 ID，原始 SHA-256 记录在 `backend/data/augment_catalog_20260924.json`。
- 同日 [CommunityDragon Arena 中文增益目录](https://raw.communitydragon.org/latest/cdragon/arena/zh_cn.json)：225 个 ID 的完整图路径及中文名，原始 SHA-256 同样记录在目录文件。按 ID 对齐，不按名称或文件名猜关联。
- 同日 [CommunityDragon 中文装备目录](https://raw.communitydragon.org/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/items.json)：仅作为斗魂装备名称与描述的本地兜底，胜率、评分、样本等统计仍来自 YOUR.GG/OP.GG。
- [Data Dragon 16.19.1 中文英雄目录](https://ddragon.leagueoflegends.com/cdn/16.19.1/data/zh_CN/champion.json)：173 个英雄 ID 的中文名及各自 `image.full` 文件名，原始 SHA-256 记录在 `backend/data/champion_names_16.19.1_zh_cn.json`；173 张对应 PNG 均从同版本官方 CDN 下载并校验后入包。
- 增益目录 554 个 ID 中 543 个图标完成 PNG 校验并入包；其余 11 个上游路径缺文件或不在已验证图片目录。已观察到的 Hexdata 小图与在线目录仍按原有路径补足，未知项保持占位。截图中的斗魂 9 个 ID 与海斗示例均已核对为 256×256 实图。

执行证据见 [r160-execution-ledger.md](r160-execution-ledger.md)。
