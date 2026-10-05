# R221 外服客户端适配执行账本

日期：2026-10-05。基线：0.12.71；`desktop/package.json` 保持 0.12.71。
对应工单：[外服客户端全面适配](WORKLIST-R221-RIOT-REGION-CLIENT-SUPPORT-JP-OVERVIEW-HISTORY-REGION-GROUPING-AND-PRACTICE-TOOL-RUNES.md)。

本账本原使用 `r220-region` 命名，因与同号“清理原始数据和 v3 残留”工单重号，已于 2026-10-05 改为 R221（工单、账本、验证记录目录 `history/reports/r221/`）；清理工单仍用 `docs/r220-execution-ledger.md` 与 `history/reports/r220/`。代码测试名（`TestR220*`、`r220-region.test.cjs`）仍沿用 R220 前缀，未改动，避免破坏已记录的验证命令。

## 执行范围及约束

按 P1–P6 实施外服路由、战绩读取、页签与搜索、对局数据和练习工具推荐。继续遵循用户此前“不构建、不发布”的要求：不构建应用、不打包、不更新版本号、不发布、不部署生产 Worker。

测试 key mode：仅虚拟 fixture Key 与 mock HTTP / Worker / Cache / limiter；不读取 Cloudflare Secret，不用真实玩家账号验证 Riot API，不产生 private/public 安装包。官方路由核对只读取公开文档。

## P1 客户端平台

- `LCUClient.platformInfo()` 支持启动参数与命令行查询；仅拿到 region、没有平台时仍执行查询补齐。
- `/api/status` 返回 `clientRegion` / `clientRegionLabel`：腾讯为 TENCENT，拳头为已收录的平台小写，未知留空。
- 每次连接启动记录 `client_platform_resolved`；数据字段仅 region、platform、source，所有参数均归入枚举白名单，不记录命令行、路径或凭据。

## P2 Riot / 中转路由与额度

- 收录 BR1、EUN1、EUW1、JP1、KR、LA1、LA2、ME1、NA1、OC1、RU、SG2、TR1、TW2、VN2。
- Match-V5 按工单的四个区域集群映射；Account-V1 经官方核对只有 americas / asia / europe，SEA 平台的账号解析改用 asia，Match 仍用 sea。
- 官方 API Reference 当前包含 ME1；旧静态 LoL 路由表仍列 PH2 / TH2 且遗漏 ME1。核对记录：[route-verification.json](history/reports/r221/route-verification.json)。该核对证明平台与入口清单及 Account 差异，未用真实 Key 探测各区接口。
- 普通查询从平台选择 provider、主机和 OP.GG 对应地区；旧 KR / ASIA 主机常量只保留在旧测试夹具。根 provider 的 KR 选择仅供明确固定韩服的职业 / 绝活哥目录。
- 内存 identity / flight / Personal-Key 限流按平台拆分；identity 磁盘键含平台；比赛键带规范 Match ID 平台前缀；OP.GG 页面与补充缓存、平均段位长期缓存、标签和时间线同样隔离。
- 旧韩服平均段位缓存保留兼容键；新平台使用独立键。职业 seed 的早期段位观察写入同一新 identity 命名空间，保持原有未定级退避行为。
- relay 与 Worker 支持所有平台及集群的合法接口族，补齐完整熟练度 / 总分接口。Account SEA 路径拒绝；任意主机、平台 / 集群错配和非法路径拒绝。
- 软件发送白名单平台枚举 `X-Riot-Platform`，Worker 不向 Riot 转发该头；响应缓存、application / method / service 冷却均带平台。删除客户端全局 application 冷却，JP 429 不锁 KR；共享 IP 限流与 Cloudflare 日额度仍全局生效。
- **生产 Worker 重部署待发布阶段执行。** 本地 15 平台代码通过测试，不代表生产地址已经启用新路由。

## P3 本人总览

- 外服当前账号的战绩优先按客户端平台读 Riot Match-V5，再回退 LCU；整页详情失败时回退，不把缺详情的一页当成完整页面推进游标。
- 段位、熟练度、本人 LCU 赛季里程碑保留本机路径；赛季统计原有来源和能力边界保持原样。
- 外服 LCU 战绩的 500 / 503 / 超时 / 连接拒绝按 3、6、12 秒重试，最多四次，取消立即退出。记录 `lcu_history_retry`。
- 前端外服本人请求也发送 NDJSON Accept，允许服务端 180 秒预算、解除普通 30 秒 write deadline，并兼容服务端返回普通 JSON；不会让自动重试被旧短预算截断。
- identityReady 从 false 变 true 时重发本人总览；平台切换重新初始化本人页签。
- `match_history_data_source_decision` 带 client_region 和 attempts；Key 缺失、日额度、429、超时、404 使用具体且不含账号参数的失败原因。

## P4 页签、搜索和前端作用域

- 15 个 Riot 平台均归入名称仍为“韩服”的分组；JP1 显示日服，title 为“日服 (JP1)”。Riot 页签不依赖本机连接；外服本人断连后保留可读数据，并可按其 Riot ID 查询。
- 页签 / 匿名玩家引用、搜索、参与者链接、时间线、平均段位与 perk-stats 请求和缓存使用实际平台。
- 搜索菜单覆盖全部 15 平台，连接或切换客户端时默认选择当前平台；同平台后续状态刷新保留用户的手动选择。
- 明确固定韩服的职业目录、英雄高手榜、吃鸡高手榜和绝活哥跳转保留显式 KR，并补注释。
- 外服隐藏 ARAMKit、TCLS 国服纯净入口和腾讯 SGP 能力项；不增加方法论 / 说明文案。

## P5 外服对局

- 对局玩家的战绩按所在平台走 Riot → LCU；实时卡片只请求所需十场，单双排 / 灵活组排使用队列筛选，避免每人三十场详情的额外额度消耗。
- 其他玩家段位走对应平台 league-v4，失败可回退同一客户端的 LCU；本人段位保持 LCU。
- 点击 live 玩家时带当前客户端平台；进对应 Riot 分组，保留覆盖层交互。

## P6 练习工具

- PRACTICETOOL + map 11，包括 queue 3140，映射 ranked。
- 空 / NONE 位置归一为空，用 OP.GG 主位置；练习工具带惩戒也不把空位置先强行当打野。
- 本人卡片和当前位置使用解析后位置，绝活哥按同一 resolvedPosition 查询；没有主位置证据时不把纯 fallback 当已推断位置。
- 仅英雄选择阶段可应用的逻辑不变，不增加推断说明文字。

## 验证结果

最后一次 Go 源码及测试改动之后，重新运行全量 `go test ./backend -count=1`，退出码 0，测试报告耗时 **260.813 秒**。完成时间为 **2026-10-05 18:09:28 +08:00**（取原始日志落盘时间）。输出不足 5 行，完整输出如下：

```text
ok  	lol-loot-assistant/backend	260.813s
```

| 检查 | 结果 | 输出 |
| --- | --- | --- |
| `go test ./backend -count=1` | 通过，260.813 秒 | [go-test.log](history/reports/r221/go-test.log) |
| `go test -race ./backend -run 'TestR220\|TestR208\|TestRiotRate' -count=1` | 通过，2.288 秒 | [go-race.log](history/reports/r221/go-race.log) |
| `go vet ./backend` | 退出码 0，无输出 | [go-vet.log](history/reports/r221/go-vet.log) |
| `node --test --test-concurrency=2 backend/web/*.test.cjs` | 938/938 通过，无跳过，30.001 秒 | [frontend-test.log](history/reports/r221/frontend-test.log) |
| `node --test relay/riot-worker/worker.test.mjs` | 15/15 通过，无跳过，0.192 秒 | [worker-test.log](history/reports/r221/worker-test.log) |
| R220 / R208 / R206 / 推荐真实 HTTP 专项 | 通过，9.716 秒 | [targeted-test.log](history/reports/r221/targeted-test.log) |

并发测试命令中的 `|` 为正则分隔符。全部完整命令及退出状态见 [validation-summary.json](history/reports/r221/validation-summary.json)。`git diff --check` 通过。最终全量之后只整理文档和验证记录，没有再改源码。

初轮全量中发现的实际问题均已定位：身份键迁移影响职业早期段位观察、国服 live 缓存兼容键，以及旧测试把 NA1 视为非法平台；均修复后专项通过。独立复核促使中转响应缓存和 KR 冷却键也统一加入平台。本轮核验还补齐平均段位磁盘缓存的平台作用域及本人请求预算。

## 未执行项

- P2 的生产 Worker 重部署：受本轮“不发布”约束，后续发布阶段按 R206 流程执行，届时验证 15 平台生产路由。
- 构建及工单列出的 Windows 真机验证：遵循用户约束未执行；日服登录、刚登录 LCU 自动恢复、练习工具阿祈尔、对局玩家数据及切回国服需真实客户端验证。
- 工单保持进行中；不能把本地 mock 回归记成真实 Riot / Windows 已验证。

## 0.12.73 发布阶段补充（2026-10-05）

用户要求“发布新版本”后恢复构建和发布授权。生产 Worker 已部署为 `a56f37a5-95bf-4997-949f-4c1336a7ce4c`，15 平台公开状态接口均为 200 有效 JSON，SEA Account 与不支持路径均为 404；没有读取或替换既有 Secret。首轮 Python urllib 请求收到非 JSON 403，改用匿名 curl 重复同一公开 URL 后全部通过，保留两轮原始记录。此验证不替代真实玩家接口和 Windows 客户端验收。完整后续发布与安装升级证据见 [0.12.73 发布账本](history/ledgers/release-0.12.73-execution-ledger.md)。
