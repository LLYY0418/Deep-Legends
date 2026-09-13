# 09-11 21:53：国服当前对局诊断

后续已补充 [本人／对象同凭据对照诊断](current-game-access-diagnostics.md)。下文保留当时日志的证据与未验证边界。

## 本轮范围

分析 `lol-loot-diagnostics-0911-2153.jsonl` 的最后一次运行。另按要求将工具页签调整为
自动 → 维护 → 征召 → 生涯 → 领奖，五个图标统一为 26×26 SVG（图形范围均为 20×20）。
未改动阵容请求、鉴权、重试策略；未操作真实客户端，未宣称国服好友阵容已修复。

界面验证：`web/suite.test.cjs`、`desktop/overview-render.test.cjs` 共 69 项通过；真实 Chromium
检查 1600/1440/820/620/420 五种窗口宽度下的图标尺寸、图形范围、标签与页面溢出、键盘焦点、
所选页签和实际面板。窄窗口使用页签栏内横向滚动，不压缩图标或将两字标题折行。

## 新日志确认的事实

- 版本 0.12.1，构建指纹 `6df2d2c287e3`；最后运行 `3ea117e6dba509bcebddd226`。
- 文件第 18683 行起，21:52:03 至 21:53:00，共 303 条，`log_seq` 从 1 到 303 连续。
- 同一运行、同一匿名凭据 `credential-4`：RANKED 十次 200，SUMMONER 两次 200，CURRENT_GAME 四次 401。
  此编号关联的是客户端、令牌种类和内容，而不只是令牌种类相同。
- 两次远程查询各一次初始请求、一次刷新后重试；本地令牌读取 200，但内容未变化。
- 两次查询的 `target_private=false`、`target_is_self=false`、`server_matches=true`（第 18804、18870 行）。
  因此不能继续用“隐藏战绩”或“凭据整体失效”解释全部失败。
- 四次阵容失败在第 18808、18814、18874、18879 行。实际 HTTP 状态都是 401，
  不是截图文案里泛称的 401/403。响应 JSON 127 字节，固定错误码 UNAUTHORIZED，类别 permission / unauthorized。
- 最后一次远程查询退回好友状态：正在游戏、queue 2400，结果 `presence-only`，零队伍、零玩家；前端已收到并展示。
- 导出汇总第 18957 行：failed=0、dropped=0、pending=0、error_kind=none。
  当前记录没有上报失败或导出时待发送迹象；这不等于所有可能的外部故障都已被日志覆盖。

## 与 LeagueAkari 的关键差异

对照官方仓库文件：

- [additional-info-controller.ts](https://github.com/LeagueAkari/LeagueAkari/blob/main/src/main/shards/ongoing-game/additional-info-controller.ts)：
  `update()` 在自己处于 in-game 时，从 `leagueClient.data.summoner.me.puuid` 取当前账号，
  然后传给 `_getGsmGameMembers` → `sgp.api.gsm.getByPuuid`。这不是查询任意好友的实现。
- `gsm.ts` 定义的 GET 路径及 league-session 令牌类型与本项目一致。
- `http-client-controller.ts` 中 `x-akari-token-type`、`x-akari-sgp-server-id` 是内部路由标记，
  发出 HTTP 前会被删除；不能把它们当成我们漏发的服务端必需请求头。
- 本项目 `overview_current_game_sgp.go` 的 `fetchCNCurrentGame` 则将其他玩家的 PlayerRef 传给该接口。

## 已定位与尚未证明的边界

直接失败点是 **GSM 对其他玩家的当前对局请求返回授权拒绝**。前端、阵容解析、好友状态和日志导出
不是本次拿不到队伍的直接原因。把“参考项目能查询自己”推导成“相同端点能查任意好友”，缺少证据。

查询对象权限/调用范围是当前优先验证的假设，并非已证实的服务端 self-only 限制。
同一凭据在其他服务成功，不保证它获得 GSM 权限。此次尚无同一凭据查询本人在局阵容的对照结果；
也没有服务端授权策略或成功的好友阵容响应，不能进一步断言唯一根因。

诊断仍有一个未知错误字段（只记录数量），没有未知固定错误码；不能猜测被隐藏字段的内容。
JWT 可解码、不含 audience 等元数据也不能单独证明其签名或权限是否有效。

## 后续最有区分度的验证

需要在用户授权的国服实机环境中，对照同一登录账号、同一有效凭据查询“本人确实正在进行的对局”与
“其他玩家的对局”的 GSM 结果。本人的本地 LCU 阵容成功不能替代这个对照，因为本项目会提前返回本地结果。

若本人 GSM 成功而其他玩家失败，优先追查查询对象授权范围；若本人也失败，优先追查 GSM 对该令牌的
授权条件/接口契约。没有这个对照前，不继续堆叠相同凭据重试，也不以历史战绩或观战启动接口伪装成实时阵容。
