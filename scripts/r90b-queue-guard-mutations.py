#!/usr/bin/env python3
"""Compile specimens, prove green/red/green, and never edit the real guard."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
BACKEND = ROOT / 'backend'
GUARD = BACKEND / 'r88_queue_guard_test.go'
OUTPUT = ROOT / 'docs/r90b-queue-guard'
FILES = ['r88_queue_guard_test.go', 'r89_queue_guard_test.go', 'r90_queue_guard_test.go', 'r90b_queue_guard_test.go']
CASES = [
    ('channel-select', 'TestR90BQueueGuardRequired/channel-select',
     'bindFacts(n.Chan, isInteger(n.Value), isQueue(n.Value))', 'bindFacts(n.Chan, isInteger(n.Value), false)'),
    ('channel-receive', 'TestR90BQueueGuardRequired/channel-receive',
     'bindFacts(n.Chan, isInteger(n.Value), isQueue(n.Value))', 'bindFacts(n.Chan, isInteger(n.Value), false)'),
    ('struct-field', 'TestR90BQueueGuardRequired/struct-field',
     'queue && !queueFields[field]', 'false && queue && !queueFields[field]'),
    ('json-marshal', 'TestR90BQueueGuardRequired/json-marshal',
     'bind(lhs[0], rhs[0]) // First returned value, never the error/status.',
     '// Disabled first-return-value propagation.'),
    ('binary-buffer', 'TestR90BQueueGuardRequired/binary-buffer',
     'bindFacts(n.Args[0], isInteger(n.Args[1]), isQueue(n.Args[1]))',
     'bindFacts(n.Args[0], isInteger(n.Args[1]), false)'),
    ('generic-comparator', 'TestR90BQueueGuardRequired/generic-comparator',
     'case "queueguard.generic-comparison":', 'case "ignored.generic-comparison":'),
]


def run(pattern, overlay=None):
    # These four files form a self-contained test package. Each R90 fixture is
    # also type-checked against a separate queue_groups.go inside its test.
    command = ['go', 'test', '-json', '-vet=off']
    if overlay:
        command += ['-overlay', str(overlay)]
    command += FILES + ['-run', pattern, '-count=1']
    result = subprocess.run(command, cwd=BACKEND,
                            env=dict(os.environ, GOCACHE=str(ROOT / '.gocache'), GOPATH=str(ROOT / '.gopath')),
                            capture_output=True, text=True, timeout=120)
    events = []
    for line in result.stdout.splitlines():
        try:
            events.append(json.loads(line))
        except json.JSONDecodeError:
            pass
    return result, events


def main():
    OUTPUT.mkdir(exist_ok=True, parents=True)
    original = GUARD.read_text()
    before_hash = hashlib.sha256(GUARD.read_bytes()).hexdigest()
    suite = 'TestR88QueueGuard|TestR89QueueGuard|TestR90QueueGuard|TestR90BQueueGuard'
    green, _ = run(suite)
    (OUTPUT / 'mutation-green-before.txt').write_text(green.stdout + green.stderr)
    if green.returncode:
        raise SystemExit('Baseline is not green; mutations were not run')
    results = []
    with tempfile.TemporaryDirectory(prefix='r90-queue-mutants-') as temporary:
        temp = Path(temporary)
        for name, test, old, new in CASES:
            assert original.count(old) == 1, (name, 'mutation anchor must be unique')
            mutant = temp / (name + '.go')
            mutant.write_text(original.replace(old, new, 1))
            overlay = temp / (name + '.json')
            overlay.write_text(json.dumps({'Replace': {str(GUARD): str(mutant)}}))
            # Anchor parent and subtest separately so sibling cases cannot kill
            # a mutation on behalf of the specimen being validated.
            pattern = '/'.join('^' + part + '$' for part in test.split('/'))
            result, events = run(pattern, overlay)
            failed = any(e.get('Action') == 'fail' and e.get('Test') == test for e in events)
            text = ''.join(e.get('Output', '') for e in events)
            assertion = 'missed R90B queue literal:' in text
            killed = result.returncode != 0 and failed and assertion
            (OUTPUT / (name + '.txt')).write_text(result.stdout + result.stderr)
            results.append({'mutation': name, 'test': test, 'exit': result.returncode, 'killed': killed,
                            'old': old, 'new': new})
            print(name, 'KILLED' if killed else 'NOT VERIFIED', flush=True)
    repaired, _ = run(suite)
    (OUTPUT / 'mutation-green-after.txt').write_text(repaired.stdout + repaired.stderr)
    unchanged = before_hash == hashlib.sha256(GUARD.read_bytes()).hexdigest()
    report = {'guard_sha256': before_hash, 'baseline_passed': green.returncode == 0,
              'restored_passed': repaired.returncode == 0, 'source_unchanged': unchanged,
              'mutations': results}
    (OUTPUT / 'mutations.json').write_text(json.dumps(report, indent=2) + '\n')
    if not (unchanged and repaired.returncode == 0 and all(row['killed'] for row in results)):
        raise SystemExit('Mutation verification failed; see docs/r90b-queue-guard/mutations.json')


if __name__ == '__main__':
    main()
