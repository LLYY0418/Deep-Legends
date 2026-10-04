# R205 执行账本

工单：[R205](WORKLIST-R205-UPDATE-DESKTOP-SHORTCUT-RECREATED-AND-ICON-POSITION-LOST.md)。基线 `f8fc39f8`，业务基线为已发布 0.12.67 的 `031715fd`；本次 **0.12.68**。实现及自动验证完成，0.12.68 已正式发布 Latest；目前不能证明用户桌面位置改变是快捷方式重建还是 Explorer 重绘。

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
- Windows 原生测试在临时目录创建实际 ShellLink，验证同目标保留字节、改目标保留参数/描述/AUMI、恢复创建/修改时间、目标缺失不创建；独立临时 HKCU 键验证 REG_SZ true 和二次不写；COM 重复初始化测试。macOS 交叉编译不等于 Windows 实际运行，正式 runner 已实际运行并通过，见下方发布证据。
- Node 检查升级命令仍带 `--updated` 且没有 `--no-desktop-shortcut`；未修改 NSIS customInstall。
- 两项 Go overlay 变异分别去掉 Restore 和 RepairKeep 调用，都触发指定测试断言 FAIL，非编译失败；完整原始证据见 [mutations.json](history/reports/r205/mutations.json)。
- 独立核查 Windows COM/vtable/指针生命周期/FILETIME/registry view 与两端日志 schema。安装流程核查提出 nil 疑点，经主线程核对已有 nil guard，补显式测试证明，不作为已确认缺陷。

## 构建与发布

- 首轮本机指定后端全量通过（250.175秒）；Node 共1144项、1143通过、1平台门禁跳过、0失败（252.021秒）；安装器全量、后端/安装器 vet、Windows 原生测试交叉编译、diff check 均通过。见 [Go日志](history/reports/r205/go-final.log)、[Node日志](history/reports/r205/node-final.log)、[安装器日志](history/reports/r205/installer-final.log)。
- 首轮本机 public 构建于2026-10-04T02:49:27Z完成，完整Go分片/vet/installer/public Key门禁/运行时/指纹/receipt/checksum通过，指纹 `b6b8ec5899e4`。Setup SHA256 `f488d5775bf99754c3efeb8277ce987945c88b3619a29e28a0c5405452573cf1`；见 [首轮构建凭证](history/reports/r205/local-first-build.json)。

0.12.68 已正式发布 Latest。key mode **public**；个人 Key 文件未读取，public 门禁脚本未修改。本机验证包只保留带 `-public` 后缀的名称。发布前已保存全部 12 个既有 Release 元数据，含禁止修改的 0.12.60 草稿；不移动旧标签、不变更旧 Release/附件。

## 用户真机验收

升级后检查 Deep Legends 及相邻桌面图标位置；导出日志核对 `update_shortcut_state` 的 before/after、created_time_changed、keep_shortcuts_reg、restore_results。没有新的用户实测日志，不能宣称图标位置已真机恢复；Windows runner 原生测试也不能代替用户 Explorer 桌面位置验收。

## Windows 候选修正

第一候选 `1e59a19c` 的正式工作流37172329108在原生测试中失败，尚未创建v0.12.68 Release；同期重复任务37172329100/37172327798取消。before字段齐全但目标匹配为false，after日志原先只打印指针，无法证明时间恢复失败。改读 GetPath 的 SLGP_RAWPATH 并以已有目标文件身份核对别名，避免只凭路径字面不同改目标；增加原生别名测试及断言字段值。此时不宣称已查明用户桌面问题或已通过Windows验证。

修正候选 `bb2b97a0b7bc2ce7c1d4155e4c99b64a44b66606`，源码指纹 `a24a41472d2b`。安装器全量、Windows交叉编译/vet及Node安装命令检查再次通过。本机public重建于2026-10-04T03:04:50Z通过所有Go分片/vet/installer/public Key/运行时/指纹/receipt/checksum，见 [最终本机构建凭证](history/reports/r205/local-release-build.json)。第一候选尚未有Release，仅本轮新建未发布标签以确切lease更新到修正候选，旧标签保留。

## R199 正式发布证据

[0.12.68 Release](https://github.com/LLYY0418/Deep-Legends/releases/tag/v0.12.68)：id **402804973**，发布时间 **2026-10-04T03:21:35Z**，`draft=false`、`prerelease=false`、`isLatest=true`。最终发布源码为 `bb2b97a0b7bc2ce7c1d4155e4c99b64a44b66606`，指纹 `a24a41472d2b`。

匿名 curl（禁用 curl 配置、无认证头、no-cache、随机参数）获取 Latest 清单，version **0.12.68**，与正式附件逐字节一致。见 [发布证明](history/reports/r205/publication-proof.json)、[Latest 查询](history/reports/r205/latest-after.json)、[匿名清单](history/reports/r205/anonymous-latest.json)、[发布元数据](history/reports/r205/published-release.json)。`gh release view` 不支持 isLatest，改用 `gh release list` 查询；没有重复发布操作。

| 正式附件 | 字节数 | SHA256 |
|---|---:|---|
| Deep-Legends-Setup-0.12.68-public.exe | 111497728 | `8fa7dd821cfb5318634bfca14c26f922553d5c3ab62ddf8f194910be6f4b7b76` |
| latest.json | 688 | `aa55ba232a9526ad4061b2eac2b6f8c81434cf442ea42c95f59daa1f6484d874` |
| SHA256SUMS-public.txt | 182 | `2e57c883d51fd12a20e5e220d3c791e2b7418884708cc59e7fbeaf98b78df7ee` |

主线程与独立复核确认三附件 size/digest、清单版本/指纹/Setup URL、两条 checksum 和 notes。草稿的 untagged URL 是发布前的临时地址，发布后正式 tag URL 已核实。见 [附件核查](history/reports/r205/formal-assets-verified.json)。正式 Windows 包与本机包摘要不同，源码指纹一致。

正式 Windows public 工作流 [37172934294](https://github.com/LLYY0418/Deep-Legends/actions/runs/37172934294) 为 success；实际 ShellLink 属性/时间戳恢复、别名、COM重复初始化和临时注册表测试均纳入 installer 全量（1.611秒）并通过，public Key 门禁、更新回归、receipt/checksum/附件检查通过。见 [正式日志](history/reports/r205/windows-success.log)、[工作流](history/reports/r205/release-workflow.json)。不把 Windows runner 结果替代用户 Explorer 图标位置验收。

发布前后全部12个既有 Release 的 id/tag/name/body/draft/prerelease/target/时间及附件 id/name/label/size/digest/state/content-type/时间/下载URL均保留（忽略Release updated_at和下载计数），包括0.12.60草稿。旧v0.12.67仍为 `031715fd8a41b1c96f0a684e0b9710455fb41601`，新v0.12.68固定为最终源码，不随账本收尾提交移动。见 [保留证明](history/reports/r205/old-releases-preserved.json)、[标签核对](history/reports/r205/tag-proof.txt)。

最终 [标签质量流水线37172934342](https://github.com/LLYY0418/Deep-Legends/actions/runs/37172934342) 和 [分支质量流水线37172932782](https://github.com/LLYY0418/Deep-Legends/actions/runs/37172932782) 均为完整 `conclusion=success`，包括全量race、Node、真实Chromium护栏、installer/vet/pool及Windows public复建/checksum。见 [标签质量结果](history/reports/r205/quality-workflow.json)、[分支质量结果](history/reports/r205/branch-quality-workflow.json)。本次所有工单代码、测试/变异、版本、重建和正式发布收尾已完成；R205仅保留用户桌面位置真机验收，索引不提前关闭。
