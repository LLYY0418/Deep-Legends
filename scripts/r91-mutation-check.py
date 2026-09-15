#!/usr/bin/env python3
"""Check R91 regression sensitivity using isolated JS copies, never production edits."""
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parent.parent
SOURCE = (ROOT / 'web/gameplay.js').read_text()
OUTPUT = ROOT / 'docs/r91'
OUTPUT.mkdir(parents=True, exist_ok=True)
MUTATIONS = {
    'keep-old-game': [
        ('    const invalidated = gameChanged || phaseBoundary;', '    const invalidated = false;'),
    ],
    'clear-on-same-game-refresh': [
        ('    const invalidated = gameChanged || phaseBoundary;', '    const invalidated = liveGamePhase(nextPhase);'),
    ],
    'wait-for-stalled-request': [
        ('    if (state.liveLoading && !force) return;', '    if (state.liveLoading) return;'),
    ],
    'abort-other-pages': [
        ('    if (state.liveLoading) state.controllers.get("live")?.abort();',
         '    if (state.liveLoading) for (const controller of state.controllers.values()) controller.abort();'),
    ],
    'accept-abandoned-response': [
        ('      if (state.liveRequestToken !== requestToken || !connected()) return;',
         '      if (!connected()) return;'),
        ('      if (state.liveRequestToken !== requestToken) return;', ''),
    ],
    'hide-loading-feedback': [
        ('    const message = data?.phase && liveGamePhase(data.phase) && data.phase !== state.beacon.phase\n      ? "对局阶段已变化，正在同步当前对局…"\n      : state.liveLoading ? "正在刷新…"\n      : liveAutoRefreshStopped(data);',
         '    const message = liveAutoRefreshStopped(data);'),
        ('      nodes.liveRefresh.textContent = state.liveLoading\n        ? "正在刷新…" : "刷新对局";',
         '      nodes.liveRefresh.textContent = "刷新对局";'),
    ],
}
results = []
TESTS = {
    'keep-old-game': 'R91 new game removes old recommendations',
    'clear-on-same-game-refresh': 'R91 same game refresh and active phase progression',
    'wait-for-stalled-request': 'R91 repeated manual refresh',
    'abort-other-pages': 'R91 repeated manual refresh',
    'accept-abandoned-response': 'R91 boundary cancels the old request',
    'hide-loading-feedback': 'R91 repeated manual refresh',
}
with tempfile.TemporaryDirectory(prefix='r91-mutants-') as directory:
    for name, replacements in MUTATIONS.items():
        mutated = SOURCE
        for before, after in replacements:
            if before not in mutated:
                raise RuntimeError(f'{name}: mutation anchor absent')
            mutated = mutated.replace(before, after, 1)
        file = Path(directory) / f'{name}.js'
        file.write_text(mutated)
        env = dict(os.environ, R91_GAMEPLAY_SOURCE=str(file))
        result = subprocess.run(['node', '--test', '--test-name-pattern', TESTS[name], 'web/r91.test.cjs'], cwd=ROOT, env=env,
                                capture_output=True, text=True, timeout=30)
        (OUTPUT / f'mutation-{name}.txt').write_text(result.stdout + result.stderr)
        killed = result.returncode != 0 and 'AssertionError' in result.stdout + result.stderr
        results.append({'mutation': name, 'killed': killed, 'exitCode': result.returncode})
(OUTPUT / 'mutations.json').write_text(json.dumps(results, indent=2) + '\n')
print(json.dumps(results, indent=2))
raise SystemExit(0 if all(result['killed'] for result in results) else 1)
