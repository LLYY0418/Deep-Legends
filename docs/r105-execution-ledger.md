# R105 执行账本

目标版本：0.12.2。P0–P3 实现、自动验证、13项变异及Windows安装包已完成；Windows 游戏客户端真机复验未完成。

依据：R105 工单、`pro-accounts-verification-2026-09-17.md`。保留既有 R95–R104 工作区修改。

工单两处计数笔误以定稿清单为准：IG 为 15 个账号、DK 为 11 个账号，合计 53。工单逐项枚举 8 个变异但总数写 9；本轮补充“删除正式旗帜 PATCH”作为第九个，另补旧快照、隐藏旧账号、opaque 数字精度、头像回读门禁，共13项。

| 要求 | 实现/验证 | 状态 |
|---|---|---|
| P0 6 队33人53账号，逐人数量、精确大小写/空格/顺序 | `TestR105_EmbeddedAccountCounts`、`PlayerAccountCounts`、`SpecialCharacters`，完整逐人顺序比对定稿文档 | PASS |
| P0 Wei/Rookie/Xun/knight/TheShy/Smash | `CriticalAccountFixes`、`AllPublicationsAndOfflineKeepReviewedAccounts`，较低段位的新活动排在高段位旧活动之前 | PASS |
| P0 页面和 pro_players 诊断 | `ReviewedPageSurvivesOldSnapshotAndWrongOwner` 从旧磁盘快照调用真实 handler 并核对诊断；jsdom 实际脚本默认渲染全部53个账号 | PASS（离线） |
| P1 头像全部可选、无拥有筛选、点击 PUT201、回读与错误提示 | jsdom真实picker/apply函数 → app POST合同；Go真实handler → LCU PUT，含未知库存、区域disabled元数据、201无变化及401等失败 | PASS（离线） |
| P2 旗帜全部可选、缩略图、点击 PATCH200 | 正式 banner action；复制完整slot，仅换3字段；嵌套data/未来字段/数值类型和精度保留；200无变化拒绝 | PASS（离线） |
| P3 卡片文案、探测记录、来源文档、内置标注 | 头像旗帜移除拥有态限制；W5/W6按工单证据追加；来源文档及CLAUDE同步；R99/R101探测实现保留 | PASS |
| 版本 0.12.2 | package.json / package-lock.json；完整重编译后再打包；源码/后端/封装内后端指纹一致 | PASS |
| build/vet/full Go/race/前端/变异 | docs/r105-validation；所有baseline通过，13/13变异捕获 | PASS |
| Windows 游戏客户端复验 | 当前环境 macOS；工单提供的 W5/W6 为既有用户证据，不能替代本版本真机复验 | 待真机 |

产品语义：清单决定归属及账号显示名称，动态来源不新增/删除清单账号；保留 R104 最近活跃优先，未知/相同活跃时间保留清单顺序，不使用 LP 决定主号。OP.GG revision_at 是来源维护的最近活跃时间，不宣称为 Riot 精确开局时间。R104 后台配额和涓流限制保持。

## 遗漏原因与本轮修复

R102 的53账号表确实已在源码中，但旧快照直接返回、动态来源覆盖/身份冲突整组排除以及前端 dormant 折叠，均可能让最终页面看似仍使用旧清单。R105 在最终响应边界固定人工归属与账号集合，避免只修改表却没有修通产品链路。默认旧账号完整显示；用户主动“只看最新账号”筛选保留。

旗帜先前仅有目录与探测，没有正式写入 action。本轮接通 `/api/facade/apply` 的 banner action，GET account loadouts → PATCH同一loadout → GET回读；仅发送 banner slot，不重写 ward 等其他槽位。`json.RawMessage` 保留 opaque 字段，包含大于2^53的整数和高精度小数。头像与旗帜成功状态必须分别为201/200；回读未改变也不会关闭弹窗并声称成功。

旗帜缩略图字段取自公开 CommunityDragon regalia 的实际 `assetPath`（2026-09-17读取，示例 `/lol-game-data/assets/ASSETS/Regalia/BannerSkins/lny2023.png`），通过既有图片代理加载；缺图显示占位。现有目录中的无效ID、无图片头像及非独立旗帜段位变体过滤保留，所有可展示目录项目均不再受拥有态或区域disabled元数据限制。

独立复核建议将 `proRosterVerifiedAt` 改成账号核对日期，本轮未采纳：2026-09-08是队伍名单审核日期，2026-09-17是账号审核日期；不得凭账号核对伪造新的队伍注册核验。该区别已在来源文档明确。已采纳头像201回读无变化的后端校验与回归测试。

## 自动验证证据

- `go-full.txt`：最终生产代码 `go test -count=1 -v .` PASS，128.439s；既有可选真机/外网capture测试仍按条件跳过。
- `go-race.txt`：`go test -race -run '^TestR105' -count=1 .` PASS，2.415s。
- `go-build.txt`、`go-build-windows.txt`：原生构建与Windows amd64交叉构建；临时输出位于 `/private/tmp`，避免用macOS二进制覆盖仓库里的Windows产物。
- `go-vet.txt`：`go vet ./...`。正式打包脚本还检查 installer 模块的 test/vet。
- `node-full.txt`：`node --test web/*.test.cjs desktop/*.test.cjs`，731项，730 PASS、1 Windows平台条件SKIP、0失败。
- `node-targeted.txt`：头像/旗帜/职业页及原有facade交互，51项全部通过。
- `pro-players-response.json`：真实handler从错误旧快照产生的公开响应；Go测试同时核对实际 `pro_players` JSONL事件。不是Windows实测日志。
- `mutations/matrix.json`：逐项baseline和变异日志。变异使用临时Go overlay或JS源码覆盖，不改工作区源码。

最初完整回归中的两项旧断言要求“来源失败时没有账号”和“目录决定页面账号数”，已按R105契约更新并重跑通过。一次 vet/变异被沙箱拒绝访问系统Go缓存，改为工作区 `.gocache` 后复跑；未将环境失败计为变异命中。

## Windows 真机复验（全部待执行）

当前执行环境为macOS，无Windows国服游戏客户端。工单引用的20:38日志只作为解锁依据，不冒充0.12.2复验。

- [ ] Wei 显示 dyjkbysb#KR1。
- [ ] Rookie 显示 벼락식혜#0070、EmberKnight#KR0。
- [ ] Xun 显示 我累铜泥丸#小重o。
- [ ] 全部33人账号数与定稿逐人一致。
- [ ] 头像卡打开全屏选择器。
- [ ] 所有目录头像可点击、无拥有筛选。
- [ ] 点击头像成功后关闭弹窗。
- [ ] 游戏客户端实际头像已更新。
- [ ] 旗帜卡打开全屏选择器。
- [ ] 所有目录旗帜可点击、无拥有筛选。
- [ ] 点击旗帜成功后关闭弹窗。
- [ ] 游戏客户端实际旗帜已更新。

因此不能勾选工单的“真机验证清单全部通过”结束条件；其余交付结果与未验证项分别记录，避免遗漏被隐藏。

## 最终安装包与源码对应

- 安装包：`dist/desktop/Deep Legends Setup 0.12.2.exe`，103,385,088 bytes，2026-09-17 22:02:55（UTC+8）生成。
- 构建指纹：`de34e1b67412`；与完成打包后重新计算的当前源码指纹一致。
- 安装包 SHA-256：`6cdb96627679f6fa61753aa8cdb1f80204ea92e4b18c07d1e9350101782ac15d`。
- 后端 SHA-256：`d1fe7eebb1cd023c9a6eb237f0f8a26ab1fd404df8ef14a34fb58c40f4671296`。
- `package-build.txt`：完整 `build-desktop.sh 0.12.2`，重编译后端、构建NSIS与安装外壳、验证asar运行文件及包内后端；沿用既有private构建模式，未发布到外部服务。
- `artifact-verification.json`：再次独立核对receipt、实际文件哈希、版本、R105后端函数与前端文案/关键账号均已嵌入最终后端。
- 原0.12.1安装包和receipt/SHA在打包前备份至 `/private/tmp/deep-legends-pre-r105.UDVXUV`。未清理任何R95–R104源码、测试或文档。
- `pro-players-response.json` 是旧快照回归导出的公开DTO；它与固定账号清单逐个匹配，动态数据刻意稀疏。Windows上安装此包后仍需完成上方真机清单。
