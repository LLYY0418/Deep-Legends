'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {bootDemoApp}=require('../../desktop/overview-render-helpers.cjs'),{waitForRender}=require('../../desktop/renderer-wait.cjs');
test('R284 delayed career close preserves a reopened dialog and its return scroll; normal close clears it',async()=>{
 const f=bootDemoApp({gameplaySourceTransform:source=>(process.env.R284_GAME_SOURCE?fs.readFileSync(process.env.R284_GAME_SOURCE,'utf8'):source).replace(/\}\)\(\);\s*$/,'window.__r284Career={state,nodes,activeTab,overviewContainer,openCareerDialog,closeCareerDialog};\n})();')});
 const w=f.window,g=w.__r284Career;
 try{
  await waitForRender(w,()=>g?.activeTab()?.data&&!g.activeTab().loading&&w.document.querySelector('.career-column'),'career fixture ready',10000);
  Object.defineProperty(w.HTMLElement.prototype,'offsetParent',{configurable:true,get(){return this.matches('[data-open-career-dialog]')?w.document.body:null}});
  const tab=g.activeTab(),container=g.overviewContainer(tab),dialog=g.nodes.careerDialog,root=g.nodes.careerDialogContent,pending=[];
  dialog.showModal=function(){this.setAttribute('open','')};
  dialog.close=function(){this.removeAttribute('open');pending.push(()=>this.dispatchEvent(new w.Event('close')))};
  const frames=()=>new Promise(r=>w.requestAnimationFrame(()=>w.requestAnimationFrame(r)));
  for(const width of [500,900])for(const normalClose of ['button','Escape']){
   Object.defineProperty(container,'clientWidth',{configurable:true,value:width});
   const opener=container.querySelector('[data-open-career-dialog]');assert(opener);
   g.openCareerDialog(container,tab,opener);await frames();root.scrollTop=187;
   const beforeSubpageScroll=root.scrollTop;g.closeCareerDialog();assert.equal(dialog.open,false);
   g.openCareerDialog(container,tab,opener,{scrollTop:beforeSubpageScroll});
   const sections=[...root.querySelectorAll('[data-career-key]')],context=g.nodes.careerDialogContext.textContent;assert(sections.length>0);assert(context);
   pending.shift()();await frames();
   assert.equal(dialog.open,true);assert(sections.every(section=>root.contains(section)),'late close cleared reopened content');
   assert.equal(w.document.getElementById('career-dialog-title').textContent,'生涯统计');
   assert.equal(g.nodes.careerDialogContext.textContent,context);assert.equal(root.scrollTop,beforeSubpageScroll);
   if(normalClose==='button')g.nodes.careerDialogClose.click();
   else dialog.dispatchEvent(new w.KeyboardEvent('keydown',{key:'Escape',bubbles:true,cancelable:true}));
   assert.equal(dialog.open,false);pending.shift()();assert.equal(root.childElementCount,0);assert.equal(g.nodes.careerDialogContext.textContent,'');
  }
  assert.deepEqual(f.errors,[]);
 }finally{w.close()}
});
