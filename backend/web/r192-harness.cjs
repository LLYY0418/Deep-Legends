'use strict';
const fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const web=__dirname;
const scripts=['runtime.js','demo-data.js','app.js','favorites-facade.js','gameplay.js','champions.js','friends.js','suite.js'];
async function until(check, message='fixture did not settle', timeout=5000){
 const end=Date.now()+timeout;
 while(Date.now()<end){if(check())return;await new Promise(r=>setTimeout(r,10));}
 throw Error(message);
}
async function boot(){
 const dom=new JSDOM(fs.readFileSync(path.join(web,'index.html'),'utf8'),{url:'http://127.0.0.1:1/?demo',runScripts:'outside-only',pretendToBeVisual:true});
 const w=dom.window,errors=[],observers=new Set(),NativeObserver=w.MutationObserver;
 w.MutationObserver=class extends NativeObserver{constructor(callback){super(callback);observers.add(this)}};
 w.onerror=(_message,_source,_line,_column,error)=>errors.push(String(error?.stack||_message));
 w.addEventListener('unhandledrejection',event=>errors.push(String(event.reason?.stack||event.reason)));
 w.console.error=(...args)=>errors.push(args.map(v=>String(v?.stack||v)).join(' '));
 w.IntersectionObserver=class{observe(){}unobserve(){}disconnect(){}};
 w.ResizeObserver=class{observe(){}unobserve(){}disconnect(){}};
 w.matchMedia=()=>({matches:false,addEventListener(){},removeEventListener(){}});
 w.scrollTo=()=>{};w.HTMLElement.prototype.scrollIntoView=()=>{};w.Element.prototype.scrollTo=()=>{};
 Object.assign(w,{structuredClone:globalThis.structuredClone,fetch:(input,init)=>String(input).startsWith("/api/diagnostics/client")?Promise.resolve(new globalThis.Response("{}")):globalThis.fetch(input,init),Response:globalThis.Response,Headers:globalThis.Headers,Request:globalThis.Request});
 let closed=false;
 const close=()=>{if(closed)return;closed=true;w.dispatchEvent(new w.CustomEvent('deep-legends:dispose'));for(const observer of observers)observer.disconnect();w.close();};
 try {
 for(const file of scripts){const filename=file==='gameplay.js'&&process.env.R192_SOURCE_FILE?process.env.R192_SOURCE_FILE:path.join(web,file);w.eval(fs.readFileSync(filename,'utf8'));}
 w.document.dispatchEvent(new w.Event('DOMContentLoaded',{bubbles:true}));
 await until(()=>w.document.querySelector('.match-list .match-entry')&&w.deepLegendsMatchCards,'full app did not mount');
 // Settle the global catalogs before measuring external card identities.
 await Promise.all(['/api/gameplay/perks','/api/gameplay/items'].map(url=>w.fetch(url)));
 await new Promise(r=>setTimeout(r,100));

 return {w,document:w.document,errors,close};
 } catch(error) { close(); throw error; }
}
function matches(){
 const list=Array.from({length:3},(_,i)=>({gameId:19201+i,queueId:1700,queueLabel:'斗魂竞技场',gameMode:'CHERRY',modeGroup:'arena',mapId:30,result:'win',duration:1200,subjectParticipantId:1,participants:[
  {participantId:1,championId:799,championName:'铁血狼母',gameName:'轻量主体',tagLine:'KR',placement:1,subteamId:1},
  {participantId:2,championId:111,championName:'深海泰坦',gameName:'轻量队友',placement:1,subteamId:1},
 ]}));
 const full={...structuredClone(list[0]),subjectParticipantId:4,participants:Array.from({length:16},(_,i)=>({participantId:i+1,teamId:100,championId:i===3?799:100+i,championName:i===3?'铁血狼母':`英雄${i+1}`,gameName:i===3?'补全主体':`补全玩家${i+1}`,tagLine:'KR',playerRef:`r192-player-reference-${i+1}`,placement:i===3?1:Math.floor(i/2)+1,subteamId:i===3?1:Math.floor(i/2)+1,championLevel:18,kills:3+i,deaths:2,assists:7,kda:5,damage:i===3?98209:10000+i,damageTaken:i===3?47140:5000+i,gold:17340,itemIds:[1036,0,0,0,0,0,0],win:i===3}))};
 return {list,full};
}
function mount(f,options={}){
 const host=f.document.createElement('section');f.document.body.append(host);
 const data=matches();const mounted=f.w.deepLegendsMatchCards.mount(host,{key:'r192-fixture',matches:data.list,region:'kr',disableReplay:true,...options});
 const card=id=>host.querySelector(`[data-match-id="${id}"]`);
 return{host,...data,card,destroy:()=>mounted.destroy()};
}
module.exports={boot,until,matches,mount};
