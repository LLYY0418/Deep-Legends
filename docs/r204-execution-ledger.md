# R204 执行账本

工单：[R204](WORKLIST-R204-RIOT-API-KEY-LOST-AFTER-ONLINE-UPDATE-USER-KEY-SETTING.md)。基线 `b684642b`（已发布 0.12.66 及 R203 收尾），本次 **0.12.67**。R203 已发布，本工单单独递增。实现与自动验证完成，0.12.67 已正式发布 Latest；Key、安装阶段实测与5秒目标保留真机验收。

## Key 设置、持久化与诊断

- 隐私与能力页提供密码输入、眼睛、保存、清除、状态，无额外说明。API 只返回 status/source/result，保存后清空输入、恢复遮挡；清除即时回退到内置 Key 或未配置。
- `riot_key_settings.go` 统一 env → user → embedded。Windows 当前用户 DPAPI，不使用 LOCAL_MACHINE；文件 `riot-user-key.dat` 在既有数据目录，非 Windows 明文 0600。显式清除留下空文件，防止以后升级迁移重新导入用户清掉的 Key。
- 验证固定 GET KR status v4：200 保存；401/403 不覆盖已有用户 Key，显示无效；网络失败保存，显示已配置。无效输入仍保留原先可用凭据，状态表示最近保存验证失败；真正请求返回 401/403 才标记当前有效凭据无效。验证禁止 redirect 转发凭据，8 秒预算。
- 运行中 401/403 标记 active Key 无效；绝活哥缺失只显示“未配置 Riot Key”和去设置，无自动重试文案；失效为“Riot Key 无效”。去设置切到隐私页并聚焦输入框。其他 `errRiotKeyMissing` 返回同一明确缺失文案。
- 保存/清除后立即换凭据、清除 Riot 账号与绝活哥失败缓存，并发送更新事件；不要求重启。旧凭据的在途响应不能把新凭据标无效。
- 安装前的 finish 阶段执行嵌入 Key 迁移，先于便携数据迁移与 launch；已存在用户文件不覆盖，无内置 Key 跳过，持久化失败停止安装。不能追溯恢复已被旧 public 升级覆盖的 Key，现有用户需先在新设置保存一次。
- `app_start` 改为 riot_key_source=env/user/embedded/none；riot_key_saved=ok/invalid/network_error；riot_key_migrated=ok/skipped_exists/skipped_no_embedded，写失败只记 failed，不输出异常或秘密。诊断与导出不包含明文、片段、哈希或密文；隐私 stores 声明实际本机保存。
- Windows 加密依据：[Microsoft CryptProtectData](https://learn.microsoft.com/en-us/windows/win32/api/dpapi/nf-dpapi-cryptprotectdata)。public Key 校验脚本保持不变。

## P7–P10

| 条目 | 落实与验证边界 |
|---|---|
| P7 图标/提示/关闭 | 图标 24px、命中至少 36px，1.6/1.8 细线及红点保留；全局 tooltip 只响应 hover/:focus-visible；皮肤、外观、生涯、更新关闭按钮共享 dialog-close-button 尺寸。R203 阻塞审计补一行；Node 测恢复焦点隐藏、鼠标及键盘可见 |
| P8 不完整计时 | 旧路径缺 parent_exited 是源码可确认的一种可能，原日志无法证明唯一原因。安装器八个字段未到达写 null；消费缺字段记 partial/missing_fields，保留其他相邻区间及 total；不可解析 parse_error、倒序/未来时间 clock_skew，读取一次删除 |
| P8 卸载与阶段进度 | stock NSIS 的两种卸载作用域外围写真实 uninstall_old_start/done，TEMP 与 Go 导入闭环；只有 isUpdated 执行 hook，不改手动安装。升级等待0–5、移除5–25、解压25–60、写入60–95、收尾95–100，实字节/无字节时渐进；单调、不超阶段范围，成功才100。每250ms读新阶段，仅新时间点写计时文件 |
| P9 临时逻辑删除 | 删除开关、helper、决策字段、静默分支、test-first-card 诊断；无序列英雄不发请求、no-pool-champion。备战席 FINALIZATION、持有规则与手动接管保留；确认选人后换英雄不再记 not-applied-after-2s。Go 回归及源码断言 |
| P10 单页/减请求 | queue 3140/3100/3110、既有人机/末日人机队列只读一页；服务端拒绝筛选时允许同一页的筛选探测与回退，不误称只有一次 HTTP。非本人 SGP 当前队列近30天已有可用10局才跳过 LCU；本人仍并行读 LCU 最新结果。失败/不足/未筛选仍回退LCU |
| P10 逐玩家 | 实际 roster 起始发布 pending 卡片，完成一人就发匿名引用快照；共享/prewarm flight 新等待者重放已有进度。每个 HTTP 请求唯一 requestId，cache generation/context 与前端 requestId/gameId/phase 三重过滤旧结果；最终响应保留全部原后处理。已有局部卡片渲染复用，不新增重轮询 |
| P10 计时 | live_load_cost 增 sgp_ms/lcu_ms（本批玩家各路最大实际耗时）、slowest_player_ms（不含等待信号量的单玩家身份+rank+history耗时）；原 matches_ms 保留整批关键路径。Go 真实模拟 roster 检查逐人发布、匿名标识和字段 |

## 安装提速决策与真机证据

本机 Downloads 已没有工单引用的 2236 原始日志；50 秒（之前73秒）、14:21:47.7旧版退出→14:22:37.9新版启动、原 timing invalid 等数字明确引用工单摘录。没有各阶段实测值，不能确定卸载或解压占大头，不能编造前后对比。

当前保留 NSIS 的既有卸载/压缩策略。下一次新计时若 uninstall_old_ms 占大头，评估在线升级直接覆盖并清理旧清单中已删除文件（减卸载耗时，但需可靠旧文件清单/失败恢复）；若 extract_ms 占大头，评估非固实或更快压缩（安装包变大，以下载+安装总耗时比较）。先取实际阶段数据再选择，手动安装流程不改。P8 数据决策保留待真机证据，自动进度测试不作为提速实测。

R198 0/1/2 映射、R200卡片锁定和备战席换入、R201只读镜头/临时测试、R202 FINALIZATION及手动接管按工单追加真机证据更新；R200/R201原需求已验证或交由后续工单接手，归档关闭。R198连续两局快照、R202开关两种持有规则仍缺完整证据，保留进行中。不会用 Windows runner 构建代替用户客户端验收。

## 验证与发布

- 最终源码 `031715fd8a41b1c96f0a684e0b9710455fb41601`，指纹 `834c40067e93`。Key 旧 flight 防护及逐人进度范围隔离均已纳入；只有有效且相等的 gameId、相同 queueId/playerRef/hidden/privateHistory/identityUnresolved 才复用 pending 行历史。六种范围变化回归通过。
- Node 指定全量最终为1143项、1142通过、1平台门禁跳过、0失败（280.324秒），见 [node-final.log](history/reports/r204/node-final.log)。Go指定完整命令在最终源码上再次通过（239.815秒），见 [go-final.log](history/reports/r204/go-final.log)；最终构建的全部Go分片也通过。最终go vet、安装器全量、Windows交叉构建及相关race通过（race 2.382秒）。
- 三项Go overlay变异（内置优先、401仍保存、Key写日志）都触发指定断言FAIL，无编译失败充数，见 [mutations.json](history/reports/r204/mutations.json)。
- 最终本机 public 重建于2026-10-04T00:56:09Z完成，完整测试分片、vet、installer、packaged runtime/fingerprint、public Key门禁、receipt/checksum全部通过。只保留 `Deep Legends Setup 0.12.67-public.exe`；本机Setup SHA256 `017838084ee14460b367bf0122580c95cda62d1fad9e6e09d25b0b5a2250bb15`，见 [local-release-build.json](history/reports/r204/local-release-build.json)。本机验证包与Windows正式包摘要可因构建平台/产物不同而不同，二者源码指纹相同。
- 正式Windows public工作流 [37166290977](https://github.com/LLYY0418/Deep-Legends/actions/runs/37166290977) 对最终提交 `conclusion=success`；build/public Key gate/更新回归/附件检查均通过。[质量流水线](https://github.com/LLYY0418/Deep-Legends/actions/runs/37166290994) 的quality job已通过完整race、Node、真实Chromium图片队列与懒CSS、installer/pool门禁；完整Windows复建于2026-10-04T01:19:46Z通过，整个流水线 `conclusion=success`；同源码分支流水线37166289376也于01:18:29Z全部通过。见 [quality-workflow.json](history/reports/r204/quality-workflow.json) 与 [branch-quality-workflow.json](history/reports/r204/branch-quality-workflow.json)。个人Key文件未读取，public密钥门禁脚本未修改。

### R199正式发布证据

[0.12.67 Release](https://github.com/LLYY0418/Deep-Legends/releases/tag/v0.12.67)：id **402764741**；发布时间 **2026-10-04T01:10:25Z**；`draft=false`、`prerelease=false`、`isLatest=true`。见 [publication-proof.json](history/reports/r204/publication-proof.json)、[Latest查询](history/reports/r204/latest-after.json)、[Release元数据](history/reports/r204/published-release.json)。匿名无认证、no-cache下载 `releases/latest/download/latest.json` 得到version **0.12.67**、fingerprint **834c40067e93**，与正式附件逐字节一致，见 [匿名清单](history/reports/r204/anonymous-latest.json)。

| 正式附件 | 字节数 | SHA256 |
|---|---:|---|
| Deep-Legends-Setup-0.12.67-public.exe | 111446528 | `6171661b827937f5ee403e3f5d77d8b1171b036338d8f7378939ae6e0bf14d97` |
| latest.json | 1259 | `0bab69a9c53535453224490c4b514e22041b1b20d1a5783628eb6519c9f85a9e` |
| SHA256SUMS-public.txt | 182 | `31a9b13fab166d72862eeb8870d106747d40aa22b02b57e25cd28c0bc6d8e71e` |

主线程与独立复核均验证三个附件的size/digest、清单版本/指纹/Setup URL及两条checksum；见 [正式附件核查](history/reports/r204/formal-assets-verified.json)。草稿的untagged下载URL发布后已转换为v0.12.67，清单使用确切正式tag URL。

发布前后比较全部11个既有Release（含0.12.60草稿）的id/tag/name/body/draft/prerelease/target/时间与附件id/name/label/size/digest/state/content-type/时间/下载URL全部一致；忽略下载计数和Release updated_at。见 [保留证明](history/reports/r204/old-releases-preserved.json)。旧v0.12.66标签仍为 `d56b126751a0ce350b442d0d3c688840957ab493`，新v0.12.67为最终提交；见 [标签核对](history/reports/r204/tag-proof.txt)。

### 修正记录与验收边界

全量检查先发现新增self并发包装缺直接panic保护，以及旧Node夹具URL/tooltip焦点假设；修正后通过。随后R204测试源码提取器受注释单引号影响，修正后通过；最后独立复核补上gameId=0不可证明同局的隔离。前两候选六个任务37165481830/37165481843/37165481852、37165894014/37165894047/37165892237取消时均无v0.12.67 Release；仅更新本轮新建未发布标签，既有标签和Release未改。正式发布后标签固定为最终提交。

索引与账本同步：R198映射真机已核实，连续两局仍待验收；R200/R201已关闭归档，未完成安装部分由R204接手；R202所提供范围验证通过，其余开关规则仍待验收。R204保持进行中，不能以自动验证或Windows构建代替用户客户端验收。

仍需用户真机：首次在设置保存Key、绝活哥可用且下一次升级Key不丢失；下一次安装八阶段耗时与进度；无序列英雄不自动选；正常网络十人战绩≤5秒。P8提速方案待实测阶段数据决定，未编造前后对比或宣称真实网络5秒目标已达成。

## R206 补充的 10-04 用户日志核验

P9 海斗卡片选人通过，无序列命中时不提交请求；其它 Key/逐人耗时验收不据此关闭。证据来自 `lol-loot-diagnostics-1004-1451.jsonl`，摘要见 [R206 日志证据](history/reports/r206/user-log-evidence.json)。
