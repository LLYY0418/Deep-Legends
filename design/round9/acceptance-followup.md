# 第九轮验收结果与收尾工单

> 2026-08-21。对照 `design/round9/README.md` 的 A~G 组逐条复核代码，结论见下。
> `go vet ./... && go test -race ./...` 已由你在开发机跑过（`ok lol-loot-assistant 3.543s`）。
> `node --test web/*.test.cjs desktop/*.test.cjs` 本次在沙箱跑过：**73 / 73 通过**（上一轮基线 65）。

## 结论：A~G 组全部【已实现】，只剩 4 条收尾项

七组问题（评分列拆分、海克斯图标与品质色、符文块改版、tooltip 残留、跨页滚动、量纲双重放大、分模式推荐）**全部在代码里落地**，且大多数带了专门的回归测试（`web/champions.test.cjs` 里六个 `round 9 *` 用例覆盖了量纲、海克斯渲染、推荐模式路由、滚动位置、tooltip 抑制、好友刷新）。

对最关键的一条做了变异验证：把 `web/gameplay.js:2436 rate()` 改回旧的 `parsed <= 1 ? parsed * 100` 双重放大逻辑后，`node --test web/champions.test.cjs` 从 50 全绿变成 **49 pass / 1 fail**，且失败的正是那条量纲测试 —— 证明它是真护栏，不是字面量型假测试。

剩下的不是 bug，是**测试覆盖缺口**和**代码整洁度**两类收尾项，不影响当前行为：

---

### 1. A1/A2（评分列拆分 + <5 标红）—— 实现正确，缺测试

`web/gameplay.js:1015`：

```js
const tone = record.score >= 8 ? " is-gold" : record.score >= 6.5 ? " is-good" : record.score < 5 ? " is-poor" : "";
```

`:1020`：

```js
return `<span class="match-score-cell"><span class="match-score-value">${scoreChip(record)}</span><span class="match-score-badge">${scoreBadgeChip(record)}</span></span>`;
```

阈值改成了 `< 5`，评分/徽章也拆成了固定两栏 —— 和工单 A1/A2 要求一致。但全仓搜索 `is-poor` / `match-score-cell` 在 `web/champions.test.cjs` 里都没有命中，**没有测试锁住这两处**。以后有人把阈值悄悄改回 `< 4`，或者把两栏又合并回去，测试不会报错。

建议补一条测试：给三个分数（4.9 / 6.4 / 8.1）断言 `is-poor` / 无 tone / `is-gold` 三种输出，并断言 `match-score-value` 与 `match-score-badge` 是两个独立节点。

### 2. E3（最小样本闸门）—— 实现正确，缺测试

`champions_structured.go:513-523`：

```go
const (
    structuredMetricMinimumGames = 50
    structuredMetricHeadRatio    = 0.01
)

func structuredSampleAllowed(play, leadingSample int) bool {
    return play >= structuredMetricMinimumGames && leadingSample > 0 && float64(play)/float64(leadingSample) >= structuredMetricHeadRatio
}
```

闸门逻辑和工单要求（绝对场次下限 + 相对头名比例）完全一致，注释也说明了取值理由。但 `*_test.go` 全仓搜索 `structuredMetricMinimumGames` / `structuredSampleAllowed` 零命中 —— **没有 Go 测试覆盖这道闸门**。建议补一条单测：构造一个 `leadingSample=1000` 的分布，验证 `play=49` 被过滤、`play=50` 且占比 `>=1%` 时保留、`play=200` 但占比 `<1%` 时仍被过滤（这是两个维度都要各自能杀住变异）。

### 3. 死代码清理（可选，不影响功能）

`champions.go:1963 loadDetailHTML`、`:2048 parseChampionBuild`、`:2176 parseMetricRows` 三个函数**依然存在且互相调用**，但 `loadDetailHTML` 本身**零调用者**（全仓搜索 `loadDetailHTML(` 只有它自己的定义行命中）。`loadDetailOnce` 现在无条件走结构化 API（`opggModeSpecs` 表驱动），这条 HTML 抓取链路已经彻底成了死代码，约 150 行。

删掉这三个函数后，工单里 E5「两个 SummonerSpells Table 被静默拼接」的风险随之自动消失（因为产生这个问题的代码不会再被执行）。不删也不影响现在的行为，纯粹是代码整洁度问题。

### 4. `opggModeSpec.MinVersion` 字段未接线（可选）

`champions_structured.go:133` 声明了 `MinVersion string`，六个模式条目（`:137-142`）全部填了字面量 `"current-2"`，但全仓搜索 `MinVersion` 除了这六处赋值和字段声明外**没有任何读取点**。实际生效的新鲜度判断是 `champions_structured.go:205-215 opggPatchIsStale` 里硬编码的 `currentMinor - dataMinor > 2`。

两个字段（结构体里的 `MinVersion` 和函数里硬编码的 `2`）表达的是同一件事，但没有连起来 —— 以后想按模式差异化新鲜度阈值（比如给 `nexus_blitz` 一个更宽松的容忍度）会发现改了 `MinVersion` 也不生效。建议要么让 `opggPatchIsStale` 读 `spec.MinVersion` 并解析出阈值，要么直接删掉这个未使用字段，避免误导。

---

## 优先级

这 4 条里，**前两条（补测试）优先于后两条（清理）**——前两条是在给已经正确的行为补护栏，后两条纯粹是代码整洁度，不着急。四条都不阻塞发布。

## 仍然建议做但与本轮代码无关

- `go build ./...` 未见到你贴出的日志，不过 `go test -race ./...` 本身就要求整个模块能编译通过，所以构建成功基本已经被间接验证了，不是必须再单独跑一遍。
- 用**包含这批改动**的构建重新采一次真机 `diagnostics.jsonl`，确认 `sgp_ranked_stats_shape` 埋点开始产出数据（第九轮工单 0.1 提到的三份旧日志早于本次改动约 10 小时，还没验证过）。
