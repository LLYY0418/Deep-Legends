#!/usr/bin/env python3
"""Portable Riot fixed/actual-mutant proof. Historical before logs are not fabricated or required."""
import importlib.util
from pathlib import Path
import sys
sys.dont_write_bytecode = True

spec = importlib.util.spec_from_file_location('r277_ci_proof', Path(__file__).with_name('r277-ci-proof.py'))
helper = importlib.util.module_from_spec(spec)
spec.loader.exec_module(helper)
ROOT = Path(__file__).resolve().parents[1]

TESTS = ('^TestR277(HiddenDirectoryOverviewProgress|OverviewYieldBounded|'
         'InternalWaitTimeoutNotNetwork|ForegroundQuotaOverflow|OverflowRelay429Countdown|'
         'BackgroundNeverQuotaOverflows|UnhealthyRelayKeepsLocalCountdown|TimeRangeIDsBoundary|'
         'EmbeddedHeaderHintCannotPinRelayAfterRecovery|OverflowDoesNotJoinBlockedLocalFIFO|'
         'MethodQuotaDoesNotOverflow|ControlledClockOverflowNoLocalWait)$')
MUTANTS = [
    ('mutant-hidden-global', 'backend/riot_routing.go',
     'hidden := proOwned && riotForeground.visibilityKnown && !riotForeground.directoryVisible',
     'hidden := riotForeground.visibilityKnown && !riotForeground.directoryVisible',
     '^TestR277HiddenDirectoryOverviewProgress$', ['TestR277HiddenDirectoryOverviewProgress']),
    ('mutant-unbounded-yield', 'backend/riot_routing.go',
     'bounded := isRiotBackground(ctx) && !proOwned', 'bounded := false',
     '^TestR277OverviewYieldBounded$', ['TestR277OverviewYieldBounded/active', 'TestR277OverviewYieldBounded/recovery']),
    ('mutant-overflow-off', 'backend/riot_routing.go',
     'if ctx.Err() != nil || isRiotBackground(ctx) || source != "user" && source != "embedded" {',
     'if true || ctx.Err() != nil || isRiotBackground(ctx) || source != "user" && source != "embedded" {',
     '^TestR277ControlledClockOverflowNoLocalWait$',
     ['TestR277ControlledClockOverflowNoLocalWait/user', 'TestR277ControlledClockOverflowNoLocalWait/embedded']),
    ('mutant-internal-timeout-network-label', 'backend/riot_api.go',
     'body.Kind = "internal-wait"\n\t\tbody.Error = "查询调度等待超时，请重试；已加载的战绩仍可查看"',
     'body.Kind = "http"\n\t\tbody.Error = "韩服数据读取超时，请稍后重试"',
     '^TestR277InternalWaitTimeoutNotNetwork$', ['TestR277InternalWaitTimeoutNotNetwork']),
]


def main():
    out = helper.arguments(__doc__)
    with helper.Proof(out, 'r277-riot-gocache', timeout=300) as proof:
        proof.run('after-proof', TESTS)
        for name, path, old, new, pattern, expected in MUTANTS:
            proof.mutate(name, path, [(old, new)], pattern, expected)
        proof.run('after-restored-race', TESTS + '|^TestR273Foreground', race=True)


if __name__ == '__main__':
    main()
