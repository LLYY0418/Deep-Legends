#!/usr/bin/env python3
"""R89 guard/diagnostic mutations via Go overlays; never edit production files."""
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
GUARD = 'r88_queue_guard_test.go'
CASES = [
    ('original-direct', GUARD, 'return constant.ToInt(value).Kind() == constant.Int', 'return constant.ToInt(value).Kind() == constant.Int && value.ExactString() != "3110"', 'TestR88QueueGuardMutations'),
    ('original-alias', GUARD, 'e.Obj != nil && aliases[e.Obj]', 'false && e.Obj != nil && aliases[e.Obj]', 'TestR88QueueGuardMutations'),
    ('original-switch', GUARD, 'if isQueue(n.Tag) {', 'if false && isQueue(n.Tag) {', 'TestR88QueueGuardMutations'),
    ('numeric-string', GUARD, 'return err == nil', 'return err == nil && false', 'TestR89QueueGuardEquivalentMutations'),
    ('stringify-call', GUARD, 'case *ast.CallExpr:\n\t\t\tfor _, arg := range e.Args {', 'case *ast.CallExpr:\n\t\t\tif len(e.Args) != 1 { return false }; for _, arg := range e.Args {', 'TestR89QueueGuardEquivalentMutations'),
    ('reassigned-literal', GUARD, 'ok && numericAliases[id.Obj]', 'ok && false && numericAliases[id.Obj]', 'TestR89QueueGuardEquivalentMutations'),
    ('equality-helper', GUARD, 'case "reflect.DeepEqual",', 'case "ignored.DeepEqual",', 'TestR89QueueGuardEquivalentMutations'),
    ('storage-fingerprint', 'storage.go', 'record["build_fingerprint"] = buildFingerprint', '', 'TestR89Diagnostic'),
]

results = []
with tempfile.TemporaryDirectory(prefix='r89-followup-mutants-') as temp:
    directory = Path(temp)
    for name, relative, old, new, pattern in CASES:
        source = ROOT / relative
        original = source.read_text()
        assert old in original, (name, old)
        mutant = directory / (name + '.go')
        mutant.write_text(original.replace(old, new, 1))
        overlay = directory / (name + '.json')
        overlay.write_text(json.dumps({'Replace': {str(source): str(mutant)}}))
        result = subprocess.run(
            ['go', 'test', '-vet=off', '-overlay', str(overlay), '-run', '^' + pattern, '-count=1', '.'],
            cwd=ROOT, env=dict(os.environ, GOCACHE=str(ROOT / '.gocache'), GOPATH=str(ROOT / '.gopath')),
            capture_output=True, text=True, timeout=120,
        )
        output = result.stdout + result.stderr
        killed = result.returncode != 0 and ('--- FAIL: ' + pattern) in output
        results.append({'mutation': name, 'test': pattern, 'exit': result.returncode, 'killed': killed})
        print(name, 'KILLED' if killed else 'FAILED TO VERIFY', flush=True)
        if not killed:
            print(output[-4000:], flush=True)
report = ROOT / 'docs/r89-r88-followup/mutations.json'
report.parent.mkdir(parents=True, exist_ok=True)
report.write_text(json.dumps(results, indent=2) + '\n')
raise SystemExit(0 if all(result['killed'] for result in results) else 1)
