# R147 站点探测结论（2026-09-24）

使用 `DeepLegends/0.12.19` 标识、现有 Referer、匿名临时 Cookie jar，对同一英雄 887 做少量只读请求。探测程序只输出状态、Cookie 名称与属性、JSON 顶层字段；临时 jar 自动删除，未记录 Cookie 值。

| 请求 | 结果 |
|---|---|
| `GET /hero/887-gwen` | 200 HTML；`Set-Cookie` 含 `hexpage`，`Max-Age=86400`、`Secure`、`HttpOnly`、`SameSite=Lax`；未观察到 `ink` 下发。 |
| 随后 `GET /data/heroes/887.json?v=hexdata-2026-09-18-167d464cf859` | 同一匿名会话曾返回 200，顶层为 `augments`、`items`、`trios` 等八个数组；另一次返回 403。 |
| 随后 `GET /api/hexdata/heroes/887` | 多次返回 200，同一八数组结构；也曾返回 403 `page_token_required`。 |
| `GET /data/postmatch.json` | 曾返回 200，顶层为英雄 ID 映射。 |
| `GET /data/hextech_insights.json` | 返回过 403 `page_token_required`。 |
| `GET /api/hexdata/postmatch`、`GET /api/hexdata/hextech-insights` | 带相同页面 Cookie 后均曾返回 200，也均曾返回 403 `page_token_required`。 |
| `GET /heroes` | 200 HTML，同样下发 `hexpage`；日志已确认原解析器无法从当前 173 行页面取得完整数据及 buildId。 |

三次独立的“英雄页 → 三个 API”探测中，英雄 API 三次均为 200；赛后 API 为 403、403、200；insights API 为 403、200、200。可见页面 Cookie 能放行现有 API 路径，但并不保证每次成功。无需执行页面脚本即可取得 `hexpage`；浏览器截图中的 `ink` 在此次匿名探测中没有出现，不能将其作为必需条件或推测其来源。

`hexpage` 的服务端声明寿命为 24 小时；未等待实际过期，过期后的错误码未知。没有验证静态 JSON 的 `v` 参数可否省略或改值，因为 API 加 Cookie 已有成功样本，本次不改数据路径。`/heroes` 的解析问题与 API 令牌拒绝分开处理，本单不修改解析器。

据此实施：仅内存保存站点 Cookie；取用户当前英雄页面；对令牌 403 最多重新取页并重试一次；过期前按 `Max-Age` 留余量刷新；最终失败继续走已有降级和熔断。站点偶发 403 仍可能使本轮数据缺失，需用新包和诊断日志做真机验收。

实现后用当前 Go 客户端、同一内存 Cookie jar 再做一次单英雄真实链路核验：英雄 887 JSON、`hextech-insights`、`postmatch` 三个现有 API 均返回 200，响应大小分别约 1.18 MB、373 KB、90 KB。该次成功证实代码的页面取令牌、Cookie 发送与三个 API 路径可协同工作；前述不同轮次的偶发 403 仍需在真机长期观察。临时核验测试已删除。
