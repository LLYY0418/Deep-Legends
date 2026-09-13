"use strict";

// ★ 这几个常量必须与 web/app.js 里的同名常量逐字一致：真正执行缩放的是渲染进程
// （:root 的 --ui-zoom + .app-frame 的 CSS zoom），外壳这份只用来在渲染进程汇报之前
// 先给标题栏覆盖层一个正确的初值，并校验渲染进程送上来的倍率。
// desktop/ui-scale.test.cjs 里有断言防止两边漂移。
//
// 自动缩放是连续的（不取档位）：1920×1080 恰好是设计基准 → 1.00，只放大不自动缩小。
// UI_SCALE_STEPS 只是设置页里手动锁档的可选值（含一个手动 90% 档，自动模式到不了）。
const UI_SCALE_STEPS = Object.freeze([0.9, 1, 1.1, 1.25, 1.4, 1.5, 1.75, 2, 2.25, 2.5]);
const UI_SCALE_BASE_WIDTH = 1920;
const UI_SCALE_BASE_HEIGHT = 900;
const UI_SCALE_MIN = 1;
const UI_SCALE_MAX = 2.5;

// Content bounds are DIP before page zoom. Electron already accounts for DPI.
function autoScaleFor({ width, height } = {}) {
  const raw = Math.min(Number(width) / UI_SCALE_BASE_WIDTH, Number(height) / UI_SCALE_BASE_HEIGHT);
  if (!Number.isFinite(raw)) return 1;
  // 量化到 1%，与渲染进程保持同一口径。
  return Math.min(UI_SCALE_MAX, Math.max(UI_SCALE_MIN, Math.round(raw * 100) / 100));
}

function normalizeScale(value) {
  if (value === "auto") return "auto";
  if ((typeof value !== "number" && typeof value !== "string") || String(value).trim() === "") return "auto";
  const number = Number(value);
  if (!Number.isFinite(number)) return "auto";
  // 手动锁档只接受档位表里的值；平手时取较小的一档。
  return UI_SCALE_STEPS.reduce((nearest, step) => Math.abs(step - number) < Math.abs(nearest - number) ? step : nearest, 1);
}

module.exports = {
  UI_SCALE_STEPS,
  UI_SCALE_BASE_WIDTH,
  UI_SCALE_BASE_HEIGHT,
  UI_SCALE_MIN,
  UI_SCALE_MAX,
  autoScaleFor,
  normalizeScale,
};
