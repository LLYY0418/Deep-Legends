import datetime
import json
import os
import pathlib
import subprocess
import sys
import time

root = pathlib.Path(__file__).resolve().parents[4]
out = pathlib.Path(__file__).resolve().parent
name, command = sys.argv[1], sys.argv[2:]
cwd = pathlib.Path(os.environ.get("R238_CHECK_CWD", root))
start = datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8)))
begin = time.monotonic()
with (out / (name + ".log")).open("w") as log:
    result = subprocess.run(command, cwd=cwd, stdout=log, stderr=subprocess.STDOUT)
end = datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8)))
lines = (out / (name + ".log")).read_text(errors="replace").splitlines()
row = dict(name=name, command=command, cwd=str(cwd), start=start.isoformat(), end=end.isoformat(),
           elapsed_seconds=round(time.monotonic() - begin, 3), exit_code=result.returncode, last_five=lines[-5:])
(out / (name + ".json")).write_text(json.dumps(row, ensure_ascii=False, indent=2) + "\n")
print(json.dumps(row, ensure_ascii=False), flush=True)
sys.exit(result.returncode)
