"""R76: real source mutations in an isolated physical copy, never the worktree."""
from pathlib import Path
import json, os, shutil, subprocess, tempfile

root = Path(__file__).resolve().parents[1]
out = root / 'output/playwright/r76/mutations'
out.mkdir(parents=True, exist_ok=True)
clone = Path(tempfile.mkdtemp(prefix='r76-mutation-', dir='/private/tmp'))
for f in root.glob('*.go'):
    shutil.copy2(f, clone / f.name)
for f in ['go.mod', 'go.sum', 'prestige_chromas.json']:
    shutil.copy2(root / f, clone / f)
for d in ['web', 'data', 'testdata']:
    shutil.copytree(root / d, clone / d)
env = dict(os.environ, GOCACHE=str(root / '.gocache'), GOTMPDIR='/private/tmp')
go = ['go', 'test', '-count=1', '-v', '-run', '^TestProRune', '.']
js = ['node', '--test', '--test-name-pattern=R76 pro degradation', 'web/gameplay.test.cjs']
mutations = [
    ('MG7', 'pro_runes.go',
     '\tif !teams[w.Metadata.Blue.TeamID] || !teams[w.Metadata.Red.TeamID] || w.Metadata.Blue.TeamID == w.Metadata.Red.TeamID {\n\t\treturn row, errors.New("livestats team mismatch")\n\t}\n', '',
     ['go', 'test', '-count=1', '-v', '-run', '^TestProRuneIndexGameRejectsLivestatsTeamMismatch$', '.'],
     'want livestats team mismatch, got <nil>'),
    ('MF-threshold', 'web/gameplay.js',
     'const stale = status.stale || (status.readAt && Date.now() - Date.parse(status.readAt) > 300000);',
     'const stale = status.stale || (status.readAt && Date.now() - Date.parse(status.readAt) > 300000) || status.reason && status.reason !== "no-sample" && status.reason !== "preparing";',
     js, '1/99 is not whole-source failure'),
    ('MF-backend-reason', 'pro_runes_recommendations.go',
     '(response.Reason != "" && response.Failed == 0)', 'response.Reason != ""',
     ['go', 'test', '-count=1', '-v', '-run', '^TestProRunePartialFailureCounts$', '.'],
     'wrong refresh coverage'),
    ('MF-double-count', 'pro_runes_recommendations.go',
     'response.Failed = len(failedGames)', 'response.Failed++',
     ['go', 'test', '-count=1', '-v', '-run', '^TestProRuneFailureCountsDeduplicateDetailAndTerminal$', '.'],
     'duplicate game or falsely stale'),
]
original = {file: (root / file).read_bytes() for _, file, *_ in mutations}
results = []
def run(name, cmd):
    proc = subprocess.run(cmd, cwd=clone, env=env, capture_output=True, text=True, timeout=120)
    log = proc.stdout + proc.stderr
    (out / (name + '.log')).write_text(log)
    return proc.returncode, log

for name, cmd in [('baseline-go', go), ('baseline-js', js)]:
    code, log = run(name, cmd)
    assert code == 0, (name, log)
try:
    for name, file, before, after, cmd, expected in mutations:
        text = original[file].decode()
        assert text.count(before) == 1, (name, 'ambiguous mutation target')
        (clone / file).write_text(text.replace(before, after, 1))
        code, log = run(name, cmd)
        killed = code != 0 and expected in log and 'build failed' not in log
        results.append(dict(mutation=name, killed=killed, exit=code, assertion=expected))
        (clone / file).write_bytes(original[file])
        restored, restored_log = run(name + '-restored', cmd)
        results[-1]['restoredPass'] = restored == 0
        assert killed and restored == 0, (name, log, restored_log)
        print(name, 'KILLED; restored PASS', flush=True)
finally:
    for file, data in original.items():
        (clone / file).write_bytes(data)
    (out / 'results.json').write_text(json.dumps({'clone':str(clone),'results':results}, indent=2))
    assert all((root / file).read_bytes() == data for file, data in original.items()), 'worktree changed during verification; review before accepting results'
print('All mutants killed by assertions; live sources unchanged.', flush=True)
