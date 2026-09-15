"""Isolated regression mutations; never rewrite production workspace files."""
import json, os, pathlib, subprocess, tempfile
root = pathlib.Path(__file__).resolve().parent.parent
out = root / 'docs/r90'
out.mkdir(parents=True, exist_ok=True)
env = dict(os.environ, GOCACHE=str(root / '.gocache'), GOPATH=str(root / '.gopath'))
results = []
def probe(name, file, old, new, test, javascript=False):
    source = (root / file).read_text()
    assert source.count(old) == 1, (name, source.count(old))
    with tempfile.TemporaryDirectory(prefix='r90-mutant-') as tmp:
        tmp = pathlib.Path(tmp)
        modified = tmp / pathlib.Path(file).name
        modified.write_text(source.replace(old, new))
        runenv = env.copy()
        if name == "queue-literal": runenv["R90_QUEUE_GUARD_SOURCE"] = str(modified)
        if javascript:
            runenv['R90_GAMEPLAY_SOURCE'] = str(modified)
            cmd = ['node', '--test', '--test-name-pattern', test, 'web/r90.test.cjs']
        else:
            overlay = tmp / 'overlay.json'
            overlay.write_text(json.dumps({'Replace': {str(root/file): str(modified)}}))
            cmd = ['go', 'test', '-overlay', str(overlay), '-count=1', '-timeout=60s', '-run', test, '.']
        result = subprocess.run(cmd, cwd=root, env=runenv, capture_output=True, text=True, timeout=90)
        output = result.stdout + result.stderr
        killed = result.returncode != 0 and ('FAIL: Test' in output or 'AssertionError' in output) and 'build failed' not in output and 'test timed out' not in output
        (out / (name + '.txt')).write_text(output)
        results.append({'id': name, 'killed': killed, 'test': test})
        (out/'mutations.json').write_text(json.dumps(results, indent=2)+'\n')
        print(name, 'KILLED' if killed else 'SURVIVED/INVALID', flush=True)
        assert killed, (name, output[-1500:])
probe('wrong-block-size','arena_live_grouping.go','index/squadSize + 1','index/2 + 1','^TestR90AlliesVerifyAndRejectScrambledPlayerlist/false$')
probe('removed-ally-check','arena_live_grouping.go','verified, rejected := arenaAlliesCorroborate(groups, response.Players, remembered, squadSize, grouping, current.PUUID)','verified, rejected := true, false','^TestR90AlliesVerifyAndRejectScrambledPlayerlist/true$')
probe('raw-identity-only','arena_live_grouping.go','names, riotIDs := livePlayerIdentityValues(raw, player)','names, riotIDs := livePlayerIdentityValues(raw, gameplayLivePlayer{})','^TestR90RealShapeGroups$')
probe('session-instead-of-playerlist','arena_live_grouping.go','for i, p := range response.Players {\n\t\t\tgroups[i] = liveClientGroupingForPlayer(grouping, raw[i], p)\n\t\t}\n\t\tsource, order = "live-client-order", true','groups, _ = arenaSessionOrderGroups(response.QueueID, raw)\n\t\tsource, order = "live-client-order", true','^TestR90SeventeenGameflowUsesEighteenPlayerlistBoundaries$')
probe('guessed-mascots','arena_live_grouping.go','return grouped && !order && arenaLiveClientMascotMapping(grouping)','return grouped','^TestR90OrderInferenceNeverEnablesMascotMapping$')
probe('removed-order-mascot-guard','arena_live_grouping.go','return grouped && !order && arenaLiveClientMascotMapping(grouping)','return grouped && arenaLiveClientMascotMapping(grouping)','^TestR90OrderInferenceNeverEnablesMascotMapping$')
probe('incomplete-not-cached','gameplay_refresh.go','if loadCtx.Err() == nil {','if loadCtx.Err() == nil && gameplayLiveSnapshotComplete(response) {','^TestR90LiveSnapshotCacheLayersAndManualRefresh/false$')
probe('complete-short-ttl','gameplay_refresh.go','(wholeGame || time.Since(c.at) < ttl)','(wholeGame && false || time.Since(c.at) < ttl)','^TestR90LiveSnapshotCacheLayersAndManualRefresh/true$')
probe('complete-always-false','web/gameplay.js','function liveSnapshotComplete(data) {','function liveSnapshotComplete(data) { return false;','R90 complete snapshots never',True)
probe('no-retry-limit','web/gameplay.js','if (inGame && (liveSnapshotComplete(state.live) && state.live?.phase === phase || Number(state.liveRetryAttempts || 0) >= 8)) return;','if (inGame && liveSnapshotComplete(state.live)) return;','R90 incomplete snapshots stop',True)
probe('truth-partition-forced','arena_live_grouping.go','"partition_match": partition,','"partition_match": true,','^TestR90TruthDiagnosticSeparatesPartitionAndBlockIdentity/wrong$')
probe('queue-literal','arena_live_grouping.go','func arenaSessionOrderGroups(queueID int64, players []lcuLivePlayer) ([]string, bool) {','func arenaSessionOrderGroups(queueID int64, players []lcuLivePlayer) ([]string, bool) { if queueID == 1750 { return nil, false };','^TestR90QueueGuardScansNewGroupingModule$')
