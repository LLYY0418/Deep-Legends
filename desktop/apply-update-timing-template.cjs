"use strict";
const fs = require("node:fs");
const path = require("node:path");
const marker = "; R201 real update timing";
function patchUpdateTiming(source) {
  if (source.includes(marker)) return source;
  const hook = stage => `  !ifmacrodef DLUpdateTiming\n    !insertmacro DLUpdateTiming ${stage}\n  !endif\n`;
  const extraction = /(^[ \t]*Nsis7z::Extract "\$\{FILE\}"\r?\n)/gm;
  const matches = [...source.matchAll(extraction)];
  if (matches.length !== 2 || !source.includes("  DoneExtract7za:\n")) {
    throw new Error("NSIS extraction template changed; update timing boundaries need review");
  }
  return marker + "\n" + source.replace(extraction, line => hook("extract_start") + line + hook("extract_done"))
    .replace("  DoneExtract7za:\n", "  DoneExtract7za:\n" + hook("copy_done"));
}
function applyUpdateTiming(desktopRoot = __dirname) {
  const target = path.join(desktopRoot, "node_modules/app-builder-lib/templates/nsis/include/extractAppPackage.nsh");
  const source = fs.readFileSync(target, "utf8").replaceAll("\r\n", "\n");
  const next = patchUpdateTiming(source);
  if (source !== next) fs.writeFileSync(target, next);
}
module.exports = { applyUpdateTiming, patchUpdateTiming };
