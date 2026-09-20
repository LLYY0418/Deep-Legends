"use strict";
// Run with Electron (not node): real preload/IPC/reload and export isolation.
const electron=require("electron");
const {app,BrowserWindow,ipcMain}=electron;
const fs=require("node:fs"),os=require("node:os"),path=require("node:path"),http=require("node:http"),assert=require("node:assert/strict");
const {createRequire}=require("node:module");
const {once}=require("node:events");
const {renderOverviewPng}=require("./share-export.cjs");
const root=path.resolve(__dirname,".."),web=path.join(root,"backend","web");
const temp=fs.mkdtempSync(path.join(os.tmpdir(),"r79-electron-"));
app.setPath("userData",temp);
let server,main;
const delay=ms=>new Promise(r=>setTimeout(r,ms));
async function eventually(fn){for(let n=0;n<100;n++){if(await fn())return;await delay(50);}throw Error("Electron condition timed out");}
app.whenReady().then(async()=>{
  server=http.createServer((req,res)=>{
    const url=new URL(req.url,"http://localhost");
    const file=path.resolve(web,url.pathname==="/"?"index.html":"."+url.pathname);
    if(!file.startsWith(web+path.sep)){res.writeHead(403).end();return;}
    if(url.pathname==="/")res.setHeader("Set-Cookie","lol_loot_token=local-test-fixture; HttpOnly; SameSite=Strict; Path=/");
    if(fs.existsSync(file)&&fs.statSync(file).isFile()){
      const types={".html":"text/html",".js":"text/javascript",".css":"text/css",".svg":"image/svg+xml",".png":"image/png"};res.setHeader("Content-Type",types[path.extname(file)]||"application/octet-stream");res.end(fs.readFileSync(file));return;
    }
    res.writeHead(404).end();
  });
  await new Promise(r=>server.listen(0,"127.0.0.1",r));
  const baseURL="http://127.0.0.1:"+server.address().port;
  // Boot the production main module without launching the real LoL backend.
  const appProxy=new Proxy(app,{get(target,key){if(key==="whenReady")return()=>({then(){}});const value=target[key];return typeof value==="function"?value.bind(target):value;}});
  const productionRequire=createRequire(path.join(__dirname,"main.cjs"));
  const shell=new Function("require","__dirname","process",fs.readFileSync(path.join(__dirname,"main.cjs"),"utf8")+'\nreturn {boot(base){backendReady={baseUrl:base,bootstrapUrl:base+"/?demo"};createMainWindow();return mainWindow;},stop(){quitting=true;}};')(
    name=>name==="electron"?{...electron,app:appProxy}:productionRequire(name),__dirname,process);
  main=shell.boot(baseURL);
  await once(main.webContents,"did-finish-load");
  await eventually(async()=>await main.webContents.executeJavaScript('!!document.querySelector(".match-summary")'));
  const mainSender={sender:main.webContents};
  ipcMain.emit("desktop-scale-set",mainSender,"fixed",2);
  await eventually(()=>Math.abs(main.webContents.getZoomFactor()-2)<1e-8);
  assert.deepEqual(main.getMinimumSize(),[1560,1200]);
  assert.equal(await main.webContents.executeJavaScript('document.querySelector("#setting-ui-scale").value'),"2");
  main.webContents.reload();await once(main.webContents,"did-finish-load");
  await eventually(()=>Math.abs(main.webContents.getZoomFactor()-2)<1e-8);
  assert.deepEqual(JSON.parse(fs.readFileSync(path.join(temp,"ui-scale.json"))),{mode:"fixed",value:2});
  // The same document and the real export renderer must have identical pixel
  // dimensions at every main-window zoom, including changes during capture.
  const exports=[];
  const markup='<div class="overview-share-surface"><main style="height:600px">R79 export isolation</main></div>';
  class ExportWindow extends BrowserWindow {
    constructor(options){super(options);assert.equal(Object.hasOwn(options.webPreferences,"zoomFactor"),false);exports.push(this);}
  }
  const results=[];
  for(const zoom of [1,2,2.5]){
    ipcMain.emit("desktop-scale-set",mainSender,"fixed",zoom);
    const render=renderOverviewPng(ExportWindow,baseURL,{markup,theme:"dark",density:""});
    const exp=exports.at(-1);
    await once(exp.webContents,"did-finish-load");
    assert.equal(exp.webContents.getZoomFactor(),1);
    assert.notEqual(exp.webContents.session,main.webContents.session);
    const cookies=await exp.webContents.session.cookies.get({url:baseURL});assert.ok(cookies.some(cookie=>cookie.name==="lol_loot_token"&&cookie.httpOnly));
    ipcMain.emit("desktop-scale-set",mainSender,"fixed",zoom===2?1.75:2);
    assert.equal(exp.webContents.getZoomFactor(),1,"main zoom while capturing must not leak into export");
    const result=await render;
    results.push({mainZoom:zoom,width:result.width,height:result.height,scale:result.scale,pngWidth:result.png.readUInt32BE(16),pngHeight:result.png.readUInt32BE(20)});
  }
  for(const r of results)assert.deepEqual([r.width,r.height,r.scale,r.pngWidth,r.pngHeight],[results[0].width,results[0].height,results[0].scale,results[0].pngWidth,results[0].pngHeight]);
  console.log(JSON.stringify({electron:process.versions.electron,results}));
  console.log("R79 real Electron IPC/reload/minimum/export isolation PASS");
  shell.stop();
}).catch(error=>{console.error(error);process.exitCode=1;}).finally(()=>{
  for(const window of BrowserWindow.getAllWindows())window.destroy();server?.close();
  app.exit(process.exitCode||0);
});
app.on("quit",()=>fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100}));
