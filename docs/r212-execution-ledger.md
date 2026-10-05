# R212 执行账本

2026-10-04；基线 0.12.71。范围见 [R212 工单](WORKLIST-R212-CLOSE-VERIFIED-WORKLISTS-UPGRADE-INSTALL-GAP-AND-RELAY-FAILURE-LABELS.md)。用户最新指示：**执行工单，不构建、不发布**。本次仅源码、文档和测试；不生成应用/安装包，不改版本，不创建发布。R211 同工作区并行开发，保留其改动。

## P1：关闭与归档

R153、R156、R195、R196、R197、R198、R202、R203、R204、R205 状态为“已关闭：用户确认（R212）”；R206、R207、R208、R209、R210 为“已关闭：用户确认 + 日志核对（R212）”。15 张工单和 15 份账本归档，各账本追加收口事实，不改写过去的待验收记录；索引和相对链接同步。路径映射见 [archive-map.json](history/reports/r212/archive-map.json)，独立核验及 32 份文档链接检查均通过、缺失链接 0，见 [archive-verification.json](history/reports/r212/archive-verification.json)。

用户日志证据只保存匿名白名单字段、原始行号及整份文件 SHA256，见 [user-log-evidence.json](history/reports/r212/user-log-evidence.json)。不复制包含身份的原始日志。重点：R204 的安装 5 秒目标、R206 升级提速转入本工单；R208 每日用量由用户自行留意；R210 的 0.12.71 本局只证明前端清空，隐藏玩家补全未触发，早先 0.12.68 另一局的 reveal 不冒充这局证据。

## P2：安装间隙与细分计时

- 等待旧程序退出时并行释放内嵌安装包，退出后 join；释放错误沿用原提示，失败等待及后续返回均清理 TEMP。目的地校验、快捷方式保护、Key 迁移、压缩/解压和 `/NCRC` 保持原逻辑。
- 添加 `payload_released`、`nsis_start`、`oninit`、`check_done` 四个里程碑，沿用现有 JSON/NSIS 阶段文件；输出四个相邻间隔并保留 `parent_exited_to_uninstall_old_start` 总间隙。`payload_released` 记录父进程退出后 join 确认 payload 就绪的时刻，衡量退出后还要等多久，并非异步写入最早完成时刻；二秒等待测试另行直接比较实际完成时间。
- `.onInit` 首指令记录 `oninit`；启动 NSIS 前记录 `nsis_start`；应用检查结束记录 `check_done`。旧八阶段文件仍可正常读取，新阶段缺失为 null，不编造 0；部分新阶段缺失标记 partial，时钟倒退保留 clock_skew。
- PowerShell 探测仅在便携升级 FIND_PROCESS 和实际 `_CHECK_APP_RUNNING` 分支执行；`--updated --parent-exited` 安装跳过两者。`BUILD_UNINSTALLER` 的 `${isUpdated}` 分支同样跳过，手动卸载仍检查。模板中全局 PowerShell 变量声明加单次声明保护，避免两处分支展开造成重复声明；真实安装模板副本的补丁严格锚点与幂等测试通过。
- **旧卸载器执行的是旧版本代码**。卸载阶段优化只有从包含本改动的版本（例如 0.12.72）再升级到下一版才生效，不能在首次升级就宣称 `uninstall_old_ms` 已下降。

原真机 0.12.68 → 0.12.71：安装总计 22545 ms，父进程退出到旧卸载开始 6972 ms，旧卸载 6460 ms，解压 7213 ms。四个新细分尚无真实 Windows 数值；“间隙 <2 秒”目标未实测。工单要求的 Windows runner 静默升级需要生成新安装包，与用户“不构建”冲突，本次留待恢复构建后执行，不以模拟测试代替真机耗时。

## P3：正常 404 与请求摘要

正常 Riot JSON 404 记 `not_found`，不进入 `failures`；400 等其他 4xx 保持 http。新增 `http_statuses` 与 account/summoner/match/league/spectator/mastery/other 固定类别计数，摘要不记录路径、PUUID、名字或 origin。所有计数在同一锁下记录、刷新和重置。

只改分类和日志，保留 R208 原有重试、冷却、熔断优先级：HTML/非 JSON/1027 中转额度页即使状态为 404 也仍是 `quota_exhausted`，不是正常 Riot not_found。追加专门回归防止把原额度保护改掉；JSON 观战 404 与 400 的实际 provider 请求、分类重置及匿名性均有测试。

## 验证

- Node 安装器及 R186/R201/R204/R205/R206 定向检查：23/23 通过，见 [node-targeted.log](history/reports/r212/node-targeted.log)。测试走实际宏源的各编译/运行分支和已安装 electron-builder 模板副本，未运行 NSIS 编译器。
- 后端完整 `go test ./backend -count=1` 通过，266.692 秒，见 [go-full.log](history/reports/r212/go-full.log)。最终新增额度页回归另见 [go-targeted-final.log](history/reports/r212/go-targeted-final.log)。
- 安装器为独立 Go 模块，在 `installer/` 运行 `go test ./... -count=1` 全部通过（主包 4.148 秒），见 [installer-final.log](history/reports/r212/installer-final.log)。backend/installer 的 `go vet` 均通过，见 [go-vet.log](history/reports/r212/go-vet.log)、[installer-vet.log](history/reports/r212/installer-vet.log)。后端 R212/R208 race 通过（1.989 秒）、安装器 R212 race 通过（5.131 秒），见 [go-race.log](history/reports/r212/go-race.log)、[installer-race.log](history/reports/r212/installer-race.log)。
- 两项强制变异均触发行为断言 FAIL，非编译失败：释放放回等待之后、PowerShell 恢复无条件执行。最终加强后的二秒等待测试仍能杀死串行变异。见 [mutations.json](history/reports/r212/mutations.json)、[payload-after-wait-final.log](history/reports/r212/payload-after-wait-final.log)、[unconditional-powershell.log](history/reports/r212/unconditional-powershell.log)。正式源码未写入变异。
- 第一次 Node 全量：1186 项，1182 通过、3 失败、1 平台跳过，317.277 秒，见 [node-full.log](history/reports/r212/node-full.log)。三项失败均为并行 R211 新增 `renderOverviewStreak` 后旧 `desktop/overview-render.test.cjs` 提取夹具未加载新依赖；与 R212 安装器无关，不能写成全量已绿。
- 独立定位确认上述三项仅缺夹具依赖；在三个依赖对象补 `renderOverviewStreak` 空桩，保留列表节点、配额恢复、生涯栏的原断言，不修改生产函数。最终指定 Node 全量 **1186 项、1185 通过、1 平台跳过、0 失败**，252.596 秒，见 [node-final.log](history/reports/r212/node-final.log)。
- 最终 `git diff --check` 通过；版本仍为 0.12.71。验证汇总见 [verification.json](history/reports/r212/verification.json)。未构建、未发布；真实 NSIS 编译和 Windows 升级耗时不在本次验证结果之中。

## 待执行边界

恢复构建后：与 R211 合并版本，再按 R199 发布规则；采集 Windows 新细分计时并与 6972 ms 基线比较，目标 <2 秒；用户下一次在线升级复核四个细分，再下一次复核旧卸载器耗时。当前没有新版本产物或发布。
