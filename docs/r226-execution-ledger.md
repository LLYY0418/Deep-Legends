# R226 执行账本

日期：2026-10-05（北京时间）。基线及当前版本：**0.12.73**。工作区 HEAD `270fc3af`，保留已有 R222～R227 的并行改动。

工单：[WORKLIST-R226](WORKLIST-R226-R223-VERIFICATION-HIDDEN-LIVE-RENDER-THROWS-R70-GUARD-FAILS-AND-CI-WOULD-BLOCK.md)。状态：修复与本地自动验收完成。本轮没有提交、推送、触发 CI、更新版本号或发布。

## 修复与回归

- `backend/web/gameplay.js` 的 `renderLive` 首先检查隐藏/销毁状态，再设置 `liveRenderSource`、清空 `liveRenderTrigger`。真正渲染实时页时仍使用 R224 的触发来源逻辑。
- 原 R70 测试及依赖注入保持原样。新增 R226 用例只注入 `{ state }`，分别检查隐藏页、已销毁实时页；预置 trigger=`sse`、source=`manual`，调用后整个 state 必须保持原值。
- 修复前专项 **15 项 / 13 通过 / 2 失败**：原 R70 抛 ReferenceError；新增用例证实 trigger 被清空、source 被改写。修复后专项 **15/15 通过，5.927 秒**。没有注入缺失依赖来绕过护栏，也没有放宽断言。

## 完整 renderer 验收

命令：`R222_NODE_TIMING_OUTPUT=docs/history/reports/r226/node-timings-local.json node scripts/test-renderers.cjs all`。

结果：**1275 项，1271 通过，0 失败，4 跳过，78.352 秒，exit 0**；macOS，8 个文件 workers。最慢文件 `desktop/overview-render-requests.test.cjs` **42.842 秒**。全部文件 ≤90 秒，集合 ≤240 秒；门槛及 runner 并发配置未修改。本轮单独运行 renderer 集合，没有同时启动 Go 全量、Chromium 或构建。

本轮 R192 文件 **9.387 秒**、R206 图片队列文件 **0.808 秒**，全部断言通过；其中包含 R223 曾失败的两项挂载和 100ms 图片启动检查。原 R223 失败记录保留，后续确认写入其账本。

4 个跳过项：

1. `R82 A/B receipt lookup and deduplication execute the real PowerShell script`：本机无 `pwsh`，原因 `PowerShell is required; set R82_POWERSHELL to run the real script tests`。
2. `R82 A/B filename, hash mismatch and duplicate mutations reach failing assertions`：同上。
3. `R86 Windows release stops on a real failing Go test and rejects an independent early build`：`requires Windows and PowerShell`。
4. `R222 Windows release entry points reject CI skip locally before any side effect`：`requires Windows and PowerShell`。

这些平台用例仍在 Windows 验收流程中，未删除或扩大 skip 条件。

## CI 与耗时确认

实际读取前三轮 Linux quality 日志，并核对第四轮 CI 汇总。四轮的 R192 五项及 R206 图片队列断言均通过，没有发现这些命名断言在 CI 中偶发失败的证据：

| CI | Linux Node 断言失败 | 集合秒 | runner 结果 |
| --- | ---: | ---: | --- |
| [37308442468 / 270fc3af](https://github.com/LLYY0418/Deep-Legends/actions/runs/37308442468/job/111757690688) | 0 | 244.784 | 超过 240 秒，exit 1 |
| [37309200638 / bd8a72b7](https://github.com/LLYY0418/Deep-Legends/actions/runs/37309200638/job/111760177621) | 0 | 246.326 | 超过 240 秒，exit 1 |
| [37311010804 / b2d8413d](https://github.com/LLYY0418/Deep-Legends/actions/runs/37311010804/job/111766148804) | 0 | 234.984 | 通过 |
| [37312091768 / 7363fa8f](https://github.com/LLYY0418/Deep-Legends/actions/runs/37312091768) | 0 | 216.631 | 通过 |

前三轮单文件均未超过 90 秒。前两轮属于 runner 的集合耗时预算失败，不能写成 R192/R206 失败或 GitHub 作业硬超时。第四轮 Linux 的两个文件分别为 17.346/1.661 秒。

R223、R217 的本地并发争用与隔离通过说明这两类时限对主机负载敏感，不能据此断言 CI 已偶发。当前未提交源码尚未执行新的 CI，本轮不宣称同 SHA CI 已验收。现有 CI 证据不要求降低任何文件并发；如果未来 CI 实际出现相同命名失败，应优先隔离 `backend/web/r192.test.cjs`、`backend/web/r206-collection-image.test.cjs` 的文件调度，避免与 200 场 jsdom 大夹具重叠，保持原断言/时限。没有据猜测修改全局 worker 数。

R223 的 8 个 >90 秒文件及原始耗时已逐项补到 [R222 耗时记录](r222-execution-ledger.md)。R201 独立 race 复测通过记录保留，未把旧失败输出改写为通过。

## 构建与证据

- JS syntax、`git diff --check` 通过。
- macOS 原生与 Windows amd64 后端重建成功；版本 **0.12.73**、key mode **public**，指纹 **`d7d3268b6b23`**，构建前后稳定。两份二进制指纹核验及原生 `--startup-warmup` 通过。
- 临时产物：`/private/tmp/deep-legends-r226-0.12.73-public`、`/private/tmp/deep-legends-r226-0.12.73-public.exe`。没有安装包，交叉编译不代替 Windows 游戏客户端验收。
- [验证汇总](history/reports/r226/validation-summary.json)、[完整文件计时](history/reports/r226/node-timings-local.json)、[CI 核对汇总](history/reports/r226/ci-flake-review.json)、[构建记录](history/reports/r226/build-final.json)。完整专项/renderer 输出保留在同目录。
