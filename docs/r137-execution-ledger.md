# R137 执行台账：短数字哨兵与过期 Node 断言

工单：`docs/WORKLIST-R137-FLAKY-SENTINEL-AND-STALE-NODE-TESTS.md`。基线为 R136 工作区，产品版本保持 0.12.19；本轮只改测试，不改产品行为或安装包。

## P0 R136 分批提交

提交前逐批检查 `git diff --cached --numstat`、文件名和 `git diff --cached --check`；没有暂存 `dist/`、探测 exe 或大于 1 MB 的二进制。用户提供的 2.5 MB JSONL 是文本证据，34 张 PNG 单张均远小于 1 MB。

| 提交 | 内容 |
|---|---|
| `1b5aa72a` | R136 后端 Key 诊断与海斗探测 |
| `71cabc68` | R136 收藏刷新、对局布局与网页测试 |
| `ecda1d80` | R136 public 产物命名、版本和构建验证 |
| `c16b92d3` | R136 文档、真机日志、Chromium 截图和 R133 归档 |

## P1 短哨兵误报

全仓扫描了测试里 `readDiagnosticLog` 后对整段文本做 `strings.Contains` 的泄漏检查，也检查了同类 `json.Marshal` 后的诊断对象检查。以下是需要处理的短纯数字项；生产代码没有改动。

| 位置 | 旧值 → 新值 | 确定性复现或边界 |
|---|---|---|
| `backend/social_test.go:68,108` | `gameTag` `4321` → `ZQ7XT`；`summonerId` `9988` → `987654321013` | 固定 `run_id=26ad6026a998814321b275cf`，两个旧数字都在 run_id 中；Go overlay 把断言恢复成旧值时 FAIL。首个社交响应测试的桩值同步。 |
| `backend/ranked_split_probe_test.go:30,35` | `summonerID` `123456` → `987654321013` | 固定 `run_id=abc123456def0123456789ab`；旧断言在 Go overlay 下 FAIL。 |
| `backend/champion_network_test.go:41,83` | 无对应夹具的旧检查 `2031` → 实际图标桩 `drop_zq7xk` | 固定 `run_id=abc2031def0123456789abcd`；恢复 `2031` 子串检查后 FAIL。图标请求、回退候选和泄漏检查使用同一哨兵。 |
| `backend/r127_test.go:513,534` | 图标 ID `1234` → `ZQ7XK` | 此处检查的是 `collector` 直接得到的事件 map，不含随机 run_id 或时间戳；改成非十六进制哨兵以防以后改变记录路径，定向测试通过。 |
| `backend/r72_test.go:277,291` | Input.ini 值 `9999` → `ZQ7XK` | 此处检查的是同步返回的诊断 map，不含随机 run_id 或时间戳；改成非十六进制哨兵，保留文件不变与字段筛选断言，定向测试通过。 |

`backend/r56_test.go` 的 `1234` 是枚举结果检查，不是诊断日志泄漏检查；`backend/diagnostics_2351_test.go` 的 `"champion_id":5` 是正向存在断言。其它扫描出的泄漏值为文字、长 ID 或已使用非十六进制哨兵，无需替换。没有引入重试、额外等待或跳过。

R137 指定的 `go test -race -count=200 -run TestSocialFriendsNeverProbesSpectatorForInGameFriends ./backend` 全绿。Go overlay 变异只读取 `/private/tmp/r137-mutations/` 中的测试副本，原文件未改；`4321`、`9988` 两个断言分别恢复时各自 FAIL，`123456` 和 `2031` 的旧断言也分别 FAIL。

## P2 更新旧前端断言

- `desktop/diagnostics-2024.test.cjs` 原文 `assert.match(render(), /不代表抽取、刷新或保底概率/)`，改为 `assert.doesNotMatch(render(), /不代表抽取、刷新或保底概率/)`。四次「次选择」和读取失败时空串的断言未动。依据 `docs/WORKLIST-R128-MAYHEM-DETAIL-OVERVIEW-MODULES-AND-AUGMENT-HEADER.md` §2.3-B：全服品质分布副标题已删除。
- `desktop/overview-render.test.cjs` 原文 `assert.ok(w.document.querySelector(".facade-left [data-facade-banners]"), "R101 保留旗帜只读入口")`，改为检查 `[data-facade-browse="icons"]`、`[data-facade-browse="banners"]` 存在，并新增点击两个入口后的收藏 section、`facade-collection` 子页、面板及对应视图 `aria-selected` 断言。测试 harness 加载页面已有的 `favorites-facade.js`。依据 `docs/history/worklists/WORKLIST-R123-FACADE-ICON-BANNER-COLLECTION-MIGRATION.md` P1-5 和 `docs/history/ledgers/r123-execution-ledger.md` §0。
- Node 隔离变异只通过 `R137_CHAMPIONS_SOURCE` / `R137_SUITE_SOURCE` 读取 `/private/tmp/r137-mutations/` 的源码副本：往品质面板加回旧免责句、撤掉旗帜入口属性，两条定向测试分别 FAIL；正常源码的两条测试 PASS。

## 最终验证

| 检查 | 结果 |
|---|---|
| Go 定向测试：社交、ranked split、R127、R72、CommunityDragon | PASS |
| Go 社交 race × 200 | PASS（6.521s） |
| `go build -o /private/tmp/r137-backend ./backend` | PASS；产品版本仍为 0.12.19 |
| `go vet ./...` | PASS |
| 最终源码连续两次 `go test -count=1 ./...` | 两轮均 PASS（201.203s、200.514s） |
| installer `go test ./...`、`go vet ./...` | PASS |
| Node 全量 `node --test backend/web/*.test.cjs desktop/*.test.cjs` | 953 项：952 PASS、0 FAIL、1 条既有跳过（279.5s） |
| R100 / R117 真 Chromium | PASS；R117 `unstyledFrames=0` |

本轮无需递增版本或重新打包；R136 私有 0.12.19 安装包仍是交付版本。
