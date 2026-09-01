# 提示框改版：交付给 GPT 的落地说明

对应问题：鼠标悬停提示框顶部有一根黄条，整体偏丑。
决策方向：反色浮层 + 淡入动效 + 标题正文分层 + 去掉顶部金色、改为贴合主题色。
原型：同目录 `tooltip-redesign.html`（浏览器直接打开，可 A/B 新旧、切换「海克斯 / 浮起」两档底色、切换全部 8 套配色；底部读数会显示当前实际生效的面板 / 描边 / 标题 / 正文色值）。

**默认落地的是「海克斯」档**（第二节 v4）。

本文件只描述改动，**未改动任何仓库代码**。

---

## 一、问题定位

`web/app.css:163` 单行规则里同时叠了三份金色强调：

```css
border: 1px solid color-mix(in oklab,var(--primary) 38%,var(--line));
border-top: 2px solid var(--primary);            /* ← 用户说的黄条 */
box-shadow: 0 12px 30px rgb(0 0 0/.28),
            0 0 0 1px color-mix(in oklab,var(--primary) 10%,transparent);
```

`--primary` 深色主题下是 `#D9A441`。一个 hover 才出现的临时浮层拿到了全页最强的视觉权重，形态上又是「顶部一条高饱和色带」，语义上等同于警告横幅。

同一段还有三处顺带问题：

| 位置 | 问题 |
| --- | --- |
| `web/app.css:166-167` | `transform-origin` 是死代码，没有任何 transform/动画在用；且缺 `left`/`right` 两个 placement 的规则，而 `position()` 确实会产出这两个值（`app.js:2452`） |
| `web/app.css:163` | `backdrop-filter: blur(12px)` 在近乎不透明的填充后面完全看不见，但每次显示都让合成器多做一次全区域采样 |
| `web/app.js:2473-2477` | `hidden = false` 后先把 left/top 设成 `0px`，下一帧 `position()` 才定位 —— 每次悬停都会在视口左上角闪一帧全不透明的提示框 |

---

## 二、配色方案（v3）

### 前两版为什么废掉

**v1**：用 `light-dark()` 从 token 推导面板色。实测在 emerald 下渲染出暖褐色 `#372E26` —— 那是**浅色分支**的值，标题纯白、正文 `#C2C1BF` 也全是浅色分支的值。`light-dark()` 写在未注册的自定义属性（没有 `@property … syntax:"<color>"`）里解析不可靠，**本项目内直接禁用**。

**v2**：改成每主题字面色值，但仍然想把主题色**掺进面板底色**（三档浓度）。方向本身错了 —— 掺出来的是浑浊的橄榄灰，不是黑金。而且在 dark/oled 上，金色（H≈80°）与深蓝表面（H≈264°）在 OKLab 里接近对角，掺到 14% 时彩度被抵消到 C=0.005（`#4B4B4E`，纯中性），主色相直接消失。

### v3 的两条规则

**规则一：提示框是全主题最亮的一层。**实测八套配色的层级亮度：

```
light     bg 1.000  surface 0.973  surface-strong 0.925
dark      bg 0.163  surface 0.209  surface-strong 0.248
azure     bg 0.167  surface 0.213  surface-strong 0.258
emerald   bg 0.162  surface 0.208  surface-strong 0.244
violet    bg 0.162  surface 0.200  surface-strong 0.237
crimson   bg 0.157  surface 0.193  surface-strong 0.227
aurora    bg 0.164  surface 0.212  surface-strong 0.259
oled      bg 0.000  surface 0.151  surface-strong 0.202
```

层级越高亮度越高，八套一致。所以浅色主题下提示框是 **`#FFFFFF`**（比卡片 `0.973` 还亮，靠描边和投影从白底上分离出来）；深色系继续往 `--surface-strong` 之上抬，取 L≈0.310。

**规则二：面板底色一律中性，主题识别交给标题色和描边。**这正是 `app.css:74-76` 自己写的原则 —— 背景与卡片保持中性深色，主题色只出现在主色、强调色、选中态与描边上。黑金的黑是底、金是描边和字。

| 主题 | 面板（默认「浮起」） | 面板（「更沉」备选） | 标题 | 正文 |
| --- | --- | --- | --- | --- |
| light | `#FFFFFF` | `#FFFFFF` | `var(--primary-strong)` | `var(--muted)` |
| dark | `#293040` | `#131823` | 同上 | `mix(--ink 62%, --muted)` |
| azure | `#253046` | `#121A28` | 同上 | 同上 |
| emerald | `#2A313A` | `#13181E` | 同上 | 同上 |
| violet | `#2D2F3F` | `#151621` | 同上 | 同上 |
| crimson | `#352E32` | `#181416` | 同上 | 同上 |
| aurora | `#25323D` | `#121B22` | 同上 | 同上 |
| oled | `#303033` | `#040405` | 同上 | 同上 |

标题和正文都不需要写死 —— `--primary-strong` 和 `--muted` 每套主题都有。**唯一需要按主题写死的只有 `--tooltip-surface` 一个。**

「更沉」那一列面板比卡片还暗，读起来更像应用自己的深色壳，但面板/卡片对比只有 1.00–1.04，完全靠描边和投影分离，浮在卡片上时容易糊。v3 内部默认用「浮起」；v4 起整体改用下面的海克斯档，原型里「浮起」保留为备选。

### 对比度实测（默认档，8 套全过）

| 主题 | 面板 | vs 页面 | vs 卡片 | 标题 | 标题 c/r | 正文 c/r |
| --- | --- | --- | --- | --- | --- | --- |
| light | `#FFFFFF` | 1.00 | 1.08 | `#AB5300` | 5.3 | 7.2 |
| dark | `#293040` | 1.46 | 1.35 | `#F0BE5C` | 7.7 | 7.9 |
| azure | `#253046` | 1.45 | 1.33 | `#8AD2F5` | 7.9 | 8.0 |
| emerald | `#2A313A` | 1.47 | 1.36 | `#74E2B2` | 8.3 | 7.9 |
| violet | `#2D2F3F` | 1.47 | 1.37 | `#C4B0FF` | 6.9 | 7.9 |
| crimson | `#352E32` | 1.47 | 1.39 | `#F07A85` | 4.9 | 7.9 |
| aurora | `#25323D` | 1.47 | 1.34 | `#7EEAD6` | 9.1 | 8.2 |
| oled | `#303033` | 1.60 | 1.49 | `#F0BE5C` | 7.7 | 8.0 |

小字判定线 4.5:1，最紧的一档是 crimson 标题 4.9 —— 这也是面板必须压到 L=0.310 的原因，L=0.355 时 crimson 标题只有 4.18，不达标。

浅色的面板/页面对比是 1.00（同为纯白），完全靠描边加投影分离，和 Windows/macOS 浅色提示框的做法一致。

### v4：海克斯档（默认落地这一档）

v3 已经不难看了，但它是一块**中性灰蓝板子**，和「海克斯黑金」没什么关系 —— 用户原话「暗色就是海克斯黑金配色吗，也不太符合主题」其实还没被解决，v3 只是把主题色收缩到了标题和描边上。

真正的海克斯黑金长什么样：**近黑的底、一圈金色细框、金色标题、暖白正文。黑是底，金只在框和字上。**所以 v4 把面板往下压到比卡片还暗（`mix(--bg 70%, --surface)`），描边从 30% 提到 46% 让它真正撑起轮廓，正文从 `--muted` 换成 `--ink` 掺 28% 主色 —— 黑金下得到 `#E5D7C3` 那种羊皮纸暖白，和客户端里的 `#F0E6D2` 是一个家族。

每套配色各取自己的「黑 + 自己的金」，公式对 8 套统一，不是只给 dark 特调：

| 主题 | 面板 | 描边 | 标题 | 正文 |
| --- | --- | --- | --- | --- |
| 浅色 | `#FFFFFF` | `#DEB598` | `#AB5300` | `#392415` |
| 海克斯黑金 | `#0D1118` | `#726249` | `#F0BE5C` | `#E5D7C3` |
| 寒冰蓝 | `#0C121B` | `#3C698B` | `#8AD2F5` | `#C2DDF0` |
| 翡翠绿 | `#0D1116` | `#37735F` | `#74E2B2` | `#C0E4D3` |
| 星域紫 | `#0E0F17` | `#5E538B` | `#C4B0FF` | `#D5CFF5` |
| 血月红 | `#110E10` | `#793840` | `#F07A85` | `#EEBEC0` |
| 极光青 | `#0C1216` | `#3A7875` | `#7EEAD6` | `#C3E8E1` |
| 纯黑 OLED | `#000000` | `#6E593A` | `#F0BE5C` | `#E8D9C3` |

对比度（小字判定线 4.5:1）：

| 主题 | 标题 c/r | 正文 c/r | 描边/面板 | 面板/卡片 |
| --- | --- | --- | --- | --- |
| 浅色 | 5.3 | 14.6 | 1.88 | 1.08 |
| 海克斯黑金 | 11.0 | 13.4 | 3.20 | 1.06 |
| 寒冰蓝 | 11.3 | 13.3 | 3.21 | 1.07 |
| 翡翠绿 | 12.0 | 13.8 | 3.41 | 1.06 |
| 星域紫 | 10.0 | 12.8 | 2.81 | 1.06 |
| 血月红 | 7.2 | 11.7 | 2.24 | 1.05 |
| 极光青 | 13.1 | 14.3 | 3.70 | 1.07 |
| 纯黑 OLED | 12.2 | 15.1 | 3.15 | 1.04 |

余量比 v3 大得多（v3 最紧的 crimson 标题只有 4.9，这里是 7.2），因为底压黑了。

**注意规则二在 v4 里的边界**：主题色掺进**文字**是安全的（`--ink` 近中性，掺主色只会被稀释成暖白/冷白），掺进**底色**才会翻车。v2 死在后者，v4 做的是前者，两者不冲突。

**两条要接受的取舍**：

1. **面板/卡片只有 1.04–1.08。**提示框浮在卡片上时，底色几乎和卡片一样，轮廓完全由那圈金框和投影划出来 —— 这正是客户端提示框的做法，但如果实机上觉得糊，把面板整体提亮到 `mix(--bg 40%, --surface)` 即可，标题对比仍全部达标。
2. **纯黑 OLED 的面板算出来就是 `#000000`**，和页面底同色。这在 OLED 上是刻意的（不糊像素、最像客户端），只有那圈金框和轮廓，投影完全看不见。如果不接受，改成 `#050506` 就能拿回一点分离，代价是 OLED 上出现一块非纯黑区域。

---

## 三、CSS 改动（`web/app.css`）

### 1. 在 `:root` 块末尾（`--hex-line` 之后、第 33 行下方）追加

```css
  /* 提示框浮层：近黑面板 + 主色细框 + 主色标题。底色不掺主色。 */
  --tooltip-surface: #FFFFFF;
  --tooltip-ink: var(--ink);
  --tooltip-muted: color-mix(in oklab, var(--ink) 80%, var(--primary));
  --tooltip-title: var(--primary-strong);
  --tooltip-line: color-mix(in oklab, var(--primary) 40%, var(--line));
  --tooltip-shadow-cast: rgb(11 14 20 / .20);
  --tooltip-shadow-contact: rgb(11 14 20 / .12);
```

### 2. 7 个深色主题块各加一行

在对应的 `:root[data-theme="…"]` 块里追加：

```css
:root[data-theme="dark"]    { … --tooltip-surface: #0D1118; }
:root[data-theme="azure"]   { … --tooltip-surface: #0C121B; }
:root[data-theme="emerald"] { … --tooltip-surface: #0D1116; }
:root[data-theme="violet"]  { … --tooltip-surface: #0E0F17; }
:root[data-theme="crimson"] { … --tooltip-surface: #110E10; }
:root[data-theme="aurora"]  { … --tooltip-surface: #0C1216; }
:root[data-theme="oled"]    { … --tooltip-surface: #000000; }
```

### 3. 深色系共用的前景与投影，加一条分组规则

放在 7 个主题块之后、`:root[data-density=…]`（`:144`）之前：

```css
/* 深色系主题共用的提示框前景与投影；面板底色在各主题块内单独给值 */
:root[data-theme="dark"], :root[data-theme="azure"], :root[data-theme="emerald"],
:root[data-theme="violet"], :root[data-theme="crimson"], :root[data-theme="aurora"],
:root[data-theme="oled"] {
  --tooltip-ink: var(--ink);
  --tooltip-muted: color-mix(in oklab, var(--ink) 72%, var(--primary));
  --tooltip-title: var(--primary-strong);
  --tooltip-line: color-mix(in oklab, var(--primary) 46%, var(--line));
  --tooltip-shadow-cast: rgb(0 0 0 / .66);
  --tooltip-shadow-contact: rgb(0 0 0 / .45);
}
```

特异度与 7 个主题块相同（都是 0,2,0），但源码在后，所以能覆盖 `:root` 的浅色值；它不声明 `--tooltip-surface`，不会覆盖各主题块的面板色。

### 4. 跟随系统深色、且没有显式 `data-theme` 的情况

`@media (prefers-color-scheme: dark) { :root:not([data-theme="light"]) { … } }`（`:61-72`）匹配的是**没有 data-theme 属性**的场景，上面第 3 条那条分组规则够不着它。所以这个块里要把 7 个变量补全（面板取 dark 档的值）：

```css
    --tooltip-surface: #0D1118;
    --tooltip-ink: var(--ink);
    --tooltip-muted: color-mix(in oklab, var(--ink) 72%, var(--primary));
    --tooltip-title: var(--primary-strong);
    --tooltip-line: color-mix(in oklab, var(--primary) 46%, var(--line));
    --tooltip-shadow-cast: rgb(0 0 0 / .66);
    --tooltip-shadow-contact: rgb(0 0 0 / .45);
```

这个块本来就是整套深色调色板的副本，加这几行与它现有的写法一致。

> 两条红线：**不要用 `light-dark()`**（v1 就死在这），**不要把 `--primary` 掺进 `--tooltip-surface`**（v2 死在这，且会在 dark/oled 上抵消彩度）。掺进 `--tooltip-muted`（文字）没问题，那是在给近中性的 `--ink` 调色温。混色统一用 `in oklab`，不要用 `in oklch` —— 极坐标插值在近中性色上不稳定。

### 5. 把 `web/app.css:163-167` 整段替换为

```css
.global-tooltip { position: fixed; z-index: 55; width: max-content; max-width: min(360px,calc(100vw - 24px)); padding: 7px 11px 8px; color: var(--tooltip-muted); background: var(--tooltip-surface); border: 1px solid var(--tooltip-line); border-radius: var(--radius-sm); box-shadow: 0 14px 34px var(--tooltip-shadow-cast),0 2px 8px var(--tooltip-shadow-contact); font-size: 11.5px; line-height: 1.5; overflow-wrap: anywhere; pointer-events: none; text-align: left; opacity: 0; transform: scale(.97) translateY(2px); transition: opacity 120ms ease-out, transform 120ms ease-out; }
.global-tooltip[data-shown="true"] { opacity: 1; transform: none; }
.global-tooltip[data-layout="titled"] { display: grid; gap: 3px; padding: 9px 12px 10px; }
.global-tooltip .tooltip-title { color: var(--tooltip-title); font-size: 12px; font-weight: 600; letter-spacing: -.005em; }
.global-tooltip[data-layout="single"] .tooltip-title { color: var(--tooltip-ink); font-size: 11.5px; font-weight: 500; }
.global-tooltip .tooltip-body { color: var(--tooltip-muted); font-size: 11.5px; line-height: 1.58; white-space: pre-line; }
.global-tooltip[data-size="compact"] { max-width: min(280px,calc(100vw - 24px)); }
.global-tooltip[data-size="item"] { max-width: min(420px,calc(100vw - 24px)); }
.global-tooltip[data-placement="top"] { transform-origin: bottom center; }
.global-tooltip[data-placement="bottom"] { transform-origin: top center; }
.global-tooltip[data-placement="left"] { transform-origin: right center; }
.global-tooltip[data-placement="right"] { transform-origin: left center; }
@media (prefers-reduced-motion: reduce) {
  .global-tooltip, .global-tooltip[data-shown="true"] { transition-duration: 1ms; transform: none; }
}
```

改动要点：删 `border-top` 金条、删金色描边与金色外圈发光、删 `backdrop-filter`；`white-space: pre-line` 从容器下放到 `.tooltip-body`（标题永远单行，用不到）；圆角 `7px` → `var(--radius-sm)`（8px），与全站一致；补上 left/right 两个 `transform-origin`。

---

## 四、JS 改动（`web/app.js`，`setupFloatingTooltips`）

### 1. `hide()`（`:2430-2436`）—— 在 `tooltip.hidden = true;` 前插一行

```js
      delete tooltip.dataset.shown;
      tooltip.hidden = true;
```

### 2. `position()` —— 两个分支都要点亮

`position()` 有两条出口：`tooltipSide === "menu"` 分支在 `:2455` 处 `return`，默认分支落在 `:2463`。**两处都要**在 `return` / 函数结尾前加：

```js
      tooltip.dataset.shown = "true";
```

漏掉 menu 分支，区服菜单里的提示框会永远停在 `opacity: 0`。

### 3. `show()`（`:2465-2478`）—— 把 `tooltip.textContent = content;` 那一行替换成

```js
      const lineBreak = content.indexOf("\n");
      const title = lineBreak < 0 ? content : content.slice(0, lineBreak);
      const body = lineBreak < 0 ? "" : content.slice(lineBreak + 1).trim();
      const titleNode = document.createElement("strong");
      titleNode.className = "tooltip-title";
      titleNode.textContent = title;
      const nodes = [titleNode];
      if (body) {
        const bodyNode = document.createElement("span");
        bodyNode.className = "tooltip-body";
        bodyNode.textContent = body;
        nodes.push(bodyNode);
      }
      tooltip.replaceChildren(...nodes);
      tooltip.dataset.layout = body ? "titled" : "single";
```

紧随其后的 `tooltip.dataset.size = next.dataset.tooltipSize || "";` **保持原样不动**（见下节测试约束）。

> **安全红线**：提示框内容包含远端玩家昵称、OP.GG 返回的说明文本。必须逐节点 `textContent` 赋值，**任何情况下都不能改用 `innerHTML`**。现在这版是 XSS 安全的，改坏了就不是了。

---

## 五、三条不能碰的测试约束

| 测试 | 约束 |
| --- | --- |
| `web/champions.test.cjs:289` | `tooltip.dataset.size = next.dataset.tooltipSize \|\| ""` 这行必须原样存在 |
| `web/champions.test.cjs:290` | `.global-tooltip[data-size="compact"] { … max-width: min(280px …}` 规则必须保留 |
| `web/champions.test.cjs:474` | 正则 `/\.global-tooltip\s*\{[^}]*position:\s*fixed[^}]*z-index:\s*55/s` —— `position: fixed` 与 `z-index: 55` 必须在**同一个** `.global-tooltip { }` 块内、且 `position` 在前 |

上面给的 CSS 已经把 `position: fixed; z-index: 55;` 放在块首两条，满足第三条。

---

## 六、一条需要配套改文案的内容

`web/app.js:5-12` 的 `CN_SERVER_MERGE_NOTE` 是全项目唯一一条**首行不是标题**的提示内容 —— 六行等权的大区列表。分层之后首行「联盟一区：…」会被错误加粗成标题。

已核对过的其余内容全部天然符合「首行即标题」：

- `web/champions.js:649-656` `assetTooltip` → `[name, description, ...numbers].join("\n")`
- `web/gameplay.js:2044` / `:2571` → `[name, description].join("\n")`
- `web/gameplay.js:2596` → `[augment.name, rarity + "级", description].join("\n")`
- `web/gameplay.js:2260-2270` `abilityTooltip` → 首行 `` `${key} · ${ability.name}` ``

建议改法（顺带也让这块内容有了说明，现在它是一堆没有抬头的地名）：在 `CN_SERVER_MERGE_NOTE` 最前面加一行

```
国服大区合并对照
```

原型最后一张卡片里左右两个例子就是这条改动的前后对比。

---

## 七、验收清单

改完请确认：

1. `node --test web/*.test.cjs desktop/*.test.cjs` 仍是 59 pass / 0 fail。
2. 前端资源是 `go:embed` 的，**必须重新 `go build`** 才能在桌面壳里看到效果。
3. 8 套配色逐个切一遍，提示框都能从背景上分离出来（纯黑 OLED 那套面板就是 `#000000`，只靠金框划轮廓，属于预期行为）。
4. **切到翡翠绿，用开发者工具确认面板计算值是 `rgb(13, 17, 22)`、标题是 `rgb(116, 226, 178)`、正文是 `rgb(192, 228, 211)`。**如果面板又是暖褐色，说明还有残留的 `light-dark()` 在漏浅色分支。
5. **切到浅色主题，提示框应该是白色面板 + 深色字**，不是深色浮层。
6. 区服切换菜单里的提示框（`data-tooltip-side="menu"`，走 left/right 分支）能正常淡入 —— 这是最容易漏的一处。
7. 悬停时视口左上角不再闪一下。
8. 系统开启「减少动态效果」后，提示框仍能立即显示（不是不显示）。
