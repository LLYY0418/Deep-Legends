# R116-F P3 探测结论：海斗平衡参数（伤害加成 / 承伤修正）——**明确放弃**

探测日期：2026-09-20（主控会话首轮）＋ **2026-09-21（本轮执行方复跑并扩大）**
工单原文：`docs/history/worklists/R116-F-概率预测器负向推荐与平衡参数探测-工单.md` 第 P3 节
执行账本：`docs/r116f-execution-ledger.md`

> **结论（工单要求二选一，不留待定）：探测不通过，明确放弃。**
> 已授权与可低成本探测的来源里**不存在**「英雄级伤害输出 +X% / 承受伤害 −Y%」这类海斗平衡乘数；
> 唯一未被网络探测排除的路径（LCU 客户端本地缓存）判定为**即使可行也不采纳**，理由见 §5。
> 本文档不留任何「待进一步调研」的开放式结尾：§6 写清了什么情况下才值得重开这张工单。

---

## 1. 一句话结论与三条判定理由

**CommunityDragon 确实携带 ARAM 专属数据，但不是我们要的那一种；DDragon 完全没有；hexdata（唯一已授权的海斗数据源）也没有。三条理由：**

1. **CommunityDragon 的 ARAM 维度是「逐技能覆盖」，不是「英雄级平衡乘数」。**
   它携带的是 `SpellDataValue` 的 ARAM 覆盖（每个英雄文件里稀疏出现）与 ARAM 起始装推荐覆盖，
   不携带任何「伤害输出 / 承受伤害」乘数表（§3 的关键词扫描：`DamageDealt` 命中 0、`Modifier` 命中 0）。
2. **口径不匹配，即使找到也不能用。**
   本项目海斗数据源是 hexdata 的 `aram_mayhem_raw`，`sampleScope.queueId = 2400`、
   `platform = CQ100+GZ100+HN1+HN10+NJ100+TJ100+TJ101`（**国服七区**）。
   CommunityDragon 的 ARAM 覆盖是**全球服经典 ARAM（mapID 12）**口径。把全球 ARAM 的数值当作
   国服海斗的数值展示，正是项目红线禁止的「用推断值代替真实值」。
   本轮新增的 `maps.json` 探测把这一条钉得更死：**全球客户端数据的地图表里根本没有海斗这张地图**（只有 0/11/12/22/30/453）。
3. **hexdata 侧没有平衡字段。**
   `/api/hexdata/meta` 的 **78 个**顶层键（本轮实测计数）逐一扫过，`balance` / `multiplier` /
   `damageDealt` / `damageTaken` / `modifier` 命中全部为 **0**，`dataHealthWarnings` 是空数组。
   已授权的数据源里没有这项数据。

---

## 2. 本轮探测方法（可复现）

所有请求都是本轮真实执行的，命令与状态码原样记录，没有凭印象填写：

```bash
# 状态码 + 字节数
curl -s -o /dev/null -w "%{http_code} %{size_download}\n" --max-time 25 "<url>"
# 目录索引 / 文件下载
curl -s --max-time 30 "<url>" -o <file>
```

CommunityDragon 的 `raw.communitydragon.org` **允许目录枚举**（返回 HTML 索引），所以本轮不是
「猜几个文件名试 404」，而是**把相关目录整个列出来再逐个排除**——这比工单原文那批点状探测强得多，
也是本轮能把结论从「没找到」升级成「已穷举，确实没有」的原因。

**Anti-scope 第 3 条（硬约束）执行情况：本轮没有向 RESG、ARAMKit 发出任何请求，连域名解析都没有。**
排除理由：无明确引用许可条款、无公开 API。下文不再列它们的路径，因为一条都没探测过。

---

## 3. 已排除路径清单（全部真实状态码）

### 3.1 CommunityDragon —— `rcp-be-lol-game-data` 插件目录（可枚举，已穷举）

| 路径（`https://raw.communitydragon.org/` 之后） | HTTP | 字节 | 结论 |
|---|---|---|---|
| `latest/plugins/rcp-be-lol-game-data/global/default/v1/`（**目录索引**） | 200 | 26,571 | **本轮新增**：可枚举，**94 个 `.json` 全部列出**，无任何 balance 类文件；唯一的 KIWI 相关条目是下面那个空文件 |
| `latest/plugins/.../v1/champion-summary.json` | 200 | 55,212 | 全文 `aram` / `balance` / `Multiplier` / `howling` **零命中**（本轮复扫确认） |
| `latest/plugins/.../v1/champions/157.json` | 200 | 69,866 | 唯一 `aram` 命中是英雄背景故事里的英文单词 "p**aram**ilitary"；`Modifier` 4 处全是技能说明占位符 `@SpellModifierDescriptionAppend@`，与平衡无关 |
| `latest/plugins/.../v1/perks.json` | 200 | 99,870 | `aram` / `balance` 零命中 |
| `latest/plugins/.../v1/items.json` | 200 | 683,297 | 装备表，无英雄平衡字段 |
| `latest/plugins/.../v1/aram-champion-balances.json` | **404** | 162 | 不存在 |
| `latest/plugins/.../v1/champions-summary.json` | **404** | 162 | 不存在（正确名是 `champion-summary.json`，无 s） |
| `latest/plugins/.../v1/aram-champions.json` | **404** | 162 | **本轮新增**：不存在 |
| `latest/plugins/.../v1/gamemodes.json` | **404** | 162 | **本轮新增**：不存在 |
| `16.18/plugins/.../v1/aram-champion-balances.json` | **404** | 162 | **本轮新增**：钉死版本目录（不只是 `latest`）同样没有 |

### 3.2 CommunityDragon —— 模式 / 队列 / 地图元数据（本轮新增，最有价值的一批）

| 路径 | HTTP | 字节 | 实测内容与结论 |
|---|---|---|---|
| `latest/plugins/.../v1/queues.json` | 200 | 354,880 | 420 条队列。**确实含海斗**：`id 2400 / 2401 / 2403 / 2405 / 2410 / 2450 / 3240 / 3270` 全部名为 `ARAM: Mayhem`，`2300` 名为 `Brawl`。但每条只有 **14 个字段**（`id name shortName description detailedDescription gameSelectModeGroup gameSelectCategory gameSelectPriority isSkillTreeQueue isLimitedTimeQueue isBotHonoringAllowed hidePlayerPosition viableChampionRoster pickMode`），balance / damage / multiplier / modifier 类字段 **0 个** |
| `latest/plugins/.../v1/maps.json` | 200 | 1,414 | 只有 map `0 Common` / `11 Summoner's Rift` / `12 Random Map(HA)` / `22 TFT` / `30 Arena` / `453 Classic Rift`。**没有海斗（KIWI）这张地图** → 全球客户端数据里海斗没有独立的地图级配置载体 |
| `latest/plugins/.../v1/game-mode-mutators.json` | 200 | 410 | `mapId 12` 下只有 4 条**地图皮肤改名**（Koeshin's Crossing / Butcher's Bridge / Bridge of Progress / SR?），无任何数值 |
| `latest/plugins/.../v1/kiwi-hub.json` | 200 | **2** | **关键发现**：`KIWI` 就是海斗的内部模式名，这个文件**存在但内容是 `[]`**——全球客户端数据里一条都没有。海斗是国服（腾讯）运营的玩法，全球包里只剩一个空壳表 |
| `latest/plugins/.../v1/augment-lists.json` | 200 | 22,353 | 3 个 `modeName`：`CHERRY`(44 项) / **`KIWI`(223 项)** / **`KIWI_JADE`(188 项)**。字段只有 `augmentList`（资产路径清单）+ `modeName`，`balance` / `damage` / `multiplier` / `modifier` 命中全为 **0**。→ 全球包里**知道海斗的海克斯池有哪些**，但**不知道任何数值**。（池归属本身对本项目也没有用途：海克斯池与图标已由 hexdata + CommunityDragon 目录覆盖，本工单不扩展范围） |

### 3.3 CommunityDragon —— `game/data` 二进制导出树（可枚举，已穷举）

| 路径 | HTTP | 字节 | 结论 |
|---|---|---|---|
| `latest/game/data/`（目录索引） | 200 | 8,771 | **本轮新增**：11 个子目录（`cfg/ characters/ images/ loadouts/ maps/ menu/ particles/ shaders/ shared/ spells/ tftteamplanner/`），balance / aram / kiwi / mayhem / mutator / modifier 命中 **0** |
| `latest/game/data/characters/`（目录索引） | 200 | 316,436 | **本轮新增**：全部英雄目录已列出，`aram` / `kiwi` / `mayhem` 命中 **0**——不存在模式级的角色配置目录 |
| `latest/game/data/cfg/` → `cfg/defaults/` | 200 | 7,358 / 7,413 | **本轮新增**：`cfg` 树下只有一个 `settingstopersist.json`（客户端设置项），无任何模式配置 |
| `latest/game/data/maps/` | 200 | 7,513 | **本轮新增**：只有 `mapgeometry/`、`shipping/`，**没有 map12 的平衡配置** |
| `latest/game/data/shared/` | 200 | 7,796 | **本轮新增**：只有 `junglebuffs/ particles/ quests/ spells/` |
| `latest/game/data/maps/map12/map12.bin.json` | **404** | 162 | **本轮新增**：不存在 |
| `latest/game/data/queues.json` | **404** | 162 | **本轮新增**：这个位置不存在（真实位置是 §3.2 的插件目录） |
| `latest/game/data/characters/aram/aram.bin.json` | **404** | 162 | 不存在 |
| `latest/game/data/characters/arammayhem/arammayhem.bin.json` | **404** | 162 | **本轮新增**：不存在 |
| `latest/game/data/characters/kiwi/kiwi.bin.json` | **404** | 162 | **本轮新增**：不存在（KIWI 没有角色级配置文件） |
| `latest/game/data/characters/yasuo/yasuo.bin.json` | 200 | 118,649 | 见 §4：**含 ARAM 逐技能覆盖，但不含英雄级平衡乘数** |
| `16.18/game/data/characters/yasuo/yasuo.bin.json` | 200 | 118,649 | **本轮新增**：钉死版本目录，与 `latest` 同字节数、同样只有那 2 处 ARAM |
| `latest/game/data/characters/teemo/teemo.bin.json`（对照样本） | 200 | 58,010 | `"ARAM"` 同样仅 2 处；`DamageDealt` 命中 **0**、`Modifier` 命中 **0** |
| `latest/game/data/characters/yasuo/yasuo.bin.hash` | **404** | 162 | 不存在 |

### 3.4 DDragon（Riot 官方，本轮新增）

| 路径（`https://ddragon.leagueoflegends.com/` 之后） | HTTP | 字节 | 结论 |
|---|---|---|---|
| `api/versions.json` | 200 | — | 最新版本 **16.18.1**，与 hexdata `meta.assetVersion` 一致（可交叉核对版本，但与平衡参数无关） |
| `cdn/16.18.1/data/en_US/champion/Yasuo.json` | 200 | 16,788 | `data.Yasuo` 下 **17 个字段**（`allytips blurb enemytips id image info key lore name partype passive recommended skins spells stats tags title`）；`aram` / `balance` / `multiplier` / `howling` / `mayhem` / `kiwi` / `damageDealt` 命中全 **0**；`modifier` 4 处全是技能说明占位符 `@SpellModifierDescriptionAppend@` |
| `cdn/16.18.1/data/zh_CN/champion/Yasuo.json` | 200 | 17,505 | 同上；中文关键词 `海斗` / `平衡` / `伤害加成` 命中 **0** |
| `cdn/16.18.1/data/en_US/aram.json` | **403** | 111 | 不存在（DDragon 对缺失路径返 403 而不是 404） |
| `cdn/16.18.1/data/en_US/aramBalance.json` | **403** | 111 | 不存在 |
| `cdn/16.18.1/data/en_us/gameMode.json` | **403** | 111 | 不存在 |

### 3.5 hexdata（本项目唯一已授权的海斗数据源）

| 检查项 | 实测 | 结论 |
|---|---|---|
| `/api/hexdata/meta` 顶层键数 | **78**（本轮实测计数；主控 2026-09-20 的笔记写 76，差异不影响结论） | — |
| `balance` / `multiplier` / `damageDealt` / `damageTaken` / `modifier` / `aramBalance` 关键词 | 命中全部 **0** | meta 里没有任何平衡参数入口 |
| `dataHealthWarnings` | `[]` | 上游自己也没有声明「有平衡数据但被降级」 |
| `source` | `{ "iesPublicRaw": "ies_public_raw", "aramMayhemRaw": "aram_mayhem_raw" }` | 海斗原始数据源的名字里没有平衡维度 |
| `sampleScope` | `queueId 2400`、`platform CQ100+GZ100+HN1+HN10+NJ100+TJ100+TJ101`、`regionCount 7` | 国服七区口径，与全球 ARAM 不同源（判定理由 2） |
| `/api/hexdata/heroes/{id}` | 顶层 8 个数组，无 balance 类字段 | 行级数据里也没有 |

### 3.6 未探测（按 Anti-scope 第 3 条主动排除，不是遗漏）

| 来源 | 状态 | 理由 |
|---|---|---|
| RESG（`bb` 字段） | **未发出任何请求** | 无明确引用许可条款、无公开 API。它页面上显示平衡数值这件事只能说明「有人拿到了」，不构成我们可授权引用的来源 |
| ARAMKit | **未发出任何请求** | 同上 |

---

## 4. 精确化工单原文的一处不准确陈述（重要，否则会误导后来人）

工单 P3「现状证据」写的是：

> `game/data/characters/yasuo/yasuo.bin.json`（200，无）

**这个「无」字不准确，必须精确化为：200，含 ARAM 逐技能覆盖，但不含英雄级平衡乘数。**
本轮把两个样本的 `"ARAM"` 命中逐处打出来核对过：

**亚索（`yasuo.bin.json`，200，118,649 B）—— `"ARAM"` 恰好 2 处：**

1. hash 键 `{f9c2333e}` 下的一个 `SpellEffectAmount` 覆盖：
   ```json
   "{f9c2333e}": { "ARAM": { "value": [27.0,27.0,25.0,23.0,21.0,19.0,19.0], "__type": "SpellEffectAmount" }, … }
   ```
   → 这是**某个技能某个数值**在 ARAM 下的 7 级覆盖，不是英雄级乘数。
2. `ItemRecommendationOverride` 的上下文：
   ```json
   "mOverrideContexts": [{ "mMapID": 12, "mModeNameStringId": "ARAM", "__type": "ItemRecommendationOverrideContext" }]
   ```
   → 这是 **ARAM 起始装推荐**，与平衡无关。

**提莫（`teemo.bin.json`，200，58,010 B，对照样本）—— `"ARAM"` 同样恰好 2 处：**
起始装推荐覆盖 + 一处 `SlowAmount` 的 ARAM 值 `[20.0 × 7]`。**`DamageDealt` 命中 0、`Modifier` 命中 0。**

**为什么这个区别重要：** 如果按工单原文记成「CD 完全没有 ARAM 维度」，后来人会以为
「ARAM 覆盖」这条路根本没通过；真实情况是**CD 有 ARAM 维度，但粒度是逐技能数值覆盖，
不是我们要的「英雄伤害输出 / 承受伤害乘数」**。这两件事的可行性完全不同，
混淆会导致下一轮调研从错误的起点开始。

顺带记录本轮对 `Multiplier` / `multiplier` 命中的逐处核对（亚索 4 处、提莫 7 处），
**没有一处是英雄级平衡乘数**：全部是技能公式内部件（`mMultiplier` + `mDataValue`）、
tooltip 列表的展示倍率（`multiplier: 100.0`）、通用暴击伤害（`critDamageMultiplier: 2.0`）、
以及对小兵/野怪的 `monstermod` / `MinionMonsterDurationMod`。

---

## 5. 工单列的另两条候选路径的处置

### 5.1 「LCU 客户端本地是否有海斗平衡参数缓存文件」→ **即使可行也不采纳**

这条需要 Windows 真机排查（游玩后本地磁盘 / 内存里的游戏内配置），本环境没有 Windows、
没有 League 客户端，**无法执行网络侧以外的验证**。但结论仍然是闭合的，因为即使真机找到了本地缓存：

- **(a) 需要逆向未公开格式。** 本地缓存是二进制 / 加密的游戏数据，没有公开 schema；
  解析出来也无法与任何公开来源交叉校验，等于把「一个无法验证的数字」显示给用户。
- **(b) 口径无法对齐。** 本地客户端缓存承载的是**客户端当前版本**的配置，而本项目海斗统计口径是
  hexdata 的国服七区 `queueId 2400` 历史样本（`seriesId: queue2400_seven_region_patch16.18_validated_subset_v1`）。
  两者的时间窗、区服、模式版本都不一致，交叉验证无从做起。
- **(c) 合规成本更高。** Anti-scope 第 3 条已经因为「无明确授权」排除了 RESG / ARAMKit 的**页面**抓取；
  逆向客户端本地文件比抓取公开页面的授权链更弱，不是更弱一点，是根本没有。
- **(d) 本轮的网络侧证据已经把「有可用来源」这个前提打掉。** §3.2 的 `kiwi-hub.json` = `[]` 与
  `maps.json` 无海斗地图说明：**全球客户端数据里海斗的数值维度是空的**。国服客户端可能另有本地配置，
  但那正好落在 (a)(b)(c) 三条里。

→ 判定：**「即使可行也不采纳」，与网络侧结论合并为同一个终局。**

### 5.2 「版本公告页面解析」→ **排除**

Riot 官方 patch notes 的 ARAM 平衡调整以自然语言散文发布（"we're increasing Yasuo's damage dealt by 5%"），
无结构化字段、格式随版本变动、需要人工核对；并且**不覆盖国服海斗**（国服由腾讯运营，平衡公告渠道不同）。
人工成本高、无法自动化、口径不匹配 → 排除。

---

## 6. 什么情况下才值得重开这张工单（避免以后重复调研）

以下三条**任一**成立时可以重开，否则不要再花时间在同一个方向上：

1. hexdata 上游新增了平衡维度字段（`/api/hexdata/meta` 出现 balance / multiplier 类键，
   或 `dataHealthWarnings` 开始声明相关内容）。**这是唯一已授权的数据源，它没有就等于我们没有。**
2. Riot 官方（DDragon 或 Riot Games API）开始发布结构化的模式平衡表。
   本轮已实测确认 DDragon 16.18.1 没有（§3.4）。**Riot Games API 本轮没有探测**：它需要开发者 API key
   （本仓库没有，也不该为这张工单去申请），且它是对局/账号维度的接口，静态平衡表历来不在它的范围内。
   这一条如实标注为「未探测」，不作为放弃的理由——放弃靠的是 §1 的三条已实测理由。
3. 拿到 RESG / ARAMKit 的**明确书面引用许可**，或它们开放公开 API。
   在此之前，Anti-scope 第 3 条持续有效，**不要以「别人都在用」为理由绕过**。

反过来，以下几条**不构成**重开理由（本轮已逐一排除，别再试）：
CommunityDragon 换个 patch 版本目录、换个人气英雄文件、猜一个新的 balance 文件名、
DDragon 换个 locale、以及「去 RESG 页面上看一眼数值是多少」。

---

## 7. 本轮探测留下的原始证据

探测过程中下载的文件与目录索引保存在会话临时目录（不进仓库，避免把 1.4 MB 的第三方数据塞进源码树）：
`$PI_SCRATCH_DIR/r116f-probe/`（`v1-index.html` / `v1-files.txt` / `queues.json` / `maps.json` /
`game-mode-mutators.json` / `augment-lists.json` / `kiwi-hub.json` / `yasuo.bin.json` / `teemo.bin.json` /
`champion-summary.json` / `champions-157.json` / `dd-yasuo-zh.json` / `chars.html` / `gamedata-index.html` /
`idx-cfg.html` / `idx-maps.html` / `idx-shared.html` / `idx-cfgdefaults.html` / `probes.txt`）。
本文档表格里的每一个状态码与字节数都出自这批文件对应的真实请求，可按 §2 的命令逐条复现。
