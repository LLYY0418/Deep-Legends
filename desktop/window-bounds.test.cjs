"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { windowBoundsForWorkArea } = require("./window-bounds.cjs");

const boundsSource = fs.readFileSync(path.join(__dirname, "window-bounds.cjs"), "utf8");
const mainSource = fs.readFileSync(path.join(__dirname, "main.cjs"), "utf8");
const appStyles = fs.readFileSync(path.join(__dirname, "..", "web", "app.css"), "utf8");

function loadBoundsModule(source) {
  const module = { exports: {} };
  Function("module", "exports", source)(module, module.exports);
  return module.exports;
}

function assertDesktopCallerContract(main = mainSource, css = appStyles) {
  // The require moved into loadDeferredModules() so it no longer runs before
  // the splash window is created; what this guard actually protects is that
  // main.cjs still gets its sizing from the shared helper instead of
  // reimplementing it, so match the require wherever it lives.
  assert.match(main, /\{ windowBoundsForWorkArea \} = require\("\.\/window-bounds\.cjs"\)/);
  assert.match(main, /const display = screen\.getPrimaryDisplay\(\);[\s\S]*return windowBoundsForWorkArea\(display\.workAreaSize\)/);
  assert.match(css, /\.topbar\s*\{[^}]*-webkit-app-region:\s*drag/s);
}

// 渲染进程接管了连续等比缩放之后，首窗不需要再靠"开得很大"来换可读性——开大只会
// 换来两侧的留白。大屏按工作区 72%/80% 打开；小屏上 72% 连 1920 设计基准都放不下，
// 退回 92%/94% 兜底（否则要把界面缩到 0.72 倍，字号会小到读不了）。
test("desktop window opens at 72/80 percent of the work area on roomy screens", () => {
  assert.deepEqual(windowBoundsForWorkArea({ width: 2560, height: 1440 }), { width: 1843, height: 1152 });
  assert.deepEqual(windowBoundsForWorkArea({ width: 3440, height: 1440 }), { width: 2200, height: 1152 });
  assert.deepEqual(windowBoundsForWorkArea({ width: 3000, height: 700 }), { width: 2160, height: 680 });
});

test("small screens fall back to the design-width floor instead of 72 percent", () => {
  assert.deepEqual(windowBoundsForWorkArea({ width: 1920, height: 1080 }), { width: 1766, height: 1010 });
  assert.deepEqual(windowBoundsForWorkArea({ width: 1366, height: 768 }), { width: 1257, height: 722 });
  assert.deepEqual(windowBoundsForWorkArea({ width: 1200, height: 2000 }), { width: 1104, height: 1400 });
});

test("desktop window clamps each dimension independently", () => {
  assert.deepEqual(windowBoundsForWorkArea({ width: 800, height: 500 }), { width: 1080, height: 680 });
  assert.deepEqual(windowBoundsForWorkArea({ width: 3840, height: 2160 }), { width: 2200, height: 1400 });
  assert.deepEqual(windowBoundsForWorkArea({ width: 5120, height: 2880 }), { width: 2200, height: 1400 });
});

test("desktop main and topbar keep the shared sizing and drag-region callers", () => {
  assertDesktopCallerContract();
});

test("R47 desktop caller and high-resolution mutation probes fail", () => {
  const widthRegression = loadBoundsModule(boundsSource.replace("const WINDOW_MAX_WIDTH = 2200;", "const WINDOW_MAX_WIDTH = 1680;"));
  assert.throws(() => assert.deepEqual(widthRegression.windowBoundsForWorkArea({ width: 3840, height: 2160 }), { width: 2200, height: 1400 }));
  const heightRegression = loadBoundsModule(boundsSource.replace("const WINDOW_MAX_HEIGHT = 1400;", "const WINDOW_MAX_HEIGHT = 1050;"));
  assert.throws(() => assert.deepEqual(heightRegression.windowBoundsForWorkArea({ width: 3840, height: 2160 }), { width: 2200, height: 1400 }));
  const proportion = loadBoundsModule(boundsSource.replace("WINDOW_WIDTH_RATIO = 0.72", "WINDOW_WIDTH_RATIO = 0.95"));
  assert.throws(() => assert.deepEqual(proportion.windowBoundsForWorkArea({ width: 2560, height: 1440 }), { width: 1843, height: 1152 }));
  // 去掉小屏兜底后 1080p 会缩到 72%，界面被压到 0.72 倍。
  const noFallback = loadBoundsModule(boundsSource.replace("Math.max(minimum, preferred, fallback)", "Math.max(minimum, preferred)"));
  assert.throws(() => assert.deepEqual(noFallback.windowBoundsForWorkArea({ width: 1920, height: 1080 }), { width: 1766, height: 1010 }));

  assert.throws(() => assertDesktopCallerContract(mainSource.replace(
    "return windowBoundsForWorkArea(display.workAreaSize);",
    "return { width: 1680, height: 1050 };",
  )));

  const noTopbarDrag = appStyles.replace(/(\.topbar\s*\{[^}]*?)-webkit-app-region:\s*drag;/s, "$1-webkit-app-region: no-drag;");
  assert.throws(() => assertDesktopCallerContract(mainSource, noTopbarDrag));
});
