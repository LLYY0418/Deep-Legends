# R92 执行与验收记录

日期：2026-09-14。对应 `WORKLIST-R92-C-GAPS-D-BUILD-PANEL-F-CARRYOVER.md`，逐项覆盖 G/H/I；I-3/I-4 只诊断，没有顺手改 UI。工作区原有 R87–R91 改动保留，未提交、打安装包或发布。

后续验收口径见 [R93 记录](r93-execution-ledger.md)：G-2 的 1070ms 等为历史观测，`<1500ms` 不再作为磁盘正确性的验收门槛。现行回归要求重建 provider 后 20 场 `disk`、0 次详情网络调用；真实采样脚本保留耗时记录，按完整 20 场磁盘命中判定。I-3 的三个模式来源调研也已补充在 R93，功能是否改变仍待用户确认。

## 总体状态

| 项目 | 状态 | 证据与限制 |
|---|---|---|
| G-1 | 实测完成，25% 目标未达成 | 同一二进制、真实 personal key、同一韩服玩家，每组至少 5 次；限速排队抑制收益，详见下方 |
| G-2 | PASS | 真实 20 场网络预热，终止并重启进程后 `matches_from_disk=20`、1070ms；禁用持久化变异为 0/1805ms |
| G-3 | PASS | 600/128MiB 具体实例、605 条写入、实际超过字节预算、重启字节账本都有测试；四类边界变异转红 |
| H 撤销 | 已整体移除 | 按最新版工单撤销总览推荐，专用文件/接口/状态/DOM/CSS/测试一并清理，最终验证见补充 |
| I-1 | PASS，有网络波动 | 三个完整旧/新样本中位数 12282→10834ms；账号/排名完整度保持；串行变异转红 |
| I-2 | PASS | 39 张真实图片冷启动 1169ms，重启 18.4ms；关闭磁盘变异重启 835.4ms，超过预先锁定门槛 |
| I-3 | 诊断完成 | skip 来自本地模式能力门禁；三次实际榜单请求均返回 5 人。不能据此宣称娱乐模式上游无数据 |
| I-4 | 根因定位；国服现场显示待确认 | 76000 为豹女默认皮肤；没有 LCU 目录时缺的是居中目录图/动画，普通原画兜底实际返回图片。历史国服有成功目录证据，当前 Mac 无 Windows LCU 实机显示证据 |

## G：真实 Riot 采样与硬边界

环境：macOS arm64，真实公开网络与本地 personal key。采样二进制 SHA-256 `8c50b206e74bee2dfdeb7afb2ec93cd888d0499b5aaa166850ed8cff74031410`。没有运行 League/Riot 客户端进程；诊断也为本机 LCU discovery unsupported。凭据仅进入子进程环境，未写入证据。每个冷样本独立数据目录，只复制相同的静态英雄 JSON，不复制身份或比赛详情；各样本开始间隔 45 秒。

第一轮交替 A/B（先 4/8、下一对先 8/4）原始 `duration_ms`：

| 有效并发 | 5 次冷样本 ms | 中位数 ms | 累计 limiter_queue_ms 中位数 |
|---|---|---:|---:|
| 显式 4 | 3595、2812、2742、2723、3036 | 2812 | 122 |
| 显式 8 | 2580、2717、2505、2871、2541 | 2580 | 3149 |

降幅 **8.25%**，不是 25%。两组实际 HTTP detail 峰值分别全部为 4、8，所有样本 20 场成功、0 磁盘命中、0 429。完整记录：`r92/riot-ab-samples.json`、`r92/riot-ab-summary.json`。另补五次完全取消并发环境变量的默认 8 测量，以严格覆盖默认配置；记录见下方追加结果及 `r92/riot-default-eight-samples.json`。

`riot_api.go` 的 `wait` 同时控制 15 次/秒、90 次/2分钟，身份、比赛列表、段位、熟练度与详情共享额度；并发 8 更容易同时等待短窗口。`limiter_queue_ms` 是各请求等待时间之和，不能从总耗时直接相减。依据已足以确认有效并发确实提高、限速约束仍在，不能用解除限速或伪装 production key 达标。仅设 production tier 也不会自动变成 20，必须同时显式设置并发值；本轮没有 production key，不冒充测量。

G-2 在第一轮最后一个全网络 20 场样本后立即终止/重启原进程，保留该样本数据目录；第二次 20 场全部磁盘命中，`duration_ms=1070`、墙钟 1080.4ms。真实禁用 `persistRiotMatch` 二进制用另一新目录跑全网络再重启，结果 `matches_from_disk=0`、1805ms，两个门槛均失败。见 `r92/riot-disk-mutant-verdict.json`。数据不靠 fixture 推算。

G-3 的 `TestR92RiotMatchConcreteDiskBudget` 直接锁定 600/128MiB，并写入 605 条、重启再写；`TestR92RiotMatchByteBudgetAcrossRestart` 写 48 个 2MiB payload，JSON/base64 后超过 128MiB，再重建缓存写两条。它独立触发字节淘汰，避免条数淘汰掩盖问题。放宽到 6000000/128GiB、只放宽字节、移除淘汰、只移除字节淘汰、丢失重启字节统计都被捕获。

## H：按新版工单整体撤销

用户看过实际区块后明确要求删除，最新版 H 组优先于原实现要求。已删除专用后端文件、路由注册、三个前端辅助函数、请求和 tab 状态、取消请求钩子、总览占位与全部专用 CSS；保留相邻的当前对局占位。专用 Go/JS 测试、演示请求桩及变异脚本也同步移除；英雄详情中仅为共用而抽取的 reader 已还原为原有内部读取逻辑。英雄页与对局页的推荐功能保留。

真实 Chromium 验收改为断开客户端状态，分别从搜索、职业账号、英雄详情打开韩服玩家；核对最近20场正常展示、三首屏请求仍并行、当前对局占位紧邻战绩列表、无额外推荐区块或请求，并覆盖空战绩和失败补全重试。后续段落及早期原始测量中的旧功能信息仅是撤销前的历史证据，不是当前交付功能。

## I-1：职业榜账号流水线

账号列表不完全由 directory 决定：三个 supplement 可以新增账号。因此目录返回就启动已审核账号，partial 回调继续提交新增账号；队列由固定 8 workers 消费，按 Riot ID 去重。身份筛选仍经过完整的一队名册/冲突合并。排名结果保存在独立 map，只应用到发布前私有快照，既不阻塞 supplement 入队，也不改变旧快照。

诊断 `duration_ms` 保持“从目录开始到本阶段结束”的原定义，增加 `stage_duration_ms`（ladder 启动到结束）和 `wait_after_supplements_ms`，不把累计时间换成单段时间制造收益。

| 完整真实样本 | directory ms | supplements 累计 ms | ladder 累计 ms |
|---|---:|---:|---:|
| 旧 1 | 2184 | 4259 | 12282 |
| 旧 2 | 7591 | 10894 | 23684 |
| 旧 3 | 1903 | 3245 | 8575 |
| 新 1 | 3603 | 5377 | 12550 |
| 新 2 | 2026 | 3409 | 7857 |
| 新 3 | 3387 | 4754 | 10834 |

中位数 **12282→10834ms（11.79%）**；新最快 7857ms 也低于工单历史 8456/12999ms。网络波动较大，不能许诺每次更快。另两次新版本目录读取失败（12001/3001ms），没有进入 ladder，完整保留但不计入完成耗时中位数。后两组可比较样本均 `accountCount=90`、`knownRanks=27`、`partial=false`，其余没有可用天梯名次的账号继续显示“—”，没有牺牲数据量。见 `r92/pro-samples.json`、`r92/pro-summary.json`。

`TestR92ProLadderStartsBeforeSupplementsFinish` 使 supplement 等待目录账号的排名请求；改回严格串行即失败。另测增量账号、去重、最终排名与已发布快照不被回写，定向 race 通过。独立代码复核未发现流水线身份或并发问题。

## I-2：CommunityDragon 独立图片缓存

先用旧二进制测量 20 头像 + 9 技能 + 10 符文，39 张全成功，合计 **705907 bytes**。JSON/base64 约 942KB；512 条 / 16MiB 可容纳约 13 组这种典型集合，并由真实物理文件大小严格淘汰。初次探索手写技能路径出现 404，随后改用公开 `summoner-spells.json` 的实际路径；配额和正式基线均只采用 39 张全成功样本，未拿失败响应估算。

接入 `community_images.go`，与装备图片共用缓存机制但目录/配额完全独立。7天 fresh、30天额外 stale；无效内容和 HTTP 错误不落盘；进程内仍有合并和负缓存。仅 CommunityDragon 回退持久化，客户端 LCU 分支保持原有内存策略。

**修改前**将门槛写入 `r92/image-acceptance-before-change.json`：重启墙钟 <150ms 且 ≤冷启动25%，39张均成功。

| 二进制 | 冷启动 ms | 重启 ms | 重启/冷启动 |
|---|---:|---:|---:|
| 旧，无磁盘 | 1007.9 | 658.6 | 65.3% |
| 新，有独立磁盘 | 1169.1 | 18.4 | 1.58% |
| 新实现的 persist=false 变异 | 1577.1 | 835.4 | 53.0% |

新实现通过预设门槛；变异恢复原本的网络开销，门槛转红。所有轮次 39/39 HTTP 200，内容总字节数相同。证据为 `r92/images-*-results.json`。单测另确认新 app/provider 跨重启不上游、目录不占用装备配额。

## I-3：娱乐模式绝活哥诊断（未改 UI）

用实际 [CommunityDragon 队列目录](https://raw.communitydragon.org/latest/plugins/rcp-be-lol-game-data/global/default/v1/queues.json) 确认：

| queue | 上游实际名称 | 当前能力 |
|---|---|---|
| 1750 | Arena 3x6 | arena，3 人小队 |
| 2400 | ARAM: Mayhem | hextech-aram |
| 3140 | Multiplayer Practice Tool Custom | 多人训练自定义；不是无限火力或快速匹配 |

`ensureSpecialistRunes` 在 `recommendationQueueHasTopPlayers` 为 false 时，**尚未发请求**就记录 `no-top-players`。后端 `recommendationModeHasTopPlayers` 仅对解析为 ranked 的经典峡谷开放；这些模式并不开放。所以现有日志的根因是能力门禁，reason 名称容易被误读成上游返回空榜。

实际调用 `loadTopPlayersForPosition(ctx,"leesin","")` 三次、每次清空 reader 内存缓存，分别对应三队列的诊断标签，真正 URL 全是 [KR 李青专家榜](https://op.gg/zh-cn/lol/leaderboards/champions/leesin?region=kr)。HTTP 全部 200、各解析 5 人、source=html-table，耗时 1291/659/915ms。现有 reader **没有 queue/mode 参数**，不能把相同 KR 榜数据冒称三种娱乐模式的榜。

确认结论：不是已经证实的“OP.GG 对这些模式返回空榜”，也没有找到已验证的娱乐模式专用端点；当前直接改“该模式无绝活哥数据”会把未经证明的上游限制写进 UI。后续工单应先明确“仅经典峡谷支持”的能力提示和 skip 原因，若要支持娱乐模式，再单独验证真正模式对应的数据源。证据 `r92/public-diagnosis.log`。本轮不改模式参数和 UI。

## I-4：背景图诊断（未改逻辑）

1. 国服历史证据：`docs/maintenance-0912-cn-retirement.md` 引用 2026-09-12 实际诊断最后 109 条均 `background_skin_id=21069`、`profile_available=true`、`source=collection`、`skin_count=1948`。这证明国服目录/配置链路曾正常，不能说目录全局永久为空。该文档也明确没有完成 Windows 最终原画显示验收；当前 Mac 无 LCU，不能冒充本轮国服实机已验证。
2. 76000 合理：[真实豹女目录](https://raw.communitydragon.org/latest/plugins/rcp-be-lol-game-data/global/default/v1/champions/76.json) 为 champion 76、skin 76000、`isBase=true`。`applyMasteryBackgroundFallback` 在没有个人背景时取最高熟练度英雄 ×1000，所以这是预期默认皮肤兜底，不是解析异常。
3. `applyOverviewSkinMedia` 只从 `a.allSkins` 补居中 poster/video。无客户端且未加载 collection 时目录为空，于是这两个字段空；它不清除已存在的 `BackgroundSource/BackgroundPath`。`web/overview-art.js` 的 render 会继续使用原画兜底。真实走 `/api/champion-asset` 得到 76000 图片：HTTP200、53256 bytes、`image/webp`，见 `r92/build-background-retry.log`。原先把它当 JPEG 解码失败是测试假设错误，图片实际类型为 WebP，已按真实 MIME 验证。

根因：离线时没有客户端 collection 的居中/动画资源映射，且日志 `poster_available` 只反映该目录字段，不能代表最终渲染的普通原画是否可用。后续方向是独立的公共皮肤目录 fallback 与“最终选择来源/是否实际加载”诊断；客户端个人选择不能由 Riot API 推断，仍须保留最高熟练度默认图的区分。国服当前 Windows 的最终显示需现场复核。本轮没有以猜测修改背景或 UI。

## 测试、变异与交付

- `go test ./...`：通过（93.061s）；最初沙箱禁止 httptest 监听，获准在本机运行后通过。
- R92 新测试与相关 R89 出装 `-race`：通过（12.894s），包含 600 条与实际字节预算跨重启。
- `scripts/r92-mutation-check.py`：16 项基础变异 + 2 项实际字节/重启账本变异，全部 KILLED。Go 使用 overlay、JS 使用临时副本，没有覆盖活动工作区代码。
- 真实网络变异：战绩 persist-off、CommunityDragon persist-off 均失败于既定验收；真实 Chromium 提前出装变异也转红。
- 撤销前的 Chromium 交互结果保留为历史证据，已不再运行或维护原区块的正向测试。最终断开客户端的总览验证见撤销补充。
- 全量 JS、vet、Windows 编译结果见下方最终检查补充。

可重跑工具：`scripts/r92-riot-benchmark.py <新证据目录> <二进制>`；`scripts/r92-public-probe.py <二进制> <新目录> images|pro`；`DEEP_LEGENDS_R92_LIVE=1 go test -run '^TestR92Live' -v`（真实公开源随网络变化可能失败）；`python3 scripts/r92-mutation-check.py`；`node desktop/r92-browser.cjs`。Riot 脚本只在进程内读取本地 key，公共诊断不需要 Riot key。

尚待用户体验复核：关闭客户端后，搜一个韩服玩家、打开职业账号、检查总览战绩列表与页面布局。另须在 Windows 连接国服客户端确认生涯背景最终实际显示；这项无法由本机 Mac 的历史日志替代。

## 最终补充结果

- 完全未设置 `DEEP_LEGENDS_RIOT_MATCH_CONCURRENCY` 的默认 8 五次全网络采样：2485、3073、2362、2331、2584ms；中位数 **2485ms**，相对并发 4 的 2812ms 下降 **11.63%**。实际详情峰值全部8，20场全成功、0磁盘命中、0限流错误；累计排队中位数 3230ms。**25% 验收门槛仍为 FAIL**，未把第一轮的8.25%或第二轮11.63%写成达标。
- `scripts/r92-verify-evidence.py` 对真实观测运行硬断言：G1 返回 exit1；正常 G2/I2 返回 exit0；两条关闭持久化的真实变异返回 exit1。汇总 `r92/real-gates.json`。因代码使用隔离 overlay，活动实现始终保持原状；变异完成后的正常定向 race 测试再次通过。
- 全量前端/桌面测试：649 项，**648 pass / 1 skip / 0 fail**，279.688s；`go vet ./...` 通过；Windows amd64 主程序交叉编译通过。没有生成或发布安装包。
- 新二进制真实采样 SHA-256：`506d5e59557163b638114058dcfbcf421c30780ef37ca181ef29bb569efdee1e`。Windows 编译产物仅位于临时验证目录。
- 两路独立复核确认流水线和共享 OP.GG/CommunityDragon 链路未发现生产问题；按复核建议额外增加了真正触发128MiB字节上限、并跨重启恢复字节账本的测试及变异。

## 最新 H 组撤销：最终验收

依据用户截图后的明确决定及改写后的 H-1 六项清单，当前总览推荐已整体移除。不是 CSS 隐藏，也没有保留专用服务端路由、旧状态或后台请求；相邻当前对局占位仍在。README、DESIGN、当前验收脚本及旧的专用附件已同步清理；原始工单文件保留原文。此前“推荐已实现”的记录只说明撤销前状态，不再代表当前产品。

- 除工单自身外，全仓库指定旧标识搜索 **0 命中**，包括源码、脚本和文档。原始历史附件另存于本机临时验收目录，仓库保留与现行功能相关的证据。
- `go build ./...` 通过；`go test ./...` 通过，80.546s。
- `node --test --test-concurrency=2 web/*.test.cjs`：**418/418 通过**。相关桌面与 R89 总览测试另 **65/65 通过**。`go vet ./...` 与最终 Windows amd64 编译通过。
- 真实 Chromium 强制 disconnected 状态，从搜索、职业账号、英雄详情三个入口读取韩服玩家：最近20场正常，三首屏请求并行；当前对局占位直接紧邻战绩列表，没有额外推荐块、空占位或专用请求；空战绩和5场→20场重试也通过。
- 隔离副本恢复旧总览 renderer 的真实变异 **KILLED**，明确失败于 `unexpected block before recent matches`；恢复现行 renderer 后再跑三入口，**PASS**。
- 最终二进制对已撤销专用接口的本机请求返回 **404**。证据集中于 `r92/removal/`。

G/I 生产逻辑未因撤销改变，原实测结果继续有效：G-1 默认并发收益11.63%，25%目标未达成；G-2重启20场磁盘命中1070ms；I-1累计中位数12282→10834ms；I-2重启39图658.6→18.4ms。国服当前Windows最终背景显示仍需现场核验。此次未发布安装包。
