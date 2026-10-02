"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const appSource = fs.readFileSync(path.join(__dirname, "app.js"), "utf8");
const css = fs.readFileSync(path.join(__dirname, "app.css"), "utf8");

function extract(name) {
  const start = appSource.indexOf(`function ${name}(`);
  assert.notEqual(start, -1, `missing ${name}`);
  const open = appSource.indexOf("{", appSource.indexOf(")", start));
  let depth = 0;
  for (let i = open; i < appSource.length; i++) {
    if (appSource[i] === "{") depth++;
    if (appSource[i] === "}" && --depth === 0) return appSource.slice(start, i + 1);
  }
  throw new Error("unbalanced");
}
function load() {
  const consts = appSource.match(/const LOOT_ICON_BOX = \d+;\n\s*const LOOT_ICON_TARGET = \d+;\n\s*const LOOT_ICON_MAX_SIDE = \d+;/)[0];
  return new Function(`${consts}\n${extract("lootIconFit")}\nreturn { lootIconFit, LOOT_ICON_BOX, LOOT_ICON_TARGET };`)();
}
function alphaFor(size, box) {
  const alpha = new Uint8Array(size * size);
  for (let y = box.y; y < box.y + box.h; y++) for (let x = box.x; x < box.x + box.w; x++) alpha[y * size + x] = 255;
  return alpha;
}
// 蓝色精粹：256 图里主体 107×169；换算到 128 采样图。
const BLUE = { x: 37, y: 11, w: 54, h: 85 };

test("R146 blue essence keeps its size and other icons match its visual size", () => {
  const { lootIconFit, LOOT_ICON_BOX } = load();
  const size = 128;
  const measure = (box) => {
    const fit = lootIconFit(alphaFor(size, box), size);
    const k = LOOT_ICON_BOX / size * fit.scale;
    return { mean: Math.sqrt(box.w * box.h) * k, side: Math.max(box.w, box.h) * k, scale: fit.scale };
  };
  const blue = measure(BLUE);
  // 蓝色精粹在旧版 CSS 里是 1.65 倍，归一后不应明显变化。
  assert.ok(Math.abs(blue.scale - 1.65) < 0.1, `blue scale=${blue.scale}`);
  // 实心方块、宽扁宝箱、瘦高钥匙：几何平均边长与蓝色精粹一致，且最大边不超过上限。
  for (const box of [{ x: 20, y: 20, w: 90, h: 90 }, { x: 15, y: 16, w: 99, h: 92 }, { x: 42, y: 26, w: 54, h: 75 }]) {
    const m = measure(box);
    assert.ok(Math.abs(m.mean - blue.mean) < 1 || m.side <= 78.01, JSON.stringify(m));
    assert.ok(m.side <= 78.01);
  }
  // 实心方块显著小于按最大边归一的结果：约为 60px 而不是 76px。
  assert.ok(measure({ x: 20, y: 20, w: 90, h: 90 }).side < 65);
});

test("R146 fit centres the visible box and leaves opaque or empty images alone", () => {
  const { lootIconFit } = load();
  const size = 128;
  const fit = lootIconFit(alphaFor(size, { x: 10, y: 20, w: 40, h: 40 }), size);
  assert.ok(fit.dx > 0 && fit.dy > 0, "偏左上的主体要向右下移回中心");
  assert.equal(lootIconFit(new Uint8Array(size * size), size), null);
  assert.equal(lootIconFit(new Uint8Array(size * size).fill(255), size), null, "自带不透明底的图不做归一");
});

test("R146 css no longer carries per-item icon scale hacks and the loader normalizes on load", () => {
  assert.doesNotMatch(css, /\.loot-card\.loot-[a-z-]+ \.loot-art img \{ --loot-icon-scale/);
  assert.match(css, /translate\(var\(--loot-icon-dx,0px\),var\(--loot-icon-dy,0px\)\) scale\(var\(--loot-icon-scale\)\)/);
  assert.match(extract("loadNextLootImage"), /normalizeLootIcon\(image\)/);
});
