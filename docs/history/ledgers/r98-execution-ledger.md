# R98 执行账本

> 证据已于 R135 移出工作区，见提交 `b62bca1b9671bccd8c3cf9f7079096d4b33a6fa0`。

执行日期：2026-09-16。范围：`WORKLIST-R98-KR-REFRESH-CHURN-AND-SLOW-LOADING.md`。保留 R95/R96/R97 既有工作和用户 NSIS 配置；未提交、未推送。

## 逐项落实

### P0：刷新顺序和失败回滚

- `web/gameplay.js`：已有列表收到 progress 时按 gameId 原位更新，不移动已有条目；完整帧才提交新增 ID。冷首屏使用后端累计预览，append 使用固定的已提交基底与累计新页，避免多次预览反复拼接。
- **不采用 `begIndex + 预览数组下标` 推断位置**：后端会跳过未完成的空洞，压缩后下标不是真实页内位置。保留已知顺序、未知新 ID 延至 complete，是工单两种方案的组合；没有添加全量排序。
- 每次普通刷新保存 matches/pagination 快照，预览后错误或取消还原快照。新刷新先还原上次未提交的预览，防止其成为新基线。
- 保留 requestToken/controller 防护；旧响应和旧 finally 不会覆盖新请求。后端 `details[index] = detail` 保持不变。
- 真 Chromium/CDP 脚本 `desktop/r98-browser.cjs` 驱动生产 `loadOverview`、NDJSON 和真实 DOM：20 条 → 乱序子集 `[13,4,18,2,11]` → 完整 20 条，RAF/MutationObserver 连续采样核对顺序，并检查非零行高与递增 top；独立验证值回滚、连续刷新旧响应隔离、额度提示及 Chovy 职业分组。

[浏览器结果](/Users/ly/personal/personal-work/deep-legends/docs/r98-validation/browser/result.json) · [连续采样](/Users/ly/personal/personal-work/deep-legends/docs/r98-validation/browser/order-samples.json) · [预览截图](/Users/ly/personal/personal-work/deep-legends/docs/r98-validation/browser/sparse-preview.png) · [回滚截图](/Users/ly/personal/personal-work/deep-legends/docs/r98-validation/browser/error-rollback.png)

### P1 #1：限流预算与提示

- 总览使用 5 秒 queue budget；若下一可用额度已经超出预算，立即返回 typed 429 和 RetryAfter，不等待整个恢复窗口。
- 新 `riot_limiter_queue.go` 用 FIFO 队列串行发放额度。仅队首执行恢复等待，其余等待自己的 turn；取消直接移除，不提前占用令牌。24 等待者并发测试验证单一 sleeping head、取消不消耗配额、队列清理。
- 前端自动重试仍启用，并在有数据和空首屏两种情况下显示中性“额度恢复中，约 X 秒后自动重试”提示，已有战绩仍可浏览。
- 保留 15/秒、90/2分钟安全余量，未上调限额。工单 `90/25=1.8` 是算术错误，实际为 3.6 次全冷查询/2分钟（暂不计其它共享消费者）。
- 本地预算错误兼容 `errors.Is(err, errThrottled)` 和 `errors.As(..., *riotStatusError)`。额外修复上游 429 的 Retry-After 等待被预算打断时丢失 typed 429 的路径，并增加真实 HTTP fixture 测试。

### P1 #2：三个接口缓存

- `matchIDsFiltered`：60 秒；键包含 PUUID、start/count、queue、type。
- `leagueEntries`：3 分钟；键包含 PUUID。
- `topMasteries`：30 分钟；键包含 PUUID 和 count。
- 均复用 `cachedPublicIdentity`（内存/磁盘缓存及单飞），不改变 key/身份隐私边界。测试验证重复请求、不同玩家/分页/过滤参数隔离、TTL 和过期重新请求。
- **缓存不减少全冷查询的 25 次请求**，减少的是相同参数的重复请求；不能将工单“25→22”理解为首次冷加载必然节省三个请求。

### P1 #3 / #4：关键路径和连续预览

- summoner 移入 profileWait，详情只等待 IDs，不再等待头像/等级。预览不读取仍由 goroutine 写入的 summoner；最终响应等待元数据完成，保留原 summoner 错误及“不在韩服”404 语义。
- 首次至少 5 条后推送，之后每新增至少 2 条或全部完成时推累计预览，移除一次性闩锁。测试阻塞 summoner，验证它完成前已收到 5 条，随后收到 7、8 条，最后等级完整。
- `matches_loaded` 在检查取消前赋值，已完成的详情不会再被取消诊断误记为 0。

### P1 #5（可选）：诊断采样

- `pro_identity_match` 加入现有 noisy event 采样名单，保留首条及后续采样，不删除 none 语义，不改职业识别函数。
- 可选的索引 memoization 未做：只按 fetchedAt 缓存可能遗漏同时间戳的补充账号变化；本轮采用低风险采样，不扩张缓存失效范围。

### P3：R97 完整链路验证

- `TestR98ChovyNullRegionFlightToOverview` 构造真实 Next Flight 包装、账号级 `region:null`，经过 parseOPGGProPlayers → proIdentitySnapshot/normalizeProAccount → publicizeOverviewReferences。
- 保留 R97 六队/33 名已核对选手白名单。每名 8 个**合成测试账号**，264 个候选；断言 허거덩#0303 靠 PUUID 命中 GEN Chovy，诊断 matched_by=puuid，公开 JSON 无原始 PUUID。
- Go 导出的公开 overview 供真浏览器消费；从普通玩家页签收到后端身份后自动转入 pro 组、显示 GEN Chovy，而非目录点击预填职业上下文。
- 264 仅是用于检测账号池坍缩的 fixture 数量，**不是实时 OP.GG 候选数**。没有扩大白名单或重新采集真实账号。

[公开测试响应](/Users/ly/personal/personal-work/deep-legends/docs/r98-validation/chovy-overview.json) · [职业分组截图](/Users/ly/personal/personal-work/deep-legends/docs/r98-validation/browser/chovy-search-promotion.png)

## 实测前后对比

同一 `TestR98Timing`，真实本地 HTTP 服务器和实际 Go 并发/限流/缓存。上游响应为受控 fixture，分别延迟 account 40ms、summoner 450ms、IDs 90ms、ranks/mastery 80ms、详情 30ms。冷查询后用同一 provider 重复刷新；不是估算，也**不代表真实 Riot 网络延迟**。

| 指标（毫秒） | 冷查询：改前 → 改后 | 重复刷新：改前 → 改后 |
|---|---|---|
| 墙钟 | 1612 → 1309 | 173 → 0（不足 1ms） |
| 首次预览 | 591 → 240 | 91 → 0 |
| limiter_queue_ms（所有 goroutine 求和） | 6750 → 7340 | 0 → 0 |
| account | 53 → 53 | 0 → 0 |
| summoner | 460 → 459 | 0 → 0 |
| matchIDs | 92 → 107 | 91 → 0 |
| ranks | 82 → 97 | 81 → 0 |
| mastery | 81 → 113 | 81 → 0 |
| details | 1097 → 1148 | 0 → 0 |
| 本次 HTTP 请求数 | 25 → 25 | 3 → 0 |

- 冷查询详情起点从 513ms 提前到 160ms；预览从一次 5 条变为 5/7/9/11/13/15/17/19/20 条。
- **队列累计耗时没有下降**：worker 更早进入共享额度队列，累计等待增加；它不是墙钟。这里的收益是更早首屏、FIFO 消除惊群、长队快速提示、缓存免去重复网络请求，不能宣称所有指标均下降。
- 全部 phase spans 和成本计数：[改前 JSON](/Users/ly/personal/personal-work/deep-legends/docs/r98-validation/timing-before.json)、[改后 JSON](/Users/ly/personal/personal-work/deep-legends/docs/r98-validation/timing-after.json)。基线源码在同目录 before-*.txt，原始执行输出为 timing-before.txt 与 go-focused.txt。

## 变异验证

最终代码 **8/8 KILLED**。`scripts/r98-mutation-check.py`：每项先运行原版，随后以临时 Go overlay 或 JS 源码环境变量注入变异，不覆盖工作区生产文件；编译失败/超时不计 kill。

1. matchIDs 缓存 TTL=0。
2. ranks 缓存 TTL=0。
3. mastery 缓存 TTL=0。
4. 删除 overview 5 秒 queue budget。
5. 删除 FIFO gate。
6. 恢复一次性 preview 闩锁。
7. 恢复刷新预览前插（真 Chromium）。
8. 删除刷新失败快照回滚（真 Chromium）。

[逐项矩阵](/Users/ly/personal/personal-work/deep-legends/docs/r98-validation/mutations/mutations.json) · [完整 runner 输出](/Users/ly/personal/personal-work/deep-legends/docs/r98-validation/mutation-run.txt)。各项原版/变异完整输出及浏览器截图保存在 mutations 子目录。

## 全量验证与回归处理

所有 Go 命令使用 `GOCACHE=/tmp/deep-legends-go-cache GOTMPDIR=/tmp/deep-legends-go-tmp`。最终结果：

| 命令 | 真实结果 | 完整日志 |
|---|---|---|
| `go build .` | exit 0 | [go-build.txt](/Users/ly/personal/personal-work/deep-legends/docs/r98-validation/go-build.txt) |
| `go vet .` | exit 0 | [go-vet.txt](/Users/ly/personal/personal-work/deep-legends/docs/r98-validation/go-vet.txt) |
| `go test -count=1 -v .` | PASS，93.783s；1158 个顶层测试通过，17 个既有 opt-in 测试跳过 | [go-test.txt](/Users/ly/personal/personal-work/deep-legends/docs/r98-validation/go-test.txt) |
| `go test -race ./...` | PASS，126.729s，无 DATA RACE | [go-race.txt](/Users/ly/personal/personal-work/deep-legends/docs/r98-validation/go-race.txt) |
| `node --test web/*.test.cjs desktop/*.test.cjs` | PASS，697 tests / 696 pass / 0 fail / 1 skip，236.071s | [node-test.txt](/Users/ly/personal/personal-work/deep-legends/docs/r98-validation/node-test.txt) |
| `node desktop/r98-browser.cjs` | PASS，67 次中间采样，无乱序；回滚、旧响应隔离、额度提示、职业分组全部通过 | [browser-run.txt](/Users/ly/personal/personal-work/deep-legends/docs/r98-validation/browser-run.txt) |
| `python3 scripts/r98-mutation-check.py` | 8/8 KILLED，每项原版 exit 0，变异断言失败 | [mutation-run.txt](/Users/ly/personal/personal-work/deep-legends/docs/r98-validation/mutation-run.txt) |

Node 唯一 skip 为现有 Windows/PowerShell release 测试（当前 macOS）；没有增加 skip。`node --check web/gameplay.js`、`node --check desktop/r98-browser.cjs`、`git diff --check` 均通过。

[最终定向测试](/Users/ly/personal/personal-work/deep-legends/docs/r98-validation/go-final-focused.txt) · [前端兼容测试](/Users/ly/personal/personal-work/deep-legends/docs/r98-validation/node-compatibility.txt) · [范围不变量与源码 SHA256](/Users/ly/personal/personal-work/deep-legends/docs/r98-validation/invariants.json)。

- 初轮 Go 暴露旧 specialist sentinel 兼容性，修生产代码，不修改原断言。新增上游 429 同类路径测试。
- 一次全量 Go 在既有 `TestGameplayOverviewOverlapsIndependentUpstreams` 的 TempDir 清理时报 season-stats/sgp 目录非空；保留 `go-test-tempdir-cleanup-failure.txt`，未修改业务代码、未删除断言，停止并行 Go 检查后完整重跑。
- 既有 `TestR91AddendumOneRequestStreamsFiveThenTwenty` 原来假设仅有一帧 progress；按新累计流语义允许中间 progress，保留首帧5条、最终20条和1次IDs/20次详情请求断言。
- 初轮 Node 的 5 个失败来自旧 `functionSource` 不跳过注释，将新增注释中的英文撇号误读为字符串；仅改写注释，5 项原测试已真实通过。
- 独立核验提出的 nil tracker 疑点已排除：recordRateLimit 自带 nil receiver guard；缓存 stale 疑点亦未成立：该命名空间 staleFor=0，StaleUntil 与 ExpiresAt 相同，不存在正常过期后继续 stale 成功的窗口。没有为猜测修改公共缓存。
- 工单提到的用户原始日志/录屏未附在本轮可用文件中；本文不声称重放了它们。按工单以 fixture + 真 Chromium 验收，不要求真实 LOL 客户端。
