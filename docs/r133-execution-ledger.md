# R133 执行台账：R130 独立验证收尾

基线 **0.12.18**（R130 之后）。工单：`docs/WORKLIST-R133-R130-VERIFICATION-FOLLOWUPS.md`（诊断人 Claude，执行人 GPT）。

三条的处置结果：

| 项 | 内容 | 结果 |
|---|---|---|
| **P1** | 看门狗注释还停在旧数值（`backend/web/app.js`） | **已完成**，验收 grep 干净 |
| **P2** | R130 验收总表要求的真机 5 分钟手动验证 | **本机做不了，仍未执行**；已把最后一步的日志判读固化成可复核的脚本 + 交接步骤，等有客户端的机器上跑 |
| **P3** | Claude 那轮没能重新编译执行 `backend/r130_test.go`（设备登录过期） | **我这边代跑了**：`go test -race ./backend` 通过 |

版本号**没有**递增，仍是 0.12.18。理由：P1 只改注释（三行），P2/P3 不改任何生产代码，新增的两个文件在 `scripts/` 下、既不被 `//go:embed web/*.js` 收录也不进安装包，运行时行为零变化。两个 0.12.18 构建靠 `build_fingerprint` 区分（`build-desktop.sh:62` 由 `desktop/source-fingerprint.cjs` 对源码取 12 位十六进制指纹，注释改动会改变指纹），诊断日志里不会混。

---

## 1. P1　注释与常量脱节

工单指出 `loadImageSources` 里那段看门狗注释仍写着「这里保证 25s 内必定推进一次」，而 `CARD_IMAGE_STALL_MS` 同一轮已经从 25000 改成 `45_000`。全文件扫了一遍，一共有**三处**注释带旧数值，不止工单点出的那两处：

| 位置 | 原文 | 改成 |
|---|---|---|
| `loadImageSources` 看门狗定时器上方（工单点出的那处） | `// 不会执行。这里保证 25s 内必定推进一次。` | `// 不会执行。这里保证 CARD_IMAGE_STALL_MS（见文件顶部那条常量的注释）内必定`<br>`// 推进一次；写死具体秒数会和常量脱节，R133 P1 就是在收拾这个。` |
| `resetDialogImage` 里（工单说的「同函数上方紧邻的另一条」实际在这里——它是评审阶段新加的弹窗清理注释） | `// 否则它在 25 秒后醒来给一张已经关掉的弹窗重新写 data-queued-src。` | `// 否则它会在 CARD_IMAGE_STALL_MS 之后醒来，给一张已经关掉的弹窗重新写 data-queued-src。` |
| `CARD_IMAGE_STALL_MS` 常量自己的推导注释 | `// 工单写的是 25 秒，实测口径下必须比这个大：…` / `// …取 25s 会让看门狗抢在第二层前面推进…` | 保留全部推导，但不再出现具体旧数值：`// R130 工单原本给的阈值比这个短，实测口径下必须放大：…阈值取在 30s 之前会让看门狗抢在第二层前面推进…45s 让它退回真正的兜底位置；工单里那个更短的值已作废（原始数值与推导过程记在 docs/r130-execution-ledger.md 第 10 节，R133 P1 复核）。` |

前两处按工单要求改成引用常量名而不是写死秒数——这样以后调阈值不会再产生第二份需要同步的数字。第三处是常量自己的推导注释，工单没点它，但它的验收标准是 `grep -n "25s"` 不再有命中，而且「工单写的是 25 秒」这句话留在代码里同样会让下一个人以为当前值该是 25 秒；原始数值改由台账承载（`docs/r130-execution-ledger.md` 第 10 节那条阻断缺陷记录里写得很完整），代码里只留结论和指向。

### 验收（工单原文的两条）

```
$ grep -n "25s" backend/web/app.js
（无命中）
$ grep -n "25 秒" backend/web/app.js
（无命中）
$ grep -n "CARD_IMAGE_STALL_MS" backend/web/app.js
17:  const CARD_IMAGE_STALL_MS = 45_000;
1668:      // 不会执行。这里保证 CARD_IMAGE_STALL_MS（见文件顶部那条常量的注释）内必定
1677:      }, CARD_IMAGE_STALL_MS);
1813:    // 否则它会在 CARD_IMAGE_STALL_MS 之后醒来，给一张已经关掉的弹窗重新写 data-queued-src。
```

常量实际值 45_000ms，推导注释里写的数字是 30s（第二层最坏放弃时间）与 45s（本常量），两处一致，没有第三个游离数字。工单说这条不需要新增测试——不过 R130 已经有一条 `P1-A 看门狗必须晚于第二层的最坏放弃时间`，它从 `image-queue.js` 读出 10s/10s/1 次算出 30s 并断言生产常量更大，所以**数值本身**是有护栏的，脱节的只可能是注释文字，而注释文字不参与运行。

## 2. P2　真机手动验证：本机做不了，交接件已备好

### 为什么做不了（不是跳过，是环境不具备）

工单要求「找一台能连英雄联盟客户端的机器」。在这台 macOS 上逐项查过：

| 检查 | 命令 | 结果 |
|---|---|---|
| 客户端进程 | `ps aux \| grep -iE "league\|riot\|LeagueClientUx"` | 无 |
| 安装路径 | `ls -d "/Applications/League of Legends.app" "/Applications/Riot Client.app"` | 不存在 |
| Riot 数据目录 | `ls "$HOME/Library/Application Support/Riot Games"` | 不存在 |
| LCU lockfile | `find "$HOME/Library/Application Support" -maxdepth 4 -iname lockfile` | 无 |
| LCU 监听端口 | `lsof -nP -iTCP -sTCP:LISTEN` | 只有 rapportd / ControlCenter / DingTalk / clash 等本机服务，没有 LCU |

R130 台账第 8 节「未验证 / 边界」第 1 条已经如实记了这一步没做，本轮结论不变：**P2 仍然未执行**。不编造结果、不拿 jsdom/虚拟 DOM 的机制验证冒充真机负载验证。

### 交接件：把工单第 6 步的日志判读做成可复核的命令

工单第 6 步要求「搜索 `card_image_stalled`，对照同一时间窗有没有大量 `local_request_client / failed(endpoint=image)`，分成第二层积压与通知失败两种，分开处理不要混在一起改」。这一步靠人眼在几千行 JSONL 里数窗口，数错就会改错方向，所以固化成脚本：

```
node scripts/r133-stall-log-report.cjs <lol-loot-diagnostics-*.jsonl> [--json]
    [--window 60000] [--forward 10000] [--backlog-threshold 3]
```

只读日志文件，不连客户端、不读存档、不改任何东西。输出构建指纹、`app_start` 版本、时间跨度、三类事件的总数、**每条 stall 单独数窗口**的结果与判定，退出码 0 = 干净或只是积压 / 1 = 需要开工单或证据不足 / 2 = 用法错误。

判定口径（证据不足一律降级，不猜）：

| 判定 | 条件 | 该怎么处理 |
|---|---|---|
| `clean` | 0 条 stall，**且**日志出自 ≥ 0.12.18 的包 | P2 结项，在执行记录里补一句「已跑，无 stall」 |
| `clean-unverified` | 0 条 stall，但日志里找不到 ≥ 0.12.18 的 `app_start` 版本记录 | **不能结项**：这个包根本不会上报该事件，「没有 stall」不是证据 |
| `backlog-only` | 每条 stall 窗口内都有 ≥ 阈值条图片失败 | 第二层在正常超时/重试，名额是被排队等满看门狗时长的卡片占住的。该调的是第二层远程/本机道名额，**不是** R130 的合成 error / 看门狗 |
| `notify-gap` | 有 stall 但窗口内 0 条图片失败 | 第二层放弃时又没通知到第一层，合成 error 的某个闸门有漏洞。**新缺陷，另开工单** |
| `inconclusive` | 窗口内图片失败数落在 1..阈值-1 | 证据不足，不要据此改任何代码；调大 `--window` 重跑或重做一次真机验证 |
| `undated` | 有 stall 但缺 `time` 字段 | 先确认日志导出是否完整 |
| `whitelist-regression` | 出现 `raw_event` 含 `card_image_stalled` 的 `client_diagnostic_rejected` | 优先级最高：上报根本没落盘，后面所有 stall 计数都是假的 |

**`clean-unverified` 这条是拿真实旧日志冒烟时逼出来的**：直接对仓库里已有的 `docs/r89-kr-player/diagnostic-samples.jsonl`（2026-09-14，`build_fingerprint: dev`，22 条记录）跑脚本，第一版输出了「P2 可以结项：整份日志里一条 card_image_stalled 都没有，R130 的修复在真实负载下成立」——那份日志比 R130 早九天，事件当时还不存在，这个结论是凭空来的。现在脚本先从 `app_start` 记录读 `version`（真实日志里只有这条带版本号，见 `backend/main.go:1645`），比 `STALL_EVENT_MIN_VERSION = "0.12.18"` 旧或读不到就一律降级成 `clean-unverified` 并明说「0 条 stall 不算证据」。同一份旧日志现在的输出是：

```
构建指纹：dev
app_start 版本：（日志里没有带 version 的记录）
card_image_stalled 事件支持：无法确认（日志里没有带 version 的 app_start 记录），0 条 stall 不算证据
…
card_image_stalled：0 条
判定：clean-unverified
```

测试 `scripts/r133-stall-log-report.test.cjs`（14 条，全部用合成夹具，文件头已注明**不是**任何一次真机验证的结果）覆盖：六种判定各一条、窗口边界（正好在 `-window` 与 `+forward` 上算进、再远一毫秒不算）、多条 stall 各自判不摊平（前一批积压 + 后一批通知失败 → 整体必须判 notify-gap）、坏行不静默跳过、白名单回退盖过其它判定、版本比较不引入 semver 依赖、命令行参数与退出码。其中两条是对抗变异：

- `把窗口算错（全量计数当成窗口计数）必须被测出来`——5 条失败里只有 2 条在窗口内，脚本若拿全局总数当窗口计数就会误判成积压。
- `旧包的日志里 0 条 stall 不得判成 clean`——0.12.15 / 0.12.17 / 0.11.99 / 无 app_start 四种情况都必须判 `clean-unverified`；同时钉住「有 stall 时版本不影响结论」（能报出 stall 就说明包支持该事件）。

`scripts/*.test.cjs` 不在 CI 的两条 `node --test` 命令里（CI 只跑 `backend/web/*.test.cjs` 与 `desktop/*.test.cjs`），和既有的 `scripts/r82-startup-ab-report.test.cjs`、`scripts/make-release.test.cjs` 一样需要手动跑：`node --test scripts/r133-stall-log-report.test.cjs`。

### 真机执行的步骤（谁有条件谁做，约 8 分钟）

1. 装 0.12.18 或更新的包，启动后先在设置页确认版本号，并确认已经连上国服客户端进入大厅。
2. 收藏页「全部皮肤」视图，快速来回拖动滚动条，尤其跳到列表很靠下的位置再跳回顶部。
3. 连续悬停 10 张以上带动态原画的皮肤卡片，每张停 1–2 秒再移到下一张。
4. 在「三合一奖池」子页与「全部皮肤」子页之间来回切换几次。
5. 上面三个动作穿插进行，**持续 5 分钟**；记下实际操作时长。
6. 5 分钟结束时看当前屏幕上的可见卡片是否都能在 2 秒内出图（不是长时间停在「加载中」）。
7. 导出诊断日志，然后：

   ```
   node scripts/r133-stall-log-report.cjs <导出的 lol-loot-diagnostics-*.jsonl>
   ```

8. 按输出处理：
   - `clean` → 把「操作时长 + `card_image_stalled` 0 条 + 脚本输出」补进 `docs/r130-execution-ledger.md` 第 8 节，P2 结项。
   - `backlog-only` → 记下每条 stall 的窗口失败数，下一轮调第二层名额，**不要**动 R130 的合成 error / 看门狗。
   - `notify-gap` → 这是新缺陷，按项目惯例另开工单，把脚本的 `--json` 输出附在工单里。
   - `inconclusive` / `undated` / `clean-unverified` → 证据不足，先按判词里写的办法补样本，不要据此改代码。

## 3. P3　Go 测试复核（代跑完成）

工单说 Claude 那轮只做了代码审阅、没能自己编译执行 `backend/r130_test.go`（往云端传文件被拒，报设备登录状态过期 `untrusted_device`），并说「如果你已经处理过那个登录提示，下次我可以重新尝试独立编译执行（连同 `-race` 那一轮）」。

我这边不需要那条通道，直接在本机跑了，**P3 要求的复核已经完成**：

| 命令 | 结果 |
|---|---|
| `go test -race ./backend -count=1` | `ok  lol-loot-assistant/backend  247.355s`，退出码 0（CI 用的就是 `-race`） |
| `go test ./backend -count=1` | `ok  lol-loot-assistant/backend  201.706s` |
| `go test ./backend -run 'R130' -count=1 -v` | 5 条全 PASS（含 4 条子测试） |
| `go build ./backend` | OK |
| `go vet ./backend` | 无输出 |
| `gofmt -l backend/features.go backend/r130_test.go` | 无输出 |

Claude 那边的登录问题属于它自己的通道，不影响本项：`-race` 这一轮已经有人真跑过了，不需要等下一次。如果它之后还想独立复核一遍，重跑上面第一条命令即可。

## 4. 本轮全部验证

| 命令 | 结果 |
|---|---|
| `node --check backend/web/app.js` | OK（P1 只改注释，仍确认一遍语法） |
| `node --test backend/web/*.test.cjs scripts/r133-stall-log-report.test.cjs` | **699 / 699 通过，0 失败**（685 条前端 + 14 条新增脚本） |
| `node --test backend/web/r130.test.cjs` | 28/28（含 `P1-A 看门狗必须晚于第二层的最坏放弃时间`，它从 `image-queue.js` 读常量算出 30s 再断言生产值更大，所以 P1 改注释不会让数值护栏失效） |
| `go test -race ./backend -count=1` | ok 247.355s |
| `go build ./backend` / `go vet ./backend` / `gofmt -l`（本轮涉及的两个 Go 文件） | 全部干净 |
| `node scripts/r133-stall-log-report.cjs docs/r89-kr-player/diagnostic-samples.jsonl` | 正确输出 `clean-unverified`，退出码 1（真实旧日志冒烟，验证版本闸门生效） |

### 先前就红、本轮未动（与 R130 台账第 8 节同一批，范围外）

| 用例 | 原因 |
|---|---|
| `desktop/overview-render.test.cjs:774` `R101 保留旗帜只读入口` | 断言 `.facade-left [data-facade-banners]`，R123 已把它换成 `data-facade-browse="banners"`（`grep -c data-facade-banners backend/web/suite.js` = 0） |
| `desktop/diagnostics-2024.test.cjs:126` | 断言 UI 里有 `不代表抽取、刷新或保底概率`，R128 §2.5 已按用户指示删净（`grep -c … backend/web/gameplay.js` = 0） |
| `scripts/setup-only-build.test.cjs` 3 条 | NSIS 打包脚本用例，需要 Windows/`makensis` 环境；本轮 `scripts/` 只新增了两个文件（`git status` 可证），未触碰该脚本 |
| `gofmt -l backend/` 列出 `champions.go`、`gameplay.go`、`hexdata.go`、`overview_current_game.go` | 均为 R133 之前的未提交改动，CI 的 gofmt 门禁本来就是红的；本轮没有顺手格式化（超范围） |

## 5. 改动文件清单

| 文件 | 改动 |
|---|---|
| `backend/web/app.js` | P1：三处看门狗注释与 `CARD_IMAGE_STALL_MS = 45_000` 对齐，改为引用常量名；无逻辑改动 |
| `scripts/r133-stall-log-report.cjs` | 新增，P2 第 6 步的日志判读脚本（只读日志，246 → 282 行） |
| `scripts/r133-stall-log-report.test.cjs` | 新增，14 条用例含 2 条对抗变异 |
| `docs/r130-execution-ledger.md` | 第 8 节「未验证 / 边界」第 1 条补上指向本台账的交接说明 |
| `docs/r133-execution-ledger.md` | 本文件 |

生产运行时行为**零变化**：`backend/web/` 下只改了注释，`scripts/` 不参与 `//go:embed`、不进安装包，`backend/` 一行未动。所以版本号保持 0.12.18，`backend/static_assets_test.go` 的嵌入清单也无需同步。
