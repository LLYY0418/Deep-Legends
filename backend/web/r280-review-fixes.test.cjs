'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const {JSDOM} = require('../../desktop/node_modules/jsdom');
const {extract} = require('./r188-harness.cjs');
const source = fs.readFileSync(__dirname + '/gameplay.js', 'utf8');

function functions(names, dependencies) {
  if(names.includes('bindOverviewContent'))names=[...names,'syncRecentHeadingTypography'];
  if (source.includes('function cancelOverviewProgress(')) names = [...names, 'cancelOverviewProgress'];
  return Function(...Object.keys(dependencies), names.map(name => extract(source, name)).join('\n') + `\nreturn {${names.join(',')}}`)(...Object.values(dependencies));
}

function progressFixture() {
  const dom = new JSDOM('<main><section class="summoner-strip">B</section><div class="overview-layout"><div class="match-list"></div><div class="match-filterbar"></div></div></main>');
  const root = dom.window.document.querySelector('main');
  const a = {key:'A', overlay:true, data:{}, overviewRequestToken:1};
  const b = {key:'B', overlay:true, data:{}, overviewRequestToken:1};
  const state = {overlay:[a,b], tabs:[], destroyed:false};
  root._overviewViewTab = b;
  const frames = new Map(), canceled = [], mutations = [];
  let next = 0;
  const noop = () => {};
  const deps = {
    document:dom.window.document, state, nodes:{playerOverlay:{hidden:false},playerOverlayContent:root},
    overviewContainer:() => root,
    renderSummonerStrip:(_data,tab) => `<section class="summoner-strip">${tab.key}</section>`,
    updateCareerCards:tab => mutations.push(tab.key), reconcileFilteredMatchList:noop,
    syncMatchFilterPending:noop, reportOverviewCardReady:noop, reportDeferredMatchesCard:noop,
    bindOverviewContent:noop, prepareImages:noop, rerenderTab:tab => mutations.push('render:' + tab.key),
    requestAnimationFrame:fn => {frames.set(++next,fn);return next;},
    cancelAnimationFrame:id => {canceled.push(id);frames.delete(id);},
  };
  return {dom,root,a,b,state,frames,canceled,mutations,deps,...functions(['patchOverviewProgress','scheduleOverviewProgress'],deps)};
}

test('R280 review: late parent overlay progress never writes the child view', () => {
  const f = progressFixture();
  f.patchOverviewProgress(f.a);
  assert.equal(f.root.querySelector('.summoner-strip').textContent,'B');
  assert.deepEqual(f.mutations,[]);
  f.state.overlay.pop();f.root._overviewViewTab = f.a;
  f.patchOverviewProgress(f.a);
  assert.equal(f.root.querySelector('.summoner-strip').textContent,'A');
  assert.deepEqual(f.mutations,['A']);
  f.dom.window.close();
});

test('R280 review: queued progress rejects stale requests, closed tabs and wrong container owners', () => {
  const f = progressFixture();
  f.scheduleOverviewProgress(f.b);f.scheduleOverviewProgress(f.b);
  assert.equal(f.frames.size,1,'same-frame notifications merge');
  f.b.overviewRequestToken++;
  [...f.frames.values()][0]();
  assert.deepEqual(f.mutations,[],'old request frame must not render newer data');
  f.b.closed = true;f.patchOverviewProgress(f.b);
  assert.deepEqual(f.mutations,[]);
  f.b.closed = false;f.root._overviewViewTab = f.a;f.patchOverviewProgress(f.b);
  assert.deepEqual(f.mutations,[],'matching overlay alone is insufficient');
  f.dom.window.close();
});

test('R280 review: switching a cached overview cancels the previous queued frame', () => {
  const f = progressFixture();
  f.a.progressFrame = 99;f.root._overviewViewTab = f.a;
  const fn = functions(['activateOverviewView'],{
    ...f.deps,overviewRenderCacheFields:() => [],resetOverviewRenderCache:() => {},disposeMatchRowHeight:() => {},
  }).activateOverviewView;
  fn(f.root,f.b);
  assert.deepEqual(f.canceled,[99]);assert.equal(f.a.progressFrame,0);
  assert.equal(f.root._overviewViewTab,f.b);
  f.dom.window.close();
});

test('R280 review: changed career share uses the complete overview and binds once', () => {
  const dom = new JSDOM('<main><section class="summoner-strip">Player A</section><div class="overview-layout"><aside class="career-column"><section data-career-key="activity">old</section></aside><div class="match-list"><article>match A</article></div></div></main>');
  const root = dom.window.document.querySelector('main'),tab = {data:{}};
  const captures = [],noop = () => {};
  const deps = {document:dom.window.document,state:{settings:{matchCount:10}},
    overviewContainer:() => root,careerSectionEntries:() => [['activity','<section data-career-key="activity"><button data-generate-overview-share>share</button></section>']],
    generateOverviewShare:container => captures.push(container),
    requestAnimationFrame:noop,setTimeout:noop,matchTierScrollRoot:() => null,
  };
  for (const name of ['bindScoreDetails','fitMatchTags','hydrateVisibleMatchTags','bindSummonerCopy','bindPlayerLinks','bindOverviewSubpageLinks','bindMatchFilterControls','bindRankHistoryControls','bindRankedQueueControls','bindMayhemRatingControls','bindMatchSentinel','bindMatchDetailControls','applyRenderedMetricStyles','prepareImages']) deps[name] = noop;
  const f = functions(['updateCareerCards','bindOverviewContent','bindOverviewShareControls'],deps);
  f.updateCareerCards(tab,'stream');
  const card = root.querySelector('[data-career-key="activity"]');
  card.querySelector('button').click();
  assert.equal(captures[0],root,'partial binding must retain the full capture owner');
  f.bindOverviewShareControls(card,root,tab);
  card.querySelector('button').click();
  assert.equal(captures.length,2,'rebinding the same button must not duplicate capture');
  assert.equal(root.querySelector('.match-list').textContent,'match A');
  dom.window.close();
});

test('R280 review: loot categories and friend empty states retain v0.12.85 inline paths', () => {
  const app = fs.readFileSync(__dirname+'/app.js','utf8');
  const friends = fs.readFileSync(__dirname+'/friends.js','utf8');
  const dom = new JSDOM('<main></main>'),list = dom.window.document.querySelector('main');
  const sprite = new JSDOM(fs.readFileSync(__dirname+'/icons.svg','utf8'),{contentType:'image/svg+xml'});
  const categories = ['材料','宝箱','英雄','皮肤','小小英雄','永恒星碑','表情','守卫','图标'];
  const categoryIcon = Function(extract(app,'lootCategoryIcon')+';return lootCategoryIcon')();
  const renderEmpty = Function('el',extract(friends,'renderEmpty')+';return renderEmpty')({list});
  const check = () => {
    for (const svg of list.querySelectorAll('svg')) {
      assert(svg.querySelector('path,circle'));
      assert.equal(svg.querySelector('use'),null);
    }
  };
  list.innerHTML = categories.map(categoryIcon).join('');assert.equal(list.querySelectorAll('svg').length,9);check();
  for (const state of ['disconnected','search','empty']) {renderEmpty(state,'fixture',false);check();}
  dom.window.close();sprite.window.close();
});
