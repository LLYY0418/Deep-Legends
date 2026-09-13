# R85 补单：手动检查更新

本次只完成 `WORKLIST-R85-ADDENDUM-MANUAL-CHECK-BUTTON.md` 的前端入口及用户追加的玩家头像悬停修改。版本保持 `0.12.1`；未执行首发工单、未改成 `0.13.0`、未发布 Release。按用户之前明确要求，本次不代为打包，可安装构建由用户自行生成。

## 修改

- 设置页版本信息旁新增「检查更新」，使用现有 `text-button` 样式与 `api()` 请求封装，发送 `POST /api/update/check`。
- 请求未返回或后端仍在异步检查时保持「正在检查…」和禁用态；下载、校验、应用升级、其他更新操作待响应时也禁止再次检查。
- 后端检查是异步的，HTTP 可能只返回 `checking`。最终结果复用已有 SSE 状态事件，避免提前提示“已是最新”。如果 SSE 已经完成检查，迟到的 HTTP `checking` 响应不会覆盖它。
- 新版本复用顶部通知按钮的 `openUpdateDialog()`、`renderUpdateDialog()` 和唯一的 `#update-dialog`。无更新时提示「已是最新版本」；HTTP/网络失败及异步检查失败均有错误文字。反馈 5 秒后清空，再次检查会清除旧反馈及计时器。
- `supported !== true` 时入口隐藏并禁用。后端 `Check(true)`、自动检查间隔和忙碌判定均未修改。
- 总览玩家标签头像作为装饰图，不再生成 `profile 17` 等图标提示，也不继承整个标签的文字提示；完整玩家名提示仅在鼠标移到被截断的名字文字时出现。标签仍有玩家名和原有键盘操作。

## 验收

| 工单判据 | 结果 |
| --- | --- |
| 设置页版本旁可见按钮 | Chromium 页面及截图验证通过 |
| 实际发送强制检查请求 | 模拟服务记录 `POST /api/update/check`，自动测试验证 `api()` 请求头 |
| 发现更新复用原弹窗 | Chromium 确认原 `#update-dialog` 打开；测试比较两个入口的弹窗完整内容一致 |
| 无更新反馈 | Chromium 与自动测试确认「已是最新版本」，测试验证超时清空 |
| 请求失败反馈 | Chromium 验证 HTTP 400；自动测试额外覆盖网络失败和 SSE 异步失败 |
| 不支持更新 | Chromium 确认按钮隐藏，自动测试确认无请求 |
| 忙碌及连点 | 自动测试覆盖 checking/downloading/verifying/applying、请求待返回、其他更新操作进行中 |

Chromium 使用真实 HTML/CSS/前端代码和本机模拟服务；截图中的目标版本 `0.13.0` 只是测试数据，不是实际发布或版本号变更。此次没有下载、校验、重启安装真实更新；完整升级链路仍需用户自行打包安装后按两阶段计划验证。

## 自动检查

- `node --test web/update.test.cjs`：13 项通过。包含 18 个元素注册遗漏变异，以及请求接口/方法错误、另建弹窗、不支持更新仍可请求、忙碌重复请求、迟到 HTTP 覆盖 SSE 结果共 5 类变异。
- `node --test --test-name-pattern='player tab|player overlay' web/champions.test.cjs`：3 项通过，包含头像及其祖先没有 tooltip/title、名字提示与键盘语义保留。
- `node --check web/app.js`、`node --check web/gameplay.js`、修改文件的 `git diff --check` 通过。

临时 Chromium 页面和本机模拟服务均已关闭。
