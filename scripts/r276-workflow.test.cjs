'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const root=path.resolve(__dirname,'..'),ci=fs.readFileSync(path.join(root,'.github/workflows/ci.yml'),'utf8');
function job(source,name){const start=source.indexOf('\n  '+name+':\n');assert(start>=0,name+' missing');const tail=source.slice(start+5),match=/\n  \S[^\n]*:\n/.exec(tail);return source.slice(start,match?start+5+match.index:undefined);}
function condition(expression,event,ref,input){return vm.runInNewContext(expression,{github:{event_name:event,ref},inputs:{measure_windows_renderers:input},startsWith:(value,prefix)=>value.startsWith(prefix),always:()=>true},{timeout:100,codeGeneration:{strings:false,wasm:false}});}
test('R276 complete CI excludes v tags while the unchanged release workflow keeps them',()=>{
 assert.match(ci,/push:\s+tags-ignore: \['v\*'\]/);
 const spec=require('../desktop/node_modules/js-yaml').load(ci).on.push;
 assert.deepEqual(spec.branches,['**'],'tags-ignore alone disables branch pushes');
 const branchRuns=(push,ref)=>ref.startsWith('refs/heads/') && (push.branches?.includes('**') || (!push.branches&&!push['branches-ignore']&&!push.tags&&!push['tags-ignore']));
 for(const ref of ['refs/heads/codex/ci-hardening-r276','refs/heads/codex/release-0.12.84','refs/heads/feature/nested'])assert.equal(branchRuns(spec,ref),true);
 const missingBranches={...spec};delete missingBranches.branches;assert.equal(branchRuns(missingBranches,'refs/heads/codex/ci-hardening-r276'),false,'the original omission must suppress the trigger');
 const release=fs.readFileSync(path.join(root,'.github/workflows/release.yml'),'utf8');assert.match(release,/push:\s+tags:\s+- "v\*"/);
 const ignored=ci.match(/tags-ignore: \['([^']+)'\]/)[1],released=release.match(/tags:\s+- "([^"]+)"/)[1];
 const matches=(pattern,value)=>new RegExp('^'+pattern.replace(/[.+?^${}()|[\]\\]/g,'\\$&').replace(/\*/g,'.*')+'$').test(value);
 for(const tag of ['v0.12.83','v0.13.0','v99.1.2']){assert.equal(matches(ignored,tag),true);assert.equal(matches(released,tag),true);}
 for(const tag of ['test-r276','release-without-v']){assert.equal(matches(ignored,tag),false);assert.equal(matches(released,tag),false);}
 assert.doesNotMatch(ci,/refs\/heads\/codex\/release-0\.12\.83/);
});
test('R276 mutation gates run every dispatch and release push, preserving ordinary branch policy',()=>{
 for(const name of ['mutations-1','mutations-2','mutations-3']){
  const block=job(ci,name),expression=block.match(/\n    if: ([^\n]+)/)[1];
  for(const [event,ref,want] of [['push','refs/heads/codex/release-0.12.84',true],['push','refs/heads/codex/ci-hardening-r276',true],['push','refs/heads/codex/other',false],['pull_request','refs/pull/1/merge',false],['workflow_dispatch','refs/heads/codex/ci-hardening-r276',true]])assert.equal(condition(expression,event,ref,false),want,name);
 }
});
test('R276 Windows acceptance pushes and default-true dispatch really measure',()=>{
 const block=job(ci,'windows-tests'),expression=block.match(/- name: Measure full desktop renderer tests on Windows\s+if: ([^\n]+)/)[1];
 for(const [event,ref,input,want] of [['push','refs/heads/codex/release-0.12.84',false,true],['push','refs/heads/codex/ci-hardening-r276',false,true],['workflow_dispatch','refs/heads/feature',true,true],['workflow_dispatch','refs/heads/feature',false,false],['push','refs/tags/v0.12.83',false,false]])assert.equal(condition(expression,event,ref,input),want);
 assert.match(ci,/measure_windows_renderers:[\s\S]*?default: true/);
});
test('R276 separate VMs preserve browser, mutation, upgrade and artifact gates',()=>{
 for(const name of ['quality','browser-main','browser-r265','mutations-1','mutations-2','mutations-3','windows-build','windows-tests','windows-upgrade','chromium-evidence'])assert(job(ci,name));
 const quality=job(ci,'quality');assert.match(quality,/scripts\/check-test-captures/);assert.match(quality,/scripts\/check-browser-cleanup\.cjs/);assert.match(quality,/scripts\/r276-cleanup-mutations\.cjs/);assert.match(quality,/r276-offline\.cjs -- go test -count=1 -race \.\/\.\.\./);
 const catalog=require('./r276-browser-catalog.cjs').guards.map(row=>row[0]);for(const name of ['r100','r117','r258','r264','r265','r266','r266-matrix','r267','r269-geometry','r269','r272'])assert(catalog.includes(name),name);
 const build=fs.readFileSync(path.join(root,'scripts/r276-public-fixture.cjs'),'utf8');assert.match(build,/verify-embedded-riot-key\.cjs/);assert.match(build,/--expect-fake/);assert.match(build,/--file=/);
 const upgrade=job(ci,'windows-upgrade');assert.match(upgrade,/needs: windows-build/);assert.match(upgrade,/name: deep-legends-windows-x64/);assert.match(upgrade,/r206-real-upgrade-windows\.ps1/);
 for(const module of ['backend','installer'])assert(job(ci,'windows-tests').includes('node scripts/r276-offline.cjs -- node scripts/run-ci-tests.cjs go '+module));
 assert.match(job(ci,'chromium-evidence'),/name: chromium-guards-linux\n/);
});
