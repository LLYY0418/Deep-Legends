# WORKLIST-R152：斗魂海克斯改用 YOUR.GG 后的三个收尾问题（单行坏数据拖垮整段、局内卡片缺场次、下发体积）

诊断人：Claude（验证 R151 时读代码发现，未在真机复现；这三条都来自对 `backend/yourgg_arena.go`、`backend/champions_structured.go`、`backend/web/gameplay.js` 的走读）。执行人 GPT。日期：2026-09-24。基线：0.12.19 + R149 + R150 + R151（未改版本号、未打包）。
状态：代码与全量验证通过，待 Windows 真机验收及 R150 历史首次加载耗时对照；按用户要求暂不改版本号或打包。执行证据见 `docs/history/ledgers/r152-execution-ledger.md`。前置：无。R150/R151 的行为保持不动。

## P1 单行数据不合格就整段回退 OP.GG，过于脆弱

### 现状

`mapYourGGArenaAggregateAugments`（`yourgg_arena.go`）里，只要**任意一行**满足下面任一条件就 `return nil, error`，随后 `loadStructuredDetail` 把整段海克斯回退成 OP.GG（约 45 条、本地 S/A/B、无综合评分）：`AugmentID <= 0`、`Matches <= 0`、`Score == nil`、档位不在 OP/S/A/B/C/D/F、`AugmentID` 重复。探测里三个英雄各 173–185 行全部合格，所以今天不会触发；但上游只要在长尾里出现一行 `matches: 0`（新出的海克斯、刚被削弱的海克斯都可能），用户看到的就是**整页海克斯突然退回旧样子**，且日志里只有一个笼统的 `reason=augment-invalid`，看不出是哪一行、哪个字段。

### 要做

1. **按行容错**：不合格的行**跳过**，不再让整段失败；跳过时按原因计数。整段回退只保留这些情形：聚合请求失败、`augments` 缺失/为空、海克斯目录不可用、跳过之后**三个品质任意一个为空**（沿用「不许悄悄变少」的原则）、或有效行少于总行数的 80%（上游结构明显变了，宁可回退）。80% 写成具名常量并在注释里说明来由。
2. **重复的 `augmentId`**：保留**场次更多**的那一行（并列取先出现的），其余计入重复数，不再整体失败。
3. **诊断**：`arena_augment_source` 增加 `rows_in`、`rows_kept`、`skipped_matches`、`skipped_score`、`skipped_grade`、`skipped_id`、`skipped_duplicate`（都是整数，不含任何行内容）；回退时 `reason` 细分为 `aggregate-failed`、`augment-missing`、`catalog-failed`、`too-many-invalid`、`quality-missing`，替换现在笼统的 `augment-invalid`。
4. 保持既有语义：评分只取上游 `score`，档位只经 `normalizeChampionGrade("yourgg", …)`，回退行仍然 `Score == 0` 且不带官方档位；不拼两个来源的海克斯。

### 测试

- Go：一行 `matches:0` → 该行被跳过、其余行保留、`skipped_matches=1`、仍走 YOUR.GG；重复 id → 保留场次多的一行；缺失 `score` / 未知档位各一行 → 跳过并各自计数；有效率低于 80% → 回退且 `reason=too-many-invalid`；跳过后某品质清空 → 回退且 `reason=quality-missing`；R151 原有 3 条测试保持通过。
- 对抗变异（让对应测试 FAIL 并整文件还原）：恢复「任一行不合格即整体失败」；重复 id 改成保留后出现的；去掉 80% 门槛；诊断里带上行内容。

## P2 局内斗魂海克斯卡片只剩「胜率」，没有「场次」

### 现状

`web/gameplay.js` 约 5573 行：斗魂来源的海克斯指标是 `胜率 + 选用率`，其他模式是 `胜率 + 场次`。R151 之后斗魂海克斯来自 YOUR.GG，**没有选用率**（`PickRate` 恒为 0，且 `omitempty` 不下发），选用率被 `hasMetric` 正确隐藏，于是局内卡片只剩一个胜率。数据里其实有场次（`Games`），只是斗魂分支没用它。

### 要做

- 斗魂来源的指标改为 `胜率 + 场次`（与其他模式一致，用同样的 `场` 后缀与格式）；**仍保留选用率的读取**：某一行确实带了 `pickRate > 0` 时（OP.GG 回退行），显示顺序为 胜率、选用率、场次里有值的项，最多两项，避免卡片变挤；两项都没有时整块不画（沿用现状）。具体取舍以真机排版为准，写进账本。
- 不新增说明文字。

### 测试

Node：给定一行只有 `winRate` 与 `games` → 显示「胜率」与「场次」；给定回退行有 `winRate` 与 `pickRate` → 显示「胜率」与「选用率」；三者都有 → 最多两项且顺序固定；都没有 → 不画统计块。对抗变异：斗魂分支改回 胜率 + 选用率 → FAIL。

## P3 海克斯条目从约 45 增到约 180，下发体积需要量一下

### 现状

R151 之前斗魂详情的海克斯是 OP.GG 的 45 行（`arena_augments_built` 日志里 `rowsOut=45`）；现在是 YOUR.GG 的 173–185 行，每行带 `Assets[0].Description`（技能描述文本）。这些行会同时出现在**详情响应**和**局内推荐响应**（`gameplay.go` 约 6831 行 `result.Augments = detail.ArenaAugments`）里。局内推荐每个英雄都会请求一次，体积和序列化耗时按 4 倍增长，而局内页每个品质最终只画前 3 个。

### 要做

1. **先量**：在真机对同一英雄（例如 887）记录 R150 基线与当前的详情响应字节数、局内推荐响应字节数、`decorate`/序列化耗时（已有 `mayhem_detail_phases_ms` 这类阶段日志的做法可参照，斗魂没有就临时加一个只记数字的事件，账本里写数值）。
2. **超过阈值才动手**：局内推荐响应比 R150 基线增长超过 150KB，或斗魂详情首次加载耗时明显变长（比 R150 基线多 300ms 以上）→ 局内推荐路径里对每个品质只下发排序后前 N 条（N 取 6，留余量给前端的 `slice(0, 3)` 与品质待确认列），并且**去掉这些行里的 `Description`**（局内卡片的 tooltip 用的是 `asset.description`，先确认 tooltip 是否还需要；需要就保留前 N 条的描述，其余不带）。详情页（斗魂详情面板）仍下发全部条目，因为它有「展开全部」。
3. 没超阈值就**不改代码**，只把测量数据写进账本。

### 测试

阈值触发时：Go 断言局内推荐每个品质条数不超过 N 且排序与前端一致（字母 → 评分 → 场次）；详情响应条目数不变；Node 断言局内卡片 tooltip 仍有名称与描述。

## GPT 的其他工作

真实依赖下 `go build` / `go vet ./...` / `go test ./...` 与 R151 基线对比，既有失败不得增加；`node --test backend/web/*.test.cjs desktop/*.test.cjs`。写 `docs/history/ledgers/r152-execution-ledger.md`，更新 `docs/WORKLIST-INDEX.md`；版本号与打包按用户此前要求暂缓。R150/R151 遗留的真机验收（头部对照截图、徽章统一性、侧边栏留白实测、账号条长名字、`arena_header_source` 日志）仍待 Windows 端，可与本单一起做。

## 已知风险

P1 放宽后，个别脏行会被静默跳过；靠新增的计数诊断可见。80% 门槛是经验值，若真机日志里出现 `too-many-invalid`，说明上游结构变了，先看探测再调门槛。
