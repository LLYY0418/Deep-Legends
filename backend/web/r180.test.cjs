'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const source=fs.readFileSync(path.join(__dirname,'gameplay.js'),'utf8');
function extract(name){const start=source.indexOf(`function ${name}(`);assert.ok(start>=0,name);const body=source.indexOf(') {',start)+2;let depth=0,quote='',escaped=false;for(let i=body;i<source.length;i++){const c=source[i];if(quote){if(escaped)escaped=false;else if(c==='\\')escaped=true;else if(c===quote)quote='';continue;}if(c==='"'||c==="'"||c==='`'){quote=c;continue;}if(c==='{')depth++;if(c==='}'&&--depth===0)return source.slice(start,i+1);}assert.fail(name);}
function compile(names,deps){return Function(...Object.keys(deps),names.map(extract).join('\n')+`\nreturn {${names.join(',')}};`)(...Object.values(deps));}
test('R180 latest merged same-queue game updates one player row, reports actual count and keeps other nodes',()=>{
 const renderer=compile(['renderLivePlayer','renderInsightMatches','champSelectEnemyPlaceholder','liveHistoryStateOf','liveHistorySettled'],{
  maskedPlayerName:p=>p.displayName,liveDisplayedChampionId:p=>p.championId,rankTitle:()=> '未定级',positionLabel:p=>p,
  renderLivePremadeTag:()=>'',proBadgeAttributes:()=>'',renderProIdentityBadge:()=>'',insightScore:()=>'',
  number:String,percent:String,kda:String,escapeHTML:String,iconFigure:(_,id)=>`<img src="champion-${id}">`
 });
 const games=Array.from({length:9},(_,i)=>({championId:101+i,championName:`hero${i}`,win:true,kills:2,deaths:1,assists:3,createdAt:1000-i}));
 const players=Array.from({length:10},(_,i)=>({playerRef:`p${i}`,displayName:`Player${i}`,teamId:i<5?100:200,isCurrent:i===0,championId:61,position:'middle',historyState:'ok',modeStats:{games:9,wins:9,losses:0,kda:5},recentRankedRecord:{games:9,wins:9,losses:0},recentGames:games}));
 const markup=(list)=>`<section id="recommendation-panel-insight" class="recommendation-panel"><div class="live-teams">${[100,200].map(team=>`<section class="live-team"><header>team</header><div class="live-player-list is-insight">${list.filter(p=>p.teamId===team).map(p=>renderer.renderLivePlayer(p,0,false,0,list,'',false,'','InProgress').replace('<article ',`<article data-live-player-row="card:${p.playerRef}" `)+renderer.renderInsightMatches(p).replace('<div ',`<div data-live-player-row="history:${p.playerRef}" `)).join('')}</div></section>`).join('')}</div></section>`;
 const dom=new JSDOM(`<main><div data-live-body>${markup(players)}</div></main>`),root=dom.window.document.querySelector('main'),reports=[];
 const patcher=compile(['liveBodyChrome','updateLivePanels','stampLiveRows','preserveLiveImages','patchLiveRosterPanel'],{
  document:dom.window.document,nodes:{liveContent:root},captureLiveScroll:()=>[],restoreLiveScroll:()=>{},bindLivePanelScope:()=>{},applyRenderedMetricStyles:()=>{},prepareImages:()=>{},recordLiveRenderRebuild:(parts,stats)=>reports.push(stats)
 });patcher.stampLiveRows(root);
 const row=(type,i)=>root.querySelector(`[data-live-player-row="${type}:p${i}"]`);
 const cards=players.map((_,i)=>row('card',i)),histories=players.map((_,i)=>row('history',i)),avatars=cards.map(c=>c.querySelector('img')),images=players.map((_,i)=>[...row('history',i).querySelectorAll('img')]);
 const changed=players.map(p=>({...p}));changed[3]={...changed[3],modeStats:{games:10,wins:10,losses:0,kda:5},recentRankedRecord:{games:10,wins:10,losses:0},recentGames:[{...games[0],championId:112,createdAt:2000},...games]};
 assert.ok(patcher.updateLivePanels(markup(changed)));
 for(let i=0;i<10;i++){
  if(i===3)continue;
  assert.equal(row('card',i),cards[i]);assert.equal(row('history',i),histories[i]);
  assert.deepEqual([...row('history',i).querySelectorAll('img')],images[i]);
 }
 assert.notEqual(row('card',3),cards[3]);assert.notEqual(row('history',3),histories[3]);
 assert.match(row('card',3).textContent,/近 10 局/);assert.match(row('card',3).textContent,/10胜 0负/);
 assert.equal(row('history',3).querySelectorAll('.insight-match').length,10);
 assert.equal(row('history',3).querySelector('img').getAttribute('src'),'champion-112');
 assert.equal(row('card',3).querySelector('img'),avatars[3]);
 for(const old of images[3])assert.ok(row('history',3).contains(old));
 assert.equal(reports.length,1);assert.equal(reports[0].rowsReplaced,1);assert.equal(reports[0].imagesRecreated,0);
 // Repeating the same result performs no further update.
 assert.ok(patcher.updateLivePanels(markup(changed)));assert.equal(reports.length,1);
 dom.window.close();
});
