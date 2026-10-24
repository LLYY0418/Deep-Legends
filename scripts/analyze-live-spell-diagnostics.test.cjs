'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const crypto=require('node:crypto');
const {collector}=require('./analyze-live-spell-diagnostics.cjs');
function fixture(){const c=collector(),base={capture:'test',run_id:'run',epoch:0,diagnostic_schema:2,sample:1};c.add(JSON.stringify({...base,event:'live_spell_start',sample:0}));c.add(JSON.stringify({...base,event:'live_spell_sample',completed:true,all_ok:true,requests:requests()}));return {c,base,add:r=>c.add(JSON.stringify({...base,...(r.event==='live_spell_stop'?{observations:1}:{}),...r}))}}
function requests(){return Object.fromEntries(['gamestats','playerlist','eventdata'].map(k=>[k,{result:'ok',parse_valid:true}]))}
function parts(payload){const text=JSON.stringify(payload),mid=Math.floor(text.length/2);return [text.slice(0,mid),text.slice(mid)].map(fragment=>({fragment,text}))}
function record(payload,index,count=2){const text=typeof payload==='object'?payload.text:payload;return {event:'live_spell_snapshot',observation:1,previous_observation:0,source:'playerlist',subject:'player:1',part_index:index,part_count:count,payload:typeof payload==='object'?payload.fragment:payload,payload_bytes:Buffer.byteLength(text),payload_sha256:crypto.createHash('sha256').update(text).digest('hex')}}
test('reassembles out-of-order detail fragments without losing original text',()=>{
 const f=fixture(),payload={source:'playerlist',subject:'player:1',fields:{'/summonerSpells/summonerSpellOne/rawDisplayName':{type:'string',value:'UnrecognizedSummonerVariant'}}},chunks=parts(payload);
 f.add(record(chunks[1],2));f.add(record(chunks[0],1));f.add({event:'live_spell_stop'});
 const r=f.c.finish();assert.equal(r.details_complete,true);assert.deepEqual(r.observations[0].fields,payload.fields);
});
test('missing, conflicting and malformed parts never count as complete evidence',()=>{
 for(const kind of ['missing','conflict','malformed']){
  const f=fixture(),chunks=parts({source:'playerlist',subject:'player:1',fields:{}});f.add(record(chunks[0],1));
  if(kind==='conflict'){f.add(record('different',1));f.add(record(chunks[1],2))}
  if(kind==='malformed')f.add(record('not-json',2));f.add({event:'live_spell_stop'});
  const r=f.c.finish();assert.equal(r.details_complete,false);assert(r.issues.length>0);
 }
});
test('legacy logs and missing previous observations are explicitly incomplete',()=>{
 const c=collector();c.add(JSON.stringify({event:'live_spell_player',capture:'legacy',epoch:0,sample:1,state:{}}));assert.equal(c.finish().legacy_only,true);
 const f=fixture(),payload={source:'playerlist',subject:'player:1',previous_observation:99,previous_sample:1,changes:[]};f.add({...record(JSON.stringify(payload),1,1),event:'live_spell_change',observation:100,previous_observation:99});f.add({event:'live_spell_stop'});
 assert(f.c.finish().issues.some(i=>i.reason==='missing-previous-observation'));
});
test('capture limits and absent stop marker remain visible',()=>{
 const f=fixture(),payload={source:'playerlist',subject:'player:1',fields:{},limits:['text:/rawDescription']};f.add(record(JSON.stringify(payload),1,1));
 const r=f.c.finish();assert.equal(r.details_complete,false);assert(r.issues.some(i=>i.reason==='capture-limit'));
});

test('missing sample records and failed requests cannot be mistaken for complete coverage',()=>{
 const f=fixture(),payload={source:'playerlist',subject:'player:1',fields:{}};f.add(record(JSON.stringify(payload),1,1));f.add({event:'live_spell_stop',sample:2});
 const r=f.c.finish();assert.equal(r.details_complete,false);assert.deepEqual(r.captures[0].missing_samples,[2]);
 const g=fixture();g.add(record(JSON.stringify(payload),1,1));g.add({event:'live_spell_sample',sample:2,completed:true,all_ok:false,requests:requests()});g.add({event:'live_spell_stop',sample:2});
 const failed=g.c.finish();assert.equal(failed.details_complete,true);assert.equal(failed.capture_coverage_complete,false);assert.equal(failed.captures[0].failed_samples,1);
});

test('change records require a valid matching predecessor and sample envelopes have explicit bounds',()=>{
 for(const payload of [{source:'playerlist',subject:'player:1',changes:[]},{source:'playerlist',subject:'player:1',changes:[],previous_observation:0,previous_sample:1}]){
  const f=fixture();f.add({...record(JSON.stringify(payload),1,1),event:'live_spell_change',observation:2});f.add({event:'live_spell_stop'});assert.equal(f.c.finish().details_complete,false);
 }
 for(const sample of [100001,'1',-1,1.5]){const f=fixture();f.add({event:'live_spell_sample',sample,completed:true,all_ok:true,requests:requests()});f.add({event:'live_spell_stop',sample});assert.equal(f.c.finish().details_complete,false)}
});
test('a valid JSON text corruption is detected by the payload digest',()=>{
 const f=fixture(),r=record(JSON.stringify({source:'playerlist',subject:'player:1',fields:{value:'old'}}),1,1);r.payload=r.payload.replace('old','new');f.add(r);f.add({event:'live_spell_stop'});assert(f.c.finish().issues.some(i=>i.reason==='detail-payload-digest-mismatch'));
});

test('an entirely missing observation is found from the final capture ledger',()=>{
 const f=fixture(),payload={source:'playerlist',subject:'player:1',fields:{}};f.add(record(JSON.stringify(payload),1,1));f.add({event:'live_spell_stop',observations:2});
 const r=f.c.finish();assert.equal(r.details_complete,false);assert.deepEqual(r.captures[0].missing_observations,[2]);
});

test('sample-scoped anonymous snapshots retain their scope and fragments must agree',()=>{
 const payload={source:'playerlist',subject:'player:1',fields:{}};
 const f=fixture();f.add({...record(JSON.stringify(payload),1,1),identity_scope:'sample',identity_matched:false});f.add({event:'live_spell_stop'});
 const r=f.c.finish();assert.equal(r.details_complete,true);assert.equal(r.observations[0].identity_scope,'sample');assert.equal(r.observations[0].identity_matched,false);
 const g=fixture(),chunks=parts(payload);g.add({...record(chunks[0],1),identity_scope:'sample'});g.add({...record(chunks[1],2),identity_scope:'capture'});g.add({event:'live_spell_stop'});
 assert(g.c.finish().issues.some(i=>i.reason==='conflicting-detail-envelope'));
});
