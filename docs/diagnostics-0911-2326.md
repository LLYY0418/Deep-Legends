# 2326 实测：查询对象被拒绝与本人阵容不完整是两件事

分析文件：`/Users/ly/Downloads/lol-loot-diagnostics-0911-2326.jsonl`。
最新运行版本 0.12.1，构建指纹 `6403b9734c23`；此文件包含前一份 2323 的记录及后续记录。
用户确认此次采集点击了一次“重试阵容查询”。请求数量不能直接当作点击次数：页面还会自动查询，手动诊断内部也有有界对照读取。

## 可复核证据

| 文件行号 | 观察 | 能说明什么 |
| --- | --- | --- |
| 16595–16596 | 正常 GSM 读取本人 200，`credential-14`；`IN_PROGRESS`，游戏 ID 有效、地图 11、队列 3110；双方条目 5/0、无效身份 4、重复身份 3、目标出现 1 次 | 已读到本人的在局游戏数据，不能再归因为“所有请求都鉴权失败”；但没有可验证的完整双方阵容 |
| 16607–16609 | 本人只读对照也是 200，同一凭据，前后 `InProgress`；旧结论 `self-only-schema-unverified` | 数据校验失败不等于 HTTP 请求失败。该次只有本人，没有与好友组成同一份对照 |
| 16661–16662 | 几秒后查询其他玩家 401，仍为 `credential-14`，没有 game 数据 | 好友请求在返回阵容之前就被拒绝，放宽阵容解析不能修复它 |
| 16988–16992 | 后续成对读取本人 404、目标 401；同一固定凭据，前后本机阶段 `None`；目标为同服、非私密 | 这份对照确实执行完了，但当时本人已不在局；不能抹掉前面本人 200 的证据，也不能把 404 称为阵容成功 |
| 16990 | 401 错误白名单 `UNAUTHORIZED`，message 仅脱敏分类为 `permission` | 响应提供了拒绝授权的证据；没有公开精确权限策略，不能断言所有国服账号永远只允许查询本人 |
| 16695、16977 | 历史 spectator GET 契约探测 405，错误分类 method-not-allowed | 历史只读请求在当前网关不可用；405 不说明换成 POST 就是安全的阵容读取契约 |

原始文件中的身份、凭据、未知错误原文不复制到报告或回归用例。`credential-14` 是进程内诊断别名，不是令牌值。

## 与 LeagueAkari 当前源码对照

核查时间：2026-09-11；以下为仓库 main 分支，后续版本可能变化。

- [实时对局调用方](https://github.com/LeagueAkari/LeagueAkari/blob/main/src/main/shards/ongoing-game/additional-info-controller.ts)：仅在本机 in-game 阶段取 `summoner.me.puuid`，传入 `gsm.getByPuuid` 补充本局信息。它不是任意好友阵容查询的成功示例。
- [GSM 请求封装](https://github.com/LeagueAkari/LeagueAkari/blob/main/src/shared/http-api-axios-helper/sgp/gsm.ts)：同样使用 GET `/gsm/v1/ledge/region/{server}/puuid/{puuid}` 和 league-session 凭据。已核对的路径、方法、凭据类型与本项目一致；这并非对其它未公开授权条件的保证。
- [GSM 响应模型](https://github.com/LeagueAkari/LeagueAkari/blob/main/src/shared/types/sgp/gsm.ts)：按 PUUID 的响应包含本人会话的游戏信息及连接凭据。`getByGameId` 的声明模型则包含终局时间、收益和终局统计；仅据该源码不能将其当成好友实时阵容备用接口，因此本次未新增调用。

结论分层：已经确认失败发生在“使用本机 league-session 凭据查询其他玩家 GSM 会话”这一步；结合 Akari 的实际调用对象，优先判断为查询对象授权范围/接口用途不匹配。尚不能证明国服不存在其它受支持的只读阵容服务，也不能承诺仅靠继续增加相同请求的日志就能恢复好友阵容。

## 本次修改

1. 修复手动刷新标记只在 request 事件携带的问题。后续 received、invalid-response、failed、canceled、stale 以及对应页面渲染继续沿用本次标记，不再默认误记为自动。
2. 对照日志增加 `game_payload_observed` 与 `game_payload_scope_comparison_ready`，保留原有严格的 `roster_scope_comparison_ready`。前者要求 HTTP 200、成功解析、有效游戏 ID/支持地图、进行中阶段、目标恰好出现一次；后者还要求同一份双对象对照、阶段稳定和本人在局。不会把 HTML 200、目标不匹配或 404 当作有效游戏数据。
3. 阵容形状日志增加固定校验失败原因，明确本人这局的空队和身份问题；不猜测缺失身份是不是人机，也不伪造对方成员。
4. 更正文档中“同服玩家已经可以通过 GSM 查询阵容”的过度表述。mock 返回十人阵容只能验证解析流程，不证明实际网关授权可用。

本次没有增加请求数量、变更凭据来源、改用未知 POST、启动观战、放宽身份校验或往页面/缓存写入诊断数据。好友完整阵容仍未取得，不能将以上诊断修正表述为功能已修复。现有日志足够做上述判断，不要求用户为了重复证明 401 再开一局或再点一次。

## 验证

- `go test ./... -count=1`：通过。
- `go test -race ./... -run 'TestAccessDiagnostics|TestCurrentGame2245|TestFlowDiagnostics' -count=1`：通过。
- `go vet ./...`、`git diff --check`：通过。
- `node --test web/gameplay.test.cjs`：38/38 通过；新增手动/自动标记断言先在旧代码失败，再在修复后通过。
- 独立复核确认没有新增网络请求或放宽生产校验。对照就绪字段允许目标 401 是有意比较“本人成功/目标拒绝”，不是目标授权成功；已补注释、说明及 200 HTML/403/404/500 回归。

以上为本地测试和既有日志分析，未打包部署 Windows 新版本，也没有新的国服好友完整阵容实测成功。
