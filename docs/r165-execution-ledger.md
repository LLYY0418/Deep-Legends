# R165 执行账本

## P0 本地数据服务退出（仍待定位）

用户截图显示 Electron 已打开主窗口，本地后端进入 ready 后退出。当前没有这次 Windows 运行的 `desktop.log`、退出码或 Go panic 堆栈；截图不能确定是哪一行改动触发。此前 0.12.29 的 Windows 构建校验与 macOS 后端启动验证不能替代 Windows 安装包冷启动。本轮按用户指示先做 P1/P2，**不将 P0 标为修复，不发布新的 Windows 安装包**。

待办：收到 `%APPDATA%\Deep Legends\logs\desktop.log`（含滚动日志）或在 Windows 真机复现后，定位崩溃点与条件，在隔离副本做崩溃变异，然后完成 Windows 安装包冷启动到总览页的真实验收。

## P1 R121 回填与直接补位字段

- `position-lobby.jsonl` 和 `position-champselect.jsonl` 已逐字节复制到 `docs/r121-validation/`；SHA-256 分别为 `f46b0411ce2fa5933bd3a549791f3e42e8a696536ab47086e5d3e88ad221206d`、`76b3673c6c25cd3e9130016e466aa443d2e3907488b6f68863c3e17481ea894a`，与用户上传件相同。两次都是 `verdict=found`，但文件 A 也读到完整选人阵容，不能视为进选人前快照；探测只记键名和类型，对方字段取值未知；两个 Swagger 来源 404。findings §1 已按真实输出回填，R121 索引已关闭。
- 实时选人只从 `myTeam[].isAutofilled` 读取直接补位事实；`theirTeam` 仅产生匿名汇总计数。选人时用本局 GameID 和会话匿名引用把己方 `true` 留在内存，InProgress/Reconnect 沿用，换局或离开实时阶段清空，不落盘。历史战绩的 `autofillLabelGate` 继续关闭。
- 新诊断 `champ_select_autofill_shape` 只记录己方人数、己方 true 数、对方 true 数、对方分配位置非空数。没有玩家标识或位置明细。
- R121 一次性 Go 探针与测试、Windows 探针和运行脚本已删除；`dist/probes/README.txt` 只保留 R153 `skinName` 真机复核。

## P2 隐藏标签语义

- 实时后端把召唤师 `Privacy=PRIVATE` 写入 `privateHistory`；匿名席位没有该属性，仍为 false。
- 实时详情独立显示“隐藏战绩”（只认 `privateHistory`）和“隐藏身份”（只认 `hidden`），两者可同时出现；总览和覆盖层的“隐藏战绩”也只认 `privateHistory`，移除旧解释 tooltip。
- `live_roster_rendered` 将旧的 `hidden_rendered` 拆为 `hidden_identity_rendered` 与 `private_history_rendered` 两个汇总计数。

## 本机验证

- Go 定向测试：通过。`go test ./...`：通过。`go vet ./...`：通过。
- 前端定向测试：通过。首次 Node 全量为 987 项，985 通过、1 跳过、1 失败：新增 CSS 覆盖规则违反 R119 对“补位样式只留一条规则”的既有护栏；多余规则已删除。全量复测：987 项，986 通过、1 跳过、0 失败。
- 对抗变异在 `/private/tmp` 的独立源码副本执行：错读 `theirTeam`、进入游戏丢失补位值、换局不清空、不设置 `PrivateHistory`、前端退回只看 `hidden`、对方分配位置计数强制为零、后端不写私密战绩汇总、前端用身份隐藏数冒充私密战绩数，8 项均对应测试失败；每次用整文件拷回，并以 `diff -q` 确认还原。最后一项测试使用 1 个隐藏身份、2 个隐藏战绩的不同计数，避免同数误过。
- `go test -race ./backend`：通过。`go build` 已生成 macOS 与 Windows amd64 的 0.12.30 public 后端验证二进制（非安装包），指纹 `c4186a398af1` 均通过校验，嵌入 Riot Key 策略校验均为 **public**。本机同源后端冷启动返回 `LOOT_READY`，15 秒后仍运行；这些验证不替代 P0 所要求的 Windows 安装包冷启动。
- 真机待办：单双排/灵活选人阶段对照客户端官方补位图标，确认仅相应己方卡片有“补位”，游戏中仍在，并导出四项计数；另找开启隐藏战绩及隐藏身份席位，分别核对两种标签与诊断计数。P0 的 Windows 冷启动验证单独保留。
