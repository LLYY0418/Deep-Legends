# R245/R246 收藏页重扫失败核验

日期：2026-10-07，Asia/Shanghai。按用户本轮指示，先单独连续运行原测试三次；三次不是全通过，所以没有完整重跑 renderer，也没有构建安装包。

## 本轮原测试结果

命令：`node --test --test-name-pattern="dirty collection rescans" desktop/refresh-orchestration.test.cjs`。

- 第 1 次：2026-10-07T16:44:38.709963+08:00 → 2026-10-07T16:44:46.286530+08:00，7.576 秒，exit 1；[完整日志](dirty-three-1.log)。
- 第 2 次：2026-10-07T16:44:46.286638+08:00 → 2026-10-07T16:44:53.980022+08:00，7.693 秒，exit 1；[完整日志](dirty-three-2.log)。
- 第 3 次：2026-10-07T16:44:53.980177+08:00 → 2026-10-07T16:44:56.737499+08:00，2.757 秒，exit 0；[完整日志](dirty-three-3.log)。

前两次相同错误：`AssertionError [ERR_ASSERTION]: view change after 60 seconds did not start the deferred rescan`，原断言位于 `desktop/refresh-orchestration.test.cjs:271`。完整汇总见 [dirty-three.json](dirty-three.json)。

## 与 R245/R246 的执行路径关系

- 此测试在 JSDOM/demo 环境中运行。`desktop/license-render-fixture.cjs:3-10` 固定返回 ACTIVE；没有真实 Go 服务、Electron BrowserWindow 或 desktopBackend 原生桥，也不读取共享向量。
- R245 的前端变更只有激活 watchdog 与启动连接文案颜色。`backend/web/license-ui.js:100-106` 的 36000 仅用于 `/api/license/activate`，本测试没有激活提交；`:47-55` 的连接文案分支只在非 ACTIVE 时执行，本测试授权 fixture 固定 ACTIVE。ACTIVE 初始化触发第一次 refreshStatus 的行为没有改动。
- R245 的原生窗口、Go 请求超时、启动续租、诊断，以及 R246 的共享向量都不在此 JSDOM 测试的执行路径中。R246 两小时常量尚未修改。`backend/web/app.js` 与原测试文件的 SHA 相对 R245 执行前均相同；见 [protected-stop.json](protected-stop.json)。
- 据此，将本次失败归为两单行为改动范围外的既有测试时序问题，按用户规则停止。没有应用此前提出的受限测试补丁，没有修改 app.js、原断言、原等待预算或 60 秒限制。

## 时序缺口与诊断边界

`desktop/refresh-orchestration.test.cjs:236-241` 在返回 status Response 之前增加 completedStatusRequests；`:260-264` 仅等这个计数，不能保证 clean/dirty 状态已经提交。`backend/web/app.js:401-405` 还要解析响应体并校验取消/控制器，`:443-449` 要通过请求 token 检查才提交状态；`:456-458` 才解除重扫门控。`:2749-2750` 又同时检查 60 秒与 in-flight gate。因此测试的“传输完成”等待条件不足以证明“门控已释放”，存在时序缺口。

本次没有捕获失败瞬间的具体请求 token 和已提交状态；精确是哪一次响应被丢弃、是否还有其他时序因素，尚未证实。以下对照用于界定证据，不替代原三连测或全量验收：

- 隔离副本仅撤回两项 R245 授权 UI 变化：三次原单测均 PASS（2.163, 2.166, 2.148 秒）；[报告](dirty-pre-r245-ui.json)。当前 UI 此前也曾三连 PASS，因此这组成功不能单独证明版本因果关系。
- 临时夹具注入 clean 响应体解析延迟 120ms，原断言/预算保留：当前 UI、撤回 UI 两种情况均 PASS；[报告](dirty-delayed-clean.json)。该注入没有复现失败，不能据此宣称精确根因已证实。
- 同一完整测试文件的诊断对照：撤回 UI 变化 exit 0（6.339 秒），当前 UI exit 0（6.057 秒）；[撤回 UI](dirty-pre-r245-ui-file.json)、[当前 UI](dirty-current-file.json)。这是测试波动的证据，不是完整 renderer 重跑。所有临时副本已删除。
- R244 修改前 app/gameplay/runtime 副本也有同一断言失败：`docs/history/reports/r244/baseline-collection-rescan.log:15`；该对照不能严格证明 R245 修改前的授权 UI 状态，故只作为旧收藏流程也存在同类问题的补充证据。

## 交付状态

新共享向量已按用户提供的 SHA-256 核验并按字节替换，共 69 条，SHA-256：`39377b860440d8f609f9d28162afe086e4e01ac91d3d2ff39da2a45d5eebfcc7`。只有 R246 P4 的向量交接步骤完成；两小时租约实现、对应测试/校验常量、P5 和 P6 尚未执行。

版本 0.12.75；未读取私钥、未使用真实注册码、未发布。**未在 Windows 实跑**。新安装包未构建，暂无新安装包 SHA-256。
