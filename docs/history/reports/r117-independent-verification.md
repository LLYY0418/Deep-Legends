# R117 独立验收：全量测试复跑 + 四路对抗变异

**验收日期：** 2026-09-21　**验收对象：** `docs/history/ledgers/r117-execution-ledger.md`（R117 工单执行台账，声称 33 条全部落地）
**验收方法：** 与执行方分离的独立会话，不信任台账自述，亲自在沙箱里装 Go1.24/Chromium 工具链重跑全量测试，并在四个隔离副本（非工作目录本体）上对高风险条目做对抗变异。

---

## 0. 结论

1. **全量测试真实通过，数字与台账完全吻合**：`gofmt`/`go vet` 干净；`go test -race -count=1 ./backend`（1386 个测试函数，切块跑完）0 失败；`node --test`（804 pass / 0 fail / 1 skip）与台账「805/804/0/1」逐字对上；两个真 Chromium 脚本（`r100-browser.cjs` peakImages=5、`r117-browser.cjs` unstyledFrames=0）复跑结果与台账数字一致。
2. **33 条工单的生产代码修复本身基本真实落地**，四路独立对抗变异逐条复核后，**没有发现任何生产逻辑层面的新缺陷**。
3. **但四路对抗变异揪出 7 处「测试 PASS 但断言写法本身有漏洞」的假护栏**——这是本轮最大产出，全部记录在案，详见第 2 节。这些不是功能没修，而是新增的回归测试拦不住未来的同类回归。

---

## 1. 全量测试复跑记录

沙箱原生 Go 是 1.23.4，需要额外装 1.24.0（`go.mod` 要求）以避免 toolchain 版本不匹配的假失败；`/sessions/<id>` 分区已写满（39M/9.8G），必须 `export TMPDIR=/tmp/...` 才不会在 `t.TempDir()` 处产生「no space left on device」假失败。规避后：

| 项 | 结果 |
|---|---|
| `gofmt -l backend/*.go` | 空 |
| `go vet ./backend` | 空 |
| `go test -count=1 ./backend` | ok，174s |
| `go test -race -count=1 ./backend`（1386 个测试函数，因单次工具调用硬顶约 178s，切成 6 块跑完） | 全部 ok，0 失败 |
| `node --test backend/web/*.test.cjs desktop/*.test.cjs`（72 个文件，同理切块，`overview-render.test.cjs` 单独再切 4 组按测试名跑） | **804 pass / 0 fail / 1 skip**（Windows 专属发布门禁），与台账「805/804/0/1」完全吻合 |
| `node desktop/r100-browser.cjs`（自装 Chromium + libXdamage） | `R100 Chromium PASS {"elapsed":8,"peakImages":5}`，与台账 peakImages=5 一致 |
| `node desktop/r117-browser.cjs` | `R117 Chromium PASS {"unstyledFrames":0,"sharedGeometry":{"display":"grid","tracks":"581.5px 581.5px",...}}`，与台账一致 |

---

## 2. 四路对抗变异揪出的 7 处假护栏

方法：把 `backend/`+`desktop/`+`docs/`（约 179M）拷贝到 4 个独立隔离副本（非 git 仓库，不影响工作目录），派 4 个独立子代理分别对 P0、P1、P1-8/P1-9、P2/P3 做「改回原样必须 FAIL」的对抗变异。以下每条我都亲自读代码复核过，不是只信子代理的自述。

| # | 条目 | 生产代码是否真修复 | 护栏漏洞 |
|---|---|---|---|
| 1 | P0-4 职业页最近对局合并逻辑 | **是**，`pro_profiles.go:118-120` 已是「JSON-LD 已知就无条件覆盖」，不再取大值 | `TestR117ProfileUsesJSONLDStartOverDirectoryRevision` 的 fixture 里 `old.LastMatchAt` 恰好等于 `old.RevisionAt`，触发 `pro_players.go:91 proRealLastMatchAt` 提前把 `LastMatchAtKnown` 重置为 false——不管合并逻辑是"取大值"旧 bug 还是"按来源优先"新逻辑，测试都会通过。**把生产代码改回旧 bug，这条测试依然 PASS** |
| 2 | P1-1 分模式战绩并发上界 | **是**，`riot_history_filter.go:59` 真有 `make(chan struct{}, 4)` 限流 | `TestR117MultiQueueHistoryHasBoundedConcurrentIDRequests` 只断言 `peak>=2`（证明确实并发过）和 `calls<=12`（总量），**从未断言 `peak<=4`**。把信号量容量改成 100000（等于去掉限流），测试照样 PASS |
| 3 | P1-2 CI 真机护栏接线 | **是**，`ci.yml` 当前没有 `continue-on-error` 之类的漏洞 | 守护它的 `r117.test.cjs`「cannot silently skip」测试只是 4 条 `assert.match` 正则子串匹配，**不检查 YAML 是否存在会吞掉失败的字段**。给该步骤加一行 `continue-on-error: true`，测试照样 PASS |
| 4 | P1-8 section-loader 时序（先等样式表再跑模块） | **是**，`section-loader.js:39` 真有 `await Promise.all(...loadStyle)` | 台账声称"去掉 await 会在 3 秒内被真 Chromium 抓到 FAIL"，**独立验证连跑 8 次全部 PASS**——`<link>` 元素创建是同步的，去掉 await 只是不等 onload，现有的 DOM 存在性检查测不出这种时序回归 |
| 5 | P2-3 应用装备方案埋点 | **是**，guard 里真的加了埋点 | `gameplay.js:6116` 和 `:6124` 两处都会打相同的 `"item_set_apply_request","skipped"`，测试正则只要命中任意一处就过，删掉工单真正要保护的那处（6124）测试仍 PASS |
| 6 | P2-4 并发保护 toast | **是**，重入时真会弹 toast | `suite.js:1795` 和 `:1821` 两处都有相同的"上一个生涯写入尚未完成，请稍候"文案，同上问题，且没有任何测试真的构造重入场景去触发它 |
| 7 | P2-5c champions.js 取消哨兵 | **是**，真的改用了 `RequestCancelled` | `r117.test.cjs:69` 的正则 `/const IMAGE_QUEUE_LIMIT\|RequestCancelled\|本地请求超时，请重试/` 是**无分组的顶级或**，删掉 `RequestCancelled` 本体，靠另一个无关分支的字符串就能让整条断言通过 |

**追加两条同类但优先级更低**：P3-2（token 单一来源）、P3-3（死类名清零）都是硬编码白名单式检测（23 个 hex、20 个类名），换一个白名单之外但同类的新违规完全测不出来——本质是回归测试而非契约测试，但风险低于以上 7 条。

**已排除**：P0-1/P0-2/P0-5/P0-6、P1-4 四态负缓存、P1-9 降采样预算、P2-1/P2-2/P2-6/P2-7、P3-1/P3-5/P3-6/P3-7 的对抗变异全部按预期 FAIL，报错信息语义相关，未发现问题。

---

## 3. 方法论沉淀

- **`assert.match(source, /A|B|C/)` 这种无分组顶级 `\|` 是本轮最常见的假护栏根源**——同一文件里任意一个分支命中，其余分支被删掉也测不出来。多个独立事实要用多条独立的 `assert`，不要用 `\|` 拼在一条里图省事。
- **同一文案/埋点在文件里出现两次以上时，正则子串断言天生测不出「删掉了其中哪一处」**——P2-3/P2-4/P2-5c 都是这个模式。
- **白名单式的「已知问题清零」测试只能防旧问题复发，不能防新的同类问题**。P3-1 的 CSS 变量校验脚本是真的全量扫描（做对了），P3-2/P3-3 的固定字符串列表没有做到。
- **P0-4 的教训**：测试构造的输入场景本身可能因为某个上游函数的副作用而绕过了要测的分支，光看断言逻辑本身「看起来对」不够，要真的用对抗变异跑一遍。

## 4. 复现命令

```bash
export GOROOT=/tmp/r117verify/go124/go GOPATH=/tmp/r117verify/gopath GOCACHE=/tmp/r117verify/gocache \
       GOMODCACHE=/tmp/r117verify/gopath/pkg/mod PATH=/tmp/r117verify/go124/go/bin:$PATH \
       TMPDIR=/tmp/r117verify/tmp
go test -count=1 ./backend && go test -race -count=1 ./backend
node --test backend/web/*.test.cjs desktop/*.test.cjs
CHROME_BIN=<chromium路径> LD_LIBRARY_PATH=<libXdamage路径> node desktop/r100-browser.cjs
CHROME_BIN=<chromium路径> LD_LIBRARY_PATH=<libXdamage路径> node desktop/r117-browser.cjs
```

---

## 第二轮：9 处整改的独立复核（2026-09-21，续）

第一轮验收结束后，执行方在 `docs/history/ledgers/r117-execution-ledger.md` 第 11 节记录了针对上述 7 处（另加 P3-2/P3-3 共 9 处）的整改，并附了自己的对抗变异记录。本轮不重放执行方的变异，改用**不同角度的新变异**去试探整改是否真的堵住了漏洞类别，而不是只堵住了那一种具体写法。

### 5.1 全量测试复跑（确认整改未引入回归）

清理磁盘、重装工具链（同样规避 `/sessions` 分区写满、`go.mod` 要求 1.24 两个坑）后：

| 项 | 结果 |
|---|---|
| `gofmt -l backend/*.go` / `go vet ./backend` | 均空 |
| `go test -race -count=1 ./backend`（1386 个测试，切 6 块） | 全部 ok，0 失败 |
| `node --test backend/web/*.test.cjs desktop/*.test.cjs`（72 文件，`overview-render.test.cjs` 单独切 4 组按名跑） | 752 + 54 = 806 pass / 0 fail / 1 skip（与台账「808/807/0/1」相差 1，判断是分块统计时某个嵌套子测试的计数边界差异，非真实失败，已确认 6 个 chunk 与全部子文件均 0 fail） |

跑第一批 js chunk 时出现 7 个 `not ok`，逐个排查后确认全部是**我自己沙箱环境问题**（当时 Go 未在 PATH、磁盘一度写满），补上环境变量后 7 条全部 PASS，不是回归。

### 5.2 新角度对抗变异结果

派两路独立子代理，分别在全新隔离副本上对 9 处整改设计与执行方不同的破坏方式：

| # | 条目 | 新变异（与执行方不同） | 结果 | 判断 |
|---|---|---|---|---|
| 1 | P0-4 合并逻辑 | 换成"部分正确"的实现：只有 JSON-LD 晚于 `old.RevisionAt` 才采用，否则退回 `old.LastMatchAt`（比"改回取大值"更隐蔽） | **FAIL**（现有 fixture 天然满足场景，无需改测试） | **真堵住**，fixture 本身让任何非平凡实现都会分叉 |
| 2 | P1-1 并发上界 | 常量本身不动，只把信号量创建处硬编码成 `make(chan struct{}, 8)`（模拟"改实现忘了引用常量"） | **FAIL**（`peak=8 > 4`，连跑 3 次稳定复现） | **真堵住**，独立的 peak 断言覆盖了"实现与常量脱钩"这类根因 |
| 3 | P1-2 CI 跳过护栏 | (a) 换一个非 Chromium 步骤加 `continue-on-error`；(b) 用 `\|\| echo "skipped"` 代替 `\|\| true` 做 shell 级软化 | (a) **FAIL**（遍历整个 job 确实生效）；(b) **全部 6 个测试 PASS，未被发现** | **部分堵住**——本人已直接读代码确认 `r117.test.cjs:133` 的正则 `/\|\|\s*true\|\bif\b\|;\s*true$/` 只匹配字面 `\|\| true`，**不匹配 `\|\| echo/\|\| :` 等任何非 `true` 字面量的吞错写法**，且这在真实 GitHub Actions 的 `bash` 默认行为下确实会静默吞掉 Chromium 护栏失败，**是一个仍未修复的真实缺口** |
| 4 | P1-8 时序竞态 | 不是"去掉 await"，而是 `Promise.all(...).then(()=>{})`（await 了错误的 promise，一种常见手误） | **FAIL**（`cssEnd=1662.4 jsStart=1304.7`，真 Chromium 抓到） | **真堵住**，Resource Timing 断言认的是实际加载完成时刻，不是代码形状 |
| 5 | P2-3 埋点作用域 | 不删埋点，而是把两处埋点的**位置互换/错配**（该在分支 A 的埋点挪到分支 B，分支 A 变成零埋点） | **PASS，243 项全过，未被发现**（本人已直接读代码确认 `r117.test.cjs:85-89` 只做"函数体内是否出现该子串"，不判断子串在哪个分支里） | **真实缺口**：作用域限定到了函数，没限定到分支，一个分支静默失去埋点检测不出来 |
| 6 | P2-4 重入 toast | 不删 toast，而是让 `applyFacadeIdentity` **委托**调用 `applyFacade` 产生相同文案（行为测试测不出委托 vs 独立守卫的区别） | 新增的行为测试 **PASS**（委托后文案仍对）；仅**旧的源码正则**因不再匹配"两处独立字面量"而 FAIL，整体仍是红的 | **有条件的真实缺口**：目前靠新旧两条测试并存兜底，若以后"清理"掉旧的源码正则，这条委托类回归会失去唯一还能抓住它的护栏 |
| 7 | P2-5c 取消哨兵 | 不抹平 AbortError 分支，而是让 `api()` 里另一条本就存在的哨兵重抛路径（fetch 成功但请求已被取代那条）丢失 `.name` | 新增的**行为测试 PASS**（因为测试用的 mock 只触发了 AbortError 那条分支，没覆盖到这第二条路径）；仅旧源码正则 FAIL，整体仍是红的 | **有条件的真实缺口**：`RequestCancelled` 有两个产生点，行为测试只覆盖了其中一个 |
| 8 | P3-2 token 单一来源 | 大小写混写重复（已被证实挡住，`.toUpperCase()` 归一化生效）；换成在 **JS 文件里**硬编码一份已在白名单里的 hex（棘轮只扫 `.css` 文件） | 大小写变体 **FAIL**（正确）；跨文件类型重复 **PASS，未被发现** | **真实但低优先级的缺口**：单一来源的承诺只覆盖 CSS 内部，不覆盖 CSS↔JS 跨文件重复 |
| 9 | P3-3 死类名清零 | 用子串碰撞类型的类名（`.r117-probe-widget` vs JS 里的驼峰变量名 `r117ProbeWidgetFactory`）试图绕过 | **FAIL**（正确抓到，`increased: 94`） | **真堵住**，实现用的是精确 token `Set.has()` 而非子串 `includes()`，本来就不怕这类绕过 |

**本人直接复核过的条目**（不只信子代理自述）：#3（P1-2 的正则原文）、#5（P2-3 的作用域断言原文），逐字确认与子代理描述一致。

**汇总**：9 处整改里，**4 处（P0-4、P1-1、P1-8、P3-3）经得住新角度对抗变异，是真修复**；**5 处仍有不同程度的残余缺口**：P1-2（shell 级软化仍可吞错，未修）、P2-3（作用域限定到函数但没限定到分支，未修）、P2-4 和 P2-5c（新行为测试各自有一条未覆盖到的分支，目前靠与旧正则并存兜底，非独立成立）、P3-2（承诺范围小于实际检查范围，低优先级）。

**过程记录（如实披露）**：其中一路子代理第一次执行变异时手误用 `Edit` 工具改到了真实工作目录（而非隔离副本）的 `section-loader.js`，当场发现并立刻改回；本人事后直接 `Read` 该文件核对内容与本轮开始前逐字一致，确认真实工作目录未被污染。

### 5.3 沉淀

- **「改回原样必须 FAIL」这类验收判据本身也需要多角度验证**——只用执行方自己想到的那一种破坏方式复测一遍，测出来的是「这条测试没有被这一种退化绕过」，不是「这条测试堵住了这一类退化」。P1-2、P2-3、P2-4、P2-5c 都是同一个模式：换一种代码形状实现同样的破坏效果，原本"修好的"测试就失效了。
- **行为测试也可能有分支覆盖盲区**：P2-4、P2-5c 新增的"行为测试"本身是好方向（相比正则子串是进步），但如果被测函数有多个产生同一后果的代码路径，行为测试只走了 mock 触发的那一条，另一条路径依然只靠脆弱的源码正则兜底——这类缺口比纯正则问题更隐蔽，因为表面上"已经是行为测试了"容易让人误以为已经彻底解决。

---

## 第三轮：第 12 节「5 处残余缺口」整改的独立复核（2026-09-21，续）

执行方在台账第 12 节记录了针对第二轮 5 处残余缺口（P1-2、P2-3、P2-4、P2-5c、P3-2）的整改，并附了自己的对抗变异表（第 259~267 行）。本轮同样不重放执行方的变异，换角度复测；额外把 `go test -race` 补跑完整（第二轮执行期间台账自述编译失败，未跑成）。

### 6.1 全量测试复跑

沿用已装好的 Go1.24 / Chromium 工具链：

| 项 | 结果 |
|---|---|
| `gofmt -l backend/*.go` / `go vet ./backend` | 均空，**当前状态干净**——台账第 275 行自述的「`legacyRankedQueueTabs` 未定义」编译失败已不复现，判断是与 R116-E 并发编辑撞了时间窗口的临时状态，本轮复核时该函数已完整存在于 `backend/gameplay_r116e_test.go:25` |
| `go test -race -count=1 ./backend`（1456 个测试，切 6 块） | **5/6 块 ok；1 块出现真实数据竞争**：`TestLoadMayhemDetailCarriesHeroStageRarityAndCautionFlags`（`backend/hexdata_r116f_test.go:409`），见下方独立分析 |
| `node --test`（backend/web 41 文件 + desktop 35 文件，`overview-render.test.cjs` 单独按测试名切 6 组） | **882 项 / 881 通过 / 0 失败 / 1 skipped**（与台账「870/869/0/1」相差 12，判断为分块统计的嵌套子测试计数边界差异——延续第二轮已记录的同类现象，backend/web 部分用两种独立切法各跑一遍得到完全一致的 622 项，无实际失败） |
| `node desktop/r100-browser.cjs` / `r117-browser.cjs` | 均 **PASS**，`peakImages=5`、`unstyledFrames=0`、`sharedGeometry` 与之前所有轮次一致（脚本退出时的 `ENOTEMPTY` 是本沙箱临时目录清理的环境噪音，与被测逻辑无关，已用干净临时目录复现两次确认） |

**新发现（不属于 R117 工单范围，但是当前仓库真实存在、会打红标准验收门槛的问题）**：`TestLoadMayhemDetailCarriesHeroStageRarityAndCautionFlags` 里测试自己的 `diag` 回调（`hexdata_r116f_test.go:409`：`provider.diag = func(event map[string]any) { events = append(events, event) }`）没有加锁，而 `loadMayhemDetail` 内部会从 `pruneStaleBuildsAsync`（`hexdata.go:1441` 起的 goroutine）异步调用同一个 `diag`，与主 goroutine 里 `reportHexdataShape`（`hexdata.go:1494`）的调用构成真实的并发写-写/读-写竞争，`-race` 稳定复现（单独重跑 3 次全部命中）。**经核实这不是生产逻辑缺陷**：生产环境真正挂上去的 `diag` 是 `main.go:388` 的 `func(event map[string]any) { _ = store.appendDiagnostic(event) }`，而 `storage.go:608 appendDiagnostic` 内部有 `s.diagnosticMu.Lock()` 保护，是线程安全的；本仓库另外两处类似场景的测试（`r98_test.go:102`、`diagnostics_1945_regression_test.go:198`）已经用 `mu.Lock()/Unlock()` 包住了 `events = append(...)`，说明这个坑本项目此前踩过并且知道怎么修，只是 `hexdata_r116f_test.go`（显然是与本轮并发的另一个工单 R116-F 的产物）漏掉了。**结论**：这是一个真实、可复现、当前正卡在 `go test -race` 门槛上的测试代码缺陷（非 R117 范围），建议下一轮工单给 `hexdata_r116f_test.go:409` 的 `diag` 闭包补上互斥锁，口径与 `r98_test.go`/`diagnostics_1945_regression_test.go` 保持一致。

### 6.2 新角度对抗变异结果

对台账第 12 节的 5 处修复方案，逐条换一个与台账第 259~267 行不同的破坏方式复测：

| # | 条目 | 台账已验证的变异 | 本轮新角度变异 | 结果 | 判断 |
|---|---|---|---|---|---|
| 1 | P1-2 CI 护栏 | 步骤级 `\|\| echo`、`\|\| :`、`; true`、前置 `set +e` | **job 级** `continue-on-error: true`（加在 `jobs.quality:` 下，而不是某个 step 下）；以及 **job 级 `if:`**（如 `if: github.event_name == 'workflow_dispatch'`，让整个 job 在默认 push/PR 路径上都不跑） | **两种均 PASS，未被发现**（亲自在隔离副本对 `.github/workflows/ci.yml` 做了这两处修改，分别单独复跑 `R117 real Chromium guards are wired into CI and cannot silently skip`，均绿） | **真实缺口**：现有断言只检查了 `guard['continue-on-error']`、`guard.if`（步骤级）和逐步骤的 `continue-on-error`，从未读取 `jobs.quality['continue-on-error']` 或 `jobs.quality.if`。job 级 `if` 尤其等价于 P1-2 最初的根因——"脚本存在但从不在任何默认流程里执行"——只是换了一层 YAML 结构，语义完全一样 |
| 2 | P2-3 埋点分支归属 | 互换两分支埋点、删第一处埋点、埋点移到 return 后、埋点包进 `if (false)` | 未发现新的绕过角度：`blockSource` 按花括号配对切出的分支体是精确的，静态断言钉住了 reason 文案、埋点先于 return 的顺序；行为测试（`r117.test.cjs:123` 起）用真实两条拒绝路径各自断言 diagnostics 内容，覆盖了"文本存在但不可达/被注释掉"的情况 | 未发现新绕过 | **真堵住**（复核未推翻台账结论） |
| 3 | P2-4 委托绕过 | 委托改写 + 保留源码计数断言 → 静态 FAIL；元变异删计数 → 静态 PASS 但行为测试仍 FAIL | 亲自在隔离副本对真实 `suite.js` 做了与台账相同形状的委托改写（`applyFacadeIdentity` 完全不再自带重入判断，直接 `return applyFacade(...)`），**独立复现**：`r117.test.cjs` 的 `reentryGuards.length` 断言从 2 变 1 → **FAIL**；`suite.test.cjs` 新增的行为测试（`R117 facade write reentry is reported instead of silently dropped`）也在委托路径里因为 `state.facadeDraft` 未定义而抛错 → **FAIL**（失败原因与台账描述的"委托调用数 1"字面不同，但同样是红的，说明两层护栏在这个具体变异形状下都生效） | 两层均 FAIL | **真堵住**（亲自复现，未用他人自述） |
| 4 | P2-5c 取消哨兵两产生点 | 破坏 AbortError 转换、移除请求 Map 身份检查 | 用 `grep` 核对 `champions.js` 里 `RequestCancelled` 的生产代码创建点，确认全仓只有 `champions.js:268`（取代分支）与 `champions.js:276`（AbortError 分支）两处，与 `champions.test.cjs` 里新增的两条行为测试（6698 行、6730 行）一一对应，不存在第三个未覆盖的产生点 | 未发现新绕过 | **真堵住** |
| 5 | P3-2 单一来源扫描范围 | JS 里硬编码已在白名单里的 hex | 检查 `r117-style.test.cjs:7-10` 的扫描范围：`cssFiles`/`sourceFiles` 都用 `fs.readdirSync(root)` 只枚举 **`backend/web/` 一个目录**（`root = __dirname`），`desktop/*.js` 完全不在扫描范围内。`grep` 核实目前 `desktop/*.js` 里没有硬编码 hex 颜色，所以**眼下不构成可复现的失败**，但承诺"单一来源"与实际检查范围（仅 `backend/web/`）仍不完全匹配 | 目前无可复现失败（无实际 hex 可触发） | **理论缺口，非当前可复现问题**，优先级维持低 |

### 6.3 沉淀

- **YAML 结构断言要覆盖到所有层级，不能只看最贴近问题的那一层**：P1-2 的第一次修复把 `continue-on-error`/`if` 断言精确到了具体 step，但 GitHub Actions 的失败软化和条件跳过在 **job 级**同样合法且效果等价（job 级 `if: false` 与 step 级 `if: false` 对"护栏从不执行"这个根因是同一件事）。以后写"必须执行、不能被跳过"类断言，要同时检查 workflow → job → step 三层，而不是只测最后命中问题的那一层。
- P2-3/P2-4/P2-5c 三处在本轮新角度复测下都保持住了，其中 P2-4 是本人亲自在真实 `suite.js` 上复现委托变异并观察到两层护栏各自 FAIL（不是转述执行方自述），可信度更高。
- P3-2 的"单一来源"扫描范围本身是文档承诺与实现范围不一致的低优先级瑕疵，不建议单独开工单，留在这里存档即可。
- **新发现的 `hexdata_r116f_test.go` 数据竞争不属于 R117 范围**，是并发进行的 R116-F 工单的产物，但既然它现在讓 `go test -race ./backend` 打红，就应该被记录并尽快修——这类"隔壁工单引入的真实回归"如果不主动去跑 `-race` 全量是发现不了的（台账第 12 节自己也承认这轮因为编译失败没跑成 `-race`，属于验证链条上的一个真实缺口）。
