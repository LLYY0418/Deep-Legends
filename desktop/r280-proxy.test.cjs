'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm'),{EventEmitter}=require('node:events');
const {extract}=require('../backend/web/r188-harness.cjs'),source=fs.readFileSync(__dirname+'/main.cjs','utf8');
test('R280 proxy refresh sends only changed resolutions, and failures remain retryable',async()=>{
 let proxy='http://127.0.0.1:7890',status=200;const posts=[];
 const box={loadDeferredModules(){},readSystemProxyTargets:async()=>({'lol-api-champion.op.gg':'https://lol-api-champion.op.gg/','kr.api.riotgames.com':'https://kr.api.riotgames.com/'}),resolveSystemProxy:async(_session,target)=>target.includes('riotgames')?'http://127.0.0.1:7892':proxy,session:{defaultSession:{}},backendReady:{baseUrl:'http://127.0.0.1:1',token:'fixture'},Buffer,appendDesktopLog(){},http:{request(_url,_options,callback){const r=new EventEmitter();r.end=body=>{posts.push(JSON.parse(body));callback({statusCode:status,resume(){}})};return r;}}};
 vm.createContext(box);vm.runInContext('let lastSystemProxy,proxyPushPending=false;\n'+extract(source,'pushSystemProxy'),box);
 await box.pushSystemProxy();await box.pushSystemProxy();assert.equal(posts.length,1);
 assert.equal(posts[0].routes['kr.api.riotgames.com'],'http://127.0.0.1:7892');
 proxy='';await box.pushSystemProxy();assert.equal(posts[1].proxy,'');assert.equal(posts[1].routes['lol-api-champion.op.gg'],'');assert.equal(posts.length,2);
 proxy='http://127.0.0.1:7891';status=503;await box.pushSystemProxy();status=200;await box.pushSystemProxy();assert.equal(posts.length,4);
 box.backendReady={baseUrl:'http://127.0.0.1:2',token:'new-fixture'};await box.pushSystemProxy();assert.equal(posts.length,5,'new backend must receive unchanged routes');
 assert.match(source,/mainWindow\.on\("focus",[^\n]+pushSystemProxy/);assert.match(source,/powerMonitor\.on\("resume",[^\n]+pushSystemProxy/);assert.match(source,/setInterval\(\(\)=>void pushSystemProxy\(\),60000\)/);
});
