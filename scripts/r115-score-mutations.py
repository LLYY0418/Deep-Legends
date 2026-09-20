#!/usr/bin/env python3
"""Offline scoring guard checks; overlays keep production files unchanged."""
import json
import os
from pathlib import Path
import subprocess
import tempfile

root = Path(__file__).resolve().parent.parent
backend = root / 'backend'
out = root / 'docs/r115-validation/scoring'
out.mkdir(parents=True, exist_ok=True)
env = dict(os.environ, GOCACHE=str(root / '.gocache'))
checks = [
    ('paired-games', 'player_ability.go', 'if counterparts != 1 {', 'if false {'),
    ('zero-death-kda', 'player_ability.go', 'player.kdaTotal / float64(player.games)', 'ratio(player.kills+player.assists, player.deaths)'),
    ('missing-axis', 'player_ability.go', 'metric.Unavailable = true', 'metric.Unavailable = false'),
    ('role-reference', 'web/gameplay.js', 'const roleMode = [400,', 'const roleMode = false && [400,'),
    ('outlier-reference', 'web/gameplay.js', 'const baseline = median(values);', 'const baseline = Math.max(...values);'),
    ('assist-credit', 'web/gameplay.js', '(item.kills + item.assists) / Math.max(1, item.deaths)', '(item.kills + item.assists * 0.8) / Math.max(1, item.deaths)'),
    ('missing-baseline-polygon', 'web/gameplay.js', '${complete ? `<polygon class="ability-radar-baseline"', '${true ? `<polygon class="ability-radar-baseline"'),
]

def run(name, command, variables=env):
    result = subprocess.run(command, cwd=root, env=variables, capture_output=True, text=True, timeout=90)
    output = result.stdout + result.stderr
    (out / (name + '.log')).write_text(output)
    return result.returncode, output

go_test = ['go', 'test', '-run', 'TestR115.*Ability', '-count=1', './backend']
web_test = ['node', '--test', 'backend/web/r115-scoring.test.cjs']
for name, command in [('baseline-go', go_test), ('baseline-web', web_test)]:
    if run(name, command)[0]:
        raise SystemExit(name + ' failed')
results = []
with tempfile.TemporaryDirectory(prefix='r115-score-mutations-') as directory:
    for name, filename, old, new in checks:
        source = (backend / filename).read_text()
        assert old in source, name
        replacement = Path(directory) / Path(filename).name
        replacement.write_text(source.replace(old, new))
        if filename.endswith('.go'):
            overlay = Path(directory) / 'overlay.json'
            overlay.write_text(json.dumps({'Replace': {str(backend / filename): str(replacement)}}))
            command = ['go', 'test', '-overlay', str(overlay), '-vet=off', '-run', 'TestR115.*Ability', '-count=1', './backend']
            code, output = run(name, command)
        else:
            code, output = run(name, web_test, dict(env, R115_SCORING_SOURCE=str(replacement)))
        killed = code != 0 and ('--- FAIL:' in output or 'AssertionError' in output) and '[build failed]' not in output
        results.append({'id': name, 'killed': killed, 'exitCode': code})
        print(name, 'KILLED' if killed else 'SURVIVED/ERROR', flush=True)
(out / 'mutations.json').write_text(json.dumps({'baseline': True, 'mutations': results}, indent=2) + '\n')
if not all(item['killed'] for item in results):
    raise SystemExit(1)
