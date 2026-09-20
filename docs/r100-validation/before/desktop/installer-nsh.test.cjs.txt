"use strict";
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const source = fs.readFileSync(path.join(__dirname,'nsis/installer.nsh'),'utf8');
function fontGuard(text) {
 const lines=text.split('\n');
 for(let i=0;i<lines.length;i++) if(/\$\{NSD_Create(?:Label|Checkbox|Button|BrowseButton|DirRequest)\}/.test(lines[i])) assert.match(lines.slice(i+1,i+5).join('\n'), /\$\{WM_SETFONT\}/,`font missing at line ${i+1}`);
 const native=text.match(/!macro DLLabel[\s\S]*?!macroend/)?.[0];
 assert.match(native,/CreateWindowExW[^\n]*\n\s*SendMessage \$0 \$\{WM_SETFONT\}/);
}
function hiddenGuard(text) {for(const id of [1028,1256,1035]) assert.match(text,new RegExp(`GetDlgItem \\$0 \\$HWNDPARENT ${id}\\s+ShowWindow \\$0 \\$\\{SW_HIDE\\}`));}
function hookGuard(text) {assert.match(text.match(/!macro customPageAfterChangeDir[\s\S]*?!macroend/)?.[0]||'',/!define MUI_PAGE_CUSTOMFUNCTION_SHOW DLInstFilesShow/);}
function headerGuard(text) {assert.doesNotMatch(text,/GetDlgItem \$0 \$HWNDPARENT 103[789]\s+ShowWindow \$0 \$\{SW_SHOW\}/);}
function progressGuard(text) {assert.match(text,/SendMessage \$mui.InstFilesPage.ProgressBar \$\{PBM_SETBARCOLOR\} 0 0x41A4D9/);}
function darkButtonGuard(text) {
 // Native push buttons cannot be recoloured (DefWindowProc ignores
 // WM_CTLCOLORBTN for BS_PUSHBUTTON), and two shipped attempts to force it --
 // blanking the theme, then "DarkMode_Explorer" -- both changed nothing on real
 // hardware. The contract now is the opposite one: do NOT reintroduce either
 // trick on a button, and keep the footer strip light so the stock buttons read
 // as deliberate chrome instead of sitting unreadably on near-black.
 assert.doesNotMatch(text, /SetWindowTheme\([^)]*DarkMode_Explorer/, 'DarkMode_Explorer does not work here; do not re-add it');
 assert.match(text, /!define DL_FOOTER_BG "EDEFF3"/);
 assert.match(text, /DLLabel \$HWNDPARENT 0 420 700 60 "" \$DLBodyFont \$\{DL_FOOTER_INK\} \$\{DL_FOOTER_BG\}/);
 // Exactly one blank-theme opt-out should remain: the progress bar, which must
 // stay theme-opted-out or its PBM_SETBARCOLOR/PBM_SETBKCOLOR calls get
 // ignored. Any more means a button regressed back to the no-op hack.
 const blanked = [...text.matchAll(/SetWindowTheme\(p [^,]+, w " ", w " "\)/g)];
 assert.equal(blanked.length, 1, `expected exactly 1 blank-theme opt-out (the progress bar only), found ${blanked.length}`);
}
function uninstallerGuard(text) {
 // The uninstaller has no custom pages of ours. Leaving the MUI colour defines
 // outside the guard left it half-converted -- dark header and dark log on
 // otherwise stock chrome -- which is worse than plain system styling.
 const guarded = text.match(/!ifndef BUILD_UNINSTALLER[\s\S]*?!endif/);
 assert.ok(guarded, "expected a BUILD_UNINSTALLER guard block");
 for (const define of ["MUI_BGCOLOR", "MUI_TEXTCOLOR", "MUI_INSTFILESPAGE_COLORS"]) {
  assert.match(guarded[0], new RegExp(`!define ${define}`), `${define} must stay inside the installer-only guard`);
 }
}
function finishClosesBeforeLaunchGuard(text) {
 // Launching the app is a ShellExecute/CreateProcess call that does not return
 // until Windows has created the process, and creating a process from a
 // just-written unsigned binary waits out the antivirus scan (2217ms measured
 // as process_to_js). Doing that with the wizard still on screen is what made
 // "完成" look frozen for several seconds, so the window has to be hidden
 // first -- and the checkbox has to be read before the dialog goes away.
 const leave = text.match(/Function DLFinishLeave[\s\S]*?FunctionEnd/)?.[0];
 assert.ok(leave, 'expected Function DLFinishLeave');
 const readState = leave.indexOf('${NSD_GetState} $DLLaunch');
 const hide = leave.indexOf('ShowWindow $HWNDPARENT ${SW_HIDE}');
 const launch = leave.indexOf('${StdUtils.ExecShellAsUser}');
 assert.ok(readState >= 0, 'expected the launch checkbox to be read');
 assert.ok(hide >= 0, 'the finish page must hide the installer window before launching');
 assert.ok(launch >= 0, 'expected the app launch');
 assert.ok(readState < hide, 'read the checkbox before tearing the dialog down');
 assert.ok(hide < launch, 'hide the window before the blocking launch, not after');
}
function progressTrackContrastGuard(text) {
 // 0x1C1410 (BGR) decodes to RGB #10141C, which is ~5-8 units per channel
 // off the page background #0B0E14 -- indistinguishable on a real monitor,
 // which is exactly why the unfilled portion of the bar read as invisible.
 assert.doesNotMatch(text, /PBM_SETBKCOLOR\} 0 0x1C1410/);
 assert.match(text, /PBM_SETBKCOLOR\} 0 0xEDE7E3/);
}
test('N-1 every visible custom text control receives font',()=>fontGuard(source));
test('N-2 MUI branding stays hidden',()=>hiddenGuard(source));
test('N-3 instfiles hook scoped to preceding custom page',()=>hookGuard(source));
test('N-4 stock headers never reappear',()=>headerGuard(source));
test('N-5 gold progress uses BGR',()=>progressGuard(source));
test('N-6 buttons stay stock and sit on a light footer instead of unreadable dark',()=>darkButtonGuard(source));
test('N-7 progress track colour has visible contrast against the page background',()=>progressTrackContrastGuard(source));
test('N-8 Core fallback keeps stock system styling',()=>uninstallerGuard(source));
test('N-9 finish page hides the wizard before the blocking launch call',()=>finishClosesBeforeLaunchGuard(source));
test('R74 installer opts out of bitmap DPI virtualization and has a visible progress track',()=>{
 assert.match(source,/^ManifestDPIAware true$/m);
 const height=source.match(/DLPosition \$mui.InstFilesPage.ProgressBar 30 198 640 (\d+)/)?.[1];
 assert.ok(Number(height)>=10);
});

test('R80 customInstall swaps the executable without changing registry or removal logic', () => {
  assert.match(source, /!define DL_UNINSTALL_SHELL "\$\{__FILEDIR__\}\/\.\.\/uninstall-shell\.exe"/);
  const hook = source.match(/!macro customInstall\r?\n([\s\S]*?)!macroend/)?.[1];
  assert.ok(hook);
  assert.ok(hook.includes('Rename "$INSTDIR\\${UNINSTALL_FILENAME}" "$INSTDIR\\resources\\uninstall-core.dat"'));
  assert.ok(hook.includes('File "/oname=$INSTDIR\\${UNINSTALL_FILENAME}" "${DL_UNINSTALL_SHELL}"'));
  assert.ok(hook.includes('Delete "$INSTDIR\\Uninstall Deep Legends Core.exe"'));
  assert.doesNotMatch(hook, /WriteReg|DeleteReg|RMDir|CreateShortCut/);
});
test('R74 Browse appends Deep Legends once without creating directories prematurely',()=>{
 const browse=source.match(/Function DLBrowse[\s\S]*?FunctionEnd/)[0];
 assert.match(browse,/StrCmp \$1 "Deep Legends" dl_selected_directory_ready/);
 assert.ok(browse.includes('StrCpy $0 "$0\\Deep Legends"'));
 assert.ok(browse.includes('StrCpy $0 "$0Deep Legends"'));
 assert.doesNotMatch(browse,/CreateDirectory/);
});
test('N-1..5 mutation guards reject each broken contract',()=>{
 assert.throws(()=>fontGuard(source.replace('SendMessage $DLLaunch ${WM_SETFONT} $DLBodyFont 1','')));
 for(const id of [1028,1256,1035]) assert.throws(()=>hiddenGuard(source.replace(`$HWNDPARENT ${id}`, '$HWNDPARENT 9999')));
 assert.throws(()=>hookGuard(source.replace('  !define MUI_PAGE_CUSTOMFUNCTION_SHOW DLInstFilesShow','')));
 assert.throws(()=>headerGuard(source+'\nGetDlgItem $0 $HWNDPARENT 1037\nShowWindow $0 ${SW_SHOW}'));
 assert.throws(()=>progressGuard(source.replace('0x41A4D9','0xD9A441')));
 assert.throws(()=>darkButtonGuard(source.replace('!define DL_FOOTER_BG "EDEFF3"','!define DL_FOOTER_BG "0E121A"')));
 assert.throws(()=>darkButtonGuard(source.replace('GetDlgItem $0 $HWNDPARENT 1\n  SendMessage $0 ${WM_SETFONT} $DLButtonFont 1','GetDlgItem $0 $HWNDPARENT 1\n  SendMessage $0 ${WM_SETFONT} $DLButtonFont 1\n  System::Call \'uxtheme::SetWindowTheme(p r0, w "DarkMode_Explorer", w 0)\'')));
 assert.throws(()=>progressTrackContrastGuard(source.replace('0xEDE7E3','0x1C1410')));
 assert.throws(()=>uninstallerGuard(source.replace('!define MUI_BGCOLOR "FFFFFF"','')));
 assert.throws(()=>finishClosesBeforeLaunchGuard(source.replace('  ShowWindow $HWNDPARENT ${SW_HIDE}\n','')));
 assert.throws(()=>finishClosesBeforeLaunchGuard(source.replace('  ShowWindow $HWNDPARENT ${SW_HIDE}\n','').replace('${StdUtils.ExecShellAsUser} $0 "$launchLink" "open" "$1"','${StdUtils.ExecShellAsUser} $0 "$launchLink" "open" "$1"\n    ShowWindow $HWNDPARENT ${SW_HIDE}')));
 assert.throws(()=>finishClosesBeforeLaunchGuard(source.replace('  ${NSD_GetState} $DLLaunch $0\n','').replace('  ShowWindow $HWNDPARENT ${SW_HIDE}\n','  ShowWindow $HWNDPARENT ${SW_HIDE}\n  ${NSD_GetState} $DLLaunch $0\n')));
});

 test('2135 white setup and larger native text/icons without bitmap scaling',()=>{
   assert.match(source,/!define MUI_BGCOLOR "FFFFFF"/);
   assert.match(source,/!define MUI_TEXTCOLOR "20242B"/);
   for (const [font,size] of [['DLFont',20],['DLBodyFont',11],['DLStepFont',10],['DLButtonFont',11]]) assert.ok(source.includes(`CreateFont $${font} "Microsoft YaHei UI" ${size}`));
   assert.match(source,/DLPosition \$DLIcon 626 18 44 48/);
   assert.match(source,/LoadImageW.*i r1, i r1/);
   assert.doesNotMatch(source,/SetCtlColors[^\n]*(?:0B0E14|131822)/);
 });
