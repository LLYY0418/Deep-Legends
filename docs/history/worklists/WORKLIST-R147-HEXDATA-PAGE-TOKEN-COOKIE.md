# WORKLIST-R147：Hexdata 新增「页面令牌」（cookie）要求，海斗英雄页胜率 / 海克斯评分 / 表现 tab 全部缺失

诊断人：Claude（R146 P3 的日志与用户截图）。**执行人 GPT 负责实现。Claude 没有改任何代码，也没有请求过 hexdata.com.cn**（Claude 的沙箱访问不了该站点），所以本单里凡是「站点行为」的部分都来自日志和用户的浏览器截图，实现前必须先做 §3 的探测确认。
日期：2026-09-24。基线：0.12.19，含 R144–R146 的未提交改动。
状态：待 GPT 实现。前置：无（授权已确认，见 §1）。

---

## 1. 授权与边界

- 用户在 2026-09-24 对话中明确说明「已向站点确认过许可」（指个人本地只读工具按需取数据）。Claude 没有看到许可原文。**GPT 开工时请让用户把许可依据（站长回复截图或链接）的存放位置写进 `docs/history/ledgers/r147-execution-ledger.md`**，以后出问题可回溯。
- 许可不改变 R116 第 6 节的红线，全部继续有效：按需单资源（用户点开某个英雄才请求）、禁止为 173 个英雄预拉取、按 `buildId` 缓存每版本只取一次、不做二次分发、任何新维度取不到就整块隐藏。
- **不伪装浏览器**：保持现有 `User-Agent: DeepLegends/<version> (…)` 与 `Referer`，不要为了通过检查而去仿造 `Sec-Ch-Ua`、`Sec-Fetch-*` 或 Chrome 的 UA。如果站点靠这些头才放行，停下来回报，不要绕。

## 2. 证据

`lol-loot-diagnostics-0924-1347.jsonl`（构建 `bf569185d8c8`，新诊断已生效）：

- `hexdata_upstream_rejected` ×9：`/api/hexdata/heroes/{id}`、`/api/hexdata/hextech-insights`、`/api/hexdata/postmatch` 全部 `status=403`、`error_code=page_token_required`，`Server: BaseHTTP/0.6 Python/3.12.14`。
- 同一次运行里 `/api/hexdata/meta`、`answer-cards` 仍是 200（`hexdata_shape` ×15）；`/heroes` 页面 `hexdata_shape_invalid kind=heroes failure_kind=parse rows=173 fields=519 buildId=""`。
- `hexdata_circuit_trip` ×3：熔断按设计生效。
- 页面后果（都是既有降级，不是新 bug）：胜率/样本来自 hero-json → 「—」；海克斯行的胜率/样本/评分同源 → 「—」；insights 不可用 → 英雄梯度回退到本地分档（已标「本地估算」）；`champions.js` `mayhemDetailTabs` 只在 `detail.performance.metrics` 非空时才加「表现」→ tab 消失。

用户在浏览器里打开英雄页（`/hero/887-gwen`）的开发者工具截图（Edge 153，同一天 05:53 GMT）：

- 浏览器实际请求的是静态数据文件 `GET https://hexdata.com.cn/data/heroes/887.json?v=hexdata-2026-09-18-167d464cf859`，返回 200，`Content-Type: application/json`，`Cache-Control: public, max-age=600, stale-while-revalidate=3600`，带 `ETag` 与 `Last-Modified`，`Content-Encoding: gzip`，`Referer: https://hexdata.com.cn/hero/887-gwen`。
- 请求带两个 cookie：`ink=<hex>.<unix时间>.<hex>` 与 `hexpage=v1_<unix时间>_<数字>_<签名>`。`hexpage` 里的时间戳换算是 05:53 UTC，与用户打开页面的时间一致。
- 推断（**未验证**）：这两个 cookie 由站点在用户打开英雄页面时下发，带时间戳和签名，会过期；「page_token_required」指的就是缺它。我们现在调用的 `/api/hexdata/...` 路径没带 cookie，所以被拒。截图里的 cookie 取值已在对话中出现，属于短期会话值，**不要抄进任何文件、日志、测试或账本**。

## 3. 第 0 步：探测（GPT 在用户真机上做，最多几十个请求，全部单资源、非批量）

目的是把推断变成事实，再动手。每步记录**状态码、头名称、响应结构**，不记录 cookie 值：

1. 带 `Referer` 请求 `GET /hero/887-gwen`（HTML）：看响应有没有 `Set-Cookie`（`ink` / `hexpage`），还是令牌藏在 HTML/脚本里、由页面脚本另一个接口取得。
2. 用第 1 步拿到的 cookie 请求 `GET /data/heroes/887.json?v=<当前 buildId>`：JSON 结构是否与现有 `parseHexdataHeroJSON`（对应 `/api/hexdata/heroes/{id}`）的夹具一致；`v` 参数是否必需、是否必须等于 meta 里的 `buildId`。
3. 用同一批 cookie 再试 `/api/hexdata/heroes/887`：是否也放行（若放行，只需要加 cookie，路径不用改）。
4. `postmatch` 与 `hextech-insights`：站点页面对应的静态文件路径是什么（很可能也在 `/data/` 下，**只有浏览器截图能确认**，请用户在对应页面的 Network 面板里看一眼），同一批 cookie 是否通用，还是每个页面各自下发。
5. 令牌寿命：cookie 里的时间戳与 `Set-Cookie` 的 `Max-Age`/`Expires`；过期后用旧 cookie 请求得到的状态码和 `error_code`（可能仍是 `page_token_required`，也可能是别的，例如 `page_token_expired`）。
6. `/heroes` 榜单页：带 cookie 后是否仍解析失败（`rows=173 fields=519 buildId=""`）；如果仍失败，看是页面结构变了还是返回的是拦截页。这一项与令牌可能无关，单独记结论，需要的话另开工单。
7. **停止条件**：如果令牌必须执行页面脚本才能得到（不是简单的 `Set-Cookie` 或可解析的响应内容），**不要**引入无头浏览器或脚本执行，停下来回报用户，本单到此结束。

探测结论写进 `docs/r147-probe-findings.md`（不含 cookie 取值），再进入 §4。

## 4. 实现要求（以探测结果为准，下面是设计约束）

`backend/hexdata.go`（`hexdataClient.fetchOnce` 约 1175–1283 行、`allowedPath` 约 964 行、`reportUpstreamRejection` 文末）：

1. **令牌获取**：为 `hexdataClient` 增加一个只在内存里的 cookie jar（`net/http/cookiejar`，限定主机 `hexdata.com.cn`，**不落盘、不进日志、不进诊断事件**）。第一次需要令牌时，带 `Referer` 请求用户正在看的那个英雄页面（`/hero/{id}-{slug}`；聚合类接口用同一 jar），从响应里取 cookie。
2. **单飞与节流**：并发的多个请求共用一次令牌获取（`singleflight` 或互斥），走既有的 `hexdataGlobalPace`；令牌获取本身算一次 hexdata 请求，纳入冷启动请求预算（现有测试钉住「冷启动 hexdata 请求数」，需要相应更新数字并说明原因）。
3. **失效重取**：收到 403 且 `error_code` 为令牌类（`page_token_required` 或探测确认的过期码）时，**最多重取一次令牌并重试一次原请求**；重试仍失败才算失败。恢复成功的这一轮**不得**给熔断计失败（`recordFailure` 只在最终失败时记）。
4. **主动过期**：按探测得到的寿命提前刷新（留安全余量），不要等 403。寿命未知时以 403 触发为准，不硬编码猜测值。
5. **路径**：按 §3 的结论把 hero-json、postmatch、insights 的请求改到正确路径（若 `/api/hexdata/...` 加 cookie 后可用，则保持不变）。新增 `/data/heroes/{id}.json` 等路径时同步加入 `allowedPath` 白名单，用严格正则（数字 id、`v` 只接受 `hexdata-` 开头的 buildId 形态），无 ID 的列表路径依旧禁止。
6. **缓存**：沿用现有按 `buildId` 的磁盘缓存与 `ETag/If-None-Match`；站点给的 `Cache-Control: max-age=600` 只当参考，不改变本应用「每版本只取一次」的策略。cookie 不进缓存键。
7. **诊断**（只记元数据，禁止 cookie 值和 `Set-Cookie` 内容）：新增 `hexdata_token_acquired{kind, status, duration_ms, ok}`、`hexdata_token_refreshed{reason}`；保留 `hexdata_upstream_rejected`（已有，只记错误码）。
8. **降级不变**：令牌拿不到或站点再次收紧时，走现有降级（整块隐藏、OP.GG 一路继续），熔断照常。**不要**因为令牌失败去重试风暴。
9. `/heroes` 页面解析失败若与令牌无关，本单只记录结论，不顺手改解析。

## 5. 测试要求

Go（用假上游，不要访问真站点）：
- 假上游按「无有效 cookie → 403 `page_token_required`」实现，验证：先取页面拿 cookie，再取 JSON 成功；cookie 过期后自动重取一次并成功；重取仍失败时最终失败且只计一次熔断失败；恢复成功的一轮不计失败。
- 并发 10 个请求只触发一次令牌获取（`-race`）。
- 诊断与日志中不出现 cookie 取值（用可识别的假 cookie 值做断言，同 `hexdata_rejection_test.go` 的做法）。
- cookie 不落盘：缓存目录里搜不到假 cookie 值。
- 路径白名单：新路径正则的接受与拒绝表（含列表路径、非数字 id、畸形 `v`）。
- 对抗变异（每项都要让对应测试 FAIL，并整文件还原）：去掉失效重取；重取无次数上限；令牌获取不单飞；把 cookie 写进诊断；恢复后仍计熔断失败；放开列表路径。
- 前端：既有「表现」tab 出现/隐藏的测试保持通过；无需新增文字。

## 6. 真机验证（用户）

- 装新包，联网，打开海斗英雄页：胜率、样本、海克斯的胜率/样本/综合评分应有值，英雄梯度为官方档位，「表现」tab 出现。
- 连续打开 5 个不同英雄，导出日志给 Claude：应有 `hexdata_token_acquired`，`hexdata_upstream_rejected` 只在令牌真过期时零星出现且随后恢复，**没有** `hexdata_circuit_trip`。
- 观察请求量：每个新英雄最多 1 次页面 + 1 次数据请求（聚合类同 build 只取一次）。

## 7. GPT 的其他工作

真实依赖下 `go build` / `go vet ./...` / `go test ./...` 与 R146 之后的基线对比，R135 记录的既有失败不得增加；`node --test backend/web/*.test.cjs desktop/*.test.cjs`。发版（版本号递增，`private` 模式，账本记 key mode、指纹、SHA256），写 `docs/history/ledgers/r147-execution-ledger.md` 与 `docs/r147-probe-findings.md`，更新 `docs/WORKLIST-INDEX.md`。可与 R146 P1/P2 同一个发版。

## 8. 已知风险

- 站点可能随时再改令牌机制，这一层是「跟随站点」的脆弱点；所以降级路径必须一直保留并保持测试覆盖。
- 探测只能确认当前行为；`Set-Cookie` 是否每次都新发、寿命多长，以探测为准。
- 如果站点的许可只覆盖「人工浏览」而不覆盖后台取页面拿令牌，那么第 4.1 条的做法超出许可范围。**GPT 开工前请向用户确认许可的措辞是否覆盖「应用代用户打开页面并取数据」**，不覆盖就停在第 0 步，只交探测结论。
