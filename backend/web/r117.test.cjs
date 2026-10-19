'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { JSDOM } = require('../../desktop/node_modules/jsdom');
const root = __dirname;
const read = name => fs.readFileSync(path.join(root, name), 'utf8');
const flush = () => new Promise(resolve => setImmediate(resolve));

test('R117 image queue exports a five-slot cap and keeps successful URL remounts behind admission', async () => {
  const dom = new JSDOM('<body></body>', { url: 'http://fixture/', runScripts: 'outside-only' });
  const w = dom.window;
  try {
    w.eval(read('image-queue.js'));
    const limit = w.deepLegendsImageQueueLimit;
    assert.equal(limit, 5);
    const images = Array.from({ length: 30 }, (_, index) => {
      const image = w.document.createElement('img');
      image.setAttribute('data-queued-src', `/api/image?fixture=${index}`);
      w.document.body.append(image);
      return image;
    });
    for (let round = 0; round < 20; round++) {
      await flush();
      const inFlight = images.filter(image => image.hasAttribute('src') && !image.dataset.imageReady);
      assert.ok(inFlight.length <= limit, `in-flight images exceeded ${limit}: ${inFlight.length}`);
      for (const image of inFlight) image.dispatchEvent(new w.Event('load'));
      if (images.every(image => image.dataset.imageReady === 'true')) break;
    }
    assert.ok(images.every(image => image.dataset.imageReady === 'true'));
    for (const image of images) {
      image.removeAttribute('src');
      delete image.dataset.imageReady;
      w.deepLegendsQueueImage(image, image.getAttribute('data-queued-src'));
    }
    for (let round = 0; round < 20; round++) {
      await flush();
      const inFlight = images.filter(image => image.hasAttribute('src') && !image.dataset.imageReady);
      assert.ok(inFlight.length <= limit, `remount in-flight images exceeded ${limit}: ${inFlight.length}`);
      for (const image of inFlight) image.dispatchEvent(new w.Event('load'));
      if (images.every(image => image.dataset.imageReady === 'true')) break;
    }
    assert.ok(images.every(image => image.dataset.imageReady === 'true'));
  } finally {
    dom.window.close();
  }
});

test('R117 browser guard reads the production image limit and keeps the twelve-image starvation fixture', () => {
  const browser = fs.readFileSync(path.join(root, '..', '..', 'desktop', 'r100-browser.cjs'), 'utf8');
  const queue = read('image-queue.js');
  const limit = Number(queue.match(/const IMAGE_QUEUE_LIMIT\s*=\s*(\d+)/)?.[1]);
  assert.equal(Number(browser.match(/imageQueueLimit=Number\(queueSource\.match\(\/const IMAGE_QUEUE_LIMIT/)?.[1] || limit), limit);
  assert.match(browser, /for\(let i=0;i<12;i\+\+\)/);
  assert.match(browser, /peakImages<=imageQueueLimit/);
});

test('R117 core frontend contracts remain wired to visible degradation and diagnostics', () => {
  const champions = read('champions.js');
  const gameplay = read('gameplay.js');
  const app = read('app.js');
  const friends = read('friends.js');
  const proPlayers = read('pro-players.js');
  const suite = read('suite.js');
  const index = read('index.html');
  const shared = read('app.css');
  // R128 §2.3 撤销 R117 P0-5：界面上不再出现「统计口径」这类方法论说明，斗魂详情
  // 也不再渲染它。原断言（必须调用 renderMeasurementTechnique）反转为必须不存在。
  assert.doesNotMatch(champions, /renderMeasurementTechnique/);
  assert.doesNotMatch(functionSource(champions, 'renderArenaDetailContent'), /measurementTechnique/);
  // P2-5c：`/A|B|C/` 这种无分组顶级或，任意一个分支命中就整条通过——删掉
  // RequestCancelled 本体也能靠另一个无关字符串蒙混过去。三个独立事实写三条断言。
  const championsApi = functionSource(champions, 'api');
  assert.match(championsApi, /cancelled\.name = "RequestCancelled"/);
  assert.match(championsApi, /if \(error\.name === "RequestCancelled"\) throw error;/);
  assert.match(championsApi, /本地请求超时，请重试/);
  assert.doesNotMatch(championsApi, /联网读取超时/, "本地请求不得再引导用户去怀疑自己的网络");
  // 取消哨兵必须在超时判定之前放行，否则 AbortError 会被抹平成「超时」。
  assert.ok(
    championsApi.indexOf('if (error.name === "RequestCancelled") throw error;') < championsApi.indexOf('本地请求超时，请重试'),
    'RequestCancelled 放行必须排在超时分支之前');
  const overview = functionSource(gameplay, 'renderOverviewBodyContent');
  assert.match(overview, /const careerSignature = \[/);
  assert.match(overview, /sameOverview && container\._careerSignature === careerSignature/);
  assert.match(overview, /container\._careerSignature = careerSignature/);
  assert.match(gameplay, /poll-failed/);
  // P2-3：两个 skipped 埋点必须留在各自的拒绝分支，函数级总数无法发现错配。
  const applyItemSet = functionSource(gameplay, 'applyItemSet');
  const missingSelfMarker = 'if (!self || !build) {';
  const missingPayloadMarker = 'if (!payload.championId || !payload.blocks.length) {';
  const missingSelfStart = applyItemSet.indexOf(missingSelfMarker);
  const missingPayloadStart = applyItemSet.indexOf(missingPayloadMarker);
  assert.ok(missingSelfStart >= 0 && missingSelfStart < missingPayloadStart, '找不到有序的 self/build 与 payload 拒绝分支');
  const missingSelfBranch = blockSource(applyItemSet, missingSelfMarker);
  const missingPayloadBranch = blockSource(applyItemSet, missingPayloadMarker);
  const skippedPattern = /recordItemSetClientDiagnostic\("item_set_apply_request", "skipped"/g;
  const skippedProbe = 'recordItemSetClientDiagnostic("item_set_apply_request", "skipped"';
  assert.equal((missingSelfBranch.match(skippedPattern) || []).length, 1, 'self/build 分支必须恰好有一处 skipped 埋点');
  assert.match(missingSelfBranch, /reason: "missing-self-or-build"/);
  assert.ok(missingSelfBranch.indexOf(skippedProbe) < missingSelfBranch.indexOf('return;'), 'self/build 分支的 skipped 埋点必须先于 return 可达');
  assert.equal((missingPayloadBranch.match(skippedPattern) || []).length, 1, 'payload 分支必须恰好有一处 skipped 埋点');
  assert.match(missingPayloadBranch, /const reason = !payload\.championId \? "missing-champion-id" : "empty-blocks"/);
  assert.match(missingPayloadBranch, /blockCount: payload\.blocks\.length, reason/);
  assert.ok(missingPayloadBranch.indexOf(skippedProbe) < missingPayloadBranch.indexOf('return;'), 'payload 分支的 skipped 埋点必须先于 return 可达');
  assert.match(applyItemSet, /recordItemSetClientDiagnostic\("item_set_apply_request", "submitted"/);
  assert.match(app, /installationLoadPromise[\s\S]*CLIENT_INSTALLATION_TTL/);
  assert.match(app, /(?:typeof document !== "undefined" && )?document\.hidden\) return;[\s\S]*const slices/);
  assert.match(friends, /friends-stale-notice[\s\S]*friends-retry/);
  assert.match(proPlayers, /catch \(error\)[\s\S]*state\.error[\s\S]*reportFlowDiagnostic/);
  assert.match(suite, /ResponseFormatError/);
  // P2-4：这是委托式回归的唯一源码护栏，必须保持两个函数各自拥有重入守卫。
  // suite.test.cjs 另用注入的 applyFacade 桩在行为层证明重入时没有委托调用。
  const reentryGuards = suite.match(/if \(state\.facadeApplying\) \{ toast\("上一个生涯写入尚未完成，请稍候"\); return false; \}/g) || [];
  assert.equal(reentryGuards.length, 2, `两个生涯写入入口必须各自有重入守卫，实际 ${reentryGuards.length}`);
  assert.doesNotMatch(index, /href="\/(champions|pro-players|suite)\.css"/);
  assert.match(index, /href="\/friends\.css"/);
  assert.match(shared, /\.mayhem-ranking-list/);
  assert.match(shared, /\.suite-panel[\s\S]*container-type:\s*inline-size/);
  // P3-7：确认卡与 toast 锚点相同，卡片打开时必须靠实测高度把 toast 抬到卡片上方。
  assert.match(shared, /--suite-confirm-clearance:\s*0px;/);
  assert.match(shared, /body\[data-suite-confirm-open\] \.toast \{ bottom: calc\(18px \+ var\(--suite-confirm-clearance\)\); \}/);
  assert.match(suite, /setProperty\("--suite-confirm-clearance"/);
  assert.match(suite, /delete document\.body\.dataset\.suiteConfirmOpen/);
  assert.match(suite, /if \(event\.key === "Tab"\)/);
});


test('R117 item set skip diagnostics execute in both rejected branches', async () => {
  const gameplay = read('gameplay.js');
  const diagnostics = [];
  const state = { live: { players: [] } };
  let payload = { championId: 0, blocks: [] };
  let apiCalls = 0;
  const applyItemSet = Function(
    'state', 'liveRecommendationsFor', 'buildItemSetPayload', 'showToast', 'recordItemSetClientDiagnostic', 'api',
    `${functionSource(gameplay, 'applyItemSet')}\nreturn applyItemSet;`,
  )(
    state,
    () => ({ build: {} }),
    () => payload,
    () => {},
    (event, outcome, detail) => diagnostics.push({ event, outcome, detail }),
    async () => { apiCalls += 1; return {}; },
  );
  const event = { currentTarget: { disabled: false, textContent: '' } };

  await applyItemSet(event);
  assert.deepEqual(diagnostics, [{
    event: 'item_set_apply_request',
    outcome: 'skipped',
    detail: { reason: 'missing-self-or-build' },
  }], '缺少 self/build 时必须实际执行对应的 skipped 埋点');

  diagnostics.length = 0;
  state.live.players = [{ isCurrent: true }];
  payload = { championId: 0, blocks: [] };
  await applyItemSet(event);
  assert.equal(diagnostics.length, 1, '无效 payload 分支必须实际执行一条 skipped 埋点');
  assert.equal(diagnostics[0].event, 'item_set_apply_request');
  assert.equal(diagnostics[0].outcome, 'skipped');
  assert.equal(diagnostics[0].detail.blockCount, 0);
  assert.equal(diagnostics[0].detail.reason, 'missing-champion-id');
  assert.equal(apiCalls, 0, '两个拒绝分支都不得发出写请求');
});
function verifyR117ChromiumWiring(workflow, runner, catalog) {
  assert.doesNotMatch(JSON.stringify(workflow),/refs\/heads\/codex\/ci-hardening-r276/,'R276 special branch must not bypass release-only acceptance');
  require('../../scripts/lib/ci-contracts.cjs').verifyMutationRouting(workflow);
  const quality = workflow.jobs['quality-web'], browser = workflow.jobs['browser-main'];
  assert.ok(quality && browser, 'quality and browser-main must exist');
  const allowedJobKeys = ['name','needs','runs-on','steps','permissions','environment','container','services','outputs'];
  for (const [name, job] of [['quality',quality],['browser-main',browser]]) {
    assert.deepEqual(Object.keys(job).filter(k=>!allowedJobKeys.includes(k)), [], name+' has an unaudited control-flow field');
    assert.equal(job.needs, undefined, name+' must not depend on a skipped job');
    for (const step of job.steps) {
      assert.equal(step['continue-on-error'], undefined, name+' step must not swallow failure');
      if (step.shell!==undefined)assert.ok(['bash','sh','pwsh','powershell','python'].includes(step.shell), 'only built-in fail-fast shells');
    }
  }
  const guard=browser.steps.find(s=>s.name==='Run isolated offline browser guards');
  const chrome=browser.steps.find(s=>s.name==='Require real Chromium');
  assert.ok(guard && chrome, 'real Chromium prerequisites and runner must execute');
  assert.deepEqual(Object.keys(guard).sort(), ['name','run'], 'guard may not add skip, environment or shell overrides');
  assert.equal(guard.run.trim(), 'node scripts/r276-browser-job.cjs main "$RUNNER_TEMP/r276-browser-main"');
  assert.deepEqual(Object.keys(chrome).sort(), ['name','run','shell']);assert.equal(chrome.shell,'bash');
  const chromeLines=[
    'chrome="$(command -v google-chrome-stable || command -v google-chrome || command -v chromium-browser || command -v chromium || true)"',
    'test -n "$chrome"',
    'echo "CHROME_BIN=$chrome" >> "$GITHUB_ENV"',
  ];
  assert.deepEqual(chrome.run.trim().split('\n').map(s=>s.trim()),chromeLines, 'Chrome discovery must fail when unavailable');
  const triggers=workflow.on??workflow[true];
  for(const name of ['push','pull_request']){
    assert.ok(Object.hasOwn(triggers,name), 'push/PR must run default guards');
    const spec=triggers[name];
    if(spec!=null){assert.equal(typeof spec,'object');for(const filter of ['branches-ignore','paths','paths-ignore','tags'])assert.equal(spec[filter],undefined);
      assert.deepEqual(spec.branches,name==='push'?['**','!codex/diagnostic-r279-*']:undefined,'every existing branch push remains eligible; only the approved diagnostic prefix is separate');
      assert.deepEqual(spec['tags-ignore'],name==='push'?['v*']:undefined,'only audited v-tag exclusion is permitted');}
  }
  const noShell=(node)=>{if(!node||typeof node!=='object')return;for(const [key,value] of Object.entries(node)){assert.notEqual(key,'shell','defaults cannot replace fail-fast shells');noShell(value);}};
  noShell(workflow.defaults);noShell(quality.defaults);noShell(browser.defaults);noShell(guard);
  for(const [label,env] of [['workflow',workflow.env],['quality',quality.env],['browser',browser.env]])for(const key of Object.keys(env??{}))assert.doesNotMatch(key,/_SOURCE$|NODE_OPTIONS|R276_BROWSER_ONLY/,label+' cannot override guarded source or selection');
  for(const [name,job] of [['quality',quality],['browser',browser]])for(const step of job.steps){if(typeof step.run==='string'&&step!==chrome)assert.doesNotMatch(step.run,/GITHUB_ENV|GITHUB_PATH/,name+' cannot inject guard environment');}
  const frontend=quality.steps.find(s=>s.name==='Test frontend and desktop renderers');assert.ok(frontend);assert.equal(frontend.if,'${{ !cancelled() }}');assert.equal(frontend['continue-on-error'],undefined);
  for(const [name,file,key] of [['r100','desktop/r100-browser.cjs','R100_BROWSER_OUTPUT'],['r117','desktop/r117-browser.cjs','R117_BROWSER_OUTPUT']]){
    const rows=catalog.guards.filter(r=>r[0]===name);assert.deepEqual(rows,[[name,file,key]],'each original guard must appear exactly once without source overrides');
    assert.equal(catalog.selectGuards('main').filter(r=>r[0]===name).length,1,'default main shard must select each guard');
    assert.ok(fs.existsSync(path.join(root,'../..',file)));
  }
  assert.match(runner,/for\(const \[name,file,key,extra=\{\}\] of selected\)/);
  assert.match(runner,/await runOffline\(process.execPath,\[file\]/);
  assert.match(runner,/assert\.equal\(result\.code,0,name\+' guard failed'\)/,'the actual child exit code must fail the job');
}
test('R117 real Chromium guards are wired into CI and cannot silently skip',()=>{
  const yaml=require('../../desktop/node_modules/js-yaml'),base=path.join(root,'../..');
  const workflow=yaml.load(fs.readFileSync(process.env.R276_CI_WORKFLOW_SOURCE||path.join(base,'.github/workflows/ci.yml'),'utf8'));
  const runner=fs.readFileSync(path.join(base,'scripts/r276-browser-job.cjs'),'utf8');
  const catalog=require('../../scripts/r276-browser-catalog.cjs');
  verifyR117ChromiumWiring(workflow,runner,catalog);
  for(const mutate of [w=>w.jobs['browser-main'].if='false',w=>w.jobs['browser-main']['continue-on-error']=true,w=>w.jobs['browser-main'].steps.find(s=>s.name==='Run isolated offline browser guards').if='false',w=>w.jobs['browser-main'].steps.find(s=>s.name==='Run isolated offline browser guards').run+=' || true',w=>w.on.push['paths']=['never'],w=>delete w.on.push.branches,w=>w.on.push.branches.push('!codex/release-*'),w=>{w.jobs['mutations-1'].if+=" || github.ref == 'refs/heads/codex/ci-hardening-r276'";}]){
    const bad=structuredClone(workflow);mutate(bad);assert.throws(()=>verifyR117ChromiumWiring(bad,runner,catalog),assert.AssertionError,'unsafe workflow mutation must FAIL');
  }
  assert.throws(()=>verifyR117ChromiumWiring(workflow,runner.replace("assert.equal(result.code,0,name+' guard failed')","void result"),catalog),assert.AssertionError,'swallowed child failure must FAIL');
  assert.throws(()=>verifyR117ChromiumWiring(workflow,runner,{...catalog,guards:catalog.guards.filter(r=>r[0]!=='r117')}),assert.AssertionError,'removed guard must FAIL');
});

test('R117 section loader waits for page styles before module execution', () => {
  const loader = read('section-loader.js');
  assert.match(loader, /await Promise\.all\(\(styles\[name\] \|\| \[\]\)\.map\(loadStyle\)\)/);
  assert.match(loader, /styleReady\.add\(name\)/);
  assert.match(loader, /styleFlights\.delete\(name\)/);
});
function functionSource(source, name) {
  let start = source.indexOf(`function ${name}(`);
  assert.ok(start >= 0, name);
  if (source.slice(start - 6, start) === 'async ') start -= 6;
  const bodyStart = source.indexOf('{', source.indexOf(')', start));
  let depth = 0, quote = '', escaped = false;
  for (let index = bodyStart; index < source.length; index += 1) {
    const char = source[index];
    if (quote) { if (escaped) escaped = false; else if (char === '\\') escaped = true; else if (char === quote) quote = ''; continue; }
    if (char === '"' || char === "'" || char === '`') { quote = char; continue; }
    if (char === '{') depth += 1;
    if (char === '}' && --depth === 0) return source.slice(start, index + 1);
  }
  assert.fail(`unbalanced ${name}`);
}

function blockSource(source, marker) {
  const start = source.indexOf(marker);
  assert.ok(start >= 0, marker);
  const bodyStart = source.indexOf("{", start);
  assert.ok(bodyStart >= 0, marker);
  let depth = 0, quote = "", escaped = false;
  for (let index = bodyStart; index < source.length; index += 1) {
    const char = source[index];
    if (quote) {
      if (escaped) escaped = false;
      else if (char === "\\") escaped = true;
      else if (char === quote) quote = "";
      continue;
    }
    if (char === '"' || char === "'" || char === "`") { quote = char; continue; }
    if (char === "{") depth += 1;
    if (char === "}" && --depth === 0) return source.slice(start, index + 1);
  }
  assert.fail(`unbalanced block ${marker}`);
}

test('R117 app installation lookup is singleflight and hidden SSE slices flush once on resume', async () => {
  const dom = new JSDOM('<body></body>', { url: 'http://fixture/', runScripts: 'outside-only' });
  const app = read('app.js');
  try {
    let installationFetches = 0;
    let launchpadRenders = 0;
    const state = { installationsLoaded: false, installationLoadedAt: 0, installationLoadPromise: null, installationLoadError: '', installations: [], officialLoginMessage: '', status: { connected: false } };
    const load = Function('state', 'api', 'renderLaunchpad', 'CLIENT_INSTALLATION_TTL', `${functionSource(app, 'loadClientInstallations')}\nreturn loadClientInstallations;`)(state, async () => { installationFetches += 1; return { items: [] }; }, () => { launchpadRenders += 1; }, 30_000);
    await Promise.all([load(), load(), load(), load(), load()]);
    await load();
    assert.equal(installationFetches, 1);
    assert.ok(launchpadRenders >= 1);

    let statusFetches = 0;
    const liveState = { liveUpdateSlices: new Set(['status']), destroyed: false, status: null, section: 'overview', favoritesPage: 'collection' };
    Object.defineProperty(dom.window.document, 'hidden', { configurable: true, value: true });
    const flush = Function('state', 'document', 'refreshStatus', 'window', `${functionSource(app, 'flushLiveUpdateSlices')}\nreturn flushLiveUpdateSlices;`)(liveState, dom.window.document, async () => { statusFetches += 1; }, dom.window);
    await flush();
    assert.equal(statusFetches, 0);
    assert.equal(liveState.liveUpdateSlices.size, 1);
    Object.defineProperty(dom.window.document, 'hidden', { configurable: true, value: false });
    await flush();
    assert.equal(statusFetches, 1);
    assert.equal(liveState.liveUpdateSlices.size, 0);
  } finally {
    dom.window.close();
  }
});
