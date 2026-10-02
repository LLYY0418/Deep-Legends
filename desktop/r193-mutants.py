from pathlib import Path
import subprocess, json, os

root = Path.cwd()
out = Path('/private/tmp/r193-mutants')
out.mkdir(exist_ok=True)
source = (root / 'backend/web/gameplay.js').read_text()
results = []
for name, old, new, test in [
    ('fake-hidden-rank', 'player.hidden === true ? (player.position ? positionLabel(player.position) : "") : arenaMode', 'arenaMode', 'R193 1 '),
    ('duplicate-bottom-message', '    if (player.hidden === true) return "";\n', '', 'R193 2 '),
]:
    assert source.count(old) == 1, name
    file = out / (name + '.js')
    file.write_text(source.replace(old, new, 1))
    result = subprocess.run(['node', '--test', '--test-name-pattern=' + test, 'backend/web/r193.test.cjs'], env={**os.environ, 'R193_SOURCE_FILE': str(file)}, capture_output=True, text=True)
    log = result.stdout + result.stderr
    (out / (name + '.log')).write_text(log)
    assert result.returncode != 0 and 'ERR_ASSERTION' in log and 'ReferenceError' not in log and 'SyntaxError' not in log, log[-2000:]
    results.append({'mutation': name, 'test': test.strip(), 'result': 'assertion FAIL (mutant killed)'})
report = root / 'docs/history/reports/r193'
report.mkdir(parents=True, exist_ok=True)
(report / 'mutations.json').write_text(json.dumps(results, ensure_ascii=False, indent=2) + '\n')
print(json.dumps(results, ensure_ascii=False))
