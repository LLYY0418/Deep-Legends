'use strict';
const {app,BrowserWindow}=require('electron');
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const root=path.resolve(__dirname,'..');
app.setPath('userData','/private/tmp/deep-legends-2244-render-profile');
app.whenReady().then(async()=>{
 try{
  const win=new BrowserWindow({show:false,width:1280,height:1100,webPreferences:{sandbox:true,contextIsolation:true}});
  const css=['app.css','gameplay.css','champions.css','suite.css'].map(f=>fs.readFileSync(path.join(root,'web',f),'utf8')).join('\n');
  await win.loadURL('data:text/html;charset=utf-8,'+encodeURIComponent(`<html><head><style>${css}body{display:block;padding:24px;overflow:auto}.summoner-strip{height:150px;margin:16px 0}.probe-copy{color:var(--ink);font-weight:700;font-size:24px}h3{margin:20px 0;color:var(--primary-strong)}.skill-priority{max-width:950px}</style></head><body><h3>实际居中原画 · 阿狸不朽 / 杰斯</h3><div id="banners"></div><h3>技能加点</h3><div id="skills"></div></body></html>`));
  await win.webContents.executeJavaScript(fs.readFileSync(path.join(root,'web/overview-art.js'),'utf8'));
  const media=[{id:103086,name:'我听见风的颜色',img:fs.readFileSync(path.join(root,'testdata/diagnostics-2244/ahri-centered.jpg')).toString('base64')},{id:126000,name:'kiin · GEN Kiin',img:fs.readFileSync(path.join(root,'testdata/diagnostics-2244/jayce-centered.jpg')).toString('base64')},{id:68000,name:'BLG Bin · 兰博',img:fs.readFileSync(path.join(root,'testdata/diagnostics-2244/rumble-centered.jpg')).toString('base64')}];
  await win.webContents.executeJavaScript(`{
   const media=${JSON.stringify(media)};
   banners.innerHTML=media.map(m=>'<section class="summoner-strip">'+deepLegendsOverviewArt.render({backgroundSkinId:m.id,backgroundPosterPath:'/lol-game-data/assets/splash_centered_0.jpg'})+'<div class="probe-copy">'+m.name+'</div><div></div><div>单排/双排</div></section>').join('');
   document.querySelectorAll('.overview-art').forEach((holder,i)=>holder.querySelectorAll('img').forEach(img=>img.src='data:image/jpeg;base64,'+media[i].img));
   deepLegendsOverviewArt.prepare(document);
  }`);
  const source=fs.readFileSync(path.join(root,'web/gameplay.js'),'utf8');
  const fn=source.slice(source.indexOf('  function renderSkillPlan('),source.indexOf('  function abilityTooltip('));
  await win.webContents.executeJavaScript(`{const escapeHTML=x=>String(x),assetIcon=()=>'',abilityTooltip=()=>'';${fn};skills.innerHTML=renderSkillPlan({skillPriority:['Q','E','W'],skillOrder:['Q','E','W','Q','Q','R','Q','E']},[]);}`);
  await new Promise(r=>setTimeout(r,400));
  for(const width of [920,1280,1800]){
   win.setSize(width,1100);await new Promise(r=>setTimeout(r,150));
   const rows=await win.webContents.executeJavaScript(`Array.from(document.querySelectorAll('[data-overview-poster]')).map((img,i)=>{const r=img.getBoundingClientRect(),c=img.closest('.overview-art').getBoundingClientRect();return {faceY:r.top+[.31,.30,.36][i]*r.height-c.top,height:c.height,covers:r.left<=c.left+.5&&r.right>=c.right-.5&&r.top<=c.top+.5&&r.bottom>=c.bottom-.5,fit:getComputedStyle(img).objectFit}})`);
   for(const row of rows){assert(row.covers && Math.abs(row.faceY-row.height/2)<1,JSON.stringify({width,row}));}
   console.log(JSON.stringify({width,faces:rows}));
  }
  const suite=fs.readFileSync(path.join(root,'web/suite.js'),'utf8');
  const claims=suite.slice(suite.indexOf('  function claimSelectionKeys('),suite.indexOf('  async function executeClaims('));
  await win.webContents.executeJavaScript(`{const state={selectedClaims:new Set(),claimFailures:new Map(),claimChoices:new Map()},escapeHTML=x=>String(x),checked=x=>x?' checked':'',imageURL=x=>x,sourceNames={grant:'奖励账本'};${claims};const sample=document.createElement('div');sample.innerHTML='<h3>活动奖励 · 同一张卡片</h3>'+claimEventGroups(Array.from({length:5},(_,i)=>({eventId:'test',eventName:'第3赛季：第1幕',key:'grant:'+i,source:'grant',items:[{id:'reward'+i,title:i===2?'25橙色精萃':'750蓝色精萃',quantity:i===2?25:750}]})));document.body.append(sample);}`);
  win.setSize(1280,1100);await new Promise(r=>setTimeout(r,150));
  fs.writeFileSync('/private/tmp/deep-legends-0006-dark.png',(await win.webContents.capturePage()).toPNG());
  await win.webContents.executeJavaScript(`document.documentElement.dataset.theme='light'`);
  await new Promise(r=>setTimeout(r,100));
  fs.writeFileSync('/private/tmp/deep-legends-0006-light.png',(await win.webContents.capturePage()).toPNG());
  win.destroy();app.quit();
 }catch(err){console.error(err);app.exit(1);}
});
