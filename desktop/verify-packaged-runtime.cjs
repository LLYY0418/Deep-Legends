"use strict";

const fs = require("node:fs");
const path = require("node:path");
const asar = require("@electron/asar");

function requiredRuntimeEntries() {
  const config = JSON.parse(fs.readFileSync(path.join(__dirname, "package.json"), "utf8"));
  return config.build.files.filter((name) => name.endsWith(".cjs")).sort();
}

function normalizeArchiveEntry(entry) {
  return String(entry || "").replace(/^[/\\]+/, "").replaceAll("\\", "/");
}

function verifyArchiveEntries(entries) {
  const packaged = new Set(entries.map(normalizeArchiveEntry));
  const missing = requiredRuntimeEntries().filter((entry) => !packaged.has(entry));
  if (missing.length > 0) {
    throw new Error(`packaged desktop runtime is missing: ${missing.join(", ")}`);
  }
  return requiredRuntimeEntries();
}

function verifyPackagedRuntime(archivePath) {
  if (!archivePath || !fs.statSync(archivePath).isFile()) {
    throw new Error(`app.asar is missing: ${archivePath || "<empty>"}`);
  }
  return verifyArchiveEntries(asar.listPackage(archivePath));
}

module.exports = { requiredRuntimeEntries, verifyArchiveEntries, verifyPackagedRuntime };

if (require.main === module) {
  const archivePath = process.argv[2];
  try {
    const verified = verifyPackagedRuntime(archivePath);
    process.stdout.write(`packaged desktop runtime verified: ${verified.join(", ")}\n`);
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
