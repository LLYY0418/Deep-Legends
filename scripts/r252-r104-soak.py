"""Run separate R104 processes without retries; retain every result and timeline."""
import argparse
import datetime
import json
import pathlib
import subprocess
import time

parser = argparse.ArgumentParser()
parser.add_argument("--binary", required=True)
parser.add_argument("--output", required=True)
parser.add_argument("--iterations", type=int, default=30)
args = parser.parse_args()
assert args.iterations >= 30
out = pathlib.Path(args.output)
out.mkdir(parents=True, exist_ok=False)
binary = str(pathlib.Path(args.binary).resolve())
rows = []
for iteration in range(1, args.iterations + 1):
    started = datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8)))
    tick = time.monotonic()
    command = [binary, "-test.v", "-test.count=1", "-test.run=^TestR104ColdStartupDripProtectsForegroundTwentyMatches$", "-test.timeout=45s"]
    with (out / f"iteration-{iteration:02d}.log").open("x") as log:
        result = subprocess.run(command, cwd=pathlib.Path(__file__).resolve().parents[1] / "backend", stdout=log, stderr=subprocess.STDOUT)
    text = (out / f"iteration-{iteration:02d}.log").read_text(errors="replace")
    passed = result.returncode == 0 and "--- PASS: TestR104ColdStartupDripProtectsForegroundTwentyMatches" in text
    row = dict(iteration=iteration, start=started.isoformat(), elapsed_seconds=round(time.monotonic()-tick, 3), exit_code=result.returncode, passed=passed, timeline_logged="R252_R104" in text)
    rows.append(row)
    print(json.dumps(row), flush=True)
failures = sum(not row["passed"] for row in rows)
summary = dict(iterations=len(rows), failures=failures, failure_rate=failures/len(rows), independent_processes=True, retries=0, foreground_wait_seconds=5, rows=rows)
with (out / "summary.json").open("x") as file:
    json.dump(summary, file, indent=2)
    file.write("\n")
raise SystemExit(1 if failures else 0)
