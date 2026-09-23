"use strict";

const test = require("node:test"), assert = require("node:assert/strict");
const fs = require("node:fs"), os = require("node:os"), path = require("node:path"), crypto = require("node:crypto");
const { spawnSync } = require("node:child_process");
const project = path.resolve(__dirname, "..");
const packageVersion = require("../desktop/package.json").version;
const sha256 = bytes => crypto.createHash("sha256").update(bytes).digest("hex");

// Execute the real Bash orchestration in a disposable tree. Go, npm and the
// wrapper compiler are synthetic: no build tools, credentials or EXEs run.
function fixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "setup-only-build-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  for (const dir of ["bin", "backend", "desktop/backend", "scripts", "installer", "dist/desktop/win-unpacked"]) {
    fs.mkdirSync(path.join(root, dir), { recursive: true });
  }
  for (const name of ["build-desktop.sh", "scripts/build-stage.cjs", "scripts/go-test-shards.cjs", "desktop/package.json", "desktop/package-lock.json", "desktop/release-build.cjs", "desktop/artifact-names.cjs",
    "desktop/verify-embedded-riot-key.cjs", "desktop/apply-portable-template.cjs", "desktop/verify-build-fingerprint.cjs"]) {
    fs.copyFileSync(path.join(project, name), path.join(root, name));
  }
  const prelude = `const fs = require("node:fs"), path = require("node:path");
    const root = process.env.R82_FIXTURE_ROOT;
    const record = value => fs.appendFileSync(path.join(root, "calls.jsonl"), JSON.stringify(value) + "\\n");
    const artifact = path.join(root, "dist/desktop/Deep Legends Setup ${packageVersion}-public.exe");
    const backend = path.join(root, "desktop/backend/loot-service.exe");
    const uninstall = path.join(root, "desktop/uninstall-shell.exe");
  `;
  const writeScript = (name, code) => {
    fs.writeFileSync(path.join(root, name), `#!${process.execPath}\n${prelude}\n${code}\n`, { mode: 0o755 });
  };
  writeScript("bin/go", `
    const args = process.argv.slice(2);
    if (args[0] === "list") {
      process.stdout.write("synthetic\\n");
      return;
    }
    if (args[0] === "test") {
      if (!args.includes("-vet=off")) throw new Error("go test must not duplicate the separate vet stage");
      if (args.includes("-list")) process.stdout.write("TestSyntheticOne\\nTestSyntheticTwo\\nok  synthetic\\n");
      return;
    }
    if (args[0] === "vet") return;
    if (args[0] !== "build") throw new Error("Unexpected synthetic Go command: " + args[0]);
    record("backend"); fs.writeFileSync(backend, "MZ synthetic backend abcdef012345");
  `);
  writeScript("bin/npm", `
    (async () => {
      const args = process.argv.slice(2);
      if (args[0] === "ci") {
        for (const flag of ["--prefer-offline", "--no-audit", "--no-fund"]) {
          if (!args.includes(flag)) throw new Error("Missing fast npm ci flag: " + flag);
        }
        for (const name of ["electron", "electron-builder", "jsdom", ".bin"]) fs.mkdirSync(path.join(root, "desktop/node_modules", name), {recursive: true});
        fs.writeFileSync(path.join(root, "desktop/node_modules/electron/package.json"), "{}");
        fs.writeFileSync(path.join(root, "desktop/node_modules/electron-builder/package.json"), "{}");
        fs.writeFileSync(path.join(root, "desktop/node_modules/jsdom/package.json"), "{}");
        fs.writeFileSync(path.join(root, "desktop/node_modules/.bin/electron-builder"), "synthetic");
        record("npm-ci"); return;
      }
      if (args[0] === "ls") {
        for (const name of ["electron", "electron-builder", "jsdom"]) {
          if (!fs.existsSync(path.join(root, "desktop/node_modules", name, "package.json"))) process.exitCode = 1;
        }
        return;
      }
      if (args[0] !== "run" || args[1] !== "pack:win-setup") throw new Error("Unexpected packaging target " + args[1]);
      if (!fs.existsSync(uninstall)) throw new Error("NSIS requires the uninstall shell");
      record({stage: "nsis", args});
      // The actual beforePack hook must work without a portable template.
      await require(path.join(root, "desktop/verify-embedded-riot-key.cjs"))();
      const {compute7zCompressArgs} = require(${JSON.stringify(path.join(project, "desktop/node_modules/app-builder-lib/out/targets/archive.js"))});
      record({compression: compute7zCompressArgs("7z", {compression: "normal", installTimeDecodable: true})});
      if (process.env.R82_FAIL_STAGE === "nsis") { process.exitCode = 23; return; }
      const packaged = path.join(root, "dist/desktop/win-unpacked/resources/app.asar.unpacked/backend/loot-service.exe");
      fs.mkdirSync(path.dirname(packaged), {recursive: true}); fs.copyFileSync(backend, packaged);
      fs.writeFileSync(path.join(root, "dist/desktop/win-unpacked/resources/app.asar"), "synthetic runtime");
      fs.writeFileSync(artifact, "synthetic NSIS payload");
    })().catch(error => {console.error(error.message); process.exitCode = 1;});
  `);
  writeScript("desktop/source-fingerprint.cjs", 'process.stdout.write("abcdef012345\\n");');
  writeScript("desktop/verify-packaged-runtime.cjs", `
    if (fs.readFileSync(process.argv[2], "utf8") !== "synthetic runtime") throw new Error("Missing runtime");
    record("runtime-verified");
  `);
  writeScript("installer/build-shell.cjs", `
    if (process.argv[2] === "--uninstall") { record("uninstaller"); fs.writeFileSync(uninstall, "synthetic uninstall"); }
    else {
      if (fs.existsSync(uninstall)) throw new Error("Temporary uninstall shell was not cleaned");
      if (fs.readFileSync(artifact, "utf8") !== "synthetic NSIS payload") throw new Error("Missing NSIS payload");
      record("installer"); fs.writeFileSync(artifact, "MZ synthetic final branded setup");
    }
  `);
  // Leftovers from a previous three-artifact build must never survive or be approved.
  for (const name of [`Deep Legends ${packageVersion}.exe`, `Deep Legends ${packageVersion}.zip`, `Deep Legends Setup ${packageVersion}.exe`, "release-build.json", "SHA256SUMS.txt"]) {
    fs.writeFileSync(path.join(root, "dist/desktop", name), "stale output");
  }
  const env = { ...process.env, PATH: `${root}/bin:${path.dirname(process.execPath)}:${process.env.PATH}`,
    R82_FIXTURE_ROOT: root, R82_FAIL_STAGE: "", DEEP_LEGENDS_KEY_MODE: "public", SKIP_NPM_INSTALL: "0",
    SKIP_SETUP: "1", ELECTRON_DIST: path.join(root, "cached Electron.zip"), ELECTRON_BUILDER_7Z_FILTER: "BCJ2" };
  delete env.ELECTRON_BUILDER_COMPRESSION_LEVEL;
  const run = overrides => spawnSync("bash", [path.join(root, "build-desktop.sh")], {
    cwd: root, env: {...env, ...overrides}, encoding: "utf8", timeout: 30000,
  });
  const calls = () => fs.readFileSync(path.join(root, "calls.jsonl"), "utf8").trim().split("\n").map(JSON.parse);
  return {root, run, calls, directory: path.join(root, "dist/desktop")};
}

test("setup-only Bash build wraps one NSIS result, hashes final bytes and removes legacy outputs", {skip: process.platform === "win32"}, t => {
  const {root, run, calls, directory} = fixture(t);
  const result = run();
  assert.equal(result.status, 0, result.stdout + result.stderr);
  const events = calls();
  assert.deepEqual(events.filter(value => typeof value === "string"), ["backend", "npm-ci", "uninstaller", "installer", "runtime-verified"]);
  const nsis = events.filter(value => value.stage === "nsis");
  assert.equal(nsis.length, 1);
  assert.ok(nsis[0].args.includes('--config.nsis.artifactName=Deep Legends Setup ${version}-public.${ext}'));
  assert.ok(nsis[0].args.includes(`--config.electronDist=${path.join(root, "cached Electron.zip")}`));
  const compression = events.find(value => value.compression).compression;
  assert.ok(compression.includes("-mx=9"), compression.join(" "));
  assert.ok(compression.includes("-mf=BCJ"), "retain NSIS-compatible filter even with an external BCJ2 override");
  assert.ok(!compression.includes("-mf=BCJ2"));
  const name = `Deep Legends Setup ${packageVersion}-public.exe`;
  assert.deepEqual(fs.readdirSync(directory).sort(), [name, "SHA256SUMS-public.txt", "release-build.json"]);
  const hash = sha256(fs.readFileSync(path.join(directory, name)));
  assert.equal(fs.readFileSync(path.join(directory, "SHA256SUMS-public.txt"), "utf8"), `${hash}  ${name}\n`);
  const receipt = JSON.parse(fs.readFileSync(path.join(directory, "release-build.json")));
  assert.equal(receipt.fingerprint, "abcdef012345"); assert.equal(receipt.mode, "public");
  assert.deepEqual(receipt.assets, {[name]: hash});
  assert.equal(fs.existsSync(path.join(root, "desktop/uninstall-shell.exe")), false);
  assert.match(result.stderr, /go-test: started/);
  assert.match(result.stderr, /backend-build: completed/);
  assert.match(result.stderr, /nsis: started/);
});

test("failed NSIS stops before wrapping or issuing a receipt and cleans the uninstall shell", {skip: process.platform === "win32"}, t => {
  const {root, run, calls, directory} = fixture(t);
  const result = run({R82_FAIL_STAGE: "nsis"});
  assert.equal(result.status, 23, result.stdout + result.stderr);
  assert.ok(!calls().includes("installer"));
  for (const name of ["release-build.json", "SHA256SUMS-public.txt", `Deep Legends Setup ${packageVersion}-public.exe`]) {
    assert.equal(fs.existsSync(path.join(directory, name)), false, name);
  }
  assert.equal(fs.existsSync(path.join(root, "desktop/uninstall-shell.exe")), false);
});

test("compression overrides are explicit and invalid levels fail before packaging", () => {
  const {configureSetupCompression} = require("../desktop/verify-embedded-riot-key.cjs");
  const defaults = {}; assert.equal(configureSetupCompression(defaults), "9");
  assert.equal(defaults.ELECTRON_BUILDER_COMPRESSION_LEVEL, "9");
  for (const level of ["0", "3", "9"]) {
    const env = {ELECTRON_BUILDER_COMPRESSION_LEVEL: level};
    assert.equal(configureSetupCompression(env), level);
  }
  for (const level of ["", "10", "-1", "3 -mf=BCJ2"]) {
    assert.throws(() => configureSetupCompression({ELECTRON_BUILDER_COMPRESSION_LEVEL: level}), /single digit 0-9/);
  }
});

test("unchanged complete npm dependencies are reused; damage or a lock change reinstalls them", {skip: process.platform === "win32"}, t => {
  const {root, run, calls} = fixture(t);
  const first = run();
  assert.equal(first.status, 0, first.stdout + first.stderr);
  const second = run();
  assert.equal(second.status, 0, second.stdout + second.stderr);
  assert.match(second.stdout, /npm-ci: cache hit/);
  assert.equal(calls().filter(value => value === "npm-ci").length, 1);

  fs.rmSync(path.join(root, "desktop/node_modules/jsdom/package.json"));
  const damaged = run();
  assert.equal(damaged.status, 0, damaged.stdout + damaged.stderr);
  assert.equal(calls().filter(value => value === "npm-ci").length, 2);

  fs.appendFileSync(path.join(root, "desktop/package-lock.json"), "\n");
  const third = run();
  assert.equal(third.status, 0, third.stdout + third.stderr);
  assert.equal(calls().filter(value => value === "npm-ci").length, 3);
});
