'use strict';
// R277-only commands, deliberately outside the calibrated renderer shards.
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),assert=require('node:assert/strict'),{spawnSync}=require('node:child_process');
const root=path.resolve(__dirname,'..');
const modes=['contracts','frontend-mutants','build-browser','browser','browser-mutants','deferred-unit','deferred-browser','cleanup-browser'];
function parse(args){
 const options={};
 const mode=args.shift();assert(modes.includes(mode),`mode must be one of ${modes.join(', ')}`);
 while(args.length){const key=args.shift();assert(['--out','--backend'].includes(key),`unknown option ${key}`);assert(args.length&&!options[key],`missing/duplicate ${key}`);options[key]=path.resolve(args.shift());}
 assert(options['--out'],'--out is required');
 if(mode.includes('browser'))assert(options['--backend'],'--backend is required');
 return {mode,out:options['--out'],backend:options['--backend']};
}
function chrome(env){
 const candidates=[env.CHROME_BIN];
 if(process.platform==='win32'){for(const home of [env.PROGRAMFILES,env['PROGRAMFILES(X86)'],env.LOCALAPPDATA])if(home)candidates.push(path.join(home,'Google/Chrome/Application/chrome.exe'));}
 else candidates.push('/usr/bin/google-chrome-stable','/usr/bin/google-chrome','/usr/bin/chromium-browser','/usr/bin/chromium','/Applications/Google Chrome.app/Contents/MacOS/Google Chrome');
 const found=candidates.find(file=>file&&fs.existsSync(file));assert(found,'real Chromium is required, no skip/fallback measurement');return found;
}
function browserMutationCommand(){
 const file=path.join(__dirname,'r277-frontend-browser-mutations.cjs');
 assert(fs.readFileSync(file,'utf8').includes('process.env.R277_REPORT_DIR'),'native mutation output support required');
 return [file];
}
async function main(args){
 const {mode,out,backend}=parse([...args]);fs.mkdirSync(out,{recursive:true});
 const scratch=process.env.PI_SCRATCH_DIR||process.env.RUNNER_TEMP||os.tmpdir();
 if(backend){const relative=path.relative(path.resolve(scratch),backend);assert(relative&&!relative.startsWith('..')&&!path.isAbsolute(relative),'browser executable must be scratch-owned, never a released installer');}
 const env={...process.env,PI_SCRATCH_DIR:scratch,GOCACHE:path.join(scratch,'r277-browser-gocache'),R277_REPORT_DIR:out,R277_MEASURE_ONLY:'0'};
 // Existing baseline-only switches must not silently weaken strict CI checks.
 delete env.R266_MATRIX_BASELINE;delete env.R266_WEB_OVERRIDE;
 const result={mode,host:process.platform,node:process.version,runs:[],status:'RUNNING'};
 const persist=()=>fs.writeFileSync(path.join(out,`${mode}-result.json`),JSON.stringify(result,null,2)+'\n');
 function run(label,command,argv,timeout=300000){
  const child=spawnSync(command,argv,{cwd:root,env,encoding:'utf8',timeout,maxBuffer:32*1024*1024});
  fs.writeFileSync(path.join(out,`${label}.log`),`COMMAND: ${command} ${argv.join(' ')}\n`+(child.stdout||'')+(child.stderr||'')+(child.error?`\nRUNNER ERROR: ${child.error.message}\n`:''));
  result.runs.push({label,command:[command,...argv],exit:child.status,error:child.error?.message});persist();
  assert(!child.error&&child.status===0,`${label} failed: ${child.error?.message||child.status}; original log retained`);
 }
 try{
  if(mode==='contracts')run('frontend-contracts',process.execPath,['--test','scripts/r277-frontend-contracts.test.cjs','scripts/r277-release-contracts.test.cjs','scripts/r277-l5-replay.test.cjs','backend/web/r277-cn-score.test.cjs']);
  if(mode==='deferred-unit'){env.R277_DEFERRED_REPORT_DIR=out;run('deferred-unit',process.execPath,['scripts/r277-deferred-assets-proof.cjs','unit']);}
  if(mode==='frontend-mutants')run('frontend-mutants',process.execPath,['scripts/r277-frontend-mutations.cjs']);
  if(mode==='build-browser')run('build-browser',process.execPath,['scripts/r276-public-fixture.cjs',backend]);
  if(mode==='browser'||mode==='browser-mutants'||mode==='deferred-browser'){
   assert(fs.existsSync(backend),'fixture backend must exist');env.R266_BACKEND=backend;env.CHROME_BIN=chrome(env);result.chromium=env.CHROME_BIN;
   if(mode==='browser'){
    env.R266_BROWSER_OUT=path.join(out,'strict-browser');
    const child=await require('./lib/progress-child.cjs').runProgressChild({command:process.execPath,args:['scripts/r277-frontend-browser.cjs'],cwd:root,env,directory:env.R266_BROWSER_OUT,logFile:path.join(out,'strict-browser.log'),label:'strict-browser',idleMs:180000,totalMs:900000});
    result.runs.push({label:'strict-browser',exit:child.exit,progress:child.progress});persist();assert.equal(child.exit,0,'strict-browser failed; original log retained');
   }else if(mode==='deferred-browser'){env.R277_DEFERRED_REPORT_DIR=out;run('deferred-browser',process.execPath,['scripts/r277-deferred-assets-proof.cjs','browser']);}
   else {
    const command=browserMutationCommand(),source=fs.readFileSync(command[0],'utf8'),ids=[...source.matchAll(/\['(p\d+-[^']+)'/g)].map(match=>match[1]);
    let child;
    try{child=await require('./lib/progress-child.cjs').runProgressChild({command:process.execPath,args:command,cwd:root,env,directory:out,logFile:path.join(out,'browser-mutants.log'),ids});}
    catch(error){result.runs.push({label:'browser-mutants',command:[process.execPath,...command],exit:null,error:error.message,progress:error.progressSnapshot});persist();throw error;}
    result.runs.push({label:'browser-mutants',command:[process.execPath,...command],exit:child.exit,progress:child.progress});persist();
    assert.equal(child.exit,0,'browser-mutants failed; original log retained');
   }
  }
  if(mode==='cleanup-browser')run('cache-clean','go',['clean','-cache','-testcache']);
  result.status='PASS';
 }catch(error){result.status='FAIL';result.error=error.message;throw error;}
 finally{
  if(mode==='cleanup-browser')fs.rmSync(backend,{force:true});
  persist();
 }
}
module.exports={parse,chrome,browserMutationCommand};
if(require.main===module)main(process.argv.slice(2)).catch(error=>{console.error(error.stack);process.exitCode=1;});
