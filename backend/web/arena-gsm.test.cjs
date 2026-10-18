'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const {compile} = require('./r188-harness.cjs');
const source = fs.readFileSync(`${__dirname}/gameplay.js`, 'utf8');
test('Arena identity shortage keeps bounded retries independently of squad inference', () => {
  const state = {section:'live', settings:{liveRefresh:true, liveInterval:3}, beacon:{phase:'InProgress'}, liveRetryAttempts:0};
  const timers=[], calls=[];
  const h=compile(source,['liveClientPositionsPending','liveSnapshotComplete','liveAutoRefreshStopped','scheduleLiveRefresh','syncLiveRetryBudget'], {
    state, document:{hidden:false}, connected:()=>true,
    liveAugmentRecommendationSource:()=> 'arena', liveRefreshDelayMs:()=>3000,
    clearTimeout:()=>{}, setTimeout:(fn,delay)=>{timers.push({fn,delay});return timers.length;}, loadLive:(...args)=>calls.push(args)
  });
  for (const queueId of [1700,1710,1750]) {
    const count=queueId===1750?18:16;
    state.live={available:true,phase:'InProgress',gameId:9034935539,queueId,gameMode:'CHERRY',arenaGroupingUnavailable:true,arenaGroupingRetryable:false,
      players:Array.from({length:count-1},(_,i)=>({playerRef:`player_${i}`,historyState:'ok'}))};
    assert.equal(h.liveSnapshotComplete(state.live),false);
    state.live.players=Array.from({length:count},(_,i)=>({playerRef:`player_${i}`,historyState:'ok'}));
    assert.equal(h.liveSnapshotComplete(state.live),true);
    state.live.arenaGroupingRetryable=true;
    assert.equal(h.liveSnapshotComplete(state.live),false);
    state.live.arenaGroupingRetryable=false;
    state.live.players[count-1].identityUnresolved=true;
    assert.equal(h.liveSnapshotComplete(state.live),false);
    state.live.players[count-1]={gameName:'T1ˇFixture',tagLine:'18602',historyState:'unavailable'};
    assert.equal(h.liveSnapshotComplete(state.live),false,'a visible name without a queryable identity must retry');
  }
  state.live.players=state.live.players.slice(0,14);
  h.scheduleLiveRefresh();
  assert.equal(timers.length,1);
  timers[0].fn();
  assert.deepEqual(calls,[[false,'interval']]);
  state.liveRetryAttempts=8;
  h.scheduleLiveRefresh();
  assert.equal(timers.length,1,'retry budget remains bounded');
  assert.match(h.liveAutoRefreshStopped(state.live),/数据尚未完整/);
});
