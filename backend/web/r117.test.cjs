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
  assert.match(gameplay, /careerSectionsSignature[\s\S]*container\._careerSignature/);
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
test('R117 real Chromium guards are wired into CI and cannot silently skip', () => {
  const ciPath = path.join(root, '..', '..', '.github', 'workflows', 'ci.yml');
  const ci = fs.readFileSync(ciPath, 'utf8');
  // P1-2 的根因是「脚本存在但从不在 CI 执行」。只做子串匹配的话，给这一步加一行
  // continue-on-error: true 就能把失败吞掉而测试照样绿，所以这里解析 YAML 按结构断言。
  const yaml = require('../../desktop/node_modules/js-yaml');
  const workflow = yaml.load(ci);
  const quality = workflow.jobs.quality;
  const steps = quality.steps;
  const guard = steps.find((step) => step?.name === 'Real Chromium image-queue and lazy-CSS guards');
  assert.equal(quality['continue-on-error'], undefined, 'job 级 continue-on-error 会让整个 job 的失败被吞掉');
  assert.equal(quality.if, undefined, 'job 级 if 会让整个 job 在默认 push/PR 流程里被跳过');
  assert.ok(guard, 'CI quality job 里找不到真 Chromium 护栏步骤');
  assert.equal(guard['continue-on-error'], undefined, 'continue-on-error 会吞掉浏览器护栏的失败');
  assert.equal(guard.if, undefined, '条件执行会让护栏在默认路径上被整步跳过');
  for (const step of steps) {
    assert.notEqual(step['continue-on-error'], true, `步骤「${step.name}」用 continue-on-error 吞掉了失败`);
  }
  const run = String(guard.run);
  const expectedLines = [
    'chrome="$(command -v google-chrome-stable || command -v google-chrome || command -v chromium-browser || command -v chromium || true)"',
    'test -n "$chrome" || { echo "no Chrome/Chromium binary on the runner; browser guards must not silently skip" >&2; exit 1; }',
    'echo "using $chrome"',
    'CHROME_BIN="$chrome" R100_BROWSER_OUTPUT="$RUNNER_TEMP/r100-browser" node desktop/r100-browser.cjs',
    'CHROME_BIN="$chrome" R117_BROWSER_OUTPUT="$RUNNER_TEMP/r117-browser" node desktop/r117-browser.cjs',
  ];
  const runLines = run.split('\n').map((line) => line.trim()).filter(Boolean);
  assert.deepEqual(runLines, expectedLines, 'CI 浏览器护栏 run 块只能包含审计过的精确命令');
  const unapprovedLines = runLines.filter((line) => !expectedLines.slice(0, 2).includes(line));
  assert.doesNotMatch(unapprovedLines.join('\n'), /\|\||&&|\bif\b|(?:^|[;\n])\s*:\s*(?:#.*)?(?:$|[;\n])|set \+e|2>\/dev\/null/, 'run 块不得加入 shell 级失败软化');
  for (const [name, expected] of [
    ['r100-browser.cjs', expectedLines[3]],
    ['r117-browser.cjs', expectedLines[4]],
  ]) {
    assert.ok(fs.existsSync(path.join(root, '..', '..', 'desktop', name)), name);
    const commandLines = runLines.filter((line) => line.includes(`node desktop/${name}`));
    assert.equal(commandLines.length, 1, `${name} 必须且只能在 CI 执行一次`);
    assert.equal(commandLines[0], expected, `${name} 命令不得附带会吞掉失败的后缀`);
  }
  assert.match(run, /browser guards must not silently skip/);
  // ---- P1-2（R120）：从黑名单改成「护栏处于默认执行路径」的结构不变量 ----
  // 原写法只列已知的坏字段与精确命令行，换一种同等效果但语法不同的形状就绕过去了
  // （例如给步骤加 `shell: bash {0}` 去掉默认的 -e、或把 on.push 的分支过滤设成永不
  // 匹配）。下面每一条都是白名单/结构断言：新形状要么落在允许清单外，要么直接缺字段。
  const triggers = workflow.on ?? workflow[true]; // js-yaml 3 会把 `on` 解析成布尔 true
  assert.ok(triggers && typeof triggers === 'object', 'workflow 缺少 on: 触发器');
  for (const trigger of ['push', 'pull_request']) {
    assert.ok(Object.prototype.hasOwnProperty.call(triggers, trigger),
      `workflow.on 必须包含 ${trigger}：只有 workflow_dispatch 时 push/PR 根本不触发护栏`);
    const spec = triggers[trigger];
    if (spec !== null && spec !== undefined) {
      assert.equal(typeof spec, 'object', `workflow.on.${trigger} 只能是 null 或映射，不能是字符串/数组过滤`);
      for (const filter of ['branches', 'branches-ignore', 'paths', 'paths-ignore', 'tags', 'tags-ignore']) {
        assert.equal(spec[filter], undefined,
          `workflow.on.${trigger}.${filter} 会让护栏在默认 push/PR 路径上整轮不触发`);
      }
    }
  }
  // `shell: bash {0}` 与 GitHub 默认的 `bash -e {0}` 只差一个 -e，去掉它之后 r100 失败、
  // r117 通过，步骤照样绿。defaults 可以作用到整个 workflow 或整个 job，所以递归检查。
  const assertNoShellKey = (label, node) => {
    if (node === null || typeof node !== 'object') return;
    for (const [key, value] of Object.entries(node)) {
      assert.notEqual(key, 'shell',
        `${label} 不得出现 shell 字段：自定义 shell 模板可以去掉默认的 -e，让护栏失败被吞掉`);
      assertNoShellKey(`${label}.${key}`, value);
    }
  };
  assertNoShellKey('workflow.defaults', workflow.defaults);
  assertNoShellKey('jobs.quality.defaults', quality.defaults);
  assertNoShellKey('护栏步骤', guard);
  for (const step of steps) {
    if (step && step.shell !== undefined) {
      assert.ok(['bash', 'sh', 'pwsh', 'powershell', 'python'].includes(step.shell),
        `步骤「${step.name}」的 shell 只允许 GitHub 内建写法（Linux 上自带 -e），实际是 ${JSON.stringify(step.shell)}`);
    }
  }
  // job 级字段用显式允许清单，而不是逐个禁止 if / continue-on-error / strategy …：
  // 任何没被审计过的控制流字段都视为可疑。env 刻意不在清单里——它能把护栏脚本读的
  // 源码换成桩文件（见下面的 *_SOURCE 断言）。
  const allowedJobKeys = ['name', 'needs', 'runs-on', 'steps', 'permissions', 'environment', 'container', 'services', 'outputs'];
  const unexpectedJobKeys = Object.keys(quality).filter((key) => !allowedJobKeys.includes(key));
  assert.deepEqual(unexpectedJobKeys, [],
    `jobs.quality 出现了未审计的字段：${unexpectedJobKeys.join(', ')}（控制流字段可以让护栏不执行或失败被吞）`);
  const allowedGuardKeys = ['name', 'run', 'env'];
  const unexpectedGuardKeys = Object.keys(guard).filter((key) => !allowedGuardKeys.includes(key));
  assert.deepEqual(unexpectedGuardKeys, [],
    `护栏步骤只允许 ${allowedGuardKeys.join(' / ')}，实际多出：${unexpectedGuardKeys.join(', ')}`);
  const allowedGuardEnv = ['CHROME_BIN', 'R100_BROWSER_OUTPUT', 'R117_BROWSER_OUTPUT'];
  if (guard.env !== undefined) {
    assert.equal(typeof guard.env, 'object', 'guard.env 必须是映射');
    const unexpectedGuardEnv = Object.keys(guard.env).filter((key) => !allowedGuardEnv.includes(key));
    assert.deepEqual(unexpectedGuardEnv, [],
      `护栏步骤的 env 只允许 ${allowedGuardEnv.join(' / ')}，实际多出：${unexpectedGuardEnv.join(', ')}`);
  }
  // desktop/r100-browser.cjs 读 R100_APP_SOURCE / R100_QUEUE_SOURCE，指向桩文件就等于
  // 护栏在测假的源码。三个层级都不允许出现 *_SOURCE 覆盖变量。
  for (const [label, env] of [['workflow.env', workflow.env], ['jobs.quality.env', quality.env], ['护栏步骤 env', guard.env]]) {
    for (const key of Object.keys(env ?? {})) {
      assert.doesNotMatch(key, /_SOURCE$/, `${label} 不得定义 ${key}：它会让护栏读取被替换掉的源码`);
    }
  }
  // ---- P3-1（R120 复测）可选加固，三条全部采纳 ----
  // 1) needs 指向一个 if:false 的 job 会让 quality 整个被跳过（needs 在允许清单里，
  //    所以必须单独钉死）；2) 任何前置步骤往 $GITHUB_ENV / $GITHUB_PATH 写一行，就能给
  //    护栏注入 R100_APP_SOURCE / NODE_OPTIONS 之类变量，绕开上面只查 env 映射的断言；
  //    3) 给运行本测试的前端步骤加 if:false，护栏自己就永远不执行。
  assert.equal(quality.needs, undefined,
    'jobs.quality 不得依赖其它 job：needs 指向一个 if:false 的 job 就能让护栏整体被跳过');
  const frontendStep = steps.find((step) => step?.name === 'Test frontend and desktop renderers');
  assert.ok(frontendStep, 'quality job 里找不到运行前端测试（含本护栏测试）的步骤');
  assert.equal(frontendStep.if, undefined, '前端测试步骤加条件会让本护栏测试自身不再执行');
  assert.equal(frontendStep['continue-on-error'], undefined, '前端测试步骤的失败不得被吞掉');
  for (const step of steps) {
    if (typeof step?.run === 'string') {
      assert.doesNotMatch(step.run, /GITHUB_ENV|GITHUB_PATH/,
        `步骤「${step.name}」不得写 GITHUB_ENV/GITHUB_PATH：前置步骤可借此给护栏注入 R100_APP_SOURCE / NODE_OPTIONS 等环境变量`);
    }
  }
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
