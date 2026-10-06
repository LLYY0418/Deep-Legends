# R230 执行账本

日期：2026-10-06（北京时间）。执行人：Codex。

状态：P1–P12 本地实现、最终自动验收与 24 张截图完成；Windows 真实客户端验收仍待完成。

## 范围与版本

- 按 [R230 工单](WORKLIST-R230-OVERVIEW-RETURN-BLANK-CAREER-SEASON-STUCK-91-GAMES-CHAMPION-TABLE-MASTERY-PAGE-ATLAS-EMPTY-ARENA-SQUAD-AFTER-RESTART-AND-UI-POLISH.md) 执行。
- 基线与当前版本均为 `desktop/package.json` 的 **0.12.74**。本次没有请求发版，未占用新版本、未提交或推 tag、未生成安装包。
- 工作区原有 R229 改动继续保留；本账本只描述 R230。构建与最终检查覆盖两单共同的当前工作区源码。

## 逐项结果

| 项目 | 实现与证据 |
|---|---|
| P1 总览返回空白 | `resetOverviewRenderCache` 与页签缓存共用字段清单；详情渲染前清缓存；左栏复用额外要求 DOM 存在。主总览和玩家浮层分别连续三次进出数据表/熟练度，卡片数量、唯一左栏与返回滚动位置均保持正确。 |
| P2 赛季坏缓存 | schema **12→13**，拒绝旧版完整缓存并重新后台回补。比较 SGP 上游消费条目数与实际解析数；丢失时记 `season_scan_parse_dropped`，保留原偏移、`Complete=false`，不跳过坏页。干净终页或真实赛季边界才完整。完成事件立即刷新、不等待 100 场或十秒节流。 |
| P3 英雄名称/分母 | 数据表、本地及 Riot 熟练度、OP.GG 数据表、好友展示改用总览中文目录；无 Riot provider 时也使用内嵌目录。皮肤表为空仍返回中文名称，熟练度分母来自目录（当前内嵌目录 173 个英雄）。空源按接口去重记录 `champion_names_empty`。`a.championNames()` 仅保留在目录合并内部。 |
| P4 数据表 | 三页签；默认单排，整个赛季单排零场且灵活有局时优先灵活，赛季段位总场数优先于近期样本。相同场次按胜率/KDA 排序；有对位的整行点击/Enter/空格切换、`aria-expanded`；删除按钮列、同步 colspan；对位与更多按钮缩进。胜率/胜场绿色、负场灰色；指定十列主行前三个不同的非零数值高亮（并列一起）。返回按钮 32px/14px，图标 20px。 |
| P5 熟练度 | 头像 72px、网格同步扩大；等级在徽章下方。标记使用自绘 SVG 菱形（灰描边/主题色实心），不使用 OP.GG 图片。提示为 S 方块及里程碑，保留进度、点数、最近日期。真实 Chromium 测量徽章底部 ≤ 等级顶部。 |
| P6 标题与顺序 | 标题右侧 inline-flex 居中；箭头 24px/700、28px 点击区域，无边框/背景，悬停仅颜色。顺序为英雄胜率→熟练度→位置偏好，生涯弹窗同步。 |
| P7 战绩标签 | 与装备同排；空间不足时整组左对齐换行，装备槽不缩小。深浅主题、1280/1000/780 测量装备组宽度始终 235px；1280 和 780 同行，1000 因左栏占宽整体换行。 |
| P8 构建切换栏 | 上下 margin 均 12px，顶部 1px 分割线；斗魂分组下方留足空间；用户后续要求去除切换头像下方等级数字，已从普通和斗魂共用渲染中删除。截图覆盖普通构建及 16 人斗魂构建。 |
| P9 图鉴重入 | 已保存的 atlas 视图在渲染时自动请求目录；空数据不会显示“没有匹配”。保留搜索/品质/选中项的会话状态，重新装入选中详情；请求 token 防止旧模式响应污染新模式。初始化与跨模式返回均有自动请求护栏。 |
| P10 小队恢复 | `arena-squad.json` 0600 原子写入最多三人身份、gameId、时间、加盐账号/大区 scope；同局、同账号/大区且两小时内才恢复。选人 gameId=0 时在真实 ID 出现后补绑定。异局/过期/异账号记录删除；明确清理/离开游戏删除磁盘记录，连接引用重置保留短期记录供重启恢复。成功记 `arena_squad_restored`，写入/删除失败有诊断。隐私 API、README 和写入点护栏同步更新。 |
| P11 名字复制 | 浏览器→Electron 受信主窗口/主 frame IPC→execCommand。成功/失败都有提示，3 秒消失；所有 `data-copy-summoner` 共用。`summoner_copy` 只含结果、方法、受控错误名，不包含名称；IPC 限长并拒绝其它窗口/frame。 |
| P12 加载/性能 | 四张卡首次带数据记录 `overview_card_ready`，含耗时及 snapshot/network/opgg 来源。普通导航、后台刷新保留 SGP 历史缓存；用户刷新及新局核验走 fresh。修复 force 原先无条件清历史缓存的路径。现有 OP.GG 并行请求与先显示缓存逻辑保留并回归。200 场真实 Chromium Performance trace 的长任务从 272ms 降至 **58ms**，见下文。 |

## 性能与截图

- [全部截图与测量](history/reports/r230/)：实际打开应用 `?demo` 入口，加载生产脚本/CSS，通过 UI 点击详情、返回、模式切换、构建页签，并聚焦熟练度卡片生成提示框。**macOS Chromium、本地数据/头像夹具**，界面自带演示角标；不是 Windows/LCU 实机证据。
- 每个主题包含：数据表 1280 展开对位、熟练度与提示框、返回后的总览左栏、标题栏、战绩卡 1280/1000/780、普通和斗魂 16 人构建栏、单双排切回图鉴。头像为明确的夹具，熟练度徽章使用既有客户端资源样本。
- [layout.json](history/reports/r230/layout.json) 包含几何与渲染计时；[performance-summary.json](history/reports/r230/performance-summary.json) 包含 200 场压力场景。
- [优化前 Performance trace](history/reports/r230/performance-trace-before.json)：200 场列表首次加载，`RunTask=272.491ms`；页签滚动控件 rAF 的布局读取触发 `Layout=200.26ms`。
- [最终 Performance trace](history/reports/r230/performance-trace.json)：视口外卡片通过 `content-visibility:auto` 延后布局/绘制，保留自动记忆高度；分享图强制 `content-visibility:visible`。最长任务 58ms，总览 body 最长约 54.5ms；常规 17 场操作没有 ≥50ms 长任务。
- `overview_card_ready` 的 network 表示本次总览响应，snapshot 表示赛季快照/已加载页面，opgg 表示 OP.GG 汇总；不宣称每一个下游请求都发生网络下载。
- Windows 日志中的原始 461ms 事件没有现场 Performance 记录，因此不能断言与本次复现同一调用栈；Windows 仍须复测 ≤200ms。

## 回归与修正记录

- R230 Go 专项覆盖：schema12 假完整回补、50 条解析一条拒绝完整、赛季边界/干净终页、两个接口空收藏名称/分母、同局/异局/过期/跨账号小队、普通缓存与手动 fresh、真实缓存命中/字节下降、安全诊断字段。
- Node 专项覆盖：三队列、排序/整行键盘展开/并列前三高亮、零值/总行排除、三级复制/3 秒反馈、完成事件、图鉴自动请求、四卡计时、赛季默认队列、IPC 受信窗口与 frame，以及主页面/浮层连续往返。
- 首轮全量抓到“显式清小队后被磁盘记录恢复”的回归，已修复：明确清理删除文件，连接引用重置保留供恢复。新诊断合法载荷补入通用白名单测试；专项已通过。
- 收尾另补不完整头页且回补偏移为 0 的恢复边界：扫描遇到已知对局仍继续推进，避免停在原点。增加专项护栏；schema12 回补夹具使用两页各 50 个独立对局，验证前台预算耗尽后的真实后台回补。
- 首轮 Node 与 Go/race/构建同时运行，有挂载超时及六个文件超过 90 秒预算；没有作为验收通过。保留 `*-first.log/json`，最终 Node 单独复跑。

## 最终验证

下表为追加恢复边界后的全量验证，生产源码指纹 `c5aa393af4f8`。用户后续头像数字调整及最终指纹见下文。Go/Node 最终复验及 public 后端构建全部通过。时间为北京时间。

| 检查 | 开始 → 结束 | 耗时 / 结果 |
|---|---|---|
| `go test -count=1 ./backend` | 02:23:44 → 02:25:42 | 117.734s，退出 0；包内报告 108.885s |
| `go vet ./backend` | 02:27:27 → 02:27:28 | 1.195s，退出 0 |
| 本单 `-race` 专项（完整筛选见 go-race-final.json） | 02:23:49 → 02:24:26 | 36.185s，退出 0，无 race 报告 |
| `node scripts/test-renderers.cjs all`（包含 backend/web、desktop、scripts） | 02:27:52 → 02:29:12 | 79.513s；1290 PASS、0 FAIL、4 平台 SKIP；最慢文件 44.613s，所有文件 ≤90s，全量 ≤240s |
| `desktop/overview-render.test.cjs` | 包含在上面的最终全量 | PASS，17.752s；此前独立运行 10/10、29.113s |
| 六个改动的生产 JS/CJS `node --check`、`git diff --check` | 最终源码复核 | 退出 0 |
| `node desktop/r230-layout.cjs` | 最终前端源码 | 24 张实际应用深浅主题截图、几何护栏与 200 场 Chromium Performance trace PASS |

Go 全量最后五行（该轮实际只有一行）：

```text
ok  	lol-loot-assistant/backend	108.885s
```

原始输出、北京时间起止、耗时、退出码见 `history/reports/r230/*.log/json`；Node 文件耗时与完整计数另存 [node-all-summary.json](history/reports/r230/node-all-summary.json)。

## 构建

- key mode：**public**，无内嵌 Riot Key；临时验证文件名均带 `-public`。只构建后端，没有安装包或发布。
- 最终指纹 **`59cb5070cc47`**（包含后续头像数字与二级表格箭头调整）；Windows/macOS 后端都重新构建并通过 `desktop/verify-build-fingerprint.cjs` 核对。版本仍为 **0.12.74**。
- Mac 隔离目录 `-self-test` 退出 0，输出 `Deep Legends 0.12.74 自检通过：奖池 554 条`；`-self-check-riot-key` 按 public 预期退出 1并报告未内嵌 Key，此项判定通过。
- 临时验证产物（不覆盖正式桌面 payload）：`/private/tmp/deep-legends-0.12.74-r230-public.exe`（Windows）与 `/private/tmp/deep-legends-0.12.74-r230-public`（macOS）。SHA-256、体积与核对结果见 [build-validation.json](history/reports/r230/build-validation.json)。Windows 二进制只做交叉构建/指纹核对，没有 Windows 执行证据。

## 用户后续：去除构建切换头像数字

- 2026-10-06 用户截图指示：去掉构建下方所有英雄切换头像底部的数字。普通对局及斗魂分组共用 `renderBuildPlayers`，已删除等级 `<small>` 和对应 CSS；小队名次、头像选中态及自己的标记保留。
- 生产 JS syntax、`git diff --check`、现有 R211 构建切换/键盘护栏及 R117 样式护栏通过（15/15）。没有后端源码改动，前述 Go 全量记录保留。
- 修正截图脚本将普通大乱斗 `90007` 误标为斗魂的问题，改用斗魂 `90004`。原目录带 `arena16` 名字的截图不作为斗魂证据，以本次后续目录的正确截图为准。
- 真实 Chromium 深浅主题普通/斗魂切换栏断言 `.build-player small` 数量为 0，截图见 [后续截图](history/reports/r230/avatar-cleanup/)，运行记录 [avatar-cleanup.log](history/reports/r230/avatar-cleanup.log)。
- 最新指纹 `bc301c268a78`，0.12.74 public Windows/macOS 后端已重建并核对；macOS 隔离自检通过。原指纹的构建记录保留为 `build-validation-before-avatar-cleanup.json`。

## 用户后续：二级表格箭头贴近头像

- 2026-10-06 用户要求：二级表格箭头贴着英雄头像，左侧留空隙。箭头从排名列移入英雄单元格，排名列留空；英雄单元格保持 32px 左侧缩进，箭头与头像间距 6px。
- 现有 R230 数据表与 R117 样式护栏 12/12 通过；生产 JS syntax、`git diff --check` 通过。真实 Chromium 深浅主题测量：箭头距单元格左侧 34px（含边框）、距头像 6px，排名列为空。截图见 [表格箭头调整](history/reports/r230/table-arrow/)，记录 [table-arrow.log](history/reports/r230/table-arrow.log)。
- 版本仍为 0.12.74，最终指纹 `59cb5070cc47`；public Windows/macOS 后端重建及指纹核对通过，macOS 隔离自检通过。没有后端源码改动，前述 Go 全量记录保留。

## 仍待 Windows 实机验收

1. 旧版本升级后，真实账号等待整季回补，核对单双排+灵活+海斗场数以及更早英雄；不能把夹具数据当成真实总场数。
2. 斗魂选人进入游戏、退出并重开应用后，自己的小队仍在前三；其它小队归属仍不推断。
3. 名字复制到实际系统剪贴板与三秒提示；本机没有用户的 Windows/LCU 会话，无法确认原故障到底是窗口焦点、遮挡还是剪贴板权限。源码确认旧路径静默吞错，现已补诊断及降级；原 Windows 原因仍未证实。
4. 真实 Windows Performance 复测与各页视觉验收。未打包、未发布，工单保持待真机验收，不关闭。

## 0.12.75 发布后续（2026-10-06）

本单共同源码已随 0.12.75 public Latest 发布，包含 R230 后续头像数字去除与二级表格箭头调整。发布 SHA `0e11afd6b9e057af9fdae6c479b56f1b4eb000a1`、指纹 `09d40c535496`；最终发布预检、同 SHA 完整 Linux/Windows CI、实际升级及匿名附件均通过。此前失败日志是历史轮次，不代表此次发布状态；真实用户 Windows/LCU 项目继续待验，不自动关闭。见 [0.12.75 发布账本](history/ledgers/release-0.12.75-execution-ledger.md)。
