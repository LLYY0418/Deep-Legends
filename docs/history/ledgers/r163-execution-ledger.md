# R163 执行账本

## 日志证据

文件：`/Users/ly/Downloads/lol-loot-diagnostics-0926-0105.jsonl`。所引时间为日志 UTC。只记录匿名人数和队伍形状，不还原玩家身份。

| 时间 | 行号 | 事件与结论 |
|---|---:|---|
| 16:46:05 | 15805 | `match_mode_classified`：`queue_id=2400`、`game_mode=KIWI`、`map_id=12`，对应最新海斗局。 |
| 16:47:31 | 16391 | `live_roster_shape`：游戏 `9000343139` 的 Gameflow 名单仅 9 人，蓝方 5、红方 4，未补人。 |
| 16:47:41 | 16422–16423 | Live Client `allPlayers` 和 `playerlist` 都有 10 人；`allPlayers` 的 Riot ID 名称与标签各有 1 个空值。 |
| 16:47:57 | 16442 | 前端收到 9 人，详情实际渲染蓝方 5、红方 4。 |

日志只有字段形状，没有缺席玩家的明文身份及逐人的队伍对应，因此不能单凭日志证明空 Riot ID 的席位就是页面缺席的那人。运行时须逐项核实阵营和缺口后才显示匿名卡片。

## 根因与修复

R161 的 10 人名单恢复、短缓存、前端快速重试只覆盖 420/440 排位队列，2400 海斗的 9 人快照因此可能被当成完整并停止更新。Live Client 海斗位置是 OTHER，即使完整 10 人也可能没有位置映射；旧探测成功条件会忽略该名单。已有补人流程还只接受有 Riot ID 并可查得 PUUID 的席位。

本轮将已知 5v5 海斗队列纳入完整性与重试判断；完整 Live Client playerlist 可参与补人。缺口侧的实名席位仍须通过本机别名接口验证 PUUID；仅当剩余缺口与缺口侧匿名席位数恰好相等、无实名解析失败、己方阵营和双方各 5 人均证实时，添加不含 PUUID 的隐藏玩家卡片。可唯一匹配的英雄名用于卡片图标，身份与战绩仍为空。恢复诊断增加 `queue_id`，不写入玩家标识。

## 验证

- `go test ./backend -run 'TestR16(1|3)' -count=1`：最终匿名英雄图标改动后通过。`go test ./...`：该小改动前全量通过，后端 194.435 秒；最后改动只在恢复函数的英雄 ID 匹配和对应测试，已由定向测试覆盖。
- `node --test backend/web/*.test.cjs desktop/*.test.cjs`：985 项，984 通过、1 跳过、0 失败。`node --check backend/web/gameplay.js` 与 `git diff --check` 通过。
- 本机及 Windows amd64 后端均以版本 0.12.28、指纹 `b3f25fca66f1` 构建并验证；产物分别为 `/private/tmp/Deep-Legends-0.12.28-public` 与 `/private/tmp/Deep-Legends-0.12.28-public.exe`。**Key mode：public**，两者均通过嵌入 Riot Key 策略检查。未打 Electron 安装包。
- Windows 真机仍需用新版本和下一份日志复核；旧日志无法显示修复效果。
