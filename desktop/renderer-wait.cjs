"use strict";
const assert = require("node:assert/strict");

// Wait for the asserted UI, then allow already queued frame work to finish.
// Negative observations and debounce/polling windows keep their existing waits.
async function waitForRender(w, predicate, label = "render did not become ready", timeoutMs = 1500) {
  const {timingRetry}=require('../scripts/lib/timing-retry.cjs');
  await timingRetry({label,within:sample=>sample.ready,measure:async()=>{
    const started=Date.now(),deadline=started+timeoutMs;
    for (;;) {
      if (await predicate()) {
        await new Promise(resolve => w.requestAnimationFrame(() => w.requestAnimationFrame(resolve)));
        if (await predicate()) return {ready:true,ms:Date.now()-started};
      }
      if(Date.now()>=deadline)return {ready:false,ms:Date.now()-started};
      await new Promise(resolve => setTimeout(resolve, 10));
    }
  }});
}
async function waitForOverview(w, matchCount = 17) {
  if(matchCount>20){
    await waitForRender(w,()=>w.document.querySelectorAll(".match-list .match-entry").length>=20,"initial twenty matches did not become ready");
    while(w.document.querySelectorAll(".match-list .match-entry").length<matchCount){
      const before=w.document.querySelectorAll(".match-list .match-entry").length;
      const more=w.document.querySelector("[data-load-more]");assert.ok(more,"real show-more control must exist");more.click();
      await waitForRender(w,()=>w.document.querySelectorAll(".match-list .match-entry").length>before,"show more did not reveal the next window");
    }
  }
  return waitForRender(w, () => {
    const root = w.document.getElementById("overview-content");
    return root?.querySelector(".summoner-strip") && root.querySelector(".career-column")
      && !root.querySelector(".gameplay-skeleton")
      && root.querySelectorAll(".match-list .match-entry").length === matchCount;
  }, `overview with ${matchCount} matches did not become ready`);
}
function waitForWatch(w) {
  return waitForRender(w, () => w.document.querySelector("#suite-watch-root [data-watch-master]")
    && w.document.querySelector("#suite-watch-root .watch-rule"), "watch controls did not become ready");
}
function waitForFacade(w) {
  return waitForRender(w, () => w.document.querySelector("#suite-facade-root [data-facade-hero]")?.options.length > 20
    && w.document.querySelector("#suite-facade-root .facade-signature"), "facade controls did not become ready");
}
module.exports = { waitForRender, waitForOverview, waitForWatch, waitForFacade };
