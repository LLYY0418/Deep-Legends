#!/usr/bin/env python3
"""Run focused R87 regressions against isolated mutations; never edit production files."""
import concurrent.futures
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'docs/r87/addendum/mutations'
OUT.mkdir(parents=True, exist_ok=True)
GO = [
    ('P2-no-prewarm', 'gameplay_refresh.go', 'if phase != "ChampSelect" && phase != "InProgress" {', 'if phase != "ChampSelect" {', 'TestR87PhasePrewarmMakesFirstLiveResponseFast'),
    ('P2-short-warm-ttl', 'gameplay_refresh.go', '(!c.at.IsZero() && phase == "InProgress") || time.Since(c.at) < ttl', 'time.Since(c.at) < ttl', 'TestR87PrewarmSurvivesLongHiddenWindow'),
    ('P3-no-end-invalidation', 'gameplay_refresh.go', 'a.invalidateOverviewPlayer(a.currentPlayerRef())', '// end invalidation removed', 'TestR87WaitingForStatsInvalidatesOnlySubjectAndDetachesFlight'),
    ('P3-force-keeps-sgp-cache', 'gameplay.go', 'if force && a.sgp != nil {', 'if false && force && a.sgp != nil {', 'TestGameplayOverviewHistoryWindowUsesCacheAcrossRequests'),
    ('P4-no-early-exit', 'catalog.go', 'if fast && validatedEvidence == len(validCatalogIDs) {', 'if false && fast && validatedEvidence == len(validCatalogIDs) {', 'TestR87CollectionSharesPayloadsAndDoesNotCacheEmptyAcrossRefreshes'),
    ('P4-no-payload-sharing', 'lcu_api.go', 'if api.reads != nil {\n\t\t\tdata, err = api.reads.captured(path)', 'if false && api.reads != nil {\n\t\t\tdata, err = api.reads.captured(path)', 'TestR87CollectionSharesPayloadsAndDoesNotCacheEmptyAcrossRefreshes'),
    ('P5-double-scan', 'claim_center.go', 'response.Scan = canonical', 'response.Scan = scanClaimsObserved(ctx, client, a.recordDiagnostic)', 'TestR87NineClaimsScanOnceEachAndTimeoutBudgetIncludesLock'),
    ('P5-no-budget', 'claim_center.go', 'context.WithTimeout(r.Context(), 12*time.Second)', 'context.WithCancel(r.Context())', 'TestR87ClaimHandlerSuppliesOwnBudget'),
    ('P6-old-arena-check', 'champselect_execution.go', 'arena := r.champSelect.groupID == "arena"', 'arena := session.QueueID == 1700 || session.QueueID == 1710', 'TestR87ArenaSentinelActuallyPatches'),
    ('A1-no-failure-reason', 'watch_rules.go', '"action": "position-broadcast", "result": result, "reason": reason', '"action": "position-broadcast", "result": result', 'TestR87PositionBroadcastEveryFailureReason'),
    ('A3-no-phase-preflight', 'watch_rules.go', 'if action == "reconnect" {', 'if false && action == "reconnect" {', 'TestR87ReconnectPreflightRejectsLostEndEvent'),
    ('P7-no-end-reset', 'watch_rules.go', 'r.autoMatchStarted = false\n\t\tr.autoMatchExhausted = false', 'r.autoMatchStarted = false\n\t\t// exhausted reset removed', 'TestR87P7EveryEndPhaseResetsMatchmakingExhaustion'),
    ('P7-silent-state-guard', 'watch_rules.go', 'r.record(map[string]any{"event": "watch_action", "action": "auto-matchmaking", "result": "skipped_state", "reason": reason})', '// state guard diagnostic removed', 'TestR87P7FourStateGuardExplainsEachSkip'),
]
JS = [
    ('P2-resync-drops-queue', 'gameplay.js', 'state.resyncPending = false;', 'state.resyncPending = false; state.liveRefreshQueued = false;', 'R87 hidden SSE'),
    ('P3-only-active-tab-dirty', 'gameplay.js', 'tab.dirty = true;', 'if (tab !== activeTab()) continue; tab.dirty = true;', 'R87 all tabs'),
    ('P5-no-15-second-timeout', 'suite.js', 'timeout = 15000', 'timeout = 30000', 'R87 suite fetch'),
    ('A2-only-EndOfGame-display', 'suite.js', '["WaitingForStats", "PreEndOfGame", "EndOfGame"].includes(phase)', '["EndOfGame"].includes(phase)', 'R87 pending loot'),
]

def mutated(text, before, after):
    count = text.count(before)
    if count != 1:
        raise ValueError(f'expected exactly one mutation target, got {count}: {before}')
    return text.replace(before, after)

def run_case(case, language):
    name, file, before, after, test = case
    with tempfile.TemporaryDirectory(prefix='r87-mutant-') as temporary:
        temp = Path(temporary)
        env = os.environ.copy()
        if language == 'go':
            source = ROOT / file
            changed = temp / file
            changed.write_text(mutated(source.read_text(), before, after))
            overlay = temp / 'overlay.json'
            overlay.write_text(json.dumps({'Replace': {str(source): str(changed)}}))
            command = ['go', 'test', '-overlay', str(overlay), '-run', '^' + test + '$', '-count=1', '-timeout=45s', '.']
        else:
            for source in (ROOT / 'web').glob('*.js'):
                (temp / source.name).write_text(source.read_text())
            source = temp / file
            source.write_text(mutated(source.read_text(), before, after))
            env['R87_TEST_WEB_ROOT'] = str(temp)
            command = ['node', '--test', '--test-name-pattern', test, 'web/r87.test.cjs']
        result = subprocess.run(command, cwd=ROOT, env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=90)
        log = result.stdout
        (OUT / (name + '.log')).write_text(log)
        caught = result.returncode != 0 and ('--- FAIL:' in log if language == 'go' else ('AssertionError' in log or 'ERR_ASSERTION' in log))
        row = {'name': name, 'test': test, 'exit_code': result.returncode, 'caught_by_assertion': caught, 'log': name + '.log'}
        print(json.dumps(row), flush=True)
        return row

if __name__ == '__main__':
    # Go overlays are independent and their caches are concurrency-safe.
    with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
        results = list(pool.map(lambda case: run_case(case, 'go'), GO))
    results += [run_case(case, 'js') for case in JS]
    (OUT / 'results.json').write_text(json.dumps(results, indent=2) + '\n')
    if not all(row['caught_by_assertion'] for row in results):
        raise SystemExit('One or more mutations escaped the guard or did not produce an assertion failure')
