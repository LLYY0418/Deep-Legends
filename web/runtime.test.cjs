const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
function runtime() {
 const context = {window:{}};
 vm.runInNewContext(fs.readFileSync(path.join(__dirname, 'runtime.js'), 'utf8'), context);
 return context.window.deepLegendsRuntime;
}
test('diagnostic timeouts stop after one retry and remain visible to export',async()=>{
 const timers=[],sent=[];
 const context={window:{},AbortController,setTimeout:(fn,ms)=>{const timer={fn,ms};timers.push(timer);return timer;},clearTimeout:()=>{},fetch:(_url,init)=>{sent.push(JSON.parse(init.body));return new Promise((_,reject)=>init.signal.addEventListener('abort',()=>reject(Error('aborted'))));}};
 vm.runInNewContext(fs.readFileSync(path.join(__dirname,'runtime.js'),'utf8'),context);
 context.window.reportFlowDiagnostic('current_game_client','failed',{});
 assert.equal(timers[0].ms,5000);timers[0].fn();await new Promise(setImmediate);
 assert.equal(timers[1].ms,750);timers[1].fn();await new Promise(setImmediate);
 timers[2].fn();await new Promise(setImmediate);assert.equal(sent.length,2);
 context.fetch=async(_url,init)=>{sent.push(JSON.parse(init.body));return {status:204};};
 await context.window.flushFlowDiagnostics();
 assert.equal(sent.at(-1).transportFailed,2);assert.equal(sent.at(-1).transportDropped,1);assert.equal(sent.at(-1).transportErrorKind,'timeout');
});
test('export flush records latest delivery counters even when no later game event arrives',async()=>{
 const sent=[];let status=400;
 const context={window:{},fetch:(_url,init)=>{sent.push(JSON.parse(init.body));return Promise.resolve({status});}};
 vm.runInNewContext(fs.readFileSync(path.join(__dirname,'runtime.js'),'utf8'),context);
 context.window.reportFlowDiagnostic('current_game_client','failed',{errorKind:'network'});await new Promise(setImmediate);
 status=204;await context.window.flushFlowDiagnostics();
 const snapshot=sent.at(-1);assert.equal(snapshot.event,'diagnostic_delivery_client');assert.equal(snapshot.reason,'export');
 assert.equal(snapshot.transportFailed,1);assert.equal(snapshot.transportDropped,1);assert.equal(snapshot.transportPending,0);
});
test('diagnostic delivery observes HTTP failure, retries once and reports recovery counters',async()=>{
 const sent=[],timers=[];let status=500;
 const context={window:{},setTimeout:fn=>{timers.push(fn);return 1;},clearTimeout:()=>{},fetch:(_url,init)=>{sent.push(JSON.parse(init.body));return Promise.resolve({status});}};
 vm.runInNewContext(fs.readFileSync(path.join(__dirname,'runtime.js'),'utf8'),context);
 const report=context.window.reportFlowDiagnostic;
 report('current_game_client','failed',{errorKind:'timeout'});await new Promise(setImmediate);
 assert.equal(timers.length,1);status=204;timers.shift()();await new Promise(setImmediate);
 assert.equal(sent.length,2);assert.equal(sent[1].transportFailed,1);assert.equal(sent[1].transportHTTPStatus,500);
 assert.equal(sent[1].transportErrorKind,'http');assert.equal(sent[1].errorKind,'timeout');
 report('current_game_client','failed',{errorKind:'timeout'});assert.equal(sent.length,2);
});
test('failed transport does not poison duplicate suppression and strips new arbitrary text',async()=>{
 const sent=[];let fail=true;
 const context={window:{},fetch:(_url,init)=>{sent.push(JSON.parse(init.body));return fail?Promise.reject(Error('secret-token')):Promise.resolve({status:204});}};
 vm.runInNewContext(fs.readFileSync(path.join(__dirname,'runtime.js'),'utf8'),context);
 const fields={requestedPosition:'middle',resolvedPosition:'mid',requestId:1,errorKind:'private-token',transportErrorKind:'private-token',key:'private-token'};
 context.window.reportFlowDiagnostic('champ_select_filter_client','failed',fields);await new Promise(setImmediate);
 fail=false;context.window.reportFlowDiagnostic('champ_select_filter_client','failed',fields);await new Promise(setImmediate);
 assert.equal(sent.length,2);assert.equal(sent[1].transportFailed,1);assert.equal(sent[1].transportDropped,1);
 assert.equal(sent[1].requestedPosition,'middle');assert.doesNotMatch(JSON.stringify(sent),/secret|private-token/);
});
test('diagnostic in-flight limit is bounded and exposes dropped events',async()=>{
 const sent=[];const resolvers=[];
 const context={window:{},fetch:(_url,init)=>{sent.push(JSON.parse(init.body));return new Promise(r=>resolvers.push(r));}};
 vm.runInNewContext(fs.readFileSync(path.join(__dirname,'runtime.js'),'utf8'),context);
 for(let i=0;i<40;i++)context.window.reportFlowDiagnostic('champ_select_filter_client','request',{requestId:i});
 assert.equal(sent.length,32);resolvers[0]({status:204});await new Promise(setImmediate);
 context.window.reportFlowDiagnostic('champ_select_filter_client','all',{requestId:41});
 assert.equal(sent.at(-1).transportDropped,8);
 resolvers.forEach(r=>r({status:204}));
});
test('flow diagnostics whitelist fields, deduplicate repeats and tolerate offline transport', async () => {
 const sent=[]; const context={window:{},fetch:(_url,init)=>{sent.push(JSON.parse(init.body));return Promise.resolve({status:204});}};
 vm.runInNewContext(fs.readFileSync(path.join(__dirname,'runtime.js'),'utf8'),context);
 const report=context.window.reportFlowDiagnostic;
 const fields={traceId:'cg-1234567890123-1',phase:'active',playersReceived:10,token:'secret',key:'private-player',gameName:'secret-name'};
 report('current_game_client','received',fields);report('current_game_client','received',fields);
 assert.equal(sent.length,1);assert.equal(sent[0].playersReceived,10);
 assert.doesNotMatch(JSON.stringify(sent),/secret|private-player/);
 context.fetch=()=>{throw Error('offline')};
 assert.doesNotThrow(()=>report('watch_settings_client','save-failed',{revision:5}));
});
test('R70 response cache is access-ordered and bounded over 100000 inserts', () => {
 const cache = runtime().createCache({max:32});
 for(let i=0;i<100000;i++) cache.set(i, i);
 assert.equal(cache.size,32);
 assert.equal(cache.timestamps.size,32);
 assert.equal(cache.has(0),false);
 cache.get(99968); cache.set('new',1);
 assert.equal(cache.has(99968),true);
 assert.equal(cache.has(99969),false);
});

test('current game diagnostics retain KR fields but drop retired CN source and failure fields', () => {
 const sent=[]; const context={window:{},fetch:(_url,init)=>{sent.push(JSON.parse(init.body));return Promise.resolve({status:204});}};
 vm.runInNewContext(fs.readFileSync(path.join(__dirname,'runtime.js'),'utf8'),context);
 context.window.reportFlowDiagnostic('current_game_client','received',{source:'LCU-presence',rosterFailure:'roster-auth-rejected',forceRefresh:true,teamsReceived:0,httpStatus:502,cacheAgeMs:1e10,gate:'sensitive-secret'});
 assert.equal(sent[0].source,undefined); assert.equal(sent[0].rosterFailure,undefined); assert.equal(sent[0].cacheAgeMs,1000000); assert.equal(sent[0].forceRefresh,true); assert.equal(sent[0].httpStatus,502);
 assert.equal(sent[0].gate,undefined);
 context.window.reportFlowDiagnostic('current_game_client','rendered',{source:'sensitive-source',rosterFailure:'sensitive-failure',gate:'destroyed',hidden:true});
 assert.equal(sent[1].gate,'destroyed'); assert.equal(sent[1].hidden,true); assert.doesNotMatch(JSON.stringify(sent),/sensitive-/);
 context.window.reportFlowDiagnostic('current_game_client','received',{source:'OP.GG',phase:'active',teamsReceived:2});
 assert.equal(sent[2].source,'OP.GG');assert.equal(sent[2].phase,'active');assert.equal(sent[2].teamsReceived,2);
});
test('R70 cache reads never extend freshness; idle expiry is pruned on access', () => {
 let now=0;
 const cache=runtime().createCache({max:3,ttl:100,now:()=>now});
 cache.set('a',1); now=99; assert.equal(cache.get('a'),1);
 now=100; assert.equal(cache.get('a'),undefined);
 assert.equal(cache.size,0); assert.equal(cache.timestamps.size,0);
});
test('R70 cache timer disposal runs once on eviction, not on same-object update', () => {
 const disposed=[];
 const cache=runtime().createCache({max:1,dispose:v=>disposed.push(v)});
 const entry={timer:1}; cache.set('a',entry); cache.set('a',entry);
 assert.equal(disposed.length,0);
 cache.set('b',{timer:2}); assert.deepEqual(disposed,[entry]);
 cache.clear(); cache.clear(); assert.equal(disposed.length,2);
});
