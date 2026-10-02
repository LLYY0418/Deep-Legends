# WORKLIST-R193：隐藏身份玩家卡片：不显示"未定级"，"客户端未公开该玩家"只出现一次

诊断人：Claude（看截图 + 读日志 + 只读核对源码）。执行人：GPT。日期：2026-10-02。基线：当前工作区（R192 之后；如 R192 尚未执行，以 0.12.55 为准，两者不冲突）。

## 结论：补人逻辑是正常的，问题只在卡片显示

用户截图：灵活组排进行中，对方打野是「隐藏玩家（隐藏身份）」，有英雄头像，`打野 · 未定级`，统计区写「客户端未公开该玩家」，卡片底部又写一次「客户端未公开该玩家」。

日志 `lol-loot-diagnostics-1002-2222.jsonl`（**仍是 0.12.49**），game 9011641813，queue 440：

- 12:52:21 `lcu_gameflow_session_shape`：team_one 只有 **4** 人（截图里对方少的就是这一个）。gameflow 本身就没给这个玩家。
- 12:52:38 `live_client_playerlist_shape`：Live Client 10 人，`riotId` 只有 **9** 人有。
- 12:52:38 `live_roster_recovery reason=anonymous-placeholder appended=1 anonymous_count=1 named_present=4`。
- 12:52:38 `live_roster_rendered rendered_100=5 rendered_200=5 hidden_identity_rendered=1`。

这和 R183 设计的结果完全一致。这个玩家在客户端里开启了隐藏身份：gameflow 不给他的记录，Live Client 不给他的 Riot ID，软件拿不到他的身份，也**不应该**去绕过。R175 定下的匿名安全路径（不查战绩、段位、身份）保持不变。

需要改的只是卡片上两处显示：

1. **`未定级` 是错的。** `gameplay.js` 约 5798 行 `rankCopy = rank?.tier ? rankTitle(rank) : "未定级"`。隐藏玩家根本没查段位，不知道他是不是定级了，写"未定级"等于给出一个错误信息。
2. **同一句话出现两次。** 统计区用 5804 行的 `emptySummary`，卡片底部的战绩行又由 6287 行输出同一句「客户端未公开该玩家」。

## 修改

1. `player.hidden === true` 的卡片：副标题只显示位置（例如 `打野`），不显示段位文字。没有位置时副标题整行不显示。
2. 同一张卡片上「客户端未公开该玩家」只出现一次：保留统计区那一处（5804 行），底部战绩行（6287 行）对 `hidden === true` 的玩家不输出任何内容（整行不渲染，不留空白高度）。
3. 隐藏玩家统计区的胜率、KDA 两列在没有数据时不显示 `—`，整列省略。只保留"当前模式：客户端未公开该玩家"这一块。
4. `identityUnresolved`（身份待公开）和 `privateHistory`（隐藏战绩）两种情况的现有显示不变。普通玩家"暂无样本"的显示不变。
5. 不新增任何文字。

## 测试（Node）

1. `hidden: true`、`position: "jungle"`、无 rank：副标题为 `打野`，不含「未定级」。
2. 同一张卡片里「客户端未公开该玩家」出现次数为 1。
3. 隐藏玩家卡片不含 `—`。
4. `identityUnresolved: true` 的卡片仍显示「身份尚未公开」，与改前一致。
5. 普通无段位玩家（非 hidden）仍显示「未定级」。

变异：副标题恢复 `rankCopy` → 测试 1 FAIL；底部行恢复输出 → 测试 2 FAIL。

`node --test backend/web/*.test.cjs desktop/*.test.cjs`、`go test ./backend -count=1`、`git diff --check` 全绿。

## 收尾

- 版本递增；`docs/WORKLIST-INDEX.md` 加 R193；账本 `docs/history/ledgers/r193-execution-ledger.md`，附改前/改后卡片截图（演示数据）。

## 真机验收（用户）

再遇到隐藏身份的玩家时：卡片显示英雄头像、`隐藏玩家`、位置；不显示"未定级"；「客户端未公开该玩家」只出现一次。
