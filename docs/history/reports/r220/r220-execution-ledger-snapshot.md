# R220 执行账本

工单：`WORKLIST-R220-REMOVE-CALIBRATION-RAW-DATA-AND-V3-RESIDUE-TESTS-SKIP-WITHOUT-SAMPLES.md`。执行日期：2026-10-05。版本 0.12.71；只改测试和清理资料，不改生产源码，不构建、打包或发布。

## 删除前清单（删除前实测）

|精确路径|文件数|逻辑字节数|MiB|Git跟踪文件数|
|---|---:|---:|---:|---:|
|`docs/history/reports/r211/opgg-samples`|368|160598347|153.159|0|
|`docs/history/reports/r211/opgg-validation-new-accounts`|282|132475216|126.338|0|
|`docs/history/reports/r216/opgg-holdout`|853|384324607|366.521|0|
|`docs/history/reports/r217/opgg-holdout`|591|269160331|256.691|0|
|`docs/history/reports/r216/account-selection`|140|43576850|41.558|0|
|`docs/history/reports/r217/account-selection`|122|29179579|27.828|0|
|`docs/history/reports/r217/training-curves.json`|1|25744028|24.551|0|
|`docs/history/reports/r217/training-selected-go.json`|1|4299787|4.101|0|
|`docs/history/reports/r217/mode-interval-probe`|18|6517389|6.215|0|
|`scripts/r211-refine-calibration.py`|1|6335|0.006|0|
|`scripts/r211-refine-official-calibration.py`|1|7802|0.007|0|
|`scripts/r211-evaluate-new-accounts.py`|1|3901|0.004|0|

合计 2379 份文件、1,055,894,172 字节（0.983 GiB）。这里使用文件逻辑字节数；原始数据与三个 v3 脚本均未被 Git 跟踪。

## P1 测试改造与删除前验收

两个 260 局测试共用 `r216AssertGolden`：仅原始文件读取返回 `os.IsNotExist` 时记为缺失，仍逐一核验全部存在的样本；任何已有样本读取错误、JSON 解码或评分不一致仍失败。核验完其余样本后，有缺失才用工单原文 `t.Skip("校准原始样本已清理，见 R220")`。Golden 自身缺失/损坏和非 260 条仍失败。

全量本来就有手动桥接/环境条件跳过项，不能将其改成通过来凑“只有两个 SKIP”。先对已有 skip guard 的测试记录当前基线，再对临时隐藏后的全量检查：只新增工单指定的两项，独立 gameplay golden 与候选/HTML/规则测试须 PASS。

## 保留与引用决定

`scripts/r211-score-calibration.py` 检索到两个直接脚本引用：`r211-refine-calibration.py:12`、`r211-refine-official-calibration.py:12`；另有 R216 工单第89行说明其 v2 参考实现。按“有引用就保留”保留该文件；上述两份 v3 引用脚本按清理表删除。其余未列入删除表的 scripts 全部保留。

两份 manifest 位于待删 holdout 目录中，将保持字节/SHA 不变，移到各报告根目录 `opgg-holdout-manifest-sha256.json`，并在原账本记录旧路径与新路径。`collection-verification.json` 与 `collection-integrity-audit.json` 等小报告不改写。

`backend/testdata/r216/`、`r217/` 全部夹具；R211/216/217 删除表外报告、截图、候选参数、评估结果与 started 标记、规则、账号清单；R216 `kr-champion-table-probe/`、所有截图与 layout 全部保留。删除前对这些文件以及生产源码/前端资源共706份记录哈希（含未修改的脚本）（`retained-hashes-before.json`）。

P1 已通过并进入P2；执行时序和删除后记录见后续小节。

### P1 边界验证记录

- 样本完整时两个260局测试 PASS；既有条件跳过基线24项，见 `test-runs/baseline-skips.json`。
- 临时缺少一份原始match文件：两个指定测试 SKIP；其余259份仍逐一核验。原文件随后恢复。
- 同时缺少一份样本并让另一份存在但损坏：两个指定测试均 FAIL，没有 SKIP。该失败是主动负向验证，样本随后恢复；没有留下损坏原始文件。见 `test-runs/corrupt-existing.json`。
- P1 临时目录改名仅针对R211两目录与R216 holdout，映射见 `temporary-directory-renames.json`；全量测试结束恢复原目录，验收成功前不删除。

## P1 全量验收与 P2 已执行

临时改名三个原始目录后 `go test -count=1 -json ./backend`：通过，开始 2026-10-05T17:02:58.802402+08:00，耗时 256.092 秒。恢复三个目录后才进入删除。与24项既有条件跳过基线相比，新增仅：

- `TestR216PythonV2MigrationConsistency260`
- `TestR216PythonV21GoldenConsistency260`

独立 `TestR216LegacyV2JSGolden20` / `TestR216V21GameplayGolden20`、生产冻结参数、R216英雄表HTML和R217冻结查表测试均PASS，不依赖原始样本的 golden 对照持续运行。详细事件、SKIP名及输出最后5行在 `test-runs/hidden-full.json`，没有新失败。

### 原字节保留的清单

|原路径|保留路径|字节数|SHA-256|
|---|---|---:|---|
|`docs/history/reports/r216/opgg-holdout/manifest-sha256.json`|`docs/history/reports/r216/opgg-holdout-manifest-sha256.json`|137441|`232ead1a63d388571c3152c28e1b50f7f7862e200eb6abf829a84aec59c50f71`|
|`docs/history/reports/r217/opgg-holdout/manifest-sha256.json`|`docs/history/reports/r217/opgg-holdout-manifest-sha256.json`|95146|`dacb2ba0c0eb837d09bead124bb3e0687f6b89feee2369f869f7432af344d146`|

两份清单仅移到报告根目录，未改写字节。`collection-verification.json`、`collection-integrity-audit.json` 及所有评估结果/started标记仍是原字节。采集清单描述删除前的历史文件集合，现已不能用来校验磁盘上的原始数据。R216/R217一次性评估脚本头部已注明“评估已完成，原始数据已删除，不可再运行”；没有执行或导入这两个脚本。

### 删除后实测

清单12条路径均已不存在（目录/文件剩余字节数0），无临时改名目录残留。仅对精确清单执行删除，没有通配清理报告目录。

扣除保留的两份清单后，移除 **1,055,661,585 字节（0.983 GiB）** 原始数据和残留。

|剩余报告目录|文件数|字节数|MiB|
|---|---:|---:|---:|
|`docs/history/reports/r211`|45|4727188|4.508|
|`docs/history/reports/r216`|25|10407128|9.925|
|`docs/history/reports/r217`|22|2528570|2.411|

删除后立即核验706份保留文件哈希，全部一致；额外验证两份移出的 manifest SHA保持。删除前后精确计数分别见 `deletion-before.json`、`deletion-after.json`。

`.gitignore` 已加入工单指定的两个目录规则，结论 JSON 和截图仍可入库。R211索引行已注明 v3 研究残留由R220清理；同号的另一份外服适配工单不在本次范围，索引状态不动。

## 收尾验证

删除后各项实际结果已补齐于末尾：Node/overview/vet通过；Go全量因并行外服改动失败并被panic中断，仍待后续复验。不构建、打包或发布，版本仍为0.12.71。

### 并行工作区变化与验证边界

P1成功、删除完成并核验706份保留文件后，另一任务“执行指定工单”（同cwd，正在执行同号的外服适配工单）于17:08:47起修改了Riot生产源码。17:09:35删除后vet报 `backend/mastery_details.go:81:27: undefined: p`。这些改动不属于本次清理，本次没有修改或回退它们；源码哈希变化另记 `concurrent-source-changes.json`。因此不把P1通过沿用为删除后的最终Go通过。

清理后Node串行全量929/929通过（17:09:30开始，45.766秒）。overview-render仍在跑；删除后Go全量将记录实际结果，待并行源码可编译再完成最终验收。保留报告、参数、golden和未列入清理的脚本哈希仍一致。

### 协调与后续复查

用户授权仅四项协调内容后，已向外服适配任务发送一条信息：本清理任务占用 `docs/r220-execution-ledger.md` 与 `docs/history/reports/r220/`；对方使用 `r220-region` 账本/报告；附上已记录的编译错误；两个R220索引行各改各自一行。此后索引只做本清理行的定点修改。

删除后Node全量929/929、overview-render55/55均PASS，分别45.766秒和337.074秒。17:17:10再次运行vet通过（1.369秒）；首次编译失败日志保留、来源为并行外服改动。删除后Go无缓存全量已在源码恢复可编译后重新运行，尚在等待结果。

## 最终收尾记录与失败归属

清理动作、保留与新增SKIP验证完成；**当前共享工作区的删除后Go全量未通过，不能记成“Go全量通过”或沿用P1的通过记录。** 用户授权可依据 `concurrent-source-changes.json` 区分并行来源，本次记录如下。

### 删除后Go全量（实际失败）

命令 `go test -count=1 -json ./backend`。开始 **2026-10-05T17:18:01.900052+08:00**，耗时 **120.859秒**，退出码 **1**。第一次删除后运行在17:12:32即编译失败（`go-full-first.json`）；编译恢复后本次全量运行了测试，但被R208 panic提前中断，不能声称已验证后续所有测试或SKIP集合。

|失败测试/子例|实际错误|
|---|---|
|`TestR69LiveHistoryStatesDistinguishUnavailableEmptyAndFailed/empty`|gameplay_test.go:4577：cached=false, want true|
|`TestR66LivePlayerMatchesCachePreventsRepeatedLCUReads`|gameplay_test.go:4823：expired cache did not reload: calls=1|
|`TestOPGGCurrentPageFreshnessAndReferences`|opgg_player_page_test.go:63：fresh=false（err=nil）|
|`TestR102NewlyRankedAccountResetsToNormalTTL`|r102_test.go:529：early rank ignored null <nil> 2|
|`TestR104RealOPGGFixtureProvidesRevisionTimeAndZeroRiot`|r104_test.go:110：cache miss|
|`TestR208RelayApplicationSharedAcrossProviders`|riotHTTPErrorBody(nil) 在 riot_api.go:2020 panic，整个全量提前结束|

上表六个顶层测试在17:02:58的P1“原始目录不可见”全量中全部PASS（包括R69 empty子例）。这些测试不调用 `r216AssertGolden`，不读取本次清理的采集目录：历史缓存使用HTTP测试桩，OP.GG页测试使用其既有夹具，中转测试使用mock provider。P1后本次未再改任何Go代码；保留夹具/报告/脚本未变，而并行外服任务持续改动生产区域路由、缓存与中转源码，差异在 `concurrent-source-changes.json`。**这些失败归入并行外服改动；清理任务不越范围修改或回退对应生产逻辑。** 两次全量、首次vet编译错误及后来vet通过记录均保存，不删失败日志。

本次Go全量输出最后5行（`-json`实际输出）：

```text
{"Time":"2026-10-05T17:20:02.679272+08:00","Action":"output","Package":"lol-loot-assistant/backend","Test":"TestR208RelayApplicationSharedAcrossProviders","Output":"created by testing.(*T).Run in goroutine 1\n"}
{"Time":"2026-10-05T17:20:02.679283+08:00","Action":"output","Package":"lol-loot-assistant/backend","Test":"TestR208RelayApplicationSharedAcrossProviders","Output":"\t/opt/homebrew/Cellar/go/1.24.5/libexec/src/testing/testing.go:1851 +0x374\n"}
{"Time":"2026-10-05T17:20:02.685088+08:00","Action":"fail","Package":"lol-loot-assistant/backend","Test":"TestR208RelayApplicationSharedAcrossProviders","Elapsed":0}
{"Time":"2026-10-05T17:20:02.685136+08:00","Action":"output","Package":"lol-loot-assistant/backend","Output":"FAIL\tlol-loot-assistant/backend\t113.842s\n"}
{"Time":"2026-10-05T17:20:02.685164+08:00","Action":"fail","Package":"lol-loot-assistant/backend","Elapsed":113.842}
```

### 删除后清理专项与其他检查

- 清理专项：2026-10-05T17:21:02.378636+08:00开始，7.996秒，退出0；仅两个指定测试SKIP，5项独立golden/冻结参数/HTML/规则测试PASS。证明保留的小证据不会被缺样本Skip掩盖。
- `go vet ./backend`：2026-10-05T17:17:10.222024+08:00复查通过，1.369秒，无输出。首次因并行编译错误失败已另记；此通过只对应该时刻源码。
- `node --test --test-concurrency=1 backend/web/*.test.cjs`：929/929通过，2026-10-05T17:09:30.338361+08:00开始，45.766秒，0 SKIP。
- `node --test desktop/overview-render.test.cjs`：55/55通过，2026-10-05T17:10:57.291547+08:00开始，337.074秒，0 SKIP。
- 清理修改范围的 `git diff --check -- .gitignore docs/WORKLIST-INDEX.md backend/r216_score_test.go scripts/r216-evaluate-holdout.py scripts/r217-evaluate-keywords.py`通过；全仓检查在后续并行改动后报 `relay/riot-worker/worker.mjs:94` trailing whitespace，该中转文件不在本次清理改动内，未越范围修正。12条删除路径及所有临时改名路径不存在；两份manifest原字节SHA保持，保留报告、golden、参数、小报告与表外脚本保持。源码差异已按并行来源独立记录。

这些带时间的结果不用于宣称并行外服任务后来修改过的源码已验收。剩余事项仅为外服任务修好并稳定共享源码后，重新运行删除后的Go无缓存全量，并在最后Go改动后按INDEX记录；当前不能关闭这项全量验收。

没有新增采集、评分/关键词评估、参数优化、构建、打包或发布。版本保持0.12.71。
