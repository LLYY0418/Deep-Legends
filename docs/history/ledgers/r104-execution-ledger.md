# R104 执行账本

> 证据已于 R135 移出工作区，见提交 `b62bca1b9671bccd8c3cf9f7079096d4b33a6fa0`。

依据：`WORKLIST-R104-PRO-RECENCY-QUOTA-BLOWOUT-AND-ICON-POISONING.md` 与 `WORKLIST-R104-ADDENDUM-OPGG-REVISION-AT-REPLACES-RIOT.md`。附录覆盖主工单 P0 的数据来源和涓流速度，其余要求继续执行。

## 最终解释与实现

- **30 个令牌**指共享 Riot 两分钟长窗口的当前占用量。后台在共享占用量达到 30、短窗口达到 15，或前台 FIFO 中有人等待时立即返回让路结果。后台不进入前台 FIFO，不建立独立的 30/2 分钟累计桶。前台仍可使用原有 90/2 分钟总量，15/秒上限不变。
- **每分钟 1 个账号**是累计刷新速度。独立 worker 第一次先等完整 60 秒；每轮最多选一个目录未覆盖的种子账号，之后再等一分钟。账号调用超时仍为 4 秒，按最久尝试/上次快照更新时间轮询；目录正在更新时不覆盖其快照，退出时随 runtime context 停止。
- `pro_players.go` 启动发布阶段仅解析目录、补入种子快照，不同步刷新全部种子。原先两处按账号数放大的整轮 context 已移除；`proSeedContext` 改为固定 20 秒上限，worker 的父 context 进一步限制为 4 秒。R102 的线性预算断言已由固定上限断言替代，因为本工单明确撤销了原要求。
- 同一份 OP.GG 职业目录载荷的 `revision_at` 经 RFC3339 解析后进入现有 `setProLastMatch`；同时复用 `solo_tier_info`、`level`、`puuid`。账号 key 只进行大小写和首尾空白归一化，之后精确比较 `game_name#tagline`。缺字段、无效时间、不同标签均不冒充命中。
- 命中的种子不请求 Riot，目录 PUUID 可直接写入缺失/到期的 30 天 `proseed:v1:` 锚点。已有有效锚点继续保留，不为更新锚点发 HTTP。
- `enrichProActivity` 的调用已从启动链移除，函数体保留供显式兜底复用；通过真实启动调用链的计数 transport 验证，并通过“把调用重新插回启动链”的变异验证。
- `proseed-lastmatch:v1:` TTL 为 **7 天**；已定级 `proseed-rank:v1:` TTL 为 **24 小时**；未定级仍为 **72 小时**。总览独立缓存及提前观察到定级后的刷新规则保持原有语义。
- 种子与活跃度路径分别记录 `pro_seed_cost`、`pro_activity_cost`，包含 `accounts_total`、`accounts_refreshed`、`riot_requests`、`skipped_by_budget`。HTTP 计数在真正调用 transport 前增加；未获准的后台请求只记跳过。

## 图标与额度恢复界面

`web/image-queue.js` 将 DOM 脱离取消与 error/timeout 分开。取消只释放在途名额并清除元素处理记录，不写 URL 失败表；真实失败进入 10 秒冷却，等待图片保留在 pending，并由到期计时器自动重新调度。每个元素最多自动重试一次，成功后清除重试记录；新元素可以正常再次排队。图片并发仍为 **2**。

`web/gameplay.js` 首屏无数据且等待额度恢复时同时渲染提示条和骨架屏。已有数据时仍渲染战绩，提示条不替换旧列表。

## 数据来源与兼容性回应

1. `revision_at` 是 **OP.GG 维护的已知最近活跃时间**，可能滞后，仅作为排序依据，不宣称是 Riot 精确对局开始时间。当前 `web/pro-players.js` 不展示 `lastMatchAt` 的精确时刻；卡片中的“更新”仍来自 `updatedAt`。
2. 保留 R102 的 `riotMatchInfo.GameStartTimestamp` 和 match-v5 兜底，供目录未覆盖账号使用。账号仍按已知时间降序排列，未知时间排在所有已知时间之后；未修改这条排序规则。
3. 本轮没有添加为未命中账号抓取 OP.GG 单人页面的路径。职业目录请求仍为原有一次；原有补充来源和 ladder 流程保留。未对 OP.GG 反爬行为作新的保证。
4. 旧磁盘快照没有 `revision_at`、没有活跃度扩展字段时仍可反序列化；账号保留且时间未知，不伪造 1970 年。已有 R102 私有活跃度快照扩展继续支持。
5. 33 人、53 账号清单和 `docs/pro-accounts-verification-2026-09-17.md` 未修改。R95–R103 已有未提交改动、缓存和工具链文件状态均保留。

## 真实 fixture

来源：`https://op.gg/zh-cn/lol/spectate/list/pro-gamer?region=kr`，本轮抓取的原始 HTML 裁剪出 BLG、DK、GEN、HLE、IG、T1 的原始 JSON 对象，保留账号结构、`revision_at`、`solo_tier_info`、`level`、`puuid`。

- 原始 HTML 临时文件：`/tmp/r104-pro-directory.html`；SHA-256：`092deb9590250e724723b55f0d9f81c1a9ce7854a8182cd5a344ceb2c7c735eb`。
- 可重复使用的 fixture：`testdata/r104-opgg-pro-directory.json`；SHA-256：`b0c07a05aa56d5def467259afc1f0271830cef8c25e969f6825f18f3a64314b7`。
- fixture 中 `Kimman#zxfkk` 的 `revision_at` 为 `2026-09-17T03:56:23+09:00`，断言投影为 `2026-09-16T18:56:23Z`，并验证复用字段、零 Riot 请求、30 天锚点落盘。

## 验证

所有网络计数测试使用本地 fake transport 和测试密钥，不消耗真实 Riot 配额。冷启动综合测试通过可控时钟运行真实目录发布、后台 worker、缓存、限流器和 20 场总览路径：

- 启动目录全流程：**0 次 Riot**，OP.GG 目录请求 1 次。
- 第 60 秒：刷新一个未命中账号，**4 次 Riot**。
- 第 120 秒：有真实前台搜索在 FIFO 中等待，后台立即让路，累计仍为 **4 次**，小于上限 12。
- 前台：**20/20 场、25 次请求、零限流失败**。

R104 的 jsdom 测试使用生产队列文件，连续三次替换仍在途的同 URL 图片，验证失败表未被写入、最终 3 张图片均被赋予 src 并触发 load。真实 error 的测试通过可控计时器验证 9999ms 内不重试、第 10000ms 自动重新赋值 src，无需 DOM 变化。

真 Chromium 复跑 `desktop/r100-browser.cjs`：图片峰值并发 **2**；状态接口 **2.4ms** 返回，恢复提示、确认退出后的 fatal 与 restart 行为均通过。结果见 `docs/r104-validation/browser/result.json`。Chromium 在沙箱内启动失败后，经自动审批允许在沙箱外执行本地测试，已完成验证。

最终验证结果（日志集中于 `docs/r104-validation/`）：

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| `go test ./...` | 通过，116.588s | `go-full.txt` |
| `go test -race . -run 'TestR104\|TestR102\|TestR103' -count=1` | 通过，7.026s | `go-race.txt` |
| R104 定向用例及 R101 过期段位额度保护 | 通过，1.814s | `go-targeted.txt` |
| `node --test web/*.test.cjs desktop/*.test.cjs` | 727 通过，0 失败，1 跳过 | `node-full.txt` |
| Chromium 图片连接池及恢复路径 | 通过 | `browser/result.json` |
| `python3 scripts/r104-mutation-check.py` | 12/12 变异均被有效断言抓住，baseline 全通过 | `mutations/matrix.json`、`mutation-run.txt` |
| `git diff --check`、mutation 脚本语法检查 | 通过 | 本轮命令输出 |

前端唯一跳过项为 R86 的 Windows/PowerShell 发布门禁测试；未运行全仓 race，race 覆盖本次及 R102/R103 的相关测试。全量 Go 在沙箱内因 `httptest` 监听回环端口被拒，随后经自动审批在沙箱外完成，最终没有未解决的验证阻塞。

12 个变异覆盖：删除后台优先级、恢复整批刷新、恢复启动活跃度调用、删除目录精确命中、取消写失败表、冷却不重新 pump、额度提示替换骨架、后台用满长窗口、去掉启动等待、段位 TTL 回退到 6 小时、活跃度 TTL 回退到 6 小时、整轮超时预算重新放大。所有失败均是预期行为断言失败，未把编译错误、超时或 panic 当成抓住变异。

## 本轮调整的既有测试

R101/R102 中与旧 6 小时 TTL、按账号数线性扩张预算直接冲突的断言按 R104 新要求重写，保留缓存命中、磁盘重启、过期重取、总览命名空间隔离、72 小时未定级退避等检验；相关 mutation 脚本同步更新目标名称与源码片段。未通过删除断言规避回归。

未打包、发布、推送或调用真实 Riot 网络；此账本不代表真实用户配额与线上数据刷新延迟的压力测试结果。
