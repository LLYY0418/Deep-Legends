# WORKLIST-R208：Riot 中转服务：整体限流时全局冷却、每日额度用尽的处理、共享出口 IP 的限流

诊断人：Claude（校验 R206 + 只读核对 `relay/riot-worker/worker.mjs`）。执行人：GPT。日期：2026-10-04。基线：`e2116023`（R206 已实现，0.12.70 未发布）。可与 R206、R207 合并发布。

## 已确认正常（不改）

- 中转地址 `https://riot.yinxiaobia.net` 可用。我这边实测：
  - `GET /r/kr/lol/status/v4/platform-data` → 200，返回韩服状态 JSON；
  - 白名单外的 `/r/kr/lol/challenges/v1/challenges/config` → 404。
- Worker 测试 8/8 通过。`workers_dev=false`、`preview_urls=false`，日志关闭。

## P1　Riot 返回"应用级"429 时只冷却了单个路径

### 问题

`worker.mjs` 收到 Riot 429 后，冷却键是 `host + path`：只有**同一个路径**在 `Retry-After` 内直接返回 429，其他路径照样继续打 Riot。

Riot 的 429 分三类，由响应头 `X-Rate-Limit-Type` 区分：

- `method`：单个接口超限；
- `application`：整个 Key 超限（production 起始额度是 10 秒 500 次、10 分钟 3 万次，所有用户共用）；
- `service`：Riot 后端自身限流。

整体超限（`application`）时，其他路径继续请求只会继续收到 429。Riot 规定持续违反限流的 Key 可能被暂停或吊销，届时所有用户一起失效。

### 修改

1. 429 时读取 `X-Rate-Limit-Type`：
   - `application`：写**全局**冷却标记（按 Riot 主机区分，`asia` / `kr` 各一个）；冷却期内该主机的所有请求直接返回 429，并带剩余的 `Retry-After`；
   - `method`：保持现在的单路径冷却；
   - `service`，或没有这个头：单路径冷却，`Retry-After` 缺失时按 5 秒。
2. 返回给软件的 429 带 `X-Relay-Cooldown: application|method|service`，软件据此退避：`application` 时整个 Riot 功能退避，不再逐个玩家重试。
3. Worker 日志只多记一个 `limit_type`。

### 测试（Worker）

- Riot 对路径 A 返回 `application` 429 → 冷却期内路径 B 不调用 Riot、返回 429；过期后恢复。
- `method` 429 → 只冷却路径 A，路径 B 正常。
- 没有 `X-Rate-Limit-Type` → 单路径冷却。

## P2　Workers 免费额度用尽时的表现

### 问题

Cloudflare Workers 免费版每天 **10 万次请求**（UTC 0 点，即北京时间早上 8 点重置）。**命中缓存也算一次 Worker 请求**（缓存只省 Riot 配额，不省 Worker 额度）。超出后 Cloudflare 直接返回错误页（错误码 1027），不会经过 Worker 代码。

软件现在把这种情况当作普通失败，每个玩家、每个请求都会再试，还可能把 HTML 错误页当 JSON 解析。

### 修改

1. 软件端（`riot_relay.go`）：
   - 中转返回的不是 JSON（`Content-Type` 不是 `application/json`），或返回 Cloudflare 1027 错误页 → 记为 `quota_exhausted`；
   - 之后到下一个 UTC 0 点前不再请求中转，绝活哥 / 职业选手页显示现有的"战绩服务暂时不可用"；
   - 用户自己保存了 Key 时仍直连 Riot，不受影响。
2. 诊断：`riot_relay_probe` 的 `result` 增加 `quota_exhausted`；另记 `riot_relay_request_summary`，每 10 分钟一条，内容为请求数、429 数、各类失败数，用于估算每日用量。
3. `relay/riot-worker/README.md` 补两点：
   - 在 Cloudflare 后台「Workers → 指标」查看每日请求数；
   - 接近 10 万次时，可以升级 Workers 付费版（每月 5 美元，含 1000 万次请求）。

### 测试（Go）

- 中转返回 HTML 错误页 → `quota_exhausted`，之后到 UTC 0 点前不再请求，界面显示不可用。
- 用户保存了 Key → 直连，不受影响。

## P3　同一出口 IP 的用户共用每分钟 120 次

### 问题

Worker 按 `CF-Connecting-IP` 限制每分钟 120 次。国内宽带、校园网、网吧经常是很多人共用一个出口 IP（运营商级 NAT），这些人会共享 120 次/分钟。

打开一个玩家的战绩大约需要 1 次账号 + 1 次比赛列表 + 最多 20 次比赛详情 + 段位 + 熟练度，约 25 次请求。同一出口下两三个人同时看战绩，就可能触发 429，看起来就是"卡住"或"加载很慢"。

### 修改

1. 每 IP 限额从 120 次/分钟改为 **300 次/分钟**。Cloudflare 限流配置的 `period` 只能是 10 或 60 秒，保持 60。
2. 软件端对比赛详情的请求，在 Worker 前先查本地磁盘缓存（比赛详情不会变）。确认现有 `matches_from_disk` 缓存对走中转的请求同样生效，不生效就补上。
3. 软件端收到中转的 IP 级 429（无 `X-Relay-Cooldown`，`Retry-After: 60`）时：
   - 当前页面保留已经加载的部分；
   - 剩余请求等 `Retry-After` 后再发，不显示错误。

### 测试

- Worker：同一 IP 第 301 次 → 429。
- Go：IP 级 429 → 已加载的比赛保留，60 秒后补齐，不发重复请求。

## 收尾

- `node --test relay/riot-worker/*.test.mjs`、`node --test backend/web/*.test.cjs desktop/*.test.cjs`、`go test ./backend -count=1`、`go vet ./backend`、`git diff --check` 全绿。
- 变异（必须 FAIL）：
  - `application` 429 仍只冷却单路径 → Worker 测试 FAIL；
  - HTML 错误页按普通失败重试 → Go 测试 FAIL。
- Worker 改动后执行 `wrangler deploy`，并用状态接口实测 200、白名单外路径实测 404，写进账本。
- `docs/WORKLIST-INDEX.md` 加 R208；账本 `docs/history/ledgers/r208-execution-ledger.md`。
- 构建和发布按用户的暂停指示执行：用户恢复后，与 R206、R207 一起发布，按 R199 规则核对。

## 真机验收（用户）

1. 没有保存自己的 Key，打开绝活哥 / 职业选手战绩，几秒内出结果。
2. 隔几天看一次 Cloudflare 后台的 Worker 每日请求数，离 10 万次还有多远。
