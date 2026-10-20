'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),{JSDOM}=require('../../desktop/node_modules/jsdom');
const source=fs.readFileSync(__dirname+'/ui-preferences.js','utf8');
test('R297 local state migrates once and survives a new origin without localStorage',async()=>{
 const saved={},calls=[];
 for(const origin of ['http://127.0.0.1:47391','http://127.0.0.1:49192']){
  const dom=new JSDOM('<main id="app-frame"></main>',{url:origin,runScripts:'outside-only'}),w=dom.window;
  try{
   w.AbortSignal=AbortSignal;
   if(!calls.length)w.localStorage.setItem('lol-loot-theme','dark');else w.localStorage.setItem('lol-loot-theme','light');
   w.fetch=async(url,init)=>{calls.push([origin,init?.body]);if(init?.method==='POST'){const b=JSON.parse(init.body);for(const [k,v]of Object.entries(b.values))if(b.mode!=='migrate'||!Object.hasOwn(saved,k)){if(v===null)delete saved[k];else saved[k]=v;}}return {ok:true,json:async()=>({schema:1,values:{...saved}})}};
   w.eval(source);await w.deepLegendsPreferenceStorage.ready;
   assert.equal(w.deepLegendsPreferenceStorage.getItem('lol-loot-theme'),'dark');
   w.deepLegendsPreferenceStorage.setItem('lol-loot-settings-app-seen','true');await w.deepLegendsPreferenceStorage.flush();
   assert.equal(saved['lol-loot-settings-app-seen'],'true');assert.equal(w.document.documentElement.dataset.uiPreferences,undefined);
  }finally{w.close()}
 }
});
