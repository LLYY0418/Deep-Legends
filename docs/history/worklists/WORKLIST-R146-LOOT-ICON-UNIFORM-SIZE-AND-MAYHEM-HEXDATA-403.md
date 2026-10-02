# WORKLIST-R146：战利品图标统一大小与去底 + 海斗英雄页数据缺失（Hexdata 返回 HTTP 403）

诊断人：Claude（读了 `lol-loot-diagnostics-0924-1326.jsonl`、用户截图、`backend/hexdata.go` / `champions.js` / `app.js` / `app.css`）。
Claude 已把 P1、P2 的代码和 P3 的诊断改完，GPT 负责真实依赖下的复核、真机验证和发版。
日期：2026-09-24。基线：0.12.19，含 R144、R145 的未提交改动。
状态：P1/P2 代码已改待验证（P2 已按用户反馈第二次调整，见 P2 末尾）；P3 已定位到上游要求页面令牌（见 P3 补充），是否对接待用户决定。

用户的三个问题：
1. 冠军杯赛挑战券图标带深青色方底，去掉，和其他图标一样透明。
2. 各图标大小不一，以蓝色精粹为准统一，以后的新物品也一样。
3. 海斗英雄页：胜率、样本、海克斯胜率/评分全是「—」，第三个 tab 也没了，是数据源丢了吗。

---

## P3（先说结论）海斗数据缺失：不是解析变了，是 Hexdata 的三个 JSON 接口现在返回 HTTP 403

### 证据（构建指纹 `43fd229f7451`，05:21–05:26Z，同一次运行）

- `champion_upstream`：op.gg、lol-api-champion.op.gg、api.your.gg、CommunityDragon、ddragon 全部 200，这些源没丢。
- Hexdata 主机（`hexdata.com.cn`）：
  - `/api/hexdata/meta` 成功（`hexdata_shape kind=meta` ×4），`answer-cards` 成功（×3）。
  - 三个 JSON 接口全部 `champion provider returned HTTP 403`，且每次打开英雄都重现（05:24:05 与 05:24:23 两个英雄，各一遍）：
    - `/api/hexdata/heroes/{id}` → `hexdata_fallback module=hero-detail`
    - `/api/hexdata/hextech-insights` → `hexdata_fallback module=hextech-insights` 与 `hexdata_official_hero_tiers_unavailable`
    - `/api/hexdata/postmatch` → `hexdata_fallback module=postmatch` 与 `hexdata_performance_unavailable`
  - `/heroes`（HTML 榜单页）：`hexdata_shape_invalid kind=heroes failure_kind=parse rows=173 fields=519 buildId=""`，05:24:03，一次。
- `mayhem_caution`：rows=10，说明页面上的海克斯行来自 OP.GG RSC（这一路没断），但 OP.GG 那一路不带胜率、样本、综合评分。

### 页面现象与代码的对应关系（都是设计好的降级，不是新 bug）

- 胜率、样本：`champions.go/hexdata.go loadMayhemDetail` 里 `Stats.WinRate` 只来自 hero-json（403）→ 「—」。
- 海克斯行的胜率/样本/综合评分：同样来自 hero-json，OP.GG 合并行只有名称与顺序 → 「—」。梯度徽章仍有，是 insights 不可用后按顺序本地分档的回退（代码已明确标「本地估算」）。
- 第三个 tab：`champions.js` `mayhemDetailTabs`（约第 1164–1166 行）只在 `detail.performance.metrics` 非空时才加「表现」。R116-B 的设计就是 postmatch 取不到就整个 tab 不出现，不显示空壳。所以 tab 消失是 postmatch 403 的直接后果。
- 我确认了：今天之前的日志（1019/1054/1102/1121）没有打开过海斗页，没有 Hexdata 事件，所以**没法在日志里定位它从什么时候开始 403**。

### 根因：没有证据，不下结论

日志只有状态码，没有上游的回复内容，区分不了「上游策略拦截」「缺请求头」「限流」。已知背景：
- R116 的探测记录（`docs/r116-mayhem-data-dimensions-proposal.md` 第 185、296–305 行）：列表接口早就 403 并写明「请停止未经许可的自动化访问」；`/heroes/{id}` 无 Referer 也是 403 `data_not_public`。当前代码已带 Referer（`hexdata.go` 约 1219 行），所以现在的 403 不是老问题回归，除非请求头被改动。
- meta 与 answer-cards 同样带 Referer 却是 200，说明 Referer 至少没有整体失效；被拒的正好是三个「数据量大的按英雄/聚合接口」，也符合上游收紧了访问的猜测。**这只是猜测。**
- Claude 这边访问不到 hexdata.com.cn（云端与用户电脑上的沙箱都被代理拒绝），所以没有实际请求过。**Claude 也没有去试伪装浏览器头之类的绕法**：上游明文警告过未经许可的自动化访问，换头绕过拦截不合适，这一步需要用户自己决定（见下）。

### 已做的修改（Claude，未提交）

`backend/hexdata.go`：Hexdata 返回非 200 时新增诊断事件 `hexdata_upstream_rejected{path, status, error_code, content_type, server}`。`error_code` 只取响应 JSON 的 `error` 字段（最多 48 字符，例如 `data_not_public`），不记响应正文其他内容。新增 `backend/hexdata_rejection_test.go`（含「正文不进日志」断言）；变异：去掉调用 → 该测试 FAIL，已还原。

### GPT 要做

1. 同 P1/P2 一起发版，让下一份日志带上 `hexdata_upstream_rejected`。
2. 拿到日志后按 `error_code` 分流（不要在没有日志前改请求头）：
   - `data_not_public` 或明确的策略拦截：数据源对我们关上了。需要用户联系站点确认授权（R116 第 6.2 条授权确认本来就是前置项），或改用 OP.GG 那一路补胜率。后者要先核实 OP.GG RSC 里到底有没有海克斯胜率、样本字段，**目前没有核实过**，Claude 不假定有。
   - 限流类（429 或带重试头）：调 `hexdataGlobalPace` 间隔。
   - 其他：另开单。
3. `hexdata_shape_invalid kind=heroes`（`/heroes` HTML 解析失败，`buildId` 为空）单独排查：对比 meta 的 buildId `hexdata-2026-09-18-167d464cf859`，怀疑页面结构变了或返回的是拦截页；同样等 `hexdata_upstream_rejected` 之外再补一条解析失败的原因字段后再定。

### 用户要做

- 装新包，打开海斗英雄页，导出日志发回（Claude 看 `error_code`）。
- 如果上游确实是策略拦截，需要你决定是否联系 hexdata.com.cn 获取许可，或者接受这几项数据缺失。这不是代码能替你定的。

---

### P3 补充（`lol-loot-diagnostics-0924-1347.jsonl`，构建 `bf569185d8c8`，新诊断已生效）

- `hexdata_upstream_rejected` ×9：`/api/hexdata/heroes/{id}`、`/hextech-insights`、`/postmatch` 全部 `status=403`、`error_code=page_token_required`，`Server: BaseHTTP/0.6 Python/3.12.14`，`application/json`。
- 结论：**上游给这三个 JSON 接口加了「页面令牌」要求**：不是 Referer 问题，也不是限流。没有令牌的请求（即脱离它自己网页的直接调用）一律 403。与之相符，`/heroes` 页面解析也仍然失败（`hexdata_shape_invalid kind=heroes`，rows=173、fields=519、buildId 空）。`hexdata_circuit_trip` ×3 说明熔断按设计生效，不会一直重试。
- 令牌从哪来、怎么带，日志里没有，Claude 没有请求过站点，**不猜**。
- 这是上游在 09-18 之后明确加上的访问控制。能否对接（从它的页面取令牌再带上）不是技术问题而是授权问题：R116 文档第 6.2 条本来就要求先确认授权，站点此前也明文警告过未经许可的自动化访问。**没有用户决定前，GPT 不要实现取令牌的逻辑。**
- 给用户的选项：A. 联系 hexdata.com.cn 询问是否提供正式的访问方式或令牌，拿到后再对接；B. 暂时接受海斗页只有 OP.GG 那一路（无胜率、样本、评分，无「表现」tab），把「—」的呈现改得更克制；C. 评估 OP.GG 那一路是否有对应字段可补（**尚未核实**）。

## P1 冠军杯赛挑战券去掉深青底

原因：这张图从 CommunityDragon 回落（R145），素材自带不透明的深青方底，其他图标是透明底。R144/R145 没处理。

修改：新增 `backend/loot_icon_background.go`（`stripSolidPNGBackground`），在 `main.go` 的 `serveCommunityDragonImage` 里，仅对 `/fe/lol-loot/assets/loot_item_icons/` 的回落结果调用（结果照旧进缓存）。逻辑：
- 只处理不透明 PNG；边缘 90% 以上像素接近同一底色才动；从边缘沿颜色连续的像素把底抠成透明（与底色距离 35–70 之间渐变，边缘不留硬边）；主体（金色票券）颜色差远大于阈值，不会被吃掉。
- 已有透明度、边缘不是纯色、抠掉面积不足 5% 或超过 85%、解码失败：原样返回。
- 这套做法与 `web/loot-icons/README.md` 里 `promotion-chest.png`「移除连续的深青背景」的既有做法一致，区别是在运行时对回落图做，因此以后遇到同类带底图标也自动适用。

测试：新增 `backend/loot_icon_background_test.go` 2 条（背景透明、主体不透明；透明图/杂边图/非法数据原样返回）；变异「不写透明度」→ FAIL，已还原。

**没有见过真实文件**：Claude 的沙箱和用户电脑的沙箱都下不了 CommunityDragon 的图，只能用合成图（深青渐变底 + 金色块）验证。真实素材若底色有强烈暗角，阈值 70 可能留一圈淡边或吞掉主体外缘，以真机截图为准，需要时调 `lootBackgroundMaxDistance` / `lootBackgroundMaxStep`。稳妥的备选是 GPT 下载原图，按 README 的做法离线处理后内置到 `web/loot-icons/`（并在 `lootImagePaths` 固定表加 `MATERIAL_CLASHTICKETS`），此时运行时回落不再用于该图。

## P2 所有战利品图标统一视觉大小，以蓝色精粹为基准（第二版，按用户反馈调整）

原因：以前靠 CSS 里逐个物品写缩放系数（精粹 1.65、圣堂花火 .94、海克斯宝箱 1.22），其余图标为 1，新物品（如挑战券）没有系数所以大小各异。

修改：
- `web/app.js`：新增 `lootIconFit(alpha, size)`（纯函数）与 `normalizeLootIcon(image)`；图片 `load` 后在 128×128 画布上量出**可见像素外框**，把外框最大边缩放到 76px（蓝色精粹在 70px 图框里 169/256 × 1.65 ≈ 76px，即以它为基准），并把外框居中。缩放限制在 0.6–3 倍；图自带不透明底（外框铺满整图）时不归一，保持原样；画布不可用（异常）时静默保持原样。规则只看图，不看物品类型，所以新物品自动适用。
- `web/app.css`：删掉三条按物品类型的缩放系数，`transform` 增加 `--loot-icon-dx/dy` 位移变量。
- 界面没有新增任何文字。

**第二版（用户反馈「其余的都太大，蓝色精粹没这么大」）**：第一版按外框最大边归一到 76px，实心方块（挑战券、宝箱、花火）比瘦长的蓝色水晶显得大很多。改为按外框**几何平均边长**归一，目标 60px（蓝色精粹旧版 1.65 倍下约 48×76，几何平均约 60），同时最大边不超过 78px。蓝色精粹保持约 1.64 倍（基本不变），本地素材的结果（70px 图框内可见宽×高）：蓝/橙精粹 48×74，两种宝箱约 62×58，钥匙 51×71，钥匙碎片 49×74；实心方块/圆形约 60px。这些数字是用 PIL 对已内置素材算出的，挑战券与花火没有素材可算，以真机为准。

测试：新增 `web/r146.test.cjs` 3 条（蓝色精粹落在 76px；宽扁宝箱、瘦高钥匙、小图标归到同一尺寸；居中；空图/不透明图返回 null；CSS 不再有逐物品系数；加载时调用归一化）；变异：去掉 load 时的调用 → FAIL，已还原。`node --test backend/web/*.test.cjs` 697/697 通过。

已知边界：
- 依赖去底之后才能量出主体：挑战券必须先 P1 去底，P2 才对它生效。
- 新图标若主体带很大的光晕（半透明），外框会偏大，图会略小于基准；这是「按可见像素」的固有取舍，阈值 alpha>16。

## GPT 要做（P1/P2）

1. 真实依赖下跑全量：`go build`、`go vet ./...`、`go test ./...`，与 R145 之后基线比较，R135 记录的既有失败不得增加。Claude 用桩依赖跑的全量失败集合是 12 项且与改动前一致（桩造成），**不是最终验证**。
2. 变更范围：`backend/main.go`（只加了 P1 的调用）、`backend/hexdata.go`（只加了诊断与 `io` 导入）、`backend/loot_icon_background.go`（新）、`backend/loot_icon_background_test.go`（新）、`backend/hexdata_rejection_test.go`（新）、`web/app.js`、`web/app.css`、`web/r146.test.cjs`（新）。
3. 与 R144/R145 同一个发版，`private` 模式，账本记 key mode、指纹、SHA256，写 `docs/r146-execution-ledger.md`，更新 `docs/WORKLIST-INDEX.md`。

## 用户在真机上做

- 账户与物品页：挑战券没有方底；蓝色精粹、橙色精粹、钥匙、钥匙碎片、宝箱、圣堂花火、神话精萃等图标看起来大小一致；截图发回来。若挑战券边缘有淡边或票券边缘被吃掉，也发截图。
- 海斗英雄页：导出日志（P3）。
