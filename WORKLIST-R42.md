# WORKLIST-R42（给 GPT 执行）

> 两件事：① 绝活哥符文页现在点不中、应用不了，这是个真回归，先修。
> ② 卡片布局按用户的新要求重排——对手信息从右侧独立列改成横排放进顶部那一行，
> 原来放在右上角的"3天前"挪到左边，行高可以增加，目的是让符文区域不再出现滚动条。

---

## A 组（P0）：绝活哥符文页选不中，应用不了 —— 真回归，根因已定位

### A-1 根因

符文来源分三个页签：OPGG / 绝活哥 / 职业选手。**OPGG 和职业选手**这两个页签走的是
`renderRuneChoice()`（`web/gameplay.js:3534`），每一条都渲染成：

```html
<button class="rune-choice-selector" type="button" role="radio" data-rune-choice="opgg-0">
```

点击事件监听在 `web/gameplay.js:3863`：

```js
for (const button of nodes.liveContent.querySelectorAll("[data-rune-choice]")) button.addEventListener("click", () => {
  state.selectedRecommendation = button.dataset.runeChoice;
  renderLive();
});
```

**但绝活哥页签走的是另一条渲染路径**：`renderSpecialistPlayers()` →
`renderSpecialistPlayerGames()`（约 `web/gameplay.js:3485-3531`），
这段渲染出来的每一局是：

```html
<div class="specialist-game-head">
  <span class="specialist-game-selector"><span>${outcomeWithPosition}</span></span>
  <time>...</time>
</div>
```

**`.specialist-game-selector` 是一个 `<span>`，不是 `<button>`，也没有 `data-rune-choice` 属性。**
点击事件监听器根本找不到它，所以点了没反应——`state.selectedRecommendation` 永远不会被设成
`specialist-N`，`selectedRuneRecommendation()`（`web/gameplay.js:3784`）算出来的
"当前选中项"永远落在 OPGG 那一条，绝活哥的符文页自然应用不了。

这条大概率是 R40-D 组把绝活哥从原来的扁平列表改成"按玩家分组+每人多局"的新布局时，
只顾着重排数据结构，漏了把选中交互也一起接上。

### A-2 改法

1. **给每一局的选择器补上和 OPGG/职业选手一致的交互**：
   `.specialist-game-selector` 改成真正的 `<button type="button" role="radio"
   data-rune-choice="specialist-N">`（`N` 用 `selectedRuneRecommendation()` 里
   `specialist-${index}` 同一套编号规则，**不要另起一套**，否则又会对不上）。
2. **选中态要有视觉反馈**：参考 `renderRuneChoice` 里 `is-selected` 那个类名的做法，
   `.specialist-game-row` 也要能反映"当前选中的是这一局"。
3. **点击整行都应该能选中**，不要要求用户精确点在某个小图标上——
   现在 `.specialist-game-head` 里塞了结果文案和时间两块内容，
   建议把整个 `.specialist-game-head` 包成按钮，或者给整个 `.specialist-game-row`
   绑定点击委托，具体怎么做看 B 组重排之后的 DOM 结构定。
4. **确认应用符文之后的行为正常**：`applyRunes()`（`web/gameplay.js:3871`）
   拿到的 `recommendation` 应该正确对应用户点的那一局，实际调用 LCU 写入的 perkIds
   要和这一局的 `selectedPerkIds` 一致。

### A-3 变异测试

这条路径**目前是零测试覆盖**（这也是为什么这个回归能在 R40 上线时溜过去）。
1. 构造一个"绝活哥页签下有 2 局数据"的场景，模拟点击第二局，
   断言 `state.selectedRecommendation` 变成对应的 `specialist-1`；
   断言 `selectedRuneRecommendation()` 返回的确实是第二局那条符文数据（不是 OPGG 那条）。
2. 断言点击后重绘，该局的选中态 class（`is-selected` 或等价类名）出现在正确的那一局上。
3. 把 `data-rune-choice` 属性去掉，测试必须 FAIL——防止以后又出现"渲染了但没接交互"的情况。

---

## B 组（P1）：卡片布局重排 —— 对手信息挪到顶部横排，时间挪到左侧

### B-1 现状问题

现在每局卡片是这个结构（`web/gameplay.css:1108`）：

```css
.specialist-game-content { grid-template-columns: minmax(0,1fr) minmax(190px,250px); }
```

左边放符文板（3列：主系/副系/属性碎片），右边单独一列（190~250px宽）放对手信息
（头像+英雄名+召唤师名+段位胜率，纵向堆叠）。用户反馈这个右侧列不好看，
而且纵向堆叠的内容比符文板本身更高，撑高了整个卡片，导致符文页区域出现了滚动条。

### B-2 新布局（按用户描述）

1. **头部行（`.specialist-game-head`）重新设计**：
   - 现在是 `[结果+位置]` 在左、`[时间]` 在右（`justify-content: space-between`）。
   - 改成：`[结果+位置]` 和 `[时间]` **都挪到左侧**紧挨着（时间跟在结果后面，
     比如"胜（上路）· 3天前"这样，或者两块之间留一点间距，具体样式你定），
     **右侧腾出来的空间横向放对手信息**：对手头像 + 英雄名 + 召唤师名 + 段位/胜率
     **在一行内左右排开，不再纵向堆叠**。
   - **这一行的高度可以增加**，以容纳头像和文字，但不要增加太多——
     用户的目的是省掉纵向空间，不是又撑出一个更高的头部。
2. **原来右侧独立的 `.specialist-opponent` 列整个去掉**，
   `.specialist-game-content` 改回单列（符文板占满宽度），
   `grid-template-columns: minmax(0,1fr) minmax(190px,250px)` 这行删掉或改成 `1fr`。
3. **符文板（3列）不再需要给对手信息让出宽度，可以适当放宽/加大图标**，
   具体比例你看着调，但要保证在窗口最小宽度下三列符文不挤压变形。
4. **改完确认"符文页区域不再需要滚动条"**——用 Playwright 截图在几个常见窗口宽度下
   验证整卡高度是否比之前矮、外层容器是否不再触发 `overflow-y` 滚动。
5. **420px 窄屏下要有对应的响应式处理**（现有 `web/gameplay.css:1422-1423` 那段
   窄屏兜底逻辑要跟着新结构一起改，不要留着适配旧结构的死代码）。

### B-3 变异测试/验证

1. 用 Playwright 截图对比改前改后同一份数据在 1440px/900px/420px 三个宽度下的渲染，
   确认：①对手信息确实横排在顶部，不再是右侧纵向列；②时间确实挪到了左侧；
   ③整卡高度变矮、符文卡片列表区域不再出现纵向滚动条（如果数据量本身很大导致
   整个列表需要滚动，那是正常的，这里说的是"单张卡片内部"不应该再有滚动条）。
2. 补一条 CSS 结构性测试：断言 `.specialist-opponent` 不再是 `.specialist-game-content`
   的第二个 grid 子项（如果沿用类名的话），或者干脆断言这个类名的定位方式已经改变——
   具体断言方式看你怎么实现，但要能在有人手滑改回旧布局时报错。

---

## 交付要求

- `go build` / `go vet` / `gofmt` 干净（这轮基本是纯前端改动，Go 侧应该不受影响，
  但仍然要跑一遍确认没有连带影响）；`node --test web/*.test.cjs desktop/*.test.cjs` 全绿
  （注意不能用 `node --test web/`，必须显式列 glob）。
- **A 组必须补真变异测试**——这条路径之前零覆盖，是这次回归能上线的直接原因。
- B 组用 Playwright 截图作为验收证据，三个宽度都要截。
- 改完后请用户重启应用复现，确认：① 点绝活哥符文能选中、能应用到客户端；
  ② 对手信息在顶部横排、时间在左侧；③ 符文卡片不再需要滚动查看。
