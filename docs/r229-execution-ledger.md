# R229 执行账本

日期：2026-10-06。基线与当前版本 **0.12.74**；本单不更新版本、不提交、不推送或发布。对应 [R229 工单](WORKLIST-R229-FOREIGN-SELF-TAB-KEPT-AFTER-CLIENT-CLOSE-RIOT-PUUID-400-SEARCH-LABEL-GROUP-SWITCH-AND-PER-GROUP-LAUNCHERS.md)。同一工作区另一聊天同时执行 R230，保留其改动；全量检查覆盖共享源码，不能把中途失败记录当成最终通过。

## 实现

| 项目 | 结果 |
| --- | --- |
| P1 | 断开时本人页签无论区服均停止计数、隐藏并清空数据，取消在途请求、当前对局轮询与重试；手动打开的外服页签保留。同账号重连强制重新加载，迟到的旧当前对局请求不能写回新会话。 |
| P2 | 新增统一 LCU→公开 PUUID 转换，以平台+客户端 PUUID 缓存六小时，并合并并发请求；身份切换清缓存，旧请求不能重新填入。本人历史、其他 LCU 玩家历史、实时对局、熟练度、段位主路径及旧段位兜底均先转换。没有 Riot ID 或转换失败时退回 LCU。公开对局转换以公开 PUUID 标记本人，随后只把本人领域引用映射回 LCU 身份。客户端引用显式标记，手动搜索的公开身份保持独立。400 decrypt 分类为 `riot-puuid-mismatch`，其他 400 为 `riot-http-400`，不保存错误体。 |
| P3 | 搜索按钮仅显示区名或大区名；跟随客户端未识别时为“外服”或“国服”。下拉菜单不变。 |
| P4 | 三组始终可选；国服空组显示纯净入口及 WeGame，外服空组显示 Riot，职业保留原空状态；搜索页签和已连接时收起入口。未检测到的入口禁用并保留重新检查按钮。关闭后回到上次客户端所属组，该组随已识别客户端持久保存；首次默认国服。 |
| P4 WeGame | 增加注册表与 WeGame 快捷方式候选，只打开主程序或快捷方式，不附加猜测参数。来源诊断仅 registry/shortcut；1223 立即取消，不尝试后续候选。 |
| P5 | 成功启动后未连接的 120 秒内每秒发现一次；被动发现启动中也最多加速 120 秒，连接后恢复原退避；未点启动且无进程时保持原频率。新增 `client_launch_to_connected` 三阶段累计毫秒，前端本人总览实际完成渲染后回报首屏完成。未观察到阶段为 -1，固定枚举与数值白名单落盘。 |
| P6 | 已连接→断开后 15 秒内对 credentials-unreadable/probe-failed 跳过启动遮罩，记录 reason=skip、skip_reason=client-exiting；原 hide_reason 不变。冷启动和新一次成功启动仍可显示遮罩。 |

## 自动验证

证据位于 [reports/r229](history/reports/r229/)。新增 Go 回归覆盖转换后历史及本人识别、缓存和失效、取消并发等待、缺 Riot ID 回退、400 分类、两注册表入口、快捷方式、无参数候选、1223、120 秒窗口及实际 JSONL 计时落盘。新增六项前端回归覆盖 P1/P3/P4/P6、同账号重连与迟到请求。

| 检查 | 结果与证据 |
| --- | --- |
| R229 Go 专项 | 通过，`go-targeted.log`。 |
| R229/区服/连接/启动器/段位 race 回归 | 通过，`go-race-targeted.log`。 |
| `go vet ./backend` | 通过，`go-vet.log`。 |
| `node --test backend/web/*.test.cjs` | 961/961 通过，19.50 秒，`frontend-full.log`。 |
| `node scripts/test-renderers.cjs all` | 1293 项，1289 通过、4 项平台跳过、0 失败；86.43 秒，最长文件 46.05 秒，均在 90 秒/4 分钟预算内；`renderers-all.log` 与 `node-timings-local.json`。 |
| R229 前端及空分组专项 | 9/9 通过，`frontend-targeted.log`；真实脚本断连→空总览→启动入口 1/1 通过，`disconnect-render.log`。 |
| JS syntax / diff whitespace | 通过。 |
| `go test ./backend -count=1` | **未通过**，111.32 秒，`go-full.log`；这轮共享源码检查有下面三项 R230 相关失败。 |

Go 全量失败是 `TestClientDiagnosticAcceptsMultipleEventWhitelists`（R230 增加 overview_card_ready / summoner_copy 字段后旧通用夹具缺少必要字段）、`TestR230OverviewNavigationReusesHistoryAndManualRefreshBypasses`（导航历史重复请求）及 `TestR95SquadSurvivesHiddenChampSelectAndScrambledLive`（R230 新增磁盘恢复后 clear 再次恢复同局小队）。前后两轮全量与独立核查证据分别在 `go-shared-intermediate.log`、`go-full.log`、`shared-r230-failures.log`。这些失败尚未按本单改动；后续由 R230 修复并重跑完整 Go 验收。**本账本不宣称 Go 全量或发版预检通过。**

中间共享源码检查还发现 R230 卡片顺序、计时依赖与样式护栏失败；最终前端与 renderer 已通过。保留 `frontend-shared-intermediate.log`，不将中途失败改写成通过。

## 构建

macOS 原生与 Windows amd64 后端均重建，版本 **0.12.74**，key mode **public**（空内嵌 Key）；两产物通过 Key 策略及源指纹校验，原生 `--startup-warmup` 通过。最终构建前后源指纹同为 **`cfa9d7039853`**。详见 [build-final.json](history/reports/r229/build-final.json)。共享源码可能继续由 R230 更新，构建记录只覆盖其记录的完成时源指纹；上面的全量测试为相应日志中的当轮共享源码结果，不背书后续 R230 改动。

产物为 `/private/tmp/deep-legends-r229-0.12.74-public`、`/private/tmp/deep-legends-r229-0.12.74-public.exe`；未生成安装包、未更新发布版本号或进行发布。

## Windows 真机边界

工单已明确真机由用户执行。本会话在 macOS，没有 Windows 客户端连接；交叉编译与合成夹具不代替真机结果。

- WeGame 当前按工单实现 `HKLM\SOFTWARE\Tencent\WeGame`、`HKCU\SOFTWARE\Tencent\WeGame` 的 `InstallPath`，检测存在的 `WeGame.exe`；这两个键的真实安装适配仍待 Windows 核实并回填。未宣称已核实，不猜协议参数。
- 日服本人关闭后立即消失、重连重新加载；搜索玩家保留。
- 日服本人日志 `selected=riot` 且不再出现 PUUID 400；无 Key/限流/超时仍可能走 LCU。
- 国服空组可切换且显示纯净入口与 WeGame；外服只显示 Riot；搜索按钮文字正确。
- 关闭时不闪启动遮罩；读取新计时事件验证各段真实耗时。连上到首屏约三秒的目标尚无真机证据。

验证产物必须为 **public**（空内嵌 Riot Key），名称含 `-public`；用户自行打包，本单不生成安装包。
