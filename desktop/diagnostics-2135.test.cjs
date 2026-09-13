"use strict";
const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const {JSDOM}=require('jsdom');
const WEB=path.join(__dirname,'../web');
const source=fs.readFileSync(path.join(WEB,'app.js'),'utf8');
test('2135 actual search buttons clear submitted fields, keep invalid input, and retain new typing',async()=>{
 const dom=new JSDOM(fs.readFileSync(path.join(WEB,'index.html'),'utf8'),{url:'http://localhost/?demo',runScripts:'outside-only',pretendToBeVisual:true});
 const w=dom.window;
 try{
  w.matchMedia=()=>({matches:false,addEventListener(){},removeEventListener(){}});
  w.IntersectionObserver=class{observe(){} unobserve(){} disconnect(){}};
  w.ResizeObserver=class{observe(){} unobserve(){} disconnect(){}};
  w.scrollTo=()=>{}; w.HTMLElement.prototype.scrollIntoView=()=>{}; w.Element.prototype.scrollTo=()=>{};
  w.structuredClone=structuredClone; w.fetch=fetch;w.Response=Response;w.Headers=Headers;w.Request=Request;
  for(const f of ['runtime.js','demo-data.js','app.js'])w.eval(fs.readFileSync(path.join(WEB,f),'utf8'));
  const d=w.document,name=d.getElementById('player-search-name'),tag=d.getElementById('player-search-tag'),go=d.getElementById('player-search-go');
  const events=[];w.addEventListener('deep-legends:open-player',e=>events.push(e.detail));
  name.value='';tag.value='KR1';go.click();assert.equal(events.length,0);assert.equal(tag.value,'KR1');
  d.getElementById('player-search-region').dataset.region='kr';
  name.value='Maldives';tag.value='0727';name.dispatchEvent(new w.Event('input'));
  go.click();assert.equal(events.length,1);assert.equal(events[0].gameName,'Maldives');assert.equal(events[0].tagLine,'0727');
  assert.equal(name.value,'');assert.equal(tag.value,'');assert.equal(d.getElementById('player-search-clear').hidden,true);
  name.value='Next player';await new Promise(r=>setTimeout(r,150));assert.equal(name.value,'Next player');
 }finally{dom.window.close();}
});
test('2135 glyph tone map preserves dark rim, raises highlights, and retains original colored artwork',()=>{
 const dom=new JSDOM('',{runScripts:'outside-only'});
 try{
  dom.window.eval(fs.readFileSync(path.join(WEB,'augment-artwork.js'),'utf8'));
  const p=new Uint8ClampedArray(400);
  for(let i=0;i<30;i++)p.set([51,68,71,255],i*4);
  for(let i=30;i<60;i++)p.set([156,178,181,255],i*4);
  assert.equal(dom.window.deepLegendsAugmentArtwork.tintPixels(p,10,10,'gold'),true);
  assert.deepEqual([...p.slice(0,4)],[76,78,64,255]);
  assert.deepEqual([...p.slice(120,124)],[255,237,198,255]);
 }finally{dom.window.close();}
});
