# 紧急修复：海克斯图标变成纯色方块

> 2026-08-22 · R8 §A 落地后的回归 · **根因是我工单示例代码写得不严谨**

---

## 现象

海克斯图标全部变成**纯色方块**（银色/金色/棱彩渐变的实心矩形），图标形状完全消失。

---

## 根因：`escapeHTML` 把 `&` 转成 `&amp;`，CSS `url()` 不解 HTML 实体

### 链路

`web/champions.js:148-157` `assetImage()`：

```js
const image = imageURL(asset?.source, asset?.path);
const maskStyle = isAugment && !isColoredAugment && asset?.source && asset?.path
  ? ` style="--augment-mask: url('${escapeHTML(image)}')"`   // ← 问题在这
  : "";
```

`imageURL()`（champions.js:130）产出：
```
/api/champion-asset?source=cdragon&path=%2Flol-game-data%2F...%2Fspellwake_small.png
```

`escapeHTML()`（champions.js:120，用 `textContent`→`innerHTML`）把 `&` 转义：
```
/api/champion-asset?source=cdragon&amp;path=%2F...
```

**这个字符串被写进 HTML 的 `style` 属性。** HTML 实体只在 HTML 解析层解码一次得到属性值，但 CSS 解析器拿到的 `url()` 内容是**原样字面量** `&amp;`——CSS 不认识 HTML 实体。

结果：请求地址变成 `?source=cdragon&amp;path=...`，`path` 参数丢失 → `/api/champion-asset` 返回错误/404 → **mask 图加载失败**。

### 为什么失败会变成纯色方块

CSS `mask-image` 加载失败时，规范行为等同于**没有遮罩**（全不透明），于是 `::after` 的 `background: var(--quality-fill)` 整块渲染出来 → 纯色渐变方块。

`web/champions.css:93` `.has-augment-mask > img { visibility: hidden }` 又把真图标藏了，所以形状彻底消失。

---

## 改法

### 方案（推荐）：CSS 上下文用专用转义，不要用 `escapeHTML`

`escapeHTML` 是给 **HTML 文本节点**用的，不适用于 CSS `url()`。新增一个 CSS 字符串转义：

```js
// 仅转义 CSS 字符串字面量里的危险字符，不碰 & 
function cssURL(value) {
  return String(value ?? "").replace(/[\\'"()\s]/g, (c) => "\\" + c);
}
```

然后：
```js
const maskStyle = isAugment && !isColoredAugment && asset?.source && asset?.path
  ? ` style="--augment-mask: url(&quot;${cssURL(image)}&quot;)"`
  : "";
```

**注意**：`style` 属性值本身仍需 HTML 转义（防止 `"` 提前闭合属性），但只能转 `"` → `&quot;`，**绝不能转 `&`**。

### 更稳的替代方案：不走 style 属性，改用 img 本身做 mask 源

避开属性转义问题——用 `-webkit-mask-image: -webkit-element()` 或直接让 `<img>` 可见并用 `filter` 上色：

```css
.augment-icon.has-augment-mask > img {
  visibility: visible;
  /* 纯白剪影 → 用 drop-shadow 叠色，或用 background-clip */
}
```

但白色剪影用 filter 上色效果不如 mask 干净，**建议优先修 A 方案的转义**。

---

## 验收

1. 海克斯图标恢复形状，且按品质着色（银/金/棱彩渐变）
2. DevTools → Network 里 `/api/champion-asset` 请求的 query **必须是 `&` 而不是 `&amp;`**，状态 200
3. DevTools → Elements 检查任一 `.has-augment-mask` 元素，`--augment-mask` 的值展开后是可点击的有效 URL
4. `arena_2026_s2*` 系列彩色图标仍走原图直显（`isColoredAugment` 分支）
5. 加测试锁死：断言生成的 style 字符串**不含** `&amp;`

---

## 教训（工单方的责任）

R8 §A 我给的示例是：
```js
style="--augment-mask:url('${imageURL(asset?.source, asset?.path)}')"
```
只在"注意"里写了「`imageURL()` 拼出的地址要注意 CSS `url()` 里的引号转义」，**没有点明 `&` 在 style 属性中的双重解码陷阱**，也没说 `escapeHTML` 不适用于 CSS 上下文。

后续凡是涉及"把 URL 写进 CSS 属性"的工单，必须显式给出转义函数，不能只写 `${url}`。

---

# 追加(19:50):真正的根因是 CSP,转义只是次要 bug

## 沙箱真机复现结论

用 Linux 版服务端 + 无头 Chromium 完整复现(前端带 cssURL 修复的当前源码):

```
mask 图片请求      → 200 image/png     ✅ 服务端没问题
style 属性解析     → 变量为空          ❌ 被丢弃
computed maskImage → none              ❌ mask 未生效
```

而同样的 style 属性写法,在**不带 CSP 的空白页面**里五种变体全部正常。

## 根因

`main.go` `securityHeaders()`:
```
Content-Security-Policy: ... style-src 'self' ...
```
**没有 `'unsafe-inline'` → 浏览器丢弃页面上所有 `style="..."` 属性。**
`--augment-mask` 从未生效过 → `mask: var(--augment-mask)` 无效 → mask 为 none → `::after` 的品质色整块渲染 = 纯色方块。

之前的 `&amp;` 转义问题是真 bug,但即使修了也会被 CSP 拦住——这就是"改了还是方块"的原因。

## 修复方案(已在真实 CSP 页面验证,效果图 mask-cssom.png)

**遵循本项目既有模式**:`applyRenderedMetricStyles()`(champions.js:1552)就是为绕 CSP 存在的——渲染只写 `data-*`,渲染后用 CSSOM(`el.style.setProperty`,CSP 允许)统一应用。

1. `assetImage()`:去掉 `style="--augment-mask:..."`,改为输出
   ```js
   data-augment-mask="${escapeHTML(image)}"
   ```
   (data 属性回到 HTML 上下文,`escapeHTML` 在这里是正确的)

2. `applyRenderedMetricStyles()` 增加一段:
   ```js
   for (const el of root.querySelectorAll("[data-augment-mask]")) {
     el.style.setProperty("--augment-mask", `url("${el.dataset.augmentMask}")`);
   }
   ```
   `dataset` 取值时 HTML 实体已解码,`&` 是真 `&`,无转义问题。

3. `cssURL()` 函数与 style 属性拼接整段删除(死代码)。

4. **改完必须重新编译**(go:embed),否则页面永远看不到效果。

## 真机验证记录

- 注入与修复方案完全一致的节点到真实页面(真 CSP、真 CSS、真接口)
- `--augment-mask` 变量读回正常,computed mask = url(...)
- 截图:三把剑图标分别渲染为银渐变/金渐变/蓝紫棱彩渐变,形状完整
- 像素采样:金档剪影区 687 彩色像素、棱彩 144,深底背景正常

## 验收

1. 图标呈品质色渐变剪影,不再是纯色方块,也不是白色
2. DevTools Elements:`[data-augment-mask]` 元素的 inline style 里能看到 JS 设上去的 `--augment-mask`
3. Console 无 CSP violation 报错
4. 测试:断言 `assetImage` 产物含 `data-augment-mask` 且**不含** `style="--augment-mask`;断言 `applyRenderedMetricStyles` 处理该属性(变异验证)

---

# 追加(20:20):mask 上色已生效,遗留两项——棱彩配色 + 图鉴说明

## ① 棱彩渐变偏蓝,应以紫为主

### 现状
`web/champions.css:96`:
```css
--quality-fill: linear-gradient(150deg,#C77DFF,#5AA9FF 55%,#FF8AC7);
```
蓝色 `#5AA9FF` 的停靠点在 55%,在 42-52px 的小图标上占据视觉主体 → 整体读作"蓝色",与棱彩徽章/边框的紫色(`#C77DFF`)不统一。这个配色是我 R8 工单里给的,责任在我。

### 改法
紫色为主、蓝粉只作点缀,与 `.is-prismatic` 边框同系:
```css
--quality-fill: linear-gradient(160deg,#E0B3FF 0%,#C77DFF 45%,#9C86FF 75%,#FF8AC7 100%);
```
(GPT 可微调停靠点,验收标准是"第一眼读作紫色、与棱彩徽章同系",不是精确色值)

## ② 图鉴说明全空("暂无说明")——沙箱实测已定位,非熔断问题

### 实测证据(沙箱跑真实服务端)

```
GET /api/champions/augments → 200, rows: 208
description+tooltip 都为空的: 208 / 208
```

R8-C 的熔断修复已落地且与此无关——说明数据链路是:

`loadAugments`(champions.go:1866-1883)→ hexdata 出行 → 说明从 `loadCommunityDragonAugments` 的 catalog 按 **ID** 合并(`rows[index].Description = meta.Description`)

### 根因:两个上游都覆盖不到海斗 ID

实测(2026-08-22):
1. `cherry-augments.json`(基础包)**根本没有 description 字段**(仅 id/nameTRA/icon/rarity 六个字段)→ base 全空
2. 说明唯一来源是 `/latest/cdragon/arena/zh_cn.json` 补充包,但它**只覆盖斗魂 ID 1~405,id≥1000 的海斗条目零覆盖**
3. 图鉴 208 条全部是海斗 ID(≥1000)→ 按 ID 合并全落空 → 208 条全空

### 尝试过的映射,都不可靠(不要用)

- `id-1000` 直减:370 条海斗里名称一致仅 105,错配 43(例:1323"残忍"-1000=323 是"地狱三头犬")
- `augmentNameId` 去 `ARAM_` 前缀匹配 `apiName`:117/370
- 中文名匹配:112/370 —— **大部分海斗海克斯是专属条目**(空投熊/大法师/吃过路兵等),斗魂数据集里根本不存在

### 建议改法(按优先级)

**方案一(推荐,待实机验证)**:复用对局内已有的 LCU 链路。
`gameplay.go:3691` 已经在客户端连接时读 `LCU /lol-game-data/assets/v1/cherry-augments.json`,`gameplayAugmentRaw` 结构里有 `description`/`tooltip` 字段(gameplay.go:3447-3448)——**客户端本地版本的这份文件带说明**(客户端自己要渲染 tooltip),web 版 CDragon 镜像被裁掉了。
改法:`loadAugments`(champions.go)的说明合并不要直连 `loadCommunityDragonAugments`,改走对局内同款"LCU 优先、web 兜底"的 catalog。客户端在线时说明就有了。
**验收前提**:需要实机(客户端运行中)确认 LCU 版文件确实带 description——沙箱无客户端,无法代验。

**方案二(兜底文案)**:LCU 不在线时,对海斗专属条目显示"启动 LOL 客户端后可查看说明"而不是"暂无说明",把能通过 arena_zh 名称匹配到的 ~112 条(名称完全一致才认)先填上。

**红线**:不要为了说明去逐条抓 hexdata 的 208 个 augment 详情页(请求量红线),也不要用 id-1000 硬映射(43 条错配会把"残忍"的说明安到"地狱三头犬"头上)。

### 验收
- 客户端在线时:图鉴详情面板显示中文说明
- 客户端离线时:海斗专属条目显示引导文案,非"暂无说明"的死状态
- 加测试:锁死"禁止 id-1000 直减映射"(变异验证)
