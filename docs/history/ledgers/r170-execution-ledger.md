# R170 执行账本

日期：2026-09-26。基线源码版本 0.12.35；本轮源码版本 0.12.36。

## 代码核对与修复

工单指出的统计范围差异成立：`modeStats` 原先直接聚合实时读取的最多 30 条混合队列记录，逐局 `recentGames` 则只取最新最多 10 条当前队列记录。排位卡的「近 N 局」文案实际已由 `recentRankedRecord` 从逐局记录计算，因此 N 本身已封顶；但排位卡的 KDA，以及其他模式的胜负、胜率、KDA，仍会使用 30 条窗口的统计值。

新增共用的最近对局筛选：按 `CreatedAt` 降序稳定排序，排除非胜负场次，按当前队列及玩家身份过滤，最多保留 10 场。实时 `modeStats` 与 `recentGames` 均从这批对局生成；排位 `recentRankedRecord` 继续从 `recentGames` 生成。汇总仍由原有 `aggregateMatches` 计算，保持时长、CS、KDA 等完整字段口径。LCU/SGP 的一次性混合队列读取窗口仍是 30，没有增加请求或逐场详情读取。

## 验证与交付边界

Go 定向测试构造 30 条混合队列记录：其中 15 条属于当前队列时，汇总与逐局记录均只用最新 10 条，胜负、KDA、CS 和每分钟 CS 按这 10 条计算；只有 3 条同队列时，两处均保留 3 条。R168 混合队列窗口测试同时核对更早的同队列场次仍能被找到，并且汇总场次等于逐局条数。`go test ./backend -count=1` 全套通过（194.349 秒）。前端已有测试核对详情最多渲染 10 条，本轮前端全套 `node --test backend/web/*.test.cjs` 734/734 通过。`git diff --check` 通过。

公开模式（**key mode: public**，`main.riotAPIKey` / `main.riotAPIKeyCipher` 均为空）构建 macOS arm64 `/private/tmp/Deep-Legends-backend-0.12.36-public` 与 Windows amd64 `/private/tmp/Deep-Legends-backend-0.12.36-public.exe`；两者均通过源码指纹 `907cca74ef48` 校验，macOS `-self-test` 输出 0.12.36 并通过。仅保留带 `-public` 后缀的验证产物，未制作或发布安装包。Windows 真机页面表现与延迟仍待实机复核。

## 打包测试误报补记

用户运行 `./build-desktop.sh` 时，R127 隐私测试把纯数字图片 ID `4379` 当作整条日志的禁用子串；该次诊断时间戳的毫秒部分碰巧包含 `4379`，导致五分片 Go 测试误报。R130 有同样的断言形状，一并将测试输入与断言改成带文字的私密路径标记。两项测试各重复 100 次通过；按打包脚本使用的 `node ../scripts/go-test-shards.cjs` 完整运行 1566 项测试，五个分片全部通过。后续的 `go vet ./...`、installer `go test -vet=off ./...` 与 `go vet ./...` 均通过。此次仅修改测试，源码指纹仍为 `907cca74ef48`，版本仍为 0.12.36；未重新制作安装包。
