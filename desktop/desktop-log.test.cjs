"use strict";
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),os=require('node:os');
const {EventEmitter}=require('node:events');
const {appendDesktopLogFile,desktopLogForExport,MAX_BYTES}=require('./desktop-log.cjs');
const {attachDiagnosticsExport}=require('./diagnostics-export.cjs');
test('R86 desktop logs stay within five two-MiB generations and export only reviewed fields',t=>{
 const root=fs.mkdtempSync(path.join(os.tmpdir(),'r86-desktop-log-'));t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
 const logs=path.join(root,'logs');
 for(let i=0;i<6500;i++)appendDesktopLogFile(logs,'x'.repeat(2000));
 const entries=fs.readdirSync(logs);assert.equal(entries.length,5);
 assert.ok(entries.reduce((sum,name)=>sum+fs.statSync(path.join(logs,name)).size,0)<=5*MAX_BYTES);
 appendDesktopLogFile(logs,'启动耗时 '+JSON.stringify({total:123,puuid:'private-id'}));
 appendDesktopLogFile(logs,'启动页加载失败：token=secret private-id');
 const data=desktopLogForExport(logs);assert.match(data,/"total":123/);assert.match(data,/启动页加载失败/);assert.doesNotMatch(data,/secret|private-id|puuid/);
 const session=new EventEmitter(),sender={},item=new EventEmitter();let completed;
 const detach=attachDiagnosticsExport({session,sender,getBaseURL:()=> 'http://127.0.0.1:1',getDirectory:()=>root,fileSystem:fs,getDesktopLog:()=>desktopLogForExport(logs),onCompleted:p=>completed=p});
 item.getSavePath=()=>item.destination;item.getURL=()=> 'http://127.0.0.1:1/api/diagnostics/log';item.setSavePath=p=>{item.destination=p;fs.writeFileSync(p,'{"event":"backend"}\n')};
 session.emit('will-download',{},item,sender);item.emit('done',{},'completed');detach();
 const exported=fs.readFileSync(completed,'utf8');assert.match(exported,/"event":"backend"/);assert.match(exported,/"event":"desktop_startup"/);
 assert.doesNotMatch(exported,/secret|private-id/);
});
test('R86 desktop rotation rejects untrusted archive paths without deleting evidence',t=>{
 const root=fs.mkdtempSync(path.join(os.tmpdir(),'r86-desktop-log-'));t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
 fs.writeFileSync(path.join(root,'desktop.log'),'x'.repeat(MAX_BYTES));fs.mkdirSync(path.join(root,'desktop.1.log'));
 assert.throws(()=>appendDesktopLogFile(root,'event'),/untrusted/);assert.equal(fs.statSync(path.join(root,'desktop.log')).size,MAX_BYTES);
});
test('R86 desktop log byte guard rejects an independent unbounded append path',t=>{
 const root=fs.mkdtempSync(path.join(os.tmpdir(),'r86-log-mutant-'));t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
 const source=fs.readFileSync(path.join(__dirname,'desktop-log.cjs'),'utf8');
 const marker='function appendDesktopLogFile(directory, message) {';
 const mutant=source.replace(marker,marker+' if(message==="bypass"){trustedDirectory(directory);fs.appendFileSync(path.join(directory,"desktop.log"),"x".repeat(MAX_BYTES*2));return;}');
 assert.notEqual(mutant,source);
 const module={exports:{}};new Function('require','module',mutant)(require,module);
 function guard(append,dir){append(dir,'bypass');assert.ok(fs.statSync(path.join(dir,'desktop.log')).size<=MAX_BYTES)}
 guard(appendDesktopLogFile,path.join(root,'normal'));
 assert.throws(()=>guard(module.exports.appendDesktopLogFile,path.join(root,'mutant')),{name:'AssertionError'});
});
