'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {installResizeBorder}=require('./resize-border.cjs');
test('R314 native resize border is Windows-only and binds the exact window handle',()=>{
 const handle=Buffer.alloc(8);let calls=0;const window={getNativeWindowHandle:()=>handle},load=()=>({install:h=>{assert.equal(h,handle);calls++;return true;}});
 assert.equal(installResizeBorder(window,{platform:'darwin',load}),false);assert.equal(calls,0);
 assert.equal(installResizeBorder(window,{platform:'win32',load}),true);assert.equal(calls,1);
 const events=[];const report=e=>events.push(e);
 assert.equal(installResizeBorder(window,{platform:'win32',report,load:()=>{throw Error('private/path/module')}}),false);
 assert.equal(installResizeBorder(window,{platform:'win32',report,load:()=>{throw TypeError('private/path/install')}}),false);
 assert.deepEqual(events,[{event:'resize_border_unavailable',error_type:'Error'}]);
});
test('R314 native resize preserves hidden titlebar, original dispatch, fullscreen/maximize and UI-thread ownership',()=>{
 const s=fs.readFileSync(path.join(__dirname,'native/resize-border/resize_border.cc'),'utf8');
 for(const expected of ['WM_NCHITTEST','DefSubclassProc(hwnd, message, wp, lp)','IsZoomed(hwnd)','WS_THICKFRAME','DWMWA_EXTENDED_FRAME_BOUNDS','GET_X_LPARAM(lp)','GET_Y_LPARAM(lp)','HTBOTTOMLEFT','HTBOTTOMRIGHT','HTLEFT','HTRIGHT','HTBOTTOM','GetCurrentThreadId()','GetCurrentProcessId()','WM_NCDESTROY','RemoveWindowSubclass'])assert(s.includes(expected),expected);
 const pkg=require('./package.json');assert(pkg.build.files.includes('native/resize-border.node'));assert(pkg.build.asarUnpack.includes('native/*.node'));
 const fingerprint=fs.readFileSync(path.join(__dirname,"source-fingerprint.cjs"),"utf8");for(const file of ["desktop/build-resize-border.cjs","desktop/native/resize-border/binding.gyp","desktop/native/resize-border/resize_border.cc"])assert(fingerprint.includes(file),file+" is a fingerprinted build input");
 const main=fs.readFileSync(path.join(__dirname,'main.cjs'),'utf8');assert(main.includes('titleBarStyle: "hidden"'));assert(main.includes('installResizeBorder(mainWindow)'));
});

test('R314 Chromium child input forwards only resize strip hits to the same UI-thread root',()=>{
 const source=fs.readFileSync(path.join(__dirname,'native/resize-border/resize_border.cc'),'utf8');
 assert(source.includes('EnumChildWindows(hwnd, InstallChild'));assert(source.includes('WM_PARENTNOTIFY'));
 assert(source.includes('ResizeHit(parent, lp)'));assert(source.includes('return SendMessageW(parent, WM_NCLBUTTONDOWN, hit, lp)'));assert(source.includes('DefWindowProcW(hwnd, message, hit, lp)'));assert(source.includes('SetCursor(LoadCursorW('));
 assert(source.includes('owner == GetCurrentProcessId() && thread == GetCurrentThreadId()'));
 assert(source.includes('RemoveWindowSubclass(hwnd, ChildResizeBorder, id)'));
 const main=fs.readFileSync(path.join(__dirname,'main.cjs'),'utf8');assert(main.includes('mainWindow.webContents.on("did-finish-load", () => {\n    require("./resize-border.cjs").installResizeBorder(mainWindow);'));
});

test('R314 fresh Windows real-Electron probe builds and copies the native module before launch',()=>{
 const probe=fs.readFileSync(path.join(__dirname,'../scripts/r248-electron.cjs'),'utf8');
 const build=probe.indexOf('require(path.join(sourceRoot, "desktop/build-resize-border.cjs")).build()'),copy=probe.indexOf('fs.copyFileSync(path.join(sourceRoot, "desktop/native/resize-border.node")'),launch=probe.indexOf('child = spawn(require(');
 assert(build>0&&copy>build&&launch>copy);assert(probe.includes('process.platform === "win32"'));
});

test('R314 parent and Chromium input HWND both own the resize cursor before default dispatch',()=>{
 const source=fs.readFileSync(path.join(__dirname,'native/resize-border/resize_border.cc'),'utf8');
 const root=source.slice(source.indexOf('static LRESULT CALLBACK ResizeBorder('));
 assert(root.indexOf('message == WM_SETCURSOR')>=0);
 assert(root.indexOf('ApplyResizeCursor(hwnd)')<root.indexOf('const LRESULT original = DefSubclassProc'));
 assert(source.includes('ApplyResizeCursor(parent)'));
});

test('R314 owns non-client move cursors after Chromium default processing without swallowing native messages',()=>{
 const source=fs.readFileSync(path.join(__dirname,'native/resize-border/resize_border.cc'),'utf8');
 const child=source.slice(source.indexOf('static LRESULT CALLBACK ChildResizeBorder('),source.indexOf('static BOOL CALLBACK InstallChild('));
 const root=source.slice(source.indexOf('static LRESULT CALLBACK ResizeBorder('));
 assert(child.includes('if (message == WM_NCMOUSEMOVE) ApplyResizeCursor(parent);'));
 assert(child.indexOf('if (message == WM_NCMOUSEMOVE)')>child.indexOf('const LRESULT original = DefSubclassProc'));
 assert(root.includes('if (message == WM_NCMOUSEMOVE) ApplyResizeCursor(hwnd);'));
 assert(root.indexOf('if (message == WM_NCMOUSEMOVE)')>root.indexOf('const LRESULT original = DefSubclassProc'));
});

test('R315 native resize release uses the static C runtime',()=>{const g=JSON.parse(fs.readFileSync(path.join(__dirname,'native/resize-border/binding.gyp'),'utf8'));assert.equal(g.targets[0].configurations.Release.msvs_settings.VCCLCompilerTool.RuntimeLibrary,0);});

test('R315 dependency inspection rejects dynamic VC runtimes and records dumpbin DLLs',()=>{
 const {verifyRuntimeDependencies}=require('./build-resize-border.cjs');
 assert.deepEqual(verifyRuntimeDependencies('test.node',{executable:'dumpbin',run:()=>({status:0,stdout:'    KERNEL32.dll\n    USER32.dll\n'})}),['KERNEL32.dll','USER32.dll']);
 for(const name of ['VCRUNTIME140.dll','VCRUNTIME140_1.dll','MSVCP140.dll'])assert.throws(()=>verifyRuntimeDependencies('test.node',{executable:'dumpbin',run:()=>({status:0,stdout:'    '+name+'\n'})}),/VC\+\+ runtime/);
});
