#!/usr/bin/env python3
"""Compile specimens, prove green/red/green, and never edit the real guard."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
GUARD = ROOT / 'r88_queue_guard_test.go'
OUTPUT = ROOT / 'docs/r90-queue-guard'
FILES = ['r88_queue_guard_test.go', 'r89_queue_guard_test.go', 'r90_queue_guard_test.go']
INDEX = 'if isQueue(n.Index) && collectionOf(n.X)&queueGuardNumericKeys != 0 {'
ASSERT = 'case *ast.TypeAssertExpr:\n\t\t\treturn isQueue(e.X)'
STAR = 'case *ast.StarExpr:\n\t\t\treturn isQueue(e.X)'
SCAN = 'var found []token.Pos\n\tast.Inspect(f, func(n ast.Node) bool {\n\t\tswitch n := n.(type) {'
CASES = [
    # One independently killed mutation for each of the four worklist specimens.
    ('literal-map', 'TestR90QueueGuardRequired/literal-map', INDEX, INDEX.replace('if ', 'if false && ', 1)),
    ('handler-map', 'TestR90QueueGuardRequired/handler-map', 'return collections[e.Obj]', 'return 0'),
    ('range-slice', 'TestR90QueueGuardRequired/range-slice', 'facts := collectionOf(n.X)', 'facts := queueGuardCollection(0)'),
    ('type-assert', 'TestR90QueueGuardRequired/type-assert', ASSERT, ASSERT.replace('return isQueue(e.X)', 'return false')),
    # All five independently proposed AST directions, including existing closure coverage.
    ('pointer', 'TestR90QueueGuardExtra/pointer', STAR, STAR.replace('return isQueue(e.X)', 'return false')),
    ('closure-capture', 'TestR90QueueGuardExtra/closure-capture', SCAN, SCAN + '\n\t\tcase *ast.FuncLit: return false'),
    ('type-switch', 'TestR90QueueGuardExtra/type-switch', ASSERT, ASSERT.replace('return isQueue(e.X)', 'return false')),
    ('iota-inheritance', 'TestR90QueueGuardExtra/iota-inheritance', 'ok && typed.Value != nil', 'ok && false && typed.Value != nil'),
    ('slice-membership', 'TestR90QueueGuardExtra/slice-membership', 'case "slices.Contains",', 'case "ignored.Contains",'),
    # Check the connections between the new paths as well.
    ('assert-comma-ok', 'TestR90QueueGuardCombinations/assert-comma-ok', 'len(lhs) == 2 && len(rhs) == 1', 'false && len(lhs) == 2 && len(rhs) == 1'),
    ('range-map-key', 'TestR90QueueGuardCombinations/range-map-key', 'bindFacts(n.Key, facts&queueGuardNumericKeys != 0, facts&queueGuardQueueKeys != 0)', 'bindFacts(n.Key, false, false)'),
    ('generic-membership-alias', 'TestR90QueueGuardCombinations/membership-alias', 'return functionName(e.X) // e.g. slices.Contains[[]int64, int64].', 'return "" // Disabled generic function alias.'),
    ('range-cast-target', 'TestR90QueueGuardCombinations/range-cast-target', 'return isInteger(valueArgument(e))', 'return false'),
    ('range-string-target', 'TestR90QueueGuardCombinations/range-string-target', 'return isInteger(valueArgument(e))', 'return false'),
    ('range-arithmetic-target', 'TestR90QueueGuardCombinations/range-arithmetic-target', 'queueGuardArithmetic(e.Op) && isInteger(e.X) && isInteger(e.Y)', 'false && queueGuardArithmetic(e.Op) && isInteger(e.X) && isInteger(e.Y)'),
    # R87/R88/R89 regression mutations; Q1/Q3 are deliberately not re-run here.
    ('original-direct', 'TestR88QueueGuardMutations', 'return constant.ToInt(value).Kind() == constant.Int', 'return constant.ToInt(value).Kind() == constant.Int && value.ExactString() != "3110"'),
    ('original-alias', 'TestR88QueueGuardMutations', 'e.Obj != nil && aliases[e.Obj]', 'false && e.Obj != nil && aliases[e.Obj]'),
    ('original-switch', 'TestR88QueueGuardMutations', 'if isQueue(n.Tag) {', 'if false && isQueue(n.Tag) {'),
    ('numeric-string', 'TestR89QueueGuardEquivalentMutations', 'return err == nil', 'return err == nil && false'),
    ('stringify-call', 'TestR89QueueGuardEquivalentMutations', 'if isQueue(arg) {', 'if len(e.Args) == 1 && isQueue(arg) {'),
    ('reassigned-literal', 'TestR89QueueGuardEquivalentMutations', 'ok && numericAliases[id.Obj]', 'ok && false && numericAliases[id.Obj]'),
    ('equality-helper', 'TestR89QueueGuardEquivalentMutations', 'case "reflect.DeepEqual",', 'case "ignored.DeepEqual",'),
]


def run(pattern, overlay=None):
    # These three files form a self-contained test package. Each R90 fixture is
    # also type-checked against a separate queue_groups.go inside its test.
    command = ['go', 'test', '-json', '-vet=off']
    if overlay:
        command += ['-overlay', str(overlay)]
    command += FILES + ['-run', pattern, '-count=1']
    result = subprocess.run(command, cwd=ROOT,
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
    suite = 'TestR88QueueGuard|TestR89QueueGuard|TestR90QueueGuard'
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
            assertion = ('missed R90 queue literal:' in text or 'missed additional queue literal:' in text or
                         'missed combined queue literal:' in text or 'missed mutation:' in text or
                         'missed equivalent queue literal comparison:' in text)
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
        raise SystemExit('Mutation verification failed; see docs/r90-queue-guard/mutations.json')


if __name__ == '__main__':
    main()
