# R89 韩服玩家查询、装备图标执行账本

> 2026-09-14 用户最终决定：D 组总览装备推荐已在 R92 中整体撤销；下文 D 组和关联旧测试结果仅记录当时状态，当前交付以 R92 撤销记录为准。

工单：`WORKLIST-R89-KR-PLAYER-LATENCY-ITEM-ICONS.md`。2026-09-14，在无 LCU 的 macOS arm64 环境执行。范围为该工单的 A–E；F 组按要求只记账。本工作区同时有 R87/R88 与另一份 R89 后续工单的在途改动，本轮未将整个工作区差异当作本工单成果，也未提交、发布或替换安装包。

## 已实现与逐项验收

| 条目 | 实现与证据 | 状态 |
|---|---|---|
| E-4 / 0.4 优先诊断 | 三个目录记录 `catalog_load` 成败/来源；每场 `riot_match_items` 记录每位 participant 的长度与非零数；客户端记录目录失败 HTTP 状态、错误类型及缺失 ID（去重、最多20个）。先加入诊断，再实现缓存。见 `diagnostic-samples.jsonl`、`catalog-failure.jsonl`。 | 完成 |
| A-2-1 三个远端图标分支 | 统一 `loadChampionRemoteAsset → loadAsset`，已有 LCU 分支保留；404 等非取消错误负缓存60秒；等待者不会继承已取消预热的失败。 | 完成 |
| A-2-2 磁盘 | 图片7天新鲜、另30天stale；独立 `champion-images`，2048项/64MiB，写入时同步淘汰。JSON路径保留原策略。 | 完成 |
| A-2-3 预热 | 首次玩家页后空闲3秒启动、并发6、总任务60秒上限，前台操作取消并重新延后；预热真实 `/cdn/{patch}/img/item/{id}.png`。以OP.GG排位出场最多20名英雄的出门/鞋/核心装备出现次数排序，最多120张；这是有界样本热度，不能称为全服精确前120榜。 | 完成，近似热度口径 |
| A-2-4 前端恢复 | 目录失败30秒后重试；目录到达使已保留的战绩卡DOM失效，补出图标。真实Chrome 504→成功场景通过。 | 完成 |
| A-3-1 同图3次 | 修改前 171/174.5/167.4ms；修改后 203.4/2.82/1.75ms，后两次<5ms。聚合日志有memory/disk状态。 | 通过 |
| A-3-2 100图、6并发 | 原代码第二轮2909.5ms；修改后第二轮21.9ms，<500ms。 | 通过 |
| A-3-3 重启 | 重启后100图首轮27.0ms，<1500ms；第二轮17.9ms。 | 通过 |
| A-3-4 单飞 | 同图20并发，计数运输层只访问1次；同时移除两层flight后测试失败。 | 通过 |
| A-3-5 容量 | 连续2000张、每张48KiB数据（含JSON/base64磁盘包络）后目录≤64MiB、≤2048项；删除淘汰逻辑后失败。 | 通过 |
| B-2 / B-4-2 接口兼容 | current-game、season-summary支持完整KR Riot ID；显式playerRef优先并保留隐私/过期检查；纯raw请求200，缺失身份400。 | 通过 |
| B-4-1 并行 | 真Chrome共用三个真实入口事件，搜索/职业/英雄三项请求发起最大差均<300ms（精确值见chromium.json），均早于总览完成。overview先5后20。 | 通过 |
| B-4-3 去重 | 每入口current-game、season-summary各一次，后端同名身份/后续ref复用缓存；当前对局水合更新名称时保留稳定ref缓存别名。30秒当前对局轮询、赛季attempt去重保留。 | 通过 |
| C-1-1 / C-3-1 | 默认8，personal最多8；显式production配置最高20，本地15/s与90/2min不变。最后一轮真实峰值8，总览4637→2508ms，下降45.9%。 | 通过，生产Key尚由用户申请 |
| C-1-2 / C-3-2 | `riot-matches`最多600场/128MiB；对局匹配ID和参与者必要字段校验后落盘。重启1053ms、20场disk、详情2ms。为达到端到端门槛，另加公开身份磁盘15分钟/召唤师5分钟，256项/4MiB，过期不回退；README说明保留内容与清理方法。 | 通过 |
| C-1-3 首屏 | 首次5场先显示，第二次请求完整设置场数并重算汇总；不可变详情复用。补齐失败保留前5场、显示“重试补齐”，真实Chrome点击后恢复20场及出装；关闭覆盖层取消出装请求，旧层迟到响应不能覆盖当前玩家。 | 完成 |
| C-3-3 限速 | 构造200次连续预约，检查所有滑动1秒与2分钟窗口；分别提高15与90的变异各自失败。 | 通过 |
| D 最小范围 | 按工单最小建议实施：最近最多20场中最常用1个英雄及该英雄主分路；仅一套出门/鞋/核心/四五六件，链接英雄详情；空战绩/无有效分路不显示卡片。 | 完成 |
| D-3-1/2/3 | 无客户端真实接口李青6组均各1条、source=OP.GG；运输层白名单仅OP.GG（元数据已有）且改为LCU host后失败；空战绩隐藏并清除已有卡片的变异失败。 | 通过 |
| E-1 阶段 | account/summoner/matchIDs/ranks/mastery/details等真实span；serialize不再包揽全程。英雄目录与详情并行；OP.GG历史赛段改为缓存读取+既有后台补全，响应中该阶段通常0ms，真实后台耗时另记 `opgg_historical_cost`。 | 完成，有落盘样本 |
| E-2 图标 | `asset_fetch`按10秒窗口聚合host、cache计数、命中率、p50/p90、失败数；最多512延迟样本，导出时flush。 | 完成，有落盘样本 |
| E-3 排队 | `riot_overview_cost.limiter_queue_ms`为所有调用累计等待（可能大于墙钟，不能与墙钟相加）；最后冷查2882ms、重启0。实际未出现429。 | 完成，有落盘样本 |

## 不把性能波动藏进结论

所有真实请求均使用独立测试数据目录和实际编译的二进制；图标测试禁用后台预热，以免混入额外流量。Riot测试在子进程环境中使用本地已有Key，不在命令行、证据或构建物中注入Key。Chrome的时序/恢复测试使用真实前端、夹具HTTP响应与1px测试图，诊断另发送至真实后端落盘；截图是夹具图，不能当成玩家真机截图。

保留中间未达标数据：最初并发8版14882→3909ms，但重启4003ms；移出历史赛段后8807→3067ms，重启1914ms；身份缓存版5119→5074ms（冷目录3233ms，未达到25%），重启742ms。最后将中文目录与详情同时执行后4637→2508ms，重启1053ms。一次顺序对照不能保证任何地区网络都维持45.9%收益，最终仍需用户真机复测。文件分别为 `riot-first-attempt.json`、`riot-history-background.json`、`riot-identity-cache.json`、`riot-final.json`。

工单列出的三种空白候选并未在用户真机上定案。测试独立复现了第四种具体问题：目录已成功到达，但保留的战绩卡DOM未重新生成。已修正，并用真实Chrome删除该修复的变异确认会重新失败。此结果不能替代用户新日志对候选①②③的判断。

## 变异与复现

- `scripts/r89-mutation-check.py` 使用Go overlay和临时JS副本，不改当前工作区；每次先跑R89专项绿色基线。35项独立变异见 `mutations.json`（32项主批次+1项补齐重试+2项覆盖层关闭/迟到保护）。每项非零退出且排除编译失败/语法失败。
- `scripts/r89-live-mutations.py` 在真实二进制/Chrome上另杀4项：取消图标缓存后同图第二次184ms、100图第二轮4148ms；关闭磁盘持久化后重启100图7513ms；改回串行启动、移除目录到达时重绘均导致Chrome断言失败。见 `live-mutations.json`。
- 图片有两层缓存，因此只禁用provider图片policy仍可能由外层memory缓存满足同进程耗时；专项额外直接验证provider缓存，policy变异会红。真实同进程“无缓存”性能变异需要同时关闭两层，而持久化变异只改persistDisk。这一差异不应隐瞒。
- 主测试：`go test ./...`；`go test -race -run 'TestR89|TestOverviewCurrentGame|TestLoadRiotOverview|TestRiotOverview|TestR86' -count=1`；`go vet ./...`；`node --test web/*.test.cjs desktop/*.test.cjs`；`node desktop/r89-browser.cjs`。
- 真实测量：`scripts/r89-asset-probe.py`、`scripts/r89-riot-probe.py`、`scripts/r89-catalog-probe.py`。所有归档证据在 `docs/r89-kr-player/`；不归档session-token、公开对局正文或身份正文。
- 一次全Go测试在高并行测试负载下触发既有 `TestHydrateMayhemAugmentCopyUsesBoundedParallelismAndCache` 的700ms时限（实际705ms），之后单独完整重跑；结果记于 `validation.json`，不把该次失败删掉。

## F组：按工单要求本轮不修

| 条目 | 保留问题与后续核验 |
|---|---|
| F-1 娱乐队列绝活哥符文 | 1750/2400/3140缺高手榜；需单独确认OP.GG是否提供榜单，确认没有后才设计“该模式无绝活哥数据”提示。 |
| F-2 职业ladder慢 | 8456/12999ms，位于supplements之后；单独研究并行/取消/缓存，不混入本轮玩家查询。 |
| F-3 CommunityDragon不稳定 | `/api/champion-asset`的新磁盘缓存已覆盖；独立的`/api/image` CommunityDragon回退仍只存内存，留待后续持久化。 |
| F-4 生涯背景 | catalog_count=0、poster_available=false、skin_id=76000原因仍未确认；本轮不改。 |

## 外部收尾

1. 生产Key申请/签发仍需用户完成。本轮没有将personal并发提高到15–20，也未抬高本地限速。
2. 本轮未新增或修改Windows专属源码，另做Windows amd64交叉编译；这不代替Windows运行验证，也不等同安装包发布。
3. 使用包含本轮修改的构建后，请关闭英雄联盟客户端，搜索一次韩服玩家，停留至少60秒再导出诊断，同时截取60秒后的战绩卡。核对目录结果、参与者装备数、缺失ID、图标命中率、三请求时序和总览阶段。若仍空白，依据这些事件判定原因，避免重复猜测。
