# R211 TOPKING 熟练度字段实测

源：TOPKING#asd 的 Riot 全量 champion-masteries、scores 与 OP.GG zh-cn 熟练度页 Flight。原始响应在本目录保存。全量 167 条记录；Riot scores=1130，与全量等级之和一致。以下五条按 championId 对齐，不按页面顺序猜测。

| 英雄 | 等级 | 等级所需标记 | tokensEarned | Riot milestoneGrades | 下一里程碑要求 / OP.GG grades | 本季里程碑 |
|---|---:|---:|---:|---|---|---:|
| 未来守护者 (126) | 72 | 2 | 65 | A+ | {"S-": 2} / S-, S- | 17 |
| 纳祖芒荣耀 (897) | 62 | 2 | 65 | B, A+, A, C, S | {"S-": 2} / S-, S- | 6 |
| 刀锋舞者 (39) | 44 | 2 | 32 | 空 | {"A-": 1} / A- | 1 |
| 铁血狼母 (799) | 44 | 2 | 92 | 空 | {"S-": 2} / S-, S- | 5 |
| 机械公敌 (68) | 42 | 2 | 66 | B, B+ | {"A-": 1} / A- | 1 |

实测映射：`tokensEarned` 的 32～92 是累积标记库，不能直接理解为一两个里程碑槽的完成数。OP.GG `grades` 与 Riot `nextSeasonMilestone.requireGradeCounts` 展开后完全对应，代表下一里程碑所需成绩；OP.GG `milestone_grades` 才对应 Riot `milestoneGrades`，不能把这两个字段混为一谈。槽数来自下一里程碑要求，前五为 2/2/1/2/1，与工单初始的“等级所需标记”2/2/2/2/2 有出入，按实测改映射。

当前点亮按 Riot `milestoneGrades` 与要求逐条匹配（同一成绩不重复占槽），前五得到 0/2、1/2、0/1、0/2、0/1。本赛季最高评价来自 `milestoneGrades` 的最高等级；里程碑号来自 `championSeasonMilestone`；点数进度来自 since / (since+until)，例如杰斯 3246/11000；日期来自 `lastPlayTime`。这些字段与 OP.GG 对应值一致。

验收限制：保存的 OP.GG `activatedIndexes` 全为空，无法验证上述点亮计算是否完全等同于 OP.GG 实际图标。不能宣称点亮一致已通过；应再核对真实客户端的当前里程碑成绩与徽章点亮。当前没有 LCU 真响应，国服仅通过公共结构的样本解析测试，真实 LCU 字段验收待补。

图标复用总览的 summoner-mastery-portrait、同一个 CommunityDragon 客户端 crest 路径；≥10 使用 crest 10，实际等级仍显示72/62等。网格56px圆形金边头像、54px徽章（108px原图），悬停结构等比缩小；徽章不裁切，数字仅压住尖端，加载失败是中性占位。Chromium 780px 五列、等级牌30px及真实弹窗视口边界已核验；英雄头像为本地布局夹具，不声称截图是完整联网客户端。
