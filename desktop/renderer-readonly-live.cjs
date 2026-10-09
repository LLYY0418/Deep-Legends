"use strict";
const assert = require("node:assert/strict");

// Only the four consumers that exclusively read the live DOM may share it.
// Guard DOM/storage/errors against writes so a future interaction test cannot
// silently contaminate another test. Mutable scenarios keep fresh windows.
module.exports = function readOnlyLiveFixture(test, bootLiveTab) {
  const retainedWindows = new Set();
  let fixture, active = false, before;
  const snapshot = () => ({
    markup: fixture.window.document.documentElement.outerHTML,
    storage: Array.from({length:fixture.window.localStorage.length}, (_, index) => {
      const key=fixture.window.localStorage.key(index);return [key,fixture.window.localStorage.getItem(key)];
    }).sort(),
    errors: [...fixture.errors],
  });
  test.afterEach(() => {
    if (!active) return;
    active = false;
    assert.deepEqual(snapshot(), before, "shared live tests must remain read-only and isolated");
  });
  test.after(() => { if (fixture) fixture.window.close(); });
  return { retainedWindows, async load() {
    if (!fixture) {
      fixture = await bootLiveTab();
      retainedWindows.add(fixture.window);
    }
    before = snapshot();
    active = true;
    return fixture;
  } };
};
