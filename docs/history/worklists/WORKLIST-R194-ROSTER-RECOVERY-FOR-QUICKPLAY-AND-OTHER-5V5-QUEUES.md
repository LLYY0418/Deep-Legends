# WORKLIST-R194：快速模式对局详情少一个人（补人逻辑只覆盖排位和海斗）

诊断人：Claude（看截图 + 读日志 + 只读核对源码）。执行人：GPT。日期：2026-10-02。基线：当前工作区（R193 之后；R192/R193 未执行时以 0.12.55 为准，互不冲突）。

## 现象

用户截图：**快速模式**进行中，详情页我方只有 4 人（打野、中路、下路、辅助），**上路整个没有**。对方 5 人正常。

## 证据

日志 `lol-loot-diagnostics-1002-2324.jsonl`（**仍是 0.12.49**），game **9011928493**，queue **480**（快速模式）：

| 时间 (UTC) | 事件 |
|---|---|
| 14:43:55 | `live_roster_shape raw_count=9 players=9 team_counts {100:4, 200:5}` |
| 14:43:55 | `live_roster_rendered rendered_100=4 rendered_200=5` |
| 14:4x | `live_client_playerlist_shape player_count=10`，ORDER 5 / CHAOS 5，五个位置各 2，`position_match_source_counts.riotId=9` |
| 15:06:12 | Reconnect 后仍是 `players=9 {100:4, 200:5}` |

和 R183 那局排位完全同一种情况：gameflow 少给一个人，Live Client 10 人里这个人没有 Riot ID（隐藏身份）。区别是：**整局没有出现任何一条 `live_roster_recovery`**，补人逻辑根本没有运行。

## 根因（源码已核对）

`backend/live_roster_recovery.go` 第 26 行：

```go
func liveTenPlayerRosterQueue(queueID int64) bool {
	if queueID == seasonQueueSoloDuo || queueID == seasonQueueFlex {
		return true
	}
	definition, ok := supportedQueueDefinition(queueID)
	return ok && definition.ModeGroup == "hextech-aram"
}
```

只有单双排 420、灵活组排 440 和海克斯大乱斗算"10 人对局"。`gameplay.go` 约 7674 行的补人入口、`gameplay_refresh.go` 151 行的"数据不完整就继续刷新"、324 行，以及 `liveAnonymousRosterQueue`，都用这个函数判断。快速模式 480 返回 false，所以既不补人，也不判定"不完整"，页头也没有提示，静默少一个人。

同样会漏掉的还有：匹配 400/430/490、极地大乱斗 450/3220/930、冠军杯 700/720、无限火力 900/1900。

## 修改

1. `liveTenPlayerRosterQueue` 改为按 `queue_groups.go` 的 `ModeGroup` 判断，以下分组返回 true：`solo`、`flex`、`match`、`aram`、`hextech-aram`、`clash`、`urf`。
   - **不包含**：`bots`、`doombots`（人机局电脑玩家没有 Riot ID，会被误当成匿名玩家）、`arena`（16 人，走自己的分组逻辑）、`custom`、`other`、`nexus-blitz`，以及未登记的队列。
   - 420/440 的特判保留，避免队列表加载顺序问题。
2. `liveAnonymousRosterQueue` 继续等于 `liveTenPlayerRosterQueue`。R183 的其余校验全部不变：本人身份和阵营核验通过；Live Client 两队各 5 人；有 Riot ID 的成员都查询成功；匿名人数正好等于缺的人数。
3. 额外保险：Live Client 里 `isBot=true` 的条目不计入匿名人数（防止以后队列表把人机局归错组）。
4. 补入的玩家走 R175/R183 的匿名安全路径：显示英雄、位置、「隐藏玩家」，不查战绩、段位、身份，不参与组队推断。如果 R193 已执行，卡片按 R193 的规则显示。
5. 诊断：`live_roster_recovery` 已有 `queue_id`，不加新字段。补人入口因队列不支持而跳过时，**同一局记一次** `live_roster_recovery reason=queue-unsupported`，以后再遇到别的模式能直接从日志看出来。

## 测试（Go）

1. **复现本局**：queue 480，gameflow 我方 4、对方 5；Live Client 10 人，我方 4 个有 Riot ID 且都在名单中，1 个匿名（TOP）→ 补 1 个，`reason=anonymous-placeholder`，两队各 5，占位玩家位置为上路。
2. 表驱动：400、430、450、480、490、700、720、900、1900、2400 都返回 true；820、880、890、950、1700、3100、4210、未知 ID 都返回 false。
3. 人机局 880：Live Client 有 `isBot=true` 的电脑玩家、gameflow 缺人 → 不补，`reason=queue-unsupported` 同一局只记一次。
4. queue 480 下 `gameplayLiveSnapshotComplete` 在 9 人时为 false（会继续刷新），补齐后为 true。
5. R183 原有 440 / 420 用例全部保持通过。

Node：

6. 用测试 1 的返回体渲染：我方 5 张卡，占位卡在上路位置。

变异（overlay，必须断言 FAIL）：

| 变异 | 期望 |
|---|---|
| `liveTenPlayerRosterQueue` 恢复为只认 420/440/海斗 | 测试 1、2 FAIL |
| 把 `bots` 加进允许分组 | 测试 2、3 FAIL |
| 不排除 `isBot` 条目 | 测试 3 FAIL |

`node --test backend/web/*.test.cjs desktop/*.test.cjs`、`go test ./backend -count=1`、`go vet ./backend`、`git diff --check` 全绿。

## 收尾

- 版本递增；`docs/WORKLIST-INDEX.md` 加 R194，R183 那行备注"匹配、快速、大乱斗等 5v5 模式的补人由 R194 放开"；账本 `docs/history/ledgers/r194-execution-ledger.md`。

## 真机验收（用户）

1. 安装新版本后打快速模式、匹配或极地大乱斗；遇到隐藏身份的玩家时，两队都是 5 人，缺的那位显示「隐藏玩家」、英雄头像和位置。
2. 导出日志：这局有 `live_roster_recovery reason=anonymous-placeholder appended=1`。
