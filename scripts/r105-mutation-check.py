#!/usr/bin/env python3
"""R105 behavior mutations: isolated overlays, never alter the working tree."""
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'docs/r105-validation/mutations'
OUT.mkdir(parents=True, exist_ok=True)
matrix = []
ENV = dict(os.environ, GOCACHE=str(ROOT / '.gocache'), GOTMPDIR='/private/tmp')

def probe(name, file, before, after, test, node=False, envkey='R105_SUITE_SOURCE'):
    source = (ROOT / file).read_text()
    assert source.count(before) == 1, (name, source.count(before))
    command = (['node', '--test', '--test-name-pattern=^' + test + '$', 'web/r105.test.cjs'] if node else
               ['go', 'test', '.', '-count=1', '-run', '^' + test + '$', '-timeout=30s'])
    baseline = subprocess.run(command, cwd=ROOT, env=ENV, text=True, capture_output=True, timeout=120)
    (OUT / (name + '-baseline.txt')).write_text(baseline.stdout + baseline.stderr)
    assert baseline.returncode == 0, (name, 'baseline failed')
    with tempfile.TemporaryDirectory(prefix='r105-mutation-') as temp:
        changed = Path(temp) / Path(file).name
        changed.write_text(source.replace(before, after))
        cmd, env = list(command), dict(ENV)
        if node:
            env[envkey] = str(changed)
        else:
            overlay = Path(temp) / 'overlay.json'
            overlay.write_text(json.dumps({'Replace': {str(ROOT / file): str(changed)}}))
            cmd[2:2] = ['-overlay=' + str(overlay)]
        mutant = subprocess.run(cmd, cwd=ROOT, env=env, text=True, capture_output=True, timeout=120)
    log = mutant.stdout + mutant.stderr
    (OUT / (name + '.txt')).write_text(log)
    killed = mutant.returncode != 0 and ('--- FAIL: ' + test in log or 'AssertionError' in log)
    assert not any(bad in log for bad in ['build failed', 'SyntaxError', 'panic:', 'timed out']), (name, log)
    matrix.append({'mutation': name, 'test': test, 'baseline': 'pass', 'killed': killed})
    (OUT / 'matrix.json').write_text(json.dumps(matrix, ensure_ascii=False, indent=2) + '\n')
    print(name, 'KILLED' if killed else 'SURVIVED', flush=True)
    assert killed, name

probe('wei-account-removed','pro_seed_accounts.go', ', {"dyjkbysb", "KR1"}', '', 'TestR105_CriticalAccountFixes')
probe('rookie-new-accounts-removed','pro_seed_accounts.go', '{"벼락식혜", "0070"}, {"EmberKnight", "KR0"}, ', '', 'TestR105_CriticalAccountFixes')
probe('total-reduced-to-52','pro_seed_accounts.go', ', {"Xun", "OOK"}', '', 'TestR105_EmbeddedAccountCounts')
probe('canyon-uppercase-i','pro_seed_accounts.go', '"JUGKlNG"', '"JUGKING"', 'TestR105_SpecialCharacters')
probe('icon-ownership-gate-restored','web/suite.js', 'button.disabled = Boolean(state.facadeApplying);', 'button.disabled = !icon.owned || Boolean(state.facadeApplying);', 'R105 all icons are selectable and a click reaches the write route', True)
probe('icon-put-removed','facade_icons.go', 'http.MethodPut, "/lol-summoner/v1/current-summoner/icon"', 'http.MethodGet, "/lol-summoner/v1/current-summoner/icon"', 'TestR105_IconClickRouteWritesUnownedAndHandlesStatuses')
probe('banner-ownership-gate-restored','web/suite.js', 'class="facade-banner-option" data-banner-id=', 'class="facade-banner-option" ${!item.owned ? "disabled" : ""} data-banner-id=', 'R105 all banners are selectable and a click reaches the write route', True)
probe('banner-data-discarded','facade_banners.go', 'variant[key] = value', 'if key != "data" { variant[key] = value }', 'TestR105_BannerPATCHPreservesEntireSlot')
probe('banner-patch-removed','facade_banners.go', 'http.MethodPatch, "/lol-loadouts/v4/loadouts/"+id', 'http.MethodPut, "/lol-loadouts/v4/loadouts/"+id', 'TestR105_BannerPATCHPreservesEntireSlot')
probe('old-snapshot-bypasses-reviewed-list','pro_players.go', 'result := a.buildReviewedProPlayers(teams)', 'result := a.buildProPlayers(teams, proRoster)', 'TestR105_ReviewedPageSurvivesOldSnapshotAndWrongOwner')
probe('reviewed-dormant-accounts-hidden','web/pro-players.js', 'visibleAccounts.filter(account => !account.dormant || account.reviewed)', 'visibleAccounts.filter(account => !account.dormant)', 'R105 all 53 reviewed accounts render including dormant accounts', True, 'R105_PRO_SOURCE')
probe('banner-opaque-number-rounded','facade_banners.go', 'variant[key] = value', 'var decoded any; json.Unmarshal(value, &decoded); variant[key], _ = json.Marshal(decoded)', 'TestR105_BannerOpaqueNumbersRemainExact')
probe('icon-readback-guard-removed','facade_icons.go', 'if next.ProfileIconID != id {', 'if false {', 'TestR105_SuccessStatusWithoutChangeIsNotSuccess')
