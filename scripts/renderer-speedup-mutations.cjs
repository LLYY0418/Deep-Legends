"use strict";
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),cp=require('node:child_process'),assert=require('node:assert/strict'),crypto=require('node:crypto');
const root=path.resolve(__dirname,'..');
const output=path.resolve(process.argv[2]);
const targets=process.argv.slice(3);
assert(targets.length,'pass at least one optimized test file');
fs.mkdirSync(output,{recursive:true});
const productionFiles=['backend/web/gameplay.js','backend/web/suite.js','backend/web/app.js','backend/web/pro-players.js','scripts/r82-startup-ab.ps1'];
const hash=file=>crypto.createHash('sha256').update(fs.readFileSync(path.join(root,file))).digest('hex');
const before=Object.fromEntries(productionFiles.map(f=>[f,hash(f)]));
const rows=[];
for(const testFile of targets){
 const sandbox=fs.mkdtempSync(path.join(os.tmpdir(),'renderer-speedup-mutant-'));
 fs.mkdirSync(path.join(sandbox,'backend/web'),{recursive:true});fs.mkdirSync(path.join(sandbox,'desktop'),{recursive:true});fs.mkdirSync(path.join(sandbox,'scripts'),{recursive:true});
 for(const name of fs.readdirSync(path.join(root,'backend/web')))fs.symlinkSync(path.join(root,'backend/web',name),path.join(sandbox,'backend/web',name));
 for(const name of fs.readdirSync(path.join(root,'desktop')))if(name.endsWith('.cjs')||name==='package.json')fs.copyFileSync(path.join(root,'desktop',name),path.join(sandbox,'desktop',name));
 fs.symlinkSync(path.join(root,'desktop/node_modules'),path.join(sandbox,'desktop/node_modules'),'dir');
 if(testFile==='scripts/r82-startup-ab-script.test.cjs')for(const name of ['r82-startup-ab-script.test.cjs','r82-startup-ab-script.test.ps1','r82-startup-ab.ps1','r82-startup-ab-report.cjs','renderer-speedup-powershell.cjs'])fs.copyFileSync(path.join(root,'scripts',name),path.join(sandbox,'scripts',name));

 let file,source,mutant;
 if(testFile==='scripts/r82-startup-ab-script.test.cjs'){
  file='scripts/r82-startup-ab.ps1';source=fs.readFileSync(path.join(root,file),'utf8');
  const boundary='$_.Value -eq $setupHash';assert.equal(source.split(boundary).length,2);
  mutant=source.replace(boundary,'$_.Name -eq [IO.Path]::GetFileName($setupPath)');
 }else if(testFile==='desktop/overview-render-career.test.cjs'){
  file='backend/web/suite.js';source=fs.readFileSync(path.join(root,file),'utf8');
  const boundary='function facadeTitleText(value) {';assert.equal(source.split(boundary).length,2);
  mutant=source.replace(boundary,boundary+' return "WRONG RENDERER TITLE";');
 }else if(new Set(['desktop/overview-render-events.test.cjs','desktop/overview-render-requests-live.test.cjs','desktop/overview-render-requests.test.cjs','desktop/overview-render-tools.test.cjs','desktop/overview-render.test.cjs','desktop/overview-render-200-expand.test.cjs','desktop/overview-render-200-filter.test.cjs','desktop/overview-render-200-timeline.test.cjs']).has(testFile)){
  file='backend/web/gameplay.js';source=fs.readFileSync(path.join(root,file),'utf8');
  const boundary='function renderOverviewBodyContent(container, tab) {';assert.equal(source.split(boundary).length,2);
  mutant=source.replace(boundary,boundary+' container.innerHTML = "<div>WRONG OVERVIEW CONTENT</div>"; return;');
 }else{throw new Error('mutation target must be explicitly reviewed: '+testFile);}
 fs.unlinkSync(path.join(sandbox,file));fs.writeFileSync(path.join(sandbox,file),mutant);
 const result=cp.spawnSync(process.execPath,[testFile],{cwd:sandbox,encoding:'utf8',timeout:90000,maxBuffer:16*1024*1024,env:{...process.env,NODE_OPTIONS:''}});
 const text=result.stdout+result.stderr;fs.writeFileSync(path.join(output,path.basename(testFile)+'.log'),text);
 const killed=result.status!==0&&!result.error&&/AssertionError|ERR_ASSERTION/.test(text)&&!/SyntaxError|Cannot find module/.test(text);
 rows.push({file:testFile,mutated_business_file:file,assertion_killed:killed,exit_code:result.status,error:result.error?.message||null});
 fs.rmSync(sandbox,{recursive:true,force:true});
}
assert.deepEqual(Object.fromEntries(productionFiles.map(f=>[f,hash(f)])),before,'mutation must not change worktree production files');
const result={rows,unchanged_production:true,all_assertion_killed:rows.every(r=>r.assertion_killed)};
fs.writeFileSync(path.join(output,'results.json'),JSON.stringify(result,null,2)+'\n');console.log(JSON.stringify(result));assert(result.all_assertion_killed,'every optimized file must reject wrong behavior through assertions');
