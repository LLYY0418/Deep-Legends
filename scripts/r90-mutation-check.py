"""Isolated regression mutations; never rewrite production workspace files."""
import json, os, pathlib, subprocess, tempfile
root = pathlib.Path(__file__).resolve().parent.parent
backend = root / 'backend'
out = pathlib.Path(os.environ.get('R90_MUTATION_OUTPUT', root / 'docs/r90'))
out.mkdir(parents=True, exist_ok=True)
env = dict(os.environ)
env.setdefault('GOCACHE', '/tmp/deep-legends-go-cache')
env.setdefault('GOTMPDIR', '/tmp/deep-legends-go-tmp')
pathlib.Path(env['GOTMPDIR']).mkdir(parents=True, exist_ok=True)
baselines = set()
results = []
def probe(name, file, old, new, test, javascript=False):
    source = (backend / file).read_text()
    assert source.count(old) == 1, (name, source.count(old))
    baseline = ['node', '--test', '--test-name-pattern', test, 'backend/web/r90.test.cjs'] if javascript else ['go', 'test', '-count=1', '-timeout=60s', '-run', test, './backend']
    if tuple(baseline) not in baselines:
        clean = subprocess.run(baseline, cwd=root, env=env, capture_output=True, text=True, timeout=90)
        output = clean.stdout + clean.stderr
        (out / (name + '-baseline.txt')).write_text(output)
        assert clean.returncode == 0 and '[no tests to run]' not in output, (name, 'baseline failed', output[-1500:])
        baselines.add(tuple(baseline))
    with tempfile.TemporaryDirectory(prefix='r90-mutant-') as tmp:
        tmp = pathlib.Path(tmp)
        modified = tmp / pathlib.Path(file).name
        modified.write_text(source.replace(old, new))
        runenv = env.copy()
        if name == "queue-literal": runenv["R90_QUEUE_GUARD_SOURCE"] = str(modified)
        if javascript:
            runenv['R90_GAMEPLAY_SOURCE'] = str(modified)
            cmd = ['node', '--test', '--test-name-pattern', test, 'backend/web/r90.test.cjs']
        else:
            overlay = tmp / 'overlay.json'
            overlay.write_text(json.dumps({'Replace': {str(backend/file): str(modified)}}))
            cmd = ['go', 'test', '-overlay', str(overlay), '-count=1', '-timeout=60s', '-run', test, './backend']
        result = subprocess.run(cmd, cwd=root, env=runenv, capture_output=True, text=True, timeout=90)
        output = result.stdout + result.stderr
        killed = result.returncode != 0 and ('FAIL: Test' in output or 'AssertionError' in output) and 'build failed' not in output and 'test timed out' not in output
        (out / (name + '.txt')).write_text(output)
        results.append({'id': name, 'killed': killed, 'test': test, 'file': file, 'anchor_count': source.count(old), 'exit_code': result.returncode})
        (out/'mutations.json').write_text(json.dumps(results, indent=2)+'\n')
        print(name, 'KILLED' if killed else 'SURVIVED/INVALID', flush=True)
        assert killed, (name, output[-1500:])
# R95 removed order inference: mutate real-field candidate validation instead of dead order helpers.
probe('wrong-block-size','arena_live_grouping.go','validArenaGroupAssignments(groups, squadSize) {','validArenaGroupAssignments(groups, 2) {','^TestR96ExplicitSubteamGuard/.*/conflict=false$')
probe('removed-ally-check','arena_live_grouping.go','verified, rejected := arenaAlliesCorroborate(groups, response.Players, remembered, squadSize, grouping, current.PUUID)','verified, rejected := true, false','^TestR96ExplicitSubteamGuard/.*/conflict=true$')
probe('raw-identity-only','arena_live_grouping.go','names, riotIDs := livePlayerIdentityValues(raw, player)','names, riotIDs := livePlayerIdentityValues(raw, gameplayLivePlayer{})','^TestR96ExplicitSubteamGuard/.*/conflict=false$')
# Inject the forbidden positional grouping into the live explicit-field path.
probe('session-instead-of-playerlist','arena_live_grouping.go','groups[i] = liveClientGroupingForPlayer(grouping, player, p)','groups[i] = strconv.Itoa(i/squadSize + 1); _ = player; _ = p','^TestR96ExplicitSubteamGuard/.*/conflict=false$')
probe('guessed-mascots','arena_live_grouping.go','return grouped && !order && arenaLiveClientMascotMapping(grouping)','return grouped','^TestR90OrderInferenceNeverEnablesMascotMapping$')
probe('removed-order-mascot-guard','arena_live_grouping.go','return grouped && !order && arenaLiveClientMascotMapping(grouping)','return grouped && arenaLiveClientMascotMapping(grouping)','^TestR90OrderInferenceNeverEnablesMascotMapping$')
probe('incomplete-not-cached','gameplay_refresh.go','if loadCtx.Err() == nil {','if loadCtx.Err() == nil && gameplayLiveSnapshotComplete(response) {','^TestR90LiveSnapshotCacheLayersAndManualRefresh/false$')
probe('complete-short-ttl','gameplay_refresh.go','(wholeGame || time.Since(c.at) < ttl)','(wholeGame && false || time.Since(c.at) < ttl)','^TestR90LiveSnapshotCacheLayersAndManualRefresh/true$')
probe('complete-always-false','web/gameplay.js','function liveSnapshotComplete(data) {','function liveSnapshotComplete(data) { return false;','R90 complete snapshots never',True)
probe('no-retry-limit','web/gameplay.js','if (inGame && (liveSnapshotComplete(state.live) && state.live?.phase === phase || Number(state.liveRetryAttempts || 0) >= 8)) return;','if (inGame && liveSnapshotComplete(state.live)) return;','R90 incomplete snapshots stop',True)
probe('truth-partition-forced','arena_truth_diagnostics.go','"partition_match": arenaTruthOrder(ids, r.squadSize),','"partition_match": true,','^TestR90TruthDiagnosticSeparatesPartitionAndBlockIdentity/wrong$')
probe('queue-literal','arena_live_grouping.go','func arenaSessionOrderGroups(queueID int64, players []lcuLivePlayer) ([]string, bool) {','func arenaSessionOrderGroups(queueID int64, players []lcuLivePlayer) ([]string, bool) { if queueID == 1750 { return nil, false };','^TestR90QueueGuardScansNewGroupingModule$')
probe('partial-truth-reported-none','arena_truth_diagnostics.go','(r.noneReported || r.truthObserved)','r.noneReported','^TestR96PartialTruthDoesNotEndAsNone$')
