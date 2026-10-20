'use strict';
const fs=require('node:fs'),path=require('node:path'),cp=require('node:child_process'),assert=require('node:assert/strict');
const root=path.resolve(__dirname,'..'),env={...process.env,GOCACHE:process.env.GOCACHE||path.join(root,'.gocache'),R287_MUTATION_PATH:'1'};
const run=(command,args)=>cp.spawnSync(command,args,{cwd:root,env,encoding:'utf8',timeout:120000});
const cases=[
 {name:'lock-required',file:'backend/auto_config_runtime.go',from:'if mode == "aram" || mode == "hextech" {',to:'if true {',command:'go',args:['test','./backend','-run','^TestR298LockAndFlash$','-count=1'],expect:'hover triggered'},
 {name:'flash-reorder',file:'backend/summoner_keys.go',from:'if slot != nil &&',to:'if false && slot != nil &&',command:'go',args:['test','./backend','-run','^TestR298LockAndFlash$','-count=1'],expect:'flash order'},
 {name:'no-default-selection',file:'backend/web/gameplay.js',from:'if(custom && !state.autoConfigSelected) { state.selectedRecommendation = ""; return null; }',to:'if(false && custom && !state.autoConfigSelected) { state.selectedRecommendation = ""; return null; }',command:'node',args:['--test','--test-name-pattern','R298 custom rune recommendation','backend/web/r298-auto-config.test.cjs'],expect:'null'},
];
const results=[];
for(const c of cases){const file=path.join(root,c.file),original=fs.readFileSync(file,'utf8');assert(original.includes(c.from),'missing mutation anchor '+c.name);let result;try{fs.writeFileSync(file,original.replace(c.from,c.to));result=run(c.command,c.args);assert.notEqual(result.status,0,'mutant survived '+c.name);assert((result.stdout+result.stderr).includes(c.expect),'unrelated failure '+c.name+' '+result.stdout+result.stderr);}finally{fs.writeFileSync(file,original);}const restored=run(c.command,c.args);assert.equal(restored.status,0,'restored baseline failed '+c.name+' '+restored.stdout+restored.stderr);results.push({name:c.name,killed:true,restored:true});}
console.log(JSON.stringify(results,null,2));
