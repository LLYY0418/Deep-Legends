'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
test('R230 preload exposes only bounded clipboard write IPC and main rejects other windows/frames',()=>{
 const source=fs.readFileSync(path.join(__dirname,'preload.cjs'),'utf8'),bridges={},calls=[];
 vm.runInNewContext(source,{require:()=>({contextBridge:{exposeInMainWorld:(key,api)=>bridges[key]=api},ipcRenderer:{on(){},removeListener(){},invoke:(...args)=>calls.push(args)}})});
 bridges.desktopClipboard.copyText('a'.repeat(500));assert.deepEqual(calls[0],['desktop-copy-text','a'.repeat(256)]);
 const main=fs.readFileSync(path.join(__dirname,'main.cjs'),'utf8');
 const start=main.indexOf('ipcMain.handle("desktop-copy-text",'),end=main.indexOf('\n  });',start)+6;
 let handler;const frame={},sender={mainFrame:frame},writes=[];
 Function('ipcMain','mainWindow','isTrustedRenderer','clipboard',main.slice(start,end))({handle:(channel,fn)=>{handler=fn}},{webContents:sender},s=>s===sender,{writeText:text=>writes.push(text)});
 assert.equal(handler({sender,senderFrame:frame},'召唤师#编号'),true);
 assert.equal(handler({sender:{mainFrame:frame},senderFrame:frame},'bad'),false);
 assert.equal(handler({sender,senderFrame:{}},'bad'),false);
 assert.equal(handler({sender,senderFrame:frame},'a'.repeat(257)),false);assert.deepEqual(writes,['召唤师#编号']);
});

test('trusted main window may write, but not read, the clipboard through navigator.clipboard',()=>{
 const main=fs.readFileSync(path.join(__dirname,'main.cjs'),'utf8');
 const declared=main.match(/const DESKTOP_RENDERER_PERMISSIONS = (new Set\([^;]+\));/);assert(declared);
 const permissions=vm.runInNewContext(declared[1]);
 assert(permissions.has('clipboard-sanitized-write'));assert(permissions.has('fullscreen'));assert(!permissions.has('clipboard-read'));
 assert.match(main,/DESKTOP_RENDERER_PERMISSIONS\.has\(permission\) && isTrustedRenderer\(webContents\)/);
 assert.match(main,/setPermissionCheckHandler\(\(webContents, permission\) => allowedPermission\(webContents, permission\)\)/);
 assert.match(main,/setPermissionRequestHandler\(\(webContents, permission, callback\) => callback\(allowedPermission\(webContents, permission\)\)\)/);
});
