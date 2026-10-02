# R166 执行账本

## P1 能力雷达

- `abilityRadarScore` 改为工单锚点间分段线性插值；0→0、1→50、≥2→95，`abilityGrade` 未改。780/840 的 DPM 为约 42.3 分，对手为 50 分；767/988 为约 26.6 分。
- SVG 顺序改为网格、黄色填充、无填充的灰色轮廓、黄点；黄点半径 3、描边 1.5。锚点解释只在代码注释中。
- 总览能力分由请求处理时现算，内存响应缓存 CN 2 秒、KR 1 分钟，无持久化能力分；重启即清空，因此不增加能力映射版本字段。
- Go 锚点、边界、单调性、示例差距与 Node 七项渲染测试已覆盖。回退平方公式及颠倒 SVG 顺序的两个变异各自触发对应测试失败，整文件恢复并核对字节一致。
- 本机演示账号在 1200/960 宽页面可看到能力雷达且轮廓显示正常；演示数据不是工单截图中的同一账号，**同一账号前后对照截图仍待游戏客户端可用时补拍**。

## P2 总览海斗短名称

- 仅改总览近期战绩卡片的 `rankedQueueNoun` 与 `rankedQueueSwitcher` 为“海斗”；模式名、英雄页页签及其他文案未动。原 `r116e.test.cjs` 对应断言已同步，并继续检查仅有一个合并页签。
- 本机演示账号没有海斗近期战绩，故 1200/960 宽的“近 20 场海斗”及三页签**真实截图仍待同一账号补拍**；未将演示账号的排位标题截图冒充海斗验收。

## P3 详情首次加载

1. OP.GG RSC 请求新增 `champion_upstream`：只记 host、kind、状态、字节数、耗时、缓存状态；不记 URL/cookie。`mayhem_detail_phases_ms` 新增 `rsc_wire`、`rsc_parse`、`rsc_descriptions`。本机隔离后端的新日志中，直点 Gwen 的 RSC 上游 402 ms，详情拆分 wire 411 / parse 47 / descriptions 0 ms；悬停预取 Yasuo 的 RSC 上游 1547 ms，点开后详情拆分 104 / 44 / 0 ms。所用事件摘录在 [network-timing-samples.jsonl](../../r166-validation/network-timing-samples.jsonl)。
2. 斗魂详情在 OP.GG 版本/详情之前启动 YOUR.GG 聚合，保留 4 秒预算与原有回退。榜单 `preload=1` 后台预热 OP.GG 版本；通用 `p.fetch` 的 versions 缓存为 20 分钟，额外的 20 分钟内存值令详情不再产生点击后的 versions 日志，过期后重新读取。前端将高手对局请求延后 200 ms，使聚合先拿 YOUR.GG 的 1 秒节流时段。首轮冷启动样本中榜单之后版本回源 730 ms，点开时聚合 slot 等待 0 ms、聚合请求 299 ms、OP.GG 详情 207 ms；高手对局 slot 等待 795 ms、上游 1126 ms。详情接口 0.339 秒、高手对局接口 1.131 秒。最终构建的新隔离冷启动样本：榜单后版本回源 823 ms，点开后无 versions 事件；聚合 slot 等待 0 ms、上游 297 ms，OP.GG 详情 338 ms，详情接口 0.375 秒；高手对局 slot 等待 819 ms、上游 1147 ms，接口 1.154 秒。此前未预热静态目录时详情为 2.324 秒，原因是约 1.5 秒的 Data Dragon 冷装饰请求。
3. 确认冷请求是 Data Dragon 的中文装备/召唤师技能目录。它现在随 RSC 并行启动；斗魂榜单预加载也后台预热目录。冷启动样本中目录请求在点击详情之前完成（716/541/182/179 ms），详情阶段命中内存。
4. 海斗榜单/梯度弹窗的单行停留 150 ms 或 pointerdown 只预取该英雄 OP.GG RSC；离开取消未开始的定时器。后端只允许一个英雄在途，预取使用独立 context 和与详情相同的 `v2|opgg-rsc|aram-mayhem|<slug>` cache key/flight；不触发 per-hero hexdata 预取。新样本：Yasuo 预取后详情 `total=917 ms`、`hero_json=280 ms`；未预取的 Gwen 首次详情 `total=628 ms`、`hero_json=241 ms`，这次直点较快仅是该次网络样本，不作延迟承诺。
5. 完成前四步后，以最终构建连续直点 12 个未缓存英雄，`rsc_wire` 依次为 432、493、1558、361、851、829、707、521、345、454、352、413 ms，只有 1/12 超过 1.5 秒；另一次悬停预取的上游为 1547 ms。未达到“经常 >1.5 秒”的条件，故不改变详情为两段返回，也未改动海克斯列表的到达顺序。首个 Ahri 冷启动总耗时 2431 ms，但其 `rsc_wire` 仅 432 ms，这个样本不能当作 RSC 长杆证据。逐项事件见上述摘录。

## 验证与限制

- `go test ./backend -count=1` 全量通过（192.950 秒）；`go test -race ./backend -run '^TestR166' -count=1` 通过；`node --test backend/web/*.test.cjs` 全量 726 项通过；`git diff --check` 通过。
- 变异测试：平方公式、SVG 顺序、RSC 预取 cache key 脱离详情 flight、遍历榜单预取、斗魂聚合改回同步、预热版本复用移除，六项各自使对应测试失败，逐项整文件恢复并确认字节一致。
- 本机网络实测使用隔离数据目录和公开模式后端，响应与诊断事件来自真实上游；并非 Windows 游戏客户端内的同账号验证。R165 的 Windows 启动 P0 仍待诊断，故本单仅构建可识别的 public 后端验证二进制，不发布安装包。Windows 安装包冷启动、同账号雷达前后图与海斗标题 1200/960 截图仍须后续真机回填。
- 源码版本 0.12.31；macOS arm64 与 Windows amd64 后端验证二进制均嵌入源指纹 `3d9d93fe6424` 且校验通过；key mode 为 **public**，仅保留 `/private/tmp/Deep-Legends-backend-0.12.31-public` 与同名 `.exe`，不是安装包。
