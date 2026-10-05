# WORKLIST-R206：Riot Key 改为中转服务；收藏首次进入误报"立即重新读取"；头像页首屏长时间空白；斗魂英雄详情首次 502；S 评级字号偏小；升级时桌面图标变白、写入阶段慢

诊断人：Claude（看截图 + 读日志 + 只读核对源码）。执行人：GPT。日期：2026-10-04。基线：0.12.68（`1ba24a67`，业务源码 `bb2b97a0`）。

日志 `lol-loot-diagnostics-1004-1451.jsonl`：10-03 为 0.12.63 / 0.12.65；**10-04 06:46 从 0.12.65 在线升级到 0.12.68**，06:47:13 起为 0.12.68（`a24a41472d2b`）。

## 已在真机确认正常（不改）

- **R205 快捷方式**：`update_shortcut_state`：桌面和开始菜单快捷方式升级前后创建时间完全相同（`created_time_changed=false`，`restore_results.current=unchanged`），`keep_shortcuts_reg=true`。这次升级没有删除或重建快捷方式。
- **海斗 / 大乱斗选人（R200/R202/R204 P9）**：
  - 0.12.68：卡片 `[41,233,13]` → 选 13（序列内），`postflight applied`；卡片 `[234,90]` → `no-pool-champion`，不发请求（临时测试选人已删除）。
  - 0.12.65：序列英雄在卡片里时都选中并生效（22、57、136）；备战席换人 268、13 都 `bench-postflight applied`。
  - R202、R204 P9 的海斗部分可以按这份日志关闭。

---

## P1　Riot API Key 改为中转服务（用户已选定方案）

### 背景

- 用户反馈：大部分用户不会申请 Riot Key，绝活哥 / 职业选手战绩显示"未配置 Riot Key"（截图），希望软件自带。
- 0.12.68 日志：`app_start riot_key_source=none`，`riot_overview_cost error_kind=riot_key_missing`。
- 把 Key 放进安装包，无论怎么加密，运行时都要还原成明文发给 Riot，有心人可以从内存或网络层拿到；Riot 要求 Key 必须妥善保管，没保管好会被吊销，届时所有用户同时失效。
- 用户选定：**Key 只放在中转服务上，软件只请求中转服务**。

### 修改

1. **中转服务代码**（新目录 `relay/riot-worker/`，Cloudflare Worker，JavaScript）：
   - Key 只从 Worker 的 secret `RIOT_API_KEY` 读取，代码和仓库里不出现 Key。
   - 只转发白名单路径（与 `backend/riot_api.go` 实际用到的一致）：
     - `asia.api.riotgames.com`：`/riot/account/v1/accounts/by-riot-id/…`、`/riot/account/v1/accounts/by-puuid/…`、`/lol/match/v5/matches/by-puuid/…/ids`、`/lol/match/v5/matches/{matchId}`、`/lol/match/v5/matches/{matchId}/timeline`（实际用到才放）
     - `kr.api.riotgames.com`：`/lol/summoner/v4/summoners/by-puuid/…`、`/lol/league/v4/entries/by-puuid/…`、`/lol/league/v4/entries/…`、`/lol/champion-mastery/v4/champion-masteries/by-puuid/…`、`/lol/status/v4/platform-data`
     - 其他路径、其他主机、非 GET 一律 404。
   - 请求格式：`GET https://<中转域名>/r/<asia|kr>/<原路径>?<原查询串>`。
   - **缓存**：用 Workers Cache API。比赛详情 `matches/{matchId}`、`timeline` 缓存 7 天；比赛 ID 列表、段位、熟练度缓存 60 秒；账号缓存 1 小时。命中缓存不消耗 Riot 配额。
   - **限流**：每个来源 IP 每分钟最多 120 次（超出返回 429 和 `Retry-After`）；Riot 返回 429 时原样转回 `Retry-After`，Worker 在该时长内对同一路径直接返回 429，不再打 Riot。
   - 不记录请求者 IP、Riot ID 等到任何持久存储；Worker 日志只记路径类别、状态码、是否命中缓存。
   - `relay/riot-worker/README.md` 写部署步骤（给用户本人操作，GPT 不替用户登录）：
     1. 注册 Cloudflare，`npm i -g wrangler`，`wrangler login`
     2. `wrangler secret put RIOT_API_KEY`
     3. `wrangler deploy`
     4. 在 Cloudflare 里给 Worker 绑定**自己的域名**（`*.workers.dev` 在国内经常无法访问，必须用自有域名）
     5. 把域名写进软件的内置中转地址列表（第 2 条）后重新发布
   - 写明 Key 类型要求：开发者门户默认的 development key **每 24 小时失效**，不能用于中转；需要申请 personal key（20 次/秒、100 次/2 分钟，适合少量用户）或 production key（面向公开用户）。
2. **软件端**（`backend/riot_api.go`、`riot_key_settings.go`）：
   - 凭据优先级改为：环境变量 `RIOT_API_KEY` → 用户在设置里保存的 Key（直连 Riot，R204 逻辑不变）→ **内置中转服务** → 无。
   - 内置中转地址是构建时写入的一个列表（第一版放一个占位常量，用户部署后填入；为空时视为未配置）。不是秘密，不需要加密。
   - 走中转时请求头**不带** `X-Riot-Token`；路径映射到 `/r/asia/…`、`/r/kr/…`。
   - 启动后第一次需要时探测中转：`GET /r/kr/lol/status/v4/platform-data`，3 秒超时；失败后 5 分钟内不再重复探测，直接显示不可用。
   - 中转返回 429：按 `Retry-After` 退避，和现有 Riot 429 处理共用逻辑。
   - 诊断：`app_start riot_key_source` 增加 `relay`；新增 `riot_relay_probe`（`result`、`duration_ms`、`http_status`，不记域名）。
3. **界面**（隐私与能力 → Riot API Key）：
   - 走中转时状态显示"使用内置服务"，输入框仍可填自己的 Key（填了就优先直连）。
   - 绝活哥 / 职业选手页：中转可用时不再出现"未配置 Riot Key"；中转不可用时显示"战绩服务暂时不可用"和「重试」。
   - 不加任何说明文字。

### 测试

- Worker（Node，mock `fetch` 和 Cache）：白名单路径转发并带 Key；非白名单路径、其他主机、POST → 404 且不调用 Riot；比赛详情第二次命中缓存；单 IP 超限 → 429；Riot 429 → 原样带 `Retry-After`，退避期内不再请求 Riot；响应里不出现 Key。
- Go：
  - 没有环境变量和用户 Key、配置了中转 → 请求发往中转，不带 `X-Riot-Token`，`riot_key_source=relay`；
  - 用户保存了 Key → 直连 Riot，不走中转；
  - 中转地址为空 → `riot_key_source=none`，行为与 0.12.68 相同；
  - 中转探测失败 → 5 分钟内不重复探测。
- Node：走中转时设置页显示"使用内置服务"；中转不可用时绝活哥页文案正确。
- 变异：走中转仍带 `X-Riot-Token` → Go 测试 FAIL；Worker 放行非白名单路径 → Worker 测试 FAIL。
- public 构建 Key 门禁脚本保持不变（安装包里仍然不能有 Key）。

---

## P2　进入收藏时先出现"客户端已经连接 / 收藏信息还在准备中 / 立即重新读取"

### 证据

| 时间 (UTC) | 事件 |
|---|---|
| 06:47:13 | 0.12.68 启动，身份 10 ms 读完 |
| 06:48:36 | 用户进入收藏 → `collection_refresh_request source=ensure`（启动后 83 秒内从没读过收藏） |
| 06:48:38.5 | `refresh_succeeded` 1.97 秒，列表渲染 1183 项 |

### 根因

`backend/web/app.js` 收藏网格渲染（约 1086–1091 行）：只要 `connected && !snapshotReady && !staleSnapshot`，就显示"客户端已经连接 / 收藏信息还在准备中，正在自动重试 / 立即重新读取"。正常的第一次读取（约 2 秒）也走这个分支，看起来像出错了；按钮调用 `el.refresh.click()`，即全量重新读取。

### 修改

1. 第一次读取收藏期间（没有 `lastError`、刷新正在进行、开始不到 15 秒）：只显示现有骨架卡片（与 `state.loading` 分支一样），`listMeta` 显示"正在读取收藏"，**不显示按钮、不显示"重试"字样**。
2. 只有满足以下任一条件，才显示"收藏信息读取失败"和按钮：`lastError` 非空、`snapshotRetryCount > 0`，或已经读了 15 秒仍未完成。
3. 按钮改为只重新读取收藏（`POST /api/refresh?source=collection_retry`，只刷新收藏范围），不再触发 `el.refresh.click()` 的全量刷新。
4. **启动后预读收藏**：身份读完且空闲 10 秒后，后台读一次收藏快照（`source=startup_prefetch`）。用户进入收藏时快照已就绪，直接显示。对局中（ChampSelect 到 EndOfGame）不预读。

### 测试（Node）

- 首次进入、刷新进行中、无错误 → 只有骨架，没有"立即重新读取"、没有按钮。
- 读取 15 秒未完成 / 有 `lastError` → 出现失败文案和按钮；点击只发收藏刷新请求，不派发全量刷新。
- Go：身份就绪 10 秒后发起一次 `startup_prefetch`；对局中不发。

---

## P3　头像页首屏很久才出图

### 证据

| 时间 (UTC) | 事件 |
|---|---|
| 06:48:55 | 进入「头像与旗帜」，`/api/facade` 1 MB，135 ms 返回 |
| 06:49:04.51 | 一张本机来源（`image_source=lcu`）图片**等满 10 秒超时**，当时有 4 张图占着名额超过 3 秒（`active_slow_count=4`） |
| 06:49:04.53 | 下一张图排队 **8.7 秒**，加载只用 11 ms |
| 06:49:14.53 | 又一张排队 **10 秒**，加载 0 ms |

图片本身毫秒级就能取到，时间全花在排队上：5 个名额被几张一直不返回的图占满了 10 秒。

### 根因（源码已核对）

1. `backend/main.go handleImage`：`/api/image?path=…` 先向客户端要图，客户端没有（404）时，在**同一个请求里**转去 `raw.communitydragon.org`。国内访问 CommunityDragon 常常 3～8 秒超时（同一份日志里 `raw.communitydragon.org status=0` 两次，分别 7986 ms、2809 ms）。
2. `backend/web/image-queue.js`：这类 URL 没有 `source=` 参数，被算成"本机道"（`laneOf` 返回 `local`），所以 R127 给远程图设的"最多占 2 个名额"不起作用，5 个名额全被它们占住，首屏其他头像只能排队。
3. 按"最新在前"排序，第一屏正好是最新的 2026 图标，客户端本地没有图的概率最高。
4. 后端没有记录哪些头像走了 CommunityDragon 回退，日志里无法直接看到是哪几张。

### 修改

1. **后端拆开本机与远程**：`/api/image` 客户端返回 404 时**不再在同一请求里转 CommunityDragon**，直接返回 404，并带响应头 `X-Image-Fallback: communitydragon`。增强符文等已有专门回退逻辑的路径保持原行为（`augmentIconPathTemplate` 那一支）。
2. **前端**：本机道收到 404 且带该响应头时，把这张图改成 `/api/image?path=…&source=communitydragon` 重新入队，这时走远程道（最多 2 个名额），不再堵本机道。
   - 前端拿不到 `<img>` 的响应头：改为把这类头像的 404 交给已有的候选地址机制（卡片 onerror 换下一个候选），候选第二个就是带 `source=communitydragon` 的地址。执行时选一种，账本写明。
3. **本机道单张超时**：从 10 秒改为 **4 秒**（本机客户端正常是毫秒级）；远程道仍 10 秒。
4. **诊断** `image_proxy_cost`：每 30 秒聚合一次——本机命中数、本机 404 数、CommunityDragon 成功 / 失败数、CommunityDragon p50/p90 耗时、触发回退的资源类别（`profile-icon` / `banner` / `loot` / `other`，不记具体路径）。

### 测试

- Go：客户端 404 的头像 → 返回 404 + `X-Image-Fallback`，**不**发 CommunityDragon 请求；增强符文路径仍回退。
- Node（image-queue）：5 张远程图 + 20 张本机图同时排队 → 本机图在第一个 100 ms 内全部开始加载；本机图 4 秒超时；远程图同时最多 2 张。
- 变异：恢复同请求回退 → Go 测试 FAIL；本机超时改回 10 秒 → Node 测试 FAIL。

---

## P4　斗魂英雄详情第一次打开失败，重新加载才出来

### 证据

| 时间 (UTC) | 事件 |
|---|---|
| 06:49:59.47 | 打开斗魂英雄（id 3）详情 |
| 06:50:02.48 | `champion_upstream lol-api-champion.op.gg status=0 duration_ms=3000`；`api.your.gg status=0 duration_ms=3000` |
| 06:50:02.50 | `local_request_client endpoint=champions http_status=502`，前端显示加载失败 |
| 06:50:04.9～05.1 | 用户重新加载：your.gg 570 ms、op.gg 685 ms，都成功 |
| 06:50:11 | 斗魂第一名数据（your.gg）6.4 秒返回 |

### 根因

1. 软件刚启动约 3 分钟，到 op.gg / your.gg 的连接还没建立。第一次请求要做 DNS 和 TLS 握手，**3 秒上游预算**不够，两个来源同时超时。
2. 任一必要来源失败，整个详情就返回 502，前端整页显示失败，不显示已经拿到的部分。

### 修改

1. 斗魂详情上游请求：**每个来源第一次请求失败（超时 / 连接错误）时，立即自动重试一次**，重试预算 5 秒；两次都失败才算失败。只针对超时和连接错误，4xx 不重试。
2. 启动后空闲时预热连接：对 `lol-api-champion.op.gg`、`api.your.gg` 各发一个轻量 HEAD / 小 GET（复用现有缓存请求即可），失败不处理、不重试。
3. **部分结果**：头部 / 榜单数据已有、某个板块（出装、海克斯、第一名）失败时，返回 200 和已有板块，失败板块单独显示"加载失败 · 重试"，只重试该板块；所有来源都失败才返回 502。
4. 诊断：`champion_upstream` 增加 `attempt`（1/2）；详情增加 `arena_detail_partial`（失败板块列表）。

### 测试（Go）

- 第一次请求两个来源都超时、第二次成功 → 返回 200，`attempt=2`。
- your.gg 两次都失败、op.gg 成功 → 200，带失败板块标记，不是 502。
- 4xx 不重试。
- 变异：去掉重试 → 第一条 FAIL。

---

## P5　S 评级字号比其他评级小

用户截图：海斗英雄列表、海克斯推荐卡里的"S"明显比"A"小。

源码：`backend/web/tier-icons/yourgg-s.svg` 的 `<text font-size="11">`，`yourgg-a/b/c/d/f.svg` 都是 `13`（`op.svg` 两个字母用 11，合理）。

修改：`yourgg-s.svg` 改为 `font-size="13"`，其他属性不变。加一个 Node 测试：所有单字母评级 SVG 的 `font-size` 相同。

---

## P6　升级：桌面图标升级中变白；"写入程序文件"阶段占时最长；卸载结束时间点没记到

### 证据

用户截图：升级到 91%（"正在写入程序文件…"）时，桌面上 Deep Legends 快捷方式变成白色空白图标。

`update_install_timing`（0.12.65 → 0.12.68，`result=partial reason=missing_fields`）：

| 区间 | 耗时 |
|---|---|
| 安装程序启动 → 旧程序退出 | 1.2 秒 |
| 旧程序退出 → 开始卸载旧版本 | **8.0 秒** |
| 卸载旧版本 + 进入解压前 | 约 5.9 秒（`uninstall_old_done` 缺失，由总时长反推） |
| 解压（7z → 临时目录） | 7.2 秒 |
| 写入（临时目录 → 安装目录） | **18.6 秒** |
| 结束 → 启动新版 | 0.5 秒 |
| 合计 | 41.5 秒 |

### 分析

1. **图标变白**：快捷方式本身没被动过（R205 证据），但图标取自 `Deep Legends.exe`。升级时旧版本先被卸载（exe 被删），新 exe 要到写入阶段才出现，中间约 30 秒 exe 不存在，资源管理器就显示空白图标。装完后应恢复；**是否恢复需要用户确认**。
2. **写入 18.6 秒**：electron-builder 升级时先解压到 `$PLUGINSDIR\7z-out`，再 `CopyFiles` 到安装目录，同一批文件写两遍，杀毒软件还会逐个扫描（用户装了腾讯电脑管家）。
3. **退出后 8 秒才开始卸载**：这段在 NSIS 初始化和 `customCheckAppRunning` 里。非便携升级走 `_CHECK_APP_RUNNING`，会启动 PowerShell 查找进程，通常要数秒。而安装程序（Go 壳）已经等父进程退出了，这次检查是重复的。
4. **`uninstall_old_done` 缺失**：`installSection.nsh` 里 done 钩子的位置正确（在两次 `handleUninstallResult` 之后、`SetOutPath $INSTDIR` 之前），start 也写进去了，但 done 没被导入。原因需要执行时查明，可能的方向：
   - 旧版卸载程序（0.12.65 的卸载壳）清理了子进程 TEMP 里的阶段文件；
   - `FileOpen … a` 在卸载后失败；
   - 导入校验（`value < parent_exited`）拒收。

### 修改

1. **图标不变白**：安装 / 升级完成后，安装程序（Go 壳）把 `Deep Legends.lnk`（当前用户桌面、开始菜单）的 `IconLocation` 改为数据目录里的一份图标副本（`%LOCALAPPDATA%\deep-legends\app.ico`，从安装包资源写出，存在且一致就不写）。数据目录升级时不删，所以升级中图标保持不变。
   - 只改 `IconLocation`，目标、参数、AppUserModelID、创建时间不变（沿用 R205 的 ShellLink 写法，并保留原创建时间）。
   - 写完对该 lnk 调用 `SHChangeNotify(SHCNE_UPDATEITEM)`。
   - 诊断 `update_shortcut_state` 增加 `icon_location_stable`（布尔）。
2. **升级时直接解压到安装目录**：仅在 `${isUpdated}` 且旧版本卸载成功时，`extractUsing7za` 改为直接 `SetOutPath $INSTDIR` 解压，跳过 `7z-out` + `CopyFiles`。解压失败走现有的重试 / 失败恢复（R201 的恢复打开旧版逻辑）。手动安装和首次安装不变。用 `apply-update-timing-template.cjs` 同样的模板补丁方式实现，模板变化时构建报错。
3. **升级时跳过重复的进程检查**：命令行带 `--updated` 且 Go 壳已确认父进程退出（新增参数 `--parent-exited`）时，`customCheckAppRunning` 不调用 `_CHECK_APP_RUNNING`。手动安装、便携升级保持原行为。
4. **查明 `uninstall_old_done` 丢失**：在 Windows 构建流水线里做一次真实的"安装旧版 → 用新版升级"，确认阶段文件里 done 有没有写出、有没有被导入；按查到的原因修复。修复后 `update_install_timing result=ok`，`uninstall_old_ms` 有值。
5. 目标：同一台机器上升级总时长从 41 秒降到 25 秒以内。下次日志以 `update_install_timing` 为准。

### 测试

- Go：图标副本存在且一致 → 不写；lnk 只有 `IconLocation` 改变，其他属性和创建时间不变。Windows 原生测试纳入 installer 全量。
- Node：模板补丁后升级分支直接解压到 `$INSTDIR`、非升级分支仍走 `7z-out`；模板结构变化时补丁抛错。
- Node：`--updated --parent-exited` 时不调用 `_CHECK_APP_RUNNING`；手动安装仍调用。
- 流水线：真实升级后 `update-install-stages.txt` 含全部八个阶段。

---

## P7　收藏页"事件触发"的刷新没有来源诊断，1 分钟 3 次

0.12.68 日志在收藏页 1 分钟内有 3 次 `collection_refresh_request source=event`（06:48:43、06:49:00、06:49:32），每次约 2 秒，最后一次因 CommunityDragon 翻译超时用了 8.6 秒（`loot_metadata 8003 ms`）。但整份日志**没有一条 `collection_dirty_marked`**：R203 加的标记诊断和 5 秒抑制只覆盖了 dirty 路径，`source=event` 是另一条触发路径，既没有记录是哪个事件触发的，也不受限流。

修改：

1. 找到所有 `source=event` 的发起点，统一记 `collection_dirty_marked`（`uri_kind`、`during_refresh`、`suppressed`），并套用 R203 的规则：刷新结束 5 秒内的事件只记录不刷新；同一类事件 30 秒内最多触发一次刷新。
2. CommunityDragon 翻译表（`loot_metadata` 的 `translations`）读取失败时用上一次成功的缓存，不阻塞刷新；超时不超过 3 秒。

测试（Go）：

- 10 秒内连续 5 个收藏事件 → 最多 1 次刷新，每个事件都有 `collection_dirty_marked`。
- 翻译表超时 → 刷新 3 秒内完成，使用缓存。

---

## 收尾

- `node --test backend/web/*.test.cjs desktop/*.test.cjs`、`go test ./backend -count=1`、`go test ./installer/... -count=1`、`go vet ./backend`、`git diff --check` 全绿；Worker 测试单独列出命令。各 P 的变异按上文执行，结果写进账本。
- 版本递增；`docs/WORKLIST-INDEX.md` 加 R206。R202、R204 的海斗部分备注"10-04 日志真机通过"；R205 备注"快捷方式未重建，已真机确认"。账本 `docs/history/ledgers/r206-execution-ledger.md`。
- 中转服务的实际部署由用户完成。账本写明：中转地址常量为空时的行为；用户部署并填入地址后，需要再发一个版本。
- 按 R199 规则发布到 GitHub Latest（release id、`isLatest=true`、匿名 `latest.json` 版本号）。

## 真机验收（用户）

1. 按 `relay/riot-worker/README.md` 部署中转并填入地址后：不填 Key，绝活哥 / 职业选手战绩能正常显示；设置页显示"使用内置服务"。
2. 刚打开软件就进收藏：只看到加载骨架，没有"立即重新读取"；1～2 秒后出列表。
3. 头像页：首屏头像 1～2 秒内出来。
4. 刚打开软件就进斗魂英雄详情：第一次就能打开。
5. 海斗英雄列表里的"S"和"A"一样大。
6. 下次在线升级：桌面图标升级过程中不变白；导出日志里 `update_install_timing result=ok`，总时长明显缩短。
