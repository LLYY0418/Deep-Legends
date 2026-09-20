"""Executable R98 mutations; temporary overlays never replace production files."""
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / 'docs/r98-validation/mutations'
OUT.mkdir(parents=True, exist_ok=True)
ENV = dict(os.environ)
ENV.setdefault('GOCACHE', '/tmp/deep-legends-go-cache')
ENV.setdefault('GOTMPDIR', '/tmp/deep-legends-go-tmp')
Path(ENV['GOTMPDIR']).mkdir(parents=True, exist_ok=True)
results = []


def probe(name, file, old, new, test='', browser=False):
    source = (ROOT / file).read_text()
    assert source.count(old) == 1, (name, 'source anchor count', source.count(old))
    command = ['node', 'desktop/r98-browser.cjs'] if browser else ['go', 'test', '-count=1', '-v', '-timeout=60s', '-run', test, '.']
    environment = ENV.copy()
    if browser:
        environment['R98_BROWSER_OUTPUT'] = str(OUT / (name + '-baseline-browser'))
    clean = subprocess.run(command, cwd=ROOT, env=environment, capture_output=True, text=True, timeout=120)
    baseline = clean.stdout + clean.stderr
    (OUT / (name + '-baseline.txt')).write_text(baseline)
    assert clean.returncode == 0 and ('R98 Chromium PASS' in baseline if browser else '=== RUN   TestR98' in baseline), (name, 'baseline failed', baseline[-2000:])
    with tempfile.TemporaryDirectory(prefix='r98-mutation-') as directory:
        modified = Path(directory) / Path(file).name
        modified.write_text(source.replace(old, new))
        if browser:
            environment['R98_GAMEPLAY_SOURCE'] = str(modified)
            environment['R98_BROWSER_OUTPUT'] = str(OUT / (name + '-browser'))
        else:
            overlay = Path(directory) / 'overlay.json'
            overlay.write_text(json.dumps({'Replace': {str(ROOT / file): str(modified)}}))
            command = command[:2] + ['-overlay', str(overlay)] + command[2:]
        result = subprocess.run(command, cwd=ROOT, env=environment, capture_output=True, text=True, timeout=120)
    log = result.stdout + result.stderr
    (OUT / (name + '.txt')).write_text(log)
    killed = (result.returncode != 0 and ('AssertionError' in log or 'FAIL: TestR98' in log)
              and 'build failed' not in log and 'test timed out' not in log)
    results.append({'id': name, 'file': file, 'test': test or 'real Chromium refresh/rollback', 'killed': killed, 'baseline_exit': clean.returncode, 'exit': result.returncode})
    (OUT / 'mutations.json').write_text(json.dumps(results, indent=2) + '\n')
    print(name, 'KILLED' if killed else 'SURVIVED/INVALID', flush=True)
    assert killed, (name, log[-2000:])


probe('matchIDs-cache-removed', 'riot_api.go', 'time.Minute, &ids,', '0, &ids,', '^TestR98OverviewEndpointCachesAndKeys$')
probe('ranks-cache-removed', 'riot_api.go', '3*time.Minute, &entries,', '0, &entries,', '^TestR98OverviewEndpointCachesAndKeys$')
probe('mastery-cache-removed', 'riot_api.go', '30*time.Minute, &entries,', '0, &entries,', '^TestR98OverviewEndpointCachesAndKeys$')
probe('overview-queue-escape-removed', 'riot_api.go', 'ctx = withRiotQueueLimit(ctx, 5*time.Second)', '// mutation: no overview queue budget', '^TestR98OverviewQueueBudgetReturnsRateLimit$')
probe('fifo-gate-removed', 'riot_api.go', 'release, err := p.enterRiotLimitQueue(ctx)', 'release, err := func() {}, error(nil)', '^TestR98LimiterFIFOHasOnlyOneSleepingHeadAndCanceledWaitersUseNoQuota$')
probe('one-shot-preview', 'riot_api.go', 'loaded > previewLoaded && (loaded >= 5 && (previewLoaded == 0 || loaded-previewLoaded >= 2) || loaded == len(ids))', 'previewLoaded == 0 && loaded >= 5', '^TestR98SummonerDoesNotGateProgressAndPreviewKeepsAdvancing$')
probe('preview-prepend-restored', 'web/gameplay.js',
      '? (previous?.matches || []).map(match => updates.get(String(match.gameId)) || match)',
      '? [...partial.matches, ...(previous?.matches || []).filter(match => !incoming.has(String(match.gameId)))]', browser=True)
probe('refresh-rollback-removed', 'web/gameplay.js',
      'if (!append && sawPreview && refreshSnapshot && tab.data) {',
      'if (false && !append && sawPreview && refreshSnapshot && tab.data) {', browser=True)
