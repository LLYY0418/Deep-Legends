'use strict';
const assert = require('node:assert/strict');
const {withBrowser} = require('./r280-browser-fixture.cjs');
withBrowser(async ({page, results, errors}) => {
  for (const theme of ['dark', 'light']) {
    const p = await page(theme);
    await p.until('__r278Game.activeTab()?.data && !__r278Game.activeTab().loading');
    await p.evaluate(`(async()=>{for(const name of ['live-arena.css','live-arena.js']){const n=document.createElement(name.endsWith('.css')?'link':'script');if(n.tagName==='LINK'){n.rel='stylesheet';n.href='/'+name;}else n.src='/'+name;await new Promise((r,j)=>{n.onload=r;n.onerror=j;document.head.append(n)});}const stage=document.createElement('main');stage.id='r288-stage';stage.style.cssText='position:absolute;top:20px;left:20px;z-index:9999;background:var(--bg);container:recommendation-area / inline-size';document.body.append(stage)})()`);
    for (const zoom of [1, 1.5]) for (const width of [1250, 600]) {
      const measurements = await p.evaluate(`(async()=>{const stage=document.getElementById('r288-stage');stage.style.width='${width}px';stage.style.zoom=${zoom};const counts=[4,10,10,10,4,10];const players=counts.map((count,i)=>({playerRef:'arena-'+i,arenaSquadKey:i<3?'mine':'premade:A',teamId:100,isCurrent:i===0,championId:103,championName:'阿狸',gameName:'玩家'+i,historyState:'ok',arenaRecord:{games:count,topHalf:2,top1:1},recentGames:Array.from({length:count},(_,j)=>({gameId:2880+j,championId:103,championName:'阿狸',kills:10,deaths:2,assists:12,win:true,placement:1}))}));stage.innerHTML=__r278Game.renderLiveInsights({phase:'InProgress',queueId:1750,gameMode:'CHERRY',mapId:30,gameId:288,arenaGrouped:true,players});await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));return [...stage.querySelectorAll('.live-team')].map(t=>({height:t.getBoundingClientRect().height,rows:[...t.querySelectorAll('.insight-match-row')].map(row=>({count:row.children.length,top:row.getBoundingClientRect().top,chip:row.firstElementChild.getBoundingClientRect().top,align:getComputedStyle(row).alignContent}))}))})()`);
      assert.equal(measurements.length, 2);
      if (width > 840) assert.equal(measurements[0].height, measurements[1].height);
      for (const card of measurements) {
        assert.equal(card.rows.length, 3);
        for (const row of card.rows) assert.equal(row.chip, row.top, JSON.stringify({theme,zoom,width,row}));
      }
      if (width > 840) for (let i=0;i<3;i++) assert.equal(measurements[0].rows[i].chip,measurements[1].rows[i].chip);
      results.push({theme, zoom, width, measurements});
      await p.shot(`r288-arena-${width}-${zoom}`);
    }
  }
  assert.deepEqual(errors, []);
}).catch(error => {console.error(error); process.exitCode=1;});
