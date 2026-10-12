'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),os=require('node:os'),path=require('node:path'),{EventEmitter}=require('node:events'),{spawnSync}=require('node:child_process');
const helper=path.resolve(process.env.R276_CLEANUP_HELPER||path.join(__dirname,'chrome-test-cleanup.cjs'));
const {cleanupChromeTest,stopChild,removeDirectories,removeDirectoriesSync}=require(helper);
class ProfileWriter extends EventEmitter {
 constructor(directory){super();this.directory=directory;this.exitCode=null;this.signalCode=null;this.pid=123;this.alive=true;this.signals=[];this.started=new Promise(resolve=>this.startedResolve=resolve);}
 kill(signal){this.signals.push(signal);this.startedResolve();return true;}
 write(){if(this.alive&&this.signals.length)fs.writeFileSync(path.join(this.directory,'Chrome-profile-write'),'still writing after kill');}
 finish(){this.write();this.alive=false;this.signalCode='SIGTERM';this.emit('exit',null,'SIGTERM');}
}
function directory(t){const p=fs.mkdtempSync(path.join(os.tmpdir(),'r276-cleanup-'));t.after(()=>fs.rmSync(p,{recursive:true,force:true}));fs.writeFileSync(path.join(p,'old'),'profile');return p;}
function scannedRemove(child){return directory=>{for(const name of fs.readdirSync(directory))fs.unlinkSync(path.join(directory,name));child.write();fs.rmdirSync(directory);};}
test('old immediate deletion deterministically throws ENOTEMPTY while killed child still writes',t=>{
 const temp=directory(t),child=new ProfileWriter(temp);child.kill('SIGTERM');
 assert.throws(()=>scannedRemove(child)(temp),{code:'ENOTEMPTY'});child.finish();
});
test('cleanup waits for actual exit before deleting a profile still written after kill',async t=>{
 const temp=directory(t),child=new ProfileWriter(temp),warnings=[];let removes=0;
 t.after(()=>{if(child.alive)child.finish();});
 const result=cleanupChromeTest({children:[child],directories:[temp]},{warn:x=>warnings.push(x),remove:p=>{removes++;scannedRemove(child)(p);}});
 await child.started;child.write();
 assert.equal(removes,0,'profile deletion began before child exit');
 assert.equal(fs.existsSync(path.join(temp,'Chrome-profile-write')),true);
 child.finish();await result;
 assert.equal(removes,1);assert.equal(fs.existsSync(temp),false);assert.deepEqual(warnings,[]);
});
test('cleanup escalates to SIGKILL then waits for exit; no clock sleeps',async()=>{
 const timers=new Map();let sequence=0;
 const clock={setTimeout:fn=>{const id=++sequence;timers.set(id,fn);return id;},clearTimeout:id=>timers.delete(id)};
 const child=new ProfileWriter('unused'),warnings=[];
 const result=stopChild(child,{clock,warn:x=>warnings.push(x)});
 assert.deepEqual(child.signals,['SIGTERM']);const fire=[...timers.values()][0];timers.clear();fire();
 assert.deepEqual(child.signals,['SIGTERM','SIGKILL']);
 child.write=()=>{};child.finish();assert.equal(await result,true);assert.equal(timers.size,0);assert.deepEqual(warnings,[]);
});
test('profile cleanup waits for both Chrome and backend exits',async t=>{
 const temp=directory(t),chrome=new ProfileWriter(temp),backend=new ProfileWriter(temp);let removed=false;
 t.after(()=>{if(chrome.alive)chrome.finish();if(backend.alive)backend.finish();});
 const pending=cleanupChromeTest({children:[chrome,backend],directories:[temp]},{remove:()=>{assert.equal(chrome.alive,false);assert.equal(backend.alive,false);removed=true;}});
 await Promise.all([chrome.started,backend.started]);chrome.finish();await Promise.resolve();assert.equal(removed,false);
 backend.finish();await pending;assert(removed);
});
test('a child that never exits leaves its profile and reports a warning',async t=>{
 const temp=directory(t),child=new ProfileWriter(temp),warnings=[],timers=new Map();let id=0;
 const clock={setTimeout:fn=>{timers.set(++id,fn);return id;},clearTimeout:i=>timers.delete(i)};
 const result=cleanupChromeTest({children:[child],directories:[temp]},{clock,warn:x=>warnings.push(x)});
 for(let n=0;n<2;n++){const fn=[...timers.values()][0];timers.clear();fn();}
 await result;assert.equal(fs.existsSync(temp),true);assert.equal(warnings.length,1);assert.match(warnings[0],/SIGKILL/);
});
test('close all server connections and await close callback before removal',async()=>{
 let closed=false,connections=false,callback;
 const server={close:fn=>{callback=fn;},closeAllConnections:()=>{connections=true;}};
 let removed=false;const pending=cleanupChromeTest({servers:[server],directories:['profile']},{remove:()=>{assert(closed);removed=true;}});
 await Promise.resolve();await Promise.resolve();assert(connections);assert.equal(removed,false);
 closed=true;callback();await pending;assert(removed);
});
test('persistent directory errors retry eight times then emit one warning without rejection',async()=>{
 let removes=0;const warnings=[];
 const clock={setTimeout:fn=>{queueMicrotask(fn);return 1;},clearTimeout:()=>{}};
 await removeDirectories(['profile'],{clock,remove:()=>{removes++;throw Object.assign(Error('busy'),{code:'ENOTEMPTY'});},warn:x=>warnings.push(x)});
 assert.equal(removes,9);assert.equal(warnings.length,1);
});
test('Electron synchronous quit cleanup warns once without throwing or changing main status',()=>{
 const previous=process.exitCode,warnings=[];process.exitCode=7;
 try{removeDirectoriesSync(['profile'],{remove:()=>{throw Error('profile busy');},warn:x=>warnings.push(x)});assert.equal(process.exitCode,7);assert.equal(warnings.length,1);}
 finally{process.exitCode=previous;}
});
test('cleanup preserves failed main exit status and never turns passing main red',()=>{
 for(const [failed,want] of [[true,7],[false,0]]){
  const script=`const {cleanupChromeTest}=require(${JSON.stringify(helper)});Promise.resolve().then(()=>{if(${failed})throw Error('main guard failure');}).catch(()=>{process.exitCode=7;}).finally(()=>cleanupChromeTest());`;
  const result=spawnSync(process.execPath,['-e',script],{encoding:'utf8'});assert.equal(result.status,want,'cleanup masked main guard failure');
 }
});
