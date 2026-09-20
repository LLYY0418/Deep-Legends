# WORKLIST-R86-ADDENDUM-2 验收后新发现

本工单是对 `WORKLIST-R86-ADDENDUM-ACCEPTANCE-FOLLOWUP.md` 四条追加工单的独立验收过程中，
**顺带测出的、与R86无关的问题**。四条追加工单本身已经逐条亲自动手做变异验证通过，不需要再处理，
详情见 `docs/r86-execution-ledger.md` 与本工单验收方的验证记录。

---

## R86-ADD2-1 国服设置锁没有真的锁上文件（真实功能缺陷）

**证据**：`go test ./...` 在当前 HEAD（`a26c41f`）跑出这个失败，**已确认与本轮任何改动无关**——
`game_settings_lock.go` 和 `r50_suite_test.go` 自 `7eb6393`/`ff3cf27`（R86 优化开始之前）就没被碰过：

```
--- FAIL: TestSettingsLockUsesTencentConfigAndRejectsSymlink (0.00s)
    r50_suite_test.go:275: settings mode = -rw-------, err = <nil>
```

测试逻辑（`r50_suite_test.go:268-276`）：调用 `a.handleSettingsLock` 对
`PersistedSettings.json` 加锁后，断言文件权限不再含写位（`info.Mode().Perm()&0o222 != 0` 应为 false，
即期望类似 `0o400` 这种只读权限）。实际文件权限还是 `-rw-------`（`0o600`），说明加锁调用没有真的去掉写权限。

**影响**：这个功能的目的是防止腾讯客户端热更新时把用户手动调整过的画面设置（分辨率、帧率等）覆盖掉——
"锁定设置"这个开关目前对用户来说是**摆设**，勾选后客户端仍然可以正常写入这个文件，起不到保护作用。

**改法**：找到 `handleSettingsLock` 实际执行 chmod 的那一步（应该在拿到 `settingsFile` 路径之后、
返回 200 之前），确认它是否真的调用了 `os.Chmod(path, 0o400)` 一类操作、有没有被某个条件分支跳过、
或者调用了但参数算错了权限位。**这是个已有功能的回归或从未生效过的bug，不是重新设计，先定位再修，
改动范围应该很小**（大概率是一行 chmod 调用缺失或权限位算错）。

**验收判据**：`go test -run TestSettingsLockUsesTencentConfigAndRejectsSymlink ./...` 转绿；
另外这个测试后半部分还测了"拒绝跟随符号链接"这个安全属性（测试名里的 RejectsSymlink），
修复时顺便确认没有破坏这部分——完整跑一遍这个测试函数而不是只看到第一个失败点就停。

**风险**：低（定位到具体缺失的调用后，改动应该很局部）。

---

## R86-ADD2-2（低优先级，可选）指纹测试拷贝了不该拷贝的构建缓存目录

**证据**：`desktop/build-fingerprint.test.cjs` 里 `"R70 fingerprint tracks embedded assets/build inputs
but excludes tests and generated binaries"` 这个测试用 `fs.cpSync` 把整个仓库树拷贝到临时目录再计算指纹，
拷贝范围**没有排除被 `.gitignore` 排除的构建缓存目录**（比如 `installer/.gocache`，62MB）。
验收过程中在磁盘紧张的沙箱环境下，这个测试对这个目录里的某个文件报了 `EIO` 导致失败：
```
error: "EIO, Input/output error '.../installer/.gocache/27'"
```
复现了两次、报错路径一致，判断是这次验收沙箱磁盘紧张（`/sessions`挂载点被挤到100%满）触发的环境性问题，
**不是产品代码bug**，但测试设计本身确实不必要地把几十MB的、跟"源码指纹"毫无关系的构建缓存也纳入了拷贝范围，
只要遇到磁盘紧张的环境就容易在这种大目录上出问题。

**改法（可选，不紧急）**：`cpSync` 时加 `filter` 选项排除 `.gitignore` 里列出的目录（至少排除
`.gocache`/`.gomodcache`/`node_modules`/`dist` 这几个已知的大缓存目录），减少这类测试对磁盘状态的敏感度。

**验收判据**：改完后在拷贝范围里造一个几十MB的 `.gocache` 假目录，确认测试执行时间和涉及的文件数明显下降，
且指纹计算结果不受影响（这些目录本来就不该参与指纹）。

**风险**：低。**这条是可选项，不是本轮验收发现的功能性缺陷，有精力再做。**
