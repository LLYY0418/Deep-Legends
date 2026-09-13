"use strict";
const test = require("node:test"), assert = require("node:assert/strict");
const { runStage } = require("./build-stage.cjs");

test("build stage reports a live wait then completes without logging arguments", async () => {
  const lines = [];
  const code = await runStage({ label: "fixture", command: process.execPath,
    args: ["-e", "setTimeout(() => {}, 150)", "private-argument-must-stay-private"], intervalMs: 20, log: line => lines.push(line) });
  assert.equal(code, 0);
  assert.match(lines[0], /fixture: started/);
  assert.ok(lines.some(line => line.includes("still running")));
  assert.match(lines.at(-1), /fixture: completed \([\d.]+s\)/);
  assert.doesNotMatch(lines.join("\n"), /private-argument-must-stay-private/);
});

test("build stage preserves nonzero exits and reports spawn failures", async () => {
  const lines = [];
  assert.equal(await runStage({ label: "failure", command: process.execPath, args: ["-e", "process.exit(7)"], log: line => lines.push(line) }), 7);
  assert.match(lines.at(-1), /failed: exit 7/);
  assert.equal(await runStage({ label: "missing", command: "/nonexistent/r82-build-fixture", log: line => lines.push(line) }), 1);
  assert.match(lines.at(-1), /failed: ENOENT/);
});
