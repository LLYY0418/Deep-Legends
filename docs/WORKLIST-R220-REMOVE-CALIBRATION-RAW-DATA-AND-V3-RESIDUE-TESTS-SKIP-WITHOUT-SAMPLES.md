# WORKLIST-R220：校准采集的原始数据（约 1GB）和 v3 研究残留不再保留——先让测试在缺样本时自动跳过，再删除

诊断人：Claude。执行人：GPT。日期：2026-10-05。基线：R218 工作区。只做测试改造和清理，不改任何生产代码，不构建、不发布。

**用户决定（2026-10-05）**：v3 已放弃（v2.1 在新样本上已证明优于 v2，v3 当时就比 v2 差）；之后不再优化评分，所以采集的原始数据不外置保留，直接删除。

## 背景

- R211、R216、R217 采集的原始数据共约 1GB，在 git 里没有被跟踪（`git ls-files` 为 0），只占本机磁盘，没进仓库。
- 评估已各完成一次（v2.1 评分、关键词），**结果文件、冻结参数、哈希清单都是小文件，留在仓库里，足以说明结论是怎么得出的**。
- 以后如果再调评分，本来就必须再采一批全新样本做验证，旧样本的价值很低。

---

## P1　先改测试：原始样本不存在时跳过，而不是失败

以下测试读取了将被删除的原始数据，改为**文件不存在时 `t.Skip("校准原始样本已清理，见 R220")`**：

| 测试 | 读取的文件 |
|---|---|
| `TestR216PythonV2MigrationConsistency260` | `docs/history/reports/r211/opgg-samples/*-match.json` |
| `TestR216PythonV21GoldenConsistency260` | 同上 |

规则：
- 只在“**目录或文件不存在**”时跳过；文件存在但内容异常仍然失败。
- **不要**跳过这些仍保留的文件（它们很小）：`r216/v21-candidate.json`、`r216/kr-champion-table-probe/*.html`（9MB，见 P2）、`r217/rule-freeze.json`。
- 这 260 局的对照结果已经固化在 `backend/testdata/r216/python-v2-golden.json` 和 `python-v21-golden.json`，**这两个 golden 文件保留**，生产回归测试继续用它们。需要检查：不依赖原始样本的 golden 对照测试仍在跑、没有被误跳过。

验收：先**临时把三个原始数据目录改名**（不要删），跑 `go test -count=1 ./backend`，输出里只能有上面两个测试显示 SKIP，其余全部通过。

---

## P2　再删数据与残留（P1 验收通过后）

### 删除

| 路径 | 大小 | 说明 |
|---|---:|---|
| `docs/history/reports/r211/opgg-samples/` | 154MB | R211 原始 154 局 |
| `docs/history/reports/r211/opgg-validation-new-accounts/` | 127MB | R211 新账号 108 局 |
| `docs/history/reports/r216/opgg-holdout/` | 369MB | R216 留出集 300 局 |
| `docs/history/reports/r217/opgg-holdout/` | 259MB | R217 留出集 200 局 |
| `docs/history/reports/r216/account-selection/` | 42MB | 账号选取时的页面快照 |
| `docs/history/reports/r217/account-selection/` | 29MB | 同上 |
| `docs/history/reports/r217/training-curves.json` | 25MB | 训练用曲线 |
| `docs/history/reports/r217/training-selected-go.json` | 4.2MB | 训练的逐人导出 |
| `docs/history/reports/r217/mode-interval-probe/` | 6.3MB | ARAM/海斗间隔探测的空结果 |
| `scripts/r211-refine-calibration.py` | | v3 研究脚本 |
| `scripts/r211-refine-official-calibration.py` | | v3 研究脚本 |
| `scripts/r211-evaluate-new-accounts.py` | | v3 的一次性评估脚本 |

`scripts/r211-score-calibration.py` 含有 v2 的 Python 参考实现，其他脚本可能引用。**删除前先 `grep` 引用**，没有引用就一起删，有引用就保留并在账本写明引用方。

### 保留（小文件，都是结论的证据）

- `r216/`：`v21-candidate.json`、`holdout-evaluation.json`、`holdout-evaluation.started.json`、`holdout-accounts.json`、`account-selection-summary.json`、`collection-integrity-audit.json`、`keyword-calibration.json`、`kr-champion-table-probe/`、所有截图和 `layout.json`。
- `r217/`：`rule-freeze.json`、`keyword-candidate.json`、`holdout-keyword-evaluation.json` 和 `.started.json`、`opgg-rule-study.json`、`training-keyword-report.json`、`holdout-accounts.json`、`account-selection-summary.json`、`collection-verification.json`、`production-code-freeze.json`。
- `r211/`：截图、`mvp-disagreements*.json`、`score-calibration-*.json`、`score-calibration-report.md`、`score-candidate-official.json`（**v3 被否决的冻结参数，留作记录**）、`score-validation-new-accounts.json`、各类小报告。
- `backend/testdata/r216/`、`r217/`：生产测试在用，不动。
- 所有 `scripts/` 里没被列入删除表的脚本。

### 同步修改

1. **manifest 与账本**：原始数据删除后，各 `manifest-sha256.json` 与 `collection-verification.json` 里的文件清单不再对应磁盘。**不要改写它们**，在各自账本里写一句“原始数据于 2026-10-05 按 R220 删除，清单保留作采集当时的记录”。评估脚本（R216/R217 的一次性评估）原本会校验这些文件，现在不能再运行，在脚本头部注释说明“评估已完成，原始数据已删除，不可再运行”。
2. **`.gitignore`** 加上，防止以后再采集又被提交：
   ```
   docs/history/reports/*/opgg-*/
   docs/history/reports/*/account-selection/
   ```
3. **`docs/WORKLIST-INDEX.md`**：加 R220 行；R211 行备注补一句“v3 研究残留已由 R220 清理”。

### 删除权限

删除必须只针对上表路径，**不要用通配符清理 `docs/history/reports/` 的其他内容**。删之前先把将要删除的目录大小和文件数写进账本，删后写剩余大小。

---

## 收尾

1. 删完后再跑一次：`go test -count=1 ./backend`（记录开始时间、耗时、最后 5 行）、`go vet ./backend`、`backend/web` Node 全量、`desktop/overview-render.test.cjs`。要求：除 P1 列出的两个 SKIP 之外，没有新增跳过或失败。
2. 账本 `docs/r220-execution-ledger.md`：删除前后的目录大小、SKIP 的两个测试名、保留清单。
3. 不构建、不打包、不发布，版本保持 0.12.71。

## 用户真机验收

无。这是仓库清理，不影响应用功能。
