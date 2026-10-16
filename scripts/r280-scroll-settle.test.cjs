'use strict';
const test = require('node:test'), assert = require('node:assert/strict');
const { waitForScrollSettlement } = require('./r280-scroll-settle.cjs');
const snapshot = (top, collapsed = true) => ({ top, height: 40000, rows: Array.from({ length: 5 }, () => ({ collapsed, height: 100 })) });
function clock() { let time = 0; return { now: () => time, sleep: async ms => { time += ms; }, intervalMs: 10, idleMs: 30, hardMs: 120 }; }
test('scroll settling waits for virtualization and three unchanged actual frame samples', async () => {
  const samples = [snapshot(0, false), snapshot(0, false), snapshot(100), snapshot(200), snapshot(200), snapshot(200)]; let reads = 0;
  await waitForScrollSettlement(async expression => { assert.match(expression, /requestAnimationFrame/); reads++; return samples.shift(); }, clock());
  assert.equal(reads, 6);
});
test('never virtualizing must explicitly fail instead of silently continuing after a pause', async () => {
  await assert.rejects(waitForScrollSettlement(async () => snapshot(200, false), clock()), /R280 scroll virtualization settle: no observable progress/);
});
test('continuously moving scroll state reaches the hard ceiling and remains a failure', async () => {
  let top = 0; await assert.rejects(waitForScrollSettlement(async () => snapshot(++top), clock()), /R280 scroll virtualization settle: hard limit/);
});
