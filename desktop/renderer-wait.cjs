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
  try {
  if(matchCount>20){
    await waitForRender(w,()=>w.document.querySelectorAll(".match-list .match-entry").length>=20,"initial twenty matches did not become ready");
    while(w.document.querySelectorAll(".match-list .match-entry").length<matchCount){
      const before=w.document.querySelectorAll(".match-list .match-entry").length;
      const more=w.document.querySelector("[data-load-more]");assert.ok(more,"real show-more control must exist");more.click();
      // Loaded-window reveals are synchronous. Keep every real button click,
      // but let their observer/frame work settle together at the final window.
      // Network pagination still waits for each actual response before clicking.
      if(w.document.querySelectorAll(".match-list .match-entry").length<=before)
        await waitForRender(w,()=>w.document.querySelectorAll(".match-list .match-entry").length>before,"show more did not reveal the next window");
    }
  }
  return await waitForRender(w, () => {
    const root = w.document.getElementById("overview-content");
    return root?.querySelector(".summoner-strip") && root.querySelector(".career-column")
      && !root.querySelector(".gameplay-skeleton")
      && root.querySelectorAll(".match-list .match-entry").length === matchCount;
  }, `overview with ${matchCount} matches did not become ready`);
  } catch (error) {
    if (error?.code !== 'TIMING_LIMIT') throw error;
    // Preserve explicit content contracts after the unchanged readiness wait.
    // Arbitrary runtime/setup exceptions are never converted to assertion kills.
    const root = w.document.getElementById('overview-content');
    assert.ok(root?.querySelector('.summoner-strip'), 'overview must render the summoner strip');
    assert.ok(root.querySelector('.career-column'), 'overview must render the career column');
    assert.equal(Boolean(root.querySelector('.gameplay-skeleton')), false, 'overview must finish its skeleton');
    assert.equal(root.querySelectorAll('.match-list .match-entry').length, matchCount, 'overview must render the requested match window');
    throw error;
  }
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
