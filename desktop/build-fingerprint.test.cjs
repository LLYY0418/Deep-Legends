"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { verifyBuildFingerprint } = require("./verify-build-fingerprint.cjs");

test("built backend must contain the exact source fingerprint", (t) => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "deep-legends-fingerprint-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const binary = path.join(root, "loot-service.exe");
  fs.writeFileSync(binary, Buffer.concat([Buffer.from([0, 1, 2]), Buffer.from("a1b2c3d4e5f6"), Buffer.from([3, 4, 5])]));
  assert.equal(verifyBuildFingerprint(binary, "a1b2c3d4e5f6"), "a1b2c3d4e5f6");
  assert.throws(() => verifyBuildFingerprint(binary, "000000000000"), /build fingerprint mismatch/);
  assert.throws(() => verifyBuildFingerprint(binary, "dev"), /invalid expected source fingerprint/);
});

test("R70 fingerprint tracks embedded assets/build inputs but excludes tests and generated binaries", (t) => {
  const { sourceFingerprint } = require("./source-fingerprint.cjs");
  const project = path.resolve(__dirname, "..");
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "deep-legends-source-inputs-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const source = fs.readFileSync(path.join(__dirname, "source-fingerprint.cjs"), "utf8");
  const inputs = [...source.match(/const buildInputs = \[([\s\S]*?)\]/)[1].matchAll(/"([^"]+)"/g)].map((match) => match[1]);
  const runtime = JSON.parse(fs.readFileSync(path.join(__dirname, "package.json"))).build.files.filter((name) => name.endsWith(".cjs")).map((name) => `desktop/${name}`);
  const go = fs.readdirSync(project).filter((name) => name.endsWith(".go") && !name.endsWith("_test.go"));
  for (const relative of new Set([...inputs, ...runtime, ...go])) {
    fs.mkdirSync(path.dirname(path.join(root, relative)), { recursive: true });
    fs.copyFileSync(path.join(project, relative), path.join(root, relative));
  }
  fs.cpSync(path.join(project, "web"), path.join(root, "web"), { recursive: true });
  fs.cpSync(path.join(project, "installer"), path.join(root, "installer"), { recursive: true });
  const original = sourceFingerprint(root);
  for (const name of ["installer/uninstall/main_windows.go", "installer/uninstall/launch_windows.go", "installer/uninstall/progress.go", "installer/ui/uninstaller.go", "installer/ui/uninstaller.html", "installer/uninstall/app.manifest", "installer/uninstall/rsrc_windows_amd64.syso"]) {
    const file = path.join(root, name), bytes = fs.readFileSync(file);
    fs.appendFileSync(file, "\n");
    assert.notEqual(sourceFingerprint(root), original, `${name} must invalidate the build`);
    fs.writeFileSync(file, bytes);
  }
  for (const name of ["prestige_chromas.json", "go.mod", "desktop/package-lock.json", "data/reroll_pool_14_5.json", "data/skin_release_dates.json", "data/skin_release_overrides.json", "web/app.js", "web/runtime.js", "desktop/nsis/portable.nsi", "installer/main_windows.go", "installer/ui/installer.html", "installer/ui/notice.html", "installer/go.sum", "installer/rsrc_windows_amd64.syso", "installer/build-shell.cjs"]) {
    const file = path.join(root, name);
    const bytes = fs.readFileSync(file);
    fs.appendFileSync(file, "\n");
    assert.notEqual(sourceFingerprint(root), original, `${name} must invalidate the build`);
    fs.writeFileSync(file, bytes);
  }
  fs.writeFileSync(path.join(root, "r70_test.go"), "package main\n");
  fs.appendFileSync(path.join(root, "web", "runtime.test.cjs"), "\n// test only");
  fs.mkdirSync(path.join(root, "desktop", "backend"), { recursive: true });
  fs.writeFileSync(path.join(root, "desktop", "backend", "loot-service.exe"), "generated");
  fs.writeFileSync(path.join(root, "desktop", "uninstall-shell.exe"), "generated uninstaller");
  fs.writeFileSync(path.join(root, "installer", "payload", "files", "setup.exe"), "generated NSIS");
  fs.writeFileSync(path.join(root, "installer", "payload", "files", "meta.json"), "generated metadata");
  fs.appendFileSync(path.join(root, "installer", "progress_test.go"), "\n// test only");
  assert.equal(sourceFingerprint(root), original);
});
