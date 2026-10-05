'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const {compile:compileSource,extract}=require('./r188-harness.cjs');
// The shared lightweight extractor does not parse comment apostrophes.
const compile=(source,names,deps)=>compileSource(source.replace(/^[ \t]*\/\/.*$/gm,''),names,{recordLiveProgressApply:()=>{},...deps});
const app=fs.readFileSync(path.join(__dirname,'app.js'),'utf8'),gameplay=fs.readFileSync(path.join(__dirname,'gameplay.js'),'utf8'),html=fs.readFileSync(path.join(__dirname,'index.html'),'utf8');
test('R204 Key input is masked and save/clear immediately update status without echo',async()=>{
 const dom=new JSDOM(html);const doc=dom.window.document;const el={};for(const [key,id]of Object.entries({settingRiotKeyInput:'setting-riot-key-input',settingRiotKeyState:'setting-riot-key-state',settingRiotKeySave:'setting-riot-key-save',settingRiotKeyClear:'setting-riot-key-clear',settingRiotKeyReveal:'setting-riot-key-reveal'}))el[key]=doc.getElementById(id);
 assert.equal(el.settingRiotKeyInput.type,'password');assert.equal(el.settingRiotKeySave.textContent,'保存');assert.equal(el.settingRiotKeyClear.textContent,'清除');
 const requests=[],events=[],state={};const f=compile(app,['renderRiotKeySettings','saveRiotKeySettings'],{el,state,api:async(url,opt)=>{requests.push([url,opt]);return {status:opt.method==='DELETE'?'unconfigured':'configured'}},window:{dispatchEvent:e=>events.push(e)},CustomEvent:dom.window.CustomEvent,showToast:e=>{throw Error(e)}});
 el.settingRiotKeyInput.value='fixture-key';await f.saveRiotKeySettings();assert.equal(el.settingRiotKeyState.textContent,'已配置');assert.equal(el.settingRiotKeyInput.value,'');await f.saveRiotKeySettings(true);assert.equal(requests[1][1].method,'DELETE');assert.equal(el.settingRiotKeyState.textContent,'未配置');assert.equal(events.length,2);dom.window.close();
 assert.match(app,/detail\?\.focus === "riot-key"/);assert.match(app,/settingRiotKeyInput\?\.focus/);
 assert.match(gameplay,/specialistKeyInvalid \? "Riot Key 无效"/);assert.match(gameplay,/specialistKeyUnavailable \|\| specialistRelayUnavailable \? ""/);assert.match(gameplay,/focus: "riot-key"/);
});
test('R204 dialog-restored pointer focus hides tooltip while keyboard and hover show it',()=>{
 const dom=new JSDOM('<button data-tooltip="新版本已下载，点击升级">update</button>',{pretendToBeVisual:true});const w=dom.window,doc=w.document,button=doc.querySelector('button');let keyboard=false;const matches=button.matches.bind(button);button.matches=selector=>selector===':focus-visible'?keyboard:matches(selector);button.getBoundingClientRect=()=>({left:20,top:20,width:36,height:36,bottom:56,right:56});
 w.matchMedia=()=>({matches:false});const f=compile(app,['setupFloatingTooltips'],{document:doc,window:w,Node:w.Node,Element:w.Element,HTMLElement:w.HTMLElement,requestAnimationFrame:fn=>{fn();return 1},cancelAnimationFrame(){}});f.setupFloatingTooltips();
 const tooltip=doc.querySelector('[role="tooltip"]');button.dispatchEvent(new w.FocusEvent('focusin',{bubbles:true}));assert.equal(tooltip.hidden,true);button.dispatchEvent(new w.Event('pointerover',{bubbles:true}));assert.equal(tooltip.hidden,false);button.dispatchEvent(new w.FocusEvent('focusout',{bubbles:true}));keyboard=true;button.dispatchEvent(new w.FocusEvent('focusin',{bubbles:true}));assert.equal(tooltip.hidden,false);dom.window.close();
});
test('R204 update icon and close controls use shared sizes',()=>{
 const css=fs.readFileSync(path.join(__dirname,'app.css'),'utf8');assert.match(css,/\.update-icon-button \.control-icon \{ width: 24px; height: 24px/);assert.match(css,/\.update-icon-button \{[^}]*min-width: 36px; min-height: 36px/);assert.match(css,/stroke-width: 1\.6/);assert.match(css,/stroke-width: 1\.8/);
 const doc=new JSDOM(html).window.document;for(const id of ['update-dialog-close','skin-dialog-close'])assert(doc.getElementById(id).classList.contains('dialog-close-button'));assert(doc.getElementById('career-dialog-close').classList.contains('dialog-close-button'));
});
test('R204 incremental roster rejects old requests/games/phases and shows a ready player before others',()=>{
 const state={liveLoading:true,liveProgressRequestId:'current',liveExpectedGameId:204,beacon:{phase:'ChampSelect'}};let renders=0;
 const f=compile(gameplay,['applyLivePlayerProgress','liveGamePhase'],{state,connected:()=>true,liveSnapshotBehindPhase:d=>d.phase==='Lobby',normalizeLiveGameId:n=>Number(n)||0,renderLive:()=>renders++});
 const live={available:true,phase:'ChampSelect',gameId:204,players:[{historyState:'ok'},{historyState:'pending'}]};assert.equal(f.applyLivePlayerProgress({requestId:'old',live}),false);assert.equal(f.applyLivePlayerProgress({requestId:'current',live:{...live,gameId:203}}),false);assert.equal(f.applyLivePlayerProgress({requestId:'current',live:{...live,phase:'Lobby'}}),false);assert.equal(f.applyLivePlayerProgress({requestId:'current',live}),true);assert.equal(state.live.players[0].historyState,'ok');assert.equal(state.live.players[1].historyState,'pending');assert.equal(renders,1);state.liveLoading=false;assert.equal(f.applyLivePlayerProgress({requestId:'current',live}),false);
 assert.match(app,/deep-legends:live-player-progress/);assert.match(extract(gameplay,'loadLive'),/requestId=/);
});
test('R204 missing/invalid Key renders only its title and working settings navigation',()=>{
 const {escapeHTML}=require('./r188-harness.cjs');
 for(const [reason,title] of [['riot-key-missing','未配置 Riot Key'],['riot-key-invalid','Riot Key 无效']]){
  const state={live:{},specialistRuneFailures:new Map([['fixture',{reason}]])};
  const f=compile(gameplay,['renderRuneSourceSection'],{state,escapeHTML,specialistRequestTarget:()=>({key:'fixture'}),specialistRuneFailure:v=>v});
  const doc=new JSDOM(f.renderRuneSourceSection({key:'specialist',items:[],failed:true},true)).window.document;
  assert.equal(doc.querySelector('strong').textContent,title);assert.equal(doc.querySelector('p'),null);assert.equal(doc.querySelector('[data-open-riot-settings]').textContent,'去设置');
 }
 const {harness}=require('./update-harness.cjs');const h=harness();try{
  const root=h.w.document.createElement('div');root.innerHTML='<button data-open-riot-settings>去设置</button>';h.w.document.body.append(root);
  const start=gameplay.indexOf('bindLiveNode(root?.querySelector("[data-open-riot-settings]"');const end=gameplay.indexOf('\n\t});',start)+5;
  Function('root','window','CustomEvent','bindLiveNode',gameplay.slice(start,end))(root,h.w,h.w.CustomEvent,(node,event,_key,callback)=>node.addEventListener(event,callback));root.querySelector('button').click();
  assert.equal(h.w.document.activeElement.id,'setting-riot-key-input');assert.equal(h.w.document.querySelector('[data-settings-page="privacy"]').hidden,false);
 }finally{h.close();}
});
test('R204 same-game background progress waits for the complete response',()=>{
 const old={playerRef:'opaque',historyState:'ok',recentGames:[{gameId:1}],modeStats:{games:10}};
 const previous={available:true,gameId:204,queueId:440,players:[old]};
 const state={liveLoading:true,liveProgressRequestId:'current',liveExpectedGameId:204,beacon:{phase:'ChampSelect'},live:previous};
 const f=compile(gameplay,['applyLivePlayerProgress','liveGamePhase'],{state,connected:()=>true,liveSnapshotBehindPhase:()=>false,normalizeLiveGameId:n=>Number(n)||0,renderLive(){assert.fail('background progress rendered');}});
 assert.equal(f.applyLivePlayerProgress({requestId:'current',live:{available:true,phase:'ChampSelect',gameId:204,queueId:440,players:[{playerRef:'opaque',historyState:'pending'}]}}),false);
 assert.equal(state.live,previous);assert.equal(state.live.players[0].recentGames,old.recentGames);assert.equal(state.live.players[0].historyState,'ok');
});

test('R204 first-load progress never reuses history from another game, queue or identity',()=>{
 for(const change of [{gameId:0},{gameId:205},{queueId:420},{playerRef:'other'},{hidden:true},{privateHistory:true},{identityUnresolved:true}]){
  const old={playerRef:'opaque',historyState:'ok',recentGames:[{gameId:1}]};
  const gameId=change.gameId??204;
  const state={liveLoading:true,liveAwaitingGame:true,liveProgressRequestId:'current',liveExpectedGameId:gameId,beacon:{phase:'ChampSelect'},live:{available:true,gameId:204,queueId:440,players:[old]}};
  const f=compile(gameplay,['applyLivePlayerProgress','liveGamePhase','normalizeLiveGameId'],{state,connected:()=>true,liveSnapshotBehindPhase:()=>false,renderLive(){}});
  const player={playerRef:'opaque',historyState:'pending'};
  for(const field of ['playerRef','hidden','privateHistory','identityUnresolved'])if(field in change)player[field]=change[field];
  assert(f.applyLivePlayerProgress({requestId:'current',live:{available:true,phase:'ChampSelect',gameId,queueId:change.queueId||440,players:[player]}}));
  assert.equal(state.live.players[0].historyState,'pending',JSON.stringify(change));assert.equal(state.live.players[0].recentGames,undefined,JSON.stringify(change));
 }
});
