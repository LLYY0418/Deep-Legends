"use strict";
const test = require("node:test"), assert = require("node:assert/strict");
const { bootDemoApp, settled, installWindowCleanup } = require("./overview-render-helpers.cjs");
installWindowCleanup(test);

test('R86 filtering 200 loaded matches hides entries without rebuilding or refetching', async () => {
 async function check(mutate=false) {
  const matchCount = mutate ? 30 : 200;
  const {window:w,errors}=bootDemoApp({matchCount,gameplaySourceTransform:mutate ? source=>source.replace('list.replaceWith(view.list);','view.list.innerHTML = view.list.innerHTML;list.replaceWith(view.list);') : undefined});
  try {
    await settled();
    const d=w.document, before=[...d.querySelectorAll('.match-list .match-entry')];
    assert.equal(before.length,matchCount);
    let creates=0,requests=0;
    const create=d.createElement.bind(d), fetch=w.fetch;
    d.createElement=(...args)=>{creates++;return create(...args)};
    w.fetch=(url,...args)=>{if(String(url).startsWith('/api/gameplay/overview'))requests++;return fetch(url,...args)};
    d.querySelector('[data-match-filter="arena"]').click();
    await Promise.resolve();
    const retained=before.filter(entry=>!entry.isConnected);
    assert.equal(retained.length,matchCount);
    assert(before.every(entry=>entry.parentNode));
    d.querySelector('[data-match-filter="all"]').click();
    await Promise.resolve();
    assert.deepEqual([...d.querySelectorAll('.match-list .match-entry')],before);
    assert.ok(before.every(entry=>!entry.hidden));
    assert.ok(creates<400,`created ${creates}`);
    assert.equal(requests,0,'complete unfiltered dataset needs no first-page request');
    assert.deepEqual(errors,[]);
  } finally {w.close()}
 }
 await check();await assert.rejects(check(true),{name:"AssertionError"});
});
