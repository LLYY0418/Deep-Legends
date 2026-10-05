# R224 执行账本（2026-10-05）

状态：**代码修复与本地回归完成；Windows 真机验收未完成，工单保留进行中。**

工单：[WORKLIST-R224](WORKLIST-R224-ARENA-BRAVERY-PICK-BLOCKED-BY-INTENT-FAILURE-STALE-TEAMTWO-DUPLICATE-SELF-LIVE-FLICKER-CHIP-GAP-AND-SQUAD-FIRST.md)。在既有 R223 未提交改动上继续，保留 R223；未提交、未发布、未生成安装包。版本保持当前 `desktop/package.json` 的 **0.12.73**，没有申请发布版本号。

## P1 自动选用

- `pickFailures` / `pickFailed` 按 **action + write_step + champion** 分开；intent、hover、lock 各有两次预算。同一动作最多触碰三个不同候选的 R200 护栏保留。
- Arena `-3` 在 PLANNING 只等待本人回合，不发 intent。BAN_PICK 本人动作生效后，lock-now 发 PATCH（`championId:-3, completed:true`）；show-then-lock / show-only 先 PATCH 亮出，继续原有锁定等待策略。
- 锁定确认接受回读 `-3`，也接受 **pick 动作 completed=true 且英雄 ID>0**。后者同时用于 postflight，避免诊断假失败；手动接管观察也不把此随机结果视为用户主动换人。
- `advancing` 根据真实配置序列、网格/可用列表和对应步骤预算查下一个候选。单候选第二次失败保留“已停止尝试，请手动操作”。停止日志按动作去重，卡片显示“自动选用已停止”，包括原本在前端硬编码“可选”的勇敢举动。
- 新 gameID / 离开后重新进入 ChampSelect 清空状态，增加回归钉子；R200/R202 卡牌、备战席/交换的既有测试随全量通过。

测试：`TestR224BraverySkipsPlanningThenExecutesStrategies`、`TestR224IntentFailureDoesNotConsumeLockBudget`、`TestR224SingleCandidateFailureExplainsStopAndResetsOnDodge`、`TestR224BraveryRandomChampionConfirmsCompletedOnly`。最后一项包含完整 evaluate 流程、manual-takeover 和超过两秒后的 postflight 核对。

**Windows 未验：** 本环境为 macOS，没有连接到 Windows 游戏客户端。上述 PATCH 是原有请求流程的实现和本地夹具验证，不能宣称已在国服实测成功。尚无成功请求原文及真实 `write-result http-success` / `confirmation applied` 记录；是否需要 PATCH 后 POST complete，必须由真机证据确定。两种策略、普通英雄预选失败到锁定、禁用正常均待 Windows 验证。

## P2 名单残留和补人

- `gameflowLiveRoster` 在 `isArenaQueue(queue, mode)` 时只接收 teamOne；teamTwo 非空写 `stale_team_two_dropped`，事件业务字段仅 `count / queue_id / game_id`。
- 所有模式在原始名单中按可见/解码的 PUUID 去重，teamOne 先加入所以优先保留；补全后再对最终 response 去重。`live_roster_shape` 增加 `duplicates_dropped`，仍保留残余重复计数 `duplicate_player_refs`。
- 位置 shape 诊断也跳过 Arena 的 teamTwo。
- **核对发现工单所称“现有补人逻辑”并不覆盖全部缺员：** `recordArenaMissingSession` 原本只记诊断，`applyArenaLiveGrouping` 只补已记住的小队成员。本次新增 `recoverArenaPlayerList`，在 playerlist 人数多于 session 时，以精确 Riot ID/已有小队身份恢复缺员；解析失败只保留 playerlist 已知名字，不编造 PUUID、英雄或小队。英雄只在现有名字表唯一匹配时填写。
- `session_missing_from_playerlist` 在补全前保留原始缺口证据；MySquad 在最终去重后再标记。前端也在输出 keyed rows 前过滤重复 key，`live_roster_duplicate_dropped` 每个 game/phase/count 仅报告一次，不传身份。

测试：18+5 残留→18 人、17+5/playerlist18→18 人、自己一条且不带旧英雄、正常 5v5/跨队重复保留 teamOne；位置和诊断既有测试通过。旧 Arena 9+9 夹具改为本局 18 人全部在 teamOne；原 17 人重试夹具现验证补成18人，仍不猜小队分组。

## P3 增量渲染

- squad/data/roster 提示条放在固定 `data-live-notice` 节点，原地更新。
- 页签文字、属性、数量按 key 同步，保持已有按钮与焦点；panel 的 loading/hidden 属性及数量原地同步。外壳比较不再把这些状态当成整块重建条件。
- `patchLiveRosterPanel` 按行 key 复用、重排、插入/删除行，跨列表也能复用图片；不会因行数或顺序不同退回替换全部行。
- `other` 的可确定代码原因是 panel 的 `is-loading` / `hidden` 属性进入 chrome 比较。本次原地更新这些属性。原日志只有 `other` 枚举，没有外壳节点证据，不能将这个代码缺口宣称为当时那一条日志的唯一原因。今后记录 `shell_node`（第一个不同节点的 tag.class，120 字符白名单，不含文本/身份）。
- 每个 render 调用标记自己的 source；只有 load 自己使用该请求的 source，其他调用使用 progress/catalog/recommendation/rune/selection/settings/navigation/timer/sse 等。status 也在本次 source 下计数，不沿用前一次加载来源。未发现另一个每秒整块渲染循环；已有每秒游戏时长更新只改 time 文本。
- 新计数（banner/tabs）、触发来源、shell_node、去重事件加入 runtime 和 `features.go` 白名单。

**离线 Chromium：** 峡谷、海斗、极地、斗魂、练习、URF 六份合成快照，各实际观察超过185秒；body/玩家行/图片对象始终保留。首轮进入只记录一次 full，提示出现/消失仅记 banner，idle 无额外重建。Arena 19份输入（重复自己）渲染18行，并保持本队前三。原始文件：`history/reports/r224/{rift,hextech,aram,arena,practice,urf}-chromium.json`；汇总：[idle-summary.json](history/reports/r224/idle-summary.json)。

首轮完成六个模式观察后，补总览截图时触发 Chrome 同源六条 SSE 连接上限，导航超时，故 `chromium-r224.json` 的整脚本 exit_code 为1。脚本改为关闭已完成观察的标签页后补图；短时视觉补跑 exit_code=0、无页面异常，记录在 `visual/`。三分钟原始观察文件保留，未拿短时补跑冒充三分钟验收。

**Windows 未验：** 没有真实各模式三分钟 `live_render_rebuild/renderer_perf`，也没有一段真实对局录像。离线的 renderer_perf 无长任务时按现有监控规则不输出，不伪造零记录。召唤师峡谷、2400、450、1700/1750（尤其5v5接斗魂）、练习及当天其他轮换的实际观测/一分钟录像仍待验。

## P4/P5 标签和排序

- 统一 `--identity-chip-gap:6px`；live、总览 name meta/level、当前对局名字、玩家浮层、玩家页签使用该 token，紧凑宽度也一致。self/premade/region 标签去除独立左边距，其他标签继续无左边距。没有新增统计口径文字。
- Arena 未分组：自己→MySquad队友→其他人；小队内按用户排序，其他人保留现有 premade 聚合。18人所有 team/win-rate/KDA/position 排序均覆盖；相同数据重复 render 顺序稳定。
- 客户端小队分组：本人组第一，组内本人第一；ChampSelect 同样本人第一。
- Chromium 截图（合成数据、图标使用离线 SVG）：[5v5](history/reports/r224/visual/rift-chips.png)、[斗魂](history/reports/r224/visual/arena-squad-first.png)、[总览](history/reports/r224/visual/overview-chips.png)、[浮层](history/reports/r224/visual/player-overlay-chips.png)。5v5包括“自己+组队+职业”和“隐藏战绩+补位”。已亲自查看5v5、斗魂及浮层截图，间距一致。

## 验证与构建

最终运行记录如下（北京时间）。完整输出、运行时间与退出码位于 [r224 证据目录](history/reports/r224/)；Go 全量必须对应最后一次 Go/测试改动。

- 前端全量：950 tests，950 pass / 0 fail / 0 skipped；`node --test --test-concurrency=3 backend/web/*.test.cjs`。初次默认并发在负载下出现既有100ms护栏/页面挂载超时，调低并发后完整通过；没有放宽这些时间断言。
- JS syntax：13 个生产 JS 文件，0 errors。
- Chromium R100：peakImages=5；R117：unstyledFrames=0、styleBeforeModule正确、双列几何正确。
- public 后端：本机和 Windows amd64 均重新构建，版本0.12.73、指纹/无内嵌Key策略通过，本机 self-test 通过。**Windows exe只交叉编译，未执行。** 不生成安装包，不推 tag；产物只留带 public 后缀的可识别文件。

### 最终检查记录

| 检查 | 开始→结束（北京时间） | 耗时 | 结果 |
|---|---|---:|---|
| `go test -count=1 ./backend` | 21:25:59→21:28:12 | 133.084s | 通过 |
| `go vet ./backend` | 21:26:08→21:26:10 | 1.405s | 通过 |
| 前端全量（并发3） | 21:16:54→21:20:04 | 190.064s | 通过 |
| Chromium R100 | 21:20:28→21:20:31 | 2.542s | 通过 |
| Chromium R117 | 21:20:37→21:20:45 | 8.157s | 通过 |
| Chromium 视觉补跑 | 21:25:19→21:25:27 | 7.877s | 通过 |
| public 后端构建/自检/指纹 | 21:26:19→21:26:29 | 10.597s | 通过 |

Go 全量最后5行（输出只有1行）：

```text
ok  	lol-loot-assistant/backend	131.517s
```

Go vet 输出为空、退出码0。已核对没有 Go 源码/测试文件的修改时间晚于最终全量开始时刻。

最终版本 **0.12.73**，key mode **public**，源码指纹 **1f1aa659b7e7**。

- `dist/r224/loot-service-0.12.73-public`：SHA256 `aea253fb61f4c5a1d340bc595094bdce53572618d5a29ccdcc156b8e5873b9c9`。
- `dist/r224/loot-service-0.12.73-public.exe`：SHA256 `17a7e65ea936ecbb8b12a11199bc802df3f1599e83edf811fa6166e82bab97f8`。

## 未完成验收清单

- [ ] Windows 实测勇敢举动立即锁定/亮出后锁定，记录实际成功请求与确认日志。
- [ ] 普通英雄预选失败后仍自动锁定、禁用照常的真机证据。
- [ ] 先5v5再斗魂，18人/唯一自己/正确英雄/三个MySquad/本队前三的真实日志和截图。
- [ ] 各真实模式三分钟重建与性能原始记录、一分钟录像；P3零变动阈值达标。
- [ ] 真机各页面标签间距核对。

已在本聊天询问可连接的 Windows 测试机方式；当前未取得连接信息。该外部环境缺口不阻止本地修复与构建，但不能关闭本工单。

## R228草稿合并与后续验证（2026-10-05）

以上未提交/未发布描述为本单实施当时记录。修复现已纳入0.12.74 public草稿（tag434caa92，指纹a3d7c1e75735），tag完整质量37329122051与最终分支37334082296均success。R228最终预算115.797/219.765/67.949s达标；草稿附件验证通过。仍未正式发布Latest，等待R228 P6确认；Windows真实游戏客户端验收边界不变。详见 [R228账本](r228-execution-ledger.md)。
