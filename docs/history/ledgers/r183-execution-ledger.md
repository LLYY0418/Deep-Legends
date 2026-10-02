# R183 执行账本

日期：2026-10-02。工单：WORKLIST-R183-RANKED-ROSTER-MISSING-ANONYMOUS-ENEMY.md。基线 R182 / 0.12.47，本轮版本 **0.12.48**；package.json 与 package-lock.json 两处版本同步。源码指纹 **14c6cc553556**。

## 范围与证据

- 以工单提供的 0.12.47 / a2e718b631f9 真机日志摘要为依据：queue 440，gameflow 9 人、Live Client 10 人且只有 9 个 Riot ID，十次恢复均 appended=0、共 40 次成功查询。没有取得原始 jsonl，本轮没有 Windows 真机排位。
- **R175 的补人逻辑这次是第一次在排位真机上触发，此前一直未验证。** 此前的合成测试不能替代真机验收；本轮也只完成代码与本机验证。
- R182 的 LP 修复及此前工作区修改均保留。前端生产源码没有修改，不增加说明文字或 tooltip。

## P1：排位匿名占位

- liveAnonymousRosterQueue 与 liveTenPlayerRosterQueue 共用队列判断，420、440 及海斗可补匿名占位，其他队列仍不允许。
- 原门禁全部保留：本人 PUUID/阵营/名称核验、Live Client 两队各 5、本人与 Live Client 阵营对应、服务器明确、具名成员成功解析、目标队人数一致、匿名数恰好等于余下缺口；保留重复 Riot ID/PUUID、阵营冲突、过量补入、取消等判断。
- 匿名占位增加 SelectedPosition，沿用 championIDForName 匹配已知英雄、NameVisibilityType=HIDDEN，不制造 PUUID、SummonerID 或名字。详情页归一化为 middle，positionSource=liveclient，既有分路排序生效。
- 沿用匿名安全链路：不查身份、战绩、段位；没有组队、小队标签；名字不可点击；对局中显示原“隐藏玩家”和“隐藏身份”标记、英雄头像。R179 选人占位语义不变。
- 合成复现：本人在 team 200，gameflow team 100 缺中路；440/420 都补一个匿名中路，最终 5/5。负例覆盖匿名 2 缺 1、具名查询失败、队伍不是 5/5、本人身份或 Live Client 阵营无法核验，仍保留 partial / self-unverified / teams-unverified。

## P2：本局缓存

- 实际 recoverClassicLiveRoster 入口接收 gameId/queue/phase/rawCount scope；本局输入相同复用完整恢复结果，包括补入玩家、查询与补入计数、原因和失败/partial 结果。
- 指纹仅在内存：规范化排序 Live Client 阵营、Riot ID 与匿名槽位，并包含位置/英雄；gameflow 玩家与队伍组成集合；当前身份、匿名开关和英雄名称映射也进入键。换客户端或 gameId 重建缓存，离开对局与 ChampSelect 清空；新局即使已完整 10 人、没有调用恢复，也会清掉旧缓存。
- 并发同键共用 flight，不在锁内查询。发布校验当前局、客户端和 flight；旧局完成不能污染新局缓存或追加旧局诊断。取消不缓存结果；panic 释放等待者并删除本次 entry，允许重试。返回玩家切片复制，调用者不能修改缓存。
- 十次实际 loadGameplayLive 加载：每次最终均 10 人，Riot ID 查询总数 **4**。孤立缓存测试也验证后九次 Stats.Cached=true；包括 partial 的十次输入只查四次。
- 覆盖：输入顺序变化命中；gameflow 玩家集合变化重算；Live Client 身份变化重算；换局清空；gameflow 已补齐人不二次追加；两路并发只查四次；旧局在途完成后新局仍命中。

## P3：计数与诊断去重

- live_roster_recovery 新增 anonymous_count、named_present、named_appended、unresolved_named、cached，不写 Riot ID、PUUID、英雄名称或位置。
- 本局复现场景：anonymous_count=1、named_present=4、named_appended=0、unresolved_named=0、appended=1、alias_attempts=4、reason=anonymous-placeholder。
- 同一缓存输入只记录一条，首次计算 cached=false；后续返回的缓存结果 Stats.Cached=true，不重复追加日志。这同时保留工单“十次加载只记一条”与缓存命中标记的断言。输入改变重新计算并记录；换局重新开始。

## 测试覆盖

- backend/r183_test.go 覆盖工单 Go 1–8：440/420 缺中路、匿名数量门禁、失败具名门禁、阵营核验、真实加载请求计数/匿名组队排除、十次缓存、换局/输入改变、新诊断字段与隐私。额外覆盖结果切片隔离、partial 缓存、并发及旧 flight。
- R175 端到端表新增 queue 440 匿名队友，沿用九人身份/历史请求计数与无 panic 断言；海斗、具名与 partial 原用例保留。R163 的旧“排位禁匿名”断言更新为允许；显式 allowAnonymous=false 的禁止路径仍保留。
- backend/testdata/r183-ranked-anonymous-live.json 来自合成 LCU/Live Client 的实际后端返回，不含真实账号。Go 回归核对人数、队列、隐藏状态、位置、英雄和空 PlayerRef。
- backend/web/r183.test.cjs 读取该返回体，真实 renderLiveInsights / liveSnapshotComplete / renderLiveRefreshStatus 在 JSDOM 渲染：我方/对方各五卡、敌方第三张为隐藏中路、有英雄图、名字 disabled 且无 player-ref；页头没有“数据尚未完整”。另删去匿名行证明原不完整提示仍生效。440/420 都验证。
- 两路 default 子代理定位缓存生命周期与测试支架，另两路独立核验实现和匿名安全；未发现要求之外的功能变更。

## 4 项变异

使用 Go -overlay，生产文件不被替换。四项均为目标测试断言 FAIL，已逐份核对，无编译失败冒充检出。

| 变异 | 检出 |
|---|---|
| 匿名恢复改回仅海斗 | TestR183RankedAnonymousEnemy/440 和 /420：仍为 9 人、未补占位。 |
| 占位不带位置 | TestR183RankedAnonymousEnemy：SelectedPosition 为空，位置断言失败。 |
| 去掉缓存命中分支 | TestR183RecoveryCacheLifecycle：第二次 Cached=false。 |
| 去掉匿名数量恰好相等 | TestR183AnonymousSafetyGates/too-many-anonymous：错误补入匿名玩家。 |

源、overlay、清单与日志：/private/tmp/r183-mutants/{hextech-only,no-position,no-cache,no-anonymous-cardinality}。

## 自动门禁

- 专项 Go（R183、R163、R161、R175 端到端）：通过，21.728 秒；/private/tmp/r183-go-target.log。
- 前端专项：通过；/private/tmp/r183-web-target.log。
- 全量前端：**784/784 通过**，16.199 秒；/private/tmp/r183-web-full.log。
- 全量 Go：通过，214.534s；/private/tmp/r183-go-full.log。
- go vet ./backend：通过；/private/tmp/r183-vet.log。
- 缓存并发与跨局 flight 的 -race：通过，1.657 秒；/private/tmp/r183-race.log。
- git diff --check：代码、索引、版本与账本文档收尾后均通过。

## 构建与真机边界

- key mode=**public**；CGO_ENABLED=0，macOS arm64 与 Windows amd64；显式 main.version=0.12.48、main.buildFingerprint=14c6cc553556、main.riotAPIKey=、main.riotAPIKeyCipher=。
- 产物：/private/tmp/Deep-Legends-backend-0.12.48-public 和 /private/tmp/Deep-Legends-backend-0.12.48-public.exe。两平台构建通过，均通过 verifyBuildFingerprint 与 verifyRiotKeyPolicy(..., "public")；最终源码指纹与两份产物一致。保持 -public 文件名，不生成或发布安装包。
- macOS 自检使用空 RIOT_API_KEY 与隔离 /private/tmp/r183-selftest：通过，0.12.48、奖池 554 条、哈希 dee5e21f5234；/private/tmp/r183-selftest.log。

Windows 真机验收仍由用户执行：

1. 我方或对方有隐藏身份的缺席玩家时，两队各五人，显示“隐藏玩家”、英雄头像与正确位置。
2. 页头不再出现“数据尚未完整，已停止自动重试”。
3. 导出日志，确认 0.12.48 / 14c6cc553556，live_roster_recovery 的 reason=anonymous-placeholder、appended=1 与匿名/具名计数；同输入只有一条，输入变化可以多一条。
