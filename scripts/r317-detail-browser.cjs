'use strict';
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict'),{browser}=require('./r297-browser-harness.cjs'),{compile}=require('../backend/web/r188-harness.cjs');
const dependencies={readMatchScores:()=>new Map(),matchPlayerGroups:m=>({groups:[{placement:1,players:m.participants}]}),arenaTeamMeta:()=>({name:'我的小队'}),arenaPlacementResult:()=> 'win',escapeHTML:String,number:String,compactNumber:String,plainInteger:String,kda:String,playerParticipantName:p=>p.name,proBadgeAttributes:()=>'',iconFigure:()=>'',augmentIconFigure:id=>`<i class="r317-augment">${id}</i>`,renderMatchScoreCell:()=>'<span>8.2</span>',renderItemIcons:()=>'<i>装备</i>',isCurrentMatchParticipant:()=>false};
const {renderArenaMatchOverview}=compile(fs.readFileSync(path.join(__dirname,'../backend/web/match-details.js'),'utf8'),['renderArenaMatchOverview'],dependencies);
const out=process.env.R317_DETAIL_OUT||'/tmp/r317-details',rows=[];
(async()=>{const b=await browser({out,transform:(name,data)=>name==='index.html'?Buffer.from('<!doctype html><html><head><meta charset="utf-8"><link rel="stylesheet" href="/app.css"><link rel="stylesheet" href="/gameplay.css"><style>body{overflow:auto;padding:20px}.fixture{container:matches-column / inline-size}.r317-augment{display:block;width:30px;height:30px;background:var(--primary);color:var(--bg);font-style:normal;font-size:9px}</style></head><body><div class="fixture"></div></body></html>'):data});
 try{await b.call('Page.navigate',{url:b.origin+'/'});await b.until('document.querySelector(".fixture")');
 for(const theme of ['dark','light'])for(const width of [1280,1600,2326,380])for(const count of [3,4,5]){
  await b.call('Emulation.setDeviceMetricsOverride',{width,height:800,deviceScaleFactor:1,mobile:false});
  const html=renderArenaMatchOverview({participants:[{participantId:1,name:'玩家甲',playerRef:'p1',augmentIds:Array.from({length:count},(_,i)=>i+1),kills:3,deaths:1,assists:2,damage:1000,damageTaken:500}]});assert(html.includes(`data-augment-count="${count}"`));
  await b.evaluate(`document.documentElement.dataset.theme='${theme}';document.querySelector('.fixture').innerHTML=${JSON.stringify(html)}`);await b.evaluate('new Promise(r=>requestAnimationFrame(r))');
  const m=await b.evaluate(`(()=>{const root=document.querySelector('.fixture'),aug=document.querySelector('.arena-detail-augments'),icons=[...aug.children].map(n=>n.getBoundingClientRect()),rect=aug.getBoundingClientRect();return{lines:new Set(icons.map(r=>r.top)).size,inside:icons.every(r=>r.left>=rect.left-.5&&r.right<=rect.right+.5),overflow:root.scrollWidth-root.clientWidth}})()`);if(width>380)assert.equal(m.lines,1);assert(m.inside);assert(m.overflow<=1,JSON.stringify(m));rows.push({theme,width,count,...m});await b.shot(`detail-${theme}-${width}-${count}`);
 }
 }finally{fs.writeFileSync(path.join(out,'result.json'),JSON.stringify(rows,null,2));await b.stop()}
})().catch(error=>{console.error(error);process.exitCode=1});
