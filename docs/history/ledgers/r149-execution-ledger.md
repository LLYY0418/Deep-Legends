# R149 执行账本

日期：2026-09-24。基线：0.12.19，工作区保留 R144–R148 的既有改动。按照用户此前要求，本轮不改版本、不制作安装包。

## 海斗首开

- R148 的 `waitForPace` 已经在全局锁下仅预留发送时刻，释放锁后才睡眠；日志中 1605 ms 的等待不是互斥锁被持有，而是 300 ms 间隔与最多 800 ms 抖动连续预约后的排队结果。
- R149 改为空闲 2 秒后给 4 个请求突发额度，额度内 `pace_wait_ms=0`；额度用完再按至少 300 ms 的间隔加最多 250 ms 抖动预约。仍在同一进程使用容量 3 的 `hexdataGlobalGate`，没有增加请求总数。跨电脑没有共享门控，真机多用户聚集的上游效果仍需日志观察。
- 新 build 需要受保护聚合时，根令牌页与 answer 同时启动；完整磁盘缓存命中时不额外取令牌。聚合 JSON 依旧必须等令牌，原来的单飞和 403 有限重试保留。
- 进入海斗榜单时，并发预热 Data Dragon 静态描述目录和 CommunityDragon 海克斯目录。两者原已由通用 `championDataCache` 磁盘持久化：Data Dragon URL 按补丁版本区分，CommunityDragon 为 `/latest` 固定 URL、fresh TTL 24h、stale 7d，单条上限 8 MiB。定向测试以同一目录重启 provider，预热首次 5 条目录请求，重启 0 条。因此没有再加一套磁盘缓存。用户日志中的 `cache: miss` 表示该次没有可用的 fresh 条目；单凭两次 miss 不能断言实现未落盘，还需核对两次运行的缓存目录、TTL/更新时刻与错误日志。
- 英雄 JSON 解析后立即装饰其独立的海克斯、装备、阶段数据，并生成召唤师技能配对及样本分档，随后才等 RSC；合并 RSC 海克斯行、把技能配对接入最终 build、以及 RSC build 装饰仍在 RSC 后。`decorate` 耗时现在累计实际装饰片段，排除了中途等待 RSC 的时间。没有启动两段式响应或预取单英雄 JSON。
- 旧 Hexdata 冷启动上限仍是 answer、meta、根令牌页、insights、postmatch 5 条，加用户点开的每个英雄 1 条 hero-json；其他主机的目录请求另行统计。

## 生涯页删除清单与保留理由

- 删除 `suite.js` 的头像卡、旗帜卡、两个跳转按钮、段位旗按钮及只服务它们的绑定；删除 `suite.css` 的专属样式、`suite.js` 的 `facadeIconImage` 和 `rankBanner` 草稿/渲染状态、`demo-data.js` 的 `rank-banner` 模拟写入、`app.js` 已无调用者的 `deepLegendsOpenFacadeCollection`。
- 删除后端 `writeFacadeRankBanner`、`rank-banner` 动作分发/诊断白名单、请求和状态里的 `RankBanner` 字段。已新增测试，旧动作返回 `errFacadeInvalid` 且不请求 LCU。客户端已有的 `preferredBannerType` 未写入或重置。
- 保留收藏页 `facade-collection`、`favorites-facade.js`、`/api/facade/icons` 与 `/api/facade/banners` 只读目录；它们仍有独立导航入口。保留生涯预览、背景、聊天身份与展示清理。
- 保留 `clear-border` 对 regalia 的 GET/PUT：该操作必须把读取的 `preferredBannerType` 原样带回，不能因删除段位旗按钮而清空。新增实请求模拟测试保护这条行为。保留 `r99_probe.go` 只读探针、`BannerAccent` 状态和挑战偏好写入中的旗帜保留逻辑，因卸下勋章/头衔依然需要它们。删除“这一页会改什么”中已不能编辑的“赛季旗帜”词，其余界面文案不新增。

## 启动职业数据调查（本单不改）

- `main.go` 每次启动调用 `loadProPlayers(runtimeContext, true)`；`force=true` 即使读到 24h 磁盘快照也继续刷新目录。职业 profile 正常 fresh TTL 15m、未定级 72h、失败 30s，成功才写 `pro-profile-v2` 磁盘缓存，并用 6 个 worker 拉 OP.GG 页面。职业梯度页本身已有 24h 磁盘缓存；`lol-api-champion` 详情有 6h fresh/24h stale。两次启动都为 `cache: miss` 可能来自 force 刷新、profile 15m 过期、失败未入盘或请求 key 不同，不能据此认定所有缓存缺失。
- 建议另开工单评估“先显示 24h 快照，后台按可见性/空闲刷新”与 profile TTL。单纯延迟 20 秒只能推迟流量；统一延长到 6–12h 会降低段位/活跃度新鲜度。本单未改职业选手功能。

## 验证

- 定向 Go：突发额度、无锁等待、answer/令牌并发、两目录预热并发与重启磁盘命中、英雄装饰与 RSC 并发、旧请求预算、段位旗写入拒绝与 `clear-border` 保留旗帜偏好，均通过；定向 `-race` 通过。
- 定向 Node：`r101.test.cjs` 与 `r123.test.cjs` 11 项通过，生涯渲染只剩预览和“这一页会改什么”两张左侧卡，收藏页仍有头像/旗帜独立入口。
- 七项实际变异均被测试抓到并逐项恢复源码：突发额度置零、突发内无条件加抖动、两目录预热串行、预取扩大到 per-hero、加回头像卡、恢复 `rank-banner` 动作、`clear-border` 覆盖当前旗帜偏好。恢复后定向 Go/Node 再次通过，`git diff --check` 通过。
- 真实依赖下 `go build -o /tmp/deep-legends-r149 ./backend` 与 `go vet ./...` 通过；`git diff --check` 通过。Node 全量 `node --test backend/web/*.test.cjs desktop/*.test.cjs`：962 项、通过 961、跳过 1、失败 0。
- `go test -race ./...` 第一次在本轮召唤师配对提前计算之前通过；最终代码的第一次全量复跑仅有未改动的 `TestR92ProLadderStartsBeforeSupplementsFinish` 失败，报 `published directory snapshot mutated`。该测试把异步梯度补全的完成时刻当作固定先后；单独 `-race -count=10` 均通过。不并行运行前端测试的最终全量复跑通过（249 秒、无 race 报告）。本单未改职业选手代码或测试；该用例的时序不稳定仍应后续单独处理。
- 真机性能验收仍需用户用新构建导出日志，对照 `hexdata_request.pace_wait_ms` 和 `mayhem_detail_phases_ms`；本轮按用户要求暂缓版本与打包，尚无真机日志可核对 2–3 秒目标。
