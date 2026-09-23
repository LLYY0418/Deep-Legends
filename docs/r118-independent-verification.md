# R118 独立验收：P1-2 / P2-8 / P3-9 复核 + 换形状对抗变异

**验收日期：** 2026-09-21　**验收对象：** `docs/r118-execution-ledger.md`（工单 `docs/WORKLIST-R118-INDEPENDENT-VERIFICATION-GAPS.md`）
**验收快照：** 用户工作树的一份隔离拷贝（`desktop/package.json` 版本 0.12.13，含 R119 未完成的改动）；另用 HEAD `4ca768ec` 的 `backend/` 做 A/B 对照。**没有触碰用户工作树里的任何代码文件**，所有变异都在隔离副本里做、事后逐文件 `diff` 确认还原。
**验收方法：** 不信任台账自述；全量复跑；对工单里已经点名的变异**换不同形状**再做一遍（R117 三轮验收沉淀的教训：「改回原样测试只证明没被这一种退化绕过，不等于堵住了这一类退化」）。
**姊妹文档：** `docs/r116-independent-verification.md`（同一轮验收，R116 部分；全量测试复跑表在那一份里，此处只引用）。

---

## 0. 结论

1. **P2-8（`hexdata_r116f_test.go` 诊断回调 race）真实修好**：加锁是承重的——把锁去掉、只留 `pruneWait.Wait()`，`go test -race` 稳定复现 `DATA RACE`；把 `Wait()` 去掉、只留锁，测试仍 PASS（`Wait` 只保证读取时机确定，不是 race 的承重件，与工单「加锁只能消除 race，不能保证时机」的表述一致）。
2. **P1-2（job 级 CI 护栏）台账自述属实，但「这一类」绕过没有堵死**：台账列的 6 条变异我全部复现并确认 FAIL（`jobs.quality.continue-on-error`、`jobs.quality.if`、`|| echo`、`|| :`、`; true`、`set +e`）；但换 10 种不同形状，**10 种全部存活**（测试仍 PASS），其中 5 种是真实等效的「护栏不执行 / 失败被吞」：step 级 `shell: bash {0}`、job 级 `defaults.run.shell`、workflow 级 `defaults.run.shell`、workflow 触发器只留 `workflow_dispatch`、触发器加永不匹配的 `branches`。根因与 R118 工单一致——测试是**逐字段黑名单**，不是「护栏必须真的会在默认路径上执行并让 job 失败」的白名单语义。
3. **新发现（台账没有）：两个真 Chromium 脚本在「打印 PASS」之后会以退出码 1 结束**。`r100-browser.cjs` / `r117-browser.cjs` 末尾 `finally` 里的 `fs.rmSync(temp, {recursive, maxRetries: 8, retryDelay: 100})` 在 Chrome 尚未退净时抛 `ENOTEMPTY`，未捕获 → `process` 退出码 1。我的沙箱里 r100 非 root 4/6 次、r117 3/5 次触发。CI 步骤默认 `bash -e`，这会让 CI 在护栏**实际通过**的情况下变红。GitHub runner 上的发生率我无法验证（环境敏感，磁盘慢时更容易），需要在真实 runner 上看一眼。
4. **台账里「`desktop/overview-render.test.cjs` 文件级 worker failure」没有复现**：该文件单独跑 54/54 通过、退出码 0、耗时 338 秒（约 5.6 分钟）。台账的「超过 4 分钟未结束」是慢而非挂；全量 Node 套件我这里 885 项 884 通过 / 0 失败 / 1 跳过。慢本身是个隐患（见 §4）。
5. **P3-9**：台账选择不实施，与工单「低优先级、不建议单独开工」一致，认可。补一条核对：`desktop/main.cjs`、`overview-banner-layout.cjs`、`share-export.cjs` 里有 hex 字面量，但都不在 `tokenHexes` 白名单内，所以「扩大扫描范围也不会立刻红」的前提仍成立；但工单里「`desktop/*.js`」这个说法不准确——desktop 目录下是 `.cjs`，将来若扩范围，正则 `/\.(?:js|html)$/` 需要同时放行 `.cjs`，否则新纳入的文件会被静默漏掉。

---

## 1. 全量复跑（摘要）

详细表与命令见 `docs/r116-independent-verification.md` §1。与本份相关的数字：

| 项 | 结果 |
|---|---|
| `go build ./backend`、`go vet ./backend` | 通过 / 空 |
| `gofmt -l backend/*.go` | **`backend/gameplay.go` 被列出**：R119 新增字段 `Autofill bool \`json:"autofill,omitempty"\`` 与下一行 `reference gameplayReference` 未对齐（`gofmt -d` 要把 `Autofill` 改成 `Autofill  bool`）。R118 台账写的「gofmt 无输出」在 R118 当时成立，是 R119 之后的新回归；CI `quality` 第一步就是 gofmt 检查，**推上去 CI 会立刻红** |
| `node --test backend/web/*.test.cjs desktop/*.test.cjs` | 885 项 / 884 通过 / 0 失败 / 1 跳过（Windows 专属发布门禁） |
| `go test -race -count=1 ./backend`（不切块，整包跑了 2 次） | **两次各红 1 个、且不是同一个**：第一次 `TestProDirectoryPublishesBeforeSlowSupplements`，第二次 `TestStructuredDetailFallsBackWhenQQ101BuildTimesOut`（附 2 处 `DATA RACE`，落在 `champions_structured.go:1285/1286`）。两者 HEAD 上同样存在，详见 R116 文档 §4。**所以台账里「全量 `go test -race` 不切块 PASS（267s）」只是单次采样，并不表示该门槛稳定为绿** |

---

## 2. P1-2：CI YAML 变异（16 种）

方法：`.github/workflows/ci.yml` 拷到隔离副本，逐个变异，每个都单独跑 `node --test --test-name-pattern="R117 real Chromium guards are wired" backend/web/r117.test.cjs`，跑完从基线整文件拷回还原。

**控制组（台账已列的 6 种，全部复现，全部 FAIL = 被拦住）**

| # | 变异 | 结果 |
|---|---|---|
| C1 | `jobs.quality.continue-on-error: true` | FAIL |
| C2 | `jobs.quality.if: github.event_name == 'workflow_dispatch'` | FAIL |
| C3 | 命令后缀 `|| echo "skipped"` | FAIL |
| C4 | 命令后缀 `|| :` | FAIL |
| C5 | 命令后缀 `; true` | FAIL |
| C6 | run 块前置 `set +e` | FAIL |

**换形状（10 种，全部 PASS = 全部存活）**

| # | 变异形状 | 是否真实绕过护栏 | 说明 |
|---|---|---|---|
| N1 | 护栏步骤加 `shell: bash {0}` | **是** | GitHub Actions 里自定义 `shell: bash {0}` 会去掉默认的 `-e`，步骤退出码 = 最后一条命令的。r100 失败、r117 通过 → 步骤绿 |
| N2 | `jobs.quality.defaults.run.shell: bash {0}` | **是** | 同上，作用到整个 job |
| N5 | workflow 级 `defaults.run.shell: bash {0}` | **是** | 同上，作用到整个 workflow |
| N3 | `on:` 只保留 `workflow_dispatch` | **是** | push / PR 不再触发整个 workflow——与 P1-2 最初根因「脚本存在但从不在流程里执行」完全同构 |
| N4 | `on.push.branches` / `on.pull_request.branches` 设为永不匹配 | **是** | 同上 |
| N9 | 护栏步骤加 `env: R100_SKIP: "1"` | 否（当前）| 脚本不读这个变量；作为「测试对未知 env 无约束」的探针。**但脚本读 `R100_APP_SOURCE` / `R100_QUEUE_SOURCE`**：CI 里给这两个变量指向一个桩文件，护栏就在测一个假的源码，测试同样拦不住（未单独变异，读代码确认） |
| N6 | 步骤 `timeout-minutes: 0` | 否 | 探针：测试对步骤级未知字段无约束 |
| N7 | 步骤 `working-directory: installer` | 否 | 会响亮失败（找不到脚本），不是静默绕过；同上仅为探针 |
| N8 | job 加 `strategy: fail-fast: false` | 否 | 不改变 job 通过/失败语义；同上仅为探针 |
| N10 | `windows-build` 去掉 `needs: quality` | 否 | 与护栏本身无关，是发布门禁语义的探针 |

**判断：** 台账「6/6 变异被拦」属实，也确实堵住了工单点名的 job 级两种形状。但整个测试的形态仍是「已知坏字段黑名单 + 命令行精确白名单」，对「同等效果、不同语法」的退化没有约束。真正的形状缺口是这 5 个（N1 N2 N5 N3 N4）。

**建议方向（写给下一份工单，不由我代改）：** 与其继续追加黑名单条目，不如把断言换成「白名单语义」：
- 断言 `workflow.on` 至少含 `push` 与 `pull_request`，且二者没有 `branches` / `paths` 过滤；
- 断言 `workflow.defaults`、`quality.defaults`、护栏步骤都没有 `shell` 字段（或恰好等于默认）；
- 断言 `quality` 除 `runs-on` / `steps` / `needs` 之外没有其他控制流字段（`if` / `continue-on-error` / `strategy` / `defaults` / `timeout-minutes` 等）。

---

## 3. P2-8：诊断回调 race

| 验证 | 结果 |
|---|---|
| 基线 `go test -race -count=8 -run '^TestLoadMayhemDetailCarriesHeroStageRarityAndCautionFlags$' ./backend` | ok，0 次 `DATA RACE` |
| M1：去掉 `eventsMu.Lock()/defer Unlock()`，保留 `pruneWait.Wait()` | **FAIL**，`DATA RACE`（锁是承重的）|
| M2：保留锁，去掉 `pruneWait.Wait()` | ok，0 次（`Wait` 不影响 race 判定，只保证断言时机）|

还原后 `diff` 与基线一致。结论：修法到位。补一条与 P2-8 无关但相关的发现：同一 `-race` 门槛下，`champions_structured.go:1178–1186` / `1285–1286` 存在**生产代码**里的真实数据竞争（不是测试问题），详见 R116 文档 §4，建议单独立项。

---

## 4. 新发现与剩余风险

### 4.1 真 Chromium 脚本「PASS 后退出码 1」（未在台账里）

复现（非 root、隔离目录、Chromium 1194；Chrome 由一个仅追加 `--disable-gpu` 的薄包装脚本启动，不改仓库脚本）：

```
r100: 6 次里 4 次  "R100 Chromium PASS {...}" 之后 Error: ENOTEMPTY: rmdir '<tmp>/r100-browser-XXXX/Default'，exit=1
r117: 5 次里 3 次  "R117 Chromium PASS {...}" 之后同样的 ENOTEMPTY，exit=1
```

位置：`desktop/r100-browser.cjs:88`（`r117-browser.cjs` 结构相同）——`.finally(()=>{ ...proc?.kill(); ...; fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100}); })`：`kill()` 后 Chrome 的子进程还在往 profile 目录写，`rmSync` 重试 0.8 秒仍可能失败，异常从 `.finally` 抛出成为未处理拒绝。

影响：CI 步骤是默认 `bash -e`，退出码 1 = 步骤失败。也就是说**护栏实际通过时 CI 也可能变红**——这会把「护栏可信」的价值反过来消耗掉（人会习惯性地重跑或加 `|| true`，而这正好是 P1-2 一直在防的事）。我无法在 GitHub runner 上验证发生率；**需要在真 runner 上看一次实际日志**。
修复方向（不由我代改）：清理改为尽力而为（`try/catch` 或 `force` 之外再吞 `ENOTEMPTY`），或先 `await` Chrome 进程 `exit` 再删目录；同时不能吞掉主流程的失败。

### 4.2 `overview-render.test.cjs` 很慢

单独跑 54/54 通过，338 秒。全量 Node 套件里这个文件占绝大部分时间，台账里「>4 分钟未结束」很可能是碰上了运行器/超时限制而不是断言失败。CI 里如果有 job 级 `timeout-minutes`，这个文件是首个会撞线的。建议给它单独记一个基线耗时；本轮不建议改文件。

### 4.3 复现方式的局限

- 云端沙箱的 CPU 只有 2 核，Go 全量约 100–210 秒/次，`-race` 更慢；所有结论以我实际运行的日志为准，未做 GitHub runner / Windows 真机验证。
- 台账说的 `$PI_SCRATCH_DIR` 是执行方的临时目录，我没有也不需要它。

---

## 5. 方法论沉淀

1. **黑名单式测试对「同类不同形」天然失效。** R118 补了 job 级两个字段，这次换了 10 种形状仍有 5 种是真绕过——下一轮不要继续加黑名单条目，改成「护栏必须处于默认执行路径」的白名单/结构不变量。
2. **「CI 护栏」要连它自己的稳定性一起验。** 一个会在通过时报红的护栏，比没有更糟：它教会人们忽略红灯。
3. **台账里的「全量 PASS」要标注是几次采样。** 本项目 `go test -race` 全量有至少 3 个随机失败源，单次绿不等于稳定绿；验收应至少整包跑两次并把失败项归类到「已知 flaky / 新失败」。
4. **变异的还原要机械化：** 每次都从基线整文件拷回再 `diff -q`，不用 `git checkout`，也不在工作树里做。

---

## 6. 复现命令

```bash
# 环境（云端沙箱）：Go 1.24.x、Node 22、Playwright Chromium；源码为工作树的隔离拷贝
export GOFLAGS=-mod=mod GOTOOLCHAIN=local
# P1-2 变异：对每个变异后的 ci.yml 运行
node --test --test-name-pattern="R117 real Chromium guards are wired" backend/web/r117.test.cjs
# P2-8
go test -race -count=8 -run '^TestLoadMayhemDetailCarriesHeroStageRarityAndCautionFlags$' ./backend
# 真 Chromium 脚本（Chrome 可执行文件路径通过 CHROME_BIN 指定；root 下需要 --no-sandbox，请用包装脚本，勿改仓库脚本）
CHROME_BIN=/path/to/chrome-wrapper R100_BROWSER_OUTPUT=/tmp/r100 node desktop/r100-browser.cjs; echo exit=$?
CHROME_BIN=/path/to/chrome-wrapper R117_BROWSER_OUTPUT=/tmp/r117 node desktop/r117-browser.cjs; echo exit=$?
# overview-render 单独计时
time node --test desktop/overview-render.test.cjs
```
