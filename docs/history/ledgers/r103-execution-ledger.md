# R103 执行账本：proseed LRU 保护测试盲区

> 证据已于 R135 移出工作区，见提交 `b62bca1b9671bccd8c3cf9f7079096d4b33a6fa0`。

日期：2026-09-17

## 范围与约束

本轮严格按 `WORKLIST-R103-LRU-PROTECTION-TEST-GAP.md` 执行，只补测试覆盖和可重复的变异校验脚本，没有修改 `accountBinaryDiskWriteLocked`、`pruneDiskLocked` 或其他生产逻辑，也没有删除或替换 R99 原有的 `TestR99SeedAnchorSurvivesLRUAndHasOnlyStableField` 1030 次压力测试。

新增文件：

- `r103_test.go`
- `scripts/r103-mutation-check.py`
- `docs/r103-validation/mutations/` 下的 baseline、mutant 日志和 `matrix.json`

## 新增测试

### `TestR103ProseedEntriesNeverSelectedForEviction`

测试在临时目录中创建五个真实的一字节文件，并手工建立 `strictEntries`：两个最早的 `proseed-` 条目、三个时间明显更晚且彼此不相同的普通条目。调用覆盖已有的 `ordinary-keep.json`，因此不会引入第六条记录，但会执行两轮预算驱逐。测试同时断言：

- 最终条目数和字节数分别为 3 和 3；
- 两个 `proseed-` 文件仍在 map 和磁盘上；
- 被删除的正好是时间最早的两个普通文件；
- 真实 `os.Remove` 路径和索引删除路径都生效。

`binaryDiskEntry.modified` 使用 2000、2001、2020、2021、2022 年的绝对时间戳，避免依赖 `time.Now()` 的调用时机或文件系统时间精度。保留两条受保护记录并触发两轮驱逐，可以同时检测“删除保护条件”和“只保护一次”的变异。

### `TestR103AllCandidatesProtectedReturnsErrorNotEviction`

测试只放入两个 `proseed-` 文件，将 `diskMaxEntries` 设为 1，并通过覆盖已有的 `proseed-a.json` 触发预算检查。测试精确断言错误消息为 `protected cache entries exceed disk budget`，两个 map 条目和两个真实文件均保留，`strictBytes` 仍为 2。这样可以检测找不到可驱逐候选时被错误地直接返回 `nil` 的变异。

## P3 调用链核验

`newRiotIdentityCache` 通过 `newPublicBinaryCache` 创建并强制 `strictDisk=true`（`riot_identity_cache.go:16-21`、`champion_images.go:17-29`）。`riotIdentityKey` 仅在 `proseed:v1:` identity 上生成 `riot-identity-v1|proseed:` 命名空间（`riot_identity_cache.go:30-38`），`championDataCache.pathFor` 再将该 key 映射为 `proseed-*.json`（`champion_cache.go:244-253`），写入最终进入 `accountBinaryDiskWriteLocked`。

`pruneDiskLocked` 的 `proseed-` 过滤位于 `strictDisk=false` 的普通磁盘 prune 路径（`champion_cache.go:430-466`）。当前调用链没有让 `strictDisk=false` cache 接收 `proseed-` 文件名的路径；`proseed-rank-v1:` 和 `proseed-lastmatch-v1:` 只是普通 identity key，经 hash 后不满足文件名保护前缀。该过滤因此是当前不可达的防御性代码，本轮保留它，也不为它增加测试。

## 验证结果

- `GOCACHE=/tmp/deep-legends-go-cache GOTMPDIR=/tmp/deep-legends-go-tmp go test -count=1 -v -run '^TestR103' .`：通过。
- `go test -count=1 -v -run 'TestR103|TestR99SeedAnchorSurvivesLRUAndHasOnlyStableField' .`：通过；R99 1030 次压力测试仍通过。
- `go test -race -count=1 -run 'TestR103|TestR99SeedAnchorSurvivesLRUAndHasOnlyStableField' .`：通过。
- `go test -count=1 ./...`：在获准的沙箱外执行并通过（`ok lol-loot-assistant`，约 118 秒）。沙箱内首次运行因现有 `httptest` 用例监听 IPv6 loopback 被系统拒绝，未将该失败归因于本轮改动。
- `git diff --check`：通过。
- `python3 scripts/r103-mutation-check.py`：两条 mutation 均 `KILLED`，两条 baseline 均退出码 0；mutant 均由对应 R103 测试失败，非编译错误、panic 或超时。

第二条 mutation 保留 `errors` 引用后将实际返回改为 `nil`，避免把未使用 import 造成的编译失败误算为测试杀死 mutation。脚本使用 Go overlay 和临时目录，未改写 checkout。

## 生产逻辑检查

本轮没有编辑生产文件。执行后的 `git diff -- binary_disk_budget.go champion_cache.go riot_identity_cache.go` 只反映本轮开始前已经存在的 R99–R102 工作区改动；R103 新增内容仅位于测试、脚本和验证文档中。

## 未执行项

未配置 Riot 真实 API 凭据，也未执行真实网络刷新或客户端验收；这些与 R103 的本地 LRU 保护测试无关。全量 `go test -race ./...` 的沙箱外审批在本轮超时，因此只执行并记录了不需要 loopback 的 R103/R99 定向竞态测试；没有把全量竞态测试宣称为通过。
