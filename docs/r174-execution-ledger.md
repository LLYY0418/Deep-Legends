# R174 执行账本

日期：2026-09-30。基线 0.12.38；本轮源码版本 **0.12.39**。

## 实现范围

仅调整 backend/web/gameplay.js / gameplay.css 的绝活哥、职业选手符文对局记录展示位置；后端、数据请求和应用逻辑未改。

- 对局行移除独立 renderRuneSpellPair，由 renderRuneEquipment 在装备行最后输出 specialist-game-spells。容器中只有两个 renderSummonerSpellIcon 图标，无新增标签、技能名小字、说明或 tooltip，保留图标本身已有属性。
- 两个技能 ID 均为 1–100000 的整数且不相等时才显示。装备存在时为标签 → 装备组 → 技能；无终局装备但有技能时只输出装备行外层与技能容器，没有孤立标签、空装备组或出门装组。两者都没有则仍返回空字符串。
- 无合法技能时原最终装备 / 出门装行输出逐字一致。R173 的标签、出门装 6 格、终局 7 格、route-divider 不变。
- 装备组的直系 div 样式排除技能容器；技能 flex:0 0 auto、nowrap、margin-left:auto，装备组仍可收缩并换行。图标共用原装备行尺寸规则，无新颜色变量。
- patchRuneStarterEquipment 的整行替换函数未改，仍使用同一 config 的技能字段，补装后技能随新节点保留。
- 与本轮开始时源码逐函数比对：recommendedRuneSpellIDs、renderRuneSpellPair、renderRuneChoice、renderBuildRecommendation、patchRuneStarterEquipment、ensureRuneStarterItems **逐字一致**。OPGG 卡片级技能行、renderRuneChoice 技能行、R171 勾选 / 应用 / 诊断、R172 核心装路线和 R173 取数 / 缓存 / 启动逻辑保持原样。

## 断言与变异

更新原 R75 / R171 行测试，要求技能存在于 specialist-game-items 的末尾容器中，不再依赖独立 rune-spell-row。原 OPGG 卡片恰好一个技能块的测试继续通过。

新增 7 个 Node 测试，覆盖：

1. 装备标签 / 装备组 / 技能直系 DOM 顺序，技能容器末尾、恰好两个图标，无新增文字。
2. 有出门装时严格次序：出门装 → 竖线 → 终局装备 → 技能。
3. 缺失、0、相同、负数、小数、无穷大、越界技能 ID 的逐字兼容输出。
4. 只有技能时只有一个容器，无孤立标签与空装备组。
5. 绝活哥 / 职业选手整行模板均没有独立技能行，也没有重复技能图标。
6. 异步补出门装前后保留技能组合、末尾位置、对局行节点 / 顺序 / 选中状态。
7. 独立 renderRuneSpellPair 原输出逐字一致；CSS 只允许装备组换行、技能不缩小且靠右。

使用 /private/tmp/r174-mutations 的 Node 文件读取 overlay（preload 只替换 gameplay.js 的读取内容），仓库生产源码未被变异覆盖。以下五项全部以实际断言 FAIL 被拦截，均非 SyntaxError / ReferenceError：

| 变异 | 捕获断言 |
|---|---|
| 技能容器放到装备前面 | 直系顺序与最后一个子节点断言 |
| 独立 renderRuneSpellPair 放回对局模板 | 整行 rune-spell-row 数量必须为 0 |
| 无终局装备直接返回空 | 只有技能时必须存在装备行 |
| 给装备行技能增加“召唤师技能”文字 | 恰好两图标 / 无标签断言 |
| 补出门装时丢弃技能字段 | 补充前后技能组合一致 |

全量 node --test backend/web/*.test.cjs：**752 / 752 通过**；go test ./backend -count=1：**通过，196.289 秒**；go vet ./backend、git diff --check 均通过。

## 真实浏览器夹具视觉验证

新增 desktop/r174-rune-equipment-layout.cjs，使用现有 headless Chromium / CDP 方式，临时独立浏览器配置与本地回环服务。加载生产 app.css / gameplay.css，复用生产 renderSpecialistPlayers、renderRuneEquipment、renderItemIcon、renderSummonerSpellIcon、proRuneRecordLabel；符文板和图标图片使用本地占位夹具，未调用赛事 / Riot / OPGG 上游。

视口 **1440 / 1100 / 820px**（夹具主栏分别为约 900 / 680 / 400px），深浅主题各一轮，共 6 张截图。几何断言确认：

- 技能容器始终在装备行末尾，不换行、不缩小，距右端内边距 10px，未与装备组相交。
- 高密度装备组随宽度缩窄高度为 34 / 68 / 102px；每种宽度装备行横向溢出均为 0，技能仍在行右侧。
- 只有技能的行在全部宽度 / 主题均为 **48px**，与不换行的单件装备对照行相同，保留原 min-height:44px 加现有内边距的实际行高。
- 已人工查看中宽深色、窄宽深色、宽屏浅色截图，技能靠右且窄宽仅装备组换行。

截图和测量记录在 docs/r174-validation。它们是 macOS Chromium **夹具验证，不是 Windows 真机截图**；Windows 实际图标 / 字体和客户端选人仍待用户验收。

## 构建

**key mode: public**，main.riotAPIKey / main.riotAPIKeyCipher 均清空。版本 0.12.39，源码指纹 **4d2e848cbe8c**。

- macOS arm64：/private/tmp/Deep-Legends-backend-0.12.39-public
- Windows amd64：/private/tmp/Deep-Legends-backend-0.12.39-public.exe

两平台 go build 均通过；两份后端均通过指纹校验，重新计算源码指纹仍为 4d2e848cbe8c。macOS 使用 LOL_LOOT_DATA_DIR=/private/tmp/r174-selftest 和空 RIOT_API_KEY 执行 -self-test，通过并输出版本 0.12.39、奖池 554 条。未制作或发布安装包。

## 仍待用户真机验收

Windows 选人打开绝活哥，确认每条底部技能位于装备最右侧，缩窄窗口装备先换行。职业来源当前无技能时行保持 0.12.38 的输出，仍等 R173 原始字段诊断后决定技能解析；OPGG 顶部技能块保持原样。本轮不替代 R173 的开局偏移或原始字段确认。
