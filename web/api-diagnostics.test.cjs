"use strict";
const test=require("node:test"),assert=require("node:assert/strict"),fs=require("node:fs"),path=require("node:path");
const source=fs.readFileSync(path.join(__dirname,"gameplay.js"),"utf8");
const start=source.indexOf("  async function api(");
const apiSource=source.slice(start,source.indexOf("  function connected()",start));
function fixture(fetch) {
 const state={controllers:new Map()},timers=[];
 const api=Function("state","fetch","setTimeout","clearTimeout",`${apiSource};return api;`)(state,fetch,fn=>{timers.push(fn);return 1;},()=>{});
 return {api,state,timers};
}
test("current-game API errors distinguish local HTTP, network, decode and body read failures",async()=>{
 for(const tc of [
  {fetch:async()=>({status:401}),kind:"http",status:401},
  {fetch:async()=>({status:503,ok:false,text:async()=>"failed"}),kind:"http",status:503},
  {fetch:async()=>{throw TypeError("offline")},kind:"network"},
  {fetch:async()=>({status:200,ok:true,json:async()=>{throw SyntaxError("invalid json")}}),kind:"decode",status:200},
  {fetch:async()=>({status:200,ok:true,json:async()=>{throw TypeError("body stream interrupted")}}),kind:"read",status:200},
 ]) {
  const f=fixture(tc.fetch);
  await assert.rejects(f.api("/api/gameplay/current-game"),e=>e.errorKind===tc.kind&&e.status===tc.status);
  assert.equal(f.state.controllers.size,0);
 }
});
test("current-game timeout remains distinct from cancellation by a newer request",async()=>{
 const f=fixture((_url,{signal})=>new Promise((_,reject)=>signal.addEventListener("abort",()=>{const e=Error("abort");e.name="AbortError";reject(e);}))); 
 const timed=f.api("/game");f.timers.shift()();await assert.rejects(timed,e=>e.name==="TimeoutError"&&e.errorKind==="timeout");
 const old=f.api("/game"),current=f.api("/game");
 await assert.rejects(old,e=>e.name==="RequestCancelled"&&e.errorKind==="canceled");
 f.state.controllers.get("/game").abort();await assert.rejects(current,e=>e.errorKind==="canceled");
});
