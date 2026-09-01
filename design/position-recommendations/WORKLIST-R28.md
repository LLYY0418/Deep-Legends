# R28 工单：出装回归 op.gg（含第六件） + 海克斯图标修复

日期 2026-08-27。**标「实测」的都是本轮真跑过 curl / 读码验证的。**

---

## 0. 日志状态：连续三次上传的是同一个文件

```
sha256 = 3d1dacdae13d180a…   1831 行   08-26 13:24:55 ~ 13:48:57
```

三次上传**逐字节相同**。而代码最后改动是 **08-27 02:19**（之后只有我留的 `zz_proof_test.go`）。
日志比代码早 11 小时，**判断不了当前状态**。

顺带纠正一个误解：**GPT 并没有放弃 lolalytics**。
`lolalytics.go` 还在、`champions.go:31` 主机仍是 `a1.lolalytics.com`、
`:609-619` 的 UA 伪装也还在。你看到的 op.gg 数据是 **403 之后的兜底路径**在工作
（`champions_structured.go:952-969` 把兜底结果标成 `ItemChainStatus = ready`）。

**导日志的正确方式**：浏览器直接开 `/api/diagnostics/log` 下载当前正在写的那份；
或确认 `%LOCALAPPDATA%`（Windows）下那个文件的修改时间是今天。
注意便携版有已知的缓存陷阱：新 exe 可能静默复用旧程序目录，
导致「你跑的」和「你导日志的」是两个副本。

---

## A. 出装：正式回归 op.gg，并加回第六件

### A.1 已定案（用户拍板）

> 「不行也还是用 opgg 的吧，只要保证和 opgg 展示的数据一样，把第六件装备也加回来，
> 去重不好做或者做出来装备少很多就算了」

### A.2 删除 lolalytics 整条链路

- 删 `lolalytics.go` / `lolalytics_test.go`
- 删 `champions.go:609-619` 的浏览器 UA + Origin + Referer + Sec-Fetch 伪装分支
  （这段是 GPT 自主加的，超出我原工单 D9 论证的边界，而且实测无效）
- 删 `champion_cache.go` 里的 `lolalyticsHost` 白名单项与 `/mega/` 缓存策略
  （顺带清掉一个死配置：缓存键是 `v1|lolalytics-itemset|…` 而白名单查的是
  `v1|a1.lolalytics.com|`，**永不匹配**，磁盘缓存本来就是失效的）
- 删 `shouldUseLolalyticsItems`、`ItemWindow`「近 30 天」文案
- `ItemChainStatus` 的 ready/unavailable 二态简化为「有数据 / 抓取失败」

### A.3 加回第六件

R25 时我建议砍掉第六件，理由是它样本极小（实测 jax [3,2,2,1,1] / garen [21,19,17,13,8]）。
**现在按你的要求加回来，与 op.gg 对齐。** 需要恢复：

- Go：`FifthItems` 之后补 `SixthItems` + `SixthSample`
- `parseOPGGDepthRows` 已经在解析 depth 4/5/6 三层（`result := map[int][]championMetricRow{4:{},5:{},6:{}}`），
  **只需把 depth 6 透传出去**，不用改解析器
- 前端 `web/champions.js:1677-1680` 的 `groups` 数组补回「第六件」

### A.4 去重：建议保留（实测不会让装备变少）

你说「去重做出来装备少很多就算了」——**实测过了，不会少。**

只剔除 `core_items[0]` 那三件（不跨列去重），剩余行数（KR / emerald_plus）：

| 英雄·分路 | 第四件 | 第五件 |
|---|---|---|
| 贾克斯·打野 | 4 / 5 | 5 / 5 |
| 卡蜜儿·上单 | 4 / 5 | 5 / 5 |
| 亚索·中单 | 4 / 5 | 4 / 5 |
| 盖伦·上单 | 4 / 5 | 5 / 5 |
| 塞拉斯·打野 | 4 / 5 | 4 / 5 |

**每列最多只被剔掉 1~2 件，全部 ≥4 行。**
而它解决的正是你最初的投诉（核心装有金身、第五件又出现金身）。

**但这与「和 op.gg 展示的数据一样」有冲突** —— op.gg 自己是不去重的。
两个要求二选一，我倾向保留去重（用户体验优先），**请你定**：
- **选项 1**：保留去重 → 与 op.gg 有细微差异，但不会出现重复装备
- **选项 2**：完全对齐 op.gg → 会重新出现「核心装和第五件重复」

第六件加回来后要注意：去重的「前缀」只用核心装三件，
**不要**把第四、第五件也算进去（那样第六件会被剔空，实测过）。

### A.5 护栏

| # | 测试 | 必须能杀死的变异 |
|---|---|---|
| A1 | lolalytics 已删干净 | 全仓 grep `lolalytics` 为 0（**不要**再写只查旧函数名的断言，见 R26 的教训） |
| A2 | 第六件透传 | fixture 断言 `SixthItems` 有 5 行；去掉透传 → 挂 |
| A3 | 去重只剔核心装（若选项 1） | 断言第五件仍可含第四件出现过的 ID；改成跨列剔 → 挂 |
| A4 | 三列都非空 | 5 个真实英雄 fixture，断言每列 ≥3 行 |

---

## B. 【已定位】海克斯图鉴部分图标缺失

### B.1 根因（实测确认）

**上游 CommunityDragon 对这些海克斯只提供 `_small.png`，没有 `_large.png`。**

我穷举测试了 `cherry-augments.json` 全部 **657 个**海克斯的图标 URL（两个 CDragon 前缀都试）：

```
总 657   失败 60   （斗魂 52 + 海斗 8）
```

逐个验证失败样本，`_small` 存在但 `_large` 不存在：

```
200  drop_bear_small.png          404  drop_bear_large.png
200  rice_and_chicken_small.png   404  rice_and_chicken_large.png
200  mercy_small.png              404  mercy_large.png
200  do_or_die_small.png          404  do_or_die_large.png
—— 对照组（正常的）——
200  tapdancer_small.png          200  tapdancer_large.png
```

完整失败清单已存到仓库：
**`design/position-recommendations/augment-icons-missing-large.txt`**（60 条，含 ID 与中文名）。

失败的多是带下划线的命名（`Drop_Bear_Small.png` / `Rice_And_Chicken_Small.png` /
`Chroma_Flux_Small.png`），说明是 Riot 那边这批资源本来就没导出大图。

### B.2 为什么现有兜底没救回来

代码链路本身是对的：

- `normalizeAugmentIconPaths`（`gameplay.go:4520`）把 `_small.png` 提升成 `_large.png`，
  **并返回原始 small 作为 fallback**
- 后端把 fallback 下发成 `imageFallbackPath`（`champions.go:1981`）
- 前端 `assetImage`（`web/champions.js:172-173`）输出 `data-augment-fallback`
- `prepareImages`（`:1836-1866`）用 `addEventListener("error")` 在大图失败时换 fallback
  —— **这里是 CSP 安全的写法（不是内联 onerror），没有踩项目那个老坑**

但它是**依赖前端图片 onerror 事件的两跳兜底**：先请求大图 → 404 → 再换小图。
链路长、且中间任何一环（`byID` 查不到 meta、`FallbackIconPath` 为空、
`isLargeAugment` 正则不匹配）断掉就直接退化成首字占位块。

### B.3 建议改法：在服务端一次解决

`serveCommunityDragonImage`（`main.go`）**已经是多候选顺序尝试**的结构：

```go
remotePaths := communityDragonImagePaths(assetPath)
for _, remotePath := range remotePaths {
    loaded, err := a.loadAsset(...)
    if err == nil && strings.HasPrefix(http.DetectContentType(loaded), "image/") {
        data = loaded; break
    }
}
```

而 `communityDragonImagePaths` 目前只返回两个前缀变体：

```go
return []string{
    "/latest/plugins/rcp-be-lol-game-data/global/default/" + relative,
    "/latest/game/" + gameRelative,
}
```

**只要在候选列表里再加上 `_large.png → _small.png` 的降级变体**，
这 60 个就会在服务端直接命中小图，前端完全不用改、也不再依赖 onerror 兜底：

```
/latest/plugins/.../xxx_large.png
/latest/game/assets/.../xxx_large.png
/latest/plugins/.../xxx_small.png     ← 新增
/latest/game/assets/.../xxx_small.png ← 新增
```

**注意**：小图分辨率较低（约 10KB vs 大图），放大显示会糊。
建议前端给用了小图的加一点 `image-rendering` 处理，或接受轻微模糊
——总比现在显示一个「灵」字占位块好。

### B.4 护栏

| # | 测试 | 必须能杀死的变异 |
|---|---|---|
| B1 | 候选路径含 small 降级 | 断言 `communityDragonImagePaths("…_large.png")` 返回 4 条且含 `_small.png`；去掉降级 → 挂 |
| B2 | 顺序正确 | 断言 large 变体排在 small 之前（优先大图） |
| B3 | 非 augment 路径不受影响 | 传一个 `champion/1.png` 断言仍只返回 2 条 |
| B4 | 真实失败样本 | 用清单里的 `Drop_Bear` / `Mercy` / `RiceAndChicken` 做 fixture |

---

## C. 仍然待办（来自 R26 / R27，尚未执行）

这两份工单都还没做，与本工单有重叠，**建议合并成一轮**：

**来自 R26（`ACCEPTANCE-R25-WORKLIST-R26.md`）**
- P0-1 `specialistRuneKey` 缓存键碰撞 —— **我复核过，仍是 `championID*10+ordinal`，没改**。
  可执行测试证明：1..1000 英雄 × 6 位置共 **495 组碰撞**（奥拉夫打野 == 艾希无分路 == 键 22）
- P0-3 低样本角标：Go 侧算了 `FourthSample`/`FifthSample` 并传到前端，
  前端 `web/champions.js:1680` 解构出 `sample` 后**模板里从未使用**
- P1-4 `lolalytics_test.go:234-244` 那条只查旧函数名的假护栏（随 A.2 一起删）

**来自 R27（`WORKLIST-R27.md`）**
- **A. 优劣势对抗用错数组**（这条价值最高，改动最小）：
  代码用 `Summary.Positions[X].Counters`（3 条）覆盖了 `Data.Counters`（27~30 条）。
  实测永恩上路顶层 30 条、positions 只有 3 条，而那 3 条
  `[164 卡蜜儿, 23 泰达米尔, 150 纳尔]` **正是你截图里显示的**。
  顶层数组本身就按请求位置返回，删掉覆盖逻辑即可
- B. 绝活哥符文：先补埋点（整条链路目前**零埋点**，无法诊断）
- C. 三处排位切换解耦（共用 `tab.rankedQueue` 导致联动）+ 去掉文字提示 +
  能力表现用 `windowMatches` 导致灵活组排没数据
- D. S1/S2/S3 赛段段位：先埋一次性真机探测
  （实测 SGP `splitsProgress` 301 次全空、LCU 无分赛段字段）

---

## D. 建议执行顺序

1. **R27 的 A**（优劣势对抗）—— 一处删除，用户可见收益最大
2. **B**（海克斯图标）—— 服务端加两个候选路径，60 个一次修好
3. **A**（删 lolalytics + 加回第六件 + 去重选项）
4. **R26 的 P0-1 / P0-3**
5. **R27 的 B / C / D**

另：仓库根目录的 `zz_proof_test.go` 是我验收时留的空壳（沙箱无删除权限），**请手动删除**。
