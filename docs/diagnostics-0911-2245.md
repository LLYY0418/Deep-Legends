# 0911 2245 国服实时阵容分析

## 本次日志的确定证据

只分析最新启动段：文件第 20945–21440 行，版本 `0.12.1`，构建指纹 `b2be5e865f81`。
共 496 条，log_seq 从 1 到 496 连续。前端导出上报 failed/dropped/pending 均为 0，
导出时对照已结束，不是日志缺尾、对照未发出或前端未渲染。

| 路径 | 凭据版本 | 实测结果 |
| --- | --- | --- |
| SUMMARY | credential-3（entitlements） | 38 次 200；不能用不同令牌的成功证明 CURRENT_GAME 凭据有效 |
| RANKED | credential-5（league-session） | 11 次 200 |
| SUMMONER | credential-5 / credential-6（league-session） | 分别 1 / 2 次 200 |
| CURRENT_GAME 正常请求 | credential-5 / credential-6（league-session） | 分别 1 / 13 次 401 |
| 同凭据对照：本人 | credential-6，固定不刷新 | 404 / no-game |
| 同凭据对照：目标 | 同一个 credential-6 | 401 / auth-rejected |

唯一实际执行的双对象对照为 cg-access-10（第 21277–21289 行），耗时 85 ms。
前后本人阶段均为 Lobby，连接／账号稳定，目标非 private、服务器匹配。
JWT sub 与本人 PUUID 字面相等，与目标不等；puuid claim 缺失。这是身份对照，不是签名验证。
完整结论为 self-no-game-target-auth-rejected，roster_scope_comparison_ready=false。

## 问题位置与仍不能证明的部分

好友状态已确认正在游戏；完整阵容的 GSM 请求在拿到阵容之前就收到上游 401，
错误分类为 permission / unauthorized。不是英雄列表解析、分队、页面渲染或者“只能显示排位”的证据。
同份凭据成功读取 SUMMONER，且本人返回不同的 404，因此不能简单归因为所有接口的令牌都失效。

本人 404 与本人当时 Lobby 相符，但 **不是本人阵容成功返回**，不能将这组结果写成
“已经证明只能查询本人”。它把问题缩小到了目标查询授权／接口使用范围，未披露具体服务端策略。

重新核对 [LeagueAkari 的实际调用点](https://raw.githubusercontent.com/LeagueAkari/LeagueAkari/main/src/main/shards/ongoing-game/additional-info-controller.ts)：
进入 in-game 后使用 leagueClient.data.summoner.me.puuid 调用 getByPuuid，补充本人所在对局。
这不是它支持任意好友阵容的证明。历史 spectator GET 对照本次仍为 405，不能自动改成 POST 或启动观战。

## 本次直接修复与补充

- 正常 CURRENT_GAME 的 7 组鉴权恢复中，1 组凭据确实改变，6 组重读后值完全不变。
  原实现仍把这 6 次完全相同的请求重新发送。现在值未变就结束该次恢复，记录跳过原因；
  值改变仍重试，保留原始 401/403，不影响其他路由，不永久关闭后续查询。
- 对照终态新增 roster_comparison_blockers：本例明确记录 self-not-in-game、
  self-roster-not-verified。区分“HTTP 对照跑完”与“在局阵容对照成立”，不额外增加网络探测。
- 添加脱敏回放测试，覆盖本次 Lobby/404/401、凭据不变、凭据变化后恢复、其他路由不受影响、
  请求计数、阶段变化／缺失、超时／缺少对象和日志隐私。

上述修改消除无效重试并补齐解释，不宣称解决了上游拒绝好友阵容。继续增加同类 401 日志
不会自动获得缺失的阵容；进一步区分接口授权范围，需要本人确实在局时的同凭据对照或新的可靠接口证据。

## 最终验证

- `go test ./... -count=1` 通过（105.072s）。
- `go test -race ./... -run 'TestCurrentGame|TestAccessDiagnostics|Test2351Auth|Test2055Credential' -count=1` 通过。
- `go vet ./...` 和 `git diff --check` 通过。
- 初始回放先复现多余重试；修改后通过。旧测试中固定要求重复请求的次数断言已同步更新。
- 本次未构建或部署 Windows 安装包，未进行国服实机验证；不声称好友十人阵容已恢复。
