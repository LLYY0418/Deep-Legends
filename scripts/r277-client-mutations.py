#!/usr/bin/env python3
"""P1 actual mutations in a scratch snapshot; portable --out, no live-file edits."""
import importlib.util
from pathlib import Path
import sys
sys.dont_write_bytecode = True

spec = importlib.util.spec_from_file_location('r277_ci_proof', Path(__file__).with_name('r277-ci-proof.py'))
helper = importlib.util.module_from_spec(spec)
spec.loader.exec_module(helper)

CASES = [
    ('snapshot-shutdown-clear', 'backend/main.go', '\ta.clearClientShutdownLocked()\n\ta.cancelClientSessionLocked()\n', '\ta.cancelClientSessionLocked()\n', 'TestR277ClearSnapshotCleansShutdownAndReopen'),
    ('background-probe', 'backend/connection_manager.go', 'probe(ctx, client, summonerID)', 'probe(context.Background(), client, summonerID)', 'TestR277ShutdownCancelsOldCollectionProbe'),
    ('shutdown-inventory', 'backend/connection_manager.go', '\tif a.shutdownClient != nil {\n\t\ta.mu.Unlock()\n\t\treturn false\n\t}\n\tduring := a.syncing', '\tduring := a.syncing', 'TestR277InventorySuppressedDuringShutdown'),
    ('manual-reconnect', 'backend/connection_manager.go', '\tif a.currentClientView().State != "ready" {\n\t\ta.markDisconnected("正在重新连接客户端")\n\t}\n', '', 'TestR277ManualRefreshReconnectsStuckView'),
]
PATTERN = '^TestR277(ClearSnapshot|ShutdownCancels|InventorySuppressed|ManualRefresh)'


def main():
    out = helper.arguments(__doc__)
    with helper.Proof(out, 'r277-client-gocache') as proof:
        proof.run('fixed', PATTERN)
        for name, path, old, new, test in CASES:
            proof.mutate(name, path, [(old, new)], '^' + test + '$', [test])
        proof.run('restored', PATTERN)


if __name__ == '__main__':
    main()
