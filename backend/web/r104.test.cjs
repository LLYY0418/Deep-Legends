'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { JSDOM } = require('../../desktop/node_modules/jsdom');
const source = fs.readFileSync(process.env.R104_QUEUE_SOURCE || path.join(__dirname, 'image-queue.js'), 'utf8');
const flush = () => new Promise(setImmediate);
function fixture() {
  const dom = new JSDOM('<main></main>', { url: 'http://localhost', runScripts: 'outside-only' });
  const w = dom.window, maps = [], timers = new Map(), writes = [];
  let now = 100000, next = 0;
  // Observe private maps without adding a production test API.
  w.Map = class extends Map { constructor(...args) { super(...args); maps.push(this); } };
  w.Date.now = () => now;
  w.setTimeout = (fn, delay) => { const id = ++next; timers.set(id, {fn, at: now + delay}); return id; };
  w.clearTimeout = id => timers.delete(id);
  const descriptor = Object.getOwnPropertyDescriptor(w.HTMLImageElement.prototype, 'src');
  Object.defineProperty(w.HTMLImageElement.prototype, 'src', {...descriptor, set(value) { writes.push([this, value]); descriptor.set.call(this, value); }});
  w.eval(source);
  return {w, writes, failed: maps[2], advance(ms) {
    const end = now + ms;
    for (;;) {
      const first = [...timers.entries()].filter(([, timer]) => timer.at <= end).sort((a,b) => a[1].at - b[1].at)[0];
      if (!first) break;
      now = first[1].at; timers.delete(first[0]); first[1].fn();
    }
    now = end;
  }, close() { w.dispatchEvent(new w.Event('deep-legends:dispose')); dom.window.close(); }};
}

test('R104 image cancellation during repeated rerender is not URL failure poisoning', async () => {
  const h = fixture(), {w} = h;
  try {
    const main = w.document.querySelector('main');
    const render = () => main.innerHTML = '<img data-queued-src="/shared.png">'.repeat(3);
    render(); await flush();
    assert.equal(main.querySelectorAll('img[src]').length, 3);
    for (let cycle = 0; cycle < 3; cycle++) {
      render(); await flush(); // replace in-flight images, like streaming progress
      assert.equal(h.failed.has('/shared.png'), false, 'cancellation must never poison URL');
      assert.equal(main.querySelectorAll('img[src]').length, 3);
    }
    let loads = 0;
    main.addEventListener('load', () => loads++, true);
    for (const img of main.querySelectorAll('img')) {
      assert.equal(img.getAttribute('src'), '/shared.png');
      img.dispatchEvent(new w.Event('load'));
    }
    assert.equal(loads, 3);
    assert.equal(h.failed.has('/shared.png'), false);
  } finally { h.close(); }
});

test('R104 real image error retries automatically after cooldown', async () => {
  const h = fixture(), {w} = h;
  try {
    const img = w.document.createElement('img');
    img.dataset.queuedSrc = '/retry.png'; w.document.body.append(img); await flush();
    assert.equal(h.writes.length, 1);
    img.dispatchEvent(new w.Event('error'));
    assert.equal(h.failed.has('/retry.png'), true);
    h.advance(9999);
    assert.equal(h.writes.length, 1, 'no request during cooldown');
    h.advance(1);
    assert.equal(h.writes.length, 2, 'timer must reassign src without DOM mutation');
    assert.equal(h.failed.has('/retry.png'), false);
    img.dispatchEvent(new w.Event('load'));
    h.advance(20000);
    assert.equal(h.writes.length, 2, 'success must finish retry');
  } finally { h.close(); }
});
