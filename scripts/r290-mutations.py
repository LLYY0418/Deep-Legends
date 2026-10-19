#!/usr/bin/env python3
"""R290 behavior mutants; each baseline/mutant/restored run uses actual source."""
from pathlib import Path
import json
import os
import subprocess

root = Path(__file__).resolve().parents[1]
out = Path(os.environ.get('R290_MUTATION_OUT', root / 'output/evidence/r290/mutations'))
out.mkdir(parents=True, exist_ok=True)
env = {**os.environ, 'GOCACHE': os.environ.get('GOCACHE', '/private/tmp/r290-go-cache')}
results = []


def go(pattern):
    return ['go', 'test', './backend', '-count=1', '-run', pattern]


def node(file):
    return ['node', '--test', 'backend/web/' + file]


cases = [
    ('ranked-publication', 'backend/season_stats.go',
     [('a.publishSeasonSnapshot(reference, playerRef, scan.cache)\n\t\t\t\ta.recordRankWindow',
       '/* delayed until mayhem finishes */\n\t\t\t\ta.recordRankWindow')],
     go('^TestR288Backfill|^TestR290RankedPublication'), 'ranked'),
    ('download-rewrite', 'backend/update_sources.go',
     [('AutoDownload: &autoDownload, ', ''), ('autoDownload := !u.autoDownloadDisabled', '')],
     go('^TestR290AutoDownloadDefaultsAndSourceRewrite$'), 'rewrite dropped choices'),
    ('short-arrow', 'backend/web/index.html',
     [('M12 18.5V5.5', 'M12 16.4V9.2'), ('M7 10.5 12 5.5l5 5', 'M8.9 12.3 12 9.2l3.1 3.1')],
     node('r204.test.cjs'), 'arrow height ratio'),
    ('star-fallback', 'backend/web/settings-preferences.js',
     [('.catch(()=>showStars(null))', '.catch(()=>showStars(0))')],
     node('r290-settings.test.cjs'), 'R290 settings stars'),
    ('status-class', 'backend/web/default/index.html',
     [('id="setting-proxy-state" class="inline-status"', 'id="setting-proxy-state" class=""')],
     ['node', 'scripts/r290-settings-browser.cjs'], 'computed status theme color'),
]

for name, file, replacements, command, failure in cases:
    path = root / file
    original = path.read_bytes()
    mutant = original.decode()
    for before, after in replacements:
        if before not in mutant:
            raise RuntimeError('mutation anchor missing: ' + name)
        mutant = mutant.replace(before, after, 1)
    statuses = {}
    try:
        for phase in ['baseline', 'mutant', 'restored']:
            path.write_bytes(mutant.encode() if phase == 'mutant' else original)
            run_env = {**env, 'R280_BROWSER_OUT': str(out / (name + '-' + phase + '-browser'))}
            run = subprocess.run(command, cwd=root, env=run_env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
            (out / (name + '-' + phase + '.log')).write_text(run.stdout)
            statuses[phase] = run.returncode
            if phase == 'mutant' and (failure not in run.stdout or '[build failed]' in run.stdout):
                raise RuntimeError('mutant failed without its behavior assertion: ' + name)
    finally:
        path.write_bytes(original)
    if statuses != {'baseline': 0, 'mutant': 1, 'restored': 0}:
        raise RuntimeError(str((name, statuses)))
    results.append({'mutation': name, **statuses, 'behaviorAssertion': failure})
    (out / 'results.json').write_text(json.dumps(results, indent=2) + '\n')
    print(name, statuses, flush=True)
