"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const original = fs.readFileSync(path.join(__dirname, "suite.js"), "utf8");

function functionSource(source, name) {
  const start = source.indexOf(`function ${name}(`);
  assert.notEqual(start, -1, `${name} missing`);
  const open = source.indexOf("{", source.indexOf(")", start));
  let depth = 0, quote = "", escaped = false;
  for (let i = open; i < source.length; i += 1) {
    const char = source[i];
    if (quote) {
      if (escaped) escaped = false;
      else if (char === "\\") escaped = true;
      else if (char === quote) quote = "";
      continue;
    }
    if (char === '"' || char === "'" || char === "`") { quote = char; continue; }
    if (char === "{") depth += 1;
    if (char === "}" && --depth === 0) return source.slice(start, i + 1);
  }
  assert.fail(`${name} unbalanced`);
}

function runInvitationControl(source) {
  const start = source.indexOf("const queuePolicies = [");
  assert.notEqual(start, -1);
  const end = source.indexOf("];", start) + 2;
  const queuePolicies = Function(`${source.slice(start, end)}; return queuePolicies;`)();
  const state = { watch: { rules: { invitations: { policies: {} } } } };
  let click, saves = 0;
  const button = { dataset: { watchPolicyCycle: "hextech-aram" }, addEventListener(type, callback) {
    assert.equal(type, "click");
    click = callback;
  } };
  const roots = { watch: {
    querySelector: () => null,
    querySelectorAll: selector => selector === "[data-watch-policy-cycle]" ? [button] : [],
  } };
  const names = ["invitationPolicyKeys", "invitationPolicy", "cycleInvitationPolicy", "invitationPolicyButton", "watchRuleControl", "bindWatchControls"];
  const deps = { queuePolicies, state, roots, escapeHTML: String, saveWatch: async () => { saves += 1; } };
  const helpers = Function(...Object.keys(deps), `${names.map(name => functionSource(source, name)).join("\n")}; return { watchRuleControl, bindWatchControls };`)(...Object.values(deps));
  const markup = helpers.watchRuleControl({ control: "invitations" }, state.watch.rules.invitations);
  assert.match(markup, /海克斯大乱斗 · 不处理/);
  assert.match(markup, /data-watch-policy-cycle="hextech-aram"/);
  helpers.bindWatchControls();
  return { click, state, saves: () => saves };
}

async function checkGroupedPolicy(source) {
  const control = runInvitationControl(source);
  for (const expected of ["accept", "decline", "ignore"]) {
    await control.click();
    for (const queueID of ["2300", "2400", "3270"]) {
      assert.equal(control.state.watch.rules.invitations.policies[queueID], expected, `${queueID} missing ${expected}`);
    }
  }
  assert.equal(control.saves(), 3);
}

test("R156 Hextech ARAM invitation switch saves 2300, 2400 and 3270 together", async () => {
  await checkGroupedPolicy(original);
  const mutated = original.replace("for (const queueID of invitationPolicyKeys(key))", "for (const queueID of invitationPolicyKeys(key).slice(0, 1))");
  assert.notEqual(mutated, original);
  await assert.rejects(checkGroupedPolicy(mutated), /2400 missing accept/);
});
