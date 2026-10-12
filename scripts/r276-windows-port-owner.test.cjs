'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),os=require('node:os');
const probe=require(process.env.R276_OWNER_SOURCE||'./r276-windows-port-owner.cjs');
const table='Active Connections\r\n  Proto Local Address Foreign Address State PID\r\n TCP 127.0.0.1:55209 0.0.0.0:0 LISTENING 321\r\n TCP [::]:55209 [::]:0 LISTENING 321\r\n TCP 127.0.0.1:55210 0.0.0.0:0 LISTENING 999\r\n TCP 127.0.0.1:55209 127.0.0.1:2 ESTABLISHED 999\r\n UDP 127.0.0.1:55209 *:* 999\r\n';
function controlledNative(file,args,options){
 if(path.win32.basename(file)==='powershell.exe')throw Object.assign(Error('controlled original CIM invocation timeout'),{code:'ETIMEDOUT'});
 assert.equal(path.win32.basename(file),'netstat.exe');assert.deepEqual(args,['-ano','-p','tcp']);assert.equal(options.timeout,5000);return table;
}
test('R276 original bounded PowerShell query fails under controlled timeout; native same-budget ownership succeeds',()=>{
 assert.throws(()=>controlledNative('powershell.exe',[],{timeout:5000}),{code:'ETIMEDOUT'});
 const dir=fs.mkdtempSync(path.join(os.tmpdir(),'r276-owner-'));try{
  let clock=0;const file=path.join(dir,'owner.json');const row=probe.verifyInstalledPortOwner({port:55209,expectedPID:321,evidence:file,execute:controlledNative,now:()=>++clock});
  assert.deepEqual(row.owners,[321]);assert.equal(row.timeout_ms,5000);assert.equal(row.raw,table);assert.deepEqual(JSON.parse(fs.readFileSync(file)),row);
 }finally{fs.rmSync(dir,{recursive:true,force:true});}
});
test('R276 exact port must have owners and every listener must equal installed PID',()=>{
 const run=text=>probe.verifyInstalledPortOwner({port:55209,expectedPID:321,execute:()=>text});
 assert.throws(()=>run(''),/CDP port belongs to another process/);
 assert.throws(()=>run(table.replaceAll('LISTENING 321','LISTENING 444')),/CDP port belongs to another process/);
 assert.throws(()=>run(table.replace('TCP [::]:55209 [::]:0 LISTENING 321','TCP [::]:55209 [::]:0 LISTENING 444')),/CDP port belongs to another process/);
 assert.throws(()=>run(table.replaceAll('LISTENING 321','LISTENING invalid')),/PID must be numeric/);
 assert.throws(()=>run(table.replaceAll('LISTENING 321','LISTENING 0')),/PID must be positive/);
 assert.throws(()=>probe.verifyInstalledPortOwner({port:55209,expectedPID:0,execute:controlledNative}),/PID proof missing/);
 const error=Object.assign(Error('native query timed out'),{code:'ETIMEDOUT'});assert.throws(()=>probe.verifyInstalledPortOwner({port:55209,expectedPID:321,execute:()=>{throw error}}),e=>e===error);
});
test('R276 all four installed UI probes preserve the exact ownership business assertion',()=>{
 for(const n of [265,266,269,272]){
  const source=fs.readFileSync(path.join(__dirname,`r${n}-persisted-ui.cjs`),'utf8');assert(source.includes("require('./r276-windows-port-owner.cjs').verifyInstalledPortOwner({port,expectedPID,evidence:"));
  assert(source.includes("assert(processIDs.length>0&&processIDs.every(pid=>pid===expectedPID),'CDP port belongs to another process')"));assert(!source.includes('Get-NetTCPConnection'));
 }
});
