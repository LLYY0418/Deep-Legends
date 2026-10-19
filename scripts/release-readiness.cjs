#!/usr/bin/env node
'use strict';
const fs=require('node:fs'),path=require('node:path'),cp=require('node:child_process'),crypto=require('node:crypto');
const {checkNotes}=require('./make-release.cjs'),{scanWeakening}=require('./lib/test-weakening.cjs');
const root=path.resolve(__dirname,'..');
const hash=bytes=>crypto.createHash('sha256').update(bytes).digest('hex');
function expectedJobs(workflow){return Object.entries(workflow.jobs).flatMap(([name,job])=>{if(job.name)throw Error('Explicit job name needs readiness mapping');let rows=[[]];for(const values of Object.values(job.strategy?.matrix||{})){if(!Array.isArray(values))throw Error('Matrix include/exclude requires explicit readiness mapping');rows=rows.flatMap(row=>values.map(value=>[...row,value]));}return rows.map(row=>row.length?`${name} (${row.join(', ')})`:name);});}
function verifyAssets({release,bytes,version,fingerprint,notes,repo}){
 const setup=`Deep-Legends-Setup-${version}-public.exe`,names=[setup,'latest.json','SHA256SUMS-public.txt'];
 if(!release?.draft||JSON.stringify(release.assets.map(a=>a.name).sort())!==JSON.stringify([...names].sort()))throw Error('Draft missing or three-asset set mismatch');
 const checked=[];
 for(const name of names){const a=release.assets.find(a=>a.name===name),body=bytes[name];if(!Number.isSafeInteger(a.id)||a.id<=0||!Buffer.isBuffer(body)||body.length===0||a.state!=='uploaded'||a.size!==body.length||a.digest!==`sha256:${hash(body)}`)throw Error(`Draft asset digest/size mismatch: ${name}`);checked.push({id:a.id,name,digest:a.digest,size:a.size,sha256:hash(body)});}
 const latest=JSON.parse(bytes['latest.json']);
 if(latest.schema!==1||latest.version!==version||latest.fingerprint!==fingerprint||latest.notes!==notes||latest.asset?.name!==setup||latest.asset.sha256!==hash(bytes[setup])||latest.asset.size!==bytes[setup].length||latest.asset.url!==`https://github.com/${repo}/releases/download/v${version}/${setup}`)throw Error('latest.json source version/fingerprint/notes/asset mismatch');
 const sums=new Map();for(const line of bytes['SHA256SUMS-public.txt'].toString('utf8').trim().split(/\r?\n/)){const m=line.match(/^([a-f0-9]{64})  ([^/\\]+)$/);if(!m||sums.has(m[2]))throw Error('Invalid SHA256SUMS');sums.set(m[2],m[1]);}
 if(sums.size!==2||sums.get(setup)!==hash(bytes[setup])||sums.get('latest.json')!==hash(bytes['latest.json']))throw Error('SHA256SUMS mismatch');
 return {latest:{version:latest.version,fingerprint:latest.fingerprint},assets:checked};
}
function assess(data){
 const {sha,branch,run,workflow,release,version}=data,checks=[],reviewReasons=[];
 const check=(name,condition,detail)=>checks.push({name,ok:Boolean(condition),detail});
 check('release branch identity',branch.startsWith('codex/release-')&&data.head===sha&&run.head_sha===sha&&run.head_branch===branch,{branch,remote:data.head,ci:run.head_sha});
 check('one complete push',run.event==='push'&&run.status==='completed'&&run.conclusion==='success'&&run.path==='.github/workflows/ci.yml',{id:run.id,event:run.event,status:run.status,conclusion:run.conclusion});
 const jobs=expectedJobs(workflow).map(name=>{const history=data.jobs.filter(j=>j.name===name).sort((a,b)=>a.observedAttempt-b.observedAttempt||a.id-b.id);const latest=history.at(-1);return {name,id:latest?.id,conclusion:latest?.conclusion,run_attempt:latest?.observedAttempt,attempts:history.map(j=>({id:j.id,run_attempt:j.observedAttempt,conclusion:j.conclusion})),steps:latest?.steps||[]};});
 check('all required jobs actually success',jobs.every(j=>j.conclusion==='success'),jobs.map(({steps,...j})=>j));
 const windows=jobs.find(j=>j.name==='windows-tests');
 for(const marker of ['run-ci-tests.cjs go backend','run-ci-tests.cjs go installer','node scripts/test-renderers.cjs desktop']){const step=workflow.jobs['windows-tests'].steps.find(s=>s.run?.includes(marker));const actual=windows?.steps.find(s=>s.name===step?.name);check('Windows '+marker,Boolean(step)&&actual?.conclusion==='success',{step:step?.name,conclusion:actual?.conclusion});}
 check('package lock and changelog',data.notes?.version===version&&Boolean(data.notes.notes),{version});
 check('Draft target',release?.draft&&release.tag_name===`v${version}`&&release.target_commitish===sha,{id:release?.id,target:release?.target_commitish,sha});
 let draftEvidence;try{draftEvidence=verifyAssets({...data,notes:data.notes.notes});check('Draft assets, digests, manifest',true,draftEvidence);}catch(e){check('Draft assets, digests, manifest',false,e.message);}
 check('no commits after green CI',data.finalHead===sha&&data.localSha===sha&&!data.dirty,{finalHead:data.finalHead,localSha:data.localSha,dirty:data.dirty});
 if(run.run_attempt!==1||jobs.some(j=>j.attempts.some(a=>a.run_attempt>1)))reviewReasons.push('manual job/workflow rerun');
 if(data.weakening.length)reviewReasons.push('test weakening scan findings');
 const infrastructure=data.changedFiles.filter(f=>/^\.github\/workflows\//.test(f)||/^scripts\/(?:release[^/]*|make-release|bump-version|build[^/]*)\.cjs$/.test(f)||/^build[^/]*\.(?:ps1|sh)$/.test(f));
 if(infrastructure.length)reviewReasons.push('CI workflow or release script changed');
 const validated=checks.every(c=>c.ok),readyToPublish=validated&&reviewReasons.length===0;
 return {schema:1,sha,branch,version,runId:run.id,runUrl:run.html_url,validated,readyToPublish,conclusion:readyToPublish?'可以直接发布':'需要 Claude 核对',reviewReasons,failedChecks:checks.filter(c=>!c.ok).map(c=>c.name),checks,jobs:jobs.map(({steps,...j})=>j),testWeakening:data.weakening,infrastructureChanges:infrastructure,draft:draftEvidence?{id:release.id,target:release.target_commitish,...draftEvidence}:null};
}
function command(name,args,options={}){return cp.execFileSync(name,args,{cwd:root,encoding:'utf8',maxBuffer:512*1024*1024,...options});}
function api(endpoint){return JSON.parse(command('gh',['api',endpoint]));}
function pages(endpoint){return JSON.parse(command('gh',['api',endpoint,'--paginate','--slurp']));}
function collect(options){
 const git=args=>command('git',args),sha=git(['rev-parse',`${options.sha}^{commit}`]).trim(),repo=options.repo||'LLYY0418/Deep-Legends',notes=checkNotes(root),version=notes.version,branch=options.branch||`codex/release-${version}`;
 if(!branch.startsWith('codex/release-')||!/^\d+$/.test(options.run)||options.draft!==`v${version}`)throw Error('Expected release-prefix --branch, numeric --run and matching --draft');
 const ref=`repos/${repo}/git/ref/heads/${branch}`,head=api(ref).object.sha,run=api(`repos/${repo}/actions/runs/${options.run}`),jobs=[],seen=new Set();
 for(let attempt=1;attempt<=run.run_attempt;attempt++)for(const page of pages(`repos/${repo}/actions/runs/${options.run}/attempts/${attempt}/jobs?per_page=100`))for(const job of page.jobs){if(!seen.has(job.id)){seen.add(job.id);jobs.push({...job,observedAttempt:attempt});}}
 let release=null,assetBytes={};
 // A missing Draft is a failed validation, never an infrastructure-only review reason.
 release=require('./release-draft.cjs').findRelease(repo,options.draft);
 if(release)for(const asset of release.assets){if(![ `Deep-Legends-Setup-${version}-public.exe`,'latest.json','SHA256SUMS-public.txt'].includes(asset.name))continue;assetBytes[asset.name]=command('gh',['api',`repos/${repo}/releases/assets/${asset.id}`,'-H','Accept: application/octet-stream'],{encoding:null});}
 const previous=api(`repos/${repo}/releases/latest`).tag_name;
 const changedFiles=git(['diff','--name-only','--no-renames','-z',previous,sha,'--']).split('\0').filter(Boolean);
 const show=(ref,file)=>{try{return git(['show',`${ref}:${file}`]);}catch(e){if(/does not exist|exists on disk, but not in|does not exist in/.test(String(e.stderr)))return '';throw e;}};
 const changes=changedFiles.map(file=>({file,before:show(previous,file),after:show(sha,file)}));
 const fingerprint=require('../desktop/source-fingerprint.cjs').sourceFingerprint(root),workflow=require('../desktop/node_modules/js-yaml').load(fs.readFileSync(path.join(root,'.github/workflows/ci.yml'),'utf8'));
 const localSha=git(['rev-parse','HEAD']).trim(),dirty=Boolean(git(['status','--porcelain','--untracked-files=all']).trim()),finalHead=api(ref).object.sha;
 return {...assess({sha,branch,repo,version,notes,fingerprint,head,finalHead,localSha,dirty,run,jobs,release,bytes:assetBytes,workflow,changedFiles,weakening:scanWeakening(changes)}),baseline:previous};
}
function parse(args){const options={};for(let i=0;i<args.length;i++){const key=args[i];if(!['--sha','--run','--draft','--branch','--repo','--json'].includes(key)||!args[i+1]||args[i+1].startsWith('--')||options[key.slice(2)])throw Error('Usage: release-readiness.cjs --sha <candidate> --run <push id> --draft <tag> [--branch <release branch>] [--json <file>]');options[key.slice(2)]=args[++i];}for(const key of ['sha','run','draft'])if(!options[key])throw Error(`Missing --${key}`);return options;}
function exitStatus(result){return result.validated?(result.readyToPublish?0:2):1;}
module.exports={expectedJobs,verifyAssets,assess,collect,parse,exitStatus};
if(require.main===module)try{const opts=parse(process.argv.slice(2)),result=collect(opts),json=JSON.stringify(result,null,2)+'\n';if(opts.json){fs.mkdirSync(path.dirname(path.resolve(opts.json)),{recursive:true});fs.writeFileSync(opts.json,json);}console.log(json);console.log(result.conclusion+(result.validated?'（核验项全部通过）':'（存在未通过核验项）'));process.exitCode=exitStatus(result);}catch(e){console.error(JSON.stringify({validated:false,readyToPublish:false,conclusion:'需要 Claude 核对',error:e.message}));process.exitCode=1;}
