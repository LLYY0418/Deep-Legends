'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),http=require('node:http');
const {runOffline}=require('./r276-offline.cjs');
test('R276 rejecting proxy rejects HTTP and CONNECT while preserving loopback',async()=>{
 const server=http.createServer((req,res)=>res.end('fixture'));
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 try{
  const code=`const http=require('node:http'),assert=require('node:assert/strict');
  (async()=>{assert.equal(await (await fetch('http://127.0.0.1:${server.address().port}')).text(),'fixture');
  assert.throws(()=>http.get('http://upstream.invalid/'),/R276 external Node socket blocked/);
  await assert.rejects(fetch('https://upstream.invalid/'));
  for(const method of ['GET','CONNECT'])await new Promise((resolve,reject)=>{const proxy=new URL(process.env.HTTP_PROXY);const req=http.request({hostname:proxy.hostname,port:proxy.port,method,path:method==='CONNECT'?'upstream.invalid:443':'http://upstream.invalid/path'});req.on('response',r=>{assert.equal(r.statusCode,502);r.resume();r.on('end',resolve)});req.on('connect',(r,socket)=>{assert.equal(r.statusCode,502);socket.destroy();resolve()});req.on('error',reject);req.end()});
  })().catch(e=>{console.error(e);process.exitCode=1});`;
  const result=await runOffline(process.execPath,['-e',code]);assert.equal(result.code,0);assert.deepEqual(result.requests.map(r=>r.method),['GET','CONNECT']);assert(result.requests.every(r=>r.host==='upstream.invalid'));
 }finally{await new Promise(resolve=>server.close(resolve));}
});
