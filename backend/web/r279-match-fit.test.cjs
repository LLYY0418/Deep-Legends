"use strict";
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {compile}=require('./r188-harness.cjs');
const source=fs.readFileSync(path.join(__dirname,'gameplay.js'),'utf8');
function run(source,sameWidth=false,positionOnly=false){
 const observed=[],attrs=new Set(),frames=[];let cramped=false,insideObserver=false,frameId=0;
 const requestAnimationFrame=callback=>{frames.push(callback);return ++frameId;};
 const flush=()=>{while(frames.length)frames.shift()();};
 const icons={getBoundingClientRect:()=>({left:0,right:54,width:54,top:0,bottom:22,height:22})},kda={getBoundingClientRect:()=>({left:cramped?45:60,right:140,width:80,top:0,bottom:22,height:22})};
 const champion={clientWidth:124};
 const main={clientWidth:408,parentElement:{clientWidth:795},querySelector:s=>s==='.match-loadout-mini'?icons:kda,querySelectorAll:s=>[...(s.includes('.match-champion')?[champion]:[]),icons,kda],setAttribute:n=>{assert.equal(insideObserver,false,'icon correction must run outside ResizeObserver delivery');attrs.add(n)},removeAttribute:n=>attrs.delete(n)};
 const more={hidden:false,setAttribute(){}},items=[{hidden:false,getBoundingClientRect:()=>({width:20}),hasAttribute:()=>true}];
 const collection={clientWidth:168,checkVisibility:()=>true,closest:()=>({querySelector:()=>main}),querySelectorAll:()=>items,querySelector:()=>more};
 class ResizeObserver{constructor(callback){this.callback=()=>{insideObserver=true;try{callback()}finally{insideObserver=false}};observed.push(this)}observe(n){(this.targets ||= []).push(n)}}
 const f=compile(source,['fitMatchTags'],{ResizeObserver,requestAnimationFrame,bindMatchTagsPopover:()=>({anchor:null})}),container={querySelectorAll:()=>[collection]};
 f.fitMatchTags(container);assert.equal(attrs.has('data-icon-overlap'),false);
 cramped=true;if(positionOnly)champion.clientWidth=107;if(!sameWidth)main.clientWidth=367;
 const changed=positionOnly?champion:sameWidth?kda:main;
 if(observed[0].targets.includes(changed)){observed[0].callback();observed[0].callback();assert.equal(frames.length,1,'observer deliveries must coalesce into one correction frame');flush();}
 assert.equal(attrs.has('data-icon-overlap'),true,'internal grid changes must invalidate icon fit even when row and tag widths stay unchanged');
 assert(observed[0].targets.includes(main),'icon grid must be observed independently of tag collection');
 observed[0].callback();flush();assert.equal(attrs.has('data-icon-overlap'),true,'observer reentry must retain the correction');
 cramped=false;main.clientWidth=450;observed[0].callback();flush();assert.equal(attrs.has('data-icon-overlap'),false,'recovered grid must clear overlap adjustment');
}
test('match icon fitting follows internal grid width independently of tag width',()=>run(source));
test('font or text geometry changes at the same main width are still measured',()=>run(source,true));
test('mutation: retaining the tag-width early return must fail icon overlap target',()=>{
 const broken=source.replace('const main=collection.closest?', 'if(collection._tagFitWidth===collection.clientWidth)return;const main=collection.closest?');
 assert.notEqual(broken,source);assert.throws(()=>run(broken),/internal grid changes must invalidate icon fit/);
});

test('mutation: omitting child geometry observations must fail same-width overlap target',()=>{
 const broken=source.replace("for(const target of main.querySelectorAll('.match-champion,.match-loadout-mini,.match-kda'))collection._tagObserver.observe(target);",'');
 assert.notEqual(broken,source);assert.throws(()=>run(broken,true),/internal grid changes must invalidate icon fit/);
});

test('container track changes are measured when tag/main/KDA/loadout dimensions stay unchanged',()=>run(source,true,true));
test('mutation: omitting champion track observation must fail position-only overlap target',()=>{
 const broken=source.replace("main.querySelectorAll('.match-champion,.match-loadout-mini,.match-kda')","main.querySelectorAll('.match-loadout-mini,.match-kda')");
 assert.notEqual(broken,source);assert.throws(()=>run(broken,true,true),/internal grid changes must invalidate icon fit/);
});

test('mutation: synchronous observer writes must fail the error-free correction target',()=>{
 const anchor='new ResizeObserver(()=>queueFit({collection,popup}))';
 const broken=source.replace(anchor,'new ResizeObserver(()=>batch([{collection,popup}]))');
 assert.notEqual(broken,source);assert.throws(()=>run(broken,true,true),/icon correction must run outside ResizeObserver delivery/);
});

function tagBatchTarget(source){
 let dirty=false,flushes=0,width=35;
 const read=()=>{if(dirty){flushes++;dirty=false;}};
 const flag=initial=>{let value=initial;return {get hidden(){return value},set hidden(next){if(value!==next)dirty=true;value=next}};};
 const collections=Array.from({length:20},()=>{
  const items=Array.from({length:3},(_,i)=>Object.assign(flag(false),{getBoundingClientRect(){read();return {width:20}},hasAttribute:()=>i===0}));
  const more=Object.assign(flag(true),{setAttribute(){}});
  return {get clientWidth(){read();return width},checkVisibility:()=>true,closest:()=>null,querySelectorAll:()=>items,querySelector:()=>more,items,more};
 });
 const f=compile(source,['fitMatchTags'],{bindMatchTagsPopover:()=>({anchor:null})}),container={querySelectorAll:()=>collections};
 f.fitMatchTags(container);assert(collections.every(c=>c.items.slice(1).every(n=>n.hidden)));
 width=100;dirty=false;flushes=0;f.fitMatchTags(container);
 assert(collections.every(c=>c.items.every(n=>!n.hidden)&&c.more.hidden),'wider rows must restore every fitting tag');
 assert(flushes<=2,'a list resize must batch tag writes before measuring, observed layout flushes='+flushes);
}

test('fitting resized tag rows batches layout reads across the retained list',()=>tagBatchTarget(source));
test('mutation: fitting each row separately must fail the layout batching target',()=>{
 const broken=source.replace('batch(fits);','for(const job of fits)batch([job]);');assert.notEqual(broken,source);assert.throws(()=>tagBatchTarget(broken),/observed layout flushes/);
});

function observerBatchTarget(source,detach=false,separate=false){
 let width=35,dirty=false,flushes=0;const frames=[],observers=[];
 const read=()=>{if(dirty){flushes++;dirty=false;}};
 const flag=initial=>{let value=initial;return {get hidden(){return value},set hidden(next){if(value!==next)dirty=true;value=next}};};
 const collections=Array.from({length:20},()=>{
  const items=Array.from({length:3},(_,i)=>Object.assign(flag(false),{getBoundingClientRect(){read();return {width:20}},hasAttribute:()=>i===0}));
  const more=Object.assign(flag(true),{setAttribute(){}});
  return {get clientWidth(){read();return width},checkVisibility:()=>true,closest:()=>null,querySelectorAll:()=>items,querySelector:()=>more,items,more};
 });
 class ResizeObserver{constructor(callback){this.callback=callback;observers.push(this)}observe(){}}
 const f=compile(source,['fitMatchTags'],{ResizeObserver,requestAnimationFrame:callback=>{frames.push(callback);return frames.length},bindMatchTagsPopover:()=>({anchor:null})});
 const doc={};
 if(separate)for(const collection of collections)f.fitMatchTags({ownerDocument:doc,querySelectorAll:()=>[collection]});
 else f.fitMatchTags({querySelectorAll:()=>collections});
 width=100;dirty=false;flushes=0;
 for(const observer of observers)observer.callback();
 assert.equal(frames.length,1,'one list resize must schedule a single correction frame across all rows');
 if(detach)for(const collection of collections)collection.isConnected=false;
 while(frames.length)frames.shift()();
 if(detach){assert(collections.every(c=>c.items.slice(1).every(n=>n.hidden)&&!c.more.hidden),'detached queued rows must retain their last fitted state');return;}
 assert(collections.every(c=>c.items.every(n=>!n.hidden)&&c.more.hidden),'one shared correction must restore every fitting tag');
 assert(flushes<=2,'observer corrections must batch row writes before measuring, observed layout flushes='+flushes);
}
test('one resize delivery batches all observed match rows in one frame',()=>observerBatchTarget(source));

test('mutation: scheduling each observed row separately fails the shared-frame target',()=>{
 const broken=source.replace('scheduler._tagFitFrame=requestAnimationFrame','job.collection._tagFitFrame=requestAnimationFrame');
 assert.notEqual(broken,source);assert.throws(()=>observerBatchTarget(broken),/one list resize must schedule a single correction frame/);
});

test('queued rows detached before their correction frame are skipped',()=>observerBatchTarget(source,true));

test('rows bound through separate containers share their document correction frame',()=>observerBatchTarget(source,false,true));
