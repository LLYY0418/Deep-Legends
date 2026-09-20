// Real keyboard/pointer + geometry guard, served from the actual working tree.
'use strict';
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
exports.verify=async({call,evaluate,output})=>{
 const wait=ms=>new Promise(r=>setTimeout(r,ms));
 const key=async(key,code=key)=>{await call('Input.dispatchKeyEvent',{type:'keyDown',key,code});await call('Input.dispatchKeyEvent',{type:'keyUp',key,code});await wait(65);};
 const state=()=>evaluate(`({open:document.querySelector('#player-group-button').getAttribute('aria-expanded'),group:document.querySelector('#player-group-button').dataset.playerGroup,focus:document.activeElement.id||document.activeElement.dataset.playerGroup,checked:[...document.querySelectorAll('#player-group-menu [aria-checked="true"]')].map(e=>e.dataset.playerGroup),count:document.querySelector('#player-group-button .player-group-count').textContent})`);
 const focusButton=()=>evaluate(`document.querySelector('#player-group-button').focus()`);
 await focusButton();await key('Enter');
 assert.equal((await state()).open,'true','keyboard Enter opens the group menu');
 assert.equal((await state()).focus,'players','keyboard Enter moves focus into menu');
 assert.deepEqual(await evaluate(`[...document.querySelectorAll('#player-group-menu [aria-disabled="true"]')].map(e=>({group:e.dataset.playerGroup,disabled:e.disabled,count:e.querySelector('small').textContent}))`),[{group:'kr',disabled:true,count:'0'},{group:'pro',disabled:true,count:'0'}]);
 await key('ArrowDown');assert.equal((await state()).focus,'players','empty groups skipped');
 await key('Escape');assert.equal((await state()).focus,'player-group-button');
 await key(' ', 'Space');assert.equal((await state()).open,'true');await key('Escape');
 // Exercise native sequential focus, rather than synthesizing a DOM keydown.
 await key('Tab');assert.notEqual((await state()).focus,'player-group-button');
 await call('Input.dispatchKeyEvent',{type:'keyDown',key:'Tab',code:'Tab',modifiers:8});
 await call('Input.dispatchKeyEvent',{type:'keyUp',key:'Tab',code:'Tab',modifiers:8});
 assert.equal((await state()).focus,'player-group-button','Tab can reach the picker');
 const open=async(detail)=>{await evaluate(`window.dispatchEvent(new CustomEvent('deep-legends:open-player',{detail:${JSON.stringify(detail)}}))`);await wait(350);};
 for(let i=1;i<=3;i++)await open({playerRef:'cn-fixture-'+i,gameName:'国内玩家'+i,tagLine:'CN1',region:'cn',serverId:'HN1',source:'search'});
 await open({playerRef:'kr-fixture',gameName:'韩服玩家',tagLine:'KR1',region:'kr',source:'search'});
 await open({playerRef:'pro-fixture',gameName:'职业玩家',tagLine:'KR1',region:'kr',source:'pro-players',teamCode:'T1',playerName:'Fixture'});
 await focusButton();await key('Enter');await key('Home');assert.equal((await state()).focus,'players');
 await key('ArrowDown');assert.equal((await state()).focus,'kr');
 await key('ArrowUp');assert.equal((await state()).focus,'players');
 await key('End');assert.equal((await state()).focus,'pro');
 await key('Home');await key('Enter');
 assert.equal((await state()).group,'players');assert.equal((await state()).count,'4');
 assert.equal((await state()).open,'false');assert.equal((await state()).focus,'player-group-button');
 await key('ArrowRight');assert.equal((await state()).group,'kr');
 await key('ArrowLeft');assert.equal((await state()).group,'players');
 // A real status update rerenders the menu while it owns focus.
 await key('Enter');await key('End');
 await evaluate(`document.querySelector('#overview-refresh').click()`);await wait(500);
 assert.equal((await state()).focus,'pro','async refresh preserves menu item focus');
 assert.equal((await state()).open,'true');await key('Escape');
 for(const theme of ['dark','light','violet'])for(const width of [1440,1100,820,620]){
  await evaluate(`(()=>{const s=document.querySelector('#setting-theme');s.value=${JSON.stringify(theme)};s.dispatchEvent(new Event('change',{bubbles:true}));})()`);
  await call('Emulation.setDeviceMetricsOverride',{width,height:1100,deviceScaleFactor:1,mobile:false});await wait(150);
  await focusButton();await key('Enter');
  const m=await evaluate(`(()=>{const head=document.querySelector('.player-workspace-head');const button=document.querySelector('#player-group-button');const tabs=document.querySelector('#player-tabs');const b=e=>{const r=e.getBoundingClientRect();return {x:r.x,y:r.y,w:r.width,h:r.height,right:r.right,bottom:r.bottom}};
   const selected=document.querySelector('#player-group-menu [aria-checked="true"]');const other=document.querySelector('#player-group-menu [aria-checked="false"]');
   return {picker:b(button),shell:b(document.querySelector('.player-tabs-shell')),head:b(head),refresh:b(document.querySelector('#overview-refresh')),vertical:b(document.querySelector('.summoner-strip')).y-b(head).y,
    clipped:[...head.querySelectorAll('*')].filter(e=>!e.closest('.game-icon')&&e.scrollWidth-e.clientWidth>1&&getComputedStyle(e).overflowX==='hidden').map(e=>e.className),
    overflow:document.documentElement.scrollWidth>innerWidth,
    visible:[...tabs.querySelectorAll('.player-tab-wrap')].filter(e=>b(e).x>=b(tabs).x-1&&b(e).right<=b(tabs).right+1).length,
    arrows:[...head.querySelectorAll('.player-tabs-arrow')].map(e=>e.hidden),
    selected:getComputedStyle(selected).backgroundColor,other:getComputedStyle(other).backgroundColor,
    selectedText:getComputedStyle(selected).color,buttonText:getComputedStyle(button).color,buttonWeight:getComputedStyle(button).fontWeight,
    primary:(()=>{const probe=document.createElement('span');probe.style.color='var(--primary)';head.append(probe);const color=getComputedStyle(probe).color;probe.remove();return color;})(),
    tabCenterOffsets:[...tabs.querySelectorAll('.player-tab')].map(e=>{const copy=e.querySelector('.player-tab-copy');return copy?Math.abs(b(copy).y+b(copy).h/2-(b(e).y+b(e).h/2)):0;}),
    checked:[...document.querySelectorAll('#player-group-menu [aria-checked="true"]')].map(e=>e.dataset.playerGroup),
    menu:b(document.querySelector('#player-group-menu')),buttonHasArrow:Boolean(button.querySelector('svg')),buttonTooltip:button.hasAttribute('data-tooltip'),
    dots:[...tabs.querySelectorAll('.player-tab-group-dot')].map(e=>getComputedStyle(e).backgroundColor),dot:getComputedStyle(button.querySelector('.player-group-dot')).backgroundColor};})()`);
  assert.ok(m.picker.w<=110,'compact picker <=110px');assert.ok(m.menu.w<=134,'compact group menu <=134px');assert.equal(m.buttonHasArrow,false,'group button has no arrow icon');assert.equal(m.buttonTooltip,false,'group button has no hover tooltip');assert.deepEqual(m.clipped,[],'header hidden clipping audit');assert.equal(m.overflow,false,'no document overflow');
  assert.deepEqual(m.checked,['players']);assert.notEqual(m.selected,m.other,'aria-checked selection must have visible highlight');assert.equal(m.dots.length,0,'player tabs have no dots');assert.equal(m.selectedText,m.primary,'selected option follows theme');assert.equal(m.buttonText,m.primary,'selected group follows theme');assert.ok(Number(m.buttonWeight)>=700,'group selector is bold');assert.ok(m.tabCenterOffsets.every(n=>n<=1),'player tab text vertically centered');
  if(width>620)assert.ok(m.vertical<=76,'workspace head to summoner strip <=76px (R76 option A)');
  else {assert.ok(Math.abs(m.picker.y-(m.shell.y+(m.shell.h-m.picker.h)/2))<=1,'mobile picker and tabs same row');assert.ok(m.refresh.y>=m.shell.bottom,'mobile refresh on next row');assert.ok(Math.abs(m.refresh.right-m.head.right)<=1,'mobile refresh aligned right');}
  if(width===1440){assert.equal(m.visible,4,'four full player tabs');assert.ok(m.arrows.every(Boolean),'unneeded scroll arrows hidden');}
  console.log('R74 groups',theme,width,JSON.stringify(m));
  const shot=await call('Page.captureScreenshot',{format:'png'});fs.writeFileSync(path.join(output,'groups-'+theme+'-'+width+'.png'),Buffer.from(shot.data,'base64'));
  await key('Escape');
 }
 // Hover and pointer/focus dismissal are tested on production timers.
 await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1100,deviceScaleFactor:1,mobile:false});await wait(100);
 const point=await evaluate(`(()=>{const r=document.querySelector('#player-group-button').getBoundingClientRect();return {x:r.x+10,y:r.y+10};})()`);
 await call('Input.dispatchMouseEvent',{type:'mouseMoved',x:point.x,y:point.y});await wait(60);assert.equal((await state()).open,'false','hover does not open immediately');await wait(100);assert.equal((await state()).open,'true');
 await call('Input.dispatchMouseEvent',{type:'mouseMoved',x:1,y:1});await wait(100);assert.equal((await state()).open,'true','leave delay preserves menu');await wait(160);assert.equal((await state()).open,'false');
 await focusButton();await key('Enter');
 await call('Input.dispatchMouseEvent',{type:'mousePressed',button:'left',clickCount:1,x:1,y:1});await call('Input.dispatchMouseEvent',{type:'mouseReleased',button:'left',clickCount:1,x:1,y:1});assert.equal((await state()).open,'false','outside pointer closes');
 await focusButton();await key('Enter');await evaluate(`document.querySelector('#overview-refresh').focus()`);await wait(250);assert.equal((await state()).open,'false','focus leaving closes');
 console.log('Production player group geometry + real keyboard/pointer PASS');
};
if(require.main===module){const r=require('node:child_process').spawnSync(process.execPath,[path.join(__dirname,'current-game-layout.cjs')],{stdio:'inherit',env:{...process.env,R74_LAYOUT_SUITE:'groups'}});process.exitCode=r.status??1;}
