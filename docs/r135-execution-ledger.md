# R135 执行账本：仓库瘦身

日期：2026-09-23。基线提交 `c2b678d0`；执行提交依次为 `0703538d`（缓存与归档）、`121b2626`（scripts）、`adb7972d`（desktop）、`00dc69ac`（Go）、`fe2230fe`（格式）。本账本记录实际执行范围；R135 没有修改产品行为、界面文案、接口、数据源、测试文件或 `desktop/package.json`。

## P0 基线

- 原工作区完整状态在 [`r135-validation/baseline-status.txt`](r135-validation/baseline-status.txt)；基线体积在 [`size-before.txt`](r135-validation/size-before.txt)。原有改动先提交为 `c2b678d0`。大于 1 MB 的未跟踪 `docs/design-mockups/r123-favorites-icon-banner-mockup.png` 没有加入基线；它是该路径的唯一副本，随后移至废纸篓 `~/.Trash/deep-legends-design-mockups-0923/` 保存。
- `go build ./backend` 在改动前就因输出名 `backend` 与现有目录冲突而失败：`go: build output "backend" already exists and is a directory`。使用等价的 `go build -o /tmp/r135-baseline-backend ./backend` 成功；`go vet ./...`、`go test -count=1 ./...` 成功（后者 204.017 秒）。
- `installer` 的 `go test ./...`、`go vet ./...` 均通过。`desktop/node_modules` 已齐备，依工单跳过 `npm ci`。
- 全量 Node：983 项，975 通过、5 失败、3 跳过。5 项失败为 `desktop/diagnostics-2024.test.cjs` 的 rarity UI、`desktop/overview-render.test.cjs` 的 R56 工具页，以及 `scripts/setup-only-build.test.cjs` 的三个 setup-only 案例（测试夹具没有 `backend/` 目录）。
- R100、R117 Chromium 验证均通过。初次沙箱浏览器启动受限，允许本机 Chrome 后基线验证通过。

## P1 本机缓存和构建产物

- `build-desktop.sh` 将默认 `GOCACHE` 改为 `go env GOCACHE`，后续构建阶段统一用导出的 `$GOCACHE`；Windows 构建脚本没有仓库内默认 Go 缓存路径。`.gitignore` 加入 `/backend/backend`、`/backend/backend.exe`、`/Claude outputs/`、`/.gomodcache/`。
- 删除可重建的 `.gocache/`、`.gopath/`、`.gomodcache/`、`dist/`、`backend/backend`、`lol-loot-assistant`、`desktop/backend/loot-service.exe` 和工作区 `.DS_Store`。删除旧环境文件 `.r90env`、`.r90env.sh`（这两个文件实际上已被 Git 跟踪，删除记录在 `0703538d`）。`desktop/node_modules/` 保留。
- `Claude outputs/` 没有删除，移至 `~/.Trash/deep-legends-claude-outputs-0923/`，由用户决定何时清空废纸篓。
- 本地 `git gc --prune=now` 后 `size-pack` 从 601.30 MiB 降到 201.18 MiB，garbage 从 1.70 MiB 降到 0；原始输出在 [`git-objects-before.txt`](r135-validation/git-objects-before.txt) 和 [`git-objects-after.txt`](r135-validation/git-objects-after.txt)。这一步没有改写 Git 历史。

## P2 Git 历史

**未执行，待用户决定。** 工单要求单独明确同意后才能改写历史并强推；本次只做了本地垃圾回收。

## P3 工单与账本归档

创建 [`WORKLIST-INDEX.md`](WORKLIST-INDEX.md)，索引 R82–R135。新归档完整清单：

- 工单移至 `docs/history/worklists/`，共 9 份：`WORKLIST-R117-FULL-PROJECT-OPTIMIZATION.md`、`WORKLIST-R118-INDEPENDENT-VERIFICATION-GAPS.md`、`WORKLIST-R123-FACADE-ICON-BANNER-COLLECTION-MIGRATION.md`、`WORKLIST-R124-FACADE-COLLECTION-UNOWNED-FILTER-GAP.md`、`WORKLIST-R125-FACADE-ICON-TILE-NATIVE-SIZE.md`、`WORKLIST-R126-CAREER-BACKGROUND-BASE-SKIN-MISMATCH.md`、`WORKLIST-R127-OVERVIEW-MATCH-LIST-SLOW-LOAD.md`、`WORKLIST-R130-COLLECTION-IMAGE-STALL-AND-FACADE-POLISH.md`、`WORKLIST-R134-STALL-LOG-REPORT-CROSS-RUN-CONTAMINATION.md`。
- 账本移至 `docs/history/ledgers/`：`r96`、`r97`、`r98`、`r103`、`r104`、`r106`、`r112`、`r113`、`r114`、`r117`、`r118`、`r123`、`r124`、`r126`、`r127`、`r130`、`r134` 的 `-execution-ledger.md`，共 17 份。
- 核查报告移至 `docs/history/reports/`：`r117-independent-verification.md`、`r118-independent-verification.md`。
- R119、R120、R121、R133 的真机待验收事项仍开放；R128、R129、R131 缺少完结账本；相关工单和证据保留。R134 已有完结账本，故归档。`AGENTS.md`、`CLAUDE.md` 写明归档规则、系统默认 Go 缓存与当前版本 0.12.18。移动涉及的注释与文档链接已同步修正。

## P4 已关闭工单的证据

从工作区删除下列已跟踪目录，文件仍能按 [`WORKLIST-INDEX.md`](WORKLIST-INDEX.md) 的提交号恢复：`docs/r96-validation/`、`docs/r97-validation/`、`docs/r98-validation/`、`docs/r103-validation/`、`docs/r104-validation/`、`docs/r106-validation/`、`docs/r117-validation/`、`docs/r123-validation/`。其中 `r98` 的 `chovy-overview.json` 是本次删除的 `desktop/r98-browser.cjs` 的输入，两者在同一提交 `adb7972d` 移除。被引用、真机待验收或工单仍在进行的证据保留；例如 `r130-validation/` 还用于 R133。

## P5 一次性脚本与不可达 Go 代码

- scripts 批次删除：`scripts/r76-mutations.py`、`scripts/r80-uninstall-mutations.py`、`scripts/r82-mutations.py`、`scripts/r83-startup-mutations.py`、`scripts/r84-cleanup-mutations.py`、`scripts/r103-mutation-check.py`。随即运行 `node --test scripts/*.test.cjs`：38 项，33 通过、3 个既有失败、2 跳过。
- desktop 批次删除：`desktop/r65-select-layout-probe.js`、`desktop/r73-mutations.py`、`desktop/r74-mutations.cjs`、`desktop/r98-browser.cjs`。随即运行 `node --test desktop/*.test.cjs`：260 项，257 通过、2 个既有失败、1 跳过。
- Go 批次删除 `deadcode -test` 仍不可达的 7 个函数：`backend/arena_live_grouping.go` 的 `arenaRosterDivisible`、`orderGroupsFromLiveClient`、`app.rememberArenaInference`、`app.carriedArenaSessionGroups`；`backend/pro_activity.go` 的 `proSeedTotalAccounts`、`proActivityAccountCount`；`backend/r99_probe.go` 的 `r99Request`。随即重新 `go build -o /tmp/r135-after-go ./backend`、`go vet ./...`、`go test -count=1 ./...`，全部通过；再次运行 `deadcode -test` 为零项。
- 分析器原始报告在 [`deadcode-prod.txt`](r135-validation/deadcode-prod.txt)、[`deadcode-with-tests.txt`](r135-validation/deadcode-with-tests.txt)、[`unused.txt`](r135-validation/unused.txt)。`staticcheck U1000` 的 13 项诊断没有被当作单独删除依据。生产分析中列出的其余 106 个函数在 `-test` 分析图中可达，均保留，名单见文末附录。
- R75、R89–R95、R99、R115 的部分候选脚本虽可能没有直接引用，但其工单仍有待验收项，不满足“三条全部满足”条件，保留。前端 JS/CSS 无可靠静态判定，本次未删。没有删除测试。

## P6 验收

| 项目 | 基线 | 清理后 |
|---|---|---|
| Go 编译、vet、完整测试 | 通过；测试 204.017 秒 | 通过；测试 195.516 秒 |
| installer 测试、vet | 通过 | 通过 |
| 全量 Node | 983 项；975 通过、5 失败、3 跳过 | 983 项；975 通过、5 失败、3 跳过 |
| R100 / R117 Chromium | 均通过 | 均通过 |
| Windows amd64 后端二进制 | 24,270,336 字节 | 24,269,824 字节（少 512 字节） |
| 后端 CLI `-h` | 15 行 | 去掉可执行文件名的 Usage 首行后，与基线逐行一致 |

`go build ./backend` 的目录名冲突在前后都存在；带 `-o` 的等价构建通过。`gofmt -l` 沿用 CI 的 `find` 命令检查为空；原有 3 个未格式化的 Go 文件仅做格式修正。奖池定向测试 `TestEmbeddedPoolHas554UniqueEntries|TestEmbeddedPoolMapsOneToOneWithoutOmissions` 通过。完整桌面构建、体积与文件数见下方最终记录。

### 测试图中可达、生产静态图中不可达的函数（106 个，保留）

这份清单来自删除前的生产 `deadcode` 报告扣除 `-test` 报告；静态可达性不等同于产品功能无用。

- `backend/accept_window_diagnostics.go`: `observeAcceptRequest`
- `backend/arena_live_grouping.go`: `validateArenaLiveGroups`, `arenaSessionOrderGroups`, `arenaOrderGroups`
- `backend/augment_contract_probe.go`: `augmentProbeMatches`, `augmentProbeContracts`, `augmentProbeOperation`, `augmentProbeTextPaths`, `augmentProbeRootShape`, `augmentProbeTraceID`, `app.collectAugmentContractProbe`, `augmentProbeAllZero`
- `backend/catalog.go`: `loadSnapshotWithClient`, `loadCollectionSnapshot`, `loadOwnedSkinIDs`, `extractOwnedIDs`
- `backend/champion_cache.go`: `championDataCache.pruneStaleHexdataBuilds`
- `backend/champions.go`: `parseArenaTeamCompositions`, `parseArenaStats`, `parseArenaAugments`, `flexibleJSONInt`, `balancedJSONObject`, `parseChampionRunes`, `runeAssetGroups`, `runeAsset`
- `backend/champions_structured.go`: `parseOPGGDepthRows`, `championProvider.structuredMetrics`, `championProvider.structuredCounters`, `arenaAugmentRows`
- `backend/claim_center.go`: `scanClaims`, `claimSignature`
- `backend/client_launcher.go`: `buildDetectedClientInstallations`, `clientInstallation.candidates`, `mergeClientLaunchCandidate`, `sameClientLaunchCandidate`, `launchClientCandidates`, `classifyClientShortcut`
- `backend/facade_icons.go`: `app.loadIconCatalog`
- `backend/gameplay.go`: `positionStatsGames`, `app.registerGameplayReference`, `app.resolveGameplayReference`, `app.loadGameplayRanks`, `loadGameplayHistory`, `gameplayOwnedTerminalItemIDs`, `gameplayNextItemSuggestionFromTrios`, `gameplayRecommendationsFromChampionDetail`, `gameplayLiveRecommendationTarget`
- `backend/hexdata.go`: `hexdataClient.circuitSnapshot`, `parseHexdataPostmatch`, `championProvider.loadHexdataHextechInsights`
- `backend/item_set_recommended.go`: `writeRecommendedItemSet`
- `backend/lcu.go`: `LCUClient.GetMediaBytes`
- `backend/lcu_api.go`: `championProvider.loadCommunityDragonLootMetadata`, `enrichLootItems`, `NewInventoryAPI`, `extractSkinAcquisitionDates`
- `backend/lcu_events.go`: `shouldRefreshForLCUEvent`
- `backend/opgg_insights.go`: `matchOPGGAverageTier`
- `backend/overview_current_game.go`: `app.parseOPGGCurrentGame`
- `backend/player_ability.go`: `gameplayAbilityAccumulator.addSide`, `buildGameplayAbilityProfileForQueueWithSnapshot`, `seasonAbilityStatsForQueue`, `gameplayAbilitySampleGamesForQueueWithSnapshot`
- `backend/position_contract_probe.go`: `positionProbeMatches`, `positionProbeResolve`, `positionProbeRefName`, `positionProbeType`, `positionProbeWalker.visit`, `positionProbeComplex`, `positionProbeResponseSchema`, `positionProbeResponseName`, `positionProbeContracts`, `positionProbeResponseKeys`, `positionProbeWalkResponse`, `positionProbeJSONType`, `positionProbeHasChildren`, `positionProbeOwnerPath`, `positionProbeTraceID`, `app.collectPositionContractProbe`, `positionProbeGet`, `positionProbeVerdict`
- `backend/pro_activity.go`: `app.enrichProActivity`
- `backend/pro_identity.go`: `proSourceTeamCode`, `proSourceTeamCodes`, `proMemberBadge`, `proDirectoryRoster`
- `backend/pro_players_ladder.go`: `enrichProLadderRanks`
- `backend/profile_facade.go`: `app.loadFacadeState`, `app.applyFacadeAction`, `app.applyFacadeActionResult`, `facadeSummaryHasTitle`
- `backend/qq101.go`: `championProvider.qq101Probe`
- `backend/rank_insights.go`: `rankScoreCacheKey`
- `backend/season_stats.go`: `localStore.saveSeasonStats`, `app.loadSeasonChampionStats`, `app.seasonScanPages`
- `backend/sgp_api.go`: `sgpProvider.entitlementsToken`
- `backend/sgp_auth_diagnostics.go`: `sgpResponseDiagnostic`
- `backend/update_platform.go`: `updateRootForExecutable`, `updateInstallationMatches`, `quoteUpdateArgument`, `updateCommandLine`
- `backend/watch_rules.go`: `newConvenienceRunner`, `loadConvenienceSettings`, `saveConvenienceSettings`, `watchRunner.broadcastPosition`

### 构建、体积及文件数最终记录

`DEEP_LEGENDS_KEY_MODE=public SKIP_NPM_INSTALL=1 bash build-desktop.sh 0.12.18` 完整成功。`backend-build`、`backend-fingerprint`、`packaged-runtime`、`packaged-fingerprint`、`release-receipt` 全部通过；`desktop/backend/loot-service.exe` 已重新生成（18,304,512 字节），构建后仓库内没有 `.gocache/`。生成的 `dist/desktop/Deep Legends Setup 0.12.18.exe` 为 101,739,520 字节，附有 `SHA256SUMS.txt`。本次使用 **public** 构建模式，没有嵌入本地 Riot key；需要韩服查询时在运行环境提供 `RIOT_API_KEY`。构建耗时约 3 分 9 秒，其中 NSIS 约 114 秒。

体积以 [`size-before.txt`](r135-validation/size-before.txt) 与 [`size-after.txt`](r135-validation/size-after.txt) 的 `du -sh` 输出为准。后表在完整打包后采集，因此包括新生成的 97 MB 安装包和 17 MB 后端；此前的同名构建产物已按 P1 删除过。

| 路径 | 清理前 | 清理及验证后 | 说明 |
|---|---:|---:|---|
| 工作区总计 | 约 13 GB（工单原始测量） | 508 MB | 包含重新生成的安装包 |
| `.gocache/` | 11 GB | 不存在 | 默认缓存改用系统目录 |
| `.git/` | 663 MB | 210 MB | 仅本地 GC；历史仍在 |
| `docs/` | 50 MB | 35 MB | 活跃工单证据仍保留，未达到工单预估的约 10 MB |
| `backend/` | 34 MB | 13 MB | 旧 Go 产物已删 |
| `desktop/` | 152 MB | 152 MB | `node_modules` 保留，后端重新生成 |
| `dist/` | 97 MB | 97 MB | 验证构建重新生成 |
| `Claude outputs/` | 54 MB | 不存在 | 原文件移至废纸篓 |
| `.gopath/` + `.gomodcache/` | 44 MB | 不存在 | 旧缓存已删 |

基线 `git ls-files` 为 2,103 个；清理后连同本账本与 `size-after.txt` 为 **1,913 个，净少 190 个**。逐项核对：28 份文档移动不改变数量；删除证据文件 186 个、脚本 10 个、旧环境文件 2 个；新增索引及分析报告 6 个、本账本与 `size-after.txt` 2 个。即 `2103 - 186 - 10 - 2 + 6 + 2 = 1913`。本工单再移入 `docs/history/worklists/` 也不改变数量。提交后 `git status --porcelain` 为空。
