'use strict';
const assert=require('node:assert/strict'),{withBrowser}=require('./r280-browser-fixture.cjs');
const only=process.env.R286_ONLY;
withBrowser(async({page,results,errors})=>{
 for(const theme of ['dark','light']){
 const p=await page(theme);await p.until('__r278Game.activeTab()?.data&&!__r278Game.activeTab().loading');
 if(!only || only==='p2'){
 await p.evaluate("__r278App.activateSection('suite');document.querySelector('[data-suite-tab=facade]').click()");await p.until('window.__r282Suite?.state.facade && document.querySelector(\'.facade-film\')');await p.evaluate("document.querySelector('[data-suite-tab=facade]').click()");await p.until("document.querySelector('.facade-background-card').getBoundingClientRect().height>0");
 for(const width of [1500,640])for(const zoom of [1,1.5]){
 await p.call('Emulation.setDeviceMetricsOverride',{width,height:1200,deviceScaleFactor:1,mobile:false});
 const r=await p.evaluate(`(()=>{document.documentElement.style.setProperty('--ui-zoom','${zoom}');document.getElementById('app-scroll').scrollTop=0;const s=__r282Suite;const skin=s.state.facade.skins[0];s.state.facade.profile={...s.state.facade.profile,backgroundSkinId:skin.id};s.state.facadeDraft.hero=String(skin.championId);const rows=[];for(const mode of ['applied','apply','disabled','apply','applied']){s.state.facadeDraft.skinId=mode==='applied'?skin.id:mode==='apply'?(s.state.facade.skins.find(x=>x.id!==skin.id)?.id || -1):0;s.renderFacade();const head=document.querySelector('.facade-background-card .suite-card-head'),film=document.querySelector('.facade-film');rows.push({mode,height:head.getBoundingClientRect().height,title:head.querySelector('h3').getBoundingClientRect().top,grid:film.getBoundingClientRect().top,button:!!head.querySelector('button:not([hidden])'),disabled:head.querySelector('button').disabled});}return rows})()`);
 assert(r.every(x=>x.height>0&&x.grid>0),'P2 requires visible geometry');for(const k of ['height','title','grid'])assert.equal(Math.max(...r.map(x=>x[k]))-Math.min(...r.map(x=>x[k])),0,'P2 fixed header '+k+JSON.stringify({theme,width,zoom,r}));
 assert(r[0].button===false&&r[1].button&&r[2].disabled);results.push({kind:'P2',theme,width,zoom,rows:r});await p.shot('career-'+width+'-'+zoom);
 }
 }
 if(!only || only==='p3'){
 await p.evaluate("__r278App.activateSection('overview');document.documentElement.style.setProperty('--ui-zoom','1');const stage=document.createElement('main');stage.id='r286-notice';stage.style.cssText='position:fixed;inset:120px 20px auto 230px;z-index:9999;background:var(--surface);padding:20px';document.body.append(stage)");
 for(const kind of ['official','relay']){
 const r=await p.evaluate(`(()=>{const tab={region:'${kind==='relay'?'KR':''}',historyFailureClass:'${kind}',data:{pagination:{historyServiceDown:${kind==='official'}},capabilities:[]}};const s=document.getElementById('r286-notice');s.innerHTML=__r278Game.renderHistoryServiceStatus(tab,true)+'<span id="r286-primary" style="color:var(--primary)">主题色</span>';const n=s.querySelector('.service-outage-compact>span'),c=getComputedStyle(n);n.parentElement.append(document.getElementById('r286-primary'));const canvas=document.createElement('canvas'),ctx=canvas.getContext('2d');function rgb(v){ctx.clearRect(0,0,1,1);ctx.fillStyle=v;ctx.fillRect(0,0,1,1);return [...ctx.getImageData(0,0,1,1).data].slice(0,3)}function lum(v){const a=rgb(v).map(x=>{x/=255;return x<=.04045?x/12.92:((x+.055)/1.055)**2.4});return .2126*a[0]+.7152*a[1]+.0722*a[2]}const fg=lum(c.color),bg=lum(getComputedStyle(n.parentElement).backgroundColor);return {text:n.textContent,color:c.color,primary:getComputedStyle(document.getElementById('r286-primary')).color,weight:c.fontWeight,contrast:(Math.max(fg,bg)+.05)/(Math.min(fg,bg)+.05)}})()`);
 assert.equal(r.text,kind==='official'?'官方战绩服务暂时中断，以下为已保存的对局':'外服战绩连接较慢');assert.equal(r.color,r.primary,'P3 primary color');assert.equal(r.weight,'700');assert(r.contrast>=4.5,'P3 text contrast '+JSON.stringify(r));results.push({kind:'P3',theme,type:kind,...r});
 }
 await p.shot('overview-outage');await p.evaluate("document.getElementById('r286-notice').remove()");
 }
 if(!only || only==='p4'){
 await p.evaluate("__r278App.activateSection('live')");await p.until('!__r278Game.state.liveLoading');
 for(const width of [1500,640])for(const zoom of [1,1.5]){
 await p.call('Emulation.setDeviceMetricsOverride',{width,height:1200,deviceScaleFactor:1,mobile:false});
 const r=await p.evaluate(`(()=>{const g=__r278Game;document.documentElement.style.setProperty('--ui-zoom','${zoom}');g.state.settings.liveRefresh=false;g.state.liveLoadingVisible=false;g.state.beacon.phase='InProgress';g.state.live={...g.state.live,phase:'InProgress',available:true,queueId:420,queueLabel:'单排/双排',historyServiceDown:false,players:[]};g.renderLive('r286');const bar=document.querySelector('.live-toolbar'),summary=bar.querySelector('.live-session-summary'),before=summary.getBoundingClientRect().toJSON(),title=summary.querySelector('strong').getBoundingClientRect().toJSON();g.state.live.historyServiceDown=true;g.renderLive('r286');const n=bar.querySelector('.is-service-outage'),status=n.getBoundingClientRect(),button=bar.querySelector('#live-refresh').getBoundingClientRect(),text=n.querySelector('span'),primary=document.createElement('i');primary.style.color='var(--primary)';n.append(primary);const c=getComputedStyle(n),color=getComputedStyle(primary).color;primary.remove();return {before,after:summary.getBoundingClientRect().toJSON(),title,titleAfter:summary.querySelector('strong').getBoundingClientRect().toJSON(),status:status.toJSON(),button:button.toJSON(),color:c.color,primary:color,weight:c.fontWeight,whiteSpace:getComputedStyle(text).whiteSpace,overflow:getComputedStyle(text).textOverflow,tooltip:text.dataset.tooltip}})()`);
 assert(Math.abs(r.status.top-r.button.top)<=1,'P4 same row '+JSON.stringify({theme,width,zoom,r}));assert(r.status.right<=r.button.left+1,'P4 left of refresh');assert.equal(r.before.width,r.after.width,'P4 title width');assert.equal(r.title.x,r.titleAfter.x,'P4 title position');assert.equal(r.title.y,r.titleAfter.y);assert.equal(r.color,r.primary);assert.equal(r.weight,'600');assert.equal(r.whiteSpace,'nowrap');assert.equal(r.overflow,'ellipsis');assert.equal(r.tooltip,'官方战绩服务暂时中断，部分玩家战绩无法读取，恢复后点「刷新对局」');results.push({kind:'P4',theme,width,zoom,...r});await p.shot('live-outage-'+width+'-'+zoom);
 }
 }
 }
 assert.deepEqual(errors,[]);
}).catch(e=>{console.error(e);process.exitCode=1});
