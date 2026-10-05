"use strict";
const test = require("node:test"), assert = require("node:assert/strict");
const { bootDemoApp, settled, installWindowCleanup } = require("./overview-render-helpers.cjs");
installWindowCleanup(test);

test('R86 image listeners and tab scroll remain bounded including independent image-listener bypass', async () => {
  async function check(mutate=false) {
    const {window:w,errors}=bootDemoApp({matchCount:200,gameplaySourceTransform:source=>{
      source=source.replace('function renderOverviewBodyContent(container, tab) {',`function renderOverviewBodyContent(container, tab) {
        const oldAdd=window.EventTarget.prototype.addEventListener;
        let calls=0;
        window.EventTarget.prototype.addEventListener=function(...args){calls++;return oldAdd.apply(this,args)};
        try { return r86OriginalRender(container,tab); } finally {window.EventTarget.prototype.addEventListener=oldAdd;window.__r86ListenerMax=Math.max(window.__r86ListenerMax||0,calls);}
      }
      function r86OriginalRender(container, tab) {`);
      if(mutate)source=source.replace('function prepareImages(container) {','function prepareImages(container) { for(const image of container.querySelectorAll("img")){image.addEventListener("load",()=>{});image.addEventListener("error",()=>{});}');
      return source;
    }});
    try {
      await settled();
      assert.equal(w.document.querySelectorAll('.match-list .match-entry').length,200);
      assert.ok(w.__r86ListenerMax<1000,`listeners=${w.__r86ListenerMax}`);
      let styles=0;const original=w.getComputedStyle.bind(w);w.getComputedStyle=(...args)=>{styles++;return original(...args)};
      const tabs=w.document.querySelector('#player-tabs');
      for(let i=0;i<20;i++)tabs.dispatchEvent(new w.Event('scroll'));
      await new Promise(resolve=>w.requestAnimationFrame(()=>resolve()));
      assert.ok(styles<=2,`computed styles=${styles}`);
      assert.deepEqual(errors,[]);
    }finally{w.close()}
  }
  await check();await assert.rejects(check(true),{name:'AssertionError'});
});
