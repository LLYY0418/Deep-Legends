# R206 执行账本

工单：`WORKLIST-R206-RIOT-KEY-RELAY-COLLECTION-FIRST-LOAD-AVATAR-QUEUE-ARENA-COLD-502-S-TIER-FONT-AND-UPGRADE-INSTALL-STAGES.md`。版本 **0.12.69**，验证与发布 key mode **public**。不读取个人 Key 文件；`desktop/verify-embedded-riot-key.cjs` 门禁保持不变。

## 实现与边界

| 项 | 已实施 | 验证 |
|---|---|---|
| P1 | Cloudflare Worker 固定 Riot GET 白名单、分类缓存、来源 IP 120/分钟和同路径 429 冷却；只从 Secret 读取 Key。软件 env → saved user → relay → none；中转无 Token，3 秒合并探测、失败冷却 5 分钟，固定不可用提示/重试和诊断 | Worker 7 项、Go 路由/优先级/探测/退避、Node 设置/错误展示；Token 和任意路径两项变异 |
| P2 | 首次正常收藏读取骨架；错误/重试/15 秒后失败按钮；collection_retry 仅刷新收藏；身份就绪空闲 10 秒后一次预读，游戏阶段禁用 | Go 空闲/阶段/收藏范围，Node 首次状态和请求行为 |
| P3 | 普通已连接 LCU 图片 404 立即返回 404/fallback header；强化图标保留专用回退。前端统一候选 URL 改到 communitydragon 远程队列；本地 4 秒、远程 10 秒、总并发 5/远程 2；30 秒匿名分类耗时汇总 | Go 404 无远程 I/O、专用强化旧回归；Node 5 远程 +20 本地首发 <100ms；同请求远程/10 秒本地两项变异 |
| P4 | OP.GG/YOUR.GG 首次网络超时或连接失败各重试一次，初次 3 秒、重试 5 秒预算（包括请求槽等待）；4xx 不重试。空闲一次 HEAD 预热；部分来源成功返回 200 并保留数据，全部失败 502。失败板块可单独重试 | Go 两端首次失败恢复/部分成功/全失败/4xx/预算/定向接口；Node 合并单板块且保留其余内容；去掉重试变异 |
| P5 | 只改 yourgg-s.svg font-size 11→13，单字评级一致 | Node 实际 SVG 单字字号 |
| P6 | 当前用户桌面/开始菜单 IconLocation 指向独立 app.ico；相同图标不重写，保留目标/参数/AppUserModelID/创建时间并通知 Explorer；诊断布尔字段。升级且卸载成功直接解压，手动/首次仍走原分支。Go 已等父进程时传 parent-exited。NSIS 阶段桥改写持久数据目录，保留原始 NSIS 和完整 8 阶段文件 | Go 文件无重写/Windows ShellLink 属性测试、Node 模板严格匹配/分支、Windows 真实旧版安装升级待流水线结果 |
| P7 | 所有 event 发起点记录匿名 uri_kind/during_refresh/suppressed；刷新后 5 秒及同类 30 秒去重；有预算的缺数据重试使用 pending_retry，避免被事件限流吞掉。翻译请求总预算 2.8 秒，失败复用成功缓存 | Go 10 秒 5 次事件仅 1 次刷新、逐事件日志；完整收藏刷新超时 <3 秒并保留缓存；旧有三次缺数据重试回归 |

中转常量列表目前为空：没有环境/用户 Key 时 source=none，不探测虚构地址。用户按 [Worker README](../relay/riot-worker/README.md) 自行部署、绑定自有域名后填入列表，需重新发布版本。development key 每 24 小时失效；公开服务需 production key，personal key 仅少量私人使用。

图片候选机制选择工单允许的前端下一候选方案：`img.onerror` 不能读取 HTTP header，第一次普通本地错误在统一队列内切换显式远程 URL；不会在后端本地请求中等待远程网络。远程失败继续现有卡片候选规则。客户端未连接时的公开图片与强化图标专用流程保留。

## 用户日志核验

来源 `lol-loot-diagnostics-1004-1451.jsonl`。只留匿名阶段/英雄与结果字段：[摘要](history/reports/r206/user-log-evidence.json)。0.12.65→0.12.68 总时长 41470ms、extract 7225ms、copy 18625ms，`uninstall_old_done` 缺失；旧日志不能确定丢失原因。阶段桥持久化及更新标志缓存是此次排查改动，Windows 实际升级仍需证明写出/导入，不能凭模板猜测把原因写成已查明。

R202/R204 的海斗选人、备战席应用及无序列不请求，在 10-04 日志提供范围内真机通过。R205 快捷方式未重建，已真机确认（创建时间未变、保留标记为 true）。桌面图标位置、此次独立图标是否避免白图标、用户电脑总升级 <25 秒仍待下一次用户实测。

## 自动验证

- `go test ./backend -count=1`：通过，248.577 秒；另新增完整收藏超时与竞技场预算定向通过（4.932 秒）。最终 public 构建再次覆盖新增测试。
- `node --test backend/web/*.test.cjs desktop/*.test.cjs`：1152 项，1151 通过、1 平台门禁跳过、0 失败（247.764 秒）。
- `(cd installer && go test ./... -count=1)`：全通过；嵌套模块不能用根模块的 `go test ./installer/...` 代替。
- `node --test relay/riot-worker/*.test.mjs`：7 项通过。
- 后端/安装器 vet、Windows 原生测试交叉编译通过；diff check 通过。
- 五项要求的变异均触发断言失败，实际源码未修改：[结果](history/reports/r206/mutations.json)，同目录保留失败原始日志。
- 首轮全量回归检出新事件标记时间导致 dirty 未清除、缺数据重试被去重，以及两个协程缺异常保护，均已修复；旧 404 回退/预算/诊断字段断言按 R206 更新。人工刷新测试明确使用 user_refresh，事件与重试分别验证。

## 构建、Windows 实际升级与发布

进行中。发布前已有 Release 基线：[releases-before.json](history/reports/r206/releases-before.json)。新标签发布前固定源码，旧标签/Release/附件保持不变。R199 的 release id、isLatest=true、匿名 latest.json 证据在正式发布后追加。
