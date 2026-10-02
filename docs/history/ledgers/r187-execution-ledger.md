# R187 执行账本

日期：2026-10-02。基线：R186 的 0.12.51 工作区；本轮版本：0.12.52。用户授权执行 R187，本轮未发布 GitHub Release。

## 实现

| 工单项 | 实现与核对 |
|---|---|
| P1 | `renderLivePlayer` 仅在 ChampSelect 敌方占位分支显示“暂无玩家信息，进入游戏后显示”。占用图标右侧剩余宽度，通常一行；窄宽度允许换行，不使用省略号。InProgress 隐藏玩家分支保留原样。没有新增图标、tooltip 或其他 UI 文案。 |
| P2-1 缓存 | `rankScoreCache.invalidatePlayer` 按完整玩家引用后缀删除所有来源、服务器与作用域，包含普通、tier-only 和负缓存；其他玩家保留。缓存删除、flight 失效与结果发布由同一锁保护。旧 flight 取消上游并立即唤醒等待者，等待者重读新 flight；旧读取即使迟到，也不能写回缓存、返回旧值或删除新 flight。 |
| P2-1 时机 | 主程序将 LP 捕获完成回调接到当前玩家缓存失效。`capture` 入口注册 defer，记录、跳过、超时及提前返回均清理；gameflow 首次结束阶段及进入 EndOfGame 时清理所有队列，不限定排位，也覆盖 WaitingForStats 后再次进入 EndOfGame。 |
| P2-2 | `observe` 写入前按同账号同队列比较 wins + losses；小于当前基线时拒绝写入，并记录固定字段 `lp_snapshot_rejected stage=observe reason=stale_regression`。同队列 60 秒去重，不记录玩家引用、账号 hash 或队列值。场次相同但分数改变、场次增加仍正常写入。捕获 pending 保护保留。 |
| P2-3 | 总览 force 参数传入排位读取：失效旧缓存后跳过缓存读取，上游新结果仍原子写回缓存；正常读取、来源隔离、负缓存 TTL 与请求合并保留。 |

## 测试覆盖

`backend/r187_test.go` 覆盖工单 Go 六项及并发和完成分支：

1. LCU / SGP / Riot / unknown 的普通与 tier-only 条目均失效，其他玩家条目保留；旧等待者唤醒、旧上游取消、旧完成不干扰新 flight。
2. 延迟旧上游返回期间，等待者已取得新值；旧 leader 最终也取得新值，替代读取只发出一次。
3. W10 L10 S50 开局 → W11 L10 S67 结算记录 +17 → 旧 observe 被拒绝，基线保持新值；重复诊断去重，60 秒后可再记录。
4. 同场次调整、场次增加照常写入。
5. InProgress → EndOfGame → 真实总览处理链读取新 ranks，上游计数增加一次。
6. force 总览读取上游并写回缓存。
7. record / skipped / timeout / session_failed / unranked 捕获结束均执行失效回调；EndOfGame 在已结束阶段之后仍清理多来源与 tier-only。
8. 独立复现无开局基线的下一局：旧 observe 不倒退，下一局 games_gap=1，负局 -18 正常记录。

`backend/web/r179.test.cjs` 覆盖完整占位文案、换行与无省略样式，以及 InProgress 保持“隐藏玩家”且不出现新文案。

### 变异（实际 overlay 执行，均为预期 FAIL）

| 变异 | 实际失败断言 |
|---|---|
| 不执行 app 的当前玩家缓存失效 | `TestR187EndOfGameOverviewReadsFreshRank` 读到旧胜场 / 胜点而失败。 |
| observe 不比较场次 | `TestR187ObserveRejectsPostCaptureRegressionAndFallbackNextGame` 基线倒退失败；`TestR187FallbackWithoutStartKeepsLatestQueueBaseline` 下一局丢失失败。两项独立测试均 FAIL。 |
| 只删除 LCU 来源 | `TestR187InvalidatePlayerAllSourcesScopesAndFlights` 检出 SGP 旧键残留。 |
| 恢复旧占位文案 | ChampSelect 完整文案行为断言 ERR_ASSERTION。 |

脚本 `/private/tmp/r187-mutants.py`；Go overlay、Node 临时源码与失败日志 `/private/tmp/r187-mutants/`。生产源码未改回错误实现。

### 验证记录

- Go 排位 / LP 专项通过；R187 与排位缓存 race 通过（2.142 秒）。
- `go test ./backend -count=1` 全量通过（217.541 秒），`go vet ./backend` 通过。
- 两个只读探子独立核验多来源缓存失效及旧 flight 竞态、捕获时机与基线防倒退，未发现实现缺陷。
- Node 初轮 1061 项：1057 通过、3 失败、1 平台条件跳过。失败是旧源码护栏：R15 直接等待返回、R63 总览调用与负缓存发布位置、R186 NSIS customHeader 嵌套宏。已同步断言和变异目标，继续检查等待者先处理 invalidated、总览经缓存管线、负结果原子发布、NSIS 函数在插件目录注册之后展开；4 项定向护栏复测通过。
- `node --test backend/web/*.test.cjs desktop/*.test.cjs` 最终全量 1061 项：1060 通过、1 平台条件跳过、0 失败（241.533 秒）。日志 `/private/tmp/r187-full-node-final.log`。
- `git diff --check` 通过。未改动既有真机原始日志目录。

## 构建产物

完整 `DEEP_LEGENDS_KEY_MODE=public ./build-desktop.sh` 成功。构建内 1663 项 Go 测试五分片、后端 / installer 测试与 vet、Windows 后端交叉构建、NSIS installer / uninstaller、外层安装器、包内 runtime、指纹与 public 收据全部通过。日志 `/private/tmp/r187-public-build.log`。

- 版本：**0.12.52**；源码、后端与包内指纹一致：**22a9cc9d8076**。
- key mode：**public**，未嵌入 Riot Key；最终安装包仅保留明确的 `-public` 名称。
- 安装包：`dist/desktop/Deep Legends Setup 0.12.52-public.exe`。
- SHA-256：`833b594c3d881f50f370c1a618f8d281dbb777462b8d3e4902c0acf148e0eddc`，与 `release-build.json` / `SHA256SUMS-public.txt` 一致。
- package / lock、AGENTS / CLAUDE、CHANGELOG 与工单索引均同步 0.12.52；R182 索引注明“赛后旧缓存回写基线由 R187 修复”。

## 真机验收边界

本机没有 Windows / 真实游戏客户端，模拟测试与交叉构建不替代真机验收。用户待验：选人敌方占位完整文案；排位结算回大厅立刻看到赛后段位胜点；新日志不再出现旧 observe 退回胜负场基线。

- **R182 负局记录仍未有真机样本**。本轮新增的模拟负局记录只验证本地代码路径。
- **R185 P2 的 `renderer_perf` / `desktop_process_metrics` 在 0.12.49 日志里没有出现，需要 0.12.50 以后的日志确认**。本轮不把旧日志中不存在的埋点当作已验证。
