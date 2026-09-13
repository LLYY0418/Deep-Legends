"""Run R80-H behavioral mutations in a disposable copy of installer/."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
ENV = dict(os.environ, GOCACHE=str(ROOT / ".gocache"), GOMODCACHE=str(ROOT / ".gomodcache"), GOTMPDIR="/tmp", TMPDIR="/tmp")
CASES = [
    ("directory disappears before Core exit", "progress.go", "if remaining.Missing {", "if false {"),
    ("deleteData true must remove backend cache", "finish.go", "if deleteData {", "if false {"),
    ("deleteData false must preserve backend cache", "finish.go", "if deleteData {", "if !deleteData {"),
    ("silent mode must not create a window", "main.go", "if options.silent() {", "if !options.silent() {"),
    ("nonzero Core exit must not report done", "finish.go", "if code != 0 {", "if false {"),
    ("Core must receive Electron data-delete flag", "pathrules.go", "if deleteData {", "if false {"),
    ("Core must retain NSIS directory tail", "pathrules.go", '\" _?=\"', '\" /D=\"'),
    ("uninstall progress must not regress", "progress.go", "max(m.shown, min(percent, 97))", "min(percent, 97)"),
]
results = []
with tempfile.TemporaryDirectory(prefix="r80-uninstall-mutations-") as temporary:
    work = Path(temporary) / "installer"
    shutil.copytree(ROOT / "installer", work)
    def run():
        return subprocess.run(["go", "test", "./uninstall"], cwd=work, env=ENV, capture_output=True, text=True)
    baseline = run()
    if baseline.returncode:
        raise RuntimeError(baseline.stdout + baseline.stderr)
    print("Unmodified baseline PASS", flush=True)
    for label, filename, old, new in CASES:
        file = work / "uninstall" / filename
        original = file.read_text()
        assert original.count(old) == 1, (filename, old)
        file.write_text(original.replace(old, new, 1))
        result = run()
        caught = result.returncode != 0 and "--- FAIL:" in result.stdout
        results.append({"mutation": label, "caughtByTest": caught})
        print(label, "CAUGHT" if caught else "MISSED", flush=True)
        if not caught:
            print(result.stdout + result.stderr, flush=True)
        file.write_text(original)
output = ROOT / "output/installer-r80/uninstall-mutations.json"
output.parent.mkdir(parents=True, exist_ok=True)
output.write_text(json.dumps(results, ensure_ascii=False, indent=2) + "\n")
raise SystemExit(0 if all(item["caughtByTest"] for item in results) else 1)
