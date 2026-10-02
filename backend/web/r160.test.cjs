'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { JSDOM } = require('../../desktop/node_modules/jsdom');

const queueSource = fs.readFileSync(path.join(__dirname, 'image-queue.js'), 'utf8');
const gameplaySource = fs.readFileSync(path.join(__dirname, 'gameplay.js'), 'utf8');

test('R160 bundled art uses local image queue capacity beside stalled remote art', () => {
  const remote = '/api/champion-asset?source=communitydragon&path=%2Flatest%2Fgame%2Fassets%2Fux%2Fcherry%2Faugments%2Ficons%2Fa_large.png';
  const builtin = '/api/champion-asset?source=builtin&path=%2Faugments%2F110.png';
  const dom = new JSDOM(`<img id="r1" data-queued-src="${remote}"><img id="r2" data-queued-src="${remote}"><img id="local" data-queued-src="${builtin}">`, { url: 'http://localhost/', runScripts: 'outside-only' });
  try {
    dom.window.eval(queueSource);
    assert.equal(dom.window.document.getElementById('local').getAttribute('src'), builtin);
    assert.equal(dom.window.deepLegendsImageSourceLabel(builtin), 'builtin');
  } finally {
    dom.window.dispatchEvent(new dom.window.Event('deep-legends:dispose'));
    dom.window.close();
  }
});

test('R160 gameplay icon proxy accepts only the explicit bundled scheme', () => {
  const start = gameplaySource.indexOf('function proxyAsset(path) {');
  const end = gameplaySource.indexOf('\n  function assetIcon(', start);
  assert.ok(start > 0 && end > start);
  const proxyAsset = Function(`${gameplaySource.slice(start, end)}; return proxyAsset;`)();
  assert.equal(proxyAsset('builtin:/augments/2095.png'), '/api/champion-asset?source=builtin&path=%2Faugments%2F2095.png');
  assert.equal(proxyAsset('hexdata:/assets/augments/icons/16.18/highroller_small.png'), '/api/champion-asset?source=hexdata&path=%2Fassets%2Faugments%2Ficons%2F16.18%2Fhighroller_small.png');
});

test('R160 overview augments paint from the fast catalog before perks arrive', () => {
  const start = gameplaySource.indexOf('function augmentMetadata(id) {');
  const end = gameplaySource.indexOf('\n  function augmentIconFigure(', start);
  assert.ok(start > 0 && end > start);
  const state = {
    perks: null,
    augmentCatalog: new Map([[110, { id: 110, iconPath: 'builtin:/augments/110.png' }]]),
  };
  const augmentMetadata = Function('state', `${gameplaySource.slice(start, end)}; return augmentMetadata;`)(state);
  assert.equal(augmentMetadata(110).iconPath, 'builtin:/augments/110.png');
  state.perks = { augments: [{ id: 110, iconPath: 'builtin:/augments/110.png', name: '晾衣绳' }] };
  assert.equal(augmentMetadata(110).name, '晾衣绳');
  assert.equal(augmentMetadata(999999), undefined);
});
