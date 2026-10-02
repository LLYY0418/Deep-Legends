# R150 上游单资源探测

日期：2026-09-24。以下只记录公开响应的字段名和用于对账的统计值，不记录 Cookie、令牌或请求体。每个 URL 单独请求；临时原始响应仅存于系统 `/tmp`。

## YOUR.GG 斗魂榜单

`GET https://api.your.gg/kr/api/arena/champions` 成功。英雄行字段为 `averagePlacement, banRate, bans, championId, firstPlacementRate, lastPlacementRate, matches, score, tier, winRate`；没有 `pickRate`、`pick_rate` 或独立 `rank`。因此头部删除选用率，不拿 OP.GG 的值拼接。排名继续按上游行序给 `i+1`。

| 英雄 ID | YOUR.GG 排名/档位 | 胜率 | 平均名次 | 场次 | OP.GG 排名/数字档位 | 胜率 | 平均名次 | 场次 |
| --- | --- | ---: | ---: | ---: | --- | ---: | ---: | ---: |
| 3 正义巨像 | 1 / OP | 55.9611% | 3.2591 | 3288 | 18 / 2 | 52.9595% | 3.3364 | 642 |
| 901 | 19 / A | 50.8541% | 3.4212 | 3044 | 31 / 2 | 50.3556% | 3.4310 | 703 |
| 157 | 63 / B | 49.8459% | 3.4831 | 2921 | 32 / 2 | 51.8018% | 3.4489 | 666 |

OP.GG 三次分别读取 `https://lol-api-champion.op.gg/api/global/champions/arena/{id}` 的 `data.summary.average_stats`。它的 `win/play`、`total_place/play` 与代码计算一致；YOUR.GG 的 `winRate` 是 0–1 小数，代码乘 100 后与截图的 55.96% 一致。当前分歧来自两个站点的不同样本和排行，不是这两处解析口径错误。

## 数字档位方向

`GET https://op.gg/zh-cn/lol/modes/arena` 的公开页面携带 173 行榜单。按页面中 `tier/rank` 配对统计：1 档排名 1–12（12 位），2 档 13–61（49 位），3 档 62–107（46 位），4 档 108–168（61 位），5 档 169–173（5 位）。确认 **1 最强、5 最弱**；该页没有 0 档样本，`0→OP` 是项目既有 OP.GG 表达的兼容映射，不声称本次 Arena 样本证明了 0 档。

`GET https://hexdata.com.cn/api/hexdata/hextech-insights` 在先取首页会话后成功，含 173 位英雄，英雄级 `tier` 实际值为 1、2、3、4、5。这个聚合接口没有英雄级 `hexTier`；`hexTier` 是 hero-json 海克斯行上的另一种字段，已有真实夹具与解析器确认 `hang/top/elite/npc/trap/insufficient`。两者没有可直接逐行对照的同一对象；本单按强到弱做展示归一 `1→S, 2→A, 3→B, 4→C, 5→F`，不把它称为上游公布的字段等价关系。

本机没有 Windows/英雄联盟客户端，无法生成 R150 所需的真实 LCU 界面截图；可在本机对 Web 渲染、响应契约和 CSS 做自动化验证，真机对账留待带该代码的 Windows 构建。
