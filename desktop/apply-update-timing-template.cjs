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
  const install = path.join(desktopRoot, "node_modules/app-builder-lib/templates/nsis/installSection.nsh");
  const section = fs.readFileSync(install, "utf8").replaceAll("\r\n", "\n");
  const patched = patchUninstallTiming(section);
  if (section !== patched) fs.writeFileSync(install, patched);
}
function patchUninstallTiming(source) {
  const marker = "; R204 real old-version removal timing";
  if (source.includes(marker)) return source;
  const start = "!insertmacro uninstallOldVersion SHELL_CONTEXT\n";
  const end = "SetOutPath $INSTDIR\n";
  if (!source.includes(start) || !source.includes("!insertmacro uninstallOldVersion HKEY_CURRENT_USER") || !source.includes(end)) throw new Error("NSIS uninstall template changed; timing boundaries need review");
  const hook = stage => `!ifmacrodef DLUpdateTiming\n  \u0024{If} \u0024{isUpdated}\n    !insertmacro DLUpdateTiming ${stage}\n  \u0024{EndIf}\n!endif\n`;
  return marker + "\n" + source.replace(start, hook("uninstall_old_start") + start).replace(end, hook("uninstall_old_done") + end);
}
module.exports = { applyUpdateTiming, patchUpdateTiming, patchUninstallTiming };
