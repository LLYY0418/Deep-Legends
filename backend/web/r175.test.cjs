'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const source=fs.readFileSync(path.join(__dirname,'gameplay.js'),'utf8');
function extract(name){
 const match=source.match(new RegExp('function '+name+'\\('));assert.ok(match,name);const start=match.index;
 const body=source.indexOf('{',source.indexOf(')',start));let depth=0,quote='',escaped=false;
 for(let i=body;i<source.length;i++){const c=source[i];if(quote){if(escaped)escaped=false;else if(c==='\\')escaped=true;else if(c===quote)quote='';continue;}if(c==='"'||c==="'"||c==='`'){quote=c;continue;}if(c==='{')depth++;if(c==='}'&&--depth===0)return source.slice(start,i+1);}
 assert.fail(name+' unbalanced');
}
test('R175 real anonymous recovery response renders five allied cards with the existing hidden identity style',()=>{
 const response=JSON.parse(fs.readFileSync(path.join(__dirname,'../testdata/r175-anonymous-live.json'),'utf8'));
 const names=['playerLabel','maskedPlayerName','liveHistoryStateOf','liveHistorySettled','renderLivePlayer','champSelectEnemyPlaceholder','livePremadeRoster','clusterPremadePlayers','orderLivePlayers','insightTeamLayout','isSummonersRiftMatch','renderLiveInsights'];
 const deps={state:{settings:{liveOrder:'team',maskNames:false}},SUMMONERS_RIFT_QUEUE_IDS:[420,440],liveDisplayedChampionId:()=>0,renderLivePremadeTag:()=>'',renderProIdentityBadge:()=>'',proBadgeAttributes:()=>'',iconFigure:()=>'<icon></icon>',escapeHTML:String,positionLabel:()=>'',rankTitle:()=>'',number:String,percent:String,kda:String,
 liveAugmentRecommendationSource:()=> 'hextech',recordLiveRosterRendered:()=>{},renderLiveRosterNoticeBars:()=>'',renderLiveTeamPortraitTags:()=>'',renderInsightMatches:()=>'',renderLiveRecentPositions:()=>'',isARAMRelatedMatch:()=>true,arenaLivePlayerGroups:()=>[]};
 const render=Function(...Object.keys(deps),names.map(extract).join('\n')+';return renderLiveInsights;')(...Object.values(deps));
 const dom=new JSDOM(render(response));try{
  const doc=dom.window.document,ally=doc.querySelector('.live-team.is-blue'),foe=doc.querySelector('.live-team.is-red');
  assert.equal(ally.querySelector('h3').textContent,'我方');assert.equal(ally.querySelectorAll('article.live-player').length,5);assert.equal(foe.querySelectorAll('article.live-player').length,5);
  const hidden=[...ally.querySelectorAll('article.live-player')].find(card=>card.querySelector('.player-tab-hidden'));
  assert.ok(hidden,'anonymous ally card missing');assert.equal(hidden.querySelector('.player-tab-hidden').textContent,'隐藏身份');assert.equal(hidden.querySelector('.live-player-name').disabled,true);assert.equal(hidden.querySelector('[data-player-ref]'),null);
 }finally{dom.window.close();}
});
