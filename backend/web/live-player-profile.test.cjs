'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const {JSDOM} = require('../../desktop/node_modules/jsdom');
const {compile, escapeHTML} = require('./r188-harness.cjs');
const source = fs.readFileSync(`${__dirname}/gameplay.js`, 'utf8');

test('a verified display-name-only profile renders its name and keeps the overview link', () => {
  const state = {settings:{maskNames:false}};
  const ui = compile(source, ['renderLivePlayer','liveHistoryStateOf','liveHistorySettled','champSelectEnemyPlaceholder','positionLabel','maskedPlayerName','playerLabel'], {
    state, escapeHTML, liveDisplayedChampionId:p=>p.championId, iconFigure:()=>'<img alt="酒桶">',
    rankTitle:()=>'', renderLivePremadeTag:()=>'', proBadgeAttributes:()=>'',renderProIdentityBadge:()=>'',
    number:String,percent:String,kda:String,
  });
  const player = {playerRef:'player_verified', displayName:'首次遇见的玩家', gameName:'', tagLine:'', hidden:false, identityUnresolved:false, privateHistory:true, championId:79, historyState:'failed', recentGames:[], modeStats:{}};
  for (const maskNames of [false,true]) {
    state.settings.maskNames=maskNames;
    const dom = new JSDOM(ui.renderLivePlayer(player,0));
    try {
      const button = dom.window.document.querySelector('.live-player-name');
      assert.equal(button.textContent,maskNames?'召唤师 1':'首次遇见的玩家');
      assert.equal(button.getAttribute('data-player-ref'),'player_verified');
      assert.equal(button.disabled,false);
      assert.doesNotMatch(dom.window.document.body.textContent,/隐藏身份|隐藏玩家/);
      assert.match(dom.window.document.body.textContent,/隐藏战绩/);
    } finally {dom.window.close();}
  }
});
