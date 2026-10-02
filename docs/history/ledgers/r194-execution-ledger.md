# R194 执行账本

日期：2026-10-03。R193 基线已保存为 `2107435d`，版本由 0.12.57 递增至 **0.12.58**。

## 逐项实现

| 工单项 | 实现与验证 |
|---|---|
| 5v5 队列扩展 | `liveTenPlayerRosterQueue` 保留 420/440 特判，其余按注册表 ModeGroup solo / flex / match / aram / hextech-aram / clash / urf 放行。bots、doombots、arena、custom、other、nexus-blitz 及未知队列排除 |
| 匿名校验保持 | `liveAnonymousRosterQueue` 继续复用同一函数；本人身份/阵营、Live Client 两队各 5、具名成员精确查询成功、匿名人数等于缺口等 R183 校验不变 |
| 电脑玩家保险 | Live Client 解析保留 IsBot，无 Riot ID 的 IsBot 条目不进入匿名候选或计数。缓存键同时纳入 IsBot，防止复用旧匿名占位结果 |
| 匿名安全 | 延续 R175/R183：占位只有英雄、位置、HIDDEN，无 PUUID / PlayerRef，不查身份、历史、段位，不参与组队推断。R193 显示规则保持 |
| 跳过诊断 | InProgress / Reconnect、原名单不足 10 且队列不支持时记 `live_roster_recovery reason=queue-unsupported`。使用原有 queue_id 和计数字段，不增加字段。锁内认领每局一次标记，换局/客户端与清空恢复缓存时重置 |

前端完整性判断和重试调度也有旧队列白名单：仅改 Go 会让快速模式 9 人名单仍被浏览器视为完整、停止刷新。同步两处列表与后端已登记的 5v5 PvP 队列，缺人时继续 5 秒重试；Node 对照 Go 注册表逐队列检查。不新增 UI 文案。

## 专项测试

`backend/r194_test.go` 覆盖全部 Go 要求：

1. queue 480，我方 4 / 对方 5，Live Client 5/5，我方 TOP 无 Riot ID。真实 loadGameplayLive 补成 5/5；anonymous-placeholder、appended=1、anonymous_count=1、named_present=4。占位 top、英雄 1、无身份/段位/组队字段；身份/历史/段位请求各 **9**，具名精确查询 **4**。Reconnect 复用结果，不重复查询或诊断。
2. 表驱动覆盖工单 true / false 队列，补齐已登记同组 ID。未知队列不推断。
3. queue 880 + isBot 的真实加载保持 9 人、不补人、精确查询 **0**；连续加载及 Reconnect 只记一次 queue-unsupported，换局及清空后可以再记。诊断仅含既有字段和通用日志元数据。
4. 独立 bot 保险：使用允许恢复的 queue 480，先补匿名人，再将原始 isBot 改为 true 并重新探测；不得补 bot 或复用旧缓存，anonymous_count=0。这样删除 isBot 防护的变异不会被 880 的队列拒绝门掩盖。
5. gameplayLiveSnapshotComplete 在 queue 480 九人时 false，补齐 5/5 后 true。

Go 专项（R194 + R183 + R163 + R161 + R175 端到端）通过，32.195 秒；日志 `/private/tmp/r194-go-target-final.log`。 两条完整性旧夹具调整后，R194 全部用例（包含 Reconnect）与 R113 / R88 再次专项通过，16.844 秒，日志 `/private/tmp/r194-go-fixtures-final.log`。R183 原 440/420、安全门、缓存用例全部保留并通过。

`R194_WRITE_FIXTURE=1 go test ./backend -run '^TestR194' -count=1` 从合成客户端响应经真实后端管线生成 `backend/testdata/r194-quickplay-anonymous-live.json`；账号全部为合成数据，不是手拼返回体或真机数据。

Node 专项 **19/19 通过**（R194 / R183 / R193 / R163 / R90），日志 `/private/tmp/r194-node-target-final.log`。读取上述 JSON，用实际 renderLiveInsights / renderLivePlayer / renderInsightMatches 渲染，两队各 5 卡，我方隐藏 TOP 第一张，英雄图标保留、名字禁用、无 player-ref；只有上路、提示一次，无段位、破折号或底部历史节点。九人 queue 480 按 5 秒重试，补齐后停止；重试达到上限时仍保留原提示。

R163 原“450 九人完整”断言按新要求改为 false。首轮 Go 全量和构建另发现 R113 / R88 的旧单人 450 夹具，与新的 10 人完整性要求冲突；将这两条测试夹具补成 5+5，保留历史失败/终态和整局缓存命中、不延长时间戳、老快照不重建等原断言。其他有意不完整的负例未改。

初次专项的一条诊断字段数量断言遗漏通用日志自动添加的 time / build_fingerprint / run_id / log_seq，改为核对既有字段集合；生产代码不因此改变。

## Overlay 变异

`python3 desktop/r194-mutants.py` 使用 Go -overlay，不替换生产文件。三项均由指定测试断言 FAIL 检出，无编译失败冒充：

| 变异 | 检出 |
|---|---|
| 恢复仅 420/440/海斗 | TestR194QuickplayAnonymousTopEndToEnd、TestR194TenPlayerQueues FAIL |
| bots 加进允许组 | TestR194TenPlayerQueues、TestR194BotsAndUnsupportedDiagnostic FAIL |
| 不排除 isBot | TestR194BotsAndUnsupportedDiagnostic/isBot-independent-gate-and-cache FAIL |

结果 `history/reports/r194/mutations.json`；源、overlay、完整日志 `/private/tmp/r194-mutants/`。

独立只读复核未发现缺陷；另一次只读扫描确认需调整的完整性旧夹具只有 R113 / R88，其他负例保留。race（R194 bot/诊断生命周期 + R183 并发/跨局 flight）通过，12.340 秒；go vet ./backend 通过。日志 `/private/tmp/r194-race.log`、`/private/tmp/r194-vet.log`。

## 全量与构建

`node --test backend/web/*.test.cjs desktop/*.test.cjs`：**1103 项，1102 通过、1 项 Windows/PowerShell 条件跳过、0 失败**，282.2 秒；日志 `/private/tmp/r194-node-full.log`。

最终 `go test ./backend -count=1`：**通过，244.420 秒**；日志 `/private/tmp/r194-go-full-final.log`。首轮因 R113 / R88 旧夹具失败，保留日志 `/private/tmp/r194-go-full.log`。

完整 `DEEP_LEGENDS_KEY_MODE=public ./build-desktop.sh`：**通过**，版本 **0.12.58**、key mode **public**。初轮构建因 R113 旧夹具失败停止，未产生新安装包；最终重新构建的 Go **1690 项**五分片全部通过（77.1 秒），backend vet、installer 全量与 vet、NSIS、打包运行时、指纹和收据校验全部通过。最终日志 `/private/tmp/r194-public-build-final.log`。

- 安装包：`dist/desktop/Deep Legends Setup 0.12.58-public.exe`。
- 指纹：`5230b7850e83`；最终重算源码、校验后端与打包运行时一致。
- 安装包 SHA256：`387fcfdd2d46666da3ed3fef707ba791027d7640cd85379ccad706260b169963`。
- 后端 SHA256：`120b48fec8c795147bd9b9f46936a01846933c58a82b90efa062aeac80ac7b16`。
- 安装包与后端 SHA256 均重新计算并与收据一致，收据与校验表归档 `history/reports/r194/release-build.json`、`SHA256SUMS-public.txt`。
- dist 仅保留明确的 -public 安装包，无与 private 同名产物。`git diff --check` 通过。

## Windows 待验

1. 安装新版本后，在快速模式、匹配或极地大乱斗遇到隐藏身份玩家，确认两队各 5 人，补入卡片显示隐藏玩家、英雄头像和正确位置，并遵守 R193 显示规则。
2. 导出日志，确认本局 `live_roster_recovery reason=anonymous-placeholder appended=1`。

本轮使用合成客户端响应；未取得工单提及的 0.12.49 原始 jsonl，不将摘要和合成验证写成 Windows 真机验收通过。
