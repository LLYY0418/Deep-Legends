"""Executable R102 mutations; temporary overlays, never mutate the checkout."""
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / 'docs/r102-validation/mutations'
OUT.mkdir(parents=True, exist_ok=True)
ENV = dict(os.environ, GOCACHE='/tmp/deep-legends-go-cache')
START = os.environ.get('R102_MUTATION_FROM', '')
RESULTS = json.loads((OUT / 'matrix.json').read_text()) if START and (OUT / 'matrix.json').exists() else []
RESULTS = [r for r in RESULTS if r['id'] < START] if START else RESULTS


def probe(name, file, old, new, test, kind='go', envkey=''):
    if START and name < START:
        return
    source = (ROOT / file).read_text()
    assert source.count(old) == 1, (name, source.count(old))
    cmd = ['/opt/homebrew/bin/go', 'test', '-count=1', '-v', '-timeout=40s', '-run', '^' + test + '$', '.'] if kind == 'go' else ['node', '--test', '--test-name-pattern=^' + test + '$', 'desktop/pro-players.test.cjs']
    clean = subprocess.run(cmd, cwd=ROOT, env=ENV, capture_output=True, text=True, timeout=90)
    (OUT / (name + '-baseline.log')).write_text(clean.stdout + clean.stderr)
    assert clean.returncode == 0, (name, 'baseline', clean.stdout[-2000:], clean.stderr[-2000:])
    env = ENV.copy()
    with tempfile.TemporaryDirectory(prefix='r102-mutation-') as tmp:
        changed = Path(tmp) / Path(file).name
        changed.write_text(source.replace(old, new, 1))
        if kind == 'go':
            overlay = Path(tmp) / 'overlay.json'
            overlay.write_text(json.dumps({'Replace': {str(ROOT / file): str(changed)}}))
            cmd = cmd[:2] + ['-overlay', str(overlay)] + cmd[2:]
        else:
            env[envkey] = str(changed)
        result = subprocess.run(cmd, cwd=ROOT, env=env, capture_output=True, text=True, timeout=90)
    log = result.stdout + result.stderr
    (OUT / (name + '.log')).write_text(log)
    expected = ('FAIL: ' + test) in log if kind == 'go' else ('AssertionError' in log and test in log)
    killed = result.returncode != 0 and expected and not any(s in log for s in ['build failed', 'test timed out', 'SyntaxError', 'panic:'])
    RESULTS.append(dict(id=name, file=file, test=test, baseline_exit=clean.returncode, mutant_exit=result.returncode, killed=killed))
    (OUT / 'matrix.json').write_text(json.dumps(RESULTS, indent=2) + '\n')
    print(name, 'KILLED' if killed else 'INVALID/SURVIVED', flush=True)
    assert killed, log[-2000:]

probe('01-hardcoded-metadata', 'pro_seed_accounts.go', 'RealName: seed.RealName, Position: seed.Position', 'RealName: "Song Eui-jin", Position: "middle"', 'TestR102SeedRealNameAndPositionComeFromRoster')
probe('02-shared-anchor', 'pro_seed_accounts.go', 'strconv.Itoa(index)', 'strconv.Itoa(0)', 'TestR102SeedAnchorKeysAreUniquePerAccount')
probe('03-creation-time', 'pro_activity.go', 'time.UnixMilli(match.Info.GameStartTimestamp)', 'time.UnixMilli(match.Info.GameCreation)', 'TestR102LastMatchStartParsesGameStartTimestamp')
probe('04-empty-detail', 'pro_activity.go', 'if len(ids) == 0 {\n\t\t\treturn nil', 'if len(ids) == 0 {\n _,_,err := p.matchByIDWithCache(ctx, "KR_0"); return err', 'TestR102LastMatchStartHandlesNoMatches')
probe('05-short-activity-ttl', 'pro_activity.go', '"proseed-lastmatch:v1:"+puuid, 7*24*time.Hour', '"proseed-lastmatch:v1:"+puuid, 3*time.Minute', 'TestR102LastMatchStartCachedSevenDays')
probe('06-unbounded-budget', 'pro_seed_accounts.go', 'context.WithTimeout(ctx, 20*time.Second)', 'context.WithTimeout(ctx, 200*time.Second)', 'TestR104SeedTotalBudgetIsBoundedRegardlessOfAccountCount')
probe('07-return-on-account-failure', 'pro_seed_accounts.go', 'account, err := a.riot.resolveProSeed(accountCtx, seed, index)', 'account, err := a.riot.resolveProSeed(accountCtx, seed, index)\n if err != nil { cancel(); cancelSeed(); return rows }', 'TestR102SeedOneAccountFailureDoesNotBlockOthers')
probe('08-drop-old-snapshot', 'pro_seed_accounts.go', 'row := old', 'row := opggProAccount{}', 'TestR102SeedKeepsPerAccountLastSnapshotOnQuotaExhausted')
probe('09-rank-before-time', 'pro_players.go', 'func proAccountLess(a, b proAccount) bool {', 'func proAccountLess(a, b proAccount) bool {\n if a.Tier != b.Tier {return proTierOrder[a.Tier] > proTierOrder[b.Tier]}', 'TestR102SortsByLastMatchTimeDescending')
probe('10-dormant-before-known', 'pro_players.go', 'func proAccountLess(a, b proAccount) bool {', 'func proAccountLess(a, b proAccount) bool {\n if a.Dormant != b.Dormant {return !a.Dormant}', 'TestR102KnownAlwaysBeforeUnknown')
probe('11-legacy-lp-ascending', 'pro_players.go', 'return a.LP > b.LP', 'return a.LP < b.LP', 'TestR102UnknownLastMatchFallsBackToRankOrder')
probe('12-primary-field', 'pro_players.go', 'type proAccount struct {', 'type proAccount struct {\n Primary bool `json:"primary"`', 'TestR102PrimaryFieldRemoved')
probe('13-badge-restored', 'web/pro-players.js', '${account.stale ?', '${account.primary ? \'<small class="pro-main-badge">主号</small>\' : ""}${account.stale ?', 'TestR102NoPrimaryBadgeRendered', 'node', 'R102_PRO_SOURCE')
probe('14-primary-filter', 'web/pro-players.js', 'visibleAccounts.slice(0, 1)', 'visibleAccounts.filter(account => account.primary).slice(0, 1)', 'TestR102LatestOnlyTogglesShowsFirstAccountOnly', 'node', 'R102_PRO_SOURCE')
probe('15-old-button-label', 'web/index.html', '只看最新账号', '只看主账号', 'TestR102ButtonLabelSaysLatestNotPrimary', 'node', 'R102_INDEX_SOURCE')
probe('16-primary-highlight', 'web/pro-players.js', 'accounts.indexOf(account) === 0', 'account.primary', 'TestR102LatestOnlyTogglesShowsFirstAccountOnly', 'node', 'R102_PRO_SOURCE')
probe('17-missing-player', 'pro_seed_accounts.go', '{TeamCode: "BLG", Player: "Bin", Accounts: []proSeedAccountRef{{"빈 스토리", "KR1"}}},', '', 'TestR102AllThirtyThreePlayersHaveSeeds')
probe('18-wrong-wei-owner', 'pro_seed_accounts.go', 'Player: "Wei", Accounts:', 'Player: "Rookie", Accounts:', 'TestR102DyjkbysbBelongsToWei')
probe('19-duplicate-account', 'pro_seed_accounts.go', '{{"빈 스토리", "KR1"}}', '{{"빈 스토리", "KR1"}, {"빈 스토리", "KR1"}}', 'TestR102SeedAccountsMatchVerificationDoc')
probe('20-error-is-unranked', 'pro_seed_accounts.go', 'if err := p.get(ctx, riotPlatformHost, "/lol/league/v4/entries/by-puuid/"+url.PathEscape(puuid), nil, &entries); err != nil {\n\t\t\treturn err', 'if err := p.get(ctx, riotPlatformHost, "/lol/league/v4/entries/by-puuid/"+url.PathEscape(puuid), nil, &entries); err != nil {\n\t\t\treturn nil', 'TestR102FailedQueryDoesNotTriggerBackoff')
probe('21-activity-backs-off', 'pro_activity.go', '"proseed-lastmatch:v1:"+puuid, 7*24*time.Hour', '"proseed-lastmatch:v1:"+puuid, 72*time.Hour', 'TestR102UnrankedBackoffDoesNotAffectLastMatchQuery')
probe('22-observed-rank-keeps-backoff', 'pro_seed_accounts.go', 'now.Add(24 * time.Hour), StaleUntil: now.Add(24 * time.Hour)', 'now.Add(72 * time.Hour), StaleUntil: now.Add(72 * time.Hour)', 'TestR102NewlyRankedAccountResetsToNormalTTL')
probe('23-no-unranked-backoff', 'pro_seed_accounts.go', 'return 72 * time.Hour', 'return 6 * time.Hour', 'TestR102UnrankedAccountBacksOffRankQuery')
probe('24-drop-activity-snapshot', 'pro_snapshot_cache.go', 'a.SeedKey, a.LastMatchAt, a.LastMatchAtKnown = e.SeedKey, e.LastMatchAt, e.LastMatchAtKnown', 'a.SeedKey, a.LastMatchAt, a.LastMatchAtKnown = e.SeedKey, "", false', 'TestR102UpstreamActivityAndSnapshotRemainPrivate')
probe('25-filter-ranked-only', 'pro_activity.go', 'p.matchIDsFiltered(ctx, puuid, 0, 1, 0, "")', 'p.matchIDsFiltered(ctx, puuid, 0, 1, 420, "ranked")', 'TestR102LastMatchStartParsesGameStartTimestamp')

# Preserve player/account/team totals and uniqueness while assigning an ordinary
# account to the wrong player; only the authoritative owner mapping can catch it.
owner_rows = '\n'.join(line for line in (ROOT / 'pro_seed_accounts.go').read_text().splitlines() if 'Player: "TheShy"' in line or 'Player: "Wei"' in line)
swapped_rows = owner_rows.replace('"The shy", "asdf"', '"R102_SWAP", "TEMP"').replace('"Kimman", "zxfkk"', '"The shy", "asdf"').replace('"R102_SWAP", "TEMP"', '"Kimman", "zxfkk"')
probe('26-swap-account-owners', 'pro_seed_accounts.go', owner_rows, swapped_rows, 'TestR102SeedAccountsMatchVerificationDoc')
