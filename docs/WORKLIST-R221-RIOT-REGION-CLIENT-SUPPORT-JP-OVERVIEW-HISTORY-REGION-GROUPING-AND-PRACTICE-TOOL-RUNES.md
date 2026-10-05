# WORKLIST-R221：外服（拳头）客户端全面适配——日服账号总览战绩读不出、页签归在国服、练习工具符文推荐为空

诊断：Claude（两张截图 + 日志 `lol-loot-diagnostics-1005-1645.jsonl` + 只读核对源码）。执行：GPT。日期：2026-10-05。
基线：0.12.71（指纹 `696da05d0ad0`，本次运行 `run_id 833ae46fccaa757f9094aef5`）。和 R211～R219 一起合并进下一个版本。

用户要求：**外服账号要做好所有支撑与适配**；外服玩家页签统一归到「韩服」分组（都是拳头服务器）。

执行状态（2026-10-05）：P1–P6 代码及自动验证已完成；Go 全量、race、vet、前端 938 项和 Worker 15 项全部通过。按用户此前“不构建、不发布”要求，版本保持 0.12.71，未构建或部署生产 Worker；生产路由及 Windows 真机验收待后续执行，工单保持进行中。详见 [外服适配执行账本](r221-execution-ledger.md)。

---

## 现象

1. **总览**（图一，日服账号 hate camille#PPPPP）：段位、熟练度、背景都正常，但战绩区一直是「战绩暂时无法读取 / 读取失败，界面已保留可核验数据」，左侧「近 0 场排位」「当前样本未发现单双排对局」「英雄胜率 0 场」全部为空。页签分组显示「国服 1」，页签被归进了国服。
2. **对局页**（图二，多人练习工具自定义，阿祈尔）：位置显示「其他」；符文页 OPGG 一直是「等待完整符文数据」，绝活哥同样没有数据。

---

## 日志证据（北京时间）

### 总览战绩

| 时间 | 事件 | 说明 |
|---|---|---|
| 16:35:54～16:36:18 | `lcu_discovery` credentials-unreadable → probe-failed → connected | 客户端刚启动，24 秒后才连上 |
| 16:36:39.8 | `ranked_data_source_decision selected=lcu`，`lcu_ranked_stats_shape` 铂金 I 17 胜 14 负 | 段位走本机客户端，正常 |
| 16:36:45.1 | `match_history_data_source_decision`：`sgp outcome=disabled "当前查询没有可用的 SGP 路由"`；`lcu outcome=failed` | **SGP 只认腾讯网关，外服直接禁用**；只剩本机客户端一条路 |
| 16:36:53.1 | `lcu_request GET /lol-match-history/v1/products/lol/current-summoner/matches` → **500**，耗时 5340ms | 客户端刚登录、战绩服务还没就绪 |
| 16:36:53.1 | `overview_load_cost duration_ms=13749`，`sgp_requests=0` | 总览这一次加载就此失败，之后没有自动重试 |
| 16:37:19.5 | 同一接口 **超时 8000ms**（用户手动重试） | 仍失败 |
| 16:38:49.5 | 同一接口 **200，137ms**（对局页读取） | 两分钟后本机接口已经好了，但总览已经停在失败状态 |

- `pro_identity_match region=""`：外服账号没有任何区服标识。日志里也没有任何地方记录客户端的大区和平台（JP / JP1），排查时只能靠猜。

### 对局符文

| 时间 | 事件 |
|---|---|
| 16:38:26 | `live_position_shape game_mode=PRACTICETOOL queue_id=3140`，所有位置为空 / `NONE` |
| 16:38:30.57 | `unknown_queue_observed queue_id=3140`；`recommendation_mode_resolved internal_mode=unsupported` |
| 16:38:30.57 | `live_recommendations_response status=failed stage=detail-load`；`specialist_runes_client_skip reason=no-top-players position=other` |
| 之后 | `live_recommendations_skip reason=backoff`，界面一直显示「等待完整符文数据」 |

符文为空**和外服无关**，原因是练习工具（`PRACTICETOOL` + 召唤师峡谷 map 11）没有映射到任何推荐模式。国服的练习工具同样会碰到。

---

## 根因（源码已核对）

1. **SGP 只支持腾讯**：`sgp_gateway.go` 的 `sgpOfficialHost` 只放行 `*.lol.qq.com`；`clientTencentServerID` 遇到 region 不是 `TENCENT` 就返回空。所以外服客户端的 `SGPAvailable=false`，`resolveMatchHistoryDataSources` 只剩 LCU 一条路。
2. **本机战绩没有重试**：客户端刚登录时，`/lol-match-history` 返回 500 或超时，总览直接显示失败，不会在客户端就绪后自动重试。
3. **Riot API 全链路写死韩服**：
   - 后端 `riot_api.go`：`riotClusterHost = "asia.api.riotgames.com"`、`riotPlatformHost = "kr.api.riotgames.com"`、`riotRegionKR = "kr"`。
   - 后端 `riot_relay.go`：`riotRelayEndpoint` 只认 asia / kr，其余一律 `errRiotRelayUnavailable`。
   - 中转 `relay/riot-worker/worker.mjs`：`HOSTS = { asia, kr }`。
   - 前端 `gameplay.js`：`riotTab(tab)` 只认 `region === "kr"`；搜索请求、对局详情、符文数据、绝活哥等多处直接写 `region: "kr"`（约 1130、3591、4185、7529 行等）。

   所以即使想用 Riot API 读日服战绩，目前也无路可走。
4. **页签分组只看韩服**：当前客户端账号的页签固定进「国服」分组（`PLAYER_GROUPS.players`），`tabServerLabel` 在非 kr 时兜底显示「国服」。
5. **练习工具没有推荐模式**：`gameplay.go` 约 4600～4616 行的模式映射只有 `CLASSIC + map 11 → ranked`，`PRACTICETOOL` 落到 `unsupported`。位置为空时也没有用英雄的主位置兜底。

---

## 修复要求

### P1　识别外服客户端的大区与平台

1. 连接成功后从 `platformInfo()`（`--region` / `--rso_platform_id`）取大区和平台，例如 `JP` / `JP1`。
2. `/api/status` 增加 `clientRegion`（`TENCENT` 或 Riot 平台码的小写形式，如 `jp1`、`kr`、`na1`、`euw1`）和 `clientRegionLabel`（日服、韩服、美服、欧西……）。
3. 新增诊断 `client_platform_resolved`：每次连接记录一条，只含 region、platform 两个枚举值和来源（启动参数 / 命令行查询）。

### P2　Riot API 支持全部拳头区服（后端 + 中转）

1. **平台主机**：`br1 eun1 euw1 jp1 kr la1 la2 me1 na1 oc1 ru sg2 tr1 tw2 vn2`。
2. **集群路由**（Match-V5、Account-V1）：
   - americas：NA1、BR1、LA1、LA2
   - asia：KR、JP1
   - europe：EUN1、EUW1、ME1、TR1、RU
   - sea：OC1、SG2、TW2、VN2

   执行前请对照 Riot 官方文档核对一遍，不要只按这份清单。
3. `riot_api.go` 去掉写死的 kr / asia 常量，所有请求带上平台参数，由平台推出集群；缓存键也要带上平台，避免不同区服串数据。
4. `riot_relay.go` 的 `riotRelayEndpoint` 支持上面全部平台和集群。
5. `relay/riot-worker/worker.mjs`：`HOSTS` 扩到全部平台和 4 个集群；平台、集群分别沿用现有的接口规则（集群：账号与对局；平台：召唤师、段位、熟练度、状态）。补齐 `worker.test.mjs` 的路由测试，然后按 R206 的方式重新部署中转并在账本里记录。
6. 额度：Personal Key 的限流按区服分别计算；沿用 R208 的冷却与熔断逻辑，但冷却状态要按区服分开记，日服限流不能把韩服查询一起锁住。

### P3　外服账号的总览战绩

1. 外服（非腾讯）客户端的本人总览，战绩来源顺序为：**Riot API（按本账号平台）→ 本机 LCU**。Riot API 有完整的 Match-V5 详情，和韩服页签使用同一套展示逻辑。
2. 本机 LCU 作为兜底时，500、超时、连接被拒这类「客户端还没就绪」的错误要自动重试：间隔 3 秒、6 秒、12 秒，最多 4 次，并且在 `identityReady` 变为 true 后重新发起一次。不能停在失败状态等用户手动点重试。
3. `match_history_data_source_decision` 增加 `client_region` 字段；Riot API 失败时把具体原因（未配置 Key、额度、超时、上游 404）写进 attempts。
4. 段位、熟练度、赛季快照继续使用本机 LCU（日志里已经正常），不需要改。

### P4　外服页签统一归到「韩服」分组

1. 当前客户端是外服时，本人页签归到「韩服」分组（`PLAYER_GROUPS.kr`），不再进入「国服」分组。
2. 页签和总览卡片显示真实区服，例如「日服」，页签 title 为「日服 (JP1)」。分组名称保持「韩服」不变。
3. `riotTab(tab)` 改成「region 是任意拳头平台」，不再只认 `kr`；`tabReady` 对拳头平台页签不依赖本机连接，和现在的韩服页签一致。
4. 前端所有写死的 `region: "kr"` 改为使用页签或对局的实际区服：搜索、点开对局里的玩家、对局详情、符文属性统计（perk-stats）、绝活哥跳转等。绝活哥、OP.GG 榜单这类本来就固定取韩服数据的功能保持韩服，但必须是显式写明的选择，不能是沿用默认值的结果。
5. 顶栏搜索的区服菜单增加其他拳头区服（至少日服、美服、欧西、欧北东、台服、东南亚），默认选中当前客户端所在区服。
6. 外服客户端下，国服专属功能（ARAMKit、TCLS 国服纯净入口、腾讯 SGP 相关诊断入口等）要静默隐藏，不能出现报错或「国服」字样。

### P5　外服对局页

1. 外服对局中其他玩家的战绩：SGP 不可用时，按 P3 的顺序走 Riot API（按对局所在平台）→ LCU；段位走 league-v4（平台主机）或 LCU。
2. 对局里点开玩家时，新页签使用对局所在平台作为区服，并归入「韩服」分组。

### P6　练习工具的符文和出装推荐（国服、外服通用）

1. `PRACTICETOOL` + map 11（queue 3140，以及国服练习工具对应的队列）映射到召唤师峡谷排位推荐（`ranked`）。
2. 位置为空或为 `NONE` 时，用该英雄在 OP.GG 的主位置作为推荐位置；对局卡片上显示这个推断出的位置，不再显示「其他」。不加任何说明文字（CLAUDE.md 界面文案红线）。
3. 绝活哥同样按推断出的位置查询。
4. 「仅英雄选择阶段可应用」的按钮逻辑不变。

---

## 测试

- **Go**：
  - 每个平台到集群的路由；缓存键带平台。
  - 外服客户端的战绩来源顺序为 Riot API → LCU；LCU 返回 500 后的重试节奏。
  - 练习工具映射到 `ranked`，位置为空时使用主位置。
  - `/api/status` 的 `clientRegion` 取值（腾讯、jp1、kr、未知）。
  - `client_platform_resolved` 诊断的字段白名单。
- **中转**：`worker.test.mjs` 覆盖每个平台和集群的放行规则，以及非法 host / 路径的拒绝。
- **前端**：
  - `clientRegion=jp1` 时，本人页签进入「韩服」分组并显示「日服」。
  - 从日服页签、日服对局点开玩家，请求里带的是 `jp1`。
  - 搜索区服菜单默认值为当前客户端区服。
  - 全仓库检查：除明确写注释的「绝活哥 / OP.GG 固定韩服」之外，不再有写死的 `region: "kr"`。
- 全量 `node --test backend/web/*.test.cjs`、`go test ./backend`、中转测试的结果写进账本。

---

## 真机验证（构建后由用户操作）

1. 登录日服客户端后打开软件：本人页签在「韩服」分组下，显示「日服」；总览战绩、近期排位、英雄胜率、位置偏好都有数据。日志里有 `client_platform_resolved region=JP platform=JP1`，以及选中 Riot API 的 `match_history_data_source_decision`。
2. 刚登录客户端就打开总览：即使 Riot API 不可用，LCU 兜底也会在客户端就绪后自动出数据，不需要手动重试。
3. 日服练习工具选择阿祈尔：OPGG 符文出现中路推荐，位置显示中路。
4. 日服匹配或排位对局：队友和对手的战绩、段位能正常显示；点开玩家进入日服页签。
5. 切回国服账号：总览、对局、搜索的行为和现在一致（回归验证）。
