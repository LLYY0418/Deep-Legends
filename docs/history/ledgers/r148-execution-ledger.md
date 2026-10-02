# R148 执行账本

日期：2026-09-24。基线：0.12.19，工作区含 R144–R147 尚未提交的改动。本轮不改版本号、不制作安装包，沿用用户此前对发版范围的明确要求。

## 数据与排序核对

- 真实站点 `/api/hexdata/hextech-insights` 返回 173 名英雄，提供 `games`、`winRate`、`tier`；`answer-cards` 只有 172 条英雄别名。缺别名者仅在 `detailUrl` 为同 ID 的站内数字路径时保留，绝不推断 slug。
- 2026-09-24 对照真实 `/heroes` 的 173 个 ID：按 `winRate` 降序、`games` 降序、ID 升序得到的 173 个位置全部一致；前 20 为 `157,887,4,120,876,200,777,25,10,141,136,910,799,114,245,147,112,11,17,233`。工单推测的样本分档加 Wilson 下界排序在前 20 名有 5 个位置不同，因此采用实测的页面顺序。胜率转换为百分比，样本直接用 `games`，梯度使用官方 `tier`。
- 仍需根页面 `/` 取一次 `hexpage` 令牌才能访问聚合 JSON；它代替旧 `/heroes` HTML 请求。`/heroes` 只保留在人类可读 citation、Referer 和独立的旧断路器测试中，不再用于榜单运行链路。删除了旧榜单解析器、指标正则、本地 Wilson 分档及相应过时断言。共用的 `parseHexdataDocument` 留给图鉴 HTML。
- 另外现场请求 `/augments`、`/augment-rarity`、`/augment/1225-dual-wield` 均返回 200，且各自同时存在符合 `hexdata-` 前缀的 meta 与 data buildId 标记；这三页不复现 `/heroes` 的 buildId 缺失。未在本单修改图鉴解析。

## 实现

- 榜单读取 `answer-cards → meta → insights` 并启动同 buildId 的 postmatch 后台预取；只预取共享聚合，不预取 173 个 hero-json。冷启动预算是 answer、meta、根页面令牌、insights、postmatch 共 5 条，重启命中磁盘缓存后 0 条。真实 Go 客户端的一次冷启动核验：榜单有 173 行，含后台 postmatch 的总耗时约 2338 ms；各请求字节与节流/网络耗时由新增 `hexdata_request` 记录。这个数值只代表该次环境。
- 详情拿到 meta 后，并发进行 hero-json、insights、postmatch，RSC 也并发；CDragon 装饰与 postmatch 重叠执行。新增 `mayhem_detail_phases_ms`，字段为 `meta,hero_json,insights,postmatch,rsc,decorate,total`。
- 全局并发门仍为 3。节流在空闲 2 秒后的首请求不等待，连续请求仍预留至少 300 ms 加抖动的发送时刻；预留时持锁，实际等待时释放锁。
- 新增 `hexdata_request` 事件：类别、路径类别、状态、字节、实际节流等待、网络读取耗时和缓存状态；不输出 Cookie 或 URL 查询串。标准 Go Transport 的 `DisableCompression` 为默认 false，且 `fetchOnceKind` 不设置 `Accept-Encoding`，因此由 Transport 自动请求并解压 gzip。hero-json 数据范围未变。
- 切换到不同英雄时重置详情 tab 为概览并写回设置；重复选择同一英雄保留当前 tab。界面文案未改。

## 启动流量归因

- 启动会调用 `loadProPlayers(runtimeContext, true)`；职业选手缓存过期时，`pro_profiles.go` 的 6 个 worker 抓取 OP.GG 召唤师页，这是启动后数十个大页面请求的主要来源。
- `handleGameplayOverview` 还会调度装备图标预热，延迟 3 秒拉取榜单与最多 20 个英雄 API；它可以解释部分 `lol-api-champion.op.gg` 请求。旧日志中的约 25 个 API 请求不能全部严格归因给这一处。本单未更改启动预热行为。

## 验证

- 已通过针对性 Go 测试：榜单字段/顺序/缺链接降级、冷启动请求预算与重启缓存、详情三请求并发、英雄 JSON 返回 504 或超时时仍尝试 OP.GG RSC、R147 令牌链路、节流无锁等待、诊断不泄漏令牌。RSC mock 本身返回 404，所以该用例核对降级请求确实发出，并不声称 RSC 数据成功返回。
- 已通过针对性 Node 测试：榜单胜率、头部样本、换英雄重置 tab 与同英雄保留 tab。
- `go build -o /tmp/deep-legends-r148 ./backend`、`go vet ./...`：通过。直接 `go build ./backend` 会与现有目录同名，故指定临时输出路径。
- `node --test backend/web/*.test.cjs desktop/*.test.cjs`：962 项，961 通过、1 跳过、0 失败。
- 第一次 `go test -race ./...` 用 259 秒，仅旧的 R116 请求顺序断言失败：它要求 hero-json、insights、postmatch 必须串行按固定顺序出现。改为核对五条请求各一次后，全量重跑通过（259 秒，无 race 报告）。随后只增加了“英雄 JSON 失败仍尝试 RSC”的测试用例；此用例单独 `go test -race` 通过，最终 `go vet ./...` 亦通过。
- 五项实际变异均被对应测试抓到，并逐项恢复原文件：榜单补回 `/heroes` 请求；聚合请求改回串行；空闲请求无条件抖动；预取加入单英雄 JSON；删除换英雄 tab 重置。恢复后 `git diff --check` 通过，针对性 Node 测试再次通过。
- 真机 UI 与改动前后耗时仍需用户用新构建导出日志，对照 `hexdata_request` 和 `mayhem_detail_phases_ms`。未制作安装包，因此本轮无 key mode、指纹或安装包 SHA256。
