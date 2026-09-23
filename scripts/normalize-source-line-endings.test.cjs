"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { normalizeSourceLineEndings } = require("./normalize-source-line-endings.cjs");
test("normalizes build text, preserves BOM and excludes binaries, dependencies and symlinks", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "source-lf-"));
  try {
    const inputs = {
      "backend/web/gameplay.js": "line1\r\nline2\r\n",
      "build.ps1": "\ufeffparam()\r\n",
      "installer/app.manifest": "<manifest/>\r\n",
      "desktop/node_modules/example/index.js": "untouched\r\n",
      "installer/payload/files/source.js": "untouched\r\n",
      "dist/result.txt": "untouched\r\n",
      "desktop/assets/icon.ico": "binary\r\n",
      "backend/data/binary.txt": "\0binary\r\n",
    };
    for (const [name, contents] of Object.entries(inputs)) {
      fs.mkdirSync(path.dirname(path.join(root, name)), { recursive: true });
      fs.writeFileSync(path.join(root, name), contents);
    }
    fs.symlinkSync(path.join(root, "dist"), path.join(root, "backend", "linked-output"), "junction");
    assert.equal(normalizeSourceLineEndings(root), 3);
    for (const [name, contents] of Object.entries(inputs)) {
      const changed = ["backend/web/gameplay.js", "build.ps1", "installer/app.manifest"].includes(name);
      assert.equal(fs.readFileSync(path.join(root, name), "utf8"), changed ? contents.replace(/\r\n/g, "\n") : contents);
    }
    assert.equal(normalizeSourceLineEndings(root), 0);
  } finally { fs.rmSync(root, { recursive: true, force: true }); }
});
