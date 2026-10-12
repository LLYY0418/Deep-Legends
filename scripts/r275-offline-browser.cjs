'use strict';
// Real R267 page/guards, with backend HTTP(S) upstreams sent to a rejecting
// loopback proxy. Browser-side nonlocal requests retain their original block.
const http=require('node:http'),fs=require('node:fs'),path=require('node:path'),os=require('node:os'),assert=require('node:assert/strict'),{spawn}=require('node:child_process');
const {cleanupChromeTest}=require('./chrome-test-cleanup.cjs');
const root=path.resolve(__dirname,'..'),out=path.resolve(process.env.R275_OFFLINE_OUT||path.join(os.tmpdir(),'r275-offline'));
const binary=process.env.R267_BACKEND;if(!binary)throw Error('R267_BACKEND must name the synthetic public build');
let backend;const requests=[],sockets=new Set();
const proxy=http.createServer((req,res)=>{requests.push({method:req.method,host:new URL(req.url).hostname});res.writeHead(502);res.end('R275 upstream deliberately unavailable');});
proxy.on('connect',(req,socket)=>{requests.push({method:'CONNECT',host:req.url.split(':')[0]});socket.end('HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n');});
proxy.on('connection',socket=>{sockets.add(socket);socket.on('error',()=>{});socket.on('close',()=>sockets.delete(socket));});
const run=(command,args,env,log)=>new Promise((resolve,reject)=>{const child=spawn(command,args,{cwd:root,env,stdio:['ignore','pipe','pipe']});const file=fs.createWriteStream(log);child.stdout.pipe(file,{end:false});child.stderr.pipe(file,{end:false});child.on('error',reject);child.on('close',code=>{file.end();resolve(code);});});
async function main(){
 fs.mkdirSync(out,{recursive:true});await new Promise(resolve=>proxy.listen(0,'127.0.0.1',resolve));const address='http://127.0.0.1:'+proxy.address().port;
 const env={...process.env,HTTP_PROXY:address,HTTPS_PROXY:address,ALL_PROXY:address,http_proxy:address,https_proxy:address,NO_PROXY:'127.0.0.1,localhost',no_proxy:'127.0.0.1,localhost',DEEP_LEGENDS_SYSTEM_PROXY:address,RIOT_API_KEY:'RGAPI-00000000-0000-0000-0000-000000000000'};
 const data=fs.mkdtempSync(path.join(os.tmpdir(),'r275-control-'));
 try{
  backend=spawn(binary,['--desktop'],{cwd:root,env:{...env,LOL_LOOT_DATA_DIR:data},stdio:['ignore','pipe','pipe']});
  const ready=await new Promise((resolve,reject)=>{let text='';const timer=setTimeout(()=>reject(Error('offline control backend timeout')),20000);backend.on('error',reject);backend.stdout.on('data',chunk=>{text+=chunk;for(const line of text.split('\n'))try{const row=JSON.parse(line.replace(/^LOOT_READY /,''));if(row.baseUrl){clearTimeout(timer);resolve(row);}}catch{}});});
  const asset='/latest/plugins/rcp-fe-lol-collections/global/default/images/item-element/crest-and-banner-mastery-10.png';
  const response=await fetch(ready.baseUrl+'/api/champion-asset?source=communitydragon&path='+encodeURIComponent(asset),{headers:{'X-Local-Token':ready.token}});
  assert.notEqual(response.status,200,'offline control unexpectedly reached an upstream');assert.ok(requests.some(r=>r.host.includes('communitydragon')),'upstream control did not use the rejecting proxy');
  fs.writeFileSync(path.join(out,'control.json'),JSON.stringify({status:response.status,requests},null,2));
 }finally{await cleanupChromeTest({children:[backend],directories:[data]});}
 for(const name of ['no-champion-fixture','no-spells-fixture','fixed']){
  const mutant=name!=='fixed',folder=path.join(out,name);fs.mkdirSync(folder,{recursive:true});
  // Compile a temporary in-memory variant under the real script filename so
  // every page assertion, backend and network guard remains identical.
  const code=`const fs=require('node:fs'),Module=require('node:module');const filename=${JSON.stringify(path.join(root,'scripts/r267-browser.cjs'))};let source=fs.readFileSync(filename,'utf8');${name==='no-champion-fixture'?`source=source.replace("['/api/image','/api/champion-asset'].includes(pathname)","pathname==='/api/image'");`:name==='no-spells-fixture'?`source=source.replace("pathname==='/api/gameplay/summoner-spells'","pathname==='/disabled-spell-fixture'");`:''}const m=new Module(filename,module);m.filename=filename;m.paths=Module._nodeModulePaths(require('node:path').dirname(filename));m._compile(source,filename);`;
  const exitCode=await run(process.execPath,['-e',code],{...env,R267_BROWSER_OUT:folder},path.join(out,name+'.log'));
  assert.equal(exitCode,mutant?1:0,name+' unexpected result');
  if(mutant){const failure=JSON.parse(fs.readFileSync(path.join(folder,'failure.json'),'utf8'));assert.ok(failure.errors.some(row=>JSON.stringify(row).includes(name==='no-champion-fixture'?'champion-asset':'summoner-spells')),'mutant failed outside the intended network guard');}
  else{const result=JSON.parse(fs.readFileSync(path.join(folder,'result.json'),'utf8'));assert.equal(result.errors.length,0);assert.equal(result.violations.length,0);}
  console.log(name+': '+(mutant?'FAIL (expected)':'PASS'));
 }
 fs.writeFileSync(path.join(out,'result.json'),JSON.stringify({blockedUpstreamControl:true,championMutant:'FAIL',spellsMutant:'FAIL',fixed:'PASS',requests},null,2));
}
main().catch(e=>{console.error(e);process.exitCode=1;}).finally(()=>{for(const socket of sockets)socket.destroy();return cleanupChromeTest({servers:[proxy]});});
