'use strict';
// All standalone real-Chromium guard entry points, including wrapper variants.
const guards=[
 ['r100','desktop/r100-browser.cjs','R100_BROWSER_OUTPUT'],
 ['r117','desktop/r117-browser.cjs','R117_BROWSER_OUTPUT'],
 ['current-game-layout','desktop/current-game-layout.cjs','CURRENT_GAME_SHOTS'],
 ['overview-banner-layout','desktop/overview-banner-layout.cjs','R276_BANNER_OUT'],
 ['pro-players-layout','desktop/pro-players-layout.cjs','R73_SCREENSHOT_DIR'],
 ['r258','scripts/r258-browser.cjs','R258_BROWSER_OUT'],
 ['r264','scripts/r264-browser.cjs','R264_BROWSER_OUT'],
 ['r265','scripts/r265-browser.cjs','R265_BROWSER_OUT'],
 ['r266','scripts/r266-browser.cjs','R266_BROWSER_OUT'],
 ['r266-matrix','scripts/r266-browser.cjs','R266_BROWSER_OUT',{R266_MATRIX:'1'}],
 ['r267','scripts/r267-browser.cjs','R267_BROWSER_OUT'],
 ['r269-geometry','scripts/r269-geometry.cjs','R269_GEOMETRY_OUT'],
 ['r269','scripts/r269-browser.cjs','R269_BROWSER_OUT',{R276_TRACE:'1'}],
 ['r272','scripts/r272-browser.cjs','R266_BROWSER_OUT'],
 ['r278','scripts/r278-browser.cjs','R278_BROWSER_OUT'],
 ['r275-offline','scripts/r275-offline-browser.cjs','R275_OFFLINE_OUT']
];
function selectGuards(shard,only){
 if(!['all','main','r265'].includes(shard))throw Error('unknown browser shard '+shard);
 if(only)for(const name of only)if(!guards.some(row=>row[0]===name))throw Error('unknown focused browser guard '+name);
 return guards.filter(row=>(!only||only.includes(row[0]))&&(shard==='all'||(shard==='r265'?row[0]==='r265':row[0]!=='r265')));
}
module.exports={guards,selectGuards};
