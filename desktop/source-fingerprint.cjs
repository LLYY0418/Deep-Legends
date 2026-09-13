"use strict";

const crypto = require("node:crypto");
const fs = require("node:fs");
const path = require("node:path");

const root = path.resolve(__dirname, "..");

function filesUnder(relativeRoot, projectRoot = root) {
  const absoluteRoot = path.join(projectRoot, relativeRoot);
  const result = [];
  const visit = (directory) => {
    for (const entry of fs.readdirSync(directory, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
      const absolute = path.join(directory, entry.name);
      if (entry.isDirectory()) visit(absolute);
      else result.push(absolute);
    }
  };
  visit(absoluteRoot);
  return result;
}

function sourceFingerprint(projectRoot = root) {
  const hash = crypto.createHash("sha256");
  const desktopConfig = JSON.parse(fs.readFileSync(path.join(projectRoot, "desktop", "package.json"), "utf8"));
  const desktopRuntimeFiles = desktopConfig.build.files
    .filter((name) => name.endsWith(".cjs"))
    .sort()
    .map((name) => path.join(projectRoot, "desktop", name));
  // Include build inputs, not test fixtures or generated binaries (the latter
  // contain this fingerprint and would create a circular dependency).
  const buildInputs = [
    "go.mod", "go.sum", "prestige_chromas.json", "desktop/package.json", "desktop/package-lock.json",
    "build-desktop.sh", "build-desktop-windows.ps1", "build-windows.ps1", "scripts/build-stage.cjs",
    "desktop/source-fingerprint.cjs", "desktop/apply-portable-template.cjs",
    "desktop/verify-embedded-riot-key.cjs", "desktop/verify-build-fingerprint.cjs", "desktop/release-build.cjs",
    "desktop/verify-packaged-runtime.cjs", "desktop/nsis/portable.nsi", "desktop/nsis/installer.nsh",
    "desktop/assets/hexcore-icon.ico", "data/reroll_pool_14_5.txt", "data/reroll_pool_14_5.json",
    "data/skin_release_dates.json", "data/skin_release_overrides.json",
  ].map((name) => path.join(projectRoot, name));
  const files = [...new Set([
    ...filesUnder("web", projectRoot).filter((name) => !name.endsWith(".cjs")),
    ...fs.readdirSync(projectRoot).filter((name) => name.endsWith(".go") && !name.endsWith("_test.go")).map((name) => path.join(projectRoot, name)),
    ...desktopRuntimeFiles,
    ...filesUnder("installer", projectRoot).filter((file) => {
      const relative = path.relative(path.join(projectRoot, "installer"), file).replaceAll(path.sep, "/");
      return !relative.startsWith("payload/files/") && !relative.startsWith("_shots/") &&
        !relative.endsWith("_test.go") && !relative.endsWith(".test.cjs") &&
        /\.(go|mod|sum|html|png|manifest|syso|cjs|js)$/.test(relative);
    }),
    ...buildInputs,
  ])].sort();
  for (const file of files) {
    hash.update(path.relative(projectRoot, file).replaceAll(path.sep, "/"));
    hash.update("\0");
    hash.update(fs.readFileSync(file));
    hash.update("\0");
  }
  return hash.digest("hex").slice(0, 12);
}

module.exports = { sourceFingerprint };
if (require.main === module) process.stdout.write(`${sourceFingerprint()}\n`);
