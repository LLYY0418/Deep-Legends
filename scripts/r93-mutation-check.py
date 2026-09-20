"""Prove the regular 20-match test rejects network fallback; never edit production."""
import hashlib
import json
import os
import pathlib
import subprocess
import tempfile
from datetime import datetime, timezone

ROOT = pathlib.Path(__file__).resolve().parent.parent
BACKEND = ROOT / 'backend'
OUTPUT = pathlib.Path(os.environ.get("R93_MUTATION_OUTPUT", "/tmp/deep-legends-r93/mutation"))
OUTPUT.mkdir(parents=True, exist_ok=True)
ENV = dict(os.environ, GOCACHE=os.environ.get("GOCACHE", "/tmp/deep-legends-r89/go-cache"))
TEST = "TestR93TwentyMatchDetailsFromDiskAfterRestart"
SOURCE = BACKEND / "riot_api.go"
original = SOURCE.read_bytes()


def run(name, overlay=None):
    command = ["go", "test", "./backend", "-run", "^" + TEST + "$", "-count=1", "-v", "-timeout=60s"]
    if overlay:
        command.extend(["-overlay", str(overlay)])
    result = subprocess.run(command, cwd=ROOT, env=ENV, capture_output=True, text=True, timeout=90)
    log = result.stdout + result.stderr
    (OUTPUT / (name + ".log")).write_text(log)
    return result.returncode, log


if run("baseline")[0]:
    raise SystemExit("Baseline failed; mutation not attempted")
with tempfile.TemporaryDirectory(prefix="r93-disk-mutation-") as directory:
    directory = pathlib.Path(directory)
    anchor = "if p.matchDisk != nil && validRiotMatchID(matchID) {"
    source = original.decode()
    if source.count(anchor) != 1:
        raise SystemExit("Disk-read anchor changed; inspect before retrying")
    mutated = directory / "riot_api.go"
    mutated.write_text(source.replace(anchor, "if false && p.matchDisk != nil && validRiotMatchID(matchID) {"))
    overlay = directory / "overlay.json"
    overlay.write_text(json.dumps({"Replace": {str(SOURCE): str(mutated)}}))
    code, log = run("disk-read-disabled", overlay)
    killed = code != 0 and "restart disk_hits=0 network_calls=20" in log and "[build failed]" not in log
    if not killed:
        raise SystemExit("Mutation survived or failed for an unrelated reason")
if SOURCE.read_bytes() != original:
    raise SystemExit("Production source changed during the check; inspect concurrent edits")
if run("restored")[0]:
    raise SystemExit("Restored baseline failed")
summary = {
    "checked_at_utc": datetime.now(timezone.utc).isoformat(),
    "test": TEST,
    "baseline": "passed",
    "mutation": "disk read disabled via Go overlay",
    "mutant": "failed: disk_hits=0 network_calls=20",
    "restored": "passed",
    "production_source_sha256": hashlib.sha256(original).hexdigest(),
    "production_source_unchanged": True,
}
(OUTPUT / "results.json").write_text(json.dumps(summary, indent=2) + "\n")
print(json.dumps(summary, indent=2))
