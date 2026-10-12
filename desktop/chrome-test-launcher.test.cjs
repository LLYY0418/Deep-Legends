'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),os=require('node:os'),path=require('node:path');
const {launchChrome}=require(process.env.R275_CHROME_HELPER||'../scripts/chrome-test-launcher.cjs');
function options(t,mode){
 const directory=fs.mkdtempSync(path.join(os.tmpdir(),'r275-startup-'));t.after(()=>fs.rmSync(directory,{recursive:true,force:true}));
 let time=0,timer,watched=false,firstClosed=false;
 return {directory,executable:process.execPath,args:[path.join(__dirname,'../scripts/test-fixtures/r275-fake-chrome.cjs'),mode,path.join(directory,'count')],
  clock:{now:()=>time,setTimeout:(callback,ms)=>{assert.equal(ms,20000);timer=callback;return callback;},clearTimeout:id=>{if(id===timer)timer=null;}},
  onStderr:({text,attempt,child})=>{if(attempt===1&&!watched){watched=true;child.once('close',()=>{firstClosed=true;});}if(attempt===2)assert.equal(firstClosed,true,'first process must close before retry');if(text.includes('fake held')){time+=20000;const fire=timer;assert.ok(fire);fire();}}
 };
}
function attempts(opts){return JSON.parse(fs.readFileSync(path.join(opts.directory,'chrome-startup.json'),'utf8')).attempts;}
test('R275 first startup timeout retries once after the fake Chrome has exited',async t=>{
 const opts=options(t,'recover'),result=await launchChrome(opts);
 const closed=new Promise(resolve=>result.child.once('close',resolve));result.child.kill();await closed;
 const rows=attempts(opts);assert.deepEqual(rows.map(r=>r.result),['timeout','connected']);assert.deepEqual(rows.map(r=>r.elapsedMs),[20000,0]);assert.ok(rows.every(r=>Number.isInteger(r.pid)&&r.stderr.includes('fake')));assert.equal(fs.readFileSync(path.join(opts.directory,'count'),'utf8'),'2');
 // The stderr gate above proves close-before-retry without relying on PID reuse or platform signal-0 behavior.
});
test('R275 two startup timeouts fail with both stderr tails and attempts',async t=>{
 const opts=options(t,'held');await assert.rejects(launchChrome(opts),e=>/attempt 1/.test(e.message)&&/fake held attempt 1/.test(e.message)&&/attempt 2/.test(e.message)&&/fake held attempt 2/.test(e.message));assert.deepEqual(attempts(opts).map(r=>r.result),['timeout','timeout']);
});
test('R275 early Chrome exit fails immediately without retry',async t=>{
 const opts=options(t,'exit');await assert.rejects(launchChrome(opts),/exit attempt 1.*exit=7/);assert.equal(fs.readFileSync(path.join(opts.directory,'count'),'utf8'),'1');assert.deepEqual(attempts(opts).map(r=>r.result),['exit']);
});
test('R275 Chrome spawn error fails immediately without retry',async t=>{
 const opts=options(t,'exit');opts.executable=path.join(opts.directory,'missing-chrome');await assert.rejects(launchChrome(opts),/spawn-error attempt 1/);assert.equal(attempts(opts).length,1);
});
test('R275 all eleven browser entry points share startup and retain page waits',()=>{
 for(const file of ['desktop/r100-browser.cjs','desktop/r117-browser.cjs','desktop/current-game-layout.cjs','desktop/overview-banner-layout.cjs','desktop/pro-players-layout.cjs','scripts/r258-browser.cjs','scripts/r264-browser.cjs','scripts/r265-browser.cjs','scripts/r266-browser.cjs','scripts/r267-browser.cjs','scripts/r269-geometry.cjs']){
  const source=fs.readFileSync(path.join(__dirname,'..',file),'utf8');assert.match(source,/await launchChrome\(/,file);assert.doesNotMatch(source,/DevTools listening on/,file);
 }
});
