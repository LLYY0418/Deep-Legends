# R161 执行账本

## 日志与截图定位

- 工具栏截图中“当前位置”位于中部，刷新按钮在最右。`backend/web/gameplay.css` 原来给空的状态栏保留 `270px` flex 基宽，刷新按钮还有自动左边距。
- 同一局 `game_id=8999882465`：诊断日志第 3682 行选人阶段 `my_team_length=5`、`their_team_length=5`，但敌方未公开身份，不能从选人名单推断缺席者。
- 第 4479 行进入 `InProgress` 时 gameflow 仅有 `team_one_length=3`、`team_two_length=5`；第 4498 行后端 `raw_count=8`、`players=8`；第 4502 行前端恰好渲染 3+5。缺失在上游名单，不是前端筛掉。
- 第 4537/4540 行同局本机 Live Client 稍后返回 10 人，`team_values={"CHAOS":5,"ORDER":5}`，带 Riot ID 与位置；第 4544 行页面仍只有 8 人。原前后端“完整快照”判断只要求有玩家且战绩已结算，导致不完整名单被整局缓存并停止自动刷新。

## 改动

1. 工具栏状态栏改为按内容占宽，刷新按钮去掉自动左边距；当前位置 chip 随概况区域占据剩余空间，贴近刷新按钮。
2. 排位 420/440 队列要求 5+5 才算完整；不完整时前端 5 秒重试且保持既有 8 次上限，后端短缓存 3 秒。其他模式不改。
3. 保留 Live Client 返回的 Riot ID、阵营、位置和英雄名。昵称/标签分字段缺失时，可从同一条的完整 `riotId` 精确拆出。同局完整 10 人后，先由本人 Riot ID 对齐阵营，再用本机 `/lol-summoner/v1/alias/lookup` 核实缺席者 PUUID；仅对验证成功且不在 gameflow 的人补行，后续沿用原排位与战绩读取链。阵营或身份矛盾时整批放弃补齐；别名未解析时仅保留已验证者，继续有界重试。
4. 新增 `live_roster_recovery` 诊断，仅含人数、尝试数和原因；不写 Riot ID 或 PUUID。英雄图标 ID 只在本地英雄名唯一匹配时补，不能唯一匹配就保持未知。

## 验证

- `go test ./... -count=1`：完整回归通过，后端耗时 194.213 秒；第一次跑发现 R87 禁止在 `gameplay_refresh.go` 直接比较队列数字，改用现有 `seasonQueueSoloDuo/seasonQueueFlex` 后重跑通过。最终两处解析护栏的 `TestR161` 与 R87 守卫又单独通过。
- `node --test backend/web/*.test.cjs desktop/*.test.cjs`：981 通过、1 跳过、0 失败；最终工具栏与刷新相关的 16 项针对性测试通过。
- 本机与 Windows amd64 后端按 0.12.26、源码指纹 `2fddb945e649` 完成 **public** 验证构建，未注入 Riot Key；Windows 文件名为 `/private/tmp/Deep-Legends-0.12.26-public.exe`。本机 `-self-test` 输出 0.12.26 自检通过，Windows 包源码指纹与公版密钥策略校验通过。未打 Electron 安装包。
- `git diff --check`、前端脚本语法检查通过。
- 真机边界：本机没有用户当时正在运行的 League 客户端，因此别名接口对那两名玩家的实际响应仍需新日志确认；无法凭当前去标识诊断日志断言其 PUUID。
