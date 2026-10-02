# R164 执行账本

## 新日志核对

文件：`/Users/ly/Downloads/lol-loot-diagnostics-0926-0214.jsonl`。时间为日志 UTC；不从字段形状推断玩家身份。

| 游戏 ID | 行号 | 游戏中与重连证据 |
|---|---|---|
| `9000389312` | 11623、11798、13019、13023 | 游戏中和重连均为后端 10 人、前端蓝 5 红 5；重复引用 0、空引用 0、补位 0。 |
| `9000414436` | 13588、13729、14573、14577 | 同样为 10 人、蓝 5 红 5；重复引用 0、空引用 0、补位 0。 |

第二局 Live Client playerlist 曾一次暂不可用（13572），随后成功返回 10 人（13629）；游戏流程名单始终完整。两局均未触发 `live_roster_recovery`。选人阶段两局都有 5 个对方空引用（如 11017、13254），进入游戏后消失。日志能证明详情人数正常，不能逐人证明哪位开启了隐藏战绩，因为旧版 `live_roster_rendered` 不记录隐藏标记；本轮仅增加汇总数量 `hidden_rendered` 供新日志核对。

## 改动

1. 实时详情根据 `player.hidden === true` 显示“隐藏战绩”标签，沿用现有标签样式。
2. 召唤师峡谷继续使用既有位置排序；其他非斗魂模式按队伍分别调用已有的组队聚拢逻辑。该逻辑要求组队编号与成员数完整，来源可为客户端直接信息或历史共同对局推测；缺资料不聚拢。
3. `live_roster_rendered` 增加仅含数量的 `hidden_rendered`，用于区分“已显示 10 人”与“其中有隐藏玩家”。

## 验证

- `go test ./...`：通过（后端 194.060 秒）。`node --test backend/web/*.test.cjs desktop/*.test.cjs`：987 项，986 通过、1 跳过、0 失败。新增加的组队排序、峡谷位置顺序、隐藏标签与诊断聚合测试通过；`node --check backend/web/gameplay.js`、`git diff --check` 通过。
- 本机与 Windows amd64 后端验证构建版本均为 0.12.29，源码指纹 `148917ed3f8e` 均通过校验。产物为 `/private/tmp/Deep-Legends-0.12.29-public` 与 `/private/tmp/Deep-Legends-0.12.29-public.exe`。**Key mode：public**；两者均通过嵌入 Riot Key 策略检查。未打 Electron 安装包。
- 真实 Windows 客户端中的具体卡片顺序、隐藏标签仍待截图验收。
