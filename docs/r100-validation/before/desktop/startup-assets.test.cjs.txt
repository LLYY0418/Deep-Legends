"use strict";
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),zlib=require('node:zlib');
const {JSDOM}=require('jsdom');
const web=path.join(__dirname,'../web'),html=fs.readFileSync(path.join(web,'index.html'),'utf8');
test('R86 initial CSS and JS gzip budget excludes dynamic demo data',()=>{
 const dom=new JSDOM(html);try{
 const scripts=[...dom.window.document.querySelectorAll('script[src]')].map(n=>n.getAttribute('src'));
 assert.ok(!scripts.includes('/demo-data.js'));
 const assets=[...scripts,...[...dom.window.document.querySelectorAll('link[rel="stylesheet"]')].map(n=>n.getAttribute('href'))];
 const bytes=assets.reduce((sum,file)=>sum+zlib.gzipSync(fs.readFileSync(path.join(web,file)),{level:1}).length,0);
 assert.ok(bytes<420*1024,`initial gzip=${bytes}`);
 }finally{dom.window.close()}
});
test('R86 demo loads only on demand, queues calls, shows badge, and preserves native fallback',async()=>{
 for(const suffix of ['', '?demo', '#demo']){
  const dom=new JSDOM(html,{url:'http://localhost/'+suffix,runScripts:'outside-only'}),w=dom.window;
  try{
   let calls=0;Object.assign(w,{fetch:async()=>{calls++;return new Response('{}')},Response,Headers,Request,structuredClone});
   w.eval(fs.readFileSync(path.join(web,'runtime.js'),'utf8'));
   const script=w.document.querySelector('script[src="/demo-data.js"]');
   if(!suffix){assert.equal(script,null);continue}
   assert.ok(script);const pending=w.fetch('/api/status');await Promise.resolve();assert.equal(calls,0);
   w.eval(fs.readFileSync(path.join(web,'demo-data.js'),'utf8'));
   assert.equal((await (await pending).json()).connected,true);
   assert.match(w.document.body.textContent,/演示数据/);
   await w.fetch('/non-demo-endpoint');assert.equal(calls,1);
  }finally{w.close()}
 }
});
test('R86 demo-only loading rejects an independent production script route',()=>{
 const source=fs.readFileSync(path.join(web,'runtime.js'),'utf8');
 function guard(source){const dom=new JSDOM(html,{url:'http://localhost/',runScripts:'outside-only'});try{dom.window.fetch=async()=>new Response('{}');dom.window.eval(source);assert.equal(dom.window.document.querySelector('script[src="/demo-data.js"]'),null)}finally{dom.window.close()}}
 guard(source);
 assert.throws(()=>guard(source+'\n {const bypass=document.createElement("script");bypass.src="/demo-data.js";document.head.appendChild(bypass)}'),{name:'AssertionError'});
});
