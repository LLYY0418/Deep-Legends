# 09-11 20:55 日志：位置筛选修复与阵容诊断补齐

## 已确定的原因

编辑序列弹窗使用 LCU 位置名 middle/bottom/utility，排名 API 使用 mid/adc/support。
请求在本机参数校验即返回 400。现改为在请求边界转换，不改变已保存的选用分路池。
每次切换递增请求序列，同时核对弹窗对象；旧成功、旧失败、关闭后返回均不得覆盖新状态。
全部与缓存命中也使旧请求失效。关闭、保存、新开弹窗取消旧请求；筛选请求最多等待 15 秒。

## 20:55 日志的阵容结论

最后一次运行 version 0.12.1，共 701 条、log_seq 连续。四次远程阵容查询共八次 HTTP 401，
本地重读令牌后仍为相同令牌，未收到队伍。RANKED/SUMMONER 同期有成功响应，但旧日志只能
证明令牌类型相同，不能证明凭据内容相同。四次查询目标均标记 private，不能据此推断隐私是原因。
日志最后时间为 20:55:10，20:56 的筛选截图不在此日志时间范围内；筛选原因另经代码复现。

## 本次增加的覆盖

| 环节 | 记录与边界 |
| --- | --- |
| 跨接口凭据对照 | SGP 请求的 credential_id 为进程内匿名编号；相同客户端、种类、内容对应同号。客户端、种类、内容变化隔离。内存最多保留 32 个摘要，淘汰后重新编号；不输出 token 或摘要。所有落盘事件仍由存储层统一加 run_id/log_seq。 |
| 本地令牌读取 | current_game_token_read 记录固定端点、阶段、耗时、HTTP 状态（已知时）、响应大小/形状、decode/empty-token/context error；不记录正文。 |
| 上游鉴权 | 固定错误码与类别、已检查对象中的未知字段数、未知错误码数、递归深度限制计数；保留时效、地区匹配、issuer/audience 类型、挑战头和固定关联头的存在性。类别不是服务端权限结论，JWT 解码不代表验证签名。 |
| 请求终止 | transport_result 记录最后 HTTP 状态与最终错误分类。明确区分 decode、response-too-large、incomplete-io、HTTP 5xx、取消与超时。 |
| 阵容结果 | roster_outcome 区分 presence-only、partial-roster、complete-roster、no-roster；响应 success 不再需要被误读为完整阵容成功。 |
| 页面错误 | current_game_client 区分本机 HTTP 401/5xx、网络、读正文、JSON 解码、超时、取消、无效业务响应。 |
| 筛选 | champ_select_filter_client 记录请求序号、原始/转换后位置、request/all/cached/received/stale/failed、数量、耗时、HTTP 与固定错误类别。 |
| 上报健康 | 每条受控前端记录附累计 failed/dropped/suppressed；transport_http_status/error_kind 表示最近一次上报失败，不是业务请求结果。并发最多 32，单次传输 5 秒，临时失败最多重试一次；4xx（429 除外）不重试。仅成功上报才写入完成去重表。 |
| 导出边界 | 点击导出先最多等待 1 秒现有上报，再限时 1.5 秒发 diagnostic_delivery_client/export 汇总，标明未完成数量；失败不阻止下载。导出回应短写/失败在后续日志中可见。 |
| 文件健康 | 写失败累计/待确认数量在诊断摘要可查，恢复后的记录携带此前失败统计；去重表重置可见；文件读超限与不可信类型分开；短写可见。 |

## 验证

- diagnostics_2055_test.go：真实处理函数的五位置契约、跨路由凭据编号、轮换/客户端隔离/并发与内存上限、脱敏、令牌读取失败矩阵、上报白名单、文件写恢复与超限、导出短写。
- web/position-filter.test.cjs：五位置与后台白名单一致、all/cache/newer/reopened 隔离、旧失败静默、失败重试、无效 rows、关闭。
- web/api-diagnostics.test.cjs：HTTP/网络/解码/读错误/超时/取消分类。
- web/runtime.test.cjs、web/export-diagnostics.test.cjs：上报失败/单次重试/去重恢复/并发上限/导出前汇总/重复点击与失败仍可下载。

## 下次实测

使用包含本次源码的新构建；先查询同区、公开战绩、确实在局的玩家，再查询隐藏战绩的在局玩家。
各点击一次重试阵容；在编辑序列中点五个位置并快速切回全部，最后再导出日志。
按 run_id、trace_id、credential_id 对照成功与失败。此次没有更换阵容端点或凭据来源，
没有增加任何选人、匹配、观战写操作，也没有完成 Windows 国服实机阵容验收。

日志仍是有界诊断，不保证断电、页面强制关闭、磁盘持续不可写时所有记录必达。
前端未送达的具体事件不能凭失败计数重建；上报摘要也无法替代服务端未披露的授权原因。
