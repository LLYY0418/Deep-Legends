'use strict';
const assert=require('node:assert/strict'),fs=require('node:fs');
const {withBrowser}=require('./r280-browser-fixture.cjs');
// Public OP.GG provider capture; the surrounding LCU and image APIs are fixtures.
const response=JSON.parse(fs.readFileSync(process.env.R292_PRO_RESPONSE,'utf8'));
withBrowser(async({page,results,errors})=>{
 for(const theme of ['dark','light'])for(const [size,width,height] of [['wide',1440,1800],['narrow',640,2400]]){
  const p=await page(theme);
  await p.call('Emulation.setDeviceMetricsOverride',{width,height,deviceScaleFactor:1,mobile:false});
  await p.evaluate(`(()=>{const base=window.fetch;window.fetch=async(input,init)=>{const url=new URL(typeof input==='string'?input:input.url,location.href);if(url.pathname==='/api/gameplay/pro-runes')return new Response(JSON.stringify(${JSON.stringify(response)}),{headers:{'Content-Type':'application/json'}});const reply=await base(input,init);if(url.pathname!=='/api/gameplay/live')return reply;const data=await reply.json(),self=data.players.find(p=>p.isCurrent);self.championId=103;self.championName='阿狸';self.position='mid';data.recommendations.resolvedPosition='mid';data.recommendations.hero.abilities=[['Q','欺诈宝珠'],['W','妖异狐火'],['E','魅惑妖术'],['R','灵魄突袭']].map(([slot,name])=>({slot,name,description:name,iconPath:'/fixture/'+slot+'.png'}));return new Response(JSON.stringify(data),{headers:{'Content-Type':'application/json'}});};__r278App.activateSection('live');})()`);
  await p.until("!__r278Game.state.liveLoading && __r278Game.state.live?.players?.some(p=>p.isCurrent&&p.championId===103) && document.querySelector('[data-rune-source=pro]')");
  await p.evaluate("document.querySelector('[data-rune-source=pro]').click()");
  await p.until("[...document.querySelectorAll('[data-specialist-player]')].some(n=>n.textContent.includes('Kyeahoo'))");
  await p.evaluate("[...document.querySelectorAll('[data-specialist-player]')].find(n=>n.textContent.includes('Kyeahoo')).click()");
  await p.until("document.querySelectorAll('#live-content .specialist-game-row').length===2 && document.querySelector('#live-content .sg-skills')");
  const checks=await p.evaluate(`(()=>{const rows=[...document.querySelectorAll('#live-content .specialist-game-row')],plan=rows[0].querySelector('.sg-skills'),seq=[...plan.querySelectorAll('.skill-order>span')],box=plan.getBoundingClientRect();return {rows:rows.length,plans:rows.map(r=>!!r.querySelector('.sg-skills')),steps:seq.length,summary:plan.querySelector('.skill-priority-summary')?.textContent,icons:plan.querySelectorAll('.skill-icon-button img').length,tooltips:plan.querySelectorAll('.skill-icon-button[data-tooltip]').length,afterItems:!!(rows[0].querySelector('.sg-items').compareDocumentPosition(plan)&Node.DOCUMENT_POSITION_FOLLOWING),sequenceRows:new Set(seq.map(n=>Math.round(n.getBoundingClientRect().top))).size,overflow:plan.scrollWidth>plan.clientWidth+1 || [...plan.querySelectorAll('.skill-priority,.skill-order')].some(n=>n.scrollWidth>n.clientWidth+1),outside:seq.some(n=>n.getBoundingClientRect().right>box.right+1),documentOverflow:document.documentElement.scrollWidth>innerWidth+1}})()`);
  results.push({theme,size,width,...checks});
  await p.shot(size+'-pro-skills');
  assert.deepEqual(checks.plans,[true,false]);assert.equal(checks.steps,17);assert.equal(checks.icons,0);assert.equal(checks.tooltips,0);assert.match(checks.summary,/^主[QWE]副[QWE]$/);assert(checks.afterItems);assert(!checks.overflow&&!checks.outside&&!checks.documentOverflow,JSON.stringify(checks));
  assert.equal(checks.sequenceRows,1,'compact sequence stays on a single line at both widths');
 }
 assert.deepEqual(errors,[]);
}).catch(error=>{console.error(error);process.exitCode=1});
