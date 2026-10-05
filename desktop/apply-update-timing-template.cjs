"use strict";
const fs = require("node:fs");
const path = require("node:path");
const marker = "; R206 direct upgrade extraction";
function stripLegacyTiming(source) {
  return source.replace(/^; R201 real update timing\n/, "").replace(/^; R204 real old-version removal timing\n/, "")
    .replace(/^[ \t]*!ifmacrodef DLUpdateTiming\n[ \t]*!insertmacro DLUpdateTiming (?:extract_start|extract_done|copy_done)\n[ \t]*!endif\n/gm, "")
    .replace(/^!ifmacrodef DLUpdateTiming\n  \$\{If\} \$\{isUpdated\}\n    !insertmacro DLUpdateTiming uninstall_old_(?:start|done)\n  \$\{EndIf\}\n!endif\n/gm, "");
}
function patchUpdateTiming(source) {
  if (source.startsWith(marker+"\n")) return source;
  source = stripLegacyTiming(source);
  const extraction = /(^[ \t]*Nsis7z::Extract "\$\{FILE\}"\r?\n)/gm;
  if ([...source.matchAll(extraction)].length !== 2 || !source.includes('  DoneExtract7za:\n') || !source.includes('!macro extractUsing7za FILE\n  Push $OUTDIR\n') || !source.includes('CopyFiles /SILENT "$PLUGINSDIR\\7z-out\\*" $OUTDIR')) {
    throw new Error("NSIS extraction template changed; update timing boundaries need review");
  }
  const hook = stage => `  !ifmacrodef DLUpdateTiming\n    !insertmacro DLUpdateTiming ${stage}\n  !endif\n`;
  const direct = `  \u0024{If} \u0024{isUpdated}
  \u0024{AndIf} $DLUpgradeExtractionReady == "1"
    Push $OUTDIR
    SetOutPath $INSTDIR
    StrCpy $R1 0
    DLDirectExtractRetry:
      IntOp $R1 $R1 + 1
      ClearErrors
${hook("extract_start")}      Nsis7z::Extract "\u0024{FILE}"
${hook("extract_done")}      IfErrors 0 DLDirectExtractDone
      \u0024{If} $R1 < 5
        Sleep 1000
        Goto DLDirectExtractRetry
      \u0024{EndIf}
      SetErrorLevel 2
      Quit
    DLDirectExtractDone:
      Pop $R0
      SetOutPath $R0
      Goto DoneExtract7za
  \u0024{EndIf}
`;
  let next = source.replace(extraction,line=>hook("extract_start")+line+hook("extract_done"));
  next=next.replace('!macro extractUsing7za FILE\n','!macro extractUsing7za FILE\n'+direct);
  next=next.replace('  DoneExtract7za:\n','  DoneExtract7za:\n'+hook("copy_done"));
  return marker+"\n"+next;
}
function patchUninstallTiming(source) {
  const marker = "; R206 stable uninstall timing and extraction readiness";
  if (source.startsWith(marker+"\n")) return source;
  source=stripLegacyTiming(source);
  const start = "!insertmacro uninstallOldVersion SHELL_CONTEXT\n";
  const end = "SetOutPath $INSTDIR\n";
  if (!source.includes(start) || !source.includes("!insertmacro uninstallOldVersion HKEY_CURRENT_USER") || !source.includes('!insertmacro handleUninstallResult HKEY_CURRENT_USER\n') || !source.includes(end)) throw new Error("NSIS uninstall template changed; timing boundaries need review");
  const hook = stage => `!ifmacrodef DLUpdateTiming\n  \u0024{If} $DLUpdatedInstall == "1"\n    !insertmacro DLUpdateTiming ${stage}\n  \u0024{EndIf}\n!endif\n`;
  const ready = `\u0024{IfNot} \u0024{Errors}
\u0024{AndIf} $R0 == 0
  StrCpy $DLUpgradeExtractionReady "1"
\u0024{EndIf}
`;
  const flags=`Var /GLOBAL DLUpdatedInstall
Var /GLOBAL DLUpgradeExtractionReady
StrCpy $DLUpdatedInstall "0"
StrCpy $DLUpgradeExtractionReady "0"
\u0024{If} \u0024{isUpdated}
  StrCpy $DLUpdatedInstall "1"
\u0024{EndIf}
`;
  return marker+"\n"+source.replace(start,flags+hook("uninstall_old_start")+start).replace(end,ready+hook("uninstall_old_done")+end);
}
function patchInstallerInitTiming(source) {
  const marker = "; R212 first onInit timing";
  if (source.startsWith(marker+"\n")) return source;
  const start = "Function .onInit\n";
  if (source.split(start).length !== 2 || !source.includes(start+"  Call setInstallSectionSpaceRequired\n")) throw new Error("NSIS onInit template changed; timing boundary needs review");
  const hook = "  !ifndef BUILD_UNINSTALLER\n    !ifmacrodef DLUpdateTiming\n      !insertmacro DLUpdateTiming oninit\n    !endif\n  !endif\n";
  return marker+"\n"+source.replace(start,start+hook);
}
function patchPowerShellDeclaration(source) {
  const marker = "; R212 reusable PowerShell check";
  if (source.startsWith(marker+"\n")) return source;
  // Both runtime branches expand the stock macro. Guard its global variable
  // declaration so it is declared once, while the probe stays in each branch.
  const declaration = "  Var /GLOBAL IsPowerShellAvailable ; 0 = available, 1 = not available\n";
  if (source.split(declaration).length !== 2 || !source.includes("!macro IS_POWERSHELL_AVAILABLE\n"+declaration)) throw new Error("NSIS PowerShell template changed; declaration needs review");
  return marker+"\n"+source.replace(declaration,"  !ifndef DL_POWER_SHELL_DECLARED\n    !define DL_POWER_SHELL_DECLARED\n"+declaration+"  !endif\n");
}
function applyUpdateTiming(desktopRoot = __dirname) {
  const target = path.join(desktopRoot,"node_modules/app-builder-lib/templates/nsis/include/extractAppPackage.nsh");
  const source=fs.readFileSync(target,"utf8").replaceAll("\r\n","\n");
  const next=patchUpdateTiming(source);if(source!==next)fs.writeFileSync(target,next);
  const install=path.join(desktopRoot,"node_modules/app-builder-lib/templates/nsis/installSection.nsh");
  const section=fs.readFileSync(install,"utf8").replaceAll("\r\n","\n");
  const patched=patchUninstallTiming(section);if(section!==patched)fs.writeFileSync(install,patched);
  for (const [relative,patch] of [["installer.nsi",patchInstallerInitTiming],["include/allowOnlyOneInstallerInstance.nsh",patchPowerShellDeclaration]]) {
    const file=path.join(desktopRoot,"node_modules/app-builder-lib/templates/nsis",relative);
    const original=fs.readFileSync(file,"utf8").replaceAll("\r\n","\n");
    const result=patch(original);if(original!==result)fs.writeFileSync(file,result);
  }
}
module.exports={applyUpdateTiming,patchUpdateTiming,patchUninstallTiming,patchInstallerInitTiming,patchPowerShellDeclaration};
