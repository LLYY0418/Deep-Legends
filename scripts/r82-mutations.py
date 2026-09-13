#!/usr/bin/env python3
"""Run the R82 handoff mutations against copies of production code; no packaging."""
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
SOURCE = (ROOT / "installer/handoff.go").read_text()
TESTS = (ROOT / "installer/handoff_test.go").read_text()
MUTATIONS = {
    "no timeout": ("case <-deadline:\n\t\t\treturn false", "case <-deadline:\n\t\t\tcontinue"),
    "no PID match": ("window.PID == pid && ", ""),
    "close before launch": ("h.ShowStarting()", "h.ShowStarting()\n\th.CloseInstaller(false)"),
    "no visibility check": ("window.Visible && ", ""),
}


def run(source):
    with tempfile.TemporaryDirectory(prefix="r82-handoff-") as directory:
        target = Path(directory)
        (target / "go.mod").write_text("module r82-handoff-mutations\n\ngo 1.23\n")
        (target / "handoff.go").write_text(source)
        (target / "handoff_test.go").write_text(TESTS)
        env = dict(os.environ, GOCACHE=str(ROOT / ".gocache"), GOTMPDIR=tempfile.gettempdir())
        result = subprocess.run(["go", "test", "-count=1", "-timeout=5s", "."], cwd=target,
                                env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=30)
        return result.returncode, result.stdout


if __name__ == "__main__":
    status, output = run(SOURCE)
    if status:
        raise SystemExit("Baseline failed:\n" + output)
    print("PASS baseline", flush=True)
    for name, (before, after) in MUTATIONS.items():
        if SOURCE.count(before) != 1:
            raise SystemExit(f"Mutation target changed: {name}")
        status, output = run(SOURCE.replace(before, after))
        if not status:
            raise SystemExit(f"SURVIVED: {name}")
        if "--- FAIL:" not in output:
            raise SystemExit(f"Mutation did not reach assertions: {name}\n{output}")
        print(f"KILLED {name}", flush=True)
