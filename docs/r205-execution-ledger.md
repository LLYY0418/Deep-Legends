# R205 执行账本

工单：[R205](WORKLIST-R205-UPDATE-DESKTOP-SHORTCUT-RECREATED-AND-ICON-POSITION-LOST.md)。基线 `f8fc39f8`，业务基线为已发布 0.12.67 的 `031715fd`；本次 **0.12.68**。实现完成，自动验证及发布进行中；目前不能证明用户桌面位置改变是快捷方式重建还是 Explorer 重绘。

## 实现与边界

- Go 升级壳在 NSIS 前读取当前/公共桌面及当前/公共开始菜单 `Deep Legends.lnk` 的存在性、创建/修改时间、目标匹配布尔值；结束后再读取。Windows KnownFolder API 定位目录，不把目录或账号写入报告。时间保留 100ns 精度；缺失/读取失败记固定状态，未知时间不推断为重建。
- 升级前把已有桌面快捷方式复制到本轮私有 TEMP；仅当结束后明确缺失或创建时间改变，且新安装 exe 存在，才覆盖回原名原目录。读取原链接的目标，若目标改变，只通过 IShellLinkW.SetPath 修改备份目标后再覆盖，保留 AppUserModelID、参数、图标等属性；恢复原创建/修改时间。未变化不动；备份失败、目标缺失、读取未知、恢复失败各有固定结果。
- `desktop_after` 是恢复前的原始证据，`desktop_final` 是恢复后状态。`created_time_changed` 合并桌面/开始菜单变化，另有两种分项布尔值；不因恢复成功抹去重建证据。开始菜单仅诊断，不扩展恢复范围。
- KeepShortcuts 位于 electron-builder 产品键 `Software\ae355ba0-3686-5750-a759-b20159e02f53`（appId UUID.v5，Node 锁定），不是 Uninstall 键。按 NSIS x64 registry view 读取 HKCU/HKLM 已有产品键；有桌面链接且值不为 true 才补 REG_SZ `true`，已有 true 先只读返回，无重复写入。便携升级跳过注册表补写。
- 报告单独保存为既有数据目录下的 `update-shortcut-state.json`，原子写入，新版后端启动消费一次并删除，输出 `update_shortcut_state`。严格 schema、scope/状态白名单、时间校验、16KiB 上限、拒绝符号链接；未知字段/损坏报告只输出固定 invalid/parse_error。没有路径、账号、链接原文或异常原文，R204 timing 文件保持独立。
- Update=false 没有 guard；`finish` 接收 nil 时直接返回。NSIS 启动失败/退出失败也先记录 before/after，再走原失败恢复；正常结束先恢复再清理 TEMP/启动新版。手动安装、NSIS 卸载及升级窗口文案不改。

原生接口依据：[Shell Links](https://learn.microsoft.com/en-us/windows/win32/shell/links)、[SetPath](https://learn.microsoft.com/en-us/windows/win32/api/shobjidl_core/nf-shobjidl_core-ishelllinkw-setpath)、[SetFileTime](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-setfiletime)、[AppUserModelID](https://learn.microsoft.com/en-us/windows/win32/properties/props-system-appusermodel-id)。

## 自动验证

- Go 测试覆盖存在/缺失/创建时间变化；删除与重建恢复原文件、未变化不写、目标 exe 缺失不恢复，以及备份/恢复失败、读取未知、开始菜单分项证据、便携/权限边界、nil guard。
- Windows 原生测试在临时目录创建实际 ShellLink，验证同目标保留字节、改目标保留参数/描述/AUMI、恢复创建/修改时间、目标缺失不创建；独立临时 HKCU 键验证 REG_SZ true 和二次不写；COM 重复初始化测试。macOS 交叉编译不等于 Windows 实际运行，正式 runner 结果待补。
- Node 检查升级命令仍带 `--updated` 且没有 `--no-desktop-shortcut`；未修改 NSIS customInstall。
- 两项 Go overlay 变异分别去掉 Restore 和 RepairKeep 调用，都触发指定测试断言 FAIL，非编译失败；完整原始证据见 [mutations.json](history/reports/r205/mutations.json)。
- 独立核查 Windows COM/vtable/指针生命周期/FILETIME/registry view 与两端日志 schema。安装流程核查提出 nil 疑点，经主线程核对已有 nil guard，补显式测试证明，不作为已确认缺陷。

## 构建与发布

- 最终本机指定后端全量通过（250.175秒）；Node 共1144项、1143通过、1平台门禁跳过、0失败（252.021秒）；安装器全量、后端/安装器 vet、Windows 原生测试交叉编译、diff check 均通过。见 [Go日志](history/reports/r205/go-final.log)、[Node日志](history/reports/r205/node-final.log)、[安装器日志](history/reports/r205/installer-final.log)。
- 本机 public 构建于2026-10-04T02:49:27Z完成，完整Go分片/vet/installer/public Key门禁/运行时/指纹/receipt/checksum通过，指纹 `b6b8ec5899e4`。Setup SHA256 `f488d5775bf99754c3efeb8277ce987945c88b3619a29e28a0c5405452573cf1`；见 [构建凭证](history/reports/r205/local-release-build.json)。

发布未完成。key mode **public**；个人 Key 文件未读取，public 门禁脚本未修改。本机验证包只保留带 `-public` 后缀的名称。发布前已保存全部 12 个既有 Release 元数据，含禁止修改的 0.12.60 草稿；不移动旧标签、不变更旧 Release/附件。

## 用户真机验收

升级后检查 Deep Legends 及相邻桌面图标位置；导出日志核对 `update_shortcut_state` 的 before/after、created_time_changed、keep_shortcuts_reg、restore_results。没有新的用户实测日志，不能宣称图标位置已真机恢复；Windows runner 原生测试也不能代替用户 Explorer 桌面位置验收。

## Windows 候选修正

第一候选 `1e59a19c` 的正式工作流37172329108在原生测试中失败，尚未创建v0.12.68 Release；同期重复任务37172329100/37172327798取消。before字段齐全但目标匹配为false，after日志原先只打印指针，无法证明时间恢复失败。改读 GetPath 的 SLGP_RAWPATH 并以已有目标文件身份核对别名，避免只凭路径字面不同改目标；增加原生别名测试及断言字段值。此时不宣称已查明用户桌面问题或已通过Windows验证。
