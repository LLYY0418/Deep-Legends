#!/usr/bin/env python3
"""Shared R277 Go proof support; snapshots are mutated, never the live checkout."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import platform
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]


def arguments(description):
    parser = argparse.ArgumentParser(description=description)
    parser.add_argument('legacy_out', nargs='?', type=Path)
    parser.add_argument('--out', type=Path)
    args = parser.parse_args()
    if (args.out is None) == (args.legacy_out is None):
        parser.error('supply exactly one --out directory (legacy positional path is supported)')
    return (args.out or args.legacy_out).resolve()


class Proof:
    def __init__(self, out, cache_name, timeout=180):
        self.out = out
        self.out.mkdir(parents=True, exist_ok=True)
        self.timeout = timeout
        self.scratch = Path(os.environ.get('PI_SCRATCH_DIR') or os.environ.get('RUNNER_TEMP') or tempfile.gettempdir())
        self.scratch.mkdir(parents=True, exist_ok=True)
        self.env = dict(os.environ, GOCACHE=str(self.scratch / cache_name))
        self.result = {'host': platform.system(), 'arch': platform.machine(), 'native_windows_executed': os.name == 'nt',
                       'gocache': self.env['GOCACHE'], 'runs': [], 'mutations': [], 'status': 'RUNNING'}
        self.temporary = None

    def __enter__(self):
        self.temporary = tempfile.TemporaryDirectory(prefix='r277-go-proof-', dir=self.scratch)
        self.root = Path(self.temporary.name)
        try:
            for name in ['go.mod', 'go.sum']:
                shutil.copy2(ROOT / name, self.root / name)
            shutil.copytree(ROOT / 'backend', self.root / 'backend', ignore=shutil.ignore_patterns('node_modules', '__pycache__'))
            # Fail closed on empty filtered selections, including newly added R277 tests.
            self.tests = sorted(set(re.findall(r'^func (Test\w+)\(t \*testing\.T\)', '\n'.join(
                p.read_text(encoding="utf-8") for p in (self.root / 'backend').glob('*_test.go')), re.MULTILINE)))
            self.persist()
            return self
        except Exception as error:
            self.__exit__(type(error), error, error.__traceback__)
            raise

    def persist(self):
        (self.out / 'result.json').write_text(json.dumps(self.result, indent=2) + '\n', encoding="utf-8")

    def run(self, label, pattern, race=False, expected_fail=()):
        command = ['go', 'test', './backend', '-count=1', '-timeout=180s', '-json', '-run', pattern]
        if race:
            command.append('-race')
        log = self.out / (label + '.jsonl')
        stderr = self.out / (label + '.stderr')
        selected = [test for test in self.tests if re.search(pattern, test)]
        if not selected:
            raise RuntimeError('empty Go test selection: ' + label)
        with log.open('w', encoding="utf-8") as stdout, stderr.open('w', encoding="utf-8") as errors:
            process = subprocess.run(command, cwd=self.root, env=self.env, stdout=stdout, stderr=errors, timeout=self.timeout)
        events = [json.loads(line) for line in log.read_text(encoding="utf-8").splitlines() if line.strip()]
        terminal = {row['Test']: row['Action'] for row in events if row.get('Test') and row['Action'] in {'pass', 'fail', 'skip'}}
        elapsed = {row['Test']: row.get('Elapsed') for row in events if row.get('Test') and row['Action'] in {'pass', 'fail', 'skip'}}
        row = {'label': label, 'command': command, 'exit': process.returncode, 'selected': selected,
               'terminal': terminal, 'elapsed_seconds': elapsed, 'expected_failed_tests': list(expected_fail), 'log': log.name, 'stderr': stderr.name}
        self.result['runs'].append(row)
        self.persist()
        if expected_fail:
            if process.returncode == 0 or not all(terminal.get(test) == 'fail' for test in expected_fail):
                raise RuntimeError('mutation survived or unrelated build/harness failure: ' + label)
        elif process.returncode != 0 or any(terminal.get(test) != 'pass' for test in selected):
            raise RuntimeError('fixed/restored test selection failed or skipped: ' + label)
        return row

    def mutate(self, label, relative, changes, pattern, expected_fail):
        path = self.root / relative
        original = path.read_bytes()
        mutant = original.decode()
        for old, new in changes:
            if mutant.count(old) != 1:
                raise RuntimeError('unexpected anchor: ' + label + ': ' + old)
            mutant = mutant.replace(old, new, 1)
        try:
            path.write_bytes(mutant.encode())
            self.run(label, pattern, expected_fail=expected_fail)
        finally:
            # Restore even on timeout/compile failure/assertion failure.
            path.write_bytes(original)
        if path.read_bytes() != original:
            raise RuntimeError('mutation was not restored: ' + label)
        self.result['mutations'].append({'name': label, 'path': relative, 'status': 'KILLED',
                                         'restored_sha256': hashlib.sha256(original).hexdigest()})
        self.persist()

    def __exit__(self, kind, error, traceback):
        self.result['status'] = 'FAIL' if error else 'PASS'
        if error:
            self.result['error'] = str(error)
        try:
            with (self.out / 'cache-clean.log').open('w', encoding="utf-8") as log:
                subprocess.run(['go', 'clean', '-cache', '-testcache'], cwd=ROOT, env=self.env,
                               stdout=log, stderr=subprocess.STDOUT, check=True)
        finally:
            if self.temporary:
                self.temporary.cleanup()
            self.persist()


def main():
    out = arguments('Run every actual R277 Go test on this host; Windows CI produces native timing JSON.')
    with Proof(out, 'r277-measure-gocache') as proof:
        # Preserve the new L5 test's actual stored bytes/model, explicitly synthetic.
        proof.env['R277_L5_OUTPUT'] = str(out / 'l5')
        proof.run('r277-native-measurements', '^TestR277', race=True)


if __name__ == '__main__':
    main()
