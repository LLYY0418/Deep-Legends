"use strict";
const { evidencePath, requireEvidence } = require('./local-evidence.cjs');
if (require.main === module && process.env.DEEP_LEGENDS_TEST_LICENSE !== "1" && !["--backend", "--probe-backend"].includes(process.argv[2])) throw Error("Explicit license test switch required: DEEP_LEGENDS_TEST_LICENSE=1");

// All mutations live in disposable copies; production sources are never edited.
const fs = require("node:fs"), os = require("node:os"), path = require("node:path"), assert = require("node:assert/strict");
const { spawnSync } = require("node:child_process");
const root = path.resolve(__dirname, ".."), out = process.env.R239_MUTATION_OUTPUT || evidencePath('r239/mutations');
const temp = fs.mkdtempSync(path.join(os.tmpdir(), "r239-mutations-")), results = [];
fs.mkdirSync(out, { recursive: true });
function run(name, args, env, expected) {
  const started = Date.now(), r = spawnSync(process.execPath, args, { cwd: root, env: { ...process.env, ...env }, encoding: "utf8", timeout: 90000 });
  const log = (r.stdout || "") + (r.stderr || ""); fs.writeFileSync(path.join(out, name + ".log"), log);
  assert.equal(r.error, undefined, name + " must reach an assertion"); assert.equal(r.status, 1, name + " must be killed"); assert.match(log, expected);
  results.push({ name, status: r.status, killed: true, elapsedMs: Date.now() - started });
}
try {
  const web = path.join(temp, "web"); fs.cpSync(path.join(root, "backend/web"), web, { recursive: true });
  const css = path.join(web, "app.css"), text = fs.readFileSync(css, "utf8"), broken = text.replace(".license-panel { width:", ".license-panel { display:grid; width:");
  assert.notEqual(broken, text); fs.writeFileSync(css, broken);
  run("a-closed-dialog-visible", [path.join(root, "scripts/r239-browser.cjs")], { R239_WEB_ROOT: web, R239_BROWSER_OUTPUT: path.join(temp, "browser") }, /P1 closed privacy dialog must be invisible/);
  for (const [name, from, to, expected] of [
    ["b-locked-bounds-write", "active !== true || switching", "active === null || switching", /pending\/locked\/transition persistence|ERR_ASSERTION/],
    ["c-locked-size-removed", "LICENSE_WIDTH = 860, LICENSE_HEIGHT = 580", "LICENSE_WIDTH = 1080, LICENSE_HEIGHT = 680", /860x580 DIP|ERR_ASSERTION/],
  ]) {
    const desktop = path.join(temp, name); fs.mkdirSync(desktop);
    for (const file of ["license-window.cjs", "license-window.license.cjs", "window-bounds-store.cjs"]) fs.copyFileSync(path.join(root, "desktop", file), path.join(desktop, file));
    const file = path.join(desktop, "license-window.cjs"), source = fs.readFileSync(file, "utf8"), mutated = source.replace(from, to);
    assert.notEqual(mutated, source); fs.writeFileSync(file, mutated);
    run(name, ["--test", path.join(desktop, "license-window.license.cjs")], {}, expected);
  }
  fs.writeFileSync(path.join(out, "results.json"), JSON.stringify(results, null, 2) + "\n"); console.log(JSON.stringify(results));
} finally { fs.rmSync(temp, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 }); }
