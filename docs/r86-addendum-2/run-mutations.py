"""Check ADD2 guards via Go overlays and disposable JS files; never edit sources."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[2])
parser.add_argument("--output", type=Path, required=True)
args = parser.parse_args()
root = args.root.resolve()
env = {**os.environ, "GOCACHE": os.environ.get("GOCACHE", str(root / ".gocache")),
       "GOTMPDIR": tempfile.gettempdir()}
results = []


def run(command):
    return subprocess.run(command, cwd=root, env=env, text=True, errors="replace",
                          stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=120)


go_cases = [
    ("wrong-lock-mode", "mode = 0o444", "mode = 0o600",
     "TestSettingsLockUsesTencentConfigAndRejectsSymlink", "lock status = 503"),
    ("verification-bypassed", "if !status.SettingsKnown || status.SettingsLocked != request.Locked {",
     "if false {", "TestSettingsLockRejectsUnverifiedResult", "want 503 verification failure"),
]
with tempfile.TemporaryDirectory(prefix="r86-add2-mutations-") as temporary:
    for name, before, after, test, evidence in go_cases:
        source = root / "game_settings_lock.go"
        original = source.read_text()
        assert original.count(before) == 1, f"{name}: ambiguous mutation boundary"
        replacement = Path(temporary) / "game_settings_lock.go"
        replacement.write_text(original.replace(before, after, 1))
        overlay = Path(temporary) / "overlay.json"
        overlay.write_text(json.dumps({"Replace": {str(source): str(replacement)}}))
        command = ["go", "test", "-json", "-overlay", str(overlay), ".", "-run", f"^{test}$", "-count=1"]
        execution = run(command)
        events = []
        for line in execution.stdout.splitlines():
            try:
                events.append(json.loads(line))
            except json.JSONDecodeError:
                pass
        output_by_test = {}
        for event in events:
            if event.get("Test") and "Output" in event:
                output_by_test.setdefault(event["Test"], []).append(event["Output"])
        assertions = [{"test": name, "output": "".join(lines).strip()}
                      for name, lines in output_by_test.items() if evidence in "".join(lines)]
        killed = (execution.returncode != 0 and bool(assertions) and
                  any(e.get("Action") == "fail" and e.get("Test") == test for e in events))
        results.append({"mutation": name, "file": source.name, "before": before, "after": after,
                        "command": command, "killed": killed, "assertionEvidence": assertions})
        print(f"{name}: killed={killed}", flush=True)
        if not killed:
            print(execution.stdout[-4000:])

source = root / "desktop/build-fingerprint.test.cjs"
original = source.read_text()
for directory in [".gocache", ".gomodcache", "node_modules", "dist"]:
    # Bypass just one policy entry at a time; fixture names are independent literals.
    before = "filter: (file) => !excludedDirectories.has(path.basename(file)),"
    after = f'filter: (file) => path.basename(file) === "{directory}" || !excludedDirectories.has(path.basename(file)),'
    assert original.count(before) == 1, "ambiguous JS mutation boundary"
    with tempfile.NamedTemporaryFile(mode="w", suffix=".cjs", prefix="r86-add2-mutation-",
                                     dir=root / "desktop", delete=False) as handle:
        handle.write(original.replace(before, after, 1))
        replacement = Path(handle.name)
    try:
        command = ["node", "--test", "--test-reporter=tap", "--test-name-pattern=R86 ADD2", str(replacement)]
        execution = run(command)
    finally:
        replacement.unlink()
    assertions = [line.strip() for line in execution.stdout.splitlines() if "must not be copied" in line]
    killed = (execution.returncode != 0 and bool(assertions) and
              "not ok 1 - R86 ADD2 fingerprint fixture" in execution.stdout and
              any(directory in line for line in assertions))
    results.append({"mutation": f"copy-{directory}", "file": "desktop/build-fingerprint.test.cjs",
                    "before": before, "after": after, "command": command, "killed": killed,
                    "assertionEvidence": assertions})
    print(f"copy-{directory}: killed={killed}", flush=True)
    if not killed:
        print(execution.stdout[-4000:])

args.output.parent.mkdir(parents=True, exist_ok=True)
args.output.write_text(json.dumps(results, ensure_ascii=False, indent=2) + "\n")
raise SystemExit(0 if all(result["killed"] for result in results) else 1)
