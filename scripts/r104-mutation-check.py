#!/usr/bin/env python3
"""R104 behavioral mutations in temporary Go overlays / JS source overrides."""
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'docs/r104-validation/mutations'
OUT.mkdir(parents=True, exist_ok=True)
ENV = dict(os.environ, GOCACHE=str(ROOT / '.gocache'), GOTMPDIR='/private/tmp')
matrix = []

def probe(name, file, before, after, test, node=None, envkey=None):
    source = (ROOT / file).read_text()
    assert source.count(before) == 1, (name, source.count(before))
    command = (['node', '--test', '--test-name-pattern=^' + test + '$', node] if node else
               ['go', 'test', '.', '-count=1', '-run', '^' + test + '$', '-timeout=30s'])
    baseline = subprocess.run(command, cwd=ROOT, env=ENV, text=True, capture_output=True, timeout=90)
    (OUT / (name + '-baseline.txt')).write_text(baseline.stdout + baseline.stderr)
    assert baseline.returncode == 0, (name, 'baseline failed')
    with tempfile.TemporaryDirectory(prefix='r104-mutation-') as temp:
        changed = Path(temp) / Path(file).name
        changed.write_text(source.replace(before, after))
        cmd, env = list(command), dict(ENV)
        if node:
            env[envkey] = str(changed)
        else:
            overlay = Path(temp) / 'overlay.json'
            overlay.write_text(json.dumps({'Replace': {str(ROOT / file): str(changed)}}))
            cmd[2:2] = ['-overlay=' + str(overlay)]
        mutant = subprocess.run(cmd, cwd=ROOT, env=env, text=True, capture_output=True, timeout=90)
    log = mutant.stdout + mutant.stderr
    (OUT / (name + '.txt')).write_text(log)
    killed = mutant.returncode != 0 and ('--- FAIL: ' + test in log or 'AssertionError' in log)
    assert not any(bad in log for bad in ['build failed', 'SyntaxError', 'panic:', 'timed out']), (name, log)
    matrix.append({'mutation': name, 'test': test, 'baseline': 'pass', 'killed': killed})
    (OUT / 'matrix.json').write_text(json.dumps(matrix, ensure_ascii=False, indent=2) + '\n')
    print(name, 'KILLED' if killed else 'SURVIVED', flush=True)
    assert killed, name

cold = 'TestR104ColdStartupDripProtectsForegroundTwentyMatches'
probe('background-priority-removed', 'pro_refresh.go',
      'if len(p.limitQueue) > 0 || len(p.shortWindow) >= 15 || len(p.longWindow) >= 30 {', 'if false {', cold)
probe('drip-restored-to-full-batch', 'pro_refresh.go',
      'a.refreshNextProSeed(ctx, now())', '_ = now(); a.loadProSeeds(withRiotBackground(ctx), nil)', cold)
probe('startup-activity-restored', 'pro_players.go',
      'ladder.apply(completed)\n\t\t\tc.mu.Lock()',
      'ladder.apply(completed)\n\t\t\ta.enrichProActivity(loadCtx, completed, previous)\n\t\t\tc.mu.Lock()', cold)
probe('exact-hit-removed', 'pro_seed_accounts.go',
      'if fromDirectory, ok := known[proLadderAccountKey(ref.GameName, ref.TagLine)]; ok {',
      'if fromDirectory, ok := known[proLadderAccountKey(ref.GameName, ref.TagLine)]; ok && false {',
      'TestR104ExactDirectoryMatchAvoidsRiotAndMissFallsBack')
probe('cancel-poisons-url', 'web/image-queue.js', 'finish(false, true)', 'finish(true)',
      'R104 image cancellation during repeated rerender is not URL failure poisoning', 'web/r104.test.cjs', 'R104_QUEUE_SOURCE')
probe('cooldown-does-not-pump', 'web/image-queue.js',
      'if ((failed.get(url) || 0) <= Date.now()) failed.delete(url);\n      pump();',
      'if ((failed.get(url) || 0) <= Date.now()) failed.delete(url);',
      'R104 real image error retries automatically after cooldown', 'web/r104.test.cjs', 'R104_QUEUE_SOURCE')
probe('quota-hides-skeleton', 'web/gameplay.js', "quotaMessage + '<div class=\"gameplay-skeleton\">",
      "quotaMessage || '<div class=\"gameplay-skeleton\">",
      'R104 quota recovery keeps the first-load skeleton and existing matches visible',
      'desktop/overview-render.test.cjs', 'R104_GAMEPLAY_SOURCE')
probe('background-uses-full-long-window', 'pro_refresh.go', 'len(p.longWindow) >= 30', 'len(p.longWindow) >= 90',
      'TestR104BackgroundAdmissionYieldsAtThirtyOrForegroundQueue')
probe('startup-warmup-removed', 'pro_refresh.go',
      '\tfor {\n\t\tif wait(ctx, proSeedRefreshInterval) != nil {',
      '\ta.refreshNextProSeed(ctx, now())\n\tfor {\n\t\tif wait(ctx, proSeedRefreshInterval) != nil {', cold)
probe('rank-ttl-restored-six-hours', 'pro_seed_accounts.go', 'return 24 * time.Hour', 'return 6 * time.Hour',
      'TestR104SeedRank24HourCacheSeparateFromOverview')
probe('activity-ttl-restored-six-hours', 'pro_activity.go', '"proseed-lastmatch:v1:"+puuid, 7*24*time.Hour',
      '"proseed-lastmatch:v1:"+puuid, 6*time.Hour', 'TestR102LastMatchStartCachedSevenDays')
probe('unbounded-total-context', 'pro_seed_accounts.go', 'context.WithTimeout(ctx, 20*time.Second)',
      'context.WithTimeout(ctx, 200*time.Second)', 'TestR104SeedTotalBudgetIsBoundedRegardlessOfAccountCount')
