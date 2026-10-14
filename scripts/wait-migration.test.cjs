"use strict";
// R279 P3: the browser guards that used fixed-window polling now wait through scripts/lib/wait-progress.cjs.
// This test pins the wiring of each migrated `until` WITHOUT a browser:
//   * the window is never shorter than the fixed window it replaced,
//   * success returns, a stalled wait still throws the guard's original timeout error,
//   * no fixed polling loop (for/while + setTimeout) is left inside the migrated function.
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs"), path = require("node:path"), vm = require("node:vm");
const root = path.resolve(__dirname, "..");

// original = the fixed window (ms) the guard used before R279.
const simple = [
  { file: "desktop/r117-browser.cjs", original: 20000, wrapped: true, error: /^condition timeout: /, expression: "document.title" },
  { file: "scripts/r258-browser.cjs", original: 20000, wrapped: false, error: /^timeout /, expression: "document.title" },
  { file: "scripts/r265-persisted-ui.cjs", original: 20000, wrapped: false, error: /^installed UI condition timeout: /, expression: "document.title" },
  { file: "scripts/r266-persisted-ui.cjs", original: 20000, wrapped: false, error: /^installed UI condition timeout: /, expression: "document.title" },
  { file: "scripts/r269-persisted-ui.cjs", original: 20000, wrapped: false, error: /^installed UI condition timeout: /, expression: "document.title" },
  { file: "scripts/r272-persisted-ui.cjs", original: 20000, wrapped: false, error: /^installed UI condition timeout: /, expression: "document.title" },
];
const diagnostic = [
  { file: "scripts/r264-browser.cjs", original: 5000 },
  { file: "scripts/r266-browser.cjs", original: 5000 },
  { file: "scripts/r267-browser.cjs", original: 5000 },
];

// Smallest prefix starting at `const until=` that parses as a complete statement.
function extractUntil(source) {
  const start = source.indexOf("const until=");
  assert.ok(start >= 0, "const until= not found");
  for (let end = source.indexOf(";", start); end !== -1; end = source.indexOf(";", end + 1)) {
    const candidate = source.slice(start, end + 1);
    try { new vm.Script(`(async()=>{${candidate}})`); return candidate; } catch (_) { /* keep growing */ }
  }
  throw new Error("could not isolate the until statement");
}

for (const row of simple) {
  test(`${row.file}: until waits through waitForPage, never shorter than ${row.original} ms`, async () => {
    const statement = extractUntil(fs.readFileSync(path.join(root, row.file), "utf8"));
    assert.ok(!/setTimeout|for\s*\(|while\s*\(/.test(statement), "no fixed polling loop may remain in until");
    const calls = [];
    let result = true;
    const sandbox = {
      evaluate: async () => ({ page: "x", live: false, section: "s" }),
      waitForPage: async (evaluate, expression, options) => { calls.push({ expression, options }); return result; },
      JSON, Error, Promise,
    };
    vm.createContext(sandbox);
    const until = vm.runInContext(`(()=>{${statement};return until;})()`, sandbox);
    assert.equal(await until(row.expression), undefined);
    assert.equal(calls.length, 1);
    assert.equal(calls[0].expression, row.wrapped ? `Boolean(${row.expression})` : row.expression);
    assert.ok(calls[0].options.idleMs >= row.original, `idleMs ${calls[0].options.idleMs} < original ${row.original}`);
    assert.ok(calls[0].options.hardMs > calls[0].options.idleMs, "a hard ceiling above the idle window must remain");
    result = false;
    await assert.rejects(until(row.expression), (error) => row.error.test(error.message));
  });
}

for (const row of diagnostic) {
  test(`${row.file}: until keeps its diagnostics and waits through waitForPage (>= ${row.original} ms)`, () => {
    const statement = extractUntil(fs.readFileSync(path.join(root, row.file), "utf8"));
    assert.ok(!/setTimeout|for\s*\(|while\s*\(/.test(statement.slice(0, statement.indexOf("const snapshot=") > 0 ? statement.indexOf("const snapshot=") : 160)), "no fixed polling loop before the diagnostics");
    const match = /waitForPage\(evaluate,.*?,\{idleMs:(\d+),hardMs:(\d+)/s.exec(statement);
    assert.ok(match, "until must call waitForPage(evaluate, expression, {idleMs, hardMs})");
    assert.ok(Number(match[1]) >= row.original);
    assert.ok(Number(match[2]) > Number(match[1]));
    assert.match(statement, /snapshot|throw/, "the original timeout diagnostics must remain after the wait");
  });
}

test("desktop/r100-browser.cjs: the status-recovery readiness wait goes through waitForPage and keeps its original 'proceed' semantics", () => {
  const source = fs.readFileSync(path.join(root, "desktop/r100-browser.cjs"), "utf8");
  assert.match(source, /await waitForPage\(evaluate,'typeof deepLegendsStatusRecovery==="function"',\{idleMs:15000,hardMs:60000,intervalMs:25\}\);/);
  assert.ok(!/for\(let i=0;i<100;i\+\+\)\{if\(await evaluate\('typeof deepLegendsStatusRecovery/.test(source));
});

test("the migrated scripts import the shared helper from the right relative path", () => {
  for (const file of [...simple, ...diagnostic, { file: "desktop/r100-browser.cjs" }].map((row) => row.file)) {
    const source = fs.readFileSync(path.join(root, file), "utf8");
    const rel = file.startsWith("desktop/") ? "../scripts/lib/wait-progress.cjs" : "./lib/wait-progress.cjs";
    assert.ok(source.includes(`const {waitForPage}=require('${rel}');`), `${file} must import waitForPage`);
    assert.ok(fs.existsSync(path.resolve(root, path.dirname(file), rel)), `${file}: helper path must resolve`);
  }
});
