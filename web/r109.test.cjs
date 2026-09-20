'use strict';
const test=require('node:test'), assert=require('node:assert/strict'), fs=require('node:fs'), path=require('node:path'), vm=require('node:vm');
const {JSDOM}=require('../desktop/node_modules/jsdom');
const harnessSource=fs.readFileSync(path.join(__dirname,'r108.test.cjs'),'utf8');
const context={require,__dirname,module:{exports:{}}};
vm.runInNewContext(harnessSource.slice(0,harnessSource.indexOf('for(const append of'))+'\nmodule.exports={fixture,payload,compile};',context);
const {fixture,payload,compile}=context.module.exports;
const profile={backgroundSkinId:103000,backgroundSource:'gtimg',backgroundPath:'/103000.jpg'};
const ranks=[{tier:'MASTER',queueType:'RANKED_SOLO_5x5'}],masteries=[{championId:103,championPoints:100}];

test('R109 changing filter accepts complete profile without replacing the existing career samples',async()=>{
 const fresh={...payload(0,0,'hextech-aram'),player:{...payload().player,...profile},ranks,masteries};
 const h=fixture(async()=>fresh),tab={key:'fixture',region:'kr',playerRef:'fixture',matchFilter:'hextech-aram',data:{...payload(),recentRanked:{games:20}}};
 assert.equal(await h.loadOverview(tab,true),true);
 assert.equal(tab.data.player.backgroundSkinId,103000);assert.equal(tab.data.ranks[0].tier,'MASTER');assert.equal(tab.data.masteries[0].championId,103);
 assert.equal(tab.data.recentRanked.games,20);assert.equal(tab.data.matches.length,0);
});

test('R109 failed complete profile preserves verified header while authoritative empty clears rank',async()=>{
 for(const state of ['failed','available']) {
  const fresh={...payload(),ranks:[],masteries:[],capabilities:[{name:'ranked-stats',state},{name:'champion-mastery',state}]};
  const h=fixture(async()=>fresh),tab={key:'fixture',region:'kr',data:{...payload(),player:{...payload().player,...profile},ranks,masteries}};
  assert.equal(await h.loadOverview(tab,true),true);
  assert.equal(tab.data.ranks.length,state==='failed'?1:0);assert.equal(tab.data.masteries.length,state==='failed'?1:0);
  assert.equal(Boolean(tab.data.player.backgroundPath),state==='failed');
 }
});

test('R109 independent tabs keep background and rank from their own streamed and complete data',async()=>{
 const finish=new Map();
 const h=fixture(async(url,options)=>{
  const id=JSON.parse(options.body).playerRef;
  const data={...payload(0),player:{playerRef:id,region:'kr',backgroundSource:'gtimg',backgroundPath:`/${id}.jpg`},ranks:[{tier:id}],masteries:[]};
  options.onProgress(data);
  await new Promise(resolve=>finish.set(id,resolve));return data;
 });
 const tabs=['one','two','three'].map(key=>({key,playerRef:key,region:'kr',matchFilter:'all'}));
 const tasks=tabs.map(tab=>h.loadOverview(tab));
 for(const tab of tabs){assert.equal(tab.data.player.backgroundPath,`/${tab.key}.jpg`);assert.equal(tab.data.ranks[0].tier,tab.key)}
 for(const key of ['two','three','one']) finish.get(key)();
 await Promise.all(tasks);
 for(const tab of tabs){assert.equal(tab.data.player.backgroundPath,`/${tab.key}.jpg`);assert.equal(tab.data.ranks[0].tier,tab.key)}
});

test('R109 retained artwork is observed again and loaded class follows the new parent',()=>{
 const dom=new JSDOM('<main></main>',{runScripts:'outside-only'}),w=dom.window, observed=new Set();
 try {
  w.ResizeObserver=class{observe(node){observed.add(node)} unobserve(node){observed.delete(node)}};
  w.eval(fs.readFileSync(path.join(__dirname,'overview-art.js'),'utf8'));
  const main=w.document.querySelector('main');
  main.innerHTML='<section class="summoner-strip">'+w.deepLegendsOverviewArt.render(profile)+'</section>';
  const img=main.querySelector('img'),art=img.parentNode;
  Object.defineProperties(img,{complete:{value:true},naturalWidth:{value:1200},naturalHeight:{value:700}});
  w.deepLegendsOverviewArt.prepare(main);assert.ok(observed.has(art));
  art.remove();w.deepLegendsOverviewArt.prepare(main);assert.equal(observed.has(art),false);
  main.innerHTML='<section class="summoner-strip"></section>';main.firstChild.append(art);
  w.deepLegendsOverviewArt.prepare(main);
  assert.ok(observed.has(art));assert.ok(main.firstChild.classList.contains('has-loaded-image'));assert.equal(main.querySelector('img'),img);
 }finally{w.close()}
});

test('R109 missing rank distinguishes loading, unavailable and verified unranked',()=>{
 const ui=compile(['renderSummonerHighlights'],{highestCurrentRank:()=>null,escapeHTML:String,number:String});
 const render=status=>ui.renderSummonerHighlights([],[],status);
 assert.match(render({pending:true,capabilities:[]}),/正在读取/);
 assert.match(render({pending:false,capabilities:[{name:'ranked-stats',state:'failed'}]}),/暂不可用/);
 assert.doesNotMatch(render({pending:false,capabilities:[{name:'ranked-stats',state:'failed'}]}),/未定级/);
 assert.match(render({pending:false,capabilities:[{name:'ranked-stats',state:'available'}]}),/未定级/);
});
