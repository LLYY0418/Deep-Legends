'use strict';
const assert=require('node:assert/strict'),{withBrowser}=require('./r280-browser-fixture.cjs');
withBrowser(async({page,results})=>{for(const theme of ['dark','light']){
 const p=await page(theme);await p.until('__r278Game.activeTab()?.data');
 await p.evaluate("const stage=document.createElement('main');stage.id='r284-arena';stage.style.cssText='position:fixed;inset:100px 20px 40px 100px;z-index:999;overflow:auto;background:var(--bg);padding:15px';document.body.append(stage)");
 for(const count of [2,4]){const r=await p.evaluate(`(()=>{const g=__r278Game,t=g.activeTab(),m=structuredClone(t.data.matches[0]);m.queueId=1750;m.gameMode='CHERRY';m.modeGroup='arena';m.participants[0].augmentIds=[89,93,347,359].slice(0,${count});m.subjectParticipantId=m.participants[0].participantId;document.getElementById('r284-arena').innerHTML=g.renderMatch(m,t.data.player.playerRef,t);return [...document.querySelectorAll('#r284-arena .match-loadout-mini.is-augments>*')].map(n=>({empty:n.classList.contains('is-empty'),...n.getBoundingClientRect().toJSON()}))})()`);
 assert.equal(r.length,4);assert.equal(r[0].x,r[1].x);assert(r[1].y>r[0].y);assert(r[2].x>r[0].x);assert.equal(r[2].y,r[0].y);results.push({theme,count,slots:r});if(count===4)await p.shot('p4-arena');
 }
}}).catch(e=>{console.error(e);process.exitCode=1});
