'use strict';
require('../../scripts/patch-notes/extract.test.cjs');
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),{JSDOM}=require('../../desktop/node_modules/jsdom');
const feed=JSON.parse(fs.readFileSync(__dirname+'/../patchnotes/patch-notes.json'));
const escape=value=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const tick=()=>new Promise(r=>setImmediate(r));
async function fixture(payload=feed.versions[0],fetcher){const dom=new JSDOM('<!doctype html><html><body><main id="root"></main></body></html>',{url:'http://127.0.0.1',runScripts:'outside-only'}),w=dom.window;w.deepLegendsRuntime={escapeHTML:escape};w.deepLegendsSections={register(){}};w.fetch=fetcher||(()=>Promise.resolve({ok:true,json:async()=>structuredClone(payload)}));w.AbortController=AbortController;w.eval(fs.readFileSync(__dirname+'/patch-notes.js','utf8'));await tick();return {dom,w,open(mode){w.document.getElementById('root').innerHTML=w.deepLegendsPatchNotes.button(mode);const b=w.document.querySelector('[data-patch-notes]');b.click();return b}}}
for(const [mode,version,expected]of [['ranked','26.20','铁血狼母 安蓓萨'],['aram-mayhem','26.20','木灵 法术强度收益率'],['arena','26.17','超越死亡的效劳']])test(`R297 patch dialog renders only ${mode}, official intro and text rows`,async()=>{const f=await fixture(feed.versions.find(v=>v.version===version));try{const opener=f.open(mode),d=f.w.document.querySelector('.pn-dialog');assert(d.textContent.includes(expected));assert(d.contains(f.w.document.activeElement));assert.equal(d.querySelectorAll('[role=tab]').length,0);assert.equal(d.querySelector('.pn-foot a').hostname,'lol.qq.com');if(mode==='ranked'){assert(d.querySelector('.pn-line').textContent.includes('高熟练度'));assert(!d.textContent.includes('木灵 法术强度收益率'))}else{assert(d.querySelector('.pn-intro'));assert(!d.textContent.includes('记分板显示优化'))}d.dispatchEvent(new f.w.KeyboardEvent('keydown',{key:'Escape',bubbles:true}));assert(!f.w.document.querySelector('.pn-dialog'));assert.equal(f.w.document.activeElement,opener)}finally{await tick();f.dom.window.close()}});
test('R297 empty mode, malicious text escape, source whitelist and bounded strings',async()=>{const f=await fixture(feed.versions.find(v=>v.version==='26.20'));try{f.open('arena');assert.match(f.w.document.querySelector('.pn-body').textContent,/本版本没有该模式的改动/);f.w.document.querySelector('.pn-scrim').dispatchEvent(new f.w.MouseEvent('mousedown',{bubbles:true}));assert(!f.w.document.querySelector('.pn-dialog'));const x=structuredClone(feed.versions.find(v=>v.version==='26.20'));x.modes.ranked.sections[0].entries[0].name='<script>window.pwned=true</script>';const html=f.w.deepLegendsPatchNotes.renderSections(x.modes.ranked.sections);f.w.document.getElementById('root').innerHTML=html;assert.equal(f.w.document.querySelectorAll('script').length,0);assert(f.w.document.body.textContent.includes('<script>'));assert.equal(f.w.pwned,undefined);assert.throws(()=>f.w.deepLegendsPatchNotes.validate({...x,sourceUrl:'https://lol.qq.com.evil.example/'}));x.modes.ranked.sections[0].intro='x'.repeat(8193);assert.throws(()=>f.w.deepLegendsPatchNotes.validate(x))}finally{await tick();f.dom.window.close()}});
test('R297 loading skeleton, unavailable retry, async focus retained and dialog Tab loops',async()=>{let resolve;const pending=new Promise(r=>resolve=r);const f=await fixture(undefined,()=>pending);try{f.open('ranked');assert.equal(f.w.document.querySelectorAll('.pn-loading i').length,3);resolve({ok:false});await tick();const retry=f.w.document.querySelector('[data-pn-retry]');assert(retry);assert(f.w.document.querySelector('.pn-dialog').contains(f.w.document.activeElement));f.w.fetch=async()=>({ok:true,json:async()=>feed.versions[0]});retry.focus();retry.click();await tick();assert(f.w.document.querySelector('.pn-entry'));const dialog=f.w.document.querySelector('.pn-dialog'),last=dialog.querySelector('a');last.focus();dialog.dispatchEvent(new f.w.KeyboardEvent('keydown',{key:'Tab',bubbles:true,cancelable:true}));assert.equal(f.w.document.activeElement,dialog.querySelector('.pn-close'))}finally{await tick();f.dom.window.close()}});

test('R297 equipment count separates system text and includes mixed plain items',async()=>{const f=await fixture();try{const sections=feed.versions.find(v=>v.version==='26.20').modes.ranked.sections;assert.match(f.w.deepLegendsPatchNotes.renderSections(sections),/3 件 · 1 项/);assert.match(f.w.deepLegendsPatchNotes.renderSections([{id:'bugfixes',title:'BUG修复',entries:[{name:'一',groups:[]}],items:['二']}]),/2 项/)}finally{await tick();f.dom.window.close()}});

test('latest Mayhem notes retain 35 additions, 13 removals and Headliners',async()=>{const f=await fixture();try{f.open('aram-mayhem');await tick();const d=f.w.document.querySelector('.pn-dialog');assert.equal(d.querySelectorAll('.pn-new').length,35);assert(d.textContent.includes('赛季之星'));assert(d.textContent.includes('已移除的强化符文'));assert(d.textContent.includes('13 项'));}finally{await tick();f.dom.window.close()}});

test('opening saved notes does not check upstream; explicit check still refreshes',async()=>{
  const calls=[];
  const f=await fixture(undefined,async url=>{calls.push(url);return {ok:true,json:async()=>({...structuredClone(feed.versions[0]),successfulAt:new Date().toISOString()})};});
  try{
    f.open('ranked');await tick();
    f.w.document.querySelector('[data-pn-close]').click();f.open('aram-mayhem');await tick();
    assert.deepEqual(calls,['/api/patch-notes']);
    f.w.document.querySelector('[data-pn-retry]').click();await tick();
    assert.deepEqual(calls,['/api/patch-notes','/api/patch-notes?refresh=1']);
  }finally{await tick();f.dom.window.close()}
});

test('patch notes separate prose, list names and augment artwork without changing text',async()=>{
  const payload=structuredClone(feed.versions[0]);
  payload.augmentIcons={'蓝色屏障':{source:'opgg',path:'/meta/images/lol/latest/aram-augment/test.png'}};
  const f=await fixture(payload);
  try{
    f.open('ranked');await tick();
    assert(f.w.document.querySelector('.pn-line.is-explanation'));
    assert(f.w.document.querySelector('.pn-line .k'));
    f.w.document.querySelector('[data-pn-close]').click();
    f.open('aram-mayhem');await tick();
    const title=[...f.w.document.querySelectorAll('.pn-entry-name')].find(n=>n.textContent==='蓝色屏障');
    const entry=title.closest('.pn-entry');
    assert(entry.classList.contains('is-augment'));
    assert.match(entry.querySelector('img').dataset.queuedSrc,/source=opgg&path=%2Fmeta%2Fimages/);
    const unknown=[...f.w.document.querySelectorAll('.pn-entry-name')].find(n=>n.textContent==='没法力，没问题');
    assert.equal(unknown.closest('.pn-entry').querySelector('img'),null);
    const root=f.w.document.getElementById('root');
    root.innerHTML=f.w.deepLegendsPatchNotes.renderSections([{id:'headliners',title:'赛季之星',items:['K/DA：收集卡片：获得奖励','<b>心之钢</b>: <img src=x onerror=alert(1)>','还有更多？？？']}]);
    assert.equal(root.querySelector('.pn-list-name').textContent,'K/DA：');
    assert.equal(root.querySelector('.pn-list-description').textContent,'收集卡片：获得奖励');
    assert.equal(root.querySelectorAll('img,b').length,0);
    assert.equal(root.querySelectorAll('.pn-list-name').length,2);
  }finally{await tick();f.dom.window.close()}
});

test('an update event during an in-flight read is replayed after that read',async()=>{let resolve;let reads=0;const pending=new Promise(r=>resolve=r);const payload=feed.versions[0];const f=await fixture(undefined,()=>++reads===1?pending:Promise.resolve({ok:true,json:async()=>({...structuredClone(payload),updateStatus:'checked'})}));try{f.w.dispatchEvent(new f.w.Event('deep-legends:patch-notes-changed'));resolve({ok:true,json:async()=>({...structuredClone(payload),updateStatus:'checking'})});await tick();await tick();assert.equal(reads,2);f.open('aram-mayhem');await tick();assert(f.w.document.querySelector('.pn-foot').textContent.includes('已检查官方公告'));}finally{await tick();f.dom.window.close()}});
