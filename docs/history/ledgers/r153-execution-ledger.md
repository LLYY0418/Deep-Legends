# R153 执行账本

日期：2026-09-25。基线：Deep Legends 0.12.19；R149–R152 的既存工作树改动保留。工单：`docs/history/worklists/WORKLIST-R153-R116-AUGMENT-PROBE-BACKFILL-AND-NODE-SCAN-LIMIT.md`。

后续状态：R155 已归档第二轮探测、按实践结论收尾判据一，并删除本账本提到的临时 Go 探针与 Windows exe；下文“待真机”和构建产物为 R153 当时的历史记录。首轮原件现名为 `docs/r116-validation/augment-contract-probe-round1-0925.jsonl`，第二轮原件及结论见 `docs/history/ledgers/r155-execution-ledger.md`。

## P1：判据一与探测器节点预算

- 已将三份用户原始文件按字节归档至 `docs/r116-validation/`，来源、SHA-256 与编码见该目录 README。首轮 `openapi-v3`、`openapi-v2` 均为 404；`help-full` 为 200、3,032,902 B、21 条命中，但 `scan_limit_reached=true`、`contract_read=false`、`negative_conclusive=false`。顶层数组 749/1468/3578 项全数计数，递归节点未扫完。21 条均为外观、TFT 或库存类名称；**本轮判据一未完成，P1-1 保持未判定**。
- `augmentProbeHelpDocument` 跳过 `values` 子数组，保留 `name` 与 `fields` 扫描；`functions`、`events`、`types` 各有独立的 100,000 节点预算。help-full 事件新增每组 `*_nodes_visited` 与 `*_limit_reached`，总体 `contract_read` 仍由完整性护栏决定。
- 夹具按本轮真机顶层规模构造 749/1468/3578 项、约 3 MB，types 每项带 48 个枚举 `values`，在首尾 `fields` 安放 `url`/`path`。定向测试证明两个端点均命中、`values` 不耗节点、`Complete=true`；另测 functions 节点耗尽不影响 events/types 的计数与命中。既有 `TestR116AugmentContractProbeMutation` 等用例通过。
- **待真机：**使用新 `dist/probes/r116-augment-probe.exe`，客户端已登录即可重跑，命令见 `dist/probes/README.txt`。新输出必须逐条人工判读；只有 `contract_read=true` 且排除静态目录/无关命中后确无候选端点，才能在 findings §2.4 写确证结论。探测源码和测试本轮保留。

## P2：判据二与 `skinName`

- 两局 KIWI（`game_id=8999110339`、`8999150286`）的 playerlist 均 HTTP 200、10 人、`ungrouped`，19 个顶层键与斗魂基线一致，含 `items`。allgamedata 中非空 `items[]` 均含 `itemID`、`slot`、`count` 等 9 键。判据二为**可行**，P1-4 保留在 R116-D。
- 两局 allgamedata 的 `$.allPlayers` 比 playerlist 均少 `skinName`、多 `<unknown-key>`；未知键的形状均为单个非空字符串。`arenaShapeKey` 的 19 个已知玩家键里只有 `skinName` 漏收。已补入安全字段表，测试核对字面量 `skinName`、没有 `<unknown-key>`、其他 18 键仍在表内、动态未知键仍被遮蔽。
- **待真机：**下一次包含 R153 代码的应用构建打海斗时，复核 `$.allPlayers.element_keys` 中出现 `skinName` 且 `<unknown-key>` 消失；目前 findings 标为“已定位，待复核”。

## P3：判据三与阶段白名单

- 两次 `ChampSelect:KIWI:2400` 的 `their_team_length=5`，但 `their_team_nonzero_counts.championId=0`；其他非零键仅说明位置/状态存在。两局进入 `InProgress` 后双方各 5 人且 `championId` 非零计数均为 5。判据三为**`==0`（看不到对方英雄）**，P1-2/P1-3 维持 InProgress/Reconnect 展示，选人阶段不展示。
- 只改 `gameplayRosterMatchupPhases` 上方注释，完整写出 `their_team_length > 0` **且** `championId` 非零计数 > 0 两条件；常量仍为 `[]string{"InProgress", "Reconnect"}`。findings §4.2–§4.4、§5 已同步回填两局原始字段。

## P4：探测命令笔误

- `probe-run.log` 为 UTF-16LE PowerShell 输出，记录一次 `flag provided but not defined: -test`。同日稍后的正确调用已产出首轮 `augment-contract-probe-round1-0925.jsonl`（R155 归档名）；不改探测代码。当时的 README 将命令整理为可整段复制的 `-test.run=...` 形式，R155 已清理该探针说明。

## 验证与边界

- 定向测试：`TestR153*`、既有 `TestR116AugmentContractProbeMutation`、`TestR116AugmentContractProbeUnreadableContractIsNotConclusive` 与 `TestR136AugmentHelp*` 均通过。
- 对抗变异均在 `/private/tmp` 的隔离副本进行：恢复全局节点预算并递归 `values` 后，首尾命中/完整性及组独立性测试 FAIL；移除 `skinName` 后，形状测试 FAIL。每次用基线**整文件拷回**，`diff -q` 确认一致，未用 `git checkout --`。
- `go build -o /private/tmp/deep-legends-r153 ./backend`：通过，最终产物 SHA-256 `8e249a6a8f3fe75a3345cd91241e0294493d0a38a484de277af95eed77ab19cc`；`go vet ./...`：通过。Windows `GOOS=windows GOARCH=amd64 go test -c -o dist/probes/r116-augment-probe.exe ./backend`：通过，最终 x64 PE SHA-256 `ea2b30a6ffa0ed97c5cae3537de60322f216e853b37bb8bc9a8ee2fc039c65c7`。
- `node --test backend/web/*.test.cjs desktop/*.test.cjs`：967 项，966 通过、1 跳过、0 失败，与 R152 基线一致。最终 `go test ./... -count=1`：通过，backend 198.954 秒（R152 基线为 203.446 秒，既有失败未增加；时长不作为性能基准）。
- 本机没有 Windows/League 客户端，不能自行补新探测结论或 `skinName` 真机复核。没有改版本号或打安装包；`desktop/package.json` 当前版本为 0.12.19。

## R212 收口（2026-10-04）

已关闭：用户确认（R212）。

依 R212 P1 用户确认关闭；不以自动测试替代历史真机验收事实。

来源：[R212 工单](../../WORKLIST-R212-CLOSE-VERIFIED-WORKLISTS-UPGRADE-INSTALL-GAP-AND-RELAY-FAILURE-LABELS.md)；[匿名日志证据](../reports/r212/user-log-evidence.json)（原日志行号及 SHA256）。以上为本次收口结论，早先的“待验收”记录保留其当时语境。
