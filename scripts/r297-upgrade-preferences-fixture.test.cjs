'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),os=require('node:os'),path=require('node:path');
const {resetLegacyPreferences}=require('./r297-upgrade-preferences-fixture.cjs');
test('independent legacy scenario archives only the previous candidate authority, retaining every other persisted file',()=>{
 const root=fs.mkdtempSync(path.join(os.tmpdir(),'r297-legacy-preferences-'));
 try{
  for(const folder of ['data','evidence'])fs.mkdirSync(path.join(root,folder));
  const source=path.join(root,'data/ui-preferences.json'),raw=Buffer.from('{"version":1,"values":{"lol-loot-search-region":"kr"}}\n');fs.writeFileSync(source,raw);
  const files=['account-salt','settings.json','history-presets.json'];for(const file of files)fs.writeFileSync(path.join(root,'data',file),'keep-'+file);
  const result=resetLegacyPreferences(root,'079');assert.equal(result.hadAuthority,true);assert(!fs.existsSync(source));assert.deepEqual(fs.readFileSync(path.join(root,'evidence',result.archive)),raw);
  for(const file of files)assert.equal(fs.readFileSync(path.join(root,'data',file),'utf8'),'keep-'+file);
  const firstUpgrade=resetLegacyPreferences(root,'080');assert.equal(firstUpgrade.hadAuthority,false);assert.deepEqual(JSON.parse(fs.readFileSync(path.join(root,'evidence/r297-independent-preferences-fixtures.json'))).map(r=>r.hadAuthority),[true,false]);
 }finally{fs.rmSync(root,{recursive:true,force:true})}
});
test('legacy isolation refuses a non-file authority without modifying other data',()=>{
 const root=fs.mkdtempSync(path.join(os.tmpdir(),'r297-legacy-preferences-'));
 try{for(const folder of ['data','evidence'])fs.mkdirSync(path.join(root,folder));const target=path.join(root,'other.json');fs.writeFileSync(target,'retain');fs.mkdirSync(path.join(root,'data/ui-preferences.json'));assert.throws(()=>resetLegacyPreferences(root,'079'),/authority file/);assert.equal(fs.readFileSync(target,'utf8'),'retain');assert.deepEqual(fs.readdirSync(path.join(root,'evidence')),[])}finally{fs.rmSync(root,{recursive:true,force:true})}
});
