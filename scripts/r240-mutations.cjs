"use strict";
if (require.main === module && process.env.DEEP_LEGENDS_TEST_LICENSE !== "1" && !["--backend", "--probe-backend"].includes(process.argv[2])) throw Error("Explicit license test switch required: DEEP_LEGENDS_TEST_LICENSE=1");

// Disposable source copies/Go overlay only. Never alter the working sources.
const fs = require("node:fs"), os = require("node:os"), path = require("node:path"), assert = require("node:assert/strict");
const { spawnSync } = require("node:child_process");
const root = path.resolve(__dirname, ".."), out = process.env.R240_MUTATION_OUTPUT || path.join(root, "docs/history/reports/r240/mutations");
const temp = fs.mkdtempSync(path.join(os.tmpdir(), "r240-mutations-")), results = [];
fs.mkdirSync(out, { recursive: true });
function mutate(file, from, to) { const source = fs.readFileSync(file, "utf8"), changed = source.replace(from, to); assert.notEqual(changed, source, "mutation target must exist"); fs.writeFileSync(file, changed); }
function killed(name, command, args, env, expected) {
  const started = new Date(), r = spawnSync(command, args, { cwd: root, env: { ...process.env, ...env }, encoding: "utf8", timeout: 90000 });
  const log = (r.stdout || "") + (r.stderr || ""); fs.writeFileSync(path.join(out, name + ".log"), log);
  assert.equal(r.error, undefined, name + " must reach an assertion"); assert.equal(r.status, 1, name + " must be killed"); assert.match(log, expected);
  results.push({ name, status: r.status, killed: true, started: started.toISOString(), finished: new Date().toISOString(), elapsedMs: Date.now() - started.getTime(), last5: log.trimEnd().split("\n").slice(-5) });
}
try {
  const web = path.join(temp, "web"); fs.cpSync(path.join(root, "backend/web"), web, { recursive: true });
  mutate(path.join(web, "index.html"), 'aria-labelledby="license-title" hidden>', 'aria-labelledby="license-title">');
  mutate(path.join(web, "license-ui.js"), '  layer.hidden = true; frame.hidden = true;\n  document.documentElement.dataset.license = "pending";\n})();', '  layer.hidden = false; frame.hidden = true;\n  document.documentElement.dataset.license = "pending";\n})();');
  killed("a-default-overlay-visible", process.execPath, [path.join(root, "scripts/r240-browser.cjs")], { R240_WEB_ROOT: web, R240_BROWSER_OUTPUT: path.join(temp, "browser") }, /P2 default pending surfaces must both be invisible/);
  for (const [name, from, to, expected] of [
    ["b-resize-without-invisibility", "window.setOpacity(0);", "window.setOpacity(1);", /P2 geometry must be invisible/],
    ["c-no-unmaximize-wait", 'record.waitTimedOut = await waitForWindowEvent(window, "unmaximize", () => window.unmaximize(), eventTimeout) || record.waitTimedOut;', "window.unmaximize();", /P3 must wait unmaximize before content sizing/],
  ]) {
    const desktop = path.join(temp, name); fs.mkdirSync(desktop);
    for (const file of ["license-window.cjs", "license-window.license.cjs", "window-bounds-store.cjs"]) fs.copyFileSync(path.join(root, "desktop", file), path.join(desktop, file));
    mutate(path.join(desktop, "license-window.cjs"), from, to);
    killed(name, process.execPath, ["--test", path.join(desktop, "license-window.license.cjs")], {}, expected);
  }
  const license = path.join(temp, "license.go"), original = path.join(root, "backend/license.go"); fs.copyFileSync(original, license);
  mutate(license, "注册码已在其他设备使用或已被重置", "注册码已被其他地方使用");
  const overlay = path.join(temp, "overlay.json"); fs.writeFileSync(overlay, JSON.stringify({ Replace: { [original]: license } }));
  killed("d-old-replacement-message", "go", ["test", "-tags=license", "-count=1", "-overlay=" + overlay, "-run", "^TestR240ReplacementAndRevocationMessages$", "./backend"], {}, /REPLACED message 注册码已被其他地方使用/);
  fs.writeFileSync(path.join(out, "results.json"), JSON.stringify(results, null, 2) + "\n"); console.log(JSON.stringify(results));
} finally { fs.rmSync(temp, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 }); }
