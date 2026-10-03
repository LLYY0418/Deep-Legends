'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const {EventEmitter}=require('node:events');
const source=fs.readFileSync(path.join(__dirname,'main.cjs'),'utf8'),preload=fs.readFileSync(path.join(__dirname,'preload.cjs'),'utf8');
function extract(name){const start=source.indexOf(`function ${name}(`);assert.ok(start>=0);return source.slice(start,source.indexOf('\n}',start)+2);}
test('R196 taskbar reminder crosses trusted IPC and stops on focus',()=>{
 const ipcMain=new EventEmitter(),handlers=new Map(),bridges={},flash=[];ipcMain.handle=(name,fn)=>handlers.set(name,fn);ipcMain.removeHandler=name=>handlers.delete(name);
 let minimized=true,focused=false,url='http://127.0.0.1:8787/';const contents={isDestroyed:()=>false,getURL:()=>url};const mainWindow=new EventEmitter();Object.assign(mainWindow,{webContents:contents,isDestroyed:()=>false,isMinimized:()=>minimized,isFocused:()=>focused,flashFrame:value=>flash.push(value)});
 const ipcRenderer={send(channel){ipcMain.emit(channel,{sender:contents});}};
 const context=vm.createContext({ipcMain,mainWindow,backendReady:{baseUrl:'http://127.0.0.1:8787'},URL});vm.runInContext(['safeURL','isTrustedRenderer','setupBackendIPC'].map(extract).join('\n')+'\nsetupBackendIPC();',context);
 vm.runInNewContext(preload,{require(){return {ipcRenderer,contextBridge:{exposeInMainWorld(name,value){bridges[name]=value;}}};}});
 bridges.desktopUpdate.ready();assert.deepEqual(flash,[true]);minimized=false;focused=true;bridges.desktopUpdate.ready();assert.deepEqual(flash,[true]);focused=false;bridges.desktopUpdate.ready();assert.deepEqual(flash,[true,true]);
 ipcMain.emit('desktop-update-ready',{sender:{...contents}});url='https://foreign.test/';bridges.desktopUpdate.ready();assert.equal(flash.length,2,'foreign sender or origin cannot flash taskbar');
 const focusBinding=source.match(/^  mainWindow\.on\("focus",[^\n]+/m);assert.ok(focusBinding);vm.runInContext(focusBinding[0],context);mainWindow.emit('focus');assert.deepEqual(flash,[true,true,false]);
 assert.match(source,/ipcMain\.removeAllListeners\("desktop-update-ready"\);/);
});
