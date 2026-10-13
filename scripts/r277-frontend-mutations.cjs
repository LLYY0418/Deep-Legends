'use strict';
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),assert=require('node:assert/strict'),{spawnSync}=require('node:child_process');
const root=path.resolve(__dirname,'..'),web=path.join(root,'backend/web'),report=path.resolve(process.env.R277_REPORT_DIR||path.join(process.env.PI_SCRATCH_DIR||os.tmpdir(),'r277-frontend-proof')),scratch=process.env.PI_SCRATCH_DIR||os.tmpdir();
fs.mkdirSync(report,{recursive:true});
const files=['app.js','gameplay.js','app.css','gameplay.css','history-filters.js','history-filters.css','history-recovery.js','collection-new-items.js','runtime.js'];
const mutations=[
 ['p1-generation','app.js','if(invalid || view.generation<=previousGeneration)','if(invalid)','P1 client view'],
 ['p2-handoff','gameplay.js','if(tab.overviewBackground && !background &&','if(false && tab.overviewBackground && !background &&','P2 user'],
 ['p3-ticker','gameplay.js','if(seconds>0)tab.quotaCountdownTimer=setTimeout','if(false)tab.quotaCountdownTimer=setTimeout','P3 active quota'],
 ['p3-mapping','history-filters.js',"advantage:'压刀'","advantage:undefined",'P3 all tag'],
 ['p3-stopped','history-recovery.js',"if(tab.advancedSearch?.stopReason==='user_stop')return;","if(false)return;",'P3 quota automatically'],
 ['p4-height','gameplay.css','.overview-strip-skeleton { gap: 16px; }','.overview-strip-skeleton { gap: 16px; min-height: 116px; }','P4 skeleton'],
 ['p4-zero','gameplay.js','if(pending)return `<section class="career-section champion-performance">','if(false)return `<section class="career-section champion-performance">','P4 pending champion'],
 ['p5-reuse','history-filters.js','const icon=icons.get(button.dataset.afOption);','const icon=null;','P5 decoded'],
 ['p6-unread','collection-new-items.js','return record?.navUnread===true;','return false;','P6 collection'],
 ['p7-nowrap','app.css','.topbar button { white-space: nowrap; flex-shrink: 0; }','.topbar button { flex-shrink: 0; }','P7 top'],
 ['p8-binding','gameplay.js','function bindRankHistoryControls(container) {','function bindRankHistoryControls(container) { return;','P8 history binding'],
 ['p9-reference','gameplay.js','return full;}','return match;}','P9 complete references'],
 ['p9-collapse','gameplay.js','const rect=entry.getBoundingClientRect();if(!(rect.height>0))return;','return;const rect=entry.getBoundingClientRect();if(!(rect.height>0))return;','P9 distant'],
 ['p10-dedup','app.js','if(state.zoomDiagnosticSignature!==signature && window.reportFlowDiagnostic)','if(window.reportFlowDiagnostic)','P10 actual zoom'],
 ['p10-transport','runtime.js','"client_view_client", "ui_zoom_state", "lp_chip_render"','"client_view_client", "lp_chip_render"','diagnostic transport'],
];
const results=[];
for(const [id,file,anchor,replacement,pattern] of mutations){
 const dir=fs.mkdtempSync(path.join(scratch,'r277-frontend-mutant-'));
 try{
  for(const name of files)fs.copyFileSync(path.join(web,name),path.join(dir,name));
  const target=path.join(dir,file),original=fs.readFileSync(target,'utf8');assert.equal(original.split(anchor).length,2,`unexpected ${id} anchor`);fs.writeFileSync(target,original.replace(anchor,replacement));
  const run=spawnSync(process.execPath,['--test','--test-reporter=tap',`--test-name-pattern=${pattern}`,path.join(__dirname,'r277-frontend-contracts.test.cjs')],{cwd:root,env:{...process.env,R277_WEB:dir},encoding:'utf8',timeout:60000});
  fs.writeFileSync(path.join(report,`mutant-${id}.log`),run.stdout+run.stderr);assert.equal(run.status,1,`${id} must be killed, got ${run.status}: ${run.error||run.stdout}`);assert(run.stdout.includes('not ok'),`${id}: actual assertion failure required`);assert(!/SyntaxError:|ReferenceError:/.test(run.stdout),`${id}: harness errors are not mutation evidence`);results.push({id,status:'killed',exit:run.status,pattern});
 }finally{fs.rmSync(dir,{recursive:true,force:true});}
}
fs.writeFileSync(path.join(report,'mutations-result.json'),JSON.stringify({host:{platform:process.platform,node:process.version},results},null,2));console.log(JSON.stringify({killed:results.length}));
