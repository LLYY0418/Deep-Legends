'use strict';
const {browser}=require('./r297-browser-harness.cjs'),fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const out=path.join(process.env.R297_REPORT_DIR||path.join(require('node:os').tmpdir(),'r297-browser-proof'),'p10-patch-notes');const feed=JSON.parse(fs.readFileSync(path.resolve(__dirname,'../backend/patchnotes/patch-notes.json')));
(async()=>{const b=await browser({out,inject:`window.__patchFeed=${JSON.stringify(feed)};window.__patchData=__patchFeed.versions[0];`,intercept:`if(pathname==='/api/patch-notes')return Promise.resolve(new Response(JSON.stringify(window.__patchData),{headers:{'Content-Type':'application/json'}}));`,transform:(name,data)=>name==='champions.js'?Buffer.from(data.toString().replace(/\}\)\(\);\s*$/,'window.__patchChampions={state,render};\n})();')):data});const results=[];try{
 await b.call('Page.navigate',{url:b.origin+'/?demo&section=champions'});await b.until("window.__patchChampions && document.querySelector('[data-patch-notes] b')?.textContent==='26.20'");
 for(const width of [1440,960])for(const theme of ['dark','light']){
  await b.call('Emulation.setDeviceMetricsOverride',{width,height:1000,deviceScaleFactor:1,mobile:false});await b.evaluate(`document.documentElement.dataset.theme='${theme}'`);const heights=[];
  for(const mode of ['ranked','aram-mayhem','arena']){
   await b.evaluate(`(()=>{const c=__patchChampions;c.state.mode='${mode}';c.state.selected=null;c.state.loading=false;c.render()})()`);
   const header=await b.evaluate(`(()=>{const h=document.querySelector('.champion-page-head'),b=h.querySelector('[data-champion-refresh]'),c=getComputedStyle(b),n=document.createElement('span');n.style.color='var(--muted)';document.body.append(n);const muted=getComputedStyle(n).color;n.remove();return {height:h.getBoundingClientRect().height,subtitle:!!h.querySelector('p'),disabled:b.disabled,color:c.color,muted,opacity:c.opacity,overflow:h.scrollWidth>h.clientWidth+1,tabs:[...document.querySelectorAll('.champion-mode-tab small')].map(n=>n.textContent)}})()`);
   assert(!header.subtitle);assert(!header.disabled);assert.notEqual(header.color,header.muted);assert(!header.overflow);assert.equal(header.tabs.length,3);heights.push(header.height);
   await b.evaluate("document.querySelector('[data-patch-notes]').click()");await b.until("document.querySelector('.pn-dialog')");
   if(mode==='arena'){assert(await b.evaluate("document.querySelector('.pn-body').textContent.includes('本版本没有该模式的改动')"));await b.shot(`arena-current-empty-${theme}-${width}`);await b.evaluate("window.__patchData=__patchFeed.versions.find(v=>v.version==='26.17');window.dispatchEvent(new CustomEvent('deep-legends:patch-notes-changed'))");await b.until("document.querySelector('.pn-head h3').textContent.includes('26.17')")}
   const dialog=await b.evaluate("(()=>{const d=document.querySelector('.pn-dialog'),r=d.getBoundingClientRect(),body=d.querySelector('.pn-body');return {width:r.width,right:r.right,center:Math.abs(r.x+r.width/2-innerWidth/2),focus:d.contains(document.activeElement),mode:d.querySelector('.pn-mode').textContent,entries:d.querySelectorAll('.pn-entry').length,scroll:body.scrollHeight>body.clientHeight,body:body.textContent.slice(0,180),source:d.querySelector('a').href}})()");
   assert.equal(dialog.width,680);assert(dialog.right<=width);assert(dialog.center<1);assert(dialog.entries>0);assert(dialog.scroll);assert(dialog.focus);results.push({theme,width,mode,header,dialog});
   await b.shot(`${mode==='ranked'?'g7':mode==='aram-mayhem'?'g8':'g9'}-${mode}-${theme}-${width}`);
   await b.evaluate("document.querySelector('.pn-dialog').dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true}))");assert(!await b.evaluate("!!document.querySelector('.pn-dialog')"));assert(await b.evaluate("document.activeElement.matches('[data-patch-notes]')"));
   if(mode==='arena'){await b.evaluate("window.__patchData=__patchFeed.versions[0];window.dispatchEvent(new CustomEvent('deep-legends:patch-notes-changed'))");await b.until("document.querySelector('[data-patch-notes] b').textContent==='26.20'")}
  }
  if(width===1440)assert(Math.max(...heights)-Math.min(...heights)<1,JSON.stringify(heights));
 }
 await b.evaluate("__patchChampions.state.loading=true;__patchChampions.render()");const busy=await b.evaluate("(()=>{const b=document.querySelector('[data-champion-refresh]');return {disabled:b.disabled,opacity:getComputedStyle(b).opacity,cursor:getComputedStyle(b).cursor}})()");assert.equal(busy.disabled,true);assert.equal(busy.opacity,'0.55');assert.equal(busy.cursor,'progress');
 const buttons=[];
 await b.evaluate("window.deepLegendsSections.loadStyle('settings-preferences')");
 await b.call('DOM.enable');await b.call('CSS.enable');
 for(const theme of ['dark','light']){
  await b.evaluate(`document.documentElement.dataset.theme='${theme}'`);
  for(const id of ['overview-refresh','live-refresh','pro-players-refresh','refresh-history','copy-diagnostics','export-diagnostics']){
   const read=()=>b.evaluate(`(()=>{const n=document.getElementById('${id}'),s=getComputedStyle(n),p=document.createElement('span');p.style.cssText='color:var(--ink);background:var(--surface-strong);border:1px solid var(--line)';document.body.append(p);const t=getComputedStyle(p),result={color:s.color,background:s.backgroundColor,border:s.borderTopColor,filter:s.filter,shadow:s.boxShadow,ink:t.color,surface:t.backgroundColor,line:t.borderTopColor};p.remove();return result})()`);
   const idle=await read();assert.equal(idle.color,idle.ink,id);assert.equal(idle.background,idle.surface,id);assert.equal(idle.border,idle.line,id);
   const doc=await b.call('DOM.getDocument'),node=await b.call('DOM.querySelector',{nodeId:doc.root.nodeId,selector:'#'+id});
   await b.call('CSS.forcePseudoState',{nodeId:node.nodeId,forcedPseudoClasses:['hover']});
   const hover=await read();assert.equal(hover.color,idle.color,id);assert.equal(hover.background,idle.background,id);assert.equal(hover.filter,idle.filter,id);assert.equal(hover.shadow,idle.shadow,id);
   await b.call('CSS.forcePseudoState',{nodeId:node.nodeId,forcedPseudoClasses:[]});buttons.push({theme,id,idle,hover});
  }
 }
 for(const name of ['failure.json','failure.png'])fs.rmSync(path.join(out,name),{force:true});assert.deepEqual(b.errors,[]);fs.writeFileSync(path.join(out,'result.json'),JSON.stringify({results,busy,buttons,errors:b.errors},null,2));
}catch(e){await b.shot('failure');fs.writeFileSync(path.join(out,'failure.json'),JSON.stringify({error:e.stack,errors:b.errors},null,2));throw e}finally{await b.stop()}})().catch(e=>{console.error(e);process.exitCode=1});
