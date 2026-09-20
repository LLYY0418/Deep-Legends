"""Executable R103 mutations; temporary overlays, never mutate the checkout."""
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parent.parent
BACKEND = ROOT / 'backend'
OUT = ROOT / "docs/r103-validation/mutations"
OUT.mkdir(parents=True, exist_ok=True)
TMP = Path("/tmp/deep-legends-go-tmp")
TMP.mkdir(parents=True, exist_ok=True)
ENV = dict(os.environ, GOCACHE="/tmp/deep-legends-go-cache", GOTMPDIR=str(TMP))
START = os.environ.get("R103_MUTATION_FROM", "")
MATRIX = OUT / "matrix.json"
RESULTS = json.loads(MATRIX.read_text()) if START and MATRIX.exists() else []
RESULTS = [result for result in RESULTS if result["id"] < START] if START else []


def probe(name, file, old, new, test):
    if START and name < START:
        return
    source_path = BACKEND / file
    source = source_path.read_text()
    assert source.count(old) == 1, (name, source.count(old))

    command = [
        "/opt/homebrew/bin/go",
        "test",
        "-count=1",
        "-v",
        "-timeout=40s",
        "-run",
        "^" + test + "$",
        "./backend",
    ]
    baseline = subprocess.run(
        command,
        cwd=ROOT,
        env=ENV,
        capture_output=True,
        text=True,
        timeout=90,
    )
    baseline_log = baseline.stdout + baseline.stderr
    (OUT / (name + "-baseline.log")).write_text(baseline_log)
    assert baseline.returncode == 0, (
        name,
        "baseline",
        baseline_log[-2000:],
    )

    with tempfile.TemporaryDirectory(prefix="r103-mutation-") as temporary:
        temporary_path = Path(temporary)
        changed_path = temporary_path / source_path.name
        changed_path.write_text(source.replace(old, new, 1))
        overlay_path = temporary_path / "overlay.json"
        overlay_path.write_text(
            json.dumps({"Replace": {str(source_path): str(changed_path)}})
        )
        mutant_command = command[:2] + ["-overlay", str(overlay_path)] + command[2:]
        mutant = subprocess.run(
            mutant_command,
            cwd=ROOT,
            env=ENV,
            capture_output=True,
            text=True,
            timeout=90,
        )

    mutant_log = mutant.stdout + mutant.stderr
    (OUT / (name + ".log")).write_text(mutant_log)
    expected_failure = "FAIL: " + test in mutant_log
    invalid_failure = any(
        marker in mutant_log
        for marker in (
            "build failed",
            "could not import",
            "undefined:",
            "syntax error",
            "test timed out",
            "panic:",
            "SyntaxError",
        )
    )
    killed = mutant.returncode != 0 and expected_failure and not invalid_failure
    result = {
        "id": name,
        "file": file,
        "test": test,
        "baseline_exit": baseline.returncode,
        "mutant_exit": mutant.returncode,
        "killed": killed,
    }
    RESULTS.append(result)
    MATRIX.write_text(json.dumps(RESULTS, indent=2) + "\n")
    print(name, "KILLED" if killed else "INVALID/SURVIVED", flush=True)
    assert killed, mutant_log[-2000:]


probe(
    "01-unprotected-proseed",
    "binary_disk_budget.go",
    'if strings.HasPrefix(filepath.Base(key), "proseed-") {',
    'if false && strings.HasPrefix(filepath.Base(key), "proseed-") {',
    "TestR103ProseedEntriesNeverSelectedForEviction",
)
probe(
    "02-protected-budget-error-removed",
    "binary_disk_budget.go",
    'return errors.New("protected cache entries exceed disk budget")',
    'if false { return errors.New("protected cache entries exceed disk budget") }; return nil',
    "TestR103AllCandidatesProtectedReturnsErrorNotEviction",
)
