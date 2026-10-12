'use strict';
// Children inherit a rejecting loopback proxy. Only loopback destinations bypass
// it; upstream failures retain their original exit code and request evidence.
const http = require('node:http');
const fs = require('node:fs'), path = require('node:path');
const {spawn} = require('node:child_process');
async function runOffline(command, args, options = {}) {
  const requests = [], sockets = new Set();
  const proxy = http.createServer((req, res) => {
    let host; try {host = new URL(req.url).hostname;} catch {host = 'invalid-target';}
    requests.push({method:req.method, host});
    res.writeHead(502); res.end('R276 upstream deliberately unavailable');
  });
  proxy.on('connect', (req, socket) => {
    requests.push({method:'CONNECT', host:req.url.split(':')[0]});
    socket.end('HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n');
  });
  proxy.on('connection', socket => {
    sockets.add(socket);socket.on('error',()=>{});socket.on('close',()=>sockets.delete(socket));
  });
  await new Promise((resolve,reject)=>{proxy.once('error',reject);proxy.listen(0,'127.0.0.1',resolve);});
  const address='http://127.0.0.1:'+proxy.address().port;
  const env={...process.env,...options.env,HTTP_PROXY:address,HTTPS_PROXY:address,ALL_PROXY:address,
    http_proxy:address,https_proxy:address,all_proxy:address,NO_PROXY:'127.0.0.1,localhost,::1',
    no_proxy:'127.0.0.1,localhost,::1',DEEP_LEGENDS_SYSTEM_PROXY:address,R276_OFFLINE_PROXY:address,
    RIOT_API_KEY:'RGAPI-00000000-0000-0000-0000-000000000000'};
  env.NODE_OPTIONS=(env.NODE_OPTIONS||'')+' --require '+JSON.stringify(path.join(__dirname,'r276-node-offline-preload.cjs'));
  let code=1;
  try {
    code=await new Promise((resolve,reject)=>{
      const child=spawn(command,args,{cwd:options.cwd||path.resolve(__dirname,'..'),env,stdio:options.stdio||'inherit'});
      child.once('error',reject);child.once('close',status=>resolve(status===null?1:status));
    });
    return {code,requests};
  } finally {
    if(options.output){fs.mkdirSync(path.dirname(options.output),{recursive:true});fs.writeFileSync(options.output,JSON.stringify({code,requests},null,2)+'\n');}
    for(const socket of sockets)socket.destroy();
    await new Promise(resolve=>proxy.close(resolve));
  }
}
module.exports={runOffline};
if(require.main===module){
  const separator=process.argv.indexOf('--');
  if(separator<0||!process.argv[separator+1])throw Error('usage: node scripts/r276-offline.cjs -- command args...');
  runOffline(process.argv[separator+1],process.argv.slice(separator+2),{output:process.env.R276_OFFLINE_OUTPUT})
    .then(result=>{process.exitCode=result.code;}).catch(error=>{console.error(error);process.exitCode=1;});
}
