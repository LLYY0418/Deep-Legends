# WORKLIST-R51（给 GPT 执行）

诊断人：Claude（只读诊断，未改仓库文件）。执行人：GPT。

> 本轮起因：验收 WORKLIST-R49 §E-4 时发现的一处遗留 bug——斗魂战力对比区在窄屏下
> **从未真正降级成单列**，用户要求的"提示更醒目"等其余两点已验证真实现，
> 唯独这一条卡在 CSS 优先级问题上，R49 执行时漏了。

---

## 口径决定（不要自行更改）

- 这不是新功能，是修一个**选择器优先级**导致的响应式失效 bug。
- 只改 `web/gameplay.css` 一处新增规则，**不改 JS、不改布局结构、不新增 `@media`**——
  沿用项目既有约束（R47 验收硬性要求：响应式一律走 `@container`）。
- `.live-teams` 的默认两列规则（`gameplay.css:969`）和 `.live-teams.is-arena` 的
  `auto-fit` 规则（`:970`）本身都是对的，**不要动这两行**，问题只在缺一条窄屏覆盖。

---

## R-1 修复：`.live-teams.is-arena` 在窄屏下没有单列降级规则

**证据（file:line）：**

- `web/gameplay.css:969-970`：
  ```css
  .live-teams { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 10px; padding-top: 10px; }
  .live-teams.is-arena { grid-template-columns: repeat(auto-fit,minmax(320px,1fr)); }
  ```
  `.live-teams.is-arena` 的选择器优先级是 (0,2,0)（两个类）。

- `web/gameplay.css:1451-1452`：
  ```css
  @container recommendation-area (max-width: 1080px) {
    .live-teams { grid-template-columns: 1fr; }
  }
  ```
  这条窄屏降级规则只写了 `.live-teams`（优先级 (0,1,0)，一个类）。

- **CSS 优先级规则**：同等 `@container`/`@media` 层级下，选择器优先级不比较代码顺序——
  `:970` 的 `.live-teams.is-arena`（0,2,0）**永远**会赢过 `:1452` 的 `.live-teams`（0,1,0），
  无论 `:1452` 写在文件里多靠后。所以斗魂模式（`is-arena` 场景）下，
  窄屏容器查询命中时该元素依然是 `repeat(auto-fit,minmax(320px,1fr))`，
  单列降级从未对斗魂生效——这是选择器优先级问题，不是媒体查询本身没生效。

**要做什么：**

在 `gameplay.css:1451-1459` 这个已有的 `@container recommendation-area (max-width: 1080px)` 块内，
紧跟 `.live-teams { grid-template-columns: 1fr; }` 之后新增一条：

```css
.live-teams.is-arena { grid-template-columns: 1fr; }
```

这样窄屏下 `.live-teams.is-arena`（0,2,0）会被同一容器查询层级内、后写且同等或更高优先级的规则覆盖，
生效降级为单列。**不要改成 `!important`，不要改用 `:where()` 削弱既有规则的优先级**——
直接补一条同样类组合的规则是影响面最小的修法。

**验收判据：**

1. 打开 `web/gameplay.css`，确认 `@container recommendation-area (max-width: 1080px)` 块内
   `.live-teams` 规则后紧跟 `.live-teams.is-arena { grid-template-columns: 1fr; }`。
2. 用现有的沙箱截图/jsdom 渲染核对手段（若无现成用例可加一条 jsdom 冒烟断言）：
   斗魂模式下，容器宽度 ≤1080px 时 `.live-teams.is-arena` 的计算样式
   `grid-template-columns` 应解析为单一轨道（等价于 `1fr`，不是多轨道的 `minmax(320px,1fr)` 展开）。
3. 非斗魂模式（`.live-teams` 不带 `.is-arena`）的两列/单列切换行为**不能变化**——
   这条修复只影响斗魂分支。

**变异测试判据（GPT 必须做，做完把新增的这一行删掉还原）：**

- 把新增的 `.live-teams.is-arena { grid-template-columns: 1fr; }` 临时删掉，
  重新截图/渲染核对，**必须能看到斗魂窄屏下退回多列**（复现 bug），
  证明这条规则确实是让测试由 FAIL 转 PASS 的那一行，不是凑巧合并邻近规则蒙对。
- 确认结束后**必须把这一行恢复**，不要把变异后的状态提交。

---

## 本轮明确不做的事

- 不新增 `@media` 查询。
- 不改 `.live-teams` / `.live-teams.is-arena` 在非窄屏下的规则（`:969-970` 两行不动）。
- 不动 E-4 已确认真实现的另外两点（当前玩家排小队第一、提示样式加强），本工单只解决第 2 点的
  "两列小队"窄屏降级问题。
- 不做英雄选择阶段（只有 1 个小队）相关的额外布局特判——`auto-fit` 已经处理了这种情况，
  这条修复只影响窄屏降级分支，不影响宽屏下 1 个小队时自动占满的既有行为。
