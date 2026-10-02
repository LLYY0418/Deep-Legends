# R159 执行账本

日期：2026-09-25。基线 0.12.23，递增至 0.12.24。保留了工作区原有 R144–R158 未提交改动；未打安装包。

## 逐项结果

| 项 | 实施与边界 |
|---|---|
| P1 英雄详情 | 解析英雄 JSON 中的 `augments[].augmentIconUrl`，仅接受 `/assets/augments/icons/<数字>.<数字>/<安全文件名>.png`；推荐卡直接使用该路径，目录补充信息不能覆盖它。无目录条目但已有有效图标时不再记 `hexdata_augment_metadata_missing`。|
| P1 图片代理 | 实测 `https://hexdata.com.cn/assets/augments/icons/16.18/highroller_small.png` 返回 302，目标为 `dl.hexdata.com.cn`；目标直接返回 200、`image/png`、6439 字节。因此 `source=hexdata` 只放行上述增益图标路径并直连下载域名，加入独立主机白名单、图片缓存和 Hexdata 功能开关。代理继续检查响应为图片类型。|
| P1 战绩与图鉴 | 从成功解析的英雄 JSON 累积按 ID 去重的增益元数据与图标，异步持久化到现有 `champion-data` 缓存。`/api/gameplay/perks` 每次响应合并已观察索引，即使基础目录早已缓存；英雄详情更新索引后通知已打开的战绩视图刷新。图鉴按图标来源选择受限 Hexdata 代理或原 CommunityDragon 路径。本机客户端与 CommunityDragon 元数据仍可补充缺字段。|
| P2 详情等待 | 海斗详情装配使用本机目录、已观察索引及已成功读取的 CommunityDragon 内存目录；CommunityDragon 的原有后台预取继续进行，但详情不再同步发起失败域名探测。这样保留可用时的稀有度/描述补充，且不会由 R157 退避到期引发周期性 4 秒等待。|
| P3 韩服总览 | 用户确认复现时同时打开了多个玩家总览标签；这与多请求共享 Riot 限流队列的日志现象一致。工单日志未证明存在新的代码缺陷，因此本单未改 Riot 并发与限流预算。App Key 获批额度是否高于当前 `20/1s、100/120s` 仍待确认；在有新证据前不调大额度。|

## 数据来源边界

- 已验证的 Hexdata 增益目录批量 API 返回 403，响应明确拒绝自动化批量访问；停止探测。公开 `/augments` HTML 列出增益，但没有可用的逐 ID 图标路径。因此本单采用实际访问过的英雄子集索引，不声称冷启动覆盖所有增益。首次访问英雄详情后，其返回的有效增益 ID 可供战绩与图鉴复用；未观察的 ID 仍取决于本机客户端或 CommunityDragon。
- R116-A 曾禁止使用 Hexdata CDN 的 `augmentIconUrl`。本单用严格路径白名单与验证过的下载主机修订该限制，仍禁止 `itemImageUrl`，装备与召唤师技能图标路径没有改动。
- 工单引用的 `c958cedf-lol-loot-diagnostics-0925-1919.jsonl` 未在工作区、Downloads 或个人工作目录找到。本轮无法重算工单记录的 2506 次缺失、耗时分布和韩服限流数字。真机验证需要 0.12.24 的新诊断日志。

## 本机验证

- 真实下载域名的图标 HEAD 请求：200 `image/png`，6439 字节。
- 后端 R159 测试：限制非法路径、代理返回图片内容类型、不同英雄增益保留不同图标、索引持久化并跨实例读取、目录补充不覆盖直出图标。R157 回归测试改为断言海斗详情不触发同步目录探测；已有失败退避测试保留。
- 前端 R159 测试：详情与战绩图标都指向受限图片代理，战绩不退化为占位图。
- `node --test backend/web/*.test.cjs desktop/*.test.cjs`：977 项，976 通过、1 项因需要 Windows/PowerShell 跳过、0 失败。最后对详情索引刷新事件增加有效图标判定后，`node --test backend/web/champions.test.cjs` 再次通过 243/243。
- `go test ./... -count=1`：将 R149 断言更新为新的非阻塞装配信号，并移除不必要的新目录创建调用点后，全量通过，backend 198.843 秒。之后把图鉴与详情的图标来源规则收敛到同一 helper，并增加离线 `/api/gameplay/perks` 响应测试；最终 `go test ./backend -run 'TestR159|TestR157|TestR149PrimaryDecorationStartsBeforeRSCFinishes|TestPrivacyStoreDirectoryCreationCallSitesArePinned' -count=1` 通过。
- `go test -race ./backend -run 'TestR159|TestR157SlowCommunityDragonDoesNotExhaustGameplayOrManualDetail' -count=1`、`go vet ./...`、`git diff --check`：通过。
- `go build -o /private/tmp/deep-legends-r159 -ldflags '-X main.version=0.12.24' ./backend`：通过；二进制 `-self-test` 输出“Deep Legends 0.12.24 自检通过：奖池 554 条”。未打安装包。

## Windows 真机复核

1. 不开 League 客户端且 CommunityDragon 不可达时，打开海斗英雄详情：推荐卡图标各不相同、与 Hexdata 对应，诊断 `mayhem_detail_phases_ms.total` 不再周期性出现约 4000ms 的目录等待。
2. 访问过英雄详情后查看包含这些增益 ID 的海克斯战绩槽位与图鉴；重启软件再查看，确认索引仍生效。未访问过的 ID 按来源可用性降级。
3. 新诊断中对比 `hexdata_augment_metadata_missing` 与 `asset_fetch`（`host=dl.hexdata.com.cn`）的请求、失败数，并核对真实图标请求状态；确认装备、召唤师技能图标仍正常。
4. 如要继续分析韩服玩家首次加载慢，已确认本次为多标签并发；还需核对 Riot 开发者后台实际 App Key 限流等级并提供同条件的新日志。
