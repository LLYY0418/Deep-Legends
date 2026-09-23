# WORKLIST-R120：R120 修复后的复测——一个被改弱的测试与两个潜伏的随机红

**撰写日期：** 2026-09-22　**基线版本：** 0.12.14
**触发：** 对 R120 台账（`WORKLIST-R120-INDEPENDENT-VERIFICATION-R116-R118-GAPS` 的执行结果）的独立复测。
**本轮性质：** 纯审查，未改动仓库任何文件。R120 各项基本落地正确（见末尾「已复核无需返工」）；下面是复测中新发现的 3 个测试问题和 1 个可选加固。**没有一项是生产代码写错。**

## 0. 三句话结论

1. **R120 P2-2 把 `TestProDirectoryPublishesBeforeSlowSupplements` 改弱了**：去掉 1 秒墙钟断言是对的，但换成的「同步点」只在补充源永远不返回时才成立；补充源自带 12 秒 × 2 次的超时，所以「生产代码改成等补充源」现在测试照样通过（P1-1）。
2. **有一个早就存在的随机红，R120 没有发现**：`Test2351BanSentinelDoesNotAuthorizeConfiguredHeroes` 单独 `-race` 跑约 18% 概率失败，而且它断言的行为和后来定下的「通配符先悬停」契约相反，大多数时候是因为断言得太早才碰巧通过（P2-1）。
3. **另一个 0.025% 的随机红**：诊断日志里的时间戳偶然包含子串 `12345`，让「不得泄漏 Riot ID」的断言误报（P2-2）。

## 执行纪律（延续 R117–R121）

- 每项验收判据都要做**对抗变异**（改回原样、或换一种同等效果的写法，对应测试必须 FAIL）。
- 变异在隔离副本里做，还原用「整文件从基线拷回 + `diff -q`」，**禁止 `git checkout -- <file>`**。
- 只改测试，不改生产口径；不允许靠加大等待时间、重试或 `t.Skip` 收敛。

## 建议执行顺序

P1-1 → P2-1 → P2-2 → P3-1（可选）。

---

# P1

## P1-1　`TestProDirectoryPublishesBeforeSlowSupplements` 现在测不出「目录发布在等补充源」

**证据（变异实测）：** `backend/pro_players.go` 的 `loadProPlayers`，在发布目录的那一行之后插入一句同步等待：

```go
teams = withProSeed(append(cloneProTeams(base), retainProSupplements(pendingProSupplements(), previous, time.Now())...), seeds)
_ = loadProSupplements(loadCtx, provider, func([]opggProTeam) {}) // 变异：发布前先等补充源
```

`go test -race -count=1 -run TestProDirectoryPublishesBeforeSlowSupplements ./backend` → **ok（25.4 秒）**。原因：测试里的补充源在 `release` 关闭前一直阻塞，但 `loadProSupplements` 每个请求有 12 秒超时、最多两次（`pro_players_supplement.go:103`），约 24 秒后自己返回；此时 `loadProPlayers` 仍然带着 42 支队伍返回，而测试唯一的失败条件是「返回错误或不足 42 队」，看门狗是 60 秒，够不着。测试注释写的「`release` 只在测试结束时关闭，所以补充源不可能返回」对「补充源自己超时返回」不成立。旧版本的 1 秒预算能挡住这个变异，R120 去掉它之后就挡不住了。

**影响：** 这条测试保护的是「目录先发布、不等补充源」这个产品承诺（页面先出名单再逐步补全）。现在有人把发布挪到补充源之后，用户会看到名单要多等 24 秒，而测试全绿。

**修复方向（我在隔离副本里验证过）：** 用一个不依赖墙钟的完成计数：

1. 补充源 handler 在返回前 `supplementDone.Add(1)`（`release` / 上下文取消之后、`return` 之前）；
2. 第一次 `loadProPlayers` 返回后立即断言 `supplementDone.Load() == 0`——只要发布等过任何一次补充源请求，这里就非 0；
3. 保留现有的 42 队、`updating` 标记、三次轮询后目录请求恰好 1 次、看门狗与 `refreshCtx` 取消。

我按这个改法实测：未变异时 `-race -count=20` 全过（2 秒）；上面的变异下 FAIL，报 `directory returned only after 6 supplement requests had finished`。

**验收判据：**
- 上面的变异使测试 FAIL；
- `GOMAXPROCS=1` 与默认并行两种条件下 `go test -race -count=100 -run TestProDirectoryPublishesBeforeSlowSupplements ./backend` 全绿（R120 P2-2 原来的「27 次假红」不能回来）；
- 再加一个变异：让轮询重新拉目录（把 `if c.updating || …` 里的 `c.updating ||` 与 `|| backoff` 去掉），`poll refetched directory` 断言必须 FAIL。

---

# P2

## P2-1　`Test2351BanSentinelDoesNotAuthorizeConfiguredHeroes`：随机红，而且断言与现行契约相反

**证据：**
- 复现率（同一条测试，代码不变）：`-race`、全新进程单跑 **27 / 150** 失败（18%）；`-race -count=400` 每轮 30–45 次失败；不带 `-race` 900 次里 1 次。基线（0.12.13）同样有（`-race -count=400` 30–36 次），所以不是 R119 / R120 引入的。整包 `-race -count=1` 空载连跑 3 次全绿；与 Node 测试并发时红过 1 次。
- 失败信息是 `empty-ban sentinel used as a wildcard`（`diagnostics_2351_test.go:77`）。我在副本里把失败时的 PATCH 内容打出来，46 次全是同一个：`{ChampionID:141 Completed:false Type:ban}`。
- 根因不是竞争，是**断言得太早**：`champselect_execution.go:86-103` 明确把「可禁列表只有 `[-1]` 且当前有进行中的 ban 动作」当成通配符，从选人网格里挑候选，走 `wildcard-grid-hover`；`champselect.go` 里 `forceHover` 使它先发一次 `completed:false` 的悬停 PATCH，延迟为 0，由 `scheduleChampSelectRequest` **异步**发出。测试在 `evaluateChampSelect` 返回后立刻看 `f.patches`，PATCH 大多还没发出去所以「碰巧为 0」，调度快一点就为 1。证明：在断言前加 `time.Sleep(300ms)`，**20 / 20 全部失败**。
- 契约冲突：`r91_addendum_test.go:250`（「wildcard must hover first」）和 `r95_test.go:304`（「wildcard must hover before lock」）把「通配符先悬停、再锁定」定为现行行为，而本测试的名字和信息还在断言「哨兵不能当通配符」，是更早（R78 / 2351）的写法，被后来的设计取代后没有同步。所以它目前的状态是：多数时候因为断言得太早而没验证任何东西，少数时候红得莫名其妙。

**修复方向：** 改测试，不改生产（悬停优先是已定契约）。让这条测试真正断言现行契约：
1. 用 `f.patchCh`（带超时，例如 5 秒）等待那一次悬停 PATCH，而不是立刻看计数；
2. 断言载荷是 `Type:"ban"`、`ChampionID` 是配置里且未被禁的英雄（141）、**`Completed:false`**；
3. 断言在没有「悬停已确认」之前不会出现 `Completed:true` 的 PATCH（参考 `r95_test.go:304` 的 `waitTakeoverIdle` 与 `f.last().Body["completed"]` 写法）；
4. 测试名与失败信息改成描述现行契约，例如「`[-1]` 哨兵只触发一次悬停，不直接锁定」。旧的「哨兵不授权任何英雄」语义如果仍想保留，请在台账写明取舍依据，不要留一条名实不符的测试。

**验收判据：**
- `go test -race -count=200 -run 'Test2351BanSentinel' ./backend` 在默认并行与 `GOMAXPROCS=1` 下全绿；
- 变异：让通配符直接锁定（`champselect.go` 中 `forceHover := side == "ban" && strings.HasPrefix(banSource, "wildcard-")` 改成 `false`）→ 新测试必须 FAIL；
- 变异：让 `[-1]` 不再产生候选（`champselect_execution.go:86` 条件置为 `false`）→ 新测试必须 FAIL；
- 整包 `go test -race -count=1 ./backend` 连续跑 3 次全绿（R120 P2-2 原判据，本条补上它漏掉的部分）。

## P2-2　诊断日志里的时间戳偶然含有 `12345`，「不得泄漏 Riot ID」断言误报

**证据：** `backend/gameplay_test.go:231`：`strings.Contains(line, "12345")` 用来检查诊断日志没有泄漏 tagLine。诊断行里有 `"time":"…43.21234509Z"` 这样的纳秒时间戳，五位纯数字子串出现的概率不可忽略。实测 `go test -count=20000 -run TestGameplayOverviewTencentLookupHTTPErrorIsExplainedAndRecorded ./backend`：**5 / 20000 失败**（约 0.025%），失败输出里能直接看到 `"time":"2026-09-21T16:01:43.21234509Z"`。整包 `-race -count=3` 时红过一次就是它。
同一类的还有 `backend/loot_metadata_test.go:198`（`"99999"`）；`123456`、`123456789`、`987654321` 这类更长的串概率可忽略。
**修复方向：** 哨兵值不要用纯数字。tagLine 换成含 `a–f` 之外字母的串，例如 `ZQ7XK`（诊断行里的数字只来自时间戳和计数，`run_id` 只含十六进制字符 `0–9a–f`，所以不会碰撞），断言同步换成同一个串；`loot_metadata_test.go:198` 同理。我在副本里把这条测试的 tagLine 与断言换成 `ZQ7XK`，`-count=20000` **0 次失败**，单跑通过。
**验收判据：** 上面两条测试 `-count=20000`（不带 `-race`）全绿；把日志里故意写进 tagLine（变异：在诊断事件里加一个 `"tag":<tagLine>` 字段）仍然必须 FAIL。

---

# P3

## P3-1　（可选加固）CI 护栏与隐私目录枚举各有几种形状仍能绕过

R120 P1-2 / P1-4 的结构性测试对工单点名的所有写法都有效（见末尾）。我另外试了几种换写法的变异，下面这些**仍然通过**。它们都需要有意为之，或是目前代码库里没有出现的写法，所以只列为可选；只想防误改可以跳过，想防到底就按下面的低成本做法补。

**CI（`backend/web/r117.test.cjs` 的「real Chromium guards are wired into CI」）：**
- `jobs.quality.needs` 指向一个 `if: false` 的新 job → 整个 quality job 被跳过，测试通过（`needs` 在允许清单里）。
- 在 `Check formatting` 之前加一步 `echo "R100_APP_SOURCE=/dev/null" >> "$GITHUB_ENV"`（或 `NODE_OPTIONS=…`）→ 护栏读到桩源码，测试通过（只检查了 workflow / job / 步骤 `env`，没检查别的步骤往 `$GITHUB_ENV` 写东西）。
- 给运行本测试的 `Test frontend and desktop renderers` 步骤加 `if: false` → 整套前端测试（包括这条）不再执行，测试自己当然不会报。
- 低成本做法：对 `quality` job 的 `needs` 断言为 `undefined`；对所有步骤的 `run` 文本断言不含 `GITHUB_ENV` / `GITHUB_PATH`；对 `Test frontend and desktop renderers` 与 `Real Chromium…` 两步断言没有 `if` / `continue-on-error`。

**隐私目录枚举（`backend/quality_test.go` 的 `TestPrivacyStoresDeclareEveryLocalDirectory`）：**
- 目前枚举 16 个目录，与源码创建点逐一对得上；新增未声明目录时，`storage.go` 启动循环、`newPublicBinaryCache(...)`、「变量 = `filepath.Join(…root, "x")` 再 `MkdirAll(变量)`」三种写法会 FAIL。
- 仍会漏的写法：行内 `os.MkdirAll(filepath.Join(store.root, "x"), …)`、目录名用常量、`writeLocalStoreFile(store, "x/y.json", …)` 直接写子目录、根变量不叫 `root`。这些我各加了一个变异，全部存活。
- 低成本做法：再加一个「生产源码里 `MkdirAll` / `os.Mkdir` 调用点清单」的钉死断言（按文件 + 次数，例如 `champion_images.go:1`、`pro_runes.go:1`、`storage.go:2`、`update.go:1`），有新增调用点就红，逼作者归类；`writeLocalStoreFile` 的调用点同样钉死。

**验收判据：** 采纳哪几条由执行方在台账里逐条写「做 / 不做 + 理由」；做了的每条对应的变异必须使测试 FAIL。

---

# P3（需要你操作）

## P3-2　沿用 R120 P3-3：真机回填仍待做

R116 的三项探测观测值（`docs/r116-probe-findings.md`）与 R119 补位对照（`WORKLIST-R119-AUTOFILL-DIAGNOSTIC-EVENTS-UNTESTED` P3-2）都还是空的。另外，CI 里的「Real Chromium…」步骤我只能在容器里以非 root 用户验证（`r100` / `r117` 各 20 次全过，无残留 Chrome 进程与临时目录），没有 GitHub runner 实测：第一次推送后请确认这一步是绿的（`ubuntu-latest` 上 Chrome 需要能创建用户命名空间）；同时看一下 quality job 总耗时，`desktop/overview-render.test.cjs` 单独就要约 340 秒。

---

## 已复核无需返工（不要重复排查）

- **构建与格式：** `go build`、`go vet`、`gofmt -l`（`backend/*.go`）全部干净；整包 `go test -race -count=1 ./backend` 空载连跑 3 次全绿；Node 全量 885 项 / 884 通过 / 0 失败 / 1 跳过；根模块的嵌入奖池测试通过。
- **P1-1：** `gameplay.go` 已对齐。
- **P1-2：** CI 结构性测试对 R117–R120 已知的 15 种形状（job / 步骤 `continue-on-error`、job `if`、`|| echo` / `|| :` / `; true` / `set +e`、`shell: bash {0}`（步骤 / job / workflow 三处）、只留 `workflow_dispatch`、永不匹配的分支过滤、步骤 `timeout-minutes: 0`、`working-directory`、`strategy fail-fast`、步骤 `env` 跳过开关）以及数组形式的 `on`、workflow 级 `R100_QUEUE_SOURCE` 覆盖、job 级 `timeout-minutes` 全部打红。
- **P1-3：** `desktop/r100-browser.cjs` 与 `r117-browser.cjs` 以非 root 用户各连跑 20 次，40 / 40 通过，跑完没有残留 Chrome 进程和临时目录（异步清理修复有效）。
- **P1-4：** `stores` 现在 12 项；删掉 `season-stats` 那一条 → 测试 FAIL；新增未声明目录的三种主要写法 → FAIL。`season-stats` 条目里的「4 MiB」「先截断逐场快照、绝不丢弃英雄统计」与 `season_stats_budget.go` 的三级处置一致。
- **P2-1：** 闭包改捕获局部通道；把它改回共享变量（`-race` / 泄漏断言 FAIL）、把两个通道分别改成无缓冲（泄漏断言 FAIL，报 `baseline=2 now=3`）全部被打红；新增的 goroutine 泄漏测试有效。
- **P2-3：** `hexdata_r116a_test.go` 的注释与实测一致（answer / meta 受保护，其余 6 处为冗余防御）。
- **P2-4：** GE2（第三级顺手清 `Stats`）、GE2b（第二级一上来清空样本）、GE4（去掉内层队列门禁）以及额外的「减半改成四分之一」「去重失效」「排序改升序」全部被打红。
- **补位诊断事件缺测试**是另一个问题，见 `WORKLIST-R119-AUTOFILL-DIAGNOSTIC-EVENTS-UNTESTED`。
