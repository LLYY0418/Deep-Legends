"use strict";
// Shelved fixtures are deliberately separate from the default CI test inventory.
// Compile all tagged packages, then prove both explicitly authorized fixtures pass.
const assert = require("node:assert/strict"), path = require("node:path"), { spawnSync } = require("node:child_process");
const { verifyGoEvents } = require("./run-ci-tests.cjs");
const root = path.resolve(__dirname, ".."), names = ["TestR233FailedCounterSaveDoesNotRefreshCachedLifetime", "TestR242LicenseExpiryStrictSignedField"];
function go(args) {
  const result = spawnSync("go", args, { cwd: root, encoding: "utf8", maxBuffer: 16 * 1024 * 1024, timeout: 120000 });
  process.stdout.write(result.stdout || ""); process.stderr.write(result.stderr || "");
  if (result.error) throw result.error;
  assert.equal(result.status, 0, "R252 explicit license verification failed: " + args.join(" "));
  return result.stdout;
}
const pattern = "^(" + names.join("|") + ")$";
go(["test", "-count=1", "-tags", "license", "-run", "^$", "./..."]);
const listed = go(["test", "-tags", "license", "-list", pattern, "./backend"]).split(/\r?\n/).filter(line => line.startsWith("Test"));
assert.deepEqual(listed, names, "R252 shelved fixture selection must contain exactly both names");
const output = go(["test", "-count=1", "-json", "-tags", "license", "-run", pattern, "./backend"]);
const expected = names.map(name => "lol-loot-assistant/backend/" + name);
const events = output.trim().split(/\r?\n/).map(line => JSON.parse(line));
console.log("R252_LICENSE_FIXTURES_VERIFIED " + JSON.stringify(verifyGoEvents(events, expected, expected)));
