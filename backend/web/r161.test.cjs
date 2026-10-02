'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const source = fs.readFileSync(path.join(__dirname, 'gameplay.js'), 'utf8');
const styles = fs.readFileSync(path.join(__dirname, 'gameplay.css'), 'utf8');

function extract(name) {
  const start = source.indexOf(`function ${name}(`);
  assert.ok(start >= 0, name);
  const tail = source.slice(start);
  const end = tail.indexOf('\n  }\n');
  assert.ok(end >= 0, name);
  return tail.slice(0, end + 4);
}

test('R161 ranked live roster remains retryable until both teams have five players', () => {
  const complete = Function('liveAugmentRecommendationSource', `${extract('liveClientPositionsPending')}; ${extract('liveSnapshotComplete')}; return liveSnapshotComplete;`)(() => 'classic');
  const players = Array.from({ length: 10 }, (_, index) => ({ teamId: index < 5 ? 100 : 200, historyState: 'ok' }));
  assert.equal(complete({ available: true, queueId: 440, players: players.slice(2) }), false);
  assert.equal(complete({ available: true, queueId: 440, players: [...players.slice(0, 4), ...players.slice(5)] }), false);
  assert.equal(complete({ available: true, queueId: 440, players }), true);
});

test('R161 partial ranked roster retries quickly and position chip has no reserved gap', () => {
  const state = { destroyed: false, settings: { liveRefresh: true, liveInterval: 3 }, beacon: { phase: 'InProgress' }, section: 'live', live: { phase: 'InProgress', queueId: 440, available: true, players: Array.from({ length: 8 }, (_, index) => ({ teamId: index < 3 ? 100 : 200, historyState: 'ok' })) }, liveRetryAttempts: 0, liveRetryStartedAt: Date.now() };
  const delays = [];
  const complete = Function('liveAugmentRecommendationSource', `${extract('liveClientPositionsPending')}; ${extract('liveSnapshotComplete')}; return liveSnapshotComplete;`)(() => 'classic');
  const schedule = Function('state', 'document', 'connected', 'clearTimeout', 'syncLiveRetryBudget', 'liveSnapshotComplete', 'liveAugmentRecommendationSource', 'liveRefreshDelayMs', 'setTimeout', 'loadLive', `${extract('liveClientPositionsPending')}; ${extract('scheduleLiveRefresh')}; return scheduleLiveRefresh;`)(state, { hidden: false }, () => true, () => {}, () => {}, complete, () => 'classic', () => 3000, (_, delay) => { delays.push(delay); return 1; }, () => {});
  schedule();
  assert.deepEqual(delays, [5000]);
  assert.match(styles, /\.live-toolbar \[data-live-status\] \{[^}]*flex: 0 1 auto;/);
  assert.match(styles, /\.live-toolbar > \.text-button \{[^}]*margin-left: 0;/);
});
