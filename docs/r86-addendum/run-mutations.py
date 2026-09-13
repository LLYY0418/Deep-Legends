"""Run the acceptance follow-up's Go mutations without editing production files."""
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
ENV = {**os.environ, "GOCACHE": os.environ.get("GOCACHE", str(ROOT / ".gocache")),
       "GOTMPDIR": os.environ.get("GOTMPDIR", tempfile.gettempdir())}
CASES = [
    ("R86-ADD-2", "champion_cache.go",
     'protected := strings.HasPrefix(item.Name(), "hexdata-")', 'protected := false',
     "TestR86ChampionDiskMigrationPruneAndThrottle", 20, "prune deleted recovery data"),
    ("R86-ADD-3", "riot_api.go",
     "\t\t\tselect {\n\t\t\tcase semaphore <- struct{}{}:\n\t\t\tcase <-ctx.Done():\n\t\t\t\treturn\n\t\t\t}",
     "\t\t\tsemaphore <- struct{}{}",
     "TestR86RiotQueuedDetailsExitWhileActiveRequestsHoldSlots", 5,
     "detail workers=12, want 4 while all four active slots remain held"),
]
results = []
with tempfile.TemporaryDirectory(prefix="r86-add-mutations-") as tmp:
    for name, filename, old, new, test, repetitions, failure in CASES:
        source_path = ROOT / filename
        source = source_path.read_text(encoding="utf-8")
        assert source.count(old) == 1, f"{name}: mutation boundary missing or ambiguous"
        replacement = Path(tmp) / filename
        replacement.write_text(source.replace(old, new, 1), encoding="utf-8")
        overlay = Path(tmp) / "overlay.json"
        overlay.write_text(json.dumps({"Replace": {str(source_path): str(replacement)}}), encoding="utf-8")
        command = ["go", "test", "-json", "-overlay", str(overlay), ".", "-run", f"^{test}$",
                   f"-count={repetitions}", "-timeout=90s"]
        run = subprocess.run(command, cwd=ROOT, env=ENV, encoding="utf-8", errors="replace",
                             stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=150)
        events = []
        for line in run.stdout.splitlines():
            try:
                events.append(json.loads(line))
            except json.JSONDecodeError:
                pass
        failures = sum(e.get("Test") == test and e.get("Action") == "fail" for e in events)
        passes = sum(e.get("Test") == test and e.get("Action") == "pass" for e in events)
        matching_failures = [e["Output"].strip() for e in events if e.get("Test") == test and failure in e.get("Output", "")]
        killed = run.returncode != 0 and failures == repetitions and passes == 0 and len(matching_failures) == repetitions
        results.append({"item": name, "test": test, "mutation": {"file": filename, "before": old, "after": new}, "command": command, "repetitions": repetitions, "failed": failures,
                        "passed": passes, "killedEveryTime": killed, "assertionEvidence": matching_failures})
        print(f"{name}: {failures}/{repetitions} killed; {passes} survived", flush=True)
        if not killed:
            print(run.stdout[-4000:], flush=True)
(ROOT / "docs/r86-addendum/mutation-results.json").write_text(
    json.dumps(results, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
raise SystemExit(0 if all(r["killedEveryTime"] for r in results) else 1)
