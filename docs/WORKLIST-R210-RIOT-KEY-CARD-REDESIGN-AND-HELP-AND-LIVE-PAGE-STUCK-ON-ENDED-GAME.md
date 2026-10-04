# WORKLIST-R210：Riot API Key 移进设置卡片、重做样式并补申请说明；对局结束后对局页一直停在上一局

诊断人：Claude（看截图 + 读日志 + 只读核对源码）。执行人：GPT。日期：2026-10-04。基线：0.12.71 工作区（R206～R209 之后）。与 R206～R209 合并发布。

---

## P1　Riot API Key：放进「隐私遮罩 / 英雄数据网络」那张卡片，重做输入框和按钮，加说明

### 现状（用户截图，设置 → 隐私与能力）

- 「隐私遮罩」「英雄数据网络」在一张圆角卡片里。
- 「Riot API Key」单独一行浮在卡片外面：左边标题加"未配置"；中间是一条很长的灰色实心输入框，几乎占满整行；右边是一个浅灰实底的眼睛按钮，加上「保存」「清除」两个文字按钮。
- 用户反馈：输入框和眼睛按钮太丑；希望这一项放进上面的卡片里，排在「英雄数据网络」下面。

### 修改

1. **位置**：`backend/web/index.html` 把 `setting-riot-key-card` 移到 `setting-network-card` 之后，作为同一张卡片里的第三行。行与行之间的分隔线与上面两行一致；删掉 `.setting-riot-key-card { margin-top: 16px; }` 这类单独浮出的间距。

2. **布局**：与「英雄数据网络」同一结构。
   - 左列：标题「Riot API Key」，下面是说明文字（第 3 条），再下面是状态（沿用 `#setting-riot-key-state`）。
   - 右列：输入框 + 眼睛按钮 + 保存 + 清除，**右对齐**，与上一行的「自动检测 / 保存」对齐。
   - 输入框宽度固定在 **280～320px**，不再拉满整行。窄窗口时右列换到下一行左对齐（复用 `.setting-network-controls` 的现有响应式规则）。

3. **说明文字**：左列，用卡片里已有的 `<p>` 样式。两段，第二段带链接。
   - 第一段：
     > 未填写时使用软件内置的查询服务，所有用户共用一份官方接口额度，高峰时段韩服战绩、绝活哥和职业选手数据可能加载变慢或暂时不可用。填写自己的 Key 后直接连接 Riot 官方接口，不受共用额度影响。
   - 第二段：
     > 申请：用拳头（Riot）账号登录 [Riot 开发者平台 ↗](https://developer.riotgames.com/)，首页即可生成开发 Key（24 小时后失效，需重新生成后再保存）；长期使用可在平台点「Register Product」申请个人 Key。
   - 链接用 `<a href="https://developer.riotgames.com/" target="_blank" rel="noopener noreferrer">`。desktop 的 `setWindowOpenHandler` 已经会用系统浏览器打开 https 链接，不需要改。
   - 链接颜色用主题现有的强调色，带下划线；悬停 / 键盘焦点有可见状态。
   - 这是用户明确要求的说明，与"不加说明文字"的旧规则不冲突。只加这两段，其他地方不加。

4. **输入框样式**：与同一卡片里的下拉框（`#setting-proxy-mode`）一致。
   - 背景、边框、圆角、高度、字号、内边距都与下拉框相同；不再用灰色实心块。
   - 占位文字："粘贴 RGAPI- 开头的 Key"。
   - 获得焦点时的边框 / 光环与下拉框一致。
   - 已保存时输入框保持为空。状态由左列的状态文字表示（沿用 R204 / R206：已配置 / 未配置 / 无效 / 使用内置服务）。

5. **眼睛按钮**：
   - 去掉浅灰实底，改成**放在输入框内部右侧**的图标按钮：透明背景，图标颜色与次要文字一致，悬停时变成主文字颜色。
   - 按下状态时图标换成"睁眼 / 闭眼"两种，沿用 `aria-pressed`；按钮尺寸至少 32px，便于点击。
   - 输入框右侧留出按钮的位置，文字不会被遮住。

6. **保存 / 清除**：沿用卡片里「保存」的文字按钮样式，与「英雄数据网络」那一行的「保存」完全一致。

### 测试（Node）

1. 「Riot API Key」所在的 `section` 位于包含 `setting-network-card` 的同一个卡片容器内，并且紧跟在它后面。
2. 说明文字包含"共用一份官方接口额度"和链接 `https://developer.riotgames.com/`；链接有 `target="_blank"` 和 `rel="noopener noreferrer"`。
3. 眼睛按钮在输入框的包裹元素内部；点击后切换 `aria-pressed` 和输入框类型（原有 R204 测试保持通过）。
4. 输入框、下拉框的高度、圆角、边框颜色使用同一组 CSS 变量（检查计算后的样式或共享的 class）。
5. R204 / R206 / R208 现有 Key 相关测试全部通过。

账本附设置页截图（演示数据，不含真实 Key）。

---

## P2　对局早已结束，对局页仍停在结束的那局

### 证据（日志 `lol-loot-diagnostics-1004-1911.jsonl`，0.12.68）

| 时间 (UTC) | 事件 |
|---|---|
| 10:35:09～12 | 对局 `9014822625`（queue 2400）从 Reconnect 进入 WaitingForStats |
| 10:35:15～16 | PreEndOfGame → EndOfGame → **Lobby** |
| 10:35:22～23 | 阶段变为 **None**；`/api/live` 返回的仍是 `game_id=9014822625`、10 名玩家，`live_roster_rendered phase=None players_received=10` |
| 10:35:27.98 | `live_roster_post_game_reveal reason=ok revealed=1`（R197 赛后补全：1 名隐藏玩家被补出） |
| 10:35:38 → 11:11:53 | 每 12 秒一次轮询，阶段一直是 None，`game_id_comparison=unavailable`、`invalidated=false`；对局页一直渲染那局的 10 个人，持续 **36 分钟以上** |

### 根因（源码已核对，`backend/live_post_game_reveal.go`、`gameplay_refresh.go`）

1. R197 的赛后补全：对局结束时，如果名单里有隐藏玩家，就把当局快照存进 `a.postGameReveal`，补全后继续保留。
2. `cachedGameplayLive` 第一行就是 `if snapshot, ok := a.postGameSnapshot(client, phase, 0); ok { return snapshot }`。`postGameSnapshot` 只在 `ChampSelect / GameStart / InProgress / Reconnect` 时拒绝；**其他任何阶段（Lobby、None、Matchmaking、ReadyCheck……）都会返回保留的旧快照**，只是把 `Phase` 改成当前阶段。
3. 保留的快照只在两种情况下清掉：进入上面四个阶段，或读到另一个 gameId。补全流程结束后（`runPostGameReveal` 的 `finish` 只把 `running` 设为 false），`client` 和 `snapshot` 仍然保留，**没有时间上限，也没有"离开结算阶段"的清理**。
4. 所以只要上一局有隐藏玩家，直到下一次进入选人之前，对局页都会显示那一局。没有隐藏玩家的对局不会进入这个流程，所以不是每局都出现。
5. 前端（`gameplay.js`）的失效判断也只在"进入对局类阶段"时清空（`phaseBoundary` 要求 `liveGamePhase(nextPhase)`）。离开 EndOfGame 回到 Lobby / None 时，不会主动清空对局页；所以后端返回什么，就显示什么。

用户在 R201 时说过："打完就清空这把信息了，打完才展示没有意义，就这样吧"。那时的预期是"对局结束后页面会清空"，现在恰恰相反：页面被这个保留快照卡住，一直不清空。

### 修改

1. **后端**：保留快照只在结算阶段有效。
   - `postGameSnapshot` 只在 `WaitingForStats / PreEndOfGame / EndOfGame` 时返回快照；其他阶段一律返回 false。
   - 阶段为 `Lobby / None / Matchmaking / ReadyCheck` 等非结算、非对局阶段时，调用 `stopPostGameReveal()` 清空。
   - `observePostGameReveal` 中：只要阶段不是结算阶段，就停止并清空（不只是上面四个对局阶段）。
   - 补全流程结束后（无论成功、失败还是重试完毕），保留时间最多到**结束后 2 分钟**；超时自动清空。可以用现有的 `now` 注入实现，便于测试。
2. **前端**：阶段从结算阶段变为 `Lobby / None` 等非对局阶段时，对局页切回"等待进入对局"的空状态，不再保留上一局的名单。R207 的推荐缓存按现有 `resetLiveGameScopedState` 规则处理；本条只影响名单和页面状态，不影响下一局。
3. **诊断**：`live_roster_post_game_reveal` 增加 `reason=expired`（超时清空）和 `reason=left_end_of_game`（离开结算阶段清空）；前端的"清空对局页"复用 R207 的 `live_scope_reset`，增加 `reason=left_end_of_game`。

### 测试

Go：

1. 有隐藏玩家的对局 → EndOfGame 时返回保留快照；进入 Lobby / None → 立刻不再返回，`stopPostGameReveal` 被调用，诊断 `left_end_of_game`。
2. 一直停在 EndOfGame → 2 分钟后不再返回快照，诊断 `expired`。
3. EndOfGame → ChampSelect（下一局）→ 不返回旧快照（原有行为）。
4. R197 原有补全测试（EndOfGame 内补全成功）保持通过。

Node（jsdom，按日志顺序回放）：

1. 依次喂入以下快照，之后对局页不再显示上一局的 10 名玩家：
   - Reconnect（game 9014822625）；
   - WaitingForStats；
   - EndOfGame；
   - Lobby；
   - None（后端仍返回旧 game_id）。
2. 下一局 ChampSelect 正常显示新名单。

变异（必须 FAIL）：

| 变异 | 失败的测试 |
|---|---|
| `postGameSnapshot` 恢复为"非对局阶段都返回" | Go 测试 1 |
| 去掉 2 分钟上限 | Go 测试 2 |
| 前端离开结算阶段不清空 | Node 测试 1 |

---

## 收尾

- `node --test backend/web/*.test.cjs desktop/*.test.cjs`、`go test ./backend -count=1`、`go vet ./backend`、`git diff --check` 全绿。
- `docs/WORKLIST-INDEX.md` 加 R210，R197 那行备注"赛后保留快照的清理由 R210 P2 修正"；账本 `docs/r210-execution-ledger.md`。
- 版本沿用合并版本；构建、发布等用户恢复后与 R206～R209 一起，按 R199 规则核对。

## 真机验收（用户）

1. 设置 → 隐私与能力：「Riot API Key」在「英雄数据网络」下面，同一张卡片里；输入框和下拉框风格一致，眼睛图标在输入框里；说明里的链接点开是 Riot 开发者平台。
2. 打一局有隐藏玩家的对局：结束回到大厅后，对局页回到"等待进入对局"，不再停在上一局。
3. 导出日志：能看到 `live_roster_post_game_reveal reason=left_end_of_game` 或 `expired`。
