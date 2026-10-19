#!/usr/bin/env node
'use strict';
const fs=require('node:fs'),path=require('node:path'),cp=require('node:child_process'),crypto=require('node:crypto');
const {checkNotes}=require('./make-release.cjs');
function disposition({event,release,sha,target}){
 if(!release)return 'build';
 if(release.draft)return 'replace-draft';
 if(event==='push'&&target===sha)return 'already-published';
 throw Error('Published Release target mismatch; refusing to replace');
}
function context(env=process.env){
 const {version}=checkNotes();
 const repo=env.GITHUB_REPOSITORY,sha=env.GITHUB_SHA,ref=env.GITHUB_REF,event=env.GITHUB_EVENT_NAME,tag=`v${version}`;
 if(!/^[\w.-]+\/[\w.-]+$/.test(repo||'')||! /^[a-f0-9]{40}$/.test(sha||''))throw Error('Invalid release repository/SHA');
 if(event==='push'){if(ref!==`refs/tags/${tag}`)throw Error('Tag does not match desktop package version');}
 else if(event==='workflow_dispatch'){if(!ref?.startsWith('refs/heads/codex/release-'))throw Error('Draft dispatch requires a codex/release- branch');}
 else throw Error('Unsupported release event');
 return {repo,sha,ref,event,tag,version};
}
function gh(args,{binary=false}={}){
 const r=cp.spawnSync('gh',args,{encoding:binary?null:'utf8',maxBuffer:512*1024*1024});
 if(r.error||r.status!==0){const e=Error(r.error?.message||String(r.stderr));e.status=Number(String(r.stderr).match(/HTTP (\d+)/)?.[1]);throw e;}
 return r.stdout;
}
function api(endpoint){return JSON.parse(gh(['api',endpoint]));}
function findRelease(repo,tag){const pages=JSON.parse(gh(['api',`repos/${repo}/releases?per_page=100`,'--paginate','--slurp']));const found=pages.flat().filter(r=>r.tag_name===tag);if(found.length>1)throw Error('Duplicate release tag metadata');return found[0]||null;}
function append(file,text){if(file)fs.appendFileSync(file,text);}
function prepare(env=process.env){
 const c=context(env);
 if(c.event==='workflow_dispatch'){
  const head=api(`repos/${c.repo}/git/ref/heads/${c.ref.slice('refs/heads/'.length)}`).object.sha;
  if(head!==c.sha)throw Error('Dispatch SHA differs from release branch head');
 }
 const release=findRelease(c.repo,c.tag);
 const target=release&&!release.draft ? (/^[a-f0-9]{40}$/.test(release.target_commitish)?release.target_commitish:api(`repos/${c.repo}/commits/${encodeURIComponent(c.tag)}`).sha):release?.target_commitish;
 const action=disposition({...c,release,target});
 append(env.GITHUB_OUTPUT,`skip=${action==='already-published'}\nversion=${c.version}\ntag=${c.tag}\n`);
 append(env.GITHUB_STEP_SUMMARY,`## Release build receipt\n\nSHA: \`${c.sha}\`\nBranch/ref: \`${c.ref}\`\nTag: \`${c.tag}\`\nAction: ${action}\n`);
 if(action==='already-published')console.log('::notice::已由预建 Draft 发布');
 return {...c,action};
}
function create(env=process.env){
 const c=context(env),release=findRelease(c.repo,c.tag);
 disposition({...c,release,target:release?.target_commitish});
 if(release)gh(['release','delete',c.tag,'--yes','--repo',c.repo]);
 const dir=path.resolve(__dirname,'../dist/release'),manifest=JSON.parse(fs.readFileSync(path.join(dir,'latest.json'),'utf8'));
 if(manifest.version!==c.version)throw Error('Release notes version mismatch');
 const names=[`Deep-Legends-Setup-${c.version}-public.exe`,'latest.json','SHA256SUMS-public.txt'];
 const notes=path.join(env.RUNNER_TEMP||require('node:os').tmpdir(),'r287-release-notes.md');fs.writeFileSync(notes,manifest.notes);
 gh(['release','create',c.tag,...names.map(name=>path.join(dir,name)),'--draft','--target',c.sha,'--title',c.tag,'--notes-file',notes,'--repo',c.repo]);
 const actual=findRelease(c.repo,c.tag);
 if(!actual?.draft||actual.target_commitish!==c.sha||JSON.stringify(actual.assets.map(a=>a.name).sort())!==JSON.stringify([...names].sort()))throw Error('Draft target/assets mismatch');
 for(const asset of actual.assets){const bytes=fs.readFileSync(path.join(dir,asset.name));if(asset.state!=='uploaded'||asset.size!==bytes.length||asset.digest!==`sha256:${crypto.createHash('sha256').update(bytes).digest('hex')}`)throw Error(`Draft asset digest/size/state mismatch: ${asset.name}`);}
 const receipt={schema:1,sha:c.sha,ref:c.ref,tag:c.tag,releaseId:actual.id,target:actual.target_commitish,fingerprint:manifest.fingerprint,assets:actual.assets.map(a=>({id:a.id,name:a.name,digest:a.digest,size:a.size,localSHA256:crypto.createHash('sha256').update(fs.readFileSync(path.join(dir,a.name))).digest('hex')}))};
 fs.writeFileSync(path.join(dir,'../release-build-receipt.json'),JSON.stringify(receipt,null,2)+'\n');
 append(env.GITHUB_STEP_SUMMARY,`\nVerified Draft receipt\n\n\`\`\`json\n${JSON.stringify(receipt,null,2)}\n\`\`\`\n`);
 return receipt;
}
module.exports={disposition,context,prepare,create,findRelease};
if(require.main===module)try{const mode=process.argv[2];if(!['prepare','create'].includes(mode))throw Error('Expected prepare or create');console.log(JSON.stringify(mode==='prepare'?prepare():create()));}catch(e){console.error(e.message);process.exitCode=1;}
