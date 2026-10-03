# R197 执行账本

日期：2026-10-03。合并版本 0.12.61。基线 R196 `e3ba8b6fe58cfb1c188bedaa75ca82c307fe70b4`。R198 取代并扩展 P2 仅核对的范围，保留其测试与诊断。

## 逐项实施

| 项目 | 实现与证据 |
| --- | --- |
| P1-1 触发与同局快照 | WaitingForStats 保留原 10 人快照，PreEndOfGame/EndOfGame 启动一次；初始以及每次重试核对 gameflow session gameId，拒绝旧局。结束阶段和大厅返回保留快照。 |
| P1-2 同源详情 | 复用总览 `loadDetailedMatches` 与已有缓存：国服官方 SGP 页及 LCU history detail 回退；只失效本人对应 history 缓存。没有新增观战、第三方身份接口或国服 Riot 直查。 |
| P1-3 匹配 | 队伍 + 英雄唯一匹配；排除已有 PUUID；要求姓名和 PUUID，详情仍隐藏、无候选、歧义均保留隐藏卡。测试刻意打乱详情顺序。 |
| P1-4 普通卡片 | 复用普通资料、段位、近期战绩、位置与匿名 playerRef 注册流程；排位按队列，乱斗无虚假段位；保留原英雄/队伍。名字可点总览，无新说明和标记。 |
| P1-5 重试与取消 | 结束后绝对 10/40/120 秒，最多三次；新 ChampSelect/活动对局、其他 gameId、断线取消，generation 与 context 阻止旧任务写回；phase 轮询也处理漏掉的活动阶段。资料 HTTP 支持 context 取消。 |
| P1-6 对局阶段 | InProgress/Reconnect 不加载赛后详情或身份，保留既有隐藏卡。 |
| P1-7 诊断 | live_roster_post_game_reveal 包含 game_id、queue_id、attempt、hidden_count、revealed、reason；覆盖工单七种 reason，不记录姓名/PUUID。 |
| P2 同步测试 | CameraMode 开局 2、结束 0、LCU 2，差值 PATCH 仅一字段并 GET 核对 result=ok；R186 同步和 R198 下一局覆盖均覆盖。 |
| P2 诊断 | game_settings_sync 增加 camera_mode_before/after，保留原同步逻辑与 save 探测；无关值记 unknown。功能扩展见 R198。 |

## 自动验证

- 工单七项 Go 与额外重试中 gameId 变化、活动阶段/断线取消、队列段位、已有 PUUID、空姓名等边界通过；Node 4 项覆盖普通卡、R193 隐藏卡、事件桥接与 busy 请求尾随刷新。
- 三项 overlay 变异均由指定断言检出，见 [mutations.json](history/reports/r197/mutations.json)。没有以编译或语法失败冒充断言失败。
- 独立只读后端/前端复核完成；逐次 gameId 校验缺口已修正。赛后通知与正在读取的名单请求重叠时，使用已有尾随刷新队列，补充语义测试、真实 SSE 浏览器复测与独立复核均通过。
- 最终完整 Node 1125 项、1124 通过、1 项 Windows PowerShell 门禁在 macOS 跳过、0 失败（251.037 秒）；Go `go test ./backend -count=1` 243.758 秒通过；专项 race 3.723 秒、vet、diff check 通过。
- 完整本地 public 构建再次通过：1725 项 Go 分片、installer test/vet、Windows 安装壳、包内后端、密钥策略与运行时门禁。源码、Git 快照和安装包内后端指纹均为 `3a9d539dfb84`。
- GitHub 完整 CI 成功：全量 race 206.587 秒；Node 1125 项、1124 通过、1 跳过、0 失败（367.601 秒）；R100/R117 真实 Chromium、installer 与 embedded pool 门禁通过。Windows CI Go 238.048 秒、Web 856/856、desktop 269 项、267 通过、2 跳过、0 失败；校验和和产物上传均成功。
- 源码提交 `302ca108139353df23fa03e9d00132a6c3d59eea`，分支 `codex/release-0.12.61`。[完整 CI 37109826915](https://github.com/LLYY0418/Deep-Legends/actions/runs/37109826915)、[Windows public 发布构建 37109827925](https://github.com/LLYY0418/Deep-Legends/actions/runs/37109827925)。后续账本提交不移动版本标签。
- 汇总见 [verification.json](history/reports/r197/verification.json)。

## 演示截图

实际生产 HTML/CSS/JS + Go 专项生成的匿名合成响应。真实本地 SSE → app 桥接 → 赛后事件 → loadLive → 完整卡片更新 → 点击名字走总览请求；主代理已查看补全前后截图，不代表 Windows 真实对局验收。

补全前：![补全前对方卡片](history/reports/r197/before-opponents.png)

补全后：![补全后对方卡片](history/reports/r197/after-opponents.png)

## 发布与真机边界

Windows public 发布构建成功：全量 Go 319.256 秒；Web 856/856；desktop 269 项、267 通过、2 项平台门禁跳过、0 失败；发布运行时 4/4，public 密钥/包内后端/指纹校验通过。草稿 id `402418510`，三个附件回下载 SHA256/size/digest/清单均通过，安装包 111,362,560 字节，SHA256 `d5430f8837aed465ebe6575ab1972fafb68d3529bc856e760b38b09b6763a71b`。旧六个 Release 的元数据与附件保持不变，包含 0.12.60 草稿；独立产物复核通过。证据见 [草稿核验](history/reports/r198/draft-verification.json)。

已正式发布 [v0.12.61 Latest](https://github.com/LLYY0418/Deep-Legends/releases/tag/v0.12.61)，release id `402418510`，发布时间 `2026-10-03T08:57:22Z`（北京时间 17:57:22），draft=false、prerelease=false、**isLatest=true**。匿名 `releases/latest/download/latest.json` 得到 **0.12.61**，并与回下载清单字节一致；标签指向上述验证提交，三个正式附件 digest/size/下载 URL 和正文核对通过。包含 R196 更新，0.12.60 草稿与其他旧 Release 元数据、附件均未改变。证据见 [正式发布核验](history/reports/r197/published-release.json)。

标签自动触发的同源码 CI `37111418872` 保留自动执行；验收采用同源码已全部成功的 `37109826915`。标签触发的重复草稿构建 `37111418886` 已确认 cancelled，不改已发布附件。

仍需用户 Windows 真机验收：隐藏玩家出现的对局结束后停在对局页 1～2 分钟，卡片补全；进行中仍隐藏。导出 live_roster_post_game_reveal 和 game_settings_sync 日志。R197 保留进行中。
