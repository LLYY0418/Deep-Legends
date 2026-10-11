'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),os=require('node:os'),{spawn}=require('node:child_process');
const {JSDOM}=require('../../desktop/node_modules/jsdom'),{install}=require('./filter-dialog-fixture.cjs'),{compile}=require('./r188-harness.cjs');
test('R270 first lazy click creates the bound modal instead of deferring an unbound opening intent',()=>{
 const dom=new JSDOM('<main><div class="match-filterbar"><button data-history-filter-load>筛选</button></div><div class="match-list"></div></main>',{url:'http://fixture'});install(dom);
 const root=dom.window.document.querySelector('main'),af=require('./history-filters.js'),tab={advancedMenu:{open:true,page:'saved'},advancedConditions:{},data:{matches:[]}},ctx={rows:()=>[],cursor:()=>0,subject:()=>({}),tags:()=>[],storage:dom.window.localStorage,container:()=>root,isActive:()=>true,change(){}};
 const source=fs.readFileSync(process.env.R270_GAMEPLAY_SOURCE || path.join(__dirname,'gameplay.js'),'utf8');
 const f=compile(source,['renderFilteredMatchView'],{overviewContainer:()=>root,globalThis:{deepLegendsHistoryFilters:af},advancedFilterContext:()=>ctx,renderMatchFilters:()=>`<div class="match-filterbar">${af.render(tab,()=>ctx)}</div>`,bindMatchFilterControls:()=>af.bind(root,tab,()=>ctx),reconcileFilteredMatchList(){},appendOverviewMatches(){}});
 f.renderFilteredMatchView(tab);assert(root.querySelector('[data-af-open]'),'the real filter trigger must replace the cold fallback');assert(root._afDialog?.matches(':modal'),'first opening intent must enter the modal top layer');assert.equal(root._afBoundTab,tab);
 const bar=root.querySelector('.match-filterbar'),dialog=root._afDialog;f.renderFilteredMatchView(tab);assert.equal(root.querySelector('.match-filterbar'),bar);assert.equal(root._afDialog,dialog);assert(dialog.matches(':modal'));dom.window.close();
});
test('R270 all three persisted probes assert the saved empty state in the current modal despite other empty states',()=>{
 const dom=new JSDOM('<p class="af-empty">没有符合的对局</p><dialog data-af-menu hidden><div class="af-saved"><p class="af-empty">暂无可选项</p></div></dialog><dialog data-af-menu open><div class="af-saved"><p class="af-empty">暂无常用筛选</p></div></dialog>');
 const document=dom.window.document,query=document.querySelector.bind(document);document.querySelector=s=>query(s.replace(':modal','[open]'));
 for(const n of [265,266,269]){const source=fs.readFileSync(path.join(process.env.R270_PERSISTED_DIR || path.join(__dirname,'../../scripts'),`r${n}-persisted-ui.cjs`),'utf8'),match=source.match(/assert\(await evaluate\("([^"\n]+暂无常用筛选[^"\n]+)"\)\)/);assert(match,'saved empty-state assertion remains required');const evaluate=new Function('document',`return ${match[1]}`);assert.equal(evaluate(document),true,`r${n} must see the current modal`);query('dialog[open] .af-empty').textContent='暂无可选项';assert.equal(evaluate(document),false,`r${n} must reject the wrong current-modal content`);query('dialog[open] .af-empty').textContent='暂无常用筛选';}dom.window.close();
});
test('R270 geometry waits for a real delayed process exit before deleting its writable profile',async()=>{
 const dir=fs.mkdtempSync(path.join(os.tmpdir(),'r270-close-')),profile=path.join(dir,'profile');fs.mkdirSync(profile);
 const lifecycle=require(process.env.R270_GEOMETRY_SOURCE || '../../scripts/r269-geometry.cjs');
 const child=spawn(process.execPath,['-e',`const fs=require('node:fs');process.on('SIGTERM',()=>setTimeout(()=>{fs.mkdirSync(${JSON.stringify(profile)},{recursive:true});fs.writeFileSync(${JSON.stringify(path.join(profile,'late-write'))},'exit flush');process.exit(0)},50));process.send('ready');setInterval(()=>{},1000);`],{stdio:['ignore','ignore','pipe','ipc']});
 const closed=lifecycle.chromeClosed(child);try{await new Promise((resolve,reject)=>{child.once('message',resolve);child.once('error',reject)});await lifecycle.stopChrome(child,closed);fs.rmSync(profile,{recursive:true,force:true});await closed;assert.equal(fs.existsSync(profile),false,'a late writer must not recreate the deleted profile');}finally{child.kill('SIGKILL');fs.rmSync(dir,{recursive:true,force:true});}
});
