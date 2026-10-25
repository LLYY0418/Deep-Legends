'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {read,extract}=require('./r188-harness.cjs');
const source=read('app.js');
function harness(){let at=0,next=0;const timers=new Map(),writes=[],values={'ui-scale':'auto',sidebar:'expanded'},el={sidebarToggle:{setAttribute(){},dataset:{}},settingsSidebarToggle:{}};
 const frame={clientWidth:1900},document={documentElement:{dataset:{uiScale:'1'},style:{setProperty:(k,v)=>writes.push([k,v])}},querySelector:sel=>sel==='.app-frame'?frame:null};
 const window={innerWidth:1900,innerHeight:1400,screen:{width:3000,height:2000},devicePixelRatio:1,Event:class{}};
 const deps={window,document,el,state:{section:'champions'},preference:(k,d)=>values[k]??d,savePreference:(k,v)=>values[k]=v,renderUiScaleAutoLabel(){},setTimeout:(f,ms)=>{const id=++next;timers.set(id,{f,end:at+ms});return id;},clearTimeout:id=>timers.delete(id),requestAnimationFrame:f=>{f();return 0;}};
 const methods=['autoUiScale','preferredUiScale','applyUiScale','scheduleUiScale','sidebarConstraint','applySidebar','toggleSidebar'];
 const f=Function(...Object.keys(deps),'const UI_SCALE_STEPS=[.9,1,1.1,1.25,1.4,1.5,1.75,2],UI_SCALE_BASE_WIDTH=1920,UI_SCALE_BASE_HEIGHT=900,UI_SCALE_MIN=1,UI_SCALE_MAX=2;let appliedUiScale=1,sidebarOverride=null,uiScaleFrame=0,uiScaleResizeTimer=0;'+methods.map(n=>extract(source,n)).join('\n')+';return {scheduleUiScale,applyUiScale,toggleSidebar};')(...Object.values(deps));
 return {...f,document,values,writes,resize(width){frame.clientWidth=width;window.innerWidth=width;f.scheduleUiScale();},tick(ms){at+=ms;for(const [id,t]of [...timers])if(t.end<=at){timers.delete(id);t.f();}}};
}
test('R309 thirty resize frames freeze auto zoom and settle exactly once',()=>{const h=harness();for(let i=0;i<30;i++){h.resize(1900+i*500/29);h.tick(16);assert.equal(h.writes.length,0);}h.tick(133);assert.equal(h.writes.length,0);h.tick(1);assert.deepEqual(h.writes,[['--ui-zoom','1.25']]);h.tick(1000);assert.equal(h.writes.length,1);});
test('R309 unchanged multiplier still folds and unfolds sidebar; override remains within constraint',()=>{const h=harness();h.resize(1200);h.tick(150);assert.equal(h.document.documentElement.dataset.sidebar,'expanded');h.resize(800);h.tick(150);assert.equal(h.document.documentElement.dataset.sidebar,'collapsed');h.toggleSidebar();assert.equal(h.document.documentElement.dataset.sidebar,'expanded');h.resize(790);h.tick(150);assert.equal(h.document.documentElement.dataset.sidebar,'expanded');h.resize(1200);h.tick(150);assert.equal(h.document.documentElement.dataset.sidebar,'expanded');assert.equal(h.writes.length,0);});
