# 紧急修复:海克斯图标应该用 Riot 官方全彩大图,不是自绘渐变

> 2026-08-22 · **推翻我此前"上游只有白描素材、必须 CSS 上色"的结论。这是我的检索错误。**

---

## 一、我错在哪(必须先说清)

我在 R8 §A 断言"CommunityDragon 只有纯白 mask 素材,必须用 CSS mask + 品质色渐变上色",并给出了自绘的 `linear-gradient`。**这个结论是错的。**

错因:我测 `_large.png` 时用的前缀是
```
/latest/plugins/rcp-be-lol-game-data/global/default/assets/ux/cherry/augments/icons/xxx_large.png  → 404
```
而正确前缀是
```
/latest/game/assets/ux/cherry/augments/icons/xxx_large.png  → 200
```

一个 404 让我认定"没有大图",于是整条 mask 上色路线都是在补救一个**本不存在的问题**。用户质疑"你自己在编什么样式"——是对的。

**线索其实早就出现过**:R8 §E 实测 YOUR.GG 时,返回里明明写着
`"image":"https://raw.communitydragon.org/latest/game/assets/ux/cherry/augments/icons/mysticpunch_large.png"`
我看到了却没追。

---

## 二、实测证据(2026-08-22)

### 大图是 Riot 官方全彩原图
```
200  mysticpunch_large.png      PNG 256x256 8-bit/color RGBA
200  heavyhitter_large.png      PNG 256x256 8-bit/color RGBA
200  bladewaltz_large.png       PNG 256x256 8-bit/color RGBA
200  spellwake_large.png        PNG 256x256 8-bit/color RGBA
200  infernalconduit_large.png  PNG 256x256 8-bit/color RGBA
```

逐像素验证(对比 `_small` 的"亮度恒 255 纯白"):
```
L_bladewaltz.png       彩色占比 37.6% | 亮度 71-255
L_heavyhitter.png      彩色占比 30.3% | 亮度 60-203
L_infernalconduit.png  彩色占比 28.4% | 亮度 72-255
L_mysticpunch.png      彩色占比 23.2% | 亮度 72-255
L_spellwake.png        彩色占比 38.6% | 亮度 73-255
```
**棱彩的虹彩渐变、黄金的暖光都是 Riot 原图自带的**,比任何自绘渐变都准确。

### 覆盖率:全量 655 条
```
有 _large 的:  482 (73.6%)
只有 _small:   173  (海斗 116 + 斗魂 57)
```
目录实测:`cherry/` 503 个 png、`kiwi/` 213 个 png,两个目录都有 large。

---

## 三、改法

### 关键:路径转换本来就是对的

`communityDragonGameAssetPath()`(champions_structured.go)已经在产出 `/latest/game/...`。
**唯一问题是 JSON 字段名叫 `augmentSmallIconPath`,值是 `_small`,没人换后缀。**

### 步骤

1. **优先取大图**:在海克斯图标路径产出处(`normalizeGameplayAugments` / `loadCommunityDragonAugments` / `champions_structured.go:814` 附近),把 `_small.png` 换成 `_large.png`

2. **兜底**:那 173 条只有 small 的,`<img>` 的 `onerror` 或后端 HEAD 探测失败时回落 `_small`,回落的才走 mask 上色(保留现有 mask 逻辑作为 fallback,不要整段删)

3. **CSS 相应调整**:
   - 命中大图的图标:正常 `<img>` 显示,`visibility` 不隐藏,**不加 mask**
   - 回落 small 的:维持现有 `.has-augment-mask` 路径
   - 建议用一个 `data-augment-fallback` 标记区分两条路径

4. **棱彩渐变配色**:命中大图后不再需要自绘渐变。仅 fallback 路径保留,顺手把 `css:96` 的 `#5AA9FF 55%` 改成紫主蓝辅(见 HOTFIX-0822-mask.md 追加节 ①)

5. **改完必须重编二进制**(go:embed)

---

## 四、验收

1. 海克斯图标显示 **Riot 原图配色**(棱彩虹彩渐变、黄金暖光),不是单色渐变剪影
2. DevTools Network:`champion-asset` 请求的 path 参数以 `_large.png` 结尾,状态 200
3. 那 173 条无大图的条目不出现破图——回落 small + mask 上色
4. 测试:断言图标路径优先 `_large`;断言 fallback 链路存在(变异验证)

---

## 五、教训

- 探测上游资源时,**前缀写错一个字母就会得出"资源不存在"的错误结论**。以后测 404 必须换 2-3 种路径形态交叉验证。
- **别人家产品的响应里就带着正确答案**(YOUR.GG 的 image 字段),实测数据要通读,不能只挑自己关心的字段看。
- 凡是要"自己画样式还原游戏内观感"的方案,先反问一句:官方素材真的没有吗?
