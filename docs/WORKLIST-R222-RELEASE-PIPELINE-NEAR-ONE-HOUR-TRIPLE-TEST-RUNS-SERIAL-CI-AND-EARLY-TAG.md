# WORKLIST-R222：发版一次将近一小时——同一套测试跑三遍、CI 串行、单个测试文件拖 9 分钟、预检没过就推 tag

诊断：Claude（只读核对 0.12.73 发布账本、`windows-release.log`、`quality-workflow.json`、GitHub Actions 运行 37298993269 / 37298993273 的步骤时间和构建脚本）。执行：GPT。日期：2026-10-05。
基线：0.12.73（提交 `245a579b`，指纹 `7c34391c96f6`）。

用户问题：**每次发布新版本都很慢，接近一小时，要查清是哪个环节卡住了。**

结论先说：用户端在线升级本身只要 **12.5 秒**（runner 实测：卸载旧版 1.8s、解压 7.7s、复制 0.26s），不是瓶颈。慢在**发布流水线**：从开始到正式 Latest 用了约 55 分钟，其中约 40 分钟花在把同一套测试重复跑和串行等待上。

---

## 0.12.73 实际时间线（北京时间）

| 时间 | 事件 | 耗时 |
|---|---|---|
| 18:34 | 开始发布（记录发布前元数据、部署 Worker、核验） | ~7 分钟 |
| 18:41 | **推送 `v0.12.72` 标签**，CI 开跑；此时本地预检还没出结果 | |
| 18:41–18:48 | 本地预检 334 项失败 6 项（全是测试夹具），取消三条工作流、修夹具、版本号改 0.12.73 | **~8 分钟白费** |
| 18:49 | 推送 `v0.12.73`，两个工作流同时开跑 | |
| 18:49–19:04:46 | `quality`（Linux）：Go race 4.5 分钟 + 前端/桌面测试 **10.3 分钟** | 15.5 分钟 |
| 18:49–19:08:51 | `release`（Windows 正式构建）：构建脚本里又把全部测试跑一遍 | 19.6 分钟（并行，不在关键路径） |
| 19:04:51–19:24:27 | `windows-build`：**等 quality 跑完才开始**，又把全部测试跑一遍，再打包、实装升级 | 19.6 分钟 |
| 19:25:58 | 正式发布 Latest | |
| 19:29 | 文档收尾提交 | |

关键路径 = tag 后 **quality 15.5 分钟 → windows-build 19.6 分钟 = 35 分钟**，再加上前面 15 分钟准备和一次失败重来。

### Windows 构建步骤（19 分钟）拆开看（`release` 作业，`windows-build` 几乎一样）

| 阶段 | 起止（UTC） | 耗时 |
|---|---|---|
| npm ci | 10:49:48–10:50:04 | 16s |
| `go test ./...`（backend 包 205.5s）+ vet + installer | 10:50:04–10:54:32 | **4 分 28 秒** |
| 前端 web 测试 938 项 | 10:54:32–10:55:36 | 62s |
| 桌面 desktop 测试 280 项 | 10:55:36–11:05:22 | **9 分 46 秒** |
| 后端编译 + 自检 | 11:05:22–11:05:47 | 25s |
| electron-builder（7z level 9 压缩 2 分 11 秒） | 11:05:47–11:08:14 | 2 分 27 秒 |
| 外壳、校验、make-release、TestUpdate | 11:08:14–11:08:31 | 17s |

**真正的打包只要约 3 分钟，其余 15 分钟都是在重跑 Linux quality 已经跑过的测试。**

---

## P1　同一提交的测试在一次发布里跑三遍，而且 windows-build 串行排在 quality 后面

现状：
- `ci.yml` 的 `quality`（Linux）跑一遍 Go race / vet / 前端 + 桌面测试；
- `ci.yml` 的 `windows-build` 有 `needs: quality`，开跑后调用 `build-desktop-windows.ps1`，脚本里**无条件**再跑 `go test ./...`、`go vet`、installer 测试、web 938 项、desktop 280 项；
- `release.yml` 同时调用 `build-public-release-windows.ps1` → 同一个 `build-desktop-windows.ps1`，**第三遍**。
- 本地预检再跑一遍 desktop/scripts/relay（351 秒）。

要求：
1. `build-desktop-windows.ps1` 增加开关（例如 `-SkipTestsInCI`），**只有 `GITHUB_ACTIONS=true` 时才允许生效**，本地运行传入时直接报错退出；本地完整构建行为不变。开关生效时跳过 Go test/vet、installer test/vet、web/desktop node 测试，保留 gofmt、`node --check`、指纹、后端自检、打包后的全部校验（verify-packaged-runtime、verify-build-fingerprint、release-build、SHA256）。
2. `ci.yml` 的 `windows-build` 和 `release.yml` 都带上该开关。
3. Linux 上跳过的 Windows 专属测试不能因此丢掉：在 `windows-build` 里单独加一步，只跑这些文件/用例（例如 R86 Windows release 失败即停、setup-only-build、需要 PowerShell/Windows 的 desktop 用例；以 Linux quality 日志里 `# SKIP` 的用例为准，逐条列进账本）。
4. 去掉 `windows-build` 的 `needs: quality`，两个作业并行。工作流整体结论仍要求两者都成功；发布规程保持「正式发布 Latest 前必须看到同一 SHA 的完整 quality-and-windows-release 成功」——这一条写进发布账本模板。`release.yml` 只产出**草稿**，跳过测试不影响这个门槛。
5. 现有护栏测试要同步更新而不是删掉：「R86 Windows release stops on a real failing Go test…」「both release entry points wrap NSIS before hashes…」「setup-only-build」等断言构建脚本结构的用例，改成同时验证：本地模式仍会因失败的 Go 测试停止；开关在非 CI 环境被拒绝。

预期：关键路径从 35 分钟降到 max(quality, windows-build≈5–6 分钟) ≈ quality 的时长。

## P2　`desktop/overview-render.test.cjs` 一个文件拖住整个桌面测试步骤 9～10 分钟

node --test 是**按文件并行、文件内串行**。Windows 日志里 desktop 测试 10:55:36 开始，其它文件很快结束，只有这一个文件从 10:55:55 一直跑到 11:05:22。最慢的 5 个用例全在这个文件里，都是用 jsdom 启动 **200 场**演示数据：

| 用例 | 耗时 |
|---|---|
| R86 ADD-1 card detail, metric, legacy damage controls and timeline stay local at 200 matches | **227.7s** |
| R86 filtering 200 loaded matches hides entries without rebuilding or refetching | 52.7s |
| R86 ADD-1 late timeline cannot reveal a filtered-out card | 50.6s |
| R86 image listeners and tab scroll remain bounded including independent image-listener bypass | 48.8s |
| 总览战绩卡：点击展开按钮只替换目标卡片并可收起 | 45.1s |

合计约 425 秒。第一条 227 秒是因为 `check()` 被调用 6 次（正常 + external + 4 个变异体），每次都重新启动一个 200 场的完整 jsdom 应用（每次约 38 秒）。Linux quality 的「Test frontend and desktop renderers」10.3 分钟也是同一原因。

要求（不得删除或放宽断言）：
1. 把这几个 200 场用例从 `overview-render.test.cjs` 拆到独立文件（例如 `overview-render-200.test.cjs` 再按用例拆 2～3 个文件），让 node --test 能并行跑。
2. 变异体检查（`detail / metric / damage / timeline` 四个 mutant）只需证明护栏能抓到整页重建，不依赖 200 这个数量：改用小样本（例如 20～40 场）启动；200 场只保留在正常路径和 external 路径上，用来守性能预算。`createElement < 500` 的预算断言保持在 200 场正常路径上。
3. 能复用同一个已启动窗口的检查（同一个 200 场 app 上依次做的只读断言）不要重复启动。
4. 顺手把测试里反复刷屏的 `live recommendation diagnostic failed … Failed to parse URL from /api/diagnostics/client` 处理掉（jsdom 里给诊断上报一个可解析的地址或替身），这是噪声，不是断言。

验收：任何单个测试文件 Windows 上 ≤ 90 秒；desktop 测试步骤 Windows ≤ 4 分钟、Linux「Test frontend and desktop renderers」≤ 4 分钟；用例总数和断言不减少，四个变异体仍然必须 reject。

## P3　本地预检还没通过就推 tag，失败后整轮重来

0.12.72：18:41 推 tag，预检之后才报 6 项失败（夹具缺依赖、KR-only 菜单断言、模拟 Go 不识别 `env GOCACHE`）→ 取消 3 条工作流、修复、换版本号 0.12.73，浪费约 8 分钟和一个版本号。0.12.73 的预检（351 秒）也是在 tag 推出去之后才跑完（`preflight-final.log` 写于 10:55 UTC，tag 在 10:49 UTC）。另外 `codex/release-0.12.72` 分支推送又多触发了一次完全相同的 quality 运行。

要求：
1. 发布规程改为：**本地预检全部通过 → 再提交版本号 → 再推 tag**。写进 AGENTS.md 的发布步骤。
2. 发布提交只推 tag（tag 已包含提交），不要同时把同一提交推到 `codex/release-*` 分支触发重复 CI；如需分支，等 tag 的工作流结束后再推，或在 `ci.yml` 加 `concurrency: { group: ci-${{ github.sha }}, cancel-in-progress: false }` 之类的去重方式（选一种，记录在账本里）。

## P4　Go backend 包测试单包 205 秒（Windows）/ race 235 秒（Linux）

`lol-loot-assistant/backend` 一个包就占 3.5～4 分钟。日志没有 `-v`，无法直接看出是哪些用例。

要求：
1. 用 `go test -json ./backend` 列出耗时前 20 的用例，记录到账本。
2. 对其中依赖真实 `time.Sleep` / 真实超时 / 真实重试间隔的用例，改用可注入时钟或缩短测试专用间隔；不改变被测逻辑和断言。
3. 验收：backend 包 Linux race ≤ 120 秒。

## 不改的部分（只记录）

- electron-builder 的 7z level 9 压缩约 2 分 11 秒，换来更小的安装包（111.8 MB），用户下载受益，本次不动。
- 用户端在线升级 12.5 秒，正常。

---

## 总体验收

下一次正式发布按同样口径记录时间线（开始、推 tag、各作业起止、发布 Latest），写进该版本账本：

- tag 推出到「可发布 Latest」≤ 15 分钟（现在 35 分钟）；
- 整次发布（含 Worker 部署与预检）≤ 30 分钟（现在约 55 分钟）；
- 每个发布 SHA 上，全部测试至少完整跑一遍（Linux），Windows 专属用例在 Windows 上跑一遍，没有任何测试被删除或断言被放宽。
