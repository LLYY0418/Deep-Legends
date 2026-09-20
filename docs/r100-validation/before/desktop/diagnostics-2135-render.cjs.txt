// Explicit Electron probe: actual PNG tone mapping + theme-aware pro tabs.
const {app,BrowserWindow}=require('electron');
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const root=path.resolve(__dirname,'..');
app.setPath('userData','/private/tmp/deep-legends-2135-render-profile');
app.whenReady().then(async()=>{
 const win=new BrowserWindow({show:false,width:1100,height:500,webPreferences:{sandbox:true,contextIsolation:true}});
 const css=['app.css','gameplay.css','champions.css'].map(f=>fs.readFileSync(path.join(root,'web',f),'utf8')).join('\n');
 const img=f=>'data:image/png;base64,'+fs.readFileSync(path.join(root,'testdata',f)).toString('base64');
 await win.loadURL('data:text/html;charset=utf-8,'+encodeURIComponent(`<html><head><style>${css}body{display:block;padding:24px;overflow:auto}.icons{display:flex;gap:30px;margin:24px}.augment-icon{width:96px!important;height:96px!important}.icons p{margin:6px 0;color:var(--muted)}</style></head><body><div id="tabs"></div><div class="icons"><div class="is-gold"><span class="augment-icon"><img id="bonk" src="${img('diagnostics-2024/bonk-large.png')}"></span><p>邦！</p></div><div class="is-gold"><span class="augment-icon"><img id="reference" src="${img('diagnostics-2135/criticalmissile-large.png')}"></span><p>暴击飞弹（原图）</p></div><div class="is-prismatic"><span class="augment-icon"><img id="bear" src="${img('diagnostics-2024/drop-bear.png')}"></span><p>空投熊（原图）</p></div></div></body></html>`));
 await win.webContents.executeJavaScript(fs.readFileSync(path.join(root,'web/augment-artwork.js'),'utf8'));
 const source=fs.readFileSync(path.join(root,'web/gameplay.js'),'utf8');
 const funcs=source.slice(source.indexOf('  function renderSpecialistPlayers('),source.indexOf('  function runeConfigurationTitle('));
 const result=await win.webContents.executeJavaScript(`(async()=>{
  ${funcs}
  const state={live:{},specialistPlayerTabs:new Map(),selectedRecommendation:''};
  const proRequestTarget=()=>({key:'test'}),escapeHTML=s=>String(s??'').replaceAll('&','&amp;').replaceAll('<','&lt;'),iconFigure=()=>'',runeConfigurationTitle=()=>'',renderUnifiedRuneBoard=()=>'';
  document.getElementById('tabs').innerHTML=renderSpecialistPlayers([{title:'DK Siwoo',recordGames:1,recordWins:1},{title:'IG TheShy',recordGames:1,recordWins:1},{title:'T1 Doran',recordGames:2,recordWins:1},{title:'HLE Zeus',recordGames:1,recordWins:0}],'pro').split('<div class="specialist-player-games"')[0];
  const themes=[];
  for(const theme of ['dark','light']){
   document.documentElement.dataset.theme=theme;
   const button=document.querySelector('.pro-player-tab'),name=button.querySelector('.pro-player-name'),record=button.querySelector('.pro-player-record');
   const nr=name.getBoundingClientRect(),rr=record.getBoundingClientRect();
   const rootStyle=getComputedStyle(document.documentElement);
   const swatch=(variable)=>{const node=document.createElement('span');node.style.color='var('+variable+')';document.body.append(node);const color=getComputedStyle(node).color;node.remove();return color};
   themes.push({theme,gap:rr.left-nr.right,centerDifference:Math.abs(nr.top+nr.height/2-rr.top-rr.height/2),nameColor:getComputedStyle(name).color,expectedName:swatch('--primary-strong'),win:getComputedStyle(record.querySelector('.is-win')).color,expectedWin:swatch('--success'),loss:getComputedStyle(record.querySelector('.is-loss')).color,expectedLoss:swatch('--danger'),font:getComputedStyle(button).fontFamily,size:getComputedStyle(name).fontSize});
  }
  document.documentElement.dataset.theme='dark';
  const icons=[];
  for(const id of ['bonk','reference','bear']){
   const image=document.getElementById(id);await image.decode();window.deepLegendsAugmentArtwork.prepare(image);
   const canvas=image.parentElement.querySelector('canvas');let bright=0,rim=0;
   if(canvas){const p=canvas.getContext('2d').getImageData(0,0,canvas.width,canvas.height).data;for(let i=0;i<p.length;i+=4){if(p[i+3]<160)continue;if(p[i]>225&&p[i+1]>205&&p[i+2]>170)bright++;if(p[i]<100&&p[i+1]<100&&p[i+2]<100)rim++;}}
   icons.push({id,tinted:!!canvas,bright,rim});
  }
  return {themes,icons};
 })()`);
 for(const t of result.themes){assert.ok(t.gap>=11.9);assert.ok(t.centerDifference<.1);assert.equal(t.nameColor,t.expectedName);assert.equal(t.win,t.expectedWin);assert.equal(t.loss,t.expectedLoss);assert.equal(t.size,'14px');assert.match(t.font,/Segoe UI/);}
 assert.equal(result.icons[0].tinted,true);assert.ok(result.icons[0].bright>500);assert.ok(result.icons[0].rim>1000);
 assert.equal(result.icons[1].tinted,false);assert.equal(result.icons[2].tinted,false);
 fs.writeFileSync('/private/tmp/deep-legends-2135-render.png',(await win.webContents.capturePage()).toPNG());
 console.log(JSON.stringify(result));app.exit(0);
}).catch(e=>{console.error(e);app.exit(1)});
