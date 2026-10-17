'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const {compile}=require('./r188-harness.cjs');
const app=fs.readFileSync(process.env.R278_CLAUDE_APP_SOURCE||__dirname+'/app.js','utf8');
function fixture(){
 const dom=new JSDOM('<main class="app-frame"></main><header class="topbar" data-topbar-compact="0"></header><button id="toggle"></button><button id="settings"></button>');
 const d=dom.window.document,frame=d.querySelector('main'),observers=[];let width=900,saved='expanded';
 Object.defineProperty(frame,'clientWidth',{get:()=>width});
 class ResizeObserver {constructor(callback){this.callback=callback}observe(target){observers.push({target,callback:this.callback})}}
 const f=compile(app,['applySidebar'],{document:d,window:dom.window,preference:()=>saved,el:{sidebarToggle:d.querySelector('#toggle'),settingsSidebarToggle:d.querySelector('#settings')}});
 return {dom,d,frame,observers,f,ResizeObserver,setWidth:w=>{width=w},setSaved:v=>{saved=v},getSaved:()=>saved};
}
test('R278 820px folds the sidebar while preserving expanded preference; 821 through 900 retain measured behavior',()=>{
 const x=fixture();try{
  for(const width of [820,819,450]){x.setWidth(width);x.f.applySidebar();assert.equal(x.d.documentElement.dataset.sidebar,'collapsed',`${width}px must collapse`);assert.equal(x.getSaved(),'expanded');}
  for(const width of [821,850,900])for(const level of [0,1,3]){x.setWidth(width);x.d.querySelector('header').dataset.topbarCompact=String(level);x.f.applySidebar();assert.equal(x.d.documentElement.dataset.sidebar,'expanded',`${width}px level${level}`);}
  x.d.querySelector('header').dataset.topbarCompact='4';x.f.applySidebar();assert.equal(x.d.documentElement.dataset.sidebar,'expanded');
  x.setSaved('collapsed');for(const width of [450,820,821,900,1920]){x.setWidth(width);x.d.querySelector('header').dataset.topbarCompact='0';x.f.applySidebar();assert.equal(x.d.documentElement.dataset.sidebar,'collapsed');}
 }finally{x.dom.window.close()}
});
test('R278 app-frame ResizeObserver reapplies the 820px threshold after viewport scaling',()=>{
 const x=fixture();try{
  const start=app.indexOf('\n  applyAppearance();\n  applySidebar();'),end=app.indexOf('\n  void setupUiScaleSetting();',start);assert(start>=0&&end>start);
  Function('document','ResizeObserver','applyAppearance','applySidebar','setupTopbarCompact',app.slice(start,end))(x.d,x.ResizeObserver,()=>{},x.f.applySidebar,()=>{});
  assert.equal(x.d.documentElement.dataset.sidebar,'expanded');
  x.setWidth(820);for(const o of x.observers)if(o.target===x.frame)o.callback();
  assert.equal(x.d.documentElement.dataset.sidebar,'collapsed','app-frame resize must collapse without changing topbar level');assert.equal(x.getSaved(),'expanded');
  x.setWidth(821);for(const o of x.observers)if(o.target===x.frame)o.callback();assert.equal(x.d.documentElement.dataset.sidebar,'expanded');
 }finally{x.dom.window.close()}
});
