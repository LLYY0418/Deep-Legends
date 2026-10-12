'use strict';
async function navigateInstalledDemo({call,until,click,origin,marker=Date.now()}){
 await call('Page.addScriptToEvaluateOnNewDocument',{source:`window.__r265ProbeLoad=${marker};window.EventSource=class{addEventListener(){}close(){}};`});
 await call('Page.navigate',{url:origin+'/?demo&section=overview'});
 // Keep the existing readiness gate and its deadline. The published/current
 // gameplay initializer restores the saved default section on its next frame.
 // A controls-only gate can click overview before that restoration wins.
 await until(`(async()=>{if(!(window.__r265ProbeLoad===${marker} && document.readyState==='complete' && window.deepLegendsMatchCards && (!window.deepLegendsMatchCards.live || window.deepLegendsMatchCards.live.state.status?.connected) && !!document.querySelector('#setting-default-page + .app-select')))return false;await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));return window.__r265ProbeLoad===${marker} && document.readyState==='complete' && !!document.querySelector('#setting-default-page + .app-select');})()`);
 await click('#section-overview');
 await until("document.querySelector('.match-list .match-entry')");
}
module.exports={navigateInstalledDemo};
