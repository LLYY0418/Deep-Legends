# WORKLIST-R183：排位对局详情少一个人（敌方匿名玩家没有补进名单）

诊断人：Claude（读日志 + 只读核对源码）。执行人：GPT。日期：2026-10-02。基线：源码 0.12.47（R182 之后，指纹 a2e718b631f9）。

## 现象

用户截图：灵活组排进行中，详情页我方 5 人、**对方只有 4 人**（缺中路）。页头显示"数据尚未完整，已停止自动重试，可手动刷新"。

## 证据

日志 `lol-loot-diagnostics-1002-0119.jsonl`，0.12.47 运行，game **9010346582**，queue **440**。软件在 16:30:09Z 对局进行中启动（刚装好新版）。

1. `lcu_gameflow_session_shape`（16:30:09）：`team_one_length=4`、`team_two_length=5`。本人在 team two（打野）。**gameflow 会话里敌方只有 4 人**，第 5 人整条记录都没有。
2. `live_client_probe_timing` / `live_client_playerlist_shape`：Live Client 名单 **10 人**，ORDER 5 / CHAOS 5，位置 BOTTOM/JUNGLE/MIDDLE/TOP/UTILITY 各 2；但 `position_match_source_counts.riotId=9`，**只有 9 人有 Riot ID**。
3. `live_roster_recovery`（R161/R163/R175 的补人逻辑，这是它第一次在排位真机上触发）：16:30:09–16:34:55 共 **10 次**，每次都是 `raw_count=9`、`playerlist_count=10`、`alias_attempts=4`、`appended=0`、`reason=partial`。
4. 同时段 `tencent_riot_id_lookup` 共 40 次（10 次 × 4），**全部 success**。
5. 最终 `live_roster_shape`：`players=9`、`team_counts {100:4, 200:5}`。

### 根因（源码已核对）

`backend/live_roster_recovery.go` `recoverClassicLiveRoster`：

- 敌方在 Live Client 里 5 人。其中 4 人有 Riot ID：逐个精确查询都成功，但他们本来就在名单里（`present`），所以没有新增。剩下 1 人**没有 Riot ID**（匿名，第 120 行进入 `anonymous`）。
- 匿名占位只在 `allowAnonymous` 为真时才补（第 162 行）。`allowAnonymous` 来自 `liveAnonymousRosterQueue`（第 29 行），**只对海克斯大乱斗返回 true**；单双排 420、灵活组排 440 都是 false。
- 结果是 `pending` 为空，返回 `partial`、`appended=0`。敌方一直只有 4 人。前端重试 8 次后显示"已停止自动重试"（`gameplay.js:181`）。

R163 当时把匿名占位限制在海斗，是因为只在海斗见过这种情况。现在排位也出现了：gameflow 直接漏掉一个隐藏身份的玩家，Live Client 里这个玩家也没有 Riot ID。

另外，`partial` 时每次刷新都把 4 个 Riot ID 重新查一遍（5 秒一次，共 40 次查询），每次结果都一样。

## P1　排位也允许匿名占位

1. `liveAnonymousRosterQueue` 改为与 `liveTenPlayerRosterQueue` 一致：420、440 和海斗都允许。**其余校验全部不变**：
   - 本人身份和阵营核验通过；
   - Live Client 两队各 5 人；
   - 所有有 Riot ID 的目标队成员都查询成功（`unresolvedNamed == 0`）；
   - 匿名人数**正好等于**缺的人数。
   任何一条不满足，仍然返回原来的原因，不补人。
2. 占位玩家带上 Live Client 里的**位置**（`SelectedPosition: anonymous[index].Position`）和**英雄**（现有 `championIDForName`）。这样排位详情页按位置排序时，这名玩家落在正确的那一路（本例为中路）。有名字的补入玩家原本就带位置，不变。
3. 占位玩家沿用 R175 的匿名安全路径：`NameVisibilityType: "HIDDEN"`、无 PUUID、不请求战绩和段位、不参与组队推断、名字不可点击。进入对局后显示现有"隐藏玩家"卡片（R179 只把选人阶段的敌方占位改成"暂无玩家信息"，对局中不变），有英雄头像和位置。不新增任何说明文字。
4. 补齐后两队各 5 人，`liveSnapshotComplete` 成立，页头不再出现"数据尚未完整"。

## P2　同一局不重复查询

`recoverClassicLiveRoster` 的结果按"游戏 ID + Live Client 名单指纹（阵营、Riot ID、匿名人数）+ gameflow 名单里的玩家集合"在本局内缓存。输入没变就直接复用上次的结果（包括补入的玩家），不再重复做 Riot ID 查询；输入变了（例如掉线玩家重连后 gameflow 补回了人）就重新计算。换局清空缓存。

## P3　诊断

`live_roster_recovery` 增加计数字段，不记身份：

- `anonymous_count`：目标队里没有 Riot ID 的人数；
- `named_present`：有 Riot ID、而且已经在名单里的人数；
- `named_appended`：有 Riot ID、由本次补进的人数；
- `unresolved_named`：有 Riot ID 但查询失败的人数；
- `cached`：是否复用了本局缓存的结果。

同一局、同一结果只记一次（与 P2 缓存一致），不再每 5 秒记一条。

## 测试

Go：

1. **复现本局**：queue 440，gameflow 我方 5、敌方 4；Live Client 10 人，敌方 4 个有 Riot ID 且都在名单中，1 个匿名（位置 MIDDLE、英雄已知）→ 补 1 个占位，`reason=anonymous-placeholder`，两队各 5，占位玩家 `position=middle`、英雄 ID 正确、无 PUUID。
2. 同样场景用 queue 420 → 结果相同。
3. 匿名人数 2、缺 1 人 → 不补，原因不变（`partial`）。
4. 有 Riot ID 的成员查询失败（`unresolvedNamed=1`）→ 不补匿名。
5. Live Client 两队人数不是 5/5，或本人阵营无法核验 → 原因保持 `teams-unverified` / `self-unverified`，不补。
6. 占位玩家不触发战绩、段位、身份请求（断言请求计数）；组队推断不包含它；R175 端到端用例在 440 下通过（不越界、不 panic）。
7. **缓存**：同一局连续 10 次加载、输入不变 → Riot ID 查询共 4 次（只在第一次），`live_roster_recovery` 只记 1 条且后续 `cached=true`；gameflow 名单变化 → 重新计算；换局 → 缓存清空。
8. 诊断字段：本局场景下 `anonymous_count=1`、`named_present=4`、`named_appended=0`、`unresolved_named=0`。

Node：

9. 用测试 1 的后端返回体渲染详情页：对方 5 张卡，占位卡显示"隐藏玩家"和英雄头像，排在中路位置；页头没有"数据尚未完整"。

变异（overlay，必须断言 FAIL）：

| 变异 | 期望 |
|---|---|
| `liveAnonymousRosterQueue` 恢复为只限海斗 | 测试 1 FAIL |
| 占位不带位置 | 测试 1（位置断言）FAIL |
| 去掉结果缓存 | 测试 7 FAIL |
| 去掉 `len(anonymous) == remaining` 校验 | 测试 3 FAIL |

`node --test backend/web/*.test.cjs`、`go test ./backend -count=1`、`go vet ./backend`、`git diff --check` 全绿。

## 收尾

- 版本递增；`docs/WORKLIST-INDEX.md` 加 R183；R175 那行备注"排位匿名补人由 R183 放开"；账本 `docs/r183-execution-ledger.md`。
- 账本写明：R175 的补人逻辑这次是第一次在排位真机上触发，此前一直未验证。
- 界面红线不变：不加说明文字、tooltip。

## 真机验收（用户）

1. 再遇到对方或我方有隐藏身份的玩家时，详情页两队都是 5 人，隐藏的那位显示"隐藏玩家"和英雄头像，位置正确。
2. 页头不再出现"数据尚未完整，已停止自动重试"。
3. 导出日志：`live_roster_recovery` 为 `reason=anonymous-placeholder`、`appended=1`，同一局只有一两条。
