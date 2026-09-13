"use strict";

// R77. The boot order is a user-visible contract, not an implementation detail.
// Measured on real hardware (desktop_startup_phases_ms, 0909-1945 log):
//
//   spawn inside whenReady   splash visible 2630ms after process create
//   spawn at module scope    splash visible 4038ms after process create
//
// Spawning a child process runs CreateProcessW inline on the main thread, and a
// freshly installed unsigned build gets scanned during process creation, so any
// spawn that happens before the splash renders is stolen directly from the time
// the user spends looking at an empty desktop. These guards exist because that
// regression compiled, passed every other test, and shipped.

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const mainSource = fs.readFileSync(path.join(__dirname, "main.cjs"), "utf8");

function moduleScopeStatements(text) {
  return text.split("\n").filter((line) => /^\S/.test(line) && !/^(?:function|const|let|var|class|\/\/|\/\*|\*)/.test(line));
}

function backendStartsAfterSplashGuard(text = mainSource) {
  const eager = moduleScopeStatements(text).filter((line) => /\bstartBackend(?:Once)?\s*\(/.test(line));
  assert.deepEqual(eager, [], "the backend must not spawn at module scope: it delays app ready and therefore the splash");

  const splashHandler = text.match(/splashWindow\.webContents\.once\("did-finish-load",[\s\S]*?^  \}\);$/m);
  assert.ok(splashHandler, "expected the splash did-finish-load handler to still exist");
  assert.match(splashHandler[0], /startBackendOnce\(\)/, "the backend must start once the splash has painted");

  // A splash that never loads must not strand the app on a logo forever.
  assert.match(text, /backendStartTimer = setTimeout\(startBackendOnce, \d+\)/, "expected a fallback timer arming the backend start");

  const once = text.match(/function startBackendOnce\(\)[\s\S]*?\n\}/);
  assert.ok(once, "expected startBackendOnce()");
  assert.match(once[0], /if \(backendStarted\) return;/, "startBackendOnce must be idempotent: the timer and the splash both call it");
  assert.match(once[0], /clearTimeout\(backendStartTimer\)/);
  // Requiring the deferred modules here keeps them off the path between the
  // backend answering and the main window appearing (ready_to_window was
  // 206ms before deferral and 1120ms when the requires landed on that path).
  assert.match(once[0], /loadDeferredModules\(\)/, "warm the deferred modules while the backend boots, not after");
}

function splashPaintStaysCheapGuard(text = mainSource) {
  // hexcore-icon.ico carries seven frames up to 256px; inlining it cost 975ms
  // of splash paint on a cold start versus 156ms for the single extracted frame.
  const splash = text.match(/function createSplashWindow\(\)[\s\S]*?\n\}/);
  assert.ok(splash, "expected createSplashWindow()");
  // Comments stripped: the block deliberately names the old icon to explain why
  // it is gone, and that prose must not satisfy or break the guard.
  const code = splash[0].replace(/^\s*\/\/.*$/gm, "");
  assert.match(code, /"splash-mark\.png"/);
  assert.doesNotMatch(code, /hexcore-icon\.ico/, "the multi-frame icon must not be inlined into the splash markup again");
}

function singleInstanceStillGuardsTheBackendGuard(text = mainSource) {
  // startBackendOnce is now only reachable from inside whenReady, which returns
  // early without the lock, so a second launch still cannot spawn a second
  // backend. Assert that early return is intact.
  const ready = text.match(/app\.whenReady\(\)\.then\(\(\) => \{[\s\S]*?\n\}\);/);
  assert.ok(ready, "expected the whenReady handler");
  assert.match(ready[0], /if \(!hasInstanceLock\) return;/, "a second instance must not create a splash or spawn a backend");
  assert.match(ready[0], /createSplashWindow\(\);/);
}

test("R77 backend spawn waits for the splash to paint", () => backendStartsAfterSplashGuard());
test("R77 splash keeps the cheap single-frame logo", () => splashPaintStaysCheapGuard());
test("R77 the instance lock still gates the backend", () => singleInstanceStillGuardsTheBackendGuard());

test("R77 mutation guards reject each broken boot order", () => {
  // The exact regression that shipped.
  assert.throws(() => backendStartsAfterSplashGuard(`${mainSource}\nif (hasInstanceLock) startBackend();\n`));
  assert.throws(() => backendStartsAfterSplashGuard(`startBackendOnce();\n${mainSource}`));
  // Splash paints but nothing follows it.
  assert.throws(() => backendStartsAfterSplashGuard(mainSource.replace("    startBackendOnce();\n", "")));
  // Fallback timer removed: a splash that fails to load hangs forever.
  assert.throws(() => backendStartsAfterSplashGuard(mainSource.replace(/backendStartTimer = setTimeout\(startBackendOnce, \d+\)/, "void 0")));
  // Idempotency dropped: the timer and did-finish-load would spawn two backends.
  assert.throws(() => backendStartsAfterSplashGuard(mainSource.replace("  if (backendStarted) return;\n", "")));
  // Deferred requires pushed back onto the post-backend critical path.
  assert.throws(() => backendStartsAfterSplashGuard(mainSource.replace("  loadDeferredModules();\n}\n\nfunction startBackend()", "}\n\nfunction startBackend()")));
  // The heavy icon creeping back into the splash markup.
  assert.throws(() => splashPaintStaysCheapGuard(mainSource.replace('"splash-mark.png"', '"hexcore-icon.ico"')));
  assert.throws(() => splashPaintStaysCheapGuard(mainSource.replace("  const markup = ", '  const legacy = "hexcore-icon.ico";\n  const markup = ')));
  // Second instance spawning its own backend.
  assert.throws(() => singleInstanceStillGuardsTheBackendGuard(mainSource.replace("  if (!hasInstanceLock) return;\n", "")));
});
