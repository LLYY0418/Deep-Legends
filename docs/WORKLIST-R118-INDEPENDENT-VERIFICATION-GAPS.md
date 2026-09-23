# WORKLIST-R118：R117 独立验收三轮遗留的真实缺口

**撰写日期：** 2026-09-21　**基线版本：** 0.12.12（`desktop/package.json`）
**触发：** R117 工单（`docs/WORKLIST-R117-FULL-PROJECT-OPTIMIZATION.md`）执行完成后，独立会话做了三轮验收（`docs/r117-independent-verification.md`）：第一轮抓到 7 处假护栏，第二轮复测 9 处整改中 5 处仍有缺口，第三轮复测第三轮整改后确认 P2-3/P2-4/P2-5c 已真正堵住，但换角度复测又在 P1-2 发现一个新形状的真实缺口，并顺带用 `go test -race` 全量跑出一个当前仓库里真实存在的数据竞争。
**本轮性质：** 纯审查延续，未改动任何文件。以下每条都带证据与复现步骤，可独立复核。
**范围说明**：本工单只收敛三轮验收里**至今仍未修复**的条目。已确认真正堵住的 P0-4 / P1-1 / P1-8 / P3-3 / P2-3 / P2-4 / P2-5c 不再重复列出，详见 `docs/r117-independent-verification.md` 各轮章节。

## 0. 三句话结论

1. **P1-2 的 CI 护栏仍能被绕过，只是换了一层 YAML 结构**：现有断言把 `continue-on-error`/`if` 精确检查到了 step 级，但从未检查 **job 级**，而 job 级的这两个字段能达到完全相同的「护栏从不在默认流程里执行」效果——这正是 P1-2 最初的根因。
2. **`go test -race ./backend` 当前会真实打红**：`backend/hexdata_r116f_test.go:409` 的测试自身有一处未加锁的并发访问，与生产代码的异步剪枝 goroutine 构成真实数据竞争。已核实生产代码本身线程安全，纯粹是测试代码的疏漏，但只要没人专门带 `-race` 跑这个文件就会一直红着。
3. **P3-2 的「单一来源」检查范围小于承诺范围**：扫描只覆盖 `backend/web/` 目录，`desktop/*.js` 完全不在检查范围内；目前没有实际重复的 hex 触发失败，属于理论缺口，优先级最低。

---

## 执行纪律（延续 R117）

- 每项的「验收判据」都必须做**对抗变异**：把修复点改回原样，或换一种能达到同等效果但语法不同的写法，对应测试必须 FAIL。只跑现有测试 PASS 不算验收。
- **改回原样测试只证明没被这一种退化绕过，不等于堵住了这一类退化**——这是本轮三次验收共同的教训。验收时应主动设计与执行方自述变异不同形状的破坏方式。
- 变异测试一律在隔离副本（`/tmp/...`，非 git 工作目录）里做，禁止用 `git checkout -- <file>` 做任何还原（R117 执行事故的教训）。

---

# P1 — CI 真机护栏仍可被绕过

## P1-2（续）　CI 真机护栏的 job 级失败软化/跳过完全不在检查范围内

**证据**

`backend/web/r117.test.cjs:160-196`（测试名 `R117 real Chromium guards are wired into CI and cannot silently skip`）用 `js-yaml` 结构化解析 `.github/workflows/ci.yml`，检查的对象全部是 **step 级**字段：

```js
const guard = steps.find((step) => step?.name === 'Real Chromium image-queue and lazy-CSS guards');
assert.equal(guard['continue-on-error'], undefined, ...);
assert.equal(guard.if, undefined, ...);
for (const step of steps) {
  assert.notEqual(step['continue-on-error'], true, ...);
}
```

`guard`/`steps` 都是从 `yaml.load(ci).jobs.quality.steps` 里取出来的，测试从未读取 `yaml.load(ci).jobs.quality['continue-on-error']` 或 `yaml.load(ci).jobs.quality.if`。

**复现（已亲自验证，隔离副本操作，未改动真实工作目录）**

在 `.github/workflows/ci.yml` 的 `jobs.quality:` 下面加一行（与 `runs-on: ubuntu-latest` 同级）：

```yaml
jobs:
  quality:
    continue-on-error: true    # ← 加这一行
    runs-on: ubuntu-latest
    steps: [...]
```

或者：

```yaml
jobs:
  quality:
    if: github.event_name == 'workflow_dispatch'    # ← 加这一行
    runs-on: ubuntu-latest
    steps: [...]
```

两种改法分别单独复跑 `node --test --test-name-pattern="real Chromium guards are wired" backend/web/r117.test.cjs`，**均 PASS**。

**影响**：`continue-on-error: true` 加在 job 级会让整个 job（包括真 Chromium 护栏步骤）的失败不再影响 workflow 的整体结果；`if:` 加在 job 级会让整个 job 在默认的 push/PR 路径上直接不执行——这与 P1-2 最初的根因「脚本存在但从不在任何流程里执行，护栏失效了半年没人发现」是同一件事，只是换了一层 YAML 结构，绕开了当前只查 step 级的检查。

**修复方向**：在既有测试里补两条断言：
1. `assert.equal(yaml.load(ci).jobs.quality['continue-on-error'], undefined, 'job 级 continue-on-error 会让整个 job 的失败被吞掉')`
2. `assert.equal(yaml.load(ci).jobs.quality.if, undefined, 'job 级 if 会让整个 job 在默认路径上被跳过')`

如果项目里还有其它 job 也依赖于「必须执行、不能被跳过」的语义（例如发布门禁 job），建议顺带审计一遍同样的 job 级字段，不要只补 `quality` 一个。

**验收判据**：分别加回 `jobs.quality.continue-on-error: true` 与 `jobs.quality.if: <永假或仅 workflow_dispatch 的条件>` 两种变异，`R117 real Chromium guards are wired into CI and cannot silently skip` 必须 FAIL。同时保留现有 step 级的 4 种变异回归（`|| echo`、`|| :`、`; true`、前置 `set +e`）不能因为这次修改而失效。

---

# P2 — 测试基础设施的真实缺陷（非本轮工单范围引入，但当前会打红标准验收门槛）

## P2-8　`hexdata_r116f_test.go` 的诊断回调未加锁，`go test -race` 稳定复现数据竞争

**证据**

`backend/hexdata_r116f_test.go:405-413`（`TestLoadMayhemDetailCarriesHeroStageRarityAndCautionFlags`）：

```go
events := make([]map[string]any, 0, 8)
provider.diag = func(event map[string]any) { events = append(events, event) }
response, err := provider.loadMayhemDetail(context.Background(), "157")
```

`loadMayhemDetail` 内部（`backend/hexdata.go:3294` → `loadHexdataMeta` → `adoptMeta` → `backend/hexdata.go:1441-1452` 的 `pruneStaleBuildsAsync`）会在**后台 goroutine** 里异步调用同一个 `provider.diag`：

```go
go func() {
    defer h.pruneWait.Done()
    removed, err := h.provider.cache.pruneStaleHexdataBuildsCount(buildID)
    ...
    h.provider.diag(event)   // ← 与主 goroutine 的 reportHexdataShape 并发写同一个 events 切片
}()
```

主 goroutine 在 `backend/hexdata.go:1492-1495`（`reportHexdataShape`）同样会调用 `p.diag(...)`，与上面的后台 goroutine 并发命中同一个未加锁的闭包。

**复现**

```bash
go test -race -count=1 -run '^TestLoadMayhemDetailCarriesHeroStageRarityAndCautionFlags$' ./backend
```

稳定复现 `WARNING: DATA RACE`（本轮独立验收连跑 3 次全部命中，第三方复核跑全量 1456 个测试切 6 块时命中在其中一块）。

**核实生产代码本身没有问题**：真实挂载的 diag 回调（`backend/main.go:388`）最终落到 `backend/storage.go:608 (*localStore).appendDiagnostic`，内部有 `s.diagnosticMu.Lock()` 保护，是线程安全的。本项目此前已经在另外两处遇到过同样的坑并且修对了——`backend/r98_test.go:102` 与 `backend/diagnostics_1945_regression_test.go:198` 的测试闭包都用 `mu.Lock()/Unlock()` 包住了 `events = append(...)`，`hexdata_r116f_test.go` 只是漏做了同样的处理。

**影响**：只要有人在本地或 CI 带 `-race` 跑 `./backend`（本项目的标准验收门槛一直是 `go test -race`），就会遇到这条假阳性失败，容易被误判成「刚改的代码引入了并发 bug」而浪费排查时间；也会掩盖真正的并发回归（如果同一个测试文件将来真的引入了生产层面的竞争，这个假阳性会混在噪音里更难分辨）。

**修复方向**：比照 `r98_test.go:102` 与 `diagnostics_1945_regression_test.go:198` 的写法，给 `hexdata_r116f_test.go:409` 的 `diag` 闭包加锁：

```go
var mu sync.Mutex
events := make([]map[string]any, 0, 8)
provider.diag = func(event map[string]any) { mu.Lock(); defer mu.Unlock(); events = append(events, event) }
```

注意该测试文件后续如果有读取 `events` 做断言的地方，也要确保发生在所有相关 goroutine（含 `pruneStaleBuildsAsync` 启动的那个）已经通过 `h.pruneWait.Wait()` 或等价方式汇合之后，否则加锁只能消除数据竞争，不能保证断言时机的确定性。

**验收判据**：`go test -race -count=1 -run '^TestLoadMayhemDetailCarriesHeroStageRarityAndCautionFlags$' ./backend` 连续跑 5 次全部无 `DATA RACE` 输出；`go test -race -count=1 ./backend` 全量跑通过（不切块，一次性跑完，避免分块掩盖偶发竞争）。

---

# P3 — 低优先级 / 理论缺口

## P3-9　设计 token 单一来源检查范围小于承诺范围（`desktop/*.js` 不在扫描内）

**证据**

`backend/web/r117-style.test.cjs:6-11`：

```js
const root = __dirname;   // = backend/web/
const cssFiles = fs.readdirSync(root).filter((name) => name.endsWith(".css"));
const sourceFiles = fs.readdirSync(root).filter((name) => /\.(?:js|html)$/.test(name) && !name.endsWith(".test.cjs"));
```

`fs.readdirSync(root)` 只枚举 `backend/web/` 一个目录，不递归、也不包含 `desktop/` 目录下的任何 `.js` 文件。测试名「R117 token colors have one source definition and no bypass drift」暗示的承诺是全仓单一来源，但实际检查范围只有 `backend/web/`。

**当前状态**：已用 `grep` 核实 `desktop/*.js` 目前没有硬编码的 6 位/3 位 hex 颜色字面量，所以这不是一个当前会失败的测试，纯粹是检查范围与命名/注释承诺不一致。

**修复方向**（可选，低优先级）：如果 `desktop/` 目录以后会引入界面相关的颜色常量（例如托盘图标、原生菜单主题色），再把 `sourceFiles`/`cssFiles` 的枚举范围从 `backend/web/` 扩展到项目里所有非测试的 `.js`/`.css`/`.html`（用 `fs.readdirSync(..., {recursive: true})` 或显式列出 `desktop/` 目录）。在此之前不建议单独开工，留在本工单存档即可，避免为了一个目前不存在的问题预先扩大扫描面导致新的假阳性风险。

**验收判据**（如果决定修）：在 `desktop/` 任意一个 `.js` 文件里硬编码一个已在 `tokenHexes` 白名单里的 hex 值，扩大范围后的测试必须 FAIL；扩大前应先跑一遍确认当前不会因为新纳入的文件而产生任何新的重复 hex（避免刚扩大范围就撞上历史遗留的重复，需要一次性排查清楚再收紧阈值）。

---

## 复现环境备注

三轮验收使用的沙箱工具链（Go 1.24.0 原生二进制、Playwright Chromium 1148 + `libXdamage1`、`TMPDIR` 指向非系统分区）与复现命令记录在 `docs/r117-independent-verification.md` 第 4 节与第三轮 `6.1` 小节，此处不重复。
