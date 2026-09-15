#!/usr/bin/env python3
"""R94 semantic regression checks on temporary copies; production is never edited."""
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / 'docs/r94'
OUT.mkdir(parents=True, exist_ok=True)
CASES = [
    ('ignore-gameid-invalidation', 'web/gameplay.js', 'const invalidated = gameChanged || phaseBoundary;', 'const invalidated = phaseBoundary;', 'R94 same-phase live responses', 'R94_GAMEPLAY_SOURCE'),
    ('phase-only-event-trigger', 'web/gameplay.js', 'if (phaseChanged || gameChanged) {', 'if (phaseChanged) {', 'R94 identity polling', 'R94_GAMEPLAY_SOURCE'),
    ('omit-poll-gameid', 'web/gameplay.js', 'handleGameplayPhase(String(payload?.phase || ""), "poll", false, payload?.gameId);', 'handleGameplayPhase(String(payload?.phase || ""), "poll", false);', 'R94 identity polling', 'R94_GAMEPLAY_SOURCE'),
    ('omit-forced-refresh', 'web/gameplay.js', 'loadLive(Boolean(state.liveGameRefreshQueued), source)', 'loadLive(false, source)', 'R94 identity polling', 'R94_GAMEPLAY_SOURCE'),
    ('accept-old-game-response', 'web/gameplay.js', 'if (state.liveAwaitingGame && normalizeLiveGameId(state.liveExpectedGameId)', 'if (false && normalizeLiveGameId(state.liveExpectedGameId)', 'R94 SSE game identity', 'R94_GAMEPLAY_SOURCE'),
    ('accept-stale-poll', 'web/gameplay.js', 'if (observedPhase !== state.beacon.phase || observedGeneration !== state.liveGameGeneration || observedId !== state.liveExpectedGameId)', 'if (false)', 'R94 a delayed poll', 'R94_GAMEPLAY_SOURCE'),
    ('omit-live-event-gameid', 'web/gameplay.js', 'Boolean(event.detail?.changed), event.detail?.gameId', 'Boolean(event.detail?.changed)', 'R94 hidden same-phase', 'R94_GAMEPLAY_SOURCE'),
    ('drop-identical-diagnostics', 'web/runtime.js', 'gameflowQueue.push(observation);', 'if (!gameflowQueue.some(row => JSON.stringify(row) === JSON.stringify(observation))) gameflowQueue.push(observation);', 'R94 diagnostics preserve every', 'R94_RUNTIME_SOURCE'),
    ('omit-identity-probe', 'gameplay_identity.go', 'result.GameID = session.GameData.GameID', 'result.GameID = 0', '^TestR94PhaseEndpointFinds', None),
    ('omit-sse-connection-wire', 'connection_manager.go', 'a.broadcastGameplayIdentity(event)', '', '^TestR94ConnectedSessionForwards', None),
    ('omit-diagnostic-gameid', 'gameplay_identity.go', '"game_id": observation.GameID', '"game_id": int64(0)', '^TestR94DiagnosticBatchPersists', None),
]
results = []
with tempfile.TemporaryDirectory(prefix='r94-mutants-') as temporary:
    for name, file, before, after, pattern, js_env in CASES:
        source = (ROOT / file).read_text()
        if source.count(before) != 1:
            raise RuntimeError(f'{name}: anchor count {source.count(before)}')
        mutant = Path(temporary) / Path(file).name
        mutant.write_text(source.replace(before, after, 1))
        env = dict(os.environ, GOCACHE=str(ROOT / '.gocache'), GOPATH=str(ROOT / '.gopath'))
        if js_env:
            env[js_env] = str(mutant)
            command = ['node', '--test', '--test-name-pattern', pattern, 'web/r94.test.cjs']
        else:
            overlay = Path(temporary) / 'overlay.json'
            overlay.write_text(json.dumps({'Replace': {str(ROOT / file): str(mutant)}}))
            command = ['go', 'test', '-overlay', str(overlay), '-count=1', '-timeout=30s', '-run', pattern, '.']
        result = subprocess.run(command, cwd=ROOT, env=env, capture_output=True, text=True, timeout=90)
        output = result.stdout + result.stderr
        killed = result.returncode != 0 and ('AssertionError' in output or 'FAIL: TestR94' in output) and 'build failed' not in output and 'test timed out' not in output
        (OUT / f'mutation-{name}.txt').write_text(output)
        results.append({'mutation': name, 'killed': killed, 'test': pattern, 'exitCode': result.returncode})
        (OUT / 'mutations.json').write_text(json.dumps(results, indent=2) + '\n')
        print(name, 'KILLED' if killed else 'SURVIVED/INVALID', flush=True)
        if not killed:
            raise RuntimeError(output[-3000:])
