# R86 执行账本

状态截至2026-09-13本轮本地执行。**不是全部外部验收完成报告**；“已落地”表示已实现且有相关护栏，最终全量验证和平台限制见下方。原始工单不改写，文档数字/实现建议与实际不符时以仓库和实验为准。

| 条目 | 状态 | 实现 / 证据 / 未完成部分 |
|---|---|---|
| P0-1 git 账本和工作区严重脱节（最高优先级，不修会放大后面所有改动的风险） | 已落地 | 7eb6393：工作区入账、构建产物移出索引；收尾提交后工作区干净，索引无exe/go-tmp构建产物。 |
| P0-2 CI 从来没有运行过一次 | 本地完成／待远端 | CI 已跟踪，独立 installer 门禁、动态版本已补；尚未推送，不能声称 GitHub Actions 绿。 |
| P0-3 Windows 发布脚本里"跑测试"是假门禁，红着也能发版 | 本地完成／待 Windows | 全部原生命令检查LASTEXITCODE；Windows专用真实t.Fatal/独立提前构建变异测试已补并接入现有JS门禁，当前macOS按明确平台条件跳过，待Windows runner执行。 |
| P0-4 macOS 发布脚本一个测试都不跑，而 README 说发版就用它 | 已落地 | macOS 脚本补全门禁；release-quality-gates.test.cjs 用真实 t.Fatal 验证构建前退出，独立提前构建旁路被杀死。 |
| P0-5 JS 测试套件现在就是红的：一条陈旧断言 | 已落地 | 修复陈旧 self tooltip 断言；测试 harness 正确先载入 runtime.js。 |
| P0-6 总览接口四段完全串行，其中两段根本不依赖战绩（战绩加载速度的最大单点） | 已落地 | b7262b6/072ee7e：独立上游并发，固定结果槽，150ms 注入与 phase 区间重叠护栏。 |
| P0-7 展开任意一场战绩 = 整个总览页 innerHTML 全量重建（前端最大单点） | 已落地 | f06979c：仅替换目标战绩卡；200 场 DOM 身份/createElement 护栏及独立旁路变异。 |
| P0-8 escapeHTML 有五份实现、三份用 createElement、两份不转义引号却被拼进属性 | 已落地 | 4d20f92：runtime 唯一纯字符串 escapeHTML，五模块共享，五字符与 Unicode/null 等价性。 |
| P1-1 每条诊断事件 = 2 次 Lstat + open + write + close，且全局串一把锁 | 已落地／已验证 | 64KiB 缓冲、内存字节计数、200ms Flush并释放闲置fd；导出/退出Flush，关闭失败有有限错误提示；万条 Lstat=3、log_seq=1..10000连续，独立stat旁路被杀死。 |
| P1-2 championDataCache 每次写盘都把整个缓存目录 ReadFile 一遍（最多 64MB），还是同步的 | 已落地／已验证 | Hexdata 前缀迁移和恢复保护，8MiB/5min异步节流；200文件/3保护记录、写入和prune读取载荷=0；独立读取旁路和独立删除恢复数据旁路均被杀死；迁移失败拒绝增长磁盘缓存。 |
| P1-3 三个 http.Transport 都没设 MaxIdleConnsPerHost（默认 2），championHTTPClient 还丢了 HTTP/2 | 已落地 | 32/8/90s 三处连接池，Champion HTTP/2；8 并发40请求握手≤10 和 H2 测试；保持 TLS/redirect 边界。 |
| P1-4 熟练度与队列名用 context.Background()，18 秒软预算和请求取消都管不到 | 已落地／已验证 | 补测发现 AllContext 内仍调用 GetBytes，已修为 GetBytesContext；queue/mastery分别永久hang、50ms deadline时总览<200ms；独立Background旁路被杀死。 |
| P1-5 SGP 战绩页缓存只按条目数淘汰（256 条），没有字节预算，估 80~130MB 常驻 | 已落地／已验证 | 24MiB+256双上限；真实300页×50请求验证累计字节守恒；后台10页499场完成且保留页0、不新增尾页缓存，独立后台缓存旁路被杀死。 |
| P1-6 Riot 对局详情只有 600 条内存缓存，没有磁盘缓存也没有 singleflight —— 而对局是不可变数据 | 部分完成／待隐私确认 | 进程内详情 singleflight、取消等待者测试完成；跨重启缓存未做：原始 PUUID/战绩落盘与 neverStores 冲突，需决定脱敏模型及声明。 |
| P1-7 自动"再来一局"三个阶段共用一个 3 秒去重键，最多迟 8.4 秒才点 | 已落地 | 更早 play-again deadline 重排，不延后、不重复；真实 POST 时序测试及旁路变异。 |
| P1-8 切换战绩筛选、以及韩服/职业玩家的外部战绩视图，同样整列表重建 | 已落地／已验证 | 按gameId隐藏/复用，缺项追加、内容变化局部替换；完整未筛选数据无需重请求，部分/服务端筛选保留原分页流程；200场全部→斗魂→全部节点相同且创建<200；外部单卡与独立重建旁路测试通过。 |
| P1-9 英雄页搜索每敲一个字都整页重建，且 championMeta 是 O(N²) | 已落地 | 数字首条优先 metadata Map、搜索缓存、150ms 防抖、局部结果渲染；170行/5键/input身份及独立旁路。 |
| P1-10 `/api/gameplay/phase` 每秒常驻轮询，与当前在哪一页无关，而 SSE 已经在推同一件事 | 已落地 | 可被 EventSource 观察的 heartbeat、45s健康窗口/12s低频与断线1s恢复；30s虚拟时钟、独立高频旁路。 |
| P1-11 进入"工具"页一次性拉全部 5 个子页签的数据 | 已落地／已验证 | 工具页只加载当前页签+rig；实际URL集合及领奖延迟加载护栏；独立eager请求旁路被杀死。 |
| P1-12 首屏同步阻塞 1.01MB JS + 418KB CSS，其中 84KB 是生产环境立即 return 的 demo 数据 | 已落地（工单允许步骤1） | 生产移除 demo 静态下载，动态就绪屏障；首屏 gzip<420KiB；?demo/#demo启动及真实Chromium已验证。不做模块/CSS延迟加载。 |
| P1-13 `--ui-zoom` 漏改四处（项目自己在 app.css:33 定了规则） | 已实测 | 200%/250% 真 Chromium 几何测量；cs-dialog 在 app-frame 内实际继承 zoom，必须 zoom:1 避免双倍；更新弹窗、下拉、toast 均已核对。 |
| P1-14 英雄目录拉取失败时没有负缓存，ddragon 不可达的机器每次总览白等 6 秒 | 已落地 | 名称目录60s失败负缓存/singleflight，调用者取消不污染负缓存；失败5次请求/到期重试护栏及旁路。 |
| P1-15 desktop.log 拿不到、也不会轮转，目录名还和产品名不一致 | a/b 已落地／c 延后 | 2MiB×5轮转，导出白名单外壳启动事件而非原始 stderr；10MiB+及真实下载完成hook测试。Electron真机导出待验；不改userData路径。 |
| P1-16 riot key 自检护栏是字符串型的，一行就能绕过 | 已落地 | AST定位真实自检分派边界；注释/提前return/独立旁路三变异，与旧字面量护栏对照。 |
| P1-17 一次缓存命中会把该轮后续所有页强制降到 20 条 | 已落地 | 缓存命中不降后续页到20；预置20条后仅一次30条请求，旁路cap变异。 |
| P1-18 对一个永不为 nil 的字段取 app 级写锁 | 已落地 | sync.Once 初始化取代 app 全局锁；100并发/race与持锁旁路行为护栏。 |
| P1-19 韩服战绩详情的信号量获取不响应取消，取消后仍烧共享 Riot 配额 | 已落地 | 可取消信号量，Do前最后限速检查，取消不计普通失败；30ID取消测试。总览窗口含4个先行请求，详情与前置额度分开断言。 |
| P2-1 死代码 20 处 + 孤儿资源 7 个 | 已清理 | 删除24个不可达函数和hextech-chest.png；最终deadcode -test执行成功且输出为空。保留有动态生产引用的6个tier SVG。 |
| P2-2 根目录 34 份历史文档归档 | 已落地 | 归档31份历史文件并更新README/docs引用；实际非34份；当前R86工单保留用户给定路径。 |
| P2-3 死 CSS | 已落地／已验证 | 1382类全量交叉核对，126无字面量候选+动态前缀排除；已删除确认的资源卡/领奖台/零散死规则。保留arena-prism-section：renderArenaItemSection以prism实参动态生成，是工单误报。 |
| P2-4 build-windows.ps1 已经没人引用 | 已落地 | 废弃脚本改为明确报错stub；source-fingerprint仍引用，不能按工单零引用假设删除。 |
| P2-5 版本号要手改 5 处，而且已经漂了 | 已落地 | 版本统一读取desktop/package.json；无第二份默认值。 |
| P2-6 五个用真实 sleep 的测试占了整套 Go 测试 58% 的时长 | 已落地／已验证 | 每实例时钟/等待/超时/进度tick注入；五项生产参数保持18s/8s/3s及250ms/750ms；真实取消、进度、去重和POST断言不变。五项合计约0.04s，未删除或跳过测试。 |
| P2-7 Windows 专属文件的自动化覆盖缺口 | 测试已补／待 Windows | installer目标目录/磁盘/父目录冒烟已补；GOOS=windows go test -c成功，但不能在macOS运行Windows测试。 |
| P2-8 客户端没运行时，lockfile 扫描仍然每轮全量 stat | 已落地 | 仅可靠零进程跳过扫描；错误/不可读保留lockfile兜底，回调调用数护栏。 |
| P2-9 连接管理状态机没有任何行为测试 | 已落地／已验证 | 同一连接循环注入依赖；3/6/8→成功→3退避、暂停/refresh、真实TLS/WS断线仅重连事件流；独立退避覆盖旁路被杀死。 |
| P2-10 隐私护栏的文件清单没有文档化，且是字面量匹配 | 已落地 | 自动遍历所有生产Go，AST识别字面量/常量拼接诊断key；新文件旁路被杀死；修复真实summoner_id泄漏为布尔值。不声称任意数据流分析。 |
| P2-11 前端渲染的常数放大器（做完 P0-7 后收益会自动缩小，优先级排在它之后） | 已落地／已验证 | 一次metric查询、图片捕获委托、ResizeObserver滚动还原、页签rAF节流；真实200场监听<1000，静止高度≤3帧且还原目标/用户滚轮终止，20次scroll≤2次style读取；独立图片监听和rAF循环旁路被杀死。 |
| P2-12 零散一致性问题 | 已落地 | 禁用按钮opacity、snapshot8MiB单文件/64MiB总量+30份、会话token原子写。64MiB是本次明确选择的预算非工单原值。 |
| P2-13 ⚠️高风险：把总览首屏契约拆开（建议排在 P0-6 之后单独一轮） | 待单独确认 | 高风险响应契约拆分未修改；需独立一轮实现及提交。 |
| P2-14 ⚠️需用户拍板：更新链路的信任根不完整 | 待用户拍板 | 更新清单签名/可信清单来源涉及发布信任根，未自行改变或生成密钥。 |
| P2-15 ⚠️需用户拍板：.git 历史里有约 293MB 误提交的 .gomodcache | 待用户拍板 | 未重写历史、未gc清理历史、未强推。 |

## 本地提交与交付状态

- `7eb6393`：初始Git账本整理；`b8db93d`：发布质量门禁/CI；`4d20f92`：统一转义。
- `b7262b6` / `072ee7e`：总览上游并发及重叠测试；`f06979c`：单场详情局部替换。
- `c5819bf`：本轮缓存/渲染/诊断/存储/测试优化；`9b4c8fa`：31份历史文档归档与引用修复。
- `504e999`：渲染测试窗口清理修复；最终全量JS验证覆盖这一源码状态，之后仅补验收文档。
- 当前分支main；均为本地提交，未推送、未发版。收尾文档提交后 `git status --porcelain` 为空，`git diff --check` 通过，跟踪索引无exe/go-tmp构建产物。原始R86工单仍在用户给定根路径。

## 验证记录

- 初始阶段全量 Go / race / build / vet / Windows vet 与 JS 576 pass 曾通过；之后又有改动，不能把该结果当作最终树结果。
- 最新完整 Go：1446 pass / 14 skip / 0 fail，38.591s（Darwin arm64、Go1.24.5）。五项慢测试分项：重连0s，总览0.01s，ready-check0.01s，SGP错误0s，更新fallback0.02s。
- 最终完整JS：588 pass / 1 skip / 0 fail，151.840s（含新增窗口清理护栏）。唯一skip为Windows专用负面构建测试；本机Bash/CI测试通过。此次全量输出无退出期TypeError。
- 全量race：通过，52.814s；独立隐藏玩家并发夹具race重复10次通过。根模块build/vet与根+installer Windows vet通过，installer独立go test/vet通过；Windows测试二进制交叉编译通过。最新deadcode -test无不可达结果。
- 独立路径变异：17项Go overlay全部被真实断言杀死，见 `r86/mutation-results.json`；初始10项另存于 `r86/mutation-results-initial.json`。可重跑 `r86/run-mutations.py`（UTF-8读写，临时目录遵从GOTMPDIR或系统默认，不硬编码macOS路径）。JS独立旁路由各test文件自行执行（英雄搜索、筛选、外部详情、SSE、工具惰性加载、图片监听、滚动、demo、外壳日志）。纯归档/死代码清理通过引用核验和全套回归，不把源码静态文本扫描当作行为证据。
- 死CSS全量交叉核对：`r86/css-crosscheck.json`，是筛选依据，不是自动删除许可。

## Chromium 缩放证据

真实应用征召弹窗，2560×1440：200% x480/y115.25/w1600/h1209.5；250% x280/y40/w2000/h1360。误加第二层zoom会出现3400px高，已纠正。
真实应用长下拉，1280×720：200%和250%实高均360px。更新弹窗使用index.html原始markup与真实CSS独立fixture：200% w1120/h1044，250% w1400/h1205，均在2560×1440视口内。Toast字体分别28/35px，padding分别20/26px和25/32.5px。
临时fixture、浏览器tab和预览服务已清理；截图曾内联检查，未保存为可审计图像，因此这里是测量记录而非截图附件。

## 日志缓冲边界

为避免闲置fd/测试目录释放问题，采用200ms批次Flush后释放fd，批次内共用fd；不是永久打开一个fd。64KiB缓冲满时自动刷盘。正常退出/导出强制Flush；SIGKILL、掉电等仍可能丢最后缓冲，不能承诺崩溃零丢失。后台Flush错误会由导出及下一次append显式返回。外壳日志导出只保留审定的启动数值及有限错误标签，未知原始stderr仅导出省略计数，避免稳定身份和私有路径进入分享产物。

## 必须单独决定的边界

1. Riot跨重启缓存的脱敏结构与隐私声明（P1-6）。
2. 总览响应契约拆分（P2-13）。
3. 更新清单签名/信任根与发布流程（P2-14，优先）。
4. 历史重写/强推（P2-15）。
5. userData目录迁移（P1-15c，可维持现状）。

## 纠正测试夹具而非只改断言

旧boot设施的 `matchCount:200` 仅修改localStorage，没有扩充17条demo数据，原“200场”测试实际仅17场。本轮已在fixture真正生成200个唯一gameId，并增加`length===200`断言，性能预算仍保持原值；现在单卡/筛选/监听护栏实际覆盖200场。

## 无法在当前机器完成的验收

- 当前macOS没有PowerShell/Windows runner；Windows真实执行、发布脚本故意失败负面验证和安装器原生行为不能由交叉编译冒充。
- 未推送远端，GitHub Actions运行记录/绿色状态未验证；工作流和所有脚本已纳入本地提交。
- Chromium布局测量不能替代Electron原生下载界面的真机导出点击验收；下载completed hook已在真实临时文件上验证。
- 这些是外部验收限制，和P1-6/P2-13/14/15需产品/安全确认的设计决策分别记录。未实施任何历史重写、强推、发布或密钥生成。

## 最终验证中发现并修复的问题

- `go test -race` 暴露旧隐藏玩家测试的`requested` map并发计数竞争（总览并发后首次稳定触发）；已加互斥保护计数和读取，而非关闭race或串行化生产路径。
- 裸 `gofmt -l .` 会递归进入已忽略的`.gomodcache/.tmpbuild`，第三方故意损坏/新语法fixture触发解析错误。本轮未改第三方代码；Bash/PowerShell/CI格式门禁改为覆盖全部项目Go文件（包括未跟踪文件），只排除依赖、构建缓存及.git/node_modules。负面门禁fixture含无效缓存Go文件和真实未跟踪失败测试，证明缓存被排除而项目红测试仍被阻断。
- 归档Markdown真实相对链接已独立复核，未发现搬移引起断链；历史code ticks中的已不存在旧工单名保留为历史叙述。
- 英雄搜索旁路变异提前触发断言时，jsdom在window.close后仍投递MutationObserver微任务，造成退出时TypeError。只修复测试窗口清理：关闭时统一dispose并断开真实Observer，不改生产全局Observer；新增测试同时证明存活期回调有效、退出后无回调和关闭幂等。独立跳过Observer清理的变异被真实断言杀死。

## 可重跑的最终检查

```bash
export GOCACHE="$PWD/.gocache" GOTMPDIR=/private/tmp
go test ./...
go test -race ./...
go build ./...
go vet ./...
GOOS=windows go vet ./...
(cd installer && go test ./... && go vet ./... && GOOS=windows go vet ./...)
node --test --test-timeout=210000 web/*.test.cjs desktop/*.test.cjs
python3 docs/r86/run-mutations.py
go test . -run TestR86Diagnostic -bench '^BenchmarkAppendDiagnostic$' -benchtime=10000x
go run golang.org/x/tools/cmd/deadcode@v0.34.0 -test ./...
find . -type d \( -name .git -o -name .gomodcache -o -name .gocache -o -name .tmpbuild -o -name node_modules \) -prune -o -type f -name '*.go' -print0 | xargs -0 gofmt -l
git diff --check
```

所有联网探针仍保留环境变量门禁，默认14项跳过未移除。没有假装在macOS执行Windows测试；本轮没有发版、推送、改更新信任根或缓存原始Riot对局到磁盘。
