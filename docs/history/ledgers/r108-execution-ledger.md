# R108：限流恢复、模式查询与客户端生涯契约

依据：`lol-loot-diagnostics-0918-1053.jsonl`（2926 条）、本轮 5 张截图及用户追加的 OP.GG 查询范围问题。附件仅作诊断证据。保留 R107 及此前工作区修改，按用户要求不构建、不打包、不改版本。

## 战绩查询

- 日志 16 次 `riot_overview_cost`；最初三个总览各下载 20 场全新详情。共有 78 次 `riot_local_rate_limited`、0 次 `riot_rate_limited`，说明这份日志中的提示来自本地预算保护（15 次/秒、90 次/2 分钟），不能据此声称 Riot 已拒绝 Key 或账户额度耗尽。
- KR handler 原来丢弃 `matchFilter`，总是读取所有模式的 ID 和详情，前端随后不断翻页筛选。现在将模式传至 Match v5 ID 查询，单队列直接分页，多队列分别读取缓存 ID 前缀、去重排序合并，只下载当前目标页的详情。单双排合集明确使用 420/440；不通过不精确的游戏类型代替队列。
- 海克斯大乱斗无历史时只需对应队列的 ID 查询，不会为确认“没有”而下载所有普通/排位对局。没有精确队列映射的特殊模式保留兼容回退，但最多自动补查 3 页，之后可手动继续，不能把有限范围内没有匹配说成全部历史都没有。
- [OP.GG 官方历史说明](https://help.op.gg/hc/en-us/articles/31088608024729-I-want-to-view-my-past-stats)写的是约 2～5 个月，并非固定三个月。本轮没有人为截断为 90 天；查询范围仍以数据源实际可返回的记录为准。公开说明未披露 OP.GG 内部查询实现，本项目采用自身 Riot API 的队列筛选能力实现相同的快速空结果体验。
- 已有不可变对局详情继续复用内存、磁盘与合并请求；短期总览快照扩为 1 分钟并包含模式维度。强制刷新仍刷新结果，但可共用正在进行的同一查询。旧请求被取消时，新刷新不会继承其取消错误。
- 页签内缓存各模式的已提交列表和游标，来回切换无需重查。不同模式结果不能拼接；尚未完成的首屏预览不能变成下一页的起点。
- 冷却期间停止无用滚动/手动请求；冷却后按原先的初页或追加页恢复。查询中只显示一个状态，不再同时显示“继续滚动”和加载按钮。未发出请求时不显示假加载动画。

## 生涯契约

实际客户端源码（2026-09-18 读取），而非根据 HTTP 200 推断：

- [Riot shared-components](https://raw.communitydragon.org/latest/plugins/rcp-fe-lol-shared-components/global/default/rcp-fe-lol-shared-components.js)：`_savePreferences` POST `/lol-challenges/v1/update-player-preferences`，`bannerAccent=selectedBannerId`；`_setCurrentSelections` 从挑战 summary 的 `bannerId` 读取；库存匹配 `idSecondary`。
- [Riot profiles](https://raw.communitydragon.org/latest/plugins/rcp-fe-lol-profiles/global/default/rcp-fe-lol-profiles.js)：`backdropObserver` 订阅 `/lol-collections/v1/inventories/{summonerId}/backdrop`，生涯显示 `backdropImage`。背景可来自 recently-played/highest-mastery/summoner-icon，指定皮肤 ID 为 0 并不等于没有背景。

### 旗帜

旧的 `REGALIA_BANNER_SLOT` PATCH 返回 200，但生涯读取另一份数据；延长等待无法修复错误写入入口。现在按目录 ID 找到 `idSecondary`，使用客户端相同的偏好 POST，保留头衔、挑战勋章及 crest。正式操作允许选择未拥有目录项目；是否实际生效以 summary.bannerId 回读为准，不假称任何未拥有物品已解锁。

W6 写入支持检测也改用同一接口，选择另一面已拥有旗帜；保存原始偏好，即使请求失败或取消也尝试恢复，并回读验证恢复。日志区分真实 POST 状态和最终 changed/restored，避免把无效的 200 当作成功。清理头衔/勋章时同步保留 summary 里的实际旗帜和 crest。

### 背景

从当前账号的 collections/backdrop 获取实际显示图片，拒绝其他账号的回包；按原画路径或自动背景 championId 对齐目录。前端优先使用真实 backdropImage，同步当前皮肤和英雄选择，并监听 backdrop 改变。返回 DTO 仅增加安全图片路径和背景类型，不带回其他原始账户字段。

### 头像

点击头像只选中预览，点击“设为头像”才提交。日志三次成功均为 `icon_scope:chat`；生涯头像 PUT 被拒绝，W5 成功试验使用的是另一个已拥有头像。实际客户端 `_saveIcon` 也只调用该账户头像 PUT，不能用聊天栏变化证明生涯头像已改变。继续准确区分聊天范围与账号生涯范围，W5 日志增加 owned/profile 标记。

**未完成真机验证的边界**：当前环境没有连接用户的 Windows LCU，不能声称新版旗帜接口已经在该账号实装成功；也没有证据证明能强制装备被服务端拒绝的未拥有生涯头像。源码修正、模拟接口回读与真机结果分别记录。

## 验证

- 网页全量 492 项通过：头像选择/确认、限流时无冗余按钮、初页和追加页恢复、模式切换缓存及游标隔离、特殊模式自动分页上限。
- Go 全量通过，148.101 秒；之后补充的取消合并请求回归及 R108 定向测试通过。
- R108 全部新增测试及错误码脱敏回归通过 `go test -race`（3.306 秒），包含实际 facade state JSON 输出验证。
- 后端覆盖直接模式查询、空结果缓存、跨队列排序去重/分页缓存、背景实际图片输出、旗帜正确接口/ID、其他偏好保留、延迟生效、200 无变更、取消后恢复与恢复无效检测。旧 loadout 测试夹具已替换为客户端实际契约。
- `git diff --check` 通过。没有执行应用构建、安装包生成、版本更新或发布。
