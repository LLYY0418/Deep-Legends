# WORKLIST-R219：客户端未启动时登录入口不显示；启动遮罩空等 15 秒并误报「客户端正在启动」

诊断：Claude（两张截图 + 日志 `lol-loot-diagnostics-1005-1605.jsonl` + 只读核对源码）。执行：GPT。日期：2026-10-05。
基线：0.12.71（指纹 `696da05d0ad0`，本次运行 `run_id f21ffebad8ed197548b97aed`）。和 R211～R218 一起合并进下一个版本。

---

## 现象

1. **登录入口不见了**（图一）：总览页「选择登录入口」卡片下方一直是「正在检查安装位置…」和「正在检查腾讯英雄联盟客户端安装位置。」，以前的「国服纯净入口」「Riot 客户端」按钮都不出现，也没有「重新检查安装位置」按钮。这两行字和 `index.html` 第 135～136 行的静态占位文字完全一样，说明页面加载后这一块从来没有被重新渲染过。
2. **启动遮罩空等**（图二）：电脑上没有启动英雄联盟客户端，打开软件后却出现「正在连接英雄联盟客户端 / 检测到客户端正在启动，请稍候。」的全屏遮罩，要等很久才消失。

---

## 日志证据（北京时间 16:00:52 启动）

| 时间 | 事件 | 说明 |
|---|---|---|
| 16:00:52.651 | `lcu_discovery result=process-not-found`，`process_count=0` | 后端启动 70ms 就确认：**没有客户端进程** |
| 16:00:52.962 | `blocking_state_client reason=show source=startup` | 前端显示启动遮罩 |
| 16:00:53.023 | `local_request_client endpoint=status` 200 | 第一次状态读取已完成，此时后端已经是 `connectionState=disconnected` |
| 16:01:07.962 | `blocking_state_client reason=hide` | 遮罩关闭…… |
| 16:01:07.963 | `blocking_state_client reason=timeout duration_ms=15000` | ……原因是 **15 秒兜底超时**，不是任何检测结果 |

- 整份日志里 `client_installations_scan` 出现 **0 次**；这次运行的 `local_request_client` 里也没有 `/api/client-installations` 请求（属于 `other` 类别，启动后 4 分钟内唯一一条 `other` 是 16:05:36 的导出日志）。也就是说，**安装位置检查从来没有发出去**。

---

## 根因（源码已核对）

### P1　`state.installations` 初始值被删，渲染登录入口卡时抛异常，安装检查永远不会发出

- `backend/web/app.js` 的 `state` 初始化里只有 `installationsLoaded / installationLoadedAt / installationLoadPromise / installationLoadError`，**没有 `installations`**。`git log -S` 显示 `installations: []` 是在 2026-09-23 的 `c2b678d0`（R135 基线）里被删掉的，从 v0.12.19 起的版本都带着这个问题。
- `renderLaunchpad`（约 880 行）在判断 `installationsLoaded` **之前**就执行 `state.installations.filter(...)`。客户端未连接、当前在总览页时，`state.installations` 是 `undefined`，这里抛出 `TypeError`。
- 调用链：`refreshStatus` → `renderStatus`（469 行）→ `renderLaunchpad` 抛错 → 跳到 `catch`。因为 `statusReceived` 已经为 true，`catch` 直接 `return`，**不记任何日志**。
- 于是 470 行的 `await loadClientInstallations()` 永远执行不到；之后每次状态刷新（事件触发或手动刷新）都同样在 `renderLaunchpad` 抛错，入口卡永远停在 HTML 静态占位。
- 连带影响：`renderStatus` 里排在 `renderLaunchpad` 之后的 `updateWorkspaceAvailability`、`renderUpdateStatus` 和 `deep-legends:status` 事件派发，在客户端未连接时也都不会执行；`refreshStatus` 里 483 行的第二次 `updateReadingOverlay`、收藏/奖池的后续加载同样被跳过。
- 现有测试没发现：`official-login.test.cjs` 的夹具总是自带 `installations: [...]`，`champions.test.cjs` 把 `renderLaunchpad` 换成了空函数。

### P2　未连接一律当成「客户端正在启动」，遮罩只能靠 15 秒超时关闭

- `updateReadingOverlay`（约 528 行）：只要 `data.connected` 为 false，就显示「正在连接英雄联盟客户端 / 检测到客户端正在启动，请稍候。」，并把轮询间隔改成 900ms。它不看后端已经给出的 `connectionState`，也拿不到进程检测结果。
- 这个函数里能关掉遮罩的分支只有两个：「已连接且身份就绪」和 `overlaySuppressed`（只在 15 秒超时后才设置）。客户端没启动时，两个条件都不会满足，所以每次冷启动都要完整等满 15 秒。
- 后端其实早就知道答案：`runConnectionManagerWith` 第一次 `discover` 失败后立刻 `markDisconnected`，把 `connectionState` 设为 `disconnected`；`a.discovery.Result` 里还有 `process-not-found`。但 `statusResponse` 没有带出检测结果，前端也没用 `connectionState`。
- 文案也错了：没有检测到进程时显示「检测到客户端正在启动」，属于误报。

---

## 修复要求

### P1　登录入口恢复显示（必做）

1. `state` 初始化补回 `installations: []`。
2. `renderLaunchpad` 改为先判断 `installationsLoaded`，再计算列表；`filter` 前用 `(state.installations || [])` 兜底，任何初始状态下都不能抛错。
3. `refreshStatus`：未连接时触发 `loadClientInstallations()` 的逻辑，不能依赖 `renderStatus` 成功执行。把它放到 `renderStatus` 之前，或者独立出来。
4. `refreshStatus` 的 `catch`：`statusReceived` 为 true 之后再出现的异常（渲染异常）不要静默吞掉。新增诊断 `status_render_failed`（只记录异常类型和抛出函数名，不记录堆栈里的路径或用户数据），runtime 白名单和 `features.go` 一起登记。
5. 不新增任何界面说明文字（CLAUDE.md 界面文案红线）。

### P2　启动遮罩按真实检测结果显示（必做）

1. 后端 `statusResponse` 增加 `clientDiscovery` 字段，取值直接用 `a.discovery.Result`（`process-not-found`、`process-query-failed`、`credentials-unreadable`、`probe-failed`；已连接时为 `connected`；还没检测过时为空字符串）。只带结果枚举，不带 detail 和路径。
2. 前端 `updateReadingOverlay` 在未连接时按下面的规则处理：
   - `clientDiscovery` 为空（第一次检测还没结束）：可以显示「正在连接英雄联盟客户端 / 正在检测英雄联盟客户端。」。正常情况下第一次状态读取时检测已经完成，这一状态应该在 1 秒内结束。
   - `process-not-found` 或 `process-query-failed`：**立即关闭遮罩**，不设置 `overlaySuppressed`，不改成 900ms 轮询，直接显示登录入口卡。
   - `credentials-unreadable` 或 `probe-failed`（进程在、接口还没就绪）：才显示「检测到客户端正在启动，请稍候。」。
   - 已连接但身份未就绪：保持现状，显示「正在读取召唤师信息」。
3. 后端每次重试检测时都会短暂把状态设成 `connecting`，前端不能因为这个短暂状态重新打开遮罩。遮罩只看 `clientDiscovery`，不要用 `connectionState === "connecting"` 判断。
4. 用户在软件打开期间才启动客户端：检测到进程（`credentials-unreadable` / `probe-failed`）后，可以按现有逻辑显示「客户端正在启动」遮罩，15 秒兜底不变。
5. `blocking_state_client` 的 `hide` 事件增加 `hide_reason`：`identity-ready` / `no-client-process` / `timeout` / `suppressed`。

---

## 测试

- **新增 `backend/web/r219.test.cjs`**：
  1. 用**不带 `installations` 字段**的全新 state，状态为未连接、总览页：`renderStatus` 不抛错；`loadClientInstallations` 被调用一次；安装检查返回 tcls + riot 后，入口列表出现两个 `[data-client-id]` 按钮。
  2. 安装检查失败或返回空列表：分别显示「安装位置检查失败」/「没有检测到可启动入口」和重新检查按钮。
  3. 状态 `connected=false, clientDiscovery="process-not-found"`：`updateReadingOverlay` 之后遮罩隐藏，`blocking_state_client hide` 的 `hide_reason=no-client-process`，没有 `timeout` 事件，`statusDelay` 不是 900。
  4. 状态 `clientDiscovery="probe-failed"`：遮罩显示「检测到客户端正在启动，请稍候。」。
  5. 状态 `clientDiscovery=""`：遮罩文案是「正在检测英雄联盟客户端。」，不是「客户端正在启动」。
  6. `renderLaunchpad` 抛错时，`status_render_failed` 会上报，并且 `loadClientInstallations` 仍会被调用。
- **新增 Go 测试**：`/api/status` 在 `process-not-found`、`probe-failed`、已连接三种情况下的 `clientDiscovery` 取值正确；`status_render_failed` 的白名单字段和裁剪规则。
- 修改现有夹具：`official-login.test.cjs` 补一条「初始 state 没有 installations」的用例，防止回退。
- 全量 `node --test backend/web/*.test.cjs` 与 `go test ./backend` 的结果写进账本。

---

## 真机验证（构建后由用户操作）

1. 关掉英雄联盟客户端，打开软件：
   - 启动遮罩不出现，或者在 1 秒内消失；不会出现「检测到客户端正在启动」。
   - 总览页登录卡片在 1～2 秒内出现「国服纯净入口」（以及安装了 Riot 客户端时的 Riot 入口）。
   - 日志：`lcu_discovery result=process-not-found` 之后，`blocking_state_client hide hide_reason=no-client-process`，没有 `timeout`；能看到 `client_installations_scan result=detected` 和 `/api/client-installations` 请求。
2. 软件开着时点「国服纯净入口」启动客户端：登录过程中按现有逻辑提示，登录进入大厅后自动连接，入口卡收起。
3. 先开客户端再开软件：行为与现在一致，遮罩显示「正在读取召唤师信息」，身份就绪后关闭。
