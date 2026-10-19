'use strict';
process.env.R287_MUTATION_PATH='1'; // All children are intentionally mutated cases.
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),assert=require('node:assert/strict'),{spawnSync}=require('node:child_process');
const root=path.resolve(__dirname,'..'),report=path.resolve(process.env.R277_REPORT_DIR||path.join(process.env.PI_SCRATCH_DIR||os.tmpdir(),'r277-cn-score-proof')),scratch=process.env.PI_SCRATCH_DIR||os.tmpdir();
fs.mkdirSync(report,{recursive:true});
const cases=[
 ['cn-route-removed','backend/gameplay_build.go','a.handleCNScoreParts(w, r, request.GameID, request.ParticipantID, request.PlayerRef, request.ServerID)','http.Error(w, "查询参数无效", http.StatusBadRequest)','^TestR277CNScore(Local|SGPCache)', 'lazy score endpoint HTTP 400'],
 ['cn-ref-participant-check-removed','backend/gameplay_build.go','if match.GameID != gameID || !scoreMatchSubjectMatches(match, participantID, reference) {','if match.GameID != gameID {','^TestR277CNScoreRejectsWrongScopeAndParticipant','exposed another scope/participant'],
 ['cn-frontend-riot-only-restored','backend/web/gameplay.js','if(!match || match.scorePartsLoaded','if(!match || !riotTab(tab) || match.scorePartsLoaded','cn hover|self expand','not ok'],
 ['cn-frontend-server-omitted','backend/web/gameplay.js','region,serverId,playerRef,refresh:\'score-parts\'','region,playerRef,refresh:\'score-parts\'','cn hover','not ok'],
];
const results=[];
for(const [id,file,anchor,replacement,pattern,expected] of cases){const dir=fs.mkdtempSync(path.join(scratch,'r277-cn-score-mutant-'));try{
 const source=fs.readFileSync(path.join(root,file),'utf8');assert.equal(source.split(anchor).length,2,`unexpected ${id} anchor`);const mutant=path.join(dir,path.basename(file));fs.writeFileSync(mutant,source.replace(anchor,replacement));
 let run;if(file.endsWith('.go')){const overlay=path.join(dir,'overlay.json');fs.writeFileSync(overlay,JSON.stringify({Replace:{[path.join(root,file)]:mutant}}));run=spawnSync('go',['test',`-overlay=${overlay}`,'./backend','-run',pattern,'-count=1','-v'],{cwd:root,env:{...process.env,GOCACHE:process.env.GOCACHE||path.join(scratch,'r277-cn-gocache')},encoding:'utf8',timeout:90000});}else run=spawnSync(process.execPath,['--test','--test-reporter=tap',`--test-name-pattern=${pattern}`,'backend/web/r277-cn-score.test.cjs'],{cwd:root,env:{...process.env,R277_CN_GAMEPLAY_SOURCE:mutant},encoding:'utf8',timeout:60000});
 fs.writeFileSync(path.join(report,`mutant-${id}.log`),run.stdout+run.stderr);assert.equal(run.status,1,`${id} must FAIL`);assert(run.stdout.includes(expected),`${id} wrong failure ${run.stdout}`);assert(!/SyntaxError:|ReferenceError:|build failed/.test(run.stdout+run.stderr),`${id} compile errors are not mutation proof`);results.push({id,status:'killed',exit:run.status,pattern});
}finally{fs.rmSync(dir,{recursive:true,force:true});}}
fs.writeFileSync(path.join(report,'mutations-result.json'),JSON.stringify({host:{platform:process.platform,arch:process.arch,node:process.version},results},null,2));console.log(JSON.stringify({killed:results.length}));
