# R120  wildcard 悬停锁定测试竞态工单执行台账

执行时间：2026-09-22 12:07 – 12:56（本机 macOS darwin/arm64，8 核，Go 1.24.5）
工单原文：`docs/WORKLIST-R120-WILDCARD-HOVER-LOCK-CHECK-RACE.md`
基线：`desktop/package.json` 0.12.15（并行会话所升，本轮未动）；仓库 HEAD `4ca768ec`

---

## 0. 一句话结论

P1-1 已闭合：`TestR91AddendumWildcardHoverClearKeepsArmedLock` 的两个挂钟竞速点全部消除（写冻结闸 + 同步 armed 事件断言 + 悬停优先钉死），只改了两个测试文件，生产代码零改动。对照组以**停顿注入**方式 100% 确定性复现了工单记录的全部三种失败签名；修复后的测试在同等停顿（50ms，约 20 倍于真实竞速窗口）+ 忙等压力 + `taskpolicy` 节流下合计 **1000+ 子用例 0 失败**；工单指定的 `forceHover=false` 变异被**最早、最锐利**的断言抓住（8/8 `wildcard must hover first`）。整包 `go test -race -count=1` 连续 2 次全绿。顺手项（r122 测试头注释滞后）也已按工单修正。

**执行中查明的一个工单描述偏差（重要，影响根因表述）**：锁的武装延迟**不是 120ms**——`lock-now` 策略下生产代码强制 `delay=0`（`champselect.go` 的 delay switch：`case config.Strategy == "lock-now": delay = 0`），测试里 `g.Ban.DelayMS = 120` 对这个分支是**死配置**。实测 armed 事件 `delay_ms=0`、armed→PATCH 完成仅 **≈2.5ms**。竞速窗口比工单描述的还窄两个数量级，这解释了为什么它在 2 核容器上整包 2/2 红、而本机 8 核用忙等/节流压力 5 种组合 1400+ 子用例都无法统计性复现——需要的是 cgroup 配额式的百毫秒级停顿，不是普通 CPU 竞争。缺陷本身真实且机制与工单判断一致（测试 goroutine 被晾过写生命周期即误判），修复方式不受此偏差影响。

---

## 1. P1-1 执行详情

### 1.1 根因复核（先测量再动手）

在隔离快照里加临时探针测试（`zz_delay_probe_test.go`，只存在于快照，未进仓库）实测：

```
armed#0: completed=false champion=141 delay_ms=0(int) trace=cs-write-4   ← tick 的悬停
armed#1: completed=true  champion=141 delay_ms=0(int) trace=cs-write-5   ← 第二次 evaluate 的锁
evaluate返回耗时=1.34ms；evaluate返回→锁PATCH出现=1.11ms；armed→PATCH=2.46ms
```

证实：① armed 诊断事件由 `scheduleChampSelectRequest` 在 evaluate 调用栈内**同步**发出（`champDiagnostic→record→observe` 全同步，`flow_diagnostics.go:75`、`watch_rules.go:1047`），evaluate 返回时必已在 `f.events`——工单修复方向 1 的前提成立；② 锁的真实延迟是 0ms（lock-now），旧测试的 peek 与后续步骤实际在和一条 ≈2.5ms 生命周期的后台写竞速；③ 写成功后 `submitted` 记录同步变 `Completed=true`（`champselect.go` write 分支），`hover-cleared-by-client` 的触发条件 `!last.Completed`（`champselect_takeover.go:106`）从此**永久**不成立——所以「clear diagnostic missing」不是断言太早，是场景被写完成摧毁了，**只把 peek 换成事件断言（工单方向 1）修不掉这一类**，必须让写本身可控（方向 2 的确定性信号思路，落地为写冻结闸）。

### 1.2 改动（2 个文件，纯测试）

**`backend/champselect_execution_test.go`（+18）**：`executionFixture` 新增可选字段 `patchGate chan struct{}`；传输层在 action PATCH 进入既有逻辑前等闸（`select { case <-gate: case <-req.Context().Done(): }`），**唤醒后复查 `req.Context().Err()`，已取消的写绝不落进 `f.patches`**。等待发生在 `f.mu` 之外（测试可正常改会话）；不设置闸门的测试零影响（本轮整包 ×2 全绿佐证）。

**`backend/r91_addendum_test.go`（+63/-9，含新 helper `r91LockArmedSeen`）**，四处强化：
1. **tick 后钉死「悬停优先」**：`f.count()==1 && f.last().Body["completed"]==false`——tick 已等到 idle，此判定确定性；`forceHover=false` 一类变异在这里最早被拦；
2. **武装锁前挂闸**（`gate` + `sync.Once` 放行 + `t.Cleanup(release)` 防提前 Fatal 留悬挂 goroutine）：锁的 PATCH 冻结在传输层，「改会话 + 第三次 evaluate」必然发生在写完成之前，clear/takeover 判定不再赌窗口；104 分支的取消由传输层 ctx 复查感知（yield 先于放行，取消的写不落地，`count==1` 确定成立）；
3. **peek pending 换成同步 armed 事件断言**（工单方向 1）：`r91LockArmedSeen` 要求事件流中**先**出现悬停 armed（completed=false）**再**出现锁 armed（completed=true，champion 141）——顺序敏感，与第 1 条形成双保险；champDiagnostic 的去重只吞「载荷逐字节相同」的事件，锁与悬停的 completed/trace_id 必不同，不可能被吞；
4. 原测试尾部三个断言（count/last、manual-takeover、clear diagnostic）**一字未动**，两个子用例（/0 与 /104）都覆盖（工单方向 3）。

### 1.3 对照组与验收实测

**对照（旧断言 + 停顿注入，全部在隔离快照，`diff -q` 还原核对一致）**——本机忙等压力无法统计性复现（见 §0 偏差说明），改用「在竞速点注入 5ms `time.Sleep`」确定性地等价模拟「调度把测试 goroutine 晾过写生命周期」：

| 注入点（旧测试） | 结果（-race -count=20，40 子用例） |
|---|---|
| A：第二次 evaluate 之后、peek 之前 | **40/40 FAIL，全部 `lock not armed`** |
| B：peek 之后、改会话/第三次 evaluate 之前 | **20/20 `/0` FAIL `clear diagnostic missing`；20/20 `/104` FAIL `different nonzero choice ignored`** |

工单在真实负载下观察到的三种失败签名全部按分支精确复现——机制认定与工单一致。

**统计性复现尝试（如实记录，未成功）**：旧测试在本机 5 种压力配置（GOMAXPROCS=2+2 忙等、1+2、2+8、1+1、taskpolicy -b 节流+2+2，合计 1400+ 子用例，含 -race 与去 -race）全部 0 失败。macOS 无 cgroup 配额式节流，忙等只均匀拖慢双方、不制造单边长停顿；工单环境的 2 核容器（整包 2/2 红、忙等 12/100 红）与 GitHub `ubuntu-latest` 共享跑者才具备这种停顿特征。这正是要修的原因：**本机绿不代表 CI 绿**。

**修复后（新断言）**：

| 条件 | 结果 |
|---|---|
| `-race -count=100`（默认并行 / GOMAXPROCS=1，真树） | ok 3.78s / ok 3.05s，0 失败 |
| 忙等压力（GOMAXPROCS=2 + 2 spinners）`-race -count=100` | ok 7.67s，0 失败 |
| `taskpolicy -b` 节流 + 忙等压力 `-race -count=100` | ok 39.2s，0 失败 |
| **双 50ms 停顿注入**（两个竞速点各晾 50ms，≈20 倍真实窗口）`-race -count=25` | **ok，0 失败**（对照组同点位 5ms 即 100% 红） |
| 双 50ms 停顿注入 + 忙等压力 `-race -count=25` | ok，0 失败 |
| 共享 fixture 回归面（`TestR91|Test2351|TestR95|Takeover|Execution` 全部相关测试）`-race -count=3` | ok 18.6s |
| **变异 `forceHover := false`（champselect.go:985，工单指定）** `-race -count=4` | **8/8 FAIL**，报错 `wildcard must hover first`（变异让 tick 的第一条写直接变锁，被新增的第 1 道断言最早抓住；旧版测试对该变异的表现是 `lock lost` / `different nonzero choice ignored`，同样 FAIL 但语义模糊） |

**验收判据核对**：压力下 `-count=100` 100% 通过 ✅（并加码：节流、停顿注入条件下亦 100%）；对照组复现失败率 ✅（以停顿注入形式，附本机统计性复现不成的如实说明）；forceHover 变异 FAIL ✅；根因、与 R120 P1-1 的关系、命令与前后对比 ✅（本台账 §1.1/§1.3）。

**与 R120 P1-1 的关系**：同一类「挂钟窗口被调度噪声吃掉」——R120 P1-1 是 1 秒墙钟预算 + 对后台请求落点的时序赌博，本条是 ≈2.5ms 写生命周期 + 对 pending map 的同步 peek；两者的修法也同构：把「赌时间」换成「确定性同步信号」（那边是完成计数 + 看门狗，这边是写冻结闸 + 同步诊断事件）。

### 1.4 顺手项（工单「已复核」节授权）

`backend/r122_autofill_diagnostic_test.go` 头注释（26-33 行）由「第 3 层只断言键存在、值恒为 0」更正为与实际代码一致的「断言与第 2 层同口径的真实数值（2/3/1）」，并保留旧口径为何不成立的演进记录（M7/M8 变异 + CUSTOM_GAME 手法）。纯注释，`go test -run TestR122` 通过。

---

## 2. 全量回归（真树，12:46–12:55）

| 检查 | 结果 |
|---|---|
| `go vet ./backend` | rc=0 |
| `gofmt`（本轮 3 个改动文件） | 无输出 |
| 整包 `go test -race -count=1 ./backend` 连续 2 次 | **ok 243.4s / 242.4s，0 个 `--- FAIL`** |
| Node | 本轮零 JS 改动，未重跑全量（昨晚基线 885/884/0/1 仍有效） |

⚠️ **并行会话遗留观察（非本轮文件，未动）**：`gofmt -l backend/*.go` 当前列出 `facade_banners.go`、`facade_icons.go`、`r105_test.go`、`r107_test.go`、`r108_test.go` 共 5 个文件——是 R121/R122/facade 会话的在写文件（vet/编译目前通过）。**若在其定稿前推送，CI 第一步 gofmt 门禁会红**，请留意。

## 3. 本轮改动文件清单（4 个，生产代码零改动）

| 文件 | 变化 | 内容 |
|---|---|---|
| `backend/champselect_execution_test.go` | +18 | fixture 新增可选 `patchGate`（ctx 感知的写冻结闸），默认 nil 零影响 |
| `backend/r91_addendum_test.go` | +63/-9 | 测试重写：悬停优先钉死 + 闸 + armed 事件顺序断言 + `r91LockArmedSeen` helper + `sync` import |
| `backend/r122_autofill_diagnostic_test.go` | +8/-6 | 仅头注释更正（工单顺手项） |
| `docs/r120-wildcard-hover-execution-ledger.md` | 新增 | 本台账 |

刻意未改：`backend/champselect.go`、`backend/champselect_execution.go`、`backend/champselect_takeover.go`、`.github/workflows/ci.yml`、`desktop/package.json`、并行会话的全部在写文件。变异与探针只发生在隔离副本（`$PI_SCRATCH_DIR/mut120c/`），还原一律基线拷回 + `diff -q`，未使用 `git checkout`。

## 4. 复现命令

```bash
cd /Users/ly/personal/personal-work/deep-legends
# 修复后验收
go test -race -count=100 -run 'TestR91AddendumWildcardHoverClearKeepsArmedLock' ./backend
GOMAXPROCS=1 go test -race -count=100 -run 'TestR91AddendumWildcardHoverClearKeepsArmedLock' ./backend
go test -race -count=3 -run 'TestR91|Test2351|TestR95|Takeover|Execution' ./backend
go test -race -count=1 ./backend   # ×2
# 对照/变异脚本（隔离快照 $PI_SCRATCH_DIR/mut120c/go，含 zz_busy_probe_test.go 忙等探针）：
#   旧测试 + 注入A/B（5ms Sleep）→ 三种签名 100% 复现
#   新测试 + 双 50ms 注入 → 全绿；champselect.go forceHover:=false → 8/8 wildcard must hover first
GOMAXPROCS=2 R120_BUSY_LOAD=2 go test -race -count=100 -run 'TestR91AddendumWildcardHoverClearKeepsArmedLock' ./backend
```

## 5. 仍待用户操作（沿用前两张工单）

R116 三项探测观测值回填、R119/R121/R122 补位真机对照、以及首次 push 后 GitHub runner 上「Real Chromium」步骤与 quality job 耗时的核对（`ubuntu-latest` 2 核共享跑者正是本工单随机红的高危环境，修复后首推应重点看这条测试）。
