'use strict';
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),assert=require('node:assert/strict'),{spawnSync}=require('node:child_process');
const root=path.resolve(__dirname,'..'),sourceFile=path.join(root,'.github/workflows/ci.yml'),source=fs.readFileSync(sourceFile,'utf8');
const expression="startsWith(github.ref, 'refs/heads/codex/release-')",special="github.ref == 'refs/heads/codex/ci-hardening-r276'";
const out=path.resolve(process.argv[2]||path.join(os.tmpdir(),'r277-ci-branch-mutations'));fs.mkdirSync(out,{recursive:true});
const positions=[];let at=0;while((at=source.indexOf(expression,at))!==-1){positions.push(at);at+=expression.length;}
assert.equal(positions.length,5,'all five original acceptance conditions must be checked');assert(!source.includes(special),'baseline must remove R276 special branch');
const rows=[];
function run(name,text,want){
 const workflow=path.join(out,name+'.yml');fs.writeFileSync(workflow,text);
 const result=spawnSync(process.execPath,['--test','--test-reporter=tap','scripts/r276-workflow.test.cjs','backend/web/r117.test.cjs'],{cwd:root,env:{...process.env,R276_CI_WORKFLOW_SOURCE:workflow},encoding:'utf8',maxBuffer:16*1024*1024});
 const log=(result.stdout||'')+(result.stderr||'');fs.writeFileSync(path.join(out,name+'.log'),log);
 const matched=want===0?result.status===0:result.status!==0&&log.includes('R276 special branch must never regain acceptance privileges')&&log.includes('R276 special branch must not bypass release-only acceptance');
 rows.push({name,exitCode:result.status,expectedPass:want===0,assertionVerified:matched});fs.writeFileSync(path.join(out,'result.json'),JSON.stringify(rows,null,2)+'\n');assert(matched,name+' must '+(want===0?'PASS':'FAIL in both structure guards'));
}
run('baseline',source,0);
for(const [index,position] of positions.entries())run('restore-r276-condition-'+(index+1),source.slice(0,position)+'('+expression+' || '+special+')'+source.slice(position+expression.length),1);
run('restored',source,0);assert.equal(fs.readFileSync(sourceFile,'utf8'),source,'mutations must not alter checkout');console.log(JSON.stringify(rows));
