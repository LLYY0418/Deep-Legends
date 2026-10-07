#!/usr/bin/env python3
"""R244 targeted regressions. Use temporary source copies; never mutate the shared checkout."""
import json
from pathlib import Path
import subprocess
import os
import tempfile
import hashlib

root = Path(__file__).resolve().parents[1]
out = Path(os.environ.get('R244_MUTATION_OUT', root / 'docs/history/reports/r244'))
out.mkdir(parents=True, exist_ok=True)
mutants = [
 ('restore-30-day-cutoff', 'backend/live_history_freshness.go', 'return recentMatchesForPlayer(matches, playerRef, 10, queueID)', 'window := []gameplayMatch{}; for _, match := range matches { if match.CreatedAt >= now.Add(-30*24*time.Hour).UnixMilli() { window = append(window, match) } }; return recentMatchesForPlayer(window, playerRef, 10, queueID)', ['go','test','./backend','-run','^TestR244OldLiveHistoryAndHeaderShareRows$','-count=1']),
 ('restore-independent-header', 'backend/web/gameplay.js', 'const recordGames = games.length;', 'const recordGames = Number(player.recentRankedRecord?.games) || 0;', ['node','--test','backend/web/r244.test.cjs']),
 ('restore-dual-lcu-sgp', 'backend/gameplay.go', 'if !sgpOK || isCurrent && previousGameID > 0 && !liveHistoryContainsGame(sgp.Matches, previousGameID) {', 'if true {', ['go','test','./backend','-run','^TestR244SGPAuthoritativeShortEmptyAndSelfCatchUp$','-count=1']),
 ('restore-idle-backoff', 'backend/client_launch_timing.go', 'return time.Second\n}', 'return 8*time.Second\n}', ['go','test','./backend','-run','^TestR244ColdConnectionSequenceAndSummonerEvent$','-count=1']),
 ('restore-repeated-sweep', 'backend/cold_client_discovery.go', 'if !s.swept {', 'if true {', ['go','test','./backend','-run','^TestR244LightDiscoveryAndOneSweepPerLaunch$','-count=1']),
 ('restore-serial-ranked', 'backend/gameplay.go', 'started := time.Now()\n\t\t\tvalue := recentRankedSampleSet{', 'started := time.Now()\n <-detailsReady\n\t\t\tvalue := recentRankedSampleSet{', ['go','test','./backend','-run','^TestR244OverviewParallelAndEarlyMatches$','-count=1']),
 ('remove-15s-fallback', 'backend/web/app.js', '}, 15_000);', '}, 150_000);', ['node','--test','backend/web/r244.test.cjs']),
]
results = []
for name, relative, old, new, command in mutants:
 path = root / relative
 original = path.read_bytes()
 source = original.decode()
 if old not in source:
  raise RuntimeError(f'{name}: mutation target missing')
 with tempfile.TemporaryDirectory(prefix='r244-mutant-') as directory:
  mutant = Path(directory) / path.name
  mutant.write_text(source.replace(old,new,1))
  env = os.environ.copy()
  run_command = list(command)
  if command[0] == 'go':
   overlay = Path(directory) / 'overlay.json'
   overlay.write_text(json.dumps({'Replace':{str(path):str(mutant)}}))
   run_command.insert(2,'-overlay='+str(overlay))
  else:
   env['R244_GAMEPLAY_SOURCE' if path.name == 'gameplay.js' else 'R244_APP_SOURCE'] = str(mutant)
  run = subprocess.run(run_command,cwd=root,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=90)
  (out/f'mutation-{name}.log').write_bytes(run.stdout)
  output = run.stdout.decode(errors='replace')
  assertion = ('--- FAIL: TestR244' in output) if command[0]=='go' else ('✖ R244' in output or 'not ok' in output)
  killed = run.returncode != 0 and assertion and '[build failed]' not in output
  unchanged = hashlib.sha256(path.read_bytes()).digest() == hashlib.sha256(original).digest()
  results.append({'mutation':name,'killed':killed,'shared_source_unchanged':unchanged,'returncode':run.returncode,'test_command':command,'isolation':'go-overlay' if command[0]=='go' else 'temporary-source'})
  print(name, 'KILLED' if killed else 'SURVIVED/INVALID', flush=True)
(out/'mutations.json').write_text(json.dumps(results,indent=2)+'\n')
if not all(r['killed'] and r['shared_source_unchanged'] for r in results):
 raise SystemExit(1)
