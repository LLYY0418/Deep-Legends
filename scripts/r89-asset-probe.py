"""Real service / real upstream measurements; token stays inside this process."""
import concurrent.futures
import http.cookiejar
import json
import pathlib
import sys
import time
import urllib.parse
import urllib.request

base, data_dir, output = sys.argv[1:4]
token = (pathlib.Path(data_dir) / "session-token").read_text().strip()
headers = {"X-Local-Token": token}
def get(path):
    started = time.perf_counter()
    with urllib.request.urlopen(urllib.request.Request(base + path, headers=headers), timeout=30) as response:
        body = response.read()
        return body, response.status, (time.perf_counter()-started)*1000

catalog, status, elapsed = get("/api/gameplay/items")
items = json.loads(catalog)["items"]
paths = ["/api/champion-asset?" + urllib.parse.urlencode({"source":"ddragon", "path":item["iconPath"][8:]}) for item in items if item["iconPath"].startswith("ddragon:")][:100]
result = {"catalog": {"status":status, "items":len(items), "duration_ms":elapsed}, "same":[], "rounds":[]}
for _ in range(3):
    _, status, elapsed = get(paths[0])
    result["same"].append({"status":status, "duration_ms":elapsed})
for _ in range(2):
    started = time.perf_counter()
    with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
        rows = list(pool.map(get, paths))
    durations = sorted(row[2] for row in rows)
    result["rounds"].append({"count":len(rows), "duration_ms":(time.perf_counter()-started)*1000,"p50_ms":durations[49],"p90_ms":durations[89],"failures":sum(row[1]!=200 for row in rows)})
pathlib.Path(output).write_text(json.dumps(result, indent=2)+"\n")
print(json.dumps(result, indent=2))
