/* R265: bounded detail-only retries; loaded lazily for foreign overviews. */
(function(root,factory){
  const api=factory();
  if(typeof module==='object' && module.exports)module.exports=api;
  else {root.deepLegendsHistoryRecovery=api;root.deepLegendsSections.register('history-recovery',()=>{});}
})(typeof window==='undefined'?globalThis:window,()=>{
  'use strict';
  const delays=[5000,15000,45000],sessions=new Set();
  function cancel(tab,abort=true){
    const s=tab?.detailRecovery;if(!s)return;
    clearTimeout(s.timer);s.timer=0;s.generation++;
    if(abort && s.running)s.abort?.();
    s.running=false;sessions.delete(tab);
  }
  function reset(tab){cancel(tab);delete tab.detailRecovery;}
  function cancelAll(){for(const tab of [...sessions])cancel(tab);}
  function schedule(tab,ctx){
    let s=tab.detailRecovery;
    // The last cards frame can complete history while retry() is still
    // rendering. Keep its running marker until finally so that frame and the
    // completion render both preserve the existing body and scroll position.
    if(!ctx.pending()){if(!s?.running)cancel(tab,false);return;}
    if(!ctx.active()){cancel(tab);return;}
    if(!s){s=tab.detailRecovery={round:0,timer:0,running:false,generation:0};}
    s.abort=ctx.abort;
    if(s.timer || s.running || s.round>=delays.length)return;
    const generation=s.generation,delay=Math.max(delays[s.round],Number(ctx.notBefore?.() || 0)-Date.now());
    sessions.add(tab);
    s.timer=setTimeout(async()=>{
      s.timer=0;
      if(generation!==s.generation || !ctx.active()){cancel(tab);return;}
      if(Number(ctx.notBefore?.() || 0)>Date.now()){schedule(tab,ctx);return;}
      s.running=true;s.round++;
      try{await ctx.retry();}finally{
        if(generation!==s.generation)return;
        s.running=false;
        if(ctx.quotaBlocked?.())s.round--;
        ctx.report?.(s.round);
        schedule(tab,ctx);
      }
    },Math.max(0,delay));
  }
  function patch(container,entries){
    const column=container.querySelector('.career-column');if(!column)return;
    const template=container.ownerDocument.createElement('template');
    entries.forEach(([,markup],i)=>{
      const old=column.children[i];
      if(old?.outerHTML===markup)return;
      template.innerHTML=markup;
      if(old)old.replaceWith(template.content.cloneNode(true));else column.append(template.content.cloneNode(true));
    });
  }
  function update(container,tab,sameOverview,ctx){
    if(!(tab.detailRecovery?.running || tab.detailRetryRunning) || !sameOverview || !container.querySelector('.overview-layout'))return false;
    const scroll=container.ownerDocument.getElementById('app-scroll'),top=scroll?.scrollTop;
    patch(container,ctx.entries());
    ctx.reconcile(container.querySelector('.match-list'),tab);
    if(!tab.initialPageError && !tab.data.pagination?.partial && !tab.loading)container.querySelectorAll('.service-outage-compact,[data-complete-overview]').forEach(n=>(n.closest('.notice') || n).remove());
    ctx.prepare(container);ctx.ready(container,tab);
    if(scroll)scroll.scrollTop=top;
    return true;
  }
  function resumeQuota(tab,retry,ctx){
    if(tab.quotaRetry!==retry || tab.closed || ctx.state.destroyed || tab.overviewRequestToken!==retry.requestToken || tab.matchFilter!==retry.filter || ctx.state.settings.matchCount!==retry.count)return;
    if(ctx.riot(tab) && !ctx.active(tab))return;
    tab.quotaRetry=null;
    return ctx.load(tab,!retry.append,retry.append,false,true,0,ctx.riot(tab) && !retry.append).then(loaded=>{
      if(loaded && tab.matchFilter===retry.filter && tab.data?.pagination?.filterFallback && !tab.data.pagination.serverFiltered)void ctx.filter(tab,retry.filter);
    });
  }
  if(typeof document!=='undefined')document.addEventListener('visibilitychange',()=>{if(document.hidden)cancelAll();});
  if(typeof window!=='undefined'){
    window.addEventListener('deep-legends:before-section',cancelAll);
    window.addEventListener('beforeunload',cancelAll);
  }
  return Object.freeze({schedule,cancel,cancelAll,reset,patch,update,resumeQuota,delays});
});
