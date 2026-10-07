import datetime
import json
import os
import pathlib
import subprocess
import sys
import time
import traceback

root = pathlib.Path(__file__).resolve().parents[4]
out = pathlib.Path(os.environ.get("R238_CHECK_OUT", pathlib.Path(__file__).resolve().parent))
out.mkdir(parents=True, exist_ok=True)
name, command = sys.argv[1], sys.argv[2:]
cwd = pathlib.Path(os.environ.get("R238_CHECK_CWD", root))
start = datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8)))
name += "-" + start.strftime("%Y%m%dT%H%M%S.%f")
begin = time.monotonic()
with (out / (name + ".log")).open("x") as log:
    try:
        result = subprocess.run(command, cwd=cwd, stdout=log, stderr=subprocess.STDOUT)
    except OSError:
        traceback.print_exc(file=log)
        result = subprocess.CompletedProcess(command, 127)
end = datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8)))
lines = (out / (name + ".log")).read_text(errors="replace").splitlines()
row = dict(name=name, command=command, cwd=str(cwd), start=start.isoformat(), end=end.isoformat(),
           elapsed_seconds=round(time.monotonic() - begin, 3), exit_code=result.returncode, last_five=lines[-5:])
(out / (name + ".json")).write_text(json.dumps(row, ensure_ascii=False, indent=2) + "\n")
print(json.dumps(row, ensure_ascii=False), flush=True)
sys.exit(result.returncode)
