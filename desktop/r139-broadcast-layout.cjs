'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

// R139_LAYOUT=1 CURRENT_GAME_SHOTS=docs/r139-validation node desktop/current-game-layout.cjs
// Runs the production Suite panel in real Chromium with demo watch settings.
exports.verify = async ({call, evaluate, output}) => {
  await call('Emulation.setDeviceMetricsOverride', {width: 1200, height: 900, deviceScaleFactor: 1, mobile: false});
  await evaluate(`(() => {
    const options = [
      {key:'camp',label:'阵营位置（保留）',template:'当前阵营位置：蓝色方 / 红色方'},
      {key:'teamComposition',label:'附带己方英雄（仅队伍频道）',template:'当前己方英雄：英雄名、英雄名',teamOnly:true},
      {key:'assignedPosition',label:'附带我的分路（客户端提供时）',template:'我的分路：上路 / 打野 / 中路 / 下路 / 辅助'}
    ];
    const original = window.fetch;
    window.fetch = async (...args) => {
      const response = await original(...args);
      if (String(args[0]).split('?')[0] !== '/api/watch/rules') return response;
      const data = await response.json();
      data.broadcastOptions = options;
      return new Response(JSON.stringify(data), {status: response.status, headers: {'Content-Type':'application/json'}});
    };
    document.getElementById('section-suite').click();
  })()`);
  await evaluate(`new Promise((resolve,reject)=>{const timer=setInterval(()=>{if(document.querySelector('[data-watch-card="position-broadcast"]')){clearInterval(timer);resolve(true)}},30);setTimeout(()=>{clearInterval(timer);reject(Error('broadcast card missing'))},8000)})`);
  const cardSelector = '[data-watch-card="position-broadcast"]';
  const measure = () => evaluate(`(() => {
    const card=document.querySelector('${cardSelector}');
    const controls=[...card.querySelectorAll('[data-watch-broadcast]')];
    const row=card.querySelector('.watch-broadcast-row');
    const input=row?.querySelector('.suite-switch input');
    const bounds=card.getBoundingClientRect();
    return {count:controls.length,keys:controls.map(node=>node.dataset.watchBroadcast),hasOptions:!!card.querySelector('.watch-broadcast-options'),hasRow:!!row,
      checked:input?.checked,appearance:input?getComputedStyle(input).appearance:null,
      title:row?.querySelector('strong')?.textContent,description:row?.querySelector('small')?.textContent,
      rowWidth:row?.getBoundingClientRect().width,cardWidth:bounds.width};
  })()`);
  const capture = async name => {
    await evaluate(`document.querySelector('${cardSelector}').scrollIntoView({block:'center'});new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))`);
    const clip=await evaluate(`(() => {const r=document.querySelector('${cardSelector}').getBoundingClientRect();return {x:Math.max(0,r.x-10),y:Math.max(0,r.y-10),width:r.width+20,height:r.height+20,scale:1}})()`);
    const shot=await call('Page.captureScreenshot',{format:'png',clip});
    fs.writeFileSync(path.join(output,name),Buffer.from(shot.data,'base64'));
  };
  const self=await measure();
  assert.equal(self.count,0,'self visibility has no checkboxes');
  assert.equal(self.hasOptions,false,'self visibility omits the options node');
  await capture('broadcast-self.png');

  await evaluate(`document.querySelector('${cardSelector} [data-watch-choice-value="team"]').click()`);
  await evaluate(`new Promise((resolve,reject)=>{const timer=setInterval(()=>{if(document.querySelector('${cardSelector} .watch-broadcast-row')){clearInterval(timer);resolve(true)}},30);setTimeout(()=>{clearInterval(timer);reject(Error('team row missing'))},8000)})`);
  const team=await measure();
  assert.deepEqual(team.keys,['teamComposition']);
  assert.equal(team.appearance,'none','team composition uses the pill switch');
  assert.equal(team.checked,false,'team composition keeps the default off');
  assert.ok(team.title.includes('附带己方英雄'));
  assert.ok(team.description.includes('当前己方英雄'));
  assert.ok(team.rowWidth<=team.cardWidth,'row fits its card');
  await capture('broadcast-team.png');

  await evaluate(`document.querySelector('${cardSelector} [data-watch-broadcast="teamComposition"]').click()`);
  await evaluate(`new Promise((resolve,reject)=>{const timer=setInterval(()=>{const input=document.querySelector('${cardSelector} [data-watch-broadcast="teamComposition"]');if(input?.checked){clearInterval(timer);resolve(true)}},30);setTimeout(()=>{clearInterval(timer);reject(Error('toggle did not persist'))},8000)})`);
  const enabled=await measure();
  assert.equal(enabled.checked,true);

  await evaluate(`document.querySelector('${cardSelector} [data-watch-choice-value="self"]').click()`);
  await evaluate(`new Promise((resolve,reject)=>{const timer=setInterval(()=>{const card=document.querySelector('${cardSelector}');if(card&&!card.querySelector('.watch-broadcast-options')){clearInterval(timer);resolve(true)}},30);setTimeout(()=>{clearInterval(timer);reject(Error('options node still present'))},8000)})`);
  assert.equal((await measure()).count,0);
  console.log('R139 Chromium PASS',JSON.stringify({self,team,enabled}));
};
