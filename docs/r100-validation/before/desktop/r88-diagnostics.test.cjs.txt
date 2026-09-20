'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),os=require('node:os'),vm=require('node:vm');
const {EventEmitter}=require('node:events');
const desktopRoot=process.env.R88_DESKTOP_ROOT||__dirname;
const {appendDesktopLogFile,desktopLogForExport,desktopCrashEvent}=require(path.join(desktopRoot,'desktop-log.cjs'));
test('R88 crash hooks write synchronous reviewed records without swallowing fatal exceptions',()=>{
 const source=fs.readFileSync(path.join(desktopRoot,'main.cjs'),'utf8');const a=source.indexOf('function installDesktopCrashDiagnostics()');const b=source.indexOf('\ninstallDesktopCrashDiagnostics();',a);assert.ok(a>0&&b>a);
 const app=new EventEmitter(),proc=new EventEmitter(),logs=[];vm.runInNewContext(source.slice(a,b)+'\ninstallDesktopCrashDiagnostics();',{app,process:proc,require:n=>{assert.equal(n,'./desktop-log.cjs');return{desktopCrashEvent}},appendDesktopLog:m=>logs.push(m)});
 app.emit('render-process-gone',{}, {},{reason:'crashed',exitCode:139,type:'Renderer',url:'private-id'});app.emit('child-process-gone',{}, {reason:'oom',exitCode:9,type:'GPU',name:'private-name'});proc.emit('uncaughtExceptionMonitor',Object.assign(Error('token=secret'),{name:'TypeError'}));
 assert.equal(logs.length,3);assert.match(logs[0],/render-process-gone/);assert.match(logs[1],/oom/);assert.match(logs[2],/TypeError/);assert.doesNotMatch(logs.join('\n'),/secret|private/);assert.equal(proc.listenerCount('uncaughtException'),0);
 // The installed monitor must not turn a real uncaught exception into success.
 const {spawnSync}=require('node:child_process');const result=spawnSync(process.execPath,['-e','process.on("uncaughtExceptionMonitor",()=>process.stdout.write("captured"));throw Error("fixture")'],{encoding:'utf8'});assert.notEqual(result.status,0);assert.equal(result.stdout,'captured');
});
test('R88 seven-day desktop export excludes old records in every generation and keeps crash fields private',t=>{
 const dir=fs.mkdtempSync(path.join(os.tmpdir(),'r88-desktop-'));t.after(()=>fs.rmSync(dir,{recursive:true,force:true}));const now=Date.parse('2026-09-14T00:00:00Z');
 for(const name of ['desktop.log','desktop.1.log','desktop.4.log'])fs.writeFileSync(path.join(dir,name),'2026-08-23T12:00:00Z 启动页加载失败：ancient\n2026-09-13T12:00:00Z 启动耗时 {"total":123,"token":"secret"}\n2026-09-13T12:01:00Z 进程异常 {"kind":"child-process-gone","reason":"crashed","exitCode":139,"processType":"GPU","token":"secret"}\n');
 const text=desktopLogForExport(dir,now);assert.doesNotMatch(text,/2026-08|ancient|secret|token/);assert.equal(text.split('\n').filter(l=>l.includes('desktop_process_failure')).length,3);assert.match(text,/"expiredLines":3/);assert.match(text,/"windowDays":7/);
 fs.writeFileSync(path.join(dir,'desktop.log'),'2026-09-07T00:00:00Z 启动页加载失败：boundary\n');assert.match(desktopLogForExport(dir,now),/2026-09-07/);
});
