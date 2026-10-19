'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),os=require('node:os'),cp=require('node:child_process');
test('Go timing helper mutants: no retry and swallowed second failure fail the actual helper tests',()=>{
 const source=fs.readFileSync(path.join(__dirname,'../backend/r287_timing_helper_test.go'),'utf8');
 const mutants=[['fixed',source,true],['no-retry',source.replaceAll('number < r287TimingAttempts()','number < 1'),false],['swallow-second-failure',source.replace('return exceeded','return nil'),false],['retry-mutant',source.replace('os.Getenv("R287_MUTATION_PATH") == "1"','false'),false]];
 const root=fs.mkdtempSync(path.join(os.tmpdir(),'r287-go-timing-mutants-'));
 try{for(const [name,text,pass]of mutants){if(!pass)assert.notEqual(text,source);const dir=path.join(root,name);fs.mkdirSync(dir);fs.writeFileSync(path.join(dir,'go.mod'),'module r287timingfixture\n\ngo 1.24.0\n');fs.writeFileSync(path.join(dir,'timing_test.go'),text);const r=cp.spawnSync('go',['test','-count=1','./...'],{cwd:dir,encoding:'utf8',timeout:120000});assert(!r.error,String(r.error));assert.equal(r.status===0,pass,(r.stdout||'')+(r.stderr||''));if(!pass)assert.match(r.stdout,/FAIL/);}}finally{fs.rmSync(root,{recursive:true,force:true});}
});
