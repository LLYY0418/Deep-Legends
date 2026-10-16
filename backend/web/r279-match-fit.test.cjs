"use strict";
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {compile}=require('./r188-harness.cjs');
const source=fs.readFileSync(path.join(__dirname,'gameplay.js'),'utf8');
function run(source,sameWidth=false,positionOnly=false){
 const observed=[],attrs=new Set();let cramped=false;
 const icons={getBoundingClientRect:()=>({left:0,right:54,width:54,top:0,bottom:22,height:22})},kda={getBoundingClientRect:()=>({left:cramped?45:60,right:140,width:80,top:0,bottom:22,height:22})};
 const champion={clientWidth:124};
 const main={clientWidth:408,parentElement:{clientWidth:795},querySelector:s=>s==='.match-loadout-mini'?icons:kda,querySelectorAll:s=>[...(s.includes('.match-champion')?[champion]:[]),icons,kda],setAttribute:n=>attrs.add(n),removeAttribute:n=>attrs.delete(n)};
 const more={hidden:false,setAttribute(){}},items=[{hidden:false,getBoundingClientRect:()=>({width:20}),hasAttribute:()=>true}];
 const collection={clientWidth:168,checkVisibility:()=>true,closest:()=>({querySelector:()=>main}),querySelectorAll:()=>items,querySelector:()=>more};
 class ResizeObserver{constructor(callback){this.callback=callback;observed.push(this)}observe(n){(this.targets ||= []).push(n)}}
 const f=compile(source,['fitMatchTags'],{ResizeObserver,bindMatchTagsPopover:()=>({anchor:null})}),container={querySelectorAll:()=>[collection]};
 f.fitMatchTags(container);assert.equal(attrs.has('data-icon-overlap'),false);
 cramped=true;if(positionOnly)champion.clientWidth=107;if(!sameWidth)main.clientWidth=367;
 const changed=positionOnly?champion:sameWidth?kda:main;
 if(observed[0].targets.includes(changed))observed[0].callback();
 assert.equal(attrs.has('data-icon-overlap'),true,'internal grid changes must invalidate icon fit even when row and tag widths stay unchanged');
 assert(observed[0].targets.includes(main),'icon grid must be observed independently of tag collection');
 observed[0].callback();assert.equal(attrs.has('data-icon-overlap'),true,'observer reentry must retain the correction');
 cramped=false;main.clientWidth=450;observed[0].callback();assert.equal(attrs.has('data-icon-overlap'),false,'recovered grid must clear overlap adjustment');
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
