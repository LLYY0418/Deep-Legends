# WORKLIST-R205：在线升级后桌面快捷方式位置变了（升级像重新安装）

诊断人：Claude（用户反馈 + 只读核对源码）。执行人：GPT。日期：2026-10-04。基线：0.12.67（`031715fd`）。

## 为什么单独开这一份

这项原本作为 R204 P11 追加，但仓库里提交的 R204 是追加之前的版本（`git show HEAD:docs/WORKLIST-R204-…` 只有 P7–P10），所以 0.12.67 没有执行。按"已执行工单发现遗漏就新开工单"的规则，单独放在 R205。


## 现象

用户反馈：在线升级后，桌面上 Deep Legends 快捷方式（以及相邻图标）的位置发生变化，感觉像重新安装。

## 源码核对（只读）

1. 软件发起的升级命令是 `"<setup>" /S /NCRC --updated /D=<目录>`（`installer/update.go upgradeSetupCommandLine`），**没有** `--no-desktop-shortcut`，`allowToChangeInstallationDirectory=false`，所以 electron-builder 的"保留快捷方式"机制应当生效：旧卸载程序带 `--keep-shortcuts --updated` 运行，新安装不删、不重建桌面和开始菜单快捷方式（`installSection.nsh` / `installUtil.nsh` / `uninstaller.nsh`）。`installer/uninstall` 也确实把 `--keep-shortcuts` 转发给内核卸载程序（`uninstall_test.go` 有用例）。
2. 机制生效的前提有两个：注册表 `KeepShortcuts=true`，且 `$appExe` 存在。如果任一不满足，旧卸载程序会删除快捷方式，新安装重新创建——新建的 .lnk 会被 Windows 放到第一个空位，图标位置就会变。
3. 此外 NSIS 在卸载和安装末尾各调用一次 `SHChangeNotify(SHCNE_ASSOCCHANGED)` 刷新桌面，可能让图标短暂重绘。
4. 以上是源码推断，**真机上到底是"快捷方式被删后重建"还是"只是重绘"，目前没有证据**。所以本项先加诊断，再按证据修。

## 修改

1. **诊断（必做）**：安装程序（Go 壳）在启动 NSIS 前、NSIS 结束后，各记一次当前用户桌面、公共桌面、开始菜单里 `Deep Legends.lnk` 的状态：是否存在、创建时间、修改时间、目标是否指向安装目录下的 exe（不记路径、不记账号）。同时读注册表 `KeepShortcuts` 是否为 `true`。新版本启动后写一条 `update_shortcut_state`：`desktop_before/after`、`start_menu_before/after`、`created_time_changed`（布尔）、`keep_shortcuts_reg`。
2. **保护（诊断显示快捷方式被重建时）**：Go 壳在 NSIS 启动前把现有桌面快捷方式文件复制到临时目录；NSIS 结束后，如果该快捷方式的创建时间变了，或者文件不存在，就用保存的原文件覆盖回去（同名同目录，保留原有 AppUserModelID 与目标；目标路径若已变化则只改目标）。Explorer 按文件名记图标位置，同名文件恢复后位置通常会回来。覆盖前先确认目标 exe 存在。
3. **`KeepShortcuts` 缺失时补写**：升级前检测到注册表没有 `KeepShortcuts=true`，但桌面快捷方式存在，则先补写这个值，让旧卸载程序保留快捷方式。
4. 不要新增用户可见文字；"升级窗口只显示升级、不显示安装"的样子（R201）保持不变，升级要跑旧卸载程序 + 覆盖安装是 NSIS 的固有流程，这里不改。

## 测试

1. Go：快捷方式快照函数在存在 / 不存在 / 创建时间变化三种情况下输出正确的 `created_time_changed`。
2. Go：NSIS 结束后快捷方式被删 → 用快照恢复；未变化 → 不动；目标 exe 不存在 → 不恢复。
3. Go：`KeepShortcuts` 缺失且快捷方式存在 → 升级前补写；已存在 → 不写。
4. Node（`installer-nsh.test.cjs`）：升级命令行不含 `--no-desktop-shortcut`，仍带 `--updated`。

## 真机验收（用户）

升级后看桌面图标位置是否保持；导出日志里 `update_shortcut_state` 的 `created_time_changed` 和 `keep_shortcuts_reg`，据此判断是否还需要继续改。

## 收尾

- `go test ./installer/... -count=1`、`go test ./backend -count=1`、`node --test backend/web/*.test.cjs desktop/*.test.cjs`、`go vet ./backend`、`git diff --check` 全绿；变异：去掉快照恢复 → 测试 2 FAIL；去掉 `KeepShortcuts` 补写 → 测试 3 FAIL。
- 版本递增；`docs/WORKLIST-INDEX.md` 加 R205；账本 `docs/history/ledgers/r205-execution-ledger.md`。
- 按 R199 规则发布到 GitHub Latest（release id、`isLatest=true`、匿名 `latest.json` 版本号）。
