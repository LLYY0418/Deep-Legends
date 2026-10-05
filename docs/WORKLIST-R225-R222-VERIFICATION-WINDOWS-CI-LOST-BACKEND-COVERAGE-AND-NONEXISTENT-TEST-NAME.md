# WORKLIST-R225：R222 核验——Windows CI 丢了后端 Windows 路径覆盖（含更新流程），且清单里有一个不存在的测试名

诊断：Claude（对 R222 最终代码做独立核验：干净导出 `1c0508a6`，其代码等于 `7363fa8f`，二者之间只差文档；读 diff、对比测试名、实际跑 Node 测试、交叉核对 CI 运行 37312091768）。执行：GPT。日期：2026-10-05。
基线：`origin/codex/r222-release-pipeline` @ `1c0508a6`，版本 0.12.73。

## 核验结论（R222 其余部分没问题，只记录不改）

- 提速目标已达成：同一 SHA 的完整 CI 37312091768 实际 success，总计 11 分 1 秒（quality 7 分 8 秒、windows-build 10 分 56 秒，并行）。
- `overview-render.test.cjs` 原 55 个用例名一个不少，另有 2 个新增（R222 external、R222 四个变异体）；断言未削弱。
- Go 侧：R96/R99/R102 只是注入等待时钟，生产默认仍是真 timer，且新增了精确序列断言；看过 `arena_live_grouping.go`、`pro_seed_accounts.go`、`main.go` 的生产改动，只是加可注入字段，默认行为不变。
- 本机实跑（干净导出）：web 938/938 通过；overview 的 200 场拆分文件 7/7 通过；其余 desktop 文件逐个运行全部通过；`release-quality-gates` 在有 Go 的环境下 7 项通过 5、Windows 专属 2 项按预期跳过。（本机沙箱没有 Go，`setup-only-build` 里 3 个依赖 gofmt 的用例只能靠 CI 证据，未在本机复核。）
- `-SkipTestsInCI` 在非 CI 环境会拒绝，代码上没问题。

---

## P1　Windows 作业不再在 Windows 上运行后端测试，R222 工单里“Windows 专属用例不能丢”没有真正做到

### 1.1 清单里有一个根本不存在的测试

`ci.yml` 的 “Test Windows-only release and PowerShell guards” 步骤里：

```
go test ./backend -run '^(TestSplitRegistryPathSupportsNativeTencentKeys|TestR204KeySaveRejectsUnauthorizedAndEncrypts|TestR204KeyRuntime401AndPrivacy)$'
```

`TestR204KeySaveRejectsUnauthorizedAndEncrypts` 在整个仓库里搜不到（`grep -rn` 无任何 Go 文件命中；只出现在 ci.yml 和 `docs/r222-execution-ledger.md`）。`go test -run` 里正则多出一个匹配不到的名字不会报错，所以这一步一直是绿的，名单上写着 3 个，实际只跑了 2 个。

真正含 Windows 分支的是 `backend/r204_key_test.go` 的 **`TestR204KeySaveAndClear`**（第 98 行 `runtime.GOOS == "windows"` 才会检查 “DPAPI did not encrypt”，第 94 行的 0600 权限检查在 Windows 上跳过）。这个用例现在没有任何 Windows 作业在跑，等于 DPAPI 加密落盘这一条 Windows 专属断言整个流水线都不再检查。

### 1.2 不止这一条：以前 Windows 上跑的整套后端测试被换成了 3 个

R222 之前，`build-desktop-windows.ps1` 在 Windows 上跑 `go test ./...`，公开发布脚本还单独跑 `go test -run TestUpdate ./...`；`-SkipTestsInCI` 把这两处都去掉了。R222 账本是按 “Linux 日志里出现 `# SKIP` 的用例” 加 “`_windows_test.go` 文件” 来列 Windows 清单的，这个方法漏掉了**没有 skip 标记、但在 Windows 上会走到 `*_windows.go` 平台代码的跨平台用例**。核对结果：

| Windows 专属源码 | 被哪些跨平台测试走到（以前 Windows 上跑，现在不跑） |
| --- | --- |
| `update_platform_windows.go`（`detectUpdateInstallation`、`updateDiskFreeBytes`、`updateLongPathName`、`launchUpdateInstaller`…，由 `update.go` 调用） | `backend/update_test.go` 全部 `TestUpdate*`（18 个，含 `TestUpdateApplyRehashPortableMinimumAndCommand`、`TestUpdateAfterUpgradeCleanupAndMinimum`）；旧公开发布脚本里显式的 `go test -run TestUpdate` 也被删了 |
| `riot_key_protection_windows.go`（DPAPI `protectRiotUserKey`/`unprotectRiotUserKey`，由 `riot_key_settings.go` 调用） | `TestR204*`（其中只有 2 个被点名，且其中 1 个名字写错，见 1.1） |
| `game_settings_watch_readonly_windows.go`（`cameraFilePermissions`、`gameSettingsReadOnly`） | `r201_test.go` |
| `client_installations_windows.go`（`detectClientInstallationsWithScan`、`launchClientInstallation`、`riotClientCandidates`…） | `main_test.go` 中的相关用例 |
| `game_camera_process_windows.go`、`accept_window_windows.go`、`process_windows.go` | 经 `game_camera_mode.go`、`accept_focus_trace.go` 等走到的相关用例 |

在线更新是用户最敏感的路径；Windows 上真实安装升级那一步只覆盖安装器，不覆盖后端 `update.go` 的 Windows 分支。Linux 上这些文件根本不参与编译（`*_other.go` 替代），所以 Linux quality 不能代替。

### 要求

1. 修正清单里的错名：改为 `TestR204KeySaveAndClear`（以及仍保留 `TestR204KeyRuntime401AndPrivacy`）。
2. 重新建立“Windows 路径覆盖”的清单，**方法要写进账本**：对每个 `backend/*_windows.go`（以及 installer 里同类文件）列出其函数，找出所有会走到它们的测试（直接引用，或经 `update.go`、`riot_key_settings.go`、`game_camera_mode.go`、`accept_focus_trace.go`、`process_*` 等共享入口），并至少恢复：全部 `TestUpdate*`、全部 `TestR204*`、`r201_test.go`、`main_test.go` 里的客户端安装/启动用例、相关相机/接受窗口/进程命令用例。
3. 实现方式二选一，由 GPT 按实测时间选，账本里写明理由：
   - A. Windows 作业直接跑 `go test ./backend`（不带 `-race`，沿用 R222 P4 提速后的用例），最省心也最不会再漏；
   - B. 保留 `-run` 清单，但清单必须由 1.2 的方法产出，并配 P2 的防空匹配守卫。
   无论哪种，tag→可发布 Latest 仍须 ≤15 分钟；windows-build 必须继续与 quality 并行，不得重新加回 `needs: quality`。
4. 不得为了省时间把 `-SkipTestsInCI` 的范围改回去：构建脚本里仍然不重复跑测试，覆盖放在 CI 的独立 Windows 测试步骤里。

验收：Windows 作业日志（用 `go test -v` 或 `-json`）里能逐条看到 `TestUpdate*`、`TestR204KeySaveAndClear`、`TestR204KeyRuntime401AndPrivacy`、`TestSplitRegistryPathSupportsNativeTencentKeys` 的 `--- PASS`；同一 SHA 的完整 CI 再跑一次 success，并把三个作业的起止时间写进 `docs/r222-execution-ledger.md` 或本工单账本。

---

## P2　按名称筛选的测试步骤匹配不到时会静默通过，需要守卫

这次的错名能漏过去，是因为 `go test -run` 和 `node --test --test-name-pattern` 都有同一个特性：**筛选条件一个测试也没匹配到，退出码仍是 0**。目前 ci.yml 里这类步骤有：

- `go test ./backend -run '^(...)$'`
- `go test ./... -run '^(...)$'`（installer）
- `node --test --test-name-pattern="R86 Windows release|R222 Windows" desktop/release-quality-gates.test.cjs`
- `node --test --test-name-pattern="setup-only Windows CI skip" scripts/setup-only-build.test.cjs`

要求：

1. 增加一个守卫（脚本或测试都可以，例如 `scripts/verify-ci-test-filters.cjs`）：解析 `ci.yml` 里这些 `-run` / `--test-name-pattern` 清单，静态确认**清单中每一个名字**都能在对应目录的 `*_test.go` / `*.test.cjs` 里找到；找不到就失败。并在 Linux quality 里运行它（这样下次再写错名字，不用等 Windows 作业就能发现）。
2. Windows 步骤改成能证明“确实运行了”：Go 用 `-json`（或 `-v`）并检查每个点名用例都出现 `pass` 事件；Node 检查期望的用例数（`# tests` 与预期一致，且 Windows 上 `skipped` 为 0）。任一不满足就失败。
3. 给守卫做一次变异验证并把结果记入账本：临时往清单里加一个不存在的名字，确认守卫失败；还原后通过。

验收：变异验证有记录；守卫已进入 Linux quality；Windows 步骤在清单缺项或意外 skip 时会失败。

---

## 不改的部分（只记录）

- `release.yml` 只生成草稿、不跑测试，靠“同 SHA 的完整 quality-and-windows-release 成功后才发布 Latest”这条发布规程把关，这是 R222 已经定下的设计。
- Windows 桌面 jsdom 全量测试仅在手动开启 `measure_windows_renderers` 时运行，也是 R222 定下的取舍。

## 总体验收

- P1：Windows 上能看到更新、DPAPI、客户端安装等后端用例实际通过；清单里没有任何不存在的测试名。
- P2：下次有人写错测试名，守卫在 Linux quality 里就会失败。
- 速度不倒退：同 SHA 完整 CI 总时长仍 ≤15 分钟（R222 实测 11 分 1 秒）。
