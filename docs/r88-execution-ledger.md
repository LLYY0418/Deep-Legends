# R88 执行与验收账本

2026-09-14。最终源指纹 `7ce0204bb969`。代码修改与自动化验收完成；实际对局提速、A7 归因等待新 build 的真机样本。

保留了 R87、打包格式扫描修复和好友横条恢复等原有未提交改动。执行前基线位于 `/tmp/r88-baseline`；不依赖该临时目录运行新的回归护栏。

| 项目 | 已实现行为 | 证据 |
|---|---|---|
| P3 前端请求量 | live 诊断每秒最多一次，live/本地耗时采样共用一个在途槽位，不重试；同 phase 不重复记录；queue 只在首次排队记录。耗时每十秒最多采样一次，包含端点类别、开始/完成与耗时，剔除 URL/身份信息。 | `web/r88.test.cjs` 模拟 1 秒 13 个 ChampSelect 事件，实际诊断 fetch ≤2；慢端点无堆积、无重试；恢复直接 POST 的变异失败。 |
| P3 状态连接 | 保留 8 秒超时，连续三次失败才整页错误；前两次静默重试，成功清零，取消/被替换请求不计入。旧重试定时器先清除。本地助手无响应与未检测到英雄联盟客户端文案分开。 | 使用真实 api 函数和 AbortController 的连续超时、恢复、取消测试。 |
| P3 后端缓存 | live 普通加载也写缓存，20 秒内复用；ChampSelect 为 3 秒。命中校验 gameId；首次消费后台预热帧才起算 TTL，后续命中不延长。客户端、身份、阶段与取消保护保留；交错校验不能返回已替换快照。斗魂分组未齐不缓存，继续原有重试。 | R87 隐藏/串局测试保留；R88 多次命中、过期、交错校验及 race；R62 17 人分组重试回归通过。 |
| P3 身份查询 | 确切 LCU `/lol-summoner/v2/summoners/puuid/{id}` 原先无跨周期结果缓存。新增按 LCU 客户端隔离的 5 分钟缓存、256 项上限及同身份 singleflight，失败不缓存；保留 summonerId 回退。 | 18 个同身份并发与多次连续查询只读一次；客户端隔离和 TTL 过期测试。R69 十人固定 fixture：冷读 30 次，TTL 内再次读的身份/排位/历史请求为 0。 |
| P2 禁用 | 本地进行中的未完成 ban 返回 `[-1]` 时，任意队列可使用 fresh grid / 同会话证据提议正 ID。保留禁用/已选/意向过滤，强制先 hover，确认后才能 lock。所有来源前缀同步为 wildcard，包括执行与界面状态。 | 1750/3110/3100 在 lock-now 策略下均先 PATCH hover、再确认锁定；空列表、读失败、非本地回合等原有安全测试保留。 |
| P2 队列护栏 | champselect 内所有整型 queue 比较移至 `queue_groups.go` 登记/兼容辅助函数。3110 分组与 legacy 探测保持原语义，未泛化 legacy。R88 覆盖整数比较、绑定别名及 switch-case；R89 追加数值字符串、字符串化、转换/包装及比较 API，边界见后续账本。 | 原三种变异继续被拒绝；新增等价写法与合法查表反向测试见 [R89 账本](r89-r88-followup-ledger.md)。`raw_head` 最多保存三个原始 ID。 |
| P4 斗魂榜 | OP 保留为 S 之上独立源档位，复用金色 OP 徽章。单行类型、未知 tier、重复 ID、无效数值等跳过；顶层结构/元数据错误或无有效行才整表失败。顺序异常记录但保留上游原始排名；不重新按胜率排序。 | 派生 OP fixture 173 行通过；XYZ 单行跳过、返回 172 行并产生带 field/value 的单条汇总诊断；多种坏行测试。原 HTTP 失败路径保持。 |
| P1 名单 | 大屏固定 370px；普通两列 1fr、14px 间距；斗魂每队三列 1fr，名单只隐藏 tag，tooltip 始终提供完整身份，遮罩仍有效。普通 116px、斗魂 118px。 | 真 Chromium 在 1920/2560/3840 测量两组各八张卡。23 个用户提供的可见斗魂名字零截断；普通 ≤8 字中文名零截断。1920 左/右缘严格 1424/1794。 |
| build 标识 | 更正：R88 当时只覆盖经过 `appendDiagnosticEvent` 的事件，遗漏早期 provider、启动 stage 与存储轮转标记。R89 已将标记下沉到 `marshalDiagnosticRecord`，覆盖后端 `diagnostics*.jsonl` 的所有新增记录；历史归档不改写。 | R88 导出契约与 marker 变异保留；完整入口清单、真实解析/启动落盘及轮转验证见 [R89 账本](r89-r88-followup-ledger.md)。 |
| A4 | Electron renderer/child process 退出记录有限 reason/exitCode/type；uncaughtExceptionMonitor 同步记录有限异常类型，保留默认异常退出行为；app_start 增加 log_rotation。 | 真实主进程函数的事件注入及导出测试；独立 Node 子进程验证 monitor 后仍非零退出；移除崩溃 hook / 轮转标记的变异失败。 |
| A5 | live 与 claim 使用分开的字段白名单；live 不再出现虚假的 claiming/done/total/duration_ms。transport_suppressed 来自运行时发送的实际计数。 | HTTP handler 到日志导出的契约测试，伪造零耗时变异失败。 |
| A6 | desktop 五代日志导出时统一过滤最近七天，保留边界当天，输出过期计数；仍遵守文件信任、体积和隐私白名单。 | 五代中陈旧错误过滤、近期 crash 保留及去除时间窗的变异测试。 |
| A7 | 已明确记录证据不足，不声称 position-broadcast 原因已查明。 | 待新 build 多局大乱斗日志。 |

## 自动化结果

- Go：`go test ./...` 通过；相关新旧缓存、禁用、解析、诊断测试 `go test -race` 通过；`go vet ./...` 通过。
- 前端/桌面：完整 `node --test web/*.test.cjs desktop/*.test.cjs` 621 通过、0 失败、1 跳过（需要 Windows/PowerShell 的发布脚本实际失败门禁）；后续连接文案与最终定向测试 24 通过。
- Windows amd64 后端交叉编译通过，验证产物为 `/tmp/r88-loot-service.exe`。没有生成新的完整安装包。
- 安装器 `go test ./...` 与 `go vet ./...` 通过。初次因隔离缓存缺少 WebView2 源码而失败，补齐 go.mod 固定依赖后重试通过，未改依赖版本。
- 19 个隔离生产回退变异全部被拦截，见 [mutations.json](r88/mutations.json)；脚本 `scripts/r88-mutation-check.py` 使用 Go overlay / 临时 JS 副本，不改生产工作区。
- 真 Chromium 基线通过；420px 和旧 max-content + space-between 两种变异均失败。报告见 [layout-summary.json](r88/layout-summary.json)、[完整测量](r88/roster/baseline.json)、[混排名单截图](r88/roster/baseline-mixed.png)。截图使用演示数据和静态测试服务，外部英雄图像未加载，不作为图片资源验收。
- [protected-layout.diff](r88/protected-layout.diff) 为 0 字节；对应 before/after 文本同时保存在 `docs/r88/`。R88 删除三个死 arena-first 容器内部规则，所有有效 `.match-main` / `.match-stats` 规则逐字一致。更正原保留理由：`gameplay.css` 剩余的 640px 规则同样没有匹配的 DOM 祖先容器，不能用于英雄详情，已在 R89 删除。R89 执行时 `champions.css` 有 `arena-first` 容器声明，但没有工单提到的同名 640px 规则；整个文件保持逐字不变。前后截图与真实交互证据见 [R89 账本](r89-r88-followup-ledger.md)。
- `git diff --check` 与构建脚本 Bash 语法检查通过；原 `.gopath` 格式检查排除规则保留并通过回归。

## 样本与结论边界

- 本次 YOUR.GG 实抓为 44,907 字节、173 行、首档 S，档位分布 S6/A29/B52/C41/D39/F6，已经不含 OP。保留原始快照及 SHA-256；OP fixture 仅将首行档位改为 OP，其余字段不变。未将其冒充工单当时的 44,951 字节响应。详见 `testdata/r88/README.md`。
- 23 个斗魂名字来自用户补充。五个 tag 后半段为用户明确标记的猜补；名单宽度验收使用真实可见名称，tooltip 验证完整传递机制，不声称这些编号是真实账号。
- 工单历史日志统计须按 build 拆分：旧 `1409a066c79f` 7,588/9,934 行（约 76%），新 `eae1f783b367` 约 19 分钟。这里引用的是工单证据，不是本轮重新采集的数据。
- 高频前端诊断确实可从代码和突发测试复现；HTTP/1.1 连接槽位挤占导致 8 秒超时仍是机制推测。新增本地请求时刻与耗时采样用于下一轮验证，不能以模拟耗时证明真机提速。
- 新 build 的 position-broadcast 只有 1 次失败（session-read-failed）和 2 次成功，不能归因。需要在本轮新 build 多打几局后重新导出。
- A4 的“崩溃重启”同样未证实。工单给出的 15:16:37.337 → 15:16:40.215 实际相差 2.878 秒，与文中“24 秒”不一致；不依据该文字推断崩溃原因。
- 旧 build match-history 17–28% 的超时/网络失败是已存在的另项问题，本轮未声称解决。
