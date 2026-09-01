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
  assert.match(main, /const \{ windowBoundsForWorkArea \} = require\("\.\/window-bounds\.cjs"\)/);
  assert.match(main, /function initialWindowBounds\(\)\s*\{[\s\S]*return windowBoundsForWorkArea\(screen\.getPrimaryDisplay\(\)\.workAreaSize\)/);
  assert.match(css, /\.topbar\s*\{[^}]*-webkit-app-region:\s*drag/s);
}

test("desktop window uses the larger 90/92 percent work-area proportions", () => {
  assert.deepEqual(windowBoundsForWorkArea({ width: 1366, height: 768 }), { width: 1229, height: 707 });
  assert.deepEqual(windowBoundsForWorkArea({ width: 1920, height: 1080 }), { width: 1728, height: 994 });
  assert.deepEqual(windowBoundsForWorkArea({ width: 2560, height: 1440 }), { width: 2304, height: 1325 });
});

test("desktop window clamps each dimension independently", () => {
  assert.deepEqual(windowBoundsForWorkArea({ width: 800, height: 500 }), { width: 1080, height: 680 });
  assert.deepEqual(windowBoundsForWorkArea({ width: 3840, height: 2160 }), { width: 2560, height: 1600 });
  assert.deepEqual(windowBoundsForWorkArea({ width: 1200, height: 2000 }), { width: 1080, height: 1600 });
  assert.deepEqual(windowBoundsForWorkArea({ width: 3000, height: 700 }), { width: 2560, height: 680 });
});

test("desktop main and topbar keep the shared sizing and drag-region callers", () => {
  assertDesktopCallerContract();
});

test("R47 desktop caller and high-resolution mutation probes fail", () => {
  const widthRegression = loadBoundsModule(boundsSource.replace("const WINDOW_MAX_WIDTH = 2560;", "const WINDOW_MAX_WIDTH = 1680;"));
  assert.throws(() => assert.deepEqual(widthRegression.windowBoundsForWorkArea({ width: 3840, height: 2160 }), { width: 2560, height: 1600 }));
  const heightRegression = loadBoundsModule(boundsSource.replace("const WINDOW_MAX_HEIGHT = 1600;", "const WINDOW_MAX_HEIGHT = 1050;"));
  assert.throws(() => assert.deepEqual(heightRegression.windowBoundsForWorkArea({ width: 3840, height: 2160 }), { width: 2560, height: 1600 }));

  assert.throws(() => assertDesktopCallerContract(mainSource.replace(
    "return windowBoundsForWorkArea(screen.getPrimaryDisplay().workAreaSize);",
    "return { width: 1680, height: 1050 };",
  )));
  const noTopbarDrag = appStyles.replace(/(\.topbar\s*\{[^}]*?)-webkit-app-region:\s*drag;/s, "$1-webkit-app-region: no-drag;");
  assert.throws(() => assertDesktopCallerContract(mainSource, noTopbarDrag));
});
