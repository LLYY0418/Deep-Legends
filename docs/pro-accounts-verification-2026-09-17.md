# 职业选手韩服账号核对清单（2026-09-17，用户核对定稿）

**状态：已于 R105 完整嵌入代码。** 唯一账号事实来源；`pro_seed_accounts.go` 保留本表 33 人、53 账号及原始顺序，`pro_reviewed.go` 保证旧快照、上游归属冲突和补充发布不会覆盖最终页面清单。

**核对方式：** 用户对着 OP.GG 小程序逐条核对了 Claude 用 dpm.lol `/pro/{选手名}` 内嵌 JSON + Leaguepedia Soloqueue IDs 栏交叉产出的初稿，改正账号归属、补充遗漏账号，最终以本文件表格为准。原稿里"大师以下 LP 全部不可信（dpm.lol 占位值 75）"“最近对局”日期来自 dpm.lol 一次性抓取快照这两条说明依旧适用于表格里非 seed 字段，但**LP 数字和最近对局日期都不用于内置**——账号身份（GameName/TagLine）和所属选手/位置才是本文件的内置依据，动态段位与活动优先读取 OP.GG 目录字段，缺失账号按 R104 涓流调度向 Riot 查询，不使用本文件里的静态日期/LP 快照。

**★ 唯一一处与用户上传原稿不同的地方：** `dyjkbysb#KR1` 用户已在 2026-09-17 对话中明确核对确认归属 **Wei**（打野），不是此前 R99 种子数据和更早一版清单里标注的 Rookie。这是用户看着 OP.GG 小程序做出的最终裁决，请勿改回 Rookie。

**去主号说明：** 下表"主号?"列仅为历史参考。R104/R105 优先用 OP.GG 的 `revision_at` 最近活跃时间排序，缺失账号以 Riot 最近一局开始时间兜底；未知或相同时间按本表顺序，不按段位/LP 推断主号。OP.GG 时间不宣称为 Riot 精确开局时间。

---

## BLG（Bilibili Gaming，7 人）

| 选手 | 位置 | 账号 | 主号?(历史标注，内置不用) |
|---|---|---|---|
| Bin | 上单 | `빈 스토리#KR1` | ✅ |
| Wenbo | 上单 | `14小孩幻想赢对线#4453` | ✅ 唯一号 |
| Flandre | 上单 | `aierlanlaozhu#KR1` | ✅ |
| Xun | 打野 | `我累铜泥丸#小重o` | ✅ |
| Xun | 打野 | `Xun#OOK` | 小号 |
| knight | 中单 | `BLG 온#KR1` | ✅（用户已确认归 knight，非 ON） |
| knight | 中单 | `쯔지 쌰 쯔지#4165` | 小号 |
| Viper | 下路 | `Blue#KR33` | ✅ |
| ON | 辅助 | `키보드에음료쏟아서고장내는이도윤#KR1` | ✅ |

## IG（Invictus Gaming，6 人）

| 选手 | 位置 | 账号 | 主号?(历史标注，内置不用) |
|---|---|---|---|
| TheShy | 上单 | `The shy#asdf` | 小号 |
| TheShy | 上单 | `은여하#1103` | ✅ |
| TheShy | 上单 | `스몰더 아빠#tsts` | 小号 |
| TheShy | 上单 | `눈사람#cold1` | 小号 |
| TheShy | 上单 | `말랑한블루베리#KR1` | 小号 |
| Wei | 打野 | `Kimman#zxfkk` | ✅ |
| Wei | 打野 | `dyjkbysb#KR1` | 小号（★用户2026-09-17核对确认归 Wei，非 Rookie） |
| **Rookie** | 中单 | `모든일은같이#KR1` | ✅ 已内置（R99/R101） |
| Rookie | 中单 | `벼락식혜#0070` | 小号 |
| Rookie | 中单 | `EmberKnight#KR0` | 小号 |
| Rookie | 中单 | `asdfzxcvasdfwqer#KR2` | 小号 |
| Assum | 下路 | `xycg#KR1` | ✅ |
| JiaQi | 下路 | `강딱풀#500` | ✅ |
| JiaQi | 下路 | `Whymesodiao#jiaq3` | 小号 |
| Meiko | 辅助 | `qweasdqweasdb#1372` | ✅ |

## T1（5 人）

| 选手 | 位置 | 账号 | 主号?(历史标注，内置不用) |
|---|---|---|---|
| Doran | 上单 | `어리고싶다#KR1` | ✅ |
| Oner | 打野 | `오 너#111` | ✅ |
| Faker | 中单 | `Hide on bush#KR1` | ✅ |
| Peyz | 下路 | `Peyz#KR11` | ✅ |
| Keria | 辅助 | `Ciro#KR10` | ✅ |

## HLE（Hanwha Life Esports，5 人）

| 选手 | 位置 | 账号 | 主号?(历史标注，内置不用) |
|---|---|---|---|
| Zeus | 上单 | `Athene#lll` | ✅ |
| Kanavi | 打野 | `vinaka#KR1` | ✅ |
| Kanavi | 打野 | `PoAtan#ASP` | 小号 |
| Zeka | 中单 | `suis#kr7` | ✅ |
| Zeka | 中单 | `Kiruru#kr7` | 小号 |
| Gumayusi | 下路 | `HLE Gumayusi#0298` | ✅ |
| Gumayusi | 下路 | `thsorre#2830` | 小号 |
| Delight | 辅助 | `플레이리스트겨울#KR1` | ✅ |

## GEN（Gen.G，5 人）

| 选手 | 位置 | 账号 | 主号?(历史标注，内置不用) |
|---|---|---|---|
| Kiin | 上单 | `kiin#KR1` | ✅ |
| Canyon | 打野 | `JUGKlNG#kr`（K 后为小写 l，非大写 I） | ✅ |
| Chovy | 中单 | `허거덩#0303` | ✅ |
| Ruler | 下路 | `강 철#샤 넬` | ✅ |
| Duro | 辅助 | `Duro#Gen` | ✅ |

## DK（Dplus KIA，5 人）

| 选手 | 位置 | 账号 | 主号?(历史标注，内置不用) |
|---|---|---|---|
| Siwoo | 上单 | `TOPKING#asd` | ✅ |
| Siwoo | 上单 | `아무것도 몰라요#12345` | 小号 |
| Lucid | 打野 | `DK Lucid#KR1` | ✅ |
| Lucid | 打野 | `너무재미있겠다#KR1` | 小号 |
| ShowMaker | 中单 | `DK ShowMaker#KR1` | ✅ |
| ShowMaker | 中单 | `MIDKING#asd` | 小号 |
| Smash | 下路 | `DK Smash#KR7` | 小号（段位更高但更久没打） |
| Smash | 下路 | `Smash#KR2` | ✅（段位较低但更活跃——用户新排序规则的直接例证） |
| Smash | 下路 | `사옥 지박령#KR4` | 小号 |
| Career | 辅助 | `팽도리#1015` | ✅ |
| Career | 辅助 | `인간 병기#0829` | 小号 |

---

## 队伍归属

dpm.lol 的 `team` 字段与 `pro_roster.go` 的 33 人名单逐一吻合，包括容易记错的三个：**Viper 在 BLG**（不是 HLE）、**Gumayusi 在 HLE**（不是 T1）、**Peyz 在 T1**。名单本身不需要改。

## 历史遗留：之前版本"需要重点裁决的 9 条"已全部由本表定稿覆盖

初稿曾列出 9 条需要用户用 OP.GG 裁决的冲突（BLG 온#KR1 归属、TheShy 主号、JiaQi 账号分歧、Keria/Zeus/Ruler 双源零交集、Gumayusi 三号未对齐、Smash 主号打架、Career 单源）。用户已对照 OP.GG 小程序逐条核对，本表就是核对后的定稿，不再单独保留待裁决列表。

## 文档更新（配合 R101/R102）

`docs/pro-players-sources.md` 里三处"Wenbo 目前没有可交叉确认归属的韩服账号"的表述已过期，Wenbo 账号 `14小孩幻想赢对线#4453` 在本表中确认。
