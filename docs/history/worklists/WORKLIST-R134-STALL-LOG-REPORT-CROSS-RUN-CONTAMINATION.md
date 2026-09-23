# WORKLIST-R134：`r133-stall-log-report.cjs` 判词会被不相关的另一次启动污染

诊断人：Claude（独立复测 R133 时用自己写的合成日志夹具发现，未改仓库代码）。
执行人：GPT。
日期：2026-09-23。
基线：0.12.18（R133 之后）。
状态：待执行。这是 R133 新增的诊断工具本身的缺陷，不是 R130 的收藏页代码问题。

---

## 0. 结论

R133 P2 新增的 `scripts/r133-stall-log-report.cjs` 是用来回答「`card_image_stalled` 出现时，是第二层积压（正常）还是通知失败（真缺陷）」的。它的判法是：对每条 `card_image_stalled`，往前 60 秒、往后 10 秒（默认参数）开一个时间窗，数窗口里有几条 `local_request_client failed(endpoint=image)`——≥3 条判"积压"，0 条判"通知失败"。

问题是这个时间窗**只按时间戳圈，不按 `run_id` 分组**（`scripts/r133-stall-log-report.cjs:137` 那行 `records.filter(isImageFailure)` 是对整份日志过滤，没有再按 stall 所在的 `run_id` 筛一遍）。诊断日志是跨多次启动累积写在同一个文件里的（R130/R133 自己的台账、以及更早的 R127 日志分析都见过同一份导出文件里有好几个不同 `run_id` 的 `app_start`）。只要用户在一分钟内重启过一次客户端助手（崩溃后自动重连、手动重启应用设置、双开等都有可能），前一次或后一次启动里发生的、和这次 stall 毫无关系的图片失败，就会被当成"同一次卡顿事件的背景噪音"计入窗口，把本该判成"通知失败"（新缺陷、需要开工单）的真事件，判成"积压"（正常、不用管）。这正好是这个脚本存在的意义要防止的那种误判——静默吃掉一个真实的回归。

## 1. 复现

用自己写的合成日志（不依赖仓库里任何既有夹具）：两次启动 `runA`、`runB`，同一个文件里时间戳挨得很近；`runA` 只有一条 stall、旁边没有任何图片失败（单独看应该判"通知失败"）；`runB` 在差不多同一时间窗口里有 4 条不相关的图片失败（比如另一次启动里大乱斗符文图标超时，这在 R127 的日志里是常见背景噪音）：

```json
{"event":"app_start","time":"2026-09-23T01:00:00.000Z","version":"0.12.18","run_id":"runA"}
{"event":"card_image_stalled","reason":"watchdog","time":"2026-09-23T01:00:30.000Z","run_id":"runA","active_card_images":8,"queued":0,"source_index":0}
{"event":"app_start","time":"2026-09-23T01:00:00.000Z","version":"0.12.18","run_id":"runB"}
{"event":"local_request_client","reason":"failed","endpoint":"image","time":"2026-09-23T01:00:10.000Z","run_id":"runB","error_kind":"timeout"}
{"event":"local_request_client","reason":"failed","endpoint":"image","time":"2026-09-23T01:00:15.000Z","run_id":"runB","error_kind":"timeout"}
{"event":"local_request_client","reason":"failed","endpoint":"image","time":"2026-09-23T01:00:20.000Z","run_id":"runB","error_kind":"timeout"}
{"event":"local_request_client","reason":"failed","endpoint":"image","time":"2026-09-23T01:00:25.000Z","run_id":"runB","error_kind":"timeout"}
```

```
$ node scripts/r133-stall-log-report.cjs cross_run.jsonl
…
  #1 2026-09-23T01:00:30.000Z reason=watchdog active=8 queued=0 sourceIndex=0 imageSource=（无）
      窗口内图片失败：4 条 → backlog

判定：backlog-only
第二层积压：……该调的是第二层的远程/本机道名额，不是 R130 的合成 error / 看门狗。
```

`runA` 的这一条 stall 独立来看应该判"通知失败"（旁边 0 条图片失败），但因为脚本没按 `run_id` 隔离，被 `runB` 的失败"借用"证据判成了"积压"，结论从"这是新缺陷，另开工单"变成了"这是正常现象，不用管"。

R133 自带的 `scripts/r133-stall-log-report.test.cjs`（14 条）里没有一条构造过"同一份日志里出现两个不同 `run_id`"的场景，所以这个缺口没被测出来。

## 2. 修复方向

在数窗口那一步，先按 `record.run_id` 把图片失败事件分组，再在窗口过滤时只看和当前 stall 同一个 `run_id` 的失败（`stall.run_id` 未知的失败——没写 `run_id` 字段的旧日志——仍按现在的全局口径处理，不要因为这个缺陷又引入新的漏判）。同时给 `parseArgs`/摘要输出加一条：如果同一份文件里出现多个 `run_id`，摘要行里提示一下"本文件跨 N 次启动"，方便人工核对判词有没有跨到别的启动。

## 3. 测试（对抗变异）

- 用本工单第 1 节那份构造日志：`runA` 唯一一条 stall 必须判 `notify-gap`（当前会错判成 `backlog-only`，先确认这条能复现失败）。
- 反过来的场景：`runA` 的 stall 旁边有 `runA` 自己的 3 条以上失败（真积压），同一时间窗口里 `runB` 完全没有任何事件——必须仍然判 `backlog-only`，不能因为改成按 run 分组就连真积压都判不出来了。
- 单 `run_id`（现有 14 条用例的场景）不能受影响，全部继续通过。
- `run_id` 缺失的旧日志（R89 那份夹具）行为不变，仍然是 `clean-unverified`。
- **对抗变异**：把按 `run_id` 分组这步去掉，第一条用例必须 FAIL。

## 4. 验收

- 上面 4 条用例全部通过，变异测试证明分组逻辑确实在生效。
- 现有 14 条 `r133-stall-log-report.test.cjs` 用例与 R133 台账里跑过的 `docs/r89-kr-player/diagnostic-samples.jsonl` 冒烟一字不差地继续通过。
- 这个脚本不进 CI（`scripts/*.test.cjs` 不在两条 `node --test` 命令里），改完仍然只能手动跑：`node --test scripts/r133-stall-log-report.test.cjs`，执行记录里写清楚跑了。
