# WORKLIST-R47-ADDENDUM（给 GPT 执行）

> R47 验收结论：A 组（P0，斗魂降级+去二分+结案注释）三项全部真实现，变异测试全过；
> B-1/B-3/B-4、C-1/C-2/C-3、D 组全部真实现；**唯一缺口是 B-2，一行没动**。

---

## B-2（补做）：换行逻辑仍是 `@media`，且测试还在断言错误写法

`web/gameplay.css:1417-1423`（`@media (max-width: 1080px)`）和 `:1437-1451`
（`@media (max-width: 700px)`）管着 `.recommendation-champion-summary`、
`.champion-matchups` 等元素的换行，**这次一行都没有改成 `@container`**。

讽刺的是正上方 `:1403-1409` 就是同一个 `.recommendation-area` 容器下
`@container recommendation-area (max-width: 900px)` / `(max-width: 620px)`
管 `.live-augment-column > div` 换行的**现成正确写法**，照抄这个模式即可：

```css
@container recommendation-area (max-width: 1080px) { /* 原 @media 1080px 的内容 */ }
@container recommendation-area (max-width: 700px)  { /* 原 @media 700px 的内容 */ }
```

`.recommendation-area` 本身已经是 `container: recommendation-area / inline-size`
（`gameplay.css:998`），`.recommendation-champion-summary` / `.champion-matchups` /
`.live-teams` 都是它的真实后代（`web/gameplay.js:3405` 起渲染在
`<section class="recommendation-area">` 内），换成 `@container` 是安全的。

**测试也要跟着改**：`web/champions.test.cjs:3814` 现在还在用
`cssBlockAfter(gameplayStyles, "@media (max-width: 1080px)")` 去定位这段样式——
这行断言本身就在锁死"用 @media"这个错误写法，请改成定位
`@container recommendation-area (max-width: 1080px)`。

**验收会做变异测试**：改完后把 `@container` 临时改回 `@media`，
确认新测试会 FAIL；恢复后确认 PASS。

---

## 交付要求

- `go build`/`go vet`/`go test ./...` 全绿；`node --test web/*.test.cjs desktop/*.test.cjs` 全绿。
- 这是本轮唯一遗留项，改完不需要动 A/B-1/B-3/B-4/C/D 组，那些都已验收通过。
