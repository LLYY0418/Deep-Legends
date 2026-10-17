'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
test('R284 P8 CSP diagnostics retain only protocol and host, including Chromium scheme tokens',async()=>{
 for(const [blockedURI,want] of [['https://user:password@game.gtimg.cn:8443/private/player.png?token=secret#player','https://game.gtimg.cn:8443'],['http://127.0.0.1:2999/private','http://127.0.0.1:2999'],['data:image/svg+xml,private','data:'],['data','data:'],['blob:https://fixture/private','blob:'],['file:///Users/private/player.png','file:'],['inline','']]){
  const listeners=new Map(),sent=[],window={addEventListener:(n,f)=>listeners.set(n,f)};
  vm.runInNewContext(fs.readFileSync(process.env.R284_RUNTIME_SOURCE||__dirname+'/runtime.js','utf8'),{window,fetch:async(_u,o)=>{sent.push(JSON.parse(o.body));return {status:204}},URL,location:{origin:'http://fixture'},AbortController,setTimeout:()=>({unref(){}}),clearTimeout(){}});
  listeners.get('securitypolicyviolation')({effectiveDirective:'img-src',blockedURI});await new Promise(setImmediate);
  const csp=sent.find(v=>v.event==='browser_error_client');assert.equal(csp.blockedOrigin,want,blockedURI);assert(!JSON.stringify(csp).includes('private'));assert(!JSON.stringify(csp).includes('password'));assert(!JSON.stringify(csp).includes('secret'));
 }
});
