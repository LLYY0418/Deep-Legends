'use strict';
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),assert=require('node:assert/strict'),{runProgressChild}=require('./lib/progress-child.cjs');
const root=path.resolve(__dirname,'..'),web=path.join(root,'backend/web'),report=path.resolve(process.env.R277_REPORT_DIR||path.join(process.env.PI_SCRATCH_DIR||os.tmpdir(),'r277-browser-mutants-proof')),scratch=process.env.PI_SCRATCH_DIR||os.tmpdir();
assert(process.env.R266_BACKEND && fs.existsSync(process.env.R266_BACKEND),'R266_BACKEND must name the project browser fixture backend');
fs.mkdirSync(report,{recursive:true});
const cases=[
 ['p4-height','gameplay.css','.is-pending-strip .game-icon.is-summoner-avatar{border-radius:50%}','.summoner-strip.is-pending-strip{height:116px!important;min-height:116px!important}.is-pending-strip .game-icon.is-summoner-avatar{border-radius:50%}','"key":"strip"'],
 ['p8-binding','gameplay.js','function bindRankHistoryControls(container) {','function bindRankHistoryControls(container) { return;','0 !== 3'],
 ['p9-full-render','gameplay.js','    const list = container.querySelector(".match-list");\n    if (list && additions.length','    const list = container.querySelector(".match-list");\n    if(list && additions.length)list.innerHTML=(tab.data.matches || []).map(match=>renderMatch(match,tab.data.player.playerRef,tab)).join("");\n    if (list && additions.length','sample.sameList && sample.sameFirst'],
];
cases.push(['p7-auto-sidebar','app.js',
 'forced: scale >= 1.5 || width <= 820', 'forced: false', 'automatic sidebar collapse']);
cases.push(['p7-refresh-label','app.css',
 '--refresh-padding:0; --refresh-label-display:none; }',
 '--refresh-padding:0; --refresh-label-display:inline; }', 'refresh label must hide in narrow topbar']);
cases.push(['p7-region-three-characters','app.js','Array.from(name).slice(0, 2).join("")','Array.from(name).slice(0, 3).join("")','compact region must show two characters']);
cases.push(['p7-compact-floor','app.css','#player-search-region-short {display:inline;text-overflow:clip;}','#player-search-region-short {display:none;text-overflow:clip;}','700px must retain compact region labels']);
async function main(){
const results=[];
for(const [id,file,anchor,replacement,expected] of cases.filter(c=>!process.env.R278_BROWSER_MUTANT_ONLY || c[0]===process.env.R278_BROWSER_MUTANT_ONLY)){
 const temp=fs.mkdtempSync(path.join(scratch,'r277-frontend-browser-mutant-')),override=path.join(temp,'web'),out=path.join(temp,'output');fs.mkdirSync(override);
 try{
  for(const name of fs.readdirSync(web).filter(n=>/\.(css|js)$/.test(n)))fs.copyFileSync(path.join(web,name),path.join(override,name));
  const target=path.join(override,file),source=fs.readFileSync(target,'utf8');assert.equal(source.split(anchor).length,2,`unexpected ${id} anchor`);fs.writeFileSync(target,source.replace(anchor,replacement));
  const run=await runProgressChild({command:process.execPath,args:[path.join(__dirname,'r277-frontend-browser.cjs')],cwd:root,env:{...process.env,R277_MUTATION_PROBE:'1',R266_WEB_OVERRIDE:override,R266_BROWSER_OUT:out},directory:out,logFile:path.join(report,`browser-mutant-${id}.log`),label:id,idleMs:180000,totalMs:900000});
  fs.writeFileSync(path.join(report,`browser-mutant-${id}-progress.json`),JSON.stringify(run.progress,null,2));assert.equal(run.exit,1,`${id} must fail`);
  const failure=JSON.parse(fs.readFileSync(path.join(out,'failure.json'),'utf8'));fs.writeFileSync(path.join(report,`browser-mutant-${id}-result.json`),JSON.stringify(failure,null,2));assert(failure.error.includes(expected),`${id} wrong failure: ${failure.error}`);results.push({id,status:'killed',error:failure.error});
 }finally{fs.rmSync(temp,{recursive:true,force:true});}
}
fs.writeFileSync(path.join(report,'browser-mutations-result.json'),JSON.stringify({host:{platform:process.platform,node:process.version},results},null,2));console.log(JSON.stringify({killed:results.length}));
}
main().catch(error=>{console.error(error.stack);process.exitCode=1;});
