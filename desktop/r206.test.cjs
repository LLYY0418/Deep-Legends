'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {patchUpdateTiming,patchUninstallTiming}=require('./apply-update-timing-template.cjs');
test('R206 upgrade extracts directly only after successful uninstall; manual uses unchanged 7z-out',()=>{
 const raw=fs.readFileSync(path.join(__dirname,'node_modules/app-builder-lib/templates/nsis/include/extractAppPackage.nsh'),'utf8').replaceAll('\r\n','\n');
 const source=patchUpdateTiming(raw);assert.equal(patchUpdateTiming(source),source);
 const direct=source.slice(source.indexOf('${If} ${isUpdated}'),source.indexOf('  CreateDirectory "$PLUGINSDIR\\7z-out"'));
 assert.match(direct,/\$DLUpgradeExtractionReady == "1"/);assert.match(direct,/SetOutPath \$INSTDIR/);assert.match(direct,/Nsis7z::Extract/);assert.match(direct,/IfErrors 0 DLDirectExtractDone/);assert(!direct.includes('CopyFiles'));assert.match(source,/CopyFiles \/SILENT "\$PLUGINSDIR\\7z-out\\\*" \$OUTDIR/);
 assert.throws(()=>patchUpdateTiming('template changed'),/need review/);
 const install=patchUninstallTiming(fs.readFileSync(path.join(__dirname,'node_modules/app-builder-lib/templates/nsis/installSection.nsh'),'utf8').replaceAll('\r\n','\n'));
 assert(install.indexOf('StrCpy $DLUpgradeExtractionReady "1"')>install.indexOf('handleUninstallResult HKEY_CURRENT_USER'));assert.match(install,/\$\{IfNot\} \$\{Errors\}\s+\$\{AndIf\} \$R0 == 0/);
});
test('R206 parent-exited fast path still checks manual and portable installs',()=>{
 const source=fs.readFileSync(path.join(__dirname,'nsis/installer.nsh'),'utf8');const macro=source.slice(source.indexOf('!macro customCheckAppRunning'),source.indexOf('!macroend',source.indexOf('!macro customCheckAppRunning')));
 assert.match(macro,/--portable-upgrade[\s\S]*FIND_PROCESS/);assert.match(macro,/--parent-exited[\s\S]*\$\{If\} \$\{Errors\}[\s\S]*_CHECK_APP_RUNNING[\s\S]*\$\{ElseIfNot\} \$\{isUpdated\}[\s\S]*_CHECK_APP_RUNNING/);
 const shell=fs.readFileSync(path.join(__dirname,'../installer/install_windows.go'),'utf8');assert.match(shell,/a\.options\.Update && a\.parentExited && !a\.options\.FreshInstall/);assert.match(shell,/--updated --parent-exited/);
});
