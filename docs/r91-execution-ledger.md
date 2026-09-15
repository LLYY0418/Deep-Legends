# R91 执行记录

日期：2026-09-14。范围：`WORKLIST-R91-LIVE-STALE-DATA.md`，包含执行期间补充的第 5 条“普通刷新立即中断 live 请求”及局部操作边界。保留工作区已有 R87–R90 等改动；本轮未改后端或发布安装包。

## 逐项落实

| 工单条目 | 最终行为 | 自动化证据 |
| --- | --- | --- |
| 1. 新局先失效旧数据 | 阶段边界立即清空 live、清理局内推荐缓存、使旧 token 失效并取消 live 请求；后台或隐藏页面也先失效。loadLive 也检测已经被 beacon 观察到的边界。gameId 变化沿用已有重置逻辑。 | `web/r91.test.cjs` 新局延迟响应、旧响应迟到、隐藏页面、直接 loader 与 gameId 测试。 |
| 1. 同局不能过度清空 | 同 phase 刷新及 ChampSelect → GameStart → InProgress ↔ Reconnect 保留当前推荐，不进入骨架或新局过渡态。 | 实际 loadLive + renderLive、DOM 内容及缓存 generation 断言。 |
| 2. 明确过渡态 | 显示“正在识别新对局…”和重试入口；同时清空上一局摘要。空名单/未就绪快照继续显示过渡态，模式不支持时显示说明；真实结束/回大厅后退出过渡态。 | 多阶段、空 available、未 available、落后大厅响应、主动离开及 unsupported 测试。 |
| 3. 失败/取消/晚到 | 错误或无替代请求的取消显示明确失败与可点重试。soft reset 与事件连接中断递增 token，旧成功、异常、finally 均不能污染新请求。事件连接中断原本清空后无说明，本轮补错误出口及重试调度。 | deferred resolve/reject、软重置、首次取消、断线及重试恢复测试。 |
| 4. 刷新反馈 | 顶栏按钮与内容状态显示“正在刷新…”，按钮设置 aria-busy，请求完成后恢复。 | 实际按钮点击及 DOM 文案、aria-busy 断言。 |
| 5. 强制刷新立即生效 | 飞行期间 `loadLive(true)` 只取消 live controller，立即发出 `?refresh=1`，不再等旧请求完成；旧 finally 不会结束新请求或多发一次请求。 | 用永不主动结束的请求证明第二次请求立即启动，再分别注入迟到成功和取消。 |
| 5. 局部职责 | 其他 controller 保持原对象且未被取消；标签页加载、分页状态、推荐选项和缓存保持。后台非强制请求继续合并。 | 预置 overview/collection controller 与分页状态，对取消信号、对象身份、状态与推荐缓存逐项断言。 |

## 验证

- 定向前端：34 项全部通过，见 [targeted-js.txt](r91/targeted-js.txt)。R91 新增 13 项，每项包含多个阶段或错误分支；旧 R87/R88/R90 测试同步接入新增依赖，R90 手动排队断言更新为 R91 的立即替换行为。
- 修复前先复现：最初 8 项中 6 项失败，见 [before-js.txt](r91/before-js.txt)。第 5 条补入后，新增立即替换/局部隔离断言在排队版本失败，见 [before-forced-retry-js.txt](r91/before-forced-retry-js.txt)。
- 六个隔离变异全部被拦截：保留旧局、同局过度清空、等待卡住请求、取消其他页面、接受废弃响应、隐藏加载反馈。见 [mutations.json](r91/mutations.json)，复跑 `python3 scripts/r91-mutation-check.py`。仅临时副本被修改。
- 独立只读复核覆盖竞态与验收范围，识别了执行期间补入工单的新要求；修复后再次确认普通刷新所有入口、局部取消及 token 收尾逻辑。后台 `force=false` 保持合并符合工单明确限定的用户主动 `force=true` 范围。
- 首轮完整 JS 回归发现一个旧单测的按钮替身缺少 `setAttribute`，已按实际 DOM 能力补齐；失败日志保留为 [full-js-initial.txt](r91/full-js-initial.txt)，不作为通过证据。
- 最终完整 JS 回归：649 项，648 通过、0 失败、1 跳过（Windows/PowerShell 发布门禁），见 [full-js.txt](r91/full-js.txt)。`node --check web/gameplay.js`、`node --check web/r91.test.cjs`、`git diff --check` 全部通过。
- [起始快照差异](r91/scoped-changes.patch) 区分本轮与已有未提交修改；另在 R88 定向测试中补一个新 helper 依赖替身，并新增 R91 测试和变异脚本。[源码 SHA256](r91/source-sha256.json) 在完整验证结束后核对一致。

复跑：`node --test web/*.test.cjs desktop/*.test.cjs`；定向 `node --test web/r91.test.cjs web/r90.test.cjs web/r87.test.cjs web/r88.test.cjs`。

## 真机验收未执行

需在 Windows 国服客户端连续进行多局，观察切局瞬间不再展示上一局推荐；请求中点击普通刷新应有反馈并重发，同时其他页面保持；对比全局“重新获取”仍有整体重置行为。

本轮自动化验证不能替代真实 LCU 请求耗时或用户体感验证，不宣称“响应慢已彻底解决”。若仍慢，应另行采集 `/api/gameplay/live` 与 LCU 耗时分布。
