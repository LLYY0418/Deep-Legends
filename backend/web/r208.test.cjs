'use strict';
const test = require('node:test'), assert = require('node:assert/strict'), fs = require('node:fs'), path = require('node:path'), vm = require('node:vm');
const {JSDOM} = require('../../desktop/node_modules/jsdom');
const source = fs.readFileSync(path.join(__dirname, 'gameplay.js'), 'utf8');
function extract(name) {
  let start = source.indexOf(`function ${name}(`);
  assert.ok(start >= 0, name);
  if (source.slice(start - 6, start) === 'async ') start -= 6;
  return source.slice(start, source.indexOf('\n  }', start) + 4);
}
const payload = n => ({player: {playerRef: 'fixture', region: 'kr'}, matches: Array.from({length: n}, (_, i) => ({gameId: i + 1})), pagination: {begIndex: 0, count: n, hasMore: true}});
const flush = () => new Promise(setImmediate);
function harness(responses) {
  const jobs = new Map(), requests = [], toasts = [];
  let sequence = 0, now = 0;
  const state = {controllers: new Map(), settings: {matchCount: 20}, tabs: [], overlay: []};
  const dom = new JSDOM('<main></main>'), root = dom.window.document.querySelector('main');
  const render = tab => { root.innerHTML = (tab.data?.matches || []).map(m => `<article>${m.gameId}</article>`).join('') + ((tab.initialPageError || tab.error) ? `<div class="notice">${tab.initialPageError || tab.error}</div>` : ''); };
  const noop = () => {};
  const context = {state, Headers, AbortController, TextDecoder, Uint8Array, Date, MAX_BROWSE_MATCHES: 1000, AUTO_PAGE_DELAY_MS: 400, AUTO_PAGE_MAX_BACKOFF_MS: 8000,
    setTimeout: (fn, ms) => {jobs.set(++sequence, {fn, ms, at: now + ms}); return sequence;}, clearTimeout: id => jobs.delete(id),
    fetch: async (url, options) => {requests.push({url, options}); assert.ok(responses.length, 'unexpected request'); return responses.shift();},
    tabReady: () => true, riotTab: () => true, tabGroup: () => 'kr', rerenderTab: render, appendOverviewMatches: render, showLoadingMoreState: noop,
    showToast: text => toasts.push(text), loadOPGGSeasonSummary: noop, loadOverviewCurrentGame: noop, syncOverviewSupplementRefs: noop, rememberTabPlayerRef: noop, playerLabel: () => 'Fixture', renderCapabilitySettings: noop,
    normalizedPagination: (p, beg) => ({...p.pagination, nextBegIndex: beg + p.pagination.count}),
  };
  vm.runInNewContext(require('./r211-harness-support.cjs').expand(source,['api','loadOverview'],context).map(extract).join('\n'), context);
  return {...context, jobs, requests, toasts, root, close: () => dom.window.close(), async advance(ms) {now += ms; for (const [id, job] of [...jobs]) if (job.at <= now) {jobs.delete(id); job.fn();} await flush();}};
}
test('R208 IP stream 429 retains partial matches silently and resumes the same page at 60 seconds', async () => {
  const frames = [{type: 'progress', overview: payload(4)}, {type: 'error', status: 429, error: 'quota', kind: 'rate-limited', retryAfter: 60, cooldownScope: 'ip'}];
  const h = harness([new Response(frames.map(v => JSON.stringify(v)).join('\n') + '\n', {headers: {'Content-Type': 'application/x-ndjson'}}), new Response(JSON.stringify(payload(6)))]);
  try {
    const tab = {key: 'fixture', region: 'kr', riotId: {gameName: 'Fixture', tagLine: 'KR1'}};
    h.state.tabs = [tab];
    assert.equal(await h.loadOverview(tab), false);
    assert.equal(tab.data.matches.length, 4); assert.equal(h.root.querySelectorAll('article').length, 4);
    assert.equal(tab.initialPageError, ''); assert.equal(tab.error, ''); assert.equal(h.root.querySelector('.notice'), null); assert.equal(h.toasts.length, 0);
    assert.ok([...h.jobs.values()].some(job => job.ms === 60000));
    await h.advance(59999); assert.equal(h.requests.length, 1);
    await h.advance(1); assert.equal(h.requests.length, 2);
    assert.equal(JSON.parse(h.requests[1].options.body).begIndex, 0);
    assert.equal(tab.data.matches.length, 6); assert.equal(tab.quotaRetry, null);
    assert.equal(h.toasts.length, 0); assert.equal(h.root.querySelector('.notice'), null);
  } finally {h.close();}
});
test('R208 quota exhausted 503 shows the existing unavailable message without a 429 retry timer', async () => {
  const h = harness([new Response(JSON.stringify({error: '战绩服务暂时不可用', kind: 'quota_exhausted'}), {status: 503})]);
  try {
    const tab = {key: 'fixture', region: 'kr', riotId: {gameName: 'Fixture', tagLine: 'KR1'}};
    h.state.tabs = [tab];
    assert.equal(await h.loadOverview(tab), false);
    assert.equal(tab.error, '战绩服务暂时不可用');
    assert.equal(h.root.querySelector('.notice').textContent, '战绩服务暂时不可用');
    assert.equal(tab.quotaRetry, null); await h.advance(60000); assert.equal(h.requests.length, 1);
  } finally {h.close();}
});
