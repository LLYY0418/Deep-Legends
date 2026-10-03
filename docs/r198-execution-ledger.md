# R198 执行账本

日期：2026-10-03。合并版本 0.12.61。R198 取代并扩展 R197 P2，R197 同步测试/诊断保留。包含未正式发布的 R196 改动。

## 逐项实施

| 项目 | 实现与证据 |
| --- | --- |
| P1 设置 | 工具 → 维护 → 安装与连接，保留对局内设置改动下面增加进入游戏时的镜头模式。四选项不修改/自由/动态/锁定，默认不修改，选择立即保存；与保留设置共用 game-settings-sync.json，互不覆盖。只读时仍可选，不加说明。 |
| P2-1 时机 | ChampSelect 一次，GameStart 再核对；大厅选择只应用 LCU；InProgress/Reconnect 无写入，每步重新核对阶段和连接。 |
| P2-2 LCU | GET → 仅 General.CameraMode PATCH → GET 核对；成功才按 OpenAPI 安全探测调用 save。已相同不 PATCH。与 R186 共用写锁。 |
| P2-3 文件 | 仅 ChampSelect/GameStart 且游戏进程未运行；双文件只替换 CameraMode 数字原字节，JSON 格式顺序/INI CRLF/其他设置保持。临时同目录文件、Sync、原子 Rename、读回全文核对。写前二次校验路径、同文件身份、原字节、权限，无符号链接、必须安装目录内。 |
| P2-4 只读 | 两文件只读跳过，LCU 照常；file_result=read_only、file_skipped=read_only。 |
| P2-5 优先级 | R186 结算可保留手动镜头，下一局按下拉覆盖；共享存储和写锁避免互相覆盖。 |
| P2-6 失败 | 不重试、不弹应用失败窗口，只记日志；偏好保存失败才反馈无法保存。 |
| P2-7 不修改 | 在 LCU 和文件读取前返回，零额外读写。 |
| P3 映射 | 本地日志、现有 LCU schema/设置和代码检索只确认 locked=2，未找到自由/动态权威枚举。集中表 free=0,dynamic=1,locked=2，**按选项顺序推断，待用户真机核实**。可单表调整。 |
| P4 诊断 | game_camera_mode_apply 的 stage/target/target_value/lcu_before/after/result/file_before/after/result/save_called 完整，无路径账号；file_skipped 标识只读/游戏进程等。in_game_60s 仅选择非 none 时加 camera_mode_matches_target，同时比较 LCU 与可读本地文件的实际值。 |

Windows 使用已有原生进程检测确认 League of Legends.exe 是否运行；检测失败和非 Windows 无法确认时保守跳过文件写入。CameraModeWASD 始终不动。原隐私自动写入清单已补充实际 CameraMode 写入声明，与实现一致。

## 自动验证

- 工单九项 Go、R197 单字段同步及下一局覆盖、大厅/实际游戏进程/verify failure/持久化/写前并发锁定改写或 symlink/60s 文件回改等测试通过。截断或空 JSON 被 json.Valid 拒绝，测试覆盖。
- 四项 overlay 变异均由指定断言检出，见 [mutations.json](history/reports/r198/mutations.json)。独立只读后端/前端复核完成。
- Node 设置四选项、默认、立即保存、重新加载保持、只读可选、保存失败回滚通过；真实 Chromium 使用生产 UI 与演示 API 验证整页刷新保持。
- 完整 Node 1124 项、1123 通过、1 跳过、0 失败；完整 Go 243.758 秒、专项 race 3.723 秒、vet/diff check 均通过。完整 public 构建 1725 项 Go 分片与安装壳/运行时/密钥/指纹门禁通过，前一轮指纹 `983e425a16cd`；赛后通知与名单请求重叠时接入既有尾随刷新队列，额外 Node 断言与真实 SSE 浏览器复测通过。最终源码指纹 `3a9d539dfb84`，全量 Node/public 构建及 GitHub CI 重跑中，见 [verification.json](history/reports/r198/verification.json)。GitHub CI 与发布待回填。

## 维护页演示

![维护页自由镜头](history/reports/r198/maintenance.png)

真实生产页面，演示数据文件为只读，选择自由镜头并刷新保持；无新增解释文字。主代理已查看维护页截图，见 [browser.json](history/reports/r198/browser.json)。

## 发布与真机边界

发布未完成。0.12.61 将作为 public Latest，包含 R197/R198 和 R196 下载测速选线、ETA、后台完成提示及细按钮。key mode=public，不含个人 Riot Key；旧草稿不修改。发布后回填 release id、isLatest=true、匿名 latest.json 版本和 SHA256/构建指纹。

仍需用户 Windows 真机验收：选择自由镜头连续开两局，进游戏不操作，Esc 选项确认为自由；导出 game_camera_mode_apply（第一局 ok、后续 unchanged）及 in_game_60s camera_mode_matches_target=true。0/1 尚待真机确认，不将自动测试或 Windows runner 构建当作用户验收。R198 保留进行中。
