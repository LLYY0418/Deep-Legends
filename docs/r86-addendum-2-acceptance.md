# R86 第二追加工单执行与验收

2026-09-13。逐项对应根目录 `WORKLIST-R86-ADDENDUM-2-NEW-FINDINGS.md`；上一份追加工单的四项不重复改动。

| 条目 | 状态 | 本轮结果 |
|---|---|---|
| R86-ADD2-1 设置锁未移除写位 | 已排查、加固并通过本机验收；原环境根因未确认 | 原代码已有 `os.Chmod(resolvedFile, 0444)`，未发现缺失调用或布尔解码/路径分支错误。增加读回状态校验，不一致或无法确认时返回 503，而不是报成功；保留 chmod、路径边界及符号链接安全检查。 |
| R86-ADD2-2 缓存目录被测试复制（可选） | 已完成 | 测试复制入口提前过滤 `.gocache`、`.gomodcache`、`node_modules`、`dist`，覆盖嵌套模块；完成 64 MiB 假缓存对照实验与四种目录的独立变异验证。 |

## ADD2-1：没有把未复现的故障当成已查明根因

- 在**未修改的 `a26c41f` 隔离副本**，原设置锁测试连续 10 次通过；随后完整 `go test -json ./...` 也通过，1447 pass / 14 skip / 0 fail。原始测试的符号链接后半段和祖先目录逃逸测试均执行通过，未跳过。
- 主代理及只读探子检查了设置路径、chmod、JSON 解码与其他文件写入，没有找到另一个生产路径重建 `PersistedSettings.json` 或覆盖权限的证据。
- 因此，本次不能确认工单所述 `/sessions` 审计环境中为何仍为 `0600`，也没有把权限常量机械改成 `0400` 来制造“修复”。**该环境的根因仍待其文件系统/挂载权限语义及当次运行证据核验。**
- 已确认的防御缺口是：旧 handler 在 chmod 返回 nil 后，不检查原有 `readRigStatus` 的读取结果就记录成功。本轮复用这次读取，要求 `SettingsKnown` 且锁定状态与请求一致；失败记录 `verify-failed` 并返回 503。没有增加额外上游读取或改变原有 0444/0644 策略。
- 新测试在第二次安装目录读取时确定性恢复旧权限或删除文件，分别覆盖加锁/解锁的四种失效情形，无 sleep、全局 hook 或生产测试注入点。原 Tencent 测试增加解锁回读、双向符号链接拒绝、目标权限不变，并避免 Stat 错误时解引用 nil。
- 修改后全部设置锁测试在 `-race -count=20` 下通过；三项顶层测试及四个子用例各通过 20 次，无跳过。独立把锁权限改为 0600、旁路新校验，均被真实断言检出。

这里验证的是读取时的只读权限状态；没有声称 chmod 能阻止管理员改权限、目录内原子替换，或读取后再次发生的外部修改，也没有重新设计设置锁机制。

## ADD2-2：实际复制量与指纹对照

对指纹测试实际使用的源码夹具复制范围造缓存，不复制整个 Git 仓库或真实依赖。两个层级的 `.gocache` 共 512 个文件、64 MiB，其余三类目录共 6 个文件、6 KiB。普通源码、嵌入资源与 `.gocache-notes` 相似名称目录均保留。

| 观测 | 无过滤 | 有过滤 |
|---|---:|---:|
| 文件数 | 806 | 288 |
| 字节数 | 74,789,553 | 7,674,545 |
| 单次复制耗时 | 174.72 ms | 57.86 ms |
| 指纹 | `86290076ea08` | `86290076ea08` |

实际少复制 518 个文件、67,115,008 字节。耗时是本机单次实验值，不是跨机器保证或 CI 阈值；文件数/字节数/目录不存在及指纹相等均有确定性断言。

日常回归只造小缓存，避免修复缓存复制问题后又强制每次测试复制 64 MiB。大缓存实验显式运行：

```sh
DEEP_LEGENDS_LARGE_CACHE_TEST=1 node --test desktop/build-fingerprint.test.cjs
```

生产 `desktop/source-fingerprint.cjs` **没有修改**。假缓存采用 Go 缓存的无扩展名文件形态，不属于现有 installer 指纹输入；不声称已改变生产遍历器对任意扩展名缓存文件的处理。现有 R70 的源码变化失效、测试与生成二进制排除护栏完整保留。

## 验收、复现与边界

- 本轮实现/测试文件仅：`game_settings_lock.go`、`game_settings_lock_test.go`、`r50_suite_test.go`、`desktop/build-fingerprint.test.cjs`。
- 共享工作区有并行 R87 改动。本轮完整验证使用 **a26c41f + 上述四文件** 的隔离副本，复用本地 Go 缓存与 Node 依赖；未回滚、覆盖或混入 R87 文件。通过结果不等同于已验收并行 R87 合并后的工作树。
- 修改后 Go 全量：1452 pass / 14 skip / 0 fail；JS 全量：594 pass / 1 Windows 专属测试 skip / 0 fail。build、vet、installer 独立 test/vet、根模块及 installer 的 Windows 交叉 vet、Go 格式与 diff 空白检查均通过。完整 race 与精确耗时见机器报告。
- Windows 原生运行、远端 CI 和原 `/sessions` 审计环境未执行，不以交叉 vet 替代这些结论。
- 两次只读独立审查没有发现本轮可行动回归；结论由主代理结合源码及实际验证复核。

完整命令、计数、环境及四个文件 SHA-256 见 [verification-summary.json](r86-addendum-2/verification-summary.json)。六种独立变异的原文、替换及失败断言见 [mutation-results.json](r86-addendum-2/mutation-results.json)。可在选定的隔离副本重跑：

```sh
GOCACHE=/path/to/reusable/cache python3 docs/r86-addendum-2/run-mutations.py \
  --root /path/to/isolated/checkout --output /tmp/r86-add2-mutations.json
```

变异使用 Go overlay 与运行后删除的 JS 临时文件；构建错误、超时、其他异常不算作护栏成功检出。用户原工单保留原文，不改写其中的发现为已确认事实。
