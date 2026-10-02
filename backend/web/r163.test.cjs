'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const source = fs.readFileSync(path.join(__dirname, 'gameplay.js'), 'utf8');

function extract(name) {
  const start = source.indexOf(`function ${name}(`);
  assert.ok(start >= 0, name);
  const tail = source.slice(start);
  const end = tail.indexOf('\n  }\n');
  assert.ok(end >= 0, name);
  return tail.slice(0, end + 4);
}

test('R163 Hextech ARAM 5+4 roster remains retryable until both teams have five players', () => {
  const complete = Function('liveAugmentRecommendationSource', `${extract('liveClientPositionsPending')}; ${extract('liveSnapshotComplete')}; return liveSnapshotComplete;`)(() => 'hextech-aram');
  const players = Array.from({ length: 10 }, (_, index) => ({ teamId: index < 5 ? 100 : 200, historyState: 'unavailable' }));
  for (const queueId of [2300, 2400, 3270]) {
    assert.equal(complete({ available: true, queueId, players: players.slice(0, 9) }), false);
    assert.equal(complete({ available: true, queueId, players }), true);
  }
  // R194: regular ARAM now shares the verified 5v5 completeness gate.
  assert.equal(complete({ available: true, queueId: 450, players: players.slice(0, 9) }), false);
});

test('R163 partial Hextech ARAM roster retries at five seconds', () => {
  const state = { destroyed: false, settings: { liveRefresh: true, liveInterval: 3 }, beacon: { phase: 'InProgress' }, section: 'live', live: { phase: 'InProgress', queueId: 2400, available: true, players: Array.from({ length: 9 }, (_, index) => ({ teamId: index < 5 ? 100 : 200, historyState: 'unavailable' })) }, liveRetryAttempts: 0, liveRetryStartedAt: Date.now() };
  const delays = [];
  const complete = Function('liveAugmentRecommendationSource', `${extract('liveClientPositionsPending')}; ${extract('liveSnapshotComplete')}; return liveSnapshotComplete;`)(() => 'hextech-aram');
  const schedule = Function('state', 'document', 'connected', 'clearTimeout', 'syncLiveRetryBudget', 'liveSnapshotComplete', 'liveAugmentRecommendationSource', 'liveRefreshDelayMs', 'setTimeout', 'loadLive', `${extract('liveClientPositionsPending')}; ${extract('scheduleLiveRefresh')}; return scheduleLiveRefresh;`)(state, { hidden: false }, () => true, () => {}, () => {}, complete, () => 'hextech-aram', () => 3000, (_, delay) => { delays.push(delay); return 1; }, () => {});
  schedule();
  assert.deepEqual(delays, [5000]);
});
