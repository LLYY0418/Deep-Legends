# WORKLIST-R102：去主号改按最近活跃排序 + 33 名职业选手账号全量内置

诊断人：Claude（只读诊断，未改仓库代码文件；本轮新增 `docs/pro-accounts-verification-2026-09-17.md`）。
执行人：GPT。
日期：2026-09-17。
输入证据：用户对照 OP.GG 小程序核对完成的账号清单（已落盘 `docs/pro-accounts-verification-2026-09-17.md`），R101 独立验收（四路子代理，P1-P10 全部符合，详见 `docs/r101-execution-ledger.md`；验收发现两处工单措辞问题——`connection_manager.go` 文件名引用有误、`TestR99SeedNeverLeaksPUUID`测试名有误，均不是代码缺陷，本轮不必处理）。

---

## 口径决定（不要自行更改）

1. **彻底去掉"主号"概念。** 用户原话："应该去除主号这个说法，因为选手有时候在某段时间喜欢打小号，然后小号没有大号段位高……哪个号更活跃就排在表格第一个，以最新对局开始的时间为标准。" 后端 `proAccount.Primary` 字段、前端"主号"徽章、`#pro-primary-only`"只看主账号"开关三处全部改造，见 P4/P5。
2. **排序的唯一权威信号是 Riot 官方 match-v5 接口查到的"最近一局对局开始时间"（`gameStartTimestamp`）**，不使用 OP.GG 的 `UpdatedAt`（代码里已有注释确认它是"源记录更新时间"不是"最后一局游戏时间"）、不使用 `docs/pro-accounts-verification-2026-09-17.md` 里任何静态日期/LP 快照——那些是核对账号归属用的一次性证据，不是产品排序要读的数据源。
3. **不限队列类型。** 查"最近一局"时不加 `queue`/`type` 过滤，任意模式的最近一局都算，理由：目标是判断"这个号是不是他现在真的在用"，限定排位反而会漏掉正在用某个号打匹配/大乱斗热身的情况。
4. **33 名选手的全部账号（含小号）本轮全部内置为 seed**，数据来源是 `docs/pro-accounts-verification-2026-09-17.md`，**逐字照抄 GameName/TagLine，不要自己再去查一遍 dpm.lol/Leaguepedia**。★ 该文件里 `dyjkbysb#KR1` 归属 **Wei**，不是 Rookie——这是用户 2026-09-17 当面核对确认的，权威性高于此前 R99 已内置的种子数据，**如果发现 Rookie 现有种子数据里还挂着这个账号，要一并移除**。
5. **Dormant（历史/长期不活跃）判定逻辑不动**，继续复用现有实现；但它不再是排序的第一优先键（详见 P4），只在"查不到最近对局时间"的账号之间做兜底排序。
6. **无法查到最近对局时间的账号（Riot 调用失败/配额耗尽/账号从未有对局记录）不允许排到有数据的账号前面**，一律沉到"已知最近对局时间"的账号之后，内部再按现有段位规则排序兜底——不许因为查不到时间就把账号从列表里丢弃或显示错误时间。
7. 沿用项目一贯口径：诊断只记结构/数量/状态码，不记玩家标识；PUUID 继续走现有磁盘缓存（30 天种子锚点 TTL 不变）。
8. **确认长期未定级的账号，段位查询要退避，不要一直按固定节奏刷新**（用户反馈，见 P9）。**这条只管段位查询频率，不影响 P2 的最近对局时间查询**——未定级账号照样可能是玩家正在用的活跃小号，活跃度信号不能停。

---

## P1 — `proSeedAccount` 改造为一个选手多个账号

**现状**（`pro_seed_accounts.go:13-24`）：

```go
type proSeedAccount struct {
	TeamCode string
	Player   string
	OPGGID   int
	GameName string
	TagLine  string
	Reviewed string
}

var proSeedAccounts = []proSeedAccount{
	{TeamCode: "IG", Player: "Rookie", OPGGID: 371, GameName: "모든일은같이", TagLine: "KR1", Reviewed: "2026-09-16"},
}
```

`GameName`/`TagLine` 是标量字段，只支持一个选手一个账号。`loadProSeeds`（`:56-95`）里还有两处选手专属硬编码：`RealName: "Song Eui-jin"`（Rookie 的真实姓名，写死在函数体内）、`Position: "middle"`（写死中单）。33 人各队位置不同，这两处必须从数据里来，不能继续硬编码单一值。

**要做什么：**

1. `proSeedAccount` 改成：

```go
type proSeedAccount struct {
	TeamCode string
	Player   string
	RealName string
	Position string
	OPGGID   int
	Accounts []proSeedAccountRef
	Reviewed string
}

type proSeedAccountRef struct {
	GameName string
	TagLine  string
}
```

	`RealName`/`Position` 从 `pro_roster.go` 现有的选手名单数据取（该文件已经有这两项字段用于别的用途，直接复用，不要另起一套）。**不要凭记忆填 RealName/Position，必须从 `pro_roster.go` 现读**——这是 R101 调研阶段踩过的坑（子代理凭记忆写错名单导致查错 5 人漏查 6 人），这次是往源码里写，错了影响更大。

2. `proSeedAccounts` 变量按 `docs/pro-accounts-verification-2026-09-17.md` 的表格逐队逐人展开，33 名选手全部有一条记录，`Accounts` 里按表格顺序列出该选手名下**全部**账号（含小号，注意"主号?"这一列本轮不使用，不要按它筛选，表格里列出的账号原样全收）。`OPGGID` 沿用各队现有 ID（Rookie 是 371，其余 5 队从现有 `previous []opggProTeam` 或 `pro_roster.go` 里能查到，**不要新编号**）。

3. `resolveProSeed`（`:26-52`）改成对 `seed.Accounts` 里每一个账号分别解析。PUUID 锚点缓存键要能区分同一选手的多个账号，现有键 `"proseed:v1:"+seed.TeamCode+"/"+strings.ToLower(seed.Player)` 只能定位到选手，改成按账号定位，例如 `"proseed:v1:"+TeamCode+"/"+Player+"/"+strconv.Itoa(索引)`（索引就用账号在 `Accounts` 切片里的下标，稳定不变，因为一旦录入顺序不会因改名而变化）。30 天 TTL 不变。

**验收判据：**
- `TestR102SeedAccountSupportsMultipleAccountsPerPlayer`：给一个 fixture 选手配 3 个 `proSeedAccountRef`，断言 `resolveProSeed` 或其上层调用为每个账号返回独立的 `riotAccount`（PUUID 不同）。
- `TestR102SeedRealNameAndPositionComeFromRoster`：断言某个非 Rookie 选手（如 Wei）加载出的种子记录里 `RealName`/`Position` 与 `pro_roster.go` 里的对应字段一致，而不是硬编码值。
- `TestR102SeedAnchorKeysAreUniquePerAccount`：同一选手 2 个账号，断言两次 `cachedPublicIdentity` 调用用的 key 不同（不能互相覆盖对方的 PUUID 缓存）。

**变异判据（必须 FAIL）：**
1. 把 `RealName`/`Position` 改回硬编码字符串 → 第二条 FAIL。
2. 把锚点 key 里的账号索引去掉，退化回只按选手定位 → 第三条 FAIL（两个账号的 PUUID 会互相覆盖）。

---

## P2 — 新增 Riot match-v5"最近一局对局开始时间"查询

**现状：** `matchIDsFiltered`（`riot_api.go:780-793`）已封装 `GET /lol/match/v5/matches/by-puuid/{puuid}/ids`，支持 `start`/`count`/`queue`/`type`；`matchByIDWithCache`（`riot_api.go:802` 附近）已封装单场详情查询并带永久缓存。**但 `riotMatchInfo` 结构体（`riot_api.go:650-661`）目前只解析了 `GameCreation`/`GameDuration`/`GameEndTimestamp`，没有 `GameStartTimestamp` 字段**——按 Riot 官方文档，`gameStartTimestamp` 才是"玩家实际进入游戏"的时间点，`gameCreation` 在有重开局等情况下会跟它不一致，本次必须用前者。

**要做什么：**

1. `riotMatchInfo` 加一个字段：`GameStartTimestamp int64 \`json:"gameStartTimestamp"\``（毫秒 Unix epoch，UTC）。
2. 新增方法 `lastMatchStart(ctx context.Context, puuid string) (time.Time, bool, error)`：
   - 用 `matchIDsFiltered(ctx, puuid, 0, 1, 0, "")`（`count=1`，不传 `queue`/`type`，按口径 3 取任意模式最近一局）。
   - 空数组（从未有对局记录）→ 返回 `(time.Time{}, false, nil)`，不是错误。
   - 非空 → 取 `ids[0]`，调 `matchByIDWithCache` 拿详情，读 `info.GameStartTimestamp`，转成 `time.UnixMilli(...)`，返回 `(t, true, nil)`。
   - 网络/限流错误照常透传，不要吞掉。
3. **新增独立缓存层**，key 前缀 `proseed-lastmatch:v1:`+puuid，**TTL 6 小时**（与 P9 的段位缓存同频，不要复用段位缓存的 key 和 TTL，两者语义不同）。缓存未命中时才发起上面两次真实调用；命中直接返回缓存值。

**验收判据：**
- `TestR102LastMatchStartParsesGameStartTimestamp`：假 match-v5 详情返回 `gameStartTimestamp: 1757980800000`，断言解析出的 `time.Time` 对应 UTC 时刻正确（不要用 `gameCreation` 的值去断言，fixture 里两个字段故意给不同值，用来防止代码接错字段）。
- `TestR102LastMatchStartHandlesNoMatches`：`ids` 端点返回空数组，断言 `ok==false`、`err==nil`，且**不发起**详情查询（断言 mock server 只收到 1 次请求不是 2 次）。
- `TestR102LastMatchStartCachedSixHours`：连续两次调用间隔 5 小时，断言第二次零 HTTP 请求；间隔 7 小时后断言发起新请求。

**变异判据（必须 FAIL）：**
1. 把解析字段从 `GameStartTimestamp` 换成 `GameCreation` → 第一条 FAIL。
2. 空数组时仍然去调用详情接口 → 第二条 FAIL。
3. TTL 改成 3 分钟（复用段位缓存的 TTL）→ 第三条 FAIL。

---

## P3 — 种子解析预算重新设计（现有 3 秒超时不够用了）

**现状：** `loadProSeeds`（`pro_seed_accounts.go:76`）给每个选手固定 `context.WithTimeout(ctx, 3*time.Second)`，当时只覆盖一个账号的 2 次 Riot 调用（`accountByRiotID` + `leagueEntries`）。P1 改成多账号后，一个选手最多有 5 个账号（TheShy），每个账号要做 `accountByRiotID` + `leagueEntries`（P9/R101 已有）+ `lastMatchStart`（P2 新增，内部 2 次调用）= 每账号最多 4 次 Riot 调用，TheShy 一人就是 20 次调用，3 秒预算显然不够，会导致后面的账号系统性拿不到数据（不是随机失败，是必然截断）。

**要做什么：**

1. 每个选手的处理预算改成**按账号数量线性放大**，而不是固定常量。建议：`perAccountBudget := 4 * time.Second`（覆盖最多 4 次调用，每次留 1 秒，配额闸门内部有自己的排队等待不算在这个粗略预算里，真正的限流交给 `withRiotSingleWaitLimit` 处理），`seedCtx, cancel := context.WithTimeout(ctx, time.Duration(len(seed.Accounts))*perAccountBudget)`。
2. **单个账号超时/失败不能拖累同一选手的其它账号**——用独立的子 context 或至少确保 for 循环里某一账号出错时 `continue` 到下一个账号，不要整个函数提前 return 导致后面账号全部拿不到数据。
3. **配额耗尽时保留上次完整快照**（沿用 P9 已定的口径），且这次要按账号粒度保留——某个账号这轮没查到新数据就用它自己上一轮缓存的数据，不要因为同选手其它账号超时就把这个账号也清空。
4. 33 人多账号情况下，一次全量刷新可能达到上百次 Riot 调用，仍在 `wait()` 现有的 15/1秒、90/2分钟排队阈值内可以被消化，只是排队等待时间变长——**这是可接受的，不需要额外加并发**，种子刷新本身不在用户请求路径上（走后台快照刷新），排队慢一点不影响前台战绩查询。

**验收判据：**
- `TestR102SeedBudgetScalesWithAccountCount`：给一个 5 账号的选手 fixture，断言分配的 context deadline 明显长于给一个 1 账号选手 fixture 分配的（用具体数值断言两者之比 ≈ 账号数之比，不要只断言"变长了"这种弱断言）。
- `TestR102SeedOneAccountFailureDoesNotBlockOthers`：3 个账号中第 2 个模拟超时，断言第 1、3 个账号仍然拿到了数据。
- `TestR102SeedKeepsPerAccountLastSnapshotOnQuotaExhausted`：模拟配额耗尽，断言每个账号各自返回自己上一轮缓存值，而不是整个选手的数据被清空成 `unavailable`。

**变异判据（必须 FAIL）：**
1. 把预算改回固定 3 秒不随账号数缩放 → 第一条 FAIL。
2. 某账号出错时把整个 for 循环 `return` 掉 → 第二条 FAIL。
3. 配额耗尽时整选手清空而不是按账号回退快照 → 第三条 FAIL。

---

## P4 — 后端排序：去掉 Primary，新增按最近对局时间排序

**现状**（`pro_players.go:100-117` 结构体，`:512-532` 排序函数，`:646-663` 赋值逻辑，均已在调研阶段完整贴出，此处不重复）：`Primary bool` 是排序之后单独打的标记（"排完序第一个非 Dormant 且已定段的账号"）；`proAccountLess` 排序键顺序是 `Dormant → Tier → Division → LPKnown → LP → RankStatus → 名称字母序`，完全不看时间。

**要做什么：**

1. `proAccount` 结构体**删除 `Primary bool` 字段**，新增：
   ```go
   LastMatchAt      string `json:"lastMatchAt,omitempty"` // RFC3339，空字符串表示未知
   LastMatchAtKnown bool   `json:"lastMatchAtKnown"`
   ```
   （用字符串而不是 `time.Time` 是为了跟现有 `UpdatedAt string` 字段风格一致，且避免零值 `time.Time` 序列化出奇怪的日期。）

2. 组装账号时（`buildProPlayers` 里原来调 `normalizeProAccount` 之后的地方），对**每一个能拿到 PUUID 的账号**（不只是 seed，OP.GG 抓取来的账号如果本身带 puuid 也一样）调用 P2 的 `lastMatchStart`，填充上面两个新字段。查不到/未知的账号 `LastMatchAtKnown=false`，`LastMatchAt=""`。

3. **重写排序函数**，改名为 `proAccountLessByActivity`（保留旧函数名 `proAccountLess` 也可以，但函数体必须换成下面的逻辑，不要两套函数并存造成调用点分裂）：
   ```
   1. LastMatchAtKnown 不同 → 已知的排前面
   2. 都已知 → LastMatchAt 时间戳新的排前面（降序）
   3. 都未知 → 退回原有排序键：Dormant → Tier → Division → LPKnown → LP → RankStatus → 名称字母序
   ```
   即：**有真实最近对局时间的账号，无论段位高低，一律排在没有这个数据的账号前面**；未知时间的账号之间，用原有的段位规则兜底排序，不要改动这部分兜底逻辑本身（避免影响 P9 已验收过的种子无段位场景）。

4. `buildProPlayers` 里原来赋值 `account.Primary = true` 的那段代码整段删除（`pro_players.go:646-663` 附近，`primarySet`/`account.Primary = true`/`primarySet = true` 三行）。不需要任何"标记第一个"的逻辑——前端拿到的账号数组本身已经是按活跃度排好序的，数组第 0 个天然就是"当前最活跃的账号"，不需要额外布尔字段。

**验收判据：**
- `TestR102SortsByLastMatchTimeDescending`：3 个账号分别有已知的最近对局时间（不同），断言排序结果严格按时间降序，且**忽略它们的段位高低**（fixture 故意让段位最高的账号时间最旧，断言它没有排第一——这是直接复现用户截图里 Smash 那种"段位高但更久没打"的场景）。
- `TestR102UnknownLastMatchFallsBackToRankOrder`：全部账号 `LastMatchAtKnown=false`，断言排序结果与旧 `proAccountLess` 的结果完全一致（回归测试，证明兜底逻辑没被破坏）。
- `TestR102KnownAlwaysBeforeUnknown`：一个已知时间但段位很低的账号 vs 一个未知时间但段位很高的账号，断言已知的排前面。
- `TestR102PrimaryFieldRemoved`：反射或 JSON 序列化检查 `proAccount` 输出里不再含 `primary` 键。

**变异判据（必须 FAIL）：**
1. 把排序键顺序改回先看 Dormant/Tier → 第一条、第三条 FAIL。
2. 把"未知时间的兜底逻辑"从原 `proAccountLess` 逻辑换成别的顺序 → 第二条 FAIL。
3. 忘记删除 `Primary` 字段或其赋值逻辑 → 第四条 FAIL。

---

## P5 — 前端"主号"UI 改造

**现状**（均已调研确认，file:line 见下）：

- `web/pro-players.js:12`：`primaryOnly` DOM 引用。
- `web/pro-players.js:77`：`state.primaryOnly ? visibleAccounts.filter(account => account.primary).slice(0, 1) : visibleAccounts.filter(account => !account.dormant)`。
- `web/pro-players.js:85`：`${account.primary ? '<small class="pro-main-badge">主号</small>' : ""}`，以及 `<tr class="${account.primary ? "pro-primary" : ""}">`。
- `web/pro-players.js:141-145`：开关点击事件处理器。
- `web/index.html:154`：`<button id="pro-primary-only" ...>只看主账号</button>`。
- `web/pro-players.css:80-84,90,94-95`：对应样式（开关按钮、徽章、高亮行）。

**要做什么：**

1. 删除"主号"徽章 `<small class="pro-main-badge">主号</small>` 及其 CSS（`.pro-main-badge`）。**不要用别的词替换徽章文案**（比如"最新"/"常用"）——本轮口径是去掉这类静态标签，账号数组本身的排列顺序已经足够表达"谁更活跃"，不需要额外贴标签制造第二套语义。
2. 保留行高亮样式类，但改名语义：`account.primary` 改成判断"是否为该选手账号数组里的第一项"（即 `index === 0`），CSS 类名可以保留 `pro-primary` 不改（避免无意义的重命名扩散到测试和 CSS 文件），但**内部判断依据必须是数组下标，不是不存在的 `account.primary` 字段**（P4 已经把它删了，前端如果还读这个字段会一直是 `undefined`，必须改）。
3. `#pro-primary-only` 按钮**文案改成"只看最新账号"**，`id` 保留 `pro-primary-only` 不改（同上，避免无意义改名，且测试文件引用的 CSS 选择器不用跟着变），行为改成：`state.primaryOnly ? visibleAccounts.slice(0, 1) : visibleAccounts.filter(account => !account.dormant)`——**不再 filter `.primary` 字段，直接取排序后数组的第一项**（因为后端已经按活跃度排好序，`slice(0,1)` 天然就是"最新账号"）。
4. `aria-label`/无障碍相关文案如果引用了"主账号"字样，一并改成"最新账号"。

**验收判据：**
- jsdom：`TestR102NoPrimaryBadgeRendered`——渲染任意 fixture，DOM 里不存在 `.pro-main-badge` 元素、不存在"主号"文本节点。
- jsdom：`TestR102LatestOnlyTogglesShowsFirstAccountOnly`——点击 `#pro-primary-only` 后，每个选手最多渲染 1 行，且该行对应的账号是 `accounts[0]`（不是靠某个布尔字段筛出来的，是数组下标）。
- jsdom：`TestR102ButtonLabelSaysLatestNotPrimary`——按钮文本内容为"只看最新账号"。
- 既有 `desktop/pro-players.test.cjs` 里非主号相关的用例（历史折叠、会话核验等）继续绿，只改跟 `primary` 直接相关的断言（见 P7）。

**变异判据（必须 FAIL）：**
1. 徽章逻辑改回读 `account.primary`（此时永远 `undefined`，恒不显示）→ 第一条测试用假的方式通过（恒 false），但第二条会 FAIL，因为 filter 也会失效。
2. `slice(0,1)`改成基于不存在字段的 `filter` → 第二条 FAIL。
3. 按钮文案漏改 → 第三条 FAIL。

---

## P6 — 内置 33 名选手全部账号

**数据来源：** `docs/pro-accounts-verification-2026-09-17.md`（已落盘，本轮已完成，见文件开头的"数据来源"与"★ 唯一一处与用户上传原稿不同的地方"两段说明）。**逐字抄账号 GameName/TagLine，不要自己核实或改动**；`RealName`/`Position`/`OPGGID` 按 P1 要求从 `pro_roster.go` 取，不要瞎填。

**要做什么：**

1. 把 `proSeedAccounts`（P1 改造后的新结构）从当前只有 Rookie 一条，扩充到 33 条（每队一份，每份含该选手全部账号）。
2. **特别检查 Rookie 现有种子数据**：R99/R101 已内置的 Rookie 种子如果历史上误挂过 `dyjkbysb#KR1`，这次必须确认已被移除（按口径 4，该账号本轮归 Wei）。
3. Wei 的账号列表要包含 `dyjkbysb#KR1`（`docs/pro-accounts-verification-2026-09-17.md` 里 Wei 那一行）。

**验收判据：**
- `TestR102AllThirtyThreePlayersHaveSeeds`：断言 `proSeedAccounts` 长度为 33，且每支队伍（BLG/IG/T1/HLE/GEN/DK）的选手数分别是 7/6/5/5/5/5。
- `TestR102DyjkbysbBelongsToWei`：断言 `dyjkbysb`/`KR1` 这个 `(GameName,TagLine)` 组合只出现在 Wei 的 `Accounts` 里，Rookie 的 `Accounts` 里不含它。
- `TestR102SeedAccountsMatchVerificationDoc`：这条不要求解析 markdown 文件（太脆弱），改成人工核对后在测试里内联抄一份账号总数校验：33 名选手账号总数固定为某个数字（照 `docs/pro-accounts-verification-2026-09-17.md` 数一下当前是多少条账号，写死断言总数，防止录入时漏行或重复行）。

**变异判据（必须 FAIL）：**
1. 漏录一支队伍的一个选手 → 第一条 FAIL。
2. 把 `dyjkbysb#KR1` 错放回 Rookie → 第二条 FAIL。
3. 复制粘贴录入时重复了某个账号 → 第三条 FAIL。

---

## P7 — 测试迁移清单（旧断言如何处理，照 R101 账本的格式记录）

以下测试直接依赖被本轮删除/改造的字段或函数，必须逐条判断"断言的是旧行为、应当更新"还是"其实在守别的东西、要保留"，并在执行账本里写清楚每一条的处理理由，**不许无脑删除了事**：

- `r73_test.go:60-68`（`TestR73DormantPrimary`）：断言 `.Primary` 字段，字段已删除，编译都过不了。需要判断这条测试原本在守什么——如果是"Dormant 账号不该排到未知时间账号前面"这类语义，改写成基于新排序结果断言（比如断言 dormant 账号仍然排在同样未知时间的非 dormant 账号之后），不要整条删掉。
- `r101_test.go:436-446`（`TestR101SeedBecomesPrimaryOverLowerRankedAccount`）：断言 `.Primary==true`。这条测试对应的产品场景（种子账号段位更高应该排前面）在新逻辑下变成"种子账号如果最近对局时间更新应该排前面"，**这条测试的价值没有过时，只是断言方式要换**——改成给两个账号都配已知的 LastMatchAt（种子的更新），断言种子排第一；不要因为字段删了就删掉整条测试，会丢失这个场景的回归保护。
- `pro_players_test.go:131`：直接调用 `proAccountLess(...)` 的签名，如果 P4 改了函数名/签名，这里要同步改调用处。
- `desktop/pro-players.test.cjs:230-259`（"R73 主号、历史折叠、只看主号及会话核验"）：`:233` 手工设置 `accounts[0].primary=true`、`:240` 断言 `.pro-main-badge` 数量、`:248-251` 点击验证。徽章相关断言按 P5 删除，点击折叠行为的断言改成断言"折叠后每人只剩 1 行且该行是 `accounts[0]`"而不是找 primary 字段；测试名字里"主号"字样一并改掉。
- `desktop/pro-players.test.cjs:299`：`Object.assign(...,{primary:true})` 构造 fixture 的写法要去掉 `primary` 键（反正也不读了），改成用数组顺序表达"哪个账号在前"。

**验收判据：** 上面 5 处改完之后，`go test -count=1 -v .` 和 `node --test web/*.test.cjs desktop/*.test.cjs` 全绿，且不能靠"删掉断言"让测试变绿——执行账本里每一条都要写清楚"改写成了什么新断言"或者"为什么这条测试的场景已经被 P4/P6 的哪条新测试覆盖所以可以合并"。

---

## P8 — 文档更新

1. `docs/pro-players-sources.md`：更新"主号"相关措辞（如果文档里提到这个概念），改成描述"按最近对局时间排序"的新机制；三处 Wenbo 表述已在 `docs/pro-accounts-verification-2026-09-17.md` 里确认账号，本轮账号已内置，同步更新这三处不再是"等待入库"而是"已入库"。
2. `docs/r99-probe-results.md` / `docs/r101-*` 相关文档如果链接到 `docs/pro-accounts-verification-2026-09-17.md`，确认链接不再是死链接（文件已落盘）。
3. 新增一节说明 P2 的 match-v5 调用会给现有 Riot 配额预算增加多少稳态成本（33 人 × 平均账号数 × 每 6 小时 2 次调用，给一个大致数字），方便以后排查配额问题时有据可查。

---

## P9 — 未定级账号不要一直刷新段位（用户反馈，新增）

**背景：** `docs/pro-accounts-verification-2026-09-17.md` 里有不少"小号"账号已确认长期未定级（比如 TheShy 的 `스몰더 아빠#tsts`、`눈사람#cold1` 都是"未定级"）。R101 引入的 `proSeedRank`（`pro_seed_accounts.go` 附近，独立 `proseed-rank:v1:` 缓存键）目前固定用 **6 小时 TTL** 无差别刷新所有种子账号的段位，包括这些确认长期无排位场次的账号。33 人扩到约 50 个账号后，这类账号会占相当比例的刷新调用量，反复查一个从不定级的账号没有意义。

**要做什么：**

1. `proSeedRank` 查询结果分两种情况区别对待：
   - **查询成功但没有 `RANKED_SOLO_5x5` 条目**（`leagueEntries` 正常返回、只是列表里没有单双排）→ 视为"确认未定级"，**这次的缓存 TTL 改成更长（建议 72 小时）**，减少无意义的重复查询。
   - **查询失败**（网络错误/限流/超时）→ **不算"确认未定级"**，沿用原有 6 小时 TTL 的正常重试节奏——这两种情况必须用返回值/错误类型明确区分，不能用"这次没拿到段位"一概而论，否则会把"查询失败"也误判成"长期无段位"从而少查。
2. **这条退避只影响段位（`leagueEntries`）查询节奏，不影响 P2 的"最近对局时间"查询。** 未定级账号也可能是玩家正在用的活跃小号（Smash"段位不如大号但更活跃"的场景同样可能发生在未定级账号上），P2 的 `lastMatchStart` 仍然按原定 6 小时 TTL 持续刷新，不能因为账号未定级就连活跃度都不查了，否则这类账号永远排不到前面即使玩家确实在用。
3. 一旦某个账号在"确认未定级"的 72 小时退避期内，通过某次提前触发的 `leagueEntries` 查询（比如退避到期或别的路径触发）观察到它其实已经定级了，**要立刻用新 TTL（6 小时）重新计时**，不要因为"上次判定还在 72 小时窗口内"就拒绝更新。

**验收判据：**
- `TestR102UnrankedAccountBacksOffRankQuery`：某账号连续查询确认未定级，断言 1 小时后不发起新的 `leagueEntries` 请求，70 小时后仍不发起，73 小时后发起新请求。
- `TestR102FailedQueryDoesNotTriggerBackoff`：某账号查询返回网络错误，断言 1 小时后仍然正常重试（不套用 72 小时退避）。
- `TestR102UnrankedBackoffDoesNotAffectLastMatchQuery`：同一个未定级账号，断言 `lastMatchStart` 仍然按 6 小时 TTL 正常刷新，不受段位退避影响。
- `TestR102NewlyRankedAccountResetsToNormalTTL`：账号在退避期内的某次查询意外拿到了真实段位，断言缓存 TTL 重置为 6 小时（不是继续用 72 小时）。

**变异判据（必须 FAIL）：**
1. 把"查询失败"也算进退避条件 → 第二条 FAIL。
2. 把段位退避逻辑也套用到 `lastMatchStart` 上 → 第三条 FAIL。
3. 拿到真实段位后仍然沿用 72 小时 TTL 不重置 → 第四条 FAIL。

---

## 本轮明确不做的事

1. 天梯名次列（种子账号继续显示 `—`，沿用 R101 口径）。
2. "只看最新账号"开关之外的其它前端交互改动（左列布局、头像旗帜相关全部是 R101 范围，本轮不碰）。
3. 给 `docs/pro-accounts-verification-2026-09-17.md` 里的账号做进一步真实性复核——本轮以该文件为唯一事实来源，不再重新核对。

---

## 只能真机/真实网络验收的清单

1. P2 的 match-v5 真实调用返回的 `gameStartTimestamp` 字段名和单位是否与 Riot 现行文档一致（本工单基于 Riot 官方文档编写，未做真实 HTTP 探测，建议执行时先用一个测试账号真实调一次核对字段存在）。
2. 33 人全量种子刷新一轮的真实耗时和 Riot 配额消耗，确认 P3 的预算设计在真实网络延迟下够用。
3. 前端"只看最新账号"折叠后的真实视觉效果（沿用 R101 已经解决的左列宽度问题，不需要重新截图，只需要确认徽章删除后没有留白）。
