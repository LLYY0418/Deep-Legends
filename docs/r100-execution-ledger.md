# R100 执行账本（P0–P7 已实现并通过自动化验收）

顺序：P0 → P1 → P2 → P4 → P3 → P5/P6/P7；P8 可选。保留已有 R95–R99 工作，未提交。

## 歧义与边界（改动前记录）
- P0：总览与后台种子使用单次额度准入上界；FIFO 排队及队首等待共同算一次准入，下一次 HTTP 准入重新计时。specialistStepContext 本来就是独立 step 的总预算，保留旧 withRiotQueueLimit（绝对时间）并用不同函数名区分。真实 context deadline/cancel 始终有效。
- P0/P4：成功详情原本已经在 matchByIDWithCache 返回前写入内存；本轮首先加取消回归证明，不把尚未成功的网络请求伪造为成功，也不无限延长用户取消的 I/O。
- P1：后端 semaphore 不能限制浏览器已打开的连接，不能冒充前端队列；h2c 也不能假设 Chromium 自动支持。采用前端资产队列 + handler 整体超时 + 状态局部降级。WriteTimeout 必须为 SSE/NDJSON 长流单独解除写 deadline，不能以保护图片为名打断总览流。
- P2：ddragon 符文与 cdragon 海克斯不是镜像，不竞速互相替代。
- P3：工单说目录正文已有磁盘缓存，但当前 fetchProDirectory 明确不落盘。允许本轮新建有界24小时公开职业快照；不把私人 LCU 数据写入该快照。R97 六队范围不扩大。
- P7：日志和分享图独立设置。目录选择仅通过可信主窗口 IPC；自动保存写入时重新校验，保留日志追加 O_NOFOLLOW/inode/dev 保护。
- 不上调15/90，不改20场，不改走OP.GG战绩。主机拆窗是允许项而非本轮必需，暂不扩大限流改动。P8 按工单许可明确延期。

## 实施与证据

### P0 — 单次准入，不是整轮墙钟
- 总览 `withRiotSingleWaitLimit(5s)`；后台种子 `100ms`；specialist 仍用原 step 总预算。限流值仍 15/秒、90/2分钟，默认20场。
- `TestR100OverviewLongRoundUsesFreshSingleWaitBudget` 真等5.2秒后继续额度准入，20场全成功且缓存20场，无本地限流事件；取消后的成功响应缓存由 `TestR100SuccessfulDetailSurvivesCallerCancellation` 实证。
- `TestR100QueueCallersKeepDistinctSemantics` 同时检查真正的 resolveProSeed 调用、过载429、specialist deadline 与两种 context key。

### P1 — 图标连接池与局部恢复
- 图标 handler 整体2.5秒；image/* 上游单请求3秒，元数据原12秒客户端未粗暴一起缩短。
- 本地候选优先；远端按彩色 large/无后缀一组并发竞速，全部失败才进入 small 一组并发，**共享一个2.5秒 deadline**。这是对“竞速第一个成功”的质量约束：不能让快返回的小图吞掉已有彩色图优先级，也不再串行6×12秒。
- 全局图片队列最多2个在途，延迟图片需进入可见区域；失败同URL不自动重试、跨元素60秒失败冷却。动态英雄、符文、头像、好友、奖励、背景及 detached 预取均接入。打包本地图标/视频不伪装为远端图标。
- `/api/status` 失败只出现顶部恢复提示，恢复成功自动隐藏；已加载内容不清除。**补漏：桌面主进程确认后端异常退出后，通过独立 IPC 触发全页告警与软件重启入口，详见下方追加记录。** HTTP WriteTimeout=30秒，SSE与180秒总览NDJSON显式解除该写deadline。
- 真 Chromium HTTP/1 fixture 使用生产图片队列和生产 refreshStatus；12个图标各挂起12秒，状态接口仍小于8秒、峰值图标连接2，连续三次503不全页崩溃，恢复后提示消失。日志 `p1-browser.txt`、`mutations/P1-six-image-sockets-baseline.txt`，截图在 browser/ 和 mutations/*-baseline-browser/。
- DOM 队列测试额外覆盖 detached 预取、失败不重试、成功图片重挂以及 dispose 清理。

### P2 — 符文先返回，可选海克斯异步补充
- perks基础目录归一化后落盘；缓存读/网络加载不持有 perkCatalogMu。海克斯独立后台3秒任务、按key互斥、失败一分钟退避，完成后前端短轮询停止。
- pending只存在**响应值副本**，从未写入缓存 payload；独立核验确认不需要在缓存清理一个不存在的 pending 标志。
- `TestR100PerksNeverWaitForOptionalAugmentsAndPersist`：cdragon模拟12秒挂起，基础响应及8个并发请求均<2秒；TryLock证明锁未覆盖可选I/O；重启从磁盘读，基础loader不执行。

### P4 — 背景结果不取消前景请求
- historical-ranks广播遇在途loading/loadingMore只记录待处理；当前token finally完成后再处理，不用后台补充去abort用户正在等的总览。
- summoner/ranks/masteries各自ready channel同步发布；progress尽早携带已就绪数据，末尾再补一次。`TestR100ProgressPublishesReadyMetadataAndQueueDiagnostic` 验证真实Go路径。
- 前端逐字段过滤null/undefined，profilePending时零等级/头像不覆盖已有值。**保留R95更严格的数组语义**：progress的空数组仍不擦旧生涯，complete显式[]才清；这不是把P4“非null可更新”解释成允许旧wipe回归。`web/r100.test.cjs` + 原R95测试共同守护。

### P3 — 职业快照与ladder缓存
- 已发布职业目录+补充状态+排名字段有界24小时磁盘快照；进程重启优先恢复。内存正常刷新5分钟策略保留，不把读取时间当新抓取时间延寿。
- 原上游结构若直接JSON会丢 `json:"-"` 的排名/补充字段；使用私有缓存DTO保留，HTTP公开结构不扩张。
- ladder复用 provider 的 fetchWithMetadataCacheKeyLoader 缓存层、TTL24小时；不直接改成普通fetch而丢失原有无cookie/严格redirect URL校验。重启两次只产生一次HTTP测试通过。
- R97管理页六队/非学院范围、职业身份匹配范围不变；不引入新战绩来源。职业目录和perks上游本来主要是OPGG/CDragon，不能把它们一概说成争抢Riot的同一配额桶。

### P5 — 中文标签三层修复
- fallback gameMode中文表→其他模式，英文LCU条目不透传；九个Arena ID均注册、进入筛选且有squadSize。
- KR列表及详情复用LCU队列目录，新增 riot_match_mode 的 queue_id/game_mode/queue_label 事件。
- 原始公开队列名称存 `arena-queue-source.json`：1701/1731/1732 的1v0登记size1，1704明确2v2，1750保持3；1720/1740采用传统双人Arena size2（非实机证实），**不据此恢复任何玩家顺序切块推断**。
- 公开目录来源：https://raw.communitydragon.org/latest/plugins/rcp-be-lol-game-data/global/default/v1/queues.json 。`TestR100QueueLabelsAndArenaRegistry` 验证注册、筛选、中文兜底、客户端覆盖；原始mode回归变异被杀。

### P6 — 七项风险逐条
1. diagnostics-1042旧断言更新为国服/韩服/职业三组常驻、disabled=false；旧断言直接与新工单相反，不删除测试。
2. 空组选择显式更新activeGroup并渲染菜单和该组空态；不是依赖不存在页签的副作用。
3. visiblePlayerGroups总返回三组；空活动组不被renderPlayerTabs踢回国服。
4. 职业自动晋升、openPlayer/openPlayerByRiotId原切组行为保留；原跨组隔离、目录返回/焦点/滚动DOM用例继续验证。
5. activeTab=null由原守卫和专属空态处理；新增DOM用例按kr→pro→kr→players连续选择并检查错误数组为空。
6. 断连时国服隐藏/清理非current非riot页签的两条逻辑未删；国服启动卡保持原样。
7. 不再CSS整体隐藏player-workspace-head；职业空态返回目录按钮仍可点击，新增DOM用例实际点击并返回。
- 同步更新R74键盘用例：ArrowDown进入空韩服，异步重绘仍保持韩服焦点；不再跳过零页签组。独立真实浏览器布局helper的相同预期同步，几何约束不变。

### P7 — 日志独立目录与写入时复核
- 新 diagnostics-export.json / diagnosticsSaveDirectory，与分享图saveDirectory分离；首次native Save dialog默认下载文件夹，成功后记忆；后续自动保存；设置页有对等读取/选择入口及云同步提醒。
- 新IPC同时验证主窗口sender与可信renderer；preload不接受任意路径参数；URL链/下载sender原门禁保留。
- 自动下载先进入userData私有staging；实际提交时realpath、非symlink、dev/ino再次校验。目录删除/替换重新选择；O_EXCL+O_NOFOLLOW防同名覆盖。原桌面日志追加O_NOFOLLOW及inode/dev校验块保持不变。
- 完成失败、取消、SavePath异常/相对路径、窗口销毁均清理staging；测试覆盖首存/重启/第二次不弹/目录替换/冲突/foreign renderer/symlink/清理。新控制器已列入Electron打包files；原NSIS toolsets配置保留。
- 边界：本机恶意进程能并发重命名目录；Node路径API不能提供完整openat目录句柄事务。当前按写入前后身份检查及独占文件句柄防覆盖，不宣称对拥有相同OS权限的恶意进程完全隔离。

## 旧基线改动与验证中发现的问题（不以删断言换绿）
- 图标候选旧测试绑定“串行恰好3次/固定路径顺序”；改为并发允许候选集合与有界次数，保留真实图片字节、状态码、诊断脱敏和彩色优先、不触发small的关键断言。fixture路径记录加锁。独立small成功fixture仅允许规范路径成功，避免两个成功镜像造成index断言随机性。
- web/r88原第三次status失败应fatal改为不fatal；真Chromium另断言恢复条出现/消失及内容保留。
- 原perks断言仍检查三个LCU端点一次，但等待后台任务后再检查，handler本身不等待。
- 图片HTML契约从src改成data-queued-src：diagnostics-2244、champions、R64测试验证相同目标URL/图像选择语义。R64旧演示fixture所有拉克丝皮肤都用同一头像，现fixture每皮肤有独立splash URL，保留“切换必须改变URL”断言，避免比较浏览器代理重写副作用。
- web/champions的缓存调用源码断言新增r.Context参数；refresh-orchestration提取函数harness注入window，业务断言不变。
- social未知ID返回“模式 99999”更新为工单强制“其他模式”；仍不按可能过期queueType伪装已知模式。
- diagnostics-export内嵌变异的声明锚点同步增加stagedFile；该旧变异仍实际执行而非跳过。
- 独立探子发现并修复头像picker双data前缀、好友图标漏队列、completed SavePath异常漏清理；主代理发现并修复detached预取无法入队。前探子把pending副本误判缓存污染，经亲读和二次独立核验否定。

## 可选项及非本轮验收
- P8按工单明确允许留到下一轮：facade新鲜度守卫及两类日志采样未改。不是遗漏，也未声称已修。
- 本轮未执行真实LOL客户端在线20场搜索、Windows EXE安装/运行；浏览器证据为真实Chromium配受控HTTP fixture，不冒充用户现场。
- R99未完成的真实客户端门禁仍按原账本保留，本轮不替代R99验收、不擅自提交或打包发布。


## 最终验收结果（2026-09-17）

所有命令真实执行，日志保存在 `docs/r100-validation/`：

| 命令 | 最终结果 | 完整日志 |
| --- | --- | --- |
| `GOCACHE=/tmp/deep-legends-go-cache GOTMPDIR=/tmp/deep-legends-go-tmp go build .` | exit 0 | go-build.txt |
| 同环境 `go vet .` | exit 0 | go-vet.txt |
| 同环境 `go test -count=1 -v .` | PASS，109.816s；1185个顶层通过，17个原有联网/本地capture可选测试跳过 | go-test.txt |
| 同环境 `go test -race ./...` | PASS，138.462s，无race报告 | go-race.txt |
| `node --test web/*.test.cjs desktop/*.test.cjs` | 707测试：706通过、0失败、1跳过；230.332s | node-test.txt |
| `node desktop/r100-browser.cjs` | 真Chromium PASS；状态1.8ms，图标峰值2 | p1-browser.txt、browser/result.json |
| `python3 scripts/r100-mutation-check.py` | 10/10真实杀死；每项基线exit0、变异exit1；无编译/语法失败或测试超时冒充kill | mutations.txt、mutations/matrix.json及逐项完整日志 |
| `node --check`（改动主JS/桌面入口）及 `git diff --check` | PASS | syntax.txt、diff-check.txt |

Node唯一跳过项为原有Windows/PowerShell发布门禁，在macOS不能执行；没有新增skip。Go的17项为原有显式opt-in在线或capture测试，本轮未启用真实账号联网。旧失败输出另保留 `go-first-regression.txt`、`node-first-regression.txt`、`node-second-regression.txt`；中途日志不是最终交付状态。

变异明细：P0绝对整轮deadline；P1图片并发6、状态失败fatal；P2同步等待可选海克斯；P4 null覆盖、背景直接重载；P5原始mode兜底；P7不持久化、信任绕过、取消inode校验。均由实际行为断言失败杀死，不是仅搜索源码判定。

新增回归文件：`r100_test.go`、`web/r100.test.cjs`、`desktop/r100.test.cjs`；真实浏览器脚本 `desktop/r100-browser.cjs`；执行器 `scripts/r100-mutation-check.py`。P6另在既有 `desktop/pro-players.test.cjs` 中增加真实DOM场景，保留原账号/焦点/滚动等断言。

最终保留用户原NSIS toolsets=1.2.1；desktop/package.json本轮只新增diagnostics-directory.cjs打包项。未commit、未push、未创建分支、未自动发布。`status-before.txt` / `status-after.txt` 记录共享工作区既有改动；`changed-files.json` 为相对本轮开始快照的既有文件差异清单（不包含新文件及后来更新的说明文档）。


## P1 追加修复 — 确认后端退出才显示全页告警（2026-09-17）

### 缺陷确认与边界
- 用户指出正确：上一轮只移除了 status 失败对 `showFatal` 的调用，没有实现“确认后端进程消失”的替代判据，导致该函数在生产代码中没有调用点。
- 桌面壳原有 `close -> failStartup -> native dialog -> quit` 尚在，因此不是完全没有进程退出告警；但这不等同于工单要求的页面全局兜底。
- 不使用连续失败次数、HTTP 超时、503 或 IPC 查询失败推断进程死亡。普通浏览器没有桌面进程桥接时继续局部降级，不伪造进程退出证据。

### 实现
- `desktop/main.cjs` 的 `onBackendClosed` 以实际 child `close` 为确认信号，保留等 stdout EOF 的语义，先让 `LOOT_QUIT user/update` 标记正常退出。正常关软件/升级不告警；尚无页面的启动失败保留原生错误框兜底。
- 已有主窗口时保留窗口，记录退出快照，通过 `desktop-backend-state` IPC 推送。`getState` 补读快照覆盖订阅建立前退出；只允许可信主窗口 renderer 查询/重启，不暴露 token 或进程句柄。
- `desktop/preload.cjs` 只暴露窄接口：状态读取、事件订阅/取消、软件重启。
- `web/app.js` 先订阅再读快照，确认 `exited` 才调用 `showFatal`，隐藏自动重试条、停止 status/SSE 重连、作废在途 status token。迟到的成功响应、旧 running 快照和额外 `renderStatus` 均不能清除确认退出的提示。
- 全页明确显示“本地数据服务已退出”和“请重启软件”，按钮经 IPC 调用 `app.relaunch()`，不是对已死后端做 `location.reload()`。IPC 重启失败时显示手动关闭重开的说明。dispose/unload 取消订阅。

### 验证（追加，不代替上一轮 Go/真实客户端验收）
- `desktop/backend-lifecycle.test.cjs` 执行生产 main、preload、renderer 函数的连通测试：连续 HTTP 失败不误报、close 全页告警、订阅前退出快照、迟到结果不覆盖、无桥接/查询失败降级、dispose、正常 user/update 退出、未授权 renderer 拒绝、重启失败说明。
- 定向 Node 测试 19/19 通过：`p1-backend-lifecycle.txt`。
- 全量 `node --test web/*.test.cjs desktop/*.test.cjs`：720 项，719 通过、0 失败、1 项原有平台跳过，约282秒；见 `p1-backend-full-node.txt`。全量运行启动后新增的重启 IPC 失败提示用例另已包含在上述19项定向复跑中。
- 真 Chromium HTTP/1 fixture：图片峰值并发 2，status 约 2ms；状态失败降级/恢复仍通过；使用生产 lifecycle/showFatal，在注入确认退出事件后出现全页告警，点击重启按钮调用桥接。`p1-backend-browser.txt`、`browser/result.json`、`browser/confirmed-backend-exit.png`。这里的退出事件和 Electron API 为测试替身，**不声称完成 Windows EXE 杀后端/重启实机验收**。
- 4 个变异均被断言杀死：移除 fatal 调用、移除退出推送、移除保留快照、绕过重启信任校验。见 `p1-backend-mutations.json` 及对应 mutation 日志。
- 本次只改 JS/CJS，未改 Go 后端；未重复宣称已重新运行 Go 全量验证。未提交/推送，也未更改用户已有 NSIS toolset 配置。
