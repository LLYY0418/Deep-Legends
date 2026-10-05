"use strict";
const test = require("node:test"), assert = require("node:assert/strict");
const { bootDemoApp, settled, installWindowCleanup } = require("./overview-render-helpers.cjs");
installWindowCleanup(test);

const { checkCardControls } = require("./overview-render-card-controls.cjs");
test("R86 ADD-1 card detail, metric, legacy damage controls and timeline stay local at 200 matches", async () => {
  await checkCardControls();
});
