# R82 执行结果与尚未完成的实测

日期：2026-09-12。对应工单：`WORKLIST-R82-CURRENT-GAME-PROBE-MATRIX.md`。

**代码与本地验证已经执行；国服缺失格子的真实 HTTP 结果尚未取得。不能称为已修复任意国服玩家阵容查询。**

本机运行中的程序显示“未检测到客户端 / 英雄联盟客户端未登录”，没有可用于本次实测的已登录 LCU。没有使用导出日志中的旧令牌，没有启动对局或观战，也没有用模拟响应填充实测表。

## P0-1：已有两次 405 的具体字段

以下是本轮前半段读取 2326 日志时保留的白名单投影；后续复查时原 Downloads 路径已不存在，故不能重新读取完整文件。这些不是新请求的结果。

| 字段 | 16695 行 | 16977 行 |
| --- | --- | --- |
| `http_status` | `405` | `405` |
| `allow_header_present` | `false` | `false` |
| `allowed_methods` | `{}` | `{}` |
| `content_type` | `application/json` | `application/json` |
| `auth_challenge_present` | `false` | `false` |
| `auth_challenge_classes` | `[]` | `[]` |
| `location_present` | `false` | `false` |
| `body_bytes` | `61` | `61` |
| `upstream_host` | `hn10-k8s-sgp.lol.qq.com` | `hn10-k8s-sgp.lol.qq.com` |
| `upstream_port` | `21019` | `21019` |

判读：观察到的是 **spectator 路径 + entitlements + GET + 他人**这一格被以 405 拒绝；按 HTTP 结果归入方法拒绝，尚不能在“路径未匹配”和“此资源不支持 GET”之间定根因。没有 `Allow`，也不是已记录的 HTML 兜底页；因此执行 OPTIONS 头部探测有意义。没有依据把生产查询改为 POST。

另需更正工单 0.3 的凭据等同性：前序 2326 分析中 CURRENT_GAME / SUMMONER 使用 `credential-14`，RANKED 的 11 次 200 使用 `credential-5`。这些是进程内别名，不是令牌。不能把它们全部称为同一份令牌的对照；其他服务可查他人也不能证明 GSM 具有同一授权口径。

## 逐项执行与验收状态

| 工单项 | 本次交付 | 真实客户端验收 |
| --- | --- | --- |
| P0-1 | 两行头部字段及有限结论如上 | 已读取既有证据；没有新请求 |
| P0-2 | 同一固定 session 令牌，连续本人 / 用户明确确认不在局的同区真人 / 随机合法 UUID 三格；不要求本人开局 | **未实测**：缺已登录 LCU 和两名当前已解析的玩家引用 |
| P0-3 | B-self、B-target、C-self、C-target、D-self；每格最多一次 GET | **全部未实测** |
| P0-4 | A-target 和 C-target 的小写区服对照 | **两格未实测** |
| P0-5 | spectator OPTIONS，只读响应头；若本轮已取得 Allow 则记录跳过原因 | **未实测** |
| P0-6 | 按 v3、v2、help 顺序读取，输出路径 / 方法 / 参数 / accepts_puuid 候选；每份文档 16 MiB、候选 1500、候选输出预算 1 MiB（防止轮转挤掉矩阵）、参数 80、引用/递归深度 8；未识别或截断明示 | **尚无真实客户端候选表**；模拟文档测试不是 R83 的实测输入 |
| P0-7 | 当前客户端 ServiceEndpoint / platformId 优先；内置回退；记录两者是否可比较和是否一致；按账号和连接缓存 | 逻辑通过测试；**未取得真实客户端一致性事件** |
| 第 3 节 | 已收到的有效在局 GSM 200 / 本机 gameflow 保留真实席位；空队与未知身份标记不完整；不凑十人 | 本地校验、生产链路及变异测试通过；未部署到 Windows 实测 |
| 第 4 节 | 没有证据证明 P0 全负，因此不提前宣布只能降级 | 保留现有好友 presence 和非好友失败时不渲染模块的行为；未新增 presence 观战按钮 |
| E/gameId | 没有新增终局统计作为实时阵容回退 | 按附录排除在本轮请求之外 |

## 实测矩阵（不拿 mock 冒充）

| 格子 | 本人 | 他人 |
| --- | --- | --- |
| A region/session GET | 既有 2326 证据：200；之后离局为 404 | 既有证据：401 |
| B region/entitlements GET | 未实测 | 未实测 |
| C spectator/session GET | 未实测 | 未实测 |
| D spectator/entitlements GET | 未实测 | 既有证据：405，无 Allow |
| F 小写区服 region/session | 本轮不重复该格 | 未实测 |
| F 小写区服 spectator/session | 本轮不重复该格 | 未实测 |
| OPTIONS spectator/session | 本轮不请求本人 | 未实测 |

P0-2 **同一次新运行**的三目标真值表仍为空：本人未实测、明确不在局真人未实测、随机 UUID 未实测。历史本人 200/404 不能填入这组固定凭据的新对照。随机 UUID 只能标注“假定不存在”，不能假装从服务端数据库确认不存在。两名他人都 401 也不能仅凭状态码证明 JWT `sub` 硬绑定。

## 使用新增入口

1. 使用包含本次代码的新构建，登录国服客户端。**不用开游戏。**
2. 总览加载两名同区玩家：待查目标，以及明确知道当前不在对局中的真实玩家（不限好友）。
3. 设置 → 隐私与安全 → 诊断与日志下方，展开“国服当前对局：只读对照探测（R82）”。
4. 选择两名玩家，勾选离线确认，执行一次。最长约 35 秒。客户端好友状态若明确表明对照玩家在局，会拒绝该对照。
5. 等结果结束再导出诊断日志。界面逐格显示状态码；详细候选清单在 JSONL 的 `current_game_endpoint_candidate` 中。

同一客户端、账号、实际令牌、网关、路径、方法、目标的已测格子复用结果，不再重发。复用标注原始时间与 trace，不把旧数据称为本次连续对照。会话最多保留 128 格；切换账号或客户端隔离缓存。自动刷新与导出日志不会触发矩阵。

普通卡片“重试阵容查询”现在仅重读产品数据，不再附带旧 A-self/A-target 双对象对照及 D-target spectator 请求；这些已测诊断格不会随日常刷新持续放大请求。

新增事件：`current_game_matrix_start/token/offline_control/cell/summary`、`sgp_gateway_resolution`、`current_game_endpoint_inventory/candidate`。每个 matrix 事件带同一个 `trace_id`；状态为 0 表示没有 HTTP 响应，不是 404。日志仅保留固定错误分类、结构计数和 JWT 字面相等布尔值，不记录原始 token、PUUID、错误原文或 schema 示例/默认值。

## 阵容修复的范围

- 必须有有效游戏 ID、支持的地图、进行中阶段和恰好一次的目标身份。
- 继续拒绝重复有效身份、选人记录冲突、错误目标及客户端/账号变化。
- 未知、隐藏身份不生成可点击引用，也不触发资料、段位或战绩补查；保留实际提供的英雄和技能。
- 返回的 5/0 不再推算成缺失五人，更不会补出十人；空队明确写“该队阵容未提供”。
- 这修复的是“已收到有效数据却整份丢弃”，**与他人请求 401 无关**。

## 验证记录

- 全量 `go test ./... -count=1` 通过（最终清理旧附带探测后为 85.244 秒；首次沙箱运行因本机端口权限中断，授权后重跑）。
- 相关 `go test -race` 通过（4.888 秒）。
- `scripts/check-r82-roster-mutation.cjs` 实际运行通过：Go overlay 临时撤销分档逻辑后，`TestR82PartialRoster` 和 `TestR82ProductionSGPPartialRoster` 均变红；源文件未改动。
- 新入口前端测试与 gameplay 回归共 41/41 通过，覆盖显式确认、禁止自动发送、阻止重复点击和中止在途请求。
- `go vet ./...`、`git diff --check`、两份改动 JavaScript 语法检查通过。
- 独立核验发现并修复了全 404 结论过度推断、schema 引用解析失效未标截断、`+json` 类型遗漏和探测引用范围校验问题。

**剩余条件只有真实运行环境与对应的玩家选择，不能以继续增加同类 401 日志替代。真实矩阵未填完前，不触发“P0 全负”的产品降级，也不宣称任意国服玩家的完整阵容已可用。**
