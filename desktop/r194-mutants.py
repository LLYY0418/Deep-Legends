from pathlib import Path
import json
import re
import subprocess

root = Path.cwd()
out = Path('/private/tmp/r194-mutants')
out.mkdir(exist_ok=True)
file = root / 'backend/live_roster_recovery.go'
source = file.read_text()
start = source.index('func liveTenPlayerRosterQueue(')
end = source.index('\nfunc liveAnonymousRosterQueue(', start)
old_queue = '''func liveTenPlayerRosterQueue(queueID int64) bool {
\tif queueID == seasonQueueSoloDuo || queueID == seasonQueueFlex {
\t\treturn true
\t}
\tdefinition, ok := supportedQueueDefinition(queueID)
\treturn ok && definition.ModeGroup == "hextech-aram"
}
'''
allowed = 'case "solo", "flex", "match", "aram", "hextech-aram", "clash", "urf":'
bot_gate = '\t\t\tif entry.IsBot {\n\t\t\t\tcontinue\n\t\t\t}\n'
assert source.count(allowed) == 1 and source.count(bot_gate) == 1
variants = [
    ('old-queue-gate', source[:start] + old_queue + source[end:], ['TestR194QuickplayAnonymousTopEndToEnd', 'TestR194TenPlayerQueues']),
    ('allow-bots', source.replace(allowed, allowed[:-1] + ', "bots":', 1), ['TestR194TenPlayerQueues', 'TestR194BotsAndUnsupportedDiagnostic']),
    ('anonymous-bot', source.replace(bot_gate, '', 1), ['TestR194BotsAndUnsupportedDiagnostic']),
]
results = []
for name, mutant, tests in variants:
    directory = out / name
    directory.mkdir(exist_ok=True)
    replacement = directory / file.name
    replacement.write_text(mutant)
    overlay = directory / 'overlay.json'
    overlay.write_text(json.dumps({'Replace': {str(file): str(replacement)}}))
    result = subprocess.run(['go', 'test', '-overlay', str(overlay), './backend', '-run', '^(' + '|'.join(tests) + ')$', '-count=1'], capture_output=True, text=True)
    log = result.stdout + result.stderr
    (directory / 'test.log').write_text(log)
    assert result.returncode != 0 and '[build failed]' not in log and 'syntax error' not in log, log[-2000:]
    for test in tests:
        assert re.search(r'--- FAIL: ' + re.escape(test) + r'\s', log), name + ': missing assertion FAIL for ' + test + '\n' + log[-2000:]
    results.append({'mutation': name, 'overlay': str(overlay), 'assertionFailures': tests, 'result': 'mutant killed'})
report = root / 'docs/history/reports/r194'
report.mkdir(parents=True, exist_ok=True)
(report / 'mutations.json').write_text(json.dumps(results, ensure_ascii=False, indent=2) + '\n')
print(json.dumps(results, ensure_ascii=False))
