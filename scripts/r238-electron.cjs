"use strict";
// Real Electron and production main/preload with an isolated synthetic backend.
const fs=require("node:fs"),path=require("node:path"),os=require("node:os"),http=require("node:http"),assert=require("node:assert/strict"),{spawn}=require("node:child_process");
const root=path.resolve(__dirname,".."),out=path.join(root,"docs/history/reports/r238/electron");
if(process.argv[2]==="--backend") {
  console.log("LOOT_READY "+JSON.stringify({baseUrl:process.env.R238_ORIGIN,bootstrapUrl:process.env.R238_ORIGIN,token:"r238-synthetic-local-token".repeat(3)}));
  setInterval(()=>{},1000);
} else {
async function run() {
  fs.mkdirSync(out,{recursive:true});
  const temp=fs.mkdtempSync(path.join(os.tmpdir(),"r238-electron-")),userData=path.join(temp,"userData"),requests=[];
  fs.mkdirSync(userData);
  const scaleFile=path.join(userData,"ui-scale.json"),boundsFile=path.join(userData,"window-bounds.json");
  const savedBounds={x:40,y:40,width:1050,height:750,maximized:false};
  fs.writeFileSync(scaleFile,JSON.stringify({mode:"fixed",value:1.25}));
  fs.writeFileSync(boundsFile,JSON.stringify(savedBounds));
  const web=path.join(root,"backend/web");
  const server=http.createServer((req,res)=>{
    requests.push(req.url);
    const name=new URL(req.url,"http://fixture").pathname;
    if(name==="/api/events") {res.writeHead(200,{"Content-Type":"text/event-stream"});res.write(": fixture\n\n");return;}
    if(name.startsWith("/api/")) {res.setHeader("Content-Type","application/json");res.end(JSON.stringify(name==="/api/status"?{connected:false,installations:[]} : {}));return;}
    const file=path.resolve(web,name==="/"?"index.html":"."+name);
    if(!file.startsWith(web+path.sep)||!fs.existsSync(file)||!fs.statSync(file).isFile()){res.writeHead(404).end();return;}
    res.setHeader("Content-Type",({".html":"text/html",".js":"text/javascript",".css":"text/css",".png":"image/png"})[path.extname(file)]||"application/octet-stream");res.end(fs.readFileSync(file));
  });
  await new Promise(r=>server.listen(0,"127.0.0.1",r));
  const origin=`http://127.0.0.1:${server.address().port}`,launcher=path.join(temp,"synthetic-backend"),results=[];
  fs.writeFileSync(launcher,"synthetic marker");
  try {
    for(const phase of ["migrate","restart","measure"]) {
      const record=path.join(out,phase+".json");
      const source=`const {app,BrowserWindow,session,screen}=require('electron'),fs=require('node:fs'),assert=require('node:assert/strict');
const cp=require('node:child_process'),nativeSpawn=cp.spawn;
cp.spawn=(command,args,options)=>command===${JSON.stringify(launcher)}?nativeSpawn(${JSON.stringify(process.execPath)},[${JSON.stringify(__filename)},'--backend'],options):nativeSpawn(command,args,options);
app.setName('R238 scale migration');app.setPath('userData',${JSON.stringify(userData)});
const origin=${JSON.stringify(origin)},result={phase:${JSON.stringify(phase)},network:[]};let main;
const sleep=ms=>new Promise(r=>setTimeout(r,ms));
const until=async(fn)=>{const start=Date.now();while(!await fn()){if(Date.now()-start>5000)throw Error('R238 Electron condition timeout');await sleep(10)}};
const state=()=>main.webContents.executeJavaScript('({select:document.querySelector("#setting-ui-scale").value,zoom:document.documentElement.style.getPropertyValue("--ui-zoom")})');
const set=async value=>{await main.webContents.executeJavaScript('(()=>{const el=document.querySelector("#setting-ui-scale");el.value='+JSON.stringify(value)+';el.dispatchEvent(new Event("change",{bubbles:true}));})()');};
app.on('browser-window-created',(_e,w)=>w.webContents.once('did-finish-load',()=>{if(w.webContents.getURL().startsWith(origin))main=w}));
app.whenReady().then(()=>session.defaultSession.webRequest.onBeforeRequest({urls:['*://*/*']},(d,cb)=>{result.network.push(d.url);cb({cancel:!d.url.startsWith(origin+'/')})}));
require(${JSON.stringify(path.join(root,"desktop/main.cjs"))});
(async()=>{
 await until(()=>main?.isVisible());await until(async()=>!!(await state()).zoom);
 result.electron=process.versions.electron;result.platform=process.platform;result.initial={bounds:main.getBounds(),content:main.getContentBounds(),state:await state(),stored:JSON.parse(fs.readFileSync(${JSON.stringify(scaleFile)},'utf8'))};
 if(result.phase==='migrate'){
  assert.deepEqual(result.initial.stored,{mode:'auto',defaultAuto:1});await until(async()=>(await state()).select==='auto');
  assert.equal(result.initial.bounds.width,1050);assert.equal(result.initial.bounds.height,750);
  await set('1.25');await until(()=>JSON.parse(fs.readFileSync(${JSON.stringify(scaleFile)},'utf8')).value===1.25);await until(async()=>(await state()).zoom==='1.25');
 }else if(result.phase==='restart'){
  assert.deepEqual(result.initial.stored,{mode:'fixed',value:1.25,defaultAuto:1});await until(async()=>(await state()).select==='1.25');assert.equal((await state()).zoom,'1.25');
 }else {
  await set('auto');await until(async()=>(await state()).select==='auto');
  result.physicalDisplay=screen.getPrimaryDisplay();
  result.simulatedDisplay={width:1920,height:1080};
  result.simulation='Keep physical macOS display unchanged; set real native window to production windowBoundsForWorkArea({width:1920,height:1080}). This is window geometry simulation, not a physical 1920x1080 display.';
  const bounds=require(${JSON.stringify(path.join(root,"desktop/window-bounds.cjs"))}).windowBoundsForWorkArea(result.simulatedDisplay);
  main.setBounds({x:0,y:0,...bounds});await sleep(400);
  result.measurement={requestedBounds:bounds,bounds:main.getBounds(),content:main.getContentBounds(),state:await state()};
  assert.equal(Number(result.measurement.state.zoom),require(${JSON.stringify(path.join(root,"desktop/ui-scale.cjs"))}).autoScaleFor(result.measurement.content));
 }
 result.final={state:await state(),stored:JSON.parse(fs.readFileSync(${JSON.stringify(scaleFile)},'utf8'))};
 fs.writeFileSync(${JSON.stringify(path.join(out,phase+".png"))},(await main.webContents.capturePage()).toPNG());
 fs.writeFileSync(${JSON.stringify(record)},JSON.stringify(result,null,2));app.quit();
})().catch(e=>{console.error(e);app.exit(1)});`;
      fs.writeFileSync(path.join(temp,"probe.cjs"),source);
      fs.writeFileSync(path.join(temp,"package.json"),JSON.stringify({name:"r238-probe",version:"1.0.0",main:"probe.cjs"}));
      await new Promise((resolve,reject)=>{
        const env={...process.env,LOOT_BACKEND:launcher,R238_ORIGIN:origin};delete env.ELECTRON_RUN_AS_NODE;
        const child=spawn(require(path.join(root,"desktop/node_modules/electron")),["--disable-gpu",temp],{env,stdio:["ignore","pipe","pipe"]});let log="";
        child.stdout.on("data",c=>log+=c);child.stderr.on("data",c=>log+=c);
        const timeout=setTimeout(()=>{fs.writeFileSync(path.join(out,phase+".log"),log);child.kill("SIGKILL");reject(Error("R238 Electron timeout"));},30000);
        child.once("error",reject);child.once("exit",code=>{clearTimeout(timeout);fs.writeFileSync(path.join(out,phase+".log"),log);code===0?resolve():reject(Error(log));});
      });
      const result=JSON.parse(fs.readFileSync(record,"utf8"));
      assert.equal(result.network.filter(url=>/license(?:-staging)?\.yinxiaobia\.net/.test(url)).length,0);
      results.push(result);
    }
    assert.equal(requests.filter(url=>url.startsWith("/api/license/")).length,0);
    fs.writeFileSync(path.join(out,"results.json"),JSON.stringify({results,authorizationRequests:0,windows:"未在 Windows 实跑"},null,2));
    console.log(JSON.stringify(results.map(r=>({phase:r.phase,electron:r.electron,initial:r.initial,final:r.final,measurement:r.measurement}))));
  }finally{server.closeAllConnections();await new Promise(r=>server.close(r));fs.rmSync(temp,{recursive:true,force:true,maxRetries:5,retryDelay:100});}
}
run().catch(e=>{console.error(e);process.exitCode=1});
}
