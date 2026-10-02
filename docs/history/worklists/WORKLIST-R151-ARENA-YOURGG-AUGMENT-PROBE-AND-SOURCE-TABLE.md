# WORKLIST-R151：斗魂海克斯能否整体改用 YOUR.GG——R150 追加的探测没有被执行

诊断人：Claude（验证 R150 执行结果时发现）。**执行人 GPT。** 日期：2026-09-24。基线：0.12.19 + R149 + R150（代码已合入，未改版本号、未打包）。
状态：分支 A 代码与全量验证通过，待网页 Network 和 Windows 真机验收；版本与打包暂缓。前置：R150 代码保持不动。

## 1. 为什么有这一单

R150 交付后我逐项核对：`docs/r150-probe-findings.md` 只有 §5 前 5 条的结论，**没有第 6 条（YOUR.GG 单英雄接口有没有海克斯 / 搭档数据）**，账本里也没有「斗魂各板块 → 数据源」对照表，也没有分支 A/B/C 的选择记录。仓库 `docs/` 里的 R150 文件只有 25058 字节，里面搜不到「分支 A」「`/kr/api/arena/champions/887`」，说明 GPT 拿到并执行的是**追加第 6 条之前的 R150 副本**（工单是手工复制，追加后需要重新复制，这一点在追加时已提醒过）。这不是 R150 代码的缺陷，所以按惯例不往已关闭的 R150 里追加，单开本单。**R150 不要再重新复制执行，以本单为准。**

用户的诉求没变：斗魂尽量都用 YOUR.GG。现状是：榜单、详情头部、核心装备、棱彩装备、冠军对局来自 YOUR.GG；海克斯、搭档、技能、反制等仍来自 OP.GG，斗魂海克斯卡片上的 S/A/B 是本地估算，也没有综合评分。

## 2. 探测（GPT 在用户真机，单资源、非批量、不记 cookie）

请求 `GET https://api.your.gg/kr/api/arena/champions/887`（即 `loadArenaChampionAggregate` 已经在用的接口，不增加请求量；也可以直接取缓存里的原始 JSON），**只记字段名与结构，不记数值**：

1. `response` 下除 `coreItems`、`prismaticItems`、`totalMatches`、`version` 之外还有哪些键？是否有 `augments` / `augmentGroups` / `silverAugments` / `goldAugments` / `prismaticAugments` / `synergies` / `partners` 一类的段？
2. 若有海克斯段：每项有哪些字段（`augmentId`、`tier`、`score`、`winRate`、`averagePlacement`、`firstPlacementRate`、`pickRate`、`matches`、品质）？条目数量大致多少，是否覆盖三个品质的全部海克斯，还是只有前若干项？`augmentId` 与 CommunityDragon `cherry-augments.json` 的 id 是否对得上（抽 3 个核对）？
3. 若有搭档 / 协同段：同样记字段，是否带官方档位。
4. 在 YOUR.GG 英雄页的 Network 面板里，是否还请求了别的接口（例如 `/kr/api/arena/augments`、`…/champions/{id}/augments`）？有就只记路径（不含 cookie），**只记录，不批量拉取**。
5. 再抽 2 个英雄（例如 3、157）重复第 1–2 条，确认结构稳定。

结论写进 `docs/r151-probe-findings.md`。

## 3. 按探测结果选分支（选定后把依据写进账本）

**分支 A：YOUR.GG 有完整海克斯数据（覆盖三个品质，带官方 `tier` 与 `score`）**——斗魂改成以 YOUR.GG 为唯一数据源：
- 海克斯行由聚合接口构造，名称、图标、品质仍用现有 CommunityDragon 海克斯目录对照；`Grade` 用官方字母（经 `normalizeChampionGrade("yourgg", …)`，不要另开映射），`Score` 用官方 `score`。**斗魂路径的本地分位估算（`applyLocalArenaAugmentGrades`）整体退役。**
- 斗魂海克斯卡片**恢复显示「综合评分」**（`score > 0` 才显示的规则保留，兜住个别缺失行）；卡片徽章全部是官方字母；`sortedGradeRows` 的排序不变（字母 → 官方评分 → 样本）。
- 搭档协同若 YOUR.GG 也有，一并切过去；没有就继续用 OP.GG，账本里记「搭档仍取 OP.GG」。
- YOUR.GG 聚合接口整体失败时，海克斯回退 OP.GG：回退行不带评分、不带官方档位，走 R150 已有的本地 S/A/B 字母兜底，沿用现有降级提示，不冒充官方。
- OP.GG 斗魂详情请求只保留 YOUR.GG 缺失的那几段（技能、反制等）需要的部分；能不请求就不请求。

**分支 B：YOUR.GG 没有海克斯，或只有部分品质 / 前若干项**——保持 R150 现状不动：海克斯继续用 OP.GG、不显示综合评分、本地 S/A/B 保留。部分覆盖时**不要**把两个来源的海克斯拼进同一张榜（口径不同，排序会失真），在账本里写明覆盖情况，交用户决定。

**分支 C：只有部分有用信息（例如只有搭档）**——按段处理：有官方数据的那一段切过去，其余不变，账本逐段列出。

无论哪个分支，都要在 `docs/r151-probe-findings.md` 里放一张「斗魂各板块 → 数据源」对照表（头部、榜单、海克斯、核心装备、棱彩装备、搭档、技能、反制、冠军对局），做完后更新。

## 4. 测试要求（仅分支 A / C 需要）

- Go：用 YOUR.GG 聚合接口的真实字段夹具（结构来自探测，数值可以脱敏）：海克斯行的字母、评分、品质、图标对照；id 缺失于目录时的降级；聚合失败时回退 OP.GG 且回退行 `Score == 0`、不带官方档位；三个品质都出现。
- Node：斗魂海克斯卡片有官方 `score` 时显示「综合评分」，没有时不显示（既有测试保持通过）；斗魂路径不再出现本地估算徽章。
- 对抗变异（让对应测试 FAIL 并整文件还原）：海克斯仍走 OP.GG；官方分被丢弃；聚合失败时不回退；回退行带上了本地分。
- 分支 B：不改代码，只交探测文件与对照表。

## 5. 仍待真机的 R150 验收项（不在本单改动范围，请顺手完成）

R150 账本已说明本机没有 Windows 客户端，下列项还没做：斗魂选正义巨像等 4 个英雄，头部与左侧选中行逐项对照并截图；各页面徽章统一性抽查（斗魂榜单/头部/海克斯卡/装备卡、海斗、排位、大乱斗、实时页）；侧边栏在默认主题与另一主题下量 logo 左缘与标题右缘留白（±1px，账本里目前是按 CSS 保守取的 214px，没有量到 Beaufort 实际宽度）；账号条长名字不溢出；导出日志确认 `arena_header_source` 有事件且 `hasListRow=true`。

## 6. GPT 的其他工作

真实依赖下 `go build` / `go vet ./...` / `go test ./...` 与 R150 基线对比，既有失败不得增加；`node --test backend/web/*.test.cjs desktop/*.test.cjs`。写 `docs/history/ledgers/r151-execution-ledger.md` 与 `docs/r151-probe-findings.md`，更新 `docs/WORKLIST-INDEX.md`。版本号 / 打包按用户此前的要求暂缓，除非用户另行通知。

## 7. 已知风险

- 分支 A 会改斗魂详情的整条海克斯路径，改动面比 R150 大，务必保持「YOUR.GG 失败 → 回退 OP.GG」的降级可用，并保留 R150 的头部/榜单一致性测试。
- YOUR.GG 若只返回每个品质的前若干项，用它替换 OP.GG 会让海克斯变少，这属于分支 B，不要为了统一来源而丢数据。
