"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { performance } = require("node:perf_hooks");
const { sourceFingerprint } = require("./source-fingerprint.cjs");
const { verifyBuildFingerprint } = require("./verify-build-fingerprint.cjs");

const excludedDirectories = new Set([".gocache", ".gomodcache", "node_modules", "dist"]);

function copySourceTree(source, destination) {
  fs.cpSync(source, destination, {
    recursive: true,
    // Prune entries before descending, including caches inside nested modules.
    filter: (file) => !excludedDirectories.has(path.basename(file)),
  });
}

function copyFingerprintFixture(project, root) {
  const source = fs.readFileSync(path.join(__dirname, "source-fingerprint.cjs"), "utf8");
  const inputs = [...source.match(/const buildInputs = \[([\s\S]*?)\]/)[1].matchAll(/"([^"]+)"/g)].map((match) => match[1]);
  const runtime = JSON.parse(fs.readFileSync(path.join(__dirname, "package.json"))).build.files.filter((name) => name.endsWith(".cjs")).map((name) => `desktop/${name}`);
  const go = fs.readdirSync(path.join(project, "backend")).filter((name) => name.endsWith(".go") && !name.endsWith("_test.go")).map((name) => `backend/${name}`);
  for (const relative of new Set([...inputs, ...runtime, ...go])) {
    fs.mkdirSync(path.dirname(path.join(root, relative)), { recursive: true });
    fs.copyFileSync(path.join(project, relative), path.join(root, relative));
  }
  copySourceTree(path.join(project, "backend", "web"), path.join(root, "backend", "web"));
  copySourceTree(path.join(project, "installer"), path.join(root, "installer"));
}

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
  const project = path.resolve(__dirname, "..");
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "deep-legends-source-inputs-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  copyFingerprintFixture(project, root);
  const original = sourceFingerprint(root);
  for (const name of ["installer/uninstall/main_windows.go", "installer/uninstall/launch_windows.go", "installer/uninstall/progress.go", "installer/ui/uninstaller.go", "installer/ui/uninstaller.html", "installer/uninstall/app.manifest", "installer/uninstall/rsrc_windows_amd64.syso"]) {
    const file = path.join(root, name), bytes = fs.readFileSync(file);
    fs.appendFileSync(file, "\n");
    assert.notEqual(sourceFingerprint(root), original, `${name} must invalidate the build`);
    fs.writeFileSync(file, bytes);
  }
  for (const name of ["backend/prestige_chromas.json", "go.mod", "desktop/package-lock.json", "backend/data/reroll_pool_14_5.json", "backend/data/skin_release_dates.json", "backend/data/skin_release_overrides.json", "backend/web/app.js", "backend/web/runtime.js", "desktop/nsis/portable.nsi", "installer/main_windows.go", "installer/ui/installer.html", "installer/ui/notice.html", "installer/go.sum", "installer/rsrc_windows_amd64.syso", "installer/build-shell.cjs"]) {
    const file = path.join(root, name);
    const bytes = fs.readFileSync(file);
    fs.appendFileSync(file, "\n");
    assert.notEqual(sourceFingerprint(root), original, `${name} must invalidate the build`);
    fs.writeFileSync(file, bytes);
  }
  fs.writeFileSync(path.join(root, "r70_test.go"), "package main\n");
  fs.appendFileSync(path.join(root, "backend", "web", "runtime.test.cjs"), "\n// test only");
  fs.mkdirSync(path.join(root, "desktop", "backend"), { recursive: true });
  fs.writeFileSync(path.join(root, "desktop", "backend", "loot-service.exe"), "generated");
  fs.writeFileSync(path.join(root, "desktop", "uninstall-shell.exe"), "generated uninstaller");
  fs.writeFileSync(path.join(root, "installer", "payload", "files", "setup.exe"), "generated NSIS");
  fs.writeFileSync(path.join(root, "installer", "payload", "files", "meta.json"), "generated metadata");
  fs.appendFileSync(path.join(root, "installer", "progress_test.go"), "\n// test only");
  assert.equal(sourceFingerprint(root), original);
});

test("R86 ADD2 fingerprint fixture prunes large caches without changing the fingerprint", (t) => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "deep-legends-fingerprint-cache-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const source = path.join(root, "source");
  copyFingerprintFixture(path.resolve(__dirname, ".."), source);
  const lookalike = path.join("installer", ".gocache-notes", "keep.txt");
  fs.mkdirSync(path.dirname(path.join(source, lookalike)), { recursive: true });
  fs.writeFileSync(path.join(source, lookalike), "not a cache directory");
  const original = sourceFingerprint(source);
  const caches = [];
  // Keep routine CI cheap; opt in to the 64 MiB acceptance experiment explicitly.
  const cacheChunkBytes = process.env.DEEP_LEGENDS_LARGE_CACHE_TEST === "1" ? 128 * 1024 : 256;
  for (const parent of ["installer", path.join("installer", "uninstall")]) {
    for (const name of [".gocache", ".gomodcache", "node_modules", "dist"]) {
      const relative = path.join(parent, name);
      caches.push(relative);
      const directory = path.join(source, relative, "27");
      fs.mkdirSync(directory, { recursive: true });
      // Real Go cache files are extensionless; no production hashing rules change.
      const bytes = Buffer.alloc(name === ".gocache" ? cacheChunkBytes : 1024, 0x52);
      for (let i = 0; i < (name === ".gocache" ? 256 : 1); i++) {
        fs.writeFileSync(path.join(directory, `${i}-d`), bytes);
      }
    }
  }
  assert.equal(sourceFingerprint(source), original, "cache data must not change this fixture's fingerprint");
  const measure = (destination, copy) => {
    const start = performance.now();
    copy(source, destination);
    const result = { milliseconds: performance.now() - start, files: 0, bytes: 0 };
    const visit = (directory) => {
      for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
        const file = path.join(directory, entry.name);
        if (entry.isDirectory()) visit(file);
        else { result.files++; result.bytes += fs.statSync(file).size; }
      }
    };
    visit(destination);
    return result;
  };
  const unfilteredRoot = path.join(root, "unfiltered"), filteredRoot = path.join(root, "filtered");
  const unfiltered = measure(unfilteredRoot, (from, to) => fs.cpSync(from, to, { recursive: true }));
  const filtered = measure(filteredRoot, copySourceTree);
  for (const relative of caches) {
    assert.equal(fs.existsSync(path.join(filteredRoot, relative)), false, `${relative} must not be copied`);
  }
  assert.equal(fs.readFileSync(path.join(filteredRoot, lookalike), "utf8"), "not a cache directory");
  assert.equal(sourceFingerprint(unfilteredRoot), original);
  assert.equal(sourceFingerprint(filteredRoot), original);
  assert.equal(unfiltered.files - filtered.files, 518);
  assert.equal(unfiltered.bytes - filtered.bytes, 512 * cacheChunkBytes + 6 * 1024);
  // Timing is evidence, not a flaky CI threshold; file/byte reductions are exact.
  t.diagnostic(JSON.stringify({ unfiltered, filtered, fingerprint: original }));
});
