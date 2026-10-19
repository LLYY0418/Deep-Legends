'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {JSDOM}=require('../../desktop/node_modules/jsdom'),{compile,escapeHTML}=require('./r188-harness.cjs');
const source=fs.readFileSync(__dirname+'/gameplay.js','utf8');
const players=Array.from({length:10},(_,i)=>({playerRef:'privacy-'+i,gameName:'R288PrivateName'+i,displayName:'R288PrivateName'+i,tagLine:'R288PrivateTag'+i,championName:i%2?'阿狸':'盖伦',championId:i%2?103:86,isCurrent:i===0,teamId:i<5?100:200}));
function assertPrivate(html){const dom=new JSDOM(html);try{const text=dom.window.document.body.textContent;for(const p of players){assert(!text.includes(p.gameName),p.gameName);assert(!text.includes(p.tagLine),p.tagLine)}assert(!html.includes('data-tooltip="R288Private'));}finally{dom.window.close()}}
test('R288 history roster uses champions for every player including self and restores names when disabled',()=>{
 const state={settings:{maskNames:true}},f=compile(source,['renderMatchPlayers','playerParticipantName','maskedPlayerName','maskedListName'],{state,escapeHTML,matchPlayerGroups:()=>({arena:false,groups:[{players:players.slice(0,5)},{players:players.slice(5)}]}),iconFigure:()=>'',arenaGroupLabel:()=>'',isCurrentMatchParticipant:p=>p.isCurrent,proBadgeAttributes:()=>''});
 assertPrivate(f.renderMatchPlayers({}));for(const p of players)assert.equal(f.playerParticipantName(p,0),p.championName);
 assert.equal(f.maskedPlayerName(players[0],0),'盖伦');assert.match(f.maskedListName(players[0],0),/^召唤师 \d+$/);
 state.settings.maskNames=false;const html=f.renderMatchPlayers({});for(const p of players)assert(html.includes(p.gameName+'#'+p.tagLine));
});
test('R288 identity/header privacy has stable numbers and no current-player exemption',()=>{
 const f=compile(source,['renderSummonerStrip'],{state:{settings:{maskNames:true},tabs:[{key:'self'}]},escapeHTML,maskedProfileIcon:()=>'<i></i>',summonerRegionChip:()=>'',renderOverviewStreak:()=>'',summonerProChip:()=>'',renderSummonerHighlights:()=>''});
 const html=f.renderSummonerStrip({player:players[0]},{key:'self'});assertPrivate(html);assert.match(html,/召唤师 \d+/);assert(!html.includes('data-copy-summoner'));
 const privacy=require('./privacy.js');const first=privacy.name(players[0],{masked:true,index:3,context:'stable-test'});assert.equal(privacy.name(players[0],{masked:true,index:8,context:'stable-test'}),first);
});
test('R288 friends use the same privacy policy and keep raw identity in state',()=>{
 const dom=new JSDOM('',{url:'https://fixture.test',runScripts:'outside-only'});try{
  dom.window.localStorage.setItem('lol-loot-mask-names','true');dom.window.eval(fs.readFileSync(__dirname+'/privacy.js','utf8'));
  const f=compile(fs.readFileSync(__dirname+'/friends.js','utf8'),['renderFriendRow'],{window:dom.window,state:{friends:players},escapeHTML,presenceKind:()=> 'online',profileIcon:()=>'',championIcon:()=>'',statusHTML:()=>''});
  const html=players.map(p=>f.renderFriendRow(p,false)).join('');assertPrivate(html);assert(!html.includes('data-game-name'));assert.match(html,/召唤师/);
 }finally{dom.window.close()}
});
