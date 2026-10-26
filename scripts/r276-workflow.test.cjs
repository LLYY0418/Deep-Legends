'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const root=path.resolve(__dirname,'..'),ci=fs.readFileSync(process.env.R276_CI_WORKFLOW_SOURCE||path.join(root,'.github/workflows/ci.yml'),'utf8');
function job(source,name){const start=source.indexOf('\n  '+name+':\n');assert(start>=0,name+' missing');const tail=source.slice(start+5),match=/\n  \S[^\n]*:\n/.exec(tail);return source.slice(start,match?start+5+match.index:undefined);}
function condition(expression,event,ref,input){return vm.runInNewContext(expression.replace(/^\$\{\{\s*|\s*\}\}$/g,''),{github:{event_name:event,ref},inputs:{measure_windows_renderers:input},startsWith:(value,prefix)=>value.startsWith(prefix),always:()=>true,cancelled:()=>false},{timeout:100,codeGeneration:{strings:false,wasm:false}});}
test('R276 complete CI excludes v tags while the unchanged release workflow keeps them',()=>{
 assert.match(ci,/push:\s+tags-ignore: \['v\*'\]/);
 const spec=require('../desktop/node_modules/js-yaml').load(ci).on.push;
 assert.deepEqual(spec.branches,['**','!codex/diagnostic-r279-*'],'all existing branches remain full CI; only the newly authorized diagnostic namespace is excluded');
 const {branchRuns}=require('./lib/ci-routing.cjs');
 for(const ref of ['refs/heads/codex/ci-hardening-r276','refs/heads/codex/release-0.12.84','refs/heads/feature/nested'])assert.equal(branchRuns(spec,ref),true);
 const missingBranches={...spec};delete missingBranches.branches;assert.equal(branchRuns(missingBranches,'refs/heads/codex/ci-hardening-r276'),false,'the original omission must suppress the trigger');
 const release=fs.readFileSync(path.join(root,'.github/workflows/release.yml'),'utf8');assert.match(release,/push:\s+tags:\s+- "v\*"/);
 const ignored=ci.match(/tags-ignore: \['([^']+)'\]/)[1],released=release.match(/tags:\s+- "([^"]+)"/)[1];
 const matches=(pattern,value)=>new RegExp('^'+pattern.replace(/[.+?^${}()|[\]\\]/g,'\\$&').replace(/\*/g,'.*')+'$').test(value);
 for(const tag of ['v0.12.83','v0.13.0','v99.1.2']){assert.equal(matches(ignored,tag),true);assert.equal(matches(released,tag),true);}
 for(const tag of ['test-r276','release-without-v']){assert.equal(matches(ignored,tag),false);assert.equal(matches(released,tag),false);}
 assert.doesNotMatch(ci,/refs\/heads\/codex\/release-0\.12\.83/);
});
test('R277 release CI has no special R276 branch conditions',()=>{
 assert.doesNotMatch(ci,/github\.ref\s*==\s*['"]refs\/heads\/codex\/ci-hardening-r276['"]/,'R276 special branch must never regain acceptance privileges');
});
test('R287 mutation jobs execute every branch push without selection or a schedule',()=>{
 const workflow=require('../desktop/node_modules/js-yaml').load(ci);require('./lib/ci-contracts.cjs').verifyMutationRouting(workflow);
 assert.equal(workflow.on.schedule,undefined);
});
test('R276 Windows acceptance pushes and default-true dispatch really measure',()=>{
 const block=job(ci,'windows-tests'),expression=block.match(/- name: Measure full desktop renderer tests on Windows\s+if: ([^\n]+)/)[1];
 for(const [event,ref,input,want] of [['push','refs/heads/codex/release-0.12.84',false,true],['push','refs/heads/codex/probe-0.12.85-1',false,false],['push','refs/heads/codex/ci-hardening-r276',false,false],['workflow_dispatch','refs/heads/feature',true,true],['workflow_dispatch','refs/heads/feature',false,false],['push','refs/tags/v0.12.83',false,false]])assert.equal(condition(expression,event,ref,input),want);
 assert.match(ci,/measure_windows_renderers:[\s\S]*?default: true/);
});
test('R276 separate VMs preserve browser, mutation, upgrade and artifact gates',()=>{
 for(const name of ['quality-go','quality-web','quality-web-total','quality-mutations','browser-main','mutations-1','r277-proof-linux-2','mutations-3','windows-build','windows-tests','windows-upgrade','triage-legacy'])assert(job(ci,name));
 require('./renderer-shard-contract.cjs').verifyRendererJobs(require('../desktop/node_modules/js-yaml').load(ci));
 const quality=['quality-go','quality-web','quality-mutations'].map(n=>job(ci,n)).join('\n');assert.match(quality,/scripts\/check-test-captures/);assert.match(quality,/scripts\/check-browser-cleanup\.cjs/);assert.match(quality,/scripts\/r276-cleanup-mutations\.cjs/);assert.match(quality,/r276-offline\.cjs -- go test -count=1 -race \.\/\.\.\./);
 const catalog=require('./r276-browser-catalog.cjs').guards.map(row=>row[0]);for(const name of ['r100','r117','r258','r264','r265','r266','r266-matrix','r267','r269-geometry','r269','r272'])assert(catalog.includes(name),name);
 const build=fs.readFileSync(path.join(root,'scripts/r276-public-fixture.cjs'),'utf8');assert.match(build,/verify-embedded-riot-key\.cjs/);assert.match(build,/--expect-fake/);assert.match(build,/--file=/);
 const upgrade=job(ci,'windows-upgrade');assert.match(upgrade,/needs: windows-build/);assert.match(upgrade,/name: deep-legends-windows-x64/);assert.match(upgrade,/r206-real-upgrade-windows\.ps1/);
 for(const module of ['backend','installer'])assert(job(ci,'windows-tests').includes('node scripts/r276-offline.cjs -- node scripts/run-ci-tests.cjs go '+module));
 assert.match(job(ci,'triage-legacy'),/name: chromium-guards-linux\n/);
});
test('R307 sweep coverage and retained stress proofs have complete commands',()=>{
 const sweep=fs.readFileSync(path.join(root,'.github/workflows/flake-sweep.yml'),'utf8'),workflow=require('../desktop/node_modules/js-yaml').load(sweep);
 assert.equal(workflow.jobs.samples.strategy.matrix.include.length,32);
 const stress=require('./r276-targeted-stress.cjs');assert.deepEqual(stress.tests,['TestR196ProbeSelectsFastMirror','TestR206FullCollectionRefreshTranslationTimeoutUnderThreeSeconds']);
 for(const match of ci.matchAll(/(?:node|python3) (scripts\/[A-Za-z0-9_.\/-]+)/g))assert(fs.existsSync(path.join(root,match[1])),match[1]);
 const browserJob=fs.readFileSync(path.join(root,'scripts/r276-browser-job.cjs'),'utf8');assert.match(browserJob,/census\.requireClean\(report,name\)/);assert.match(browserJob,/selected\.filter\(row=>row\[0\]!=='r275-offline'\)/);
});

test('R276 quality prepares locked Node modules before the real geometry fixture',()=>{
 const quality=job(ci,'quality-mutations'),install=quality.indexOf('run: npm ci --prefix desktop'),geometry=quality.indexOf('run: node scripts/r276-filter-geometry-mutations.cjs');
 assert(install>=0&&geometry>install,'quality must install locked Node dependencies before geometry fixture');
});
