'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {patchUninstallTiming}=require('./apply-update-timing-template.cjs');
test('R204 old-uninstaller timing wraps both scopes and preserves manual flow',()=>{
 const source=fs.readFileSync(path.join(__dirname,'node_modules/app-builder-lib/templates/nsis/installSection.nsh'),'utf8').replaceAll('\r\n','\n');const next=patchUninstallTiming(source);
 assert.equal(patchUninstallTiming(next),next);
 assert(next.indexOf('DLUpdateTiming uninstall_old_start')<next.indexOf('!insertmacro uninstallOldVersion SHELL_CONTEXT'));
 assert(next.indexOf('DLUpdateTiming uninstall_old_done')>next.indexOf('!insertmacro handleUninstallResult HKEY_CURRENT_USER'));
 for(const stage of ['start','done'])assert.match(next,new RegExp('\\$\\{If\\} \\$\\{isUpdated\\}\\s+!insertmacro DLUpdateTiming uninstall_old_'+stage));
 assert.equal((next.match(/!insertmacro uninstallOldVersion/g)||[]).length,2);
 assert.throws(()=>patchUninstallTiming('changed'),/need review/);
});
