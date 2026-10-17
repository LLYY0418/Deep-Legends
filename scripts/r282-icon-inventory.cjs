'use strict';
const fs=require('node:fs'),path=require('node:path'),cp=require('node:child_process');
const root=path.resolve(__dirname,'..'),{extract}=require('../backend/web/r188-harness.cjs');
const base=f=>cp.execFileSync('git',['show','v0.12.85:'+f],{cwd:root,encoding:'utf8'});
function inventory(){
 const diff=cp.execFileSync('git',['diff','--unified=0','v0.12.85','76b65d06','--','backend/web'],{cwd:root,encoding:'utf8',maxBuffer:8*1024*1024});
 let file='',line=0;const rows=[];
 for(const row of diff.split('\n')){
  if(row.startsWith('--- a/'))file=row.slice(6);
  const h=row.match(/^@@ -(\d+)/);if(h){line=Number(h[1]);continue;}
  if(!row.startsWith('-') || row.startsWith('---'))continue;
  const text=row.slice(1),at=line++;
  if(!/\.(js|html)$/.test(file) || /test|harness/.test(file))continue;
  for(const match of text.matchAll(/<svg\b[\s\S]*?<\/svg>/g)){
   let svg=match[0],before=text.slice(0,match.index),id=[...before.matchAll(/\bid="([\w-]+)"/g)].at(-1)?.[1],classes=[...before.matchAll(/class="([^"$]+)"/g)].at(-1)?.[1] || '';
   const selector=id?'#'+id+' svg':classes.trim()?'.'+classes.trim().split(/\s+/).join('.')+' svg':'svg';
   rows.push({file,line:at,selector,classes,id,svg,dynamic:svg.includes('${')});
  }
  const outside=text.replace(/<svg\b[\s\S]*?<\/svg>/g,'');
  for(const match of outside.matchAll(/[›▸▾×↻◆◉★⬡🔒✓]/gu))rows.push({file,line:at,selector:'icon character',classes:'',svg:'<span class="inventory-glyph">'+match[0]+'</span>',dynamic:false});
 }
 // Parametric helpers are exercised with every design, not an unresolved template.
 const read=f=>process.env.R280_BASELINE==='1'?base(f):fs.readFileSync(path.join(root,f),'utf8');
 const game=read('backend/web/gameplay.js'),app=read('backend/web/app.js');
 for(const row of rows)if(!row.dynamic && row.svg.startsWith('<svg') && !read(row.file).includes(row.svg))throw Error('unrestored icon '+row.file+':'+row.line+' '+row.selector);
 const {JSDOM}=require('../desktop/node_modules/jsdom'),dom=new JSDOM('<main></main>'),list=dom.window.document.querySelector('main');
 const renderEmpty=Function('el',extract(read('backend/web/friends.js'),'renderEmpty')+';return renderEmpty')({list});
 for(const state of ['disconnected','search','empty']){renderEmpty(state,'',false);rows.push({file:'backend/web/friends.js',line:232,selector:'.friends-empty '+state,classes:'friends-empty',svg:list.querySelector('svg').outerHTML,dynamic:true});}
 for(const row of rows)if(row.file.endsWith('friends.js') && row.dynamic && row.line===232 && row.svg.includes('${icons')){renderEmpty('empty','',false);row.svg=list.querySelector('svg').outerHTML;}
 dom.window.close();
 const multi=Function('let multikillGradientSeq=0;'+extract(game,'renderMultiKillTag')+';return renderMultiKillTag')();
 rows.forEach(row=>{
  if(row.dynamic && row.svg.includes('linearGradient'))row.svg=multi(5,1);
  else if(row.dynamic && row.svg.includes('paths[category]'))row.svg=Function(extract(app,'lootCategoryIcon')+';return lootCategoryIcon')()('材料');
  else if(row.dynamic)row.svg=row.svg.replace(/\$\{path\}/g,'m15 5-7 7 7 7').replace(/\$\{[^}]*\}/g,'');
  if(process.env.R280_BASELINE!=='1' && row.file.endsWith('index.html') && row.svg.includes('↻'))row.svg=read(row.file).match(/<svg class="control-icon refresh-icon"[\s\S]*?<\/svg>/)[0];
 });
 for(const level of [2,3,4,5])rows.push({file:'backend/web/gameplay.js',line:game.slice(0,game.indexOf('function renderMultiKillTag')).split('\n').length,selector:'.multikill-tag.is-mk-'+level,classes:'',svg:multi(level,100+level),dynamic:true});
 const loot=Function(extract(app,'lootCategoryIcon')+';return lootCategoryIcon')();
 for(const category of ['材料','宝箱','英雄','皮肤','小小英雄','永恒星碑','表情','守卫','图标'])rows.push({file:'backend/web/app.js',line:app.slice(0,app.indexOf('function lootCategoryIcon')).split('\n').length,selector:'.loot-category-icon '+category,classes:'loot-category-icon',svg:loot(category),dynamic:true});
 const suite=fs.readFileSync(path.join(root,'backend/web/suite.js'),'utf8');
 for(const match of suite.matchAll(/<img class="([^"\n]*cs-brave-avatar)" src="\/bravery.svg" alt="勇敢举动">/g))rows.push({file:'backend/web/suite.js',line:suite.slice(0,match.index).split('\n').length,selector:'.'+match[1].split(' ').join('.'),classes:'',svg:process.env.R280_BASELINE==='1'?'<span>⚔</span>':match[0],dynamic:true,exception:'P16 explicitly replaces bravery artwork'});
 return rows.map((r,index)=>({...r,case:'icon-'+String(index+1).padStart(3,'0')}));
}
function gallery(rows){return '<main class="inventory">'+rows.map(r=>`<article id="${r.case}" class="inventory-case"><small>${r.case} · ${r.file.split('/').pop()}:${r.line}</small><div class="inventory-preview"><span class="${r.classes.replace(/[^\w -]/g,'')}">${r.svg}</span></div></article>`).join('')+'</main>';}
async function run(){
 const {withBrowser}=require('./r280-browser-fixture.cjs'),rows=inventory();
 await withBrowser(async({page,out,results})=>{
  fs.writeFileSync(path.join(out,'inventory.json'),JSON.stringify(rows,null,2));
  for(const theme of ['dark','light'])for(const zoom of [1,1.5]){
   const p=await page(theme);await p.until("document.querySelector('.match-entry') && !__r278Game.activeTab().loading");
   await p.call('Emulation.setDeviceMetricsOverride',{width:1500,height:1100,deviceScaleFactor:1,mobile:false});
   await p.evaluate(`(()=>{localStorage.setItem('lol-loot-ui-scale','${zoom}');document.documentElement.dataset.theme='${theme}';document.documentElement.dataset.uiScale='${zoom}';document.documentElement.style.setProperty('--ui-zoom','${zoom}');document.body.innerHTML=${JSON.stringify(gallery(rows))};const style=document.createElement('style');style.textContent='.inventory{display:grid;grid-template-columns:repeat(6,1fr);gap:8px;padding:12px;background:var(--bg);color:var(--ink)}.inventory-case{height:126px;overflow:hidden;border:1px solid var(--line);background:var(--surface);padding:6px}.inventory-case small{display:block;height:28px;font:10px sans-serif;color:var(--muted)}.inventory-preview{position:relative;display:flex;align-items:center;justify-content:center;height:78px;overflow:hidden}.inventory-preview>span{position:relative!important;display:inline-flex;align-items:center;justify-content:center;min-width:50px;min-height:50px;padding:0!important;margin:0!important}.inventory-preview img{width:24px;height:24px}.inventory-preview svg{width:24px;height:24px;max-width:78px;max-height:60px;flex:none}.inventory-preview .multikill-tag svg{width:100%;height:100%}.inventory-preview .multikill-tag{min-width:0;min-height:0}.inventory-glyph{font-size:22px}.inventory-preview .loot-category-icon svg{fill:currentColor}';document.head.append(style);})()`);
   for(let start=0;start<rows.length;start+=36){await p.evaluate(`document.getElementById('${rows[start].case}').scrollIntoView({block:'start'})`);await p.shot(`icons-${zoom}-sheet-${Math.floor(start/36)+1}`);}
   for(const row of rows){
    const rect=await p.evaluate(`(()=>{const n=document.getElementById('${row.case}');n.scrollIntoView({block:'center'});const r=n.getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,height:r.height,scale:1}})()`);
    const image=await p.call('Page.captureScreenshot',{format:'png',clip:rect,captureBeyondViewport:false});
    fs.writeFileSync(path.join(out,`${theme}-${zoom}-${row.case}.png`),Buffer.from(image.data,'base64'));
   }
   results.push({kind:'icon-inventory',theme,zoom,cases:rows.length});
  }
 });
}
if(require.main===module)run().catch(e=>{console.error(e);process.exitCode=1});module.exports={inventory,gallery,run};
