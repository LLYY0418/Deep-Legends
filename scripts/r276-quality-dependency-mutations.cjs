'use strict';
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),assert=require('node:assert/strict'),{spawnSync}=require('node:child_process');
const root=path.resolve(__dirname,'..'),out=path.resolve(process.argv[2]||'');assert(process.argv[2]);fs.mkdirSync(out,{recursive:true});
const file=path.join(root,'.github/workflows/ci.yml'),source=fs.readFileSync(file,'utf8'),rows=[];
function run(name,workflow,negative){
 const start=performance.now(),result=spawnSync(process.execPath,['--test','--test-reporter=tap','scripts/r276-workflow.test.cjs'],{cwd:root,env:{...process.env,...(workflow?{R276_CI_WORKFLOW_SOURCE:workflow}:{})},encoding:'utf8'});const log=result.stdout+result.stderr;fs.writeFileSync(path.join(out,name+'.log'),log);
 const ok=negative?result.status!==0&&log.includes('quality must install locked Node dependencies before geometry fixture')&&!/SyntaxError|MODULE_NOT_FOUND/.test(log):result.status===0;
 rows.push({name,exitCode:result.status,assertionVerified:ok,duration_ms:performance.now()-start});fs.writeFileSync(path.join(out,'result.json'),JSON.stringify(rows,null,2)+'\n');console.log(JSON.stringify(rows.at(-1)));assert(ok,name);
}
run('baseline',null,false);
const directory=fs.mkdtempSync(path.join(os.tmpdir(),'r276-quality-dependency-'));
try{
 const needle='      - name: Install quality fixture Node dependencies\n        run: npm ci --prefix desktop\n';assert.equal(source.split(needle).length,2);const changed=path.join(directory,'ci.yml');fs.writeFileSync(changed,source.replace(needle,''));run('missing-quality-npm-preparation',changed,true);
}finally{fs.rmSync(directory,{recursive:true,force:true});}
assert.equal(fs.readFileSync(file,'utf8'),source);run('restored',null,false);
