'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),Module=require('node:module');
const {readTraceStream}=require('./lib/cdp-trace.cjs');
async function discardTarget(read){const calls=[];assert.equal(await read(async(method,args)=>{calls.push([method,args]);if(method==='IO.read')throw Error('discarded trace must not enter the blocking reader');},'fixture',false),'');assert.deepEqual(calls,[['IO.close',{handle:'fixture'}]]);}
test('discarded normal traces close without reading their unused bytes',()=>discardTarget(readTraceStream));
test('retained trace chunks preserve plain/base64 evidence and close the stream',async()=>{
 const calls=[],parts=[{data:'{"traceEvents":['},{data:Buffer.from(']}').toString('base64'),base64Encoded:true,eof:true}];
 const trace=await readTraceStream(async(method,args)=>{calls.push([method,args]);if(method==='IO.read')return parts.shift();},'fixture',true);
 assert.equal(trace,'{"traceEvents":[]}');assert.equal(calls.length,3);assert(calls.slice(0,2).every(([m,a])=>m==='IO.read'&&a.size===1024*1024));assert.deepEqual(calls[2],['IO.close',{handle:'fixture'}]);
});
test('a failed read still closes and retains its original error even if cleanup fails',async()=>{const failure=Error('original read failure'),calls=[];await assert.rejects(readTraceStream(async method=>{calls.push(method);throw method==='IO.read'?failure:Error('cleanup failure');},'fixture',true),e=>e===failure);assert.deepEqual(calls,['IO.read','IO.close']);});
test('a failed close cannot turn a discarded sample into success',async()=>{await assert.rejects(readTraceStream(async()=>{throw Error('close failure')},'fixture',false),/close failure/);});
test('mutation: draining a discarded trace fails the nonblocking discard target',async()=>{
 const file=path.join(__dirname,'lib/cdp-trace.cjs'),source=fs.readFileSync(file,'utf8'),changed=source.replace('if(keep)for','if(true)for');assert.notEqual(changed,source);const m=new Module(file,module);m.filename=file;m.paths=module.paths;m._compile(changed,file);await assert.rejects(discardTarget(m.exports.readTraceStream),/discarded trace must not enter the blocking reader/);
});
