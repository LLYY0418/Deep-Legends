'use strict';
async function waitInstalledStartup({until}){
 // Replace the original controls-only startup gate, with the same until/deadline.
 // Do not abort Electron's initial loadURL while controls exist but load is pending.
 await until(`(async()=>{if(!(document.readyState==='complete' && window.deepLegendsMatchCards && !!document.querySelector('#setting-default-page + .app-select')))return false;await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));return document.readyState==='complete' && !!document.querySelector('#setting-default-page + .app-select');})()`);
}
async function navigateInstalledDemo({call,until,click,origin,marker=Date.now()}){
 await call('Page.addScriptToEvaluateOnNewDocument',{source:`window.__r265ProbeLoad=${marker};window.EventSource=class{addEventListener(){}close(){}};(() => {
  // Test-helper synchronization only: preserve transport, bytes, and Response.
  const wrap = delegate => function(...args) {
    const result = Reflect.apply(delegate, window, args);
    const input = args[0];
    const url = new URL(typeof input === 'string' ? input : input?.url || input, location.href);
    if (url.origin !== location.origin || url.pathname !== '/api/status') return result;
    return Promise.resolve(result).then(async response => {
      const signal = args[1]?.signal || input?.signal;
      while (!window.deepLegendsMatchCards) {
        if (signal?.aborted) throw signal.reason;
        await new Promise(resolve => setTimeout(resolve, 10));
      }
      if (signal?.aborted) throw signal.reason;
      return response;
    });
  };
  // Runtime and demo-data both assign fetch; every wrapper owns its delegate.
  let current = wrap(window.fetch);
  Object.defineProperty(window, 'fetch', {
    configurable: true, enumerable: true,
    get: () => current,
    set: delegate => { current = wrap(delegate); },
  });
})();
`});
 await call('Page.navigate',{url:origin+'/?demo&section=overview'});
 // Keep the existing readiness gate and its deadline. The published/current
 // gameplay initializer restores the saved default section on its next frame.
 // A controls-only gate can click overview before that restoration wins.
 await until(`(async()=>{if(!(window.__r265ProbeLoad===${marker} && document.readyState==='complete' && window.deepLegendsMatchCards && (!window.deepLegendsMatchCards.live || window.deepLegendsMatchCards.live.state.status?.clientView?.state === 'ready') && !!document.querySelector('#setting-default-page + .app-select')))return false;await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));return window.__r265ProbeLoad===${marker} && document.readyState==='complete' && !!document.querySelector('#setting-default-page + .app-select');})()`);
 await click('#section-overview');
 await until("document.querySelector('.match-list .match-entry')");
}
module.exports={navigateInstalledDemo,waitInstalledStartup};
