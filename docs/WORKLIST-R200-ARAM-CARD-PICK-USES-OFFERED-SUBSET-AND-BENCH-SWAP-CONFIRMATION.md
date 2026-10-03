# WORKLIST-R200：大乱斗 / 海克斯大乱斗自动选用：只在发给自己的英雄卡里选；备战席换人要确认生效

诊断人：Claude（看截图 + 读日志 + 只读核对源码）。执行人：GPT。日期：2026-10-03。基线：0.12.60 工作区（R197/R198/R199 之后；未执行时以当前工作区为准，互不冲突）。

## 现象

用户截图：征召 → 大乱斗类，选用序列 1 寒冰射手、2 铸星龙王、3 巨魔之王、4 扭曲树精、5 符文法师，策略「立即锁定」，备战席自动换人已开。

- 第一局：开局发的卡片和备战席里都没有寒冰，软件一直在选寒冰；卡片里有龙王，却不去选。
- 第二局：同样一直选寒冰，备战席出现瑞兹也没换到。

用户补充需求：这类模式开局会给自己 2～3 张英雄卡，卡里有选用序列里的英雄就应该直接选。

## 证据

日志 `lol-loot-diagnostics-1003-1604.jsonl`，版本 `530db182f4c1`，两局都是 **queue 2400**（KIWI），`group_id=aram`，选用序列 `[22, 136, 48, 57, 13]`。

**第一局（07:34）**

| 时间 (UTC) | 事件 |
|---|---|
| 07:34:16 | `lcu_champ_select_session_shape`：10 个 `PICK` 行动全部进行中；`benchChampions` 长度 0；顶层有 `allowSubsetChampionPicks` |
| 07:34:17 | `champselect_candidates available_count=173`（= `pickable-champion-ids`，就是**全部已拥有英雄**）→ 选中 22（寒冰），`reason=selected` |
| 07:34:17 | `write lock` 寒冰 → `http-success`，但 `postflight observed_champion_id=0`，`not-applied-after-2s` |
| 07:34:20 | 再试一次，仍然不生效 → `confirmation attempt-limit`，`schedule attempt-limit`：**整个选人行动到此放弃**，没有试序列里的下一个（龙王 `reason=later-priority`） |
| 07:34:21～28 | 其他玩家选完，备战席从 2 涨到 10；每次 `bench-gate no-preferred-target`：备战席里没有序列里的英雄 |

**第二局（07:47）**：完全同样的过程，寒冰两次提交成功但不生效后放弃。07:47:32 FINALIZATION 时备战席出现瑞兹（13），`champselect-bench` 换瑞兹 `write-result http-success`。之后**没有任何确认是否换到的记录**，用户反馈没换到。

## 根因（源码已核对）

1. **这个模式能选的只有发给自己的那几张卡，但软件用的是全部已拥有英雄。** 选人候选取自 `champselect.go:819` 的 `/lol-champ-select/v1/pickable-champion-ids`（173 个）。会话里 `allowSubsetChampionPicks` 已经说明这局只能从子集里选，代码里没有任何地方读这个字段或子集列表（全仓库搜不到 `subset`）。所以软件挑了"已拥有、排第一"的寒冰。客户端接受了请求（HTTP 成功），但寒冰不在卡里，选不上。
2. **一个英雄选不上就整轮放弃。** `champselect_execution.go:169` 达到 2 次上限后记 `attempt-limit`，提示"已停止尝试，请手动操作"，不会换序列里的下一个英雄。所以即使卡里有龙王也不会去选。
3. **备战席换人没有生效确认。** `handleChampSelectBench` 发出 `/session/bench/swap/{id}` 后只看 HTTP 结果，没有像选人那样的 postflight 核对。瑞兹那次请求成功但实际没换到，日志里看不出原因。
4. **诊断不够。** 日志里没有发给自己的卡片 ID、备战席英雄 ID、自己当前英雄，只有数量。

## P1　卡片阶段：只在发给自己的卡里按序列选

1. **识别**：会话 `allowSubsetChampionPicks === true`（或执行时查到的等价字段）时，本局按"卡片模式"处理。
2. **读取发给自己的卡片**：没有可用的离线接口说明（仓库里没有导出文件；用户客户端的 `/swagger/v3/openapi.json`、`/swagger/v2/swagger.json` 在 10-02 日志里都是 404，swagger 未开启；公开文档 lcu.vivide.re 里也没有 subset 接口）。所以改为**运行时发现**，不阻塞实现：
   - 卡片模式下，每个选人会话第一次需要时读一次 `GET /help`（LCU 自带接口列表，不依赖 swagger），找名称或路径里含 `subset`（不区分大小写）且属于 champ-select / team-builder 的 GET 接口，记 `subset_help_matches`（只记路径名）。
   - 依次尝试：`/help` 找到的路径 → `GET /lol-lobby-team-builder/champ-select/v1/subset-champion-list` → `GET /lol-champ-select/v1/subset-champion-list`。第一个返回 200 且能解析成英雄 ID 列表（整数数组，或含 `championId` 的对象数组）的就用它，本会话内记住。每个候选记一条 `subset_probe`：路径、HTTP 状态、解析结果、ID 数量。
   - 全部失败：按第 5 条处理（不选、`subset-unavailable`）。
   - 账本写明"接口未在真机核实"，由用户下一局的日志确认实际用的是哪个。
3. **选择**：按选用序列从前往后，取**第一个在卡片列表里**的英雄。按用户设置的策略执行（仅亮出 / 亮出后锁定 / 立即锁定、锁定等待），和普通模式一样。「避让队友预选」照常生效。
4. **卡里没有序列英雄**：不提交任何选人请求，留给用户自己选；记一次 `champselect_trace stage=subset reason=no-pool-champion`。
5. **卡片列表读不到**（接口不存在或失败）：卡片模式下**不退回**用全部已拥有英雄去选（那就是这次的错误），不提交，记 `reason=subset-unavailable`。
6. **卡片会变**（重随等）：会话更新时重新读卡片列表；还没锁定时，序列里更靠前的英雄出现在卡里就改选它。
7. 非卡片模式（排位、匹配、普通大乱斗等）完全不变。

## P2　一个英雄选不上，换序列里的下一个

所有模式通用：某个英雄达到 `attempt-limit` 后，把它记为本回合不可用，按序列取下一个**仍然可选**的英雄继续（同样最多 2 次）。本回合最多换 3 个不同英雄，之后才停止并显示现有提示。用户自己手动选了别的英雄时，按现有规则停止自动选用（不变）。

## P3　备战席换人确认生效

1. 换人请求 HTTP 成功后，像选人一样做 postflight：2 秒内会话里自己的英雄应变成目标英雄。
2. 没生效：重新读会话，目标还在备战席就再试一次（最多 2 次）；不在了就放弃这个目标，按序列看备战席里有没有下一个。
3. 还没选定英雄（卡片阶段自己的选人行动未完成）时，不发换人请求，等选人完成后再判断（现有 `current` 为 0 的情况）。
4. 记录 `champselect_trace stage=bench-postflight`：`reason`（`applied` / `not-applied` / `target-gone`）、`target_id`、`observed_champion_id`、`attempt`。

## P4　诊断补全

`champselect_trace` 以下阶段加字段（英雄 ID 不是隐私，可以记）：

- `stage=source`：`subset_mode`（布尔）、`subset_ids`（发给自己的卡片 ID 列表）、`subset_source`（实际使用的接口名或 `unavailable`）。
- `stage=bench-gate`：`bench_ids`（备战席英雄 ID 列表）、`current_champion_id`。
- `champselect_candidates`：卡片模式下 `available_count` 记卡片数量，并加 `availability_source=subset`。

## 测试（Go）

1. 复现第一局：卡片模式，卡片 `[136, 75, 99]`，序列 `[22, 136, 48, 57, 13]` → 选 136，**不请求 22**；`subset_ids` 正确。
2. 卡片 `[75, 99, 101]`（没有序列英雄）→ 不发选人请求，`reason=no-pool-champion`。
3. 子集接口失败 → 不发选人请求，不使用 `pickable-champion-ids`，`reason=subset-unavailable`。
4. 卡片从 `[57, 75]` 变成 `[136, 57]`（未锁定）→ 改选 136。
5. 普通模式：22 两次不生效 → 改选 136；22、136、48 都不生效 → 停止并给出现有提示；总请求数不超过 6。
6. 备战席换 13：会话里自己仍是原英雄 → 再试一次；13 已不在备战席 → 放弃，`target-gone`；成功 → `applied`。
7. 自己选人行动未完成时，备战席有序列英雄 → 不发换人请求。
8. 排位 420 的现有选人、禁用用例全部保持通过。

Node：

9. 「本局记录」里：卡片模式选中时显示"从开局卡片中选择 铸星龙王"（复用现有记录条的格式；没有序列英雄时显示"开局卡片中没有选用序列里的英雄"）。只新增这两条记录文字。

变异（overlay，必须断言 FAIL）：

| 变异 | 期望 |
|---|---|
| 卡片模式仍用 `pickable-champion-ids` | 测试 1 FAIL |
| 子集读取失败时退回全部英雄 | 测试 3 FAIL |
| `attempt-limit` 后不换下一个 | 测试 5 FAIL |
| 换人不做 postflight | 测试 6 FAIL |

`node --test backend/web/*.test.cjs desktop/*.test.cjs`、`go test ./backend -count=1`、`go vet ./backend`、`git diff --check` 全绿。

## 收尾

- 版本：和 R197、R198 合并在 **0.12.61** 一起发布（见 R198 收尾）；如果 R197/R198 已经发布，则本工单单独递增版本并按 R199 的规则发布。
- `docs/WORKLIST-INDEX.md` 加 R200；账本 `docs/r200-execution-ledger.md`，写明本机实际使用的子集接口名。

## 真机验收（用户）

1. 打海克斯大乱斗或大乱斗：开局卡片里有选用序列中的英雄时，软件自动选它（按你设的策略锁定）；卡片里没有时，软件不动，你自己选。
2. 备战席出现序列里更靠前的英雄时，自动换过去；「本局记录」里能看到换人结果。
3. 导出日志：`stage=source` 有 `subset_mode=true` 和 `subset_ids`；换人有 `bench-postflight reason=applied`。
