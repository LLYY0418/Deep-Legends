'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const fs=require('node:fs'),os=require('node:os'),path=require('node:path'),cp=require('node:child_process');
const root=path.resolve(__dirname,'..');
function fixture(t){
 const dir=fs.mkdtempSync(path.join(os.tmpdir(),'augment-snapshot-check-'));
 t.after(()=>fs.rmSync(dir,{recursive:true,force:true}));
 fs.mkdirSync(path.join(dir,'scripts'));fs.mkdirSync(path.join(dir,'backend/data'),{recursive:true});
 const script=path.join(dir,'scripts/update-augment-descriptions.cjs'),target=path.join(dir,'backend/data/augment_descriptions_20261017.json');
 fs.copyFileSync(path.join(root,'scripts/update-augment-descriptions.cjs'),script);
 fs.writeFileSync(target,JSON.stringify({augments:Array.from({length:200},(_,i)=>({id:i+1,name:'固定强化'+i,desc:'固定中文说明'}))})+'\n');
 fs.writeFileSync(path.join(dir,'backend/data/augment_description_overrides_zh_cn.json'),'{"augments":[]}');
 const noNetwork=path.join(dir,'no-network.cjs');fs.writeFileSync(noNetwork,'globalThis.fetch=()=>{throw Error("unexpected build-time network request")};');
 const run=()=>cp.spawnSync(process.execPath,['--require',noNetwork,script,'--check'],{encoding:'utf8',cwd:dir});
 return {target,run};
}
test('build snapshot validation is offline and preserves the committed source bytes',t=>{
 const f=fixture(t),before=fs.readFileSync(f.target),r=f.run();
 assert.equal(r.status,0,r.stderr);assert.equal(JSON.parse(r.stdout).checked,true);
 assert.deepEqual(fs.readFileSync(f.target),before);
});
test('build snapshot validation rejects an incomplete local catalog without rewriting it',t=>{
 const f=fixture(t),invalid='{"augments":[]}';fs.writeFileSync(f.target,invalid);
 const r=f.run();assert.notEqual(r.status,0);assert.match(r.stderr,/Invalid Chinese augment catalog/);assert.equal(fs.readFileSync(f.target,'utf8'),invalid);
});
test('both desktop build entry points check the frozen snapshot before fingerprinting',()=>{
 for(const [file,pattern] of [['build-desktop-windows.ps1',/update-augment-descriptions\.cjs"\)\s+--check/],['build-desktop.sh',/augment-snapshot node scripts\/update-augment-descriptions\.cjs --check/]]){
  const source=fs.readFileSync(path.join(root,file),'utf8');assert.match(source,pattern);
  assert(source.indexOf('update-augment-descriptions.cjs')<source.indexOf('source-fingerprint.cjs'),'validate the snapshot before calculating the source fingerprint');
 }
});
