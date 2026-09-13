"use strict";
const test = require("node:test"), assert = require("node:assert/strict");
const fs = require("node:fs"), os = require("node:os"), path = require("node:path");
const { spawnSync } = require("node:child_process");
const source = path.join(__dirname, "r82-startup-ab.ps1");
const harness = path.join(__dirname, "r82-startup-ab-script.test.ps1");
const engine = process.env.R82_POWERSHELL || (process.platform === "win32" ? "powershell.exe" : "pwsh");
const probe = spawnSync(engine, ["-NoProfile", "-NonInteractive", "-Command", "$PSVersionTable.PSVersion.ToString()"], { encoding: "utf8", timeout: 15000 });
const skip = !process.env.R82_POWERSHELL && probe.error?.code === "ENOENT"
  ? "PowerShell is required; set R82_POWERSHELL to run the real script tests" : false;

function run(file) {
  assert.equal(probe.status, 0, probe.error?.message || probe.stderr);
  const result = spawnSync(engine, ["-NoProfile", "-NonInteractive", "-File", harness, "-ScriptPath", file], { encoding: "utf8", timeout: 45000 });
  assert.ifError(result.error);
  return { ...result, output: result.stdout + result.stderr };
}
test("R82 A/B receipt lookup and deduplication execute the real PowerShell script", { skip }, () => {
  const result = run(source);
  assert.equal(result.status, 0, result.output);
  assert.match(result.output, /PASS all 16 fixture checks/);
});
test("R82 A/B filename, hash mismatch and duplicate mutations reach failing assertions", { skip }, t => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "r82-ab-mutations-"));
  t.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const original = fs.readFileSync(source, "utf8");
  const cases = [
    ["lookup by filename", "$_.Value -eq $setupHash", "$_.Name -eq [IO.Path]::GetFileName($setupPath)", /renamed content must match by hash/],
    ["accept mismatched hash", "if (!$matchingAsset) {", "if ($false) {", /mismatched content must report the expected error/],
    ["disable duplicate preflight", "if ($previous | Where-Object { $_.fingerprint -eq $fingerprint })", "if ($false)", /duplicate fingerprint must/],
    ["count both groups", "@($previous | Where-Object { $_.group -eq $Group }).Count", "@($previous).Count", /full control must not block empty prewarm/],
    ["reject at two", "$groupCount -ge 3", "$groupCount -ge 2", /control\/2 must allow the third sample/],
    ["wait until four", "$groupCount -ge 3", "$groupCount -gt 3", /control\/3 full group must fail before launch/],
  ];
  for (const [name, before, after, failure] of cases) {
    assert.equal(original.split(before).length, 2, `mutation target changed: ${name}`);
    const file = path.join(directory, "r82-startup-ab.ps1");
    fs.writeFileSync(file, original.replace(before, after));
    const result = run(file);
    assert.notEqual(result.status, 0, `mutation survived: ${name}`);
    assert.match(result.output, /ASSERTION FAILED:/, result.output);
    assert.match(result.output, failure, result.output);
    t.diagnostic(`KILLED ${name}: expected fixture assertion failed`);
  }
  const check = original.match(/    \$groupCount = [\s\S]*?    }\r?\n/)[0];
  const launched = "    $setup = Start-Process -FilePath $setupPath -PassThru";
  assert.ok(original.includes(launched));
  const file = path.join(directory, "late-preflight.ps1");
  fs.writeFileSync(file, original.replace(check, "").replace(launched, `${launched}\n${check}`));
  const late = run(file);
  assert.notEqual(late.status, 0, "post-launch preflight mutation survived");
  assert.match(late.output, /ASSERTION FAILED: control\/3 full group must fail before launch/, late.output);
  t.diagnostic("KILLED post-launch group check: installer mock was called before rejection");
});
