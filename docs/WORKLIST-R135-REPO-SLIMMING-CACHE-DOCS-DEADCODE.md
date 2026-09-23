# R135 · 仓库瘦身：本机缓存、Git 历史、docs 工单归档与无引用代码清理

- 编号：R135（本工单一个文件、按 P0–P6 分节；执行账本写 `docs/r135-execution-ledger.md`）
- 目标：在**不改变任何现有功能、界面文案、测试覆盖和打包产物**的前提下，降低仓库目录占用，并把 docs/ 里越堆越多的工单、账本、验证证据理顺。
- 执行者：GPT（Codex），在用户 Mac 本机仓库根目录执行。
- 红线：
  1. 不改任何产品行为、UI 文案（遵守 CLAUDE.md「界面文案红线」）、接口、数据源、`desktop/package.json` 的 `build.files`。
  2. **禁止 `git clean -fdx` / `git clean -fd`**。当前工作区有 103 个未跟踪文件，其中大量是在用的源码和测试（如 `backend/match_tier_cache.go`、`backend/season_stats_budget.go`、`backend/web/favorites-facade.js`、`backend/r126_test.go`…），一清就丢。
  3. 不删除、不合并任何 `*_test.go` / `*.test.cjs` 回归测试（按 R 号命名的测试文件都是护栏，数量多但不是冗余）。唯一例外：P5 中随被删死代码一起失效的测试，必须逐条记入账本。
  4. `docs/pro-accounts-verification-2026-09-17.md` 原样保留（CLAUDE.md 规定的唯一归属来源）。
  5. P2（改写 Git 历史 + 强推）**默认不执行**，必须用户在对话里明确回复同意后才做。

## 现状测量（2026-09-23，本机）

仓库目录合计 **13 GB**，其中 Git 跟踪的文件只有约 56 MB：

| 路径 | 大小 | 是否跟踪 | 性质 |
|---|---|---|---|
| `.gocache/` | **12 GB** | 否（已 ignore） | Go 构建缓存，`build-desktop.sh` 默认把 `GOCACHE` 指到仓库内，所以一直涨 |
| `.git/` | 607 MB | — | 历史里提交过 `.gotoolchain/`（完整 Go 工具链）、`.tmpbuild/`、`deep-legends.exe`、`deep-legends/__TEXT__*` 等二进制，且已推到 `origin/main` |
| `desktop/node_modules/` | 133 MB | 否 | 测试依赖（jsdom/electron），**保留** |
| `dist/` | 98 MB | 否 | 打包产物，可重建 |
| `Claude outputs/` | 54 MB | 否（未 ignore） | 历次会话导出：`r132-src.tgz` 17 MB、`r121-position-probe-v2.zip` 11 MB、`r121-position-probe-v2/` 27 MB |
| `docs/` | 50 MB | 是 | 其中 19 个 `rNN-validation` / `rNN` 证据目录占大头（r98 8.6M、r100 8.0M、r101 7.2M、r88 4.0M、design-mockups 3.9M…） |
| `lol-loot-assistant` | 24 MB | 否 | 旧项目名的旧二进制（9 月 19 日） |
| `.gopath/` | 23 MB | 否 | 旧验证用 GOPATH |
| `backend/backend` | 22 MB | **否且未 ignore** | `go build ./backend` 默认产物，会被误提交 |
| `.gomodcache/` | 21 MB | 否 | 旧沙箱模块缓存 |
| `desktop/backend/loot-service.exe` | 18 MB | 否 | `build-desktop.sh` 第 64–66 行产物，打包时重建 |
| `.r90env` / `.r90env.sh` | — | 否 | 指向已不存在的 `/sessions/...` 沙箱路径 |
| `.git` 垃圾 | 1.7 MB | — | `git count-objects` 报 3 个 garbage（tmp_obj、孤立 .mtimes） |

另外：工作区有 120 个已修改 + 1 个删除 + 103 个未跟踪文件尚未提交；CLAUDE.md 写「当前版本 0.12.7」，而 `desktop/package.json` 已是 0.12.18。

---

## P0 · 基线（必须先做，否则后面的任何删除都不许开始）

1. `git status --short > docs/r135-validation/baseline-status.txt`，记录当前 120 M / 1 D / 103 ?? 的完整列表。
2. **先把现有改动落一个基线提交**（不含 `backend/backend`、`Claude outputs/`、`.r90env*`）：先按 P1-4 把 ignore 规则补上，再 `git add -A && git commit -m "chore: R135 baseline before repo slimming"`。提交前 `git status` 复核，确认没有把 >1 MB 的二进制加进去（`git diff --cached --stat` + `git diff --cached --numstat | sort -k1 -rn | head`）。
   - 如果用户有不想提交的在制改动，停下来在账本里列出并询问，不要自作主张 stash/丢弃。
3. 跑一遍完整基线并把结果（通过数 / 跳过数 / 失败数 / 耗时）写进账本：
   ```bash
   go build ./backend && go vet ./... && go test -count=1 ./...
   (cd installer && go test ./... && go vet ./...)
   npm ci --prefix desktop   # 若 node_modules 已在可跳过
   node --test backend/web/*.test.cjs desktop/*.test.cjs scripts/*.test.cjs
   CHROME_BIN="<本机 Chrome>" R100_BROWSER_OUTPUT=/tmp/r135-r100 node desktop/r100-browser.cjs
   CHROME_BIN="<本机 Chrome>" R117_BROWSER_OUTPUT=/tmp/r135-r117 node desktop/r117-browser.cjs
   ```
   基线里如已有失败项，原样记下，**不在本工单里修**（另开新 R 号）。
4. `du -sh .[!.]* * | sort -rh` 输出存为 `docs/r135-validation/size-before.txt`。

## P1 · 本机缓存与构建产物（不涉及 Git 跟踪文件，预计释放约 12.3 GB）

1. 删除可重建缓存与产物：
   ```bash
   rm -rf .gocache .gocache-* .gopath .gomodcache .mut102gopath .gotoolchain .tmpbuild
   rm -f  backend/backend lol-loot-assistant .r90env .r90env.sh
   rm -rf dist
   find . -name .DS_Store -not -path './.git/*' -not -path './desktop/node_modules/*' -delete
   ```
   `desktop/backend/loot-service.exe` 可删（打包时由 `build-desktop.sh` 重建），但删后 P6 必须真跑一次后端构建阶段确认能再生成。`desktop/node_modules/` **不删**。
2. `Claude outputs/`：属于用户的会话导出，**不要 `rm`**，移到废纸篓（`mv "Claude outputs" ~/.Trash/deep-legends-claude-outputs-$(date +%m%d)`），账本注明位置，由用户决定是否清空。
3. 修复缓存回涨的根源——`build-desktop.sh` 第 16、57、64、177、185 行的默认值 `$project_root/.gocache` 改为系统默认缓存：
   ```bash
   export GOCACHE="${GOCACHE:-$(go env GOCACHE)}"
   ```
   其余四处的 `${GOCACHE:-$project_root/.gocache}` 统一改成 `$GOCACHE`（已在第 16 行 export）。只改默认路径，不动构建参数、`-trimpath`、指纹校验等任何其他逻辑。`build-desktop-windows.ps1` / `build-windows.ps1` 若有同类仓库内缓存默认值，同样处理。
4. `.gitignore` 补充（保留现有全部规则）：
   ```gitignore
   # R135：构建产物与会话导出
   /backend/backend
   /backend/backend.exe
   /Claude outputs/
   /.gomodcache/
   ```
5. 清理 Git 本地垃圾（不改历史）：`git gc --prune=now`，记录前后 `git count-objects -vH`。

## P2 · Git 历史瘦身（约 600 MB → 预计 <60 MB；**默认跳过，需用户明确同意**）

历史中提交过以下与产品无关的路径（`git log --all -- <path>` 可见，最早在 `ff3cf27c` / `7eb6393c` / `b62bca1b` 等提交），已推到 `origin/main`：
`.gotoolchain/`、`.tmpbuild/`、`deep-legends.exe`、`deep-legends/`（Mach-O 段拆出的 `__TEXT__*` 等文件）。

用户同意后才执行：
1. 先做完整镜像备份：`git clone --mirror . ../deep-legends-backup-$(date +%m%d).git`。
2. `brew install git-filter-repo`，然后
   `git filter-repo --invert-paths --path .gotoolchain/ --path .tmpbuild/ --path deep-legends.exe --path deep-legends/`
   （执行前用 `git ls-files | grep -E '^(\.gotoolchain|\.tmpbuild|deep-legends/|deep-legends\.exe$)'` 确认当前 HEAD 不跟踪这些路径，已核实为 0 个。）
3. 比对：HEAD 的 `git ls-files` 列表与改写前完全一致，`git diff <旧HEAD> HEAD --stat` 为空（树相同，只是哈希变）。
4. `git remote add origin <原地址>`（filter-repo 会移除 remote）后 `git push --force-with-lease origin main`。
5. 账本注明：历史哈希全部变化，旧文档里引用的提交号失效；其他机器上的克隆需重新 clone。

用户未同意时：本节整体跳过，账本写「P2 未执行，待用户决定」。

## P3 · 工单与账本的归档规则（解决「工单太多」）

现状：`docs/` 根目录 144 项，其中 19 份 `WORKLIST-R117…R134`、约 50 份 `rNN-execution-ledger.md` / 核查报告、19 个证据目录；`docs/history/worklists/` 已有 R86–R116 的 36 份。

**以后固定的规则**（写进 CLAUDE.md 和 AGENTS.md「目录布局」段，两处一致）：
- `docs/` 根目录只放：进行中的 `WORKLIST-*`、它们对应的账本、长期参考文档（见下方保留清单）。
- 工单关闭（存在对应执行账本且账本结论为完成）后，工单移到 `docs/history/worklists/`，账本移到 `docs/history/ledgers/`，核查报告/提案/探测结论移到 `docs/history/reports/`。
- 新建 `docs/WORKLIST-INDEX.md`：一张表列出所有 R 号 → 标题、状态（进行中 / 已关闭 / 未执行）、工单路径、账本路径、证据所在提交号。以后查工单先看这张表。

本次执行：
1. 逐份判断 `docs/WORKLIST-R117…R134` 的状态：有同号执行账本（如 `r117-execution-ledger.md`、`r120-*-execution-ledger.md`、`r130-execution-ledger.md`、`r133-execution-ledger.md`）且结论为完成的 → 已关闭；R125 由 `r123`/`r130` 账本覆盖的按账本结论判断；**R128、R129、R131、R134 未找到对应账本，视为进行中，原地保留**。拿不准的一律保留。
2. 已关闭的工单、账本、报告用 `git mv` 移入上面三个 history 子目录（`git mv` 保留历史）。
3. **被代码或测试引用的 docs 文件不许移动或删除**，除非同一提交里把引用路径改掉并让测试通过。当前已知引用（`grep -rnoE "docs/[A-Za-z0-9_./-]+" backend desktop scripts .github build-desktop.sh installer`）：
   - 必须原地保留：`docs/pro-accounts-verification-2026-09-17.md`（`r102_test.go`、`web/r105.test.cjs` 读取）、`docs/pro-players-sources.md`、`docs/r89-kr-player/`（`scripts/r133-stall-log-report.test.cjs`）。
   - 仅出现在代码注释里的：`r116-probe-findings.md`、`r116b/d/e/f-execution-ledger.md`、`r119-execution-ledger.md`、`r121-position-probe-findings.md`、`r130-execution-ledger.md`、`WORKLIST-R119-AUTOFILL-LABEL-GAPS.md`、`docs/history/...`。可以移动，但必须同一提交里用 `sed` 更新注释路径（只改注释，不改代码）。
   - 浏览器脚本的输出目录（`r100-validation/browser`、`r117-validation/browser` 等）：确认脚本会 `mkdir -p` 再写；不会的话保留空目录或改用环境变量默认值，不许让 CI 的两条浏览器护栏失败。
4. 更新 `docs/assistant-conversation-digest.md` 末尾「原始记录查阅」与 CLAUDE.md 中若有被移动文件的路径，同步修正。

## P4 · docs 验证证据瘦身（跟踪文件，预计 docs/ 50 MB → 约 10 MB）

证据截图、HAR、原始 JSON 在工单关闭后不再需要常驻工作区，Git 历史里永远可取回。
1. 对**已关闭**工单的 `docs/rNN-validation/`、`docs/rNN/`、`docs/rNN-addendum*/`、`docs/r89-r88-followup/`、`docs/r90*-queue-guard/`、`docs/design-mockups/` 等目录：删除前先 `git log -1 --format=%H -- <目录>` 取最后一次包含它的提交号，写入 `WORKLIST-INDEX.md` 的「证据所在提交号」列，然后 `git rm -r`。
2. 例外（保留）：P3-3 列出的被引用路径；进行中工单（R128/R129/R131/R134 等）的证据；`docs/index.html`（项目主页）。
3. 账本里指向已删证据的相对链接不必逐条改，在每个被移动账本顶部加一行：「证据已于 R135 移出工作区，见提交 `<hash>`」。

## P5 · 无引用代码与一次性脚本（跟踪文件，只删「确定没人用」的）

判定标准（三条全部满足才删）：
- 全仓 `grep -rF <文件名>`（排除 `docs/`、`node_modules`、`.git`）无任何引用；
- 不在 `.github/workflows/ci.yml`、`desktop/package.json`（scripts / build.files / beforePack）、`build-desktop*.sh|ps1`、`installer/` 中出现；
- 对应工单已关闭。

1. 候选（已初步核实无引用，执行时逐个复核）：
   - `scripts/`：`r75-live-probe.py`、`r76-mutations.py`、`r80-uninstall-mutations.py`、`r82-mutations.py`、`r83-startup-mutations.py`、`r84-cleanup-mutations.py`、`r89-asset-probe.py`、`r89-catalog-probe.py`、`r89-riot-probe.py`、`r89-live-mutations.py`、`r90-mutation-check.py`、`r90-queue-guard-mutations.py`、`r90b-queue-guard-mutations.py`、`r92-*.py`（5 个）、`r93-*.py`（2 个）、`r103-mutation-check.py`、`r115-mutation-check.py`、`r115-score-mutations.py`、`r115-browser.cjs`。
   - `desktop/`：`r65-select-layout-probe.js`、`r73-mutations.py`、`r74-mutations.cjs`、`r89-browser.cjs`（仅被上面要删的 `r89-live-mutations.py` 引用）、`r89-arena-detail-layout.cjs`、`r91-addendum-browser.cjs`、`r92-browser.cjs`、`r95-browser.cjs`、`r98-browser.cjs`、`r99-facade-picker-layout.cjs`。
   - **保留**：`desktop/r100-browser.cjs`、`desktop/r117-browser.cjs`（CI 在跑）；`desktop/r74-current-game-checks.cjs`、`desktop/diagnostics-1555-layout.cjs`（被 `current-game-layout.cjs` 引用）；`desktop/diagnostics-*-render.cjs`（被 testdata README 引用，先确认有无测试 require，无则只在账本列出、不删）；`scripts/r82-startup-ab*`（installer 测试与 ps1 引用）；`scripts/r133-*`、`go-test-shards*`、`build-stage*`、`make-release*`、`extract-skin-release-dates.py`（数据再生工具）、`run-benchmarks.sh`（R115 基准入口）；`tools/` 两个子项目；`backend/data/reroll_pool_14_5_source.jpg`（奖池数据的原始出处，README 引用）。
2. Go 死代码：在 `backend/` 跑
   ```bash
   go run golang.org/x/tools/cmd/deadcode@latest -test ./backend > docs/r135-validation/deadcode-with-tests.txt
   go run golang.org/x/tools/cmd/deadcode@latest ./backend       > docs/r135-validation/deadcode-prod.txt
   go run honnef.co/go/tools/cmd/staticcheck@latest -checks U1000 ./backend > docs/r135-validation/unused.txt
   ```
   - 只删除 **`-test` 模式下仍不可达**（即生产和测试都不调用）的函数/类型/变量/常量。
   - 「仅被测试调用」的函数（出现在 prod 报告、不在 with-tests 报告）**一律不删**，只列表写进账本，留给后续单独评估。
   - `r99_probe.go`、`ranked_split_probe.go`、`augment_contract_probe.go`、`position_contract_probe.go` 是由环境变量/命令行开关触发的诊断探针，属于功能，**不删**。
   - `//go:embed` 覆盖的 `backend/web/*.js|css|html|png|svg` 和 `data/*` 不因「Go 里没引用」而删；`web/demo-data.js` 被 `runtime.js` 使用，保留。
3. 前端 JS/CSS 不做死代码删除（没有可靠的静态工具，误删风险高），本工单只在账本记录明显疑似项，不改。
4. 每删一批（scripts、desktop、Go 死代码各一批）单独提交，并立即跑 P6 对应部分；任何一项失败就回滚该批次，不改测试迁就。

## P6 · 验收（全部满足才算完成）

1. P0-3 的命令全部重跑，**通过数、跳过数与基线完全一致**（P5 若删了随死代码失效的测试，逐条列出并说明，数量差必须能对上）；失败数不得增加。
2. `gofmt -l` 为空（沿用 CI 的 find 命令），`go vet ./...` 通过。
3. `go test -run 'TestEmbeddedPoolHas554UniqueEntries|TestEmbeddedPoolMapsOneToOneWithoutOmissions' ./...` 通过。
4. 真实跑一次 `build-desktop.sh` 至少到 `backend-build` + `backend-fingerprint` 阶段，确认 `desktop/backend/loot-service.exe` 能重新生成，且此时仓库内**没有再出现 `.gocache/`**；能完整打包则跑完整打包，确认 `dist/desktop` 下安装包生成、`packaged-fingerprint` 通过。
5. 对比 `go build` 产物：删死代码前后各构建一次 `GOOS=windows GOARCH=amd64 go build -trimpath -o /tmp/r135-{before,after}.exe ./backend`，记录体积差；`go run ./backend -h`（或现有 CLI 帮助入口）输出一致。
6. `git status` 干净，`git ls-files | wc -l` 与前后差值能被 P3–P5 的移动/删除清单逐条解释。
7. `du -sh .[!.]* * | sort -rh` 存为 `docs/r135-validation/size-after.txt`，账本给出前后对比表。
8. 顺手修正：CLAUDE.md 与 AGENTS.md 的「当前版本」改为 `desktop/package.json` 实际值（0.12.18），并加一句「Go 构建缓存使用系统默认目录，不放仓库内」。

## 交付物

- `docs/r135-execution-ledger.md`：每节做了什么、删了/移了哪些路径（完整清单）、前后体积、测试前后对比、P2 是否执行。
- `docs/WORKLIST-INDEX.md`。
- `docs/r135-validation/`：baseline-status、size-before/after、deadcode/unused 报告（纯文本，体积小）。
- 本工单关闭后按 P3 规则移入 `docs/history/worklists/`。
