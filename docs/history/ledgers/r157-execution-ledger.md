# R157 执行账本

日期：2026-09-25。基线版本 0.12.21，工作区已有 R144–R156 未提交改动，未回退。产品版本递增至 0.12.22；未打安装包。

## 逐项结果

| 项 | 实施 | 本机证据 |
|---|---|---|
| P1 | 仅给 `loadCommunityDragonAugments` 的 Cherry 主目录和 Arena 补充目录加各自 45 秒失败退避。两跳共用 4 秒总预算；真正的请求超时或无 HTTP 响应才启动退避。调用方取消、明确 HTTP 4xx/5xx 和成功均不启动退避。成功缓存仍可命中，Arena 补充目录失败仍返回 Cherry 基础目录。诊断 `community_dragon_augments_backoff` 记录开始、跳过、结束、时间和跳过次数；短路不会伪装成一次 `champion_upstream` 网络请求。 | 假时钟验证首次失败、快速短路、到期重试与成功恢复；EOF 无响应触发退避，调用方自身超时不会误封主机；404/503 明确响应仍可立即再次探测；补充目录失败不丢基础目录；Hexdata、Data Dragon、OP.GG 主机仍可请求。持续超时下，斗魂第二次详情少于 2 秒，目录网络请求总数为 1。移除退避的隔离变异导致请求数断言失败。 |
| P2 | 海斗一次详情内首次与 RSC 合并后的两次装饰共用同一目录索引；斗魂 YOUR.GG 映射失败后将已取到的目录或错误传给 OP.GG 兜底分支，保留原兜底行、说明和诊断。 | 503 场景下两种详情各只发一次 Cherry 目录请求；测试强制海斗 RSC 成功，确实执行第二次装饰。分别恢复两模式重复调用的隔离变异均多出一次请求并失败。 |
| P3 | 保留直播推荐的 15 秒总预算；4 秒增益目录预算避免首次探测耗尽它，后续退避使装饰快速完成。 | 模拟 CommunityDragon 持续到请求上下文超时：首个海斗详情在 15 秒内返回，随后手动 `/api/champions/detail` 在 2 秒内返回 200；`mayhem_detail_phases_ms.total` 未触及 15000ms，第二次 `decorate` 小于 1000ms。斗魂首个详情少于 10 秒，退避后的第二次少于 2 秒且保留兜底数据。去掉增益目录预算的变异再次耗满 15 秒。 |

## 诊断依据与边界

- 工单引用的 `372f2c9a-lol-loot-diagnostics-0925-1724.jsonl` 原件未在当前工作区发现，未擅自生成或归档原始诊断文件。工单所述 61 次 CommunityDragon 请求、27 次失败及 15000–15013ms 聚集，只能作为工单提供的证据摘要，本轮无法独立重算。
- 4 秒是增益目录两跳的总预算，仅限这两个 JSON 资源；其他 CommunityDragon 资产、Hexdata、Data Dragon 和 OP.GG 的超时策略未改。45 秒期间会跳过未缓存的目录网络请求，到期后才有机会探测主机是否恢复；在收到实际 HTTP 响应前，程序不能提前得知网络已恢复。
- 真机网络与 Windows 客户端未在本机模拟；真实 `decorate` 时长和新的日志分布需下一份诊断复核。测试使用可控 HTTP transport，不宣称已在真实上游复现。

## 验证记录

- `go test ./backend -run 'TestR157' -count=1`：通过。
- `go test -race ./backend -run 'TestR157|TestLoadCommunityDragonAugmentsSupplementsArenaMetadata' -count=1`：补强 RSC fixture、EOF/调用方超时和短路日志核对后通过；随后新增斗魂持续超时整链用例，再运行 `go test -race ./backend -run 'TestR157' -count=1` 通过。
- 隔离副本对抗变异：P1 去退避、P2 海斗去复用、P2 斗魂去复用、P3 去 4 秒预算，四项对应断言均按预期失败；主工作区未变异。
- `go build -o /private/tmp/deep-legends-r157 -ldflags '-X main.version=0.12.22' ./backend`：最后日志调整后重新构建通过；`-self-test` 输出“Deep Legends 0.12.22 自检通过”。未打安装包。
- `go vet ./...`：最后日志调整后重新通过。
- `go test ./... -count=1`：在最终实现和新增斗魂整链用例后完整复跑通过，backend 207.891 秒。
- Node 首轮全量触发一条旧源码形状护栏：旧断言要求海斗函数里保留 `byID := gameplayAugmentIndexAll(catalog)` 的字面写法。已将护栏改为核对新的请求内索引构建及按 ID 查找；`node --test backend/web/champions.test.cjs` 242/242 通过。全量复跑 971 项，970 通过、1 跳过、0 失败，与 R156 基线持平。

## 待真机复核

1. 用新版本重复海斗和斗魂详情操作，导出新诊断；核对 `community_dragon_augments_backoff` 的 `started`、`skipped`、`ended` 及 `skipped_requests`，并核对同一阶段的 `champion_upstream` 主机失败次数。
2. 对照 `mayhem_detail_phases_ms` 的 `decorate` 与 `total`，确认反复切换英雄时不再稳定卡在 15000ms。若仍有个别请求接近 15 秒，再分析与增益目录无关的阶段；不得据此直接扩大本单改动范围。
