"use strict";
const test=require("node:test"),assert=require("node:assert/strict"),fs=require("node:fs"),path=require("node:path");
const {JSDOM}=require("../../desktop/node_modules/jsdom");
const source=fs.readFileSync(path.join(__dirname,"license-ui.js"),"utf8");
function fixture(t,fetch,native){
  const dom=new JSDOM(`<html><body><div id="startup-loading" hidden></div><div id="license-overlay"><form id="license-form"><input id="license-code"><p id="license-message"></p><button id="license-submit">激活</button></form></div><div id="app-frame" hidden inert><p id="setting-license-expiry">授权到期：永久</p></div></body></html>`,{url:"http://127.0.0.1:1/",runScripts:"outside-only"});
  const w=dom.window;w.fetch=fetch;w.Response=Response;if(native)w.desktopBackend=native;w.requestAnimationFrame=callback=>w.setTimeout(callback,0);w.eval(source);t.after(()=>{w.dispatchEvent(new w.CustomEvent("deep-legends:dispose"));w.close()});return w;
}
test("R232 first frame remains pending and never sends business requests",async t=>{
  let business=0;const w=fixture(t,async url=>{if(String(url)==="/api/license/status")return new Response(JSON.stringify({state:"LOCKED"}));business++;return new Response("{}")});
  assert.equal(w.document.querySelector("#license-overlay").hidden,true);assert.equal(w.document.querySelector("#app-frame").hidden,true);
  await w.deepLegendsLicense.poll();await assert.rejects(w.fetch("/api/gameplay/live"),/取消/);assert.equal(business,0);
  w.document.querySelector("#license-overlay").hidden=true;w.document.querySelector("#app-frame").removeAttribute("inert");await assert.rejects(w.fetch("/api/watch/rules"),/取消/);assert.equal(business,0);
});
test("R232 activation coalesces submissions and replacement cancels late responses",async t=>{
  let status="LOCKED",activations=0,releaseActivation,releaseBusiness;
  const w=fixture(t,url=>{
    if(String(url)==="/api/license/status")return Promise.resolve(new Response(JSON.stringify({state:status,message:status==="REPLACED"?"注册码已在其他设备使用或已被重置":""})));
    if(String(url)==="/api/license/activate"){activations++;return new Promise(resolve=>{releaseActivation=()=>{status="ACTIVE";resolve(new Response(JSON.stringify({state:status})))}})}
    return new Promise(resolve=>{releaseBusiness=()=>resolve(new Response("private late response"))});
  });
  const form=w.document.querySelector("#license-form"),input=w.document.querySelector("#license-code");input.value="TEST-ONLY";
  form.dispatchEvent(new w.Event("submit",{cancelable:true}));form.dispatchEvent(new w.Event("submit",{cancelable:true}));assert.equal(activations,1);releaseActivation();await new Promise(r=>setImmediate(r));
  assert.equal(w.deepLegendsLicense.isActive(),true);assert.equal(input.value,"");assert.equal(w.document.querySelector("#license-overlay").hidden,true);
  const business=w.fetch("/api/gameplay/live");status="REPLACED";await w.deepLegendsLicense.poll();releaseBusiness();await assert.rejects(business,/取消/);
  assert.equal(w.document.querySelector("#app-frame").hidden,true);assert.equal(w.document.querySelector("#license-message").textContent,"注册码已在其他设备使用或已被重置");
});
test("R232 demo interceptor cannot answer license status",async t=>{
  const w=fixture(t,async()=>new Response(JSON.stringify({state:"LOCKED"})));
  w.fetch=async()=>new Response(JSON.stringify({state:"ACTIVE"}));
  await w.deepLegendsLicense.poll();assert.equal(w.deepLegendsLicense.isActive(),false);
});
test("R242 expiry row updates across ACTIVE renewals and permanent reentry",async t=>{
  let value={state:"ACTIVE",license_expires_at:Math.floor(Date.now()/1000)+23*86400};
  const w=fixture(t,async()=>new Response(JSON.stringify(value))),row=w.document.querySelector("#setting-license-expiry");
  await w.deepLegendsLicense.poll();
  const date=new Date(value.license_expires_at*1000),day=`${date.getFullYear()}-${String(date.getMonth()+1).padStart(2,"0")}-${String(date.getDate()).padStart(2,"0")}`;
  assert.equal(row.textContent,`授权到期：${day}（剩 23 天）`);
  value={state:"ACTIVE",license_expires_at:Math.floor(Date.now()/1000)+1};await w.deepLegendsLicense.poll();assert.match(row.textContent,/（剩 1 天）$/);
  value={state:"ACTIVE"};await w.deepLegendsLicense.poll();assert.equal(row.textContent,"授权到期：永久");
  value={state:"REVOKED",message:"注册码已过期"};await w.deepLegendsLicense.poll();
  assert.equal(w.document.querySelector("#license-message").textContent,"注册码已过期");assert.equal(w.deepLegendsLicense.isActive(),false);
  w.dispatchEvent(new w.Event("online"));await new Promise(r=>setImmediate(r));assert.equal(w.deepLegendsLicense.isActive(),false);
});
test("R242 native handshake retains expiry and updates same-state terminal reason",async t=>{
  let apply,value={state:"ACTIVE",license_expires_at:Math.floor(Date.now()/1000)+90*86400};
  const w=fixture(t,async()=>new Response(JSON.stringify(value)),{onLicenseApply:fn=>{apply=fn},licenseObserved(){},licenseRendered(){}});
  apply({...value,renderId:1});assert.match(w.document.querySelector("#setting-license-expiry").textContent,/（剩 90 天）$/);
  value={state:"ACTIVE",license_expires_at:Math.floor(Date.now()/1000)+30*86400};await w.deepLegendsLicense.poll();assert.match(w.document.querySelector("#setting-license-expiry").textContent,/（剩 30 天）$/);
  apply({state:"REVOKED",message:"注册码已停用",renderId:2});value={state:"REVOKED",message:"注册码已过期"};await w.deepLegendsLicense.poll();assert.equal(w.document.querySelector("#license-message").textContent,"注册码已过期");
});

test("R243 activation stays busy for the combined retry budget and native updates do not flash errors",async t=>{
 let apply,finish,activationTimeout;const w=fixture(t,url=>String(url)==="/api/license/activate"?new Promise(r=>finish=r):Promise.resolve(new Response(JSON.stringify({state:"NETWORK_LOCKED"}))),{onLicenseApply:fn=>apply=fn,licenseObserved(){},licenseRendered(){}});
 const nativeSetTimeout=w.setTimeout.bind(w);w.setTimeout=(fn,ms,...args)=>{if(ms===36000)activationTimeout=ms;return nativeSetTimeout(fn,ms,...args)};
 const form=w.document.querySelector("#license-form"),button=w.document.querySelector("#license-submit"),message=w.document.querySelector("#license-message");
 form.dispatchEvent(new w.Event("submit",{cancelable:true}));assert.equal(button.textContent,"正在激活…");assert.equal(button.disabled,true);assert.equal(activationTimeout,36000);
 apply({state:"NETWORK_LOCKED",message:"无法连接激活服务，请联网后重试",renderId:1});assert.equal(message.textContent,"正在激活…");assert.equal(button.disabled,true);
 finish(new Response(JSON.stringify({state:"ACTIVE"})));await new Promise(r=>setImmediate(r));assert.equal(button.disabled,false);assert.equal(button.textContent,"激活");
});

test('R245 expired startup shows connecting in normal tone, then a genuine failure in error tone',async t=>{
 let value={state:'NETWORK_LOCKED',message:'正在连接激活服务…'};
 const w=fixture(t,async()=>new Response(JSON.stringify(value))),message=w.document.querySelector('#license-message');
 await w.deepLegendsLicense.poll();assert.equal(message.textContent,'正在连接激活服务…');assert.equal(message.dataset.tone,'normal');assert.equal(w.deepLegendsLicense.isActive(),false);
 value={state:'NETWORK_LOCKED',message:'无法连接激活服务，请联网后重试'};await w.deepLegendsLicense.poll();assert.equal(message.dataset.tone,'error');assert.equal(message.textContent,value.message);
});
