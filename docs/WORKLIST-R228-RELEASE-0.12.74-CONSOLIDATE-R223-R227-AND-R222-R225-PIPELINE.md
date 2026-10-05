# WORKLIST-R228：发布 0.12.74——合并 R223/R224/R226/R227 修复与 R222/R225 新流水线，走一遍新发布流程

诊断：Claude（只读核对工作区状态、各执行账本、分支与 CI 记录）。执行：GPT。日期：2026-10-05。
基线：正式 Latest 是 0.12.73（国服战绩全空，见 R223）。本次目标版本 **0.12.74**，key mode **public**，不读取或嵌入个人 Key。

## 为什么要发、为什么现在还不能直接发

线上 0.12.73 的国服战绩读取是坏的（R223 P1），要尽快出补丁。但目前的源码状态不能直接打 tag：

1. **代码没提交。** R223、R224、R226、R227 共约 168 个文件的改动，全部是未提交状态，躺在本地 `codex/r222-release-pipeline`（HEAD `270fc3af`）的工作区里，从来没有在 CI 上跑过（R223 账本原话：“当前未提交源码的新 CI 尚未运行”）。
2. **新流水线在别的分支。** 本地分支停在 R222 的第一个提交，缺少 `origin/codex/r225-windows-coverage`（`1a119519`）上的 9 个提交：R222 验收修复、R225 的完整 Windows 后端/installer 测试与防空匹配守卫。核对过：这 9 个提交改动的文件与工作区未提交改动**没有重叠**，合并应当不会冲突。
3. **R226 / R227 只在本机验过。** R226 修的是 R224 引入的 R70 护栏失败（会让 CI 挡住发布），R227 修的是门禁误报停止；二者都没在 CI 上跑过，R227 在索引里还是“待确认关闭”。
4. **Windows 真机验收未做。** R223、R224 账本都明确写着没做。国服事故的真实触发字段至今没有证据（R223 账本：“具体字段尚无证据，不能写成已经查明”），所以“国服战绩已修好”目前只在合成数据上成立。

---

## P1　整理源码并入库（不发布）

1. 先备份工作区状态（例如 `git stash` 前先 `git diff > 本地备份`，或新建分支提交），不要丢失任何未提交改动。
2. 在新分支（建议 `codex/release-0.12.74`）上，**先合并 `origin/codex/r225-windows-coverage`，再提交工作区改动**，按工单拆提交：R223、R224、R226、R227 各一个（共享文件无法拆开时合为一个，提交说明里写清涉及的工单号）。提交信息遵循仓库现有风格。
3. **不要提交**：`docs/history/reports/r222/*.jsonl`、`*.jsonl.gz`、`*.log.gz` 这些原始日志（R222 账本已说明只留本地）；`docs/r116-validation/`、`docs/r121-validation/`、`docs/r166-validation/`、`docs/r174-validation/` 这四个旧验证目录（0.12.73 发布时也是排除的）。提交前核对暂存内容无 Riot Key、GitHub token、私钥。
4. 工单/账本归档沿用 R225 已确立的位置约定（R225 把工单和账本放进了 `docs/history/worklists/`、`docs/history/ledgers/`）；R223/R224/R226/R227 的账本位置与索引行保持一致，别让链接失效。
5. 验收：`git status` 干净（除上面明确排除的文件）；分支包含 R225 分支的全部提交；`git diff origin/codex/r225-windows-coverage --stat` 里只有 R223/R224/R226/R227 及其账本/测试。

## P2　合并后代码的本地预检（版本号还不动）

按 AGENTS.md 发布步骤 1 的预检集合，在**合并后的最终源码**上全部跑一遍，结果以最终源码为准（中途修了东西就重跑受影响项）：

- 根模块 Go：`go test -count=1 ./...`、`go test -count=1 -race ./...`、`go vet ./...`、gofmt 检查；
- `installer` 模块 test / vet；
- JS syntax（`backend/web/app.js`、`suite.js`、`desktop/main.cjs`）；
- `node scripts/verify-ci-test-filters.cjs`；
- `node scripts/test-renderers.cjs all`（门槛：单文件 ≤90s、集合 ≤240s、0 失败；允许的 skip 仅限本机没有 `pwsh`/Windows 的那 4 项，逐条列出）；
- Riot Worker 测试 `node --test relay/riot-worker/*.test.mjs`；
- 真实 Chromium 护栏 `desktop/r100-browser.cjs`、`desktop/r117-browser.cjs`；
- public 后端重新构建 + `--self-test` + 指纹核验。

R223～R227 新增的测试数量与耗时要记录：新增用例不能让 Linux backend race 超过 120s，也不能让任何 Node 测试文件超过 90s；超了就按 R222 的做法拆分/注入时钟，**不得删除或放宽断言**。

## P3　先在分支上跑一次完整 CI（推荐，避免再烧一个 tag）

0.12.72 就是没先验证、直接推 tag，结果预检失败、tag 作废、白占一个版本号。这次合并了 5 个工单的未提交代码，风险更大：

1. 把 P1 的分支推到 GitHub（**不是 tag**，不带 `v*`），触发完整 `quality-and-windows-release`；
2. 要求同一提交 `conclusion=success`：Linux quality（含 Go race ≤120s、Node ≤240s）、Windows 后端/installer 全量、R82 PowerShell、public 构建与真实安装升级；
3. 失败就在分支上修，修完重跑；**修复不得放宽任何断言**。如果是 R226 提到的那类“新代码触发旧护栏”，按原则改生产代码，不改护栏。
4. 记录分支 CI 的运行号、起止时间、各作业耗时。

## P4　版本号与发布说明

P2、P3 全部通过后才动版本号：

1. `desktop/package.json`、`desktop/package-lock.json`、`CHANGELOG.md` 同步为 **0.12.74**（只改这三处，版本号提交里不夹带代码改动）。
2. CHANGELOG / 发布说明用中文，只写用户能感知的变化：国服战绩读取恢复、日服大区识别、登录入口点「否」后不再直接启动客户端、外服搜索菜单与分组名称、斗魂勇敢举动自动选用、对局页闪烁与标签间距、我的小队排序，等。**不要**写“统计口径”类解释文字（项目标准已定），**不要**把 R222/R225 的流水线改动写进用户说明。
3. 不得声称“国服战绩已在真实账号验证修复”——除非 P7 的真机检查已经做完并有记录。可写“修复国服战绩读取失败的问题”。

## P5　只推 tag，观察整条流水线并记录时间线

1. 按 `docs/release-ledger-template.md` 建立 `release-0.12.74-execution-ledger`，从开始时刻起记录时间线。
2. 发布提交**只推 tag `v0.12.74`**（tag 携带提交），不同时把同一提交推到 `codex/release-*` 分支；确需保存发布分支，等 tag 的工作流结束后再推。
3. tag 会同时触发两条工作流：`quality-and-windows-release`（完整质量）和 `Windows public release draft`（只生成草稿）。要求：
   - 两者都成功后才算“可发布”；**同一 SHA 的完整 quality-and-windows-release 必须 success**，只看草稿作业成功不行；
   - 草稿恰好三个附件：`Deep-Legends-Setup-0.12.74-public.exe`、`latest.json`、`SHA256SUMS-public.txt`；下载实文件核对 size/SHA256/API digest、manifest 版本/指纹/URL、Release body 与 CHANGELOG 一致；安装包内不含个人 Key（public receipt 与内嵌 Key 门禁）。
4. 这是 R222/R225 新流水线的**第一次真实发版**，把 R222 留下的两个验收项补上实测：
   - tag 推出到“可发布 Latest” ≤ **15 分钟**；
   - 从开始（含预检）到正式 Latest ≤ **30 分钟**；
   - 实际达不到就如实写，不要用分支 CI 的数字代替；超标的环节单独列出来，留作下一张工单的输入。

## P6　发布 Latest 前停下，等用户确认

本工单授权的范围到“草稿 + 同 SHA 完整 CI 成功 + 附件验证通过”为止。**把 Release 从草稿改为正式 Latest 之前，把证据摆给用户并等明确确认**（用户决定是否先做 P7 的国服真机检查）。用户确认后：

1. 发布，确认 `draft=false`、`prerelease=false`、`isLatest=true`；
2. 匿名下载 Latest 清单，版本为 0.12.74，与已验证的 `latest.json` 字节一致；
3. 三个附件再经匿名下载核对 size/SHA256；
4. 旧 Release/附件、既有标签保持不变（发布前后做对比并存证）；
5. Windows runner 的实际升级记录写进账本（0.12.65 → 0.12.68 → 0.12.74，各阶段耗时）。

只有这些全部有证据，才能写“已发布”；缺任何一项只能写“发布未完成”。

## P7　真机检查清单（用户 Windows 机器；GPT 把步骤写成可照做的清单，截图/日志由用户提供）

这些来自 R223/R224 账本里“尚待验收”的部分，**CI 和 Windows runner 不能替代**：

1. **国服（最重要）**：用真实国服账号打开总览，确认最近 20 场战绩、赛季统计、对局详情的玩家都能显示；如果仍然全空，保留诊断日志并**新开工单**（不要回头改 R223），日志里要能看到是 SGP 返回的哪个字段导致解码失败。
2. 日服：登录识别、本人战绩、总览分组、与国服来回切换。
3. 登录入口：弹出 UAC 点「否」后，不再直接启动客户端，入口仍在。
4. 斗魂：勇敢举动自动选用（含随机英雄）、暂停/接管/关闭设置后恢复不会误报“已停止”。
5. 对局页：各模式连续观察 3 分钟，没有整页闪烁；名字与标签间距；“我的小队”排在最前。
6. 在线更新：从 0.12.73 用客户端内更新升到 0.12.74，设置/Key/桌面快捷方式/图标位置保持。

不在本工单内：新的 Worker 部署（先看 `relay/` 有没有改动，没有就不部署、不读取不替换 Secret）；7z 压缩等级；任何新功能。

---

## 总体验收

- 分支包含 R225 的全部提交与 R223/R224/R226/R227 的改动，无多余日志文件入库；
- 本地预检与分支 CI 在同一份合并源码上全部通过，门槛（race ≤120s、Node 集合 ≤240s、单文件 ≤90s）未倒退；
- `v0.12.74` 的同 SHA 完整 CI success，草稿三个附件逐项验证；
- 发布时间线实测（tag→可发布 ≤15 分钟、总计 ≤30 分钟）有记录，缺什么写什么；
- 正式发布只在用户确认后进行；国服真机检查结果单独记录，失败则新开工单。
