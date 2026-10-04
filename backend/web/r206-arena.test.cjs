'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {compile:compileSource,escapeHTML}=require('./r188-harness.cjs');
const compile=(source,names,deps)=>compileSource(source.replace(/^[ \t]*\/\/.*$/gm,''),names,deps);
const source=fs.readFileSync(path.join(__dirname,'champions.js'),'utf8');
test('R206 partial arena keeps successful sections and retries only the requested block',async()=>{
 const detail={failedBlocks:['items'],build:{coreItems:['old'],skills:['unchanged']},arenaAugments:['ready']};const state={mode:'arena',selected:{champion:'fixture',championId:67},arenaRequestToken:2,detail};const requests=[];
 const f=compile(source,['renderArenaDetailContent','renderArenaBlockFailure','retryArenaBlock'],{state,escapeHTML,renderArenaAugmentSection:()=>'<div>ready augments</div>',renderArenaItemSection:()=>'',renderArenaCoreSection:()=>'<div>ready build</div>',renderArenaFirstPlaces:()=>'',renderArenaSynergySection:()=>'',render(){},api:async(url)=>{requests.push(url);return {build:{coreItems:['new'],prismItems:['prism']}}}});
 const html=f.renderArenaDetailContent(detail);assert.match(html,/ready augments/);assert.match(html,/ready build/);assert.match(html,/加载失败 · 重试/);
 await f.retryArenaBlock('items');assert.deepEqual(requests,['/api/champions/detail?mode=arena&champion=fixture&block=items']);assert.deepEqual(state.detail.build.skills,['unchanged']);assert.deepEqual(state.detail.arenaAugments,['ready']);assert.deepEqual(state.detail.failedBlocks,[]);
});
