# R97 执行账本

> 证据已于 R135 移出工作区，见提交 `b62bca1b9671bccd8c3cf9f7079096d4b33a6fa0`。

执行日期：2026-09-16。范围：`WORKLIST-R97-PRO-ACCOUNT-REGION-REGRESSION-AND-SCOPE-REVERT.md`。未提交、未推送；保留 R95/R96 其它工作与用户 NSIS 配置。

## 工单逐项落实

### 1. 修复账号 region 清零

- `normalizeProAccount` 仅在 trim 后的 region 非空且明确不是 KR 时拒绝。外层 `parseOPGGProPlayers` 的 KR 目录来源约束未放宽。
- 徽章索引的账号输入也复用 `normalizeProAccount`，避免管理页恢复账号、徽章却继续因空 region 丢失的双重口径。
- 新增 `TestR97ProAccountOptionalRegionSurvivesKRDirectory`（按工单放在 `pro_players_test.go`）：构造完整六队包装的真实 Next Flight 结构，分两段传输，T1 五名已核对选手各有一个账号；覆盖账号 region 为 null、缺失、空字符串、纯空白、大小写/空格 KR、显式 NA。
- fixture 经过真实解析和账号组装，而不是直接传空字符串调用 validator。前五种输入均保留五个账号，选手状态 available，徽章对应 T1；显式 NA 不收录。

### 2. 管理页与徽章收回同一六队范围

- `handleProPlayers` 改回 `buildProPlayers(teams, proRoster)`。原六队名单及 33 名选手条目不变：BLG / IG / T1 / HLE / GEN / DK。
- 徽章索引只接收 `proReviewedMemberBadge` 筛过的已核对选手：昵称+实名、成员 team_id/身份、非二队、当前队伍或逐人 AllowTeams 例外。例外仍映射到当前已核对队伍（例如 NIP 来源的 Rookie → IG），不会把来源队伍加入筛选。
- `matchProIdentity` 函数体逐字节不变；PUUID 优先、完整 Riot ID 精确匹配、冲突图拒绝、隐私与无新增请求规则保持。
- `web/pro-players.js` 固定六队筛选（加“全部战队”共七个按钮），不再从响应扩容，并排除 secondary 行；前后加载都不显示扩展队伍。
- `pro_players_ladder.go` 未修改，确认仍调用原 `buildProPlayers(teams, proRoster)`。
- `proDirectoryRoster` 函数体逐字节保留，生产代码无调用，测试仍验证它可显式扩展以备未来复用。
- 二队 UI helper 未删除，但生产徽章全部 secondary=false；管理页也不渲染二队响应行。没有为此改动对局网格或展示布局。
- `docs/pro-players-sources.md` 已同步六队范围、空 region 规则、排除边界与历史来源例外。

### 3. 范围对抗与旧测试更新

- `TestR97ManagementAndBadgesShareReviewedSixTeams` 使用同一份真实 Flight 解析结果，实际调用管理 API handler 与徽章索引：断言六队、33 人、7 个 fixture 账号一致；额外加入 Winners / Young Miracles / Machi Esports / Suning Gaming-S / Anyone's Legend.Young / Suning / VSG / T1 Academy，均不入管理页或四个徽章 surface；T1 内未核对的 Painter 同样排除；Rookie 的历史队标例外正确归入 IG。
- `desktop/pro-players.test.cjs` 新 R97 测试加载真实 HTML 与生产 JS（JSDOM），输入六队外全部上述队名、学院队以及同 code 的 secondary 行，断言按钮、渲染行、账号数量与点击上下文。这里不声称真实浏览器几何/客户端验证。
- 原 R95 专门期待全量一二队徽章的两处测试按 R97 新范围调整：保留学院队不误认 Faker 的负例；PUUID 优先改用白名单内 Chovy/Faker 两身份；跨归属冲突仍保留，不能用删冲突断言换绿。

## 变异实跑

`scripts/r97-mutation-check.py` 共五项，**5/5 KILLED**。每项先跑原版基线，然后用临时 Go overlay 或 JS 源码环境变量注入变异。工作区生产文件不覆盖；编译失败/超时不计 kill。

1. `account-region-required`：把条件改回强制非空，null / missing / empty 三个真实结构 fixture 均断言账号从 5 变 0。
2. `management-expanded-directory`：管理入口恢复全目录，六队/人数/账号数量断言失败。
3. `badge-expanded-directory`：徽章输入恢复全目录，候选范围断言失败。
4. `frontend-expanded-filters`：筛选加入 Winners，筛选集合断言失败。
5. `frontend-secondary-leak`：删除 secondary 排除，同 code 学院行泄露，DOM 断言失败。

命令：

```sh
GOCACHE=/tmp/deep-legends-go-cache GOTMPDIR=/tmp/deep-legends-go-tmp \
python3 scripts/r97-mutation-check.py
```

[完整变异输出](/Users/ly/personal/personal-work/deep-legends/docs/r97-validation/mutation-run.txt) · [矩阵及同目录各项原版/变异完整日志](/Users/ly/personal/personal-work/deep-legends/docs/r97-validation/mutations/mutations.json)

## 全量验证

所有 Go 命令使用 `GOCACHE=/tmp/deep-legends-go-cache GOTMPDIR=/tmp/deep-legends-go-tmp`。

| 验证命令 | 结果 | 完整日志 |
|---|---|---|
| `go build .` | exit 0 | [go-build.txt](/Users/ly/personal/personal-work/deep-legends/docs/r97-validation/go-build.txt) |
| `go vet .` | exit 0 | [go-vet.txt](/Users/ly/personal/personal-work/deep-legends/docs/r97-validation/go-vet.txt) |
| `go test -count=1 -v .` | PASS，94.380s；1151 个顶层测试通过，17 个既有 opt-in 网络/采集 fixture 测试按条件跳过 | [go-test.txt](/Users/ly/personal/personal-work/deep-legends/docs/r97-validation/go-test.txt) |
| `go test -race ./...` | PASS，117.939s，无 DATA RACE | [go-race.txt](/Users/ly/personal/personal-work/deep-legends/docs/r97-validation/go-race.txt) |
| `node --test web/*.test.cjs desktop/*.test.cjs` | PASS，697 tests / 696 pass / 0 fail / 1 skip，255.117s | [node-test.txt](/Users/ly/personal/personal-work/deep-legends/docs/r97-validation/node-test.txt) |

Node 唯一跳过项为已有 Windows/PowerShell release 测试（当前 macOS）；没有新增 skip 或关闭测试。R97 新增 fixture 均执行通过。

补充证据：[Go 定向日志](/Users/ly/personal/personal-work/deep-legends/docs/r97-validation/go-focused.txt)、[前端定向日志](/Users/ly/personal/personal-work/deep-legends/docs/r97-validation/node-focused.txt)、[函数体/名单/用户配置不变核验](/Users/ly/personal/personal-work/deep-legends/docs/r97-validation/invariants.txt)。

按工单不要求真实 LOL 客户端验收；本轮没有查询外部账号归属，也不把 fixture 账号数量当真实来源数量。
