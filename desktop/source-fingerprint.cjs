"use strict";

const crypto = require("node:crypto");
const fs = require("node:fs");
const path = require("node:path");

const root = path.resolve(__dirname, "..");

function filesUnder(relativeRoot) {
  const absoluteRoot = path.join(root, relativeRoot);
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

function sourceFingerprint() {
  const hash = crypto.createHash("sha256");
  const files = [
    ...filesUnder("web"),
    ...fs.readdirSync(root).filter((name) => name.endsWith(".go")).sort().map((name) => path.join(root, name)),
    ...["main.cjs", "preload.cjs", "proxy-resolution.cjs"].map((name) => path.join(__dirname, name)),
  ];
  for (const file of files) {
    hash.update(path.relative(root, file).replaceAll(path.sep, "/"));
    hash.update("\0");
    hash.update(fs.readFileSync(file));
    hash.update("\0");
  }
  return hash.digest("hex").slice(0, 12);
}

module.exports = { sourceFingerprint };
if (require.main === module) process.stdout.write(`${sourceFingerprint()}\n`);
