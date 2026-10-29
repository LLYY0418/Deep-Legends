'use strict';
const fs=require('node:fs'),path=require('node:path'),cp=require('node:child_process'),assert=require('node:assert/strict');
const {extract}=require('../backend/web/r188-harness.cjs'),{decodePNG}=require('./png-pixels.cjs'),{withBrowser}=require('./r280-browser-fixture.cjs');
const root=path.resolve(__dirname,'..'),base=f=>cp.execFileSync('git',['show','v0.12.85:'+f],{cwd:root,encoding:'utf8'});
async function run(){
 const current=fs.readFileSync(path.join(root,'backend/web/gameplay.js'),'utf8'),old=base('backend/web/gameplay.js'),css=fs.readFileSync(path.join(root,'backend/web/gameplay.css'),'utf8'),oldCSS=base('backend/web/gameplay.css');
 assert.equal(extract(current,'renderMultiKillTag'),extract(old,'renderMultiKillTag'));
 // R315 P3 adds the shared close lifecycle; its own tests cover those calls.
 // Remove only those exact additions before preserving the R282 interaction contract.
 let popover=extract(current,'bindMatchTagsPopover');
 for(const addition of ['touchUntil=0,stopLifecycle=null;','stopLifecycle?.();stopLifecycle=null;','        stopLifecycle=win.deepLegendsFloatingLayers?.watch(button,panel,close);\n'])assert(popover.includes(addition),addition);
 popover=popover.replace('touchUntil=0,stopLifecycle=null;','touchUntil=0;').replace('stopLifecycle?.();stopLifecycle=null;','').replace('        stopLifecycle=win.deepLegendsFloatingLayers?.watch(button,panel,close);\n','');
 // R283 P7 changes only portal zoom coordinates.
 const unscaledPopover=popover.replace(/^        const scale=Number\(win\.getComputedStyle\(panel\)\.zoom\)\|\|1;\n/m,'').replace('panel.offsetWidth*scale,height=panel.offsetHeight*scale','panel.offsetWidth,height=panel.offsetHeight').replaceAll('/scale}','}');
 assert.equal(unscaledPopover,extract(old,'bindMatchTagsPopover'));
 const blocks=s=>s.slice(s.indexOf('.multikill-tag {'),s.indexOf('.match-tags-popover',s.indexOf('.multikill-tag {')));
 assert.equal(blocks(css),blocks(oldCSS));assert(!fs.readFileSync(path.join(root,'backend/web/icons.svg'),'utf8').includes('dl-icon-multikill-'));
 await withBrowser(async({page,out,results,errors})=>{
  for(const theme of ['dark','light'])for(const zoom of [1,1.5]){
   const p=await page(theme);await p.until("document.querySelector('.match-entry')&&!__r278Game.activeTab().loading");
   await p.call('Emulation.setDeviceMetricsOverride',{width:1500,height:1100,deviceScaleFactor:1,mobile:false});
   let reference;
   for(const [label,source,style] of [['v0.12.85',old,oldCSS],['current',current,css]]){
    const render=Function('let multikillGradientSeq=0;'+extract(source,'renderMultiKillTag')+';return renderMultiKillTag')();
    const html=[2,3,4,5].map(level=>render(level,282)).join('');
    await p.evaluate(`(()=>{for(const sheet of [...document.styleSheets])if(sheet.href?.endsWith('/gameplay.css'))sheet.disabled=true;document.getElementById('multikill-css')?.remove();const s=document.createElement('style');s.id='multikill-css';s.textContent=${JSON.stringify(style)};document.head.append(s);document.body.innerHTML='<main id="badges">'+${JSON.stringify(html)}+'</main>';const n=document.getElementById('badges');n.style.cssText='position:fixed;left:40px;top:40px;width:300px;height:80px;display:flex;align-items:center;gap:16px;background:${theme==='dark'?'#17202c':'#edf2f7'};zoom:${zoom}';document.documentElement.style.zoom='1';document.body.style.zoom='1';for(const a of document.getAnimations()){a.pause();a.currentTime=400;}return new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));})()`);
    const clip=await p.evaluate("(()=>{const r=document.getElementById('badges').getBoundingClientRect();return{x:r.x,y:r.y,width:r.width,height:r.height,scale:1}})()");
    const shot=await p.call('Page.captureScreenshot',{format:'png',clip,captureBeyondViewport:false}),bytes=Buffer.from(shot.data,'base64');fs.writeFileSync(path.join(out,`${theme}-${zoom}-${label}.png`),bytes);const image=decodePNG(bytes);
    if(!reference){reference=image;continue;}assert.equal(image.width,reference.width);assert.equal(image.height,reference.height);let changed=0;for(let y=0;y<image.height;y++)for(let x=0;x<image.width;x++)if(image.pixel(x,y).some((v,i)=>v!==reference.pixel(x,y)[i]))changed++;
    results.push({kind:'P1-multikill-pixels',theme,zoom,kills:[2,3,4,5],animationTimeMs:400,pixels:image.width*image.height,differentPixels:changed});assert.equal(changed,0);
   }
  }
  assert.deepEqual(errors,[]);
 });
}
if(require.main===module)run().catch(e=>{console.error(e);process.exitCode=1});module.exports={run};
