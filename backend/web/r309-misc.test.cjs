'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {read,compile}=require('./r188-harness.cjs');
test('R309 undo center follows content area for expanded/collapsed sidebar and zoomed bounds',()=>{const node={style:{}},rect={left:240,right:1440};const f=compile(read('player-tags.js'),['alignPlayerTagUndo'],{document:{querySelector:()=>({getBoundingClientRect:()=>rect})},innerWidth:1440});for(const left of [240,64,300,80]){rect.left=left;f.alignPlayerTagUndo(node);assert(Math.abs(parseFloat(node.style.left)-(left+1440)/2)<=2);}});

test('R309 focused fixtures inject each real deferred body rather than wrappers or stubs',()=>{
 const fs=require('node:fs'),{extract}=require('./r188-harness.cjs'),{join}=require('./r277-deferred-fixture.cjs');
 const source=fs.readFileSync(__dirname+'/gameplay.js','utf8'),joined=join(source);
 for(const [file,names]of [['history-filters.js',['advancedFilterContext','advancedHistoryState','advancedHistoryHasMore','loadAdvancedHistory','ensureHistoryFilterSearch']],['live-arena.js',['renderLivePlayer','renderLiveInsights','renderLiveARAMKitCombos','renderLiveRecentPositions','liveConfidenceText','liveEvidenceSuffix']]])for(const name of names){assert.equal(extract(joined,name),extract(fs.readFileSync(__dirname+'/'+file,'utf8'),name));assert(!extract(joined,name).includes('R309 deferred'));}
 assert.equal(join(source),joined,'cached extraction retains exact production bodies');
});
test('R309 deferred live combos preserve the hextech source and render real pair markup',()=>{
 const live=require('./live-arena.js');const html=live.aramKitCombos({},'hextech',{liveAugmentRecommendationSource:()=> 'hextech',liveRecommendationsFor:()=>({augmentCombinations:[{assets:[{name:'珠光护手'},{name:'最强大脑'}],games:300,winRate:55}]}),renderLiveAugmentIcon:(_value,source)=>`<img data-source="${source}">`,escapeHTML:String,rate:String,number:String});
 assert.equal((html.match(/data-source="hextech"/g)||[]).length,2);assert.match(html,/珠光护手/);assert.match(html,/最强大脑/);assert.match(html,/live-combo-pair/);
 const fs=require('node:fs'),{extract}=require('./r188-harness.cjs');assert.match(extract(fs.readFileSync(__dirname+'/gameplay.js','utf8').replaceAll('R309 deferred','R309 bridge'),'renderLiveARAMKitCombos'),/aramKitCombos\(data,source,/);
});
test('R309 deferred filter commit tolerates a tab detached before image preparation',()=>{
 const {compile,read}=require('./r188-harness.cjs');let prepared=0;
 const f=compile(read('gameplay.js'),['prepareImages'],{window:{deepLegendsOverviewArt:{prepare:()=>prepared++}},ResizeObserver:class{constructor(){assert.fail('detached tab must not create an observer')}}});
 assert.doesNotThrow(()=>f.prepareImages(null));assert.doesNotThrow(()=>f.prepareImages(undefined));assert.equal(prepared,0);
});
