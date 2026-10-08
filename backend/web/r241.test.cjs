'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const {read,compile,escapeHTML}=require('./r188-harness.cjs'),{expand}=require('./r211-harness-support.cjs');
const source=process.env.R252_GAMEPLAY_SOURCE ? require('node:fs').readFileSync(process.env.R252_GAMEPLAY_SOURCE,'utf8') : read('gameplay.js');
function api(deps={},s=source){const merged={state:{matchTimelines:new Map()},escapeHTML,scoreBadgeChip:()=>'<b class="match-badge-chip">MVP</b>',matchTimelineKey:()=>'',renderMultiKillTag:()=>'<b class="multikill-tag">双杀</b>',...deps};return compile(s,['matchDataTags','matchKeywordMetadata','renderMatchTags','fitMatchTags','bindMatchTagsPopover'],merged);}

function fixture(){const subject={participantId:1,teamId:100,damage:10000,gold:2000,kills:12,assists:0,deaths:0,cs:200,totalHeal:400,damageTaken:3000,timeCCingOthers:50,totalDamageShieldedOnTeammates:400,damageDealtToTurrets:3000,soloKills:3,killsNearEnemyTurret:8,killsUnderOwnTurret:8,maxCsAdvantageOnLaneOpponent:60,knockEnemyIntoTeamAndKill:6,keyword:'unstoppable',multiKill:2};const players=[subject,...Array.from({length:9},(_,i)=>({...subject,participantId:i+2,teamId:i<4?100:200,damage:100,gold:1000,kills:1,cs:1,totalHeal:1,damageTaken:1,timeCCingOthers:1,totalDamageShieldedOnTeammates:1,damageDealtToTurrets:1,soloKills:0}))];return {match:{gameId:241,mapId:11,duration:1800,result:'win',participants:players},subject};}
const core=['damage','control','heal','shield','tank','kills'],rift=['efficiency','participation','tower','gold','cs','solo','dive','defense','advantage','hook'];
test('R252 missing map falls back to the server mode group; explicit maps keep precedence',()=>{
 const f=api();
 for(const [modeGroup,allowed] of [['solo',[...core,...rift]],['flex',[...core,...rift]],['match',[...core,...rift]],['aram',[...core,'efficiency','hook']],['hextech-aram',[...core,'efficiency','hook']],['hextech-classic',[...core,'efficiency','hook']],['hextech-qualifier',[...core,'efficiency','hook']],['arena',core],['unsupported',core]]) {
  for(const mapId of [0,undefined]) { const {match,subject}=fixture();Object.assign(match,{mapId,modeGroup});assert.deepEqual(f.matchDataTags(match,subject).map(t=>t.kind).sort(),allowed.slice().sort(),`${modeGroup}/${mapId}`); }
 }
 const {match,subject}=fixture();Object.assign(match,{mapId:30,modeGroup:'solo'});assert.deepEqual(f.matchDataTags(match,subject).map(t=>t.kind).sort(),core.slice().sort());
});
test('R241 map matrix filters before excluded observations are read',()=>{const f=api();for(const [mapId,allowed]of [[11,[...core,...rift]],[12,[...core,'efficiency','hook']],[30,core],[99,core]]){const {match,subject}=fixture();match.mapId=mapId;const tags=f.matchDataTags(match,subject);assert.deepEqual(tags.map(t=>t.kind).sort(),allowed.slice().sort(),`map ${mapId}`);assert(tags.every(t=>!/[★第一]|队内/.test(t.label)));if(mapId!==11){Object.defineProperty(subject,'cs',{get(){throw Error('excluded CS was read')}});f.matchDataTags(match,subject)}}const {match,subject}=fixture();assert(api().matchDataTags(match,subject).some(t=>t.label==='输出'&&t.star&&t.description.startsWith('全场最高输出')));match.participants[5].damage=20000;assert(api().matchDataTags(match,subject).some(t=>t.label==='输出'&&!t.star&&t.description.startsWith('队内最高输出')))});
test('R241 hidden tags only, pinned score and one body popup with focus/escape/scroll/touch cleanup',async()=>{
 const f=api(),{match,subject}=fixture(),dom=new JSDOM('<main>'+f.renderMatchTags(match,subject,{badge:'MVP'},{})+f.renderMatchTags(match,subject,{badge:'MVP'},{})+'</main>',{pretendToBeVisual:true});const doc=dom.window.document;
 for(const row of doc.querySelectorAll('.match-tags-collection')){Object.defineProperty(row,'clientWidth',{value:150,configurable:true});row.querySelectorAll('[data-match-tag]').forEach(n=>n.getBoundingClientRect=()=>({width:55}));}
 f.fitMatchTags(doc);const [row,second]=doc.querySelectorAll('.match-tags-collection'),more=row.querySelector('button'),popup=doc.querySelector('.match-tags-popover');
 assert.equal(row.querySelector('[data-match-tag-pinned]').hidden,false);assert.equal(more.dataset.tooltip,undefined);
 const hidden=[...row.querySelectorAll('[data-match-tag]')].filter(n=>n.hidden).map(n=>n.textContent),shown=[...row.querySelectorAll('[data-match-tag]')].filter(n=>!n.hidden).map(n=>n.textContent);
 more.focus();assert.equal(popup.parentElement,doc.body);assert.equal(popup.hidden,false);assert.deepEqual([...popup.children].map(n=>n.textContent),hidden);assert(![...popup.children].some(n=>shown.includes(n.textContent)));assert.equal(popup.querySelectorAll('[data-tooltip][tabindex="0"]').length,hidden.length);
 second.querySelector('button').focus();assert.equal(more.getAttribute('aria-expanded'),'false');assert.equal(doc.querySelectorAll('.match-tags-popover:not([hidden])').length,1);
 doc.dispatchEvent(new dom.window.KeyboardEvent('keydown',{key:'Escape',bubbles:true}));assert(popup.hidden);
 more.focus();doc.dispatchEvent(new dom.window.Event('scroll'));assert(popup.hidden);
 second.querySelector('button').focus();second.querySelector('button').blur();assert(popup.hidden);
 more.dispatchEvent(new dom.window.MouseEvent('pointerover',{bubbles:true}));assert(!popup.hidden);more.dispatchEvent(new dom.window.MouseEvent('pointerout',{bubbles:true}));await new Promise(r=>setTimeout(r,75));assert(popup.hidden);
 const down=new dom.window.Event('pointerdown',{bubbles:true});Object.defineProperty(down,'pointerType',{value:'touch'});more.dispatchEvent(down);more.focus();more.click();assert(!popup.hidden);more.click();assert(popup.hidden);
 Object.defineProperty(row,'clientWidth',{value:35});f.fitMatchTags(doc);assert.equal(row.querySelector('[data-match-tag-pinned]').hidden,false);assert.equal([...row.querySelectorAll('[data-match-tag]')].filter(n=>!n.hidden).length,1);
 more.dispatchEvent(new dom.window.MouseEvent("pointerover",{bubbles:true}));row.remove();await new Promise(setImmediate);assert(popup.hidden);
 dom.window.close();
});
test('R241 skips offscreen and unchanged-width cards',()=>{const {match,subject}=fixture(),f=api(),dom=new JSDOM(f.renderMatchTags(match,subject,{badge:'MVP'},{})),doc=dom.window.document,row=doc.querySelector('.match-tags-collection');let reads=0;Object.defineProperty(row,'clientWidth',{get(){reads++;return 150}});row.checkVisibility=()=>false;f.fitMatchTags(doc);assert.equal(reads,0);row.checkVisibility=()=>true;row.querySelectorAll('[data-match-tag]').forEach(n=>n.getBoundingClientRect=()=>({width:55}));f.fitMatchTags(doc);row.querySelectorAll('[data-match-tag]').forEach(n=>n.getBoundingClientRect=()=>{throw Error('unchanged width relayout')});f.fitMatchTags(doc);dom.window.close()});
test('R241 does not render an empty tags container',()=>{const f=api({scoreBadgeChip:()=>''}),subject={participantId:1,deaths:1};assert.equal(f.renderMatchTags({result:'win',mapId:30,participants:[subject,{}]},subject,null,{}),'')});

test('R241 numeric placement tags are absent; MVP/SVP remain pinned',()=>{const {match,subject}=fixture(),f=api({scoreBadgeChip:r=>r?.badge?`<b>${r.badge}</b>`:''});for(const rank of [2,3,8])assert.doesNotMatch(f.renderMatchTags(match,subject,{rank,total:10},{}),/match-rank-chip|第.*名/);for(const badge of ['MVP','SVP'])assert.match(f.renderMatchTags(match,subject,{badge},{}),new RegExp('data-match-tag-pinned[^>]*><b>'+badge));});
