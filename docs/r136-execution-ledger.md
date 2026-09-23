# R136 执行台账：0.12.19

工单：`docs/WORKLIST-R136-KR-KEY-R116-PROBE-ACCOUNT-FLICKER-LIVE-GAP-FONT.md`。基线 HEAD `6558fe85`、版本 0.12.18；本轮源码版本 0.12.19。两份用户日志保存在 `docs/r136-validation/`，未改 `docs/r116-probe-findings.md` 或 `docs/r121-position-probe-findings.md`。

## P1 韩服 Key 与安装包

- public Setup、如生成的 zip、校验和文件名统一带 `-public`；private 沿用原名。`desktop/artifact-names.cjs` 供构建收据、installer shell 和发布脚本共用，Bash/PowerShell 构建入口都按 key mode 命名；`release-build.json` 继续记录 mode。发布脚本的 public 资产名同步带后缀。
- 缺 Key 的用户错误改为「此安装包未内置 Riot API Key，韩服战绩暂不可用。」；加密及 `-ldflags` 指令只留在 `backend/riot_api.go` 开发注释。斗魂对局详情接口也按同一 sentinel 返回 503，诊断仍归类 `not-configured`，避免误判成 502 认证失败。
- `riot_overview_cost` 新增 `error_kind`，账号查询缺 Key 时为 `riot_key_missing`，其它账号错误走 `championProviderErrorKind`；原来的 `first_error_kind` 仍专指单场失败。`TestR136MissingRiotKeyIsUserFacingAndDiagnosedAtAccountLookup` 验证文案、0 场请求与事件字段。
- `AGENTS.md` 和 `CLAUDE.md` 约定：每次验证打包记录 key mode，public 产物只保留带后缀的名称。

## P2 R116 探测

- `/help?format=Full` 改成结构化遍历 `functions[]`、`events[]`、`types[]`：扫顶层 `name` 与任意深度的 `url`/`path`/`uri`，记录三组数组长度、已扫描数和元素键名并集。静态 `cherry-augments.json` 单列；命中计数与保留上限分开。缺数组、无效 JSON/元素、元素或节点上限、明细截断都撤销 `contract_read` 与否定资格。删除旧文本路径扫描分支和不再使用的函数。
- 海斗在 `InProgress`/`Reconnect` 启动每局单例后台采样器；每 10 秒读 allgamedata 与 playerlist，分别在首次 200 且 JSON/结构合法时只写一次 shape；5 分钟上限，gameflow 离开或客户端断开即停。失败尝试只保留次数与最后 `error_kind`，结束写 `live_client_mayhem_sample_summary`。错误种类为连接拒绝、超时、HTTP 状态、无效 JSON。旧的斗魂采样分支保留。
- 本轮旧日志是自定义单人局（`my_team_length=1`、`their_team_length=0`、`paused_custom`），对方阵容判据不可判；没有把它回填成 R116 结论。
- `dist/probes/r116-augment-probe.exe` 与 `r121-position-probe.exe` 是同一份 Windows amd64 测试二进制的两个命名副本。`dist/probes/README.txt` 给出 PowerShell 变量、完整命令、R121 大厅与英雄选择两次运行步骤和日志交付清单；exe 不进 Git。

## P3 收藏刷新

- `loadAccount` 后台刷新保留旧面板；相同 markup 跳过 DOM 和图片处理，变化时才替换；请求序号丢弃乱序旧响应。首次仍显示骨架；后台失败保留旧内容。
- `loadPools` 对表格和选择器使用 markup/签名比较；`loadPoolCatalog` 首次可显示加载态，后台重载保留网格、相同数据不重绘、失败保留旧数据。断连清空签名并使在途响应失效。
- `backend/web/r136.test.cjs` 以 jsdom 的 MutationObserver、图片/表格节点引用及乱序响应验证；恢复每次插骨架或取消比较的变异均被测试检出。

## P4 对局页间距

- 状态移入 `.live-toolbar` 的固定槽位，去掉内容区 40px 空占位和额外刷新按钮；`#live-content` 的 16px 顶内边距归零；斗魂提示后的推荐区不再叠加 14px。状态仍独立于 `_recommendationMarkup`，不触发阵容图标重建。
- Chromium `desktop/r136-live-gap-layout.cjs` 在 1200/960px 各跑经典、ARAM、海斗、斗魂、等待、不支持、失败、识别新局八种状态，最长状态消息有/无两种情况共 32 张截图，另有字体截图两张，保存在 `docs/r136-validation/live-gap/`。全部首块间距为 10px，页签 y 坐标差 <0.5px。
- 更新 `r90.test.cjs`（去重刷新按钮）、`r91.test.cjs`、`r94.test.cjs`、`r95.test.cjs`（状态断言改量工具栏）、`r129.test.cjs`（markup 状态位置护栏）、`champions.test.cjs`（TFT 桩函数）。静态测试确认内容区无旧占位。隔离变异：恢复 40px 内容占位、把消息移回内容区，Chromium 断言均失败；副本从基线恢复后 `diff -q` 一致。

## P5 海斗表现字号

- 分组标题/指标名 13px、数值 14px、较平均 12px、多杀标题 12px、数值 13px、底部原说明 12px；指标行最小高度 40px。`mayhem-pane` 宽度 ≤760px 单列、≤520px 多杀两列，四个七位数在 1200/960 Chromium 中均未截断。说明文字和「对比 N 位英雄」保持原样。
- `r136.test.cjs` 对七处字号逐项断言，只允许 DESIGN.md 字阶；每处改回 9px 的变异都被检出。R116-B、R128 原文断言继续通过；R128 响应式断言更新为容器宽度条件。R117 CSS 重复声明预算仍为 45，没有增加。

## P6 R130/R133 真机压测

用户 2026-09-23 在 Windows 真机按 R133 P2 步骤压测 5 分钟。原始日志 `docs/r136-validation/lol-loot-diagnostics-0923-1137.jsonl` 共 4,849 条，同一 run `0514595efe88fc36a008f2c4`，时间 03:00:14–03:37:06Z，`app_start.version=0.12.18`、指纹 `1de5cc3d1e8c`。`scripts/r133-stall-log-report.cjs` 判 `clean`：`card_image_stalled=0`、`local_request_client/failed(endpoint=image)=0`、`client_diagnostic_rejected=0`。日志无页面导航事件，具体操作及 5 分钟时长以用户口述为准。R130 §8 已补记；R133 P1/P2/P3 全完成，工单与账本归档，索引更新。

## 验证与产物

- Go 全量、vet、host 构建、installer 测试：见最终运行记录。
- Node 全量：`backend/web/*.test.cjs desktop/*.test.cjs` 共 952 项、949 通过、2 失败、1 跳过；`scripts/*.test.cjs` 共 38 项、36 通过、0 失败、2 条 PowerShell 平台跳过。R135 基线 5 条失败含 `scripts/setup-only-build.test.cjs` 的 3 条，本轮修复其缺失 fixture 后，现存基线失败仅 `desktop/diagnostics-2024.test.cjs` 的旧免责声明断言和 `desktop/overview-render.test.cjs` 的旧旗帜入口断言；它们与 R128/R123 决策冲突，未放宽或篡改。
- R100/R117 Chromium 护栏通过；Go 四组隔离变异（help 名称扫描、采样只一次、缺 Key 旧文案、丢失 `error_kind`）均使定向测试失败，恢复副本 `diff -q` 一致。
- private 安装包与探测程序的最终指纹、SHA256 见下方最终记录。

### 最终运行记录

| 检查 | 结果 |
|---|---|
| `go build -o /private/tmp/r136-backend-final ./backend` | 通过 |
| `go vet ./...` | 通过 |
| `go test ./...` | 通过（`backend` 202.793s）；构建脚本的 5 个 Go 分片也均通过 |
| `cd installer && go test ./... && go vet ./...` | 通过 |
| `go test -race ./backend -run 'TestR136Mayhem\|TestR136Augment\|TestR136MissingRiotKey\|TestArenaMatchDetailWithoutAPIKeyFailsClosed' -count=1` | 通过 |
| `node --test backend/web/*.test.cjs desktop/*.test.cjs` | 952 项：949 通过、2 条既有失败、1 跳过，无新增失败 |
| `node --test scripts/*.test.cjs` | 38 项：36 通过、0 失败、2 条平台跳过 |
| R100 / R117 Chromium | 均 PASS；R117 `unstyledFrames=0` |
| R136 Chromium | 1200/960px，八种状态×有无消息，32 组间距全为 10px，页签 y 不跳；多杀 4×7 位数不截断 |

最终私有构建由 `bash build-desktop.sh 0.12.19` 完整执行，`release-build.json.mode=private`，指纹 `fb940f4f3ce3` 与当前 `desktop/source-fingerprint.cjs` 输出一致。打包过程检查嵌入 Key 策略、打包后端一致性和运行时文件。`dist/desktop/Deep Legends Setup 0.12.19.exe`（无 `-public`）SHA256：`13e5bae8b32a4839100c9f2a97b3f6839458c84c3f716b7a43ee033af37e1ab8`；`SHA256SUMS.txt` 与实算一致。`dist/probes/r116-augment-probe.exe`、`r121-position-probe.exe` 均为 Windows x64 console PE，SHA256 同为 `de5d8f9ffdba6194b68fbf0574c7c56feb3ad6dfbe83f21c9785124bac3c4ae4`（同一测试二进制的两个命名副本）。

构建曾被仓库既有 `TestSocialFriendsNeverProbesSpectatorForInGameFriends` 偶发误报挡住一次：测试在整条 JSONL 搜索字串 `9988`，当次随机 `run_id=26ad6026a99881e817b275cf` 恰好含 `9988`。未改该范围外测试；完整重试后 5 个 Go 分片及 private 打包通过。

## 待 Windows 真机

安装 0.12.19 后，确认韩服玩家页战绩可读、账户首次进入后无闪、对局页间距与海斗表现字号；在**匹配** 5v5 海克斯大乱斗英雄选择期间运行 R116，在大厅和英雄选择分别运行 R121，进游戏后停留至少 2 分钟并导出诊断日志。完整命令见 `dist/probes/README.txt`。收到日志后再判定 R116 三项和 R121 位置结论；本轮没有声称本机验证了 `app_start.riot_key=true` 或匹配局的实际数据。
