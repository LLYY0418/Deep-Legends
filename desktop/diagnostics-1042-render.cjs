'use strict';
const {app,BrowserWindow}=require('electron');
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const root=path.resolve(__dirname,'..');
app.setPath('userData','/private/tmp/deep-legends-1042-render-profile');
app.whenReady().then(async()=>{try {
 const win=new BrowserWindow({show:false,width:1280,height:1100,webPreferences:{sandbox:true,contextIsolation:true}});
 const samples=[['ahri-centered',103086,.31,true],['ahri-ordinary',103086,.16,false],['xerath-centered',101000,.29,true],['xerath-ordinary',101000,.20,false]];
 const media=samples.map(([file,id,y,centered])=>({file,id,y,centered,img:fs.readFileSync(path.join(root,`testdata/diagnostics-2244/${file}.jpg`)).toString('base64')}));
 const css=['app.css','gameplay.css'].map(file=>fs.readFileSync(path.join(root, "backend", "web",file),'utf8')).join('\n');
 await win.loadURL('data:text/html;charset=utf-8,'+encodeURIComponent(`<html><head><style>${css}body{display:block;padding:24px;overflow:auto}.summoner-strip{height:140px;margin:20px 0}</style></head><body></body></html>`));
 await win.webContents.executeJavaScript(fs.readFileSync(path.join(root,'backend/web/overview-art.js'),'utf8'));
 await win.webContents.executeJavaScript(`{
  const samples=${JSON.stringify(media)};
  document.body.innerHTML=samples.map(m=>'<section class="summoner-strip">'+deepLegendsOverviewArt.render({backgroundSkinId:m.id,backgroundPosterPath:m.centered?'/splash_centered_0.jpg':'',backgroundSource:'gtimg',backgroundPath:'/ordinary.jpg'})+'<h3>'+m.file+'</h3></section>').join('');
  document.querySelectorAll('[data-overview-poster]').forEach((img,i)=>img.src='data:image/jpeg;base64,'+samples[i].img);
  deepLegendsOverviewArt.prepare(document);
 }`);
 for(const width of [920,1280,1800]) {
  win.setSize(width,1100);await new Promise(r=>setTimeout(r,300));
  const rows=await win.webContents.executeJavaScript(`Array.from(document.querySelectorAll('[data-overview-poster]')).map((img,i)=>{const r=img.getBoundingClientRect(),c=img.parentNode.getBoundingClientRect();return {loaded:img.naturalWidth>0,face:r.top+${JSON.stringify(samples.map(x=>x[2]))}[i]*r.height-c.top,height:c.height,covers:r.left<=c.left+.5&&r.right>=c.right-.5&&r.top<=c.top+.5&&r.bottom>=c.bottom-.5}})`);
  for(const row of rows)assert(row.loaded&&row.covers&&Math.abs(row.face-row.height/2)<1,JSON.stringify({width,row}));
  console.log(JSON.stringify({width,rows}));
 }
 win.setSize(1280,1100);await new Promise(r=>setTimeout(r,200));
 fs.writeFileSync('/private/tmp/deep-legends-1042-banners.png',(await win.webContents.capturePage()).toPNG());
 win.destroy();app.quit();
}catch(err){console.error(err);app.exit(1)}});
