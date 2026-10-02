# R156 执行账本

日期：2026-09-25。基线为含 R144–R155 未提交改动的工作区；没有回退这些改动。产品版本由 0.12.20 递增至 0.12.21。未打安装包。

## 逐项结果

| 项 | 实施 | 本机验证 |
|---|---|---|
| P1 | 邀请队列优先读 `gameConfig.queueId` / `queueID`，没有有效值时回退顶层。首次非空邀请列表只记录一次顶层及 `gameConfig` 的键名。R50 原有假数据已改为嵌套，新增扁平兼容、嵌套优先及 accept/decline 用例。 | 定向测试通过；隔离副本移除嵌套读取后，嵌套 accept 用例按预期失败。真实邀请 DTO 尚未取得，不能把大厅 DTO 的形状当作邀请 DTO 的实证。 |
| P2 | GET 失败、无策略、POST 成功/失败和自定义房间暂停都记录 `watch_action`，保留原有界面事件。读取列表期间切入自定义房间也记录跳过。诊断只含数值队列、策略、结果和固定原因。 | 模拟 HTTP 的九种场景通过；另将模拟海克斯邀请接入 `app.recordDiagnostic`，从实际诊断文件读到 `fired`、2400、`accept` 和 shape 事件，检查邀请 ID 与召唤师名未落盘。隔离副本删除动作记录后，GET 失败断言按预期失败。 |
| P3 | 连接会话每 30 秒扫描并发布已过 10 秒窗口的请求桶；连接结束时刷新余桶并停止计时器。固定邀请路径单独保留，动态邀请 ID 脱敏为 `{id}`，避免路径误归类和标识泄漏。 | 假时钟证明孤立请求在连接不断开、没有后续请求时落盘；窗口轮转和 `Close` 路径通过；隔离副本移除定时刷新后，孤立请求断言按预期失败。 |
| P4 | 邀请策略新增一个「海克斯大乱斗」选项，循环切换时同时写入 2300、2400、3270 三个独立策略键。后端队列匹配仍逐键处理。 | UI 生成与保存状态测试通过；只写入首个键的变异被拦截。 |

## 证据边界

- 更新版工单给出的 `queue_id:2400` 是**当前房间**的海克斯大乱斗证据，不能单凭它确认好友发来的那条邀请也属于 2400。450 与 2300/2400/3270 确实是不同队列；旧配置只覆盖 420/440/450，不会自动接受海克斯大乱斗邀请。
- 工单提到的 `lol-loot-diagnostics-0925-1427.jsonl` 原件不在本工作区，本轮没有拿到真实邀请 JSON，也无法在本机模拟 Windows League 客户端好友邀请。P1 真因与 P4 对那次具体邀请的解释仍待真实邀请或下一份 `lcu_invitation_shape` / `watch_action` 日志核实。
- 本轮没有自动替用户开启海克斯大乱斗「接受」：新选项沿用默认「不处理」，用户需要在设置里显式点击。

## 验证记录

- `go test ./backend -run 'TestR156|TestWatchEventsRouteSkipCelebrationAndInvitationPolicies|TestLCURequestDiagnosticsCaptureRealTLSAndConnectionReuse|TestR66LCURequestDiagnosticsIncludePreferencePOSTWithoutBody' -count=1`：通过。
- `node --test backend/web/r156.test.cjs`：通过，内含 P4 对抗变异断言。
- P1/P2/P3 对抗变异：在 `/private/tmp` 隔离副本分别移除嵌套读取、邀请动作记录、定时刷新；对应测试均按预期失败，主工作区未变异。
- `go build -o /private/tmp/deep-legends-r156 -ldflags '-X main.version=0.12.21' ./backend`：通过；`-self-test` 输出“Deep Legends 0.12.21 自检通过”。
- `go vet ./...`：通过。
- `go test ./... -count=1`：通过，backend 191.694 秒。
- `node --test backend/web/*.test.cjs desktop/*.test.cjs`：971 项，970 通过、1 跳过、0 失败；R154 基线为 970 项，969 通过、1 跳过、0 失败。
- `go test -race ./backend/... -count=1`：通过，backend 239.298 秒。
- 全量测试后补充了扁平 `queueID` 拒绝用例和模拟邀请落盘用例；`go test ./backend -run 'TestR156|TestWatchEventsRouteSkipCelebrationAndInvitationPolicies' -count=1` 再次通过。

## 待真实客户端复核

1. 用户在「房间邀请处理」里把「海克斯大乱斗」切到「接受」，核对诊断中保存的 2300、2400、3270 均为 `accept`。
2. 让好友邀请进入海克斯大乱斗房间，核对 `lcu_invitation_shape` 的实际键名、`watch_action` 的队列与结果，以及脱敏的 `lcu_request` 路径；随后确认客户端确实进入房间。若遇到 `skipped_no_policy`，依据实际 `queue_id` 调整策略，不推测队列。
