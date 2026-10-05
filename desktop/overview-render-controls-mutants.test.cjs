"use strict";
const test = require("node:test"), assert = require("node:assert/strict");
const { bootDemoApp, settled, installWindowCleanup } = require("./overview-render-helpers.cjs");
installWindowCleanup(test);

const { checkCardControls } = require("./overview-render-card-controls.cjs");
test("R222 all four independent full-render mutants reject the DOM guard", async () => {
  for (const mutant of ["detail", "metric", "damage", "timeline"]) {
    await assert.rejects(checkCardControls(mutant), error => error.name === "AssertionError" && /match-list was replaced|another card was replaced|createElement=/.test(error.message), `${mutant} independent full-render bypass must fail the DOM guard`);
  }
});
