#!/usr/bin/env python3
"""P9/P10 actual mutations in scratch; fixed + mutants, no historical-baseline dependency."""
import importlib.util
from pathlib import Path
import sys
sys.dont_write_bytecode = True

spec = importlib.util.spec_from_file_location('r277_ci_proof', Path(__file__).with_name('r277-ci-proof.py'))
helper = importlib.util.module_from_spec(spec)
spec.loader.exec_module(helper)

CASES = [
    ('p9-double-full', 'backend/overview_wire.go', [('if revision, ok := seen[match.GameID]; ok && match.Revision != "" && revision == match.Revision {', 'if revision, ok := seen[match.GameID]; false && ok && match.Revision != "" && revision == match.Revision {')], 'TestR277OverviewWireSingleFullRecordAndLazyScores'),
    ('p9-score-parts', 'backend/overview_wire.go', [('Score *overviewListScore `json:"score,omitempty"`', 'Score *matchScore `json:"score,omitempty"`'), ('players[i].Score = &overviewListScore{p.Score.Value, p.Score.Rank, p.Score.Total, p.Score.Badge}', 'players[i].Score = p.Score')], 'TestR277OverviewWireSingleFullRecordAndLazyScores'),
    ('p10-no-dedup', 'backend/r277_client_diagnostics.go', [('duplicate := a.r277ClientDiagnosticSeen[string(signature)]', 'duplicate := false')], 'TestR277NewClientDiagnosticsAcceptedAndDeduplicated'),
    ('p10-no-zoom-event', 'backend/features.go', [('"ui_zoom_state":                  {"applied": true},', '')], 'TestR277NewClientDiagnosticsAcceptedAndDeduplicated'),
]
PATTERN = '^TestR277(OverviewWire|NewClientDiagnostics)'


def main():
    out = helper.arguments(__doc__)
    with helper.Proof(out, 'r277-wire-gocache') as proof:
        proof.run('fixed', PATTERN)
        for name, path, changes, test in CASES:
            proof.mutate(name, path, changes, '^' + test + '$', [test])
        proof.run('restored', PATTERN)


if __name__ == '__main__':
    main()
