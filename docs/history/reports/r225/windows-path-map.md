# R225 Windows 路径覆盖映射

基线：1c0508a6 / 0.12.73。本表列全量编译恢复的调用入口，并明确 fake、静态断言和真实原生调用边界。

方法：枚举 backend 与 installer（含 uninstall、internal/webviewhost）的全部 `*_windows.go`；逐文件提取所有具名函数/方法，再由符号调用向共享入口及测试追溯。不能只看 `_windows_test.go` 或 Linux skip。三组独立只读探查后，主线程点验了 R204 的 write/load DPAPI 链和 newUpdateManager 的安装探测，修正了把 fake HTTP/launch 误当成全部平台实现的推断。匿名回调归属于包含它的具名函数。

实现选择 A：CI 独立步骤完整跑 Windows backend（无 race）与 installer 所有包；不把本表当 `-run` 白名单，不恢复构建脚本重复测试。Go AST 源清单 + `go list` 当前平台编译清单 + `go test -count=1 -json` 终态比较，保证全部已编译顶层测试有终态；关键 Windows 后端族、Windows 原生声明和全部 installer 必须 pass。相关源码恢复是基线覆盖的恢复，不宣称所有原生错误分支已覆盖。

## backend/accept_window_windows.go

函数/方法（括号内为基线行号）：`nativeAcceptWindowState` (16)、`acceptWindowCategory` (26)、`nativeAcceptWindowDetails` (62)。

调用/测试：accept_focus_trace.go before/after/sample、accept_focus_history.go、watch_rules.go → accept_focus_trace_test.go 的 Request/Sampler/Canceled/History/ExportInspection。

边界：Windows runner 上 trace 采样选中真实 nativeAcceptWindowState/Details；断言检查诊断/取消/脱敏，不逐项验证 HWND 分类、窗口遍历、每个 Win32 失败分支。

## backend/client_installations_windows.go

函数/方法（括号内为基线行号）：`detectClientInstallationsWithScan` (17)、`launchClientInstallation` (35)、`launchWindowsClientCandidate` (39)、`windowsErrorCode` (84)、`detectClientShortcuts` (92)、`scanClientShortcutDirectory` (112)、`queryRegistryValues` (137)、`splitRegistryPath` (172)、`riotClientCandidates` (189)、`collectRiotExecutables` (208)、`uniquePaths` (227)、`regularFile` (246)。

调用/测试：client_launcher.go cached detection/launchDetectedClientInstallation → main_test.go 客户端安装/启动全组；client_installations_windows_test.go。

边界：TestSplitRegistryPathSupportsNativeTencentKeys 直接验证 root 解析；WindowsClientDetection 是源结构断言；builder/candidate/HTTP 组验证共享入口，注入 detector/launcher 时不执行真实 registry/ShellExecute/shortcut。全量恢复原先 Windows 编译与共享契约，未新增真实 LoL 客户端启动测试。

## backend/game_camera_process_windows.go

函数/方法（括号内为基线行号）：`gameCameraProcessRunning` (5)。

调用/测试：game_camera_mode.go applyGameCameraMode → r198_test.go 全组，r201_test.go。

边界：r198 fixture 注入 cameraProcessRunning；gameCameraProcessRunning 的 nativeProcessCommands 实际进程枚举没有专用单测，不能把 fake 计作原生执行。

## backend/game_settings_watch_readonly_windows.go

函数/方法（括号内为基线行号）：`gameSettingsReadOnly` (8)、`cameraFilePermissions` (19)。

调用/测试：game_camera_mode.go/game_settings_lock.go/game_settings_watch.go → r201_test.go、r198_test.go。

边界：TestR201ReadOnlyCameraAndRelockRetry:25–44 的注入包装继续调用真实 cameraFilePermissions/gameSettingsReadOnly；InGame/None 场景的解锁闭包是禁止调用的 fake。保留只读属性/重锁验证。

## backend/process_windows.go

函数/方法（括号内为基线行号）：`hideCommandWindow` (30)、`nativeLeagueProcessCommands` (34)、`nativeRiotClientProcessCommands` (38)、`nativeProcessCommands` (42)、`windowsProcessCommandLine` (80)、`queryProcessCommandLine` (101)。

调用/测试：lcu.go leagueProcessCommands、riot_client.go discoverRiotClient、game_camera_process_windows.go → riot_client_test.go、r86_disk_test.go、相关 camera tests。

边界：现有 ProcessOutput/EmptyProcessDiscovery 是共享解析与构造 result；无直接 nativeProcessCommands/windowsProcessCommandLine/queryProcessCommandLine 单测。保留 Windows 编译及原共享测试，真实 NtQuery/Toolhelp 错误分支仍未证明。

## backend/riot_key_protection_windows.go

函数/方法（括号内为基线行号）：`protectRiotUserKey` (12)、`unprotectRiotUserKey` (21)。

调用/测试：riot_key_settings.go loadRiotKeyStore/writeLocked/save → r204_key_test.go 全部 5 项。

边界：真实 protect/unprotect；TestR204KeySaveAndClear:94–101 检查 Windows 密文不含 key 并重新加载，TestR204KeyRuntime401AndPrivacy 检查日志不泄密。HTTP validation 为 fake，DPAPI 不是 fake。

## backend/update_platform_windows.go

函数/方法（括号内为基线行号）：`updateLongPathName` (17)、`updateDiskFreeBytes` (35)、`detectUpdateInstallation` (44)、`installedUpdateDirectory` (71)、`launchUpdateInstaller` (75)、`launchPortableUpdateInstaller` (89)、`startUpdateInstaller` (106)、`updateRecoveryCommandLine` (145)。

调用/测试：update.go newUpdateManager/normalizeUpdateInstallationPathWith → update_test.go 全部 18 个 TestUpdate*；r201_test.go 的父进程参数护栏。

边界：updateTestManager 先 newUpdateManager（实际 detectUpdateInstallation），再覆盖 freeBytes/launch。安装探测可走原生；磁盘/启动/ack/timeout 多数通过 fake，不据此声称所有 Win32 分支已测。真实 installer 升级是另一路证据。

## installer/dialog_windows.go

函数/方法（括号内为基线行号）：`initialInstallDir` (16)、`chooseDirectory` (61)、`nearestExistingDirectory` (82)、`diskFreeBytes` (96)、`checkPath` (112)。

调用/测试：prepareDestination/validatePath → r86_smoke_windows_test.go。

边界：真实 disk/directory smoke；chooseDirectory 原生对话框未专测。

## installer/execute_command_windows.go

函数/方法（括号内为基线行号）：`configureWarmProcess` (9)。

调用/测试：warm command hooks → execute_prewarm*、startup_log_contract_test.go:176。

边界：AST 检查 HideWindow/CREATE_NO_WINDOW；fake warm hooks 不是原生 process creation。

## installer/handoff_windows.go

函数/方法（括号内为基线行号）：`inspectApplicationWindow` (25)、`hasApplicationWindow` (42)、`handoffApplication` (48)。

调用/测试：completion → handoffApplication/runApplicationHandoff → handoff_windows_test.go、handoff_test.go、completion_test.go。

边界：两个 Windows 测试创建 native test window 并做 EnumWindows；handoff 生命周期用 fake hooks。真实安装升级保留。

## installer/install_windows.go

函数/方法（括号内为基线行号）：`releasePayload` (19)、`prepareDestination` (57)、`install` (104)、`installerEnvironment` (238)、`directoryBytes` (252)、`extractedBytes` (274)。

调用/测试：install → releasePayloadWhileWaiting/completeInstallation/recoverUpdateApplication；r212_test.go、completion_test.go、r86_smoke_windows_test.go。

边界：共享 hooks/fake 完成策略与真实 prepareDestination smoke；完整 install/NSIS/UI 由实际安装升级补充。

## installer/internal/webviewhost/failure_windows.go

函数/方法（括号内为基线行号）：`ReportStartupFailure` (12)。

调用/测试：startup failure/fallback → startup_test.go 等。

边界：共享诊断状态；原生 message/UI 报错未单独验证。

## installer/internal/webviewhost/runtime_windows.go

函数/方法（括号内为基线行号）：`CheckRuntime` (13)。

调用/测试：embed/startup → startup_test.go。

边界：共享启动状态机；没有原生 runtime probe 专测。

## installer/internal/webviewhost/settings_windows.go

函数/方法（括号内为基线行号）：`configureController` (14)、`Configure` (49)。

调用/测试：WindowsSurface.Configure → settings_windows_test.go。

边界：fake COM vtable 的 HRESULT 全配置护栏，不是真实 controller。

## installer/internal/webviewhost/surface_windows.go

函数/方法（括号内为基线行号）：`comCall` (16)、`Resize` (38)、`Visible` (48)、`Prepare` (55)、`coreFromController` (62)、`Navigate` (73)、`Show` (87)、`Close` (103)、`NavigationResult` (108)。

调用/测试：WindowsSurface/comCall/NavigationResult → surface_windows_test.go、settings_windows_test.go、abi_test.go。

边界：COM fake、HRESULT、ABI、navigation failure 非 readiness；真实窗口/controller 的各 API 分支仍属集成边界。

## installer/main_windows.go

函数/方法（括号内为基线行号）：`main` (13)。

调用/测试：main → createShellWindow/embed/install；startup/completion/wiring tests。

边界：入口自身无直接单测；完整构建/安装运行证据。

## installer/stable_icon_windows.go

函数/方法（括号内为基线行号）：`shortcutIcon` (14)、`setShortcutIcon` (26)、`stabilizeWindowsShortcutIcons` (83)。

调用/测试：completion icon guard → r206_windows_test.go、r206_icon_test.go。

边界：TestR206IconOnlyChangesIconAndPreservesCreation 真实快捷方式；shared no-write guard。

## installer/uninstall/launch_windows.go

函数/方法（括号内为基线行号）：`commandParameters` (19)、`relocate` (29)、`loadWorker` (55)、`command` (83)、`exitCode` (94)、`runWithoutUI` (104)、`cleanup` (116)。

调用/测试：prepareLaunch/dispatch → uninstall/prepare_test.go、uninstall/uninstall_test.go TestCoreCommandPreservesUpgradeFlagsAndNSISTail。

边界：共享资源 payload/flags；self-copy/OpenProcess/wait/encoded cleanup/worker spawn 无专门原生单测，旧卸载与真实升级保留。

## installer/uninstall/main_windows.go

函数/方法（括号内为基线行号）：`main` (13)、`run` (15)、`showUI` (53)。

调用/测试：main/run/showUI → uninstall/uninstall_test.go TestSilentDispatchNeverCreatesWindow。

边界：共享 dispatchMode；实际 UI 入口未逐分支单测。

## installer/uninstall/settings_windows.go

函数/方法（括号内为基线行号）：`configureWebView` (5)。

调用/测试：configureWebView → WindowsSurface.Configure。

边界：主 installer 的 fake COM 配置测试不等于 uninstall wrapper 原生集成。

## installer/uninstall/webview_windows.go

函数/方法（括号内为基线行号）：`cacheDirectory` (26)、`embed` (33)、`emit` (78)、`fail` (85)、`onMessage` (91)、`uninstall` (151)。

调用/测试：onMessage/uninstall → uninstall/uninstall_test.go 的 data deletion/completion/progress。

边界：共享删除/进度/完成策略；WebView callbacks/native close 不是 fake 测试的原生证据。

## installer/uninstall/window_windows.go

函数/方法（括号内为基线行号）：`enableDPIAwareness` (87)、`createShellWindow` (103)、`shellWndProc` (150)、`dispatch` (210)、`drainUI` (220)、`drag` (230)、`show` (235)、`runMessageLoop` (241)、`acquireSingleInstance` (254)。

调用/测试：uninstall window lifecycle → shared window/startup policy。

边界：没有直接 Win32 window/mutex/message loop 单测；卸载实装另证。

## installer/update_parent_windows.go

函数/方法（括号内为基线行号）：`legacyUpgradeParent` (13)、`legacyUpdateWindows` (60)。

调用/测试：webview onMessage update route → message_wiring_test.go、r201_test.go recovery policy。

边界：静态 route/shared recovery；WM_CLOSE/parent enumeration 没有专门原生单测，实际升级保留。

## installer/update_shortcuts_windows.go

函数/方法（括号内为基线行号）：`call` (20)、`withShortcutObject` (31)、`shortcutTarget` (66)、`shortcutTargetsMatch` (83)、`retargetShortcut` (94)、`snapshotWindowsShortcut` (114)、`copyShortcutFile` (142)、`restoreShortcutFile` (160)、`keepShortcutRegistryRoots` (207)、`readKeepShortcutsRegistry` (223)、`repairKeepShortcutsRegistry` (244)、`ensureKeepShortcutsValue` (260)、`newWindowsShortcutUpdate` (276)。

调用/测试：newWindowsShortcutUpdate/shortcut guard → r205_windows_test.go、r205_test.go。

边界：真实 COM/registry round trip、alias、KeepShortcuts、COM already initialized；共享 restore/creation guards。完整安装链保留实装证据。

## installer/webview_windows.go

函数/方法（括号内为基线行号）：`embed` (30)、`emit` (108)、`fail` (116)、`validatePath` (132)、`onMessage` (149)。

调用/测试：embed/onMessage → message_wiring_test.go、startup/state machine tests。

边界：AST wiring 与 fake WebView；真实 WebView2/controller 生命周期由安装运行补充，不冒充单元 API 覆盖。

## installer/window_windows.go

函数/方法（括号内为基线行号）：`enableDPIAwareness` (89)、`createShellWindow` (105)、`shellWndProc` (152)、`dispatch` (212)、`drainUI` (222)、`drag` (232)、`show` (237)、`runMessageLoop` (243)、`acquireSingleInstance` (256)。

调用/测试：completion/message wiring/window abstractions → completion_test.go、message_wiring_test.go。

边界：fake window/AST wiring；DPI/mutex/message loop/drag 未由单测逐分支证明。

## 后端恢复的具体名称

### backend/update_test.go

- `TestUpdateVersions`：83
- `TestUpdateDevDisabled`：100
- `TestUpdateMirrorFallbackSingleFlightAndLastError`：110
- `TestUpdateCacheTTLAndNoDowngrade`：152
- `TestUpdateBusyOperationsRejectOverlappingWork`：200
- `TestUpdateManifestAndSettingsValidation`：311
- `TestUpdateVersionNamedAssetAndLegacyCompatibility`：339
- `TestUpdatePublicManifestCheckAvailableAndReady`：367
- `TestUpdateCheckFailureDiagnosticsPersistSafeStages`：415
- `TestUpdateDownloadHashRetryAndCleanup`：467
- `TestUpdateResumeReprobesEachMirrorAndHashesPrefix`：511
- `TestUpdateCancelAndSpaceMargin`：564
- `TestUpdateFiveSecondSpeedWindow`：596
- `TestUpdateApplyRehashPortableMinimumAndCommand`：609
- `TestUpdateProgressSSEAndCleanup`：653
- `TestUpdateAfterUpgradeCleanupAndMinimum`：686
- `TestUpdateProgressAndStalledSourceFallback`：739
- `TestUpdateHTTPStatusAuthAndInitialEventSnapshot`：805

### backend/r204_key_test.go

- `TestR204KeyPriority`：42
- `TestR204KeySaveAndClear`：65
- `TestR204KeyMigration`：118
- `TestR204KeyRuntime401AndPrivacy`：160
- `TestR204CredentialChangeDetachesOldSpecialistFlight`：200

### backend/r201_test.go

- `TestR201ReadOnlyCameraAndRelockRetry`：17
- `TestR201MissingLCUCameraIsNotPresent`：63
- `TestR201InGameAndNoneNeverUnlock`：71
- `TestR201TimingConsumedOnceAndInvalidDeleted`：100
- `TestR201ProbeWindowCancelsSlowRoutes`：134
- `TestR201BothUpdatePathsPassParent`：188
- `TestR204SubsetWithoutPoolChampionNeverSelects`：195
- `TestR201SubsetConfiguredStrategies`：212
- `TestR201CanceledProbeStillFallsBackAfterDownloadError`：241

### backend/r198_test.go

- `TestR198CameraSingleFieldPatchAndSave`：69
- `TestR198CameraAlreadyTargetDoesNotPatch`：84
- `TestR198CameraFileBytesPreserved`：92
- `TestR198MalformedJSONIsRejected`：106
- `TestR198ReadOnlyStillPatchesLCU`：113
- `TestR198NoWritesDuringGame`：121
- `TestR198NoneDoesNotReadOrWrite`：138
- `TestR198WASDNeverChanges`：147
- `TestR198UnsafeFilePathsRejected`：162
- `TestR198LobbyOnlyLCUAndPreferencesRetained`：191
- `TestR198GameStartRunningProcessSkipsFilesAndVerifyFailure`：216
- `TestR198JSONShapesAndAmbiguousFields`：236
- `TestR198InGameSnapshotReportsActualFiles`：249
- `TestR198FinalRecheckAfterStageProbe`：274

### backend/accept_focus_trace_test.go

- `TestAcceptFocusPreferencesNeverLogTokensOrStrings`：16
- `TestAcceptFocusHelpOnlyCandidateSymbols`：33
- `TestAcceptFocusHistoryBoundedAndRedacted`：41
- `TestAcceptFocusExportRetainsRotationAndRejectsSymlink`：62
- `TestAcceptFocusHelpContractHasOnlySafeMetadata`：89
- `TestAcceptFocusCanceledJobHasTerminalRecord`：96
- `TestAcceptFocusRequestHasExactStatusAndSharedTrace`：119
- `TestAcceptFocusSamplerStopsOnCancellationAndSingleFlight`：146
- `TestAcceptFocusExportInspectionReadOnlyAndRedacted`：175

### backend/main_test.go

- `TestClientLaunchRejectsUnknownInstallation`：488
- `TestMergeClientLaunchCandidatePreservesOrderAndDeduplicates`：498
- `TestWindowsClientDetectionUsesOrderedProductionBuilder`：525
- `TestBuildDetectedClientInstallationsPutsShortcutsLast`：542
- `TestLaunchClientCandidatesFallsBackInOrderAndStopsOnSuccess`：588
- `TestLaunchClientCandidatesReturnsSanitizedFailures`：614
- `TestOfficialLoginLaunchUsesOnlyDetectedTCLSAndRecordsSafeDiagnostics`：635
- `TestOfficialLoginLaunchRejectsCredentialFieldsBeforeLaunching`：687
- `TestOfficialLoginLaunchRejectsTrailingJSONBeforeLaunching`：712
- `TestOfficialLoginLaunchRejectsOversizedBodyBeforeLaunching`：737
- `TestClientLaunchSerializesRequestsAndAppliesSuccessCooldown`：760
- `TestClientLaunchFailureReleasesServerLock`：804
- `TestOfficialLoginLaunchFailureDoesNotLogUnderlyingError`：830
- `TestClientInstallationsDoNotExposeExecutableField`：864
- `TestClientInstallationScanDiagnosticDoesNotExposePaths`：880
- `TestClassifyClientShortcut`：908

### backend/riot_client_test.go

- `TestRiotClientCommandLineUsesIndependentCredentials`：15
- `TestPowerShellProcessOutputParserRejectsCorruption`：44

### backend/r86_disk_test.go

- `TestR86EmptyProcessDiscoverySkipsDiskButErrorsRetainFallback`：17

### backend/client_installations_windows_test.go

- `TestSplitRegistryPathSupportsNativeTencentKeys`：11
