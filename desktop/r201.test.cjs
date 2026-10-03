"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { patchUpdateTiming } = require("./apply-update-timing-template.cjs");
test("R201 patches both real extraction paths and the successful copy boundary, idempotently", () => {
 const source = fs.readFileSync(path.join(__dirname,"node_modules/app-builder-lib/templates/nsis/include/extractAppPackage.nsh"),"utf8").replaceAll("\r\n","\n");
 const patched = patchUpdateTiming(source);
 assert.equal(patchUpdateTiming(patched),patched);
 assert.equal((patched.match(/DLUpdateTiming extract_start/g)||[]).length,2);
 assert.equal((patched.match(/DLUpdateTiming extract_done/g)||[]).length,2);
 assert.match(patched,/DoneExtract7za:\n\s*!ifmacrodef DLUpdateTiming\n\s*!insertmacro DLUpdateTiming copy_done/);
 assert.throws(()=>patchUpdateTiming("changed template"),/boundaries need review/);
});
