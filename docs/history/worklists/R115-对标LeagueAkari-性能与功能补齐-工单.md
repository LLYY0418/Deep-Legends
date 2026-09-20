# R115：对标 LeagueAkari 的性能优化与功能补齐

**目标：**
（1）补齐与 LeagueAkari 对比后确认存在的**性能短板**：前端首屏全量加载、LCU 事件分发逐条分配、性能护栏缺失、图片无预热、web 资源未预压缩；（**注**：原「更新包全量下载」一项经用户 2026-09-19 裁决**不做**，见 Anti-scope 第 10 条）
（2）补齐确认存在的**功能缺口**：打野路径与倾向分析、战绩聚合评分、系统托盘与全局快捷键、复活倒计时浮窗、玩家标记库（本地哈希身份）、英雄选择播报泛化、远程配置与公告通道、缓存 schema 版本迁移；
（3）清理工作区卫生问题（`.mut102gopath/` 未忽略、占位测试文件残留）。

**背景：**
本轮起因于用户要求把本项目与开源项目 **LeagueAkari**（`https://github.com/LeagueAkari/LeagueAkari`，Electron + Vue 3 + MobX + TypeORM/SQLite，v1.5.2-beta，默认分支 `dev`，4.2k star）做对比，找出可借鉴的优化与功能。

对比结论：本项目在**后端数据工程**上强于对方 —— 响应头驱动的 per-host × per-endpoint-family 限流（`riot_rate_headers.go`，`:15 riotRateScope{host, method}`、`:193-197` method 分桶）、FIFO 准入队列（`riot_limiter_queue.go:11-110`，全文 110 行）、singleflight、per-directory 磁盘预算（`binary_disk_budget.go:18` init、`:34-58` 记账与驱逐，全文 58 行）、8 阶段启动打点（`desktop_startup.go:24-43`）与 4 个里程碑白名单（`desktop_startup_stage.go:32`）、165 个 Go 测试文件。对方只有 `quick-lru` + `p-queue`。

**差距集中在三块：**
1. **分发与启动体积** —— 更新包全量约 103MB，前端 5.0MB 未压缩首屏全量加载；
2. **分析深度** —— 时间线已拉取但只解析装备与技能顺序，无打野路径分析；无成套评分公式；
3. **桌面交互形态与运维通道** —— 无托盘、无全局快捷键、无独立浮窗、无远程配置/公告、无缓存 schema 迁移。

**版本：** 当前 0.12.5。`CHANGELOG.md` 已有「未发布」段。本轮完成后由执行方按现有惯例递增版本号，并同步 `desktop/package.json`、`desktop/package-lock.json`、构建注入（`build-desktop.sh` 的 `-X main.version`）、`CHANGELOG.md`。

---

## 前置更正：以下能力本项目**已经实现**，本轮不得重复实现

上一轮口头对比中有 4 条判断有误，经代码核对后**撤回**。执行方若在本工单任何一条中发现与下表重叠，**必须停下报告，不得另写一套**：

| 上一轮误判为缺口 | 实际情况（证据） |
| --- | --- |
| 「自动游戏流程不完整」 | **已完整**。`watch_rules.go` 共 9 条规则：`accept` / `reconnect` / `play-again` / `auto-honor`（策略 `prefer-party` `party-only` `any-teammate` `abstain`，见 `watch_rules.go:235`）/ `skip-celebration` / `promote-leader` / `invitations`（按队列分别 接受/拒绝/不处理，`watch_rules.go:249`）/ `auto-matchmaking` / `position-broadcast`（`watch_rules.go:363-375`）。另有阶段机 `Matchmaking`/`ReadyCheck`/`WaitingForStats`/`PreEndOfGame`/`EndOfGame`/`Reconnect`（`watch_rules.go:442-457`）与优先级仲裁（点赞中顺延下一把，`web/suite.js:344`）。**比对方更完整**。 |
| 「缺少自动交换英雄 / 大乱斗 bench」 | **已实现**。`champselect.go:19-20`（`champSelectActionBench` / `champSelectActionTrade`）、`:49`（`HandleTrade`）、`:59-60`（`HasBench` / `HasTrade`）、`:64-69` 各队列已按模式标注 `HasBench`/`HasTrade`、`:149,160`（`benchEnabled` / `benchObserved`）。 |
| 「缺少主题系统」 | **已实现 7 套**：`dark` `azure` `emerald` `violet` `crimson` `aurora` `oled`（`web/gameplay.css:1048-1053` 起）。 |
| 「缺少系统代理支持」 | **已实现**。`desktop/proxy-resolution.cjs` 解析 PAC 指令（`PROXY` / `HTTPS` / `SOCKS5` / `SOCKS`）并经 `session.resolveProxy` 回退系统代理。 |

同时确认**已有的、不要重做的**能力：预组队识别（`gameplay.go:5969-6085` 的 `livePremadeInput` / `applyLivePremadeAssignments`）、OP.GG 观战入口（`web/gameplay.js:1954`）、战绩筛选（`riot_history_filter.go`）、Game Client Live Data 接入（`gameplay.go:39-40`）、头像/旗帜/生涯外观写入（`facade_*.go`）。

---

## 执行顺序

P7（卫生，先清障）→ P0（首屏瘦身，收益最直接）→ P1（性能护栏，防止后续退化）→ P3（桌面外壳：复活浮窗 + 托盘 + 快捷键，几乎零成本）→ P9（播报泛化，复用现有聊天通道）→ P8（玩家标记库，复用现有加盐哈希）→ P4（图片预热）→ P2（打野分析）→ P5（评分公式）→ P6（远程配置与迁移）。

---

## P7：工作区卫生（先做，低风险）

### 现状证据

1. `.mut102gopath/` 体积 **256MB**，git 状态为 `??`（未跟踪），且**不在 `.gitignore` 中** —— 一次 `git add -A` 会误提交整个 Go 工具链；同时它会污染所有全仓搜索（本轮对比调研中多次被它干扰）。
2. **`.gotoolchain/` 性质已查明（2026-09-19 核实）**：它是**误提交进仓库的完整 Go 1.24.0 工具链源码树** —— 14,087 个文件 / **236.5 MB**（`VERSION` 为 `go1.24.0`，time 2025-02-10），由唯一提交 `bfcb095`（2026-09-15）整体加入。磁盘上现已删除但删除未提交，故工作区显示 **14,087 个 `D`**（`.gotoolchain` 内**无任何本项目代码**）。成因见 `.r90env.sh`（R90 沙箱会话脚本，`export GOROOT="$O/.gotoolchain"`，路径 `/sessions/youthful-determined-volta/` 在本机已失效）。**已验证删除安全**：`build-desktop.sh:28` 的 gofmt 门禁以 `-name '.*' -prune` 剪掉所有点目录；`desktop/release-quality-gates.test.cjs:8` 用 `fs.mkdtempSync` 自造临时 fixture，不依赖仓库这份；系统 Go 为 1.24.5、`GOTOOLCHAIN=auto`、`go.mod` 只要求 `go 1.24`。其余工作区状态：**106 个修改**（`web` 24、`desktop` 16、`scripts` 3、`docs` 2、`watch_rules.go` 1、`summoner_identity_test.go` 1）+ **128 个未跟踪**。当前分支 `main`，HEAD `bfcb095`；`.git` 已达 **788 MB / 仅 18 个提交**。
3. 残留占位文件（内容为空或仅 13B，会制造「测试通过」的假象）：
   - `adhoc_watchrules_champselect_test.go`（13B）
   - `zz_verify_p2_test.go`（13B）
   - `lol-loot-assistant-go-tmp-umask`（0B）
   - `r87_independent_verify_test.go` 与 `r87_independent_p7_test.go`（各 160B / 3 行，首行注释已自述 `safe to delete`，经核实无真实断言）
4. 其他磁盘残留（**仅信息记录，本轮不处理、不删除**）：`.gocache/` 7.3GB、`.mut102gopath/` 256MB、`.gopath/` 23MB、`.gomodcache/` 21MB，合计可回收约 7.6GB；除 `.mut102gopath/` 外均已在 `.gitignore` 中。清 `.gocache` 会导致首次构建重编全部依赖。

### 实现要求

1. `.gitignore` 新增 `.mut102gopath/`、`.gotoolchain/`、`.r90env*`（三者当前均**不在** `.gitignore` 中；`.gocache/`、`/.gomodcache`、`.gopath/` 已在）。**只改 `.gitignore` 文本，不执行任何 git 暂存或提交操作。**
2. **确认 `.mut102gopath/` 是否仍被 `scripts/r*-mutation-check.py` 依赖**：
   - 若为一次性产物 → 清理，并在账本记录释放的体积；
   - 若仍被依赖 → 仅加 ignore，并在 `docs/` 说明它的用途与重建方式。
3. **`.gotoolchain/` 的 14,087 个删除由用户自行提交，执行方不得介入。** 用户 2026-09-19 明确「删除你不用管，我后面会提交」。**执行方不得对该目录执行任何 `git add` / `git rm` / `git commit`，也不得做历史重写（`git filter-repo` 之类）**；本项只需在账本中记录「性质已查明、由用户处置」。同理**不得删除或修改 `.r90env.sh` / `.r90env`**（属用户处置范围）。
4. 清理 3 个空/占位文件（`adhoc_watchrules_champselect_test.go` 13B、`zz_verify_p2_test.go` 13B —— 两者均仅 `package main`；`lol-loot-assistant-go-tmp-umask` 0B）。**`r87_independent_*_test.go` 已核实只有 2 个**（`_verify_test.go`、`_p7_test.go`），各 160B / 3 行，首行即 `// Scratch verification file left by an audit pass; safe to delete.` —— **均为纯占位、无真实断言，可直接删除**（无需再逐个确认）。
5. **不得顺手重构、不得改动任何业务逻辑**（本轮 P7 只做清理）。

### 验证判据

**必须 PASS：**
1. `git status --porcelain | grep -c '^??'` 中不再出现 `.mut102gopath`
2. `git status --porcelain | grep -c '^??'` 数量从 128 下降（占位文件清理后）
3. `go build ./...` exit 0；`go vet ./...` 无输出
4. `go test -count=1 .` 全绿（确认删除占位文件没有删掉有效断言）
5. `node --test web/*.test.cjs` 与 `node --test desktop/*.test.cjs` 全绿

---

## P0：首屏加载瘦身（收益最直接）

> **⚠️ 范围变更（2026-09-19 用户裁决）**：原 P0-A「更新分发 / 差分包」**不做**，已移入 Anti-scope 第 10 条。本节只保留首屏加载瘦身。

### 现状证据

1. **前端首屏全量加载**：`web/index.html:10-27` 一次性加载 **8 个 CSS + 10 个 JS**，全部 `defer`，无懒加载、无 `type="module"`。体积（已逐一实测）：`gameplay.js` 436KB / 6935 行、`app.js` 211KB / 3835 行、`suite.js` 161KB、`champions.js` 147KB、`gameplay.css` 160KB、`app.css` 91KB、`champions.css` 85KB、`demo-data.js` 84KB、`suite.css` 53KB。`web/` 合计 **5.0MB**。
2. **web 资源原样内嵌**：`main.go:45-49` 用 `go:embed web/*.js|css|html|png|svg` 把 5.0MB 原始内容嵌进 exe；压缩只在运行时懒做（`static_assets.go:25-85`，single-flight `gzip(BestSpeed)` + SHA-256 ETag）。
3. 背景事实（**仅供理解，本轮不据此改动**）：`desktop/package.json:79` 为 `build.nsis.differentialPackage: false`，`dist/**` 下 `.blockmap` 为 0 个，update 链路 grep `blockmap|differen|delta|差分` 零命中 —— 即当前是全量更新（R107 账本记录 Setup 103,408,128 B、后端 exe 19,712,512 B）。

### 实现要求（分两步，第 2 步可单独否决）

1. **懒加载**：`champions.js` / `pro-players.js` / `friends.js` / `suite.js` 及对应的 `champions.css` / `pro-players.css` / `friends.css` / `suite.css` 改为**进入对应一级导航时动态注入**。首屏只保留 `runtime.js` `image-queue.js` `overview-art.js` `augment-artwork.js` `app.js` `gameplay.js` 与必需 CSS（`app.css` `build-item-row.css` `gameplay.css` `metrics.css`）。
   - **硬性要求**：直接深链 / 刷新到某个页签时资源仍必须正确加载（不得白屏）；重复进入不得重复注入；加载失败必须有**可见降级**而不是静默空白（对齐「缺失显示横线，不补造」口径）。
2. **`demo-data.js`（84KB）从生产路径剔除**：确认它只服务演示 / 测试；若是，则不参与生产 `go:embed`（或仅在显式 demo 模式下加载）。
3. **本轮不做 minify / 不引入前端构建链。** 原因：会改动 5MB 源码格式、让 diff 不可读、与「不瞎改」冲突。只在账本中记录为后续候选。
4. **不得改动任何更新 / 分发相关配置**（`desktop/package.json` 的 `build.nsis.*`、`update.go` / `update_download.go` / `update_http.go`）。原 P0-A 已否决，**执行方不得顺手开启 `differentialPackage`**。

### 边界（不要动）

- 不改 `static_assets.go` 的 ETag / gzip 语义与缓存头。
- 不改 `web/image-queue.js` 的调度参数（并发 6 / `rootMargin` 160px / 10s 重试 ×1 / 10min URL 复用），那是 R107 定稿值。
- 不改任何页面的视觉与文案。

### 验证判据

**必须 PASS：**
1. **首屏字节数对比**：在 Electron 中测量首屏实际加载的 JS/CSS 字节数，前后对比写入账本（可复用 `desktop/main.cjs` 的 startup marks 与 `desktop_startup_stage.go:32` 的里程碑打点）。
2. `node --test web/*.test.cjs` 全绿（33 个文件）。
3. `node --test desktop/*.test.cjs` 全绿（35 个文件）。
4. **深链清单**：对「生涯 / 英雄 / 对局 / 收藏 / 职业选手 / 好友 / 工具集」每一个一级导航，各执行一次「URL 直达刷新」，确认无白屏、无重复注入、控制台无 404。
5. **`desktop/package.json` 的 `build.nsis.differentialPackage` 仍为 `false`**（有断言，防止顺手改动）。

### 对抗变异

1. **变异 #1：删掉动态注入的失败降级分支** → 必须有测试或清单项抓到（页签显示可见错误而非空白）。
2. **变异 #2：懒加载后重复进入同一页签** → 断言脚本 / 样式节点数量不增长。

---

## P1：性能护栏与 LCU 事件路由

### 现状证据

1. **几乎没有性能护栏**：全项目**只有 1 个** `func Benchmark`，在 `r86_diagnostics_test.go`；`installer/` 与 `tools/` 为 0。但 R107→R113 **连续 7 轮**都在修性能与额度问题（图片并发、分页 rerender、`matchFilter` 丢弃、响应头限流、自动翻页失控、推荐串行等待）。对方有 `src/shared/utils/radix-matcher.bench.ts` + `scripts/run-radix-benchmark.mjs` + `scripts/radix-benchmark-report.mjs` 的基准文化。
2. **LCU 事件分发逐条分配**：`lcu_events.go:82` 订阅单一 `OnJsonApiEvent`（这点与对方一致，是对的）；但 `:136` 对**每一条事件**执行 `strings.ToLower(event.URI)`，随后 `:137-156` 串行 5+ 次 `strings.HasPrefix`。对局中 LCU 事件量大，`ToLower` 每条都产生字符串分配。`:98,119-123` 已有 `uriStats` / `topLCUEventURIStats` 诊断统计。

### 实现要求

**A. Go benchmark 套件**

1. 新增 `bench_core_test.go`，覆盖 6 个性能敏感路径：
   - `riot_rate_headers.go` 的桶准入 / in-flight 预留
   - `riot_limiter_queue.go` 的 FIFO 准入与取消（cancel-safe 路径）
   - `asset_cache.go` 命中 / 未命中 / singleflight（内存 1200 条 / 256MiB FIFO + 负缓存 TTL）
   - `overview_cache.go` LRU 256 与 TTL 分支
   - LCU 事件路由（见 B 项）
   - `static_assets.go` 的 gzip + ETag 路径
2. 新增 `scripts/run-benchmarks.sh`：执行 `go test -bench=. -benchmem -benchtime=200ms -count=5`，输出 markdown 报告到 `docs/r115-validation/bench/`，并支持与上一份基线做百分比对比（若本机无 `benchstat` 则用脚本内简单对比，**不引入新 Go 依赖**）。
3. **硬性约束：benchmark 不得发起真实网络请求**（必须用 `httptest` 或纯内存桩）。否则会在 CI/本地烧掉 Riot 额度 —— 撞「把使用过的额度还给我」红线。在 `bench_core_test.go` 顶部写注释说明该约束。

**B. LCU 事件路由去分配**

1. 把 `lcu_events.go:136-156` 的 `ToLower` + 串行 `HasPrefix` 换为**大小写不敏感的前缀树**，或先按 URI 第一段（`/lol-xxx/`）做 `map` 一级分发 + 段内前缀匹配。
2. **匹配语义必须完全不变**：同样的 URI 必须命中同样的分支，包括大小写混写、尾部斜杠、带 query、以及 `/lol-champ-select/v1/session` 这类前缀族（`:137,145,149,152,156`）。
3. **不引入第三方路由库**（`go.mod` 只有 4 个依赖是项目优点，不要破坏）。对方用 radix 树是因为要支持用户自定义订阅通配符，**我们不需要通配符能力，不要照抄 radix 树**。
4. **保留** `uriStats` 统计与 `topLCUEventURIStats` 诊断（日志完备红线）。

### 验证判据

**必须 PASS：**
1. `go test -run=XXX -bench=. .` 全部有输出、无 SKIP；基线报告入库。
2. 新增 `TestR115_LCUEventRoutingParity`：表驱动列出**现有所有分支的 URI 样本**（含大小写混写、尾部斜杠、带 query、每种前缀族的边界值），断言新旧实现分支归属一致。
3. 路由 bench 的 `allocs/op` 相对基线**下降**（记录具体数值）。
4. `uriStats` / `topLCUEventURIStats` 输出结构不变（有测试断言）。

### 对抗变异

1. **变异 #1：把大小写不敏感改成敏感** → parity 测试必须失败。
2. **变异 #2：去掉 `asset_cache` 的 singleflight** → 对应 bench 的 `allocs/op` 应显著上升（证明该 bench 真的在测这条路径，而不是空转）。
3. **变异 #3：路由表删掉 `/lol-champ-select/v1/session` 前缀族分支** → parity 测试必须失败。

---

## P3：桌面外壳能力 —— 复活倒计时浮窗 + 系统托盘与全局快捷键（低成本，优先于 P2/P5）

### P3-A：复活倒计时浮窗

#### 现状证据

- Game Client Live Data API **已接入**：`gameplay.go:39-40` 定义 `liveClientPlayerListURL = "https://127.0.0.1:2999/liveclientdata/playerlist"` 与 `liveClientAllGameDataURL = "https://127.0.0.1:2999/liveclientdata/allgamedata"`。
- **更正（经独立核对）：`respawn` 并非零命中。** `respawnTimer` 已有 3 处：`arena_truth_diagnostics.go:250`（**allgamedata 诊断字段白名单 —— 该字段已在数据流中流转，是 P3-A 探针的现成入口**）、`r90_test.go:114`（fixture 字段白名单）、`gameplay_test.go:2418`（live player 桩）。`复活` 确为 0 命中；`web/*.js` 中 `respawn` / `复活` / `isDead` 均为 0。结论不变：**数据在手但没有任何 UI 或浮窗使用它**；探针应先从 `arena_truth_diagnostics.go:250` 入手。
- `desktop/main.cjs:10` 的 require 只有 `app, BrowserWindow, dialog, ipcMain, nativeTheme, session, shell` —— **无 `Tray`、无 `globalShortcut`**（`desktop/` 全目录两者均 0 命中）；`main.cjs:136` `splashWindow`、`:452` `mainWindow`。**更正（经独立核对）：全项目共 7 处 `new BrowserWindow`**，除上述 2 处外还有 `desktop/share-export.cjs:139`（`exportWindow`）与 4 个 `diagnostics-*-render.cjs`（`:7` / `:8` / `:9` / `:7`）。**实现浮窗前必须对照 `share-export.cjs` 的既有窗口生命周期范式，不要新写一套。**
- 对方参考实现：`src/main/shards/respawn-timer/respawn-timer-controller.ts`（监听 `gameflow.phase === 'InProgress'`，按 `RESPAWN_TIMER_POLL_INTERVAL` 轮询，离开 InProgress 时清零）与 `src/renderer/src-cd-timer-window/`。

#### 实现要求

1. **先探针，后实现**（沿用 R99/R101 的探测纪律）：新增探针记录 `allgamedata` 中 `isDead` / `respawnTimer` 的真实取值与刷新粒度，输出到 `docs/r115-validation/respawn-probe.md`，并明确标注抓取时间、客户端版本、是否真机。
2. 基于探针结论实现浮窗：一个独立的、可置顶的 Electron 小窗，显示复活倒计时。
   - **只在 `gameflow.phase == 'InProgress'` 时运行**，离开该阶段立即停止轮询并清零（避免空转与无意义本机请求）。
   - 轮询间隔**不得快于探针实测的客户端刷新粒度**；间隔写成常量并注释依据。
   - 主窗口关闭/隐藏时该浮窗的行为需明确定义，并在设置页可关闭（默认关闭）。
3. **复用已有的 `allgamedata` 读取**，不得为此新增第二个 2999 端口轮询循环（避免与 `gameplay.go` 现有逻辑抢请求、避免状态不一致）。
4. **隐私口径**：2999 是新的本机依赖，必须在设置页明确声明「本机读取 Game Client 实时数据（127.0.0.1:2999）」，且**浮窗上不得显示端口、令牌或本地路径**（对齐 PRODUCT.md「不展示令牌、本地路径」）。
5. **视觉不得模仿对方**（PRODUCT.md Anti-references 明确禁止照搬对方品牌、颜色、图标与页面编排）；沿用本项目海克斯黑金风格，遵守 `prefers-reduced-motion`，动效 160–220ms。

#### 验证判据

**必须 PASS：**
1. 探针文档存在，且明确写出「真机 / 非真机」边界。
2. 阶段机测试：`InProgress` 开始轮询、离开 `InProgress` 停止并清零、重进 `InProgress` 重新开始 —— 三条路径各有测试。
3. 设置项默认关闭；关闭后不产生任何 2999 请求（有测试断言请求计数为 0）。
4. `desktop/*.test.cjs` 新增浮窗生命周期测试（沿用现有 jsdom/stub 方式）。
5. 真机清单见文末。

#### 对抗变异

1. **变异 #1：离开 `InProgress` 不停止轮询** → 阶段机测试必须失败。
2. **变异 #2：设置关闭后仍发起请求** → 请求计数断言必须失败。
3. **变异 #3：轮询间隔改到快于探针实测粒度** → 常量断言测试必须失败。

### P3-B：系统托盘与全局快捷键

#### 现状证据

- `desktop/main.cjs:10` 的 `require` 中**无 `Tray`、无 `globalShortcut`**（只有 `app, BrowserWindow, dialog, ipcMain, nativeTheme, session, shell`）。
- 主窗口关闭即退出，无常驻入口；`autoAccept` 等规则虽在后端运行，但用户无法在不打开主窗口的情况下快速查看状态或唤起窗口。
- 对方参考实现：`src/main/shards/tray/`（4 个文件）、`src/main/shards/keyboard-shortcuts/`（6 个文件）。

#### 实现要求

1. **系统托盘**：常驻托盘图标，提供「显示主窗口 / 退出」最小菜单；图标沿用现有 `assets/hexcore-icon.ico`，**不得引入对方图标**。
   - 必须明确定义「点击关闭主窗口」的语义：最小化到托盘 or 退出。二者只能选一种，并在设置页说明；**不得出现「用户以为在运行、其实已退出」的歧义状态**（对齐声明准确性红线）。
   - 托盘菜单文案全中文。
2. **全局快捷键**：至少提供「唤起 / 隐藏主窗口」一个快捷键。
   - **默认不注册**（避免抢占用户已有快捷键），在设置页可自定义；注册失败（被占用）必须**可见地告知**，不得静默失败。
   - 应用退出时必须注销全部快捷键（避免残留钩子）。
   - **本轮不得用全局快捷键触发任何写操作**（如自动接受、符文写入），只允许窗口显隐类动作。若要扩展到写操作，需单独排单并确认「单次触发」语义。
3. 托盘与快捷键都要有开关，默认状态在设置页可见、可解释。

#### 验证判据

**必须 PASS：**
1. `desktop/*.test.cjs` 新增：托盘创建/销毁生命周期；关闭主窗口语义与设置项一致；快捷键注册成功 / 注册失败 / 注销三条路径。
2. 快捷键被占用时 UI 有可见提示（有测试断言），且不影响其余功能。
3. 退出后无残留全局钩子（有测试断言注销被调用）。
4. 默认值符合要求：托盘默认开、快捷键默认不注册（有测试断言）。

#### 对抗变异

1. **变异 #4：应用退出不注销全局快捷键** → 注销断言必须失败。
2. **变异 #5：快捷键注册失败被静默吞掉** → 可见提示断言必须失败。
3. **变异 #6：让全局快捷键直接触发写操作** → 动作白名单断言必须失败。

---

## P4：图片资源预热

### 现状证据

- 英雄/皮肤/头像/原画**一律按需下载**：来源 `raw.communitydragon.org`、`ddragon.leagueoflegends.com`、`game.gtimg.cn`（定义在 `prestige.go:18 prestigeArtworkHost`）、`opgg-static.akamaized.net`，经 `/api/image`、`/api/champion-asset`、`/api/skin-art`、`/api/prestige-image`、`/api/media` 代理后落磁盘缓存。响应体上限**分散在多个文件**（7 个数值已逐一核对）：champion JSON 6MiB / HTML 5MiB / 图片 4MiB 在 `champions.go:36-38`；Riot 4MiB 在 `riot_api.go:128`；时间线 16MiB 在 `riot_api.go:130`；qq101 2MiB 在 `qq101.go:18`；ARAMKit 2MiB 在 `aramkit_rating.go:22`；feature gates 64KiB 在 `feature_gates.go:19`。
- **没有离线图片包**：`data/` 仅 6 项且均为奖池/发售日相关（`reroll_pool_14_5.json` 与 `.txt`、`reroll_pool_14_5_source.jpg`、皮肤发售日 JSON、`README.md`），**无英雄/皮肤图片离线包**；UI 图标是内嵌的（`arena-team-icons/` `position-icons/` `tier-icons/` `rune-styles/` `loot-icons/` `rank-crests/`）。
- **R99→R101→R104→R106→R107→R111 连续六轮都在修图片问题**（401、投毒、被误隐藏、并发、三层观察器互相打架、DOM 变化全页扫描）。这是明确的历史痛点。
- 已有可复用的基础：`live_prewarm.go`（**全文 106 行**；`warm()` 在 `:50`、容量上限 32 条在 `:83`、异步 goroutine 在 `:89-104`、冷却常量 15s / 3min / 30s 在 `:90,94,96`；`:45 loadDetail` 即英雄详情预热）、`champion_images.go:63,69,90` 的 `assetFetchStats`（每 host/窗口最多 512 个延迟样本）、`web/image-queue.js`（并发 6 `:4` / `rootMargin` 160px `:16` / 10s 重试 ×1 `:7` / 10min URL 复用 `:24`）。
- 对方参考实现：`src/main/shards/extra-assets/asset-refresh-controller.ts`（受控的本地资源包刷新）。

### 实现要求

1. **扩展 `live_prewarm.go` 的预热范围**，而不是新写一套调度器：在启动后低优先级预取「当前版本的常用英雄方图 + 段位徽章」。
2. **必须让位于前台**：预热请求优先级低于用户主动请求，遵守 `riot_limiter_queue.go` 的 FIFO 准入与 `pro_refresh.go:21-53` 的「后台让位于前台」既有模式。
3. **必须可关闭**，默认行为需在设置页说明（发送了什么、发往哪个已声明数据源）。
4. **绝不预取 Riot API 数据**（只预取静态图片资源），避免烧额度。
5. 预热必须受 `binary_disk_budget.go` 的 per-directory 磁盘预算约束（`:18` init、`:34-58` 记账与驱逐、`:41` 为 `proseed-` 保护；**该文件全文仅 58 行**，勿按越界行号定位），且**不得触碰 `proseed-` 受保护条目**。
6. **记录命中率**：预热后统计「首次打开页面时仍需网络下载的图片数」与「预热命中数」，写入诊断。

### 验证判据

**必须 PASS：**
1. 预热请求数、命中数、跳过数在诊断日志中可见且可解释。
2. 关闭预热后，启动阶段不产生任何图片请求（有测试断言）。
3. 预热不得突破 `binary_disk_budget.go` 的预算（有测试，且 `proseed-` 条目完好）。
4. 冷启动实测对比：首屏图片等待时间前后对比写入账本。
5. `go test -race -run 'TestR115' -count=1 .` 无数据竞争。

### 对抗变异

1. **变异 #1：让预热请求抢到前台之前** → 限流/优先级测试必须失败。
2. **变异 #2：让预热突破磁盘预算** → 预算测试必须失败。
3. **变异 #3：把预热扩到 Riot API 数据** → 必须有测试抓到（断言预热请求的目标 host 白名单不含 Riot API 域）。

---

## P2：打野路径与倾向分析

### 现状证据

- 时间线**已拉取并缓存**：`match_timeline.go:378` `matchTimelineCache`、`:414` `matchTimelineRequest`；对外端点 `POST /api/gameplay/match-timeline`；Riot 时间线响应上限 16MiB。
- **但解析极浅**：`timelineEvent`（`:27-38`）只有 `Type` / `EventType` / `Timestamp` / `ParticipantID` / `ItemID` / `BeforeID` / `AfterID` / `SkillSlot` / `LevelUpType` / `rawKeys`。**没有** `monsterType`、`position`、`killerId`、`victimId`、`assistingParticipantIds`、`buildingType`、`towerType`、`laneType`、`wardType`、`creatorId`。
- `timelineFrame`（`:60-63`）只有 `Timestamp` + `Events`，**没有** `participantFrames`（即拿不到逐分钟位置）。
- 对外响应 `matchTimelineResponse`（`:82-90`）只输出 `Available` / `Source` / `Detail` / `Attempts` / `FallbackReason` / `ItemGroups` / `SkillOrder`。
- **全文件 `jungle` / `打野` 零命中** —— 确认无打野分析。
- 对方参考实现：`src/shared/data-adapter/analysis/player/aggregate/jungle.ts`、`player/single/jungle.ts`、`player/single/objectives.ts`、`player/single/early-deaths.ts`、`player/utils/geometry.ts`、`player/types/helpers.ts`（`GankPoint` / `MinutePositionPoint`）。

### 实现要求

1. **扩展 `timelineEvent` 的 JSON 字段**：新增 `MonsterType`(`monsterType`)、`MonsterSubType`、`Position{X,Y}`(`position`)、`KillerID`(`killerId`)、`VictimID`(`victimId`)、`AssistingParticipantIds`(`assistingParticipantIds`)、`BuildingType`、`TowerType`、`LaneType`、`WardType`、`CreatorID`、`KillType`。
   - **`UnmarshalJSON` 的 `rawKeys` 行为必须保持不变**（`:40-58` 已排序保存字段名，现有测试依赖它）。
   - 按需扩展 `timelineFrame` 增加 `participantFrames`（仅在需要位置轨迹时）。
2. **新增独立文件 `match_timeline_jungle.go`**，不要把逻辑继续堆进 `match_timeline.go`。计算内容：
   - **首轮清野**：限定打野位参与者，按 `timestamp` 排序的野怪击杀序列（`MONSTER_TYPE` 族事件），输出前 N 个营地与时间点；
   - **首个 buff / 三狼 / 石甲虫 / 蛤蟆 / 河蟹**的识别与各自时间；
   - **目标控制**：`ELITE_MONSTER_KILL`（DRAGON / BARON / HERALD / VOIDGRUB）的归属队伍与时间；
   - **3 级 / 4 级早期 gank**：结合 `LEVEL_UP` 时间戳与 `CHAMPION_KILL` 位置判定早期击杀；
   - **三分区权重**：按击杀位置 Y 坐标划分上/中/下三区，统计 gank 分布。
3. **判定必须可解释**：分区阈值、野怪营地识别方式（`monsterType` 字符串 + 坐标范围）全部写成**具名常量**，并在 UI 悬浮说明里写清依据。**数据不足时必须输出「未提供」/ 降级状态，不得用推断值或 0 代替**（数据准确性红线）。
4. **绝不为此额外发起请求**：只在**已经拉取到时间线**的对局上计算。时间线不可用时，分析区块显示明确降级状态（对齐 R112「缺失场次显示横线，不补造段位」的口径）。
5. **ARAM 系模式（无野区）必须走降级路径**，不得输出空的野区数据冒充结果。
6. 前端在战绩详情里加**折叠区块**，沿用本项目海克斯黑金视觉；**不得模仿对方布局与配色**。

### 验证判据

**必须 PASS：**
1. 表驱动测试（用 `testdata/` 中已有真实时间线样本；若无则新增一份**脱敏 fixture 并注明来源与抓取时间**）覆盖：首轮清野序列、目标归属、三分区计数、3/4 级 gank 判定。
2. **四类降级路径各有测试**：时间线缺失 / 字段缺失 / 非打野位 / ARAM 模式。
3. **额度守护测试**：断言本功能在时间线已缓存时不产生任何新的上游请求（对齐 R110「本地限流与 Riot 429 不得混为一谈」的日志口径）。
4. `go test -count=1 .` 全绿；`node --test web/*.test.cjs` 全绿（含新增 UI 测试）。
5. 坐标/阈值常量的依据写进 `docs/` 与 UI 悬浮说明。

### 对抗变异

1. **变异 #1：把三区分割阈值改错** → 分区计数测试必须失败。
2. **变异 #2：把「时间线不可用」改成返回空数组而非降级标记** → 降级路径测试必须失败。
3. **变异 #3：把非打野位参与者也纳入首轮清野** → 位置过滤测试必须失败。
4. **变异 #4：ARAM 模式走正常路径** → 模式降级测试必须失败。

---

## P5：战绩聚合与 DL 评分公式

### 现状证据

- 本项目已有 `rank_insights.go`、`season_stats.go`、`aramkit_rating.go`、`riot_history_filter.go`，但**没有成套的通用评分**（`评分` / `rating` 在根 Go 文件中的命中集中在 `aramkit_rating.go` 与诊断，不在通用战绩聚合）。
- 对方参考实现：`src/shared/data-adapter/analysis/player/scoring.ts` + `constants.ts`，评分由 KDA（`Math.sqrt(kda - baseline) × weight`，见 `scoring.ts` 的 `scoreKda`）、参团率、补刀/分钟、治疗占团队平均承伤比、胜率线性映射组成；常量集中在 `constants.ts`，有单元测试；聚合维度见 `aggregate/{positions,champions,win-loss,summary,team-side,spells,details,akari}.ts`。

### 实现要求

1. 实现「DL 评分」，**公式必须完整公示**：所有权重、基线、上下限写成具名常量，并在 UI 悬浮说明中给出每个分项的原始值、分项得分与总分构成。
2. **数据不足时不给分**：场次不足、字段缺失、模式不适用时显示明确降级状态，**不得用 0 分或估算值代替**。
3. 聚合维度至少包含：位置分布、英雄聚合、胜负汇总、连胜连败。每个维度都要能说明「样本数是多少」。
4. **不得混用统计口径**（PRODUCT.md Anti-references ④：不得把炫彩、任务皮肤形态、基础皮肤、战利品碎片和普通皮肤混为同一口径）——评分同理，不同队列/模式的样本不得混算。
5. 纯本地计算，**不新增任何外部数据源请求**。

### 验证判据

**必须 PASS：**
1. `scoring` 常量的单元测试：每个分项的边界值（下限、上限、基线、越界）各有断言。
2. 总分等于各分项之和（或公示的合成规则）——有测试断言，防止公式与展示不一致。
3. 降级路径测试：场次不足 / 字段缺失 / 模式不适用 三类各一条。
4. UI 展示的分项明细与后端返回的常量一致（有测试断言两者同源，不得前端硬编码一套公式）。
5. `go test -count=1 .` 与 `node --test web/*.test.cjs` 全绿。

### 对抗变异

1. **变异 #1：把某个分项上限改错** → 边界测试必须失败。
2. **变异 #2：前端硬编码一份不同权重的公式** → 同源断言必须失败。
3. **变异 #3：数据不足时返回 0 分而非降级** → 降级测试必须失败。

---

## P6：远程配置与公告通道 + 缓存 schema 迁移

### 现状证据

- 本项目有 `feature_gates.go` 与 `update.go` / `update_download.go` / `update_http.go`，但**未见远程配置、公告、发布信息的多数据源 + 缓存 + 回退 + 诊断链路**。
- R112（OP.GG `average_tier` 混用对象与 `$undefined`，旧实现整页反序列化遇字符串就丢掉整页）、R113（Hexdata `/v1/public/match-report` 返回 403 `web_lookup_only`）都是**上游突变** —— 若有远程开关即可不发版关掉某个数据源。
- 缓存清单庞杂且各有 TTL/容量：`asset_cache` 内存 1200/256MiB FIFO（`asset_cache.go:10,11`）、`overview_cache` LRU 256/TTL 2s（KR 1min，`overview_cache.go:15`）、`facade_view_cache` 30s/2s、`gameplay_summoner_cache` 256/5min、`prestige_cache` 磁盘 512/192MiB/45天/单条 8MiB（`prestige_cache.go:17-19`）、`pro_snapshot_cache` 磁盘 2/16MiB/24h、`riot_match_cache` 磁盘 600/128MiB/365天（`riot_api.go:131`）、`riot_identity_cache` 磁盘 1024/4MiB。
- **⚠️ 措辞更正（经独立核对）：并非「没有 schema 版本」。** 已有 **5 套** per-store 版本与不匹配处理：`lp_tracker.go:28 lpHistorySchemaVersion = 1`、`storage.go:32,58` 的 `SnapshotRecord` / `PoolManifest` `SchemaVersion`（`:257` v2、`:283` v1）、`watch_rules.go:83,137,229 watchSettingsVersion`、`season_stats.go seasonStatsCacheSchemaVersion`（`season_stats_test.go:254` 已有「版本 -1 被拒」测试）、`features.go:30,395 schemaVersion 5`。**真正缺的是统一入口与统一迁移路径，不是版本号本身。执行方不得重新发明版本号，必须复用上述既有字段、只做归拢。**
- 对方参考实现：`src/main/shards/akari-api/`（`config-loader.ts` / `notice-loader.ts` / `release-loader.ts` / `cached-resources.ts` / `bootstrap-controller.ts` / `protocol-controller.ts`）、`src/main/shards/config-migrate/`（15 个文件）、`src/main/shards/storage/upgrades/version-10.ts`、`version-15.ts`。

### 实现要求

**A. 远程配置与公告**

1. 实现远程配置读取：**多数据源 + 缓存 + 回退 + 诊断**，遵循本项目既有的数据源纪律（`DataSourceAttempt` / `fallbackReason` 结构）。
2. **必须先确认「已声明的固定数据源」清单是否允许新增端点**。若不允许，则**只做本地 `feature_gates.go` 的扩展**（可运行时热改的本地闸门），并在账本说明「未新增外部端点」。
3. **公告**：小型 popover 预览核心内容，可关闭、不重复打扰；不得使用阻塞式弹窗（用户偏好：去冗余交互、不要提示框、不二次确认）。
4. **失败必须静默降级**，不得因远程配置不可达而阻塞启动或报错弹窗；诊断日志记录原因。
5. **逐条记录读取结果**（成功/回退/失败），并把「本次使用的配置来源」写入诊断。

**B. 缓存 schema 版本与迁移**

6. 为各磁盘缓存引入**统一的 schema 版本号**与**迁移入口**：版本不匹配时按既有惯例处理（`lp-history.json` 的做法是「损坏文件不迁移直接清空重写」，0600 原子写）。
7. 迁移必须**幂等**且**不得影响功能**（用户红线：清缓存但「绝对不可以影响到功能」）。
8. 沿用 `binary_disk_budget.go` 的预算约束，`proseed-` 受保护条目在迁移中不得被驱逐。

### 验证判据

**必须 PASS：**
1. 远程配置：成功 / 多源回退 / 超时 / 不可达四条路径各有测试；不可达时启动仍成功。
2. 诊断日志能逐条还原「读了哪个源、结果如何、最终采用哪个」。
3. 缓存迁移：旧版本文件 → 新版本结构的迁移测试；每类磁盘缓存至少一条。
4. 迁移幂等性测试（连跑两次结果一致）。
5. **迁移不得影响功能**：迁移后原有读取路径的测试全绿，`proseed-` 条目完好。
6. 公告不阻塞启动、可关闭、不重复（有测试）。

### 对抗变异

1. **变异 #1：远程配置不可达时抛错阻塞启动** → 降级测试必须失败。
2. **变异 #2：迁移非幂等** → 幂等测试必须失败。
3. **变异 #3：迁移误删 `proseed-` 条目** → 保护测试必须失败。

---

## P8：玩家标记库（本地哈希身份）

### 现状证据

- 全项目**无自有的玩家标记存储**：`web/friends.js:2` 明确「分组、顺序、折叠初始态与备注完全来自客户端（`/api/social/friends`），只读」—— 现有的「备注」是**客户端好友备注**，不是本项目的标记库。
- **落盘原语已齐备，可直接复用，无需新增任何加密实现：**
  - `storage.go:126-145`：`account-salt` 已存在 —— 32 字节随机 salt，`atomicWriteFile(..., 0o600)` 落盘，缺失时自动生成；已存在但非 32 字节时**报错拒绝启动**（`invalid local account salt`）。
  - `storage.go:350-362`：`(*localStore).accountHash(summoner Summoner)` 已实现**加盐 SHA-256 取前 16 位**；PUUID 为空时回退 `summoner:{SummonerID}`。
  - `storage.go:172`：`atomicWriteFile(path, data, mode)`；`lp_tracker.go:182` 已用 `0o600` 写 `lp-history.json`。
  - `storage.go:118`：`localStore.root` 的子目录惯例（`updates` / `pools` / `snapshots` / `season-stats` / `logs` / 两个缓存目录）。
  - `storage.go:467` `pruneSnapshots()`、`lp_tracker.go:30` `lpHistoryLimit = 400`：现成的清理与容量上限范式。
  - `lp_tracker.go:126` `validLPAccountHash`：现成的哈希格式校验范式。
- 对方参考实现：`SavedPlayers`（主键 `puuid + selfPuuid + region + rsoPlatformId`，字段 `tag` / `updateAt` / `lastMetAt`）、`EncounteredGames`（逐局相遇记录，分页 40）、`playerTagPhrases`（最多 20 条、每条 ≤100 字、trim + 去重）。**对方明文存 PUUID，本项目不得照抄。**

### P8-A：标记库（本轮做）

1. **身份键必须复用 `(*localStore).accountHash`**，不得另写一套哈希，**绝不落 PUUID / SummonerID 明文**。
2. 存储文件：`localStore.root` 下新增 `player-tags.json`，用 `atomicWriteFile(..., 0o600)`；文件损坏或 schema 版本不匹配时**清空重写**（沿用 `lp-history.json` 既有惯例），**不得让损坏文件阻塞启动**。
3. 每条记录字段：`accountHash` / `tag` / `updatedAt` / `lastMetAt`。
   - **`lastMetAt` 必须有真实来源**（对局中实际遇到该玩家的时间），**不得用「本次写入时间」冒充** —— 这与 R107「`initUpdatedAt` 被当成开局时间」是同一类错误，必须避免。
4. **身份键形态必须一致（本项最大的数据准确性陷阱）：** `accountHash` 在 PUUID 缺失时回退到 `summoner:{SummonerID}`，两种形态对同一玩家会产出**不同的键**。必须：
   - 记录每条标记使用的形态；
   - 同一玩家在形态切换时**不得产生两条互不相认的标记**（要么统一优先 PUUID 形态、缺失时跳过写入并记诊断，要么提供显式合并策略；二选一并写明理由）；
   - **必须有测试覆盖。**
5. 容量上限：定一个具名常量（量级参照 `lpHistoryLimit = 400`，建议 ≤1000 条），超限按 `lastMetAt` 最旧淘汰；**淘汰不得误删最近的标记**。
6. 标记内容上限 ≤100 字；预设短语最多 20 条，trim + 去重（避免无上限增长）。
7. API：读、写标记各一个端点。**写入必须走项目既有的 token 鉴权与 Sec-Fetch 防护**（`authorized` 起于 `main.go:626`、校验体 `:628-637`；`isTrustedNavigation` 在 `:679-685`；security headers 在 `:687-694`）。**可直接照抄 `POST /api/gameplay/match-timeline`（`main.go:510`，已包在 `a.authorized` 内）的既有范式。不得开放未鉴权写入。**
8. UI：在对局内玩家卡片（`web/gameplay.js:1954` 附近的 `playerRow`）显示已有标记，并提供编辑入口。
   - **不得只用颜色表达标记状态**（DESIGN.md 无障碍要求）；标记文案必须可见。
   - 无标记时**不显示任何占位或空标签**，不得让空标签看起来像「已标记」。
   - 沿用海克斯黑金风格与现有卡片高度/颜色规范，**不得模仿对方布局**。
9. **隐私声明**：设置页必须写明「本地保存的是不可反查的加盐哈希，不含 PUUID 与召唤师 ID；标记不上传、不跨设备同步」。**声明必须与实际写操作逐条一致**（红线）。
10. 诊断日志：记录标记读写次数、淘汰条数、身份形态；**除哈希外不得记录任何身份信息，也不记录 tag 明文**（沿用 ARAMKit 那轮的脱敏口径）。

### P8-B：相遇对局历史（本轮**不做**，明确推迟）

- 对方的 `EncounteredGames`（逐局记录 `gameId` / `puuid` / `queueType`）会持续写入，需要独立的体积上限与清理策略；其核心价值「上次遇到是什么时候」已由 P8-A 的 `lastMetAt` 覆盖。
- **本轮只交付 `lastMetAt`，不交付逐局历史。** 若后续要做，单独排单，沿用 `lpHistoryLimit` 的上限思路 + `pruneSnapshots()` 的清理范式。
- **执行方不得顺手实现 P8-B**（超范围改动零容忍）。

### 验证判据

**必须 PASS：**
1. **落盘内容断言**：写入后读取 `player-tags.json`，断言**不含任何 PUUID / SummonerID 明文**，且键为 16 位十六进制（沿用 `validLPAccountHash` 的校验思路）。
2. 文件权限 `0o600`；写入为原子写。
3. **身份形态一致性测试**（对应 P8-A 第 4 条）：同一玩家在 PUUID 有 / 无两种输入下不得产生两条互不相认的标记。
4. `lastMetAt` 来源测试：断言它**不等于**写入时间，且取自对局数据。
5. 容量上限测试：超限后按 `lastMetAt` 淘汰最旧，最近的标记完好。
6. 损坏文件测试：写入非法 JSON → 启动不失败 → 标记被清空重写。
7. 鉴权测试：无 token / 错误 Sec-Fetch 头的写请求被拒绝。
8. 标记长度与短语数量上限测试（>100 字截断、>20 条截断、重复短语去重）。
9. `go test -count=1 .`、`go test -race -run 'TestR115' -count=1 .`、`node --test web/*.test.cjs` 全绿。
10. 设置页声明与实际写操作逐条核对（有测试或清单项）。

### 对抗变异

1. **变异 #1：身份键改成明文 PUUID** → 落盘内容断言必须失败。
2. **变异 #2：`lastMetAt` 用写入时间冒充** → 来源测试必须失败。
3. **变异 #3：容量淘汰改成删最新** → 淘汰测试必须失败。
4. **变异 #4：损坏文件直接报错阻塞启动** → 损坏文件测试必须失败。
5. **变异 #5：写端点绕过 token 鉴权** → 鉴权测试必须失败。

---

## P9：播报泛化 —— 把 `positionBroadcast` 扩展为可配置播报

### 现状证据

- 本项目**已有一条完整、带护栏的「向游戏聊天发消息」写通道**：`watch_rules.go:1378-1418` 的 `broadcastPositionContext`：
  - `:1380` 读 `/lol-chat/v1/me` + `/lol-chat/v1/conversations`
  - `:1385-1390` 筛选 `type` 含 `champion` / `champselect` 的会话
  - `:1391` `safeLCUChatIdentifier(conversationID)` 校验
  - `:1394-1397` `type` 按 `Visibility` 取 `celebration`（self）或 `chat`（team）
  - `:1398-1401` 消息体；`:1405-1406` POST `/lol-chat/v1/conversations/{id}/messages`
  - `:1402`、`:1407-1409` 两处 `customPaused()` 暂停检查
  - **`:1413` POST 结果不确定时不重试，注释明写「避免重复聊天」**
- 触发与状态机：`:526`（`MasterEnabled && PositionBroadcast.Enabled && !customPaused()`）、`:536` pending、`:546-547` 清理、`:557` 调用、`:563` `watch:skipped:position-broadcast:unavailable`、`:1323` / `:1329` 诊断记录、`:1411` / `:1416` emit。
- 配置结构：`:76` `PositionBroadcast watchBroadcastRule`、`:143` 默认 `Visibility: "self"`、`:239-242` Visibility 校验（非法值回落 `self`）、`:375-376` enabled 判定。
- 队伍判定：`:1369-1377`（`1/ONE/BLUE/100` → 蓝色方，`2/TWO/RED/200` → 红色方，其余 `skipped_no_team`）。
- UI：`web/suite.js:265` 规则定义（`control: "visibility"`、`phaseKey: "ChampSelect"`、描述限「极地大乱斗、海克斯大乱斗」）、`:285-286` `watchChoiceButtons` 渲染 self / team。
- 对方参考：`src/main/shards/in-game-send/preset-controller.ts`、`presets/{index,jungle,premade,rating,name-display}.ts`、`setting-schemas.ts`（**只借鉴「预设 + 声明」的组织方式，不借鉴其游戏内键盘注入**）。

### 实现要求

1. **只泛化内容，不新增通道。** 必须复用 `broadcastPositionContext` 现有的会话发现 + `safeLCUChatIdentifier` + `customPaused` + 「不确定不重试」**全部护栏**；**不得为播报另写一条 POST 路径**。
2. 把「播报什么」抽象成可配置项，**在现有 `positionBroadcast` 规则下扩展，不新增顶层规则**（避免与既有 9 条规则的优先级仲裁冲突）。至少支持：阵营位置（现有）、阵容 / 位置提示。**每一项都必须默认关闭或维持现有默认行为**，不得让升级后的用户突然多发消息。
3. **发送内容必须完全来自本机已有数据**（LCU 会话、champ-select session、已缓存的对局信息）。**严禁为生成播报内容而发起任何新的外部数据源请求**（额度红线）。
4. **一次触发只发一条消息**（对齐「明确点击后发起单次客户端操作」的产品口径）。若要播报多项，必须合并为**一条**消息，不得连发多条刷屏。
5. `Visibility` 语义保持：`self` → `celebration`（仅自己可见）、`team` → `chat`（全队可见）。**新增播报项若涉及其他玩家信息，必须限制在 `team` 语义下且不得泄露非公开身份**；无法确定时**默认 `self`**。
6. 设置页必须逐项声明「开启后会向英雄选择聊天发送什么内容」，文案与实际发送内容**逐字一致**（声明准确性红线）。
7. **模式限制保持**：现有描述限定「极地大乱斗、海克斯大乱斗」。新增播报项若适用于其他模式，必须显式扩展并在 UI 说明，**不得静默扩大适用范围**。
8. 诊断：沿用 `:1323` / `:1329` 的 `watch_action` 记录格式，为每个播报项记录 `action` / `result` / `reason`；**现有 `result` 取值集合不得变化**（现有测试依赖它）。

### 验证判据

**必须 PASS：**
1. **护栏回归测试（最重要）**：断言新实现仍然 (a) 走 `safeLCUChatIdentifier` 校验、(b) 受 `customPaused` 两处暂停、(c) POST 失败 / 不确定时**不重试**、(d) 非法 `Visibility` 回落 `self`。
2. **不新增外部请求测试**：断言播报触发过程中对 Riot / OP.GG / SGP 等外部数据源的请求数为 **0**（只允许 LCU 本机端点）。
3. **单次单条测试**：一次触发只产生 1 次 POST；多项合并为一条消息体。
4. **默认行为不变测试**：升级后（未改动设置的用户）发送的消息与升级前**逐字一致**。
5. 每个新播报项在数据缺失时走降级（不发空消息、不发「未提供」占位冒充内容）。
6. UI 声明文案与实际发送内容一致（有测试断言同源，不得前端硬编码一份文案）。
7. `go test -count=1 .`、`go test -race -run 'TestR115' -count=1 .`、`node --test web/*.test.cjs` 全绿。

### 对抗变异

1. **变异 #1：POST 失败后重试** → 「不确定不重试」护栏测试必须失败。
2. **变异 #2：为生成播报内容发起外部数据源请求** → 外部请求数为 0 的断言必须失败。
3. **变异 #3：一次触发连发多条消息** → 单次单条测试必须失败。
4. **变异 #4：升级后默认多发了消息** → 默认行为不变测试必须失败。

---

## 产品口径裁决记录（N1 否决 / N2a→P9 / N2b 否决 / N3→P8）

本节 4 项已全部裁决完毕：**N1 已否决**（不做）、**N2a 已裁决做**（提升为 P9）、**N2b 已否决**（不做）、**N3 已裁决做**（提升为 P8）。本节仅作决策留痕，**不含可执行内容**；可执行内容见 P0–P9。

### N1. 对局分析独立置顶小窗 —— **用户已否决，不做**

2026-09-19 用户明确「不要」。理由与本项目判断一致：对方有 5 个窗口（`window-manager` 20 个文件，含位置记忆与背景材质），本项目 `desktop/main.cjs` 只有 splash + main 两个窗口；新增窗口若各自拉取数据会**直接烧穿 R110 才刚修好的 90/2min 额度窗口**。**本项已从范围移除，不得实现。**

### N2. 局内发送（in-game-send）—— **N2a 已裁决做（见 P9），N2b 已否决**

**先更正上一轮的错误判断**：我曾说这是「PRODUCT.md 边界之外的新写操作类别」，**错**。本项目 `positionBroadcast` **已经是一个向游戏聊天发消息的写操作**（`watch_rules.go:1378-1418`）：读 `/lol-chat/v1/me` 与 `/lol-chat/v1/conversations` → 找到 `type` 含 `champion`/`champselect` 的会话 → POST `/lol-chat/v1/conversations/{id}/messages`，内容为 `"当前阵营位置：蓝色方"`，`type` 按 `Visibility` 取 `celebration`（self）或 `chat`（team）。且已有良好护栏：`safeLCUChatIdentifier` 校验会话 ID、`customPaused()` 暂停、POST 结果不确定时**不重试以免重复发消息**（`:1413` 注释）。

因此「往游戏聊天发消息」这条线**本项目早已跨过且跨得干净**。真正的差别在两点：

**N2a —— 用户已裁决「做」，见 P9**

2026-09-19 用户明确「N2a 做」。**本项已提升为可执行的 P9 节**：把 `positionBroadcast` 泛化为可配置播报，复用现有 LCU 聊天 POST 通道，**不新增写操作类别**，并保留全部现有护栏。详见 P9。

**N2b（建议不做）—— 游戏内发送，需原生键盘注入**
- 对方 `send-executor.ts` 在游戏内走**完全不同的通道**：`nativeInput` + `IN_GAME_SEND_ENTER_KEY_CODE` **模拟键盘逐行打字并回车**（因为对局中 LCU 聊天不可用），靠 `isGameClientForeground` 判断前台 + 取消快捷键 abort 兜底。这是 24 个文件那一套：预设面板（打野 / 评分 / 预组队 / 名称展示 / 固定文本）+ 每预设独立快捷键 + 自定义模板。
- **本项目没有这条通道，也没有原生键盘注入能力**（对方依赖自维护的 `native/win32-x64` .node addon）。
- 风险等级不同：键盘注入会**往当前前台窗口打字**，一旦前台判断失效就会把内容打进别的程序。
- **结论：N2b 不纳入本轮。** 若未来要做，必须单独排单，且先在 Windows 真机验证前台判断与取消快捷键。

### N3. 玩家标记库（saved-player）—— **用户已裁决「做」，见 P8**

2026-09-19 用户明确「标记也做」。**本项已从「待确认」提升为可执行的 P8 节。** 隐私口径结论：**不需要放宽** DESIGN.md 红线 —— 本项目 `storage.go:126-145` 已有 `account-salt`（32 字节随机、`0o600`），`storage.go:350-362` 已有 `accountHash()`（加盐 SHA-256 取前 16 位）；标记库只需要「同一个人再出现时能匹配上」，**不需要反查 PUUID**，因此可完全复用现有原语，不新增任何加密实现。相遇对局历史（`EncounteredGames`）本轮不做，详见 P8-B。

---

## 明确不做（Anti-scope）

1. **不抄对方品牌、颜色、图标与页面编排**（PRODUCT.md Anti-references ②明确禁止）。
2. **不引入 Vue / MobX / Pinia / Naive UI 重写前端** —— 零框架原生 JS 是既有选择，重写等于瞎改。
3. **不换 SQLite** —— 会破坏 `CGO_ENABLED=0` 交叉编译（对方需为此维护 `trim-packaged-app.cjs` 裁 prebuild）。
4. **不做 i18n 双语**、**不做 macOS 支持** —— 均为既有设计决定（Windows 真机优先、全中文文案）。
5. **不引入第三方路由/DI/队列框架** —— `go.mod` 只有 4 个依赖是项目优点。
6. **不重复实现已有能力** —— 见文首「前置更正」表。
7. **本工单不包含大重构** —— 275 个 `.go` 全在根 `main` 包（10.2 万行）是长期技术债，但拆包属于大重构，**不在本轮范围**，只记录在案。
8. **不做对局分析独立置顶小窗（原 N1）** —— 用户 2026-09-19 明确否决。
9. **不做游戏内发送 / 原生键盘注入（原 N2b）** —— 风险等级与现有 LCU 聊天写操作不同，需单独排单。
10. **不做差分更新 / 增量安装包（原 P0-A）** —— 用户 2026-09-19 明确否决。**不得改动 `desktop/package.json` 的 `build.nsis.*`，也不得改动 `update.go` / `update_download.go` / `update_http.go`**；`differentialPackage` 必须保持 `false`。

---

## 通用验证要求

### 编译与测试

1. `go build ./...`，exit code 0
2. `go vet ./...`，无警告
3. `go test -count=1 .`，全绿
4. `go test -race -run 'TestR115' -count=1 .`，无数据竞争
5. `node --test web/*.test.cjs`，33 个文件全绿
6. `node --test desktop/*.test.cjs`，35 个文件全绿
7. `gofmt -l .` 无输出（构建脚本已有 gofmt 门禁）

### 额度守护（硬性）

1. 本轮所有改动**不得增加任何外部数据源请求**。
2. 所有新测试**不得发起真实网络请求**（必须 httptest 或内存桩）。
3. 诊断日志口径必须区分**本地限流**与**上游 429**，沿用 R108/R110/R111/R112 反复强调的口径（日志里的 `riot_local_rate_limited` 不得表述为「Riot 返回 429」）。

### 对抗变异

执行各 P 节列出的全部变异（共 **33 个**：P0 2 个、P1 3 个、P2 4 个、P3-A 3 个、P3-B 3 个、P4 3 个、P5 3 个、P6 3 个、P8 5 个、P9 4 个），确认每个变异都能被对应测试抓住，且 baseline（未变异）版本全部通过。变异矩阵输出到 `docs/r115-validation/mutations/matrix.json`。

### 构建打包

**按项目惯例，本轮只跑测试，不构建、不打包**（用户通常自己打包）。原 P0-A 的「打包两次实测差分收益」已随该项否决而取消 —— **本轮无任何需要用户配合打包的验证项。**

---

## 真机验证清单

### P7 卫生
- [ ] `git status` 中不再出现 `.mut102gopath`
- [ ] `.gotoolchain` 性质已记入账本（误提交的 Go 1.24.0 工具链源码树），**由用户自行提交删除，执行方未介入任何 git 操作**
- [ ] 占位测试文件已清理，测试仍全绿

### P0 首屏瘦身
- [ ] 首屏 JS/CSS 字节数前后对比
- [ ] 7 个一级导航逐个深链刷新，无白屏、无重复注入、无 404
- [ ] `demo-data.js` 不再进入生产加载

### P1 护栏与路由
- [ ] bench 报告可复现，与基线对比可见
- [ ] 对局中（InProgress）LCU 事件路由行为与改动前一致
- [ ] 诊断中的事件 URI 统计仍可读

### P3 桌面外壳（复活浮窗 + 托盘 + 快捷键）
- [ ] 真机探针记录 `isDead` / `respawnTimer` 实测值
- [ ] 进入对局后浮窗出现并倒计时
- [ ] 离开对局后浮窗停止并清零
- [ ] 设置关闭后无 2999 请求
- [ ] 浮窗不显示端口 / 令牌 / 本地路径
- [ ] 托盘图标常驻，菜单文案全中文，图标为现有 `hexcore-icon.ico`
- [ ] 关闭主窗口的行为与设置项一致（最小化 or 退出，无歧义状态）
- [ ] 自定义快捷键注册成功；被占用时有可见提示
- [ ] 应用退出后无残留全局快捷键钩子
- [ ] 全局快捷键只触发窗口显隐，未触发任何写操作

### P4 图片预热
- [ ] 冷启动后打开页面，首次仍需下载的图片数明显下降
- [ ] 磁盘预算未被突破，`proseed-` 条目完好
- [ ] 设置关闭预热后启动不产生图片请求

### P2 打野分析
- [ ] 打野位玩家的战绩详情能看到首轮清野与三分区
- [ ] 非打野位玩家不显示野区分析
- [ ] ARAM 系对局显示明确降级状态
- [ ] 时间线不可用时显示降级而非空白/0

### P5 DL 评分
- [ ] 悬浮说明能看到每个分项的原始值与得分
- [ ] 场次不足时显示降级而非 0 分
- [ ] 不同队列/模式样本未混算

### P6 远程配置与迁移
- [ ] 断网启动不报错、不弹窗
- [ ] 诊断日志可还原配置来源链路
- [ ] 旧缓存文件升级后功能正常，`proseed-` 完好

### P8 玩家标记库
- [ ] 落盘文件 `player-tags.json` 权限 0600，内容不含 PUUID / 召唤师 ID 明文
- [ ] 标记键为 16 位十六进制，与 `accountHash()` 输出一致
- [ ] 同一玩家在 PUUID 有 / 无两种输入下未产生两条互不相认的标记
- [ ] 对局中遇到已标记玩家时卡片显示标记文案（不只靠颜色）
- [ ] 无标记时不显示空标签或占位
- [ ] `lastMetAt` 为真实相遇时间，不等于写入时间
- [ ] 超限后淘汰最旧标记，最近标记完好
- [ ] 损坏文件不阻塞启动，标记被清空重写
- [ ] 设置页声明与实际写操作逐条一致

### P9 播报泛化
- [ ] 升级后未改动设置的用户，收到的消息与升级前逐字一致
- [ ] 一次触发只发 1 条消息（多项合并为一条，不刷屏）
- [ ] 播报触发过程中对外部数据源（Riot / OP.GG / SGP）请求数为 0
- [ ] POST 结果不确定时未重试，未产生重复消息
- [ ] `customPaused` 暂停生效（两处检查点均覆盖）
- [ ] 非法 `Visibility` 回落 `self`，未误发全队消息
- [ ] 设置页逐项声明的发送内容与实际消息体逐字一致
- [ ] 数据缺失的播报项未发送空消息或占位文本

---

## 注意事项

1. **数据准确性红线**：证据不足时明确降级（显示「未提供」/ 横线），**不得用推断值或 0 代替**。P2 的坐标阈值、P5 的评分公式尤其适用。
2. **日志完备**：新增的每个异步路径（预热、浮窗轮询、远程配置、迁移）都要有可还原的日志，避免二次返工。
3. **隐私声明与实际写操作必须一致**：P3 新增 2999 依赖、P4 新增预热流量、P8 新增本地标记落盘、**P9 会向游戏聊天发消息**，都必须在设置页更新声明；**不得出现声明与实际不符**。
4. **写操作安全**：P2 / P3-A / P4 / P5 全部是只读计算或本机读取，**不得产生对客户端或用户数据的写入**。P3-B 与 P8 只允许写**本项目自己的本地存储**（设置项 / `player-tags.json`）。**P9 是本轮唯一的客户端写操作**，且必须严格限定在既有 `/lol-chat/v1/conversations/{id}/messages` 通道内（见 P9 实现要求第 1 条），**不得新增其他 LCU 写端点**（如符文、装备方案、头像、旗帜）。若任何一条实现中发现需要超出上述范围的写入，**必须停下报告**。
5. **视觉纪律**：新增 UI 沿用海克斯黑金风格，与现有样式/高度/颜色统一，全中文文案；**不得模仿对方**。
6. **不遗漏**：本工单 **P0–P9 共 10 节**必须逐条执行。任一节无法完成时，在账本中明确写出**未完成项与原因**，不得静默跳过。
7. **对照检查**：对方 v1.5.0 曾修「英雄选择秒退后自动匹配可能继续执行」。本项目 `watch_rules.go:373` 有 `auto-matchmaking`，而全项目 `秒退` / `dodge` **零命中**。作为**核查项**（不是新功能）：确认 ChampSelect → 秒退 → Lobby 过程中该规则是否会被重新触发；若确认无风险，**把证据写进账本，不要为凑改动而加代码**。

---

## 交付物

1. 代码改动（对应各 P 节）
2. 新增测试：`bench_core_test.go`、`TestR115_*` 系列、`match_timeline_jungle` 相关测试、P8 落盘与鉴权测试、`desktop/*.test.cjs` 新增项
3. `scripts/run-benchmarks.sh` 与基线报告
4. 探针文档：`docs/r115-validation/respawn-probe.md`
5. 执行账本：`docs/r115-execution-ledger.md`
6. 验证 artifacts：`docs/r115-validation/`（含 bench 报告、首屏字节数对比、深链清单）
7. 对抗变异结果：`docs/r115-validation/mutations/matrix.json`
8. 更新后的 `CHANGELOG.md`、`PRODUCT.md`（如涉及声明）、`DESIGN.md`（如涉及隐私边界）
9. 更新后的 `.gitignore`
10. P8 / P9 交付物：`player-tags.json` 读写实现与身份形态一致性测试、播报可配置项与设置页逐项声明文案、P9 护栏回归测试

---

## 独立核对记录（本工单的事实校验）

本工单的**每一处代码引用与否定性断言**都经过一次独立的只读核对（不信任撰写者自述）：约 40 处 `文件:行号` 引用、9 条「零命中 / 未找到」类断言、13 项数值断言。

**核对结果**：语义准确率高（约 40 处引用中 30 处逐字精确）；**「前置更正」表 4 条全部经得起核对** —— 即没有把项目已有功能说成缺口。

**已修正的 9 处不准确（本文档为修正后版本）：**

1. 「`respawn` 零命中」→ 实际 3 处，其中 `arena_truth_diagnostics.go:250` 是 P3-A 探针的现成入口（已改写 P3-A）。
2. 「全项目仅 2 个窗口」→ 实际 7 处 `new BrowserWindow`，`share-export.cjs:139` 是可复用的窗口范式（已改写 P3-A）。
3. P6「未见缓存 schema 版本」→ 已有 **5 套** per-store 版本，只缺统一入口（已改写 P6，并明令不得重新发明版本号）。
4. `champions.go:27-45` 只含 3 个响应上限 → 其余 4 个出处已逐一改正为 `riot_api.go:128,130` / `qq101.go:18` / `aramkit_rating.go:22` / `feature_gates.go:19`（已改写 P4）。
5. `dist/desktop/` 内容描述错误 → 实际 5 个文件，无 `.nsis.7z`、无 `win-unpacked/`；「无 `.blockmap`」的结论成立（已改写 P0）。
6. 三处行号越过文件末尾 → `riot_limiter_queue.go`（110 行）、`binary_disk_budget.go`（58 行）、`live_prewarm.go`（106 行），已改为「文件级引用 + 函数名 + 正确行号」（已改写背景 / P4）。
7. `gameplay.js` / `app.js` 行数与体积已过期 → 更正为 6935 行 / 436KB、3835 行 / 211KB（已改写 P0）。
8. P4「`data/` 只有奖池与皮肤发售日 JSON」→ 实际 6 项（已改写；实质结论「无离线图片包」不变）。
9. `desktop_startup_stage.go:11-30` → 里程碑白名单实际在 `:32`（已改写背景）。

**独立核对同时确认成立的断言（执行方可直接依赖）：**
- `秒退` / `dodge` 在项目 `.go` 与 `web/*.js` 中 0 命中（注意事项第 7 条的核查项成立）
- `match_timeline.go` 内 `jungle` / `打野` / `monsterType` / `participantFrames` 0 命中（P2 成立）
- `blockmap|differen|delta|差分` 在 update 相关文件中 0 命中（事实成立；但原 P0-A「差分更新」已被用户 2026-09-19 否决，见 Anti-scope 第 10 条）
- `desktop/` 全目录 `Tray` / `globalShortcut` 0 命中（P3-B 成立）
- `.go` 中 `player-tags|playerTag|PlayerTag|标记库` 0 命中（P8 成立）
- `web/*.test.cjs` = **33**、`desktop/*.test.cjs` = **35**（精确）
- `go.mod` 恰好 4 个 require；`web/image-queue.js` 并发 6 / 160px / 10s×1 / 10min 全部无误
- `installer/` 顶层 17 个测试、`tools/` 4 个测试
- `r87_independent_*_test.go` 只有 2 个且均为占位（P7 第 4 条已有答案）
- `POST /api/gameplay/match-timeline` 在 `main.go:510` 且已包在 `a.authorized` 内（P8 第 7 条的现成范式）

**未能穷尽验证的项（执行方接手时请自行确认，不得当作已验证）：**
- `.gotoolchain` 的 14,087 个删除、106 个修改、128 个未跟踪、HEAD `bfcb095`、`.git` 788MB / 18 个提交、`.mut102gopath/` 256MB 未被忽略 —— **以上均已用 git / shell 实测核实**（先前「未能验证」的标注作废）
- `web/` 合计 5.0MB（9 个大文件合计 1.46MB 已核，其余未逐一累加）
- 根目录 144 个 `*_test.go`、「只有 1 个 `func Benchmark`」的**唯一性** —— `.mut102gopath/` 下有数千个 `*_test.go`，只读搜索无法排除污染
- R107 账本的 103,408,128 B / 19,712,512 B 未打开 `docs/` 账本逐字核对
- LeagueAkari 侧的全部引用（对方仓库不在工作区，行号以其 `dev` 分支 2026-09-19 快照为准）

---

## 工单结束标志

- [ ] **P0–P9 共 10 节**全部实现要求完成，或未完成项已在账本中明确写出原因
- [ ] 所有验证判据通过（编译 / vet / test / race / 前端测试）
- [ ] 33 个对抗变异全部被抓住，baseline 全绿
- [ ] 额度守护三条硬性要求全部满足
- [ ] 真机验证清单逐项勾选，**并明确标注哪些因无 Windows 真机而未验证**
- [ ] 执行账本与验证 artifacts 已提交到 `docs/`
- [ ] 已确认本轮未触碰「明确不做」与「前置更正」两节中的任何一项
