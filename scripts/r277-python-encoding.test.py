#!/usr/bin/env python3
"""Portable proof I/O: execute under a simulated Windows cp1252 default."""
import ast
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
PROOF = Path(os.environ.get('R277_ENCODING_PROOF', ROOT / 'scripts/r277-ci-proof.py'))
INSTALLER = Path(os.environ.get('R277_ENCODING_INSTALLER', ROOT / 'scripts/r277-installer-callbacks.py'))


def load(path, name):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    module.ROOT = ROOT
    return module


original_open = Path.open


def cp1252_open(self, mode='r', buffering=-1, encoding=None, errors=None, newline=None):
    return original_open(self, mode, buffering, 'cp1252' if 'b' not in mode and encoding in (None, 'locale') else encoding, errors, newline)


class EncodingTests(unittest.TestCase):
    def test_all_proof_text_io_declares_utf8(self):
        for path in (PROOF, INSTALLER):
            tree = ast.parse(path.read_text(encoding='utf-8'))
            calls = []
            for node in ast.walk(tree):
                if not isinstance(node, ast.Call) or not isinstance(node.func, ast.Attribute):
                    continue
                text_io = node.func.attr in {'read_text', 'write_text', 'open'}
                text_process = node.func.attr == 'run' and any(k.arg == 'text' and isinstance(k.value, ast.Constant) and k.value.value is True for k in node.keywords)
                if text_io or text_process:
                    calls.append(node)
                    self.assertTrue(any(k.arg == 'encoding' and isinstance(k.value, ast.Constant) and k.value.value == 'utf-8' for k in node.keywords), f'{path.name}:{node.lineno} requires explicit UTF-8')
            self.assertGreater(len(calls), 4)

    def test_installer_audit_reads_chinese_sources_with_cp1252_default(self):
        installer = load(INSTALLER, 'r277_encoding_installer')
        with tempfile.TemporaryDirectory(prefix='r277-encoding-audit-') as directory, patch.object(Path, 'open', cp1252_open):
            out = Path(directory)
            installer.audit(out)
            report = json.loads((out / 'callback-audit.json').read_text(encoding='utf-8'))
            self.assertEqual(report['callback_count'], 15)

    def test_go_snapshot_and_jsonl_with_cp1252_default(self):
        helper = load(PROOF, 'r277_encoding_proof')
        real_run = subprocess.run

        def command(command, **kwargs):
            if command[:2] == ['go', 'clean']:
                return subprocess.CompletedProcess(command, 0)
            self.assertEqual(command[:2], ['go', 'test'])
            # Emit real UTF-8 bytes, as Go does, into the helper's own log handles.
            event = {'Action': 'pass', 'Test': 'TestUTF8', 'Output': '中文诊断\u008d'}
            code = 'import sys;sys.stdout.buffer.write(' + repr((json.dumps(event, ensure_ascii=False) + '\n').encode('utf-8')) + ')'
            return real_run([sys.executable, '-c', code], **kwargs)

        with tempfile.TemporaryDirectory(prefix='r277-encoding-proof-') as directory, patch.object(Path, 'open', cp1252_open), patch.object(helper.subprocess, 'run', command):
            with helper.Proof(Path(directory), 'r277-encoding-gocache') as proof:
                self.assertIn('TestR277ReopenDiscoveryIdentityGatewayProductionPath', proof.tests)
                proof.tests = ['TestUTF8']
                proof.run('utf8-jsonl', '^TestUTF8$')
            self.assertEqual(json.loads((Path(directory) / 'result.json').read_text(encoding='utf-8'))['status'], 'PASS')
            self.assertIn('中文诊断', (Path(directory) / 'utf8-jsonl.jsonl').read_text(encoding='utf-8'))


if __name__ == '__main__':
    unittest.main()
