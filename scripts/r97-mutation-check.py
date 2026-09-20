"""R97 executable regression mutations; production files are never overwritten."""
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / 'docs/r97-validation/mutations'
OUT.mkdir(parents=True, exist_ok=True)
ENV = dict(os.environ)
ENV.setdefault('GOCACHE', '/tmp/deep-legends-go-cache')
ENV.setdefault('GOTMPDIR', '/tmp/deep-legends-go-tmp')
Path(ENV['GOTMPDIR']).mkdir(parents=True, exist_ok=True)
RESULTS = []


def probe(name, file, old, new, test, javascript=False):
    source = (ROOT / file).read_text()
    assert source.count(old) == 1, (name, 'anchor count', source.count(old))
    command = (['node', '--test', '--test-name-pattern', test, 'desktop/pro-players.test.cjs'] if javascript
               else ['go', 'test', '-count=1', '-v', '-timeout=60s', '-run', test, '.'])
    baseline = subprocess.run(command, cwd=ROOT, env=ENV, text=True, capture_output=True, timeout=90)
    baseline_log = baseline.stdout + baseline.stderr
    (OUT / f'{name}-baseline.txt').write_text(baseline_log)
    assert baseline.returncode == 0 and '[no tests to run]' not in baseline_log, (name, 'baseline failed')
    assert ('R97 management' in baseline_log if javascript else '=== RUN   TestR97' in baseline_log), (name, 'no matched tests')
    with tempfile.TemporaryDirectory(prefix='r97-mutation-') as directory:
        changed = Path(directory) / Path(file).name
        changed.write_text(source.replace(old, new))
        environment = ENV.copy()
        if javascript:
            environment['R97_PRO_PLAYERS_SOURCE'] = str(changed)
        else:
            overlay = Path(directory) / 'overlay.json'
            overlay.write_text(json.dumps({'Replace': {str(ROOT / file): str(changed)}}))
            command = command[:2] + ['-overlay', str(overlay)] + command[2:]
        result = subprocess.run(command, cwd=ROOT, env=environment, text=True, capture_output=True, timeout=90)
    output = result.stdout + result.stderr
    (OUT / f'{name}.txt').write_text(output)
    killed = (result.returncode != 0 and ('FAIL: TestR97' in output or 'AssertionError' in output)
              and 'build failed' not in output and 'test timed out' not in output)
    RESULTS.append({'id': name, 'file': file, 'test': test, 'baseline_exit': baseline.returncode, 'exit': result.returncode, 'killed': killed})
    (OUT / 'mutations.json').write_text(json.dumps(RESULTS, indent=2) + '\n')
    print(name, 'KILLED' if killed else 'SURVIVED/INVALID', flush=True)
    assert killed, (name, output[-2000:])


probe('account-region-required', 'pro_players.go',
      'region != "" && !strings.EqualFold(region, "kr")', '!strings.EqualFold(region, "kr")',
      '^TestR97ProAccountOptionalRegionSurvivesKRDirectory/(null|missing|empty)$')
probe('management-expanded-directory', 'pro_players.go',
      'result := a.buildProPlayers(teams, proRoster)', 'result := a.buildProPlayers(teams, proDirectoryRoster(teams, proRoster))',
      '^TestR97ManagementAndBadgesShareReviewedSixTeams$')
probe('badge-expanded-directory', 'pro_identity.go',
      'badge, reviewed := proReviewedMemberBadge(team, m)',
      'badge, reviewed := proMemberBadge(team, m, proSourceTeamCodes(teams)), proDirectoryMemberValid(team, m)',
      '^TestR97ManagementAndBadgesShareReviewedSixTeams$')
probe('frontend-expanded-filters', 'web/pro-players.js',
      'const teams = ["BLG", "IG", "T1", "HLE", "GEN", "DK"];',
      'const teams = ["BLG", "IG", "T1", "HLE", "GEN", "DK", "Winners"];',
      'R97 management', True)
probe('frontend-secondary-leak', 'web/pro-players.js',
      ' || team.secondary === true', '', 'R97 management', True)
