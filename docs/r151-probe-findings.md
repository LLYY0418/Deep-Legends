# R151 YOUR.GG 斗魂聚合探测

日期：2026-09-24。三个英雄各对既有单英雄端点发起一次独立 GET，请求不带 Cookie。下文只记录字段、结构和条目覆盖数量，不记录胜率、评分、场次等指标值；原始响应仅暂存系统 `/tmp`，不入仓库。

## 单英雄响应

`GET https://api.your.gg/kr/api/arena/champions/{id}` 对英雄 887、3、157 均返回 HTTP 200。根对象为 `success, statusCode, response`；`response` 键为 `version, totalMatches, performance, augments, coreItems, prismaticItems`。`performance` 键为 `championId, matches, bans, winRate, banRate, averagePlacement, firstPlacementRate, lastPlacementRate`。

`augments` 是扁平数组；三份响应每行字段一致：`augmentId, tier, score, winRate, averagePlacement, firstPlacementRate, matches`。没有 `pickRate` 或品质字段；不能推算选用率。`tier` 是 YOUR.GG 的 `OP/S/A/B/C/D/F` 字母，`score` 为数值。三个英雄每一行都有这两项，`augmentId` 在各自数组内无重复。

| 英雄 ID | 海克斯条目 | 银色 | 黄金 | 棱彩 | 返回 ID 对上 CommunityDragon |
| --- | ---: | ---: | ---: | ---: | ---: |
| 887 | 179 | 57 | 67 | 55 | 179/179 |
| 3 | 185 | 60 | 72 | 53 | 185/185 |
| 157 | 173 | 63 | 58 | 52 | 173/173 |

品质由 CommunityDragon `cherry-augments.json` 以 `augmentId=id` 查得；在每个英雄数组的首项、中项、末项另做 3/3 抽样核对。该目录含其他模式与当前英雄不可选的条目，不能把目录总量当作单英雄应返回数。YOUR.GG [斗魂英雄页](https://your.gg/en/kr/arena/champions)默认显示英雄 3，海克斯首屏 6 项加“更多 179 项”，合计 185 项，与该英雄的接口响应相符；未见首屏截断成为 API 截断的证据。三位英雄的三种品质都有数十项，并非每品质仅前几项。

`response` 无 `synergies`、`partners` 或类似搭档段，故搭档不能切换到 YOUR.GG。`coreItems` 和 `prismaticItems` 仍是现有装备数组；其条目含 `itemId, tier, score, winRate, averagePlacement, firstPlacementRate, pickRate, matches`。

## 网页附加请求边界

已在站点页面确认默认英雄及海克斯完整展示，并检查该页直接引用的英雄页脚本：没有发现写死的其他 Arena API 路径。Codex 内置浏览器没有 Network 事件接口；隔离的无登录 Chrome 单页抓包被站点返回 403，未能可靠取得网页 Network 面板里的实际请求列表。因此**不能断言网页没有其他接口**，也未按猜测批量请求 `/augments` 等路径。本次实现只使用已实测且项目既有的 `/kr/api/arena/champions/{id}`。

## 分支与当前数据来源

选择 **分支 A**：三个英雄的聚合数组均有银、金、棱彩三类，全部行有官方 `tier` 与 `score`，ID 全部对得上现有目录；接口失败或品质不完整时整段回退 OP.GG，不拼榜。搭档段仍取 OP.GG。

| 斗魂板块 | 当前来源 | 缺失或失败时 |
| --- | --- | --- |
| 详情头部 | YOUR.GG 榜单中当前英雄行 | 缺行则隐藏该组统计与梯度 |
| 左侧英雄榜单 | YOUR.GG `/kr/api/arena/champions` | 无替代榜单 |
| 海克斯统计、字母、评分 | YOUR.GG 单英雄聚合 `augments`；名称、图标、品质由 CommunityDragon 对照 | 整段回退 OP.GG `augment_group`；本地 S/A/B 字母，`Score=0` |
| 核心装备 | YOUR.GG 单英雄聚合 `coreItems`；装备名称和图标由 CommunityDragon 对照 | OP.GG `core_items`，沿用 R150 本地档位逻辑 |
| 棱彩装备 | YOUR.GG 单英雄聚合 `prismaticItems`；装备名称和图标由 CommunityDragon 对照 | OP.GG `prism_items`，沿用 R150 本地档位逻辑 |
| 搭档协同 | OP.GG `synergies`；英雄元数据来自本地目录 | 空段不展示 |
| 技能 | OP.GG `skill_masteries` 解析到详情对象 | 当前斗魂页面没有技能板块；无额外回退 |
| 反制 | 当前斗魂模式未启用该段（`HasCounters=false`） | 当前斗魂页面没有反制板块 |
| 冠军对局 | YOUR.GG `/kr/api/arena/champions/{id}/top-builds`；“我的战绩”另取本地对局 | YOUR.GG 无符合条件对局时显示不可用，不借其他来源补值 |

工单背景将技能和反制概称为 OP.GG 段；上表按当前运行代码区分了“已解析但页面未展示”和“模式未启用”，没有据此增加超范围 UI。
