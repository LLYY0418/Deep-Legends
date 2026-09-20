"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { readWindowBounds, writeWindowBounds } = require("./window-bounds-store.cjs");

test("window bounds store writes and restores visible bounds", (t) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "deep-legends-window-bounds-"));
  t.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const file = path.join(directory, "window-bounds.json");
  const bounds = { x: 120, y: 80, width: 1500, height: 900, maximized: true };
  assert.equal(writeWindowBounds(file, bounds), true);
  assert.deepEqual(readWindowBounds(file, [{ workArea: { x: 0, y: 0, width: 1920, height: 1040 } }]), bounds);
});

test("window bounds store rejects bounds that no longer intersect a display", (t) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "deep-legends-window-bounds-"));
  t.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const file = path.join(directory, "window-bounds.json");
  assert.equal(writeWindowBounds(file, { x: 2500, y: 100, width: 1200, height: 800, maximized: false }), true);
  assert.equal(readWindowBounds(file, [{ workArea: { x: 0, y: 0, width: 1920, height: 1080 } }]), null);
});

test("window bounds store silently falls back on corrupt or unwritable data", (t) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "deep-legends-window-bounds-"));
  t.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const file = path.join(directory, "window-bounds.json");
  fs.writeFileSync(file, "not-json");
  assert.equal(readWindowBounds(file, []), null);
  assert.equal(writeWindowBounds(directory, { x: 0, y: 0, width: 1200, height: 800 }), false);
});
