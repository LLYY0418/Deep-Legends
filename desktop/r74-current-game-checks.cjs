'use strict';
const assert = require('node:assert/strict');
exports.verify = async ({evaluate,width,theme}) => {
 const m = await evaluate(`(()=>{
 const card=document.querySelector('.current-game-card');
 const rows=[...card.querySelectorAll('.current-game-player')].map(row=>{
  const rb=row.getBoundingClientRect();
  const q=s=>{const e=row.querySelector(s);const r=e.getBoundingClientRect();const c=getComputedStyle(e);return {x:r.x-rb.x,right:r.right-rb.x,w:r.width,h:r.height,justify:c.justifySelf,overflow:c.overflowX};};
  const recent=row.querySelector('.current-game-recent');
  const visible=[...recent.children].filter(e=>getComputedStyle(e).display!=='none');
  return {h:rb.height,badges:q('.current-game-badges'),recent:q('.current-game-recent'),rank:q('.current-game-rank'),
   crest:q('.current-game-rank > .rank-crest-icon'),
   team:row.closest('.current-game-team').getBoundingClientRect().width,
   preferredWidth:parseFloat(getComputedStyle(row).getPropertyValue('--cg-recent-w')),
   count:visible.length,total:recent.children.length,
   whole:visible.every(e=>e.getBoundingClientRect().right<=recent.getBoundingClientRect().right+1),
   tabular:getComputedStyle(row.querySelector('.current-game-rank small')).fontVariantNumeric};
 });
 const clipped=[...card.querySelectorAll('*')].filter(e=>!e.closest('.game-icon')&&e.scrollWidth-e.clientWidth>1&&getComputedStyle(e).overflowX==='hidden').map(e=>e.className);
 const color=variable=>{const e=document.createElement('span');e.style.color='var('+variable+')';card.append(e);const c=getComputedStyle(e).color;e.remove();return c;};
 return {rows,clipped,win:getComputedStyle(card.querySelector('.current-game-recent-item.is-win')).borderBottomColor,loss:getComputedStyle(card.querySelector('.current-game-recent-item.is-loss')).borderBottomColor,
 success:color('--success'),danger:color('--danger'),spells:card.querySelectorAll('.current-game-recent-spells').length,
 time:card.querySelector('time').textContent,tooltip:card.querySelector('.current-game-recent-item').dataset.tooltip,
 streaks:[...card.querySelectorAll('.current-game-streak')].map(e=>({svg:e.querySelector('svg')?.getBoundingClientRect().toJSON(),position:e.parentElement.querySelector('.current-game-position').getBoundingClientRect().toJSON(),label:e.getAttribute('aria-label'),tip:e.dataset.tooltip}))};
 })()`);
 const spread = values => Math.max(...values)-Math.min(...values);
 assert.deepEqual(m.clipped,[],`hidden clipping audit ${theme}/${width}: ${m.clipped}`);
 assert.ok(spread(m.rows.map(r=>r.badges.x))<=1,'badge left edge spread <=1px');
 assert.ok(spread(m.rows.map(r=>r.rank.x))<=1,'rank left edge spread <=1px');
 assert.ok(spread(m.rows.map(r=>r.crest.x))<=1,'crest left edge spread <=1px');
 assert.ok(spread(m.rows.map(r=>r.h))<=.5,'row height spread <=0.5px');
 assert.ok(spread(m.rows.map(r=>r.badges.x-r.recent.right))<=1,'badge gap constant');
 for(const r of m.rows) {
  assert.ok(r.badges.x>=r.recent.right&&r.badges.right<=r.rank.x,'badges between recent and rank');
  assert.equal(r.badges.justify,'start','badge start alignment contract');
  assert.equal(r.rank.justify,'start','rank start alignment contract');
  assert.equal(r.rank.w,104,'rank fixed width contract');
  // Independent geometry and computed-style contracts: a redundant repair may absorb a single mutation.
  assert.ok(Math.abs(r.recent.w-r.preferredWidth)<=1,'fixed recent track geometry contract');
  assert.equal(r.recent.overflow,'hidden','recent must not create a scrolling surface');
  assert.equal(r.count,Math.min(r.total,r.team<=373?6:8),'container whole-item count');
  assert.equal(r.whole,true,'no partial recent icons');
  assert.equal(r.tabular,'tabular-nums');
 }
 assert.equal(m.spells,0,'recent items must not contain spell icons');
 assert.equal(m.win,m.success,'current-game wins must be success green');
 assert.equal(m.loss,m.danger,'current-game losses must be danger red');
 assert.match(m.time,/^已进行 \d+:\d{2}$/);
 assert.match(m.tooltip,/李青 · 胜利 · 闪现 \/ 惩戒/);
 assert.equal(m.streaks.length,2);
 for(const streak of m.streaks) {
  assert.ok(streak.svg,'streak must use SVG');
  assert.equal(streak.svg.width,17);assert.equal(streak.svg.height,17);
  assert.ok(Math.abs(streak.svg.width-streak.position.width)<=1);
  assert.equal(streak.label,streak.tip);assert.match(streak.label,/^最近处于连[胜败]状态$/);
 }
 console.log('R74 geometry',JSON.stringify({theme,width,badgeSpread:spread(m.rows.map(r=>r.badges.x)),rankSpread:spread(m.rows.map(r=>r.rank.x)),heights:[...new Set(m.rows.map(r=>r.h))],clipped:m.clipped.length,counts:m.rows.map(r=>r.count)}));
};
