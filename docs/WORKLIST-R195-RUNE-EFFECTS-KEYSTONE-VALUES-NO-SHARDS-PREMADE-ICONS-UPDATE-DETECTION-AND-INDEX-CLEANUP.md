# WORKLIST-R195：符文效果去掉属性碎片、补上致命节奏/神奇之鞋数值；组队弹窗改成英雄头像；在线升级的安装位置识别；工单索引清理

诊断人：Claude（看截图 + 读日志 + 只读核对源码 + 查 GitHub 发布页）。执行人：GPT。日期：2026-10-03。基线：0.12.58（R194 已执行，若工作区还没提交，先提交 R194）。完成后版本升到 0.12.59，**并按现有流程发布到 GitHub Release**（P3 的在线升级需要一个比 0.12.58 新的版本才能实测）。

## 证据范围

日志 `lol-loot-diagnostics-1003-0228.jsonl`。前半段是 0.12.49，**17:51:26Z 起是 0.12.58**（运行 `1ff07f3c…`，指纹 `5230b7850e83`），到 18:28Z 共打了 2 局海克斯大乱斗。0.12.58 是第一次在真机上运行 R185–R194 的代码。

0.12.58 真机已确认正常（不用改）：

- R189：`loot_metadata_source catalog=emotes entries=2373 source=client`；不再出现表情的 `loot_name_fallback`。只剩 R144 那条空白记录，重试 3 次后正常结算。
- R190/R191：`perk_stats_presence`（SGP 有变量的参与者 200/200）、`perk_effect_sample` 已采集到样本；`augment_descriptions_loaded ok=4/4`。
- R185：`renderer_perf`、`desktop_process_metrics`、`backend_runtime_metrics` 都有。长任务最多 688 ms/分钟，没有持续卡顿；后端内存 57–80 MB，goroutine 14；Electron 主进程约 100 MB。
- R186 P2：`game_settings_sync result=skipped_no_change`。两局里 PersistedSettings 没有变化，视角相关项没有变化，只有 `game.cfg SystemMouseSpeed` 在开局和结束时于 8 和 0 之间来回变（游戏自己写的，不影响操作）。
- 其他：hexdata 请求全部 200；对局详情、战绩新鲜度、推荐都正常。

## P1　构建页「符文效果」：去掉属性碎片；补上致命节奏、神奇之鞋的数值

### 1-1　去掉属性碎片行

用户截图：效果列表最下面的「属性碎片」三行（「获得10%攻击速度。」等）没有意义，还占高度。

`backend/web/gameplay.js` 约 3971 行 `renderRuneEffects` 末尾的 `shards-line` 整段删除，相关 CSS 一并删除。左侧符文树里的碎片图标不受影响（共享符文板不改）。

### 1-2　致命节奏（8008）显示「已造成的伤害」

现在 `PERK_EFFECT_OVERRIDES = { 8008: [], 8304: [] }`（约 3893 行），所以这两个基石什么数值都不显示，只显示描述文字（截图里的「精密 · 攻击一个敌方英雄会为你提供攻击…」）。

R191 加的 `perk_effect_sample` 这次采到了真实变量：

| 局时长 | 队列 | vars |
|---|---|---|
| 1929 秒 | 440 | `[520, 520, 0]` |
| 861 秒 | 480 | `[775, 775, 0]` |
| 1093 秒 | 480 | `[932, 972, 0]` |

Riot 的模板是 `最大攻速运转时间：@eogvar1@:@eogvar2@<br>已造成的伤害：@eogvar2@`：

- 第一行按"分:秒"套用会得到 `520:520`、`932:972`，秒数大于 59，**明显不是时间**，这一行是 Riot 写错的，丢弃。
- 第二行「已造成的伤害 = var2」：三局都是 `var2 ≥ var1`，数量级和局时长相符（几百到一千），可以采用。

修改：`PERK_EFFECT_OVERRIDES[8008] = ["已造成的伤害：@eogvar2@"]`。按现有标签规则生成「已造成的伤害」，`kind=damage`，计入头部伤害汇总。var1 的含义仍不确定，不显示。

### 1-3　神奇之鞋（8304）显示「鞋子到达时间」

样本两局都是 `[7, 3, 0]`。模板 `…：@eogvar1@:@eogvar2@@eogvar3@` 的意思是 **分 : 十位秒 个位秒**，即 `7:30`（12 分钟基础时间，减去击杀/助攻提前的时间，7:30 合理）。模板没写错，是我们的解析器不认识这种写法。

修改：

- `perkEffectLines` 增加一种值模板：`@eogvar1@:@eogvar2@@eogvar3@` → `${v1}:${v2}${v3}`。要求 v1 ≥ 0，v2、v3 都是 0–9 的整数，否则丢弃这一行。`kind=other`。
- 从 `PERK_EFFECT_OVERRIDES` 里删掉 8304，直接用客户端模板。标签按现有规则生成（zh_cn 模板里的原文，例如「鞋子到达时间」之类，以客户端为准）。

### 1-4　基石有数值时的显示

基石行（44px 图标、主题色底）有数值时：右侧显示数值（18px、`--primary-strong`），名字下面那行灰字只保留所属系名（「精密」），**不再显示描述文字**。没有数值时维持现状（系名 · 描述）。

### 测试（Node）

1. `renderRuneEffects` 输出里没有「属性碎片」，也没有 `.shards-line`。
2. 8008，vars `[932, 972, 0]` → 一行，label「已造成的伤害」，value 972，kind=damage；不出现「最大攻速运转时间」。
3. 8304，vars `[7, 3, 0]`、客户端模板 `X：@eogvar1@:@eogvar2@@eogvar3@` → value `7:30`。
4. 8304，vars `[7, 12, 0]`（v2 不是个位数）→ 不输出这一行。
5. 基石有数值：名字下面一行不含描述原文；没有数值时含描述原文。
6. 头部伤害汇总包含 8008 的 972。

变异：恢复 `8008: []` → 测试 2 FAIL；删掉新值模板 → 测试 3 FAIL；保留碎片行 → 测试 1 FAIL。

## P2　组队弹窗：英雄名字换成英雄头像，放在召唤师名字前面

现在（R188）每行是「名字（左）　英雄名（右，灰字）」。用户要求：英雄头像放在名字前面，不显示英雄名字。

1. `renderLivePremadeTag`（约 5720 行）的 roster 每项改为 `{ name, championIconURL }`：
   - `championIconURL`：`Number(member.championId) > 0 && !member.championPickPending` 时取 `proxyAsset(assetPath("champion", liveDisplayedChampionId(member, currentChampionId)))`，否则空串；
   - 不再输出 `champion` 文字。
2. `app.js` 约 4040 行：每行 `[头像 20×20 圆角 4px][名字]`，间距 8px。头像只接受 `/api/image?path=` 开头的地址，走现有图片队列（`data-queued-src`）。没有头像时放一个同尺寸的空占位（`--surface` 底色），名字仍然对齐。
3. CSS：`.tooltip-roster-player` 改为 `grid-template-columns: 20px minmax(0,1fr)`；删除 `.tooltip-roster-champion`。名字样式不变（12px、600、省略号）。
4. 标题「组队 N 人 / 预组队 N 人」不变，不加其他文字。

### 测试（Node）

7. 两人组队、都已选英雄：两个 `<img>`，地址都是 `/api/image?path=` 开头，名字在头像后面；弹窗里没有英雄名文字。
8. 一人 `championId=0`：这一行没有 `<img>`，有占位元素，名字仍在。
9. 隐藏名字设置下名字仍打码。

变异：恢复英雄名文字 → 测试 7 FAIL。

## P3　在线升级：安装位置识别和实测

### 现状

- 0.12.49 之前只能"前往发布页"。用户在 0.12.49 里看到 0.12.58 时（17:42 `update_check_succeeded state=available`），用的还是 0.12.49 的旧更新逻辑，所以没有一键升级。**这是预期的**：一键升级要从带 R186 代码的版本（0.12.51 起）出发才有。用户随后手动装了 0.12.58。
- 另外，0.12.49 在 12:12、14:34 两次检查到的最新版本都是 **0.12.19**。GitHub 发布页现在只有 v0.12.19 和 v0.12.58（10-02 17:38 发布，Latest），也就是说在 0.12.58 发布之前，在线更新一直只能看到 0.12.19。账本里说的"0.12.50 已正式发布 Latest"现在在发布页上找不到。请在账本里说明 0.12.50 是否被删除，以及以后发布流程的规则（每次发布都要保留为 Latest，不能删除）。
- **0.12.58 的启动诊断有问题**：`update_install_detection result=registry_location_mismatch registry_display_found=true location_matches=false`。注册表里有 Deep Legends，但位置和当前运行目录对不上。按现在的代码（`update.go` 232 行），这种情况会被当成便携版，升级时走"全新安装到 `%LOCALAPPDATA%\Programs\Deep Legends`"的路径。

### 风险

用户是用安装包装的 0.12.58，正常应该是 `installed`。可能的原因有：注册表 `InstallLocation` 带引号或结尾斜杠、8.3 短路径、大小写以外的路径差异，或者 `updateRootForExecutable` 算出的根目录不是安装目录。

如果当前运行目录**就是** `%LOCALAPPDATA%\Programs\Deep Legends`，便携路径会让 NSIS 往正在运行的目录里装。按 R186 账本，NSIS 检测到目标进程在运行时会中止，**升级会失败**。

### 修改

1. **诊断补全**（只记布尔值和计数，不记路径）：`update_install_detection` 增加
   - `registry_entries`：DisplayName 为 Deep Legends 的条目数；
   - `location_quoted`、`location_trailing_sep`、`location_exists`；
   - `root_is_default_dir`、`location_is_default_dir`（与 `%LOCALAPPDATA%\Programs\Deep Legends` 比较）；
   - `match_after_normalize`：按第 2 条规范化之后是否一致。
2. **路径比较规范化**：`updateInstallationMatches` 比较前对两边都做：去掉首尾引号和空白、`filepath.Clean`、Windows 上 `GetLongPathName`、`EvalSymlinks`（失败就用原值），然后不区分大小写比较。
3. **兜底**：规范化后仍不一致，但当前运行目录就是默认目录且目录里有卸载程序时，按 `installed` 处理（走原来的安装版升级路径）。
4. 测试（Go）：
   - 带引号；
   - 结尾斜杠；
   - 短路径（用注入的路径转换函数模拟）；
   - 运行目录为默认目录且有卸载程序 → `installed`；
   - 真正不同的两个目录 → 仍是 `registry_location_mismatch`；
   - 诊断里不出现路径字符串。
5. **实测**：本工单打包后发布 0.12.59 到 GitHub Release（Latest）。用户在 0.12.58 里检查更新，应看到"立即升级"→下载进度→"立即重启升级"。
   - 注意：0.12.58 → 0.12.59 这一次走的是 0.12.58 里的旧判断。如果因为上面的问题失败，界面会显示失败和发布页入口，用户手动装一次 0.12.59 即可，从 0.12.59 往后就会用修正后的判断。账本里要写明这一点。

## P4　工单索引清理

用户已确认，以下工单在 `docs/WORKLIST-INDEX.md` 改为**已关闭**（附关闭依据一句话）：

- R86–R132 中所有"进行中 / 未执行"的旧工单（含 R116）：已被后续工单覆盖，用户确认关闭。
- R179–R185、R187、R189、R192–R194：真机日志或用户确认正常。
- R190、R191：除本工单 P1 接手的部分外已正常，关闭并注明"剩余部分由 R195 接手"。
- R188：组队弹窗由 R195 P2 接手，对位条正常，关闭并注明。
- R186：P2（设置同步）真机正常；P1（在线升级）由 R195 P3 接手，关闭并注明。
- R142：GitHub 首次发布已完成（发布页现有 v0.12.19、v0.12.58），关闭。
- R144–R178 中，本次 0.12.58 日志里对应功能正常、且没有新问题的工单一并关闭：R144–R152、R154、R155、R157–R178。
- **保留进行中**：R153（skinName 真机复核未做）、R156（日志里没有出现过邀请，自动接受未验证）、R195。

索引顶部说明里加一句："关闭 = 真机日志或用户确认正常，或已被后续工单接手。"

## 收尾

- `node --test backend/web/*.test.cjs desktop/*.test.cjs`、`go test ./backend -count=1`、`go vet ./backend`、`git diff --check` 全绿。
- 版本 0.12.59，完整 public 构建；发布到 GitHub Release，设为 Latest，附 latest.json；`docs/r195-execution-ledger.md` 附符文效果和组队弹窗的改前/改后截图（演示数据）。
- 不新增除上文明确写出以外的界面文字。

## 真机验收（用户）

1. 打开 0.12.58，检查更新：应看到 0.12.59 的"立即升级"。点了以后看是否能下载、重启、升级完成；失败的话截图，再手动装 0.12.59。
2. 打开一场带致命节奏的对局 → 展开详情 → 构建：致命节奏显示「已造成的伤害」数值；没有「属性碎片」那一块。
3. 选人或对局中把鼠标移到「组队 ×N」上：每行是英雄头像 + 名字。
4. 导出日志：`update_install_detection` 应为 `installed`；如果仍是 mismatch，把日志发给我，看新增的那几个布尔值。
