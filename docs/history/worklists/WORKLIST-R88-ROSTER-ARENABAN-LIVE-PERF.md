# WORKLIST-R88 — 名单收窄 / 斗魂禁用 / 英雄页斗魂502 / 对局慢与连接错误

诊断日期：2026-09-14
证据来源：`lol-loot-diagnostics-0913-2329.jsonl`（9934 行，导出于本地 23:29 = UTC 15:29，本地 = UTC+8）+ 前两份日志做对照 + 用户截图四张 + 真 Chromium 实测 + 上游接口实抓。
所有 `file:line` 均为本轮亲自复核；与 R87 的关系（既有问题 / R87 回归）逐条标注。

---

## 红线

1. **`.match-main` 的四条轨道、`.match-stats { justify-self:start }` 一个字都不能动。** 这是第三轮重申，上一轮 R87 守住了，这轮继续守住。
2. **P1 的最终宽度值是实测出来的，不要凭感觉改。** 370px 是"斗魂真实名字零截断"的最小宽度，往下调就会开始截断。
3. **P3 不允许用"加大超时"糊弄。** 根因是请求风暴挤占浏览器同源连接数，必须从源头削请求量。
4. 每条都要有会因修改由红转绿、回滚即变红的护栏测试。

---

## ★★★ 先看这条：本轮日志有 76% 来自 R87 上线前的旧 build

`app_start` 的 `build_fingerprint` 显示：
- `1409a066c79f`（**R87 之前**）：11:51:24 ~ 15:09:48 UTC，log_seq 15720→21928，占 9934 行里的 **7588 行（76%）**；
- `eae1f783b367`（**R87 之后**）：15:09:48 UTC 起（本地 23:09），只有两段会话，合计约 19 分钟。

两个 build 的 `version` 字段**都是 "0.12.1"**，光看版本号分不出来。
⇒ **任何对 R87 效果的统计，都必须先按 `build_fingerprint` 过滤。** 本轮我第一遍统计 `position-broadcast` 的 reason 分布时就踩了这个坑：旧 build 下 78 条 `failed` 记录**根本没有 reason 键**（旧代码还没这个参数，不是 bug），真正落在新 build 上的只有 1 条失败样本。
⇒ **A1（position-broadcast 100% 失败）本轮仍然无法归因**，新 build 只产生了 1 次失败（reason 正确为 `session-read-failed`）+ 2 次成功，样本量远不够。**请不要拿本轮日志声称 A1 已查明。**

**要做的**：给 `app_start` 之外的关键统计事件也带上 build 标识，或者至少在导出日志的头部写一行 build 摘要，避免每次都要人工拆分。

---

## P1 · 总览玩家名单：固定宽度收窄到 370px + 两列等分 + 斗魂去掉编号

### 用户诉求（第二版，第一版方案已被否）
第一轮改成"420px 固定宽 + `space-between`"后，用户反馈「太丑了」。
第二轮我提了"靠右紧贴"，被明确否掉：「**像右边靠不行**」。
最终诉求原话：「玩家列表的宽度**还是一样宽**，但是**宽度要缩小点，中间不留空隙**，**斗魂玩家的名称也缩短**，反正**宽度肯定要一样宽**」。

⇒ 正确解读：**保持"所有卡片名单块严格等宽"这个硬约束，但把这个固定宽度调小到名字基本填满，于是中间自然没有空隙。**

### 当前代码
- `web/gameplay.css:620` `.match-summary` 第三条轨道 = `var(--match-roster-width,...)`
- `web/gameplay.css:804` `@container matches-column (min-width:1080px)` 下 `--match-roster-width: clamp(300px,36cqi,420px)` → **大屏下恒为 420px**
- `web/gameplay.css:697` `.match-players { width:100%; grid-template-columns: repeat(2,minmax(0,max-content)); justify-content: space-between; gap: 2px 14px; }` ← `space-between` 就是"中间大空隙"的来源

### 实测数据（真 Chromium，1920 宽，`lol-loot-ui-scale=1` 钉死）

**名字实际像素宽（用生产按钮 `.match-team-list button`，10.5px 字号，含头像+间距，克隆整张 `.match-entry` 保留完整祖先链）**：

| 真实样本 | 按钮宽 |
|---|---|
| 欧美风情#60098 | 112.53 |
| 尼玛派过来的1#83655 | 134.89 |
| 梦短梦长俱是梦#11401 | 138.70 |
| 我听见风的颜色#26247 | 138.70 |
| 这一波我是总编剧#55673 | 149.20 |
| 名字比以前更难取#13668 | 149.20 |
| FLAGGED 54895549#28336 | 172.11 |

min 112.53 / p50 138.70 / **p75 149.20** / p90 158.36 / max 172.11

**★方法学警告（执行方验收时务必避开）**：第一次测量把按钮克隆到脱离 `.match-team-list` 祖先的裸 `<div>` 里，丢掉了 `.match-team-list button { font-size:10.5px }` 这条后代选择器，字号回退到默认 ~16px，测量值系统性偏大 25~33%，算出来的"目标宽度"恰好是 420，会得出"不需要收窄"的**假结论**。**必须克隆整张 `.match-entry` 卡片保留完整祖先链再量。**

**四种候选组合（斗魂用 23 个用户真实截图里的名字做样本）**：

| 组合 | 宽度 | 斗魂单列宽 | 斗魂截断 | 截断率 | 普通卡片(≤8字) | 左/右缘 | 卡片高(斗魂/普通) |
|---|---|---|---|---|---|---|---|
| (a) 310px + 斗魂保留编号 | 310 | 88.66 | 12/12 | **100%** | 不截断 | 1484/1794 | 118/116 |
| (b) 310px + 斗魂去编号 | 310 | 88.66 | 5/23 | 21.7% | 不截断 | 1484/1794 | 118/116 |
| (c) 340px + 斗魂去编号 | 340 | 98.66 | 1/23 | 4.3% | 不截断 | 1454/1794 | 118/116 |
| **(d) 370px + 斗魂去编号** | **370** | **108.66** | **0/23** | **0%** | **不截断** | **1424/1794** | **118/116** |

- 三个宽度、六种名字长度下 `columnGap` **全部恒定 14px**，不再随名字长短伸缩 ——「中间不留空隙」达成。
- 同一宽度下 8 张卡片的 `playersWidth` / 左右缘**完全一致** ——「宽度一样宽」达成。
- 340px 唯一撞线的是 `BigRose丶Rengar`（英文名 + 全角"丶"），370px 才彻底盖住。
- 普通卡片 7 个真实样本里唯一截断的是 `FLAGGED 54895549`（22 字符纯英文），**各宽度下表现一致，与本次改动无关**。

### 要做的改动
1. **`web/gameplay.css:804`**：`--match-roster-width` 从 `clamp(300px,36cqi,420px)` 改为 **`370px`**（或 `clamp(300px,32cqi,370px)`，上限 370）。比现状收窄 50px（约 12%）。
2. **`web/gameplay.css:697`** `.match-players`：
   - `grid-template-columns: repeat(2,minmax(0,max-content))` → **`repeat(2,minmax(0,1fr))`**
   - **删掉 `justify-content: space-between`**
   - `width:100%`、`justify-self:end`、`gap: 2px 14px` 保持不变
   ★ R79 当年否决过 `1fr`，理由是"420px 下会把富余宽度平均塞进两列尾巴"。**实测确认那个否决只在宽度过大时成立**：270/310/350/370 四档下 `1fr` 均未复现末尾堆空白，因为末尾堆积是旧的 `space-between + max-content` 组合造成的，与 `1fr` 本身无关。
3. **斗魂名单去掉 `#编号`**：
   - `web/gameplay.js:5361` `playerParticipantName(player, index)` 里拼的 `${gameName}${tagLine ? "#"+tagLine : ""}` —— **不要直接改这个函数**，普通卡片和斗魂**共用**它。
   - 正确改法：`web/gameplay.js:2478` 的 `playerButton` 闭包里按 `grouping.arena` 分支处理 —— 斗魂分支只显示 `gameName`，普通分支（`.match-team-list`，2488 行）保留 `#编号`。
   - 斗魂三列同步改成 `repeat(3,minmax(0,1fr))`。
   - **★产品代价，必须做的补偿**：去掉编号后斗魂里**同名不同号的玩家完全无法区分**（LoL 允许重名，21 人局撞名概率比 5v5 高得多）。**要求：hover 提示（tooltip）里保留完整的 `名字#编号`**，只是列表可见文本去掉编号。
4. **清理死代码**（用户已确认"先不动，清理死代码"）：`web/gameplay.css:803 / 814 / 822` 附近三条 `@container arena-first (max-width:720px)` 规则，**`arena-first` 这个容器名从未在总览页任何祖先节点上声明过**，永不命中。
   - 复核证据：从 `.match-summary.is-arena` 向上遍历到 `<html>` 逐层读 computed `container-name`，只出现 `matches-column`（挂 `.matches-column`，`gameplay.css:608`）和 `gameplay-page`（挂 `.gameplay-content`），**全程无 `arena-first`**；全仓库唯一声明 `container: arena-first` 的是 `web/champions.css:877` 的 `.arena-first-section`（英雄详情页"吃鸡战绩"区块，与总览无 DOM 祖先关系）。
   - 删掉这三条，避免将来有人照着它改。

### 必须同步改判据的护栏
- **`desktop/ui-scale-layout.cjs`**：
  - `R79_ROSTER_MUTANT === 'players-1fr'` 分支目前把"改成 1fr"当作要抓的**坏**变异 —— **现在 1fr 是正确目标，语义必须反转**（改成"不许退回 max-content + space-between"）。
  - `verify('420-cap', ...)` 硬编码 420px 为正确值 → 改为 370。
  - `verify('content-columns', ...)` 断言"短名字必须比长名字有更大 columnGap" —— 这是旧 `space-between` 行为，**新设计下 columnGap 恒为 14**，此断言必须改为"columnGap 在所有名字长度下恒等于 14"。
- **`web/champions.test.cjs`**：涉及 `.match-players` / `.match-summary` 的正则断言要按新的 `repeat(2,minmax(0,1fr))` 重核，别继续断言旧的 `max-content` / `justify-content:space-between`。

### 验收判据
- 同页 8 张不同名字长度的卡片，`playersWidth` 严格相等（容差 0），`columnGap` 恒 14px。
- 斗魂卡片与普通卡片同页混排时左右缘严格对齐（1424 / 1794）。
- 斗魂 23 个真实名字样本零截断；普通卡片 ≤8 字中文名零截断。
- 卡片高度：普通 116px、斗魂 118px 均不变。
- `.match-main` / `.match-stats` 的 CSS 文本逐字未变（贴空 diff 证明）。
- 斗魂 hover tooltip 里能看到完整 `名字#编号`。

---

## P2 · 斗魂自动禁用仍然失效（★根因已由 R87 新埋点钉死）

### 症状
用户原话：「斗魂禁用还是不行，**选用可以了**」。
⇒ R87 P6 修好了勇敢举动自动选用（用户截图 23:16:03「已确认选用并锁定勇敢举动」✅），**禁用仍然全败**。

用户截图记录（本地 23:15:31~23:15:54）：
```
23:15:54  检测到手动禁用，已让位
23:15:34  客户端禁用列表没有提供任何英雄 ID，并非队友已选择全部候选；已记录原始 ID 与会话状态，等待客户端更新
23:15:34  跳过腕豪 / 暗夜猎手 / 血港鬼影：客户端可用列表未包含
```

### R87 新加的 `champselect_ban_probe` 直接给出答案（168 条，取值只有两种）
```
148条: {has_ban_action:true, queue_id:1750, raw_count:1,   requested:true, status_code:200}
20条:  {has_ban_action:true, queue_id:1750, raw_count:174, requested:true, status_code:200}
```
**这同时推翻了 R87 列的 A、B 两个假设**：`has_ban_action` 为 true（不是"没有 ban action 所以没请求"）、`requested` 为 true 且 `status_code` 200（不是"端点没被调用"）、返回体也不是空的。

### 根因（已证实）：又一处硬编码 queueID，这次是 3110

`champselect_execution.go:86`：
```go
if len(raw) == 1 && raw[0] == -1 && session.QueueID == 3110 && active && action.Type == "ban" {
```
`raw_count == 1` 那 148 条，返回的就是哨兵值 **`[-1]`**（"客户端不给具体列表"的通配符）。处理 `[-1]` 的这条兼容路径被**硬编码限制在 queueID == 3110（自定义房间）**，斗魂 1750 进不去 → 落到 `champSelectIDSet`（`champselect.go:1090-1098`，`if value > 0` 过滤掉负数）→ 空集合 → 打出"客户端禁用列表没有提供任何英雄 ID"。

**这和 R87 P6 修的 `champselect_execution.go:212` 是同一类错误的第二处。** 上一轮只修了 212 行，没扫到 86 行。

### 时间线还原（UTC = 本地 - 8h）
- 15:15:25.35 PLANNING 阶段，正确选用"勇敢举动"（intent）。
- **15:15:34.76** `champselect_evaluation`：进入 BAN_PICK，`has_local_action=true, action_type=ban` —— 真正轮到本人 ban 的唯一窗口起点。
- 15:15:34.80 同周期 `bannable-champion-ids` 返回 `[-1]` → available=0 → 555/67/875 全判 `unavailable` → 打出"跳过XXX" → `watch_action` 记 `exhausted`。
- 15:15:36.66 ~ 15:15:54.67，约 20 秒内**连续 45 次以上**探测全部是 `[-1]`，无一次翻正。
- **15:15:54.77** 突然出现 `raw_count=174`（真实列表，**含 555/67/875**）；但**同一周期** `champselect_trace` 记 `manual-takeover`（人已手动 hover 了 ban），程序按设计"已让位"退出。
- 15:15:57.30 `has_local_action=false`，ban 回合结束。

⇒ **关键发现（超出原假设）**：真实列表**只在人工已经手动 hover 之后才出现**。形成"自动化必须先有 hover 才能拿到真列表，但没有真列表就永远 hover 不了"的**死锁** —— 这正是 `champselect_execution.go:63` 注释里描述的 3110 自定义房间那套 LCU 行为，只是这次发生在 1750。
⇒ 顺带证伪了假设乙：真实列表出现时 555/67/875 **全都在里面**，从未被"候选不在列表内"挡住。

### 要做的改动
1. **`champselect_execution.go:86`**：把 `session.QueueID == 3110` 这个队列限定**去掉**，改为"任意队列，只要 `raw == [-1]` 且是本地未完成的 ban 动作"就走 grid 兜底候选路径。`source` 标签从 `custom-*` 改成更通用的命名（如 `wildcard-grid-hover` / `wildcard-session-evidence-hover`）。
2. **全仓库扫一遍同类硬编码**：`champselect.go:360`、`champselect.go:556` 也各有一处 `3110`，一并评估是否需要同样处理。
3. **AST 守卫必须扩容**（见下）。

### AST 守卫为什么没拦住（护栏漏洞，必修）
`r87_test.go:50-91` 的 `arenaLiteralComparisons` 只识别字面量 **1700 / 1710 / 1750** 三个 Arena 队列号（见 `isArena` 里的 `value == 1700 || value == 1710 || value == 1750`）。86 行比较的是 **3110**，不在枚举里 —— **不是漏检，是从未被纳入扫描目标**。
加上 R87 验收时已经发现的另外两个绕过口子（先把 `session.QueueID` 赋给变量再比较、改用 `switch session.QueueID { case 1700,1710: }`），这个守卫目前有**三个已知缺口**。

**要做的**：把守卫从"枚举 Arena 三个队列号"改为"**champselect 相关文件里禁止出现任何 queueID 与整型字面量的直接比较**（包括 switch-case 和先赋值再比较），一律要求走 `queue_groups.go` 的登记表 / `groupID` 判定"。并补三条变异测试分别覆盖：直接比较、变量中转、switch-case。

### 需补的埋点
`champselect_ban_probe` 目前只有 `raw_count`，**没有 raw 的实际值**。本轮是靠另一份重量级的 `champselect_source` 导出才补上"raw 就是 `[-1]`"这个证据。
**要求补一个 `raw_head` 字段（raw 的前 3 个元素）**，下次不必依赖重量级导出即可直接判定。

### 验收判据
- 单元测试：构造 QueueID=1750、`bannable-champion-ids` 返回 `[-1]`、本地 ban action 进行中，验证 grid 兜底路径被走到且最终发出 ban 的 PATCH。**把 86 行的 `session.QueueID == 3110` 加回去，该测试必须变红。**
- 3110 自定义房间原有行为不回归。
- 新 AST 守卫：分别注入"直接比较 3110""变量中转比较 1700""switch-case 1700/1710"三种写法，**三种都必须被拦截**。

---

## P3 · 对局页极慢 + 「无法连接本地助手 / 本地请求超时」（★R87 新引入的回归）

### 症状
用户原话：「对局现在**反应很慢**，都进游戏很大一会了都没有展示出推荐，有时候会像图四这样**连接错误**，**问题很大**」。
截图：对局页整页报错「无法连接本地助手 / 本地请求超时，请重试 / 重新连接」，左下角「未检测到客户端」。

### 先证伪两条容易想当然的猜测
**(1) "预热让每次计算变慢"——证伪。** `live_load_cost.total_ms` 分布（已按 build 拆分）：

| 版本 | n | p50 | p90 | p99 | max |
|---|---|---|---|---|---|
| 1646 旧 | 244 | 64 | 1407 | 4884 | 7648 |
| 1951 旧 | 405 | 50 | 1981 | 7772 | 12322 |
| **2329 新（仅 post-R87 两段）** | 82 | **16** | 845 | 6921 | 9967 |

新版单次耗时与旧版同量级、p50 反而更快。按玩家数拆：ChampSelect(3人) p50=9/p90=29/max=2332ms；InProgress(18人) p50=53/p90=3196/max=9967ms —— 18 人局的慢是 LCU/Riot 侧固有延迟，两版都有。

**(2) "预热撑爆 LCU 连接池/抢锁"——证伪。** 两个新会话的 `conn_wait_ms` 全部 ≤20ms，match-history / ranked-stats 错误率 **0%**（旧版反而有 17~28% 的 `http_status=0` 超时），没有队列堆积证据。

### 已证实的真根因：R87 新埋点 `recordLiveRefresh` 零节流，把浏览器同源连接数打满

**(a) 请求风暴是实测出来的**：`live_refresh_client` 事件 **361 条**（旧日志 0 条，R87 新增），全部集中在 post-R87 的两段短会话里（19 分钟）。
- reason 分布：phase 193 / queue 71 / load 97；
- **15:15:40~15:15:41 这 1 秒内出现 13 条**，5 秒窗口内最高 33 条；
- 总体 **66.7% 的相邻间隔 < 1 秒**。

**(b) 代码里确实没有任何节流**：`web/gameplay.js:5981-5985` 的 `recordLiveRefresh` 直接 `fetch("/api/diagnostics/client", {method:"POST"})`，**无防抖、无合并、无采样**。它被两处无条件调用：
- `web/gameplay.js:5956` `handleGameplayPhase` 开头 `recordLiveRefresh("phase", ...)` —— **不管 phase 有没有真变化都发**；
- `web/gameplay.js:5958` `queueLiveEventRefresh` 开头 `recordLiveRefresh("queue", ...)`。
而 `handleGameplayPhase` 的上游是 SSE 的 `gameflow:` / `champselect:changed`（后端 300ms 防抖）**加上每秒一次的 `pollGameflowPhase`**。ChampSelect 阶段 LCU 本身就高频推送微小会话变更（英雄意向、悬停），被这条无节流埋点放大成每秒十几次额外 HTTP。

**(c) 为什么会变成"整页连接错误"**：
- 文案位置：`web/app.js:386`「本地请求超时，请重试」、`web/app.js:2510/2514`「未检测到客户端」「无法连接本地助手」。
- 触发链：`web/app.js:405-450` 的 `refreshStatus()` 用 8000ms 超时请求**自己的** `/api/status`（**不是 LCU**），catch 里**无重试、无连续失败阈值**，一次失败就直接 `showFatal(error.message)`（`web/app.js:447`）整页翻脸。
- ⇒ **左下角"未检测到客户端"是 `showFatal` 的硬编码文案，具有误导性** —— 实际是前端连不上自己的本地后端，不是 LCU 掉线。
- 机制：`main.go:586-591` 后端是纯 HTTP/1.1 无 TLS，Chromium 对同源 HTTP/1.1 **限制 6 个并发连接**。361 条无节流诊断 POST 与 `/api/status`、`/api/gameplay/phase`、`/api/gameplay/live` 抢同一组 6 个槽位，13 条/秒的突发足以把 `/api/status` 挤到排队超过 8 秒。
  **★这一步是推测机制**（日志里没有"前端请求自己后端"的耗时埋点），方向合理但非直接证实 —— 见下方"需补埋点"。

### 另一个拖慢点（非 R87 引入，但值得一并改）
`gameplay_refresh.go:119-125`：`cachedGameplayLive` 是"**预热后仅消费一次**"设计（每次命中后立即 `c.at = time.Time{}` 清零）。普通轮询把预热帧吃掉之后，后续仍要做全量 18 人重新拉取。这解释了 InProgress 阶段 p90=3196ms、时有约 10 秒的单次刷新。旧版同等量级已存在，只是被前端埋点风暴放大了主观感受。

### 要做的改动（按优先级）
1. **【P0】`recordLiveRefresh` 加节流**：最小间隔 500ms~1s，或把 phase/queue/load 合并成一次上报。**这是砍掉突发量的根本手段。**
   - 更彻底的做法：诊断埋点改为**本地攒批**，每 5~10 秒或攒够 N 条再一次性 POST。
2. **【P0】`refreshStatus` 失败不要一次就 `showFatal`**：改为连续 N 次（建议 3 次）失败才整页报错，中间静默重试；超时从 8s 放宽或加一次静默重试。
3. **【P1】文案要诚实**：连不上本地后端时不要显示"未检测到客户端"，应明确区分「本地助手无响应」与「未检测到英雄联盟客户端」这两件完全不同的事。
4. **【P1】`cachedGameplayLive` 改成"命中后在 TTL 内可多次复用"**，而不是消费一次即失效。

### 需补的埋点
前端对**自己后端**的请求耗时目前零埋点，导致"连接错误到底是排队还是后端真卡"只能推测。建议在 `web/app.js` 的 `api()` 里记录每次请求的发起/完成时刻与耗时（采样上报即可，**注意别再造一次请求风暴**）。

### 验收判据
- 前端测试：模拟 ChampSelect 阶段 1 秒内 13 次 phase 事件，`/api/diagnostics/client` 的实际 POST 次数必须 ≤2。回滚节流则变红。
- 前端测试：`/api/status` 单次超时不得触发 `showFatal`；连续 3 次才触发。
- 手工：进对局后对局页数据出现时间明显快于改动前（对照组用当前 build）。

### 发现的其他相关异常
- `/lol-summoner/v2/summoners/puuid/{id}` 在 post-R87 的 18 人局里 **12.6 分钟打了 585 次（约 46 次/分钟）**，远高于 pre-R87 同类窗口约 20 次/分钟。**怀疑身份解析没有跨刷新周期缓存**，建议单独复核。
- 旧版基线 `/lol-match-history/.../matches` 错误率 **17~28%**（`http_status=0`，超时/网络错误）。这是**预先存在**的问题，与 R87 无关，但很可能是"进游戏很久没推荐"的**另一半原因**，建议单独立项。

---

## P4 · 英雄页「斗魂竞技场」页签整页 502（★上游改版触发的既有缺陷，非 R87 回归）

### 症状
用户截图：英雄页 →「斗魂竞技场」页签 → 整页「数据读取失败 / 英雄数据暂时不可用，请稍后重试 / [重试]」。另外两个页签正常。

### 根因（已证实，我亲自抓了上游接口复现）
链路：前端 `/api/champions/rankings?mode=arena` → `champions.go:695 handleChampionRankings` case "arena" → `champions.go:1891 loadArenaRankings` → 直接 fetch `https://api.your.gg/kr/api/arena/champions` → `yourgg_arena_rankings.go:10 parseYourGGArenaRankings` 强校验。

**`yourgg_arena_rankings.go:41`**：
```go
tiers := map[string]int{"S": 0, "A": 1, "B": 2, "C": 3, "D": 4, "F": 5}
```
**`yourgg_arena_rankings.go:43`**：`tier, ok := tiers[row.Tier]`，`!ok` 时 → `return championRankingResponse{}, invalid` —— **整表作废**。

**我实抓的上游现状**（沙箱能出网，2026-09-14）：
```
http=200 bytes=44951   (与日志里记录的字节数完全吻合)
总数 173
tier 分布: B:52  C:46  D:35  A:28  F:6  S:5  OP:1
首条: {"championId":3, "tier":"OP", "score":98.03, ...}
```
⇒ **YOUR.GG 新增了 "OP" 这一档（在 S 之上），全表 173 个英雄里只有 1 个是 OP**。就因为这一个未知档位，**整份 173 个英雄的列表被整体丢弃** → 502 → 用户看到整页失败。

**文案对应**：`champions.go:1056 writeChampionResponse` 里 `http.Error(w, "英雄数据暂时不可用，请稍后重试", 502)`，前端 `web/champions.js:243` 抛错 → `:422` 存入 `state.error` → `:2065 renderError` 渲染"数据读取失败"。**与用户截图逐字符匹配。**

**日志佐证**：`champion_upstream` host=api.your.gg 共 24 条，**status 全部 200**，无一次网络失败；15:13~15:17 同一字节数 44951 反复命中内存缓存但每次仍报错 ⇒ **解析层 100% 确定性失败**，不是网络抖动。

**不是 R87 回归**：`git status` 确认 R87 未改动 `champions.go` / `yourgg_arena.go` / `yourgg_arena_rankings.go` / `champions_structured.go` 任何一行。

### 要做的改动
1. **`yourgg_arena_rankings.go:41`** tiers map 加 `"OP"`。产品决策点：映射到 S 同档，还是新增一个独立最高档（前端六色徽章可能要加一档）。
2. **【更重要】把"未知 tier → 整表作废"改成"跳过该行并计数"**。一个新分档就能让整个页面挂掉，这种脆弱性比这次的具体 bug 更值得修。同一函数里其它强校验条件（`row.Score > lastScore`、`AveragePlacement` 范围等）也要一并评估：**哪些该整表作废，哪些该跳过单行**。
3. **补埋点**：`parseYourGGArenaRankings` 目前**零诊断埋点**，日志上完全看不出是哪条校验规则触发的。加一条 `arena_rankings_parse_failed`，带上具体触发的字段名和值。这次是靠我直接 curl 上游才定位到，下次不能再靠运气。
4. **上游不可用时的降级**：区分「HTTP 失败/超时」（维持现有 502 文案）与「拿到 200 但格式变更」（应尽量展示已知档位、跳过未知行，而不是整页失败）。

### 验收判据
- 单元测试：喂一份含 `"tier":"OP"` 的上游 payload，必须成功解析出 173 行（回滚修改则变红）。
- 单元测试：喂一份含完全未知 tier（如 `"XYZ"`）的 payload，必须跳过该行并返回其余行 + 产生一条 `arena_rankings_parse_failed` 诊断。

### 存疑
未直接哈希比对用户当时收到的字节与我复测的 44951 字节是否为同一份内容（仅由字节数吻合推断）。OP 档在 UI 上归入哪个色阶需产品确认。

---

## 附加发现

### A4 · R87 新 build 上线 7 分钟后在斗魂 BAN/PICK 阶段疑似崩溃重启【推测，n=1】
run_id `145c36200b4a6d90921813c6`（新 build）最后一条是 15:16:37.337 的 `champselect_ban_probe`（queue 1750，`champselect_trace` 正在持续轮询 ban/pick，`previous_repeats:59`），日志直接断掉；**24 秒后**新 run_id `1ddfbdd5620c2e5e79874e23` 出现 `app_start`（15:16:40.215），`desktop_startup` 记 `total:1121ms / spawnToReady:501ms`，**明显快于正常冷启动的 4400~4700ms**，符合"刚崩溃重启、系统缓存还热"的特征。build fingerprint 未变，不是自动更新触发。

**`desktop/main.cjs` 全文检查未发现 `render-process-gone` / `child-process-gone` 等崩溃捕获钩子**，只有 `before-quit`(680行) / `window-all-closed`(688行) 两条正常退出路径 —— **异常终止时不会写任何诊断记录**，这就是现在完全看不到根因的原因。

**要做的**：给 Electron 主进程加 `render-process-gone` / `child-process-gone` / `uncaughtException` 捕获并写入 desktop.log；给 `app_start` 增加"是否因日志轮转触发"的标记位（见 `main.go:1570-1578` 的 `enableDiagnosticRotationSnapshot`），把"日志轮转"和"真实进程重启"区分开。

### A5 · `live_refresh_client` 有四个永远为死值的字段【R87 新埋点自身缺陷】
361 条记录里 `duration_ms` 100% 为 0、`claiming` 100% false、`total`/`done` 100% 为 0、`hidden` 100% false。
根因：前端 `web/gameplay.js:5981-5985` 的 `recordLiveRefresh` 只发 7 个字段（event/reason/source/hidden/section/phaseChanged/liveRefreshQueued），根本不带 `claiming/done/total/durationMS`；但后端把 `live_refresh_client` 和 `claim_progress_client` 塞进**同一个分支**处理，无条件写入这四个字段（`features.go:198-210`）。
**危害**：不影响功能，但极具误导性 —— 我分析时一度以为它证明"刷新总是瞬时完成"，实际是字段从未被赋值。
**要做的**：把两个事件的字段白名单拆开，或让后端只写实际收到的键。

### A6 · 桌面壳日志只按 2MB 滚动、不按时间过期，导出里混进 3 周前的陈年报错
本轮日志末尾出现两条 `desktop_error`，时间戳是 **2026-08-23** 和 **2026-08-29**（距今 3 周），消息都是"启动页加载失败"，和今天的 `desktop_startup` 混在一起。
`desktop/desktop-log.cjs:36-53` 的 `desktopLogForExport` 无条件读取 `desktop.log` ~ `desktop.4.log` 共 5 代全部内容，滚动阈值 `MAX_BYTES = 2*1024*1024`（第 3 行）只按大小触发、**不设时间上限**。
**危害**：不是功能 bug，但会持续误导未来的日志诊断 —— "启动页加载失败"这种低频错误可能几个月才滚出 5 代文件，每次导出都重新出现，容易被误判为"最近发生"。
**要做的**：导出时按时间窗过滤（如只带最近 7 天），或给每条 desktop 事件打上"距本次导出多久"的相对标记。

### A7 · A1（position-broadcast 100% 失败）本轮仍无法归因
见文首"76% 旧 build"那一节。新 build 只产生 1 次失败 + 2 次成功。**请不要拿本轮日志声称已查明**；需要用户在新 build 上多打几局大乱斗后重新导出。

---

## 执行顺序建议

1. **P3**（用户说"问题很大"，且是 R87 引入的回归，优先级最高）
2. **P2 + AST 守卫扩容**（根因已钉死，改动小；守卫的三个缺口必须一起补，否则下次还会有第三处硬编码）
3. **P4**（上游已改版，现在是必现故障；顺带把"整表作废"这个脆弱模式改掉）
4. **P1**（纯 CSS + 一处 JS 分支 + 三组护栏判据重写；宽度值已实测锁定为 370px）
5. **A4/A5/A6**（先补埋点与捕获，下一轮才有证据）
6. **A7**（等新 build 的日志）
