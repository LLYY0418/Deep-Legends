# R206 执行账本

工单：`WORKLIST-R206-RIOT-KEY-RELAY-COLLECTION-FIRST-LOAD-AVATAR-QUEUE-ARENA-COLD-502-S-TIER-FONT-AND-UPGRADE-INSTALL-STAGES.md`。版本 **0.12.70**，验证与发布 key mode **public**。不读取个人 Key 文件；`desktop/verify-embedded-riot-key.cjs` 门禁保持不变。

## 实现与边界

| 项 | 已实施 | 验证 |
|---|---|---|
| P1 | Cloudflare Worker 固定 Riot GET 白名单、分类缓存、来源 IP 120/分钟和同路径 429 冷却；只从 Secret 读取 Key。软件 env → saved user → relay → none；中转无 Token，3 秒合并探测、失败冷却 5 分钟，固定不可用提示/重试和诊断 | Worker 8 项、Go 路由/优先级/探测/退避、Node 设置/错误展示；Token 和任意路径两项变异 |
| P2 | 首次正常收藏读取骨架；错误/重试/15 秒后失败按钮；collection_retry 仅刷新收藏；身份就绪空闲 10 秒后一次预读，游戏阶段禁用 | Go 空闲/阶段/收藏范围，Node 首次状态和请求行为 |
| P3 | 普通已连接 LCU 图片 404 立即返回 404/fallback header；强化图标保留专用回退。前端统一候选 URL 改到 communitydragon 远程队列；本地 4 秒、远程 10 秒、总并发 5/远程 2；30 秒匿名分类耗时汇总 | Go 404 无远程 I/O、专用强化旧回归；Node 5 远程 +20 本地首发 <100ms；同请求远程/10 秒本地两项变异 |
| P4 | OP.GG/YOUR.GG 首次网络超时或连接失败各重试一次，初次 3 秒、重试 5 秒预算（包括请求槽等待）；4xx 不重试。空闲一次 HEAD 预热；部分来源成功返回 200 并保留数据，全部失败 502。失败板块可单独重试 | Go 两端首次失败恢复/部分成功/全失败/4xx/预算/定向接口；Node 合并单板块且保留其余内容；去掉重试变异 |
| P5 | 只改 yourgg-s.svg font-size 11→13，单字评级一致 | Node 实际 SVG 单字字号 |
| P6 | 当前用户桌面/开始菜单 IconLocation 指向独立 app.ico；相同图标不重写，保留目标/参数/AppUserModelID/创建时间并通知 Explorer；诊断布尔字段。升级且卸载成功直接解压，手动/首次仍走原分支。Go 已等父进程时传 parent-exited。NSIS 阶段桥改写持久数据目录，保留原始 NSIS 和完整 8 阶段文件 | Go 文件无重写/Windows ShellLink 属性测试、Node 模板严格匹配/分支、Windows 真实旧版安装升级待流水线结果 |
| P7 | 所有 event 发起点记录匿名 uri_kind/during_refresh/suppressed；刷新后 5 秒及同类 30 秒去重；有预算的缺数据重试使用 pending_retry，避免被事件限流吞掉。翻译请求总预算 2.8 秒，失败复用成功缓存 | Go 10 秒 5 次事件仅 1 次刷新、逐事件日志；完整收藏刷新超时 <3 秒并保留缓存；旧有三次缺数据重试回归 |

0.12.69 草稿的中转列表为空，保持不探测虚构地址。用户完成本机 OAuth 授权并提供 `yinxiaobia.net` 后，已部署到 `https://riot.yinxiaobia.net`，0.12.70 内置此公开地址。用户在 Cloudflare 自行设置 Secret，并确认是 production Key；不读取凭据值。部署验证见 [relay-deployment.json](history/reports/r206/relay-deployment.json)。development key 每 24 小时失效；公开服务需 production key，personal key 仅少量私人使用。

图片候选机制选择工单允许的前端下一候选方案：`img.onerror` 不能读取 HTTP header，第一次普通本地错误在统一队列内切换显式远程 URL；不会在后端本地请求中等待远程网络。远程失败继续现有卡片候选规则。客户端未连接时的公开图片与强化图标专用流程保留。

## 用户日志核验

来源 `lol-loot-diagnostics-1004-1451.jsonl`。只留匿名阶段/英雄与结果字段：[摘要](history/reports/r206/user-log-evidence.json)。0.12.65→0.12.68 总时长 41470ms、extract 7225ms、copy 18625ms，`uninstall_old_done` 缺失；旧日志本身不能确定丢失原因。后续流水线逐阶段快照与 NSIS 原文已证实文件头覆盖（见下方修复记录）；修复后的 Windows 实际升级仍待恢复验证。

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

按用户 2026-10-04 最新指示，暂停构建和发布，取消尚在运行的 0.12.70 Windows 流水线。代码、非构建验证及部署已完成；修复后的实际 Windows 升级验证和正式发布留待恢复。发布前已有 Release 基线：[releases-before.json](history/reports/r206/releases-before.json)。新标签发布前固定源码，旧标签/Release/附件保持不变。R199 的 release id、isLatest=true、匿名 latest.json 证据在正式发布后追加。

## 候选构建进展

候选 c70e6b0e 的本机 public 完整重建通过，源指纹 **030c59db9431**。Go 1793 项分片、vet、installer、公钥门禁、运行时、指纹、receipt/checksum 全通过。Setup SHA256 `2ccf212a374ec7ce7aac67cf29bc2b59eb345f0593b036bb6d701fc3f8b08f58`；见 [构建凭证](history/reports/r206/local-build.json) 与 [完整构建日志](history/reports/r206/local-build.log)。只保留 `-public` 安装包。

首次 NSIS 构建发现 `_CHECK_APP_RUNNING` 在两个条件分支中展开两次，导致 `doStopProcess` 标签重复；改为一次展开，增加实际宏展开次数护栏后重建通过。独立复核补充断线清空收藏起点/去重状态，并加入回归。

Windows 流水线候选 384c2c2c 增加真实 0.12.65→0.12.68 阶段文件观测，再升级到 0.12.69。历史日志只能确认 start 已导入而 done 缺失（不能误读为空的 done-to-start 为 start 缺失）；该次运行已失败，快照随后证实阶段写入发生文件头覆盖（见下方修复记录）。

0.12.69 候选源码 e0d263a52fe707cce2823f8fa1540de2a71a2f33 将新增独立图标也纳入指纹，public 重建通过。候选指纹 **33ca002c7114**，详见 [最终构建凭证](history/reports/r206/local-release-build.json)。首次候选凭证保留，不替代最终附件校验。v0.12.69 固定到此源码，正式 Windows 草稿构建已完成，但实际升级验证失败，保留草稿而未发布。

Windows API 核对使用微软原文：[SetIconLocation](https://learn.microsoft.com/en-us/windows/win32/api/shobjidl_core/nf-shobjidl_core-ishelllinkw-seticonlocation)、[SHChangeNotify](https://learn.microsoft.com/en-us/windows/win32/api/shlobj_core/nf-shlobj_core-shchangenotify)。只更新图标位置并通知精确快捷方式路径。

最终指定全量回归通过：`go test ./backend -count=1` 251.091 秒，`node --test backend/web/*.test.cjs desktop/*.test.cjs` 1153 项/1152 通过/1 平台门禁跳过/0 失败（257.398 秒）；最新源码后端 vet 与 diff check 通过。指纹资源新增项另有实际变更图标字节必使指纹变化的回归。

独立遗漏审计未发现其它实现缺失。Worker 段位白名单限定实际调用的 `/lol/league/v4/entries/by-puuid/{puuid}`（riot_api.go 使用点）；不开放泛化的未知 entries 子路径，符合“与实际用到一致”的限定。

## 0.12.70 中转实际部署与阶段写入修复

Cloudflare Worker 版本 `815f2bd4-eae1-42a0-9095-24c7df49bb93`，仅绑定自有子域名，关闭 workers.dev 和预览 URL。首次缺 Secret 的 503 符合预期；添加 Secret 后 502，在本地 workerd + 假 Key 中复现：`redirect:error` 被运行时拒绝。改为 manual 并显式拒绝全部 3xx，禁止凭据跟随重定向；8 项 Worker 测试通过，实际状态接口返回 200。真实账号、段位、熟练度、比赛详情/时间线及拒绝路径的结果见部署验证，证据不含账号 ID、PUUID、Key 或请求 URL。

Windows 候选 384c2c2c 实际升级失败，缺 uninstall_old_done/extract_done，未发布。实际旧版 0.12.68 的逐次快照显示 done→extract_start→extract_done→copy_done 每次从文件头替换，尾部留下短行；0.12.69 原始阶段文件仅剩 copy_done 和残留字符。结合 [NSIS FileOpen 原文](https://nsis.sourceforge.io/Reference/FileOpen)（所有模式均从文件头开始）确认缺少 FileSeek 为覆盖原因。修复为打开成功后 FileSeek 0 END，再写阶段，保留原 error flag/寄存器。旧卸载的 TEMP 假说不作为已查明原因。完整原始证据见 [失败运行快照](history/reports/r206/failed-upgrade-0.12.69/legacy-0.12.68-nsis-snapshots.json)。

保留 v0.12.69 未发布草稿和固定标签，0.12.70 本机重建在用户暂停指示前完成；修复后的实际 Windows 升级验证及发布现暂停，尚未发布。

0.12.70 本机 public 完整重建通过：Go 1793 项分片、vet、installer、运行时与 public Key 门禁、版本/指纹、receipt/checksum 全通过。指纹 **ccd986308896**，Setup SHA256 `5d5e727540da313c9a1c1370602c8e3e98eeaea6b64217b0e0e68b3291d1738b`，见 [本机构建凭证](history/reports/r206/local-build-0.12.70.json)。新增中转后，无凭据测试明确隔离地址配置，避免测试无凭据场景实际启用中转。Go 同类 HTTP 客户端以三秒预算访问中转：HTTP 200、合法 JSON、817ms，见 [Go 实际中转检查](history/reports/r206/go-relay-live.log)。追加两项重定向变异均触发断言失败，见 [变异结果](history/reports/r206/relay-runtime-mutations.json)。

正式 Windows 草稿构建：[37191701705](https://github.com/LLYY0418/Deep-Legends/actions/runs/37191701705)；完整质量与真实 Windows 升级：[37191701730](https://github.com/LLYY0418/Deep-Legends/actions/runs/37191701730)。两者均按用户指示取消，不自动重试或新建构建任务；不以本机构建替代实际升级。

## 本轮收尾（暂停构建/发布）

用户要求“先不要构建和发布，先把其他的做完”。已取消 `37191701705`、`37191701730`，重复分支运行 `37191697679` 此前也已取消。没有发布 0.12.69 或 0.12.70；0.12.68 仍是 Latest。R206 P1–P7 实现、部署与非构建验证已完成；保留修复后的 Windows 升级运行及用户界面/安装速度/桌面图标真机验收待办。不会自动恢复构建或发布。


## 0.12.71 合并正式发布

2026-10-04 用户要求「发布新版本」，恢复构建/打包/发布。本工单随 **0.12.71 public Latest** 正式发布，release id **403010042**，`draft=false`、`prerelease=false`、`isLatest=true`，匿名 Latest 清单为 **0.12.71**。源码 `ea6d64c99a3e4ab86f9d9055a7a3fc4b300de4e6`，指纹 `696da05d0ad0`。正式 Windows 构建与完整质量/真实升级流水线全部通过，公开附件逐字节校验通过，旧 Release/草稿/标签保留。完整发布与附件证据见 [合并发布账本](history/ledgers/release-0.12.71-execution-ledger.md)。原有用户真机验收待办保持，不以 runner 结果提前关闭工单。
