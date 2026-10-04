# R204 执行账本

工单：[R204](WORKLIST-R204-RIOT-API-KEY-LOST-AFTER-ONLINE-UPDATE-USER-KEY-SETTING.md)。基线 `b684642b`（已发布 0.12.66 及 R203 收尾），本次 **0.12.67**。R203 已发布，本工单单独递增。实现完成；完整验证、构建与发布证据下方补齐，当前发布未完成。

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

针对性 Go、installer、Node 回归已通过。三项 Go overlay 变异（内置优先、401仍保存、Key写日志）都触发指定测试断言 FAIL，无编译失败充数。正式全量结果、key mode=public 构建与 R199 三项发布证据完成后追加。

仍需用户真机：Key 保存/下一次升级不丢失/绝活哥可用；下一次安装八阶段耗时与进度；无序列英雄不自动选；正常网络十人战绩≤5秒。源码实现与模拟检查不等于已达到真实网络5秒。

全量验证发现并修正了新增 self 并发包装缺少直接 panic 保护、旧 Node 夹具固定 URL 和 tooltip 焦点假设。修正后 Go 指定全量命令通过（236.570秒）；Node 1140项、1139通过、1平台门禁跳过、0失败。最终 Key 更换的旧 flight 防护增加后，再由完整构建测试分片、相关竞态及74项Node回归验证；最终源码的指定全量命令仍在运行，最终结果以发布证据为准。同局增量后台刷新保留已知战绩，不把卡片短暂清空。

三项变异结果见 [mutations.json](history/reports/r204/mutations.json)。安装器全量、Windows 后端交叉构建、go vet 与相关 race 通过。验证性打包始终 key mode=public，输出只保留 -public 文件名；个人 Key 文件未读取，public 密钥门禁脚本未修改。
