# WORKLIST-R171：符文推荐卡片补召唤师技能推荐，并在"应用符文"时一并写入客户端

诊断人：Claude（只读代码调研，未做实机操作）。
执行人：GPT。
日期：2026-09-27。
来源：用户在对局页提的功能需求——符文推荐卡片（OPGG / 绝活哥 / 职业选手三个来源）差一个召唤师技能推荐；并问"应用符文时能不能把召唤师技能也一起改了"。

---

## 结论先行

两件事都能做，但数据完备程度因来源而异，需要如实告知：

1. **OPGG 来源**：召唤师技能的胜率/使用率数据**已经在抓**，只是没有出现在符文卡片里——它现在被渲染在"出装推荐"卡片而不是"符文推荐"卡片。改动主要在前端，把已有数据多展示一处。
2. **绝活哥（KR 专家）来源**：召唤师技能 ID 在抓到的真实对局里**已经解析出来了**（`riotParticipant.Summoner1ID`/`Summoner2ID`），只是 `specialist_runes.go` 组装符文推荐时把它们丢了没往下传。补上就行，后端改动很小。
3. **职业选手来源**：目前这条链路用的上游接口（LoL Esports 的 `window/details` 逐帧数据）**只解析了装备和符文，完全没有召唤师技能字段**（见 `backend/pro_runes.go` 的 `proDetailParticipant`/`proTeamMetadata` 结构体）。这不是"漏写"，是现有代码从来没从这个接口里取过这个字段——我没法确认这个接口本身有没有暴露召唤师技能（这类"帧数据"接口通常只给金币/装备/符文，不给征召阶段的选择）。**这一条需要 GPT 先去确认上游接口原始响应里有没有 `spells`/`summonerSpells` 这类字段**，有就加解析，没有就老实空着（不要编造），前端对应位置做"暂无数据"处理，不要为了填满而拿 OPGG 数据顶替职业选手数据源。

**应用符文时一并写入召唤师技能，技术上可行**：客户端在英雄选择阶段提供 `PATCH /lol-champ-select/v1/session/my-selection`，请求体 `{"spell1Id": <ID>, "spell2Id": <ID>}` 即可改自己的召唤师技能，和现有 `applyRunePage` 走的 `/lol-perks/v1/...` 系列是并列的、同样在英雄选择阶段才能用的客户端接口。仓库里 `backend/champselect.go` 已经有对 `/lol-champ-select/v1/session/...` 做 `PATCH` 的先例（自动选人/禁用），风格和请求方式是一致的，不是要引入新的调用模式。

---

## P1：符文卡片里加召唤师技能推荐区块

### 现状代码

- OPGG：`backend/gameplay.go:7016` `result.Build.SpellOptions = recommendationOptions(detail.Build.SummonerSpells)`——数据已经算好，挂在 `gameplayRecommendationBuild.SpellOptions` 上，目前只被 `backend/web/gameplay.js:6307` 附近的"出装推荐"面板（`renderRecommendationBuild` 一类函数）消费，符文卡片（`renderRuneRecommendations` → `renderRuneSourceSection` → `renderRuneChoice`，`backend/web/gameplay.js:6039-6237`）完全没碰它。
- 绝活哥：`backend/specialist_runes.go` 里构造 `gameplayRecommendationRune` 的地方（约 694-716 行，`return gameplayRecommendationRune{...}, true` 那段），可以直接访问的 `participant`（`riotParticipant` 类型）已经有 `Summoner1ID`/`Summoner2ID`（`backend/riot_api.go:626-627`），组装 `gameplayRecommendationRune` 时没有塞进去。
- 职业选手：`backend/pro_runes_recommendations.go` 里用的 `participant`（`proDetailParticipant`，定义于 `backend/pro_runes.go:91-95`）只有 `ID`/`Items`/`Perks` 三个字段，upstream 请求（`proDetails`/`proTeamMetadata` 相关的抓取函数）目前完全没碰过召唤师技能——需要先确认上游 JSON 里有没有这个字段，见下方"需要 GPT 先做的确认"。

### 改动方向

1. **后端**：给 `gameplayRecommendationRune` 结构体（`backend/gameplay.go:4275` 附近）加两个可选字段，例如 `Spell1ID int64 \`json:"spell1Id,omitempty"\`` / `Spell2ID int64 \`json:"spell2Id,omitempty"\``。
   - 绝活哥来源：在 `specialist_runes.go` 组装 `gameplayRecommendationRune{...}` 那处加上 `Spell1ID: participant.Summoner1ID, Spell2ID: participant.Summoner2ID`（同一份 `riotParticipant`，字段已经在，不用改抓取逻辑）。
   - 职业选手来源：**先确认**上游 `proDetails`/`window/details`（或职业选手数据实际请求的那个 LoL Esports 接口）原始 JSON 里是否有类似 `summonerSpells`/`spells` 的字段；有就在 `proDetailParticipant` 里加对应字段解析，同样透传进 `gameplayRecommendationRune`；没有就保留 `Spell1ID`/`Spell2ID` 为空（前端按"暂无召唤师技能数据"处理，不要报错、也不要拿别的来源顶替）。
   - OPGG 来源：`gameplayRecommendationRune` 目前是逐条符文页记录，`SpellOptions` 是英雄整体的召唤师技能推荐（不挂在某一条具体符文页上）——不需要塞进 `gameplayRecommendationRune`，直接在符文卡片渲染时把 `build.spellOptions`（已经在 `data.recommendation.build` 里）取第一条（胜率/使用率最高）单独展示成一个小区块即可，逻辑上和"某条符文页"是平行关系，不是它的字段。

2. **前端**（`backend/web/gameplay.js`）：
   - 在 `renderRuneSourceSection`/`renderRuneChoice`（约 6060-6224 行）里，每条符文推荐下面加一行召唤师技能展示：绝活哥/职业选手来源用 `config.spell1Id`/`config.spell2Id`（有数据才渲染，没有就不显示这一行，不要显示"0"或占位图标）；OPGG 来源的区块是卡片级别的（不是每条符文页一份），渲染 `spellOptions[0]`（已按胜率/使用率排好序，参考现有 `inUpstreamOrder(build.spellOptions...)` 的取法，`backend/web/gameplay.js:6307`）。
   - 召唤师技能图标复用已有的 `spellIconFigure`/`summonerSpellIcon`（`backend/web/gameplay.js:6516` 附近已有同名图标渲染逻辑，出装卡片在用），不要另起一套。

---

## P2：应用符文时可选一并应用召唤师技能

### 现状代码

- 前端"应用符文"按钮最终调用 `POST /api/gameplay/runes/apply`（`backend/main.go:558`），对应后端 `handleGameplayRuneApply`（`backend/gameplay.go:8483` 起）：先查 `gameflow-phase` 必须是 `ChampSelect`，校验请求体（`gameplayRuneApplyRequest`），再调 `applyRunePage(ctx, client, request)`（`backend/gameplay.go:8576`）——这个函数只发 `/lol-perks/v1/...` 系列请求（建符文页、写内容、切当前页），完全没碰召唤师技能。
- `handleGameplayRuneApply` 已经有 `perk_apply_attempt` 诊断埋点（成功/失败都记），阶段校验、请求校验、LCU 写入失败的错误处理都齐全，是一个可以直接复用的骨架。

### 改动方向

1. `gameplayRuneApplyRequest` 结构体加两个可选字段 `Spell1ID`/`Spell2ID`（不强制要求，前端不传就只应用符文，保持现有行为不变）。
2. 新增一个小函数，比如 `applySummonerSpells(ctx, client, spell1, spell2 int64) error`，内部就是一次 `client.RequestJSON(ctx, http.MethodPatch, "/lol-champ-select/v1/session/my-selection", map[string]any{"spell1Id": spell1, "spell2Id": spell2}, nil)`。
3. 在 `handleGameplayRuneApply` 里，`applyRunePage` 成功之后，如果请求体带了 `Spell1ID > 0 && Spell2ID > 0`，再调这个新函数；**召唤师技能写入失败不应该让整次"应用"报失败**——符文已经应用成功了，技能写入失败只需要在响应里单独标一个 `spellApplied: false` 之类的字段，前端提示"符文已应用，召唤师技能未能同步"，不要因为技能这半步失败就让用户以为符文也没应用上。诊断埋点（`perk_apply_attempt`）里加一个 `spell_applied` 布尔字段，方便以后从日志里看这条路径的实际成功率。
4. **已知限制，写进工单让 GPT 心里有数**：`my-selection` 这个接口只在英雄选择阶段、且你自己还没锁定的时候才能改；如果这次"应用符文"是在快锁定/已经进入 `FINALIZATION` 阶段点的，技能写入大概率会被客户端拒绝——这属于正常的"来不及了"，不是 bug，只要前端把失败提示做得清楚（"来不及改召唤师技能了，符文已应用"），不需要额外去猜测/重试。

### 前端改动方向

- "应用符文"按钮旁边加一个默认勾选的小选项，例如"同时应用召唤师技能"（有召唤师技能推荐数据时才显示这个选项；某个来源没有技能数据时，这次应用就只发符文，不显示选项或显示为禁用）。
- 请求体里带上当前展示的 `spell1Id`/`spell2Id`；成功响应里如果 `spellApplied === false`，弹出对应的提示文案（区分"符文和技能都应用成功" / "符文应用成功，技能没跟上"两种 toast）。

---

## 测试要求

- Go 单测：
  - `specialist_runes.go` 组装 `gameplayRecommendationRune` 的地方，补一条断言：给定一个带 `Summoner1ID`/`Summoner2ID` 的 `riotParticipant`，输出的 `gameplayRecommendationRune.Spell1ID`/`Spell2ID` 与之一致。
  - `handleGameplayRuneApply`：用 `httptest` 构造一个假 LCU（比照现有 `update_test.go`/`champselect_execution_test.go` 里搭 fake LCU server 的写法），验证请求体带 `Spell1ID`/`Spell2ID` 时确实对 `/lol-champ-select/v1/session/my-selection` 发了一次 PATCH，且 body 里 `spell1Id`/`spell2Id` 与请求一致；再补一条"符文成功但技能接口返回非 200"的用例，断言整体响应仍然是"应用成功"（HTTP 状态码不因为技能失败而变成失败），但响应体里 `spellApplied` 是 `false`。
  - 不传 `Spell1ID`/`Spell2ID` 时（老请求体），确认行为和现在完全一样，不发多余请求——这是兼容性回归项。
- 职业选手来源如果确认上游确实没有召唤师技能字段：补一条测试/断言"职业选手来源在没有技能数据时，`gameplayRecommendationRune.Spell1ID`/`Spell2ID` 恒为 0，前端不渲染这一行"，避免以后有人"顺手"拿别的来源数据填进来。
- 前端（Node `.test.cjs`）：补一条断言，三个来源里任一条 `Spell1ID`/`Spell2ID` 为 0（或缺失）时，召唤师技能这行不渲染，不出现占位或错误图标。

---

## 验收标准

1. 符文推荐卡片里，OPGG / 绝活哥两个来源能看到召唤师技能推荐（图标 + 名称），职业选手来源视上游数据是否可得而定（可得就一起做，不可得就如实留空，别的来源的数据不能顶上去凑数）。
2. 点"应用符文"时，如果当前来源有召唤师技能数据且用户没有取消勾选，客户端里符文和召唤师技能应该同时被改掉；技能写入失败（比如已经锁定英雄）时符文仍然应用成功，只是弹出对应提示。
3. 不带召唤师技能数据的老流程（比如某个来源确实没数据）完全不受影响，和现在的"只应用符文"行为一致。

---

*本工单由 Claude 基于代码只读调研完成，未做任何实机操作，未修改任何源码。职业选手来源的召唤师技能数据可得性需要 GPT 先核实上游接口原始响应后再决定要不要做，不要跳过这一步直接假设数据齐全。*
