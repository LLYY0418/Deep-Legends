// Mutate the actual source, require real Chromium assertions to fail, then restore
// byte-for-byte. Never use git checkout: the checkout contains ongoing user work.
'use strict';
const fs=require('node:fs');
const path=require('node:path');
const assert=require('node:assert/strict');
const {spawnSync}=require('node:child_process');
const root=path.resolve(__dirname,'..');
const files=['backend/web/gameplay.css','backend/web/gameplay.js'];
const original=Object.fromEntries(files.map(f=>[f,fs.readFileSync(path.join(root,f),'utf8')]));
const output=path.join(root,'output/playwright/r74');fs.mkdirSync(output,{recursive:true});
let active;
const restore=()=>{
 if(!active)return;
 const {file,from,to,before}=active;const filename=path.join(root,file);const current=fs.readFileSync(filename,'utf8');
 if(current===before){active=null;return;}
 // Reverse only our mutation, preserving unrelated edits from another task.
 assert.equal(current.split(to).length,2,'cannot safely restore mutation target: '+file);
 const restored=current.replace(to,from);fs.writeFileSync(filename,restored);active=null;
 assert.equal(restored,before,'Concurrent source edit preserved; stop and rerun on a stable tree.');
};
const replace=(text,from,to)=>{assert.equal(text.split(from).length,2,'mutation target must be unique: '+from);return text.replace(from,to);};
const cases=[
 ['M1','backend/web/gameplay.css','minmax(0, var(--cg-recent-w)) minmax(var(--cg-badges-w), 1fr)','minmax(0, 1fr) minmax(var(--cg-badges-w), 1fr)','recentOverflow|fixed recent track geometry contract|badges between recent and rank|badge left edge'],
 ['M2','backend/web/gameplay.css','align-self: center; justify-self: start; justify-content: flex-start; gap: 3px','align-self: center; justify-self: end; justify-content: flex-start; gap: 3px','badge start alignment contract|badges between recent and rank|badge left edge'],
 ['M3','backend/web/gameplay.css','justify-self: start; width: 100%; gap: 8px; min-width: 0; font-size: 12px','justify-self: end; gap: 8px; min-width: 0; font-size: 12px','rank left edge|rank start alignment contract'],
 ['M4','backend/web/gameplay.css','grid-area: recent; display: flex; min-width: 0; gap: 3px; overflow: hidden','grid-area: recent; display: flex; min-width: 0; gap: 3px; overflow: auto','recent must not create a scrolling surface|recentOverflow|row height'],
 ['M5','backend/web/gameplay.css','@container current-game-team (max-width: 372px)','@container current-game-team (max-width: 0px)','recentOverflow|hidden clipping audit|container whole-item count'],
 ['M6','backend/web/gameplay.css','.current-game-recent-item.is-win { border-color: var(--success); }','.current-game-recent-item.is-win { border-color: var(--accent); }','current-game wins must be success green'],
 ['M7','backend/web/gameplay.js','${iconFigure("champion", recent.championId, recent.championName, "small", false)}</span>','${iconFigure("champion", recent.championId, recent.championName, "small", false)}<span class="current-game-recent-spells">${(recent.spells || []).map(id => spellIconFigure(id, "small")).join("")}</span></span>','recent items must not contain spell icons|hidden clipping audit|recentOverflow'],
 ['M8','backend/web/gameplay.css','.player-group-menu button[aria-checked="true"]','.player-group-menu button[aria-selected="true"]','aria-checked selection must have visible highlight','groups'],
 ['M9','backend/web/gameplay.js','nodes.playerGroups?.addEventListener("keydown", event => {','nodes.playerGroups?.addEventListener("r74-disabled-keydown", event => {','keyboard Enter opens the group menu|keyboard Enter moves focus into menu','groups']
];
const results=[];
try {
 for(const [id,file,from,to,expected,suite]of cases){
  const before=fs.readFileSync(path.join(root,file),'utf8');
  active={file,from,to,before};fs.writeFileSync(path.join(root,file),replace(before,from,to));
  const result=spawnSync(process.execPath,[path.join(__dirname,'current-game-layout.cjs')],{cwd:root,encoding:'utf8',timeout:120000,maxBuffer:10*1024*1024,env:{...process.env,R74_LAYOUT_SUITE:suite||'',CURRENT_GAME_SHOTS:path.join(output,'mutation-shots',id)}});
  const log=(result.stdout||'')+(result.stderr||'');fs.writeFileSync(path.join(output,id+'.log'),log);
  assert.ok(!result.error,`${id}: browser infrastructure failure ${result.error}`);
  assert.notEqual(result.status,0,`${id} survived`);assert.match(log,/AssertionError/,`${id} must fail a test, not startup`);assert.match(log,new RegExp(expected),`${id} failed an unrelated assertion`);
  const failure=log.slice(log.indexOf('AssertionError'),log.indexOf('AssertionError')+900);
  results.push({id,status:'KILLED',exit:result.status,failure});restore();console.log(id,'KILLED',failure.split('\n')[0]);
 }
} finally {restore();fs.writeFileSync(path.join(output,'mutations.json'),JSON.stringify(results,null,2));}
for(const [file,text]of Object.entries(original))assert.equal(fs.readFileSync(path.join(root,file),'utf8'),text);
console.log('All nine real-source mutations killed; sources restored byte-for-byte.');
